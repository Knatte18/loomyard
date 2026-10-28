# Codex code-review bench with the decision record as yardstick: one added line, two medium runs

Bench of 2026-10-28, a follow-up to `docs/research/codex-code-review-bench-2026-10-28/report.md`.
Codex CLI 0.164.0, ChatGPT sign-in, Linux.
Two `codex exec` handler runs, one per input, `gpt-6-astra` at medium effort for handler and forks.
Each handler read the diff, then spawned the three Code-Review lens forks through `spawn_agent` with `fork_turns="all"`.
Nothing was built in `internal/`: the bench is the earlier bench's shell scripts, one changed prompt block, the reviewers' own output and a hand grading.

## Conclusion

With the added line, the forked fan found all 5 of the Claude fan's findings on `spawn-text-file-busy`, where the earlier bench's four runs found 2 of 5, but still found the same 3 of 6 on `fixer-overcap-review` and none of the record findings `F3`, `F5` and `F6` there.
The line drew no false finding on either input.
It moved the verdict on `spawn-text-file-busy` from `BLOCKING` to `APPROVED`, and the fan's `BLOCKING` finding was found at `MEDIUM`.

| Input | Fan findings found, this run (earlier runs) | Fan `BLOCKING` found | Verdict, this run (earlier runs) against fan | Real findings the fan lacks | False findings |
|---|---|---|---|---|---|
| `spawn-text-file-busy` | 5 of 5 (2 of 5, 2 of 5) | found at `MEDIUM`, not `BLOCKING` (0 of 1, 0 of 1) | `APPROVED` (`BLOCKING`, `BLOCKING`) against `BLOCKING` | 0 (1 and 0) | 0 (1 and 2) |
| `fixer-overcap-review` | 3 of 6 (3 of 6, 3 of 6) | none | `BLOCKING` (`BLOCKING`, `BLOCKING`) against `APPROVED` | 5 (3 and 4) | 0 (0 and 0) |

- The line let the fan find the record findings on `spawn-text-file-busy`.
  `F1`, that the start-failed cause does not read `start-failed after <n> tries: <error>`, was raised in the review as a `MEDIUM` finding citing `D4` and its acceptance criterion; `F2`, the loop-trace fallback gated on `res.startFailed`, was raised citing the same decision.
  The two earlier runs of this input rejected both by name.
  The review also found `F4`, which the earlier runs rejected as an extra test.
- On `fixer-overcap-review` the line changed none of the three record findings.
  `F5` (the dirty-tree halt reason) and `F6` (two stale docs) are in neither the review nor any lens report.
  `F3`, the spent-fixer no-review criterion, is in the code-tests lens report ("that table prohibits fixer roles but does not explicitly prohibit review roles") and was dropped when the handler consolidated, so the lens read the criterion and the review did not carry it.
- The review's tests finding on `fixer-overcap-review` now cites "explicit D3, D5 and D6 acceptance requirements" and was raised at `BLOCKING`, where the earlier runs raised it at `MEDIUM`; one run cannot tell whether the line caused the higher severity.
- The false `BLOCKING` finding of both earlier runs on `spawn-text-file-busy`, a `Warn` per ETXTBSY retry, was not raised; the review lists it under `## Rejected` on `D1`'s ground.
  The earlier bench's graders counted it as false for that same ground.
- The earlier bench found the same fan findings in both runs of each input, so 5 of 5 against 2 of 5 and 2 of 5 is outside that spread.
  One run per input still measures a single draw and not a rate; see Limits.
- The verdict `APPROVED` on `spawn-text-file-busy` sits beside five `MEDIUM` findings that the review says "still require correction".
  The fan's verdict there is `BLOCKING` for its one `BLOCKING` finding, which the review rated `MEDIUM`.

## The added line

`data/prompts/block-v2-code.md` is the earlier block with one bullet added after the first:

> Judge the diff against the decision record's decisions and its acceptance criteria as well as against the board entry snapshot: they are a yardstick beside it, and where the rubric above calls the record only the writer's own claim, that does not apply to them here; a divergence of the diff from a decision or an acceptance criterion is a finding, whether or not the board entry names it.

`diff` against the earlier block shows this bullet alone.
The build puts the block on the handler prompt and into the fork boilerplate as before, so every fork carries the line.

