# `loom` — round 6 (opus-medium-r6) fixer report

Companion to [loom-review-opus-medium-r6.md](loom-review-opus-medium-r6.md). Every row is written as its fix lands
green and is committed, never reconstructed at the end.

Branch `crucible-loom-glyph-hardening`, no pushes. One commit per finding, message `loom: fix R6-N — <what/why>`.

## Implemented

| ID | Severity | What changed | Test that would have caught it | Commit |
|---|---|---|---|---|
| R6-1 | BLOCKING | `Startup` no longer calls a gate on a phrase alone: it requires positive evidence a dialog is rendered — an accepting-option LINE (matched by what the line BEGINS with once caret/numbering are stripped) or claude's `Enter to confirm` gate footer — and locates that line with `locateGateLines`, now shared with `TrustDismissSequence` so classifier and dismissal can never disagree. | `TestTrustDismissSequence_PressesNothingIntoALiveAgentsPane` plus four `TestStartup_Classification` prose/footer cases (`internal/shuttleengine/claudeengine/startup_test.go`) | see git log |
| R6-3 | BLOCKING | `resolveContainment` now indexes cards by `writingTargetCards` (`c.Targets` alone, which already carries both `Pairs` endpoints) instead of `targetCards` (Targets+Uses+Pairs), so read-only `Uses:` refs no longer emit a blocking containment finding. `targetCards` is unchanged for `statusFindings`/`DetectDrift`. | `TestResolveContainment_ReadOnlyRefsAreNotAContainmentHazard` — sabotage-proofed: fails on the pre-fix index, passes on the fix | see git log |
| R6-2 | MEDIUM | `gateAcceptNeedles` gains `yes,proceed`, the older trust-gate wording `Startup`'s own fixture set already treated as a recognized gate but `TrustDismissSequence` could never act on. | `TestTrustDismissSequence/older_Yes,_proceed_trust-gate_wording_is_dismissable` (`internal/shuttleengine/claudeengine/startup_test.go`) | see git log |

## Deferred / not fixed this round

See the review report's "Recorded but NOT fixed this round" section: the ~45-item residue from the sweep over
loom's pre-glyph pipeline machinery, which this round's prompt declares out of scope.
