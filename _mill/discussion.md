# Discussion: Add a local-only file category to weft

```yaml
task: Add a local-only file category to weft
slug: weft-local-only-files
status: discussing
parent: main
```

## Problem

A loom run's authoritative FSM state lives in `_lyx/loom/status.json` — `shedengine.Status` (`current_producer`, `state`, `history`, `activity`), described by `manifest/designs/loom.md` as "the single source of truth for orchestration state".
Today `lyx loom run` commits that file exactly twice: once at seed (`internal/loomcli/run.go:119`) and once as the landing checkpoint each landing producer takes through `landingshed.Deps.CommitStatus` (`internal/loomcli/landingdeps.go:66`).
Every persist in between leaves it as an uncommitted working-tree modification, so a second machine pulling the branch mid-run sees either a stale seed or a finished run.
`manifest/designs/loom.md:273` states this outright: "resume across machines does not work today".

PR #208 (`loom-status-file-merge-conflict`, closed without merging) tried to fix a different symptom — `landingshed.Finalize` runs a real `fabricengine.Fabric.Merge` against the parent, and a frequently-rewritten tracked file on that path produces merge conflicts on every landing — by moving `status.json` to gitignored `.lyx/`.
That trade was rejected: it makes the file machine-local forever and gives up cross-machine resume permanently, to buy off a merge-boundary problem that has a known, already-shipped solution.

**Why now:** the fix has been built once already, in Millhouse. `mill-merge`'s Step 4/5 solves this exact problem for `_mill/`: delete the local-only path set on the child branch in a pre-merge cleanup commit, then, once the merge has computed its diff, restore that same path set on the parent side from the parent's own pre-merge HEAD via unstage + checkout.
`mill-merge`'s own doc names the hazard it prevents — "a parent that independently tracks `task_dir/_mill/status.md` at the same relative path would otherwise have its file deleted by the squash diff (the #497 bug-2 corruption)".
That is precisely the `_lyx/` scenario.
Nothing needs inventing; the mechanism needs generalizing into `fabricengine`, and Shed needs to start committing per transition so there is genuine live state worth protecting.

## Scope

**In:**

- `internal/shedengine` — a new injected `Shed.CommitStatus func() error` seam, called from `persist`, so the status file is committed on every FSM transition.
- `internal/loomrecipe` — a `ShedPaths.CommitStatus` field threaded into the constructed `shedengine.Shed`.
- `internal/loomcli` — filling that closure with a commit-then-push over loom's own status path, and filling the new local-only path set on `landingshed.Deps`.
- `internal/fabricengine` — `MergeOptions.LocalOnlyPaths`, the parent-side restore in `Fabric.Merge`, the symmetric child-side restore in `Fabric.MergeIn`, forced `--no-ff` when the set is non-empty, and a new `PushAnchored(l, opts)` wrapper beside the existing `CommitAnchoredPaths`.
- `internal/landingshed` — `Deps.LocalOnlyPaths`, a new `Deps.DropLocalOnly func() error` closure, and the pre-merge delete step in `Finalize.Call`.
- `CONSTRAINTS.md` — the Durable-vs-Ephemeral State Invariant gains a third category.
- `manifest/designs/loom.md` and `manifest/designs/shed.md` — the corresponding design-doc updates, same commit, per the Documentation Lifecycle rule.

**Out:**

- **The millhouse repo.** Whether `mill-merge`'s own `_mill`-hardcoded Step 4/5 should generalize into a configurable local-only path list is a separate call in a separate repo, and worktree isolation forbids touching it from here. This task implements the mechanism in `fabricengine` and does not resolve the millhouse question.
- **`internal/landingshed/publish.go`.** `Publish` needs no change at all — see the `publish-needs-nothing` decision.
- **Moving `status.json` to `.lyx/`.** PR #208's approach is explicitly rejected; `main` still has the file tracked at `_lyx/loom/status.json`, unchanged, which is where this task starts from.
- **`fabric.yaml`.** No new config key. Geometry is structural, never config-overridable.
- **Any second member of the local-only path set.** The set has exactly one entry, `_lyx`, and is not expected to grow.

