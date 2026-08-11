# Plan: batcher: split out of webster into a standalone configreg module with its own batcher.yaml

```yaml
task: 'batcher: split out of webster into a standalone configreg module with its own batcher.yaml'
slug: 'batcher-standalone-split'
approved: false
started: '20260811-044213'
parent: 'main'
root: ""
verify: go build ./...
```

## Batch Index

_The fenced yaml block below is the authoritative DAG mill-go reads to schedule batches.
Every batch lives at `NN-<batch-slug>.md` in this directory and is mirrored as one entry here._

```yaml
batches:
  - number: 1
    name: batcher-config-module
    file: 01-batcher-config-module.md
    depends-on: []
    verify: go build ./... && go test ./internal/batcher/... ./internal/configreg/... && go test -run 'TestTierPurity_UntaggedTestsSpawnNothing|TestHermeticGitEnv_GitSpawningPackagesHaveTestMain|TestSandboxCoverage_AllModulesCoveredOrExcluded' ./cmd/lyx/...
  - number: 2
    name: call-site-migration
    file: 02-call-site-migration.md
    depends-on: [1]
    verify: go build ./... && go test ./internal/websterengine/... ./internal/webstercli/... && go test -tags integration ./internal/websterengine/... ./internal/webstercli/...
  - number: 3
    name: documentation
    file: 03-documentation.md
    depends-on: [2]
    verify: go build ./... && go test ./internal/websterengine/... ./internal/configcli/...
```

## Shared Decisions

_Cross-cutting decisions every batch inherits: naming conventions, error-handling posture, test frameworks, style/lint constraints.
One subsection per decision.
Batch-local decisions live in each batch file._

### Decision: entry-point-name

- **Decision:** the exported entry point is `Active(baseDir string) (Batcher, error)` and the key inside `batcher.yaml` is `active:`.
  `Select` stays exported and unchanged.
- **Rationale:** "active" is `internal/batcher`'s own existing vocabulary (`registry.go`'s `Select` doc says it "resolves the active batcher by name";
  `webstercli/cli.go`'s field is `activeBatcher`), so `batcher.yaml`'s `active:` key and `batcher.Active()` pair one-to-one.
  `Active` takes only `baseDir` — the module name `"batcher"` is batcher's own constant, not a parameter, unlike `websterengine.LoadConfig(baseDir, module)`.
- **Applies to:** all batches

### Decision: default-value

- **Decision:** `internal/batcher/template.yaml`'s shipped content is `active: ""`, not `active: identity`.
- **Rationale:** preserves the existing empty-resolves-to-`DefaultName` semantics `Select` and its tests already pin, so the default path stays exercised by the shipped template rather than bypassed by a literal.
- **Applies to:** all batches

### Decision: file-layout

- **Decision:** `internal/batcher` mirrors the config-module shape every other module already uses — `template.yaml` holding the literal YAML, `template.go` with `//go:embed template.yaml` plus `ConfigTemplate() string`, and `config.go` with an unexported config struct plus exported `Active`.
- **Rationale:** `perchengine`, `reedengine`, `shuttleengine`, and `websterengine` all share the embed-and-accessor shape, and `configreg.Module.Template` is typed `func() string` and expects exactly it.
  Folding the loading into `registry.go` was rejected: it breaks the shared shape and mixes registry mechanics with config I/O.
- **Applies to:** batcher-config-module

### Decision: two-distinct-error-paths

- **Decision:** `Active` surfaces two different messages and they must not be conflated.
  When `<baseDir>/_lyx` does not exist, `configengine.Load` returns an error containing `not initialized` and `Active` rewrites it to exactly `not initialized here; run "lyx fabric reconcile"`, matching `websterengine.LoadConfig`'s own `strings.Contains(err.Error(), "not initialized")` rewrite.
  When `_lyx/` exists but `_lyx/config/batcher.yaml` does not, `configengine.Load` returns `config file <path> not found; run "lyx config reconcile"` and `Active` passes it through unchanged.
- **Rationale:** `fabric reconcile` wires a worktree;
  `config reconcile` materializes a module's file.
  The pass-through message is what the reconcile-required-for-pre-registry-wefts decision depends on, and it already names its own fixing command.
  There is no fallback to `DefaultName` on either path — a module that silently tolerates a missing config is the one module whose config never gets created.
