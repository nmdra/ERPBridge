# Six-Invariant ToolPlane Conformance

Status: **not-release-verified**
Architecture coverage: **partial**
Release integrity: **unknown**
Historical run attribution: **unknown**

| Invariant | Status | Scenarios |
|---|---|---|
| erp-tool-separation | observed | AC-01-happy-path |
| reviewed-desired-resources | observed | AC-03-admission-tamper |
| discovery-convergence | observed | AC-04-reconcile-restart |
| active-resource-authorization | observed | AC-06-authz-denied, AC-07-queued-withdrawal |
| serving-revision-binding | observed | AC-08-serving-transition, AC-09-bound-active-revision, AC-10-stale-or-inactive-revision |
| secret-indirection | observed | AC-01-happy-path |

Credential values were not recorded.

Limitations: single-process isolated deployment; author-authored oracle; fixed ERPNext profile.
