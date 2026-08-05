# Discussion: fabric: shrink hubgeometry to the minimal illusion primitive (slice 7)

```yaml
task: 'fabric: shrink hubgeometry to the minimal illusion primitive (slice 7)'
slug: fabric-illusion-core
status: discussing
parent: main
```

## Problem

`internal/hubgeometry` (591 lines in `hubgeometry.go`, plus `anchor.go` 53 and `worktreelist.go` 78) is a path authority for the whole codebase. Roughly 30 of its exported symbols are either (a) Fabric's own weft/junction/portal/launcher plumbing that leaked into a package every module imports, or (b) ~20 per-module path constructors (`PlanDir`, `BuilderDir`, `WebsterDir`, `DiscussionDir`, `PerchRunsDir`, …) that make the module a bottleneck: a module cannot add its own subdirectory under `_lyx` without editing `hubgeometry`. That bottleneck is exactly what GitHub issue #127 complained about.

Why now: slices 1–6 already moved the two things that justified the central authority. Slice 1 made the junction name-set config-driven (`fabric.yaml`'s `pathspec`, read from `<Hub>/_board/_lyx/config/fabric.yaml`), and slice 5 made the anchor a recorded marker on `weft:main`. Fabric now owns the geometry; `hubgeometry` is a leftover middleman. `CONSTRAINTS.md:5-18` already carries the *replacement* invariant text (committed in `8cb07070`) describing the narrow post-slice-7 contract, while the code and `internal/hubgeometry/enforcement_test.go` still implement the old one — the documentation is ahead of the code, and this task closes that gap. Slices 8, 9 and 10 all depend on this one.

## Scope

**In:**

- Rename `internal/hubgeometry` → `internal/lyxcwd`, and its `Layout` type → `Location`.
- Shrink the module to exactly three operations: `Getwd()`, `Resolve(cwd)`/`ResolveWithAnchor(cwd, anchor)`/`ResolveWorktree(root)`, and the `Location` coordinates it returns.
- Replace the `Layout{Cwd, WorktreeRoot, Hub, RelPath, Prime, Repo}` struct with `Location{RepoName, HubPath, WorktreeName, AnchorRel}` plus derived `WorktreePath()` / `AnchorPath()` accessors.
- Tighten the cwd gate from "at or below the anchor" to **strict equality** with `AnchorPath()`, with a diagnostic error message.
- Rename the anchor marker file `.fabric-anchor` → `.lyx-anchor`, no compatibility fallback, plus a loud stale-marker error.
- Move ~30 exported symbols out to their owning modules (full destination map under Decisions).
- Move `ConfigDir`/`ConfigFile`/`DotEnv` to `internal/configengine`.
- Fix the seven non-`fabric*` production files that call `Weft*`/`Host*Link` methods directly (slice 8's mechanical half, pulled forward because slice 7 makes those methods private).
- Export `BoardDir` from `internal/fabricengine`; keep a private `boardDir()` inside `internal/lyxcwd` for the anchor read.
- Wire the cwd→`_board` junction explicitly in `fabricengine`, outside the `pathspec`/warp-weft-mirror machinery.
- Extract `internal/weftname` — a stdlib-only leaf owning the `-weft` naming convention, imported by both `fabricengine` and `lyxtest`.
- Delete `SiblingLayout` outright; `fabricengine/hostlayout.go` constructs the sibling `Location` inline.
- Delete the four pattern-specific weft/host accessors (`WeftPatternDir`, `WeftPatternDirFor`, `HostPatternLink`, `HostPatternLinkHere`) rather than moving them — PATTERN must not know weft exists.
- Rewrite `enforcement_test.go` from a single-package allowlist to a per-token ownership map.
- Widen the leaf-invariant allowlists for `lyxtest`, `modelspec`, `tokenvocab`; update all six leaf/seam invariants that name `internal/hubgeometry`.
- Update `CONSTRAINTS.md`, `docs/overview.md`, `manifest/designs/fabric-unified-view.md`, and `docs/shared-libs/hubgeometry.md`.

**Out:**

- Slice 8's open policy question — whether `buildercli`/`perchcli`/`webstercli` CLI output should ever say "weft" to the end user. This task only makes those call sites stop querying `Layout` for weft paths; the wording decision stays in slice 8.
- Slice 9's `.lyx` hygiene work: relocating transients (`_lyx` → `.lyx`), registering `.lyx` as a pathspec junction, removing `crossModuleMachineLocalExcludes`.
- Slice 10's warp-URL binding.
- Moving `Resolve` itself into `fabricengine`. Rejected this slice — see the "resolution-stays-below-fabric" decision.
- Renaming `_board`. Considered 2026-08-05 (`_lyxharness`, `_system`, `_registry`) and dropped.
- Any change to warp git behaviour. Warp stays ordinary git.

## Decisions

### module-name-and-type

- Decision: rename `internal/hubgeometry` → `internal/lyxcwd`; rename the `Layout` type → `Location`. Call sites read `lyxcwd.Getwd()`, `lyxcwd.Resolve(cwd) (*lyxcwd.Location, error)`, `loc.HubPath`, `loc.AnchorPath()`, `lyxcwd.ErrCwdOutsideAnchor`.
- Rationale: after the shrink the module does not do geometry at all — constructing paths from structural tokens is precisely what it stops doing, and that work moves wholesale to Fabric, which becomes the only geometry owner. What remains is an entry gate: it converts "the process started somewhere" into "these are the coordinates of a legal lyx worktree, or here is why this is not one." `lyxcwd` names that job literally, and it is the module that owns cwd — the `os.Getwd` ban is the reason it must exist at all.
- Rejected: `internal/geometry` (describes the one thing it stopped doing; Fabric owns geometry now). `internal/anchor` (collides with `AnchorRel`/`AnchorPath()`, and the module does more than store an anchor). `internal/worktreeid` (`fabricengine` is the package that actually manages worktrees). `internal/lyxroot` (the design doc explicitly warns against overloading "root"). Keeping `hubgeometry` (zero churn, but advertises a role the module no longer has — the exact confusion this slice exists to end).

### location-struct

- Decision: the struct holds only irreducible facts, ordered hierarchically outermost-first, with everything derivable exposed as a method:

```go
type Location struct {
    RepoName     string // TrimSuffix(Base(HubPath), "-HUB")
    HubPath      string // container dir holding every worktree of this repo
    WorktreeName string // this worktree's dir name within HubPath
    AnchorRel    string // anchored subpath within the worktree ("." when unanchored)
}

func (l *Location) WorktreePath() string // filepath.Join(HubPath, WorktreeName)
func (l *Location) AnchorPath() string   // filepath.Join(WorktreePath(), AnchorRel)
```

- Rationale: every field name that denotes a path now says `Path`; `RepoName`/`WorktreeName` say `Name` because they are names, not paths. `RepoName` comes first because a repo is the outermost identity — a repo has hubs, a hub has worktrees, a worktree has an anchored subpath. The worktree is a direct child of the hub by construction (`hub := filepath.Dir(worktreeRoot)` today, and `WorktreePath(slug) = filepath.Join(l.Hub, slug)` already assumes it), so storing the full worktree path alongside `HubPath` would be redundant storage of a derivable quantity.
- Rejected: keeping `WorktreeRoot` as a stored field (redundant, and "root" was the naming defect being fixed). Keeping `Repo` as a bare name (it is a name — say so). Keeping `Prime` (see `prime-and-list-move`).

### cwd-is-not-a-field

- Decision: `Cwd` stops being a struct field. It remains the *parameter* to `Resolve(cwd)`, but is not stored on `Location`. All ~25 production sites reading `layout.Cwd` become `loc.AnchorPath()`.
- Rationale: lyx is valid only when invoked from the anchored directory, because only there are the warp→weft junctions wired and only there does `_lyx` exist. Under that contract cwd is provably equal to `AnchorPath()` after a successful `Resolve`, so storing it duplicates a derivable value. The field is also already dishonest in two of the three constructors: `SiblingLayout` sets `Cwd` to *another worktree's* root (`hubgeometry.go:179`), and `ResolveWorktree` sets it to the worktree root it was handed — neither is a process working directory. cwd must still be the *input*, because it is the only thing the process knows at startup and `git rev-parse` has to run somewhere.
- Rejected: keeping `Cwd` as a field for the gate (the gate runs inside `Resolve`, which has the parameter). Storing a depth-from-anchor instead (obfuscation with no benefit).

### strict-anchor-gate

- Decision: `Resolve` hard-errors with `ErrCwdOutsideAnchor` unless `cwd == AnchorPath()` exactly. The message names both sides, e.g. `lyx must be run from the anchored directory <AnchorPath>` (recorded in `_board/.lyx-anchor`); `cwd is <cwd>`.
- Rationale: today `hubgeometry.go:121-127` rejects only cwd *outside* the anchor subtree — `filepath.Rel(anchorAbs, cwd)` returning `internal/foo` passes. So cwd may sit arbitrarily deeper than the anchor, and lyx then survives `Resolve` and dies further downstream at `configengine.FindBaseDir` with `not initialized: _lyx/ directory not found` (`configengine/config.go:23` checks `<baseDir>/_lyx`). Strict equality turns a confusing late failure into an immediate, accurate one. The error is a truth about the filesystem, not a policy: outside the anchored directory the junctions are not wired, so `_lyx` genuinely is not there.
- Rejected: keeping the at-or-below gate (leaves the latent subdirectory bug). Auto-walking up from a subdirectory to the anchor (silently changes which directory a command acts on — the failure mode `hubgeometry` was built to prevent).
- Comparison semantics: "exactly" means both sides are passed through `filepath.EvalSymlinks` and then `filepath.Clean` before comparison; the compare is byte-exact on Linux/macOS and case-folded (`strings.EqualFold`) on Windows. If `EvalSymlinks` fails on either side (path does not exist yet), fall back to `Clean`-only on that side rather than erroring. This must be a named helper in `internal/lyxcwd`, not an inline `==`.
- Rationale for the semantics: the worktree side comes from `git rev-parse --show-toplevel` (`hubgeometry.go:103-112`) while cwd comes from `os.Getwd`, and the two disagree in three routine cases — macOS `/tmp` is a symlink to `/private/tmp`, `lyxtest` fixtures live under symlinked temp dirs, and Windows/macOS filesystems are case-insensitive while Go string compare is not. Without normalization the strict gate would reject correct invocations on developer machines, and it now gates *every* invocation including unanchored repos that had no gate at all before.
- Note for the implementer: this is stricter for unanchored repos too. With `AnchorRel = "."` (see `anchor-naming`), lyx is accepted only at the worktree root, never in a subdirectory. That is intended, and it is a user-visible behaviour change worth calling out in the commit message.

### anchor-naming

- Decision: `RelPath` → `AnchorRel`. When no marker is recorded, `AnchorRel` falls back to `"."`, not to the cwd-derived relative path. "Anchor" becomes the vocabulary throughout — field names, doc comments, error text, and the marker filename.
- Rationale: the field holds the recorded anchor subpath, and the codebase already says "anchor" everywhere else (`.fabric-anchor`, `ErrCwdOutsideAnchor`, `fabricengine/doc.go:110` calls it "the lyx-anchor subpath"), so `RelPath` was the odd one out. The old fallback (`filepath.Rel(worktreeRoot, cwd)`, `hubgeometry.go:116`) would make the new name a lie, and it is dubious on its own terms: it makes `_lyx` resolve to a different place depending on where the user happened to stand.
- Rejected: keeping `RelPath` with an anchor-sourced doc comment (leaves the vocabulary split). `AnchorRel` with the cwd-derived fallback retained (name over-promises exactly where it matters least).

### marker-file-rename

- Decision: `.fabric-anchor` → `.lyx-anchor`, with **no** compatibility fallback read. `fabricengine` additionally detects a leftover `.fabric-anchor` in `_board` and hard-errors with a "re-clone required" message.
- Rationale: the marker anchors the whole weft repo, not the fabric module specifically — `fabricengine/doc.go:110` already calls the value "the lyx-anchor subpath". Surface is small: sole writer is `fabricengine/clone.go:175`, readers are `anchor.go:43` and the adopt-path re-read at `clone.go:152`. Without the stale-marker detection the break would be silent rather than loud: an old clone would simply fall back to `AnchorRel = "."`, which for a subpath-anchored repo resolves `_lyx` to the wrong place.
- Rejected: a read-only fallback to `.fabric-anchor` (carries the old name forward into a later slice). `.anchor` (too generic for a file in a shared checkout). Keeping `.fabric-anchor` (leaves the misnomer).

### anchor-read-ownership

- Decision: `internal/lyxcwd` is the sole reader of the `.lyx-anchor` file format. It exposes the anchor read plus a `ResolveWithAnchor(cwd, anchor)` variant alongside `Resolve(cwd)`. `fabricengine`'s clone drops its own `os.ReadFile` (`clone.go:152`) and calls the module's exported read for the adopt check, then passes the resolved anchor into `ResolveWithAnchor` for the post-clone layout. `lyxtest` uses the same override instead of writing marker files into synthetic hubs.
- Rationale: today `fabricengine/clone.go:152` reads the marker itself, so `hubgeometry` is *not* the sole reader; leaving that in place would move the duplication rather than remove it. The override also removes clone's read-then-resolve ordering dependency and gives `lyxtest` a seam.
- Clone ordering fact for the implementer: the marker is never read from GitHub. `CloneHub` (`fabricengine/clone.go:79`) clones the weft repo to local disk at step 6 (`:120`), materializes `<Hub>/_board` as a second weft worktree checked out on `main` at step 7 (`:137`), and only then reads the marker at step 8 (`:146-178`). Steps 1–7 have no anchor available locally and cwd is outside any hub, so `Resolve` must not be called during that window.
- Correction to that ordering, and a required fix: today's code **does** call it. `clone.go:112` calls `hubgeometry.Resolve(hostWorktreePath)` at step 5 to install the post-checkout hook — before any marker exists, and from a cwd that is not `hostWorktreePath`. Under the strict gate that call fails every time. Replace it with a direct `Location` construction: `HubPath` = the clone's hub path, `WorktreeName` = `filepath.Base(hostWorktreePath)`, `AnchorRel` = `"."` — no gate, no marker read. The hook install needs a path, not a resolution. Keep the existing non-fatal error handling around it.
- Rejected: no override, clone keeps calling `Resolve(primeCwd)` after writing the marker (keeps the ordering dependency, no `lyxtest` seam). `fabricengine` owning the file entirely and always injecting (cleanest layering, but `logger/sink.go` resolves with no fabric involvement and would silently get `AnchorRel = "."`).

### resolution-stays-below-fabric

- Decision: cwd resolution stays in `internal/lyxcwd`, documented as an infrastructure exception to "every module asks Fabric" — the module sits *below* `fabricengine` in the dependency graph, not beside it. `internal/logger` keeps calling it directly.
- Rationale: verified import cycle, not an inherited assumption. `internal/logger/sink.go:74,79` calls `hubgeometry.Getwd()` then `hubgeometry.Resolve(cwd)` to place its trace file, and `internal/fabricengine/coalesce.go:18` and `spawn.go:19` import `internal/logger`. Moving the resolver into `fabricengine` produces `fabricengine → logger → fabricengine`. Keeping the module stdlib + `internal/gitexec` only is what holds that cycle closed.
- Rejected: moving resolution into `fabricengine` and injecting the log directory into `logger` from `cmd/lyx/main.go` — a cleaner end state that eliminates the module entirely, but it pulls `logger` initialization rework into an already 24-package slice, and `logger`'s lazy resolve exists so early-boot logging works before explicit setup. Record this in the design doc as the intended follow-up. A private duplicate resolver inside `logger` is rejected outright: two resolvers is the defect this module prevents.

### config-path-move

- Decision: `ConfigDir`, `ConfigFile` and `DotEnv` move to `internal/configengine`, exported.
- Rationale: `configengine` is already the consumer and owns everything around these paths — `configengine/config.go:42` calls `hubgeometry.ConfigFile(baseDir, module)` and `:23` reaches for `hubgeometry.LyxDirName`. The `<base>/_lyx/config/<module>.yaml` layout is a genuine cross-module convention, not one module's subdirectory, so it does not belong in an owning feature module. Production call sites are ~20 lines (`configengine`, `configcli`, `configsync`, `burlerengine`, `modelspec`, `scoutengine`, `lyxtest`); the 115/85 raw grep counts were inflated by tests.
- Rejected: leaving them in the shrunk module (re-establishes it as a path authority, contradicting the whole task). A new `internal/configpath` leaf (avoids widening `modelspec`'s allowlist, but invents a package to dodge a one-line allowlist edit).

### weftname-leaf

- Decision: extract a new stdlib-only leaf `internal/weftname` holding the `-weft` naming convention: the suffix constant, `SiblingPath(container, base)`, and `BareSiblingPath(container, base)`. `internal/fabricengine` and `internal/lyxtest` both import it. `-weft` is owned by `internal/weftname` alone in the token ownership map.
- Rationale: `lyxtest` does not consume weft geometry — it *produces* the on-disk shape production expects. `buildWeftPrime()` (`lyxtest.go:161-201`) creates `<tmp>/<base>-weft` as a real git repo plus `<base>-weft-bare`, and `CopyPaired`/`CopyPairedLocal` (`:452`, `:518`) re-derive that name when copying the template into a per-test container; the comment at `:451` says "must preserve the -weft suffix". The fixture is valid only if its directory names match what production derives. Today `lyxtest` gets `WeftSiblingPath` from `hubgeometry`; under `weft-junction-move` that symbol becomes private to `fabricengine`, and `lyxtest → fabricengine` is a compile-time cycle. A shared leaf gives both one source of truth without the cycle, and also removes the `base+"-weft-bare"` string that `lyxtest` already hardcodes today at `:196` and `:458`.
- Rejected: `lyxtest` hardcoding its own `-weft` constant (two sources of truth for a convention whose whole risk is drift, and `lyxtest.go` is production code so the literals guard scans it). Keeping `WeftSiblingPath` exported on `lyxcwd` (weft naming never leaves the shrunk module, which is the point of the slice).
- Rejected, and worth recording as the intended end state: having `lyxtest` build fixtures by calling real `fabric clone`/`fabric add` instead of hand-rolling directories. This is architecturally correct — it is what the fixtures are meant to be testing — but it is blocked twice over. `lyxtest → fabricengine` is a cycle because 25 `fabricengine` test files are in-package (`package fabricengine`) and import `lyxtest`; and the standard escape hatch of converting them to `package fabricengine_test` fails because 19 of those 25 need unexported access (`commitWeft`, `commitWeftLocked`, `seedLyxJunction`, `checkJunctionHealth`, `rollbackAdd`, `teardownHub`, `ensureBoardWorktree`, `loadCorrIndex`, `corrIndexPath`, `applyStaleRemoval`, `scanOnDiskJunctionNames`, `weftPathspecFilter`, `parseWarpSHATrailer`, and more). Moving the fixture builders into a separate fabric-aware package does not help either, because those same in-package tests are the predominant consumers of `PairedFixture`. Doing it properly needs an export-for-test shim across `fabricengine` first, which is a slice of its own and would have to land before this one.

### siblinglayout-removal

- Decision: delete `SiblingLayout` (`hubgeometry.go:164-185`). Its only production caller, `fabricengine/hostlayout.go:26`, constructs the sibling `Location` inline: same `HubPath`, same `AnchorRel`, different `WorktreeName`. `siblinglayout_test.go` is deleted with it rather than moved.
- Rationale: `SiblingLayout` exists to derive another worktree's `Layout` from that worktree's *root path* without spawning git, which under the old struct meant recomputing `Hub`, `RelPath`, `Prime` and `Repo`. Under `Location{RepoName, HubPath, WorktreeName, AnchorRel}` a hub sibling differs in exactly one field, so there is nothing left to compute. Deleting it also removes the last constructor that sets a dishonest `Cwd` (`:179`) and makes the "exactly three operations" scope statement literally true. Its unchecked direct-child-of-hub precondition (documented at `:164-168`) becomes structurally impossible to violate, since `WorktreeName` is a name rather than a free-standing path; `hostlayout.go:22` already rejects the non-sibling case before calling.
- Rejected: keeping it as a `Location` method `Sibling(name)` (a fourth operation on a primitive whose value is having exactly three).

### per-module-constructors

- Decision: each of the ~20 per-module path constructors moves to its owning module as a local constructor over a private relative-path constant. Destinations: `PlanDir`/`PlanDirRel`/`PlanOverview`/`DiscussionDir`/`DiscussionDecisionRecord`/`DiscussionSupportLog`/`LoomStatusFile`/`LoomStatusLock` → `internal/loomengine`; `BuilderDir`/`BuilderReportsDir` → `internal/builderengine`; `WebsterDir`/`WebsterReportsDir`/`WebsterPromptsDir` → `internal/websterengine`; `PerchRunsDir` → `internal/perchengine`; `ScoutDaemonStateFile`/`ScoutDaemonLock` → `internal/scoutengine`; `PatternDir`/`PatternFile`/`PatternFileHere` → `internal/pattern`; `WorktreeLogsDir` → `internal/logger`; `HubLogsDir` → `internal/reedengine`; `LyxDir`/`DotLyxDir` dissolve into whichever module joins onto `AnchorPath()`.
- Rationale: this is the bottleneck issue #127 named. The `_lyx` junction these subdirectories live under is already config-driven (slice 1); what changes is only that adding a module subdirectory stops being a change to a shared package.
- Rejected: a registry of relative subpaths in one place (same bottleneck with extra indirection).

### constructor-anchoring

- Decision: there is **no single base**. Each relocated constructor keeps the base it has today, per this table. `manifest/designs/fabric-unified-view.md`'s wording that modules join onto `cwd`, and the identical wording at `CONSTRAINTS.md:12`, both get corrected in this task.

| Base | Directory | Constructors |
|---|---|---|
| `AnchorPath()` | `_lyx` (durable, weft-synced, git-tracked) | `PlanDir`, `PlanDirRel`, `PlanOverview`, `DiscussionDir`, `DiscussionDecisionRecord`, `DiscussionSupportLog`, `LoomStatusFile`, `LoomStatusLock`, `BuilderDir`, `BuilderReportsDir`, `WebsterDir`, `WebsterReportsDir`, `WebsterPromptsDir`, `PerchRunsDir`, `PatternDir`, `PatternFile`, `PatternFileHere` |
| `WorktreePath()` | `.lyx` (ephemeral, machine-bound, never git-tracked) | `WorktreeLogsDir`, `ScoutDaemonStateFile`, `ScoutDaemonLock` |
| `HubPath` | `.lyx` at hub level | `HubLogsDir` |

- Rationale: a blanket "join onto `AnchorPath()`" would silently relocate four constructors. `HubLogsDir` is hub-anchored on purpose (`hubgeometry.go:399`) so one reed server per hub resolves to one deterministic place. `WorktreeLogsDir`, `ScoutDaemonStateFile` and `ScoutDaemonLock` use `dotLyxDirName` (`.lyx`, `:365`, `:406`), not `_lyx` — these are PIDs, sockets and rotating logs, which must never be git-tracked, and `.lyx` is the correct home for untracked lyx state. The `_lyx` group moves from `WorktreeRoot` to `AnchorPath()` because the `_lyx` junction itself lives at worktree-root + anchor (`HostLyxLinkHere`); for an unanchored repo (`AnchorRel == "."`) that is the same directory, and for a subpath-anchored repo the old `WorktreeRoot` base was pointing above the junction.
- Consequence for the batch-2 equivalence test: the "resolves to the same absolute path as before" check is **anchor-aware**, not byte-identical. For `AnchorRel == "."` every constructor in all three groups is byte-identical before and after. For a subpath-anchored repo the `_lyx` group intentionally moves down by `AnchorRel`, and the `.lyx` and `HubPath` groups stay byte-identical. Write the table test with both an unanchored and a subpath-anchored fixture so the intended move is asserted rather than assumed.
- Rejected: joining every constructor onto `AnchorPath()` uniformly (simpler rule, but moves the ephemeral and hub paths and would start git-tracking daemon PIDs). Joining onto a raw `cwd` field (the field no longer exists). Separate `Base()`/`Cwd` accessors (two names for one directory).

### weft-junction-move

- Decision: all `Weft*` (`WeftSuffix`, `WeftRepoRoot`, `WeftWorktree`, `WeftLyxDir`/`For`, `WeftRaddleDir`, `WeftHostSlug`), all `Host*Link`/`HostJunction`/`HostJunctions`/`HostJunctionsHere`, `PortalsDir`/`PortalLink`/`PortalTarget`, `LaunchersDir`/`LauncherDir`/`MenuLauncherPath`/`menuLauncherName`/`LauncherSpawnRel`/`MenuLauncherRel`, `WorktreePath(slug)`, `HubPath(parent,name)`/`HubSuffix`, `HubReservedNames`/`IsReservedHubName`, and the `LyxDirName`/`PatternDirName`/`BoardDirName` constants move into `internal/fabricengine`, private unless an in-scope caller needs otherwise. `WeftSuffix`/`WeftSiblingPath` go to `internal/weftname` instead — see `weftname-leaf`.
- Decision: the four pattern-specific accessors — `WeftPatternDir`, `WeftPatternDirFor`, `HostPatternLink`, `HostPatternLinkHere` (`hubgeometry.go:502-545`) — are **deleted**, not moved.
- Rationale: this is Fabric's own illusion-maintenance plumbing. No consumer needs to know weft exists — and PATTERN least of all, since the illusion's whole promise is that nothing outside Fabric can tell there are two repos. The four accessors violate that by naming both sides of the junction for one specific junction, and they are redundant: `_pattern` is already wired by the generic config-driven junction machinery. `hubgeometry.go:313` deliberately excludes `PatternDirName` from the reserved-name set precisely so it flows through `pathspec` like any other name, and `fabricengine/config_driven_junctions_integration_test.go` exercises that path. The accessors have zero production consumers — every reference is a test (`fabricengine/*_test.go`, one in `loomengine/preflight_integration_test.go`, and `hubgeometry`'s own). Those tests get rewritten against the generic junction list, which is what they should have asserted against all along.
- Consequence: `internal/pattern` keeps only `PatternDir`/`PatternFile`/`PatternFileHere` (see `per-module-constructors`) and imports neither `fabricengine` nor `weftname`. The Pattern Leaf Invariant needs no widening — only the package rename.
- Rejected: keeping a public compatibility layer in the shrunk module (defeats the purpose). Moving the four accessors into `fabricengine` private (preserves a weft-aware view of PATTERN that should not exist, and duplicates the generic machinery).

### seven-leak-fixes

- Decision: fix all seven non-`fabric*` production callers of `Weft*`/`Host*Link` in this task rather than shimming them: `internal/buildercli/weft.go`, `internal/builderengine/spawn.go`, `internal/loomengine/preflight.go`, `internal/perchcli/run.go`, `internal/webstercli/weft.go`, `internal/websterengine/audit.go`, `internal/websterengine/runlevel.go` (plus `internal/lyxtest`). Route each through a `fabricengine` exported accessor or an operation return value.
- Rationale: making those methods private breaks these files, so slice 7 must deal with them either way; a temporary exported shim would be surface deliberately created in order to delete it one slice later. All seven are read-only uses (reporting/audit text, and `websterengine/audit.go` building a regex to scrub the weft path *out* of logs), so none needs an independent weft git operation. Slice 8 then owns only its open policy question.
- Rejected: a thin exported shim deleted by slice 8 (interim surface). Merging slices 7 and 8 into one task (too large).

### prime-and-list-move

- Decision: `worktreelist.go` in full (`List`, `WorktreeEntry`, `parseWorktreePorcelain`) plus `Prime`, `PrimeName` and `deriveRepo` move into `internal/fabricengine`. `Prime` disappears from the returned struct. Of the three non-`fabric*` production callers, `internal/ideengine/menu.go:38` (which wants the sibling list) and `internal/loomengine/preflight.go:67` (which only checks `l.Prime != ""`) take narrow `fabricengine` exports; `internal/vscode/color.go` instead gains a parameter — `PickColor(l *lyxcwd.Location, primeName string)` — supplied by its sole caller `internal/ideengine/spawn.go:21`, which is already fabric-aware.
- Rationale for the `vscode` parameter: `vscode/color.go` imports only stdlib + `hubgeometry` today, and it wants `l.Prime` (`:47`) for one thing — skipping the prime worktree's directory while scanning siblings for used colours. Sourcing that from `fabricengine` would pull the entire fabric engine into a colour picker and reintroduce a `git worktree list` subprocess per call, which is the exact cost this decision exists to delete. Passing the name in keeps `vscode` a leaf and costs one argument.
- Rationale: `List(cwd)` spawns `git worktree list` on **every** `Resolve()` call purely to find the entry flagged `Main`, even though the result is identical for every worktree under one hub. Ordinary callers never needed it. Removing it deletes a subprocess from the hot resolve path.
- Rejected: moving `List` but keeping `Prime` on the struct (keeps the per-`Resolve` subprocess this slice exists to delete).

### reponame-derivation

- Decision: `RepoName` stays on `Location`, derived as `strings.TrimSuffix(filepath.Base(HubPath), "-HUB")`. Consequently `-HUB` remains a token `internal/lyxcwd` knows, and joins `_board` as dual-owned in the token ownership map.
- Rationale: repo identity is useful to have on the coordinates struct even though today it has exactly one production consumer (`internal/tokenvocab/tokenvocab.go:25`, a `repo` template token — the other grep hits were the unrelated `gitrepo.Repo` type). The alternative source, `filepath.Base(Prime)` (`hubgeometry.go:157`), dies with `Prime`, and reviving it would restore the `git worktree list` subprocess.
- Behaviour change to note: for a non-hub layout (an ordinary clone, or a `lyxtest` synthetic hub) this yields the parent directory's name rather than the prime worktree's name. Both are heuristics; the new one costs no subprocess.
- Rejected: dropping `RepoName` and having `tokenvocab` ask `fabricengine` (purer layering, but the user wants the value on the struct). Deriving as bare `filepath.Base(HubPath)` (yields `loomyard-HUB`, changing the rendered `repo` token).

### boarddir-ownership

- Decision: `internal/fabricengine` exports `BoardDir`; `internal/lyxcwd` keeps a **private** `boardDir()` used only for the anchor read.
- Rationale: keeps the module's public surface at exactly the resolution symbols. `BoardDir` has ten production callers outside it (`boardcli`, `boardengine`, `configsync`, `fabriccli` ×9, `fabricengine`, `ideengine`), all of which are Fabric-layer or board-layer callers. The duplicated `_board` literal is sanctioned by the dual-ownership entry in the token map, not a leak.
- Rejected: continuing to export `BoardDir` from the shrunk module (re-adds a path constructor to the module whose point is having none). Making `_board` config-driven (it is structural, not configurable — inventing a config key that should not exist).

### board-junction

- Decision: `fabricengine` wires the `_board` junction **explicitly and unconditionally** — link `<AnchorPath>/.board` → target `<HubPath>/_board` — as a hub-level junction outside the `pathspec` mechanism. `_board` is **not** added to `fabric.yaml`'s `pathspec`. `filterHubReserved` (`junctionnames.go:20`), `ScopedPathspec`, and `Topology.Add`'s reserved union are all left exactly as they are. It is wired at the same points the pathspec junctions are (clone, add, reconcile) and is covered by the same drift/health checks, but as a named special case rather than a list entry.
- Rationale: this reverses the earlier "put it in the pathspec" decision, because two facts found in review make the pathspec route wrong rather than merely awkward. First, `pathspec` is dual-purpose: `fabriccli/weft_verbs.go:102` feeds the **raw unfiltered** `cfg.Dirs()` into `ScopedPathspec(l.RelPath, …)`, so `filterHubReserved` guards only the junction half — adding `_board` would silently inject `<rel>/_board` into the weft *commit* pathspec, which is wrong: `_board` is itself a weft worktree, not content to be committed from the warp side. Second, `_board` does not fit the junction record shape at all. Every pathspec junction is a warp↔weft **mirror pair**: `HostJunctions`/`HostJunctionsHere` (`hubgeometry.go:565-591`) build Link=`<worktree>/<rel>/<name>` and Target=`<weftWorktree>/<rel>/<name>` from *one* name used on both sides. `_board` has a different name on each side (`.board` vs `_board`) and its target is hub-level, not weft-worktree-level, so it breaks the name mapping, the target derivation, and every reconcile/drift/health loop that iterates the wired names. Forcing it through would cost four separate exceptions and leave a config entry every consumer must special-case. Millhouse's `.wiki` junction — the stated model — is likewise wired explicitly, not listed as content.
- Rejected: adding `_board` to `pathspec` with exceptions in `filterHubReserved`, `ScopedPathspec` and `Topology.Add` (honours "one list, one config file" in letter while making the list mean two different things). Deferring to slice 9 (slice 9 is `.lyx` hygiene; this is not that).

### enforcement-rewrite

- Decision: rewrite `TestEnforcement_GeometryLiterals` from a single-package allowlist into a per-token ownership map:

| Token | Owner(s) |
|---|---|
| `-weft` | `internal/weftname` |
| `_portals`, `_launchers`, `_raddle` | `internal/fabricengine` |
| `_pattern` | `internal/pattern`, `internal/fabricengine` |
| `_lyx` | `internal/configengine`, plus each module owning a subdirectory under it |
| `_board` | `internal/lyxcwd`, `internal/fabricengine` |
| `-HUB` | `internal/lyxcwd`, `internal/fabricengine` |

- `_pattern` is dual-owned because `internal/pattern` holds the worktree-side constructors while `fabricengine` still needs the bare name as a git pathspec (`pull.go:299`). `internal/lyxtest` needs **no** entry: it gets `-weft` from `weftname` and `_lyx` from `configengine`, so it constructs no geometry literal of its own. `TestEnforcement` (the `os.Getwd` / `--show-toplevel` ban) keeps its shape with the allowlist path updated to `internal/lyxcwd`.
- `.lyx` stays **unpoliced** this slice, as it is today (`enforcement_test.go:224` does not list it). Batch 2 spreads it to `logger`, `scoutengine` and `reedengine` per the `constructor-anchoring` table, and slice 9 — which registers `.lyx` as a pathspec junction and removes `crossModuleMachineLocalExcludes` — is where it gets an owner. Adding it to the map now would have to be undone one slice later. Stated explicitly so a later reader does not read the omission as an oversight.
- Rationale: the current test bans every geometry token in path-construction context anywhere outside `internal/hubgeometry`, so slice 7's core move — each module owning its own `_lyx/<module>` constant — violates it by construction. A per-token map is strictly stronger than a blanket allowlist: it encodes *who* owns each token rather than just "one package owns all of them", and it is what proves batches 1–4 actually moved ownership rather than merely copied code.
- Rejected: dropping `_lyx` from the ban entirely (lets the token spread unowned across ~20 modules). Retiring the literals guard and keeping only the `os.Getwd` ban (weakest enforcement, at exactly the moment enforcement matters most). Adding a separate import-direction test (overlaps the existing leaf tests and needs a hand-maintained exception list). Adding an exported-symbol-count assertion on the shrunk module (encodes "stays shrunk" but is brittle).

### leaf-invariant-updates

- Decision: widen `internal/lyxtest`'s leaf allowlist from `stdlib + internal/hubgeometry` to `stdlib + internal/lyxcwd + internal/configengine + internal/weftname`; widen `internal/modelspec`'s and `internal/tokenvocab`'s to include `internal/configengine`. `internal/pattern`'s allowlist is **not** widened — only the package name changes. Update all six invariants that name `internal/hubgeometry` to `internal/lyxcwd`: lyxtest Leaf, Modelspec Leaf, Treadle Runner-Seam (which names it in a *negative* clause), Tokenvocab Leaf, Scoutengine Leaf, Pattern Leaf.
- Rationale: `lyxtest` uses exactly four things from `hubgeometry` today — `ConfigDir`/`ConfigFile` (`lyxtest.go:38,45,181`), `WeftSiblingPath` (`:172,452,518`), `LyxDirName` (`:228`), and `Resolve`/`Layout`. After the shrink those land in three packages, all of which sit strictly *below* `lyxtest`, so the dependency graph keeps today's one-directional shape. `configengine` is safe to depend on: all three of its test files are `package configengine_test` and none imports `lyxtest`, and its own imports are `envsource`/`lyxcwd`/`yamlengine`. `modelspec` calls `ConfigFile` (`modelspec/load.go:23`), which now lives in `configengine`. The rename ripple is mechanical but must not be missed — a stale package name in a leaf allowlist silently stops enforcing.
- Note: `lyxtest` must **not** import `internal/fabricengine`. That is a compile-time cycle (25 in-package `fabricengine` test files import `lyxtest`), which is the reason `weftname` exists — see `weftname-leaf`.
- Rejected: letting `lyxtest`/`modelspec` hardcode their own literals (scatters exactly the tokens this slice is centralizing per-owner, and `lyxtest.go` is production code so the literals guard scans it).

### batching

- Decision: five batches. (1) module rename + `Location` reshape + strict gate (with the normalized comparison helper) + `SiblingLayout` deletion + `.lyx-anchor` rename + the `clone.go:112` step-5 fix + `ConfigDir`/`ConfigFile`/`DotEnv` move + `internal/weftname` extraction + leaf-allowlist and invariant-name updates. (2) the ~20 per-module constructors into their owning modules, per the `constructor-anchoring` table. (3) `Weft*`/`Host*`/junction/portal/launcher move into `fabricengine` + deletion of the four pattern accessors + the seven leak fixes + `BoardDir` export + explicit `_board` junction wiring. (4) `Prime`/`List`/`worktreelist.go` move + the three consumer fixes including the `PickColor` signature change. (5) `enforcement_test.go` ownership-map rewrite + `CONSTRAINTS.md` + `docs/overview.md` + `docs/shared-libs/hubgeometry.md` + design-doc updates.
- Rationale: each batch must leave `go build ./...` and the full suite green. Batch 1 carries the rename, so it must land first and touch every importer's import block once; `weftname` must be extracted in the same batch because batch 3 makes `WeftSiblingPath` private and `lyxtest` would break at that moment otherwise. Batch 3 has the widest blast radius and is isolated. Batch 5 lands last because it is the guard that proves batches 1–4 moved ownership rather than copying it — running it earlier would fail against a half-moved tree.
- Rejected: three batches merging (1)+(2) and (3)+(4) (bigger blast radius per implementer pass). One atomic batch (24+ packages with a red tree throughout).

## Technical context

Current module layout: `internal/hubgeometry/hubgeometry.go` (591 lines — `Layout`, `Resolve`, `ResolveWorktree`, `resolveCore`, `SiblingLayout`, and ~30 path constructors), `anchor.go` (53 — `FabricAnchorName`, `ErrCwdOutsideAnchor`, `readRecordedAnchor`), `worktreelist.go` (78 — `List`, `WorktreeEntry`, `parseWorktreePorcelain`), plus `enforcement_test.go` (453) and 14 other test files.

Post-shrink target is roughly 120–150 lines total: `Getwd`, `Resolve`, `ResolveWithAnchor`, `ResolveWorktree`, `Location` + two accessors, the normalized path-comparison helper, `AnchorFileName`/`readRecordedAnchor`/private `boardDir()`, and the two error sentinels. No `SiblingLayout` — see `siblinglayout-removal`. Dependencies must stay stdlib + `internal/gitexec` only — that is what keeps the `logger` cycle closed.

Call-site weight before the move (grep counts include tests): `Layout` 233, `Resolve` 157, `ConfigFile` 115, `ConfigDir` 85, `LyxDirName` 76, `BoardDir` 73, `Getwd` 41, then a long tail. 24+ packages import the module.

Field consumers outside `hubgeometry`/`fabric*`/`lyxtest`, which is what justifies keeping each coordinate: `Cwd` ~25 files (mostly `*cli` packages passing a base to `configengine.Load`); `WorktreeRoot` ~15 (`logger/sink.go`, `shuttleengine/rundir.go`, `reedengine/lock.go`, `websterengine/runlevel.go`, `perchengine`, `burlerengine`, `scoutengine/refs.go`, `configcli` — all worktree-wide singletons); `Hub` ~11 (`reedengine/lock.go`+`server.go`, `boardcli`, `vscode/color.go`, `scoutcli`, `tokenvocab`, `ideengine/menu.go` — hub-wide singletons, one reed server per hub); `RelPath` ~10 (four of which are weft-adjacent and disappear with the leak fixes); `Repo` exactly 1; `Prime` 2.

Key files to read before implementing: `internal/hubgeometry/hubgeometry.go:102-185` (`resolveCore` and the gate), `internal/hubgeometry/anchor.go:42-53` (`readRecordedAnchor`), `internal/fabricengine/clone.go:137-182` (board worktree materialization then marker adopt-or-create), `internal/fabricengine/junctionnames.go:20-34` (`filterHubReserved`), `internal/fabricengine/config.go` (`Config.Dirs()`, `LoadConfig`), `internal/configengine/config.go:20-50` (`FindBaseDir`, `Load`), `internal/logger/sink.go:70-85` (the cycle-forcing call), `internal/hubgeometry/enforcement_test.go:212-453` (the guard being rewritten).

Gotcha: `fabricengine.LoadConfig` is the one module config anchored at `BoardDir(hub)` rather than a per-worktree base, because fabric's junction pathspec must be one repo-wide fact. Do not "normalize" it to the per-worktree convention.

Gotcha: `internal/fabricengine/list.go:10-18` already type-aliases `WorktreeEntry` and wraps `List` — batch 4 collapses the alias rather than creating a new one.

Gotcha: 19 of the 25 `fabricengine` test files that import `lyxtest` reach unexported `fabricengine` identifiers, so they cannot be converted to `package fabricengine_test`. Any change that would require `lyxtest` to import `fabricengine` is therefore a hard compile error, not a style question.

Gotcha: `fabriccli/weft_verbs.go:102` passes the **raw unfiltered** `cfg.Dirs()` to `ScopedPathspec`, while the junction side passes it through `filterHubReserved`. `pathspec` is two lists wearing one name; anything added to it lands in the weft commit scope as well as the junction set.

## Constraints

From `CONSTRAINTS.md`:

- **Hub Geometry Invariant** — retired and replaced by this task. `CONSTRAINTS.md:5-18` already holds the narrow replacement text; it needs the module renamed to `internal/lyxcwd`, the `_board`/`-HUB`/`_pattern` dual ownership recorded, the strict-equality gate stated, and the enforcement pointer updated. `CONSTRAINTS.md:12` also carries the same "joined onto `cwd` directly" wording that `constructor-anchoring` corrects, and must be fixed in step with `manifest/designs/fabric-unified-view.md` — the two must not be allowed to disagree. The heading should become the **Cwd Resolution Invariant**, since that is what the rewritten `enforcement_test.go` polices.
- **lyxtest Leaf Invariant**, **Modelspec Leaf Invariant**, **Tokenvocab Leaf Invariant**, **Scoutengine Leaf Invariant**, **Pattern Leaf Invariant**, **Treadle Runner-Seam Invariant** — all six name `internal/hubgeometry` and all six need the new package name; three also need widened allowlists per `leaf-invariant-updates`. The Pattern Leaf Invariant needs the rename only.
- **Test Tier Purity Invariant** — untagged tests spawn nothing. Any new test that needs a real worktree must carry an `integration`/`smoke`/`scout` build tag; the strict-gate and anchor-rename tests are prime candidates to be written untagged with pure string math instead.
- **Hermetic Git Test Environment Invariant** — any test package newly spawning git needs a `TestMain` calling `lyxtest.HermeticGitEnv()`.
- **Fabric Git Invariant (warp + weft)** — unchanged by this task; warp stays ordinary git.
- **Documentation Lifecycle** — this task changes cross-cutting infrastructure, so `manifest/designs/fabric-unified-view.md`, `docs/overview.md` and `CONSTRAINTS.md` update in the same commits as the code.

Discovered during discussion:

- Module dependency ceiling: `internal/lyxcwd` may import only stdlib and `internal/gitexec`. Any other import risks the `fabricengine → logger → lyxcwd` chain becoming a cycle.
- `docs/overview.md`'s "Hub Geometry Invariants" section needs rewriting to the narrower contract; its "Junction model" section is already accurate and must not be touched.
- `docs/shared-libs/hubgeometry.md` is a full module doc for the package being renamed and gutted, documenting the entire departing API. Per the Documentation Lifecycle it is renamed to `docs/shared-libs/lyxcwd.md` and rewritten to the three-operation contract in batch 5. Sibling docs in that directory (`configengine.md` in particular, which gains `ConfigDir`/`ConfigFile`/`DotEnv`) need the corresponding additions.

## Testing

TDD candidates, in the order they should be written:

- **Strict anchor gate** (`internal/lyxcwd`) — table test over `(cwd, anchorRel, worktreePath)` triples: exact match resolves; a subdirectory of the anchor errors; a parent of the anchor errors; a sibling errors. Pure string math, no git spawn, so untagged. This is the behaviour change most likely to be got wrong.
- **Gate comparison normalization** (`internal/lyxcwd`) — the named comparison helper, covered separately from the gate itself: trailing separator, `.`/`..` segments, mixed separators, a symlinked path that resolves to the target, and a case-differing path that must match on Windows and must *not* match on Linux. The symlink case needs a temp dir; the rest is string math.
- **`Location` accessors** — `WorktreePath()` and `AnchorPath()` over hub/name/anchor combinations including `AnchorRel == "."`, and a Windows-separator case. Untagged.
- **`RepoName` derivation** — `-HUB` suffix present, absent, and a directory literally named `-HUB`. Untagged.
- **Anchor fallback** — no marker yields `AnchorRel == "."`; empty/whitespace-only marker is treated as absent; unreadable marker is treated as absent. Untagged with a temp dir.
- **Stale-marker detection** (`internal/fabricengine`) — a `_board` containing `.fabric-anchor` but no `.lyx-anchor` produces the "re-clone required" error, not a silent `"."` fallback. This is the guard against the no-fallback rename failing silently.
- **`_board` junction wiring** (`internal/fabricengine`) — the `.board` link is created at `<AnchorPath>/.board` pointing at `<HubPath>/_board` after clone and after add, and is repaired by reconcile when missing or mispointed. Paired with a negative assertion that `_board` appears in **neither** `filterHubReserved`'s output **nor** `ScopedPathspec`'s — the regression this guards is someone "simplifying" the special case back into the pathspec.
- **`weftname` round-trip** (`internal/weftname`) — `SiblingPath`/`BareSiblingPath` over container/base combinations, plus an assertion that `lyxtest`'s fixture names and `fabricengine`'s production derivation agree for the same input. Untagged.
- **Rewritten `TestEnforcement_GeometryLiterals`** — keep the existing `predicate` sub-test shape (synthetic positive/negative Go snippets parsed with `go/parser`, whole-token matching so `_boardroom`/`-weft-bare` stay negatives) and extend it per token/owner. Keep the `scanned_non_empty` sanity sub-test; a misconfigured walk must not produce a vacuous pass.

Scenarios that must be covered somewhere, tagged as needed:

- `ResolveWithAnchor` produces the same `Location` as `Resolve` when the marker on disk matches the injected anchor.
- Clone still succeeds end-to-end with the renamed marker, and a re-clone adopts the recorded anchor rather than re-anchoring.
- Each relocated per-module constructor resolves to the path the `constructor-anchoring` table says it should — run over both an unanchored (`AnchorRel == "."`) and a subpath-anchored fixture, so the `_lyx` group's intended move is asserted and the `.lyx`/`HubPath` groups are pinned byte-identical. This is what makes batch 2 safe to review.
- `fabric clone` still installs the post-checkout hook at step 5 with no marker present and cwd outside the hub.
- `_pattern` is still wired, health-checked and repaired through the generic junction path after the four pattern accessors are deleted — this is the coverage those deleted tests were standing in for.
- The seven leak-fixed call sites still produce identical user-visible output.
- `PickColor` still skips the prime worktree when the prime name is passed in, and degrades sanely when it is empty.

Existing test files in `internal/hubgeometry` that test departing symbols (`weft_test.go`, `planpath_test.go`, `discussionpath_test.go`, `loomstatus_test.go`, `scoutdaemon_test.go`, `webstergeom_test.go`, `pattern_test.go`, `raddle_guard_test.go`, `worktreelist_test.go`, `worktreelogs_test.go`) move with their symbols rather than being deleted — losing that coverage would be the silent cost of this refactor. The two exceptions are `siblinglayout_test.go`, deleted with `SiblingLayout`, and the `WeftPatternDir`/`HostPatternLink` sub-tests in `pattern_test.go:88-118`, deleted with those accessors; the rest of `pattern_test.go` moves to `internal/pattern`.

## Q&A log

- **Q:** Are `ConfigDir`/`ConfigFile` already in `configengine`? **A:** No — they are defined in `hubgeometry.go:188-195`; `configengine/config.go:42` is the caller. Moving them makes `configengine.Load` self-contained.
- **Q:** Fix the seven `Weft*` leak callers now, or shim them for slice 8? **A:** Fix them now.
- **Q:** Is `Hub` the root everything else derives from? **A:** No, the reverse — `hub := filepath.Dir(worktreeRoot)`, so resolution runs cwd → worktree → hub. `HubPath` is a product of resolution, not its root.
- **Q:** Is the full worktree path needed, or does the name suffice? **A:** The name suffices; the worktree is a direct child of the hub by construction, so the path is derivable and storing it would be redundant.
- **Q:** Is `cwd` derived from the other three fields? **A:** Under today's at-or-below gate, no — cwd may sit deeper. Under the new strict-equality contract, yes, so it stops being a stored field but remains the `Resolve` parameter.
- **Q:** Must cwd be used to find the worktree, and can that happen inside Fabric? **A:** cwd yes, inside Fabric no — `logger/sink.go` calls the resolver and `fabricengine` imports `logger`, so moving it would create a cycle.
- **Q:** Is the anchor read from GitHub during `fabric clone`? **A:** No — the weft repo is cloned to local disk (step 6) and `_board` materialized (step 7) before the marker is read (step 8). No remote read exists.
- **Q:** Should `RepoName` come first in the struct? **A:** Yes — outermost identity first: repo, hub, worktree, anchor.
- **Q:** Is `hubgeometry` still a good name? **A:** No. After the shrink the module does no geometry — Fabric owns geometry. It is an entry gate that validates cwd and returns coordinates, hence `internal/lyxcwd` with type `Location`.
- **Q:** Why not name the package `anchor`? **A:** It collides with `AnchorRel`, the name of a field inside it, and the module does more than store an anchor.
- **Q:** Reviewer model schedule for discussion review? **A:** `opushigh` for rounds 1–2, `sonnetxhigh` for rounds 3–5, via `millpy-review-discussion.py --reviewer`; no config change.
- **Q:** How does `lyxtest` reach weft geometry today, and can it keep doing the same? **A:** Today it imports `hubgeometry` one-directionally, which works only because `hubgeometry` is the god-module. After the shrink `WeftSiblingPath` moves above `lyxtest`, so the same arrangement is impossible; `internal/weftname` restores it.
- **Q:** Can `fabric` itself build the test fixture instead of `lyxtest` hand-rolling it? **A:** Architecturally yes, mechanically no — `lyxtest → fabricengine` is a cycle and 19 of the 25 in-package `fabricengine` test files need unexported access, so they cannot be moved out. Recorded as the intended future slice, gated on an export-for-test shim.
- **Q:** Why does PATTERN have any relation to "weft" and "host"? **A:** It shouldn't — that is a bug. The four pattern-specific accessors violate the illusion and duplicate the generic config-driven junction machinery; they are deleted, not moved.
- **Q:** What is `SiblingLayout` and is it needed? **A:** It derived another worktree's `Layout` from that worktree's root without spawning git. Under `Location` a hub sibling differs by one field, so it is deleted.
- **Q:** Should `_board` go in `fabric.yaml`'s `pathspec`? **A:** Initially yes, reversed in review. `pathspec` also feeds the weft commit scope, and `_board` does not fit the warp↔weft mirror-pair junction record shape. It is wired explicitly in `fabricengine` instead — always wired, just not via the list.
- **Q:** Do the relocated log/daemon paths move under `_lyx`? **A:** No. `.lyx` is correct for untracked lyx state, and these are PIDs, sockets and rotating logs that must never be git-tracked. Only `_lyx`-durable paths re-anchor onto `AnchorPath()`.
