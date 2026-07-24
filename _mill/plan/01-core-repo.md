# Batch: core-repo

```yaml
task: 'gitrepo: generic, repo-agnostic git primitives'
batch: core-repo
number: 1
cards: 4
verify: go test -tags integration ./internal/gitrepo/
depends-on: []
```

## Batch Scope

Delivers the `internal/gitrepo` package foundation: the `Repo` type, `New`, the shared git-run
helper, and the read/commit primitives every later batch and consumer builds on — `CurrentSHA`,
`StageAndCommit`, `ChangedFilesSince`, `SHAExists` — plus the hermetic test scaffolding
(`testmain_test.go`) and their tests. The external interface the `push` and `snapshot` batches
consume is the `Repo` struct, its unexported `run` helper, and the package's error-posture
conventions. One batch because these all live in `gitrepo.go` + `doc.go` and share the same tiny
context (`gitexec` only).

## Cards

### Card 1: Repo type, New, package doc, CurrentSHA

- **Context:**
  - `internal/gitexec/gitexec.go`
  - `_mill/discussion.md`
- **Edits:** none
- **Creates:**
  - `internal/gitrepo/gitrepo.go`
  - `internal/gitrepo/doc.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** Declare `package gitrepo`. `doc.go` holds the sole `// Package gitrepo …`
  package-doc comment (a short initial version — the full design rationale is folded in by the
  docs batch); `gitrepo.go` opens with a plain file comment and `package gitrepo` (no second
  package-doc comment, to avoid a duplicate-package-comment vet warning). In `gitrepo.go`: define
  `type Repo struct { path string }` and `func New(path string) *Repo` that only stores `path`
  (no validation, no I/O, cannot fail). Add an unexported helper
  `func (r *Repo) run(args ...string) (stdout, stderr string, code int, err error)` that calls
  `gitexec.RunGit(args, r.path)`. Define `var ErrNoCommits = errors.New("gitrepo: repository has
  no commits")`. Implement `func (r *Repo) CurrentSHA() (string, error)`: run
  `rev-parse HEAD`; on spawn error return it; on a non-zero exit whose stderr indicates an empty
  repo (contains `ambiguous argument 'HEAD'` or `unknown revision`) return `"", ErrNoCommits`;
  any other non-zero exit returns an error including stderr; on success return
  `strings.TrimSpace(stdout)`.
- **Commit:** `feat(gitrepo): add Repo type, New, and CurrentSHA`

### Card 2: StageAndCommit with nothing-to-commit signal

- **Context:**
  - `internal/gitexec/gitexec.go`
  - `internal/boardengine/git.go`
  - `_mill/discussion.md`
- **Edits:**
  - `internal/gitrepo/gitrepo.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Add `func (r *Repo) StageAndCommit(msg string, files []string) (sha string,
  committed bool, err error)`. Stage exactly the listed files with `git add -- <files...>` (never
  `add -A`/wildcard); a non-zero exit returns an error. Detect a no-op with
  `git diff --cached --quiet`: exit 0 → nothing staged → return `"", false, nil` (the
  nothing-to-commit signal, not an error); exit 1 → staged changes exist, continue; any other exit
  → error. Commit with `git commit -m msg`; non-zero exit → error. Then call `CurrentSHA()` and
  return `sha, true, nil`. An empty `files` slice stages nothing and therefore returns the
  `committed=false` signal.
- **Commit:** `feat(gitrepo): add StageAndCommit with nothing-to-commit signal`

### Card 3: ChangedFilesSince and SHAExists

- **Context:**
  - `internal/gitexec/gitexec.go`
  - `_mill/discussion.md`
- **Edits:**
  - `internal/gitrepo/gitrepo.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Add `func (r *Repo) SHAExists(sha string) bool`: run
  `rev-parse --verify --quiet <sha>^{commit}`; return `true` only when `err == nil && code == 0`;
  swallow every other outcome (spawn error, non-zero exit) into `false`. Add
  `func (r *Repo) ChangedFilesSince(sha string) ([]string, error)`: run
  `diff --name-only <sha>..HEAD`; a non-zero exit (which includes a missing/invalid `sha`) returns
  an error including stderr; on success split stdout on newlines, drop empty lines, and return the
  repo-relative paths (committed changes only — no working-tree/staged inspection).
- **Commit:** `feat(gitrepo): add ChangedFilesSince and SHAExists`

### Card 4: hermetic test scaffolding and core read/commit tests

- **Context:**
  - `internal/gitexec/testmain_test.go`
  - `internal/gitexec/gitexec_test.go`
  - `internal/lyxtest/lyxtest.go`
  - `internal/warpengine/clone_integration_test.go`
  - `_mill/discussion.md`
- **Edits:** none
- **Creates:**
  - `internal/gitrepo/testmain_test.go`
  - `internal/gitrepo/gitrepo_test.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** `testmain_test.go`: first line `//go:build integration`, package
  `gitrepo_test`, a `TestMain(m *testing.M)` that calls `lyxtest.HermeticGitEnv()` then
  `os.Exit(m.Run())` — mirror `internal/gitexec/testmain_test.go` exactly. `gitrepo_test.go`:
  `//go:build integration`, package `gitrepo_test`. Build throwaway repos under `t.TempDir()` with
  `lyxtest.MustRun(t, dir, "git", "init", "-b", "main")` (identity is pinned by `HermeticGitEnv`).
  Cover: `CurrentSHA` returns HEAD on a repo with a commit and `ErrNoCommits` (via `errors.Is`) on
  a freshly-init'd empty repo; `StageAndCommit` commits only the listed files and returns
  `(sha, true, nil)`, leaves an unlisted dirty file uncommitted, returns `("", false, nil)` when
  the listed files are unchanged, and never stages an unlisted file; `ChangedFilesSince` returns
  the correct set for `sha..HEAD`, an empty slice when `sha == HEAD`, excludes an uncommitted edit,
  and errors on a fabricated SHA; `SHAExists` is `true` for a real SHA and `false` for a
  fabricated one and for garbage input.
- **Commit:** `test(gitrepo): hermetic tests for core read/commit primitives`

## Batch Tests

`verify: go test -tags integration ./internal/gitrepo/` compiles the package and runs
`gitrepo_test.go` under the hermetic `TestMain`. Scope is the single new package; no cross-cutting
helper is touched, so the per-package scope is correct. The untagged Tier-1 test arrives in the
snapshot batch — this batch's surface is entirely integration-tagged.
