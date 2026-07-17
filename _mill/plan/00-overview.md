# Plan: Master Builder: new, parallel fork-based implementation module

```yaml
task: 'Master Builder: new, parallel fork-based implementation module'
slug: 'master-builder'
approved: false
started: '20260717-111247'
parent: 'main'
root: ""
verify: go vet ./...
```

## Batch Index

_The fenced yaml block below is the authoritative DAG mill-go reads to
schedule batches. Every batch lives at `NN-<batch-slug>.md` in this
directory and is mirrored as one entry here._

```yaml
batches:
  - number: 1
    name: seam-extensions
    file: 01-seam-extensions.md
    depends-on: []
    verify: go test ./internal/shuttleengine/... ./internal/builderengine/...
  - number: 2
    name: webster-foundation
    file: 02-webster-foundation.md
    depends-on: [1]
    verify: go test ./internal/websterengine/... ./internal/hubgeometry/... ./internal/configreg/...
  - number: 3
    name: webster-audit-policy
    file: 03-webster-audit-policy.md
    depends-on: [2]
    verify: go test ./internal/websterengine/...
  - number: 4
    name: webster-templates
    file: 04-webster-templates.md
    depends-on: [3]
    verify: go test ./internal/websterengine/...
  - number: 5
    name: bracket-verbs
    file: 05-bracket-verbs.md
    depends-on: [4]
    verify: go test ./internal/websterengine/...
  - number: 6
    name: recover-batch
    file: 06-recover-batch.md
    depends-on: [5]
    verify: go test ./internal/websterengine/...
  - number: 7
    name: run-level
    file: 07-run-level.md
    depends-on: [6]
    verify: go test ./internal/websterengine/...
  - number: 8
    name: webstercli-registration
    file: 08-webstercli-registration.md
    depends-on: [7]
    verify: go test ./internal/webstercli/... ./cmd/lyx/...
  - number: 9
    name: sandbox-and-docs
    file: 09-sandbox-and-docs.md
    depends-on: [8]
    verify: go test ./...
```

## Shared Decisions

### Decision: reuse-by-import-never-copy

- **Decision:** `websterengine` imports `builderengine`'s mechanism-agnostic,
  dir-parameterized functions directly: `ParsePlan`, `Validate`, `Fingerprint`,
  `Distill`/`Digest`, `Classify`/`ClassifyInputs`/`PollUntilTerminal`,
  `TurnEnded`/`StrandLive`, `ParseReport`, `ParseOutcome`/`ArchiveStaleOutcome`,
  `ChainMembers`/`ChainEndFor`, `BatchReportFileName`, the pause helpers
  (`RequestPause`/`PauseRequested`/`ClearPause`/`PauseFlagName` — all take an
  explicit dir), and the gitquery helpers (`HeadSHA`/`ChangedFiles`/`Dirty`/
  `ResetHard`). No shared contract (plan format, batch-report schema, outcome
  schema, digest shape) is ever parsed by two implementations.
- **Rationale:** discussion.md `reuse-by-import`; drift between the two builders
  must be impossible. Import direction `websterengine -> builderengine` only.
- **Applies to:** all batches

### Decision: webster-owns-its-own-domain-types

- **Decision:** webster defines its OWN `Role` consts (`RoleMaster`,
  `RoleMasterOversized`, `RoleRecovery`), its own `Config`, its own
  `State`/`BatchState` (with `Kind`, `Digest`, transcript-attribution fields;
  strand fields populated for recovery batches only), its own `RestartChain`
  variant, and its own sentinel errors (`ErrPaused`, `ErrRunBusy`,
  `ErrFingerprintMismatch`, ... in `websterengine`). It never reuses builder's
  `Role` names, `State` struct, or sentinels — the two modules' state files and
  error identities stay independent.
- **Rationale:** discussion.md `state-schema` (rejected: reusing builder's
  `State` with nil-ed strand fields); `errors.Is` across modules would conflate
  two different runtimes.
- **Applies to:** all batches

### Decision: provider-seam-placement

