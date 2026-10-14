# twill fork0 bench, Plan-Write: fork0 against after, both with the critical fixer

2026-10-14, prime `/home/knatte/Code/loomyard-LYXHUB/loomyard`, board note `twill-fork-timing`.
Six Plan-Write runs, one per arm per reference, graded by one grader in one pass.
The raw evidence stayed in the prime's git-ignored `.scratch/twill-fork0-bench/`, which is not committed: per-finding tables and key grades in `finding-grades.md`, setup files in `setup/`, evidence per run in `runs/<run>/` (prompt, plan, lens reports, transcripts, traces), keys in `keys/<reference>/`.
It follows the [fixer bench](twill-fixer-bench.md), whose arm D it reruns as arm f; the [rationale bench](twill-rationale-bench.md) adds a rationale file to arm f and records the outcome.

## The bench

- Arms, both with the critical fixer (Burler's two-sided weighing) as the judge step:
  - a, "after": the lens forks spawn after the writer has written the plan and passed its self-check (today's code).
  - f, "fork0": the master explores and forks one fork0 that writes the plan.
    The master then reads the finished plan fresh and spawns the lens forks, which never saw the writing.
- References:
  - i, `intake-bugs-1012` (code `e5b56fe91`, records `1b7fab0a`), key KI1–KI5 as in the [fixer bench](twill-fixer-bench.md).
  - w, `run-window-bugs` (code `58534d414`, records `3b538871`), key KW1–KW4 as in the [fixer bench](twill-fixer-bench.md).
  - p, `plan-gaps` (code `a5334fc72`, records `cc312812`), new key KP1–KP13.
- Pairs `twb-f2-ia`, `-if`, `-wa`, `-wf`, `-pa`, `-pf`, set up as `twb-fx-*`: `lyx fabric add`, code reset to the base, discussion records checked out from the records commit, `plan: opus[medium]`, `plan_producer: twill`, status moved onto Plan-Write, `lyx loom pause --after Plan-Write`, `lyx loom step --until-stop`.
- The arm text is spliced into each run's `prompt.md` by the `claude:` wrapper `setup/wrap-f2.sh`, which reuses the fixer bench's three fragments unchanged.
  The spliced prompts equal the fixer bench's arm B and arm D prompts except for paths.
- All six runs ended `paused after Plan-Write` at Plan-Bouncer; none asked for a parent review, and none showed a permission dialog.

### Why plan-gaps

- Its real Plan-Review ran two rounds and found 14 problems, 1 BLOCKING and 7 MEDIUM, the richest key among recent landed tasks of moderate size.
- Its problems are of the kinds the lenses target: a missing directory creation, an unreached acceptance criterion, a pattern invariant left false, bound statements, lease-test determinism and test economy.
- Its plan is small (5–6 cards), which kept the run cost near the intake runs'.
- Its discussion records are complete at the Plan-Write commit, and its code base is the last code commit before its Plan-Write started.
- Caveat: it is from 2026-10-11, so today's Plan-Write template and rubric meet older code; the same holds, less so, for i and w.

### Key for plan-gaps

From the real rounds 1 and 2 (`keys/plan-gaps/reviews/`):

- KP1 (B): the journal's scratch directory is never created.
- KP2 (M): restore holds journaled names to `cardSlugRe`, stricter than the plan format.
- KP3 (M): the prose-mention scan misses plural and list forms (`cards 2, 3`).
- KP4 (M): `PATTERN-sole-parsers`' closed write-path list is left without the new writers.
- KP5 (M): the verb card misstates or omits restore-renumber's bound and cost.
- KP6 (M): the archive-untouched acceptance criterion has no test.
- KP7 (M): the CLI tests duplicate the planparser tests, or test one verb per test.
- KP8 (L): a non-integer card number is read as a run-id.
- KP9 (L, M in round 2): the lease test cannot see a release or the lease start.
- KP10 (L): the precheck keys on a readable journal, not an existing one.
- KP11 (L): the stated bounds omit the journal writes outside the plan directory.
- KP12 (L): reversal does not say an absent created name is skipped.
- KP13 (L): an unreadable journal has no outcome in restore.

## Method

- The scale is the [fixer bench](twill-fixer-bench.md)'s:
  - every lens finding is real, wrong or noise against the code at the run's base and the decision record, with its severity re-rated;
  - every disposition is right or wrong;
  - every final plan gets A, P or R per key item.
- One grader graded all six runs in one pass, so every count below is on one scale; the rules are listed at the top of `finding-grades.md`.
- Cost is `go run ./tools/tokencount` over the six pairs (`tokencount-all.md`).
  Per run it is the writer session plus its forks, without the orchestrator; the split into writer, fork0 and lenses comes from the earlier bench's `usage.sh`.

## Per run

| Run | Arm | Cost | Writer (+fork0) | Lens forks | Span | Findings | Real (B/M/L) | Wrong | Noise | Distinct real | Disputed | Wrong disp. | Key R | Key P |
|---|---|---|---|---|---|---|---|---|---|---|---|---|---|---|
| ia | after | $7.26 | $6.47 | $0.80 | 16.6 min | 19 | 12 (0/1/11) | 0 | 7 | 11 | 4 | 0 | 1 | 1 |
| if | fork0 | $6.43 | $4.64 + $0.88 | $0.90 | 10.9 min | 17 | 13 (0/3/10) | 0 | 4 | 11 | 0 | 0 | 2 | 1 |
| wa | after | $9.60 | $8.30 | $1.31 | 18.6 min | 15 | 13 (0/4/9) | 0 | 2 | 13 | 0 | 1 | 1 | 1 |
| wf | fork0 | $9.32 | $6.38 + $1.62 | $1.32 | 17.3 min | 29 | 27 (0/10/17) | 0 | 2 | 19 | 2 | 0 | 1 | 1 |
| pa | after | $4.90 | $4.03 | $0.88 | 13.0 min | 21 | 16 (0/6/10) | 0 | 5 | 16 | 1 | 0 | 2 | 2 |
| pf | fork0 | $6.59 | $4.31 + $1.05 | $1.23 | 11.6 min | 20 | 18 (0/7/11) | 0 | 2 | 16 | 1 | 0 | 2 | 1 |

## Key per final plan

| Run | KI1 | KI2 | KI3 | KI4 | KI5 |
|---|---|---|---|---|---|
| ia | A | A (walk extracted and shared) | A | P | R |
| if | A | R (own copy, no pipeline or `\|\|` row) | A | P | R |

| Run | KW1 | KW2 | KW3 | KW4 |
|---|---|---|---|---|
| wa | A | P | A (lens finding) | R |
| wf | A | P | A (writer) | R |

| Run | KP1 | KP2 | KP3 | KP4 | KP5 | KP6 | KP7 | KP8 | KP9 | KP10 | KP11 | KP12 | KP13 |
|---|---|---|---|---|---|---|---|---|---|---|---|---|---|
| pa | A | A | R | A (lens) | P | A (lens) | A (lens) | A | P | A | R | A | A |
| pf | A | A (lens) | R | A | A | A (lens) | A | A | P | A | A | A | R |

## Per arm

| Arm | Cost/plan | Writer (+fork0) | Lens forks | Findings | Real | Real M+ | Distinct real | Wrong + noise | Wrong disp. | Key R / P | Key items avoided through a lens finding |
|---|---|---|---|---|---|---|---|---|---|---|---|
| after | $7.25 | $6.27 | $1.00 | 55 | 41 | 11 | 40 | 14 | 1 | 4 / 4 | 4 (KW3, KP4, KP6, KP7) |
| fork0 | $7.45 | $6.29 | $1.15 | 66 | 58 | 20 | 46 | 8 | 0 | 5 / 3 | 2 (KP2, KP6) |

Total for the six runs: $44.10, plus $7.97 of orchestrator cost attributed to their windows, $52.07 against the $60 cap.

## Findings

- **Cost:** level.
  fork0 is $0.20 a plan more expensive here.
  The master plus fork0 cost what the after writer costs ($6.29 against $6.27), and the lens forks cost about the same ($1.15 against $1.00).
  The [fixer bench](twill-fixer-bench.md)'s $1.16 saving for fork0 did not repeat: on plan-gaps fork0 cost $1.69 more, and on the other two references $0.28–0.83 less.
- **Time:** fork0 finished first on every reference (10.9 against 16.6, 17.3 against 18.6, 11.6 against 13.0 minutes).
- **Lens detection:** fork0's lenses reported more real findings in all three pairs, and more distinct real findings on w only: 58 against 41 real, 20 against 11 at MEDIUM or above, 46 against 40 distinct.
  Most of the gap is wf (27 real and 19 distinct, against wa's 13 and 13), whose plan held more to find.
  Among wf's findings: one planned test row could no longer reach its branch, and another was broken by the plan's own change.
  On i and p the distinct real counts are level (11 and 11, 16 and 16).
- **Blind catches:**
  - pf's forced-edits lens caught that `insert-card <N>` fails `cmd/lyx/clitree_test.go:163`'s lower-case `Use` grammar; pa's plan has the same `<N>` and none of its lenses flagged it.
  - if's lenses caught an ancestor row that cannot fail and a decode row that cannot tell a re-read from none; ia's lenses caught the missing ancestor row but not the decode gap.
- **Lens precision:** no finding in six runs was wrong.
  Noise was 7, 2 and 5 in the after runs against 4, 2 and 2 in the fork0 runs.
- **Judging:** the critical fixer made one wrong disposition in 106.
  wa accepted a noise finding and added a test that parses `pushretry.go` to pin a doc comment's wording.
  All 8 of its disputes and 7 own-change fixes held up, and it turned down no real finding, as in the [fixer bench](twill-fixer-bench.md).
- **Key:** level, with 8 items repeated or partly repeated per arm (after 4 R + 4 P, fork0 5 R + 3 P, of 22 key items each).
  - i: ia is better by one item (KI2), because ia's writer extracted the chain walk and if's fork0 copied it; no lens in either run flagged it.
  - w: identical grades.
  - p: pf is better by one item (2 R + 1 P against 2 R + 2 P), with different misses.
    pa states no bound for the journal writes (KP11) and leaves restore's cost out of the verb card (KP5); pf leaves an unreadable journal without an outcome (KP13).
  - Shared blind spots: KI5, KW4 and KP3 are repeated in both arms, and no lens in any run flagged them; KI4, KW2 and KP9 are partly repeated in both arms.
- **Where the key misses come from:** the writer, as in the [fixer bench](twill-fixer-bench.md).
  The after arm's lenses moved four key items to A, fork0's two.
  fork0's writer avoided two items (KW3, KP4) that the after arm needed a lens for.

## Pooled with the earlier benches (fork0 against after)

The three benches used different references, graders and judges, so these sums show a direction, not a measurement.

| Bench | Plans per arm | Cost/plan after | Cost/plan fork0 | Real after | Real fork0 | Real M+ after | Real M+ fork0 | Key R+P after | Key R+P fork0 |
|---|---|---|---|---|---|---|---|---|---|
| Fork-timing (`run-time-by-role`, today's judge, J1–J9) | 2 | $4.60 | $4.40 | 23 | 24 | 6 | 9 | 8 | 11 |
| [Fixer](twill-fixer-bench.md) (i, w; both judges) | 4 | $8.13 | $6.97 | 47 | 69 | 7 | 23 | 11 | 13 |
| This bench (i, w, p; critical fixer) | 3 | $7.25 | $7.45 | 41 | 58 | 11 | 20 | 8 | 8 |
| Pooled | 9 | $7.05 | $6.56 | 111 | 151 | 24 | 52 | 27 | 32 |

- In all three benches fork0's lenses reported at least as many real findings and more at MEDIUM or above.
  Pooled, that is 36 % more real findings and twice as many at MEDIUM or above.
- fork0 was cheaper in two benches of three; pooled it is about $0.50 a plan cheaper, inside the spread between runs of one arm (this bench's after runs range $4.90–9.60).
- Key repeats: fork0 was worse by 3 and 2 items in the first two benches and level here; pooled it is 32 against 27, over about 58 graded item-plans per arm.

## What six runs support

- **Supported:** fork0 costs no more than after.
  The saving the fixer bench saw did not repeat, so cost is level, not a reason for fork0.
- **Supported, now across three benches:** fork0's lenses report more real findings, more of them at MEDIUM or above, with no loss in precision.
- **Supported:** with the critical fixer, fork0's final plans repeat no more key items than after's here (8 against 8); the earlier gaps (11 against 8, 13 against 11) did not repeat.
  Each difference here traces to one writer's choice (KI2, KP5, KP11, KP13), which fits the reading that the earlier gap was writer variance.
- **Supported:** the critical fixer is safe in both arms: one wrong disposition in 106, and no real finding turned down.
- **Supported:** the key misses that survive are shared blind spots of every lens (KI5, KW4, KP3); neither arm reaches them.
- **Not shown:** that fork0's lenses are sharper rather than its plans holding more defects.
  There is still no per-plan defect count to adjust by, and wf alone carries most of this bench's gap.
- **Not shown:** run-to-run variance.
  There is one run per arm per reference, and the earlier benches saw within-arm spreads as large as most gaps here.
- **Not shown:** any final-plan quality gain from fork0.
  The key is level, and the extra lens findings fixed in fork0's plans are mostly LOW and MEDIUM test and doc gaps the key does not grade.
- **Caveats:**
  - There was one grader, and real against noise is a judgement call.
  - Each key comes from one real Plan-Review and is a narrow list.
  - plan-gaps is a 2026-10-11 task planned with today's template.
  - The pooled table mixes graders and judges.

## Verdict

1. Cost is level: fork0 $7.45 against after $7.25 a plan; pooled over nine plans per arm, fork0 is about $0.50 cheaper, inside the run-to-run spread.
2. fork0's blind lenses found more real problems again (58 against 41, 20 against 11 at MEDIUM or above), the third bench in a row in that direction, with no wrong findings.
3. With the critical fixer, fork0's final plans held up: 8 key items were repeated or partly repeated in each arm, so the earlier gap looks like writer variance.
4. The critical fixer judged well in both arms (one wrong disposition in 106), so fork0's fixing without the writer's reasoning did not hurt here.
5. On this evidence fork0 with the critical fixer is at least as good as after at the same cost, with sharper review; more runs per reference would turn that lead into a result.

The operator's decision on which Twill design to build is recorded in the [rationale bench](twill-rationale-bench.md#outcome-operator-2026-10-14).
