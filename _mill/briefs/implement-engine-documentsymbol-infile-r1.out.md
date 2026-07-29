All 4 cards (Card 3, 4, 5, 6) are committed since the batch start, matching the batch's declared 4 cards exactly. Verify passed.

{"status":"success","commit_sha":"7998f2fe","session_id":"07e00689-386e-4122-93c7-f1553b425de9","cards_done":[3,4,5,6]}

Summary: 4 of 4 cards committed — all complete.

Files touched (absolute paths):
- `/home/knatte/Code/loomyard/wts/codeintel-daemon-persistence/internal/codeintelengine/lspclient.go` (Card 3: `lspDocumentSymbol` wire type, `documentSymbol()`, `DocumentSymbolProvider`/`supportsDocumentSymbol()`)
- `/home/knatte/Code/loomyard/wts/codeintel-daemon-persistence/internal/codeintelengine/doc.go` (Cards 3+4: method-count/list update, `--in-file` resolver-mode description)
- `/home/knatte/Code/loomyard/wts/codeintel-daemon-persistence/internal/codeintelengine/refs.go` (Card 4: `InFileQuery`, `Query.InFile`, `resolvePosition`'s InFile branch, `collectInFileMatches`)
- `/home/knatte/Code/loomyard/wts/codeintel-daemon-persistence/internal/codeintelengine/lspclient_test.go` (Card 5: `documentSymbol` parse + capability-parse tests)
- `/home/knatte/Code/loomyard/wts/codeintel-daemon-persistence/internal/codeintelengine/refs_test.go` (Card 6: `collectInFileMatches` unit tests, end-to-end `resolvePosition`/InFile tests)

`go test ./internal/codeintelengine/...` (the batch's `verify:`) passes with no failures; git status is clean (all changes committed and pushed).

{"status":"success","commit_sha":"7998f2fe","session_id":"07e00689-386e-4122-93c7-f1553b425de9","cards_done":[3,4,5,6]}
