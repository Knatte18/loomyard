# Batch: migrate-snapshot-push-reads

```yaml
task: 'native clients: migrate gitrepo to go-git + selfreport gh-CLI to go-github'
batch: migrate-snapshot-push-reads
number: 4
cards: 6
verify: go test -tags integration -race -count=1 ./internal/gitrepo/...
depends-on: [3]
```

## Batch Scope

Migrates the remaining local reads, which all live in `snapshot.go` and `push.go` and are all harder than batch 3's because they sit **inside CLI-bound methods**. After this batch the go-git/CLI boundary is call-granular, not method-granular: `SnapshotSHA` keeps its CLI fetch while its ref read moves; `SetSnapshotSHA` stays CLI-bound by contract while losing both of its own `r.run` calls to go-git.

Migrating here: `remoteName`, `SnapshotSHA`'s ref read, `SetSnapshotSHA`'s `^{commit}` canonicalization and its adopted-ref read, `isStrictDescendant`, and `hasUnpushed`. Staying on the CLI: `SnapshotSHA`'s best-effort fetch, `advanceAndPushSnapshotRef`, `adoptSnapshotRef`'s fetch, and every push path — go-git never invokes a git credential helper, and this repo's remote is HTTPS behind Git Credential Manager.

Batch-local decision, applying to **every card in this batch without exception**: each migrated read holds the read lock (`RLock`/`RUnlock`) for the **whole duration of its use of the go-git handle**, not merely across the `goGit()` call, per the contract stated in `goGit`'s godoc. `remoteName` is the case to watch — like `CurrentBranch` it reads an unresolved reference and repository config rather than an object, so the lookup helper does not cover it and the lock must be taken explicitly.

Batch-local decision, and the one that makes the difference between a correct migration and a silent data-loss bug: `SetSnapshotSHA` must **probe the handle explicitly and return the wrapped open error before entering its adopt-on-conflict loop**. It calls `remoteName` (returns a bare `string`) and `isStrictDescendant` (returns a bare `bool`), so a failed open would let it run the loop against fabricated values — `"origin"` and `false` — and return `nil`: a silent no-op on a ref-mutating method. The explicit probe is what makes that failure loud, and it is also what makes the `extensions.worktreeConfig` refusal reach a caller instead of vanishing into a default string.

## Cards

### Card 14: Migrate remoteName

- **Context:**
  - `internal/gitnativepoc/read.go`
  - `internal/gitrepo/gogit.go`
  - `internal/gitrepo/gitrepo.go`
- **Edits:**
  - `internal/gitrepo/snapshot.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Replace `remoteName`'s CLI config read with the go-git implementation lifted from `gitnativepoc`'s `remoteName`: read `plumbing.HEAD` **unresolved**, confirm it is a symbolic reference, and look up `branch.<name>.remote` in the repository config, holding `RLock` across both. Neither is an object lookup, so the fingerprint-gated helper does not apply here and the read lock is the only protection this method gets. Its documented `"origin"` fallback is unchanged and now also absorbs a failed handle open, since the method returns a bare `string` and has nowhere to put an error. That silent degradation is safe in isolation and is made loud at the one call site where it is not — see card 17.
- **Commit:** `refactor(gitrepo): migrate remoteName to go-git`

### Card 15: Migrate SnapshotSHA's ref read

- **Context:**
  - `internal/gitnativepoc/read.go`
  - `internal/gitrepo/gogit.go`
  - `internal/gitrepo/oracle_test.go`
- **Edits:**
  - `internal/gitrepo/snapshot.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** In `SnapshotSHA`, migrate the ref read to go-git and leave the best-effort fetch on the CLI. Note the ordering, which is easy to get backwards: the `remoteName()` call comes **before** the fetch, not after, so after card 14 this method already begins with a go-git call, then runs a CLI fetch, then performs a go-git ref read of state that fetch may have written — a mixed-backend sequence inside one method body. Preserve the three-way outcome exactly: a set ref returns its SHA, a **verified-absent** ref returns `("", nil)`, and an unreadable store returns an error. Collapsing the third into the second is the specific failure this method must not have — on a broken handle it would report every key as absent, with no error, forever, and a consumer would re-process from scratch indefinitely. `validSnapshotKey` still runs first and still yields `ErrInvalidSnapshotKey`. Route the lookup through the batch-1 helper: the CLI fetch immediately preceding it is a named pack-writing site, so this is one of the places the fingerprint gate exists for.
