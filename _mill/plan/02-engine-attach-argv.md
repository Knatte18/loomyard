# Batch: engine-attach-argv

```yaml
task: 'reed: attach doesn''t reconcile session geometry with the terminal'
batch: 'engine-attach-argv'
number: 2
cards: 2
verify: go test ./internal/reedengine/...
depends-on: [1]
```

## Batch Scope

This batch delivers the single attach-argv builder both CLIs will call in batch 3: `Engine.AttachArgv(cols, rows int) []string`, the first exported reed engine method that returns no error.
It owns the whole pre-flight — the two option pins, both readbacks, the state read, the pane list, the guards, and the layout plan — inside one `withOpLock` acquisition, and it degrades to today's bare `attach-session` argv on every failure.

The external interface batch 3 consumes is exactly `AttachArgv`;
nothing else in this batch is exported.
This batch touches no CLI, no `go.mod`, and no file outside `internal/reedengine/`.

Batch-local decision: `AttachArgv` calls `withOpLock` exactly once, blocking, the same way the `Status()` call already standing in both CLIs' pre-flight does.
`withOpLock` is documented non-reentrant and is the package's single acquisition point, so every step must sit inside that one closure — never a nested acquisition, and never a second exported call the CLI has to remember.

## Cards

### Card 6: Engine.AttachArgv — the single attach argv builder

- **Context:**
  - `internal/reedengine/windowsize.go`
  - `internal/reedengine/apply.go`
  - `internal/reedengine/overlay.go`
  - `internal/reedengine/lock.go`
  - `internal/reedengine/spawn.go`
  - `internal/reedengine/lifecycle.go`
  - `internal/reedengine/render/types.go`
  - `internal/reedcli/attach.go`
- **Edits:** none
- **Creates:**
  - `internal/reedengine/attach.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  Create `internal/reedengine/attach.go` in `package reedengine`, with a file-header comment stating that this file owns the attach argv both `internal/reedcli` and `internal/loomcli` build their terminal handover from, and that the builder never refuses.
  Declare three identifiers:

  1. `func bareAttachArgv(socket, session string) []string` — returns exactly `[]string{"-L", socket, "attach-session", "-t", exactSessionTarget(session)}`, the five-element argv both CLIs build today.
     Build a fresh slice on every call;
     never return a shared package-level slice, since the caller may append to it.
  2. `func chainedAttachArgv(socket, session, layout string) []string` — returns the ten-element argv: the five elements of `bareAttachArgv`, then the literal one-character element `";"`, then `"select-layout"`, `"-t"`, `exactSessionWindowTarget(session)` and `layout`.
     The separator is a literal single-character `;` argv element, never `"\\;"` — `exec.Command` passes argv directly and never sees a shell, so a backslash would be passed through as a literal backslash-semicolon and tmux would not read it as a command separator.
     Build the result without aliasing `bareAttachArgv`'s backing array (copy into a slice of the final length, or allocate with the full capacity).
     The chained `select-layout` carries its own explicit `-t =<session>:` target rather than relying on whichever window the new client lands in, matching the exact-target discipline every other reed call site follows.
  3. `func (e *Engine) AttachArgv(cols, rows int) []string` — the exported builder.
     It returns no error, by contract: attach is the operator's escape hatch into a session, including a broken one, so no engine-side failure may ever block the handover.
     Document that contract in the method's godoc, together with the meaning of `cols`/`rows` (the attaching client's own terminal size in columns and rows;
     a non-positive value means no client size is known).

  `AttachArgv`'s body must have this shape and this order:
  - Compute `bare := bareAttachArgv(e.Socket(), e.SessionName())` up front, and return it from every degraded path below.
  - If `cols <= 0` or `rows <= 0`, log via `logger.Warn` that no client terminal size is available and return `bare` immediately, without taking the lock.
  - Otherwise call `e.withOpLock(func() error { ... })` exactly once, and inside that closure, in this order:
    1. `e.requireSessionLocked()` — return its error on failure, which degrades to `bare`.
    2. `e.pinGeometryOptionsLocked()` — the pins are made here, by the builder itself, not by a second exported call a CLI must remember.
       The ordering is load-bearing: the told box is only correct once `status off` has landed, since that is what makes the post-attach window equal the client's rows rather than `rows - 1`.
    3. `if !e.readWindowSizeLatestLocked()` — suppress the chain.
       Anything other than `latest` means the post-attach window will not become the client's size, so the told box's whole premise has failed and chaining would hand tmux a wrong-height string to rescale — worse than not chaining.
    4. `reserved, ok := e.readStatusRowsLocked()`;
       `if !ok` — suppress the chain.
       A `#{status}` that reads back as something other than `off` does NOT suppress it: the reserved-row count is simply taken from that value instead.
    5. `st, err := e.loadOrInitStateLocked()` — return its error on failure.
       Never call `SaveState`: this builder is read-only with respect to `reed.json`.
    6. `live, err := e.tmux.listPanes(e.SessionName())` — return its error on failure.
    7. Reproduce `applyLayoutLocked`'s two skip guards exactly: suppress the chain when `len(live) < 2`, and when `!anyPlacedStrand(st.Strands, liveIDSet(live))`.
       These are load-bearing, not stylistic — a layout string enumerating zero panes is accepted by tmux (exit 0) and answered by destroying every pane in the session, and an attach that wipes the session it is attaching to would be a far worse bug than the one being fixed.
    8. `layout, _, err := e.planLayout(st, live, render.Box{X: 0, Y: 0, W: cols, H: rows - reserved})` — return its error on failure.
       The focus target is deliberately discarded: the chain carries `select-layout` only, never `select-pane`.
       The box is the TOLD client box;
       `liveBoxLocked` must not be called anywhere on this path, because at argv-build time the live window is still the pre-attach size and would be exactly the wrong answer.
    9. On success, assign `chainedAttachArgv(e.Socket(), e.SessionName(), layout)` to a variable captured from the enclosing scope.
  - Every suppression above is expressed as a returned error (a package-level sentinel such as `errAttachChainSuppressed`, declared in this file, keeps the reason legible), and every non-nil error out of `withOpLock` — including a lock-acquisition failure and the told-geometry validation `withOpLock` performs — results in one `logger.Warn` naming the reason and a `return bare`.
    A suppression is an expected, benign outcome, so word its warn line as a degradation rather than a failure.
  - When the closure succeeded, return the chained argv.

  Do not add `attach-session` or any other subcommand to `requiredSubcommands`.
  Do not give `AttachArgv` an `error` return, a timeout, or a non-blocking lock variant: `lock.AcquireWriteLock` blocks without a deadline, which is exactly what the `Status()` call already two lines earlier in both CLIs' pre-flight does, and "no engine-side failure blocks the attach" is about errors, not about waiting.
