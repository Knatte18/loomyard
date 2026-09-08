# Plan: Unify webster/burler CLI wiring into a shared module

```yaml
task: "Unify webster/burler CLI wiring into a shared module"
slug: "unify-webster-burler-wiring"
approved: false
skip_checks: ["verify-full-suite"]
started: "20260908-131955"
parent: "crucible-loom-glyph-hardening"
root: ""
verify: null
```

## Batch Index

_The fenced yaml block below is the authoritative DAG mill-go reads to schedule batches.
Every batch lives at `NN-<batch-slug>.md` in this directory and is mirrored as one entry here._

```yaml
batches:
  - number: 1
    name: cliwire-package
    file: 01-cliwire-package.md
    depends-on: []
    verify: go test ./... && go test -tags integration ./internal/cliwire/... ./internal/webstercli/... ./internal/burlercli/... ./internal/standalonegeom/...
  - number: 2
    name: rewire-clis
    file: 02-rewire-clis.md
    depends-on: [1]
    verify: go test ./... && go test -tags integration ./internal/cliwire/... ./internal/webstercli/... ./internal/burlercli/... ./internal/standalonegeom/...
  - number: 3
    name: enforcement-and-docs
    file: 03-enforcement-and-docs.md
    depends-on: [2]
    verify: go test ./... && go test -tags integration ./internal/cliwire/... ./internal/webstercli/... ./internal/burlercli/... ./internal/standalonegeom/...
```

## Shared Decisions

### Decision: no behaviour change, proven by moved assertions

- **Decision:** every operator-facing refusal message, every resolution rule and every mode decision keeps its current observable behaviour byte-for-byte.
  The existing tests' message assertions move into `internal/cliwire`'s own test file unchanged in substance, so a reworded message fails the suite.
  No card may "improve" a message, reorder a check, or relax a guard.
- **Rationale:** all six crucible round-6 findings (R6-7, R6-8, R6-9, R6-15, R6-16, R6-17) are already fixed on this branch's HEAD.
  This task moves those fixes into one place;
  it does not re-apply them and does not add new ones.
- **Applies to:** all batches

### Decision: descriptor data, never branching, carries per-CLI variance

- **Decision:** the two CLIs' differing message fragments live as fields on a `cliwire.Module` value each CLI declares in its own package (`var wireModule = cliwire.Module{...}`).
  No production file under `internal/cliwire` names `webstercli` or `burlercli`.
  `internal/cliwire`'s enforcement tests necessarily name both, exactly as `internal/gitkit/callerset_enforcement_test.go` names `internal/lyxcwd` in a const while production `gitkit` never does.
- **Rationale:** this is the `internal/shedrecipe` split — one implementation, callers varying only in their own data.
  A `cliwire.Webster`/`cliwire.Burler` pair would make the shared module import-aware of its own callers.
- **Applies to:** all batches

### Decision: `wire`, `wireHub` and `wireStandalone` keep their exact signatures

- **Decision:** `(*websterCLI).wire(loc, mode, cwd, stencilsDirFlag, planDirFlag, targetDirFlag) error` and `(*burlerCLI).wire(loc, mode, cwd, stencilsDirFlag, targetDirFlag) error` keep their current signatures, as do `wireHub` and `wireStandalone` in both packages.
  Only their bodies change.
  The mode decision stays inside `wire`.
- **Rationale:** keeps `cli.go`'s `resolvePersistentPreRun` and every existing composition test compiling unchanged, confining the diff to the two `wiring.go` files, their two test files, `internal/burlercli/run.go`, and the new package.
- **Applies to:** batch 2

### Decision: verify is the two-command pair, and the full untagged run is deliberate

- **Decision:** every batch's `verify:` is `go test ./... && go test -tags integration ./internal/cliwire/... ./internal/webstercli/... ./internal/burlercli/... ./internal/standalonegeom/...`.
  The unscoped untagged half is intentional and is recorded as a `verify-full-suite` skip-check justification in each batch's `## Batch Tests`.
