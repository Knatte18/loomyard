# twill-plan bench, sonnet arm

2026-10-11, prime `/home/knatte/Code/loomyard-LYXHUB/loomyard`.
One arm: twill with the plan writer on `sonnet[high]` (forks inherit it), two runs per reference, set beside round 2's single `opus[high]` and twill `opus[medium]` (see `twill-plan-bench.md`, Round 2).
Run in parallel with the round-3 twill `opus[high]` arm, which this record does not touch.

## Setup

- Exactly round 2's setup: the same directive (`round-sonnet/directive.md`, byte-identical to `twill-plan-bench/round-2-directive.md`), the same four lenses, the same cited-rejection rule, the same Step 2b splice into `prompt.md` and the same code-base notice.
  The wrapper `round-sonnet/wrap-twill.sh` is round 2's with its log and directive paths under the scratchpad's `rs/`.
- Same mechanics: one `twb-son-*` note per pair, `lyx fabric add`, code branch reset to the base, frozen records checked out with `git checkout <weft sha> -- _lyx/discussion _lyx/reviews/discussion`, `lyx fabric commit`, `lyx shed seed self --recipe loom --driver go --param parent=main`, two `lyx loom step` calls, `status.json` edited onto Plan-Write with Discussion-Write and Discussion-Bouncer done, `lyx fabric commit`, `lyx loom pause --after Plan-Write`, `lyx loom step --until-stop`.
- Pair `loom.yaml` `plan: sonnet[high]`; pair `shuttle.yaml` `claude:` the wrapper and `claude_deny_agent_tool: false`.
  The writer process ran `--model sonnet --effort high`, and every writer and fork message is `claude-sonnet-5-5`.
- Reference 1 (`shuttle-startup-ready`): base `a35c8c2b1`, records `995a85ba`, pairs `twb-son-s-1`, `twb-son-s-2`.
  Reference 2 (`run-time-by-role`): base `c00b2e524`, records `8f5c017a`, pairs `twb-son-r-1`, `twb-son-r-2`.
- Wave 1 (s-1, r-1) ran from 20:28 local; wave 2 (s-2, r-2) from 20:43, once wave 1's cost showed the cap would hold.

## Results

Cost and writer turn time from `go run ./tools/tokencount` (report in `twill-plan-bench/tokens-sonnet.md`), orch excluded.

| Ref | Run | Writer | Forks | Total | Writer time | Messages | Cards |
|---|---|---|---|---|---|---|---|
| 1 | s-1 | $2.97 | $1.09 | $4.06 | 8m48s | 57 | 5 |
| 1 | s-2 | $3.57 | $1.30 | $4.87 | 8m35s | 60 | 5 |
| 2 | r-1 | $3.91 | $1.30 | $5.21 | 13m11s | 57 | 8 |
| 2 | r-2 | $3.82 | $1.12 | $4.94 | 11m13s | 64 | 7 |

Arm spend $19.09 of the $20 cap ($14.27 writers, $4.82 forks), orch excluded; tokencount attributes $7.83 of orch to the window, which holds the parallel round-3 agent and the hub orch as well.
Sonnet averages $4.77 per plan against $5.24 for twill `opus[medium]` (9% less) and $2.63 for single `opus[high]`.
The per-token price gap does not carry through: the sonnet writers emit more output (65k-107k tokens against 59k-70k) and read more cache (8.5M-11.3M against 6.7M-9.8M).

## Grading

Same keys and method as round 2: R repeated, P partly, A avoided, n/a another route.

### Reference 1 (`shuttle-startup-ready`)

| Key | s-1 | s-2 |
|---|---|---|
| K1 no-tail rows | A (`skillReed.ReadyCapture` gives the skill-load death row a ready screen; `TestStartup_MechanismFailure` row with status failing after a capture) | A (`skillReed.Banner`; mechanism row with a capture, then a foreign strand to the retry cap) |
| K2 new top-level test | R (new `TestStartup_GateDismissalGap` beside `TestStartup_UndismissableGateUntilWindowExpires`) | R (same) |
| K3 capture until first send | A (constant ready capture) | A (same) |
| K4 middle state | A | A |
| K5 stale keep reason | A (reworded) | A (reworded) |
| K6 test package vs glyph | A (file-only `Create`, package `claudeengine_test`) | A (`claudeengine#` glyph in `session_test.go`, package `claudeengine`) |
| N1 start-level criterion | P (start test drives the color and prompt sends; the skill-load send is waived, see Rejections) | A (color, skill-load and prompt sends, with a `Stop` hook line written on the skill-load paste) |
| **R / P** | **1 / 1** | **1 / 0** |

