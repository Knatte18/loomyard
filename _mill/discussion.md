# Discussion: shuttleengine + reedengine + tokenvocab told-geometry

```yaml
task: shuttleengine + reedengine + tokenvocab told-geometry
slug: shuttle-reed-told-geometry
status: discussing
parent: standalone-producers
```

## Problem

The three lowest-level producer packages — `internal/shuttleengine`, `internal/reedengine`, `internal/tokenvocab` — each take a `*lyxcwd.Location` and derive the directories they need from it.
That makes them unrunnable outside a fully-initialized lyx hub: there is no `Location` to hand them when `lyx` is pointed at a plain downloaded repo, and the only way to call them anyway is to fake one.

Faking one is not safe here, which is why this is a task rather than a convenience.
`reedengine`'s `New` derives the **tmux socket name** from `Location.HubPath` (`lock.go:43`, via `socketName`).
A synthetic `Location` with a plausible-looking `HubPath` silently names the socket after whatever directory happens to be the parent of the target, so two standalone runs in sibling folders collide or share a tmux server in ways nobody designed — and no compiler and no test catches it.

**Why now.** This is task **T3** of `manifest/designs/producers-standalone.md`, wave 1, and it depends on nothing.
It is on the critical path for T7 (`websterengine` standalone) and T8 (`burlercli`/`perchcli` standalone), both of which construct these engines.
The parent branch is `standalone-producers`.

This task is the **producer layer** half of the told-geometry decision: engine constructors take the directories they actually read, as plain absolute strings (or a per-engine struct when the count would exceed a readable positional list).
The engines get no `HubPath`-derivation, no `RepoName` derivation, and no concept of a hub — those values are *told* to them.

## Scope

**In:**

- `internal/shuttleengine` — `NewRunner` and `FindRun` take `anchorPath, worktreeRoot string` / `anchorPath string` instead of `*lyxcwd.Location`; `runDirRoot` takes `anchorPath string`. `internal/lyxcwd` leaves the package's production imports.
- `internal/reedengine` — `New(cfg Config, geom Geometry)`, where `Geometry` is reed's own told-geometry struct (7 fields, enumerated below). `internal/lyxcwd` **and** `internal/fabricengine` both leave the package's production imports.
- `internal/tokenvocab` — `Ctx.Layout *lyxcwd.Location` becomes two plain fields, `RepoName` and `HubPath`. `internal/lyxcwd` leaves the package's production imports and its leaf-enforcement allowlist.
- **New package `internal/hubgeom`** — the single hub-mode `*lyxcwd.Location` → told-geometry conversion, exporting `ReedGeometry(*lyxcwd.Location) reedengine.Geometry` today and gaining per-engine siblings in T6/T7.
- `reedengine.HubLogsDir` **moves** to `fabricengine.HubLogsDir(hubPath string)`.
- The five hub-mode construction sites: `internal/burlercli/cli.go` (103-104), `internal/perchcli/cli.go` (143-144), `internal/webstercli/cli.go` (179-181), `internal/shuttlecli/cli.go` (92-93), `internal/reedcli/cli.go` (83).
- The two **production** callers of `shuttleengine.FindRun` in `internal/websterengine`: `recoverbatch.go:182` and `runlevel.go:529` — one-token edits (`deps.Layout` becomes `deps.Layout.AnchorPath()`).
- **Every out-of-package caller of the four changed exported symbols**, enumerated symbol-by-symbol (see the Technical context call-site table). Beyond the five CLI construction sites and the two `websterengine` production files above, that is eleven test files: `internal/shuttlecli/cli_test.go:237`, `internal/shuttlecli/smoke_interrupt_test.go:280,282`, `internal/webstercli/verbs_test.go:227`, `internal/websterengine/recoverbatch_test.go:189`, `internal/treadleengine/smoke_judge_test.go:288,289`, `internal/burlerengine/smoke_cluster_test.go:135,136`, `internal/burlerengine/smoke_round_test.go:317,318`, `internal/fabricengine/hubscratch_test.go:70`, `internal/reedcli/smoke_debuglog_test.go:40,169`, and `cmd/lyx/constructoranchoring_test.go:96,144`.
- `cmd/lyx/constructoranchoring_test.go` — its `reedengine.HubLogsDir` rows (96 and 144) retarget onto `fabricengine.HubLogsDir`.
- `CONSTRAINTS.md` — two invariants reworded (see Constraints below).
- Docs: `internal/tokenvocab/doc.go`, `internal/reedengine/doc.go`, `internal/shuttleengine/doc.go`, a new `internal/hubgeom/doc.go`, `docs/overview.md`, `docs/shared-libs/README.md`.
- `manifest/designs/producers-standalone.md` — three edits only: a one-line pointer to `internal/hubgeom` in the **T6** and **T7** entries, and a correction to **row 64** of the Location-consumption table (which attributes `HubLogsDir` to `reedengine/lifecycle.go`). Nothing else in that doc is edited (see the design-doc decision below).

