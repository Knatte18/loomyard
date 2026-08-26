# fabric: unified-repo view — the single entry-portal that makes warp+weft look like one repo

> **Status: slices 1-5 and 7-10 shipped; slice 6's orchestration half is the one open item.** The original 6-slice campaign (clone/commit/snapshot/rebase mechanics) landed in full except that half (see Open questions). A 2026-08-05 discussion, prompted by GitHub issue #127, found that the then-`internal/hubgeometry` module carried far more than the illusion needs; slices 7-10 addressed that and have all landed — the shrunk resolver is now `internal/lyxcwd`. Per the [documentation lifecycle](../../docs/overview.md#documentation-lifecycle), durable parts fold into `internal/fabricengine`'s and `internal/lyxcwd`'s package docs as each slice lands; this file is deleted once the still-open half of slice 6 is done.

## The redefined scope: fabric is the one-repo illusion portal, nothing more

`fabric` operates two git repos (warp and weft) but its whole job is to make it look, from the outside, like there is **one flat repo**. `fabric` owns topology + wiring — clone, worktree-add, branch pairing, junction wiring, all of it;
there is no separate init phase.
Session bootstrap (seeding `_lyx` *content*) stays `loom`'s. `fabric`'s git-API is deliberately simplified and is not for humans: every git operation LYX's own code or an LLM agent LYX drives performs, warp or weft, goes through `Fabric.Commit`/`internal/fabricengine` — an agent or Go caller inside lyx never has a reason to know two repos exist back there.
Only an external actor (a human at their own keyboard,
or a tool outside LYX) keeps plain `git` in their own warp worktree;
that is what makes the whole illusion feasible in the first place (see CONSTRAINTS.md's Fabric Git Invariant).

## Shipped foundation (slices 1-6, 2026-07/08) — compacted

The original campaign restructured `fabricengine` in place (never a parallel `FabricV2` package) across six landed slices:

> **Superseded:** `_pattern` no longer exists as a junction, and `_raddle` is anchor-level, never junction-reached — see `manifest/designs/raddle.md`.
> Item 1's `_pattern` mention below is historical narrative describing the state as of when this slice landed;
> its `HubReservedNames()` token set has since narrowed.

1. **Config-driven junction list.**
   The weft-backed junction name-set (`_lyx`/`_pattern`) moved from a hardcoded list into `fabric.yaml`'s `pathspec` key. The then-`hubgeometry` resolver stayed config-blind and the sole owner of path *construction*;
   `fabricengine` injects the name-set as an explicit `[]string`.
   Hub-structural entries (`_board`, `_portals`, `_launchers`) stay hardcoded via `fabricengine.HubReservedNames()` — they are composed at the hub level, not per-worktree weft junctions.
   See `internal/fabricengine/junctionnames.go`.
2. **`Fabric.Commit` (classify+dispatch) + unified `Fabric.Diff`/`Status`.**
   The single entry point every file-writing caller now uses;
   warp-first ordering, report-not-rollback partial-failure semantics (`CommitResult` + `*PartialCommitError`), detached async push. `lyx fabric diff` wraps `Fabric.Diff`.
   See `internal/fabricengine/commit.go`, `doc.go`.
3. **Warp-side commit lock + push coalescing.**
   A warp-side write lock symmetric with weft's, and `fabricengine.CoalescePush` (which `internal/boardengine.Sync` now delegates to instead of a parallel implementation).
4. **Snapshot-as-trailer.**
   Retired `gitrepo`'s separate `refs/loomyard/snapshot/<key>` mechanism;
   `Fabric.Commit`'s `snapshotTags` writes a `Snapshot:` trailer alongside `Warp-SHA`, including the empty-commit rule for a tags-only call with nothing else to commit.
5. **Clone-does-everything + subpath-in-weft + `init` dissolution.** `lyx init` is gone.
   The lyx-anchor subpath is recorded once on `weft:main` as a plain marker at `fabricengine.BoardDir(Hub)` — spelled `.fabric-anchor` when this slice landed, renamed to `.lyx-anchor` since, because the marker anchors the whole repo rather than the fabric module.
   `lyxcwd.Resolve` reads it (record wins over cwd), requires cwd to equal the anchored directory exactly (`ErrCwdOutsideAnchor` otherwise), and falls back to `"."` only when the marker is genuinely absent — a board still carrying only the pre-rename spelling is a hard error (`lyxcwd.ErrStaleAnchorMarker`), never a silent re-anchor at the repo root. `CloneHub`/`worktree add` wire junctions, create `_lyx`, and reconcile config in one call.
   See `internal/lyxcwd/anchor.go`, `internal/fabricengine/clone.go`.
6. **Warp-rebase / remote-reconcile — fabric-layer half.**
   Detection via ancestry (`gitrepo.IsAncestor`, never `SHAExists`) + correspondence re-anchor + a `PullResult` PATTERN-residue document, via `Fabric.Pull`/`lyx fabric pull`.
   **The orchestration-layer half stays open** — see Open questions.

Full shipped-behavior detail for all six lives in `internal/fabricengine/doc.go`'s package comment, not repeated here.

## Slices 7-10 (2026-08-05 discussion; all four shipped) — the shared resolver stops being a path authority

GitHub issue #127 asked whether the shared path-authority module (then `internal/hubgeometry`, now the far narrower `internal/lyxcwd`) still earned its keep, now that fabric already sets the anchor (slice 5) and already owns the junction name-set (slice 1).
Investigation during discussion confirmed: no — most of that 591-line module was either (a) genuinely Fabric's illusion-plumbing that leaked into a shared package, or (b) ~20 per-module path constructors (`PlanDir`, `BuilderDir`, `WebsterDir`, `DiscussionDir`, `PatternDir`, etc.) that make hubgeometry a bottleneck every module extension must go through, exactly issue #127's original complaint.

### Slice 7 — shrink the shared resolver to the minimal illusion primitive; Fabric absorbs the rest (shipped)

> **Reading note:** the design prose in this section is the 2026-08-05 discussion as written, so it names `internal/hubgeometry` throughout.
> That package no longer exists;
> what shipped is `internal/lyxcwd`, and the "Shipped correction" notes below record where the as-built result differs from the plan.

**What every consumer actually needs, and no more:** `Cwd` and one opaque root path (working name in discussion: `worktreePath` — final Go identifier is an implementation decision, not pinned by this doc).
No consumer needs to know weft exists. The resolver supplies `Cwd` + root + (when an anchor is recorded) `RelPath`;
a caller needing an absolute path joins root+RelPath+its own registered relative subpath itself.

**Naming discipline for whoever implements this (and every reviewer of it):** "root" means the worktree/repo root, always — `WorktreeRoot`/`Hub`/`WeftRepoRoot`, the handful of genuine root concepts above.
It is never a stand-in name for "whatever base path a module joins its own subdirectory onto."
Per point 6/point 5 of the 2026-08-05 discussion, most modules (the `WebsterDir`-style constructors above) join their own relative subdirectory directly onto **`cwd`**, not onto a "root" parameter — call it `cwd` in every such signature and doc comment, not `root`.
This is not pedantry: `internal/hubgeometry` exists in the first place because cwd/root confusion was a recurring, real defect source in this codebase (including in LLM-generated code) — shrinking hubgeometry removes the module that used to absorb that confusion in one place, so the naming discipline that prevented it has to move into every call site it used to be centralized in.
Get this wrong here and the failure mode is the same one hubgeometry was built to prevent, just spread across the ~20 places it now happens instead of one.

**Shipped correction (slice 7, updated slice 9): "joins onto `cwd`" above is not what landed.**
The as-built anchoring table — recorded in the plan's Shared Decisions — is anchor-aware, not a single blanket base: the durable, weft-synced, git-tracked `_lyx` group (`PlanDir`, `DiscussionDir`, `WebsterDir`, `PatternDir`,
and the rest) joins onto `Location.AnchorPath()`, not `cwd` directly;
the ephemeral, machine-bound, never-git-tracked `.lyx` group (`logger.LogsDir`, renamed from `WorktreeLogsDir`; `ScoutDaemonStateFile`, `ScoutDaemonLock`, `LoomStatusFile`) also joins onto `Location.AnchorPath()` as of slice 9, no longer `Location.WorktreePath()` — the two groups now share one anchoring rule, so a subpath-anchored repo has exactly one `.lyx` root instead of two;
and `HubLogsDir` alone joins onto `fabricengine.HubScratchDir(Location.HubPath)`, deliberately hub-anchored so one reed server per hub resolves to one deterministic place.
A blanket "join onto `cwd`" would have silently relocated the last three.
The two docs must not be allowed to disagree — re-read both after editing either.

