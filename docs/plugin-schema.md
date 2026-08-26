# External Plugin Resource Schema

This page defines the control-plane and wire contract for a plugin that runs in a separate process or container. ERPBridge stores the plugin endpoint and does not start, install, or update plugin code.

## Plugin

A `Plugin` identifies one exact plugin release and an HTTP endpoint:

```yaml
apiVersion: erpbridge.io/v1
kind: Plugin
metadata:
  name: response-transformer
  version: 1.0.0
  type: api
  isActive: true
spec:
  endpoint: https://plugin-host:9000
  timeoutMilliseconds: 5000
  auth:
    type: api-key
    credentialRef: PLUGIN_RESPONSE_TRANSFORMER_KEY
    header: X-API-Key
```

`metadata.type` is `api` or `docker`. If it is omitted, ERPBridge stores `api`.
The type describes the plugin deployment. ERPBridge does not start, install, or
update Docker images.

`spec.endpoint` must be an absolute `http` or `https` URL. It must not contain
userinfo, query parameters, or a fragment. ERPBridge sends requests to
`/v1/process` below that endpoint. The timeout must be between 1 millisecond
and 5 minutes.

`spec.auth` is optional. It supports `bearer` and `api-key` authentication.
`api-key` can use `header`, or the default `X-API-Key` header. `credentialRef`
must name a `PLUGIN_` environment variable. Manifests never contain the
credential value. Bearer authentication cannot set `header`. API-key
authentication cannot use reserved HTTP headers. At invocation, ERPBridge
resolves the reference and sends exactly one header: `Authorization: Bearer
<value>` for bearer auth, or the raw value in the API-key header. If the
reference is missing or empty, ERPBridge makes no plugin request and reports a
safe configuration error without exposing the credential.

Plugin JSON and YAML manifests reject unknown fields. This prevents a misspelled
or raw credential field from changing the authentication behavior.

A credentialed plugin requires `API_AUTH_TOKEN` and an authenticated admin
request. Set `PLUGIN_ENDPOINT_ALLOWLIST` to comma-separated exact `host:port`
values. The plugin endpoint must match one value before ERPBridge stores it.

## PluginBinding

A `PluginBinding` connects exact plugin and tool versions:

```yaml
apiVersion: erpbridge.io/v1
kind: PluginBinding
metadata:
  name: transform-orders
  isActive: true
spec:
  pluginRef:
    name: response-transformer
    version: 1.0.0
  toolRef:
    name: list-orders
    version: 1.0.0
  phase: after_response
  priority: 10
  failurePolicy: continue
  config:
    mode: safe
```

The control plane accepts `raw_response` and `after_response` phases. Raw
bindings run before response normalization; after-response bindings run after a
successful tool result has passed its output schema. Active bindings use
ascending priority order within each phase. The default failure policy is
`continue`; `fail` returns a generic tool error.
Plugin resources are versioned declarative records. Bindings are named
declarative records that reference exact plugin and tool versions. A soft delete
sets `isActive: false` and retains the record. A plugin cannot be hard-deleted
while an active binding references that exact plugin version. Inactive
bindings remain retained until they are explicitly hard-deleted.

## Plugin HTTP contract

The v1 plugin protocol defines a synchronous JSON `POST /v1/process` exchange.
When the response pipeline processes an active `after_response` binding, the
request contains only the protocol version, an invocation ID, the exact tool
identity, the normalized result, and binding configuration:

```json
{
  "protocolVersion": "v1",
  "invocationId": "generated-id",
  "tool": {"name": "list-orders", "version": "1.0.0"},
  "result": {"id": "order-1"},
  "config": {"mode": "safe"}
}
```

A `raw_response` binding instead receives the bounded ERP response before
normalization. Its payload contains status, normalized content type, and a
body tagged as either a decoded JSON value or base64 text:

```json
{
  "protocolVersion": "v1",
  "invocationId": "generated-id",
  "tool": {"name": "read-invoice-text", "version": "1.0.0"},
  "rawResponse": {
    "status": 200,
    "contentType": "image/png",
    "body": {"encoding": "base64", "value": "..."}
  },
  "config": {"mode": "ocr"}
}
```

Raw invocations omit `result`. Legacy after-response invocations retain an
explicit `result: null` when their result is nil. A plugin response always
contains a JSON object with a `result` member; that member replaces only the
response body and cannot change HTTP status.

The plugin must return a JSON object with a `result` member:

```json
{"result":{"id":"order-1","processed":true}}
```

The protocol does not include original tool arguments, inbound headers, caller
identity, caller tokens, or ERP credentials. Authentication headers are
transport metadata and are not included in the JSON payload. Request and
response JSON are limited to 1 MiB. Calls use the invocation context and
resource timeout, disable redirects, and do not retry.

Raw bindings can inspect terminal HTTP responses, but successful response
normalization and after-response bindings apply only to 2xx responses. Raw and
after-response bindings run only on a cache miss. The cache stores the final
transformed MCP result. Applying, updating, or deleting
a plugin or binding flushes the affected tool cache entries.

## CLI management

Use `bridgectl plugin apply|get|delete|validate` for plugin resources. Plugin
identities use `name@version`. Use `bridgectl plugin binding
apply|get|delete|validate` for named bindings. Apply accepts JSON, YAML
sequences, multi-document YAML, and directories of resource files. Validate
accepts a JSON or YAML resource file. Use `--hard --yes` to skip the
hard-delete confirmation.
