The ide menu_test.go uses "worktree" only as the git subcommand (`git worktree add`), not the lyx module — that's fine. board/git.go is covered by Card 2.

I now have all findings confirmed. The core issues:

1. **`internal/configcli/configcli_integration_test.go`** imports `internal/worktree` and calls `worktree.New`/`worktree.Add`/`worktree.Config`/`worktree.AddOptions`. After batch 3 deletes `internal/worktree`, this file won't compile. NOT in plan. The module-wide gate in batch 9 (`go test -tags integration ./...`) would catch it, but it's never fixed — a guaranteed build break.

2. **`internal/configreg/configreg_test.go:13`** asserts `want := []string{"board", "worktree", "weft"}`. After Card 9 renames the module to `warp`, `TestNames` fails. NOT in plan's edits. Batch 3 verify runs `go test ./internal/configreg/` — this WILL fail.

3. **`internal/configcli/configcli_test.go`** uses `"worktree"` as the config module name (editOne, menu "worktree (configured)"). After the module rename these assertions/lookups break.

MILL_REVIEW_BEGIN
# Review: Introduce warp: the host↔weft-coordinated git module — holistic

```yaml
verdict: REQUEST_CHANGES
reviewer_model: opushigh
reviewed_file: plan/
date: 2026-06-25
```

## Findings

### [BLOCKING] configcli integration test imports deleted internal/worktree
**Location:** Batch 3, Card 10 (and batch 9 gate)
**Issue:** `internal/configcli/configcli_integration_test.go` imports `internal/worktree` and calls `worktree.New`/`worktree.Add`/`worktree.Config`/`worktree.AddOptions`; batch 3 deletes that package but this file is in no card's `Edits`, so it cannot compile and `go test -tags integration ./...` (batch 9) breaks.
**Fix:** Add a card (batch 3) editing `configcli_integration_test.go` to import `internal/warp` and use `warp.New`/`warp.AddOptions`; list it in `All Files Touched`.

### [BLOCKING] configreg_test asserts the old "worktree" module name
**Location:** Batch 3, Card 9
**Issue:** Card 9 renames the configreg entry `worktree`→`warp`, but `internal/configreg/configreg_test.go:13` hard-asserts `want := []string{"board","worktree","weft"}`; it is in no card, so batch 3's own `go test ./internal/configreg/` verify fails.
**Fix:** Edit `configreg_test.go` in Card 9 to expect `"warp"`; add it to `Edits` and `All Files Touched`.

### [BLOCKING] configcli tests reference the renamed config module
**Location:** Batch 3, Card 9
**Issue:** `internal/configcli/configcli_test.go` uses the module name `"worktree"` (`editOne(...,"worktree",...)`, menu assertion `"worktree (configured)"`); after the module rename these lookups/assertions break and the file is in no card.
**Fix:** Add an edit updating `configcli_test.go` (and any configcli production lookup of `"worktree"`) to `"warp"`; record in `All Files Touched`.

### [NIT] init dormant-pairing detection mechanism unspecified
**Location:** Batch 4, Card 14
**Issue:** Card 14 says init must report "no weft pairing" when the host worktree has no weft sibling, but does not name the predicate (`weftRepoExists`/sibling stat) used to decide, so the implementer must guess the check.
**Fix:** Name the concrete check (e.g. `weftRepoExists(l)` / `os.Stat(l.WeftWorktree())`) the dormant-pairing branch keys on.

### [NIT] weft/cli.go caller of Status not in Card 20 Edits
**Location:** Batch 6, Card 20
**Issue:** Card 20 drops `hostLink`/`weftLyxDir` params from `weft.Status` and says "adjust the caller", but `internal/weft/cli.go` (the caller at `Status(weftWorktree, l.HostLyxLinkHere(), l.WeftLyxDir(), pathspec)`) is only in `Context`, not `Edits`.
**Fix:** Move `internal/weft/cli.go` into Card 20 `Edits`.

## Verdict

REQUEST_CHANGES
Three out-of-plan files (configcli + configreg tests) break the build under the worktree→warp rename/delete.
MILL_REVIEW_END
