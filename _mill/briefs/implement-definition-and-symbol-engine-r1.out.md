All 5 cards (28, 29, 30, 31, 32) have matching commits, and the batch declares 5 cards — an exact match. All cards committed, verify passed.

{"status":"success","commit_sha":"4fdba719","session_id":"851e087b-b42c-4515-af00-438e91eed23d","cards_done":[28,29,30,31,32]}

Summary: 5 of 5 cards committed for batch `definition-and-symbol-engine`. Verify (`go test -count=1 ./internal/codeintelengine/...`) passes, including race detector runs.

Files touched:
- `/home/knatte/Code/loomyard/wts/codeintel-v1/internal/codeintelengine/lspclient.go` — added `lspClient.definition` + `parseDefinitionResult` (card 28), added `Kind int` field to `symbolInformation` (card 30)
- `/home/knatte/Code/loomyard/wts/codeintel-v1/internal/codeintelengine/definition.go` (new) — `Definition`, sharing the `lookup` pipeline with `References` (card 29)
- `/home/knatte/Code/loomyard/wts/codeintel-v1/internal/codeintelengine/symbol.go` (new) — `SymbolMatch`, `symbolFromClient`, `Symbol` (card 30)
- `/home/knatte/Code/loomyard/wts/codeintel-v1/internal/codeintelengine/lspclient_test.go` — table-driven `TestLSPClient_DefinitionParsesMultipleWireShapes` covering bare Location, Location[], LocationLink[], null (card 31)
- `/home/knatte/Code/loomyard/wts/codeintel-v1/internal/codeintelengine/definition_test.go` (new) — `TestDefinition_NonExistentServerBinaryYieldsErrServerNotFound` (card 31)
- `/home/knatte/Code/loomyard/wts/codeintel-v1/internal/codeintelengine/symbol_test.go` (new) — legacy-path regression proof plus `symbolFromClient` coverage for two-candidates/zero-candidates/unsupported-capability paths (card 32)

{"status":"success","commit_sha":"4fdba719","session_id":"851e087b-b42c-4515-af00-438e91eed23d","cards_done":[28,29,30,31,32]}