| Arm | Runs (R / P) | Mean R / P | Cost per plan | Writer time |
|---|---|---|---|---|
| single `opus[high]` (round 2) | 1 / 1, 1 / 0 | 1.0 / 0.5 | $2.58 | 6m10s |
| twill `opus[medium]` (round 2) | 0 / 1, 1 / 0 | 0.5 / 0.5 | $5.56 | 9m40s |
| twill `sonnet[high]` | 1 / 1, 1 / 0 | 1.0 / 0.5 | $4.47 | 8m42s |

### Reference 2 (`run-time-by-role`)

| Key | r-1 | r-2 |
|---|---|---|
| J1 repeated seed rows | R (new `TestDecodeSeed` with `started_at`, `parent` and unknown-key rows; the test-economy finding was rejected) | A (rows cut to bytes-in, empty params key and malformed JSON; names the `ReadSeed` tests that cover the rest) |
| J2 parse abort, seed-less status | P (status without seed row; a bad `at` is skipped, not aborted; no `started_at` or `last_at` row) | A (bad `started_at` and bad `at` abort rows naming slug, tag and file; no-seed row) |
| J3 "All runs" sum vs union | P (two runs summed, no wall-clock overlap) | P (same, on hand-set `Time`) |
| J4 shedrun constructor claim | R (`doc.go` gains a paragraph, the claim stays, `paths.go` header unlisted) | P (`doc.go` paragraph and `paths.go` header list amended; the claim itself unaddressed) |
| J5 run-id error wrapping | A (wrapped with slug, tag and file) | A (same, with a row) |
| J6 cwd parent vs worktree parent | R (`sameHub` against the parent of `wd`) | R (`checkHubMatchesWorktree` against the parent of `wd`) |
| J7 missing-commit skip row | A (junk-line row "a line without a sha") | A (`archive/no-sha` row) |
| J8 `orch_test.go` unlisted | A (`TestCountOrch` edited) | A (same) |
| J9 bare `weft` in tests | R (`TestRunRefusesAForeignHubWithoutWeft`, "an error naming -weft") | A (assertions on `"-" + repositoryFlag`, no test name or literal spells `weft`; follows the base's own `calibrate_integration_test.go`, the scan is never named) |
| **R / P** | **4 / 2** | **1 / 2** |

| Arm | Runs (R / P) | Mean R / P | Cost per plan | Writer time |
|---|---|---|---|---|
| single `opus[high]` (round 2) | 6 / 1, 3 / 4 | 4.5 / 2.5 | $2.68 | 7m28s |
| twill `opus[medium]` (round 2) | 2 / 3, 2 / 4 | 2.0 / 3.5 | $4.93 | 10m41s |
| twill `sonnet[high]` | 4 / 2, 1 / 2 | 2.5 / 2.0 | $5.08 | 12m12s |

r-2 is the best reference-2 plan of any arm so far (1 R); r-1 sits between the two round-2 arms.
J6 survived again in both runs; no lens names Decision-wording fidelity of a comparison.

## Lens finding counts

Severity words in each fork's final message (B BLOCKING, M MEDIUM, total), counted as in round 2.

| Lens | s-1 | s-2 | r-1 | r-2 |
|---|---|---|---|---|
| criteria-coverage | 0 / 3 / 9 | 0 / 1 / 6 | 0 / 3 / 7 | 0 / 2 / 9 |
| row-reachability | 0 / 2 / 5 | 0 / 0 / 8 | 0 / 3 / 7 | 0 / 4 / 10 |
| forced-edits | 1 / 1 / 3 | 1 / 0 / 2 | 1 / 2 / 6 | 0 / 1 / 7 |
| test-economy | 1 / 2 / 10 | 1 / 1 / 5 | 0 / 0 / 6 | 0 / 1 / 7 |
| Total | 27 | 21 | 26 | 33 |
| Dispositions (fixed / rejected) | 16 / 7 | 16 / 4 | 20 / 3 | 23 / 4 |

Sonnet forks report more findings than round 2's opus forks (21-33 against 14-23), mostly LOW, and every fork returned a report.

## Behavior

- Spawn point: every writer spawned its four forks in one message after Step 2 and before its first plan write, as round 2's did.

| Run | First Agent call | Fourth Agent call | First write under `_lyx/plan/` |
|---|---|---|---|
| s-1 | 18:32:18 | 18:32:30 | 18:33:58 |
| s-2 | 18:47:53 | 18:48:06 | 18:49:28 |
| r-1 | 18:33:14 | 18:33:27 | 18:33:45 |
| r-2 | 18:47:03 | 18:47:15 | 18:49:18 |

  Times are UTC.
- `validate-plan`: every writer ended on a passing run; the first run passed only in s-1.
  s-2 and r-2 first failed on a `Uses` entry, r-1 and r-2 on `delete-before-reference`, and r-2 also on a verify line reaching the fabric repository; each was fixed in one or two more runs.
  r-2's card 7 uses a `**Prosa:**` field that the validator accepted.
- Rejections: every cited `path:line` checked out at the base (spot-checked `wait.go:543-561`, `wait.go:583-613`, `wait.go:203-208`, `run.go:746-756`, `startup_test.go:59-64`, `tools/tokencount/main.go:167`, `count.go:420-425`, `count.go:483-495`, `seed_test.go:175-219`, `calibrate_test.go:22`).
  Two dispositions kept a key defect, which round 2's opus writers never did:
  - s-1 marked criteria-coverage's MEDIUM (the skill-load send untested) "fixed in part" and waived the skill-load send because "a skill turn needs a provider events file this fake cannot honestly produce", citing `loadSkillTurn` with no `path:line`; s-2 and round 2's plans script that events file, so the premise is wrong and the waiver is an uncited rejection under the rule.
  - r-1 rejected test-economy's J1 finding citing `internal/shedrun/seed_test.go:175-219`; the citation is accurate (those tests drive `ReadSeed` over a file), but `DecodeSeed` is the same `decodeSeed`, so the rows still repeat.
- r-2's forced-edits fork found a defect outside the key: a Webster fork's transcript repeats its parent's lines with their timestamps, so a `+sub` union would hold the parent's pre-fork time; the writer read a real subagent transcript to confirm it and planned a dedup.

## Verdict

- Twill `sonnet[high]` is competitive on quality: on reference 1 it ties single `opus[high]` (1.0 / 0.5) and trails twill `opus[medium]` by half a repeat; on reference 2 it repeats more than twill `opus[medium]` (2.5 against 2.0) but leaves fewer partial (2.0 against 3.5), and both beat single `opus[high]` (4.5 / 2.5).
  The spread inside the arm (1-4 R on reference 2) is wider than any gap to twill `opus[medium]`.
- It is not cheaper in a way that matters: $4.77 per plan against $5.24 (9% less), and writer time is about the same overall (10m27s against 10m10s), longer on reference 2.
- Its judging is weaker: two of 18 rejections kept a key defect, one on a wrong premise without a line citation, against none of round 2's six; sonnet forks also produce more, lower-severity findings for the writer to judge.
- Recommendation: no reason to prefer `sonnet[high]` over `opus[medium]` for the twill Plan-Write writer; the cost saving is small and the rejection discipline slips.
  Two runs per reference and one grader: a direction, not an effect size.

## Deviations

- Prod moved twice during the arm, by other sessions' landings: `86982fa43` at the start, `b2e4cf913` at 20:30 and `3a627c00d` at 20:37 local.
  Wave 1's `lyx loom step` processes started on `86982fa43`, and their `validate-plan` calls ran a newer binary; wave 2 ran on `3a627c00d`.
  Between `86982fa43` and `3a627c00d`, `internal/planparser` and `loom-template-plan.md` are unchanged; `loom-rubric-plan-review.md`, which the forks read, changed only in its leading comment and one sentence on the fixer's `fix` key; round 2 records prod `c279a38ba`.
  Each landing ran a config reconcile into the pairs' weft (`lyx: reconcile config`), which kept `plan: sonnet[high]`.
- The grading is one grader's reading against round 2's keys; P marks a key a plan addresses with a gap the key's finding still names.
- Torn down: all four pairs with `lyx fabric remove --remote` (no worktree or branch left locally or on either origin; records under `archive/twb-son-*` tags in the weft repository), all four notes, no reed strand left.
  Plans, lens-review files, the directive and the wrapper are under `round-sonnet/`, the token report in `twill-plan-bench/tokens-sonnet.md`.
- Afterwards `lyx stencil list` showed all 72 stencils and specs `untouched`; the `_board` stencil and spec commits since 20:20 are the two landings' `lyx: seed` commits, not this arm's; the prime's working tree is clean and `twill-plan-bench.md` and `round-3/` were not touched.
