# Batch: migrate-core-reads

```yaml
task: 'native clients: migrate gitrepo to go-git + selfreport gh-CLI to go-github'
batch: migrate-core-reads
number: 3
cards: 4
verify: go test -tags integration -race -count=1 ./internal/gitrepo/...
depends-on: [2]
```

## Batch Scope

Switches the four read methods in `internal/gitrepo/gitrepo.go` — `CurrentSHA`, `SHAExists`, `CurrentBranch`, `ChangedFilesSince` — from `gitexec` to go-git. This is the first batch where the parity harness from batch 2 stops being a tautology: the oracle stays on the CLI while the implementation moves, so every case from cards 7 and 9 becomes a live differential assertion.

Three of the four implementations are lifted from `internal/gitnativepoc/read.go` rather than reimplemented. `CurrentBranch` is the exception and must be written from scratch — it is one of the five methods the spike never examined, has no counterpart in the poc, and is the one method where the obvious implementation is wrong.

Batch-local decision, applying to **every card in this batch without exception**: each migrated method must hold the read lock (`RLock`/`RUnlock`) for the **whole duration of its use of the go-git handle**, not merely across the `goGit()` call. `goGit`'s write `Lock` covers only the cache-check-and-open step and hands back an unprotected handle, so a caller that reads outside the read lock races a concurrent open or reindex elsewhere. This applies to reads that never touch an object at all — `CurrentBranch` reads an unresolved reference directly and so is not covered by the lookup helper. `goGit`'s godoc states the obligation; the cards below rely on it rather than restating it per line.

Batch-local decision: every object lookup goes through batch 1's fingerprint-gated helper, not through a direct storer call. That is not a style preference — `SHAExists` swallows every failure into `false`, so an `object not found` never escapes it and the reactive reindex would be structurally unreachable from exactly the method most likely to need it.

No method's exported signature, error sentinels, or documented behaviour change. `Repo.run` stays byte-unchanged and keeps serving the CLI-bound methods.

## Cards

### Card 10: Migrate CurrentSHA

- **Context:**
  - `internal/gitnativepoc/read.go`
  - `internal/gitrepo/gogit.go`
  - `internal/gitrepo/oracle_test.go`
- **Edits:**
  - `internal/gitrepo/gitrepo.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Replace `CurrentSHA`'s `r.run("rev-parse", "HEAD")` body with a go-git read via `r.goGit()`, lifting the shape from `gitnativepoc`'s `CurrentSHA`. The error contract is exact and must not drift: an unborn HEAD returns `ErrNoCommits`, which now comes from mapping `plumbing.ErrReferenceNotFound` instead of matching git's English `ambiguous argument 'HEAD'` / `unknown revision` stderr; every other failure returns a plain wrapped error. This is a `Head()` **ref** read, not an object lookup, so it does **not** go through the batch-1 helper — refs are never cached by go-git and go to disk on every call, which is why the fingerprint gate has nothing to protect here. It does need the read lock held across the call, like every other use of the handle. Update the method's godoc to say the unborn-HEAD detection is now typed rather than message-matched — that is a real change in robustness (it stops depending on git's locale) and the package doc's locale paragraph is rewritten against it in batch 9.
- **Commit:** `refactor(gitrepo): migrate CurrentSHA to go-git`

### Card 11: Migrate SHAExists

- **Context:**
  - `internal/gitnativepoc/read.go`
  - `internal/gitrepo/gogit.go`
  - `internal/gitrepo/doc.go`
- **Edits:**
  - `internal/gitrepo/gitrepo.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Replace `SHAExists`'s `rev-parse --verify --quiet <sha>^{commit}` call with a go-git lookup. Two contracts must survive exactly. First, the `validSHA` guard runs **before** any backend is reached, so a non-hex string returns `false` with no repository access at all — it is also the source of the typed `ErrInvalidSHA` sentinel elsewhere, so it is contract, not merely defence. Second, the lookup must **peel to a commit**, not merely resolve: the CLI's `^{commit}` suffix is the whole point of the method, so a tree or blob SHA must be `false`. The poc's bare `ResolveRevision` does not peel this way and is the known-wrong shape here. Every failure — including a failed handle open — swallows into `false`, which the method's godoc already documents. Route the lookup through the batch-1 helper so a packfile-only object gets its one reindex-and-retry before the swallow; this method is documented as a cheap staleness probe callers run *expecting* absence, which is exactly why the fingerprint gate rather than an unconditional reindex is what keeps it cheap.
