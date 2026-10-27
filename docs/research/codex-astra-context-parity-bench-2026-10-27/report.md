# Codex Astra fork fan with the Claude reviewers' context on one plan

Bench of 2026-10-27, a follow-up to `docs/research/codex-astra-fork-fan-bench-2026-10-27/report.md`.
Codex CLI 0.164.0, ChatGPT sign-in (plan `prolite`), Linux.
One `codex exec` handler run on the plan `P4` (`machine-handover`), `gpt-6-astra` at high effort for the handler and its forks, one run, no repeat, no Claude run.
The handler read the plan once, then spawned three lens forks through `spawn_agent` with `fork_turns="all"`, as in the forked fan bench.
Only the context changed: the PATTERN overview inlined in the prompt as production renders it, and the snapshot's `CLAUDE.md` loaded as `AGENTS.md`.
Nothing was built: the bench is shell scripts under `data/harness/`, the reviewers' own output and a hand grading.

## Conclusion

With the Claude round's context the forked fan found 2 of the Claude fan's 11 findings on `P4` and none of its 3 `BLOCKING` ones, against 1 of 11 and none for the same fan without it.
The handler returned the fan's verdict (`BLOCKING`) and 8 findings: 2 shared with the fan, 6 `unique-real`, none `false`.
The one fork finding the handler dropped is also `unique-real`.

- The gained finding is the fan's `F6`: card 17's shortened `PATTERN.md` line drops the exception for a Finalize past its parent-side merge and states every owner judgment as made against origin.
  The handler found it in its own review, and rated it `BLOCKING` where the fan rates it `MEDIUM`.
  It is the finding that needs the declared-changes text and the overview, so the PATTERN block is the likely cause, but one run cannot show it.
- The fan's `F9` (public symbols bundled into consumer cards) was the shared finding of the forked fan bench and is shared again, as `public-api-granularity`.
- The three `BLOCKING` findings of the Claude fan:
  - `F1`, the owner guard on `retryWeftPush` reading origin's Bolt outside the reconcile: not found.
  - `F2`, `adoptcli` missing from `PATTERN-cli-cobra`'s background: not found, although the handler read `pattern/PATTERN-cli-cobra.md` in two batches of backgrounds, and the output of a read held the deviation list (`battencli`, `shedcli`, `cleanupcli`) that `adoptcli` is missing from.
  - `F3`, the one-second production wait in `TestSetOwners`: not found.
- Most of the other findings repeat ones the earlier Astra benches found: the skip-variable bypass, the slug-only `Takeover` guard that a prime claim passes, the adopter-first repair beside the merge-and-push advice, adopt waiting behind a hub pause, and the `envkit` seam inventory.
  New on `P4` are the repair naming the parent head as the task's squash, and `lyx board list --text` dropping the owner.
- The run took 980 s and 324k fresh input tokens against 737 s and 299k for the forked fan without the context.
  The forks' inherited prefix grew from about 147k to about 155k tokens.

With one input, one run per cell and one grader, a difference of one finding is inside the noise.
The run shows that the context moved the PATTERN-fit finding the forked fan lacked, and left the other two `BLOCKING` findings out of reach.

## Summary table

One run per cell, `P4` only.
The fan reference is 11 findings, 3 of them `BLOCKING`, from `opus` at medium effort with 3 lens forks, as the earlier benches recorded.
"Found" counts distinct fan findings that a finding of the setting matched.

