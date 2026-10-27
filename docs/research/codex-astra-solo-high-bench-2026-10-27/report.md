# Codex Astra at high effort against medium effort, solo, on one plan

Bench of 2026-10-27, a follow-up to `docs/research/codex-astra-solo-bench-2026-10-25/report.md`.
Codex CLI 0.164.0, ChatGPT sign-in (plan `prolite`), Linux.
One `codex exec` call in all: `gpt-6-astra` at `high` effort, solo with no lens forks, over the plan `P4` (`machine-handover`).
Nothing was built: the bench is shell scripts under `data/harness/`, the reviewer's own output and a hand grading.

## Conclusion

On `P4`, raising Astra's effort from medium to high did not raise recall of the Claude fan's findings: it reproduced 1 of the fan's 11 findings where medium reproduced 2, and none of the 3 `BLOCKING` ones in either run.
High returned 3 findings against medium's 4, all three real on reading and none false, and returned the fan's verdict (`BLOCKING`) as medium did.

- Reasoning tokens rose from 988 to 5615, output tokens from 4.1k to 11.2k, fresh input from 77k to 134k and wall time from 166 s to 487 s, about 2.9 times.
- Kept from medium: the skip-variable bypass of the publish-before-start rule (`BLOCKING`) and the bundling of public symbols into consumer cards (fan `F9`).
- Lost against medium: the prime claim that card 3's no-`Takeover` guard does not cover, and card 8's prime claim against the offline legacy-run resume (fan `F5`).
- New at high: card 3's post-push `Lost` rule puts a skipped absent entry in `Lost`, which card 8 refuses, though card 8 expects an absent entry to pass (`MEDIUM`).
- Neither effort found the fan's three `BLOCKING` findings: the owner guard on `retryWeftPush` reading origin's Bolt outside `ReconcileOrigin`, the missing `adoptcli` entry in `PATTERN-cli-cobra`'s background, and the one-second production wait in `TestSetOwners`.

With one input, one run per cell and one grader, the sample cannot say that effort does or does not help: a repeat of either run could change a count of one or two.
It shows that on this plan the extra effort bought a different set of findings, not a larger one.

## Summary table

`P4` only, one run per cell.
The fan reference is 11 findings, 3 of them `BLOCKING`, from `opus` at medium effort with 3 lens forks, as the earlier benches recorded.
"Found" counts distinct fan findings that a finding of the setting matched.
The medium solo and fan rows are the solo bench's `P4` rows, not re-run.

| Setting | Findings | Shared with fan | Unique real | False | Fan findings found | `BLOCKING` found | Verdict as fan's | Fresh input tokens | Output tokens | Reasoning tokens | Wall time | Window points |
|---|---|---|---|---|---|---|---|---|---|---|---|---|
| `gpt-6-astra` high, solo | 3 | 1 | 2 | 0 | 1 of 11 | 0 of 3 | yes | 134k | 11.2k | 5.6k | 487 s | weekly 8 percent after the call |
| `gpt-6-astra` medium, solo | 4 | 2 | 2 | 0 | 2 of 11 | 0 of 3 | yes | 77k | 4.1k | 1.0k | 166 s | at most 15 of the 5-hour window |
| Claude `opus` medium, fan | 11 | | | | | | | 489k, plus 79k in forks | | | 735 s | none |

Fresh input tokens: `input_tokens` less `cached_input_tokens` of the `turn.completed` event; cached reads are 93.5 percent of the high run's input.
The Claude fan's figures count cache creation as fresh input, so they are not the same measure.

## Findings of the high run

The review returned `BLOCKING` with 3 findings.
Each was read against the cited card lines and, where a claim concerns code, against the snapshot; `data/match.tsv` holds each reading.

| Finding | Severity | Label | Defect |
|---|---|---|---|
| `F1` | `BLOCKING` | `unique-real` | Card 3 lets `BOARD_SKIP_GIT` and `BOARD_SKIP_PUSH` report an unpushed owner write as recorded, against D3 and D5 |
| `F2` | `MEDIUM` | `shared`, fan `F9` (`LOW`) | Independently reviewable public APIs are bundled into their consumers' cards |
| `F3` | `MEDIUM` | `unique-real` | Card 3's post-push `Lost` rule turns a skipped absent entry into an owner refusal |

