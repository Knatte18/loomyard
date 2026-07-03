# Discussion: Dedicated sandbox suite for mux

```yaml
task: Dedicated sandbox suite for mux
slug: mux-sandbox-suite
status: discussing
parent: internal-mux
```

## Problem

mux-testing is fundamentally different from the rest of `lyx`. The general
`tools/sandbox/SANDBOX-SUITE.md` is host/weft-centric: a black-box agent drives `lyx`
non-interactively against real GitHub test repos and reads JSON output. mux does not fit
that model — it needs a live psmux server + pwsh, operates on a worktree's ephemeral
`.lyx/` overlay (not the host/weft hub model), `attach` is an interactive terminal
takeover, and the value is visual: seeing panes pop up and the layout hold.

When PR #52 (internal-mux) landed, mux was pushed in as scenario **S9** of the main
suite only to satisfy the coverage guard (`cmd/lyx/sandbox_coverage_test.go`). That is
the wrong home. This task extracts mux into its own suite file with its own launcher,
generalizes the coverage guard to scan multiple suite files, moves S9 out with a
pointer, and updates the CONSTRAINTS.md invariant + docs in the same commit. mux is
growing (clusters via psmux windows, daemon later), so the dedicated suite is the right
foundation now.

## Scope

**In:**

- New `tools/sandbox/MUX-SANDBOX-SUITE.md` — mux-specific preconditions and scenarios
  (`M0`…`Mn`), mirroring the main suite's structure (fingerprint header, verdict
  buckets, findings capture, session-log format).
- New `mux-suite` subcommand in the `tools/sandbox` Go tool + a new
  `mux-sandbox-suite.cmd` launcher — full launcher parity with the existing `suite`
  subcommand, sharing its mechanics via a parameterized `runSuite`.
- Generalize `cmd/lyx/sandbox_coverage_test.go` (`parseCoveredModules`) to scan every
  file matching the glob `tools/sandbox/*SANDBOX-SUITE.md`, and update every
  single-file reference in it: the three assertion error-message strings, the
  `t.Fatalf` in `parseCoveredModules`'s read path, and the package/function doc
  comments that describe single-file behaviour.
- Move S9 out of `SANDBOX-SUITE.md`, leave a short pointer, and fix the collateral
  drift there (session-log template lists S9; "Capturing findings" says refs are
  `S0`–`S8`).
- Update `CONSTRAINTS.md` Sandbox Suite Coverage invariant to the multi-suite scan
  (same commit as the guard change).
- Docs: `docs/sandbox-howto.md`, `docs/sandbox-hub.md`, `docs/modules/mux.md` pointer,
  and `docs/overview.md` only if its docs-list wording needs the new file named.

**Out:**

- No changes to the mux implementation (`internal/muxcli`, `internal/muxengine`,
  render) — this task tests it, it does not modify it.
- No changes to the `fetch` subcommand or the report contract — the mux suite reuses
  the shared `sandbox-report.json` name and the existing `sandbox-fetch.cmd` flow.
- No `lyx mux update` CLI verb — surfacing a hidden strand stays engine-API-only (v1
  design); the suite scopes around it rather than adding surface area.
