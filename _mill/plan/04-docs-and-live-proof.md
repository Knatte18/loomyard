# Batch: docs-and-live-proof

```yaml
task: 'reed: attach doesn''t reconcile session geometry with the terminal'
batch: 'docs-and-live-proof'
number: 4
cards: 3
verify: go test ./internal/reedengine/... && go test -tags integration -run TestAttachGeometry ./internal/reedengine/...
depends-on: [3]
```

## Batch Scope

This batch closes the task: it records the new contract where reed's readers look for it, and converts the operator's screenshot into a numeric assertion.
Two cards are documentation the Documentation Lifecycle requires in the same task — `internal/reedengine/doc.go` (reed's module doc;
reed has no `manifest/designs/reed.md`) and both embedded `reed.yaml` templates' `width`/`height` comments.
The third is the tier-2 proof: a build-tagged, Linux-only test that drives a real `attach-session` through a real pty of a deliberately different size and asserts the window dimensions and the exact `window_layout` string — the assertion that fails before this task and passes after.

Nothing downstream consumes an interface from this batch.

Batch-local decisions:
- The tier-2 test is tagged `integration`, not `smoke`.
  `integration` is what the hub's `pipeline.done_gate` (`go test ./... && go test -tags integration ./...`) actually executes, and it is the tag the package's existing live-tmux suites (`contract_integration_test.go`, `mouse_boot_integration_test.go`) already carry, so the new test reuses their `seedReedConfig`/`newIntegrationEngine`/`waitUntil` harness instead of building a second one.
- The build constraint also names `linux`, which is how the POSIX-only platform gate is expressed here: the pty harness is built directly on `golang.org/x/sys/unix`'s `/dev/ptmx` ioctls, so the file must not be compiled on Windows/psmux at all.
  A tag-based exclusion is stronger than a runtime `t.Skip` for a file that cannot compile off Linux, and it is why this file does not use the `t.Skip`-with-a-stated-reason shape `internal/reedcli/smoke_lifecycle_test.go` uses for its runtime-decidable premises.
  `golang.org/x/sys` is already a direct requirement, so this adds no dependency.

## Cards

### Card 12: record the live-geometry rule and the attach chain in reed's module doc

- **Context:**
  - `internal/reedengine/windowsize.go`
  - `internal/reedengine/attach.go`
  - `internal/reedengine/apply.go`
  - `internal/reedengine/lifecycle.go`
  - `internal/reedengine/probe.go`
- **Edits:**
  - `internal/reedengine/doc.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  Extend the package godoc in `internal/reedengine/doc.go` so it records what this task changed, in the file's existing voice — each claim carrying the rationale that makes it load-bearing, in the shape the "Load-bearing behavioral assumptions" list already uses.
  Add:
  - **The live-geometry rule**, as a new paragraph near the layout material: the render box is no longer the config-pinned `width`/`height`.
    `planLayout` is always TOLD its box and queries nothing;
    `applyLayoutLocked` resolves the live box with `display-message -p -t '=<session>:' '#{window_width} #{window_height}'` and falls back to the configured pair on any failure;
    `AttachArgv` passes the attaching client's own told size and never issues the live query, because at argv-build time the live window is still the pre-attach size.
  - **A new bullet in the load-bearing list: silent layout rescale.** `select-layout` with a layout string whose dimensions disagree with the live window exits 0 and silently rescales the layout proportionally (measured live on tmux 3.6: a `220x50` string applied to a `100x30` window turned a 3-row collapsed strip into 1 row), so every absolute row budget reed computes is scaled by `live_height / string_height` unless the string is sized to the live window.
    Note the detached counterpart in the same bullet: an OVER-budget string is not refused either — detached, tmux grows the window to fit the cells, so a client-less session can end up taller than its configured boot height until the next attach snaps it back;
    with a client attached, `window-size latest` holds the window at the client's size and tmux rescales the cells instead.
  - **A new bullet: the chained attach.** The attach argv is `attach-session … ; select-layout -t '=<session>:' <layout>`, with the separator a literal one-character `;` argv element (never `\;` — `exec.Command` passes argv directly and never sees a shell).
    The chained command runs after the client has attached and the window has already been resized to it, so the layout lands verbatim with no rescale.
    `attach-session` is first in the chain, so a failing or unsupported `select-layout` still leaves the operator attached — strictly no worse than before.
    State the stale-layout window explicitly: the string is planned under the op lock and applied seconds later, outside it, and when the pane count has changed tmux REFUSES the layout (exit 1, `have 3 panes but need 2`) and destroys nothing;
    when the count matches but membership shifted, cells apply positionally, so a strand is mis-sized rather than lost.
  - **A new bullet: the two geometry option pins.** `status off` and `window-size latest` are pinned session/window-targeted (`-t '=<session>:'`, and `-w` for `window-size`) at boot and again in the attach pre-flight, and their EFFECTIVE values are read back with `display-message`, because a `-g` pin plus exit 0 is not proof the option took — a session-scoped `status on` survives `set-option -g status off` with exit 0, and a window-scoped `window-size manual` survives the global `latest` the same way (both verified live).
    `#{status}` feeds the reserved-row count (`off`→0, `on`→1, N→N);
    a `#{window-size}` other than `latest`, or either readback erroring or returning an unrecognised value, suppresses the chain.
    State that both pins and both readbacks are NON-FATAL, unlike the `remain-on-exit`/`mouse` pins beside them, and why: those two are correctness dependencies, these two are geometry-quality options whose absence degrades to a working session, and psmux's support for them is unverified.
  - **One sentence** recording that `requiredSubcommands` did not grow: every subcommand this design spends is already listed, so there is no capability-probe change and no new psmux risk.
  - Extend the existing "Session targeting" paragraph's parenthetical lists so `select-layout`'s chained call and the new `display-message`/`set-option` targets are covered by the same exact-match grammar it already documents.

  Do not restate the whole design here — this is godoc, not a design doc.
  Do not edit `docs/overview.md`: its reed and loom entries describe the handover only as "hands the operator's stdio to a `tmux attach-session` child", which stays true when that child's argv gains a chained `select-layout`, and no module is added or removed from the module table.
