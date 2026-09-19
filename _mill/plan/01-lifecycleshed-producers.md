# Batch: lifecycleshed producers

```yaml
task: "Worktree spawn/teardown as Shed producers"
batch: "lifecycleshed producers"
number: 1
cards: 7
verify: go test ./internal/lifecycleshed/...
depends-on: []
```

## Batch Scope

This batch delivers `internal/lifecycleshed` in full: the three `shedengine.ShedProducer` implementations (`WorktreeCreate`, `LoomRun`, `WorktreeTeardown`), their injected-seam types (`LoomRunDeps`, `TeardownDeps`, `PrimeLock`), the package's own `entryErr`/`cancelErr` pair, its own `reportStuck` copy, and the Tier-1 test suite over fakes.
It is one batch because the three producers share `ctx.go`, `stuck.go`, and `deps.go`, and because none of them can be meaningfully tested without all three seam types existing.

Nothing outside `internal/lifecycleshed` is touched.
The external interface batch 2 consumes is exactly the three `New*` constructors plus the three seam types in `deps.go`.

Batch-local decision, not in the overview: the package takes told absolute paths and injected closures only — it has no production import of `internal/lyxcwd`, `internal/fabricengine`, or `internal/reedengine`, and `seam_enforcement_test.go` enforces that by allowlist.

## Cards

### Card 1: package doc and the context-check pair

- **Context:**
  - `internal/preflightshed/ctx.go`
  - `internal/preflightshed/doc.go`
  - `internal/landingshed/doc.go`
- **Edits:** none
- **Creates:**
  - `internal/lifecycleshed/doc.go`
  - `internal/lifecycleshed/ctx.go`
  - `internal/lifecycleshed/ctx_test.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** Create `package lifecycleshed` with a package doc comment in `doc.go` stating: the package owns the three task-worktree lifecycle producers; it is bound by the Told-Geometry Invariant (every path told, no `internal/lyxcwd` import); it is outside the Fabric Vocabulary Invariant's owner set, so no identifier, string literal, or comment in the package may name either side of the pair — write "the task worktree" and "the pair" instead; and it carries its own copy of the context-check helpers for the reason `internal/preflightshed/doc.go` already records.
  In `ctx.go` declare `entryErr(ctx context.Context, name string) error` and `cancelErr(ctx context.Context, name string) error`, byte-shaped after `internal/preflightshed/ctx.go` but with the `lifecycleshed: ` message prefix in place of `preflightshed: `.
  `ctx_test.go` covers both helpers: nil for a live context, non-nil wrapping `ctx.Err()` and naming the told producer name for a cancelled one.
- **Commit:** `feat(lifecycleshed): add package doc and the context-check helper pair`

### Card 2: the stuck-reason carrier

- **Context:**
  - `internal/landingshed/stuck.go`
- **Edits:** none
- **Creates:**
  - `internal/lifecycleshed/stuck.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** Declare `const stuckFileSuffix = "-stuck.md"` and `func reportStuck(producer, reason, scratchDir string, fields ...any)`, a direct copy of `internal/landingshed/stuck.go`'s own helper with the `lifecycleshed: ` log-message prefix in place of `landingshed: `.
  The doc comment must state why the carrier exists rather than a returned string: `shedengine.Run` persists its own fixed reason (`"stuck with no OnStuck target"`) for every stuck verdict regardless of what the producer knows, so a producer-supplied reason reaches a human only through the `internal/logger` warning and the one-line reason file this helper writes.
  `scratchDir` is created with `os.MkdirAll` on every write path, and a write failure is logged rather than swallowed and never replaces the caller's verdict.
- **Commit:** `feat(lifecycleshed): add the stuck-reason log-and-file carrier`

### Card 3: the three seam types

- **Context:**
  - `internal/landingshed/deps.go`
  - `internal/shedengine/status.go`
  - `internal/lock/lock.go`
