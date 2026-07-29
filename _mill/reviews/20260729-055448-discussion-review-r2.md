MILL_REVIEW_BEGIN
# Review: codeintel V1 — LSP-backed lookups (Go-only, CLI + EnsureServer)

```yaml
verdict: GAPS_FOUND
reviewer_model: sonnet
reviewed_file: _mill/discussion.md
date: 2026-07-29
```

## Findings

### [GAP] Toolchain-install concurrency race is out of the stated lock's scope
**Section:** Decisions > toolchain-manager-authority (Cache-dir root) vs. concurrency-locking; Scope > "Concurrency safety for EnsureServer"
**Issue:** The toolchain cache is explicitly "machine-global and shared across every worktree/hub on the machine," but the only concurrency mechanism decided (`internal/lock` via `TryAcquireWriteLock`) fences "EnsureServer calls racing to spawn the same worktree's daemon" — worktree-scoped. Two lyx processes in two different worktrees both cold-starting Go's toolchain install at once race on the same cache path with no fencing discussed.
**Fix:** State whether the toolchain-install step gets its own machine-global lock (e.g. a lock file inside the cache dir itself) or relies on some named atomicity guarantee of `go install`'s output step.

### [GAP] native's reconnect behavior is internally contradictory and its daemon-address discovery is unspecified
**Section:** Decisions > supervised-reconnect-transport (native aside) vs. native-strategy-wire-compatibility; Testing > native wire-compatibility
**Issue:** native-strategy-wire-compatibility states native needs "zero wire-protocol changes... only the launch argv changes" (every call re-spawns `gopls -remote=auto`, which itself attaches cheaply). supervised-reconnect-transport then says native's reconnect path (every call after the first) "reuses this exact same dial-transport code" — i.e. dials the daemon's socket directly instead of respawning — but that address is auto-resolved by gopls itself, not chosen by lyx as in `supervised`, and no discovery mechanism is given. The listed "native wire-compatibility" test only re-verifies gopls's own dedup across two spawned clients, never a lyx-side direct-dial reconnect.
**Fix:** Pick one: either native keeps respawning `gopls -remote=auto` per call (strike the "reuses... code" claim), or specify how `EnsureServer` learns the auto-resolved socket address to record/dial, plus matching test coverage.

### [GAP] Batch-mode argument cardinality for definition/symbol is unaddressed
**Section:** Scope > In (batch-mode bullet); Decisions > batch-mode-cli
**Issue:** batch-mode-cli only changes `refs` from `cobra.ExactArgs(1)` to `MinimumNArgs(1)` (verified against current `internal/codeintelcli/cli.go`); nothing states whether the newly-added `definition`/`symbol` verbs also take multiple positional symbols, and the per-symbol JSON shape described (`"references": [...] | "candidates": [...]`) is refs-specific.
**Fix:** State whether `definition`/`symbol` get the same batch treatment, and if so, name their per-symbol JSON field(s).

### [NOTE] plan-format-v3.md's cross-reference to codeintel's roadmap status is stale
**Section:** Technical context > External design references (docs/reference/plan-format-v3.md)
**Issue:** That doc's "Deferred / forward-compat" section says the symbol fields wait on codeintel, "which is deprioritized (see the roadmap's Someday list)" — but codeintel was promoted to Planned (`manifest/roadmap.md` line ~18), so the cross-reference is now inaccurate.
**Fix:** Not this task's fix (consumer wiring is explicitly out of scope), but worth a one-line correction whenever that slice lands.

## Verdict

GAPS_FOUND
Three GAPs: toolchain-install cross-worktree locking, native reconnect self-contradiction, batch-mode verb scope.
MILL_REVIEW_END
