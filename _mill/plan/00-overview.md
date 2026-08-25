# Plan: loom: interactive Discussion-Write

```yaml
task: 'loom: interactive Discussion-Write'
slug: 'loom-discussion-write-interactive'
approved: false
started: '20260825T150258Z'
parent: 'loom-webster-review-producer'
root: ""
verify: go vet ./...
```

## Batch Index

_The fenced yaml block below is the authoritative DAG mill-go reads to schedule batches.
Every batch lives at `NN-<batch-slug>.md` in this directory and is mirrored as one entry here._

```yaml
batches:
  - number: 1
    name: shuttle-await-operator-and-run-outcome
    file: 01-shuttle-await-operator-and-run-outcome.md
    depends-on: []
    verify: go test ./internal/shuttleengine/
  - number: 2
    name: shuttle-attach
    file: 02-shuttle-attach.md
    depends-on: [1]
    verify: go test ./internal/shuttleengine/
  - number: 3
    name: shedadapters-probe-before-archive
    file: 03-shedadapters-probe-before-archive.md
    depends-on: [2]
    verify: go test ./internal/shedadapters/ ./internal/shedrecipe/ ./internal/shedbuild/ ./internal/loomrecipe/
  - number: 4
    name: loom-mode-selector
    file: 04-loom-mode-selector.md
    depends-on: [1]
    verify: go test ./internal/loomengine/ ./internal/loomcli/
  - number: 5
    name: loomrecipe-regression-and-docs
    file: 05-loomrecipe-regression-and-docs.md
    depends-on: [3, 4]
    verify: go test ./internal/loomrecipe/ ./internal/lyxcwd/
```

## Shared Decisions

_Cross-cutting decisions every batch inherits: naming conventions, error-handling posture, test frameworks, style/lint constraints.
One subsection per decision.
Batch-local decisions live in each batch file._

### Decision: discussion-md-is-the-authority

- **Decision:** `_mill/discussion.md`'s `## Decisions` section is the binding specification for this task.
  Every named decision anchor (`asking-non-terminal-via-a-new-spec-field`, `attach-lives-in-shuttleengine`, `attach-reconstructs-the-run-explicitly`, `mechanism-failures-do-not-attach-and-do-not-blindly-respawn`, `attach-only-a-run-that-never-terminated`, `leftover-run-dir-from-a-completed-run`, `candidate-evaluation-order`, `one-live-match-or-none`, `probe-before-archive`, `attach-restarts-the-deadline`, `attach-normalizes-the-spec-it-matches-on`, `ladder-step-1-survives-only-inside-attach`, `accepted-residual-the-completed-crash-window`) is a resolved question, not an open one.
  Where a card's `Requirements:` restates a decision, the discussion's own wording wins on any discrepancy.
- **Rationale:** the design took seven review rounds to converge, and several of its choices are counter-intuitive in isolation (a `"running"` sentinel rather than "empty means attachable"; erroring rather than respawning on an absent reed state file; restarting the deadline rather than inheriting `CreatedAt`).
  An implementer that re-derives them from first principles will reach the rejected alternatives, each of which is named and refuted in the discussion.
- **Applies to:** all batches

### Decision: go-not-python

- **Decision:** this is a Go repository. `verify:` commands are `go test <packages>` with no `PYTHONPATH=` prefix, per mill-plan's own non-Python carve-out.
  New tests are plain `*_test.go` files in the package under test, untagged (Tier 1), driven against the existing hermetic fakes.
- **Rationale:** the repo-wide convention, and the `Test Tier Purity Invariant` forbids an untagged test from spawning git, tmux, or an external binary — every seam this task touches already has a hermetic double (`fakeReed`, `fakeEngine`, `fakeClock`, `fakeShuttle`, `fakeLoomShuttle`).
- **Applies to:** all batches

### Decision: constraints-that-bind-every-batch

- **Decision:** four `CONSTRAINTS.md` invariants bind every card that touches production code here, and no card may resolve a difficulty by breaking one:
  the **Shuttle Provider-Seam Invariant** (`internal/shuttleengine` never imports `internal/shuttleengine/claudeengine`; `AwaitOperator` and `Attach` stay provider-invariant),
  the **Told-Geometry Invariant** (`Attach` takes its scan root from the `Runner` it hangs off via the existing `runDirRoot(cfg, anchorPath)`, and derives no path of its own),
  the **Lyxdirs Single-Declarer Invariant** (no production code may write the `_lyx` or `.lyx` literal; `lyxdirs.LyxDirName` / `lyxdirs.DotLyxDirName` are the only spellings),
  and the **Config Strictness Invariant** (`loomengine` is strict-side, so `discussion_interactive` lands in `template.yaml` and `Config` together or `configengine.Load` fails).
- **Rationale:** `internal/shuttleengine/seam_enforcement_test.go` and `internal/loomengine/config_test.go`'s `TestConfigTemplate_ContainsEveryConfigYAMLTag` already fail the build for two of these; the other two are review obligations that a wrong turn makes silently.
- **Applies to:** all batches

