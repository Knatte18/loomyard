# Plan split tune: round 1 changes and their replay

2026-11-02, hub `loomyard-LYMHUB`, tasks `plan-split-tune` and `plan-split-replay`.
The report records the round-1 changes to the plan-split writers and why each was made, then the replay of those changes on the installed `lym`.
The replay figures record one run per input on one machine and are not claiming to stay true.

## Question

Can the instructions and gates of the plan-split writers (Parts-Write, the part writers and the joining chair) be tuned so a split plan joins and passes its gates with no hand edit of `parts.md`?
The starting point is the defects `plan-split-bench-2026-11-01/README.md` lists.

## Why the loop stopped after the changes

Stencils are read only from the hub-wide Bolt copy, and the production install takes only `main`.
So no replay can run this branch's `lym` and stencils before the branch lands.
The parent recorded the split in the decision record: this run lands round 1's changes with no replay and no scratch worktree, and `plan-split-replay` runs the replays on the installed `lym` afterwards, tuning further within the cap left over.

## Defects and the change for each

| bench defect | cause | round-1 change |
|---|---|---|
| `wv` hand edit 1: `heddlecli#NewCommand` exported where `PATTERN-cli-cobra` requires `Command()` | Parts-Write had the PATTERN index in its prompt, but no step had it check an export's name against the entries for that package | A new Parts-Write step: before writing exports, read the PATTERN entries that govern each export's package or kind of name, read their background files, and spell the export as they require |
| `wv` hand edit 2: `WarpSeam`, `SeedWarpExclude`, `InstallWarpSeam` exported, refused later by `validate-plan --part` as `fabric-vocabulary-name` | The parts gate ran no name check, so the defect surfaced only at the part gate, where the writer cannot change `parts.md` | The parts gate runs `fabric-vocabulary-name` and `handle-malformed` on every `plan:` export, as a part's card declaring it would face them |
| `rv` part gate: `part-export-missing` on `landingshed#DescribeTable`, an export whose signature the part keeps, through three attempts | `Exports` admitted existing members ("each existing member they re-sign or delete"), and nothing at Parts-Write could tell a kept member from a re-signed one | An export is only what the cards create, a `plan:` handle or a new file path; the parts gate refuses `export-existing` for any other ref that names a member or a path the worktree holds, and `ExportMet` counts only a Create target or Rename destination |
| `wv` review F3: a side token in a heddle signature | a card-level naming defect no gate refuses | No change: the name checks above apply to declared names, not signature bodies |
| `rv` chair: wait failed on "reed did not track strand" | reed strand loss, not a writer result | No change: out of this task's scope |

## Design notes

- An existing member a part re-signs or deletes is now named in its `Delivers`, not its `Exports`.
  A later part uses it by the spelling `lym quarry` gives it, which the part check already resolves from the code.
  The cost is that a contract no longer pins a re-sign as a checked export; the joined plan's gate and Plan-Review still see the cards.
- The export name checks judge a `plan:` handle under the plan format's default language, which a part's plan keeps unless it names another, since a parts file carries no language.
- No gate check enforces `PATTERN-cli-cobra`'s constructor name; that defect rests on the new Parts-Write step alone.
  The replays are what will show whether the step is enough.

## Replay

`plan-split-replay` ran the loop on the installed `lym`, built after the round-1 changes landed on `main`.
Files: `plan-split-replay-2026-11-02/` holds each split plan's `plandag` report and its Plan-Review round.

### Method

- Inputs: the bench's `warp-visibility` and `run-verbs-shared` records.
  The 2026-11-02 author rewrite changed every commit SHA, so the bench's SHAs no longer resolve; the orch mapped each by date to its rewritten equivalent.

  | input | bench code | rewritten code | bench record weft | rewritten record weft |
  |---|---|---|---|---|
  | `warp-visibility` | `d407ef69` | `f91866f5` | `576c4b3a31` | `62083317f4` |
  | `run-verbs-shared` | `52b69848` | `273ea6ca` | `b8a86e5c59` | `4e6d7e3783` |

- Two scratch worktrees made by the orch with `lym fabric add --local`: `psr-wv` and `psr-rv`, each with `plan_split: true` in its own `_lym/config/tapestry.yaml`, `_lym/discussion` and `_lym/reviews/discussion` taken from the record commit.
- As in the bench, each run was seeded with `lym shed seed self --recipe tapestry --driver go --param parent=main`, stepped through Preflight, and its `status.json` edited onto `Parts-Write` with Discussion-Write and Discussion-Bouncer recorded done.
  `lym shed pause --after Parts-Plan-Write` stopped each run after the join; a second pause after `Plan-Burler-Review` and `lym loom resume` ran the one Plan-Review round, which stopped at the fixer.
- The hub's config of the day set models, efforts and fans; the two runs ran at once, so wall times include contention.
- Cost and time: `go run ./tools/tokencount -hub ~/Code/loomyard-LYMHUB -detail psr-wv psr-rv`.

### Round 1

| input | joined | gates that refused | hand edits | writing cost | writing wall | cards |
|---|---|---|---|---|---|---|
| `warp-visibility` | yes | `plan-part` once, on the `entry-merge` part: `handle-dangling` for `plan:internal/boardengine#FabricReplay.Replay`; the part writer fixed it on the gate's re-prompt | none | $18.69 | 24m08s | 36 |
| `run-verbs-shared` | yes | none | none | $29.36 | 29m07s | 37 |

Writing cost and wall cover Parts-Write, the part writers and the chair.
The parts gate refused nothing on either input, and `join-parts` joined both without a hand edit, so the loop ended after round 1 of the cap of 2.
The `status.json` edits and pauses are the bench's harness steps, not repairs of a writer's output.
The `loom-crash-resume.md` friction note in `psr-wv` comes from the `status.json` edit made while the run was parked at `Parts-Write`.

### Plan-Review, one round on round 1's plans

| input | cost | findings | blocking | medium | low |
|---|---|---|---|---|---|
| `warp-visibility` | $7.92 | 8 | 0 | 5 | 3 |
| `run-verbs-shared` | $7.38 | 12 | 0 | 4 | 8 |

The cost covers the reviewer, its forks and the seed, to the round's end; the fixer did not run.

### Card overlap

| input | cards | direct dependencies | critical path | widest level |
|---|---|---|---|---|
| `warp-visibility` | 36 | 60 | 8 | 15 |
| `run-verbs-shared` | 37 | 119 | 20 | 7 |

### Reading

- Both defects the bench's hand edits repaired did not recur: no `heddlecli#NewCommand`-style export, no vocabulary-barred export name, and no `part-export-missing` through three attempts on `run-verbs-shared`, which now joined.
  The new Parts-Write step and the parts gate's checks are consistent with that, on one run per input.
- The bench's `warp-visibility` split cost $16.69 to write with 34 cards and 80 dependencies; this run cost $18.69 with 36 cards and 60 dependencies.
  The bench's `run-verbs-shared` plan arm cost $28.03 for 19 cards; the split arm now cost $29.36 for 37.
- Plan-Review found no blocking finding on either plan, against 3 to 4 on the bench's `warp-visibility` arms and 2 on its `run-verbs-shared` plan arm; the reviewers read different plans, so the counts are not paired.
- No later round was needed, so nothing waits on a tuning that has not landed, and no next replay needs a check.

### Deviations

- Drift-Baseline was not run, as in the bench.
- One run per input, no repeat: a direction, not an effect.
- The Plan-Review fixer was not run, so the review findings are unresolved.
