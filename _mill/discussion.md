# Discussion: Decide tmux mouse-mode default for lyx mux

```yaml
task: Decide tmux mouse-mode default for lyx mux
slug: mux-mouse-default
status: discussing
parent: cluster-fork-spike
```

## Problem

While demoing `lyx mux attach` against the sandbox Hub on Linux (tmux backend), the
operator found that click-to-switch-pane did not work. `tmux show-options -g mouse`
reported mouse **off**, and nothing in `internal/muxengine` ever sets it. Running
`tmux set-option -g mouse on` live fixed it immediately, confirming the gap is a bare
unset default, not a missing capability. The operator remembered mouse working by
default under the Windows psmux binary, raising the question of whether lyx has a
real cross-platform behavior gap.

**Why now:** the gap surfaced during a live demo and is a papercut for anyone watching
agent panes and expecting to click between them. tmux defaults `mouse` off; lyx boots
its per-hub server without ever setting the option, so the backend default leaks
through and behavior depends on which multiplexer (tmux vs psmux) and which default
that build happens to ship. We are closing the ambiguity by having lyx set the option
explicitly at boot, and giving the operator a documented knob to control it.

## Scope

**In:**

- Add a `mouse` config key to the mux module config (`internal/muxengine/config.go` +
  both embedded templates `template_posix.yaml` and `template_windows.yaml`),
  default **off**.
- At server boot, explicitly run `set-option -g mouse <on|off>` to match the configured
  value — alongside the existing `set-option -g remain-on-exit on` call in
  `ensureServerAndSessionLocked` (lifecycle.go).
- Up-front validation of the `mouse` value (accept `on`/`off` case-insensitive; fail the
  boot loud on any other value), mirroring how `debugLogArgs` validates `debug_log`
  before any psmux round-trip.
- Unit test for the value-parse/validate helper and an integration test asserting
  `show-options -g mouse` reflects the configured value after `Up`.
- Docs: update the mux module doc under `docs/modules/` and document the new config key
  (including that existing hubs adopt it via `lyx config reconcile`, like `debug_log`).

**Out:**

- No new CLI command or flag (e.g. `lyx mux mouse on`). Mouse is a server-global (`-g`)
  tmux concept with no per-pane variant to expose, so a CLI flag would be
  over-specification with no backing mechanism. Config/env only.
- No per-strand or per-session mouse override. The option is server-global by nature.
- No change to the psmux binary itself. Whether psmux defaults mouse on is not
  investigated further (see Decision: psmux-default-moot) — lyx sets the option
  explicitly regardless.
- No investigation of Windows Terminal passthrough or other host-terminal mouse
  mechanisms; out of scope for the mux module.

## Decisions

### psmux-default-moot

- Decision: Do not attempt to confirm whether the Windows psmux binary defaults
  mouse-on. Treat the question as moot and have lyx set the `mouse` option explicitly at
  boot regardless of the backend default.
- Rationale: psmux is an external binary (`LYX_MUX_PSMUX`), not vendored in this repo, so
  its default is not verifiable from source here. More importantly, the same philosophy
  already governs `remain-on-exit`: lyx sets what it needs rather than trusting backend
  defaults, so behavior is deterministic across multiplexers and platforms. Explicitly
  setting mouse each boot makes the operator's memory of "psmux defaulted on" irrelevant.
- Rejected: Blocking the task on a manual Windows psmux experiment to confirm the
  default first — unnecessary once lyx sets the value explicitly either way.

### mouse-config-key-default-off

- Decision: Add a `mouse` config key defaulting to **off**. Operators opt in via config
  or the `LYX_MUX_MOUSE` env override.
- Rationale: Mouse-on in tmux hijacks native terminal drag-select — the operator must
  hold **Shift** to select/copy text. That is exactly why tmux itself defaults mouse
  off, and it is the one real cost of enabling it. Defaulting off preserves native
  copy/select for the broad operator base while making mouse a single, documented toggle
  away for anyone (like the demo operator) who wants click-to-switch. The config key —
  rather than an unconditional set-on or leaving it to manual `tmux set-option` — gives
  the escape hatch without new CLI surface and mirrors the existing `debug_log` pattern.
