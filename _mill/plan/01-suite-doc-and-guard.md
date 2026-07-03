# Batch: suite-doc-and-guard

```yaml
task: Dedicated sandbox suite for mux
batch: suite-doc-and-guard
number: 1
cards: 4
verify: go test ./cmd/lyx/
depends-on: []
```

## Batch Scope

This batch delivers the new dedicated mux suite document, generalizes the sandbox
coverage guard from a single hardcoded suite file to a glob over
`tools/sandbox/*SANDBOX-SUITE.md`, extracts scenario S9 from the main suite (leaving a
pointer), and updates the `CONSTRAINTS.md` Sandbox Suite Coverage invariant in the same
batch (doc-lifecycle rule). Card order is load-bearing for keeping the tree green after
each card: the new suite file is created first (both files then tag `mux`, the union is
fine), the guard is generalized second, S9 is removed third, the invariant text fourth.
The external interface batch 2 consumes: `tools/sandbox/MUX-SANDBOX-SUITE.md` exists on
disk (the `//go:embed` target).

## Cards

### Card 1: Create MUX-SANDBOX-SUITE.md

- **Context:**
  - `tools/sandbox/SANDBOX-SUITE.md`
  - `docs/modules/mux.md`
- **Edits:** none
- **Creates:**
  - `tools/sandbox/MUX-SANDBOX-SUITE.md`
