# PATTERN-gate-self-check-parity

A mechanical gate's **closure** and its CLI self-check verb call the same package function for every mode.

## Pairs

- Discussion-Write's and Discussion-Burler's gates and `validate-discussion`: `discussionparser.Validate`.
- Plan-Write's and Plan-Burler's gates and `validate-plan`: `planglyph.ValidateFormat`.
- PR-Rework's gate and `validate-plan --rework`: `loomshed.ValidateReworkPlan`, which checks the whole new plan plus the told `first_card`.
- Describe's gate and `validate-description`: `summaryparser.ValidateDescription`.
- The webster `verify` gate (`websterengine.NewVerifyGate`) and Webster-Burler's `verify` gate (`loomshed.NewVerifyGate`) and `lyx webster verify`: `verifytree.Verify`.

## Rules

- Adding a mechanical gate means adding its verb and its parity check in the same task.
- The verb's `--require-approved` mode, running the full check set, has no recipe counterpart by design.
  Both plan gate sites run strictly before the Plan-Review segment's approve seam writes the approval flag, so demanding it would fail every fix round.
  The flag's guarantee rests on that seam failing loudly, never on a row re-checking it.
- Moving a gate from a standalone row into a producer's own closure changed *where* the call sits, never the property this entry binds, which is why the entry survives the move.
