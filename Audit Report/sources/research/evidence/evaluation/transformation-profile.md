# Executable OpenAPI transformation profile

## Status

This is the frozen profile for the author-authored ERPNext conformance oracle. It is stricter than the historical reference-prototype behavior. A rule is included only when it has a deterministic candidate-generation interpretation and a required HTTP serialization or response-handling test. Until Stage B implements and tests it, this document is a conformance requirement, not a prototype capability claim.

The historical eight-row probe remains under `pilots/historical-structural-probe/`. Its results are not silently reclassified or used as a post-audit supported-subset denominator.

## Inputs and output

A typed transformation consumes a registration `R` and one OpenAPI operation `O` and produces either a candidate resource or a rejection:

\[
T(R,O) \rightarrow \operatorname{Candidate}(S_{MCP}, E_{ERP}) \;\mid\; \operatorname{Reject}(code, location).
\]

`R` supplies the registered API identity, credential reference, policy references, and approved-origin binding. `O` supplies the method, path, selected server, parameters, request body, and response declarations. A simple registration without OpenAPI is eligible only if it supplies the same typed request, response, serialization, and execution fields explicitly; otherwise it is rejected as `registration_incomplete` rather than inferred from prose.

## Ordered algorithm

1. Validate `R`, including credential-reference syntax, policy references, and approved origin.
2. Resolve one OpenAPI path and operation. Reject a Path Item or Operation-level `servers` override as `unsupported_server_override`.
3. Validate that all media, security, reference, schema, and serialization constructs are in this profile.
4. Select the root-level first `servers` URL when present, otherwise the registered URL. Canonicalize the effective target and reject it if an allowed `ERP_BASE_URL` relocation escapes the admitted origin binding.
5. Normalize the tool name from `operationId`, or deterministically derive it from method and path. Reject a normalized-name collision as `name_collision`.
6. Lower every eligible parameter to a typed MCP input and preserve source name, location, requiredness, default, and supported serialization metadata. Reject duplicate or reserved generated fields.
7. Lower an eligible JSON body to typed object properties or one complete-body argument. Reject an unsupported body schema.
8. Select one supported JSON success response deterministically, derive an output schema only when the selected response schema is eligible, and derive an extraction path only when that schema proves the property. Distinguish `output_schema_omitted` from rejection.
9. Bind method, effective target, parameter locations, response rules, credential reference, and policy references into a canonical candidate. Emit the canonical digest for operator admission.

The generator must sort paths and methods before these steps. It must not silently fall back to another server, response, media type, security alternative, or serialization rule.

## Eligibility and serialization rules

| Input feature | Eligible rule | Rejection or required outcome |
| --- | --- | --- |
| Path scalar | `string`, `integer`, `number`, or `boolean`; value is required and percent-encoded as one path segment. | Reject arrays, objects, null, or omitted required value. |
| Query scalar | `string`, `integer`, `number`, or `boolean`; use documented form/default scalar serialization. | Reject unsupported style, null serialization ambiguity, or object query value. |
| Query array | `style: form` only, with explicit `explode: true` or `explode: false` rule and byte-level test. | Reject unspecified or unsupported style/explode combination until implemented. |
| Header scalar | Non-reserved scalar only; emitted after model arguments are validated. | Reject credential, routing, host, and other protected header collisions. |
| JSON object body | Declared properties with supported primitive, object, and array values; preserve requiredness and defaults. | Reject unsupported composition, recursion, and unbounded additional-property semantics. |
| Complete JSON body | Primitive, array, or free-form JSON body through one explicit complete-body input. | Reject a mix of complete-body and field-level mapping. |
| Local reference | Acyclic, local reference that remains resolvable after extraction. | Reject external or recursive reference. |
| Schema direction/null | Explicit required, optional, default, omitted, and null behavior; direction-aware `readOnly`/`writeOnly` rule required. | Reject until the corresponding rule is implemented and tested. |
| Security | Use the registered credential reference only. | Reject operation-specific security alternatives. |

MCP inputs must retain an object root. `null` is distinct from omitted: an omitted optional value is not serialized, while a supplied null value is rejected unless a frozen per-location null rule exists. A default is applied only when the MCP contract states that it applies; it is not guessed from an ERP-side default.

## Response and error rules

The profile considers JSON `200` and `201` success responses, plus the explicitly frozen `202` JSON deletion response for `DELETE /resource/Item/{name}`, only. If both are present, the response selected by the frozen oracle is part of the canonical candidate. A `204` response, non-JSON response, ambiguous success response, malformed JSON response, extraction-path failure, ERP error response, and output-schema validation failure each require a distinct recorded result code. An output schema is omitted only when the selected eligible success response has no derivable schema; a response mapping is a failure when an eligible oracle expects one but the generated candidate does not match it.

The normalized result is validated against the candidate output schema before structured result release. Response processors, when enabled, run only in the revision-bound order specified by the conformance contract; their output is revalidated. Error results do not masquerade as successful structured output.

## Historical eligibility audit

The historical `GET /integration/echo/last` row expected an object at `data`, but the selected response defines `data` using `allOf` and `nullable`. Both constructs are outside the declared historical subset. The historical generator emitted `responsePath: data` without an output schema, producing `response_mapping_correct=False`. The row is retained as an **eligibility-oracle inconsistency** and response-mapping limitation. It is neither a supported-pass nor a corrected rerun.

The frozen five-operation ERPNext oracle at `oracles/real-sanitized-transformations.json` was authored before new generator output is inspected. It records eligibility, covered feature rules, expected serialization, response, deterministic state, cleanup, author, absence of an independent reviewer, freeze time, and contract hash. Unsupported rows remain visible but do not enter the supported-fidelity denominator.
