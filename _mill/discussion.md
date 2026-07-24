# Discussion: gitrepo — generic, repo-agnostic git primitives

```yaml
task: 'gitrepo: generic, repo-agnostic git primitives'
slug: gitrepo
status: discussing
parent: main
```

## Problem

`internal/gitexec` today is one function — `RunGit(args, cwd) (stdout, stderr, exitCode, err)` —
and every caller that wants a *semantic* git operation (current SHA, changed-files-since,
stage+commit, push, snapshot tracking) parses raw git stdout itself. As `fabric` (the coming
merge of today's `warp`+`weft`), `raddle`, `codeintel`, `webster`, and a future board-on-weft
rewrite all need the same small set of typed operations, that parsing wants to live in one typed
layer instead of being re-implemented per consumer.

**Why now:** `gitrepo` is the **first Planned item** and a hard prerequisite for `fabric` — it is
deliberately built and tested *standalone* before `fabric` (which will hold two `gitrepo.Repo`
instances) is touched, because `fabric`'s correctness (SHA correspondence, snapshot tracking,
`RevertWithWeft`) is only as trustworthy as this primitive layer. See
`manifest/designs/gitrepo.md` and `manifest/designs/fabric.md`.

## Scope

**In:**

- New package `internal/gitrepo` exposing a typed `Repo` over **one** local git checkout, built on
  top of `gitexec.RunGit` (never shelling out directly).
- `Repo` API: `New`, `StageAndCommit`, `Push`, a **coalescing push** mode, `CurrentSHA`,
  `ChangedFilesSince`, `SHAExists`, `SnapshotSHA`, `SetSnapshotSHA`.
- Snapshot tracking stored as git refs under `refs/loomyard/snapshot/<key>`, **pushed to remote**.
- Integration-tagged, hermetic test suite that spawns real git against throwaway repos.
- Design capability requirement: the push surface must be able to **fully replace** board's
  current `git.go`/`sync.go` coalescing logic, so the later board rewrite is a pure
  delete-and-delegate with zero `gitrepo` changes.

**Out:**

- **`gitexec` is NOT merged into `gitrepo`.** `gitexec` stays a zero-dependency leaf with 80
  call-sites across ~13 packages (`hubgeometry`, `warpengine`, `weftengine`, `builderengine`,
  `initengine`, `webster`, `cmd/lyx`, …), several *lower* in the layering than `gitrepo`. Merging
  would force all of them to import the full `Repo` type for a raw git call and give the
  foundational `hubgeometry` an upward dependency. `gitrepo` is one *consumer* of `gitexec`.
- **No repo creation / clone / worktree / checkout / topology.** `New` does not create or clone a
  repo — warp and weft repos exist beforehand. Clone, worktree add/remove, coordinated checkout,
  reconcile, prune, branch naming are **`fabric`'s** job (built on `gitexec` directly), not
  `gitrepo`'s.
- **Not a general-purpose git wrapper:** no rebase, interactive staging, cherry-pick, conflict
  resolution. A human uses plain `git` in the working tree for those.
- **No CLI.** `gitrepo` is a pure library like `gitexec`/`fslink`; no `gitrepocli`, no cobra
  command.
- **The board rewrite is NOT in this task.** `gitrepo` is made *capable* of replacing board's git
  logic; actually rewiring board (delete its `git.go`/`sync.go` coalescing, delegate to `gitrepo`)
  belongs to a later task — naturally the board-on-weft:main migration
  (`manifest/designs/board-weft-storage.md`). Board's `writeLock` domain mutex stays board's.
- **Uncommitted / working-tree changes** are not a `ChangedFilesSince` concern (see Decisions).

## Decisions

### New — lazy wrapper, no validation, no creation

- Decision: `func New(path string) *Repo` just stores the path. No git call, no I/O, cannot fail.
- Rationale: git commands already fail clearly on a bad path; early validation is I/O in a
  constructor for little value and forces every caller to handle an extra error. Repo
  creation/clone is `fabric`'s topology concern, explicitly out of scope.
- Rejected: `New(path) (*Repo, error)` that verifies the path is a git repo.

### gitexec stays a separate leaf layer

- Decision: keep `internal/gitexec` as the raw-exec leaf; `gitrepo` sits on top and is one of its
  consumers. `fabric` will use **both** — `gitrepo` for stage/commit/push/diff semantics, `gitexec`
  directly for topology (clone, worktree add/remove, checkout) that `gitrepo` deliberately omits.
