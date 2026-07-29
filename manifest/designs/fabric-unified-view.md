# fabric: unified-repo view — the single entry-portal that makes warp+weft look like one repo

> **Status: Planned — not built.** Promoted from Someday and substantially expanded (2026-07): what began as "extend the illusion through a `Fabric.Commit` and a unified diff" grew, through design discussion, into a fundamental clone/init/topology reshaping of `fabric`. Sequenced **after** the Planned `board` item (which removes `board-url` from clone) and **after** `native clients` (build fabric's git logic against the final go-git `gitrepo` from the start, the same reasoning that sequences `loom` after it); **`Shed` follows this**. Per the [documentation lifecycle](../../docs/overview.md#documentation-lifecycle), the durable parts fold into `internal/fabricengine`'s package doc when this lands and this file is deleted.

## The redefined scope: fabric is the one-repo illusion portal, nothing more

`fabric` operates two git repos (warp and weft) but its whole job is to make it look, from the outside, like there is **one flat repo**. That is the entire scope — and stating it this crisply resolves a long-standing confusion about "what is `lyx init` vs what is `fabric`":

- **`fabric` owns topology + wiring.** Clone, worktree-add, branch pairing, junction wiring — all of it. There is no separate "init phase" that wires junctions after the fact (see "Clone does everything" below); that concept **dissolves into `fabric`**.
- **Session bootstrap is `loom`'s, not fabric's.** Seeding `_lyx` *content* when a session starts is a different concern that stays with `loom`. Clean line: **topology + wiring = fabric; session start = loom.**

`fabric`'s git-API is **deliberately simplified and is not for humans** — it is for `lyx` and other LoomYard code. We expose only what is needed, never a general-purpose git wrapper. A human always keeps plain `git` in their own warp worktree; fabric's surface is not a replacement for that (see "Warp stays ordinary git" below). This answers the old doc's open "humans or only agents?" question: the API is tooling-facing, warp-raw-git is human-facing.

## `Fabric.Commit` — the centerpiece (this reverses the old conclusion)

The original exploration tentatively concluded `Fabric.Commit` had **no confirmed caller** and might be dropped. That conclusion was reached under the premise "warp stays raw git, weft is Go-only, so nobody needs a unified classify-and-dispatch commit." The illusion-first framing **changes that premise and gives `Fabric.Commit` its caller: everything in LYX that writes files.**

`Fabric.Commit([files], msg, [snapshot-tags])` classifies each path against the known weft junction mountpoints (a trivial path-prefix check) and dispatches to the warp or weft repo accordingly. The value is **not** correspondence — the correspondence link is recorded weft-side regardless (see below). The value is **API uniformity / the illusion itself**: a caller (LLM or Go) never has to know which of the two repos a file physically lives in. That is the point of the design, and it is real.

Two things still hold and must be designed for:

- **Not atomic.** A `Fabric.Commit` spanning both warp-side and weft-side paths is still two underlying git commits, not a cross-repo transaction. The illusion is "one repo" for the *interface*, but crash-safety across the two commits needs a defined story (commit weft-first or warp-first, and what happens if the second fails). This is the honest cost of the illusion — an open question below.
- **The "LLM never decides weft-commit timing" invariant survives — but decouple mechanism from policy.** Its original enforcement was *accidental*: an LLM's `git add <weft-file>` simply failed (git add operates on warp; the weft file isn't tracked there). `Fabric.Commit` makes weft writes clean, so that accidental guardrail is gone. Keep the invariant anyway, as **deliberate policy** — weft-commit timing is orchestration's call (Finalize, raddle-regen at phase boundaries) — and rewrite its justification from "git add would fail" to "orchestration controls weft-commit timing." Cleaner than before: a conscious rule, not a side-effect. (This does not decide that LLMs *should* commit weft — only that the mechanism no longer forbids it by accident.)

## Clone does everything — the `lyx init` phase is gone

Today setup is two steps: `fabric clone` clones the repos, then a human `cd`s into the right subpath and runs `lyx init` there to wire junctions. The reason init was separate: **`RelPath` (the lyx-anchor subpath) is positional today** — `hubgeometry.Resolve(cwd)` computes `RelPath = filepath.Rel(WorktreeRoot, cwd)`, so you had to physically stand in the subpath for the right anchor to be captured. Clone couldn't know it.

