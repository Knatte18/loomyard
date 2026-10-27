# Codex Astra forked fan with reading block v2 on the plan P1: one high run against two medium runs

Bench of 2026-10-27, a follow-up to `docs/research/codex-astra-tuned-p1-high-bench-2026-10-27/report.md` and `docs/research/codex-astra-tuned-p1-med-bench-2026-10-27/report.md`.
Codex CLI 0.164.0, ChatGPT sign-in (plan `prolite`), Linux.
Three `codex exec` handler runs on the plan `P1` (`gate-runs-once`, 33 cards): `gpt-6-astra` at high effort, and twice at medium effort, each handler and its forks at the same effort.
Each handler read the plan, then spawned three lens forks through `spawn_agent` with `fork_turns="all"`.
The runs are identical but for effort, and differ from the earlier `P1` benches in two things: block v2 replaces block v1, and the fork boilerplate's cap is `at most 20 tool calls` instead of `at most 6`.
Nothing was built: the bench is shell scripts under `data/harness/`, the reviewers' own output and a hand grading.

## Conclusion

With block v2 and the 20-call cap, the forked fan found 6 of the Claude fan's 12 findings on `P1` at high effort, 5 on one medium run and 6 on the other, and both of the fan's `BLOCKING` findings in two of the three runs.
The earlier block v1 rows found 4 of 12 at high and 3 of 12 at medium.

| Fan `BLOCKING` finding | High | Medium 1 | Medium 2 |
|---|---|---|---|
| `F1`: card 23 shortens `PATTERN.md` only after cards 3, 7 and 19 grow it | yes, as `F2` | no | yes, as `F2` |
| `F2`: the hand-enumerated readers of cards 1, 6 and 14 miss members | yes, as `F1` | yes, as `F1` | yes, as `F1` |

- `F2` is the finding no earlier Astra run made.
  All three runs name `templateDefaults` in `internal/gateslot/config_test.go`; the high run also names the direct `Gates` assignments in `TestCardGateCommand`, `TestBeginBatch_PromptFile` and `TestRecoverSpawnOrAttach`, four of the six card 1 readers the fan lists.
  No run names the `darnengine` dead code after card 6, the `tree_scope_verify` text in `docs/overview.md`, or the `guard_tier` mentions after card 14, so the finding is found in part and counted as found, as the earlier benches counted a finding that names one of its members.
- The high run and medium 2 found `F1`; medium 1 did not.
  The `F1` miss is the only difference in the fan findings the two medium runs found (5 against 6).
- No run found the fan's `F5`, `F7`, `F9`, `F10`, `F11` or `F12`.
  The high, medium 1 and medium 2 handlers rate 2, 3 and 6 findings `BLOCKING`, against the fan's 2.
- The cap of 20 did not bind: the most commands any fork ran on its own was 12.
  Forks ran 7 to 12 commands each, against 5 and 6 in the `P1` high bench under block v1.
- Which setting is the better buy:
  the high run found the same 6 fan findings as medium 2 and as the union of the two medium runs, with 8 findings against 10, for 352k fresh input tokens against 305k for medium 2 and 543k for the two medium runs together.
  One medium run costs 68 and 87 percent of the high run's fresh input tokens, and the two together 154 percent, for no gain over medium 2 alone.
  On this input, one run at either effort found the same 6 fan findings or one fewer; a second medium run added no fan finding to medium 2, and added to medium 1 the `BLOCKING` `F1`.
  The data cannot tell effort from chance (see Limits).

## Summary table

One run per cell, `P1` only.
The fan reference is 12 findings, 2 of them `BLOCKING`, from `opus` at medium effort with 3 lens forks, as the earlier benches recorded.
"Found" counts distinct fan findings that a finding of the setting matched.
The wall time of the union is the longer of the two medium runs, which ran in parallel.
The window figures are the weekly `used_percent` of the first and last request of each run; the three runs and the sibling benches `codex-v2-p2-bench` and `codex-v2-p4-bench` ran on the same account over the same minutes, so no run's points can be separated out.

| Setting | Findings | Shared with fan | Unique real | False | Fan findings found | `BLOCKING` found | Fresh input tokens | Window | Wall time |
|---|---|---|---|---|---|---|---|---|---|
| High, block v2, cap 20 | 8 | 6 | 2 | 0 | 6 of 12 | 2 of 2 | 352k | 28 to 54 | 763 s |
| Medium 1, block v2, cap 20 | 7 | 5 | 2 | 0 | 5 of 12 | 1 of 2 | 238k | 28 to 51 | 402 s |
| Medium 2, block v2, cap 20 | 10 | 6 | 4 | 0 | 6 of 12 | 2 of 2 | 305k | 28 to 54 | 717 s |
| Union of medium 1 and 2 | 10 | 6 | 4 | 0 | 6 of 12 | 2 of 2 | 543k | 28 to 54 | 717 s |

