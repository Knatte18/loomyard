# Codex Astra forked fan with block v2 and a 20-call fork cap on plan P2: one high run against two medium

Bench of 2026-10-27, a follow-up to `docs/research/codex-astra-tuned-prompt-bench-2026-10-27/report.md`.
Codex CLI 0.164.0, ChatGPT sign-in (plan `prolite`), Linux.
Three `codex exec` handler runs of `gpt-6-astra` on the plan `P2` (`quarry-csharp`), each spawning three lens forks through `spawn_agent` with `fork_turns="all"`: one at `high` effort and two at `medium`, identical but for effort.
The lenses are the Claude fan's round 1 lenses on `P2`: `affected-set`, `guard-reach` and `way-forward`.
Nothing was built: the bench is shell scripts under `data/harness/`, the reviewers' own output and a hand grading.
The sibling benches `codex-v2-p1-bench` and `codex-v2-p4-bench` ran the same setup on `P1` and `P4` on the same account at the same time.

## Conclusion

Each run found 1 or 2 of the Claude fan's 13 findings on `P2`; together the three runs found 3 of 13 and all 3 `BLOCKING` ones, each `BLOCKING` finding by a different set of runs.
Every run returned the fan's verdict (`BLOCKING`), and none returned a `false` finding.

| Fan `BLOCKING` finding | High | Medium 1 | Medium 2 |
|---|---|---|---|
| `F1`: the `declaredNameIndex` fallback picks `IProvider` in card 9's own row | yes, as `F3` | yes, as `F3` | yes, as `F2` |
| `F2`: the spaced `IPair<string, int>` head answers `invalid_container` in quarry | no | no | yes, as `F6` |
| `F3`: card 4's `go` row reports the finding it claims absent | yes, as `F5` | no | no |

- One high run and the union of two medium runs found the same count of fan findings, 2 of 13, and 2 of 3 `BLOCKING`, though not the same two `BLOCKING` findings; the high run found `F1` and `F3`, the union of the medium runs `F1` and `F2`.
- The union of the medium runs returned 7 distinct findings against the high run's 5, and 5 `unique-real` against 3, at 429k fresh input tokens against 290k, and 370 s against 673 s of wall time when the two medium runs run in parallel.
- The two medium runs differ from each other as much as either differs from the high run: 1 against 2 fan findings, 4 against 6 findings, and 3 of their 7 distinct findings in common.
  One run per setting cannot rank the settings; the first repeat measure of these benches says a difference of one fan finding is inside the run-to-run spread.
- Block v2 and the 20-call cap did not reach the findings that need a read beyond the cards.
  The forks ran 2 to 7 commands each, far under the cap of 20, and the fan's `F4` to `F13` were found by no run.
  The `way-forward` fork returned no finding in any run, so none of the fan's findings that its lens produced (`F6`, `F7`, `F8`, `F11`, `F12`) was found.
- The reading that block v2's second clause asks for, every reader of a changed symbol, produced the finding all three runs share and the fan lacks: the new status and `Unit` readers of cards 6, 8, 11 and 12 are missing from `allowedStatusConsumers` in `internal/planglyph/status_enforcement_test.go`.

## Summary table

One run per cell, `P2` only.
The fan reference is 13 findings, 3 of them `BLOCKING`, from `opus` at medium effort with 3 lens forks, as the earlier benches recorded.
"Found" counts distinct fan findings that a finding of the setting matched.
The window is the weekly window's `used_percent` at the first and last `token_count` of the setting's handler rollouts; the sibling benches drew on the same account over the same minutes, so the points cannot be attributed to a setting.

| Setting | Findings | Shared with fan | Unique real | False | Fan findings found | `BLOCKING` found | Verdict as fan's | Fresh input tokens | Window points | Wall time |
|---|---|---|---|---|---|---|---|---|---|---|
| `gpt-6-astra` high, forked fan, block v2 | 5 | 2 | 3 | 0 | 2 of 13 | 2 of 3 | yes | 290k | 29 to 54 | 673 s |
| `gpt-6-astra` medium 1, forked fan, block v2 | 4 | 1 | 3 | 0 | 1 of 13 | 1 of 3 | yes | 192k | 29 to 47 | 255 s |
| `gpt-6-astra` medium 2, forked fan, block v2 | 6 | 2 | 4 | 0 | 2 of 13 | 2 of 3 | yes | 237k | 29 to 51 | 370 s |
| Union of the two medium runs | 7 | 2 | 5 | 0 | 2 of 13 | 2 of 3 | yes | 429k | 29 to 51 | 370 s in parallel |

