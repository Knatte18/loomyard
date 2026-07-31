MILL_REVIEW_BEGIN
# Review: Diagnostic tracing (trace) on the logger module

```yaml
verdict: GAPS_FOUND
reviewer_model: opushigh
reviewer_self_id: Claude Opus (Anthropic); runtime reports model id claude-opus-5 — not independently verifiable from inside the session
reviewed_file: C:\Code\loomyard\wts\trace-logging\_mill\discussion.md
date: 2026-07-31
```

## Findings

### [GAP] Shared sync.Once conflates ID mint with sink open
**Section:** `trace-id-mint-and-propagate` (2nd bullet) vs `lazy-sink-open` / Testing "Sink naming and lazy open"
**Issue:** ID resolution is placed "under the same `sync.Once` as the sink open", but the sink must open only on the first **Info+** write while `trace=` must be stamped on **every** line including a `Debug` emitted first — one Once cannot do both, and the test "no file exists after `Debug` calls alone" would fail if the Once opens the sink.
**Fix:** Split the two: a Once (or lazy resolver) for trace identity that fires on any emit, and a separate Once for geometry+mkdir+sweep+create that fires only on the first Info+ write.

### [GAP] Fan-out test spec contradicts the Info+ durable sink
**Section:** Testing → "Dual-handler fan-out" vs `dual-handler-fan-out`
**Issue:** The assertion "an `Info` under `-v` likewise [reaches stderr and does **not** reach the durable sink]" is a leftover from the rejected Warn+ design and directly contradicts the r2 decision to pin the durable sink at Info+; worse, the one case the motivating incident needs — an `Info` at **default** verbosity (stderr threshold Warn) reaching the durable file — is asserted nowhere.
**Fix:** Rewrite that bullet to "an `Info` reaches the durable sink at every verbosity including the default, and reaches stderr only at `-v`+", which also pins the composite handler's `Enabled` being an OR of the two gates.

### [GAP] The "next spawn site" rule mis-classifies board/fabric re-execs
**Section:** `long-lived-child-env` → "The general rule, for the next spawn site"
**Issue:** The rule (strip when the child (a) outlives the spawning invocation **and** (b) runs lyx code that logs) literally captures `internal/boardengine/spawn.go:27` and `internal/fabricengine/spawn.go:62` — both `proc.Detach` + `cmd.Start()` with no `Wait()`, both re-execing the lyx binary — which `trace-id-mint-and-propagate` requires to **inherit** the ID.
**Fix:** Restate criterion (a) as "is a shared singleton reused by *later, unrelated* invocations" (tmux server) rather than "outlives the spawner", and name board/fabric's detached one-shot children as the explicit non-matching case.

### [GAP] scoutengine's spawn-logging obligation is never scheduled
**Section:** `scoutengine-allowlist` vs Scope → In / `adoption-scope`
**Issue:** The allowlist widening is justified by scoutengine being "currently non-compliant" with Live-Substrate Spawn Observability at the `ensureserver.go:520` daemon spawn, but nothing in Scope or `adoption-scope` schedules the `logger.Info`/`Warn` spawn+teardown lines at that site — the scope covers only converting existing stderr writes.
**Fix:** State explicitly whether the daemon spawn/teardown `Info` (and the wedged-daemon-kill `Warn`) lands in this task or is deferred; if deferred, re-justify the allowlist widening on the stderr conversions alone.

### [GAP] Widening CleanClaudeEnv pollutes the persisted StrippedEnv field
**Section:** `long-lived-child-env` → "Implementation note"
**Issue:** `CleanClaudeEnv` (`internal/reedengine/env.go:18`) returns `strippedKeys`, which `lifecycle.go:646,711` stamps into `ReedState.StrippedEnv` (`state.go:48`, JSON `strippedEnv`, doc-scoped to Claude-injected vars); routing `LYX_TRACE_ID` through that helper silently changes an observable, persisted diagnostic field and its doc, and `state_test.go:32` pins that field's shape.
**Fix:** Decide between widening `CleanClaudeEnv` (then say what happens to `strippedKeys`/`StrippedEnv` and its doc) and applying a separate `LYX_TRACE_ID` filter at the `cmd.Env = clean` site (`lifecycle.go:355`).

### [NOTE] Integration test has no named Info+ trigger
**Section:** Testing → "One `//go:build integration` test"
**Issue:** Every existing Info/Warn emitter needs live substrate (burler round `engine.go:108`, shuttle run `run.go:167`, treadle judge, fabric's diverged-push `coalesce.go:86`), so "provoking an Info+ line" from a freshly built `lyx` child in a fixture worktree has no cheap, deterministic trigger.
**Fix:** Name the exact command and condition the child runs (or add one deliberate Info at a cheap, always-reached point) so the test's feasibility is settled before planning.

### [NOTE] "Two stderr sites" is two files, six call sites
**Section:** Scope → In / Technical context
**Issue:** `internal/scoutengine/lspclient.go` has five diagnostic `fmt.Fprintf(os.Stderr, …)` calls (596, 599, 604, 627, 630) and `ensureserver.go` one (465); "two sites, not three" counts files, under-sizing the conversion by four.
**Fix:** Say "two files, six call sites" and note that `lspclient.go:211`/`ensureserver.go`'s child-stderr wiring is not part of it.

## Verdict

GAPS_FOUND
Five gaps: Once conflation, contradictory fan-out test, spawn-rule mis-classification, unscheduled scout logging, StrippedEnv fallout.
MILL_REVIEW_END
