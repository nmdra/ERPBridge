# Plan: Harden Plugin and ERP Credentials

## Goal

Allow ERPBridge to authenticate an external plugin with an environment-backed
bearer token or API key, classify a plugin as `api` or `docker`, and remove
legacy plaintext ERP credentials from the local API registry. The work must
prevent a control-plane caller from turning ERPBridge into a secret-exfiltration
or SSRF proxy, preserve the plugin `/v1/process` payload and unbound-tool
behavior, and require protected transport for credentialed outbound calls.

## Current State

- `PluginMetadata` has only name, version, and active state, and `PluginSpec`
  has only an endpoint and timeout (`internal/mcp/plugin.go:43-63`).
  `PluginClient.Process` sends only JSON content headers, with no endpoint-auth
  mechanism (`internal/mcp/plugin_client.go:51-103`).
- Tool credentials are references resolved from the process environment at call
  time, and a missing reference fails closed (`internal/mcp/tool.go:203-207`,
  `internal/mcp/tool.go:338-347`).
- Plugin resources are accepted through the admin route only when
  `API_AUTH_TOKEN` is configured. In open mode, all management routes are open
  (`internal/mcp/auth.go:23-59`; `internal/mcp/server.go:690-701`). A plugin
  endpoint may currently be any absolute HTTP(S) URL
  (`internal/mcp/plugin.go:235-252`), so an authenticated-plugin feature must
  not let an open-mode caller select an endpoint and an arbitrary environment
  variable.
- The plugin client already disables redirects. The ERP connector applies
  credentials before `http.Client.Do`, retries transient errors, and currently
  follows redirects (`internal/mcp/plugin_client.go:35-48`,
  `internal/connector/client.go:73-88`, `internal/connector/client.go:91-147`).
- SQLite persists tools and plugins as JSON `TEXT`, not encrypted secret data
  (`internal/mcp/store.go:49-99`; `internal/mcp/plugin_store.go:25-49`). The
  local IDP registry instead has raw `AuthKey` and `AuthToken` fields and writes
  `~/.bridgectl/registry.json` with permission `0600`
  (`internal/idp/registry.go:16-30`, `internal/idp/registry.go:96-125`).
- Both generator paths ignore the registered API credential and hard-code the
  `ERP_PRIMARY_KEY` reference (`internal/idp/generator.go:61-70`,
  `internal/idp/generator.go:208-216`). `bridgectl api register --auth-key`
  writes the raw key, and `bridgectl api test` reads it
  (`internal/cli/api.go:33-61`, `internal/cli/api.go:123-157`).
- `logger.RedactAttr` recognizes common secret keys and headers
  (`internal/logger/redact.go:18-55`), but the root output handler does not
  install it (`internal/logger/logger.go:111-141`), and connector debug logs
  emit truncated raw request/response bodies (`internal/connector/client.go:101-105`,
  `internal/connector/client.go:168-171`).
- The default Compose stack calls authenticated MockERP through HTTP
  (`docker-compose.yml:41-49`); the plugin integration fixture calls an HTTP
  plugin and also supplies `ERP_PRIMARY_KEY`
  (`internal/integration/plugin_system_test.go:31-39`,
  `docker-compose.plugin-test.yml:12-14`).
- OWASP recommends HTTPS for authenticated REST calls and says API keys must
  not appear in URLs:
  <https://cheatsheetseries.owasp.org/cheatsheets/REST_Security_Cheat_Sheet.html>.
  RFC 6750 defines bearer credentials in the `Authorization` header:
  <https://www.rfc-editor.org/rfc/rfc6750.html>.

## Decisions

1. **Persist references, never credential values, in outbound-resource
   paths.** Tool, plugin, and local API resources contain only `credentialRef`
   names. Resolved values exist only in process memory while a request is built.
   This plan does not redesign separately configured inbound client tokens.
   ERPBridge will not add application-level encryption because it will no longer
   intentionally persist outbound credential material; operators encrypt
   volumes and use an external secret store where required.

2. **Make plugin type canonical and descriptive.** Add
   `metadata.type: api|docker`. An omitted type is canonicalized to `api`
   before validation and persistence, so get/list/CLI output is unambiguous.
   `docker` means an operator deploys a container; ERPBridge still only invokes
   `spec.endpoint` and never pulls, starts, or manages an image.

