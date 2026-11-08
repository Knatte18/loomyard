# What a darn costs, and a darn family against a tapestry, measured 2026-11-08

The `arras` recipe splits a large change into a family of darns instead of one tapestry.
This report prices the darn side; `docs/research/loom/writer-cost-remeasure-2026-11-08/report.md` priced the tapestry side.
Every figure derives from the scripts and tables beside this file; the sample size stands beside each number.
No agent run, test or gate run was made: the work reads usage records, transcripts, `lym report` output and git history.
Cost is API list price of the tokens.

## Finding

The sample is 291 finished darn runs and 16 finished tapestry runs, all counted by `tools/tokencount` since 2026-10-25.
285 darns have a landing commit, the first landed on 2026-10-27; 284 enter the means, after leaving out `comment-cleanup`.
15 tapestry runs enter the comparison.

- A darn costs $5.45 on average, $4.63 at the median and $10.54 at the 90th percentile, and takes 38 awake minutes.
  The mean landing changes 1,229 lines and the median 386.
  Size explains little of the cost: a least-squares fit gives $4.48 a run plus $1.43 per 1000 lines, r 0.31 over 279 darns.
- No darn of the sample ran a Discussion agent.
  A darn's Discussion is its decision record, which Go writes from the board entry,
  or a Motif run ahead of a raddle split; the Discussion line is $0 in all 284 darns.
- The writer is the larger cost.
  Writer and its lookup subagents are $3.13 a darn, 58% of the cost; the Code-Review perch is $2.03, 37%.
  The ratio is 1.54 over all darns, 1.35 in `light`, 1.13 in `solid`, and 1.23 among the 222 darns that ran a review.
- Priced at Sonnet list price for the same tokens, an Opus writer session costs 63% as much, a saving of 37%.
  This is an estimate: a Sonnet writer may take more turns.
- The 12 darns of the `fabric-test-speed` raddle split cost $39.88, and $58.53 with the Motif Discussion run and the raddle run before them,
  for 4,134 changed lines: $14.16 per 1000 lines against $11.29 for the 15 tapestry runs.
  The tapestry runs are larger, 6,742 lines on average, and their cost per 1000 lines falls with size.
  A least-squares fit of the tapestry runs puts one tapestry of 4,134 lines at $60.08 and 171 awake minutes.
  The family cost 3% less than that and its runs summed to 417 minutes, 225 minutes from the first Motif start to the last darn's end.
- The split moves cost between phases.
  A tapestry spends 42% of its cost on Plan; the family spends 4% on the raddle run.
  The family spends 28% on Code-Review where a tapestry spends 4%, and 36% on writing where a tapestry spends 30%.

## Sample and method

- Runs: `go run ./tools/tokencount -since 2026-10-25` counts 307 finished darn and tapestry runs from their session transcripts.
  `parse-tokencount.sh` splits its report into `parts.tsv`, the cost of each segment and part of each run,
  and `producers.tsv`, the entries and time of each producer.
  A slug reused by several runs counts once, as its newest run.
- Usage records: `collect.sh` reads `_lym/usage` at every archive tag since 2026-10-25 into `runs.tsv` and `sessions.tsv`.
  Only the 86 runs from 2026-11-05 on hold one; they give the profile, whether the run had a stored record, and landed lines.
  The 208 darns before that carry no profile and are one group, "before profiles".
  Their recorded session costs are 92% to 101% of the transcript costs of the same 76 darns; the records leave out some forks.
- Wall time: `run-table.sh` reads `lym report timing --since 2026-10-25 --format json`, run from the prime worktree.
  The figure is the awake span, from the run's start to its end less machine sleep.
- Changed lines: the lines added plus deleted by the run's landing commit,
  the newest commit on `main` titled after the change description archived at the run's tag and committed within the run's span.
  For the 79 runs with a `landed.json` record it equals the record.
  Six darns, the `codex-engine-*` and `codex-*` merges, have no landing commit and no line figure.
