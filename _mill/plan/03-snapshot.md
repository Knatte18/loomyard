# Batch: snapshot

```yaml
task: 'gitrepo: generic, repo-agnostic git primitives'
batch: snapshot
number: 3
cards: 2
verify: go test -tags integration ./internal/gitrepo/
depends-on: [1]
```

## Batch Scope

Delivers snapshot tracking in a new `snapshot.go`: `SnapshotSHA`/`SetSnapshotSHA` over git refs
(`refs/loomyard/snapshot/<key>`) pushed to remote, with key validation, remote resolution, the
fast-forward-only adopt-on-conflict write, and the fetch-degrades-to-local read. Ships two test
files: the untagged Tier-1 `keyvalidation_test.go` (pure logic) and the integration
`snapshot_test.go` (two-clone remote fixture). Touches only new files and reads `Repo` from batch
1 — no overlap with the push batch, so the two run in parallel.

## Cards

### Card 8: snapshot ref tracking

- **Context:**
  - `internal/gitexec/gitexec.go`
  - `internal/gitrepo/gitrepo.go`
  - `_mill/discussion.md`
- **Edits:** none
- **Creates:**
  - `internal/gitrepo/snapshot.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** Add `var ErrInvalidSnapshotKey = errors.New("gitrepo: invalid snapshot key")`.
  Add unexported `func validSnapshotKey(key string) bool` — accept only keys matching
  `^[A-Za-z0-9][A-Za-z0-9._-]*$` and containing no `..` (accepts `codeintel-go`, `raddle`; rejects
  whitespace, `~ ^ : ? * [ \`, leading/trailing `/`, empty). Add helper
  `func snapshotRef(key string) string` returning `"refs/loomyard/snapshot/" + key`, and
  `func (r *Repo) remoteName() string` resolving the current branch's configured remote
  (`git symbolic-ref --short HEAD` then `git config --get branch.<name>.remote`), falling back to
  `"origin"`. `func (r *Repo) SnapshotSHA(key string) (string, error)`: return `ErrInvalidSnapshotKey`
  on an invalid key; best-effort `git fetch <remote> +refs/loomyard/snapshot/*:refs/loomyard/snapshot/*`,
  **ignoring any fetch failure (degrade to the local ref)**; then `rev-parse --verify --quiet
  <ref>`; return the trimmed SHA, or `("", nil)` when the ref is absent. `func (r *Repo)
  SetSnapshotSHA(key, sha string) error`: reject invalid key; `git update-ref <ref> <sha>`;
  `git push <remote> <ref>` (default push is fast-forward-only); on a rejection (stderr contains
  `non-fast-forward` / `rejected` / `fetch first`) run `git fetch <remote> +<ref>:<ref>` to
  **adopt** the remote value into the local ref and return nil; any other push failure returns an
  error including stderr.
- **Commit:** `feat(gitrepo): add snapshot ref tracking (SnapshotSHA/SetSnapshotSHA)`

### Card 9: key-validation and snapshot round-trip tests

- **Context:**
  - `internal/lyxtest/lyxtest.go`
  - `internal/warpengine/clone_integration_test.go`
  - `internal/gitrepo/snapshot.go`
  - `_mill/discussion.md`
- **Edits:** none
- **Creates:**
  - `internal/gitrepo/keyvalidation_test.go`
  - `internal/gitrepo/snapshot_test.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** `keyvalidation_test.go`: **untagged** (no build constraint), internal package
  `gitrepo` (to reach unexported `validSnapshotKey`), spawns no git and contains none of
  `gitexec.RunGit` / `exec.Command` / `lyxtest.Copy*` (Test Tier Purity). Table-test
  `validSnapshotKey`: accept `codeintel-go`, `codeintel-py`, `raddle`; reject `""`, `has space`,
  `bad~key`, `a:b`, `a..b`, `/lead`, `trail/`. `snapshot_test.go`: `//go:build integration`,
  package `gitrepo_test`; a bare remote plus two clones sharing it. Cover: `SnapshotSHA` returns
  `("", nil)` before any set; `SetSnapshotSHA` in clone A is visible to clone B after B's
  `SnapshotSHA` (which fetches first); fast-forward-only adopt-on-conflict — when clone B has
  advanced the key, clone A's `SetSnapshotSHA` of a non-descendant push is rejected and adopts B's
  value with no error; an invalid key returns `ErrInvalidSnapshotKey` from both methods.
- **Commit:** `test(gitrepo): snapshot round-trip and key-validation tests`

## Batch Tests

`verify: go test -tags integration ./internal/gitrepo/` runs both new test files: the untagged
`keyvalidation_test.go` (also runs on any plain `go test`, Tier 1) and the integration
`snapshot_test.go` under the hermetic `TestMain`. The two-clone-on-a-bare-remote fixture is what
exercises the remote push, the cross-clone visibility, and the adopt-on-conflict path.
