# twill fork timing, Plan-Write: spawn after writing against a writer fork (fork0)

2026-10-13, prime `/home/knatte/Code/loomyard-LYXHUB/loomyard`, board note `twill-fork-timing`.
Segment: Plan-Write, twill `opus[medium]`, two fresh runs per arm on one reference.
The operator has decided that the lens forks spawn after the plan is written; this bench asks whether a writer fork (fork0) on top of that is worth it.

## Arms

- **after**: the writer explores, writes the plan, runs `lyx loom validate-plan` until it passes, writes the ready marker, then spawns the four lens forks.
  The fork boilerplate has a fork review the files it inherited instead of re-reading them.
  The directive is wave 1's after-arm directive, byte for byte.
- **fork0**: the writer (master) explores, then spawns one fork, fork0, whose whole prompt is to think the plan through, write it, run `validate-plan` until it passes and return one line with no reasoning.
  The master then reads the written files fresh, runs `validate-plan`, writes the ready marker and spawns the same four lens forks.
  Then it judges the findings, fixes the plan, records the lens review and runs `validate-plan` again until it passes.
  The lens-fork boilerplate is the after arm's, with "as the writer read them after they were written" in place of "as the writer wrote and edited them".
- Same model and effort (`opus[medium]`), same lenses (`criteria-coverage`, `row-reachability`, `forced-edits`, `test-economy`), same cited-rejection rule.
- Spawn before writing (`twill-plan-bench.md` Round 2, `p2r-tw-1`, `p2r-tw-2`) is the older reference point, run on an older binary.

## Method

