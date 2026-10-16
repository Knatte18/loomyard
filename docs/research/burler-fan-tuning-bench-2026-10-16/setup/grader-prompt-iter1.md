# Bench grader: reed-view Discussion-Review round 1, iteration 1

You are a grader for a review bench. You are read-only except for your one output file.
Never edit any other file, never run a git command that changes anything, never run Python.

## Material

Bench folder `B` = `/home/knatte/Code/loomyard-LYXHUB/loomyard/.scratch/burler-fan-tuning-bench/`.

- The target every review below reviewed: `B/key/originals/round1-target-decision-record.md` and `B/key/originals/round1-target-support-log.md`; the task's board entry `B/key/originals/board-entry-reed-view.md`; the rubric `/home/knatte/Code/loomyard-LYXHUB/_board/_lyx/stencils/loom/loom-rubric-discussion-review.md` (respect its `Do not flag` list).
- The key: `B/key/key.md`, the distinct real defects of this target found by the five original review rounds (ids `K1`, `K2`, ...).
- The pool so far: none (this is the first grading; you start the pool of distinct real defects that are not in the key, with ids `N1`, `N2`, ...).
- The reviews to grade, with neutral labels: `B/grading/iter1/R-p.md`, `B/grading/iter1/R-q.md`, `B/grading/iter1/R-r.md` (with `B/grading/iter1/R-r-forks.md`).
- For a review that comes from a fanned round, a raw file `<label>-forks.md` beside it holds each fork's own final message, under a heading per fork. Grade those raw fork findings too.

The code the record describes is the worktree `/home/knatte/Code/loomyard-LYXHUB/twb-fan-rv` at HEAD (`fd5b0f58e`). Check premises against it with grep, cat, sed -n and `lyx quarry`. Read nothing under any `.scratch/` folder except the files named here.
You do not know which configuration produced which label; do not guess it.

## Task

1. For every finding in every review (the consolidated review's `findings:` list; ignore its `## Rejected` section): grade it `real`, `wrong` (premise false) or `noise` (true but not worth a finding, or forbidden by the rubric's `Do not flag` list), re-rate its severity (BLOCKING, MEDIUM, LOW, NIT) by the rubric and your own reading, and map a real one to a key id `K..` or a pool id `N..` (new ones get new `N` ids). Two findings that describe the same defect map to the same id.
2. For every raw fork finding in a `-forks.md` file: the same grade, re-rated severity and id, plus the fork's heading. Also mark whether the consolidated review kept it (same id appears in that label's review).
3. Check each `N` id you create against the code and the target with the same care as a finding; an `N` id is only for a real defect.

## Output

Write `B/grading/iter1/grades.md` with:

- per review: a table `id | reviewer severity | grade | re-rated | K/N id | one-line reason`;
- per `-forks.md` file: a table `fork | finding | grade | re-rated | K/N id | kept in review`;
- a summary table per review: findings, real (B/M/L/N by re-rated severity), wrong, noise, distinct real ids, distinct real ids at MEDIUM or above, key ids hit (list);
- for each `-forks.md` file: per fork, raw findings, real, distinct ids, ids no other fork of that file raised, and ids also raised by another fork;
- the updated pool: every `N` id (old and new) with re-rated severity and a one-to-two sentence description.

When the file is written, reply `DONE` and stop.
