# twill fixer bench, Plan-Write: after and fork0 against today's judge and a critical fixer

2026-10-14, prime `/home/knatte/Code/loomyard-LYXHUB/loomyard`, board note `twill-fork-timing`.
This pass only grades the existing runs; no new runs.
The per-finding tables, key tables and per-run tallies are in `.scratch/twill-fixer-bench/finding-grades.md`, which is not committed.

## The bench

A 2x2 on twill's Plan-Write step:
- Writer timing: "after" (lens forks spawn after the plan files are written) or "fork0" (lens forks spawn blind at the start).
- Judge step: today's judge, or a critical fixer modelled on Burler's fixer (`contracts/stencils/burler/burler-step-fix.md`), which weighs each finding's premise, then its proposed change, and disputes it, fixes it its own way, or fixes it as proposed.
- Arms: A after+today, B after+critical, C fork0+today, D fork0+critical.
- References: `intake-bugs-1012` (i) and `run-window-bugs` (w), one run per arm per reference, eight runs in all, stopped at the bench's $60 cap.

## Method

- Every lens finding of the eight runs was graded real, wrong or noise against the code at the run's base (`e5b56fe91` for i, `58534d414` for w).
  The method and scale are those of the earlier fork-timing grading.
- Each finding's severity was re-rated by the grader, and the counts below use the re-rated severity.
- Each disposition was graded right or wrong:
  - A real finding turned down is wrong.
  - A wrong finding accepted is wrong.
  - A fix that does not resolve a real premise is wrong.
  - Accepting noise counts as right unless it made the plan worse.
- Each final plan was checked against its reference's key, taken from that reference's real Plan-Review rounds.
  - intake:
    - KI1: the plan lets an argument outside the worktree root (`..`) pass (MEDIUM).
    - KI2: the scan's scan-end branches have no row that reaches them (MEDIUM).
    - KI3: a read error after the one re-step is used has no stated outcome or row (LOW).
    - KI4: the unplaceable `-C` rows reach no branch (LOW).
    - KI5: the Bound paragraph omits the scan-end passes (LOW).
  - window:
    - KW1: `loom fmt`'s file argument is read as a run-id (BLOCKING).
    - KW2: a nonexistent path's refusal is unstated or has no row (LOW).
    - KW3: the new executor is missing from the guard table (LOW).
    - KW4: the unreadable-file refusal has no row that reaches it (MEDIUM).
- Grading was done by two grader sessions, one per reference, so all four arms of a reference were graded by the same session.

## Per run

| Run | Arm | Findings | Real (B/M/L) | Wrong | Noise | Distinct real | Real turned down | Wrong accepted | Other wrong disp. | Key R | Key P |
|---|---|---|---|---|---|---|---|---|---|---|---|
| ia | A | 15 | 10 (0/0/10) | 0 | 5 | 10 | 0 | 0 | 0 | 3 | 1 |
| ib | B | 18 | 10 (0/0/10) | 1 | 7 | 9 | 0 | 1 | 0 | 3 | 1 |
| ic | C | 22 | 15 (0/2/13) | 0 | 7 | 14 | 1 | 0 | 0 | 3 | 1 |
| id | D | 23 | 19 (0/5/14) | 1 | 3 | 17 | 0 | 0 | 1 | 3 | 1 |
| wa | A | 22 | 14 (0/3/11) | 0 | 8 | 14 | 1 | 0 | 0 | 0 | 1 |
| wb | B | 17 | 13 (0/4/9) | 0 | 4 | 11 | 0 | 0 | 0 | 1 | 1 |
| wc | C | 21 | 14 (0/6/8) | 0 | 7 | 10 | 0 | 0 | 0 | 1 | 2 |
| wd | D | 24 | 21 (1/9/11) | 0 | 3 | 18 | 0 | 0 | 0 | 1 | 1 |

## Key per final plan

A means the plan avoids the problem, P that it partly repeats it, R that it repeats it.

| Run | KI1 | KI2 | KI3 | KI4 | KI5 |
|---|---|---|---|---|---|
| ia | A | R | P (stated, no row) | R | R |
| ib | R (brought in by a lens finding) | R | A | R | P |
| ic | A | R | P (row turned down) | R | R |
| id | A | R | P | R | R |

| Run | KW1 | KW2 | KW3 | KW4 |
|---|---|---|---|---|
| wa | A | A (row added after a lens finding) | A | P |
| wb | A | P (stated, no row) | A | R |
| wc | A | P (stated, no row) | R | P |
| wd | A (the fork0 writer missed it, a lens caught it) | A (row added after a lens finding) | R | P |

## Per arm

