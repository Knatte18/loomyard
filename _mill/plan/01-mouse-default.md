# Batch: mouse-default

```yaml
task: "Decide tmux mouse-mode default for lyx mux"
batch: "mouse-default"
number: 1
cards: 6
verify: go test ./internal/muxengine/
depends-on: []
```

## Batch Scope

Adds a `mouse` config key to the mux module (default `off`) and pins the tmux/psmux
`mouse` server-global option to that value on every fresh boot, alongside the existing
`remain-on-exit` boot option. Everything lives in one package (`internal/muxengine`) plus
its package godoc, so it is one Sonnet-sized batch: a value-validator helper + unit test,
the config field and template lines, the boot wiring, a config-load test extension, a
real-`Up()` integration test, and the godoc boot-option update. There is no external
interface a later batch consumes — this batch is self-contained. Batch-local decisions
follow the overview's Shared Decisions (mouse-value-contract, explicit-set-both-ways,
helper-lives-in-mouse.go, docs-target-reconciliation, integration-test-gating).

## Cards

### Card 1: Add `mouseOption` value validator and its unit test

- **Context:**
  - `internal/muxengine/serverlog.go`
  - `internal/muxengine/config.go`
- **Edits:** none
- **Creates:**
  - `internal/muxengine/mouse.go`
  - `internal/muxengine/mouse_test.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** In `mouse.go` (package `muxengine`) define `mouseOption(raw string) (string, error)`: it trims surrounding whitespace and lowercases `raw`, returns `"on"` for `on`, `"off"` for `off`, and for every other value — the empty string included — returns an error such as `fmt.Errorf("invalid mouse value %q: want \"on\" or \"off\"", raw)`. It never silently defaults. Give it a godoc comment mirroring the shape of `debugLogArgs` in `serverlog.go` and stating explicitly that an empty string is an error, not a default (see Shared Decision mouse-value-contract). In `mouse_test.go` (package `muxengine`, so it can call the unexported helper) write a table-driven test covering: `on`/`off` returning the canonical lowercase form; case and whitespace variants (`ON`, `Off`, ` on `) normalizing correctly; and invalid inputs (`yes`, `1`, arbitrary garbage, and the empty string `""`) each returning a non-nil error. The empty-string case is explicit: assert `mouseOption("")` errors rather than returning `off`.
- **Commit:** `feat(muxengine): add mouseOption on/off validator helper`

### Card 2: Add `Mouse` config field and template lines

- **Context:**
  - `internal/muxengine/serverlog.go`
- **Edits:**
  - `internal/muxengine/config.go`
  - `internal/muxengine/template_posix.yaml`
  - `internal/muxengine/template_windows.yaml`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** In `config.go` add a `Mouse string \`yaml:"mouse"\`` field to the `Config` struct, placed adjacent to `DebugLog`, with a doc comment that mirrors the `DebugLog` field's comment shape: a string (not a bool) so an `${env:...}` override never breaks yaml parse; validated and mapped to `on`/`off` by `mouseOption` (mouse.go), not by this struct; takes effect only on the boot that spawns the shared per-hub server; and a hub whose `mux.yaml` predates this field needs `lyx config reconcile` to adopt it. In BOTH `template_posix.yaml` and `template_windows.yaml`, add a line `mouse: ${env:LYX_MUX_MOUSE:-off}` with an inline comment documenting it as the tmux/psmux mouse-mode default (on/off; off preserves native terminal text selection/copy; set `on` for click-to-switch-pane; takes effect on a fresh mux server boot only — a live toggle needs a server restart). Follow the existing parallel `debug_log:` lines in the two files exactly (same key, matching comment) — do not let the two templates drift.
- **Commit:** `feat(muxengine): add mouse config key defaulting off`

### Card 3: Pin the mouse option at server boot

- **Context:**
  - `internal/muxengine/serverlog.go`
- **Edits:**
  - `internal/muxengine/lifecycle.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** In `Engine.ensureServerAndSessionLocked` (lifecycle.go), resolve and validate the mouse value up front, at the same early point where `debugLogArgs(e.cfg.DebugLog)` is validated (before the capability probe and any psmux round-trip): call `mouseOption(e.cfg.Mouse)` and return its error immediately if non-nil, so an invalid `mouse` value fails the boot loud rather than partway through. Then, immediately after the existing `e.psmux.run("set-option", "-g", "remain-on-exit", "on")` call near the end of the same function, add `e.psmux.run("set-option", "-g", "mouse", <resolved>)` where `<resolved>` is the `"on"`/`"off"` string returned by `mouseOption`; wrap its error the same way (e.g. `fmt.Errorf("set mouse: %w", err)`). The call runs unconditionally on this fresh-boot path for both `on` and `off` (Shared Decision explicit-set-both-ways-at-boot). Do not move or alter the existing early-return that skips this path when the session is already up with live panes — the no-live-toggle contract depends on it.
- **Commit:** `feat(muxengine): set tmux mouse option at server boot`

### Card 4: Assert the mouse default in the config-load test

- **Context:**
  - `internal/muxengine/config.go`
- **Edits:**
  - `internal/muxengine/config_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Extend the existing config-load/default-resolution test in `config_test.go` (the one that seeds `ConfigTemplate()` and asserts resolved defaults) so it also asserts the `mouse` key resolves to its default `"off"` on the `Config.Mouse` field when no `LYX_MUX_MOUSE` env override is set. Follow the existing assertion style for the other defaulted fields; do not add a redundant separate test if the existing one already asserts each defaulted field generically — in that case just add the `Mouse` field to the expected set.
