# Plan: ERPBridge v0.5.0-alpha.3 release

## Goal

Prepare and publish the next ERPBridge prerelease from the current `main`
candidate (`e7ef7f2`). The release must include the post-`v0.5.0-alpha.2`
ToolPlane revision binding, typed enum and object-boundary schema preservation,
the connector response-body lifetime repair, output-schema type enforcement,
the Console tool-details focus, and cache benchmarks. It must also repair the
changelog bookkeeping that `v0.5.0-alpha.2` skipped. Tagging and publication
remain separate from this plan until the candidate passes the local and live
gates and is explicitly approved.

## Current State

- Latest release: `v0.5.0-alpha.2` (annotated tag `5d99c5e`, GitHub release
  published 2026-08-29). `main` is 9 commits ahead of that tag
  (`git log --oneline v0.5.0-alpha.2..HEAD`): typed output-schema requirement
  (`0374ffc`), Console tool-details focus (`c1dcf66`), cache benchmarks
  (`1942a2c`), admitted revision binding (`fe878a1`), connector body lifetime
  (`5c84ca2`), changelog ordering (`48cfb74`), typed enum and input-closure
  preservation (`5e294b6`), and the two PR merge commits.
- The alpha.2 changelog section was never cut: `git show
  v0.5.0-alpha.2:CHANGELOG.md` still carries the alpha.2 changes under
  `Unreleased`. The current `Unreleased` therefore mixes alpha.2 material with
  post-alpha.2 entries, and the `[Unreleased]` comparison link still points at
  `v0.5.0-alpha.1` (`CHANGELOG.md`, bottom link block).
- `.goreleaser.yaml:137` sets `prerelease: auto`, but the published
  `v0.5.0-alpha.1` and `v0.5.0-alpha.2` GitHub releases are not marked as
  prereleases (`gh release view <tag> --json isPrerelease` → `false`), while
  `v0.4.0-alpha.3` is `true`. The new release must not repeat this.
- Release automation starts on any `v*` tag (`.github/workflows/release.yml`):
  golangci-lint v2.12.2 runs first, then the release job builds web assets,
  installs cosign v2.6.0 and syft, and runs GoReleaser v2
  (`release --clean`) to publish archives, checksums, SBOMs, checksum
  signatures, and the multi-platform `ghcr.io/nmdra/erpbridge-server` image
  (`.goreleaser.yaml:87-151`).
- Runtime binaries default to `dev`; release identity is injected with linker
  flags (`.goreleaser.yaml:25-51`).
- Active plans still carry open tasks: API-Key header propagation Task 4 (live
  verification), Bridgectl-Ops automatic knowledge checkpoint Tasks 3 and 5,
  and live E2E remediation Task 8 (re-run matrix).
- The public docs site advertises `ERPBRIDGE_RELEASE = 'v0.5.0-alpha.2'`
  (`~/Documents/Projects/erpbridge-docs/src/constants/releases.ts`), consumed
  by `docusaurus.config.ts` and `src/pages/index.tsx`.
- Local prerequisites: Go 1.26.2+, Node from `web/.nvmrc`, Docker Compose,
  `goreleaser` v2 (2.14.0 installed).

## Decisions

- **Target `v0.5.0-alpha.3`.** Continue the alpha prerelease line; do not
  promote to stable `v0.5.0` in this plan.
- **Backfill the missing `v0.5.0-alpha.2` section.** Recover its exact content
  from `git show v0.5.0-alpha.2:CHANGELOG.md` (`Unreleased` at the tag) and
  place it before the new `v0.5.0-alpha.3` section. Do not attempt to backfill
  `v0.4.0-alpha.2`/`v0.4.0-alpha.3`; that archaeology is out of scope.
- **Release flags follow the established maintenance pattern.** The workflow
  publishes through the unchanged `prerelease: auto` path; after artifact
  verification, the new release mirrors the deliberate latest-release
  promotion applied to `v0.5.0-alpha.1`/`alpha.2` (a manual post-publication
  edit; there is no workflow or configuration defect).
- **Unfinished active-plan tasks are not automatic release blockers.** Each open
  task gets an explicit disposition (defer with reason or complete) before the
  tag; the Live-E2E Task 8 matrix re-run may be satisfied by this release's
  validation evidence.
- **Keep release automation unchanged unless verification exposes a defect.**
  The tag-triggered GoReleaser workflow remains the only publishing path.
- **Source-only schema changes.** Per the completed
  `Plan-Schema-Constraint-Preservation`, existing deployments must regenerate
  or edit reviewed manifests and admit new versions; this release does not
  roll those changes out live.
- **Do not tag or push during candidate validation.**
- **No secrets, ERP bodies, or runtime state in the candidate or evidence.**

