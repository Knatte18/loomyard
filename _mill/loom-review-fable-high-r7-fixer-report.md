# loom review — round 7 (fable-high-r7) — fixer report

Branch `crucible-loom-glyph-hardening`, no pushes. One commit per finding, message naming the finding. Table rows written as each fix lands, per the review prompt's commit-per-fix rule.

## Implemented

| ID | Severity | What landed | Test that would have caught it | Commit |
|----|----------|-------------|--------------------------------|--------|
| F1 | BLOCKING | Gate evidence in `claudeengine.Startup`/`TrustDismissSequence` now carries an adjacency requirement, applied through one shared rule (`acceptLineIsGateOption`): an accepting-option line counts only adjacent to the last caret (≤1 line — both live-transcribed gates show 0/1), the footer only strictly below it (≤4 lines — every live gate draws it 3 under), and a caret-less capture keeps its gate classification (dismissal presses nothing, startup window bounds the wait). A prose list item beginning with an accept phrase, or prose saying "press Enter to confirm", no longer classifies a healthy pane as a gate — and `TrustDismissSequence` refuses to walk a caret that is not adjacent to the accept line, so no keys are ever pressed into a live pane's input box. | `TestStartup_Classification/ready_agent_prose_list_item_starting_with_the_accept_label`, `.../ready_agent_prose_with_footer_phrase_and_needle_mention`, `TestTrustDismissSequence/prose accept line far from the input-box caret presses nothing at all` (`internal/shuttleengine/claudeengine/startup_test.go`) | `963ba557c` |
| F2 | MEDIUM | A told `--stencils-dir` is stat'd (exists + is a directory) at the wiring boundary in BOTH modes and BOTH CLIs, through one new shared `cliwire.Module.RefuseUnreadableStencilsDir` method called from `ResolveStandalone`'s override branch and from both hub wirings — so a typo'd override refuses before the run lock, quarry resolve, or (standalone) reed's tmux boot, instead of at the first prompt render. The derived standalone default stays exempt (it is seeded where derived). Live re-verified: the S7 scenario now refuses with the new message; a valid override still works. | `TestResolveStandalone_Stencils/AbsentToldStencilsDirIsRefused` + `/FileToldStencilsDirIsRefused` (`internal/cliwire/cliwire_test.go`), `TestWireHub_AbsentStencilsDirIsRefused` (both `internal/webstercli/wiring_test.go` and `internal/burlercli/wiring_test.go`) | `9e20c46f3` |

| F3 | LOW | `TestDeriveCallerSet_CliwireOnly` now walks the WHOLE repository (skipping `.git` and `testdata`) instead of only `internal/` and `cmd/`, so a production `standalonestate.Derive` caller under `tools/` or a future top-level directory cannot slip the pin — and a dot-import of standalonestate in a production file is refused outright, since it hides every `Derive` call from the selector match. | The enforcement test IS the guard; its widened scan ran green against the whole current tree (`internal/cliwire/callerset_enforcement_test.go`) | `a9c706a93` |
| F4 | NIT | `bannedWiringDeclarations` also bans cliwire's own current helper spellings (exported and unexported), not just the nine historical webstercli/burlercli names — re-declaring a helper under the name cliwire gives it was the most natural way to copy one back out. The fresh-name residual is now stated in the list's own doc, pointing at the Derive pin as the complementary guard. | The enforcement test IS the guard (`internal/cliwire/bannedecl_enforcement_test.go`) | `b9c77ef27` |
| D1 | LOW | New `internal/logger/sink_callsite_integration_test.go` (integration-tagged, external test package, own `TestMain` with `gitkit.HermeticGitEnv`) drives R6-6's fix at its real call site: `logger.Info` with no sink override, from inside a real plain git checkout (asserts NO `.lyx` is created) and from inside a lyx-owned worktree with `_lyx` present (asserts the trace file lands under `.lyx/logs`). | `TestDurableSink_CwdFallbackNeverArmsInAPlainCheckout`, `TestDurableSink_CwdFallbackArmsInALyxOwnedWorktree` | `64b7b452d` |
| D2 | LOW | `TestPlanFindingsHaveBlocking_UnrecognizedSeverityFailsClosed` added to `internal/loomcli/validate_test.go`, the exact four-case shape of loomshed's half, so both halves of the R6-27 parity pair are now pinned independently. | the new test itself | `6c218891a` |

## Deferred / not fixed this round

None — all 6 findings (F1–F4, D1, D2) fixed and committed.

## Changed files

- `internal/shuttleengine/claudeengine/startup.go`, `startup_test.go`, `doc.go` (F1)
- `internal/cliwire/module.go`, `standalone.go`, `cliwire_test.go` (F2); `callerset_enforcement_test.go` (F3); `bannedecl_enforcement_test.go` (F4)
- `internal/webstercli/wiring.go`, `wiring_test.go` (F2)
- `internal/burlercli/wiring.go`, `wiring_test.go` (F2)
- `internal/logger/sink_callsite_integration_test.go` (D1, new file)
- `internal/loomcli/validate_test.go` (D2)
