# Batch: perchengine-adoption

```yaml
task: Diagnostic tracing (trace) on the logger module
batch: perchengine-adoption
number: 12
cards: 1
verify: go test ./internal/perchengine/...
depends-on: [5]
```

## Batch Scope

`internal/perchengine` has zero `logger` calls today (greenfield, per discussion.md's adoption-baseline table) and is a thin, deliberately fail-loud wiring layer with no direct `exec.Command`/lock/subprocess calls of its own. This batch adds the first adoption-pass calls at its three best-audited passthrough boundaries.

## Cards

### Card 39: First adoption pass on perch's error boundaries

- **Context:** none
- **Edits:**
  - `internal/perchengine/engine.go`
  - `internal/perchengine/config.go`
  - `internal/perchengine/identity.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  Apply the `adoption-scope` done-criterion to perch's three best-qualifying passthrough sites (all fail loud today — none are swallowed or retried, so this adds diagnostic visibility at perch's own boundary rather than changing fail-safe posture, consistent with internal/perchengine's deliberately fail-loud design):
  - `internal/perchengine/engine.go:152-155` — `te.Run(tp, runDir)` (delegating to `treadleengine.Engine.Run`, which itself owns lock acquisition, the gate subprocess, and `state.json` I/O) has its error passed through bare, with no perch-level log naming the profile/`runDir` before it surfaces. Add `logger.Warn` naming the profile hash/`runDir` and the error immediately before returning it.
  - `internal/perchengine/config.go:140-143` — `modelspec.LoadRegistry(baseDir)` (reads `models.yaml`) returned bare, no context added. Add `logger.Warn` naming `baseDir` and the error.
  - `internal/perchengine/identity.go:100-103` — `TerminalOutcome` delegates to `treadleengine.TerminalOutcome(runDir)` (reads `state.json`) and passes its error straight through unlogged. Add `logger.Warn` naming `runDir` and the error.

  These are perch's best three candidates per the research audit — re-confirm each still qualifies against the `adoption-scope` criterion while editing (a bare passthrough of a process/lock/file/subprocess result with no perch-level context is exactly the target case), and extend to a further site only if the audit surfaces one meeting the same bar. Existing suite must stay green.
- **Commit:** `feat(perchengine): add first-pass logger.Warn calls at the three passthrough boundaries`

## Batch Tests

`verify: go test ./internal/perchengine/...` — existing suite must stay green.
</content>
