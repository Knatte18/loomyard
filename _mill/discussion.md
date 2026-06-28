# Discussion: Sandbox test-suite launcher and task harvester

```yaml
task: Sandbox test-suite launcher and task harvester
slug: sandbox-suite
status: discussing
parent: main
```

> **Title note.** The recorded title is legacy. The **"task harvester" half is
> dropped** (see Decisions → *No task harvester*). This task delivers only the
> **test-suite launcher**.

## Problem

The sandbox Hub (`docs/sandbox-hub.md`) is a dogfooding bench: a deployed `lyx.exe`
exercised against real GitHub test repos (`Knatte18/lyx-test` host +
`Knatte18/lyx-test-weft` weft + its wiki board), materialized on disk at
`C:\Code\lyx-test-HUB\` by `lyx warp clone`. Today `tools/sandbox` only *builds /
resets* that Hub (a clone wrapper). The actual dogfooding — driving `lyx` by hand
in the Hub and treating every break as a LoomYard bug — is still a fully manual
ritual described in a `.scratch` markdown scheme.

We want a one-command way, run **from the lyx repo**, that **starts an interactive
Claude (CLI) session** ("the sandbox-agent") whose single instruction is *"read this
file and follow it"* — where the file is a **fresh copy** of a tracked test-scheme
that defines the tests to run. The sandbox-agent then executes those tests against
`lyx.exe` **as a black box** (it must know nothing about lyx's source), finding bugs
by using the real binary in a real repo. **Why now:** cobra-based self-describing
help has landed, so an operator-with-only-the-binary scenario is finally testable;
the manual scheme lives untracked in `.scratch` and needs to become a tracked,
evolving artifact wired to a launcher.

## Scope

**In:**

- A new **`suite` subcommand** on the existing `tools/sandbox` Go tool (the current
  clone behavior becomes the default / `build` subcommand — back-compat preserved).
- A **tracked test-scheme markdown** at `tools/sandbox/test-scheme.md`, refreshed
  from the existing `.scratch/test-scheme-sandbox.md` and brought up to date
  (cobra help has landed; findings now flow through `lyx ghissues create`, not the
  old "roll into LoomYard tasks" wording).
- `sandbox suite` behavior:
  - **`//go:embed`** the tracked `test-scheme.md` into the binary (so each `go run`
    rebuild embeds the current tracked version → the copy is always fresh).
  - Resolve the deployed `lyx` on PATH and **fingerprint** it (abs path + size +
    modtime + short sha256); stamp that into the header of the rendered scheme.
  - Write the rendered scheme to the **Hub host repo root** as `SANDBOX-SUITE.md`,
    overwritten fresh each run.
  - Append `SANDBOX-SUITE.md` to the Hub host repo's **`.git/info/exclude`**
    (idempotent) so the agent's black-box `git status` stays clean.
  - Launch **`claude --dangerously-skip-permissions "<instruction>"`** as an
    interactive TUI, **cwd = Hub host repo**, inheriting the terminal stdio/env;
    wait for it and propagate its exit code.
  - Default instruction: `Read ./SANDBOX-SUITE.md and follow the instructions in
    it exactly.` Overridable via a `-prompt` flag; claude binary overridable via
    `-claude`.
  - If the Hub does not exist, **error** with a message telling the operator to run
    the build first (no auto-build).
- Update `docs/sandbox-hub.md` to document the `suite` command and workflow (same
  commit).

**Out:**

- **The "task harvester"** — dropped from the old 031 design. Do not build it.
  (Findings reach the board via a separate downstream pipeline: sandbox-agent →
  `lyx ghissues create` → GitHub issue → `mill-ghissues-to-tasks`. None of that is
  this task.)
- **Any change to lyx itself / the `cmd/lyx` CLI surface.** No `lyx version`
  command, no new `internal/` module. (Version traceability is handled entirely
  inside `tools/sandbox` via the binary fingerprint — see Decisions.)
- **Any change to `tools/deploy`.** Deploy stays a separate manual step; `suite`
  does not auto-deploy.
- **psmux / env-sanitization wiring.** Not needed yet (the `mux` module is not
  written). When `mux` lands it will *replace* the `claude` launch in `suite`;
  until then it is a plain interactive `claude` TUI with inherited env.
- Authoring the *content* of future test scenarios — the template ships a starter
  set; the operator evolves it over time.

## Decisions

### Subcommand on `tools/sandbox`, not a sibling dir

- Decision: Add a `suite` subcommand to `tools/sandbox`; current clone behavior
  becomes the default (`build`). The launcher (`sandbox.cmd`) passes the subcommand
  through.
- Rationale: One tool, one Hub. A sibling `tools/sandbox-suite/` next to
  `tools/sandbox/` is vague ("which one is just `sandbox`?").
- Rejected: `tools/sandbox-suite/` or `tools/sandbox-agent/` sibling dir.

### No task harvester

- Decision: Do not build the report-reading / `lyx board upsert` harvester.
- Rationale: Superseded by the `lyx ghissues create` → GitHub → `mill-ghissues-to-tasks`
  pipeline, which the sandbox-agent drives itself.
