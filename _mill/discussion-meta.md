# Discussion metadata: fabric: merge-conflict primitive

Record kept out of `_mill/discussion.md` so that file carries only what mill-plan needs to write the plan.
Nothing here is normative — the decisions themselves live in `discussion.md`.

## Rejected alternatives

### fabric-is-a-content-blind-git-operator

- Rejected: scoping the weft-side merge's content to `_lyx/raddle/` per the current wording of `finalize.md`;
  filtering task-local artifacts out of the merge;
  a merge that knows `_lyx` is special.

### git-primitive-in-gitrepo-coordination-in-fabricengine

- Rejected: `fabricengine`-only, calling `gitexec` directly for merge plumbing.

### two-verbs-mergein-then-merge

- Rejected: one `Merge` verb used in both directions.
  The two calls have genuinely different guards, different failure modes, and different worktrees;
  collapsing them hides that.

### merge-runs-on-a-handle-opened-at-the-target

- Rejected: `Merge(source, target)` with Fabric resolving and checking out the target itself — that is the forbidden "check the target out here" shape in disguise;
  a worktree-free merge computed in the object database (`merge-tree`/`commit-tree`/`update-ref`) — it removes the target-worktree requirement but refuses whenever the target *is* checked out, because advancing the branch ref would desynchronise that worktree's index and files, and that is the normal case here;
  Fabric creating and tearing down a temporary target pair (worktree lifecycle is `add`/`cleanup`'s domain).

### default-git-merge-semantics-with-a-squash-option

- Rejected: hard-coded squash (contradicts "nearly always", and contradicts the ordinary-git surface);
  per-side strategy (forces the caller to know there are two sides);
  a fabric-composed default message (invents a format git already provides).

### a-recorded-merge-not-a-derived-one

- Rejected: deriving `MergeInProgress` from git state (all four defects above);
  a worktree-visible state file under `.lyx` — the git-dir placement follows `corrIndexPath`'s precedent, sits in git-metadata space the Durable-vs-Ephemeral State Invariant's `_lyx`/`.lyx` mirroring rule does not govern, and can never itself conflict, be reset away, or appear in any status output.

### no-new-commit-until-both-sides-are-clean

- Rejected: `--no-ff` on both sides to force stoppable merges (changes history shape, contradicting plain-git semantics);
  first-side-commit-then-second with rollback (leaves a real rollback window and cannot be described without naming a side);
  report-not-rollback in the style of `*PartialCommitError`/`*PartialPullError` (necessarily names a side, which is exactly what is forbidden here).

### already-up-to-date-is-a-result-not-a-fabrication

- Rejected: forcing an empty commit on the no-op side;
  treating one-side-no-op as an error (it is the routine case whenever only one repo changed).

### commit-phase-concludes-per-side-and-never-rolls-back

- Rejected: `checkout.go`-style rollback of a landed merge commit (destroys resolution work);
  a partial-report error naming the landed side (forbidden);
  pretending the window does not exist (the earlier draft's position).

### conflicts-are-reported-as-unified-worktree-relative-paths

- Rejected: raw repo-relative paths from each side (ambiguous, leaks the two-repo reality);
  absolute filesystem paths (unambiguous but not what `git merge` hands you);
  a synthetic prefix for unmapped weft paths (a path meaningful in only one repo is itself a leak);
  paths relative to `AnchorPath()` (git's own enumeration is repo-root-relative, and on a subpath-anchored hub the wired content sits under `<AnchorRel>/…` in *both* namespaces, so root-relative is the one choice where the identity mapping holds verbatim).

### merges-name-a-sha-never-a-branch

- Rejected: SHA-resolving the weft side only (leaks through marker style instead of marker text);
  rewriting markers after the fact (Fabric would be editing content, which the content-blind decision forbids);
  `merge.conflictStyle`/`-X` tuning (changes marker *format*, never the label);
  renaming the weft branch scheme (the suffix is load-bearing geometry — `WeftBranchName` is its sole declarer — and the leak is the label, not the name).

### combined-lock-around-mutating-steps-only

- Rejected: a new warp-side lock (duplicates what exists);
  holding the lock for the entire merge span (`raddle.md` demands that span, but that is `Finalize`'s campaign-level lock, not Fabric's);
  no lock.

### safety-guards-are-aggregated-and-side-free

- Rejected: leaving the guards to the caller (every caller would reimplement them, and the failure modes are silent);
  first-failure reporting (reveals evaluation order and therefore arity);
  passing git's own dirty-worktree refusal through (raw, unowned, potentially side-revealing);
  a mandatory fetch that fails the merge when offline (the fetch is a freshness aid, not a correctness gate — `--ff-only` against the last-known upstream still catches genuine divergence).

### merge-conflicts-are-redirected-to-mergein

- Rejected: leaving `Merge`'s conflicts in place for the caller to resolve — unresolvable where they land, by policy;
  omitting the lifecycle from `Merge` entirely — `Merge` must still detect and clean up after a conflict, so it needs the machinery either way.

### lifecycle-quartet-on-both-verbs

- Rejected: a quartet scoped to `MergeIn` only (leaves `Merge`'s crash windows outside the Fabric Git Invariant);
  git-derived in-progress state (see the `a-recorded-merge-not-a-derived-one` decision).

### weft-source-is-derived-and-must-exist

- Rejected: warp-only merge when the weft counterpart is missing;
  fork-on-demand per `add.go`'s adopt-vs-create precedent (that precedent is for creating a *pair*, where fabricating the weft fork is the caller's explicit intent).

### verify-before-conclude-not-post-commit-rollback

- Rejected: shipping a two-sided reset verb here (scope, and it deserves its own destruction-gate design);
  silently dropping the millhouse affordance without recording it (the earlier draft's position — the gap is now in Scope Out and Audit findings).

### public-surface-shapes

- Rejected: conflicts-as-typed-error (the merge did what a merge does — the envelope's `partial` rule and the plain-git surface both read better with conflicts as data);
  per-verb result types (four shapes to keep side-free instead of one).

### mutation-recording-stays-scenario-symmetric

- Rejected: suppressing or coarsening merge mutations to hide targets (violates the invariant's provably-total recording, and the destruction recorder is threaded into `destroy.go` precisely so it cannot be bypassed).

### weft-side-gated-reset-in-destroy-dot-go

- Rejected: `dirtinessNA` (the dirtiness is real and known, not inapplicable — declaring it NA would be false);
  routing around the gate (banned tokens, and the recorder threading exists precisely to prevent it).

### correspondence-is-recorded-for-the-pair-a-merge-lands-in

- Rejected: leaving it to the caller — the caller is `Finalize`, which does not know weft exists and therefore cannot know correspondence exists.

### cli-mirrors-git

- Rejected: an optional `merge-in` branch defaulting to a fabric-resolved parent (the forbidden topology-resolution shape, and it contradicts the Q&A);
  bare `merge-in` merging the configured upstream à la no-argument `git merge` (collides with the weft side's routinely absent upstream and buys nothing `Finalize` needs);
  separate `merge-continue`/`merge-abort` subcommands (flatter help tree, diverges from git);
  a `--status` flag (in-progress state is reachable through the Go API;
  surfacing it in `lyx fabric status` is an audit follow-up, not a fourth mode on `merge`).

### ship-only-what-finalize-and-hardener-need

- Rejected: shipping the cheap read-only gaps (`log`, `show`, `branch --list`) alongside merge;
  shipping the full surface.

### finalize-doc-must-be-reworded

- Rejected: leaving the doc and having Fabric honour it.

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
