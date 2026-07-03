# Plan: Dedicated sandbox suite for mux

```yaml
task: Dedicated sandbox suite for mux
slug: mux-sandbox-suite
approved: true
started: 20260703-081340
parent: internal-mux
root: ""
verify: null
```

## Batch Index

```yaml
batches:
  - number: 1
    name: suite-doc-and-guard
    file: 01-suite-doc-and-guard.md
    depends-on: []
    verify: go test ./cmd/lyx/
  - number: 2
    name: mux-suite-launcher
    file: 02-mux-suite-launcher.md
    depends-on: [1]
    verify: go test ./tools/sandbox/
  - number: 3
    name: docs
    file: 03-docs.md
    depends-on: [1, 2]
    verify: go test ./...
```

## Shared Decisions

### Decision: mirror-the-main-suite

- **Decision:** `MUX-SANDBOX-SUITE.md` mirrors the structural skeleton of
  `SANDBOX-SUITE.md`: What this is / Pre-conditions / Black-box rule / Fingerprint
  header / How to run a scenario / Verdict key / Capturing findings / Scenarios /
  Session log format / Notes. Scenario IDs use an `M` prefix (`M0`, `M1`, …) so report
  refs are unambiguous across suites.
- **Rationale:** The launcher plumbing, report contract, and agent ergonomics are
  shared; only the scenario content differs.
- **Applies to:** all batches

### Decision: shared-report-and-fetch

- **Decision:** The mux suite agent writes findings to the same `./sandbox-report.json`
  (same schema, `source: "sandbox-report"`, `ref` values `M0`…) in the Hub host repo,
  and the existing `fetch` subcommand / `sandbox-fetch.cmd` collects it unchanged.
- **Rationale:** Each suite run deletes the stale report before launching and the
  operator fetches per session, so run→fetch never mixes suites; reuse beats a parallel
  report pipeline. `tools/sandbox/report.go` is not touched by this task.
- **Applies to:** all batches

### Decision: controlled-black-box-exceptions

- **Decision:** The mux suite keeps the main suite's black-box rule (drive `lyx` from
  PATH only, never read the source tree) with two documented, controlled exceptions:
  (a) direct `psmux` verbs (`psmux -L <socket> kill-server`, `psmux -L <socket>
  list-panes`) are allowed for crash simulation and layout verification, where
  `<socket>` comes from `lyx mux status` JSON output (`socket` field); (b) the attach
  scenario is operator-assisted — the agent pauses and asks the operator to run
  `lyx mux attach` in a second terminal and confirm visually.
- **Rationale:** Mirrors the S6 precedent of a scoped, documented exception. The agent
  session owns the operator's terminal (`launchAgent` inherits stdio), so an agent-run
  `attach` cannot demonstrate the visual takeover.
- **Applies to:** suite-doc-and-guard, docs

### Decision: go-verify-shape

- **Decision:** `verify:` commands use the native Go test runner without a
  `PYTHONPATH=` prefix (Go project rule). Batches 1 and 2 scope to the packages they
  touch; batch 3 (docs, last batch) runs the full `go test ./...` as the terminal gate.
- **Rationale:** The lyx test suite is hermetic and fast (`-tags smoke` is opt-in and
  excluded by default), so a full-suite final gate is cheap; scoped verifies keep the
  earlier loops tight.
- **Applies to:** all batches

### Decision: no-roadmap-update

- **Decision:** `docs/roadmap.md` is not touched.
- **Rationale:** This task extracts and hardens delivered work; it does not complete or
  add a planned milestone (CLAUDE.md roadmap rule).
- **Applies to:** all batches

## All Files Touched

- `CONSTRAINTS.md`
- `cmd/lyx/sandbox_coverage_test.go`
- `docs/modules/mux.md`
- `docs/overview.md`
- `docs/sandbox-howto.md`
- `docs/sandbox-hub.md`
- `mux-sandbox-suite.cmd`
- `tools/sandbox/MUX-SANDBOX-SUITE.md`
- `tools/sandbox/SANDBOX-SUITE.md`
- `tools/sandbox/main.go`
- `tools/sandbox/main_test.go`
- `tools/sandbox/suite.go`
- `tools/sandbox/suite_test.go`