3. **Use a narrow optional plugin auth contract.** Add:

   ```yaml
   spec:
     auth:
       type: bearer             # bearer or api-key
       credentialRef: PLUGIN_FOO_TOKEN
       header: X-API-Key        # api-key only; default X-API-Key
   ```

   `bearer` sends `Authorization: Bearer <value>`. `api-key` sends the raw
   value in the default or declared header. Auth is absent when `spec.auth` is
   absent. Reject an auth `header` for bearer auth, invalid HTTP token header
   names, and reserved or hop-by-hop headers (`Authorization`, `Content-Type`,
   `Accept`, `Host`, `Connection`, `Transfer-Encoding`, `Cookie`, and
   `Proxy-Authorization`).

4. **Fail closed at every plugin manifest ingress.** HTTP apply and CLI
   JSON/YAML parsing must reject unknown fields, including `token`, `value`,
   `password`, and arbitrary auth properties, rather than dropping them.
   Validate `credentialRef` as an environment-variable identifier and require
   plugin references to begin `PLUGIN_`; this prevents selecting unrelated
   ERPBridge process secrets such as `API_AUTH_TOKEN` or `ERP_PRIMARY_KEY`.

5. **Require a protected control plane and endpoint allowlist for plugin auth.**
   Applying or updating a plugin with `spec.auth` requires `API_AUTH_TOKEN` to
   be configured and the authenticated admin route to be used. It also requires
   the endpoint's normalized `host:port` to appear in the exact, comma-separated
   `PLUGIN_ENDPOINT_ALLOWLIST`. Empty allowlist means credentialed plugin
   resources are rejected. This is an application guard, not a replacement for
   network egress policy.

6. **Credentialed calls use HTTPS, exact HTTP exceptions, and no redirects.**
   Plugin and ERP calls carrying a credential reject `http://` by default. An
   exact comma-separated `INSECURE_AUTH_ALLOWED_HOSTS` host:port allowlist is
   the only override; each accepted HTTP call logs a warning with no secret.
   The allowlist is for private development fixtures, not production. Disable
   redirects for credentialed ERP calls as well as plugin calls, preventing a
   secure initial URL from redirecting a credential to a different or insecure
   destination. Unauthenticated HTTP calls retain their current behavior.

7. **Make legacy credential removal deliberate and non-silent.** Replace
   `idp.API.AuthKey` and `AuthToken` with `CredentialRef`. Loading a registry
   that contains legacy secret fields marks it as legacy and blocks every
   state-changing operation and `api test` until the user runs the confirmed
   destructive `bridgectl api scrub-credentials --yes`. Scrub atomically
   rewrites only non-secret metadata, does not create another plaintext backup,
   and never prints old values. Operators then set a reference per API with
   `bridgectl api set-credential-ref <name> --credential-ref NAME`; generation
   and testing require that reference for authenticated APIs.

8. **Protect logs and responses.** Install `RedactAttr` in every root log sink,
   stop logging ERP request/response bodies, and emit only safe metadata such
   as method, endpoint identity, status, and duration. Plugin errors, APIs,
   CLI output, stored JSON, and test diagnostics must never expose resolved
   credentials, authorization values, raw legacy fields, or plugin response
   bodies.

9. **Rotation means deployment refresh.** Environment changes normally require
   an ERPBridge restart or rollout. Operators rotate a value, restart/roll out
   ERPBridge, then use the authenticated cache-flush endpoint when cached tool
   output must be recomputed immediately. No automatic environment reload or
   secret-manager integration is part of this change.

## Scope

### In scope

- Plugin metadata type, strict resource parsing, bearer/API-key auth, endpoint
  allowlisting, and reference-prefix validation.
- Protected transport and redirect policy for credentialed ERP/plugin calls.
- Systematic log/body redaction hardening.
- Migration away from raw local-registry ERP credentials with explicit scrub and
  per-API reference assignment.
- Unit, API, runtime, Compose, black-box, developer-doc, public-doc, and
  changelog coverage.

### Out of scope

- OAuth, Basic Auth, HMAC signing, mTLS, certificate management, dynamic secret
  providers, or application-level database encryption.
- Plugin deployment, image management, health discovery, retries, or changes
  to the plugin JSON request/response envelope.
- Automatic migration of a raw credential to an environment variable, or a
  plaintext backup of legacy credentials.
- Replacing operator network policy, disk encryption, or secret-manager access
  control.

## Tasks

Every code task follows red → green → refactor. Run its focused `Verify:`
command and `make test` before its stated Conventional Commit; update the
listed in-repository docs and `CHANGELOG.md` in that same commit.

