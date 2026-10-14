# PATTERN-gate-self-check-parity

A mechanical gate's **closure** and its CLI self-check verb call the same package function for every mode.

## Pairs

- Discussion-Write's and Discussion-Burler-Fix's gates and `validate-discussion`: `discussionparser.Validate`.
- The Darn-Write row's `discussion` gate and `validate-discussion` on a darn run: `loomshed.ValidateDarnDiscussion`.
- Plan-Burler-Fix's gate, a Plan-Write run's gate whose record carries no fork authorization and `validate-plan`: `loomshed.ValidatePlan` (`planindex.Index.ValidateFormat`, implemented by `planglyph`, plus the done-card checks).
- A twill-spawned Plan-Write run's gate and `validate-plan --lens-review`: `loomshed.ValidatePlanLensReview`, which adds the presence of `lens-review.md`, the run's recorded fork authorization selecting it.
- PR-Rework's gate and `validate-plan --rework`: `loomshed.ValidateReworkPlan`, which checks the whole new plan plus the told `first_card`.
- `validate-plan --overlay`, a self-check mode no gate runs: it calls the flag-less mode's function, `loomshed.ValidatePlan`, over a plan parsed through a `planparser.Overlay`.
- Describe's gate and `validate-description`: `summaryparser.ValidateDescription`.
- The webster `verify` gate (`websterengine.NewVerifyGate`) and `lyx webster verify`: `verifytree.Verify`.
  Webster-Burler-Fix's `verify` gate (`loomshed.NewVerifyGate`) runs the comment lint and the derived round command through `verifytree.Verify`.
- Every card gate's comment lint and `lyx loom lint-comments`: `commentlint.Lint`.
  The Webster-Burler-Fix round gate (`loomshed.NewVerifyGate`) joins this pair when it runs the lint.
- Every card gate's headroom lint and `lyx loom lint-pattern`: `patternlint.Lint` over `pattern.Headroom`.
  The Webster-Burler-Fix round gate (`loomshed.NewVerifyGate`) is the pair's third caller when it is told a merge base.
- Every burler round's `review` gate and `lyx burler validate-review`: `burlerengine.CheckReviewFile`, over `ParseReview`.

## Rules

- Adding a mechanical gate means adding its verb and its parity check in the same task.
- The verb's `--require-approved` mode, running the full check set, has no recipe counterpart by design.
  Both plan gate sites run strictly before the Plan-Review segment's approve seam writes the approval flag, so demanding it would fail every fix round.
  The flag's guarantee rests on that seam failing loudly, never on a row re-checking it.
- Moving a gate from a standalone row into a producer's own closure changed *where* the call sits, never the property this entry binds, which is why the entry survives the move.
