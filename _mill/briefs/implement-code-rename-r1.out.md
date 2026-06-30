No uncommitted tracked changes. All clean.

{"status":"success","commit_sha":"cdd9965cd3fc13ad87563203d8e32b2d927caa7e","session_id":"477bc462-901e-4b7d-aae4-7837e9342f2f"}

All 4 cards are complete and verified:

- Card 1: `internal/ghissuesengine/ghissues.go` moved to `internal/selfreportengine/selfreport.go`; package declaration and doc comments updated.
- Card 2: `internal/ghissuescli/cli.go` and `cli_test.go` moved to `internal/selfreportcli/`; package, import, `Use`, `Short`, `Long`, and all comments updated.
- Card 3: `cmd/lyx/main.go` import/registration/root Long updated; `helptree_test.go` and `jsonhelp_test.go` pinned guards updated including test function renames.
- Card 4: `internal/lyxtest/leaf_enforcement_test.go` banned-import entries and `doc.go` package doc updated.

`go build ./... && go test ./...` pass cleanly with all 32 packages building and all test packages green, including `internal/selfreportcli`, `internal/lyxtest`, and `cmd/lyx`.

{"status":"success","commit_sha":"cdd9965cd3fc13ad87563203d8e32b2d927caa7e","session_id":"477bc462-901e-4b7d-aae4-7837e9342f2f"}
