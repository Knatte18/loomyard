# Plan split bench: Parts-Write and Parts-Plan-Write against Plan-Write

2026-11-01, hub `loomyard-LYMHUB`, `lym` built from `a99b3b4d`.
The figures record one run of each arm on one machine and are not claiming to stay true.
The report recommends and decides nothing.

## Question

`tapestry.plan_split: true` replaces `Plan-Write` with `Parts-Write` and `Parts-Plan-Write`.
Does the split write a plan at lower cost, in less time or with fewer defects than `Plan-Write` alone?

## Method

- Inputs: the approved decision records and discussion files of two landed tapestry runs, taken from the weft archive tags `archive/warp-visibility/9d6f960b829b` and `archive/run-verbs-shared/100f752e28ed`.
  The records are the weft commits right before each run's Plan-Write: `576c4b3a31` and `b8a86e5c59`.
- Code bases: `main` when each run started, `d407ef69` for `warp-visibility` and `52b69848` for `run-verbs-shared`.
- Four scratch worktrees made by the orch with `lym fabric add --local`: `psb-wv-plan`, `psb-wv-split`, `psb-rv-plan` and `psb-rv-split` (`wv` is `warp-visibility`, `rv` is `run-verbs-shared`).
  Each code branch was reset to its base, and `_lym/discussion` and `_lym/reviews/discussion` were checked out of the record commit.
  The two split worktrees carry `plan_split: true` in their own `_lym/config/tapestry.yaml`; the hub-wide layer is untouched.
- Each run was seeded with `lym shed seed self --recipe tapestry`, stepped through Preflight, then `status.json` was edited onto `Plan-Write` (plan arms) or `Parts-Write` (split arms) with Discussion-Write and Discussion-Bouncer recorded done.
  `lym shed pause --after Plan-Burler-Review` stopped each run after one Plan-Review round, before the fixer.
  `lym loom step --until-stop` drove the rest.
- Models, efforts and fans are the hub's config of the day: `opus[medium]` for the plan, parts, part-writer and chair producers, `opus[medium]` for the first Plan-Review round with the `review-plan` fan, `twill-plan` lenses on the Plan-Write lead.
- All four runs ran at once, from 18:18 to 19:03 local, so wall times include contention.
- Cost and time: `go run ./tools/tokencount -hub ~/Code/loomyard-LYMHUB -detail <four slugs>` (`tokens.md`).
  Turns and context per session: `session-context.sh` (`session-context.tsv`).
  Card overlap: `go run ./tools/plandag` per plan (`plandag-*.md`).
  Review findings: each run's `round-1-review.md`.

## Results

### Writing the plan

| run | writing sessions | cost | wall | joined plan passes `plan` gate first time | cards |
|---|---|---|---|---|---|
| `wv` plan | writer and 6 lens forks | $14.93 | 25m07s | yes | 15 |
| `wv` split | Parts-Write, 5 part writers, chair | $16.69 | 15m39s | yes, after 2 hand edits to `parts.md` | 34 |
| `rv` plan | writer and 6 lens forks | $28.03 | 31m59s | yes | 19 |
| `rv` split | Parts-Write, 6 part writers, chair | $12.52 | not taken | no plan joined: infrastructure failure | none |

The landed plans of both inputs had 20 cards.
Wall is the history stamp from the end of the previous row to the end of the writing row; the `wv` split figure includes the waits for the two hand edits.
The `rv` split cost is partial: the chair's wait failed on "reed did not track strand" with the part writers still running, the run state is `failed`, and the orch recorded the arm as stopped.
The failure is reed strand loss, not a Parts-Plan-Write result, and nobody touched reed for that session.

### Turns and context per session

Turns and context are from `session-context.tsv`; context is input plus cache write plus cache read of a message.

| run | session | turns | mean context | peak context |
|---|---|---|---|---|
| `wv` plan | writer | 48 | 261k | 408k |
| `wv` plan | 6 writer forks | 3 to 39 | 338k to 393k | 341k to 445k |
| `wv` split | Parts-Write | 14 | 51k | 71k |
| `wv` split | part writers, 5 | 15 to 37 | 104k to 169k | 149k to 245k |
| `wv` split | chair | 61 | 102k | 204k |
| `rv` plan | writer | 109 | 452k | 674k |
| `rv` plan | 6 writer forks | 2 to 37 | 572k to 591k | 573k to 637k |
| `rv` split | Parts-Write | 16 | 58k | 77k |
| `rv` split | part writers, 6 | 17 to 24 | 100k to 169k | 128k to 267k |
| `rv` split | chair (ended before the join) | 21 | 64k | 81k |

