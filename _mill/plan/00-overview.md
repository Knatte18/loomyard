# Plan: loom: Discussion-Write producer

```yaml
task: 'loom: Discussion-Write producer'
slug: loom-discussion-write-producer
approved: false
started: '20260824-134548'
parent: main
root: ""
verify: go build ./...
```

## Batch Index

_The fenced yaml block below is the authoritative DAG mill-go reads to schedule batches.
Every batch lives at `NN-<batch-slug>.md` in this directory and is mirrored as one entry here._

```yaml
batches:
  - number: 1
    name: loomengine seam and stencil rewrite
    file: 01-loomengine-and-stencil.md
    depends-on: []
    verify: go test ./internal/loomengine/...
  - number: 2
    name: loomshed DiscussionWrite commit decorator
    file: 02-loomshed-decorator.md
    depends-on: []
    verify: go test ./internal/loomshed/...
  - number: 3
    name: registry entry, recipe row flip, and wiring
    file: 03-registry-recipe-wiring.md
    depends-on: [1, 2]
    verify: go test ./internal/shedrecipe/... ./internal/loomrecipe/... ./internal/loomcli/...
  - number: 4
    name: documentation lifecycle
    file: 04-docs.md
    depends-on: [3]
    verify: go test ./internal/lyxcwd/...
```

## Shared Decisions

### Decision: autonomous-only

- **Decision:** `Discussion-Write` runs with `Interactive: false`.
  The wiring site passes `autonomous: true` to `loomengine.DiscussionSpec` unconditionally, and `loomengine.DiscussionSpec` keeps its `autonomous bool` parameter with both `modeRules` branches intact.
- **Rationale:** interactive interviewing does not survive resume with today's machinery — `SingleLLMProducer.Call` archives freshly-written output files and respawns an agent that knows nothing of the interview, and a resume-on-output-files pre-check cannot distinguish an interrupted interview from a `Discussion-Validate` bounce.
  Keeping the parameter and the unreached interactive branch means the committed follow-up roadmap item flips one argument rather than re-authoring tested prose.
- **Applies to:** all batches

### Decision: spec-closure-in-env

- **Decision:** `shedrecipe.Env` gains two per-producer passthrough fields, `DiscussionSpec shedadapters.SpecSource` and `CommitDiscussion func() error`, both filled by `internal/loomcli`'s `wire()`.
  No generic `SpecSources` map, and no refactor of `loomengine.DiscussionSpec`'s signature.
- **Rationale:** `loomengine.DiscussionSpec` takes a `*lyxcwd.Location`, and the Shed Recipe Registry Invariant bars `internal/shedrecipe` from a direct `internal/lyxcwd` production import.
  `Env` already solves this shape twice with whole-value passthrough fields (`WebsterDeps`, `Landing`) and already carries per-producer named fields (`DecisionRecordPath`, `SupportLogPath`).
  A generic keyed map is speculative generality for one call site; Wave 3's `Plan-Write` adds `Env.PlanSpec` symmetrically when it needs it.
- **Applies to:** batch 3

### Decision: commit-produced-artifacts

- **Decision:** a thin `internal/loomshed` decorator commits `_lyx/discussion/`'s whole directory into the weft as soon as the wrapped `SingleLLMProducer` reports `Done`, before `Discussion-Validate` has judged anything.
  A commit failure returns an error, never `Stuck`.
- **Rationale:** `DiscussionDir` anchors at `lyxdirs.LyxDirName` (`_lyx`), while `fabricengine`'s weft exclude list covers only `lyxdirs.DotLyxDirName` (`.lyx`) — so both produced files are ordinary trackable weft content nothing currently commits, leaving the weft dirty for `Finalize`'s merge guard and any fresh `Preflight` fabric scan.
  Committing before validation is intentional: the commit keeps the weft clean and the artifact durable, it does not certify it.
  A git fault is not something re-writing the discussion can fix, which is exactly the reasoning `discussionvalidate.go` already applies to a non-not-exist read failure.
- **Applies to:** batches 2, 3

### Decision: directory-pathspec-includes-archives

