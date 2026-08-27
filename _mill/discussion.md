# Discussion: Add a local-only file category to weft

```yaml
task: Add a local-only file category to weft
slug: weft-local-only-files
status: discussing
parent: main
```

## Problem

A loom run's authoritative FSM state lives in `_lyx/loom/status.json` — `shedengine.Status` (`current_producer`, `state`, `history`, `activity`).
Today `lyx loom run` commits it exactly twice: at seed (`internal/loomcli/run.go:119`) and as the landing checkpoint each landing producer takes through `landingshed.Deps.CommitStatus` (`internal/loomcli/landingdeps.go:66`).
Every persist in between leaves it an uncommitted working-tree modification, so a second machine pulling the branch mid-run sees a stale seed or a finished run. `manifest/designs/loom.md` states it outright: resume across machines does not work today.

PR #208 (`loom-status-file-merge-conflict`, closed without merging) attacked a different symptom.
`landingshed.Finalize` runs a real `fabricengine.Fabric.Merge` against the parent, which merges **both** sides of the pair, and a frequently-rewritten tracked file on the weft side produces merge conflicts at every landing.
PR #208 proposed moving `status.json` to gitignored `.lyx/`, which buys off the conflict by making the file machine-local forever.
That trade was rejected and the PR closed without merging, so `main` still tracks the file at `_lyx/loom/status.json`, unchanged.

**Why now:** the conflict has a root cause nobody had named. The weft repo holds system files that belong to exactly one worktree and one branch — loom's FSM state, its discussion and plan artifacts, webster's state, the parent-branch provenance record.
None of it describes the code being landed, and none of it has any meaning on another branch.
Merging it was never useful; it only ever produced conflicts, and a substantial amount of machinery exists purely to survive those conflicts.
Removing weft from merging entirely dissolves the problem PR #208 tried to work around, and makes per-transition commits unconditionally safe.

## Scope

**In:**

- `internal/fabricengine` — `Fabric.Merge` and `Fabric.MergeIn` stop merging the weft side. No exported signature changes.
- `internal/fabricengine` — remove the raddle fold-back gate from `Topology.Cleanup` (`raddleFoldedBack` and its `Protected` branch), so an orphan weft branch is deletable without `--force`.
- `internal/shedengine` — a new injected `Shed.CommitStatus func() error` seam, called from `persist`.
- `internal/loomrecipe` — a `ShedPaths.CommitStatus` field threaded into the constructed `shedengine.Shed`.
- `internal/loomcli` — filling that closure with commit-then-push over loom's status path.
- `internal/fabricengine` — a new `PushAnchored(l, opts)` beside the existing `CommitAnchoredPaths`.
- `internal/fabricengine` — a new `MergeStateActive(l) (bool, error)`, the vocabulary-neutral merge-state probe the commit closure consults.
- `internal/fabricengine` — the weft-side merge guards and `syncSideBeforeMerge`'s weft call (`weft-guards-drop-with-it`), and the weft arm of `resetMergeSides` (`abort-does-not-reset-weft`).
- `internal/fabricengine` — `Fabric.Pull`'s weft arm becomes non-fatal, which reshapes `PartialPullError` and the weft-first-ordering Shared Decision behind it (`pull-does-not-stall-on-weft`).
- Docs, same commit: `CONSTRAINTS.md` (ordered *after* a merge-in of `main`; see `constraints-gains-one-sentence`), `manifest/designs/loom.md`, `manifest/designs/shed.md`, `internal/fabricengine/doc.go`, `internal/fabricengine/cleanup.go`'s package-level flag matrix and the `Protected` field comment, and the project `CLAUDE.md`'s `_lyx/raddle/` clause.

**Out:**

- **A per-path local-only mechanism of any kind.** No `MergeOptions.LocalOnlyPaths`, no delete-then-restore, no forced `--no-ff`, no path-scoped reset or checkout. The split is already structural; a per-path rule would re-encode it in config.
- **`internal/gitrepo`.** No new primitives are needed — skipping a side means not calling `MergeStart` on it.
- **`internal/landingshed`.** No new deps, no new step, no `Publish` change. Both producers are untouched.
- **Teardown as part of a loom run.** Deleting the weft branch cannot happen from inside the worktree it belongs to; it stays an outside verb.
- **Raddle.** `_lyx/raddle/` does not exist, and building it would not reintroduce a fold-back concern: raddle regenerates against the parent's HEAD and commits onto the parent pair directly, never by merging the child's copy. The placeholder gate is removed as structurally unnecessary, not deferred. See `raddle-gate-removed` for the precision this rests on.
- **The millhouse repo.** `mill-merge`'s own `_mill`-hardcoded delete-then-restore is not generalized here; different repository, and this design needs no equivalent.
- **Moving `status.json` to `.lyx/`.** PR #208's approach stays rejected.

## Decisions

### weft-is-never-merged

- Decision: `Fabric.Merge` and `Fabric.MergeIn` merge the warp side only. The weft side is not a merge participant in either direction.
- Rationale: everything routed to the weft belongs to one worktree and one branch. It never describes another branch's state, so there is nothing a merge could usefully carry. Merging it produced conflicts on loom's own bookkeeping — the exact failure PR #208 was opened over.
- Rejected: a per-path local-only set with delete-then-restore at the merge boundary (the wiki brief's design — solves one file's symptom while leaving every other weft file merging, and needs a delete that loom cannot safely perform on itself mid-run); leaving weft merging and accepting conflicts.