- **Deletes:** none
- **Moves:** none
- **Requirements:** Write the dedicated mux suite document. Mirror the structural
  skeleton of `tools/sandbox/SANDBOX-SUITE.md` (same section order and bold-label
  style: `**Goal:**` / `**Covers:**` / `**Watch:**` / `**Verdict:**`), with this
  content:
  - **H1 + What this is:** `# MUX-SANDBOX-SUITE -- lyx mux black-box agent suite`. A
    structured test-loop for exercising `lyx mux` against a **live psmux server** in
    the sandbox Hub host repo. Unlike the host/weft-centric main suite
    (`SANDBOX-SUITE.md`), the value here is partly **visual**: panes popping up,
    layout holding. Not an automated suite — an agent drives it, an operator watches.
  - **Pre-conditions:** (1) deploy a fresh binary via `deploy.cmd`; (2) Hub built via
    `sandbox-build.cmd` (the session cwd is the Hub host repo root, same operating
    model as the main suite); (3) **live-psmux requirement**: `psmux.exe` on PATH
    (installed at `C:\Code\tools\bin\psmux.exe`) and PowerShell 7 present. State the
    caveat verbatim in spirit: if psmux or pwsh is unavailable in the session,
    **note that as the session outcome rather than treating it as a mux defect**; the
    `**Covers:** mux` tag satisfies the coverage guard regardless of runtime
    availability.
  - **Black-box rule:** same rule as the main suite (drive `lyx.exe` from PATH only;
    no peeking at the lyx source tree), plus a subsection `### Controlled psmux
    exceptions` declaring exactly two sanctioned deviations: (a) direct `psmux -L
    <socket>` verbs (`kill-server`, `list-panes`, `ls`) are allowed **only** for crash
    simulation and layout/stray-state verification, where `<socket>` is taken from
    `lyx mux status` output (its JSON result carries `session`, `socket`, and
    `strands[]` with `guid`/`name`/`paneId`/`live`); (b) scenario M7 (attach) is
    operator-assisted — see M7. Model the wording on S6's controlled-exception note in
    the main suite.
  - **Fingerprint header:** same wording as the main suite — the launcher prepends the
    binary-under-test block when copying this file into the Hub; the agent never
    transcribes it.
  - **How to run a scenario / Verdict key:** copy the main suite's model (`OK` /
    `WARN` / `FAIL`, goal-not-commands ethos).
  - **Capturing findings:** same JSON schema and file as the main suite —
    `./sandbox-report.json` in the host-repo cwd, `source: "sandbox-report"`, `items`
    only for WARN/FAIL, **always write the file** even with zero findings; `ref` is
    the scenario id (`M0`–`M11`).
  - **Scenarios** (each with Goal/Watch/Verdict; `**Covers:** mux` appears exactly
    once, on M2):
    - **M0 — Discovery:** the `lyx mux` help surface — does `lyx mux` list all
      subcommands (`up`, `add`, `remove`, `status`, `attach`, `resume`, `down`) with
      accurate `Short`s, and does each `--help` explain itself?
    - **M1 — Pre-up ergonomics:** from a fresh state (no session), `lyx mux add --cmd
      ...` and `lyx mux remove <guid>` must fail with the friendly JSON-envelope error
      `no mux session; run "lyx mux up"` — that message is the `OK` outcome, not a
      finding.
    - **M2 — Up (Covers: mux):** `lyx mux up` boots the named server + this
      worktree's session; a second `up` is a clean no-op (idempotent). It runs no
      strand command (substrate-only).
    - **M3 — Add (visible):** `lyx mux add --cmd <long-running command>` returns an
      assigned `guid` + resolved `name`; the pane exists and the layout applies.
    - **M4 — Status:** `lyx mux status` reports `session`, `socket`, and the strand
      from M3 with `live: true`.
    - **M5 — Add hidden:** `lyx mux add --anchor hidden --cmd ...` creates **no
      pane**; `status` still lists the strand; `lyx mux resume` **skips** it (hidden
      strands are pending, not dead — still no pane after resume). Note explicitly:
      surfacing a hidden strand is engine-API-only in v1 (there is no `lyx mux
      update` verb) — do **not** attempt it, and its absence is not a finding.
    - **M6 — Layout sanity (≥2 top, 0 stack):** with at least two `--anchor top`
      strands and no below-parent strands, the last top band stretches to tile the
      full box (no torn/leftover rows). Verify via `psmux -L <socket> list-panes`
      geometry (controlled exception) or visually at M7.
    - **M7 — Attach (operator-assisted visual):** the agent pauses and instructs the
      operator to run `lyx mux attach` **in a second terminal**, visually confirm the
      pane layout, then detach; the agent records the operator's verdict. Rationale to
      state in the scenario: the agent session owns the current terminal, so it cannot
      demonstrate or observe the takeover itself. Also note: `attach` is a documented
      envelope exception — pre-flight failures come as the JSON envelope; a successful
      handover emits no JSON. Neither behaviour is a finding.
    - **M8 — Resume semantics:** `lyx mux resume` with all strands live leaves them
      untouched (no double launch). Then kill one strand's pane via `psmux -L <socket>
      kill-pane -t <paneId>` (controlled exception; `paneId` from `status`) and run
      `resume` again: the strand's pane is recreated and its stored resume/launch
      command replays.
    - **M9 — Crash-resume:** `psmux -L <socket> kill-server` (controlled exception)
      simulates a server crash; `lyx mux resume` boots a fresh server + session and
      replays every non-hidden strand's command into a new pane.
    - **M10 — Recursive remove:** removing a strand that has children without
      `--recursive` fails with `strand has children, use --recursive`; with
      `--recursive` the removal cascades over the subtree and the result JSON lists
      every removed strand.
    - **M11 — Down without stray state:** `lyx mux down` kills the server and clears
      the worktree's strand state; `psmux -L <socket> ls` (controlled exception)
      confirms no server survives, and a follow-up `lyx mux status` reports the
      friendly no-session error rather than stale strands.
  - **Session log format:** mirror the main suite's block with `M0:`–`M11:` lines and
    the `sandbox-report.json written: <count>` footer.
  - **Notes:** host/weft scenarios stay in `SANDBOX-SUITE.md`; this suite grows with
    mux (windows for clusters, daemon) — add `M` scenarios here, not in the main
    suite.
- **Commit:** `sandbox(mux): add MUX-SANDBOX-SUITE.md dedicated mux suite doc`

### Card 2: Generalize the coverage guard to a suite-file glob

- **Context:**
  - `cmd/lyx/registration_test.go`
  - `tools/sandbox/SANDBOX-SUITE.md`
- **Edits:**
  - `cmd/lyx/sandbox_coverage_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Generalize `parseCoveredModules` from reading the single hardcoded
  `tools/sandbox/SANDBOX-SUITE.md` to scanning every file matching
  `filepath.Glob(filepath.Join(repoRoot, "tools", "sandbox", "*SANDBOX-SUITE.md"))`:
  - Change the return type to `map[string][]string` — module token → sorted list of
    suite-file basenames that declare it — so Assert-2's stale-tag error can name the
    offending file(s). Keep the repo-root resolution via `runtime.Caller(0)` and the
    three-`filepath.Dir` walk-up unchanged.
  - Add a vacuous-glob guard: `t.Fatalf` when the glob matches fewer than **2** files,
    with a message explaining that the repo ships (at least) `SANDBOX-SUITE.md` and
    `MUX-SANDBOX-SUITE.md` and a shorter match means the pattern or directory is
    wrong. This is a committed decision, not optional.
  - Update `TestSandboxCoverage_AllModulesCoveredOrExcluded` for the new map type:
    membership checks become `len(covered[m]) > 0` or `_, ok :=` lookups; Assert-1's
    error message references the glob pattern `tools/sandbox/*SANDBOX-SUITE.md`
    instead of the single file; Assert-2's stale-tag error names the specific file(s)
    the stale token came from (`covered[m]` values).
  - Update every remaining single-file reference: the file-header doc comment (lines
    1–5), the `coversLinePattern` doc comment, the test's doc comment, the
    `parseCoveredModules` doc comment, the `discovered_non_empty` sub-test's error
    string, and the `t.Fatalf` in the read path (now per-file inside the glob loop).
    The comment noting S0/S1/S5 carry no Covers line generalizes to "scenarios without
    a Covers line are skipped" (do not enumerate scenario ids from one suite).
  - Keep both sub-assertions and the `excludedModules` allowlist semantics unchanged.