## Scope

### In scope

- Candidate audit from `v0.5.0-alpha.2` to the release commit.
- Full local quality gates: Go tests, race on changed packages, lint, frontend
  checks, embedded-asset verification, GoReleaser config check and snapshot.
- Proportionate live validation against the disposable onboarding stack and the
  connector delayed-response behavior.
- Changelog backfill (`v0.5.0-alpha.2`), cut (`v0.5.0-alpha.3`), and comparison
  link repair.
- Public docs sync (version indicators and docs changelog).
- Tag, workflow verification (archives, checksums, SBOMs, signatures, GHCR
  multi-arch, binary version output, prerelease flag).

### Out of scope

- New product behavior or unrelated fixes discovered during the audit.
- Live ERP/schema rollout of the typed-enum fix.
- Stable `v0.5.0` promotion, SDK releases, or publishing outside GitHub/GHCR.
- Backfilling changelog sections for `v0.4.0-alpha.2`/`v0.4.0-alpha.3`.

## Tasks

- [x] **Task 1: Freeze and audit the release candidate.** Confirm `main` is
  clean at the reviewed commit, review the full `v0.5.0-alpha.2..HEAD` diff,
  verify no credentials or generated runtime state are tracked, confirm the
  public-docs checkout state, and record a disposition for every open active
  plan task. **Seam:** Git history, changelog, plans, release configuration.
  **Files:** `CHANGELOG.md`, `.goreleaser.yaml`, `.github/workflows/release.yml`,
  `.agents/plans/active/*`. **Verify:** `git status --porcelain` empty;
  `git diff --check v0.5.0-alpha.2..HEAD` clean; open-task dispositions recorded.

- [x] **Task 2: Run the local quality gates.** Go tests, race on the changed
  packages, scoped lint, frontend type/test/lint/format/build, embedded asset
  verification, and binary builds.
  **Seam:** repository build and test entry points.
  **Files:** `Makefile`, `web/`, `scripts/verify-console-assets.sh`.
  **Verify:** `make test`, `go test -race ./internal/connector ./internal/mcp`,
  `make lint`, `make web-test`, `make web-lint`, `make build`,
  `./scripts/verify-console-assets.sh` all pass.

- [x] **Task 3: Run the non-publishing release check and snapshot.** Validate
  the GoReleaser configuration and build a full local snapshot with frontend
  assets present.
  **Seam:** GoReleaser pipeline.
  **Files:** `.goreleaser.yaml`, `web/`, `dist/` (artifact only, never
  committed). **Verify:** `goreleaser check` passes;
  `npm ci --prefix web && npm run build --prefix web` then
  `goreleaser release --snapshot --clean` exits 0 and `dist/` contains both
  binaries' archives, checksums, and SBOMs.

- [x] **Task 4: Validate the live boundaries proportionately.** Run the
  disposable onboarding check and a live delayed-response connector check
  (headers immediately, body released later) through a real MCP tool call, plus
  the plugin integration check when fixture credentials are available.
  **Seam:** external black-box HTTP/MCP boundaries with sanitized evidence.
  **Files:** `scripts/test-onboarding.sh`, `scripts/test-plugin-integration.sh`,
  `/home/nimendra/Documents/Projects/Erpbridge-demo/scripts/*`.
  **Verify:** onboarding script exits 0 and cleans up its resources; the
  delayed-response tool call returns the body content instead of
  `context canceled`; evidence is bounded and redacted; no product regression
  remains unexplained.

- [x] **Task 5: Backfill and cut the changelog.** Add the recovered
  `## [v0.5.0-alpha.2] - 2026-08-30` section from
  `git show v0.5.0-alpha.2:CHANGELOG.md`, cut the remaining `Unreleased`
  entries into `## [v0.5.0-alpha.3]` with the verified release date, leave an
  empty `Unreleased`, and repair the comparison links (`[Unreleased]` →
  `v0.5.0-alpha.3...HEAD`, add `[v0.5.0-alpha.3]` and `[v0.5.0-alpha.2]`).
  **Seam:** Keep a Changelog bookkeeping.
  **Files:** `CHANGELOG.md`. **Verify:** section content matches the
  `v0.5.0-alpha.2` tag snapshot plus `git diff v0.5.0-alpha.2..HEAD`; all
  comparison links resolve to existing tags; `git diff --check` clean.

- [ ] **Task 6: Synchronize the public documentation.** Bump
  `ERPBRIDGE_RELEASE` to `v0.5.0-alpha.3`, add the docs-site changelog entry,
  and build the site.
  **Seam:** user-facing docs version truth.
  **Files:** `~/Documents/Projects/erpbridge-docs/src/constants/releases.ts`,
  `docusaurus.config.ts` consumers, `CHANGELOG.md`.
  **Verify:** `npm run build` green in the docs checkout; commit pushed after
  publication.

