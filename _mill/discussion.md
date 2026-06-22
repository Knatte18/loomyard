# Discussion: Adopt internal/state in board and muxpoc

```yaml
task: Adopt internal/state in board and muxpoc
slug: adopt-internal-state
status: discussing
parent: main
```

## Problem

`internal/state` was extracted to provide generic locked typed JSON I/O —
`WriteJSON[T]` and `ReadJSON[T]` do exactly "create parent dirs → acquire file
lock → marshal/unmarshal → atomic write". But two existing call sites still
hand-roll that same triplet:

- `internal/board/store.go` — `Save` (manual `json.MarshalIndent` +
  `fsx.AtomicWriteBytes` under a swap lock) and `Load` (read lock + `os.ReadFile`
  + `json.Unmarshal`).
- `internal/muxpoc/state.go` — `SaveState` (manual marshal + `fsx.AtomicWrite`
  under a write lock) and `LoadState` (read lock + `os.ReadFile` + `json.Unmarshal`).

This is accidental duplication of the persistence mechanics. `internal/state`
currently has **no production callers** — it is used only by its own tests.

**Why now:** the extraction already happened; leaving the two original sites
hand-rolling the same logic defeats the point of having the module. This is a
cleanup task, but it is *not* a mechanical find-and-replace: two real
incompatibilities (below) shape the work, and the operator has decided to use
this cleanup to also remove the error-swallowing both sites currently do.

## Scope

**In:**

- Extend `internal/state` so `WriteJSON`/`ReadJSON` take an **explicit lock
  path** (caller names the lock file), instead of hardcoding `<path>.lock`.
- Rewrite `board.Store.Save` and `board.Store.Load` to call
  `state.WriteJSON` / `state.ReadJSON`, passing the existing swap lock
  (`tasks.json.swaplock`).
- Collapse `Store.Save(boardPath, relPath)` to `Store.Save()` — it currently
  ignores `s.filePath` and re-joins the same path the constructor was given.
  Update the single caller (`board.go`).
- Rewrite `muxpoc.SaveState` and `muxpoc.LoadState` to call
  `state.WriteJSON` / `state.ReadJSON`.
- Change corruption handling at **both** read sites so a corrupt JSON file
  surfaces as an **error** (no longer swallowed). This is the operator's
  explicit instruction: "swallow of error is not acceptable."
- Change `muxpoc.LoadState`'s signature from `(*MuxpocState, string, error)` to
  `(*MuxpocState, error)` — the `warn` string channel existed only for the
  corruption case, which is now an error, so it becomes vestigial. Update all
  six callers.
- Update affected tests (`state_test.go`, `muxpoc/state_test.go`, and any board
  test that asserts on corruption/lock-file behavior) to match the new
  error-surfacing behavior.

**Out:**

- `muxpoc.DeleteState` — no marshal/lock-write involved; left untouched.
- The board **coarse** write lock (`tasks.json.lock`) held by
  `board.writeOp` — it stays exactly as-is; only the inner swap lock moves into
  `state`. (See Decision *board-two-lock-design*.)
- The board read ops bypassing the coarse lock — unchanged; reads still take
  only the swap lock.
- `fsx.AtomicWrite` / `fsx.AtomicWriteBytes` — unchanged. `state` already builds
  on `AtomicWriteBytes`.
- `internal/lock` — unchanged.
- No new behavior beyond the persistence swap and the corruption-as-error
  change. The DependsOn nil→`[]` normalization in board.Load is preserved.
- No change to file paths, on-disk JSON format, or the wiki/board CLI contract.

## Decisions

### state-gets-explicit-lock-path

- Decision: change `state.WriteJSON` / `state.ReadJSON` to accept the lock-file
  path explicitly, e.g. `WriteJSON[T any](path, lockPath string, v T) error`
  and `ReadJSON[T any](path, lockPath string) (T, bool, error)`. Each caller
  passes its lock path: board passes `<tasks.json>.swaplock`, muxpoc passes
  `<muxpoc-state.json>.lock`.
- Rationale: board **requires** a lock file distinct from `<path>.lock`. Its
  coarse write lock is literally `tasks.json.lock`, held by `writeOp` across
  Load → mutate → Save. If `state` hardcoded `<path>.lock`, `Save` would
  re-acquire the coarse lock the caller already holds → **self-deadlock**, and
  routing `Load` through it would force readers onto the coarse lock, regressing
  the read-latency design. An explicit lock path is the minimal change that lets
  board keep its two-lock design while still reusing state's marshal +
  atomic-write + lock core. It is cheap because `state` has no production
  callers — only its own tests update.
