# Plan: Shed recipe: engine registry

```yaml
task: 'Shed recipe: engine registry'
slug: 'shed-recipe-engine-registry'
approved: true
started: '20260821-083627'
parent: 'main'
root: ""
verify: go vet ./...
```

## Batch Index

_The fenced yaml block below is the authoritative DAG mill-go reads to schedule batches.
Every batch lives at `NN-<batch-slug>.md` in this directory and is mirrored as one entry here._

```yaml
batches:
  - number: 1
    name: loomshed constructor exports
    file: 01-loomshed-exports.md
    depends-on: []
    verify: go test ./internal/loomshed/...
  - number: 2
    name: shedrecipe foundations
    file: 02-shedrecipe-foundations.md
    depends-on: []
    verify: go test ./internal/shedrecipe/...
  - number: 3
    name: registry and value-only entries
    file: 03-registry-and-simple-entries.md
    depends-on: [1, 2]
    verify: go test ./internal/shedrecipe/... ./internal/loomshed/...
  - number: 4
    name: SingleLLM entry
    file: 04-singlellm-entry.md
    depends-on: [3]
    verify: go test ./internal/shedrecipe/...
  - number: 5
    name: Bouncer and BurlerRound entries
    file: 05-review-entries.md
    depends-on: [4]
    verify: go test ./internal/shedrecipe/...
  - number: 6
    name: guards and docs
    file: 06-guards-and-docs.md
    depends-on: [5]
    verify: go test ./internal/shedrecipe/... ./internal/lyxcwd/...
```

## Shared Decisions

_Cross-cutting decisions every batch inherits: naming conventions, error-handling posture, test frameworks, style/lint constraints.
One subsection per decision.
Batch-local decisions live in each batch file._

### Decision: package and file layout

- **Decision:** all new production code lives in the new package `internal/shedrecipe`, one file per concern:
  `doc.go` (package doc), `recipe.go` (`Constructor`, `Config`, `Env`), `config.go` (the typed `Config` accessors plus the unknown-key rejector), `env.go` (the `Env` field validators), `paths.go` (the relative-`Config`-path resolver), `registry.go` (the single `map[string]Constructor` literal plus `Lookup`/`Names`), and one `entries_*.go` file per entry family.
- **Rationale:** the registry table stays greppable in one file while each entry family stays reviewable on its own;
  this mirrors `internal/batcher`'s split of `registry.go` from its members.
- **Applies to:** all batches.

### Decision: error text prefix

- **Decision:** every error this package returns is built with `fmt.Errorf` and starts with the literal prefix `shedrecipe: `, followed by the entry name when the error comes from inside an entry — e.g. `shedrecipe: Bouncer: config key "run_subdir" is required`.
  No `errors.New` sentinel values and no error types are introduced;
  every wrapped underlying error uses `%w`.
- **Rationale:** matches `internal/batcher`'s `batcher: unknown batcher %q` shape and `internal/shedadapters`' `shedadapters: %s (%s): ...` shape;
  no caller in this task or in piece 2 branches on a specific error identity, so a sentinel would be unused surface.
- **Applies to:** all batches.

### Decision: the fixed constructor signature and the two told inputs

- **Decision:** the registry's value type is

  ```go
  type Constructor func(name string, cfg Config, env Env) (shedengine.ShedProducer, error)
  ```

  `Config` is `type Config map[string]any` — the recipe row's static, portable, already-decoded configuration.
  `Env` is the caller-filled bundle of absolute roots and injected seams, defined once in `recipe.go`:

  ```go
  type Env struct {
      Cwd                string
      AnchorPath         string
      WorktreeRoot       string
      StatusPath         string
      StatusLockPath     string
      StencilsDir        string
      RunRoot            string
      DecisionRecordPath string
      SupportLogPath     string

      Shuttle     shedadapters.Shuttle
      Burler      shedadapters.BurlerRunner
      WebsterRun  shedadapters.WebsterRunner
      WebsterDeps websterengine.RunDeps
      Landing     landingshed.Deps
      Now         func() time.Time
  }
  ```

  `Env` carries roots and run-wide values only;
  anything that differs between two rows is a relative path or scalar in `Config`.
- **Rationale:** `manifest/designs/shed-recipe.md` requires the builder to be "registry lookup + fixed-signature constructor call, no reflection", which a uniform signature is what makes true;
  the `error` return is forced by the four already-fallible underlying constructors (`NewBouncer`, `NewBurlerProducer`, `NewPublish`, `NewFinalize`).
- **Applies to:** all batches.

### Decision: `Config` strictness — unknown keys, required keys, empty-as-absent

- **Decision:** every entry ends its extraction with a call to `configRejectUnknown(cfg, known...)`, which errors naming the first unrecognised key in sorted order.
  Every recognised key is explicitly required or optional per the table in each entry's own batch file.
  A required key whose value is an empty string (or, for `artifact_paths`/`output_files`, an empty list) is the same error as an omitted key.
  An absent optional key falls back to the Go zero value.
- **Rationale:** unknown-key strictness matches `CONSTRAINTS.md`'s Config Strictness Invariant posture;
  missing-key strictness exists because an absent `run_subdir` would silently resolve `RunDir` to a bare `Env.RunRoot` and reinstate the cross-segment overwrite that key exists to prevent.
- **Applies to:** batches 2, 3, 4, 5.

### Decision: `Env` validation is per-entry, never global

- **Decision:** each entry validates exactly the `Env` fields it consumes and no others — a path root must be non-empty and `filepath.IsAbs`, an injected seam must be non-nil — erroring with the offending field name.
  `Env.Now` is the single exception: nil is accepted and passed through to the underlying constructor, which defaults it to `time.Now`.
