# External Plugin Resource Schema

ERPBridge can call a plugin that runs in a separate process or container. ERPBridge stores the plugin endpoint and does not start, install, or update plugin code.

## Plugin

A `Plugin` identifies one exact plugin release and an HTTP endpoint:

```yaml
apiVersion: erpbridge.io/v1
kind: Plugin
metadata:
  name: response-transformer
  version: 1.0.0
  isActive: true
spec:
  endpoint: http://plugin-host:9000
  timeoutMilliseconds: 5000
```

`spec.endpoint` must be an absolute `http` or `https` URL. It must not contain
userinfo, query parameters, or a fragment. ERPBridge sends requests to
`/v1/process` below that endpoint. The timeout must be between 1 millisecond
and 5 minutes.

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

The only supported phase is `after_response`. Bindings run in ascending
priority order after a successful tool result has passed its output schema.
The default failure policy is `continue`; `fail` returns a generic tool error.

## Plugin HTTP contract

ERPBridge sends a synchronous JSON `POST /v1/process` request. The request
contains only the protocol version, an invocation ID, the exact tool identity,
the normalized result, and binding configuration:

```json
{
  "protocolVersion": "v1",
  "invocationId": "generated-id",
  "tool": {"name": "list-orders", "version": "1.0.0"},
  "result": {"id": "order-1"},
  "config": {"mode": "safe"}
}
```

The plugin must return a JSON object with a `result` member:

```json
{"result":{"id":"order-1","processed":true}}
```

ERPBridge does not send original tool arguments, inbound headers, caller
identity, caller tokens, or ERP credentials. Request and response JSON are
limited to 1 MiB. Plugin calls use the invocation context and resource timeout,
disable redirects, and do not retry.
