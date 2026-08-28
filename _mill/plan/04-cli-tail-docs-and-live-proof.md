# Batch: cli-tail-docs-and-live-proof

```yaml
task: 'reed: watchdog daemon'
batch: 'cli-tail-docs-and-live-proof'
number: 4
cards: 6
verify: go test ./internal/reedcli/... ./internal/reedengine/... ./cmd/lyx/... && go vet -tags integration ./internal/reedengine/...
depends-on: [3]
```

## Batch Scope

This batch turns the engine-side loop into shipped behaviour and closes the task's documentation and acceptance obligations: the header pane's `--blocking` tail calls `Engine.Watch` behind a discarded stderr sink and then still parks unconditionally; `reed header`'s `Long` text says the pane self-heals the layout; `internal/reedengine/doc.go` records the load-bearing behavioural assumptions this design rests on; `manifest/roadmap.md`'s Someday entry is amended in place; the reed sandbox suite gains the operator-assisted resize scenario that would have caught M7; and a new `integration && linux` pty test reproduces M7 end to end in both directions.

It is one batch because every card here is a consumer or a record of the same finished mechanism, and the documentation-lifecycle rule requires the docs to land in the same task as the behaviour they describe.

Batch-local decision: the header tail's two moving parts — the watch call and the park — are routed through package-level function variables so `header_test.go` can substitute both and assert the keepalive-survival contract without an infinite loop.
This is the smallest seam that makes the discussion's "the tail reaches `blockForever()` rather than falling out of `RunE`" a real assertion rather than a review note.

## Cards

### Card 19: call Watch from the header pane's blocking tail

- **Context:**
  - `internal/reedengine/watchloop.go`
  - `internal/reedcli/cli.go`
  - `internal/logger/logger.go`
  - `cmd/lyx/helptree_test.go`
  - `cmd/lyx/seamsignature_test.go`
  - `CONSTRAINTS.md`
- **Edits:**
  - `internal/reedcli/header.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Rework the `--blocking` tail of `headerCmd` in `internal/reedcli/header.go`.

  Add two package-level function variables above `headerCmd`, each with a godoc line stating it is a var so `header_test.go` can substitute it:

  ```go
  // headerWatch is the resize self-heal loop the blocking tail enters after painting
  // the header text. A package var so header_test.go can substitute a fake that
  // returns, and assert the tail still reaches headerPark.
  var headerWatch = func(ctx context.Context, eng *reedengine.Engine) error { return eng.Watch(ctx) }

  // headerPark is the keepalive park the blocking tail ends on, unconditionally.
  var headerPark = blockForever
  ```

  Change the `if blocking { ... }` body to, in this exact order:

  1. Print the rendered text as it does today, unchanged: `fmt.Fprint(out, "\x1b[2J\x1b[H"+strings.TrimRight(text, "\r\n"))`.
  2. `logger.SetOutput(io.Discard)`.
  3. `if err := headerWatch(cmd.Context(), c.eng); err != nil { logger.Warn("reed: header pane watch loop returned", "err", err) }`.
  4. `headerPark()`.

  Step 2 is load-bearing and must carry a comment saying so: the header pane's stdout/stderr **is** its visible screen, `internal/logger`'s stderr half defaults to `slog.LevelWarn`, and the watcher reaches already-shipped `Warn` call sites (`liveBoxLocked` on a failed or malformed window-size query, `pinGeometryOptionsLocked` on a failed pin), so without this the first degraded tmux round trip paints a slog line over the operator console.
  Only the stderr half is discarded — `logger.SetOutput` rebinds that half alone, and the durable handler is enabled unconditionally at `Info` and above, so nothing is lost for diagnosis, it just stops being drawn.

  Step 3's error handling must carry a comment saying that a non-nil return is **logged only**: never `output.Err`, never `fmt.Fprint`, never anything written to stdout or stderr, because the pane's stdio is its screen.

  Step 4's unconditional call must carry a comment saying it is deliberate redundancy rather than dead code: `Watch` never returns while the pane must live, and this guarantees that no future edit to `Watch` can make `RunE` fall through and kill the keepalive pane — the one failure this design must never permit.

  Add `context`, `io`, and the `internal/logger` and `internal/reedengine` imports as needed; keep `time` (still used by `blockForever`) and every existing import that is still used.
  Do not change the non-blocking branch, the `HeaderText` call, the `clihelp.ShouldAbort` guard, or the `--blocking` flag registration.

  Also update the command's `Long` text: add one sentence to the paragraph describing `--blocking`, stating that the blocking pane additionally runs reed's resize self-heal watch loop, which re-applies the planned layout after the terminal window is resized, and that it is turned off with `watchdog: off` in `reed.yaml` followed by `lyx reed down` + `up`.
  Leave `Use` and `Short` untouched — `cmd/lyx/helptree_test.go` and `cmd/lyx/seamsignature_test.go` must stay green unchanged.
- **Commit:** `feat(reed): run the resize watch loop from the header pane's blocking tail`