**Out:**

- **No standalone CLI entry point.** This task changes signatures only; nothing gains a `--target` flag, a `--stencils-dir` flag, or a standalone mode. Those are T7 and T8.
- **No `internal/preflight`, `internal/buildinfo`, or `internal/standalonestate`.** Those are T5's deliverables; nothing here depends on them.
- **No config-loader change.** `LoadOrTemplate` is T2. The `LoadConfig` calls at each construction site keep their present `layout.AnchorPath()` argument and their present strict behaviour.
- **`internal/burlerengine`, `internal/perchengine`, `internal/websterengine`, `internal/scoutengine` are not converted.** T6, T7 and T9 own those. This task touches `websterengine` in exactly two **production** files (`recoverbatch.go`, `runlevel.go`) and only because they call the changed `shuttleengine.FindRun`; those edits neither require nor anticipate T7.
  One `websterengine` **test** file is also touched — `recoverbatch_test.go:189` calls `shuttleengine.NewRunner` — which is a signature fixup, not a conversion of the package.
  T7 rewrites `recoverbatch.go`/`runlevel.go` wholesale and must land after this task.
- **No new named told-geometry invariant** and no new allowlist-enforcement test for `reedengine` or `shuttleengine`. T10 owns naming the cross-cutting rule.
- **No `manifest/roadmap.md` edit.** T3 is one wave of an already-listed item; `CLAUDE.md` reserves roadmap moves for completing or adding a planned item.
- **No rewrite of T3's own Files/Verify lines** in `manifest/designs/producers-standalone.md`, and no edit to that file beyond the three named above (T6 pointer, T7 pointer, row 64).
- **No additive twins.** The old `*lyxcwd.Location` signatures are replaced, never kept beside the new ones as wrappers (`producers-standalone.md`, "no additive twins" decision).

## Decisions

### reedengine.Geometry — seven fields, including WorktreeRoot

- **Decision.** `reedengine.New(cfg Config, geom Geometry) *Engine`, where:

  | Field | Hub-mode source | Consumer |
  |---|---|---|
  | `SocketKey` | `reedengine.ServerName(l.HubPath)` | the tmux `-L` socket name (`lock.go:43`, `Engine.Socket`) |
  | `SessionName` | `reedengine.SessionName(l.WorktreePath())` | the tmux session name (`Engine.SessionName`) |
  | `AnchorPath` | `l.AnchorPath()` | `stateDir()` (`reed.json`/`reed.lock`, `lifecycle.go:44`), pane spawn cwd (`lifecycle.go:305`, `lifecycle.go:500`) |
  | `WorktreeRoot` | `l.WorktreePath()` | `Strand.Worktree` and `resolveStrandName`'s `<WORKTREE>` token (`strand.go:175-176`) |
  | `LogsDir` | `fabricengine.HubLogsDir(l.HubPath)` | the shared server's runtime log dir (`lifecycle.go:256`) |
  | `RepoName` | `l.RepoName` | the header pane's `repo` token, via `tokenvocab` |
  | `HubPath` | `l.HubPath` | the header pane's `hub` token, via `tokenvocab` |

- **Rationale.** `WorktreeRoot` is **not** in the design doc's T3 table — that is an omission found during exploration, not a scope addition.
  `strand.go` uses `e.layout.WorktreePath()` twice: once to stamp `Strand.Worktree` (persisted into `reed.json`, `state.go:21`) and once for `resolveStrandName`'s `<WORKTREE>` substitution (`strand.go:124-137`, `filepath.Base(worktreeRoot)`).
  With `SessionName` told rather than derived, the worktree root is no longer recoverable from anything else in the struct, so both would silently degrade.
  `SessionName` stays told rather than being computed from `WorktreeRoot` because T5 requires standalone entries to name the session from the shared `hash8`, which is unrelated to the target directory's basename.
  A struct rather than a positional parameter list because seven strings positional is exactly the smell "told-geometry structs per engine" exists to avoid.
- **Rejected.** (a) Carrying `WorktreeRoot` only and deriving `SessionName = filepath.Base(WorktreeRoot)` inside reed — reed re-derives geometry it was supposed to be told, and standalone loses control of the session name.
  (b) Carrying told `SessionName` only and stamping `Strand.Worktree` from `AnchorPath` — a behaviour change in a persisted field for no gain.

### Naming — `AnchorPath`, not `AnchorRoot`; `WorktreeRoot` keeps `Root`

- **Decision.** The `Location.AnchorPath()`-derived value is spelled **`AnchorPath`** (struct field) and **`anchorPath`** (parameter) everywhere in this task.
  The `Location.WorktreePath()`-derived value keeps **`WorktreeRoot`** / **`worktreeRoot`**.
  This overrides the T3 brief's own spelling in `manifest/designs/producers-standalone.md:291-292`, which says `anchorRoot`.
