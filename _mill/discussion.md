# Discussion: burlerengine + perchengine told-geometry

```yaml
task: burlerengine + perchengine told-geometry
slug: burler-perch-told-geometry
status: discussing
parent: standalone-producers
```

## Problem

`internal/burlerengine` and `internal/perchengine` are producers, but both still take a `*lyxcwd.Location` in their constructors.
A producer that holds a `Location` is holding an orchestrator's geometry object — `HubPath`, `RepoName`, an anchor-relative subpath — when the only thing it reads off it is two derived directory strings.
That type coupling is what blocks `lyx burler run` and `lyx perch run` from ever running outside a lyx worktree, and it is what the producers-standalone design's one rule forbids: *an orchestrator resolves geometry and requires it; a producer is told its paths and requires nothing.*

**Why now.** This is task **T6** of `manifest/designs/producers-standalone.md`, wave 3.
Waves 1 and 2 landed (`planparser`, `configengine.LoadOrTemplate`, `shuttleengine`/`reedengine`/`tokenvocab`, `pattern`, `internal/preflight`, `internal/buildinfo`, `internal/standalonestate`, and `internal/hubgeom` itself).
Wave 4's T8 — the standalone CLI path this whole line of work exists for — cannot start until burler and perch take told strings, because T8's pinned-values table has no parameter to pin values *to* until this task creates them.

Burler and perch are one task, not two: `perchengine` imports `burlerengine` directly and `perchcli` imports both, so changing `burlerengine.New` without a matching `perchengine`/`perchcli` change does not compile.

## Scope

**In:**

- `internal/burlerengine`: a package-owned `Geometry` type, `New` taking it instead of `*lyxcwd.Location`, `Engine.Run` reading the told roots. `internal/lyxcwd` leaves the package's production imports entirely.
- `internal/perchengine`: a package-owned `Geometry` type, `New` taking it instead of `*lyxcwd.Location`; `RunsDir`/`ScratchDir` taking a told `anchorRoot string`. `internal/lyxcwd` leaves the package's production imports entirely.
- `internal/hubgeom`: adds `BurlerGeometry(l)` and `PerchGeometry(l)` beside the existing `ReedGeometry`, plus their tests.
- `internal/burlercli/cli.go` and `internal/perchcli/cli.go` / `run.go`: construction sites repointed through `hubgeom`.
- Tests in all five packages, plus the `perchengine.RunsDir`/`ScratchDir` rows in `cmd/lyx/constructoranchoring_test.go` and `cmd/lyx/notransients_test.go`.
- `doc.go` updates in `burlerengine`, `perchengine`, and `hubgeom`.
- `manifest/roadmap.md`: the burler/perch half of the "producers standalone: producer engines" Planned item moves to Done.

**Out:**

- **Standalone mode itself.** The CLI construction sites keep passing `layout.WorktreePath()` / `layout.AnchorPath()` (via `hubgeom`) unchanged. No `--target-dir`, no `--stencils-dir`, no branch around `lyxcwd.Resolve`. That is T8.
- **Any change to where a directory resolves in a real worktree.** Every path this task touches must resolve byte-identically before and after.
- `internal/websterengine` / `internal/webstercli` — that is T7, running in parallel.
- `internal/shuttleengine`, `internal/reedengine`, `internal/pattern`, `internal/treadleengine` — already converted or never coupled.
- `internal/shedadapters` — already fully told (`NewPerchProducer` takes `runDirBase`/`scratchDirBase`/`stencilsDir` as strings); it mentions `perchengine.New` only in a doc comment, which may need a wording touch but no code change.
- `CONSTRAINTS.md` — no invariant changes here (see Decisions).
- `docs/overview.md` — no module added or removed, no execution-stack change.
- `manifest/designs/producers-standalone.md` — the design doc survives until the last wave lands; it is not annotated per-task.

## Decisions

### told-geometry carrier — a package-owned `Geometry` struct per engine, not loose string params

