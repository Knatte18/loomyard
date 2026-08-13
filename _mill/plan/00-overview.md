# Plan: Unblock t.Parallel on hub-fixture tests that currently t.Chdir

```yaml
task: Unblock t.Parallel on hub-fixture tests that currently t.Chdir
slug: hubforge-parallel-chdir
approved: false
started: 20260813-143839
parent: main
root: ""
verify: go vet -tags integration ./...
```

## Batch Index

_The fenced yaml block below is the authoritative DAG mill-go reads to schedule batches.
Every batch lives at `NN-<batch-slug>.md` in this directory and is mirrored as one entry here._

```yaml
batches:
  - number: 1
    name: cwd-context seam
    file: 01-cwd-context-seam.md
    depends-on: []
    verify: go test ./internal/lyxcwd/... ./internal/clihelp/... ./cmd/lyx/...
  - number: 2
    name: module seams
    file: 02-module-seams.md
    depends-on: [1]
    verify: go build ./... && go test ./internal/scoutcli/... ./internal/configcli/... ./cmd/lyx/... && go test -tags integration ./internal/loomengine/... ./internal/fabriccli/... ./internal/configcli/... ./internal/perchcli/...
  - number: 3
    name: test migration and guard
    file: 03-test-migration-guard.md
    depends-on: [2]
    verify: go test ./cmd/lyx/... && go test -tags integration ./internal/fabriccli/... ./internal/perchcli/... ./internal/configcli/... ./internal/webstercli/... ./internal/idecli/... ./internal/reedcli/... ./internal/loomengine/...
```

## Shared Decisions

_Cross-cutting decisions every batch inherits: naming conventions, error-handling posture, test frameworks, style/lint constraints.
One subsection per decision.
Batch-local decisions live in each batch file._

### Decision: batch boundaries are the discussion's three commits

- **Decision:** the three batches in the DAG above are exactly the three self-contained commits `_mill/discussion.md`'s `three-commits-each-self-contained` decision names — infrastructure, module seams, test migration.
  Within a batch, mill commits per card;
  a card is authored so that the tree still compiles at that card's own commit wherever that is achievable without splitting an inherently atomic change.
  The "green on its own" property the discussion states is enforced at batch granularity, which is where `verify:` runs.
- **Rationale:** mill's execution unit is the batch and its commit unit is the card, so a literal one-commit-per-discussion-commit plan would be three cards for ~40 files.
  Mapping commit → batch keeps the discussion's isolation property (risky module-seam work separated from mechanical test migration) at the granularity mill actually gates.
- **Applies to:** all batches

### Decision: the signature-pinning assertion is split across batches 1 and 2

- **Decision:** `cmd/lyx/seamsignature_test.go` is created in batch 1 pinning the eleven existing `RunCLI` shapes only.
  The `RunCLIIn` half is appended in batch 2, in the same batch that creates the ten `RunCLIIn` functions.
- **Rationale:** this corrects a defect in `_mill/discussion.md`'s `runcli-gains-a-sibling-rather-than-changing` decision, which assigns the whole assertion — "covering both seam shapes across the 10 modules" — to commit 1.
  A compile-time `[]func(string, io.Writer, []string) int{…}` over `RunCLIIn` cannot compile in commit 1, because no module declares `RunCLIIn` until commit 2.
  Splitting keeps every batch compiling while still closing the hole in the same task, and keeps each half in the same commit as the surface it pins, per CLAUDE.md's docs-land-together rule.
- **Applies to:** cwd-context seam, module seams

### Decision: the `CONSTRAINTS.md` amendment is split the same way

- **Decision:** batch 1 amends the CLI/Cobra Invariant's "Enforced by" line and the Cwd Resolution Invariant for the `lyxcwd`/`clihelp` surface it adds.
  Batch 2 amends the Seam bullet to name `RunCLIIn` and its `cwd == ""` sentinel.
