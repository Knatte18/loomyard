# Codex Astra forked fan with a tuned reviewer prompt at medium effort on one plan

Bench of 2026-10-27, a follow-up to `docs/research/codex-astra-tuned-prompt-bench-2026-10-27/report.md`.
Codex CLI 0.164.0, ChatGPT sign-in (plan `prolite`), Linux.
One `codex exec` handler run on the plan `P4` (`machine-handover`), `gpt-6-astra` at medium effort for the handler and its forks, one run, no repeat.
The handler read the plan, then spawned three lens forks through `spawn_agent` with `fork_turns="all"`.
Every condition of the tuned-prompt bench is kept (snapshot, Claude reviewers' context, `AGENTS.md`, lenses, reading block, cap of 6 commands per fork); only the effort changes from high to medium.
Nothing was built: the bench is shell scripts under `data/harness/`, the reviewers' own output and a hand grading.

## Conclusion

At medium effort the forked fan with the Claude reviewers' context and the reading block found 3 of the Claude fan's 11 findings on `P4` and 2 of its 3 `BLOCKING` ones, where the same setup at high effort found 4 of 11 and the same 2 of 3.
The handler returned the fan's verdict (`BLOCKING`) and 6 findings: 3 shared with the fan, 3 `unique-real`, none `false`.

| Fan `BLOCKING` finding | Found | By |
|---|---|---|
| `F1`: the owner guard on `retryWeftPush` reads origin's Bolt outside `ReconcileOrigin` | no | |
| `F2`: `adoptcli` missing from `PATTERN-cli-cobra`'s background | yes, as `F1` | `affected-set` fork |
| `F3`: the one-second production wait in `TestSetOwners` | yes, as `F2` | the handler |

- The two found findings are the two the high-effort run found, by the same route: `affected-set` cited `pattern/PATTERN-cli-cobra.md` lines 27 to 35, and the handler flagged the production wait after reading `PATTERN-test-speed.md` and the plan's test card.
- The missed `F1` is missed again, and no finding of the run touches `retryWeftPush`.
- Against the high-effort run, medium lost the fan's `F8` (a run without a batten record never reads `owned_elsewhere`) and all six of that run's `unique-real` findings: the foreground `loom run` and `loom step` bypass of the owner guard, the skip-variable bypass, `Finalize`'s parent fetch ahead of the past-merge exemption, the unarmed `wire` callers, the absent entry classified as `Lost`, and the equality case of `SleepWatch`.
  Medium found the fan's `F9` (bundled public symbols, rated `MEDIUM` against the fan's `LOW`) as high did.
- Medium's three `unique-real` findings are new against the high run's handler review: card 3's `SetOwners` has no route to the pair-parent resolver (arguable), card 9's shed owner verdict loses the unreachable-origin provenance that D3's recovery message needs, and the owner-pause callback copied before the root pre-run installs it.
  The last is the fork finding the high run's handler dropped, and the earlier forked fan's `F1`; here `guard-reach` found it and the handler kept it, at `MEDIUM`.
- The handler dropped none of the forks' 3 findings.
- The run took 492 s and 236k fresh input tokens in all, against 950 s and 631k for the high-effort run.
  Medium finished in 52 percent of the wall time on 37 percent of the fresh input.

One input, one run per cell and one grader cannot show that effort caused the difference: a repeat of either run could change a count of one or two.
Both runs reached the same two `BLOCKING` fan findings, which suggests that effort did not decide those two on this plan; the sample cannot show what did.

## Summary table

One run per cell, `P4` only.
The fan reference is 11 findings, 3 of them `BLOCKING`, from `opus` at medium effort with 3 lens forks, as the earlier benches recorded.
"Found" counts distinct fan findings that a finding of the setting matched.

| Setting | Findings | Shared with fan | Unique real | False | Fan findings found | `BLOCKING` found | Verdict as fan's | Fresh input tokens | Wall time |
|---|---|---|---|---|---|---|---|---|---|
| `gpt-6-astra` medium, forked fan, tuned prompt and parity context | 6 | 3 | 3 | 0 | 3 of 11 | 2 of 3 | yes | 236k | 492 s |
| `gpt-6-astra` high, forked fan, tuned prompt and parity context | 10 | 4 | 6 | 0 | 4 of 11 | 2 of 3 | yes | 631k | 950 s |
| `gpt-6-astra` high, forked fan, parity context | 8 | 2 | 6 | 0 | 2 of 11 | 0 of 3 | yes | 324k | 980 s |
| `gpt-6-astra` high, forked fan | 9 | 1 | 8 | 0 | 1 of 11 | 0 of 3 | yes | 299k | 737 s |
| `gpt-6-astra` medium, cold-call fan | 8 | 0 | 8 | 0 | 0 of 11 | 0 of 3 | yes | 466k | 907 s |
| `gpt-6-astra` high, solo | 3 | 1 | 2 | 0 | 1 of 11 | 0 of 3 | yes | 134k | 487 s |
| `gpt-6-astra` medium, solo | 4 | 2 | 2 | 0 | 2 of 11 | 0 of 3 | yes | 77k | 166 s |
| Claude `opus` medium, solo | 6 | 2 | 4 | 0 | 2 of 11 | 0 of 3 | no (`APPROVED`) | 128k | 389 s |
| Claude `opus` medium, fan | 11 | | | | | | | 489k, plus 79k in forks | 735 s |