- Rejected:
  - *Hardcode `<path>.lock` (status quo) and drop board from scope* — leaves the
    board duplication the task was created to remove.
  - *Keep `WriteJSON(path,v)` default + add `WriteJSONLocked(path,lockPath,v)`*
    — two functions doing the same thing; more API surface than a single
    explicit param for the sake of one redundant `<path>.lock` at the muxpoc
    call site.
  - *Functional options (`WithLockPath`)* — over-engineered for 4 call sites in
    a module with two functions.

### board-two-lock-design

- Decision: only the **swap lock** moves into `state`. The coarse
  `tasks.json.lock` (acquired in `board.writeOp`, `board.go:50`) stays where it
  is and is untouched. board.Save/Load pass `s.filePath + ".swaplock"` as the
  lock path to state.
- Rationale: the two locks have different jobs. The coarse lock serializes
  *writers* across the whole Load→mutate→Save→git cycle. The swap lock is
  fine-grained — it fences *readers* against the rename instant only, so reads
  (which take **only** the swap lock, never the coarse one) wait microseconds
  instead of the full git round-trip. Collapsing them would either deadlock or
  make every read block on the coarse lock.
  `TestConcurrentReadsDuringUpserts` and `TestConcurrentUpsertsDoNotLoseWrites`
  (in `internal/board/boardtest/concurrency_test.go`) are the guardrails: a
  naive swap that re-uses `<path>.lock` would hang them.
- Rejected: *single lock for board* — breaks the read-bypasses-write-lock design
  and the concurrency tests.

### corruption-surfaces-as-error

- Decision: a corrupt/unparseable JSON file is returned as an `error` at both
  read sites. board.Load no longer falls back to an empty task list on parse
  error; muxpoc.LoadState no longer returns a warning string for a corrupt
  file. This matches `state.ReadJSON`'s existing philosophy ("corruption is not
  swallowed").
- Rationale: operator instruction — "swallow of error is not acceptable." A
  silent empty board or a corrupt-but-ignored session file hides real data loss.
  Failing loudly forces the corrupt file to be dealt with (deleted via `down` /
  `DeleteState`, or investigated) rather than masked.
- Rejected:
  - *Preserve swallow/warn behavior by wrapping ReadJSON* — directly contradicts
    the operator constraint, and `ReadJSON`'s single-error model cannot cleanly
    reproduce the old read-error-vs-corrupt split anyway.
  - *Enhance ReadJSON with a typed corruption sentinel so callers can still
    swallow* — pointless given corruption must now surface regardless.

### muxpoc-loadstate-signature

- Decision: `LoadState` becomes `func LoadState(cwd string) (*MuxpocState, error)`.
  The `warn string` return is removed.
- Rationale: `warn` was non-empty only in the corruption case (`state.go:74`).
  With corruption now an error, `warn` would always be `""` — dead weight.
  Removing it is cleaner than threading a perpetually-empty value through six
  callers.
- Behavioral consequence to call out for the plan:
  - `cmdUp` (`up.go`): previously printed the warn and continued with
    `state == nil`, spawning a fresh session over a corrupt file. Now it returns
    an error and aborts. Intended (no silent overwrite of a corrupt file).
  - `daemon` poll loop (`daemon.go`): previously printed warn and continued;
    now the corrupt case hits the existing `if err != nil { …; continue }`
    branch — it logs and retries on the next tick. Behavior is effectively
    preserved (log + keep polling).
  - Remaining callers (`attach.go`, `down.go`, `review.go`, `status.go`,
    `muxpoc_smoke_test.go`) use `_` for warn today and just need the arity
    update.
- Rejected: *keep the 3-return signature with warn always `""`* — vestigial API.

### muxpoc-lock-filename-moves

- Decision: muxpoc's lock file moves from `.lyx/muxpoc-state.lock` to
  `.lyx/muxpoc-state.json.lock` (i.e. `<statePath> + ".lock"`), and `SaveState`
  + `LoadState` adopt it **together**. Remove the now-unused `lockRelPath`
  constant.
- Rationale: state derives the lock from the data path; the natural muxpoc lock
  is `<statePath>.lock`. Save and Load must use the *same* lock file or the
  read/write fence breaks — so both move in lockstep. No external referrers to
  `muxpoc-state.lock` exist (grep confirms the constant is referenced only
  inside `state.go`), so the rename is safe. The `.lyx/` lock file is ephemeral
  and gitignored, not part of any contract.