### Plan-Review, one round

| run | cost | findings | blocking | medium | low | nit |
|---|---|---|---|---|---|---|
| `wv` plan | $4.96 | 13 | 3 | 5 | 5 | 0 |
| `wv` split | $5.74 | 14 | 4 | 6 | 4 | 0 |
| `rv` plan | $6.06 | 16 | 2 | 5 | 6 | 3 |

The reviewers read different plans, so the counts are not paired findings.

### Card overlap

`plandag` counts a dependency when two cards change an overlapping ref.

| run | cards | direct dependencies | critical path | widest level |
|---|---|---|---|---|
| `wv` plan | 15 | 29 | 7 | 4 |
| `wv` split | 34 | 80 | 10 | 10 |
| `rv` plan | 19 | 75 | 13 | 5 |

## Defects the split arm showed

- `wv` split, hand edit 1: `parts.md` exported `heddlecli#NewCommand` where `PATTERN-cli-cobra` requires `Command()`, and `join-parts` would have refused it.
- `wv` split, hand edit 2: `parts.md` spelled `WarpSeam`, `SeedWarpExclude` and `InstallWarpSeam` in exports, which `PATTERN-fabric-vocabulary` bars outside the owner set; `validate-plan --part heddle` refused them.
- `wv` split review: F3 blocking, a side token (`warpRoot`) in a heddle signature, the same vocabulary rule surviving into a card.
- `rv` split: the part gate `part-export-missing` blocked the `landing-describe` part for an export that keeps its signature (`landingshed#DescribeTable`), through all three attempts; the parent chose a same-signature re-sign arrow noted in `join.md`, recorded in the decision record, and counted as one hand intervention.
- Both hand edits were made only because a gate would otherwise have refused the run; the quality defect no gate refused was left in place.

## Reading

- One run per arm and input, no repeat: a direction, not an effect.
- On `warp-visibility` the split cost 12% more to write ($16.69 against $14.93) and 38% less wall time (15m39s against 25m07s).
  The writing sessions were smaller: the largest split session peaked at 245k, the plan writer at 408k and its forks at 445k.
- The split plan had 34 cards against 15, with 80 dependencies against 29; Plan-Review found a similar number of findings, with one more blocking.
- On `run-verbs-shared` the plan arm cost $28.03 and the writer peaked at 674k context; the split arm did not produce a plan, so nothing compares.
- The split gate passed first time only after hand edits to a parts file whose contracts broke two PATTERN rules; `Parts-Write` is the source of those defects.

## Recommendation

- Keep `plan_split` off.
  The evidence is one comparison, with a hand-repaired split arm.
- Before a repeat, make the parts check refuse an export that breaks `PATTERN-cli-cobra` or `PATTERN-fabric-vocabulary`, and let a part export that keeps its signature pass without a re-sign arrow.
- A repeat of `rv` split, outside this study's cap, needs the operator's approval; its reed failure says nothing about the method.
- Wall time and per-session context are where the split looks better, and cost, card count and dependency count are where it looks worse.

## Deviations

- Drift-Baseline was not run (an extra agent session beyond the cap); the runs moved from Preflight straight to `Plan-Write` or `Parts-Write`.
- The status edits and pause are by hand, as in earlier plan benches.
- The 4 runs share one machine and one clock window.
- Hand interventions are listed above; the status edits are common to every arm.

## Files

- `report.md`: this report.
- `session-context.sh`, `session-context.tsv`: turns and context per session.
- `tokens.md`: the `tokencount` report for the four runs.
- `plandag-wv-plan.md`, `plandag-wv-split.md`, `plandag-rv-plan.md`: card dependency reports.
- `round-1-review-wv-plan.md`, `round-1-review-wv-split.md`, `round-1-review-rv-plan.md`: the Plan-Review round of each plan.
