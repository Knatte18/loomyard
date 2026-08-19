# Discussion: fabric: merge-conflict primitive

```yaml
task: 'fabric: merge-conflict primitive'
slug: fabric-merge-conflict-primitive
status: discussing
parent: standalone-producers
```

## Problem

`Finalize` — `loom`'s and `Hardener`'s merge-back producer — has to take a finished task branch and land it on its parent.
It cannot, because Fabric has no merge.
`internal/gitrepo` has no `Merge` of any kind, and `Fabric.Diff`/`Fabric.Status` are read-only reporting surfaces that cannot detect a conflict.
`manifest/designs/finalize.md` records this as a hard dependency: `Finalize` is blocked until this primitive exists, and `manifest/roadmap.md` blocks the "loom: phase-machine scaffolding" item's `Finalize` row on the same thing.

The design constraint that makes this more than a thin `git merge` wrapper is Fabric's whole reason for existing: **Fabric exposes exactly one repo.**
Underneath it coordinates two — a warp checkout and a weft checkout — but the existence of those two sides must not be visible from the outside, ever.
`Finalize` is not in the Fabric Vocabulary Invariant's owner set, so it does not know weft exists;
it must be able to call a merge and get back what an ordinary `git merge` on an ordinary monorepo gives back: either it merged, or there are conflicts to fix.
Never "the warp side merged but the weft side conflicted".

The settling test for every decision below: take two scenarios differing only in which internal side was involved — the caller-observable result (value, error, message, exit code, envelope shape) must be identical.
That binds not just field names but result cardinality, guard-failure ordering, and partial-progress reporting.

**Why now:** the `loom` phase-machine scaffolding task is next on the roadmap and carries `Finalize` in its producer list, explicitly blocked on this one.

## Scope

**In:**

- Git-level merge primitives on `gitrepo.Repo` — the single-repo half, in lyx's own git wrapper: start (normal and squash), conclude-commit, conflicted-path enumeration, `MERGE_HEAD` detection.
- `fabricengine.MergeIn` — merge a source branch into the *current* pair, in the current worktree.
  This is where conflicts are surfaced and resolved.
- `fabricengine.Merge` — merge the task pair into a *target* pair (squash-capable), expected conflict-free.
- `fabricengine.MergeContinue`, `MergeAbort`, `MergeInProgress` — the conflict lifecycle, attached uniformly to both verbs.
- A fabric-owned per-pair merge-state record (see the `a-recorded-merge-not-a-derived-one` decision) — pre-merge SHAs, per-side outcomes, commit progress — that makes abort, crash recovery, and foreign-merge refusal possible.
- A weft-side gated hard-reset request inside `internal/fabricengine/destroy.go`, per the Fabric Destruction Chokepoint Invariant — abort and self-abort need it, and it does not exist today.
- A merge-in-progress refusal on the sibling mutating verbs (`Commit`, `Pull`, `Checkout`, `Remove`/`Cleanup`), so an open merge cannot be corrupted by an unrelated fabric write — see the lock decision's consequence for the full disposition table.
- Unified, side-free conflict reporting: one flat, sorted list of paths relative to the single visible worktree root, with a defined mapping rule and a defined refusal for paths the mapping cannot express.
- Two-sided indivisibility: no *new commit* is created on either side until both sides are conflict-free (a fast-forward moves a ref without creating a commit and is reversible from the recorded pre-merge SHA — see the `no-new-commit-until-both-sides-are-clean` decision).
- Aggregated, deterministic precondition guards on both verbs — dirty state, upstream sync, source resolution, foreign merge state — reported as one side-free error, never as a first-failure that reveals evaluation order.
- A merge-in-progress refusal guard on `Fabric.Commit`, so a routine commit during the resolution window fails with a typed fabric error instead of git's raw "cannot do a partial commit during a merge".
- Recording correspondence (`RecordCorrespondence`) for the pair a merge lands in, so the index survives a merge.
- CLI verbs `lyx fabric merge-in` and `lyx fabric merge`, with envelope and help-tree tests per the CLI/Cobra Invariant.
- Pinned-list updates in the same commit as the code they pin: the gitrepo Client Boundary method list, and any new `Kind` members in `internal/fabricengine/mutation.go` with their guard-test entries.
- Doc updates in the same commit: `internal/gitrepo/doc.go`'s scope-boundary paragraph, `internal/fabricengine/doc.go`, and the `manifest/designs/finalize.md` reword described under Decisions.
- The audit the roadmap asks for, recorded under "Audit findings" below.

**Out:**

- **Any knowledge of what is inside the repos.**
  Fabric knows `_lyx` exists only because it wired that junction from a configured name list.
  During a merge it is a directory like any other, which may or may not conflict.
- Deleting, unwiring, or excluding any directory before a merge — `Finalize`'s job, entirely outside Fabric.
- Spawning an LLM to resolve conflicts — `Finalize`'s job.
  Fabric reports conflicts and stops.