### weft-guards-drop-with-it

- Decision: the weft loses its power to block a merge, not only its participation in one. Four guards drop their weft arm to warp-only — `pairDirtyReason` (`mergeguards.go:137-152`), `syncedToUpstreamReason` via `sideNotSyncedToUpstream(f.weft, …)` (`:229-234`), `detachedHeadReason` (`:182-196`), and `resolveMergeSources`' weft arm (`:84-97`) — and `syncSideBeforeMerge(rec, f.weft, f.weftPath, "weft")` (`merge.go:447`) is not called at all.
- Carve-out inside `resolveMergeSources`: the weft SHA **read** stays, because `mergestate-weft-fields-stay` needs a value for `WeftStart`/`WeftSource`. What is dropped is that arm's power to append `mergeReasonNotFabricManaged` or `mergeReasonSourceNotFound`; an unresolvable weft counterpart now leaves those fields best-effort or empty rather than refusing the merge.
- Rationale: a non-participant's state cannot affect a warp-only merge's correctness, so checking it can only refuse a merge that would have been right. The sync guard is the load-bearing case: `commit-and-push-every-transition` warns and continues on a rejected push, which makes a locally-diverged weft a **routine, expected** state rather than an edge case — and a retained `syncedToUpstreamReason` weft arm would then refuse every subsequent landing with `mergeReasonNotSynced`. That is PR #208's blocked-landing failure relocated into a new refusal, not removed. All four guards currently OR the weft arm into the same aggregated reason as warp, unconditionally.
- Rejected: narrowing only the sync guard and leaving dirty/detached/unresolvable weft blocking a merge it cannot affect; hard-erroring on push failure so the weft can never diverge — that contradicts `commit-hard-errors-push-warns`, whose whole point is that an offline laptop must not kill an autonomous run.
- Test pins that change: `TestMerge_FetchedDivergedWeftRefuses` and `TestMerge_UnfetchedDivergedWeftRefuses` (`merge_target_integration_test.go:793`, `:817`) assert today's refusal and are rewritten to assert the merge now proceeds.

### pull-does-not-stall-on-weft

- Decision: `Fabric.Pull`'s weft arm becomes non-fatal. A failed weft `git pull --ff-only` warns, `PullResult` reports the weft unpulled, and the warp pull proceeds.
- Rationale: `Fabric.Pull` pulls the weft first and returns immediately on its error (`pull.go:243-245`), so warp fetch/reconcile never runs. `Repo.Pull` is a plain `--ff-only` (`internal/gitrepo/pull.go:18-27`), which hard-refuses a diverged local weft — and `commit-and-push-every-transition`'s warn-and-continue makes a diverged weft *routine*. Without this, one rejected push blocks the operator's own resume verb for the whole pair, defeating the multi-machine case the task exists to enable. Code sync is unrelated to the weft's bookkeeping state.
- Manual recovery, named rather than automated: the operator reconciles with `git -C <weft> reset --hard origin/<branch>`, losing that machine's local status history. A push rejection means another machine advanced the same branch's FSM state; `commit-and-push-every-transition` already decided that is a human call, never something fabric resolves by rewriting history.
- Scope warning: this is a larger diff than one flag. `PartialPullError`'s doc comment asserts "`WeftPulled` is always true for this type: a weft-side failure never produces a `*PartialPullError` at all" (`pull.go:86-87`), its `Error()` hardcodes "weft pull succeeded, warp %s failed" (`:99-101`), and both rest on a weft-first-ordering / report-not-rollback Shared Decision. The struct, its doc comment, its `Error()` text, and that decision all need rework together.
- Rejected: documenting the manual recovery without changing `Pull`, which leaves the *warp* pull hard-blocked by a weft-only problem; force-resetting the weft inside `Pull`, which contradicts this document's own stance that a push rejection is a human decision.

### abort-does-not-reset-weft

- Decision: `resetMergeSides` drops its weft arm (`destroy.go:1211-1218`). Only the warp checkout is reset on abort.
- Rationale: all four call sites — `MergeAbort` (`mergelifecycle.go:397`) and the three self-abort sites (`merge.go:288`, `:520`, `:657`) — pass `st.WeftStart` with `force: true` into an unconditional hard reset. The weft was never a merge participant, so an abort has nothing to restore there; with the weft advancing per transition, a reset instead discards already-pushed status history and leaves the local weft *behind* its own origin, so the next push fails too — compounding straight into `pull-does-not-stall-on-weft`'s failure.
- `WeftStart` stays recorded, per `mergestate-weft-fields-stay`. It simply stops being a reset target.
- Rejected: keeping the arm but re-reading the weft SHA at reset time, which is a no-op dressed as a restore; keeping it and accepting the loss on the grounds that aborts are rare and FSM state is rebuildable — this is not only data loss but an actively broken push state afterwards.

### no-api-change

- Decision: neither verb gains a flag, an option parameter, or a warp-only variant. The behaviour changes inside `fabricengine`; every caller and every signature stays as written.
- Rationale: only `fabricengine` knows warp and weft exist — from outside there is one repo called Fabric. A `MergeOptions.LocalOnlyPaths` or a `MergeIn(source, opts)` widening would push the two-repo split into `mergeresolve.MergeSurface`, `landingshed`, and the CLI. What callers observe is simply that merging carries code, not system files.
- Rejected: widening `MergeIn` to take `MergeOptions` and rippling that through `MergeSurface` and its call sites; a second `MergeInWithOptions` door; storing a path set on `Fabric` at open time.