## Decisions

### local-only-set-is-told-not-configured

- Decision: the local-only path set reaches `fabricengine` as `MergeOptions.LocalOnlyPaths []string`, told by the caller — `landingshed.Deps.LocalOnlyPaths`, filled by `internal/loomcli`. It is neither a `fabricengine` constant nor a `fabric.yaml` key.
- Rationale: a structural constant inside `fabricengine` would make the generic fabric layer name a product-specific directory, which the Lyxdirs Single-Declarer rule forbids; a `fabric.yaml` key would make geometry operator-editable, which the Cwd Resolution Invariant forbids outright ("Geometry is structural, never config/env-overridable"). Told-by-caller matches how every other told value in `landingshed.Deps` already works.
- Rejected: `structuralLocalOnlyPaths` constant in `fabricengine`; a `local_only:` key in `fabric.yaml`.

### membership-is-the-whole-lyx-tree

- Decision: the set has exactly one entry, `_lyx`, directory-granular — the whole tree, mirroring `mill-merge`'s `git rm -r _mill/`. Not `_lyx/loom/status.json` alone.
- Rationale: the wiki brief's "starting with `_lyx/loom/status.json`" reading would leave `_lyx/discussion.md`, `_lyx/plan/`, and every review artifact still landing in the parent, and would make each future `_lyx/` file an unflagged leak nobody is prompted to notice. One directory-granular entry never needs extending.
- Rejected: single-file membership; a whole-tree rule with a per-file keep-list carve-out (reintroduces the same membership-list maintenance one level down).

### scaffolding-never-reaches-parent

- Decision: `_lyx/discussion.md`, `_lyx/plan/`, and the review artifacts live only on the task branch and in its archive tag. They never land on the parent branch.
- Rationale: this is exactly the `_mill/` precedent — `mill-merge` deletes it pre-squash and the archive tag is the only durable record. The parent carries the code a task produced, not the scaffolding that produced it.
- Rejected: carving `discussion.md`/`plan/` out of the local-only set so the record lands in the parent.

### fabric-is-one-repo-from-outside

- Decision: `LocalOnlyPaths` entries are **fabric-relative, anchor-relative paths**. `Fabric.Merge` classifies them internally through the existing `classifyPaths` (`internal/fabricengine/classify.go`, the same routing `Fabric.Commit` uses at `commit.go:148`) and applies the restore per side. No caller-facing identifier, field name, or doc comment may say "weft" or "warp".
- Rationale: only `fabricengine` knows warp and weft are two repositories; from outside there is one repo called Fabric. This is the Fabric Vocabulary Invariant, and it binds regardless of whether a warp-side local-only path has a use case today. Encoding "weft-scoped" into the API would leak the split into every caller.
- Rejected: a weft-scoped `LocalOnlyPaths` documented as weft-only; a runtime refusal when a supplied path classifies warp-side. The first leaks vocabulary; the second defends against an input the actual caller cannot structurally produce, while still naming the split in the error text.
- Note for the plan: today's single member `_lyx` does in fact route weft-side (`structuralCommittedDirs`, `internal/fabricengine/junctionnames.go:25`). That is an implementation fact of the routing table, not part of the contract, and must not appear in the exported doc comments.

### child-side-delete-is-index-only

- Decision: the child-side pre-merge cleanup commit uses `git rm -r --cached` (index-only), not `git rm -r`. The working-tree files survive.
- Rationale: `mill-merge` can afford a destructive `git rm -r` because its worktree is torn down immediately after the merge. A loom run continues past `Finalize` — Shed persists again the moment `Finalize` returns — and the operator's own `discussion.md`, `plan/`, and review artifacts must not vanish from disk at landing time. Index-only removal produces the identical merge diff, which is the only thing the mechanism depends on.
- Safety check performed: making those files untracked cannot make the merge itself refuse. `pairDirtyReason` (`internal/fabricengine/mergeguards.go:137-145`) calls `worktreeDirty` with `scopeTracked` on both sides, so untracked files are invisible to the dirty guard.
- Rejected: destructive `git rm -r`, matching `mill-merge` literally.