### Card 20: tier-1 tests for the header tail

- **Context:**
  - `internal/reedcli/header.go`
  - `internal/reedcli/cli.go`
  - `internal/reedcli/cli_test.go`
  - `internal/reedengine/lock.go`
  - `internal/reedengine/config.go`
  - `internal/reedengine/geometry.go`
  - `internal/reedengine/header.go`
  - `internal/logger/logger.go`
  - `CONSTRAINTS.md`
- **Edits:**
  - `internal/reedcli/header_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Extend `internal/reedcli/header_test.go` (untagged), keeping its two existing tests unchanged.

  Substitute `headerWatch` and `headerPark` in each new test and restore both via `t.Cleanup`, recording how many times each ran.

  Every test that drives the blocking path must **also** restore the logger's stderr sink, because card 19's tail calls `logger.SetOutput(io.Discard)` and that rebind is process-global — left unrestored it would silence the discarding sink for the rest of the `internal/reedcli` test binary.
  Add `t.Cleanup(func() { logger.SetOutput(os.Stderr) })` to each such test, matching the pairing every other `logger.SetOutput` call site in this repo already uses.

  Cover:

  - The tail reaches the park after a nil-returning watch: run the `header --blocking` command against a capture writer, assert `headerWatch` ran exactly once and `headerPark` ran exactly once after it.
  - The tail reaches the park after an **error**-returning watch: identical assertions, plus that the command's `RunE` returned nil and that nothing beyond the rendered header text was written to the capture writer — no JSON envelope, no error text.
    This is the keepalive-survival assertion: an obvious implementation propagates the error and kills the pane.
  - The non-blocking mode is unaffected: `headerWatch` and `headerPark` both ran zero times and the JSON envelope is emitted as before.
  - `Long` mentions the self-heal behaviour: assert the string contains `watchdog`.

  **The `c.eng` fixture is mandatory, not optional.** `headerCmd`'s `RunE` calls `c.eng.HeaderText()` unconditionally, before the `if blocking` branch is reached at all, and `HeaderText` dereferences `e.cfg` immediately — so the two existing tests' `&reedCLI{}` shape, which never runs `RunE`, cannot be carried into these new tests: every one of them would panic on a nil `*Engine` before `headerWatch` or `headerPark` was ever called.
  Build each new test's CLI as `&reedCLI{eng: reedengine.New(reedengine.Config{}, reedengine.Geometry{RepoName: "test-repo", HubPath: t.TempDir()})}`.
  An empty `Config.Header.Template` makes `HeaderText` fall back to the embedded default template, and `RepoName`/`HubPath` are the only two `Geometry` fields `tokenvocab.Ctx` consumes, so this renders cleanly with no filesystem or process I/O.
  Drive the command with `cmd.SetOut(buf)`, `cmd.SetArgs([]string{"--blocking"})`, and `cmd.Execute()`; `clihelp.ShouldAbort` and `clihelp.SetExit` are both nil-safe on a bare `context.Background()`, so no `clihelp` context seeding is needed.
  Leave `TestHeaderCmd_UseAndShort` and `TestHeaderCmd_BlockingFlagRegistered` on their existing `&reedCLI{}` shape — they still never run `RunE`.
  Update the file-header comment, which currently states the file "never runs RunE/PreRunE and never invokes the --blocking path, since that path blocks forever by design": that is no longer true now that `headerPark` is substitutable, so reword it to say the blocking path is exercised with both function vars stubbed.
- **Commit:** `test(reed): assert the header tail always parks, even when the watch loop errors`

### Card 21: record the watchdog's load-bearing assumptions in doc.go

- **Context:**
  - `internal/reedengine/watchdog.go`
  - `internal/reedengine/watchloop.go`
  - `internal/reedengine/reapply.go`
  - `internal/reedengine/windowsize.go`
  - `internal/reedengine/apply.go`
  - `internal/reedengine/headerpane.go`
  - `internal/reedengine/lifecycle.go`
  - `internal/reedcli/header.go`
  - `CLAUDE.md`
  - `CONSTRAINTS.md`
- **Edits:**
  - `internal/reedengine/doc.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Add new bullets to `internal/reedengine/doc.go`'s existing "Load-bearing behavioral assumptions, each with the rationale that makes it load-bearing" list, in that list's established voice and wrapping.
  Each bullet names the file it governs, states the verified fact, and states what breaks without it.

  Add one bullet each for:

  - **`window-resized` is the only usable resize event source** (`windowsize.go`, `watchdog.go`).
    On a client resize the hooks fire `client-resized` → `window-layout-changed` → `window-resized`; `client-resized` reports the **stale** pre-resize window size, so it cannot plan a correct layout, and `window-layout-changed` is self-triggering, so reed's own `select-layout` would re-enter the watcher in an infinite loop.
    `window-resized` fires exactly once per settled size, after the window already has the new geometry, on both growth and shrink.
  - **SIGWINCH is not a substitute** (`reedcli/header.go`'s blocking tail).
    With the header pinned to one row, growing the window delivers SIGWINCH every time — and that growth *is* the layout bug — but **shrinking delivers nothing** while the strand budgets below are silently violated (at 30 rows the bottom strand had been squeezed from 15 rows to 2).
    A watcher that self-heals only on growth is worse than none, because the operator learns to trust it.
  - **`select-layout` does not fire `window-resized`** (`apply.go`, `reapply.go`).
    Verified in all four cases — attached, detached, re-applying an identical layout, and the documented detached over-budget apply that genuinely grew the window 40 → 60 rows — zero fires each time.
    So `window-resized` tracks *client-driven* size changes, not layout-driven ones, which is exactly the property that makes it usable where `window-layout-changed` is not.
    The box-equality guard inside `reapplyLayout` is kept anyway: the probe settles tmux 3.6 but not psmux, and a silent infinite loop inside the session keepalive is the worst available failure mode.
  - **The plain `set-hook` form replaces; `-a` accumulates** (`windowsize.go`).
    Four identical plain installs yield exactly one fire per resize; three further `-a` appends yield four.
    `pinGeometryOptionsLocked` runs on every `AttachArgv` pre-flight as well as at boot, so `-a` would cost N `run-shell` spawns per resize after N attaches.
  - **The hook readback is `show-options`, not `show-hooks`** (`reapply.go`).
    In tmux 3.6 hooks are options, and `show-hooks` prints nothing for a session-scoped hook that demonstrably fires — a `show-hooks`-based probe would report "no hook" every time and pin every watcher into poll mode.
  - **`run-shell` without `-b` blocks the tmux server** (`watchdog.go`).
  - **`liveBoxLocked` never reports failure through its box** (`windowsize.go`, `reapply.go`).
    A degraded query returns the configured `cfg.Width`/`cfg.Height` pair, which is a perfectly plausible-looking box, so any caller comparing boxes across calls must consume the method's second return value — otherwise a fallback that happens to equal the last applied box skips forever and one that differs re-applies forever.
  - **The header pane's stdout/stderr is its screen** (`reedcli/header.go`).
    The `--blocking` tail rebinds the logger's stderr sink to a discarding writer before entering the loop; the durable sink is untouched.
  - **`testing.Testing()` gates the header launch line** (`headerpane.go`, `lifecycle.go`).
    No Go test can exercise a header-hosted watch loop by booting a header pane, which is why the tier-2 proof runs the loop in-process against a real session instead.

  Do not restructure the existing list, reword existing bullets, or move the file's other sections.
- **Commit:** `docs(reed): record the resize watchdog's load-bearing assumptions in doc.go`

### Card 22: amend the roadmap's watchdog entry in place

- **Context:**
  - `CLAUDE.md`
  - `_mill/discussion.md`
- **Edits:**
  - `manifest/roadmap.md`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Amend the **reed: watchdog daemon** entry's prose in `manifest/roadmap.md` in place.
  It stays exactly where it is, in the Someday section — **no section move**: CLAUDE.md restricts roadmap movement to completing or adding a *Planned* item, this is a Someday entry, and only one of its two halves ships here.

  The rewritten entry must:

  - Drop the "a standalone per-worktree daemon" description and replace it with what actually shipped: a watch loop hosted **inside the existing per-worktree header pane**, driven by a session-scoped `window-resized` tmux hook that touches a signal file, with a slow poll fallback where the hook cannot be verified to install (Windows/psmux), a trailing-edge debounce, and a `watchdog: on|off` key in `reed.yaml`.
    The correction matters because "standalone" is now the opposite of the truth, and the remaining half will be built on this host.
  - State that the **resize-geometry reconcile half is done**, so a resize while already attached now re-applies the planned layout with no `lyx reed` op.
  - State that the **pane-reap half remains**, together with its stated prerequisite — cheapening the reap probe, today a fresh pwsh plus full `Win32_Process` WMI enumeration per poll — and that its real open question is the policy distinguishing a bug-induced pane from an intentional scratch pane, which is unwritten.
  - Keep the entry's existing pointer to the Slack-relay item below it intact and keep every existing inline markdown link resolving (`cmd/lyx`'s Markdown Link Integrity guard checks file parts and `#anchor`s under `manifest/`).

  Follow this repo's markdown rule: one sentence per line, and break long sentences at internal independent-clause boundaries.
  Never hard-wrap at a fixed column.
  Do not touch any other roadmap entry and do not move anything into or out of the Done section.