The rows below the first two are the earlier benches' `P4` rows, copied from their reports.
Fresh input tokens for Astra are `input_tokens` less `cached_input_tokens` of the rollout's last `token_count`, summed over the handler and its forks in this run; Opus counts cache creation as fresh input, so the figures are not the same measure.

## Per call

| Call | Wall time | Fresh input tokens | Output tokens | Commands | Weekly window after |
|---|---|---|---|---|---|
| Handler | 492 s | 151k | 9.6k | 22 | 27 percent |
| Fork `affected-set` | 131 s | 31k | 2.4k | 6 | 24 percent |
| Fork `guard-reach` | 176 s | 35k | 3.1k | 6 | 24 percent |
| Fork `way-forward` | 157 s | 20k | 2.8k | 4 | 25 percent |

The handler's figures are its own thread's, so the forks' rows add to it: 236k fresh input and 18.0k output tokens in all.
The forks' wall times run from spawn to their last message; their window readings come from the last `token_count` in each fork's rollout, so they are not a sequence.
The handler's wall time is the whole `codex exec`: 109 s of reading before the first spawn, 187 s more until the last fork returned, and 195 s of consolidation.
The forks' command counts are their own: each fork's rollout also holds the 15 commands of the handler's reading, which it inherited.

Each fork's first request held about 129k input tokens, of which about 126k were cache hits, so a fork's fresh reading is under 3k before its own commands.
The high-effort run's forks started from 196k with 112k cached; the handler's reading was larger there (19 commands, 172 s).

The window is a single weekly window of 10080 minutes, which read 18 percent at the call's first request and 27 at its end.
Sibling benches (the `P1` benches of the same series) may have drawn on the same account over the same hour, so the 9 points cannot be attributed to this run.
The stop at 70 percent was never near.

## Fork evidence

The rollouts are the check the task asked for; `data/rollout-evidence.txt` holds the figures.

- Started: the handler's rollout holds three `spawn_agent` calls, `affected_set`, `guard_reach` and `way_forward`, each with `fork_turns` set to `"all"`, at 12:43:57, 12:44:08 and 12:44:17 UTC.
  The fork bodies are encrypted, so the fork prompts themselves cannot be read.
- Concurrent: the forks ran 12:43:57 to 12:46:08, 12:44:08 to 12:47:04 and 12:44:17 to 12:46:54, so all three overlap from 12:44:17 to 12:46:08.
- Inherited context: each fork's `session_meta` carries `forked_from_id` of the handler's session, and its first request already held 128.5k, 129.0k and 129.4k input tokens against 16k for the handler's first.
  Each fork's rollout holds the handler's 15 earlier commands, including the single read of the `pattern/` background files.
- `AGENTS.md` loaded: the handler's rollout, and each fork's, holds an `AGENTS.md instructions for` block naming the run's working directory; the hash of the snapshot's `AGENTS.md` equals that of its `CLAUDE.md` (`data/context-hashes.txt`).

Both checks the task names passed, so the run is graded.

## Context and prompt

The context is the tuned-prompt bench's, rebuilt from the same snapshot with the same scripts.
`data/context-hashes.txt` equals that bench's file: `AGENTS.md`, `CLAUDE.md`, `PATTERN.md` and `tuned-block.md` have the same hashes.

- The prompt carries the PATTERN directive a production burler reviewer gets, built by `harness/build.sh` from the code commit's `pattern-directive-review-fix.md`, the commit's `PATTERN.md` and the decision record's declared `PATTERN-owner-machine` change.
- The snapshot's root holds `CLAUDE.md` copied to `AGENTS.md`.
- The lenses are the Claude fan's round 1 lenses on `P4`: `affected-set`, `guard-reach` and `way-forward`, read from the snapshot's `internal/lenses/template.yaml`.
- The reading block `data/prompts/tuned-block.md` is copied word for word.
  It tells the reviewer to check each card's claims about existing code by reading that code and its callers and the paths the change reaches, to read the `pattern/` background of every PATTERN entry the plan touches and check where each new command, package or file appears, and to read the tests the plan adds or changes for waits, sleeps and timing.
  `harness/build-forkfan.sh` appends it to the handler prompt and inserts it, as one line, into the fork boilerplate.

Remaining differences from the Claude round's context are the tuned-prompt bench's: no plugin skills, empty parent and edit directives, and the snapshot's `CLAUDE.md`.

## The handler's findings

The handler returned `BLOCKING` with 6 findings: 3 of its own and 3 from the forks, one per fork.
`data/match.tsv` holds each reading.