### merge-plumbing-stays

- Decision: the pair's conflict plumbing is not removed. `unifyConflictPaths`, `mergeresolve`'s conflict-resolution session, and `MergeStageResolved` all stay.
- Rationale: `unifyConflictPaths(warpConflicts, weftConflicts, ...)` still serves the warp list; the weft list simply becomes permanently empty. Removing the weft half would be a large diff for a function that keeps working. `fabriccli/merge_verbs.go`'s junction-staging path becomes unreachable rather than wrong, and is left alone.
- Rejected: deleting the now-unreachable weft-conflict paths in the same task.

### correspondence-unchanged

- Decision: `RecordCorrespondence(newWarpHEAD, weftHEAD)` stays at all three merge call sites (`merge.go:314`, `merge.go:544`, `mergelifecycle.go:331`), passing the current, unmoved weft SHA.
- Rationale: the index maps a warp SHA to the weft SHA current at that point (`index.go:102-118`), and `Merge` already records it even when one side never moved. A warp commit landing against an unchanged weft is a correct pair, not a missing one.
- Rejected: skipping the record when weft did not move; narrowing the correspondence concept.

### teardown-is-external

- Decision: the weft branch is deleted by an outside verb — `Topology.Cleanup`, or `removeWeftWorktree(..., alsoDeleteBranch, ...)` via `Topology.Remove` — never by a loom producer and never from inside the worktree being torn down. `Finalize` merges the warp side and stops, exactly as it does today.
- Rationale: teardown cannot run from within the worktree it removes. Nothing in this task adds a producer row or a step to either landing producer.
- Rejected: a teardown producer after `Finalize`; folding branch deletion into `Finalize.Call`.

### raddle-gate-removed

- Decision: `raddleFoldedBack` (`internal/fabricengine/cleanup.go:93-95`) and the `Protected` branch it feeds are removed, along with the fold-back row of `Cleanup`'s documented flag matrix.
- Rationale: it is a stub returning `false`, so today every fabric-managed orphan weft branch is protected unless `--force` — which makes routine teardown require the destructive flag.
- Why raddle does not bring it back: raddle's mechanism was never "merge the weft, carrying the child's `_lyx/raddle/` forward". `manifest/designs/raddle.md` has it regenerate **fresh** at merge time against the parent's actual current HEAD, committed directly onto the parent pair inside `Finalize`'s own critical section via `SyncWeft` — the "regenerate-don't-merge" property `manifest/designs/fabric-unified-view.md:228` names. It never depended on `Merge`/`MergeIn` touching the weft, so this task supersedes nothing in raddle's design and there is no fold-back for a gate to guard.
- Precision the plan must preserve: raddle's *output* is genuinely meant to land in the parent's weft. What never travels is the child's copy of it, by git-merge. Do not restate this as "`_lyx/raddle/` is per-branch-local and never merged" — that flattens a regenerate-and-commit step into a merge and is what misled an earlier reviewer.
- Doc correction in the same commit: the project `CLAUDE.md` directs durable notes to `_lyx/raddle/` as "anything versioned and merged into `main`". That clause is imprecise under this design and is corrected — a one-clause precision fix, not a redesign.
- Rejected: keeping the stub gate and routing teardown through it; keeping it warm as a placeholder for raddle; declaring raddle's merge-time fold-back superseded, which misreads a design that never asked for one; leaving `CLAUDE.md` alone and deferring the trip to the next reader.

### commit-hook-lives-in-persist

- Decision: a new `shedengine.Shed.CommitStatus func() error` field, nil-checked (nil = absent, matching `shedadapters.BouncerConfig.Commit`, `internal/shedadapters/bouncer.go:59-63`), called by `persist` after every successful write.
- Rationale: `persist` (`internal/shedengine/run.go:344`) is already the loop's single write path, so one call site covers running, paused, blocked, failed, stuck-bounce, and the resume write. It must be an injected closure: `internal/shedengine` is import-capped to stdlib plus `internal/state` and `internal/lock` by `seam_enforcement_test.go`, so it can never import `fabricengine`.
- Rejected: firing only when `current_producer` changes (`state`/`history`/`error` change without it, so a second machine still sees stale state); a commit decorator around each producer in `loomrecipe` (misses the pause and resume writes, which happen outside any producer call).

### commit-and-push-every-transition

- Decision: the loom-side closure commits and pushes on every transition, respecting `SyncOptions.SkipPush`. The push goes through a new synchronous `fabricengine.PushAnchored(l, opts)`, a vocabulary-neutral `l`-in / no-path-out wrapper mirroring `CommitAnchoredPaths` (`internal/fabricengine/commitweftpaths.go:95`).
  Its underlying primitive is **`gitrepo.PushRebaseFree`, never `gitrepo.PushCoalesced`** — the same choice `PushWarpRebaseFreeAt` (`spawn.go:110-123`) already made, for the same two reasons.
  A rejected push surfaces as `gitrepo.ErrPushRejected` and is treated as an ordinary push failure: warn and continue, never retry, never rebase.
