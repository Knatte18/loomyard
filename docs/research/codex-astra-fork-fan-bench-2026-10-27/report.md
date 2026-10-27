# Codex Astra fan with real forks against the Claude fan on one plan

Bench of 2026-10-27, a follow-up to `docs/research/codex-astra-fan-bench-2026-10-27/report.md`.
Codex CLI 0.164.0, ChatGPT sign-in (plan `prolite`), Linux.
One `codex exec` handler run on the plan `P4` (`machine-handover`), `gpt-6-astra` at high effort for the handler and its forks, one run, no repeat.
The handler read the plan once, then spawned three lens forks through `spawn_agent` with `fork_turns="all"`.
Nothing was built: the bench is shell scripts under `data/harness/`, the reviewers' own output and a hand grading.

## Conclusion

The forked fan found 1 of the Claude fan's 11 findings on `P4` and none of its 3 `BLOCKING` ones.
The cold-call Astra fan found none of the 11 and solo Astra 2, so real forks at high effort did not raise recall against this reference.
The handler returned the fan's verdict (`BLOCKING`) and 9 findings: 1 shared with the fan, 8 `unique-real`, none `false`.

- Rollouts confirm the run shape: three forks started 8 s apart, ran concurrently for about 2.5 minutes, were children of the handler's thread and began with the handler's reading in context (see "Fork evidence").
- The handler kept all 7 fork findings, as 6 after merging the one both `guard-reach` and `way-forward` reported, and added 3 of its own.
- Four of the 8 `unique-real` findings are the ones the cold-call fan and the solo reviewers already found: the owner-pause seam copied before the root pre-run installs it (`BLOCKING`), the skip-variable bypass (`BLOCKING`), the slug-only `Takeover` guard that a prime claim passes, and `Finalize`'s parent fetch ahead of the owner checks.
- Four are new on `P4`: adopt waiting behind a standing hub pause, an absent entry classified as `Lost`, the nil-seam policy of `internal/testkit/envkit`, and adopt's `lym loom resume` subprocess against its in-process decision.
- The one shared finding is the bundling of public symbols into the adopt card (fan `F9`).
- The fan's three `BLOCKING` findings need a read the forks did not make: the owner guard on `retryWeftPush` reading origin's Bolt outside the reconcile, the missing `adoptcli` entry in `PATTERN-cli-cobra`'s background, and the one-second production wait in `TestSetOwners`.
  No fork reported the first or the third, and none reported the `adoptcli` omission.
- The run took 737 s and 299k fresh input tokens against 907 s and 466k for the cold-call fan, and 166 s and 77k for solo Astra.
  The handler's own 213k is most of it: after the spawns it ran 22 more model turns on a context of 145k to 220k tokens, most of them while the forks ran, and the forks' reading cost 23k to 33k fresh each because the inherited prefix was cached.

With one input, one run per cell and one grader, the sample cannot rank the forked fan against the other settings.
It shows that real forks at high effort gave Astra the same kind of result as cold lens calls at medium effort: real findings the Claude fan lacks, and not the fan's own.

## Summary table

One run per cell, `P4` only.
The fan reference is 11 findings, 3 of them `BLOCKING`, from `opus` at medium effort with 3 lens forks, as the earlier benches recorded.
"Found" counts distinct fan findings that a finding of the setting matched.

| Setting | Findings | Shared with fan | Unique real | False | Fan findings found | `BLOCKING` found | Verdict as fan's | Fresh input tokens | Wall time | Window points |
|---|---|---|---|---|---|---|---|---|---|---|
| `gpt-6-astra` high, forked fan | 9 | 1 | 8 | 0 | 1 of 11 | 0 of 3 | yes | 299k | 737 s | weekly 5 |
| `gpt-6-astra` medium, cold-call fan | 8 | 0 | 8 | 0 | 0 of 11 | 0 of 3 | yes | 466k | 907 s | weekly at most 3 |
| `gpt-6-astra` medium, solo | 4 | 2 | 2 | 0 | 2 of 11 | 0 of 3 | yes | 77k | 166 s | at most 15 of the 5-hour window |
| Claude `opus` medium, solo | 6 | 2 | 4 | 0 | 2 of 11 | 0 of 3 | no (`APPROVED`) | 128k | 389 s | none |
| Claude `opus` medium, fan | 11 | | | | | | | 489k, plus 79k in forks | 735 s | none |

The last four rows are the cold-call fan bench's `P4` rows.
Fresh input tokens for Astra are `input_tokens` less `cached_input_tokens` of the rollout's last `token_count`; Opus counts cache creation as fresh input, so the figures are not the same measure.
The window points are those of the weekly window only, from 3 percent before the run to 8 after.

