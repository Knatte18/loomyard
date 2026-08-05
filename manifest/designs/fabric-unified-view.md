# fabric: unified-repo view — the single entry-portal that makes warp+weft look like one repo

> **Status: mostly shipped; four new slices identified 2026-08-05, not built.** The original 6-slice campaign (clone/commit/snapshot/rebase mechanics) landed in full except one open half of slice 6 (see Open questions). A 2026-08-05 discussion, prompted by GitHub issue #127, found that `internal/hubgeometry` still carries far more than the illusion needs — this doc now also covers slices 7-10, the next campaign. Per the [documentation lifecycle](../../docs/overview.md#documentation-lifecycle), durable parts fold into `internal/fabricengine`'s and `internal/hubgeometry`'s package docs as each slice lands; this file is deleted once slice 10 and the still-open half of slice 6 are both done.

## The redefined scope: fabric is the one-repo illusion portal, nothing more

`fabric` operates two git repos (warp and weft) but its whole job is to make it look, from the outside, like there is **one flat repo**. `fabric` owns topology + wiring — clone, worktree-add, branch pairing, junction wiring, all of it; there is no separate init phase. Session bootstrap (seeding `_lyx` *content*) stays `loom`'s. `fabric`'s git-API is deliberately simplified and is not for humans: every git operation LYX's own code or an LLM agent LYX drives performs, warp or weft, goes through `Fabric.Commit`/`internal/fabricengine` — an agent or Go caller inside lyx never has a reason to know two repos exist back there. Only an external actor (a human at their own keyboard, or a tool outside LYX) keeps plain `git` in their own warp worktree; that is what makes the whole illusion feasible in the first place (see CONSTRAINTS.md's Fabric Git Invariant).

## Shipped foundation (slices 1-6, 2026-07/08) — compacted

The original campaign restructured `fabricengine` in place (never a parallel `FabricV2` package) across six landed slices:

1. **Config-driven junction list.** The weft-backed junction name-set (`_lyx`/`_pattern`) moved from a hardcoded list into `fabric.yaml`'s `pathspec` key. `hubgeometry` stayed config-blind and the sole owner of path *construction*; `fabricengine` injects the name-set as an explicit `[]string`. Hub-structural entries (`_board`, `_portals`, `_launchers`, `_raddle`) stayed hardcoded via `hubgeometry.HubReservedNames()` — they are composed at the hub level, not per-worktree weft junctions. See `internal/fabricengine/junctionnames.go`.
2. **`Fabric.Commit` (classify+dispatch) + unified `Fabric.Diff`/`Status`.** The single entry point every file-writing caller now uses; warp-first ordering, report-not-rollback partial-failure semantics (`CommitResult` + `*PartialCommitError`), detached async push. `lyx fabric diff` wraps `Fabric.Diff`. See `internal/fabricengine/commit.go`, `doc.go`.
3. **Warp-side commit lock + push coalescing.** A warp-side write lock symmetric with weft's, and `fabricengine.CoalescePush` (which `internal/boardengine.Sync` now delegates to instead of a parallel implementation).
4. **Snapshot-as-trailer.** Retired `gitrepo`'s separate `refs/loomyard/snapshot/<key>` mechanism; `Fabric.Commit`'s `snapshotTags` writes a `Snapshot:` trailer alongside `Warp-SHA`, including the empty-commit rule for a tags-only call with nothing else to commit.
5. **Clone-does-everything + subpath-in-weft + `init` dissolution.** `lyx init` is gone. The lyx-anchor subpath is recorded once on `weft:main` as a plain `.fabric-anchor` marker at `hubgeometry.BoardDir(Hub)`; `hubgeometry.Resolve` reads it (record wins over cwd), validates cwd is at or below the anchored subtree, and falls back to cwd-derived `RelPath` only when the marker is absent. `CloneHub`/`worktree add` wire junctions, create `_lyx`, and reconcile config in one call. See `internal/hubgeometry/anchor.go`, `internal/fabricengine/clone.go`.
6. **Warp-rebase / remote-reconcile — fabric-layer half.** Detection via ancestry (`gitrepo.IsAncestor`, never `SHAExists`) + correspondence re-anchor + a `PullResult` PATTERN-residue document, via `Fabric.Pull`/`lyx fabric pull`. **The orchestration-layer half stays open** — see Open questions.

