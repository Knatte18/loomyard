# loom review — round 7 (fable-high-r7)

Reviewer-fixer: Fable 5, high effort. Clean-room pass per `_mill/loom-review-prompt.md` (round 7).
Scope: whole loom/glyph surface + `#004` standalone-webster material + FIRST review of the `unify-webster-burler-wiring` merge (`7e54cc280`, `internal/cliwire`).

Status: JOB 1 COMPLETE — independent findings final below; prior-round material consulted only AFTER the list was closed.

## Executive summary

Seventh round, second Fable deployment. The independent pass found **1 BLOCKING, 1 MEDIUM, 1 LOW, 1 NIT** new findings, plus the two seeded coverage gaps (D1, D2) confirmed real and in scope to close.

- The `unify-webster-burler-wiring` merge (`internal/cliwire`, first-ever review) is **sound**: all six round-6 guarantees (R6-7/8/9/15/16/17) verified preserved for BOTH callers by code reading, by the hermetic suites, and by live driving of the standalone refusal scenarios (S1–S9 below). The two enforcement tests have real but bounded blind spots (F3, F4). One genuine behavioral gap found: `--stencils-dir` is the only told-directory flag with no existence check at the wiring boundary (F2).
- The provider-startup seam (fourth consecutive round producing a finding here): round 6's "positive evidence" rule has its own general-case blind spot (F1) — the evidence lines are never required to sit anywhere near each other, so one line of ordinary agent prose can still classify a healthy pane as a gate and get keys pressed into it. Same defect class as R6-1, surviving through a narrower window.
- Everything else read (runlevel, bracket verbs, fingerprint, drift, containment, plan/discussion validators, logger sink) held up; no R6-28-shaped `bufio.Scanner` sibling exists anywhere in production.

**Merge-readiness / convergence verdict: see end of report** (written after fixes).

## What was tested

- `git rev-parse --show-toplevel` → `/home/knatte/Code/loomyard/wts/crucible-loom-glyph-hardening`; branch `crucible-loom-glyph-hardening`; HEAD `4df538ef6` (re-seed) atop `7e54cc280` (cliwire merge).
- Read in full, clean-room (no prior `loom-review-*` file opened before findings were final): `CONSTRAINTS.md`, `internal/cliwire/**` (all 7 files incl. both enforcement tests and `cliwire_test.go`), `internal/webstercli/{wiring,cli,run}.go`, `internal/burlercli/{wiring,cli,run}.go`, `internal/shuttleengine/claudeengine/startup.go`, `internal/shuttleengine/{wait,engine,run}.go`, `internal/logger/sink.go`, `internal/websterengine/{fingerprint,beginbatch,recordbatch,runlevel}.go`, `internal/planglyph/{containment,drift}.go`, `internal/loomshed/planvalidate.go`, `internal/loomcli/validate.go`, R6-28's fix commit (`345218ca5`, discussionparser).
- `CGO_ENABLED=1 go build ./...` → EXIT 0.
- `CGO_ENABLED=1 go vet <full review-prompt package set>` → EXIT 0.
- `CGO_ENABLED=1 go test ./...` (whole repo) → EXIT 0, 82 packages ok (`scratchpad/gotest-all.log`).
- `grep bufio.NewScanner` production-wide → 3 sites, all check `scanner.Err()` (no R6-28 sibling).
- Live driving (fresh `go build -o <scratch>/lyx ./cmd/lyx` at HEAD; no tmux/claude spawned anywhere — every scenario is a wiring-boundary refusal; teardown check ran, zero stray substrate processes):
  - S1/S2: standalone `webster validate --target-dir ./nope` and `--target-dir <file>` → both refused with the R6-7 message (resolved path named). ✓
  - S3: `burler run --target-dir ./nope` → same refusal, burler wording. ✓ (both callers through one shared path)
  - S4: `XDG_STATE_HOME` pointed through a symlink INSIDE the target → nested-geometry refusal fires (R6-15 preserved through `NormalizeForContainment`). ✓
  - S5: standalone `webster run --plan-dir <real plan elsewhere>` → run refuses before any substrate boot, recourse names the default (R6-8/F-A3). ✓
  - S6: `webster status --plan-dir <override>` → honored (read-only verbs keep the override). ✓
  - S7: `webster status --stencils-dir <nonexistent>` → **accepted silently** (F2 confirmation). ✗→F2
  - S8/S9: missing-plan refusal flagless and with an override → verb-aware recourse text, override case names the DEFAULT as `run`'s recourse (R6-9). ✓