## Per call

| Call | Wall time | Fresh input tokens | Output tokens | Commands | Weekly window after |
|---|---|---|---|---|---|
| Handler | 737 s | 213k | 17.1k | 34 | 8 percent |
| Fork `affected-set` | 176 s | 33k | 3.6k | 6 | 6 percent |
| Fork `guard-reach` | 233 s | 23k | 4.9k | 6 | 7 percent |
| Fork `way-forward` | 272 s | 30k | 5.6k | 5 | 7 percent |

The handler's figures are its own thread's: its total grows with its own turns and not with the forks', so the forks' rows add to it.
The forks' wall times are from spawn to their last message, and their window readings come from the last `token_count` in each fork's rollout, so they are not a sequence.
Wall time of the handler is the whole `codex exec`: 147 s of reading before the first spawn, about 290 s until the last fork returned, and 300 s of consolidation.

## Fork evidence

The rollouts are the first check the task asked for; `data/rollout-evidence.txt` holds the figures.

- Started: the handler's rollout holds three `spawn_agent` calls, `affected_set`, `guard_reach` and `way_forward`, each with `fork_turns` set to `"all"`, at 11:13:03, 11:13:10 and 11:13:18 UTC.
  The fork bodies are encrypted in the rollout, so the fork prompts themselves cannot be read.
- Concurrent: the three forks' intervals, 11:13:03 to 11:15:59, 11:13:10 to 11:17:03 and 11:13:18 to 11:17:50, overlap from 11:13:18 to 11:15:59.
- Inherited context: each fork's `session_meta` carries `forked_from_id` and `parent_thread_id` of the handler's session, and its first model request already held 146.7k, 147.1k and 147.4k input tokens, of which 145.4k, 145.8k and 146.2k were cache hits, against 143k for the handler's request at the spawn.
  The forks ran 6, 6 and 5 commands, within the cap of 6 that the boilerplate sets, and each ended with its tool-call log as the boilerplate asks.

## Features enabled

`spawn_agent` with `fork_turns` was available with the stable `multi_agent` feature, which is on by default in this CLI; the bench run set no feature flag.
A probe before the run (two `codex exec` calls, a handler spawning two forks that return a secret word, with and without `-c features.multi_agent_v2=true`) spawned forks with `fork_turns="all"` under both settings, so `multi_agent_v2` was not needed and was not used.
The scratch `CODEX_HOME` holds only a link to the operator's `auth.json`, so the operator's `config.toml` did not apply; the model and effort came from `-m` and `-c model_reasoning_effort`.

## The handler's findings

The handler returned `BLOCKING` with 9 findings: 3 of its own and 6 from the forks.
The forks returned 7 findings: `affected-set` 1, `guard-reach` 4 and `way-forward` 2.
The handler kept all 7, as 6 findings, since both `guard-reach` and `way-forward` reported the orch-pause defect.
`data/match.tsv` holds each reading.

| Finding | Severity | Origin | Defect |
|---|---|---|---|
| `F1` | `BLOCKING` | `guard-reach`, `way-forward` | Card 13 copies the owner-pause seam in `orchcli.Command`, built in `newRoot` before the root pre-run installs it, so a fresh process captures nil and starts without the owner pauses |
| `F2` | `BLOCKING` | handler | Card 3 lets `BOARD_SKIP_GIT` and `BOARD_SKIP_PUSH` report an unpushed owner write as recorded |
| `F3` | `MEDIUM` | `guard-reach` | A first-act prime claim is not covered by the slug-only `Takeover` guard and replaces an owner the setter's pull found |
| `F4` | `MEDIUM` | `guard-reach` | `Finalize.Call` fetches the parent tips in `holdParent` before step 1b3, so an unreachable origin never reaches the past-merge carve-out |
| `F5` | `MEDIUM` | `guard-reach` | Card 16 plans no network-gate exemption, so adopt's first reconcile waits behind a standing hub pause that the card says it preserves |
| `F6` | `MEDIUM` | `way-forward` | Card 3's post-push `Lost` classification also catches an absent entry that it deliberately skips, which card 8 passes |
| `F7` | `MEDIUM` | `affected-set` | The new optional seams of cards 10 to 12 are in no `nilLegal` entry of `internal/testkit/envkit`, and no card names it |
| `F8` | `MEDIUM` | handler | Card 16 bundles `DropLocal`, `ResumeInWindow` and `CloneSecond` into the adopt card |
| `F9` | `LOW` | handler | Card 16 spawns `lym loom resume` for a halted child, against its decision to call modules in-process |

