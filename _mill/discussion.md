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
- Add the cwd→`_board` junction via `fabric.yaml`'s `pathspec`, with a documented `filterHubReserved` exception.
- Rewrite `enforcement_test.go` from a single-package allowlist to a per-token ownership map.
- Widen the leaf-invariant allowlists for `lyxtest`, `modelspec`, `tokenvocab`; update all six leaf/seam invariants that name `internal/hubgeometry`.
- Update `CONSTRAINTS.md`, `docs/overview.md`, and `manifest/designs/fabric-unified-view.md`.

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
- Clone ordering fact for the implementer: the marker is never read from GitHub. `CloneHub` (`fabricengine/clone.go:79`) clones the weft repo to local disk at step 6 (`:120`), materializes `<Hub>/_board` as a second weft worktree checked out on `main` at step 7 (`:137`), and only then reads the marker at step 8 (`:146-178`). Steps 1–7 have no anchor available locally and cwd is outside any hub, so `Resolve` cannot and must not be called during that window — today's code correctly does not call it, and this task must preserve that.
- Rejected: no override, clone keeps calling `Resolve(primeCwd)` after writing the marker (keeps the ordering dependency, no `lyxtest` seam). `fabricengine` owning the file entirely and always injecting (cleanest layering, but `logger/sink.go` resolves with no fabric involvement and would silently get `AnchorRel = "."`).

### resolution-stays-below-fabric

- Decision: cwd resolution stays in `internal/lyxcwd`, documented as an infrastructure exception to "every module asks Fabric" — the module sits *below* `fabricengine` in the dependency graph, not beside it. `internal/logger` keeps calling it directly.
- Rationale: verified import cycle, not an inherited assumption. `internal/logger/sink.go:74,79` calls `hubgeometry.Getwd()` then `hubgeometry.Resolve(cwd)` to place its trace file, and `internal/fabricengine/coalesce.go:18` and `spawn.go:19` import `internal/logger`. Moving the resolver into `fabricengine` produces `fabricengine → logger → fabricengine`. Keeping the module stdlib + `internal/gitexec` only is what holds that cycle closed.
- Rejected: moving resolution into `fabricengine` and injecting the log directory into `logger` from `cmd/lyx/main.go` — a cleaner end state that eliminates the module entirely, but it pulls `logger` initialization rework into an already 24-package slice, and `logger`'s lazy resolve exists so early-boot logging works before explicit setup. Record this in the design doc as the intended follow-up. A private duplicate resolver inside `logger` is rejected outright: two resolvers is the defect this module prevents.

### config-path-move

