# Batch: standalone-watcher

```yaml
task: "Replace reed's header pane with a status-line and Selvage"
batch: "standalone-watcher"
number: 6
cards: 3
verify: go test ./internal/burlercli/ ./internal/webstercli/
depends-on: [5]
```

## Batch Scope

This batch closes the standalone half of the watchdog rehoming. The detached per-hub daemon batch 5 introduced is a **hub-mode** mechanism only; standalone reed sessions instead run `Engine.Watch(ctx)` as an in-process goroutine off the same `reedUp` seam that already boots them, with no lock, no discovery, no second process and no `--hub-path`.
It is one batch because the seam's signature change reaches both standalone CLIs at once — `reedUp func() error` becomes `reedUp func(ctx context.Context, watch bool) error` — and all three of its call sites must move with it in one step or the tree does not compile.

Batch-local decisions beyond `## Shared Decisions`: the `watch bool` is an **explicit parameter** rather than an implicit rule precisely so the `recover-batch` asymmetry is visible at both call sites instead of being rediscovered from a comment.
Standalone never computes a hub lock path and never spawns a daemon: `fabricengine.HubScratchDir(hub)` is meaningless there (standalone's `Geometry.HubPath` is a derived `stateDir`, not a hub), and worktree discovery is meaningless too (standalone has exactly one target and one session, known at wiring time).
This batch accepts, rather than discovers later, one behaviour change: today the standalone header pane keeps watching after the run that booted it finishes, and with this seam the watcher's life is bounded to that run.

## Cards

### Card 41: widen the reedUp seam to carry a context and a watch flag

- **Context:**
  - `internal/reedengine/watchloop.go`
  - `internal/standalonegeom/reedgeom.go`
  - `_mill/discussion.md`
- **Edits:**
  - `internal/burlercli/cli.go`
  - `internal/burlercli/wiring.go`
  - `internal/webstercli/cli.go`
  - `internal/webstercli/wiring.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** In `internal/burlercli/cli.go` and `internal/webstercli/cli.go`, change the `reedUp` field's type from `func() error` to `func(ctx context.Context, watch bool) error`, adding the `context` import where absent. Extend each field's doc comment to state that it brings the standalone reed session up idempotently and, when `watch` is true and the boot succeeded, also starts that session's in-process resize watcher bound to `ctx`; and that the flag is explicit rather than implicit so the `recover-batch` asymmetry is visible at the call sites. In `internal/burlercli/wiring.go` and `internal/webstercli/wiring.go`, rewrite each `c.reedUp = func() error { _, err := reedEngine.Up(); return err }` closure to take `(ctx context.Context, watch bool)`, boot the engine exactly as it does today, return the boot error unchanged on failure, and — only when the boot succeeded **and** `watch` is true — start `go reedEngine.Watch(ctx)` before returning nil. Document at each closure why standalone uses a goroutine rather than the detached per-hub daemon: standalone reed is never booted by `lyx reed up` (that verb is hub-only), it is booted in-process by a long-lived supervising run that exists for exactly the session's working lifetime, and that supervising process is precisely what hub mode lacks and why hub mode needs a daemon at all — so reusing it is the smaller mechanism, not a special case. State also that the caller's context is what stops the watcher, and that standalone computes no hub lock path and spawns no daemon. Both closures keep being assigned in `wireStandalone` only; `wireHub` still leaves `c.reedUp` nil.
- **Commit:** `feat(burlercli,webstercli): widen the reedUp seam with a context and a watch flag`

### Card 42: pass the watch disposition at all three call sites

- **Context:**
  - `internal/burlercli/cli.go`
  - `internal/webstercli/cli.go`
  - `internal/burlercli/wiring.go`
  - `internal/webstercli/wiring.go`
- **Edits:**
  - `internal/burlercli/run.go`
  - `internal/webstercli/run.go`
  - `internal/webstercli/recoverbatch.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Update the three `c.reedUp()` call sites to the new signature, each passing the disposition the discussion's `standalone-runs-the-watch-loop-in-process` decision fixes: `internal/burlercli/run.go` calls `c.reedUp(cmd.Context(), true)`; `internal/webstercli/run.go` calls `c.reedUp(cmd.Context(), true)`; and `internal/webstercli/recoverbatch.go` calls `c.reedUp(cmd.Context(), false)`. Extend the comment block above `recoverbatch.go`'s call with the reason for its `false`: `recover-batch` is a short-lived verb that spawns a cold recovery strand and returns, so a watcher bound to its context would be dead before it observed anything, while one detached from that context would be an unowned goroutine in an exiting process. Leave each site's existing error handling, envelope message and nil-check (`if c.reedUp != nil`) exactly as they are — hub mode still leaves the seam nil, and the guard is what keeps hub mode from booting a session that is the operator's or loom's to manage. Change nothing else in the three files.
- **Commit:** `feat(burlercli,webstercli): pass the watch disposition at each reedUp call site`

### Card 43: pin the standalone watcher's lifetime and the recover-batch asymmetry

- **Context:**
  - `internal/burlercli/run.go`
  - `internal/webstercli/run.go`
  - `internal/webstercli/recoverbatch.go`
  - `internal/burlercli/wiring.go`
  - `internal/webstercli/wiring.go`
- **Edits:**
  - `internal/burlercli/wiring_test.go`
  - `internal/webstercli/wiring_test.go`
  - `internal/webstercli/cli_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Retarget `internal/webstercli/cli_test.go`'s existing `c.reedUp = func() error { … }` assignment in `TestRecoverBatchCmd_BootsStandaloneReedSessionFirst` to the widened signature — it is the one stray assignment outside the two `wiring.go`/`wiring_test.go` pairs, and card 41's field-type change stops it compiling. Its recording closure takes `(ctx context.Context, watch bool)`, and its existing assertion that `recover-batch` performs exactly one bring-up is extended to assert the `watch` argument it received was `false`, which is this test's own subject made stricter rather than a new one. `internal/burlercli` has no equivalent stray assignment. In both `internal/burlercli/wiring_test.go` and `internal/webstercli/wiring_test.go`, retarget the existing `c.reedUp == nil` / `c.reedUp != nil` assertions to the new signature (they assert seam presence, not shape, so only the surrounding construction changes). Add assertions over the seam's behaviour, substituting a fake engine boot rather than booting a real tmux server: the watcher goroutine starts on a successful boot with `watch: true`; it does not start when the boot fails; it does not start when `watch: false`; and it stops when the passed context is cancelled. Add assertions that the three call sites pass what this task decided — `burlercli/run.go` and `webstercli/run.go` pass `true`, `webstercli/recoverbatch.go` passes `false` — since that asymmetry is the whole reason the parameter exists; drive each through its own `RunE` with a recording fake in `c.reedUp` rather than by reading source text. Assert too that standalone never computes a hub lock path and never spawns a daemon: neither package may reference `fabricengine.HubScratchDir` nor `reed watchdog` anywhere in its production files. Keep every test in both files untagged and spawning nothing, per the Test Tier Purity Invariant — the fakes make that possible, and `internal/burlercli/cli_test.go`'s existing comment about `run`'s RunE never reaching `c.reedUp` on its fixture records why that matters for this package's tier.
- **Commit:** `test(burlercli,webstercli): pin the standalone watcher lifetime and recover-batch asymmetry`

## Batch Tests

`verify: go test ./internal/burlercli/ ./internal/webstercli/` covers exactly the two packages this batch edits, and both halves of what it changes: the seam's own behaviour (`wiring_test.go` in each) and the three call sites' dispositions, driven through each verb's `RunE` with a recording fake rather than a live boot.
Every test stays untagged and spawns nothing — which matters here more than usual, because both packages' existing suites already sit deliberately inside Tier 1 and `internal/burlercli/cli_test.go` carries a comment recording that `run`'s `RunE` must never reach `c.reedUp` on its fixture for exactly that reason.
The overview's module-wide `verify: go build ./...` catches any other caller of the widened seam this batch missed.
