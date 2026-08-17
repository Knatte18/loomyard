# Producers standalone — running burler/perch/webster outside a lyx worktree

## What it is

`lyx burler run --profile p.yaml` should review an arbitrary directory — a "Models" folder, a downloaded repo, anything — with a custom prompt, from a machine with no lyx hub, no Fabric, no `_lyx/config/` seeding, not even a git repository.
The same for `lyx perch run`, and eventually for Webster.
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
Lifting them into a shared, orchestrator-agnostic preflight is what makes this model real rather than aspirational, and it is task **T8** below.

## What actually blocks standalone today

Every row below was verified against the current tree, not inherited from the discovery task.

### Layer 1 — engine constructors take `*lyxcwd.Location`

| Package | Sites | What it reads from the `Location` |
|---|---|---|
| `internal/shuttleengine` | `run.go` (`NewRunner`, `Runner.layout`), `rundir.go` (`runDirRoot`, `FindRun`) | `AnchorPath()`, `WorktreePath()` |
| `internal/reedengine` | `lock.go` (`New`, `Engine.layout`, `socketName`, `SessionName`), `lifecycle.go` (`HubLogsDir`, pane spawn cwd) | `HubPath` (tmux socket name), `WorktreePath()` (session name), `AnchorPath()` (pane cwd) |
| `internal/tokenvocab` | `tokenvocab.go` (`Ctx.Layout`) | `RepoName`, `HubPath` — two fields of a 327-line package; built only at `reedengine/header.go:16` |
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

### Layer 3 — three of five config loads hard-fail on an absent file

| Loader | Behaviour with no config present | Blocks standalone |
|---|---|---|
| `modelspec.LoadRegistry` | returns `builtins()` — `sonnet`/`opus`/`haiku`/`fable` mapped to the `claude` engine | no |
| `burlerengine.LoadConfig` | own `os.ReadFile`, absent file decodes to the zero `Config{}` | no |
| `shuttleengine.LoadConfig` | `configengine.Load` → `"not initialized: _lyx/ directory not found"` | **yes** |
| `reedengine.LoadConfig` | same | **yes** |
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

