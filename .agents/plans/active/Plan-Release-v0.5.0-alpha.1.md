# Plan: ERPBridge v0.5.0-alpha.1 release

## Goal

Prepare the next ERPBridge prerelease from the current `main` candidate. The
release must include the post-`v0.4.0-alpha.3` MCP tool annotations and
namespaced discovery metadata, split tool generation output, and the completed
control-plane, security, resilience, plugin, and Console work. Release
execution remains separate from this plan until the candidate passes the live
matrix and local release gates.

## Current State

- The latest repository tag is `v0.4.0-alpha.3`; the current candidate is
  ahead of that tag and contains the MCP metadata, annotation, split-output,
  and live-E2E follow-up commits (`git log --oneline v0.4.0-alpha.3..HEAD`).
- Release automation starts on any `v*` tag and runs Go lint before the
  GoReleaser job (`.github/workflows/release.yml:3-27`). The release job builds
  the web assets, runs GoReleaser, publishes GitHub artifacts and packages,
  and has signing identity permission (`.github/workflows/release.yml:29-87`).
- GoReleaser builds the server and `bridgectl` for the supported OS/architectures
  with version, commit, and date linker values, and builds the multi-platform
  server image (`.goreleaser.yaml:12-101`). It also generates checksums,
  archive SBOMs, and checksum signatures (`.goreleaser.yaml:102-151`).
- Runtime binaries default to `dev`; release identity is injected by linker
  flags (`services/erpbridge-server/main.go:29-34`,
  `tools/bridgectl/main.go:8-11`).
- The changelog has one accumulated `Unreleased` section and an outdated
  comparison link that must be cut and relinked for the next tag
  (`CHANGELOG.md:8-94`, `CHANGELOG.md:435-436`).
- The disposable black-box workspace requires unique Compose projects,
  environment-only credentials, bounded evidence, and a finalizer-scoped
  teardown (`/home/nimendra/Documents/Projects/Erpbridge-demo/README.md:13-53`).
  Its phase runner builds fresh containers and verifies ERPBridge and MockERP
  health (`/home/nimendra/Documents/Projects/Erpbridge-demo/scripts/run-phase.sh:6-14`,
  `/home/nimendra/Documents/Projects/Erpbridge-demo/scripts/run-phase.sh:152-193`).

## Decisions

- **Target `v0.5.0-alpha.1`.** This is the user-selected new minor alpha
  release from the current `v0.4.0-alpha.3` line; do not promote to a stable
  `v0.5.0` release while the repository remains on the alpha line.
- **Run the full live matrix before release preparation.** A fixture limitation
  may remain `BLOCKED_FIXTURE` only when it has a sanitized report and does
  not weaken a production safety rule. A product regression is a release
  blocker.
- **Keep release automation unchanged unless verification exposes a concrete
  defect.** The existing tag-triggered GoReleaser workflow is the release
  contract; this plan does not add a second publishing path.
- **Do not tag or push during candidate validation.** Tagging, GitHub release
  publication, and GHCR publication are final actions after the candidate is
  explicitly approved.
- **Keep schemas, MCP negotiation, credentials boundaries, and authorization
  semantics unchanged.** Annotations and `_meta` remain optional client hints;
  server-side authorization remains authoritative.

## Scope

### In scope

- Candidate audit from `v0.4.0-alpha.3` to `HEAD`.
- Fresh Docker live testing of all documented black-box phases and the plugin
  integration boundary.
- Go, frontend, lint, asset, and release-snapshot verification.
- Cutting the accumulated changelog into `v0.5.0-alpha.1` and correcting
  comparison links.
- A final release checklist for tag, artifact, image, checksum, SBOM, and
  signature verification.

### Out of scope

- Implementing unrelated product changes discovered during the release audit.
- Weakening security or changing fixtures to turn a blocked capability into a
  false positive.
- Publishing a tag, GitHub release, container image, or release commit before
  candidate approval.

## Tasks

- [x] **Task 1: Freeze and audit the release candidate.** Review the complete
  diff from `v0.4.0-alpha.3`, confirm the working tree and public-docs sync
  state, and check that no credentials or generated runtime state are part of
  the candidate. **Seam:** Git history, changelog, release workflow, and
  GoReleaser configuration. **Files:** `CHANGELOG.md`, `.goreleaser.yaml`,
  `.github/workflows/release.yml`; **Verify:** `git diff --check
  v0.4.0-alpha.3..HEAD` passed; ERPBridge source was clean except this new
  release plan; the public docs checkout was clean; `.env.example` was
  classified as a template, not a credential; no release-tracked secret was
  found.

