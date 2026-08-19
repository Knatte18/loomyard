# Batch: gitrepo merge primitives

```yaml
task: 'fabric: merge-conflict primitive'
batch: gitrepo merge primitives
number: 1
cards: 2
verify: go test ./cmd/lyx/ ./internal/lyxcwd/ && go test -tags integration ./internal/gitrepo/
depends-on: []
```

## Batch Scope

Delivers the single-repo git merge primitives on `gitrepo.Repo` that `fabricengine`'s two-sided coordination (batches 2–5) composes: merge start (normal and squash) with four-way outcome classification, conclude-commit, conflicted-path enumeration, `MERGE_HEAD` detection, a fast-forward-only advance for the pre-merge sync step, and a general ref→SHA resolver.
It also lands every same-commit obligation those methods trigger: the `gitrepoPinnedRunBoundMethods` additions, the `internal/gitrepo/doc.go` scope-boundary amendment, and the `CONSTRAINTS.md` method enumeration.
The external interface the next batch consumes is exactly the method set of card 1.
Batch-local decisions: none beyond the Shared Decisions (`MergeHeadPresent` via runChecked; `MergeFFOnly`/`ResolveSHA` as support primitives).

## Cards

### Card 1: merge primitives on gitrepo.Repo, pinned lists, doc scope

- **Context:**
  - `_mill/discussion.md`
  - `internal/gitrepo/gitrepo.go`
  - `internal/gitrepo/ancestry.go`
  - `internal/gitrepo/reset.go`
  - `internal/gitrepo/gogit.go`
  - `internal/gitrepo/worktree.go`
  - `internal/gitexec/gitexec.go`
  - `cmd/lyx/checkedcall_test.go`
- **Edits:**
  - `internal/gitrepo/doc.go`
  - `cmd/lyx/gitrepoboundary_test.go`
  - `CONSTRAINTS.md`