Full shipped-behavior detail for all six lives in `internal/fabricengine/doc.go`'s package comment, not repeated here.

## Slices 7-10 (2026-08-05 discussion, not built) — hubgeometry stops being a path authority

GitHub issue #127 asked whether `internal/hubgeometry` as a standalone path-authority module still earns its keep, now that fabric already sets the anchor (slice 5) and already owns the junction name-set (slice 1). Investigation during discussion confirmed: no — most of `internal/hubgeometry` (591 lines) is either (a) genuinely Fabric's illusion-plumbing that leaked into a shared package, or (b) ~20 per-module path constructors (`PlanDir`, `BuilderDir`, `WebsterDir`, `DiscussionDir`, `PatternDir`, etc.) that make hubgeometry a bottleneck every module extension must go through, exactly issue #127's original complaint.

### Slice 7 — shrink `hubgeometry` to the minimal illusion primitive; Fabric absorbs the rest

**What every consumer actually needs, and no more:** `Cwd` and one opaque root path (working name in discussion: `worktreePath` — final Go identifier is an implementation decision, not pinned by this doc). No consumer needs to know weft exists. `hubgeometry.Resolve` supplies `Cwd` + root + (when an anchor is recorded) `RelPath`; a caller needing an absolute path joins root+RelPath+its own registered relative subpath itself.

**Naming discipline for whoever implements this (and every reviewer of it):** "root" means the worktree/repo root, always — `WorktreeRoot`/`Hub`/`WeftRepoRoot`, the handful of genuine root concepts above. It is never a stand-in name for "whatever base path a module joins its own subdirectory onto." Per point 6/point 5 of the 2026-08-05 discussion, most modules (the `WebsterDir`-style constructors above) join their own relative subdirectory directly onto **`cwd`**, not onto a "root" parameter — call it `cwd` in every such signature and doc comment, not `root`. This is not pedantry: `internal/hubgeometry` exists in the first place because cwd/root confusion was a recurring, real defect source in this codebase (including in LLM-generated code) — shrinking hubgeometry removes the module that used to absorb that confusion in one place, so the naming discipline that prevented it has to move into every call site it used to be centralized in. Get this wrong here and the failure mode is the same one hubgeometry was built to prevent, just spread across the ~20 places it now happens instead of one.

**What stays in `internal/hubgeometry`** (shrinks from ~591 lines to roughly the size of `anchor.go`+the `Resolve`/`Layout` core): `git rev-parse --show-toplevel` (`WorktreeRoot`), `Hub` (`filepath.Dir(WorktreeRoot)`, pure string math), and anchor-reading (`os.ReadFile` against `<Hub>/_board/.fabric-anchor`; a plain file read, no git subprocess, because that directory is always a materialized weft:main worktree, never something requiring `git show`). This keeps the package dependency-free (stdlib + `gitexec` only) — **load-bearing, not cosmetic**: `internal/fabricengine` imports `internal/logger` (for its own tracing), and `internal/logger/sink.go` calls `hubgeometry.Resolve` directly to anchor its log directory regardless of invocation-time cwd depth. If the core resolver moved bodily into `fabricengine`, `fabricengine → logger → fabricengine` would be an import cycle. `logger` keeps calling the shrunk `hubgeometry` directly — a documented infrastructure exception to "every module asks Fabric," not a violation of it (below `fabricengine` in the dependency graph, not a peer of ordinary consumer modules).

