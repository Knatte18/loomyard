# Discussion: the standalone CLI path

```yaml
task: the standalone CLI path
slug: standalone-cli-entry
status: discussing
parent: standalone-producers
```

## Problem

`lyx burler run --profile p.yaml` and `lyx perch run --profile p.yaml` die in their `PersistentPreRunE` before reaching any producer logic whenever the current directory is not the root of a git worktree.
Worse, in an *ordinary* git repository — a downloaded repo with no lyx hub beside it — they do not die: `lyxcwd.Resolve` succeeds, and the CLI proceeds in hub mode with a fictional `HubPath` (the repo's parent directory) and a fictional `RepoName`, mis-naming the tmux socket and writing a `.lyx` tree into the repository being reviewed.
This is T8 of `manifest/designs/producers-standalone.md` — the task the whole design exists for.

**Why now.** Waves 1–3 have all landed on `main` (T1–T7, commits `9fc03426`, `33018982`, `3255efa6`).
Every prerequisite is in the tree: `internal/preflight` with its two predicates, `internal/buildinfo`, `internal/standalonestate`, `internal/hubgeom`, `internal/standalonegeom`, told-geometry constructors on `burlerengine`/`perchengine`/`reedengine`/`shuttleengine`, and degrading config loaders.
T7 additionally shipped the *entire pattern* on a third CLI — `internal/webstercli/wiring.go` is a working, tested reference implementation of exactly what this task must do to `burlercli` and `perchcli`.
This task is therefore substantially a mirroring exercise, not a design exercise, and its main risk is divergence from the shipped precedent rather than novelty.

## Scope

**In:**

- `internal/burlercli` — a new `wiring.go` (`wire`/`wireHub`/`wireStandalone`), `cli.go` reduced to cwd + one `preflight.HubPresent` probe + delegation, two new persistent flags, and a `Long` that documents the two modes.
- `internal/perchcli` — the same, plus removing `c.layout` from the receiver and rerouting `run.go`'s three `c.layout` uses onto told values.
- `internal/standalonegeom` — new `BurlerGeometry(target, stateDir)` and `PerchGeometry(target, stateDir)` builders, siblings of the existing `ReedGeometry`/`WebsterGeometry`.
- Two new persistent flags on each of `lyx burler` and `lyx perch`: `--stencils-dir` and `--target-dir`.
- Standalone stencils bootstrap via `stencilstore.Reconcile` for the derived default directory only.
- Operator-visible reporting of the standalone state and stencils directories in the run verbs' JSON envelopes.
- Tests: one untagged `wiring_test.go` per CLI covering the mode-selection truth table and every pinned standalone value, and one `//go:build integration` test per CLI driving `RunCLIIn` from a directory outside any git repository.
- `CONSTRAINTS.md` — verification pass over the Stencil Ownership and Durable-vs-Ephemeral State invariants (see the decision below; the expected outcome is no edit).

**Out:**

- `cmd/lyx/stencilseed.go` and `cmd/lyx/main.go` — the root pre-run's fictional-hub stencil-write gate is already closed by T5, and this task must not touch it.
- `internal/buildinfo` and `internal/standalonestate` — T5's packages, imported here, never modified here.
- `internal/burlerengine` and `internal/perchengine` — T6 already converted both to told geometry; no signature changes remain.
- `cmd/lyx/constructoranchoring_test.go` — already in its post-T6 told-string shape; no row this task changes.
- `internal/webstercli` — T7's standalone entry is done and is the reference, not a target.
- `internal/scoutcli`/`scoutengine` — that is T9, optional and independent.
- The three-tier `CONSTRAINTS.md` invariant, `docs/overview.md`, and `manifest/roadmap.md` — T10's consolidation.
- A `--plan-dir` flag — webster-only; burler and perch parse no plan.
- `lyx burler run <path>` as a positional argument — explicitly rejected by the design.
- Any change to where hub mode *resolves* or *writes*. Every hub-mode path stays byte-identical; the one deliberate hub-visible change is the three additive envelope fields (see the byte-identity decision, which names both exceptions).

## Decisions

### mode-trigger — `preflight.HubPresent`, not `fabricengine.Ready`

- **Decision:** mode selection is `loc, hubPresent := preflight.HubPresent(cwd)`; `hubPresent` true selects hub mode, false selects standalone.
  `preflight.Wired` is never consulted.
- **Rationale:** this is a considered override of T8's own brief text, and the override is already shipped and documented — see `internal/preflight/doc.go`'s "Why there are two predicates" section and `internal/webstercli/wiring.go`'s `wire` doc comment.
  `fabricengine.Ready` probes the *paired sibling of the current worktree*, not the hub, so it is false at `<hub>/_board`, false in an unpaired sibling, and false in a worktree whose pair was removed — three real, healthy hub situations that run burler and perch verbs today.
  Keying mode selection on `Ready` would route all three to standalone and silently relocate a live hub's state into the per-OS state directory: strictly worse than the misclassification it would be avoiding.
  `HubPresent` (a `lyxcwd.Resolve` plus one `os.Stat` of `<hub>/_board/_lyx`) asks the honest question — does a hub-level directory exist for this write to target — and still routes a plain downloaded repo to standalone, which was the hazard the split was built for.
  It also satisfies the brief's "a wired-but-broken hub is refused, never silently degraded to standalone" requirement by a different and better mechanism: a damaged hub stays in hub mode and fails loudly at the point of use.
- **Rejected:** the brief's literal `Ready`-class trigger with a `fabricengine.BoardDir(filepath.Dir(worktreeRoot))` discriminator — superseded by T5/T7, and re-implementing it here would make burler and perch select modes differently from webster in the same tree.
  Also rejected: importing `internal/fabricengine` into either CLI to reach `Ready` directly, which the design forbids outright.

### structure — a `wiring.go` per CLI, mirroring `webstercli`

- **Decision:** each CLI gains `internal/<mod>cli/wiring.go` holding `func (c *<mod>CLI) wire(loc *lyxcwd.Location, hubPresent bool, cwd, stencilsDirFlag, targetDirFlag string) error`, delegating to `wireHub` and `wireStandalone`.
  `cli.go`'s `PersistentPreRunE` becomes an extracted method `resolvePersistentPreRun` that does the group-command guard, `lyxcwd.CwdFrom`, one `preflight.HubPresent` call, and one `c.wire(...)` call.
  The mode decision lives **inside** `wire`, not in `resolvePersistentPreRun`.
- **Rationale:** the extraction is what makes the truth-table test tier 1.
  A test that drives the real pre-run reaches `lyxcwd.Resolve`, which spawns git through `gitexec.Run` and breaches the Test Tier Purity Invariant; a test that calls `wire` with a told `(loc, hubPresent)` pair does not.
  Placing the decision inside `wire` rather than upstream is precisely what lets the test drive both arms.
  Following `webstercli` byte-for-byte in shape means a reader who knows one of the three CLIs knows all three.
- **Rejected:** keeping an inline `PersistentPreRunE` closure with an `if` — untestable at tier 1, which is the design's stated non-negotiable.
  Also rejected: a shared `internal/standalonewiring` package factoring the three CLIs' wiring together — the three stacks differ in their engine sets, their fabric relationships, and their flags, and there is no third-use pressure yet.

### geometry — `standalonegeom.BurlerGeometry` / `PerchGeometry`

- **Decision:** add two builders to `internal/standalonegeom`, siblings of the existing `ReedGeometry`/`WebsterGeometry`:
  - `BurlerGeometry(target, stateDir string) burlerengine.Geometry` → `{WorktreeRoot: target, AnchorPath: stateDir}`.
  - `PerchGeometry(target, stateDir string) perchengine.Geometry` → `{GateDir: target, AnchorPath: stateDir}`.
  Neither takes `hash8`: no value in either struct is hash-derived, and `standalonegeom.WebsterGeometry` already set the precedent of omitting the parameter rather than carrying an unused one for symmetry.
  **Neither carries the stencils directory** — see the next decision for where that value comes from.
- **Rationale:** this is the `worktreeRoot`/`anchorRoot` split the design calls "the load-bearing move", expressed once per engine in the one package that already owns told-mode geometry construction.
  `WorktreeRoot`/`GateDir` = the target directory, because `burlerengine`'s `Profile.validate` resolves `Target.Paths`, `Fasit.Paths`, `ReviewPath`, `FixerReportPath` and both prior-report lists against it, and perch's `GateDir` is the gate command's working directory.
  `AnchorPath` = `<state>`, because it is the base every `_lyx`/`.lyx` path joins onto — which is what relocates burler's `.lyx/burler`, perch's `RunsDir`/`ScratchDir`, reed's `stateDir`, and shuttle's `runDirRoot` automatically, rather than as four separate things an implementer must remember.
  `standalonegeom` deliberately never calls `standalonestate.Derive` (its package doc says so): the CLI derives once at the argument boundary and tells both builders.
- **Rejected:** constructing the two structs inline at each CLI site — the design explicitly says the geometry conversion belongs in the geom packages rather than re-derived inline, and `hubgeom` already holds the hub-mode twins.

### flags — `--stencils-dir` in both modes, `--target-dir` standalone-only

- **Decision:** both are `PersistentFlags()` on each module's parent command, bound to `stencilsDirFlag`/`targetDirFlag` fields on the CLI receiver, with empty string meaning "not passed".
  - `--stencils-dir`: honoured in **both** modes, read-only in both.
    Hub default: `fabricengine.StencilsDir(loc.HubPath)`, exactly as today.
    Standalone default: `<state>/_lyx/stencils`.
  - `--target-dir`: honoured in standalone only, defaulting to cwd; passing it in hub mode is a **hard error** naming why, mirroring `webstercli`'s wording.
- **Rationale:** `--stencils-dir` names a directory that is only ever read, so pointing a real worktree at an experimental stencil set is harmless and is the flag's most useful application.
  `--target-dir` is the base `Profile.validate` resolves `ReviewPath`/`FixerReportPath` against — it decides where the round *writes* — so honouring it in hub mode would place artifacts outside the anchored subtree that Fabric's positive-only commit pathspec covers, silently stranding them.
  In hub mode the value is structurally `loc.WorktreePath()`.
  Both flags are persistent rather than per-verb because they configure the stack the pre-run builds, before any verb runs — the same reason `webstercli` made all three persistent.
- **Rejected:** refusing `--stencils-dir` in hub mode (forbids the one thing it is most useful for); making `--target-dir` silently ignored in hub mode (a silent write-location surprise is exactly what the refusal exists to prevent); per-verb flags (the pre-run cannot see them).

### standalone target resolution

- **Decision:** reuse `webstercli`'s `resolveStandaloneTarget(cwd, targetDirFlag)` shape verbatim per package: empty flag → `cwd`; absolute flag → `filepath.Clean(flag)`; relative flag → `filepath.Join(cwd, flag)`.
  The result is always absolute, which is `standalonestate.Derive`'s own precondition.
- **Rationale:** `Derive` normalises through `EvalSymlinks`+`Clean` and compares case-insensitively on Windows, so two spellings of the same directory must not produce different `<state>` values — but that only holds if the input is already absolute.
- **Rejected:** exporting the helper from a shared package — three near-identical five-line functions across three CLI packages is cheaper than a package that exists only to hold one of them; revisit if a fourth standalone CLI appears.

### standalone stencils directory — one owner, `standalonegeom.StencilsDir`

- **Decision:** the standalone stencils default is produced by a new exported helper,
  `standalonegeom.StencilsDir(stateDir string) string` → `filepath.Join(stateDir, lyxdirs.LyxDirName, "stencils")`, in a new `internal/standalonegeom/stencilsdir.go`.
  Both CLIs' `wireStandalone` call it and carry the result as a plain local/receiver `stencilsDir string`, **never** as a geometry field.
  `standalonegeom.WebsterGeometry` is repointed at the same helper for its own `StencilsDir` field, replacing the inline `filepath.Join(stateDir, lyxdirs.LyxDirName, "stencils")` it computes today — a one-line, same-package change that leaves the shipped value byte-identical.
- **Rationale:** this closes a real hole in the first draft of this discussion, which copied `webstercli`'s `geom.StencilsDir` snippet without checking that the field exists.
  It does not: `websterengine.Geometry` carries a `StencilsDir` field, but `burlerengine.Geometry` is `{WorktreeRoot, AnchorPath}` (`internal/burlerengine/geometry.go:12`) and `perchengine.Geometry` is `{GateDir, AnchorPath}` (`internal/perchengine/geometry.go:10`).
  Adding a `StencilsDir` field to either engine's geometry would be wrong: burler already takes `stencilsDir` as its own fourth `New` parameter, and perch takes it per-call at `Engine.Run`, so a geometry field would be a second, competing home for a value both engines already accept directly.
  A single exported helper gives the `<state>/_lyx/stencils` literal exactly one construction site across all three CLIs, which is what makes "standalone's `<state>` plays the hub's role, mirroring `fabricengine.StencilsDir`" checkable rather than repeated by hand.
- **Rejected:** a per-CLI local `filepath.Join(stateDir, lyxdirs.LyxDirName, "stencils")` — three copies of the same literal, the exact drift the helper prevents.
  Also rejected: adding `StencilsDir` to `burlerengine.Geometry`/`perchengine.Geometry` (a competing home for a value both `New`/`Run` signatures already take, and a T6 signature change this task is scoped out of).
  Also rejected: leaving `WebsterGeometry`'s inline join alone (would leave two construction sites the moment the helper exists).

### stencils bootstrap — the derived default only, and a hard error

- **Decision:** when `--stencils-dir` is **unset** in standalone, seed the derived directory returned by `standalonegeom.StencilsDir(stateDir)` on first use via `stencilstore.Reconcile(stencilsDir, stencils.Registry(), stencilstore.ModeFor(buildinfo.IsDev()), "")` — a told plain string, not a geometry field — and treat a `Reconcile` error as a hard pre-run failure.
  When `--stencils-dir` **is** set, the directory is read and never written, in either mode, and the helper is not consulted at all.
- **Rationale:** nothing else will ever create the derived directory, so a silent failure there surfaces much later as an opaque prompt-render error.
  Conversely, an operator who pointed the flag at a curated stencil set must never have it reconciled out from under them — that is what makes the "read-only" characterisation literally true rather than approximately true.
  `ModeFor(buildinfo.IsDev())` reuses the existing dev/prod selector so a dev binary keeps dev seeding semantics standalone exactly as in a hub; the empty fourth argument is the "no source tree here" value that keeps the port-back drift warning silent.
  Standalone genuinely has no `contracts/stencils` source tree beside it.
- **Rejected:** best-effort logged seeding (mirrors the root pre-run, but the root pre-run has a hub-resident fallback and this has none); seeding an explicitly-told directory; hardcoding `ModeProduction`.

### perch's three `layout` uses — told values plus a nil-able fabric opener

- **Decision:** remove the `layout *lyxcwd.Location` field from `perchCLI` entirely.
  Replace its three uses in `run.go` with:
  - `fabricengine.StencilsDir(c.layout.HubPath)` (line ~301) → a told `c.stencilsDir string`, set by whichever wiring branch ran.
  - `fabricengine.ScopedPathspec(c.layout.AnchorRel, ...)` (line ~334) → a told `c.anchorRel string`, `loc.AnchorRel` in hub mode and `""` in standalone.
  - `fabricengine.Open(c.layout)` (line ~344) → `c.openFabric func() (*fabricengine.Fabric, error)`, a closure in hub mode and **nil** in standalone.
  When `c.openFabric` is nil the whole block-exit fabric sync is skipped and the envelope reports `fabricCommitted: false`.
- **Rationale:** this is `webstercli`'s exact shape (`c.refMatcher` + nil `c.openFabric`), and it is what makes "no `*lyxcwd.Location` survives on the receiver" true rather than nearly true.
  Standalone has no fabric repo by construction — not a broken one, an absent one — so nil is the honest representation, and a closure that would stat-fail if called is not.
  The existing `opts.SkipGit` short-circuit stays exactly as it is: it is a separate CI/test bypass, not a mode question, and the two conditions compose.
- **Rejected:** keeping a nil-able `c.layout` and branching on `c.layout != nil` — reintroduces the fictional-`Location` shape the whole design rejects, and invites a future reader to dereference it.
  Also rejected: synthesising a `Location` for standalone (the design's named anti-pattern, and the reason `reedengine`'s socket naming was called out).

### burler's stack — no fabric relationship at all

- **Decision:** `burlercli` carries **no fabric relationship and no `*lyxcwd.Location`** — no `openFabric`, no `anchorRel`, no `layout`.
  Its receiver holds `c.engine *burlerengine.Engine`, the two raw flag fields (`stencilsDirFlag`, `targetDirFlag`), and the three reporting fields the wiring branches set for the envelope (`mode`, `stateDir`, `stencilsDir`).
  Both wiring branches construct the engine as `burlerengine.New(runner, geom, burlerCfg, stencilsDir)`; the only differences between branches are where `geom`, the config base directory, and `stencilsDir` come from.
- **Rationale:** `internal/burlercli/run.go` references neither `layout` nor `fabricengine` today — verified by grep, which returns nothing.
  Burler is the simplest of the three CLIs, and the wiring split must not invent a fabric relationship it does not have.
  The three reporting fields are listed explicitly because `run.go` can only read them off the receiver, so "stores only `c.engine`" would have been false the moment the envelope decision landed.
- **Rejected:** threading the reporting values through `burlerengine.Result` (they are CLI-level facts about how the stack was wired, not results of a review round, and the engine must stay unaware of which mode built it).

### config base directory — `<state>` in standalone, unchanged in hub

- **Decision:** in standalone every config load takes `stateDir` as its base: `shuttleengine.LoadConfig(stateDir, "shuttle")`, `reedengine.LoadConfig(stateDir, "reed")`, `burlerengine.LoadConfig(stateDir)`, and for perch additionally `modelspec.LoadRegistry(stateDir)` and `perchengine.LoadConfigWithRegistry(stateDir, "perch", modelReg)`.
  In hub mode every one keeps `loc.AnchorPath()`, unchanged.
- **Rationale:** this falls out of `anchorRoot = <state>` — all five loaders already take a plain `baseDir`, and T2 already made the three blocking ones degrade to their embedded template when `_lyx/` or the file is absent.
  So the common standalone case needs no config file at all, while an operator who *does* need one — reed's `tmux` and `shell` keys are genuinely machine-specific and no template default gets them right everywhere — has a deterministic, discoverable place to put it at `<state>/_lyx/config/`.
- **Rejected:** loading config from the target directory (would make a reviewed folder's contents configure the reviewer); skipping config loads entirely in standalone (loses the machine-specific reed keys).

### operator visibility — the run envelopes name the directories

- **Decision:** both run verbs' success envelopes gain three fields **in both modes**: `mode` (`"hub"` or `"standalone"`), `stateDir` (the derived `<state>` in standalone, empty string in hub mode), and `stencilsDir` (the resolved directory, populated in both).
  This is a deliberate, hub-visible output change, and it is the second named exception to hub byte-identity — recorded in that decision rather than left to be discovered.
  For burler this means `resultEnvelope` takes the values as parameters; for perch they are added to the existing `output.Ok` map alongside `runDir`/`scratchDir`/`fabricCommitted`.
- **Rationale:** T8's Watch note is explicit — the `--stencils-dir` bootstrap and the `<state>` tree both write files, so the command must say where it wrote them — and the operator-config decision depends on `<state>` being findable rather than guessed.
  The JSON envelope is the only output channel available: a stray `fmt.Println` would corrupt the envelope contract that every caller parses.
  T7 did not add this for webster; that is a gap in T7, not a precedent to copy, and it is out of scope to fix here.
  Emitting them in both modes rather than standalone-only is the deliberate call: a `mode` field that exists only in standalone cannot be used to tell the two modes apart, which is its whole purpose, and `stencilsDir` is equally worth reporting in a hub run that was pointed at an experimental stencil set via the flag.
  The fields are additive JSON keys, so no existing consumer breaks — but "no consumer breaks" is not the same as "nothing changed", which is why it is recorded as an exception rather than waved through.
- **Rejected:** printing to stderr outside the envelope (breaks the machine-readable contract for anything reading combined output); a separate `lyx burler where` verb (a whole verb for one string); scoping the three fields to standalone only (cheaper against byte-identity, but it makes `mode` self-defeating and leaves a hub `--stencils-dir` override invisible).

### `CONSTRAINTS.md` — verify, expect no edit

- **Decision:** confirm that the Stencil Ownership Invariant's read-location and seed-pass bullets and the Durable-vs-Ephemeral State Invariant's standalone bullet already cover burler and perch, then leave them unchanged.
  Edit only if some wording names webster (or `webstercli`) specifically where it should be general.
- **Rationale:** T8's brief demands these rewords land in this task's own commit rather than deferring to T10, but T7 (commit `3255efa6`, which touched `CONSTRAINTS.md` for 21 lines) already landed both in generalised, producer-agnostic form.
  Current text reads "a standalone-capable CLI's own producer resolves it under the per-OS state directory" and "The seed/refresh pass runs once per process, either at `cmd/lyx`'s root pre-run in hub mode or at the producer CLI's own pre-run in standalone mode", plus the Durable-vs-Ephemeral bullet naming `internal/standalonestate.Derive`'s state directory as a legitimate root.
  None of it names webster.
  Re-wording correct text to mention burler and perch by name would make a general invariant less general — the opposite of the intent.
- **Rejected:** editing `CONSTRAINTS.md` for its own sake to satisfy the brief's letter; deferring the verification to T10 (the brief's explicit refusal, and the verification is cheap).

### hub mode is byte-identical

- **Decision:** every hub-mode value — config base directories, `hubgeom.ReedGeometry`/`BurlerGeometry`/`PerchGeometry` outputs, `fabricengine.StencilsDir(loc.HubPath)`, `perchengine.RunsDir`/`ScratchDir` anchoring, the fabric sync's pathspec and commit message — resolves exactly as it does on `main` today.
  Byte-identity is claimed over **resolved paths and write locations**, and there are exactly **two** intentional deviations, both named here:
  1. **The plain-git-repo case.** A repository with no `<hub>/_board/_lyx` beside it moves from (fictional) hub mode to standalone.
     This is the behaviour change the whole design exists to make.
  2. **Three additive envelope fields.** `mode`/`stateDir`/`stencilsDir` appear in both modes' run-verb success envelopes (see the operator-visibility decision).
     Output-shape only: no path resolves differently and nothing new is written in hub mode.

  Any third deviation discovered during implementation is a bug in this plan, not a licence.
- **Rationale:** stated rather than smuggled, per the design.
  A wired lyx worktree is unaffected because `HubPresent` is true there.
  This must be pinned by test, since the nested-init anchoring case (`TestRunCLI_Pause_NestedInitAnchorsRunDirsAtCwd`) is exactly the kind of thing a careless refactor breaks.

## Technical context

**The reference implementation.** `internal/webstercli/wiring.go` is the file to read first and to mirror.
It contains `wire`, `wireHub`, `wireStandalone`, `setRunner`, `resolveStandaloneTarget`, and a plan-dir helper burler/perch do not need.
`internal/webstercli/cli.go`'s `resolvePersistentPreRun` (lines ~112–134) and its persistent-flag block are the `cli.go` half.
`internal/webstercli/wiring_test.go` (six tests) and `internal/webstercli/cli_integration_test.go` (two tests) are the test shapes.

**Packages this task imports and does not modify:**

- `internal/preflight` — `HubPresent(cwd) (*lyxcwd.Location, bool)` and `Wired(cwd) (*lyxcwd.Location, bool)`.
  Only `HubPresent` is used here.
- `internal/standalonestate` — `Derive(target string) (stateDir, hash8 string, err error)`.
  Called exactly once per invocation, in `wireStandalone` only.
- `internal/buildinfo` — `IsDev() bool`.
- `internal/stencilstore` — `ModeFor(dev bool) Mode` and `Reconcile(baseDir string, registry Registry, mode Mode, sourceDir string) ([]string, error)`.
- `contracts/stencils` — `Registry()`.
- `internal/hubgeom` — `ReedGeometry(*lyxcwd.Location)`, `BurlerGeometry(*lyxcwd.Location)`, `PerchGeometry(*lyxcwd.Location)`, all three already present.

**Engine constructors, already told-geometry after T3/T6:**

- `burlerengine.New(shuttle Shuttle, geom Geometry, cfg Config, stencilsDir string) *Engine`, with `Geometry{WorktreeRoot, AnchorPath string}`.
- `perchengine.New(burler Burler, shuttle Shuttle, cfg Config, geom Geometry, opts Options) *Engine`, with `Geometry{GateDir, AnchorPath string}`.
- `perchengine.RunsDir(anchorPath string) string`, `perchengine.ScratchDir(anchorPath string) string`.
- `reedengine.New(cfg, geom Geometry)`, with `Geometry{SocketKey, SessionName, AnchorPath, PaneCwd, WorktreeRoot, LogsDir, RepoName, HubPath}`.
- `shuttleengine.NewRunner(reedEngine, engine, anchorPath, worktreeRoot string, shuttleCfg)`.

**Engine geometry structs carry no stencils directory.**
Only `websterengine.Geometry` has a `StencilsDir` field; `burlerengine.Geometry` is `{WorktreeRoot, AnchorPath}` and `perchengine.Geometry` is `{GateDir, AnchorPath}`.
Burler takes `stencilsDir` as `New`'s fourth parameter and perch takes it per-call at `Engine.Run`, so both CLIs carry it as a plain string from `standalonegeom.StencilsDir(stateDir)` (standalone) or `fabricengine.StencilsDir(loc.HubPath)` (hub).
Do not copy `webstercli`'s `geom.StencilsDir` expression into either CLI — it does not compile there.

**`standalonegeom.ReedGeometry(target, stateDir, hash8)`** already produces every pinned reed value: `SocketKey: "lyx-"+hash8`, `SessionName: filepath.Base(target)+"-"+hash8`, `AnchorPath: stateDir`, `PaneCwd: target`, `WorktreeRoot: target`, `LogsDir: filepath.Join(stateDir, "logs")`, `RepoName: filepath.Base(target)`, `HubPath: stateDir`.
Reuse it as-is for both CLIs; do not re-derive.

**Files this task edits:**

| File | Change |
|---|---|
| `internal/burlercli/cli.go` | receiver fields (`engine`, two flag fields, three reporting fields), extracted `resolvePersistentPreRun`, two persistent flags, mode-aware `Long` |
| `internal/burlercli/wiring.go` | new — `wire`/`wireHub`/`wireStandalone`/`resolveStandaloneTarget` |
| `internal/burlercli/run.go` | `resultEnvelope` gains `mode`/`stateDir`/`stencilsDir` |
| `internal/burlercli/wiring_test.go` | new — tier-1 truth table and pinned values |
| `internal/burlercli/cli_test.go` | `TestRunCLI_Run_MissingProfile` — state-root redirect and stale double-failure comment |
| `internal/burlercli/cli_integration_test.go` | new — `RunCLIIn` from outside a git repo |
| `internal/perchcli/cli.go` | as burlercli, plus removing the `layout` field |
| `internal/perchcli/wiring.go` | new |
| `internal/perchcli/run.go` | three `c.layout` uses rerouted; envelope fields; nil-`openFabric` sync skip |
| `internal/perchcli/wiring_test.go` | new |
| `internal/perchcli/cli_test.go` | `TestRunCLI_Pause_MissingRunID` — state-root redirect and stale double-failure comment |
| `internal/perchcli/cli_integration_test.go` | extended with the standalone pre-run case |
| `internal/standalonegeom/burlergeom.go` | new — `BurlerGeometry` |
| `internal/standalonegeom/perchgeom.go` | new — `PerchGeometry` |
| `internal/standalonegeom/stencilsdir.go` | new — `StencilsDir(stateDir)`, the sole construction site |
| `internal/standalonegeom/webstergeom.go` | one line — inline join repointed at `StencilsDir` |
| `internal/standalonegeom/standalonegeom_test.go` | extended |
| `internal/standalonegeom/doc.go` | contract sentence updated to name the builders plus `StencilsDir` |
| `CONSTRAINTS.md` | verification pass; expected no-op |

**Gotchas found during exploration:**

- The group-command guard (`if cmd.Name() == "burler" { return nil }` / `"perch"`) must survive the extraction unchanged.
  `TestRunCLI_GroupGuard_OutsideGitRepo` exists in both packages and pins it.
- `perchcli` constructs `perchengine.New` **per invocation inside the run verb**, not in the pre-run, because its `PauseRequested` seam closes over a `scratchDir` only known after `--profile`/`--run-id` resolve.
  The wiring function therefore stores `perchGeom`, `runDirBase`, `scratchDirBase`, `perchCfg`, `modelReg`, `burlerEngine`, `runner`, and now `stencilsDir`/`anchorRel`/`openFabric` — it does not construct the perch engine.
- `runDirBase`/`scratchDirBase` are computed as `perchengine.RunsDir(perchGeom.AnchorPath)` / `perchengine.ScratchDir(perchGeom.AnchorPath)` in **both** modes.
  The nested-init comment in `cli.go` explaining why they anchor at `AnchorPath` and not `WorktreeRoot` must be preserved — `TestRunCLI_Pause_NestedInitAnchorsRunDirsAtCwd` enforces it.
- `perchcli` already has a hermetic `TestMain` (`internal/perchcli/testmain_test.go`), but it calls `gitkit.HermeticGitEnv()` only — git-config isolation, **not** state-directory isolation.
  `burlercli` has no `TestMain` at all.
  Neither package is protected from `standalonestate.Derive` reaching the operator's real home directory.
- `standalonestate.Derive` reads live `XDG_STATE_HOME` and `LOCALAPPDATA` via `os.Getenv`, and `HOME` via `os.UserHomeDir()` on the non-Windows branch.
  **Every** test that reaches `wireStandalone` — new or existing — must redirect those with `t.Setenv`, or drive the geometry builders directly.
  This is why `standalonegeom` never calls `Derive` itself (see its package doc), and it is what makes the two flipping tests named in Testing a correctness issue rather than a tidiness one.
- `internal/standalonegeom` is deliberately **not** a leaf: it imports engine packages and must not be added to `internal/buildinfo`'s or `internal/standalonestate`'s leaf-enforcement allowlists.
- `cmd/lyx/constructoranchoring_test.go` is already in post-T6 shape and needs no row change; `cmd/lyx/helptree_test.go` asserts module presence and `Short`, not flags, so it should pass untouched — but run it, since the new flags change help output.

## Constraints

From `CONSTRAINTS.md` (authoritative):

- **Cwd Resolution Invariant** — `internal/lyxcwd` alone resolves cwd.
  Neither CLI may call `os.Getwd`, spawn git, or re-derive a path; `lyxcwd.CwdFrom(ctx)` and `preflight.HubPresent` are the only entry points, and every other path is told.
- **Stencil Ownership Invariant** — prompts are read at call time from a told absolute directory, never embedded bytes; `internal/stencilstore` is the sole owner of seeding and reading; the seed pass runs once per process, at the root pre-run in hub mode or the producer CLI's own pre-run in standalone, never lazily inside `stencilstore.Read`.
- **Durable-vs-Ephemeral State Invariant** — every never-tracked file lives under `.lyx` at the mirrored subpath of its `_lyx` content; a standalone session's `_lyx` and `.lyx` are ordinary siblings under the `standalonestate.Derive` root.
  The `worktreeRoot`/`anchorRoot` split is what makes this true structurally rather than per-write-site.
- **CLI / Cobra Invariant** — `Command()`/`RunCLI`/`RunCLIIn` seam preserved; `Short` non-empty on every command; `Long` carries concrete examples and must stay accurate now that observable behaviour changes.
- **Test Tier Purity Invariant** — the truth-table tests must not spawn a process, which is the entire reason `wire` is extracted with `hubPresent` as a parameter.
- **Fabric Git Invariant** — perch's block-exit sync stays a `ScopedPathspec`, positive-pathspec commit through `internal/fabricengine`, unchanged in hub mode and absent in standalone.
- **Provider-Seam Invariant** — `burlercli`/`perchcli` remain the modules' `claudeengine` wiring point; both wiring branches call `claudeengine.New()`.
- **Documentation Lifecycle** (`CLAUDE.md`) — observable CLI behaviour changes, so docs land in the same commit.
  `manifest/designs/producers-standalone.md` is T10's to delete; this task updates it only if T8's entry needs a correction note.

Discovered:

- The design doc's T8 brief is stale on the mode trigger (see the mode-trigger decision).
  The plan should record that divergence rather than silently implement the shipped behaviour, so a later reader of the design doc is not misled.
- The design doc's T8 brief is also stale on `CONSTRAINTS.md`: T7 already landed both rewords.

## Testing

**`internal/burlercli/wiring_test.go` and `internal/perchcli/wiring_test.go` (untagged, tier 1).**
TDD candidates — write these before the wiring.
Each calls `wire` directly on a receiver the test holds, with a told `(loc, hubPresent)` pair and a `t.TempDir()`-redirected state root:

- Mode-selection truth table, all four rows explicit: `(resolved, hubPresent=true)` → hub; `(resolved, hubPresent=false)` — the plain downloaded repo — → standalone; `(unresolved, false)` → standalone; and the assertion that `wire` never itself resolves anything.
  The plain-git-repo row is the one the design's own r5 review caught, so it must be named, not folded into "not hub".
- Every pinned standalone value: `WorktreeRoot`/`GateDir` = target, `AnchorPath` = `<state>`, reed `SocketKey`/`SessionName`/`LogsDir`/`RepoName`/`HubPath`, perch `runDirBase`/`scratchDirBase` under `<state>`, config base = `<state>`, stencils default = `<state>/_lyx/stencils`.
- Hub mode resolves the same values it does today, given a told `Location`.
- `--target-dir` refused in hub mode with an error naming the reason.
- `--stencils-dir` honoured in both modes; the standalone default seeded; an explicit `--stencils-dir` never written to.
- `--target-dir` resolution: unset → cwd; absolute → cleaned; relative → joined onto cwd.
- perch only: `openFabric` non-nil in hub mode and nil in standalone; `anchorRel` = `loc.AnchorRel` in hub and `""` in standalone.

**`internal/standalonegeom` tests.**
Extend the existing table with `BurlerGeometry` and `PerchGeometry`, asserting the two-root split field by field and that neither builder touches the environment.

**Integration tests (`//go:build integration`), one per CLI.**
Drive `RunCLIIn(<temp dir outside any git repo>, out, args)` and assert the pre-run reaches the run verb's own flag validation (`burler: --profile is required` / perch's equivalent) rather than a resolution error — this is what pins the real wiring rather than the extracted helper.
Follow `internal/perchcli/cli_integration_test.go`'s existing shape.
Add a companion asserting the target directory is unchanged after the invocation — nothing hidden written into it — mirroring `TestRunCLIIn_StandalonePreRun_TargetDirectoryUnchanged`.