- **Decision:** Claude-specific grammar stays in
  `internal/shuttleengine/claudeengine`: the `/model <name>` slash-command
  choreography is a new `Engine` seam method returning `[]PaneInput`
  (mirroring `ComposeSend`), and all transcript parsing (including the new
  parent-write/parent-bash facts and incremental fork listing) stays in
  `claudeengine/audit.go`. `internal/shuttleengine` gains only
  provider-invariant types/fields and the generic `Runner.Inject` player.
  `websterengine` consumes the `shuttleengine.Engine` interface and invariant
  fact structs only — it never reads transcripts or emits TUI grammar.
- **Rationale:** Shuttle Provider-Seam Invariant (CONSTRAINTS.md), enforced by
  `internal/shuttleengine/seam_enforcement_test.go`.
- **Applies to:** seam-extensions, webster-audit-policy, bracket-verbs, run-level

### Decision: weft-git-only-in-webstercli

- **Decision:** every weft commit is an in-process
  `weftengine.Commit`+`Push` call inside `webstercli` verbs, using
  `websterWeftPathspec` (scoped `_lyx`, excluding `*.lock`,
  `*/webster/pause`, and `*/webster/prompts/*`). `websterengine` is
  weft-blind. Prompt templates must never instruct any git operation against
  the weft (review obligation, pinned by template property tests).
- **Rationale:** Weft Git Invariant; discussion.md `weft-ownership` (four
  commit points: begin-batch, record-batch, recover-batch spawn+terminal, run
  exit backstop).
- **Applies to:** webstercli-registration, webster-templates

### Decision: fail-loud-archive-never-refuse

- **Decision:** malformed outcome/summary/report files are hard errors, never
  guessed. Stale artifacts (`outcome.yaml`, `summary.md`, a stuck batch report
  before recovery) are archived in place with the timestamp-rename +
  `-1`/`-2` collision discipline (`FirstFreeArchivePath`), never deleted and
  never a refusal on the decided resume/recovery paths.
- **Rationale:** builder's established posture (builder-contract.md), carried
  over verbatim per discussion.md.
- **Applies to:** all batches

### Decision: test-tiers-and-hermetic-git

- **Decision:** untagged `*_test.go` files spawn nothing (no `exec.Command`,
  no `gitexec.RunGit`, no `lyxtest.Copy*`); any test that needs git or fixture
  trees is `//go:build integration`-tagged, and every git-spawning webster test
  package carries a `testmain_test.go` calling `lyxtest.HermeticGitEnv()`
  before `m.Run()`. Engine logic is tested with fakes injected through the
  deps structs (fake `Starter`, fake `shuttleengine.Engine`, fake
  `shuttleengine.MuxOps`, fake injector, injectable clock/sleeper).
- **Rationale:** Test Tier Purity Invariant + Hermetic Git Test Environment
  Invariant (both machine-enforced by `cmd/lyx` guards).
- **Applies to:** all batches

### Decision: template-parser-co-versioning

- **Decision:** webster's templates and the Go code parsing/rendering their
  contracts move in the same batch: the fork template's report-schema section
  is pinned against `builderengine.ParseReport`'s field set, the master
  template's digest-field list against `builderengine.Digest`, and the
  outcome/summary schemas against `ParseOutcome`/`ParseSummary` — all by
  property tests in the same style as `builderengine/template_test.go`
  (literal-statement and exact-field-list assertions, `stencil.Fill`
  round-trips with `missingkey=error`).
- **Rationale:** builder-contract.md's co-versioning rule; a prompt keying off
  a field Go no longer produces raises no compile error.
- **Applies to:** webster-templates, bracket-verbs, run-level

### Decision: cli-shape

- **Decision:** `webstercli` mirrors `buildercli` exactly: cobra parent
  `webster` with `RunE = clihelp.GroupRunE`, verb-per-file subcommands, all
  results/errors through the `internal/output` JSON envelope, a
  `PersistentPreRunE` that resolves cwd → layout → configs → registry → roles
  → mux → claudeengine → `shuttleengine.NewRunner` and stores everything
  (including resolved `_lyx/webster` dirs anchored at `layout.Cwd`, never
  `WorktreeRoot`) on the `*websterCLI` receiver; tests inject fakes by
  populating receiver fields directly, bypassing `PersistentPreRunE`.
  Non-empty `Short` on every command.
