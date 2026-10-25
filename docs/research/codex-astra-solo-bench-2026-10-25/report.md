# Codex Astra against solo Opus as a plan reviewer

Bench of 2026-10-25, a follow-up to `docs/research/codex-review-bench-2026-10-25/report.md`.
Codex CLI 0.164.0, ChatGPT sign-in (plan `plus`), Linux.
Four reviews in all, one run each: `gpt-6-astra` and Claude `opus`, both at medium effort and solo with no lens forks, on the plans `P1` (`gate-runs-once`) and `P4` (`machine-handover`).
Nothing was built: the bench is shell scripts under `data/harness/`, the reviewers' own output and a hand grading.

## Conclusion

On these two plans Astra and solo Opus reproduced the same share of what the Claude fan found, and both fell well short of its `BLOCKING` findings.
Each reproduced 4 of the fan's 23 findings; Opus reproduced 1 of its 5 `BLOCKING` ones and Astra none.
Both added findings that hold up on reading: 7 of Opus's 12 findings and 2 of Astra's 6 are real and absent from the fan's list.
No finding of either was graded false.

- Astra is about 2.3 to 3 times faster (166 s and 184 s against 389 s and 563 s) and writes fewer findings: 6 against 12.
- Astra returned the fan's verdict on both plans (`BLOCKING`); Opus returned `APPROVED` on `P4`, where the fan blocked on three `BLOCKING` findings.
- Astra's two `P4` `BLOCKING` findings are real on reading, and Opus filed the second as `MEDIUM`.
  The `P4` finding on the skip variables is Astra's alone.
- Astra's two runs took 22 points of the 5-hour window on `P1` and at most 15 on `P4`.
  Opus took none of it.
- Astra's findings are thin on `P1`: two, both already in the fan's list, against Opus's six.

With two inputs, one run per cell and one grader, the sample cannot rank the two.
It does not show that Astra reviews at a lower level than solo Opus, and it does not show that either replaces the fan: both missed the fan's `BLOCKING` findings about code-reading defects on `P4`.

## Summary table

Medium effort, one run per cell.
The fan reference over `P1` and `P4` is 23 findings, 5 of them `BLOCKING`, from `opus` at medium effort with 3 lens forks per input, as the earlier bench recorded in `claude-findings.tsv` and `claude-usage.tsv`.
"Found" counts distinct fan findings that a finding of the setting matched.

| Setting | Inputs | Findings | Shared with fan | Unique real | False | Fan findings found | `BLOCKING` found | Verdict as fan's | Fresh input tokens | Wall time | 5-hour window |
|---|---|---|---|---|---|---|---|---|---|---|---|
| `gpt-6-astra` medium | 2 of 2 | 6 | 4 | 2 | 0 | 4 of 23 | 0 of 5 | 2 of 2 | 169k | 350 s | 22 points on `P1`, at most 15 on `P4` |
| Claude `opus` medium, solo | 2 of 2 | 12 | 5 | 7 | 0 | 4 of 23 | 1 of 5 | 1 of 2 | 335k | 952 s | none |
| Claude `opus` medium, fan | 2 of 2 | 23 | | | | | | | 873k, plus 171k in forks | 1339 s | none |

Fresh input tokens: for Astra `input_tokens` less `cached_input_tokens` of the `turn.completed` event; for Opus `input_tokens` plus `cache_creation_input_tokens` summed over the session transcript's assistant messages, once per message id.
The two figures count different things, since Codex reports no cache-creation figure.
Cached reads are 90 percent of Astra's input and 95 to 97 percent of Opus's.
The Opus transcript's `output_tokens` is 30k and 49k; Astra's is 4k for both runs, with 1k and 2k of reasoning.

## Per input

### `P1`, `gate-runs-once` plan, 33 cards

The fan found 12 findings, 2 of them `BLOCKING`, and blocked.

| Setting | Findings | Verdict | Shared with fan | Unique real | False | Wall time | Fresh input tokens | 5-hour window |
|---|---|---|---|---|---|---|---|---|
| Astra | 2 | `BLOCKING` | 2 | 0 | 0 | 184 s | 93k | 22 points |
| Opus solo | 6 | `BLOCKING` | 3 | 3 | 0 | 563 s | 206k | none |
| Fan | 12 | `BLOCKING` | | | | 604 s | 384k, plus 92k in forks | none |