- **Rationale:** `shedadapters.NewSingleLLMProducer` validates nothing, so an under-filled `Env` would otherwise produce a row that constructs cleanly and fails at every `Call`;
  validating all of `Env` up front would instead force every caller to fill fields its recipe never uses.
- **Applies to:** batches 2, 3, 4, 5.

### Decision: relative `Config` paths join a named `Env` root

- **Decision:** a `Config` path value is always relative.
  The shared helper `resolveUnderRoot(root, value string) (string, error)` in `paths.go` rejects an absolute value and one escaping `root` via `..`, and otherwise returns `filepath.Join(root, value)`.
  Roots per key: `artifact_paths` and `output_files` against `Env.WorktreeRoot`, `run_subdir` against `Env.RunRoot`.
  `stencil` and `rubric_stencil` are not paths — they are `stencilstore` names resolved by `stencilstore.Read(env.StencilsDir, name)`.
  The single exception is `BurlerRound`'s `profile.target.paths` and `profile.fasit.paths`, passed through relative and unjoined and not absolute-checked, because `burlerengine.Profile.validate` resolves them against its own told worktree root.
- **Rationale:** `NewBouncer` rejects non-absolute `ArtifactPaths` and `SingleLLMProducer.Call` rejects non-absolute `spec.OutputFiles`, so the join has to happen somewhere;
  the entry is the first layer holding both the recipe's relative value and the caller's told root.
- **Applies to:** batches 2, 4, 5.

### Decision: godoc and comment discipline

- **Decision:** every exported identifier in `internal/shedrecipe` carries a godoc comment starting with its own name.
  Unexported helpers carry a comment too, in this repo's prevailing style.
  Two sites carry an explicit *why* comment because the reasoning is not visible from the code: the `BurlerRound` entry's relative-path exception, and the hand-maintained duplication of `internal/burlercli`'s kebab-case profile key names.
- **Rationale:** matches `mill:golang-comments` and the surrounding packages, which document non-obvious decisions at the site rather than in a design doc alone.
- **Applies to:** all batches.

### Decision: tests use `t.TempDir()` exclusively

- **Decision:** every `Env` a test builds is assembled from `t.TempDir()`-derived absolute paths.
  No test reads or writes a real repository path, and no test writes outside its own temp dir.
  Shared test scaffolding (a filled-`Env` builder, fake `Shuttle`/`BurlerRunner`/`WebsterRunner` implementations) lives in `internal/shedrecipe/fixture_test.go`, mirroring `internal/loomshed/fixture_test.go`.
- **Rationale:** a test accidentally passing a real repo path would mask a told-geometry violation, which is the exact property this package's own seam-enforcement test exists to protect.
- **Applies to:** batches 2, 3, 4, 5, 6.

### Decision: `loomshed`'s exported constructors return the seam interface

- **Decision:** the six renamed constructors keep their parameter lists and bodies unchanged and widen only their declared return type from the unexported concrete type to `shedengine.ShedProducer`.
  The concrete types (`stubProducer`, `batchifier`, `planValidate`, `discussionValidate`, `loomPreflightProducer`, `websterProducer`) stay unexported.
- **Rationale:** the registry must call these from outside `internal/loomshed`, and returning the interface is what keeps the concrete types package-private, which is what they were for.
- **Applies to:** batches 1, 3.

## All Files Touched

_Full union of every `Creates:` / `Edits:` / `Moves:` **target** path across every batch, sorted alphabetically (Move **source** paths are excluded — they disappear, like `Deletes:` tokens).
Cards are the source of truth;
this section is the input `_plan_validate.py`'s `all-files-touched-mismatch` check cross-references against the derived union of every card's `Edits:`/`Creates:`/Move-target paths, to catch drift between the hand/agent-maintained list here and that derived union._

- `CONSTRAINTS.md`
- `docs/overview.md`
- `internal/loomshed/batchifier.go`
- `internal/loomshed/batchifier_test.go`
- `internal/loomshed/discussionvalidate.go`
- `internal/loomshed/discussionvalidate_test.go`
- `internal/loomshed/loompreflight.go`
- `internal/loomshed/loompreflight_test.go`
- `internal/loomshed/loomshed.go`
- `internal/loomshed/planvalidate.go`
- `internal/loomshed/planvalidate_test.go`
- `internal/loomshed/resume_test.go`
- `internal/loomshed/stub.go`
- `internal/loomshed/stub_test.go`
- `internal/loomshed/webster.go`
- `internal/loomshed/webster_test.go`
- `internal/shedrecipe/config.go`
- `internal/shedrecipe/config_test.go`
- `internal/shedrecipe/coverage_guard_test.go`
- `internal/shedrecipe/doc.go`
- `internal/shedrecipe/entries_bouncer.go`
- `internal/shedrecipe/entries_bouncer_test.go`
- `internal/shedrecipe/entries_burler.go`
- `internal/shedrecipe/entries_burler_test.go`
- `internal/shedrecipe/entries_simple.go`
- `internal/shedrecipe/entries_simple_test.go`
- `internal/shedrecipe/entries_singlellm.go`
- `internal/shedrecipe/entries_singlellm_test.go`
- `internal/shedrecipe/env.go`
- `internal/shedrecipe/env_test.go`
- `internal/shedrecipe/fixture_test.go`
- `internal/shedrecipe/paths.go`
- `internal/shedrecipe/paths_test.go`
- `internal/shedrecipe/recipe.go`
- `internal/shedrecipe/registry.go`
- `internal/shedrecipe/registry_test.go`
- `internal/shedrecipe/seam_enforcement_test.go`
- `manifest/designs/shed-recipe.md`
- `manifest/roadmap.md`