- **Commit:** `refactor(gitrepo): migrate SnapshotSHA ref read to go-git`

### Card 16: Migrate isStrictDescendant

- **Context:**
  - `internal/gitnativepoc/read.go`
  - `internal/gitrepo/gogit.go`
  - `internal/gitrepo/oracle_test.go`
- **Edits:**
  - `internal/gitrepo/snapshot.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Replace `isStrictDescendant`'s `merge-base --is-ancestor` call with the go-git ancestry check lifted from `gitnativepoc`. Keep it **strict**: a commit is not a strict descendant of itself. Every failure, including a failed handle open, degrades to `false` — its documented meaning is "not provably ahead, do not retry" — but route the lookup through the batch-1 helper first, because this is the named victim of the silent-drop path: `SetSnapshotSHA` → `adoptSnapshotRef`'s CLI fetch → `isStrictDescendant` reading a packfile-only object → `false` → `return nil`, silently dropping a strictly-newer snapshot value while reporting success. That is precisely the bug the adopt-on-conflict loop exists to prevent, so the reindex-and-retry must run before the fallback, not instead of it.
- **Commit:** `refactor(gitrepo): migrate isStrictDescendant to go-git ancestry`

### Card 17: Migrate SetSnapshotSHA's local reads and add the handle probe

- **Context:**
  - `internal/gitnativepoc/read.go`
  - `internal/gitrepo/gogit.go`
  - `internal/gitrepo/gitrepo.go`
- **Edits:**
  - `internal/gitrepo/snapshot.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Two changes to `SetSnapshotSHA`, which stays CLI-bound by contract for its push and fetch. First, migrate its two pure local reads: the `^{commit}` canonicalization and the adopted-ref read. The second is byte-identical to `SnapshotSHA`'s ref read from card 15 — leaving it on the CLI would ship the same read in two backends inside one file. Both reads route their object lookups through the batch-1 fingerprint-gated helper — the canonicalization resolves a caller-supplied SHA to a commit, which is exactly the shape that hits a packfile-only object after this method's own CLI fetch. **Preserve the canonicalization's best-effort semantics exactly:** today an unresolvable `sha` is passed through unchanged so that `update-ref` rejects it with git's own error message. go-git's `ResolveRevision` returns an error instead, so the migrated code must swallow a resolution failure and leave `sha` as given — otherwise a case that currently produces git's error becomes a different, earlier failure. Second, add an explicit `r.goGit()` probe at the top of the method that returns the wrapped open error before the adopt-on-conflict loop begins. `remoteName` and `isStrictDescendant` both swallow an open failure into a default, so without this probe a broken handle lets the loop run against `"origin"` and `false` and return `nil` — a silent no-op on a ref-mutating method. This probe is the guaranteed-loud surface for the whole package's open failures.
- **Commit:** `refactor(gitrepo): migrate SetSnapshotSHA local reads and probe handle explicitly`

### Card 18: Migrate hasUnpushed

- **Context:**
  - `internal/gitnativepoc/read.go`
  - `internal/gitrepo/gogit.go`
  - `internal/gitrepo/oracle_test.go`
