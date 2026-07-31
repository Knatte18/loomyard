# Batch: reedengine-trace-env-filter

```yaml
task: Diagnostic tracing (trace) on the logger module
batch: reedengine-trace-env-filter
number: 8
cards: 2
verify: go test ./internal/reedengine/...
depends-on: [2]
```

## Batch Scope

Adds the `LYX_TRACE_ID` filter at reed's tmux-server-boot `cmd.Env` assignment, per discussion.md's `long-lived-child-env` decision. Depends only on batch 2 (the `LYX_TRACE_ID` env-var name/precedence is defined there); does not depend on the durable sink or fan-out landing, since this filter concerns child-process environment hygiene, not logging output.

## Cards

### Card 32: Filter `LYX_TRACE_ID` at the tmux-server-boot `cmd.Env` site

- **Context:**
  - `internal/reedengine/env.go`
  - `internal/reedengine/state.go`
  - `internal/reedengine/state_test.go`
  - `internal/reedengine/lifecycle.go`
- **Edits:**
  - `internal/reedengine/lifecycle.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  In `internal/reedengine/lifecycle.go`, at the tmux-server-boot block (`clean, stripped := CleanClaudeEnv(os.Environ())` at line 338 through `cmd.Env = clean` at line 355), add a **separate, explicitly-named** filter that removes any `LYX_TRACE_ID` entry from `clean` before it is assigned to `cmd.Env` — do **not** widen `CleanClaudeEnv` (`internal/reedengine/env.go`, full file) to also strip `LYX_TRACE_ID`, per discussion.md's `long-lived-child-env` "Implementation note" paragraph: `CleanClaudeEnv`'s `strippedKeys` return value is persisted verbatim into `ReedState.StrippedEnv` (`internal/reedengine/state.go:48`, `json:"strippedEnv"` tag) at `lifecycle.go:646` and `:711`, and that field is documented and pinned (internal/reedengine/state_test.go, lines 26-32) as recording Claude-injected variables specifically — routing `LYX_TRACE_ID` through the same helper would silently change that diagnostic contract for an unrelated reason.

  Add a small local helper (e.g. `stripTraceID(env []string) []string` in `lifecycle.go`, following the same `strings.SplitN(entry, "=", 2)[0]`-then-compare shape `CleanClaudeEnv` already uses) and apply it to `clean` immediately before line 355's `cmd.Env = clean` — so the final environment excludes both the Claude-Code keys `CleanClaudeEnv` already strips and, separately, `LYX_TRACE_ID`. Leave `stripped` (the `CleanClaudeEnv` return value stamped into `StrippedEnv` at lines 646/711) completely untouched — this filter's removed key must **not** be added to `stripped` or `StrippedEnv`.

  This is the reed tmux server's boot path only. Per `long-lived-child-env`'s "Scout's daemon spawn is explicitly NOT in scope" paragraph, no equivalent filter is added at scoutengine's ensureserver.go daemon spawn (its child is a language-server binary, never `lyx`, and never reads `LYX_TRACE_ID`) — do not add one there. Also leave `internal/boardengine/spawn.go:27` and `internal/fabricengine/spawn.go:62` untouched — both are detached one-shots that must keep inheriting the trace ID (they are this invocation's own work in a child process, not a shared singleton reused by later, unrelated invocations).
- **Commit:** `feat(reedengine): strip LYX_TRACE_ID from the tmux server's environment without touching CleanClaudeEnv`

### Card 33: Long-lived child env test

- **Context:** none
- **Edits:**
  - `internal/reedengine/lifecycle_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  Add a test asserting on the **computed env slice** (not a real spawn) that the environment reed's tmux server boot would use excludes `LYX_TRACE_ID`, given one set in the parent process (`t.Setenv("LYX_TRACE_ID", "somevalue")` then call `CleanClaudeEnv(os.Environ())` followed by Card 32's `stripTraceID` helper, or whatever composed function the implementation exposes for this) — following `CleanClaudeEnv`'s own existing testability precedent (it is already unit-tested by asserting on its return value, not a real spawn).

  **Scope note, resolving a discrepancy between two sections of discussion.md:** its Testing section's "Long-lived child env" bullet says to also assert scout's daemon-spawn env excludes `LYX_TRACE_ID`, but its Decisions section's `long-lived-child-env` entry explicitly rejects stripping there ("Scout's daemon spawn is explicitly NOT in scope, and the earlier draft was wrong to include it" — the Testing bullet was not updated when review r3 corrected this). This card follows the Decision, not the stale Testing cross-reference: assert only on reed's tmux-server-boot computed env; add no scout-side assertion and no scout-side code change.
- **Commit:** `test(reedengine): cover LYX_TRACE_ID exclusion from the tmux server's computed environment`

## Batch Tests

`verify: go test ./internal/reedengine/...` runs the existing `state_test.go` (confirming `StrippedEnv`'s `CLAUDECODE`/`CLAUDE_CODE_*` contract is unchanged) alongside Card 33's new assertion.
</content>
