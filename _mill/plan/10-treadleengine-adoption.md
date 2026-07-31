# Batch: treadleengine-adoption

```yaml
task: Diagnostic tracing (trace) on the logger module
batch: treadleengine-adoption
number: 10
cards: 2
verify: go test ./internal/treadleengine/...
depends-on: [5]
```

## Batch Scope

Fixes the now-stale "geometry-blind, excludes hubgeometry" prose in `internal/treadleengine`'s seam-enforcement test and CONSTRAINTS.md (no allowlist code change — internal/logger is already allowlisted there and the test only walks this package's own files, not transitive imports), and applies the adoption-pass to `run.go`'s unlogged error-return paths.

## Cards

### Card 36: Treadle Runner-Seam Invariant prose amendment

- **Context:**
  - `internal/hubgeometry/hubgeometry.go`
- **Edits:**
  - `internal/treadleengine/seam_enforcement_test.go`
  - `CONSTRAINTS.md`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  `internal/treadleengine/seam_enforcement_test.go`'s `allowedImports` map does **not** change — internal/logger is already in the allowlist, and `TestRunnerSeamInvariant_AllowlistOnly` walks only this package's own directory, so a transitive `logger → hubgeometry` edge does not fail it. Fix only the prose that is now inaccurate: the file's header comment (lines 1-9) and the doc block immediately preceding `allowedImports` (lines 23-27, containing "Deliberately excludes internal/hubgeometry: the engine is geometry-blind") both claim treadle's import graph excludes `hubgeometry` outright. After this task, internal/logger (an allowlisted **direct** import) pulls in hubgeometry **transitively**, so the claim is true only for direct imports. Reword both comment blocks to say "excludes internal/hubgeometry as a direct import" (or equivalent), not "excludes internal/hubgeometry."

  Amend CONSTRAINTS.md's "Treadle Runner-Seam Invariant" entry the same way: its bullet listing the import allowlist currently ends "...deliberately NOT internal/hubgeometry: the engine is geometry-blind (caller-supplied absolute `runDir`/`GateDir`)" — qualify this to state the exclusion holds for direct imports only, now that the allowlisted internal/logger transitively pulls it in. Same "amend the comments too" discipline discussion.md's Constraints section calls for on this exact entry: "a machine-check whose own comment states something no longer true is exactly the rot this repo's invariant discipline exists to prevent."
- **Commit:** `docs(treadleengine): correct the geometry-blind claim now that logger transitively imports hubgeometry`

### Card 37: `run.go` adoption pass

- **Context:**
  - `internal/treadleengine/state.go`
- **Edits:**
  - `internal/treadleengine/run.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  Apply the `adoption-scope` done-criterion to `internal/treadleengine/run.go`'s currently-unlogged candidates (verified against the existing 10/5/2/2 `logger.Warn` calls already in `judge.go`/`targeting.go`/`handoff.go`/`run.go` — all of which are fail-safe-default patterns; the sites below are the ones without an accompanying log call):
  - `run.go:224-227` — `if saveErr := saveState(runDir, st); saveErr != nil { return Result{}, saveErr }` (line 224-225) returns `saveErr` directly — it is **not** the discarded value. What's actually discarded is the **original gate-command error** (`err`, the one line 227's `e.errf("round %d gate command: %w", round, err)` would have returned had `saveState` succeeded): when `saveErr` fires, execution returns at line 225 and never reaches line 227, so `err` — the real reason this round failed — never surfaces anywhere, logged or otherwise, while only the unrelated persistence failure (`saveErr`) is visible to the caller. Add `logger.Warn` naming `runDir`, `round`, `err` (the original gate-command failure being lost), and `saveErr` (the persistence failure masking it) immediately before line 225's `return`, since the masked `err` is exactly a "swallowed by a fallback" case per `adoption-scope`'s criterion (a).
  - `run.go:183` and `run.go:426-429` — both call `moveStaleArtifacts(e.name, runDir, round, <1 or attempt>)` (pre-round clear at 183, retry-attempt clear at 426-429) and propagate its failure with no log call. `moveStaleArtifacts` itself wraps via `moveStaleIfExists` (`state.go:204,213`) with `name`/`path` context only — no `round`/`attempt` field, unlike `run.go:447-448`'s own wrap (`"round %d attempt run: %w"`), which is why these two qualify while 447-448 (below) does not: a reader of the bare wrapped error knows which file failed to move but not which round or attempt was in progress. Add `logger.Warn` naming `e.name`, `round`, `attempt` (1 at the 183 site), and the error at both call sites.
  - `run.go:486-490` — the died/timeout retry fall-through: when `result.Outcome` is neither `OutcomeDone` nor `OutcomeAsking` (a died/timeout attempt) and `attempt == 1`, the loop silently `continue`s to a second attempt with no log call at all — the retry itself is currently invisible. Add `logger.Warn` naming `round`, `result.Outcome`, and `result.SessionID` immediately before the implicit `continue` at the bottom of the loop body, so the retry is observable, not only its eventual "failed twice" outcome (which already errors with full context at line 488's sibling `attempt == 2` branch and does not need an additional call). Do **not** add a call at `run.go:447-448` (`RunAttempt`'s own Go-error return, `e.errf("round %d attempt run: %w", round, err)`) — that error already names `round` in its wrapped message, so it does not qualify under `adoption-scope`'s negative case (context already present).

  Re-audit the rest of `run.go` against the stop-rule while making these edits — this is a verified floor, not an exhaustive ceiling. Existing suite must stay green (instrumentation only, no behavior change); no new log-line-wording assertions per the "Adoption-pass tests" policy.
- **Commit:** `feat(treadleengine): adopt logger.Warn across run.go's unlogged retry and cleanup paths`

## Batch Tests

`verify: go test ./internal/treadleengine/...` — the existing suite (including `TestRunnerSeamInvariant_AllowlistOnly`, unaffected by Card 36's prose-only edit) must stay green.
</content>
