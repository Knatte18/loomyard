# Grader brief: Burler twill-fixer bench

You grade one Burler fix round per run, in pairs of arms on the same reference: `s` (today's fixer) and `t` (the same fixer, which after fixing spawned three read-only forks of itself, `consistency`, `claims` and `way-forward`, and folded their findings in before writing its fixer-report).
The method and scale are those of `docs/research/twill-fixer-bench.md` and `docs/research/twill-discussion-fork0-bench.md`; read their method sections first.
Grade both arms of a reference in one pass with the same reading, so they compare.

## Material per run (`runs/<pair>/`)

- `reviews/round-N-review.md`: the review the fixer acted on (identical in both arms; byte-pinned by its acceptance digest).
- `reviews/round-N-fixer-report.md`: the fixer's report; the `t` arm's has a `## Self-check` section.
- `forks/*.md`: the `t` arm's three fork reports.
- `artifact/`: the record or plan after the fix; `fix.diff`: the artifact against its pre-fix state (`keys/<ref>/before/`).
- `reviews/round-(N+1)-review.md`: a fresh reviewer's next round over the fixed artifact (opus[medium], today's stencils).
- `keys/<ref>/reviews/`: the reference's own round-N fixer report and its later rounds, where the fix-introduced finding the reference was picked for appears; `keys/<ref>/after-ref/`: the reference's own post-fix artifact.
- Code: read it at the run's base, in the prime with `git -C /home/knatte/Code/loomyard-LYXHUB/loomyard show <base>:<path>`; never read later code.

## What to grade

1. Every round-N finding's disposition: right or wrong.
   A real finding not fixed, or fixed so the premise still holds, is wrong; a wrong finding applied is wrong; a dispute or own change is right only if its evidence holds.
2. `t` only: every fork finding: real, wrong or noise, with your re-rated severity (B/M/L), and the fixer's disposition of it, right or wrong.
3. Every finding of the round-(N+1) review: real, wrong or noise, re-rated severity, and its class:
   - `A`: the round-N fix brought it in (the defective text is in `fix.diff`'s added or changed lines, or the fix left a sibling contradicting what it changed);
   - `B`: it was there before the fix (present in `keys/<ref>/before/`);
   - `setup`: it comes from the bench's code base, not from the artifact's own content (for example a commit the reference's branch had and the bench base lacks).
   For every class-A finding also mark its cause:
   - `proposal`: the reviewer's proposed change was itself flawed and the fixer applied it as proposed;
   - `execution`: the proposal was sound, and the fixer's own way of carrying it out, or the interplay of several fixes, brought the defect in.
4. Your own audit of `fix.diff`: every defect the fix brought in that the round-(N+1) review did not report (class A, with severity and cause).
5. The key: the reference's own fix-introduced finding(s) (named below); per run, whether the fixed artifact repeats it (R), partly (P) or avoids it (A), and if A, whether a fork finding got it there ("A (fork)").

## Rules

- Real against noise is a judgement call: state the rule you adopt once and apply it to every run.
- Duplicates are merged only for the distinct-real count.
- Accepting noise is right unless it made the artifact worse.

## Output

Write one file, `finding-grades-<perch>.md`, in the bench directory, with one section per run: the tables for items 1-4 (one row per finding: source, severity, short title, grade, re-rated severity, class, cause, disposition, right?), the key line, and a per-run tally line: round-N wrong dispositions; fork findings real/wrong/noise; fork dispositions wrong; round-(N+1) findings real/wrong/noise; class A (proposal/execution) by the review and by your audit; class B; setup.
Then end with the one-line rules you adopted.
Write nothing else and change no file outside the bench directory.