**What stays in the shrunk resolver, shipped as `internal/lyxcwd`** (down from ~591 lines to roughly the size of `anchor.go`+the `Resolve`/`Location` core): `git rev-parse --show-toplevel`, the hub (`filepath.Dir(worktree root)`, pure string math), and anchor-reading (`os.ReadFile` against `<Hub>/_board/.lyx-anchor`;
a plain file read, no git subprocess, because that directory is always a materialized weft:main worktree, never something requiring `git show`).
This keeps the package dependency-free (stdlib + `gitexec` only) — **load-bearing, not cosmetic**: `internal/fabricengine` imports `internal/logger` (for its own tracing), and `internal/logger/sink.go` calls the resolver directly to anchor its log directory regardless of invocation-time cwd depth.
If the core resolver moved bodily into `fabricengine`, `fabricengine → logger → fabricengine` would be an import cycle. `logger` keeps calling the shrunk resolver directly — a documented infrastructure exception to "every module asks Fabric," not a violation of it (below `fabricengine` in the dependency graph, not a peer of ordinary consumer modules).

**`Prime`/`List`-based sibling-worktree resolution moves into `fabricengine`**, private.
It is not per-worktree data at all — `List(cwd)` (a `git worktree list` subprocess) runs on every single `Resolve()` call today just to find the entry flagged `Main`, even though the result is identical for every worktree under one hub.
Ordinary callers never needed it;
only `fabricengine`'s own weft-sibling-path derivation (`WeftRepoRoot`) does.
Moving it out also removes the wasted subprocess spawn from the hot consumer-facing resolve path.