- **Applies to:** batcher-config-module

### Decision: runlevel-call-site

- **Decision:** `websterengine.RunDeps` gains a `Batcher batcher.Batcher` field populated by `webstercli/run.go` from the already-resolved `c.batcher`.
  `runlevel.go`'s `batcher.Select(deps.Config.Batcher)` block is deleted outright and the engine never loads `batcher.yaml`.
- **Rationale:** this matches the existing layering — `RunDeps` already carries `Config` and `Roles` pre-resolved by the CLI, and `websterengine.Run` does no config I/O today.
  The alternative (`Run` calling `batcher.Active(deps.WorktreeRoot)` itself) would make `Run` the first engine entry point requiring a materialized `_lyx/` tree on `WorktreeRoot`, a requirement every future `Run` test would inherit forever.
  Both options cost the same one-line fixture edit;
  the standing consequence is what separates them.
- **Applies to:** call-site-migration

### Decision: nil-batcher-contract

- **Decision:** `Run` guards `deps.Batcher == nil` and returns the sentinel `ErrNilBatcher = errors.New("webster: RunDeps.Batcher not populated")`, declared in `runlevel.go` beside `ErrRunBusy`.
  The guard takes the deleted `Select` block's position, immediately before the `deps.Batcher.Batch(plan.Cards)` call — not beside the zero-batch refusal, which sits *after* that call and would therefore run after the very call that panics.
- **Rationale:** today `RunDeps.Config.Batcher`'s zero value is `""`, which `Select` safely resolves to `identity`, so a caller that forgot the field still worked.
  A zero-value `batcher.Batcher` is a nil interface and `deps.Batcher.Batch(...)` panics.
  The name and message text are fixed here because a dedicated test asserts them;
  the shape follows webster's existing sentinels exactly (package-level `var Err… = errors.New("webster: …")` with a lowercase message).
- **Applies to:** call-site-migration

### Decision: no-migration-path

- **Decision:** this task ships no migration handling for a pre-split worktree — no orphan-key reporting code, no migration test, no strict unknown-key validation in `configengine`.
- **Rationale:** user override recorded in the discussion.
  Once `batcher:` leaves `internal/websterengine/template.yaml`, `yamlengine.Reconcile` already reports it as a `Removed` key for module `webster` and prunes it on `lyx config reconcile --apply`;
  `configengine.Load` validates only *missing* keys, and `yaml.Unmarshal` into a `Config` with no `Batcher` field silently ignores an extra one.
  A leftover key is therefore already reported-and-never-honoured by existing generic machinery.
- **Applies to:** all batches

### Decision: test-seeding-mechanism

- **Decision:** every new `internal/batcher` test seeds config with a plain-filesystem helper — `os.MkdirAll(configengine.ConfigDir(baseDir), 0o755)` then `os.WriteFile(configengine.ConfigFile(baseDir, "batcher"), …)` over a `t.TempDir()` base — copying `internal/websterengine/config_test.go`'s own `seedConfig`.
  `lyxtest.SeedConfig` is banned here.
  `internal/batcher` gains no `TestMain` and no build tag.
- **Rationale:** `lyxtest.SeedConfig` spawns git, which drags `internal/batcher` under the Hermetic Git Test Environment Invariant (a `TestMain` calling `lyxtest.HermeticGitEnv()`) and out of Tier 1, contradicting the "Tier-1 (pure logic, no git, no TestMain)" header all three existing test files carry.
  `configengine.FindBaseDir` checks only that `<baseDir>/_lyx` exists — no git repository is involved anywhere in `Active`'s path.
- **Applies to:** batcher-config-module

### Decision: seed-only-omitted

- **Decision:** the `configreg` entry is `{Name: "batcher", Template: batcher.ConfigTemplate}` with `SeedOnly` omitted, i.e. `false`.
- **Rationale:** batcher's key set is closed and template-owned (one `active:` key), so reconcile manages the file normally rather than seeding it once and never touching it again — unlike `models` (operator-owned aliases) and `burler` (lenses/fans).
  `configreg_test.go`'s `TestModules_SeedOnly` computes its `want` as `m.Name == "models" || m.Name == "burler"`, which is already `false` for `batcher`, so that test needs no edit.
- **Applies to:** batcher-config-module

### Decision: doc-site-ownership

