# Plan: ERPBridge v0.5.0-alpha.6 release

## Goal

Publish the read cache TTL change: generated GET and HEAD tools default to 60 seconds instead of
300 seconds. Write methods keep `enabled: false`.

## Reason

The live ERPNext catalogue caches reads for five minutes. A one-minute TTL returns fresher ERP data
while keeping the shared read-only cache.

## Tasks

- [x] **Task 1: Change the default.** `internal/idp/generator.go`: `generatedReadCacheTTLSeconds`
  300 to 60. (**Verify:** `go test ./internal/idp -count=1`.)
- [x] **Task 2: Update tests and documentation.** `internal/idp/generator_test.go` and
  `docs/tool-schema.md`. (**Verify:** focused tests pass; the documentation states one minute.)
- [ ] **Task 3: Release.** Commit, tag `v0.5.0-alpha.6`, push, and let the workflow publish the
  GitHub release and the GHCR image. (**Verify:** workflow success and recorded digest.)
- [ ] **Task 4: Live registry.** Set `cache.ttlSeconds` to 60 on the live read revisions. This is
  tracked in `.agents/plans/Plan-ERPNext-Surface-Batch-A-B.md` follow-up work.

## Notes

The TTL is part of the revision digest, so the live update needs a delete and re-apply of each read
revision at the same version. The operator revision `get_company@1.0.1` keeps its own 30 second TTL.
