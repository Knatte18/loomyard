# Batch: registration-and-suite

```yaml
task: "Build burler - the review+fix round worker"
batch: "registration-and-suite"
number: 4
cards: 3
verify: go build ./... && go vet -tags smoke ./internal/burlerengine/... && go test ./internal/burlercli/... ./cmd/lyx/... ./tools/sandbox/...
depends-on: [3]
```

## Batch Scope

Makes burler operable and provable: the sandbox suite (doc + tool registration + launcher),
the `cmd/lyx` root registration, and the opt-in real-engine smoke test. Suite-before-
registration card order is deliberate: `TestSandboxCoverage_AllModulesCoveredOrExcluded`
fails for any registered module with no `**Covers:**` tag, so the suite file (card 8) must
exist before the registration commit (card 9) for every intermediate commit to keep
`go test ./cmd/lyx/...` green.

## Cards

### Card 8: SANDBOX-BURLER-SUITE — doc, sandbox-tool registration, launcher

- **Context:**
  - `_mill/discussion.md`
  - `tools/sandbox/SANDBOX-SHUTTLE-SUITE.md`
  - `sandbox-shuttle-suite.cmd`
  - `docs/sandbox-howto.md`
- **Edits:**
  - `tools/sandbox/suite.go`
  - `tools/sandbox/main.go`
  - `tools/sandbox/main_test.go`
- **Creates:**
  - `tools/sandbox/SANDBOX-BURLER-SUITE.md`
  - `sandbox-burler-suite.cmd`
