# Codex Astra lens fan against the Claude fan on one plan

Bench of 2026-10-27, a follow-up to `docs/research/codex-astra-solo-bench-2026-10-25/report.md`.
Codex CLI, ChatGPT sign-in (plan `prolite`), Linux.
Four `codex exec` calls in all, one run each: three lens reviews and one handler review, `gpt-6-astra` at medium effort, over the plan `P4` (`machine-handover`).
Nothing was built: the bench is shell scripts under `data/harness/`, the reviewers' own output and a hand grading.

## Conclusion

Astra's fan found none of the Claude fan's 11 findings on `P4`, and none of its 3 `BLOCKING` ones.
Solo Astra had reproduced 2 of the 11 and none of the `BLOCKING` ones, so the fan shape did not raise recall against this reference.
The fan did return the fan's verdict (`BLOCKING`) and 8 findings, every one of which holds on reading and is absent from the fan's list, against solo Astra's 2 unique real findings.

- The handler's 8 findings all graded `unique-real`, none `false`; the one lens finding it dropped also holds, marginally.
- Two of the 8 are the defects both solo reviewers found: the prime claim that the no-`Takeover` guard does not cover, and the skip-variable bypass.
- Six are new on `P4`: the owner-pause seam copied before the root pre-run installs it (`BLOCKING`), `Finalize`'s parent fetch ahead of the owner checks, the pull replay that reaches a fast-forward push unguarded, the adopter-first repair beside a contradicting push recovery, the strict sleep threshold, and two `adopt` refusals that name no correction.
- The fan's three `BLOCKING` findings need a read the lenses did not make: the owner guard on `retryWeftPush` reading origin's Bolt outside the reconcile, the missing `adoptcli` entry in `PATTERN-cli-cobra`'s background, and the one-second production wait in `TestSetOwners`.
  Lens `affected-set` came nearest on the first, with a different defect in the same cards (5 and 6).
- The fan ran 907 s over four calls against 166 s for solo Astra, and 466k fresh input tokens against 77k.

With one input, one run per cell and one grader, the sample cannot rank the fan against solo Astra or against the Claude fan.
It shows that the fan shape gave Astra more real findings, not more of the Claude fan's.

## Summary table

Medium effort, one run per cell, `P4` only.
The fan reference is 11 findings, 3 of them `BLOCKING`, from `opus` at medium effort with 3 lens forks, as the earlier benches recorded.
"Found" counts distinct fan findings that a finding of the setting matched.

| Setting | Findings | Shared with fan | Unique real | False | Fan findings found | `BLOCKING` found | Verdict as fan's | Fresh input tokens | Wall time | Window points |
|---|---|---|---|---|---|---|---|---|---|---|
| `gpt-6-astra` medium, fan | 8 | 0 | 8 | 0 | 0 of 11 | 0 of 3 | yes | 466k | 907 s | weekly at most 3 |
| `gpt-6-astra` medium, solo | 4 | 2 | 2 | 0 | 2 of 11 | 0 of 3 | yes | 77k | 166 s | at most 15 of the 5-hour window |
| Claude `opus` medium, solo | 6 | 2 | 4 | 0 | 2 of 11 | 0 of 3 | no (`APPROVED`) | 128k | 389 s | none |
| Claude `opus` medium, fan | 11 | | | | | | | 489k, plus 79k in forks | 735 s | none |

The solo and Claude fan rows are the solo bench's `P4` rows.
Fresh input tokens for Astra are `input_tokens` less `cached_input_tokens` of the `turn.completed` event; Opus counts cache creation as fresh input, so the figures are not the same measure.

## Per call

| Call | Wall time | Fresh input tokens | Output tokens | Findings | Weekly window after |
|---|---|---|---|---|---|
| Lens `affected-set` | 263 s | 128k | 6.0k | 3 | 1 percent |
| Lens `guard-reach` | 220 s | 139k | 5.2k | 3 | 2 percent |
| Lens `way-forward` | 187 s | 103k | 4.1k | 2 | 3 percent |
| Handler | 237 s | 96k | 5.5k | 8 | 3 percent |

The plan reports one rate-limit window, of 10080 minutes, and no 5-hour window; the stop at 70 percent was never near.
The first reading came after the first call, so the whole bench cost at most 3 points of the weekly window.

## The handler's findings

The handler returned `BLOCKING` with 8 findings: 2 of its own and 6 from the lenses.
The lenses returned 8 findings: the handler kept 7 of them as 6 findings (two lens reports described the pull replay and merged into one) and dropped 1 with a reason.
All were read against the cited card lines and, where a claim concerns code, against the snapshot; `data/match.tsv` holds each reading.

