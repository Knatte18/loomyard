{"status":"success","commit_sha":"ef2a18ab0e501d1e4bdf5bc43a2b745b9c9a4733","session_id":"f42648da-82c7-4d9d-be87-ed7097e3ef0e","cards_done":[8,9,10,11,12,13]}

All 6 of 6 cards in batch `02-rundir-clear.md` (cards 8 through 13) are committed, one commit per card, verified against the range start `ef283edb` (the `mill-go: start batch rundir-clear` commit). `verify:` (`go test ./internal/shedadapters/... ./internal/shedrecipe/... ./internal/loomrecipe/...`) passes. Working tree is clean (no uncommitted tracked changes).

Summary of what changed, by file (all paths relative to `/home/knatte/Code/loomyard/wts/loom-bouncer-anchor-rundir-fix`):

- `internal/shedadapters/archive.go`, `internal/shedadapters/archive_test.go` — Card 8: added `archiveRunDir`, the run-directory archive-and-recreate helper.
- `internal/shedadapters/bouncer.go` — Card 9: split `judged` into `judgedVerdict` (returns the parsed verdict) plus a thin `judged` wrapper.
- `internal/shedadapters/bouncer.go`, `internal/shedadapters/bouncer_clear_test.go` (new), `internal/shedadapters/bouncer_replay_test.go`, `internal/shedadapters/bouncer_commit_test.go` — Card 10: the clear-and-re-seed step in `Call`, its four falsified doc/comment sites, and full test coverage plus repairs to the two test files the change falsified.
- `internal/shedadapters/doc.go`, `internal/shedadapters/burler.go` — Card 11: rewrote the package-doc "Outcome mapping"/"round-artifact convention" sections and `BurlerProducer`'s two-row budget-cap doc.
- `contracts/recipes/loom-recipe.yaml`, `internal/loomrecipe/revalidate_test.go` — Card 12: comment-only rewrites of `Plan-Revalidate`'s `on_stuck` rationale and the revalidate test's doc comment.
- `internal/shedrecipe/entries_bouncer_test.go` — Card 13: repaired `TestBouncerEntry_CommitSeam`'s three subtests to drive the harvest vehicle instead of the removed APPROVED-replay vehicle, via a new local `judgeSeamFakeShuttle`.

Note per the Test Integrity Guardrail: `TestBouncer_Clear_ArchiveFailureDegradesToStuck` in `bouncer_clear_test.go` covers only the "rename failure" trigger for `archiveRunDir`'s degrade path, not a separately-isolated "recreate failure" trigger — the test's own comment documents why: under POSIX permission semantics, `os.MkdirAll`'s write requirement on `runDir`'s parent is a strict subset of what `os.Rename` already needs there, so a parent permission that lets the rename proceed necessarily lets the recreate proceed too, making the two failure causes mechanically inseparable at this call site without filesystem-level dependency injection. This was a judgment call, not a skipped requirement; flagging it for reviewer awareness.
