{"status":"success","commit_sha":"985703615c3da6328b0cd82e744ddcd0c991c567","session_id":"7a5bbd4a-96e7-46f9-bc08-6fa4f9eba6c4","cards_done":[13,14,15,16]}

Summary: 4 of 4 cards committed (Card 13, 14, 15, 16 — all complete, confirmed by matching commit subjects against the batch file's declared `Commit:` messages in `git log 4cb37249..HEAD --oneline`).

Work done:
- `internal/websterengine/state.go` — `Dir`/`ReportsDir`/`ScratchDir`/`PromptsDir` converted to take `anchorRoot string`; dropped `internal/lyxcwd` import; doc comments updated, `ReportsDir`'s stray "Hub Geometry Invariant" reference corrected to the Cwd Resolution Invariant.
- `internal/websterengine/doc.go` — "engine/cli split" section corrected to credit `websterengine` itself (not `internal/lyxcwd`) with declaring `_lyx/webster`/`.lyx/webster` subpaths.
- `internal/websterengine/webstergeom_test.go` — both existing tests now capture `l.AnchorPath()` into a local and pass the string; added `TestWebsterGeometryHelpers_ToldDirectory` driving all four accessors with a plain told directory, no Location involved.
- `internal/webstercli/cli.go`, `cli_test.go`, `verbs_test.go` — all four accessor call sites now pass `layout.AnchorPath()`.
- `cmd/lyx/constructoranchoring_test.go`, `cmd/lyx/notransients_test.go` — all rows/table entries converted to pass `l.AnchorPath()`.
- `internal/reedengine/lifecycle_test.go` — reworded one comment sentence (batch 1's file, out-of-original-scope) that spelled out the literal token `exec.Command` in prose and false-triggered the Hermetic Git Test Environment / Test Tier Purity enforcement scans in `./cmd/lyx/...`, which is part of this batch's `verify:`. Confirmed via `standalone-producers` comparison that this is a same-task (batch 1) regression this batch's `verify:` is the first to surface, not pre-existing on the parent branch and not caused by this batch's own edits. Extended `_mill/plan/05-webster-accessors-told.md` card 16's `Edits:` list and committed that plan edit before making the code change, per the file-scope protocol.

Verify: `go test ./internal/websterengine/... ./internal/webstercli/... ./cmd/lyx/...` and `go test -tags integration ./internal/webstercli/...` both pass.

{"status":"success","commit_sha":"985703615c3da6328b0cd82e744ddcd0c991c567","session_id":"7a5bbd4a-96e7-46f9-bc08-6fa4f9eba6c4","cards_done":[13,14,15,16]}
