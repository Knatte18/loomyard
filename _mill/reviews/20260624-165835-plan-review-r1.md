All three of these survive:
- **board/cli_test.go (Card 8):** `filepath` survives via line 164 (`homePath := filepath.Join(cwd, "board", "Home.md")`). Safe.
- **bench_test.go (Card 10):** survives via lines 53, 73, 185 (`boardDir`, tasks.json, facade Path). Safe.
- **main_test.go (Card 11):** the conversions touch lines 47/51/55, 83/87/91, 187/191; but note line 130 `boardPath` in initcli — wait, that's a different file. In main_test.go there's no other `filepath.` usage. Let me check: lines 47,51,55,83,87,91,187,191 are ALL config-path constructions Card 11 converts. After conversion, `path/filepath` would be orphaned in main_test.go too.

Wait, let me re-examine. main_test.go's only filepath uses are those 8 lines, all `_lyx`/config paths the card converts. So `path/filepath` becomes orphaned → **Card 11 has the same bug.** And Card 11 explicitly says "keep `os`/`path/filepath` as used." Let me confirm there's no other filepath usage I missed — the grep returned only those 8 lines, all converted. So yes, Card 11 also orphans filepath.

So the orphaned-filepath BLOCKING bug affects Cards 5, 7, 11, 13 (and possibly more). This is the central finding. Let me confirm there isn't a missed `filepath` usage in each by trusting the greps (they're exhaustive).

Cards 5, 7, 11, 13 all orphan `path/filepath`. This breaks the verify gate of batches 2 (configcli, configsync), 3 is fine, 4 (cmd/lyx, update).

I have enough to write the review. Let me compose the findings. The verdict is REQUEST_CHANGES due to the build-breaking orphaned imports in multiple cards that explicitly instruct to keep `path/filepath`.

MILL_REVIEW_BEGIN
# Review: Fix failing TestRunCLI in internal/worktree — holistic

```yaml
verdict: REQUEST_CHANGES
reviewer_model: opushigh
reviewed_file: plan/
date: 2026-06-24
```

## Findings

### [BLOCKING] Cards 5/7/11/13 orphan path/filepath import
**Location:** Batch 2 Card 5 (configcli_test.go), Card 7 (configsync_test.go); Batch 4 Card 11 (main_test.go), Card 13 (update_test.go)
**Issue:** In each of these files every `filepath.` call is a config-path the card converts to `paths.ConfigDir`/`paths.ConfigFile` (configcli: all 17 sites; configsync: lines 13/19/67/73/113/133; main_test: 47/51/55/83/87/91/187/191; update: 25/30/113/152), leaving `path/filepath` with zero remaining references — an unused import that fails `go build`, breaking the batch-2 and batch-4 `verify:` gate. Cards 7/11/13 even instruct "keep `os`/`path/filepath` as used," which is factually wrong for these files.
**Fix:** Per-card, instruct removal of the now-unused `path/filepath` import (keep `os`); confirm zero residual `filepath.` references before keeping the import.

### [NIT] Batches 2-4 sweep exceeds the cited constraint and the task
**Location:** Overview "Shared Decisions"; Batches 2, 3, 4
**Issue:** The cited Path Invariant is enforced only by `internal/paths/enforcement_test.go`, which bans `os.Getwd`/`--show-toplevel` and explicitly skips `_test.go` files (line 48); it does not flag hardcoded `_lyx` literals, so the "latent migration breakage" the sweep claims to remove is not constraint-enforced and unrelated to "Fix failing TestRunCLI" (delivered entirely by batch 1).
**Fix:** Acceptable as a deliberate consistency sweep, but consider scoping the task to batch 1 or label batches 2-4 as cosmetic so reviewers don't treat them as constraint fixes.

## Verdict

REQUEST_CHANGES
Four cards orphan `path/filepath`, breaking the batch-2 and batch-4 build gate.
MILL_REVIEW_END