**The ~20 per-module path constructors move to their owning modules** as private, relative-path constants (e.g. `websterengine.WebsterDir(cwd)` returns `filepath.Join(cwd, "_lyx", "webster")` locally, not a call into `hubgeometry`).
This is not new plumbing — the junction (`_lyx`) these subdirectories live under is already config-driven (slice 1);
what changes is only that a module no longer needs a `hubgeometry` code change to add its own subdirectory under an already-wired junction.

**`Weft*`/`Warp*Link`/junction-construction methods** (`WeftWorktree`, `WeftRepoRoot`, `WarpLyxLink`, `WarpJunctions`, `PortalLink`, `LauncherDir`, etc.) move into `fabricengine`, private — they are Fabric's own illusion-maintenance plumbing, never part of the public "ask Fabric for cwd" contract.

**Keep the name `_board` — renaming was considered (2026-08-05) and dropped as unnecessary churn.** `fabricengine.BoardDir` (`<Hub>/_board`, a real `git worktree add` of weft on branch `main` — not a junction) hosts more than board's own data: the recorded lyx-anchor marker `.lyx-anchor` lives there (spelled `.fabric-anchor` when this decision was taken — see item 5 above for the rename), the `.lyx-warp` warp-URL binding lives beside it,
and the repo-wide `fabric.yaml` lives there (`<BoardDir>/_lyx/config/fabric.yaml`, via the same generic `configengine.Load(baseDir, module, …)`/`<baseDir>/_lyx/config/<module>.yaml` convention every module's config uses — `fabricengine.LoadConfig` is just the one caller that fixes `baseDir = BoardDir(hub)` instead of the usual per-worktree cwd, because fabric's junction pathspec must be one repo-wide fact, not a per-worktree copy;
today it is the only module config anchored at `_board` rather than the ordinary per-worktree `_lyx/config/`). `_lyxharness`/`_system`/`_registry` were all considered as renames and rejected — `_lyxharness` read as more confusing than the status quo on reflection, `_system`/`_registry` as too OS-generic. `_board` stays `_board`;
nothing here forces `internal/boardengine`'s own feature data to move.

