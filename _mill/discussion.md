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

**Why now:** the `loom` phase-machine scaffolding task is next on the roadmap and carries `Finalize` in its producer list, explicitly blocked on this one.

## Scope

**In:**

- Git-level merge primitives on `gitrepo.Repo` — the single-repo half, in lyx's own git wrapper.
- `fabricengine.MergeIn` — merge a source branch into the *current* pair, in the current worktree.
  This is where conflicts are surfaced and resolved.
- `fabricengine.Merge` — merge the task pair into a *target* pair (squash-capable), expected conflict-free.
- `fabricengine.MergeContinue`, `MergeAbort`, `MergeInProgress` — the conflict lifecycle, attached uniformly to both verbs.
- Unified, side-free conflict reporting: one flat list of paths relative to the single visible worktree root.
- Two-sided indivisibility: neither side commits until both sides are conflict-free.
- Recording correspondence (`RecordCorrespondence`) for the pair a merge lands in, so the index survives a merge.
- CLI verbs `lyx fabric merge-in` and `lyx fabric merge`, with envelope and help-tree tests per the CLI/Cobra Invariant.
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
- The remaining monorepo verb surface (`log`, `show`, `branch`, `tag`, `stash`, `revert`, `restore`, `rm`/`mv`, `rebase`, `cherry-pick`, `blame`) — inventoried under "Audit findings", spun out, not built here.
- Renaming `.weft/weft.write.lock`, and changing `PullResult`/`ChangeEntry`'s existing side-labelled fields.
  Both are recorded as findings, neither is touched.

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

- Decision: the git-level merge primitives (merge, continue, abort, in-progress detection, conflicted-path enumeration) land on `gitrepo.Repo`, operating on one repo.
  `fabricengine` owns the two-sided coordination, the guards, the lock, and the unified result shaping.
  No production code calls git except through `internal/gitexec`, which both packages already do.
- Rationale: `gitrepo` is lyx's own wrapper over git operations, so a git operation belongs there.
  `fabricengine` already calls `gitexec.Run` directly for topology work (`add.go`, `checkout.go`, `destroy.go`, `index.go`), so either placement would have compiled — but putting the raw merge plumbing in `fabricengine` would leave `gitrepo` a wrapper that cannot do the one git operation this task is about.
- Rejected: `fabricengine`-only, calling `gitexec` directly for merge plumbing.
- Consequence: `internal/gitrepo/doc.go`'s "Scope boundaries" paragraph currently reads "Rebase, interactive staging, cherry-pick, **conflict resolution**, and general-purpose branch/checkout management are explicitly not supported".
  That sentence must be amended in this task's commit to admit the merge surface, with the same honesty the rest of that paragraph shows — merge is admitted, rebase and cherry-pick stay out.

### two-verbs-mergein-then-merge

- Decision: two verbs, mirroring the millhouse workflow.
  `MergeIn(<source>)` merges the parent branch into the current pair, in the current worktree, and is where conflicts are surfaced and resolved.
  `Merge(<source>, opts)` then merges the task pair into the target pair, conflict-free by construction.
- Rationale: conflict resolution cannot happen in the parent's worktree.
  Resolving there would mean checking the parent branch out in the current worktree, which git refuses when that branch is already checked out in another worktree of the same repo — and the parent branch always is, in a hub layout.
  `mill-merge-in` and `mill-merge` in millhouse solved exactly this and are the working precedent.
- Rejected: one `Merge` verb used in both directions.
  The two calls have genuinely different guards, different failure modes, and different worktrees;
  collapsing them hides that.

### merge-runs-on-a-handle-opened-at-the-target

- Decision: `Merge` is called on a `Fabric` handle opened at the **target** pair's worktree, taking the source branch as its single branch argument — `git merge <branch>` transposed.
  Fabric resolves no topology of its own to find the target.
