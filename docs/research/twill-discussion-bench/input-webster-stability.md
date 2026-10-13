Twill Discussion bench input, 2026-10-11: the `webster-stability` board body (Bolt `4a0db933`) as the first round's note body.

## Goal

Close the Webster faults that stop or mislead runs outside the audit: background gate shells, Master fixing verify failures itself, recovery attribution and gates, and Create checks that cannot pass for unexported or build-tagged members.

## 1. A background gate shell is cut off at a turn end (#505, #509, #517)

Master's wait counted a turn end while a background `lyx gate test --tags integration` shell was still running; after `background_shell_wait_min` the wait stopped, the run finished `done` because Master's output files existed, and shuttle removing the strand killed the shell.
A run can report `done` with a gate tier unfinished.
Expected: a running gate shell holds the turn end, or lyx stops it explicitly and records in the run that its result was discarded, so the outcome never reads as "tests passed".

## 2. Master fixes a verify failure itself

In `driver-context` (2026-10-10) Master (`sonnet[medium]`) edited and committed a failing assertion itself on Publish verify attempt 3, although its stencil sends verify failures to one fixer fork; the run-exit audit blocked it as a parent-write and the fix had to be reverted, `accept-audit`ed and redone through a fork.
Expected: the rule is stated where Master reads a verify failure, not only in its rules list, or Master has a verb that spawns the verify fixer, so the right path is the easy one.

## 3. A second recovery treats the first recovery's files as foreign (#522)

In `quick-recipe` (2026-10-10) batch 01's first recovery edited 19 files for card 1 and was stopped uncommitted when Webster restarted; the second recovery's prompt listed them under "Not written by this run: leave it untouched", and the strand blocked.
Expected: files an earlier recovery of the same batch wrote count as the batch's own work (attribution by batch, not by shuttle run id), or the second recovery starts from a clean tree with the first attempt's diff offered as reference.
Also: `recover-batch 1` exited 0 while the batch ended failed and terminal; it should exit non-zero.

## 4. The recovery prompt does not name its allowed gates

The same recovery was unsure whether `lyx gate test` and `lyx loom lint-comments` fell under the ban on fabric commands, and could only compile-check tmux-tagged card steps.
Expected: the recovery prompt names the card and batch gates as always allowed, and says tmux-tagged steps are left to verify.

## 5. Create check fails on unexported targets (#511)

`record-batch` refused two batches with `create-not-done` for unexported targets (`plan:cmd/testtiming#rusageStats`, `plan:internal/testkit/tmuxkit#tmuxAllowed`); recovery failed the same way and the batches died.
Expected: find whether the check resolves unexported identifiers and identifiers in test files; fix it if not, and make the refusal say how the target was looked up, so a missing symbol is told apart from a resolver limit.

## 6. Build-tagged and per-GOOS members resolve ambiguous (#500, #507)

A member declared once per mutually exclusive build-constraint set (`tmuxAllowed` in `allow_tmux.go` and `allow_none.go`), or once per GOOS (`StartTime` in `internal/proc`, `runChild` in `internal/gatecli`), resolves `ambiguous` in quarry, and DoneChecks accept only `found` or `multipart`, so its Create or Edit can never pass.
Expected: DoneChecks accept an ambiguous answer whose candidates all sit in files with disjoint build constraints (or quarry honours build constraints), and the plan validator flags such a glyph at Plan time rather than at record-batch.