**New requirement, 2026-08-05: a junction from every worktree's `cwd` into `_board`**, on the same model as millhouse's own `.wiki` junction (every mill worktree reaches the shared wiki clone through a `.wiki` link) — because `_board` is where `internal/boardengine`'s manifest/task data lives,
and it should be reachable from inside a normal worktree the same way.

**Shipped (batch 7): the `_board` junction landed as an operator-convenience-only link, not a pathspec entry.**
The link sits at `<AnchorPath>/_board`, targets `<HubPath>/_board`, and is wired by `internal/fabricengine` at clone, add, and (unconditionally with respect to junction health) reconcile, and removed on unwire.
Three properties are hard rules a later caller may not quietly opt out of: it is **wire-only and unmonitored** — `Healthy`, `checkJunctionHealth` and `junctionRepointedDetail` never inspect it, so a broken `_board` link can never block loom preflight;
it is **unconditionally re-wired** on every reconcile pass, since nothing diagnoses its breakage, the repair cannot be conditioned on detection;
and it is **read by no lyx code path** — every `BoardDir` consumer keeps resolving `<HubPath>/_board` directly, and all board mutation continues through `internal/boardengine`.
It is deliberately **not** added to `fabric.yaml`'s `pathspec`: `pathspec` is dual-purpose (it also feeds the raw, unfiltered weft *commit* pathspec), and `_board` is itself a weft worktree, not committable content from the warp side, so folding it into that list would be wrong, not merely awkward.

**Reversed (hub-dotlyx-into-board, 2026-08-14): the `_board` convenience junction is deleted.**
The link was pure redundancy — the board is already reachable at `<hub>/_board` and no lyx code path ever read through it (see the "read by no lyx code path" property above, which held from the day it shipped) — and it turned out to be the one thing that broke the fabric illusion from the inside: unlike `_lyx`/`.lyx`, it was neither warp nor weft, it was shared across every worktree in the hub rather than paired to one, and it was physically writable while bypassing `BoardWriteLockPath`.
millhouse's own `.wiki` junction — cited above as the model this link followed — is the empirical case for the risk: same shape, a distinctly named link into a shared store, guarded by an explicit prohibition against editing through it, and still edited by mistake anyway.
The unbuilt plan two paragraphs below to junction `_portals` and `_launchers` into every worktree is cancelled by the same invariant — see `CONSTRAINTS.md`'s Hub Containment Invariant, which now forbids junctioning any hub-level container into a worktree.

**CONSTRAINTS.md**: retire the Hub Geometry Invariant in its current ("the shared resolver owns all geometry/paths") form;
record the narrower replacement (it owns only cwd/root/anchor resolution;
Fabric owns weft-sibling and junction plumbing;
each module owns its own relative subpath). **`docs/overview.md`**: the "Hub Geometry Invariants" section and the `enforcement_test.go`-described ban on raw `os.Getwd`/`git rev-parse --show-toplevel` outside the resolver+`cmd/lyx/main.go` stays in spirit (still true, still enforced) but the surrounding prose describing the resolver's scope needs rewriting to the narrower contract;
the "Junction model" section's description of the config-driven pathspec is already accurate and does not change.

**Execution model: full discussion/plan/mill-go pipeline**, not manual/mechanical — this touches 24+ importing packages and retires a CI-enforced invariant.

**Shipped (batch 1): the `"-weft"` naming convention landed as its own leaf, `internal/weftname`** — not in `fabricengine`, despite `fabricengine` owning the rest of the weft-sibling path surface.
The reason is a compile-time cycle: `internal/lyxtest` (the fixture builder of the time, since split into `internal/gitkit`, the below-fabric git-primitives leaf, and `internal/hubforge`, the real-hub fixture factory), which built test fixtures for roughly 30 of `fabricengine`'s ~50 test files, could not import `fabricengine` to construct those fixtures with real `fabric clone`/`fabric add` calls, because `lyxtest` → `fabricengine` would have been a cycle (`fabricengine`'s own tests imported `lyxtest`).
Those ~30 files were in-package (`package fabricengine`, not `fabricengine_test`) and needed unexported access, so they could not simply be converted to the external test-package form to break the cycle — doing that properly needed an export-for-test shim across `fabricengine` first, which was a slice of its own, not a side effect of this one. `weftname.Suffix` (`"-weft"`) was therefore a stdlib-only leaf both `lyxtest` and `fabricengine` imported, letting fixtures compute a weft sibling name without either side reaching into the other.
The cycle this passage describes was subsequently broken by the `lyxtest-real-hubs` task: `hubforge` now imports `fabricengine` directly (via `fabriccli.CloneAndWire`) to build real cloned hub fixtures, and `fabricengine`'s own in-package fixture-consuming tests moved to external `package fabricengine_test` files, so the constraint that motivated pulling `weftname` out as a stdlib-only leaf is no longer live — a reader should not take this passage as describing the current import graph.