- Reference: `run-time-by-role` (Round 2's reference 2, the fair test), code base `c00b2e524`, frozen records weft `8f5c017a`, key J1-J9 as in Round 2.
- Pairs `twb-ftq-a-{1,2}` (after) and `twb-ftq-f-{1,2}` (fork0), set up as in wave 1 and Round 2:
  - `lyx fabric add` with a `twb-*` note;
  - code branch reset to the base, `_lyx/discussion` and `_lyx/reviews/discussion` checked out from `8f5c017a`;
  - pair `loom.yaml` `plan: opus[medium]`, `plan_producer: single`;
  - pair `shuttle.yaml` `claude:` pointing at the scratchpad wrapper, `claude_deny_agent_tool: false`;
  - seed, two `lyx loom step` calls, `status.json` moved onto Plan-Write, `lyx loom pause --after Plan-Write`, `lyx loom step --until-stop`.
- The wrapper (wave 1's, under `ftq/`) picks the directive by pair name.
  It splices the directive into the run's `prompt.md` as `## Step 2b` before Step 3, sets `CLAUDE_CODE_FORK_SUBAGENT=1`, and appends the code-base notice and a one-line pointer to the system prompt.
  The splice log shows the right directive in each of the four prompts.
- Binary: prod `lyx` installed 15:03 local, after `b1c731a2` (15:02).
  Shuttle's in-flight hold worked: every run ended after its lens review, the gap that ended wave 1 early.
- Waves: wave 1 (`a-1`, `f-1`) in parallel from 15:37 local; wave 2 (`a-2`, `f-2`) from 15:51, once wave 1's $9.28 was known.
- Cost comes from the transcripts, deduplicated by message id and keeping each id's last line, priced at the Opus 5.5 row of `tools/tokencount/cost.go`.
  Each fork's copy of the spawn message is excluded.
  The orch is excluded.
- Scripts: `usage.sh`, `sum.sh`, `lens.sh`, `seq.sh`, `wspan.sh`, plus new `ctx.sh` (a fork's inherited context at spawn and its cache totals) and `vp.sh` (every `validate-plan` call, pass or fail).
- Evidence: `.scratch/twill-fork-timing-plan/after2/ftq-a-{1,2}/` and `fork0/ftq-f-{1,2}/` in the hub prime.
  Each holds the transcripts, the final plan, `lens-review.md` and the traces.
- Grading: one grader session read the four final plans, the 16 lens reports and the lens-review files against the key, reading code at the base.

## Results

Every run wrote `.lyx/twill-plan/lens-review.md`, after its forks returned, and ended on a passing `validate-plan`.
All four runs are valid.

| Run | Arm | Writer | fork0 | Lens forks | Total | Writer time | Writer msgs | Cards |
|---|---|---|---|---|---|---|---|---|
| a-1 | after | $4.00 | | $1.00 | $5.00 | 12m21s | 56 | 4 |
| a-2 | after | $3.33 | | $0.87 | $4.20 | 10m27s | 46 | 4 |
| f-1 | fork0 | $2.19 | $1.07 | $1.01 | $4.28 | 11m47s | 41 | 5 |
| f-2 | fork0 | $2.58 | $1.07 | $0.87 | $4.52 | 9m25s | 50 | 5 |
| p2r-tw-1 (Round 2) | before | $3.63 | | $1.69 | $5.32 | 11m17s | 53 | 5 |
| p2r-tw-2 (Round 2) | before | $3.88 | | $1.50 | $5.38 | 12m17s | 59 | 5 |

Writer time is the writer's first to last assistant message; fork0's time falls inside it.
Round 2's costs here are recomputed with the same scripts, last line per message id.

| Run | Lens context at spawn | Lens cache write per fork | Lens cache read per fork | Lens tool calls after the wait | `validate-plan` calls (fail / pass) |
|---|---|---|---|---|---|
| a-1 | 193k | 5.6k | 677k | 6 | writer 1 / 2 |
| a-2 | 187k | 5.2k | 560k | 4 | writer 1 / 3 |
| f-1 | 133k | 8.3k | 469k | 7 | fork0 1 / 1, master 1 / 2 |
| f-2 | 153k | 5.2k | 497k | 5 | fork0 1 / 1, master 1 / 3 |
| p2r-tw-1 | 150k | 23.8k | 925k | | |

| Arm | Mean writer | Mean fork0 | Mean lens forks | Mean total | Writer + fork0 output tokens |
|---|---|---|---|---|---|
| after | $3.67 | | $0.94 | $4.60 | 76k, 61k |
| fork0 | $2.39 | $1.07 | $0.94 | $4.40 | 48k, 52k |
| before (Round 2) | $3.76 | | $1.60 | $5.35 | |

- **Cost:** fork0 runs $0.20 a run cheaper (4 %), well inside the spread of either arm ($4.20-$5.00 after).
  Writer plus fork0 ($3.26, $3.65) is close to the after writer ($4.00, $3.33).
  The pair writes about a third fewer output tokens in total: fork0 writes 23-26k, the master 25-26k.
- **Lens forks:** they cost the same in both arms ($0.87-$1.01 per four).
  Their inherited context is 40-55k tokens smaller in the fork0 arm (133k and 153k against 187k and 193k), but that context is a cache read at spawn.
  Each fork's first message writes about 2k in both arms, so the smaller prefix saves only cache-read cents, and f-1's forks made more calls.
  Both arms keep the after-arm saving over spawning before writing: 5-8k cache write per fork against 21-24k, and $0.94 against $1.60 per run.
- **Master cache:** it stayed warm through fork0's run (4m20s and 4m56s).
  In f-1 the master's first message after fork0 returned wrote 651 tokens and read 120k.
  The shared prefix is kept warm by fork0's own turns, so no `lyx shuttle wait` is needed.
  A fork0 that ran much past five minutes would put this at risk.
- **Exploration moved into fork0:** the masters spawned fork0 after 1.5-2.5 minutes and 21-27 read calls.
  The after writers explored 5.5-6 minutes and 28-37 calls before their first write, the difference mostly thinking between calls.
  fork0 then made 4-7 reads of its own.
- **Writer time:** about the same, 9m25s-11m47s against 10m27s-12m21s.
- **`validate-plan` rounds:** the fork0 arm runs more (5-6 against 3-4), since fork0 and the master each validate.
  Each master's post-review fix failed `validate-plan` once, as did a-1's and a-2's pre-ready writes.

## Grading

Key J1-J9 as in `twill-plan-bench.md` Round 2; R repeated, P partly, A avoided, n/a another route.

| Key | a-1 | a-2 | f-1 | f-2 |
|---|---|---|---|---|
| J1 repeated seed rows | P (the remaining `DecodeSeed` success row repeats `started_at` and the absent driver) | A (only `TestValidateDriver` rows) | A (only the `"bogus"` driver row, covering tests named) | R (keeps the legacy-`parent` and default-driver rows, drops the refusal rows, following a test-economy finding) |
| J2 parse abort, seed-less status | P (`at` and no seed; no `started_at`, `last_at`) | P (same) | P (`at`, `started_at`, no seed; no `last_at`) | P (same) |
| J3 "All runs" sum vs union | A (overlapping runs, sums asserted) | A | A | A |
| J4 shedrun constructor claim | P (`doc.go` for the new pair only; `paths.go` claimed to stay true) | P (exception for the new pair only) | P (same) | P (same) |
| J5 run-id error wrapping | A (slug, tag, file) | A | A (slug, tag, both files) | P (file dropped in the fix) |
| J6 cwd parent vs worktree parent | R | R | R | R |
| J7 missing-commit skip row | n/a | P (malformed lines unnamed) | n/a | R (only "a line without a date" row) |
| J8 `orch_test.go` unlisted | A | A | A | A |
| J9 bare `weft` in tests | A (`"-" + repositoryFlag`) | A | P (asserts an error "naming -weft", spelling open) | A |
| **R / P** | **1 / 3** | **1 / 3** | **1 / 3** | **3 / 3** |

Means: after 1.0 / 3.0, fork0 2.0 / 3.0.
Round 2's spawn-before runs scored 2 / 3 and 2 / 4, but on an older binary whose `validate-plan` had no fabric-vocabulary check, so its J9 R does not compare.
f-2's two extra R come from the lens review: J1 from a wrong test-economy finding the master followed, J7 from a row set fork0 left.

New defects outside the key:

- a-2: `checkHubRepository` calls `filepath.Abs(hub)`, a raw cwd query that breaks `PATTERN-cwd-resolution`, and no lens flagged it.
  f-1's forced-edits fork flagged the same call BLOCKING and the master fixed it; a-1's writer avoided it on its own.
- a-2: cards 2 and 4 rewrite `tools/tokencount/main.go`'s package doc without listing the file.
- f-2: `00-overview.md` lacks the `language: go` frontmatter.
- a-1, a-2 and f-2 declare a new one-method `FileAtRevision` interface instead of reusing `PlanHistory`; only f-2's forced-edits fork flagged it, and f-2 fixed it.
- No test-budget violation: every plan lists `test-budget.yaml` and its counts hold.

## Lens findings

Severity words in each fork's final report, B BLOCKING / M MEDIUM / total.

| Lens | a-1 | a-2 | f-1 | f-2 |
|---|---|---|---|---|
| criteria-coverage | 0 / 2 / 6 | 0 / 3 / 8 | 0 / 2 / 6 | 0 / 3 / 7 |
| row-reachability | 0 / 0 / 4 | 0 / 1 / 4 | 0 / 3 / 4 | 0 / 2 / 4 |
| test-economy | 0 / 0 / 4 | 0 / 0 / 4 | 0 / 0 / 5 | 0 / 0 / 4 |
| forced-edits | 0 / 0 / 2 | 0 / 0 / 3 | 1 / 0 / 4 | 0 / 1 / 4 |
| Total | 0 / 2 / 16 | 0 / 4 / 19 | 1 / 5 / 19 | 0 / 6 / 19 |
| Dispositions (fixed / rejected) | 11 / 5 | 12 / 2 | 15 / 2 | 13 / 2 |

Every fork returned a report and none re-read a plan file.

Found only by the fork0 arm's lens forks:

- The `filepath.Abs` cwd query (f-1 forced-edits, BLOCKING); a-2 carries the same defect unflagged.
- An unstated premise: the archived paths assume records sit at the tree root (`AnchorRel`), and no card says so (f-2 forced-edits, LOW).
  All four plans assume it; only this fork named it.
- Reuse: `ArchiveReader` duplicates `PlanHistory` (f-2 forced-edits); a-1 and a-2 have the same duplicate.
- A guard the plan calls unreachable is reachable: slug `.` passes `CountRun` and fails `ValidateRunID` (f-1 criteria-coverage, MEDIUM).
  a-1's writer called the same guard an unreachable wiring guard, and its forks accepted that.
- Fixture premises: a `live` row that passes without the in-flight rule and a `broken` row that dies in `CountRun` first (f-1 row-reachability).
  Also two tags whose ref order matches their date order cannot pin newest-by-date (f-2 criteria-coverage).

Found only by the after arm's lens forks: a persistent `t.Setenv("GIT_COMMITTER_DATE")` that makes the newest-tag pin flaky (a-2, three lenses, MEDIUM), stale flag help and file comments (a-2 forced-edits), an unreached empty-`Params` check (a-1 row-reachability).

- Findings that a card leaves something unsaid came from both arms at a similar rate: how the refusal test gets `wd` (a-1), which file holds a test (a-2), that the bad-seed row needs a valid status (a-2).
  So "blind forks catch unstated premises" does not separate the arms here.
- The fork0 arm logged more BLOCKING or MEDIUM findings (6 and 6 against 2 and 4), and only it flagged reuse and the tree-root assumption.
  Part of that may be that fork0's plans had more to find.

Fixing without the reasoning:

- f-2: fork0's `..` row demanded an error holding "the file's relative path", which a refused id cannot have.
  Three forks flagged it and pointed at `shedrun`'s refusal text, which names the file.
  The master instead dropped the file from the error, weakening Decision 9 (J5 P); a-2, whose writer held its own reasoning, kept the file through `shedrun`'s error.
  This is the one case where the master resolved a contradiction against intent it did not have.
- f-2: following a wrong test-economy finding into J1's R is the lens's error, not missing reasoning.
- f-1: every fix matches the record's intent.

## Verdict

- Cost: no difference that two runs can show.
  fork0 is $4.40 against $4.60 per plan; the writer's thinking moves into fork0, and the lens forks cost the same.
  The predicted net-zero cost effect holds.
  Both arms keep spawn-after's saving over spawn-before ($0.94 against $1.60 in the lens forks).
- Quality on the key: level or slightly worse for fork0 (2.0 / 3.0 against 1.0 / 3.0 R / P), all of the gap in one run (f-2) and caused in its lens review.
- Lens forks judging the files blind: they raised more BLOCKING and MEDIUM findings, including the one BLOCKING defect an after-arm plan shipped and one unstated assumption no after-arm fork named.
  The master fixing without fork0's reasoning cost one decision's fidelity in f-2.
- Two runs per arm on one reference: a direction, not an effect size.
  The direction is no cost gain, a possible gain in lens sharpness, and a risk in fixing without the intent; it does not justify building fork0 on this evidence.
  If it is benched further, the cheap change is to have fork0 also write a short rationale file the lens forks do not inherit but the master reads before fixing.

## Deviations

- The arms run Round 2's prototype route (`plan_producer: single` plus a spliced Step 2b), as wave 1 did, not today's `plan_producer: twill`.
  So `b1c731a2`'s lens-review gate did not apply, and the lens-review file sits in `.lyx/twill-plan/`, not the plan directory.
  Each run's lens review was checked by hand.
  The shuttle in-flight hold from the same commit did apply.
- One reference (`run-time-by-role`) for all four runs; wave 1 of the earlier bench ran one after-arm run on each reference.
- The fork0 masters explored less than the after writers before handing off (see Results), so fork0 read 4-7 more things itself; the directive told the master to explore until it held what the plan needs, and it judged that sooner.
- The wrapper was written through a shell heredoc into the scratchpad, not with an editor tool; it is a scratch file outside the repository.
- Grading is one grader session's reading; Round 2's grades came from another session.
- The orch approved no parent review in these runs (Plan-Write paused before Plan-Review) and its cost is excluded.
- Torn down: all four pairs with `lyx fabric remove` (finished on the second call each), all four `twb-ftq-*` notes; no worktree, branch or tmux session of them is left.

# Round 3: fork0 stays alive

2026-10-13, the operator's next arm from the board note (section "Next arm to bench: fork0 stays alive and revises").
fork0 writes the plan and stays alive to judge the lens findings and revise the plan with its own reasoning, while the lens forks stay blind to that reasoning.

## Arm

- **alive**: the master explores, then spawns fork0 and waits on `lyx shuttle await` for fork0's ready marker.
  fork0 writes the plan, runs `validate-plan` until it passes, creates the findings directory and writes the ready marker.
  The master then reads the plan files (so the lens forks inherit them) and spawns the same four lens forks, then ends its turn; it never reads a finding and changes no plan file.
  Each lens fork writes its report with one Write call to `.lyx/twill-plan/findings/lens-<name>.md` and returns only that file's path.
  fork0 runs the scratch merge script `alive/merge.sh` in a loop: it waits up to 240 s for all four lens files, concatenates them into `findings.md` and prints `ready: true`, or prints `ready: false` for fork0 to re-run it in a turn that does nothing else.
  fork0 then judges every finding under the same cited-rejection rule, revises, writes `lens-review.md`, runs `validate-plan` until it passes, and ends; the master's last turn only ends.
- Everything else is the fork0 arm's: reference, base, records, lenses, fork boilerplate (bar the report going to a file), the prototype route and the pair setup.
  The directive is `directive-alive.md` beside the wrapper `ftr/wrap-twill.sh` in the session scratchpad; the merge script uses only shell builtins, `cat` and `mv`.

## Method

- Pairs `twb-ftr-1` and `twb-ftr-2`, set up exactly as `twb-ftq-*` (base `c00b2e524`, records `8f5c017a`, `plan: opus[medium]`, `plan_producer: single`, the same status edits and pause), run in parallel from 16:52 local.
- Binary: prod `lyx` installed 16:44 local, after the Round 2 runs.
- Same scripts as above, plus `alive/f0.sh`: fork0's messages from its first merge call on, with each message's cache write and read, to show whether its cache stayed warm across the wait.
- Lens findings are counted from the lens files (severity per finding heading), not from a final message.
- Grading: one fresh grader session graded all six plans (`a-1`, `a-2`, `f-1`, `f-2`, `r-1`, `r-2`) against J1-J9, so the grades compare; it re-graded the four earlier plans, and a few of their cells differ from the earlier grader's.
- Evidence: `.scratch/twill-fork-timing-plan/alive/ftr-{1,2}/` (transcripts, final plan, `findings/`, `lens-review.md`, traces), plus `alive/merge.sh` and `alive/f0.sh`.

## Results

Both runs ended on a passing `validate-plan` with a lens review written by fork0; the master read no finding and wrote no plan file.

| Run | Master | fork0 | Lens forks | Total | Writer time | fork0 merge calls | fork0 cache write on waking | `validate-plan` (fail / pass) | Lens B / M / total | Fixed / rejected |
|---|---|---|---|---|---|---|---|---|---|---|
| r-1 | $2.00 | $1.70 | $1.25 | $4.94 | 10m59s | 1 | 313 tokens (read 184k) | fork0 1 / 3 | 2 / 2 / 16 | 15 / 0 |
| r-2 | $2.04 | $2.36 | $1.40 | $5.80 | 11m56s | 1 | 312 tokens (read 205k) | fork0 1 / 3 | 0 / 5 / 22 | 18 / 2 |

| Arm | Mean master or writer | Mean fork0 | Mean lens forks | Mean total | Lens B / M per run | Key R / P (one grader) |
|---|---|---|---|---|---|---|
| after | $3.67 | | $0.94 | $4.60 | 0 / 2, 0 / 4 | 1 / 3, 1 / 3 |
| fork0 | $2.39 | $1.07 | $0.94 | $4.40 | 1 / 5, 0 / 6 | 1 / 3, 3 / 4 |
| alive | $2.02 | $2.03 | $1.33 | $5.37 | 2 / 2, 0 / 5 | 2 / 3, 2 / 2 |

- **fork0's cache stayed warm:** each merge call returned `ready: true` after about two minutes, inside one call, and fork0's next message wrote 313 and 312 tokens against 184k and 205k read.
  The 240 s cap was never reached.
- **The master's cache stayed warm too:** it awaited fork0's marker in two `lyx shuttle await` calls per run, and its turns after the lens spawn (woken by each lens fork's one-line return) wrote under 610 tokens each, its final turn included.
  The lens forks' returns cost the master about $0.03 a wake.
- **fork0's revise phase** cost about $0.56 and $0.78 (from waking to its end), against the fork0 arm's masters, who did the same work on top of their exploration context.
- **Lens forks cost more** ($1.25, $1.40 against $0.87-$1.01): their inherited context was larger (163k and 171k against 133k and 153k, since these masters explored longer before the spawn: fork0's context at spawn was 149k and 156k against 120k and 139k), and writing the report as a file adds output and a cache write (10-12k write per fork against 5-8k).
  That is spread, not arm: nothing in the arm makes the inherited context larger.
- **Writer time** is level with both earlier arms (11-12 minutes).
- **Mechanics held:** every lens fork made exactly one Write to its own file and nothing else; fork0 never read a single lens file; neither run hit the merge cap or a relaunch.

## Grading

One grader session, all six plans; R repeated, P partly, A avoided, n/a another route.

| Key | a-1 | a-2 | f-1 | f-2 | r-1 | r-2 |
|---|---|---|---|---|---|---|
| J1 repeated seed rows | P | A | A | R | R (keeps the `{"recipe":"loom"}` default row a test-economy finding suggested, which `TestReadSeed_AbsentDriverDefaultsToGo` already reaches) | P (`parent`, unknown key, absent driver repeat) |
| J2 parse abort, seed-less status | P | P | P | P | P (`started_at` has no abort path) | A (`at`, `last_at`, `started_at` and a seed-less status, each a named row) |
| J3 "All runs" sum vs union | A | A | A | A | A | A |
| J4 shedrun constructor claim | P | P | P | P | P (`doc.go`, new pair only) | R (the "every constructor" claim stays, `paths.go` untouched) |
| J5 run-id error wrapping | A | A | A | P | P (slug and tag, "since no file path exists") | P (declared unreachable, file part undefined) |
| J6 cwd parent vs worktree parent | R | R | R | R | R | R |
| J7 missing-commit skip row | n/a | P | n/a | R | n/a | n/a |
| J8 `orch_test.go` unlisted | A | A | A | A | A | A |
| J9 bare `weft` in tests | A | A | P | P | A | A |
| **R / P** | **1 / 3** | **1 / 3** | **1 / 3** | **3 / 4** | **2 / 3** | **2 / 2** |

Sums of R + P: after 8, alive 9, fork0 11.

New defects outside the key in the alive plans:

- r-1: `run` calls `filepath.Abs`, the raw cwd query a-2 also shipped; no r-1 lens flagged it.
- r-1: an unparseable `started_at` has no abort path, which breaks Decision 9.
- r-1 and r-2: the `seed.go` header comment stays stale; r-1's fix of a forced-edits finding about it dropped the edit instead of retargeting it.
- r-1 and r-2 add a one-method `FileAtRevision` interface that `PlanHistory` covers, as a-1, a-2 and f-1 do; only f-2 reuses `PlanHistory`.

fork0's judgment of the findings:

- r-1 rejected nothing; r-2 rejected two, and the grader judged both rejections right (one cites `main.go:180-187` against "abort pinned only at `readArchivedRun`", the other keeps an archived-status test out of the calibration test, whose header scopes it to calibration parity).
- r-1 followed a wrong test-economy suggestion into J1's R, the same failure f-2's master had.
- r-1 dropped the file from Decision 9's run-id error ("no file path exists"), the same J5 miss f-2's master made; r-2 declared the refusal unreachable, with sound reasoning, but left the wrapping undefined.
- r-2's merged J2 fix made it the only plan of the six with J2 at A.

## Verdict

- Cost: the alive arm is the dearest, $5.37 against $4.40 (fork0) and $4.60 (after) per plan.
  Of the gap, about $0.40 is lens-fork spread from larger inherited contexts, not the arm; the rest is fork0's revise phase ($0.56-$0.78), which the fork0 arm's master did more cheaply on its own warm context.
  Keeping fork0 alive costs nothing in cache: the merge loop kept fork0 warm (about 300 tokens written on waking), and the master stayed warm through its awaits and lens-fork wakes.
- Quality: level with the other arms on the key (alive 9 R + P, after 8, fork0 11, two runs each).
  The fix that the fork0 arm was thought to lack, fixing with the writer's own reasoning, did not show: r-1 repeated both of f-2's lens-review misses (a wrong test-economy finding followed into J1, and the file dropped from Decision 9's error), and fork0's rejections were few and right in both arms.
  The misses look like lens errors and plan judgment, not lost intent.
- Lens findings: about as many BLOCKING and MEDIUM as the fork0 arm (2 / 2 and 0 / 5 against 1 / 5 and 0 / 6), so the blind lens forks stay as sharp; r-1's lenses missed the `filepath.Abs` call f-1's caught.
- The mechanism works as a prototype (file-per-lens, a mechanical merge, a warm wait), but two runs show no quality gain to pay $0.75-1.00 a plan for.
  On this evidence, spawn-after with the writer fixing (the after arm) stays the cheapest arm with level quality, and neither fork0 variant earns building.

## Deviations, Round 3

- The same prototype route and single reference as Rounds 1-2 of this bench.
- The master read the plan files after the ready marker so the lens forks inherit them, as the fork0 arm did; the brief left this implicit.
- The master's last turns are woken by each lens fork's one-line return; it was told to take no action on them.
- The grader re-graded the four earlier plans in the same session, so one cell here differs from the earlier table (f-2 J9 P where it was A, making f-2 3 / 4); the sums compare within this table only.
- The orch is excluded from all costs; the grader session is not counted in the runs' cost.
- Torn down: `twb-ftr-1` and `twb-ftr-2` with `lyx fabric remove` (finished on the second call each) and both board notes; no worktree, branch or tmux session of them is left.
