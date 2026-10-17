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
- [x] Publish the exact verified commit. **Files:** annotated tag `v0.6.0-alpha.1`, no workflow changes. **Seam:** existing GitHub release workflow. **Verify:** origin/main equals release commit, tag points to it, successful release workflow, GitHub prerelease with archives/checksums/signatures/SBOMs, and a recorded multi-architecture GHCR digest. Verify a downloaded CLI archive checksum and release version/MCP identity. Push synchronized public docs after artifact publication; do not merge unrelated work or force push.

## Candidate evidence

- User confirmed `v0.6.0-alpha.1` and approved fixing the release-version mismatch before publication.
- The initial linker-version smoke and new `tools/bridgectl/main_test.go` regression both reproduced MCP reporting `dev` while the CLI reported `0.6.0-alpha.1`. Setting `cli.Version` alongside `cli.RootCmd.Version` at the executable entry point makes both identities agree.
- `make test`; fresh race tests for `tools/bridgectl`, `internal/cli`, and `internal/climcp`; scoped CLI-entry-point lint (0 issues); active delta/full LSP checks (0 errors); `goreleaser check`; public-site build (76 documents); and both repositories' `git diff --check` pass.
- Changelog release date was read from the system clock: 2026-10-17. Current public ERPBridge release indicators are prepared for publication; SDK v1.1.0 and historical records remain unchanged.

## Published outcome

- Release: https://github.com/nmdra/ERPBridge/releases/tag/v0.6.0-alpha.1, published 2026-10-17 as a non-draft prerelease.
- Annotated tag and release workflow both resolve to `7a2d7a4233cee679f745d2788c44b679f68916e8`. Origin/main matched that commit at tagging.
- Workflow https://github.com/nmdra/ERPBridge/actions/runs/39031853926 succeeded, including lint and GoReleaser. An independent verifier confirmed 18 nonempty uploaded assets: 8 binary archives, 8 matching SBOMs, checksums, and signature.
- Image `ghcr.io/nmdra/erpbridge-server:0.6.0-alpha.1` has index digest `sha256:274940d90c368eec23b38529ac19b6b67e1a59b88399e236e6a984857d1ba7b6`, with runnable linux/amd64 and linux/arm64 manifests. The existing latest tag resolves to the same index.
- Downloaded `bridgectl_Linux_x86_64.tar.gz` matches its published SHA256 `17fca94c90bc4337de3c4373a6d8de6abb11aadf519dfbd09470af6a378fc8e0`. The published binary reports `0.6.0-alpha.1` through both CLI version and MCP initialize; default discovery returns 12 read-only tools. EOF shutdown passes. No ERP request or persistent configuration write was made.
- Public docs commits `8b31cb6` and `f7ec709` are pushed. Deployment https://github.com/nmdra/erpbridge-docs/actions/runs/39032415455 succeeded.
- The prep commit does not match the console workflow path filters. Equivalent candidate console checks passed locally: types, all 56 frontend tests, lint, format, build, and embedded assets. The preceding merged main commit also passed its GitHub console workflow.

## Follow-up and limits

- No production rollout or live registry change was performed.
- Existing dependency audit findings remain. Candidate npm audit reported 13 findings (3 moderate, 8 high, 2 critical); no dependency upgrades were included in this release.
- Signature/SBOM assets were confirmed present; cryptographic signature validity and server image execution were not part of artifact acceptance. CLI archive contents and checksum were checked as described above.
- Existing unrelated dirty plan indexes and untracked plans remain untouched.
