# Batch: mux-suite-launcher

```yaml
task: Dedicated sandbox suite for mux
batch: mux-suite-launcher
number: 2
cards: 3
verify: go test ./tools/sandbox/
depends-on: [1]
```

## Batch Scope

This batch delivers full launcher parity for the mux suite: `runSuite` in
`tools/sandbox` is parameterized over a suite spec, a `mux-suite` subcommand is added
to the dispatch, the new doc is embedded, tests cover the new path while proving the
existing `suite` behaviour unchanged, and a `mux-sandbox-suite.cmd` wrapper lands at
the repo root. Depends on batch 1 because the `//go:embed MUX-SANDBOX-SUITE.md`
directive needs the file on disk. The external interface batch 3 documents:
`mux-sandbox-suite.cmd` at the repo root and the `mux-suite` subcommand.

## Cards

### Card 5: Parameterize runSuite and add the mux-suite subcommand

- **Context:**
  - `tools/sandbox/report.go`
- **Edits:**
  - `tools/sandbox/suite.go`
  - `tools/sandbox/main.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** In `tools/sandbox/suite.go`:
  - Introduce an unexported `suiteSpec` struct with three string fields: `fileName`
    (the file written into the Hub host repo), `doc` (the embedded suite body), and
    `instruction` (the default prompt handed to claude).
  - Add `//go:embed MUX-SANDBOX-SUITE.md` into a new `var muxSandboxSuiteMD string`,
    next to the existing `sandboxSuiteMD` embed.
  - Define two package-level spec values: `mainSuite = suiteSpec{fileName:
    "SANDBOX-SUITE.md", doc: sandboxSuiteMD, instruction: "Read ./SANDBOX-SUITE.md
    and follow the instructions in it exactly."}` and `muxSuite = suiteSpec{fileName:
    "MUX-SANDBOX-SUITE.md", doc: muxSandboxSuiteMD, instruction: "Read
    ./MUX-SANDBOX-SUITE.md and follow the instructions in it exactly."}`. The
    `suiteFileName` and `defaultInstruction` consts are absorbed into `mainSuite`
    (remove them or keep them as the spec's initializers — prefer removal; update all
    references).
  - Change `runSuite(parentDir, claudeOverride, promptOverride string)` to
    `runSuite(parentDir, claudeOverride, promptOverride string, spec suiteSpec)`:
    write `spec.fileName` with `renderScheme(info, spec.doc)` (change `renderScheme`
    to take the doc body as a second parameter instead of reading the package var),
    git-exclude `spec.fileName`, and fall back to `spec.instruction` when
    `promptOverride` is empty. The fingerprint, stale-`reportFileName` delete, report
    git-exclude, claude resolution, and `launchAgent` tail are unchanged and shared.
  - Update the file-header comment to describe the two suites over one mechanic.

  In `tools/sandbox/main.go`:
  - Extend the dispatch with `case "mux-suite":` mirroring the `suite` case exactly
    (same `-claude`/`-prompt` flagset, parsed from `fs.Args()[1:]`), calling
    `runSuite(absParent, *claudeFlag, *promptFlag, muxSuite)`; the existing `suite`
    case passes `mainSuite`.
  - Update the file-header comment (three subcommands → four: build, suite,
    mux-suite, fetch).
- **Commit:** `sandbox: add mux-suite subcommand via parameterized runSuite`

### Card 6: Test the mux-suite path

- **Context:**
  - `tools/sandbox/suite.go`
  - `tools/sandbox/main.go`
  - `tools/sandbox/report.go`
- **Edits:**
  - `tools/sandbox/suite_test.go`
  - `tools/sandbox/main_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Extend the existing table/seam-based tests (reuse the `lookPath`
  and `launchAgent` seams and the existing host-repo fixture helpers):
  - `suite_test.go`: existing tests are updated mechanically for Card 5's signature
    and const changes: `runSuite` call sites gain the `mainSuite` spec argument;
    `TestRenderScheme_ContainsHeaderAndBody`'s `renderScheme(info)` call becomes
    `renderScheme(info, sandboxSuiteMD)`; and the existing references to the removed
    consts switch to the spec fields — `defaultInstruction` → `mainSuite.instruction`,
    `suiteFileName` → `mainSuite.fileName`. Beyond these mechanical retargets the
    assertions must remain unchanged (this is the refactor-is-behaviour-preserving
    proof). Add mux-path
    coverage asserting `runSuite(..., muxSuite)`: writes `MUX-SANDBOX-SUITE.md` (not
    `SANDBOX-SUITE.md`) into the host repo with the fingerprint header prepended;
    appends `MUX-SANDBOX-SUITE.md` and `sandbox-report.json` to `.git/info/exclude`;
    deletes a pre-seeded stale `sandbox-report.json`; and passes the mux default
    instruction (`Read ./MUX-SANDBOX-SUITE.md and follow the instructions in it
    exactly.`) to the `launchAgent` seam when no `-prompt` override is given, and the
    override verbatim when one is.
  - `main_test.go`: add dispatch coverage for the `mux-suite` subcommand following the
    existing `suite` dispatch tests' pattern (flag parsing of `-claude`/`-prompt`
    after the subcommand token, error propagation, exit codes).
- **Commit:** `test(sandbox): cover mux-suite subcommand`

### Card 7: Add the mux-sandbox-suite.cmd launcher

- **Context:**
  - `sandbox-suite.cmd`
- **Edits:** none
- **Creates:**
  - `mux-sandbox-suite.cmd`
- **Deletes:** none
- **Moves:** none
- **Requirements:** Create `mux-sandbox-suite.cmd` at the repo root as an exact
  structural mirror of `sandbox-suite.cmd` (same `@echo off` / REM header style /
  `pushd "%~dp0"` / exit-code capture / `popd` pattern), invoking
  `go run ./tools/sandbox -parent C:\Code mux-suite %*` so `-claude`/`-prompt`
  overrides pass through. Adjust the REM comment to say it launches the interactive
  mux black-box agent session (live psmux required).
- **Commit:** `sandbox: add mux-sandbox-suite.cmd launcher`

## Batch Tests

`verify: go test ./tools/sandbox/` runs `suite_test.go`, `main_test.go`, and
`report_test.go` — covering the parameterized `runSuite` for both specs, the new
dispatch case, and proving the shared fetch/report path is untouched. Scope is correct:
this batch's Go surface is exactly the `tools/sandbox` package; the `.cmd` wrapper has
no runnable test surface (it is a two-line trampoline mirroring an existing, proven
wrapper).