- **Rationale.** `CONSTRAINTS.md`'s Cwd Resolution Invariant is explicit: "`root` always means the git worktree/repo root; the current working directory is `cwd`. Never name a parameter, field, or local variable `root` for a value that is actually `cwd`, or vice versa."
  `AnchorPath()` is exactly the value cwd is gated to equal (`cwd must equal AnchorPath() exactly`, same invariant), so naming it `AnchorRoot` is the precise inversion the rule bans.
  `manifest/designs/fabric-unified-view.md:61-65` repeats the rule and records why it exists — cwd/root confusion was a recurring real defect source in this codebase, including in generated code — and the same decomposition doc's **T4** already spells the identical concept `anchorPath` (line 330-331).
  Pinning `anchorPath` therefore matches CONSTRAINTS, matches T4, and keeps one spelling across the two wave-1/wave-2 tasks converting the same value.
  `WorktreeRoot` is left alone because it *is* the git worktree root — the one thing `root` is reserved for.
- **Rejected.** (a) Following the T3 brief's `anchorRoot` verbatim — `CONSTRAINTS.md` is authoritative over a task brief, and the brief's own sibling task contradicts it.
  (b) Spelling it `cwd`, per `fabric-unified-view.md:63`'s advice for modules joining onto cwd — reed and shuttle are *told* this path by a caller and never read a process working directory, so `cwd` would misdescribe it; `AnchorPath` names the resolved concept `lyxcwd` already exports.
- **Consequence.** `hubgeom.ReedGeometry` sets `AnchorPath: l.AnchorPath()`, and the two-string shuttle signature reads `anchorPath, worktreeRoot string`.

### HubLogsDir moves to fabricengine

- **Decision.** Delete `reedengine.HubLogsDir(l *lyxcwd.Location)` (`lifecycle.go:35-37`) and add `fabricengine.HubLogsDir(hubPath string) string`, returning `filepath.Join(HubScratchDir(hubPath), "logs")`.
  `reedengine` is then told its logs directory as `Geometry.LogsDir`; `lifecycle.go:256`'s `logsDir := HubLogsDir(e.layout)` becomes `logsDir := e.geom.LogsDir`.
- **Rationale.** `fabricengine.HubScratchDir` is `reedengine`'s **only** `internal/fabricengine` reference, so this removes `internal/fabricengine` from reed's import set outright — and with it the `treadleengine` → `shuttleengine` → `reedengine` → `fabricengine` transitive path the Treadle Runner-Seam Invariant currently has to acknowledge as real.
  `fabricengine` already owns `HubScratchDir`, so the derivation stays named and lives beside its base; the `cmd/lyx/constructoranchoring_test.go` rows 96/144 retarget rather than die, and the four existing callers get a one-token edit.
- **Rejected.** (a) Deleting it and inlining `filepath.Join(fabricengine.HubScratchDir(hub), "logs")` at every caller — four sites re-deriving a path, against the repo's own told-never-derives rule that `HubScratchDir`'s comment states.
  (b) Keeping it in `reedengine` with a `hubPath string` parameter — cheapest, but keeps `fabricengine` in reed's imports and forfeits the invariant rewording this task is meant to deliver.

### internal/hubgeom — one hub-mode teller, reused, never duplicated

- **Decision.** A new package `internal/hubgeom` holds the hub-mode `*lyxcwd.Location` → told-geometry conversion.
  It exports `ReedGeometry(l *lyxcwd.Location) reedengine.Geometry` today, populating all seven fields from the table above.
  All nine hub-mode call sites (five CLI construction sites plus four smoke tests) use it; none builds the struct itself.
  `hubgeom` imports `internal/lyxcwd`, `internal/reedengine` and `internal/fabricengine`; the engines it serves import none of it, so the told direction is preserved.
- **Rationale.** The operator's standing rule is *reuse, never duplicate*.
  Without a shared teller, nine sites each repeat a seven-field literal mixing `AnchorPath()`, `WorktreePath()` and `HubPath` — and a swapped anchor/worktree compiles cleanly and fails silently, which is the single failure mode this whole refactor introduces.
  A generic name (`hubgeom`, not `reedgeom`) is chosen so T6 (`burlerengine`/`perchengine`) and T7 (`websterengine`) add `BurlerGeometry`/`PerchGeometry`/`WebsterGeometry` to the same package instead of forcing a rename or spawning three sibling packages.
  Standalone CLIs (T7/T8) simply do not call it — they have no `Location` — which is the point: `hubgeom` is the hub-mode teller, not a dependency of the engines.
- **Governance note — this task names a package two later waves will extend.** `internal/hubgeom` appears nowhere in `manifest/designs/producers-standalone.md`; it is new architecture introduced by a wave-1 task whose dependency line reads "Depends on. Nothing."
  T6 and T7 have not been discussed yet and have had no chance to weigh in on the name or shape.
  That is accepted deliberately, with one explicit limit: **`hubgeom` binds nothing for T6/T7.**
  Its contract today is exactly one exported function, `ReedGeometry`.
  If T6's or T7's own discussion concludes a different home, name or shape fits better, renaming a package with one function and nine call sites is cheap, and this task claims no veto over that.
  What T3 does claim is that *this* task's nine sites get one teller rather than nine copies.
