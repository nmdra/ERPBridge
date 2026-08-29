# Operate external plugins and bindings

Read this guide when a task mentions an ERPBridge external plugin, plugin
endpoint, plugin authentication, a `PluginBinding`, post-response processing,
or plugin-related cache and runtime failures.

## Boundary and prerequisites

ERPBridge stores plugin definitions and invokes an already-running HTTP
endpoint. It does not install, start, upgrade, or manage plugin code, images,
processes, or containers.

Before a plugin change:

1. Record `bridgectl version`, `bridgectl context list`, and the selected
   `--context`.
2. Confirm that the caller has the required authenticated admin access. A
   `raw_response` binding always needs an authenticated admin control-plane
   request, even when the plugin has no `spec.auth`.
3. Confirm the separately operated plugin endpoint, exact plugin version, exact
   target tool version, timeout, failure policy, and expected cache impact.
4. Keep credentials in environment variables or a mounted credential directory.
   A manifest contains a `PLUGIN_*` logical `credentialRef` and optional
   `credentialSource: file`, never a credential value. File mode requires
   `ERPBRIDGE_CREDENTIALS_DIR` and a reference-named file.

The complete resource and wire contract is in
[the plugin schema](../../../docs/plugin-schema.md). Use the field guides
[`../assets/plugin.yaml`](../assets/plugin.yaml) and
[`../assets/plugin-binding.yaml`](../assets/plugin-binding.yaml) as starting
points, not as substitutes for validation.

## Plugin resource

A plugin identifies one exact release and an endpoint:

- `apiVersion` is `erpbridge.io/v1` and `kind` is `Plugin`.
- `metadata.name` and `metadata.version` identify the plugin. The version must
  be valid SemVer. `metadata.type` is `api` or `docker`; omitted type is stored
  as `api`. The type describes deployment ownership; ERPBridge does not deploy
  it.
- `spec.endpoint` is an absolute `http` or `https` URL without userinfo, query
  parameters, or a fragment. ERPBridge posts to `/v1/process` below that
  endpoint.
- `spec.timeoutMilliseconds` is bounded from 1 millisecond through 5 minutes.
- `spec.auth` is optional. It supports `bearer` or `api-key`. API-key auth uses
  the declared `header`, or `X-API-Key` when omitted. Bearer auth uses
  `Authorization: Bearer <value>` and cannot declare a custom header.
- `spec.auth.credentialRef` must name a `PLUGIN_*` logical reference. The
  optional `spec.auth.credentialSource` is `env` by default or `file` when
  `ERPBRIDGE_CREDENTIALS_DIR` supplies the reference-named file. File content
  is resolved immediately before each invocation, never falls back to the
  environment, and fails closed when missing, empty, invalid, non-regular, or
  larger than 64 KiB. The resolved value is not persisted or placed in the
  plugin JSON payload. Authenticated file-backed plugin bindings bypass the
  response cache.

Manifests are strict. Unknown JSON/YAML fields, including guessed token or
password fields, must be removed rather than ignored. Validate before any
server request.

## PluginBinding resource

A binding connects one exact plugin version to one exact tool version:

- `apiVersion` is `erpbridge.io/v1` and `kind` is `PluginBinding`.
- `metadata.name` is the binding identity.
- `spec.pluginRef.name` and `.version` must identify an active plugin release.
- `spec.toolRef.name` and `.version` must identify an active tool release.
- `spec.phase` is `raw_response` or `after_response`. Raw processing runs
  before response normalization. After-response processing runs after a
  successful normalized result passes its output schema.
- A `raw_response` binding requires an active HTTP-backed tool with an explicit
  object-shaped final `outputSchema`. Its plugin endpoint must be present in
  `PLUGIN_ENDPOINT_ALLOWLIST`, even without plugin authentication. Missing
  prerequisites keep the binding inactive during reconciliation.
- `spec.priority` is non-negative. Active bindings run in ascending priority;
  the binding name breaks equal-priority ties.
- `spec.failurePolicy` is `continue` or `fail`; the default is `continue`.
  `spec.config` must be JSON-compatible.

A binding is accepted only when both exact referenced resources are active.
Use exact versions when preparing upgrades or rollback plans; do not rely on a
latest-version guess.

## Apply and verify

Use the CLI command references for the installed release:

- [plugin apply](../../../docs/cli/bridgectl_plugin_apply.md),
  [get](../../../docs/cli/bridgectl_plugin_get.md),
  [validate](../../../docs/cli/bridgectl_plugin_validate.md), and
  [delete](../../../docs/cli/bridgectl_plugin_delete.md)
- [binding apply](../../../docs/cli/bridgectl_plugin_binding_apply.md),
  [get](../../../docs/cli/bridgectl_plugin_binding_get.md),
  [validate](../../../docs/cli/bridgectl_plugin_binding_validate.md), and
  [delete](../../../docs/cli/bridgectl_plugin_binding_delete.md)

Use this sequence:

1. `bridgectl plugin validate -f <plugin-file>`.
2. `bridgectl plugin binding validate -f <binding-file>`.
3. Obtain the exact change confirmation required by `SKILL.md`.
4. Apply the plugin first, then the binding:
   `bridgectl plugin apply -f <plugin-file>` and
   `bridgectl plugin binding apply -f <binding-file>`.