- **Creates:**
  - `internal/gitrepo/merge.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  Create `internal/gitrepo/merge.go` containing, with the doc comments from `_mill/discussion.md`'s `public-surface-shapes` decision adapted as godoc:

  ```go
  type MergeOutcome int

  const (
      MergeStaged          MergeOutcome = iota // merged into index/worktree, uncommitted
      MergeConflicted                          // unmerged index entries present
      MergeFastForwarded                       // HEAD moved; nothing staged, no MERGE_HEAD
      MergeAlreadyUpToDate                     // nothing to do
  )

  func (r *Repo) MergeStart(ref string, squash bool) (MergeOutcome, error)
  func (r *Repo) MergeConclude(msg string) error
  func (r *Repo) ConflictedFiles() ([]string, error)
  func (r *Repo) MergeHeadPresent() (bool, error)
  func (r *Repo) MergeFFOnly(ref string) error
  func (r *Repo) ResolveSHA(ref string) (string, error)
  ```

  `MergeStart` behaviour: reject a `ref` with a leading `-` (return `ErrInvalidSHA`, mirroring `IsAncestor`'s argument pre-check in `ancestry.go`).
  Capture HEAD via `CurrentSHA` before running.
  Run `runChecked("merge", "--no-commit", ref)` when `squash` is false, `runChecked("merge", "--squash", ref)` when true.
  On error, recover `*gitexec.GitError` with `errors.As` (the `ancestry.go` idiom);
  when recovered, probe `ConflictedFiles()` — non-empty means `MergeConflicted, nil`;
  otherwise return the genuine error.
  The probe is `ConflictedFiles()`, not a second `git ls-files -u` spelling — see the Shared Decision "MergeStart's post-error conflict probe reuses ConflictedFiles", which names the `public-surface-shapes` godoc clause it supersedes and why.
  On success, probe staged state via `runChecked("diff", "--cached", "--quiet")` classified by the existing `errors.As` + `ExitCode == 1` idiom from `StageAndCommit` (exit 1 = something staged), and HEAD movement via `CurrentSHA`:
  HEAD moved with nothing staged → `MergeFastForwarded`;
  HEAD unmoved with nothing staged → `MergeAlreadyUpToDate`;
  otherwise → `MergeStaged`.
  `MergeStart` classifies on repo state, never on exit code alone, and adds no raw gitexec site.

  `MergeConclude`: with non-empty `msg` run `runChecked("commit", "-m", msg)`;
  with empty `msg` run `runChecked("commit", "--no-edit")`, which takes git's prepared `MERGE_MSG`/`SQUASH_MSG` without opening an editor — the `--no-edit` spelling is mandatory (a bare `git commit` would hang a non-interactive caller on the configured editor).

  `ConflictedFiles`: `runChecked("diff", "--name-only", "--diff-filter=U")`, split non-empty lines, return repo-root-relative paths (empty slice, never nil, when none).

  `MergeHeadPresent`: `runChecked("rev-parse", "--verify", "--quiet", "MERGE_HEAD")`;
  exit 0 → `(true, nil)`;
  `*gitexec.GitError` with `ExitCode == 1` → `(false, nil)`;
  anything else → the wrapped error (see Shared Decision "MergeHeadPresent resolves via runChecked").

  `MergeFFOnly`: reject a leading-`-` ref, then `runChecked("merge", "--ff-only", ref)`, wrapping any error with the standard `"gitrepo: merge --ff-only %s in %s: %w"` shape — the loud-failure sync mechanism the fabricengine guards decision requires (never `reset --hard`).

  `ResolveSHA`: resolve an arbitrary ref (branch, `origin/<branch>`, SHA) to a full SHA via go-git — `r.goGit()` then `repo.ResolveRevision(plumbing.Revision(ref))` wrapped in `lookupObjectRetrying` per `gogit.go`'s pattern;
  a resolution failure returns a wrapped error the caller can treat as "not found".
  go-git read of on-disk state, so it stays off the pinned CLI list per the gitrepo Client Boundary Invariant.

  Error wrapping throughout follows the package convention: `fmt.Errorf("gitrepo: <args> in %s: %w", r.path, err)`.

  In `cmd/lyx/gitrepoboundary_test.go`: add `"MergeStart"`, `"MergeConclude"`, `"ConflictedFiles"`, `"MergeHeadPresent"`, `"MergeFFOnly"` to `gitrepoPinnedRunBoundMethods`, and correct `gitrepoBoundaryMinScannedFiles`' doc comment's file enumeration: it is already stale, omitting the existing `blobread.go`, so add **both** `blobread.go` and `merge.go` and state the count as 10 non-test files (`internal/gitrepo` has 9 today).
  Do not touch `cmd/lyx/checkedcall_test.go` — every new call uses `runChecked`, so `internal/gitrepo`'s pinned raw-site count stays 3.

  In `internal/gitrepo/doc.go`'s "Scope boundaries" paragraph: amend the sentence "Rebase, interactive staging, cherry-pick, conflict resolution, and general-purpose branch/checkout management are explicitly not supported" so merge start/conclude and conflicted-path enumeration are admitted (used by fabric's two-sided merge coordination) while rebase, interactive staging, cherry-pick, and general-purpose branch/checkout management stay out — keeping the paragraph's existing honesty about what is and is not covered.
  Conflict *resolution* remains unsupported — gitrepo reports conflicts; resolving them is the caller's (ultimately an operator's or agent's) job.

  In `CONSTRAINTS.md`, "gitrepo Client Boundary Invariant" section: extend the method enumeration ("used for `StageAndCommit`, …") with `MergeStart`, `MergeConclude`, `ConflictedFiles`, `MergeHeadPresent`, `MergeFFOnly`, per that invariant's own same-commit rule.
- **Commit:** `feat(gitrepo): merge primitives — MergeStart, MergeConclude, ConflictedFiles, MergeHeadPresent, MergeFFOnly, ResolveSHA`

### Card 2: gitrepo merge integration tests

- **Context:**
  - `internal/gitrepo/merge.go`
  - `internal/gitrepo/gitrepo_test.go`
  - `internal/gitrepo/reset_test.go`
  - `internal/gitrepo/push_test.go`
  - `internal/gitrepo/testmain_test.go`
  - `internal/gitkit/gitkit.go`
- **Edits:** none
- **Creates:**
  - `internal/gitrepo/merge_integration_test.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  New `//go:build integration` test file in `package gitrepo_test`, reusing `gitrepo_test.go`'s fixture helpers (`newRepo`, `writeFile`, `commitAll`, `runGit`) and the table-driven `got = %v; want %v` conventions.
  Every test function name contains `Merge` (the batch verify pattern relies on it).
  Cover, in this order (the discussion's TDD list):
  - `ConflictedFiles` on a manufactured conflict (two branches editing the same line) returns the conflicted path; empty slice, never nil, on a clean tree.
  - `MergeStart` outcome classification across all four outcomes: clean-staged (assert changes staged and uncommitted via `git diff --cached` and unchanged HEAD), conflicted (unmerged entries present, nil error), fast-forwarded (assert HEAD moved, nothing staged, no `MERGE_HEAD` — pinning the documented ff-defeats-`--no-commit` behaviour), already-up-to-date (HEAD unmoved, nothing staged).
  - squash start: assert no `MERGE_HEAD` exists, changes staged and uncommitted; and squash on an ff-possible source stages without moving HEAD (classified `MergeStaged`).
  - `MergeStart` rejects a leading-`-` ref with `ErrInvalidSHA` before spawning git.
  - `MergeConclude` with an explicit message (commit message equals it) and with empty message (commit message comes from git's prepared `MERGE_MSG`).
  - `ResetHard` to the pre-merge SHA clears in-progress merge state — `MergeHeadPresent` false and `ConflictedFiles` empty afterwards (the abort mechanism's load-bearing property).
  - `MergeHeadPresent` true after a conflicted non-squash merge, false after `ResetHard` and false after `MergeConclude`.
  - `MergeFFOnly` advances a behind checkout to its target and fails loudly (non-nil error, HEAD unmoved) on a genuinely diverged pair.
  - `ResolveSHA` resolves a branch name, an `origin/<branch>` remote-tracking ref (fixture via `push_test.go`'s `newBareRemote`/`cloneFromBare` helpers), and a full SHA to the same 40-char SHA; an unknown ref returns an error.
- **Commit:** `test(gitrepo): integration coverage for merge primitives`

## Batch Tests

`verify` runs the untagged `cmd/lyx` guard suite (pinned-list equality: `gitrepoboundary_test.go`, `checkedcall_test.go`, tier purity for the new tagged test file) plus the full `internal/gitrepo` integration suite, which includes every test card 2 adds alongside the existing regression suite for the package this batch edits.
The gitrepo suite is small (seconds), so no `-run` scoping is needed here.
`./internal/lyxcwd/` joins every batch's untagged run (batches 1–5 as well as 6) so `TestEnforcement_FabricVocabulary` and the Markdown Link Integrity walk see each batch's new production files and doc edits in the batch that writes them — `internal/gitrepo` is outside the Fabric Vocabulary owner set, so a stray side token in this batch's `merge.go` or `doc.go` must fail here, not five batches later.
