# Review: `_mill/discussion.md` — fabric: merge-conflict primitive

Reviewer: mill-reviewer (Fable 5).
Scope per brief: factual accuracy, internal coherence, governing-principle violations, gaps blocking autonomous planning, the MergeIn/Merge split vs the millhouse precedent.
Findings are severity-ranked BLOCKING then NIT, each with citations.
Every code claim below was verified against the worktree at `/home/knatte/Code/loomyard/wts/fabric-merge-conflict-primitive`.

## Verdict

The document's factual claims about the existing codebase are almost all accurate (verified list at the end).
The design itself is not implementable as written: the two-sided `--no-commit` lifecycle it specifies is defeated by git's own fast-forward behaviour, the git-state-only `MergeInProgress` is incoherent against states git can actually produce, the unified conflict-path mapping has no defined rule for the paths that need one most, and the Constraints section omits three machine-enforced invariants that will fail the guard tests of any plan derived from the document alone.

---

## BLOCKING

### B1. `--no-commit` cannot stop a fast-forward, so "neither side commits until both are clean" is unachievable as specified

`_mill/discussion.md:117-127` decides both sides are merged with `--no-commit`, and only when both are conflict-free does Fabric commit both.
Git's documented behaviour: `git merge --no-commit` does not and cannot stop a fast-forward update — an ff merge moves HEAD immediately, creates no merge commit, stages nothing, and records no `MERGE_HEAD`.
The ff case is routine here, not exotic: `MergeIn` never squashes (`discussion.md:108`), and a task pair whose weft branch has no task-side weft commits since its fork from `<parent>-weft` (`internal/fabricengine/add.go:167-172`) fast-forwards on the weft side the moment `<parent>-weft` has advanced.
Consequences, all unhandled:

- One side's HEAD moves while the other conflicts — exactly the half-finished state `discussion.md:121-124` says must never exist.
- `MergeAbort` (`discussion.md:175-178`) has nothing to `git merge --abort` on the ff side, and no captured SHA to reset to — the pre-merge-SHA capture is specified for the squash case only, and the state file that could persist SHAs is rejected at `discussion.md:180`.
- `MergeInProgress` derived from `MERGE_HEAD` (`discussion.md:176,237`) reports false on the ff side while that side has in fact already absorbed the merge.

The document flags only `--squash` as the no-`MERGE_HEAD` case (`discussion.md:236-238`) and misses ff entirely.
Fixing this forces a real design choice the document does not make: `--no-ff` (changes history shape, contradicting "Semantics are plain `git merge`" at `discussion.md:104-105`), or pre-captured SHAs plus reset-based abort on every path (which reopens the rejected persistent-state question), or accepting ff asymmetry (which breaks the one-repo illusion).
An implementer cannot proceed without inventing this decision.

### B2. The already-up-to-date-on-one-side lifecycle is undefined

When one side merges and the other reports "Already up to date" (no `MERGE_HEAD`, nothing staged, nothing to commit), the specified lifecycle breaks:

- `MergeContinue` "commits both sides" (`discussion.md:311`) — the up-to-date side has nothing to commit; forcing an empty commit fabricates history, skipping requires state discrimination the document never defines.
- `RecordCorrespondence` (`discussion.md:182-187`, `internal/fabricengine/index.go:102`) would pair a new SHA on one side with an unchanged SHA on the other — never stated as legal or illegal.
- The test matrix (`discussion.md:302-318`) has no already-up-to-date row at all, and no both-sides-no-op row, even though `mill-merge-in`'s step-1 no-op fast path is cited as precedent at `discussion.md:253` (`mill-merge-in/SKILL.md:33-48`).

What `MergeIn` returns when there is nothing to merge anywhere — the fast path `mill-merge` depends on (`mill-merge-in/SKILL.md:216`) — is unspecified.

### B3. Git-state-only `MergeInProgress` cannot distinguish a Fabric merge from a human's plain-git merge, and the design adopts the human's merge