- Rationale: matches `git merge`'s own call shape, keeps Fabric from inventing a parent-resolution rule, and keeps the caller in control of which worktree gets mutated.
- Rejected: `Merge(source, target)` with Fabric resolving and checking out the target itself — that is the forbidden "check the parent out here" shape in disguise.
- Note: the operator's cross-worktree discipline (never `cd` into another worktree, always `git -C <path>`) is honoured for free, because a `Fabric` handle is already path-anchored — `f.warpPath`/`f.weftPath` — and every `gitexec.Run` call takes its directory explicitly.

### default-git-merge-semantics-with-a-squash-option

- Decision: `MergeOptions{Squash bool, Message string}`.
  Semantics are plain `git merge`: merge automatically when possible, report conflicts and require manual resolution when not.
  `Squash` is applied identically to both sides.
  `MergeIn` never squashes — it is a real merge, preserving ancestry, exactly as `mill-merge-in` does.
  The CLI's default is git's default (no squash);
  `Finalize` passes `Squash: true`.
- Rationale: squash-merge is the normal case for task branches — without it, parent history accumulates a merge commit plus every task commit, which the operator described as unacceptable clutter.
  But Fabric is a general git operator, not Finalize's helper, so the option is the caller's.
- Rejected: hard-coded squash (contradicts "nearly always", and contradicts the ordinary-git surface);
  per-side strategy (forces the caller to know there are two sides).

### neither-side-commits-until-both-are-clean

- Decision: both sides are merged with `--no-commit` (implied by `--squash` in the squash case).
  Both sides are attempted even when the first one conflicts, so the returned conflict list is complete.
  Only when both sides are conflict-free does Fabric commit both.
- Rationale: this is what makes the one-repo illusion hold.
  If warp were committed and weft then conflicted, the caller would hold a half-finished merge, and there would be no way to describe that state without naming a side.
  Attempting only the first side would be worse: the caller resolves warp conflicts, continues, and only then discovers weft conflicts.
  One call, one outcome — merged, or conflicts.
