All 5 cards (33-37) declared in this batch are complete: card 33 committed alone (925b7826), cards 34+35 combined into one commit named per card 35's message (7eded480, as explicitly permitted for cards that necessarily touch the same file), card 36 (4fb44af5), and card 37 (65a9f5f3). Verify (`go test -count=1 ./internal/codeintelcli/... ./cmd/lyx/...`) passes, and the working tree is clean of tracked changes.

One notable finding during Card 37: the plan's literal test assertion (checking that the `symbol` verb's `ErrNoLanguage` envelope contains the literal string `"foo.go:1:1"`) was factually impossible given the actual code — `DetectLanguage` (`internal/codeintelengine/detect.go`) never reads `Options.Query` at all, so `refs foo.go:1:1` and `symbol foo.go:1:1` against an empty target dir produce byte-identical JSON output (verified empirically). I resolved this by extracting `symbolCommand`'s one-line query construction into a small named helper `symbolQuery(arg string) codeintelengine.Query` in `internal/codeintelcli/cli.go` (within the file's existing edit scope for cards 35/37) and wrote `TestRunCLI_Symbol_TreatsFileLineColArgumentAsLiteralSearchString` to pin the real, testable contract: `symbolQuery` keeps the argument literal (`Pos == nil`) while the same string driven through `parseQuery` (which `refs`/`definition` use) does parse as a position — while still driving the full `RunCLI` path to confirm the `ErrNoLanguage` envelope fires as expected.

Relevant files (all absolute paths under `/home/knatte/Code/loomyard/wts/codeintel-v1`):
- `internal/codeintelcli/cli.go` — `emitLookupResult`, `definitionCommand`, `symbolCommand`, `symbolQuery`, `symbolMatchFields`, updated `Command()`/package doc.
- `internal/codeintelcli/cli_test.go` — four new tests per card 37.
- `cmd/lyx/helptree_test.go` — pinned `codeintel` subcommand set.

{"status":"success","commit_sha":"65a9f5f3","session_id":"a83f535e-4885-42cd-b937-d0e49339bdf4","cards_done":[33,34,35,36,37]}