- [x] **Task 1: Close existing credential log exposure.** Write failing logger
  and connector tests first. Install `RedactAttr` in the root text/JSON handler
  and replace connector body logging with safe size/metadata logging. Prove
  request bodies, ERP response bodies, authorization values, and sensitive
  structured attributes do not reach stdout, buffered logs, or MCP log
  notifications.
  (**Seam:** root logger construction and ERP connector observability;
  **Files:** `internal/logger/logger.go`, `internal/logger/logger_test.go`,
  `internal/logger/redact.go`, `internal/connector/client.go`,
  `internal/connector/client_test.go`, `docs/architecture.md`, `CHANGELOG.md`;
  **Verify:** `go test ./internal/logger ./internal/connector -run 'Test.*(Redact|Body|Log)'`;
  **Commit:** `fix: redact outbound ERP logs`.)

- [ ] **Task 2: Define strict, canonical plugin resources and protected
  admission.** Write failing tests before implementation. Add plugin type
  constants, canonical defaulting, `PluginAuth`, environment-reference syntax
  and `PLUGIN_` prefix validation, safe auth-header validation, and strict
  JSON/YAML decoding. Reject unknown manifest fields at HTTP and CLI ingress.
  Require an enabled inbound admin credential and exact
  `PLUGIN_ENDPOINT_ALLOWLIST` membership before a credentialed plugin may be
  stored. Test accepted API/Docker values, default persistence as `api`, invalid
  references/headers, raw/unknown auth fields, open-mode rejection, missing or
  mismatched endpoint allowlist, and API `422` behavior.
  (**Seam:** `Plugin.Validate`, plugin API decoding, and CLI document decoding;
  **Files:** `internal/mcp/plugin.go`, `internal/mcp/plugin_api.go`,
  `internal/mcp/plugin_test.go`, `internal/mcp/plugin_api_test.go`,
  `internal/cli/plugin.go`, `internal/cli/plugin_test.go`,
  `internal/cli/plugin_binding_test.go`,
  `docs/plugin-schema.md`, `CHANGELOG.md`;
  **Verify:** `go test ./internal/mcp ./internal/cli -run 'Test(Plugin|PluginAPI|DecodePlugin)'`;
  **Commit:** `feat: define protected plugin authentication resources`.)

- [ ] **Task 3: Send bounded authenticated plugin requests.** Add tests before
  implementation using an HTTPS `httptest` server or capture transport. Resolve
  `spec.auth.credentialRef` after endpoint validation but before request
  creation; a missing value must make zero network calls. Add exactly one
  bearer or API-key header, preserve the existing JSON `PluginInvocation`, size
  limits, timeout, no-retry behavior, disabled redirects, and safe errors.
  Test absent auth, bearer, default/custom API-key headers, missing references,
  secret absence from errors/logs, and rejection-before-I/O.
  (**Seam:** `PluginClient.Process`; **Files:** `internal/mcp/plugin_client.go`,
  `internal/mcp/plugin_client_test.go`, optionally
  `internal/credentials/credentials.go` and
  `internal/credentials/credentials_test.go` for shared environment-reference
  validation/resolution; `docs/plugin-schema.md`, `CHANGELOG.md`;
  **Verify:** `go test ./internal/mcp -run 'TestPluginClient_Process'`;
  **Commit:** `feat: authenticate external plugin calls`.)

- [ ] **Task 4: Enforce outbound transport policy.** Add a dependency-free
  `internal/security` helper that validates an endpoint URL plus whether a
  credential is present; it must not import `mcp` or `connector`. Apply it to
  plugin and ERP requests before I/O. Credentialed HTTP is rejected unless the
  normalized exact host:port occurs in `INSECURE_AUTH_ALLOWED_HOSTS`; an allowed
  exception emits a safe warning. Use a per-request/copy client redirect policy
  to disable redirects for credentialed ERP calls. Test HTTPS acceptance, HTTP
  rejection, exact allowlist acceptance, nonmatching-host rejection,
  unauthenticated HTTP compatibility, and HTTPS-to-HTTP redirect non-following.
  Update the existing connector HTTP-auth test intentionally.
  (**Seam:** endpoint validation before `http.Client.Do`; **Files:**
  `internal/security/transport.go`, `internal/security/transport_test.go`,
  `internal/connector/client.go`, `internal/connector/client_test.go`,
  `internal/mcp/plugin_client.go`, `internal/mcp/plugin_client_test.go`,
  `.env.example`, `docker-compose.yml`, `docker-compose.plugin-test.yml`,
  `docs/environment-variables.md`, `docs/docker.md`, `CHANGELOG.md`;
  **Verify:** `go test ./internal/security ./internal/connector ./internal/mcp -run 'Test(Outbound|Client|PluginClient)'` and
  `MOCK_ERP_CREDENTIALS_JSON='{}' MOCK_PLUGIN_IMAGE=example.invalid/mock:latest ERP_PRIMARY_KEY=test docker compose -f docker-compose.yml -f docker-compose.plugin-test.yml config --quiet`;
  **Commit:** `fix: require secure credential transport`.)

