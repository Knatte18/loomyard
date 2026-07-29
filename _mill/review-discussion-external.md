# External review of discussion.md — from the codeintel-v1 session

Reviewer context: I ran the benchmark that produced this task's problem statement (items 3-5) and shipped items 1-2 (`2574c14f`) on the parent `codeintel-v1` branch. This is a manual read-through review, not a `mill-review-*` skill invocation.

**Verdict: strong overall.** Decisions are well-reasoned, internally consistent, and technically accurate against the actual source (I cross-checked the file/function names in "Technical context" against what I read in `internal/codeintelengine` myself). One real design gap should be resolved before implementation starts; two are minor and non-blocking.

## [BLOCKING] Ordering between wedged-restart escalation and native-fallback is unspecified

`item5-native-fallback` lists "dial failure" as a trigger to fall back to native. `item5-wedged-daemon-escalation` targets the *same* signal — "dial-or-finalize fails" against a non-stale state — and resolves it with a forced restart-and-retry instead. The two decisions don't say which one fires first, and the two orderings produce genuinely different systems:

- **Escalation first, native as last resort:** a dial failure triggers the forced restart+retry; only if *that* also fails does the caller fall back to native. Native fallback becomes insurance of last resort, at the cost of restart+retry latency before it kicks in.
- **Native fallback first:** any dial failure falls straight over to native, bypassing the escalation entirely. In that ordering, the wedged-daemon escalation — item 5's headline fix for the exact gap this task exists to close — never actually fires against a live "healthy but wedged" daemon, because native fallback wins the race and masks it. Every future caller still hits the same wedged daemon, since nothing ever restarted it.

My read: escalation should run first — it's the only mechanism that actually fixes the daemon for *subsequent* callers, not just this one call. Falling back to native without ever attempting the restart leaves the wedge in place for everyone after you. Recommend making this explicit in `item5-native-fallback`'s own decision text (not just implied by `item5-wedged-daemon-escalation`), so a reader of either decision alone gets the right sequencing.

## [NIT] Constant rename isn't threaded through to existing tests

`item5-supervised-idle-timeout` renames `nativeDaemonIdleTimeout` to a shared name. `internal/codeintelengine/ensureserver_test.go` already has two tests referencing the old name directly — `TestNativeArgv_IncludesExtendedIdleTimeout` and `TestNativeArgv_PreservesBinPathAndExtraArgs` (from `2574c14f`, item 2's predecessor commit, already on this branch). Worth a one-line mention in Technical Context or Gotchas so the implementer updates both call sites rather than discovering it via a compile failure.

## [NIT] KillPID test coverage should name both platform files explicitly

Technical Context correctly says `proc_linux.go` + `proc_windows.go` both get `KillPID`. The Testing section's "Unit: proc.KillPID — cross-platform behaviour" doesn't say the tests belong in both `proc_linux_test.go` and `proc_windows_test.go`, matching the `IsAlive`/`DetachBreakaway` precedent already set in this codebase (`daemon-state-and-locking` and `ensure-server-supervised` batches). Minor, but worth pinning explicitly since only Linux is exercised in this environment — easy to forget the Windows half otherwise.

## Everything else

Scope boundary, rejected alternatives, and the Q&A log are internally consistent with each other and with what I found in the source during the parent benchmark investigation: `resolvePosition`'s LSP-range-without-byte-round-trip discipline, the Leaf Invariant's exact import list, and `daemonStale`'s two-part check are all described accurately. No other gaps found.