- Rejected: warp-first-commit-then-weft with rollback (`Commit`'s ordering plus rollback) — leaves a real rollback window;
  report-not-rollback, the Shared Decision `Commit` and `Pull` follow — it necessarily names a side, which is exactly what is forbidden here.
- Precedent: `checkout.go`'s all-or-nothing coordinated switch, not `commit.go`'s partial-report.

### conflicts-are-reported-as-unified-worktree-relative-paths

- Decision: the conflict result is a flat list of paths relative to the single visible worktree root, with no side label anywhere in the type.
  Weft-relative paths are mapped into that namespace through the junction geometry Fabric already owns (`junctionnames.go`, `WiredNames`).
- Rationale: the two repos have separate path namespaces and can hold the same relative path, so raw repo-relative paths would be ambiguous as well as side-revealing.
  The mapping is pure geometry — Fabric's own domain — not content knowledge.
  A caller resolving conflicts sees ordinary conflicted files at ordinary paths in one tree, with ordinary conflict markers, because the weft worktree is junctioned into the warp worktree and the files are literally there.
- Rejected: raw repo-relative paths from each side (ambiguous, leaks the two-repo reality);
  absolute filesystem paths (unambiguous but not what `git merge` hands you).

### combined-lock-around-mutating-steps-only

- Decision: reuse the existing combined write lock for the whole two-sided merge, taken around `Merge`, `MergeIn`, `MergeContinue` and `MergeAbort` — never held across the conflict-resolution window.
- Rationale: the operator's point stands — a merge is a joint operation and must lock both sides, not just one.
  It already does: `.weft/weft.write.lock` is described in `commit.go` as "the combined write lock", acquired "whenever anything lands, **even warp-only**".
  Its name says weft;
  its semantics cover both sides.
  So the correct answer is to reuse it, not to invent a second lock.
  Holding it across conflict resolution would block every other worktree for as long as an LLM takes to fix a conflict, which can be unbounded.
- Rejected: a new warp-side lock (duplicates what exists);
  holding the lock for the entire merge span (`raddle.md` demands that span, but that is `Finalize`'s campaign-level lock, not Fabric's);
  no lock.
- Consequence: the lock's misleading name is recorded as an audit finding, not renamed here.

### safety-guards-live-inside-merge

- Decision: `Merge` verifies its preconditions on both sides before touching anything — the target worktree is clean, and the target branch is fast-forwarded to its upstream via `merge --ff-only`, never `reset --hard`.
  A guard failure halts before any mutation.
- Rationale: these are git preconditions, not content policy, so they are Fabric's.
  millhouse learned both the hard way: a dirty parent worktree silently absorbs a partially-applied squash, and a stale parent ref makes the subsequent push fail as non-fast-forward.
  `reset --hard` is explicitly not the fast-forward mechanism, because it silently discards local-only commits on the target;
  `merge --ff-only` fails loudly instead.
- Rejected: leaving the guards to the caller — every caller would reimplement them, and the failure modes are silent.

### merge-conflicts-are-redirected-to-mergein

- Decision: `Merge` should not conflict, because `MergeIn` is what prevents it.
  If it does, `Merge` aborts both sides itself, restoring the target pair exactly, and returns a typed error telling the caller to run `MergeIn` first.
  It never leaves a conflicted state behind in the target worktree.
- Rationale: conflicts cannot be resolved in the target worktree at all (see two-verbs decision), so leaving conflicts there would strand the caller.
  Redirecting to `MergeIn` is both the only workable recovery and the millhouse workflow.
- Rejected: leaving `Merge`'s conflicts in place for the caller to resolve — unresolvable where they land;
  omitting the lifecycle from `Merge` entirely — `Merge` must still detect and clean up after a conflict, so it needs the machinery either way.

### lifecycle-quartet-on-both-verbs

- Decision: `MergeContinue`, `MergeAbort` and `MergeInProgress` apply uniformly to both verbs.
  `MergeInProgress` is derived from git state (a merge recorded on either side), not from a Fabric-owned state file.
  `MergeAbort` restores both sides;
  in the squash case, where git records no merge to abort, it restores from the pre-merge SHAs Fabric captured before starting.
- Rationale: uniformity keeps the surface git-shaped, and deriving in-progress state from git rather than a sidecar file means an operator using plain git in the worktree cannot desynchronise Fabric's view.
- Rejected: a persistent merge-state file under `.lyx` — needed only for the squash-conflict case, which cannot persist because `Merge` self-aborts within the same call.

### correspondence-is-recorded-for-the-pair-a-merge-lands-in

- Decision: after a merge commits on both sides, Fabric records the new warp↔weft correspondence via `RecordCorrespondence`, in whichever pair the merge landed in.
- Rationale: every existing write path maintains the correspondence index;
  a merge that skipped it would leave the target pair's index unable to resolve its own new HEAD, breaking `Diff`, revert-target resolution, and `Pull`'s re-anchor logic.
- Rejected: leaving it to the caller — the caller is `Finalize`, which does not know weft exists and therefore cannot know correspondence exists.

### cli-mirrors-git

- Decision: `lyx fabric merge <branch> [--squash] [-m <message>]`, `lyx fabric merge --continue`, `lyx fabric merge --abort`, and `lyx fabric merge-in [<branch>]`.
  Flags are git's own.
- Rationale: nothing new to learn, and it is the outward proof of the one-repo illusion.
  `merge-in` is a distinct verb because git has no such concept — it is a workflow step, not a git operation, and naming it as its own verb is honest about that.
- Rejected: separate `merge-continue`/`merge-abort` subcommands (flatter help tree, diverges from git);
  a `--status` flag (in-progress state is reachable through the Go API and belongs in `lyx fabric status`, not a fourth mode on `merge`).

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
`WeftBranchName` (`internal/fabricengine/branchname.go`) is the sole derivation of the weft name and the only place `-weft` may be composed.
`internal/fabricengine/add.go:116-169` forks the weft branch from `<parent>-weft` at worktree-creation time;
`weftwiring.go:18,107` states this is done "to preserve the merge-base for future squash-merge-back" — that merge-base is what this task finally consumes.

**The handle.**
`Fabric` (`fabric.go`) holds `warp`/`weft` as unexported `*gitrepo.Repo` fields plus `warpPath`/`weftPath`.
The package's stated rule is that a single-sided operation earns a named `Fabric` method only when it must be callable from outside the package (`warpforward.go` is the existing example);
merge is genuinely cross-repo, so it earns Fabric methods outright.

**Existing two-sided precedents, and which one applies.**
`commit.go`'s `commitBothSides` is warp-first under the combined lock with three-outcome `*PartialCommitError` reporting.
`pull.go`'s `Fabric.Pull` is weft-first with `*PartialPullError`.
Both report partial failure and name a side — the pattern this task must *not* follow.
`checkout.go` is the applicable precedent: an all-or-nothing coordinated switch that rolls both sides back on any failure so the pair is never left half-switched.

**Git plumbing likely needed.**
Conflicted paths come from `git diff --name-only --diff-filter=U`, the same enumeration `mill-merge-in` uses.
A merge in progress is detectable from `MERGE_HEAD`;
note that `git merge --squash` deliberately records no `MERGE_HEAD`, so squash-case recovery is `reset --hard <pre-merge SHA>` against SHAs captured before the merge started, not `git merge --abort`.
`gitrepo.Repo` already has `CurrentSHA`, `ResetHard`, `IsAncestor`, `CurrentBranch`, `WorktreeChangedFiles` and `Fetch` — the merge primitives compose with these rather than duplicating them.

**Where the illusion is already leaky, for the implementer's awareness.**
`PullResult` exposes `WeftPulled`, `WarpFetched`, `WarpAdvanced`, `AnchorWeftSHA`, `ReanchorWeftSHA`;
`ChangeEntry.Side` is literally `"warp"`/`"weft"`.
These are existing surfaces and stay as they are.
The merge result must not follow their example.

**Prior art for the second conflict shape.**
`finalize.md` describes two artifact shapes — an ordinary git conflict, and a "discrepancy document" for divergence git cannot express as a conflict.
`PullResult.PatternResidue` (`pull.go:67-81`) is the existing instance of the second shape: post-anchor weft commits touching hand-authored content that a history rewrite made stale.
This task ships the ordinary-git-conflict shape only;
the discrepancy document is inventoried below.

**Millhouse precedent, worth reading before implementing.**
`mill-merge-in/SKILL.md` steps 1-3 (no-op fast path, rollback checkpoint, merge, conflict enumeration) and `mill-merge/SKILL.md` step 5 (dirty-target check, `merge --ff-only` sync, `merge --squash`, commit) encode the exact sequence and the exact guards, including why `reset --hard` is never the sync mechanism.

## Constraints

From `CONSTRAINTS.md`:

- **Fabric Git Invariant (warp + weft).**
  Every git operation lyx performs on either repo goes through `internal/fabricengine` in Go, in-process — never raw git, never an LLM agent.
  This is the reason `Merge` must expose a complete lifecycle: a caller that had to finish a conflicted merge with plain git would violate it.
- **Fabric Vocabulary Invariant.**
  `internal/fabricengine` and `internal/fabriccli` are in the owner set, so warp/weft naming is permitted *inside* them.
  But the merge result type crosses to `Finalize`, which is not in the owner set — so no exported field, error message, or CLI string on the merge surface may name a side.
  The prose-doc split applies too: `finalize.md` describes a consumer, so its wording drops the qualifier.
  Enforced by `internal/lyxcwd/enforcement_test.go` (`TestEnforcement_FabricVocabulary`) for identifiers, literals and comments in production Go plus an `internal/**/*.md` walk.
- **Cwd Resolution Invariant.**
  `internal/lyxcwd` alone resolves cwd.
  The merge surface takes paths from the `Fabric` handle and passes them explicitly to `gitexec.Run` — no `os.Chdir`, no process-wide cwd mutation, and no geometry literals.
- **gitkit Leaf Invariant.**
  `internal/gitkit` stays a leaf;
  merge tests use it only for `HermeticGitEnv`/`MustRun`, never for hub construction.
- **hubforge Fabric-Fixture Invariant.**
  Real hub fixtures come from `internal/hubforge` (`NewHub`, `AddPair`), which drives `fabriccli.CloneAndWire` — never a hand-assembled stand-in.
- **CLI/Cobra Invariant.**
  New verbs go through the module `Command()`/`RunCLI` seam, carry a `Short`, and are covered by help-tree tests.
  `internal/fabriccli/envelope.go` and its contract test cover the output envelope.
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

**`internal/gitrepo` — the single-repo primitives.**
Table-driven tests over a temp repo built with `gitkit.MustRun` under `HermeticGitEnv`, following the existing `pull_test.go`/`reset_test.go`/`push_test.go` shape.
TDD candidates, in order: conflicted-path enumeration on a known conflict;
clean merge;
squash merge (assert no `MERGE_HEAD` is recorded, and that changes are staged and uncommitted);
abort restoring the exact pre-merge SHA in both the `MERGE_HEAD` and squash cases;
in-progress detection true after a conflicted merge and false after abort and after continue.

**`internal/fabricengine` — the two-sided coordination.**
Integration tests over `hubforge.NewHub` + `AddPair`, driving genuine divergent commits.
The scenario matrix that must be covered, each asserting the *pair's* end state rather than either side's:

- Both sides clean → both committed, correspondence recorded, one clean result.
- Warp conflicts, weft clean → nothing committed on either side, conflicts reported, both sides restorable by `MergeAbort`.
- Weft conflicts, warp clean → identical observable outcome to the previous case.
  This is the single most important test in the task: the two cases must be indistinguishable from the result value.
- Both sides conflict → one flat list containing paths from both, in the unified namespace.
- Conflicts resolved in the worktree → `MergeContinue` commits both sides and records correspondence.
- `MergeAbort` after a conflict → both sides back at their exact pre-merge SHAs, worktree clean.
- Squash and non-squash variants of the clean path, asserting the resulting history shape on both sides.
- `Merge` with a dirty target → halts before mutating anything on either side.
- `Merge` with a stale target ref → fast-forwards via `--ff-only`;
  and with a genuinely diverged target → fails loudly, mutating nothing.
- `Merge` that would conflict → self-aborts, target pair unchanged, typed redirect-to-`MergeIn` error.
- Path mapping: a conflict in a junctioned path is reported at its unified worktree-relative path, and a same-named path existing on both sides does not collide.

**Vocabulary enforcement.**
The existing `TestEnforcement_FabricVocabulary` walk covers production Go and `internal/**/*.md` automatically.
Add an explicit assertion that the merge result type's exported surface and its error strings contain no side name — the enforcement test permits those tokens inside the owner set, so it will not catch a leak here on its own.

**CLI.**
Help-tree and arity tests alongside the existing `cli_test.go`/`argsarity_test.go`, plus an envelope-contract test for the new verbs matching `envelopecontract_integration_test.go`.

## Audit findings

The roadmap asks this task to audit for other gaps `Finalize`/`Hardener` need from Fabric, not only to build the one primitive.
Recorded here;
only the merge surface is built.

**Gaps that block nothing today, spun out.**
Fabric's current verb surface is `clone`, `add`, `list`, `remove`, `checkout`, `pairs`, `reconcile`, `prune`, `cleanup`, `unwire`, `status`, `commit`, `push`, `pull`, `sync`, `diff`.
Against ordinary monorepo git it is missing `log`, `show`, `branch` (create/list/delete), `tag`, `stash`, `reset` (non-hard), `revert`, `restore`, `rm`/`mv`, `rebase`, `cherry-pick` and `blame`.
None is needed by `Finalize` or `Hardener` today.
Worth a follow-up roadmap item scoped by actual need, not by completing the list for its own sake.

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
`internal/fabricengine/cleanup.go:92` is a stub returning `false` unconditionally, gating weft-branch deletion on whether `_lyx/raddle/` has been squash-merged back.
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
