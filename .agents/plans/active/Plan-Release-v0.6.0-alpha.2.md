# Plan: ERPBridge v0.6.0-alpha.2 release

Status: Approved by the user. Publish v0.6.0-alpha.2 using existing release automation, merge companion public docs, and finish on main. Preserve unrelated working-tree changes.

## Goal and Evidence

Publish the merged Windows CLI safeguards, native Windows CI, PowerShell guidance, and embedded bridgectl-ops 3.5.0 guidance. PR #21 is already merged at `379ec0d0ece7b45f9eaf70794787295c174d5e24`. Both main workflows passed: Windows/Linux compatibility run `39215307195` and Console run `39215307285`. Latest existing release is v0.6.0-alpha.1. Public docs PR #6 was approved for merge and merged for this release.

The release pipeline runs on pushed v* tags (`.github/workflows/release.yml`). GoReleaser produces six bridgectl archives, two Linux server archives, checksums/signature and archive SBOMs, and publishes the Linux multi-architecture server image (`.goreleaser.yaml`). Its existing version/latest image tags are unchanged. GitHub prerelease classification remains automatic; v0.6.0-alpha.2 must be verified as a prerelease. Do not change the previous release's Latest status.

## Scope and Constraints

- No production code, Linux behavior, dependencies, deployment settings, or release-workflow changes.
- Do not publish from the dirty working tree locally. Build/publication runs from the exact committed tag in GitHub Actions.
- Do not commit unrelated plan, memory, or Audit Report files. Never move or overwrite an existing remote tag or release.
- Manual Windows browser launch, interactive Ctrl+C cleanup, a clean-user installation walkthrough, PowerShell 5.1 runtime, and ARM64 runtime remain unverified. Publish these limits, not a claim that their checks passed. User approved alpha publication with these limits; do not close the original Windows plan falsely.
- Release-preparation commits are made on short-lived local branches, reviewed, fast-forwarded to main, and pushed. Finish both checkouts on main. No live ERP operation is authorized.

## Tasks

- [ ] **1. Prepare source release metadata.** Cut CHANGELOG Unreleased to v0.6.0-alpha.2 using the UTC date from `date -u +%Y-%m-%d`, keep a fresh Unreleased section and comparison links, update Windows guide release examples to this version, and add curated release notes with exact manual-validation limits. **Files:** `CHANGELOG.md`, `docs/windows.md`, new `docs/releases/v0.6.0-alpha.2.md`, this plan. **Seam:** changelog/guide/version references and existing release configuration. **Verify:** `make test`; `goreleaser check`; `git diff --check`; review scoped diff before commit; merged-source main Windows/Linux and Console CI already green with no runtime changes in this task.
- [ ] **2. Sync public release documentation.** Merge companion PR #6, update ERPBridge/CLI release indicators from v0.6.0-alpha.1 to v0.6.0-alpha.2 without changing SDK version or historical entries, update Windows examples and validation wording, and record the documentation change in Unreleased. **Files:** public `src/constants/releases.ts`, current-release mentions in relevant `docs/` pages, `docs/bridgectl/windows.mdx`, `CHANGELOG.md`. **Seam:** site version labels and user instructions. **Verify:** `npm --prefix ../erpbridge-docs run build`; `git diff --check` in that repo; diff review; merged PR and pushed main commit confirmed.
- [ ] **3. Publish and verify exact tag.** Confirm local main equals intended origin/main, remote tag does not exist, all release-preparation commits are pushed, and notes are ready. Push a new annotated v0.6.0-alpha.2 tag pointing at that exact source commit. Wait for the existing Release with GoReleaser workflow. Apply curated release notes after successful automatic publication without changing assets or prerelease classification. Download release artifacts to a fresh temporary directory, compare SHA256 entries, inspect both Windows ZIPs for bridgectl.exe, and execute the Linux x64 CLI's version command against the intended version. Inspect the published server image manifest for both Linux architectures and record its digest. **Files:** this plan's evidence only; release/tag metadata and existing pipeline outputs. **Seam:** GitHub release assets/version, GHCR manifest, CI completion. **Verify:** `gh run view <release-run> --json conclusion` is success; `gh release view v0.6.0-alpha.2` shows expected tag/prerelease and asset set; downloaded checksum comparisons pass; Linux archive binary reports 0.6.0-alpha.2; image manifest has linux/amd64 and linux/arm64. Confirm both worktrees finish on main with unrelated source changes preserved.

## Completion

Record exact source/docs commits, tag, workflow/release URLs, asset/checksum verification, and image digest. Archive only this release plan after all three task gates pass. Keep the Windows implementation plan active for its manual gates. Any failed publication stops automatic retries until the failure and existing remote state are inspected. Do not delete/replace remote artifacts to hide a failure.
