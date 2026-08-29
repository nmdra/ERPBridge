# Plan: Increase default tool rate limits

## Goal

Increase ERPBridge's default tool-execution token bucket to 10 requests per
second with a burst of 20, matching the main Docker Compose configuration.
Keep explicit environment overrides and rate-limit enforcement behavior
unchanged.

## Current State

- `parseRateLimitConfig` defaults to 5 requests per second and burst 10 in
  `services/erpbridge-server/main.go:59-78`.
- The main `docker-compose.yml:53-54` already configures 10 requests per
  second and burst 20.
- The example environment file and environment-variable reference document
  5/10 in `.env.example:7-8` and `docs/environment-variables.md:19-20`.
- The demo Compose fallback and connectivity guide also document 5/10 in
  `/home/nimendra/Documents/Projects/Erpbridge-demo/compose/erpbridge-server.compose.yml:13-14`
  and `docs/connectivity.md:87-88`.
- Rate settings are validated as positive and finite before server startup;
  explicit environment values must continue to override the defaults.

## Decisions

- Set the application default, example configuration, demo Compose fallback,
  and documentation to 10 RPS / burst 20.
- Do not change the token-bucket algorithm, enforcement scope, error contract,
  or MCP protocol version.
- Do not add rate-limit metadata as part of this change; that remains the scope
  of issue #10.

## Scope

In scope: default values, focused parser coverage, local and public
configuration documentation, changelog entries, and the completed plan record.

Out of scope: runtime rate-limit semantics, deployment secrets, issue #10
metadata, and unrelated skill-refresh changes.

## Tasks

- [x] Task 1: Add a parser test for the 10/20 defaults. (**Seam:**
  `parseRateLimitConfig`; **Files:** `services/erpbridge-server/main_test.go`;
  **Verify:** `go test ./services/erpbridge-server -run TestParseRateLimitConfig`)
- [x] Task 2: Change the server and configuration defaults to 10/20 while
  preserving overrides. (**Seam:** server startup configuration;
  **Files:** `services/erpbridge-server/main.go`, `.env.example`,
  `/home/nimendra/Documents/Projects/Erpbridge-demo/compose/erpbridge-server.compose.yml`;
  **Verify:** focused parser tests and `make test`)
- [x] Task 3: Synchronize rate-limit documentation and changelogs. (**Seam:**
  documented configuration contract; **Files:** `docs/connectivity.md`,
  `docs/environment-variables.md`, `CHANGELOG.md`,
  `/home/nimendra/Documents/Projects/erpbridge-docs/docs/erpbridge/connectivity.mdx`,
  `/home/nimendra/Documents/Projects/erpbridge-docs/CHANGELOG.md`;
  **Verify:** `git diff --check` and public docs build)
- [x] Task 4: Close the plan after verification. (**Seam:** repository plan
  workflow; **Files:** `.agents/plans/README.md`,
  `.agents/plans/completed/README.md`, this plan;
  **Verify:** plan is prefixed `[COMPLETED]` and listed in completed indexes)

## Verification

- `go test ./services/erpbridge-server -run TestParseRateLimitConfig`
- `make test`
- `git diff --check`
- `npm --prefix /home/nimendra/Documents/Projects/erpbridge-docs run build`
- Confirm explicit `RATE_LIMIT_RPS` and `RATE_LIMIT_BURST` overrides still pass
  existing validation tests.

## Open Questions

None.
