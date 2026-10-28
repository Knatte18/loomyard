# Codex Astra forked fan as Code reviewer on two darn diffs: two medium runs each

Bench of 2026-10-28, the first Codex bench over code; its siblings reviewed plans (`docs/research/codex-v2-p1-bench-2026-10-27/report.md`).
Codex CLI 0.164.0, ChatGPT sign-in, Linux.
Four `codex exec` handler runs, `gpt-6-astra` at medium effort for handler and forks: two darn diffs, each reviewed twice.
Each handler read the diff, then spawned the three Code-Review lens forks through `spawn_agent` with `fork_turns="all"`.
Nothing was built in `internal/`: the bench is shell scripts under `data/harness/`, the reviewers' own output and a hand grading.

## Conclusion

On these two diffs the forked fan found 2 of the Claude fan's 5 findings on `spawn-text-file-busy` and 3 of its 6 on `fixer-overcap-review`, identically in both runs of each, and none of the fan's one `BLOCKING` finding.
It does not stand in for the Claude Code-Review fan as it ran here.

| Input | Fan findings found, run 1 and run 2 | Fan `BLOCKING` found | Verdict, Codex against fan | Real findings the fan lacks | False findings |
|---|---|---|---|---|---|
| `spawn-text-file-busy` | 2 of 5, 2 of 5 | 0 of 1, 0 of 1 | `BLOCKING`, `BLOCKING` against `BLOCKING` | 1 and 0 | 1 and 2 |
| `fixer-overcap-review` | 3 of 6, 3 of 6 | none | `BLOCKING`, `BLOCKING` against `APPROVED` | 3 and 4 | 0 and 0 |

- The fan's one `BLOCKING` finding, that the start-failed cause does not read `start-failed after <n> tries: <error>` as decision D4 settles, was named by neither run and explicitly rejected by both ("the board snapshot asks for explanatory spawn detail, not this exact format").
  The decision record's `D4` is the writer's own claim under the rubric, and the board entry names no spelling, so the rejection follows the rubric's text; the fan weighed the same record differently.
  Both runs returned `BLOCKING` on a finding of their own, which grading found false, so the matching verdict is a coincidence of severity and not of cause.
- On `fixer-overcap-review` both runs returned `BLOCKING` against the fan's `APPROVED`.
  Their `BLOCKING` findings are real readings of `PATTERN-tolerant-records` and `PATTERN-gate-slot` that the fan did not raise, so the stricter verdict is not a false one; it is the only input where the Codex fan found PATTERN violations the Claude fan missed.
- What both fans find is test-coverage gaps: every shared finding is a missing test row (`MEDIUM` or `LOW`), and the Codex fan folds several of the fan's rows into one finding.
  What the Codex fan does not find is a divergence between the code and the decision record or acceptance criteria: the fan's `BLOCKING` `F1` and `MEDIUM` `F2` on `spawn-text-file-busy`, and `F3`, `F5` and `F6` on `fixer-overcap-review` (a spent-fixer test, a halt-reason tail and two stale docs).
- Run-to-run spread is small on the fan's findings (the same ones in both runs of each input) and larger on the rest: findings per run were 4 and 4, and 4 and 7.
  One grader and two runs per input measure a spread, not a rate; see Limits.

## Inputs

Both are darn runs that landed on `main` on 2026-10-28, with real Go diffs and a round-1 Claude Code review that returned findings.
The runs were picked from the darn runs that landed that day with Go diffs, by the entry's three rules; the two differ in the packages they touch.

| Input | Run | Landing commit | Packages | Reviewed commit | Review range | Fan verdict |
|---|---|---|---|---|---|---|
| `STB` | `spawn-text-file-busy` | `161c7903` | `internal/shedverbs`, `internal/proc`, `tools/deploy` | `7615d129` | `161a4a7f..7615d129`, 10 files, +246 −22 | `BLOCKING` |
| `FOR` | `fixer-overcap-review` | `90d09fc7` | `internal/landingshed`, `contracts/`, `docs/`, `pattern/` | `d50e5277` | `161a4a7f..d50e5277`, 19 files, +705 −48 | `APPROVED` |

