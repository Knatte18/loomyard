# Batch: parity-oracle

```yaml
task: 'native clients: migrate gitrepo to go-git + selfreport gh-CLI to go-github'
batch: parity-oracle
number: 2
cards: 5
verify: go test -tags integration -race -count=1 ./internal/gitrepo/...
depends-on: [1]
```

## Batch Scope

Lifts `internal/gitnativepoc`'s differential-parity harness into `internal/gitrepo` **before** any method changes backend, and — the part that is not a copy — writes the test-only CLI oracle the harness loses when it moves.

This is the single most important structural decision in the plan. In the poc, the reference side *is* `gitrepo`'s CLI implementation: `internal/gitnativepoc/read_test.go` calls `gitrepo.New(dir).CurrentSHA()` as the truth to compare against. The moment those methods become go-git, a copied harness compares go-git against go-git — it asserts nothing while still passing green, which is worse than no test because it looks like coverage. So the lift includes writing an independent oracle: raw `gitexec.RunGit` invocations plus the exact output parsing production is about to delete (the `-z` NUL-split, the `--verify --quiet` exit-0/1/other convention, the unborn-HEAD stderr match). That parsing lives on in test code precisely so the thing it validates can stop depending on it.

The tests written here compare the CLI oracle against `gitrepo`'s still-CLI methods, so they pass trivially in this batch. **That is intended and is not dead weight:** they establish the oracle and the case table while both sides are known-good, so that batches 3 and 4 flip one side at a time and any divergence is attributable. Batch 5 adds the falsification pass that proves each case can fail.

Batch-local decision: unexported methods (`remoteName`, `hasUnpushed`, `isStrictDescendant`) cannot be reached from `package gitrepo_test`, so their parity cases live in the internal `package gitrepo` file created in batch 1. Exported-method parity lives in the external test package alongside the existing suite.

## Cards

### Card 5: Test-only CLI oracle

- **Context:**
  - `internal/gitexec/gitexec.go`
  - `internal/gitrepo/gitrepo.go`
  - `internal/gitrepo/snapshot.go`
  - `internal/gitrepo/push.go`
  - `internal/gitnativepoc/harness_test.go`
- **Edits:** none
- **Creates:**
  - `internal/gitrepo/oracle_test.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** Create an integration-tagged file in `package gitrepo_test` holding a CLI-only oracle built directly on `gitexec.RunGit`, independent of any `gitrepo` method. It must reimplement, in test code, the parsing production is deleting: `oracleCurrentSHA` (`rev-parse HEAD`, mapping the `ambiguous argument 'HEAD'` / `unknown revision` stderr match to a sentinel meaning "no commits"), `oracleSHAExists` (`rev-parse --verify --quiet <sha>^{commit}`, exit 0 ⇒ true, exit 1 ⇒ false, anything else ⇒ test failure — the `^{commit}` peel is contractual, not incidental), `oracleChangedFilesSince` (`diff --name-only -z --no-renames <sha> HEAD`, split on NUL, dropping the trailing empty element; `--no-renames` is what keeps a rename reported as delete-plus-add instead of folded into one entry), `oracleCurrentBranch` (`symbolic-ref --short HEAD`), `oracleSnapshotSHA` (`rev-parse --verify --quiet <ref>` against `refs/loomyard/snapshot/<key>`), `oracleRemoteName` (read `branch.<name>.remote`, falling back to `origin`), `oracleHasUnpushed` (`rev-list --count @{u}..HEAD`, with **any** non-zero exit meaning `true`, matching `push.go`'s documented behaviour), and `oracleIsStrictDescendant` (`merge-base --is-ancestor` plus a not-equal check). Each oracle takes an explicit worktree directory so linked-worktree fixtures can address either side. Add a file-level comment stating why this duplication exists — that it is the oracle, and that deleting it in favour of calling `gitrepo` would silently reduce every parity test to a tautology.
- **Commit:** `test(gitrepo): add test-only CLI oracle for differential parity`

### Card 6: Parity harness scaffolding

- **Context:**
  - `internal/gitnativepoc/harness_test.go`
  - `internal/gitnativepoc/read_test.go`
  - `internal/gitrepo/fixtures_test.go`
  - `internal/gitrepo/oracle_test.go`
- **Edits:** none
- **Creates:**
  - `internal/gitrepo/parity_test.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** Create an integration-tagged file in `package gitrepo_test` carrying the harness shape lifted from `internal/gitnativepoc/harness_test.go`: a per-case fixture builder, a comparison helper that reports oracle-vs-implementation divergence with both values, and the repo-shaping helpers the cases need (commit a file, create a branch, write a snapshot ref, add a remote). Preserve the existing convention that `ChangedFilesSince` results are **sorted on both sides** before comparison — output order is not contractual, the method's godoc describes a set of changed paths, and no consumer indexes positionally. Do not lift the poc's assertions yet; this card delivers only the scaffolding the next two cards populate.
- **Commit:** `test(gitrepo): lift differential-parity harness scaffolding from gitnativepoc`

### Card 7: Exported-method parity cases

- **Context:**
  - `internal/gitnativepoc/read_test.go`
  - `internal/gitrepo/oracle_test.go`
  - `internal/gitrepo/fixtures_test.go`
  - `internal/gitrepo/gitrepo.go`
  - `internal/gitrepo/snapshot.go`
