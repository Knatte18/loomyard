# Plan: self-report Tier 1: Go-detected structural anomalies

```yaml
task: 'self-report Tier 1: Go-detected structural anomalies'
slug: 'self-report-tier1'
approved: true
started: '20260912-110828'
parent: 'main'
root: ""
verify: null
discussion_sha: e13b9d3a8fc8ac54ca2fcc7bad730c4f38537b7c
```

## Batch Index

_The fenced yaml block below is the authoritative DAG mill-go reads to schedule batches.
Every batch lives at `NN-<batch-slug>.md` in this directory and is mirrored as one entry here._

```yaml
batches:
  - number: 1
    name: leaf-surfaces
    file: 01-leaf-surfaces.md
    depends-on: []
    verify: go test ./internal/selfreportengine/ ./internal/selfreportcli/ ./internal/shedadapters/ ./internal/loomengine/ && go test ./cmd/lyx/ -run 'TestNoTransientsUnderLyx|TestConstructorAnchoring'
  - number: 2
    name: anomaly-detector
    file: 02-anomaly-detector.md
    depends-on: []
    verify: go test ./internal/loomengine/
  - number: 3
    name: drive-wiring
    file: 03-drive-wiring.md
    depends-on: [1, 2]
    verify: go test ./internal/loomcli/ ./internal/loomengine/ && go test -tags smoke -run 'TestSmokeBootstrap_BringsUpSessionStrandAndDriver|TestSmokeDriveStandalone_AdvancesMachineFromExistingSeed' ./internal/loomcli/ && go test ./internal/lyxcwd/ -run TestEnforcement_MarkdownLinks
```

## Shared Decisions

### Decision: no-new-cross-cutting-invariant

- **Decision:** This task adds no entry to `CONSTRAINTS.md`.
  The two new production import edges (`loomcli` → `selfreportengine`, `loomcli` → `shedadapters`) are not CLI/Cobra Invariant deviations.
- **Rationale:** the invariant's "Package naming" clause governs the `<module>cli` ↔ `<module>engine` naming pair, and both of its listed deviations are cases where a cli's engine-side counterpart is not named `<module>engine` at all.
  Production `loomcli` already imports `burlerengine`, `websterengine`, `reedengine`, `fabricengine`, `landingshed`, `batcher`, `shedrecipe`, `shuttleengine`, and `planparser` with no entry for any of them, and it still imports `loomengine`, so its naming pair stays intact.
  Settled in the discussion's own Q&A log, not handed forward.
- **Applies to:** all batches

### Decision: failure-posture-is-warn-and-continue

- **Decision:** every failure in the detect-and-file path degrades to a `logger.Warn` and never changes `drive`'s outcome, its exit code, or either envelope.
  A failed filing call leaves that anomaly's title out of the marker so the next run retries it;
  a failed marker read is treated as an empty marker;
  a failed marker write is warned and nothing else.
  No function added by this task returns an error to `drive`.
- **Rationale:** this is a diagnostics side-channel on a long autonomous run — a GitHub outage, an unresolvable token, or a rate limit must never turn a completed loom run into a reported failure.
  `Bouncer.runSeedSpawn` sets the same precedent in the adjacent code, and omitting the marker write on failure buys the retry with no retry loop.
- **Applies to:** all batches

### Decision: detector-stays-pure-and-imports-no-shedadapters

- **Decision:** `internal/loomengine`'s detector and body renderer are pure over told inputs — no file reads, no path derivation, no spawns — and `loomengine` gains no import of `internal/shedadapters`.
  Ledger content crosses the package boundary as a `loomengine`-declared value type populated by the caller, never as a read call.
- **Rationale:** `coherence.go` already houses exactly this shape in the same package (a pure, no-I/O, table-tested Tier-1 validator over a decoded `shedengine.Status`), and keeping the data-only boundary is what avoids a `loomengine` → `shedadapters` edge for a task that needs none.
  It is also what keeps every detection, title, and body assertion in a Tier-1 table.
- **Applies to:** all batches

### Decision: no-test-fixture-may-file-a-real-issue