- Rationale: an unpushed commit does not deliver cross-machine resume, and pulling the branch is the whole point. Transitions are minutes apart — each is a real LLM session — so one narrowly-scoped push is noise. Synchronous because push failure is warn-and-continue, and a detached push cannot report the failure the warning carries.
  `PushCoalesced` is disqualified twice over: its `pushWithRebaseRetry` path runs `git pull --rebase` on a rejected push, which rewrites this side's SHAs and invalidates the correspondence index — contradicting `### correspondence-unchanged` and turning a rejection into a silent history rewrite of a *running* weft;
  and it takes a repo-root push-lock file, which would contend with `SpawnDetachedPush` children and landing-time pushes on every transition, and returns on a failed acquisition before `HasUnpushed` is ever consulted. `PushRebaseFree` does neither, so the transition push has no lock, no residue, and no contention with fabric's existing push paths.
  A rejection means another machine advanced the branch — exactly the multi-machine case this feature enables — and resolving that is a human decision, not something a background persist may rewrite history over.
- Rejected: `PushCoalesced` (above); commit-only with a separate push verb; `SpawnDetachedPush` (failure invisible); debounced push; opening a `*Fabric` per transition to call `PushWeft`.

### commit-hard-errors-push-warns

- Decision: a commit failure returns an error from `persist`, halting the run. A push failure logs a warning and continues.
- Rationale: a git fault on the run's own bookkeeping is infrastructure breakage, and it is the disposition `landingshed` already gives a failing `CommitStatus`. An offline laptop must not kill an autonomous run, and the next transition's push catches the branch up.
- Rejected: both hard-error; both warn-and-continue.

### skip-while-mid-merge

- Decision: `CommitStatus` consults a new `fabricengine.MergeStateActive(l) (bool, error)` and skips when it reports true — no commit, no push, no error — logging at warn.
  `MergeStateActive` answers "is the **weft** mid-merge at the git level", consulting `MergeHeadPresent()` and `ConflictedFiles()` on the weft alone — not the two-sided form the unexported `foreignMergeStatePresent` (`internal/fabricengine/mergestate.go:257-276`) uses. It takes an `l *lyxcwd.Location`, not an open `*Fabric`, matching `CommitAnchoredPaths`/`PushAnchored`'s shape.
- Weft-only, deliberately: warp and weft are independent clones with independent `.git` (`clone.go:274` does a separate `cloneRepo(opts.WeftURL, weftPath)`), and the status commit runs in the weft worktree, so warp-side git state cannot block it. Inheriting the two-sided form would be worse than merely redundant — it would freeze every status commit for the whole duration of a live warp conflict-resolution session, the one moment a resuming machine most needs to know the run is Stuck and since when.
- Rationale: git refuses a path-scoped commit while the **weft's own** `MERGE_HEAD` is live, and that state is reachable — `mergeresolve.mergeInErrorResult` (`internal/mergeresolve/mergeresolve.go:68-78`) deliberately leaves foreign merge state untouched and goes Stuck. Without the skip, every subsequent persist would hard-error and turn a recoverable Stuck into a dead run.
  `Fabric.MergeInProgress` cannot serve as the probe: it is `mergeRecordExists()`'s bare boolean and "never consults `foreignMergeStatePresent`" (`mergelifecycle.go:407-413`), so it is false in precisely the foreign-state case the skip exists for — and it needs an open `*Fabric`, which the closure does not hold.
- Probe-error disposition: a non-nil error from `MergeStateActive` is treated exactly like `true` — warn and skip, no commit, no push, no error out of `persist`. An unreadable probe is the same category of "git state cannot be trusted right now" that the skip exists for, and arguably a stronger instance of it: probe I/O failures are most likely precisely when foreign merge machinery is touching the repo, which is the worst moment to attempt a path-scoped commit.
- Rejected: probing via `Fabric.MergeInProgress`; hard-erroring by design; falling back to a full-tree commit mid-merge; hard-erroring on a probe error, which defeats the skip's own purpose; committing anyway on a probe error, which risks the git-refuses-mid-merge failure the skip was built to avoid.

### mergestate-weft-fields-stay

- Decision: `mergeState`'s four weft fields (`WeftStart`, `WeftSource`, `WeftOutcome`, `WeftCommitted` — `internal/fabricengine/mergestate.go:44-51`) are kept and filled, recording the weft as unmoved: the current weft SHA for start and source, the up-to-date outcome string, and whatever the conclude path leaves in `WeftCommitted`.
- Rationale: they are load-bearing, not descriptive. `mergeAttemptIncompleteReason` (`mergelifecycle.go:236-240`) refuses a resume when `WeftOutcome == ""`, and `mergeguards.go:296,324` read `WeftCommitted`/`WeftOutcome`/`WeftSource`. Filling them as unmoved also leaves the persisted JSON schema byte-compatible, so a merge-state file written by a pre-change binary stays readable by a post-change one and vice versa.
- Rejected: dropping the fields (breaks resume and the persisted schema); leaving them empty (`mergeAttemptIncompleteReason` refuses every resume).

### landing-checkpoint-stays

- Decision: `landingshed.Deps.CommitStatus` and its calls in **both** landing producers (`internal/landingshed/finalize.go:123` and `publish.go:114`) are kept unchanged.
- Rationale: with the per-transition hook wired it is a no-op on the ordinary path (`StageAndCommit` reports `committed == false` on a clean tracked path), and it is the only protection if a product wires `Shed.CommitStatus` as nil. Removing it would couple the generic landing producers to a Shed persistence policy they cannot see.
- Rejected: removing the field as subsumed; marking it deprecated.

### task-branch-is-the-record

