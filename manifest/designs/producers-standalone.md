# Producers standalone — running burler/perch/webster outside a lyx worktree

## What it is

`lyx burler run --profile p.yaml` should review an arbitrary directory — a "Models" folder, a downloaded repo, anything — with a custom prompt, from a machine with no lyx hub, no Fabric, no `_lyx/config/` seeding, not even a git repository.
The same for `lyx perch run`, and for Webster.
Today every one of those commands dies in its `PersistentPreRunE` before reaching any producer logic.

This is not a feature request against the producers themselves.
It is a statement about *who is allowed to require geometry*.
The rule this doc lands is one sentence:

> **An orchestrator resolves geometry and requires it; a producer is told its paths and requires nothing.**

That rule is what lets `loom` plug Webster, Perch and Burler into its flat producer list as interchangeable `ShedProducer`s without any of them importing or reaching into whichever orchestrator — `loom`, `Hardener`, or a human at a shell — happens to be driving.
See [shed.md](shed.md) for the producer list this serves.

## The three resolution tiers

The single most common misconception about this area — the one the originating discovery task encoded, and the one that has to be corrected before anything else makes sense — is that `lyxcwd.Resolve()` proves a worktree is lyx-initialized and Fabric-wired.
It does not.

Reading `internal/lyxcwd/lyxcwd.go`'s `resolveCore`/`buildLocation` and `internal/lyxcwd/anchor.go`'s `readRecordedAnchor`, `Resolve(cwd)` checks exactly this:

1. `git rev-parse --show-toplevel` must succeed at `cwd`, else `ErrNotAGitRepo`.
   This is its only real validation.
2. It reads `<dir-of-worktree>/_board/.lyx-anchor` for `AnchorRel`.
   **An absent marker is not an error** — `AnchorRel` falls back to `"."`.
   Only a *stale* `.fabric-anchor` with no renamed marker beside it hard-errors.
3. `cwd` must equal `Join(worktreeRoot, AnchorRel)`.
   With no marker that reduces to "cwd is the git worktree root".
4. `HubPath` is assigned `filepath.Dir(worktreeRoot)` **unconditionally**, never verified to be a hub;
   `RepoName` is `Base(hubPath)` with `-HUB` trimmed, with no check that the suffix was ever there.

`lyxcwd.go`'s own doc comment on `Resolve` says it outright: *"Resolve does NOT check for `_lyx/`"*.
It checks nothing whatsoever about Fabric junctions.
**`lyxcwd.Resolve` succeeds in any ordinary git repository run from its root**, and hands back a `Location` whose `HubPath` and `RepoName` are fiction in that case.

The gate that *does* prove full initialization already exists — it is `loomengine.Preflight(cwd)` (`internal/loomengine/preflight.go`), which layers `lyxcwd.Resolve` → `fabricengine.PrimeName` → `fabricengine.Clean` → `fabricengine.Ready` → `fabricengine.Healthy` → `loom`'s own `status.json` coherence.
So the correct model is three tiers, not two:

| Tier | Function(s) | What it proves | Who must require it |
|---|---|---|---|
| 1 — geometry | `lyxcwd.Resolve` | cwd is the root of a git worktree; `AnchorRel` is whatever the marker says, or `"."` | any caller that wants a `*lyxcwd.Location` at all |
| 2 — fabric | `fabricengine.Ready` / `Healthy` / `Clean` / `PrimeName` | Fabric is wired here, junctions intact, warp and weft in sync, tree clean | orchestrators |
| 3 — orchestrator state | `loomengine.Preflight` | tiers 1 and 2, plus this orchestrator's own status seed is present and coherent | `loom` today; `Hardener` later |

**Producers require none of the three.**
An orchestrator requires tier 3 and threads the extracted plain values down through its whole producer list.
A standalone CLI invocation of a single producer never enters tier 1 in the first place.

Tiers 1 and 2 are currently welded inside `internal/loomengine`, so `Hardener` and any future `Shed` product would have to re-implement them.
Lifting them into a shared, orchestrator-agnostic preflight is what makes this model real rather than aspirational, and it is task **T5** below.

## What actually blocks standalone today

Every row below was verified against the tree at discovery time, not inherited from the discovery task.
Rows whose task has since LANDED are marked **done** inline and record the shipped state rather than the blocker;
an unmarked row is a discovery-time reading that no later task has re-verified, not a fresh one.

### Layer 1 — engine constructors take `*lyxcwd.Location`