The fix: **store the subpath↔weft binding in the `weft` repo itself.** One weft repo is 1:1 with one host repo and one anchor subpath (multi-subpath is explicitly out of scope — see below), so that binding is intrinsic to weft. `lyx fabric clone` then does the whole job in one shot — prime worktree, prime-weft worktree, and all junction wiring at the right subpath — with **no init step**. It runs in create-or-adopt mode, exactly the pattern `suffixWeftPrimaryBranch` already uses for the primary weft branch:

- **Weft remote already carries the binding** (a re-clone on a new machine): read the subpath from weft, wire accordingly. Any `--subpath` flag is ignored (or validated against the record and errors on mismatch, so you never accidentally re-anchor).
- **Weft remote has no binding** (the genuine first-ever setup — first clone *is* create): this is where the human supplies the anchor. Because the prime worktree does not exist yet at clone time, **cwd cannot specify a subpath inside it** — you are standing where the *hub* will be created, not inside a worktree. So the subpath is an **explicit flag**, `--subpath <rel>` (default `"."` = root, the common case), recorded into the new weft config as the permanent source of truth. `--weft-url` is the one genuinely underivable input (where to push the new weft repo). Post-`board`, no `board-url` is needed (board lives in `weft:main`).

Because this is a once-ever, source-of-truth decision, create-mode **echoes and confirms** the resolved anchor before writing:

```
Anchoring lyx at "<RelPath>"  (relative to repo root <WorktreeRoot>)
Weft repo → <url>
Junctions to wire: _lyx _raddle _pattern …   (from template)
Proceed? [y/N]
```

The one truly bad failure here — cd'd/flagged the wrong anchor, now locked forever — is caught by that echo, without forcing extra ceremony on the common root case.

**Multi-subpath is not supported.** One prime worktree, one anchor, one weft. Two independent lyx roots in one host repo would require two weft repos and two separate clones — low probability, deliberately out of scope. This is what keeps the binding a clean 1:1 and "store the subpath in weft" unambiguous.

## Consequence: `RelPath` moves from positional to recorded

This is the real work of the clone change. Today `Resolve(cwd)` trusts cwd. Once the subpath is a recorded binding in weft, **runtime `Resolve` must consult that binding** (or a marker at the anchor), not blindly re-derive from cwd — otherwise a command run from the wrong subdir (e.g. repo root, above the anchor) resolves `RelPath = "."` ≠ the recorded anchor and the geometry is wrong. The recorded binding becomes source of truth; cwd-derivation is demoted from "the resolver" to, at most, a consistency check (it happens to agree when you stand at or below the anchor: `Rel(prime, prime/X) = X`). Reconciling the two — record wins, cwd validates — is the concrete change this item must make in `hubgeometry`. Subpath support itself is already comprehensive: `RelPath` is threaded through every junction/portal/launcher path and every relative-link climb, handling both `"."` and arbitrary segments — it is not vestigial.

## Config-driven junction list — fabric stops enumerating modules

Today the wired-junction set is hardcoded (`hubgeometry.IsReservedHubName`: `_lyx`, `_raddle`, `_board`, `_portals`, `_launchers`) — adding `_pattern` or `_codeintel` means a code change. Instead, the set of weft-backed folder names lives in a **config file with a template** that any new weft-backed module appends its junction name to. `fabric` never has to know about every module that might want a folder.

This lives naturally in the **same weft config as the subpath binding** — both are per-repo setup facts, and `lyx fabric clone` reads both from one place. It grows out of today's `fabric.yaml` (`pathspec` key already there).

