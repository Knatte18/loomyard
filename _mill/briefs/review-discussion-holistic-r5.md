**If you find issues, REPORT them — do NOT fix them.**

You are an independent discussion reviewer for **loom: Plan-Write/Plan-Validate approval deadlock (F7)**.
Round **5**.
Reviewer model: **opusmedium**.

**You MAY use Read, Grep, and Glob to verify claims against source files.**
**CRITICAL: The one exception beyond that is Write -- use it exactly once, to write your full report to the file named in this brief's output-contract footer.**
**CRITICAL: Do NOT use Edit, or run git/bash.**
**CRITICAL: Review-only. Do NOT suggest modifications. Findings only.**
**CRITICAL: Do NOT read `reviews/`. Evaluate fresh each round.**

---

## Task

Read the discussion at `/home/knatte/Code/loomyard/wts/loom-plan-approval-gate/_mill/discussion.md`. The discussion file is the authoritative scope. Read files referenced in `## Technical Context` to verify claims.

Constraints:
# Constraints

Short, authoritative list of the repo's structural invariants.
Each is partly machine-enforced (named test, fails `go test`/CI) and partly a review obligation.
This file states rules only — no rationale, no incident narratives, no historical justification.
Fuller design/how-to lives in godoc and `docs/`.

## Cwd Resolution Invariant

`internal/lyxcwd` owns cwd resolution and nothing else — never weft, never a junction path, never any per-module subdirectory.

- **`root` always means the git worktree/repo root;
  the current working directory is `cwd`.**
  Never name a parameter, field, or local variable `root` for a value that is actually `cwd`, or vice versa.
- **What `Resolve` validates, in four sub-points.**
  1. `git rev-parse --show-toplevel` must succeed at `cwd`, else `ErrNotAGitRepo` — the only validation `Resolve` makes of the **repository** itself.
     Sub-points 2 and 3 below are genuine checks too, but they are checks about the anchor marker and the caller's position, not about whether the repository is a lyx worktree.
  2. An **absent** anchor marker is not an error — `AnchorRel` falls back to `"."`.
     Only a stale pre-rename marker hard-errors, with `ErrStaleAnchorMarker`.
  3. `cwd` must equal `Join(worktreeRoot, AnchorRel)`, else `ErrCwdOutsideAnchor`;
     with no marker this reduces to "cwd is the git worktree root".
  4. `HubPath` is `filepath.Dir(worktreeRoot)` **unconditionally** — never verified to be a hub — and `RepoName` is `Base(hubPath)` with a `-HUB` suffix trimmed, with no check the suffix was ever there.

     The consequence is the whole point of this bullet: `Resolve` succeeds in any ordinary git repository run from its root, and `HubPath`/`RepoName` are fiction in that case.
     Proving a worktree is lyx-initialized and Fabric-wired is [tier 2's and tier 3's](#told-geometry-invariant) job, not tier 1's.
- All cwd/worktree-root queries go through `lyxcwd.Getwd()`/`Resolve()`.
  Raw `os.Getwd` and `git rev-parse --show-toplevel` are banned outside `internal/lyxcwd` and `cmd/lyx/main.go`.
- `lyxcwd.Resolve` exposes only `RepoName`, `HubPath`, `WorktreeName`, `AnchorRel`,
  and the two derived accessors (`WorktreePath()`, `AnchorPath()`) built from them.
  It never resolves or exposes a weft path, a junction path, or any per-module subdirectory — those are not geometry `lyxcwd` owns.
- cwd must equal `AnchorPath()` exactly;
  `Resolve` returns `ErrCwdOutsideAnchor` otherwise. `ResolveWithAnchor` and `ResolveWorktree` are ungated — `ResolveWithAnchor` is a documented bypass, used only by callers that legitimately stand somewhere the gate would reject (fabric's clone, `gitkit`'s primitive repo fixtures).
- A module's own durable-storage subdirectory (e.g. `_lyx/plan`, `_lyx/webster`) is that module's own private relative-path constant, joined onto `AnchorPath()` directly — never a `lyxcwd` function call.
  Adding a module's own subdirectory is never a `lyxcwd` change.
  Its ephemeral twin is the Durable-vs-Ephemeral State Invariant below.
- `internal/lyxcwd`'s own imports are capped at stdlib plus `internal/gitexec` — this is what keeps `fabricengine` → `logger` → `lyxcwd` acyclic.
- Weft-sibling paths and junction construction belong to `internal/fabricengine`, never `lyxcwd`: `WeftWorktree`/`WeftRepoRoot`/`WarpLyxLink`/`WarpJunctions`/portal and launcher paths,
  and the `Prime`/sibling-worktree-list lookup they're built from, are `fabricengine`-private. `lyxcwd` never mentions weft.
  See the Fabric Vocabulary Invariant below for the vocabulary rule this bullet is one instance of.
- Geometry is structural, never config/env-overridable.
- The weft-backed junction name-set is injected from fabric config (`fabric.yaml`'s `pathspec`, read at `<Hub>/_board/_lyx/config/fabric.yaml`) — `fabricengine`'s concern, not `lyxcwd`'s.
- `AnchorRel` resolves from the recorded `.lyx-anchor` marker, not positionally from cwd;
  cwd is a validated exact-equality gate (`ErrCwdOutsideAnchor` if violated), falling back to `"."` only when the marker is absent. `ResolveWorktree`/`ResolveWithAnchor` read the same anchor with no cwd gate.
- The `"."` fallback applies to an ABSENT anchor only, never a stale one: a board carrying the pre-rename `.lyx-anchor` spelling (`lyxcwd.StaleAnchorFileName`) with no renamed marker beside it returns `ErrStaleAnchorMarker` from every resolver.
  `lyxcwd` is the single declarer of both marker names;
  fabric's clone-time guard aliases them rather than re-declaring the literals.
- `lyxcwd.WithCwd(ctx, dir)` / `lyxcwd.CwdFrom(ctx)` are the context-carried per-call cwd-injection seam: a caller threads an explicit cwd through a call chain instead of every callee reading the process working directory directly.
  The injected value must be absolute;
  `WithCwd` panics otherwise. `CwdFrom` falls back to `Getwd()` when `ctx` carries no injected value, so `Getwd` stays the single raw `os.Getwd` site.
  `context` is stdlib, so this seam does not affect the import cap below.
- `cmd/lyx/cwdmutation_test.go` guards a named per-file subject set of integration test files against reintroducing a process-wide `t.Chdir(` or `os.Chdir(` call, either spelling, once a file has been migrated onto the `RunCLIIn`/`WithCwd` seam.
  The subject set is per-file, never a package prefix, and carries exactly one allowlisted exemption, `internal/fabricengine/coalesce_integration_test.go`, whose cwd mutation is itself the assertion under test rather than a migration leftover.
  A file joins the subject set only when it is migrated onto the seam, never by default.
- **Enforced by** `internal/lyxcwd/enforcement_test.go` (`TestEnforcement_GeometryLiterals`) for the geometry-literal ban,
  `internal/lyxcwd/leaf_enforcement_test.go` (`TestLeafInvariant_AllowlistOnly`) for the import cap,
  and `cmd/lyx/cwdmutation_test.go` (`TestCwdMutation_MigratedFilesStayChdirFree`) for the chdir-mutation regression guard.

## Told-Geometry Invariant

An engine is handed the absolute paths it operates on and derives none of its own, so it runs identically inside a lyx hub and in a bare directory that is not a git repository.

- **Three resolution tiers:** 1) `lyxcwd.Resolve` — cwd is a git worktree root.
  2) `preflight.Check` (`fabricengine.Ready`/`Healthy`/`Clean`/`PrimeName`) — fabric wired, junctions intact, warp/weft in sync, tree clean.
  3) `loomengine.CheckSeed` — the orchestrator's own status seed alone, a separate producer row from tier 2 rather than a function composing it, so tier 3 does not re-run tiers 1 and 2.