- Decision: `_lyx/discussion/`, `_lyx/plan/`, `_lyx/webster/`, and `_lyx/fabric/` live only on the task branch. They never reach the parent, and there is no archive mechanism preserving them past branch deletion.
- Rationale: the parent carries the code a task produced, not the scaffolding that produced it. There is deliberately no claim about archive tags: loomyard creates no git tags, and `fabricengine/doc.go` records that a squash leaves no ancestry link, so archive tagging "needs a source outside git". The branch alone is the record for as long as it exists.
- Rejected: claiming an archive tag preserves the record; carving artifacts out so they land in the parent.

### constraints-gains-one-sentence

- Decision: `CONSTRAINTS.md`'s Durable-vs-Ephemeral State Invariant gains a single rule — weft content is per-branch and is never a merge participant in either direction — with no third category introduced.
- Rationale: the brief asked for a *tracked-but-merge-local* category because it assumed a per-path rule. There is no per-path rule: the split is structural, so the invariant states one more fact rather than growing a category. The file was trimmed to rules-only — no rationale, no narrative — by `d66cefe5` on `main`, and the addition matches that trimmed voice.
- Ordering prerequisite, already discharged: `main` was merged into this branch at `60d83a96`, bringing `d66cefe5`. `CONSTRAINTS.md` here is now the 259-line trimmed file, so the addition is written against the shape `main` actually carries. Had the pre-trim 659-line copy been edited instead, the new sentence would have landed inside text `main` had already deleted, and the landing merge would have surfaced a large conflict unrelated to this task.
- Caveat the plan must check: `d66cefe5` is not on `origin/main` (`2ac41110` at the time of writing) — it reached this branch from the local `main` ref only. Pushing this branch publishes it. If `main` is rebased or the commit is amended before it lands, re-verify the file's shape before editing.
- Rejected: a third bullet-group category; a separate cross-referencing invariant section; editing this branch's pre-trim copy and letting the merge sort it out.

## Technical context

**Where the status file is written.**
`internal/shedengine/run.go:344` — `Shed.persist` is the loop's single write path, one `state.UpdateJSON` call whose mutate overwrites the Shed-owned fields.
Every state change in `run.go` (lines 113, 134, 177, 191, 201, 217, 227, 242, 254, 267) funnels through it.
`internal/shedengine/shed.go` holds the plain exported-field `Shed` struct — no `New` constructor by design; `Run` validates every field.

**Threading the closure down.**
`internal/loomrecipe/loomrecipe.go:25-39` defines `ShedPaths`; `New` at line 78 copies its fields onto the constructed `Shed` at line 96.
`internal/loomcli/wiring.go:48` and `:248` are the two fill sites.
`New`'s existing StatusPath/StatusLockPath divergence guards need no equivalent for a closure field — there is no second copy to disagree with.

**The commit call to mirror.**
`internal/loomcli/landingdeps.go:66` —
`fabricengine.CommitAnchoredPaths(fabricengine.NewMutations(""), l, []string{loomengine.LoomStatusRel()}, msg, fabricengine.EnvSyncOptions())`,
discarding `(sha, committed)` in favour of the error alone, which makes a second call over an already-clean path a no-op rather than a failure.
`CommitAnchoredPaths` (`internal/fabricengine/commitweftpaths.go:95`) is the vocabulary-neutral wrapper: `l`-in, no path out, anchor-relative paths. `PushAnchored` mirrors its doc-comment shape.
Existing push entry points: `Fabric.PushWeft` (`weftgit.go:299`, needs an open pair), `SpawnDetachedPush` (`spawn.go:40`, fire-and-forget), `PushWarpAt`/`PushWarpRebaseFreeAt` (`spawn.go:92`, `:124`, the `At`-shaped model).

**Where the weft merge happens.**
`Fabric.Merge` — `internal/fabricengine/merge.go:344`. Its weft-side calls: `syncSideBeforeMerge(rec, f.weft, f.weftPath, "weft")` (line ~438), the post-sync `f.weft.CurrentSHA()`/`IsAncestor` probe, `f.weft.MergeStart(sources.weftSHA, opts.Squash)` (line 503), and `concludeMergeSides`' weft arm (`mergelifecycle.go:70-91`).
`Fabric.MergeIn` — `merge.go:116` — has the same shape plus `unifyConflictPaths(warpConflicts, weftConflicts, ...)`.
`mergeState` carries `WeftStart`, `WeftSource`, `WeftOutcome`, `WeftCommitted`, kept and filled as unmoved per `mergestate-weft-fields-stay`.
`MergeAbort`/`resetMergeSides` restore both sides.

**The weft-side guards, which are not merge participation.**
Four guards evaluate the weft unconditionally and OR its verdict into the same aggregated reason as warp, so none of them reveals which side failed:
`pairDirtyReason` → `worktreeDirty(scopeTracked, f.weftPath)` (`mergeguards.go:137-152`),
`detachedHeadReason` → `f.weft.HeadDetached()` (`:182-196`),
`syncedToUpstreamReason` → `sideNotSyncedToUpstream(f.weft, f.weftPath)` (`:229-234`),
and `resolveMergeSources`' weft arm (`:84-97`), which can append `mergeReasonNotFabricManaged` or `mergeReasonSourceNotFound`.
Separately, `merge.go:447` calls `syncSideBeforeMerge(rec, f.weft, f.weftPath, "weft")` before the merge proper.
`weft-guards-drop-with-it` disposes of all six sites.

