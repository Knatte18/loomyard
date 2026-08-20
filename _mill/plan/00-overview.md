# Plan: shedadapters: Burler-round producer

```yaml
task: 'shedadapters: Burler-round producer'
slug: 'shedadapters-burler-producer'
approved: false
started: '20260820-152710'
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
    name: burlerengine ClusterExclude
    file: 01-cluster-exclude.md
    depends-on: []
    verify: go test ./internal/burlerengine/
  - number: 2
    name: focus-file contract
    file: 02-focus-file.md
    depends-on: []
    verify: go test ./internal/shedadapters/
  - number: 3
    name: BurlerProducer
    file: 03-burler-producer.md
    depends-on: [1, 2]
    verify: go test ./internal/shedadapters/
  - number: 4
    name: docs
    file: 04-docs.md
    depends-on: [3]
    verify: go test ./internal/lyxcwd/ ./internal/shedadapters/
```

## Shared Decisions

_Cross-cutting decisions every batch inherits: naming conventions, error-handling posture, test frameworks, style/lint constraints.
One subsection per decision.
Batch-local decisions live in each batch file._

### Decision: comment style and semantic line breaks

- **Decision:** Go comments follow the `golang:golang-comments` skill and the surrounding files' own house style — a file-header comment naming what the file implements, godoc on every exported identifier, `--` (double hyphen) rather than an em dash inside `internal/shedadapters` Go comments, matching `perch.go`/`singlellm.go`/`archive.go`.
  Markdown edits use semantic line breaks (one sentence per line, no fixed-column hard wrap), matching `CLAUDE.md`'s rule and the existing prose in `manifest/designs/shed.md` and `docs/overview.md`.
- **Rationale:** `internal/burlerengine` uses em dashes in its own comments and `internal/shedadapters` uses `--`; each batch matches the package it edits rather than imposing one style across both.
- **Applies to:** all batches

### Decision: error text and log-field conventions

- **Decision:** Every error `BurlerProducer` returns is prefixed `shedadapters: <name> (burler): ` exactly as `PerchProducer` and `SingleLLMProducer` do, using the package-level label constant `burlerEngineLabel = "burler"`.
  Every `logger.Warn` call carries at minimum `"producer", p.name` and `"engine", burlerEngineLabel`.
  Errors returned by `internal/burlerengine` code are prefixed `burler: `, matching that package's own existing text.
- **Rationale:** the two packages already have distinct, consistent prefixes; a new adapter that invents a third shape makes an operator's grep across a Shed run's logs unreliable.
- **Applies to:** all batches

### Decision: no new package, no new engine surface beyond `Profile.ClusterExclude`

- **Decision:** the producer lives in the existing `internal/shedadapters` package and reuses `entryErr`/`cancelErr` from `ctx.go` and `archiveStaleOutputs` from `archive.go` verbatim.
  The only new exported surface in `internal/burlerengine` is the `Profile.ClusterExclude` field.
  Nothing is added to, changed in, or extracted from `internal/shedengine`, `internal/perchengine`, or `internal/treadleengine`.
- **Rationale:** the Shed Producer-Seam Invariant forbids `internal/shedengine` gaining anything, and the task's Scope explicitly rules the other three packages out.
- **Applies to:** all batches

### Decision: tests stay Tier 1 (untagged, no substrate spawn)

- **Decision:** every test file this plan adds or edits stays untagged and spawns nothing — no `git`, no `exec.Command`, no `gitkit`/`hubforge` fixture helper, no `time.Sleep` of a second or more.
  Filesystem work uses `t.TempDir()` only.
  `internal/shedadapters` gains no `TestMain`, because it spawns no git.
- **Rationale:** CONSTRAINTS.md's Test Tier Purity Invariant (enforced by `cmd/lyx/tierpurity_test.go`) and its Hermetic Git Test Environment Invariant (enforced by `cmd/lyx/hermeticenv_test.go`).
  `internal/burlerengine` already has a `TestMain` calling `gitkit.HermeticGitEnv()`; this plan adds no git-spawning test there either.
- **Applies to:** all batches

### Decision: the pair predicate is the single completion test

- **Decision:** a round `N` counts as complete if and only if **both** `round-<N>-review.md` and `round-<N>-fixer-report.md` exist in the told `runDir`, where `N` is a positive decimal integer with no leading zeros and the filename matches that shape exactly.
  One scan predicate serves round resolution and prior-round hydration, so the two can never disagree.
  The same predicate is what the follow-on `Bouncer` task will use to tell its seed call from its judge call.
- **Rationale:** file existence *is* the protocol between this producer and the `Bouncer`, and a review-only orphan (a process killed in the phase-A-written/phase-B-pending window) must read as "round `N` incomplete" rather than as "round `N` complete", or hydration names a fixer report that does not exist and `burlerengine`'s `requireExistingPaths` wedges the segment permanently.
- **Applies to:** batch 3, batch 4

### Decision: authorship split for fail-safety

- **Decision:** operator-authored config stays fail-loud — `ResolveFan` is unchanged, and a `ClusterExclude` set against an empty `ClusterFan` is a `validate` error.
  LLM-authored per-round directives clamp instead: an exclusion naming a lens not in the resolved fan is a no-op for that name, an exclusion that would empty the fan drops the whole exclusion, and a directive arriving when the template `Profile` names no fan is dropped by the producer before it ever reaches `validate`.
  Every clamp emits a `logger.Warn`; none is an error.
- **Rationale:** the fan is authoritative config an operator must see fail; the focus file is an advisory, LLM-authored directive whose fallback (run the full fan again) costs tokens, never correctness.
- **Applies to:** batch 1, batch 3

## All Files Touched

_Full union of every `Creates:` / `Edits:` / `Moves:` **target** path across every batch, sorted alphabetically (Move **source** paths are excluded — they disappear, like `Deletes:` tokens).
Cards are the source of truth;
this section is the input `_plan_validate.py`'s `all-files-touched-mismatch` check cross-references against the derived union of every card's `Edits:`/`Creates:`/Move-target paths, to catch drift between the hand/agent-maintained list here and that derived union._

- `docs/overview.md`
- `internal/burlerengine/doc.go`
- `internal/burlerengine/profile.go`
- `internal/burlerengine/profile_test.go`
- `internal/shedadapters/burler.go`
- `internal/shedadapters/burler_test.go`
- `internal/shedadapters/doc.go`
- `internal/shedadapters/focus.go`
- `internal/shedadapters/focus_test.go`
- `manifest/designs/shed.md`
- `manifest/roadmap.md`
