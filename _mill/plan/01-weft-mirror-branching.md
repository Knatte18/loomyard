# Batch: weft-mirror-branching

```yaml
task: "Ensure weft branches are orphan branches"
batch: "weft-mirror-branching"
number: 1
cards: 4
verify: go test ./internal/worktree/
depends-on: []
```

## Batch Scope

This single batch makes weft branch creation mirror host-repo branching: the new weft branch
forks from the **parent's weft branch** (the host worktree's current branch name) instead of
the weft repo's arbitrary HEAD, and `Add` aborts cleanly when the host HEAD is not a branch.
All production code lives in `internal/worktree` (`weft.go`, `add.go`); tests live in the
sibling `_test.go` files; documentation is updated in the `weft.go` header and
`docs/overview.md`. It is one batch because the signature change in `createWeftWorktree`, its
sole caller in `add.go`, the tests, and the docs are a single cohesive change well under a
Sonnet context window. There is no external interface for a later batch to consume. Batch-local
decisions: none beyond `## Shared Decisions` in the overview.

## Cards

### Card 1: Thread a start-point into `createWeftWorktree`

- **Context:**
  - `internal/worktree/add.go`
  - `internal/git/git.go`
  - `internal/paths/paths.go`
- **Edits:**
  - `internal/worktree/weft.go`
- **Creates:** none
- **Deletes:** none
- **Requirements:** Change the signature of `createWeftWorktree` from
  `createWeftWorktree(l *paths.Layout, slug, branch string) error` to
  `createWeftWorktree(l *paths.Layout, slug, branch, startPoint string) error`. Change the git
  invocation from `[]string{"worktree", "add", "-b", branch, weftPath}` to
  `[]string{"worktree", "add", "-b", branch, weftPath, startPoint}` so the new weft branch
  forks from `startPoint` (the parent weft branch) rather than the weft repo's current HEAD.
  Update the function's doc comment to state that the new branch roots on the parent weft
  branch (`startPoint`), preserving the shared merge-base. Do not change any other behavior in
  this function. (The sole caller is updated in Card 2.)
- **Commit:** `feat(worktree): fork weft branch from parent weft branch start-point`

### Card 2: Resolve parent host branch and guard detached/unborn HEAD in `Add`

- **Context:**
  - `internal/worktree/weft.go`
  - `internal/git/git.go`
  - `internal/paths/paths.go`
- **Edits:**
  - `internal/worktree/add.go`
- **Creates:** none
- **Deletes:** none
- **Requirements:** In `(*Worktree).Add`, after the weft prechecks block (the `weftBranchExists`
  check that returns the `"weft branch %q already exists"` error) and **before** the host
  worktree creation (`git worktree add -b <branch> <target>` in `l.WorktreeRoot`), resolve the
  parent host branch: call `git.RunGit([]string{"rev-parse", "--abbrev-ref", "HEAD"}, l.WorktreeRoot)`.
  If `exitCode != 0` (unborn branch) OR `strings.TrimSpace(stdout) == "HEAD"` (detached HEAD),
  return `AddResult{}, fmt.Errorf("cannot spawn weft branch: host worktree is on a detached HEAD or unborn branch")`.
  Placing this before any creation means no rollback is required. Bind the trimmed branch name
  to a local `parentBranch`. At the weft-creation step, change the call from
  `createWeftWorktree(l, slug, branch)` to `createWeftWorktree(l, slug, branch, parentBranch)`.
  `strings`, `fmt`, and `git` are already imported. Update the step-list doc comment on `Add` to
  mention the parent-branch resolution and the new start-point argument.
- **Commit:** `feat(worktree): mirror host branch as weft fork point with detached-HEAD guard`

### Card 3: Tests for mirrored fork point, subtask isolation, and detached guard

- **Context:**
  - `internal/worktree/weft.go`
  - `internal/worktree/add.go`
  - `internal/lyxtest/lyxtest.go`
  - `internal/git/git.go`
