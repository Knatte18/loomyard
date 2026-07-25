# Batch: gitrepo-bisect-primitive

```yaml
task: 'webster: rewrite for flat card list'
batch: gitrepo-bisect-primitive
number: 4
cards: 2
verify: go test -tags integration ./internal/gitrepo/...
depends-on: []
```

## Batch Scope

Add the single new `internal/gitrepo` primitive this task introduces: a detached-checkout +
branch-restore pair used ONLY by the integration bisect (batch 8) to check a candidate SHA
out in-place in the single worktree, run `## verify:`, then restore HEAD. This is the sole
`gitrepo` addition in this task; the multi-card git-log-range enumerator (a separate future
grouping-batcher concern) is explicitly NOT added. Methods hang on the existing `*Repo`
receiver and route through the existing unexported `run(...)` choke point, matching the
package's established style. This batch is independent of every other (`depends-on: []`) and
can land early.

## Cards

### Card 13: CheckoutDetached / RestoreBranch / CurrentBranch on *Repo

- **Context:**
  - `internal/gitrepo/gitrepo.go`
  - `internal/gitrepo/snapshot.go`
  - `internal/gitexec/gitexec.go`
- **Edits:**
  - `internal/gitrepo/gitrepo.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Add three methods on `*Repo`, each invoking git via the existing unexported `func (r *Repo) run(args ...string)` choke point (never `gitexec.RunGit` directly, never a new dir arg — the checkout path is the receiver's `r.path`): `func (r *Repo) CurrentBranch() (string, error)` (`git symbolic-ref --short HEAD`, trimmed; return a wrapped error on detached HEAD so the caller captures the branch name BEFORE detaching); `func (r *Repo) CheckoutDetached(sha string) error` (`git checkout --detach <sha>`; validate `sha` with the existing `validSHA` helper first, returning `ErrInvalidSHA` on a non-hex value, matching `ChangedFilesSince`/`SHAExists`); `func (r *Repo) RestoreBranch(ref string) error` (`git checkout <ref>` to move HEAD back to the named branch). Keep signatures `(…, error)`-shaped and errors wrapped in the package's existing style. These three compose the bisect's in-place checkout/restore cycle: `CurrentBranch` → loop `CheckoutDetached(candidate)` → `RestoreBranch(branch)`.
- **Commit:** `feat(gitrepo): detached-checkout and branch-restore primitives for bisect`

### Card 14: gitrepo doc + integration tests

- **Context:**
  - `internal/gitrepo/gitrepo.go`
  - `internal/gitrepo/gitrepo_test.go`
  - `internal/gitrepo/testmain_test.go`
- **Edits:**
  - `internal/gitrepo/doc.go`
  - `internal/gitrepo/gitrepo_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** In `doc.go` add the three new methods to the "The Repo API" bullet list, and reconcile the "Scope boundaries" section: it currently states checkout/branch operations are "explicitly not supported… fabric layers topology operations on top of gitrepo." Narrow that boundary to admit this detached-checkout + branch-restore pair as the single in-place-bisect exception (a sequential post-run read-only-ish inspection, distinct from the parallel topology operations fabric owns), keeping the rest of the boundary text intact. In `gitrepo_test.go` add `//go:build integration`-tagged tests (the file is already integration-tagged and the package already has a hermetic `TestMain` — reuse both) for the three methods against a real temp repo: capture branch, detach to an older commit, assert HEAD is detached at that SHA, restore, assert HEAD is back on the branch; assert `CheckoutDetached` rejects a non-hex SHA with `ErrInvalidSHA` and `CurrentBranch` errors on detached HEAD. Follow `golang:golang-testing`.
- **Commit:** `docs(gitrepo): document bisect primitives and add integration tests`

## Batch Tests

`verify: go test -tags integration ./internal/gitrepo/...` — the `-tags integration` flag is
REQUIRED because the new tests spawn real git (detached checkout against a temp repo) and are
therefore integration-tagged per the Test Tier Purity Invariant; without the flag they would
not run and the gate would pass vacuously. The package's existing hermetic `TestMain`
(`testmain_test.go`) neutralizes the operator's gitconfig.