**`Prime`/`List`-based sibling-worktree resolution moves into `fabricengine`**, private. It is not per-worktree data at all — `List(cwd)` (a `git worktree list` subprocess) runs on every single `Resolve()` call today just to find the entry flagged `Main`, even though the result is identical for every worktree under one hub. Ordinary callers never needed it; only `fabricengine`'s own weft-sibling-path derivation (`WeftRepoRoot`) does. Moving it out also removes the wasted subprocess spawn from the hot consumer-facing resolve path.

**The ~20 per-module path constructors move to their owning modules** as private, relative-path constants (e.g. `websterengine.WebsterDir(cwd)` returns `filepath.Join(cwd, "_lyx", "webster")` locally, not a call into `hubgeometry`). This is not new plumbing — the junction (`_lyx`) these subdirectories live under is already config-driven (slice 1); what changes is only that a module no longer needs a `hubgeometry` code change to add its own subdirectory under an already-wired junction.

**`Weft*`/`Host*Link`/junction-construction methods** (`WeftWorktree`, `WeftRepoRoot`, `HostLyxLink`, `HostJunctions`, `PortalLink`, `LauncherDir`, etc.) move into `fabricengine`, private — they are Fabric's own illusion-maintenance plumbing, never part of the public "ask Fabric for cwd" contract.

**Keep the name `_board` — renaming was considered (2026-08-05) and dropped as unnecessary churn.** `hubgeometry.BoardDir` (`<Hub>/_board`, a real `git worktree add` of weft on branch `main` — not a junction) hosts more than board's own data: `.fabric-anchor` lives there, and the repo-wide `fabric.yaml` lives there (`<BoardDir>/_lyx/config/fabric.yaml`, via the same generic `configengine.Load(baseDir, module, …)`/`<baseDir>/_lyx/config/<module>.yaml` convention every module's config uses — `fabricengine.LoadConfig` is just the one caller that fixes `baseDir = BoardDir(hub)` instead of the usual per-worktree cwd, because fabric's junction pathspec must be one repo-wide fact, not a per-worktree copy; today it is the only module config anchored at `_board` rather than the ordinary per-worktree `_lyx/config/`). `_lyxharness`/`_system`/`_registry` were all considered as renames and rejected — `_lyxharness` read as more confusing than the status quo on reflection, `_system`/`_registry` as too OS-generic. `_board` stays `_board`; nothing here forces `internal/boardengine`'s own feature data to move.

**New requirement, 2026-08-05: a junction from every worktree's `cwd` into `_board`**, on the same model as millhouse's own `.wiki` junction (every mill worktree reaches the shared wiki clone through a `.wiki` link) — because `_board` is where `internal/boardengine`'s manifest/task data lives, and it should be reachable from inside a normal worktree the same way. Not yet scoped: the junction's name in `cwd`, whether it is added by slice 7 or its own slice, and how it interacts with the `_board` worktree already being reachable today via its own absolute hub-level path (`hubgeometry.BoardDir`) for callers that don't want a cwd-relative link.

**CONSTRAINTS.md**: retire the Hub Geometry Invariant in its current ("hubgeometry owns all geometry/paths") form; record the narrower replacement (hubgeometry owns only cwd/root/anchor resolution; Fabric owns weft-sibling and junction plumbing; each module owns its own relative subpath). **`docs/overview.md`**: the "Hub Geometry Invariants" section and the `enforcement_test.go`-described ban on raw `os.Getwd`/`git rev-parse --show-toplevel` outside `hubgeometry`+`cmd/lyx/main.go` stays in spirit (still true, still enforced) but the surrounding prose describing hubgeometry's scope needs rewriting to the narrower contract; the "Junction model" section's description of the config-driven pathspec is already accurate and does not change.

**Execution model: full discussion/plan/mill-go pipeline**, not manual/mechanical — this touches 24+ importing packages and retires a CI-enforced invariant.

### Slice 8 — close the weft-visibility leak