### Why it is worded this way

The rendered prompt told the runs, in three places, to use the board snapshot alone as the measuring stick:

- the fasit text of `darn-recipe.yaml`'s Code-Burler-Review profile: "The record's other sections are the writer's own claim, checked against the snapshot and never the answer key";
- the rubric's "A board entry snapshot" bullet: "check them against the snapshot, never use them as the measuring stick";
- the rubric's "Check every claim the board entry, the decision record and the change description make", which sets the record as a claim to test.

A line that only said "the record is a yardstick" would contradict these without saying which wins, and a model resolving it would side with the longer, earlier rubric text.
So the line names the exception (`D<n>` decisions and acceptance criteria, not the record's other sections), says that it overrides the rubric's "writer's own claim" reading for them, and says a divergence is a finding whether or not the board entry names it, since the earlier runs rejected `F1` for the reason that the entry names no spelling.
The line does not touch the rubric, the fasit or the target text; the entry fixes the change to the block.

## Setup

The earlier bench's harness, copied to `data/harness/` with the root path changed to this worktree's scratch tree and nothing else.
The staged prompts differ from the earlier bench's by the added line and path text only, checked with `diff` for the handler prompt and both instruction files of each input.
Inputs, reviewed commits (`7615d129` and `d50e5277`, base `161a4a7f`), weft commits (`abfabfcf6`, `c9fb5eb56`), reference findings, context parity, the fork cap of 20, the read-only tool-use rules and the model and effort are the earlier bench's; see its `## Inputs` and `## Setup`.
`data/context-hashes.txt` shows `AGENTS.md` equal to the earlier bench's hash and loaded once by each of the eight rollouts.
The reference findings are graded from `docs/research/codex-code-review-bench-2026-10-28/data/reference/`, which is not copied.

## Results

One run per cell.
The fan reference is 5 findings (1 `BLOCKING`) on `STB` and 6 (none `BLOCKING`) on `FOR`.
"Found" counts distinct reference findings that a Codex finding matched, as the earlier bench counted; one Codex finding may match several.
`data/match.tsv` holds each reading and `data/reviews/<input>/<run>/` holds each handler's review and the three forks' reports.

| Run | Findings | Shared | Unique real | False | Fan findings found | `BLOCKING` found | Verdict | Fresh input tokens | Wall time |
|---|---|---|---|---|---|---|---|---|---|
| `STB` this run | 5 | 5 | 0 | 0 | 5 of 5 | at `MEDIUM` | `APPROVED` | 126k | 327 s |
| `STB` earlier run 1 | 4 | 2 | 1 | 1 | 2 of 5 | 0 of 1 | `BLOCKING` | 108k | 306 s |
| `STB` earlier run 2 | 4 | 2 | 0 | 2 | 2 of 5 | 0 of 1 | `BLOCKING` | 115k | 276 s |
| `FOR` this run | 6 | 1 | 5 | 0 | 3 of 6 | none | `BLOCKING` | 188k | 377 s |
| `FOR` earlier run 1 | 4 | 1 | 3 | 0 | 3 of 6 | none | `BLOCKING` | 180k | 391 s |
| `FOR` earlier run 2 | 7 | 3 | 4 | 0 | 3 of 6 | none | `BLOCKING` | 168k | 347 s |
| Claude fan, `STB` | 5 | | | | | | `BLOCKING` | 138k | 131 s |
| Claude fan, `FOR` | 6 | | | | | | `APPROVED` | 178k | 147 s |

Fresh input tokens are `input_tokens` less `cached_input_tokens` of each rollout's last `token_count`, summed over the handler and its three forks (`data/usage.txt`), as in the earlier bench; the Claude fan's are not the same measure, see that report.
Wall time is the whole `codex exec`.
The weekly `used_percent` went from 5 to 9 over the two runs, which ran in parallel on one account; the stop at 70 percent was not reached.

### Per call

| Run | Handler wall | Handler fresh | Fork fresh, three forks | Longest fork's own commands |
|---|---|---|---|---|
| `STB` | 324 s | 87k | 12k, 11k, 17k | 3 |
| `FOR` | 374 s | 118k | 23k, 26k, 21k | 4 |

Each handler's rollout holds three `spawn_agent` calls, `code_correctness`, `code_affected_set` and `code_tests`, each with `fork_turns` set to `"all"`; each fork's `session_meta` names its handler as `forked_from_id`.
The cap of 20 never bound.
The fork prompts are encrypted in the rollouts, so that every fork held the added line rests on the handler following the cluster rules, as in the earlier bench; `data/rollout-evidence.txt` has the rest.

## The record findings

| Input | Record finding | Found | How |
|---|---|---|---|
| `STB` | `F1` (`BLOCKING`), cause format | yes, at `MEDIUM` | `F1`: "D4 requires `start-failed after <n> tries: <error>`", with the overstated count of a cancelled backoff folded in |
| `STB` | `F2` (`MEDIUM`), loop-trace fallback | yes | `F2`: "D4 and its acceptance criterion require" the fallback for every evidence-free interruption |
| `FOR` | `F3` (`MEDIUM`), spent-fixer criterion | no | read by the code-tests lens, absent from the review |
| `FOR` | `F5` (`LOW`), dirty-tree halt reason | no | absent from the review and every lens report |
| `FOR` | `F6` (`LOW`), two stale docs | no | the review raised a different stale comment, on `Finalize.Call`, and not `llm-model-spec.md:128` or `docs/overview.md:585` |

New false findings drawn by the line: none on either input.
On `STB` the review's `## Rejected` names the `Warn` per retry and a helper-only `IsSpawnBusy` test, both with reasons.

## The findings

### `spawn-text-file-busy`

- Found: `F1`, `F2`, `F3` (a cancelled backoff reports five tries, folded into the review's `F1` and its own test finding), `F4` (the review's `F4` names the empty callback result and precedence, not the nil-callback row) and `F5`.
- Missed: none.
- Unique real: none.
  The earlier run 1's trace-precedence point is part of the review's `F4`.
- False: none.

### `fixer-overcap-review`

- Found: `F1`, `F2` and `F4`, as one `BLOCKING` finding (`F3` of the review), against `MEDIUM` in the earlier runs.
- Missed: `F3`, `F5` and `F6`, as above.
- Unique real: five, all raised by one or both earlier runs: the approval rewrite dropping unknown keys against `PATTERN-tolerant-records` (`BLOCKING`), the missing self-check and parity check of the `fix-review-report` gate (`BLOCKING`, arguable on the reading the earlier bench gave it), a wait error from a started review session returning before the dirty-tree and HEAD checks (`MEDIUM`), the bare `verdict: approved` passing the parser (`MEDIUM`, `LOW` before) and the `Finalize.Call` comment (`LOW`).

## Limits

- Two runs, one per input: no rate can be read, and a difference from the earlier runs is a difference from four draws and one.
  The gain on `STB` lies outside the earlier runs' spread of the fan findings found (the same ones in both runs of each input), but one run could reach 5 of 5 by chance.
- The fan's findings found rose on one input and not on the other, so the line cannot be said to work on both kinds of record finding; `FOR`'s missed findings are a dropped consolidation (`F3`) and two low-severity findings the review never raised (`F5`, `F6`), which a yardstick line does not address.
- One grader, the writer of the earlier bench's matching rule and of this run's too; the real-or-false calls on `FOR`'s unique findings are carried over from the earlier bench's readings of the same defects, and the gate-parity one is arguable.
- The reference is one run of a `sonnet` fan at medium: a finding it raised is not a proven defect, and one it did not raise is not proven absent.
- The line is one wording; another wording is not tested, and the line states that it overrides the rubric's text, so it may not carry over to a rubric worded otherwise.
- The fan and the Codex fan differ in more than the engine and the line, as the earlier report lists: model, forks' cap, reading block and tool-use rules.

## Reproducing

`data/harness/setup-codex-home.sh stb1 for1`, then `build.sh STB abfabfcf6 7615d129 161a4a7f` and `build.sh FOR c9fb5eb56 d50e5277 161a4a7f`, then `run-forkfan.sh <STB|FOR> <label> medium` per run, then `evidence.sh <input> <label>`.
The scripts take the clone and the weft repository from the paths at their head, which need this worktree and the hub's `loomyard-weft`.
The scratch tree under `.scratch/`, including the two scratch `CODEX_HOME` directories, was removed after the runs.