| Finding | Severity | Origin | Defect |
|---|---|---|---|
| `F1` | `BLOCKING` | handler | Card 13 copies the owner-pause seam in `orchcli.Command`, built in `newRoot` before the root pre-run installs it, so a fresh process captures nil and starts without the owner pauses |
| `F2` | `BLOCKING` | handler | Card 3 lets `BOARD_SKIP_GIT` and `BOARD_SKIP_PUSH` report an unpushed owner write as recorded |
| `F3` | `MEDIUM` | `guard-reach` | A prime claim is not covered by the slug-only `Takeover` guard and replaces an owner the setter's pull found |
| `F4` | `MEDIUM` | `guard-reach` | `Finalize.Call` fetches the parent tips in `holdParent` before step 1b3, so a fully offline run never reaches the new checks |
| `F5` | `MEDIUM` | `affected-set`, `guard-reach` | A pull replays old-owner records and the following push is a fast-forward that never meets the guarded retry |
| `F6` | `MEDIUM` | `way-forward` | The adopter-first repair is appended to a push error that tells the reader to merge origin and push |
| `F7` | `LOW` | `affected-set` | `SleepWatch.Observe` records only a gap above the tolerance, but card 10 needs a sleep of at least a minute |
| `F8` | `LOW` | `way-forward` | Two `adopt` argument refusals name no correction |

- The handler's `BLOCKING` on `F2` matches the solo Astra rating of the same defect; the decision record does not say whether the skip variables count as test-only, so the finding stays arguable.
- Dropped by the handler: `affected-set`'s finding that card 16 does not add `adoptcli` to the lists of `cmd/lyx/seamsignature_test.go`.
  The claim holds, but the lists already omit `battencli`, `shedcli` and other modules, so graded `unique-real` and marginal.
- The lens reports end in the lists their procedures ask for; `guard-reach` and `way-forward` list every guard and refusal they traced, and the handler's `## Rejected` section gives one line per rejected item.

## Method

### Inputs and prompts

`P4` is rebuilt with `data/harness/build.sh` from the solo bench's weft commit and code commit in `data/inputs.txt`: a `git archive` of the code commit extracted under `.scratch/`, with the plan, the decision record, the era's rubric and stencils and the round's focus directive.
`data/harness/build-fan.sh` builds the fan prompts from the code commit's stencils: the explore step with the fork-reading block appended, one lens prompt per lens, and the handler prompt.
The lenses are the lenses of round 1: `affected-set`, `guard-reach` and `way-forward`, with their text from `internal/lenses/template.yaml` at the code commit.
All prompts are in `data/prompts/`.

### Lens runs

Each lens is its own `codex exec` over a fresh copy of the snapshot.
A Claude fork inherits the handler's reading; a separate call has none, so the lens prompt carries the round's explore step and its fork-reading block, which the forks inherited, then the fork rules and `Your lens: <name>` with the lens text.
The fork boilerplate changes in two places: its inherited-context sentence becomes an instruction to do the reading, and its six-tool-call cap is dropped, since a run with no inherited context needs its reading.
The read-only rule, the rule to report only the lens's own procedure and the final-message output rule stay.
The lens output is the call's last message.

### Handler run

The handler prompt is the round-1 reviewer template with the fan cluster rules, in which the spawn paragraph and the fork rules are replaced by the three lens outputs as files in the handler's copy.
Its holistic review, consolidation, `origin:` tags and `## Rejected` section are the stencil's.
The handler read the lens files itself; no lens finding was pasted into the prompt.

### Calls

```text
CODEX_HOME=<scratch>/codex-home codex exec -m gpt-6-astra -c model_reasoning_effort="medium" \
  -s workspace-write -C <work> --skip-git-repo-check --json -o <last-message> - < prompt
```

`data/harness/run-call.sh` runs one call, after `setup-codex-home.sh` made the scratch `CODEX_HOME` and linked it to the operator's `auth.json`.
The calls ran in sequence, so each window reading belongs to one call.
The window comes from the last `token_count` event of the call's rollout (`windows.sh`).

### Grading

One Claude session graded every handler finding and the dropped lens finding as the solo bench did: against the fan's round-1 findings of `P4`, and an unmatched finding against the cited card lines and the snapshot's code.
`shared` means the finding names the same defect as a fan finding; `unique-real` means the claim holds and the fan's round has no finding of it; `false` means it does not hold.
A finding on the same card as a fan finding but naming another defect is `unique-real`.

## Deviations and limits

- One input, one run per call, one grader: no variance is measured, and a repeat could change a count of one or two.
- The grader is a Claude session and may favour its own family's phrasing; the rates are not validated.
- The lens runs differ from forks in reading for themselves, so the fan's token and time figures are not those of forks inheriting context, and the lens prompt's changes to the boilerplate favour Astra on reading depth.
- The plan is now ChatGPT Pro (`prolite` in the rollout) where the solo bench ran on `plus`; only a weekly window is reported, so the window points are not comparable with the solo bench's 5-hour points, and no reading precedes the first call.
- Recall is against the fan's list, not against the truth; the 8 findings all hold on reading, so the fan reference is not complete for `P4`.
- `F2` and `F7` rest on a reading of the decision record's silence and of a boundary, and stay arguable.
- The operator's `~/.codex/config.toml` was not hashed before or after this bench.

## Data

`data/` holds:

- `inputs.txt`: the input with weft commit, round start time and code commit.
- `usage.txt`: one line per call with return code, wall seconds, tokens and the weekly `used_percent` after the call, then the handler's verdict.
- `match.tsv`: the grading of every handler finding and of the dropped lens finding.
- `reviews/P4/`: the three lens outputs and the handler's review.
- `prompts/`: the lens prompts, the handler prompt and the two instruction files they read.
- `harness/`: `build.sh`, `build-fan.sh`, `run-call.sh`, `setup-codex-home.sh`, `windows.sh` and `dump.sh`, working on `.scratch/bench/`, which is not committed.
