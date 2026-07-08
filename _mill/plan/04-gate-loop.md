# Batch: gate-loop

```yaml
task: "Build perch - the review gate loop"
batch: "gate-loop"
number: 4
cards: 4
verify: go test ./internal/perchengine/
depends-on: [3]
```

## Batch Scope

The engine itself: the `Burler` seam and `Engine` scaffold, the pluggable convergence gate
(command execution + feed-forward), and `Engine.Run` — the deterministic round loop with
milestone-laddered stuck detection, non-done handling, pause, and resume. This is the
module's core deliverable and its strong test surface; card 13 is the scenario suite that
encodes the discussion's Testing section. External interface for batch 5:
`perchengine.New`, `perchengine.Options`, `perchengine.Engine.Run(p Profile, runDir string)
(Result, error)`, plus the batch-2 exports (`DeriveRunID`, `ProfileHash`, `PauseFlagPath`).

## Cards

### Card 10: Burler seam, Engine scaffold, Options

- **Context:**
  - `internal/burlerengine/engine.go`
  - `internal/burlerengine/profile.go`
  - `internal/hubgeometry/hubgeometry.go`
  - `internal/perchengine/config.go`
  - `internal/perchengine/judge.go`
- **Edits:** none
- **Creates:**
  - `internal/perchengine/engine.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** engine.go: `type Burler interface { Run(burlerengine.Profile,
  burlerengine.RunOpts) (burlerengine.Result, error) }` with compile-time proof `var _
  Burler = (*burlerengine.Engine)(nil)`. `type CommandRunner func(argv []string, dir string,
  timeout time.Duration) (output []byte, exitZero bool, err error)` — the gate-command seam
  (err is reserved for could-not-run failures: not-found, timeout kill; a non-zero exit is
  `exitZero=false` with nil err). `type Options struct { PauseRequested func() bool;
  RunCommand CommandRunner }` — zero value means "no pause source" and "the real exec runner
  from card 11". `type Engine struct` holding `burler Burler`, `shuttle Shuttle`, `cfg
  Config`, `layout *hubgeometry.Layout`, `pauseRequested func() bool`, `runCommand
  CommandRunner`; `func New(burler Burler, shuttle Shuttle, cfg Config, layout
  *hubgeometry.Layout, opts Options) *Engine` stores the Options fields VERBATIM (nil
  allowed) — engine.go never names `execGateCommand` or any other gate.go symbol; the nil
  defaults are substituted at the use sites in card 12's run.go (nil `pauseRequested` →
  never paused; nil `runCommand` → `execGateCommand`). This keeps cards 10 and 11 free of a
  mutual compile dependency: card 10 compiles alone, and card 11 references only card 10's
  `CommandRunner` type. Doc comments state the
  layering: perch -> burler -> shuttle is a strict chain; the engine is weft-blind and
  geometry-blind (operates on a caller-supplied absolute runDir); the shuttle seam exists
  solely for the judge/triage utility calls (burler reaches shuttle itself). Compile-only
  card: its behavior is exercised by cards 11-13's tests.
- **Commit:** `perch: add Burler seam, Engine scaffold, and Options`

### Card 11: gate command execution and convergence evaluation

- **Context:**
  - `internal/perchengine/engine.go`
  - `internal/perchengine/profile.go`
  - `internal/burlerengine/verdict.go`
  - `_mill/discussion.md`
- **Edits:** none
- **Creates:**
  - `internal/perchengine/gate.go`
  - `internal/perchengine/gate_test.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** gate.go: `func execGateCommand(argv []string, dir string, timeout
  time.Duration) ([]byte, bool, error)` — the production `CommandRunner`: `exec.CommandContext`
  with a `context.WithTimeout`, `cmd.Dir = dir`, `CombinedOutput()`; exit non-zero →
  `(output, false, nil)`; context deadline exceeded → error naming the timeout; start
  failure → error. No shell — argv is executed directly (portable, quoting-safe). `func
  writeGateOutput(path string, argv []string, output []byte, exitZero bool) error` — writes
  `round-<token>-gate.md`: a small header naming the argv and pass/fail, then the raw
  combined output (this file is what the next round's burler hydration and the operator
  read). `func converged(mode GateMode, verdict burlerengine.Verdict, gatePassed *bool)
  bool` — `GateLLMVerdict`: verdict == VerdictApproved; `GateCommand`: gatePassed != nil &&
  *gatePassed (the burler verdict does NOT decide convergence in this mode — its review
  still drives B's fixes); `GateBoth`: both. gate_test.go: `converged` truth table across
  all three modes; `execGateCommand` against a real trivial command cross-platform (use `go
  version` for pass; `go bogus-subcommand` for fail — the Go toolchain is the one binary
  guaranteed present in this repo's test environment) plus the not-found error path;
  `writeGateOutput` shape test.
- **Commit:** `perch: add gate command runner, gate output file, and convergence evaluation`

### Card 12: Engine.Run — the round loop

- **Context:**
  - `internal/perchengine/profile.go`
  - `internal/perchengine/result.go`
  - `internal/perchengine/roundfiles.go`
  - `internal/perchengine/state.go`
  - `internal/perchengine/judge.go`
  - `internal/perchengine/judgeverdict.go`
  - `internal/perchengine/gate.go`
  - `internal/burlerengine/engine.go`
  - `internal/burlerengine/verdict.go`
  - `internal/shuttleengine/engine.go`
  - `internal/logger/logger.go`
  - `_mill/discussion.md`
- **Edits:** none
- **Creates:**
  - `internal/perchengine/run.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** run.go: `func (e *Engine) Run(p Profile, runDir string) (Result, error)`.
  Entry: `p.validate(e.cfg)`; `os.MkdirAll(runDir)`; `ProfileHash(p)`;
  `clearPauseFlag(runDir)`; `loadOrInitState(runDir, hash, p.RoundCaps)` (its fresh /
  resume / hash-mismatch / already-finished classification is card 6's; hard errors
  propagate). Then loop, one iteration per round N starting at `len(state.Rounds)+1`:
  Seam defaulting happens here, once, at Run entry: `pause := e.pauseRequested` (nil → a
  func returning false), `runCmd := e.runCommand` (nil → `execGateCommand`) — card 10's New
  stores Options verbatim precisely so this file owns the defaults.
  1. **Pause boundary:** if `pause()` → persist state, return
     `Result{Outcome: OutcomePaused, ...}`. Checked ONLY here — never mid-round.
  2. **Round with retry:** for attempt = 1, 2: `moveStaleArtifacts`; `artifactPaths(runDir,
     N, attempt)`; assemble hydration from all completed rounds' records (ReviewPath +
     GatePath-when-set into priorReviews, FixerReportPath into priorFixerReports);
     `buildRoundProfile`; `e.burler.Run(roundProfile, burlerengine.RunOpts{Model: p.Model,
     Effort: p.Effort, Timeout: p.Timeout, Round: roundToken(N, attempt)})`. Hard error from burler →
     propagate. Branch on `Result.Outcome`: `OutcomeDone` → proceed to step 3.
     `OutcomeAsking` → `runTriage` (fail-safe): `TriageRetry` → count as this round's one
     retry and re-attempt (attempt 2), recording TriagePath; `TriageGiveUp` → return error
     `"perch: round %d agent gave up asking: %s (session %s, run dir %s)"` carrying the
     triage rationale, SessionID, and burler Result.RunDir. `OutcomeDied`/`OutcomeTimeout`
     → retry once (attempt 2). A SECOND consecutive non-done attempt → return error
     `"perch: round %d failed twice (%s); session %s, kept shuttle run dir %s"` — an
     infrastructure error, deliberately NOT OutcomeStuck.
  3. **Gate command** (mode `GateCommand`/`GateBoth` only): `e.runCommand(p.Gate.Command,
     e.layout.WorktreeRoot, p.Gate.Timeout)`; `writeGateOutput` to the round's Gate path
     (written on pass AND fail — the record is cheap; only a FAILED gate file is hydrated
     forward); a runner error (not non-zero exit) → propagate as a hard error.
  4. **Convergence:** `converged(p.Gate.Mode, verdict, gatePassed)` → record the round,
     persist terminal `Outcome: "APPROVED"`, return OutcomeApproved.
  5. **Stuck ladder** (only on a non-converged round; all judge triggers are
     burler-verdict-based — a round with VerdictApproved but a failing command skips 5a-5c
     entirely, bounded by 5d and the feed-forward): let caps = resolved RoundCaps:
     (5a) N == caps[last] → record, persist `STUCK`/`hard-cap`, return (no judge at the
     final rung). (5b) N ∈ caps[:-1] AND verdict is VerdictBlocking → `runMilestone`
     (HardCap = caps[last]): `JudgeStop` → `STUCK`/`milestone-stop`; `JudgeContinue`/
     `JudgeUncertain` → continue (Warn on uncertain is card 9's). The milestone gate
     REPLACES the circling check for that round. (5c) otherwise, N >= 2 AND verdict is
     VerdictBlocking → `runCircling` over the full prior-review history:
     `JudgeCircling` → `STUCK`/`circling`; else continue. (5d) a VerdictApproved
     non-converged round (command mode) runs no judge and continues.
  6. **Persist:** append the roundRecord (paths, verdict, blocking count = count of
     SeverityBlocking findings, attempts, judge/gate outcomes, SessionID); `saveState`.
  The returned Result mirrors the state's rounds as RoundSummary values. Every error message
  is `"perch: "`-prefixed. run.go contains no `_lyx`, weft, or provider references.
- **Commit:** `perch: implement Engine.Run round loop with milestone ladder and stuck detection`

### Card 13: deterministic loop scenario suite

- **Context:**
  - `internal/perchengine/run.go`
  - `internal/perchengine/engine.go`
  - `internal/perchengine/state.go`
  - `internal/perchengine/result.go`
  - `internal/perchengine/profile.go`
  - `internal/perchengine/roundfiles.go`
  - `internal/burlerengine/engine.go`
  - `internal/burlerengine/verdict.go`
  - `internal/shuttleengine/engine.go`
  - `_mill/discussion.md`
- **Edits:** none
- **Creates:**
  - `internal/perchengine/run_test.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** run_test.go: a scripted `fakeBurler` (queue of `burlerengine.Result`
  values; records every received Profile/RunOpts; writes the review/fixer files its scripted
  done-results imply so hydration/existence checks hold) and a scripted `fakeShuttle` (writes
  a caller-scripted verdict file for each judge/triage Spec, or errors). Scenarios — each
  from the discussion's Testing section, table-driven where natural:
  loop-until-dry (BLOCKING, BLOCKING, APPROVED → OutcomeApproved, RoundsRun 3, hydration
  accumulates: round 3's profile lists rounds 1-2's reviews and fixer reports);
  hard cap (still BLOCKING at caps[last] → STUCK/hard-cap, and the fakeShuttle proves NO
  judge spec was issued for that final round);
  milestone gate (CONTINUE at a rung → loop proceeds; STOP → STUCK/milestone-stop;
  UNCERTAIN → continues; the rung round issues exactly one judge call — milestone replaces
  circling);
  per-round circling (CIRCLING at a mid-window round ≥ 2 → STUCK/circling immediately; no
  judge call on round 1; no judge call on an APPROVED-verdict round);
  judge fail-safe (shuttle error / non-done / garbage verdict file → loop continues, never
  errors, never STUCK);
  gate modes (llm-verdict ignores a scripted failing command runner; command mode with
  VerdictApproved + failing command → loop continues AND the next round's priorReviews
  include the round-N-gate.md path; command mode with VerdictBlocking + passing command →
  OutcomeApproved; both requires both; fake CommandRunner records dir == layout
  WorktreeRoot);
  non-done outcomes (died then done → round completes with Attempts 2 and a `b`-token review
  path; died twice → error containing the session id and RunDir, not STUCK; asking + triage
  RETRY → retry; asking + triage GIVE_UP → error carrying the rationale; triage fail-safe →
  RETRY);
  resume (run to round 2 mid-block, new Engine.Run on the same runDir continues at round 3;
  terminal state → error; hash mismatch → error naming a fresh --run-id; a stale
  half-written review file for the incoming round is renamed `.stale` and the round re-runs);
  pause (PauseRequested true before round 2 → OutcomePaused after round 1, burler called
  exactly once; resume clears the flag file and continues).
- **Commit:** `perch: add deterministic loop scenario suite (fake burler, fake judge)`

## Batch Tests

`verify:` runs `go test ./internal/perchengine/` — the full deterministic surface: gate truth
table + real-exec runner (card 11) and the loop scenario suite (card 13) on temp run dirs
with fakes for all three seams. No LLM, no psmux, no weft.
