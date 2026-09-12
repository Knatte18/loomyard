# Batch: frictionengine

```yaml
task: 'self-report Tier 2: per-agent friction notes for unsupervised runs'
batch: 'frictionengine'
number: 3
cards: 4
verify: go test ./internal/frictionengine/... ./contracts/stencils/...
depends-on: [1]
```

## Batch Scope

This batch delivers `internal/frictionengine`: the aggregation-and-reflection step that scans the friction directory, skips out when there is nothing to reflect on, otherwise spawns one autonomous reflection agent over the aggregated notes through a narrow `Shuttle` seam, and archives the consumed notes on a clean return only.
It imports `internal/friction` (batch 1) for `ReportFileName` and `EnsureDir`, and nothing else new.

It changes no existing package.
The single call site that drives it lands in batch 7.

The external interface batch 7 consumes is `frictionengine.Deps`, `frictionengine.Reflect(Deps) (Report, error)`, `frictionengine.Report`, and the three `Report.Status` constants.

Batch-local decision: `Reflect` is a package-level function that validates its own `Deps` as its first act, rather than a `New(Deps) (*Reflector, error)` constructor plus a method.
`internal/mergeresolve` uses the constructor shape because its `Resolver` is driven repeatedly across a merge's attempt loop;
`Reflect` is called exactly once per run, so a constructor would add a type and an error site for no second call.
The validation discipline itself is unchanged — the same distinct-error-per-field shape `mergeresolve.New` uses (`internal/mergeresolve/deps.go:106-123`).

## Cards

### Card 9: `frictionengine` package skeleton — the seam, Deps, Report, and doc

- **Context:**
  - `internal/mergeresolve/deps.go`
  - `internal/mergeresolve/doc.go`
  - `internal/shuttleengine/spec.go`
  - `internal/modelspec/modelspec.go`
  - `internal/friction/friction.go`
- **Edits:** none
- **Creates:**
  - `internal/frictionengine/deps.go`
  - `internal/frictionengine/doc.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  Create package `frictionengine` with production imports limited to the standard library, `internal/friction`, `internal/logger`, `internal/modelspec`, `internal/shuttleengine`, `internal/stencil`, and `internal/stencilstore`.
  It must not import `internal/lyxcwd` and must derive no path of its own, per the Told-Geometry Invariant.

  `deps.go` declares, following `internal/mergeresolve/deps.go`'s shape exactly:

  - `type Shuttle interface { Run(shuttleengine.Spec) (shuttleengine.Result, error) }`, the one-method seam, with the compile-time assertion `var _ Shuttle = (*shuttleengine.Runner)(nil)`.
  - `type Clock interface { Now() time.Time }` — the archive timestamp's seam, so a test asserts an exact archive directory name rather than a pattern.
    A nil `Clock` on `Deps` selects a package-local `realClock` wrapping `time.Now`.
  - `type Deps struct` with, every field told by the caller and derived by nobody here: `Shuttle Shuttle`;
    `FrictionDir string` (the absolute friction directory);
    `ArchivePrefix string` (the absolute prefix an archive sibling's timestamp is appended to);
    `StencilsDir string` (the absolute directory the reflection prompt is read from);
    `FrictionSpec string` (the model-spec string selecting the reflection agent's model);
    `Registry modelspec.Registry`;
    `Timeout time.Duration`;
    and `Clock Clock`.
  - `type Report struct { Status string; ReportPath string; NoteCount int }` plus three exported status constants, `StatusSkipped = "skipped"`, `StatusReflected = "reflected"`, and `StatusFailed = "failed"`.
    There is deliberately no `"filed"` status: nothing in Go parses the agent's report file, so whether an issue was actually created is not something this package can honestly assert.
    `ReportPath` is populated only for `StatusReflected`, and carries the **post-archive** path.

  `doc.go` carries the package godoc: why zero notes means no agent is spawned at all, why a failed reflection leaves the notes in place, why the agent's own report file is excluded from the note scan, and why every runtime failure returns a nil error.
- **Commit:** `feat(frictionengine): add the reflection step's told-value surface`

### Card 10: the reflection `shuttleengine.Spec` builder

- **Context:**
  - `internal/mergeresolve/spec.go`
  - `internal/loomengine/discussion.go`
  - `internal/frictionengine/deps.go`
  - `contracts/stencils/friction/friction-template-reflection.md`
  - `internal/friction/friction.go`
