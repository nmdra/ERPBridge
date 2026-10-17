# Plan: ERPBridge v0.6.0-alpha.1 feature release

## Goal

Publish merged PR #20 as the user-approved minor alpha release `v0.6.0-alpha.1`. Stay on main and preserve unrelated local plan files. The user approved publishing through the existing workflow, including its GHCR latest tag update.

## Evidence and scope

- PR #20 merged as `2927190`; local main was fast-forwarded to origin/main. Main's console workflow run `39030646614` passed.
- The latest published release is `v0.5.0-alpha.6`; the requested tag does not exist remotely.
- `.github/workflows/release.yml` runs lint and GoReleaser on pushed `v*` tags. `.goreleaser.yaml` builds Linux server binaries, cross-platform CLI archives, checksums, SBOMs, signatures, and linux/amd64 plus linux/arm64 GHCR images. Prerelease detection is automatic.
- Current Unreleased changelog describes the management MCP feature and its safety/CLI fixes. The existing alpha.6 active plan describes a different published release and live registry follow-up; do not execute or edit it.
- Public documentation currently advertises an older release. Keep its current-release indicators synchronized without rewriting historical entries.

## Decisions

- New minor prerelease rather than a stable declaration or alpha.7 patch-series continuation.
- Preserve existing workflow/dependency versions. No production deployment, registry migration, or ERP calls.
- Validate release-injected binary version and MCP initialize metadata before tagging. Stop for a scoped correction if metadata disagrees.
- Do not include user-owned dirty plan indexes or untracked plans in release commits.

## Tasks

- [x] Prepare release metadata and verify the candidate. **Files:** `CHANGELOG.md` and this plan; narrowly scoped version-wiring regression only if confirmed and approved; public docs current-release constants/status pages and changelog plus matching plan. **Seam:** full Go tests, release-ldflags binary build and stdio initialization, console pipeline, public-site build. **Verify:** `make test`; focused race/lint on any changed Go packages; release CLI `version`/MCP identity smoke; `npm --prefix ../erpbridge-docs run build`; `git diff --check`. Commit scoped release changes to main with the user's release authorization.
- [ ] Publish the exact verified commit. **Files:** annotated tag `v0.6.0-alpha.1`, no workflow changes. **Seam:** existing GitHub release workflow. **Verify:** origin/main equals release commit, tag points to it, successful release workflow, GitHub prerelease with archives/checksums/signatures/SBOMs, and a recorded multi-architecture GHCR digest. Verify a downloaded CLI archive checksum and release version/MCP identity. Push synchronized public docs after artifact publication; do not merge unrelated work or force push.

## Candidate evidence

- User confirmed `v0.6.0-alpha.1` and approved fixing the release-version mismatch before publication.
- The initial linker-version smoke and new `tools/bridgectl/main_test.go` regression both reproduced MCP reporting `dev` while the CLI reported `0.6.0-alpha.1`. Setting `cli.Version` alongside `cli.RootCmd.Version` at the executable entry point makes both identities agree.
- `make test`; fresh race tests for `tools/bridgectl`, `internal/cli`, and `internal/climcp`; scoped CLI-entry-point lint (0 issues); active delta/full LSP checks (0 errors); `goreleaser check`; public-site build (76 documents); and both repositories' `git diff --check` pass.
- Changelog release date was read from the system clock: 2026-10-17. Current public ERPBridge release indicators are prepared for publication; SDK v1.1.0 and historical records remain unchanged.

## Completion

Report release URL, commit, tag, workflow evidence, published image reference/digest, and remaining follow-up. No live rollout is included.