### Earlier rows for `P2`

These come from `docs/research/codex-review-bench-2026-10-25/`; the later Astra benches did not run `P2`.
Fresh input tokens for Astra and Sol are `input_tokens` less `cached_input_tokens` of the rollout's last `token_count`; Opus counts cache creation as fresh input, so the figures are not the same measure.

| Setting | Findings | Shared with fan | Unique real | False | Fan findings found | `BLOCKING` found | Verdict as fan's | Fresh input tokens | Wall time |
|---|---|---|---|---|---|---|---|---|---|
| Claude `opus` medium, fan | 13 | | | | | | | 359k, plus 71k in forks | 609 s |
| `gpt-6.1-sol` medium, solo | 1 | 1 | 0 | 0 | 1 of 13 | 1 of 3 | yes | 88k | 224 s |
| `gpt-6-luna` medium, solo | 0 | 0 | 0 | 0 | 0 of 13 | 0 of 3 | no (`APPROVED`) | 50k | 39 s |
| `gpt-6-astra` medium, solo | | | | | | | | | refused in 2 s by the usage limit, no result |

## Per call

| Run | Call | Wall time | Fresh input tokens | Output tokens | Reasoning tokens | Commands |
|---|---|---|---|---|---|---|
| high | Handler | 673 s | 218k | 14.7k | 5.9k | 27 |
| high | Fork `affected-set` | 240 s | 30k | 4.3k | 2.3k | 7 |
| high | Fork `guard-reach` | 253 s | 16k | 4.9k | 2.6k | 6 |
| high | Fork `way-forward` | 131 s | 26k | 2.3k | 1.4k | 3 |
| medium 1 | Handler | 255 s | 120k | 7.3k | 1.6k | 17 |
| medium 1 | Fork `affected-set` | 95 s | 31k | 2.3k | 0.8k | 5 |
| medium 1 | Fork `guard-reach` | 87 s | 26k | 2.2k | 0.6k | 4 |
| medium 1 | Fork `way-forward` | 53 s | 15k | 1.2k | 0.5k | 2 |
| medium 2 | Handler | 370 s | 135k | 10.0k | 3.0k | 19 |
| medium 2 | Fork `affected-set` | 127 s | 37k | 3.0k | 1.0k | 7 |
| medium 2 | Fork `guard-reach` | 156 s | 38k | 3.9k | 1.5k | 6 |
| medium 2 | Fork `way-forward` | 72 s | 27k | 1.8k | 0.7k | 4 |

The handler's figures are its own thread's, so the forks' rows add to it; a handler's wall time is the whole `codex exec`, and a fork's runs from spawn to its last message.
A fork's command count is its own: its rollout also holds the handler's commands made before the spawn, 21 in the high run, 9 in medium 1 and 11 in medium 2, which `data/harness/evidence.sh` subtracts.
`data/usage.txt` holds every figure with the window readings per rollout.

## Fork evidence

The rollouts are the check the task asked for; `data/rollout-evidence.txt` holds the figures.
All three runs pass it.

- Started: each handler's rollout holds three `spawn_agent` calls, `affected_set`, `guard_reach` and `way_forward`, each with `fork_turns` set to `"all"`, about 10 s apart.
  The fork bodies are encrypted, so the fork prompts themselves cannot be read.
- Concurrent: in the high run the forks ran 13:28:59 to 13:32:59, 13:29:09 to 13:33:22 and 13:29:19 to 13:31:30 UTC, overlapping from 13:29:19 to 13:31:30; medium 1 overlaps from 13:27:27 to 13:28:20 and medium 2 from 13:27:39 to 13:28:51.
- Inherited context: each fork's `session_meta` carries `forked_from_id` of its handler's session, and its first request already held 204k to 205k input tokens in the high run, 99k to 100k in medium 1 and 109k to 110k in medium 2, against 16k for each handler's first.
  The high handler had made 21 commands before it spawned, against 9 and 11 for the medium handlers, so its forks inherited more reading.
