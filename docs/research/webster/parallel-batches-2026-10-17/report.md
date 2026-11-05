# Parallel batches in real loom plans, 2026-10-17

How often could cards, and the batches webster ran, have run in parallel worktrees?
This is the card-DAG width analysis the board note `weaver` names as its decision gate, run on real plans.
A Sonnet agent measured it read-only; this file holds its returned text, placed by the orch.

## Sample and method

- 34 runs (238 cards, 106 batches): the newest final `_lyx/webster/state.json` per run GUID in the records repo, 2026-10-15 to 2026-10-17. `creel-core` and `creel-stage1-fixes` appear twice, re-planned and re-run.
- Card graph: each card's `Create`/`Edit`/`Delete`/`Rename` lists are its targets, `Uses:` its reads. A card depends on an earlier one when their targets overlap or its `Uses:` hits the earlier card's targets.
- Three levels: file-only (shared explicit paths; weak, since most targets are `pkg#Symbol` glyphs with no file), symbol (exact or `Type.Method` overlap, the plan spec's independence rule; the main number), package (symbol plus same package, a conservative merge-risk bound).
- Critical path (cp) is the longest dependency chain; width (w) is the largest set of cards at one chain depth, a lower bound on the true maximum.
- Batches are the `partition` in the final `state.json`; actual changes are `git diff startSha..head_sha` per batch in the task repo (all 106 resolvable).

## Per run

"disjoint" is consecutive batch pairs with no symbol-level dependency.

| slug | cards | batches | cp-sym | cp-pkg | w-sym | w-pkg | disjoint |
|---|---|---|---|---|---|---|---|
| orch-context-overview | 2 | 1 | 2 | 2 | 1 | 1 | - |
| glyph-audit-tool | 6 | 1 | 6 | 6 | 1 | 1 | - |
| role-language-skills | 9 | 2 | 5 | 5 | 3 | 3 | 0/1 |
| kickstart-per-card | 4 | 1 | 2 | 3 | 3 | 2 | - |
| viewer-off-shell | 6 | 1 | 5 | 5 | 2 | 2 | - |
| webster-master-no-code | 4 | 1 | 4 | 4 | 1 | 1 | - |
| report-timing | 12 | 3 | 9 | 9 | 4 | 3 | 0/2 |
| sibling-overlap-check | 2 | 1 | 2 | 2 | 1 | 1 | - |
| plan-test-file-path | 3 | 1 | 2 | 2 | 2 | 2 | - |
| creel-core (1st run) | 16 | 3 | 10 | 10 | 6 | 6 | 0/2 |
| engine-registry | 14 | 4 | 11 | 13 | 2 | 2 | 0/3 |
| creel-core (2nd run) | 1 | 1 | 1 | 1 | 1 | 1 | - |
| lighter-test-rules | 8 | 3 | 2 | 2 | 7 | 7 | 1/2 |
| discussion-relevant-list | 7 | 2 | 3 | 4 | 3 | 3 | 0/1 |
| engine-config-seams | 11 | 9 | 8 | 9 | 3 | 2 | 1/8 |
| engine-model-choice | 14 | 13 | 8 | 10 | 5 | 3 | 6/12 |
| card-own-entry-edit | 4 | 1 | 1 | 3 | 4 | 2 | - |
| engine-usage-readers | 6 | 5 | 4 | 4 | 2 | 2 | 0/4 |
| webster-deviations-list | 4 | 1 | 2 | 2 | 3 | 3 | - |
| orch-continue | 4 | 1 | 2 | 3 | 3 | 2 | - |
| creel-stage1-fixes (1st run) | 14 | 7 | 9 | 10 | 2 | 2 | 1/6 |
| shuttle-open-permission | 3 | 1 | 1 | 1 | 3 | 3 | - |
| orch-intake-fork | 3 | 1 | 2 | 2 | 2 | 2 | - |
| creel-sandbox-suite | 3 | 1 | 1 | 2 | 3 | 2 | - |
| orch-role-stencil-fixes | 4 | 1 | 2 | 2 | 3 | 3 | - |
| loom-stencil-wording | 7 | 4 | 4 | 5 | 3 | 2 | 2/3 |
| reed-add-parentless-placement | 8 | 6 | 4 | 7 | 4 | 2 | 2/5 |
| creel-stage1-fixes (2nd run) | 11 | 3 | 8 | 8 | 2 | 2 | 0/2 |
| reed-down-and-pane-path | 6 | 2 | 5 | 5 | 2 | 2 | 0/1 |
| webster-fork-stencil-fixes | 17 | 12 | 9 | 12 | 4 | 3 | 2/11 |
| fan-prompt-tuning | 8 | 4 | 5 | 6 | 3 | 2 | 0/3 |
| pr-gate-run-identity | 6 | 2 | 3 | 5 | 2 | 2 | 0/1 |
| creel-cli-output-fixes | 3 | 1 | 3 | 3 | 1 | 1 | - |
| shedrecipe-capabilities | 8 | 6 | 5 | 6 | 3 | 2 | 2/5 |

## Medians

- Median run: 6 cards, 2 batches; critical path 4 (symbol) and 4.5 (package).
- Best speedup with unlimited worktrees (cards / critical path): 1.6x symbol-level, 1.33x package-level; median width 3 and 2.
- 28 of 34 runs have a critical path shorter than their card count; the other 6 are strict chains or single cards.
- Batch level (18 multi-batch runs, 90 batches): median speedup about 1.0 to 1.1x, best 1.6x; only 8 of the 18 runs have two batches that could run side by side.
- Consecutive batch pairs: 17 of 72 (24%) dependency-free at symbol level, 12 (17%) at package level.

## Plan against reality

- 83 of 106 batches (78%) changed at least one file outside their declared targets: 299 of 1,883 changed files (16%). Glyph targets name no file and the count includes test files, so this overstates edits no card foresaw.
- Of 116 symbol-disjoint batch pairs, 16 (14%) touched a common file; of the 17 consecutive disjoint pairs, 3 (18%).
- Files hit by more than one batch: `cmd/lyx/testdata/test-budget.yaml` (9 runs, 26 batch touches), `internal/loomcli/wiring.go` and `wiring_test.go`, `contracts/specs/refusal-spec.md`, `internal/webstercli/wiring.go`, several `doc.go`.

## Serializing files declared by cards

| File | Runs where 2+ cards edit it | Card touches |
|---|---|---|
| `cmd/lyx/testdata/test-budget.yaml` | 11 | 40 |
| `contracts/specs/refusal-spec.md` | 4 | 15 |
| `internal/websterengine/doc.go` | 4 | 13 |
| `internal/shuttleengine/doc.go` | 4 | 13 |
| `internal/reedengine/doc.go` | 3 | 9 |
| `internal/shuttleengine/claudeengine/doc.go` | 3 | 8 |
| `internal/creelcli/doc.go` | 3 | 7 |
| `internal/creelengine/doc.go` | 3 | 14 |
| `docs/overview.md` | 3 | 7 |

156 of 238 cards (65%) list at least one of `doc.go`, `PATTERN.md`, `test-budget.yaml` or `docs/overview.md`.
Dropping those from the graph lifts the median package-level speedup from 1.33x to about 1.5x.

`test-budget.yaml` is one line per package, so two cards raising different packages' rows merge cleanly in git unless the rows are adjacent or both insert at one spot; it is a false dependency for scheduling, not a real one.

## Verdict

1. Most plans are mostly sequential: about 1.6x at best with unlimited worktrees (1.3x package-level), 2 to 3 cards runnable together; many plans are chains because later cards use symbols earlier cards create.
2. Webster's batches expose almost none of it (about 1.0 to 1.1x), since batching by package already groups dependent cards.
3. Pairs disjoint on paper still collide on unlisted files 14 to 18% of the time, so parallel worktrees need merge-conflict handling, not only a post-merge build.
4. A parallelism-aiming batchifier gains most by treating shared docs and `test-budget.yaml` as non-blocking or moving them to a final card, and by honest per-card file lists: roughly 1.3x to 1.6x on 8 to 17-card plans, nothing on the about 40% of plans with 4 or fewer cards.

## Not measured

- The file of each glyph, so file-level overlap is approximate; the package-level figures are the conservative bound.
- Edits per card (only per batch).
- Exact maximum width, whether batches could have been split, and whether a parallel schedule would have stayed green.
- Wall-clock savings.
- Runs before 2026-10-15.
