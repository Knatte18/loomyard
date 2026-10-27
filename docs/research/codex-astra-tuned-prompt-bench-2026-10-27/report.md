# Codex Astra forked fan with a tuned reviewer prompt against the Claude fan on one plan

Bench of 2026-10-27, a follow-up to `docs/research/codex-astra-fork-fan-bench-2026-10-27/report.md`.
Codex CLI 0.164.0, ChatGPT sign-in (plan `prolite`), Linux.
One `codex exec` handler run on the plan `P4` (`machine-handover`), `gpt-6-astra` at high effort for the handler and its forks, one run, no repeat.
The handler read the plan, then spawned three lens forks through `spawn_agent` with `fork_turns="all"`.
Nothing was built: the bench is shell scripts under `data/harness/`, the reviewers' own output and a hand grading.

## Conclusion

With the Claude reviewers' context and one block of reading directions, the forked fan found 4 of the Claude fan's 11 findings on `P4` and 2 of its 3 `BLOCKING` ones.
The forked fan bench, without either addition, found 1 of 11 and none of the 3.
The handler returned the fan's verdict (`BLOCKING`) and 10 findings: 4 shared with the fan, 6 `unique-real`, none `false`.

| Fan `BLOCKING` finding | Found | By |
|---|---|---|
| `F1`: the owner guard on `retryWeftPush` reads origin's Bolt outside `ReconcileOrigin` | no | |
| `F2`: `adoptcli` missing from `PATTERN-cli-cobra`'s background | yes, as `F4` | `affected-set` fork |
| `F3`: the one-second production wait in `TestSetOwners` | yes, as `F5` | the handler |

- The two found findings are the two that the block's second and third clauses ask for: the handler read every `pattern/` background file before it spawned, `affected-set` read `PATTERN-cli-cobra.md` lines 27 to 35 and named `cleanupcli` as the listed exception, and the handler read `PATTERN-test-speed.md` and the plan's test card before it flagged the production wait.
- The missed `F1` needs a read of `retryWeftPush` and of what the card's guard reads from origin; no clause of the block names that, and no fork or handler finding touches the guard.
- The fan's `F8` was found as `F1`: a run without a batten record never reads `owned_elsewhere`.
  The fan rates it `LOW`; Astra rates it `BLOCKING`.
- The fan's `F9`, the bundled public symbols, was found again as `F9`, rated `MEDIUM` against the fan's `LOW`.
- Six findings are `unique-real`: the foreground `loom run` and `loom step` bypass of the owner guard (`BLOCKING`), the skip-variable bypass (`BLOCKING`, found by every earlier Astra run), `Finalize`'s parent fetch ahead of the past-merge exemption, the unarmed `wire` callers of card 8, the absent entry classified as `Lost`, and the equality case of `SleepWatch`.
- The handler dropped one of the forks' 8 findings: `way-forward`'s finding that card 13 copies the owner pause before the root pre-run installs it.
  The handler's reason, that `Command` assigns runtime fields in its pre-run, contradicts card 13 line 36, which has `Command` do the copy; the finding is `unique-real` and the same as the earlier forked fan's `F1`.
- The run took 950 s and 631k fresh input tokens in all, against 737 s and 299k for the forked fan bench.

One input, one run per cell and one grader cannot show that the block caused the gain.
The run differs from the forked fan bench in three things at once: the inlined PATTERN overview, `AGENTS.md`, and the block.
The sibling bench `codex-astra-context-parity-bench` runs the first two without the block; compare its report to separate them.

## Summary table

One run per cell, `P4` only.
The fan reference is 11 findings, 3 of them `BLOCKING`, from `opus` at medium effort with 3 lens forks, as the earlier benches recorded.
"Found" counts distinct fan findings that a finding of the setting matched.

| Setting | Findings | Shared with fan | Unique real | False | Fan findings found | `BLOCKING` found | Verdict as fan's | Fresh input tokens | Wall time |
|---|---|---|---|---|---|---|---|---|---|
| `gpt-6-astra` high, forked fan, tuned prompt and parity context | 10 | 4 | 6 | 0 | 4 of 11 | 2 of 3 | yes | 631k | 950 s |
| `gpt-6-astra` high, forked fan | 9 | 1 | 8 | 0 | 1 of 11 | 0 of 3 | yes | 299k | 737 s |
| `gpt-6-astra` medium, cold-call fan | 8 | 0 | 8 | 0 | 0 of 11 | 0 of 3 | yes | 466k | 907 s |
| `gpt-6-astra` medium, solo | 4 | 2 | 2 | 0 | 2 of 11 | 0 of 3 | yes | 77k | 166 s |
| Claude `opus` medium, solo | 6 | 2 | 4 | 0 | 2 of 11 | 0 of 3 | no (`APPROVED`) | 128k | 389 s |
| Claude `opus` medium, fan | 11 | | | | | | | 489k, plus 79k in forks | 735 s |