| Setting | Findings | Shared with fan | Unique real | False | Fan findings found | `BLOCKING` found | Verdict as fan's | Fresh input tokens | Wall time | Window points |
|---|---|---|---|---|---|---|---|---|---|---|
| `gpt-6-astra` high, forked fan with the Claude context | 8 | 2 | 6 | 0 | 2 of 11 | 0 of 3 | yes | 324k | 980 s | weekly 8, shared with a sibling bench |
| `gpt-6-astra` high, forked fan | 9 | 1 | 8 | 0 | 1 of 11 | 0 of 3 | yes | 299k | 737 s | weekly 5 |
| `gpt-6-astra` medium, cold-call fan | 8 | 0 | 8 | 0 | 0 of 11 | 0 of 3 | yes | 466k | 907 s | weekly at most 3 |
| `gpt-6-astra` medium, solo | 4 | 2 | 2 | 0 | 2 of 11 | 0 of 3 | yes | 77k | 166 s | at most 15 of the 5-hour window |
| Claude `opus` medium, solo | 6 | 2 | 4 | 0 | 2 of 11 | 0 of 3 | no (`APPROVED`) | 128k | 389 s | none |
| Claude `opus` medium, fan | 11 | | | | | | | 489k, plus 79k in forks | 735 s | none |

The last five rows are the forked fan bench's `P4` rows.
Fresh input tokens for Astra are `input_tokens` less `cached_input_tokens` of the rollout's last `token_count`, the handler's and the three forks' summed; Opus counts cache creation as fresh input, so the figures are not the same measure.
The window points are those of the weekly window only, from 9 percent at the handler's first request to 17 at its last.
The sibling bench `codex-astra-tuned-prompt-bench` ran on the same account over part of the time, so the 8 points are not this run's alone.

## Per call

| Call | Wall time | Fresh input tokens | Output tokens | Commands | Weekly window after |
|---|---|---|---|---|---|
| Handler | 980 s | 213k | 17.9k | 30 | 17 percent |
| Fork `affected-set` | 166 s | 48k | 3.4k | 6 | 12 percent |
| Fork `guard-reach` | 310 s | 38k | 5.8k | 6 | 15 percent |
| Fork `way-forward` | 278 s | 26k | 5.5k | 6 | 14 percent |

The handler's figures are its own thread's: its total grows with its own turns and not with the forks', so the forks' rows add to it.
The forks' wall times are from spawn to their last message, and their window readings come from the last `token_count` in each fork's rollout, so they are not a sequence.
The handler's wall time is the whole `codex exec`: 133 s of reading before the first spawn, 454 s until the last fork returned, and 525 s of consolidation.

## Checks from the rollouts

The rollouts are the first check the task asked for; `data/rollout-evidence.txt` holds the figures.

- Started: the handler's rollout holds three `spawn_agent` calls, `affected_set`, `guard_reach` and `way_forward`, each with `fork_turns` set to `"all"`, at 12:17:29, 12:17:40 and 12:17:48 UTC.
- Concurrent: the three forks' intervals, 12:17:29 to 12:20:15, 12:17:40 to 12:22:50 and 12:17:48 to 12:22:26, overlap from 12:17:48 to 12:20:15.
- Inherited context: each fork's `session_meta` carries `forked_from_id` and `parent_thread_id` of the handler's session, and its first model request already held 155.3k, 155.7k and 156.0k input tokens, of which 152.6k, 153.0k and 153.3k were cache hits, against 152.4k for the handler's request at the spawn.
  The forks ran 6 commands each, within the cap of 6 that the boilerplate sets, and each ended with its tool-call log as the boilerplate asks.
- `AGENTS.md` loaded: the handler's rollout holds, as its eighth item (ordinal 7), a `user` message `# AGENTS.md instructions for <work>` of 4962 characters whose body is the snapshot's `CLAUDE.md`.
  The forks inherited it with the rest of the handler's context.
  Codex found the file in the `-C` directory, which the bench staged without a `.git`.

## Context against the Claude round

What this run gave the handler, and what the Claude fan's round 1 on `P4` had that it still lacks.

Given:

- The PATTERN directive of `pattern.RoleReviewFix`, as `burlerengine.Review` renders it at the code commit: the `pattern-directive-review-fix` stencil filled with the snapshot's whole `PATTERN.md` and the declared-changes fragment built from the decision record's `## PATTERN changes` bullets (one, `PATTERN-owner-machine`), in the explore step's `pattern_directive` slot.
  The overview is 15.7k characters; the lens forks inherit it.