- Decision: `ConfigDir`, `ConfigFile` and `DotEnv` move to `internal/configengine`, exported.
- Rationale: `configengine` is already the consumer and owns everything around these paths — `configengine/config.go:42` calls `hubgeometry.ConfigFile(baseDir, module)` and `:23` reaches for `hubgeometry.LyxDirName`. The `<base>/_lyx/config/<module>.yaml` layout is a genuine cross-module convention, not one module's subdirectory, so it does not belong in an owning feature module. Production call sites are ~20 lines (`configengine`, `configcli`, `configsync`, `burlerengine`, `modelspec`, `scoutengine`, `lyxtest`); the 115/85 raw grep counts were inflated by tests.
- Rejected: leaving them in the shrunk module (re-establishes it as a path authority, contradicting the whole task). A new `internal/configpath` leaf (avoids widening `modelspec`'s allowlist, but invents a package to dodge a one-line allowlist edit).

### per-module-constructors

- Decision: each of the ~20 per-module path constructors moves to its owning module as a local constructor over a private relative-path constant. Destinations: `PlanDir`/`PlanDirRel`/`PlanOverview`/`DiscussionDir`/`DiscussionDecisionRecord`/`DiscussionSupportLog`/`LoomStatusFile`/`LoomStatusLock` → `internal/loomengine`; `BuilderDir`/`BuilderReportsDir` → `internal/builderengine`; `WebsterDir`/`WebsterReportsDir`/`WebsterPromptsDir` → `internal/websterengine`; `PerchRunsDir` → `internal/perchengine`; `ScoutDaemonStateFile`/`ScoutDaemonLock` → `internal/scoutengine`; `PatternDir`/`PatternFile`/`PatternFileHere` → `internal/pattern`; `WorktreeLogsDir` → `internal/logger`; `HubLogsDir` → `internal/reedengine`; `LyxDir`/`DotLyxDir` dissolve into whichever module joins onto `AnchorPath()`.
- Rationale: this is the bottleneck issue #127 named. The `_lyx` junction these subdirectories live under is already config-driven (slice 1); what changes is only that adding a module subdirectory stops being a change to a shared package.
- Rejected: a registry of relative subpaths in one place (same bottleneck with extra indirection).

### constructor-anchoring

- Decision: relocated constructors join their relative subpath onto the caller-supplied base, and that base is `Location.AnchorPath()`. `manifest/designs/fabric-unified-view.md`'s wording that modules join onto `cwd` gets corrected in this task.
- Rationale: the design doc's "join onto cwd" is wrong twice over. Today `PlanDir`, `DiscussionDir`, `LoomStatusFile`, `ScoutDaemonStateFile` and `WorktreeLogsDir` are deliberately `WorktreeRoot`-anchored so a caller invoked from a subdirectory resolves the one true `status.json`; and the `_lyx` junction itself lives at worktree-root + anchor (`HostLyxLinkHere`), not at cwd. Under the `cwd-is-not-a-field` decision cwd equals `AnchorPath()` anyway, so `AnchorPath()` is both correct and the single name for the concept.
- Rejected: joining onto a raw `cwd` field (behaviour change for five constructors, wrong under an anchor, and the field no longer exists). Separate `Base()`/`Cwd` accessors (two names for one directory).

### weft-junction-move

- Decision: all `Weft*` (`WeftSuffix`, `WeftSiblingPath`, `WeftRepoRoot`, `WeftWorktree`, `WeftLyxDir`/`For`, `WeftPatternDir`/`For`, `WeftRaddleDir`, `WeftHostSlug`), all `Host*Link`/`HostJunction`/`HostJunctions`/`HostJunctionsHere`, `PortalsDir`/`PortalLink`/`PortalTarget`, `LaunchersDir`/`LauncherDir`/`MenuLauncherPath`/`menuLauncherName`/`LauncherSpawnRel`/`MenuLauncherRel`, `WorktreePath(slug)`, `HubPath(parent,name)`/`HubSuffix`, `HubReservedNames`/`IsReservedHubName`, and the `LyxDirName`/`PatternDirName`/`BoardDirName` constants move into `internal/fabricengine`, private unless an in-scope caller needs otherwise.
- Rationale: this is Fabric's own illusion-maintenance plumbing. No consumer needs to know weft exists.
- Rejected: keeping a public compatibility layer in the shrunk module (defeats the purpose).

### seven-leak-fixes

- Decision: fix all seven non-`fabric*` production callers of `Weft*`/`Host*Link` in this task rather than shimming them: `internal/buildercli/weft.go`, `internal/builderengine/spawn.go`, `internal/loomengine/preflight.go`, `internal/perchcli/run.go`, `internal/webstercli/weft.go`, `internal/websterengine/audit.go`, `internal/websterengine/runlevel.go` (plus `internal/lyxtest`). Route each through a `fabricengine` exported accessor or an operation return value.
- Rationale: making those methods private breaks these files, so slice 7 must deal with them either way; a temporary exported shim would be surface deliberately created in order to delete it one slice later. All seven are read-only uses (reporting/audit text, and `websterengine/audit.go` building a regex to scrub the weft path *out* of logs), so none needs an independent weft git operation. Slice 8 then owns only its open policy question.
- Rejected: a thin exported shim deleted by slice 8 (interim surface). Merging slices 7 and 8 into one task (too large).

### prime-and-list-move

- Decision: `worktreelist.go` in full (`List`, `WorktreeEntry`, `parseWorktreePorcelain`) plus `Prime`, `PrimeName` and `deriveRepo` move into `internal/fabricengine`. `Prime` disappears from the returned struct. The three non-`fabric*` production callers are served by narrow `fabricengine` exports: `internal/vscode/color.go:47` and `internal/loomengine/preflight.go:67` (which want the prime worktree's name/existence) and `internal/ideengine/menu.go:38` (which wants the sibling list).
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

- Decision: add the cwd→`_board` junction by adding `_board` to `fabric.yaml`'s `pathspec`, with a documented exception carved into `filterHubReserved` (`fabricengine/junctionnames.go:20`). The host-side link name differs from the hub directory name (link `.board` → target `<HubPath>/_board`) so no collision is possible.
- Rationale: the junction list already lives in one config file — that is exactly slice 1's `pathspec` — so a second junction source would contradict "one list, one config file". `filterHubReserved` currently drops `_board` unconditionally to stop a hub-structural name wiring a colliding per-worktree junction; the exception is explicitly "`_board` is the one hub-structural name that is also a legitimate per-worktree junction target", and the guard keeps rejecting `_portals`, `_launchers` and `_raddle`. Mirrors millhouse's own `.wiki` junction, which is the stated model.
- Rejected: wiring the `_board` link outside the pathspec in `fabricengine` (cleanest guard, but two junction sources). Deferring to slice 9 (slice 9 is `.lyx` hygiene; this is not that).

### enforcement-rewrite

- Decision: rewrite `TestEnforcement_GeometryLiterals` from a single-package allowlist into a per-token ownership map. Ownership: `-weft`, `_portals`, `_launchers`, `_raddle`, `_pattern` → `internal/fabricengine` only; `_lyx` → `internal/configengine` plus each module that owns a subdirectory under it; `_board` → `internal/lyxcwd` and `internal/fabricengine`; `-HUB` → `internal/lyxcwd` and `internal/fabricengine`. `TestEnforcement` (the `os.Getwd` / `--show-toplevel` ban) keeps its shape with the allowlist path updated to `internal/lyxcwd`.
- Rationale: the current test bans every geometry token in path-construction context anywhere outside `internal/hubgeometry`, so slice 7's core move — each module owning its own `_lyx/<module>` constant — violates it by construction. A per-token map is strictly stronger than a blanket allowlist: it encodes *who* owns each token rather than just "one package owns all of them", and it is what proves batches 1–4 actually moved ownership rather than merely copied code.
- Rejected: dropping `_lyx` from the ban entirely (lets the token spread unowned across ~20 modules). Retiring the literals guard and keeping only the `os.Getwd` ban (weakest enforcement, at exactly the moment enforcement matters most). Adding a separate import-direction test (overlaps the existing leaf tests and needs a hand-maintained exception list). Adding an exported-symbol-count assertion on the shrunk module (encodes "stays shrunk" but is brittle).

### leaf-invariant-updates

- Decision: widen `internal/lyxtest`'s leaf allowlist to include `internal/fabricengine`; widen `internal/modelspec`'s and `internal/tokenvocab`'s to include `internal/configengine` (and `internal/fabricengine` for `tokenvocab` if `reponame-derivation` is later revisited). Update all six invariants that name `internal/hubgeometry` to `internal/lyxcwd`: lyxtest Leaf, Modelspec Leaf, Treadle Runner-Seam (which names it in a *negative* clause), Tokenvocab Leaf, Scoutengine Leaf, Pattern Leaf.
- Rationale: `lyxtest` calls `WeftSiblingPath`, `ConfigDir`, `ConfigFile` and `LyxDirName` today; building synthetic hubs is inherently Fabric-shaped work, so the import is honest. `modelspec` calls `ConfigFile` (`modelspec/load.go:23`), which now lives in `configengine`. The rename ripple is mechanical but must not be missed — a stale package name in a leaf allowlist silently stops enforcing.
- Rejected: letting `lyxtest`/`modelspec` hardcode their own literals (scatters exactly the tokens this slice is centralizing per-owner, and `lyxtest.go` is production code so the literals guard scans it).

### batching

- Decision: five batches. (1) module rename + `Location` reshape + strict gate + `.lyx-anchor` rename + `ConfigDir`/`ConfigFile`/`DotEnv` move + leaf-allowlist and invariant-name updates. (2) the ~20 per-module constructors into their owning modules. (3) `Weft*`/`Host*`/junction/portal/launcher move into `fabricengine` + the seven leak fixes + `BoardDir` export. (4) `Prime`/`List`/`worktreelist.go` move + the three consumer fixes. (5) `enforcement_test.go` ownership-map rewrite + `CONSTRAINTS.md` + `docs/overview.md` + design-doc updates.
- Rationale: each batch must leave `go build ./...` and the full suite green. Batch 1 carries the rename, so it must land first and touch every importer's import block once. Batch 3 has the widest blast radius and is isolated. Batch 5 lands last because it is the guard that proves batches 1–4 moved ownership rather than copying it — running it earlier would fail against a half-moved tree.
- Rejected: three batches merging (1)+(2) and (3)+(4) (bigger blast radius per implementer pass). One atomic batch (24+ packages with a red tree throughout).

## Technical context

Current module layout: `internal/hubgeometry/hubgeometry.go` (591 lines — `Layout`, `Resolve`, `ResolveWorktree`, `resolveCore`, `SiblingLayout`, and ~30 path constructors), `anchor.go` (53 — `FabricAnchorName`, `ErrCwdOutsideAnchor`, `readRecordedAnchor`), `worktreelist.go` (78 — `List`, `WorktreeEntry`, `parseWorktreePorcelain`), plus `enforcement_test.go` (453) and 14 other test files.

Post-shrink target is roughly 120–150 lines total: `Getwd`, `Resolve`, `ResolveWithAnchor`, `ResolveWorktree`, `SiblingLayout`, `Location` + two accessors, `AnchorFileName`/`readRecordedAnchor`/private `boardDir()`, and the two error sentinels. Dependencies must stay stdlib + `internal/gitexec` only — that is what keeps the `logger` cycle closed.

Call-site weight before the move (grep counts include tests): `Layout` 233, `Resolve` 157, `ConfigFile` 115, `ConfigDir` 85, `LyxDirName` 76, `BoardDir` 73, `Getwd` 41, then a long tail. 24+ packages import the module.

Field consumers outside `hubgeometry`/`fabric*`/`lyxtest`, which is what justifies keeping each coordinate: `Cwd` ~25 files (mostly `*cli` packages passing a base to `configengine.Load`); `WorktreeRoot` ~15 (`logger/sink.go`, `shuttleengine/rundir.go`, `reedengine/lock.go`, `websterengine/runlevel.go`, `perchengine`, `burlerengine`, `scoutengine/refs.go`, `configcli` — all worktree-wide singletons); `Hub` ~11 (`reedengine/lock.go`+`server.go`, `boardcli`, `vscode/color.go`, `scoutcli`, `tokenvocab`, `ideengine/menu.go` — hub-wide singletons, one reed server per hub); `RelPath` ~10 (four of which are weft-adjacent and disappear with the leak fixes); `Repo` exactly 1; `Prime` 2.

Key files to read before implementing: `internal/hubgeometry/hubgeometry.go:102-185` (`resolveCore` and the gate), `internal/hubgeometry/anchor.go:42-53` (`readRecordedAnchor`), `internal/fabricengine/clone.go:137-182` (board worktree materialization then marker adopt-or-create), `internal/fabricengine/junctionnames.go:20-34` (`filterHubReserved`), `internal/fabricengine/config.go` (`Config.Dirs()`, `LoadConfig`), `internal/configengine/config.go:20-50` (`FindBaseDir`, `Load`), `internal/logger/sink.go:70-85` (the cycle-forcing call), `internal/hubgeometry/enforcement_test.go:212-453` (the guard being rewritten).

Gotcha: `fabricengine.LoadConfig` is the one module config anchored at `BoardDir(hub)` rather than a per-worktree base, because fabric's junction pathspec must be one repo-wide fact. Do not "normalize" it to the per-worktree convention.

Gotcha: `internal/fabricengine/list.go:10-18` already type-aliases `WorktreeEntry` and wraps `List` — batch 4 collapses the alias rather than creating a new one.

Gotcha: `SiblingLayout` has an unchecked precondition (worktree must be a direct child of the hub) documented at `hubgeometry.go:164-168`. Under `Location` that precondition becomes structurally true, since the struct stores `HubPath` + `WorktreeName` rather than a free-standing path.

## Constraints

From `CONSTRAINTS.md`:

- **Hub Geometry Invariant** — retired and replaced by this task. `CONSTRAINTS.md:5-18` already holds the narrow replacement text; it needs the module renamed to `internal/lyxcwd`, the `_board`/`-HUB` dual ownership recorded, the strict-equality gate stated, and the enforcement pointer updated. The heading should become the **Cwd Resolution Invariant**, since that is what the rewritten `enforcement_test.go` polices.
- **lyxtest Leaf Invariant**, **Modelspec Leaf Invariant**, **Tokenvocab Leaf Invariant**, **Scoutengine Leaf Invariant**, **Pattern Leaf Invariant**, **Treadle Runner-Seam Invariant** — all six name `internal/hubgeometry` and all six need the new package name; three also need widened allowlists per `leaf-invariant-updates`.
- **Test Tier Purity Invariant** — untagged tests spawn nothing. Any new test that needs a real worktree must carry an `integration`/`smoke`/`scout` build tag; the strict-gate and anchor-rename tests are prime candidates to be written untagged with pure string math instead.
- **Hermetic Git Test Environment Invariant** — any test package newly spawning git needs a `TestMain` calling `lyxtest.HermeticGitEnv()`.
- **Fabric Git Invariant (warp + weft)** — unchanged by this task; warp stays ordinary git.
- **Documentation Lifecycle** — this task changes cross-cutting infrastructure, so `manifest/designs/fabric-unified-view.md`, `docs/overview.md` and `CONSTRAINTS.md` update in the same commits as the code.

Discovered during discussion:

- Module dependency ceiling: `internal/lyxcwd` may import only stdlib and `internal/gitexec`. Any other import risks the `fabricengine → logger → lyxcwd` chain becoming a cycle.
- `docs/overview.md`'s "Hub Geometry Invariants" section needs rewriting to the narrower contract; its "Junction model" section is already accurate and must not be touched.

## Testing

TDD candidates, in the order they should be written:

- **Strict anchor gate** (`internal/lyxcwd`) — table test over `(cwd, anchorRel, worktreePath)` triples: exact match resolves; a subdirectory of the anchor errors; a parent of the anchor errors; a sibling errors. Pure string math, no git spawn, so untagged. This is the behaviour change most likely to be got wrong.
- **`Location` accessors** — `WorktreePath()` and `AnchorPath()` over hub/name/anchor combinations including `AnchorRel == "."`, and a Windows-separator case. Untagged.
- **`RepoName` derivation** — `-HUB` suffix present, absent, and a directory literally named `-HUB`. Untagged.
- **Anchor fallback** — no marker yields `AnchorRel == "."`; empty/whitespace-only marker is treated as absent; unreadable marker is treated as absent. Untagged with a temp dir.
- **Stale-marker detection** (`internal/fabricengine`) — a `_board` containing `.fabric-anchor` but no `.lyx-anchor` produces the "re-clone required" error, not a silent `"."` fallback. This is the guard against the no-fallback rename failing silently.
- **`filterHubReserved` exception** (`internal/fabricengine`) — `_board` survives the filter while `_portals`, `_launchers`, `_raddle` are still dropped; the host link name is `.board` and never collides with the hub directory.
- **Rewritten `TestEnforcement_GeometryLiterals`** — keep the existing `predicate` sub-test shape (synthetic positive/negative Go snippets parsed with `go/parser`, whole-token matching so `_boardroom`/`-weft-bare` stay negatives) and extend it per token/owner. Keep the `scanned_non_empty` sanity sub-test; a misconfigured walk must not produce a vacuous pass.

Scenarios that must be covered somewhere, tagged as needed:

- `ResolveWithAnchor` produces the same `Location` as `Resolve` when the marker on disk matches the injected anchor.
- Clone still succeeds end-to-end with the renamed marker, and a re-clone adopts the recorded anchor rather than re-anchoring.
- Each relocated per-module constructor resolves to the same absolute path it did before the move — a straightforward before/after table per module, which is what makes batch 2 safe to review.
- The seven leak-fixed call sites still produce identical user-visible output.

Existing test files in `internal/hubgeometry` that test departing symbols (`weft_test.go`, `planpath_test.go`, `discussionpath_test.go`, `loomstatus_test.go`, `scoutdaemon_test.go`, `webstergeom_test.go`, `pattern_test.go`, `raddle_guard_test.go`, `worktreelist_test.go`, `worktreelogs_test.go`, `siblinglayout_test.go`) move with their symbols rather than being deleted — losing that coverage would be the silent cost of this refactor.

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
