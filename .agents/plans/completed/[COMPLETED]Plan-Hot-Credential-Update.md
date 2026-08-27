# Plan: Hot Credential Updates Without Container Restart

**Status:** Completed — implementation and verification passed.

## Goal

Keep environment-variable resolution as the default for downstream
`credentialRef` values while allowing individual ERP tools, API probes,
resources, and plugins to opt into an operator-mounted secret file. File-backed
references must rotate without restarting ERPBridge, and a failed file rotation
must not cause an outbound request with an old, partial, or environment-fallback
credential.

## Current State

- `Security.CredentialRef` stores a reference rather than a secret
  (`internal/mcp/tool.go:93-99`). `prepareERPCall` resolves it before an ERP
  request (`internal/mcp/tool.go:150-214`), and `credentials.Resolve` validates
  the reference and reads `os.LookupEnv` (`internal/credentials/credentials.go:12-33`).
- The same resolver is used by API probes, resources, plugins, and the CLI's
  host-side API test (`internal/mcp/api.go:48-77`, `internal/mcp/resource.go:36`,
  `internal/mcp/plugin_client.go:165`, `internal/cli/api.go:360-365`).
- Missing credentials fail before transport for tools, resources, and plugins
  (`internal/mcp/tool_test.go:188-212`, `internal/mcp/resource_test.go:53-76`,
  `internal/mcp/plugin_client_test.go:430-445`). The connector receives only
  the resolved value and applies auth/transport safety controls
  (`internal/connector/client.go:82-113`, `:153-172`).
- Cache lookup occurs before tool execution (`internal/mcp/middleware.go:168-219`).
  A cached result can therefore be returned without resolving a credential.
- Tool/plugin state is reconciled from SQLite independently of credential
  resolution (`internal/mcp/server.go:371-434`). API registration persists a
  credential reference and generation copies it to generated tools
  (`internal/idp/registry.go:20-33`, `internal/idp/generator.go:374-405`), so
  a source selector must round-trip through registry and generation. A reload
  mechanism must not depend on reconciliation.
- The documented contract is environment-only (`AGENTS.md:55-57`,
  `docs/environment-variables.md:7,27-29`, `docs/architecture.md:165-174`).
  Existing Compose does not mount ERPBridge credential files
  (`docker-compose.yml:43-57`).