- **Commit:** `test(muxengine): assert mouse config default is off`

### Card 5: Integration test — boot pins mouse, no live toggle

- **Context:**
  - `internal/muxengine/contract_integration_test.go`
  - `internal/muxengine/lifecycle.go`
  - `internal/muxengine/lock.go`
  - `internal/muxengine/config.go`
  - `internal/hubgeometry/hubgeometry.go`
- **Edits:** none
- **Creates:**
  - `internal/muxengine/mouse_boot_integration_test.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** Create a `//go:build integration`-tagged test file (package `muxengine`) that self-skips when the configured multiplexer binary is absent, mirroring `contract_integration_test.go`'s skip/scratch-socket/`seedMuxConfig` harness. Build an `Engine` via `New(cfg, layout)` (lock.go) against a `t.TempDir()`-rooted `hubgeometry.Layout` (Cwd/WorktreeRoot/Hub set under the temp dir) and a `Config` whose `Mouse` field is set for the case under test (construct the `Config` directly or seed the template and override `cfg.Mouse`). Always tear the scratch server down (`kill-server` on `e.Socket()`) via `t.Cleanup`, exactly as the contract test does, so no scratch server leaks. Two cases: (1) **fresh boot pins the option** — with `Mouse: "off"` call `e.Up()` then assert `show-options -g mouse` on `e.Socket()` reports `off`; with a fresh temp hub and `Mouse: "on"` assert it reports `on`. (2) **no live toggle without restart** — boot once with `Mouse: "off"` and confirm `off`; then, without tearing the session down, build a second `Engine` on the SAME layout with `Mouse: "on"` and call `Up()` again; assert `show-options -g mouse` is STILL `off`, proving the already-up session hits the early return and does not re-apply `set-option`. Read `mouse` back using the same raw-psmux command style the contract test uses for its option assertions.
- **Commit:** `test(muxengine): integration test for mouse boot option and no-live-toggle`

### Card 6: Document the mouse boot option in package godoc

- **Context:**
  - `internal/muxengine/lifecycle.go`
- **Edits:**
  - `internal/muxengine/doc.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Update `doc.go`'s package godoc to document the mouse boot option alongside `remain-on-exit`. In the "Subcommand set" sentence that lists `set-option -g remain-on-exit`, include `set-option -g mouse` as another server-global the engine sets at boot. Add a short load-bearing note (in the same style as the existing "Dead-pane adoption via remain-on-exit" bullet) stating that the engine pins `-g mouse` to the configured `mouse` value (default `off`) on a fresh boot, that this is applied only on the boot that spawns the session — like `remain-on-exit` and `debug_log` — so toggling `mouse` in config or `LYX_MUX_MOUSE` on an already-running hub has no effect until the mux server restarts, and that `off` preserves native terminal text selection/copy while `on` enables click-to-switch-pane.
- **Commit:** `docs(muxengine): document mouse boot option in package godoc`

## Batch Tests

The batch `verify: go test ./internal/muxengine/` runs the package's default (untagged)
unit tests after every implementer and fixer round. That covers `mouse_test.go` (Card 1's
table-driven `mouseOption` test, the primary correctness gate for the
fail-loud-on-invalid/empty contract) and `config_test.go` (Card 4's default-resolution
assertion), plus the rest of the package's existing unit tests as a compile/regression
guard for the `config.go`, `lifecycle.go`, and `doc.go` edits. Scope is the single
package the batch touches — not the whole repo — so the gate stays fast.

The integration test created in Card 5 (`mouse_boot_integration_test.go`) is
`//go:build integration`-tagged and therefore excluded from that untagged `verify:` run,
exactly like the existing `contract_integration_test.go`. It is validated separately via
`go test -tags integration ./internal/muxengine/` in an environment with a real tmux/psmux
binary, and self-skips cleanly where none is present (Shared Decision
integration-test-gating). It is the end-to-end proof that boot pins `-g mouse` in both
directions and that the early-return/no-live-toggle contract holds.