- **Decision.** `burlerengine.Geometry{WorktreeRoot, AnchorRoot string}` and `perchengine.Geometry{GateDir, AnchorRoot string}`, each declared by its own engine. `hubgeom.BurlerGeometry(l) burlerengine.Geometry` and `hubgeom.PerchGeometry(l) perchengine.Geometry` are the hub-mode tellers.
- **Rationale.** `internal/hubgeom/doc.go` — already on `main`, landed in wave 2 — states the contract outright: *"it converts a resolved `*lyxcwd.Location` into the geometry struct each engine holds … neither will burlerengine's or perchengine's or websterengine's own geometry types [import hubgeom] — so the told direction stays one-way"*, and names T6 as the task adding `BurlerGeometry` and `PerchGeometry`. `reedengine.Geometry` + `hubgeom.ReedGeometry` is the shipped precedent for exactly this pair.
  Two loose strings at a call site are also the shape that makes the one silent failure mode possible: `New(shuttle, anchorRoot, worktreeRoot, …)` compiles and passes any test whose fixture lets the two coincide. Named struct fields make the swap visible at the call site.
- **Rejected.** (a) Plain positional string params, which is the T6 brief's literal wording — written before wave 2 landed `hubgeom`, and superseded by `hubgeom`'s own committed doc. It also leaves `hubgeom.BurlerGeometry` with nothing coherent to return. (b) One shared geometry type across engines — each engine would then carry fields it never reads, and the told-direction rule is per-engine by design. `shuttleengine.NewRunner(reed, engine, anchorPath, worktreeRoot, cfg)` took plain strings in wave 1 and stays as it is; it predates `hubgeom` and is not re-opened by this task.

### `stencilsDir` stays a separate parameter, out of both `Geometry` structs

- **Decision.** `burlerengine.New(shuttle Shuttle, geom Geometry, cfg Config, stencilsDir string)` — `stencilsDir` keeps its own parameter. `perchengine.Engine.Run(p, runDir, scratchDir, stencilsDir string)` is untouched.
- **Rationale.** Perch takes `stencilsDir` at `Run` time, not at construction, so a symmetric `Geometry.StencilsDir` is impossible for perch and would make the two structs disagree for no gain. `stencilsDir` is also flag-overridable in both modes once T8 lands (`--stencils-dir` is honoured in a real worktree too); geometry is structural, config-shaped values are not.
- **Rejected.** Folding `StencilsDir` into `burlerengine.Geometry` and having `hubgeom.BurlerGeometry` default it from `fabricengine.StencilsDir(l.HubPath)` — `hubgeom` already imports `fabricengine` so it would cost no new import, but it buries a value T8 must override inside the struct T8 must not override.

### `perchengine.RunsDir` / `ScratchDir` take a told `anchorRoot string`

- **Decision.** `RunsDir(anchorRoot string) string` and `ScratchDir(anchorRoot string) string`, mirroring `planparser.PlanDir(anchorPath string)` and `pattern.File(baseDir string)` from waves 1 and 2. Their doc comments keep the "per the Cwd Resolution Invariant, no other package may construct this path" line — still true, and still the point.
- **Rationale.** `perchcli` needs both bases in `PersistentPreRunE`, before the per-invocation `perchengine.New` in `run.go` — so they cannot be methods on `*Engine`. Taking `Geometry` instead of a bare string would force every non-perch caller (`cmd/lyx` tests, `perchcli` integration tests) to build a `Geometry` to ask a one-field question.
- **Rejected.** Methods on `*Engine` (wrong lifetime); taking `Geometry` (needless ceremony at every call site).

### `perchengine.Geometry`'s worktree-side field is named `GateDir`, not `WorktreeRoot`

- **Decision.** `perchengine.Geometry{GateDir, AnchorRoot string}`. `hubgeom.PerchGeometry` maps `l.WorktreePath()` onto `GateDir`.
- **Rationale.** Perch's only use of that root is `treadleengine.Profile.GateDir` — the gate command's working directory. It does not resolve profile-relative paths against it the way burler does. Naming the field after what perch does with it keeps the engine honest about the narrow thing it was told, which is the whole point of told-geometry; the T6 brief uses the same word (`gateDir`).
- **Rejected.** `WorktreeRoot`, for symmetry with `burlerengine.Geometry` and with T8's pinned-values table. The symmetry is real but shallow — T8's table already records that perch's `GateDir` *is* the `worktreeRoot` row, so no information is lost, and the field name stays true to its single consumer.

### `Engine` stores the whole `Geometry` value

- **Decision.** Both engines store `geom Geometry` as one field, not unpacked strings; `perchengine.Engine` therefore carries an `AnchorRoot` it does not read today.
- **Rationale.** `reedengine.Engine` does the same, and the struct is the told-geometry unit — unpacking it into loose fields on construction reintroduces the swap hazard one layer in. `perchengine`'s unread `AnchorRoot` is documented as the caller's `RunsDir`/`ScratchDir` base carried alongside, so the two roots stay visible together at every perch call site.
- **Rejected.** Storing only the fields each engine reads. Saves one string and costs the co-located pair.

