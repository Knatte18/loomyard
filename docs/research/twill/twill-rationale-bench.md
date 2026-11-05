# twill rationale bench, Plan-Write: fork0 with a rationale file against fork0

2026-10-14, prime `/home/knatte/Code/loomyard-LYXHUB/loomyard`, board note `twill-fork-timing`.
Three Plan-Write runs of arm r, one per reference, graded against the arm f runs of the [fork0 bench](twill-fork0-bench.md).
The raw evidence stayed in the prime's git-ignored `.scratch/twill-rationale-bench/`, which is not committed: `finding-grades.md`, `setup/`, `runs/{ir,wr,pr}/`, `tokencount-all.md`.

## The arm

- **Arm r:** arm f of the [fork0 bench](twill-fork0-bench.md) (fork0 + critical fixer) unchanged, plus one thing: fork0 also writes `PAIRDIR/.scratch/fork0-rationale.md`, giving for each card its non-obvious choices, why it made them, and the alternatives it rejected.
- **Order:** the master spawns the lens forks first, and the lens forks are told not to read the file.
  The master reads it only after every lens fork has returned, then judges and fixes as arm f's critical fixer.
- **Limits on the rationale** (judge fragment, `setup/rationale-judge.md`):
  - It states fork0's intent and is not an authority.
  - Every finding is still judged on its premise against the code and the decision record.
  - "fork0 meant it this way" is never by itself a ground to dispute.
  - It may only point to why a finding's premise is false, and the dispute then cites the code or record text, never the rationale; or it may help choose a better fix.
  - A disposition that used it must say `rationale:`.

## Setup

- **Same as arm f:** references, bases and config: i `intake-bugs-1012` (`e5b56fe91` / `1b7fab0a`), w `run-window-bugs` (`58534d414` / `3b538871`), p `plan-gaps` (`a5334fc72` / `cc312812`), with `plan: opus[medium]` and `plan_producer: twill`; the same pair setup and steps; all three runs in one wave.
- **Outcome:** every run ended paused after Plan-Write; no run asked for a parent review, and no pane showed a permission dialog.
- **Wrapper:** `setup/wrap-f3.sh` is `wrap-f2.sh` with arm r added.
  It reuses arm f's `fork0-preamble.md`, `fork0-block.md` and `critical-judge.md` byte for byte, and adds three fragments:
  - `rationale-fork0.md`, appended to fork0's prompt;
  - `rationale-master.md`, after the preamble: do not read the file before the forks return, plus the lens forks' "never read" line;
  - `rationale-judge.md`, under step 3, before the judge step.
- **Proof of a clean comparison** (`setup/prompt-diff-{i,w,p}.txt`):
  - With the pair name normalized, the prompt before splicing is identical to arm f's for every reference.
  - The spliced prompts differ from arm f's only by the 18 added fragment lines and the arm marker.
  - In arm f mode, `wrap-f3.sh` reproduces arm f's three spliced prompts byte for byte.
  - Because of the normalization, the rationale path in the diffs reads `twb-f2-*`; the real prompts name `twb-f3-*`.

## The blindness held

Checked in each run's transcripts (`runs/<run>/order-check.txt`):

| Run | Lens forks spawned | Last lens fork returned | Master's first read of the rationale | Lens-fork tool calls naming it | "Never read" line in every lens prompt |
|---|---|---|---|---|---|
| ir | 17:52:04–19 | 17:53:40 | 17:53:42 | 0 of 11 | yes |
| wr | 17:59:10–28 | 18:02:16 | 18:02:19 | 0 of 9 | yes |
| pr | 17:52:36–51 | 17:53:59 | 17:54:01 | 0 of 7 | yes |

The only other touch: ir's master ran `ls _lyx/plan .scratch` in its first exploration call, before fork0 was spawned and the directory existed.

## Scale check against the earlier grader

The key cells of the arm-f final plans (if, wf, pf) were re-graded before grading arm r; not fully blind, since the earlier grades were part of the background reading.

- **wf and pf:** every cell agrees.
- **if:** KI1, KI2, KI3 and KI5 agree; KI4 differs.
  - The earlier grader gave P; this grader would give R, because if's plan states that an unplaceable `-C` ends the scan but has no row that reaches it, which is the whole of KI4.
  - To keep the cells comparable, ir's identical pattern was graded P, the earlier convention.
  - With this grader's reading applied to both arms, f becomes 6 R / 2 P and r 8 R / 2 P; the gap does not change.

## Per run

| Run | Cost | Writer + fork0 | Lens forks | Span | Findings | Real (B/M/L) | Noise | Distinct real | Disputed | Wrong disp. | Key R / P |
|---|---|---|---|---|---|---|---|---|---|---|---|
| if (f) | $6.43 | $4.64 + $0.88 | $0.90 | 10.9 min | 17 | 13 (0/3/10) | 4 | 11 | 0 | 0 | 2 / 1 |
| ir (r) | $6.47 | $4.23 + $1.04 | $1.20 | 11.8 min | 22 | 16 (0/4/12) | 6 | 14 | 2 | 0 | 2 / 1 |
| wf (f) | $9.32 | $6.38 + $1.62 | $1.32 | 17.3 min | 29 | 27 (0/10/17) | 2 | 19 | 2 | 0 | 1 / 1 |
| wr (r) | $10.25 | $6.99 + $1.65 | $1.62 | 21.7 min | 24 | 22 (0/6/16) | 2 | 19 | 1 | 0 | 2 / 1 |
| pf (f) | $6.59 | $4.31 + $1.05 | $1.23 | 11.6 min | 20 | 18 (0/7/11) | 2 | 16 | 1 | 0 | 2 / 1 |
| pr (r) | $6.11 | $4.19 + $0.88 | $1.05 | 10.8 min | 20 | 20 (0/7/13) | 0 | 18 | 0 | 0 | 3 / 1 |

