# Discussion: Facilitate Linux support (Win11-side prep)

```yaml
task: Facilitate Linux support (Win11-side prep)
slug: facilitate-linux
status: discussing
parent: main
```

## Problem

lyx must run on **both** Windows and Linux, but today it is Windows-fixated in
several places. The foundation is already platform-seamed (`internal/proc`,
`internal/fslink`, `internal/vscode` via `_windows.go`/`_linux.go`), and the
`_other.go → _linux.go` rename is already done (commit `2e11522`). This task is
**audit-first**: enumerate every Windows-coupled surface, then close the ones
that can be **prepared and verified from a Win11 dev box** — the code-level Linux
paths, the shell abstraction, the psmux→tmux config-swap plumbing, and the
contract-test/capability-probe scaffolding.

**Why now:** Linux support is a committed direction, and the foundation is
already seamed, so the code-level port work can proceed without a Linux machine.
**Final on-real-Linux validation** (running the smoke suite green, real tmux
behavioral validation, real process-tree/PID execution) is a deliberate,
explicit follow-up on an actual Linux box and is **out of this task's scope** —
this task produces code that cross-compiles and is unit-tested, not a
Linux-validated binary.

## Scope

**In:**

- **Verify (don't rewrite) the already-seamed packages** — `internal/proc`
  (`Setsid` detach), `internal/fslink` (symlink Linux impl), `internal/vscode`
  config, `configengine/edit.go` (`vi` fallback), `tools/deploy` (`.exe` via
  GOOS). Confirm the Linux build-tag paths are complete and logically correct.
- **Seam + write a real `/proc`-based Linux impl of the process-tree probes** in
  `internal/muxengine/lifecycle.go` (`descendantClosurePIDs`,
  `serverProcessesOnSocket`), which today run PowerShell `Get-CimInstance
  Win32_Process` and **silently degrade to `nil` on Linux**.
- **Introduce a provider-invariant `internal/shell` abstraction** (pwsh + posix
  implementations) and route `claudeengine`'s command construction and
  `shuttleengine/posix.go` path handling through it.
- **Implement `internal/vscode/launch_linux.go`** (currently returns
  `ErrUnsupported`) so `lyx ide spawn` works on Linux.
- **Add a Linux `.sh` launcher branch** to `internal/warpengine/launchers.go`
  and make the launcher-filename extension GOOS-aware in `internal/hubgeometry`.
- **Config-swap plumbing:** GOOS-aware template defaults (`tmux`/`bash` on Linux)
  + a pinned multiplexer min-version constant.
- **A `//go:build integration` multiplexer contract test** asserting the exact
  `list-panes -F` format, the subcommand set, and every load-bearing behavioral
  assumption, run against the configured binary (psmux on Windows, tmux on
  Linux).
- **A startup capability probe** that fails loud on an unknown/old multiplexer
  surface.
- **Document the psmux/tmux contract surface** in the `internal/muxengine`
  godoc (`doc.go`).
- **Record the deferred real-Linux follow-up** as a planned roadmap milestone +
  an explicit checklist (see the deferred list under "Out").

**Out:**

- **Running the sandbox smoke suite green on Linux** — deferred to the follow-up.
- **Real tmux behavioral validation** — every psmux edge-case assumption
  (silent split failure, dead-pane adoption, `-l` leading-dash bug, empty-layout
  destruction, async kill-server) must be re-verified against real tmux on a
  Linux box; the contract test is the vehicle, but its *execution* against tmux
  is the follow-up.
- **Real `/proc` execution validation** — the `/proc` reaping logic is written
  and unit-tested here against fixtures, but running it against a live Linux
  process tree is the follow-up.
- **Non-Claude engines** — the shell abstraction is provider-invariant by
  design, but no second engine is built here.
- **muxpoc / muxpoccli** — POC code, already seamed (`spawnattach_{windows,linux}.go`);
  not part of the production port.
- **PATH provisioning / install tooling for Linux** (getting `tmux`, `bash`,
  `code`, `claude` onto a Linux box) — environmental, not code.

## Decisions

### proc-tree-reaping — seam + real `/proc` impl

- **Decision:** Extract the two WMI probes in `lifecycle.go`
  (`descendantClosurePIDs`, `serverProcessesOnSocket`) behind a small
  platform-seamed interface: a `_windows.go` file keeping the existing
  `Get-CimInstance Win32_Process` pwsh probe, and a `_linux.go` file that walks
  `/proc/<pid>/stat` to build the PID→PPID map and computes the descendant
  closure. The descendant-closure computation (pure, given a PID→PPID map) is
  unit-tested against hand-authored `/proc/<pid>/stat` fixtures. Real-Linux
  execution validation is deferred.
- **Rationale:** The current code silently returns `nil` on any platform where
  `Win32_Process` does not exist, which quietly drops the "no stray process /
  worktree-busy" guarantees that `Down`/boot depend on — a landmine, not a
  seam. `/proc` parenthood is deterministic and unit-testable from fixtures
  without a Linux box, so the logic can ship now honestly.
- **`descendantClosurePIDs` semantics:** Use the `/proc/<pid>/stat` **PPID
  chain** (field 4) to compute the descendant closure — the direct analog of the
  WMI parent-walk, preserving current Windows semantics. Rejected: grouping by
  session/process-group id (`Setsid` gives each detached child a new session) —
  more robust to re-parenting but diverges from the established WMI subtree
  semantics.
- **`serverProcessesOnSocket` semantics (Linux):** The Windows rationale — every
  psmux CLI probe exits 0/1 identically, so the OS process table is the *only*
  trustworthy liveness signal — does **not** hold for tmux, whose
  `has-session`/`list-sessions` honestly return non-zero when no server runs, and
  the hardcoded `"psmux.exe"`/`__warm__` names have no tmux analog. So on Linux:
  (a) use tmux's **real CLI absence signal** (non-zero `has-session` exit) for
  liveness, and (b) scan `/proc/*/cmdline` for processes whose argv contains the
  configured tmux binary **and** the `-L <socket>` token *only* to satisfy the
  "no stray process / worktree-busy" guarantee — replacing the hardcoded
  `psmux.exe`/`__warm__` match. The `/proc/cmdline` scan is the stray-process
  backstop; the CLI signal is authoritative for liveness where tmux provides it.
  This split is the platform seam's Linux side (`_linux.go`); Windows keeps the
  WMI process-table hunt unchanged.
- **Rejected:** (a) leaving the silent-`nil` degrade as a pure audit note —
  leaves the landmine; (b) a typed "not-implemented on Linux" stub — makes the
  gap loud but ships no working logic when the logic is cheap and testable;
  (c) a literal `/proc/*/cmdline`-only scan that ignores tmux's honest CLI
  absence signal — uniform with Windows but redundant when tmux reports absence
  directly.

### shell-abstraction — new `internal/shell` leaf

- **Decision:** Introduce `internal/shell`, a **provider-invariant** package with
  a `pwsh` implementation and a `posix` implementation, selected by
  `runtime.GOOS`. It owns: argument quoting (`pwshSingleQuote` becomes the pwsh
  impl; a posix single-quote-escaping variant is the other), command-chain
  building (the pwsh call-operator `& <bin>` + `Get-Content -Raw <file>` idiom
  vs. the posix `<bin> "$(cat <file>)"` / direct-exec form), and any
  path-shape conversion. `internal/shuttleengine/claudeengine/command.go` builds
  its launch/resume commands through `internal/shell`; the git-bash path
  conversion in `internal/shuttleengine/posix.go` folds into the posix shell
  (a no-op where paths are already POSIX).
- **Rationale:** The brief explicitly calls for "the pwsh/shell abstraction," and
  a real abstraction (not inline `GOOS` branches) removes the duplication and is
  the seam a future non-Claude engine would also use.
- **Provider-seam compliance:** `internal/shell` is provider-invariant and must
  contain **no** Claude specifics; the Claude command *content* (flags,
  `--session-id`, `--settings`, prompt-file handling) stays inside
  `claudeengine`, honoring the **Shuttle Provider-Seam Invariant**. `shell`
  provides only shell *mechanics* (quoting, chaining, file-read idiom).
- **Shell-family selection assumption:** the shell family (pwsh vs posix) is
  chosen by `runtime.GOOS`, but the pane's actual shell is the configured
  `cfg.Pwsh` binary (env-overridable via `LYX_MUX_PWSH`). This task assumes
  **GOOS determines the shell family and the configured pane-shell binary matches
  that family** — a deliberate cross-family override (e.g. pointing
  `LYX_MUX_PWSH` at pwsh on Linux) would make `internal/shell` emit posix syntax
  into a pwsh pane and is **unsupported / out of scope**. Record this as a
  documented constraint; keying the selector off the configured binary's family
  (basename detection) is the more-robust alternative the plan may adopt if it is
  cheap, but the GOOS default is the baseline.
- **Rejected:** (a) bare `runtime.GOOS` branches inline in `claudeengine` —
  duplicated builder paths, weaker seam; (b) staying pwsh-only — contradicts the
  brief.

### vscode-linux-launch — implement, don't stub

- **Decision:** Implement `internal/vscode/launch_linux.go` as
  `exec.Command("code", worktreeDir)` (dropping the Windows `cmd /c` PATH shim),
  mirroring `launch_windows.go`'s behavior.
- **Rationale:** The brief lists vscode under "verify, don't rewrite," but the
  verify reveals a non-functional `ErrUnsupported` stub that leaves `lyx ide
  spawn` dead on Linux. The fix is a one-liner and directly serves "facilitate
  Linux." Execution validation is deferred like everything else.
- **Rejected:** leaving `ErrUnsupported` — a knowingly-broken command
  contradicts the task's purpose.

### sh-launchers — full branch + GOOS-aware filenames

- **Decision:** Add a non-Windows branch to `warpengine/launchers.go`'s
  `writeLaunchers` that generates `ide.sh`, `warp-checkout.sh`, and `ide-menu.sh`
  (shebang `#!/usr/bin/env bash`, body `cd "$(dirname "$0")/<climb>" && lyx …`,
  `chmod 0755`, LF line endings, forward-slash paths). Make the *menu*
  launcher-filename extension GOOS-aware in `internal/hubgeometry` (the
  `MenuLauncherPath()` at `hubgeometry.go:309` hardcodes `ide-menu.cmd`); the
  `ide`/`warp-checkout` extensions stay in `warpengine` (see the geometry note
  below).
- **Testability seam:** `writeLaunchers` branches on `runtime.GOOS` inline, so
  its Linux branch cannot be exercised by TDD on a Windows dev/CI box as-is.
  Extract a **pure, GOOS-parameterized content builder** (takes the target OS +
  climb path, returns the launcher-file bytes) that both branches call, so both
  the `.cmd` and `.sh` outputs are fixture-testable regardless of host OS. The
  thin `runtime.GOOS`-selecting + file-writing wrapper stays untested-by-unit.
- **Rationale:** Launchers are wanted on Linux; the current early-return no-op
  leaves the worktree without them.
- **Rejected:** deferring launchers — audit-only leaves a functional gap the
  brief explicitly names.
- **Geometry-invariant note (scoped):** only the **menu launcher** filename
  lives in `internal/hubgeometry` (`MenuLauncherPath()` at `hubgeometry.go:309`,
  hardcoded `ide-menu.cmd`), so *its* GOOS-aware extension logic moves into
  `hubgeometry` per the **Hub Geometry Invariant**. The other two launcher
  filenames (`ide.cmd`, `warp-checkout.cmd`) are built inside
  `warpengine/launchers.go` and are **not** geometry tokens — their `.cmd`/`.sh`
  extension logic stays in `warpengine`. Do not over-migrate them into
  `hubgeometry`.

### config-defaults-and-version-pin

- **Decision:** Make the `muxengine` template defaults GOOS-aware: Windows keeps
  `…/psmux.exe` / `…/pwsh.exe`; Linux defaults to `tmux` / `bash` (PATH-resolved).
  Env overrides (`LYX_MUX_PSMUX`, `LYX_MUX_PWSH`, …) still win. (Only
  `internal/muxengine/template.yaml` ships the `psmux`/`pwsh` keys;
  `internal/shuttleengine/template.yaml` ships `claude` and has no pwsh/psmux
  binary of its own — the pwsh binary is muxengine's `cfg.Pwsh`.)
- **Version pin — per-binary, GOOS-selected:** psmux and tmux are distinct
  binaries with independent `-V` output and version numbering, so a single min
  constant cannot compare against both. Ship **two pinned min-version constants**
  — one for psmux, one for tmux — each with its own `-V` parse (psmux's `-V`
  shape vs. tmux's `tmux X.Y`), selected by `runtime.GOOS` at probe time. The pin
  applies to the **multiplexer only** — pwsh/bash are not version-pinned.
- **Rationale:** The binary swap is genuinely config-driven (`Config.Psmux`,
  `Config.Pwsh` are both `yaml`-backed string paths), so a Linux default + a
  per-binary version floor is the whole "config-swap plumbing" deliverable.
  Pinning catches version-drift that would break the tuned edge-case assumptions.
- **Rejected:** single Windows-defaulted template relying on env overrides (worse
  out-of-box Linux UX); one generic min constant across both binaries
  (meaningless across two `-V` schemes); dropping the numeric pin entirely
  (loses the drift canary); pinning only psmux now (leaves tmux drift uncaught
  when the swap lands).

### mux-contract-test — Go integration test

- **Decision:** A `//go:build integration` Go test at
  `internal/muxengine/contract_integration_test.go` that spawns a **real** server
  via the *configured* binary (so the same test runs against psmux on Windows and
  tmux on Linux), and asserts: the exact `list-panes -F "#{pane_id} #{pane_dead}
  #{pane_top} #{pane_width} #{pane_height} #{pane_pid}"` format and its parse; the
  full subcommand set used by the engine (`new-session`, `split-window`,
  `select-layout`, `select-pane`, `send-keys -l`, `capture-pane`, `list-panes`,
  `list-sessions`, `display-message`, `set-option -g remain-on-exit`,
  `kill-pane`, `kill-server`); and each behavioral assumption (remain-on-exit
  dead-pane visibility, `pane_dead=1` reporting, `-l` leading-dash handling,
  select-layout against the live pane set). Skips with a clear message when the
  binary is absent.
- **Rationale:** The task wants *exact-format* assertions — a precise
  programmatic contract, not an agent-driven suite. This is the canary for both
  version-drift and the tmux swap.
- **Complements, doesn't replace:** the existing agent-driven `SANDBOX-MUX-SUITE`
  (needs live psmux) stays. Rejected: extending only the sandbox suite (looser,
  no `go test` gate); doing both (redundant for this task).

### capability-probe — fail loud at server-ensure

- **Decision:** A probe run at server-ensure / `mux up` time (once per server
  boot) that queries `<binary> -V` (version), parses it with the
  **GOOS-selected per-binary `-V` parser** (psmux vs tmux shape) and compares
  against that binary's pinned min (see config-defaults decision), and verifies
  the required subcommands and `#{pane_*}` format vars are supported; on a
  missing surface or a version below the pin it returns a **typed error** through
  the `internal/output` envelope (`output.Err`), failing loud rather than
  half-working.
- **Rationale:** The brief wants "fail loud on an unknown multiplexer surface."
  Boot-time is the earliest honest failure point.
- **Rejected:** lazy first-call probe (later, murkier failure); config/compile-time
  assertion only (can't see the actual installed binary).
- **CLI note:** if this changes observable `mux up` behavior, re-read and update
  the affected `Short`/`Long` per the **CLI/Cobra Invariant**'s help-accuracy
  obligation. Errors stay on the JSON envelope.

### contract-doc-location — godoc `doc.go`

- **Decision:** Document the psmux/tmux **contract surface** in
  `internal/muxengine/doc.go` (godoc): the ~6 `#{pane_*}` format vars the engine
  parses, the subcommand set it depends on, and each load-bearing behavioral
  assumption (silent split failure, dead-pane adoption, `-l` leading-dash bug,
  empty-layout session destruction, async kill-server / probe-always-exits-0).
- **Rationale:** The standalone mux module doc was already deleted per the
  **Documentation Lifecycle**; godoc is the canonical module-doc home. A separate
  `docs/reference/mux-contract.md` would create a second source that drifts — the
  existing `docs/reference/psmux_scripting.md` is a broad tmux-compatible command
  reference, not a scoped contract, and stays as-is.
- **Rejected:** a durable standalone doc; extending `psmux_scripting.md`.

### deferred-followup-recording

- **Decision:** Add a "Phase 3: real-Linux validation" **planned milestone** to
  `docs/roadmap.md` (a new planned goal, which is the roadmap's stated purpose),
  and enumerate the exact deferred checklist in this discussion (see "Out") so
  the follow-up task inherits it verbatim.
- **Rationale:** The roadmap is the durable home for planned milestones per
  CLAUDE.md; the discussion carries the concrete checklist.
- **Rejected:** a GitHub issue (less discoverable in-repo); both (redundant).

## Technical context

Key files and findings from the audit (for mill-plan):

- **`internal/muxengine/config.go:21-30`** — `Config` with `Psmux string`
  (`yaml:"psmux"`) and `Pwsh string` (`yaml:"pwsh"`); defaults in
  `internal/muxengine/template.yaml:1-2` (`${env:LYX_MUX_PSMUX:-C:\…\psmux.exe}`,
  `${env:LYX_MUX_PWSH:-C:\…\pwsh.exe}`).
- **`internal/muxengine/lifecycle.go`** — pane shell spawned by handing psmux the
  pwsh binary as the pane command (`:162-169`, uses `e.cfg.Pwsh`; **binary path
  only**, so the Linux default from config-swap covers it). The **hard** coupling
  is `descendantClosurePIDs` (`:559-566`) and `serverProcessesOnSocket`
  (`:662-667`), both `exec.Command(e.cfg.Pwsh, "-NoProfile", "-NonInteractive",
  "-Command", <Win32_Process script>)`, degrading to `nil`/bare-roots on failure
  (comments `:546-548`, `:660-661`). `serverProcessesOnSocket` is what hunts the
  psmux `__warm__` helper on the `-L` socket.
- **`internal/muxengine/overlay.go:45-68,110`** — the actual `exec.Command(psmuxPath,
  fullArgs…)` with `-L <socket>`, and the single `list-panes -F "#{pane_id}
  #{pane_dead} #{pane_top} #{pane_width} #{pane_height} #{pane_pid}"` format
  string. Parsed by `parse.go:37-87` (`parsePaneList` → `LivePane`, dead keyed on
  `pane_dead == "1"` at `:58`). This layer is **pure CLI, ports cleanly.**
- **`internal/muxengine/apply.go` / `reconcile.go` / `spawn.go`** — the tuned
  psmux edge-case behavior, all documented in comments: silent split failure
  (`spawn.go:32-38`), dead-pane adoption (`spawn.go:26-29`), `-l` leading-dash
  drop handled by `sendKeysLiteralArg` (`spawn.go:75-88`), empty-layout
  destruction guarded by `anyPlacedStrand` (`apply.go:85-89`), async kill-server
  / probe-always-exits-0 (`lifecycle.go:182-193`, `:722-729`). These are the
  behavioral assumptions the contract test and godoc must capture.
- **`internal/shuttleengine/claudeengine/command.go`** — `pwshSingleQuote`
  (`:65-67`), `claudeBinary` (`:72-77`), `buildLaunchCmd` (`:104-119`, uses `&
  <bin> (Get-Content -Raw <path>) --session-id … --settings …`), `buildResumeCmd`
  (`:127-132`). `maxLaunchPromptBytes = 30000` (`:29`) is justified by the
  Windows 32,767-char `CreateProcess` cap — a Linux port could relax it (leave
  as-is unless trivial). `startup.go` is pure string classification, **no OS
  coupling** (one incidental pwsh-profile prompt note at `:47-50`).
- **`internal/shuttleengine/posix.go`** — `PosixPath` (`:22-37`) converts
  `C:\a b\c` → `/c/a b/c` for git-bash hook commands; **Windows-input-only**
  (rejects non-drive-rooted paths). Consumer: `claudeengine.go:97` (`Prepare`).
  This folds into the posix shell impl.
- **`internal/clihelp/exec.go`** — despite the name, **no exec code**; it is
  cobra exit-state plumbing. Nothing to port.
- **`internal/warpengine/launchers.go`** — `writeLaunchers` (`:31`) early-returns
  on non-Windows (`:32-34`); generates `.cmd` batch files (`:44,55,77`) with
  `@cd /d "%~dp0…"`, backslash paths, CRLF. `removeLaunchers` (`:94`) is neutral.
- **`internal/hubgeometry/hubgeometry.go:309`** — `MenuLauncherPath()` hardcodes
  `ide-menu.cmd`; extension must become GOOS-aware here (Hub Geometry Invariant
  owns geometry tokens/paths).
- **`internal/configengine/edit.go:41-45`** — already portable (`notepad`
  Windows / `vi` else, then `exec.Command`).
- **`internal/vscode/launch_linux.go`** — returns `ErrUnsupported`;
  `launch_windows.go:17` is `exec.Command("cmd", "/c", "code", worktreeDir)` +
  `proc.HideWindow`. `config.go`'s generated `tasks.json` hardcodes `"command":
  "claude"` (`:79`, platform-neutral). `ideengine/spawn.go:16` wires
  `CodeLauncher = vscode.Launch`.
- **`internal/proc`** — `proc_windows.go` (`CREATE_NO_WINDOW` +
  `CREATE_NEW_PROCESS_GROUP`) / `proc_linux.go` (`HideWindow` no-op, `Detach`
  sets `SysProcAttr{Setsid: true}`). Symmetric, **ports cleanly.**
- **`internal/fslink`** — `fslink_linux.go` `CreateDirLink` = `os.Symlink`;
  `IsLink` via `os.ModeSymlink`; `PointsTo` via `EvalSymlinks`. Complete.
- **`tools/deploy/main.go:47-48`** — appends `.exe` via GOOS, already portable.
- **Non-test `runtime.GOOS` inventory:** exactly three sites —
  `tools/deploy/main.go:47`, `configengine/edit.go:41`, `warpengine/launchers.go:32`.
  All accounted for.

## Constraints

From `CONSTRAINTS.md` (authoritative) and discussion:

- **Hub Geometry Invariant** — the GOOS-aware launcher-filename extension lives in
  `internal/hubgeometry`; no other package may construct launcher paths or use
  geometry tokens (`_launchers`, etc.) in path construction. `_lyx`/config paths
  resolve through `hubgeometry.ConfigFile(base, module)`, in test code too.
- **Shuttle Provider-Seam Invariant** — `internal/shell` must be
  provider-invariant (no Claude marker strings, flags, or hook shapes); Claude
  specifics stay inside `internal/shuttleengine/claudeengine`. `shuttleengine`
  never imports `claudeengine`. Enforced by
  `shuttleengine/seam_enforcement_test.go` (import half) + review (semantic half).
- **CLI / Cobra Invariant** — if the capability probe changes observable `mux up`
  behavior, update the affected `Short`/`Long`; errors stay on the
  `internal/output` JSON envelope. Any new command (none currently planned) must
  be registered in `newRoot()` and appear in the pinned help-tree/registration
  tests.
- **lyxtest Leaf Invariant** — `internal/lyxtest` imports only stdlib +
  `hubgeometry`; tests needing real config use `lyxtest.SeedConfig`.
- **Sandbox Suite Coverage** — no new registered module is planned, so no new
  `**Covers:**` tag is required; if a module is added, it must be covered or
  allowlisted (`cmd/lyx/sandbox_coverage_test.go`).
- **Documentation Lifecycle** — the mux contract surface goes in godoc
  (`muxengine/doc.go`), not a standalone doc. Update `docs/overview.md` only if
  the module table/execution stack changes; add the roadmap milestone per the
  roadmap rules. This task adds cross-cutting infra (the `internal/shell` seam),
  so its doc/godoc updates ship **in the same commit** as the code.
- **New candidate invariant:** if `internal/shell` becomes a hard seam (provider
  code never builds shell strings directly), record it in `CONSTRAINTS.md` in the
  same commit — decide during planning whether it warrants a machine-checked or
  review-only invariant.
- **`fslink` directory-only contract** — no reliance on Windows file symlinks;
  junctions/symlinks are directory-only. (No change expected here; noted for
  awareness.)

## Testing

Per-module approach (TDD candidates named; assertion shapes left to mill-plan):

- **`/proc` descendant-closure (`muxengine`, TDD):** the pure closure computation
  over a PID→PPID map is the primary TDD candidate — author `/proc/<pid>/stat`
  fixtures (including edge cases: missing pid mid-walk, a pid re-parented to 1,
  a cycle-guard, the target pid itself) and drive the descendant-set logic. The
  thin `/proc` filesystem-read layer is kept minimal behind the pure function.
- **`serverProcessesOnSocket` `/proc/*/cmdline` scan (`muxengine`, TDD):** the
  pure matcher — given a set of `(pid, argv)` pairs, return the pids whose argv
  contains both the tmux binary and the `-L <socket>` token — is fixture-driven
  (matches, near-misses like a different socket, the binary-without-`-L` case).
  The tmux CLI-absence liveness path is exercised by the integration/contract
  test, not the unit test.
- **`internal/shell` quoting/chaining (TDD):** both the pwsh impl (single-quote
  doubling, `& <bin>`, `Get-Content -Raw`) and the posix impl (single-quote
  escaping, direct exec / `$(cat …)`), plus path-shape handling — pure string
  transforms, fixture-driven, no OS calls.
- **`.sh` launcher generation (`warpengine`, TDD):** drive the **pure,
  GOOS-parameterized content builder** (target OS + climb → bytes) so both the
  `.cmd` and `.sh` outputs are asserted on a Windows box — shebang, body, `cd`
  path (forward slashes for `.sh`), mode bits, and line endings (LF for `.sh`,
  CRLF for `.cmd`); keep the existing `.cmd` tests green. Cross-check the
  GOOS-aware **menu** filename against `hubgeometry` (only the menu launcher's
  extension lives there).
- **Capability probe (unit, faked):** unit-test the surface check with a fake
  multiplexer responder — version below pin → typed error; missing subcommand /
  format var → typed error; healthy → ok. No live binary in the unit test.
- **Multiplexer contract test (`//go:build integration`):** the live behavioral
  assertions — spawns a real server via the configured binary, exercises the
  full subcommand set + `-F` format + edge-case behaviors, skips cleanly when the
  binary is absent. Runs against psmux now (Windows); against tmux in the
  follow-up.
- **Seamed-package verification:** confirm `proc`/`fslink`/`vscode` Linux
  build-tag files compile and their logic is correct by reading + existing tests;
  add a `GOOS=linux go build ./...` **cross-compile CI gate** as the mechanical
  proof the whole tree builds for Linux.
- **Deferred (follow-up, not here):** running any of the above against a live
  Linux process tree / real tmux / the green sandbox smoke suite.

## Q&A log

- **Q:** Process-tree reaping — write a real Linux impl or defer? **A:** Seam it
  and write a real `/proc`-based impl now, unit-tested against fixtures; leaving
  the silent-`nil` degrade is a landmine.
- **Q:** `/proc` descendant discovery — PPID walk or session/process-group id?
  **A:** PPID chain from `/proc/<pid>/stat` — direct analog of the WMI
  parent-walk, preserves existing semantics.
- **Q:** pwsh/shell coupling — real abstraction or inline GOOS branches? **A:** A
  provider-invariant `internal/shell` leaf with pwsh + posix impls; keeps the
  Shuttle Provider-Seam intact.
- **Q:** VS Code launch on Linux — implement or leave `ErrUnsupported`? **A:**
  Implement (`exec.Command("code", worktreeDir)`); the stub leaves `lyx ide
  spawn` knowingly broken.
- **Q:** Launchers — full `.sh` branch or defer? **A:** Full `.sh` branch +
  GOOS-aware launcher filename in `hubgeometry`.
- **Q:** Contract test — Go integration test or sandbox-suite scenario? **A:** Go
  `//go:build integration` test with exact-format assertions; complements (not
  replaces) `SANDBOX-MUX-SUITE`.
- **Q:** Capability probe — when does it run? **A:** At server-ensure / `mux up`,
  once per boot; typed error on missing surface or sub-pin version.
- **Q:** Linux config defaults + version pin? **A:** GOOS-aware template defaults
  (`tmux`/`bash`) + pinned multiplexer min-version enforced by the probe; env
  overrides still win; pin is multiplexer-only.
- **Q:** Where to document the contract surface? **A:** `muxengine/doc.go` godoc
  — the standalone module doc was already deleted per the Documentation
  Lifecycle; a separate doc would just drift.
- **Q:** How to record the deferred real-Linux follow-up? **A:** A "Phase 3:
  real-Linux validation" roadmap milestone + the explicit checklist in this
  discussion's "Out" section.
- **Q:** Testing strategy? **A:** TDD the pure logic (`/proc` closure, shell
  quoting, `.sh` generation), fake-unit-test the probe, integration-test the live
  multiplexer, add a `GOOS=linux go build` cross-compile CI gate.
- **Q:** [review r1 gap] How does the Linux `serverProcessesOnSocket` discover
  server processes on a `-L` socket, given tmux (unlike psmux) reports absence
  honestly and has no `psmux.exe`/`__warm__` analog? **A:** Use tmux's real CLI
  absence signal (non-zero `has-session`) for liveness, plus a `/proc/*/cmdline`
  scan (argv contains tmux binary + `-L <socket>`) only for the stray-process /
  worktree-busy guarantee. Not a literal WMI-style scan (redundant vs tmux's
  honest signal), not deferred.
- **Q:** [review r1 gap] One version pin can't compare psmux and tmux (different
  `-V` schemes) — how to pin? **A:** Two per-binary pinned min-version constants,
  each with its own `-V` parser, GOOS-selected at probe time. Not a single
  generic constant, not capability-only (keep the numeric floor), not
  psmux-only.
