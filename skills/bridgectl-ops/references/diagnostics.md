# Diagnose and report ERPBridge issues

## Triage loop

1. Capture the selected context, `bridgectl version`, server/SDK version when
   available, timestamp with timezone, and the smallest affected API or tool.
2. Classify the failure: context/authentication, role authorization, API
   connectivity, schema/registry, MCP client/SDK protocol handling, cache, or
   server runtime.
3. Inspect the narrowest evidence first: API test, tool describe/readback, log
   stats/tail, cache stats, then the relevant source and tests. Use repository
   history and `git status` without altering unrelated work.
4. Reproduce with a minimum safe request. Do not use production personal data,
   raw tokens, authorization headers, or ERP credentials in reproduction
   artifacts.
5. State the observed result, the expected result, and the evidence that
   separates them. If an operation would alter production state, obtain the
   change gate described in `SKILL.md`.

## Plugin diagnosis

For plugin or binding failures, read [Plugins](plugins.md) before changing
state. Classify the failure as strict manifest validation, protected admission
or endpoint allowlist, exact reference resolution, missing credential,
timeout, non-2xx response, malformed or oversized response, transformed-output
schema failure, binding failure policy, or stale cache. Start with plugin and
binding readback, bounded log stats/tail, and cache stats. Reproduce with safe
deterministic data and preserve the distinction between an original result and
a transformed result.

Keep plugin evidence redacted: omit authorization headers, credential values,
sensitive endpoints, request/response payloads, plugin response bodies, ERP
records, and opaque invocation tokens. For repository diagnosis, use the
existing plugin unit tests and `go test -tags pluginintegration
./internal/integration -run TestPluginSystemBlackBox -count=1` when its fixture
stack is available.

## Bug-report workflow

Create `.agents/reports/<YYYY-MM-DD>-<slug>.md` from
`assets/bug-report.md` at the skill root. Sanitize it before writing:
replace credentials, authorization headers, cookies, personal data, and opaque
tokens with descriptive placeholders. Do not stage or commit a report draft
unless the user requests it.

Offer GitHub issue creation only after the user approves the final title and
body. The issue must link to the affected component/version and include the
minimal reproduction, expected/actual behavior, redacted evidence, and impact.
