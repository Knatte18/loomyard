# Batch: webster-startup-window-docs

```yaml
task: Shuttle guarantees a started run is past its startup gates
batch: webster-startup-window-docs
number: 2
cards: 3
verify: go test ./internal/websterengine/ ./internal/webstercli/ && go test -tags integration ./internal/websterengine/ ./internal/webstercli/ && go test -tags smoke -run '^$' ./internal/webstercli/
depends-on: [1]
```

## Batch Scope

Webster needs no control-flow change: `RecoverBatch`'s `Starter.Start` and `websterengine.Run`'s `StartMaster` inherit batch 1's guarantee.
What changes is what webster's own contracts and docs promise, because both spawns run under webster's state-mutation lease and both persist the spawned strand's guid only after the start returns — and the start now includes the provider's startup window.
This batch rewords the lease contract, states the two persist-before-block residuals and the two start-error residuals, rewords every place that promises `recover-batch` blocks at most `poll_wait_s`, and adds the one webster test the discussion asks to verify (a `Starter` error surfacing from the recovery spawn).
It is its own batch because it touches a different module pair (`internal/websterengine`, `internal/webstercli`, the webster stencil) and depends only on batch 1's `shuttleengine.ErrNotStarted` sentinel and blocking-start semantics.
Batch-local decisions beyond `## Shared Decisions`: none.

## Cards

### Card 6: Webster's lease contract and residuals state the startup window

- **Context:**
  - `_mill/discussion.md`
  - `internal/shuttleengine/run.go`
  - `internal/shuttleengine/wait.go`
  - `internal/websterengine/config.go`
- **Edits:**
  - `internal/websterengine/state.go`
  - `internal/websterengine/recoverbatch.go`
  - `internal/websterengine/runlevel.go`
  - `internal/websterengine/strand.go`
  - `internal/websterengine/doc.go`
  - `internal/websterengine/awaitbatch.go`
  - `internal/webstercli/recoverbatch.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  Implements the discussion's decisions "Webster's state-mutation lease across the startup window" and "Webster's persist-before-block windows"; comments and help text only, no control-flow change.
  In `internal/websterengine/state.go`, rewrite `AcquireStateMutation`'s doc comment: a spawn's startup window, bounded by shuttle's `startup_timeout_s` (typically one or two probe intervals, about 5–10 s under the shipped config, at most 90 s), is part of the load-mutate-save sequence at the two sites that spawn under the lease (`recover-batch`'s spawn and `Run`'s Master spawn), and concurrent verbs block on the lease for that time rather than failing;
  an unbounded or poll-length wait (`RecoverAwait`, Master's own `Wait`) still never runs under it.
  Keep the existing justification that holding the lease across the spawn is what serialises two concurrent `recover-batch` calls, so the second sees the first's recorded guid and attaches instead of spawning a duplicate.
  In `internal/websterengine/recoverbatch.go`:
  rewrite the file header's "every call (the first included) blocks at most one wait window" to say the call that spawns the recovery strand first waits for its provider to come up (normally seconds, bounded by `startup_timeout_s`), and every call then blocks at most one wait window;
  keep the header's lease paragraph consistent with the new `AcquireStateMutation` wording (the lease is held across the spawn, including its startup window, never across the bounded wait);
  extend `RecoverSpawnOrAttach`'s doc comment with the Accepted residuals: (1) a process killed inside the startup window leaves a live recovery strand whose guid was never persisted, so the next call's `prior.StrandGUID` reclaim (`removeStrandIfLive`) cannot see it and the next spawn runs beside it;
  (2) a startup mechanism failure (shuttle could not get a liveness answer from reed `maxStatusRetries` times) returns an error with the strand left live and no guid persisted, with the same consequence, accepted because tearing it down could kill a working agent and a reed in that state usually fails the next `AddStrand` too — the error names the strand guid so an operator can remove it by hand;
  (3) a not-ready start no longer leaks (shuttle tears the strand down) unless that teardown's own strand removal fails, which the returned error then states.
  In `internal/websterengine/runlevel.go`:
  extend `MasterHandle`'s doc comment with the same three residuals for the Master strand (killed inside the startup window, or a startup mechanism failure, leaves a live Master pane `MasterStrand` never recorded, invisible to entry-time reclaim; a failed not-ready teardown is stated by the error), and change "available immediately after the start" to "available once the start returns, which includes the provider's startup window";
  at the `deps.Starter.StartMaster(spec, deps.Gate)` call site, add a sentence to the surrounding comment that the state-mutation lease acquired earlier is held across `StartMaster`, now including the provider's startup window (bounded by `startup_timeout_s`), and that at run entry no batch forks exist yet, so the hold stalls nothing in practice;
  keep the existing lease-acquire comment ("never held across Master's own wait") true.
  In `internal/websterengine/strand.go`, rewrite `Starter`'s "Start is deliberately non-blocking." to: Start blocks until the spawned provider is past its startup gates (shuttle's guarantee) and returns an error when it never became ready; it never waits for the run to finish.
  In `internal/websterengine/doc.go`, rewrite the "Every call, including the first, blocks for at most poll_wait_s" sentence under the cold-recovery heading to the same statement as the recoverbatch.go header.
  In `internal/websterengine/awaitbatch.go`, rewrite the parenthetical "(each call blocks at most one wait window; ...)" that describes recover-batch's idiom so it stays accurate: recover-batch's re-polls each block at most one wait window.
  In `internal/webstercli/recoverbatch.go`:
  rewrite the file header's "a single wait blocks up to poll_wait_s" passage to state that the spawn phase under the lease now includes the provider's startup window (bounded by `startup_timeout_s`), while the wait phase that blocks up to `poll_wait_s` still runs with the lease released;
  rewrite the `recoverBatchCmd` `Long` help's "then blocks for up to --wait watching it" so it says a call that spawns the recovery strand first waits for its provider to come up (normally seconds), and every call then blocks for up to `--wait` watching it;
  keep `Short` unchanged and keep `Long` in help-text register.
- **Commit:** `docs(webster): state the startup window the state-mutation lease now spans`

### Card 7: Master stencil states the spawning recover-batch call's extra wait

- **Context:**
  - `_mill/discussion.md`
  - `internal/websterengine/template_test.go`
- **Edits:**
  - `contracts/stencils/webster/webster-template-master.md`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  Implements the stencil part of the discussion's decision "Webster's state-mutation lease across the startup window".
  In `contracts/stencils/webster/webster-template-master.md`, rewrite both places that promise a bounded call:
  the bullet "`recover-batch <NN>` returns a `running` snapshot → re-call ... — each call blocks at most `{{.poll_wait_s}}` seconds, so re-polling immediately is not busy-waiting." and the Tuning-knobs line "a single `recover-batch` call blocks at most `{{.poll_wait_s}}` seconds before returning a `running` snapshot for you to re-call."
  Each must say that the call which spawns the recovery strand additionally waits for its provider to come up (normally seconds), and every re-poll after it is bounded by `{{.poll_wait_s}}` seconds, so re-polling immediately is still not busy-waiting.
  Introduce no new template variable — the startup bound is described in words, never interpolated — and keep `{{.poll_wait_s}}` present in the stencil (`internal/websterengine/template_test.go` asserts that marker renders).
  Keep one sentence per line, as the stencil already does.
- **Commit:** `docs(webster): Master stencil names the spawning recover-batch call's startup wait`

