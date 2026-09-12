# Plan: self-report Tier 2: per-agent friction notes for unsupervised runs

```yaml
task: 'self-report Tier 2: per-agent friction notes for unsupervised runs'
slug: 'self-report-tier2'
approved: false
discussion_sha: 'e045dc6c6fa8df34555d31ef86f42fa8a9f43d34'
started: '20260912-105329'
parent: 'main'
root: ""
verify: go build ./...
```

## Batch Index

_The fenced yaml block below is the authoritative DAG mill-go reads to schedule batches.
Every batch lives at `NN-<batch-slug>.md` in this directory and is mirrored as one entry here._

```yaml
batches:
  - number: 1
    name: friction-leaf
    file: 01-friction-leaf.md
    depends-on: []
    verify: go test ./internal/friction/... ./contracts/stencils/...
  - number: 2
    name: loom-paths-and-config
    file: 02-loom-paths-and-config.md
    depends-on: []
    verify: go test ./internal/loomengine/... ./cmd/lyx/... ./internal/configreg/...
  - number: 3
    name: frictionengine
    file: 03-frictionengine.md
    depends-on: [1]
    verify: go test ./internal/frictionengine/... ./contracts/stencils/...
  - number: 4
    name: compose-loom
    file: 04-compose-loom.md
    depends-on: [1, 2]
    verify: go test ./internal/loomengine/... ./contracts/stencils/...
  - number: 5
    name: compose-burler
    file: 05-compose-burler.md
    depends-on: [1]
    verify: go test ./internal/burlerengine/... ./internal/shedadapters/... ./internal/burlercli/... ./contracts/stencils/...
  - number: 6
    name: compose-webster
    file: 06-compose-webster.md
    depends-on: [1]
    verify: go test ./internal/websterengine/... ./contracts/stencils/...
  - number: 7
    name: wiring
    file: 07-wiring.md
    depends-on: [2, 3, 4, 5, 6]
    verify: go test ./internal/loomcli/... ./internal/webstercli/... ./internal/burlercli/...
  - number: 8
    name: docs
    file: 08-docs.md
    depends-on: [7]
    verify: go test ./internal/lyxcwd/...
```

## Shared Decisions

### Decision: "Tier 2 off" is an empty path string, never a boolean

- **Decision:** the single resolved value every layer carries is the friction directory (or, at the composer boundary, the composed note path).
  Empty means Tier 2 is off.
  `friction.Directive` returns `("", nil)` with no stencil read attempted for an empty note path, and `friction.NotePath` returns `""` for an empty friction directory, so "off" propagates through both calls as an empty string.
  No engine holds a separate enabled flag.
- **Rationale:** two values that can contradict each other is the failure this avoids;
  it mirrors `internal/pattern.Directive`'s own empty-`anchorPath` case (`internal/pattern/pattern.go:78-84`).
- **Applies to:** all batches

### Decision: seven composers, and only the three named ones convert `Fill` -> `FillOptional`

- **Decision:** the `{{.friction_directive}}` marker is injected at seven composers — Discussion-Write (`internal/loomengine/prompt.go`), Plan-Write (`internal/loomengine/plan.go`), the Burler round's instruction 1 (`internal/burlerengine/prompt.go`), and all four `internal/websterengine/render.go` composers.
  Exactly three of them need converting from `stencil.Fill` to `stencil.FillOptional`: `internal/loomengine/prompt.go:30`, `internal/websterengine/render.go:162` (fork), and `internal/websterengine/render.go:225` (integration).
  The other four already call `FillOptional` for `pattern_directive` and only gain the new name in their optional-names slice.
- **Rationale:** `stencil.Fill` is literally `FillOptional(template, values, nil)` (`internal/stencil/stencil.go:20-22`), so each conversion is a one-line change plus the optional-names argument.
- **Applies to:** 04-compose-loom, 05-compose-burler, 06-compose-webster

### Decision: the Burler round's friction marker goes in instruction 1, beside `pattern_directive`