- **Commit:** `refactor(gitrepo): migrate SHAExists to go-git with commit peeling`

### Card 12: Write CurrentBranch on go-git

- **Context:**
  - `internal/gitnativepoc/read.go`
  - `internal/gitrepo/gogit.go`
- **Edits:**
  - `internal/gitrepo/gitrepo.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Rewrite `CurrentBranch` on go-git. There is no poc implementation to lift and the obvious one is wrong: `Repository.Head()` **resolves** HEAD and therefore succeeds on a detached HEAD, whereas this method contracts on returning a wrapped **error** when HEAD is detached — a caller that failed to capture a branch name has no safe ref to hand `RestoreBranch`. The correct shape already exists in the poc under another name: `remoteName` reads `r.repo.Reference(plumbing.HEAD, false)` **unresolved** and checks `head.Type() != plumbing.SymbolicReference`. `CurrentBranch` uses that same unresolved read and returns `head.Target().Short()`. Because that read touches no object it does **not** go through the lookup helper, which makes it one of the two places where the read lock has to be taken explicitly — hold `RLock` across the whole `Reference` call, per `goGit`'s documented contract. Because the read is unresolved it also returns the branch name on an **unborn** HEAD, matching `git symbolic-ref --short HEAD`, which exits 0 and prints the name even with no commit — do not add an existence check that would break that. Record in the method's godoc that this method touches only one unresolved reference and therefore passes even on a completely broken handle, so it must never be used as a smoke test for the migration.
- **Commit:** `refactor(gitrepo): migrate CurrentBranch to go-git via unresolved HEAD`

### Card 13: Migrate ChangedFilesSince

- **Context:**
  - `internal/gitnativepoc/read.go`
  - `internal/gitrepo/gogit.go`
  - `internal/gitrepo/parity_test.go`
- **Edits:**
  - `internal/gitrepo/gitrepo.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Replace `ChangedFilesSince`'s `diff --name-only -z --no-renames` call and NUL-splitting with a go-git tree diff, lifting `gitnativepoc`'s implementation including its `commitTree` helper. The critical detail: it must call `object.DiffTree` **directly**. `Tree.Diff` and `DiffTreeWithOptions` perform rename detection by default since go-git v5.1.0, which folds a rename into a single entry and loses the old path — precisely what the CLI's `--no-renames` flag exists to prevent, and one of the two known-wrong shapes the falsification pass in batch 5 proves against. `ErrInvalidSHA` must still be returned from the `validSHA` guard before anything is resolved. Non-ASCII paths come back as verbatim bytes with no C-quoting layer to strip, which is a simplification over the CLI path, not a behaviour change. Route object lookups through the batch-1 helper. State in the godoc that the returned order is **not contractual** — the method describes a set of changed paths and no consumer indexes positionally; the parity comparison sorts both sides deliberately, as a documented choice rather than an accident of the harness.
- **Commit:** `refactor(gitrepo): migrate ChangedFilesSince to go-git DiffTree`

## Batch Tests

`verify:` runs `go test -tags integration -race -count=1 ./internal/gitrepo/...`, unchanged in scope.

The batch's real test surface is the harness built in batch 2, which becomes meaningful here: `internal/gitrepo/parity_test.go`'s exported-method cases and `internal/gitrepo/gogit_test.go`'s linked-worktree runs now compare a CLI oracle against a go-git implementation. Every case from cards 7 and 9 that touches these four methods is a live assertion as of this batch — in particular the tree/blob `SHAExists` case, the four `CurrentBranch` HEAD states, and the rename-not-folded `ChangedFilesSince` case.

The package's existing tests (`gitrepo_test.go`, `snapshot_test.go`, `push_test.go`, `pull_test.go`, `reset_test.go`, `keyvalidation_test.go`) must all keep passing **untouched**. These four methods' public behaviour is unchanged, so an existing test needing an edit is evidence the contract drifted, not evidence the test was wrong.