- **Edits:**
  - `internal/gitrepo/push.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Replace `hasUnpushed`'s `rev-list --count @{u}..HEAD` call with a go-git reachability check. `@{u}` is unusable: go-git's revision parser recognizes the syntax but `ResolveRevision` never implements the `AtUpstream` case, so resolve the upstream ref from branch config instead. The upstream's **full ancestor set** must be walked and passed as `NewCommitPreorderIter`'s `seenExternal` map — seeding only the upstream tip would wrongly report HEAD as ahead whenever HEAD is strictly behind, the second of the two known-wrong shapes the falsification pass proves against. Add a **shortcut before any walk**: when HEAD's hash equals the upstream ref's hash, return `false` immediately, which makes the overwhelmingly common nothing-to-push case a single comparison. Route every object lookup in this method — the `CommitObject` reads behind both the upstream ancestor walk and the HEAD walk — through the batch-1 fingerprint-gated helper. That is not optional here: `hasUnpushed` is one of the three methods the helper exists for, because it swallows failure into `true` and so can never surface an `object not found` on its own, and `PushCoalesced` calls it immediately after CLI pushes that write packs. The failure contract inverts relative to the poc and must follow the **CLI**: a failed handle open, and any go-git failure after a successful open, both return `(true, nil)`, matching the CLI's "any non-zero exit ⇒ true". The poc's `(false, err)` shape would turn `PushCoalesced` from "attempt the push anyway" into a hard failure at `boardengine/sync.go` and `fabricengine/weftgit.go`. The CLI's `(false, err)` spawn-failure branch has no go-git analogue — there is no process to fail to spawn — so it disappears; say so in the godoc rather than leaving the absence unexplained. "No upstream configured" still returns `true`.
- **Commit:** `refactor(gitrepo): migrate hasUnpushed to go-git reachability with equal-hash shortcut`

### Card 19: Mixed-backend interop parity cases

- **Context:**
  - `internal/gitrepo/oracle_test.go`
  - `internal/gitrepo/fixtures_test.go`
  - `internal/gitrepo/gitrepo.go`
  - `internal/gitrepo/snapshot.go`
  - `internal/gitrepo/push.go`
  - `internal/gitrepo/gogit.go`
- **Edits:**
  - `internal/gitrepo/parity_test.go`
  - `internal/gitrepo/gogit_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Add a parity case for each mixed-backend call site, since each is a go-git read of state a CLI write just produced on a handle opened *before* that write. The sites, exhaustively: `StageAndCommit` and `StageAllAndCommit` each end with `r.CurrentSHA()` and must return the SHA the CLI commit actually created; `SetSnapshotSHA` must see the correct remote name and ancestry after its own CLI push; `SnapshotSHA` must read the ref the CLI-side `advanceAndPushSnapshotRef` wrote; `PushCoalesced` must gate on an accurate `hasUnpushed` after a CLI push. Add the hard variant explicitly: a repack (`git gc` or `git repack -ad`) **between** the CLI write and the go-git read, which is what `Push`'s `pull --rebase` retry can trigger in production and what the fingerprint-gated reindex exists to survive. Add one further case for the cross-instance path: hold a warmed handle on one `Repo`, perform the pack-writing write through a **separate** `gitrepo.New` value on the same path, then assert the first handle still resolves the new object. That case fails under a per-instance counter gate and passes under the fingerprint gate, so it is what pins the design rather than merely restating it. Place exported-surface cases in `parity_test.go` and cases touching `hasUnpushed`/`isStrictDescendant` in the internal file.
- **Commit:** `test(gitrepo): cover mixed-backend interop and cross-instance reindex`

## Batch Tests

`verify:` runs `go test -tags integration -race -count=1 ./internal/gitrepo/...`, unchanged in scope.

This batch completes the migrated read surface, so the whole parity table from batch 2 is now live — every case compares a CLI oracle against go-git. The cases that matter most are the ones added in card 19 (mixed-backend, repack-between-write-and-read, cross-instance fingerprint) and the linked-worktree runs of `hasUnpushed` and `isStrictDescendant` from card 9, which are the two methods that cannot report a problem because they swallow failure into `true` and `false`.

`internal/gitrepo/push_test.go`'s existing concurrent `PushCoalesced` case is now driving two goroutines through a shared go-git handle, which is why `-race` is mandatory rather than nice-to-have. Every existing test in the package must still pass untouched; the CLI-bound methods keep their current tests unchanged, and if any of them needs editing, the boundary was drawn wrong.
