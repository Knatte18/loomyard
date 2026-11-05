# Do Plan and Discussion reviewers already write concrete findings?

2026-11-03, board task `concrete-review-findings`.
The planned bench was eight review runs, the review stencil as it is against the stencil plus a rule that every finding names its place and the change to make.
No review was run: the parent's recorded decision ("Audit before any review run") replaced the bench with an audit of the findings reviewers wrote in the last days, because the stencil already carries the rule.

## Conclusion

- `contracts/stencils/burler/burler-review-format.md` has required, since `add9f93d` (2026-10-11), a non-empty `location` and a `fix` on every finding, `fix` being "the exact change to make, and where, written so a fixer that has not seen your work can apply it from the review alone".
- Of the 374 findings in the 44 Plan and Discussion review files written from 2026-10-31 to 2026-11-03, every one has both.
  4 (1.1 percent) leave a choice or an investigation to the reader instead of stating one change.
  The rest state an edit and its place.
- There is no non-compliance for a stricter rule to fix, so the recommendation is: **do not add the rule to the stencils**.
  The eight-run bench is not run.
- `fixers-on-sonnet`'s third arm (findings "written in the concrete form") has no input distinct from its second: the findings as written today are the concrete form.
  Its entry needs amending before it runs; the set it can use is `data/fixers-input.tsv`.

## Method

- **Population:** every `round-N-review.md` added under `_lym/reviews/plan/` or `_lym/reviews/discussion/` on any ref of the hub's weft repository since 2026-10-31: 22 plan files and 22 discussion files, 148 plan and 226 discussion findings.
  The file list is the `git log --diff-filter=A` over those paths, and `data/parse.awk` turns each file's frontmatter into `data/findings.tsv` (kind, weft commit, file, id, severity, location, fix).
- **Check, mechanical:** a finding counts as concrete when
  - `location` names a file, and
  - the `fix` (or the location) names a file, a backticked symbol or a path, and the `fix` contains an editing verb.
- **Check, by reading:** the 12 findings the mechanical check did not match were read by hand, as were all `fix` keys opening with a decide, settle, either, consider, clarify or ensure word, and every `fix` under 80 characters.
- **Not checked:** whether the named place is the right one or the change correct.
  This audit judges form only, which is what a fixer needs to start.
  Reviews before 2026-10-31 were not audited, so it cannot say why the operator saw otherwise on 2026-10-22.

## Result

| Kind | Findings | `location` names a file | `fix` non-empty | Matched mechanically | Matched on reading (the rest) | Leaves a choice or investigation |
|---|---|---|---|---|---|---|
| plan | 148 | 148 | 148 | 145 | 3 of 3 | 0 |
| discussion | 226 | 226 | 226 | 217 | 9 of 9 | 4 |
| all | 374 | 374 | 374 | 362 | 12 of 12 | 4 |

Mean `fix` length is about 440 characters.
Severity mix: plan 14 `BLOCKING`, 50 `MEDIUM`, 78 `LOW`, 6 `NIT`; discussion 28, 92, 94, 12.

The 4 that leave a choice, all discussion findings:

| Review | Finding | Why it is not one change |
|---|---|---|
| `16cbd75704` round 1 | F2 | "Settle in D9 how …", with options |
| `4e79893bf6` round 1 | F3 | "Either route D4's branch push through the pattern …", two edits to pick from |
| `54b92cea09` | F4 | "Settle the link now" by reading a reflog: an investigation, not an edit |
| `d5f8c2b0cc` round 2 | F5 | "Decide in D12 how …", with an example |

A fixer on a cheaper model would need a decision for these first.

## Inputs kept for `fixers-on-sonnet`

Two rounds, each at the state its review saw, listed in `data/inputs.txt` (weft commit of the round's records, the round's start time, and the last `main` commit before it as the code the reviewer read):

| Input | Weft commit | Code commit | Findings |
|---|---|---|---|
| plan | `bb7bba83fe` | `c7cde044af` | 9, verdict `BLOCKING` |
| discussion | `4e79893bf6` | `b194fbc4c2` | 16, verdict `BLOCKING` |

Both were picked for having `BLOCKING` findings and a mid-sized artifact.
`data/fixers-input.tsv` holds their 24 findings that state one change, dropping discussion F3 of `4e79893bf6` (two alternatives).
The review file of each round is `_lym/reviews/<kind>/round-1-review.md` at its weft commit, and the artifact is `_lym/plan/` (without the `lens-review.*` files) or `_lym/discussion/` (without the `support-log.lens-*` files) at the same commit.

## What the bench would still have measured

- **Cost of asking:** the entry's worry that a rule costs the reviewer more output and changes what it finds is untested, but with the rule already in place the baseline arm is what a stricter arm would differ from, and the audit shows nothing left to tighten.
- **Method note for a later bench:** the hub's reviewer is `opus` at medium effort with a fork fan of three or four lenses, which an in-session subagent cannot spawn.
  A faithful run needs a reed-spawned reviewer through the repository's own launch path, or accepts the solo-reviewer round (`burler-cluster-solo.md`) as a different measurement.

## Files

- `data/findings.tsv`: every audited finding.
- `data/parse.awk`: the frontmatter parser behind it.
- `data/inputs.txt`: the two kept rounds.
- `data/fixers-input.tsv`: the findings kept as the `fixers-on-sonnet` input.