- **Commit:** `docs(roadmap): amend the reed watchdog entry for the shipped resize half`

### Card 23: add the resize self-heal sandbox scenario

- **Context:**
  - `cmd/lyx/sandbox_coverage_test.go`
  - `CONSTRAINTS.md`
- **Edits:**
  - `tools/sandbox/SANDBOX-REED-SUITE.md`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Add a new scenario `### M26 -- Resize self-heal (operator-assisted visual)` to `tools/sandbox/SANDBOX-REED-SUITE.md`, placed after the existing `### M25 -- Down names the session it abandons` section and before the `## Session log format` heading, separated by the same `---` rule the other scenarios use.

  Model it on M7's shape — `**Goal:**`, `**Watch:**`, `**Verdict:** \`OK\` / \`WARN\` / \`FAIL\`` — and carry a `**Covers:** reed` tag so the Sandbox Suite Coverage guard sees it.

  Its `**Watch:**` must instruct the operator to: attach with `lyx reed attach` in a second terminal against a session holding a header pane and at least two strands; confirm the header is exactly `header.height_rows` tall and the strand budgets look right; then **drag the terminal window larger** and confirm the layout re-applies within about a second with no `lyx` command run; then **drag it smaller** and confirm the same.
  It must state explicitly that the shrink direction is the non-negotiable half — it is the one SIGWINCH misses entirely — and that a header that grows past its configured row count, or a bottom strand squeezed below `min_full_rows`, is a `FAIL`.
  It must also have the operator confirm that the cursor did **not** jump to another pane across either resize (the focus-steal regression), and that typing into a pane before a resize leaves the same pane focused after it.

  Also add the two missing session-log lines to the `## Session log format` block, which currently stops at `M24`: append `M25: <OK|WARN|FAIL> -- <one-line note if not OK>` and `M26: <OK|WARN|FAIL> -- <one-line note if not OK>` after the `M24:` line.
  M25's absence is a pre-existing gap; leaving the template listing M24 then M26 would be worse than fixing both while this file is open.

  Follow this repo's markdown rule: one sentence per line, no fixed-column hard wrap.