### Card 8: RecoverSpawnOrAttach surfaces a not-ready recovery start

- **Context:**
  - `_mill/discussion.md`
  - `internal/websterengine/recoverbatch.go`
  - `internal/websterengine/strand.go`
  - `internal/shuttleengine/wait.go`
- **Edits:**
  - `internal/websterengine/recoverbatch_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  Implements the discussion's Testing bullet for websterengine: no existing test covers a `Starter` whose `Start` errors (the file's `recoverFixture` wires a real `*shuttleengine.Runner` over fakes that always come up ready).
  In `internal/websterengine/recoverbatch_test.go` (build tag `integration`), add a local double `erroringStarter` implementing `websterengine.Starter` whose `Start` returns `(nil, fmt.Errorf("...: %w", shuttleengine.ErrNotStarted))`, and a test `TestRecoverSpawnOrAttach_NotReadyStartSurfacesAndRecordsNothing`:
  build a fixture with `newRecoverFixture`, replace `fx.Deps.Starter` with the double, call `websterengine.RecoverSpawnOrAttach(fx.Deps, 1, clk)` with the file's `recoverFakeClock`, and assert the returned error satisfies `errors.Is(err, shuttleengine.ErrNotStarted)`, `spawned` is false, the returned `*BatchState` is nil, and `fx.Deps.State.Batches[1]` is still nil (no guid recorded for a strand shuttle already tore down).
- **Commit:** `test(websterengine): a not-ready recovery start surfaces and records no batch state`

## Batch Tests

`verify:` runs `internal/websterengine` and `internal/webstercli` untagged (`template_test.go` renders the Master stencil card 7 edits, and the CLI help-tree tests cover `recover-batch`'s `Long` text) and their `integration` tiers: `recoverbatch_test.go` (card 8's new test, plus the existing `RecoverBatch` tests that now pass through batch 1's startup step via a real `*shuttleengine.Runner` whose fake engine reports `StartupReady`), `runlevel_test.go`, and `webstercli/verbs_test.go` (a real `*shuttleengine.Runner` over fakes that register the spawned strand live).
`internal/webstercli`'s `smoke` tier is compiled but not executed (`-run '^$'`, overview Shared Decision "webstercli smoke tier is not run"): executing it launches a real headless `claude` and fails on this worktree's baseline for unrelated reasons, and none of its tests reaches `recover-batch`, whose help text and header comment are this batch's only `internal/webstercli` edits — so compiling the tagged files against the edited package is the check that matters.