- **Decision:** the burler round's `{{.friction_directive}}` marker is added to `contracts/stencils/burler/burler-step-1-explore.md`, the same instruction file that already carries `pattern_directive`, and is filled through the same `stencil.FillOptional` call at `internal/burlerengine/prompt.go:68`.
- **Rationale:** instruction 3 (fix) is the round's last-read file and would be the marginally better placement for a note written at the end of a round, but instruction 3 is filled with plain `stencil.Fill` and choosing it would add a fourth `Fill` -> `FillOptional` conversion the discussion's decision explicitly counts at three.
  Instruction 1 is read by the same session that later runs instruction 3, so the directive still reaches the agent that writes the note.
- **Applies to:** 05-compose-burler

### Decision: `websterengine` carries `FrictionDir` on three Deps structs, not one

- **Decision:** `FrictionDir string` is added to `websterengine.RunDeps`, `websterengine.BeginDeps`, and `websterengine.RecoverDeps` — not to `RunDeps` alone, and not to `websterengine.Geometry`.
- **Rationale:** `_mill/discussion.md`'s "How the friction directory reaches each of the three engines" decision names four call sites — `beginbatch.go:311`, `recoverbatch.go:154`, `runlevel.go:548`, `runlevel.go:564` — and says each reads `RunDeps.FrictionDir`.
  Two of those four do not have a `RunDeps` in scope: `beginbatch.go` takes `BeginDeps` (`internal/websterengine/beginbatch.go:74`) and `recoverbatch.go` takes `RecoverDeps` (`internal/websterengine/recoverbatch.go:52`).
  The discussion's reason for keeping the value off `Geometry` (hubgeom/standalonegeom are the Told-Geometry Invariant's only `Geometry` constructors, and this value needs no geometry derivation) holds unchanged, so the field is replicated across the three Deps structs the four call sites actually receive.
- **Applies to:** 06-compose-webster, 07-wiring

### Decision: `internal/webstercli` resolves the friction directory in hub mode, tolerantly

- **Decision:** `internal/webstercli`'s hub wiring (`wireHub`) resolves the friction directory exactly as `internal/loomcli/wiring.go` does — `loomengine.LoomFrictionDir(loc)` when a tolerantly-loaded `loom.yaml` reports a non-empty `friction` key, `""` otherwise — and fills it into `BeginDeps`, `RecoverDeps`, and `RunDeps`.
  A `loomengine.LoadConfig` failure logs at `Warn` and yields `""` rather than failing the webster verb.
  `wireStandalone` leaves the value `""` unconditionally.
- **Rationale:** `contracts/stencils/webster/webster-template-master.md` drives the batch loop by having Master shell out to `lyx webster begin-batch <NN>` and `lyx webster recover-batch <NN>`, so the implementer fork's and the recovery strand's prompts are composed inside a **separate `lyx webster` process** whose Deps are built by `internal/webstercli/wiring.go`, not by `internal/loomcli/wiring.go`.
  Filling only `loomcli`'s `RunDeps` would give the directive to the Master and integration prompts and to nothing else — the per-batch implementer fork, the most common agent in a run, would never receive it, and Tier 2 would produce zero notes from the one place it matters most.
  `internal/webstercli` already imports several non-webster engines (`fabricengine`, `shuttleengine`, `reedengine`, `batcher`, `planparser`), so a `loomengine` import is not a new CLI/Cobra Invariant deviation;
  the hub-branch-only scoping is what keeps standalone webster free of loom.
- **Applies to:** 07-wiring

### Decision: `burlerengine.New` gains a told `frictionDir`, and `RunOpts` gains a told `NoteID`

- **Decision:** `burlerengine.New` takes a fifth parameter, `frictionDir string`, and `burlerengine.RunOpts` gains a `NoteID string` field the caller fills per round.
  `internal/shedadapters.BurlerProducer` composes `NoteID` as `"burler-" + filepath.Base(p.runDir) + "-r" + strconv.Itoa(round)`.