**Teardown.**
`Topology.Cleanup` (`internal/fabricengine/cleanup.go:97-102`) finds weft branches with no warp worktree sibling and deletes them per a flag matrix; `raddleFoldedBack` at line 93 is the stub gate being removed.
The primary weft branch is protected unconditionally by `primaryWeftBranch` and stays so.
`removeWeftWorktree(rec, l, slug, branch, force, alsoDeleteBranch, branchPrefix)` (`weftwiring.go:202-224`) is the per-pair path.

**Gotchas found during exploration.**

- `fabric.yaml`'s `pathspec` key routes *extra* directory names to the weft side. `template.yaml` seeds it as `""` and the only `_extra` occurrences in the tree are test fixtures, so by default the weft holds `_lyx` alone. The rule needs no exceptions list: anything routed to weft is per-branch-local.
- A task worktree's `_lyx/` holds `loom/status.json`, `discussion/` (`decision-record.md` and `support-log.md`, per `internal/loomengine/config.go:60,73`), `plan/` (`planparser.PlanDirRel`), `webster/`, and `fabric/` (the provenance record). Stencils are **not** in it — they live at `<hub>/_board/_lyx/stencils`, the board worktree (`junctionnames.go:122-126`), and are untouched by this change.
- `git` refuses to stage through a junction, so `_lyx/` paths can only be staged in the weft worktree directly (`internal/fabriccli/merge_verbs.go:91,216`). Nothing in this design stages `_lyx/` during a merge, which is one more thing the redesign removes.
- A child's weft branch is created from the parent's weft at spawn, so a child inherits whatever `_lyx/` the parent branch carries. Under this design the parent accumulates none, so the inherited tree is empty in practice — except on a parent that ran loom directly on itself. The plan should confirm this rather than assume it.
- `pairDirtyReason` (`internal/fabricengine/mergeguards.go:137-145`) checks dirtiness with `scopeTracked`, so untracked files never block a merge.
- **The pull side is an operator step, not loom's.** Nothing in `internal/loomcli` pulls; a second machine resumes by pulling the branch itself (`lyx fabric pull`). The transition push is branch-scoped rather than path-scoped, so it also carries the artifact commits made by the separate discussion and plan closures (`wiring.go:179`, `:205`) — those reach the remote through it, not through any push of their own.

## Constraints

From `CONSTRAINTS.md` as it stands on this branch after the rules-only trim (`d66cefe5`) was merged in — 259 lines, rules only, no rationale and no narrative.
Keep the addition in that voice.
Each entry below names the invariant heading verbatim:

- **Durable-vs-Ephemeral State Invariant** — the invariant being extended. `_lyx` holds tracked content only; `.lyx` holds never-tracked content at the mirrored subpath; neither is read from `fabric.yaml`'s `pathspec`.
- **Fabric Vocabulary Invariant** — warp/weft vocabulary is `fabricengine`-private. No new caller-facing identifier may contain `Weft` or `Warp`: `PushAnchored`, never `PushAnchoredWeft`.
- **Fabric Git Invariant (warp + weft)** — every git operation LYX performs goes through `internal/fabricengine`, in-process, never raw git and never an agent. Weft-commit callers pass positive-only pathspecs built via `ScopedPathspec`.
- **Told-Geometry Invariant** — an engine is handed its absolute paths and derives none. `shedengine` is bound with machine enforcement.
- **Shed Producer-Seam Invariant** — `internal/shedengine` production imports are allowlisted to stdlib, `internal/state`, `internal/lock`.
- **Lyxdirs Single-Declarer Invariant** — no hand-built join naming the `_lyx` literal outside its declarer.
- **Fabric Destruction Chokepoint Invariant** — branch deletion and worktree removal go through the declared chokepoint; the `Cleanup` change must not route around it.
- **hubforge Fabric-Fixture Invariant** — every hub fixture is built by `internal/hubforge` through `fabriccli.CloneAndWire`.
- **gitrepo Client Boundary Invariant** / **gitexec Checked-Call Invariant** — not engaged: this task adds no `gitrepo` method and no checked call.
- **Never Force-Add Invariant** — the per-transition commit passes a positive-only pathspec and never reaches for `git add -f`, even when the status path is excluded on the other side.
- **Mutation Record Invariant** — every mutating result type embeds `MutationRecord`. `PushResult` already does (`internal/fabricengine/weftgit.go:269-271`), so `PushAnchored` returning one satisfies this without a `rec *Mutations` parameter — matching `PushWarpAt`/`PushWarpRebaseFreeAt`, neither of which takes a recorder.
- **Markdown Link Integrity** — `loom.md`'s `#crash-recovery--resume-on-output-files-not-live-processes` heading is linked from `roadmap.md` and from within `loom.md`; the heading text stays exactly as written.