- **Rationale:** CLI/Cobra Invariant; buildercli is the proven shape
  (`internal/buildercli/cli.go:134-228`).
- **Applies to:** webstercli-registration

### Decision: commit-message-style

- **Decision:** implementer card commits use the repo's `<scope>: <summary>`
  style with these scopes: `shuttle:` (seam extensions), `builder:` (export
  renames), `webster:` (all webster code), `lyx:` (cmd/lyx registration),
  `sandbox:` (suite files), `docs:` (documentation/rename sweep).
- **Rationale:** matches recent history (`spawn:`, `docs:`, `builder:`
  prefixes in `git log`).
- **Applies to:** all batches

## All Files Touched

- `cmd/lyx/helptree_test.go`
- `cmd/lyx/main.go`
- `docs/long-term-ideas.md`
- `docs/modules/builder-contract.md`
- `docs/overview.md`
- `docs/roadmap.md`
- `internal/builderengine/runlevel.go`
- `internal/builderengine/spawn.go`
- `internal/configreg/configreg.go`
- `internal/hubgeometry/hubgeometry.go`
- `internal/hubgeometry/webstergeom_test.go`
- `internal/shuttleengine/claudeengine/audit.go`
- `internal/shuttleengine/claudeengine/audit_incremental_test.go`
- `internal/shuttleengine/claudeengine/audit_parentfacts_test.go`
- `internal/shuttleengine/claudeengine/modelswitch_test.go`
- `internal/shuttleengine/claudeengine/startup.go`
- `internal/shuttleengine/engine.go`
- `internal/shuttleengine/fakes_test.go`
- `internal/shuttleengine/forkaudit.go`
- `internal/shuttleengine/run.go`
- `internal/shuttleengine/run_inject_test.go`
- `internal/webstercli/beginbatch.go`
- `internal/webstercli/cli.go`
- `internal/webstercli/cli_test.go`
- `internal/webstercli/pause.go`
- `internal/webstercli/recordbatch.go`
- `internal/webstercli/recoverbatch.go`
- `internal/webstercli/run.go`
- `internal/webstercli/status.go`
- `internal/webstercli/testmain_test.go`
- `internal/webstercli/validate.go`
- `internal/webstercli/verbs_test.go`
- `internal/webstercli/weft.go`
- `internal/websterengine/audit.go`
- `internal/websterengine/audit_test.go`
- `internal/websterengine/beginbatch.go`
- `internal/websterengine/beginbatch_test.go`
- `internal/websterengine/chain.go`
- `internal/websterengine/chain_test.go`
- `internal/websterengine/config.go`
- `internal/websterengine/config_test.go`
- `internal/websterengine/doc.go`
- `internal/websterengine/fork-template.md`
- `internal/websterengine/master-template.md`
- `internal/websterengine/recordbatch.go`
- `internal/websterengine/recordbatch_test.go`
- `internal/websterengine/recoverbatch.go`
- `internal/websterengine/recoverbatch_test.go`
- `internal/websterengine/render.go`
- `internal/websterengine/roles.go`
- `internal/websterengine/runlevel.go`
- `internal/websterengine/runlevel_test.go`
- `internal/websterengine/state.go`
- `internal/websterengine/state_test.go`
- `internal/websterengine/summary.go`
- `internal/websterengine/summary_test.go`
- `internal/websterengine/template.go`
- `internal/websterengine/testmain_test.go`
- `internal/websterengine/template.yaml`
- `internal/websterengine/template_test.go`
- `sandbox-webster-suite.cmd`
- `tools/sandbox/SANDBOX-WEBSTER-SUITE.md`
- `tools/sandbox/main.go`
- `tools/sandbox/suite.go`