- **Rejected.** (a) Inlining the literal at all nine sites — faithful to the design doc's Files list, but nine copies of the same six derivations and no anti-swap guard.
  (b) `internal/reedgeom` with per-engine sibling packages later — tighter naming, four packages by T7, same conversion pattern written four times.
  (c) A single `hubgeom.All(*lyxcwd.Location)` returning every engine's geometry at once — forces `hubgeom` to import every engine and every CLI to pull the whole set.
  (d) Putting the helper on `internal/hubforge` for tests and inlining it in the CLIs — splits one derivation into two homes.

### SocketKey is trusted, not validated

- **Decision.** `reedengine.New` keeps its `(*Engine)`-only return; it stores `Geometry` verbatim and validates nothing.
  `Engine.Socket()` returns `geom.SocketKey`, `Engine.SessionName()` returns `geom.SessionName`.
  The obligation to pass a socket-safe key is stated in `New`'s doc comment, naming `reedengine.ServerName(hubPath)` as the hub-mode answer.
  `ServerName`, `SessionName` and `socketName` stay exactly as they are in `server.go` — `ServerName` is the sanitizing derivation `hubgeom` calls.
- **Rationale.** Matches the existing total-function style of `ServerName`/`SessionName`, which never fail.
  Adding validation would give `New` an error return that ripples through all nine construction sites and every `newTestEngine`-style fixture, for a class of bug that only a caller bypassing `hubgeom` and `ServerName` could produce.
- **Rejected.** (a) `New` returning `(*Engine, error)` after checking non-empty + socket-safe charset.
  (b) `New` silently sanitizing the key — hides mistakes instead of surfacing them.

### shuttleengine takes two plain strings, sourced from the same Geometry

- **Decision.** `NewRunner(reed ReedOps, engine Engine, anchorPath, worktreeRoot string, cfg Config) *Runner`; `FindRun(cfg Config, anchorPath, guid string)`; `runDirRoot(cfg Config, anchorPath string)`.
  No `shuttleengine.Geometry` struct — two values do not warrant one.
  At each CLI site the two arguments come from the `reedengine.Geometry` already built on the adjacent line: `hubgeom.ReedGeometry(layout)` is called once, `reedengine.New(reedCfg, g)` and `shuttleengine.NewRunner(reedEngine, claudeengine.New(), g.AnchorPath, g.WorktreeRoot, shuttleCfg)` both read from it.
- **Rationale.** The design doc's T3 brief specifies this signature explicitly.
  Sourcing both from the single `Geometry` value keeps the anchor/worktree pairing decided in exactly one place rather than re-derived beside a struct that already holds both.
- **Rejected.** (a) `layout.AnchorPath(), layout.WorktreePath()` inline at each CLI site — reintroduces the swap hazard next to a struct that already has the answer.
  (b) A `shuttleengine.Geometry` struct plus `hubgeom.ShuttleGeometry` — symmetric, but over-structured for two values.

### tokenvocab.Ctx becomes two plain fields

- **Decision.** `Ctx{RepoName, HubPath string}`; the registry's two tokens resolve `c.RepoName` and `c.HubPath` directly.
  `reedengine/header.go:16` becomes `tokenvocab.Ctx{RepoName: e.geom.RepoName, HubPath: e.geom.HubPath}`.
  `internal/tokenvocab/leaf_enforcement_test.go`'s `allowedImports` drops `internal/lyxcwd`, leaving stdlib plus `internal/stencil`.
- **Rationale.** The registry's only two tokens are exactly those fields (`tokenvocab.go:25-26`), so the `Location` buys nothing.
  Dropping it removes `internal/lyxcwd` from the package entirely — `tokenvocab` becomes a stdlib-plus-`stencil` leaf.
  `reedengine/header.go` is `tokenvocab`'s only consumer in the repo, so the change is fully contained.
  `RepoName`/`HubPath` are carried in `Geometry` precisely so `Engine.HeaderText` keeps rendering real values instead of empty tokens.
- **Rejected.** Dropping the two fields from `Geometry` and letting the header render empty tokens — a silent regression in the header pane at every boot.

### Enforcement scope stays at the two existing invariants

- **Decision.** Reword the two named invariants in `CONSTRAINTS.md` (Tokenvocab Leaf, Treadle Runner-Seam) and tighten `tokenvocab/leaf_enforcement_test.go`'s allowlist.
  Do **not** add new allowlist/banned-import enforcement tests for `reedengine` or `shuttleengine`, and do not introduce a new named told-geometry invariant.
- **Rationale.** T10 of `producers-standalone.md` owns naming the cross-cutting told-geometry rule; adding a half-named version here would have to be renamed or merged later.
  The `tokenvocab` allowlist is different — the T3 Verify line explicitly requires it be tightened rather than left permissive, and it is an existing test, not a new invariant.