- `F8` is the shared finding: the fan's `F9` names `DropLocal` among the bundled symbols and rates the defect `LOW`; the handler rates it `MEDIUM`.
- `F7` started as the fork's narrower finding about `TestWire_EverySeamFilled` in `battencli`; the fork asked the handler to inspect `envkit.NilSeams` before keeping it, and the handler widened the finding to the shared policy after reading it.
- `F2` and `F6` rest on a reading of the decision record's silence, and `F9` on a reading of a deliberate subprocess; they stay arguable.
- No fork finding was dropped, so `data/match.tsv` holds no dropped row.

## Method

### Inputs and prompts

`P4` is rebuilt with `data/harness/build.sh` from the earlier benches' weft commit and code commit in `data/inputs.txt`, and `build-fan.sh` builds the explore step with the fork-reading block appended, as in the cold-call fan bench.
`data/harness/build-forkfan.sh` builds the handler prompt: the round-1 reviewer template with the code commit's fan cluster rules, `burler-cluster-fan.md`.
Its Agent-tool wording becomes Codex's: one `spawn_agent` call per lens, `fork_turns` set to `"all"`, no name, model or effort given for a fork, and a wait on every fork before consolidation.
Everything else is the stencil's: the lenses of round 1 (`affected-set`, `guard-reach`, `way-forward`), the fork boilerplate with its inherited-context sentence and its cap of 6 tool calls, the handler's holistic review while the forks run, the consolidation, the `origin:` tags and the `## Rejected` section.
The only edit to the boilerplate is the rule against spawning another agent, which no longer names the Agent tool, and the git-only wording of the read-only rule, which becomes "run only commands that read".
All prompts are in `data/prompts/`.

### Call

```text
CODEX_HOME=<scratch>/codex-home codex exec -m gpt-6-astra -c model_reasoning_effort="high" \
  -s workspace-write -C <work> --skip-git-repo-check --json -o <last-message> - < prompt
```

`data/harness/run-forkfan.sh` runs the one call, after `setup-codex-home.sh` made the scratch `CODEX_HOME` and linked it to the operator's `auth.json`, then copies every rollout the call created.
The windows come from the last `token_count` event of each rollout (`windows.sh`).

### Grading

One Claude session graded every handler finding as the earlier benches did: against the fan's round-1 findings of `P4`, and an unmatched finding against the cited card lines and the snapshot's code.
`shared` means the finding names the same defect as a fan finding; `unique-real` means the claim holds and the fan's round has no finding of it; `false` means it does not hold.
A finding on the same card as a fan finding but naming another defect is `unique-real`.

## Deviations and limits

- One input, one run, one grader: no variance is measured, and a repeat could change a count of one or two.
- The grader is a Claude session and may favour its own family's phrasing; the rates are not validated.
- The effort is high where the earlier Astra rows are medium, so the forked fan differs from the cold-call fan in two things, the fork shape and the effort; the table cannot say which one moved a figure.
- The fork prompts are encrypted in the rollout; that each fork's lens text was the boilerplate and the lens alone rests on the handler following the cluster rules, and the forks' final messages starting with their lens lists.
- The handler's fresh input tokens count 22 turns of a growing context after the spawns; a cache that hits less often would change the figure.
- A probe of two short `codex exec` calls preceded the run to choose the feature; its weekly cost is inside the 3 percent that the run started from.
- The weekly window moved from 3 to 8 percent over the run, and the stop at 70 percent was never near.
- Recall is against the fan's list, not against the truth; the 8 `unique-real` findings hold on reading, so the fan reference is not complete for `P4`.
- The operator's `~/.codex/config.toml` was not hashed before or after this bench.

## Data

`data/` holds:

- `inputs.txt`: the input with weft commit, round start time and code commit.
- `usage.txt`: one line per call with return code, wall seconds, tokens and the weekly `used_percent`, then the handler's verdict.
- `rollout-evidence.txt`: start, end, parent thread, command count and tokens of each rollout, and the handler's `spawn_agent` calls.
- `match.tsv`: the grading of every handler finding.
- `reviews/P4/`: the handler's review and the three forks' final messages.
- `prompts/`: the handler prompt and the two instruction files it reads.
- `harness/`: `build.sh`, `build-fan.sh`, `build-forkfan.sh`, `run-forkfan.sh`, `setup-codex-home.sh`, `windows.sh` and `dump.sh`, working on `.scratch/bench/`, which is not committed; `run-call.sh` is the cold-call fan bench's runner, kept for its lens calls.