| Arm | Cost/plan | Findings | Real | Real M+ | Distinct real | Wrong + noise | Turned down (i/w) | Wrong dispositions | Key R / P |
|---|---|---|---|---|---|---|---|---|---|
| A after+today | $7.66 | 37 | 24 | 3 | 24 | 13 | 4/8 | 1 (real finding turned down) | 3 / 2 |
| B after+critical | $8.60 | 35 | 23 | 4 | 20 | 12 | 1/3 | 1 (wrong finding accepted, which put KI1 in the plan) | 4 / 2 |
| C fork0+today | $7.21 | 43 | 29 | 8 | 24 | 14 | 7/7 | 1 (real finding turned down) | 4 / 3 |
| D fork0+critical | $6.72 | 47 | 40 | 15 | 35 | 7 | 1/1 | 1 (a fix whose row cannot fail) | 4 / 2 |

## Per factor

| Factor | Level | Cost/plan | Real | Real M+ | Distinct real | Wrong dispositions | Key R / P |
|---|---|---|---|---|---|---|---|
| timing | after (A+B) | $8.13 | 47 | 7 | 44 | 2 | 7 / 4 |
| timing | fork0 (C+D) | $6.97 | 69 | 23 | 59 | 2 | 8 / 5 |
| judge | today (A+C) | $7.44 | 53 | 11 | 48 | 2 | 7 / 5 |
| judge | critical (B+D) | $7.66 | 63 | 19 | 55 | 2 | 8 / 4 |

## Findings

- **Lens detection:** in the fork0 runs the lenses found more real problems, on both references: 69 against 47 overall, and 23 against 7 at MEDIUM or above.
  Part of this is that the fork0 plans had more to find.
  wd's writer missed KW1, and both fork0 plans repeat KW3, which both after writers avoided.
  The counts are not adjusted for how many defects each plan held, so this shows more defects found, not that the lenses were sharper.
- **Lens precision:** only 2 of 162 findings were wrong (one in ib, one in id); the other non-real findings were noise.
- **Judging:**
  - Today's judge turned down 26 findings.
    Two were real: wa's symlinked-root row and ic's spent re-step row (KI3).
    The rest were noise and rightly turned down.
  - The critical fixer turned down 6 findings, all rightly.
    Its 4 fixed-differently dispositions (1 in i, 3 in w) were all right.
  - The critical fixer's two wrong dispositions were of another kind:
    - ib accepted a wrong finding, which put KI1, the reference's own MEDIUM, into the plan.
    - id's `wantDetail` fix cannot fail, because `./internal/x` is a substring of `./internal/x_test`.
  - Each judge made 2 wrong dispositions, one per arm.
- **Why the critical fixer turned down fewer:** the findings it saw held less noise (B 11, D 6, against A 13, C 14), and accepting noise does no harm.
  So a low turn-down count does not mean it was less critical.
- **Key:**
  - Every arm repeats KI2, KI4 and KI5, and no lens in any run flagged them.
  - No lens flagged KW3 either; it survives only in the fork0 plans, so it comes from the writer.
  - KW1, the reference's BLOCKING, is avoided in all eight plans.
  - Key items repeated or partly repeated, per arm: A 5, B 6, C 7, D 6.
    The spread is one or two items and follows writer timing, not the judge.
- **Cost:** D is the cheapest arm ($6.72) and B the most expensive ($8.60).
  fork0 saves about $1.16 per plan.
  The judge factor makes about no difference ($0.22 apart, with opposite signs under the two timings).

## What eight runs support

- **Supported:** the critical fixer does not turn down real findings more often than today's judge, and its disputes and its own fixes held up in every case graded.
- **Supported:** fork0-arm lenses report more real findings, especially at MEDIUM, at no extra cost, which repeats the earlier six-run bench.
- **Supported:** the lenses share blind spots (KI2, KI4, KI5, KW3) that no arm or judge reaches.
  The key misses come from the writer and the lenses, not from the judge.
- **Not shown:** any effect of the judge on final-plan quality.
  One wrong disposition per arm is within noise, with one run per arm per reference.
- **Not shown:** whether fork0 lenses are sharper or fork0 plans are simply worse, since the bench has no per-plan defect count to adjust by.
- **Not shown:** run-to-run variance.
  The earlier bench saw a within-arm spread of 5 to 7 distinct findings, as large as most gaps here.
- **Caveats:** there were two grader sessions, one per reference, and real against noise is a judgement call.
  Each key comes from a single Plan-Review per reference, so it is a narrow list, not a full answer key.

## Verdict

1. The critical fixer is safe: every one of its disputes and its own fixes was right, and it turned down no real finding.
   Today's judge turned down two.
2. It does not improve the final plans: both judges made 2 wrong dispositions, and the key score is level within one or two items.
3. In the fork0 runs the lenses found many more real MEDIUM problems (23 against 7), but the fork0 plans also started with more defects, so lens sharpness is not shown.
4. The key misses (KI2, KI4, KI5, KW3) are blind spots shared by every lens; no judge can fix what no lens reports.
5. D (fork0 with the critical fixer) is the cheapest arm and found the most real problems, but with one run per arm per reference this is a lead to re-bench, not a result.