The [Stencil Ownership Invariant](../../CONSTRAINTS.md#stencil-ownership-invariant) currently pins the read location to `<hub>/_board/_lyx/stencils/` specifically.
Its actual load-bearing content is "read from a file at call time, never from embedded bytes", which a told directory satisfies exactly.
The invariant is reworded to name a told absolute directory, with `<hub>/_board/_lyx/stencils/` as what the hub-resident path resolves to — task **T10**.

### no additive twins — parallelism comes from wave scheduling

The tempting way to parallelise a signature migration is to add the plain-string function beside the old one and leave the old one as a wrapper, so every package ships independently.
That is rejected.
Two near-identical signatures sitting side by side is a real cost paid immediately against a cleanup that is only promised, and the duplication reliably outlives the migration.

Parallelism is bought by scheduling instead.
The genuine constraint is not the import graph — it is *file* contention, and it concentrates in exactly two files, `internal/burlercli/cli.go` and `internal/perchcli/cli.go`, which every constructor change converges on.
Grouping tasks into waves whose file sets are disjoint gets 3-wide, then 2-wide, then 2-wide, then 2-wide, with no duplicated API at any point.

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
**Parallel-safe with.** T2, T4.
Shares `internal/webstercli/cli.go` with T4, at line 194 against T4's lines 179-181 — separate hunks, but sequence them if a merge conflict appears.
**Verify.** `go test ./internal/planparser/... ./internal/loomengine/... ./internal/webstercli/... ./cmd/lyx/...`, plus confirm `internal/loomengine` no longer appears in `internal/webstercli`'s production import set.

---

**T2 — config degrades to its embedded template**
`slug: config-template-fallback`

**Brief.** `shuttleengine.LoadConfig`, `reedengine.LoadConfig` and `perchengine.LoadConfigWithRegistry` all route `configengine.Load`, which refuses on an absent `_lyx/` (`config.go:52`) and on an absent config file (`config.go:60`), rewrapped by each caller as `"not initialized here; run \"lyx fabric reconcile\""`.
Add `configengine.LoadOrTemplate(baseDir, module string, template []byte) ([]byte, error)`: identical to `Load` except that both refusal branches instead resolve the caller-supplied `template` through `envsource.Build`/`yamlengine.Resolve`.
Repoint exactly those three loaders.
`configengine.Load` stays strict and unchanged for its five hub-scoped callers (`fabricengine`, `boardengine`, `loomengine`, `batcher`, `websterengine`), where an absent config means a broken hub.
This brings all five producer-path loaders to a single behaviour, matching `modelspec.LoadRegistry`'s `builtins()` fallback and `burlerengine.LoadConfig`'s zero-`Config{}` fallback, which already degrade.

**Files.** `internal/configengine/config.go`, `internal/configengine/config_test.go`; `internal/shuttleengine/config.go` (line 38), `internal/reedengine/config.go` (line 42), `internal/perchengine/config.go` (line 58).

**Depends on.** Nothing.
**Parallel-safe with.** T1, T4.
Touches `shuttleengine/config.go`, `reedengine/config.go` and `perchengine/config.go` — all distinct from the files T4 and T5 touch in those same packages.
**Verify.** `go test ./internal/configengine/... ./internal/shuttleengine/... ./internal/reedengine/... ./internal/perchengine/...`, plus a new test per loader asserting a `baseDir` with no `_lyx/` returns the template-derived config rather than an error.

---

**T4 — `shuttleengine` + `reedengine` + `tokenvocab` told-geometry**
`slug: shuttle-reed-told-geometry`

**Brief.** Convert the three lowest-level producer packages off `*lyxcwd.Location` in one task, because all their construction sites are the same five CLI files and splitting them would only manufacture conflicts.
`shuttleengine.NewRunner(reed, engine, layout, cfg)` becomes `NewRunner(reed, engine, anchorRoot, worktreeRoot string, cfg)`;
`runDirRoot` and `FindRun` take `anchorRoot` instead of `layout`.
`reedengine.New(cfg, layout)` becomes `New(cfg, socketKey, sessionName, anchorRoot string)` — `socketKey` replacing the `socketName(layout.HubPath)` derivation is the single most important line in this task, since it is where a faked `HubPath` would otherwise silently mis-name the tmux socket;
`HubLogsDir` takes the hub path as a told string.
`tokenvocab.Ctx.Layout` becomes two plain fields (`RepoName`, `HubPath`), which drops `internal/lyxcwd` from `tokenvocab`'s import set entirely.

**Files.** `internal/shuttleengine/run.go`, `internal/shuttleengine/rundir.go`; `internal/reedengine/lock.go`, `internal/reedengine/lifecycle.go`, `internal/reedengine/header.go` (line 16); `internal/tokenvocab/tokenvocab.go`, `internal/tokenvocab/doc.go`; construction sites `internal/burlercli/cli.go` (103-104), `internal/perchcli/cli.go` (143-144), `internal/webstercli/cli.go` (179-181), `internal/shuttlecli/cli.go` (92-93), `internal/reedcli/cli.go` (83); the tests in each package; `CONSTRAINTS.md` ([Tokenvocab Leaf Invariant](../../CONSTRAINTS.md#tokenvocab-leaf-invariant) loses `internal/lyxcwd` from its allowlist).

**Watch.** `reedengine` spawns real OS processes, so the [Live-Substrate Spawn Observability](../../CONSTRAINTS.md#live-substrate-spawn-observability) invariant binds every touched spawn path — the `logger.Info`/`Warn` calls in `lifecycle.go` must survive the refactor intact.

**Depends on.** Nothing.
**Parallel-safe with.** T1, T2.
**Verify.** `go test ./internal/shuttleengine/... ./internal/reedengine/... ./internal/tokenvocab/... ./internal/burlercli/... ./internal/perchcli/... ./internal/webstercli/... ./internal/shuttlecli/... ./internal/reedcli/...`, plus `go test -tags integration ./internal/reedengine/...` for the tmux paths, plus confirm `internal/lyxcwd` is gone from `internal/tokenvocab`'s production imports.

---

### Wave 2 — mid-layer (2 parallel)

---

**T3 — `pattern` told-geometry**
`slug: pattern-told-geometry`

**Brief.** `pattern.Directive(l *lyxcwd.Location, stencilsDir string, role Role)` uses `l` for exactly one thing: `isActive(l)` → `FileHere(l)` → `File(Join(l.WorktreePath(), l.AnchorRel))`, which is `File(l.AnchorPath())`.
Change it to `Directive(anchorPath, stencilsDir string, role Role)`, with the empty string meaning inactive — replacing today's `l == nil` branch and preserving the existing `("", nil)` behaviour exactly.
Delete `FileHere`;
the already-exported `File(baseDir string)` is what it wraps.
`isActive` takes the same string.
This drops `internal/lyxcwd` from `pattern`'s imports, shrinking the [Pattern Leaf Invariant](../../CONSTRAINTS.md#pattern-leaf-invariant) allowlist to stdlib plus `lyxdirs`, `stencilstore` and `stencil`.

**Files.** `internal/pattern/pattern.go`, `internal/pattern/doc.go`, `internal/pattern/pattern_test.go`, `internal/pattern/leaf_enforcement_test.go`; call sites `internal/burlerengine/engine.go` (103), `internal/websterengine/render.go` (174, 237), `internal/loomengine/plan.go` (70); `CONSTRAINTS.md`.

**Depends on.** Nothing structurally, but scheduled after wave 1 because it edits `burlerengine/engine.go`, `websterengine/render.go` and `loomengine/plan.go`, which T5, T7 and T1 respectively also touch.
**Parallel-safe with.** T8.
**Verify.** `go test ./internal/pattern/... ./internal/burlerengine/... ./internal/websterengine/... ./internal/loomengine/...`, plus confirm `internal/lyxcwd` is gone from `internal/pattern`'s production imports and that the leaf-enforcement allowlist was tightened rather than left permissive.

---

**T8 — lift the orchestrator preflight out of `loomengine`**
`slug: orchestrator-preflight`

**Brief.** `loomengine.Preflight` is the only implementation of tiers 1 and 2 — geometry resolution plus `fabricengine.PrimeName`/`Clean`/`Ready`/`Healthy` — and its check 4 is `loom`-specific (`_lyx/loom/status.json`).
`Hardener`, and any future `Shed` product, would have to re-implement checks 1-3 verbatim.
Extract checks 1-3 into an orchestrator-agnostic package that returns the same report-not-error shape, and have `loomengine.Preflight` compose it with its own seed check.
This is the half of the three-tier model that makes "orchestrators require full initialization" enforceable rather than a convention, and it is the reason a producer can safely require nothing.

**Files.** `internal/loomengine/preflight.go`, `internal/loomengine/report.go` (or wherever `Report`/`Failure`/`CheckID` live), their tests;
a new `internal/*` package for the lifted checks plus its `doc.go` and tests;
`CONSTRAINTS.md` if the three-tier rule lands as a named invariant (see T10).

**Open for the implementer.** Whether the lifted package is new (`internal/preflight`) or the checks move onto `internal/fabricengine` as a composite verb.
New package is the recommendation — `fabricengine` is already the repo's largest package and owns destruction, write containment and mutation recording, and a preflight that merely *reads* does not belong in that blast radius.

**Depends on.** Nothing.
**Parallel-safe with.** T3.
**Verify.** `go test ./internal/loomengine/... ./internal/fabricengine/...` plus the new package's own tests;
`loomengine.Preflight`'s existing behaviour must be unchanged — same `Report`, same failure classification, same report-not-error contract for every case its current tests pin.

---

### Wave 3 — producer engines (2 parallel)

---

**T5 — `burlerengine` + `perchengine` told-geometry**
`slug: burler-perch-told-geometry`

**Brief.** These two are one task, not two: `perchengine` imports `burlerengine` directly, and `perchcli` imports both, so a `burlerengine.New` signature change without a matching `perchengine`/`perchcli` change does not compile.
`burlerengine.New(shuttle, layout, cfg, stencilsDir)` becomes `New(shuttle, worktreeRoot, anchorRoot string, cfg, stencilsDir)` — `worktreeRoot` for `Profile.validate`, `anchorRoot` for the `.lyx/burler` directory.
`perchengine.New(burler, shuttle, cfg, layout, opts)` takes `gateDir` (today `layout.WorktreePath()`) as a told string;
`RunsDir(l)`/`ScratchDir(l)` take `anchorRoot`.
The CLI construction sites pass `layout.WorktreePath()`/`layout.AnchorPath()` unchanged for now — T6 is what makes them optional.

**Files.** `internal/burlerengine/engine.go`, `internal/burlerengine/doc.go` and tests; `internal/perchengine/engine.go`, `internal/perchengine/identity.go`, `internal/perchengine/doc.go` and tests; `internal/burlercli/cli.go` (105), `internal/perchcli/cli.go` (145 and the `runDirBase`/`scratchDirBase` assignments), `internal/perchcli/run.go` (294, 301).

**Depends on.** T3 (`pattern.Directive`'s new signature is called from `burlerengine/engine.go:103`), T4 (`shuttleengine.NewRunner`'s new signature is called from the same CLI blocks).
**Parallel-safe with.** T7.
**Verify.** `go test ./internal/burlerengine/... ./internal/perchengine/... ./internal/burlercli/... ./internal/perchcli/...`, plus `go test -tags integration ./internal/perchcli/...`.

---

**T7 — `websterengine` + `webstercli` told-geometry**
`slug: webster-told-geometry`

**Brief.** Webster carries the largest `*lyxcwd.Location` surface — `state.go` ×4, `render.go` ×3, `runlevel.go` ×2, and `Layout` fields threaded through `RunDeps`, `beginbatch.go`, `recordbatch.go` and `recoverbatch.go`.
Convert them to told strings on the same rule as T5.
`render.go` additionally derives `fabricengine.StencilsDir(l.HubPath)` at three sites, and `runlevel.go` at one — those become a told `stencilsDir`, which is what removes `internal/fabricengine` from `websterengine`'s reason to know about hubs.
This is deprioritised relative to burler/perch for the standalone goal, but it is required before Webster can be a `ShedProducer` driven by an orchestrator that is not `loom`.

**Files.** `internal/websterengine/state.go`, `render.go`, `runlevel.go`, `beginbatch.go`, `recordbatch.go`, `recoverbatch.go`, `doc.go` and their tests; `internal/webstercli/cli.go`, `internal/webstercli/sync.go` and their tests.

**Watch.** `internal/websterengine` is the one package whose raw-git-mutation ban is machine-checked (`cmd/lyx/rawgitmutation_test.go`), and the [Fabric Git Invariant](../../CONSTRAINTS.md#fabric-git-invariant-warp--weft) binds every git call it makes.
Nothing in this refactor should touch those paths, but the guard will catch it if it does.

**Depends on.** T3, T4.
**Parallel-safe with.** T5.
**Verify.** `go test ./internal/websterengine/... ./internal/webstercli/... ./cmd/lyx/...`.

---

### Wave 4 — the standalone entry (2 parallel)

---

**T6 — the standalone CLI path**
`slug: standalone-cli-entry`

**Brief.** The task the whole design exists for.
Make `burlercli` and `perchcli`'s `PersistentPreRunE` branch: when `lyxcwd.Resolve(cwd)` fails, do not abort — build the engine stack from told values instead.
Add `--stencils-dir <path>` to replace `fabricengine.StencilsDir(layout.HubPath)`, bootstrapping it on first use via `stencilstore.Reconcile(dir, stencils.Registry(), mode, "")` with none of `cmd/lyx/stencilseed.go`'s Fabric-bound commit half.
Decide and document what the target directory is in standalone mode — the recommendation is that the positional target (or `--target-dir`, mirroring `scoutcli`) supplies both `worktreeRoot` and `anchorRoot`, and that reed's `socketKey` is minted per invocation rather than derived from any directory.
With T2 landed, the three config loaders already degrade, so no config file is required.
After this task `lyx burler run --profile p.yaml --stencils-dir <dir>` works in a directory that is not a git repository.

**Files.** `internal/burlercli/cli.go`, `internal/burlercli/run.go` and tests; `internal/perchcli/cli.go`, `internal/perchcli/run.go` and tests; `cmd/lyx/*_test.go` for the help-tree and `Short`/`Long` obligations under the [CLI / Cobra Invariant](../../CONSTRAINTS.md#cli--cobra-invariant).

**Watch.** Every new flag needs its `Short`/`Long` updated, and help accuracy is a review obligation whenever observable behaviour changes.
The `--stencils-dir` bootstrap writes files, so it must state clearly where it wrote them.

**Depends on.** T2, T3, T4, T5.
**Parallel-safe with.** T9.
**Verify.** `go test ./internal/burlercli/... ./internal/perchcli/... ./cmd/lyx/...`;
plus a manual acceptance check that is the whole point of this work — run `lyx burler run --profile p.yaml --stencils-dir <dir>` from a scratch directory outside any git repository and confirm it reaches a real round rather than a resolution error.

---

**T9 — `scoutengine` told-geometry (OPTIONAL)**
`slug: scout-told-geometry`

**Brief.** Scout already works standalone, so this task delivers no capability — it buys uniformity only, and dropping it costs nothing but a documented deviation.
`scoutengine.DaemonStateFile(l, lang)` and `DaemonLock(l, lang)` read `l.AnchorPath()` and nothing else;
`Options.Layout` exists solely to feed them.
Replace with a told `anchorRoot string`, and delete `scoutcli`'s `resolveLocation` synthesis (`cli.go:455-468`) — with told geometry there is no object left to synthesize, which is the cleanest possible outcome for the one place in the repo that currently mints a fictional `Location`.
`lookupContext`'s registry fallback to `BuiltinRegistry()` stays exactly as it is;
it is already the correct shape.

**Files.** `internal/scoutengine/daemonstate.go`, `ensureserver.go`, `refs.go` and tests; `internal/scoutcli/cli.go` and tests.

**Depends on.** Nothing.
**Parallel-safe with.** every other task — no other task touches `scoutengine` or `scoutcli`.
**Verify.** `go test ./internal/scoutengine/... ./internal/scoutcli/...` and `go test -tags scout ./internal/scoutengine/...`;
plus confirm `lyx scout` still resolves symbols in a directory outside any hub, which is the behaviour this task must not regress.

---

### Wave 5 — consolidation

---

**T10 — invariants and docs for the told-geometry rule**
`slug: standalone-docs-and-invariants`

**Brief.** Land the cross-cutting rule this work establishes, once every package obeys it.
Add a named invariant to `CONSTRAINTS.md` stating the three tiers and the producer/orchestrator split, with its enforcement basis named honestly (an import-allowlist test per producer package where one exists, review obligation where none does).
Reword the [Stencil Ownership Invariant](../../CONSTRAINTS.md#stencil-ownership-invariant) to name a told absolute stencils directory rather than the hub path specifically.
Reword the [Cwd Resolution Invariant](../../CONSTRAINTS.md#cwd-resolution-invariant) to state what `Resolve` actually validates, since the current text leaves room for the misreading this whole document had to correct.
Record scout's remaining deviation if T9 was skipped.
Update `docs/overview.md` if the execution stack description changes, and move the roadmap entries to Done.

**Files.** `CONSTRAINTS.md`, `docs/overview.md`, `manifest/roadmap.md`, this file (deleted per the [documentation lifecycle](../../docs/overview.md#documentation-lifecycle) once the work ships), and the `doc.go` of each converted package.

**Depends on.** T1 through T7 (T9 optional).
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
  T6 should not invent a second way to say the same thing.

## Related

- [shed.md](shed.md) — the `ShedProducer` model this work serves, and [its engine-adapter seam](shed.md#engine-adapters--a-thin-shared-seam-not-one-per-producer), which already applies told-geometry one layer above the engines this doc converts.
- [loom.md](loom.md) — `loom`'s producer list, the first consumer that needs tier-3 preflight to be real.
- [hardener.md](hardener.md) — the second orchestrator, and the reason T8 lifts the preflight rather than leaving it inside `loomengine`.
- [../../CONSTRAINTS.md](../../CONSTRAINTS.md) — the [Cwd Resolution](../../CONSTRAINTS.md#cwd-resolution-invariant), [Stencil Ownership](../../CONSTRAINTS.md#stencil-ownership-invariant), [Pattern Leaf](../../CONSTRAINTS.md#pattern-leaf-invariant), [Tokenvocab Leaf](../../CONSTRAINTS.md#tokenvocab-leaf-invariant), [Planparser Sole-Parser](../../CONSTRAINTS.md#planparser-sole-parser-invariant), [Treadle Runner-Seam](../../CONSTRAINTS.md#treadle-runner-seam-invariant) and [Shed Producer-Seam](../../CONSTRAINTS.md#shed-producer-seam-invariant) invariants this work touches or is bound by.
- `internal/shedadapters/doc.go` — the "told, never derived" language this doc generalises downward.
- `internal/scoutcli/cli.go` — the working precedent for standalone degradation, and the one place a fictional `Location` is minted today.