- **Decision:** no test in this repo may reach a live `CreateIssue`.
  A test that can stub `selfreportengine.NewGitHubClient` does so;
  a test that cannot — because it drives the real compiled binary as a subprocess — must set `selfreport: false` in its own loom config fixture instead.
  Today that is exactly one suite, `internal/loomcli`'s `smoke_test.go`, disarmed in card 9.
- **Rationale:** the knob's own template comment already says a run in CI or against a fork must set it false;
  this repo's own smoke suite is the first such run, and nothing else in the plan applied that rule to it.
  The suite deliberately drives runs into the bounce-budget-exhausted halt, which is a trigger, and the warn-and-continue posture means a filed issue would not fail the test — so the failure mode is silent upstream issue spam, not a red build.
  Any future test that boots the real binary inherits this rule.
- **Applies to:** all batches

### Decision: tier-1-only-tests

- **Decision:** every test this task adds is untagged Tier 1.
  No test calls `gitexec.Run`/`RunGit`, `exec.Command`, `gitkit.Copy*`, or `hubforge.NewHub`.
  `t.TempDir()` fixture files and an `httptest` server are both in-scope for Tier 1 and are the only I/O any new test performs.
- **Rationale:** the Test Tier Purity Invariant, and the extracted-function decision exists precisely so no branch needs tmux, git, or a real run to reach.
- **Applies to:** all batches

### Decision: docs-land-with-the-observable-change

- **Decision:** the three documentation edits (`manifest/designs/self-report-tier1.md`, `manifest/roadmap.md`, `docs/overview.md`) land in the same card — and therefore the same commit — as the `drive` call-site wiring that makes the feature observable, rather than in a separate docs card or a separate batch.
- **Rationale:** CLAUDE.md requires docs for a change to observable CLI behavior to land in the same commit as that change.
  The behavior becomes observable at exactly one card: the `drive` wiring.
  A separate docs card would put the docs in a different commit and violate that rule literally.
- **Applies to:** drive-wiring

### Decision: verify-scope-is-per-package

- **Decision:** every batch's `verify:` names the specific packages that batch touches, never a bare `go test ./...`.
  `pipeline.done_gate` (`go test ./... && go test -tags integration ./...`, already configured in `mill-config.yaml`) is what covers the rest of the tree at task end.
- **Rationale:** a verify command runs after every implementer and fixer round.
  The repo-wide gate already exists and runs once.
- **Applies to:** all batches

### Decision: cgo-is-a-build-prerequisite

- **Decision:** every `verify:` command in this plan assumes `CGO_ENABLED=1` and a C compiler on `PATH`.
  No card sets or unsets `CGO_ENABLED`.
- **Rationale:** the Quarry CGO Requirement Invariant — `lyx` links tree-sitter's C grammars, and `CGO_ENABLED` already defaults to `1` for a native build when a compiler is present, so nothing needs setting on an ordinary developer machine.
- **Applies to:** all batches

## All Files Touched

- `cmd/lyx/constructoranchoring_test.go`
- `cmd/lyx/notransients_test.go`
- `docs/overview.md`
- `internal/loomcli/drive.go`
- `internal/loomcli/selfreport.go`
- `internal/loomcli/selfreport_github_test.go`
- `internal/loomcli/selfreport_test.go`
- `internal/loomcli/smoke_test.go`
- `internal/loomengine/anomaly.go`
- `internal/loomengine/anomaly_test.go`
- `internal/loomengine/anomalybody.go`
- `internal/loomengine/anomalybody_test.go`
- `internal/loomengine/config.go`
- `internal/loomengine/config_test.go`
- `internal/loomengine/loomstatus_test.go`
- `internal/loomengine/template.yaml`
- `internal/selfreportcli/cli.go`
- `internal/selfreportengine/selfreport.go`
- `internal/selfreportengine/selfreport_test.go`
- `internal/shedadapters/ledgeraccess.go`
- `internal/shedadapters/ledgeraccess_test.go`
- `manifest/designs/self-report-tier1.md`
- `manifest/roadmap.md`