- **Decision:** the commit pathspec is the whole `_lyx/discussion/` directory via a new `loomengine.DiscussionDirRel()`, not the two output-file paths.
- **Rationale:** `shedadapters.archiveStaleOutputs` renames a stale output to a timestamped sibling in the same directory, so each bounce round leaves a `decision-record-<stamp>.md` beside the live files.
  A two-file pathspec would leave those as untracked weft dirt, re-creating the exact problem this decision exists to eliminate; the archived draft is also the only surviving record of what the validator rejected.
  The accessor lives in `loomengine` because the Cwd Resolution Invariant makes `loomengine` the sole declarer of that path.
- **Applies to:** batches 1, 3

### Decision: tests-stay-tier-1

- **Decision:** every test this plan adds or changes is hermetic and untagged (Tier 1).
  No test spawns a real agent, resolves a real cwd, or needs a real git repo.
  The commit decorator is tested with an injected `commit` closure; the registry entry is tested with an injected `SpecSource` and a fake `Shuttle`.
- **Rationale:** the Test Tier Purity Invariant, plus `internal/loomcli/smoke_test.go`'s own doc comment, which already leans on the durable status file rather than on live producers.
  An end-to-end test driving a real autonomous agent would be slow and non-deterministic.
- **Applies to:** all batches

### Decision: fixture-shuttle-writes-by-default

- **Decision:** `internal/loomrecipe`'s `buildSequenceFixture` gains a fake `Shuttle` that writes both discussion output files with valid content and reports `shuttleengine.OutcomeDone`.
  The two bounce-routing tests that deliberately keep the decision record absent opt into a non-writing variant of the same fake.
- **Rationale:** row 3 stops being a `Stub` that reports `Done` without touching disk.
  A real `SingleLLMProducer` archives its stale output files on every `Call`, so a non-writing fake would archive the fixture's pre-written discussion files away and break `Discussion-Validate` in the clean sequence test.
  Conversely, a writing fake would re-create the decision record the bounce tests remove, destroying their premise.
  The two behaviours are genuinely different fixtures, so the knob is explicit rather than inferred.
- **Applies to:** batch 3

### Decision: markdown-semantic-line-breaks

- **Decision:** every `.md` file this task touches uses semantic line breaks — one sentence per line, with an additional break at an internal independent-clause boundary inside a long sentence.
  No fixed-column hard wrap, no trailing double-spaces, no backslash line breaks.
- **Rationale:** `CLAUDE.md`'s markdown rule, which applies to prose paragraphs and list items in every `.md` file in this repo, not only newly-written ones.
  Table cells and blockquotes stay on one line.
- **Applies to:** batches 1, 4

## All Files Touched

- `CONSTRAINTS.md`
- `contracts/recipes/loom-recipe.yaml`
- `contracts/stencils/loom/loom-template-discussion.md`
- `internal/loomcli/wiring.go`
- `internal/loomcli/wiring_test.go`
- `internal/loomengine/config.go`
- `internal/loomengine/discussion.go`
- `internal/loomengine/discussionpath_test.go`
- `internal/loomengine/prompt.go`
- `internal/loomengine/prompt_test.go`
- `internal/loomengine/template.yaml`
- `internal/loomrecipe/coverage_guard_test.go`
- `internal/loomrecipe/fixture_test.go`
- `internal/loomrecipe/resume_test.go`
- `internal/loomrecipe/sequence_test.go`
- `internal/loomrecipe/shape_test.go`
- `internal/loomshed/discussionwrite.go`
- `internal/loomshed/discussionwrite_test.go`
- `internal/loomshed/doc.go`
- `internal/loomshed/stub.go`
- `internal/shedrecipe/entries_discussionwrite.go`
- `internal/shedrecipe/entries_discussionwrite_test.go`
- `internal/shedrecipe/fixture_test.go`
- `internal/shedrecipe/recipe.go`
- `internal/shedrecipe/registry.go`
- `internal/shedrecipe/registry_test.go`
- `manifest/designs/loom.md`
- `manifest/designs/plan-card-format.md`
- `manifest/designs/shed-recipe.md`
- `manifest/roadmap.md`