- **Commit:** `feat(reedengine): own the attach argv and chain a client-sized select-layout onto it`

### Card 7: tier-1 coverage for the argv shape and every skip guard

- **Context:**
  - `internal/reedengine/attach.go`
  - `internal/reedengine/windowsize.go`
  - `internal/reedengine/overlay.go`
  - `internal/reedengine/parse.go`
  - `internal/reedengine/lock_test.go`
  - `internal/reedengine/strand_test.go`
  - `internal/reedengine/render/types.go`
- **Edits:** none
- **Creates:**
  - `internal/reedengine/attach_test.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  Create `internal/reedengine/attach_test.go` in `package reedengine`, untagged (tier 1), building its engine with `newTestEngine(t)` and scripting every tmux round trip through `e.tmux.execHook`, in the shape `strand_test.go`'s kill-pane recorder already uses.
  Do not call `exec.Command`, and do not sleep.
  A hook must answer, at minimum: `has-session` (success, so `requireSessionLocked` passes), `set-option` (success), `display-message` for `#{window-size}` and `#{status}`, and `list-panes` with a scripted pane list in the six-field `#{pane_id} #{pane_dead} #{pane_top} #{pane_width} #{pane_height} #{pane_pid}` shape `parsePaneList` expects.
  Discriminate on the format argument, not on call order, so the tests do not silently pass when the call sequence changes.

  Cover:
  - **The chained shape.** With a known client size, `window-size latest`, `status off`, at least two live panes and a strand owning a present one, assert the argv has exactly ten elements, that elements 0-4 are the bare form, that element 5 is the one-character string `";"` (assert its length is 1, so `"\\;"` cannot pass), that elements 6-8 are `select-layout`, `-t` and the `=<session>:` target, and that element 9 is the layout string `planLayout` produces for `render.Box{W: cols, H: rows}`.
  - **The told box, and that no live query is made.** Set `e.cfg.Width`/`e.cfg.Height` to a pair distinct from the client size, and assert the emitted layout string carries the client's dimensions.
    Separately, assert the hook is never asked for `#{window_width} #{window_height}`: a hook that fails that specific query must not change the argv at all.
  - **Reserved rows.** With `#{status}` answering `on`, assert the layout is planned for `rows - 1`;
    with `2`, for `rows - 2`;
    with `off`, for `rows`.
  - **The chain gate.** `#{window-size}` answering `manual`, `largest`, garbage, or erroring yields the bare five-element argv;
    `#{status}` answering garbage or erroring yields the bare argv;
    `#{status}` answering `on` does NOT yield the bare argv (it is an input, not a gate).
  - **Every other degraded path yields exactly the bare argv**, asserted element by element against `[]string{"-L", <socket>, "attach-session", "-t", "=" + <session>}`: a zero or negative `cols`, a zero or negative `rows`, `has-session` failing, fewer than two live panes, no strand owning a present pane, a `list-panes` error, and a plan error (a strand carrying `render.AnchorOwnWindow`, which `render.Rules` rejects).
    This bare-argv assertion is where the two deleted CLI-side `TestAttachArgv` tests' coverage lands — their pinned `-L <socket> attach-session -t =<session>` expectation is reproduced here, so it is not lost with them.
  - **The pins are made by the builder.** Assert that a successful call issued both `set-option` invocations, and that the `status off` pin was issued before the `#{status}` readback — the ordering the told box depends on.
  - **No state write.** Assert the hook was never asked to run anything that would mutate the session (no `select-layout`, no `select-pane`, no `kill-pane`, no `split-window`), and that `reed.json` under `e.stateDir()` is not created or modified by the call.

  Give the file a header comment naming the session-wipe guard as the reason the guard cases are tested at all.
- **Commit:** `test(reedengine): pin the attach argv shape, its box source, and every skip guard`

## Batch Tests

`verify: go test ./internal/reedengine/...` — the same scope as batch 1, for the same reason: every file this batch touches lives under `internal/reedengine/`, and the untagged suite there is hermetic and fast.
It covers the file this batch creates (`attach_test.go`) plus every sibling suite that shares the engine helpers `AttachArgv` composes over (`apply_test.go`, `spawn_test.go`, `lifecycle_test.go`, `generation_test.go`, `state_test.go`), so a regression in `loadOrInitStateLocked`, `listPanes` parsing, or the layout guards surfaces here rather than at the CLI boundary.
Nothing outside this package can be affected: `AttachArgv` has no caller until batch 3.