No lens finding in any of the six runs was wrong.

**Key, arm r against arm f:**

- **i:** ir has the same grades as if: KI1 A, KI2 R, KI3 A, KI4 P, KI5 R.
  For KI2, ir again copied the chain walk instead of sharing it, and its rationale names the shared walker as the alternative it rejected.
- **w:** wr is KW1 A, KW2 P, KW3 R, KW4 R.
  KW3 is new against wf: the new recording executor `leftoverRemoval.removeLeftover` in `destroy.go` is missing from `destructiveGuardRecordingExecutors`; the writer's miss, flagged by no lens.
- **p:** pr differs from pf in two places.
  pr repeats KP11, which pf avoided: its bound does not mention the journal under `.lyx`.
  pr's writer avoided KP2, where pf needed a lens finding.
  The other cells match pf's: KP3 R, KP9 P, KP13 R, the rest A.

## Per arm

Arms a and f are the [fork0 bench](twill-fork0-bench.md)'s.

| Arm | Cost/plan | Real findings | Real M+ | Distinct real | Wrong + noise | Wrong dispositions | Key R / P |
|---|---|---|---|---|---|---|---|
| a, after | $7.25 | 41 | 11 | 40 | 14 | 1 | 4 / 4 |
| f, fork0 | $7.45 | 58 | 20 | 46 | 8 | 0 | 5 / 3 |
| r, fork0 + rationale | $7.61 | 58 | 17 | 51 | 8 | 0 | 7 / 3 |

## Rationale-use grades (arm r)

Only 2 of arm r's 60 dispositions cited the rationale, and both uses were right; none disputed a real finding on fork0's intent.

| Run | Disposition | How the rationale was used | Grade |
|---|---|---|---|
| ir | Disputed, premise false: the finding said the `PATTERN-failure-stop-visible` background edit is outside Scope In. | The rationale pointed at the record's Constraints line; the dispute cites the decision record's own text, and the finding is noise anyway. | right |
| pr | Fixed as proposed: the `role` audience keeps the three verbs out of the operator command index the acceptance criterion names. | The rationale's reason for choosing `role` was weighed and overruled by `internal/helpcli/cli.go:103` and `internal/clihelp/index.go:24`. | right |
| wr | No citation. | – | – |

Two things the rationale exposed that nobody acted on; neither is a fault of the arm, since the fixer may only use the rationale to settle findings:

- **pr:** its rationale states a deliberate departure from the decision record: the journal path is declared in `planparser`, not in `internal/lyxdirs` as the record says.
  No lens flagged it.
- **ir:** its rationale names the shared chain walk as the rejected alternative, which is exactly KI2's fix.
  No finding raised it.

## Findings

- **Cost:** about level, $7.61 against $7.45 a plan.
  Writing the rationale costs fork0 nothing measurable ($1.19 against $1.18 a plan); the lens forks cost $0.14 a plan more, and wr's master $0.61 more than wf's.
- **Time:** level on i and p; wr took 21.7 min against wf's 17.3.
- **Lens findings:** the same totals (66 findings, 58 real), more distinct real (51 against 46), fewer at MEDIUM or above (17 against 20).
  The lens forks never saw the rationale, so this is run-to-run variance, not an effect of the arm.
- **Judging:** as strict as arm f: no wrong dispositions in 60, no real finding turned down; all 3 disputes and 4 own-change fixes were right.
- **Key:** r repeats or partly repeats 10 items against f's 8.
  Every difference comes from fork0's plan with no lens finding involved, so the rationale, which acts only through the fixer, could not have changed them.

## Total cost

| Item | Cost |
|---|---|
| Three runs (writer + forks) | $22.83 |
| Prime orchestrator session during the runs | $2.48 |
| Bench agent's session (setup, waiting, grading), from its own transcript at tokencount's prices | about $6.9 |
| **Total** | **about $32.2**, under the $35 cap |

`tokencount`'s orchestrator figure for the runs' window is $5.97: the prime's $2.48 plus $3.49 of the bench agent's session; its grading after the window closed, about $1.7, falls outside it.

## Verdict

1. Cost is level: r costs $7.61 against f's $7.45 a plan, and the rationale adds nothing measurable to fork0's cost.
2. The rationale did not soften the critical fixer: no wrong disposition in 60, no real finding disputed, and both uses of the rationale were right.
3. It did not help the fixer either: it was cited twice, both where the code or decision record alone settled the finding, and it changed no outcome compared with arm f.
4. The final plans are level on i and slightly worse on w and p (10 key items repeated or partly repeated against 8), all from fork0's writing, which the rationale cannot reach through the fixer.
5. On three runs there is no case for the rationale file; arm f, fork0 without it, stays the simpler arm.

## Outcome (operator, 2026-10-14)

The Twill design to build is fork0, arm f of the [fork0 bench](twill-fork0-bench.md).
The master explores, and fork0 writes the plan files.
The master then reads the plan without fork0's reasoning, spawns the lens forks, and judges and fixes as a strict critical fixer.
The operator holds the Burler-style fixer prompt, as benched in the [fixer bench](twill-fixer-bench.md), to be essential to that step.
There is no rationale file.
One extra lens is added for the blind spot every lens shared across the three benches: branches with no test row that reaches them (KI2, KI4, KW4 and the like).