Ownership boundary to set deliberately (CONSTRAINTS.md's **Hub Geometry Invariant** says `hubgeometry` owns all geometry/paths): keep `hubgeometry` the owner of the *paths* (it computes `<Hub>/<slug>/<RelPath>/<name>`), but inject the *name set* from fabric config. The invariant holds (geometry owns paths) while fabric stops hardcoding modules.

## Eager wiring at worktree-add

Every `lyx fabric worktree add` wires all junctions immediately, under the hood — no dormant `_lyx` waiting for a later activation. `WireJunctions` already lives in `fabric` and wires the whole configured set in one pass; folding it into `worktree add` (after the weft worktree exists, so the junction targets resolve and the host-pristine guard is satisfied) is what makes fabric the sole, end-to-end wirer. The old "junction wired ⇔ active session" distinction is dropped as finer than it is worth; an empty `_lyx` on an idle worktree harms nothing.

## Snapshot-tracking folds into the `Warp-SHA` trailer mechanism

Today there are **two** SHA-bookkeeping mechanisms: `gitrepo`'s `refs/loomyard/snapshot/<key>` refs (`SetSnapshotSHA`/`SnapshotSHA`) and `fabricengine`'s `Warp-SHA` trailers + correspondence index. Unify them: snapshot-recording becomes an **optional trailer on the weft commit** — `Fabric.Commit([files], msg, snapshotTags=["raddle"])` writes a `Snapshot: raddle` trailer alongside the `Warp-SHA` trailer, and a snapshot's baseline is derived from the latest weft commit carrying that tag (its `Warp-SHA` trailer *is* the warp SHA the snapshot describes — exactly what raddle needs). Same architecture the correspondence index already uses: trailer is truth, an index on top is a rebuildable cache.

Fold it into `Commit` rather than a standalone `Fabric.Snapshot("tag")`: a snapshot is meaningless except in relation to a specific committed state, so coupling it to the commit that produced that state is more correct (and matches raddle's "advance only on confirmed success"). A standalone no-commit snapshot call is only warranted if a consumer must record a baseline without producing weft content — which raddle/codeintel (both commit their output) never do; leave it out until a real caller appears. This retires the separate `refs/loomyard/snapshot/` mechanism.

## Warp-rebase and remote-reconcile — the hardest part, but bounded

`fabric` must handle warp history that moves underneath it: a non-LYX collaborator (or the same operator on another machine) pushes to warp remote, and later a LYX+fabric session on this machine must **sync those commits down** — resolving conflicts (the intent is to spawn an LLM), including the extreme case where the remote was **rebased**.

The naïve fear is "replay all of weft onto the rebased warp." That fear shrinks once decomposed, because most of weft is a **pure function of the code that regenerates**, not something merged:

- **Detection is already honest and shipped.** After a warp rewrite, weft's `Warp-SHA` trailers point at warp SHAs that no longer exist; `SHAExists` catches this rather than trusting a dead reference (the "staleness survives rebuild" tests already guard it).
- **raddle / codeintel self-heal.** Both regenerate at merge-time (raddle.md) — pure functions of code. Post-rebase: stale → regenerate → new weft commit with a fresh `Warp-SHA` trailer → correspondence re-established.
- **`_lyx` never propagates to parent** (finalize.md) — no re-alignment needed against a rebased parent.
- **The residue is small:** genuinely hand/LLM-authored weft content (`PATTERN`). Rare, small. This is the only thing that needs real re-alignment.

So rebase-recovery = re-anchor the correspondence (the `RevertWithWeft` "nearest-older" logic is the building block) + regenerate derived content + a small hand/LLM re-alignment for `PATTERN`. The layering keeps the shipped invariant intact: **`fabric` core detects and precomputes the diff; an orchestrator above it spawns the LLM** for genuine content conflicts, using finalize.md's document-driven mechanism (Go hands the agent a plain document, never git conflict markers across a junction). "Rebase is part of fabric" means the mechanics + detection are fabric's; the LLM resolution sits just above, in orchestration — never an LLM deciding weft-commit timing.

## Warp stays ordinary git — preserved, and it is why all this is feasible

Plain `git add`/`git commit` (and rebase, amend, force-push) stays the norm for warp, for humans and agents alike. `Fabric.Commit` is not a mandatory door on warp — nothing can force *other* collaborators through it, so fabric's job is to stay correct under arbitrary external warp git activity, not to be the only entrance. This is exactly what makes the whole illusion feasible: **the correspondence link is one-directional (weft records `Warp-SHA` pointing at warp), recorded at weft-commit time from warp's current HEAD.** Warp therefore never needs to route through fabric for correspondence to work — a raw `git commit` in warp is fine, and the next weft commit picks up warp's new HEAD in its trailer. Only weft holds the linking info; warp behaves as if lyx never piggybacked it. A post-checkout hook already fires drift warnings so an out-of-band warp branch (no weft yet) is *detected*, not silently mishandled.

Consequence: a warp-only `Fabric.Commit` is legitimate (it completes the illusion — the two-repo split stays invisible even for warp-only writes) even though it buys nothing for correspondence. That is fine; the uniformity *is* the reason.

## Scope boundary — still not a general-purpose git wrapper

`gitrepo`'s scope deliberately excludes rebase, interactive staging, cherry-pick, conflict resolution, and preserves "a human always has plain git in either working tree." That conscious boundary stands. Routing *every* git verb through `Fabric` (blame, stash, branch, log …) reprises the already-rejected "forwarding method per operation" pattern at large scale and is not the goal. What fabric wraps is the small set that affects the two-repo illusion and correspondence: commit/push/pull/sync, clone/worktree/branch topology, and the unified diff/status. Read-only verbs the caller can run directly; where a *unified view across both repos* is wanted (a single `Fabric.Diff`/`Status` for "what changed in my worktree since a point"), that is an ordinary per-repo diff merged via the correspondence index, not a new primitive.

## Dependencies and sequencing

- **After `board: move storage to weft:main`.** `board: move storage to weft:main` removed `board-url` and the board-clone step from `CloneHub` (board moved into `weft:main` — see the `internal/boardengine` package documentation), so this item inherits a 2-repo clone (host+weft) to restructure, not 3-repo. It also introduced prime's second weft checkout (`weft:main` for board, alongside `weft:main-weft`) and the "everything lyx-related lives in weft" pattern the subpath/junction-config binding slots into. The two share the weft-branch adopt-or-create primitive (`suffixWeftPrimaryBranch`), which board hardened for the `weft:main` case first.
- **After `native clients`.** Build fabric's clone/commit/snapshot git logic against the final go-git-based `gitrepo`, so it isn't re-validated if the CLI→library swap surfaces any subtle behavioral difference — the same reasoning that sequences `loom` after `native clients`.
- **`Shed` follows this.**

## Build order — slices, not one task, and extend-in-place

`fabric` V2 is not a from-scratch rewrite and not a parallel `FabricV2` package. The Warp+Weft→V1 merger justified a parallel reference because it was a genuine architectural *union of two modules*; V2 is ~six changes layered onto `fabricengine`, whose core (two `gitrepo` instances, weft pairing, `Warp-SHA` trailer correspondence, weft-git plumbing, junction primitives, branch scheme) is reused wholesale. A parallel package would be massive duplication for no gain. **Extend `fabricengine` in place, one landable slice at a time**, with git history + green tests between slices as the reference — not a second copy of the module.

Suggested slice order (none individually "enormous" — the size was always the sum):

1. **Config-driven junction list** — replace the hardcoded reserved-name set (`hubgeometry.IsReservedHubName`) with a config-read list + a template new weft-backed modules append to. Small, near-mechanical, independent. `hubgeometry` keeps owning the paths; only the name set comes from config. Note the handoff with the Planned `PATTERN.md` item: `pattern-wiring` ships against today's *hardcoded* list (it adds `_pattern` there, which is correct — it is not blocked on this slice), so by the time this slice runs `_pattern` is already a hardcoded entry. This slice is therefore where PATTERN's (and `_lyx`/`_raddle`/`_board`'s) hardcoded junction wiring is migrated into config — it cleans up the existing entries, not just hypothetical future modules.
2. **`Fabric.Commit` (classify+dispatch) + unified `Fabric.Diff`/`Status`** — pure API additions over the existing `Warp`/`Weft` handles, `CommitWeft`, and `ChangedFilesSince`. Independent; the atomicity / partial-failure story lands here.
3. **Snapshot-as-trailer** — fold `refs/loomyard/snapshot/` into the `Warp-SHA` trailer mechanism; coordinate with `native clients` (which ports the ref-based snapshot to go-git first — this slice supersedes that, a minor, harmless overlap).
4. **Clone-does-everything + subpath-in-weft + `init` dissolution** — the structural heart, after `board`. This is where **`lyx init` dissolves.** `initengine.Init` today does six things: cwd→`RelPath` anchor resolution, weft-pairing check, `WireJunctions`, `_lyx`/`_lyx/config` creation, the `.lyx/` `.gitignore` block, and `configsync.ReconcileAll`. Under V2 the topology parts (wiring, `.gitignore`, `_lyx` dirs) fold into `fabric`'s clone/worktree-add (eager wiring); the cwd-anchor mechanism is *replaced* by the weft-recorded subpath; config reconciliation stays in `configsync`/`configcli` (called once by clone, or a separate post-clone `lyx config` step). Net: `initengine`/`initcli` shrink toward deletion, with `--undo`'s teardown moving onto `fabric`'s existing `UnwireJunctions` + config revert. Optionally split 4a (subpath binding + `RelPath` positional→recorded) from 4b (fold wiring into clone/add). Develop the risky new clone path as a **coexisting entry point inside `fabricengine`** — the old clone stays until the new one is proven, then swap and delete — a local safety valve, not a module-wide fork.
5. **Warp-rebase / remote-reconcile** — last. Detection (`SHAExists`) + correspondence re-anchor (`RevertWithWeft` nearest-older) + `PATTERN` document-driven resolution land with fabric; the full self-heal leans on raddle regeneration (Someday), so reconcile's complete form trails there. The LLM conflict-resolver sits in orchestration above fabric (finalize.md's document-driven path), never deciding weft-commit timing.

The roadmap item is therefore a small campaign — 4–5 board tasks when picked up — not one atomic task. Slices 1–3 don't touch clone and could technically precede `board`, but keeping them after `board` avoids reopening the sequence set above.

## Open questions (for whoever builds this)

- **Partial-failure semantics for a two-sided `Fabric.Commit`** — commit weft-first or warp-first, and the recovery/report story when the second commit fails (no cross-repo transaction exists).
- **The RelPath record-vs-cwd reconciliation mechanism** — a marker file at the anchor, a value read from weft config, or a climb — and how `Resolve` consults it while keeping cwd as a consistency check.
- **Home of the junction-name config** — `fabric.yaml` read by `hubgeometry`, vs `hubgeometry` owning it and pulling values from config; keep geometry the owner of paths either way.
- **Weft remote provisioning at first clone** — the first-ever setup needs a weft remote to push to (empty is fine); either require it pre-created (the GitHub-wiki wrinkle) or have clone provision it.
- **Exact rebase/remote-reconcile orchestration** — which layer drives pull → conflict-resolve → raddle-regen, and how `PATTERN` re-alignment is presented to the resolving agent (reusing finalize.md's document-driven path).
- **Whether `Fabric.Diff` is a CLI verb or Go-internal only** — depends on who needs it (a human debugging, an instructed agent, or only internal callers).

## Related

- The `internal/boardengine` package documentation — the shipped `board: move storage to weft:main` item this sequences after; removed `board-url` from clone, established weft-as-home and prime's two-weft-checkout shape.
- [native-clients-migration.md](native-clients-migration.md) — the `gitrepo` go-git migration this builds its git logic on top of.
- [finalize.md](finalize.md) — the document-driven, non-git-marker weft-conflict mechanism the rebase/reconcile path reuses; also the weft-side merge-back this shares primitives with.
- [raddle.md](raddle.md) — the regenerate-don't-merge property that bounds rebase recovery; the snapshot-staleness consumer the trailer-fold serves.
- [host-visibility.md](host-visibility.md) — the narrower sibling illusion (`CLAUDE.local.md`), same junction mechanism.
- [pattern.md](pattern.md) — the hand-authored weft content that is the real residue of rebase re-alignment; also a `_pattern` junction consumer of the config-driven list.
- `internal/fabricengine` (doc.go) — the shipped base this generalizes; the durable parts fold here on landing. `CONSTRAINTS.md`'s "Orchestration, not agent" section is the invariant this enforces more consistently, never violates.