**Two existing untagged tests flip from "pre-run aborts" to "pre-run succeeds standalone" and must be handled explicitly.**
`internal/burlercli/cli_test.go:81` (`TestRunCLI_Run_MissingProfile`) and `internal/perchcli/cli_test.go:99` (`TestRunCLI_Pause_MissingRunID`) both `t.Chdir(t.TempDir())` and then drive a *real* subcommand (`run` / `pause`).
Today their `PersistentPreRunE` aborts because the temp dir is not a git repository, which is exactly what their doc comments call "the same documented double-failure shape as shuttlecli's `TestRunCLI_Run_FlagValidation`".
After this task `HubPresent` is false there, so the pre-run enters `wireStandalone`, calls `standalonestate.Derive` against the operator's **live** `XDG_STATE_HOME`/`HOME` (or `LOCALAPPDATA`), and `Reconcile`s a stencils tree into the operator's real state directory — from an untagged unit test.
`webstercli` never hit this because its own chdir tests stop at the group guard.
Required disposition, both tests:

- Redirect the state root before the call: `t.Setenv("XDG_STATE_HOME", t.TempDir())` **and** `t.Setenv("LOCALAPPDATA", t.TempDir())`, so both `Derive` branches land inside the test's own temp tree on every platform.
  `gitkit.HermeticGitEnv()` in `internal/perchcli/testmain_test.go` does **not** cover this — it isolates git config only — and `internal/burlercli` has no `TestMain` at all.