- Raddle regeneration and the campaign-level merge lock that must span it (`manifest/designs/raddle.md`) — `Finalize`'s job.
- PR creation (`finalize.md`'s `require_pr_to_base` path).
- Branch, worktree, junction and portal teardown after a merge — `lyx fabric cleanup` already owns this.
- Pushing after a merge.
  `git merge` does not push;
  Fabric already has `PushWeft` and `CoalescePushBothAt` for callers that want to.
- Post-commit rollback of a landed merge — the millhouse checkpoint affordance (`mill-merge-in` step 2's `git reset --hard "$CHK"` after a failed post-commit verify).
  This design deliberately replaces it with verify-before-conclude plus `MergeAbort` (see the `verify-before-conclude-not-post-commit-rollback` decision);
  the remaining gap — undoing a merge that has already committed — needs a two-sided reset-to-SHA verb that does not exist, recorded under "Audit findings".
- The remaining monorepo verb surface (`log`, `show`, `branch`, `tag`, `stash`, `revert`, `restore`, `rm`/`mv`, `rebase`, `cherry-pick`, `blame`) — inventoried under "Audit findings", spun out, not built here.
- Renaming `.weft/weft.write.lock`, and changing `PullResult`/`ChangeEntry`'s existing side-labelled fields.
  Both are recorded as findings, neither is touched.
- Surfacing merge-in-progress state in `lyx fabric status` output — reachable through the Go API now, recorded as a follow-up under "Audit findings".

## Decisions

### fabric-is-a-content-blind-git-operator

- Decision: Fabric is a git operator that masks two repos as one, and nothing more.
  It has no knowledge of, and takes no decisions about, the content of the branches it merges.
  Every "what should cross the merge boundary" question belongs to the caller.
- Rationale: this is the governing principle the whole design falls out of, restated by the operator repeatedly during discussion.
  Every alternative considered below was rejected the moment it required Fabric to reason about a path's meaning.
- Rejected: scoping the weft-side merge's content to `_lyx/raddle/` per the current wording of `finalize.md`;
  filtering task-local artifacts out of the merge;
  a merge that knows `_lyx` is special.

### git-primitive-in-gitrepo-coordination-in-fabricengine

- Decision: the git-level merge primitives (merge start, conclude-commit, conflicted-path enumeration, `MERGE_HEAD` detection) land on `gitrepo.Repo`, operating on one repo.
  `fabricengine` owns the two-sided coordination, the guards, the lock, the merge-state record, and the unified result shaping.
  No production code calls git except through `internal/gitexec`, which both packages already do.
- Rationale: `gitrepo` is lyx's own wrapper over git operations, so a git operation belongs there.
  `fabricengine` already calls `gitexec.Run` directly for topology work (`add.go`, `checkout.go`, `destroy.go`, `index.go`), so either placement would have compiled — but putting the raw merge plumbing in `fabricengine` would leave `gitrepo` a wrapper that cannot do the one git operation this task is about.
- Rejected: `fabricengine`-only, calling `gitexec` directly for merge plumbing.
- Consequence: `internal/gitrepo/doc.go`'s "Scope boundaries" paragraph currently reads "Rebase, interactive staging, cherry-pick, **conflict resolution**, and general-purpose branch/checkout management are explicitly not supported".
  That sentence must be amended in this task's commit to admit the merge surface, with the same honesty the rest of that paragraph shows — merge is admitted, rebase and cherry-pick stay out.
- Consequence: every new merge method reaching the git CLI lands on `runChecked` and joins the gitrepo Client Boundary Invariant's pinned method list in the same commit;
  the per-package raw-site counts in the gitexec Checked-Call Invariant stay unchanged.

### two-verbs-mergein-then-merge

- Decision: two verbs, mirroring the millhouse workflow.
  `MergeIn(<source>)` merges the source branch into the current pair, in the current worktree, and is where conflicts are surfaced and resolved.
  `Merge(<source>, opts)` then merges the task pair into the target pair, conflict-free by construction.
- Rationale: conflict resolution belongs in the task pair's worktree, never the target's.
  The grounds are policy, not git mechanics: the agent driving resolution is bound to its own worktree (worktree isolation), the target worktree must never be disturbed mid-resolution, and the unbounded resolution window must not sit inside the target pair.
  Git itself would permit resolving a `git merge <source>` conflict in the target worktree;
  what git forbids is checking out a branch already checked out in another worktree — which is why the inverse design, pulling the target branch into the task worktree to resolve there, is impossible whenever the target branch is materialized elsewhere.
  `mill-merge-in` and `mill-merge` in millhouse solved exactly this split and are the working precedent.
- Rejected: one `Merge` verb used in both directions.
  The two calls have genuinely different guards, different failure modes, and different worktrees;
  collapsing them hides that.

### merge-runs-on-a-handle-opened-at-the-target

- Decision: `Merge` is called on a `Fabric` handle opened at the **target** pair's worktree, taking the source branch as its single branch argument — `git merge <branch>` transposed.
  Fabric resolves no topology of its own to find the target.
  The caller acquires the handle exactly as for any other verb: resolve the target pair's worktree to a `*lyxcwd.Location` (for a non-cwd worktree, via `lyxcwd.ResolveWorktree(worktreeRoot)`) and call `fabricengine.Open(l)`.
  Discovering *which* worktree is checked out on the target branch is the caller's job, done through fabric's existing pair enumeration (the `pairs` verb / `PairStatus` machinery) — the lyx analogue of millhouse resolving the parent worktree from `git worktree list`.
  When no worktree is checked out on the target branch, `Merge` cannot run: the caller gets `Open`'s ordinary failure and materializes a pair first via `lyx fabric add`/`checkout`.
  Fabric never materializes a pair on the caller's behalf.
- Rationale: matches `git merge`'s own call shape, keeps Fabric from inventing a parent-resolution rule, and keeps the caller in control of which worktree gets mutated.
  A merge is materialized in an index and working tree, so it needs a worktree on the target — and in the hub layout the target branch normally has one, since parents are ordinary checked-out pairs.
  Requiring the caller to materialize the rare unmaterialized case is far simpler than a second, worktree-free merge mechanism, which would need its own conflict path, its own recovery, and its own guard refusing the *common* case where the target is checked out.
  The Go API is path-anchored, not cwd-gated, so `Finalize` — a Go producer running with the task worktree as its cwd — drives the target-pair merge without violating the Cwd Resolution Invariant or worktree isolation: mutating the target pair through the sanctioned merge verb is the lyx analogue of `mill-merge`'s `git -C <parent-path>`.
  The CLI verb keeps the standard cwd gate (run `lyx fabric merge` from inside the target pair), matching every other fabric verb.
- Rejected: `Merge(source, target)` with Fabric resolving and checking out the target itself — that is the forbidden "check the target out here" shape in disguise;
  a worktree-free merge computed in the object database (`merge-tree`/`commit-tree`/`update-ref`) — it removes the target-worktree requirement but refuses whenever the target *is* checked out, because advancing the branch ref would desynchronise that worktree's index and files, and that is the normal case here;
  Fabric creating and tearing down a temporary target pair (worktree lifecycle is `add`/`cleanup`'s domain).
- Note: the operator's cross-worktree discipline (never `cd` into another worktree, always `git -C <path>`) is honoured for free, because a `Fabric` handle is already path-anchored — `f.warpPath`/`f.weftPath` — and every `gitexec.Run` call takes its directory explicitly.

### default-git-merge-semantics-with-a-squash-option

- Decision: `MergeOptions{Squash bool, Message string}`.
  Semantics are plain `git merge`: merge automatically when possible — including fast-forwarding when git would — report conflicts and require manual resolution when not.
  `Squash` is applied identically to both sides.
  `MergeIn` never squashes — it is a real merge, preserving ancestry, exactly as `mill-merge-in` does.
  When `Message` is empty, each side's conclude-commit uses git's own prepared message (`MERGE_MSG`/`SQUASH_MSG`), exactly as a non-interactive `git commit` concluding a merge would;
  a non-empty `Message` is used verbatim on both sides.
  The CLI's default is git's default (no squash, no `-m`);
  `Finalize` passes `Squash: true`.
- Rationale: squash-merge is the normal case for task branches — without it, parent history accumulates a merge commit plus every task commit, which the operator described as unacceptable clutter.
  But Fabric is a general git operator, not Finalize's helper, so the option is the caller's.
  Deferring to git's prepared messages keeps the surface git-shaped and gives the CLI a working no-`-m` default.
- Rejected: hard-coded squash (contradicts "nearly always", and contradicts the ordinary-git surface);
  per-side strategy (forces the caller to know there are two sides);
  a fabric-composed default message (invents a format git already provides).

### a-recorded-merge-not-a-derived-one

- Decision: every merge is recorded in a fabric-owned, per-pair, never-exported state record, written under the combined write lock *before* the first merge command runs and deleted when the merge concludes (both sides committed), aborts, or self-aborts.
  It lives beside the correspondence index, inside the weft checkout's git directory (`weftGitDir()`), as `fabric-merge.json`.
  `MergeInProgress` reports true iff the record exists.
  Git-level merge state (a `MERGE_HEAD`, unmerged index entries) found on either side *without* a record is a foreign merge — one Fabric did not start — and every merge verb refuses it with `*ErrForeignMergeState`, telling the operator to conclude or abort their own merge with plain git first.
- Rationale: this reverses an earlier decision to derive in-progress state from git alone, and the reversal is forced by four defects of the derived design at once.
  (1) `git merge --no-commit` cannot stop a fast-forward — git's own documentation: "fast-forward updates do not create a merge commit and therefore there is no way to stop those merges with --no-commit" — so one side's ref can move while the other conflicts, and only pre-captured SHAs make that reversible.
  (2) A squash merge records no `MERGE_HEAD`, so a crash between a conflicted `--squash` and the self-abort leaves a state git-derivation reports as "no merge in progress" while the pre-merge SHAs needed for recovery died with the process;
  the record survives the crash and `MergeAbort` completes the recovery, keeping the lifecycle inside the Fabric Git Invariant.
  (3) `CONSTRAINTS.md` explicitly permits a human to run ordinary git in their warp worktree, so a human's conflicted `git merge` puts git-level merge state on one side;
  a git-derived `MergeInProgress` would adopt that foreign half-merge, and `MergeContinue`/`MergeAbort` would then act on a merge whose other side has no counterpart and no captured SHAs.
  The record turns exactly that case into a clean refusal.
  (4) The public `MergeAbort` needs pre-merge SHAs from *some* cross-call store in every non-`MERGE_HEAD` case;
  without the record that store does not exist.
- Rejected: deriving `MergeInProgress` from git state (all four defects above);
  a worktree-visible state file under `.lyx` — the git-dir placement follows `corrIndexPath`'s precedent, sits in git-metadata space the Durable-vs-Ephemeral State Invariant's `_lyx`/`.lyx` mirroring rule does not govern, and can never itself conflict, be reset away, or appear in any status output.
- Consequence: the record is fabric-internal;
  its field names may use warp/weft vocabulary (owner set), and nothing of it is ever exported, serialized into a result, or printed.

### no-new-commit-until-both-sides-are-clean

- Decision: both sides are attempted before returning, even when the first attempt conflicts, so the returned conflict list is complete.
  A non-squash merge runs with `--no-commit`, a squash merge with `--squash`;
  in both cases no *commit* is created on either side until both sides are conflict-free, at which point Fabric concludes both (see the commit-phase decision below).
  A side that fast-forwards is allowed to fast-forward — git moves the ref, creates no commit, stages nothing — and a side that is already up to date is left untouched;
  each side's outcome (staged / conflicted / fast-forwarded / already-up-to-date) is written into the merge-state record, never into the result.
  When the merge does not conclude in the same call, `MergeAbort` restores *both* sides to their recorded pre-merge SHAs unconditionally — including a fast-forwarded side, and including a side that never moved — through the gated reset executors, so the pair returns to its exact pre-merge state and the abort's observable behaviour never depends on which side did what.
- Rationale: this is what makes the one-repo illusion hold against git's real behaviour.
  The earlier wording — "neither side commits until both are clean" — was unachievable as specified, because `--no-commit` cannot stop a fast-forward;
  the honest invariant is "no new commit until both are clean, and every ref movement is reversible from the record".
  During the resolution window the visible worktree looks exactly like a mid-merge monorepo: non-conflicted files (including junctioned ones a fast-forward updated) already carry merged content, conflicted files carry ordinary markers.
  Attempting only the first side would be worse: the caller resolves one side's conflicts, continues, and only then discovers the other side's.
  One call, one outcome — merged, or conflicts.
- Rejected: `--no-ff` on both sides to force stoppable merges (changes history shape, contradicting plain-git semantics);
  first-side-commit-then-second with rollback (leaves a real rollback window and cannot be described without naming a side);
  report-not-rollback in the style of `*PartialCommitError`/`*PartialPullError` (necessarily names a side, which is exactly what is forbidden here).
- Precedent: `checkout.go`'s all-or-nothing coordinated switch for the pre-commit window — not `commit.go`'s partial-report.

### already-up-to-date-is-a-result-not-a-fabrication

- Decision: when *both* sides have nothing to merge, `MergeIn` returns `MergeResult{AlreadyUpToDate: true}` without taking the lock, writing a record, or recording a mutation — the degenerate no-op, per `Fabric.Commit`'s no-op precedent and `mill-merge-in`'s step-1 fast path that `mill-merge` depends on.
  The both-sides case is decided by a **pre-lock `IsAncestor` probe** on each side (the source ref already an ancestor of that side's HEAD), never by `MergeStart`'s `MergeAlreadyUpToDate` outcome, which is only observable after the lock is taken and the record written.
  `MergeStart`'s outcome serves the one-side case only.
  When only one side has something to merge, the merge proceeds: the no-op side's outcome is recorded as already-up-to-date, `MergeContinue` concludes only sides whose recorded outcome is staged and uncommitted, and no empty commit is ever fabricated on the no-op side.
  `RecordCorrespondence` then pairs the post-merge HEADs of both sides — pairing a new SHA with an unchanged one is legal and correct, because the index maps corresponding *states*, not deltas.
- Rationale: an empty commit on the up-to-date side would fabricate history to satisfy symmetry no caller can observe anyway;
  skipping it requires per-side discrimination, which the record provides internally without exporting it.
- Rejected: forcing an empty commit on the no-op side;
  treating one-side-no-op as an error (it is the routine case whenever only one repo changed).

### commit-phase-concludes-per-side-and-never-rolls-back

- Decision: once both sides are conflict-free, `Merge`/`MergeContinue` conclude the merge under the combined lock: warp's conclude-commit first, then weft's — a fixed internal order — writing each side's resulting SHA into the record as it lands.
  If the second commit fails after the first landed, nothing is rolled back:
  the record is kept, the verb returns `*ErrMergeIncomplete` (fixed, side-free text instructing the caller to run `MergeContinue` again;
  the underlying git failure is written to the internal log, never into the error), and `MergeContinue` is idempotent — a side whose recorded committed-SHA is already set is skipped, so the retry concludes only what remains.
  Only when both sides have concluded is correspondence recorded and the record deleted.
- Rationale: two `git commit` invocations on two repos cannot be atomic, and this is the only genuinely dangerous window.
  Rolling the landed side back would hard-reset away the merge commit *and* the caller's just-resolved content — a materially worse outcome than the branch-switch rollback `checkout.go`'s precedent covers — so the recovery affordance is idempotent completion, not undo.
  `MergeAbort` remains available and honest even here: it returns both sides to the recorded pre-merge SHAs, explicitly discarding the resolution work, which is what abort means.
- Rejected: `checkout.go`-style rollback of a landed merge commit (destroys resolution work);
  a partial-report error naming the landed side (forbidden);
  pretending the window does not exist (the earlier draft's position).

### conflicts-are-reported-as-unified-worktree-relative-paths

- Decision: the conflict result is one flat, lexically sorted list of paths relative to the single visible worktree root, with no side label anywhere in the type.
  The mapping rule: a warp-side conflicted path (already repo-root-relative from `git diff --name-only --diff-filter=U`) passes through unchanged;
  a weft-side conflicted path maps by identity iff it lies under `<AnchorRel>/<wired-name>/` for a name in the wired junction set (`WiredNames`) — the junction geometry guarantees the weft checkout mirrors the anchor subpath, so the weft-relative path *is* the visible worktree-relative path, and the file is literally reachable there through the junction.
  A weft-side conflicted path outside that set — repo-root files such as the warp-binding record, a README, pre-fabric legacy content — has no visible-tree address at all;
  a conflict there aborts the merge on both sides (gated resets to the recorded SHAs) and returns `*ErrUnmergeableState` with a fixed, side-free message ("merge produced conflicts outside the fabric-managed tree; operator intervention required"), the offending detail going to the internal log only.
  The same refusal applies to the theoretical collision where both sides report the same unified path.
  Where the geometry comes from, since `Fabric` holds only `warp`/`weft`/`warpPath`/`weftPath` and `Open` discards the `*lyxcwd.Location` it was given: the merge path re-resolves it with `lyxcwd.ResolveWorktree(f.warpPath)`, taking `AnchorRel` from the returned location and the wired name set from `RepoWiredNames(l)` — the one call that derives the config base itself, so no caller re-derives it.
  `filepath.Dir(f.warpPath)` is **not** the config base: it is the hub, which `Fabric.ResetHard` uses as a containment container only, whereas the fabric config lives at `<hub>/_board`.
  Resolution happens once per merge call, before any mutation, so a failure costs nothing: a resolve or config-read failure is a guard-stage failure returning the wrapped error, with nothing started and no record written.
  It is deliberately not cached on the handle — `Commit` re-reads the same config per call for the same reason, and a merge is rare enough that one extra read is irrelevant beside a wrong answer after a `reconcile`.
- Rationale: the identity mapping is pure geometry — Fabric's own domain — not content knowledge, and for wired content it is exact by construction.
  The unmappable class cannot arise from fabric-driven history: fabric's own weft write path routes commits through the pathspec/commit-routing set only, so two fabric-managed branches can never diverge on an unmapped weft path;
  such a conflict proves out-of-band writes, which Fabric refuses to half-report rather than inventing a synthetic namespace that would disclose the second repo.
  Collision for wired names is impossible by construction, too: wired-name paths are excluded from the warp checkout's index (`.git/info/exclude`, plus the Never Force-Add Invariant), so warp-side merges cannot conflict there.
  Sorting the final list removes any per-side ordering information.
- Rejected: raw repo-relative paths from each side (ambiguous, leaks the two-repo reality);
  absolute filesystem paths (unambiguous but not what `git merge` hands you);
  a synthetic prefix for unmapped weft paths (a path meaningful in only one repo is itself a leak);
  paths relative to `AnchorPath()` (git's own enumeration is repo-root-relative, and on a subpath-anchored hub the wired content sits under `<AnchorRel>/…` in *both* namespaces, so root-relative is the one choice where the identity mapping holds verbatim).

### merges-name-a-sha-never-a-branch

- Decision: every merge command Fabric issues names a resolved commit SHA, never a branch name.
  Both verbs resolve the ref they were given to a SHA before starting, and do so on **both** sides — warp and weft alike — even though only the weft side's branch name carries a leaking token.
- Rationale: git writes the merged ref's own label into the conflict markers it puts in the file — `>>>>>>> <source>-weft` — and `--squash` behaves identically.
  Because wired-name paths are excluded from the warp checkout's index, every conflict under a wired path is weft-origin by construction, so this is the label on *every* conflict marker a resolving agent will ever open, not a rare corner.
  The marker content crosses into `Finalize`'s hands through the junction, which puts the internal branch-naming scheme verbatim inside content that left the Vocabulary Invariant's owner set.
  Merging a SHA makes git label the marker with the SHA instead.
  Resolving on both sides is what keeps it inside the settling test: SHA-labelling only the weft side would make marker *style* an observable tell distinguishing which side conflicted, the same asymmetry `no-new-commit-until-both-sides-are-clean` and `mutation-recording-stays-scenario-symmetric` exist to rule out.
  The cost is one `CurrentSHA`-class resolution per side before the merge starts.
- Rejected: SHA-resolving the weft side only (leaks through marker style instead of marker text);
  rewriting markers after the fact (Fabric would be editing content, which the content-blind decision forbids);
  `merge.conflictStyle`/`-X` tuning (changes marker *format*, never the label);
  renaming the weft branch scheme (the suffix is load-bearing geometry — `WeftBranchName` is its sole declarer — and the leak is the label, not the name).

### combined-lock-around-mutating-steps-only

- Decision: reuse the existing combined write lock for the mutating phases of the merge — taken around the merge attempts, the conclude-commits, and the abort resets, in `Merge`, `MergeIn`, `MergeContinue` and `MergeAbort` — never held across the conflict-resolution window.
- Rationale: the operator's point stands — a merge is a joint operation and must lock both sides, not just one.
  It already does: `.weft/weft.write.lock` is described in `commit.go` as "the combined write lock", acquired "whenever anything lands, **even warp-only**".
  Its name says weft;
  its semantics cover both sides.
  So the correct answer is to reuse it, not to invent a second lock.
  The lock lives at `<weftPath>/.weft/weft.write.lock` — inside the pair's own weft checkout — so it is per-pair: holding it across resolution would block that pair's own writers (not, as an earlier draft claimed, every other worktree), and the resolution window is unbounded when an LLM drives it, which is still reason enough not to hold it.
- Rejected: a new warp-side lock (duplicates what exists);
  holding the lock for the entire merge span (`raddle.md` demands that span, but that is `Finalize`'s campaign-level lock, not Fabric's);
  no lock.
- Consequence: during the resolution window the pair's clean side holds an in-merge index, so a routine `Fabric.Commit` on the pair would hit git's raw "cannot do a partial commit during a merge".
  `Fabric.Commit` therefore gains a cheap guard: it refuses with a typed, side-free error before touching anything when the merge-state record exists, and equally when git-level merge state exists on either side without a record — the foreign-merge case a human's plain-git merge produces.
  Both refusals exist for the same reason: without them the caller receives git's raw, unowned "cannot do a partial commit during a merge", which is the outcome the guard exists to prevent and is not made less raw by fabric not having started the merge.
- Consequence: the same refusal is extended to every sibling verb whose mutation would corrupt or be corrupted by a live merge, decided here rather than left to the plan.
  Refuse while a record exists: `Fabric.Commit` (partial commit during a merge), `Fabric.Pull` (hard-resets and re-anchors, which would discard the in-progress merge), `Checkout` (a coordinated branch switch out of a half-merged pair), and `Remove`/`Cleanup` for the pair itself (tearing down a worktree mid-merge).
  Deliberately left unguarded: `PushWeft` and the push half of `sync`, because pushing already-committed history is unaffected by an open merge in the working tree, and `Status`/`Diff`/`List`/`pairs` and every other read-only verb, which are exactly what an operator inspecting a stuck merge needs.
  All refusals reuse one typed, side-free error naming the open merge, never git's raw refusal.
- Consequence: the lock's misleading name is recorded as an audit finding, not renamed here.

### safety-guards-are-aggregated-and-side-free

- Decision: each verb verifies its full precondition set on both sides before mutating anything, evaluates *every* guard rather than stopping at the first failure, and reports all failures as one `*MergeGuardError` carrying a sorted, deduplicated list of fixed reason strings drawn from a closed set — never per-side, never path-bearing, never order-revealing.
  `Merge`'s guard set: no merge in progress (recorded or foreign);
  target worktree clean (tracked-scope dirtiness, matching `dirtyScopeTracked`);
  target branch synced to its upstream — a best-effort fetch of the target branch first (millhouse's fetch-then-sync, failure tolerated and logged), then `merge --ff-only`, never `reset --hard`;
  a side whose branch has no upstream skips the sync as a vacuous pass, per `Fabric.Pull`'s existing no-upstream rule;
  source branch resolvable and fabric-managed (see the weft-source decision).
  `MergeIn`'s guard set: no merge in progress (recorded or foreign);
  current pair clean (same scope);
  source resolvable and fabric-managed, with millhouse's freshness rule applied per side — best-effort fetch of the source's remote-tracking ref, then merge the remote-tracking ref when the local branch is behind it or absent, the local branch otherwise;
  a source resolvable on neither local nor remote is a guard failure.
  A guard failure halts before any mutation, so there is nothing to roll back.
  The upstream sync is deliberately **not** a guard: fetching and fast-forwarding move refs, and a guard set that mutates could not honour the halt-before-mutation rule while still evaluating every member.
  It is a pre-merge step that runs only after every guard has passed, on both sides, and it is recorded as a mutation.
  The guard-stage check is the read-only half — the target tip is not diverged from its upstream — and the sync then performs the advance.
  Pre-merge SHAs are captured **after** the sync, so `MergeAbort` returns the pair to its synced state rather than undoing a legitimate upstream advance the merge did not cause.
  The closed set is pinned here verbatim, so no plan-time or implementation-time author can phrase one of them into a leak: `"merge already in progress"`, `"unresolved conflicts remain"`, `"no merge in progress"`, `"worktree dirty"`, `"branch not synced to upstream"`, `"source branch not found"`, `"source branch is not fabric-managed"`.
  A reason string never interpolates a branch name or any other value;
  where the offending branch must be reported, it travels in a typed error's own field (`ErrMergeInRequired.Source`), never inside a reason string.
  Adding a member is a same-commit change to this list and to the vocabulary assertion that covers it;
  none may name a side, carry a path, or imply an order.
- Rationale: these are git preconditions, not content policy, so they are Fabric's.
  millhouse learned each one the hard way: a dirty target worktree silently absorbs a partially-applied squash, a stale target ref makes the subsequent push fail as non-fast-forward, and a stale local source merges yesterday's parent (`mill-merge-in` step 1 prefers `origin/<parent>` for exactly that reason).
  `reset --hard` is explicitly not the fast-forward mechanism, because it silently discards local-only commits on the target;
  `merge --ff-only` fails loudly instead.
  Aggregation is what keeps the guard surface inside the settling test: two scenarios differing only in which side is dirty produce the byte-identical error, and no failure ordering ever discloses that two subjects were checked.
  The dirty-pair guard on `MergeIn` — which millhouse does not have — exists because git's own refusal ("your local changes would be overwritten by merge") arrives as a raw, unowned error naming paths Fabric may have no unified address for.
- Rejected: leaving the guards to the caller (every caller would reimplement them, and the failure modes are silent);
  first-failure reporting (reveals evaluation order and therefore arity);
  passing git's own dirty-worktree refusal through (raw, unowned, potentially side-revealing);
  a mandatory fetch that fails the merge when offline (the fetch is a freshness aid, not a correctness gate — `--ff-only` against the last-known upstream still catches genuine divergence).

### merge-conflicts-are-redirected-to-mergein

- Decision: `Merge` should not conflict, because `MergeIn` is what prevents it.
  If it does, `Merge` aborts both sides itself — the same record-driven gated resets `MergeAbort` uses — deletes the record, restores the target pair exactly, and returns `*ErrMergeInRequired` telling the caller to run `MergeIn` first.
  It never leaves a conflicted state behind in the target worktree.
- Rationale: by the two-verb decision's policy grounds, conflict resolution belongs in the task pair's worktree;
  leaving conflicts in the target would strand them where no resolving agent is allowed to work.
  Redirecting to `MergeIn` is both the only recovery consistent with that policy and the millhouse workflow.
- Rejected: leaving `Merge`'s conflicts in place for the caller to resolve — unresolvable where they land, by policy;
  omitting the lifecycle from `Merge` entirely — `Merge` must still detect and clean up after a conflict, so it needs the machinery either way.

### lifecycle-quartet-on-both-verbs

- Decision: `MergeContinue`, `MergeAbort` and `MergeInProgress` apply uniformly to both verbs, driven entirely by the merge-state record.
  `MergeAbort` restores both sides to the recorded pre-merge SHAs through the gated resets and deletes the record — covering the `MERGE_HEAD` case, the squash case, and the fast-forwarded case with one mechanism, since `git reset --hard` clears git's own merge state as a side effect.
  `MergeContinue` refuses (`*MergeGuardError`, reason "unresolved conflicts remain") while either side still has unmerged entries, then concludes per the commit-phase decision;
  its optional message overrides the recorded one, and an empty message falls back to the record, then to git's prepared message.
  Both refuse with `*ErrNoMergeInProgress` when no record exists — mirroring git's "There is no merge in progress" — and never touch foreign git merge state.
  On the happy path `Merge` concludes or self-aborts within one call, so for `Merge` the quartet's cross-call role is crash recovery: a `Merge` that died mid-flight leaves its record, `MergeInProgress` reports it, `MergeAbort` restores the target pair, and `MergeContinue` can conclude a merge that crashed after clean staging.
- Rationale: uniformity keeps the surface git-shaped, and the record is what makes the uniformity real rather than decorative — the earlier git-derived design left public `MergeAbort`'s squash branch with no SHA source at all, and left every crash window unrecoverable without raw git.
- Rejected: a quartet scoped to `MergeIn` only (leaves `Merge`'s crash windows outside the Fabric Git Invariant);
  git-derived in-progress state (see the `a-recorded-merge-not-a-derived-one` decision).

### weft-source-is-derived-and-must-exist

- Decision: for a caller-supplied source branch `<source>`, the warp side merges `<source>` and the weft side merges `WeftBranchName(source)` — `<source>-weft`, composed solely by `branchname.go`'s existing derivation.
  When `<source>-weft` does not exist in the weft repo (probed via the existing `weftBranchExists`), the guard fails: `<source>` is not a fabric-managed branch, and the aggregated guard error carries the closed set's literal reason `"source branch is not fabric-managed"`.
  The branch name is never interpolated into that string;
  it travels in `ErrMergeInRequired.Source`-style typed fields where a caller needs it, per the guards decision's rule.
- Rationale: the derivation is the same sole-composition rule every other fabric surface obeys, and it belongs in a normative decision, not only the Q&A.
  A hard refusal is the only answer consistent with two-sided indivisibility: a silent warp-only merge would let the two sides' histories diverge under fabric's own hands, and forking `<source>-weft` on demand would fabricate weft history for a branch fabric never managed, without the caller asking.
  Every branch `Finalize` merges was created by `fabricengine.Add`, which forks both sides — so the refusal bites only genuinely foreign branches, where refusing is correct.
- Rejected: warp-only merge when the weft counterpart is missing;
  fork-on-demand per `add.go`'s adopt-vs-create precedent (that precedent is for creating a *pair*, where fabricating the weft fork is the caller's explicit intent).

### verify-before-conclude-not-post-commit-rollback

- Decision: the caller's post-merge verification (build, tests) runs in the resolution window — after conflicts are resolved in the worktree, before `MergeContinue` — where the visible tree already holds the fully merged content.
  A failed verify is recovered by `MergeAbort`, which is exact and two-sided.
  Fabric ships no verb that undoes a merge after it has concluded.
- Rationale: this consciously replaces `mill-merge-in`'s checkpoint branch, whose post-`merge --continue` rollback (`git reset --hard "$CHK"`) has no Fabric-level equivalent — undoing a landed merge needs a two-sided reset-to-SHA verb resolving the paired weft SHA through the correspondence index, which does not exist and is real design work of its own.
  Ordering verify before conclude gives `Finalize` the same safety with machinery this task already builds;
  the record *is* the checkpoint for the whole uncommitted window.
- Rejected: shipping a two-sided reset verb here (scope, and it deserves its own destruction-gate design);
  silently dropping the millhouse affordance without recording it (the earlier draft's position — the gap is now in Scope Out and Audit findings).

### public-surface-shapes

- Decision: the public Go surface, verbatim.
  In `internal/gitrepo` (each CLI-reaching method lands on `runChecked` and joins the pinned method list in the same commit):

  ```go
  // MergeOutcome classifies what state MergeStart left the repo in.
  type MergeOutcome int

  const (
      MergeStaged          MergeOutcome = iota // merged into index/worktree, uncommitted
      MergeConflicted                          // unmerged index entries present
      MergeFastForwarded                       // HEAD moved; nothing staged, no MERGE_HEAD
      MergeAlreadyUpToDate                     // nothing to do
  )

  // MergeStart runs `git merge --no-commit <ref>` (squash false) or
  // `git merge --squash <ref>` (squash true) and classifies the result.
  // A conflicted merge exits non-zero, so runChecked returns *gitexec.GitError;
  // MergeStart classifies on repo state, never on exit code alone. It captures
  // HEAD before the call, then on any error uses errors.As to recover the
  // GitError and probes: unmerged index entries (git ls-files -u) => Conflicted;
  // otherwise the error is genuine and returned. On success it probes HEAD
  // movement and the index: HEAD moved with nothing staged => FastForwarded;
  // nothing staged and HEAD unmoved => AlreadyUpToDate; else Staged.
  // This keeps the checked form (errors.As + state probe, the ancestry.go idiom)
  // and adds no raw gitexec site, so no pinned raw-site count moves.
  func (r *Repo) MergeStart(ref string, squash bool) (MergeOutcome, error)

  // MergeConclude commits a staged merge or staged squash. With a message it
  // runs `git commit -m <msg>`; with an empty message it runs
  // `git commit --no-edit`, which takes git's prepared MERGE_MSG/SQUASH_MSG
  // without opening an editor. The --no-edit spelling is mandatory: a bare
  // `git commit` with no -m launches the configured editor and would hang a
  // non-interactive caller forever.
  func (r *Repo) MergeConclude(msg string) error

  // ConflictedFiles enumerates unmerged paths, repo-root-relative:
  // `git diff --name-only --diff-filter=U`.
  func (r *Repo) ConflictedFiles() ([]string, error)

  // MergeHeadPresent reports whether MERGE_HEAD exists.
  func (r *Repo) MergeHeadPresent() (bool, error)
  ```

  In `internal/fabricengine` (exported surface — side-free by the Vocabulary Invariant, since it crosses to `Finalize`):

  ```go
  // MergeOptions carries the caller-facing merge knobs.
  type MergeOptions struct {
      Squash  bool
      Message string
  }

  // MergeResult is the single result type all four merge verbs return.
  type MergeResult struct {
      MutationRecord
      // AlreadyUpToDate reports the merge was a complete no-op.
      AlreadyUpToDate bool `json:"already_up_to_date"`
      // Conflicts is the unified, sorted, worktree-root-relative conflicted-path
      // list; empty and never nil when there are none.
      Conflicts []string `json:"conflicts"`
      // Committed reports whether the merge landed.
      Committed bool `json:"committed"`
  }

  func (f *Fabric) MergeIn(source string) (MergeResult, error)
  func (f *Fabric) Merge(source string, opts MergeOptions) (MergeResult, error)
  func (f *Fabric) MergeContinue(msg string) (MergeResult, error)
  func (f *Fabric) MergeAbort() (MergeResult, error)
  func (f *Fabric) MergeInProgress() (bool, error)
  ```

  The named error types (all messages fixed and side-free; none wraps a git error into its string — causes go to the internal log):

  ```go
  // MergeGuardError aggregates every failed precondition as a sorted,
  // deduplicated list of fixed reason strings from a closed set.
  type MergeGuardError struct{ Reasons []string }

  // ErrMergeInRequired: Merge found conflicts, self-aborted, and the caller
  // must run MergeIn in the source pair's worktree first.
  type ErrMergeInRequired struct{ Source string }

  // ErrForeignMergeState: git-level merge state exists that fabric did not start.
  type ErrForeignMergeState struct{}

  // ErrNoMergeInProgress: MergeContinue/MergeAbort with no recorded merge.
  type ErrNoMergeInProgress struct{}

  // ErrMergeIncomplete: the conclude phase did not finish; re-run MergeContinue.
  type ErrMergeIncomplete struct{}

  // ErrUnmergeableState: the merge produced conflicts outside the
  // fabric-managed tree; the merge was aborted and the pair restored.
  type ErrUnmergeableState struct{}
  ```

  The internal record (never exported; warp/weft naming permitted, `fabricengine` is in the owner set):

  ```go
  // mergeState is the on-disk merge record at weftGitDir()/fabric-merge.json.
  type mergeState struct {
      Verb          string    `json:"verb"`   // "merge-in" | "merge"
      Source        string    `json:"source"` // caller-supplied branch
      Squash        bool      `json:"squash"`
      Message       string    `json:"message"`
      WarpStart     string    `json:"warp_start"` // pre-merge HEAD SHAs
      WeftStart     string    `json:"weft_start"`
      WarpOutcome   string    `json:"warp_outcome"` // staged|conflicted|fast_forwarded|up_to_date
      WeftOutcome   string    `json:"weft_outcome"`
      WarpCommitted string    `json:"warp_committed"` // conclude SHAs, set as each lands
      WeftCommitted string    `json:"weft_committed"`
      StartedAt     time.Time `json:"started_at"`
  }
  ```

  Semantics binding the shapes: conflicts are a *result state*, not an error — `MergeIn` with conflicts returns `(MergeResult{Conflicts: […]}, nil)`;
  the CLI maps a non-empty `Conflicts` to a failure envelope and exit code 1, mirroring `git merge`'s nonzero conflict exit.
  `partial` stays **false** on that conflict envelope even though the mutation record is non-empty and the envelope reports failure, because the Mutation Record Invariant derives `partial` from exactly one rule — `error ≠ nil ∧ record non-empty` — and a reported conflict returns a nil error.
  This is stated because the mismatch between "failure envelope" and `partial: false` is exactly the place a plan writer would guess the other way.
  Each verb allocates its `*Mutations` recorder internally and embeds it in the result per the Mutation Record Invariant, threading it into `destroy.go`'s executors for the reset primitives.
- Rationale: the earlier draft described the surface without shaping it, leaving the entire public API to be invented at plan time;
  every field, error and default above is now decided here.
- Rejected: conflicts-as-typed-error (the merge did what a merge does — the envelope's `partial` rule and the plain-git surface both read better with conflicts as data);
  per-verb result types (four shapes to keep side-free instead of one).

### mutation-recording-stays-scenario-symmetric

- Decision: the merge verbs record mutations per the existing per-primitive contract — new `Kind` members land in `mutation.go` in the same commit as their recording sites and guard-test entries — and the design keeps the record *scenario-symmetric*: because every phase operates on both sides unconditionally (both attempted, both reset on abort, both concluded on success), two scenarios differing only in which side conflicted produce the same entry kinds against the same fixed target set in the same fixed order, differing only in SHAs.
  New kinds: `KindMergeStaged` (a merge command observably changed a checkout's state), `KindMergeCommitted` (a conclude-commit landed, detail the new SHA);
  abort's resets record through the existing `KindWorktreeReset` inside `destroy.go`, which must never be worked around.
- Rationale: the Mutation Record Invariant fixes the envelope's diagnostic channel and its per-primitive grain — including target paths — as a given;
  the illusion is therefore defended at the level this design controls, by making the recorded shape independent of which internal side did what.
- Rejected: suppressing or coarsening merge mutations to hide targets (violates the invariant's provably-total recording, and the destruction recorder is threaded into `destroy.go` precisely so it cannot be bypassed).

### weft-side-gated-reset-in-destroy-dot-go

- Decision: abort and self-abort reset both checkouts, and the weft side has no gated reset today — `weft.ResetHard(` is a banned bypass token outside `destroy.go`.
  What is genuinely new is narrow: `destroy.go`'s `resetHardTo(rec, req, repo, sha)` executor is already repo-generic — only the exported `Fabric.ResetHard` hardcodes `f.warp`, `ownedWarpCheckout` and `force: false` — so no new executor is needed, only new abort-specific `pathRequest` values declared inside `destroy.go`: container the hub, target the respective checkout, dirtiness `dirtyScopeTracked`, and `force` **true**, the one deliberate divergence from `Fabric.ResetHard`'s hardcoded `force: false`.
  The plan must first establish whether the existing `ownedRegisteredLinkedWorktree(repoDir)` ownership kind already covers the weft checkout;
  a new weft-specific kind is added only if it is shown insufficient, not assumed.
- Rationale: an abort's entire purpose is to discard an intentionally dirty worktree — unresolved conflict markers are tracked-file modifications, so a `force: false` request would refuse exactly the state the verb exists to clean up.
  Forcing is safe here and only here because the abort is record-gated: `MergeAbort` refuses to run without a fabric-written record, so what it discards is exactly the dirt accumulated since the merge started — which abort exists to discard by definition — rather than an operator's pre-existing work, which the pre-merge clean-pair guard had already excluded.
  The claim is deliberately not "provably merge-produced": `verify-before-conclude-not-post-commit-rollback` puts build- and test-fixing inside the same window, so tracked edits unrelated to conflict resolution can legitimately exist at abort time, and abort discards those too.
- Rejected: `dirtinessNA` (the dirtiness is real and known, not inapplicable — declaring it NA would be false);
  routing around the gate (banned tokens, and the recorder threading exists precisely to prevent it).

### correspondence-is-recorded-for-the-pair-a-merge-lands-in

- Decision: after a merge concludes on both sides, Fabric records the new warp↔weft correspondence via `RecordCorrespondence`, in whichever pair the merge landed in — pairing the two post-merge HEADs even when one side's SHA is unchanged (the one-side-no-op case).
- Rationale: every existing write path maintains the correspondence index;
  a merge that skipped it would leave the target pair's index unable to resolve its own new HEAD, breaking `Diff`, revert-target resolution, and `Pull`'s re-anchor logic.
- Rejected: leaving it to the caller — the caller is `Finalize`, which does not know weft exists and therefore cannot know correspondence exists.

### cli-mirrors-git

- Decision: `lyx fabric merge <branch> [--squash] [-m <message>]`, `lyx fabric merge --continue [-m <message>]`, `lyx fabric merge --abort`, and `lyx fabric merge-in <branch>`.
  Flags are git's own;
  `-m` is optional everywhere, falling back per the message-default decision.
  Arity is mode-dependent, exactly as git's own `merge` is, and the cobra `Args` validator is decided here: `merge` and `merge-in` take **exactly one** positional branch argument in their default mode, and `merge --continue`/`--abort` take **none**, rejecting any positional.
  Fabric resolves no default topology, so the branch is never optional in the modes that take one — per the handle decision and the Q&A's "the caller supplies it".
  A conflicted `merge-in` is concluded with `lyx fabric merge --continue` (or abandoned with `--abort`): the lifecycle is shared, so the `merge` verb's modes serve both, and this asymmetry is stated rather than implied.
  Exit codes mirror git: 0 on a clean or already-up-to-date merge, 1 with a failure envelope carrying the `conflicts` array when conflicts are reported.
- Rationale: nothing new to learn, and it is the outward proof of the one-repo illusion.
  `merge-in` is a distinct verb because git has no such concept — it is a workflow step, not a git operation, and naming it as its own verb is honest about that.
- Rejected: an optional `merge-in` branch defaulting to a fabric-resolved parent (the forbidden topology-resolution shape, and it contradicts the Q&A);
  bare `merge-in` merging the configured upstream à la no-argument `git merge` (collides with the weft side's routinely absent upstream and buys nothing `Finalize` needs);
  separate `merge-continue`/`merge-abort` subcommands (flatter help tree, diverges from git);
  a `--status` flag (in-progress state is reachable through the Go API;
  surfacing it in `lyx fabric status` is an audit follow-up, not a fourth mode on `merge`).

### ship-only-what-finalize-and-hardener-need

- Decision: this task ships the merge surface and nothing else of the wider "all monorepo git verbs" mandate.
  The remaining gap is inventoried under "Audit findings" and spun out.
- Rationale: the standing principle is that Fabric must eventually expose everything needed to operate git on a monorepo, but the roadmap's instruction for *this* task is to audit — find and record — not to implement the whole surface.
  Building it all here would block `Finalize` behind work `Finalize` does not need.
- Rejected: shipping the cheap read-only gaps (`log`, `show`, `branch --list`) alongside merge;
  shipping the full surface.

### finalize-doc-must-be-reworded

- Decision: `manifest/designs/finalize.md`'s sentence "Merge-back forwards only Raddle's regenerated output … via a Fabric commit scoped to `["_lyx"]`" is reworded in this task's commit so it reads as `Finalize`'s own content policy, not as a constraint on Fabric's merge.
  The same section's "The exact commit call this uses is part of the `fabric: merge-conflict primitive` task's scope" is resolved: it is not a scoped commit call at all — `Finalize` deletes or unwires whatever must not cross, then calls an ordinary merge.
- Rationale: as written, the doc describes a Fabric that filters content by path, which the content-blind decision rules out.
  Leaving it would leave the next reader implementing the wrong thing.
- Rejected: leaving the doc and having Fabric honour it.

## Technical context

**Where the two sides live.**
A task pair is a warp worktree on branch `<slug>` plus a weft worktree on branch `<slug>-weft`.
`WeftBranchName` (`internal/fabricengine/branchname.go`) is the sole derivation of the weft name and the only place `-weft` may be composed;
`weftBranchExists` (`weftwiring.go:90-104`) is the existing existence probe the fabric-managed guard reuses.
`internal/fabricengine/add.go:116-172` forks the weft branch from `<parent>-weft` at worktree-creation time;
`weftwiring.go:18,107` states this is done "to preserve the merge-base for future squash-merge-back" — that merge-base is what this task finally consumes.
Branches outlive worktrees by design (`add.go`'s remove-then-re-add guidance), so a target branch is not guaranteed to have a checked-out pair;
in the hub layout it normally does, and `Merge` requires the caller to materialize one when it does not.

**The handle.**
`Fabric` (`fabric.go`) holds `warp`/`weft` as unexported `*gitrepo.Repo` fields plus `warpPath`/`weftPath`;
`Open(l)` (`open.go:12`) is the sole out-of-package constructor, and `lyxcwd.ResolveWorktree(worktreeRoot)` builds a Location for a non-cwd worktree without the cwd gate.
The package's stated rule is that a single-sided operation earns a named `Fabric` method only when it must be callable from outside the package (`warpforward.go` is the existing example);
merge is genuinely cross-repo, so it earns Fabric methods outright.

**Existing two-sided precedents, and which one applies where.**
`commit.go`'s `commitBothSides` is warp-first under the combined lock with three-outcome `*PartialCommitError` reporting;
`pull.go`'s `Fabric.Pull` is weft-first with `*PartialPullError`.
Both report partial failure and name a side — the pattern the merge result must *not* follow.
`checkout.go` is the applicable precedent for the pre-commit window: an all-or-nothing coordinated switch that rolls both sides back on any failure.
For the conclude phase the applicable precedent is neither: idempotent completion via the record, per the commit-phase decision.
`Fabric.Pull`'s `weftHasUpstream` vacuous no-op (`pull.go:129,218`) is the precedent for the guard's no-upstream rule, and `Commit`'s degenerate no-op (no lock, no push) is the precedent for the already-up-to-date fast path.

**Git behaviour the design turns on.**
`git merge --no-commit` cannot stop a fast-forward — git's own documentation: "fast-forward updates do not create a merge commit and therefore there is no way to stop those merges with --no-commit."
`git merge --squash` deliberately records no `MERGE_HEAD`.
`git reset --hard <sha>` clears git's in-progress merge state (`MERGE_HEAD`, the merge index) as well as restoring content — which is what lets one record-driven reset mechanism serve the `MERGE_HEAD`, squash, and fast-forward abort cases uniformly.
Conflicted paths come from `git diff --name-only --diff-filter=U`, repo-root-relative, the same enumeration `mill-merge-in` uses.
`gitrepo.Repo` already has `CurrentSHA`, `ResetHard`, `IsAncestor`, `CurrentBranch`, `WorktreeChangedFiles` and `Fetch` — the merge primitives compose with these rather than duplicating them.

**Where fabric-owned per-pair state already lives.**
The correspondence index sits inside the weft checkout's git directory (`corrIndexPath`, `index.go:70-76`, via `weftGitDir()`), and the lock artifacts sit under `<weftPath>/.weft/` with seeded git-excludes.
The merge-state record follows the correspondence index: git-metadata space, per-pair, outside every worktree, untouched by any reset, invisible to every status surface.

**Where the illusion is already leaky, for the implementer's awareness.**
`PullResult` exposes `WeftPulled`, `WarpFetched`, `WarpAdvanced`, `AnchorWeftSHA`, `ReanchorWeftSHA`;
`ChangeEntry.Side` is literally `"warp"`/`"weft"`.
These are existing surfaces and stay as they are.
The merge result must not follow their example.
The mutations envelope's per-primitive entries (targets included) are invariant-mandated and stay;
the merge design's answer is scenario-symmetry, per the mutation-recording decision.

**Prior art for the second conflict shape.**
`finalize.md` describes two artifact shapes — an ordinary git conflict, and a "discrepancy document" for divergence git cannot express as a conflict.
`PullResult.PatternResidue` (`pull.go:67-81`) is the existing instance of the second shape: post-anchor weft commits touching hand-authored content that a history rewrite made stale.
This task ships the ordinary-git-conflict shape only;
the discrepancy document is inventoried below.

**Millhouse precedent, worth reading before implementing.**
`mill-merge-in/SKILL.md` steps 1-3: the no-op fast path (fetch, prefer `origin/<parent>` when the local ref is behind, exit cheaply when `HEAD..MERGE_REF` is empty — the contract `mill-merge` depends on), the checkpoint branch, the merge, and `--diff-filter=U` conflict enumeration.
`mill-merge/SKILL.md`: parent-worktree resolution from `git worktree list --porcelain` with an in-place fallback mode, and step 5's dirty-target check, fetch-then-`merge --ff-only` sync (with the explicit never-`reset --hard` rationale), `merge --squash`, commit.
This design adopts the guards, the fetch-first freshness rule, and the no-op fast path;
it consciously replaces the checkpoint with verify-before-conclude (see that decision) and consciously narrows worktree resolution to the caller's job (see the handle decision).
Every git command in this design is long-established porcelain — no version floor beyond what the repo already assumes.

## Constraints

From `CONSTRAINTS.md`:

- **Fabric Git Invariant (warp + weft).**
  Every git operation lyx performs on either repo goes through `internal/fabricengine` in Go, in-process — never raw git, never an LLM agent.
  This is the reason the lifecycle must be complete across crashes too: a caller that had to finish or recover a conflicted merge with plain git would violate it.
  The same invariant's carve-out — a human keeps ordinary git in their warp worktree — is why foreign git merge state must be refused rather than adopted.
- **Fabric Vocabulary Invariant.**
  `internal/fabricengine` and `internal/fabriccli` are in the owner set, so warp/weft naming is permitted *inside* them.
  But the merge result type crosses to `Finalize`, which is not in the owner set — so no exported field, error message, or CLI string on the merge surface may name a side.
  The prose-doc split applies too: `finalize.md` describes a consumer, so its wording drops the qualifier.
  Enforced by `internal/lyxcwd/enforcement_test.go` (`TestEnforcement_FabricVocabulary`) for identifiers, literals and comments in production Go under `internal/` and `cmd/`, plus an `internal/**/*.md` and `contracts/stencils/**/*.md` walk and the embedded agent prompt templates;
  test files and docs outside those trees are a review obligation.
- **Fabric Destruction Chokepoint Invariant.**
  `internal/fabricengine/destroy.go` is the only file permitted the destructive primitives, and `warp.ResetHard(`/`weft.ResetHard(` are banned bypass tokens outside it (`cmd/lyx/destructiveguard_test.go`, `TestNoDestructiveBypass_FabricengineProductionSource`).
  The abort/self-abort resets therefore land as gated requests inside `destroy.go` — see the weft-side-gated-reset decision for the shapes and the force-true dirtiness call.
- **Mutation Record Invariant.**
  All four merge verbs are mutating, so `MergeResult` embeds `MutationRecord`, and every merge envelope carries `mutations` (array, never null) and `partial` (bool, never absent), with `partial` derived solely from "error ≠ nil ∧ record non-empty".
  New `Kind` members land in `mutation.go` in the same commit as their recording sites and guard-test entries (`TestMutationRecord_FabricengineProductionSource`).
  `MergeInProgress` is read-only, returns a bare bool with no result type, and therefore stays off the embed table.
- **gitrepo Client Boundary Invariant.**
  `MergeStart`, `MergeConclude`, `ConflictedFiles` and any other new CLI-reaching method join the pinned `gitexec`-using method list in the same commit, or `cmd/lyx/gitrepoboundary_test.go` fails;
  `MergeHeadPresent` may resolve via go-git (a ref/file read) and stay off the list.
- **gitexec Checked-Call Invariant.**
  Every new call site uses checked `runChecked`;
  the per-package pinned raw-site counts (`internal/gitrepo` 3, `internal/fabricengine` 2) do not change.
- **Durable-vs-Ephemeral State Invariant.**
  The merge-state record lives in the weft checkout's git directory beside the correspondence index — git-metadata space, not worktree content, so the `_lyx`/`.lyx` mirrored-subpath rule does not govern it, and no worktree-visible transient is introduced.
- **Test Tier Purity Invariant** and **Hermetic Git Test Environment Invariant.**
  Every new git-spawning test file carries a `//go:build` tag naming `integration`, and both `internal/gitrepo` and `internal/fabricengine` already run under a `gitkit.HermeticGitEnv()` `TestMain` — new test files join those packages' existing compliance rather than adding any.
- **Cwd Resolution Invariant.**
  `internal/lyxcwd` alone resolves cwd.
  The merge surface takes paths from the `Fabric` handle and passes them explicitly to `gitexec.Run` — no `os.Chdir`, no process-wide cwd mutation, and no geometry literals.
  Target-pair handles for `Merge` come from `lyxcwd.ResolveWorktree`, the documented ungated resolver.
- **gitkit Leaf Invariant.**
  `internal/gitkit` stays a leaf;
  merge tests use it only for `HermeticGitEnv`/`MustRun`, never for hub construction.
- **hubforge Fabric-Fixture Invariant.**
  Real hub fixtures come from `internal/hubforge` (`NewHub`, `AddPair`), which drives `fabriccli.CloneAndWire` — never a hand-assembled stand-in.
- **CLI/Cobra Invariant.**
  New verbs go through the module `Command()`/`RunCLI` seam, carry a `Short`, and are covered by help-tree tests.
  `internal/fabriccli/envelope.go` and its contract test cover the output envelope.
- **Never Force-Add Invariant.**
  Nothing in the merge surface runs `git add -f`;
  the conflict-path mapping's no-collision argument leans on this plus the warp-side junction excludes.
- **Documentation Lifecycle.**
  Docs land in the same commit: `internal/gitrepo/doc.go` (scope boundary), `internal/fabricengine/doc.go` (the new cross-repo operation set), and the `finalize.md` reword.
  `manifest/roadmap.md` moves only because this is a planned item completing.
  No new cross-cutting invariant is expected;
  if the implementer finds one, it goes in `CONSTRAINTS.md` in the same commit.

Project constraints from `CLAUDE.md`:

- Markdown uses semantic line breaks, never fixed-column wrapping.
- Worktree isolation: this task's work stays in this worktree;
  no direct push to `main`.

## Testing

Everything here is plain git operating on real repos, and hubforge makes a real wired hub cheap to build, so the bias is strongly toward integration tests over mocks.
All new git-spawning test files carry the `integration` build tag, and both target packages already run under `HermeticGitEnv` `TestMain`s (Test Tier Purity and Hermetic Git Test Environment Invariants).

**`internal/gitrepo` — the single-repo primitives.**
Table-driven tests over a temp repo built with `gitkit.MustRun` under `HermeticGitEnv`, following the existing `pull_test.go`/`reset_test.go`/`push_test.go` shape.
TDD candidates, in order: conflicted-path enumeration on a known conflict;
`MergeStart` outcome classification across all four outcomes — clean-staged (assert staged and uncommitted), conflicted, fast-forwarded (assert HEAD moved, nothing staged, no `MERGE_HEAD` — the documented ff-defeats-`--no-commit` behaviour, pinned as a test), already-up-to-date;
squash start (assert no `MERGE_HEAD`, changes staged and uncommitted);
`MergeConclude` with an explicit message and with the git-prepared fallback;
`ResetHard` clearing in-progress merge state (the abort mechanism's load-bearing property);
`MergeHeadPresent` true after a conflicted merge, false after reset and after conclude.

**`internal/fabricengine` — the two-sided coordination.**
Integration tests over `hubforge.NewHub` + `AddPair`, driving genuine divergent commits.
The scenario matrix that must be covered, each asserting the *pair's* end state rather than either side's:

- Both sides clean → both concluded, correspondence recorded, one clean result, record deleted.
- Warp conflicts, weft clean → nothing committed on either side, conflicts reported as a result (nil error), record present.
- Weft conflicts, warp clean → observable outcome byte-identical to the previous case (result value, error, envelope).
  This is the single most important test in the task.
- Both sides conflict → one flat, sorted list containing paths from both, in the unified namespace.
- One side fast-forwards while the other conflicts → conflicts reported;
  `MergeAbort` returns the fast-forwarded side to its recorded pre-merge SHA (the B1 case, pinned).
- One side already up to date, the other merges → concluded with no empty commit fabricated on the no-op side;
  correspondence pairs the new SHA with the unchanged one.
- Both sides already up to date → `AlreadyUpToDate` result, no lock taken, empty mutation record, no state record written.
- Conflicts resolved in the worktree → `MergeContinue` concludes both sides, records correspondence, deletes the record;
  `MergeContinue` with unresolved conflicts refuses with the fixed guard reason.
- `MergeAbort` after a conflict → both sides at their exact pre-merge SHAs, worktree clean, record deleted, in-progress false.
- Crash recovery: build the mid-merge state (record written, one side conflicted or squash-staged), then drive recovery through public `MergeAbort` on a fresh handle → exact restore;
  and a crashed-after-clean-staging state through `MergeContinue` → concluded.
- Conclude-phase partial failure: force the second side's conclude-commit to fail (e.g. a pre-commit hook or index sabotage in the fixture) → `*ErrMergeIncomplete` with fixed text, record retained;
  re-running `MergeContinue` concludes the remaining side only (idempotency pinned by SHA comparison on the already-landed side).
- Foreign merge state: a plain-git conflicted merge staged directly in the warp checkout (permitted to humans) → `MergeInProgress` false, and all four verbs refuse with `*ErrForeignMergeState`;
  the foreign state is left untouched.
- Squash and non-squash variants of the clean path, asserting the resulting history shape on both sides.
- Conflict-marker content: a weft-only conflict's markers contain no `-weft`-suffixed name — assert the `>>>>>>>` label is the merged SHA, and assert the same marker style on a warp-only conflict so the two are indistinguishable.
- `Merge` with a dirty target → halts before mutating anything on either side;
  dirty-warp-only and dirty-weft-only produce byte-identical `*MergeGuardError` values (guard-report shape pinned).
- `Merge` with a stale target ref → fetches and fast-forwards via `merge --ff-only`;
  with a genuinely diverged target → fails loudly, mutating nothing;
  with a no-upstream side → guard passes vacuously and the result is indistinguishable from the with-upstream clean case.
- `MergeIn` freshness: local source behind its remote-tracking ref → the remote-tracking ref is merged (millhouse's origin-preference rule);
  source existing only remotely → merged;
  source resolvable nowhere → guard failure.
- Source without a fabric counterpart (`<source>-weft` absent) → guard failure with the fixed not-fabric-managed reason, nothing mutated.
- `Merge` that would conflict → self-aborts, target pair unchanged, `*ErrMergeInRequired` returned, no record left, and the conflicting side is not disclosed.
- Unmappable-path conflict: a weft-side conflict manufactured outside the wired name-set → merge aborted on both sides, `*ErrUnmergeableState`, pair restored.
- Path mapping: a conflict in a junctioned path is reported at its unified worktree-root-relative path on a subpath-anchored hub (the `<AnchorRel>/…` case), and the reported file is reachable at that path through the junction.
- `Fabric.Commit` during a recorded merge → typed refusal, nothing mutated;
  and during foreign git merge state with no record → the same typed refusal, not git's raw message.
- Sibling-verb dispositions during a live record: `Pull`, `Checkout`, and `Remove`/`Cleanup` on the pair refuse with the same typed error and mutate nothing;
  `PushWeft` and every read-only verb (`Status`, `Diff`, `List`, `pairs`) succeed unchanged.

**Vocabulary enforcement.**
The existing `TestEnforcement_FabricVocabulary` walk covers production Go and the `internal/**/*.md`/`contracts/stencils/**/*.md`/template surfaces automatically.
Add an explicit assertion that the merge result type's exported surface, every named error's `Error()` output, and the closed guard-reason set contain no side name — the enforcement test permits those tokens inside the owner set, so it will not catch a leak here on its own.

**CLI.**
Help-tree and arity tests alongside the existing `cli_test.go`/`argsarity_test.go` (branch argument required on both verbs), plus an envelope-contract test for the new verbs matching `envelopecontract_integration_test.go`, including the conflict path's exit-1 failure envelope carrying the `conflicts` array and the fixed `mutations`/`partial` keys.

## Audit findings

The roadmap asks this task to audit for other gaps `Finalize`/`Hardener` need from Fabric, not only to build the one primitive.
Recorded here;
only the merge surface is built.

**Gaps that block nothing today, spun out.**
Fabric's current verb surface is `clone`, `add`, `list`, `remove`, `checkout`, `pairs`, `reconcile`, `prune`, `cleanup`, `unwire`, `status`, `commit`, `push`, `pull`, `sync`, `diff`.
Against ordinary monorepo git it is missing `log`, `show`, `branch` (create/list/delete), `tag`, `stash`, `reset` (non-hard), `revert`, `restore`, `rm`/`mv`, `rebase`, `cherry-pick` and `blame`.
None is needed by `Finalize` or `Hardener` today.
Worth a follow-up roadmap item scoped by actual need, not by completing the list for its own sake.

**No post-commit rollback: a two-sided reset-to-SHA verb is missing.**
millhouse's checkpoint lets a failed verify roll back a merge *after* `merge --continue` commits it;
this design replaces that with verify-before-conclude plus `MergeAbort`, which covers the whole uncommitted window but nothing after.
Undoing a concluded merge needs a `Fabric`-level reset to a visible (warp) SHA that resolves the paired weft SHA through the correspondence index and routes both resets through the destruction gate — real design work, spun out.
Until it exists, `Finalize` must run its verification before `MergeContinue`/`Merge` conclude, or accept that a landed merge is final at the Fabric layer.

**Merge-in-progress state is not yet surfaced in `lyx fabric status`.**
`MergeInProgress` ships as Go API only;
folding it into the `status` verb's output is a small follow-up.

**`finalize.md`'s second conflict shape is not built.**
The "discrepancy document" — Fabric precomputing a divergence it cannot express as a git conflict, handing the agent a document to resolve and write back — is out of scope here.
The existing `PullResult.PatternResidue` is the same shape and already exists for the rewrite case.
`fabric-unified-view.md`'s open question "which layer drives pull → conflict-resolve → raddle-regen, and how `PullResult`/`PATTERN` re-alignment is presented to an LLM resolving agent" is the same question and is still open;
it should be answered once, for both, when `Shed`/`loom` exist to consume it.

**`.weft/weft.write.lock` is misnamed.**
It is the combined two-sided write lock (`commit.go`: acquired "whenever anything lands, even warp-only"), but its name says weft.
This task reuses it and does not rename it.
A rename touches the lock path on disk and needs its own task.

**`cleanup.go`'s `raddleFoldedBack` is content-aware logic inside a content-blind module.**
`internal/fabricengine/cleanup.go:93-95` is a stub returning `false` unconditionally, gating weft-branch deletion on whether `_lyx/raddle/` has been squash-merged back.
That question is about content, which by this task's governing decision is not Fabric's to answer.
Left as-is;
flagged for whoever builds `Finalize`, who will have to decide whether the gate moves out of Fabric or the stub stays permanent.

**Squash-merge leaves no ancestry link.**
After a squash merge, git cannot answer "was this branch merged?" — there is no merge commit linking them.
Anything that needs that answer (`cleanup`'s gate above, archive tagging, branch teardown) needs a source outside git.
millhouse solves it with an `archive/<slug>` tag plus its own status file.
Not Fabric's problem, but it is a direct consequence of a decision made here, so it is recorded here.

## Q&A log

- **Q:** Where does the merge primitive live — `gitrepo`, `fabricengine`, or split? **A:** Split. `gitrepo` is our wrapper over git operations, so the git-level primitive belongs there; `fabricengine` coordinates the two repos on top. All git goes through a package, never raw.
- **Q:** Where does the target branch come from? **A:** The caller supplies it. Fabric's functions must look as much as possible like completely ordinary git operations on a monorepo — that is 100% the concept of Fabric.
- **Q:** Does the weft side get a real branch merge, or a scoped commit forwarding only raddle output? **A:** A real merge. Merging `<slug>-weft` onto `<parent>-weft` *is* a merge.
- **Q:** Should the merge exclude `_lyx` or other task-local content? **A:** No. Fabric has no idea what is inside its repos. It knows `_lyx` exists only because it wired that junction from a name list. Deleting or unwiring directories before a merge is Finalize's job, totally outside Fabric's domain.
- **Q:** Squash, merge commit, or an option? **A:** An option, and squash is nearly always what we use — otherwise history becomes horrible clutter.
- **Q:** What happens when one side merges cleanly and the other conflicts? **A:** Fabric says "conflicts, fix them before proceeding". Nothing more. Fabric exposes exactly one repo. What is in warp and weft must never be visible from the outside.
- **Q:** Lock only the weft side? **A:** No — a merge is a joint operation, so it must lock both. (Resolved by reusing `.weft/weft.write.lock`, which despite its name is already the combined two-sided lock.)
- **Q:** How far does the "all monorepo git functions" mandate reach in this task? **A:** Only what our use needs.
- **Q:** Which worktree does the merge run in? **A:** Same as millhouse: merge parent into the slug first and resolve all conflicts there, so the agent never disturbs the parent; then squash-merge slug into parent, conflict-free. See `mill-merge`, which runs `mill-merge-in` first.
- **Q:** Should `Merge` carry the conflict lifecycle if it should never conflict? **A:** Yes, but a conflict there redirects to `MergeIn`. Resolving conflicts in the parent's worktree makes no sense: it would require checking the parent out in the current worktree, which git forbids when the parent is already checked out in another worktree of the same repo.
- **Q:** Do the dirty-target and fast-forward guards belong inside `Merge`? **A:** Yes. If there are conflicts between parent and slug, a `MergeIn` is required first.
- **Q:** Why must the target branch be checked out anywhere — could `Merge` compute the result in the object database instead (`merge-tree`/`commit-tree`/`update-ref`), needing no target worktree? **A:** We do not need that. The concept is unfamiliar and unwanted here, so `Merge` stays an ordinary worktree merge against a checked-out target.
