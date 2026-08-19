# Plan: landing: Publish + Finalize producers

```yaml
task: 'landing: Publish + Finalize producers'
slug: landing-publish-finalize-producers
approved: true
started: '20260819-125210'
parent: 'standalone-producers'
root: ""
verify: go vet ./...
```

## Batch Index

_The fenced yaml block below is the authoritative DAG mill-go reads to schedule batches.
Every batch lives at `NN-<batch-slug>.md` in this directory and is mirrored as one entry here._

```yaml
batches:
  - number: 1
    name: merge-stage-resolved verb
    file: 01-merge-stage-resolved-verb.md
    depends-on: []
    verify: go test ./internal/gitrepo/... ./internal/fabricengine/... ./cmd/lyx/... && go test -tags integration ./internal/gitrepo/... ./internal/fabricengine/...
  - number: 2
    name: remote and push helpers
    file: 02-remote-and-push-helpers.md
    depends-on: []
    verify: go test ./internal/gitrepo/... ./internal/githubclient/... ./internal/fabricengine/... ./cmd/lyx/... && go test -tags integration ./internal/gitrepo/... ./internal/fabricengine/...
  - number: 3
    name: mergeresolve engine
    file: 03-mergeresolve-engine.md
    depends-on: [1]
    verify: go test ./internal/mergeresolve/... ./contracts/stencils/... ./internal/lyxcwd/...
  - number: 4
    name: landingshed producers
    file: 04-landingshed-producers.md
    depends-on: [2, 3]
    verify: go test ./internal/landingshed/... ./internal/configreg/... ./internal/fabricengine/... ./cmd/lyx/...
  - number: 5
    name: loomshed wiring and integration
    file: 05-loomshed-wiring-and-integration.md
    depends-on: [4]
    verify: go test ./internal/loomshed/... ./internal/landingshed/... ./cmd/lyx/... && go test -tags integration ./internal/loomshed/... ./internal/landingshed/...
  - number: 6
    name: documentation lifecycle
    file: 06-documentation-lifecycle.md
    depends-on: [5]
    verify: go test ./internal/lyxcwd/... ./cmd/lyx/... ./internal/landingshed/... ./internal/mergeresolve/...
```

## Shared Decisions

_Cross-cutting decisions every batch inherits: naming conventions, error-handling posture, test frameworks, style/lint constraints.
One subsection per decision.
Batch-local decisions live in each batch file._

### Decision: neither new package may name a fabric-internal side

- **Decision:** No production file in `internal/mergeresolve` or `internal/landingshed` may contain either fabric-internal side's name in an identifier, a string literal, or a comment — including as a selector on a call into the engine that owns those names. Their documentation describes one repository throughout. The same ban binds the new conflict stencil.
- **Rationale:** This is machine-enforced, not stylistic. The enforcement walk inspects every identifier node in every non-owner production file under `internal/` and `cmd/`, plus the Markdown under `internal/` and `contracts/stencils/`. Neither new package is in the owner set, so a call expression naming the forbidden verb fails the build on the identifier alone — the call would not even need to run. Three consequences follow and they shape the told-value struct: it carries no path field for either side, the push is injected as a closure the caller fills because the push verb's own name is forbidden here, and the remote URL is a told string rather than something either package reads for itself.
- **Applies to:** all batches

### Decision: told values only, and the scratch directory is told rather than derived

- **Decision:** Both new packages take every absolute path from their caller and derive none. Each ships its own import-policing test excluding direct geometry-resolution imports, modelled on the existing sibling package's test. Whoever writes into the told scratch directory creates it first, on every write path.
- **Rationale:** The Told-Geometry Invariant's membership predicate is a direct production import, and machine-enforced membership means a test in the package polices its own import set. Telling the scratch path rather than deriving it is mandatory rather than preferred: deriving it would name a reserved directory literal only one package may declare, and would compute geometry these packages are forbidden outright. Creating a told directory is legal and is exactly what the producer engine already does for its own lock parents; deriving one would not be.
- **Applies to:** all batches

### Decision: a stuck verdict carries its reason in a log line and a file, never in the verdict

- **Decision:** Every "stuck with a distinct message" in this plan means two concrete artifacts — a structured warning log line with the case's own fields, and a one-line reason file in the told scratch directory, overwritten each attempt — while the producer returns a bare stuck verdict.
- **Rationale:** The producer seam returns only a verdict, an output pointer, and an error. The driving engine persists a fixed reason string of its own for every stuck verdict, so a producer-supplied reason has nowhere to go; the output pointer is never persisted at all; and returning an error instead would flip the persisted state from blocked, which a human resumes, to failed, which ends the run. Consequence for testing: every assertion in this plan that two stuck causes are "distinguishable" asserts on the reason file's contents, never on a returned string and never on the engine's persisted reason, which is identical in all of them.
- **Applies to:** landingshed producers, loomshed wiring and integration

### Decision: verify mechanically before concluding, and abort rather than guess

- **Decision:** A conflict is declared resolved by a mechanical re-scan of the conflicted paths, never by the session's own verdict. Clean scan stages the resolved paths and then concludes, in that order. A still-dirty scan retries the session once and then aborts and reports stuck. Merge state a human left behind is never touched, and an error the plan does not name explicitly escalates with its text surfaced rather than falling into a wrong branch.
- **Rationale:** Concluding is irreversible at the engine layer — there is no post-conclude undo, and the abort verb covers only the uncommitted attempt window — so the abort call is the only checkpoint that exists. The scan is deliberately biased toward refusing: it is line-anchored and content-only, so a resolved file whose legitimate content carries line-start markers escalates rather than concluding. That is the safe direction, and it is why the conflict stencil may never contain a literal line-start marker.
- **Applies to:** mergeresolve engine, landingshed producers, loomshed wiring and integration