### Decision: no-shedengine-edit

- **Decision:** no card in this plan edits `internal/shedengine`.
  If an implementer finds itself needing to, that is a signal the design drifted and the card should stop rather than proceed.
- **Rationale:** the **Shed Producer-Seam Invariant** restricts that package's production imports to stdlib, `internal/state`, and `internal/lock`, enforced by `internal/shedengine/seam_enforcement_test.go`.
  The whole point of putting attach in `shuttleengine` and the probe in `shedadapters` is that Shed's own resume contract is unchanged: it still re-`Call`s `current_producer` unconditionally.
- **Applies to:** all batches

### Decision: docs-land-with-their-code

- **Decision:** package documentation (`internal/shuttleengine/doc.go`, `internal/shedadapters/doc.go`) is edited in the same batch as the behaviour it describes.
  The repo-level design docs (`manifest/designs/loom.md`, `manifest/designs/shed.md`, `docs/overview.md`, `manifest/roadmap.md`) land together in batch 5, once the behaviour they describe is fully in place.
- **Rationale:** `CONSTRAINTS.md`'s **Documentation Lifecycle** and `CLAUDE.md`'s task-completion rule both require docs in the same commit as the change.
  Splitting the repo-level docs into batch 5 keeps a single coherent rewrite of `loom.md`'s crash-recovery discipline rather than five partial edits to one section.
- **Applies to:** all batches

### Decision: markdown-style-and-link-integrity

- **Decision:** every `.md` edit uses semantic line breaks (one sentence per line, plain newlines, never a fixed-column hard wrap and never trailing double-spaces), per `CLAUDE.md`'s markdown rule.
  The heading `### Crash recovery — resume on output files, not live processes` in `manifest/designs/loom.md` **is not renamed**, so its anchor `#crash-recovery--resume-on-output-files-not-live-processes` stays valid.
- **Rationale:** `CONSTRAINTS.md`'s **Markdown Link Integrity** invariant is machine-enforced by `internal/lyxcwd/docslink_test.go`'s `TestEnforcement_MarkdownLinks`, and both `manifest/roadmap.md` and `manifest/designs/loom.md` link that anchor.
  Renaming the heading is a separate change that would have to update every inbound link in the same commit.
- **Applies to:** loomrecipe-regression-and-docs

### Decision: run-outcome-sentinel-vocabulary

- **Decision:** the persisted `RunState.Outcome` values are exactly the four `shuttleengine.Outcome` string values (`"done"`, `"asking"`, `"died"`, `"timeout"`) plus the one new sentinel `"running"`, declared as an unexported constant `runOutcomeRunning` in `internal/shuttleengine/rundir.go`.
  Anything else — including the empty string a pre-change `run.json` decodes to — is respawn-eligible and never attachable.
- **Rationale:** `attach-only-a-run-that-never-terminated`. Inverting the default (explicit `"running"` rather than "empty means attachable") is what keeps an in-flight worktree upgraded mid-`Asking` from attaching to an idle pane and waiting out a fresh 480-minute discussion timeout.
- **Applies to:** shuttle-await-operator-and-run-outcome, shuttle-attach

## All Files Touched

_Full union of every `Creates:` / `Edits:` / `Moves:` **target** path across every batch, sorted alphabetically (Move **source** paths are excluded — they disappear, like `Deletes:` tokens).
Cards are the source of truth;
this section is the input `_plan_validate.py`'s `all-files-touched-mismatch` check cross-references against the derived union of every card's `Edits:`/`Creates:`/Move-target paths, to catch drift between the hand/agent-maintained list here and that derived union._

- `docs/overview.md`
- `internal/loomcli/wiring.go`
- `internal/loomcli/wiring_test.go`
- `internal/loomengine/config.go`
- `internal/loomengine/config_test.go`
- `internal/loomengine/discussion.go`
- `internal/loomengine/discussion_test.go`
- `internal/loomengine/template.yaml`
- `internal/loomrecipe/fixture_test.go`
- `internal/loomrecipe/resume_test.go`
- `internal/shedadapters/doc.go`
- `internal/shedadapters/singlellm.go`
- `internal/shedadapters/singlellm_test.go`
- `internal/shedbuild/fixture_test.go`
- `internal/shedrecipe/fixture_test.go`
- `internal/shuttleengine/attach.go`
- `internal/shuttleengine/attach_test.go`
- `internal/shuttleengine/doc.go`
- `internal/shuttleengine/rundir.go`
- `internal/shuttleengine/run.go`
- `internal/shuttleengine/run_test.go`
- `internal/shuttleengine/spec.go`
- `internal/shuttleengine/spec_test.go`
- `internal/shuttleengine/wait.go`
- `internal/shuttleengine/wait_test.go`
- `manifest/designs/loom.md`
- `manifest/designs/shed.md`
- `manifest/roadmap.md`
