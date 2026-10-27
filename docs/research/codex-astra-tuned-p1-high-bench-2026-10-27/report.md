# Codex Astra forked fan with a tuned reviewer prompt on the plan P1

Bench of 2026-10-27, the `P1` counterpart of `docs/research/codex-astra-tuned-prompt-bench-2026-10-27/report.md`, which ran the same setup on `P4`.
Codex CLI 0.164.0, ChatGPT sign-in (plan `prolite`), Linux.
One `codex exec` handler run on the plan `P1` (`gate-runs-once`, 33 cards), `gpt-6-astra` at high effort for the handler and its forks, one run, no repeat.
The handler read the plan, then spawned three lens forks through `spawn_agent` with `fork_turns="all"`.
Every condition of the `P4` bench is kept: the harness, the inlined PATTERN overview, `AGENTS.md` and the tuned reading block, unchanged; only the input changed.
Nothing was built: the bench is shell scripts under `data/harness/`, the reviewers' own output and a hand grading.

## Conclusion

On `P1` the forked fan found 4 of the Claude fan's 12 findings and 1 of its 2 `BLOCKING` ones; on `P4` it found 4 of 11 and 2 of 3.
The handler returned the fan's verdict (`BLOCKING`) and 10 findings: 4 shared with the fan, 6 `unique-real`, none `false`.
Every earlier `P1` row of a solo run found 2 of the fan's 12 findings.

| Fan `BLOCKING` finding | Found | By |
|---|---|---|
| `F1`: card 23 shortens `PATTERN.md` only after cards 3, 7 and 19 grow it | yes, as `F1` | the handler |
| `F2`: the hand-enumerated readers of cards 1, 6 and 14 miss members | no | |

- `F1` is the finding the solo Opus run also found and no solo Astra run found.
  The handler adds that moving card 23 earlier is not enough, because the last growing card crosses the headroom line again, and spreads the shortening over the growing cards instead of moving the card.
- `F2` is the fan's `affected-set` lens finding.
  This run's `affected-set` fork returned one finding, on other readers (the tree-sitter link guard and the tmuxkit permission switch, both real); no fork or handler finding names `templateDefaults`, `TestCardGateCommand`, `TestRecoverSpawnOrAttach`, `TestBeginBatch_PromptFile`, the retired `tree_scope_verify` helpers or the `guard_tier` mentions.
- Of the fan's 12 findings, 4 were found: `F1` (`BLOCKING`), `F3` and `F4` (`MEDIUM`), `F8` (`LOW`).
  Not found: `F2`, `F5`, `F6`, `F7`, `F9`, `F10`, `F11`, `F12`.
- Astra rates 6 findings `BLOCKING`, against the fan's 2.
  The 5 it rates `BLOCKING` beyond the fan's `F1` are all `unique-real`; `F6` is the one solo Opus rated `LOW`.
- The handler dropped none of the forks' findings.
  The forks returned 6 findings, and the handler merged two of them (the `way-forward` fork's reconcile finding and the `guard-reach` fork's retired-list finding) into one.
- The run took 1114 s and 412k fresh input tokens in all, against 950 s and 631k for the `P4` run.

One input, one run per cell and one grader cannot separate the plan from the setup: the `P1` and `P4` results differ in the plan, the fan reference and chance at once.

## Summary table

One run per cell, `P1` only.
The fan reference is 12 findings, 2 of them `BLOCKING`, from `opus` at medium effort with 3 lens forks, as the earlier benches recorded.
"Found" counts distinct fan findings that a finding of the setting matched.

| Setting | Findings | Shared with fan | Unique real | False | Fan findings found | `BLOCKING` found | Verdict as fan's | Fresh input tokens | Wall time |
|---|---|---|---|---|---|---|---|---|---|
| `gpt-6-astra` high, forked fan, tuned prompt and parity context | 10 | 4 | 6 | 0 | 4 of 12 | 1 of 2 | yes | 412k | 1114 s |
| `gpt-6-astra` medium, solo | 2 | 2 | 0 | 0 | 2 of 12 | 0 of 2 | yes | 93k | 184 s |
| Claude `opus` medium, solo | 6 | 3 | 3 | 0 | 2 of 12 | 1 of 2 | yes | 206k | 563 s |
| Codex `gpt-6.1-sol` medium, solo | 7 | 4 | 3 | 0 | 2 of 12 | 0 of 2 | yes | 156k | 491 s |
| Claude `opus` medium, fan | 12 | | | | | | | 384k, plus 92k in forks | 604 s |

The `P4` row of this setup, for reference: 10 findings, 4 shared, 6 `unique-real`, 4 of 11 found, 2 of 3 `BLOCKING`, 631k, 950 s.
No other Astra setting of the earlier benches ran on `P1`; their `P4` rows are in their own reports.
The `sol` row is `codex-review-bench-2026-10-25`: its 7 findings include 4 shared findings, of which two name fan `F4` and two fan `F3`; the table counts its distinct fan findings found.
Fresh input tokens for Astra and `sol` are `input_tokens` less `cached_input_tokens`; Opus counts cache creation as fresh input, so the figures are not the same measure.
This run's figure sums the handler and its three forks.