- [ ] **Task 5: Prove persistence, pipeline, and cache invariants.** Add
  persistence round-trip tests proving plugin type/auth references are stored
  but resolved values are absent from raw SQLite JSON. Use the real
  `PluginClient` with a capture transport in MCP and direct-invoke tests to
  prove missing references make no outbound call and obey `continue`/`fail`.
  Extend cache coverage to prove header value A is sent on the first miss, a
  cache hit makes no call, and after an authenticated cache flush/restart-style
  reconfiguration header value B is sent on the next miss. Retain existing
  unbound-tool byte-for-byte tests.
  (**Seam:** Store JSON persistence, `executeTool`, and `CacheMiddleware`;
  **Files:** `internal/mcp/plugin_store_test.go`,
  `internal/mcp/server_plugin_test.go`, `internal/mcp/plugin_api_test.go`,
  `internal/mcp/middleware_test.go`; **Verify:**
  `go test ./internal/mcp -run 'Test(PluginStore|PluginAPI|ServerPlugin|CacheMiddleware)'`;
  **Commit:** `test: cover authenticated plugin runtime behavior`.)

- [ ] **Task 6: Authenticate the real plugin fixture.** Add optional API-key
  enforcement to the separate mock plugin while leaving `/health` unprotected.
  Generate a distinct `MOCK_PLUGIN_API_KEY`, inject it only into mock-plugin and
  ERPBridge, and apply the fixture `Plugin` with `metadata.type: docker`, a
  `PLUGIN_MOCK_API_KEY` reference, and `X-API-Key`. Configure the exact HTTP
  exceptions for both `mock-plugin:8080` and `mock-erp:8081`; do not use a
  global insecure boolean. Add direct black-box checks for missing/wrong key
  `401` responses and correct-key success, then preserve MCP/direct transformed
  and ordinary-tool assertions. Generated values must never be printed.
  (**Seam:** real Compose HTTP boundary; **Files:**
  `../ERPBridge-Plugins/plugins/mock-plugin/main.go`,
  `../ERPBridge-Plugins/plugins/mock-plugin/main_test.go`,
  `docker-compose.plugin-test.yml`, `scripts/test-plugin-integration.sh`,
  `internal/integration/plugin_system_test.go`, `Makefile`, `docs/docker.md`,
  `CHANGELOG.md`; **Verify:**
  `cd ../ERPBridge-Plugins/plugins/mock-plugin && go test ./...`, then
  `make test-plugin-integration` and
  `docker compose -p erpbridge-plugin-test -f docker-compose.yml -f docker-compose.plugin-test.yml ps`;
  **Commit:** `test: authenticate plugin integration fixture`.)

- [ ] **Task 7: Remove legacy plaintext ERP credential persistence.** First
  write failing IDP/CLI tests. Replace raw `AuthKey` and `AuthToken` registry
  fields with `CredentialRef`; make generated tools use each API's reference
  rather than fixed `ERP_PRIMARY_KEY`. Replace `--auth-key` with
  `--credential-ref`, add `api set-credential-ref`, and resolve the reference
  for `api test` through the shared credential helper. Detect legacy fields on
  registry load and block test/register/delete/reference mutation until the
  confirmed scrub completes. Implement atomic destructive scrub without a raw
  backup; test no-op/missing registry, legacy detection, atomic write failure,
  no secret in output, and generator/test failure until a reference is set.
  Regenerate all Cobra API command Markdown after commands stabilize.
  (**Seam:** local IDP registry and CLI API lifecycle;
  **Files:** `internal/idp/registry.go`, `internal/idp/registry_test.go`,
  `internal/idp/generator.go`, `internal/idp/generator_test.go`,
  `internal/cli/api.go`, `internal/cli/api_test.go`,
  `internal/cli/doc.go`, `docs/cli/bridgectl_api.md`,
  `docs/cli/bridgectl_api_register.md`, `docs/cli/bridgectl_api_test.md`,
  `docs/environment-variables.md`, `docs/agent-integrations.md`, `CHANGELOG.md`;
  **Verify:** `go test ./internal/idp ./internal/cli -run 'Test(API|Registry|Generator).*Credential|Test.*Scrub'` and
  `tmp=$(mktemp -d) && go build -o "$tmp/bridgectl" tools/bridgectl/main.go && (cd "$tmp" && ./bridgectl doc) && diff -ru docs/cli "$tmp/docs/cli"`;
  **Commit:** `fix: remove plaintext ERP registry credentials`.)