- **Commit:** `test(lyx): generalize sandbox coverage guard to multi-suite glob`

### Card 3: Move S9 out of the main suite

- **Context:**
  - `docs/modules/mux.md`
- **Edits:**
  - `tools/sandbox/SANDBOX-SUITE.md`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Remove the whole `### S9 -- Mux lifecycle` section (including its
  `**Covers:** mux` tag and live-psmux note) from `tools/sandbox/SANDBOX-SUITE.md`.
  In its place is no scenario — instead:
  - Add a one-paragraph pointer at the end of the `## Scenarios` section (after S8):
    mux has its own dedicated suite, `MUX-SANDBOX-SUITE.md` in the same directory,
    launched via `mux-sandbox-suite.cmd` — mux needs a live psmux server and visual
    verification, a different test mode from this suite.
  - Remove the `S9: <OK|WARN|FAIL> ...` line from `## Session log format`.
  - Confirm the `## Capturing findings` sentence "`ref` is the scenario id
    (`S0`-`S8`)" is accurate again post-removal (it is; leave as-is).
  - Update the first `## Notes` bullet ("Scenario set is deliberately small and
    host/weft-centric...") to also state that a module whose testing model is
    fundamentally different gets its own sibling suite file (`*SANDBOX-SUITE.md`),
    with mux as the precedent; the coverage guard scans all of them.
- **Commit:** `sandbox: move S9 mux lifecycle out of the main suite`

### Card 4: Update the Sandbox Suite Coverage invariant

- **Context:**
  - `cmd/lyx/sandbox_coverage_test.go`
- **Edits:**
  - `CONSTRAINTS.md`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Rewrite the `## Sandbox Suite Coverage` section of
  `CONSTRAINTS.md` for the multi-suite model:
  - **Tagging** bullet: a scenario in **any** suite file matching
    `tools/sandbox/*SANDBOX-SUITE.md` (today: `SANDBOX-SUITE.md`,
    `MUX-SANDBOX-SUITE.md`) declares coverage via the `**Covers:**` line; the guard
    unions tags across all matched files.
  - Add one sentence noting the guard fails fast if the glob matches fewer than two
    files (vacuous-glob protection).
  - **Allowlist** and **Exists ⇒ covered or excluded** bullets: unchanged semantics;
    reword any single-file phrasing.
  - **Enforced by** bullet: unchanged
    (`cmd/lyx/sandbox_coverage_test.go`, `TestSandboxCoverage_AllModulesCoveredOrExcluded`).
- **Commit:** `docs(constraints): multi-suite sandbox coverage invariant`

## Batch Tests

`verify: go test ./cmd/lyx/` runs the generalized coverage guard
(`sandbox_coverage_test.go`) plus the package's other pinned-set guards
(`drift_test.go`, `helptree_test.go`, `registration_test.go`, `longlist_test.go`),
which together prove: `mux` is covered via the new suite file, the union is non-empty,
the glob matches ≥2 files, and no stale tags exist in either suite. The scope is
correct because this batch's Go surface is exactly `cmd/lyx`'s guard; the `tools/`
markdown files have no other runnable surface.
