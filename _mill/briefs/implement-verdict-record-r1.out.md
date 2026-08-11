All 3 cards accounted for: card 1 (commit 130a2c69), card 2 (commit be1f621d), and card 3 (Commit: none, verified via wiki read-back). 3 of 3 cards committed/completed — this is the full batch.

{"status":"success","commit_sha":"be1f621dc27a07b840f233d8c1c6be5f9da0d8fa","session_id":"92dc2aca-9401-43e4-8805-c409320f3bac","cards_done":[1,2,3]}

Summary: all 3 of 3 cards in batch `01-verdict-record.md` are complete.

- Card 1 (`docs(gitexec): record the RunGit error-shape verdict`, commit `130a2c69`): rewrote `/home/hanf/Code/loomyard/wts/gitexec-error-shape-decision/manifest/designs/gitexec-error-shape.md` in full as the recorded verdict, transcribed from `_mill/discussion.md`.
- Card 2 (`docs(roadmap): replace the gitexec decision item with the implementation item`, commit `be1f621d`): replaced the roadmap's decision item at `/home/hanf/Code/loomyard/wts/gitexec-error-shape-decision/manifest/roadmap.md` with the implementation item.
- Card 3 (`Commit: none`): filed `gitexec-checked-entry-point` in the wiki via `wiki._client.upsert_task`, `depends_on: ['fabric-corrindex-record-race']`, and verified via `get_task` read-back (slug, title, and `depends_on` all matched) — no git diff produced, as expected.

Both `go run ./tools/mdreflow` runs reported 0 mismatches, and `go test ./internal/lyxcwd/ -run TestEnforcement_MarkdownLinks` passed after each edit. Working tree is clean.

{"status":"success","commit_sha":"be1f621dc27a07b840f233d8c1c6be5f9da0d8fa","session_id":"92dc2aca-9401-43e4-8805-c409320f3bac","cards_done":[1,2,3]}