- No `docs/roadmap.md` entry — this is an extraction/hardening of delivered work, not
  a planned milestone (per CLAUDE.md's roadmap rule).
- No smoke-test (`-tags smoke`) changes — the hermetic and smoke tests are unaffected.

## Decisions

### Launcher parity, not a doc-only runbook

- Decision: The mux suite gets full launcher integration: a new `mux-suite` subcommand
  in `tools/sandbox` and a new `mux-sandbox-suite.cmd` wrapper, launching an
  interactive claude agent in the Hub host repo with the instruction
  "Read ./MUX-SANDBOX-SUITE.md and follow the instructions in it exactly."
- Rationale: The operator wants autonomy — the agent drives the suite, the operator
  watches. Most of the existing plumbing (fingerprint header, git-exclude, stale-report
  delete, claude launch) is reused verbatim.
- Rejected: Doc-only manual runbook (no autonomy); a half-way copy-only step (no reason
  to stop short once `runSuite` is parameterized).

### Launcher shape: parameterized `runSuite`, separate subcommand

- Decision: Refactor `runSuite` to take the embedded doc content, the suite file name,
  and the default instruction as parameters. `suite` and `mux-suite` are two thin
  dispatches over it; `MUX-SANDBOX-SUITE.md` is embedded via its own `//go:embed`.
  A new `mux-sandbox-suite.cmd` mirrors `sandbox-suite.cmd` (including `-claude` /
  `-prompt` pass-through).
- Rationale: Matches the documented "each sandbox launcher does exactly one thing"
  rule; keeps the mechanics single-sourced.
- Rejected: A `-mux` flag on the existing `suite` subcommand (muddies the
  one-launcher-one-thing convention).

### Runs in the sandbox Hub host repo

- Decision: A mux suite session runs in `lyx-test-HUB/lyx-test`, exactly like the main
  suite — same cwd, same black-box rule.
- Rationale: Fully parallel with the main suite; the host repo is already
  `lyx init`-ed; isolated from the dev repo; the per-hub psmux server name
  (`lyx-<hub-basename>-<short-hash>`) guarantees no collision with a real dev-hub mux
  session.
- Rejected: A real loomyard worktree (risks stomping a live mux session); a dedicated
  scaffolded directory (setup cost, no isolation gain).

### Shared report file and fetch flow

- Decision: The mux suite agent writes findings to the same `./sandbox-report.json`
  (same schema, same `source: "sandbox-report"`), and the existing shared `fetch`
  subcommand / `sandbox-fetch.cmd` collects it. Scenario refs distinguish suites.
- Rationale: Each suite run deletes the stale report before launching and the operator
  fetches per session, so there is no collision in the run→fetch cycle; reuse beats a
  parallel report pipeline.
- Rejected: Per-suite `mux-sandbox-report.json` (would need fetch changes or a manual
  drain path for no benefit).

### Coverage guard discovers suites via glob

- Decision: `parseCoveredModules` scans every file matching
  `tools/sandbox/*SANDBOX-SUITE.md` (via `filepath.Glob`) and unions their
  `**Covers:**` tags. Both sub-assertions stay: `discovered_non_empty` guards the
  union, and Assert-2 (drift) still catches stale tags. Error messages name the
  scanned pattern (and, where useful, the specific file a stale tag came from).
- Rationale: Future suites (shuttle, review, loom) count automatically with zero test
  edits. A deleted suite file still fails Assert-1 indirectly because its module loses
  coverage. As a guard against a mistyped glob silently matching only one file, the
  test asserts the glob matches at least two files (the count in the repo today; a
  future suite only raises it).
- Rejected: An explicit hardcoded file list (the hand-maintained-list pattern the
  invariant avoids elsewhere).

### Scenario IDs use an `M` prefix

- Decision: The mux suite numbers scenarios `M0`, `M1`, … and report `ref` values use
  those IDs.
- Rationale: Unambiguous refs across suites in the shared report file.
- Rejected: Continuing `S` numbering from S9 (ties the files' numbering together).

### `attach` is an operator-assisted visual checkpoint

- Decision: The attach scenario instructs the agent to pause and tell the operator to
  run `lyx mux attach` in a **second terminal**, visually confirm the layout, then
  detach; the agent records the operator's verdict.
- Rationale: The agent session runs in the operator's terminal (`launchAgent` inherits
  stdin/stdout), so an agent-run `lyx mux attach` would collide with its own terminal:
  either the takeover seizes the terminal the claude TUI owns, or the command runs as
  a piped tool subprocess with no visible takeover at all. Either way the agent cannot
  visually confirm panes — and visual confirmation is the suite's stated value, which
  only a human can give. Everything else stays autonomous.
- Rejected: Agent-only indirect verification via `capture-pane` (never validates the
  visual takeover); optional human check (dilutes the one scenario that matters
  visually).

### Scenario list (confirmed, with two amendments)

- Decision: Scenarios cover: `up`; `add` (non-hidden); `add --anchor hidden`; `status`;
  `attach` (operator-assisted, above); `resume` (replay of the launch/resume cmd);
  recursive `remove`; `add`-before-`up` friendly error
  (`no mux session; run "lyx mux up"`); ≥2-top-strands-0-stack layout sanity (the last
  top band tiles the full box); crash-resume (server killed → `resume` adopts a new
  pane); `down` leaving no stray psmux state. Amendments:
  1. The hidden scenario is scoped to what is black-box reachable: `add --anchor
     hidden` → no pane created, `status` shows the strand, `resume` skips it.
     Surfacing is engine-API-only in v1 (no `lyx mux update` verb) and is not tested.
  2. Crash-resume kills the psmux server directly (`psmux -L <server> kill-server`) —
     a documented, controlled exception to the black-box rule, in the same style as
     S6's controlled `lyx init` exception in the main suite.
- Rationale: Matches the proposal; the amendments keep every step actually drivable
  from the CLI surface.
- Rejected: Dropping the hidden scenario (add-hidden/status/resume-skip is still a
  meaningful contract); testing surface via engine API (not black-box).

### Live-psmux caveat

- Decision: The suite carries the same caveat S9 had: if psmux (or pwsh) is
  unavailable in the sandbox session, the agent notes that as the outcome — it is not
  a mux defect. The `**Covers:** mux` tag satisfies the coverage guard regardless of
  runtime availability.
- Rationale: Coverage is a static doc property; runtime availability is environmental.
- Rejected: Failing the session on missing psmux.

## Technical context

- **Main suite launcher** (`tools/sandbox/suite.go`): `runSuite(parentDir,
  claudeOverride, promptOverride)` fingerprints `lyx.exe` from PATH
  (`binaryFingerprint`, first-12 SHA-256), renders `fingerprint header + embedded doc`
  (`renderScheme`), writes it into `lyx-test-HUB/lyx-test/`, `ensureGitExclude`s the
  suite file and `sandbox-report.json`, deletes the stale report, resolves claude, and
  `launchAgent`s interactively with `--dangerously-skip-permissions`. Constants:
  `suiteFileName = "SANDBOX-SUITE.md"`, `defaultInstruction = "Read ./SANDBOX-SUITE.md
  and follow the instructions in it exactly."`. The doc is embedded via
  `//go:embed SANDBOX-SUITE.md`. Test seams: `lookPath`, `launchAgent` package vars.
- **Dispatch** (`tools/sandbox/main.go`): subcommands `build` / `suite` / `fetch`;
  shared consts `hubName = "lyx-test-HUB"`, `hostDirName = "lyx-test"` (suite.go).
  `.cmd` wrappers live at the repo root (`sandbox-suite.cmd` etc.) and hardcode
  machine-specific paths; `sandbox-suite.cmd -claude <path>` / `-prompt <text>`
  forward to flags.
- **Report** (`tools/sandbox/report.go`): `reportFileName = "sandbox-report.json"`,
  `reportSourceID = "sandbox-report"`; `runFetch` validates and stamps
  `meta.fingerprint`, writing `.scratch/sandbox-report-<fingerprint>.json`. Unchanged
  by this task.
- **Coverage guard** (`cmd/lyx/sandbox_coverage_test.go`):
  `TestSandboxCoverage_AllModulesCoveredOrExcluded` enumerates modules from
  `newRoot().Commands()` (skipping `help`/`completion`), parses `**Covers:**` lines
  via `coversLinePattern`, asserts covered-or-excluded (Assert-1) and no stale tags
  (Assert-2), with a `discovered_non_empty` sub-test. `parseCoveredModules` resolves
  the repo root from `runtime.Caller(0)` (three `filepath.Dir` walk-ups) and reads the
  single hardcoded suite path — this is the function to generalize. Every
  single-file reference needs rewording for multi-suite: three `t.Errorf`/`t.Error`
  assertion strings, the `t.Fatalf` in the read path, and the package/function doc
  comments (file header, test doc comment, `parseCoveredModules` doc comment) that
  name `tools/sandbox/SANDBOX-SUITE.md` and describe single-file behaviour.
- **Main suite doc** (`tools/sandbox/SANDBOX-SUITE.md`): S9 sits at the end of
  `## Scenarios` with `**Covers:** mux` and the live-psmux caveat note — the block to
  extract. Collateral drift to fix when it moves: the `## Session log format` template
  lists `S9:`, and `## Capturing findings` says `ref` is the scenario id (`S0`-`S8`)
  — after removal the range is `S0`–`S8` again, so that line becomes correct; verify
  wording. A `## Notes` bullet says the scenario set is "deliberately small and
  host/weft-centric … add scenarios as modules grow (shuttle, review, loom)" — this is
  where the multi-suite model can be acknowledged alongside the S9 pointer.
- **mux CLI surface** (from `docs/modules/mux.md` + `internal/muxcli`): verbs `up`,
  `add`, `remove <guid>`, `status`, `attach`, `resume`, `down`. `add` flags: `--cmd`
  (required), `--role`, `--round`, `--name`, `--resume-cmd`, `--parent <guid>`,
  `--anchor top|below-parent|hidden` (default `below-parent`; `own-window` rejected as
  deferred), `--focus`. `remove` requires `--recursive` on a non-leaf. Pre-flight
  session check gives `add`/`remove` before `up` the friendly error. `attach` is the
  documented envelope exception (pre-flight errors stay on the JSON envelope; the
  handover tail emits no JSON). Strand state persists in `.lyx/mux.json`; the op lock
  is `.lyx/mux.lock`. Server name: `lyx-<hub-basename>-<short-hash>`; session name:
  worktree basename. `lyx mux status` output includes what the agent needs to identify
  the server/session for the controlled kill-server step.
- **Environment facts the suite doc must encode** (from `docs/modules/mux.md`): psmux
  at `C:\Code\tools\bin\psmux.exe`, default pane shell PowerShell 7; explicit binary
  paths, never PATH aliases (the 0-byte WindowsApps `pwsh` stub renders nothing under
  ConPTY); mux sanitizes the psmux server env of `CLAUDE_CODE_*` vars — relevant
  because the suite agent *is* a Claude session spawning `lyx`, which is mux's primary
  use case.
- **Hub geometry invariant note:** the suite doc and launcher are outside the Go
  production tree (`tools/` + a test file change in `cmd/lyx/`), but the guard test
  must keep resolving paths via `runtime.Caller` as today — no new geometry-token
  usage is involved.

## Constraints

From `CONSTRAINTS.md` (read this session):

- **Sandbox Suite Coverage** — the invariant this task modifies. The updated text must
  describe multi-suite scanning (glob `tools/sandbox/*SANDBOX-SUITE.md`), keep the
  module-granularity rule, the allowlist, and the exists⇒covered-or-excluded rule, and
  land in the same commit as the guard change (Documentation Lifecycle).
- **CLI / Cobra Invariant** — no CLI changes in this task, but the suite doc's command
  descriptions must match the actual help/behaviour (help accuracy is a review
  obligation; the doc is derived from the live surface, not from stale drafts).
- **Hub Geometry Invariant** — untouched; no production path construction is added.
  The guard test already resolves the repo root via `runtime.Caller`, which stays.
- **Documentation Lifecycle** — docs updates (`docs/sandbox-howto.md`,
  `docs/sandbox-hub.md`, `docs/modules/mux.md`, CONSTRAINTS.md) ship in the same
  commit as the behaviour change.
- Project rule: `docs/roadmap.md` is not updated (not a planned milestone).

## Testing

- **`cmd/lyx/sandbox_coverage_test.go`** — the TDD candidate. Generalize
  `parseCoveredModules` to the glob; assert post-change: `mux` is covered via
  `MUX-SANDBOX-SUITE.md`, the union is non-empty, Assert-2 still flags a stale tag in
  *either* file. Both existing sub-tests (`discovered_non_empty`, the assert loops)
  survive. The test asserts the glob matches ≥2 files (decided, not optional) so a
  silently-wrong pattern can't produce a vacuous pass on one file.
- **`tools/sandbox` unit tests** (`suite_test.go`, `main_test.go`) — extend for the
  parameterized `runSuite`: the `mux-suite` subcommand writes `MUX-SANDBOX-SUITE.md`
  (not `SANDBOX-SUITE.md`) into the host repo, prepends the fingerprint header,
  git-excludes the new file name, deletes the stale shared report, and passes the
  mux-specific default instruction to the `launchAgent` seam. Existing `suite` tests
  must stay green unchanged (proving the refactor is behaviour-preserving).
- **Doc-only assertions** — the moved S9 content: main suite no longer contains
  `**Covers:** mux` (the guard test itself proves the tag now comes from the new
  file); session-log template no longer lists S9.
- **No smoke/integration additions** — the `-tags smoke` psmux test is out of scope;
  the suite itself is the manual/agent-driven test layer.
- Full `go test ./...` green is the gate; the coverage guard runs on every `go test`.

## Q&A log

- **Q:** Doc-only runbook or full launcher integration for the mux suite? **A:**
  Full launcher parity — the operator wants autonomy; reuse the existing
  `tools/sandbox` plumbing, it is "just another launcher".
- **Q:** Where does a mux suite session run? **A:** Sandbox Hub host repo — fully
  parallel with the main suite.
- **Q:** How does the coverage guard find suite files — glob or explicit list?
  **A:** Glob `tools/sandbox/*SANDBOX-SUITE.md`.
- **Q:** Shared or per-suite findings report? **A:** Shared `sandbox-report.json` +
  shared fetch (follows from launcher parity and same run cwd; per-run stale-delete
  and per-session fetch avoid collisions).
- **Q:** Scenario ID scheme? **A:** `M0`, `M1`, … — unambiguous refs across suites.
- **Q:** Launcher shape? **A:** New `mux-suite` subcommand + `mux-sandbox-suite.cmd`,
  with `runSuite` parameterized and shared; `fetch` untouched.
- **Q:** How can the agent exercise `attach` without a TTY? **A:** Operator-assisted
  visual checkpoint — the agent pauses and asks the operator to attach in a second
  terminal and confirm; the agent records the verdict.
- **Q:** Scenario list amendments? **A:** Accepted both: hidden scenario scoped to
  add-hidden/status-shows/resume-skips (no CLI surface for surfacing in v1), and
  crash-resume via direct `psmux kill-server` as a documented controlled black-box
  exception.
- **Q:** Docs to update? **A:** All four touchpoints — sandbox-howto.md,
  sandbox-hub.md, docs/modules/mux.md pointer, CONSTRAINTS.md invariant;
  overview.md only if its docs-list wording needs it.