Two concrete leaks found by grepping actual call sites, not by inspection alone:

- **Seven files outside `fabricengine`/`fabriccli` call `Layout`'s `Weft*`/`Host*Link` methods directly**: `internal/websterengine/audit.go`, `internal/buildercli/weft.go`, `internal/perchcli/run.go`, `internal/webstercli/weft.go`, `internal/builderengine/spawn.go`, `internal/loomengine/preflight.go`, `internal/lyxtest`. All read-only (reporting/audit, not independent weft git operations) — `buildercli`/`perchcli` build "committed to weft" output text; `websterengine/audit.go` builds a regex to scrub the weft path *out* of logs. Fix: route these through Fabric's own operation return values (e.g. a commit result's own fact about which side was touched) instead of an independent `Layout` query — none of them need `WeftWorktree()` itself once Fabric's return values are complete enough.
- **Open decision, not yet made**: should `buildercli`/`perchcli`/`webstercli`'s CLI output ever say "weft" to the end user at all? The illusion (per this doc's own framing) says no; today's shipped code says yes. Resolve explicitly in this slice's discussion phase, not silently.

Depends on slice 7 (routes through Fabric's new, narrower return-value contract). File-disjoint from slices 9 and 10 — can run in parallel with either.

### Slice 9 — `.lyx` hygiene (relocate transients, fix `.lyx`'s own junction geometry)

Carries forward the parked `dotlyx-scratch-hygiene` task, narrowed by slice 7 landing first:

- **Relocate misplaced never-tracked transients** `_lyx` → `.lyx`: perch's run/mutate locks, webster's pause flag + rendered fork prompts, builder's pause flag. Unchanged by slice 7 — purely "which directory does this file belong in," independent of the path-authority rework.
- **Fix `.lyx`'s own geometry.** Today a committed `.gitignore` `.lyx/` block keeps it out of the host repo (wrong — a tracked artifact for a host→weft junction in the *user's own* repo). Once slice 7 lands, this shrinks to: register `.lyx` through the same config-driven junction mechanism (slice 1, unchanged by slice 7) every other weft-backed directory already uses, replacing the committed `.gitignore` block with the warp `.git/info/exclude` seeding pattern `fabric-collapse-external-surface` already established as the interim guard. No bespoke fix needed — `.lyx` becomes one more entry in the existing pathspec, not a special case.
- Remove `crossModuleMachineLocalExcludes` / the `_lyx`-transient portion of `seedWeftArtifactExcludes` (`weftgit.go:64-101,103-116`) once the transients move out; retire the corresponding CONSTRAINTS "Cross-module exclusions" mechanism.

Depends on slice 7. **Sequenced before slice 10** (not parallel) — both touch `internal/fabriccli/clone.go`'s `runCloneWithReset` in the same ~45-line span (arg parsing and the `gitignore.Ensure(".lyx/")` call sit a few lines apart in that one function); geometry-fix-before-feature avoids building slice 10 on a clone.go structure this slice is about to change underneath it.

### Slice 10 — store the warp-URL binding in `weft:main`; fold bootstrap into `fabric clone`

A weft repo is bound to exactly one warp (many-to-one: one warp can back many wefts). Store that binding (warp URL + `--subpath`) on **weft:main** via the `Bolt` handle (`fabric-collapse-external-surface`), the same weft:main area slice 5's `.fabric-anchor` already lives in — **a distinct piece of data from the subpath anchor**: the anchor says *where in warp* lyx is rooted; this binding says *which warp repo* this weft pairs with at all.

No new verb — `fabric clone` takes the optional warp URL directly:

```
lyx fabric clone <weft-url>                 # binding already exists on weft:main — warp derived, as today
lyx fabric clone <weft-url> <warp-url>      # bootstrap — weft has no binding yet, warp given explicitly
```