- **Rationale:** the untagged half is the cross-cutting half — batch 3's two enforcement tests walk every package under `internal/` and `cmd/`, so a package anywhere in the tree can fail them, and `cmd/lyx/prerunlogging_test.go` also asserts the standalone sink redirect this task relocates.
  The tagged half is not optional: `internal/webstercli/cli_integration_test.go`, `internal/burlercli/cli_integration_test.go` and `internal/standalonegeom/reedgeom_symlink_integration_test.go` carry `//go:build integration`, and the webster one exists precisely to exercise the real `standalonestate.Derive` and the real standalone stencil seed end-to-end — the code this task moves.
  An untagged-only gate would never run the moved prologue's only end-to-end coverage.
- **Applies to:** all batches

### Decision: `pipeline.done_gate` stays as configured

- **Decision:** `mill-config.yaml`'s existing `pipeline.done_gate: go test ./... && go test -tags integration ./...` is left unchanged, and no lint command is added to it.
- **Rationale:** the configured gate is already the repo-wide test command the discussion names for the done gate.
  `golangci-lint` is not installed on this host and the repo carries no `.golangci.yml`, so defaulting the gate to a lint command would make every future task in this hub depend on tooling that is not present.
- **Applies to:** all batches

### Decision: docs land in the task's landing commit, as their own card

- **Decision:** `CONSTRAINTS.md` and `docs/overview.md` are updated in batch 3, card 10 — one card, one commit on the task branch.
  No `manifest/designs/` file is written, and `manifest/roadmap.md` is not touched.
- **Rationale:** CLAUDE.md's "docs land in the same commit" is satisfied at the granularity this task lands at — mill squash-merges the whole task branch onto its parent as one commit — while mill's own one-commit-per-card convention forbids folding two cards' diffs into one commit.
  `docs/overview.md`'s Documentation Lifecycle makes `manifest/designs/<module>.md` a draft for a planned, not-yet-built module that is deleted when the module lands, so writing one in the landing commit would create an immediately-deletable doc;
  the design rationale goes in `internal/cliwire/doc.go` instead.
  Per CLAUDE.md the roadmap moves only for completing or adding a planned item, and this is a consolidation pass.
- **Applies to:** batch 3

### Decision: cliwire's dependency set is fixed, and two exclusions are load-bearing

- **Decision:** `internal/cliwire` production code imports only stdlib plus `internal/standalonestate`, `internal/standalonegeom`, `internal/logger`, `internal/stencilstore`, `internal/buildinfo`, and `contracts/stencils`.
  It never imports `internal/lyxcwd` and never imports `internal/planparser`.
- **Rationale:** `lyxcwd` is barred by the Told-Geometry Invariant and is never needed — `cwd` and `loc` arrive from the caller.
  `planparser` would put webster's plan layout inside a module burler shares;
  the plan-directory default arrives instead as `PlanRules.DefaultPlanDir func(base string) string`, which webster fills with `planparser.PlanDir`.
  `cliwire` never parses a plan, so the Planparser Sole-Parser Invariant is untouched.
- **Applies to:** all batches

### Decision: every moved test stays tier 1 and untagged

- **Decision:** every test written into `internal/cliwire` is untagged, spawns no process, and resolves no cwd.
  Cases that reach `standalonestate.Derive` redirect **both** `XDG_STATE_HOME` and `LOCALAPPDATA` to a `t.TempDir()` before the call, and none of those cases is `t.Parallel()` because `t.Setenv` panics under a parallel test.
  A symlink-dependent case calls `t.Skipf` when `os.Symlink` fails rather than taking an `integration` tag.
- **Rationale:** the Test Tier Purity Invariant, and the existing pattern in both `wiring_test.go` files that these tests are moved from.
- **Applies to:** batches 1 and 2

## All Files Touched

- `CONSTRAINTS.md`
- `docs/overview.md`
- `internal/burlercli/cli_test.go`
- `internal/burlercli/run.go`
- `internal/burlercli/wiring.go`
- `internal/burlercli/wiring_test.go`
- `internal/cliwire/bannedecl_enforcement_test.go`
- `internal/cliwire/callerset_enforcement_test.go`
- `internal/cliwire/cliwire_test.go`
- `internal/cliwire/doc.go`
- `internal/cliwire/module.go`
- `internal/cliwire/paths.go`
- `internal/cliwire/standalone.go`
- `internal/webstercli/wiring.go`
- `internal/webstercli/wiring_test.go`
