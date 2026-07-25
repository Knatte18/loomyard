# Batch: gitrepo-stage-all

```yaml
task: 'board: use gitrepo as its git operator'
batch: gitrepo-stage-all
number: 1
cards: 3
verify: go test -tags integration ./internal/gitrepo/...
depends-on: []
```

## Batch Scope

This batch adds the one new primitive board needs and documents it. It is a
self-contained, purely-additive change to `internal/gitrepo`: a new wildcard
`StageAllAndCommit` method (board's opt-in `add -A` exception), the doc-comment
reconciliation of the two places `doc.go` currently asserts wildcard staging
never enters gitrepo, and an integration test for the new method. The external
interface the next batch (`boardengine-migration`) consumes is the method
signature `func (r *Repo) StageAllAndCommit(msg string) (sha string, committed
bool, err error)`. No behavior of the existing `StageAndCommit` / `Push`
surface changes. Batch-local: none beyond `## Shared Decisions`.

## Cards

### Card 1: Add StageAllAndCommit wildcard method to gitrepo

- **Context:**
  - `internal/gitrepo/doc.go`
- **Edits:**
  - `internal/gitrepo/gitrepo.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Add an exported method `func (r *Repo) StageAllAndCommit(msg
  string) (sha string, committed bool, err error)` to
  `internal/gitrepo/gitrepo.go`, placed immediately after `StageAndCommit`. It
  is the wildcard sibling of `StageAndCommit`, mirroring its `(sha, committed,
  err)` return semantics. Body, using the existing unexported `r.run` helper:
  (1) `r.run("add", "-A")` — on `err` return `("", false, err)`; on non-zero
  exit return `("", false, fmt.Errorf("gitrepo: git add -A: %s", stderr))`.
  (2) `r.run("diff", "--cached", "--quiet")` (unscoped — no `-- <files>`) — on
  `err` return it; then `switch code`: `case 0` return `("", false, nil)` (clean
  tree / nothing staged, the documented no-op signal); `case 1` fall through to
  commit; `default` return `("", false, fmt.Errorf("gitrepo: git diff --cached
  --quiet: %s", stderr))`. (3) `r.run("commit", "-m", msg)` — on `err` return it;
  on non-zero exit return `("", false, fmt.Errorf("gitrepo: git commit: %s",
  stderr))`. (4) `sha, err = r.CurrentSHA()` — on `err` return `("", false,
  err)`; else return `(sha, true, nil)`. Add a godoc comment stating this stages
  **all** working-tree changes via `git add -A` (unlike `StageAndCommit`, which
  never wildcard-stages), that it exists as board's `Sync`/`commitDirty` opt-in
  exception, and that other consumers should keep using the explicit-list
  `StageAndCommit`.
- **Commit:** `feat(gitrepo): add StageAllAndCommit wildcard-stage method`

### Card 2: Reconcile doc.go's two "never wildcard" assertions

- **Context:**
  - `internal/gitrepo/gitrepo.go`
- **Edits:**
  - `internal/gitrepo/doc.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Update **both** places `internal/gitrepo/doc.go` asserts
  wildcard staging never enters gitrepo so neither goes stale after Card 1:
  (a) the **Scope boundaries** section ("stage+commit (explicit file list, never
  wildcard-stage)") — add that `StageAllAndCommit` is a separate wildcard variant
  provided as board's opt-in exception, not a relaxation of the explicit-list
  default; (b) the **Push surface** section (the sentence "committing is always
  the caller's separate, prior StageAndCommit call, so a wildcard `add -A` never
  enters gitrepo") — reword so it acknowledges `StageAllAndCommit` as board's
  wildcard-stage escape hatch while keeping the point that committing is still
  the caller's separate step (push never stages). Also add `StageAllAndCommit`
  alongside `StageAndCommit` in the "# The Repo API" method list. State
  explicitly that `fabric`/`raddle`/`codeintel` keep using the explicit-list
  `StageAndCommit`.
- **Commit:** `docs(gitrepo): document StageAllAndCommit exception in package doc`

### Card 3: Integration test for StageAllAndCommit

- **Context:**
  - `internal/gitrepo/testmain_test.go`
- **Edits:**
  - `internal/gitrepo/gitrepo_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Add `TestStageAllAndCommit_*` integration tests to
  `internal/gitrepo/gitrepo_test.go` (the file is already `//go:build
  integration` and runs under the hermetic `TestMain`). Reuse the same
  real-git repo-construction helper the existing `StageAndCommit` tests use (see
  `TestStageAndCommit_CommitsOnlyListedFiles`). Cover three cases: (1) a working
  tree dirtied with **both** a new untracked file and a modification to an
  already-tracked file → `StageAllAndCommit` returns `committed == true`, a
  non-empty `sha`, and leaves the working tree clean (both changes captured in
  the commit); (2) a clean tree → returns `("", false, nil)` and creates no new
  commit (HEAD unchanged); (3) it captures a file that an explicit-list
  `StageAndCommit` would miss — i.e. a file not named in any explicit list is
  still committed by the `add -A` path. Exact assertion shapes are the test's.
- **Commit:** `test(gitrepo): cover StageAllAndCommit wildcard staging`

## Batch Tests

`verify: go test -tags integration ./internal/gitrepo/...` runs the whole
`internal/gitrepo` package under the integration tag, exercising the new
`TestStageAllAndCommit_*` tests (Card 3) plus every existing test
(`gitrepo_test.go`, `push_test.go`, `snapshot_test.go`, `keyvalidation_test.go`)
so the additive change is proven not to regress the existing `StageAndCommit` /
`Push` surface. Scoped to the one package this batch touches.
