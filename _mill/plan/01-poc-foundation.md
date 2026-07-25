# Batch: poc-foundation

```yaml
task: 'git-native-library: feasibility spike'
batch: poc-foundation
number: 1
cards: 3
verify: go test -tags integration ./internal/gitnativepoc/ && go test ./cmd/lyx/ -run 'TestTierPurity_UntaggedTestsSpawnNothing|TestHermeticGitEnv_GitSpawningPackagesHaveTestMain'
depends-on: []
```

## Batch Scope

This batch stands up the `internal/gitnativepoc` package and the test
scaffolding every later batch consumes: the go-git dependency in `go.mod`, the
go-git-backed `Repo` type and its constructor, the integration-tagged hermetic
`TestMain`, and the differential parity-harness helpers (fixture builders +
assert helpers). It delivers no operation implementations yet — those land in
batches 2 (read) and 3 (write), which both depend on this batch. The external
interface those batches consume is: the `Repo` type + `OpenRepo` constructor
(from `gitnativepoc.go`) and the harness helper functions (from
`harness_test.go`). The batch is one unit because the harness helpers and the
`Repo` type are useless apart and must compile together for the guards to pass.

## Cards

### Card 1: go-git dependency + package skeleton

- **Context:**
  - `internal/gitrepo/gitrepo.go`
  - `internal/gitexec/gitexec.go`
  - `go.mod`
- **Edits:**
  - `go.mod`
  - `go.sum`
- **Creates:**
  - `internal/gitnativepoc/gitnativepoc.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** Add the go-git dependency by running
  `go get github.com/go-git/go-git/v5` (the module path MUST include the `/v5`
  suffix — the bare `github.com/go-git/go-git` path resolves the deprecated
  pre-module version); this updates `go.mod` and `go.sum` with the resolved,
  pinned version. Create `internal/gitnativepoc/gitnativepoc.go` in
  `package gitnativepoc` opening with a plain file-level comment (a
  `// gitnativepoc.go — ...` comment, NOT a `// Package gitnativepoc` doc comment:
  the canonical package-doc comment is authored in `doc.go` in batch 4, and two
  package-doc comments would duplicate) noting this is a throwaway-but-kept
  feasibility-spike package, never wired into `cmd/lyx`, not a registered module.
  Define an exported `Repo` type that wraps a filesystem path
  to a git checkout, mirroring `gitrepo.Repo`'s role, and an `OpenRepo(path string) (*Repo, error)`
  constructor that opens the checkout via go-git's `git.PlainOpen` (return the
  go-git error unchanged on failure). Store both the path and the opened go-git
  `*git.Repository` handle on `Repo` so later methods can use either the go-git
  object model or a path-scoped fallback. Do not implement any operation methods
  in this card. Keep the file OS-portable (no Linux-only path assumptions).
- **Commit:** `feat(gitnativepoc): add go-git dep and package skeleton`

### Card 2: integration-tagged hermetic TestMain

- **Context:**
  - `internal/gitrepo/testmain_test.go`
  - `internal/lyxtest/hermetic.go`
  - `cmd/lyx/hermeticenv_test.go`
- **Edits:** none
- **Creates:**
  - `internal/gitnativepoc/testmain_test.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** Create `internal/gitnativepoc/testmain_test.go` with a
  `//go:build integration` constraint as its first line, in `package gitnativepoc`
  (in-package test so later cards can exercise unexported helpers). Add a
  `TestMain(m *testing.M)` whose first line calls `lyxtest.HermeticGitEnv()` then
  `os.Exit(m.Run())` — mirror `internal/gitrepo/testmain_test.go` exactly. This
  satisfies the Hermetic Git Test Environment Invariant for the whole package.
- **Commit:** `test(gitnativepoc): integration-tagged hermetic TestMain`

### Card 3: differential parity-harness helpers

- **Context:**
  - `internal/gitrepo/gitrepo_test.go`
  - `internal/gitrepo/push_test.go`
  - `internal/gitrepo/snapshot_test.go`
  - `internal/gitrepo/gitrepo.go`
  - `internal/gitexec/gitexec.go`
  - `internal/lyxtest/lyxtest.go`
  - `internal/gitnativepoc/gitnativepoc.go`
- **Edits:** none
- **Creates:**
  - `internal/gitnativepoc/harness_test.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** Create `internal/gitnativepoc/harness_test.go` with a
  `//go:build integration` first line, in `package gitnativepoc`. Provide the
  reusable fixture builders (each returning a temp-dir path built under
  `t.TempDir()`, using `lyxtest.MustRun` for `git` setup, mirroring the helper
  style in `internal/gitrepo/gitrepo_test.go`): (1) `newRepoFixture` — a repo on
  branch `main` with one initial commit; (2) `newEmptyRepoFixture` — `git init -b main`
  with an unborn HEAD (no commit); (3) `newNonASCIIFixture` — a repo with a commit
  adding a non-ASCII filename (e.g. `å.txt`); (4) `newRenameFixture` — a repo
  where a file is renamed across two commits; (5) `newSnapshotRefFixture` — a repo
  with a `refs/loomyard/snapshot/<key>` ref set; (6) `newBareRemoteFixture` — a
  bare remote plus two clones both tracking `main`, for `@{u}`, push-rejection,
  and rebase-retry cases. Provide assert helpers: `assertParitySHA`,
  `assertParityFileList` (sorted compare), `assertParityBool`, and
  `assertParityErrClass` (compares typed-error class / `errors.Is` on both sides).
  Provide a `runGit(t, dir, args...) (stdout, stderr string, code int, err error)`
  passthrough to `gitexec.RunGit` for direct fixture assertions. Helpers may be
  unused until batches 2–3; that is fine (unused package-level functions do not
  break the Go build). Keep everything OS-portable.
- **Commit:** `test(gitnativepoc): differential parity-harness helpers`

## Batch Tests

`verify` runs `go test -tags integration ./internal/gitnativepoc/` (compiles the
package plus the integration test files and runs `TestMain`; with no `Test*`
functions yet it exits 0 after the hermetic setup) and the two repo-wide guard
tests scoped by `-run` — `TestTierPurity_UntaggedTestsSpawnNothing` and
`TestHermeticGitEnv_GitSpawningPackagesHaveTestMain` — to confirm the new
integration-tagged files are correctly tagged and the package carries the
hermetic `TestMain`. The `-run` scoping keeps the gate fast (the full `cmd/lyx`
suite includes slow e2e tests).