- [x] **Task 2: Run the complete fresh-container live matrix.** Use a unique
  `live-*` Compose project for each `redis-plugin`, `memory`, `protected`,
  `open`, `cors`, `rate`, and `redis-unavailable` phase. Run onboarding,
  control-plane, HTTP MCP, stdio, generated-tool, cache, auth, CORS, rate,
  plugin, and failure-policy checks, including a reviewed explicit metadata
  probe that proves explicit `false` annotations, namespaced `_meta`, generated
  method hints, sensitive-field exclusion, and an authorized guarded call.
  Run the standalone plugin integration check as well. **Seam:** external
  black-box HTTP/MCP/stdio boundaries and sanitized evidence. **Files:**
  `/home/nimendra/Documents/Projects/Erpbridge-demo/scripts/run-phase.sh`,
  `/home/nimendra/Documents/Projects/Erpbridge-demo/scripts/onboarding.sh`,
  `/home/nimendra/Documents/Projects/Erpbridge-demo/scripts/test-*.py`,
  `scripts/test-plugin-integration.sh`; **Verify:** 2026-08-28 run passed all
  available checks: onboarding, 11 control-plane checks, 9 HTTP MCP checks,
  stdio, plugin/auth/cache/resilience checks, 100 generated-tool calls,
  protected/open/CORS/rate/Redis-unavailable phases, and the standalone plugin
  integration test. The phase finalizers removed 7 unique projects and all
  runtime secrets; the standalone plugin check removed its own Compose
  project; no product failure was observed.

- [x] **Task 3: Run local release quality gates.** Verify Go tests, frontend
  type checks/tests/format, frontend lint, Go lint, embedded asset checks,
  binary builds, and a GoReleaser snapshot or configuration validation without
  publishing. **Seam:** repository build and test entry points. **Files:**
  `Makefile`, `.goreleaser.yaml`, `web/`; **Verify:** `make test`,
  `make web-test`, `make web-lint`, `make lint`, `make build`,
  `./scripts/verify-console-assets.sh`, and `goreleaser check` all passed.

- [ ] **Task 4: Cut and synchronize release notes.** Move the accumulated
  `Unreleased` entries into `v0.5.0-alpha.1` with the verified release date,
  retain an empty `Unreleased` section, and update comparison links. Keep the
  root developer documentation and the public documentation checkout aligned
  for the user-visible MCP and CLI behavior. **Seam:** Keep a Changelog and
  paired documentation review. **Files:** `CHANGELOG.md`, relevant `docs/`
  pages, `/home/nimendra/Documents/Projects/erpbridge-docs/CHANGELOG.md`, and
  affected public pages; **Verify:** changelog link checks, a clean Docusaurus
  `npm run build`, and `git diff --check` in both repositories.

- [ ] **Task 5: Execute the approved release.** After all prior tasks are
  green, create the annotated `v0.5.0-alpha.1` tag at the reviewed commit and
  push only the tag. Confirm the GitHub Actions release workflow publishes the
  expected archives, binaries, multi-platform image, checksums, SBOMs, and
  signatures, then verify artifact checksums and version/commit/date metadata.
  **Seam:** Git tag trigger and GoReleaser publication workflow. **Files:** no
  source files; Git tag and release artifacts only. **Verify:** workflow is
  green, GitHub release assets are complete, GHCR image tags resolve, and
  published checksums/signatures verify locally.

## Verification

- No product failures remain unexplained in the sanitized live capability
  matrix. The available live checks passed; controlled ERP fault modes and
  plugin fault modes not exposed by the pinned fixtures remain fixture-only
  coverage gaps.
- `make test`, frontend checks, scoped lint, asset verification, and the
  non-publishing release check are green.
- Changelog and public documentation builds are synchronized.
- No credentials, ERP bodies, plugin payloads, token values, raw logs, or
  runtime state are committed or retained after cleanup.
- Release publication is not attempted until the candidate checklist is
  reviewed and approved.

## Open Questions

- The full pi-lens baseline reports four blocking findings outside the release
  plan: three `zizmor:cache-poisoning` findings in
  `.github/workflows/release.yml` and one pre-existing schema finding in
  `.lefthook-local.yml`. The local test and lint commands pass; the user has
  explicitly accepted these pre-existing findings for this requested alpha
  release, and they remain a documented release risk.
- A stable release would require a new plan and explicit scope change.