- Rejected: Re-implementing the 031 harvester.

### Interactive TUI, never headless

- Decision: Launch `claude "<instruction>"` as an interactive terminal TUI (with
  `--dangerously-skip-permissions`), run in the foreground, propagate exit code.
- Rationale: CLAUDE.md's economic stance — headless `claude -p` bills as API;
  interactive keeps subscription coverage. The operator also wants to *watch*.
- Rejected: `claude -p` headless; psmux-driven session (deferred until `mux` exists).

### Tracked template, embedded, copied fresh each run

- Decision: The master scheme is tracked at `tools/sandbox/test-scheme.md`,
  `//go:embed`-ed into the binary, and written into the Hub host repo as
  `SANDBOX-SUITE.md` on each `suite` run.
- Rationale: The original belongs in the lyx repo (developed alongside `lyx.exe`);
  the agent must read a *copy* in the sandbox repo. `go run` rebuilds every
  invocation, so the embedded bytes are always the current tracked version — "fresh
  copy" falls out for free, with no runtime file-path resolution.
- Rejected: Reading the scheme from a runtime source path (fragile under `go run`);
  leaving it in `.scratch` (untracked, can't evolve in version control).

### Black-box isolation

- Decision: The agent runs with **cwd = Hub host repo** (`lyx-test`), a separate git
  repo with no lyx source beside it; the launcher passes it only `lyx` (on PATH) and
  the copied scheme. Black-box framing lives *inside* the scheme; the wrapper prompt
  stays minimal.
- Rationale: The whole point is to find bugs by using `lyx.exe` like a real user who
  has never seen the code.
- Rejected: Running in the lyx repo / cd-ing from it (would expose source).

### Version capture via binary fingerprint (not `lyx version`)

- Decision: `suite` resolves the deployed `lyx` on PATH and fingerprints it
  (absolute path, file size, modtime, short sha256), stamping that into the
  `SANDBOX-SUITE.md` header so each run — and each issue the agent files —
  references exactly which binary was under test.
- Rationale: lyx has **no** version surface today (no `--version`, no ldflags). The
  launcher can't trust the lyx repo's git HEAD either — `deploy` is a separate manual
  step, so source may have moved since the deployed snapshot. A file fingerprint is
  the only black-box-safe "which binary" identifier available without changing lyx.
- Rejected: Adding a real `lyx version` + deploy ldflags stamping (expands into the
  cobra CLI invariant and `tools/deploy` — explicitly out of scope; a good future
  LoomYard task but not this one); deriving version from lyx repo git HEAD
  (unreliable vs. the deployed snapshot).

### Keep the copied scheme out of the agent's `git status`

- Decision: Append `SANDBOX-SUITE.md` to the Hub host repo's `.git/info/exclude`
  (idempotent), rather than editing a tracked `.gitignore`.
- Rationale: Keeps the agent's black-box `git status` clean without mutating tracked
  files in the test repo.
- Rejected: Editing tracked `.gitignore`; leaving it visible as untracked noise.

### Build and run stay separate

- Decision: `suite` errors if the Hub is absent (no auto-build) and does not run
  `deploy`.
- Rationale: Keeps "what version did this run exercise" deliberate; auto-deploy would
  silently rebuild the binary under test and blur the snapshot.
- Rejected: Auto-build Hub; auto-deploy fresh binary.

## Technical context

- **Existing tool:** `tools/sandbox/main.go` — `flag`-based, consts `hostURL`,
  `weftURL`, `hubName = "lyx-test-HUB"`; seams `cloneRun(parentDir)` and
  `removeAll`; `decideClone(hubPath, reset)`; `main()` requires `-parent`, computes
  `hubPath = <parent>/lyx-test-HUB`. The launcher `sandbox.cmd` invokes
  `go run ./tools/sandbox -parent C:\Code %*`.
- **Hub geometry:** host repo lives at `<parent>/lyx-test-HUB/lyx-test/` (basename of
  `hostURL`). The agent's cwd and the `SANDBOX-SUITE.md` destination are this dir.
  Add a `hostDirName = "lyx-test"` const (or derive from `hostURL` basename) — mirror
  the existing const style.
- **Subcommand wiring gotcha:** Go's `flag` package stops at the first non-flag
  token. Keep `-parent` (and `-reset`, for back-compat) on the top-level flagset so
  the existing `sandbox.cmd` and `sandbox.cmd -reset` (no subcommand → `build`) keep
  working; dispatch on the first positional arg; let `suite` parse its own
  `-claude` / `-prompt` from the remaining args. Bare invocation and `-reset` route
  to `build`.
- **Existing claude-launch reference:** `internal/muxpoc` resolves `claude` via
  `exec.LookPath("claude")` and launches it — reuse that resolution idiom (PATH +
  optional `-claude` override). Do **not** copy muxpoc's env sanitization (out of
  scope per Decisions).
- **Reference scheme to refresh:** `.scratch/test-scheme-sandbox.md` (host/weft-
  centric S0–S6 scenarios). Bring forward, but (a) drop the "operator must be handed
  the command surface until cobra lands" caveats (cobra has landed — S0 *is* the
  help-discovery test), and (b) replace the "Roll ❌/⚠️ into LoomYard tasks" capture
  step with: file each finding via `lyx ghissues create` from inside the Hub.