- `AGENTS.md` loaded: every rollout, handler and fork, holds an `AGENTS.md instructions for` block; the hash of the snapshot's `AGENTS.md` equals that of its `CLAUDE.md` (`data/context-hashes.txt`).

## The findings

Each handler returned the fan's verdict, `BLOCKING`.
No handler dropped a finding its forks returned: every fork finding is in its handler's list, and the `way-forward` forks returned none.
`data/match.tsv` holds each reading.

| Defect | Severity (own) | High | Medium 1 | Medium 2 | Grade |
|---|---|---|---|---|---|
| `declaredNameIndex` fallback picks `IProvider` | `MEDIUM` | `F3` | `F3` | `F2` | shared, fan `F1` |
| Spaced `IPair<string, int>` head and quarry's `checkMintedContainer` | `MEDIUM` | | | `F6` | shared, fan `F2` |
| Card 4's dotted `Glyphchain.Sharp#` is a file scope under `go` | `MEDIUM` | `F5` | | | shared, fan `F3` |
| The operator refusal of card 10 is reached by no test through `renameDeclSource` | `BLOCKING` | `F1` | `F1` | `F1` | `unique-real` |
| Cards 6, 8, 11 and 12 add status and `Unit` readers outside `allowedStatusConsumers` | `MEDIUM` | `F2` | `F4` | `F4` | `unique-real` |
| Card 3's malformed Create row also yields `glyph-rejected` | `MEDIUM` | `F4` | | | `unique-real` |
| Card 14 defers the package docs and the PATTERN text past their implementing cards | `BLOCKING` | | `F2` | | `unique-real`, arguable |
| Card 6's resolve half reads a `.cs` symbol from a `Uses` ref as a changed C# target | `MEDIUM` | | | `F3` | `unique-real` |
| `renameDeclSource` calls `ParseRef` with no plan to give it | `MEDIUM` | | | `F5` | `unique-real` |

- Astra rates the fan's three `BLOCKING` findings `MEDIUM` in every run that found them; the one `BLOCKING` finding common to all three runs, the unreached operator refusal, is not in the fan's list.
  It holds on card 10 lines 35 to 43: the only unsupported-member test calls `renameCSharpDecl` and asserts `false`, and the `Bad(Int32)` row reaches the malformed-handle branch of the same check, not this one, against `PATTERN-refusal-way-forward`.
