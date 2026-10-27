# Codex Astra forked fan on plan P2 with block v2 and three gap study texts: two medium runs

Bench of 2026-10-27, a follow-up to `docs/research/codex-v2-p2-bench-2026-10-27/report.md`.
Codex CLI 0.164.0, ChatGPT sign-in (plan `prolite`), Linux.
Two `codex exec` handler runs of `gpt-6-astra` at `medium` effort on the plan `P2` (`quarry-csharp`), each spawning three lens forks through `spawn_agent` with `fork_turns="all"`, one run after the other with no other Codex bench on the account.
The lenses are the Claude fan's round 1 lenses on `P2`: `affected-set`, `guard-reach` and `way-forward`.
The setup is `codex-v2-p2-bench`'s unchanged except for three texts copied word for word from `docs/research/claude-review-gaps-2026-10-27/report.md`: Proposals 1, 2 and 4.
Nothing was built: the bench is shell scripts under `data/harness/`, the reviewers' own output and a hand grading.

## Conclusion

The three texts did not measurably change the yield: the two runs together returned 12 findings, none false, at 500k fresh input tokens, against 10 findings at 429k for the two medium runs of `codex-v2-p2-bench`, which is 2.4 against 2.3 findings per 100k fresh input tokens.
The two new runs differ from each other more than the pooled figure differs from the earlier one: 9 findings against 3, and 3.4 against 1.3 findings per 100k fresh tokens.

| Measure | New run 1 | New run 2 | Earlier medium 1 | Earlier medium 2 |
|---|---|---|---|---|
| Findings | 9 | 3 | 4 | 6 |
| Real findings | 9 | 3 | 4 | 6 |
| `BLOCKING` findings, all real | 3, of which 2 arguable | 0 | 2, of which 1 arguable | 1 |
| False findings | 0 | 0 | 0 | 0 |
| Claude fan `BLOCKING` findings found | 1 of 3 | 1 of 3 | 1 of 3 | 2 of 3 |
| Fan findings found | 1 of 13 | 1 of 13 | 1 of 13 | 2 of 13 |
| Commands per fork (`affected-set`, `guard-reach`, `way-forward`) | 6, 9, 6 | 5, 7, 6 | 5, 4, 2 | 7, 6, 4 |
| Findings from the `way-forward` fork | 0 | 0 | 0 | 0 |
| Fresh input tokens | 263k | 237k | 192k | 237k |
| Window points | 3 | 2 | not attributable | not attributable |
| Wall time | 597 s | 374 s | 255 s | 370 s |
| Verdict | `BLOCKING` | `APPROVED` | `BLOCKING` | `BLOCKING` |

- Real findings per window point were 3.0 in run 1 and 1.5 in run 2, 2.4 over the two runs (12 findings in 5 points); real `BLOCKING` findings per point were 0.6 over the two runs (3 in 5 points, 0.2 with the two arguable ones left out).
  The earlier bench's points cannot be set against these: its medium runs read 29 to 47 and 29 to 51 while two sibling benches drew on the same window, so the earlier rows have no points of their own.
  The comparison therefore falls back on fresh input tokens: 2.4 real findings per 100k fresh tokens over the new runs against 2.3, and 0.6 `BLOCKING` real against 0.7 (0.2 against 0.5 without the arguable ones).
- A difference of one or two findings is inside the run-to-run spread, and so is every difference in this table: one input, one grader, and two runs of the same setting that returned 9 and 3 findings.
  This bench cannot say the texts helped or hurt.
- The texts did not reach the fan's other findings: each new run found only `F1` of the fan's 13, the fallback that picks `IProvider`, and neither found `F2` or `F3`.
  The earlier medium runs found `F1` and, in one, `F2`.
- The `way-forward` fork returned no finding in either run, as in every run of `codex-v2-p2-bench`.
  With Proposal 4 its forks ran 6 commands each against 2 and 4 and read the recovery, rebaseline and fingerprint code of the commands the cards name, and still found no failing precondition; the fan's five `way-forward` findings (`F6`, `F7`, `F8`, `F11`, `F12`) were not found.
- The `guard-reach` fork of run 1 ran 9 commands, the most of any fork, and returned the one new `BLOCKING` finding that rests on tracing an entry point: an earlier Delete and a re-sign into the same overload id can share a batch, and the delete's done check then answers before the re-sign pass runs (`F1` of run 1).
  One run does not tell whether Proposals 1 and 2 caused it.