- Hub-mode halves of R6-7 (`RefuseTargetDirInHubMode`) and R6-8 (override recorded off `geom.PlanDir`, `run` refusal is mode-agnostic) verified by code reading + the packages' own hermetic tests (`TestWire_TargetDirRefusedInHubMode`, `TestWire_PlanDirResolution` in both CLI packages); not driven live — no sandbox hub was built this round (see limits).

## Findings (final, severity-ranked)

### F1 — BLOCKING — startup gate classification still trips on ordinary prose; keys pressed into a healthy pane (CONFIRMED)

`internal/shuttleengine/claudeengine/startup.go:62-111,179-190` (`Startup`, `gateIsRendered`, `isGateAcceptOptionLine`, `locateGateLines`, `TrustDismissSequence`).

Round 6's R6-1 fix requires "positive evidence a dialog is rendered": a gate needle PLUS either a locatable accepting-option line or claude's gate footer. The blind spot: **none of the evidence lines are required to sit anywhere near each other or near the caret.** A real rendered gate is a two-option select list — its caret sits ON one of two ADJACENT option lines, footer immediately beneath — but the code accepts an accept-line anywhere in the capture paired with a caret anywhere else.

Concrete failure scenarios, both constructible from one line of ordinary agent output:

1. A transcript line that is a markdown list item beginning with an accept phrase — `- Yes, I accept the risk of X` or `> Yes, I accept …` (`-`, `>`, digits, `.`, `)` are all in `gateOptionDecoration`, so the prefix survives stripping). That single line supplies BOTH the startup-gate needle (`yes,iaccept` in the normalized whole-capture) AND the accepting-option-line evidence. `Startup` → `StartupTrustPrompt`. During the startup window, `Wait` then plays `TrustDismissSequence`: the caret line is the healthy pane's own input-box `❯` (the READY marker!) at the bottom, the accept line is the prose line far above → dozens of Up presses (history recall in claude's input box) + Enter (submits whatever got recalled) into a live pane, `*started` never set, run killed `OutcomeDied` at the startup deadline. Mid-run, `requireReadyAgentPane` refuses every Send/Interrupt while the line stays on screen.
2. The footer path: prose containing "…press Enter to confirm…" (`entertoconfirm` after normalization) plus any gate-needle mention anywhere → gate classified; no keys pressed (no accept line) but the healthy run is still killed at the startup deadline and Send/Interrupt refused.

This is the R6-1 defect class surviving through a narrower window — and this codebase's own domain makes the trigger realistic: loom drives agents whose prompts and transcripts *discuss these exact gates* (the campaign's own review prompts contain `yes,iaccept` verbatim), and the mill conversation convention has agents write numbered option lists whose first entry is the recommended "Yes, …" option.

Fix: gate evidence gains an adjacency requirement, in ONE shared helper so `Startup` and `TrustDismissSequence` cannot disagree (the same can-never-disagree principle R6-1 itself established): an accept line (or footer line) counts as rendered-gate evidence only when no caret exists yet (booting pane — dismissal presses nothing, bounded by the startup window) or the last caret line is within a few lines of it; `TrustDismissSequence` refuses to walk a caret that is not adjacent to the accept line. A real gate always satisfies adjacency; a healthy pane's input-box caret and a prose line never do.

### F2 — MEDIUM — `--stencils-dir` is the one told-directory flag never checked at the wiring boundary (CONFIRMED, live S7)

`internal/cliwire/standalone.go:84` (override branch of `ResolveStandalone`) and both hub wirings (`internal/webstercli/wiring.go:138`, `internal/burlercli/wiring.go:116`).

`--target-dir` is stat'd and refused (R6-7); `--plan-dir` is content-checked; a typo'd `--stencils-dir` is honoured silently in both modes and both CLIs (live S7: `ok:true`). The failure surfaces only at the first stencil read — for `webster run`/`burler run` that is AFTER the run lock is taken, the plan re-resolved through quarry, and (standalone) the private reed session's tmux server booted. `cliwire/module.go`'s own doc states the principle this violates: refuse "before any substrate is booted, which is the only point at which a refusal costs nothing to recover from". Not silent-wrong-target (the eventual error is loud and names the path), hence MEDIUM, not BLOCKING.