- Update both doc comments: the double-failure shape is gone, since the pre-run now succeeds and only the verb's own flag error is emitted.
  Leaving the stale rationale in place would teach the next reader a behaviour the code no longer has.
- The assertions themselves (`exitCode == 1`, output contains `--profile is required` / `--run-id is required`) still hold and should stay — what changes is the surrounding output, not the flag-validation contract each test exists to pin.

**Regression coverage that must keep passing unchanged:**
`TestRunCLI_GroupGuard_OutsideGitRepo` (both packages — the group guard returns before any wiring, so these are genuinely untouched), `TestCommand_EveryCommandHasShort` (both), `TestRunCLI_Pause_NestedInitAnchorsRunDirsAtCwd`, and the `cmd/lyx` help-tree and constructor-anchoring suites.

**Verify command:**
`go test ./internal/burlercli/... ./internal/perchcli/... ./internal/standalonegeom/... ./cmd/lyx/...`, then `go test -tags integration ./internal/burlercli/... ./internal/perchcli/...`, then `go test ./...` as the task-wide gate.
Smoke check on top of the tests, never instead of them: `lyx burler run --profile p.yaml --stencils-dir <dir>` from a scratch directory outside any git repository.

## Q&A log

- **Q:** Should the hub/standalone trigger follow T8's brief (`fabricengine.Ready`-class plus a `_board` discriminator) or the shipped `preflight.HubPresent` that T5 and T7 landed? **A:** [auto-pick] Follow `preflight.HubPresent`. **Why:** `internal/preflight/doc.go` documents the override as considered, not accidental; `Ready` probes the paired sibling rather than the hub and is false in three healthy hub situations, so keying on it would relocate live hub state into `<state>` — and burler/perch must not select modes differently from webster in the same tree.
- **Q:** Extract the wiring into a per-CLI `wiring.go` with `wire`/`wireHub`/`wireStandalone`, or branch inline in `PersistentPreRunE`? **A:** [auto-pick] Extract per CLI, mirroring `webstercli`. **Why:** the truth-table test must stay tier 1, and driving the real pre-run reaches `lyxcwd.Resolve`'s git spawn, breaching Test Tier Purity.
- **Q:** Where do the standalone burler/perch geometry structs get built? **A:** [auto-pick] New `BurlerGeometry`/`PerchGeometry` in `internal/standalonegeom`. **Why:** `hubgeom` already holds the hub twins and `standalonegeom` already holds `ReedGeometry`/`WebsterGeometry`; inline construction is what the design explicitly rules out.
- **Q:** Which flags, on which commands, with what mode semantics? **A:** [auto-pick] `--stencils-dir` and `--target-dir` as persistent flags on both parents; `--stencils-dir` honoured read-only in both modes, `--target-dir` standalone-only and a hard error in hub mode; no `--plan-dir`. **Why:** the read/write asymmetry is the design's stated reason for the opposite rulings, and burler/perch parse no plan.
- **Q:** What happens to perch's three `c.layout` uses in standalone? **A:** [auto-pick] Delete the field; store told `stencilsDir` + `anchorRel` and a nil-able `openFabric`, skipping the block-exit fabric sync when nil. **Why:** matches `webstercli`'s shipped shape and keeps "no `Location` on the receiver" literally true; standalone has no fabric repo to open, so nil is honest and a stat-failing closure is not.
- **Q:** Hard-error or best-effort when the standalone stencils `Reconcile` fails? **A:** [auto-pick] Hard error in the pre-run. **Why:** unlike the root pre-run's logged best-effort, nothing else will ever create this directory, so the failure would resurface much later as an opaque render error.
- **Q:** Is an explicitly-passed `--stencils-dir` ever seeded? **A:** [auto-pick] Never, in either mode. **Why:** it is what makes the read-only characterisation literally true and protects a curated stencil set from being reconciled out from under the operator.
- **Q:** Does `CONSTRAINTS.md` need the Stencil Ownership and Durable-vs-Ephemeral rewords this brief demands? **A:** [auto-pick] Verify only; expect no edit. **Why:** T7's commit `3255efa6` already landed both in generalised, producer-agnostic wording that names no module — re-wording it to name burler and perch would make a general invariant less general.
- **Q:** How does the operator learn where `<state>` and the seeded stencils landed? **A:** [auto-pick] Add `mode`/`stateDir`/`stencilsDir` to both run verbs' success envelopes. **Why:** T8's Watch note requires the command to say where it wrote; the JSON envelope is the only channel that does not corrupt the machine-readable output contract.
- **Q:** What test tiers pin the behaviour? **A:** [auto-pick] One untagged tier-1 `wiring_test.go` per CLI for the truth table and pinned values, plus one `//go:build integration` test per CLI driving `RunCLIIn` outside a git repo. **Why:** the design names both tiers as required, and the untagged tier is only reachable through the wiring extraction.
- **Q:** Keep a nil-able `layout` field for convenience? **A:** [auto-pick] No — remove it entirely from both CLIs. **Why:** a nil-able `*lyxcwd.Location` on a receiver is the fictional-`Location` shape the whole design was written to eliminate, and it invites a later dereference no compiler catches.
- **Q:** May hub-mode behaviour shift at all? **A:** [auto-pick] Byte-identical over resolved paths and write locations, with exactly two named exceptions. **Why:** the plain-git-repo reclassification is the change the design exists to make, and the three additive envelope fields are an output-shape-only change; both are named in the byte-identity decision so no third deviation can slip in as precedent.
- **Q:** (r1 gap) Who constructs the standalone stencils directory, given that neither `burlerengine.Geometry` nor `perchengine.Geometry` has a `StencilsDir` field? **A:** A new `standalonegeom.StencilsDir(stateDir string) string`, the sole construction site, with `standalonegeom.WebsterGeometry` repointed at it. **Why:** the first draft copied `webstercli`'s `geom.StencilsDir` expression, which does not compile for burler or perch; both engines already accept `stencilsDir` as a told parameter, so a geometry field would be a competing home for a value they already take, and a per-CLI inline join would put the same literal in three places.