The union counts distinct findings by defect.
Every finding of medium 1 matches a finding of medium 2, so the union holds medium 2's 10 findings; medium 1 adds one claim inside a matched finding, that `internal/testkit/tmuxkit/allow_tmux.go` is built under `tmux || llm` only.
All runs returned the fan's verdict, `BLOCKING`.
Fresh input tokens are `input_tokens` less `cached_input_tokens` of each rollout's last `token_count`, summed over the handler and its three forks.

### Beside every earlier row for `P1`

| Setting | Findings | Shared with fan | Unique real | False | Fan findings found | `BLOCKING` found | Fresh input tokens | Wall time |
|---|---|---|---|---|---|---|---|---|
| `gpt-6-astra` high, forked fan, block v2 | 8 | 6 | 2 | 0 | 6 of 12 | 2 of 2 | 352k | 763 s |
| `gpt-6-astra` medium 1, forked fan, block v2 | 7 | 5 | 2 | 0 | 5 of 12 | 1 of 2 | 238k | 402 s |
| `gpt-6-astra` medium 2, forked fan, block v2 | 10 | 6 | 4 | 0 | 6 of 12 | 2 of 2 | 305k | 717 s |
| `gpt-6-astra` high, forked fan, block v1 | 10 | 4 | 6 | 0 | 4 of 12 | 1 of 2 | 412k | 1114 s |
| `gpt-6-astra` medium, forked fan, block v1 | 9 | 4 | 5 | 0 | 3 of 12 | 0 of 2 | 263k | 414 s |
| `gpt-6-astra` medium, solo | 2 | 2 | 0 | 0 | 2 of 12 | 0 of 2 | 93k | 184 s |
| Claude `opus` medium, solo | 6 | 3 | 3 | 0 | 2 of 12 | 1 of 2 | 206k | 563 s |
| Codex `gpt-6.1-sol` medium, solo | 7 | 4 | 3 | 0 | 2 of 12 | 0 of 2 | 156k | 491 s |
| Claude `opus` medium, fan | 12 | | | | | | 384k, plus 92k in forks | 604 s |

The block v1 rows are the two tuned `P1` benches; the last four rows are the solo bench's and the review bench's.
Fresh input tokens for Astra and `sol` are as above; Opus counts cache creation as fresh input, so the figures are not the same measure.
The block v2 rows hold more shared findings than any earlier row, and fewer `unique-real` ones than the block v1 rows, because v2 runs found more of the fan's defects rather than more defects of their own.

## How far the two medium runs differ

This is the first repeat measure of these benches.

- Findings: 7 against 10.
  Fan findings found: 5 against 6; the difference is `F1` (`BLOCKING`), the `PATTERN.md` growth order, which medium 2's handler found and medium 1's did not.
- Of medium 2's 10 findings, 6 match 7 of medium 1's findings (medium 1's `F1` and `F2` both fall under medium 2's `F1`).
  Medium 2 has 4 findings medium 1 lacks: the `PATTERN.md` growth order, the package-wide guard exclusion in card 14 (`BLOCKING`), the duplicate force-setting test in card 20 (`BLOCKING`), and a caller `-run` filter that exempts a guard run in card 15.
  Medium 1 has no finding medium 2 lacks.
- Both medium runs found the fan's `F2`, `F3`, `F4`, `F6` and `F8`; medium 2 found `F1` on top, and both returned the same two `unique-real` defects, of card 24's checker and of the tag readers, in part.
- A single medium run therefore leaves out, on this input, between zero and four findings that a second run finds, and one of the fan's two `BLOCKING` findings for one of the two.
  The medium runs differ from each other by as much as either differs from the high run, so effort at one run per cell is not separable from run-to-run spread here.

## Per call

| Run | Call | Wall time | Fresh input tokens | Output tokens | Reasoning tokens | Commands |
|---|---|---|---|---|---|---|
| High | Handler | 763 s | 218k | 16.9k | 6.6k | 32 |
| High | Fork `affected-set` | 246 s | 47k | 5.4k | 2.1k | 10 |
| High | Fork `guard-reach` | 266 s | 44k | 5.4k | 3.0k | 8 |
| High | Fork `way-forward` | 149 s | 43k | 3.3k | 1.1k | 7 |
| Medium 1 | Handler | 402 s | 123k | 9.0k | 2.0k | 19 |
| Medium 1 | Fork `affected-set` | 145 s | 36k | 2.9k | 0.7k | 8 |
| Medium 1 | Fork `guard-reach` | 158 s | 47k | 3.2k | 0.6k | 12 |
| Medium 1 | Fork `way-forward` | 126 s | 32k | 2.5k | 0.5k | 7 |
| Medium 2 | Handler | 717 s | 169k | 14.0k | 4.3k | 29 |
| Medium 2 | Fork `affected-set` | 238 s | 48k | 4.3k | 1.4k | 10 |
| Medium 2 | Fork `guard-reach` | 274 s | 47k | 4.9k | 1.8k | 9 |
| Medium 2 | Fork `way-forward` | 150 s | 41k | 3.1k | 0.5k | 10 |

