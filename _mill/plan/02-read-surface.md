# Batch: read-surface

```yaml
task: 'git-native-library: feasibility spike'
batch: read-surface
number: 2
cards: 4
verify: go test -tags integration ./internal/gitnativepoc/ && go test ./cmd/lyx/ -run 'TestTierPurity_UntaggedTestsSpawnNothing|TestHermeticGitEnv_GitSpawningPackagesHaveTestMain'
depends-on: [1]
```

## Batch Scope

This batch reimplements `gitrepo`'s **read** surface — the primary MIGRATE
candidates — over go-git in `internal/gitnativepoc/read.go`, and adds the
differential parity tests in `read_test.go` using the batch-1 harness helpers.
Each operation is classified MIGRATE or CLI-BOUND by the hard-gate rubric
((a) typed result, (b) behavioural parity, (c) Windows-capable), with the
verdict recorded as a test-level comment and later folded into `doc.go` (batch 4).
This batch depends only on batch 1; batch 3 (write-surface) depends on this batch
in turn, because Card 10 there reuses symbols defined in this batch's `read.go`
(same package). All new `_test.go` files carry the `//go:build integration` tag
and live in `package gitnativepoc`.

## Cards

### Card 4: CurrentSHA + SHAExists over go-git

- **Context:**
  - `internal/gitrepo/gitrepo.go`
  - `internal/gitnativepoc/gitnativepoc.go`
  - `internal/gitnativepoc/harness_test.go`
- **Edits:** none
- **Creates:**
  - `internal/gitnativepoc/read.go`
  - `internal/gitnativepoc/read_test.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** In `read.go`, add methods `CurrentSHA() (string, error)` and
  `SHAExists(sha string) bool` on `*Repo` using go-git's object model
  (`Repository.Head`, `Repository.ResolveRevision`, `CommitObject`). Mirror
  `gitrepo.CurrentSHA`'s contract: an unborn HEAD returns a typed sentinel —
  define `var ErrNoCommits` in this package (checkable via `errors.Is`) — not a
  generic error. `SHAExists` must peel to a commit and swallow any
  failure/missing/garbage/non-hex SHA into `false` (mirror `gitrepo`'s
  `validSHA` hex-shape guard; re-declare the equivalent check locally rather than
  importing gitrepo internals). In `read_test.go`, add differential tests
  asserting parity against `gitrepo.Repo` (the oracle) using `newRepoFixture` and
  `newEmptyRepoFixture`: SHA equality for a committed repo, `ErrNoCommits`-class
  parity for the unborn-HEAD case, and `false`/`false` parity for missing and
  non-hex SHAs. Comment each test with its MIGRATE/CLI-BOUND verdict per the
  `cli-bound-is-a-recorded-outcome` shared decision.
- **Commit:** `feat(gitnativepoc): CurrentSHA + SHAExists over go-git with parity tests`

### Card 5: ChangedFilesSince over go-git

- **Context:**
  - `internal/gitrepo/gitrepo.go`
  - `internal/gitnativepoc/gitnativepoc.go`
  - `internal/gitnativepoc/harness_test.go`
- **Edits:**
  - `internal/gitnativepoc/read.go`
  - `internal/gitnativepoc/read_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Add `ChangedFilesSince(sha string) ([]string, error)` on
  `*Repo` in `read.go`, computing the committed-history diff between `sha` and
  HEAD via go-git's tree diff (`CommitObject(...).Tree()`, `Tree.Diff`/`Patch`).
  Mirror `gitrepo.ChangedFilesSince`'s three tricky contracts: (1) non-ASCII paths
  returned **verbatim** (no C-quoting); (2) a rename reported as **both** the old
  path (deleted) and the new path (added), never folded (the `--no-renames`
  equivalent — configure go-git's diff to not detect renames, or post-process to
  split); (3) a non-hex `sha` returns a typed `ErrInvalidSHA` (define locally)
  without touching the repo. In `read_test.go`, add differential tests against
  `gitrepo.Repo` using `newNonASCIIFixture` and `newRenameFixture`: assert the
  sorted file lists match, the non-ASCII name survives verbatim on both sides, and
  the rename splits into delete+add on both sides. If go-git cannot reproduce a
  contract, record it as CLI-BOUND per `cli-bound-is-a-recorded-outcome` and
  assert the divergence explicitly.
