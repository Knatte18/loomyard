# Conflict Resolution Brief

Your sole job is to resolve git conflict markers in the listed files, stage each resolved file, and report success. Do NOT commit. Do NOT run `git merge --continue` — the SKILL does that after receiving `{"status":"success"}`.

## Task intent

These excerpts describe what THIS branch is trying to accomplish. When the merge introduces a parent-side change that conflicts with this branch's intent, the resolution preserves THIS branch's intent. In particular: if a file appears under a batch's `Deletes:` list and the merge introduces a modified version of that file from the parent, the resolution is to delete the file (your branch's intent overrides). Stage the deletion with `git -C /home/knatte/Code/loomyard/wts/fabric rm <file>`.

### From discussion.md

# Discussion: fabric: unify warp + weft into one git-coordination module

```yaml
task: 'fabric: unify warp + weft into one git-coordination module'
slug: fabric
status: discussing
parent: main
```

## Problem

The shipped `warp` (host↔weft git topology: clone, dual-worktree add/remove, coordinated
checkout, reconcile, pairs, prune, cleanup) and `weft` (git into the paired weft repo:
status/commit/push/pull/sync) modules split one concern — coordinating two paired git
repos — across two coupled packages, each parsing raw git output itself. `internal/gitrepo`
(the generic single-repo primitive layer) has now landed standalone with zero production
consumers, exactly so a unified module can be built on it. `fabric` is that module: a
full, no-remainder replacement for both `warp` and `weft`, built on two `gitrepo.Repo`
instances plus the cross-repo coordination neither layer has today (`SyncWeft` with a
`Warp-SHA` commit trailer, `RevertWithWeft`, a rebuildable correspondence index).

**This task builds fabric alongside warp/weft — it does NOT replace them.** Full design:
`manifest/designs/fabric.md` (Build order step 1). The coordinated cutover that rewires
consumers and deletes warp/weft is a later, separate task (step 2).

## Scope

**In:**

- New `internal/fabricengine` + `internal/fabriccli` implementing everything warp and
  weft do today, plus the new coordination surface (`SyncWeft`, `RevertWithWeft`,
  `Warp-SHA` trailer, correspondence index).
- `lyx fabric` registered in `cmd/lyx` alongside `lyx warp` / `lyx weft` (flat tree,
  14 verbs — see Decisions).
- Growing `internal/gitrepo` with the generic git mechanics it lacks: fast-forward
  pull and a SHA-validated hard reset (for `RevertWithWeft`). Pathspec-scoped staging
  already ships (`StageAndCommit` accepts an explicit pathspec list, directories
  included); push serialization reuses the existing `PushCoalesced` lock; the weft
  write lock stays at the fabric layer (an `internal/lock` flock around gitrepo calls,
  the pattern board-use-gitrepo landed).
- New uniform branch-naming scheme enforced by fabric: host `<slug>` ↔ weft
  `<slug>-weft`, no exceptions, primary worktree included (host `main` ↔ weft
  `main-weft`), effective from `lyx fabric clone`.
- Differential back-to-back integration tests proving fabric reproduces warp/weft
  behavior on the same fixtures.
- New sandbox suite file with `**Covers:** fabric` scenarios, run against the dedicated
  empty test repos (see Testing).
- One `fabric.yaml` config registered in configreg.
- CONSTRAINTS.md Weft Git Invariant updated for the parallel-build period; docs updated
  per Documentation Lifecycle (same commits as the code).

**Out — explicitly NOT in this task:**

- **No cutover.** No consumer (`initengine`, `loomengine`, `buildercli`, `webstercli`,
  `perchcli`, `configcli`, `cmd/lyx` wiring of warp/weft) is rewired to fabric. warp and
  weft stay registered, shipped, and untouched. This must stay crystal clear in every
  downstream instruction: fabric EXISTS SIMULTANEOUSLY with warp/weft and is validated
  back-to-back against them.
- No deletion of `warpengine`/`warpcli`/`weftengine`/`weftcli` or their configs/tests.
- No migration of existing hubs to the new branch-naming scheme. Existing hubs keep
  mirrored same-name branches until cutover.
- No changes to warp/weft behavior; the existing sandbox repo and warp/weft sandbox
  suites stay untouched.
- Push-timing policy (after every commit / every N / end of plan) — a webster/raddle
  policy decision, deliberately not opinionated by fabric (per design doc).

## Decisions

### Parallel build, no cutover

- Decision: fabric is built complete and registered, coexisting with warp/weft. The old
  modules serve as the reference fixture; validation is back-to-back equivalence testing.
  Cutover (rewire consumers, delete old modules, migrate hubs) is a separate future task.
- Rationale: warp/weft are tightly coupled to how git state is read across the codebase;
  the design doc mandates parallel-build-then-cutover. Operator answered explicitly:
  "fabric skal eksistere SAMTIDIG med warp og weft-modulene, og testet back to back."
- Rejected: doing both phases in this task (originally recommended, overruled by
  operator).

### Module structure and naming