- **Rejected.** (a) Adding enforcement tests for `reedengine`/`shuttleengine` now — locks the win in immediately but pre-empts T10's naming.
  (b) Rewording invariants without tightening the `tokenvocab` allowlist — leaves the invariant text and its enforcer disagreeing.

### Design doc is not rewritten, but T6/T7 get a breadcrumb

- **Decision.** Do not rewrite T3's **Files** or **Verify** lines in `manifest/designs/producers-standalone.md`.
  Do add a **one-line pointer** to `internal/hubgeom` in that doc's **T6** and **T7** entries, naming the package and what it exports, so a future explorer reading only the design doc finds it.
  Also correct **row 64** of that doc's Location-consumption table (line 64), which attributes `HubLogsDir` to `internal/reedengine/lifecycle.go` — false once the function lives in `fabricengine`.
  Those three edits are the complete permitted edit set for that file.
  `internal/hubgeom/doc.go` carries the same statement — that it is the hub-mode teller, that engines never import it, and that later waves add their own `*Geometry` functions beside `ReedGeometry` — since a grep for the package name lands there first.
- **Rationale.** The decomposition doc is the plan of record for T6-T10, and `CLAUDE.md`'s same-commit docs rule targets module docs, `docs/overview.md` and `CONSTRAINTS.md`, not a task-decomposition doc — so a wholesale rewrite of T3's entry is out of scope.
  A pointer is a different, smaller thing, and it addresses a concrete risk rather than a bookkeeping one: without it, whoever spawns T6 or T7 from the design doc alone never learns `hubgeom` exists and re-derives the seven-field construction inline — exactly the duplication this task spent a package avoiding.
  Two lines is the cheapest possible fix for that.
- **Known cost, flagged deliberately.** T3's Files and Verify lines in that doc stay incomplete as written: they omit `WorktreeRoot`, `internal/hubgeom`, and the six test files in `treadleengine`/`burlerengine`/`fabricengine`/`reedcli`/`shuttlecli` listed under Scope above.
  Anyone reading the doc to plan T6 or T7 should treat this discussion file as the corrected record for T3.

## Technical context

**Current shapes.**

- `internal/shuttleengine/run.go:29,35-36` — `Runner{reed, engine, layout *lyxcwd.Location, cfg}`; `NewRunner(reed, engine, layout, cfg)`.
  `layout` is read at `run.go:89` (`r.layout.WorktreePath()` → `spec.validate`), `run.go:95`, `run.go:186` (`filepath.Join(r.layout.AnchorPath(), lyxdirs.DotLyxDirName)` → `reedengine.LoadState`), `run.go:201`, `run.go:248/264/282` (`FindRun`), and `wait.go:257` (`run.runner.layout.AnchorPath()` → `AuditForks`).
  Exactly two accessors are used across the package: `AnchorPath()` and `WorktreePath()`.
- `internal/shuttleengine/rundir.go:49-57` — `runDirRoot(cfg, layout)` joins on `layout.AnchorPath()` in both branches, deliberately (the doc comment explains why one function must not resolve against two bases).
  `rundir.go:150-152` — `FindRun(cfg, layout, guid)`.
- `internal/reedengine/lock.go:32-55` — `Engine{cfg, layout, tmux}`; `New` builds `NewTmuxCmd(cfg.Tmux, socketName(layout.HubPath))`.
  `Socket()` and `SessionName()` re-derive from `e.layout` on every call.
- `internal/reedengine/lifecycle.go:35-45` — `HubLogsDir(l)` and `stateDir()`.
  `lifecycle.go:256, 305, 500` are the three `e.layout` reads in the file.
- `internal/reedengine/strand.go:175-176` — the two `WorktreePath()` reads.
- `internal/reedengine/server.go` — `ServerName(hubPath)` (sha256-suffixed, socket-safe), `SessionName(worktreeRoot)` (= `filepath.Base`), unexported `socketName` (= `ServerName`).
  All three stay; only their *callers* move outward.
- `internal/tokenvocab/tokenvocab.go:7-27` — `Ctx{Layout *lyxcwd.Location}` and the two-token registry.
- `internal/tokenvocab/render.go` — `Render(template []byte, c Ctx)`, unchanged in shape.

**Call-site table — every out-of-package reference to a changed exported symbol.** Produced by a symbol-by-symbol grep over all four (`shuttleengine.NewRunner`, `shuttleengine.FindRun`, `reedengine.New`, `reedengine.HubLogsDir`), not by enumerating construction sites only.

