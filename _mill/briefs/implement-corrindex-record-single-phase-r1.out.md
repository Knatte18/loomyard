1 of 1 card committed — the batch's single Card 4 is complete and matches its declared `Commit:` message exactly.

{"status":"success","commit_sha":"413ae4809325cef1b7afb8ba394e1b43e4d5936a","session_id":"86a933c1-db48-414f-a416-4980bc444b75","cards_done":[4]}

Summary: 1 of 1 cards committed for batch `corrindex-record-single-phase`. I added the reproducing test `TestCorrIndex_RecordDoesNotClobberConcurrentExternalWrite` in `/home/hanf/Code/loomyard/wts/fabric-corrindex-record-race/internal/fabricengine/corrindex_test.go`, confirmed it failed pre-fix, then rewrote `corrIndex.record` in `/home/hanf/Code/loomyard/wts/fabric-corrindex-record-race/internal/fabricengine/corrindex.go` to compose its upsert via `state.UpdateJSON` against the freshly-read on-disk base instead of the stale `ix.recs` snapshot, updating the doc comment per the plan's requirements. `go test ./internal/fabricengine/...` passes. Working tree is clean (no dirty tracked files). Committed as `413ae4809325cef1b7afb8ba394e1b43e4d5936a` and pushed to `origin/fabric-corrindex-record-race`.

{"status":"success","commit_sha":"413ae4809325cef1b7afb8ba394e1b43e4d5936a","session_id":"86a933c1-db48-414f-a416-4980bc444b75","cards_done":[4]}
