# Key grader: reed-view Discussion-Review, all five original rounds

You are a grader for a review bench. You are read-only except for one output file.
Never edit any file other than the output file, never run git commands that change anything, never run Python.

## Material

All under `/home/knatte/Code/loomyard-LYXHUB/loomyard/.scratch/burler-fan-tuning-bench/key/originals/`:

- `round1-target-decision-record.md`, `round1-target-support-log.md`: the discussion artifact pair exactly as round 1 reviewed it.
- `round-N-review.md`, `round-N-fixer-report.md`, `round-N-focus.md` for N = 1..5: the five original review rounds of this task, the fixer's report after each, and each round's focus file.
- `final-decision-record.md`: the record after round 5's fix.
- `board-entry-reed-view.md`: the task's board entry at round 1.
- The rubric the reviewers applied: `/home/knatte/Code/loomyard-LYXHUB/_board/_lyx/stencils/loom/loom-rubric-discussion-review.md` (respect its `Do not flag` list).

The code the record describes is the worktree `/home/knatte/Code/loomyard-LYXHUB/twb-fan-rv` at its HEAD (`fd5b0f58e`, the code at the time of round 1). Check premises against it with grep, cat, sed -n and `lyx quarry`. Do not read anything under any `.scratch/` directory other than the originals folder above and your output file.

## Task

1. Go through every finding of rounds 1-5 (39 findings). For each decide:
   - grade: `real` (a true defect of the record), `wrong` (its premise is false), or `noise` (true but not a defect worth a finding, or forbidden by the rubric's `Do not flag` list);
   - re-rated severity (BLOCKING, MEDIUM, LOW, NIT) by the rubric's standard and your own reading, not the reviewer's label;
   - `in-r1`: whether the defect it names was already present in the round-1 target (`yes`), was introduced by a later fixer (`no`, say which round's fix), or `partial`.
2. Group the findings into distinct defects: two findings that describe the same defect (also across rounds) are one defect. Give each distinct defect an id `K1`, `K2`, ... ordered by re-rated severity, with a one-to-two sentence description precise enough that another grader can decide whether a new review's finding names the same defect, plus the location in the round-1 target (decision id and line) when `in-r1` is yes or partial.
3. The key is the set of distinct defects that are `real` and `in-r1` yes or partial. Mark for each key item the round that first raised it, so late-found items (first raised in round 3 or later) are visible.

## Output

Write `/home/knatte/Code/loomyard-LYXHUB/loomyard/.scratch/burler-fan-tuning-bench/key/key.md` with:

- a per-finding table: `round | id | reviewer severity | grade | re-rated severity | in-r1 | defect id | one-line reason`;
- a key table: `K id | re-rated severity | first round | decision / line in round-1 target | description`;
- a short list of defects that are real but not in-r1 (fix-introduced), which are not part of the key;
- totals.

When the file is written, reply `DONE` and stop.
