# Plan: Bouncer: the generic review-gate producer

```yaml
task: 'Bouncer: the generic review-gate producer'
slug: 'shedadapters-generic-bouncer-producer'
approved: false
started: '20260820-153008'
parent: 'main'
root: ""
verify: null
```

## Batch Index

_The fenced yaml block below is the authoritative DAG mill-go reads to schedule batches.
Every batch lives at `NN-<batch-slug>.md` in this directory and is mirrored as one entry here._

```yaml
batches:
  - number: 1
    name: file-contracts
    file: 01-file-contracts.md
    depends-on: []
    verify: go test ./internal/shedadapters/...
  - number: 2
    name: stencils
    file: 02-stencils.md
    depends-on: [1]
    verify: go test ./contracts/stencils/...
  - number: 3
    name: bouncer-producer
    file: 03-bouncer-producer.md
    depends-on: [1, 2]
    verify: go test ./internal/shedadapters/...
  - number: 4
    name: judge-replay-coverage
    file: 04-judge-replay-coverage.md
    depends-on: [3]
    verify: go test ./internal/shedadapters/...
  - number: 5
    name: manifest-docs
    file: 05-manifest-docs.md
    depends-on: [4]
    verify: go test ./internal/lyxcwd/...
```

## Shared Decisions

_Cross-cutting decisions every batch inherits: naming conventions, error-handling posture, test frameworks, style/lint constraints.
One subsection per decision.
Batch-local decisions live in each batch file._

### Decision: everything the Bouncer owns stays unexported except `ResolveRound`

- **Decision:** the three parsers, the focus writer, the round-path helpers, the `judged` predicate, and every contract type are package-unexported identifiers in `internal/shedadapters`.
  `ResolveRound`, `Bouncer`, `BouncerConfig`, and `NewBouncer` are the only new exported names.
- **Rationale:** `_mill/discussion.md` names exactly one export obligation — the round-resolution helper, so the later Burler-round producer resolves the same round from the same convention without duplicating it.
  Everything else is consumed only from inside this package (the Burler-round producer lands in this same package), and the package's own no-new-engine-surface posture is what `archive.go`'s header comment already records for `firstFreeArchivePath`.
- **Applies to:** all batches

### Decision: two-layer error posture — fail-loud parsers, fail-safe caller

- **Decision:** every parser returns a `bouncer: `-prefixed error and never defaults silently.
  Every swallow of such an error lives in `Call` (or a helper `Call` alone reaches), as a `logger.Warn` plus `shedengine.Stuck`.
  The two layers live in separate functions so the split stays visible.
- **Rationale:** this is the posture `internal/treadleengine/judgeverdict.go` and `internal/treadleengine/handoff.go` already document across two packages; the Bouncer has both layers in one package, so keeping them in separate functions is what preserves the distinction.
- **Applies to:** all batches

### Decision: `Done` is unreachable from any degraded path

- **Decision:** every fail-safe fallback in `Call` resolves to `shedengine.Stuck`.
  `shedengine.Done` is returned only after a verdict file has been read and parsed as `APPROVED`.
- **Rationale:** a false stuck costs a few bounded extra rounds, bounded by `ProducerDef.MaxBounces`; a false pass ships an unreviewed artifact.
  Degrading toward `Done` is the one genuinely unsafe direction.
- **Applies to:** all batches

### Decision: every discriminator is "exists and parses", never bare existence

- **Decision:** the seed-versus-re-bounce discriminator, the `judged(N)` predicate, and both focus-synthesis triggers all require the file to parse, not merely to be present.
- **Rationale:** a truncated or malformed artifact must never be mistaken for a real one.
  Keying on mere existence would leave a crash between a spawn writing a malformed file and the fallback repairing it permanently unrecoverable.
- **Applies to:** batches 1, 3, 4

### Decision: the focus-file format is stated in both templates, in identical wording

- **Decision:** `bouncer-template-seed.md` and `bouncer-template-judge.md` each state the focus-file format in full, using byte-identical wording for the shared section, and both name `internal/shedadapters/bouncerfiles.go`'s parser as the authority the format is enforced by.
- **Rationale:** the Producer Pointer-Rule Invariant binds an instruction file duplicating *another producer's* format contract.
  Both templates drive the same producer and the same contract owner, and the focus file is the seed prompt's only required output — a seed agent pointed at a contract it was given no path to could not write its one file at all.
  Identical wording is what keeps a later edit to one from silently diverging from the other.
- **Applies to:** batch 2

### Decision: Go markers are `{{.name}}`, filled with `stencil.Fill`

- **Decision:** every marker in both templates is a top-level `{{.name}}` substitution, filled via `stencil.Fill` (never `FillOptional`), with no `{{if}}` or `{{range}}` anywhere in either file.
- **Rationale:** `internal/stencil/stencil.go`'s `Fill` errors on any top-level marker absent or empty, which makes the marker set a hard contract between template text and Go call site.
  A marker inside a conditional branch renders silently blank when present-but-empty, which every existing stencil's own header comment warns against.
- **Applies to:** batches 2, 3, 4

### Decision: markdown style in every new or edited `.md`

- **Decision:** semantic line breaks — one sentence per line, plus a break at an internal independent-clause boundary in a long sentence.
  Never a fixed-column hard wrap, never a trailing double-space or backslash.
- **Rationale:** the repo's own `CLAUDE.md` rule, binding on the two new stencil templates and every doc this task touches.
- **Applies to:** batches 2, 5

### Decision: `docs/overview.md` is amended alongside `manifest/designs/shed.md`

- **Decision:** `docs/overview.md`'s module-tree line for `internal/shedadapters` and its `shed` bullet both say "three ... adapters" today, naming `SingleLLMProducer`, perch, and Webster.
  Both become four, with the Bouncer named.
- **Rationale:** this is the same falsehood the `doc.go` and `shed.md` edit lists are careful about, in a third document.
  The module map changing is exactly the trigger the repo's task-completion rule names for touching `docs/overview.md`.
- **Applies to:** batch 5

## All Files Touched

- `contracts/stencils/bouncer/bouncer-template-judge.md`
- `contracts/stencils/bouncer/bouncer-template-seed.md`
- `contracts/stencils/registry_test.go`
- `contracts/stencils/stencils.go`
- `docs/overview.md`
- `internal/shedadapters/bouncer.go`
- `internal/shedadapters/bouncer_config_test.go`
- `internal/shedadapters/bouncer_judge_test.go`
- `internal/shedadapters/bouncer_replay_test.go`
- `internal/shedadapters/bouncer_seed_test.go`
- `internal/shedadapters/bouncerfiles.go`
- `internal/shedadapters/bouncerfiles_test.go`
- `internal/shedadapters/doc.go`
- `internal/shedadapters/round.go`
- `internal/shedadapters/round_test.go`
- `manifest/designs/shed.md`
- `manifest/roadmap.md`
