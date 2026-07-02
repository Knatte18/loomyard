All batch cards are committed, verify passed, and the working tree is clean.

**Summary of changes:**
- `internal/initcli/initcli.go` — registered the `--undo` flag on `lyx init`, dispatching to `runUndo` while `runInit` stays unchanged; updated `Short`/`Long` help text.
- `internal/initcli/undo.go` (new) — implements `runUndo`: `warpengine.UnwireJunctions` (hard-abort on inconsistency), weft-side `_lyx` clearing with a weft-worktree-existence guard, unconditional `weftengine.Commit`/`Push`, and `gitignore.Remove`.
- `internal/initcli/undo_test.go` (new) — integration tests: happy path, never-initialized, never-weft-paired, idempotent re-run, real-directory guard, target-mismatch guard, and partial-recovery (junction-only crash, and committed-but-unpushed crash with a real push assertion).
- `internal/weftengine/sync.go` — fixed `Commit` to tolerate git's "pathspec ... did not match any files" as a no-op (needed for `--undo` run twice against an already-fully-removed pathspec to stay idempotent, per Card 8's unconditional-Commit requirement). This was a discovered-during-implementation fix; I recorded it in `_mill/plan/04-initcli-undo.md`'s Card 8 (extending its `Edits:` list) and committed that plan edit before touching the code, per the brief's file-scope protocol.
- `internal/weftengine/sync_test.go` — added `TestCommit_PathspecAlreadyRemoved` covering the fix.
- `docs/overview.md` — extended the **init** module bullet to document `--undo`.

Verify command `go test -tags integration ./internal/initcli/... -count=1` passes, as does `go build ./...` and `go test -tags integration ./internal/weftengine/... ./internal/weftcli/...`.

{"status":"success","commit_sha":"18005a790062554d5591c161b74731cc3499b51d","session_id":"9519cdd3-9e4e-4c8d-82be-61570890600c"}