- **Commit:** `docs(reedengine): record the live-geometry rule, the attach chain, and the two option pins`

### Card 13: restate what width and height mean in both reed.yaml templates

- **Context:**
  - `internal/reedengine/config.go`
  - `internal/reedengine/lifecycle.go`
  - `internal/reedengine/apply.go`
- **Edits:**
  - `internal/reedengine/template_posix.yaml`
  - `internal/reedengine/template_windows.yaml`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  Update the trailing `#` comments on the `width:` and `height:` keys in BOTH embedded templates so they say what those keys now mean: the size the tmux session is created at while no client has ever attached (`new-session -x/-y`), and the fallback render box used when the live window size cannot be read — no longer the box reed lays panes out against.
  Add that once a client attaches, tmux resizes the window to that client and reed renders against the live size instead.
  Keep the two files' comments consistent with each other in the same way the existing `width`/`height` lines already are.

  Do NOT rename either key, do not add a key, and do not change either value.
  A rename would hard-fail every already-materialized `reed.yaml`: `internal/configengine`'s `load()` runs `MissingKeys(template, fileBytes)` and errors with `config file <path>: missing keys: …; run "lyx config reconcile"` whenever the TEMPLATE names a key the on-disk file lacks, so the new name would be absent from every existing file.
  The reverse direction is silent — `internal/reedengine/config.go` unmarshals with plain `yaml.Unmarshal` and no `KnownFields`, so the old key would be dropped without error — which means a migration path would have to handle the missing-key failure, not an unknown-key rejection.
  Note also that `lyx config reconcile` is key-based and never rewrites an existing value, so these comment edits reach an already-materialized `reed.yaml` only through the template, never as a value change.
- **Commit:** `docs(reedengine): restate reed.yaml's width/height as the detached-boot size`

### Card 14: prove the handover lands the exact layout, against a real pty

- **Context:**
  - `internal/reedengine/attach.go`
  - `internal/reedengine/windowsize.go`
  - `internal/reedengine/apply.go`
  - `internal/reedengine/strand.go`
  - `internal/reedengine/lifecycle.go`
  - `internal/reedengine/overlay.go`
  - `internal/reedengine/contract_integration_test.go`
  - `internal/reedengine/mouse_boot_integration_test.go`
  - `internal/reedengine/render/types.go`