- Rationale: 80 call-sites / ~13 packages, some below `gitrepo` in the layering (`hubgeometry`).
  Merging inverts the layering and couples everything to the `Repo` type.
- Rejected: fold `gitexec` into `gitrepo`.

### StageAndCommit — explicit file list, "nothing to commit" is a signal not an error

- Decision: `StageAndCommit(msg string, files []string) (sha string, committed bool, err error)`.
  Always an explicit file list (`git add -- <files>`), **never** a wildcard/`add -A` stage. When
  the listed files produce no staged change, return `committed=false` with `err=nil` (not a
  failure) — the caller decides whether that is acceptable. On a real commit, `committed=true` and
  `sha` is the new HEAD.
- Rationale: a wildcard stage risks silently committing an unrelated leftover file from an earlier
  interrupted operation. "Nothing to commit" is a normal, expected outcome the caller inspects, not
  an exceptional one; no empty commits enter history.
- Rejected: `--allow-empty` (garbage commits); a hard `ErrNothingToCommit` that forces error
  handling (user preferred a plain signal); silent no-op returning the old SHA (caller can't tell a
  commit happened).

### ChangedFilesSince — committed diff only

- Decision: `ChangedFilesSince(sha string) ([]string, error)` returns `git diff --name-only
  sha..HEAD`, repo-relative paths — committed changes only. Uncommitted working-tree/staged edits
  are excluded.
- Rationale: deterministic and matches the snapshot model, which is entirely SHA-to-SHA.
  Uncommitted changes are a separate concern.
- Rejected: including working-tree/staged changes (answer changes on every edit; not reproducible).

### SHAExists — bool, swallows git failure as "false"

- Decision: `SHAExists(sha string) bool`. A git failure or a missing SHA both yield `false`, which
  callers treat as a staleness signal → rebuild/re-sync.
- Rationale: the caller's response is identical either way ("when in doubt, rebuild"), so the extra
  error return buys nothing. Extends "advance state only on confirmed success" to cover "the ground
  truth moved out from under us" (rebase/amend/force-push invalidating a stored SHA).
- Rejected: `SHAExists(sha) (bool, error)` distinguishing "absent" from "check failed".

### Snapshot tracking — git refs under refs/loomyard/snapshot/<key>, pushed to remote

- Decision: `SnapshotSHA(key string) (string, error)` and `SetSnapshotSHA(key, sha string) error`
  store the value as `refs/loomyard/snapshot/<key>`. Refs are **pushed to remote** so snapshot
  state is shared across clones/machines, not just across worktrees of one clone. `SnapshotSHA`
  returns `("", nil)` when no ref exists yet; a real git failure is an error.
- Rationale: refs are the natural, self-correcting store — a consumer only calls `SetSnapshotSHA`
  after confirmed success, so a partial failure leaves the ref un-advanced and the next attempt
  recomputes from the old SHA (catches everything missed). Remote sharing lets multiple clones
  coordinate the same per-consumer tracking.
