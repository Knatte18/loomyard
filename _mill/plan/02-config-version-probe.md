# Batch: config-version-probe

```yaml
task: "Facilitate Linux support (Win11-side prep)"
batch: "config-version-probe"
number: 2
cards: 5
verify: GOOS=linux go build ./internal/muxengine/... && go test ./internal/muxengine/... ./internal/muxcli/...
depends-on: [1]
```

## Rename mechanic

_This batch has a non-empty `Moves:` field (card 4). For each `Moves:` pair the
implementer MUST:_

1. _Run `git mv <old> <new>` FIRST, before making any other change to the moved file._
2. _Make ONLY surgical edits — touch only the lines that must change after the move._
3. _Use a full-file `Creates:` entry only for genuinely new files that have no predecessor._
4. _Never write the relocated file from scratch and delete the original — that breaks git
   rename history and inflates review diffs._

## Batch Scope

This batch delivers the "config-swap plumbing" and the "fail-loud capability probe": (a)
GOOS-aware `muxengine` template defaults — Windows keeps `psmux.exe`/`pwsh.exe`, Linux/POSIX
defaults to PATH-resolved `tmux`/`bash`, env overrides (`LYX_MUX_PSMUX`, `LYX_MUX_PWSH`) still
winning; (b) two per-binary pinned min-version constants (psmux and tmux have independent `-V`
schemes, so one constant cannot compare both) with GOOS-selected pure parsers; and (c) a
capability probe run once at server-ensure that checks `<binary> -V` (version floor) and
`<binary> list-commands` (required subcommand set), returning a typed error that propagates
through `Engine.Up()` onto the existing `output.Err` JSON envelope. It edits `lifecycle.go`
(wiring the probe into `ensureServerAndSessionLocked`), which batch 1 also edits — hence
`depends-on: [1]`.

The template embed is split by build tag: the current single `//go:embed template.yaml` in
`template.go` becomes a GOOS-selected pair (`template_windows.go` / `template_posix.go`), with
`template.go` retaining only the untagged `ConfigTemplate()` accessor. The version parsers and
the probe's decidable core are pure and host-tested; the exact `#{pane_*}` format-var contract
is delegated to batch 3's integration test, not checked here (that needs a live pane).

## Cards

### Card 4: GOOS-aware muxengine template defaults

- **Context:**
  - `internal/muxengine/config.go`
- **Edits:**
  - `internal/muxengine/template.go`
- **Creates:**
  - `internal/muxengine/template_posix.yaml`
  - `internal/muxengine/template_windows.go`
  - `internal/muxengine/template_posix.go`
- **Deletes:** none
- **Moves:**
  - `internal/muxengine/template.yaml` -> `internal/muxengine/template_windows.yaml`
- **Requirements:** `git mv internal/muxengine/template.yaml internal/muxengine/template_windows.yaml`
  first; the moved file keeps the Windows defaults verbatim
  (`psmux: ${env:LYX_MUX_PSMUX:-C:\Code\tools\bin\psmux.exe}`,
  `pwsh: ${env:LYX_MUX_PWSH:-C:\Code\tools\powershell7\pwsh.exe}`, and all other keys
  unchanged). Create `template_posix.yaml` identical to it except the two binary defaults become
  PATH-resolved POSIX names: `psmux: ${env:LYX_MUX_PSMUX:-tmux}` and
  `pwsh: ${env:LYX_MUX_PWSH:-bash}` (keep the same env-override tokens and all other keys/values
  identical). In `template.go`, remove the `//go:embed template.yaml` directive and the
  `var configTemplate string` declaration, keeping the package comment and the
  `func ConfigTemplate() string { return configTemplate }` accessor (now untagged, referencing
  the tag-selected var). Create `template_windows.go` (`//go:build windows`, package `muxengine`)
  with `//go:embed template_windows.yaml` above `var configTemplate string`, plus the
  `import _ "embed"`. Create `template_posix.go` (`//go:build !windows`, package `muxengine`)
  with `//go:embed template_posix.yaml` above `var configTemplate string`, plus `import _ "embed"`.
  Do not use a `_windows.go`/`_linux.go` filename suffix here — use explicit build tags so the
  POSIX variant covers all non-Windows GOOS.
- **Commit:** `feat(muxengine): GOOS-aware template defaults (tmux/bash on Linux)`

### Card 5: Per-binary min-version constants + pure parsers

- **Context:**
  - `internal/muxengine/config.go`
  - `internal/muxengine/lock.go`