The handler's figures are its own thread's, so the forks' rows add to it.
The forks' wall times run from spawn to their last message.
The handler's wall time is the whole `codex exec`: the reading before the first spawn took 17, 8 and 12 commands in the three runs, and the rest of the wall time is the forks and the consolidation.
The forks' command counts are their own: each fork's rollout also holds the handler's reading commands, which it inherited, and `data/harness/evidence.sh` subtracts them.
The first request of a fork held 158k, 93k and 129k input tokens in the three runs, almost all cache hits.

Window readings, weekly `used_percent` of the account, over the 12 minutes the runs shared:

| Run | First request | Last request |
|---|---|---|
| High | 28 | 54 |
| Medium 1 | 28 | 51 |
| Medium 2 | 28 | 54 |

The 26 points between 28 and 54 are the draw of the three runs and the two sibling benches together, and no run's share can be read from them.
The stop at 70 percent was not reached.

## Fork evidence

The rollouts are the check the entry asked for; `data/rollout-evidence.txt` holds the figures.

- Started: each handler's rollout holds three `spawn_agent` calls, `affected_set`, `guard_reach` and `way_forward`, each with `fork_turns` set to `"all"`, 10 to 13 seconds apart.
  The fork prompts are encrypted in the rollout, so that each held block v2 and the 20-call cap rests on the handler following the cluster rules.
- Concurrent: in every run the three forks' time spans overlap (high from 13:27:57 to 13:30:26 UTC, medium 1 from 13:27:01 to 13:29:02, medium 2 from 13:27:39 to 13:30:09).
- Inherited context: each fork's `session_meta` carries `forked_from_id` of its handler's session, and its first request already held 93k to 159k input tokens against 16k for a handler's first.
- `AGENTS.md` loaded: the handler's rollout, and each fork's, holds an `AGENTS.md instructions for` block naming the run's working directory; its hash equals that of the snapshot's `CLAUDE.md` (`data/context-hashes.txt`).

## Prompt

`data/prompts/block-v2.md` holds block v2 as the entry gives it, written once.
`harness/build-forkfan.sh` appends the block to the handler prompt and inserts it, as one line, into the fork boilerplate; it also changes the boilerplate's `at most 6 tool calls` to `at most 20 tool calls` and exits if either anchor is missing.
The context, the lenses, `AGENTS.md`, the PATTERN overview as production renders it and the round-1 focus file are the `P1` high bench's.
The prompts as the run built them are in `data/prompts/`.
The review step's read-only rule applied to every handler as it did to the Claude round; the high and medium 1 handlers named that they ran no `validate-plan` overlay and withheld concrete patches, and medium 2's handler tried the overlay and found no `lyx` on its path.

## The findings

`data/match.tsv` holds each reading; `data/reviews/P1/<run>/` holds each handler's review and the three forks' final messages.
No handler dropped a fork finding: every finding of the forks appears in its handler's review, some merged.

