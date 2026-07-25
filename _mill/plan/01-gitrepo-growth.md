# Batch: gitrepo-growth

```yaml
task: 'fabric: unify warp + weft into one git-coordination module'
batch: gitrepo-growth
number: 1
cards: 2
verify: go test -tags integration ./internal/gitrepo
depends-on: []
```

## Batch Scope

Grows `internal/gitrepo` with the two generic single-repo mechanics fabric needs and
gitrepo lacks: a fast-forward pull and a SHA-validated hard reset. Both follow gitrepo's
existing house style exactly — every git spawn through the unexported `run` helper, error
messages that name the repo path and the git exit code without leaking raw stderr,
caller-supplied SHAs validated via `validSHA` before reaching git, godoc-quality method
comments, and `doc.go` updated in the same commit. No fabric code in this batch; the new
surface consumed by batch 5 is `(*Repo).Pull() error` and `(*Repo).ResetHard(sha string)
error`. No batch-local decisions differ from the overview.

## Cards

### Card 1: fast-forward Pull

- **Context:**
  - `internal/gitrepo/gitrepo.go`
  - `internal/gitrepo/push.go`
  - `internal/gitrepo/gitrepo_test.go`
  - `internal/gitrepo/push_test.go`
  - `internal/gitrepo/testmain_test.go`
  - `internal/weftengine/sync.go`
  - `internal/gitexec/gitexec.go`
- **Edits:**
  - `internal/gitrepo/doc.go`
- **Creates:**
  - `internal/gitrepo/pull.go`
  - `internal/gitrepo/pull_test.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** Add `func (r *Repo) Pull() error` in the new `pull.go`: run
  `git pull --ff-only` via the existing unexported `run` helper. On spawn error wrap it;
  on non-zero exit return an error naming the repo path and exit code (mirror
  `weftengine`'s `Pull` semantics and `push.go`'s no-stderr-leak error style — the tests
  must assert the error string does NOT contain `fatal:`). Extend the pull section of
  `doc.go`'s package comment: `Pull` is fast-forward-only by contract; a diverged branch
  is an error, never a merge commit — divergence recovery stays a caller policy.
  `pull_test.go` is `//go:build integration`-tagged, reuses `push_test.go`'s fixture
  helpers (`newBareRemote`, `cloneFromBare`) and covers: remote advanced → `Pull`
  fast-forwards and the file content matches; local diverged from remote → error, local
  history unchanged; no remote configured → error naming the repo path without raw
  stderr.
- **Commit:** `feat(gitrepo): add fast-forward Pull`

### Card 2: SHA-validated ResetHard

- **Context:**
  - `internal/gitrepo/gitrepo.go`
  - `internal/gitrepo/gitrepo_test.go`
  - `internal/gitrepo/keyvalidation_test.go`
  - `internal/gitrepo/testmain_test.go`
- **Edits:**
  - `internal/gitrepo/doc.go`
- **Creates:**
  - `internal/gitrepo/reset.go`
  - `internal/gitrepo/reset_test.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** Add `func (r *Repo) ResetHard(sha string) error` in the new
  `reset.go`: reject any `sha` failing `validSHA` with a wrapped `ErrInvalidSHA` before
  any git spawn (exactly like `ChangedFilesSince`), then run `git reset --hard <sha>`
  via `run`; non-zero exit returns an error naming the repo path and exit code, no raw
  stderr. Document in `doc.go` that `ResetHard` is the primitive `RevertWithWeft` builds
  on and that the SHA-shape validation guarantees option-shaped strings can never become
  flags. `reset_test.go` is `//go:build integration`-tagged, reuses `gitrepo_test.go`'s
  helpers (`newRepo`, `writeFile`, `commitAll`, `runGit`) and covers: reset to an earlier
  commit SHA restores that commit's file state and moves `CurrentSHA` accordingly;
  invalid SHA argument (e.g. `--hard`, empty string) → `errors.Is(err, ErrInvalidSHA)`
  and no git spawned; well-formed hex SHA not present in history → error surfaced.
- **Commit:** `feat(gitrepo): add SHA-validated ResetHard`

## Batch Tests

`verify: go test -tags integration ./internal/gitrepo` runs the whole gitrepo package —
the two new integration-tagged files (`pull_test.go`, `reset_test.go`) plus the existing
`gitrepo_test.go`/`push_test.go`/`snapshot_test.go`/`keyvalidation_test.go`, proving the
additions and no regression in the package fabric builds on. Scope is the one package
this batch touches; the module-wide overview `verify: go test ./...` guards everything
else at the batch boundary.