- Rejected: *keep `muxpoc-state.lock` by passing it as the explicit lockPath* —
  works, but there is no reason to preserve the odd separate-basename lock now
  that the lock path is explicit; `<statePath>.lock` is the obvious convention
  and matches what state's own tests assume.

## Technical context

Key files:

- `internal/state/state.go` — the target module. `WriteJSON` =
  `MkdirAll(dir)` → `lock.AcquireWriteLock(path+".lock")` →
  `json.MarshalIndent(v,"","  ")` → `fsx.AtomicWriteBytes(path,data)`.
  `ReadJSON` = `MkdirAll(dir)` → `lock.AcquireReadLock(path+".lock")` →
  `os.ReadFile` (NotExist → `(zero,false,nil)`) → `json.Unmarshal` (error
  surfaced) → `(v,true,nil)`. Both release via `defer`. The only change here is
  threading an explicit `lockPath` instead of `path+".lock"`.
- `internal/board/store.go` —
  - `Save(boardPath, relPath string)` (line 99): marshals `s.tasks`, acquires
    `AcquireWriteLock(Join(boardPath,relPath)+".swaplock")`, `AtomicWriteBytes`.
    Becomes `Save()` → `state.WriteJSON(s.filePath, s.filePath+swapLockSuffix, s.tasks)`.
  - `Load()` (line 55): `AcquireReadLock(s.filePath+".swaplock")`, `os.ReadFile`
    (NotExist → empty, real read error → surfaced), release lock, `json.Unmarshal`
    (parse error → **silent empty today; becomes error**), then normalize nil
    `DependsOn` → `[]`. Becomes
    `tasks, found, err := state.ReadJSON[[]Task](s.filePath, s.filePath+swapLockSuffix)`,
    propagate `err`, `found==false` → `s.tasks = []Task{}`, then keep the
    DependsOn normalization loop.
  - `swapLockSuffix = ".swaplock"` constant (line 24) — keep; it's the lock-path
    argument now.
  - Empty `s.filePath` short-circuit in `Load` (line 56) — preserve (return
    empty tasks without touching disk).
- `internal/board/board.go` — `writeOp` (line 42) holds the coarse
  `writeLockFile = "tasks.json.lock"` (from `sync.go:24`), calls `store.Load()`
  (line 58) then `store.Save(b.boardPath, "tasks.json")` (line 70). Read ops
  `GetTask`/`ListTasksBrief`/`ListTasksFull` (lines 192-233) call `store.Load()`
  with **no** coarse lock. Update the `Save` call to `store.Save()`.
- `internal/muxpoc/state.go` —
  - `SaveState(cwd, s)` (line 83): nil guard (keep), MkdirAll, write lock,
    marshal, `fsx.AtomicWrite(cwd, stateRelPath, content)`. Becomes nil guard +
    `state.WriteJSON(statePath, statePath+".lock", s)` where
    `statePath = Join(cwd, stateRelPath)`.
  - `LoadState(cwd)` (line 48): becomes
    `v, found, err := state.ReadJSON[MuxpocState](statePath, statePath+".lock")`;
    `err != nil → return nil, err`; `!found → return nil, nil`;
    else `return &v, nil`.
  - `stateRelPath = ".lyx/muxpoc-state.json"` — keep. `lockRelPath` — remove.
- `internal/lock/lock.go` — `gofrs/flock`. Same lock path acquired twice in one
  process **blocks** (this is why the board collision is a real deadlock, not a
  no-op). Unchanged.
- `internal/fsx/fsx.go` — `AtomicWriteBytes(absPath, data)` and
  `AtomicWrite(dir, relPath, content)` (= `PathGuard` + `AtomicWriteBytes`).
  state uses `AtomicWriteBytes`. muxpoc's switch from `AtomicWrite` to state
  drops the `PathGuard` step, which is harmless: `stateRelPath` is a trusted
  constant, not untrusted input.

Gotchas:

- The board collision is the central reason this isn't a mechanical swap: state
  hardcoding `<path>.lock` would equal board's coarse `tasks.json.lock`.
- Lock-path consistency within each site is mandatory: a site's Save and Load
  must lock the same file. board: both `.swaplock`. muxpoc: both
  `<statePath>.lock`.
- `state.ReadJSON` holds its lock across the unmarshal; board.Load currently
  releases before unmarshalling. The slightly longer hold is acceptable (and
  arguably safer); it is not a behavior contract anything depends on.
