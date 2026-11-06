# Astra with the Code-Review lens fan as an assistant reviewer: one real run on a landed commit

Trial of 2026-11-06, the first run of the assistant fan, `code_assist_reviewer_fan`.
One `lym shuttle run --model "astra[medium]"` from this worktree (`gpt-6-astra`, Codex, medium effort), launched by the writer through the interactive shuttle, no headless launch, no retry.
Nothing in `internal/` or `contracts/` changed.
Part 2, the real composed seat, is still to come and is named at the end.

## Conclusion

The fan works as the Codex assist-fan stencil describes it.
Astra spawned one subagent per lens with `spawn_agent`, waited for all three with `wait_agent` inside its one turn, saved three lens reports, and wrote a consolidated review that `lym burler validate-review` accepts.
The run took 266 s of turn time and returned three `MEDIUM` findings, all of which I checked against the code and found real.

Two details differ from the stencil's text, neither of which broke the run: Codex's wait tool is `wait_agent`, not `wait`, and the three `spawn_agent` calls were issued one after another over 29 s, not in one go.

## Inputs

- Target: landed commit `b93a0a52` (`webster begin-batch: name a delete-before-reference refusal plan_order`), read through `git show`.
- Prompt: `data/prompt.md`, written by hand from the stencils an assistant reads, since no verb composes it outside a round: `burler-template-assist-reviewer`, the Codex `assist-reading` and `edit-directive` blocks, the explore and review steps (`burler-step-1-explore`, `burler-step-review`) with the fork-reading part, `burler-assist-cluster-fan` with the Codex `assist-fan` block, and the `seat-directive-assistant` block.
  The lenses are the `review-code` fan of `internal/lenses/template.yaml`: `code-correctness`, `code-affected-set`, `code-tests`, with their texts verbatim.
  The fasit is the commit message plus `PATTERN.md`; no decision record exists for the target, so the stencil's decision-record lines are kept with a note saying so.
  The rubric is a four-line stand-in, since the real round's rubric lives in the Code-Review profile.
- Output files: the review at `.scratch/fan-trial/review.md` as the run's one `--output-file`; the lens reports sit beside it as the stencil directs.
- Data: `data/` holds the prompt, the review and the three lens reports verbatim.
  The Codex session records read for the numbers below are `~/.codex/sessions/2026/11/06/rollout-2026-11-06T12-53-12-01a1b0b4-*.jsonl` and the three subagent rollouts beside it.

## 1. Did the fan run

| Lens | Subagent started | Returned | Tool calls it reports | Findings |
|---|---|---|---|---|
| `code-correctness` | 11:54:32Z | 11:54:57Z | none | none (18 cases walked, all correct) |
| `code-affected-set` | 11:54:44Z | 11:55:55Z | 3 | 2 `MEDIUM` |
| `code-tests` | 11:55:01Z | 11:55:37Z | 1 | 1 `MEDIUM` |

- A subagent started for each lens and every one returned a final message.
  Each lens report file opens with the spec's `lens:` and `outcome: reported` block and is a plain report, none is `no report`.
- The assistant waited inside its turn.
  The parent record holds one `task_started` and one `task_complete`; three `wait_agent` calls (10 s timeouts) sit between the last spawn and the consolidation, and the report was written after the last subagent returned.
- The subagent prompts are encrypted in the session record, so whether each was exactly the boilerplate and the lens text, with no hint added, cannot be checked from the records.
  The lens reports answer in the form each lens text asks for ("Your list: ..."), which is consistent with the lens text arriving as given.
- `fork_turns: "all"` was set on all three spawns.

## The report as a review

`lym burler validate-review` on `data/review.md` prints `ok`: the frontmatter carries `verdict: APPROVED` and three findings with `origin:` keys, and the body has one `### [MEDIUM]` block per finding.
The review has no `## Rejected` section: no subagent finding was dropped, and the three findings are the lens findings unmerged (`F1`, `F2` from `code-affected-set`, `F3` from `code-tests`).
It adds no `handler` finding of its own, and says so: "The engine's classification and error rendering match the commit's requested behavior on inspection."

## Time and tokens

| Session | Wall time | Fresh input | Cached input | Output | Total |
|---|---|---|---|---|---|
| Handler | 265.8 s (`task_complete`) | 91,095 | 1,377,792 | 7,301 | 1,476,188 |
| `code-correctness` | 25 s | 1,201 | 90,624 | 712 | 92,537 |
| `code-affected-set` | 71 s | 17,719 | 383,872 | 1,542 | 403,133 |
| `code-tests` | 36 s | 5,780 | 184,064 | 992 | 190,836 |

