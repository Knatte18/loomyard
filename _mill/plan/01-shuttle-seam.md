# Batch: shuttle-seam

```yaml
task: 'Seeded driver choice: ly-drive strand as the child''s driver'
batch: shuttle-seam
number: 1
cards: 2
verify: go build ./... && go test ./internal/shuttleengine/...
depends-on: []
```

## Batch Scope

This batch adds the two things `internal/shuttleengine` is missing before a caller can start a long-lived run it never `Wait`s on: a `NameOverride` pass-through on `Spec`, so the driver strand carries the byte-stable name the next bootstrap looks it up by, and an exported `RunDir()` accessor on `*Run`, so a caller that refuses after `Start` can name the directory holding `prompt.md`, `settings.json` and the events file.
It is one batch because both are one-field, no-logic additions to the same package with no dependency on anything else in this task, and because batch 4 cannot compose its spec without the first or word its probe refusal without the second.

The external interface batch 4 consumes is exactly those two symbols.
Nothing else in the package changes: if a test in `wait.go`/`attach.go`'s completion-signal tripwire needs touching, that is a signal the change drifted out of scope.

Batch-local decision beyond the overview's: `NameOverride` is forwarded **verbatim and never interpreted**, which is the same shape `SessionID` already has on this struct.
`Spec.validate` must not gain a clause about it — not a non-empty check, not a character check.
`reedengine.validateIfAbsent` is the only validator of that field anywhere, and it fires only when `IfAbsent` is set, which this task never sets.

## Cards

### Card 1: Spec.NameOverride pass-through

- **Context:**
  - `internal/reedengine/strand.go`
- **Edits:**
  - `internal/shuttleengine/spec.go`
  - `internal/shuttleengine/run.go`
  - `internal/shuttleengine/spec_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Add a `NameOverride string` field to `shuttleengine.Spec` in `internal/shuttleengine/spec.go`, placed beside `Role`/`Round`/`Parent` since it is the fourth piece of strand identity.
  Its doc comment must state that it is forwarded verbatim into the `reedengine.AddSpec` `Runner.Start` builds and never interpreted — the same contract `SessionID`'s own doc comment already states — and that an empty value leaves reed's own `<ROLE>:<ROUND>:<SHORT_GUID>` display-name template in force.
  In `internal/shuttleengine/run.go`, add `NameOverride: spec.NameOverride` to the `reedengine.AddSpec` literal inside `Runner.Start`, beside the existing `Role`/`Round`/`Parent` entries.
  Do not add any clause about the new field to `Spec.validate` — it is engine/reed vocabulary, exactly like `Effort` and `Version`, both of whose doc comments already say `validate` does not inspect them.
  In `spec_test.go` assert that `validate` leaves a non-empty `NameOverride` untouched and accepts an empty one, so a later "tidy up the validator" change cannot quietly start rejecting either.
- **Commit:** `feat(shuttleengine): forward Spec.NameOverride into the strand add spec`

### Card 2: Run.RunDir accessor

- **Context:**
  - `internal/shuttleengine/wait.go`
- **Edits:**
  - `internal/shuttleengine/run.go`
  - `internal/shuttleengine/run_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Add `func (run *Run) RunDir() string` to `internal/shuttleengine/run.go`, returning the unexported `run.runDir` field, placed beside the existing `func (run *Run) StrandGUID() string` and following its shape exactly — a one-line accessor with a doc comment, no computation.
  Its doc comment must say what the directory holds (`prompt.md`, `settings.json`, the events file) and why the accessor is exported: a caller that starts a run and never `Wait`s on it has no other way to name the one directory an operator would look in, and batch 4's pane-liveness probe refuses the bootstrap with exactly that path.
  Do not export `runDir` as a field and do not add a setter.
  In `run_test.go` assert `RunDir()` returns the same directory `Start` created, by comparing it against the run-directory root the test's own `Runner` config names.
- **Commit:** `feat(shuttleengine): expose a run's directory through Run.RunDir`

## Batch Tests

`verify: go build ./... && go test ./internal/shuttleengine/...` runs the package's whole untagged suite plus a whole-module build.
The module build is in scope because `Spec` is a struct every shuttle caller constructs by field name; a field added in the wrong position breaks nothing, but this is the batch where a typo in `run.go`'s `AddSpec` literal would surface as a compile error in a package this verify's test list does not name.

Two assertions in this batch are load-bearing beyond their size.
The `validate`-leaves-`NameOverride`-alone assertion in `spec_test.go` is what pins the field as pass-through vocabulary rather than validated input, which is the property the Shuttle Provider-Seam Invariant depends on here — the moment `validate` starts inspecting it, `shuttleengine` has opinions about strand naming, which is reed's vocabulary.
The `RunDir()` assertion is what stops the accessor being quietly re-pointed at the worktree root or the run-dir root, either of which would still compile and would send an operator chasing a refusal into a directory holding nothing.