Fix: stat the told stencils directory at the wiring boundary — in `ResolveStandalone`'s override branch and in a shared `Module` helper both hub wirings call — refusing a missing/non-directory value with a message naming the raw flag and the resolved path, mirroring `resolveStandaloneTarget`'s wording. The derived default is exempt (it is seeded right there).

### F3 — LOW — `callerset_enforcement_test.go`'s Derive pin has two structural escape hatches (CONFIRMED by construction)

`internal/cliwire/callerset_enforcement_test.go:54` walks only `internal/` and `cmd/` — a production caller of `standalonestate.Derive` under `tools/` (dev tooling that ships in-repo and DOES construct paths) or any future top-level Go dir slips the pin silently. And `callsDerive` matches only selector expressions, so a dot-import (`import . "…/standalonestate"` + bare `Derive(…)`) passes undetected. Fix: walk every top-level dir containing Go packages (skip testdata), and flag a dot-import of standalonestate outright as a violation (there is no legitimate production dot-import of it).

### F4 — NIT — `bannedecl_enforcement_test.go`'s name list has known gaps (CONFIRMED by construction)

`internal/cliwire/bannedecl_enforcement_test.go:30` — the ban is a fixed nine-name list, trivially defeated by a rename (inherent to the approach, partially compensated by the Derive pin — accepted), but the list also omits `planDirHasContent` (only the old `standalonePlanDirHasContent` spelling is banned), so re-declaring the helper under cliwire's own current name passes. Fix: add the current cliwire helper names to the banned set and state the rename residual in the file doc.

### D1 — LOW (seeded coverage gap) — R6-6's regression test never exercises its call site (CONFIRMED)

`internal/logger/sink_test.go:456` `TestIsLyxWorktree_GatesTheCwdAnchoredFallback` drives the `isLyxWorktree` helper directly; nothing drives `ensureDurableSink`'s cwd-anchored fallback end-to-end against a plain git repo to prove no `.lyx` is created. Fix: add a test that chdirs into a fresh plain git worktree-root fixture, arms the sink with no override, and asserts no sink file/dir appears — and the mirror case with `_lyx` present asserting it does.

### D2 — LOW (seeded coverage gap) — `loomcli.planFindingsHaveBlocking` has no dedicated fail-closed test (CONFIRMED)

Only `internal/loomshed/planvalidate_test.go` pins the fail-closed rule; `internal/loomcli`'s twin is verified only by the two bodies being identical today. Fix: `TestPlanFindingsHaveBlocking_UnrecognizedSeverityFailsClosed` in `internal/loomcli`, same three-case shape as loomshed's.

## Explicitly hunted, found sound (the "tried to find nothing" record)

- All six cliwire-consolidation guarantees, both callers, live + hermetic (see What was tested).
- `NormalizeForContainment`'s deepest-existing-ancestor walk: broken-symlink ancestor, root-terminating walk, missing leaf — all correct; `pathContains` matches shuttleengine's own rule.
- `RepositoryRootOf`: nearest-`.git` wins, `.git` FILE (linked worktree) counted, no-repo returns unchanged — all pinned by tests, re-verified.
- `ResolveStandalone` ordering obligation (derive → nested guard → sink redirect → stencils → plan) intact and pinned by `TestResolveStandalone_SinkRedirectOrdering`.
- The three surviving production `bufio.NewScanner` sites all check `Err()` — no R6-28 sibling.
- `writingTargetCards` (R6-3) correctly indexes `Targets` only; dedupe-by-card-id correct.
- Both halves of the R6-27 parity pair fail closed; `runlevel.go`'s own `hasBlockingFinding` (equals-blocking over `ValidateDispatch` output that stamps severities itself) is consistent.
- `restampFingerprint` call sites (begin-batch, record-batch ×2, run-level) each sit immediately after their rewriting call, ahead of every refusal; no masking of the primary error.
- `sweepOrphansOpportunistic` absent/unreadable reed-state skip, `errStrandNotTracked`/`errStrandPaneBindingCleared` mechanism-failure split, attached-run startup-probe skip — all re-read, no gaps found.
- `DetectDrift`'s two gates, evidence-tier exclusion from the deleted sweep, and post-repair scoped revalidation (R6-11) — re-read, no gaps found.