- **Edits:** none
- **Creates:**
  - `internal/frictionengine/spec.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  Create `internal/frictionengine/spec.go` with an unexported `buildReflectionSpec(deps Deps, notes []string, reportPath string) (shuttleengine.Spec, error)`, following `internal/mergeresolve/spec.go`'s resolution shape: `modelspec.Parse(deps.FrictionSpec)`, then `deps.Registry.Resolve(parsed)`, then `Model`/`Effort`/`Version` off the resolved value.

  Declare `const reflectionStencilName = "friction-template-reflection"` in this file, the one place that name appears in Go.

  The prompt is read from `deps.StencilsDir` via `stencilstore.Read` and filled with `stencil.Fill` — never composed from a Go string literal, per the Stencil Ownership Invariant.
  The three marker values are `friction_dir` (`deps.FrictionDir`), `report_path` (`reportPath`), and `note_list` (the note filenames rendered one per line as a markdown bullet list, in the sorted order the scan produced).

  The returned `Spec` sets `Prompt`, `OutputFiles: []string{reportPath}`, the three resolved model fields, `Interactive: false`, `ForkSubagents: false`, `Role: "friction"`, and `Timeout: deps.Timeout`.
  `Interactive` is false because `lyx loom run` is by definition the unattended path and an interactive spec would hang waiting for a human who is not there;
  `ForkSubagents` is false because the agent has nothing to fan out over, and authorizing forks with no user present is authorization for nothing.
  Document both in the function's doc comment.

  `reportPath` is `filepath.Join(deps.FrictionDir, friction.ReportFileName)`.
  It is the agent's mandatory output file: `shuttleengine.Spec.OutputFiles` is the completion signal, so a spec with an empty set could never report done at all, which is why this file is mandatory even though a friction note never can be.
- **Commit:** `feat(frictionengine): build the reflection agent's shuttle spec`

### Card 11: `Reflect` — scan, skip, spawn, archive

- **Context:**
  - `internal/mergeresolve/mergeresolve.go`
  - `internal/shuttleengine/engine.go`
  - `internal/frictionengine/deps.go`
  - `internal/frictionengine/spec.go`
  - `internal/friction/friction.go`
  - `internal/logger/logger.go`