- **Edits:** none
- **Creates:**
  - `internal/lifecycleshed/deps.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** Declare three exported types.
  `PrimeLock` carries `Path string` (the told absolute lock path, named in a contention stuck reason) and `Acquire func() (release func() error, ok bool, err error)`, whose doc comment states that a `(nil, false, nil)` return is contention rather than an error, mirroring `lock.TryAcquireWriteLock`'s own contract, and that the lock file carries no holder record so the reason can name the path and nothing else.
  `LoomRunDeps` carries `Spawn func(ctx context.Context) error`, `ResolveStatus func() (statusPath, statusLockPath string, err error)`, `ReadStatus func(statusPath, statusLockPath string) (shedengine.Status, bool, error)`, `Now func() time.Time`, and `Sleep func(d time.Duration)`.
  Document that `ResolveStatus` is evaluated on `Call`, never at wiring time, because the task worktree does not exist until `WorktreeCreate` has run; and that a nil `Now` selects `time.Now` and a nil `Sleep` selects `time.Sleep`, so a test may hold the clock still and skip every real sleep.
  `TeardownDeps` carries `Shutdown func(ctx context.Context) (abandonedSession string, err error)` and `Remove func(ctx context.Context) error`, documented as two fields rather than one closure because the producer must call `Shutdown` strictly before `Remove`, must not call `Remove` at all when `Shutdown` fails, must say which of the two failed in its stuck reason, and must surface the abandoned-session value on an otherwise-`Done` row.
- **Commit:** `feat(lifecycleshed): add the PrimeLock, LoomRunDeps and TeardownDeps seam types`

### Card 4: WorktreeCreate

- **Context:**
  - `internal/lifecycleshed/deps.go`
  - `internal/lifecycleshed/ctx.go`
  - `internal/lifecycleshed/stuck.go`
  - `internal/preflightshed/preflight.go`
  - `internal/shedengine/producer.go`
- **Edits:** none
- **Creates:**
  - `internal/lifecycleshed/create.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** Declare `func NewWorktreeCreate(name, slug string, createWorktree func(context.Context) error, primeLock PrimeLock, scratchDir string) shedengine.ShedProducer` over an unexported `worktreeCreateProducer` struct carrying those five values, with the usual `var _ shedengine.ShedProducer = (*worktreeCreateProducer)(nil)` assertion.
  `Call` runs in this order: `entryErr`; the prime-lock acquire seam; then `createWorktree(ctx)`; then `shedengine.Done`.
  An acquire error is a returned hard error (mechanism failure).
  An acquire reporting `ok == false` is `shedengine.Stuck` with a reason naming `primeLock.Path` and the slug — never a holder identity, which the lock cannot report.
  The release closure is invoked on every exit path taken after a successful acquire, including every `Stuck` one, via `defer`, and a release error is logged at `Warn` rather than replacing the producer's verdict.
  A `createWorktree` error is `Stuck` with the reason passing that error's text through verbatim and unreworded, because fabric's own refusals — the dirty-driving-worktree probe and the pre-existing-branch refusal — already name their remedies.
  Every non-`Done` return consults `cancelErr` first and returns that error in place of its verdict when the context is cancelled.
  Every `Stuck` routes through `reportStuck(name, reason, scratchDir, "slug", slug)`.
- **Commit:** `feat(lifecycleshed): add the WorktreeCreate producer`

### Card 5: LoomRun

- **Context:**
  - `internal/lifecycleshed/deps.go`
  - `internal/lifecycleshed/ctx.go`
  - `internal/lifecycleshed/stuck.go`
  - `internal/shedengine/status.go`
  - `internal/shedengine/producer.go`
