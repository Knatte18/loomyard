# Plan: Ensure weft branches are orphan branches

```yaml
task: "Ensure weft branches are orphan branches"
slug: "weft-orphan-branches"
approved: true
started: "20260624-130200"
parent: "main"
root: ""
verify: null
```

## Batch Index

```yaml
batches:
  - number: 1
    name: weft-mirror-branching
    file: 01-weft-mirror-branching.md
    depends-on: []
    verify: go test -tags integration -run 'TestWeft|TestAdd|TestSeeder' ./internal/worktree/
```

## Shared Decisions

### Decision: non-orphan, mirror host topology

- **Decision:** Per-task weft branches are **non-orphan**. A new weft branch forks from the
  **parent's weft branch** — the weft branch whose name equals the host worktree's current
  branch at spawn time — passed as the `git worktree add` start-point. The task title says
  "orphan branches" but discussion **rejected** orphan: `_codeguide` must (in a future task)
  squash-merge back to the parent weft branch with 3-way conflict detection, which requires a
  shared merge-base. See `_mill/discussion.md` → Decisions `reject-orphan-keep-shared-base`,
  `mirror-host-topology`.
- **Rationale:** Today `createWeftWorktree` forks from the weft repo's current HEAD (prime
  weft `main`), which is wrong for mid-work subtasks (parent ≠ `main`). Mirroring host
  topology roots each weft branch on its parent's weft branch.
- **Applies to:** all batches

### Decision: detached/unborn host HEAD aborts

- **Decision:** If the host worktree's HEAD is not a usable branch name at spawn time, `Add`
  aborts with a clear error before creating anything. Two distinct git signals both trigger
  the abort: a **detached HEAD** makes `git rev-parse --abbrev-ref HEAD` print literal `"HEAD"`
  (exit 0); an **unborn branch** makes it exit **non-zero**. See `_mill/discussion.md` →
  Decisions `detached-head-guard`.
- **Rationale:** With no branch name there is nothing to mirror; `"HEAD"` would be a bogus
  start-point. Placing the guard before any creation means no rollback is needed. (This
  supersedes `_mill/discussion.md`'s phrasing that the abort "performs the existing full paired
  rollback" — the plan places the guard ahead of host creation, so nothing is created and no
  rollback path runs. Same end state: no partial worktree.)
- **Applies to:** all batches

### Decision: codeguide merge-back is out of scope

- **Decision:** This task does **not** implement `_codeguide` squash-merge-back. `_codeguide`
  is not in Loomyard's weft pathspec yet (default `_lyx`); there is nothing to merge or test.
  The mechanism is specified in `_mill/discussion.md` → `future-mergeback-design` for the
  future codeguide-in-weft task.
- **Rationale:** YAGNI; no real input, no testable surface today.
- **Applies to:** all batches

### Decision: Go testing conventions

- **Decision:** Tests are Go table-driven where natural, use the existing `internal/lyxtest`
  fixtures (`CopyPaired`/`CopyPairedLocal`, `MustRun`) and `internal/git.RunGit`. Spawn tests
  pass `AddOptions{SkipPush: true}` to avoid the empty bare remote. Discriminating fork
  assertions compare a captured tip SHA against `git merge-base`, never "non-empty".
- **Rationale:** Matches existing `weft_test.go` / `add_test.go` patterns.
- **Applies to:** all batches

## All Files Touched

- `docs/overview.md`
- `internal/worktree/add.go`
- `internal/worktree/add_test.go`
- `internal/worktree/weft.go`
- `internal/worktree/weft_test.go`

## Out of plan (operator bookkeeping)

- **Wiki proposal rewrite.** `proposal-weft-orphan-branches.md` lives in the mill wiki, not
  this worktree, and per `CLAUDE.md` is rewritten only via the wiki daemon / `/mill-wiki-push`
  — never an in-tree `Edit`/`Write`. It is therefore **not** an implementer card. The operator
  corrects the wiki proposal separately after merge; the durable design already lives in
  `_mill/discussion.md`, `internal/worktree/weft.go`, and `docs/overview.md`.