| Symbol | Production call sites | Test call sites |
|---|---|---|
| `shuttleengine.NewRunner` | `burlercli/cli.go:104`, `perchcli/cli.go:144`, `webstercli/cli.go:181`, `shuttlecli/cli.go:93` | `shuttlecli/cli_test.go:237`, `shuttlecli/smoke_interrupt_test.go:282`, `webstercli/verbs_test.go:227`, `websterengine/recoverbatch_test.go:189`, `treadleengine/smoke_judge_test.go:289`, `burlerengine/smoke_cluster_test.go:136`, `burlerengine/smoke_round_test.go:318` |
| `shuttleengine.FindRun` | `websterengine/recoverbatch.go:182`, `websterengine/runlevel.go:529` | in-package `shuttleengine` tests only (`runlevel_test.go:215` is a comment, not a call) |
| `reedengine.New` | `burlercli/cli.go:103`, `perchcli/cli.go:143`, `webstercli/cli.go:179`, `shuttlecli/cli.go:92`, `reedcli/cli.go:83` | `shuttlecli/smoke_interrupt_test.go:280`, `treadleengine/smoke_judge_test.go:288`, `burlerengine/smoke_cluster_test.go:135`, `burlerengine/smoke_round_test.go:317`, plus in-package `reedengine` tests |
| `reedengine.HubLogsDir` | none | `fabricengine/hubscratch_test.go:70`, `reedcli/smoke_debuglog_test.go:40,169`, `cmd/lyx/constructoranchoring_test.go:96,144` |

Note the pairing: every file that calls `reedengine.New` out of package calls `shuttleengine.NewRunner` on the very next line, except `reedcli` (reed only) and the four `NewRunner`-only test files.
That pairing is what makes one `hubgeom.ReedGeometry(layout)` per site feed both constructors.

**Construction sites, present form.** All five follow the same pattern: resolve `layout`, `reedengine.LoadConfig(layout.AnchorPath(), "reed")`, then `reedengine.New(reedCfg, layout)` and (except `reedcli`) `shuttleengine.NewRunner(reedEngine, claudeengine.New(), layout, shuttleCfg)`.
`burlercli` and `perchcli` additionally pass `layout` to `burlerengine.New` — that argument is **T6's**, left untouched here.
`perchcli` and `webstercli` also keep `c.layout = layout` for their own use — also untouched.

**Test fixtures.** `internal/reedengine/lock_test.go:22-35` defines `newTestEngine(t)`, the shared fixture nearly every test in the package builds on; it constructs a `Location` over a `t.TempDir()` and calls `New(cfg, layout)`.
Direct `New(cfg, layout)` calls also appear at `contract_integration_test.go:408,507`, `header_test.go:20`, `lock_test.go:52`, `mouse_boot_integration_test.go:51,126`.
Converting `newTestEngine` to build a `Geometry` covers most of them.
`mouse_boot_integration_test.go:126` reads `e1.layout` directly and becomes `e1.geom`.

Outside the package, the four smoke tests use `hubforge` fixtures and pass `h.Location` (`internal/hubforge/hub.go:147`); those become `hubgeom.ReedGeometry(h.Location)`.

**Gotcha — `cmd/lyx/constructoranchoring_test.go`.** A flat table of `assertPath` rows and the named enforcer of the Durable-vs-Ephemeral State Invariant.
Per `producers-standalone.md`, it is edited in place, per task, never split or retired.
This task rewrites rows 96 and 144 (`reedengine.HubLogsDir` → `fabricengine.HubLogsDir`) and nothing else; the file header comment at line 17 and the section comment at line 94 both name `reedengine.HubLogsDir` and need the same retarget.
T1 edits rows 71-72/120-121 in the same wave — far enough apart that no merge adjacency arises.

**Gotcha — `internal/fabricengine/hubscratch_test.go:57-70`** currently imports `reedengine` to assert `HubLogsDir`'s `MkdirAll` idempotency against a fabric-created `.lyx`.
Once `HubLogsDir` lives in `fabricengine`, that test no longer needs the `reedengine` import.

**Gotcha — spawn observability.** `lifecycle.go` is one of the four known instrumented call sites under the Live-Substrate Spawn Observability invariant (`CONSTRAINTS.md:401`).
Every `logger.Info`/`logger.Warn` call in the touched spawn paths (`lifecycle.go:258, 268, 272, 279, 315`, and the surrounding boot loop) must survive the refactor byte-identical in intent.

**Import-graph effect after this task.**

- `internal/tokenvocab` → stdlib + `internal/stencil`.
- `internal/reedengine` → loses both `internal/lyxcwd` and `internal/fabricengine`.
- `internal/shuttleengine` → loses `internal/lyxcwd`.
- New: `internal/hubgeom` → `internal/lyxcwd`, `internal/reedengine`, `internal/fabricengine`.
- The `treadleengine` → `shuttleengine` → `reedengine` → `fabricengine` transitive path ceases to exist.

## Constraints

From `CONSTRAINTS.md`:

- **Tokenvocab Leaf Invariant** (line 133) — currently allows stdlib, `internal/lyxcwd`, `internal/stencil`.
  **Reworded in this commit** to stdlib + `internal/stencil`.
  Enforced by `internal/tokenvocab/leaf_enforcement_test.go` (`TestLeafInvariant_AllowlistOnly`), whose `allowedImports` map must be tightened to match.