- **Rationale:** CLAUDE.md requires a doc change in the same commit as the change causing it.
  Naming `RunCLIIn` in batch 1 would document an API that does not exist yet.
- **Applies to:** cwd-context seam, module seams

### Decision: the injected cwd is absolute, and `WithCwd` panics otherwise

- **Decision:** `lyxcwd.WithCwd(ctx, dir)` panics when `dir` is empty or `!filepath.IsAbs(dir)`.
  It never normalises via `filepath.Abs`, and never defers the complaint to `CwdFrom`.
  The empty string is a sentinel in `RunCLIIn(cwd, …)` only, meaning "seed nothing, fall back to `Getwd()`".
- **Rationale:** carried verbatim from `_mill/discussion.md`'s `the-injected-cwd-contract`.
  `WithCwd` returns only a `context.Context` and has nowhere to put an error;
  seeding a relative cwd is a programmer error at a call site the programmer controls, so failing loudly at the seeding site puts the diagnostic at the cause. `filepath.Abs` normalisation would resolve against the process cwd, reintroducing the dependency this task removes.
- **Applies to:** all batches

### Decision: `RunCLIIn` branches on the sentinel rather than delegating uniformly

- **Decision:** every `RunCLIIn(cwd string, out io.Writer, args []string) int` reads:
  `cwd == ""` calls `clihelp.Execute(Command(), out, args)`;
  any other value calls `clihelp.ExecuteIn(Command(), cwd, out, args)`.
  `RunCLI(out, args)` is rewritten as `return RunCLIIn("", out, args)`.
- **Rationale:** `WithCwd` panics on an empty dir, so a uniform `RunCLIIn → ExecuteIn` delegation would panic on every existing `RunCLI` call in the repo. `ExecuteIn` itself never receives `""` and needs no empty-string tolerance.
- **Applies to:** module seams

### Decision: the seam governs geometry, and the two argument-resolving verbs are rebased explicitly

- **Decision:** the injected cwd governs every `lyxcwd.Resolve`/`ResolveWorktree` input and everything derived from the resulting `Location`.
  It does not automatically govern a relative path supplied as a flag or positional argument, so the two verbs that take one are rebased explicitly: `lyx fabric clone --into <dir>`, and `scoutcli`'s four `--target-dir` defaulting points plus `parseQuery`/`inFileQuery`.
- **Rationale:** a seam honoured for geometry but ignored for arguments returns a confidently wrong answer instead of an error.
  `scoutcli` is rebased at the flag's defaulting point rather than at each `filepath.Abs` occurrence, because the raw `--target-dir` value leaves the package unresolved and is absolutised outside it, in `internal/scoutengine/ensureserver.go`.
- **Applies to:** module seams

### Decision: `t.Parallel()` is added only to the three files whose sole blocker is chdir

- **Decision:** `internal/reedcli/cli_integration_test.go`, `internal/loomengine/preflight_integration_test.go`, and `internal/perchcli/cli_integration_test.go` gain `t.Parallel()` in each migrated test function.
  Five further integration files get their chdir removed but stay serial, each carrying a comment naming its remaining blocker: the four `t.Setenv` files, plus `internal/idecli/cli_test.go` with its `ideengine.CodeLauncher` package-level swap.
- **Rationale:** `t.Setenv` panics under `t.Parallel()` exactly as `t.Chdir` does, and swapping a production package-level var under parallelism is a data race plus a restore that fires while siblings still run.
  The `WEFT_SKIP_*` config seam and per-invocation `CodeLauncher` injection are both explicitly Out of scope.
- **Applies to:** test migration and guard

### Decision: no repo-wide behaviour change beyond the seam

