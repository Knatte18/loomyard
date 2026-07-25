# Discussion: board: use gitrepo as its git operator

```yaml
task: 'board: use gitrepo as its git operator'
slug: board-use-gitrepo
status: discussing
parent: main
```

## Problem

`internal/boardengine` still talks to git through hand-rolled `gitexec.RunGit`
call sites. There are two of them, doing overlapping plumbing: `git.go`
(`Pull` fast-forward + `CommitPush` stage/commit/push-with-rebase-retry) and
`sync.go` (the detached `lyx board sync` pusher: `commitDirty` via `add -A`,
plus `pushUnpushed` with its own single-pusher lock, `hasUnpushed` check, and
2-attempt rebase-retry loop). Each reimplements stage/commit/push/rebase-retry
independently by parsing raw git output.

`internal/gitrepo` now exists as the typed `Repo` layer over `gitexec` that
every git-backed consumer was meant to share — and it was built explicitly
anticipating board: its `PushCoalesced` is documented as "the board sync.go
push-loop replacement," and its `rebaseRetryTriggers` set (`non-fast-forward`,
`rejected`, `fetch first`) is "the full trigger set board's sync.go:pushUnpushed
matches." **Why now:** board is the first real production consumer of gitrepo,
landing right after gitrepo itself so any API gap surfaces while the primitive
is still cheap to adjust (before `fabric` also builds on it). This task rewires
board's live git plumbing onto `gitrepo.Repo` and deletes the dead `git.go`
call site. It changes only *how* board talks to git, not *where* board stores
data (that is the separate, `fabric`-dependent `board-weft-storage` redesign).

## Scope

**In:**

- Rewrite `internal/boardengine/sync.go`'s git plumbing onto a single
  `gitrepo.Repo` instance:
  - `commitDirty` → `repo.StageAllAndCommit("board sync")` (new gitrepo method).
  - The push path (`pushUnpushed` + `hasUnpushed` + the `pushLockFile` +
    the 2-attempt retry loop) → `repo.PushCoalesced()`.
- Add **one** new method to `internal/gitrepo`: `StageAllAndCommit(msg string)
  (sha string, committed bool, err error)` — the opt-in `add -A` wildcard-stage
  variant, board's documented exception to gitrepo's explicit-file-list rule.
  Document it as such in `internal/gitrepo/doc.go`.
- Delete `internal/boardengine/git.go` in full: `Pull`, `CommitPush`, and the
  `BoardPushError` type. All three become unreferenced once `sync.go` migrates
  and the dead functions go (`Pull`/`CommitPush` have zero production callers).
- Delete `internal/boardengine/boardtest/git_test.go` (it only tested the
  deleted `Pull`/`CommitPush`).
- Add a gitrepo integration test for `StageAllAndCommit`.
- Documentation lifecycle: fold the durable design into `internal/boardengine`'s
  package doc and update `internal/gitrepo/doc.go` for the new method; delete
  `manifest/designs/board-use-gitrepo.md` (per its own header + the
  Documentation Lifecycle constraint).

**Out:**

- **Board's storage location, branch model, or data format** — that is
  `board-weft-storage.md`, depends on `fabric`, unrelated here.
- **Board's write-lock / render / detached-spawn architecture** — `board.go`'s
  `writeOp`, `spawn.go`'s detached `lyx board sync`, `render.go`, and the
  `writeLockFile` write-serialization are untouched. Only the git-command
  plumbing inside `Sync`/`commitDirty` changes.
- **Board's rebase-retry *behavior*** — it is preserved, just relocated:
  gitrepo's `PushCoalesced`/`Push` already implement the identical policy (1
  push → 1 `pull --rebase` → 1 retry, aborting the rebase on failure).
- **New gitrepo primitives beyond the wildcard method** — no `pull --ff-only`
  method (nothing calls `Pull` anymore), no new rebase surface. gitrepo's
  existing `Push`/`PushCoalesced` already model `pull --rebase` + `rebase
  --abort`, so the design's "expected gitrepo gap" for the push path is already
  filled and requires no new method.
- **`CurrentSHA`/`SHAExists`/`ChangedFilesSince`/snapshot calls** — board tracks
  no snapshot SHA and delegates rebase to gitrepo, so it needs none of these.
  The design mentioned them speculatively; YAGNI.
- **No new CONSTRAINTS invariant** — board is an ordinary project repo (the Weft
  Git Invariant explicitly exempts non-weft repos); nothing enforces gitrepo
  usage for other consumers either, so adding a "board git through gitrepo"
  invariant would be unmatched precedent for low value.