- The status-registry finding holds on `status_enforcement_test.go` lines 64 to 99 and 135 at the snapshot, and no card names `allowedStatusConsumers`.
- Card 14's finding rests on `CLAUDE.md`'s rule that docs land in the same commit as the code; a plan may deliberately order its docs card last, so the grade is arguable.
- The fan findings found by no run are `F4` to `F13`: the language-mismatch evidence of card 6 (`F4`; medium 2's `F3` is a different defect on the same card), the `Lang` source of card 3's fallback, the way forward of a rejected C# Create body, the way forwards of cards 7, 8, 11 and 12, and the `LOW` and `NIT` findings of cards 5 and 13.

## How far the two medium runs differ

| Measure | Medium 1 | Medium 2 | In common |
|---|---|---|---|
| Findings | 4 | 6 | 3 of 7 distinct |
| Fan findings found | 1 (`F1`) | 2 (`F1`, `F2`) | `F1` |
| `BLOCKING` fan findings found | 1 of 3 | 2 of 3 | `F1` |
| `unique-real` findings | 3 | 4 | 2 |
| Fresh input tokens | 192k | 237k | |
| Wall time | 255 s | 370 s | |

- The shared findings are the fallback (`F1`), the unreached operator refusal and the status registry; medium 1 alone found the deferred docs, medium 2 alone the `Uses` reading, the missing plan parameter and the spaced head.
- Medium 2's handler read more before it spawned (11 commands against 9) and ran 45 percent longer; its forks ran 7, 6 and 4 commands against 5, 4 and 2.
  The spaced-head finding, the only fan `BLOCKING` finding a medium run found that the other did not, came from the handler's own reading after the forks returned, not from a fork.

## Method

### Inputs and prompts

`P2` is rebuilt with `data/harness/build.sh` from the weft commit `394715f7f` and the code commit `750903aef` in `data/inputs.txt`, which `docs/research/codex-review-bench-2026-10-25/data/inputs.txt` records.
The round's focus file is part of the input, as in the Claude round.
`build.sh`, `build-fan.sh`, `dump.sh` and `windows.sh` are the tuned-prompt bench's with the path changed.
`build-forkfan.sh` differs by the block file and by one added edit: the cap sentence's `at most 6 tool calls` becomes `at most 20 tool calls`, and the script fails when the old number remains or the new one is absent.
`run-forkfan.sh` takes the run name and the effort and gives each run its own `CODEX_HOME` (`setup-codex-home.sh <run>`), so the concurrent runs' rollouts stay apart; `evidence.sh` is new.

The context is the tuned-prompt bench's: the PATTERN overview inlined as production renders it, and the snapshot's `CLAUDE.md` as `AGENTS.md`.
`data/prompts/block-v2.md` holds block v2, word for word as the board entry gives it, and `build-forkfan.sh` appends it to the handler prompt and inserts it, as one line, into the fork boilerplate.
The prompts are in `data/prompts/`: the handler prompt with the block, the two instruction files it reads, and the prompt as the high run rewrote it for its working copy.

### Call

```text
CODEX_HOME=<scratch>/codex-home-<run> codex exec -m gpt-6-astra -c model_reasoning_effort="<high|medium>" \
  -s workspace-write -C <work> --skip-git-repo-check --json -o <last-message> - < prompt
```

The three calls started within one second of each other, at a weekly window of 29 percent.
The scratch `CODEX_HOME` of each run holds only a link to the operator's `auth.json`, so the operator's `config.toml` did not apply.
No feature flag was set.

### Grading

One Claude session graded every handler finding as the earlier benches did, against the fan's round-1 findings of `P2` (`docs/research/codex-review-bench-2026-10-25/data/claude-findings.tsv`), and an unmatched finding against the cited card lines and the snapshot's code.
`shared` means the finding names the same defect as a fan finding; `unique-real` means the claim holds and the fan's round has no finding of it; `false` means it does not hold.
The union row counts a defect once whichever medium run found it.

## Deviations and limits

- One input, one run per setting and one grader: the two medium runs show a spread of one fan finding and three findings, so a gap of that size between settings is not a result.
- The window figures are shared with the siblings and with each other: three runs and the sibling benches drew on the weekly window at once, so a run's own points are unknown.
  The window reached 54 percent at the highest reading, under the 70 percent stop.
- The union row's wall time assumes the two medium runs in parallel, as they ran; its fresh tokens are the sum.
- The grader is a Claude session and may favour its own family's phrasing; the `unique-real` rates are not validated, and the card 14 grade is arguable.
- The fork prompts are encrypted in the rollouts; that each fork's text held block v2 and the cap of 20 rests on the build script's checks and on the handler following the cluster rules.
- Recall is against the fan's list, not against the truth; the `unique-real` findings hold on reading, so the fan reference is not complete for `P2`.
- No run of block v1 on `P2` exists, so the report cannot say what block v2 changed on this plan; the sibling benches on `P1` and `P4` have the earlier block's rows.
- The operator's `~/.codex/config.toml` was not hashed before or after this bench.

## Data

`data/` holds:

- `inputs.txt`: the input with weft commit, round start time and code commit.
- `usage.txt`: one line per call with wall seconds, tokens, command count and the weekly `used_percent` at the call's first and last `token_count`, then each handler's verdict.
- `rollout-evidence.txt`: start, end, parent thread, commands, first request and totals of each rollout, the handler's `spawn_agent` calls and the `AGENTS.md` loads, per run.
- `context-hashes.txt`: the hashes of `AGENTS.md`, `CLAUDE.md`, `PATTERN.md` and block v2.
- `match.tsv`: the grading of every handler finding.
- `reviews/P2/<run>/`: each handler's review and its three forks' final messages.
- `prompts/`: the handler prompt, the two instruction files it reads, block v2 and the run's rewritten prompt.
- `harness/`: `build.sh`, `build-fan.sh`, `build-forkfan.sh`, `run-forkfan.sh`, `setup-codex-home.sh`, `windows.sh`, `dump.sh` and `evidence.sh`, working on `.scratch/bench/`, which is not committed.