### `perchcli` keeps `c.layout`, fabric-only

- **Decision.** `perchCLI` keeps its `layout *lyxcwd.Location` field and gains `perchGeom perchengine.Geometry`. `c.layout` is used only by the fabric call sites (`run.go:334` `fabricengine.ScopedPathspec(c.layout.AnchorRel, …)`, `run.go:344` `fabricengine.Open(c.layout)`) and by `fabricengine.StencilsDir(c.layout.HubPath)` at `run.go:301`. `perchengine.New` at `run.go:294` takes `c.perchGeom`; `c.runDirBase`/`c.scratchDirBase` come from `perchengine.RunsDir(c.perchGeom.AnchorRoot)` / `ScratchDir(c.perchGeom.AnchorRoot)`.
- **Rationale.** Fabric sync is genuinely hub-mode-only and genuinely needs the `Location`. This task is about what the *engines* are told, not about making the CLI hub-blind — that is T8, which is where those fabric sites get their standalone answer (skipped entirely).
- **Rejected.** Dropping `layout` and threading extracted strings to the fabric sites too — that is T8's work done early, in the task that is supposed to change no behaviour.

### `burlerengine.New` keeps its positional shape

- **Decision.** `New(shuttle Shuttle, geom Geometry, cfg Config, stencilsDir string)` — `layout` becomes `geom` in place. `perchengine.New(burler Burler, shuttle Shuttle, cfg Config, geom Geometry, opts Options)` likewise.
- **Rationale.** Smallest reviewable diff; every existing call site and test changes in exactly one argument position.
- **Rejected.** An options-struct constructor for either engine — unrelated churn in a task whose contract is "nothing resolves anywhere different".

### `cmd/lyx` anchoring rows are rewritten in place, and go tautological — deliberately

- **Decision.** In `cmd/lyx/constructoranchoring_test.go`, the four `perchengine.RunsDir`/`ScratchDir` rows (lines 88, 98, 145, 155) plus the `dotLyxConstructors` map entry (line 176) become `perchengine.RunsDir(l.AnchorPath())` / `ScratchDir(l.AnchorPath())`. `cmd/lyx/notransients_test.go`'s five call sites (lines 65, 66, 75, 80, 157) get the identical mechanical swap. The file-header and inline comments are extended to name `perchengine`'s rows alongside the `planparser`/`pattern.File` rows already carrying the "tautological with respect to anchoring; the real proof lives at the production call site" note.
- **Rationale.** This is exactly what wave 1 and wave 2 did for `planparser.PlanDir` and `pattern.File`, and the comment explaining it is already in the file. Once a function takes the anchor as a parameter, a row that passes `l.AnchorPath()` in and compares against an anchor-derived expectation can no longer catch a production site that passes the wrong root — but it still pins the join arithmetic and the `_lyx`-vs-`.lyx` group placement, which is why the design doc says this file is *edited in place, per task, never split or retired*.
- **Rejected.** Deleting the rows. The mirrored-subpath equality check in `notransients_test.go` (`RunsDir` → swap `_lyx` for `.lyx` → must equal `ScratchDir`) is load-bearing for the Durable-vs-Ephemeral State Invariant and has nothing to do with anchoring.

### `manifest/roadmap.md`: split the Planned item rather than leaving it for whoever lands second

- **Decision.** Reword Planned item 1 ("producers standalone: producer engines") to name only the remaining Webster half, and add a Done entry for the burler/perch half.
- **Rationale.** That roadmap line covers T6 *and* T7, which run in parallel. "Move it when both are done" has an obvious failure mode: each task reads the rule, sees a half-done item, and skips — so it never moves. Splitting is self-coordinating: whichever lands first shrinks the item, whichever lands second moves what remains to Done. The cost is one extra one-paragraph merge conflict with T7, on top of the `constructoranchoring_test.go` conflict the design doc already predicts.
- **Rejected.** Not touching the roadmap (the never-moves failure mode above); moving the whole item to Done (false — Webster is not done).

### `CONSTRAINTS.md` is not edited by this task

