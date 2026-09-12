# Plan: lyx loom step + external supervisor skill

```yaml
task: "lyx loom step + external supervisor skill"
slug: "loom-step"
approved: true
started: "20260912-104814"
parent: "main"
root: ""
verify: null
discussion_sha: "3e113fe3db5f3d56354809f5e57c4eb4bb4cf146"
```

## Batch Index

_The fenced yaml block below is the authoritative DAG mill-go reads to schedule batches.
Every batch lives at `NN-<batch-slug>.md` in this directory and is mirrored as one entry here._

```yaml
batches:
  - number: 1
    name: shedengine-step
    file: 01-shedengine-step.md
    depends-on: []
    verify: go test ./internal/shedengine/...
  - number: 2
    name: loomshed-interrupt-policy
    file: 02-loomshed-interrupt-policy.md
    depends-on: []
    verify: go test ./internal/loomshed/... ./internal/loomrecipe/...
  - number: 3
    name: loomcli-shared-helpers
    file: 03-loomcli-shared-helpers.md
    depends-on: []
    verify: go test ./internal/loomcli/...
  - number: 4
    name: loom-step-verb
    file: 04-loom-step-verb.md
    depends-on: [1, 2, 3]
    verify: go test ./internal/loomcli/... ./cmd/lyx/... ./internal/lyxcwd/...
  - number: 5
    name: status-interrupt-policy
    file: 05-status-interrupt-policy.md
    depends-on: [4]
    verify: go test ./internal/loomcli/... ./internal/lyxcwd/...
  - number: 6
    name: ly-plugin-supervise-skill
    file: 06-ly-plugin-supervise-skill.md
    depends-on: [5]
    verify: go test ./internal/lyxcwd/...
  - number: 7
    name: design-doc-and-roadmap
    file: 07-design-doc-and-roadmap.md
    depends-on: [6]
    verify: go test ./internal/lyxcwd/...
```

## Shared Decisions

_Cross-cutting decisions every batch inherits: naming conventions, error-handling posture, test frameworks, style/lint constraints.
One subsection per decision.
Batch-local decisions live in each batch file._

### Decision: go-native-verify-no-pythonpath-prefix

- **Decision:** every `verify:` command in this plan is a bare `go test` invocation with no `PYTHONPATH= ` prefix.
- **Rationale:** `PYTHONPATH= ` scopes a Python-interpreter reset and is mill's convention for Python projects only. This repo is Go; the native runner is `go test`. The `verify-not-isolated` validator check is conditional on project language and does not fire for Go.
- **Applies to:** all batches

### Decision: cgo-required-for-every-verify

- **Decision:** every `go test` run in this plan needs `CGO_ENABLED=1` and a C compiler on `PATH`, and no batch sets `CGO_ENABLED=0` or adds a build tag to avoid it.
- **Rationale:** `CONSTRAINTS.md`'s Quarry CGO Requirement Invariant — `lyx` links quarry's tree-sitter grammars through cgo, so the whole module tree fails to build without it. `CGO_ENABLED` already defaults to `1` on a machine with a compiler on `PATH`, so no batch sets it explicitly.
- **Applies to:** all batches

### Decision: behaviour-preserving-extraction-means-existing-tests-unedited

- **Decision:** in batches 1 and 3 the existing test suites (`internal/shedengine`'s `run_routing_test.go`, `run_persist_test.go`, `run_pause_test.go`, `run_commitstatus_test.go`; `internal/loomcli`'s `bootstrap_test.go`, `wiring_test.go`, `parity_test.go`) must pass **unedited**.
- **Rationale:** those suites are the only mechanical guarantee that the `Run`-body extraction and the `run`/`drive` helper extractions preserve behaviour. A test edited to accommodate a refactor stops being that guarantee. If one of them fails, the refactor is wrong, not the test.
- **Applies to:** 1, 3

### Decision: new-tests-stay-untagged-and-pure