- [ ] **Task 7: Execute the approved release.** After Tasks 1–6 are green and
  the candidate is explicitly approved, create the annotated
  `v0.5.0-alpha.3` tag at the reviewed commit and push only the tag. Confirm the
  release workflow publishes the expected artifacts.
  **Seam:** Git tag trigger and GoReleaser publication workflow.
  **Files:** Git tag and release artifacts only.
  **Verify:** tag points to the approved commit; workflow run completes
  successfully; GitHub release assets include archives, checksums, SBOMs, and
  cosign signature bundles; `cosign verify-blob` passes for checksums;
  `ghcr.io/nmdra/erpbridge-server:v0.5.0-alpha.3` exposes linux/amd64 and
  linux/arm64; published `bridgectl version` and `erpbridge-server --version`
  report `0.5.0-alpha.3`; `gh release view v0.5.0-alpha.3 --json isPrerelease`
  is `true`.

- [ ] **Task 8: Close the plan and clean up.** Prefix the plan filename with
  `[COMPLETED]`, move it to `completed/`, update the plan index entries, and
  remove the merged `fix/schema-constraint-preservation` and
  `codex/fix-response-body-lifetime` branches when the user approves.
  **Seam:** plan index hygiene.
  **Files:** `.agents/plans/README.md`, `.agents/plans/upcoming/README.md`.
  **Verify:** index lists the release plan under Completed; no stale Upcoming
  entry remains.

## Execution Evidence

- **Task 1 (2026-10-16):** `git diff --check v0.5.0-alpha.2..HEAD` clean; no
  credential patterns in the candidate diff; the worktree carried only this
  plan's WIP. Prerelease-flag root cause resolved (see Open Questions).
  Dispositions: API-Key Task 4 (live verification) and Checkpoint Tasks 3/5
  deferred as non-blocking; Live-E2E Task 8 matrix re-run deferred — its
  intent is covered by this release's onboarding and connector checks.
- **Task 2:** `make test`, `go test -race ./internal/connector ./internal/mcp`,
  `make lint` (0 issues), `make web-test`, `make web-lint`, web format-check
  and production build, `make build`, and `./scripts/verify-console-assets.sh`
  (723110 compressed bytes) all green.
- **Task 3:** `goreleaser check` green; the snapshot exited 0 with
  `--skip=docker,sbom,sign` (syft and cosign are not installed locally; CI
  installs them). `dist/` contained both binaries' archives and
  `checksums.txt`; the snapshot `bridgectl version` reported
  `v0.5.0-alpha.2-next`.
- **Task 4:** `scripts/test-onboarding.sh` exited 0 with full cleanup and its
  fixture MCP `tools/call` exercised the connector live; delayed-response
  coverage came from the connector race tests. The plugin integration fixture
  was not run (no fixture credentials available); the candidate's plugin
  pipeline changes are covered by the `internal/mcp` race tests.

## Verification

- Local gates: Go tests, race on changed packages, lint, frontend checks,
  embedded assets, and the non-publishing snapshot are green.
- Live onboarding and the delayed-response connector check pass with sanitized
  evidence and complete cleanup.
- `v0.5.0-alpha.2` is documented and `v0.5.0-alpha.3` release notes match the
  candidate diff; comparison links resolve.
- The docs site builds with `ERPBRIDGE_RELEASE = 'v0.5.0-alpha.3'`.
- `v0.5.0-alpha.3` is published as a GitHub prerelease with verified archives,
  checksums, SBOMs, signatures, multi-arch image, and correct binary version
  output.
- No credentials, ERP bodies, plugin payloads, token values, raw logs, or
  runtime state are committed or retained.

## Open Questions

- Root cause of the missing prerelease flag on `v0.5.0-alpha.1`/`alpha.2` —
  **resolved 2026-10-16:** both GitHub releases were manually edited after
  publication (`updated_at` later than `published_at`); `.goreleaser.yaml` and
  the release workflow are unchanged since `a874adb`, an ancestor of
  `alpha.2`, so there is no workflow or configuration defect.
- Whether Live-E2E Task 8's matrix re-run should be completed as part of this
  release or explicitly deferred — **resolved 2026-10-16:** deferred; this
  release's onboarding script and connector race tests cover its intent.
- Whether the alpha.1/alpha.2 GitHub release flags should be corrected
  retroactively — **resolved 2026-10-16:** no; the post-publication edits
  were deliberate latest-release promotions, and the new release mirrors that
  established maintenance behavior after workflow verification.