- Shared, both: the missing re-sign arrows on card 4 and card 14 (fan `F4`, `MEDIUM`).
  Opus names both cards and adds the test caller of `darnTreeScope`, which the fan's text gives as a consequence of the missing arrow.
- Shared, Astra only: the retag cards use `Custom` on a premise that `Edit` rules out (fan `F3`, `MEDIUM`).
  Astra rates it `BLOCKING`, so its verdict rests on a finding the fan rated `MEDIUM`.
- Shared, Opus only: card 23 shortens `PATTERN.md` after cards 3, 7 and 19 have grown it, so their card gates fail the pattern lint (fan `F1`, `BLOCKING`).
  Opus finds in addition that card 33 grows the file past the line again after card 23.
  Astra did not find this.
- Unique real, Opus: card 20's new integration test repeats what its own rewritten tests cover (`LOW`); card 7 puts `green.json` at `WaitDir(anchor)` while `gatecli` calls `WaitDir` with both the anchor and the worktree path (`LOW`); card 7 omits the write through `internal/state` that `PATTERN-tolerant-records` requires (`LOW`).
- Neither found the fan's second `BLOCKING` finding, the hand-enumerated readers of the retirement cards (`F2`), nor the fan's `MEDIUM` findings `F5` and `F6` about code facts.

### `P4`, `machine-handover` plan, 17 cards

The fan found 11 findings, 3 of them `BLOCKING`, and blocked.

| Setting | Findings | Verdict | Shared with fan | Unique real | False | Wall time | Fresh input tokens | 5-hour window |
|---|---|---|---|---|---|---|---|---|
| Astra | 4 | `BLOCKING` | 2 | 2 | 0 | 166 s | 77k | at most 15 points |
| Opus solo | 6 | `APPROVED` | 2 | 4 | 0 | 389 s | 128k | none |
| Fan | 11 | `BLOCKING` | | | | 735 s | 489k, plus 79k in forks | none |

- Shared with the fan: card 8's prime claim against the offline legacy-run resume (Astra, fan `F5`, `MEDIUM`); the bundling of public symbols into consumer cards (both, fan `F9`, `LOW`); card 17's short `PATTERN.md` line dropping the past-merge exception (Opus, fan `F6`, `MEDIUM`).
- Found by both and absent from the fan's list: card 3's no-`Takeover` guard covers slugs only, so a prime claim can replace another machine's prime owner.
  Astra rates it `BLOCKING`, Opus `MEDIUM`.
- Unique real, Astra: card 3 reads an owner write under `BOARD_SKIP_GIT` or `BOARD_SKIP_PUSH` as recorded, though nothing was pushed, and the decision record requires a push before a start or resume (`BLOCKING`).
  `ApplySkipEnv` reads ordinary environment variables, so the case is reachable outside tests.
  The decision record does not say whether the skip variables count as test-only.
- Unique real, Opus: card 12 states its past-merge carve-out without the bound the decision record gives it (`MEDIUM`); card 17 plans a `Pattern-Departure` trailer for a commit that departs from nothing (`LOW`); the `ParentOf` type of card 6's `SeamOptions` disagrees with its `OwnerReader` builder (`LOW`).
- Neither found the fan's three `BLOCKING` findings: the owner guard on `retryWeftPush` reading origin's Bolt outside `ReconcileOrigin`, the missing `adoptcli` entry in `PATTERN-cli-cobra`'s background, and the one-second production wait in `TestSetOwners`.
  All three need a read of code or of a PATTERN background file that neither reviewer opened in its explore step.
- Opus approved `P4` with six findings and no `BLOCKING`, including one the fan rated `MEDIUM` and Astra `BLOCKING` for the same defect.
  The rubric's rating of that defect is not settled by this sample.

## What the two found that the fan did not

Astra: the skip-variable bypass of the publish-before-start rule on `P4`.
Opus: card 7's `green.json` location and its missing `internal/state` write on `P1`, card 12's unstated bound, card 17's trailer and card 6's type mismatch on `P4`, card 20's redundant test on `P1`.
Both: the prime claim without `Takeover` on `P4`.
All were read against the cited card text, and where a claim concerns code against the snapshot's code.
`data/match.tsv` holds the reading of each.