`discussion.md:176-179` derives `MergeInProgress` from "a merge recorded on either side", claiming this prevents desynchronisation.
`CONSTRAINTS.md:283-284` explicitly permits a human to run ordinary git in their warp worktree.
A human `git merge X` that conflicts in the warp worktree puts `MERGE_HEAD` on one side only.
Then: `MergeInProgress` reports true for a merge Fabric never started; `MergeAbort` "restores both sides" but the weft side has no merge to abort and no pre-merge SHA exists anywhere; `MergeContinue` would commit the human's half-merge on one side, invent something on the other, and record a correspondence for it.
The claimed benefit inverts into Fabric adopting foreign single-sided merge state.
The document rejects the sidecar marker (`discussion.md:180`) without ever addressing this case; the brief's exact question — one side has `MERGE_HEAD`, the other does not — has no answer anywhere in the document.

### B4. The squash crash window strands a state Fabric can neither see nor fix

`Merge` self-aborts a squash conflict "within the same call" (`discussion.md:165-171,180`).
If the process dies after `git merge --squash` conflicts and before the self-abort completes, the target pair holds a conflicted index with no `MERGE_HEAD` on the squash side: `MergeInProgress` (git-derived) reports false, and the pre-merge SHAs needed for restore died with the process.
Recovery then requires raw git — violating the Fabric Git Invariant the document itself cites as the reason the lifecycle must be complete (`discussion.md:259-261`).
The no-state-file decision rests on the unstated assumption that self-abort always completes.

### B5. Commit-phase partial failure is unspecified on the only genuinely dangerous path

Two `git commit` invocations on two repos cannot be atomic.
If warp's commit lands and weft's fails (index corruption, disk, hooks — the lock excludes only concurrent Fabric writers), the caller holds precisely the state `discussion.md:121-124` declares indescribable.
`checkout.go` is named as the rollback precedent (`discussion.md:127,232`), but rollback of a landed merge commit means resetting away a merge the user just resolved — a materially different decision than rolling back a branch switch, and the document never makes it.
Same hole in `MergeContinue` (`discussion.md:311`).

### B6. The unified path mapping has no rule for weft paths outside the wired name-set, and the no-collision claim has no mechanism

`discussion.md:129-137` maps weft-relative conflict paths through junction geometry (`junctionnames.go` `WiredNames`, verified at `internal/fabricengine/junctionnames.go:271`).
Only wired names under the anchor are junctioned into the warp worktree.
The weft repo also holds content outside that set — repo-root files (the warp-binding record, `internal/fabriccli/fabric.go:79-82`; README; any pre-fabric legacy content, `internal/fabricengine/cleanup.go:14-19`) — for which there is NO warp-worktree-relative path at all.
"the files are literally there" (`discussion.md:135`) is false for those paths: a conflict there is invisible in the single visible tree, unresolvable through it, and any raw path reported for it either collides with warp's namespace or reveals the second repo.
The test row "a same-named path existing on both sides does not collide" (`discussion.md:318`) asserts an outcome the document provides no mapping rule to produce.
Also unspecified: whether unified paths are relative to the worktree root or to `AnchorPath()` — a real distinction on a subpath-anchored hub (`CONSTRAINTS.md:31`, `internal/fabricengine/pull.go:402-406`).

### B7. `merge-in [<branch>]` with an optional branch contradicts the document's own no-topology-resolution rule

