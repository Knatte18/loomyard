All packages pass. The whole module-wide Tier 1 loop is green, confirming the guard is clean across the tree.

{"status":"success","commit_sha":"4e03204","session_id":"a022da72-e4d2-4c9f-98ff-42a8732ed677"}

Summary: 1 of 1 card committed for batch `tier-purity-guard` (card 9), plus one preparatory plan-scope-extension commit.

- Card 9 commit `4e03204` — created `C:\Code\loomyard\wts\test-suite-regression\cmd\lyx\tierpurity_test.go` with `TestTierPurity_UntaggedTestsSpawnNothing`, and added the `## Test Tier Purity Invariant` section to `C:\Code\loomyard\wts\test-suite-regression\CONSTRAINTS.md`.
- While implementing, the guard's deliberate raw-substring matching correctly flagged three pre-existing untagged doc-comment mentions of banned tokens left over from batch 2: `C:\Code\loomyard\wts\test-suite-regression\cmd\lyx\main_test.go`, `C:\Code\loomyard\wts\test-suite-regression\internal\perchcli\cli_test.go`, `C:\Code\loomyard\wts\test-suite-regression\internal\perchcli\run_test.go`. Per protocol I stopped, extended card 9's `Edits:` list in `C:\Code\loomyard\wts\test-suite-regression\_mill\plan\03-tier-purity-guard.md` (commit `2f25dcd`), then reworded the three comments (meaning unchanged, e.g. `gitexec.RunGit` → `gitexec's RunGit`).
- Batch verify `go test ./cmd/lyx -run TestTierPurity -count=1` passes. Additionally ran `go test ./... -count=1` (full module) — all packages green.
- golangci-lint on the changed packages (`./cmd/lyx/...`, `./internal/perchcli/...`) reported only one pre-existing, unrelated finding in `drift_test.go` (not touched by this batch).
- `git status --porcelain --untracked-files=no` is clean; no uncommitted tracked changes.

{"status":"success","commit_sha":"4e03204","session_id":"a022da72-e4d2-4c9f-98ff-42a8732ed677"}