### Slice 8 — close the weft-visibility leak (shipped)

Made fabric's public surface incapable of telling anyone that warp and weft exist: `Open(l)` replaced `New(warpPath, weftPath)` as the sole constructor outside the package;
`CommitResult.Committed()` replaced direct `WeftCommitted` reads;
`Ready(l)` replaced loom preflight's raw weft `os.Stat`;
`NewRefScanner(l)`/`RefScanner.Matches` replaced webster audit's own weft-reference regex;
`Healthy` gained a typed `HealthReason` so `loomengine` classifies drift without substring-matching a display string.
Every non-owner identifier, string literal, comment, and agent prompt template mentioning `weft`/`warp`/a fabric-sense `host` was reworded or renamed, and `TestEnforcement_FabricVocabulary` now machine-checks the boundary going forward — see CONSTRAINTS.md's Fabric Vocabulary Invariant and `internal/fabricengine`'s package doc for the durable contract.
The CLI-wording question below was resolved: consumer-emitted prose says "fabric," never "weft," while the wrapped error detail — which fabric itself produces — keeps naming the weft repo and path freely.

### Slice 9 — `.lyx` hygiene (relocate transients, fix `.lyx`'s own junction geometry) (shipped)

Carried forward the parked `dotlyx-scratch-hygiene` task, narrowed by slice 7 landing first.
What actually landed:

> **Superseded:** `_pattern` no longer exists as a junction, and `_raddle` is anchor-level, never junction-reached — see `manifest/designs/raddle.md`.
> The `_pattern`/`_raddle` mentions below are historical narrative describing the state as of when this slice landed.

- **Relocated every misplaced never-tracked transient** `_lyx` → `.lyx`: the round loop's run/mutate locks, webster's pause flag + rendered fork prompts, builder's pause flag — the mirrored-subpath rule, mechanical and reviewable.
- **Fixed `.lyx`'s own geometry — but not as one more `pathspec` entry.**
  The slice's own prediction ("no bespoke fix needed — `.lyx` becomes one more entry in the existing pathspec, not a special case") turned out to be wrong, deliberately: `.lyx` shipped as a **structural, code-injected junction** (`structuralNeverCommittedDirs`, alongside `_lyx`'s `structuralCommittedDirs`), never read from `fabric.yaml`'s `pathspec`.
  Geometry is structural, never config/env-overridable — an operator-editable `pathspec` value is not where obligatory geometry may live, and folding `.lyx` into `pathspec` would have let a config edit silently strand machine-local scratch unwired.
  The committed `.gitignore` `.lyx/` block is gone, in both `clone.go` (no more `gitignore.Ensure` call) and `unwire.go` (no more `gitignore.Remove` call, no `Gitignore` field, no `gitignore` output-envelope key); `.lyx` is excluded through the warp's own `.git/info/exclude` alone, seeded by `WireJunctions` the same way every other junction is.
  A pre-existing real `.lyx` — every worktree in existence before this change, since several of lyx's own subsystems write it unconditionally — is adopted into the weft target on first `reconcile` after upgrade, rather than hard-erroring; `_lyx` and `_pattern` keep their hard refusal, since `.lyx` alone is guaranteed to be lyx's own disposable scratch.
  Upgrade is signalled through health (`CauseJunctionMissing` on an un-adopted worktree) rather than a dedicated migration step; downgrade against a pre-fix binary is unsupported (its `applyStaleRemoval` unwires `.lyx` and strands scratch), a one-way upgrade by design.