- Kubernetes documents that environment-injected Secret values require a
  restart, while normal Secret-volume updates are eventually propagated;
  `subPath` mounts do not update ([Kubernetes](https://kubernetes.io/docs/tasks/inject-data-application/distribute-credentials-secure),
  [Secrets](https://kubernetes.io/docs/concepts/configuration/secret/)). AWS
  EKS, AKS Key Vault, and GKE Secret Manager provide CSI-style mounted-secret
  integrations with optional/configurable rotation; their timing is provider
  and workload configuration, not an ERPBridge guarantee
  ([AWS](https://docs.aws.amazon.com/eks/latest/userguide/manage-secrets.html),
  [Azure](https://learn.microsoft.com/en-us/azure/aks/csi-secrets-store-configuration-options),
  [GKE](https://docs.cloud.google.com/secret-manager/docs/secret-manager-managed-csi-component)).

## Decisions

1. **Add an explicit per-reference source selector.** Add optional
   `credentialSource: env|file` beside `credentialRef` in tool/resource
   security, registered APIs, API-probe requests, and plugin authentication.
   Omission means `env`, preserving every existing manifest and registry entry.
   `file` requires `ERPBRIDGE_CREDENTIALS_DIR` and resolves only
   `${ERPBRIDGE_CREDENTIALS_DIR}/${credentialRef}`. Do not use a global file
   override, an environment fallback for a `file` reference, or source prefixes:
   each makes source selection ambiguous or breaks current reference validation.
2. **Keep `credentialRef` logical and declarative.** It remains a safe reference
   such as `ERP_PRIMARY_KEY`; manifests and SQLite never contain a secret,
   secret URI, or filesystem path. Do not add a runtime credential mutation API.
3. **Resolve on every credential-bearing operation.** Read a `file` reference
   immediately before constructing the outbound request. Do not use a watcher,
   poller, process-wide last-known-good value, or credential cache. An in-flight
   request may finish with the version it already read.
4. **Define observed-file, not provider-global, rotation semantics.** A request
   after ERPBridge observes a complete mounted-file replacement uses that value;
   a request before observation may use the previous value. For writable local
   mounts, staged same-directory rename is recommended. CSI/projected mounts
   are read-only and provider-managed, so their documented eventual refresh is
   accepted; permit their symlink-based generations and do not require a
   producer-side rename protocol. Never use `subPath` for a rotating mount.
5. **Fail closed in file mode.** Missing, unreadable, oversized, empty, or
   invalid-content files fail the affected operation before outbound transport.
   File contents are read exactly; reject control characters rather than silently
   trimming or repairing credentials. Never use an environment or old in-memory
   fallback after a `file`-source failure. `env` retains the existing resolver
   behavior.
6. **Bypass cache based on immutable source metadata.** A tool with
   `credentialSource: file`, or with an active bound plugin using file auth,
   skips cache reads and writes before `cache.Get`. Environment-backed tools
   retain current cache behavior. This prevents a result created with a previous
   credential from being served after rotation.
7. **Expose only bounded outcome telemetry.** Add outcome-only metrics and safe
   failure logs. Allowed labels are fixed source/outcome values such as
   `file/missing` or `file/success`; never include the reference, directory,
   file contents, byte count, hash, authorization header, or credential-derived
   label.
8. **Limit the guarantee to one process and its mounted source.** Multiple
   replicas may observe the replacement at different times, an in-flight request
   may use the previous value, and adding a new cloud secret can require a
   `SecretProviderClass`/workload mapping update and rollout before its filename
   is mounted. Coordinated fleet cutover, lease renewal, direct Vault/cloud SDKs,
   and `API_AUTH_TOKEN` rotation are separate work.

## Scope

### In scope

- Default environment-backed resolution plus explicit file-backed opt-in for
  existing tool, API-probe, resource, plugin, registered-API, generated-tool,
  and host-side local API-test consumers of `credentialRef`.
- Source-selector validation, bounded file reads, observed-replacement and
  fail-closed behavior, and compatibility with provider-managed projected files.
- Cache bypass for file-backed credential-bearing execution paths.
- Safe metrics, logs, tests, AWS/Azure/GCP/Kubernetes deployment guidance,
  rollback guidance, and public documentation synchronization.

### Out of scope

- Runtime admin endpoints or CLI commands that accept or store secret values.
- SQLite, Redis, or cache storage of credential values.
- Automatic authentication to Vault, cloud secret managers, or external control
  planes.
- Hot reload of `API_AUTH_TOKEN` or other server process environment variables.
- A globally synchronized cutover across replicas.
- Changes to `.agents/plans/active/Plan-Live-E2E-Issue-Fixes.md`.

## Tasks

- [x] **Task 1: Define environment-default and file-opt-in resolver contracts
  test-first.** Start with failing tests for omitted/`env` source retaining the
  current lookup, `file` source requiring a directory, file failure while a
  same-named environment variable exists (no fallback), empty references
  remaining unauthenticated, invalid source rejection, safe reference validation,
  bounded single-open file reads, provider-managed symlink target acceptance,
  exact content handling, rejected empty/control-character content, and atomic
  old-or-new local replacement reads. Add shared `CredentialSource` validation
  and a metadata-only `IsFileBacked` query for cache policy without resolving or
  exposing a credential. Use a 64 KiB maximum and return errors that contain no
  credential bytes or full filesystem path. **Seam:** `credentials.Resolve` and
  source-selection boundary. **Files:** `internal/credentials/credentials.go`,
  `internal/credentials/credentials_test.go`. **Verify:**
  `go test ./internal/credentials -run 'Credential|Resolve' -count=1`.

- [x] **Task 2: Persist and propagate source selection through every
  credentialRef consumer.** Add failing registry/generator round-trip tests for
  omitted source, `env`, and `file`; generated file-backed tools must remain
  file-backed. Add rotation tests that replace a mounted file between two
  requests and verify the second outbound request uses the new value for an ERP
  tool, API probe, resource, plugin, and host-side `bridgectl api test --local`
  diagnostic. Extend API registration CLI flags and requests, `idp.API`, both
  generation paths, `Security`, plugin auth, and `APIProbeRequest`. Update
  unresolved-credential diagnostics and API-probe preflight to use the shared
  source contract rather than direct `os.Getenv`; any non-empty authenticated
  reference is credential-bearing for insecure-transport rejection regardless
  of whether current resolution succeeds. Preserve connector auth-header, HTTPS,
  redirect, timeout, and redaction behavior. Ensure a missing or invalid file
  prevents connector or plugin transport. Document that `--local` resolves in
  the CLI process and therefore requires access to the same configured file
  mount; server-side probes resolve in ERPBridge. **Seam:** API registry →
  generator → tool/resource/API-probe/plugin preparation immediately before
  transport. **Files:** `internal/idp/registry.go`, `internal/idp/registry_test.go`,
  `internal/idp/generator.go`, `internal/idp/generator_test.go`,
  `internal/mcp/tool.go`, `internal/mcp/api.go`, `internal/mcp/resource.go`,
  `internal/mcp/plugin.go`, `internal/mcp/plugin_client.go`,
  `internal/mcp/server.go`, `internal/cli/api.go`, `internal/cli/api_test.go`,
  `internal/mcp/tool_test.go`, `internal/mcp/api_test.go`,
  `internal/mcp/resource_test.go`, `internal/mcp/plugin_client_test.go`, and
  `internal/mcp/server_plugin_test.go`. **Verify:**
  `go test ./internal/idp ./internal/mcp ./internal/cli ./internal/connector -run 'Credential|Probe|Resource|Plugin|Transport|Generator|Registry' -count=1`.

- [x] **Task 3: Make cache behavior safe during file-backed rotation.** Write
  failing tests showing that a tool declared `credentialSource: file` neither
  consumes an existing cache entry nor writes a new one; an authenticated active
  plugin binding declared file-backed also bypasses the tool cache; a failed
  file read cannot return a stale cached result; omitted/`env` tools retain
  cache-hit behavior; unrelated tools remain cacheable; and updating the same
  tool name/version from `env` to `file` cannot serve an entry through a
  middleware closure that captured old metadata. Before `cache.Get`, resolve
  the current registered tool metadata once and decide bypass from that actual
  tool and its active plugin bindings, without resolving a credential merely to
  select cache behavior. Keep cache keys free of credential values and file
  metadata. **Seam:** `Server.CacheMiddleware`, current registry lookup, and
  active plugin-binding lookup. **Files:** `internal/mcp/middleware.go`,
  `internal/mcp/registry.go`,
  `internal/mcp/plugin_registry.go`, `internal/mcp/middleware_test.go`,
  `internal/mcp/server_plugin_test.go`, `internal/cache/manager_test.go`.
  **Verify:**
  `go test ./internal/mcp ./internal/cache -run 'Cache.*Credential|Credential.*Cache|Plugin.*Cache' -count=1`.

- [x] **Task 4: Add safe reload telemetry and failure handling.** Add fixed
  source/outcome counters for file-resolution attempts and safe failure logs.
  Make reconciliation diagnostics source-aware without opening files merely to
  warn, and remove credential-reference labels from diagnostics. Test that
  metrics, logs, errors, control-plane responses, and plugin failure paths
  contain no credential value, file contents, authorization header, directory
  path, hash, or reference. Test invalid replacement files for direct tools and
  plugins, preserving existing `continue` and `fail` plugin policies. **Seam:**
  resolver outcome reporting through metrics/logger sinks and server
  reconciliation diagnostics. **Files:** `internal/metrics/metrics.go`,
  `internal/metrics/metrics_test.go`, `internal/credentials/credentials.go`,
  `internal/credentials/credentials_test.go`, `internal/mcp/server.go`,
  `internal/mcp/server_test.go`, `internal/mcp/plugin_client_test.go`,
  `internal/mcp/server_plugin_test.go`. **Verify:**
  `go test ./internal/credentials ./internal/metrics ./internal/mcp -run 'Credential|Metric|Redact|Plugin' -count=1`.

- [x] **Task 5: Add process-level rotation integration coverage.** Create a
  deterministic test with an ERP `httptest.Server`, a real ERPBridge server,
  one active file-backed tool, and a temporary credential directory. Assert the
  first call uses version A, atomically replace the local file, and assert the
  next call uses version B while the ERPBridge process remains the same. Add a
  projected-generation symlink fixture to prove complete target reads; do not
  claim to simulate a cloud provider's refresh schedule. Assert missing,
  oversized, empty, or control-character content fails before the upstream
  receives a request, while an environment-backed tool still succeeds in the
  same process. Record only statuses, request counts, and process identity;
  never retain credentials or raw bodies. **Seam:** real server HTTP/MCP
  invocation through the shared credential resolver and connector. **Files:**
  `internal/integration/credential_rotation_test.go`,
  `internal/mcp/integration_test.go`, `internal/connector/client_test.go`.
  **Verify:**
  `go test ./internal/integration ./internal/mcp ./internal/connector -run 'CredentialRotation|HotCredential' -count=1`.

- [x] **Task 6: Document environment-default and cloud-mounted deployment.**
  Update developer documentation with optional `credentialSource: file`, its
  default `env` behavior, `ERPBRIDGE_CREDENTIALS_DIR`, a read-only
  mounted-directory example, the stable logical `credentialRef` contract, the
  64 KiB/control-character/empty-file rules, fail-closed file behavior, cache
  bypass, local staged-rename rollback, and per-replica observed-file timing.
  Give provider-specific deployment links and concise guidance for AWS EKS ASCP,
  AKS Key Vault CSI, and GKE Secret Manager CSI: use workload identity and
  least-privilege access, mount a directory rather than `subPath`, enable and
  configure provider rotation, and expect eventual refresh. State that adding a
  new remote secret can require changing the provider mount mapping and rolling
  the workload before its filename exists; existing environment-backed tools do
  not change and environment-variable changes still require recreation.
  **Seam:** operator deployment contract and credential-reference schema.
  **Files:** `.env.example`, `docs/environment-variables.md`, `docs/docker.md`,
  `docs/caching.md`, `docs/api.md`, `docs/tool-schema.md`,
  `docs/plugin-schema.md`, `docs/onboarding.md`, `CHANGELOG.md`. **Verify:**
  `grep -RIn 'credentialSource\|ERPBRIDGE_CREDENTIALS_DIR\|subPath\|AWS\|Azure\|GKE\|cache bypass\|multi-replica' docs .env.example CHANGELOG.md`.

- [x] **Task 7: Synchronize public documentation.** Mirror the final
  environment-default/file-opt-in contract, source-selector schema, AWS/Azure/
  GKE deployment links and limitations, cache implications, plugin/tool
  behavior, local versus provider-managed rotation behavior, and replica limits
  in the matching `../erpbridge-docs` pages. Commit the public documentation
  separately as required by the repository instructions. Do not modify the
  active live-E2E plan or `docs/cli/bridgectl_api_scrub-credentials.md`.
  **Seam:** in-repo developer documentation to Docusaurus user documentation.
  **Files:** `../erpbridge-docs/docs/erpbridge/environment-variables.mdx`,
  `../erpbridge-docs/docs/erpbridge/docker.mdx`,
  `../erpbridge-docs/docs/erpbridge/caching.mdx`,
  `../erpbridge-docs/docs/erpbridge/api.mdx`,
  `../erpbridge-docs/docs/erpbridge/tool-schema.mdx`,
  `../erpbridge-docs/docs/erpbridge/plugins.mdx`,
  `../erpbridge-docs/docs/erpbridge/onboarding.mdx`, and
  `../erpbridge-docs/CHANGELOG.md`. **Verify:**
  `npm run build --prefix ../erpbridge-docs`.

## Verification

1. Omitted `credentialSource` and explicit `env` preserve every current
   environment-backed deployment and cache behavior, even when another tool in
   the same process uses file mode.
2. Explicit `file` requires a configured directory and never falls back to an
   environment value. A complete file version observed by ERPBridge changes the
   credential used by the next tool, probe, resource, or plugin request without
   restarting the ERPBridge process or container.
3. Each request sees one complete locally replaced or provider-projected file
   version; partial, empty, invalid, missing, or oversized files fail before
   outbound transport, with no environment or last-known-good fallback.
4. File-source tool and authenticated file-source plugin execution cannot
   consume or write cache entries; environment-backed and unrelated tools retain
   current cache behavior.
5. Registry/generator/API-probe/CLI round trips preserve source selection;
   server-side probes resolve on ERPBridge, while `bridgectl --local` resolves
   only when the CLI process has the configured file mount.
6. Metrics, logs, errors, manifests, SQLite records, and cache keys contain no
   credential value, file content, directory path, hash, authorization header,
   or credential-reference label.
7. The process-level integration test proves the server identity is unchanged
   across a successful locally observed rotation and that failed file reads do
   not reach the ERP or plugin. It does not claim cloud-provider refresh timing.
8. Docs state normal-volume/no-`subPath` requirements, eventual provider
   refresh, workload-identity least privilege, and the possible rollout needed
   to add a new provider-mounted secret name.
9. Focused tests, scoped lint on changed Go packages, and `make test` pass.
10. Public documentation builds successfully, and the existing active plan and
    protected scrub document remain unchanged.

## Open Questions

- Does production require a coordinated fleet cutover guarantee or dynamic
  addition of arbitrary cloud-secret names with no workload mapping rollout? If
  yes, this CSI/file design is insufficient; plan a separate readiness/drain
  protocol or direct shared secret-provider integration before implementation.
