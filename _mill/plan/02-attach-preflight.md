# Batch: attach-preflight

```yaml
task: 'reed: AddStrand and attach self-heal a cold worktree'
batch: 'attach-preflight'
number: 2
cards: 1
verify: go test ./internal/reedcli/ ./internal/reedengine/
depends-on: [1]
```

## Batch Scope

This batch wires the second and last self-heal call site: `lyx reed attach`'s CLI pre-flight gains `EnsureSession` ahead of the `Status` call it already makes.
It is its own batch because it is the only change outside `internal/reedengine` that alters behaviour rather than prose, and because it consumes batch 1's exported seam.

Batch-local decision: the change is strictly additive.
Nothing is swapped out, nothing is reordered after the insertion point, and the terminal-handover tail is untouched.
`reed attach` is a registered interactive-handoff exception under CONSTRAINTS.md's CLI/Cobra Invariant, so the new fallible step must sit in the pre-flight, on the JSON envelope, and never migrate into the handover tail.

## Cards

### Card 5: attach pre-flight boots before it reports

- **Context:**
  - `internal/reedengine/lifecycle.go`
  - `internal/reedengine/attach.go`
  - `internal/reedengine/spawn.go`
  - `internal/reedcli/up.go`
  - `internal/logger/`
- **Edits:**
  - `internal/reedcli/attach.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  In the attach subcommand's run function, insert an `EnsureSession` call on the engine immediately before the existing `Status` call, keeping that `Status` call and its envelope-error handling exactly as they are.
  An error from `EnsureSession` aborts the same way the `Status` error does today — the same exit-setting and envelope-error pair, followed by an immediate return — so a boot failure reaches the operator on the envelope before any stdio handover.
  When `EnsureSession` reports that it booted, log once at `Info` via the already-imported logger package, naming the attach verb as the cause alongside the engine's socket and session name.
  Log nothing on the warm path.

  The ordering is load-bearing and must be commented as such: `Status` calls `requireSessionLocked`, so running it first would refuse on a cold worktree before anything booted, reinstating the bug this task removes.
  The comment must also say why `Status` is kept rather than replaced — it reaches `loadOrInitStateLocked`, which is what makes a warm attach refuse on an unreadable `.lyx/reed.json` and on a live foreign session, and `EnsureSession`'s early return reads no state at all.

  Rewrite the file's header comment, whose final sentence currently claims the engine's `Status` call is the only pre-flight step that can abort with an envelope error.
  There are now two such steps, in a fixed order, and the sentence must say so.
  Keep the rest of that header — the terminal-size read and the `AttachArgv` builder call still degrade rather than abort, and that distinction is what the sentence exists to draw.

  Extend the subcommand's long help text to say that attach boots this worktree's session when none is up, rather than refusing.
  Leave its use line and short description unchanged.
  Nothing below the terminal-size read is touched: the handover command, its stdio wiring, and the exit-code propagation tail all stay exactly as they are.
- **Commit:** `feat(reedcli): attach boots a cold worktree's session before its status pre-flight`

## Batch Tests

`verify: go test ./internal/reedcli/ ./internal/reedengine/` runs the untagged tier of the package this batch edits plus the engine package it consumes.
`internal/reedcli`'s untagged tier is small — `cli_test.go` and `header_test.go` — and covers no live-tmux behaviour, so it functions here as a compile-and-wiring gate rather than a behavioural one: it catches a signature or import mistake against batch 1's new exported method immediately.
`internal/reedengine` is included because this batch's correctness depends on batch 1's seam still behaving, and re-running it at this boundary catches a cross-package regression at the batch that introduced it.
The real behavioural coverage for this card — cold attach boots, warm attach keeps both of its refusals, warm attach reaps no operator pane and writes no state — is `smoke`-tagged and lives in batch 5.
