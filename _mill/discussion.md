# Discussion: Ensure weft branches are orphan branches

```yaml
task: Ensure weft branches are orphan branches
slug: weft-orphan-branches
status: discussing
parent: main
```

## Problem

The task was filed (see `proposal-weft-orphan-branches.md`) on the premise that each
per-task weft branch should be a git **orphan branch** — a clean scratchpad with no
shared merge-base, "never merged anywhere," a pure sink. During discussion the operator
rejected that premise. The corrected requirement:

- The weft repo holds two kinds of content with **opposite flow needs**:
  - **`_lyx/`** — lyx system state (config, status, board, orchestration). Per-task. Must
    **never flow back** to the parent.
  - **`_codeguide/`** — durable documentation *about the source code*. These are real
    files that, when CG is active, **must merge back** to the corresponding parent's weft
    branch (squash, mother-repo style), with real 3-way conflict detection.
- Orphan branches have **no common ancestor by definition**, so a `git merge` of two
  orphan branches treats every overlapping `_codeguide` file as "added on both sides" →
  a conflict on every file, every time. A file-copy doesn't avoid this — it silently
  clobbers the parent's version (data loss). Orphan is therefore the **worst** topology
  for content that must merge back. **Orphan is rejected.**

**Why now:** the proposal's orphan instruction, if implemented, would have hard-broken the
future `_codeguide` merge-back. The branch model must be corrected (and the proposal
rewritten) *before* `_codeguide` is wired into weft, because the shared merge-base is a
prerequisite for that future work.

The governing invariant the operator set: **weft branching mirrors host-repo branching at
all times — anything else is just mess.** Concretely: weft branch name = host branch name
(already true); weft branch X forks from the weft branch of X's *host parent*; weft X
squash-merges back to its parent weft when host X merges to its host parent; weft X is
torn down when host X is. Today's spawn layer violates the *fork-parentage* half of this
for mid-work subtasks.

## Scope

**In:**

- Fix weft branch creation so it **mirrors host topology**: the new weft branch forks from
  the **parent's weft branch** (the weft branch whose name equals the host worktree's
  current branch at spawn time), passed as the `git worktree add` start-point — instead of
  always forking from the prime weft HEAD. Branches stay **non-orphan** with a real shared
  merge-base.
- Guard the parent-branch resolution: if the host worktree is on a **detached HEAD or an
  unborn branch** (so there is no branch name to mirror), `Add` aborts with a clear error
  and performs the full paired rollback (see Decisions → `detached-head-guard`).
- Confirm (and test) that teardown already mirrors host topology: weft branch + worktree +
  remote ref are removed when the task is torn down, leaving no leftover refs.
- Add/extend tests, including the **subtask-mid-work** case: a sub-branch spawned while
  working in another branch must produce a weft branch that **shares a merge-base with its
  parent's weft branch** (explicitly **not** orphan, and rooted on the parent — not on
  prime `main`).
- **Correct the wiki proposal `proposal-weft-orphan-branches.md`** to reject the orphan
  premise and record the mirror-host-topology model plus the deferred `_codeguide`
  squash-merge-back design. This is a **wiki-module operation**, not an in-tree edit: the
  proposal is a mill-wiki artifact (it does not exist in this worktree), so it is rewritten
  only via the wiki daemon / `/mill-wiki-push` — never a raw `git`/`Edit`/`Write` on the
  wiki. Treat this as a separate bookkeeping step from the code change; the durable design
  already lives in this `discussion.md`, the `internal/worktree` package header, and
  `docs/overview.md`.

**Out:**

- **No `_codeguide` merge-back implementation.** `_codeguide` is not part of Loomyard's
  weft pathspec yet (default pathspec is `_lyx`; the `_codeguide` junction activation is
  still deferred per `docs/overview.md`). The merge-back has no real input to act on and
  cannot be exercised by a test against real codeguide today. It is **fully specified here
  and in the rewritten proposal** but implemented by the future codeguide-in-weft task.