- Run 2 returned `APPROVED`, the one verdict that differs from the fan's `BLOCKING` in any `P2` run of these benches; its three findings are all `MEDIUM`.
  Its fan finding `F1` is rated `MEDIUM` by Astra in both new runs, as in the earlier ones.

## Summary table

Per-run measures, `P2` only.
The fan reference is 13 findings, 3 of them `BLOCKING`, from `opus` at medium effort with 3 lens forks, as the earlier benches recorded.
"Real" counts findings that hold on reading, arguable ones included; "Shared" counts findings that name a defect of the fan's list.
Fresh input tokens are the handler's plus the three forks', each rollout's `input_tokens` less `cached_input_tokens` of its last `token_count`.
The window is the weekly window's `used_percent`: run 1 read 62 at its first request and 65 at its last token count, run 2 read 65 and 67.

| Setting | Findings | Shared with fan | Unique real | False | Fan findings found | `BLOCKING` found | Verdict as fan's | Fresh input tokens | Window points | Wall time |
|---|---|---|---|---|---|---|---|---|---|---|
| `gpt-6-astra` medium 1, forked fan, block v2 and gap texts | 9 | 1 | 8 | 0 | 1 of 13 | 1 of 3 | yes | 263k | 62 to 65 | 597 s |
| `gpt-6-astra` medium 2, forked fan, block v2 and gap texts | 3 | 1 | 2 | 0 | 1 of 13 | 1 of 3 | no (`APPROVED`) | 237k | 65 to 67 | 374 s |
| Union of the two runs | 10 | 1 | 9 | 0 | 1 of 13 | 1 of 3 | | 500k | 62 to 67 | 971 s in sequence |
| Earlier medium 1, block v2 | 4 | 1 | 3 | 0 | 1 of 13 | 1 of 3 | yes | 192k | 29 to 47, shared | 255 s |
| Earlier medium 2, block v2 | 6 | 2 | 4 | 0 | 2 of 13 | 2 of 3 | yes | 237k | 29 to 51, shared | 370 s |
| Earlier union of the two medium runs | 7 | 2 | 5 | 0 | 2 of 13 | 2 of 3 | yes | 429k | 29 to 51, shared | 370 s in parallel |

The union row's findings are distinct defects: of the 12 findings the runs returned, 2 name defects both runs found.
The earlier rows are `codex-v2-p2-bench`'s, copied from its report.

## Per call

| Run | Call | Wall time | Fresh input tokens | Output tokens | Reasoning tokens | Commands |
|---|---|---|---|---|---|---|
| 1 | Handler | 597 s | 155k | 12.3k | 4.3k | 19 |
| 1 | Fork `affected-set` | 144 s | 40k | 2.5k | 0.7k | 6 |
| 1 | Fork `guard-reach` | 265 s | 37k | 4.8k | 2.2k | 9 |
| 1 | Fork `way-forward` | 139 s | 31k | 2.1k | 0.8k | 6 |
| 2 | Handler | 374 s | 125k | 7.6k | 2.2k | 17 |
| 2 | Fork `affected-set` | 122 s | 38k | 2.4k | 0.5k | 5 |
| 2 | Fork `guard-reach` | 151 s | 39k | 2.7k | 0.9k | 7 |
| 2 | Fork `way-forward` | 105 s | 35k | 1.9k | 0.5k | 6 |

The handler's figures are its own thread's, so the forks' rows add to it; a handler's wall time is the rollout's, and a fork's runs from spawn to its last message.
A fork's command count is its own: its rollout also holds the handler's commands made before the spawn, 11 in run 1 and 10 in run 2, which `data/harness/usage.sh` subtracts.
`data/usage.txt` holds every figure with the window readings per rollout.

## Fork evidence

`data/rollout-evidence.txt` holds the figures; both runs pass the checks the earlier bench set.

- Started: each handler's rollout holds three `spawn_agent` calls with `fork_turns` set to `"all"`, about 14 s apart.
  The fork bodies are encrypted, so the fork prompts cannot be read.
- Concurrent: the forks of run 1 ran 14:25:47 to 14:28:11, 14:26:01 to 14:30:27 and 14:26:16 to 14:28:35 UTC, overlapping from 14:26:16 to 14:28:11; run 2 overlaps from 14:36:31 to 14:38:04.
- Inherited context: each fork's `session_meta` carries `forked_from_id` of its handler's session, and its first request already held 116k to 118k input tokens in run 1, against 16k for the handler's first.
- `AGENTS.md` loaded: every rollout holds an `AGENTS.md instructions for` block; the snapshot's `AGENTS.md` and `CLAUDE.md` hash alike (`data/context-hashes.txt`).
- The runs ran one after the other, run 2 starting 20 s after run 1 ended, and no other `codex exec` ran on the machine (`ps` read zero before run 1).