- **Decision:** every test file this plan creates is untagged — no `//go:build integration` and no `//go:build smoke`. Where a behaviour would otherwise need a real fabric, a real reed session, or a real git spawn, the plan factors the decision into a pure function and tests that instead.
- **Rationale:** `CONSTRAINTS.md`'s Test Tier Purity Invariant bars `gitexec`/`exec.Command`/`hubforge.NewHub` from untagged test files. The repo's own precedent for this is `internal/loomcli/bootstrap.go`, whose pure decisions (`mustSpawnDriver`, `awaitRunLock`, `dispositionForHandshake`, `resolveStatusStrandAction`) are fully tested with no real lock, process, or clock — batch 4 mirrors that split for `step`'s envelope and refusal-kind decisions.
- **Applies to:** 1, 2, 3, 4, 5

### Decision: step-envelope-key-set-is-closed-at-ten

- **Decision:** `lyx loom step`'s success envelope carries exactly ten keys: `producer`, `outcome`, `output`, `next`, `state`, `reason`, `continue`, `history_length`, `next_interrupt_policy`, `status_file` — plus `ok`, which `output.Ok` injects itself.
- **Rationale:** the discussion pins this tuple as the single target for the "full envelope key set is present" test. A key outside it has no test and no documented meaning.
- **Applies to:** 4

### Decision: refusal-kind-vocabulary-is-closed-at-five

- **Decision:** `lyx loom step`'s failure envelopes go out through `output.ErrFields` carrying exactly one `kind` value from `{"busy", "unseeded", "ownership", "bootstrap", "producer"}`, declared as Go constants in `internal/loomcli/step.go` and asserted as an exact set by a test.
- **Rationale:** the skill branches on `kind` mechanically, and its one-retry rule applies to `kind: "producer"` alone. An undeclared sixth kind would silently fall into the skill's hand-back path with no operator-facing explanation. `output.ErrFields` is the existing seam for per-verb keys and already carries them for `internal/loomcli/validate.go`, `internal/webstercli/beginbatch.go`, `internal/shuttlecli/run.go`, and `internal/fabriccli/envelope.go`; `output.Err` itself is not changed.
- **Applies to:** 4

### Decision: helpers-are-lock-agnostic

- **Decision:** the three helpers batch 3 extracts (`seedAndCommitBootstrap`, `ensureStatusStrand`, `buildLoomShed`) neither acquire nor release `loomengine.LoomBootstrapLock`. Each calling verb wraps its own lock window around them.
- **Rationale:** the lock's position differs between the verbs. `run` acquires at its step 4 — after seed, ownership verify, and `CommitAnchoredPaths` — and holds it across the strand work, the driver spawn, and the run-lock handshake, releasing explicitly at step 7. `step` spawns no driver and runs no handshake, so it wraps the strand block alone and releases **before** calling the producer, since it must never hold the lock across a minutes-long LLM row. A helper that encoded either choice would force the other verb's behaviour to change.
- **Applies to:** 3, 4

### Decision: bootstrap-is-not-serialised-by-the-run-lock-and-that-is-inherited

- **Decision:** two concurrent `lyx loom step` invocations can both execute `CommitAnchoredPaths` unserialised before either reaches the run lock, and this plan accepts that rather than fixing it.
- **Rationale:** `Step` acquires the run lock inside `shedengine`, after the whole bootstrap has already run, and `run` today takes the bootstrap lock later than its own seed/verify/commit block. The commit is idempotent (`StageAndCommit` reports `committed == false` on an already-clean, already-tracked path), `loomshed.Seed` is serialised by its own lock and signals `ErrSeedExists`, and widening the bootstrap lock to cover the commit would change `run`'s observable behaviour, which this task explicitly does not do. No card may present the run lock as covering bootstrap — it covers the producer call.
- **Applies to:** 3, 4

### Decision: interrupt-policy-is-go-supplied-never-a-producer-name