- Decision: `internal/fabricengine` (engine, returns `(T, error)`, no cobra/io.Writer) +
  `internal/fabriccli` (cobra tree, JSON envelope). Central type is `fabricengine.Fabric`
  holding `Warp *gitrepo.Repo` and `Weft *gitrepo.Repo` fields exposed directly — no
  forwarding-method-per-operation. `Trunk` (the design-doc sketch's type name) is an
  obsolete name for this module and must not be used.
- Rationale: CLI/Cobra Invariant's `<module>cli`/`<module>engine` split; design doc's
  rejected-alternatives list already rules out forwarding methods and nested internal
  packages (flat structure).
- Rejected: `fabricengine.Trunk` (fossil vocabulary); nested `fabric/internal/warp`
  packages.

### CLI: flat `lyx fabric` tree

- Decision: one flat command tree:
  `lyx fabric clone|add|list|remove|checkout|pairs|reconcile|prune|cleanup|status|commit|push|pull|sync`.
  Topology verbs map to today's `lyx warp` verbs one-to-one; weft verbs map to today's
  `lyx weft` verbs one-to-one. `status` is unambiguously the weft status (pair status is
  `pairs`, same as today's warp).
- Rationale: no name collision exists between the two verb sets; flat matches today's
  verbs one-to-one. Operator rejected a nested `fabric weft ...` subgroup as solving a
  non-problem.
- Rejected: `lyx fabric weft <verb>` subgroup; keeping `lyx warp`/`lyx weft` command
  names over the fabric engine.

### Most git mechanics grow into gitrepo

- Decision: generic single-repo git operations move down into `internal/gitrepo`:
  fast-forward pull and a hard reset (`ResetHard(sha)`-style, needed by
  `RevertWithWeft`; the caller-supplied SHA is validated as plain hex exactly like
  `SHAExists`/`ChangedFilesSince` do). Pathspec-scoped staging is NOT a gap —
  `StageAndCommit` already stages an explicit caller-supplied pathspec list
  (directories included); fabric supplies the scoped list. Push serialization reuses
  gitrepo's existing `PushCoalesced` / `.gitrepo-push.lock` rather than porting
  weftengine's separate push lock. **The write lock does NOT move into gitrepo** —
  the merged board-use-gitrepo work set the precedent: consumers serialize their own
  writes with an `internal/lock` flock held *around* gitrepo calls (board:
  `tasks.json.lock`/`tasks.json.push.lock` around `StageAllAndCommit`/`Push`). fabric
  follows suit: the weft write lock (weftengine's `.weft/weft.write.lock` equivalent)
  lives at the fabric layer around `StageAndCommit`. This supersedes the earlier
  "lock serializes every commit path" consequence — board's wildcard path is already
  serialized by board's own locks, and a gitrepo-level lock would double-lock it. fabric itself keeps only what is genuinely
  coordination or policy: two-repo operations (SyncWeft, RevertWithWeft, coordinated
  topology), `SkipGit`/`SkipPush` env gating (`EnvSyncOptions` equivalent), and pathspec
  configuration.
- Rationale: operator: "Mesteparten av gitoperasjoner skal i gitrepo. Derfor eksisterer
  den modulen." gitrepo is the designated home for repo-agnostic git mechanics.
- Rejected: keeping Pull/pathspec staging inside fabric (strands generic ops in a
  coordination module); pushing policy (env gating, pathspec config) into gitrepo (bloats
  the primitive layer).

### SyncWeft: behavior parity plus the Warp-SHA trailer

- Decision: `SyncWeft` reproduces today's weft sync observable behavior — pathspec-scoped
  staging, commit under write lock, push with rebase-retry, `SkipGit`/`SkipPush` gating —
  with one deliberate delta: every weft commit carries a `Warp-SHA: <sha>` trailer
  recording the warp SHA it corresponds to, and `RecordCorrespondence` updates the index
  alongside. Push/recording sequencing is split by call path:
  - **Engine method `SyncWeft` is synchronous:** commit-with-trailer → in-process push →
    re-read `CurrentSHA` (a push that recovered via rebase rewrites local SHAs — gitrepo's
    documented contract) → `RecordCorrespondence` with the post-push SHA. This is the
    canonical coordinated operation.
  - **CLI `sync` verb keeps today's parity:** commit-with-trailer + detached push spawn
    (as weftcli's `spawnPush` does today). The index records the commit SHA immediately,
    pre-push; in the rare case a rebase-recovery rewrites it, lookups self-correct — index
    entries are validated with `SHAExists` and a stale entry triggers `RebuildIndex`
    (trailers live in commit messages, survive rebase replay, and are the sole source of
    truth).
  - **Async push stays first-class:** `SkipPush` gating, the detached push path, and
    gitrepo's `PushCoalesced` remain available so consumers are never forced to wait on
    a synchronous push. This is an operator constraint for board's sake — but note board
    will use gitrepo's push primitives (`PushCoalesced`) **directly** on `weft:main`,
    NOT fabric's `SyncWeft`/`RevertWithWeft` coordination API (board-weft-storage.md is
    explicit on this); what must stay first-class is the gitrepo-level non-blocking
    push, not a fabric entry point.
- Rationale: cutover safety demands parity (the old modules are the reference fixture);
  the trailer is fabric's core new capability. A detached fire-and-forget push can never
  hand the final SHA back to the committer, so the detached path leans on the
  already-decided self-correction machinery instead of pretending to know the post-push
  SHA.
- Rejected: simplifying away env gating or the detached spawn during the move (mixes
  behavior redesign into the replacement); making the detached push process update the
  index itself post-push (moves coordination logic into the push verb).

### Correspondence index: gitignored local cache

- Decision: the warp↔weft SHA correspondence index is a local, never-committed cache
  file stored **inside the weft worktree's git directory** (resolved via
  `git rev-parse --git-dir`, which in a linked worktree names the per-worktree gitdir —
  deliberately so: the index is **per-worktree**, i.e. per host↔weft pair, covering that
  pair's own weft branch history; `RebuildIndex` scans the current weft branch's
  trailers) — not in the working tree, so no `.gitignore` entry is needed or written.
  **Layering:** the index component takes an explicit file path and never touches git
  itself; the fabric layer owns gitdir resolution and hands the path in. Atomic-write/
  lock handling follows `internal/state`'s patterns. Sorted for binary-search "nearest
  older" lookup. API per design doc: `RecordCorrespondence`,
  `WeftSHAForWarpSHA`, `RebuildIndex` (full trailer scan via `git interpret-trailers`,
  reconstructs the cache; trailers in weft history are the sole source of truth).
- Rationale: the index is pure derived state, per-worktree rebuildable from that
  worktree's branch trailers; sharing it (e.g. via a snapshot ref) adds sync complexity
  for no gain.
- Rejected: snapshot-ref storage à la `refs/loomyard/...`; a committed mapping file
  (already rejected in the design doc — can drift).

### RevertWithWeft: nearest-older with explicit gap report

- Decision: `RevertWithWeft(warpSHA)` resets **both** repos — the method owns the warp
  reset; it is not left to the caller. **Ordering:** correspondence resolution (exact /
  gap-with-range / no-older → error) happens FIRST, before any reset, so the error path
  mutates nothing. Then warp is reset to `warpSHA`, then weft to the resolved point.
  **Partial-failure posture:** if the weft reset fails after the warp reset succeeded,
  warp is rolled back to its pre-revert SHA (mirroring Checkout's all-or-nothing,
  host-rollback-on-weft-failure discipline); if that rollback itself fails, the method
  returns a typed error reporting both repos' states loudly. Both resets go through
  gitrepo's new SHA-validated reset method (see "Most git mechanics grow into gitrepo").
  When the target warp SHA has no exact weft correspondence, it uses the nearest older
  correspondence and returns a typed result stating
  exact-match vs gap (including the warp-SHA range in the gap) so the caller can flag
  weft/raddle as stale. Error only when no older correspondence exists at all. All stored
  SHAs (trailer values, index entries) are checked with `SHAExists` before use.
- Rationale: weft does not sync per warp commit, so most warp SHAs have no exact match;
  a hard error would make revert unusable. Resolves the design doc's open question as it
  leaned.
- Rejected: hard error on missing exact match; silently treating nearest-older as exact.

### Stale SHA handling: typed error, no auto-recovery

- Decision: when `SHAExists` reveals a stored SHA reference (trailer value, index entry,
  snapshot value) no longer exists (rebase/amend/force-push), fabric surfaces a typed
  error with context. It never auto-triggers recovery; the human/orchestrator chooses
  (which may be running `RebuildIndex` or a re-sync).
- Rationale: explicit-over-implicit is the project line; the right recovery is
  situation-dependent. Resolves the design doc's second open question.
- Rejected: automatic rebuild/re-sync on staleness detection.

### Branch naming: uniform `<slug>` / `<slug>-weft`, primary included, no migration

- Decision: fabric enforces host branch `<slug>` ↔ weft branch `<slug>-weft`
  uniformly, with no exceptions — including the primary worktree (host `main` ↔ weft
  `main-weft`), established from `lyx fabric clone` onward. `weft:main` is thereby never
  claimed (the board-weft-storage requirement). **Composition rule:** the weft branch is
  always the full host branch name plus the `-weft` suffix —
  `weftBranch = hostBranch + "-weft"`. For task worktrees the host branch is
  `branch_prefix + slug` (as warp builds it today), so a non-empty prefix yields e.g.
  host `hanf/foo` ↔ weft `hanf/foo-weft`. The primary's host branch is whatever clone
  checks out (`main`), never prefixed, so its weft branch is `main-weft`. No migration
  of existing hubs: today's warp-created hubs use mirrored same-name branches in both
  repos and keep them until cutover.
- Rationale: this is a deliberate behavior change fabric introduces — today NOTHING
  implements `-weft` branch naming (`hubgeometry.WeftSuffix` governs sibling *directory*
  names only; `warpengine/add.go` creates the identical branch name in both repos).
  board-weft-storage.md explicitly delegates this enforcement to fabric.
- Rejected: migration in this task (operator: "DU trenger ingen migrering"); keeping
  mirrored names in fabric (blocks board-weft-storage).

### Self-contained junction/portal/launcher mechanics

- Decision: fabricengine gets its own implementations of the junction/portal/launcher/
  post-checkout-hook filesystem mechanics, adapted from warpengine's unexported code.
  Deliberate duplication for the parallel period, so cutover is a pure deletion of the
  old modules.
- Rationale: warpengine's mechanics are unexported and the design doc rejected extra
  package nesting; a shared helper package would be permanent structure for a temporary
  overlap. All links go through `internal/fslink` (directory junctions on Windows),
  all geometry through `internal/hubgeometry`.
- Rejected: extracting a shared package both modules import; exporting warpengine
  internals for fabric to import.

### Clone: full parity, board repo included

- Decision: fabric's `clone` replicates warp's `CloneHub` behavior — clones host,
  weft, AND the board repo into `<name>-HUB`, with the board URL optional and the same
  `resolvedBoardURL` return and strict-abort teardown. "Replicates" is scoped to the
  three-repo cloning, teardown, and `resolvedBoardURL` contract: the one deliberate
  delta is the Branch naming decision — fabric's clone checks the weft primary out on
  `main-weft` (warp leaves it mirrored on `main`), normalized by the differential clone
  test. It lives as a package-level
  function in fabricengine (clone runs before any `Fabric` instance exists); the
  `Fabric` struct still holds only `Warp`/`Weft` — the board is cloned, not coordinated.
- Rationale: no-remainder parity, and the differential clone test asserts equivalent end
  state including board setup. Moving board's storage into `weft:main` is a separate,
  already-planned task (board-weft-storage) — until it ships, board remains a third
  cloned repo, and fabric must not preempt that design.
- Test seam: fabric gets its own `RemoveAll`-equivalent teardown seam (mirroring warp's
  exported `RemoveAll` var), self-contained like the rest of fabric's mechanics — the
  differential clone test tears each side down via its own module's seam, never warp's
  seam against fabric-cloned repos.
- Rejected: dropping board from fabric clone (breaks parity and the differential test);
  reusing warp's `RemoveAll` seam for fabric's teardown (couples fabric tests to the
  module being deleted at cutover).

### Complete exported surface in the parallel build

- Decision: fabric implements equivalents of warp's consumer-facing helpers now —
  `PairInSync` and `HostClean` (loom's preflight checks) included — even though no
  consumer calls them until cutover. The parallel build delivers the complete
  replacement surface; dead-until-cutover exported functions are expected and validated
  by the differential tests.
- Rationale: "everything either module does today moves into fabric"; a plan writer
  needs the full exported surface enumerated, and cutover must be a pure rewire with no
  gap-filling implementation work.
- Rejected: deferring loom-preflight helpers to the cutover task.

### Coordination with board-use-gitrepo — RESOLVED by merging its branch in

- Decision: `board-use-gitrepo` completed implementation (crucible hardening still
  running on its side), and its branch has been **merged into this task branch** at the
  operator's direction — the concurrency concern from the ad hoc review is resolved:
  fabric's plan is written against gitrepo's real, merged surface, and the anticipated
  `doc.go` conflict was pre-empted. What the merge brought in: `StageAllAndCommit`
  (wildcard `add -A` + commit — **board's opt-in exception**; gitrepo's `doc.go`
  explicitly requires fabric, raddle, and codeintel to keep using explicit-list
  `StageAndCommit`), rewritten push-surface docs, and boardengine rewired onto gitrepo
  (its hand-rolled `git.go` deleted) — making boardengine gitrepo's first production
  consumer. Any further hardening changes on that task arrive via the standard merge-in
  of the parent branch once it lands on `main`.
- Rationale: planning against a stale gitrepo snapshot guaranteed rework; the merge
  makes the surface concrete. fabric must NOT use `StageAllAndCommit` — SyncWeft's
  pathspec-scoped explicit list is exactly what `StageAndCommit` covers.
- Rejected: planning against the pre-merge snapshot and resolving conflicts at land
  time.

### Config: one `fabric.yaml`

- Decision: one `fabric.yaml` (via `hubgeometry.ConfigFile`) carrying both settings —
  branch prefix (warp.yaml's `BranchPrefix` equivalent) and pathspec (weft.yaml's
  `Pathspec` equivalent) — registered in `internal/configreg` alongside the existing
  warp.yaml/weft.yaml registrations. Cutover later removes the two old registrations.
- Rationale: one module, one config file; reading the old modules' config files would
  couple fabric to what it replaces.
- Rejected: fabric reading warp.yaml + weft.yaml.

### CONSTRAINTS.md: Weft Git Invariant amended now

- Decision: in the same commit that lands fabric's weft-touching code, the Weft Git
  Invariant is amended: weft-internal git goes through `weftengine` **or**
  `fabricengine`; coordinated topology through `warpengine` **or** `fabricengine`; with
  an explicit parallel-build note to be removed at cutover. The agent-never-drives-weft-
  git half is unchanged and applies to fabric identically.
- Rationale: otherwise fabric's own tests and sandbox runs formally violate a written
  invariant for the whole parallel period.
- Rejected: leaving the invariant untouched until cutover.

### Documentation

- Decision: `docs/overview.md` module table gains a fabric row (marked as parallel-build,
  not yet the owner — warp/weft rows stay). `manifest/designs/fabric.md` is NOT deleted
  (Documentation Lifecycle deletes it when the module fully lands, i.e. at cutover);
  its status note is updated to record that the parallel build is done and only cutover
  remains, and durable rationale starts folding into `fabricengine`'s package doc. The
  roadmap's fabric item stays in Planned, amended to record the parallel build landed
  and cutover remains.
- Rationale: CLAUDE.md's Task completion rules (docs in same commit); the design doc's
  own lifecycle note; roadmap items move to Done only when complete.

## Technical context

- **`internal/gitrepo`** (first production consumer: `boardengine`, since the merged
  board-use-gitrepo work; fabric is its second):
  `New(path)` (no validation, no I/O), `CurrentSHA`, `StageAndCommit(msg, files) (sha,
  committed, err)` (explicit file list), `StageAllAndCommit(msg)` (wildcard `add -A` —
  board's opt-in exception, off-limits to fabric per `doc.go`), `ChangedFilesSince`, `SHAExists`
  (bool-swallowing posture; validates SHA-shaped args → `ErrInvalidSHA`), `Push`
  (rebase-retry on non-fast-forward/rejected/fetch-first), `PushCoalesced`
  (cross-process coalescing via `.gitrepo-push.lock` in the worktree root),
  `SnapshotSHA`/`SetSnapshotSHA` (`refs/loomyard/snapshot/<key>`, fetch-before-read,
  fast-forward-only with adopt-on-conflict). Repo topology (clone, worktree add) is
  explicitly NOT gitrepo's job — fabric builds those directly on `internal/gitexec`.
  Contract gotcha: after a push that recovered via rebase, pre-push SHAs may be
  off-history — re-read `CurrentSHA` before recording anything.
- **`internal/warpengine`** — the topology reference. `Worktree` handle (`New(cfg)`),
  methods `Add` (transactional paired create: clean-check → branch → host worktree →
  weft worktree create-or-adopt → portal junction → launchers → push both; best-effort
  `rollbackAdd`), `Remove`, `Checkout` (all-or-nothing host+weft switch, host rollback
  on weft failure, forks missing weft branch from the parent's weft branch, re-points
  junctions), `Reconcile` (repair-and-adopt sweep), `Status` (pairs + drift +
  pollution), `Prune`, `Cleanup` (weft branches without host sibling; dry-run/apply/force
  matrix with raddle-fold-back gate), `List`; package-level `CloneHub(cwd, hostURL,
  weftURL, boardURL)` (clones into `<name>-HUB`, strict-abort teardown; `DeriveHostName`;
  `RemoveAll` test seam), `PairInSync`, `HostClean` (used by loom preflight);
  `WireJunctions`/`UnwireJunctions` (used by initengine), `InstallPostCheckoutHook` +
  `post-checkout.sh`. Unexported: portals, launchers, weftwiring helpers, hostlayout.
- **`internal/weftengine`** — the weft-git reference. `Status(weftWorktree, pathspec)`,
  `Commit(weftPath, pathspec, message, opts)` (pathspec-scoped staging under
  `.weft/weft.write.lock`), `Push` (rebase-retry + push lock), `Pull` (fast-forward),
  `SyncOptions{SkipGit, SkipPush}` + `EnvSyncOptions()` (env `WEFT_SKIP_GIT`/
  `WEFT_SKIP_PUSH`), `ScopedPathspec`, `DefaultCommitMessage = "weft sync"`.
  `weftcli sync` = commit + detached push spawn (`spawn.go` launches
  `lyx weft --weft-path <abs> push` detached).
- **Branch naming today:** one derivation formula — `branch := w.cfg.BranchPrefix +
  slug` — applied at several sites (`warpengine/add.go:89`, `warpengine/remove.go:49`,
  and the branch handling in checkout/reconcile); host and weft branches are mirrored
  identical names. `hubgeometry.WeftSuffix = "-weft"` is directory naming only. The
  `<slug>-weft` branch scheme is NEW with fabric.
- **Cutover blast radius (context for the FUTURE task — untouched now):** `cmd/lyx`
  (registration + pinned help-tree/registration/longlist test sets), `configreg` (both
  templates), `initengine` (`WireJunctions`, `UnwireJunctions`, weft sync quartet),
  `loomengine/preflight.go` (`HostClean`, `PairInSync`), `buildercli`/`webstercli`/
  `perchcli` (weft quartet: `EnvSyncOptions`/`ScopedPathspec`/`Commit`/`Push`),
  `configcli.go:395` (`weftcli.RunCLI(w, []string{"sync"})`), `tools/sandbox/main.go`
  (shells `lyx warp clone`), lyxtest leaf-enforcement test (names warp/weft as banned
  imports — fabricengine must be added to the banned list for lyxtest in this task).
- **Registering `lyx fabric`** requires, in the same commit: `newRoot()` import +
  `AddCommand`, root `Long` module-list entry, and updates to the pinned sets in
  `cmd/lyx/drift_test.go` / `helptree_test.go` / `registration_test.go` /
  `longlist_test.go`, plus sandbox coverage (below).
- **Geometry:** all cwd/worktree resolution via `hubgeometry.Getwd()`/`Resolve()`;
  geometry tokens (`-weft`, `-HUB`, `_lyx`, `_portals`, `_launchers`, `_raddle`,
  `_board`) may not appear in path construction outside hubgeometry — fabric composes
  paths only through hubgeometry helpers. Enforced by
  `TestEnforcement_GeometryLiterals` on every `go test`.
- Known-stale doc (independent of fabric, fix opportunistically at cutover, not now):
  `docs/overview.md` still lists warp's `status` verb; it was renamed `pairs`.

## Constraints

From `CONSTRAINTS.md` (authoritative; read it before writing code):

- **Hub Geometry Invariant** — hubgeometry owns all cwd/geometry/config paths; token ban
  machine-enforced.
- **lyxtest Leaf Invariant** — lyxtest never imports feature packages; add
  `fabricengine`/`fabriccli` to the banned-import expectations.
- **CLI / Cobra Invariant** — `Command()`/`RunCLI` seam, `Short` on every command,
  JSON envelope (`output.Ok`/`output.Err`), `GroupRunE` on the parent, pinned help-tree
  test sets updated in the same commit, `<module>cli`/`<module>engine` split (engine
  returns `(T, error)`, never imports cobra/cli).
- **Weft Git Invariant** — amended this task (see Decisions): weft git via weftengine OR
  fabricengine; agents never drive weft git — fabric's CLI is driven by Go orchestration
  or the operator, never by agent prompts.
- **Sandbox Suite Coverage** — registered module ⇒ `**Covers:** fabric` scenario in a
  `tools/sandbox/*SUITE.md` file (or allowlist entry; we add real scenarios instead).
- **Test Tier Purity Invariant** — anything spawning git (`gitexec.RunGit`,
  `exec.Command`, `lyxtest.Copy*`) lives in `//go:build integration`-tagged files.
- **Hermetic Git Test Environment Invariant** — every git-spawning test package has a
  `TestMain` calling `lyxtest.HermeticGitEnv()`.
- **Documentation Lifecycle** — see Decisions/Documentation.

## Testing

- **Differential back-to-back integration tests (the task's central validation):** the
  same lyxtest fixture is copied twice; the warp/weft operation runs on one copy and the
  fabric equivalent on the other; assert equivalent end state — worktree list, branch
  topology, junction/portal/launcher state, weft content and commit effects — modulo the
  one deliberate delta (fabric's `<slug>-weft`/`main-weft` branch names, which the
  comparison normalizes). Cover: add, remove, checkout (existing + missing weft branch),
  reconcile, pairs/status, prune, cleanup, weft commit/push/pull/sync, and clone
  (fabric clone vs warp clone, integration-tagged like warp's).
- **TDD candidates (pure logic, untagged Tier-1 tests):** branch-name derivation
  (`<slug>` ↔ `<slug>-weft`, primary `main` ↔ `main-weft`); correspondence-index
  operations (`RecordCorrespondence`, `WeftSHAForWarpSHA` exact + nearest-older binary
  search, empty index) — exercised against an **explicit temp file path, no git spawn**
  (the index component never resolves gitdir itself, per the Correspondence index
  decision's layering, keeping these untagged under the Test Tier Purity Invariant);
  `RevertWithWeft` gap classification (exact / gap-with-range / no older correspondence
  → error); trailer formatting/parsing round-trip. Gitdir resolution and trailer
  scanning spawn git and are integration-tagged.
- **gitrepo additions** (pull, pathspec staging, write-lock serialization) get their own
  tests in `internal/gitrepo` following its existing integration-tagged + hermetic
  pattern.
- **Trailer/index integration:** SyncWeft writes the trailer (verify via
  `git interpret-trailers`); `RebuildIndex` reconstructs an index equal to the
  incrementally-built one; synchronous `SyncWeft` records the post-push SHA (never a
  pre-push SHA after a rebase-recovered push — simulate a non-fast-forward push); the
  CLI detached path's stale index entry (rewritten SHA) is caught by `SHAExists`
  validation at lookup and healed by `RebuildIndex`.
- **Staleness:** `SHAExists`-gated paths surface the typed stale error after a simulated
  history rewrite (amend/force-push in the fixture).
- **Sandbox:** new `tools/sandbox/SANDBOX-FABRIC-SUITE.md` with `**Covers:** fabric`
  scenarios mirroring the warp/weft ones, run against the dedicated empty test repos
  `https://github.com/Knatte18/lyx-fabric-test` (host) and
  `https://github.com/Knatte18/lyx-fabric-test-weft` (weft) — NOT the existing sandbox
  repo, which stays reserved for warp/weft testing. **Precondition (operator action,
  requested during discussion):** the GitHub wiki on `lyx-fabric-test-weft` must be
  initialized (first page created) so clone's derived board URL
  `<weftURL>.wiki.git` exists; the clone scenario exercises the default derivation
  with no explicit board-url.
- All new test packages: `TestMain` with `lyxtest.HermeticGitEnv()`; spawning tests
  integration-tagged; fabriccli help-tree covered by the existing pinned-set tests.

## Q&A log

- **Q:** Is the cutover (rewire consumers, delete warp/weft) part of this task?
  **A:** No — emphatically. fabric is built and validated ALONGSIDE warp/weft;
  both old modules stay untouched and registered. Cutover is a separate future task.
- **Q:** CLI surface? **A:** Single `lyx fabric` module registered next to
  `lyx warp`/`lyx weft`.
- **Q:** Flat tree or `fabric weft` subgroup? **A:** Flat — no collision exists
  (`pairs` vs `status` disambiguates already); nested subgroup solves a non-problem.
- **Q:** Migration of existing hubs to `<slug>-weft` naming? **A:** None needed. Testing
  happens against a dedicated sandbox; operator provided fresh empty test repos
  (`Knatte18/lyx-fabric-test`, `Knatte18/lyx-fabric-test-weft`) so fabric testing never
  touches the warp/weft sandbox repo.
- **Q:** SyncWeft semantics? **A:** Behavior parity with today's weft sync (pathspec,
  locks, SkipGit/SkipPush, detached push in CLI) + the Warp-SHA trailer as the one
  deliberate addition.
- **Q:** Where do weftengine's generic git extras land? **A:** Most git operations go
  into gitrepo — that is why the module exists. fabric keeps only coordination and
  policy.
- **Q:** RevertWithWeft without exact correspondence? **A:** Nearest-older + typed gap
  report; error only when nothing older exists.
- **Q:** Stale-SHA handling? **A:** Typed error, no auto-recovery.
- **Q:** Config? **A:** One `fabric.yaml` (branch prefix + pathspec) in configreg.
- **Q:** Weft Git Invariant during the parallel period? **A:** Amend CONSTRAINTS.md in
  the same commit as the code (weftengine OR fabricengine), parallel-build note removed
  at cutover.
- **Q:** Junction/portal/launcher code? **A:** Self-contained copies in fabricengine;
  deliberate duplication so cutover is pure deletion.
- **Q:** Type name from the design sketch (`Trunk`)? **A:** Obsolete vocabulary — the
  type is `fabricengine.Fabric`.
- **Q:** (review r1 gap) SyncWeft's detached push contradicts post-push SHA re-read —
  which wins? **A:** Engine `SyncWeft` pushes in-process (post-push re-read +
  `RecordCorrespondence`); the CLI `sync` verb keeps the detached spawn, with stale
  entries self-correcting via `SHAExists` + `RebuildIndex`. Constraint: async push must
  remain first-class — board (on `weft:main` per board-weft-storage) must never be
  forced to wait on a synchronous push.
- **Q:** (review r2 gap) Index TDD tests were claimed Tier-1 but path resolution spawns
  git (`rev-parse --git-dir`) — Tier Purity conflict. **A:** Split layering: the index
  component takes an explicit file path and never touches git (Tier-1 untagged tests);
  the fabric layer owns gitdir resolution (integration-tagged tests).
- **Q:** (review r3 gap) Does fabric `clone` replicate warp's board-repo cloning?
  **A:** Yes — full parity (optional board-url, `resolvedBoardURL`, board cloned into
  the hub), as a package-level function; `Fabric` holds only Warp/Weft. Board's move
  into `weft:main` is the separate, already-planned board-weft-storage task.
- **Q:** (review r4 gap) Where does RevertWithWeft's git reset live? **A:** A new
  SHA-validated hard-reset method on `gitrepo.Repo` — generic ops go to gitrepo.
- **Q:** (review r4 gap) Clone's derived board URL (`<weftURL>.wiki.git`) doesn't exist
  for the fabric test repos — sandbox clone would abort. **A:** Operator initializes the
  GitHub wiki on `lyx-fabric-test-weft`; the scenario tests the default derivation.
- **Q:** Standing instruction for review handling? **A:** From round 4 on, the operator
  pre-authorized auto-picking the recommended resolution for every review finding.
- **Q:** (review r5 gap) RevertWithWeft's error path ran after the warp reset, and a
  weft-reset failure had no stated posture. **A:** Correspondence resolves before any
  reset (error path mutates nothing); weft-reset failure rolls warp back to the
  pre-revert SHA, Checkout-style; rollback failure is a typed both-states error.
- **Q:** (adhoc orchestrator review) board-use-gitrepo concurrently grows gitrepo
  (StageAllAndCommit already committed on its branch) — coordinate how? **A:**
  *(SUPERSEDED by the post-handoff merge entry below — the write-lock clause here was
  retracted: there is no gitrepo-level lock, and fabric never calls
  `StageAllAndCommit`.)* Plan against gitrepo's real surface at plan time; landing
  order stays an operator decision; expect a `doc.go` merge-in conflict.
- **Q:** (adhoc) Does board route through fabric's SyncWeft? **A:** No — board uses
  gitrepo's `PushCoalesced` directly on `weft:main` per board-weft-storage.md; the
  async-push constraint targets the gitrepo layer.
- **Q:** (adhoc) Clone teardown seam? **A:** fabric gets its own `RemoveAll`-equivalent
  test seam; differential test tears each side down via its own module's seam.
- **Q:** (post-handoff) board-use-gitrepo finished — how to consume its gitrepo
  changes? **A:** Operator directed merging `origin/board-use-gitrepo` into this task
  branch; discussion updated against the merged surface. Consequences: gitrepo growth
  shrinks to pull + reset (no gitrepo-level write lock — fabric keeps its weft write
  lock at the fabric layer, `internal/lock` flock around gitrepo calls, per board's
  landed pattern); fabric never calls `StageAllAndCommit`.


### From _mill/plan/00-overview.md


```yaml
task: 'fabric: unify warp + weft into one git-coordination module'
slug: fabric
approved: false
started: '20260725-063143'
parent: main
root: ""
verify: go test ./...
```

### From _mill/plan/01-gitrepo-growth.md


```yaml
task: 'fabric: unify warp + weft into one git-coordination module'
batch: gitrepo-growth
number: 1
cards: 2
verify: go test -tags integration ./internal/gitrepo
depends-on: []
```



- **Edits:**
  - `internal/gitrepo/doc.go`
- **Creates:**
  - `internal/gitrepo/pull.go`
  - `internal/gitrepo/pull_test.go`
- **Deletes:** none
- **Edits:**
  - `internal/gitrepo/doc.go`
- **Creates:**
  - `internal/gitrepo/reset.go`
  - `internal/gitrepo/reset_test.go`
- **Deletes:** none

### From _mill/plan/02-fabric-core.md


```yaml
task: 'fabric: unify warp + weft into one git-coordination module'
batch: fabric-core
number: 2
cards: 5
verify: go test ./internal/fabricengine ./internal/configreg
depends-on: []
```



- **Edits:**
  - `internal/configreg/configreg.go`
  - `internal/configreg/configreg_test.go`
- **Creates:**
  - `internal/fabricengine/doc.go`
  - `internal/fabricengine/config.go`
  - `internal/fabricengine/config_test.go`
  - `internal/fabricengine/template.go`
  - `internal/fabricengine/template.yaml`
  - `internal/fabricengine/template_test.go`
  - `internal/fabricengine/testmain_test.go`
- **Deletes:** none
- **Edits:** none
- **Creates:**
  - `internal/fabricengine/branchname.go`
  - `internal/fabricengine/branchname_test.go`
- **Deletes:** none
- **Edits:** none
- **Creates:**
  - `internal/fabricengine/trailer.go`
  - `internal/fabricengine/trailer_test.go`
- **Deletes:** none
- **Edits:** none
- **Creates:**
  - `internal/fabricengine/corrindex.go`
  - `internal/fabricengine/corrindex_test.go`
- **Deletes:** none
- **Edits:** none
- **Creates:**
  - `internal/fabricengine/fabric.go`
  - `internal/fabricengine/fabric_test.go`
- **Deletes:** none

### From _mill/plan/03-fabric-topology-mechanics.md


```yaml
task: 'fabric: unify warp + weft into one git-coordination module'
batch: fabric-topology-mechanics
number: 3
cards: 5
verify: go test -tags integration ./internal/fabricengine
depends-on: [2]
```



- **Edits:** none
- **Creates:**
  - `internal/fabricengine/hostlayout.go`
  - `internal/fabricengine/ancestors.go`
  - `internal/fabricengine/ancestors_test.go`
  - `internal/fabricengine/portals.go`
  - `internal/fabricengine/launchers.go`
  - `internal/fabricengine/launcher_content.go`
  - `internal/fabricengine/launcher_content_test.go`
- **Deletes:** none
- **Edits:** none
- **Creates:**
  - `internal/fabricengine/junction.go`
  - `internal/fabricengine/hook.go`
  - `internal/fabricengine/post-checkout.sh`
  - `internal/fabricengine/hook_test.go`
- **Deletes:** none
- **Edits:** none
- **Creates:**
  - `internal/fabricengine/weftwiring.go`
- **Deletes:** none
- **Edits:** none
- **Creates:**
  - `internal/fabricengine/clone.go`
  - `internal/fabricengine/clone_test.go`
- **Deletes:** none
- **Edits:** none
- **Creates:**
  - `internal/fabricengine/clone_differential_test.go`
- **Deletes:** none

### From _mill/plan/04-fabric-pair-lifecycle.md


```yaml
task: 'fabric: unify warp + weft into one git-coordination module'
batch: fabric-pair-lifecycle
number: 4
cards: 9
verify: go test -tags integration ./internal/fabricengine
depends-on: [3]
```



- **Edits:** none
- **Creates:**
  - `internal/fabricengine/topology.go`
  - `internal/fabricengine/add.go`
- **Deletes:** none
- **Edits:** none
- **Creates:**
  - `internal/fabricengine/remove.go`
  - `internal/fabricengine/list.go`
- **Deletes:** none
- **Edits:** none
- **Creates:**
  - `internal/fabricengine/checkout.go`
- **Deletes:** none
- **Edits:** none
- **Creates:**
  - `internal/fabricengine/reconcile.go`
- **Deletes:** none
- **Edits:** none
- **Creates:**
  - `internal/fabricengine/status.go`
- **Deletes:** none
- **Edits:** none
- **Creates:**
  - `internal/fabricengine/prune.go`
  - `internal/fabricengine/cleanup.go`
- **Deletes:** none
- **Edits:** none
- **Creates:**
  - `internal/fabricengine/drift.go`
  - `internal/fabricengine/hostclean.go`
- **Deletes:** none
- **Edits:** none
- **Creates:**
  - `internal/fabricengine/lifecycle_differential_test.go`
- **Deletes:** none
- **Edits:** none
- **Creates:**
  - `internal/fabricengine/reconcile_differential_test.go`
- **Deletes:** none

### From _mill/plan/05-fabric-weft-git.md


```yaml
task: 'fabric: unify warp + weft into one git-coordination module'
batch: fabric-weft-git
number: 5
cards: 5
verify: go test -tags integration ./internal/fabricengine
depends-on: [1, 2]
```



- **Edits:**
  - `CONSTRAINTS.md`
- **Creates:**
  - `internal/fabricengine/index.go`
  - `internal/fabricengine/index_integration_test.go`
- **Deletes:** none
- **Edits:** none
- **Creates:**
  - `internal/fabricengine/weftgit.go`
- **Deletes:** none
- **Edits:** none
- **Creates:**
  - `internal/fabricengine/syncweft.go`
  - `internal/fabricengine/revert.go`
  - `internal/fabricengine/revert_test.go`
- **Deletes:** none
- **Edits:** none
- **Creates:**
  - `internal/fabricengine/weftgit_differential_test.go`
- **Deletes:** none
- **Edits:** none
- **Creates:**
  - `internal/fabricengine/syncweft_integration_test.go`
- **Deletes:** none

### From _mill/plan/06-fabric-cli-registration.md


```yaml
task: 'fabric: unify warp + weft into one git-coordination module'
batch: fabric-cli-registration
number: 6
cards: 5
verify: go test -tags integration ./internal/fabriccli ./cmd/lyx
depends-on: [4, 5]
```



- **Edits:** none
- **Creates:**
  - `internal/fabriccli/fabric.go`
  - `internal/fabriccli/clone.go`
- **Deletes:** none
- **Edits:**
  - `internal/fabriccli/fabric.go`
- **Creates:**
  - `internal/fabriccli/weft_verbs.go`
  - `internal/fabriccli/spawn.go`
  - `internal/fabriccli/cli_test.go`
  - `internal/fabriccli/testmain_test.go`
- **Deletes:** none
- **Edits:**
  - `cmd/lyx/main.go`
  - `cmd/lyx/helptree_test.go`
  - `internal/lyxtest/leaf_enforcement_test.go`
- **Creates:**
  - `tools/sandbox/SANDBOX-FABRIC-SUITE.md`
- **Deletes:** none
- **Edits:**
  - `tools/sandbox/main.go`
  - `tools/sandbox/main_test.go`
- **Creates:**
  - `sandbox-fabric-suite.cmd`
- **Deletes:** none
- **Edits:**
  - `docs/overview.md`
  - `manifest/designs/fabric.md`
  - `manifest/roadmap.md`
- **Creates:** none
- **Deletes:** none

## Conflicting files

- `internal/boardengine/board.go`
- `internal/boardengine/sync.go`
- `manifest/roadmap.md`

## Instructions

For each file listed above:

1. Read the file and locate every conflict block (`<<<<<<<`, `=======`, `>>>>>>>`).
2. Understand both sides of the conflict — what each branch intended.
3. Write a resolution that preserves the intent of both sides. When both sides modify **different, non-overlapping parts** of the same conflict region — for example, different columns of one table row, different keys of one object, or disjoint lines of a prose block — **combine both edits** into a single resolved structure. Do NOT pick one side wholesale just because the region overlaps syntactically; picking one side wholesale is correct only when the two changes are genuinely mutually exclusive (e.g. the same key is renamed to two different values). Worked example: if `ours` changes column A and `theirs` changes column B of the same table row, the resolution keeps both column changes in a single row — it does not discard either.
4. Run `git -C /home/knatte/Code/loomyard/wts/fabric add <file>` to stage the resolved file.
5. For modify/delete (DU) conflicts: if Task intent above lists this file under a batch's `Deletes:`, run `git -C /home/knatte/Code/loomyard/wts/fabric rm <file>` instead of editing; that stages the intentional deletion.
6. For UD conflicts — files this branch **modified** that the parent branch **deleted**: do not silently keep the modification. Instead:
   a. Run `git log --diff-filter=D --oneline MERGE_HEAD -- <file>` to find the deletion commit on the parent.
   b. Run `git show <deletion-commit>` to inspect context.
   c. If the deletion commit message mentions a replacement file (e.g. "replaced by", "moved to", "consolidated into"), or the commit also adds a file in the same directory with overlapping content: stage the deletion — `git -C /home/knatte/Code/loomyard/wts/fabric rm <file>`.
   d. If detection is inconclusive: report `{"status":"stuck","stuck_type":"logic","reason":"modify/delete conflict on <file>: cannot determine if parent deletion is a replacement -- operator must decide"}` and halt. Do NOT silently keep the modification.

Never use `git checkout --ours` or `git checkout --theirs` — they silently discard one side of the conflict.

## Report

Your last output line MUST be a bare JSON object (no code fence, no backticks):

On success (nothing discarded):

{"status":"success"}

On success with discarded content — if you had to drop content from one side (e.g. two sides made mutually exclusive changes and only one could survive), list each dropped item:

{"status":"success","discarded":["<short description of what was dropped from which side>"]}

An empty or absent `discarded` field means nothing was lost. If anything was discarded, you MUST list it; an empty list when content was actually dropped is a protocol violation. The `mill-merge-in` frontend reads this field and surfaces any losses to the operator before continuing, rather than silently running `git merge --continue`.

If you cannot resolve one or more conflicts:

{"status":"stuck","stuck_type":"logic","reason":"<one-line description of what you could not resolve>"}

Anything other than this JSON object on the last line is a protocol violation; the merge-in dispatcher treats that as stuck_type: logic with reason "no structured report" — your work is lost. Do not wrap the JSON in a code fence; do not add commentary after it.

## Tools

Available: Read, Edit, Write, Bash, Grep, Glob. Use `git -C /home/knatte/Code/loomyard/wts/fabric` for any git commands; do not `cd`. Worktree cwd is `/home/knatte/Code/loomyard/wts/fabric`.