- **Edits:** none
- **Creates:**
  - `internal/lifecycleshed/loomrun.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** Declare `func NewLoomRun(name, slug string, deps LoomRunDeps, pollInterval time.Duration, pollAttempts int, scratchDir string) shedengine.ShedProducer` over an unexported `loomRunProducer`, with the compile-time `shedengine.ShedProducer` assertion.
  `Call` runs `entryErr`, then `deps.ResolveStatus()` — a resolution error is a returned hard error, since the task worktree is required to exist by the time this row runs — then logs the spawn at `Info` through `internal/logger` naming the slug, then `deps.Spawn(ctx)`, then logs the completed wait at `Info` (the Live-Substrate Spawn Observability invariant requires both, because this producer waits for its child rather than detaching).
  A `Spawn` error is `Stuck`.
  It then polls up to `pollAttempts` times: each attempt calls `deps.ReadStatus(statusPath, statusLockPath)`; a read error is a returned hard error, because a status file that exists but does not decode, or a status lock that cannot be taken, is mechanism failure rather than a producer verdict; `found == false` is `Stuck`, with a reason stating the child's own handshake already confirmed a driver took the run lock so a missing seed at this point is a real inconsistency.
  The verdict table over the persisted `shedengine.Status.State` is exhaustive over all five values: `shedengine.StateDone` is `shedengine.Done`; `shedengine.StateBlocked`, `shedengine.StatePaused` and `shedengine.StateFailed` are each `Stuck` with a reason naming the state, the status's own `Error` field and its `CurrentProducer`; `shedengine.StateRunning` consumes one attempt and continues.
  A `Stuck` on `StateFailed` must be reached from the terminal arm of this switch and must not consume a poll attempt.
  Any other value is a returned hard error.
  Between attempts the producer calls `deps.Sleep(pollInterval)` and consults `cancelErr`.
  Exhausting `pollAttempts` with the state still `StateRunning` is `Stuck`, with a reason naming the interval, the attempt count and the elapsed wall clock computed from `deps.Now()` readings taken before the first attempt and at exhaustion.
  Nil `deps.Now` resolves to `time.Now` and nil `deps.Sleep` to `time.Sleep`, both resolved once in `NewLoomRun`.
  Every `Stuck` routes through `reportStuck`, and every non-`Done` return consults `cancelErr` first.
- **Commit:** `feat(lifecycleshed): add the LoomRun producer with its bounded poll`

### Card 6: WorktreeTeardown

- **Context:**
  - `internal/lifecycleshed/deps.go`
  - `internal/lifecycleshed/ctx.go`
  - `internal/lifecycleshed/stuck.go`
  - `internal/lifecycleshed/create.go`
  - `internal/shedengine/producer.go`
- **Edits:** none
- **Creates:**
  - `internal/lifecycleshed/teardown.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** Declare `func NewWorktreeTeardown(name, slug string, deps TeardownDeps, primeLock PrimeLock, scratchDir string) shedengine.ShedProducer` over an unexported `worktreeTeardownProducer`, with the compile-time `shedengine.ShedProducer` assertion.
  `Call` runs `entryErr`, then acquires and defers the release of `primeLock` exactly as `NewWorktreeCreate`'s producer does, with the same three dispositions (error is a hard error, `ok == false` is `Stuck` naming `primeLock.Path`, release errors logged at `Warn`).
  It then calls `deps.Shutdown(ctx)`.
  A `Shutdown` error is `Stuck` with a reason that names session shutdown as the failed half, and `deps.Remove` is not called at all on that path — abandoning that ordering is the single thing this one-row producer exists to prevent.
  A non-empty `abandonedSession` return is logged at `Warn` through `internal/logger` naming the slug and the session, and does not change the verdict.
  It then calls `deps.Remove(ctx)`; a `Remove` error is `Stuck` with a reason that names worktree removal as the failed half and states that session shutdown already succeeded, so the two halves are distinguishable in the reason text.
  On success the row returns `shedengine.Done`.
  Every non-`Done` return consults `cancelErr` first, and every `Stuck` routes through `reportStuck`.
- **Commit:** `feat(lifecycleshed): add the WorktreeTeardown producer sequencing shutdown before removal`

