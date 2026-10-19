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

- [x] **1. Prepare source release metadata.** Cut CHANGELOG Unreleased to v0.6.0-alpha.2 using the UTC date from `date -u +%Y-%m-%d`, keep a fresh Unreleased section and comparison links, update Windows guide release examples to this version, and add curated release notes with exact manual-validation limits. **Files:** `CHANGELOG.md`, `docs/windows.md`, new `docs/releases/v0.6.0-alpha.2.md`, this plan. **Seam:** changelog/guide/version references and existing release configuration. **Verify:** `make test`; `goreleaser check`; `git diff --check`; review scoped diff before commit; merged-source main Windows/Linux and Console CI already green with no runtime changes in this task.
- [x] **2. Sync public release documentation.** Merge companion PR #6, update ERPBridge/CLI release indicators from v0.6.0-alpha.1 to v0.6.0-alpha.2 without changing SDK version or historical entries, update Windows examples and validation wording, and record the documentation change in Unreleased. **Files:** public `src/constants/releases.ts`, current-release mentions in relevant `docs/` pages, `docs/bridgectl/windows.mdx`, `CHANGELOG.md`. **Seam:** site version labels and user instructions. **Verify:** `npm --prefix ../erpbridge-docs run build`; `git diff --check` in that repo; diff review; merged PR and pushed main commit confirmed.
- [x] **3. Publish and verify exact tag.** Confirm local main equals intended origin/main, remote tag does not exist, all release-preparation commits are pushed, and notes are ready. Push a new annotated v0.6.0-alpha.2 tag pointing at that exact source commit. Wait for the existing Release with GoReleaser workflow. Apply curated release notes after successful automatic publication without changing assets or prerelease classification. Download release artifacts to a fresh temporary directory, compare SHA256 entries, inspect both Windows ZIPs for bridgectl.exe, and execute the Linux x64 CLI's version command against the intended version. Inspect the published server image manifest for both Linux architectures and record its digest. **Files:** this plan's evidence only; release/tag metadata and existing pipeline outputs. **Seam:** GitHub release assets/version, GHCR manifest, CI completion. **Verify:** `gh run view <release-run> --json conclusion` is success; `gh release view v0.6.0-alpha.2` shows expected tag/prerelease and asset set; downloaded checksum comparisons pass; Linux archive binary reports 0.6.0-alpha.2; image manifest has linux/amd64 and linux/arm64. Confirm both worktrees finish on main with unrelated source changes preserved.

## Completion

Record exact source/docs commits, tag, workflow/release URLs, asset/checksum verification, and image digest. Archive only this release plan after all three task gates pass. Keep the Windows implementation plan active for its manual gates. Any failed publication stops automatic retries until the failure and existing remote state are inspected. Do not delete/replace remote artifacts to hide a failure.

## Published outcome

All three task gates passed. Release is published and verified.

- **Source preparation:** commit `ee67ea8` (`chore(release): prepare v0.6.0-alpha.2`) on `main`; `origin/main` equaled `ee67ea8` at tagging. `make test`, `goreleaser check`, and `git diff --check` passed.
- **Public documentation:** commit `d07191d` (`docs(release): sync ERPBridge v0.6.0-alpha.2`) pushed to `erpbridge-docs` `main`; companion PR #6 was already merged as `f32027b`; `npm run build` succeeded (77 documents). SDK `v1.1.0` and historical entries were left unchanged.
- **Tag:** annotated `v0.6.0-alpha.2`, tag object `26f9884`, target commit `ee67ea83da5f6afe538161aa4bc31ea84ac99672`. It did not exist remotely before the push.
- **Workflow:** https://github.com/nmdra/ERPBridge/actions/runs/39221921379 completed with `success`.
- **Release:** https://github.com/nmdra/ERPBridge/releases/tag/v0.6.0-alpha.2, published 2026-10-19T08:26:05Z, non-draft, `isPrerelease: true`. Curated notes from `docs/releases/v0.6.0-alpha.2.md` were applied with `gh release edit --notes-file`; draft and prerelease flags were unchanged. 18 assets: 8 archives, 8 archive SBOMs, `checksums.txt`, and `checksums.txt.sig`.
- **Checksum verification:** all 16 entries in `checksums.txt` matched the downloaded files (`sha256sum -c`, 16 OK, 0 FAILED) in a fresh temporary directory.
- **Archive inspection:** `bridgectl_Windows_x86_64.zip` and `bridgectl_Windows_arm64.zip` each contain `bridgectl.exe` (24234496 and 22617088 bytes).
- **Binary identity:** the extracted `bridgectl_Linux_x86_64` archive ran `bridgectl version` and reported `0.6.0-alpha.2`.
- **Server image:** `ghcr.io/nmdra/erpbridge-server:0.6.0-alpha.2` is an OCI image index with digest `sha256:3a630862e8d61785a9aabcdad5aa50710e10b899c3cdf8846f723ffbcf55243c`, carrying `linux/amd64` (`sha256:ed85d0747f025de8f638cf9e336608e78558959253eefcff9628fe035d519372`) and `linux/arm64` (`sha256:a449b2b2552a9785a3743789ee8cf3525248349774caa86e4f58a61f095cdc7e`).
- **Worktrees:** the ERPBridge checkout and the `erpbridge-docs` checkout both finish on `main`. Unrelated working-tree changes (plan indexes, untracked plans, skill memory, Audit Report, security-report) were preserved and not committed.

## Remaining limits

- The Windows manual gates stay unverified and are published as limits: default-browser launch, interactive Ctrl+C cleanup, a clean non-admin installation walkthrough, PowerShell 5.1 runtime, and ARM64 runtime. `Plan-Bridgectl-Windows-Support.md` remains active for those gates.
- No live ERP operation, production deployment, or registry migration was performed. The previous release's GitHub Latest status was not changed.
- Cryptographic signature validity of `checksums.txt.sig` and server image execution were not part of acceptance.
