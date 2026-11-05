# Plan and Discussion fixers: Sonnet against Opus

2026-11-03, board task `fixers-on-sonnet`.
Eight fixer runs on two kept review rounds, `opus[medium]` against `sonnet[medium]`, two repeats per arm and input, each fixed artifact scored by one solo reviewer.

## Conclusion

- **`plan_fixer_model`: set `sonnet[medium]`.**
  All four Plan runs fixed all 9 findings, and the fixed plans carried 3 new defects under Opus and 2 under Sonnet, none above LOW.
- **`discussion_fixer_model`: keep it empty, falling back to `fixer_model` (`opus[medium]`), until a larger bench.**
  Opus fixed 28 of 30 findings and left 2 partial, Sonnet fixed 25 and left 5 partial, with 7 and 6 new defects.
  The gap is three findings on two runs per arm, inside the spread within an arm, so this is a lean and not a result.
  The three partials Sonnet has beyond Opus's are on F10, which asks for research and cited evidence, and F8, a lease rule that fails in one case.
- Sonnet ran at a lower token count on Discussion (mean 88k against 145k) and about level on Plan (mean 75k against 72k).
  The session had no prices, so no dollar cost is recorded.
  `docs/research/tokens/token-audit-2026-10-22` estimates that a swap saves about half a role's non-cache-read cost.

## Method

- **Arms:** `opus[medium]` and `sonnet[medium]` fixers on the findings as written.
  The entry's third arm, Sonnet on findings "in the concrete form", was dropped by the parent's recorded decision: `docs/research/burler/concrete-review-findings-2026-11-03/report.md` found every finding already names its place and change, so the arm has no input distinct from the second.
- **Inputs:** the two rounds that report keeps in `data/fixers-input.tsv`.
  - Plan: weft `bb7bba83fe`, code `c7cde044af`, 9 findings (1 `BLOCKING`, 2 `MEDIUM`, 5 `LOW`, 1 `NIT`).
  - Discussion: weft `4e79893bf6`, code `b194fbc4c2`, 15 findings, the round's F3 left out because it offers two alternatives.
  - Each run got the artifact as it stood at the weft commit (without the `lens-review.*` and `support-log.lens-*` files) and the round's review file.
- **Fixer:** an in-session subagent following `burler-template-fixer.md` and `burler-step-fix.md`, with its write surface set to a scratch copy of the artifact and its report.
  It read the code only through `git show` and `git grep` at the round's code commit, and ran no `lym` command.
- **Scorer:** one solo reviewer per fixed artifact, `opus[medium]`, the same prompt for every run.
  It judged each original finding `fixed`, `partial` or `open` against the code and the plan or discussion rubric, and listed the defects the fixer's edits introduced.
  It saw the original artifact for the diff and read the fixer's report only after its verdicts.
- **Cost:** the subagent's reported tokens, tool uses and wall time.

## Results

| Run | Model | Findings | Fixed | Partial | Open | New defects | Fixer tokens | Fixer time | Reviewer tokens |
|---|---|---|---|---|---|---|---|---|---|
| plan 1 | opus | 9 | 9 | 0 | 0 | 1 (NIT) | 71,768 | 87 s | 72,461 |
| plan 2 | opus | 9 | 9 | 0 | 0 | 2 (LOW, NIT) | 71,490 | 77 s | 67,625 |
| plan 1 | sonnet | 9 | 9 | 0 | 0 | 2 (LOW, NIT) | 85,323 | 68 s | 61,397 |
| plan 2 | sonnet | 9 | 9 | 0 | 0 | 0 | 64,149 | 51 s | 75,149 |
| discussion 1 | opus | 15 | 14 | 1 | 0 | 5 (MEDIUM, 2 LOW, 2 NIT) | 169,776 | 452 s | 107,250 |
| discussion 2 | opus | 15 | 14 | 1 | 0 | 2 (LOW, LOW) | 119,313 | 321 s | 92,568 |
| discussion 1 | sonnet | 15 | 13 | 2 | 0 | 4 (3 LOW, NIT) | 88,935 | 158 s | 96,510 |
| discussion 2 | sonnet | 15 | 12 | 3 | 0 | 2 (LOW, NIT) | 87,194 | 166 s | 81,575 |

| Input | Model | Fixed | Partial | Open | New defects | Mean fixer tokens |
|---|---|---|---|---|---|---|
| plan | opus | 18 of 18 | 0 | 0 | 3 | 71,629 |
| plan | sonnet | 18 of 18 | 0 | 0 | 2 | 74,736 |
| discussion | opus | 28 of 30 | 2 | 0 | 7 | 144,545 |
| discussion | sonnet | 25 of 30 | 5 | 0 | 6 | 88,065 |

No run left a finding `open`, and no fixer disputed a finding.
The only deferral is part of F10 in the first Sonnet Discussion run.

## Findings

- **Plan:** the findings were concrete edits, and both models applied them.
  The new defects are stale summary lines and one unnamed lock path, each small.
  One Opus run fixed F2 differently, targeting the whole file instead of a test function, and the scorer judged it fixed.
- **Discussion F2 is partial in all four runs.**
  Each left a way forward that does not recover a landed run whose archive tag cannot be found or chosen; the finding asks the fixer to design that way forward, not to copy a stated edit.
  A stronger fixer did not close it.
- **Where Sonnet fell short:** F10 (research both open risks and cite evidence) was partial in both Sonnet runs and fixed in both Opus runs, and Sonnet's second run was also partial on F8 (a lease that fails when origin is ahead of the local tip).
  Both Sonnet runs said they could not read the hub's run logs, and the scorer found no cited evidence for the git claim in either; the Opus runs' fixes were judged to hold.
- **New defects:** the largest was a `MEDIUM` in Opus discussion 1, where D1's new local-archive rule and a Land way forward contradict each other and leave a dead end.
  The Sonnet runs' new defects were `LOW` or `NIT`, mostly sentences elsewhere in the record that the edit left stale.

## What eight runs support

- **Supported:** on findings that state their edit, as the Plan round's do, Sonnet at medium fixes as many as Opus, with no more new defects.
- **Not shown:** that Sonnet matches Opus on findings that ask for research or design.
  The Discussion gap is three findings, all on F8 and F10 in the Sonnet runs, with two repeats per arm.
- **Not shown:** the dollar cost.
  Tokens are the subagent totals; prices and the cache split were not available.
- **Caveats:**
  - One reviewer scored every run and was not blind to the arm, since the run directories name the model.
  - The reviewer saw the original findings, so it measures "fixed as asked" and may anchor on them.
  - The fixers ran as in-session subagents rather than through the repository's reed launch path, with `lym` commands barred, so no run could run the discussion or plan check the production fixer can.
  - The artifacts were not re-reviewed by the hub's fork-fan reviewer; the parent's decision accepted a solo reviewer.

## Files

- `data/<input>-<model>-<n>/score.md`: the reviewer's verdict table and new-defect list for each run.
- `data/<input>-<model>-<n>/fixer-report.md`: each fixer's own report.