- **Deletes:** none
- **Moves:** none
- **Requirements:** Write `SANDBOX-BURLER-SUITE.md` following `SANDBOX-SHUTTLE-SUITE.md`'s
  structure (What this is; Pre-conditions: deploy.cmd, sandbox-build.cmd, live psmux +
  PowerShell 7 + logged-in claude on PATH, `lyx init` for `_lyx/config/shuttle.yaml` +
  `mux.yaml`; Black-box rule — the agent works exclusively inside the Hub host repo and
  discovers the surface via `lyx burler --help` / `lyx burler run --help`; the fingerprint
  header note; How to run a scenario / report conventions matching the shuttle suite).
  Scenarios, each with the shuttle suite's `**Goal:**`/`**Watch:**`/`**Verdict:**` bold-label
  shape: **S1** (tagged `**Covers:** burler`) — the toy round: create a fixture text file in
  the Hub host repo whose chair color mismatches its table color, an inline rubric string
  "the chair's color must match the table's color", and a profile YAML (`fix-scope:
  overlay`, `tool-use: false`, `cluster-n: 0`, fresh `review-path`/`fixer-report-path`);
  run `lyx burler run --profile <file>`; verify the JSON envelope reports `outcome: done`
  and `verdict: BLOCKING` with the review file's frontmatter carrying ≥1 finding, the
  fixture text actually edited so the colors match, and a non-empty fixer-report. **S2** —
  the APPROVED path: same fixture already color-matched; expect `verdict: APPROVED`
  (non-blocking polish permitted; fixer-report still written). **S3** — black-box error
  paths: `cluster-n: 1` → cluster-unsupported error in the JSON envelope; a profile whose
  `fasit` is entirely empty → validation error; re-running with an already-existing
  `review-path` → shuttle's pre-existing-output-file rejection. In `tools/sandbox/suite.go`
  add `//go:embed SANDBOX-BURLER-SUITE.md` into `var burlerSuiteDoc string` and a
  `burlerSuite suiteSpec` (fileName `SANDBOX-BURLER-SUITE.md`, instruction "Read
  ./SANDBOX-BURLER-SUITE.md and follow the instructions in it exactly."), mirroring
  `shuttleSuite`. In `tools/sandbox/main.go` add a `case "burler-suite":` mirroring
  `shuttle-suite` (own flagset, `-claude`/`-prompt`, `runSuite(..., burlerSuite)`) and
  update the file-top doc comment's subcommand enumeration. In `main_test.go` add
  `TestRun_BurlerSuiteRoutesToLaunch` mirroring `TestRun_MuxSuiteRoutesToLaunch` (asserts
  launchAgent fires with `burlerSuite.instruction`). Create root `sandbox-burler-suite.cmd`
  as a copy of `sandbox-shuttle-suite.cmd` with the positional changed to `burler-suite`
  and the comment updated.
- **Commit:** `sandbox: add SANDBOX-BURLER-SUITE (doc, burler-suite subcommand, launcher)`

### Card 9: Register lyx burler at the cobra root

- **Context:**
  - `internal/burlercli/cli.go`
- **Edits:**
  - `cmd/lyx/main.go`
  - `cmd/lyx/helptree_test.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** In `cmd/lyx/main.go`: import
  `github.com/Knatte18/loomyard/internal/burlercli`, append `burlercli.Command()` to the
  `root.AddCommand(...)` call in `newRoot()` (after `shuttlecli.Command()`), and extend
  the root `Long`'s module list line to `Available modules: init, board, config, ide,
  mux, weft, warp, selfreport, shuttle, burler.` In `cmd/lyx/helptree_test.go`: add
  `"burler"` to the pinned module-name list (the slice currently ending with
  `"shuttle"`) and a new table entry `{name: "burler", module: "burler", wantSubs:
  []string{"run"}}`. No other pinned set needs hand-editing: `drift_test.go`,
  `registration_test.go`, and `longlist_test.go` enumerate the live root, and
  `sandbox_coverage_test.go` finds card 8's `**Covers:** burler` tag.
- **Commit:** `lyx: register burler module at the cobra root`

### Card 10: Opt-in smoke test — one real toy round

- **Context:**
  - `_mill/discussion.md`
  - `internal/shuttlecli/smoke_run_test.go`
  - `internal/shuttlecli/smoke_guardrail_test.go`
  - `internal/burlerengine/engine.go`
  - `internal/burlerengine/profile.go`
  - `internal/lyxtest/lyxtest.go`
- **Edits:** none
- **Creates:**
  - `internal/burlerengine/smoke_round_test.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** One `//go:build smoke` test file, external test package
  (`package burlerengine_test`) so it may wire the real engine — the test IS the caller,
  and the Provider-Seam Invariant restricts shuttleengine/muxengine, not test code. Follow
  the `internal/shuttlecli/smoke_*.go` conventions, reproducing (not importing) the shared
  helpers per the smoke-files-are-self-contained convention: claude binary discovery via
  `LYX_MUX_CLAUDE` env then PATH with `t.Skip` when absent; the fixture hub via `lyxtest`
  exactly as `smoke_run_test.go` builds it (including config seeding for shuttle + mux and
  the mux session lifecycle); the pwsh path constant and orphaned-conhost teardown guard;
  poll-with-deadline waits only, never fixed sleeps. `TestSmokeBurlerRoundToyFixture`:
  write a toy target file (~20 words: the chair is red, the table is blue) in the fixture
  worktree; build a `Profile` with `Target` naming that file, `Fasit.Instructions` stating
  the fixture rule ("the chair's color must match the table's color" — the rule IS the
  source of truth here), a short `Rubric` mapping a color mismatch to a BLOCKING finding,
  `FixScope: FixScopeOverlay`, `ToolUse: false`, `ClusterN: 0`, and fresh
  `ReviewPath`/`FixerReportPath` under the fixture; construct the real stack
  (`muxengine.New(muxCfg, layout)`, `claudeengine.New()`,
  `shuttleengine.NewRunner(...)`, `burlerengine.New(runner, layout)`) and call `Run`.
  Assert: `Outcome == shuttleengine.OutcomeDone`; `Verdict == VerdictBlocking` with at
  least one finding; the target file's content changed so the two colors match; the
  fixer-report exists and is non-empty. The assertions are deliberately trivial (the toy
  is unambiguous on purpose — this proves the A→B machinery + file contract + verdict
  parse against a real engine, never review quality). Tear down all mux/psmux state; the
  test must leave zero stray processes per the guardrail helper.
- **Commit:** `burler: add opt-in smoke test driving one real toy round`

## Batch Tests

`verify:` builds everything, compiles the smoke file without running it
(`go vet -tags smoke ./internal/burlerengine/...` — the real-engine test stays opt-in), and
runs the three affected hermetic suites: `./internal/burlercli/...` (unchanged package,
regression), `./cmd/lyx/...` (helptree/registration/longlist/drift pinned sets +
`sandbox_coverage` now seeing the registered module and its `**Covers:** burler` tag), and
`./tools/sandbox/...` (the new `burler-suite` dispatch test).