- **Producer/orchestrator split:** a producer requires none of the three tiers.
  An orchestrator requires tier 3 and threads the extracted plain values down its producer list.
  A standalone CLI invocation requires none of the three, but its pre-run probes tier 1 via `preflight.ResolveMode`, which **degrades** to standalone (no hub lyx directory found, `ErrNotAGitRepo`, or `ErrCwdOutsideAnchor` with no hub geometry) or **refuses** (any other error, including `ErrCwdOutsideAnchor` inside a wired hub worktree's subdirectory).
- **Adapter direction:** where an engine takes a `Geometry` struct, `internal/hubgeom` (hub mode) and `internal/standalonegeom` (told mode) are its two sole constructors;
  both depend on the engines, never the reverse.
  Two packages instead take plain told values with no `Geometry` struct or adapter: `internal/treadleengine` (`runDir`, `Profile.GateDir`) and `internal/shedengine` (`StatusPath`/`LockPath`/`StatusLockPath`).
- **Mode trigger:** a standalone-capable CLI's pre-run consults `preflight.ResolveMode` only — never `preflight.Wired`, never a bare `HubPresent`.
- **Membership predicate:** a package is *bound* by this invariant when it takes its absolute paths from its caller and has no **direct** production import of `internal/lyxcwd` (transitive is fine).
  It is *machine-enforced* when a test in the package polices its production import set to exclude `internal/lyxcwd`;
  otherwise it is a *review obligation*.
  The two lists below are not exhaustive — they enumerate the packages converted by the producers-standalone waves.
- **Machine-enforced:** `internal/tokenvocab`, `internal/pattern`, `internal/buildinfo`, `internal/standalonestate` (each via `leaf_enforcement_test.go`'s `TestLeafInvariant_AllowlistOnly`), `internal/shedengine` (`seam_enforcement_test.go`'s `TestProducerSeamInvariant_AllowlistOnly`), `internal/treadleengine` (`seam_enforcement_test.go`'s `TestRunnerSeamInvariant_AllowlistOnly`), `internal/loomshed`, `internal/landingshed`, `internal/mergeresolve`, `internal/shedrecipe`, `internal/shedbuild`, `internal/loomrecipe` (each via `seam_enforcement_test.go`'s `TestToldGeometryInvariant_AllowlistOnly`).
- **Review obligation** (no machine guard for the told-geometry property): `internal/planparser`, `internal/configengine`, `internal/shuttleengine`, `internal/reedengine`, `internal/burlerengine`, `internal/websterengine`.
- **`internal/hubgeom`/`internal/standalonegeom` are adapters, not told packages** — they legitimately import `internal/lyxcwd` (hubgeom) or build from told strings (standalonegeom).
  They are bound instead by the adapter-direction rule above, which is itself a review obligation.
- **Enforced by** the twelve tests named above, for the machine-enforced half;
  the review-obligation half and the adapter-direction rule have no machine check.

## Lyxdirs Single-Declarer Invariant

`internal/lyxdirs` is the sole declarer of the two lyx directory-name tokens, `_lyx` (`LyxDirName`) and `.lyx` (`DotLyxDirName`).

- `internal/lyxdirs` stays stdlib-only, a zero-import leaf, so every module that needs either token can import it without cycle risk.
- No other production file may name either literal in path-construction context (a `filepath.Join` argument, a `+` operand, or a string const declaration value) — every caller uses `lyxdirs.LyxDirName` / `lyxdirs.DotLyxDirName` instead.
- **Enforced by** `internal/lyxcwd/enforcement_test.go` (`TestEnforcement_GeometryLiterals`).

## Durable-vs-Ephemeral State Invariant

Every never-tracked file lives under `.lyx`, at the mirrored subpath of the `_lyx` content it relates to. `_lyx` holds tracked content only.

- `_lyx` and `.lyx` are directory siblings under `AnchorPath()` — sole exception the hub-wide pair under `BoardDir(hub)`.
- A standalone session's `_lyx` and `.lyx` are ordinary directory siblings too, under the per-OS state directory `internal/standalonestate.Derive` returns rather than under `AnchorPath()` — the mirrored-subpath rule holds at that root exactly as it does at a hub anchor;
  standalone is a different root, not a deviation from the rule.
- No engine derives its own `.lyx` path — each module exposes a scratch accessor beside its durable one.
- `_lyx`/`.lyx` are structural (`fabricengine`'s `structuralCommittedDirs`/`structuralNeverCommittedDirs`), never read from `fabric.yaml`'s `pathspec` key, which is reserved for optional, explicitly-named dirs only.
- `.lyx` is in the wired name-set (`WiredNames`/`RepoWiredNames`) but never in the pathspec/commit-routing set (`PathspecNames`).
- The hub-wide never-tracked tree is `<hub>/_board/.lyx`, the mirrored sibling of `<hub>/_board/_lyx`, created by `fabricengine.CloneHub` after the board worktree exists — a real directory, never a junction. `fabricengine.HubScratchDir` is its sole constructor;
  `.lyx` stays slug-reserved via `structuralNeverCommittedDirs`, not via hub geometry.
- **Enforced by** `cmd/lyx/notransients_test.go`, `cmd/lyx/constructoranchoring_test.go`, `internal/fabricengine/structuraldirs_test.go`, `template_test.go`, `dotlyxjunction_integration_test.go`.
  A newly added transient's mirrored-subpath placement is a review obligation.

## Hub Containment Invariant

No hub-level container is ever junctioned into a worktree. `_board`, `_portals` and `_launchers` are reachable from the hub and only from the hub.

- A worktree sees warp and weft woven into one repo and nothing else — that is the fabric illusion, and it is also what makes worktree isolation geometric rather than a rule agents must remember.
- `_portals`/`_launchers` links point hub-inward (`<hub>/_portals/<anchor>/<slug>` → the worktree's `_lyx`), never worktree-outward. A per-worktree link to either is banned, not merely unbuilt.
- The `_board` convenience junction was wired until this rule landed and is now removed; the board is reached at `<hub>/_board`.
- **Enforced by** review discipline plus `internal/fabricengine`'s wiring having no worktree-side hub-container call site.

## gitkit Leaf Invariant

`internal/gitkit` production code imports only stdlib, `internal/lyxcwd`, `internal/weftname`, `internal/configengine`, and `internal/lyxdirs` — never `internal/configreg`, never a feature package.

- `gitkit` owns `MustRun`, `SeedConfig`, `HermeticGitEnv`, `GitStatusPorcelain`, and the primitive repo fixtures.
- `gitkit.CopyRepo` is callable from `internal/lyxcwd` alone;
  every other package takes a real hub from `internal/hubforge`.
- The other helpers are unpinned — `internal/gitrepo` uses `MustRun` and no fixture.
- Tests needing real config call `gitkit.SeedConfig(tb, dir, map[string]string{...})`.
- **Enforced by** `internal/gitkit/leaf_enforcement_test.go` (`TestLeafInvariant_AllowlistOnly`).

## hubforge Fabric-Fixture Invariant

Every hub fixture in the repo is built by `internal/hubforge` through `fabriccli.CloneAndWire`.
No hub is hand-assembled.

- `hubforge` builds fixtures only;
  it asserts nothing about fabric.
- No package inside `internal/fabriccli`'s dependency set may import `hubforge`.
  Such tests use an external `*_test` package, or `gitkit`.
- `hubforge.NewHub` is safe under concurrent use.
- Fixture teardown removes junctions via `internal/fslink` before `tb.TempDir()` cleanup.
- **Self-enforcing:** an in-package test importing `hubforge` from inside fabric's dependency set is a compile error.

## Modelspec Leaf Invariant

`internal/modelspec` production code imports only stdlib, `internal/configengine`, and `gopkg.in/yaml.v3`.

- `configreg` → `modelspec` is allowed (for `modelspec.ConfigTemplate`);
  the reverse is never allowed.
- **Enforced by** `internal/modelspec/leaf_enforcement_test.go` (`TestLeafInvariant_AllowlistOnly`).

## Treadle Runner-Seam Invariant

`internal/treadleengine` never imports `internal/burlerengine` or any `internal/*cli` package;
round runners adapt onto treadle's `RoundRunner` vocabulary in their own packages.

- Import allowlist: stdlib, `internal/lock`, `internal/logger`, `internal/state`, `internal/stencil`, `internal/stencilstore`, `internal/shuttleengine`, `gopkg.in/yaml.v3` — not `internal/lyxcwd` directly.
  Policed on direct imports only, not the transitive closure: `lyxcwd` is reachable through both `logger` and `shuttleengine`, so excluding it buys no isolation.
  What the exclusion enforces is that treadle is *told* its geometry and never derives it — `Engine.Run` takes a caller-supplied absolute `runDir`, a block's `Profile` carries a caller-supplied `GateDir`, and every path this package builds is joined onto one of those.
  `internal/stencilstore` takes a fully resolved absolute stencils directory from its caller and derives no geometry of its own, so treadle is still *told* its stencils directory exactly as it is told `runDir` and `Profile.GateDir` — the exclusion of `internal/lyxcwd` still means what it meant.
  The seed/refresh pass that keeps that directory populated runs once at `cmd/lyx`'s root pre-run rather than lazily inside `stencilstore.Read` — what that buys is that treadle is told its stencils directory and derives none of its own,
  not a transitive exclusion of `internal/fabricengine` from treadle's stack: `internal/treadleengine` → `internal/shuttleengine` → `internal/reedengine` → `internal/fabricengine` is no longer even a real transitive path — `HubLogsDir` moved to `internal/fabricengine`, and `reedengine.Engine` is now told its logs directory as `Geometry.LogsDir` rather than deriving it by calling in.
- **Enforced by** `internal/treadleengine/seam_enforcement_test.go` (`TestRunnerSeamInvariant_AllowlistOnly`).

## Shed Producer-Seam Invariant

`internal/shedengine` production code imports only the standard library, `internal/state`, and `internal/lock`;
producers adapt onto the package's own `ShedProducer` seam in their own packages.

- Import allowlist: stdlib, `internal/state`, `internal/lock` — not `internal/loomengine`, not any engine adapter package, not `internal/lyxcwd`, and not `internal/logger`.
  What the exclusion of `internal/lyxcwd` enforces is that `Shed` is *told* its geometry and never derives it — `StatusPath`, `LockPath`, and `StatusLockPath` are all caller-supplied, and the only paths the package constructs are the two lock parents it creates so a told path is usable.
  Policed on direct imports only, matching what the test checks; this particular allowlist happens to buy a stronger fact too, though — `internal/lyxcwd` is excluded transitively as well, because `internal/lock` imports no internal package at all and `internal/state` imports only `internal/fsx` and `internal/lock`.
  `internal/logger` is excluded rather than kept for future convenience: nothing in this package logs, the package starts no OS process so the Live-Substrate Spawn Observability invariant does not engage, and keeping `internal/logger` on the allowlist would forfeit the transitive property above for zero present benefit — `internal/logger` itself imports `internal/lyxcwd`.
- **Enforced by** `internal/shedengine/seam_enforcement_test.go` (`TestProducerSeamInvariant_AllowlistOnly`).

## Shed Recipe Registry Invariant

Every value in `internal/shedrecipe`'s registry constructs a `shedengine.ShedProducer` and nothing else — never an arbitrary Go module —
and the registry is one central `map[string]Constructor` literal reached only through `Lookup` and `Names`, never by `init()` self-registration and never by a runtime `Register` call.

`internal/shedrecipe` takes every absolute path from its caller and has no direct production import of `internal/lyxcwd`, in the precise form the Told-Geometry Invariant requires — **every root is told and none is derived; the package's only path construction is joining a told root with a recipe-relative value.**

`internal/shedbuild` is the registry's first outside caller: it reaches `internal/shedrecipe` only through its two exported accessors, `Lookup` and `Names`, and adds no registration mechanism of its own — a recipe naming an unregistered engine is an error for `internal/shedbuild` to report, never a reason to register one.

- **Enforced by** `internal/shedrecipe/seam_enforcement_test.go` (`TestToldGeometryInvariant_AllowlistOnly`) for the told-geometry half, and, for the registry-coverage half, two tests in two homes: `internal/loomrecipe/coverage_guard_test.go` (`TestCoverageGuard_EveryLoomRowHasAnEngine`), which drives loom's real row list against the registry,
  and `internal/shedrecipe/registry_test.go` (`TestRegistry_ShipsFourteenEntries`), which pins the registry's exact fourteen names.
  The `ShedProducer`-only restriction itself is a review obligation, since the `Constructor` signature already makes it a compile-time fact.

## Tokenvocab Leaf Invariant

`internal/tokenvocab` production code imports only stdlib and `internal/stencil`.

- Reverse import (`tokenvocab` → `reed`/`loom`/any feature package) is never allowed.
- **Enforced by** `internal/tokenvocab/leaf_enforcement_test.go` (`TestLeafInvariant_AllowlistOnly`).

## Buildinfo Leaf Invariant

`internal/buildinfo` production code imports nothing at all — not even the standard library — so `cmd/lyx` and every standalone CLI package can read the build channel with no cycle risk.

- The package exposes `Channel` and `IsDev()` only, and deliberately does not return a `stencilstore.Mode`, because `internal/stencilstore` imports `internal/logger` and `internal/stencil` and returning its type would destroy the leaf property.
- The mapping site is `stencilstore.ModeFor`.
- The ldflags stamp path `github.com/Knatte18/loomyard/internal/buildinfo.Channel` is guarded against silent drift by a test in `tools/deploy/main_test.go`, because Go's linker does not error on an unmatched `-X`.
- **Enforced by** `internal/buildinfo/leaf_enforcement_test.go` (`TestLeafInvariant_AllowlistOnly`).

## Standalonestate Leaf Invariant

`internal/standalonestate` production code imports only the standard library, with no permitted non-stdlib import.

- The package never resolves a working directory — no `filepath.Abs`, no `os.Getwd` — and rejects a relative target with an error, keeping cwd resolution wholly with `internal/lyxcwd` per the Cwd Resolution Invariant.
- `Derive` creates nothing on disk.
- **Enforced by** `internal/standalonestate/leaf_enforcement_test.go` (`TestLeafInvariant_AllowlistOnly`); the no-`filepath.Abs` half is a review obligation rather than a machine check.

## Pattern Leaf Invariant

`internal/pattern` production code imports only stdlib, `internal/lyxdirs`, `internal/stencilstore`, and `internal/stencil` — never `websterengine`, `burlerengine`, `loomengine`, or any other feature package.
Reverse import never allowed.
`internal/lyxdirs` is admissible because it is a stdlib-only zero-import leaf (its own Lyxdirs Single-Declarer Invariant), and therefore cannot participate in a cycle by construction.
`internal/stencil` is admissible for the same reason: it is a zero-import leaf, importing no internal package at all, and so cannot participate in a cycle by construction either.
`internal/stencilstore` is admissible on different grounds — it is **not** a leaf, importing `internal/stencil` and `internal/logger` — because the invariant restricts *feature* packages and `internal/stencilstore` is shared infrastructure rather than one, and its import closure is verified acyclic: nothing reachable from it imports `internal/pattern`.

- **Enforced by** `internal/pattern/leaf_enforcement_test.go` (`TestLeafInvariant_AllowlistOnly`).

## Stencil Ownership Invariant

Every producer prompt is read at call time from a told, absolute stencils directory, never from embedded bytes.
`<hub>/_board/_lyx/stencils/` is what that directory resolves to in hub mode, not the only possibility —
a standalone-capable CLI's own producer resolves it under the per-OS state directory instead (see the Durable-vs-Ephemeral State Invariant).

- `//go:embed` in the top-level `contracts/stencils` package carries seed defaults only and is never a live read path.
- `internal/stencilstore` is the sole owner of seeding, hash-stamping, edit detection, reading, and validation, and takes a fully resolved absolute base directory from its caller.
- A file whose body hash does not match its stamp is never overwritten.
- The seed/refresh pass runs once per process, either at `cmd/lyx`'s root pre-run in hub mode or at the producer CLI's own pre-run in standalone mode — never lazily inside `stencilstore.Read`.
- The seeding commit is a `board.lock`-holding, positive-pathspec commit through `internal/fabricengine`, never `Bolt` and never a stage-all.
- **Enforced by** `contracts/stencils/registry_test.go` for registry completeness, `internal/stencilstore`'s edit-detection tests, and `internal/lyxcwd/enforcement_test.go` for the vocabulary walk.
  Not reached: `contracts/stencils/stencils.go` is production Go outside `internal/` and `cmd/`, so it falls outside the Go half of the Fabric Vocabulary walk, whose `.md` half does now cover `contracts/stencils/**/*.md`.

## CLI / Cobra Invariant

Every lyx CLI module is a cobra subtree assembled under one root in `cmd/lyx/main.go`.

- **Seam.**
  Each module exposes `Command() *cobra.Command` and `RunCLI(out io.Writer, args []string) int` = `clihelp.Execute(Command(), out, args)`.
  Eleven of the twelve seam modules also carry `RunCLIIn(cwd string, out io.Writer, args []string) int`, which delegates `RunCLI` as `RunCLIIn("", out, args)` — the empty string means "read the process cwd", and any other value seeds `cwd` into the execution context via `clihelp.ExecuteIn`.
  `internal/selfreportcli` is the one seam module without `RunCLIIn`: it references `lyxcwd` nowhere, so a `RunCLIIn` there would accept a cwd argument nothing reads.
- **Alias shape.**
  A module may register an alias command beside its own subtree, as a second root child, via a separately-named exported constructor.
  That alias carries no seam function of its own — no `RunCLI`/`RunCLIIn` — because it delegates entirely into the subtree's own verb.
  `internal/loomcli`'s `RunAliasCommand()`, registered in `cmd/lyx/main.go` as `lyx run` alongside the `lyx loom` subtree, is the first instance of this shape.
- **Registration.**
  A new module is wired into `newRoot()`: import, `root.AddCommand(...)`, and appended to the root `Long` module-list.
  Unregistered ⇒ invisible to `--help`.
- **`Short` on every command** (parent + sub), non-empty.
  Self-discoverable commands also carry a `Long` with concrete examples.
- **Help accuracy is a review obligation.**
  When a change alters observable behaviour, the reviewer must re-check every affected `Short`/`Long`.
- **Errors are JSON**, via the `internal/output` envelope (`output.Ok`/`output.Err`), one JSON object per line, through `clihelp.Execute`/root seam.
  No bare plain-text error paths.
  Parent groups set `RunE = clihelp.GroupRunE`.
- **One envelope per invocation**, and `clihelp.ShouldAbort` is what keeps it that way: `clihelp.Abort` records an exit code but does NOT stop cobra from running `RunE`, so a `RunE` that emits before checking `ShouldAbort` writes a second envelope on top of the one `PersistentPreRunE` already wrote.
  Every `RunE` therefore checks `ShouldAbort` **first**, ahead of its own validation — the placement `clihelp.ShouldAbort`'s own godoc specifies.
  A consumer unmarshalling the output as a single object (which the smoke suites do) fails to parse two, and the second envelope names the secondary problem while the first names the one to fix.
- **Interactive-handoff exception (narrow, per-command).**
  A subcommand that hands stdio to another interactive program and blocks, or self-displays and then blocks forever, is exempt from the envelope only on that terminal-handover/keepalive tail — everything that can fail stays pre-flight, on the envelope.
  `lyx loom status --watch` and `lyx loom run` (alias `lyx run`) are two more registered holders of this exception, in the same shape `internal/reedengine`'s `attach`/`header --blocking` pair already establishes: `status --watch` self-displays the polled status line, then blocks forever as its own keepalive tail, and `run` hands the operator's stdio to a `tmux attach-session` child as its own terminal-handover tail — in both cases every fallible step runs pre-flight, on the envelope, and only the named tail itself is exempt from emitting JSON.
- **Package naming.**
  A cobra-registered package is `<module>cli`;
  its domain kernel is `<module>engine`. cli imports engine;
  engine never imports cli or cobra.
  Litmus: returns `(T, error)` with no cobra/`io.Writer`/exit codes ⇒ engine.
  Skip the engine only for trivial wrappers or a throwaway proof-of-concept meant to be deleted.
  **Named deviation:** `stencilcli`'s domain kernel is `internal/stencilstore`, not `stencilengine` — `internal/stencil` already holds the singular name and the top-level `stencils` package holds the plural, so a `stencilengine` would make three packages one character apart, and `stencilstore` says what the package actually is.
- **Enforced by** `cmd/lyx/drift_test.go` (non-empty `Short` only), `helptree_test.go`, `registration_test.go`, `longlist_test.go`, and `cmd/lyx/seamsignature_test.go`, which pins the `RunCLI(io.Writer, []string) int` seam shape across all twelve modules and the `RunCLIIn(string, io.Writer, []string) int` seam shape across the eleven modules that carry it, both at compile time.

## Shuttle Provider-Seam Invariant

Provider specifics live ONLY under `internal/shuttleengine/claudeengine`.

- `internal/shuttleengine` and `internal/reedengine` stay provider-invariant: they define the `Engine` interface (and, for reed, the opaque `cmd`/`resumeCmd`/strand contract) and never reference Claude specifics.
- `internal/shuttleengine` never imports `internal/shuttleengine/claudeengine` — the reverse import only.
  Wiring a concrete engine happens in `internal/shuttlecli`.
- **Enforced by** `internal/shuttleengine/seam_enforcement_test.go` (`TestProviderSeamImportRule`) for the import-graph half;
  no Claude-specific leakage outside `claudeengine` is a review obligation.

## Shell Mechanics Seam

Pane-shell command strings — argument quoting, the call operator, and the prompt-file read idiom — are built ONLY via `internal/shell`.

- `internal/shell` defines the provider-invariant `Shell` interface (`Quote`/`Invoke`/`ReadFile`) with `pwsh` and `posix` implementations, selected via `shell.ForGOOS()`.
  Stdlib-only, no Claude specifics.
- `internal/shuttleengine/claudeengine` (and any future provider engine) never emits raw pwsh/posix shell syntax directly — only via `internal/shell`.
- **Enforced by** review obligation today (candidate future grep guard).

## Fabric Vocabulary Invariant

**Fabric** (capital F) names the fully wired-up composite — the warp repo with junctions into weft inside it.
Any reader meaning *the repo as a whole* says Fabric.
**warp** and **weft** name the two sides and are used — including in CLI help text and user-visible messages — at exactly those points where the two sides genuinely must be told apart, e.g. `lyx fabric clone <weft-url> [<warp-url>]` and `fabric: warp/weft out of sync`.
"repo" alone is too vague to denote warp and is never a substitute for it.
**`host` is retired** and is never used in any of these senses, anywhere — including inside the owner set below.

The phrase predicate is the sense-discriminator, retained unchanged: `host` is policed via the fabric-sense phrase list (`host repo`, `host repository`, `host worktree`, `host working tree`, `host checkout`, `host branch`, `host junction`, `host path`, `host side`, `host HEAD`, any case, hyphenated or spaced) plus the policed geometry identifiers (`hostBranch`, `hostLayoutFor`, `hostReason`, `HostJunction`, `hostClean`), never as a bare word.
The bare word — the verb sense, the machine/OS sense, and the PowerShell `Write-Host` cmdlet — still passes untouched, because a whole-word ban would rewrite ordinary English in modules with no connection to fabric.
Keep these lists verbatim: they are the ban list, and renaming them would delete the rule.

- **Owner set carves out the bare weft/warp rule only, never the host rule.**
  Owner set: `internal/fabricengine`, `internal/fabriccli`, `internal/weftname`, `internal/gitkit`, `internal/hubforge`, `internal/boardengine`, `internal/configsync` (string literals and comments, never identifiers).
  The narrower `weftname`-import subset is `internal/fabricengine`, `internal/fabriccli`, `internal/gitkit`, and `internal/hubforge`.
  `tools/` and `sandbox/` are not in the owner set — they lie outside the enforcement walk entirely, since the Go walk covers `internal/` and `cmd/` only, so an owner-map row for them would be dead code that never matches.
  Vocabulary in `tools/` and `sandbox/` is a review obligation, not machine-checked.
- **Prose-doc split — review obligation, not machine-checked:** a doc explaining fabric's own mechanism keeps the vocabulary;
  a doc describing a consumer module's behaviour rewords to Fabric or drops the qualifier, because that module does not know weft exists.
  A token scan cannot express this distinction, so it is not covered by the enforcement test.
- This invariant binds every module, template, and doc that talks about fabric — `internal/lyxcwd` is merely one of the packages it binds, not its owner.
  The enforcement test's placement in `internal/lyxcwd/enforcement_test.go` is a file-layout convenience — it reuses that file's `filepath.WalkDir` helper — not an ownership claim.
- **What the machine check does and does not reach — stated honestly, not implying full coverage.**
  Production Go under `internal/` and `cmd/` is machine-guarded, plus an `internal/**/*.md` **and** `contracts/stencils/**/*.md` walk and the embedded agent prompt templates.
  `*_test.go` files are excluded from all three rules.
  `hostGeometryIdentifiers` is five exact lowercased names, so `HostJunctions`, `hostPath`, `hostBare`, `CopyHostHub`, and `HostFixture` are matched only by the phrase half, and only where they occur inside a policed phrase.
  Test files, documentation outside `internal/`, shell, and `tools/` remain a **review obligation**, not a machine check.
- **Enforced by** `internal/lyxcwd/enforcement_test.go` (`TestEnforcement_FabricVocabulary`), covering identifiers, string literals, and comments in production `.go` files under `internal/` and `cmd/`, plus an `internal/**/*.md` **and** `contracts/stencils/**/*.md` walk and the embedded agent prompt templates.
  The host rule is machine-checked everywhere this test reaches, including the owner dirs;
  the prose-doc split above is a review obligation the machine check does not cover.

## Fabric Git Invariant (warp + weft)

Every git operation that LYX/LoomYard's own code performs — on **either** the weft repo or the warp repo — goes through `internal/fabricengine` in Go, in-process, never raw git and never an LLM agent.
This binds LYX's own code only;
a human or any tool outside LYX keeps ordinary git in their warp worktree, untouched.

- **Module ownership.**
  Weft-internal git (`commit`/`push`/`pull`/`sync`) and coordinated warp↔weft topology (checkout, dual-worktree add/remove/clone) both go through `internal/fabricengine`.
  The same holds for warp: no LYX package other than `internal/fabricengine` runs raw git against warp.
  Read-only verbs (current SHA, `git status --porcelain`) are exempt — only *mutating* warp git must dispatch through fabric;
  see `fabric-unified-view.md`'s "Scope boundary" section for the current warp-mutation call sites.
- **Orchestration, not agent.**
  The weft commit is Go calling the engine in-process at a round/phase boundary the loop owner (loom) controls — never an LLM agent, not raw git, not by shelling `lyx fabric`.
  Agents ride the file contract: they **write** overlay files into `_lyx` via the junction — raddle content lives at `_lyx/raddle/` and therefore arrives through this same junction;
  Go **reads and commits** them.
  An agent does commit its own code to the **warp** repo (commit-per-fix) — the weft, never.
  **Board carve-out:** `internal/boardengine`'s writes to `weft:main` are the one exception to timing control — any LLM session, in any worktree, may trigger a board write at any time — but module ownership still holds (board's git flows through `Bolt`, never raw git);
  only the *timing*-control half is scoped away.
- **Cross-module exclusions.**
  The `_lyx` tree is shared by every round-loop module, so every weft-commit caller passes a **positive-only** file list — no `:(exclude)` pathspec magic — built via `fabricengine.ScopedPathspec`.
  Machine-local artifacts (pause flags, fork prompts, module `*.lock` files) live under `.lyx` (see Durable-vs-Ephemeral State Invariant), never reaching a weft-commit pathspec.
  `fabricengine.seedWeftArtifactExcludes` covers the weft repo's never-tracked operational artifacts: fabric's own `.weft/` lock directory, gitrepo's push-lock file, `.lyx/`, and every module's `*.lock`/`*.swaplock` write- and swap-lock files.
  **Known limitation:** does not untrack an artifact a pre-fix sync already committed — `git rm --cached <path>` is the manual remedy.
- **Never-committed routing.** `structuralNeverCommittedDirs` membership makes a path uncommittable, filtered only where the pathspec is constructed (`ScopedPathspec` callers, via `pathspecNames`) — never in `Config.Dirs()`, `WiredNames`, or the slug-reservation union.
  `classifyPaths` routes such a path to a third bucket; `Commit` hard-errors on a non-empty third bucket rather than dropping silently.
  `weftPathspecFilter`'s `git ls-files` probe passes `--exclude-standard`.
- **Junction exclusion** goes through `.git/info/exclude` on both sides (warp: `WireJunctions`; weft: `seedWeftArtifactExcludes`), never a tracked `.gitignore`.
  That file lives in the repo's COMMON gitdir, so it is one repo-wide file, never per-worktree: an exclude entry is removed only once NO warp worktree in the hub still wires a junction of that name (`namesWiredInSiblingWorktrees`).
  Because it is repo-wide, every read-modify-write of it — warp and weft alike — goes through `fabricengine.mutateGitExclude`, which holds a repo-wide flock across read, rewrite and write and replaces the file by same-directory rename.
  No caller may read or write `info/exclude` directly: an unsynchronised `os.ReadFile`/`os.WriteFile` pair loses a sibling worktree's update, and `os.WriteFile`'s truncate-then-write lets a concurrent reader observe an empty file and write that emptiness back, destroying the operator's own exclude patterns along with fabric's junction exclusions.
- **Unwire** removes warp junctions and their warp `.git/info/exclude` entries only — weft-side `_lyx`/`.lyx` content is always preserved.
  Downgrade (a pre-fix binary's `applyStaleRemoval` against this change's output) is unsupported.
- **Enforced by** review obligation: agent prompt templates never mention the two-repo structure at all, per `templates-describe-one-repo` — stronger than merely never instructing a weft git op.
  Never-committed routing: `internal/fabricengine/classify_test.go`, `structuraldirs_test.go`, `internal/fabriccli/cli_test.go`.
  Junction exclusion / unwire: `internal/fabricengine/dotlyxjunction_integration_test.go`, `unwire_test.go`.
  Module ownership is machine-checked for `internal/boardengine` (`cmd/lyx/boardguard_test.go`) and for `internal/websterengine` (`cmd/lyx/rawgitmutation_test.go`, `TestNoRawGitMutation_WebsterProductionSource`);
  every other `fabricengine` caller remains a review obligation.
  The agent half is machine-checked for webster runs by `fabricengine.RefScanner` (a fork or Master Bash command matching a fabric-driving command spelling or the weft sibling worktree path is a hard, round-failing violation) **in hub-mode runs only** — a standalone run supplies `websterengine.NeverMatches` instead,
  since standalone has no weft worktree and no fabric verb for a fork to drive, so there is nothing there for the check to catch;
  a reader must not take the guard as universal across both modes.

## Fabric Destruction Chokepoint Invariant

`internal/fabricengine/destroy.go` is the only file in `package fabricengine` permitted to perform a destructive primitive: `os.RemoveAll`/`os.Remove`, `git worktree remove`, `git branch -D`, `fslink.Remove`, and a warp checkout's `ResetHard`.

- The guard's walk skips `*_test.go`;
  the live-state builders in `package fabricengine_test` are outside this invariant's subject.
- The banned bypass tokens are `RemoveAll(`, `os.Remove(`, `"worktree", "remove"`, `"branch", "-D"`, `warp.ResetHard(`, `weft.ResetHard(`, `fslink.Remove(`, and `createdToken{`.
- Every destructive executor runs the gate's four checks first, always in this fixed order, stopping at the first failure: containment, ownership, dirtiness, force.
  Ahead of the four sits a request-shape check that is not one of them: a `pathRequest` declaring no ownership kind, no dirtiness, or a `dirtinessNA` with an empty reason is refused before containment runs.
  That refusal borrows `CheckOwnership`/`CheckDirtiness` to name the missing declaration, so its `Check` value must not be read as evidence that containment passed.
- **Containment is resolved, not lexical, AND bound to the act.**
  Both the container and the target's ANCESTRY go through `filepath.EvalSymlinks` before they are related;
  the target's own final component stays unresolved, because a junction removal's target is itself a link.
  Comparing nominal paths let a symlink planted at an intermediate segment of a gate target carry a destructive primitive outside its container while the check passed — found and closed in fabric's R2 crucible round.
  `ownedUnderGeometryRoot` resolves the same way, since it is the one ownership kind with no independent resolved-path authority (git's worktree registration, `fslink.RawTarget`) to cross-check against.
  The check alone is not enough: it resolves at one instant, and a symlink dangling at check time then flipped live-and-escaping before the act carried a gated removal outside the container anyway (fabric's R3 crucible round).
  The two arbitrary-path executors (`removePath`, `removeLink`) therefore remove through `removeContainedPath`, an `os.Root` rooted at the gate's container, so component resolution and the unlink are one `openat` chain that atomically refuses any component escaping the container at removal time — binding containment to the act rather than to an earlier resolve. `removeGitWorktree`/`resetHardTo` delegate their act to git, which re-validates at its own instant.
  `removeLaunchers`' launcher-DIRECTORY removal is the third arbitrary-path removal and routes through the same `removeContainedPath` with `recursive` false, never a raw `os.Remove` on the nominal path: it must not use `removePath` (whose directory branch is `RemoveAll` and would destroy operator content beside the launchers), but the non-recursive `os.Root.Remove` refuses a non-empty directory exactly as `os.Remove` does, so it keeps that preservation property while still binding containment to the act (fabric's R8 crucible round).
  The two CREATE-side minters bind creation to a rooted act: `createExclusiveDir` creates its single-component leaf through an `os.Root` rooted at the parent, which refuses a symlink at the leaf it creates (EEXIST) — its parent ancestry is resolved by `os.OpenRoot`, not refused, which is safe only because its sole caller mints the hub as one new component under the operator-chosen clone parent, not inside a live hub; a caller with an attacker-influenced parent ancestry must use the fixed-container rooting pattern instead — and `createGitWorktree` routes through `containedWorktreeAdd`, which stages `git worktree add` at a slug-named leaf inside an unguessable 0700 `os.Root`-created random parent, moves it to the real target with `os.Root.Rename`, and — because `os.Root.Rename` refuses a symlink at the destination and at an intermediate source component but renames a symlink at the source's own final component as a link — verifies fail-closed via `stagedWorktreeContained` that both the staging leaf (after git writes) and the placed target (after the rename) are real directories reached without traversing a symlink, cleaning up and refusing rather than reporting an out-of-hub worktree, before `git worktree repair` fixes git's registration — since `git worktree add` is a subprocess that resolves and follows a symlinked destination argument itself and cannot be rooted directly.
  A same-UID or root planter racing the add can still make git transiently write into a directory it already controls (unpreventable by any staging location), but the add is never reported as success and never leaves the target a dangling out-of-hub symlink.
- `--force` answers dirtiness only.
  It never satisfies containment and never satisfies ownership.
- A gate refusal (`*destructiveRefusal`) is never discarded on a best-effort path — every such site wraps its executor call in `surfaceRefusal` (or, where the call site cannot return an error at all, logs the refusal via `logger.Warn`) rather than swallowing it.
- Every entry on the guard's per-file allowlist carries a reason.
- **Known guard blind spot:** the check is raw substring matching, so an alternative argument-slice spelling with different spacing, a dynamically built argument slice, and aliasing a raw repo handle to a local all evade it, and the allowlist is per-file, so a new raw call added inside an already-allowlisted file is not caught.
  A shared static-analysis-guard framework (issue #135) would close this class of blind spot repo-wide;
  this invariant does not resolve that question.
- The recorder (`rec *Mutations`) is threaded **into** `destroy.go` and must never be worked around by recording at a call site outside it — that is what makes destructive coverage provably total rather than a per-call-site review obligation;
  see the Mutation Record Invariant below.
- **Enforced by** `cmd/lyx/destructiveguard_test.go` (`TestNoDestructiveBypass_FabricengineProductionSource`).

## Fabric Write-Side Containment Invariant

A `package fabricengine` write to a hub-level structural container an attacker can pre-plant a static symlink at — `<hub>/_launchers/…` (`writeLaunchers`) and `<hub>/_portals/…` (`createPortal`) — must route its filesystem write through an `os.Root` rooted at the hub, never a raw `os.MkdirAll`/`os.WriteFile`/`fslink` that resolves and follows the container path itself.

- This is the create-side twin of the Destruction Chokepoint's containment rule.
  `writeLaunchers` wrote `ide.sh`/`fabric-checkout.sh` to `<hub>/_launchers/<AnchorRel>/<slug>` via raw `os.MkdirAll`+`os.WriteFile`, and `createPortal` created its junction via `fslink.CreateDirLink` whose own parent-`mkdir` followed a planted symlink — either one carried the write OUTSIDE the hub while `add` reported `ok:true` with a mutation record naming a hub-relative path (fabric's R7 crucible round, the create-side twin of the delete-side M3).
  Both now write through an `os.Root` at `l.HubPath` (`writeLaunchers` for its files, `createPortal` via `ensureContainedLinkParent` for the link's parent chain), so any component escaping the hub is refused at write time by the kernel's `openat` chain rather than followed.
- The banned raw-write tokens are `os.MkdirAll(`, `os.Mkdir(`, `os.WriteFile(`, `os.Create(`, `os.OpenFile(`, `os.Symlink(`, `os.Link(` — the `os.`-qualified spellings, deliberately not the bare forms, so the rooted `os.Root` method calls (`root.MkdirAll`/`root.WriteFile`) that ARE the write-side chokepoint pass.
- The guard's allowlist covers the raw writes that are NOT in this exploit class, each with a reason: a **git-owned** path resolved by git (`hook.go`'s hooks dir, `gitexclude.go`'s `.git/info`), or a worktree/board directory fabric just minted through a **contained** minter (`createExclusiveDir`/`containedWorktreeAdd`) in the same call (`clone.go`, `warpbinding.go`, `weftgit.go`, `junction.go`'s weft-target materialisation).
  Those are race-only — a post-creation same-UID race, never a static pre-plant, the same accepted residual class as the gate's dirtiness window — because `add.go` refuses a pre-existing worktree path and the minter is fail-closed.
- **Known guard blind spot:** raw substring matching and a per-file allowlist, exactly as the Destruction Chokepoint guard — a new raw write inside an allowlisted file is not caught, and an aliased or dynamically-built write evades it.
- **Enforced by** `cmd/lyx/uncontainedwrite_test.go` (`TestNoUncontainedWrite_FabricengineProductionSource`).

## Mutation Record Invariant

Every mutating fabric verb accumulates a `*Mutations` record of the primitives it actually performed, and every mutating result type exposes that record under a fixed, always-present envelope key set — so a consumer can tell "no error was returned" apart from "something was actually mutated" without parsing prose.

- Every destructive executor in `internal/fabricengine/destroy.go` takes a `rec *Mutations` parameter and appends its own primitive to it, **after** the primitive observably changed state — never on a no-op, never on a refusal, never before the act.
- Every mutating verb's result type embeds `MutationRecord`;
  a read-only verb's result type must not.
  There are exactly two of those, and which verb each serves is not the natural guess: `StatusResult` is the **pairs** verb and `DiffResult` is `diff`.
  The other two read-only verbs (`status`, `list`) return bare slices with no result type, so the guard's companion table has two rows by construction, not by omission.
- `internal/fabricengine/mutation.go` is the single declarer of the `Kind` enum.
  A new member lands in the same commit as its recording site and its guard-test entry, never ahead of either.
- A `CheckForce` member must never be added to `Check`: force is consulted only inside `checkPathDirtiness`, where it makes the dirtiness check *pass* rather than fail, so a refusal can never be attributed to it.
- The envelope's key set is fixed for every envelope emitted from a **verb outcome**: `mutations` is always an array (empty, never `null`) and `partial` always a bool (`false`, never absent), on success and failure alike;
  `partial` derives from exactly one rule, `error ≠ nil ∧ record non-empty`.
  **Pre-flight carve-out:** a handler that fails before calling its verb (cwd/location resolution, `LoadConfig`, an argument `usage: …` error) emits a bare `output.Err` with neither key, because nothing was mutated and there is no result to read a record from.
  Without that clause this rule is machine-quotable and false against the shipped envelope.
- **Enforced by** `cmd/lyx/destructiveguard_test.go`'s `TestMutationRecord_FabricengineProductionSource`, with its blind spots named honestly: it pins the parameter and the embed by raw source inspection, not the correctness of any recording call, and a new `Kind` with no recording site is caught by review, not by the guard.

## Markdown Link Integrity

Every inline markdown link (`[text](target)`) in a `.md` file under `manifest/` or `docs/` resolves — both its file part and, for a `.md` target carrying one, its `#anchor`.

- **The root restriction is source-side only.**
  `manifest/` and `docs/` name which files are *scanned* for outgoing links;
  they do not restrict where those links may *point*.
  Every link target is resolved wherever it lands in the repo, and any `.md` target gets its `#anchor` resolved too, whether that target sits inside `manifest/`/`docs/` or not.
  Reading the root restriction as licence to skip anchor resolution for an out-of-root target would silently un-guard `docs/shared-libs/configengine.md`'s `../../CONSTRAINTS.md#lyxdirs-single-declarer-invariant` link and the `../../internal/*/doc.go` targets this task creates.
- **A file-layout convenience, not an ownership claim.**
  The enforcing test lives in `internal/lyxcwd` (`docslink_test.go`'s `TestEnforcement_MarkdownLinks`), reusing that package's `repoRootForEnforcement` and `walkEnforcementRoots` helpers.
  That placement is a file-layout convenience, not an ownership claim on markdown links by `internal/lyxcwd` — the Cwd Resolution Invariant scopes that package to cwd resolution and nothing else, exactly the caveat the Fabric Vocabulary Invariant above already states for its own test.
- **What the machine check does and does not reach — stated honestly, not implying full coverage.**
  Not reached: external `http`/`https`/`mailto` URLs, never fetched;
  reference-style links (`[text][ref]`) and `<...>` autolinks, out of grammar by decision, not by oversight;
  link-shaped text inside fenced code blocks, deliberately skipped;
  prose mentions of a filename that are not markdown links — `manifest/roadmap.md`'s bare `loom-plan-spec.md` mention (naming it as a structural format spec, not linking it) is a live example this task leaves standing;
  and `.md` files outside `manifest/` and `docs/` as **scan sources**, so `README.md`, `CLAUDE.md`, and `internal/**/*.md` have their own outgoing links checked by nobody.
- **The allowlist's self-expiring contract.**
  Keyed by `(file, target)`, never by line number, with every entry naming its owning task.
  An entry whose key is not matched by any break in a scan is reported as deletable — this covers both a link that was fixed and a keyed file that was renamed or deleted away, since neither case is ever visited by the scan again.
- **Enforced by** `internal/lyxcwd/docslink_test.go` (`TestEnforcement_MarkdownLinks`).

## Review Round Invariant

One review+fix round (burler now, hardener later) follows: A-before-B (review fully written to disk before any target file is touched);
every recorded finding is fixed in B, all severities including LOW/NIT;
no self-grading (round N's fix is judged by round N+1's fresh review, never its own);
commit-per-fix on warp source, never push.
In a cluster round, fork reports, the handler's own holistic review, and the consolidation into one review file are ALL part of A;
fork reviewers are read-only (no writes, no git), mechanically enforced by the fork audit.

- **Enforced by** `internal/burlerengine/template_test.go` (`TestTemplate_StatesRoundDiscipline`, `TestTemplate_StatesClusterForkDiscipline`, `TestTemplate_OrchestratorExcludesDownstreamBodies`).
  No-self-grading and commit-per-fix discipline are review obligations, not machine-checked.

## Live-Substrate Spawn Observability

Any code path that starts a real OS process on behalf of a round/strand/session (a tmux server, a provider session, any subprocess) logs the spawn and its teardown via `internal/logger` — `logger.Info` for normal spawn/teardown events, `logger.Warn` for a retry or a teardown that did not confirm clean.
The durable Info+ trace-file sink captures these regardless of verbosity or env-var configuration (under `go test`, gated by `LYX_TRACE=1`).

- A new spawn point for a live-substrate module must add its own `logger.Info`/`Warn` call in the same change — review obligation, not machine-enforced.
- A spawned pane/child must never re-exec `os.Executable()` while running under `go test`: a Go test binary invoked with positional arguments does not error on them, so the arguments are silently ignored and the full suite runs unfiltered.
  Guarded by `reedengine`'s `headerLaunchLine` (suppresses header re-exec when `testing.Testing()`) and `gitkit.HermeticGitEnv` (`refuseCLIReexec` refuses any test binary invoked with a leading positional argument).
- A retry loop around a real process spawn must cap attempt COUNT, not only elapsed time — a fast-failing spawn burns a time-only budget in far more attempts than it was sized for. `maxBootAttempts` in `internal/reedengine/lifecycle.go` is the pattern: track an attempt counter, exit on whichever of (time, count) is hit first.
- Known instrumented call sites: `internal/reedengine/lifecycle.go`, `internal/shuttleengine/run.go`, `internal/shuttleengine/wait.go`, `internal/burlerengine/engine.go`.
- **A live-substrate module's operational messages all belong on `internal/logger`, not only the spawn and teardown lines themselves.**
  A retry, a probe that could not read the substrate, and a cleanup that skipped are exactly the events an operator goes looking for after the fact, and the stdlib `log` package reaches neither the durable trace file nor a trace correlation id — so a bare `log.Printf` there loses the only record that the unhappy path happened at all.
  `internal/shuttleengine` pins this for itself with a source scan (`run_test.go`'s `TestShuttleengine_LiveSubstrateLoggingGoesThroughLogger`);
  every other live-substrate module remains a review obligation.

## Sandbox Suite Coverage

Every registered lyx module must be exercised by the black-box sandbox suite or be explicitly excluded with a reason.

- **Tagging.**
  A scenario in any suite file (`tools/sandbox/*SUITE.md`) that drives a specific module declares it with a `**Covers:** <module>[, <module>...]` line.
  Coverage is checked at module granularity against the live cobra root (`newRoot().Commands()`, skipping `help`/`completion`).
- **Allowlist.**
  Modules intentionally never sandbox-exercised are named on `excludedModules` with a one-line reason: `ide`, `selfreport` today.
- **Exists ⇒ covered or excluded.**
  A new registered module needs either a `**Covers:**` scenario or a new allowlist entry with a reason.
- **Enforced by** `cmd/lyx/sandbox_coverage_test.go` (`TestSandboxCoverage_AllModulesCoveredOrExcluded`).

## Test Tier Purity Invariant

Untagged test files perform no expensive spawns — no `git init`/`git worktree add`/fixture-tree copies;
Tier 1 stays offline and fast.

- A test file whose first non-empty line is not a `//go:build` constraint mentioning `integration` or `smoke` is "untagged" and must not call `gitexec.Run` (which also matches `gitexec.RunGit`), `exec.Command`/`exec.CommandContext`, `gitkit.Copy*`, or `hubforge.NewHub`.
  Raw substring match — a comment or string-literal mention also trips it.
- Substrate definition (real git/tmux/filesystem/cross-compile/external-binary spawn) lives in `docs/benchmarks/running-tests.md`'s "## The two tiers" section.
- Allowlist: `internal/proc` (its tests must spawn), `cmd/lyx/tierpurity_test.go` itself (carries the banned tokens as test data).
- Additive real-time-wait guard: an untagged file's `time.Sleep(...)` with a compile-time-constant duration ≥ 1s is flagged unless allowlisted (`allowedLongSleepers` in `cmd/lyx/tiersleep_test.go`);
  an unresolvable duration expression is conservatively flagged too.
- **Enforced by** `cmd/lyx/tierpurity_test.go` (`TestTierPurity_UntaggedTestsSpawnNothing`).

## Hermetic Git Test Environment Invariant

Every test package whose tests spawn git — directly or via a `gitkit`/`hubforge` fixture helper — runs under the hermetic git test environment, so no test behaviour depends on the operator's `~/.gitconfig` or the system gitconfig.

- A package is "git-spawning" when any `*_test.go` file spawns git directly (`gitexec.Run`, which also matches `gitexec.RunGit`, or `exec.Command`/`exec.CommandContext`) or indirectly via a fixture helper (`gitkit.Copy*`, `gitkit.MustRun`, `gitkit.SeedConfig`, `hubforge.NewHub`).
  Every such package must have a `TestMain` calling `gitkit.HermeticGitEnv()` before `m.Run()`, or be allowlisted.
- Allowlist: `internal/proc` (spawns non-git processes).
- **Enforced by** `cmd/lyx/hermeticenv_test.go` (`TestHermeticGitEnv_GitSpawningPackagesHaveTestMain`) — proves presence of the call only;
  a real, correctly-ordered `TestMain` is a review obligation.

## Dev/Prod Binary Separation

The sandbox tooling resolves the dev binary from the derived `.dev-bin` (falling back to PATH) through `resolveLyx`, never a bare-PATH `lyx` lookup that could silently resolve prod.

- `resolveLyx` (`tools/sandbox/resolve.go`) is the single allowlisted resolution site: checks `.dev-bin/lyx` first, falls back to `lookPath("lyx")`.
  Covers both `lookPath("lyx")` and the separator-free `exec.Command("lyx", …)`/`exec.CommandContext("lyx", …)` form.
- The dev binary (`tools/deploy -dev`) builds into `<repoRoot>/.dev-bin` (gitignored), never the production install location.
- `.dev-bin` is prepended only to the agent child-process PATH (`launchAgent`), never the operator's own PATH.
- **Enforced by** `tools/sandbox/pathresolve_guard_test.go` (`TestPathResolveGuard_NoBarePathLyxOutsideResolve`) for the mechanical half;
  agent-only PATH prepend and never-installed-to-prod are review obligations.

## Planparser Sole-Parser Invariant

`internal/planparser` is the SOLE parser of the on-disk plan format (`_lyx/plan/`).

- No other package parses `00-overview.md`/`NN-<card-slug>.md`;
  consumers read plan-level sections only from the `planparser.Plan` model a caller hands in.
- `planparser` is the sole declarer of the plan directory's path.
  `PlanDirName`/`PlanDirRel()` declare the worktree-relative token, and `PlanDir`/`PlanOverview` declare the absolute form.
  The package never resolves cwd and never imports `internal/lyxcwd`;
  the caller supplies the anchor path — `AnchorPath()`, never `WorktreePath()`.
- **Enforced by** review obligation today (candidate future import/grep guard).

## Discussionparser Sole-Parser Invariant

`internal/discussionparser` is the sole reader of `_lyx/discussion/`'s on-disk format, and no other package parses `decision-record.md`'s or `support-log.md`'s section shape.

- It declares no on-disk location of its own — `loomengine`'s `DiscussionDecisionRecord` / `DiscussionSupportLog` remain the sole declarers of where `_lyx/discussion/` is, deliberately unlike `planparser`, because those accessors take a `*lyxcwd.Location` a stdlib-only leaf may not import.
- It imports the standard library and nothing else.
- **Enforced by** `internal/discussionparser/leaf_enforcement_test.go` (`TestLeafInvariant_AllowlistOnly`).

## Gate Self-Check Parity Invariant

A mechanical gate's `ShedProducer` row and its CLI self-check verb call the same package function, and neither re-implements the other's check.

- Today's two instances: Discussion-Validate's `ShedProducer` row and the `validate-discussion` verb both call `discussionparser.Validate`;
  Plan-Validate's `ShedProducer` row and the `validate-plan` verb both call `planparser.Validate`.
- The verb's envelope distinguishes a findings failure from an I/O fault **structurally**, by the presence of the `findings` key, never by message wording, because that is what the three-way comparison keys off.
- Adding a mechanical gate means adding its verb and its parity test in the same task.
- **Enforced by** `internal/loomcli/parity_test.go` (`TestGateParity_DiscussionValidate`, `TestGateParity_PlanValidate`).

## Recipe-Format Sole-Parser Invariant

`internal/shedbuild` is the SOLE parser of the recipe file format.

- No other package decodes a recipe document;
  every consumer reaches a recipe only through the `shedbuild.Recipe` model `Parse`/`Load` returns.
- `internal/shedbuild` declares no on-disk location for recipe files — no directory constant, no filename convention, no embedded default — which its own package doc already asserts,
  so a shipped recipe's location is its owning product's to declare, as `contracts/recipes` does for loom.
- `internal/shedbuild` reaches the engine registry only through `shedrecipe.Lookup`/`Names` and adds no registration mechanism of its own — see the Shed Recipe Registry Invariant above, which this bullet cross-references rather than restates.
- **Enforced by** review obligation today (candidate future import/grep guard).

## Batcher Registry+Config Invariant

webster's execution unit is the batchifier-derived batch, not the raw plan card — batching is a standalone step webster consumes, not webster's own execution-policy decision.

- Batching is selected by `internal/batcher`'s name-keyed registry plus `batcher.yaml`'s `active:` config key (default `identity`), owned by `internal/batcher` rather than by webster — no plan-supplied batching, no batch grouping in the plan format itself.
- **Enforced by** review obligation.

## Producer Pointer-Rule Invariant

An instruction file — a producer's own prompt or skill — must never duplicate or paraphrase another producer's format-contract content, only point at it, so that editing that one format-contract file alone is sufficient to change what both its producer and its consumers do.

- Binds **instruction files** (agent prompts and skills) and format-contract docs, not Go source, and not design docs restating the rule for a human reader.
- **Enforced by** review obligation.

## Config Strictness Invariant

`internal/configengine` offers two loading policies and a caller adopts exactly one.
`Load` is strict: an absent `_lyx/` directory or an absent config file is an error. `LoadOrTemplate` degrades: either absence resolves the caller's embedded template instead.

- **The membership rule**, stated as a predicate a future caller can apply rather than a bare list: a module belongs on the degrading side when it has, or is slated to have, a **standalone entry point** — a way to be invoked outside a lyx hub — because a config-less invocation is then a supported mode.
  A module that only ever runs inside a hub stays strict, because there an absent config means the hub is broken.
- **The two pinned sets** as they stand today: degrading is `{shuttleengine, reedengine, websterengine, batcher}`;
  strict is `{fabricengine, boardengine, loomengine}`.
- **A third class, explicitly outside this invariant's guard subject: own-loader modules.**
  These never call either entry point — they resolve the path with `configengine.ConfigFile` and read the file themselves with their own absent-file fallback.
  `internal/burlerengine` (`burler.yaml`, absent file returns a zero `Config`, bypassing `Load` because `MissingKeys` would misfire on its open-ended lenses/fans key set) and `internal/modelspec` (`models.yaml`, absent file returns `builtins()`;
  it cannot call a logging `Load` at all, being capped by the Modelspec Leaf Invariant) already have the degrading behaviour and are deliberately not repointed.
  A set-equality grep over the two entry-point tokens is structurally blind to them — without this clause the invariant would read as though the two pinned sets enumerate every module config in the repo, which they do not.
- **Absence is typed, not textual.** `FindBaseDir` wraps the exported `configengine.ErrNotInitialized` sentinel on its absent-`_lyx/` branch and deliberately does not wrap it on a stat failure, so a degrading caller falls back only on `errors.Is(err, ErrNotInitialized)`.
  The four strict callers still use the older `strings.Contains(err.Error(), "not initialized")` rewrap;
  the sentinel makes migrating them possible, but the migration is available rather than done.
- **A watch item that has fired:** `batcher` used to sit on the strict side because it had no standalone entry of its own, even though its config is read on webster's batching path.
  The websterengine + webstercli told-geometry, and Webster standalone entry task gave webster a standalone entry point, so a standalone Webster now reaches `batcher.Active` on every verb outside a hub, where `_lyx/` does not exist;
  that task moved `batcher` to the degrading side and the pinned sets above already reflect the move.
- **Known guard blind spot:** a substring scan cannot see a call reached through an alias or a function value.
- **Enforced by** `cmd/lyx/configstrictness_test.go` (`TestConfigStrictness_PinnedCallSiteSets`).

## GitHub Auth Invariant

All GitHub authentication goes through `internal/githubclient`;
no other production package shells out to `gh`.

- Token resolution, token caching, and construction of an authenticated `*github.Client` live solely in `internal/githubclient`.
  No other production package invokes `gh` (via `exec.Command`/`exec.CommandContext` or bare `LookPath("gh")`) or otherwise builds its own GitHub credential path.
- `internal/githubclient` production imports are allowlisted to stdlib, `go-github`, `golang.org/x/sys`, and `internal/proc`.
- **Enforced by** `cmd/lyx/ghguard_test.go` (`TestGHGuard_NoShellOutOutsideGithubclient`) and `internal/githubclient/leaf_enforcement_test.go` (`TestLeafInvariant_AllowlistOnly`).

## gitrepo Client Boundary Invariant

`internal/gitrepo` splits local-vs-remote by client: go-git owns local object and ref access, `gitexec` owns anything that authenticates to a remote or mutates the working tree.

- go-git handles reads that resolve state already on disk — commit/tree/blob lookups and ref reads. `gitexec` is the only path to the git CLI, used for `StageAndCommit`, `CommitEmpty`, `StageAllAndCommit`, `Push`, `PushCoalesced`, `PushRebaseFree`, `Pull`, `Fetch`, `ResetHard`, `CheckoutDetached`, `RestoreBranch`, `IsAncestor`, `HasUnpushed`, `MergeStart`, `MergeConclude`, `ConflictedFiles`, `MergeHeadPresent`, `MergeHeads`, `MergeFFOnly`, `StageResolved`.
  Any new `gitexec` call added inside `internal/gitrepo` must update this list in the same commit.
- The guard's pinned method set is keyed on `r.run` and `r.runChecked` together — whichever chokepoint a method's body calls, it belongs on the same list;
  the raw/checked split within that set is invisible to this guard by design (see the gitexec Checked-Call Invariant below, which is keyed by call site instead).
- The guard separately asserts exactly two `gitexec.Run`/`gitexec.RunGit` call expressions exist in the package's non-test source: one inside `run`'s own body, one inside `runChecked`'s.
- Known guard blind spot: the method-name check is set-equality, so a new `r.run`/`r.runChecked` call slipped inside an already-pinned method is not caught — per-call review still applies to those methods.
- **See also:** this invariant answers *which methods may reach the git CLI at all*, keyed by method name;
  the gitexec Checked-Call Invariant below answers *which call sites may use the raw form*, keyed by call site.
  A new CLI call added inside an already-pinned method trips the Checked-Call Invariant and not this one;
  a new method reaching the CLI trips both.
- **Enforced by** `cmd/lyx/gitrepoboundary_test.go` (`TestGitrepoBoundary_PinnedRunCallSites`).

## gitexec Checked-Call Invariant

`gitexec.Run`/`runChecked` is the default entry point;
`gitexec.RunGit`/`r.run` (the raw forms) survive only at a pinned set of call sites, each carrying an adjacent `//gitexec:raw` marker.

- Every remaining raw `gitexec.RunGit` or `r.run` call site in non-test source carries an adjacent `//gitexec:raw — <why the raw form is correct here>` marker, on the same line or the line immediately above.
  The justification must be true: the two truthfully-markable classes are (1) a pure predicate whose signature has no error channel to report the exec path through, and (2) a test-pinned, deliberate-suppression contract.
- Per-package pinned raw-site counts: `internal/gitrepo` 3 (`run`'s own body, `Pull`, `Fetch`), `internal/fabricengine` 2 (`weftRepoExists`, `weftBranchExists`), `internal/lyxcwd` 0, `internal/fabriccli` 0, `internal/websterengine` 0.
  A package with no entry here is pinned zero.
- Test files are exempt from the marker requirement entirely.
- Known guard blind spot: a raw-substring scan, not an AST walk — it cannot see a raw call written in a spelling its two literal tokens miss, and it cannot tell a genuinely new raw call sitting near an already-`//gitexec:raw`-marked region from the one call the marker was actually written to justify;
  per-call review remains necessary there.
- **See also:** this invariant answers *which call sites may use the raw form*, keyed by call site;
  the gitrepo Client Boundary Invariant above answers *which methods may reach the git CLI at all*, keyed by method name.
- **Enforced by** `cmd/lyx/checkedcall_test.go` (`TestCheckedCallInvariant_RawSitesMarkedAndPinned`).

## Never Force-Add Invariant

Fabric/gitrepo never runs `git add -f`.

Transients are kept out of the index by each repo's own `.git/info/exclude` (warp: `seedGitExclude`;
weft: `seedWeftArtifactExcludes`), never by force-adding past them and never by per-call `:(exclude)` pathspec magic.

This is enforced structurally — `gitrepo.StageAndCommit` has no `-f` code path at all — plus a machine-checked grep guard against its reintroduction.

- **Enforced by** `internal/gitrepo/noforceadd_test.go` (`TestNoForceAdd_GitrepoSourceHasNoForceAddBranch`).

## Documentation Lifecycle

Which docs are kept vs deleted (mechanical per-module docs vs durable design docs): see [docs/overview.md#documentation-lifecycle](docs/overview.md#documentation-lifecycle).


## Source-grounding rule

Never fabricate file contents or code behaviour you have not actually read.
Do not infer from filenames or positions.

## Criteria (apply briefly to each)

- **Undecided items** — TBDs, unresolved options, multiple alternatives without a choice.
- **Scope** — what's in/out;
  could a plan writer disagree?
- **Constraint coverage** — CONSTRAINTS.md items acknowledged;
  implicit perf/compat constraints stated.
- **Tooling/validator claims** — any testing-plan claim about tooling, validator, or command-prefix requirements (e.g. `PYTHONPATH=`) must be cross-checked against CLAUDE.md and the actual enforcement (e.g. `_plan_validate.py`); a contradiction is `[BLOCKING:consistency]`.
- **Failure modes** — empty states, concurrency, invalid input, partial failures addressed.
- **Testing** — strategy named (unit/integration/e2e);
  absence or non-commital language flagged.
- **Ambiguity** — requirements needing interpretation ("fast", "handle errors").
- **Feasibility** — technical obstacles not addressed, based on source files read.
- **Decisions** — each `### Decision:` has rationale + rejected alternatives;
  implicit decisions surfaced.

Independently state, in the `reviewer_self_id:` field below, what model/version you believe yourself to be — this is your own best-effort assessment, distinct from the `reviewer_model:` value already dictated to you above.

## Output format — STRICT

Wrap your entire output in `MILL_REVIEW_BEGIN` / `MILL_REVIEW_END` markers, each on its own line.
Everything outside these markers is ignored by the backend.
**No preamble inside the markers.**
No "I reviewed..." sentences.
No narrative intro.

Per finding: 3–5 lines total, short and factual.
The consumer has full context of the discussion;
do NOT explain background.
Cite the section, state what's wrong, propose the fix.

Target length: ~300 tokens for APPROVE (just verdict + brief summary), ~600–900 tokens for REQUEST_CHANGES (one finding block per issue).
If you produce more than ~1200 tokens, you are being verbose — compress.

```
MILL_REVIEW_BEGIN
# Review: loom: Plan-Write/Plan-Validate approval deadlock (F7)

```yaml
verdict: APPROVE | REQUEST_CHANGES
reviewer_model: opusmedium
reviewer_self_id: <your own model self-identification, if known>
reviewed_file: <artefact reference>
date: <UTC YYYY-MM-DD>
```

## Findings

### [BLOCKING:design] <short title, <60 chars>
**Section:** <§ or heading> **Issue:** <one sentence — what's missing or ambiguous> **Fix:** <one sentence — what to clarify or add>

### [NIT:scope] <short title>
**Section:** <§> **Issue:** <one sentence> **Fix:** <one sentence>

## Verdict

<APPROVE | REQUEST_CHANGES>
<one sentence — max 20 words>
MILL_REVIEW_END
```

Severity rules:
- `BLOCKING` — must resolve before plan writing can proceed.
- `NIT` — record but do not block.

**Severity vocabulary is closed.** Use ONLY `BLOCKING` or `NIT` as the bracketed label in a finding heading -- never invent another word. If a finding's severity feels ambiguous, default to `BLOCKING`, never `NIT`.

Verdict rules:
- `APPROVE` — zero BLOCKINGs. NITs fine.
- `REQUEST_CHANGES` — one or more BLOCKINGs.

**Class is the second axis, encoded in the same bracket as severity, colon-separated, lowercase: `### [BLOCKING:design] <title>`.**
A finding with no class, or a class outside the four names below, is a reviewer defect.
The four recognised classes, identical in meaning across every review stage:

- `design` — a decision is missing, wrong, or rests on a false premise.
  Example: the discussion never says which of two incompatible caching strategies the plan should use.
- `scope` — the work inventory is incomplete, or the enumeration method is unreliable.
  Example: the discussion names three affected modules but the source tree shows a fourth with the same pattern.
- `decision` — a named artifact with no stated disposition.
  Example: the discussion references a legacy config key it never says whether to keep, migrate, or delete.
- `consistency` — the artefact contradicts itself, carries a superseded statement, or violates an established repo convention.
  Example: the discussion's constraints section says "no new dependencies" while a later section proposes adding one.

**Class governs who decides and when the loop stops, never whether a finding gets fixed.**

Omit the `## Findings` section entirely if there are zero findings. Never invent findings to pad the review.

## Out of scope for this stage

- Call-site enumeration and compile-breakage enumeration belong to the build and to code review, not to discussion review.
- An unreliable enumeration method is ONE `design` finding about the method itself, never N `scope` findings naming individual files.


---

## Output contract

Write your full report to this file: /home/knatte/Code/loomyard/wts/loom-plan-approval-gate/_mill/briefs/review-discussion-holistic-r5.out.md

Any format the prompt above asks for (including a `MILL_REVIEW_BEGIN` / `MILL_REVIEW_END` wrapped report) is the content of /home/knatte/Code/loomyard/wts/loom-plan-approval-gate/_mill/briefs/review-discussion-holistic-r5.out.md -- write it there, not into chat.

Your final chat message must be exactly one line and nothing else: `WROTE /home/knatte/Code/loomyard/wts/loom-plan-approval-gate/_mill/briefs/review-discussion-holistic-r5.out.md`