## Method

### Inputs and prompt

`P1` and `P4` are the earlier bench's inputs, rebuilt with `data/harness/build.sh`, whose paths now point at this worktree.
The weft commits and code commits equal the earlier bench's `data/inputs.txt` (`c36025a89` over `ab038b4e8` and `045707759` over `750903aef`), copied into `data/inputs.txt`.
Each snapshot is a `git archive` of the code commit extracted under `.scratch/`, with the plan, the decision record, the era's rubric and stencils, and the reviewer prompt of the earlier bench: round 1, the solo cluster rule and the round's focus directive, without the inlined PATTERN overview.
Both reviewers read the same `prompt.md` over a fresh copy of the same snapshot.

### Astra

```text
CODEX_HOME=<scratch>/codex-home codex exec -m gpt-6-astra -c model_reasoning_effort="medium" \
  -s workspace-write -C <work> --skip-git-repo-check --json -o <last-message> - < prompt
```

`data/harness/run.sh` runs it, with the scratch `CODEX_HOME` that `setup-codex-home.sh` makes and links to the operator's `auth.json`.
The 5-hour and weekly `used_percent` come from the last `token_count` event of each run's rollout (`windows.sh`).

### Opus solo

`data/harness/run-opus.sh` copies the snapshot as `run.sh` does and runs `lyx shuttle run --model opus --effort medium --prompt-file <prompt> --output-file <work>/.bench/out/review.md`: an interactive tmux session, never `claude -p`.
The solo cluster rule in the prompt spawns no forks.
Wall time is the script's wall clock, which includes the session's start and its wait for the output file.
Tokens come from the session transcript, once per assistant message id.

### Grading

One Claude session graded every finding, in the earlier bench's way: a finding against the fan's round-1 findings of the same input, and an unmatched finding against the cited card lines and, where it concerns code, the snapshot.
`shared` means the finding names the same defect as a fan finding; `unique-real` means the claim holds and the fan's round has no finding of it; `false` means it does not hold.
A unique finding shared by Astra and Opus counts once for each.
Every label is in `data/match.tsv`, with the reviewer's own severity beside the fan finding's.

## Deviations and limits

- The 5-hour window was not read before the first Astra run, as the task asked: no reading exists without a run, since only a rollout holds one, and the newest rollout with a reading was over a day old.
  The first reading is the 15 percent after the `P4` run, so that run cost at most 15 points; the `P1` run cost 22 (15 to 37).
  The weekly window read 18 and then 21 percent.
  No run was refused or cut off, and the stop at 70 percent was not reached.
- Two inputs and one run per cell: no variance is measured.
  Differences of one or two findings, and Astra's 2 findings on `P1` against Opus's 6, are within what a repeat could change.
- The grader is a Claude session, which grades Opus findings and Astra findings alike but may favour its own family's phrasing.
  One grader read each finding, and the rates are not validated.
  Card 20's redundant test (Opus `P1` `F4`) was graded on the card text without running the tests, and Astra's skip-variable finding stays arguable on whether the decision record treats the variables as test-only.
- The Opus session runs in this worktree and so has its `CLAUDE.md` and plugin skills loaded, which the Codex run does not.
  Its prompt points at the snapshot only.
- Opus's token figure counts cache creation as fresh input; Codex reports none, so the two figures are not the same measure.
  The transcript's `output_tokens` per message may undercount streamed output.
- Recall is against the fan's list, not against the truth: the fan's `LOW` findings are in the 23.
- The operator's `~/.codex/config.toml` was not hashed before or after this bench.

## Data

`data/` holds:

- `inputs.txt`: the two inputs with weft commit, round start time and code commit.
- `usage.txt`: one line per run, with return code, wall seconds, tokens, verdict, finding count and, for Astra, the 5-hour and weekly `used_percent` after the run.
- `match.tsv`: the grading of every finding.
- `reviews/<input>/<setting>.md`: the four review files.
- `harness/`: `build.sh`, `run.sh`, `run-opus.sh`, `setup-codex-home.sh`, `windows.sh` and `dump.sh`, working on `.scratch/bench/`, which is not committed.