- The handler rollout starts at 11:53:12Z (by its file name); the 265.8 s turn time from `task_complete` is the only timing claim made for the handler.
- Each subagent's total includes the context it inherited, so the four totals do not add up to a cost: fresh input plus output is the comparable figure, about 98,000 for the handler and about 28,000 in all for the lenses.
- For scale, the Claude reviewer of `round-1-usage.yaml` at weft commit `327f541d1d` used 111,975 fresh tokens in 196 s, with three forks at 17,752 fresh in all.
  That round ran different code and a different reviewer, so the two are one data point each, not a measured ratio.

## What it found

All three findings are against code that already holds the fixes of the earlier review, so they are new, and each reads true in the tree:

- `F1`, `MEDIUM`, `contracts/stencils/webster/webster-template-master.md:251`: after the plan fixer returns `fixed`, the stencil says that a `plan_conflict` on the retried `begin-batch` runs the rung again from step 1.
  The stencil now sends `plan_order` straight to stuck at line 242, but the retry line sends any new conflict back to the fixer, so a retry that surfaces `plan_order` goes to a fixer that cannot clear it.
  Confirmed by reading lines 242 and 249-252.
- `F2`, `MEDIUM`, `docs/overview.md:448`: the overview, `internal/webstercli/cli.go:239`, and `internal/webstercli/doc.go:62` still route every begin-batch plan conflict to the cold plan fixer; overview line 645 says Merriam reroutes to plan review only when the fixer could not clear the conflict.
  Confirmed at all four places.
- `F3`, `MEDIUM`, `internal/websterengine/beginbatch_test.go:837`: no test row pins a blocking delete-order finding beside an unrelated informational finding.
  Confirmed in `beginbatch_test.go:836-842`: the only informational row puts the advisory on the delete-order check and a different blocker beside it, so moving `orderOnly` out of the severity guard at `beginbatch.go:500-506` would pass every row.

### Against the earlier review

`round-1-review.md` and the assistant's solo report (`round-1-review.assist-assist.md`) at weft commit `327f541d1d` read the code before its fixes, so this comparison is rough: the same findings could not recur.

| Earlier finding | Earlier source | State in `b93a0a52` |
|---|---|---|
| Master stencil routes `plan_order` through the plan fixer (`MEDIUM`) | Claude lens `code-affected-set` | Fixed at line 242; `F1` here is the retry branch beside it |
| Stencil quotes the old way forward (`MEDIUM`) | Claude lens `code-affected-set` | Fixed; the quote is gone |
| Two planglyph test readers keep the check literal (`LOW`; `BLOCKING` in the solo report) | Solo Astra assistant | Fixed; the commit's stat lists both files |

- Astra's fan found none of the three again, because none remained.
  What it found instead is the next layer of the same two themes: the fixer route still stated unconditionally in the retry line and four documents (`F1`, `F2`), and a test gap in the new table (`F3`).
- The old chair review rejected a code-correctness finding and a code-tests finding as false; Astra's `code-correctness` lens found nothing, and its `code-tests` lens found a real gap the earlier tests lens did not name.
  That comparison is of two different code states, so it does not show that Astra reads better than the Claude fan.
- Severity: the earlier solo report called the literal `BLOCKING`, the chair demoted it to `LOW`.
  Astra's fan returned `MEDIUM` for all three and `APPROVED`; every finding is a stale reader or a missing row, none a wrong behavior, so `MEDIUM` reads right.
- No false finding: all three were checked against the tree.

## Limits

- One run, one commit, one reader of the result.
  A fan that finds three real findings once does not show it does so on a typical diff or twice in a row.
- The prompt was composed by hand from the stencils, not by `composeAssist`; a divergence from the real composed prompt (the rubric, the focus directive, the explore and review steps' markers) would change what Astra reads.
  Part 2 runs the real composition.
- Subagent prompts are encrypted in the session record, so the "no hint added" rule is unchecked.
- The numbers are the Codex records' own: `total_token_usage` of the last `token_count` event of each session, and `task_complete.duration_ms`.

## Findings of the trial for the fan's stencils

No failure, so nothing is filed as a note from this part.
Two wording points for the owners of the Codex `assist-fan` block, neither blocking:

- The block says to wait "with `wait`"; Codex's tool is `wait_agent`, which Astra used without trouble.
- The block says to spawn "all in one go"; Astra issued the three spawns as separate calls over 29 s, and the subagents ran meanwhile, so the cost of that is wall time only.

## Part 2: the real seat

Still to come.
This task's `run_settings` turn on Code-Review with the assistant fanned, so the review of this report is the real composed seat on a small diff.
The writer did nothing for this part.
