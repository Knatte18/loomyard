# Burler twill-fixer bench: today's fixer against a fixer that checks its own fix with forks

2026-10-16, prime `/home/knatte/Code/loomyard-LYXHUB/loomyard`, board note `burler-fixer-fork`.
This is stage 0 of `docs/research/burler-fork-review-audit-2026-10-16/report.md`, cut by the operator to a proof of concept: does a twill fixer work at all?
Three references, one run per arm per reference: six valid runs.

## Files

- `setup/`: the wrapper (`wrap-btf.sh`), the spliced block (`selfcheck-block.md`), the fix-check rubric, the code-base notice, the grader brief, and the pair, wave, review, harvest, teardown, usage and fork-check scripts.
- `finding-grades-plan.md`, `finding-grades-discussion.md`: the grades.
- `tokencount.md`: the token count over the six kept pairs.
- The transcripts, round files, fork reports, fix diffs and reference keys stayed in the bench's scratch folder and are not kept.

## Verdict

1. The twill fixer works.
   In all three runs it spawned exactly three forks in one message.
   Each fork made 1-2 read calls and returned in 15-24 seconds, with no write, no Agent call and no git command.
   The fixer then judged every fork finding with no wrong disposition.
2. It brought fewer defects into the next round: 3 class-A findings against 8 (next review and grader audit together), and 2 against 5 counting the next review alone.
   It made 1 wrong round-1 disposition against 3.
3. It cost +$0.71 and +1 minute a fixer session, at the top of the audit's +$0.55-0.75 estimate.
   The fixer ended with about 22k more context (85k against 63k).
4. The forks do not catch a defect that comes from the reviewer's own proposal: 7 of the 8 class-A findings in the single arm were `proposal`, and twill's remaining 3 are all `proposal`.
   In two of them (KI, KR) the claims or way-forward fork read the flawed text and endorsed it.
   A consequence check of the proposed fix belongs at reviewer level, or in a fork lens that challenges the proposal itself, not only its execution.
5. Whether it saves a round is not shown: three pairs is too few, and no run went on past the next review.

## Arms

- **s, single:** today's `burler-step-fix.md`, unchanged.
- **t, twill:** the same text, plus a `## Self-check of your fix by forks` block (`setup/selfcheck-block.md`) spliced in before `## Fixer-report rule`.
  - After its last fix, the fixer reads a short fix-check rubric (`setup/fix-check-rubric.md`).
  - It spawns the `consistency`, `claims` and `way-forward` forks in one message, with twill's fork boilerplate and a 6-call budget, read-only.
  - It judges the findings with the `twill-judge-critical.md` text and records them in a `## Self-check` section of the fixer-report.
- Both arms: `opus[medium]` fixer and next reviewer, the hub's `sonnet` Bouncer judge, `CLAUDE_CODE_FORK_SUBAGENT=1`, `deny_agent_tool: false`.
  The bench's test copies showed the `s` fix step is byte-identical to the stencil.

## Method

- **References:** round-1 fixes whose next review found a defect the fix brought in.
  | Reference | Perch | Round | Code base | Key |
  |---|---|---|---|---|
  | `intake-bugs-1012` (i) | Plan | 1 | `e5b56fe91` | KI: a row the fix added rests on a false premise ("would report `./nope`"); KI5: card 2's Bound omits the passes the new rows pin |
  | `runs-overview` (r) | Plan | 1 | `d22189d84` | KR: the fix for the duplicated format brought in a no-test clause |
  | `shed-tackler` (k) | Discussion | 1 | `9ea1f7474` | KS: the fix states a write bound on the tackler that the agent settings do not hold |
- **Pairs:** `twb-btf-{i,r,k}{s,t}` (`setup/setup-pair.sh`):
  - a `twb-*` note with the reference's board body, `lyx fabric add`, code branch reset to the base;
  - `_lyx/discussion`, `_lyx/plan` and `_lyx/reviews/*` checked out from the records commit `<P>-Burler-Fix -> running` of round 1;
  - pair `claude.yaml` `binary:` set to `setup/wrap-btf.sh`;
  - pair `loom.yaml` pinning `opus[medium]` review and fix, with `skip_alignment: true` and raised budgets;
  - seed, Preflight and Loom-Preflight, then the status placed onto the fix row (`setup/place-status.sh`).