- `AGENTS.md` in the snapshot's root, a copy of the snapshot's `CLAUDE.md`.
- Everything the forked fan bench already gave: the round-1 explore and review steps with the focus directive, the fan cluster rules, the lenses and the fork boilerplate.

Still different:

- Plugin skills: a Claude session loads the scribe, ly and other skills the seat is launched with (`seatSkills`); Codex loads none.
- Harness: Claude Code's own system prompt, tool set and settings against Codex's own.
  The scratch `CODEX_HOME` holds only `auth.json`, so the operator's `config.toml` and any user-level `AGENTS.md` did not apply.
- Tool-use rules: the bench's explore step says "read files, run nothing", where production fills `toolUseRules` from the profile; the handler wrote that this stopped it running the overlay validation the rubric names.
  The forks and the handler did read with shell commands, as the earlier benches allowed.
- Fork mechanism: Claude's `Agent` tool with `subagent_type: "fork"` against Codex's `spawn_agent` with `fork_turns="all"`; the boilerplate's wording was changed to match, as in the forked fan bench.
- The snapshot is a plain directory: no git history, no `lym` binary, no board, and no hub specs outside `.bench/`.
- Model and effort: `gpt-6-astra` at high against `opus` at medium.

## The handler's findings

The handler returned `BLOCKING` with 8 findings: 3 of its own and 5 from the forks.
The forks returned 7 findings: `affected-set` 2, `guard-reach` 2 and `way-forward` 3.
The handler kept 6 of them, as 5 findings, since it merged both of `affected-set`'s, and dropped 1.
`data/match.tsv` holds each reading.

| Finding | Severity | Origin | Defect |
|---|---|---|---|
| `owner-write-skip-bypasses-publication` | `BLOCKING` | handler | Card 3 lets `BOARD_SKIP_GIT` and `BOARD_SKIP_PUSH` report an unpushed owner write as recorded |
| `short-owner-invariant-changes-rule` | `BLOCKING` | handler | Card 17's shortened `PATTERN-owner-machine` line forbids the declared Finalize exception and overstates origin-only reads |
| `prime-claim-overwrites-pulled-owner` | `MEDIUM` | `guard-reach` | A first-act prime claim is not covered by the slug-only `Takeover` guard and replaces an owner the setter's pull found |
| `adopter-first-retains-merge-remedy` | `MEDIUM` | `way-forward` | The adopter-first repair is appended to a push error that also says to merge origin and push |
| `repair-assumes-squash-is-head` | `MEDIUM` | `way-forward` | Card 12 names the task's squash by the parent head's SHA, which a later local commit makes another commit |
| `adopt-held-by-hub-network-pause` | `MEDIUM` | `way-forward` | Card 16 plans no network-gate exemption, so adopt's first reconcile waits behind a standing hub pause |
| `affected-set-enumeration-incomplete` | `MEDIUM` | `affected-set` | `list --text` drops the owner, and the new seams of cards 10 and 12 are in no `envkit.nilLegal` entry |
| `public-api-granularity` | `MEDIUM` | handler | Independent public surfaces are bundled into the cards that first consume them |

- `short-owner-invariant-changes-rule` is the shared finding the PATTERN context bought: the fan's `F6` names both omissions in card 17's line.
  The handler cites the declared `PATTERN-owner-machine` text and the plan spec's `Pattern-Departure` contract, which only the inlined directive and its declared-changes fragment put in front of it.
- `public-api-granularity` is shared with the fan's `F9`; the handler rates it `MEDIUM` and the fan `LOW`.
- The dropped finding is `guard-reach`'s: card 13 has `Command` copy the owner-pause seam that `newRoot`'s pre-run installs, so a fresh process captures nil.
  The handler rejected it because `Command` fills its other seams inside its `PersistentPreRunE`; the card's wording leaves both readings open, so it is graded `unique-real` and arguable.
- `affected-set-enumeration-incomplete` merges two fork findings; its `list --text` half is arguable, since card 4 tests `owner_machine` on the JSON list.
- The handler's `## Rejected` section also lists four candidates of its own that it did not keep.