- Cost lines: Discussion, writer (the Darn segment's writer) and writer forks (its lookup subagents),
  reviewers (Code-Review reviewer and its forks), judge and seed, fixers (Code-Review fixer and its forks),
  gate fixer, landing (the describe, pr-reviewer and landing-fixer seats), and other.
  `cost-table.sh` writes them per run to `cost-table.tsv`, for the 291 darns and the 16 tapestry runs.
- Review rounds are the run's Code-Burler-Review entries.
- Group figures: `tables.sh`.
  Per-1000-line figures divide a group's summed cost by its summed lines, over the darns of 1 to 10,000 changed lines.
- The family and tapestry comparison: `family.sh`, with the Motif and raddle costs of `family-pre.tsv`,
  read by `tokencount -no-cutoff fabric-test-speed` since both runs share that slug.
- Sonnet estimate: `sonnet-writer.sh`.

Left out of the means:
`comment-cleanup`, a darn whose comment-lead seat cost $335 of its $387.44;
the six darns with no landing commit;
and, from the per-1000-line and fit figures only, the five darns of more than 10,000 changed lines
(`repo-size-run-time`, the two `batching-hook-remeasure` runs, `turn-savings-measure` and `gate-one-binary-per-package`),
most of them measurement darns that commit data tables.
`cold-gate-fixer` is a tapestry run whose Discussion, Plan and Webster transcripts are not on this machine; it is left out of the tapestry figures.

## Darn cost by profile

Mean per darn:

| group | runs | changed lines | wall min | review rounds | Discussion | writer | writer forks | reviewers | judge and seed | fixers | gate fixer | landing and other | total |
| --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- |
| docs | 16 | 8700 | 20 | 0.1 | $0.00 | $3.75 | $0.74 | $0.02 | $0.01 | $0.02 | $0.00 | $0.01 | $4.55 |
| docs, stored record | 1 | 61 | 6 | 0.0 | $0.00 | $0.60 | $0.00 | $0.00 | $0.00 | $0.00 | $0.00 | $0.00 | $0.60 |
| light | 33 | 345 | 38 | 1.0 | $0.00 | $2.39 | $0.20 | $0.90 | $0.21 | $0.81 | $0.14 | $0.16 | $4.81 |
| light, stored record | 10 | 294 | 30 | 1.0 | $0.00 | $1.76 | $0.00 | $0.82 | $0.19 | $0.42 | $0.11 | $0.09 | $3.40 |
| solid | 12 | 786 | 62 | 1.7 | $0.00 | $3.87 | $0.42 | $1.50 | $0.31 | $2.00 | $0.30 | $0.32 | $8.72 |
| solid, stored record | 2 | 977 | 41 | 1.5 | $0.00 | $2.83 | $0.00 | $1.06 | $0.24 | $0.62 | $0.17 | $0.11 | $5.04 |
| speed | 2 | 52 | 16 | 0.5 | $0.00 | $0.72 | $0.00 | $0.29 | $0.09 | $0.23 | $0.00 | $0.04 | $1.38 |
| before profiles | 208 | 885 | 39 | 0.9 | $0.00 | $3.03 | $0.12 | $0.88 | $0.24 | $1.04 | $0.06 | $0.22 | $5.59 |
| all landed darns | 284 | 1229 | 38 | 0.9 | $0.00 | $2.96 | $0.17 | $0.85 | $0.22 | $0.96 | $0.08 | $0.20 | $5.45 |

`lym report cost --since 2026-10-25` reads the same usage records and agrees where it has a cell:
`docs` $4.55 over 16 runs, `light` $6.01 over 34 (the extra run is `comment-cleanup`, recorded at $50),
`light` with a stored record $3.29 over 10, and `solid` $8.41 over 12.
Its `speed`, `docs` with a stored record, `solid` with a stored record and non-darn cells hold fewer than 5 runs.
The `docs` mean of 8,700 lines is driven by `repo-size-run-time`'s 57,920 and the two `batching-hook-remeasure` runs'.
`docs` darns run no review: they are the writer alone.

Per 1000 changed lines, over the darns of 1 to 10,000 lines:

| group | runs | thousand lines | writer and forks | reviewers | judge and seed | fixers | gate fixer | other | total | wall min |
| --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- |
| docs | 13 | 6.5 | $5.26 | $0.06 | $0.02 | $0.05 | $0.00 | $0.01 | $5.41 | 30.2 |
| docs, stored record | 1 | 0.1 | $9.84 | $0.00 | $0.00 | $0.00 | $0.00 | $0.00 | $9.84 | 100.0 |
| light | 33 | 11.4 | $7.51 | $2.60 | $0.62 | $2.36 | $0.40 | $0.46 | $13.94 | 111.1 |
| light, stored record | 10 | 2.9 | $5.99 | $2.80 | $0.65 | $1.42 | $0.38 | $0.31 | $11.55 | 103.2 |
| solid | 12 | 9.4 | $5.46 | $1.91 | $0.40 | $2.54 | $0.39 | $0.41 | $11.10 | 79.3 |
| solid, stored record | 2 | 2.0 | $2.89 | $1.09 | $0.25 | $0.64 | $0.18 | $0.11 | $5.15 | 42.0 |
| speed | 2 | 0.1 | $14.08 | $5.73 | $1.75 | $4.37 | $0.00 | $0.78 | $26.70 | 319.4 |
| before profiles | 206 | 138.8 | $4.66 | $1.30 | $0.35 | $1.53 | $0.10 | $0.34 | $8.27 | 57.9 |
| all landed darns | 279 | 171.2 | $4.92 | $1.40 | $0.37 | $1.58 | $0.13 | $0.33 | $8.73 | 62.3 |

Groups of fewer than 5 runs and the small `speed` and stored-record groups are single cases, not rates.
A darn's cost is mostly per run, so the per-1000-line figure falls as a darn grows:
the five dearest darns are `repo-size-run-time` ($34.73), `fabric-remote-async` ($25.84), `pawl-loom-recipe` ($18.34),
`agent-skills-and-effort` ($18.19) and `fabric-test-speed-survey` ($17.50).
The darns that ran 0, 1 and 2 review rounds are 63, 185 and 43 of the 291.
Where the gate fixer ran, in 92 of the 291 darns, it cost $0.27 a run.

## Writer against Code-Review perch

| group | runs | writer and forks per run | share of total | perch per run | share of total | writer over perch | total per run |
| --- | --- | --- | --- | --- | --- | --- | --- |
| docs | 16 | $4.49 | 99% | $0.06 | 1% | 80.79 | $4.55 |
| docs, stored record | 1 | $0.60 | 100% | $0.00 | 0% | no review | $0.60 |
| light | 33 | $2.59 | 54% | $1.92 | 40% | 1.35 | $4.81 |
| light, stored record | 10 | $1.76 | 52% | $1.43 | 42% | 1.23 | $3.40 |
| solid | 12 | $4.29 | 49% | $3.81 | 44% | 1.13 | $8.72 |
| solid, stored record | 2 | $2.83 | 56% | $1.93 | 38% | 1.46 | $5.04 |
| speed | 2 | $0.72 | 53% | $0.61 | 44% | 1.19 | $1.38 |
| before profiles | 208 | $3.15 | 56% | $2.15 | 38% | 1.46 | $5.59 |
| all landed darns | 284 | $3.13 | 58% | $2.03 | 37% | 1.54 | $5.45 |

The perch is reviewers, judge and seed, and fixers, forks included; it runs on Sonnet.
Within it the fixers are $0.96 a darn, the reviewers $0.85 and the judge and seed $0.22.
The writer leads in every group that runs a review, and its lead is smallest in `solid`:
`solid` darns spend 44% of their cost on the perch, with $2.00 of fixers a darn.
Among the 222 darns that ran at least one round, the writer is $3.19 a darn and the perch $2.60.

### The writer at Sonnet list price

54 Opus writer sessions in the usage records have a recorded cost equal to their tokens at Opus list price;
the sessions that changed model partway and `comment-cleanup` are left out.
They cost $119.81.
The same tokens at Sonnet list price cost $75.18, 63% of the Opus cost.
Cache reads, 26% of the Opus cost, are priced alike on the two models, while cache writes (43%) and output (32%) halve.
Applied to the mean writer cost of $2.96 a darn, a Sonnet writer saves up to $1.10 a darn, 20% of the darn.
The bound is generous: 15 of the 78 writer sessions in the usage records already ran on Sonnet.
The estimate holds the tokens fixed; a Sonnet writer may need more turns or more perch rounds, and nothing here measures either.

## The fabric-test-speed family against the tapestry runs

The darns of the family are the 12 parts of one raddle split, `fabric-test-speed`, filed on 2026-11-07,
whose proposal names them and whose Motif run discussed the work.
The task named this family the fabricengine rewrite; its project is "Take internal/fabricengine tier 1 time down".

| darn | profile | changed lines | wall min | writer and forks | Code-Review | gate fixer | landing and other | total | rounds |
| --- | --- | --- | --- | --- | --- | --- | --- | --- | --- |
| `battencli-templates` | light | 24 | 29 | $0.59 | $0.72 | $0.00 | $0.06 | $1.37 | 1 |
| `copyhub-fixtures-and-hook` | light | 35 | 13 | $1.05 | $0.74 | $0.00 | $0.07 | $1.86 | 1 |
| `dotgit-worktree-reader` | solid | 1355 | 39 | $2.79 | $1.85 | $0.20 | $0.10 | $4.94 | 1 |
| `fabric-anchor-origin-templates` | light | 259 | 31 | $1.98 | $1.97 | $0.12 | $0.11 | $4.18 | 1 |
| `fabric-livestate-merge-templates` | light | 411 | 34 | $4.32 | $1.97 | $0.19 | $0.11 | $6.59 | 1 |
| `fabric-test-folds` | light | 783 | 45 | $2.51 | $1.65 | $0.33 | $0.14 | $4.63 | 1 |
| `fabric-test-speed-remeasure` | docs | 61 | 6 | $0.60 | $0.00 | $0.00 | $0.00 | $0.60 | 0 |
| `git-config-from-go` | light | 180 | 23 | $0.97 | $1.24 | $0.14 | $0.06 | $2.41 | 1 |
| `hubforge-registered-templates` | solid | 599 | 43 | $2.86 | $2.01 | $0.15 | $0.11 | $5.13 | 2 |
| `hubforge-slim-copy` | light | 187 | 38 | $1.27 | $1.65 | $0.12 | $0.08 | $3.12 | 1 |
| `hubreconcile-templates` | light | 131 | 34 | $1.27 | $1.13 | $0.12 | $0.11 | $2.63 | 1 |
| `loomcli-templates` | light | 109 | 25 | $0.98 | $1.26 | $0.10 | $0.08 | $2.42 | 1 |
| sum of the 12 darns | | 4134 | 359 | $21.19 | $16.19 | $1.47 | $1.03 | $39.88 | |
| Motif Discussion run before them | | | | | | | | $16.34 | |
| raddle run | | | | | | | | $2.31 | |
| family | | 4134 | | | | | | $58.53 | |

The Motif run ran 40 minutes from 13:13 UTC, the raddle run 18 from 13:57,
and the darns ran from 14:15 to 16:58, 163 minutes of calendar time for 359 minutes of awake time.
11 of the 12 writers ran on Opus; `fabric-test-speed-remeasure` ran on Sonnet.
The Motif Discussion run is $4.81 of writer, $7.93 of reviewer, $3.23 of fixer, and $0.37 of judge and seed.

The tapestry runs of the same span, 15 of the 16 counted:

| tapestry | changed lines | wall min | Discussion | Plan | Webster | Code-Review | gate fixer, landing and other | total |
| --- | --- | --- | --- | --- | --- | --- | --- | --- |
| `run-verbs-shared` | 9150 | 253 | $25.59 | $54.68 | $51.93 | $2.01 | $1.79 | $136.00 |
| `gate-run-keeps-cache-warm` | 5068 | 181 | $16.12 | $27.03 | $18.46 | $3.82 | $2.03 | $67.46 |
| `interactive-recipe` | 3630 | 138 | $16.10 | $29.01 | $14.90 | $2.45 | $1.22 | $63.68 |
| `machine-switch` | 6734 | 215 | $18.35 | $28.04 | $25.03 | $3.43 | $0.94 | $75.79 |
| `plan-split-producers` | 7089 | 180 | $16.75 | $34.65 | $23.69 | $3.19 | $1.88 | $80.16 |
| `reed-status-buttons` | 2695 | 154 | $17.16 | $25.65 | $10.51 | $2.66 | $2.31 | $58.29 |
| `warp-visibility` | 7365 | 201 | $19.18 | $39.44 | $22.48 | $2.91 | $0.94 | $84.95 |
| `batten-remote-order` | 5261 | 296 | $23.40 | $31.65 | $21.61 | $4.20 | $3.66 | $84.52 |
| `decision-record-on-board` | 1969 | 143 | $9.58 | $19.48 | $6.96 | $3.24 | $2.88 | $42.14 |
| `shed-sub-run-block` | 2775 | 121 | $9.15 | $19.01 | $7.95 | $1.63 | $1.65 | $39.39 |
| `drift-rework` | 16913 | 373 | $20.65 | $48.06 | $45.73 | $4.11 | $5.95 | $124.50 |
| `pawl-orch-landing` | 8484 | 225 | $11.72 | $32.25 | $26.74 | $5.85 | $3.45 | $80.01 |
| `raddle-recipe` | 11648 | 311 | $22.35 | $43.01 | $36.45 | $4.95 | $5.76 | $112.52 |
| `slack-relay` | 7416 | 177 | $0.00 | $29.79 | $14.18 | $4.02 | $2.13 | $50.12 |
| `creel-cross-hub-build` | 4927 | 206 | $0.00 | $21.33 | $15.66 | $2.62 | $2.71 | $42.32 |
| sum of the 15 | 101124 | 3174 | $226.10 | $483.08 | $342.28 | $51.09 | $39.30 | $1141.85 |

`slack-relay` and `creel-cross-hub-build` ran no Discussion-Write session.
Plan, Webster and Discussion costs include their reviewers, fixers and forks;
Code-Review is the final review of the whole change.

Per 1000 changed lines, with each phase's share of the total:

| phase | family USD per 1000 lines | family share | tapestry USD per 1000 lines | tapestry share |
| --- | --- | --- | --- | --- |
| Discussion | $3.95 | 28% | $2.24 | 20% |
| Split or Plan | $0.56 | 4% | $4.78 | 42% |
| Write | $5.13 | 36% | $3.38 | 30% |
| Code-Review | $3.92 | 28% | $0.51 | 4% |
| gate fixer, landing and other | $0.60 | 4% | $0.39 | 3% |
| total | $14.16 | 100% | $11.29 | 100% |

Phases are matched by role.
Discussion is the Motif run's Discussion for the family and the tapestry's Discussion segment.
Split or Plan is the raddle run for the family and the tapestry's Plan segment.
Write is the darns' writers for the family and the tapestry's Webster segment.

Least-squares fits of total cost and wall minutes on thousands of changed lines:

| set | runs | USD fixed | USD per 1000 lines | r | wall min fixed | wall min per 1000 lines | r |
| --- | --- | --- | --- | --- | --- | --- | --- |
| darns of 1 to 10,000 lines | 279 | $4.48 | $1.43 | 0.31 | 35 | 5 | 0.18 |
| tapestry runs | 15 | $34.65 | $6.15 | 0.80 | 106 | 16 | 0.86 |

At the family's 4,134 lines the tapestry fit gives $60.08 and 171 awake minutes.
The family cost $58.53 and its runs summed to 417 minutes:
the Motif run's 40, the raddle run's 18 and the darns' 359 awake minutes.

## What the figures say for arras filing darns

- At the family's size the split is no cheaper than a tapestry.
  $58.53 against a fitted $60.08 is within the spread of the fit; the 15 tapestry runs scatter widely around it.
  Per 1000 lines the family is 25% dearer than the tapestry mean, but that mean is 6,742 lines a run and cost per 1000 lines falls with size.
  The three tapestry runs nearest the family in size, of 3,630, 4,927 and 5,068 lines, cost $63.68, $42.32 and $67.46.
- The split trades the Plan phase for a Code-Review phase per darn.
  The raddle run costs $2.31 where a tapestry's Plan costs $32.21 on average,
  and the darns' Code-Review costs $16.19 where a tapestry's costs $3.41.
  Plan wins $4.22 per 1000 lines; Code-Review loses $3.41, and writing loses $1.75.
  The perch cost scales with the number of darns, because each runs its own reviewer, judge and fixer,
  at $1.35 a darn in the family and $2.03 over all darns.
- Splitting does not make the work faster in these runs.
  The family took 225 minutes from the first Motif start to the last darn's end, against 171 awake minutes fitted for a tapestry of its size.
  The darns' own 163 minutes of calendar time hold a dependency chain, `dotgit-worktree-reader` ahead of `fabric-test-folds` and others.
- The Motif Discussion run is the largest single line after the writers, $16.34, 28% of the family.
  A tapestry's mean Discussion is $15.07, so filing darns does not shrink the Discussion, which is paid once per split either way.
- A darn has a per-run floor.
  The family's darns average $3.32; the fit gives $4.48 a run before size.
  Each extra darn adds a writer start and a perch pass, so the saved Plan phase has to outweigh that floor times the number of darns.
  At 12 darns of 344 lines on average the two came out about even.
- The two levers are in the darn.
  An Opus writer on Sonnet saves an estimated 37% of its cost, $7.66 over the family's 11 Opus writers, 13% of the family.
  Each darn's review at one round costs $1.35, 41% of a family darn, and `light` darns spend 40% of their cost on the perch.

## Limits

- 15 tapestry runs and 12 family darns; the family is one split, of one project, filed on one day.
  Every rate is a case count over few cases.
- The profile and stored-record flag exist only for the 86 runs from 2026-11-05 on; the 208 earlier darns are one unlabelled group.
  The small groups, `speed` (2), `docs` with a stored record (1) and `solid` with a stored record (2), are single cases.
- The orch's session and the operator's conversation are not priced: the orch cost is $0 in every run, as `tokencount` reports it.
  The Discussion of a darn without a Motif run happens there, so a darn's Discussion line understates its true cost.
- Cost is list price of the tokens in the transcripts; `comment-cleanup` is left out of the means, and with it a $335 comment-lead cost.
- Changed lines count tests, docs and deleted lines, so a darn that deletes a lot or commits a data table reads as large.
- The tapestry sample is 15 runs from 2026-10-31 to 2026-11-06, with a mix of sizes that differs from the family's,
  and the fit across them leaves an unexplained spread.
- A slug reused by several runs enters once, as its newest run.
- The Sonnet estimate holds the tokens fixed and counts neither more turns nor more review rounds.
- The study measures cost, rounds and time, not quality: a darn's review pass catches findings the cost figures do not show.