- **Unwire stopped deleting weft-side content.** `Unwire` used to `os.RemoveAll` the weft-side `_lyx` and commit `"lyx fabric unwire: clear _lyx"` to weft — an asymmetry with `_pattern`, which was always preserved.
  `Unwire` now reverses wiring only (junctions + warp exclude entries); every weft-side directory, `_lyx`/`.lyx`/`_pattern` alike, survives untouched. `UnwireVerbResult.WeftContent`'s value set is now `"preserved"` | `"not_present"`, never `"cleared"`.
- Removed `crossModuleMachineLocalExcludes` / the `_lyx`-transient portion of `seedWeftArtifactExcludes` once the transients moved out; retired the corresponding CONSTRAINTS "Cross-module exclusions" mechanism, replaced by the Durable-vs-Ephemeral State Invariant and the Fabric Git Invariant's junction-exclusion clause.
- `<hub>/.lyx` shipped as a new hub-level geometry element alongside `<hub>/_board`: a real directory (the hub is not a git repo), created by `CloneHub`, reserved so no worktree slug can claim the name.
  It was subsequently moved inside `_board`, to `<hub>/_board/.lyx`, and is no longer hub geometry of its own — see the hub-scratch-move task.

Depended on slice 7.
**Sequenced before slice 10** (not parallel) — both touch `internal/fabriccli/clone.go`'s `runCloneWithReset` in the same ~45-line span;
slice 10 has since shipped.

### Slice 10 — store the warp-URL binding in `weft:main`; fold bootstrap into `fabric clone` (shipped)

A weft repo is bound to exactly one warp (many-to-one: one warp can back many wefts).
Store that binding on **weft:main** via the `Bolt` handle (`fabric-collapse-external-surface`), the same weft:main area slice 5's `.lyx-anchor` already lives in — **a distinct piece of data from the subpath anchor**: the anchor says *where in warp* lyx is rooted;
this binding says *which warp repo* this weft pairs with at all.

**Shipped correction: the record holds the warp URL only, never `--subpath`.**
The subpath already has one authoritative home in the `.lyx-anchor` marker;
a second copy in the binding record would create two records that can disagree with no rule for which wins.

No new verb — `fabric clone` takes the optional warp URL directly:

```
lyx fabric clone <weft-url>                 # binding already exists on weft:main — warp derived, as today
lyx fabric clone <weft-url> <warp-url>      # bootstrap — weft has no binding yet, warp given explicitly
```

**Breaking change: `clone`'s argument order flipped to weft-first** (previously warp-first: `internal/fabriccli/clone.go` read `warpURL := args[0]; weftURL := args[1]`).
`runCloneWithReset`'s arg parsing, `fabricengine.CloneHub`'s parameter order, every call site, help text, and test were updated in the same commit as the flip.

**Conflict rule (shipped as designed)**: if `<warp-url>` is supplied and a binding already exists — matches: no-op, proceed.
Differs: hard error, never silently re-point (re-pointing is a distinct, deliberate operation).
No `<warp-url>` supplied and no binding recorded: error, unbound weft requires an explicit warp URL.

**Shipped additions this design did not anticipate:**

- A throwaway pre-hub probe clone of the weft candidate (`warpprobe.go`) reads the recorded binding before any hub directory exists, because the hub is named after the warp repo and so has no path until the warp URL is known.
- The one-argument form's old-order bootstrap guard: a two-argument call that would bootstrap a fresh binding refuses when the weft candidate's history carries neither the recorded binding nor the `.lyx-anchor` marker (nor an empty tree), since that shape usually means the caller passed a warp URL where a weft URL belongs.
  `--force-bootstrap` bypasses the guard for a caller who genuinely means it.
- `CloneHub`'s parameters became an options struct (`CloneOptions`) rather than growing a sixth positional, since two adjacent optional URL strings is exactly the shape that produces silent argument-order bugs — the same shape this slice's own breaking change fixes.
- `Reconcile` backfills the binding once per hub from the warp side's `origin` remote, for every hub that predates it, with an outcome set (`recorded`, `present`, `diverged`, `skipped`, `deferred`, `record_failed`) reported through the CLI layer.

