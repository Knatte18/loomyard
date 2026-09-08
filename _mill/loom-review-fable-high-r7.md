# loom review — round 7 (fable-high-r7)

Reviewer-fixer: Fable 5, high effort. Clean-room pass per `_mill/loom-review-prompt.md` (round 7).
Scope: whole loom/glyph surface + `#004` standalone-webster material + FIRST review of the `unify-webster-burler-wiring` merge (`7e54cc280`, `internal/cliwire`).

Status: JOB 1 IN PROGRESS — findings below are provisional until the "Findings" section is marked final.

## Executive summary

(to be written at the end of Job 1)

## What was tested

- `git rev-parse --show-toplevel` → `/home/knatte/Code/loomyard/wts/crucible-loom-glyph-hardening`; branch `crucible-loom-glyph-hardening`; HEAD `4df538ef6` (re-seed) atop `7e54cc280` (cliwire merge).
- Read in full, clean-room (no prior `loom-review-*` file opened): `CONSTRAINTS.md`, `internal/cliwire/{doc,module,paths,standalone}.go`, `internal/cliwire/{bannedecl,callerset}_enforcement_test.go`, `internal/webstercli/{wiring,cli,run}.go`, `internal/burlercli/{wiring,cli,run}.go`, `internal/shuttleengine/claudeengine/startup.go`, `internal/shuttleengine/{wait,engine,run}.go`.

## Provisional findings log (running, unfinalized)

- [P1] `claudeengine/startup.go` — R6-1's "positive evidence" rule has its own general-case gap: a single agent-prose line that begins (after list decoration) with an accept-needle phrase (e.g. a markdown bullet "- Yes, I accept …") supplies BOTH the startup-gate needle and the "accepting-option line" evidence, so a healthy pane is classified StartupTrustPrompt; the pane's own ready marker "❯" (the input box) then serves as the caret line and TrustDismissSequence walks arrow keys + Enter into the live pane. Same class for the footer path: prose "…press Enter to confirm…" plus any gate-needle mention classifies a gate (no keys pressed, but the healthy run is killed at the startup deadline, and Send/Interrupt are refused). A REAL gate's caret sits on one of two adjacent option lines with the footer immediately beneath — no proximity requirement exists between the located lines. Candidate fix: shared locator gains an adjacency requirement (accept/footer evidence counts only when the last caret line is within a few lines of it; TrustDismissSequence refuses to walk a caret further than that), keeping Startup and the dismissal agreeing by construction. Provisional severity: BLOCKING. CONFIRMED (unit-testable from code).
- [P2] `internal/cliwire/standalone.go` (`ResolveStandalone`) + both hub wirings — `--stencils-dir` is the one told-directory flag with NO existence/directory check at the wiring boundary (`--target-dir` is stat'd per R6-7, `--plan-dir` content-checked). A typo'd `--stencils-dir` is honoured silently, and the failure surfaces only at first stencil read — for `run`, after the standalone reed session (tmux server) has already been booted, exactly the "refusal after substrate boot" cost module.go's own doc argues must not happen. Not silent-wrong-target (loud, late) so lower than R6-7. Provisional severity: MEDIUM. CONFIRMED.
- [P3] `internal/cliwire/callerset_enforcement_test.go` — scans only `internal/` and `cmd/`; a production caller of `standalonestate.Derive` under `tools/` (or a new top-level dir) slips the pin. Also a dot-import (`import . "...standalonestate"`) calls `Derive` as a bare ident and the selector-based matcher misses it. Both far-fetched but the invariant text says "the only production caller". Provisional severity: LOW/NIT. CONFIRMED (by construction of the scanner).
- [P4] `internal/cliwire/bannedecl_enforcement_test.go` — ban is a fixed name list; a re-implementation under fresh names passes. Inherent to the approach and partially compensated by the Derive pin, but the name list also misses e.g. a re-declared `planDirHasContent` (only `standalonePlanDirHasContent` is banned). Provisional severity: NIT.
