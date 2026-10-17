# Diagnose and report ERPBridge issues

| Task | Section |
| --- | --- |
| Select evidence and reproduce | [Triage loop](#triage-loop) |
| Plugin or binding failure | [Plugin diagnosis](#plugin-diagnosis) |
| Interpret a stable error | [Stable-code evidence](#stable-code-evidence) |
| Prepare a redacted report | [Bug-report workflow](#bug-report-workflow) |

## Triage loop

1. Run [core preflight and only the relevant profiles](preflight.md) for the
   affected context. Remote diagnostics do not require local Docker, localhost
   health, MockERP, RedisInsight, or an API probe. Use the selected `CTX` in
   every CLI deployment command as `--context "$CTX"`; management MCP uses
   its pinned context and [result rules](mcp.md).
2. Capture `bridgectl version`, safe context names, server/SDK version when
   available, timestamp with timezone, and the smallest affected API or tool.
   Do not source `.env`; use Compose's `--env-file` and inspect only booleans,
   names, status codes, and timings.
3. Classify the failure: context/authentication, role authorization, API
   connectivity, credential source or rotation, schema/registry, MCP
   client/SDK protocol handling, cache, or server runtime. Route stable
   onboarding/API codes through the numbered
   [recovery branches](onboarding.md#recovery-branches).
4. Inspect the narrowest evidence first: exact resource readback, bounded log
   stats/tail, or cache stats. Check deployment health only when relevant.
   A server-side API test can cause downstream effects. Obtain exact approval
   before testing the named API; management MCP also requires the `probe`
   grant. Do not use a probe as a universal read-only health check. Use
   repository history and `git status` without altering unrelated work.
5. Reproduce with a minimum safe request. Do not use production personal data,
   raw tokens, authorization headers, ERP credentials, upstream bodies, or
   plugin payloads in reproduction artifacts.
6. State the observed result, expected result, stable code, and evidence that
   separates them. Every mutation or effectful test needs exact per-action
   confirmation, not only production actions. Follow the shared
   [verification gates](preflight.md#action-and-verification-gates), independently
   read back changes, and invalidate affected evidence after a new failure or
   change. For an unknown write outcome, read back before considering a retry.

For the local Compose deployment only, a changed quoted environment value
requires the explicit Compose
`--force-recreate` path; recreating a container does not prove health. For a
file-backed resource, inspect only the presence, permissions, regular-file
status, and bounded size of the reference-named file under
`ERPBRIDGE_CREDENTIALS_DIR`; never print its content or use an environment
fallback. The normal `bridgectl api test <name> --context "$CTX"` is
server-side and body-free, but is still effectful. Use `--local` only for a
separately approved and explicitly labelled host-side diagnostic.

## Plugin diagnosis

For plugin or binding failures, read [Plugins](plugins.md) before changing
state. Classify the failure as strict manifest validation, protected admission
or endpoint allowlist, exact reference resolution, missing or invalid
credential source, timeout, non-2xx response, malformed or oversized response,
transformed-output schema failure, binding failure policy, or stale cache.
Start with plugin and binding readback, bounded log stats/tail, and cache stats.
Remember that file-backed authenticated bindings bypass response caching, while
MCP annotations and `_meta` affect discovery only and never grant access.
Reproduce with safe deterministic data and preserve the distinction between an
original result and a transformed result.

Keep plugin evidence redacted: omit authorization headers, credential values,
sensitive endpoints, request/response payloads, plugin response bodies, ERP
records, and opaque invocation tokens. For repository diagnosis, use the
existing plugin unit tests and `go test -tags pluginintegration
./internal/integration -run TestPluginSystemBlackBox -count=1` when its fixture
stack is available.

## Stable-code evidence

Keep the machine-readable `error` value and numeric exit code together. Common
recovery keys are `CONTEXT_NOT_FOUND`, `LEGACY_REGISTRY`,
`REGISTRY_CONFLICT`, `CONTROL_PLANE_URL_INVALID`, `VALIDATION_FAILED`,
`AUTHENTICATION_FAILED`, `AUTHORIZATION_DENIED`, `UPSTREAM_UNREACHABLE`,
`INSECURE_TRANSPORT`, `HEALTH_CHECK_FAILED`, `RECONCILIATION_FAILED`,
`RESOURCE_NOT_FOUND`, `METHOD_NOT_ALLOWED`, and `API_PROBE_FAILED`. Record the
code and safe suggestion, then follow the matching onboarding branch. Never
replace a stable code with an HTML response, stack trace, or upstream body.

## Bug-report workflow

Create `.agents/reports/<YYYY-MM-DD>-<slug>.md` from
`assets/bug-report.md` at the skill root. Sanitize it before writing:
replace credentials, authorization headers, cookies, personal data, and opaque
tokens with descriptive placeholders. Do not stage or commit a report draft
unless the user requests it.

Offer GitHub issue creation only after the user approves the final title and
body. The issue must link to the affected component/version and include the
minimal reproduction, expected/actual behavior, redacted evidence, and impact.