### delete-sits-between-merge-in-and-parent-merge

- Decision: the child-side delete-commit is a new step in `landingshed.Finalize.Call`, placed **after** `mergeInStep` returns clean and **before** `parentHandle.Merge`. It is taken through a new injected `Deps.DropLocalOnly func() error` closure filled by `internal/loomcli`, mirroring `Deps.CommitStatus` in every respect.
- Rationale: the ordering is load-bearing. `MergeIn`'s own protection (see `mergein-protects-symmetrically`) only means something if the file is still alive during merge-in, so the delete must come after it. Nothing rewrites `status.json` between the delete-commit and the merge, so the tree stays clean for the merge guard.
- Rejected: folding the delete into the `CommitStatus` closure at step 1b (makes the merge-in protection unexercised and untestable); doing the delete inside `fabricengine.Merge` against the source branch (`Merge` holds no handle on the child's worktree and would have to resolve one, which Told-Geometry forbids).

### restore-means-match-parent-head-exactly

- Decision: the parent-side restore is defined as "make each local-only path match the parent's own pre-merge HEAD exactly", **including when HEAD carries no such path** — in which case the entry the merge introduced is removed from the index and working tree, not left in place.
- Rationale: most parents never ran a loom, so absence from parent HEAD is the *common* case, not an edge case. A restore that skips absent paths, or that swallows the `pathspec did not match any file known to git` error, lets the merge's own add of `_lyx/` survive into the parent in exactly that common case — which is the leak this task exists to close.
- Rejected: probe-HEAD-and-skip-when-absent; tolerate-the-checkout-error-and-continue.

### force-no-ff-when-set-is-non-empty

- Decision: when `len(MergeOptions.LocalOnlyPaths) > 0`, `Fabric.Merge` forces a non-fast-forward merge, so the merge always leaves a staged diff for the restore to act on before the single conclude-commit.
- Rationale: `gitrepo.MergeStart` (`internal/gitrepo/merge.go:64-124`) runs `merge --squash` (HEAD never moves, always staged) or `merge --ff --no-commit` (a fast-forward **moves HEAD outright**, returning `MergeFastForwarded` with nothing staged). `landingshed.Finalize` reads `Squash` from `landing.yaml` config, so the FF path is reachable in production, and on it the local-only path silently takes the child's tip content with no window to fix it. Forcing `--no-ff` bakes the fix into the same single landing commit — reset + checkout before the one `git commit`, exactly like `mill-merge` Step 5.
- Rejected: a uniform post-hoc restore commit after `concludeMergeSides` (lands a *second* commit in precisely the cases that needed restoring); detecting `MergeFastForwarded` and restore-then-amend (amends a commit the merge did not author); requiring `Squash: true` and refusing otherwise (punts the problem onto config).

### mergein-protects-symmetrically

- Decision: `Fabric.MergeIn` protects the same path set in the opposite direction, unconditionally — after staging, the child's own pre-merge content for each local-only path is restored, so a parent's `_lyx/` never overwrites the child's live state.
- Rationale: `Finalize` step 2 runs merge-in against the parent branch *mid-run*, and `lyx merge-in` can be run standalone at any point. During the run the child's `_lyx/loom/status.json` is deliberately alive and growing, so a parent that is itself running a loom can genuinely collide with it. Unconditional beats conflict-triggered: two independently-evolving JSON blobs can merge without git flagging a conflict at all, silently blending nonsense.
- Rejected: leaving `MergeIn` alone; auto-resolving-ours only on a flagged conflict.

### commit-hook-lives-in-persist

- Decision: a new `shedengine.Shed.CommitStatus func() error` field, nil-checked (nil = absent, matching `shedadapters.BouncerConfig.Commit`'s convention at `internal/shedadapters/bouncer.go:59-63`), called by `persist` after every successful write.
- Rationale: `persist` (`internal/shedengine/run.go:344`) is already the loop's single write path, so one call site covers running, paused, blocked, failed, stuck-bounce, and the resume write — every write that changes resume-relevant state. It must be an injected closure and not a direct call: `internal/shedengine` is import-capped to stdlib plus `internal/state` and `internal/lock` by `seam_enforcement_test.go`, so it can never import `fabricengine`.
- Rejected: firing only when `current_producer` changes (`state`/`history`/`error` change without it, so a second machine still sees stale state); a commit decorator wrapping each producer in `loomrecipe` (misses the pause and resume writes, which happen outside any producer call).

### commit-and-push-every-transition

- Decision: the loom-side closure commits **and pushes** on every transition, respecting `SyncOptions.SkipPush`. The push goes through a new synchronous `fabricengine.PushAnchored(l, opts)`, a vocabulary-neutral `l`-in / no-path-out wrapper mirroring `CommitAnchoredPaths`' own shape.
- Rationale: an unpushed commit does not deliver cross-machine resume, which is the entire premise of the task — a second machine has to be able to *pull* the branch mid-run. Transitions are already minutes apart (each corresponds to a real LLM session), so the marginal cost of one narrowly-scoped push is noise. The wrapper must be synchronous because push failure is warn-and-continue (below), and a detached fire-and-forget push cannot report the failure that warning is supposed to carry.
- Rejected: commit-only with a separate explicit push command (leaves cross-machine resume aspirational — the exact gap PR #208 was reopened over); `SpawnDetachedPush` (failure invisible); debounced/interval push (reopens the staleness window loom's crash-recovery story is supposed to close); opening a `*Fabric` in the closure to call `PushWeft` (opens a pair on every FSM transition, stat-checking a layout the run may be mid-mutation on, and names weft in the call).

### commit-hard-errors-push-warns

- Decision: a commit failure returns an error from `persist`, halting the run loudly. A push failure logs a warning and continues.
- Rationale: a git fault on the run's own bookkeeping is real infrastructure breakage, and it is the same disposition `landingshed` already gives a failing `CommitStatus` (`internal/landingshed/finalize.go`, step 1b). A push failure is different in kind: an offline laptop must not kill an autonomous run, and the next transition's push catches the branch up, so the condition is self-healing.
- Rejected: both hard-error (makes every loom run require network for no real gain); both warn-and-continue (a silently uncommitted status file is PR #208's exact failure mode returning by a different door).

### landing-checkpoint-stays

- Decision: `landingshed.Deps.CommitStatus` and its call at `Finalize.Call` step 1b are kept unchanged.
- Rationale: with the per-transition hook wired it is a no-op on the ordinary path — `StageAndCommit` reports `committed == false` on a clean tracked path — and it is the only thing that saves the landing rows if a product ever wires `Shed.CommitStatus` as nil. Removing it would couple the generic landing producers to a Shed persistence policy they cannot see.
- Rejected: removing the field as subsumed; keeping it marked deprecated (a documentation hedge, not a decision).

### publish-needs-nothing

- Decision: `internal/landingshed/publish.go` is not touched. No pre-push delete, no local-only handling of any kind.
- Rationale: `Publish` opens a GitHub pull request against the **warp** side — `Deps.PushBranch` resolves to `Fabric.PushBranch`, which is `PushWarpRebaseFreeAt(f.warpPath)` (`internal/fabricengine/fabric.go:163-169`), and `Deps.OriginURL` comes from `Fabric.OriginURL`, the warp remote. `_lyx/` is weft content and lives in a different repository, so it cannot appear in that pull request's diff at all. Fabric is not one real repo; the PR only ever sees one side of the pair, which is what `finalize.go`'s own header already says.
- Rejected: a `Publish`-side delete-commit. Beyond being unnecessary, it would not hold: with the per-transition commit hook, the branch tip re-acquires `_lyx/loom/status.json` on the very next persist after `Publish` returns `Done`.

### constraints-third-category-in-place

- Decision: `CONSTRAINTS.md`'s existing **Durable-vs-Ephemeral State Invariant** (line 91) is extended in place with a third category, *tracked-but-merge-local*: committed for real history and resume on the task's own branch, but never propagated to the parent by a landing — deleted from the index pre-merge on the child side, restored from the parent's own pre-merge HEAD on the parent side.
- Rationale: the invariant's whole subject is where loom state lives and how durable it is; a third category belongs beside the other two. Splitting it into a cross-referencing second section fragments one answer across two places.
- Rejected: a separate new invariant section; both (stub plus extension).
- Enforcement line: the new `internal/fabricengine` merge integration tests, plus a review obligation for adding any path to a product's local-only set.

### millhouse-generalization-not-resolved-here

- Decision: this task implements the mechanism in `fabricengine` and does not touch millhouse. Whether `mill-merge`'s own Step 4/5 should learn a general local-only path list, rather than staying hardcoded to `_mill/` via `TASK_DIR_REL`, needs its own task in that repo.
- Rationale: worktree isolation — an agent operates only within the worktree it was spawned in, and millhouse is a different repository entirely. The wiki brief already states this task only flags the duplication rather than resolving it.
- Follow-up to record in the design doc: the two repos now carry the identical mechanism, built twice.

## Technical context

**Where the status file is written.**
`internal/shedengine/run.go:344` — `Shed.persist` is the loop's single write path, one `state.UpdateJSON` call whose mutate overwrites the Shed-owned fields.
Every state change in `run.go` (lines 113, 134, 177, 191, 201, 217, 227, 242, 254, 267) funnels through it.
`internal/shedengine/shed.go` holds the plain exported-field `Shed` struct — no `New` constructor by design, `Run` validates every field.

**Why the hook must be a closure.**
`internal/shedengine/seam_enforcement_test.go` (`TestProducerSeamInvariant_AllowlistOnly`) is an allowlist over direct imports: production code in that package may import only stdlib, `internal/state`, and `internal/lock`.
`internal/loomrecipe/seam_enforcement_test.go` is the same shape, allowing `contracts/recipes`, `internal/shedbuild`, `internal/shedrecipe`, `internal/shedengine`.
A `func() error` field crosses both without adding an import.

**Threading the closure down.**
`internal/loomrecipe/loomrecipe.go:25-39` defines `ShedPaths` (the told values `Shed` itself reads); `New` at line 78 copies them onto the constructed `Shed` at line 96.
`internal/loomcli/wiring.go:48` and `:248` are the two fill sites.
Note `New`'s existing StatusPath/StatusLockPath divergence guards — a new closure field needs no equivalent, since there is no second copy to disagree with.

**The existing commit call to mirror.**
`internal/loomcli/landingdeps.go:66` —
`fabricengine.CommitAnchoredPaths(fabricengine.NewMutations(""), l, []string{loomengine.LoomStatusRel()}, msg, fabricengine.EnvSyncOptions())`,
discarding `(sha, committed)` in favour of the error alone, which is what makes a second call over an already-clean path a no-op rather than a failure.
`internal/loomcli/run.go:119` does the same for the seed with `[]string{loomengine.LoomStatusRel(), fabricengine.OriginRecordRel()}`.
`CommitAnchoredPaths` (`internal/fabricengine/commitweftpaths.go:95`) is the vocabulary-neutral wrapper: `l`-in, no path out, taking anchor-relative paths.
`PushAnchored` should mirror its doc-comment shape exactly.

**Push entry points that exist today.**
`Fabric.PushWeft(opts)` (`internal/fabricengine/weftgit.go:299`) needs an open pair.
`SpawnDetachedPush(warpPath, weftPath)` (`spawn.go:40`) is fire-and-forget.
`PushWarpRebaseFreeAt` / `PushWarpAt` (`spawn.go:92`, `:124`) are the warp-side `At`-shaped equivalents `PushAnchored` should be modelled on.

**The merge itself.**
`Fabric.Merge` — `internal/fabricengine/merge.go:344`.
Order inside it: foreign-state refusal → aggregated guard stage (`mergeInProgressReason`, `pairDirtyReason`, `detachedHeadReason`, `syncedToUpstreamReason`, `resolveMergeSources`) → weft write lock → `recheckMergePreconditionsUnderLock` → `syncSideBeforeMerge` per side → post-sync already-up-to-date probe → `saveMergeState` → `warp.MergeStart` → `weft.MergeStart` → conflict self-abort → `concludeMergeSides` → `RecordCorrespondence` → `deleteMergeState`.
**The restore belongs between the two `MergeStart` calls and `concludeMergeSides`** (`merge.go:490-532`) — that is the window where the diff is computed but nothing is committed.
`concludeMergeSides` lives at `internal/fabricengine/mergelifecycle.go:41`; note its per-side skip arms for `mergeOutcomeFastForwarded` / `mergeOutcomeAlreadyUpToDate`, which is the second reason forcing `--no-ff` matters.
`Fabric.MergeIn` is at `merge.go:116` with the same conclude phase.

**Path routing.**
`classifyPaths` (`internal/fabricengine/classify.go`) splits a caller-supplied path list into warp-side, weft-side, and never-committed pathspecs; `Fabric.Commit` calls it at `commit.go:148` with `pathspecNames(cfg)`.
`structuralCommittedDirs = []string{lyxdirs.LyxDirName}` and `structuralNeverCommittedDirs` are at `internal/fabricengine/junctionnames.go:19-31` — structural, never read from `fabric.yaml`'s `pathspec`.
`ScopedPathspec(anchorRel, dirs)` (`fabric.go:171`) is the single place an anchor join happens.

**The landing producer.**
`internal/landingshed/finalize.go` — `Finalize.Call` steps are commented inline (1b commit status, 2 merge-in, 3 open parent pair, 4 parent-side merge, 5 merge-in-required retry, 6-7 guard-error handling).
`internal/landingshed/deps.go` documents every told field; `Deps.CommitStatus`'s own doc comment explains why a path is not passed directly (naming the status file here would make the package declare a location it may not declare).
`Deps.LocalOnlyPaths` and `Deps.DropLocalOnly` follow the same reasoning — the closure owns the pathspec and the commit message.

**Gotchas found during exploration.**

- `gitrepo.MergeStart`'s non-squash arm is `merge --ff --no-commit`; `--no-commit` does **not** suppress a fast-forward. The switch at `internal/gitrepo/merge.go:119-124` classifies `headAfter != headBefore` as `MergeFastForwarded`.
- `pairDirtyReason` uses `scopeTracked`, so untracked files never block a merge. This is what makes the index-only delete safe.
- `loomyard` itself does not track any `_lyx/` content (`git ls-files | grep _lyx` matches only a test file name). Every test of this mechanism needs a real hub fixture from `internal/hubforge` — the hubforge Fabric-Fixture Invariant requires every hub fixture in the repo to be built through `fabriccli.CloneAndWire`.
- The loom recipe order is `... → Webster-Bouncer → Publish → Finalize` (`contracts/recipes/loom-recipe.yaml:302-307`). `Finalize` is the authoritative landing regardless of whether the PR is merged remotely.
- After `Finalize`'s delete-commit, Shed persists once more when `Finalize` returns `Done`, so `_lyx/loom/status.json` is re-committed onto the task branch. That is correct and harmless: the parent has already merged by then, and the file is where a resume expects it.

## Constraints

From `CONSTRAINTS.md`:

- **Durable-vs-Ephemeral State Invariant** (line 91) — the invariant being extended. `_lyx` holds tracked content only; `.lyx` holds never-tracked content at the mirrored subpath. Neither is read from `fabric.yaml`'s `pathspec`.
- **Cwd Resolution Invariant** — "Geometry is structural, never config/env-overridable." This is what rules out a `fabric.yaml` key for the local-only set.
- **Told-Geometry Invariant** — an engine is handed the absolute paths it operates on and derives none of its own. `landingshed` derives nothing; every new field is told. `shedengine` is bound by this invariant with machine enforcement.
- **Fabric Vocabulary Invariant** — weft-sibling paths and warp/weft vocabulary are `fabricengine`-private. No new caller-facing identifier may contain `Weft` or `Warp`; `PushAnchored`, not `PushAnchoredWeft`.
- **Lyxdirs Single-Declarer Invariant** — no hand-built join naming the `_lyx` literal outside its declarer. The local-only set's single entry must come from `lyxdirs.LyxDirName`, not a string literal.
- **Shed Producer-Seam Invariant** — `internal/shedengine` production imports are allowlisted to stdlib, `internal/state`, `internal/lock`.
- **hubforge Fabric-Fixture Invariant** — every hub fixture is built by `internal/hubforge` through `fabriccli.CloneAndWire`.
- **Documentation Lifecycle** — `manifest/designs/loom.md`, `manifest/designs/shed.md`, and `CONSTRAINTS.md` update in the same commit as the code. `manifest/roadmap.md` does **not** move: this is a reopened bug/hardening item, not a completed or newly added planned item.
- **Markdown Link Integrity** — `manifest/designs/loom.md`'s `#crash-recovery--resume-on-output-files-not-live-processes` heading is linked from `manifest/roadmap.md` and from within `loom.md` itself; the heading text must stay exactly as written even though the text beneath it changes.
- **Markdown semantic line breaks** — one sentence per line, break at internal independent-clause boundaries, plain newlines only.

Discovered during discussion:

- `manifest/designs/loom.md:273-276` currently asserts that cross-machine resume does not work and that making it work "is a `Shed` persistence-policy decision with a real per-transition git cost, not a property this doc can assert into existence". This task makes that decision, so those three lines are rewritten rather than deleted.
- `manifest/designs/loom.md:277-282`'s landing-checkpoint paragraph stays, but must now explain that the checkpoint is a no-op safety net rather than the only commit.

## Testing

**`internal/fabricengine` — integration tests over real `hubforge` fixtures.**
These are the load-bearing tests; write them alongside the implementation rather than after.
Four merge-diff shapes must each be covered:

1. The local-only path is **absent from the parent's pre-merge HEAD** (the common case — parent never ran a loom). The merge's own add must not survive; parent ends with no such path.
2. The parent **has its own diverged copy**. Parent's copy is byte-identical before and after; the child's content never appears.
3. The **merge base shared the path and the child deleted it** — the #497 bug-2 shape. Parent's copy must survive the squash diff's delete.
4. **FF-able parent with `Squash: false`** — asserts the forced `--no-ff` actually leaves a staged diff and the restore runs. This case does not get to be untested: the FF hole is exactly the "self-evidently correct" assumption that was already wrong once.

Plus: `MergeIn` in the opposite direction, with the parent carrying its own `_lyx/` — the child's live content must survive unchanged, including the shape where git would merge two JSON blobs cleanly without flagging a conflict.
Plus: an empty `LocalOnlyPaths` must leave `Merge`/`MergeIn` behaviour bit-for-bit unchanged, including no forced `--no-ff`.

**`internal/shedengine` — TDD candidate.**
Unit-test that `persist` calls `CommitStatus` on **every** write path: the resume write, running, paused (with `consumePause`), blocked, failed, stuck-bounce, and done.
Test that a nil `CommitStatus` is a silent no-op.
Test that a returned error from the closure propagates out of `persist` and halts `Run`, and that the status file write itself still happened first.

**`internal/landingshed` — TDD candidate.**
Test the step ordering in `Finalize.Call` with fakes: `CommitStatus` → merge-in → `DropLocalOnly` → parent-side merge.
Assert `DropLocalOnly` is not called when merge-in returns stuck or errors, and that it is called exactly once on the merge-in-required retry path (which re-runs merge-in but must not re-delete).
Assert a `DropLocalOnly` failure maps to a returned error, matching `CommitStatus`'s disposition.
Add the new fields to the existing every-field-populated drift guard in `internal/loomcli/landingdeps_test.go`.

**`internal/loomcli` — wiring.**
Assert both `wiring.go` fill sites populate `ShedPaths.CommitStatus`.
Assert the closure commits then pushes, and that a push failure does not surface as an error while a commit failure does.

**Not attempted:** an end-to-end `lyx loom run` test of any of this. Those runs spawn real LLM sessions, take tens of minutes, and would not exercise the diverged-parent or FF-parent merge shapes at all — the wrong tool for pinning merge-boundary correctness.

**Task-wide verify:** `go build ./cmd/lyx && go test ./...` (per `README.md:123-124` — the full suite includes the structural invariant tests).

## Q&A log

- **Q:** Who declares the local-only path set, and how does it reach `fabricengine`? **A:** Told by the caller via `MergeOptions.LocalOnlyPaths`, filled by `loomcli`. Not a `fabricengine` constant (Lyxdirs single-declarer), not a `fabric.yaml` key (config-overridable geometry is banned).
- **Q:** What about a fast-forwarding parent, where `merge --ff --no-commit` moves HEAD with nothing staged? **A:** Force `--no-ff` when the set is non-empty. A post-hoc restore commit would land as a *second* commit in exactly the cases that needed restoring; forcing `--no-ff` bakes it into the same single landing commit, as `mill-merge` Step 5 does.
- **Q:** Should `MergeIn` (parent → child) be protected symmetrically? **A:** Yes, unconditionally, not conflict-triggered. During a run the file is deliberately alive and growing, so a parent running its own loom can genuinely collide with it — and two evolving JSON blobs can merge without git flagging any conflict, silently blending nonsense.
- **Q:** Where does the per-transition commit hook live? **A:** `Shed.CommitStatus`, called from `persist`. Hooking producer transitions instead would miss `state`/`history`/`error` changes and the pause/resume writes.
- **Q:** Does the per-transition commit also push? **A:** Yes, respecting `SkipPush`. An unpushed commit does not deliver cross-machine resume, which is the whole premise. Transitions are already minutes apart, so push cost is noise.
- **Q:** Where in `Finalize.Call` does the delete-commit go? **A:** Between merge-in and the parent-side merge. Placing it earlier would make the `MergeIn` protection untestable and defeat its own purpose.
- **Q:** Does `Publish` need the same delete before it pushes? **A:** No — and the initial "yes" was wrong. `Publish` opens the PR against the **warp** side; `_lyx/` is weft content in a different repository and cannot appear in that diff. Fabric is not one real repo.
- **Q:** Does `landingshed.Deps.CommitStatus` survive? **A:** Yes. It is a no-op on the ordinary path and the only protection if a product wires `Shed.CommitStatus` as nil.
- **Q:** What happens when the commit or the push fails? **A:** Commit failure hard-errors from `persist`; push failure warns and continues, self-healing on the next transition. Both-hard would make every run require network; both-soft reopens PR #208's silent-uncommitted-state failure mode.
- **Q:** How many members does the local-only set have on day one? **A:** One, `_lyx`. The API is `[]string` because a plural signature costs nothing now and avoids a breaking change later, but nothing speculative goes in it.
- **Q:** Is `LocalOnlyPaths` weft-scoped? **A:** No. Only `fabricengine` knows warp and weft exist — from outside there is one repo called Fabric. Paths are fabric-relative and classified internally through `classifyPaths`. No caller-facing name or doc may encode the split, whether or not a warp-side use case exists today. This also renamed the push wrapper from `PushAnchoredWeft` to `PushAnchored`.
- **Q:** Is it just `status.json`, or the whole `_lyx` folder? **A:** The whole folder — temporary and local to each branch, deleted before the merge to parent, exactly as `mill-merge` does `git rm -r _mill/`.
- **Q:** So `discussion.md`, `plan/`, and the review artifacts never reach the parent? **A:** Correct and intended. The parent carries the code the task produced, not the scaffolding that produced it; the task branch and its archive tag are the durable record.
- **Q:** Destructive `git rm -r` or index-only `git rm -r --cached`? **A:** Index-only. `mill-merge`'s worktree is torn down right after; a loom run continues past `Finalize`, and the operator's own artifacts must not vanish from disk. Verified safe: `pairDirtyReason` uses `scopeTracked`, so the now-untracked files cannot make a later merge refuse.
- **Q:** Does millhouse's `mill-merge` get generalized in this task? **A:** No. Different repository, and worktree isolation forbids touching it. It needs its own task there; this task only records that the mechanism now exists twice.