### Card 7: Tier-1 tests and the seam-enforcement guard

- **Context:**
  - `internal/lifecycleshed/deps.go`
  - `internal/lifecycleshed/create.go`
  - `internal/lifecycleshed/loomrun.go`
  - `internal/lifecycleshed/teardown.go`
  - `internal/lifecycleshed/stuck.go`
  - `internal/loomrecipe/seam_enforcement_test.go`
  - `internal/shedengine/status.go`
  - `internal/fabricengine/add.go`
- **Edits:** none
- **Creates:**
  - `internal/lifecycleshed/create_test.go`
  - `internal/lifecycleshed/loomrun_test.go`
  - `internal/lifecycleshed/teardown_test.go`
  - `internal/lifecycleshed/seam_enforcement_test.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** Write table-driven, untagged Tier-1 tests over fake seams, using `t.TempDir()` for every `scratchDir`.
  `create_test.go` covers: the happy path returning `shedengine.Done`; a `createWorktree` error mapped to `Stuck` with the error's own text present verbatim in the written stuck-reason file; the pre-existing-branch case asserting the remedy wording in fabric's own message survives unreworded; the dirty-driving-worktree case asserting only verbatim pass-through of the bare string `source worktree has uncommitted changes`, since that message carries no remedy and an assertion demanding one would be unsatisfiable; an unavailable prime lock mapped to `Stuck` with `PrimeLock.Path` in the reason; an `Acquire` error surfaced as a returned error; the release closure invoked on the `Done` path, on the `createWorktree`-error `Stuck` path and on a cancelled-context path; and a cancelled context surfacing as a non-nil error and never as `Stuck`.
  `teardown_test.go` covers, with a fake recording the call sequence: `Shutdown` strictly before `Remove` on the happy path; `Remove` never called at all when `Shutdown` fails; the stuck reason naming which of the two halves failed, asserted separately for each half so a producer conflating them fails; a merge-in-progress `Remove` refusal as its own case so the mapping is not silently dirtiness-only; a non-empty `abandonedSession` on an otherwise-`Done` row; prime-lock acquisition, contention, and release-on-every-path including the `Stuck` ones; and the cancellation case.
  `loomrun_test.go` covers the full verdict table over a fake `ReadStatus`, a fake `Now` and a fake `Sleep` that never sleeps: all five `shedengine.State` values, `StateRunning` continuing to poll, an absent status after a successful `Spawn`, a read error asserted as a returned error rather than a verdict, a `Spawn` failure as `Stuck`, a `ResolveStatus` failure as a returned error, and the cancellation case.
  Two cases are load-bearing and get explicit assertions: `StateFailed` must consume zero poll attempts, asserted by counting `ReadStatus` and `Sleep` calls; and the attempt cap must fire on attempt count with the fake clock held still, proving the bound is not a wall-clock deadline in disguise, in unmeasurable real time so the file stays inside Test Tier Purity's no-`time.Sleep`-at-or-above-one-second rule.
  `seam_enforcement_test.go` is modelled on `internal/loomrecipe/seam_enforcement_test.go`: the same directory walk, the same imports-only parse, the same stdlib rule, and an allowlist holding exactly `internal/shedengine` and `internal/logger`, plus the named denial constant for `internal/lyxcwd` so that specific violation is reported by name.
- **Commit:** `test(lifecycleshed): cover the three producers and pin the told-geometry allowlist`

## Batch Tests

`verify:` runs `go test ./internal/lifecycleshed/...`, which covers every file this batch creates: `ctx_test.go`, `create_test.go`, `loomrun_test.go`, `teardown_test.go`, and `seam_enforcement_test.go`.
The scope is the new package alone — nothing outside it is edited, so no wider run is warranted at this boundary.
Every test in the batch is untagged Tier 1: no process is spawned, no git is invoked, and the only filesystem use is `t.TempDir()` for the stuck-reason scratch directory.