- **Rationale:** `_mill/discussion.md` says the value is "told on `burlerengine`'s constructor's config", but `burlerengine.Config` (`internal/burlerengine/config.go:39`) is `burler.yaml`'s strict decode shape — a field there would become an operator-settable YAML key — and `burlerengine.Geometry` is `hubgeom`/`standalonegeom`'s to construct under the Told-Geometry Invariant.
  An explicit constructor parameter is the remaining told seam.
  `NoteID` exists because the discussion's stem for this site is "the run subdir plus the round number", and `burlerengine` is told neither: `internal/shedadapters.BurlerProducer` holds `runDir` (`internal/shedadapters/burler.go:68`) and the round number, so it composes the stem and tells it.
- **Applies to:** 05-compose-burler, 07-wiring

### Decision: an absent `{{.friction_directive}}` marker warns from inside the leaf

- **Decision:** `internal/friction` exports one helper that takes the template bytes, the stencil's registered name, and the resolved note path, and emits a `logger.Warn` naming the stencil and pointing at `lyx stencil diff` / `lyx stencil sync` when the note path is non-empty and the literal `{{.friction_directive}}` is absent from the bytes.
  The seven composers call the helper and continue;
  none of them logs.
- **Rationale:** `internal/stencilstore/reconcile.go` never refreshes a `StateEdited` stencil and, in dev mode, does not refresh a `StateUntouched` one either, so an existing worktree can hold templates with no marker;
  `stencil.FillOptional`'s guarantee is one-directional (it checks markers with no value, never a value with no marker), so a directive computed but never rendered would be dropped silently and Tier 2 would produce zero notes forever with no signal.
  Logging inside the helper is what makes `internal/logger` a member of the Friction Leaf Invariant's allowlist, and `internal/friction` already pulls `logger` transitively through `internal/stencilstore`.
- **Applies to:** all batches

### Decision: every Tier 2 failure is a `Warn`, never a run failure

- **Decision:** a failed `os.MkdirAll` of the friction directory, a failed stencil read behind an already-composed prompt, a `Shuttle` error, `shuttleengine.OutcomeDied`, `shuttleengine.OutcomeTimeout`, and an unreadable friction directory all log at `Warn` via `internal/logger` and leave the run's own outcome untouched.
  The one exception is a malformed `frictionengine.Deps`, which is a wiring bug and returns a non-nil error.
- **Rationale:** the run's outcome is about the task's work;
  failing a successful, already-merged run because an optional bookkeeping agent timed out is strictly worse than filing nothing.
- **Applies to:** all batches

### Decision: documentation lands with the task, not with a single card

- **Decision:** `CONSTRAINTS.md`'s new Friction Leaf Invariant lands in batch 1 (with the package it constrains), and `docs/overview.md`, `manifest/roadmap.md`, and `manifest/designs/self-report-tier2.md` land in batch 8.
- **Rationale:** `CLAUDE.md`'s "Task completion — docs land in the same commit" rule is satisfied at task granularity: the task branch is squash-merged to `main` as one commit, and mill-plan's own "never require two separately-numbered cards to land in the same commit" principle forbids folding a later card's diff into an earlier card's pushed commit.
- **Applies to:** all batches

### Decision: `manifest/designs/self-report-tier2.md` is updated, not deleted

- **Decision:** the design doc stays on disk with its Status line flipped to shipped and its three Open questions closed in place.
- **Rationale:** `docs/overview.md#documentation-lifecycle` says a module-design doc is deleted when its module lands, but two sibling design docs link to this one — `manifest/designs/self-report-tier1.md:31` and `manifest/designs/loom-step.md:32` — and the Markdown Link Integrity invariant requires every inline link under `manifest/` to resolve.
  Deleting the file would break both links;
  closing the questions in place is what `_mill/discussion.md`'s own Scope section specifies.
- **Applies to:** 08-docs

### Decision: verify commands are native `go test`, scoped per batch

