# Batch: write-surface

```yaml
task: 'git-native-library: feasibility spike'
batch: write-surface
number: 3
cards: 3
verify: go test -tags integration ./internal/gitnativepoc/ && go test ./cmd/lyx/ -run 'TestTierPurity_UntaggedTestsSpawnNothing|TestHermeticGitEnv_GitSpawningPackagesHaveTestMain'
depends-on: [1, 2]
```

## Batch Scope

This batch probes `gitrepo`'s **write** surface over go-git in
`internal/gitnativepoc/write.go` with parity tests in `write_test.go`, built
**only to locate where the CLI boundary falls** — it does not migrate any real
write path. The pivotal case is `Push`'s rebase-retry (go-git's reported weak
spot), which the design mandates verifying. Each op is classified MIGRATE or
CLI-BOUND; a CLI-BOUND verdict here is the expected, legitimate outcome and is
asserted explicitly, not fixed. This batch depends on batch 2: `write.go` and
`read.go` are the same package, and Card 10's `SetSnapshotSHA` reuses
`read.go`'s `isStrictDescendant` (card 7) and snapshot-key validators (card 6),
so the package will not compile — and the batch verify will not pass — until
batch 2's `read.go` exists. (Cards 8 and 9 are self-contained and need nothing
from batch 2; only Card 10 creates the cross-card symbol dependency, but the
whole batch is scheduled after batch 2 regardless since they co-inhabit one
package.) It is therefore NOT parallel with batch 2. All new `_test.go` files
carry the `//go:build integration` tag and live in `package gitnativepoc`.

## Cards

### Card 8: StageAndCommit + StageAllAndCommit over go-git

- **Context:**
  - `internal/gitrepo/gitrepo.go`
  - `internal/gitnativepoc/gitnativepoc.go`
  - `internal/gitnativepoc/harness_test.go`
- **Edits:** none
- **Creates:**
  - `internal/gitnativepoc/write.go`
  - `internal/gitnativepoc/write_test.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** In `write.go`, add `StageAndCommit(msg string, files []string) (sha string, committed bool, err error)`
  and `StageAllAndCommit(msg string) (sha string, committed bool, err error)` on
  `*Repo` using go-git's worktree model (`Repository.Worktree`, `Worktree.Add`,
  `Worktree.Commit`, `Worktree.Status`). Mirror the `gitrepo` contracts: an empty
  `files` list stages nothing and returns `("", false, nil)` with no work; a
  pathspec-scoped commit stages/commits exactly the listed paths (leaving other
  staged entries untouched); "nothing to commit" returns `("", false, nil)` (not
  an error) on both the scoped and the wildcard (`add -A`-equivalent) variants; a
  real commit returns the new HEAD SHA with `committed=true`. In `write_test.go`,
  add differential tests against `gitrepo.Repo` using `newRepoFixture`: real
  commit SHA parity, nothing-to-commit parity, empty-list no-op parity, and the
  scoped-commit-leaves-other-staged-entries case. Identity: go-git commits
  require an explicit author signature, and go-git v5 does **not** honor
  `GIT_CONFIG_GLOBAL`/`GIT_CONFIG_NOSYSTEM` (it resolves global config from
  `$HOME`/XDG), so `lyxtest.HermeticGitEnv()`'s neutral identity is invisible to
  go-git — reading identity "via go-git config" would either find nothing or leak
  the operator's real `~/.gitconfig` (non-hermetic). The poc MUST therefore pass
  an explicit `object.Signature` (a fixed test identity, e.g. name `Test` / email
  `test@test.com` mirroring the hermetic neutral config) on `Worktree.Commit`;
  do not rely on go-git reading committer identity from config. Record
  MIGRATE/CLI-BOUND per op.
- **Commit:** `feat(gitnativepoc): StageAndCommit + StageAllAndCommit over go-git with parity tests`

### Card 9: Push + rebase-retry over go-git (pivotal CLI-BOUND probe)

- **Context:**
  - `internal/gitrepo/push.go`
  - `internal/gitnativepoc/gitnativepoc.go`
  - `internal/gitnativepoc/harness_test.go`
  - `internal/gitnativepoc/write.go`
- **Edits:**
  - `internal/gitnativepoc/write.go`
  - `internal/gitnativepoc/write_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Add `Push() error` on `*Repo` in `write.go` using go-git's
  `Repository.Push`, plus an attempt at the `gitrepo.pushWithRebaseRetry`
  recovery: on a non-fast-forward rejection, attempt the `pull --rebase`
  equivalent (fetch + rebase-replay) and retry once. Because go-git's rebase
  support is the reported weak spot, this card's primary output is the
  **verdict**: if go-git cannot perform the rebase-replay recovery, the method
  records `Push`/rebase-retry as **CLI-BOUND** (documented sentinel) and the test
  asserts that limitation. In `write_test.go`, add the pivotal test using
  `newBareRemoteFixture`: both clones advance `main` to force a non-fast-forward
  on the second push, then assert whether the go-git path can recover-and-push
  (MIGRATE) or genuinely cannot (CLI-BOUND) — comparing against `gitrepo.Push`'s
  behaviour on the identical fixture as the oracle. Mark any Windows-hinged
  aspect `Win11-pending`. This is the single most important test in the spike;
  its comment must state the observed verdict and the evidence.
- **Commit:** `feat(gitnativepoc): Push + rebase-retry probe over go-git with pivotal parity test`

### Card 10: SetSnapshotSHA fast-forward push + adopt-on-conflict over go-git

- **Context:**
  - `internal/gitrepo/snapshot.go`
  - `internal/gitnativepoc/gitnativepoc.go`
  - `internal/gitnativepoc/harness_test.go`
  - `internal/gitnativepoc/read.go`
  - `internal/gitnativepoc/write.go`
- **Edits:**
  - `internal/gitnativepoc/write.go`
  - `internal/gitnativepoc/write_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Add `SetSnapshotSHA(key, sha string) error` on `*Repo` in
  `write.go`, mirroring `gitrepo.SetSnapshotSHA`: validate `key` and `sha`
  (reuse the local validators from batch 2 / card 6); advance
  `refs/loomyard/snapshot/<key>` locally via go-git's reference API and push it
  fast-forward-only to the remote; on a rejected push, fetch-and-adopt the
  remote's value, and if the adopted value is a strict ancestor of `sha`
  (`isStrictDescendant` from card 7), re-advance and retry, bounded by a local
  `snapshotPushMaxAttempts` cap. In `write_test.go`, add a differential test
  against `gitrepo.SetSnapshotSHA` using `newBareRemoteFixture` with a simulated
  concurrent writer (a second clone sets the ref first, forcing the adopt path):
  assert the final ref value parity and the adopt-on-conflict resolution match. If
  go-git cannot do fast-forward-only ref push or the adopt round-trip, record
  CLI-BOUND with the divergence asserted. Note `gitrepo`'s rejection detection
  currently keys off stderr substrings — call out in the test comment whether
  go-git offers a typed rejection instead (a MIGRATE data point).
- **Commit:** `feat(gitnativepoc): SetSnapshotSHA adopt-on-conflict over go-git with parity test`

## Batch Tests

`verify` runs the package integration suite
(`go test -tags integration ./internal/gitnativepoc/`) — now including the
write-surface parity tests in `write_test.go`, of which the rebase-retry test
(card 9) is the pivotal one — plus the two scoped guard tests. The
`newBareRemoteFixture` tests are the heaviest (two clones over a bare remote) but
are the exact scenario the spike must exercise. Every CLI-BOUND op is asserted as
an explicit recorded divergence against the `gitrepo.Repo` oracle, not a failure.