## Per call

| Call | Wall time | Fresh input tokens | Output tokens | Commands | Weekly window after |
|---|---|---|---|---|---|
| Handler | 1114 s | 315k | 22.6k | 45 | 28 percent |
| Fork `affected-set` | 184 s | 39k | 4.1k | 6 | 25 percent |
| Fork `guard-reach` | 219 s | 33k | 4.6k | 5 | 26 percent |
| Fork `way-forward` | 209 s | 26k | 4.8k | 6 | 26 percent |

The handler's figures are its own thread's, so the forks' rows add to it.
The forks' wall times run from spawn to their last message, and their window readings come from the last `token_count` in each fork's rollout, so they are not a sequence.
The handler's wall time is the whole `codex exec`: 190 s of reading before the first spawn, 230 s more until the last fork returned, and 691 s of consolidation.
The forks' command counts are their own: each fork's rollout also holds the 22 commands of the handler's reading, which it inherited.

Each fork's first request held about 183k input tokens, of which 180k were cache hits, so a fork's fresh reading before its own commands was about 3k.
The `P4` run's forks started from 196k with 112k cached.
The cache hit count is as measured; the cause of the difference is not known.

## Fork evidence

The rollouts are the check the entry asked for; `data/rollout-evidence.txt` holds the figures.

- Started: the handler's rollout holds three `spawn_agent` calls, `affected_set`, `guard_reach` and `way_forward`, each with `fork_turns` set to `"all"`, at 12:44:17, 12:44:28 and 12:44:35 UTC.
  The fork bodies are encrypted, so the fork prompts themselves cannot be read.
- Concurrent: the forks ran 12:44:17 to 12:47:21, 12:44:28 to 12:48:07 and 12:44:35 to 12:48:04, so all three overlap from 12:44:35 to 12:47:21.
- Inherited context: each fork's `session_meta` carries `forked_from_id` of the handler's session, and its first request already held 182.6k, 183.0k and 183.5k input tokens against 16k for the handler's first.
  Each fork's rollout holds the handler's 22 earlier commands, including the reads of the `pattern/` background files.
- `AGENTS.md` loaded: the handler's rollout, and each fork's, holds an `AGENTS.md instructions for` block naming the run's working directory; its hash equals that of the snapshot's `CLAUDE.md` (`data/context-hashes.txt`).

## Context and prompt

The context is the `P4` bench's: `harness/build.sh` fills the code commit's `pattern-directive-review-fix.md` with that commit's `PATTERN.md` and the decision record's declared `PATTERN` changes, and the snapshot's `CLAUDE.md` is copied to `AGENTS.md`.
The `P1` input also carries the round-1 focus file, which `build.sh` routes into the explore instruction as it did for the Claude fan.
`data/prompts/tuned-block.md` is the `P4` bench's block, byte for byte; `build-forkfan.sh` appends it to the handler prompt and inserts it, as one line, into the fork boilerplate.
The lenses are the Claude fan's round 1 on `P1`, `affected-set`, `guard-reach` and `way-forward`, as `round-1-usage.yaml` of the weft records them.
Remaining differences from the Claude round's context are those of the `P4` bench: no plugin skills, empty parent and edit directives, and the snapshot's `CLAUDE.md`.
The review step's read-only rule ("read files, run nothing") applied to the handler as it did to the Claude round; the handler named in its review that it ran no `validate-plan` overlay and withheld four concrete patches for that reason.

## The handler's findings

The handler returned `BLOCKING` with 10 findings: 5 of its own and 5 from the forks.
The forks returned 6: `affected-set` 1, `guard-reach` 2 and `way-forward` 3.
`data/match.tsv` holds each reading.

| Finding | Severity | Origin | Defect | Grade |
|---|---|---|---|---|
| `F1` | `BLOCKING` | handler | `PATTERN.md` growth fails the per-card gate before card 23 can shorten it | shared, fan `F1` |
| `F2` | `BLOCKING` | `affected-set` | Two tag consumers stay on the old spellings: the tree-sitter link guard's `go list` and the tmuxkit permission switch | `unique-real` |
| `F3` | `BLOCKING` | `guard-reach` | Card 24's Verify regex accepts `-tags nottier0` for `tier0` | `unique-real` |
| `F4` | `BLOCKING` | `way-forward` | Card 19's default-layer way forward edits the prime's own batten file | `unique-real` |
| `F5` | `BLOCKING` | `way-forward` | `commandRefWayForward` still tells the operator to add the `switch` key card 20 forbids | `unique-real` |
| `F6` | `BLOCKING` | handler | Card 20's new forced-window test repeats an existing case | `unique-real` |
| `F7` | `MEDIUM` | `guard-reach`, `way-forward` | The retired-list refusal is not reached through `LoadConfig`, and its reconcile alternative is a dry run | shared, fan `F8` |
| `F8` | `MEDIUM` | handler | The retag cards use `Custom` on a false premise | shared, fan `F3` |
| `F9` | `MEDIUM` | handler | `darnTreeScope` and `guardRunOf` lack the re-sign arrow | shared, fan `F4` |
| `F10` | `MEDIUM` | handler | Cards 10 and 19 bundle exported cross-package symbols into feature cards | `unique-real` |