- **Decision:** every batch's `verify:` is a plain `go test ./internal/<pkg>/...` invocation with no `PYTHONPATH= ` prefix, scoped to the packages that batch edits.
  The overview's module-wide `verify:` is `go build ./...`.
- **Rationale:** this is a Go repository, so the `PYTHONPATH= ` isolation prefix does not apply.
  The module-wide `go build ./...` is the cheap cross-package gate that catches a signature change in one batch breaking a caller in a package that batch does not test —
  `burlerengine.New` and the four `render.go` composers both change signature, and a compile break is the failure mode that matters there.
  Whole-repo regression coverage is already the job of `pipeline.done_gate`, which this hub sets to `go test ./... && go test -tags integration ./...`.
- **Applies to:** all batches

## All Files Touched

_Full union of every `Creates:` / `Edits:` / `Moves:` **target** path across every batch, sorted alphabetically._

- `CONSTRAINTS.md`
- `cmd/lyx/notransients_test.go`
- `contracts/stencils/burler/burler-step-1-explore.md`
- `contracts/stencils/friction/friction-directive-implementer.md`
- `contracts/stencils/friction/friction-directive-interview.md`
- `contracts/stencils/friction/friction-directive-orchestrator.md`
- `contracts/stencils/friction/friction-directive-review-fix.md`
- `contracts/stencils/friction/friction-template-reflection.md`
- `contracts/stencils/loom/loom-template-discussion.md`
- `contracts/stencils/loom/loom-template-plan.md`
- `contracts/stencils/rubric_test.go`
- `contracts/stencils/stencils.go`
- `contracts/stencils/webster/webster-prefix-fork.md`
- `contracts/stencils/webster/webster-prefix-recovery.md`
- `contracts/stencils/webster/webster-template-integration.md`
- `contracts/stencils/webster/webster-template-master.md`
- `docs/overview.md`
- `internal/burlercli/wiring.go`
- `internal/burlerengine/engine.go`
- `internal/burlerengine/profile.go`
- `internal/burlerengine/prompt.go`
- `internal/burlerengine/prompt_test.go`
- `internal/burlerengine/template_test.go`
- `internal/friction/doc.go`
- `internal/friction/friction.go`
- `internal/friction/friction_test.go`
- `internal/friction/leaf_enforcement_test.go`
- `internal/friction/notepath_test.go`
- `internal/frictionengine/deps.go`
- `internal/frictionengine/doc.go`
- `internal/frictionengine/reflect.go`
- `internal/frictionengine/reflect_test.go`
- `internal/frictionengine/seam_enforcement_test.go`
- `internal/frictionengine/spec.go`
- `internal/loomcli/drive.go`
- `internal/loomcli/friction_test.go`
- `internal/loomcli/run.go`
- `internal/loomcli/wiring.go`
- `internal/loomcli/wiring_test.go`
- `internal/loomengine/config.go`
- `internal/loomengine/config_test.go`
- `internal/loomengine/discussion.go`
- `internal/loomengine/friction_test.go`
- `internal/loomengine/plan.go`
- `internal/loomengine/plan_test.go`
- `internal/loomengine/prompt.go`
- `internal/loomengine/prompt_test.go`
- `internal/loomengine/template.yaml`
- `internal/shedadapters/burler.go`
- `internal/shedadapters/burler_test.go`
- `internal/webstercli/beginbatch.go`
- `internal/webstercli/cli.go`
- `internal/webstercli/recoverbatch.go`
- `internal/webstercli/run.go`
- `internal/webstercli/wiring.go`
- `internal/webstercli/wiring_test.go`
- `internal/websterengine/beginbatch.go`
- `internal/websterengine/recoverbatch.go`
- `internal/websterengine/render.go`
- `internal/websterengine/render_test.go`
- `internal/websterengine/runlevel.go`
- `internal/websterengine/template_test.go`
- `manifest/designs/self-report-tier2.md`
- `manifest/roadmap.md`
