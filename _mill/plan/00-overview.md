# Plan: burlerengine + perchengine told-geometry

```yaml
task: "burlerengine + perchengine told-geometry"
slug: "burler-perch-told-geometry"
approved: true
started: "20260818-084814"
parent: "standalone-producers"
root: ""
verify: go build ./...
```

## Batch Index

_The fenced yaml block below is the authoritative DAG mill-go reads to schedule batches.
Every batch lives at `NN-<batch-slug>.md` in this directory and is mirrored as one entry here._

```yaml
batches:
  - number: 1
    name: geometry types and hubgeom tellers
    file: 01-geometry-types-and-hubgeom-tellers.md
    depends-on: []
    verify: go test ./internal/hubgeom/... ./internal/burlerengine/... ./internal/perchengine/...
  - number: 2
    name: burler told-geometry
    file: 02-burler-told-geometry.md
    depends-on: [1]
    verify: go test ./internal/hubgeom/... ./internal/burlerengine/... ./internal/burlercli/... ./internal/perchcli/... && go vet -tags smoke ./internal/burlerengine/...
  - number: 3
    name: perch told-geometry
    file: 03-perch-told-geometry.md
    depends-on: [2]
    verify: go test ./internal/hubgeom/... ./internal/perchengine/... ./internal/perchcli/... ./internal/shedadapters/... ./cmd/lyx/... && go test -tags integration ./internal/perchcli/...
```

## Shared Decisions

### Decision: told-geometry carrier is a package-owned `Geometry` struct per engine

- **Decision:** `burlerengine.Geometry{WorktreeRoot, AnchorPath string}` and `perchengine.Geometry{GateDir, AnchorPath string}`, each declared by its own engine in its own `geometry.go`, mirroring `internal/reedengine/geometry.go`.
  `hubgeom.BurlerGeometry(l)` and `hubgeom.PerchGeometry(l)` are the hub-mode tellers, siblings of the shipped `hubgeom.ReedGeometry`.
- **Rationale:** `internal/hubgeom/doc.go` (already on `main`) states the contract outright and names this task as the one adding both functions.
  Named struct fields make the one silent failure mode — a swapped anchor/worktree pair — visible at every call site; two loose positional strings would compile either way.
- **Applies to:** all batches

### Decision: field names copy `reedengine.Geometry` exactly

- **Decision:** the anchor-side field is `AnchorPath` (never `AnchorRoot`) in both new structs; burler's worktree-side field is `WorktreeRoot`; perch's is `GateDir`.
- **Rationale:** `reedengine/geometry.go` ships the asymmetric `AnchorPath`/`WorktreeRoot` pairing and `hubgeom.ReedGeometry` fills `AnchorPath` from `l.AnchorPath()`; copying a pattern while renaming its fields makes a reader check whether the two values are the same thing.
  The Cwd Resolution Invariant also states that `root` always means the git worktree/repo root, and the anchor is a worktree subdirectory whenever `AnchorRel != "."` — so `AnchorRoot` would name a non-root `root`.
  Perch's field is `GateDir` because its only consumer is `treadleengine.Profile.GateDir`, the gate command's working directory.
- **Applies to:** all batches

### Decision: behaviour preservation is the contract — every path resolves byte-identically

- **Decision:** no path this task touches may resolve anywhere different before and after.
  The CLI construction sites keep passing `layout.WorktreePath()` / `layout.AnchorPath()` values, now routed through `hubgeom`.
  No `--target-dir`, no `--stencils-dir`, no branch around `lyxcwd.Resolve` — that is task T8.
- **Rationale:** this is a type-coupling change, not a behaviour change; the existing suites are the primary evidence and a test that only changes shape must keep its original assertion intact.
- **Applies to:** all batches

### Decision: `stencilsDir` stays a separate parameter, out of both `Geometry` structs

- **Decision:** `burlerengine.New(shuttle Shuttle, geom Geometry, cfg Config, stencilsDir string)` keeps `stencilsDir` as its own trailing parameter; `perchengine.Engine.Run(p, runDir, scratchDir, stencilsDir string)` is untouched.
- **Rationale:** perch takes `stencilsDir` at `Run` time, not at construction, so a symmetric `Geometry.StencilsDir` is impossible for perch and would make the two structs disagree for no gain.
  `stencilsDir` is also flag-overridable in both modes once T8 lands; geometry is structural, config-shaped values are not.
- **Applies to:** all batches

