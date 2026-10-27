# Codex Astra forked fan with reading block v2 and a 20-call fork cap on P4: one high run against two medium runs

Bench of 2026-10-27, a follow-up to `docs/research/codex-astra-tuned-p4-med-bench-2026-10-27/report.md` and `docs/research/codex-astra-tuned-prompt-bench-2026-10-27/report.md`.
Codex CLI 0.164.0, ChatGPT sign-in (plan `prolite`), Linux.
Three `codex exec` handler runs on the plan `P4` (`machine-handover`), `gpt-6-astra` for the handler and its forks: one at `high` effort and two at `medium`, identical but for effort.
Each handler read the plan, then spawned three lens forks through `spawn_agent` with `fork_turns="all"`.
Every condition of the tuned-prompt bench is kept (snapshot, Claude reviewers' context, `AGENTS.md`, lenses) except two: block v1 is replaced by block v2 (`data/prompts/block-v2.md`), and the fork boilerplate's cap sentence reads `at most 20 tool calls` where it read `at most 6`.
Nothing was built: the bench is shell scripts under `data/harness/`, the reviewers' own output and a hand grading.

## Conclusion

Block v2 with the raised cap did not find the Claude fan's missed findings.
The three runs found 3, 2 and 3 of the fan's 11 findings, against 4 for block v1 at high and 3 at medium.
No run found the fan's `F1` (the owner guard on `retryWeftPush`), and none found a finding that needs every reader of a changed symbol (`F7`, the new leaf `machinename` absent from `PATTERN-leaf-packages`'s background and the overview's module tree).
The one fan finding the high run lost against block v1 is `F2` (`adoptcli` missing from `PATTERN-cli-cobra`'s background), which the second medium run found.

| Fan `BLOCKING` finding | High | Medium 1 | Medium 2 |
|---|---|---|---|
| `F1`: the owner guard on `retryWeftPush` reads origin's Bolt outside `ReconcileOrigin` | no | no | no |
| `F2`: `adoptcli` missing from `PATTERN-cli-cobra`'s background | no | no | yes, as `F2`, `affected-set` fork |
| `F3`: the one-second production wait in `TestSetOwners` | yes, as `F4`, handler | yes, as `F2`, handler | no |

- One high run is not the better buy here.
  It found 3 of 11 fan findings and 1 of 3 `BLOCKING` for 511k fresh input tokens, 27 weekly-window points and 1209 s; the union of the two medium runs found 4 of 11 and 2 of 3 `BLOCKING` for 612k, 7 points and 631 s when run in parallel.
  The union costs 20 percent more fresh input and 74 percent fewer window points (a figure the sibling benches muddy), and it finishes in 52 percent of the high run's wall time.
- Each medium run alone found 2 or 3 of 11 and 1 of 3 `BLOCKING`, which is what the high run found.
  Each medium run cost 54 to 65 percent of the high run's fresh input.
- The two medium runs differ much more than their counts do: they share one fan finding (`F5`) and one handler finding in 11 distinct, and each holds the `BLOCKING` fan finding the other lacks.
  The overlap is the first repeat measure of these benches; see "Variance between the medium runs".
- The high run's 14 findings hold 11 `unique-real`, 5 of which a medium run also reached; the three runs have no `false` finding.
- No handler dropped a fork finding in any run.

One input, one run per cell and one grader cannot show that effort or the block caused any difference: the medium pair, which differ in nothing, differ by one `BLOCKING` fan finding in each direction.

## Summary table

One run per cell, `P4` only.
The fan reference is 11 findings, 3 of them `BLOCKING`, from `opus` at medium effort with 3 lens forks, as the earlier benches recorded.
"Found" counts distinct fan findings that a finding of the setting matched.
The union row counts the distinct findings of the two medium runs, with fresh input and window points summed and wall time as the longer run, since the two runs share nothing.

| Setting | Findings | Shared with fan | Unique real | False | Fan findings found | `BLOCKING` found | Fresh input tokens | Window points | Wall time |
|---|---|---|---|---|---|---|---|---|---|
| Block v2, high | 14 | 3 | 11 | 0 | 3 of 11 (`F3`, `F8`, `F9`) | 1 of 3 (`F3`) | 511k | 27 | 1209 s |
| Block v2, medium run 1 | 5 | 2 | 3 | 0 | 2 of 11 (`F3`, `F5`) | 1 of 3 (`F3`) | 277k | 3 | 427 s |
| Block v2, medium run 2 | 7 | 3 | 4 | 0 | 3 of 11 (`F2`, `F5`, `F8`) | 1 of 3 (`F2`) | 334k | 4 | 631 s |
| Block v2, union of the two medium runs | 11 | 4 | 7 | 0 | 4 of 11 (`F2`, `F3`, `F5`, `F8`) | 2 of 3 (`F2`, `F3`) | 612k | 7 | 631 s |

All four rows are `gpt-6-astra` forked fans with the Claude reviewers' context, block v2 and a cap of 20 calls per fork; all returned the fan's verdict (`BLOCKING`).
The window points are the weekly window's `used_percent` at the first and last request of each call; sibling benches `codex-v2-p1-bench` and `codex-v2-p2-bench` ran on the same account at the same time, so the points cannot be attributed to a run, and the high run's 27 points against 3 and 4 for the medium runs says little about their cost.
The weekly window peaked at 62 percent, so the stop at 70 percent was never reached.

### Beside every earlier row for `P4`

| Setting | Findings | Shared with fan | Unique real | False | Fan findings found | `BLOCKING` found | Verdict as fan's | Fresh input tokens | Wall time |
|---|---|---|---|---|---|---|---|---|---|
| `gpt-6-astra` high, forked fan, block v2 (this bench) | 14 | 3 | 11 | 0 | 3 of 11 | 1 of 3 | yes | 511k | 1209 s |
| `gpt-6-astra` medium, forked fan, block v2, run 1 (this bench) | 5 | 2 | 3 | 0 | 2 of 11 | 1 of 3 | yes | 277k | 427 s |
| `gpt-6-astra` medium, forked fan, block v2, run 2 (this bench) | 7 | 3 | 4 | 0 | 3 of 11 | 1 of 3 | yes | 334k | 631 s |
| `gpt-6-astra` medium, forked fan, tuned prompt (block v1) and parity context | 6 | 3 | 3 | 0 | 3 of 11 | 2 of 3 | yes | 236k | 492 s |
| `gpt-6-astra` high, forked fan, tuned prompt (block v1) and parity context | 10 | 4 | 6 | 0 | 4 of 11 | 2 of 3 | yes | 631k | 950 s |
| `gpt-6-astra` high, forked fan, parity context | 8 | 2 | 6 | 0 | 2 of 11 | 0 of 3 | yes | 324k | 980 s |
| `gpt-6-astra` high, forked fan | 9 | 1 | 8 | 0 | 1 of 11 | 0 of 3 | yes | 299k | 737 s |
| `gpt-6-astra` medium, cold-call fan | 8 | 0 | 8 | 0 | 0 of 11 | 0 of 3 | yes | 466k | 907 s |
| `gpt-6-astra` high, solo | 3 | 1 | 2 | 0 | 1 of 11 | 0 of 3 | yes | 134k | 487 s |
| `gpt-6-astra` medium, solo | 4 | 2 | 2 | 0 | 2 of 11 | 0 of 3 | yes | 77k | 166 s |
| Claude `opus` medium, solo | 6 | 2 | 4 | 0 | 2 of 11 | 0 of 3 | no (`APPROVED`) | 128k | 389 s |
| Claude `opus` medium, fan | 11 | | | | | | | 489k, plus 79k in forks | 735 s |

The rows below the first three are the earlier benches' `P4` rows, copied from their reports.
Fresh input tokens for Astra are `input_tokens` less `cached_input_tokens` of each rollout's last `token_count`, summed over the handler and its forks; Opus counts cache creation as fresh input, so the figures are not the same measure.
The earlier Astra rows ran on other days' account load, so their wall times are not matched to these.

## The fan's `BLOCKING` findings and which run found them

- `F1` (the owner guard on `retryWeftPush` reads origin's Bolt outside `ReconcileOrigin`, against the record's constraint that every other owner read uses the local Bolt): no run, as in every earlier Astra run.
  The high run's `F5` is the nearest finding, a pull that replays a diverged weft before a push the guarded retry never sees; it concerns the same guard but not the constraint the fan's finding rests on, so it is `unique-real`.
- `F2` (`adoptcli` missing from `PATTERN-cli-cobra`'s background): the second medium run only.
  Its `affected-set` fork named the engine-less module cli and the exceptions list.
  Block v1's high and medium runs both found it; the high and first medium run here did not.
- `F3` (the production second in `TestSetOwners`): the high run and the first medium run, both through the handler's own reading of `PATTERN-test-speed.md` and card 3.
  The second medium run missed it.

## Variance between the medium runs

The medium runs are identical in every condition, so their difference is the run-to-run variation of this setup on one plan.

- Fan findings found: run 1 `F3`, `F5`; run 2 `F2`, `F5`, `F8`; one in common (`F5`), four in the union.
- `BLOCKING` fan findings found: run 1 `F3`, run 2 `F2`; none in common, two in the union.
- Findings returned: 5 and 7; of the 11 distinct, one is in both runs (card 8's prime claim before the offline legacy-run resume, `F5`).
- Origin of the findings: run 1's handler wrote 1 of its 5; run 2's wrote none of its 7, all came from forks.
- Fresh input: 277k and 334k; wall time 427 s and 631 s.

Two runs of one setting on one plan share 1 finding of 11 distinct, and a count of fan findings found ranging from 2 to 3 hides that the sets differ.
One pair of repeats on one input cannot size this variation, so a difference of one or two in a count between settings is within what the pair shows.
The high run holds 7 of the medium union's 11 findings; it lacks the fan's `F2` and `F5`, which the medium runs found.

## Per call

| Run | Call | Wall time | Fresh input tokens | Output tokens | Reasoning tokens | Own commands | Weekly window after |
|---|---|---|---|---|---|---|---|
| High | Handler | 1208 s | 316k | 22.3k | 8.0k | 42 | 55 percent |
| High | Fork `affected-set` | 333 s | 73k | 4.9k | 1.4k | 12 | 52 percent |
| High | Fork `guard-reach` | 569 s | 82k | 8.7k | 3.8k | 16 | 53 percent |
| High | Fork `way-forward` | 256 s | 39k | 4.9k | 2.5k | 7 | 50 percent |
| Medium 1 | Handler | 424 s | 142k | 9.3k | 2.1k | 23 | 58 percent |
| Medium 1 | Fork `affected-set` | 232 s | 62k | 4.3k | 1.5k | 11 | 58 percent |
| Medium 1 | Fork `guard-reach` | 122 s | 27k | 2.6k | 0.7k | 5 | 56 percent |
| Medium 1 | Fork `way-forward` | 178 s | 46k | 3.6k | 1.0k | 9 | 57 percent |
| Medium 2 | Handler | 629 s | 168k | 12.6k | 5.0k | 23 | 62 percent |
| Medium 2 | Fork `affected-set` | 247 s | 73k | 5.2k | 1.8k | 12 | 61 percent |
| Medium 2 | Fork `guard-reach` | 181 s | 40k | 3.5k | 1.1k | 7 | 60 percent |
| Medium 2 | Fork `way-forward` | 208 s | 54k | 4.0k | 1.6k | 10 | 61 percent |

The handler's figures are its own thread's, so a run's rows add up to its total: 511k fresh input and 40.8k output tokens for the high run, 277k and 19.7k for medium 1, 334k and 25.3k for medium 2.
A fork's rollout also holds the commands the handler ran before it spawned (22 for the high run, 10 for medium 1, 11 for medium 2); a fork's own command count is its rollout's count less those.
The forks' wall times run from their rollout's first to its last event; their window readings come from the last `token_count` in each fork's rollout, so they are not a sequence.
No fork came near the cap of 20 calls: the most any made was 16, where the forks of the block v1 bench stopped at about 6.
The forks' command counts were 5 to 12 at medium and 7 to 16 at high.
No command in any rollout names `lym quarry`.

The handler's wall time is the whole `codex exec`: the high run read for 2 minutes 31 seconds before its first spawn and the medium runs for 72 s and 84 s.

Window figures: the weekly window is one window of 10080 minutes.
It read 28 percent at the high handler's first request and 55 at its end; the first medium run began at 55 and ended at 58, the second began at 58 and ended at 62.
The sibling benches `codex-v2-p1-bench` and `codex-v2-p2-bench` ran on the same account at the same time, so none of these points can be attributed to one run.

## Fork evidence

The rollouts are the check the task asked for; `data/rollout-evidence.txt` holds the figures.

- Started: each handler's rollout holds three `spawn_agent` calls, `affected_set`, `guard_reach` and `way_forward`, each with `fork_turns` set to `"all"`, within 23 s of one another in every run.
  The fork bodies are encrypted, so the fork prompts cannot be read.
- Concurrent: in every run all three forks started before the first one ended; the shortest fork ran 122 s and the spawns are 11 to 12 s apart.
- Inherited context: each fork's `session_meta` carries `forked_from_id` of its handler's session, and its first request already held 104k to 192k input tokens, against 16k for the handler's first.
  Each fork's rollout holds its handler's earlier commands.
- `AGENTS.md` loaded: every rollout holds an `AGENTS.md instructions for` block naming the run's working directory (the high run's handler and first two forks hold two blocks); the hash of the snapshot's `AGENTS.md` equals that of its `CLAUDE.md` (`data/context-hashes.txt`).

Both checks the task names passed in all three runs, so all three are graded.

## Context and prompt

The context is the tuned-prompt bench's, rebuilt from the same snapshot with the same scripts.
`data/context-hashes.txt` equals the earlier benches' for `AGENTS.md`, `CLAUDE.md` and `PATTERN.md`; only the block's hash differs, as it should.

- The prompt carries the PATTERN directive a production burler reviewer gets, built by `harness/build.sh` from the code commit's `pattern-directive-review-fix.md`, the commit's `PATTERN.md` and the decision record's declared `PATTERN-owner-machine` change.
- The snapshot's root holds `CLAUDE.md` copied to `AGENTS.md`.
- The lenses are the Claude fan's round 1 lenses on `P4`: `affected-set`, `guard-reach` and `way-forward`, read from the snapshot's `internal/lenses/template.yaml`.
- Block v2 is `data/prompts/block-v2.md`, copied word for word from the board entry.
  It adds to block v1 an every-reader search for each changed symbol, type, config key, file or command, a one-hop read of the callers and callees of each changed function, a check of what each card's gate sees given the cards before it, and a direction to batch searches.
  `harness/build-forkfan.sh` appends it to the handler prompt and inserts it, as one line, into the fork boilerplate.
- The fork boilerplate's cap sentence reads `You may make at most 20 tool calls`; the script replaces `at most 6 tool calls` and stops if the sentence is absent, and nothing else of the boilerplate changed.

Remaining differences from the Claude round's context are the earlier benches': no plugin skills, empty parent and edit directives, and the snapshot's `CLAUDE.md`.

## The handlers' findings

`data/match.tsv` holds each reading.
The grader had the fan's round-1 review (`_lyx/reviews/plan/round-1-review.md` at the plan's weft commit), which the earlier benches' reports describe by notes only.

Fan findings, from that review: `F1` `BLOCKING` the `retryWeftPush` guard's origin-Bolt read; `F2` `BLOCKING` `adoptcli` and `PATTERN-cli-cobra`; `F3` `BLOCKING` the `TestSetOwners` production wait; `F4` `MEDIUM` `ownerDue` and the sleep watch living on `childWait`; `F5` `MEDIUM` card 8's prime claim and the offline legacy-run resume; `F6` `MEDIUM` card 17's short PATTERN line; `F7` `MEDIUM` the `machinename` leaf absent from `PATTERN-leaf-packages` and the overview; `F8` `LOW` `recordedPairs`; `F9` `LOW` bundled public symbols; `F10` `LOW` card 16's reworded leftover-branch step; `F11` `LOW` the `TestChildSpawnError` row with no owner.

| Run | Finding | Severity | Defect | Grade |
|---|---|---|---|---|
| High | `F1` | `BLOCKING` | Direct loom and darn runs have no batten record and escape the owner verdict | shared, fan `F8` |
| High | `F2` | `BLOCKING` | `Arm` and `ArmAt` build receivers with no reconcile seam | `unique-real` |
| High | `F3` | `BLOCKING` | `SetOwners` has no route to the pair-parent resolver | `unique-real` (arguable) |
| High | `F4` | `BLOCKING` | The deferred-push test waits a production second | shared, fan `F3` |
| High | `F5` | `MEDIUM` | A pull replays an old owner's weft ahead of the guarded push | `unique-real` |
| High | `F6` | `MEDIUM` | The completed old machine's lifted pause cannot reach Land | `unique-real` |
| High | `F7` | `MEDIUM` | `Finalize`'s parent fetch precedes the offline policy | `unique-real` |
| High | `F8` | `MEDIUM` | The no-takeover guard omits the prime | `unique-real` |
| High | `F9` | `MEDIUM` | The repair note keeps the forbidden merge advice | `unique-real` |
| High | `F10` | `MEDIUM` | Adopt waits in the hub pause's network gate | `unique-real` |
| High | `F11` | `MEDIUM` | New nil-legal seams are absent from `envkit`'s `nilLegal` | `unique-real` |
| High | `F12` | `MEDIUM` | An external-package test cannot reach a private helper | `unique-real` |
| High | `F13` | `MEDIUM` | `RenderCompact` shows no owner | `unique-real` (arguable) |
| High | `F14` | `MEDIUM` | Public surfaces bundled into consumer cards | shared, fan `F9` |
| Medium 1 | `F1` | `BLOCKING` | Card 13 copies the owner pause before the root pre-run installs it | `unique-real` |
| Medium 1 | `F2` | `BLOCKING` | The retry test waits a production duration | shared, fan `F3` |
| Medium 1 | `F3` | `MEDIUM` | The prime claim defeats the offline legacy-run resume | shared, fan `F5` |
| Medium 1 | `F4` | `MEDIUM` | Lifting the completed old owner's pause reinstalls it | `unique-real` |
| Medium 1 | `F5` | `MEDIUM` | Adopt cannot resume an owner-halted LLM child on a clone with no driver strand | `unique-real` |
| Medium 2 | `F1` | `BLOCKING` | Ownership is checked only for batten-recorded pairs | shared, fan `F8` |
| Medium 2 | `F2` | `BLOCKING` | `adoptcli` is missing from `PATTERN-cli-cobra`'s exceptions | shared, fan `F2` |
| Medium 2 | `F3` | `MEDIUM` | New seams omit reflective fixture readers | `unique-real` |
| Medium 2 | `F4` | `MEDIUM` | An external-package test cannot reach its private subject | `unique-real` |
| Medium 2 | `F5` | `MEDIUM` | The no-takeover guard omits the prime | `unique-real` |
| Medium 2 | `F6` | `MEDIUM` | The prime claim defeats the offline legacy-run resume | shared, fan `F5` |
| Medium 2 | `F7` | `MEDIUM` | The repair note keeps the forbidden merge advice | `unique-real` |

- Claims checked on the snapshot: the high run's `F2` rests on `reconcileResume` returning nil on a nil seam (`internal/battencli/resumereconcile.go:39`); its `F13` on `RenderCompact` building explicit columns with no owner, while D9 names `owner_machine` on `board list`, so it stays arguable; medium 1's `F5` on card 16 line 64.
  The other `unique-real` findings are graded on the cited card lines and the handlers' own source citations, not re-run.
- Severity differs from the fan's: the fan's `F8` is `LOW` and the high run and medium 2 rate it `BLOCKING`; the fan's `F9` is `LOW` and the high run rates it `MEDIUM`; their `F5` matches `MEDIUM` for `MEDIUM`.
- Dropped fork findings: none, in any run.
  Medium 1's `affected-set` fork reported card 13's pause copy, the same finding as `guard-reach`, and the handler kept one copy.
- Unmatched fan findings in all three runs: `F1`, `F4`, `F6`, `F7`, `F10`, `F11`.

## Did the block's new clauses show in the work

- The every-reader clause did not produce `F7`, which is a missing reader of a new leaf package.
  The forks cited cards and `pattern/` files; the high run's `F11` (the `nilLegal` list in `envkit`) is the one finding that reads as a reader of a changed contract that no card covers, found by the `affected-set` fork in the high run and in medium 2, not in medium 1.
- The one-hop clause is visible in the high run's `guard-reach` fork, whose `F5`, `F6` and `F7` follow a card's function to its callers and callees in `fabricengine`, `battencli` and `landingshed`.
- The raised cap was used: the forks ran up to 16 commands, against about 6 under block v1 and a cap of 6.
  The extra reading produced new `unique-real` findings in the high run, not more fan findings.

These are readings of three runs; the rollouts do not show which clause caused a finding.

## Method

### Inputs and prompts

`P4` is rebuilt with `data/harness/build.sh` from the earlier benches' weft commit and code commit in `data/inputs.txt`.
The harness is the earlier P4 medium bench's, with the path root changed to this worktree and three edits: `build-forkfan.sh` replaces the cap sentence's number and reads `block-v2.md`, `run-forkfan.sh` takes the effort and a run label, and `setup-codex-home.sh` takes a label so each run has its own scratch `CODEX_HOME`.
The prompts are in `data/prompts/`: the handler prompt with the block, the two instruction files it reads, the block, and the prompt as the high run's script rewrote it for its working copy (the other runs' differ in the path only).

### Call

```text
CODEX_HOME=<scratch>/codex-home-<label> codex exec -m gpt-6-astra -c model_reasoning_effort="<effort>" \
  -s workspace-write -C <work> --skip-git-repo-check --json -o <last-message> - < prompt
```

The three runs ran one after another, in the order high, medium 1, medium 2, each in its own copy of the working tree and its own scratch `CODEX_HOME` holding only a link to the operator's `auth.json`, so the operator's `config.toml` did not apply.
The medium pair's wall times count as run in parallel because the two share no state; they were not run at the same time.
The forks inherited the handler's effort; no model or effort was passed to `spawn_agent`.

### Grading

One Claude session graded every handler finding as the earlier benches did: against the fan's round-1 findings of `P4` and an unmatched finding against the cited card lines and the snapshot's plan and code.
`shared` means the finding names the same defect as a fan finding; `unique-real` means the claim holds and the fan's round has no finding of it; `false` means it does not hold.
Two findings on the same card are not matched unless they name the same defect: the high run's `F6` and medium 1's `F4` are on card 10 as the fan's `F4` is, and are not the fan's.

## Deviations and limits

- One input, one run per cell and one grader: the only variance measured is one medium pair, and it is large.
- The grader is a Claude session and may favour its own family's phrasing; the rates are not validated.
- The fork prompts are encrypted in the rollout; that each fork's text held block v2 and the cap of 20 rests on the handler following the cluster rules and on the script's anchor checks.
- Sibling benches ran on the same account at the same time, so the window points cannot be attributed to a run.
- The runs were sequential, so the medium pair's parallel wall time is the longer of the two and not a measured figure.
- Recall is against the fan's list, not against the truth; the 11 `unique-real` findings of the high run hold on reading, so the fan reference is not complete for `P4`.
- The operator's `~/.codex/config.toml` was not hashed before or after this bench.

## Data

`data/` holds:

- `inputs.txt`: the input with weft commit, round start time and code commit.
- `usage.txt`: one line per call with wall seconds, tokens and the weekly `used_percent`, then the fork command counts, the verdicts and the window note.
- `rollout-evidence.txt`: start, end, parent thread, commands, first request and totals of each rollout, each handler's `spawn_agent` calls and the `AGENTS.md` loads.
- `context-hashes.txt`: the hashes of `AGENTS.md`, `CLAUDE.md`, `PATTERN.md` and the block.
- `match.tsv`: the grading of every handler finding.
- `reviews/high/`, `reviews/med1/`, `reviews/med2/`: each handler's review and the three forks' final messages.
- `prompts/`: the handler prompt, the two instruction files it reads, the block and a run's rewritten prompt.
- `harness/`: `build.sh`, `build-fan.sh`, `build-forkfan.sh`, `run-forkfan.sh`, `setup-codex-home.sh`, `windows.sh`, `dump.sh` and `evidence.sh`, working on `.scratch/bench/`, which is not committed.
