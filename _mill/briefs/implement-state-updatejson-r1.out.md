All 3 cards committed and matched against the batch's declared `Commit:` messages exactly. 3 of 3 cards committed — all complete.

{"status":"success","commit_sha":"827ae12da0f48627cb53275c34b7f5afc9da2173","session_id":"8b91524e-f5e3-471c-8f10-312f9c017201","cards_done":[1,2,3]}

Summary: All 3 cards in batch `01-state-updatejson` are committed (3 of 3). Verify (`go test ./internal/state/...`) passes.

Files touched:
- `/home/hanf/Code/loomyard/wts/fabric-corrindex-record-race/internal/state/state.go` — extracted `readJSONUnlocked`/`writeJSONUnlocked` lock-free cores (card 1), added `UpdateJSON[T any](path, lockPath string, mutate func(cur T, found bool) (T, error)) error` (card 2).
- `/home/hanf/Code/loomyard/wts/fabric-corrindex-record-race/internal/state/update_test.go` — new test file covering missing-file, existing-file, mutate-error, corrupt-file, and concurrency dispositions of `UpdateJSON` (card 2).
- `/home/hanf/Code/loomyard/wts/fabric-corrindex-record-race/internal/state/doc.go` — new godoc package comment stating the locked read-modify-write rule (card 3).

{"status":"success","commit_sha":"827ae12da0f48627cb53275c34b7f5afc9da2173","session_id":"8b91524e-f5e3-471c-8f10-312f9c017201","cards_done":[1,2,3]}