- [ ] **Task 8: Synchronize public documentation and run final security gates.**
  Before changing the public repository, create its required plan at
  `../erpbridge-docs/.agents/plans/Plan-plugin-credential-security.md` for
  this credential-security update. Then mirror the in-repository schema,
  credential-reference, plugin allowlist, HTTPS exception, Docker, rotation,
  and legacy-migration behavior in its plugin, API, architecture, Docker, and
  environment-variable pages; make a separate public-docs commit. In ERPBridge,
  scan staged changes with the repository secret scanner if configured, and add
  sentinel-value assertions over serialized resources, management responses,
  captured logs, and CLI output. Run focused package tests, targeted lint, the
  full suite, Compose validation, integration cleanup, diff checks, and both
  documentation builds.
  (**Seam:** release acceptance boundary; **Files:**
  `../erpbridge-docs/.agents/plans/Plan-plugin-credential-security.md`,
  `../erpbridge-docs/docs/erpbridge/plugins.mdx`,
  `../erpbridge-docs/docs/erpbridge/api.mdx`,
  `../erpbridge-docs/docs/erpbridge/architecture.mdx`,
  `../erpbridge-docs/docs/erpbridge/docker.mdx`,
  `../erpbridge-docs/docs/erpbridge/environment-variables.mdx`,
  `../erpbridge-docs/CHANGELOG.md`; **Verify:**
  `go test ./internal/logger ./internal/security ./internal/connector ./internal/idp ./internal/mcp ./internal/cli`,
  `golangci-lint run ./internal/logger ./internal/security ./internal/connector ./internal/idp ./internal/mcp ./internal/cli`,
  `make test`, `MOCK_ERP_CREDENTIALS_JSON='{}' MOCK_PLUGIN_IMAGE=example.invalid/mock:latest ERP_PRIMARY_KEY=test docker compose -f docker-compose.yml -f docker-compose.plugin-test.yml config --quiet`,
  `make test-plugin-integration`, `git diff --check`, and
  `cd ../erpbridge-docs && npm run build`.)

## Verification

1. Root, buffered, and MCP logs never include an authorization value, resolved
   credential, or full ERP request/response body.
2. Omitted plugin type is persisted and returned as `api`; only `api` and
   `docker` are accepted, and neither changes deployment ownership.
3. Unknown plugin/auth fields and raw-secret-looking fields are rejected at
   HTTP and CLI ingress. Plugin references must be valid `PLUGIN_*` environment
   variable names.
4. Credentialed plugin configuration requires both inbound admin authentication
   and exact `PLUGIN_ENDPOINT_ALLOWLIST` membership; it cannot be enabled in
   open-auth mode.
5. Bearer and API-key plugins send exactly one expected header; secrets never
   appear in URLs, plugin JSON, API output, errors, logs, SQLite data, or CLI
   output.
6. Credentialed outbound ERP/plugin HTTP is rejected except for exact configured
   development hosts. Credentialed ERP redirects are not followed.
7. Missing plugin credentials perform zero outbound calls and obey `continue`
   or `fail`. Existing unauthenticated plugins and unbound tools retain their
   current behavior.
8. Cache hits make no plugin call; a post-rotation rollout and cache flush uses
   the new credential on the next miss.
9. The integration stack rejects direct missing/wrong plugin keys, accepts the
   correct key, and leaves no project containers or volumes after cleanup.
10. New local API registrations never persist raw credentials. Legacy registry
    state is detected and blocked until explicit scrub; scrub emits no secret,
    preserves non-secret metadata, and generation/testing require a configured
    reference afterward.
11. Package tests, lint, full tests, Compose validation, integration tests,
    `git diff --check`, and both documentation builds are green.

## Open Questions

None. The plan deliberately defers dynamic secret managers and application-level
secret encryption; adding either requires a separate threat model and design.