| Package | Sites | What it reads from the `Location` |
|---|---|---|
| `internal/shuttleengine` | `run.go` (`NewRunner`, `Runner.layout`), `rundir.go` (`runDirRoot`, `FindRun`) | `AnchorPath()`, `WorktreePath()` |
| `internal/reedengine` — **done (T3)** | takes a `reedengine.Geometry` (`geometry.go`), holds no `Location`, and has no `Engine.layout` field | nothing: `hubgeom.ReedGeometry` reads `HubPath`/`WorktreePath()`/`AnchorPath()`/`RepoName` off the `Location` at the `reedcli` seam and hands the engine seven told strings. `fabricengine.HubLogsDir(hubPath string)` owns the hub-logs derivation |
| `internal/tokenvocab` — **done (T3)** | `tokenvocab.go` (`Ctx.RepoName`, `Ctx.HubPath`) | nothing: `Ctx` carries the two plain strings, `lyxcwd` left the package's import allowlist entirely (see [CONSTRAINTS.md's Tokenvocab Leaf Invariant](../../CONSTRAINTS.md#tokenvocab-leaf-invariant)), and `reedengine/header.go` fills `Ctx` from its own told `Geometry` |
| `internal/pattern` | `pattern.go` (`Directive`, `FileHere`, `isActive`) | `WorktreePath()` + `AnchorRel`, i.e. exactly `AnchorPath()` |
| `internal/burlerengine` | `engine.go` (`New`, `Engine.layout`, `Run`) | `WorktreePath()` (profile validation), `AnchorPath()` (`.lyx/burler`) |
| `internal/perchengine` | `engine.go` (`New`, `Engine.layout`), `identity.go` (`RunsDir`, `ScratchDir`) | `WorktreePath()` (gate dir), `AnchorPath()` (run/scratch dirs) |
| `internal/websterengine` | `state.go` ×4, `render.go` ×3, `runlevel.go` ×2, `beginbatch.go`/`recordbatch.go`/`recoverbatch.go` ×1 each | assorted; `render.go` also derives `fabricengine.StencilsDir(l.HubPath)` |

Not one of these re-resolves, spawns git, or validates anything.
Every one reads plain derived strings.
That is why layer 1 is a *type* coupling, not a capability one — and why it is fixable purely mechanically.

### Layer 2 — the CLI pre-run resolves unconditionally

`internal/burlercli/cli.go`'s `PersistentPreRunE` and `internal/perchcli/cli.go`'s equivalent both do this, with no bypass:

```go
layout, err := lyxcwd.Resolve(cwd)                                    // hard-fails outside a git repo
shuttleCfg, err := shuttleengine.LoadConfig(layout.AnchorPath(), "shuttle")
reedCfg, err := reedengine.LoadConfig(layout.AnchorPath(), "reed")
burlerCfg, err := burlerengine.LoadConfig(layout.AnchorPath())
reedEngine := reedengine.New(reedCfg, layout)
runner := shuttleengine.NewRunner(reedEngine, claudeengine.New(), layout, shuttleCfg)
c.engine = burlerengine.New(runner, layout, burlerCfg, fabricengine.StencilsDir(layout.HubPath))
```

`perchcli` additionally loads `modelspec.LoadRegistry` and `perchengine.LoadConfigWithRegistry`.
Fixing every constructor in layer 1 does not help while this code stands: nothing branches around `Resolve` itself.

### Layer 3 — config loads that hard-fail on an absent file

Which loader sits on the strict side and which degrades is pinned by [CONSTRAINTS.md's Config Strictness Invariant](../../CONSTRAINTS.md#config-strictness-invariant), which is the authority here;
this table records the discovery-time reading plus whichever rows a landed task has since moved.

| Loader | Behaviour with no config present | Blocks standalone |
|---|---|---|
| `modelspec.LoadRegistry` | returns `builtins()` — `sonnet`/`opus`/`haiku`/`fable` mapped to the `claude` engine | no |
| `burlerengine.LoadConfig` | own `os.ReadFile`, absent file decodes to the zero `Config{}` | no |
| `shuttleengine.LoadConfig` | `configengine.Load` → `"not initialized: _lyx/ directory not found"` | **yes** |
| `reedengine.LoadConfig` — **done** | `configengine.LoadOrTemplate` → resolves the embedded template on a proven-absent `_lyx/` or config file | no |
| `perchengine.LoadConfigWithRegistry` | same | **yes** |

All five already take `baseDir string` — the loaders themselves are not coupled to `Location`.
What blocks is `configengine.Load`'s two refusal branches (`FindBaseDir`'s `_lyx/` stat at `config.go:52`, and the file-not-found at `config.go:60`).
Each module already embeds a complete `template.yaml` and already passes it into `Load` for the missing-key check, so the material for a graceful fallback is already in hand at every call site.
`envsource.Build(baseDir)` returns an empty map when `.env` is absent and has no `_lyx` requirement of its own, so nothing downstream of the refusal branches objects.

### Layer 4 — stencils

`fabricengine.StencilsDir(hub)` resolves `<hub>/_board/_lyx/stencils`, which standalone has no hub to supply.
The read path itself is trivially portable: `stencilstore.Read(baseDir, name)` is a bare `os.ReadFile` with no parsing, schema, or validation.
The heavy machinery — seeding, hash-stamping, edit detection, and the `board.lock`-holding commit — all lives in `Reconcile` and in `cmd/lyx/stencilseed.go`, and standalone skips every bit of it.
The producers that read stencils need a *directory* of named files, not one file: burler reads four (`burler-template-round-orchestrator`, `burler-step-1-explore`, `burler-step-2-review`, `burler-step-3-fix`), `pattern.Directive` a fifth, `treadleengine` four more.

## Decisions

### told-geometry — engines take plain strings, never a `*lyxcwd.Location`

Each engine constructor takes the directories it actually reads, as plain absolute strings.
It has no `HubPath`, no `RepoName`, and no concept of a hub.
Standalone passes the target directory for both roots and is done.

The rejected alternative is a **synthetic `Location`**: leave every signature alone and have the caller fake the object when there is no real one.
That alternative is not hypothetical — `internal/scoutcli/cli.go`'s `resolveLocation` ships it today, and it is why `lyx scout` already works in any folder (see "Corrections" below).
It is dramatically cheaper.
It is still the wrong shape here, for one concrete reason: in the synthetic object `HubPath` is a lie, and `reedengine/lock.go`'s `New` derives the **tmux socket name** from `HubPath`.
A faked `HubPath` silently names the socket after whatever directory happens to be the parent of the target, so two standalone runs in sibling folders collide or share a socket in ways nobody designed — and no compiler or test catches it.
`burlerengine.Run`'s `p.validate(e.layout.WorktreePath(), …)` has the same shape of exposure.
Scout survives its own fiction only because it reads `AnchorPath()` and nothing else, and its doc comment explicitly forbids widening;
that bound does not generalise.

Told-geometry is also what the three newest packages already do, and what their invariants already require: `internal/shedengine` ([Shed Producer-Seam Invariant](../../CONSTRAINTS.md#shed-producer-seam-invariant)), `internal/treadleengine` ([Treadle Runner-Seam Invariant](../../CONSTRAINTS.md#treadle-runner-seam-invariant)), and `internal/shedadapters` (whose package doc already states "every constructor receives already-resolved absolute paths … no adapter calls lyxcwd, os.Getwd, or git").
This work makes the producer layer beneath those adapters match them.

### config — a strict loader and a degrading loader, not one loader with a flag

Add `configengine.LoadOrTemplate(baseDir, module string, template []byte)`: identical to `Load` except that an absent `_lyx/` directory or an absent config file resolves the caller's own embedded `template` through the same `envsource.Build`/`yamlengine.Resolve` pipeline instead of erroring.
Repoint the three producer loaders at it.
Leave `Load` strict for the five hub-scoped callers (`fabricengine`, `boardengine`, `loomengine`, `batcher`, `websterengine`), where an absent config genuinely means a broken hub and must stay loud.

The two functions share a signature, which is normally a smell.
It is warranted here because the contracts genuinely differ, and because `internal/lyxcwd` already carries the same deliberate pair for the same reason — `Resolve` (gated) beside `ResolveWorktree` (ungated), documented as different contracts rather than as a migration artifact.
What this is *not* is a migration twin: neither function is scheduled for deletion, and neither is a wrapper over the other's old shape.

### stencils — a told directory, bootstrapped by `Reconcile`

Standalone takes `--stencils-dir <path>`.
Nothing about it is hub-shaped: `stencilstore.Read` is `os.ReadFile`, and the directory is simply told.
For bootstrap, the standalone path calls `stencilstore.Reconcile(dir, stencils.Registry(), mode, "")` on first use, which writes the shipped registry into the told directory;
only the *commit* half of `cmd/lyx/stencilseed.go` is Fabric-bound, and standalone skips it.
`mode` reuses the existing `buildChannel == "dev"` selector from `cmd/lyx/stencilseed.go:73-76` verbatim rather than hardcoding `ModeProduction`, so a dev binary keeps its dev seeding semantics standalone exactly as it does in a hub.
The fourth argument stays `""` — that is the "no source tree here" value which keeps the port-back drift warning silent, and standalone genuinely has no `contracts/stencils` source tree beside it.

The [Stencil Ownership Invariant](../../CONSTRAINTS.md#stencil-ownership-invariant) currently pins the read location to `<hub>/_board/_lyx/stencils/` specifically.
Its actual load-bearing content is "read from a file at call time, never from embedded bytes", which a told directory satisfies exactly.
The invariant is reworded to name a told absolute directory, with `<hub>/_board/_lyx/stencils/` as what the hub-resident path resolves to — task **T10**.

### no additive twins — parallelism comes from wave scheduling

The tempting way to parallelise a signature migration is to add the plain-string function beside the old one and leave the old one as a wrapper, so every package ships independently.
That is rejected.
Two near-identical signatures sitting side by side is a real cost paid immediately against a cleanup that is only promised, and the duplication reliably outlives the migration.

Parallelism is bought by scheduling instead.
The genuine constraint is not the import graph — it is *file* contention, and it concentrates in three files: `internal/burlercli/cli.go` and `internal/perchcli/cli.go`, which every constructor change converges on, plus `cmd/lyx/constructoranchoring_test.go`, which pins the anchoring of nearly every constructor this work touches and is therefore edited by T1, T3, T4, T6, T7 and T9 alike.
Grouping tasks into waves whose file sets are disjoint gets 3-wide, then 2-wide, then 2-wide, then 2-wide, with no duplicated API at any point.

**Disposition of `cmd/lyx/constructoranchoring_test.go`: edited in place, per task, never split or retired.**
It is a flat table of one-line `assertPath` assertions and a named enforcer of the [Durable-vs-Ephemeral State Invariant](../../CONSTRAINTS.md#durable-vs-ephemeral-state-invariant);
fragmenting its rows into per-package tests would trade a single readable overview of every anchored path for nothing this work needs.
Each task therefore rewrites only its own package's rows into whatever shape its new constructor takes, and every task listed above carries `./cmd/lyx/...` in its Verify line.
One same-wave adjacency is real and is called out rather than papered over: in wave 3, T6's `perchengine.RunsDir`/`ScratchDir` rows (79, 89, 128, 138) sit directly beneath T7's `websterengine.Dir`/`ReportsDir`/`PromptsDir`/`ScratchDir` rows (77-78, 87-88, 126-127, 136-137), so whichever merges second rebases.
That is a one-line mechanical resolution, not a design conflict, and it does not make T6 and T7 serial.
Wave 1's pair is farther apart — T1 edits rows 71-72/120-121 and T3 edits rows 96/144 — so no adjacency arises there.

Twins remain available as a named escape hatch if a specific pair turns out to be unschedulable in practice.
They are not the default and should not be reached for without saying why in the task that does it.

### scout is not migrated

Scout already works standalone (see below).
Migrating it onto told-geometry delivers no new capability and only buys uniformity.
It is task **T9**, explicitly optional, and dropping it costs nothing but a documented deviation.

## Corrections to the originating discovery task

The discovery task filed under this slug was explicit that it was assembled from incremental spot-checks rather than a systematic trace, and that it was probably incomplete.
It was right to say so.
Five of its findings are stale or wrong against the current tree, and two of them would have misdirected the work badly.

- **`lyxcwd.Resolve` does far less than claimed.**
  The doc says it "validates it is inside a git repo, carries a `.lyx-anchor` marker, has fabric junctions wired, and matches a resolved `AnchorPath()` exactly".
  The anchor marker is optional with a `"."` fallback, and junction wiring is never checked at all.
  See "The three resolution tiers" above.
- **Scout is not a problem — it is the existing precedent.**
  The doc suspected scout of "the same category of problem".
  The reverse holds: `internal/scoutcli/cli.go`'s `resolveLocation` and `lookupContext` already implement graceful degradation at both the geometry layer (fall back to a synthesized `Location`) and the config layer (fall back to `scoutengine.BuiltinRegistry()`), and the engine already takes a told `Options.TargetDir`.
  `lyx scout` runs against an arbitrary folder with zero lyx setup **today**.
  Its synthesized `Location` is a deliberate, documented fiction bounded by its own doc comment to `DaemonStateFile`/`DaemonLock`, which read `AnchorPath()` only.
- **`pattern.Directive` is already standalone-tolerant.**
  `pattern.go`'s `Directive` returns `("", nil)` with no read attempted for a `nil` `Location`.
  It was never a blocker, only an awkward signature.
- **`reedengine`'s coupling has shrunk.**
  The doc records `lock.go` ×5, `lifecycle.go` ×3, `strand.go` ×2, `header.go` ×1.
  Current: `lock.go` ×3 and `lifecycle.go` ×1;
  `strand.go` and `header.go` no longer reference `lyxcwd` at all (`header.go` reaches it only transitively, through `tokenvocab.Ctx.Layout`).
- **The claimed disjointness is wrong, and its conclusion inverted.**
  The doc verifies that `shuttleengine`, `reedengine` and `pattern` share no files and no import relationship, and concludes they "can proceed in parallel".
  File-disjointness is not the relevant test for a signature change.
  `pattern` is imported by `burlerengine` and `websterengine`;
  `shuttleengine` by both of those plus `treadleengine` and four CLIs;
  `reedengine` by `shuttleengine` plus five CLIs.
  Changing any of the three forces same-commit edits in packages the other two also touch, so those three are the *most* serialized tasks in the set, not the most parallel.
  The wave schedule below is built from actual file contention.

One finding survives intact and is preserved as **T1**: `internal/webstercli/cli.go:194` resolves its plan directory as `loomengine.PlanDir(layout)`, which is `internal/webstercli`'s only reason to import `internal/loomengine` at all.

## Task decomposition

Ten tasks in five waves.
Every entry is written extraction-ready — lifting one into the wiki is copy-paste, and should happen only when that task is about to be spawned, since these reliably shift during implementation.

Task IDs run in wave order, so every `Depends on` edge points backwards and the numbers can be read as a sequence:

| Wave | Tasks | Parallel |
|---|---|---|
| 1 | T1, T2, T3 | 3-wide |
| 2 | T4, T5 | 2-wide |
| 3 | T6, T7 | 2-wide |
| 4 | T8, T9 (optional) | 2-wide |
| 5 | T10 | — |

**Enumeration method for every `Files` list below: every caller of every changed exported symbol, not merely every construction site.**
Stating it matters because the constructor-only reading is wrong at least once — `shuttleengine.FindRun` is exported and is called from `internal/websterengine` as well as from within `shuttleengine` itself, so a task that changes it reaches a package none of its constructors do.
A task picking this decomposition up should re-run the enumeration for its own symbols before trusting the list, since the tree moves.

Every task's verification baseline is `go test ./...` from the worktree root;
task-specific checks are named per entry.

### Wave 1 — foundations (3 parallel)

---

**T1 — `planparser` owns the plan-directory path**
`slug: planparser-plan-dir`

**Brief.** `internal/webstercli/cli.go:194` resolves its plan directory as `loomengine.PlanDir(layout)`, which is the sole reason `webstercli` imports `loomengine`.
The value does not belong to `loom`: `internal/planparser` already declares itself the plan format's sole owner and already exports the worktree-relative token `PlanDirRel()`, built from its own `PlanDirName` constant.
`loomengine.PlanDir(l)` is that same token re-anchored onto `l.AnchorPath()`, and `loomengine.PlanOverview(l)` repeats the pattern while hardcoding `"00-overview.md"` as a duplicate of `planparser`'s own unexported `overviewFileName`.
Add `planparser.PlanDir(anchorPath string) string` and `planparser.PlanOverview(anchorPath string) string`, delete both `loomengine` twins, and repoint every caller.
Note that `planparser` currently imports only `internal/lyxdirs` — taking a plain string keeps it that way, where taking a `*lyxcwd.Location` would add an import to a package that has none.

**Files.** `internal/planparser/parse.go`; `internal/loomengine/config.go` (lines ~32-42), `internal/loomengine/plan.go` (lines 67-68), `internal/loomengine/planpath_test.go`, `internal/loomengine/plan_test.go`; `internal/webstercli/cli.go` (line 194, and the `loomengine` import), `internal/webstercli/cli_test.go`, `internal/webstercli/verbs_test.go`; `cmd/lyx/constructoranchoring_test.go` (lines 71-72, 120-121), `cmd/lyx/notransients_test.go` (lines 50-51); `CONSTRAINTS.md`.

**Also fix.** The [Planparser Sole-Parser Invariant](../../CONSTRAINTS.md#planparser-sole-parser-invariant) states "Resolves `_lyx/plan/` via `lyxcwd`, never string literals".
That is already false — `planparser` does not import `lyxcwd` — and after this change the package is *told* an absolute anchor path.
Reword in the same commit.

**Depends on.** Nothing.
**Parallel-safe with.** T2, T3.
Shares `internal/webstercli/cli.go` with T3, at line 194 against T3's lines 179-181 — separate hunks, but sequence them if a merge conflict appears.
**Verify.** `go test ./internal/planparser/... ./internal/loomengine/... ./internal/webstercli/... ./cmd/lyx/...`, plus confirm `internal/loomengine` no longer appears in `internal/webstercli`'s production import set.

---

**T2 — config degrades to its embedded template**
`slug: config-template-fallback`

**Brief.** `shuttleengine.LoadConfig`, `reedengine.LoadConfig`, `perchengine.LoadConfigWithRegistry` and `websterengine.LoadConfig` all route `configengine.Load`, which refuses on an absent `_lyx/` (`config.go:52`) and on an absent config file (`config.go:60`), rewrapped by each caller as `"not initialized here; run \"lyx fabric reconcile\""`.
Add `configengine.LoadOrTemplate(baseDir, module string, template []byte) ([]byte, error)`: identical to `Load` except that both refusal branches instead resolve the caller-supplied `template` through `envsource.Build`/`yamlengine.Resolve`.
Repoint exactly those four loaders.
`websterengine.LoadConfig` belongs here, not in the strict group: its shape is identical to `shuttleengine`/`reedengine`/`perchengine`'s (`configengine.Load(baseDir, module, ConfigTemplate())`, config_test.go's own `TestLoadConfig_NotInitialized` pins the refusal today), and `webster.yaml` is an operator-tunable producer config — role/model-spec settings — not hub state, the same distinction that keeps `burlerengine.LoadConfig` off the strict list already.
Webster's inclusion among the "hub-scoped" group in an earlier draft of this task conflated its *own* config with the genuinely hub-scoped callers below; it does not belong there once Webster has a standalone entry (T7).
`configengine.Load` stays strict and unchanged for its four remaining hub-scoped callers (`fabricengine`, `boardengine`, `loomengine`, `batcher`), where an absent config means a broken hub.
This brings all five producer-path loaders to a single behaviour, matching `modelspec.LoadRegistry`'s `builtins()` fallback, which already degrades.

**Files.** `internal/configengine/config.go`, `internal/configengine/config_test.go`; `internal/shuttleengine/config.go` (line 38), `internal/reedengine/config.go` (line 42), `internal/perchengine/config.go` (line 58), `internal/websterengine/config.go` (line 53).

**Depends on.** Nothing.
**Parallel-safe with.** T1, T3.
Touches `shuttleengine/config.go`, `reedengine/config.go`, `perchengine/config.go` and `websterengine/config.go` — all distinct from the files T3, T6 and T7 touch in those same packages.
**Verify.** `go test ./internal/configengine/... ./internal/shuttleengine/... ./internal/reedengine/... ./internal/perchengine/... ./internal/websterengine/...`, plus a new test per loader asserting a `baseDir` with no `_lyx/` returns the template-derived config rather than an error.

---

**T3 — `shuttleengine` + `reedengine` + `tokenvocab` told-geometry**
`slug: shuttle-reed-told-geometry`

**Brief.** Convert the three lowest-level producer packages off `*lyxcwd.Location` in one task, because their construction sites are the same five CLI files and splitting them would only manufacture conflicts.
Note that construction sites are not the whole blast radius: `shuttleengine.FindRun` is exported and is also called from `internal/websterengine` at `recoverbatch.go:182` and `runlevel.go:529`, so this task reaches into `websterengine` even though T7 is what converts that package properly.
Those two are one-token edits (`deps.Layout` becomes `deps.Layout.AnchorPath()`), and they do not require or anticipate T7.
`shuttleengine.NewRunner(reed, engine, layout, cfg)` becomes `NewRunner(reed, engine, anchorRoot, worktreeRoot string, cfg)`;
`runDirRoot` and `FindRun` take `anchorRoot` instead of `layout`.
`reedengine.New(cfg, layout)` becomes `New(cfg, Geometry)`, where `Geometry` is reed's own told-geometry struct — a positional parameter list would reach five strings here, which is exactly the smell this decision's name ("told-geometry structs per engine") exists to avoid.
`Geometry` carries every value reed today derives from a `Location`, and enumerating them is what makes each one's standalone answer a decision rather than an omission:

| Field | Derived today from | Consumer |
|---|---|---|
| `SocketKey` | `socketName(l.HubPath)` | the tmux socket name |
| `SessionName` | `SessionName(l.WorktreePath())` | the tmux session name |
| `AnchorRoot` | `l.AnchorPath()` | `stateDir()` (`reed.json`/`reed.lock`), pane spawn cwd |
| `LogsDir` | `fabricengine.HubScratchDir(l.HubPath)/logs` | the shared server's runtime log |
| `RepoName`, `HubPath` | `l.RepoName`, `l.HubPath` | the header pane's `repo`/`hub` tokens, via `tokenvocab` |

`SocketKey` is the single most important field, since it is where a faked `HubPath` would otherwise silently mis-name the tmux socket.
`LogsDir` being told rather than derived matters structurally beyond this task: `fabricengine.HubScratchDir` is `reedengine`'s **only** `internal/fabricengine` reference (`lifecycle.go:36`), so telling reed its logs directory removes `internal/fabricengine` from `reedengine`'s import set outright — and with it the `treadleengine` → `shuttleengine` → `reedengine` → `fabricengine` transitive path the [Treadle Runner-Seam Invariant](../../CONSTRAINTS.md#treadle-runner-seam-invariant) currently has to acknowledge as real.
Update that invariant's text in the same commit.
`RepoName`/`HubPath` are carried because `Engine.HeaderText` renders the header pane at every boot from `tokenvocab.Ctx` (`header.go:16`), whose only two tokens are exactly those fields (`tokenvocab.go:25-26`);
dropping them would leave the header rendering empty tokens with nothing in the design saying so.

`tokenvocab.Ctx.Layout` becomes the same two plain fields (`RepoName`, `HubPath`), which drops `internal/lyxcwd` from `tokenvocab`'s import set entirely.

**Files.** `internal/shuttleengine/run.go`, `internal/shuttleengine/rundir.go`; `internal/reedengine/lock.go`, `internal/reedengine/lifecycle.go`, `internal/reedengine/header.go` (line 16); `internal/tokenvocab/tokenvocab.go`, `internal/tokenvocab/doc.go`; construction sites `internal/burlercli/cli.go` (103-104), `internal/perchcli/cli.go` (143-144), `internal/webstercli/cli.go` (179-181), `internal/shuttlecli/cli.go` (92-93), `internal/reedcli/cli.go` (83); non-constructor callers of the changed exported symbols, `internal/websterengine/recoverbatch.go` (182) and `internal/websterengine/runlevel.go` (529), both calling `shuttleengine.FindRun`; the tests in each package; `cmd/lyx/constructoranchoring_test.go` (its `reedengine.HubLogsDir` rows, 96 and 144); `CONSTRAINTS.md` ([Tokenvocab Leaf Invariant](../../CONSTRAINTS.md#tokenvocab-leaf-invariant) loses `internal/lyxcwd` from its allowlist, and the [Treadle Runner-Seam Invariant](../../CONSTRAINTS.md#treadle-runner-seam-invariant) loses its `reedengine` → `fabricengine` transitive-path note).

**Watch.** `reedengine` spawns real OS processes, so the [Live-Substrate Spawn Observability](../../CONSTRAINTS.md#live-substrate-spawn-observability) invariant binds every touched spawn path — the `logger.Info`/`Warn` calls in `lifecycle.go` must survive the refactor intact.

**Depends on.** Nothing.
**Parallel-safe with.** T1, T2.
Its two `websterengine` edits are in `recoverbatch.go`/`runlevel.go`, which no wave-1 task touches, and which are distinct files from T4's `render.go` in wave 2 — but T7 must land after this task, since it rewrites the same two files wholesale.
**Verify.** `go test ./internal/shuttleengine/... ./internal/reedengine/... ./internal/tokenvocab/... ./internal/websterengine/... ./internal/burlercli/... ./internal/perchcli/... ./internal/webstercli/... ./internal/shuttlecli/... ./internal/reedcli/... ./cmd/lyx/...`, plus `go test -tags integration ./internal/reedengine/...` for the tmux paths, plus confirm `internal/lyxcwd` is gone from `internal/tokenvocab`'s production imports and `internal/fabricengine` from `internal/reedengine`'s.

---

### Wave 2 — mid-layer (2 parallel)

---

**T4 — `pattern` told-geometry**
`slug: pattern-told-geometry`

**Brief.** `pattern.Directive(l *lyxcwd.Location, stencilsDir string, role Role)` uses `l` for exactly one thing: `isActive(l)` → `FileHere(l)` → `File(Join(l.WorktreePath(), l.AnchorRel))`, which is `File(l.AnchorPath())`.
Change it to `Directive(anchorPath, stencilsDir string, role Role)`, with the empty string meaning inactive — replacing today's `l == nil` branch and preserving the existing `("", nil)` behaviour exactly.
Delete `FileHere`;
the already-exported `File(baseDir string)` is what it wraps.
`isActive` takes the same string.
This drops `internal/lyxcwd` from `pattern`'s imports, shrinking the [Pattern Leaf Invariant](../../CONSTRAINTS.md#pattern-leaf-invariant) allowlist to stdlib plus `lyxdirs`, `stencilstore` and `stencil`.

**Files.** `internal/pattern/pattern.go`, `internal/pattern/doc.go`, `internal/pattern/pattern_test.go`, `internal/pattern/leaf_enforcement_test.go`; call sites `internal/burlerengine/engine.go` (103), `internal/websterengine/render.go` (174, 237), `internal/loomengine/plan.go` (70); `cmd/lyx/constructoranchoring_test.go` (its `pattern.FileHere` rows, 80 and 129, which the deletion of `FileHere` retires or rewrites onto `pattern.File`); `CONSTRAINTS.md`.

**Depends on.** Nothing.
**Parallel-safe with.** T5.
Scheduled into wave 2 rather than wave 1 for file contention alone, not for any dependency: it edits `burlerengine/engine.go`, `websterengine/render.go` and `loomengine/plan.go`, which T6, T7 and T1 respectively also touch.
**Verify.** `go test ./internal/pattern/... ./internal/burlerengine/... ./internal/websterengine/... ./internal/loomengine/... ./cmd/lyx/...`, plus confirm `internal/lyxcwd` is gone from `internal/pattern`'s production imports and that the leaf-enforcement allowlist was tightened rather than left permissive.

---

**T5 — lift the orchestrator preflight out of `loomengine`, plus the shared standalone-CLI foundations**
`slug: orchestrator-preflight`

**Brief.** `loomengine.Preflight` is the only implementation of tiers 1 and 2 — geometry resolution plus `fabricengine.PrimeName`/`Clean`/`Ready`/`Healthy` — and its check 4 is `loom`-specific (`_lyx/loom/status.json`).
`Hardener`, and any future `Shed` product, would have to re-implement checks 1-3 verbatim.
Extract checks 1-3 into an orchestrator-agnostic package that returns the same report-not-error shape, and have `loomengine.Preflight` compose it with its own seed check.
This is the half of the three-tier model that makes "orchestrators require full initialization" enforceable rather than a convention, and it is the reason a producer can safely require nothing.

**Placement is decided, not open:** a new `internal/preflight` package, never a composite verb on `internal/fabricengine`.
`fabricengine` is already the repo's largest package and owns destruction, write containment and mutation recording;
a preflight that merely *reads* does not belong in that blast radius, and putting it there would drag every consumer of a read-only check into the dependency set of the repo's most dangerous package.

**T5 also lands the shared standalone-CLI foundations every standalone-entry task after it reuses — introduced once here because three tasks need them, not one.**
These originally lived inside T8's own brief, on the assumption `burlercli`/`perchcli` were the only standalone entry points.
They are not: T7 now delivers a standalone `lyx webster run` too, T7 sits in wave 3, and T8 sits in wave 4 — so anything only T8 introduced would not exist yet when T7 needs it.
Lifting it into T5, which already precedes both, removes the ordering hazard instead of making T7 depend on T8 or duplicating the logic a second time.

- **`internal/buildinfo`** — a stdlib-only leaf package holding the exported `Channel` variable (today an unexported `package main` variable at `cmd/lyx/stencilseed.go:29`, stamped by `tools/deploy/main.go:62` as `-X main.buildChannel=dev`) plus a `StencilMode()` accessor.
  T5 repoints the ldflags path to `-X github.com/Knatte18/loomyard/internal/buildinfo.Channel=dev` and has `cmd/lyx/stencilseed.go` read from there instead of its own variable.
  The stamping mechanism and the [Dev/Prod Binary Separation](../../CONSTRAINTS.md#devprod-binary-separation) invariant are unchanged — only the symbol's home moves.
  A stdlib-only leaf is what keeps it importable from `cmd/lyx` and from any `*cli` package without cycle risk; `burlercli`, `perchcli` (T8) and `webstercli` (T7) each read `buildinfo.StencilMode()` directly rather than re-deriving it.
- **`internal/standalonestate`** — a stdlib-only leaf holding the `<state>` derivation every standalone CLI entry needs.
  `hash8` is `SHA-256` over the normalized absolute target path, truncated to the first eight hex characters.
  Normalization is not optional: the input goes through `filepath.EvalSymlinks` then `filepath.Clean`, falling back to `Clean` alone when `EvalSymlinks` fails, and compared case-insensitively on Windows — exactly the semantics `internal/lyxcwd/anchor.go`'s `normalizePath`/`samePath` already implement for the same class of problem.
  Without it, two spellings of the same directory (a symlinked path, a differing-case path on Windows or macOS) hash differently and produce a different `<state>`, socket and session, silently destroying the "one tmux server per target directory, resumable" property this buys.
  `<state>` is per-OS:

  | GOOS | `<state>` |
  |---|---|
  | `windows` | `%LOCALAPPDATA%\lyx\<hash8>\` |
  | everything else | `$XDG_STATE_HOME/lyx/<hash8>/`, falling back to `~/.local/state/lyx/<hash8>/` |

  `internal/standalonestate` exports the target-path → `(stateDir, hash8)` derivation as one function.
  Each consuming CLI (`burlercli`, `perchcli`, `webstercli`) derives its own `reed.Geometry.SocketKey`/`SessionName`/`RepoName`/`HubPath` and `anchorRoot` from the shared `hash8`/`stateDir` pair, so two standalone runs against the same target directory — regardless of which producer CLI — agree on socket, session and state location.
- **The root pre-run's fictional-hub stencil write is gated here, not in T8.**
  `cmd/lyx/main.go:97` sets `cobra.EnableTraverseRunHooks`, so root's `seedStencils` (`main.go:87`) runs before every module pre-run — and it triggers on bare `lyxcwd.Resolve` success (`stencilseed.go:51-56`), then calls `seedStencilsAt(l.HubPath, …)`.
  Pointed at a plain downloaded repo, that writes `<repo-parent>/_board/_lyx/stencils/**` and tries to commit it: the fictional-hub write the tier-1-AND-tier-2 trigger rule exists to prevent, happening one layer above any single CLI.
  This is a live defect today, independent of standalone — invisible only because nobody points `lyx` at a non-lyx git repo on purpose yet.
  T5 gates `seedStencils` on the identical tier-1-AND-tier-2 check its own lifted `internal/preflight` package makes reachable, which protects every module's standalone entry — T7's and T8's alike — without either re-implementing the gate.

This means T8's own brief no longer originates `internal/buildinfo`, the `hash8` rule, or the root-gate fix — it consumes all three from here, and T7 does the same.

**Files.** `internal/loomengine/preflight.go`, `internal/loomengine/report.go` (or wherever `Report`/`Failure`/`CheckID` live), their tests;
a new `internal/preflight` package for the lifted checks plus its `doc.go` and tests;
a new `internal/buildinfo` package plus its `doc.go` and tests;
a new `internal/standalonestate` package plus its `doc.go` and tests;
`cmd/lyx/stencilseed.go` (reads `buildinfo.StencilMode()` instead of its own `buildChannel`; gates `seedStencils` on tier-1-AND-tier-2); `tools/deploy/main.go` (line 62, the ldflags path);
`CONSTRAINTS.md` if the three-tier rule lands as a named invariant (see T10).

**Depends on.** Nothing.
**Parallel-safe with.** T4.
Touches `cmd/lyx/stencilseed.go` and `tools/deploy/main.go`, neither of which T4 touches.
**Verify.** `go test ./internal/loomengine/... ./internal/fabricengine/... ./internal/preflight/... ./internal/buildinfo/... ./internal/standalonestate/...`;
`loomengine.Preflight`'s existing behaviour must be unchanged — same `Report`, same failure classification, same report-not-error contract for every case its current tests pin;
plus a test asserting `seedStencils` no-ops (writes nothing, commits nothing) when run against a plain git repository with no `_board` beside it.

---

### Wave 3 — producer engines (2 parallel)

---

**T6 — `burlerengine` + `perchengine` told-geometry**
`slug: burler-perch-told-geometry`

**Brief.** These two are one task, not two: `perchengine` imports `burlerengine` directly, and `perchcli` imports both, so a `burlerengine.New` signature change without a matching `perchengine`/`perchcli` change does not compile.
`burlerengine.New(shuttle, layout, cfg, stencilsDir)` becomes `New(shuttle, worktreeRoot, anchorRoot string, cfg, stencilsDir)` — `worktreeRoot` for `Profile.validate`, `anchorRoot` for the `.lyx/burler` directory.
`perchengine.New(burler, shuttle, cfg, layout, opts)` takes `gateDir` (today `layout.WorktreePath()`) as a told string;
`RunsDir(l)`/`ScratchDir(l)` take `anchorRoot`.
The CLI construction sites pass `layout.WorktreePath()`/`layout.AnchorPath()` unchanged for now — T8 is what makes them optional, and T8's pinned-values table is where each of these parameters gets its standalone answer.
This task changes nothing about where those directories resolve in a real worktree.
`internal/hubgeom` holds the hub-mode `Location`-to-geometry conversion and exports `ReedGeometry` today;
this task adds its own `BurlerGeometry`/`PerchGeometry` there rather than re-deriving the construction inline at each CLI site.

**Files.** `internal/burlerengine/engine.go`, `internal/burlerengine/doc.go` and tests; `internal/perchengine/engine.go`, `internal/perchengine/identity.go`, `internal/perchengine/doc.go` and tests; `internal/burlercli/cli.go` (105), `internal/perchcli/cli.go` (145 and the `runDirBase`/`scratchDirBase` assignments), `internal/perchcli/run.go` (294, 301); `cmd/lyx/constructoranchoring_test.go` (its `perchengine.RunsDir`/`ScratchDir` rows, 79/89/128/138 — directly beneath T7's rows, see the wave-3 adjacency note above).

**Depends on.** T3 (`shuttleengine.NewRunner`'s new signature is called from the same CLI blocks), T4 (`pattern.Directive`'s new signature is called from `burlerengine/engine.go:103`).
**Parallel-safe with.** T7.
**Verify.** `go test ./internal/burlerengine/... ./internal/perchengine/... ./internal/burlercli/... ./internal/perchcli/... ./cmd/lyx/...`, plus `go test -tags integration ./internal/perchcli/...`.

---

**T7 — `websterengine` + `webstercli` told-geometry, and Webster's own standalone entry**
`slug: webster-told-geometry`

**Brief.** Webster carries the largest `*lyxcwd.Location` surface — `state.go` ×4, `render.go` ×3, `runlevel.go` ×2, and `Layout` fields threaded through `RunDeps`, `beginbatch.go`, `recordbatch.go` and `recoverbatch.go`.
Convert them to told strings on the same rule as T6.
`render.go` additionally derives `fabricengine.StencilsDir(l.HubPath)` at three sites, and `runlevel.go` at one — those become a told `stencilsDir`, which is what removes `internal/fabricengine` from `websterengine`'s reason to know about hubs.

**This task also delivers `lyx webster run`'s standalone entry — Webster is standalone-capable in this decomposition, not deferred to a follow-up.**
`webstercli`'s `PersistentPreRunE` branches on the same tier-1-AND-tier-2 trigger T5's lifted `internal/preflight` makes reachable, exactly as T8 wires it for `burlercli`/`perchcli`: `(resolved, wired)` selects hub mode, everything else selects standalone — including the `(resolved, not wired)` plain-git-repo row.
`webstercli` gains `--stencils-dir` and `--target-dir`, with the same semantics T8 defines for them: `--stencils-dir` is read-only and honoured in both modes; `--target-dir` is refused in hub mode (where it is structurally `layout.WorktreePath()`) and defaults to cwd in standalone.
Both flags, and the build channel and `<state>` derivation behind them, come from T5's `internal/standalonestate`/`internal/buildinfo` — this task consumes them, it does not reintroduce either.

**Webster additionally needs a told plan directory, which burler and perch never had to answer.**
Today `internal/webstercli/cli.go:194` resolves it as `loomengine.PlanDir(layout)` — the value T1 moves onto `planparser.PlanDir(anchorPath string) string`.
`webstercli` gains `--plan-dir`, read-only in both modes: in hub mode it stays `planparser.PlanDir(l.AnchorPath())` exactly as today; in standalone it defaults to `planparser.PlanDir(<state>)`, overridable by the flag.
Unlike `--stencils-dir`, an absent plan directory in standalone has no bootstrap — a plan is authored content, not a shipped default, so a missing or empty `--plan-dir` in standalone is a plain usage error naming the flag, not a silent no-op.

**Webster's own written artifacts — batch state, per-card reports, run scratch — follow the same `worktreeRoot`/`anchorRoot` split T8 defines.**
Each card's `Target.Paths` resolve against `worktreeRoot` (the edited directory, `--target-dir`), while `websterengine.Dir`/`ReportsDir`/`PromptsDir`/`ScratchDir` resolve against `anchorRoot` (`<state>`) — so a standalone Webster run never writes hidden state into the directory it is editing, the same property T8's split gives burler and perch.
`internal/hubgeom` holds the hub-mode `Location`-to-geometry conversion and exports `ReedGeometry` today;
this task adds its own `WebsterGeometry` there rather than re-deriving the construction inline at the `webstercli` call site.

**Files.** `internal/websterengine/state.go`, `render.go`, `runlevel.go`, `beginbatch.go`, `recordbatch.go`, `recoverbatch.go`, `doc.go` and their tests; `internal/webstercli/cli.go`, `internal/webstercli/run.go` (new, mirroring `burlercli`/`perchcli`'s standalone-wiring split), `internal/webstercli/sync.go` and their tests; `cmd/lyx/constructoranchoring_test.go` (its `websterengine.Dir`/`ReportsDir`/`PromptsDir`/`ScratchDir` rows, 77-78/87-88/126-127/136-137 — directly above T6's rows, see the wave-3 adjacency note above); `cmd/lyx/*_test.go` for the help-tree and `Short`/`Long` obligations under the [CLI / Cobra Invariant](../../CONSTRAINTS.md#cli--cobra-invariant), since `--stencils-dir`/`--target-dir`/`--plan-dir` are new observable behaviour; `CONSTRAINTS.md` — the [Stencil Ownership](../../CONSTRAINTS.md#stencil-ownership-invariant) and [Durable-vs-Ephemeral State](../../CONSTRAINTS.md#durable-vs-ephemeral-state-invariant) rewords T8 lands for burler/perch should already generalise to Webster's identical shape; confirm the wording does not name burler/perch specifically, and adjust in this task's own commit if it does.

**Watch.** `internal/websterengine` is the one package whose raw-git-mutation ban is machine-checked (`cmd/lyx/rawgitmutation_test.go`), and the [Fabric Git Invariant](../../CONSTRAINTS.md#fabric-git-invariant-warp--weft) binds every git call it makes.
Nothing in this refactor should touch those paths, but the guard will catch it if it does.

**Depends on.** T1 (`planparser.PlanDir`), T2 (`websterengine.LoadConfig`'s template fallback), T3, T4, T5 (`internal/preflight`, `internal/buildinfo`, `internal/standalonestate`).
**Parallel-safe with.** T6, T8 — T7 does not depend on T8 and shares no files with either (`webstercli` is a distinct package from `burlercli`/`perchcli`).
**Verify.** The standalone entry is pinned the same way T8 pins its own, since it is the same class of behaviour landing on a third CLI:

- **Untagged unit test**: the "build the engine stack from told values" wiring, factored out of `PersistentPreRunE`, covering the same mode-selection truth table T8's test covers, plus the plan-dir default/override/missing-in-standalone cases.
- **`//go:build integration` test**: drive `RunCLIIn(<temp dir outside any git repo>, …)` and assert the pre-run reaches the run verb's own flag validation rather than a resolution error.

Plus `go test ./internal/websterengine/... ./internal/webstercli/... ./cmd/lyx/...`, `go test -tags integration ./internal/webstercli/...`, and one manual acceptance run of `lyx webster run --plan-dir p/ --stencils-dir s/` from a scratch directory outside any git repository.

---

### Wave 4 — the standalone entry (2 parallel)

---

**T8 — the standalone CLI path**
`slug: standalone-cli-entry`

**Brief.** The task the whole design exists for.
Make `burlercli` and `perchcli`'s `PersistentPreRunE` branch between hub mode and standalone mode instead of aborting.

**The trigger is tier 1 AND tier 2, never "`Resolve` errored".**
This is the one place the three-tier model has to be applied rather than merely described.
"`Resolve` failed" is not a usable trigger, because `Resolve` succeeds in any ordinary git repository run from its root — and a downloaded repo, named in this doc's own goal statement, is exactly that.
Triggering on `Resolve` alone would leave such a target in hub mode with `HubPath` set to its parent directory and `RepoName` to that directory's basename: the precise fictional-hub hazard this design rejects the synthetic `Location` for, complete with a mis-named tmux socket and a `.lyx` tree written into the repo being reviewed.

So: **hub mode requires tier 1 and tier 2 both** — `lyxcwd.Resolve` succeeds *and* Fabric is actually wired here (`fabricengine.Ready`-class, reached through the `internal/preflight` package T5 lifts, never by importing `fabricengine` into a CLI directly).
Everything else is standalone.
A plain git repo therefore resolves to **standalone**, which is the intended answer: there is no hub to coordinate with, so there is nothing for hub mode to be right about.

This is a deliberate behaviour change for one existing case, and it is stated rather than smuggled: a plain git repo run through `lyx burler run` today gets hub mode with fictional geometry, and after T8 it gets standalone.
A wired lyx worktree is unaffected — `Ready` is true there, so hub mode is selected exactly as today.
The check costs one `os.Stat`.

**The root pre-run's stencil-seeding fictional-hub write is already closed by T5, not by this task.**
`cmd/lyx/main.go:97` sets `cobra.EnableTraverseRunHooks`, so root's `seedStencils` (`main.go:87`) runs before every module pre-run, and would otherwise write a fictional `<repo-parent>/_board/_lyx/stencils/**` into a plain downloaded repo — exactly the write the trigger analysis above exists to prevent, happening one layer above any single CLI.
T5 gates `seedStencils` on the identical tier-1-AND-tier-2 check, protecting every module's standalone entry — T7's and T8's alike — before either lands.
This task does not touch `cmd/lyx/stencilseed.go`'s gating or `cmd/lyx/main.go`.

**A wired-but-broken hub is refused, never silently degraded to standalone.**
"Everything else is standalone" would otherwise decide the `(resolved, hub-damaged)` row by omission — a worktree whose junctions broke would quietly relocate its config reads and `.lyx` state to `<state>`, masking the breakage instead of reporting it.
The discriminator is `fabricengine.BoardDir(filepath.Dir(worktreeRoot))`: a plain git repo has no `_board` beside it, a damaged hub does.
So tier 2 failing **with** a `_board` present is a hard error naming `lyx fabric reconcile`, while tier 2 failing **without** one is an ordinary standalone target.
Standalone must never become the place broken hubs go to hide.
Add `--stencils-dir <path>` to replace `fabricengine.StencilsDir(layout.HubPath)`, bootstrapping it on first use via `stencilstore.Reconcile(dir, stencils.Registry(), mode, "")` with none of `cmd/lyx/stencilseed.go`'s Fabric-bound commit half.
With T2 landed, the three config loaders already degrade, so no config file is required.
After this task `lyx burler run --profile p.yaml --stencils-dir <dir>` works in a directory that is not a git repository.

**Every told value is pinned here, not left to the implementer.**
T3 and T6 turn each of these into a parameter;
this task is where each one gets its standalone answer, so that no value is silently defaulted from a fictional hub:

**The load-bearing move is splitting `worktreeRoot` from `anchorRoot`.**
In a real worktree they are usually the same directory, which is why it is tempting to point both at the target — but they answer different questions, and standalone is exactly where that difference becomes visible:

- `worktreeRoot` is the base every *caller-named* relative path resolves against.
  `burlerengine`'s `Profile.validate` resolves `Target.Paths`, `Fasit.Paths`, `ReviewPath`, `FixerReportPath` and both prior-report lists against it (`internal/burlerengine/profile.go:59-66`), and perch's `GateDir` is the gate command's working directory.
  This must be the target directory.
- `anchorRoot` is the base every *lyx-internal* `_lyx`/`.lyx` path is joined onto.
  This must be `<state>`.

Pointing both at the target is what would push a hidden `.lyx` tree into the reviewed folder, because `reedengine.stateDir()` (`lifecycle.go:43-44`, holding `reed.json`/`reed.lock`), `shuttleengine.runDirRoot`'s default (`rundir.go:49-56`), `burlerengine`'s `.lyx/burler`, and perch's `RunsDir`/`ScratchDir` are all `anchorRoot`-derived.
With the split, every one of them relocates automatically — the rows marked *derived* below are consequences of the two roots, not separate decisions to make or forget.

| Told value | Hub-resident source today | Standalone value |
|---|---|---|
| `worktreeRoot` | `l.WorktreePath()` | the absolute target directory (`--target-dir`, defaulting to cwd) |
| `anchorRoot` | `l.AnchorPath()` | `<state>` |
| config `baseDir` (all three loaders) | `l.AnchorPath()` | *derived* — `<state>`, so operator config lives at `<state>/_lyx/config/` |
| reed `Geometry.SocketKey` | `socketName(l.HubPath)` | `lyx-<hash8>`, deterministic from the target's absolute path |
| reed `Geometry.SessionName` | `SessionName(l.WorktreePath())` = `filepath.Base(worktreeRoot)` | `<basename of target>-<hash8>` |
| reed `Geometry.LogsDir` | `fabricengine.HubScratchDir(l.HubPath)/logs` = `<hub>/_board/.lyx/logs` | `<state>/logs`, told directly — **not** a told hub path, which would yield `<state>/_board/.lyx/logs` |
| reed `Geometry.RepoName` | `l.RepoName` | the target directory's basename |
| reed `Geometry.HubPath` | `l.HubPath` | `<state>` |
| reed state dir (`stateDir`) | `Join(AnchorPath(), ".lyx")` | *derived* — `<state>/.lyx` |
| shuttle run dir (`runDirRoot`) | `Join(AnchorPath(), ".lyx", "shuttle")` | *derived* — `<state>/.lyx/shuttle` |
| burler scratch | `Join(AnchorPath(), ".lyx", "burler")` | *derived* — `<state>/.lyx/burler` |
| perch `RunsDir` / `ScratchDir` | `AnchorPath()`-anchored `_lyx`/`.lyx` pair | *derived* — under `<state>` |
| stencils dir | `fabricengine.StencilsDir(l.HubPath)` | `--stencils-dir <path>`, optional; defaults to `<state>/_lyx/stencils`, bootstrapped on first use |
| stencil `Reconcile` mode | `buildChannel == "dev"` selector at `cmd/lyx/stencilseed.go:73-76` | the same selector, reached through a new `internal/buildinfo` — see below |

The standalone stencils default is `<state>/_lyx/stencils`, not a third top-level `<state>/stencils`, so it mirrors the hub exactly: stencils are `_lyx`-resident at `<hub>/_board/_lyx/stencils` (`fabricengine.StencilsDir`), and standalone's `<state>` plays the hub's role, sitting beside `<state>/_lyx/config/` under the same `_lyx` root.

**`--stencils-dir` is optional in both modes, and its default is what differs.**
Standalone defaults it to `<state>/_lyx/stencils`;
a resolved worktree defaults it to `fabricengine.StencilsDir(l.HubPath)` exactly as today.
An explicit `--stencils-dir` is honoured in both — refusing it in-hub would forbid the one thing it is most useful for, pointing a real worktree at an experimental stencil set, and buys nothing.
Omitting it is never an error, which is what keeps the standalone command a two-flag invocation rather than a three-flag one.

**Bootstrap applies to the standalone default only, never to a told directory.**
`<state>/_lyx/stencils` is `Reconcile`-seeded on first use because nothing else would ever create it;
an explicitly-passed `--stencils-dir` is read, never written, in either mode.
That is what makes the read-only characterisation in the `--target-dir` rationale below literally true rather than approximately true, and it means an operator who points the flag at a curated stencil set never has it silently reconciled out from under them.

**`hash8` and the per-OS `<state>` table are defined once, in T5's `internal/standalonestate` — see that task's brief rather than this one.**
This task's tables below name the standalone value each told parameter resolves to; the mechanism producing `<state>` and `hash8` themselves is T5's, reused here unchanged.

**The build channel and `<state>` derivation come from T5, not from this task.**
T5's `internal/buildinfo` and `internal/standalonestate` exist precisely because T7 (Webster's own standalone entry) needs them too and lands in an earlier wave than this task — see T5's brief for why they could not originate here.
`burlercli` and `perchcli` read `buildinfo.StencilMode()` and derive `<state>`/`hash8` through `standalonestate` exactly as `webstercli` does.

The two reed header tokens are pinned rather than blanked because `Engine.HeaderText` renders them at every boot and an unpinned value shows the operator an empty or fictional header.
`repo` naming the target's basename and `hub` naming `<state>` are both literally true in standalone: the thing being worked on, and where its state lives.

**Operator config is supported in standalone, at `<state>/_lyx/config/`.**
This falls out of `anchorRoot = <state>`, since all three loaders already take that same base.
It matters because reed's config carries genuinely machine-specific keys — `tmux` and `shell` (`internal/reedengine/config.go:19-20`) — which a template default cannot get right on every machine.
With T2 landed the directory is optional (an absent file resolves the embedded template), so the common case needs no config at all;
when an operator does need one, `<state>` is deterministic from the target path and the command prints it, so the path is findable rather than guessed.

`<state>` is per-OS, because this repo ships Windows (`internal/fslink`'s junctions, `shell.ForGOOS`'s `pwsh`, `cmd/lyx/crosscompile_test.go`) and the hashing rule below is itself Windows-aware, so a POSIX-only state path would contradict the same paragraph:

| GOOS | `<state>` |
|---|---|
| `windows` | `%LOCALAPPDATA%\lyx\<hash8>\` |
| everything else | `$XDG_STATE_HOME/lyx/<hash8>/`, falling back to `~/.local/state/lyx/<hash8>/` |

`hash8` namespaces state by target directory, the same way `shedadapters`' perch run-ids namespace by profile content.

**The socket is deterministic, not per-invocation, and that is what makes resume work.**
`reedengine.ReedState` persists both `Socket` and `Session` (`internal/reedengine/state.go:32-36`) and reed's entire Up/Resume model reads them back.
A per-invocation socket would make that persisted state unresumable and reduce the deterministic session name to decoration.
Deriving `socketKey` from the same `hash8` as the session name and the state directory makes all three agree: one tmux server per target directory, resumable across invocations, and no collision between two standalone runs in sibling folders.
Standalone therefore reuses reed's existing server lifecycle unchanged, with `<state>` playing the role the hub plays today — the server persists deliberately, and `lyx reed down` is the existing teardown verb;
T8 adds no new lifecycle concept and must not silently kill a server it did not start.

**The target directory receives only what the caller explicitly named** — the profile's `review-path` and `fixer-report-path`, and nothing else.
Standalone never writes hidden state into a folder it was asked to review, which is what makes `lyx burler run` safe to point at an arbitrary directory.
That is a property of the `worktreeRoot`/`anchorRoot` split, not a rule an implementer has to remember at each write site.
It also settles the [Durable-vs-Ephemeral State Invariant](../../CONSTRAINTS.md#durable-vs-ephemeral-state-invariant) question cleanly: the invariant places every never-tracked file under `.lyx` at the mirrored subpath of the `_lyx` content it relates to, and standalone's `_lyx`/`.lyx` pair are ordinary siblings under `<state>`, so the rule is satisfied rather than bent or dodged.
Say so in the invariant's own text as part of this task's `CONSTRAINTS.md` edit.

**Files.** `internal/burlercli/cli.go`, `internal/burlercli/run.go` and tests; `internal/perchcli/cli.go`, `internal/perchcli/run.go` and tests; `cmd/lyx/*_test.go` for the help-tree and `Short`/`Long` obligations under the [CLI / Cobra Invariant](../../CONSTRAINTS.md#cli--cobra-invariant); `CONSTRAINTS.md`.
`internal/buildinfo` and `internal/standalonestate` are T5's files, not this task's — this task only imports them.

**Invariant rewords land in this task's own commit, not deferred to T10.**
This task is what introduces the told stencils directory and the standalone state locations, so both [Stencil Ownership Invariant](../../CONSTRAINTS.md#stencil-ownership-invariant) bullets it falsifies, plus the Durable-vs-Ephemeral clarification above, belong here:

- the **read-location** bullet, which pins reads to `<hub>/_board/_lyx/stencils/` — reworded to name a told absolute directory, with the hub path as what hub mode resolves to;
- the **seed-pass** bullet, which says the seed/refresh pass "runs once per process at `cmd/lyx`'s root pre-run" — reworded to "once per process: at `cmd/lyx`'s root pre-run in hub mode, or at the producer CLI's own pre-run in standalone mode".
  The load-bearing half of that bullet, *never lazily inside `stencilstore.Read`*, is preserved exactly;
  what changes is only which pre-run does it.
Deferring them to T10 would leave the shipped code contradicting a live invariant across two waves, against `CLAUDE.md`'s same-commit docs rule that T1, T4 and T3 all honour.
T10 keeps only the new three-tier invariant and the cross-doc consolidation.

**`--target-dir` is a resolution base, not a review target.**
It supplies `worktreeRoot` — the directory relative profile paths resolve against — and nothing more.
It never names what to review: that is the profile's `target.paths`, and the out-of-scope note about `lyx burler run <path>` below is precisely the rule that keeps these from becoming two ways to say the same thing.
**In hub mode `--target-dir` is refused, not honoured** — deliberately the opposite ruling from `--stencils-dir`, and for a reason rather than by inconsistency.
`--stencils-dir` names a directory that is only ever *read*, so pointing a real worktree at an experimental stencil set is harmless and useful.
`--target-dir` is the base that `Profile.validate` resolves `ReviewPath` and `FixerReportPath` against — it decides where the round *writes* — so honouring it in hub mode would place artifacts outside the anchored subtree that Fabric's positive-only commit pathspec covers, silently stranding them.
In hub mode the value is structurally `layout.WorktreePath()`;
standalone defaults it to cwd and honours the flag.

**Watch.** Both new flags — `--stencils-dir` and `--target-dir` — need `Short`/`Long` text, and help accuracy is a review obligation whenever observable behaviour changes.
The `--stencils-dir` bootstrap and the `<state>` directory both write files, so the command must say where it wrote them.

**Depends on.** T2, T3, T4, T5, T6 — every wave before this one except T1, which is independent of the standalone path.
**T5** is the non-obvious edge: the hub-mode trigger is a tier-2 check, and T5 is what makes such a check reachable from a CLI package without importing `internal/fabricengine` into one.
T5 is also where `internal/buildinfo`, `internal/standalonestate` and the root-pre-run stencil gate now live, all three reused here rather than introduced by this task — see T5's brief.
That edge is also the answer to T5 otherwise looking like the one task delivering nothing toward standalone burler — it delivers the trigger, and the shared plumbing besides.
**Parallel-safe with.** T9.
**Verify.** The one behaviour this whole design exists for must be pinned by an automated test, not only by a manual run — nothing else in the ten tasks covers it, and T2 already sets the precedent by requiring a new test per config loader.
Two tiers, both required:

- **Untagged unit test**, one per CLI: factor the "build the engine stack from told values" wiring out of `PersistentPreRunE` into a function taking the resolved-or-not state as a parameter, and assert it produces a fully-built stack with the pinned standalone values above.
  Cover the mode-selection truth table explicitly, since the plain-git-repo row is the one the r5 review caught: `(resolved, wired)` selects hub mode, and `(resolved, not wired)` — the downloaded repo — selects standalone exactly as `(unresolved, …)` does.
  This must be tier 1, which is only possible via the extraction — a test that drives the real pre-run reaches `lyxcwd.Resolve`, which spawns git through `gitexec.Run` and would breach the [Test Tier Purity Invariant](../../CONSTRAINTS.md#test-tier-purity-invariant).
- **`//go:build integration` test**, one per CLI: drive `RunCLIIn(<temp dir outside any git repo>, …)` and assert the pre-run reaches the run verb's own flag validation rather than a resolution error.
  This is what pins the actual wiring rather than the extracted helper.
  `internal/perchcli/cli_integration_test.go` is the existing shape to follow, and the package already has a hermetic `TestMain`.

Plus `go test ./internal/burlercli/... ./internal/perchcli/... ./cmd/lyx/...`, `go test -tags integration ./internal/burlercli/... ./internal/perchcli/...`, and one manual acceptance run of `lyx burler run --profile p.yaml --stencils-dir <dir>` from a scratch directory outside any git repository — kept as a smoke check on top of the tests, never as the only evidence.

---

**T9 — `scoutengine` told-geometry (OPTIONAL)**
`slug: scout-told-geometry`

**Brief.** Scout already works standalone, so this task delivers no capability — it buys uniformity only, and dropping it costs nothing but a documented deviation.
`scoutengine.DaemonStateFile(l, lang)` and `DaemonLock(l, lang)` read `l.AnchorPath()` and nothing else;
`Options.Layout` exists solely to feed them.
Replace with a told `anchorRoot string`, and delete `scoutcli`'s `resolveLocation` synthesis (`cli.go:455-468`) — with told geometry there is no object left to synthesize, which is the cleanest possible outcome for the one place in the repo that currently mints a fictional `Location`.
`lookupContext`'s registry fallback to `BuiltinRegistry()` stays exactly as it is;
it is already the correct shape.

**Files.** `internal/scoutengine/daemonstate.go`, `ensureserver.go`, `refs.go` and tests; `internal/scoutcli/cli.go` and tests; `cmd/lyx/constructoranchoring_test.go` (its `scoutengine.DaemonStateFile`/`DaemonLock` rows, 91-92/140-141).

**Depends on.** Nothing.
**Parallel-safe with.** every other task — no other task touches `scoutengine` or `scoutcli`.
**Verify.** `go test ./internal/scoutengine/... ./internal/scoutcli/... ./cmd/lyx/...` and `go test -tags scout ./internal/scoutengine/...`;
plus confirm `lyx scout` still resolves symbols in a directory outside any hub, which is the behaviour this task must not regress.

---

### Wave 5 — consolidation

---

**T10 — invariants and docs for the told-geometry rule**
`slug: standalone-docs-and-invariants`

**Brief.** Land the cross-cutting rule this work establishes, once every package obeys it.
Add a named invariant to `CONSTRAINTS.md` stating the three tiers and the producer/orchestrator split, with its enforcement basis named honestly (an import-allowlist test per producer package where one exists, review obligation where none does).
Reword the [Cwd Resolution Invariant](../../CONSTRAINTS.md#cwd-resolution-invariant) to state what `Resolve` actually validates, since the current text leaves room for the misreading this whole document had to correct.
The Stencil Ownership and Durable-vs-Ephemeral rewords are **not** here — they land in T8's own commit alongside the code that makes them true, per T8's brief.
Record scout's remaining deviation if T9 was skipped.
Update `docs/overview.md` if the execution stack description changes, and move the roadmap entries to Done.

**Files.** `CONSTRAINTS.md`, `docs/overview.md`, `manifest/roadmap.md`, this file (deleted per the [documentation lifecycle](../../docs/overview.md#documentation-lifecycle) once the work ships), and the `doc.go` of each converted package.

**Depends on.** T1 through T8 (T9 optional) — every task that changes code this one documents.
T5 in particular: the three-tier invariant this task lands in `CONSTRAINTS.md` is only true once the orchestrator-agnostic preflight actually exists, so writing the rule before T5 ships would pin a model the code does not yet implement.
**Verify.** `go test ./...`;
`internal/lyxcwd/docslink_test.go` for markdown link integrity across the reworded docs.

---

## What is deliberately not in scope

- **Changing the Cwd Resolution Invariant's substance.**
  This work changes *which callers must invoke* `lyxcwd.Resolve`, never what `internal/lyxcwd` owns or what `Resolve` validates.
  T10's rewording makes the existing text accurate;
  it does not relax the gate.
- **A non-reed spawn path.**
  Every LLM spawn goes through reed by explicit design, for the interactive-tmux subscription reason stated in `CLAUDE.md`.
  The fix is to make `reedengine` geometry-optional, never to route around it.
- **Wiring `internal/shedadapters` into production.**
  Nothing in `cmd/` or any `*cli` package imports it yet.
  That is `loom`'s own producer-list work — see the Planned `loom` items in [../roadmap.md](../roadmap.md).
- **`lyx burler run <path>` as a positional-argument shape.**
  The motivating sentence in the discovery task says `lyx burler run <path>`, but `burler run` is already profile-driven (`--profile`), and a profile's `target.paths` is where a directory belongs.
  T8 should not invent a second way to say the same thing.

## Related

- [shed.md](shed.md) — the `ShedProducer` model this work serves, and [its engine-adapter seam](shed.md#engine-adapters--a-thin-shared-seam-not-one-per-producer), which already applies told-geometry one layer above the engines this doc converts.
- [loom.md](loom.md) — `loom`'s producer list, the first consumer that needs tier-3 preflight to be real.
- [hardener.md](hardener.md) — the second orchestrator, and the reason T5 lifts the preflight rather than leaving it inside `loomengine`.
- [../../CONSTRAINTS.md](../../CONSTRAINTS.md) — the [Cwd Resolution](../../CONSTRAINTS.md#cwd-resolution-invariant), [Durable-vs-Ephemeral State](../../CONSTRAINTS.md#durable-vs-ephemeral-state-invariant), [Stencil Ownership](../../CONSTRAINTS.md#stencil-ownership-invariant), [Pattern Leaf](../../CONSTRAINTS.md#pattern-leaf-invariant), [Tokenvocab Leaf](../../CONSTRAINTS.md#tokenvocab-leaf-invariant), [Planparser Sole-Parser](../../CONSTRAINTS.md#planparser-sole-parser-invariant), [Treadle Runner-Seam](../../CONSTRAINTS.md#treadle-runner-seam-invariant), [Shed Producer-Seam](../../CONSTRAINTS.md#shed-producer-seam-invariant), [CLI / Cobra](../../CONSTRAINTS.md#cli--cobra-invariant), [Test Tier Purity](../../CONSTRAINTS.md#test-tier-purity-invariant) and [Dev/Prod Binary Separation](../../CONSTRAINTS.md#devprod-binary-separation) invariants this work touches or is bound by.
  The first four are reworded by tasks in this decomposition (T1, T4, T3, T8, T10);
  the rest bind it without changing.
- `internal/shedadapters/doc.go` — the "told, never derived" language this doc generalises downward.
- `internal/scoutcli/cli.go` — the working precedent for standalone degradation, and the one place a fictional `Location` is minted today.