- **Edits:** none
- **Creates:**
  - `internal/frictionengine/reflect.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  Create `internal/frictionengine/reflect.go` with `Reflect(deps Deps) (Report, error)`.

  **Deps validation is its first act, and the only source of a non-nil error.**
  Reject, with a distinct error each, in this order: a nil `deps.Shuttle`;
  an empty or non-`filepath.IsAbs` `deps.FrictionDir`;
  an empty `deps.ArchivePrefix`;
  an empty `deps.StencilsDir`.
  Each is a wiring bug at the one call site, surfaced loudly.
  Every runtime failure below returns a **nil** error and a `Report` instead, per the decision that the reflection step can never change the run's outcome.

  **Scan.** Read `deps.FrictionDir`.
  A directory that does not exist, a directory with zero entries, a directory whose entries are all non-`.md`, and a directory whose only `.md` entry is `friction.ReportFileName` all yield the same result: a `logger.Info` naming the outcome, a `Report{Status: StatusSkipped, NoteCount: 0}`, a nil error, and **no spawn at all**.
  A clean run produces no notes and is the common case;
  spawning a full LLM session to conclude "nothing to report" on every clean run is pure cost.
  The note set is every `*.md` entry **except** the one whose name equals `friction.ReportFileName`, sorted by name.
  That exclusion is what stops a half-written report from a timed-out run being counted as new friction on the next scan.
  A directory that exists but cannot be read is a runtime failure: `logger.Warn`, `Report{Status: StatusFailed}`, nil error.

  **Stale-report delete.** Before composing the spec, delete any existing file at `filepath.Join(deps.FrictionDir, friction.ReportFileName)`.
  This is not optional: `shuttleengine.Spec.validate` rejects an `OutputFiles` entry that already exists, so without the delete a timed-out run leaves a file that makes every subsequent `Reflect` fail at spec validation rather than at the agent.
  A delete failure is a runtime failure: `logger.Warn`, `Report{Status: StatusFailed}`, nil error.

  **Spawn.** Build the spec via `buildReflectionSpec`, log the spawn at `Info` via `internal/logger` per the Live-Substrate Spawn Observability invariant, and run it through `deps.Shuttle.Run`.
  A non-nil error, `shuttleengine.OutcomeDied`, `shuttleengine.OutcomeTimeout`, and `shuttleengine.OutcomeAsking` are each a runtime failure: `logger.Warn` naming the outcome, `Report{Status: StatusFailed, NoteCount: <n>}`, nil error — **and the friction directory is left exactly as it is**, not archived.
  A failure means nothing was reflected on and nothing was filed, so archiving would discard the run's un-acted-on signal while creating no duplicate-filing risk to avoid, and the next trigger in the same task must reflect on those same notes.
  Only `shuttleengine.OutcomeDone` proceeds.

  **Archive.** On a clean return, compose the archive directory as `deps.ArchivePrefix + <compact UTC timestamp from the clock>`, declared with a package-local `archiveTimestampFormat` constant in this file, and `os.Rename` the friction directory onto it.
  Then call `friction.EnsureDir(deps.FrictionDir)` to recreate the directory empty — the one place that removed the directory is the cheapest, hardest-to-get-wrong place to put it back, rather than teaching every future caller to re-ensure it.
  A rename failure is a runtime failure: `logger.Warn`, `Report{Status: StatusFailed}`, nil error.

  On success return `Report{Status: StatusReflected, NoteCount: <n>, ReportPath: <the post-archive report path>}` and emit a `logger.Info` naming the outcome and that **post-archive** path — the rename has already happened by the time this logs, so the pre-archive path would be stale and point at nothing.
  The logger line is deliberately a second surface beside the envelope key batch 7 adds, because `lyx loom run` gives the detached `loom drive` process a driver-log file for its stdout and stderr, so drive's envelope reaches the driver log and nothing else.

  Never write `_lyx/loom/status.json`: it is `internal/state`'s serialized Shed status with a fixed schema and is not a third surface for this.
- **Commit:** `feat(frictionengine): add Reflect, the scan-skip-spawn-archive step`

### Card 12: `frictionengine` tests and its Told-Geometry seam guard

- **Context:**
  - `internal/mergeresolve/seam_enforcement_test.go`
  - `internal/mergeresolve/mergeresolve_test.go`
  - `internal/frictionengine/reflect.go`
  - `internal/frictionengine/deps.go`
  - `internal/frictionengine/spec.go`
  - `internal/friction/friction.go`
  - `contracts/stencils/friction/friction-template-reflection.md`
- **Edits:** none
- **Creates:**
  - `internal/frictionengine/reflect_test.go`
  - `internal/frictionengine/seam_enforcement_test.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  `seam_enforcement_test.go` is modelled directly on `internal/mergeresolve/seam_enforcement_test.go`: an allowlist walk of every non-test `.go` file in the package using `go/parser` with `parser.ImportsOnly`, admitting only stdlib plus the six allowed internal packages, so `internal/lyxcwd` and anything else that would drag geometry resolution in fails the test with no list maintenance.

  `reflect_test.go` drives everything through a fake `Shuttle` seam, a fake `Clock`, and a `t.TempDir()` friction directory, following the fake-seam pattern `internal/burlerengine`'s and `internal/shedadapters`' own tests use.
  Every test in it is untagged Tier 1: no `exec.Command`, no `gitexec`, no real spawn, no `time.Sleep` at or above one second.
  Cover:

  - Missing directory -> no spawn, `Report.Status` is `StatusSkipped`, nil error.
  - Directory present but empty -> same skipped result, no spawn.
  - Directory containing only non-`.md` files -> same skipped result, no spawn.
  - Directory containing only `friction.ReportFileName` and no other `.md` -> skipped, **no spawn**.
    This is the stale-report-from-a-timed-out-run case and it must not be mistaken for one note.
  - One note present -> exactly one spawn, and the composed `Spec` carries the configured model, effort, and timeout, `Interactive: false`, `ForkSubagents: false`, `Role: "friction"`, and a non-empty `OutputFiles`.
  - The spawn's `Prompt` names the friction directory.
  - After a successful spawn, the directory is archived to the exact timestamped sibling the injected clock implies **and recreated empty** — assert both the archive's existence and that the original path exists and is empty.
    A second `Reflect` against the same location then reports skipped rather than re-spawning.
  - A `Shuttle` error, `shuttleengine.OutcomeDied`, `shuttleengine.OutcomeTimeout`, and `shuttleengine.OutcomeAsking` each yield `Report.Status == StatusFailed` with a **nil** error, **and the friction directory is left exactly as it was** — assert the original path still exists with its notes and that no timestamped archive sibling was created.
    Assert both halves: a runtime failure leaking out as a non-nil error would defeat the decision that the reflection step can never change the run's outcome.
  - A stale `friction.ReportFileName` present alongside a real note before `Reflect` runs is deleted before the spec is composed, so the composed `Spec.OutputFiles` entry does not already exist.
  - A malformed `Deps` is the **only** case yielding a non-nil error: assert one case per validated field — a nil `Shuttle`, an empty `FrictionDir`, a relative `FrictionDir`, an empty `ArchivePrefix`, and an empty `StencilsDir`.
- **Commit:** `test(frictionengine): cover the skip, spawn, archive, and failure paths`

## Batch Tests

`verify: go test ./internal/frictionengine/... ./contracts/stencils/...` covers both surfaces.

`./internal/frictionengine/...` runs `reflect_test.go` (every branch of the scan, the stale-report delete, the spec's shape, the archive-and-recreate, the three runtime-failure paths that must not archive, and the five `Deps`-validation errors) and `seam_enforcement_test.go` (the Told-Geometry import allowlist).

`./contracts/stencils/...` is included because card 10 pins `friction-template-reflection`'s three marker names (`friction_dir`, `report_path`, `note_list`) from the Go side;
the stencils package's own `registry_test.go` and `rubric_test.go` are what catch a stencil file edited out of step with them.

The scope is per-batch, not whole-repo: this batch creates one new package and edits no existing one, so nothing outside these two trees can regress from it.
