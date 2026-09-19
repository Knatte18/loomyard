# Plan: reed: extract Selvage-pane lifecycle

```yaml
task: 'reed: extract Selvage-pane lifecycle'
slug: reed-selvage-pane-extraction
approved: true
started: '20260919-095315'
parent: 'main'
root: ""
verify: go vet ./internal/reedengine/...
discussion_sha: e780a508d3b4827473114637fb841f39bfe6548d
```

## Batch Index

_The fenced yaml block below is the authoritative DAG mill-go reads to schedule batches.
Every batch lives at `NN-<batch-slug>.md` in this directory and is mirrored as one entry here._

```yaml
batches:
  - number: 1
    name: selvagepane-extraction
    file: 01-selvagepane-extraction.md
    depends-on: []
    verify: go test ./internal/reedengine/
  - number: 2
    name: docs-and-full-verification
    file: 02-docs-and-full-verification.md
    depends-on: [1]
    verify: go test ./internal/reedengine/
```

## Shared Decisions

### Decision: pure-refactor-no-behavior-change

- **Decision:** no observable behavior change anywhere in this task.
  Same tmux calls, same order, same error strings, same log messages, same doc-comment prose on every moved function.
  No opportunistic fix is folded in, however small or however obviously correct it looks along the way.
- **Rationale:** it keeps the diff reviewable as a move rather than a rewrite, and it keeps the package's existing suite usable as the regression net — including `internal/reedcli`'s `smoke` tier, which drives the CLI end-to-end against a live tmux server and is expected to pass unedited.
  A behavior fix discovered along the way is a separate roadmap item, filed rather than folded in.
- **Applies to:** all batches

### Decision: seam-naming-under-the-enforcement-check

- **Decision:** the enforcement test added in card 1 flags any AST identifier containing `selvage` case-insensitively outside `selvagepane.go` / `state.go` / `config.go`, exempting only call-expression function positions and composite-literal field keys.
  Every name this plan introduces is chosen to satisfy that predicate without needing a third exemption.
  A helper's own name may carry `Selvage`, because a host only ever names it in a call position: `clearSelvagePaneBinding`, `seedSelvageClaim`, `selvageRenderParams`, `ensureSelvagePaneLocked`.
  A name a host holds in a non-call position carries no `selvage` at all: the reconcile-policy type is `reapPolicy`, its constructor `newReapPolicy`, its three methods `exemptFromDeadKill` / `exemptFromUntrackedReap` / `authorizesReap`, and the split-target seam's new return value is `insertAbove`.
  No member of a value a host file holds carries `selvage` in its name, field or method alike, even where a call position would technically exempt it.
- **Rationale:** the predicate has to be satisfiable by the end state, or it condemns the design it protects.
  Naming the policy type or any of its host-visible members for Selvage would re-announce Selvage in the four files this task exists to clear, in exactly the positions the check has no exemption for.
- **Applies to:** all batches

### Decision: seam-helpers-take-state-never-a-pane-id

- **Decision:** every seam helper takes `*ReedState` and, where it needs config, is an `Engine` method.
  No host file reads `st.SelvagePaneID`, reads `e.cfg.Selvage`, or constructs a `render.Selvage` value to hand in.
- **Rationale:** a seam taking a bare pane id would leave the very field selector the enforcement check bans at every call site, which would force allowlisting `reconcile.go`, `spawn.go`, `apply.go` and `generation.go` — i.e. allowlisting the scatter this task exists to remove.
- **Applies to:** all batches

### Decision: comments-move-verbatim-with-their-code

- **Decision:** every explanatory comment moves with the code it explains, verbatim, and is never paraphrased, shortened, or summarised into the new file's own file-level doc comment.
  This binds three blocks in particular: `reconcile.go`'s reasoning for why a dead-but-present Selvage is spared the dead-pane kill (the tmux 3.6 stale-layout-cell hazard), its reasoning for why presence-exemption and aliveness-authorization stay three separate questions, and `splitSelvagePaneAtBottomLocked`'s reasoning for the even-vertical retry.
- **Rationale:** those comments record live-verified hazards, each traceable to a named review finding.
  A move that paraphrases them loses the evidence that made them load-bearing, and the next reader re-derives the hazard by hitting it.
- **Applies to:** selvagepane-extraction

### Decision: one-batch-for-the-whole-extraction

- **Decision:** the enforcement test, the new file, all four host rewires, and the test moves land as a single batch rather than as a sequence of smaller ones.
- **Rationale:** two independent constraints force it.
  Go requires a moved function and its changed call sites in the same compilable unit, so the seam changes cannot be split across a batch boundary.
  And the enforcement test only goes green once every one of the four host files is cleared, so any earlier batch boundary would end red.
  Card 1 is deliberately committed red — its commit message records the pre-extraction violation list as the evidence that the check can actually detect the scatter — and the batch ends green once the later cards land.
- **Applies to:** all batches

### Decision: go-project-verify-shape

- **Decision:** `verify:` commands use the native Go test runner with no `PYTHONPATH=` prefix, and are scoped to `./internal/reedengine/` rather than the repository.
  Every build needs `CGO_ENABLED=1` and a C compiler on `PATH`, which is already the native default on this machine.
- **Rationale:** this is a Go module, not a Python one, so the `PYTHONPATH=` isolation prefix has nothing to reset.
  The task is confined to one package's unexported surface, so a package-scoped run is the whole affected blast radius; the repo-wide and integration tiers are covered by the configured `pipeline.done_gate`, and the `smoke` tier by card 14.
- **Applies to:** all batches

### Decision: design-doc-is-kept-with-a-status-section

- **Decision:** `manifest/designs/reed-selvage-pane-extraction.md` survives this task and gains a `## Status: Implemented` section, rather than being deleted on landing.
- **Rationale:** `docs/overview.md`'s Documentation Lifecycle says a module-design doc is deleted when its module lands, and read literally that would delete this file.
  It is cited here because it is the closest rule and a reviewer will reach for it.
  It does not govern this file: `internal/reedengine` landed long ago, and this doc records a post-merge audit of one follow-up item rather than a not-yet-built module's design.
  The live precedent in the same directory is `reed-header-selvage.md`, a shipped item whose doc is kept and carries its own `Status: Implemented` section.
  The audit numbers are the durable artefact — the whole task is measured against them — so deleting the file would delete the only before-state the after-state can be compared to.
- **Applies to:** docs-and-full-verification

## All Files Touched

- `internal/reedengine/apply.go`
- `internal/reedengine/doc.go`
- `internal/reedengine/generation.go`
- `internal/reedengine/lifecycle.go`
- `internal/reedengine/lifecycle_test.go`
- `internal/reedengine/reconcile.go`
- `internal/reedengine/reconcile_test.go`
- `internal/reedengine/selvagepane.go`
- `internal/reedengine/selvagepane_enforcement_test.go`
- `internal/reedengine/selvagepane_test.go`
- `internal/reedengine/spawn.go`
- `internal/reedengine/spawn_test.go`
- `manifest/designs/reed-header-selvage.md`
- `manifest/designs/reed-selvage-pane-extraction.md`
- `manifest/roadmap.md`