## Decisions

### delete-dead-git.go

- Decision: Delete `git.go` entirely — `Pull`, `CommitPush`, and
  `BoardPushError` — and its test file `boardtest/git_test.go`.
- Rationale: `Pull` and `CommitPush` have **no production callers** (verified
  repo-wide; the only other `Pull(` is `weftengine.Pull`, an unrelated package).
  Only the integration test `git_test.go` exercised them. `BoardPushError` is
  referenced solely by `git.go` and `sync.go`; once `sync.go` propagates
  gitrepo's own errors, it is fully unused. Removing dead code shrinks the
  rewire surface to just `sync.go`.
- Rejected: (a) rewiring `Pull`/`CommitPush` onto gitrepo — keeps dead code
  alive and forces a `pull --ff-only` gitrepo method nobody calls; (b) leaving
  `git.go` untouched — leaves two divergent git-plumbing styles in the package.

### push-path-collapses-onto-PushCoalesced

- Decision: `Sync` adopts `gitrepo.PushCoalesced()` wholesale for the push half.
  Drop board's `pushLockFile` constant, its `pushLock` acquisition, `hasUnpushed`,
  and `pushUnpushed`. `Sync` keeps only its commit loop; each iteration commits
  (under the existing `writeLockFile`) then, unless `skipPush`, calls
  `repo.PushCoalesced()`.
- Rationale: gitrepo's `PushCoalesced` *is* the documented replacement — it
  provides the same cross-process single-pusher coalescing (via its own
  `.gitrepo-push.lock`), the same `hasUnpushed` short-circuit, and the same
  rebase-retry policy board hand-rolls. Board's retry policy is byte-for-byte
  equivalent to gitrepo's, so nothing is lost.