The reviewed commit is the task branch tip at the time the Claude round started (15:06 and 15:07 local), before the round's fix commits of 15:10 to 15:14; the merge into `main` squashes them.
Both commits are reachable only as the task branches' history in the hub repository, so the harness's snapshot is a shared clone checked out there, with the production range `git diff 161a4a7f..HEAD` working in it.
The reference is `data/reference/<input>/round-1-review.md`, the Claude fan's consolidated round-1 review, read from the weft history of the run's Code-Bouncer commit (`abfabfcf6`, `c9fb5eb56`), beside the round's focus file, usage record, the writer's change description and the board entry snapshot.
That fan was `sonnet` at medium effort with the three lenses `code-correctness`, `code-affected-set` and `code-tests`, from `generation-record.json` and `round-1-usage.yaml`.
The unselected runs, in the order they were weighed: `dobby-task-limit-hold` (two `LOW` findings, a thin reference), `usage-limit-wake` (test findings only) and `shuttle-hook-signals` (two rounds).

### The reference findings

| Input | Id | Severity | What the fan found |
|---|---|---|---|
| `STB` | `F1` | `BLOCKING` | the start-failed cause does not read `start-failed after <n> tries: <error>` |
| `STB` | `F2` | `MEDIUM` | the loop-trace fallback requires `res.startFailed`, so a started child that dies leaves an empty `trace_file` |
| `STB` | `F3` | `LOW` | a cancelled backoff reports tries that never happened |
| `STB` | `F4` | `LOW` | the empty-path fallback cases have no test row |
| `STB` | `F5` | `LOW` | the rename-failure branch of `installBinary` has no test row |
| `FOR` | `F1` | `MEDIUM` | no test for the approval writing `Reviewed`, nor for a moved HEAD being reviewed again |
| `FOR` | `F2` | `MEDIUM` | no row for a review timeout, a failed start or the report re-prompt |
| `FOR` | `F3` | `MEDIUM` | the spent-fixer no-review criterion is not pinned |
| `FOR` | `F4` | `MEDIUM` | the fix review's model-spec and timeout are asserted nowhere |
| `FOR` | `F5` | `LOW` | the dirty-tree halt embeds a reason whose tail contradicts its own way forward |
| `FOR` | `F6` | `LOW` | two docs still describe the fixer keys as the merge fixer's alone |

## Setup

Reused from `docs/research/codex-v2-p1-bench-2026-10-27/data/harness/`: the staging, the `codex exec` call with a scratch `CODEX_HOME` holding a link to the operator's `auth.json`, the evidence script and the finding dump.
`data/harness/build.sh` assembles the prompts as `internal/burlerengine`'s `composeReviewPrompt` does for a fanned Code-Review round, from the stencils at the reviewed commit:

- the explore step, filled with the PATTERN overview as `pattern-directive-review-fix.md` renders it, the round's focus file, the target and fasit text of `darn-recipe.yaml`'s Code-Burler-Review profile (the committed diff, judged against the board entry snapshot and the decision record) and the Code-Review rubric with `review_base` set to `161a4a7f`;
- the fork reading, the fanned cluster rules with the three lens texts from `internal/lenses/template.yaml` at the commit, the review step, the review-file format and the reviewer seat stencil;
- `AGENTS.md` as a copy of the snapshot's `CLAUDE.md`, whose hash `data/context-hashes.txt` shows and every rollout, handler and fork, shows loaded once.

Changes from production, each mechanical:

- Claude's spawn mechanics become Codex's as in the plan benches: `spawn_agent` once per lens with `fork_turns="all"`, no name, model or effort, and a wait before consolidation.
- The fork boilerplate carries block v2 (below) as one line, and its cap of 6 tool calls is 20.
- The tool-use rules are the read-only text the plan benches ran, where production's Code-Review rows tell the handler to drive the real substrate (build, run, test).
  The snapshot builds under cgo and a raw test run is outside the repository's gate rule; the fan's own forks were read-only in production.
- The run's `PATH` holds no `lym` and no `go`.
  The fork reading's "board entry through `lym board get`" names the snapshot's `_lym/discussion/board-entry.md` instead.
- The fan's hub skills (language skills) are not loaded; a Codex run has none.

### Block v2 for code