- **Decision:** the policy word (`"reinvoke"` / `"handback"`) is computed in Go from a single exported table in `internal/loomshed` and handed to the skill on the envelope. No card may put a producer name inside `plugins/ly/skills/ly-supervise/SKILL.md`.
- **Rationale:** `docs/overview.md` Principle 7 keeps phase sequencing in deterministic Go, and this task's own Scope § Out rule says the skill never names a producer. Branching on `current_producer == "Webster"` inside the skill would put a fragment of phase knowledge there and rot the moment a row is renamed.
- **Applies to:** 2, 4, 5, 6

### Decision: docs-land-in-the-same-commit-as-the-change

- **Decision:** `docs/overview.md`'s loom verb enumeration and `manifest/designs/loom.md`'s verb-list prose move in batch 4 alongside the verb itself; `loom.md`'s status-envelope description moves in batch 5 alongside the key; `loom.md`'s `/ly-*` skills-table row moves in batch 6 alongside the skill. Only `manifest/designs/loom-step.md`'s status flip and `manifest/roadmap.md`'s Planned-item removal are deferred to batch 7, because both are completion markers for the task as a whole.
- **Rationale:** `CLAUDE.md`'s task-completion rule requires the module doc, `docs/overview.md`, and `CONSTRAINTS.md` to move in the same commit as the change that makes them true. `manifest/roadmap.md` is the documented exception: it moves only on completing the planned item.
- **Applies to:** 4, 5, 6, 7

### Decision: loom-md-edits-are-serialised-through-the-batch-chain

- **Decision:** batches 4, 5, and 6 each edit `manifest/designs/loom.md`, and the DAG gives them a linear `depends-on` chain (4 → 5 → 6) for that reason alone.
- **Rationale:** three batches editing one file with no dependency edge between them is a `parallel-modifies-overlap` defect. Batch 5's only genuine code dependency is batch 2; the edge to batch 4 exists to serialise the shared doc edit, and is recorded here so a reviewer does not read it as a claimed code dependency.
- **Applies to:** 4, 5, 6

### Decision: no-version-bumps-and-no-new-invariant-expected

- **Decision:** `plugins/ly/.claude-plugin/plugin.json` ships at version `1.0.0`, its `.claude-plugin/marketplace.json` entry likewise, and no existing plugin or package version is bumped. No card adds an entry to `CONSTRAINTS.md`.
- **Rationale:** unpublished loomyard plugins stay at `1.0.0`. On the invariant: the discussion expects none, and the extracted `stepLocked` seam is pinned mechanically by batch 1's `Run`/`Step` equivalence test rather than by review discipline — which is what a `CONSTRAINTS.md` entry would otherwise buy. If an implementer finds a cross-cutting rule that genuinely cannot be machine-checked, `CLAUDE.md` requires recording it in `CONSTRAINTS.md` in the same commit, and that remains open to them.
- **Applies to:** all batches

## All Files Touched

- `.claude-plugin/marketplace.json`
- `cmd/lyx/helptree_test.go`
- `docs/overview.md`
- `internal/loomcli/cli.go`
- `internal/loomcli/cli_test.go`
- `internal/loomcli/drive.go`
- `internal/loomcli/run.go`
- `internal/loomcli/sharedbootstrap.go`
- `internal/loomcli/sharedbootstrap_test.go`
- `internal/loomcli/status.go`
- `internal/loomcli/status_test.go`
- `internal/loomcli/step.go`
- `internal/loomcli/step_test.go`
- `internal/loomcli/wiring_test.go`
- `internal/loomrecipe/interruptpolicy_meta_test.go`
- `internal/loomshed/interruptpolicy.go`
- `internal/loomshed/interruptpolicy_test.go`
- `internal/shedengine/run.go`
- `internal/shedengine/step_equivalence_test.go`
- `internal/shedengine/step_test.go`
- `manifest/designs/loom-step.md`
- `manifest/designs/loom.md`
- `manifest/roadmap.md`
- `plugins/ly/.claude-plugin/plugin.json`
- `plugins/ly/skills/INDEX.md`
- `plugins/ly/skills/ly-supervise/SKILL.md`
