# Plan: Reconsider the collapsed strand strip default size

```yaml
task: "Reconsider the collapsed strand strip default size"
slug: "reed-collapsed-strip-readability"
approved: false
started: "20260831-102642"
parent: "main"
root: ""
verify: null
```

## Batch Index

_The fenced yaml block below is the authoritative DAG mill-go reads to schedule batches.
Every batch lives at `NN-<batch-slug>.md` in this directory and is mirrored as one entry here._

```yaml
batches:
  - number: 1
    name: collapsed-strip-default
    file: 01-collapsed-strip-default.md
    depends-on: []
    verify: go test ./internal/reedengine/...
```

## Shared Decisions

_Cross-cutting decisions every batch inherits: naming conventions, error-handling posture, test frameworks, style/lint constraints.
One subsection per decision.
Batch-local decisions live in each batch file._

### Decision: value-and-assertions-land-together

- **Decision:** the two template value changes, the two `config_test.go` default assertions, the two template inline comments, and the `doc.go` reword are ONE card and ONE commit.
- **Rationale:** flipping the `config_test.go` assertions to `6` without the template change leaves a red tree, and flipping the templates without the assertions does the same in the other direction.
  `CLAUDE.md`'s "docs land in the same commit" additionally binds the template comments and the `doc.go` reword to the value change, and `_mill/discussion.md`'s Constraints section restates that explicitly ("not in a follow-up").
  The TDD sequence in `_mill/discussion.md`'s Testing section (flip assertions, watch them fail, then change the templates) is an implementation ordering inside the card, not a commit boundary.
- **Applies to:** all batches

### Decision: no-render-or-configsync-change

- **Decision:** no file under `internal/reedengine/render/` and no file under `internal/configsync/` is edited, and no test outside `internal/reedengine/config_test.go` changes.
- **Rationale:** `internal/reedengine/render`'s `stackHeights`, `clampToFit`, `planCells`, and `FixedHeightPins` all treat `CollapsedStripRows` as an opaque absolute row budget and are magnitude-agnostic;
  `configsync`'s `ReconcileAll` is key-based by construction and deliberately never rewrites an existing key's value, which is the `no-value-migration` decision in `_mill/discussion.md`.
  Every `CollapsedStripRows: 2` in `apply_test.go`, `lock_test.go`, and the `render` package's tests is a deliberately-chosen unit input, not the template default;
  moving one would be a scope violation rather than a fix.
- **Applies to:** all batches

### Decision: integration-tier-is-a-landing-gate

- **Decision:** `go test -tags integration ./...` must run green before handoff, and a self-skip on a machine without the configured multiplexer satisfies the gate.
- **Rationale:** `internal/reedengine/attachgeometry_integration_test.go` asserts a collapsed parent pane's live height against `e.cfg.CollapsedStripRows` and its fixture writes `ConfigTemplate()` verbatim, so it starts exercising `6` against real tmux geometry the moment the template changes — it is the only check that can see a real-multiplexer clamp or rescale.
  No plan change is needed to enforce this: `pipeline.done_gate` in `mill-config.yaml` is already `go test ./... && go test -tags integration ./...`, so mill-go runs the tagged tier from `git_root` before marking the task done.
- **Applies to:** all batches

## All Files Touched

_Full union of every `Creates:` / `Edits:` / `Moves:` **target** path across every batch, sorted alphabetically (Move **source** paths are excluded — they disappear, like `Deletes:` tokens).
Cards are the source of truth;
this section is the input `_plan_validate.py`'s `all-files-touched-mismatch` check cross-references against the derived union of every card's `Edits:`/`Creates:`/Move-target paths, to catch drift between the hand/agent-maintained list here and that derived union._

- `internal/reedengine/config_test.go`
- `internal/reedengine/doc.go`
- `internal/reedengine/template_posix.yaml`
- `internal/reedengine/template_windows.yaml`
