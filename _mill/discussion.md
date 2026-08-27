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
- `CONSTRAINTS.md`, `manifest/designs/loom.md`, `manifest/designs/shed.md` — docs, same commit.

**Out:**

- **A per-path local-only mechanism of any kind.** No `MergeOptions.LocalOnlyPaths`, no delete-then-restore, no forced `--no-ff`, no path-scoped reset or checkout. The split is already structural; a per-path rule would re-encode it in config.
- **`internal/gitrepo`.** No new primitives are needed — skipping a side means not calling `MergeStart` on it.
- **`internal/landingshed`.** No new deps, no new step, no `Publish` change. Both producers are untouched.
- **Teardown as part of a loom run.** Deleting the weft branch cannot happen from inside the worktree it belongs to; it stays an outside verb.
- **Raddle.** `_lyx/raddle/` does not exist yet. Its fold-back concern is deferred wholesale, and its placeholder gate is removed rather than kept warm.
- **The millhouse repo.** `mill-merge`'s own `_mill`-hardcoded delete-then-restore is not generalized here; different repository, and this design needs no equivalent.
- **Moving `status.json` to `.lyx/`.** PR #208's approach stays rejected.

## Decisions

### weft-is-never-merged

- Decision: `Fabric.Merge` and `Fabric.MergeIn` merge the warp side only. The weft side is not a merge participant in either direction.
- Rationale: everything routed to the weft belongs to one worktree and one branch. It never describes another branch's state, so there is nothing a merge could usefully carry. Merging it produced conflicts on loom's own bookkeeping — the exact failure PR #208 was opened over.
- Rejected: a per-path local-only set with delete-then-restore at the merge boundary (the wiki brief's design — solves one file's symptom while leaving every other weft file merging, and needs a delete that loom cannot safely perform on itself mid-run); leaving weft merging and accepting conflicts.

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
- Rationale: it is a stub returning `false`, so today every fabric-managed orphan weft branch is protected unless `--force` — which makes routine teardown require the destructive flag. Raddle does not exist; the gate is re-added with it if it is wanted.
- Rejected: keeping the stub gate and routing teardown through it.

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
  `MergeStateActive` answers "is either side of the pair mid-merge at the git level", consulting exactly what the unexported `foreignMergeStatePresent` (`internal/fabricengine/mergestate.go:257-276`) already consults: `MergeHeadPresent()` and `ConflictedFiles()` on both sides. It takes an `l *lyxcwd.Location`, not an open `*Fabric`, matching `CommitAnchoredPaths`/`PushAnchored`'s shape.
- Rationale: git refuses a path-scoped commit while `MERGE_HEAD` is live, and that state is reachable — `mergeresolve.mergeInErrorResult` (`internal/mergeresolve/mergeresolve.go:68-78`) deliberately leaves foreign merge state untouched and goes Stuck. Without the skip, every subsequent persist would hard-error and turn a recoverable Stuck into a dead run.
  `Fabric.MergeInProgress` cannot serve as the probe: it is `mergeRecordExists()`'s bare boolean and "never consults `foreignMergeStatePresent`" (`mergelifecycle.go:407-413`), so it is false in precisely the foreign-state case the skip exists for — and it needs an open `*Fabric`, which the closure does not hold.
- Rejected: probing via `Fabric.MergeInProgress`; hard-erroring by design; falling back to a full-tree commit mid-merge.

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
- Rationale: the brief asked for a *tracked-but-merge-local* category because it assumed a per-path rule. There is no per-path rule: the split is structural, so the invariant states one more fact rather than growing a category. The file was trimmed on `main` to rules-only, no rationale, and the addition matches that voice.
- Rejected: a third bullet-group category; a separate cross-referencing invariant section.

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
`mergeState` carries `WeftStart`, `WeftSource`, `WeftOutcome`, `WeftCommitted`; the plan decides whether those stay recorded as unmoved or are dropped.
`resolveMergeSources` resolves both sides' source SHAs; `MergeAbort`/`resetMergeSides` restore both.

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

From `CONSTRAINTS.md` (trimmed to rules-only on `main`; keep additions in that voice):

- **Durable-vs-Ephemeral State Invariant** — the invariant being extended. `_lyx` holds tracked content only; `.lyx` holds never-tracked content at the mirrored subpath; neither is read from `fabric.yaml`'s `pathspec`.
- **Fabric Vocabulary Invariant** — warp/weft vocabulary is `fabricengine`-private. No new caller-facing identifier may contain `Weft` or `Warp`: `PushAnchored`, never `PushAnchoredWeft`.
- **Fabric Git Invariant (warp + weft)** — every git operation LYX performs goes through `internal/fabricengine`, in-process, never raw git and never an agent. Weft-commit callers pass positive-only pathspecs built via `ScopedPathspec`.
- **Told-Geometry Invariant** — an engine is handed its absolute paths and derives none. `shedengine` is bound with machine enforcement.
- **Shed Producer-Seam Invariant** — `internal/shedengine` production imports are allowlisted to stdlib, `internal/state`, `internal/lock`.
- **Lyxdirs Single-Declarer Invariant** — no hand-built join naming the `_lyx` literal outside its declarer.
- **Fabric Destruction Chokepoint Invariant** — branch deletion and worktree removal go through the declared chokepoint; the `Cleanup` change must not route around it.
- **hubforge Fabric-Fixture Invariant** — every hub fixture is built by `internal/hubforge` through `fabriccli.CloneAndWire`.
- **gitrepo Client Boundary Invariant** / **gitexec Checked-Call Invariant** — not engaged: this task adds no `gitrepo` method and no checked call.
- **Documentation Lifecycle** — `CONSTRAINTS.md`, `manifest/designs/loom.md`, `manifest/designs/shed.md` update in the same commit. `manifest/roadmap.md` does **not** move: this is a reopened bug, not a completed or newly added planned item.
- **Markdown Link Integrity** — `loom.md`'s `#crash-recovery--resume-on-output-files-not-live-processes` heading is linked from `roadmap.md` and from within `loom.md`; the heading text stays exactly as written.
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
- **Q:** What about the raddle fold-back gate on `Cleanup`? **A:** Removed. Raddle does not exist, and the stub gate protects every orphan weft branch from ordinary teardown. It is re-added with raddle if wanted.
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
- **Q:** What happens to `mergeState`'s weft fields when weft never merges? **A:** Kept and filled as unmoved. `mergeAttemptIncompleteReason` refuses a resume when `WeftOutcome == ""`, and keeping them leaves the persisted JSON schema compatible in both directions.