- **Treadle Runner-Seam Invariant** (line 109) — its final bullet asserts that `internal/treadleengine` → `internal/shuttleengine` → `internal/reedengine` → `internal/fabricengine` "is a real transitive path (`reedengine.HubLogsDir` calls `fabricengine.HubScratchDir`)".
  **Reworded in this commit**: that path no longer exists.
  The invariant's allowlist itself does not change.
- **Live-Substrate Spawn Observability** (line ~401) — binds every touched spawn path in `internal/reedengine/lifecycle.go` and `internal/shuttleengine/run.go`; the `logger` calls must survive intact.
- **Cwd Resolution Invariant** — two halves bind here.
  (a) `internal/lyxcwd` owns cwd resolution alone: this task moves *consumption* of a resolved `Location` outward to `internal/hubgeom` and the CLI layer, and must not add any `os.Getwd`, git discovery, or path resolution to `hubgeom`, which only reads accessors off a `Location` its caller already resolved.
  (b) The naming half — "`root` always means the git worktree/repo root … never name a parameter, field, or local variable `root` for a value that is actually `cwd`" — governs every new field and parameter this task introduces.
  Since `cwd must equal AnchorPath() exactly`, the anchor-derived value is named `AnchorPath`/`anchorPath` and never `AnchorRoot`/`anchorRoot`; see the naming decision above.
  `manifest/designs/fabric-unified-view.md:61-65` records why the rule exists and is worth re-reading before naming anything here.
- **CLI/Cobra Invariant** — the module `Command()`/`RunCLI` seam is unchanged by this task; no command is added, removed, or renamed, so the help-tree tests should not move.
- **Durable-vs-Ephemeral State Invariant** — enforced by `cmd/lyx/constructoranchoring_test.go`; the anchoring of every path must be byte-identical before and after. `HubLogsDir`'s value stays `<hub>/_board/.lyx/logs` regardless of which package computes it.
- **Documentation Lifecycle** / `CLAUDE.md` same-commit docs rule — a task introducing cross-cutting infrastructure updates `docs/overview.md` and `CONSTRAINTS.md` in the same commit.

Discovered during discussion:

- **No additive twins.** The old `*lyxcwd.Location` signatures are replaced outright. No wrapper, no deprecation window (`producers-standalone.md`, "no additive twins").
- **Behaviour must be byte-identical in hub mode.** Every derived value — socket name, session name, run-dir root, state dir, logs dir, strand name, `Strand.Worktree`, header tokens — resolves to exactly what it resolves to today when the geometry comes from `hubgeom.ReedGeometry(layout)`.

## Testing

**TDD candidate — `internal/hubgeom`.** Write this first, before any signature change.
A table test over a **subpath-anchored** fixture (`AnchorRel != "."`, so hub, worktree root and anchor path are three distinct directories, plus a `RepoName` distinct from every basename) asserting each of the seven `Geometry` fields lands on the right source.
This is the only guard against the anchor/worktree swap that is this refactor's one silent failure mode; a fixture where anchor == worktree would pass while swapped and is therefore worthless here.
Assert `SocketKey == reedengine.ServerName(hub)` and `SessionName == reedengine.SessionName(worktreeRoot)` against the real functions rather than hardcoded strings, so the test tracks those derivations rather than freezing them.

**`internal/reedengine`.** Convert `newTestEngine` (`lock_test.go:22-35`) to build a `Geometry` over its `t.TempDir()`; most of the package's tests then need no edit.
Give the fixture *distinct* values per field (not the same temp dir for anchor, worktree and hub) so a field mix-up inside the engine surfaces.
Preserve every existing assertion — `server_test.go`'s `ServerName`/`socketName` determinism and per-hub uniqueness tests are unchanged, since `server.go` is unchanged.
`header_test.go` must still assert that `repo` and `hub` render real values, now sourced from `Geometry`.
`contract_integration_test.go` and `mouse_boot_integration_test.go` are build-tagged integration tests over real tmux — they must compile and pass under `go test -tags integration ./internal/reedengine/...`.

**`internal/shuttleengine`.** `rundir_test.go` covers `runDirRoot`'s three branches (empty `RunDir`, absolute `RunDir`, relative `RunDir`); all three must keep asserting that the base is the anchor root, now passed directly.
`run_test.go`/`wait_test.go`/`run_inject_test.go` build `Runner`s via `NewRunner` or the fakes in `fakes_test.go` — the two new string parameters must be distinct values in the fixtures, for the same reason as above.
`seam_enforcement_test.go` (`TestProviderSeamImportRule`) is unaffected.

**`internal/tokenvocab`.** `tokenvocab_test.go` builds a `Ctx` and asserts the resolved map; converting it to the two plain fields is mechanical.
`leaf_enforcement_test.go` needs its `allowedImports` tightened to stdlib + `stencil` — that tightening is itself the assertion that `lyxcwd` is gone, so it is a TDD candidate: tighten the allowlist first, watch it fail, then remove the import.