- **Edits:** none
- **Creates:**
  - `internal/muxengine/version.go`
  - `internal/muxengine/version_test.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** In a build-tag-free `version.go` (package `muxengine`) add two pinned
  min-version constants — `minPsmuxVersion` and `minTmuxVersion` — and pure parsers.
  `parseTmuxVersion(out string) ([3]int, error)`: parse tmux's `-V` output shape `tmux X.Y`
  (and tolerate a `tmux next-X.Y` / trailing letter form by extracting the leading numeric
  `major.minor`), returning `[major, minor, patch]` (patch 0 when absent).
  `parsePsmuxVersion(out string) ([3]int, error)`: parse psmux's `-V` output — the implementer
  MUST run the psmux binary currently on PATH (`psmux -V`) to determine its exact output shape
  and set `minPsmuxVersion` to the version that binary reports (or one patch below), so the pin
  is a drift canary that does not break `mux up` on the current dev box. Set `minTmuxVersion` to
  `3.3`. Add `versionAtLeast(got, min [3]int) bool` doing lexicographic `[3]int` comparison, and
  GOOS-selected accessors `minMultiplexerVersion() [3]int` and
  `parseMultiplexerVersion(out string) ([3]int, error)` that switch on `runtime.GOOS`
  (Windows → psmux; else → tmux). In `version_test.go` drive the pure parsers and comparator
  with fixtures: a valid tmux `-V` line, a `next-`/lettered tmux line, a valid psmux `-V` line,
  malformed input (error), and `versionAtLeast` above/at/below the pin.
- **Commit:** `feat(muxengine): per-binary min-version constants and -V parsers`

### Card 6: Capability probe

- **Context:**
  - `internal/muxengine/overlay.go`
  - `internal/muxengine/config.go`
  - `internal/muxengine/lock.go`
  - `internal/output/output.go`
- **Edits:** none
- **Creates:**
  - `internal/muxengine/probe.go`
  - `internal/muxengine/probe_test.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** In `probe.go` (package `muxengine`) define a typed error
  `type CapabilityError struct { Reason string }` implementing `error` (so callers/tests can
  `errors.As` it), a `requiredSubcommands` slice covering the engine's dependency set
  (`new-session`, `split-window`, `select-layout`, `select-pane`, `send-keys`, `capture-pane`,
  `list-panes`, `list-sessions`, `display-message`, `set-option`, `kill-pane`, `kill-server`),
  and a pure-core `probeCapability(run func(args ...string) (string, error)) error`. It runs
  `run("-V")`, parses via `parseMultiplexerVersion`, and returns a `*CapabilityError` if below
  `minMultiplexerVersion()`; then runs `run("list-commands")`, parses the emitted command names
  (first whitespace-delimited token per line), and returns a `*CapabilityError` naming any
  missing required subcommand. The injected `run` closure is what makes this host-testable.
  Add a method `func (e *Engine) probeCapabilityLocked() error` that binds `run` to
  `exec.Command(e.cfg.Psmux, args...).Output()` (the raw multiplexer binary **without** the
  overlay's `-L <socket>` prefix, since `-V`/`list-commands` are socket-free) and calls
  `probeCapability`. In `probe_test.go` drive `probeCapability` with a fake `run`: healthy
  version + full command set → nil; version below pin → `*CapabilityError`; a missing required
  subcommand → `*CapabilityError`.
- **Commit:** `feat(muxengine): capability probe for multiplexer version and command surface`

### Card 7: Wire probe into server-ensure

- **Context:**
  - `internal/muxengine/lock.go`
  - `internal/muxengine/overlay.go`
- **Edits:**
  - `internal/muxengine/lifecycle.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** In `lifecycle.go`, call `e.probeCapabilityLocked()` (from `probe.go`) near
  the top of `ensureServerAndSessionLocked()` (`lifecycle.go:120`), before the `spawnSession`
  boot loop, returning its error immediately if non-nil so a `*CapabilityError` propagates up
  through `Engine.Up()` to `muxcli`'s existing `output.Err` envelope. The probe runs once per
  ensure/boot; do not thread it into any hot path. Keep the change minimal — a guarded early
  call plus error return; no other lifecycle logic changes.
- **Commit:** `feat(muxengine): fail loud on unknown multiplexer surface at server-ensure`

### Card 8: Update `mux up` help for the capability check

- **Context:**
  - `internal/muxengine/lifecycle.go`
- **Edits:**
  - `internal/muxcli/up.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Per the CLI/Cobra Invariant's help-accuracy obligation, extend the `up`
  command's `Long` in `upCmd()` (`internal/muxcli/up.go`) to state that boot now verifies the
  configured multiplexer meets the pinned minimum version and exposes the required command
  surface, failing loud with a JSON error (`output.Err`) otherwise. Do not change `Short`,
  the `Use` line, the `RunE` body, or the success envelope — the error path is already correct
  (engine errors route through `output.Err`). Text-only edit to keep help matching behavior.
- **Commit:** `docs(muxcli): note capability probe in mux up help`

## Batch Tests

`verify` cross-compiles `muxengine` for Linux (`GOOS=linux go build ./internal/muxengine/...`,
which compiles the new `//go:build !windows` template file and batch 1's `proctree_linux.go`)
then runs `go test ./internal/muxengine/... ./internal/muxcli/...`. Host tests cover the pure
version parsers/comparator (`version_test.go`) and the capability probe core with a fake runner
(`probe_test.go`); `muxcli` normal tests confirm the `up` help edit compiles and the command
tree is intact (integration-tagged psmux smoke tests are excluded by the default no-tag run).
The POSIX template variant is compiled by the cross-compile step and its runtime values are
validated in the deferred real-Linux follow-up. Scope covers the two packages this batch edits.
