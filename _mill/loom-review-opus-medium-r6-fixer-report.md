# `loom` — round 6 (opus-medium-r6) fixer report

Companion to [loom-review-opus-medium-r6.md](loom-review-opus-medium-r6.md). Every row is written as its fix lands
green and is committed, never reconstructed at the end.

Branch `crucible-loom-glyph-hardening`, no pushes. One commit per finding, message `loom: fix R6-N — <what/why>`.

## Implemented

| ID | Severity | What changed | Test that would have caught it | Commit |
|---|---|---|---|---|
| R6-1 | BLOCKING | `Startup` no longer calls a gate on a phrase alone: it requires positive evidence a dialog is rendered — an accepting-option LINE (matched by what the line BEGINS with once caret/numbering are stripped) or claude's `Enter to confirm` gate footer — and locates that line with `locateGateLines`, now shared with `TrustDismissSequence` so classifier and dismissal can never disagree. | `TestTrustDismissSequence_PressesNothingIntoALiveAgentsPane` plus four `TestStartup_Classification` prose/footer cases (`internal/shuttleengine/claudeengine/startup_test.go`) | see git log |
| R6-3 | BLOCKING | `resolveContainment` now indexes cards by `writingTargetCards` (`c.Targets` alone, which already carries both `Pairs` endpoints) instead of `targetCards` (Targets+Uses+Pairs), so read-only `Uses:` refs no longer emit a blocking containment finding. `targetCards` is unchanged for `statusFindings`/`DetectDrift`. | `TestResolveContainment_ReadOnlyRefsAreNotAContainmentHazard` — sabotage-proofed: fails on the pre-fix index, passes on the fix | see git log |
| R6-4 | MEDIUM | `persistPlanFingerprintRebaseline` now re-loads state under the still-held lease and persists ONLY `PlanFingerprint`, instead of saving the caller's whole in-memory `*State` — which on the record-batch path also persisted `SeenForkTranscripts`, marking a fork's transcript consumed on a call that failed. | `TestPersistPlanFingerprintRebaseline/only_the_fingerprint_is_persisted,_never_the_caller's_other_mutations` (`internal/webstercli/verbs_test.go`) | see git log |
| R6-5 | MEDIUM | `normalizeCardPath` returns an empty or single-`/`-prefixed entry cleaned-but-unjoined, so the malformed marker survives to `card-path-malformed` under a non-empty `root:` instead of being collapsed into a clean relative path (or into the root directory itself). | `TestNormalizeCardPath` gains `malformed_single-/_prefix_survives_a_set_root`, `empty_entry_survives_a_set_root`, and a past-the-root `..` case (`internal/planparser/normalize_test.go`) | see git log |
| R6-6 | MEDIUM | `armDurableSinkLocked`'s cwd-anchored fallback now arms only when the resolved location is a worktree lyx owns (`isLyxWorktree`: `_lyx` present at the anchor), so a standalone refusal — or any non-zero exit inside a plain checkout — no longer creates `<repo>/.lyx/logs`. Recorded as a bullet under CONSTRAINTS.md's Durable-vs-Ephemeral State Invariant. | `TestIsLyxWorktree_GatesTheCwdAnchoredFallback` (`internal/logger/sink_test.go`) | see git log |
| R6-2 | MEDIUM | `gateAcceptNeedles` gains `yes,proceed`, the older trust-gate wording `Startup`'s own fixture set already treated as a recognized gate but `TrustDismissSequence` could never act on. | `TestTrustDismissSequence/older_Yes,_proceed_trust-gate_wording_is_dismissable` (`internal/shuttleengine/claudeengine/startup_test.go`) | see git log |

## Deferred / not fixed this round

See the review report's "Recorded but NOT fixed this round" section: the ~45-item residue from the sweep over
loom's pre-glyph pipeline machinery, which this round's prompt declares out of scope.
