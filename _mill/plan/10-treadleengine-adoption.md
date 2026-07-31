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

- **Context:** none
- **Edits:**
  - `internal/treadleengine/run.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  Apply the `adoption-scope` done-criterion to `internal/treadleengine/run.go`'s currently-unlogged candidates (verified against the existing 10/5/2/2 `logger.Warn` calls already in `judge.go`/`targeting.go`/`handoff.go`/`run.go` — all of which are fail-safe-default patterns; the sites below are the ones without an accompanying log call):
  - `run.go:224-227` — `saveState(runDir, st)`'s error (`saveErr`) is swallowed as a bare error and the round is aborted with a different error (`e.errf("round %d gate command: %w", round, err)`) — the `saveErr` itself never surfaces anywhere, logged or otherwise. Add `logger.Warn` naming `runDir`, `round`, and `saveErr` before the function returns, since this is exactly a "swallowed by a fallback" case per `adoption-scope`'s criterion (a).
  - `run.go:426-429` — `moveStaleArtifacts(e.name, runDir, round, attempt)`'s failure is wrapped and propagated with no log call. Add `logger.Warn` naming `e.name`, `round`, `attempt`, and the error — this is a retry-adjacent cleanup step per `level-policy`.
  - `run.go:447-448` — a died/timeout/failed `RunAttempt` call (which the caller retries once) is wrapped in an error and returned with no log call before the retry proceeds. Add `logger.Warn` naming `round` and the attempt's failure before the retry, so the retry itself is observable, not only its eventual outcome.

  Re-audit the rest of `run.go` against the stop-rule while making these edits — this is a verified floor, not an exhaustive ceiling. Existing suite must stay green (instrumentation only, no behavior change); no new log-line-wording assertions per the "Adoption-pass tests" policy.
- **Commit:** `feat(treadleengine): adopt logger.Warn across run.go's unlogged retry and cleanup paths`

## Batch Tests

`verify: go test ./internal/treadleengine/...` — the existing suite (including `TestRunnerSeamInvariant_AllowlistOnly`, unaffected by Card 36's prose-only edit) must stay green.
</content>