**Breaking change: `clone`'s argument order flips to weft-first** (today warp-first: `internal/fabriccli/clone.go` reads `hostURL := args[0]; weftURL := args[1]`). Update `runCloneWithReset`'s arg parsing, `fabricengine.CloneHub`'s parameter order, every call site/help text/test, and `docs/`/this file's own examples above (which still show today's order and must be corrected in the same commit as the flip).

**Conflict rule**: if `<warp-url>` is supplied and a binding already exists — matches: no-op, proceed. Differs: hard error, never silently re-point (re-pointing is a distinct, deliberate operation). No `<warp-url>` supplied and no binding recorded: error, unbound weft requires an explicit warp URL.

Depends on slice 7 (builds the new bootstrap logic against slice 7's already-narrowed clone/junction structure, not the pre-slice-7 one) and slice 9 (clone.go collision, see above — sequenced after).

## Warp stays ordinary git — preserved, unaffected by slices 7-10

Plain `git`/rebase/amend/force-push stays available in warp for external actors (a human, or any tool outside LYX/LoomYard) who don't know weft exists — `fabric`'s job is to stay correct under arbitrary external warp activity, not to be the only entrance for the whole world. The correspondence link is one-directional (weft records `Warp-SHA` pointing at warp, recorded at weft-commit time), so warp never needs to route through fabric for correspondence to hold. This is not a carve-out for LYX's own code or LLM agents LYX drives — for those, every git operation goes through `Fabric.Commit`/`internal/fabricengine`, no exception (CONSTRAINTS.md's Fabric Git Invariant).

## Scope boundary — still not a general-purpose git wrapper

`gitrepo` deliberately excludes rebase, interactive staging, cherry-pick, conflict resolution — a human always has plain git in either working tree. What fabric wraps is the set that affects the two-repo illusion and correspondence: commit/push/pull/sync, clone/worktree/branch topology, unified diff/status, and (per CONSTRAINTS.md's Fabric Git Invariant) single-sided warp-only mutating verbs that need to dispatch through `internal/fabricengine`. Read-only verbs the caller can run directly.

## Open questions

- **Weft remote provisioning at first clone** — still open. First-ever setup needs a weft remote to push to (empty is fine); either require it pre-created, or have clone provision it.
- **Slice 6's orchestration-layer half** — still open. `Fabric.Pull` (fabric-layer detection + re-anchor + `PullResult` residue) is shipped; which layer drives pull → conflict-resolve → raddle-regen, and how `PullResult`/`PATTERN` re-alignment is presented to an LLM resolving agent, stays open until `loom`/`Shed` exist to consume it.
- **Slice 8's CLI-wording question** — new, see slice 8 above: should "weft" ever appear in user-facing CLI output?
- **`worktreePath` naming** — new, see slice 7 above: `worktreePath` was this discussion's working label for hubgeometry's public root field; not pinned as the final Go identifier.

## Related

- `internal/fabricengine` (doc.go), `internal/hubgeometry` (hubgeometry.go, anchor.go) — the shipped bases slices 7-10 restructure; durable parts fold here on landing.
- [finalize.md](finalize.md) — the document-driven weft-conflict mechanism slice 6's orchestration half will reuse.
- [raddle.md](raddle.md) — the regenerate-don't-merge property bounding rebase recovery; the snapshot-staleness consumer slice 4 serves.
- [host-visibility.md](host-visibility.md) — the narrower sibling illusion (`CLAUDE.local.md`), same junction mechanism slice 1 generalized.
- [pattern.md](pattern.md) — hand-authored weft content; a `_pattern` junction consumer of the slice-1 config-driven list; also the residue of rebase re-alignment.
- `fabric-v2-crucible` (wiki) — the final hardening slice, sequenced after every slice in this doc including 7-10, per project policy that it runs last.
- CONSTRAINTS.md's Fabric Git Invariant and Hub Geometry Invariant — the invariants this doc's shipped work enforces and slice 7 narrows.