- **Commit:** `docs(sandbox): add the reed resize self-heal scenario M26`

### Card 24: live pty proof that the layout self-heals in both directions

- **Context:**
  - `internal/reedengine/attachgeometry_integration_test.go`
  - `internal/reedengine/mouse_boot_integration_test.go`
  - `internal/reedengine/contract_integration_test.go`
  - `internal/reedengine/watchloop.go`
  - `internal/reedengine/watchdog.go`
  - `internal/reedengine/reapply.go`
  - `internal/reedengine/windowsize.go`
  - `internal/reedengine/apply.go`
  - `internal/reedengine/lifecycle.go`
  - `internal/reedengine/overlay.go`
  - `internal/reedengine/config.go`
  - `internal/reedengine/render/types.go`
  - `internal/shell/shell.go`
  - `CONSTRAINTS.md`
- **Edits:** none
- **Creates:**
  - `internal/reedengine/watchdog_integration_test.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** Create `internal/reedengine/watchdog_integration_test.go` with the build constraint `//go:build integration && linux` on its first line, followed by a file-header comment stating that this file is the live reproduction of the M7 resize defect and that it is Linux-only for the same reason `attachgeometry_integration_test.go` is — the pty harness is built directly on `golang.org/x/sys/unix`'s `/dev/ptmx` ioctls, which have no portable equivalent, and psmux's behaviour under a real pty is unverified anywhere in this repo.

  **Reuse, do not re-create.** `startInPTY`, `setupAttachGeometryFixture`, `waitForClientAttached`, `windowSizeNow`, `windowLayoutNow` (all in `attachgeometry_integration_test.go`) and `waitUntil` (in `contract_integration_test.go`) are all in this same package and same build-tag set; call them.
  Write no second pty harness.

  **Drive the loop in-process.** `lifecycle.go` passes `testing.Testing()` as `underTest` to the header launch line, so under `go test` the header pane is a bare shell and no Go test can boot a header-hosted watcher.
  Start the loop instead as a goroutine calling `e.watchLoop(ctx, t)` with a `context.WithCancel` context cancelled in `t.Cleanup`, and a `watchTiming` built from `watchDefaultTiming()` with only the durations shortened where a test would otherwise be needlessly slow.
  Set `e.cfg.Watchdog = "on"` on the fixture engine before starting, and call `e.pinGeometryOptionsLocked()` under the op lock (or run one op that reaches it) so the hook is genuinely installed against the live session.

  Resize the client by driving `unix.TIOCSWINSZ` on the pty master and sending `SIGWINCH`, exactly as the existing file already does.
  Assert every observable from **outside** the pty, via the engine's own `TmuxCmd`, using `waitUntil` with a bounded timeout rather than a fixed sleep.

  The file must cover, as separate `Test...` functions:

  1. **Grow self-heals.** Boot the fixture, attach a pty client of a known size, let the layout settle, start the loop, grow the window, and assert within a bounded wait that `#{window_layout}` becomes the string the engine plans for the new box — specifically that the header pane is back to exactly `cfg.Header.HeightRows` rows.
  2. **Shrink self-heals.** The same, shrinking.
     This case is non-negotiable: it is the one SIGWINCH misses entirely, and a watcher passing only the grow case is the failure mode this task must not ship.
  3. **A burst coalesces.** Drive a rapid succession of size changes and assert the layout converges to the final size, and that the number of `select-layout` calls observed is far below one-per-event — count them by wrapping the engine's `execHook`-free path with a `#{window_layout}` sampling loop, or by asserting the settle happens once rather than repeatedly.
  4. **The degraded path still converges.** With the hook made uninstallable (unset it after boot and script nothing else), the poll fallback still heals the layout after a resize, within a bounded wait proportional to `PollCycle`.
  5. **The loop survives an induced tmux failure.** Kill the session out from under the loop, wait past several ticks, assert the goroutine has not returned, then boot a fresh session and assert the loop is still functioning.
  6. **Focus is never stolen.** Select a specific pane as live-active before the resize (`select-pane` on a pane that is *not* the persisted table's focus strand), resize, and assert `#{pane_active}` still names that same pane afterwards.
     Only a real session can demonstrate this.
  7. **No self-trigger loop.** After the watcher's own apply settles, assert `#{window_layout}` stops changing and no further apply occurs without a new client resize — the live counterpart of the box-equality guard and of the `select-layout`-fires-nothing probe.
  8. **The hook probe matches against a live tmux.** After `pinGeometryOptionsLocked` has run against the real session, assert `hookInstalledLocked()` reports `(true, true)`.
     This is the assertion that catches any tmux-side normalisation of the hook command string that would silently pin every watcher into poll mode on Linux, which no tier-1 test can see.

  Skip cleanly when the configured multiplexer binary is absent, the same way `newIntegrationEngine` already does.
  Do not add a `TestMain` — the package already has one.
- **Commit:** `test(reed): prove the resize self-heal live against a real pty in both directions`

## Batch Tests

`verify: go test ./internal/reedcli/... ./internal/reedengine/... ./cmd/lyx/... && go vet -tags integration ./internal/reedengine/...`

Three packages, each for a reason.
`./internal/reedcli/...` covers card 19's tail change and card 20's new tests, plus the existing untagged reed CLI suite (`cli_test.go`'s help-shape assertions are the ones the `Long` edit could disturb).
`./internal/reedengine/...` re-runs batches 1–3's coverage against the finished tree.
`./cmd/lyx/...` is required rather than optional: this batch's doc and sandbox edits are exactly what three guards in that package check — `sandbox_coverage_test.go` (the `**Covers:** reed` tag), the Markdown Link Integrity guard over `manifest/` (card 22's roadmap edit), `helptree_test.go` and `seamsignature_test.go` (card 19 must leave both green), and `tierpurity_test.go`/`tiersleep_test.go` (cards 20 and the untagged tests from batches 1–3).

`go vet -tags integration` is appended because card 24's file is build-tagged and would otherwise never be compiled by this batch's verify at all; vet compiles it without needing a live tmux server, which is what catches a signature or import mistake at the introducing batch instead of at the done gate.
The full tagged run — `go test -tags integration ./...` — is the hub's configured `pipeline.done_gate` and is where the live pty assertions actually execute.
