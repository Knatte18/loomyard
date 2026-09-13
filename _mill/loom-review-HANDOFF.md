# loom-step + self-report crucible campaign — orchestrator handoff

## Current state
Round 1 (`opus-high-r1`) complete and independently verified by the orchestrator. Round 2 not yet spawned — waiting on the operator's model + effort pick (rotate away from Opus per the method).

## CLOSED-AND-VERIFIED (round 1, commit range `4e0b3265c..4c875f690`)

Orchestrator independently reproduced, from a cold checkout, on the committed tree:

- `go build ./...` — OK
- `go vet` over all nine trio packages + shedadapters/websterengine — OK
- `go test -count=5` over all nine trio packages + `./cmd/lyx/...` — OK
- `go test ./...` (whole repo) — OK, no regressions
- `go test -tags smoke ./internal/loomcli/... -run Smoke -v -count=1` — 15/15 PASS (13 pre-existing + 2 new), zero real LLM subprocesses confirmed by source inspection

**Sabotage-proved personally** (fix reverted, new/changed test watched fail at the claimed assertion, fix restored, `git diff --stat` empty after each):
- **F-0 (BLOCKING)** `713ab509a` — removed `awaitRunLockHalted` from `dispositionForHandshake`'s proceed set; `TestDispositionForHandshake/Halted` failed `dispositionForHandshake(2) = 1; want 0`, exactly as the fixer report claimed.
- **F-1 (MEDIUM)** `1d671f44c` — commented out the `ensureFrictionDirAfterSeed` call site; `TestSmokeBootstrap_FirstSeedClearsFrictionNotesAndReentryKeepsThem` failed "stale friction note still present after a genuine first seed".
- **F-3 (MEDIUM)** `78a407698` — no-opped `ensureStatusLockDir`; `TestSmokeStatusAndPause_OnNeverBootstrappedPairNameTheRemedy` failed both subtests with the exact pre-fix internal lock-path leak text.
- **F-4 (MEDIUM)** `eb6af7720` — removed the `awaitLiveSeed` probe call from the Bouncer's re-bounce branch; `TestBouncer_ReBounceProbesForALiveSeed` failed both subtests ("returned from the re-bounce without probing for a live seed").
- **F-6 (LOW)** `6a0750a7e` — bypassed the reflection lock's `!free` branch; `TestReflectFriction_SkipsWhenAnotherDriverHoldsTheReflectionLock` failed with a nil-pointer panic on the bypassed guard (a harder failure than the original assertion, still conclusive: the guard is load-bearing).

**Reviewed by diff, not independently sabotage-proved** (low-risk: doc-only or a one-line log-level change): F-2 (doc-only, deliberate — see below), F-5 (`Info`→`Warn` log level), F-7 (comment rewrite), D-1, D-2 (both doc-only).

**Live-fire confirmed real**, via `gh issue view`:
- #240 — `loom anomaly: escalation-to-human — dummy-r2 — Preflight#0` — OPEN, label `bug`, created 2026-09-13T13:40:43Z. The deliberate Tier-1 live-fire.
- #241 — `Plan spec should document card format spec file path` — OPEN, label `documentation`, created 2026-09-13T13:41:06Z. Filed by Tier 2's reflection agent on its own judgment off the same halt.

**Docs updated in the same commits, confirmed via `git diff --stat`:** `manifest/designs/loom-step.md`, `loom.md`, `self-report-tier1.md`, `self-report-tier2.md`, `plugins/ly/skills/ly-supervise/SKILL.md`, `tools/sandbox/SANDBOX-CORE-SUITE.md`. `docs/overview.md`/`CONSTRAINTS.md` correctly untouched (no module moved, no new cross-cutting invariant). `manifest/roadmap.md` correctly untouched (hardening, not a roadmap move).

**F-2 is a deliberate documentation-only fix**, not a residual: making `step` fire Tier 1 automatically would let a primitive a supervisor calls up to ~40×/run mint public GitHub issues unattended — an outward-facing design decision, not a hardening defect. The exemption is now documented in three places instead of nowhere; that's the actual fix.

## Residual seeded for round 2 (see `_mill/loom-review-prompt.md`)

Round 1 itself flagged four things worth a second, independent pass:
1. F-0's `halted` predicate is a deliberate narrowing of the refusal (proceeds on an already-`blocked` resume before its first persist) — round 1 called this out for a second opinion, not as a defect.
2. F-6's fix is real and sabotage-proved at the unit level, but the live two-driver race it targets was never reproduced (forbidden by the cost declaration — needs two concurrent dummy-task drives).
3. No producer spontaneously chose to write a Tier-2 friction note during round 1 — the injection and aggregation/reflection/filing legs were both verified live, but separately (hand-placed notes for the second leg). A model-judgment event, not forceable without doctoring a prompt.
4. `Publish` and `Finalize` were never driven — `dummy-r1` stopped at `Webster-Bouncer`.

Also worth a second instance's confirmation: **F-4's fix lives in the generic `shedadapters.Bouncer`**, so it should hold identically on `Plan-Bouncer`/`Webster-Bouncer`, not just the `Discussion-Bouncer` instance round 1 drove — round 1 never checked a second segment's re-bounce branch.

## Live-fire self-report tracking

**Already captured — do NOT re-trigger.** Round 2's dummy-task fixture(s) must set `selfreport: false` and `friction: ""` (round 1 did this on its own fixture after capturing the proof, per hard rule 7).

- Issue #240 (Tier 1) — OPEN, real, confirmed via `gh issue view`.
- Issue #241 (Tier 2) — OPEN, real, confirmed via `gh issue view`.
- Closed: **not yet** — campaign has not converged (round 1 found and fixed a BLOCKING defect; a safety pass has not yet run). Close both, labeled as deliberate crucible test-fires, only at final hand-off.

## Next action
Ask the operator for round 2's model + effort pick (rotate away from Opus — Fable or Sonnet — per the method's diversity rationale), then spawn `subagent_type: crucible-reviewer-<effort>` with `model: <pick>`, prompt: "Read `_mill/loom-review-prompt.md` and do exactly what it says." Tag it `<model>-<effort>-r2`.