- **Decision.** No invariant text changes.
- **Rationale.** No invariant is falsified. The Cwd Resolution Invariant's "a module's own durable-storage subdirectory … is that module's own private relative-path constant, joined onto `AnchorPath()` directly" stays literally true — `perchengine` still owns `perchDirName`, and hub mode still joins it onto `AnchorPath()`, now via `hubgeom`. The Durable-vs-Ephemeral State Invariant's `_lyx`/`.lyx` sibling rule is unaffected; the Stencil Ownership and Durable-vs-Ephemeral rewords the design doc anticipates belong to T8 and T10, where standalone actually relocates a root.
- **Rejected.** Pre-emptively rewording Durable-vs-Ephemeral for a standalone `<state>` root that does not exist yet.

## Technical context

**The exact `*lyxcwd.Location` surface being removed** (verified against the tree, not inherited from the design doc):

| Site | Reads | Becomes |
|---|---|---|
| `internal/burlerengine/engine.go:31` | `layout` field | `geom Geometry` |
| `internal/burlerengine/engine.go:41` | `New(shuttle, layout, cfg, stencilsDir)` | `New(shuttle, geom, cfg, stencilsDir)` |
| `internal/burlerengine/engine.go:97` | `e.layout.WorktreePath()` → `p.validate` | `e.geom.WorktreeRoot` |
| `internal/burlerengine/engine.go:103` | `e.layout.AnchorPath()` → `pattern.Directive` | `e.geom.AnchorRoot` |
| `internal/burlerengine/engine.go:111` | `e.layout.AnchorPath()` → `.lyx/burler` | `e.geom.AnchorRoot` |
| `internal/perchengine/engine.go:52` | `layout` field | `geom Geometry` |
| `internal/perchengine/engine.go:58` | `New(burler, shuttle, cfg, layout, opts)` | `New(burler, shuttle, cfg, geom, opts)` |
| `internal/perchengine/engine.go:101` | `e.layout.WorktreePath()` → `treadleengine.Profile.GateDir` | `e.geom.GateDir` |
| `internal/perchengine/identity.go:33` | `RunsDir(l *lyxcwd.Location)` | `RunsDir(anchorRoot string)` |
| `internal/perchengine/identity.go:43` | `ScratchDir(l *lyxcwd.Location)` | `ScratchDir(anchorRoot string)` |

`pattern.Directive` already takes `(anchorPath, stencilsDir string, role Role)` — wave 2 landed it, so `engine.go:103` needs only the argument swapped, not the call reshaped.