The last five rows are the forked fan bench's `P4` rows.
Fresh input tokens for Astra are `input_tokens` less `cached_input_tokens` of the rollout's last `token_count`, summed over the handler and its forks in this run; Opus counts cache creation as fresh input, so the figures are not the same measure.

## Per call

| Call | Wall time | Fresh input tokens | Output tokens | Commands | Weekly window after |
|---|---|---|---|---|---|
| Handler | 950 s | 286k | 19.1k | 37 | 18 percent |
| Fork `affected-set` | 191 s | 121k | 4.2k | 6 | 14 percent |
| Fork `guard-reach` | 269 s | 107k | 5.5k | 5 | 15 percent |
| Fork `way-forward` | 224 s | 117k | 4.7k | 6 | 15 percent |

The handler's figures are its own thread's, so the forks' rows add to it.
The forks' wall times run from spawn to their last message; their window readings come from the last `token_count` in each fork's rollout, so they are not a sequence.
The handler's wall time is the whole `codex exec`: 172 s of reading before the first spawn, 278 s more until the last fork returned, and 500 s of consolidation.
The forks' command counts are their own: each fork's rollout also holds the 19 commands of the handler's reading, which it inherited.

Each fork's first request held about 196k input tokens, of which 112k were cache hits, so a fork's fresh reading is 83k before its own commands.
The forked fan bench's forks started from 147k with 145k cached.
The larger context is the inlined overview and the pattern background the handler read; the lower hit count is as measured and unexplained.

## Fork evidence

The rollouts are the check the task asked for; `data/rollout-evidence.txt` holds the figures.

- Started: the handler's rollout holds three `spawn_agent` calls, `affected_set`, `guard_reach` and `way_forward`, each with `fork_turns` set to `"all"`, at 12:18:59, 12:19:08 and 12:19:17 UTC.
  The fork bodies are encrypted, so the fork prompts themselves cannot be read.
- Concurrent: the forks ran 12:18:59 to 12:22:10, 12:19:08 to 12:23:37 and 12:19:17 to 12:23:01, so all three overlap from 12:19:17 to 12:22:10.
- Inherited context: each fork's `session_meta` carries `forked_from_id` of the handler's session, and its first request already held 195.6k, 196.1k and 196.5k input tokens against 16k for the handler's first.
  Each fork's rollout holds the handler's 19 earlier commands, including the reads of the `pattern/` background files.
- `AGENTS.md` loaded: the handler's rollout, and each fork's, holds an `AGENTS.md instructions for` block naming the run's working directory; its hash equals that of the snapshot's `CLAUDE.md` (`data/context-hashes.txt`).

## Context and prompt

### Context the Claude fan's round 1 had

The prompt carries the PATTERN directive a production burler reviewer gets: `harness/build.sh` fills the code commit's `pattern-directive-review-fix.md` with that commit's `PATTERN.md` and the decision record's declared `PATTERN-owner-machine` change, the text `pattern.Directive` returns for `RoleReviewFix`, and puts it in the explore instruction file.
The snapshot's root holds `CLAUDE.md` copied to `AGENTS.md`.
The decision record declares that `PATTERN-owner-machine` change, so the fragment of declared changes is in the directive too.

Remaining differences from the Claude round's context:

- Plugin skills a Claude session loads are absent; Codex has no equivalent loaded.
- The parent and edit directives are empty, as in every earlier Codex bench.
- The `CLAUDE.md` is the snapshot's, not the current one, as the Claude round saw.
- The sibling bench's context files did not exist when this run was built, so they are built here from the shared description in the two board entries; the hashes in `data/context-hashes.txt` let a reader compare them with the sibling's.

### The tuned block

`data/prompts/tuned-block.md` holds the block, written once.
It tells the reviewer to check each card's claims about existing code by reading that code and its callers and the paths the change reaches, to read the `pattern/` background of every PATTERN entry the plan touches and check where each new command, package or file appears, and to read the tests the plan adds or changes for waits, sleeps and timing.
It names kinds of reading and no defect of the reference.
`harness/build-forkfan.sh` appends the block to the handler prompt and inserts it, as one line, into the fork boilerplate that the cluster rules give each fork.
The cap of 6 commands per fork stayed as the stencil sets it, and the forks used 6, 5 and 6.

## The handler's findings

The handler returned `BLOCKING` with 10 findings: 3 of its own and 7 from the forks.
The forks returned 8: `affected-set` 2, `guard-reach` 4 and `way-forward` 2.
`data/match.tsv` holds each reading, with the dropped fork finding last.

