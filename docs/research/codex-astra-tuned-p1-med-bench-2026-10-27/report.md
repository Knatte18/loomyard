# Codex Astra forked fan with a tuned reviewer prompt on the P1 plan at medium effort

Bench of 2026-10-27, a follow-up to `docs/research/codex-astra-tuned-prompt-bench-2026-10-27/report.md`.
Codex CLI 0.164.0, ChatGPT sign-in (plan `prolite`), Linux.
One `codex exec` handler run on the plan `P1` (`gate-runs-once`, 33 cards), `gpt-6-astra` at medium effort for the handler and its forks, one run, no repeat.
The handler read the plan, then spawned three lens forks through `spawn_agent` with `fork_turns="all"`.
Every condition of the tuned-prompt bench is kept (context, `AGENTS.md`, reading block, harness); only the input (`P4` to `P1`) and the effort (high to medium) changed.
Nothing was built: the bench is shell scripts under `data/harness/`, the reviewers' own output and a hand grading.

## Conclusion

The forked fan found 3 of the Claude fan's 12 findings on `P1` and neither of its 2 `BLOCKING` ones.
The handler returned the fan's verdict (`BLOCKING`) and 9 findings: 4 shared with the fan (3 distinct fan findings), 5 `unique-real`, none `false`.

| Fan `BLOCKING` finding | Found | By |
|---|---|---|
| `F1`: card 23 shortens `PATTERN.md` only after cards 3, 7 and 19 grow it, so their card gates fail the headroom lint | no | |
| `F2`: the cards enumerate the readers of what they change by hand and miss members (card 1's `Gates` type change breaks four test builds, cards 6 and 14 leave dead code and stale text) | no | |

- The missed `F1` needs the byte arithmetic of `PATTERN.md` against the headroom line and the card order; no clause of the block names it, and no fork or handler finding touches the order of the `PATTERN.md` cards.
- The missed `F2` is a search for every reader of a changed type, key or symbol.
  The `affected-set` fork found two other missing readers (the tree-sitter link guard's `go list` tags, and `latestDarnOutcome` on a skipped `Darn-Write`), none of the fan's.
- Found: the fan's `F3` (the retag cards use `Custom` where `Edit` fits, and the scan departs from D5) as the handler's `F7` and `F8`, the fan's `F4` (missing re-sign arrows) as `F9`, and the fan's `F8` (bare `lyx config reconcile` is a dry run) as `F4`, rated `MEDIUM` against the fan's `LOW`.
- Not found, besides the two above: the fan's `F5`, `F6`, `F7`, `F9`, `F10`, `F11` and `F12`.
  The handler's `F2` is in the sentence of the fan's `F6` (card 19's non-bool recovery) but names a different defect, so `F6` counts as not found.
- Five findings are `unique-real`, and two of them are `BLOCKING`: card 1's retired-list refusal sits behind the template fill that rejects the old shape before `decodeConfig` runs (`F1`), and card 19's recovery cannot repair a legitimate numeric operand and names the wrong layer for per-worktree modules (`F2`).
  Both rest on a reading of the snapshot's code, not a run; the handler rates both `BLOCKING`, so its verdict stands on two findings the fan's round did not make.
- The handler dropped part of one fork finding: `affected-set` reported that `rerouteCount` and batten's failure episodes do not end at a skip.
  The handler kept only the `Darn-Write` part (`F6`) and held the rest unsettled; decision record line 146 says a skip ends the segment's episode as a `Done` does, so the dropped part is `unique-real`.
- The run took 414 s and 263k fresh input tokens in all.

One input, one run per cell and one grader cannot show that medium effort, rather than the input, produced any difference from the `P4` run.

## Summary table

One run per cell, `P1`.
The fan reference is 12 findings, 2 of them `BLOCKING`, from `opus` at medium effort with 3 lens forks (`affected-set`, `guard-reach`, `way-forward`).
"Found" counts distinct fan findings that a finding of the setting matched.

| Setting | Findings | Shared with fan | Unique real | False | Fan findings found | `BLOCKING` found | Verdict as fan's | Fresh input tokens | Wall time |
|---|---|---|---|---|---|---|---|---|---|
| `gpt-6-astra` medium, forked fan, tuned prompt and parity context | 9 | 4 | 5 | 0 | 3 of 12 | 0 of 2 | yes | 263k | 414 s |
| `gpt-6-astra` medium, solo | 2 | 2 | 0 | 0 | 2 of 12 | 0 of 2 | yes | 93k | 184 s |
| Claude `opus` medium, solo | 6 | 3 | 3 | 0 | 2 of 12 | 1 of 2 | yes | 206k | 563 s |
| Claude `opus` medium, fan | 12 | | | | | | | 384k, plus 92k in forks | 604 s |

The last three rows are the solo bench's `P1` rows (`docs/research/codex-astra-solo-bench-2026-10-25/report.md`).
The review bench's `gpt-6-astra` run on `P1` failed at once (`rc=1`, 2 s), so the solo bench holds Astra's earlier `P1` row; no earlier Astra fan or forked-fan run exists for `P1`.
Fresh input tokens for Astra are `input_tokens` less `cached_input_tokens` of the rollout's last `token_count`, summed over the handler and its forks in this run; Opus counts cache creation as fresh input, so the figures are not the same measure.

### Against the tuned-prompt bench on `P4`

| Setting | Input | Findings | Shared with fan | Unique real | False | Fan findings found | `BLOCKING` found | Fresh input tokens | Wall time |
|---|---|---|---|---|---|---|---|---|---|
| `gpt-6-astra` medium, tuned forked fan | `P1` | 9 | 4 | 5 | 0 | 3 of 12 | 0 of 2 | 263k | 414 s |
| `gpt-6-astra` high, tuned forked fan | `P4` | 10 | 4 | 6 | 0 | 4 of 11 | 2 of 3 | 631k | 950 s |

The two rows differ in input and effort at once, and the fan references differ in size; the rows show the setting's output on each plan, not an effect of effort.

## Per call

| Call | Wall time | Fresh input tokens | Output tokens | Commands | Weekly window after |
|---|---|---|---|---|---|
| Handler | 414 s | 150k | 9.6k | 18 | 27 percent |
| Fork `affected-set` | 126 s | 42k | 2.6k | 6 | 21 percent |
| Fork `guard-reach` | 116 s | 38k | 2.5k | 6 | 21 percent |
| Fork `way-forward` | 131 s | 33k | 2.8k | 6 | 22 percent |

The handler's figures are its own thread's, so the forks' rows add to it: 263k fresh input and 17.6k output in all.
The forks' wall times run from spawn to their last message; their window readings come from the last `token_count` in each fork's rollout, so they are not a sequence.
The handler's wall time is the whole `codex exec`: 73 s of reading before the first spawn, 150 s more until the last fork returned, and 189 s of consolidation.
The handler's 18 commands are 17 reads and the `apply_patch` that wrote the review.
The forks' command counts are their own: each fork's rollout also holds the 9 commands of the handler's reading, which it inherited.

Each fork's first request held about 108k input tokens, of which 105k were cache hits, so a fork's fresh reading before its own commands is under 3k.
The handler's first request held 16k.

The windows: the account reported one window, the weekly one, at 18 percent at the call's first request and 27 at its end; it reported no 5-hour window.
The sibling benches `codex-astra-tuned-p1-high-bench` and `codex-astra-tuned-p4-med-bench` may have drawn on the same account over the same minutes, so the nine points cannot be attributed to this run; the stop at 70 percent was never near.

## Fork evidence

The rollouts are the check the task asked for; `data/rollout-evidence.txt` holds the figures.

- Started: the handler's rollout holds three `spawn_agent` calls, `affected_set`, `guard_reach` and `way_forward`, each with `fork_turns` set to `"all"`, at 12:42:45, 12:42:53 and 12:43:03 UTC.
  The fork bodies are encrypted, so the fork prompts themselves cannot be read.
- Concurrent: the forks ran 12:42:45 to 12:44:51, 12:42:53 to 12:44:49 and 12:43:03 to 12:45:14, so all three overlap from 12:43:03 to 12:44:49.
- Inherited context: each fork's `session_meta` carries `forked_from_id` of the handler's session, and its first request already held 107.8k, 108.3k and 108.7k input tokens against 16.3k for the handler's first.
  Each fork's rollout holds the handler's 9 earlier commands, including the read of `PATTERN.md` and 16 `pattern/` background files at 12:42:06.
- `AGENTS.md` loaded: the handler's rollout, and each fork's, holds an `AGENTS.md instructions for` block naming the run's working directory; its hash equals that of the snapshot's `CLAUDE.md` (`data/context-hashes.txt`).

## Context and prompt

The context and the reading block are the tuned-prompt bench's, built by the same scripts over the `P1` snapshot.

- The explore instruction file carries the PATTERN directive a production burler reviewer gets: `harness/build.sh` fills the code commit's `pattern-directive-review-fix.md` with that commit's `PATTERN.md` and the decision record's declared changes.
  The snapshot's root holds `CLAUDE.md` copied to `AGENTS.md`.
- The `P1` round had a focus file (`round-1-focus.md`), which `build.sh` adds to the explore instruction as the production focus directive; the `P4` round had none.
- `data/prompts/tuned-block.md` is the block, byte-identical to the tuned-prompt bench's (`cmp`).
  `harness/build-forkfan.sh` appends it to the handler prompt and inserts it, as one line, into the fork boilerplate that the cluster rules give each fork.
  The cap of 6 commands per fork stayed as the stencil sets it, and each fork used 6.
- The lenses are the three the Claude fan's round 1 on `P1` recorded in `round-1-usage.yaml`.
- Remaining differences from the Claude round's context are the earlier benches': no plugin skills, empty parent and edit directives, the snapshot's `CLAUDE.md`.

## The handler's findings

The handler returned `BLOCKING` with 9 findings: 3 of its own and 6 from the forks.
The forks returned 6: two each.
`data/match.tsv` holds each reading, with the dropped fork finding last.

| Finding | Severity | Origin | Defect | Grade |
|---|---|---|---|---|
| `F1` | `BLOCKING` | `guard-reach` | Card 1's retired-list refusal is unreachable: the template fill rejects the old shape before `decodeConfig` | `unique-real` |
| `F2` | `BLOCKING` | `way-forward` | Card 19's non-bool recovery cannot repair a legitimate numeric operand and names the wrong layer for per-worktree modules | `unique-real` |
| `F3` | `MEDIUM` | `guard-reach` | A caller's `-run` filter exempts a guard run from card 15's missing-tag check | `unique-real` |
| `F4` | `MEDIUM` | `way-forward` | Bare `lyx config reconcile` is a dry run on the worktree's files | shared, fan `F8` |
| `F5` | `MEDIUM` | `affected-set` | The tree-sitter link guard keeps the retired tags; card 11 omits its `go list` | `unique-real` |
| `F6` | `MEDIUM` | `affected-set` | `latestDarnOutcome` returns a skipped `Darn-Write` as the writer's outcome | `unique-real` |
| `F7` | `MEDIUM` | handler | The retag cards use `Custom` where `Edit` is supported | shared, fan `F3` |
| `F8` | `MEDIUM` | handler | The scan lands after the last retag, against D5 | shared, fan `F3` |
| `F9` | `MEDIUM` | handler | Re-sign arrows missing on `darnTreeScope` and `guardRunOf` | shared, fan `F4` |

- `F7` and `F8` are one fan finding, so the four shared findings cover three of the fan's.
- `F5` is the defect the review bench's `gpt-6.1-sol` run also found on `P1` and graded real.
- `F1`, `F2`, `F3` and `F6` hold on reading the cited card lines and the snapshot's code (`config.go:234`, `fill.go:112`, `hubwide.go:102`, `tierrun.go:165`, `darnwiring.go:110`); none was run.
  `F2` stays arguable on whether the fan's `F6` and its numeric-operand point are one defect: they share a sentence of card 19 and differ in the fault.
- The handler read every card in full, so the fan's `F9` (the `Verify` line fails on an empty `covers` list) and `F12` (a duplicated test) were in view and went unflagged.
- The handler's review ends with a `Focus departures` section stating that it did not run the plan overlay validator and did not read the retag surface exhaustively.

## Method

### Inputs and prompts

`P1` is rebuilt with `data/harness/build.sh` from the weft commit and code commit in `data/inputs.txt`, the earlier benches' input.
The harness is the tuned-prompt bench's with `ROOT` changed to this worktree, the block path changed, and `effort=high` changed to `effort=medium` in `run-forkfan.sh`.
The prompts are in `data/prompts/`: the handler prompt with the block, the two instruction files it reads, the block and the run's rewritten prompt.

### Call

```text
CODEX_HOME=<scratch>/codex-home codex exec -m gpt-6-astra -c model_reasoning_effort="medium" \
  -s workspace-write -C <work> --skip-git-repo-check --json -o <last-message> - < prompt
```

`run-forkfan.sh` runs the one call, then copies every rollout the call created.
The scratch `CODEX_HOME` holds only a link to the operator's `auth.json`, so the operator's `config.toml` did not apply.
`spawn_agent` with `fork_turns` was available with the stable `multi_agent` feature; no feature flag was set.

### Grading

One Claude session graded every handler finding as the earlier benches did: against the fan's round-1 findings of `P1` (`claude-findings.tsv` of the review bench and the weft's `round-1-review.md`), and an unmatched finding against the cited card lines and the snapshot's code.
`shared` means the finding names the same defect as a fan finding; `unique-real` means the claim holds and the fan's round has no finding of it; `false` means it does not hold.
The forks' final messages were taken from the `FINAL_ANSWER` inputs in the handler's rollout.

## Deviations and limits

- One input, one run, one grader: no variance is measured, and a repeat could change a count of one or two.
- Input and effort changed together against the tuned-prompt bench's `P4` run, so the table cannot say which one moved a figure.
- The grader is a Claude session and may favour its own family's phrasing; the rates are not validated.
- The fork prompts are encrypted in the rollout; that each fork's text held the block rests on the handler following the cluster rules and on the forks' command counts staying at the cap.
- Recall is against the fan's list, not against the truth; the `unique-real` findings hold on reading and were not run, so the fan reference is not complete for `P1`.
- The weekly window moved from 18 to 27 percent, but sibling benches may have drawn on the same account, so the points are not this run's cost alone.
- The operator's `~/.codex/config.toml` was not hashed before or after this bench.

## Data

`data/` holds:

- `inputs.txt`: the input with weft commit, round start time and code commit.
- `usage.txt`: one line per call with wall seconds, tokens and the weekly `used_percent`, then the handler's verdict.
- `rollout-evidence.txt`: start, end, parent thread, commands, first request and totals of each rollout, the handler's `spawn_agent` calls, the `AGENTS.md` loads and the handler's `pattern/` reads.
- `context-hashes.txt`: the hashes of `AGENTS.md`, `CLAUDE.md`, `PATTERN.md` and the block.
- `match.tsv`: the grading of every handler finding and the dropped fork finding.
- `reviews/P1/`: the handler's review and the three forks' final messages.
- `prompts/`: the handler prompt, the two instruction files it reads, the block and the run's rewritten prompt.
- `harness/`: `build.sh`, `build-fan.sh`, `build-forkfan.sh`, `run-forkfan.sh`, `setup-codex-home.sh`, `windows.sh` and `dump.sh`, working on `.scratch/bench/`, which is not committed.