- Rejected:
  - Config key defaulting **on** — makes click-to-switch the default but silently breaks
    native text selection/copy for everyone who does not know to hold Shift.
  - Unconditional mouse-on at boot with no key — no opt-out at all.
  - Do nothing (status quo, manual `tmux set-option`) — this is precisely the papercut
    that prompted the task.

### mouse-key-shape

- Decision: The key is a string, `mouse: ${env:LYX_MUX_MOUSE:-off}`, mapped to on/off
  (case-insensitive). Env-overridable via `LYX_MUX_MOUSE`.
- Rationale: Mirrors `debug_log` exactly — a string default means an `${env:...}`
  override can never fail yaml parsing, and reconcile adoption of the new key is
  consistent with how `debug_log` was introduced. The env override gives a per-session
  escape hatch without editing config.
- Rejected: A plain yaml bool `mouse: true/false` — simpler to read but no env override,
  and it would be the first bool in the mux Config struct, diverging from the established
  string-with-env-default pattern.

### explicit-set-both-ways

- Decision: Boot always runs `set-option -g mouse <on|off>` with the resolved value —
  including explicitly setting `off` when the config says off. It is never a skipped call.
- Rationale: This is the whole point of psmux-default-moot, and it matters *more* now
  that off is the default: to make the state deterministic regardless of any backend
  default, boot must pin the option to exactly the configured value in both directions.
  Skipping the call when off would re-open the cross-platform-default ambiguity the task
  is closing.
- Rejected: Skip the `set-option` call when `mouse: off` (rely on backend default) —
  cheaper by one psmux call but re-introduces the ambiguity.

### validate-up-front

- Decision: Accept only `on`/`off` (case-insensitive). Any other value fails the boot
  loud, validated up front before any psmux round-trip — the same placement as
  `debugLogArgs` validation in `ensureServerAndSessionLocked`.
- Rationale: A misconfigured `mouse` value is a pure config error, unrelated to
  server/session state, so it must surface immediately and loudly rather than partway
  through a spawn or as a cryptic tmux error. Restricting to on/off (no true/false/1/0
  aliases) keeps the accepted set minimal and matches tmux's own `set-option mouse`
  vocabulary.
- Rejected: Also accepting `true`/`false`/`1`/`0` as aliases — extra surface with no
  demand; on/off is the tmux-native spelling.

## Technical context

Relevant module: `internal/muxengine`.

- **Boot site.** `Engine.ensureServerAndSessionLocked` (lifecycle.go, ~line 187) is the
  single boot path. After the boot loop brings the session up, it runs
  `e.psmux.run("set-option", "-g", "remain-on-exit", "on")` (~line 341). The new
  `set-option -g mouse <on|off>` call belongs immediately alongside it. Note this
  function returns early (`false, nil, nil`, ~line 223) when the worktree's session is
  already up with live panes, so the set-option calls run on a fresh boot/new-session,
  not on every `Up` — same semantics as `remain-on-exit`. Because `mouse` is a
  server-global (`-g`) option on the shared per-hub server, setting it on any boot that
  runs `new-session` is sufficient.
- **Up-front validation precedent.** The same function validates `debug_log` first via
  `debugArgs, err := debugLogArgs(e.cfg.DebugLog)` (~line 191) and returns the error
  before the capability probe or any psmux round-trip. The `mouse` value should be parsed
  and validated by an analogous helper (e.g. `mouseOption(e.cfg.Mouse) (string, error)`
  returning `"on"`/`"off"` or an error) at the same early point, so an invalid value
  fails before anything touches psmux.
- **Config plumbing.** `Config` (config.go) mirrors the mux.yaml keys; `LoadConfig` runs
  `configengine.Load` against `ConfigTemplate()` then `yaml.Unmarshal`s into the struct.
  Add a `Mouse string \`yaml:"mouse"\`` field with a doc comment matching the `DebugLog`
  comment's shape (including the "existing hubs need `lyx config reconcile` to adopt this
  key" note). The template is embedded per-GOOS: `template_posix.go` and
  `template_windows.go` `//go:embed` `template_posix.yaml` / `template_windows.yaml` into
  the package-level `configTemplate` var, surfaced by `ConfigTemplate()` (template.go).
  Add the `mouse: ${env:LYX_MUX_MOUSE:-off}` line to **both** yaml files with a matching
  inline comment (the two files already carry parallel `debug_log:` lines with identical
  comments — follow that exactly).
