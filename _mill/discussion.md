# Discussion: fabric: collapse external API surface onto Commit — stop leaking warp/weft

```yaml
task: 'fabric: collapse external API surface onto Commit — stop leaking warp/weft'
slug: fabric-collapse-external-surface
status: discussing
parent: main
```

## Problem

Fabric's whole job is the one-repo illusion (`manifest/designs/fabric-unified-view.md`): a caller never has to know which of the two physical repos (warp/host and weft) a file lives in. A point-in-time audit (2026-08-02) of every exported `internal/fabricengine` symbol against every real call site outside the package found several verbs that break that illusion by construction — callers reach past the unified `Fabric.Commit` into warp/weft/host-named primitives, so the abstraction only holds by convention, not by structure.

**Why now:** `fabric-v2-crucible` (#023) is meant to review Fabric's external surface holistically and must sequence *after* this task, because this task is what settles that surface. Landing the collapse first keeps Crucible reviewing the intended API rather than the leaky one.

## Scope

**In:**

- Migrate `internal/buildercli/weft.go`, `internal/perchcli/run.go`, `internal/webstercli/weft.go` off `CommitWeft` + synchronous `PushWeftAt` onto the unified `Fabric.Commit` (async push).
- Introduce a Fabric-owned handle type `Bolt` (methods `Commit`/`Push`/`Sync`) for the unpaired weft:main area. Migrate `boardengine.Sync` and `fabriccli/clone.go`'s board-dir operations onto it. Remove `CommitWeftAt`/`PushWeftAt`/`CoalescePush` under their current free-standing names once nothing outside `fabricengine`/`fabriccli` uses them.
- Unexport `CommitWeft` → package-private `commitWeft`. Migrate `fabriccli`'s weft verbs (`internal/fabriccli/weft_verbs.go`) off the exported name too, so the only remaining callers are internal (`syncweft.go`, `unwire.go`).
- Rename `HostClean` → package-level `Clean(l *hubgeometry.Layout)`, extended to check both warp and weft cleanliness. Update its one caller `internal/loomengine/preflight.go`.
- Rename `PairInSync` → `Healthy` (a cheap health check). Update `preflight.go`.
- Dead exported methods: wire `Fabric.Diff` as `lyx fabric diff`; make `Fabric.Status` (unified, both-sides view) **replace** the existing `lyx fabric status` verb, dropping/unexporting the weft-only `StatusWeft` that currently backs it (a weft leak — a `status` that shows only the weft side contradicts the one-repo illusion). `lyx fabric pairs` (topology diagnostic) stays untouched. Revise `doc.go:80` (which frames the three status surfaces as deliberately distinct) in the same commit. Drop/unexport `SnapshotWarpSHA`, `RevertWithWeft`, `SyncWeft`.
- Confirm-and-leave the already-clean sweep symbols: `New`, the narrow git interfaces (`CurrentBranch`/`CheckoutDetached`/`RestoreBranch`/`ResetHard`), `EnvSyncOptions`, `ScopedPathspec`, `SyncOptions`, `ConfigTemplate`, `CoalescePushBothAt`.
- Trim comments in every file this task touches to the new `golang-comments` shape (millhouse#769: what+why doc comments, why-only inline, no mandatory-per-step inline; file-level comments unchanged).
- Bookkeeping: update `fabric-v2-crucible` (#023) `depends_on` to include this slug (per the task body's "Depends on").

**Out:**

- **Warp-binding-in-weft / weft-only `fabric clone` / new `fabric init`** — a good idea raised during discussion, but a distinct bootstrap-UX feature. Filed as separate task `fabric-warp-binding-in-weft`, sequenced after this one. This task only builds the `Bolt` handle forward-compatibly; it persists no warp pointer.
- The `New(warpPath, weftPath)` constructor's naming — accepted as the one legitimate place the two physical worktrees surface at the boundary.
- Board's weft:main / no-warp-pairing case itself — a permanent, accepted exception to the one-repo illusion. This task *names* the exception (via `Bolt`); it does not eliminate it.
- `internal/fabriccli` is not treated as an "external caller" to be walled off — but its use of these primitives IS in scope and gets migrated onto `Commit`/`Bolt`.
- `CoalescePushBothAt` (`coalesce.go:112`, used by `fabriccli/weft_verbs.go` for the *paired* `lyx fabric weft sync`) — the paired counterpart, not the board exception. Stays.
- Comment density anywhere in `internal/fabricengine` outside the files this task otherwise touches. No repo-wide comment sweep.

## Decisions

### cli-push-is-async

- Decision: Migrate builder/perch/webster to `Fabric.Commit` and rely on `Commit`'s existing async, fire-and-forget detached push. **Drop** their explicit synchronous `PushWeftAt` calls. No sync-push option is added to `Commit`.
- Rationale: These CLIs are invoked by LLM agents; a synchronous network push stalls the agent needlessly. `Commit` already commits synchronously under lock (so commit-side errors are still surfaced inline) and only the push is detached — and the coalescing design means the next sync sweeps up anything left unpushed. This is strictly better than today's sync-commit + sync-push.
- Rejected: adding a sync-push option to `Commit` (stalls the agent — the very thing to avoid); keeping `CommitWeft`+`PushWeftAt` (keeps the bypass). Consequence accepted: perch loses its inline "weft sync failed" message for *push* failures specifically (commit failures are still reported).

### bolt-names-the-unpaired-exception

- Decision: New Fabric-owned type `Bolt` with methods `Commit` / `Push` / `Sync`, wrapping a single weft:main-backed repo path. `Sync` owns the absorbing-lock, loop-until-clean coalescing (the logic currently in `CoalescePush`); board supplies only the per-iteration step closure (which keeps doing `ensureLockfilesIgnored` + `commitDirty` under its own `board.lock`). New file (e.g. `bolt.go`) — the name `boardweft.go` is already taken by `ensureBoardWorktree`.
- Rationale: The name must hide warp/weft entirely — from the outside this is "a dedicated local mini-repo area in Fabric" that under the hood lives on weft:main; the reader should not see weft/warp. `Bolt` (a bolt of cloth: a self-contained, finished roll) is on-theme with the repo's textile vocabulary and connotes a standalone unit. Board is the sole sanctioned consumer today, and the weft:main default branch is permanently reserved for it (fabric's `-weft` suffix rule leaves bare `main` unclaimed — see `boardengine/board.go`).
- Rejected: `UnpairedWeft` / `BoardWeft` (leak "weft", or couple the name to the board consumer); handle-owns-all-of-Sync-including-commit with board injecting lock config (bigger change for no gain); free-standing `*At` functions (the name doesn't signal the exception — the whole defect being fixed). Homonym note: "bolt" also means a lock; Fabric already has push locks — accept the mild overload, it lost to no better textile candidate.

### unexport-commitweft

- Decision: Unexport `CommitWeft` to package-private `commitWeft`. Migrate every cross-package caller: the three CLIs (onto `Commit`), and `fabriccli`'s weft verbs + `clone.go` (onto `Commit` / `Bolt`). Internal callers `syncweft.go` and `unwire.go` switch to `commitWeft`.
- Rationale: An exported symbol advertises "meant to be called from outside." This one must not be called from outside `fabricengine`. `fabriccli`'s `lyx fabric weft commit` is reimplemented on `Commit` with a weft-scoped file list — same observable behavior (weft-only commit), plus async push as a consistency bonus.
- Rejected: keeping `CommitWeft` exported as a "sanctioned low-level skin" for `fabriccli` (contradicts the export-means-external-use principle; the user rejected it explicitly).

### clean-package-level-both-sides

- Decision: Rename `HostClean` → package-level `Clean(l *hubgeometry.Layout)`, checking cleanliness of both `l.WorktreeRoot` (warp/host) and `l.WeftWorktree()`. Combined dirty reason when either side is dirty.
- Rationale: `preflight.go` holds a `Layout`, not a `Fabric`; keeping `Clean` package-level preserves the zero-`Fabric` call shape. The name carries no warp/weft/host. The existing host-only `git status --porcelain` (untracked-strict) check is applied to both worktree paths.
- Rejected: making it a `*Fabric` method (forces `New(warp, weft)` construction in preflight for no benefit). Note for the plan: the current doc comment justifies host-only strictness via the Weft Git Invariant; extending to weft means updating that rationale, and confirming the added weft-cleanliness check does not duplicate/conflict with preflight's separate weft pairing/sync check (`Healthy`) and weft-worktree existence stat.

### healthy-rename-keep-cheap

- Decision: Rename `PairInSync` → `Healthy`. Keep it cheap and fast.
- Rationale: The user frames it as a health check and wants "pair" out of the name; `Healthy` also leaves room to check more later. Explicit constraint: it must stay lightweight — NOT a heavyweight scan like millhouse's notoriously slow "wiki healthy". No new expensive work may be added under this rename.
- Rejected: `InSync` (keeps the precise "two sides in step" semantic but the user preferred the health-check framing); leaving `PairInSync` (leaks the pairing concept in the name).

### dead-methods-diff-status-kept

- Decision: Wire `Fabric.Diff` as a new `lyx fabric diff` verb. Wire `Fabric.Status` (unified `[]ChangeEntry`, both sides' uncommitted changes side-labelled) as `lyx fabric status`, **replacing** the current weft-only `StatusWeft`-backed `status` verb; drop/unexport `StatusWeft` (its only consumer is that verb — confirmed no other callers). Drop/unexport `SnapshotWarpSHA`, `RevertWithWeft`, `SyncWeft`.
- Rationale: `Diff`/`Status` give a genuine one-repo-view inspection surface we expect to use; their names are already warp/weft-free. A `lyx fabric status` that reports only weft branch/dirty/ahead/behind (`StatusWeft`) is itself a warp/weft leak — from the outside there is one repo, so `status` must show the unified view. The other three dead methods have no callers, are warp/weft-named, and the task forbids leaving unexercised ambient API — YAGNI.
- Context — fabric has THREE deliberately-distinct status surfaces today (`doc.go:80`): `Topology.Status` (paired host↔weft topology / branch pairing / junction health, via `lyx fabric pairs`); `StatusWeft` (weft-only dirty/ahead/behind, via `lyx fabric status`); and the unified `Fabric.Status` (both sides merged, no verb). This decision collapses that from three to two on the CLI: `lyx fabric pairs` (topology — a legitimately different, non-leaky concern) stays; `lyx fabric status` moves from `StatusWeft` to `Fabric.Status`. `doc.go:80`'s "three deliberately-distinct surfaces" framing MUST be revised in the same commit — this decision overrides that doc on purpose. Note `Fabric.Status` drops the branch/ahead/behind detail `StatusWeft` reported; that sync-state detail is the "which repo" leak and belongs (if anywhere) with `pairs`/topology, not the one-repo `status`.
- Rejected: keep all five (task forbids ambient API); drop all five (loses the useful `Diff`/`Status`); give `Fabric.Status` a distinct non-colliding name like `lyx fabric changes` while keeping the weft-only `status` (rejected by the user — a weft-only `fabric status` should not exist at all).

## Technical context

Key symbols and call sites (re-grep at implementation time; audit may have drifted):

- **`Fabric.Commit`** — `internal/fabricengine/commit.go:126`: `Commit(files []string, msg string, snapshotTags []string, opts SyncOptions) (CommitResult, error)`. Commits both sides synchronously under one combined write lock, releases the lock, then fires an async detached both-sides push via `spawnDetachedPushFn` (only when something landed). `classifyPaths` (`classify.go`, no I/O) routes each entry warp-side vs weft-side by path prefix.
- **KEY IMPLEMENTATION RISK — perch's exclude-magic pathspec.** `perchcli/run.go:422` builds `append(fabricengine.ScopedPathspec(relPath, []string{LyxDirName}), ":(exclude)*.lock")`. `CommitWeft` takes a git *pathspec*; `Commit` takes `files []string` and classifies by prefix. The `:(exclude)*.lock` magic entry must survive through `Commit`'s weft-side `git add`, or perch's lock exclusion must be reworked (perch's locks live inside the scoped `_lyx` and must never be committed). The plan MUST resolve this explicitly. Candidate resolutions: (a) `Commit` tolerates pathspec magic on the weft side; (b) route perch's lock exclusion through fabric's **existing git-exclude backstop** `seedWeftArtifactExcludes` / `crossModuleMachineLocalExcludes` (`weftgit.go:97`, `:116`) instead of a per-call `:(exclude)` pathspec — but note this requires **deepening the exclude pattern**: perch's locks sit two levels deep (`_lyx/perch/<block>/run.lock`, per `run_integration_test.go`), which the current `**/_lyx/*/*.lock` pattern does NOT reach; it would need e.g. `**/_lyx/*/**/*.lock`. This is the single biggest migration hazard.
- **`CommitWeft`** — `weftgit.go:504`. Internal callers to repoint at `commitWeft`: `syncweft.go:48`, `unwire.go:121`. Cross-package callers to migrate: `buildercli/weft.go:138`, `perchcli/run.go:437`, `webstercli/weft.go:135`, `fabriccli/weft_verbs.go:183/222/282`.
- **`CommitWeftAt`** — `weftgit.go:564` (package-level, operates on a bare weft path). Callers: `boardengine/sync.go:102` (→ `Bolt`), `fabriccli/clone.go:77` on `res.BoardDir` (→ `Bolt`).
- **`PushWeftAt`** — `weftgit.go:544`. Callers: the three CLIs (drop — async push via `Commit`), `boardengine/sync.go:77` (→ `Bolt`), `unwire.go:126` (internal), `fabriccli/clone.go:80` (→ `Bolt`).
- **`CoalescePush`** — `coalesce.go:30`. Only external-ish caller: `boardengine/sync.go:86` (absorbs into `Bolt.Sync`). `CoalescePushBothAt` (`coalesce.go:112`, paired) used by `fabriccli/weft_verbs.go:212` — leave.
- **`boardengine/sync.go`** — `Sync(boardPath, skipGit, skipPush)` composes the coalesce loop from the three primitives; `commitDirty` (board.lock write-lock) and `ensureLockfilesIgnored` are board-specific and stay in board's step closure. `boardPath` arrives from `hubgeometry.BoardDir(hub)` per the Hub Geometry Invariant.
- **`HostClean`** — `hostclean.go`, package-level `func(l *hubgeometry.Layout) (clean bool, reason string, err error)`, `git status --porcelain` (untracked-strict). Caller: `preflight.go:100`. `Layout` exposes `WorktreeRoot` and `WeftWorktree()`.
- **`PairInSync`** — caller `preflight.go:120`.
- **Dead methods** — `Diff` `diff.go:84`, `Status` `diff.go:119`, `SnapshotWarpSHA` `snapshot.go:62`, `RevertWithWeft` `revert.go:122`, `SyncWeft` `syncweft.go:47`. `fabriccli` command wiring: `fabriccli/fabric.go` builds the `fabric` parent (verbs `clone/add/list/remove/checkout/pairs/reconcile/prune/cleanup/unwire`), and `addWeftVerbs(cmd)` (`fabric.go:285`) attaches `status/commit/push/pull/sync` **flat onto the same `fabric` command** (weft_verbs.go:295) — so `lyx fabric status/commit/push/pull/sync` are all siblings of `pairs`, not nested under a `weft` subcommand.
- **Three status surfaces** (`doc.go:80`) — `Topology.Status` (`status.go:88`, → `lyx fabric pairs`, `fabric.go:188/459`): paired host↔weft topology, branch pairing, junction health, host-index pollution scan — operator/plumbing diagnostic, legitimately two-repo-aware, LEAVE. `StatusWeft` (`weftgit.go:178`, → `lyx fabric status`, weft_verbs.go:150): weft-only branch/dirty/ahead/behind `map[string]any` — its ONLY caller is that verb; DROP/unexport and repoint the verb. `Fabric.Status` (`diff.go:119`): unified `[]ChangeEntry`, both sides' uncommitted changes side-labelled — becomes the new `lyx fabric status`. `doc.go:80`'s "three deliberately-distinct surfaces" wording must be revised to match (two CLI surfaces after this task: topology `pairs`, unified `status`).
- **`boardweft.go`** — already exists (`ensureBoardWorktree`, bootstrap of the `_board` second worktree). Not the `Bolt` type; do not reuse the file name.
- **Comment trim** — millhouse#769 has landed in the plugin cache; `golang-comments` now says: doc comments = what+why (not internal how), inline comments = why-only (refactor if you need "what"), no mandatory-per-step rule, file-level comments unchanged. Several touched files (`sync.go`, `hostclean.go`, `boardweft.go` if touched) currently carry long how-it-works doc comments that must be trimmed.

## Constraints

From `CONSTRAINTS.md`:

- **Hub Geometry Invariant** — geometry tokens (`_board`, `-weft`, …) are owned solely by `internal/hubgeometry`. `Bolt` must receive its path via `hubgeometry.BoardDir(hub)`; it must never construct the `_board` literal. `Clean` reads `l.WorktreeRoot` / `l.WeftWorktree()` from the injected `Layout`.
- **CLI/Cobra Invariant** — the new `lyx fabric diff` / `lyx fabric status` verbs need a `Short` on every command and must keep the help-tree tests passing; go through the module `Command()`/`RunCLI` seam.
- **Documentation Lifecycle** — this task changes observable CLI behavior (new `lyx fabric diff`; repurposed `lyx fabric status`) and the Fabric external surface, so same-commit doc updates are required: `manifest/designs/fabric-unified-view.md` (the surface it describes); `internal/fabricengine/doc.go:80` (the "three deliberately-distinct status surfaces" paragraph — now two CLI surfaces); and `docs/overview.md` if the module table / execution stack shifts. No new cross-cutting invariant is expected (so no `CONSTRAINTS.md` change), but if the `Bolt` handle or the "warp+host stays pristine" rule warrants one, record it in the same commit.
- **Markdown** — one line per paragraph, no hard-wrap, for any `.md` touched.
- **golang-comments (millhouse#769)** — new trimmed shape, applied to touched files only.

## Testing

- **buildercli / perchcli / webstercli** (`weft.go`, `run.go`): preserve the `committed` bool semantics and the `SkipGit` short-circuit-before-`New` behavior across the `Commit` migration. TDD candidate: a test proving a `*.lock` file under the scoped `_lyx` is NOT committed through the new `Commit` path (guards the exclude-magic risk). Verify commit-side errors still surface; accept that push is now async.
- **`Bolt`**: port the coalescing expectations from `coalesce_test.go` / `commitweftat_test.go` / `coalesce_integration_test.go` onto `Bolt.Commit`/`Push`/`Sync`. Board's `Sync` integration must still coalesce a burst of writes into as few pushes as possible and hold `board.push.lock` once across the loop.
- **`Clean`**: table test — dirty warp only → not clean; dirty weft only → not clean; both clean → clean; git failure → error (not a dirty verdict). Update/extend the preflight integration.
- **`Healthy`**: repoint existing `PairInSync` test references; assert it stays cheap (no new heavyweight work introduced).
- **`Diff` / `Status` CLI verbs**: help-tree test coverage, verb wiring, output-envelope shape; ensure `lyx fabric status` (Fabric.Status) does not collide with the existing `pairs`/`Topology.Status` path.
- **Dropped methods**: delete or fold the now-orphaned tests for `SnapshotWarpSHA` (`snapshot*_test.go`), `RevertWithWeft` (`revert_test.go`), `SyncWeft` (`syncweft*_test.go`), and `StatusWeft` as appropriate. **Cross-dependency:** `diff_integration_test.go` (:59/:66) currently uses `f.SyncWeft(...)` to record correspondence as the setup for the KEPT `Fabric.Diff`/`Fabric.Status` verbs. When `SyncWeft` is dropped, migrate that setup onto `Fabric.Commit` (which also records correspondence) so `Diff`/`Status` keep their integration coverage — do not lose the coverage along with the method.
- **Regression**: full `go test ./...`, the geometry-enforcement test, and the CLI help-tree tests must stay green.

## Q&A log

- **Q:** Sync or async push when the three CLIs migrate to `Commit`? **A:** Async — a sync push stalls the LLM agent; `Commit` already does async detached push and the next sync sweeps up anything unpushed. Drop their explicit `PushWeftAt`.
- **Q:** Keep `CommitWeft` exported for `fabriccli`'s own weft CLI? **A:** No — exported means "meant for external use"; it must not be. Migrate `fabriccli`'s weft verbs onto `Commit` too and unexport.
- **Q:** Name for the unpaired weft:main handle? **A:** `Bolt` — must hide warp/weft (it reads as a standalone local mini-repo backed by weft:main); on-theme textile term. (`UnpairedWeft`/`BoardWeft` rejected for leaking "weft" / coupling to the consumer.)
- **Q:** Is weft↔warp 1:1? **A:** No — many-to-one: one weft belongs to exactly one warp, but one warp can back many wefts. (Informs the spun-off `fabric-warp-binding-in-weft` task, not this one.)
- **Q:** Where does `Clean` live and what does it check? **A:** Package-level `Clean(l *hubgeometry.Layout)`, both warp and weft cleanliness; preflight has a Layout, not a Fabric.
- **Q:** `PairInSync` new name? **A:** `Healthy` — but keep it cheap; explicitly not a slow scan like millhouse's "wiki healthy".
- **Q:** The five dead methods? **A:** Keep + wire `Diff`/`Status` as CLI verbs; drop `SnapshotWarpSHA` / `RevertWithWeft` / `SyncWeft` (no callers, warp/weft-named).
- **Q:** The warp-binding-in-weft / weft-only-clone / `fabric init` idea? **A:** Good, but out of scope here — filed as separate task `fabric-warp-binding-in-weft`, sequenced after this one. Build `Bolt` forward-compatibly.
- **Q:** (r1 review GAP) `lyx fabric status` verb name already taken — collision? **A:** Confirmed: `status/commit/push/pull/sync` are flat under `lyx fabric` (`addWeftVerbs`, fabric.go:285). Resolution: `Fabric.Status` (unified) REPLACES the weft-only `StatusWeft`-backed `status` verb; `StatusWeft` dropped; `doc.go:80` revised. A weft-only `fabric status` is itself a leak and should not exist.
- **Q:** Are there more "status" surfaces? What is `lyx fabric pairs`? **A:** Three total: `Topology.Status` (`pairs` — operator topology/junction-health diagnostic, legitimately two-repo, stays), `StatusWeft` (`status` — weft-only, dropped), `Fabric.Status` (unified, becomes `status`). `pairs` is the accepted plumbing-diagnostic boundary, like `New(warp, weft)`.
- **Q:** (r1 review NOTE) Perch exclude-magic resolution options? **A:** Added the existing `seedWeftArtifactExcludes`/`crossModuleMachineLocalExcludes` git-exclude backstop as a candidate, noting perch's locks are two-deep (`_lyx/perch/<block>/run.lock`) so the pattern needs deepening (`**/_lyx/*/**/*.lock`).
- **Q:** (r1 review NOTE) Dropping `SyncWeft` breaks kept `Diff`/`Status` test setup? **A:** Migrate `diff_integration_test.go`'s correspondence-recording setup onto `Fabric.Commit` so the kept verbs keep integration coverage.