## The findings

`data/match.tsv` holds each reading; `data/reviews/P2/<run>/` holds the handlers' reviews and the forks' final messages.
No handler dropped a finding its forks returned.

| Defect | Severity (own) | Run 1 | Run 2 | Grade |
|---|---|---|---|---|
| `declaredNameIndex` fallback picks `IProvider` | `MEDIUM` | `F4` | `F1` | shared, fan `F1` |
| Cards 6, 8, 11 and 12 add status and `Unit` readers outside `allowedStatusConsumers` | `MEDIUM` | `F6` | `F2` | `unique-real` |
| Card 6's resolve half reads a `.cs` symbol from a `Uses` ref as a changed C# target | `MEDIUM` | `F5` | | `unique-real` |
| `renameDeclSource` calls `ParseRef` with no plan to give it | `MEDIUM` | | `F3` | `unique-real` |
| An earlier Delete and a re-sign into the same overload id can share a batch, and the delete's done check answers before `RebindResigns` | `BLOCKING` | `F1` | | `unique-real` |
| `TestRenameCSharpDecl` tests an unexported helper directly | `BLOCKING` | `F2` | | `unique-real`, severity arguable |
| Card 14 defers the package docs and the PATTERN text past their implementing cards | `BLOCKING` | `F3` | | `unique-real`, arguable |
| `PathEntryPath` is an exported symbol with two consumers, created inside card 6 | `MEDIUM` | `F7` | | `unique-real` |
| Card 5 says `none` already skipped every gated check, but an existing row says it fires | `LOW` | `F8` | | `unique-real` |
| `Plan.Language`'s field comment names only `go` and `none` | `LOW` | `F9` | | `unique-real` |

- The fallback finding and the status-registry finding are the two defects both new runs share; both were also in the earlier medium runs.
  The earlier runs' unreached operator refusal of card 10, which all three runs of `codex-v2-p2-bench` found and rated `BLOCKING`, appears in neither new run.
- The overload-replacement finding holds by reading: card 11 lines 39 to 40 permit an earlier card to delete an overload before another is re-signed into its id; `renameBoundaries` (`internal/batcher/boundary.go` lines 12 to 29) covers only a later Rename old side or Delete, and card 13's new boundary only a later use of the re-sign's old glyph, so the two cards may share a batch; after the batch the id exists again, `DoneChecks` treats every answer but `not_found` as still existing for a Delete (`internal/planglyph/donecheck.go` lines 96 to 100 and 201 to 205), and `postBatchChecks` returns on that before the re-sign pass (`internal/websterengine/recordbatch.go` lines 829 to 849).
  It was graded by reading the cards and code, not by running a plan.
- The private-helper finding holds on `pattern/PATTERN-test-economy.md` line 7 and card 10 line 41; whether it is `BLOCKING` is arguable, as is card 14's, which rests on `CLAUDE.md`'s rule that docs land in the same commit as the code.
- The fan findings found by neither new run are `F2` to `F13`, as for the earlier runs but `F2`, which one earlier run found.

## Method

### Inputs and prompts

`P2` is rebuilt with `data/harness/build.sh` from the weft commit `394715f7f` and the code commit `750903aef` in `data/inputs.txt`.
The round's focus file is part of the input, as in the Claude round.
The harness is `codex-v2-p2-bench`'s with `ROOT` changed to this worktree, plus three changes and one new script:

- `build-fan.sh` overrides the template's `guard-reach` and `way-forward` texts after reading them, each by replacing the closing `Your list` sentence and failing when that sentence is not the template's.
  `guard-reach` keeps the template's sentences up to "are findings." and takes Proposal 1's `Find the recurrences` and `For a value a card copies` sentences, Proposal 2's added `For each check a card prescribes` sentence and Proposal 1's `Your list`, in that order, since Proposal 1 replaces the old `Your list` and Proposal 2 adds before it.
  `way-forward` keeps the template's sentences up to "are findings." and takes Proposal 4's two sentences and its `Your list`.
- `build-forkfan.sh` changes "one hop beyond it" to "beyond it along the input's path" in the fork boilerplate (Proposal 2) and fails when the old words remain or the new ones are absent.
  Block v2's own "one hop beyond the cards" bullet is unchanged.