- **Edits:**
  - `internal/worktree/weft_test.go`
  - `internal/worktree/add_test.go`
- **Creates:** none
- **Deletes:** none
- **Requirements:** Add Go tests (use `lyxtest` fixtures, `internal/git.RunGit`, and
  `AddOptions{SkipPush: true}`). All `git merge-base` / `rev-parse` queries run with
  cwd `f.Layout.WeftRepoRoot()`.
  1. **Top-level fork point (discriminating).** From the host prime on `main`, capture weft
     `main`'s tip SHA (`rev-parse refs/heads/main`) *before* spawning, run `Add` for a slug,
     then assert `git merge-base <new-weft-branch> main` **equals that captured SHA** — proving
     the new weft branch forks from weft `main`'s tip. A non-empty merge-base check is
     insufficient and must not be used.
  2. **Subtask-mid-work isolation (anti-regression).** Put the host prime on a non-`main`
     branch `Y` (`git checkout -b Y` + a commit in `l.WorktreeRoot`). Create a matching weft
     branch `Y` in the weft repo and **advance its tip one commit beyond weft `main`** (e.g. via
     a temporary weft worktree checked out on `Y` with an extra commit, then remove the temp
     worktree) so `tip(Y) != tip(main)`; capture `tip(Y)`. Run `Add` for a new slug. Assert
     `git merge-base <new-weft-branch> Y` **equals `tip(Y)`** — proving the new branch forks
     from weft-`Y`'s tip, not prime weft `main`. This discriminating assertion is the explicit
     guard against both the old prime-`main` behavior and the rejected orphan design (an orphan
     would have no merge-base at all).
  3. **Detached/unborn HEAD guard.** Put the host prime on a detached HEAD
     (`git checkout --detach`) and assert `Add` returns an error containing the substring
     `"detached HEAD"` (matching Card 2's message) and leaves no host worktree dir, host branch,
     weft worktree dir, or weft branch behind (no partial state). May be added as a case in
     `TestAdd` or as a dedicated test.
  4. **Teardown mirror (confirm).** Ensure `TestAddRollback` still passes with the new
     `createWeftWorktree` signature and continues to assert the weft branch is gone after
     rollback; extend it only if the signature change requires it.
- **Commit:** `test(worktree): assert weft fork mirrors host branch and detached-HEAD aborts`

### Card 4: Document the weft branch model

- **Context:**
  - `internal/worktree/add.go`
  - `_mill/discussion.md`
- **Edits:**
  - `internal/worktree/weft.go`
  - `docs/overview.md`
- **Creates:** none
- **Deletes:** none
- **Requirements:** In the `internal/worktree/weft.go` file header comment, add a short
  paragraph recording the durable design: weft branches mirror host-repo branching — each weft
  branch forks from its parent's weft branch (non-orphan, shared merge-base), `_lyx` never
  merges back while `_codeguide` (future) squash-merges back to the parent weft branch, and a
  detached/unborn host HEAD aborts the spawn. In `docs/overview.md`, under the
  `## Weft overlay model` section, add a `### Branch model` subsection stating the same: weft
  branch name = host branch name; weft-X forks from the weft branch of X's host parent;
  branches are non-orphan to preserve the merge-base needed for the future `_codeguide`
  squash-merge-back; `_lyx` is isolated by pathspec (never merges back), not by orphan
  topology. Keep it concise; do not duplicate the full discussion.
- **Commit:** `docs(worktree): record weft branch-mirrors-host model`

## Batch Tests

`verify: go test ./internal/worktree/` runs the entire `internal/worktree` package, which is
exactly the surface this batch touches (`weft.go`, `add.go` and their `_test.go` siblings). The
new tests in Card 3 live in `weft_test.go` (fork-point and subtask-isolation) and `add_test.go`
(detached-HEAD guard, rollback confirmation). The docs-only edits in Card 4 have no runnable
surface and are covered by the package compiling. Scope is the single affected package — no
unbounded cross-repo suite is needed.