- **Edits:** none
- **Creates:**
  - `internal/reedengine/attachgeometry_integration_test.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  Create `internal/reedengine/attachgeometry_integration_test.go` in `package reedengine`, whose first non-empty line is the build constraint `//go:build integration && linux`, followed by a file-header comment stating what the file proves and why it is Linux-only (the pty harness is built on `golang.org/x/sys/unix`'s `/dev/ptmx` ioctls, and psmux's behaviour is unverified anywhere in this repo).
  Every test function's name must begin with `TestAttachGeometry`, because the batch `verify:` selects them with `-run TestAttachGeometry`.

  Build the fixture with the package's existing integration harness — `newIntegrationEngine(t, "off")` (which seeds the config, skips cleanly when the configured multiplexer binary is absent, and registers a `kill-server` cleanup) and `waitUntil` for polling.
  Boot with `Up()`, then add at least two strands via `AddStrand(AddSpec{...})` with `Display.Anchor` set to `render.AnchorBelowParent`, at least one of them carrying `ShrinkWhenWaitingOnChild: true` with a child beneath it, so a collapsed strip and a full pane are both present alongside the header pane.
  Use a strand `Cmd` that holds its pane open without spawning anything expensive, in the shape the existing integration tests already use.

  Write a pty helper in this file — do not add a module dependency for it.
  Open `/dev/ptmx`, clear the lock with `unix.IoctlSetPointerInt(fd, unix.TIOCSPTLCK, 0)`, resolve the slave name from `unix.IoctlGetInt(fd, unix.TIOCGPTN)`, set the master's window size with `unix.IoctlSetWinsize(fd, unix.TIOCSWINSZ, &unix.Winsize{Col: …, Row: …})`, and start the child with the slave as its stdin/stdout/stderr and `SysProcAttr{Setsid: true, Setctty: true}`.
  Close both descriptors in a `t.Cleanup`, and drain the master in a goroutine so the child never blocks on a full pty buffer.

  Cover these cases:
  1. **The exact-layout assertion.** With a pty deliberately unequal to the configured boot size (e.g. 100x30 against the template's 220x50), capture `argv := e.AttachArgv(100, 30)` and the layout string it embeds, exec `cfg.Tmux` with that argv in the pty child, wait for the client to attach, then assert FROM OUTSIDE the pty — via `e.tmux.output("display-message", …)` on the same socket — that `#{window_width}` is 100 and `#{window_height}` is exactly 30 (status line off, so the window is the client's rows, not `rows - 1`), and that `#{window_layout}` equals the argv's own layout string byte for byte.
     This is the assertion that fails before this task: tmux's rescale rewrites the string.
  2. **The row budgets survived.** From the same attached session, assert via `list-panes` that the header pane is exactly `cfg.Header.HeightRows` rows and the collapsed strip exactly `cfg.CollapsedStripRows` rows — the operator-visible half, stated as a number.
     Choose the pty size so those budgets are satisfiable without clamping, so the assertion pins the unclamped values.
  3. **The degraded path still attaches.** `e.AttachArgv(0, 0)` yields the bare argv;
     exec it in a pty and assert the client attaches successfully (the session lists a client, the process does not exit non-zero) with no chained layout.
  4. **The stale-layout race is safe.** Build the argv, then mutate the pane set before exec-ing it (add or remove a strand so the live pane count no longer matches the string's cell count), then attach and assert three things: the attach still succeeded, the chained `select-layout` was refused rather than obeyed (the live `#{window_layout}` is NOT the planned string), and the session's pane set is intact — no pane was destroyed.
     This pins the property the accepted build-vs-apply window rests on, so a future tmux behaviour change surfaces here rather than as a lost pane.

  Detach or kill the client at the end of each case so the next case starts clean, and let `newIntegrationEngine`'s cleanup reap the server.
  Give every assertion a failure message naming the expected and actual values, in the style the existing integration tests use.
  Do not weaken case 1 to a substring or prefix comparison: the byte-for-byte `window_layout` equality is the whole point of the test.
- **Commit:** `test(reedengine): prove the attach handover lands the client-sized layout verbatim`

## Batch Tests

`verify: go test ./internal/reedengine/... && go test -tags integration -run TestAttachGeometry ./internal/reedengine/...`

Two halves, both needed and both narrow:
- The untagged half re-runs `internal/reedengine` and its `render` subpackage.
  Cards 12 and 13 edit `doc.go` and the two embedded templates, and the templates are compiled into the binary via `template_posix.go`/`template_windows.go` and read by `config_test.go`, so a malformed comment or a broken YAML line surfaces there rather than at the first real boot.
- The tagged half runs only the new file's `TestAttachGeometry*` functions.
  It is deliberately `-run`-scoped rather than the whole `integration` tag: `contract_integration_test.go` and `mouse_boot_integration_test.go` each boot and reap real tmux servers and are re-run in full by the task-wide `pipeline.done_gate` (`go test ./... && go test -tags integration ./...`) at the end, so running them on every implementer and fixer round of this batch would spend minutes per round for coverage the done gate already provides.

The tagged half self-skips when the configured multiplexer binary is absent (`newIntegrationEngine`'s `t.Skipf`), and the file is not compiled at all off Linux, so this verify is green on a box without tmux rather than failing — the same posture the package's existing integration suites already take.
The operator acceptance check named in the discussion — running `lyx loom run` in a real terminal and confirming the session takes the whole window — sits outside the suite and outside this verify, by design.
