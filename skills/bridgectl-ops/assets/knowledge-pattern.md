---
schema_version: 1
id: <AREA>_<ERROR_CODE>-<short-root-cause>
revision: 1
status: active
area: <api|tools|plugins|auth|cache|runtime|diagnostics>
resource_kinds:
  - <resource kind>
operations:
  - <operation>
error_codes:
  - <OFFICIAL_ERROR_CODE>
resolution_codes:
  - <LEARNED_ROOT_CAUSE_CODE>
keywords:
  - <stable keyword>
valid_for:
  bridgectl: unknown
  erpbridge_server: unknown
skill_version: <version-or-unknown>
confidence: low
occurrences: 0
first_seen: <YYYY-MM-DD>
last_seen: <YYYY-MM-DD>
---

# <Short title>

## Trigger

<Describe when this pattern is relevant.>

## Symptoms

<Describe safe observable symptoms only.>

## Learned cause

<State the supported cause and uncertainty.>

## Preferred action

<Describe the smallest safe diagnostic or recovery path. Current skill gates
and confirmation requirements still apply.>

## Avoid

<List misleading, ineffective, obsolete, or unsafe actions.>

## Verification

<Describe the observable confirmation.>

## Evidence

- <execution-id>

## Contradictions

- None recorded.

## Notes

<Optional bounded context. Do not add credentials, payloads, ERP records,
personal data, unrestricted logs, or private reasoning.>