| Finding | Severity | Origin | Defect | Grade |
|---|---|---|---|---|
| `F1` | `BLOCKING` | `guard-reach` | Existing runs without a local batten record receive no owner judgment | shared, fan `F8` |
| `F2` | `BLOCKING` | `guard-reach` | Foreground `loom run` and `loom step` still execute another machine's run | `unique-real` |
| `F3` | `BLOCKING` | `guard-reach` | Skip flags certify an owner claim that was never published | `unique-real` |
| `F4` | `BLOCKING` | `affected-set` | `adoptcli` is missing from `PATTERN-cli-cobra`'s exceptions | shared, fan `F2` |
| `F5` | `BLOCKING` | handler | The deferred-push test waits a production second and its fixture blocks on the lock | shared, fan `F3` |
| `F6` | `MEDIUM` | `way-forward` | Past-merge recovery stops at the parent fetch and the divergence guard | `unique-real` |
| `F7` | `MEDIUM` | `affected-set` | Unarmed `wire` callers reach the claim callback with a nil setter | `unique-real` |
| `F8` | `MEDIUM` | handler | The `Lost` calculation catches an absent entry that card 8 passes | `unique-real` |
| `F9` | `MEDIUM` | handler | The adopt card bundles three cross-package public APIs | shared, fan `F9` |
| `F10` | `LOW` | `guard-reach` | An exactly one-minute sleep does not trigger the planned check | `unique-real` |

- `F3` carries the tag `handler` in the review file, but `guard-reach` reported it.
- `F5` goes beyond the fan's `F3`: the handler also finds that the fixture holds the push lock the retry's pull blocks on.
- The fan findings not found are `F1` (`BLOCKING`), `F4`, `F5`, `F6`, `F7`, `F10` and `F11`.
- `F3`, `F6`, `F8` and `F10` rest on readings the earlier benches also graded `unique-real`; `F8` stays arguable, since the decision record says only that an absent entry has no owner.

## Method

### Inputs and prompts

`P4` is rebuilt with `data/harness/build.sh` from the earlier benches' weft commit and code commit in `data/inputs.txt`.
`build.sh` differs from the forked fan bench's by the directive and `AGENTS.md`; `build-forkfan.sh` differs by the block.
`build-fan.sh`, `dump.sh`, `setup-codex-home.sh` and `windows.sh` are the forked fan bench's with the path changed, and `run-forkfan.sh` is its runner.
The prompts are in `data/prompts/`: the handler prompt with the block, the two instruction files it reads, and the prompt as the run rewrote it for its working copy.

### Call

```text
CODEX_HOME=<scratch>/codex-home codex exec -m gpt-6-astra -c model_reasoning_effort="high" \
  -s workspace-write -C <work> --skip-git-repo-check --json -o <last-message> - < prompt
```

`run-forkfan.sh` runs the one call, then copies every rollout the call created.
The scratch `CODEX_HOME` holds only a link to the operator's `auth.json`, so the operator's `config.toml` did not apply.
`spawn_agent` with `fork_turns` was available with the stable `multi_agent` feature, as in the forked fan bench; no feature flag was set.

### Grading

One Claude session graded every handler finding as the earlier benches did: against the fan's round-1 findings of `P4`, and an unmatched finding against the cited card lines and the snapshot's code.
`shared` means the finding names the same defect as a fan finding; `unique-real` means the claim holds and the fan's round has no finding of it; `false` means it does not hold.
The new claims of `F2`, `F7` and the dropped finding were checked against `arm.go`, `lifecycle_integration_test.go`, `main.go` and card 13.

## Deviations and limits

- One input, one run, one grader: no variance is measured, and a repeat could change a count of one or two.
- Context, `AGENTS.md` and the block changed together against the forked fan bench, so the table cannot say which one moved a figure; the sibling bench's context-only run is the comparison.
- The grader is a Claude session and may favour its own family's phrasing; the rates are not validated.
- The fork prompts are encrypted in the rollout; that each fork's text held the block rests on the handler following the cluster rules and on the forks' reading of `pattern/PATTERN-cli-cobra.md` and the test cards.
- The weekly window read 9 percent at the call's first request and 18 at its end, but the sibling bench drew on the same account over the same hours, so the points cannot be attributed to this run; the stop at 70 percent was never near.
- Recall is against the fan's list, not against the truth; the `unique-real` findings hold on reading, so the fan reference is not complete for `P4`.
- The operator's `~/.codex/config.toml` was not hashed before or after this bench.

## Data

`data/` holds:

- `inputs.txt`: the input with weft commit, round start time and code commit.
- `usage.txt`: one line per call with wall seconds, tokens and the weekly `used_percent`, then the handler's verdict.
- `rollout-evidence.txt`: start, end, parent thread, commands, first request and totals of each rollout, the handler's `spawn_agent` calls and the `AGENTS.md` loads.
- `context-hashes.txt`: the hashes of `AGENTS.md`, `CLAUDE.md`, `PATTERN.md` and the block.
- `match.tsv`: the grading of every handler finding and the dropped fork finding.
- `reviews/P4/`: the handler's review and the three forks' final messages.
- `prompts/`: the handler prompt, the two instruction files it reads, the block and the run's rewritten prompt.
- `harness/`: `build.sh`, `build-fan.sh`, `build-forkfan.sh`, `run-forkfan.sh`, `setup-codex-home.sh`, `windows.sh` and `dump.sh`, working on `.scratch/bench/`, which is not committed.