- **Commit:** `feat(gitnativepoc): ChangedFilesSince over go-git with parity tests`

### Card 6: SnapshotSHA read + remoteName over go-git

- **Context:**
  - `internal/gitrepo/snapshot.go`
  - `internal/gitrepo/gitrepo.go`
  - `internal/gitnativepoc/gitnativepoc.go`
  - `internal/gitnativepoc/harness_test.go`
- **Edits:**
  - `internal/gitnativepoc/read.go`
  - `internal/gitnativepoc/read_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Add `SnapshotSHA(key string) (string, error)` and an
  unexported `remoteName() string` on `*Repo` in `read.go`. `remoteName` mirrors
  `gitrepo.remoteName`: resolve the current branch's tracked remote via go-git's
  config/reference model, falling back to `"origin"` when unset. `SnapshotSHA`
  mirrors `gitrepo.SnapshotSHA`: validate `key` (re-declare the equivalent of
  `validSnapshotKey` locally — reject `..`, trailing `.`, `.lock` suffix, and the
  character class — returning a local `ErrInvalidSnapshotKey`); best-effort fetch
  the `+refs/loomyard/snapshot/*:refs/loomyard/snapshot/*` namespace (ignore fetch
  failure, degrade to local ref); a **missing** ref reads as `("", nil)`, a
  corrupt/non-repo store surfaces an error. In `read_test.go`, add differential
  tests against `gitrepo.Repo` using `newSnapshotRefFixture`: value parity for a
  set ref, `("", nil)` parity for an absent ref, and a direct git-fixture
  assertion for the `remoteName` `origin` fallback (unexported, so no `gitrepo`
  oracle — assert against a fixture with and without `branch.<b>.remote` set).
  Record the custom-refspec-fetch capability as MIGRATE or CLI-BOUND per the
  observed go-git behaviour.
- **Commit:** `feat(gitnativepoc): SnapshotSHA read + remoteName over go-git with parity tests`

### Card 7: hasUnpushed + isStrictDescendant over go-git

- **Context:**
  - `internal/gitrepo/push.go`
  - `internal/gitrepo/snapshot.go`
  - `internal/gitnativepoc/gitnativepoc.go`
  - `internal/gitnativepoc/harness_test.go`
- **Edits:**
  - `internal/gitnativepoc/read.go`
  - `internal/gitnativepoc/read_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Add unexported `hasUnpushed() (bool, error)` and
  `isStrictDescendant(ancestor, descendant string) bool` on `*Repo` in `read.go`.
  `hasUnpushed` mirrors `gitrepo.hasUnpushed`: count commits in `@{u}..HEAD` via
  go-git's revision walk; **no upstream configured must be treated as unpushed
  (true)**, not an error. `isStrictDescendant` mirrors `gitrepo.isStrictDescendant`:
  `ancestor` reachable from `descendant` **and not equal** (equal commit → false);
  any go-git failure → false. In `read_test.go`, add tests using
  `newBareRemoteFixture` for `hasUnpushed` (ahead-of-upstream → true;
  up-to-date → false; no-upstream → true) and direct git-fixture assertions for
  the `isStrictDescendant` truth table (ancestor→true, equal→false,
  unrelated→false), since both are unexported on `gitrepo` and have no public
  oracle. Record MIGRATE/CLI-BOUND verdicts per test.
- **Commit:** `feat(gitnativepoc): hasUnpushed + isStrictDescendant over go-git with parity tests`

## Batch Tests

`verify` runs the package integration suite
(`go test -tags integration ./internal/gitnativepoc/`) — which now includes the
read-surface parity tests in `read_test.go` — plus the two scoped guard tests.
The read tests build fresh hermetic fixtures per case via the batch-1 helpers and
assert typed-result parity against the `gitrepo.Repo` oracle (or direct git
fixtures for the unexported helpers). A CLI-BOUND op is asserted as an explicit
recorded divergence, not a failure.
