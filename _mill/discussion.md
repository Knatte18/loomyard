# Discussion: fabric: clone-does-everything + subpath-in-weft + init dissolution

```yaml
task: 'fabric: clone-does-everything + subpath-in-weft + init dissolution'
slug: fabric-clone-subpath
status: discussing
parent: main
```

## Problem

`fabric` operates two git repos — **warp** (the host repo, ordinary code) and **weft** (lyx's own sidecar: `_lyx`, `_pattern`, …) — and its whole job is to make them look like one flat repo. Today, setting that up is two steps: `lyx fabric clone` clones the repos, then a human `cd`s into the right subpath and runs `lyx init` there to wire junctions, create `_lyx`, maintain `.gitignore`, and reconcile config. The reason `init` is separate is that the lyx-anchor subpath (`RelPath`) is **positional**: `hubgeometry.Resolve(cwd)` computes `RelPath = filepath.Rel(WorktreeRoot, cwd)` (hubgeometry.go:127), so a human had to physically stand in the subpath for the right anchor to be captured. Clone couldn't know it.

This is fragile (a command run from the wrong subdir resolves the wrong `RelPath` and every geometry-derived path points somewhere wrong) and clumsy (clone, then `cd`, then `init`; a re-clone on a new machine must re-remember the subpath). Slice 5 of the fabric-unified-view campaign (`manifest/designs/fabric-unified-view.md`, Build order slice 5 — slices 1–4 are DONE) is the structural heart that fixes it: store the subpath and the junction set as **per-repo facts in the weft repo**, make `clone`/`worktree add` do all wiring in one shot, make `Resolve` consult the recorded anchor instead of trusting cwd, and dissolve `lyx init` entirely. **Why now:** slices 1–4 (config-driven junctions, `Fabric.Commit`, commit-lock/coalescing, snapshot-as-trailer) have landed and `board` has moved to `weft:main`, so the 2-repo clone and the shared repo-wide weft branch this design binds to now exist.

## Scope

**In:**
- **Subpath-in-weft**: record the lyx-anchor subpath as a per-repo fact on the `weft:main` branch. First-ever clone supplies it via `--subpath <rel>` (default `.`); every later clone reads it — no `cd`, no `init`.
- **`RelPath` positional → recorded**: `hubgeometry.Resolve` consults the recorded anchor as the source of truth for `RelPath`, demoting cwd to a validated consistency gate.
- **Clone-does-everything**: `lyx fabric clone` does the whole topology job in one shot — clone host + weft, check out the weft primary pairing, materialize the `weft:main` board worktree, **wire all junctions**, create `_lyx`/`_lyx/config`, maintain the `.gitignore` `.lyx/` block, and run `configsync.ReconcileAll` once. No separate `init` step.
- **Eager wiring at `worktree add`**: `Topology.Add` folds in junction wiring so every new worktree is wired immediately (drops today's dormant-`_lyx` model, add.go:304).
- **Junction set → repo-wide**: move `pathspec` (the junction name-set) from per-worktree `fabric.yaml` into the same repo-wide weft record as the subpath. Reconcile converges **every** worktree in the hub to the one repo-wide set.
- **Declarative junction convergence**: extend `lyx fabric reconcile` to add missing junctions, **remove stale ones** (present on disk but no longer in `pathspec`), and no-op correct ones. This is the single mechanism clone/add and later module-activation all use.
- **`init` dissolution**: delete `internal/initengine` and `internal/initcli`, unregister from `cmd/lyx/main.go` and the help tree. Move the `init --undo` teardown onto a new `lyx fabric unwire` verb.

**Out:**
- **Slice 6 (warp-rebase / remote-reconcile)** — the `SHAExists`-detection + correspondence re-anchor + `PATTERN` document-driven resolution. Explicitly a later, separate task.
- **Weft-remote provisioning** — the weft remote must pre-exist (empty is fine at first attach). No `gh`/API repo creation; that stays a manual/out-of-scope prerequisite.
- **Multi-subpath** — one prime worktree, one anchor, one weft, clean 1:1. Two lyx roots in one host repo is deliberately unsupported.
- **The actual `_raddle` activation** — this task builds the repo-wide-`pathspec` + reconcile-converges-all-worktrees machinery that makes later per-module activation possible, but does not add `_raddle` to `pathspec` or ship raddle itself. Raddle is a future consumer.
- **`branch_prefix` semantics** — it moves along with `pathspec` into the repo-wide `fabric.yaml` (it is already a repo-wide, env-resolved convention), but its meaning/derivation is unchanged.
- **The Sandbox-repo one-time migration** — the one real pre-existing lyx hub needs a one-time marker write into its `weft:main`; that is a documented follow-up chore in a separate repo, not code in this task (see Q&A log).

## Decisions

### full-slice-5-in-one-task

- Decision: Do all three intertwined parts — subpath-in-weft, clone/add-does-everything, and `init` dissolution — in this single task, not split into 5a/5b.
- Rationale: They are coupled — clone-does-everything needs the recorded subpath to know where to wire, and `init` dissolution needs both. The filed brief names all three. The coexisting-entry-point safety valve is dropped in favour of in-place edits guarded by tests (see `clone-modified-in-place`), so no task boundary is needed to manage risk.
- Rejected: 5a-only (record subpath + `RelPath` recorded now, defer clone-wiring/init to a follow-up) — leaves an awkward half-state where the anchor is recorded but only `init` writes it.

### subpath-recorded-in-weft-main

- Decision: The lyx-anchor subpath is a per-repo fact stored on the **`weft:main` branch** (the branch that is "board"). It is the durable source of truth (survives re-clone). It is read at runtime from wherever `weft:main` is checked out on disk — in today's geometry, the worktree at `hubgeometry.BoardDir(Hub)` (the directory that happens to be named `_board`).
- Rationale: One host repo = one anchor = one weft = one subpath, and it is identical across all worktrees, so a per-worktree marker would be needless duplication. `weft:main` is the one shared, repo-wide, git-durable branch, and its checkout at `BoardDir(Hub)` is a fixed, `RelPath`-independent location every command can reach (`Hub = filepath.Dir(WorktreeRoot)`, always known from git). This breaks the circularity of storing it under `RelPath` (a per-worktree `fabric.yaml` sits at `<weft>/<RelPath>/_lyx/config/`, so reading it would require already knowing `RelPath`). This matches the design doc: subpath and junction-list are both per-repo setup facts that live in the same weft config (fabric-unified-view.md line 55).
- Rejected: a per-weft-worktree marker at each `<slug>-weft` root (duplication, N copies to keep coherent); storing it in the per-worktree `fabric.yaml` under `RelPath` (circular).

### two-repo-wide-files-on-weft-main

- Decision: Two files committed on `weft:main`: a plain single-line `.fabric-anchor` (holds only the subpath string, e.g. `backend` or `.`) read by `hubgeometry`; and `fabric.yaml` (holds `pathspec` + `branch_prefix`) read by `fabricengine`. `pathspec` moves out of the per-worktree `fabric.yaml` into this repo-wide one.
- Rationale: `hubgeometry` imports only stdlib + `gitexec` and must stay YAML-free; the subpath is geometry (its concern) so it gets a plain marker it can read with `os.ReadFile` + `TrimSpace`. `pathspec`/`branch_prefix` are fabric config (fabricengine's concern, YAML is fine there). Different owners, different concerns — the split is principled, not redundant duplication.
- Rejected: one repo-wide `fabric.yaml` with a hand-rolled `subpath:`-line scan in `hubgeometry` (fragile against YAML quoting/formatting); one repo-wide `fabric.yaml` with a YAML parser added to `hubgeometry` (drags YAML into the lean geometry package, against its near-leaf profile).

### relpath-record-wins-cwd-is-a-hard-gate

- Decision: `hubgeometry.Resolve` reads the `.fabric-anchor` marker and sets `RelPath` from it (record is truth). It then validates that cwd is **at or below** `<WorktreeRoot>/<anchor>`; if cwd is outside the anchored subtree (a sibling subdir, above the anchor, or otherwise outside), that is a **hard error**, not a warning. When the marker is **absent**, `Resolve` falls back to today's cwd-derived `RelPath`.
- Rationale: For a root-anchored repo (`RelPath="."`) everything under the root is at-or-below, so the gate never fires — commands work from anywhere. For a subpath-anchored repo, running lyx from outside the anchored subtree is genuinely wrong and must fail loudly (the user's explicit call: "det vil ikke funke å bruke lyx i en annen mappe enn den er initialisert for … ordentlig error"). The absent-marker fallback is purely defensive robustness — for mid-clone (before the marker is written), lyxtest synthetic hubs, and non-fabric git repos — never a supported "run a real hub without a marker" mode; every real fabric hub always carries the marker after a new clone.
- Rejected: record-wins-but-cwd-mismatch-is-only-a-warning (too permissive — the user wants a hard error outside the subtree); cwd ignored entirely (loses the cheap detection); marker required with no fallback (would crash mid-clone / in lyxtest / in non-hub repos).

### anchor-value-always-explicit

- Decision: The `.fabric-anchor` marker is always written on a new clone, including the common root case where it holds `.`. There is no "marker absent = assume root" special case for freshly-cloned hubs.
- Rationale: The marker always exists after a new clone, so `Resolve` reads a concrete value uniformly, with no ambiguity between "absent because root" and "absent because broken". (The absent-marker fallback in `relpath-record-wins-cwd-is-a-hard-gate` is for transient/non-hub contexts only.)
- Rejected: writing the marker only for non-root anchors (introduces the ambiguous missing-means-root state).

### first-clone-validates-subpath-exists

- Decision: First-ever clone takes `--subpath <rel>` (default `.`) and **validates that the directory exists in the freshly-cloned host worktree**; if `<hostWorktree>/<subpath>` does not exist, hard error. No interactive confirmation prompt. On success the resolved anchor is echoed in the JSON result (e.g. `{"ok":true,"hub":…,"anchor":"backend"}`).
- Rationale: `lyx` is a JSON-envelope CLI (one JSON object to stdout per invocation, output.go:20/32) called non-interactively by automation, so an interactive `[y/N]` prompt is a poor fit. Validating the subpath against the cloned host tree catches the one truly-bad failure — a typo like `--subpath backedn` that would otherwise silently anchor to a wrong/created directory forever — precisely, non-interactively, and automation-safe.
- Rejected: an interactive TTY-gated confirm with `--yes` bypass (more machinery, a prompt-to-stderr wrinkle, and weaker than existence-validation anyway).

### create-or-adopt-clone

- Decision: Clone runs in adopt-or-create mode, mirroring `suffixWeftPrimaryBranch` (clone.go:141). **Adopt** (re-clone on a new machine): the weft remote already carries `.fabric-anchor` — read the subpath from it and wire accordingly. Any `--subpath` passed on a re-clone is **validated against the record and hard-errors on mismatch** (never a silent re-anchor). **Create** (genuine first-ever setup): no record exists yet, so the human supplies `--subpath` (validated per `first-clone-validates-subpath-exists`) and it is written into `weft:main` as the permanent source of truth.
- Rationale: The subpath is a once-ever, source-of-truth decision; re-anchoring by accident is the failure to guard against, so a conflicting flag on re-clone must fail loudly rather than vanish.
- Rejected: silently ignoring `--subpath` on re-clone (a contradicting value disappears without a trace).

### weft-remote-must-preexist

- Decision: The weft remote must already exist (empty is fine at first attach). Clone does not provision it.
- Rationale: `CloneHub` today simply `git clone`s the weft URL (clone.go:98); provisioning needs a git-host API/token, an out-of-scope dependency class. Keep the two-URL `clone <host-url> <weft-url>` signature.
- Rejected: clone provisions the weft remote via `gh`/API (drags in host-API auth; defer).

### clone-modified-in-place

- Decision: Modify `CloneHub` (and `Topology.Add`) in place, guarded by tests. Do not build a parallel `CloneHubWithAnchor` coexisting entry point.
- Rationale: Clone has no external callers beyond `internal/fabriccli`; a single well-tested change is cleaner than carrying a dead parallel path. The design's "coexisting entry point, swap-and-delete" advice targeted a risky manual migration, not a test-fenced mill task.
- Rejected: the coexisting-entry-point valve (double maintenance in one commit for no gain here).

### declarative-junction-reconcile

- Decision: Junction wiring is a **declarative convergence** to the repo-wide `pathspec`: add junctions in `pathspec` but missing on disk, **remove junctions on disk but absent from `pathspec`**, no-op correct ones. `lyx fabric reconcile` exposes this (extended with the new stale-removal step), and clone/`worktree add` apply the same convergence to their new worktree for eager wiring. Because `pathspec` is repo-wide, `reconcile` converges **every** worktree in the hub — activating a module (updating `pathspec` once) wires it everywhere on the next reconcile.
- Rationale: A single declarative "make wiring match config" beats imperative `wire X`/`unwire X` verbs — it self-heals drift in both directions and is the natural entry point both for setup (clone/add) and for later per-repo module activation (the user's raddle example: activation is per-repo, so fabric must wire it in all worktrees). Today's `Reconcile` (reconcile.go:95) already adds/repoints missing junctions via `WireJunctions`; the missing half is stale-removal (`checkJunctionHealth` only iterates the config name-set and never sees an on-disk junction absent from config).
- Rejected: imperative `lyx fabric wire [names…]` verb (leaves convergence half-declarative; the user prefers "update config, ask fabric to re-wire"); keeping `pathspec` per-worktree (activating raddle in one worktree would not wire it in the others — the user's explicit requirement is repo-wide wiring).

### init-dissolves-to-fabric-verbs

- Decision: Delete `internal/initengine` and `internal/initcli`; remove `initcli.Command()` from `cmd/lyx/main.go:117` and the module list at main.go:87, and update the help-tree tests. The six things `Init` did (init.go): cwd-anchor resolution is **replaced** by the recorded subpath; junction wiring, `_lyx`/`_lyx/config` creation, and the `.gitignore` `.lyx/` block fold into clone/add; `configsync.ReconcileAll` is called once by clone. The `init --undo` teardown (undo.go — `UnwireJunctions` + weft `_lyx` clear/commit/push + `.gitignore` revert) moves to a new **`lyx fabric unwire`** verb, a full deactivation distinct from `reconcile`'s config-convergence (reconcile-to-empty must never delete weft content).
- Rationale: Once clone/add do all wiring and `Resolve` reads the recorded anchor, `init` has no remaining job — "init dissolves" is the whole point of slice 5. The teardown is still needed, so it lands on fabric as its own verb.
- Rejected: keeping `lyx init` as a deprecated stub pointing to clone (leaves dead CLI surface slice 5 exists to remove).

### clone-runs-reconcileall-once

- Decision: `clone` calls `configsync.ReconcileAll` once (after wiring, materializing `fabric.yaml` and all module configs) so clone is genuinely complete. Config reconciliation stays owned by `configsync`/`configcli` — clone just invokes it.
- Rationale: "Clone does everything" means no manual post-clone config step; but the ownership boundary (config lives in `configsync`) is preserved — clone calls into it, does not reimplement it.
- Rejected: a separate `lyx config` step after clone (reintroduces the second manual step the design removes).

## Technical context

Modules and seams mill-plan needs:

- **`internal/hubgeometry/hubgeometry.go`** — `Resolve(cwd)` (line 107) is the single geometry resolver, called by ~27 sites across every module; `RelPath` is computed at line 127. This task makes `Resolve` read `.fabric-anchor` from `BoardDir(l.Hub)` for `RelPath` (with the cwd at-or-below gate and absent-marker fallback). `hubgeometry` imports only stdlib + `gitexec` (keep it YAML-free — hence the plain marker). Relevant existing helpers: `BoardDir(hub)` (line 381), `WeftWorktree()`/`WeftWorktreePath()`, `HostJunctions`/`HostJunctionsHere(names)`/`IsReservedHubName`, `HubReservedNames()`. Because almost every caller gets its `RelPath` transparently through `Resolve`, fixing `Resolve` fixes them all — the risk is concentrated here, so this is the primary TDD target.
- **`internal/fabricengine/clone.go`** — `CloneHub(cwd, hostURL, weftURL)` (line 55) is edited in place to: accept the subpath (adopt-or-create per `create-or-adopt-clone`), validate it, write `.fabric-anchor` + repo-wide `fabric.yaml` onto `weft:main`, wire all junctions, create `_lyx`/config, maintain `.gitignore`, and run `ReconcileAll`. `suffixWeftPrimaryBranch` (line 141) is the exact adopt-or-create pattern to mirror. `ensureBoardWorktree` (boardweft.go:35) materializes the `weft:main` checkout at `BoardDir` — the read/write point for the two repo-wide files.
- **`internal/fabricengine/reconcile.go`** — `Topology.Reconcile(l)` (line 95) already sweeps all host worktrees and repoints missing/broken junctions via `WireJunctions` (line 165); `checkJunctionHealth` (line 340) iterates only `HostJunctionsHere(names)`. Add the stale-junction-removal step (detect on-disk junctions absent from the repo-wide `pathspec` and unwire them via the existing `removeHostJunction`/`unseedLyxJunction` primitives in junction.go/weftwiring.go). Update the "run `lyx init` to activate" strings and the raw-adopt dormant behavior (reconcile.go:87,230,235) to wire eagerly instead.
- **`internal/fabricengine/junction.go` / `weftwiring.go`** — `WireJunctions(l, slug, names)` (junction.go:55) wires a given name-set idempotently; `UnwireJunctions(l, slug, names)` (line 207) is the teardown; `removeHostJunction` (weftwiring.go:137) removes a single host junction. These are the convergence and `unwire`-verb building blocks. Note `seedLyxJunction`'s host-pristine guard — wiring must run after the weft worktree exists so targets resolve.
- **`internal/fabricengine/config.go` + `template.yaml`** — `Config{BranchPrefix, Pathspec}` + `LoadConfig(baseDir)` + `WiredNames`/`junctionNames`. `pathspec` moves from the per-worktree base to the repo-wide `fabric.yaml` on `weft:main`; `LoadConfig`/`WiredNames` callers (reconcile, undo, add) switch to reading the repo-wide file at `BoardDir`. The `"not initialized here; run \"lyx init\""` message (config.go:47) and its twins in shuttle/reed/board engines must be updated once `init` is gone.
- **`internal/fabricengine/add.go`** — `Topology.Add` (line 83) currently leaves `_lyx` dormant (comment line 304); fold `WireJunctions` in after the weft worktree exists, and update `rollbackAdd` to unwire on failure.
- **`internal/initengine` + `internal/initcli`** — deleted. `init.go`'s six responsibilities are re-homed as above; `undo.go` is the reference for the new `lyx fabric unwire` verb (note its deliberate `_lyx`-only / never-`_pattern` asymmetry, undo.go:60-73 — preserve it).
- **`internal/fabriccli/clone.go` + `fabric.go`** — clone handler (clone.go:21) gains a `--subpath` flag; `fabric.go` usage strings (line 64,85 "run \"lyx init\"…") update, and the new `unwire` verb registers alongside `reconcile` (fabric.go:192). CLI/Cobra invariant: every command needs `Short`; help-tree tests must be regenerated.
- **`internal/output/output.go`** — `Ok`/`Err` envelope; clone's success adds the `anchor` field.

Gotchas: `Resolve` runs one `git rev-parse` subprocess per process already, so one extra tiny `os.ReadFile` of `.fabric-anchor` per invocation is negligible (no caching needed). The `weft:main` writes (both files) must go through fabric's commit/push path (like board's `Sync`) so they reach the remote and survive re-clone.

## Constraints

From `CONSTRAINTS.md` (authoritative), directly touched:

- **Hub Geometry Invariant** — `internal/hubgeometry` owns all cwd/worktree-root/geometry resolution; `Resolve`/`Getwd` are the only cwd path; geometry tokens (`_board`, `-weft`, `-HUB`, `_lyx`, `_pattern`, …) are hubgeometry-owned; **geometry is structural, never config/env-overridable**. This task extends the invariant: `RelPath` is now resolved from the recorded `.fabric-anchor` marker (read from `BoardDir(Hub)`), with cwd demoted to a validated at-or-below gate. The marker is a *structural geometry artifact* (a fixed per-repo anchor), not a config/env override, so the "structural, not config-overridable" rule holds — the anchor is not runtime-tunable, only set once at create. `hubgeometry` stays YAML-free (plain marker). The junction *name-set* injection rule (fabric config → `hubgeometry` as `[]string`) is unchanged; only the *home* of `pathspec` moves (per-worktree → repo-wide). **CONSTRAINTS.md must be updated in the same commit** to record the recorded-anchor resolution and the cwd-gate.
- **CLI/Cobra Invariant** — module `Command()`/`RunCLI` seam, `Short` on every command, help-tree tests. Deleting `initcli` and adding `lyx fabric unwire` changes the command tree; the help-tree tests and the module list (main.go:87) update in the same commit.
- **Documentation Lifecycle** — see Testing/Task-completion below.

## Testing

- **`hubgeometry.Resolve` (primary TDD target)** — table tests for: root anchor (`.`) resolves `RelPath="."` from any cwd under the root; subpath anchor (`backend`) resolves `RelPath="backend"` when cwd is at or below `<root>/backend`; cwd **outside** the anchored subtree (sibling `frontend/`, or the repo root above a subdir anchor) is a **hard error**; marker **absent** falls back to today's cwd-derived `RelPath` (mid-clone / lyxtest / non-hub). Reuse `lyxtest` for synthetic hubs (respect the lyxtest Leaf Invariant — no feature-package imports).
- **`CloneHub`** — create path: `--subpath backend` with an existing `backend/` writes `.fabric-anchor`=`backend` + repo-wide `fabric.yaml` on `weft:main`, wires junctions, echoes `anchor`; `--subpath backedn` (nonexistent) hard-errors and tears down (extend the existing strict-abort teardown tests, clone_test.go/clone_adopt_test.go). Adopt path: re-clone reads the recorded subpath; a conflicting `--subpath` on re-clone hard-errors; a matching one succeeds. Root default (`.`) writes `.` explicitly.
- **Reconcile convergence** — extend reconcile_stale_registration_test.go et al.: a junction in `pathspec` but missing on disk is wired; a junction on disk but absent from `pathspec` is **removed** (the new behavior); a correct junction is no-op; the sweep converges **all** worktrees to the one repo-wide `pathspec` (add `_raddle` to the repo-wide set → every worktree gets `_raddle`).
- **`worktree add` eager wiring** — Add wires `_lyx`/`_pattern` immediately (no dormant state); `rollbackAdd` unwires on a mid-add failure. Update add_test.go/add_rollback_adopt_test.go.
- **`lyx fabric unwire`** — mirrors the old `init --undo` tests (undo_test.go is the reference): removes all junctions + exclude entries, clears/commits/pushes weft `_lyx` **only** (never `_pattern`), reverts the `.gitignore` block; idempotent; no-ops on an unpaired/never-wired host.
- **`init` deletion** — help-tree tests no longer list `init`; the CLI tree still builds; no dangling references to `initcli`/`initengine`.
- **Follow `golang:golang-testing`** conventions; run `golang:golang-build` before handoff.

## Task-completion / docs (same commit)

- `CONSTRAINTS.md` — update the Hub Geometry Invariant (recorded-anchor `RelPath` resolution + cwd at-or-below gate) and the CLI/Cobra command-tree change.
- `internal/fabricengine/doc.go` — document slice 5's shipped behavior (subpath-in-weft, clone-does-everything, repo-wide `pathspec`, declarative reconcile with stale-removal, `unwire`), per the design's "durable parts fold into `fabricengine`'s package doc when this lands".
- `manifest/designs/fabric-unified-view.md` — mark slice 5 **DONE** in the Build order and record the resolved decisions (the "RelPath record-vs-cwd reconciliation mechanism" open question is now answered). Do **not** delete the file — slice 6 remains.
- `docs/overview.md` — remove `init` from the module list / execution stack; update the clone-does-everything and repo-wide-`pathspec` description of the junction geometry.
- `manifest/roadmap.md` — no move (bugfix/hardening rule does not apply, but this is a mid-campaign slice, not a completed/added roadmap item; the fabric-unified-view entry stays until the whole campaign lands).

## Q&A log

- **Q:** Full slice 5 in one task, or split 5a/5b? **A:** Full slice 5 — the three parts are coupled and the filed brief names all three.
- **Q:** Where does the recorded subpath live, without the `fabric.yaml`-under-`RelPath` circularity? **A:** On the `weft:main` branch (the "board" branch — `_board` is only the incidental checkout dir name), read via its checkout at `BoardDir(Hub)`. One repo-wide record, not per-worktree.
- **Q:** After `RelPath` is recorded, what is cwd's role? **A:** Record wins; cwd must be at or below the anchor or it is a **hard error** (running lyx outside the folder it is anchored for must fail loudly). Marker absent → fall back to cwd (defensive only).
- **Q:** File layout for the two per-repo facts? **A:** Two files on `weft:main` — plain `.fabric-anchor` (subpath, read by hubgeometry, YAML-free) + `fabric.yaml` (`pathspec` + `branch_prefix`, read by fabricengine).
- **Q:** Canonical value for the common root case? **A:** Always write `.` explicitly on a new clone; no "absent = assume root" for fresh hubs.
- **Q:** How to guard against a typo in first `--subpath`? **A:** No prompt — validate the subpath directory exists in the cloned host worktree; hard error if not. Echo the resolved anchor in the JSON result.
- **Q:** Re-clone when `--subpath` is passed anyway? **A:** Validate against the record; hard error on mismatch (never silently re-anchor).
- **Q:** Weft remote at first clone? **A:** Must pre-exist (empty is fine). No provisioning by clone.
- **Q:** Fate of `lyx init`? **A:** Delete `initengine`/`initcli` + unregister; move `--undo` teardown to `lyx fabric unwire`. `fabric clone` does everything now.
- **Q:** `configsync.ReconcileAll` placement? **A:** Clone calls it once; config ownership stays in `configsync`.
- **Q:** Implementation strategy for the new clone path? **A:** Modify `CloneHub` in place, test-fenced — no parallel coexisting entry point.
- **Q:** Existing hubs without the (newly-introduced) marker? **A:** `Resolve` falls back to cwd when absent — defensive robustness, not migration. The one real pre-existing hub (Sandbox) gets a **one-time marker write** into its `weft:main` as a separate follow-up chore (out of this task's code).
- **Q:** Eager wiring at `worktree add`? **A:** Yes — fold `WireJunctions` into `Topology.Add`; fabric becomes the sole end-to-end wirer.
- **Q:** Activate raddle in one worktree — wire in all worktrees? **A:** Yes. Junction activation is **per-repo**, not per-worktree, so `pathspec` moves to the repo-wide record and `reconcile` converges every worktree to it.
- **Q:** Post-hoc single-junction wiring — imperative verbs or reconcile? **A:** Declarative — update `pathspec`, then `lyx fabric reconcile` converges (add missing / remove stale / no-op correct). No `wire [names]`/`unwire [names]` imperative verbs; `lyx fabric unwire` is the distinct full-deactivation.