**Production call sites** (the enumeration was re-run for this task's symbols; `go test ./...` is the backstop):

- `internal/burlercli/cli.go:107` — `burlerengine.New(runner, layout, burlerCfg, fabricengine.StencilsDir(layout.HubPath))`. `hubgeom` is already imported here (line 17) for `ReedGeometry` at line 104, so `hubgeom.BurlerGeometry(layout)` costs no new import. `layout` is still needed for `fabricengine.StencilsDir(layout.HubPath)` and the four config loads.
- `internal/perchcli/cli.go:147` — same `burlerengine.New` call, same already-present `hubgeom` import (line 20).
- `internal/perchcli/cli.go:160,166` — `perchengine.RunsDir(layout)` / `ScratchDir(layout)`. The long comments at 152-159 and 161-165 explaining *why* these are `AnchorPath()`-anchored must survive the rewrite; they are the record of a real bug class (a nested-init repo stranding artifacts outside fabric).
- `internal/perchcli/run.go:294` — `perchengine.New(c.burlerEngine, c.runner, c.perchCfg, c.layout, …)`.
- `internal/shedadapters/perch.go` — no call, one doc-comment mention of `perchengine.New` at line 34 (about `Options.PauseRequested` consumption at construction time). Still accurate; check the wording survives.

**Test call sites** (`*_test.go`), all mechanical:

- `internal/burlerengine/engine_test.go` — 8 `&lyxcwd.Location{…}` literals (lines 89, 174, 191, 224, 249, 274, 459, 579).
- `internal/burlerengine/smoke_round_test.go:321`, `smoke_cluster_test.go:140` — `burlerengine.New(runner, h.Location, …)` from the external test package, using a `hubforge` hub. These may call `hubgeom.BurlerGeometry(h.Location)`: an external `_test` package importing `hubgeom` creates no cycle even though `hubgeom` imports `burlerengine`.
- `internal/perchengine/run_test.go` — `newTestLayout` (line 267) and ~10 `New(fb, qs, Config{}, layout, Options{})` call sites.
- `internal/perchengine/identity_test.go` — `TestScratchDir`'s two-case table built from synthetic `Location`s.
- `internal/perchcli/cli_integration_test.go` (lines 63, 81, 104, 114, 148, 162) and `run_integration_test.go` (lines 243, 250) — `perchengine.RunsDir(h.Location)` → `RunsDir(h.Location.AnchorPath())`.
- `cmd/lyx/constructoranchoring_test.go`, `cmd/lyx/notransients_test.go` — see the Decision above. **`notransients_test.go` is not in the design doc's `Files` list for T6** — it was missed there. The doc itself warns that its enumerations go stale and instructs re-running them; this is one of the hits.

**`internal/hubgeom` is the pattern to copy exactly.** `hubgeom.go`'s `ReedGeometry` is 12 lines of field assignment with a doc comment stating it performs no `os.Getwd`, no git discovery, and no path resolution of its own. `hubgeom_test.go`'s file comment states its own reason for existing, which applies verbatim to the two new functions: *"the load-bearing guard against this refactor's one silent failure mode: a swapped anchor/worktree pair compiles cleanly and passes every test built on a fixture where the two happen to coincide"* — hence its fixture keeps hub, worktree root, and anchor path as three distinct directories with `RepoName` differing from every basename.

**The perchcli anchoring proof already exists and does not need to be written.** `internal/perchcli/cli_integration_test.go:96` `TestRunCLI_Pause_NestedInitAnchorsRunDirsAtCwd` builds a real `hubforge.NewHub(t, "nested")`, drives the real `PersistentPreRunE` through `RunCLIIn(h.Location.AnchorPath(), …)`, and asserts the pause verb finds a run dir created under the anchored `_lyx/perch`. A production regression to `WorktreePath()` makes that lookup miss and the test fail. This is the perch equivalent of `webstercli`'s `TestPersistentPreRunE_PlanDirAnchoredAtSubpath`, which is the proof T4 relied on when its own `cmd/lyx` rows went tautological.

**`burlerengine`'s equivalent proof needs strengthening, not creating.** `TestEngine_Run_MaterializesInstructionFiles` (`engine_test.go:450`) already builds a `Location` with `AnchorRel: "sub/dir"` precisely so the instruction dir's `AnchorPath` anchoring is observable, and asserts the round dir lands under `AnchorPath()/.lyx/burler`. Rewritten onto `Geometry`, it becomes the direct swap guard: distinct `WorktreeRoot` and `AnchorRoot`, instruction files under `AnchorRoot` only, profile-relative paths resolving under `WorktreeRoot` only.

**Coordination with T7 (`webster-told-geometry`), running in parallel.** Expected conflicts, both mechanical:

1. `cmd/lyx/constructoranchoring_test.go` — T6's `perchengine` rows (88/98/145/155) sit directly beneath T7's `websterengine` rows (86-87/96-97/143-144/150-ish). Whichever merges second rebases.
2. `manifest/roadmap.md` — the split described above; whichever merges second rewrites the remaining half.

Neither is a logic conflict. Nothing else is shared: `burlercli`/`perchcli` and `webstercli` are distinct packages, and `hubgeom.WebsterGeometry` is a separate function in the same file (a possible third, trivial conflict in `hubgeom.go`/`hubgeom_test.go`/`doc.go` if both add functions — resolve by keeping both).

## Constraints

From `CONSTRAINTS.md` (read this session):

- **Cwd Resolution Invariant.** `internal/lyxcwd` alone owns cwd resolution. Neither engine may call `lyxcwd.Resolve`, `os.Getwd`, or `git rev-parse` — after this task neither imports `lyxcwd` at all. A module's own durable subdirectory (`perchDirName`) stays that module's private constant joined onto a told anchor. `root` always means a worktree/repo root, `cwd` means the current directory — the new field names must not blur that (`AnchorRoot` is a root; it is never a cwd).
- **Lyxdirs Single-Declarer Invariant.** `_lyx` and `.lyx` only ever appear as `lyxdirs.LyxDirName` / `lyxdirs.DotLyxDirName` in path construction. Enforced by `internal/lyxcwd/enforcement_test.go`.
- **Durable-vs-Ephemeral State Invariant.** `RunsDir` (durable, `_lyx`) and `ScratchDir` (never-tracked, `.lyx`) must stay mirrored siblings differing in exactly that one segment. Enforced by `cmd/lyx/notransients_test.go` and `cmd/lyx/constructoranchoring_test.go` — both of which this task edits, so the guards must stay guards.
- **CLI / Cobra Invariant.** No new command or flag here, so the help-tree obligations are untouched; the `Command()`/`RunCLI` seam in both CLIs stays as it is.
- **Test Tier Purity Invariant.** Anything spawning a subprocess or building a real hub is `//go:build integration`. `hubgeom_test.go` and the `cmd/lyx` anchoring tables stay untagged pure-`filepath.Join` arithmetic.
- **Hermetic Git Test Environment Invariant** — applies to the `hubforge`-backed integration tests being touched.
- **Documentation Lifecycle** (project `CLAUDE.md`). Docs land in the same commit: `doc.go` for each touched module, and `manifest/roadmap.md` for the completed half. `docs/overview.md` is unchanged (no module added, no execution-stack change).

Discovered during exploration:

- **The `hubgeom` told-direction rule is one-way and must stay so:** `hubgeom` imports the engines; no engine may import `hubgeom`. External `_test` packages (`package burlerengine_test`) are exempt from this in the compiler's eyes and may import it.
- **`internal/hubgeom/doc.go` names T6 and T7 as future work.** It must be updated in this commit to stop describing `BurlerGeometry`/`PerchGeometry` as not-yet-existing, leaving only T7's `WebsterGeometry` as pending.

## Testing

**Behaviour-preservation is the contract.** Every path must resolve byte-identically before and after; the existing suites are the primary evidence, and a test that only changes shape (a `Location` literal becoming a `Geometry` literal) must keep its original assertion intact.

**`internal/hubgeom` — TDD candidate, write first.**
`TestBurlerGeometry` and `TestPerchGeometry`, modelled on the existing `TestReedGeometry` and reusing its deliberately-hostile fixture: hub, worktree root, and anchor path as three distinct directories, `AnchorRel` a real nested subpath, `RepoName` differing from every basename. Assert each field against an independently computed `filepath.Join`, so a swapped `WorktreeRoot`/`AnchorRoot` assignment fails. This is the cheapest place to pin the mapping, and it is where the swap actually gets made.

**`internal/burlerengine`.**
- Convert every `New(...)` call to a `Geometry` literal. Where a test's fixture currently collapses the two roots (`HubPath: filepath.Dir(root), WorktreeName: filepath.Base(root)` with no `AnchorRel`), keep it collapsed — those tests are not about anchoring.
- Strengthen `TestEngine_Run_MaterializesInstructionFiles` into the explicit swap guard: distinct `WorktreeRoot` and `AnchorRoot` directories, assert the round directory lands under `AnchorRoot/.lyx/burler` and *not* under `WorktreeRoot`, and assert a profile-relative path resolves under `WorktreeRoot`. A swapped constructor must fail this, not merely be caught by a downstream file-not-found.
- `Profile.validate`'s worktree-root resolution has existing coverage in `profile_test.go`; confirm it still exercises the told root.
- Smoke tests (`smoke_round_test.go`, `smoke_cluster_test.go`) are integration-tier and change only their construction line.

**`internal/perchengine`.**
- `identity_test.go`'s `TestScratchDir` takes told `anchorRoot` strings directly; keep both cases (unanchored and a nested subpath) and keep the mirrored-segment assertion — it is the package-local half of the Durable-vs-Ephemeral guard.
- `run_test.go`'s `newTestLayout` becomes a `newTestGeometry` helper returning distinct `GateDir` and `AnchorRoot` values.
- Add (or confirm) a case asserting `treadleengine.Profile.GateDir` equals the told `Geometry.GateDir` — that is perch's entire remaining geometry use, and it currently has no direct assertion.

**`internal/burlercli` / `internal/perchcli`.**
- Untagged unit tests change only where they build the engine stack.
- The integration tests' `perchengine.RunsDir(h.Location)` call sites become `RunsDir(h.Location.AnchorPath())`.
- `TestRunCLI_Pause_NestedInitAnchorsRunDirsAtCwd` is the production-call-site anchoring proof and must keep passing unmodified in substance. Do not weaken it into computing its expectation from the CLI's own value.

**`cmd/lyx`.**
- `constructoranchoring_test.go` and `notransients_test.go` rows rewritten as described; the `notransients_test.go` mirrored-pair equality check and the non-empty-table sanity guard must both still fire.

**Verify.**

- Per-package: `go test ./internal/hubgeom/... ./internal/burlerengine/... ./internal/perchengine/... ./internal/burlercli/... ./internal/perchcli/... ./internal/shedadapters/... ./cmd/lyx/...`
- Integration: `go test -tags integration ./internal/perchcli/... ./internal/burlerengine/...`
- Done gate (task-wide): `go test ./...` from the worktree root.
- Grep check: `internal/lyxcwd` must be absent from `internal/burlerengine`'s and `internal/perchengine`'s production (non-`_test`) imports.

## Q&A log

- **Q:** How is the told geometry carried — an engine-owned `Geometry` struct, plain string params per the T6 brief, or one shared type? **A:** [auto-pick] Engine-owned `Geometry` struct per engine. **Why:** `internal/hubgeom/doc.go`, already on `main`, states the contract as "the geometry struct each engine holds" and names T6 as the task adding `BurlerGeometry`/`PerchGeometry`; `reedengine.Geometry` + `hubgeom.ReedGeometry` is the shipped precedent, and named fields make the anchor/worktree swap visible at every call site.
- **Q:** Does `stencilsDir` belong inside `burlerengine.Geometry`? **A:** [auto-pick] No — it stays a separate parameter. **Why:** perch takes it at `Run` time, not construction, so folding it in would make the two structs asymmetric; it is also flag-overridable in both modes once T8 lands, and geometry is structural, not config-shaped.
- **Q:** What signature do `perchengine.RunsDir`/`ScratchDir` take? **A:** [auto-pick] A told `anchorRoot string`. **Why:** `perchcli` needs both bases in `PersistentPreRunE`, before the per-invocation `New` in `run.go`, so methods on `*Engine` are impossible; taking `Geometry` would force every caller to build a struct to ask a one-field question. Mirrors `planparser.PlanDir`/`pattern.File` from waves 1-2.
- **Q:** Does `perchcli` keep `c.layout`? **A:** [auto-pick] Yes — fabric-only, alongside a new `c.perchGeom`. **Why:** `fabricengine.ScopedPathspec`/`Open`/`StencilsDir` genuinely need the `Location` and are genuinely hub-mode-only; making the CLI hub-blind is T8's job, not this task's.
- **Q:** Does `burlerengine.New` keep its positional shape? **A:** [auto-pick] Yes — `layout` becomes `geom` in place. **Why:** smallest reviewable diff in a task whose contract is "nothing resolves anywhere different"; an options-struct rewrite is unrelated churn.
- **Q:** What happens to the `cmd/lyx` anchoring rows once they go tautological? **A:** [auto-pick] Rewritten in place with the same "real proof lives at the production call site" comment waves 1-2 added. **Why:** the design doc says this file is edited in place per task, never split or retired; the rows still pin the join arithmetic and the `_lyx`-vs-`.lyx` group placement even after they stop catching a wrong-root call site.
- **Q:** Where does the anchor/worktree swap guard live? **A:** [auto-pick] New `hubgeom_test.go` cases on the three-distinct-directories fixture, plus strengthening `burlerengine`'s existing `TestEngine_Run_MaterializesInstructionFiles`; perch's proof already exists at `perchcli/cli_integration_test.go:96`. **Why:** `hubgeom_test.go`'s own file comment declares this exact failure mode as its reason for existing, and writing a new perchcli test would duplicate a passing one.
- **Q:** What happens to `manifest/roadmap.md`, given the Planned item covers T6 and T7 together? **A:** [auto-pick] Split it — burler/perch half to Done, Webster half stays Planned. **Why:** "move it when both are done" fails silently when both parallel tasks defer to each other; splitting is self-coordinating, at the cost of one extra one-paragraph conflict with T7.
- **Q:** Does `CONSTRAINTS.md` change? **A:** [auto-pick] No. **Why:** no invariant is falsified — hub mode still joins `perchDirName` onto `AnchorPath()`, just via `hubgeom`; the Stencil Ownership and Durable-vs-Ephemeral rewords belong to T8/T10, where a standalone root actually appears.
- **Q:** What is the verification set? **A:** [auto-pick] The design doc's per-package set plus `./internal/hubgeom/...` and `./internal/shedadapters/...`, the perchcli integration tag, and `go test ./...` as the task-wide done gate. **Why:** the signature change reaches packages the constructor-only reading misses — `cmd/lyx/notransients_test.go` is already one confirmed omission from the design doc's own `Files` list.