- `data/prompts/block-v2-gap.md` is `block-v2.md` with Proposal 1's reading bullet added after the bullet on the functions a card changes and their callers.
- `usage.sh` is new and prints one line per rollout.

`data/prompts/diff-against-earlier-handler-prompt.txt` and `diff-against-earlier-step-2.txt` hold the only differences against `codex-v2-p2-bench`'s prompts: the added bullet, the two lens texts and the boilerplate wording.
The context is as before: the PATTERN overview inlined as production renders it, the snapshot's `CLAUDE.md` as `AGENTS.md`, and the fork cap of 20 tool calls.

### Call

```text
CODEX_HOME=<scratch>/codex-home-<run> codex exec -m gpt-6-astra -c model_reasoning_effort="medium" \
  -s workspace-write -C <work> --skip-git-repo-check --json -o <last-message> - < prompt
```

Run 1 started at 14:24:27 UTC and run 2 at 14:34:47 UTC, run 2 only after run 1 had ended.
The scratch `CODEX_HOME` of each run holds only a link to the operator's `auth.json`, so the operator's `config.toml` did not apply.
No feature flag was set.

### Grading

One Claude session graded every handler finding as the earlier benches did, against the fan's round-1 findings of `P2` (`docs/research/codex-review-bench-2026-10-25/data/claude-findings.tsv`), and an unmatched finding against the cited card lines and the snapshot's code.
`shared` means the finding names the same defect as a fan finding; `unique-real` means the claim holds and the fan's round has no finding of it; `false` means it does not hold.
The main measure is real findings, not overlap with the fan's list: two runs of a reviewer at this temperature do not find the same things, so matching one Claude round's list is a weak measure.

## Deviations and limits

- One input, two runs and one grader: a difference of one or two findings, or of a few tenths of findings per window point, is inside the run-to-run spread, and the two new runs themselves differ by 6 findings.
- The window readings are whole percents, so a run's 3 or 2 points carry an error of about one point each, which is a third to a half of the figure.
  Run 1's start of 62 is the first request's reading, so any use before it is not counted; run 2's start is run 1's last reading.
  Nothing else drew on the Codex account during the two runs as far as the machine shows, but the account is the operator's and other devices are not visible.
- The earlier bench's points are shared with its siblings, so no per-point comparison exists; the tokens-based rate stands in, and fresh tokens are a different cost from window points (the new run 1 used 263k tokens for 3 points and run 2 237k for 2).
- The grader is a Claude session and may favour its own family's phrasing; the `unique-real` rates are not validated, and the `BLOCKING` grades of the private-helper and card 14 findings are arguable, so the `BLOCKING` real counts show the arguable ones apart.
- The fork prompts are encrypted in the rollouts; that each fork's text held the new lens texts, the reading bullet, the changed boilerplate and the cap of 20 rests on the build script's checks and on the handler following the cluster rules.
- Recall is against the fan's list, not against the truth; the `unique-real` findings hold on reading, so the fan reference is not complete for `P2`.
- Proposals 1, 2 and 4 target defects of `P4` and `P1`, not of `P2`, so this bench tests whether the texts cost or add on a plan they were not written for; it does not test whether they catch their targets.
- No run of block v2 without the three texts exists in this bench; the comparison is to `codex-v2-p2-bench`, whose two medium runs ran at the same effort but concurrently with two sibling benches.
- The operator's `~/.codex/config.toml` was not hashed before or after this bench.

## Data

`data/` holds:

- `inputs.txt`: the input with weft commit, round start time and code commit.
- `usage.txt`: one line per call with wall seconds, tokens, command count and the weekly `used_percent` at the call's first and last `token_count`.
- `rollout-evidence.txt`: start, end, parent thread, commands, first request and totals of each rollout, then the `spawn_agent` and `AGENTS.md` counts per rollout.
- `context-hashes.txt`: the hashes of `AGENTS.md`, `CLAUDE.md`, `PATTERN.md` and both block files.
- `match.tsv`: the grading of every handler finding.
- `reviews/P2/<run>/`: each handler's review and its three forks' final messages.
- `runs/`: each run's exit code, wall time and last turn's usage.
- `prompts/`: the handler prompt, the review step's instructions, block v2 with Proposal 1's bullet, the two changed lens texts and the two diffs against the earlier bench.
- `harness/`: `build.sh`, `build-fan.sh`, `build-forkfan.sh`, `run-forkfan.sh`, `setup-codex-home.sh`, `windows.sh`, `dump.sh`, `evidence.sh` and `usage.sh`, working on `.scratch/bench/`, which is not committed.