`data/prompts/block-v2-code.md` is block v2 reworded for a diff; the entry asked for lines that only fit a plan to be dropped.
One line has no code analogue and is dropped: "Where several cards change the same file or limit, check what each card's gate sees given the cards before it."
Five lines name cards or the plan and are kept with the noun changed to the diff, the board entry, the decision record or the change description, since dropping them leaves one line of seven; the `lym quarry` alternative is dropped from the search line because `lym` is not on the run's path.
The last line, on batching searches, is unchanged.
That rewording is this bench's own choice and a departure from the entry's word "dropped".

## Results

One run per cell.
The fan reference is 5 findings (1 `BLOCKING`) on `STB` and 6 (none `BLOCKING`) on `FOR`.
"Found" counts distinct reference findings that a Codex finding matched, as the plan benches counted; one Codex finding may match several.
`data/match.tsv` holds each reading and `data/reviews/<input>/<run>/` holds each handler's review and the three forks' reports.

| Run | Findings | Shared | Unique real | False | Fan findings found | `BLOCKING` found | Verdict | Fresh input tokens | Wall time |
|---|---|---|---|---|---|---|---|---|---|
| `STB` run 1 | 4 | 2 | 1 | 1 | 2 of 5 | 0 of 1 | `BLOCKING` | 108k | 306 s |
| `STB` run 2 | 4 | 2 | 0 | 2 | 2 of 5 | 0 of 1 | `BLOCKING` | 115k | 276 s |
| `FOR` run 1 | 4 | 1 | 3 | 0 | 3 of 6 | none | `BLOCKING` | 180k | 391 s |
| `FOR` run 2 | 7 | 3 | 4 | 0 | 3 of 6 | none | `BLOCKING` | 168k | 347 s |
| Claude fan, `STB` | 5 | | | | | | `BLOCKING` | 138k | 131 s |
| Claude fan, `FOR` | 6 | | | | | | `APPROVED` | 178k | 147 s |

Fresh input tokens are `input_tokens` less `cached_input_tokens` of each rollout's last `token_count`, summed over the handler and its three forks (`data/usage.txt`).
The Claude fan's figures are `round-1-usage.yaml`'s review half (`fresh_tokens` plus `fork_fresh_tokens`, the wall time of the whole half); Claude counts cache creation as fresh input, so the two token columns are not the same measure, and the Claude fan was `sonnet`, a smaller model than `gpt-6-astra`.
Wall time for a Codex run is the whole `codex exec`.
The weekly `used_percent` rose from 0 to 5 over the four runs, which ran in two parallel pairs on one account; the stop at 70 percent was not reached.

### Per call

| Run | Handler wall | Handler fresh | Fork fresh, three forks | Longest fork's own commands |
|---|---|---|---|---|
| `STB` run 1 | 306 s | 80k | 9k, 12k, 7k | 2 |
| `STB` run 2 | 276 s | 73k | 12k, 16k, 15k | 3 |
| `FOR` run 1 | 391 s | 117k | 18k, 27k, 18k | 4 |
| `FOR` run 2 | 347 s | 108k | 19k, 22k, 19k | 4 |

The forks' wall times run from spawn to their last message, 54 to 145 s; the cap of 20 never bound, since no fork ran more than 4 commands of its own.
Handlers read for 6 to 10 commands before the first spawn.
Each fork's first request held 68k to 104k input tokens, almost all cache hits, which is the inherited context.
`data/rollout-evidence.txt` has the rest.

### Fork evidence

Each handler's rollout holds three `spawn_agent` calls, `code_correctness`, `code_affected_set` and `code_tests`, each with `fork_turns` set to `"all"`, 10 to 19 seconds apart.
Each fork's `session_meta` carries `forked_from_id` of its handler's session, and the fork time spans overlap.
The fork prompts are encrypted in the rollouts, so that each held block v2 and the 20-call cap rests on the handler following the cluster rules, as in the plan benches.
Every handler saved the three lens reports beside its review, `review.lens-<lens>.md`, as the cluster rules direct.

## The findings

### `spawn-text-file-busy`

