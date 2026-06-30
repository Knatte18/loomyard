Tree is clean, all 8 cards committed, and verify passes in full (`go build`, `go test`, `go vet -tags integration` all exit 0).

{"status":"success","commit_sha":"6f6b168145623df865f802ea6659fb1a54c59114","session_id":"64e80e11-c737-4e5b-beea-9dcd4888bfe8"}

Batch `code-rename` complete. Summary:

- **Card 1** — `git mv` moved all 9 files from `internal/paths/` to `internal/hubgeometry/` (renaming `paths.go`/`paths_test.go`/`paths_unit_test.go` to `hubgeometry.*`), preserving git rename history. Surgical edits: package clauses (`hubgeometry` / `hubgeometry_test`), the package doc comment, black-box test imports/qualifiers, the two enforcement allowlist literals (`"internal/hubgeometry"` at `internal/hubgeometry/enforcement_test.go` lines 69 and 347), and the codeguide-guard filename skip (`"hubgeometry.go"` at `internal/hubgeometry/codeguide_guard_test.go` line 48).
- **Cards 2–8** — retargeted the 53 importer files (cmd/lyx, board, config family, ide, warp family, weft, leaf consumers) by swapping the `internal/paths` import for `internal/hubgeometry` and `paths.<Exported>` selectors for `hubgeometry.<Exported>`, plus comment updates. The lyxtest Leaf Invariant is preserved — `leaf_enforcement_test.go`'s `bannedImports` list was left untouched (only its doc comment changed).

Verification: `go build ./...` (0), `go test ./...` (0, including the `internal/hubgeometry` enforcement/codeguide guards and `internal/lyxtest` leaf guard), and `go vet -tags integration ./...` (0).

One scope note for the reviewer: six **out-of-scope** files carry stale `paths.<Exported>` references in **comments only** (not imports) and were deliberately not in any card or the plan's "All Files Touched" union, so I left them untouched per the no-out-of-scope-edits rule. They do not affect the build/tests and are not matched by the Shared Decision's end-state grep set. They are:
- `cmd/lyx/unknown_subcommand_test.go` (lines 58, 73)
- `internal/boardengine/config.go` (lines 21, 60)
- `internal/boardengine/template_test.go` (line 28)
- `internal/idecli/cli_test.go` (line 20)
- `internal/muxpoccli/muxpoc_smoke_test.go` (line 55)
- `internal/warpcli/clone_cli_test.go` (lines 79, 82)

{"status":"success","commit_sha":"6f6b168145623df865f802ea6659fb1a54c59114","session_id":"64e80e11-c737-4e5b-beea-9dcd4888bfe8"}