- `F1` is the defect medium solo Astra rated `BLOCKING` and Opus did not find; the decision record does not say whether the skip variables count as test-only, so it stays arguable.
  High adds that card 8's claim test depends on the bypass and proposes a recording fake `Set` instead.
- `F2` names eight symbols across cards 3, 5, 11, 12, 15 and 16 where medium names the bundling in general; the fan rated the same defect `LOW`, high `MEDIUM`.
- `F3` holds on the card text.
  Card 3 gives an absent or done entry no claim and no owner, then reports each slug whose recorded owner is not `Machine` in `Lost`.
  `Owners.Effective` reads an absent slug as owned by no one, and card 8 refuses any claim whose slug is in `Lost` while expecting an absent entry to pass.
  The fan has no finding of it.
- The review's `## Rejected` section lists six items examined and dropped, among them the unbounded `Takeover` and the Finalize exemption; the prime-claim gap that medium filed as `BLOCKING` is not among them, so high did not weigh it.

## Against the medium solo run

| Medium finding | At high |
|---|---|
| `F1` skip variables, `BLOCKING` | found, `F1` |
| `F2` prime claim without `Takeover`, `BLOCKING` | not found; the review rejects an "unbounded `Takeover`" item as confined to adopt and does not weigh a prime claim |
| `F3` prime claim before reconcile against the offline legacy resume, fan `F5` | not found |
| `F4` bundled public APIs, fan `F9` | found, `F2` |

High found 2 of medium's 4 and added 1 finding medium did not make.

## Method

### Input and prompt

`P4` is rebuilt with `data/harness/build.sh` from the solo bench's weft commit and code commit in `data/inputs.txt`: a `git archive` of the code commit extracted under `.scratch/`, with the plan, the decision record, the era's rubric and stencils, the round's focus directive and the reviewer prompt of the earlier benches (round 1, solo cluster rule).
The prompt is the one the medium run read; only the effort differs.

### Call

```text
CODEX_HOME=<scratch>/codex-home codex exec -m gpt-6-astra -c model_reasoning_effort="high" \
  -s workspace-write -C <work> --skip-git-repo-check --json -o <last-message> - < prompt
```

`data/harness/run.sh` runs it, with the scratch `CODEX_HOME` that `setup-codex-home.sh` makes and links to the operator's `auth.json`.
`run.sh`, `dump.sh` and `windows.sh` are the solo bench's; `build.sh` and `setup-codex-home.sh` are the fan bench's, whose snapshot builder takes the code commit as an argument.
Their only changes are the root paths.
The windows come from the last `token_count` event of the call's rollout (`windows.sh`).

### Grading

One Claude session graded every finding as the earlier benches did: against the fan's round-1 findings of `P4`, and an unmatched finding against the cited card lines and the snapshot's code.
`shared` means the finding names the same defect as a fan finding; `unique-real` means the claim holds and the fan's round has no finding of it; `false` means it does not hold.

## Deviations and limits

- One input, one run, one grader: no variance is measured, and the difference between 4 and 3 findings, or between 2 and 1 fan findings found, is within what a repeat could change.
- The plan reports one rate-limit window, of 10080 minutes, and no 5-hour window, so its points cannot be set against the medium run's 5-hour points; no reading precedes the call, so the 8 percent is the weekly window after it, not the call's own cost.
  The stop at 70 percent was not reached.
- The medium solo row was run on a ChatGPT `plus` plan and this run on `prolite`; the plan tier may differ in more than the windows.
- The grader is a Claude session and may favour its own family's phrasing; the rates are not validated.
- `F1` stays arguable on whether the decision record treats the skip variables as test-only.
- Recall is against the fan's list, not against the truth: the unique findings hold on reading, so the list is not complete for `P4`.
- Wall time is the script's clock around the call, including the copy of the snapshot.
- The operator's `~/.codex/config.toml` was not hashed before or after this bench.

## Data

`data/` holds:

- `inputs.txt`: the input with weft commit, round start time and code commit.
- `usage.txt`: the call's line, with return code, wall seconds, tokens, verdict, finding count and the weekly `used_percent` after it.
- `match.tsv`: the grading of every finding.
- `reviews/P4/gpt-6-astra-high-r1.md`: the review file.
- `harness/`: `build.sh`, `run.sh`, `setup-codex-home.sh`, `windows.sh` and `dump.sh`, working on `.scratch/bench/`, which is not committed.
