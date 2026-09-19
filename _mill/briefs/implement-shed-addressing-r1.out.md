All 5 cards (27, 28, 29, 30, 31) committed, verify passes in full (`go build ./...`, the four-package untagged test set, and the integration-tagged shedcli suite). No uncommitted tracked changes remain.

Key files touched:
- `internal/loomcli/arm.go`, `internal/battencli/arm.go` -- added `ArmAt`/`armAt` Location-taking entry points (card 27)
- `internal/shedcli/table.go`, `internal/shedcli/doc.go`, `internal/shedcli/cli.go` (incl. extracted `armFromSeed`) -- deleted `--recipe`, seed-driven `resolvePersistentPreRun` (card 28)
- `internal/shedcli/seed.go`, `internal/shedcli/seed_test.go` -- new `lyx shed seed` command (card 29)
- `internal/shedcli/table_test.go`, `internal/shedcli/cli_test.go`, `internal/shedcli/parity_test.go` -- rewritten arming/precedence/parity tests (card 30)
- `CONSTRAINTS.md`, `cmd/lyx/helptree_test.go`, `internal/shedverbs/step.go` -- invariant/doc updates (card 31)
- `cmd/lyx/constructoranchoring_test.go`, `cmd/lyx/notransients_test.go` -- fixed pre-existing breakage from earlier same-task batches (deleted `loomengine.LoomStatusFile`/`LoomStatusLock`/`LoomRunLock`, misclassified `battencli.StatusFile`), surfaced by this batch's own verify
- `_mill/plan/07-shed-addressing.md` -- two scope-extension plan edits, each committed before its corresponding code change

{"status":"success","commit_sha":"63a9a2413dd4715ce78b6f99e6e3d5e5df3b82cc","session_id":"a66a298d-f8e6-441e-8b93-ac5032f9891a","cards_done":[27,28,29,30,31]}