- **Decision:** `cmd/lyx/main.go` is not touched in any batch. `internal/selfreportcli` gains no `RunCLIIn`. `internal/hubforge`, the `WEFT_SKIP_*` env seam, the twelve `//go:build smoke` files, `internal/fabricengine/coalesce_integration_test.go`, and `manifest/roadmap.md` are all untouched.
- **Rationale:** `RunRootCtx` is a sibling precisely so `main.go` needs no change;
  `selfreportcli` references `lyxcwd` nowhere, so a `RunCLIIn` there would accept a cwd argument nothing reads;
  the rest are the discussion's Out list, each with its own recorded rationale.
- **Applies to:** all batches

### Decision: verification protocol is `-race -count=2` under both tag sets

- **Decision:** batch 3 runs the affected packages with `-race -count=2` under both `-tags integration` and untagged, before and after the migration, and records the wall-clock delta as a row in `docs/benchmarks/test-suite-timing.md`.
- **Rationale:** `-race` covers the parallel-safety of the three newly parallelized files;
  it does not catch a cwd dependence removed incorrectly, because the process working directory is not race-detectable memory.
  Assertion preservation is bought instead by the per-call-site notes carried into each migration card. `-count=2` catches fixture-teardown ordering bugs that only surface on a second run in the same binary.
- **Applies to:** test migration and guard

### Decision: repo prose and vocabulary rules bind every doc edit

- **Decision:** every markdown edit uses semantic line breaks (one sentence per line, break at internal independent-clause boundaries, never a fixed-column hard wrap).
  New prose and the `--into` help text obey the Fabric Vocabulary Invariant: `host` in any fabric sense is banned, and warp/weft are used only where the two sides must be told apart.
  Any new inline markdown link under `manifest/` or `docs/` must resolve, anchor included.
- **Rationale:** `CLAUDE.md`'s markdown rule and `CONSTRAINTS.md`'s Fabric Vocabulary Invariant and Markdown Link Integrity are machine-checked or review-obligated on exactly these edits.
- **Applies to:** all batches

## All Files Touched

_Full union of every `Creates:` / `Edits:` / `Moves:` **target** path across every batch, sorted alphabetically (Move **source** paths are excluded — they disappear, like `Deletes:` tokens).
Cards are the source of truth;
this section is the input `_plan_validate.py`'s `all-files-touched-mismatch` check cross-references against the derived union of every card's `Edits:`/`Creates:`/Move-target paths, to catch drift between the hand/agent-maintained list here and that derived union._

- `CONSTRAINTS.md`
- `cmd/lyx/cwdmutation_test.go`
- `cmd/lyx/seamsignature_test.go`
- `cmd/lyx/tierpurity_test.go`
- `docs/benchmarks/running-tests.md`
- `docs/benchmarks/test-suite-timing.md`
- `docs/overview.md`
- `internal/boardcli/cli.go`
- `internal/burlercli/cli.go`
- `internal/clihelp/exec.go`
- `internal/clihelp/exec_test.go`
- `internal/configcli/configcli.go`
- `internal/configcli/configcli_integration_test.go`
- `internal/fabriccli/cli_test.go`
- `internal/fabriccli/clone.go`
- `internal/fabriccli/fabric.go`
- `internal/fabriccli/unwire.go`
- `internal/fabriccli/weft_verbs.go`
- `internal/idecli/cli.go`
- `internal/idecli/cli_test.go`
- `internal/loomengine/export_test.go`
- `internal/loomengine/preflight.go`
- `internal/loomengine/preflight_integration_test.go`
- `internal/lyxcwd/cwdcontext.go`
- `internal/lyxcwd/cwdcontext_test.go`
- `internal/lyxcwd/lyxcwd.go`
- `internal/perchcli/cli.go`
- `internal/perchcli/cli_integration_test.go`
- `internal/perchcli/run_integration_test.go`
- `internal/reedcli/cli.go`
- `internal/reedcli/cli_integration_test.go`
- `internal/scoutcli/cli.go`
- `internal/scoutcli/cli_test.go`
- `internal/shuttlecli/cli.go`
- `internal/webstercli/cli.go`
- `internal/webstercli/verbs_test.go`