- **Path invariant (CONSTRAINTS.md):** applies to `internal/` + `cmd/lyx/main.go`
  only. `tools/` is exempt and uses `filepath` + the `-parent` flag (no `os.Getwd`,
  no `git rev-parse`). Keep it that way — build all paths from `-parent`.
- **Cobra invariant:** does **not** apply — this is a `tools/` dev binary, not a lyx
  CLI module. No `Command()` / `RunCLI` seam, no helptree-test updates.

## Constraints

- **Black-box:** the launcher must never hand the agent a path into the lyx source
  tree; cwd is the Hub host repo and the only inputs are `lyx` (PATH) + the copied
  scheme.
- **No headless claude.** Interactive TUI only (`claude "<prompt>"`).
- **`tools/` stays self-contained** — no import of `internal/paths`; paths derive
  from `-parent`. No `os.Getwd` / `git rev-parse` (path invariant, even though
  `tools/` is technically exempt, follow the spirit).
- **Idempotent `.git/info/exclude`** edit — never duplicate the entry, never clobber
  existing content; create the `info/` dir / `exclude` file if missing.
- **Docs in the same commit** (`docs/sandbox-hub.md`). Not the roadmap (tooling, not
  a planned milestone).

## Testing

Follow the existing `tools/sandbox/main_test.go` pattern: testability seams as
package-level `var` function values, `t.TempDir()` fixtures, no network and no real
`claude`/`lyx` launches.

TDD candidates / scenarios to cover:

- **Subcommand dispatch** — no positional → `build`; `-reset` (bare) → `build` with
  reset; `suite` positional → suite path; back-compat for current `sandbox.cmd`
  invocations.
- **Hub-missing error** — `suite` with no Hub host repo present returns a clear,
  actionable error (and does not launch claude).
- **Binary fingerprint** — `fingerprintBinary(path)` over a temp file yields correct
  size + stable sha256; missing binary → error (message points operator to deploy).
- **Scheme rendering** — the rendered `SANDBOX-SUITE.md` embeds the fingerprint
  header and the embedded template body; written to the host repo root, overwriting
  any prior copy.
- **`.git/info/exclude` idempotency** — entry added once; second call is a no-op;
  pre-existing unrelated content preserved; missing `info/exclude` created.
- **Launch seam** — a `launchAgent` seam is invoked with the expected cwd (host
  repo), resolved claude path, and instruction string; `-prompt` and `-claude`
  overrides are honored; the seam's exit code is propagated.
- **claude-not-on-PATH** — `LookPath` failure surfaces a clear error.

Manual verification (note for finalize, not a unit test): an interactive `claude`
TUI launched as a child of `go run` must attach to the real console on Windows —
confirm the TUI renders and accepts input in a live `sandbox.cmd suite` run.

## Q&A log

- **Q:** What does the "test-suite launcher" run? **A:** It starts an interactive
  Claude CLI session whose one instruction is to read a copied test-scheme `.md` and
  do what it says; the agent executes those tests against `lyx.exe`. lyx has **no**
  orchestrator pipeline — that earlier framing was wrong.
- **Q:** Build the task harvester? **A:** No — dropped from the 031 design; replaced
  by sandbox-agent → `lyx ghissues create` → GitHub issue → `mill-ghissues-to-tasks`.
- **Q:** Headless or interactive? **A:** Never headless. Interactive TUI
  (`claude "<instruction>"`). psmux deferred until `mux` is written (it will later
  replace the claude launch).
- **Q:** Where does the scheme live? **A:** Tracked original in the lyx repo
  (`tools/sandbox/test-scheme.md`), fresh copy pushed into the Hub host repo as
  `SANDBOX-SUITE.md` each run.
- **Q:** Agent working directory? **A:** The Hub host repo (`lyx-test`) — black-box,
  no lyx source beside it.
- **Q:** New sibling tool dir? **A:** No — a `suite` subcommand on `tools/sandbox`
  (two `sandbox*` dirs would be vague).
- **Q:** Hub missing on `suite`? **A:** Error and tell the operator to build first;
  do not auto-build.
- **Q:** Wrapper prompt wording? **A:** Minimal — `Read ./SANDBOX-SUITE.md and
  follow the instructions in it exactly.` — easily overridable (`-prompt`). Black-box
  rules live inside the scheme.
- **Q:** Auto-deploy a fresh binary first? **A:** No — keep deploy separate so the
  tested snapshot stays deliberate; the scheme reminds the operator.
- **Q:** Keep the copied scheme out of `git status`? **A:** Yes — append to the Hub
  host repo's `.git/info/exclude`.
- **Q:** Strip `CLAUDECODE` / `CLAUDE_CODE_*` env vars? **A:** Not needed yet (no
  `mux` module); `mux` will later replace the launch.
- **Q:** Capture the tested lyx.exe version? **A:** Yes, but **not** via a `lyx
  version` command — fingerprint the deployed binary inside `tools/sandbox` (path +
  size + modtime + short sha256) and stamp it into the scheme header.