### Decision: constructors keep their positional shape

- **Decision:** `layout` becomes `geom` in place — `burlerengine.New(shuttle, geom, cfg, stencilsDir)` and `perchengine.New(burler, shuttle, cfg, geom, opts)`.
  No options-struct rewrite for either engine.
- **Rationale:** smallest reviewable diff; every existing call site and test changes in exactly one argument position, which is what makes a "nothing resolves anywhere different" contract auditable.
- **Applies to:** all batches

### Decision: the told direction stays one-way

- **Decision:** `internal/hubgeom` imports the engines; no engine may import `hubgeom` from production code.
  External `_test` packages (`package burlerengine_test`) are exempt and may import it.
- **Rationale:** stated by `internal/hubgeom/doc.go` and the reason `hubgeom` exists as a separate package at all.
  An external test package importing `hubgeom` creates no import cycle.
- **Applies to:** all batches

### Decision: `Engine` stores the whole `Geometry` value

- **Decision:** both engines store `geom Geometry` as one field, not unpacked strings.
  `perchengine.Engine` therefore carries an `AnchorPath` it does not read today.
- **Rationale:** `reedengine.Engine` does the same, and the struct is the told-geometry unit — unpacking it on construction reintroduces the swap hazard one layer in.
  Perch's unread `AnchorPath` is the caller's `RunsDir`/`ScratchDir` base carried alongside, so the two roots stay visible together at every perch call site.
- **Applies to:** all batches

### Decision: no import-allowlist enforcement test is added here

- **Decision:** this task adds no test pinning "`internal/lyxcwd` is absent from `burlerengine`'s and `perchengine`'s production imports".
  The property is verified once, by the grep step named in each batch's `## Batch Tests`, and left unguarded thereafter.
- **Rationale:** task T10 already owns this question and answers it for every converted package at once, with its enforcement basis named honestly.
  Deciding it here for two packages is what would produce the inconsistency: `reedengine` made this exact conversion in wave 1 and got no guard, and still names `lyxcwd` in prose comments that a naive grep-based guard would flag.
- **Applies to:** all batches

### Decision: no shared-doc edits outside the touched modules

- **Decision:** `CONSTRAINTS.md`, `docs/overview.md`, `manifest/roadmap.md`, and `manifest/designs/producers-standalone.md` are not edited by this task.
  `internal/shedadapters` is not edited either.
- **Rationale:** no invariant is falsified (hub mode still joins `perchDirName` onto `AnchorPath()`, now via `hubgeom`); no module is added or removed; the wave-3 roadmap item covers this task and T7 together, so the wave-closing task performs the single move.
  `shedadapters` is already fully told and its one `perchengine.New` doc mention is about `Options.PauseRequested`, naming no geometry.
- **Applies to:** all batches

### Decision: doc comments are edited wherever this change falsifies them

- **Decision:** `doc.go` **and** file-header and function comments in every edited production file are corrected in the same card as the code they describe.
- **Rationale:** the project Documentation Lifecycle requires docs in the same commit, and the falsified statements here are mostly file-header prose (`perchengine/engine.go:11`, `burlerengine/prompt.go:10`, `hubgeom/hubgeom.go:1-2`) rather than `doc.go` bodies.
  `internal/burlerengine/doc.go` was checked and names no `layout` field or `*lyxcwd.Location` — it needs no edit.
- **Applies to:** all batches

## All Files Touched

- `cmd/lyx/constructoranchoring_test.go`
- `cmd/lyx/notransients_test.go`
- `internal/burlercli/cli.go`
- `internal/burlerengine/engine.go`
- `internal/burlerengine/engine_test.go`
- `internal/burlerengine/geometry.go`
- `internal/burlerengine/prompt.go`
- `internal/burlerengine/smoke_cluster_test.go`
- `internal/burlerengine/smoke_round_test.go`
- `internal/hubgeom/doc.go`
- `internal/hubgeom/hubgeom.go`
- `internal/hubgeom/hubgeom_test.go`
- `internal/perchcli/cli.go`
- `internal/perchcli/cli_integration_test.go`
- `internal/perchcli/run.go`
- `internal/perchcli/run_integration_test.go`
- `internal/perchengine/doc.go`
- `internal/perchengine/engine.go`
- `internal/perchengine/geometry.go`
- `internal/perchengine/identity.go`
- `internal/perchengine/identity_test.go`
- `internal/perchengine/run_test.go`
