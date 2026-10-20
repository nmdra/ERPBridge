# OpenAPI transformation support matrix

## Status

This matrix separates historical reference-prototype behavior from the executable profile required for the next frozen evaluation. `Supported profile` means a construct is eligible only after its deterministic rule and HTTP-level test exist. It does not claim that the current prototype has passed every execution test. Unsupported input is `out_of_scope`, not a generator failure.

| Construct | Historical reference behavior | Supported profile rule | Status |
| --- | --- | --- | --- |
| Root-level `servers` | Uses the first URL when present. | Select first root URL or registered URL, then canonicalize effective target. | Profiled |
| Path Item or Operation-level `servers` | Not selected by generation. | Reject before candidate emission. | Required Stage B |
| Scalar path parameter | Typed argument/location emitted. | Require and percent-encode one supported scalar segment. | Profiled |
| Scalar query parameter | Typed argument/location emitted. | Support only documented default/form scalar semantics. | Profiled |
| Query array/object serialization | Not established. | Support query arrays only after explicit `form`/`explode` rules and tests; reject objects and other styles. | Required Stage B |
| Non-reserved scalar header | Typed argument/location emitted. | Emit after validation; protect routing and credential headers. | Profiled |
| JSON request body | Maps object fields or one complete-body input for primitive/array body. | Support declared properties, nested objects/arrays, and complete body with explicit null/default rules. | Profiled |
| Required/default/enum/nested values | Preserved in generated schemas within observed subset. | Preserve only where direction, omission, and serialization semantics are specified. | Profiled |
| Local schema reference | Resolved. | Allow only acyclic references that remain resolvable after extraction. | Profiled |
| External or recursive reference | Not resolved. | Reject. | Required Stage B |
| Schema composition | `allOf`, `anyOf`, `oneOf`, and related composition are not established. | Reject. | Required Stage B |
| Nullable/readOnly/writeOnly/additional properties | Not established as execution semantics. | Reject until a direction-aware, tested rule exists. | Required Stage B |
| Multiple request/response media types | JSON-oriented behavior only. | Reject ambiguous or non-JSON media selection. | Required Stage B |
| Callbacks | Not generated. | Reject. | Required Stage B |
| Operation-specific security alternatives | Authentication derives from registration. | Reject. | Required Stage B |
| Success response | Considers `200` and `201` JSON responses for output schema. | Select one frozen eligible JSON response deterministically. | Profiled |
| `204`, non-JSON, malformed, ERP error | No complete generated-MCP outcome evidence. | Record distinct normalized error/result outcomes. | Required Stage B |
| Top-level response `data` | Emits a path only when selected schema proves the property. | Emit only for an eligible non-composed selected schema; otherwise reject or omit under the frozen oracle. | Profiled |

The source baseline is `research/project/ToolPlane-Reference-Implementation-Architecture.md`. The algorithm and eligibility rules are in [transformation-profile.md](transformation-profile.md). The historical `GET /integration/echo/last` row uses unsupported `allOf` and nullable semantics and is retained in `pilots/historical-structural-probe/`; it must not enter a future supported-profile denominator.