From the project `CLAUDE.md` and `docs/overview.md` (the trimmed `CONSTRAINTS.md`'s **Documentation Lifecycle** section is now only a pointer to `docs/overview.md#documentation-lifecycle`, so these are not CONSTRAINTS.md rules):

- **Documentation lifecycle** — the docs named in Scope-In update in the same commit. `internal/fabricengine/doc.go` is the module's authoritative narrative and asserts two-sided merge semantics throughout (`:858-880` on both-sides self-abort, `:1023-1046` on both-sides outcome flags); `cleanup.go:3-11,70-77,175-181` documents the `raddleFoldedBack` flag matrix being deleted; `mergeguards.go`'s comments assert two-sided guard semantics throughout, and `sideConcludeMayHaveLanded` (`:424-437`) states outright that "an up_to_date side is never concluded and cannot move" — false for a weft that commits every transition; `pull.go:85-101`'s `PartialPullError` contract asserts a weft-side failure can never produce one. A plan writer who skips these leaves all four asserting removed behaviour. `manifest/roadmap.md` does **not** move: this is a reopened bug, not a completed or newly added planned item.
- **Markdown semantic line breaks** — one sentence per line, break at internal independent-clause boundaries, plain newlines only.

Discovered during discussion:

- `manifest/designs/loom.md`'s resume paragraph asserts cross-machine resume does not work and that fixing it "is a `Shed` persistence-policy decision with a real per-transition git cost". This task makes that decision, so those lines are rewritten.
- The same file's landing-checkpoint paragraph explains the checkpoint as load-bearing against the merge guard. It must now say the checkpoint is a no-op safety net, and that the weft is no longer merged at all.
- `manifest/designs/shed.md`'s status-file contract section gains the per-transition commit as a Shed-level persistence policy.

## Testing

**`internal/fabricengine` — integration tests over real `hubforge` fixtures.**

- A landing merge where the child branch changed `_lyx/loom/status.json` many times: the parent's warp advances, the parent's weft is byte-identical before and after, and no conflict is reported.
- The same with the parent carrying its own diverged `_lyx/loom/status.json`: parent's copy survives untouched.
- The shape that conflicts today — both sides evolving `_lyx/` from a shared base — must now complete cleanly rather than returning `ErrMergeInRequired`.
- `MergeIn` in the opposite direction: a parent's `_lyx/` never reaches the child, and the child's live content is unchanged.
- Warp-side merging and warp-side conflict reporting are unchanged, including a genuine warp conflict still reaching `unifyConflictPaths` and `mergeresolve`.
- `Cleanup` deletes an orphan weft branch without `--force`; the primary weft branch stays protected; a checked-out weft branch stays protected.
- A dirty weft, a detached weft `HEAD`, and a weft diverged from its upstream each no longer refuse a merge that warp alone can complete. `TestMerge_FetchedDivergedWeftRefuses` and `TestMerge_UnfetchedDivergedWeftRefuses` (`merge_target_integration_test.go:793`, `:817`) invert. Warp-side dirty/detached/not-synced still refuse, unchanged.
- An unresolvable weft counterpart leaves `WeftStart`/`WeftSource` empty and still merges, rather than appending `mergeReasonNotFabricManaged`/`mergeReasonSourceNotFound`.
- `MergeAbort` and each of the three self-abort paths reset the warp only: a weft carrying status commits made during the attempt keeps them, and its HEAD is where the abort found it.
- `Fabric.Pull` against a diverged weft still pulls the warp, returning a result that reports the weft unpulled rather than an error that stops the pair. A weft with no upstream keeps its existing vacuous-success path.

**`internal/fabricengine` — direct tests for the two new functions.**

- `MergeStateActive` reports true for foreign merge state on either side (`MergeHeadPresent` or a non-empty `ConflictedFiles`), false for a clean pair, and surfaces a probe error rather than swallowing it — the closure, not the probe, decides that an error means skip.
- `PushAnchored` honours `SkipGit`/`SkipPush` from `SyncOptions`, and surfaces `gitrepo.ErrPushRejected` unwrapped so the closure can warn-and-continue on exactly that error and not on others.

**`internal/shedengine` — TDD candidate.**
`persist` calls `CommitStatus` on every write path: resume write, running, paused (with `consumePause`), blocked, failed, stuck-bounce, done.
A nil `CommitStatus` is a silent no-op.
A closure error propagates out of `persist` and halts `Run`, with the status file write itself having happened first.

**`internal/loomcli` — wiring.**
Both `wiring.go` fill sites populate `ShedPaths.CommitStatus`.
The closure commits then pushes; a push failure does not surface as an error while a commit failure does; an in-progress merge causes a skip rather than either.

**Not attempted:** an end-to-end `lyx loom run` test. Those spawn real LLM sessions, take tens of minutes, and would not exercise the diverged-parent merge shapes at all.

**Task-wide verify:** `go build ./cmd/lyx && go test ./...` (`README.md:123-124` — the full suite includes the structural invariant tests).

## Q&A log

- **Q:** Should weft merge at all? **A:** No. Weft holds system files local to one worktree and one branch; they need merging nowhere because they are local anyway. This replaces the wiki brief's per-path delete-then-restore design entirely.
- **Q:** How does loom's `status.json` survive a producer deleting the directory it lives in? **A:** It does not, which is why no deletion happens. `Finalize` is a producer inside the run and Shed persists again the moment it returns; any child-side delete is undone by the run's own next write, and in between the live FSM state is missing from the tree it is read from. Removing weft from merging removes the need for a delete at all.
- **Q:** Does the merge-in from the parent need weft? **A:** No — warp only, same argument, same direction-independent reason.
- **Q:** Does either verb gain a flag or option to express this? **A:** No. From outside there is one repo called Fabric; the behaviour changes inside `fabricengine` and no signature moves.
- **Q:** What about the raddle fold-back gate on `Cleanup`? **A:** Removed outright, not deferred. The stub gate protects every orphan weft branch from ordinary teardown, and raddle would not resurrect it — though not for the reason first written down. Raddle regenerates fresh against the parent's HEAD and commits onto the parent pair inside `Finalize`'s critical section, so it never depended on merging the child's weft and there is no fold-back to guard. Its output *does* reach the parent; only the child's copy of it never travels by merge. `CLAUDE.md`'s "`_lyx/raddle/` … merged into `main`" clause is corrected in the same commit.
- **Q:** Who tears down the weft branch? **A:** An outside verb, after nothing is in the worktree any more. Teardown cannot run from inside the worktree it removes, so no loom producer does it.
- **Q:** What happens to warp↔weft correspondence when weft never moves during a merge? **A:** Nothing changes. The index maps a warp SHA to the weft SHA current at that point, and `Merge` already records correspondence even when one side never moved.
- **Q:** What about `fabric.yaml`'s `pathspec` key, which can route extra directories to weft? **A:** It defaults to empty and has no shipped use case — the only `_extra` occurrences are test fixtures. The rule takes no exception for it: anything routed to weft is per-branch-local.
- **Q:** Where does the per-transition commit hook live? **A:** `Shed.CommitStatus`, called from `persist`. Hooking producer transitions would miss `state`/`history`/`error` changes and the pause/resume writes.
- **Q:** Does the per-transition commit also push? **A:** Yes, respecting `SkipPush`. An unpushed commit does not deliver cross-machine resume.
- **Q:** What happens when the commit or push fails? **A:** Commit failure hard-errors from `persist`; push failure warns and continues, self-healing on the next transition.
- **Q:** And while a merge is in progress, when git refuses a path-scoped commit? **A:** Skip — no commit, no push, no error. Otherwise a recoverable Stuck becomes a dead run.
- **Q:** Does `landingshed.Deps.CommitStatus` survive? **A:** Yes. It is a no-op on the ordinary path and the only protection if a product wires `Shed.CommitStatus` as nil.
- **Q:** Do `discussion/`, `plan/`, and `webster/` reach the parent? **A:** No, and that is intended. The parent carries the code, not the scaffolding.
- **Q:** Is the archive tag the durable record of the scaffolding? **A:** No — loomyard creates no tags and there is no snapshot mechanism; every `Snapshot` in the tree is `Mutations.Snapshot()`, an in-memory mutation record. The branch alone is the record.
- **Q:** Does `CONSTRAINTS.md` gain a third state category? **A:** No. There is no per-path rule to describe, so the existing invariant gains one sentence: weft content is per-branch and never a merge participant.
- **Q:** Which push primitive does `PushAnchored` use? **A:** `gitrepo.PushRebaseFree`, never `PushCoalesced`. `PushCoalesced`'s rebase-retry rewrites this side's SHAs on a rejected push and invalidates the correspondence index, and it takes a repo-root push lock that would contend with existing push paths on every transition.
- **Q:** How does the closure detect that a merge is in progress? **A:** A new `fabricengine.MergeStateActive(l)`, consulting `MergeHeadPresent()` and `ConflictedFiles()` on both sides. `Fabric.MergeInProgress` cannot serve — it answers "does fabric have a merge record", is false for foreign merge state, and needs an open `*Fabric`.
- **Q:** Is this branch's `CONSTRAINTS.md` the file the addition is written against? **A:** It is now. The branch predated the rules-only trim and carried the 659-line version, so `main` was merged in at `60d83a96` and the file here is the 259-line trimmed one. The trim commit `d66cefe5` is not yet on `origin/main`, so pushing this branch publishes it.
- **Q:** Does the weft keep its power to *block* a merge once it stops participating in one? **A:** No — all four guards drop their weft arm and `syncSideBeforeMerge` skips the weft entirely. A non-participant's state cannot affect a warp-only merge's correctness, and keeping the sync guard would be decisive against the task: warn-and-continue on a rejected push makes a diverged weft a routine state, so `mergeReasonNotSynced` would refuse every subsequent landing — PR #208's failure relocated rather than removed. `resolveMergeSources` keeps its weft SHA *read* for the mergeState fields, losing only its power to refuse.
- **Q:** Does the probe check both sides? **A:** No, the weft alone. Warp and weft are independent clones with independent `.git`, and the commit runs in the weft worktree, so warp merge state cannot block it. Checking it anyway would freeze status commits for the whole of a warp conflict-resolution session — exactly when a resuming machine needs to see the run is Stuck.
- **Q:** What happens to the operator's `lyx fabric pull` once a diverged weft is routine? **A:** The weft arm stops being fatal: it warns, the warp pull proceeds, and the result reports the weft unpulled. Reconciling a diverged weft stays a named manual step, because a rejected push means another machine advanced the same FSM state. This reshapes `PartialPullError`, whose contract currently says a weft-side failure can never produce one.
- **Q:** And a merge abort, now that the weft moves on its own? **A:** It resets the warp only. Resetting the weft would discard already-pushed status commits and leave it behind its own origin, breaking the next push too.
- **Q:** And when the `MergeStateActive` probe itself errors? **A:** Warn and skip, same as `true`. An unreadable probe is the same "git state cannot be trusted" category the skip exists for, and probe failures cluster precisely when foreign merge machinery is touching the repo.
- **Q:** What happens to `mergeState`'s weft fields when weft never merges? **A:** Kept and filled as unmoved. `mergeAttemptIncompleteReason` refuses a resume when `WeftOutcome == ""`, and keeping them leaves the persisted JSON schema compatible in both directions.