| Finding | Severity | Origin | Defect | Grade |
|---|---|---|---|---|
| `F1` | `BLOCKING` | `affected-set` | `adoptcli` is missing from `PATTERN-cli-cobra`'s deviations | shared, fan `F2` |
| `F2` | `BLOCKING` | handler | The deferred-push test waits a production second | shared, fan `F3` |
| `F3` | `MEDIUM` | handler | `SetOwners` has no route to the pair-parent resolver | `unique-real` |
| `F4` | `MEDIUM` | `guard-reach` | The owner-pause callback is copied before the root pre-run installs it | `unique-real` |
| `F5` | `MEDIUM` | `way-forward` | The shed owner verdict loses the offline provenance D3's message needs | `unique-real` |
| `F6` | `MEDIUM` | handler | The adopt card bundles three cross-package public APIs | shared, fan `F9` |

- `F3` rests on card 3 lines 43 and 47: `Config.ParentOf` reaches the stores `Board` builds, but `SetOwners` loads its store directly and `OwnerRequest` lists no resolver.
  It stays arguable, since the card does not say the setter lacks one.
- `F4` is the same defect as the earlier forked fan's `F1`, there rated `BLOCKING`; here it is `MEDIUM`.
- `F5` holds on card 9 line 31 against D3's lines 69 to 71 of the decision record: a local Bolt showing another owner is refused naming the owner and a rerun once origin answers, which card 8 carries for batten and card 9 drops.
- The fan findings not found are `F1` (`BLOCKING`), `F4`, `F5`, `F6`, `F7`, `F8`, `F10` and `F11`.

## Method

### Inputs and prompts

`P4` is rebuilt with `data/harness/build.sh` from the earlier benches' weft commit and code commit in `data/inputs.txt`.
The harness is the tuned-prompt bench's with the path root changed to this worktree and the effort set to `medium` in `run-forkfan.sh`; `evidence.sh` is new and prints `data/rollout-evidence.txt` from the rollouts.
The prompts are in `data/prompts/`: the handler prompt with the block, the two instruction files it reads, the block and the prompt as the run rewrote it for its working copy.

### Call

```text
CODEX_HOME=<scratch>/codex-home codex exec -m gpt-6-astra -c model_reasoning_effort="medium" \
  -s workspace-write -C <work> --skip-git-repo-check --json -o <last-message> - < prompt
```

`run-forkfan.sh` runs the one call, then copies every rollout the call created.
The scratch `CODEX_HOME` holds only a link to the operator's `auth.json`, so the operator's `config.toml` did not apply.
The forks inherited the handler's effort; no model or effort was passed to `spawn_agent`.

### Grading

One Claude session graded every handler finding as the earlier benches did: against the fan's round-1 findings of `P4` (`docs/research/codex-review-bench-2026-10-25/data/claude-findings.tsv` and the earlier benches' `match.tsv` notes), and an unmatched finding against the cited card lines and the snapshot's plan.
`shared` means the finding names the same defect as a fan finding; `unique-real` means the claim holds and the fan's round has no finding of it; `false` means it does not hold.
The new claims of `F3` and `F5` were checked against cards 3, 8 and 9 and the decision record's D3.

## Deviations and limits

- One input, one run, one grader: no variance is measured, and a repeat could change a count of one or two.
- The only change against the high-effort run is the effort, but a single pair of runs cannot separate effort from run-to-run variation.
- The grader is a Claude session and may favour its own family's phrasing; the rates are not validated.
- The fork prompts are encrypted in the rollout; that each fork's text held the block rests on the handler following the cluster rules and on the `affected-set` fork's citation of `PATTERN-cli-cobra.md` lines 27 to 35.
- Sibling benches may have drawn on the same account, so the window points cannot be attributed to this run.
- Recall is against the fan's list, not against the truth; the `unique-real` findings hold on reading, so the fan reference is not complete for `P4`.
- The fan's finding texts are not in the repository; the match against the fan rests on the earlier benches' notes and severities.
- The operator's `~/.codex/config.toml` was not hashed before or after this bench.

## Data

`data/` holds:

- `inputs.txt`: the input with weft commit, round start time and code commit.
- `usage.txt`: one line per call with wall seconds, tokens and the weekly `used_percent`, then the handler's verdict.
- `rollout-evidence.txt`: start, end, parent thread, commands, first request and totals of each rollout, the handler's `spawn_agent` calls and the `AGENTS.md` loads.
- `context-hashes.txt`: the hashes of `AGENTS.md`, `CLAUDE.md`, `PATTERN.md` and the block.
- `match.tsv`: the grading of every handler finding.
- `reviews/P4/`: the handler's review and the three forks' final messages.
- `prompts/`: the handler prompt, the two instruction files it reads, the block and the run's rewritten prompt.
- `harness/`: `build.sh`, `build-fan.sh`, `build-forkfan.sh`, `run-forkfan.sh`, `setup-codex-home.sh`, `windows.sh`, `dump.sh` and `evidence.sh`, working on `.scratch/bench/`, which is not committed.
