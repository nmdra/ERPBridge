# Diagnose and report ERPBridge issues

## Triage loop

1. Run the read-only [onboarding preflight](onboarding.md#preflight) with the
   affected `--context`. It establishes context, stack health, quoted Compose
   input, credential-reference presence, control-plane root, server-side probe
   mode, registry scope, and manifest ownership before a retry.
2. Capture `bridgectl version`, safe context names, server/SDK version when
   available, timestamp with timezone, and the smallest affected API or tool.
   Do not source `.env`; use Compose's `--env-file` and inspect only booleans,
   names, status codes, and timings.
3. Classify the failure: context/authentication, role authorization, API
   connectivity, credential source or rotation, schema/registry, MCP
   client/SDK protocol handling, cache, or server runtime. Route stable
   onboarding/API codes through the numbered
   [recovery branches](onboarding.md#recovery-branches).
4. Inspect the narrowest evidence first: health checks, server-side API probe,
   tool describe/readback, bounded log stats/tail, then cache stats. Use
   repository history and `git status` without altering unrelated work.
5. Reproduce with a minimum safe request. Do not use production personal data,
   raw tokens, authorization headers, ERP credentials, upstream bodies, or
   plugin payloads in reproduction artifacts.
6. State the observed result, expected result, stable code, and evidence that
   separates them. If an operation would alter production state, obtain the
   change gate described in `SKILL.md`.

A changed quoted environment value requires the explicit Compose
`--force-recreate` path; recreating a container does not prove health. For a
file-backed resource, inspect only the presence, permissions, regular-file
status, and bounded size of the reference-named file under
`ERPBRIDGE_CREDENTIALS_DIR`; never print its content or use an environment
fallback. The normal `bridgectl api test` is server-side and body-free. Use
`--local` only when the report explicitly labels a host-side offline
diagnostic.

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