`discussion.md:191` makes the branch optional; `discussion.md:97-100` and the Q&A (`discussion.md:365`) say Fabric resolves no parent and the caller always supplies the branch.
Bare `lyx fabric merge-in` therefore has no defined behaviour: deriving a default parent is the forbidden shape, and merging the configured upstream (git's own no-argument `merge` semantics) is never stated — and collides with B9's no-upstream weft case.
One of the two lines is wrong; a planner cannot tell which.

### B8. The weft-side source branch derivation and its absence case are unspecified in the normative sections

`MergeIn(source)`/`Merge(source)` take one branch; the weft side must merge some derivative of it.
Only the Q&A hints at `WeftBranchName(source)` (`discussion.md:366`); no Decision or Scope line states it.
The case where `<source>-weft` does not exist in the weft repo — merging in a pre-fabric or externally created branch; `weftBranchExists` (`internal/fabricengine/weftwiring.go:90-104`) exists precisely because this happens — is fully unspecified: hard error, warp-only merge (violating two-sidedness), or fork-on-demand per `add.go`'s adopt-vs-create precedent.
This is the brief's "source branch not existing on one side" edge, and it blocks planning.

### B9. Fetch, upstream-absence, and MergeIn's guard set are all unspecified

- `Merge`'s guard fast-forwards "the target branch to its upstream via `merge --ff-only`" (`discussion.md:155`) — millhouse fetches first (`mill-merge/SKILL.md:305-308`); the document never says whether `Merge` fetches, and never says what happens on a side with NO upstream. A weft branch routinely has no upstream (`internal/fabricengine/pull.go:31-35,215-218` treats it as a vacuous no-op) — is the guard skipped, failed, or vacuous per side? Skipping on one side and enforcing on the other is a behavioural asymmetry observable from outside.
- `MergeIn` is given no guards at all: dirty current pair (git's own refusal leaks a raw, unowned error), stale local source vs `origin/<source>` (`mill-merge-in/SKILL.md:36-43` prefers origin when ahead — cited as precedent at `discussion.md:253`, silently dropped), source existing only as a remote ref.

### B10. `Merge`'s target-handle acquisition is unspecified, and the "parent is always checked out" premise is not an invariant

`Merge` runs on a handle opened at the target pair (`discussion.md:97`); `Open(l)` (`internal/fabricengine/open.go:12`) needs that pair's `Location`.
Nothing says how Finalize — running in the task worktree — finds the target pair's worktree path or Location, nor what happens when NO worktree is checked out on the target branch (pair removed; branches outlive worktrees by design, `internal/fabricengine/add.go:69-75`).
millhouse resolves the parent worktree from `git worktree list` and has an in-place fallback (`mill-merge/SKILL.md:232,55-59`) — both silently dropped.
The supporting claim that the parent branch "always is" checked out in a hub layout (`discussion.md:89`) is asserted, not established, anywhere in the codebase.
For the CLI the problem is sharper: `lyx fabric merge` needs cwd inside the target pair (`CONSTRAINTS.md:20-21` cwd gate), which an agent bound by worktree isolation (project `CLAUDE.md`) cannot reach from the task worktree — the document never confronts how the merge-to-parent step is actually driven.

### B11. The lifecycle quartet is vacuous for `Merge`, and public `MergeAbort`'s squash branch can never execute

`Merge` either commits or self-aborts within one call (`discussion.md:165-171`), so no `Merge` ever leaves in-progress state for the quartet to act on — "uniformly to both verbs" (`discussion.md:175`) is unfalsifiable decoration for `Merge`, and the document should say the quartet in practice serves `MergeIn` only.
Worse, `MergeAbort`'s specified squash behaviour — "restores from the pre-merge SHAs Fabric captured before starting" (`discussion.md:177-178`) — describes a cross-call public API path whose inputs exist only inside the `Merge` invocation that already self-aborted, with the persistence that could carry them explicitly rejected (`discussion.md:180`).
As written an implementer cannot determine what public `MergeAbort`'s squash branch is for or where its SHAs come from.

### B12. No result/error type shapes, and the Constraints section omits three binding machine-enforced invariants

No merge result type is named or sketched: clean-path contents (SHAs? committed flags?), whether conflicts arrive as a result state or an error, the name and payload of the typed redirect-to-`MergeIn` error (`discussion.md:166-167`), the dirty-target and diverged-target error types, `MergeContinue`'s message source, the CLI `-m` default, and the squash-with-empty-`Message` default are all unspecified — the entire public surface must be invented.
Worse, the Constraints section (`discussion.md:255-282`) omits invariants that materially shape this exact code:

- **Mutation Record Invariant** (`CONSTRAINTS.md:365-381`): every mutating fabric verb's result type must embed `MutationRecord` and its envelope must carry `mutations`/`partial`. `MergeIn`/`Merge`/`MergeContinue`/`MergeAbort` are mutating verbs; a plan from this document alone ships result types that fail `TestMutationRecord_FabricengineProductionSource`.
- **Fabric Destruction Chokepoint Invariant** (`CONSTRAINTS.md:321-350`): `warp.ResetHard(`/`weft.ResetHard(` are banned bypass tokens outside `destroy.go`; the squash/ff abort paths and self-abort restore REQUIRE hard resets on both sides, so they must route through the gate (request shape, ownership, dirtiness — note an abort resets an intentionally dirty worktree, so the dirtiness declaration is a real design point) or fail `TestNoDestructiveBypass_FabricengineProductionSource`.
- **gitrepo Client Boundary Invariant** (`CONSTRAINTS.md:541-555`) and **gitexec Checked-Call Invariant** (`CONSTRAINTS.md:557-571`): every new merge primitive on `gitrepo.Repo` must land on `runChecked` and update the pinned method list in the same commit, or `TestGitrepoBoundary_PinnedRunCallSites` fails.

### B13. The millhouse post-commit rollback (checkpoint) is silently dropped with no replacement

`mill-merge-in` creates a checkpoint branch (step 2) that outlives the merge commit: a failed verify AFTER `merge --continue` rolls back via `git reset --hard "$CHK"` (`mill-merge-in/SKILL.md:150,199-210`).
In this design, once `MergeContinue` commits both sides there is no Fabric-level way to undo the landed merge — no two-sided reset-to-SHA verb exists or is planned (the audit at `discussion.md:335` spins out even single-sided non-hard reset), and Finalize cannot use raw git (`CONSTRAINTS.md:282`, cited by the document itself at `discussion.md:259-261`).
The workflow the two-verb split "claims to mirror" (`discussion.md:85,90`) therefore has a recovery affordance this design removes without recording the removal — it appears in neither Scope Out nor Audit findings.

---

## NIT

### N1. The lock rationale rests on a false premise: the combined write lock is per-pair, not hub-wide

`discussion.md:147` justifies not holding the lock across conflict resolution because it "would block every other worktree".
The lock lives at `<weftPath>/.weft/weft.write.lock` — inside the pair's own weft worktree (`internal/fabricengine/weftgit.go:26-27,45-54`, `commit.go:178-186`) — so holding it blocks only that pair's writers.
The decision may still be right (that pair's own agents commit during resolution), but the recorded rationale is factually wrong.
Related unexamined consequence: during the resolution window the clean side holds a staged in-merge index, and any routine `Fabric.Commit` on the pair then hits git's "cannot do a partial commit during a merge" as a raw, unowned error — worth a sentence.

### N2. "Conflicts cannot be resolved in the target worktree at all" is false as a git-mechanics claim

`discussion.md:168` (and the tangled sentence at `discussion.md:88-89`).
Conflicts from `git merge <source>` run in the target worktree are resolvable exactly there; git forbids only checking out a branch already checked out elsewhere.
The real reasons are policy — agent locality, worktree isolation, not disturbing the parent (`mill-merge/SKILL.md:13-19`).
The redirect-to-`MergeIn` decision is fine; its stated mechanical justification would mislead a planner into believing git forces the self-abort.

### N3. Small citation and description inaccuracies

- `discussion.md:352`: "`cleanup.go:92` is a stub returning `false`" — the function body is `cleanup.go:93-95`; line 92 is its comment. Trivially off.
- `discussion.md:266`: the vocabulary walk is described as "production Go plus an `internal/**/*.md` walk"; the actual walk also covers `contracts/stencils/**/*.md` and the embedded agent templates (`CONSTRAINTS.md:271-276`). Incomplete, not wrong, and irrelevant to the merge surface.
- `discussion.md:221`: "`add.go:116-169`" — the fork call spans through line 172; the cited range stops one call short of the create branch's closing brace. Cosmetic.
- `discussion.md:238` says the merge primitives "compose with" `WorktreeChangedFiles` et al. — all six named methods verified to exist (`gitrepo.go:72,237`, `reset.go:13`, `ancestry.go:21`, `worktree.go:23`, `pull.go:33`).

### N4. Guard probe ordering can leak side existence through error sequencing

Existing precedent probes warp first (`internal/fabricengine/fabric.go:62-66`; `ErrMissingPath` names a single path, `fabric.go:36-47`).
If `Merge`'s dirty/ff guards run side-by-side and return the first failure, which path gets named — and in which order two dirty sides are reported — is observable structure.
The document requires side-free error strings (`discussion.md:262-266`) but never constrains ordering or how a dirty weft-side path is rendered in the unified namespace; the dirty-target test row (`discussion.md:314`) asserts only the halt, not the report's shape.
Belongs in the vocabulary-assertion test spec (`discussion.md:320-322`).

### N5. Testing section omits the tier/hermeticity constraints its own tests trip

New git-spawning tests must carry the `integration` build tag (Test Tier Purity, `CONSTRAINTS.md:446-457`) and live in packages with a `HermeticGitEnv` `TestMain` (`CONSTRAINTS.md:459-467`).
Both packages already comply, so this is free — but a document claiming plan-sufficiency should say it.

### N6. `merge --continue` after a conflicted `merge-in` is implied but never stated

The CLI decision (`discussion.md:191-196`) gives `--continue`/`--abort` on `merge` only; since the quartet is shared, the operator resolving a `merge-in` conflict presumably runs `lyx fabric merge --continue`.
Never said, and it reads oddly against `merge-in` being "a distinct verb".
One sentence fixes it.

---

## Verified-accurate claims (spot-check record)

- No `Merge` anywhere in `internal/gitrepo` (grep over all methods); `Fabric.Diff`/`Fabric.Status` read-only (`internal/fabricengine/diff.go:69,102`). `discussion.md:14` ✓.
- `manifest/designs/finalize.md:26-27` quotes and the reword target ✓; `manifest/roadmap.md:16-19,26` blocked rows and audit instruction ✓.
- `internal/gitrepo/doc.go:139-141` scope-boundary sentence verbatim ✓.
- `internal/fabricengine/weftwiring.go:18-19,107` merge-base quotes ✓; `branchname.go:12-14` sole `-weft` derivation ✓.
- `commit.go:74` "combined write lock (acquired whenever anything lands, even warp-only)" ✓; lock name `.weft/weft.write.lock` = `weftgit.go:26-27` ✓.
- `pull.go` weft-first + `*PartialPullError` ✓; `PullResult` fields and `ChangeEntry.Side` literals ✓ (`pull.go:37-66`, `diff.go:42-55`); `PatternResidue` at `pull.go:67-81` ✓.
- `checkout.go:1-10` all-or-nothing precedent ✓.
- 16-verb surface list matches `internal/fabriccli/fabric.go:2-11` plus `unwire.go` ✓.
- `TestEnforcement_FabricVocabulary` at `internal/lyxcwd/enforcement_test.go:758` ✓; owner set per `CONSTRAINTS.md:262` ✓ (Finalize not in it).
- `hubforge.NewHub`/`AddPair` (`internal/hubforge/hub.go:215,315`), `gitrepo` `pull_test.go`/`reset_test.go`/`push_test.go`, `fabriccli` `envelope.go`/`envelopecontract_integration_test.go`/`cli_test.go`/`argsarity_test.go` — all exist ✓.
- `mill-merge-in/SKILL.md` steps 1-3 and `mill-merge/SKILL.md` step 5 described accurately, including `--diff-filter=U` (`mill-merge-in:88`) and the never-`reset --hard` ff rationale (`mill-merge:322`) ✓.
- `git merge --squash` records no `MERGE_HEAD` ✓ (git's documented behaviour).
- `RecordCorrespondence(warpSHA, weftSHA)` at `internal/fabricengine/index.go:102` ✓; `WiredNames` at `junctionnames.go:271` ✓; `Open(l)` sole constructor at `open.go:12` ✓; `PushWeft`/`CoalescePushBothAt` (`weftgit.go:299`, `coalesce.go:86`) ✓.
