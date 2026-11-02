# Plan split tune: round 1 changes, replays pending

2026-11-02, hub `loomyard-LYMHUB`, task `plan-split-tune`.
The report records the round-1 changes to the plan-split writers and why each was made.
It holds no replay results: those are pending the follow-up task `plan-split-replay`.

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

## Pending: replays

`plan-split-replay` runs the loop this task planned, on the installed `lym` once this lands:

- Inputs: `warp-visibility` (code `d407ef69`, record weft `576c4b3a31`) and `run-verbs-shared` (code `52b69848`, record weft `b8a86e5c59`), as in the bench.
- One round: replay Parts-Write and Parts-Plan-Write on both inputs with `plan_split` on, and record per input whether the parts joined, which gates refused, and every hand edit needed.
- At most 3 rounds, so at most 6 split writings, and one Plan-Review round per input on the last round's plans only.
- The loop ends early at the first round where both inputs join and pass their gates with no hand edit.

| round | input | joined | gates that refused | hand edits | cost |
|---|---|---|---|---|---|
| 1 | `warp-visibility` | pending | pending | pending | pending |
| 1 | `run-verbs-shared` | pending | pending | pending | pending |
