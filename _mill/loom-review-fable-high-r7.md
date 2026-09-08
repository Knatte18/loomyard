# loom review — round 7 (fable-high-r7)

Reviewer-fixer: Fable 5, high effort. Clean-room pass per `_mill/loom-review-prompt.md` (round 7).
Scope: whole loom/glyph surface + `#004` standalone-webster material + FIRST review of the `unify-webster-burler-wiring` merge (`7e54cc280`, `internal/cliwire`).

Status: ROUND COMPLETE — Job 1 findings final below (formed clean-room; prior-round material consulted only after the list was closed and committed at `d4dcedbc1`); Job 2 fixed all 6 findings, one commit each (`963ba557c`, `9e20c46f3`, `a9c706a93`, `b9c77ef27`, `64b7b452d`, `6c218891a`). Fixer report: `_mill/loom-review-fable-high-r7-fixer-report.md`.

## Executive summary

Seventh round, second Fable deployment. The independent pass found **1 BLOCKING, 1 MEDIUM, 1 LOW, 1 NIT** new findings, plus the two seeded coverage gaps (D1, D2) confirmed real and in scope to close.

- The `unify-webster-burler-wiring` merge (`internal/cliwire`, first-ever review) is **sound**: all six round-6 guarantees (R6-7/8/9/15/16/17) verified preserved for BOTH callers by code reading, by the hermetic suites, and by live driving of the standalone refusal scenarios (S1–S9 below). The two enforcement tests have real but bounded blind spots (F3, F4). One genuine behavioral gap found: `--stencils-dir` is the only told-directory flag with no existence check at the wiring boundary (F2).
- The provider-startup seam (fourth consecutive round producing a finding here): round 6's "positive evidence" rule has its own general-case blind spot (F1) — the evidence lines are never required to sit anywhere near each other, so one line of ordinary agent prose can still classify a healthy pane as a gate and get keys pressed into it. Same defect class as R6-1, surviving through a narrower window.
- Everything else read (runlevel, bracket verbs, fingerprint, drift, containment, plan/discussion validators, logger sink) held up; no R6-28-shaped `bufio.Scanner` sibling exists anywhere in production.

**Merge-readiness / convergence verdict: see "Verdicts" at the end of this report** (written after fixes).

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

### Post-fix gates (Job 2, all green)

- `CGO_ENABLED=1 go build ./...` → 0.
- `CGO_ENABLED=1 go vet` over the full review-prompt package set → 0.
- `CGO_ENABLED=1 go test -count=5` over the full set + `./cmd/lyx/...` → 0, 20 package sets ok, zero flakes (`scratchpad/gotest-count5.log`).
- `CGO_ENABLED=1 go test -tags integration` over the prompt's set + `internal/logger` → 0.
- `CGO_ENABLED=1 go test ./...` (whole repo) → 0, 82 packages ok.
- `goimports -l` over all 14 changed .go files → clean.
- F2 live re-verified with a rebuilt HEAD binary: the S7 scenario now refuses with the new message; a valid `--stencils-dir` override still works.

### Observation OBS-1 — 18 leaked tmux servers on this host, NOT from this round's code (recorded for the orchestrator, cleaned up)

The end-of-round teardown sweep found 18 live `tmux -L lyx-<hash8>` servers, every one created Sep 7 19:18–21:57 (before this round began) with pane cwds under deleted `/tmp/TestRunCLIIn_StandalonePreRun_ReachesRunsOwnValidationGate*/001` TempDirs — the `unify-webster-burler-wiring` task's own development-window test runs (interrupted suites or mid-development states; `t.Cleanup` never runs on a killed test binary). Current HEAD verified NOT leaking: an isolated `-count=1` run of that exact test at HEAD tore its server down (only a dead socket file remains — `/tmp/tmux-1000` also holds hundreds of stale socket FILES from past runs, which are inert). All 18 servers killed after confirming each one's cwd no longer exists; zero live `lyx-*` tmux servers remain. Not a finding against this branch's code (R5-1's `tearDownStandaloneReed` works at HEAD); recorded because the campaign's teardown discipline exists exactly to catch this class, and the residue predated this round.

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

## High-yield focus items — what was attempted, how far each got