- Found by both runs: `F3`, a cancelled backoff reports five tries, and `F5`, the rename-failure cleanup has no row.
- Missed by both: `F1`, `F2` and `F4`.
  Both runs rejected `F1` and `F2` by name, on the reading that the board snapshot, not the decision record, is the measuring stick, and rejected `F4` as an extra test of an already covered branch.
- False, in both runs: a `BLOCKING` finding that each of the four ETXTBSY retries logs at `Warn` against the logger's episode policy.
  Decision `D1` of the record settles `Warn` per retry, the logger's hard rule bounds a flood at roughly ten iterations in a loop and this loop makes at most four.
  Run 2 added a `LOW` finding that no test asserts the retry log levels, which stands on that premise.
- Unique real, in run 1 only: no row pins that a child trace and non-empty stderr outrank the loop trace when the fallback is armed.
  Run 2 considered the same point and rejected it as a synthetic combination.

### `fixer-overcap-review`

- Found by both runs: `F1`, `F2` and `F4`, as one `MEDIUM` finding in run 1 and as three in run 2.
- Missed by both: `F3`, `F5` and `F6`.
- Unique real, in both runs: the approval rewrite through `WriteFixerAttempts` drops unknown keys of the attempt record, against `PATTERN-tolerant-records` (`BLOCKING` in both).
  The helper predates the diff and every attempt-record write uses it, so the finding names a gap the new rewrite inherits.
  Also in both: the parser accepts a bare `verdict: approved` with no `## Findings` heading, although the spec requires it (`LOW`).
- Unique real, in run 1 only: the `fix-review-report` gate, which validates the report inside a producer closure, has no CLI self-check and parity check, which `PATTERN-gate-slot` asks a mechanical gate for (`BLOCKING`).
  It is a re-prompt gate of one session and the pattern lists only recipe gates, so the reading is arguable; it is counted real because the pattern's text names a gate in a producer's own closure.
- Unique real, in run 2 only: a wait error from a started review session returns before the dirty-tree and HEAD checks (`MEDIUM`), and the `Finalize.Call` overview still says every over-cap fix reroutes or halts (`LOW`).

## How far the two runs of an input differ

This is the first repeat measure of the code bench.

- The fan's findings found are the same in both runs of each input: `F3` and `F5` on `STB`, `F1`, `F2` and `F4` on `FOR`.
  A second run added no reference finding to the first.
- Findings differ: 4 and 4 on `STB`, of which 3 are the same; 4 and 7 on `FOR`, where run 2 holds every finding of run 1 but the gate parity one, and adds three.
- The `BLOCKING` severity moved on the same defect not at all: the tolerant-records finding is `BLOCKING` in both `FOR` runs, and the false `STB` one in both `STB` runs.
- A second run is therefore worth buying for unique findings, not for fan coverage, on these two inputs; it cost 108k to 180k fresh input tokens.

## Limits

- Two inputs and four runs: no rate can be read from them, and a finding class that did not occur in these diffs is not tested.
- One grader, the writer, who also set the matching rule; the real-or-false calls on the unique findings are mine, from reading the code and the rule text in the snapshot, and two of them (`STB`'s `Warn` per retry and `FOR`'s gate parity) are judgment calls the other way is defensible on.
- The reference is itself one run of a `sonnet` fan at medium: a finding it raised is not a proven defect, and one it did not raise is not proven absent, which is why unique real findings are reported apart from false ones.
  The fan's round-1 review is the reference even where its fixer later changed the fix.
- The fan and the Codex fan differ in more than the engine: model, forks' cap (6 against 20), reading block and tool-use rules, so the result is for the pair of setups.
- The reviewed commits sit only in the hub repository's unreachable history; a garbage collection there removes the snapshots' source, and the `reference/` and `reviews/` files are the record.
- The Claude fan's token and wall figures come from its usage record and are not measured the same way.

## Reproducing

`data/harness/setup-codex-home.sh stb1 stb2 for1 for2`, then `build.sh STB abfabfcf6 7615d129 161a4a7f` and `build.sh FOR c9fb5eb56 d50e5277 161a4a7f`, then `run-forkfan.sh <STB|FOR> <label> medium` per run, then `evidence.sh <input> <label>`.
The scratch tree under `.scratch/`, including the four scratch `CODEX_HOME` directories, was removed after the runs.