**Known limitation, not fixed by this slice**: the hub still stays `<cwd>/<warp-name>-HUB`, so two wefts bound to warps of the same name collide on that directory name.
This is true today and predates this slice;
nothing here changes it.

Depended on slice 7 (built the new bootstrap logic against slice 7's already-narrowed clone/junction structure, not the pre-slice-7 one) and slice 9 (clone.go collision, see above — sequenced after).

## Warp stays ordinary git — preserved, unaffected by slices 7-10

Plain `git`/rebase/amend/force-push stays available in warp for external actors (a human, or any tool outside LYX/LoomYard) who don't know weft exists — `fabric`'s job is to stay correct under arbitrary external warp activity, not to be the only entrance for the whole world.
The correspondence link is one-directional (weft records `Warp-SHA` pointing at warp, recorded at weft-commit time), so warp never needs to route through fabric for correspondence to hold.
This is not a carve-out for LYX's own code or LLM agents LYX drives — for those, every git operation goes through `Fabric.Commit`/`internal/fabricengine`, no exception (CONSTRAINTS.md's Fabric Git Invariant).

## Scope boundary — still not a general-purpose git wrapper

`gitrepo` deliberately excludes rebase, interactive staging, cherry-pick, conflict resolution — a human always has plain git in either working tree.
What fabric wraps is the set that affects the two-repo illusion and correspondence: commit/push/pull/sync, clone/worktree/branch topology, unified diff/status, and (per CONSTRAINTS.md's Fabric Git Invariant) single-sided warp-only mutating verbs that need to dispatch through `internal/fabricengine`.
Read-only verbs the caller can run directly.

## Open questions

- **Weft remote provisioning at first clone** — still open.
  First-ever setup needs a weft remote to push to (empty is fine);
  either require it pre-created, or have clone provision it.
- **Slice 6's orchestration-layer half** — still open. `Fabric.Pull` (fabric-layer detection + re-anchor + `PullResult` residue) is shipped;
  which layer drives pull → conflict-resolve → raddle-regen, and how `PullResult`/`PATTERN` re-alignment is presented to an LLM resolving agent, stays open until `loom`/`Shed` exist to consume it.
- **`worktreePath` naming** — new, see slice 7 above: `worktreePath` was this discussion's working label for the resolver's public root field;
  not pinned as the final Go identifier.

## Related

> **Superseded:** `_pattern` no longer exists as a junction, and `_raddle` is anchor-level, never junction-reached — see `manifest/designs/raddle.md`.
> The `_pattern` mention below is historical narrative describing the state as of when slice 1 landed.

- `internal/fabricengine` (doc.go), `internal/lyxcwd` (lyxcwd.go, anchor.go) — the shipped bases slices 7-10 restructured;
  durable parts fold here on landing.
- [internal/mergeresolve](../../internal/mergeresolve/doc.go) — only the ordinary git-conflict shape shipped there; the document-driven conflict mechanism slice 6's orchestration half would have reused remains an unscheduled item (see `manifest/roadmap.md`'s Someday `finalize: the discrepancy-document conflict shape` item).
- [raddle.md](raddle.md) — the regenerate-don't-merge property bounding rebase recovery;
  the snapshot-staleness consumer slice 4 serves.
- [warp-visibility.md](warp-visibility.md) — the narrower sibling illusion (`CLAUDE.local.md`), same junction mechanism slice 1 generalized.
- [internal/pattern](../../internal/pattern/doc.go) — hand-authored weft content;
  a `_pattern` junction consumer of the slice-1 config-driven list;
  also the residue of rebase re-alignment.
- `fabric-v2-crucible` (wiki) — slice 11, the hardening pass sequenced after every slice in this doc including 7-10, per project policy that it runs last.
  It landed 2026-08-09 and turned out **not** to be the final slice: it surfaced four defect *shapes* it could not close, scoped as slices 12-15 and now landed — see [internal/fabricengine](../../internal/fabricengine/doc.go)'s package doc.
- CONSTRAINTS.md's Fabric Git Invariant and Cwd Resolution Invariant — the invariants this doc's shipped work enforces and slice 7 narrowed.