- Consequence (intended, benign): commits (`writeLockFile`) and pushes
  (gitrepo's `.gitrepo-push.lock`) are no longer held under one shared board
  lock across the whole `Sync`. Concurrency is still correct — `writeLockFile`
  serializes commits, gitrepo's lock serializes pushes — and a second concurrent
  `Sync` process still coalesces to a fast no-op. This is slightly more
  concurrent than today, never less safe.
- Consequence (improvement): gitrepo pushes with `-c push.autoSetupRemote=true`,
  so board's very first push on a branch with no upstream now establishes
  tracking instead of relying on pre-existing `@{u}` config.
- Rejected: keeping board's own retry loop and adding primitive `push` /
  `pull --rebase` methods to gitrepo — duplicates logic gitrepo already owns and
  adds rebase surface gitrepo's scope boundaries deliberately exclude.

### wildcard-stage-method-shape

- Decision: Add `func (r *Repo) StageAllAndCommit(msg string) (sha string,
  committed bool, err error)` to `internal/gitrepo`. It does `git add -A` →
  unscoped `git diff --cached --quiet` (0 = nothing staged → return `("", false,
  nil)`; 1 = proceed; else error) → `git commit -m msg` → `CurrentSHA()`.
  Signature and return semantics mirror `StageAndCommit` exactly.
- Rationale: The design mandates a *separate, explicitly-named* wildcard method
  — never a change to `StageAndCommit`'s explicit-file-list contract — so
  "never wildcard" stays the default and this is an opt-in escape hatch. Mirroring
  `StageAndCommit`'s `(sha, committed, err)` shape keeps the two symmetric and
  lets board's `commitDirty` map straight onto it. `commitDirty`'s current
  `status --porcelain` dirty-check is subsumed by the method's `diff --cached
  --quiet` (returns `committed=false` on a clean tree), so board no longer needs
  a separate status probe.
- Documentation: `doc.go`'s Scope-boundaries section must state this is board's
  own exception (board's `Sync`/`commitDirty` path only), **not** a general
  relaxation — `fabric`/`raddle`/`codeintel` keep using the explicit-list
  `StageAndCommit`.
- Rejected: an option/bool on `StageAndCommit` (violates the design's "not a
  change to StageAndCommit's contract"); a differently-named method like
  `CommitAll` (same behavior, less symmetric with the existing name).

### board-only-uses-three-gitrepo-calls

- Decision: Board's migrated code uses exactly `gitrepo.New`,
  `(*Repo).StageAllAndCommit`, and `(*Repo).PushCoalesced`. No `CurrentSHA`,
  `SHAExists`, `ChangedFilesSince`, `Push`, or snapshot calls.
- Rationale: board's push path delegates the rebase to gitrepo and tracks no
  snapshot SHA, so it never needs to inspect what it is racing against. YAGNI.
- Rejected: wiring `CurrentSHA`/`SHAExists` "for later" — unused calls today.

### error-propagation

- Decision: `sync.go` lets gitrepo's typed errors propagate, wrapping with a
  short board-side context string where useful (e.g. `fmt.Errorf("sync commit:
  %w", err)`), and drops the `BoardPushError` string type.
- Rationale: gitrepo already returns descriptive, git-stderr-bearing errors;
  re-stringifying them into `BoardPushError` adds nothing. `Sync`'s callers
  (`board sync` CLI, `b.Sync()`) only check for non-nil, so the concrete error
  type is not part of any contract.
- Rejected: keeping `BoardPushError` — becomes unreferenced once `git.go` is
  deleted.

## Technical context

- **`internal/gitrepo` API** (`gitrepo.go`, `push.go`, `doc.go`):
  - `New(path) *Repo` — no I/O, no validation; wraps an existing checkout.
  - `StageAndCommit(msg, files)` — explicit-list, never wildcards. The new
    `StageAllAndCommit` is its wildcard sibling; add it in `gitrepo.go` next to
    `StageAndCommit`, reusing the same `run` helper and `CurrentSHA`.
  - `Push()` / `PushCoalesced()` (`push.go`) — both push-only; both run
    `pushWithRebaseRetry` (one push, one `pull --rebase` on a
    `rebaseRetryTriggers` rejection, one retry, `rebase --abort` on failure).
    `PushCoalesced` wraps that in a `lock.AcquireWriteLock` on
    `.gitrepo-push.lock` in the repo root + an internal `hasUnpushed` guard.
- **`internal/boardengine/sync.go`** — the file to rewrite:
  - Keep `Sync(boardPath, skipGit, skipPush)`'s outer shape: `skipGit` early
    return, `ensureLockfilesIgnored`, then the commit loop keyed on whether a
    commit was made.
  - `commitDirty` keeps acquiring `writeLockFile` (`tasks.json.lock`) — that
    write-vs-commit serialization is board's, not gitrepo's — but its body
    becomes a single `repo.StageAllAndCommit("board sync")` call returning
    `committed`.
  - Construct one `repo := gitrepo.New(boardPath)` in `Sync` and pass it to
    `commitDirty`; the push step is `if !skipPush { repo.PushCoalesced() }`.
  - Delete `pushUnpushed`, `hasUnpushed`, and the `pushLockFile` const.
- **`.gitignore` / lock-file interaction (important):** board's
  `StageAllAndCommit` runs `add -A`, which would otherwise stage gitrepo's
  `.gitrepo-push.lock`. `ensureLockfilesIgnored` already appends `*.lock` to the
  board dir's `.gitignore` (alongside `*.swaplock` and `renderManifestFile =
  ".board-rendered.json"`), and `.gitrepo-push.lock` matches `*.lock` — so it is
  ignored with no change needed. `ensureLockfilesIgnored` must keep running
  before the first commit. This is exactly the "board wildcard-stages, so board
  owns ignoring gitrepo's lock" situation gitrepo's doc flags (gitrepo itself
  manages no `.gitignore` because its own `StageAndCommit` never wildcards).
- **No import cycle:** `boardengine` already imports `internal/gitexec` and
  `internal/lock`; `gitrepo` imports the same two and never imports
  `boardengine`. No boardengine leaf invariant exists. Adding the `gitrepo`
  import is clean.
- **`board.go` package doc** is where the durable design note lands (the
  detached-sync + gitrepo-operator description), per the Documentation Lifecycle.
- **Callers of `Sync`:** `board.go`'s `(*Board).Sync()` and `boardcli/cli.go`'s
  `board sync` subcommand (line ~544). Both only check for a non-nil error;
  neither depends on `BoardPushError`.

## Constraints

From `CONSTRAINTS.md` (relevant subset):

- **Hermetic Git Test Environment Invariant** — the new gitrepo test spawns git,
  so it must run under `gitrepo`'s existing hermetic `TestMain`
  (`testmain_test.go` already calls `lyxtest.HermeticGitEnv()`; keep the new test
  in a package/build-tag it covers). `boardtest` likewise already has a hermetic
  `TestMain`.
- **Test Tier Purity Invariant** — any test that spawns git (`gitexec.RunGit`,
  `exec.Command`, `lyxtest.Copy*`) must be `//go:build integration`-tagged. The
  new `StageAllAndCommit` test and the migrated `sync_test.go` are integration
  tests; nothing new lands untagged.
- **CLI / Cobra Invariant** — `board sync`'s `Short`
  ("Commit and push pending board changes to the remote") stays accurate; no
  command surface changes, so no help-tree/registration edits.
- **Documentation Lifecycle** — same-commit doc updates: fold durable design into
  `boardengine`'s package doc, update `gitrepo/doc.go` for `StageAllAndCommit`,
  and delete `manifest/designs/board-use-gitrepo.md`. This task does not touch
  the module table in `docs/overview.md` (both modules already exist), and adds
  no roadmap item (it completes a planned one — move it Planned→Done with a
  pointer, per the roadmap rule).
- **Weft Git Invariant** — does not apply: board is an ordinary repo, not the
  weft. No new invariant is added.

## Testing

- **`internal/gitrepo` — `StageAllAndCommit` (new, integration-tagged, TDD
  candidate):** cover (a) a dirty tree with untracked + modified files → one
  commit, all changes captured, returns `(sha, true, nil)`; (b) a clean tree →
  `("", false, nil)`, no commit, no git spawn beyond the checks; (c) that it
  stages files an explicit-list `StageAndCommit` would miss (the whole point of
  `add -A`). Use the same fixture helpers as `gitrepo_test.go`
  (`lyxtest.CopyHostHub` / `CopyWeft`).
- **`internal/boardengine/boardtest/sync_test.go` — migrate as-is, behavior
  unchanged (the safety net):** its existing assertions must all still pass
  against the gitrepo-backed `Sync`:
  - `TestSyncCommitsAndPushes` — remote gains exactly 1 commit; tree clean after.
  - `TestSyncCoalescesBurstIntoOneCommit` — 5 writes → 1 coalesced commit.
  - `TestSyncSkipPushCommitsLocallyOnly` / `TestSkipSeam` — `SkipPush=true`
    commits locally, leaves `@{u}..HEAD` unpushed; `SkipGit=true` is a full
    no-op.
  - `TestSyncCleanTreeIsNoOp` — first sync commits `.gitignore`, second is a
    no-op.
  - `TestSyncIgnoresLockfiles` — `.gitignore` committed; no `.lock`/`.swaplock`
    tracked. This also implicitly guards that gitrepo's `.gitrepo-push.lock` is
    never committed (matches `*.lock`); consider an explicit assertion that
    `.gitrepo-push.lock` is untracked after a push.
- **Delete `boardtest/git_test.go`** — it only tested the removed
  `Pull`/`CommitPush`.
- **Full run:** `go test ./...` plus the integration tier
  (`-tags integration`) must pass, including the CONSTRAINTS enforcement tests
  (hermetic-env, tier-purity, help-tree).

## Q&A log

- **Q:** Delete, rewire, or leave `git.go`'s `Pull`/`CommitPush`? **A:** Delete
  them and their test — zero production callers; `BoardPushError` goes too.
- **Q:** Adopt gitrepo's `PushCoalesced` wholesale, or keep board's retry loop
  and add primitive push/rebase methods to gitrepo? **A:** Adopt `PushCoalesced`
  wholesale; board's retry policy is identical to gitrepo's, and gitrepo's doc
  names this the push-loop replacement.
- **Q:** Shape/name of the new wildcard-stage method? **A:**
  `StageAllAndCommit(msg string) (sha string, committed bool, err error)`,
  mirroring `StageAndCommit`; separate method, never a change to
  `StageAndCommit`'s explicit-list contract; documented as board's exception.
- **Q:** Should board also wire `CurrentSHA`/`SHAExists`? **A:** No — YAGNI;
  board delegates rebase to gitrepo and tracks no snapshot SHA. Only `New` +
  `StageAllAndCommit` + `PushCoalesced`.
- **Q:** Test strategy? **A:** Migrate `sync_test.go` unchanged as the safety
  net, add a gitrepo integration test for `StageAllAndCommit`, delete
  `git_test.go`.
- **Q:** Is the design's "expected gitrepo gap" (`pull --rebase` / `rebase
  --abort` not modeled) real? **A:** No — gitrepo's `Push`/`PushCoalesced`
  already implement that internally; the only additive gitrepo change is
  `StageAllAndCommit`.
- **Q:** Does board's `add -A` risk committing gitrepo's `.gitrepo-push.lock`?
  **A:** No — board's existing `ensureLockfilesIgnored` writes `*.lock`, which
  covers `.gitrepo-push.lock`; keep it running before the first commit.