5. Read back the exact resources with
   `bridgectl plugin get <name>@<version>` and
   `bridgectl plugin binding get <name>`.
6. Verify a safe test tool call through MCP and, where appropriate, the direct
   invocation seam. Confirm the transformed result, output-schema behavior,
   and absence of secrets in output and logs.

Apply accepts JSON, YAML sequences, multi-document YAML, and directories of
resource files. Validate the exact file before applying a batch, and inspect
which resources the directory contains.

## Authentication and transport

Credentialed plugin admission requires all of the following:

- `API_AUTH_TOKEN` is configured on the ERPBridge server.
- The control-plane request is authenticated as an admin.
- The normalized plugin `host:port` is an exact member of the comma-separated
  `PLUGIN_ENDPOINT_ALLOWLIST`. This allowlist is also mandatory for every
  `raw_response` binding when plugin authentication is absent.

At invocation, a configured credential is resolved from its `PLUGIN_*` reference
and sent as exactly one authentication header. A missing or empty reference
fails closed and makes no plugin request. Credentialed outbound HTTP is for
private development exceptions only; production plugin endpoints use HTTPS and
must not depend on `INSECURE_AUTH_ALLOWED_HOSTS`.

Do not put credentials in endpoint URLs, query strings, configuration values,
logs, reports, or test fixtures. Redact authorization headers, sensitive
endpoints, payloads, plugin response bodies, personal data, and opaque tokens
before sharing evidence.

## Runtime contract

The v1 plugin exchange is a synchronous JSON `POST /v1/process`.
An `after_response` request contains `protocolVersion`, an invocation ID, exact
tool identity, the normalized result, and binding configuration. A
`raw_response` request instead contains a bounded `rawResponse` with status,
normalized content type, and a tagged body:

```json
{
  "status": 200,
  "contentType": "image/png",
  "body": {"encoding": "base64", "value": "..."}
}
```

The `encoding` is `json` for one complete decoded JSON document, or `base64`
for binary, empty, malformed, or non-JSON bodies. Raw invocations omit
`result`; legacy after-response invocations retain `result: null` when needed.
The response contains a JSON `result` value that replaces only the body. It
cannot change the upstream status.

- The request contains `protocolVersion`, an invocation ID, exact tool identity,
  the normalized result for `after_response`, or `rawResponse` for
  `raw_response`, and binding configuration.
- It does not contain original tool arguments, inbound headers, caller
  identity, caller tokens, or ERP credentials.
- The response must be a JSON object with a `result` member.
- Each request and response JSON document is limited to 1 MiB.
- Calls use the invocation context and resource timeout, disable redirects, and
  do not retry.

Raw bindings run before `responsePath`, output-schema validation, and
`after_response`; each phase uses priority order independently. Only successful
2xx responses run success-only normalization and after-response processing. A
raw-bound terminal non-2xx response retains error state, even when a raw plugin
returns a replacement JSON value. A failed raw chain never exposes an
unfiltered ERP body: `continue` uses the original captured response only when
it can satisfy the final schema, otherwise it returns a safe error. Use
`failurePolicy: fail` for image conversion unless a compatible fallback is
known. All bindings run only on a cache miss. The final transformed result is
validated against the developer-owned tool output schema again. Plugins never
change MCP schemas dynamically; publish a new MCP-visible tool name and exact
tool version when output meaning or type changes. With `fail`, the caller
receives the generic plugin-processing error rather than the endpoint, payload,
or plugin response body.

Applying, updating, or deleting a plugin or binding flushes cache entries for
affected tools. For environment-backed credential rotation, refresh the
provider process and perform the narrow authenticated cache flush required to
force a new miss. File-backed authenticated plugin bindings bypass the cache;
replace a local file atomically or wait for the provider-managed mount to
refresh, with no process restart or cache flush needed merely to observe the
new value. Mount the directory rather than a Kubernetes `subPath`; projected
secret updates are eventual and replicas can observe different generations
briefly.

## Deletion and diagnosis

A normal delete is a soft delete: it marks the resource inactive and retains
its record. A hard delete permanently removes it and requires explicit
confirmation; use `--hard --yes` only after repeating the exact target and
impact. An active binding blocks hard deletion of its exact plugin version.
Delete or deactivate the binding first when removal is intentional.

Classify failures before changing state:

- validation or strict-decoding failure: inspect the manifest fields and exact
  SemVer references;
- admission failure: check inbound admin authentication,
  `PLUGIN_ENDPOINT_ALLOWLIST`, and server environment configuration;
- binding reference failure: read back exact active plugin and tool versions;
- credential failure: check only whether the named environment reference or
  mounted file source is configured and readable, never its value; file-backed
  resources fail closed and do not fall back to the environment;
- timeout, non-2xx, malformed, missing-result, or oversized response: inspect
  bounded status and timing evidence without copying plugin bodies;
- transformed-output failure: check the tool output schema and binding order;
- stale result: inspect cache stats and the affected tool's narrow cache entry.

Start with `bridgectl plugin get`, `bridgectl plugin binding get`, bounded log
stats/tail, and cache stats. Reproduce with safe deterministic data. For
repository diagnosis, use the existing plugin unit tests and the
`pluginintegration` black-box test rather than sending production records.
