# `loom` review — round 3 (sonnet5-xhigh-r3)

Independent clean-room review + fix round. Scope: thread A regression-alertness (converged, light
touch), thread B residual-close (the `Started`-gating coverage gap), thread C open adversarial pass
over loom's bootstrap/crash-recovery machinery.

Tag: `sonnet5-xhigh-r3`. Worktree: `/home/knatte/Code/loomyard/wts/crucible-loom-refshape-registry`,
branch `crucible-loom-refshape-registry`.

This file is built incrementally during Job 1 per the review prompt's "Log as you go" rule: test
observations and provisional findings are appended as they happen; only the executive summary and
final severity ordering are written last, after Job 1 completes.

## What was tested

### Code reading / tracing (Job 1, clean-room — no prior review material opened yet)

- Diffed all three thread-B commits in full (`git show d0e5a0e7b`, `git show aba2c270a`,
  `git show 69886823e`).
- Read `internal/shuttleengine/wait.go`, `attach.go`, `run.go`, `rundir.go`, `engine.go`, `spec.go`,
  `config.go` in full (current state, not just diffs).
- Read `internal/loomengine/seed.go` (`CheckSeed`, `VerifySeedOwnership`), `coherence.go`,
  `report.go` in full.
- Read `internal/loomcli/bootstrap.go`, `run.go`, `drive.go` in full.
- Read `internal/loomshed/loompreflight.go` in full (the `Loom-Preflight` producer wrapping
  `CheckSeed`).
- Read `internal/shedadapters/singlellm.go`'s `Call` (the `Attach`-then-`Start` composition
  Discussion-Write/Plan-Write use) to confirm production wiring matches wait.go/attach.go's own
  doc-comment claims.
- Traced production call sites of `Runner.Attach` (`grep`): `shedadapters/burler.go:467`,
  `shedadapters/singlellm.go:118`, `shedadapters/bouncer.go:312,501,608`.
- Read `manifest/designs/loom.md`'s "Crash recovery" section (lines 337-381) in full, including its
  one documented "Accepted residual" (the done-but-not-yet-persisted window) — confirmed my own
  thread-C findings below are NOT the same window and are not otherwise documented anywhere.

### Traced-but-NOT-a-finding (investigated, confirmed sound, recorded so it isn't re-litigated)

- **`VerifySeedOwnership` vs `CheckSeed`'s disposition-sharing claim, for failure modes other than
  `state.ErrDecode`.** Traced `internal/state.ReadJSONStrict`'s three failure shapes: a lock-acquire
  failure (unwrapped), `state.ErrRead` (an `os.ReadFile` failure other than not-exist), and
  `state.ErrDecode` (a decode failure). Both `CheckSeed`'s `rerr`-handling and
  `VerifySeedOwnership`'s now-fixed handling escalate anything that is not `ErrDecode` — a
  lock-acquire failure and a genuine permission-denied read both escalate identically in both
  functions. `CheckSeed` additionally has an earlier `os.Stat`-based gate (`CheckSeedUnreadable`)
  that classifies a stat failure as a determined, non-escalating verdict — `VerifySeedOwnership` has
  no analog for this gate. Traced whether this is a live disposition mismatch: `CheckSeed`'s own doc
  comment states plainly that this branch "carries no unreachability claim" but is reachable ONLY via
  a TOCTOU race between Shed's own step-1 read and CheckSeed's later `os.Stat` (both reading the same
  path at two different times, as two different pieces of code) — `VerifySeedOwnership` runs
  chronologically BEFORE step 1 ever executes (it gates `lyx loom run`/`lyx loom drive` before
  `Shed.Run` starts), so it structurally cannot ever be racing against "step 1's own prior read" the
  way CheckSeed's TOCTOU branch is. For every failure mode actually reachable from
  `VerifySeedOwnership`'s own call sites (a persistent, non-transient permission/lock condition), both
  functions escalate identically. Conclusion: NOT a finding — the fix's own claim holds for every
  practically-reachable case; `CheckSeedUnreadable`'s narrow TOCTOU-only path is not something
  `VerifySeedOwnership` can experience the same way. Recorded so a later round does not need to
  re-derive this.

### Hermetic commands (all green, cold state)

- `go build ./...` — clean, no output.
- `go vet ./internal/loomengine/... ./internal/loomcli/... ./internal/loomshed/... ./internal/planparser/... ./internal/planglyph/... ./internal/websterengine/... ./internal/webstercli/... ./internal/shuttleengine/...` — clean, no output.
- `go test -count=5 ./internal/loomengine/... ./internal/loomcli/... ./internal/loomshed/... ./internal/planparser/... ./internal/planglyph/... ./internal/websterengine/... ./internal/webstercli/... ./internal/shuttleengine/... ./cmd/lyx/...` — all `ok`.
- `go test ./...` (full repo, once) — all `ok`, nothing skipped that shouldn't be.

## Findings (provisional, severity TBD at the end)

- **F-C1 (thread C, code, severity TBD — leaning LOW, CONFIRMED via trace, not live-reproduced —
  see reasoning).** `internal/shuttleengine/run.go`'s `Start` has a narrow, structurally-inherent
  race between `r.reed.AddStrand(...)` succeeding (which actually creates the live tmux pane and
  starts the launch command running inside it) and `saveRunState(runDir, state)` persisting
  `run.json` for it. A process killed in exactly that window (a real `kill -9`, not merely "before
  the first liveness tick" — this is narrower, and earlier, than the window the seeded residual and
  `d0e5a0e7b` are about) leaves a genuinely live, running pane registered in reed's own strand table
  with NO `run.json` anywhere naming its `StrandGUID`. Traced the consequences: (1)
  `Attach`/`collectAttachCandidates` can never discover it (candidate matching scans `run.json`
  files, never reed's strand table directly), so `SingleLLMProducer.Call`'s `Attach` probe reports
  `found=false`; (2) `sweepOrphansOpportunistic`/`sweepOrphans` only removes run DIRECTORIES whose
  strand is no longer live — it has no reverse check for a live strand with no owning directory at
  all, so this orphaned strand is never flagged or cleaned up; (3) the next `Start` call therefore
  proceeds to `AddStrand` a genuinely NEW pane for the same step, running the same prompt — the exact
  two-agents-on-one-task duplicate hazard this module's design otherwise goes to considerable lengths
  to prevent (`errStrandNotTracked`, `errStrandPaneBindingCleared`, `verdictError`, the whole
  `Attach` mechanism). Confirmed this is NOT the same window as `manifest/designs/loom.md`'s one
  documented "Accepted residual" (that one is about `finalize`'s own done-but-not-yet-persisted
  window, entered only once a run has ALREADY reached a terminal outcome; this one is about the
  registration step of a run that has not yet even started waiting). Not live-reproduced (timing a
  real process kill to land inside a single-digit-microsecond window between two syscalls is not
  practically achievable from a black-box smoke test), but traced end-to-end against the actual
  production code path with no substitution.
  Fix approach (see Job 2): this looks structurally unclosable by re-ordering the two writes (any
  ordering just relocates the window between two independent stores — reed's own persisted state and
  shuttleengine's own run.json — a two-phase-commit problem, not a bug in either store on its own),
  so the fix is a documentation one: name it explicitly as a second "Accepted residual" alongside the
  existing one in `manifest/designs/loom.md`, plus a code comment at the exact spot in `run.go`,
  matching this codebase's own established idiom for narrow, currently-unfixable residuals (e.g.
  `sendVerified`'s "Residual, stated rather than papered over" comment).

## Executive summary

(written last)

## Scope assessment

(written last)

## Docs & operability findings

(written last)
