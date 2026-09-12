# Batch: compose-burler

```yaml
task: 'self-report Tier 2: per-agent friction notes for unsupervised runs'
batch: 'compose-burler'
number: 5
cards: 3
verify: go test ./internal/burlerengine/... ./internal/shedadapters/... ./internal/burlercli/... ./contracts/stencils/...
depends-on: [1]
```

## Batch Scope

This batch injects the friction directive into the Burler review+fix round: the marker goes into `burler-step-1-explore`, the engine gains a told `frictionDir` on its constructor and a told `NoteID` on its per-round options, and `internal/shedadapters.BurlerProducer` composes the note stem.

It depends on batch 1 only.
`burlerengine` must not import `loomengine` — it does not today and must not start — so the friction directory is **told**, exactly as the discussion specifies.
`hubgeom.BurlerGeometry` carries only `WorktreeRoot` and `AnchorPath` (`internal/burlerengine/geometry.go:12-22`), so it is not the seam either.

`burlerengine.New`'s signature changes, so this batch also updates its two other call sites, in `internal/burlercli/wiring.go`.
Both `burlercli` call sites and `internal/loomcli/wiring.go`'s call site pass the empty string here;
`loomcli`'s is replaced with the real resolved value in batch 7, which depends on this batch.
`burlercli`'s two stay empty permanently: standalone and hub `lyx burler` runs are not loom runs and have no friction directory.

The external interface batch 7 consumes is `burlerengine.New`'s fifth parameter.

## Cards

### Card 16: add `{{.friction_directive}}` to `burler-step-1-explore`

- **Context:**
  - `contracts/stencils/burler/burler-step-3-fix.md`
  - `contracts/stencils/friction/friction-directive-review-fix.md`
  - `internal/stencil/stencil.go`
- **Edits:**
  - `contracts/stencils/burler/burler-step-1-explore.md`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  Add the literal marker `{{.friction_directive}}` on its own line to `contracts/stencils/burler/burler-step-1-explore.md`, immediately after the existing `{{.pattern_directive}}` line at `:11`.

  Update the file's opening HTML banner comment — which today counts the required markers and names `pattern_directive` as the fifth, optional one — to name `friction_directive` as a sixth marker, also optional, also filled via `stencil.FillOptional`, rendering as nothing when Tier 2 is off.
  Keep the banner's existing statement that the file carries no `{{if}}`/`{{range}}` conditionals, and do not introduce one.

  Instruction 1 is the placement rather than instruction 3, even though the note is written at the end of a round, because instruction 3 is filled with plain `stencil.Fill` (`internal/burlerengine/prompt.go:96`) and choosing it would add a fourth `Fill` -> `FillOptional` conversion beyond the three the discussion counts.
  The same session reads instruction 1 and later instruction 3, so the directive still reaches the agent that writes the note.
- **Commit:** `feat(stencils): add the friction_directive marker to burler's explore instruction`

### Card 17: told `frictionDir` on `New`, told `NoteID` on `RunOpts`, directive in `composePrompt`

- **Context:**
  - `internal/friction/friction.go`
  - `internal/burlerengine/geometry.go`
  - `internal/burlerengine/config.go`
  - `internal/stencil/stencil.go`
  - `internal/pattern/pattern.go`
  - `internal/logger/logger.go`