1. **First-ever adversarial review of the cliwire merge — DONE, in depth.** All seven files read adversarially, all six R6 guarantees verified preserved for BOTH callers (code + hermetic + live S1–S9 through a fresh HEAD binary). Yield: F2 (real behavioral gap, fixed), F3/F4 (enforcement-test blind spots, fixed). The consolidation itself is sound; the descriptor split, the single ordered prologue, and the sink-redirect ordering all hold and are pinned by tests.
2. **General adversarial sweep — DONE.** runlevel/bracket verbs/fingerprint/drift/containment/validators/sink re-read fresh; R6-28-sibling hunt (unchecked `bufio.Scanner`) ran repo-wide and came back empty; the "found sound" list in this report is the record. Yield beyond items 1/4: nothing new — earned, not skipped.
3. **Close the two round-6 coverage gaps — DONE.** D1 (sink fallback call-site integration test, new file) and D2 (loomcli fail-closed severity test) both landed.
4. **Provider-startup seam re-examination — DONE, and it yielded this round's BLOCKING finding.** F1: round 6's positive-evidence rule lacked any adjacency requirement between its evidence lines, so one prose list item beginning with an accept phrase (or a "press Enter to confirm" mention) still classified a healthy pane as a gate — same consequence class as R6-1 through a narrower window. Fixed with a shared adjacency rule calibrated against the live-transcribed gate fixtures; not driven live (the defect and fix are fully constructible from captures, and the fix's calibration data IS the live-transcribed fixtures; a fresh-fixture live run was not needed to prove either direction).
5. **What a second Fable deployment turned up that six passes didn't — F1.** The recurring campaign shape ("a rule correct for the case it was written against, wrong in general") appeared again, in the round-6 fix itself — the fourth consecutive round to find real material in this one seam.

## Verdicts

**Merge-readiness: QUALIFIED YES.** Every finding of this round is fixed, committed, and green across all gates; the cliwire merge — this round's primary never-reviewed material — is verified sound and its six inherited guarantees are proven preserved for both callers, live where cheap and hermetically everywhere. Nothing known-broken remains on the branch. The qualification is the convergence verdict below plus the stated limits.

**Convergence: NOT MET — but the trajectory has changed shape.** The campaign's own bar is a safety pass finding nothing severe; this round found 1 BLOCKING (F1), making rounds 4-5-6-7 an unbroken four-round run of BLOCKING material, all four in or around the provider-startup seam's evolving fix lineage (R4 fingerprint wedges aside). What is different: (a) this round's BLOCKING is a strict narrowing — R6-1's whole-capture matching → R7-F1's missing adjacency — each iteration closing most of the prior hole, and the residual now stated in the code is one rendering-quirk wide, not one prose line wide; (b) everything OUTSIDE that seam and the brand-new cliwire material came back clean under a genuinely adversarial second-Fable pass. My read, for the operator: the startup-gate seam specifically has not yet earned "converged" — one more clean adversarial look at `startup.go` (cheap, one file) is defensible before declaring it done; the rest of the glyph/cliwire surface looks converged by this round's evidence. Whether that costs a full round 8 or a targeted seam-only check is the operator's call, per campaign discipline.

## Honest limits (per the README's "state the limits" guidance)

- **Windows path behavior** — unreachable from this Linux host, all seven rounds; `pathContains`'s case-fold half and `NormalizeForContainment` on Windows remain mechanical mirrors, never driven.
- **No live Master/fork startup scenario was driven this round.** F1's fix is calibrated against round 5/6's live-transcribed gate captures and proven hermetically; a real fresh-fixture startup run (which would require a never-claude-seen path per the round-5 lesson) was judged unnecessary for this fix's shape but remains the strongest possible confirmation an operator-assisted check could add.
- **No sandbox hub was built** — the hub-mode halves of R6-7/8 and of F2 are verified by code + hermetic tests only; the standalone halves were driven live.
- **`burlercli` standalone reed bring-up** — still never live-verified by any round (unchanged).
- **`DetectDrift` exact-tier auto-repair through a real Webster fork** — still never triggered live by any round (unchanged, open since round 3).
- The ~45-item pre-glyph residue from round 6 remains out of scope per the seed.