- **Edits:**
  - `internal/gitrepo/parity_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Populate the harness with parity cases for the exported migrating methods, each asserted against the oracle on the same fixture. Carry over every case the spike's harness had, because each caught something real: non-ASCII paths in `ChangedFilesSince` (verbatim bytes, no C-quoting layer to strip); a rename reported as delete-plus-add rather than folded; a non-hex SHA returning `ErrInvalidSHA` with no backend call at all; `CurrentSHA` on both a committed repo and an unborn HEAD; `SHAExists` across a real SHA, a well-formed-but-absent SHA, and a non-hex string; `SnapshotSHA` across a set ref, a never-set ref, and an invalid key. Add three cases the spike's harness does **not** cover: (a) `SHAExists` on a **tree or blob** SHA, which must be `false` — the poc's bare `ResolveRevision` does not peel the way `^{commit}` does, and the existing real/absent/non-hex cases cannot see the difference; (b) `CurrentBranch` across all four HEAD states — on a normal branch, on a **detached** HEAD (must return a wrapped error, never an empty string, since a caller that failed to capture a branch has no safe ref to hand `RestoreBranch`), on an **unborn** HEAD (where `git symbolic-ref --short HEAD` exits 0 and prints the branch name even with no commit, so the implementation must return that name and not an error), and on an **orphan** branch from `git checkout --orphan`; (c) `SnapshotSHA`'s three-way distinction — a set ref returns its SHA, a verified-absent ref returns `("", nil)`, and an unreadable store returns an error — since folding the third into the second would tell a consumer "no snapshot" forever.
- **Commit:** `test(gitrepo): add exported-method parity cases`

### Card 8: Unexported-method parity cases

- **Context:**
  - `internal/gitnativepoc/read_test.go`
  - `internal/gitrepo/oracle_test.go`
  - `internal/gitrepo/fixtures_test.go`
  - `internal/gitrepo/push.go`
  - `internal/gitrepo/snapshot.go`
- **Edits:**
  - `internal/gitrepo/gogit_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Add parity cases for `remoteName`, `hasUnpushed`, and `isStrictDescendant` to the internal `package gitrepo` test file, since they are unexported and unreachable from the external test package. Cases: `remoteName`'s `origin` fallback and its configured `branch.<name>.remote` path; `isStrictDescendant` across an ancestor, self (must be false — strict), and an unrelated orphan-branch commit; `hasUnpushed` across **five** states, three of which the spike never covered — HEAD ahead of upstream; HEAD **strictly behind** upstream, which the naive single-hash exclusion gets wrong; **no upstream configured at all**, which must be `true` because that is what makes the first push of a branch happen instead of being skipped as nothing-to-do; **configured but never fetched**; and the **failure-swallowing** path, where the CLI returns `(true, nil)` on any non-zero `rev-list` exit while the poc returns `(false, err)` on a `Head()`/`CommitObject` failure. That last divergence would turn `PushCoalesced` — called from `boardengine/sync.go` and `fabricengine/weftgit.go` — from "attempt the push anyway" into a hard error on an unreadable or unborn-HEAD repo, so the case pins the swallow-into-`true` contract before the implementation moves.
- **Commit:** `test(gitrepo): add unexported-method parity cases`

### Card 9: Linked-worktree parity runs

- **Context:**
  - `internal/gitrepo/fixtures_test.go`
  - `internal/gitrepo/oracle_test.go`
  - `internal/gitrepo/parity_test.go`
- **Edits:**
  - `internal/gitrepo/gogit_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Run the parity cases a second time against the **linked-worktree** fixture from batch 1, covering `CurrentSHA`, `CurrentBranch`, `remoteName`, `SnapshotSHA`'s ref read with the ref written from one worktree and read from the other, `SHAExists`, `ChangedFilesSince`, **and `hasUnpushed` and `isStrictDescendant`**. The last two matter most precisely because they cannot report a problem: both swallow failure (into `true` and `false` respectively), so a common-dir mishandling reaches them as a wrong answer with no error attached, and `isStrictDescendant` is the named victim of the `SetSnapshotSHA` silent-drop path. Include the strictly-behind `hasUnpushed` case and the on-branch and detached `CurrentBranch` cases here. Add a junction-reached run over the same fixture, since that is how lyx addresses these directories in production. The exported-method runs stay in `parity_test.go` if that is the natural home; the unexported ones must be in the internal file. This is not extra thoroughness — the linked worktree is the only topology production runs in, and the standalone `git init` fixtures cannot substitute for it.
- **Commit:** `test(gitrepo): run parity cases against linked-worktree and junction fixtures`

## Batch Tests

`verify:` runs the package's full Tier-2 suite with `-race`, unchanged in scope from batch 1.

New coverage: `internal/gitrepo/oracle_test.go` (the CLI oracle), `internal/gitrepo/parity_test.go` (exported-method cases), and additions to `internal/gitrepo/gogit_test.go` (unexported and linked-worktree cases).

**Every case in this batch passes trivially on completion**, because both sides of each comparison are still the git CLI. State this plainly rather than treating it as a defect: the batch's deliverable is the oracle and the case table, established while both sides are known-good so that batches 3 and 4 flip exactly one side and any divergence has a single possible cause. Batch 5 runs the falsification pass that proves each case can actually fail.
