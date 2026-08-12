All 5 cards of batch `cli-envelope` (cards 23–27) are committed and verified. Summary of work:

- `internal/fabriccli/envelope.go` (new) — `okWithRecord`/`errWithRecord` helper pair declaring the fixed `mutations`/`partial` (and conditional `refusal`) key set once.
- `internal/fabriccli/fabric.go`, `internal/fabriccli/unwire.go` — topology verb handlers (`add`, `checkout`, `prune`, `cleanup`, `remove`, `unwire`) route through the helpers; pre-mutation `output.Err` sites stay bare with a one-line carve-out comment; `list`/`pairs` untouched.
- `internal/fabriccli/clone.go` — `CloneAndWire` converts to named results with a defer-populated recorder seeded from `CloneHub`'s own record, recording the repo-wide/per-worktree config writes and the weft:main commit (Bolt.Push deliberately unrecorded, with reasoning comment); `runCloneWithReset` emits through the helpers.
- `internal/fabriccli/fabric.go` (`runReconcile`) — builds its recorder before the first mutation (`ReconcileFabricAt`), records the config write and commit backfill, emits through the helpers on every post-`l` return.
- `internal/fabriccli/weft_verbs.go` — `commit`/`push`/`pull`/`sync` route through the helpers; `push`/`sync` concatenate composed-call records in execution order; `push --bypass` uses an empty hub root; `sync` records exactly one `push_spawned` entry, never `branch_pushed`.
- `internal/fabriccli/envelope_test.go` (new, untagged) and `internal/fabriccli/cli_test.go` (integration-tagged, extended) — assert the envelope shape end-to-end, including a real gate refusal driven through `fabric unwire` against a hand-drifted `_board` junction.

Verify command passed: `go test ./internal/fabriccli/ ./internal/output/ && go test -tags integration ./internal/fabriccli/`. Working tree is clean (no uncommitted tracked changes).

{"status":"success","commit_sha":"91a1a35df0360f7d0256e3cf4204ca1e856e5dec","session_id":"ac031fe0-5c08-4f56-b701-9f109fe3f0df","cards_done":[23,24,25,26,27]}