- **Edits:**
  - `internal/burlerengine/engine.go`
  - `internal/burlerengine/engine_test.go`
  - `internal/burlerengine/profile.go`
  - `internal/burlerengine/prompt.go`
  - `internal/burlercli/wiring.go`
  - `internal/loomcli/wiring.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  **`internal/burlerengine/profile.go`.**
  Add a `NoteID string` field to `RunOpts`, documented as the caller-supplied friction-note **stem** for this round — unique across sites, with `friction.NotePath` supplying uniqueness across invocations of the same site.
  An empty `NoteID` means no note path is composed for this round, exactly as an empty friction directory does.

  **`internal/burlerengine/engine.go`.**
  `New` gains a fifth parameter, `frictionDir string`, stored on a new unexported `frictionDir` field of `Engine` beside the existing `stencilsDir` field.
  Document in `New`'s doc comment why it is told rather than derived: `burlerengine` must not import `loomengine`, and `burlerengine.Geometry` is `internal/hubgeom`/`internal/standalonegeom`'s to construct under the Told-Geometry Invariant, so an explicit constructor parameter is the remaining told seam.
  An empty value means Tier 2 is off for this engine.

  In `Engine.Run`, immediately after the existing `pattern.Directive` call at `internal/burlerengine/engine.go:104`, resolve `notePath := friction.NotePath(e.frictionDir, opts.NoteID)` and `frictionDirective, err := friction.Directive(notePath, e.stencilsDir, friction.RoleReviewFix)`.
  A non-nil error here is **swallowed, never returned**: log it at `Warn` via `internal/logger` naming the role and the stencil, and continue with an empty directive string, exactly as if Tier 2 were off.
  Do **not** wrap it as `fmt.Errorf("burler: %w", err)` the way the `pattern.Directive` error one line above is wrapped — that error propagates out of `Run` and is treated as a full task failure by the calling producer, which would kill the run over optional bookkeeping.
  The `pattern.Directive` call itself keeps its existing propagating handling unchanged;
  see the overview's "a composer swallows a `friction.Directive` error" Shared Decision for why the two adjacent calls are deliberately not symmetrical.
  Pass `frictionDirective` into `composePrompt` as a new parameter beside `patternDirective`.

  **`internal/burlerengine/prompt.go`.**
  `composePrompt` gains a `frictionDirective string` parameter, adds `friction.MarkerName` to `instruction1Values`, calls `friction.WarnIfMarkerAbsent(instruction1Template, "burler-step-1-explore", frictionDirective)` after reading that template and before filling it, and widens the existing `stencil.FillOptional` optional-names slice at `internal/burlerengine/prompt.go:68` from `[]string{"pattern_directive"}` to `[]string{"pattern_directive", friction.MarkerName}`.
  No `Fill` -> `FillOptional` conversion is needed in this file.

  **Call sites.**
  Update all three `burlerengine.New` call sites to pass a fifth argument:
  `internal/burlercli/wiring.go:131` and `internal/burlercli/wiring.go:189` pass `""` with a one-line comment stating that a `lyx burler` run is not a loom run and has no friction directory in either mode;
  `internal/loomcli/wiring.go:266` passes `""` for now with a one-line comment stating that the real resolved value is wired in the same change that resolves it for the other two engines.
  All three are updated here rather than in batch 7 so the signature change and every caller land together and the tree compiles at this batch's boundary.
- **Commit:** `feat(burlerengine): inject the friction directive into the review+fix round`

### Card 18: `shedadapters` composes the note stem, and the round's tests

- **Context:**
  - `internal/burlerengine/engine.go`
  - `internal/burlerengine/profile.go`
  - `internal/burlerengine/prompt.go`
  - `internal/friction/friction.go`
  - `contracts/stencils/burler/burler-step-1-explore.md`
- **Edits:**
  - `internal/shedadapters/burler.go`
  - `internal/shedadapters/burler_test.go`
  - `internal/burlerengine/prompt_test.go`
  - `internal/burlerengine/template_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  **`internal/shedadapters/burler.go`.**
  At every point where `BurlerProducer` overwrites its `opts` template's `Round` field per attempt, also set `opts.NoteID` to `"burler-" + filepath.Base(p.runDir) + "-r" + strconv.Itoa(round)`.
  `p.runDir` is the segment's run directory, whose base is that segment's `run_subdir` recipe value (`webster`, `plan`, `discussion`), so the composed stem distinguishes `Plan-Burler` round 3 from `Webster-Burler` round 3 rather than letting them collide in one directory.
  `BurlerProducer` is the layer that holds both halves — `runDir` at `internal/shedadapters/burler.go:68` and the round number in its own loop — which is why the stem is composed here and told rather than derived inside `burlerengine` from `Profile.ReviewPath`'s directory shape.
  Do not set `NoteID` on the `shuttleengine.Spec` built inside `probeLiveRound`: that spec is an attach probe matching on `OutputFiles`, not a round dispatch.

  **`internal/shedadapters/burler_test.go`.**
  Add one test asserting the composed `NoteID` for a known `runDir` and round — that `runDir` ending in `webster` at round 3 yields `burler-webster-r3` — so the stem's shape is pinned rather than incidental.

  **`internal/burlerengine/prompt_test.go` and `internal/burlerengine/template_test.go`.**
  Extend both with the same three cases the other composer batches use, in each file's own existing style:

  1. **Enabled.** With a non-empty `frictionDir` and a non-empty `opts.NoteID`, instruction 1 contains the resolved absolute note path verbatim.
  2. **Disabled.** With an empty `frictionDir`, the round composes successfully, instruction 1 carries no directive text, and no friction stencil is read.
  3. **Marker-free template.** A seeded `burler-step-1-explore` whose bytes carry no `{{.friction_directive}}` literal still composes successfully while Tier 2 is enabled — never an error.

  `internal/burlerengine/template_test.go` additionally extends its existing required-marker deletion sweep to exclude `friction_directive` alongside `pattern_directive`, since both are optional markers and the sweep asserts that each **required** marker's absence fails.

  Keep every added test untagged Tier 1.
- **Commit:** `feat(shedadapters): tell burler its per-round friction note stem`

## Batch Tests

`verify: go test ./internal/burlerengine/... ./internal/shedadapters/... ./internal/burlercli/... ./contracts/stencils/...` covers every package this batch writes to.

`./internal/burlerengine/...` runs the extended `prompt_test.go` and `template_test.go` (enabled, disabled, marker-free, and the widened optional-marker sweep) plus the package's existing round and profile tests, which is what catches the `New` and `RunOpts` signature changes breaking an existing construction.

`./internal/shedadapters/...` runs the new `NoteID`-composition assertion plus the existing Burler producer tests.

`./internal/burlercli/...` is in scope because this batch edits `internal/burlercli/wiring.go`'s two `burlerengine.New` call sites;
it is the package that would fail to compile if the fifth argument were added inconsistently.

`./contracts/stencils/...` covers the edited `burler-step-1-explore.md` through `registry_test.go` and `rubric_test.go`.

`internal/loomcli/wiring.go`'s call site is also edited here (it must be, or the tree does not compile once the signature changes), but `./internal/loomcli/...` is deliberately **not** in this batch's verify scope: the edit there is a literal `""` argument with no behaviour of its own, and that package's own friction wiring and tests are batch 7's subject.
The cross-package compile gate for this batch's signature change is the overview's module-wide `verify: go build ./...`.