- `ReadJSON` runs `MkdirAll(dir)` on the read path. Harmless for both sites
  (board dir exists by the time Load runs after the `os.Stat` short-circuit;
  muxpoc's LoadState already did MkdirAll).

## Constraints

From `CONSTRAINTS.md` (Path Invariant): all cwd / worktree-root resolution must
go through `internal/paths`; raw `os.Getwd` / `git rev-parse` are banned outside
`internal/paths` and `cmd/lyx/main.go`, enforced by
`internal/paths/enforcement_test.go`. This task touches none of those — paths
flow in as arguments (`cwd`, `boardPath`, `s.filePath`) — so the invariant is
unaffected. Do not introduce any raw path primitives.

Lock files must stay gitignored: `internal/board/boardtest/sync_test.go:153`
asserts no `*.lock` / `*.swaplock` is committed, and `sync.go` cleanup globs
both patterns. board keeps `.swaplock`; muxpoc's `.json.lock` still matches the
`*.lock` glob. No change needed, but don't break it.

## Testing

Existing tests are the guardrail; update them where behavior intentionally
changes (corruption) and rely on them where it doesn't (persistence, concurrency).

- `internal/state/state_test.go`: update for the new `lockPath` parameter.
  `TestLockFileLocation` and the "exactly two files" test now pass an explicit
  lock path (use `path + ".lock"`) and assert the lock lands there. Round-trip
  and NotExist tests just gain the extra argument. Corruption test (if present)
  stays — corruption is still an error.
- `internal/board/store_test.go` + `internal/board/boardtest/*`: persistence
  round-trips and the concurrency tests
  (`TestConcurrentReadsDuringUpserts`, `TestConcurrentUpsertsDoNotLoseWrites`,
  `BenchmarkGetDuringUpsert`) must still pass unchanged — they prove the
  swap-lock/coarse-lock split survived the refactor (a regression here = the
  deadlock). Add/adjust a test asserting `Load` now returns an **error** on a
  corrupt `tasks.json` (previously it silently produced an empty board). Verify
  the DependsOn nil→`[]` normalization is still covered.
- `internal/muxpoc/state_test.go`:
  - `TestLoadStateCorrupt` — **rewrite**: now expects `err != nil` and
    `state == nil` (was: `warn != ""`, `err == nil`).
  - `TestLoadStateMissing` — update to the 2-return signature; still
    `(nil, nil)`.
  - `TestSaveLoadRoundtrip` — update to the 2-return signature; behavior
    unchanged.
  - Confirm the lock-file rename to `muxpoc-state.json.lock` doesn't break any
    assertion (none found that asserts the lock basename).
- Caller compile-fix coverage: `attach.go`, `daemon.go`, `down.go`, `review.go`,
  `status.go`, `up.go`, `muxpoc_smoke_test.go` must build against the new
  `LoadState` arity. The smoke test (`muxpoc_smoke_test.go:210`) should still
  pass.
- TDD candidate: the `state` lock-path change — write the updated state tests
  first (they pin the contract: lock lands at the caller-supplied path), then
  refactor the two call sites against the green module.

## Q&A log

- **Q:** How should board adopt `state` given its write path self-deadlocks
  (state hardcodes `<file>.lock` = board's coarse lock; board needs a separate
  swap lock)? **A:** Operator delegated ("finn på noe smart"). Decision: give
  `state` an explicit `lockPath` parameter (cheap — no prod callers); board
  passes `tasks.json.swaplock`, preserving its two-lock read-fence design.
- **Q:** `state.ReadJSON` errors on corrupt JSON, but board swallows it (empty)
  and muxpoc warns (`TestLoadStateCorrupt`). How to handle the read path?
  **A:** Operator: "finn på noe smart. Swallow av error er ikke akseptabelt."
  Decision: corruption surfaces as an error at both sites; drop muxpoc's
  vestigial `warn` return; update the guard tests to expect errors.
- **Q:** Is the muxpoc lock-file rename (`muxpoc-state.lock` →
  `muxpoc-state.json.lock`) safe? **A:** Yes — the constant is referenced only
  inside `state.go`, the lock file is ephemeral/gitignored, and Save+Load move
  together so the fence stays intact.
- **Q:** Should `Store.Save`'s redundant `(boardPath, relPath)` params be kept?
  **A:** No — collapse to `Save()` using `s.filePath`; the lone caller already
  passes the same path the constructor received.
