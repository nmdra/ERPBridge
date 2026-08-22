# Plan: Bridgectl Operations Skill

## Goal

Replace the narrow API-registration skill with a production-grade, model-invoked ERPBridge operations skill. It must cover onboarding, lifecycle maintenance, authentication and authorization, observability, troubleshooting, documentation discovery, and sanitized bug reporting.

## Task

- [x] **O1 — Create the `bridgectl-ops` skill.** Renamed the existing package; replaced stale instructions with workflow-specific references; added current AuthN/AuthZ, SDK, and public documentation context; removed unsafe duplicated command guidance; and added tool/report templates. **Verify:** `npx --yes skills-ref validate skills/bridgectl-ops`; all entry-point relative links resolve; stale-name and raw-secret audits are clean; `make test` is green.
