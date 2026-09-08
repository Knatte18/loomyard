# loom review — round 7 (fable-high-r7) — fixer report

Branch `crucible-loom-glyph-hardening`, no pushes. One commit per finding, message naming the finding. Table rows written as each fix lands, per the review prompt's commit-per-fix rule.

## Implemented

| ID | Severity | What landed | Test that would have caught it | Commit |
|----|----------|-------------|--------------------------------|--------|
| F1 | BLOCKING | Gate evidence in `claudeengine.Startup`/`TrustDismissSequence` now carries an adjacency requirement, applied through one shared rule (`acceptLineIsGateOption`): an accepting-option line counts only adjacent to the last caret (≤1 line — both live-transcribed gates show 0/1), the footer only strictly below it (≤4 lines — every live gate draws it 3 under), and a caret-less capture keeps its gate classification (dismissal presses nothing, startup window bounds the wait). A prose list item beginning with an accept phrase, or prose saying "press Enter to confirm", no longer classifies a healthy pane as a gate — and `TrustDismissSequence` refuses to walk a caret that is not adjacent to the accept line, so no keys are ever pressed into a live pane's input box. | `TestStartup_Classification/ready_agent_prose_list_item_starting_with_the_accept_label`, `.../ready_agent_prose_with_footer_phrase_and_needle_mention`, `TestTrustDismissSequence/prose accept line far from the input-box caret presses nothing at all` (`internal/shuttleengine/claudeengine/startup_test.go`) | `963ba557c` |
| F2 | MEDIUM | A told `--stencils-dir` is stat'd (exists + is a directory) at the wiring boundary in BOTH modes and BOTH CLIs, through one new shared `cliwire.Module.RefuseUnreadableStencilsDir` method called from `ResolveStandalone`'s override branch and from both hub wirings — so a typo'd override refuses before the run lock, quarry resolve, or (standalone) reed's tmux boot, instead of at the first prompt render. The derived standalone default stays exempt (it is seeded where derived). Live re-verified: the S7 scenario now refuses with the new message; a valid override still works. | `TestResolveStandalone_Stencils/AbsentToldStencilsDirIsRefused` + `/FileToldStencilsDirIsRefused` (`internal/cliwire/cliwire_test.go`), `TestWireHub_AbsentStencilsDirIsRefused` (both `internal/webstercli/wiring_test.go` and `internal/burlercli/wiring_test.go`) | `9e20c46f3` |

## Deferred / not fixed this round

(none so far)