- Rejected: local-only refs (would not share across clones — user chose remote); a separate mapping
  file (drifts from history, doesn't travel with clones).

### Snapshot remote sync — fast-forward-only, adopt-on-conflict; read fetches first

- Decision: `SetSnapshotSHA` pushes the ref fast-forward-only; on rejection (another clone already
  advanced the same key), fetch and **adopt** the remote value with no error. `SnapshotSHA` fetches
  the snapshot refs before reading so it reflects other clones' advances.
- Rationale: a key advances monotonically forward through history; a rejected push means someone
  processed further, so their SHA is the correct one to take. Self-correcting, no manual conflict
  resolution.
- Rejected: force-push last-writer-wins (can drag another consumer's progress backwards); treat the
  local ref as sole truth with fetch/push as a separate caller-driven step.

### Snapshot key validation — reject invalid keys

- Decision: validate `key` before it becomes part of a ref name; reject keys containing ref-illegal
  characters (whitespace, `~ ^ : ? * [ \`, `..`, leading/trailing `/`, etc.) with a clear error.
  Legitimate keys like `codeintel-go`, `codeintel-py`, `raddle` pass.
- Rationale: an unvalidated key produces a corrupt or colliding ref. Fail loudly before git makes a
  mess.
- Rejected: silent sanitisation (hides mistakes, risks two keys collapsing to one ref).

### Push surface — plain Push + a coalescing pusher; detachment is the caller's job

- Decision: two push entry points on `Repo`:
  1. `Push() error` — a single synchronous push with **rebase-retry resilience** (on
     non-fast-forward: `pull --rebase`, one retry, `rebase --abort` on failure). Used by
     `fabric`/`raddle`/`webster`/`codeintel`, which push deliberately and need no coalescing.
  2. A **coalescing pusher** — takes a single-pusher lock, then loops "commit any pending
     work / push anything unpushed" until the tree is clean and nothing is unpushed, with the same
     rebase-retry. This is the board-shaped "no stacking" push: a burst of writes coalesces into as
     few pushes as possible because a second pusher blocks on the lock, finds nothing to do, and
     exits. This mode must be able to fully replace board's `sync.go`.
  The **detachment** (fire-and-forget so a short-lived CLI caller neither blocks on a slow push nor
  dies before it finishes) stays at the **CLI layer** via the existing `internal/proc` `Detach`,
  because it must re-exec a concrete command (`lyx board sync`, later `lyx fabric sync`) that a
  generic library cannot know.
- Rationale: mirrors how board is already structured — engine in `sync.go`, detachment in
  `spawn.go` via `proc.Detach`. `gitrepo` owns the pure, testable coalescing engine; process
  spawning stays where a re-invocable command exists. An in-process goroutine is wrong: a
  short-lived CLI caller exits before the goroutine's push completes.
- Rejected: `PushDetached(argv []string)` inside `gitrepo` (leaks the "there is a re-invocable sync
  command" assumption into a generic library); in-process goroutine (dies with the parent).

### Lock ownership — push-coalescing lock in gitrepo, domain write-mutex in the consumer

- Decision: the **single-pusher lock** (coalescing) lives in `gitrepo`'s coalescing pusher; its
  lock file lives in the **worktree root** (auto-added to `.gitignore`), landing — for the future
  board case — in the single `_board` worktree that always has weft:main checked out and through
  which all board interaction flows. The **domain write-mutex** that serialises a consumer's data
  mutations against the commit snapshot (board's `tasks.json.lock`) stays in the **consumer**
  (board), not in `gitrepo`. Consumers other than board need neither lock by default.
- Rationale: coalescing is a git-push concern (gitrepo); protecting a consumer's own data files
  mid-mutation is that consumer's concern (board). Keeps `gitrepo` free of board-domain knowledge.
- Rejected: putting the domain mutex in `gitrepo`; `.git/`-internal lock location (user chose the
  worktree root, aligned with the `_board` worktree model).

### CurrentSHA on an empty repo — typed error

- Decision: `CurrentSHA() (string, error)` returns a typed `ErrNoCommits` when HEAD has no commit
  yet.
- Rationale: no SHA exists; forcing the caller to handle it beats returning an ambiguous empty
  string.
- Rejected: `("", nil)`.

### ChangedFilesSince against a missing SHA — error

- Decision: if `sha` no longer exists (rebased/invalid), `ChangedFilesSince` returns an error;
  callers check `SHAExists` first and treat a missing SHA as staleness → full rebuild.
- Rationale: consistent with the self-correcting staleness model.
- Rejected: interpreting a missing SHA as "everything changed" (diff against empty tree).

### Commit identity — use the repo's configured identity

- Decision: `StageAndCommit` uses the repo's configured git identity; `gitrepo` sets no override.
  If identity is unset, git fails and the error surfaces.
- Rationale: least surprise; under `HermeticGitEnv` the identity is pinned in tests.
- Rejected: `gitrepo` stamping a fixed loomyard identity on every commit.

### Concurrency — Repo is not goroutine-safe for writes; document, no mutex

- Decision: methods on a single `Repo` are not goroutine-safe for concurrent writes to the same
  repo. Cross-process push serialisation is handled by the coalescing pusher's single-pusher lock;
  in-process callers serialise their own writes. Documented in the package doc; no per-`Repo`
  mutex.
- Rationale: matches board; keeps the type simple. The only real concurrency hazard (concurrent
  pushers) is already covered by the push lock.
- Rejected: a built-in per-`Repo` mutex on write methods.

## Technical context

- **`internal/gitexec`** (`gitexec.go`): `RunGit(args []string, cwd string) (stdout, stderr string,
  exitCode int, err error)`. Non-zero git exit is **not** a Go error (err stays nil, exitCode
  carries it); only spawn failures return a non-nil err with exitCode -1. `gitrepo` builds every
  operation on this and interprets exitCode/stderr itself.
- **`internal/boardengine`** is the reference implementation for the coalescing pusher — read it,
  do not re-invent:
  - `git.go`: `Pull` (`pull --ff-only`), `CommitPush` (stage explicit paths → `diff --cached
    --quiet` to detect no-op → commit → push with one rebase-retry on `non-fast-forward`/`rejected`).
    `BoardPushError` is a `string` error type.
  - `sync.go`: `Sync` loops `commitDirty` + `pushUnpushed` under `pushLock` until clean; `hasUnpushed`
    uses `rev-list --count @{u}..HEAD` (no upstream ⇒ treat as unpushed so the first push sets it);
    `commitDirty` runs under `writeLock` (**board-domain**, stays in board). Board's `Sync` uses
    `add -A` — `gitrepo`'s coalescing pusher must instead take an explicit file set (see the
    StageAndCommit decision) rather than wildcard-staging.
  - `spawn.go`: `spawnSync` re-execs `lyx board --board-path <abs> sync` via `proc.Detach`, `Start()`
    without `Wait()` — the detachment pattern that stays at the CLI layer.
  - Locks via `internal/lock` (`flock.AcquireWriteLock` → `Release`).
- **`internal/lock`** (`flock`): the file-lock helper the coalescing pusher's single-pusher lock uses.
- **`internal/proc`**: `HideWindow` (used by `gitexec`), `Detach` (used by the CLI layer for
  fire-and-forget).
- **Snapshot ref plumbing:** write via `git update-ref refs/loomyard/snapshot/<key> <sha>`, read via
  `git rev-parse`/`show-ref`, push/fetch the `refs/loomyard/snapshot/*` namespace explicitly
  (custom refs are not pushed/fetched by default refspecs).
- **`manifest/designs/gitrepo.md`** and **`manifest/designs/fabric.md`** hold the durable design
  rationale; per the documentation lifecycle, when `gitrepo` lands its rationale folds into the
  `internal/gitrepo` package doc and `gitrepo.md` is deleted.

## Constraints

From `CONSTRAINTS.md`:

- **Hermetic Git Test Environment Invariant** — `gitrepo`'s test package spawns git, so it **must**
  contain a `TestMain` calling `lyxtest.HermeticGitEnv()` before `m.Run()`. Machine-checked
  (presence scan across every test file).
- **Test Tier Purity Invariant** — untagged tests must not spawn git. `gitrepo`'s git-spawning
  tests must carry `//go:build integration` (matching `gitexec_test.go`) so plain `go test` (Tier 1)
  stays offline/fast.
- **Weft Git Invariant** — weft-internal git goes through `weftengine`/`warpengine` (later
  `fabric`), driven by Go in-process, never by an agent. `gitrepo` itself is repo-agnostic and this
  task touches **no** weft repo — its tests run against throwaway hermetic repos — so the invariant
  is not engaged here; it constrains `gitrepo`'s *consumers* (`fabric`), which own weft git.
- **CLI / Cobra Invariant** — does **not** apply: `gitrepo` registers no cobra command (pure library,
  like `gitexec`/`fslink`). No `Command()`/`RunCLI`/`Short`/help-tree obligations.
- **Documentation Lifecycle** — landing `gitrepo` folds `manifest/designs/gitrepo.md` into the
  package doc and deletes the manifest file; move the roadmap's `gitrepo` item Planned → Done in the
  same commit series (see `CLAUDE.md` task-completion rule).

No new cross-cutting invariant is introduced by this task.

## Testing

**Package:** `internal/gitrepo`, external test package (`package gitrepo_test`), all git-spawning
tests `//go:build integration`, with a `TestMain` calling `lyxtest.HermeticGitEnv()` before
`m.Run()` (Hermetic Git Test Environment + Test Tier Purity invariants). Tests build throwaway
repos with real `git init`/commits — never touch a real warp/weft/board repo.

**TDD candidates (pure-logic, worth writing test-first):**

- Snapshot key validation (accept `codeintel-go`/`raddle`; reject whitespace, `~^:?*[\`, `..`,
  leading/trailing `/`).
- The "nothing to commit" signal path of `StageAndCommit` (`committed=false, err=nil`).

**Scenarios that must be covered (integration, real git):**

- `New` + `CurrentSHA`: on a repo with commits (returns HEAD); on an empty repo (`ErrNoCommits`).
- `StageAndCommit`: commits exactly the listed files and returns the new SHA; leaves an unlisted
  dirty file uncommitted; returns `committed=false` when listed files are unchanged; never
  wildcard-stages.
- `ChangedFilesSince`: correct set for `sha..HEAD`; empty when `sha == HEAD`; excludes uncommitted
  edits; errors on a missing `sha`.
- `SHAExists`: true for a real SHA, false for a fabricated/removed one, false (not panic) on git
  failure.
- `SnapshotSHA`/`SetSnapshotSHA`: round-trip through `refs/loomyard/snapshot/<key>`; `("", nil)`
  before any set; remote push on set; fast-forward-only with adopt-on-conflict when a second clone
  advanced the same key; read reflects a remote advance after fetch. Use two clones sharing a bare
  remote as the fixture.
- Plain `Push`: succeeds fast-forward; recovers via one rebase-retry on a non-fast-forward; surfaces
  a genuine push failure.
- Coalescing pusher: a burst of pending changes collapses to as few pushes as possible; a second
  concurrent pusher blocks on the single-pusher lock, finds nothing to do, and exits; loops to catch
  a change that lands mid-push; uses an explicit file set, not `add -A`; must be shown sufficient to
  replace board's `sync.go` behaviour.

## Q&A log

- **Q:** Should `New` create/clone a repo, or validate the path? **A:** Neither — lazy wrapper.
  Warp/weft exist beforehand; creation/clone is `fabric`'s topology job, out of scope.
- **Q:** Is `gitexec` made redundant / should it merge into `gitrepo`? **A:** No — keep it a
  zero-dep leaf (80 call-sites, ~13 packages, some below `gitrepo`); `gitrepo` and `fabric` are
  consumers, `fabric` uses both.
- **Q:** `StageAndCommit` with a no-op file set? **A:** Return a "nothing to commit" signal
  (`committed=false, err=nil`), not an error, not an empty commit, not a silent old-SHA no-op.
- **Q:** What counts as "changed" in `ChangedFilesSince`? **A:** Committed diff `sha..HEAD` only;
  uncommitted changes are out.
- **Q:** `SHAExists` error handling? **A:** Return bare `bool`; git failure ⇒ `false` (staleness).
- **Q:** Snapshot refs local-only or pushed? **A:** Pushed to remote (shared across clones).
- **Q:** Remote snapshot conflict / read model? **A:** Fast-forward-only push, adopt remote value on
  rejection (no error); read fetches snapshot refs first.
- **Q:** Coalescing-pusher lock file location? **A:** Worktree root (+ auto-gitignore); for board it
  lands in the single `_board` worktree (weft:main) all board interaction flows through.
- **Q:** Does `gitrepo` get a CLI? **A:** No — pure library like `gitexec`/`fslink`.
- **Q:** Test build tag? **A:** `//go:build integration` + `TestMain` → `HermeticGitEnv()`, matching
  `gitexec`.
- **Q:** Snapshot `key` validation? **A:** Validate and reject ref-illegal keys with a clear error;
  no silent sanitisation.
- **Q:** `CurrentSHA` on an empty repo? **A:** Typed `ErrNoCommits`.
- **Q:** `ChangedFilesSince` against a missing SHA? **A:** Error; caller checks `SHAExists` first.
- **Q:** `Repo` thread-safety? **A:** Not goroutine-safe for concurrent writes; push lock covers the
  real hazard; document, no mutex. The domain write-mutex is the consumer's (board's), not
  `gitrepo`'s.
- **Q:** Commit identity? **A:** Use the repo's configured identity; no override.
- **Q:** Is rewriting board to use `gitrepo` in scope? **A:** No — land `gitrepo` standalone +
  tested, but design its push surface so it can fully replace board's git logic; the board rewrite
  is a later task (board-on-weft:main migration). Avoids permanent duplication without breaking the
  standalone landing.