- **Run:** `lyx loom pause --after <P>-Bouncer` and `lyx loom step --until-stop` ran the fixer and the judge.
  `lyx loom pause --after <P>-Burler-Review` and another step ran a fresh round-2 review.
  Every Bouncer bounced, so no `goto` was needed.
- **Wrapper:** it touches only a fixer session.
  It finds the fix-step file named in the fixer's `prompt.md`, splices the arm's block, fails closed on a missing or repeated marker, and appends the code-base notice to every session.
  No stencil, board copy or Bolt entry was edited.
- **Measures:**
  - cost from the transcripts (`setup/usage.sh`: deduplicated, last line per message id, Opus 5.5 row, the fork's copy of the spawn message excluded), judge cost from `tokencount.md` (it runs on Sonnet);
  - fixer wall time from its first to its last message;
  - fixer context at its last message;
  - fork checks from `setup/forkcheck.sh`.
- **Grading:** one grader session per perch graded both arms of each reference with the same reading.
  It used the scale of the earlier twill benches: real/wrong/noise, re-rated severity and right/wrong dispositions.
  Each round-2 finding got a class: `A` (the round-1 fix brought it in), `B` (there before) or `setup` (comes from the bench's base).
  Each class-A finding also got a cause: `proposal` (the reviewer's proposed change was flawed and applied as proposed) or `execution` (the fixer's own way of carrying it out).
  The grader also audited each fix diff for defects the review missed.

## Per run

| Run | Arm | Fixer | Forks | Fixer + forks | Judge | Next review | Fixer wall | Fork wall | Fixer end context | Round-1 wrong disp. | Fork findings real/wrong/noise | Fork disp. wrong | Round-2 real/wrong/noise (+setup) | Class A review / audit (proposal/execution) | Class B |
|---|---|---|---|---|---|---|---|---|---|---|---|---|---|---|---|
| is | s | $0.93 | | $0.93 | $0.14 | $1.46 | 3m22s | | 71k | 1 | | | 3/0/0 | 3 / 0 (2/1) | 0 |
| it | t | $1.17 | $0.27 | $1.44 | $0.15 | $1.61 | 3m17s | 24s | 85k | 1 | 1/0/1 | 0 | 3/0/2 | 1 / 0 (1/0) | 4 |
| rs | s | $0.51 | | $0.51 | $0.16 | $2.32 | 1m02s | | 59k | 0 | | | 3/0/0 (+2) | 1 / 0 (1/0) | 2 |
| rt | t | $0.74 | $0.19 | $0.93 | $0.15 | $1.54 | 1m41s | 15s | 72k | 0 | 0/0/1 | 0 | 0/0/0 (+2) | 0 / 1 (1/0) | 0 |
| ks | s | $0.56 | | $0.56 | $0.15 | $0.89 | 1m49s | | 59k | 2 | | | 1/0/1 (+1) | 1 / 3 (4/0) | 0 |
| kt | t | $1.45 | $0.31 | $1.76 | $0.15 | $0.82 | 4m22s | 24s | 98k | 0 | 4/0/1 (3 distinct) | 0 | 1/0/0 | 1 / 0 (1/0) | 0 |

Fork tool calls after the spawn: it 1/1/1, rt 1/1/1, kt 2/1/1, all reads (grep or sed); no fork re-read a changed file whole.

## Per arm

| Arm | Fixer session (mean) | Fixer wall (mean) | Fixer end context (mean) | Round-1 wrong disp. | Class A in next review | Class A, review + audit | Cause proposal / execution | Key R / P / A |
|---|---|---|---|---|---|---|---|---|
| s, single | $0.67 | 2m04s | 63k | 3 | 5 | 8 | 7 / 1 | 4 / 0 / 0 |
| t, twill | $1.38 | 3m07s | 85k | 1 | 2 | 3 | 3 / 0 | 2 / 0 / 2 |

## Key

| Run | KI | KI5 | KR | KS |
|---|---|---|---|---|
| s | R | R | R | R |
| t | R (claims and way-forward forks endorsed the false premise, and the fixer also wrote it into the card) | A (consistency fork) | R (the claims fork endorsed the no-test clause; found by the grader's audit, the review missed it) | A (the fixer's own fix, not a fork) |

## Findings

- **Mechanics:** every twill fixer followed the block: one spawn message, three forks, a `## Self-check` section with one line per fork finding.
  Forks inherited 65-90k of context, read 1-2 things and cost $0.05-0.11 each, $0.19-0.31 a run.
  This is cheaper than the audit's $0.40-0.55 estimate, because no fork re-read a file.
  Most of the added cost is the fixer itself: folding, re-checking and a longer report, $0.23-0.89 more than the single fixer of the same reference.
- **What the forks caught:** in `it` the consistency fork caught KI5, which the single arm's next review reported.
  In `kt` the forks raised 3 distinct real findings, all fixed, and the next review found 1 class-A finding against the single arm's 1 plus 3 more found by the grader's audit.
  In `rt` the forks found nothing real.
- **What they missed:** all three class-A findings left in the twill arm come from the reviewer's proposal.
  Two of them the forks saw and endorsed: the claims lens checks a claim against the code, but not whether a row "would report `./nope`" (KI), or whether "no test of its own" leaves a format untested (KR).
  The one `execution` defect of the bench is in the single arm (`is`).
- **Disposition quality:** twill made one wrong round-1 disposition (`it`, the same as `is`), and the single arm two more in `ks`.
  No fork disposition was wrong, and no fork finding was wrong.
- **Fixer context:** +22k at the end (85k against 63k), mostly the three fork reports and the folding turns, far from the 200k risk the audit names for a master design.

## What six runs support

- **Supported:** the twill fixer runs as designed with today's fork machinery, at about +$0.7 and +1 minute a fixer session, and its forks stay within budget.
- **Supported, as a direction:** fewer fix-introduced findings (3 against 8) and fewer wrong dispositions (1 against 3).
  With one run per arm per reference and one grader per perch, this is a lead, not a measured effect.
- **Supported:** the remaining defects come from the reviewer's proposed fix, which the forks take as given.
- **Not shown:** rounds saved.
  The break-even (one avoided round per 2.5-4 self-reviewed fixes, `burler-fork-review-audit-2026-10-16`) needs production runs with the mode on, which the operator has chosen as the reference.

## Deviations

- **Scope:** the operator cut the bench to a proof of concept mid-run, so three references instead of six, one run per arm.
- **Six invalid pairs, dropped and removed:** `twb-btf-{e,l,t}{s,t}` replayed rounds 2-3 (`engine-model-choice`, `logging-cleanup`, `tender`).
  The weft commits a perch's artifact edits only when the perch ends ("discussion artifacts", "plan artifacts"), so the round-N `Burler-Fix -> running` commit holds the pre-round-1 text for any N above 1.
  Those fixers got a round-N review over the first draft; the orch caught it in `es`/`et`.
  Only round-1 replays are sound with this method.
  They cost $14.70 (Opus-priced, so the Sonnet judges are slightly overstated).
- **Setup findings:** the bench base for `runs-overview` lacks `bc9ee7a02` (`lyx cleanup`, landed 8 minutes after the reference pair was made and evidently in its branch by Plan-Write).
  So two round-2 findings in each of `rs` and `rt` are `setup` (excluded, equal in both arms).
  The `ks` reviewer read main despite the code-base notice and reported the design as landed (`setup`).
  Both intake fixers also cleared `edit-test-file` gate findings, which come from today's binary and did not exist at `e5b56fe91` (graded setup).
- **Reviewer timing:** the Discussion reviews ran 30 minutes after their fixers, once the dropped pairs were cleared.
  The Plan reviews of `rs` and `rt` waited about 6 minutes for the Plan wave.
- **Pair config:** `claude.yaml` `binary` and `deny_agent_tool` were set at the pair's worktree layer, since the local layer refuses them; both arms carry the same settings.
- **tokencount:** `tools/tokencount/tokencount.sh` writes the shared `.scratch/token-usage-by-role.md`.
  It was run once over the six kept pairs, copied to `tokencount.md`, and the previous report was put back.
  Its figures match `usage.sh` for fixers and forks.
- **Graders** ran as in-session subagents, not through reed; their cost is not in the totals (about 185k tokens together).
- **Spend:** kept runs $13.89 (tokencount), dropped pairs $14.70, graders about $3: about $32 in all, under the $60 cap.
- **Teardown:** all twelve pairs and their `twb-*` board notes were removed after the bench.
  No parent-review request came from any pair.
  No committed file, stencil or Bolt entry was edited.
