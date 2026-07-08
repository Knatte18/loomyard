{"status":"success","commit_sha":"c8b11b839ea4ab8273021dfd6618450eddc33bca","session_id":"779e2984-c2d2-4321-a645-f707dfd2ffe4"}

## Summary

Implemented batch `05-docs-lifecycle` (Card 11) for the burler docs lifecycle:

- Deleted `docs/modules/burler.md` (`git rm`) — durable design already lives in `internal/burlerengine/doc.go`.
- `docs/overview.md`: rewrote the burler module bullet to Implemented, marked the execution-stack diagram's burler line done, updated the "no product CLI" line to note `lyx burler run` is debug-only, added an as-built `internal/burlerengine` bullet to "Other docs", and retargeted the `modules/perch.md`/`modules/burler.md` and reviews/README.md pointer bullets.
- `docs/roadmap.md`: marked the burler half of milestone 11 done (perch half + milestone stay open), added a new `### Deferred burler enhancements` section (cluster-N, generic tools-restriction, bulk+caching, per-round provider selector), and retargeted the milestone-23 and milestone-24 `burler.md` back-references at the deferred section / package doc.
- `docs/modules/README.md`: replaced the `burler.md` table row with an as-built row following the mux/shuttle pattern.
- `docs/modules/perch.md`, `docs/modules/loom.md`, `docs/modules/hardener.md`, `docs/reviews/README.md`, `docs/shared-libs/stencil.md`: replaced every `burler.md` link with a non-link reference to the `internal/burlerengine` package documentation.
- Discovered mid-batch that `internal/burlercli/run.go`'s help-text example profile pointed `target.paths` at `docs/modules/burler.md` (about to be deleted). Per protocol, extended `_mill/plan/05-docs-lifecycle.md`'s Edits list first (committed as `plan: extend docs-lifecycle refs for stale burler.md example path`, pushed), then repointed the example to `docs/overview.md`.

Verified zero remaining `burler.md` references anywhere outside `_mill/`. Ran `go build ./...` and `go test ./...` — all packages pass. Final commit `c8b11b8` on branch `internal-burler`, pushed to origin.

Files touched (all absolute paths under `C:\Code\loomyard\wts\internal-burler`):
- `docs\modules\burler.md` (deleted)
- `docs\overview.md`
- `docs\roadmap.md`
- `docs\modules\README.md`
- `docs\modules\perch.md`
- `docs\modules\loom.md`
- `docs\modules\hardener.md`
- `docs\reviews\README.md`
- `docs\shared-libs\stencil.md`
- `internal\burlercli\run.go`
- `_mill\plan\05-docs-lifecycle.md` (plan extension)

{"status":"success","commit_sha":"c8b11b839ea4ab8273021dfd6618450eddc33bca","session_id":"779e2984-c2d2-4321-a645-f707dfd2ffe4"}