**`internal/fabricengine`.** `hubscratch_test.go:57-70` keeps asserting the same idempotency, now against `fabricengine.HubLogsDir` in-package.
Add a direct assertion that `HubLogsDir(hub) == filepath.Join(HubScratchDir(hub), "logs")`.

**`cmd/lyx/constructoranchoring_test.go`.** Rows 96 and 144 must keep asserting the identical expected path (`filepath.Join(hub, "_board", ".lyx", "logs")`) — the point of the rows is that the value does not move, only the package that computes it.

**Verify command** (widened from T3's, which omits the packages the smoke tests live in):

```
go test ./internal/shuttleengine/... ./internal/reedengine/... ./internal/tokenvocab/... ./internal/hubgeom/... ./internal/websterengine/... ./internal/burlercli/... ./internal/perchcli/... ./internal/webstercli/... ./internal/shuttlecli/... ./internal/reedcli/... ./internal/treadleengine/... ./internal/burlerengine/... ./internal/fabricengine/... ./cmd/lyx/...
```

plus `go test -tags integration ./internal/reedengine/...` for the tmux paths.

**Structural checks** (part of done, not optional):

- `internal/lyxcwd` absent from `internal/tokenvocab`, `internal/reedengine` and `internal/shuttleengine` production imports.
- `internal/fabricengine` absent from `internal/reedengine` production imports.
- `internal/tokenvocab/leaf_enforcement_test.go`'s allowlist tightened, not left permissive.

## Q&A log

- **Q:** `Geometry`'s field set — the design doc's table omits the worktree root, but `strand.go` needs it twice. **A:** Add `WorktreeRoot` alongside told `SessionName`, seven fields. Told `SessionName` survives because T5 needs standalone to name the session from `hash8`.
- **Q:** Where does `reedengine.HubLogsDir` go, given `fabricengine` must leave reed's imports? **A:** Move it to `fabricengine.HubLogsDir(hubPath string)` — `fabricengine` already owns `HubScratchDir`, and the `constructoranchoring_test.go` rows retarget rather than die.
- **Q:** How is the seven-field hub-mode construction handled across five CLI sites and four smoke tests? **A:** Never duplicated — a single shared teller, reused everywhere. This is a standing operator rule, not a per-task preference.
- **Q:** Name and reach of that shared package? **A:** `internal/hubgeom`, generically named so T6 and T7 add `BurlerGeometry`/`PerchGeometry`/`WebsterGeometry` to the same package rather than forcing a rename or spawning sibling packages.
- **Q:** Does shuttle's `anchorPath, worktreeRoot` pair come from the same `Geometry` value? **A:** Yes — one `hubgeom.ReedGeometry(layout)` call feeds both `reedengine.New` and `shuttleengine.NewRunner` at each site, so the anchor/worktree pairing is decided once.
- **Q:** Does `reedengine.New` validate the told `SocketKey`? **A:** No. It stays a total function returning `*Engine` only; the doc comment names `reedengine.ServerName(hubPath)` as the caller's hub-mode obligation. Validation would ripple an error return through nine construction sites for a bug only a caller bypassing `hubgeom` could produce.
- **Q:** Do `reedengine` and `shuttleengine` get their own allowlist-enforcement tests now? **A:** No — T10 owns naming the cross-cutting told-geometry invariant. Only the two existing invariants are reworded, and only `tokenvocab`'s allowlist is tightened (the T3 Verify line requires that one explicitly).
- **Q:** Is `manifest/designs/producers-standalone.md` amended, given its T3 Files/Verify lines are incomplete? **A:** Not rewritten — the same-commit docs rule targets module docs, `docs/overview.md` and `CONSTRAINTS.md`. The gap is recorded in this discussion file, which is the corrected record for T3.
- **Q:** (orchestrator review) Nothing in the design doc routes a future T6/T7 explorer to `hubgeom`, so they could re-derive the seven-field construction inline. **A:** Add a one-line pointer to `hubgeom` in the design doc's T6 and T7 entries, plus the same statement in `internal/hubgeom/doc.go`. Narrow exception to "don't edit the design doc"; it fixes a concrete duplication risk rather than a bookkeeping gap.
- **Q:** (orchestrator review) `hubgeom` is wave-1 architecture that binds T6 and T7 before either has been discussed. **A:** Accepted deliberately, with the limit recorded in the decision: `hubgeom` has one exported function and nine call sites, so T6's or T7's own discussion can rename or reshape it freely. T3 claims only that *its* nine sites share one teller.
- **Q:** Which docs land in this commit? **A:** `CONSTRAINTS.md` (two invariants), `internal/tokenvocab/doc.go`, `internal/reedengine/doc.go`, `internal/shuttleengine/doc.go`, a new `internal/hubgeom/doc.go`, `docs/overview.md` (the package tree at ~line 231-239 and the shared-infrastructure list at line 314), `docs/shared-libs/README.md`, and the two T6/T7 pointer lines in `manifest/designs/producers-standalone.md`. No `manifest/roadmap.md` edit.