- No generic/dormant merge-back engine acting on an empty set (rejected as dead code —
  see Decisions).
- No change to `internal/weft` sync semantics (commit/push/pull/status). The change is
  purely in the worktree/spawn layer's branch creation.
- No change to host-branch creation — host branches are already correct (fork from the
  host's current branch).
- No `_lyx` content-flow changes; `_lyx` continues to be committed/synced to the weft
  branch's own remote and is never part of any merge-back.

## Decisions

### reject-orphan-keep-shared-base

- Decision: Per-task weft branches are **non-orphan**. They retain a shared merge-base with
  their parent weft branch.
- Rationale: `_codeguide` must merge back with 3-way conflict detection (mother-repo-style
  squash). 3-way merge requires a common ancestor; orphan branches have none, forcing a
  conflict on every overlapping file or a silent clobber. The proposal's "pure sink" framing
  was optimizing `_lyx` cleanliness while silently breaking the `_codeguide` requirement.
- Rejected: (a) orphan + clobbering file-copy for codeguide — silent data loss; (b) separate
  weft branches per directory (orphan for `_lyx`, parent-based for `_codeguide`) — clean but
  "an extra set of branches just for codeguide is too heavy" and breaks the one-weft-branch-
  per-host-branch invariant.

### mirror-host-topology

- Decision: Weft branching mirrors host-repo branching exactly. The new weft branch forks
  from the **parent's weft branch**, where the parent weft branch name equals the host
  worktree's current branch at spawn time. Implemented by passing that branch as the
  start-point to `git worktree add -b <branch> <weftPath> <parentWeftBranch>` in
  `createWeftWorktree`.
- Rationale: A top-level task (host parent `main`) already forks correctly from prime weft
  `main`. A mid-work **subtask** (host parent = some task branch Y) currently forks from
  prime weft HEAD instead of weft-Y, so it would not build on Y's in-progress codeguide and
  would not share Y's base. Mirroring host topology makes the subtask weft branch root on
  weft-Y, giving `_codeguide` the correct ancestor for a future merge-back to weft-Y.
- Rejected: Always fork from prime weft `main` (today's behavior) — wrong base for subtasks;
  breaks the mirror invariant.

### lyx-isolation-by-pathspec-not-topology

- Decision: `_lyx` is kept out of the (future) merge-back by **pathspec scoping**, not by
  branch topology. The future merge-back set is `_codeguide` only; `_lyx` is never in it.
- Rationale: Within one weft branch per host branch, two directories have opposite flow
  needs. Topology alone cannot satisfy both. Scoping the merge-back to `_codeguide` lets
  `_lyx` carry *forward* at spawn (inheriting hub config is desirable) while never flowing
  *back*. The proposal's "merge footgun" (merging the whole weft branch incl. `_lyx`) is
  closed by tooling always doing a pathspec-scoped squash-merge.
- Rejected: orphan branches as the isolation mechanism (breaks codeguide); excluding `_lyx`
  from the weft branch entirely (it must be synced/portable — that is weft's purpose).

### defer-codeguide-mergeback

- Decision: Do **not** implement `_codeguide` squash-merge-back in this task. Specify it
  fully; implement it in the future codeguide-in-weft task.
- Rationale: `_codeguide` is not in the weft pathspec yet, so a merge-back engine has no
  real input and can only be tested against a synthetic stand-in directory — proving
  plumbing, not the feature. Its conflict policy and completion-time trigger point are
  codeguide-task decisions that would be guesses today. YAGNI.
- Rejected: building a generic dormant pathspec-driven merge-back engine now (operates on a
  currently-empty merge-back pathspec, tested via a fake dir) — dead code exercised only by
  synthetic tests, and the codeguide task would revisit it anyway.

### detached-head-guard

- Decision: If the host worktree's HEAD is **not a branch** at spawn time — detached HEAD
  (`git rev-parse --abbrev-ref HEAD` returns the literal `"HEAD"`) or an unborn branch —
  `Add` aborts with a clear error (e.g. "cannot spawn weft branch: host worktree is on a
  detached HEAD / unborn branch") and performs the existing full paired rollback. No weft
  branch is created.
- Rationale: The mirror invariant requires a named host branch to mirror; with no branch
  there is nothing to fork the weft branch from and `"HEAD"` would be a bogus start-point.
  Normal spawns are always from a named branch (prime on `main`, task worktrees on their
  task branch), so this guard never fires on the happy path — it only rejects the abnormal
  "checked out a tag/raw commit" state. This mirrors the existing "missing parent weft
  branch" failure shape.
- Rejected: (a) silently fall back to forking from prime weft `main` — violates the mirror
  invariant and hides operator error; (b) pass `"HEAD"` through to git — worse diagnostics,
  late failure.

### future-mergeback-design (specified, not implemented)

- Decision (for the future codeguide task, recorded so it is settled): when a task completes
  and host X squash-merges to its host parent, weft X squash-merges its **merge-back set
  (`_codeguide` only, never `_lyx`)** into the parent weft branch, **squash** (single commit,
  mother-repo style), using the shared merge-base for real 3-way conflict detection, with
  **conflicts surfaced for resolution** (no silent clobber). This runs *before*
  `removeWeftWorktree`. The exact git mechanism (e.g. `git merge --squash` then dropping
  non-mergeback paths, or `git merge-tree` on the subtree) and the precise completion-flow
  trigger are that task's call.
- Rationale: Records the agreed model so the codeguide task does not re-derive it.
- Rejected: file-copy propagation (clobbers); non-squash merge (diverges from mother-repo
  convention).

## Technical context

What mill-plan needs to know:

- **Branch creation lives in the worktree/spawn layer, not `internal/weft`.**
  `internal/weft` is branch-relative (operates on whatever HEAD the weft worktree is on;
  push to `@{u}`, `pull --ff-only`/`--rebase`) and needs **no change**.
- **Primary change site:** `internal/worktree/weft.go` →
  `createWeftWorktree(l *paths.Layout, slug, branch string)`. Today it runs
  `git worktree add -b <branch> <weftPath>` in `l.WeftRepoRoot()`, which bases the new
  branch on the weft repo's current HEAD (prime weft `main`). It must instead base on the
  **parent weft branch** = the host worktree's current branch name. That branch name must
  be resolved from the host side (e.g. `git -C <WorktreeRoot> rev-parse --abbrev-ref HEAD`
  / `symbolic-ref --short HEAD`) and threaded into `createWeftWorktree` as a start-point
  argument. `WorktreeRoot` is the git toplevel of the caller's cwd, so for a mid-work
  subtask spawn it correctly reflects the parent task's host worktree (on branch Y). If the
  resolution yields `"HEAD"` (detached) or fails (unborn branch), abort per
  `detached-head-guard`. Respect the `internal/paths` invariant — derive all geometry via `paths.Layout`
  (`WeftRepoRoot()`, `WeftWorktreePath(slug)`), never raw cwd/rev-parse outside the allowed
  files.
- **Caller:** `internal/worktree/add.go` → `(*Worktree).Add(...)`, step 8
  (`createWeftWorktree(l, slug, branch)`). The host worktree is created in step 7 with
  `git worktree add -b <branch> <target>` in `l.WorktreeRoot` — confirming the host forks
  from the host's current branch; the parent branch is whatever HEAD `WorktreeRoot` is on.
  The parent weft-branch name to pass equals that current host branch name (mirror
  invariant: weft branch name = host branch name; `branch = cfg.BranchPrefix + slug` is the
  *new* branch, not the parent).
- **Signature change ripple:** threading the start-point arg changes `createWeftWorktree`'s
  signature. The plan must sweep **all** callers — currently only `add.go:142`
  (`createWeftWorktree(l, slug, branch)`), but the plan must confirm by grepping the tree
  (incl. `internal/worktree/weft_test.go` and any other test) so the build is not left
  half-migrated.
- **Parent weft branch existence:** since the weft repo is shared (all weft worktrees and
  branches live in one repo), the parent weft branch already exists when its host worktree
  exists (prime weft `main`, or weft-Y for a subtask of Y). If the parent weft branch is
  missing, fail with a clear error rather than silently forking from HEAD.
- **Teardown** is already mirrored: `internal/worktree/weft.go` →
  `removeWeftWorktree(l, slug, branch, force)` does `git worktree remove [--force]`,
  `git branch -D <branch>`, `git worktree prune`; the rollback path in `add.go`
  (`rollbackAdd`) does the same. This satisfies proposal item 3 — confirm via test, no code
  change expected.
- **Weft pathspec config:** `internal/weft/config.go` (`Pathspec` field, `Dirs()` splitter)
  and `internal/weft/template.yaml` (`pathspec: _lyx`). `_codeguide` is opt-in and not
  active yet — relevant only to the deferred merge-back, not this task.
- **Seeding note (no longer a blocker):** the orphan-empty-worktree seeding problem
  (junction target missing, config absent) was a concern *only* under the rejected orphan
  model. Because the new weft branch now forks from the parent weft branch, the new worktree
  inherits the parent's committed `_lyx` content, so the host `_lyx` junction target exists
  and config carries forward as before. No seeding step is needed.
- **Test scaffolding:** `internal/lyxtest/lyxtest.go` builds host+weft fixtures
  (`buildWeftPrime` commits `_lyx/config/placeholder` on weft `main`; `CopyPaired` /
  `CopyPairedLocal` produce paired host+weft hubs). Existing weft spawn tests live in
  `internal/worktree/weft_test.go` and `internal/worktree/add_test.go`. Tests must use
  `AddOptions{SkipPush:true}` (or `SkipGit`) to avoid hitting the empty bare remote, per
  existing patterns.
- **Git version:** 2.53 (orphan support exists but is now irrelevant since orphan is
  rejected).

## Constraints

From `CONSTRAINTS.md` (hub root):

- **Path Invariant:** all cwd/worktree-root and weft geometry must resolve through
  `internal/paths` (`paths.Getwd()`, `paths.Resolve()`, `Layout` methods). Raw `os.Getwd`
  and `git rev-parse --show-toplevel` are banned outside `internal/paths` and
  `cmd/lyx/main.go`, enforced at `go test`/CI by `internal/paths/enforcement_test.go`.
  Resolving the host's *current branch* via `git rev-parse --abbrev-ref HEAD` is a
  branch query (not a toplevel/cwd query) and is run with an explicit cwd via the existing
  `git.RunGit(args, dir)` helper, so it does not trip the enforcement scan — verify against
  the enforcement test regardless.
- **Documentation Lifecycle:** durable design rationale lives in package header comments /
  `docs/overview.md`, mechanical per-module docs are deleted when the module lands. Record
  the corrected weft branch model in the relevant package header (`internal/worktree`) and,
  if appropriate, the weft overlay section of `docs/overview.md`.

Project convention (CLAUDE.md): this worktree is mill-managed; durable notes go in
versioned docs/comments, not file-memory.

## Testing

Go (`golang-testing` conventions; table-driven where natural). TDD candidates:

- **`createWeftWorktree` start-point (TDD):** spawning a weft worktree roots the new weft
  branch on the **parent weft branch**, not on the weft repo's arbitrary HEAD. Capture the
  parent weft branch's tip SHA at spawn time, then assert `git merge-base <new> <parent>`
  **equals that exact SHA** — proving the new branch forks from the parent's *tip*, not
  merely that it shares *some* ancestor. A merge-base-non-empty check is insufficient: it
  cannot distinguish forking-from-tip from forking-from-an-older-common-ancestor (e.g. prime
  `main`), so it would not reject the old behavior.
- **Subtask-mid-work (TDD, proposal item 4):** from a host worktree on a non-`main` branch
  Y (i.e. a subtask spawned mid-work), spawn a new slug. Assert with the **discriminating**
  check: capture weft-Y's tip SHA at spawn and assert `git merge-base <new-weft> <weft-Y>`
  **equals weft-Y's tip SHA** — proving the new branch forks from weft-Y's tip, *not* from
  prime weft `main`. This is the explicit anti-regression guard against both the old
  prime-`main` behavior and the rejected orphan design (an orphan would have no merge-base
  at all).
- **Detached/unborn host HEAD (TDD):** spawning from a host worktree on a detached HEAD (or
  unborn branch) makes `Add` fail with a clear error and perform full paired rollback — no
  weft branch or worktree left behind. Guards `detached-head-guard`.
- **Top-level task:** spawning from host `main` roots the new weft branch on prime weft
  `main` (regression guard that the common case is unchanged).
- **Teardown mirror:** after teardown, the weft branch, worktree, and (when pushed) remote
  ref are gone, with no dangling refs; `rollbackAdd` likewise tears the weft side down.
  Extend existing teardown/rollback tests rather than duplicating.
- **Missing parent weft branch:** if the parent weft branch does not exist, `Add` fails
  with a clear error and performs full paired rollback (no partial worktree left behind).
- **`internal/paths/enforcement_test.go`** must still pass after any new git-branch query is
  added.

Out of test scope: `_codeguide` merge-back (no implementation this task). The future
codeguide task owns merge/conflict/squash tests.

## Q&A log

- **Q:** Should weft branches be orphan, as the proposal says? **A:** No — orphan is
  rejected. `_codeguide` are real files that must merge back to the parent weft branch with
  3-way conflict detection, which requires a shared merge-base; orphan branches have none.
- **Q:** Doesn't a file-copy of `_codeguide` avoid conflicts? **A:** No — it hides them by
  silently clobbering the parent's version (data loss). Conflict-aware squash-merge
  (mother-repo style) is required.
- **Q:** What is the governing branch invariant? **A:** Weft branching mirrors host-repo
  branching at all times — anything else is just mess. Weft X forks from the weft branch of
  X's host parent; merges/teardown mirror the host.
- **Q:** How do `_lyx` (never merge back) and `_codeguide` (merge back) coexist in one
  branch? **A:** By pathspec-scoping the merge-back to `_codeguide` only — not by branch
  topology. `_lyx` carries forward at spawn but is never in the merge-back set.
- **Q:** Implement the `_codeguide` merge-back now? **A:** No. `_codeguide` is not in
  Loomyard's weft pathspec yet; the merge-back has no real input and cannot be tested
  against real codeguide. Specify it fully here and in the rewritten proposal; implement it
  in the future codeguide-in-weft task. It SHALL be added later.
- **Q:** Build a generic dormant merge-back engine now (empty merge-back set, synthetic
  tests)? **A:** No — dead code that proves only plumbing; the codeguide task would revisit
  it anyway. Keep this task to the branch model + tests + proposal rewrite.
- **Q (review r1 gap):** What if the host worktree is on a detached HEAD / unborn branch at
  spawn (no branch name to mirror)? **A:** Fail fast — `Add` aborts with a clear error and
  full paired rollback; never fall back to prime `main` or pass `"HEAD"` through. Confirmed
  this never fires on the happy path (normal spawns are always on a named branch).
- **Q (review r1 gap):** How should the subtask-mid-work test prove the fork is rooted on
  the parent, not prime `main`? **A:** Capture the parent weft tip SHA at spawn and assert
  `git merge-base <new> <parent>` *equals that SHA* — not merely non-empty, which would not
  discriminate forking-from-tip from forking-from-an-older-ancestor.