## Method

### Inputs and prompts

`P4` is rebuilt with `data/harness/build.sh` from the earlier benches' weft commit and code commit in `data/inputs.txt`, then `build-fan.sh` builds the explore step with the fork-reading block appended and `build-forkfan.sh` builds the handler prompt, all as in the forked fan bench.
The one change is in `build.sh`: it fills the explore step's `pattern_directive` and copies the snapshot's `CLAUDE.md` to `AGENTS.md`.
The directive is built from the snapshot's own `contracts/stencils/pattern/` stencils, and the declared changes are the bullets of the record's `## PATTERN changes` section, as `discussionparser.DeclaredPatternChanges` returns them.
`data/harness/evidence.sh` prints `data/rollout-evidence.txt` from the rollouts.
All prompts are in `data/prompts/`, with the `AGENTS.md` the handler loaded.

### Call

```text
CODEX_HOME=<scratch>/codex-home codex exec -m gpt-6-astra -c model_reasoning_effort="high" \
  -s workspace-write -C <work> --skip-git-repo-check --json -o <last-message> - < prompt
```

`data/harness/run-forkfan.sh` runs the one call, after `setup-codex-home.sh` made the scratch `CODEX_HOME` and linked it to the operator's `auth.json`, then copies every rollout the call created.
The handler wrote its review to the file the prompt names; `-o` held an empty last message, so `reviews/P4/handler.md` is that file.
Each fork's review is the last assistant message of its rollout.
The windows come from the last `token_count` event of each rollout (`windows.sh`).

### Grading

One Claude session graded every handler finding as the earlier benches did: against the fan's round-1 findings of `P4`, and an unmatched finding against the cited card lines and the snapshot's code.
`shared` means the finding names the same defect as a fan finding; `unique-real` means the claim holds and the fan's round has no finding of it; `false` means it does not hold.
A finding on the same card as a fan finding but naming another defect is `unique-real`.

## Deviations and limits

- One input, one run, one grader: no variance is measured, and a repeat could change a count of one or two.
- The grader is a Claude session and may favour its own family's phrasing; the rates are not validated.
- The gain of one shared finding over the forked fan bench is inside run-to-run noise that was never measured; that the PATTERN block caused it is a reading of the finding's wording, not a result.
- The context added about 8k tokens to the forks' inherited prefix, which raises every fork's reading cost against the forked fan bench's.
- Many of the handler's reads came back truncated by the per-command output cap it set itself, so what it saw of a file is not what the file holds; this was not compared with the forked fan bench.
- The weekly window moved from 9 to 17 percent over the run with a sibling bench running on the same account, so its points are not this run's alone; the stop at 70 percent was never near.
- The fork prompts are encrypted in the rollout; that each fork's lens text was the boilerplate and the lens alone rests on the handler following the cluster rules, and on the forks' final messages starting with their lens lists.
- Recall is against the fan's list, not against the truth; the 6 `unique-real` findings and the dropped one hold on reading, so the fan reference is not complete for `P4`.
- The operator's `~/.codex/config.toml` was not hashed before or after this bench.

## Data

`data/` holds:

- `inputs.txt`: the input with weft commit, round start time and code commit.
- `usage.txt`: one line per call with return code, wall seconds, tokens and the weekly `used_percent`, then the handler's verdict.
- `rollout-evidence.txt`: start, end, parent thread, command count and tokens of each rollout, the handler's `spawn_agent` calls and its `AGENTS.md` message.
- `match.tsv`: the grading of every handler finding and of the dropped fork finding.
- `reviews/P4/`: the handler's review and the three forks' final messages.
- `prompts/`: the handler prompt, the two instruction files it reads and the `AGENTS.md` it loaded.
- `harness/`: `build.sh`, `build-fan.sh`, `build-forkfan.sh`, `run-forkfan.sh`, `setup-codex-home.sh`, `windows.sh`, `evidence.sh` and `dump.sh`, working on `.scratch/bench/`, which is not committed.