### Decision: pinned guard tables and invariant lists move in the same commit as the code

- **Decision:** Every set-equality guard this task touches is updated in the same commit as the change that makes it necessary: the boundary method list and its prose counterpart, the mutation-record result-type table, the strict config-loading set, the config module registry and its expected-names list, the stencil registry, the producer-list import allowlist, and the invariant's machine-enforced package list.
- **Rationale:** These guards fail in both directions, so a table entry landing a commit early is as broken as one landing a commit late. This is also the Mutation Record Invariant's explicit same-commit rule for a new mutation kind, its recording site, and its guard entry.
- **Applies to:** all batches

### Decision: no test contacts a real model or a real remote service, and tier membership is deliberate

- **Decision:** Every unit tier runs against injected seams — the merge surface, the session runner, the push, the two pair openers, and the service client are all fakes or local test servers. A test file that spawns git or uses a hub fixture carries the integration build constraint as its first line; a test file that spawns nothing stays untagged.
- **Rationale:** The Test Tier Purity Invariant forbids an untagged file from spawning anything, and the Hermetic Git Test Environment Invariant requires a git-spawning package to wire the hermetic environment in its own test entry point. Both are machine-checked, so getting tier membership wrong fails the build rather than merely slowing the suite. The integration tier is kept small and purposeful: two producers, one merge verb, and the two helpers that genuinely need a repository on disk.
- **Applies to:** all batches

### Decision: nothing in this task fills the two pair-opener closures, and that is deliberate

- **Decision:** The told-value struct declares both opener closures, the producer list passes them through, and no production caller fills either. This task adds no command and builds no resolution chain for the parent pair's path.
- **Rationale:** The producer-list constructor has no production caller anywhere in the tree today — only its own tests reference it — so there is nothing to wire into. The resolution chain is specified in the struct's own documentation and built by the next roadmap item, which is what makes the phase machine reachable at all. Until then both closures are exercised only by this task's tests, which fill them directly against real fixtures; a nil required closure is a construction error rather than a silent no-op.
- **Applies to:** landingshed producers, loomshed wiring and integration

## All Files Touched

_Full union of every `Creates:` / `Edits:` / `Moves:` **target** path across every batch, sorted alphabetically (Move **source** paths are excluded — they disappear, like `Deletes:` tokens)._

- `CONSTRAINTS.md`
- `cmd/lyx/configstrictness_test.go`
- `cmd/lyx/destructiveguard_test.go`
- `cmd/lyx/gitrepoboundary_test.go`
- `contracts/stencils/landing/landing-template-conflict.md`
- `contracts/stencils/stencils.go`
- `docs/overview.md`
- `internal/configreg/configreg.go`
- `internal/configreg/configreg_test.go`
- `internal/fabricengine/mergeerrors.go`
- `internal/fabricengine/mergeerrors_test.go`
- `internal/fabricengine/mergestage.go`
- `internal/fabricengine/mergestage_integration_test.go`
- `internal/fabricengine/mutation.go`
- `internal/fabricengine/pushrebasefree_integration_test.go`
- `internal/fabricengine/spawn.go`
- `internal/githubclient/parseownerrepo.go`
- `internal/githubclient/parseownerrepo_test.go`
- `internal/gitrepo/merge.go`
- `internal/gitrepo/remote.go`
- `internal/gitrepo/remote_integration_test.go`
- `internal/landingshed/config.go`
- `internal/landingshed/config_test.go`
- `internal/landingshed/configtemplate.go`
- `internal/landingshed/ctx.go`
- `internal/landingshed/deps.go`
- `internal/landingshed/doc.go`
- `internal/landingshed/finalize.go`
- `internal/landingshed/finalize_integration_test.go`
- `internal/landingshed/finalize_test.go`
- `internal/landingshed/publish.go`
- `internal/landingshed/publish_integration_test.go`
- `internal/landingshed/publish_test.go`
- `internal/landingshed/seam_enforcement_test.go`
- `internal/landingshed/stuck.go`
- `internal/landingshed/template.yaml`
- `internal/landingshed/testmain_integration_test.go`
- `internal/loomshed/loomshed.go`
- `internal/loomshed/loomshed_test.go`
- `internal/loomshed/seam_enforcement_test.go`
- `internal/loomshed/stub.go`
- `internal/loomshed/stub_test.go`
- `internal/mergeresolve/ctx.go`
- `internal/mergeresolve/deps.go`
- `internal/mergeresolve/doc.go`
- `internal/mergeresolve/markers.go`
- `internal/mergeresolve/markers_test.go`
- `internal/mergeresolve/mergeresolve.go`
- `internal/mergeresolve/mergeresolve_test.go`
- `internal/mergeresolve/seam_enforcement_test.go`
- `internal/mergeresolve/spec.go`
- `manifest/designs/fabric-unified-view.md`
- `manifest/designs/loom.md`
- `manifest/designs/raddle.md`
- `manifest/designs/shed.md`
- `manifest/roadmap.md`