- **Decision:** this task owns every site whose claim it itself falsifies.
  Batch 3's four cards carry the corrections that are only writable once the code has landed;
  two further corrections live inline in batch 2 card 5 (`websterengine/config.go`'s `Config` type doc and `websterengine/template.go`'s `ConfigTemplate` doc) because the clauses they fix are falsified by that card's own edits and would otherwise leave the tree self-contradicting across the whole of batch 2.
  No site count is asserted here — several files carry more than one falsified clause, so a file count and a clause count disagree, and the per-card `Requirements:` are self-contained.
- **Rationale:** the enumeration method, not a number, is what makes the completeness claim checkable: grep `batcher.Select`, `batcher:`, and `batcher.yaml` across *all* production Go and markdown, not a `doc.go`-and-`.md`-only sweep, then read the surrounding paragraph of every hit for the two recurring claims (webster ownership; the `webster.yaml` key pin).
  An earlier discussion draft enumerated only package docs and reference docs and therefore missed three in-code file-header/struct-doc comments.
  Two sites were found by applying the stated rule during planning and plan review: `internal/configcli/configcli_test.go`'s "The other nine modules are absent" comment (registering `batcher` makes it ten), and `internal/websterengine/template.go`'s `ConfigTemplate` doc, which says "batchifier selection" and so is reachable only by reading around the grep hits, never by the grep itself.
- **Applies to:** documentation, call-site-migration

### Decision: no-cobra-command-no-sandbox-tag

- **Decision:** no `lyx batcher` cobra command and no `**Covers:** batcher` sandbox tag.
- **Rationale:** "standalone" means configreg-registered, not user-facing — a batchifier has no verb to invoke.
  The CLI/Cobra Invariant does not apply because nothing is registered on the cobra root.
  The Sandbox Suite Coverage Invariant must *not* be satisfied: `cmd/lyx/sandbox_coverage_test.go` enumerates `newRoot().Commands()`, i.e. cobra registration rather than `configreg`, so adding a `**Covers:** batcher` tag would fail that test's drift assert.
- **Applies to:** all batches

### Decision: out-of-scope-files

- **Decision:** `manifest/designs/loom.md` row 8 and `manifest/roadmap.md` are not edited by this task.
- **Rationale:** `loom.md:56` names `webster.yaml`'s `batcher:` key and is falsified by this task, but writing that row is explicitly task E's per `manifest/designs/shed-followups.md`'s Scope section, and task E is sequenced after this one precisely so it can reflect what this task lands.
  `manifest/roadmap.md` moves only on completing or adding a planned item, and this task is one of six already-recorded follow-ups.
- **Applies to:** all batches

## All Files Touched

_Full union of every `Creates:` / `Edits:` / `Moves:` **target** path across every batch, sorted alphabetically (Move **source** paths are excluded — they disappear, like `Deletes:` tokens).
Cards are the source of truth;
this section is the input `_plan_validate.py`'s `all-files-touched-mismatch` check cross-references against the derived union of every card's `Edits:`/`Creates:`/Move-target paths, to catch drift between the hand/agent-maintained list here and that derived union._

- `CONSTRAINTS.md`
- `docs/overview.md`
- `docs/reference/plan-format.md`
- `docs/reference/webster-contract.md`
- `internal/batcher/config.go`
- `internal/batcher/config_test.go`
- `internal/batcher/doc.go`
- `internal/batcher/registry.go`
- `internal/batcher/template.go`
- `internal/batcher/template.yaml`
- `internal/configcli/configcli_test.go`
- `internal/configreg/configreg.go`
- `internal/configreg/configreg_test.go`
- `internal/planparser/doc.go`
- `internal/webstercli/cli.go`
- `internal/webstercli/run.go`
- `internal/webstercli/verbs_test.go`
- `internal/websterengine/beginbatch.go`
- `internal/websterengine/config.go`
- `internal/websterengine/config_test.go`
- `internal/websterengine/doc.go`
- `internal/websterengine/master-template.md`
- `internal/websterengine/recordbatch.go`
- `internal/websterengine/runlevel.go`
- `internal/websterengine/runlevel_test.go`
- `internal/websterengine/template.go`
- `internal/websterengine/template.yaml`
- `tools/sandbox/SANDBOX-WEBSTER-SUITE.md`