- **Integration-test precedent.** `contract_integration_test.go` documents and exercises
  the real-tmux boot contract, including that "production always sets remain-on-exit at
  boot (lifecycle.go)" and asserts it via `set-option`/`show-options`. A mouse integration
  test mirrors this: boot with default config, assert `show-options -g mouse` reports
  `off`; boot (or a config variant) with `mouse: on`, assert it reports `on`.

## Constraints

From `CONSTRAINTS.md` (hub root) and CLAUDE.md, the ones that bear on this task:

- **Hub Geometry Invariant** — `internal/hubgeometry` owns all cwd/geometry and
  `_lyx`/config paths. This task does not add or change any path resolution; the config
  key flows through the existing `configengine.Load` seam, so the invariant is untouched.
- **CLI / Cobra Invariant** — module `Command()`/`RunCLI` seam, `Short` on every command,
  help-tree tests. This task adds **no** CLI command or flag (see Scope: Out), so it does
  not interact with this invariant.
- **Documentation Lifecycle** (CLAUDE.md) — a change to observable behavior / config
  surface must update docs in the **same commit**: the mux module doc under
  `docs/modules/` and the new config-key documentation. This is an observable-behavior
  change (a new config key; boot now pins mouse), so the doc update is mandatory, not a
  follow-up. No new cross-cutting invariant is introduced, so `CONSTRAINTS.md` itself does
  not change. `docs/roadmap.md` is **not** touched — this is delivered work, not a planned
  milestone.

## Testing

Module under test: `internal/muxengine`.

- **Value helper (unit, TDD candidate).** The parse/validate helper (e.g. `mouseOption`)
  is the natural TDD candidate — pure input→output with no psmux dependency. Table-driven
  test in the style of the `debug_log` helper test: cover `on`, `off`, case variations
  (`ON`, `Off`), the default resolution, and invalid values (`yes`, `1`, empty-after-env,
  garbage) asserting a loud error. This is the primary correctness gate for the
  fail-loud-on-invalid decision.
- **Boot sets the option (integration, real tmux).** Mirror `contract_integration_test.go`:
  bring a server up via the engine, then assert `show-options -g mouse` matches the
  configured value — `off` under the default config, `on` when configured on. This is the
  end-to-end proof that boot pins the option in both directions (explicit-set-both-ways).
  Gate/skip consistently with the existing integration tests if they are environment-gated
  on tmux availability.
- **Config load (unit).** If there is an existing config round-trip/template test, extend
  it so the new `mouse` key is present in the resolved template and unmarshals into the
  `Config.Mouse` field with the expected default. Do not add a redundant new test if the
  existing template test already covers all keys generically.
- Exact assertion shapes are left to mill-plan; the scenarios above are the required
  coverage.

## Q&A log

- **Q:** How do we resolve the brief's "does psmux actually default mouse-on?" question? **A:** Treat it as moot — lyx sets the option explicitly at boot regardless of backend default (same philosophy as `remain-on-exit`).
- **Q:** Mechanism and default for lyx mux? **A:** Config key `mouse`, default **off**. (Operator initially answered default-on, then revised to default-off to preserve native terminal text-selection/copy — the reason tmux itself defaults mouse off — with mouse one config/env toggle away.)
- **Q:** Config key shape? **A:** String `mouse: ${env:LYX_MUX_MOUSE:-off}`, case-insensitive on/off, env-overridable — mirrors `debug_log` (env override never breaks yaml parse; consistent reconcile adoption).
- **Q:** Accepted values and invalid handling? **A:** `on`/`off` only (case-insensitive); invalid fails the boot loud, validated up front like `debugLogArgs`. No true/false/1/0 aliases.
- **Q:** When `mouse: off`, does boot still call set-option? **A:** Yes — explicitly `set-option -g mouse off`. Deterministic regardless of backend default; matters more now that off is the default.
- **Q:** Scope boundaries — anything pulled back in? **A:** No. No CLI flag (mouse is a `-g`-only tmux concept with no per-pane variant to expose, so a flag would be over-specification), no per-strand override, no psmux change; docs get updated.