- `F2`, `F3`, `F4` and `F5` were checked against the snapshot's files and hold: `cmd/lyx/treesitterlink_integration_test.go:40` and `internal/testkit/tmuxkit/allow_tmux.go:1`; the regex on card 24's Verify line, which has no boundary before the tier name; `batten` registered without `HubWide` in `configreg.go` and `selectedLayer` in `configcli/hubwide.go`; and `internal/shedrecipe/entries_command.go:19`.
- `F4` shares card 19's sentence with the fan's `F6` and is a different defect: the fan's `F6` is the `run_settings` half, which this run did not find.
- `F6` is the finding solo Opus filed on `P1` as `LOW`; it holds on the card text and was not run.
- `F7` goes beyond the fan's `F8`: `configengine.Load` fills the template and rejects the sequence in `gates.card` before `decodeConfig` runs (`internal/gateslot/config.go:134-141`, `internal/yamlengine/fill.go:112-116`).
- `F10` stays arguable: each symbol has one consumer in the plan, and the rubric's Granularity rule asks for a card of its own for a public or cross-package symbol even then.

## Method

### Inputs and prompts

`P1` is rebuilt with `data/harness/build.sh` from the solo bench's weft commit and code commit in `data/inputs.txt`.
The harness is the `P4` bench's `data/harness/` with the repository root and the harness directory changed to this worktree and bench directory; no other line changed.
The prompts are in `data/prompts/`: the handler prompt with the block, the two instruction files it reads, the block, and the prompt as the run rewrote it for its working copy.

### Call

```text
CODEX_HOME=<scratch>/codex-home codex exec -m gpt-6-astra -c model_reasoning_effort="high" \
  -s workspace-write -C <work> --skip-git-repo-check --json -o <last-message> - < prompt
```

`run-forkfan.sh` runs the one call, then copies every rollout the call created.
The scratch `CODEX_HOME` holds only a link to the operator's `auth.json`, so the operator's `config.toml` did not apply.
`spawn_agent` with `fork_turns` was available with the stable `multi_agent` feature; no feature flag was set.

### Grading

One Claude session graded every handler finding as the earlier benches did: against the fan's round-1 findings of `P1` in the weft's `round-1-review.md`, and an unmatched finding against the cited card lines and the snapshot's code.
`shared` means the finding names the same defect as a fan finding; `unique-real` means the claim holds and the fan's round has no finding of it; `false` means it does not hold.

## Deviations and limits

- One input, one run, one grader: no variance is measured, and a repeat could change a count of one or two.
- The grader is a Claude session and may favour its own family's phrasing; the rates are not validated.
- The fork prompts are encrypted in the rollout; that each fork's text held the block rests on the handler following the cluster rules and on the forks' reading of the `pattern/` files and the test cards.
- The weekly window read 18 percent at the call's first request and 28 at its end, but the sibling benches `codex-astra-tuned-p1-med-bench` and `codex-astra-tuned-p4-med-bench` ran on the same account over the same hours, so the 10 points cannot be attributed to this run; the stop at 70 percent was never near.
- Recall is against the fan's list, not against the truth; the `unique-real` findings hold on reading, so the fan reference is not complete for `P1`.
- `F4`'s default-layer reading was checked on `configreg.go`, `selectedLayer` and the fork's cited lines, and not run.
- The operator's `~/.codex/config.toml` was not hashed before or after this bench.

## Data

`data/` holds:

- `inputs.txt`: the input with weft commit, round start time and code commit.
- `usage.txt`: one line per call with wall seconds, tokens and the weekly `used_percent`, then the handler's verdict.
- `rollout-evidence.txt`: start, end, parent thread, commands, first request and totals of each rollout, the handler's `spawn_agent` calls, the `AGENTS.md` loads and the handler's reads of `pattern/` files.
- `context-hashes.txt`: the hashes of `AGENTS.md`, `CLAUDE.md`, `PATTERN.md` and the block.
- `match.tsv`: the grading of every handler finding.
- `reviews/P1/`: the handler's review and the three forks' final messages.
- `prompts/`: the handler prompt, the two instruction files it reads, the block and the run's rewritten prompt.
- `harness/`: `build.sh`, `build-fan.sh`, `build-forkfan.sh`, `run-forkfan.sh`, `setup-codex-home.sh`, `windows.sh` and `dump.sh`, working on `.scratch/bench/`, which is not committed.