- Found by all three runs, as the fan found them: `F2` (partly, see above), `F3` (the retag cards use `Custom` on a false premise), `F4` (missing re-sign arrows), `F6` (card 19's recovery edits `run_settings` after Seed-Child) and `F8` (a bare `lyx config reconcile` is a dry run).
  Block v1 found `F3`, `F4` and `F8` and, in the high run, `F1`.
  `F6` and the card 1 `F2` readers are new with v2.
- `F6` came from the `way-forward` fork in all three runs, and its evidence is the `run_settings` text of `lyx board upsert` and `internal/battenshed/seamchild.go`, which the forks read in their own commands.
- Unique-real, found in at least one run:
  - Card 24's checker passes a flow-style tier mapping and accepts `-tags nottier0` for `tier0` (all three runs; the second half was `unique-real` in the high block v1 bench).
  - The tree-sitter link guard's `go list` and tmuxkit's `allow_tmux.go` keep the old tags (high, medium 1, and the first half in medium 2).
  - Card 14 keeps guard exclusion package-wide, so a tier0 run over a package holding a tier1 guard compiles and runs no such guard, against D6 (medium 2, `BLOCKING`).
  - Card 20's new force-setting test repeats `stepCommandRows_FollowTheTasksLocalSwitch` (medium 2, `BLOCKING`; Opus solo filed it as `LOW`).
  - A caller `-run` among the args exempts a guard run from the empty-tier check, against the card's own sentence (medium 2).
  - Cards 10 and 19 bundle `ReadRunRecords` and `CheckSettingKey` into feature cards (high; arguable, each has one consumer).
  - Card 3's rendered `--tier` assertions in five tests, and `TestDerive_OverARepository`'s `map[string][]string` comparison after card 14 (high, and the second in medium 2).
- None of the nine forks' findings is `false`.
  Two claims were checked beyond the review text against the snapshot: card 14's `Targets.Guards` retype against `TestDerive_OverARepository` at `internal/impactset/derive_integration_test.go:102`, and medium 2's guard exclusion against `internal/impactset/targets.go:126` and `internal/gatecli/scope.go:138`.
  The others rest on the handlers' cited lines, as in the earlier benches.
- Astra rates the transition-check, tag-reader and force-setting findings `BLOCKING` where the fan files its `F9` as `LOW` or has no finding; the `unique-real` label is the grader's reading of the cited lines, not a run.

## Method

### Inputs and prompts

`P1` is rebuilt with `data/harness/build.sh` from the weft commit and code commit in `data/inputs.txt`; `build-fan.sh` and `build-forkfan.sh` stage the fan prompts.
The harness is the `P1` high bench's with the repository root and directory changed, with four changes: `build-forkfan.sh` uses block v2 and the cap number, `run-forkfan.sh` takes a run label and an effort, `setup-codex-home.sh` makes one scratch `CODEX_HOME` per label, and `evidence.sh` extracts the figures of a run's rollouts.

### Call

```text
CODEX_HOME=<scratch>/codex-home-<label> codex exec -m gpt-6-astra -c model_reasoning_effort="<effort>" \
  -s workspace-write -C <work> --skip-git-repo-check --json -o <last-message> - < prompt
```

The three runs started within a second of each other, each in its own copy of the input and its own `CODEX_HOME`, which held only a link to the operator's `auth.json`; the operator's `config.toml` did not apply.
`run-forkfan.sh` copies every rollout the call created.
`spawn_agent` with `fork_turns` was available with the stable `multi_agent` feature; no feature flag was set.

### Grading

One Claude session graded every handler finding as the earlier benches did: against the fan's round-1 findings of `P1` in the weft's `round-1-review.md`, and an unmatched finding against the cited card lines and the snapshot's code.
`shared` means the finding names the same defect as a fan finding; `unique-real` means the claim holds and the fan's round has no finding of it; `false` means it does not hold.
A finding that names part of a multi-member fan finding (the fan's `F2`) is `shared`.

## Limits

- One input, one run per cell, one grader: the high-against-medium comparison and the medium-to-medium difference cannot separate effort from chance, and a repeat could move a count by one or two.
  The two medium runs differ by one fan finding and three findings overall; that is the size of spread to read the other comparisons against.
- The grader is a Claude session and may favour its own family's phrasing; the rates are not validated.
- The fork prompts are encrypted in the rollouts.
- The window figures include the sibling benches' draw and each other's.
- The fan reference is not complete for `P1`: the `unique-real` findings hold on reading.
- The operator's `~/.codex/config.toml` was not hashed before or after this bench.
- Block v2 and the cap changed together against block v1, so the gain over the v1 rows cannot be split between them; the cap did not bind, which points at the block.

## Data

`data/` holds:

- `inputs.txt`: the input with weft commit, round start time and code commit.
- `usage.txt`: one line per call with wall seconds, tokens, commands and the windows, then each run's total.
- `rollout-evidence.txt`: per run, each rollout's start, end, parent, commands, first request and totals, the handler's `spawn_agent` calls and the `AGENTS.md` loads.
- `context-hashes.txt`: the hashes of `AGENTS.md`, `CLAUDE.md`, `PATTERN.md` and the block.
- `match.tsv`: the grading of every handler finding of the three runs.
- `reviews/P1/<run>/`: the handler's review and the three forks' final messages.
- `prompts/`: block v2, the handler prompt, the two instruction files it reads, and the prompt as a run rewrote it for its working copy.
- `harness/`: `build.sh`, `build-fan.sh`, `build-forkfan.sh`, `run-forkfan.sh`, `setup-codex-home.sh`, `windows.sh`, `dump.sh` and `evidence.sh`, working on `.scratch/bench/`, which is not committed.
