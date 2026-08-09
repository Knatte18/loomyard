# Plan: builder: delete internal/builderengine and internal/buildercli, retire builder-contract.md as a reference

```yaml
task: 'builder: delete internal/builderengine and internal/buildercli, retire builder-contract.md as a reference'
slug: builder-retire
approved: false
started: '20260809-085916'
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
    name: code-retirement
    file: 01-code-retirement.md
    depends-on: []
    verify: go build ./... && go test ./cmd/lyx/... ./internal/configreg/... ./internal/configcli/... ./internal/loomengine/... ./internal/fabricengine/... ./internal/scoutcli/... ./internal/webstercli/... && go test -tags integration ./internal/webstercli/... ./internal/loomengine/...
  - number: 2
    name: comment-sweep
    file: 02-comment-sweep.md
    depends-on: [1]
    verify: go build ./... && go vet ./... && go test ./internal/websterengine/... ./internal/webstercli/... ./internal/pattern/... && go test -tags integration ./internal/websterengine/... ./internal/webstercli/...
  - number: 3
    name: reference-contracts
    file: 03-reference-contracts.md
    depends-on: [2]
    verify: null
  - number: 4
    name: module-phase-docs
    file: 04-module-phase-docs.md
    depends-on: [3]
    verify: null
  - number: 5
    name: spec-repair-acceptance
    file: 05-spec-repair-acceptance.md
    depends-on: [4]
    verify: go build ./... && go test ./... && go test -tags integration ./...
```

## Shared Decisions

_Cross-cutting decisions every batch inherits: naming conventions, error-handling posture, test frameworks, style/lint constraints.
One subsection per decision.
Batch-local decisions live in each batch file._

### Decision: no-intermediate-broken-build

- **Decision:** the deletion of `internal/builderengine` + `internal/buildercli`, the unregistration from `cmd/lyx/main.go` and `internal/configreg`, every direct importer's repair, and the sandbox-suite retirement all land in **one commit** (batch 1, card 1).
  Every other card in the plan is separately committable and leaves the tree building and green on its own.
- **Rationale:** a package deletion is atomic by nature — splitting it guarantees an intermediate commit that does not build.
  The discussion's "one task producing one compiling commit" rule exists to forbid a non-compiling intermediate state, and that is what card 1's atomicity guarantees.
  The remaining cards are comment rewrites and documentation, none of which can break a build or a test, so folding them into card 1 would produce one unreviewably large diff for no correctness gain.
  Note this is a deliberate reading of the discussion's wording: the task produces several commits, each of which compiles, rather than literally one commit.
- **Applies to:** all batches

### Decision: sweep-completion-is-a-grep-not-a-judgment

- **Decision:** the task is complete when six repo-wide grep patterns return zero hits against an explicitly enumerated exclusion list, not when a per-site inventory is exhausted.
  The patterns and the full exclusion list live in batch 5, card 18.
- **Rationale:** a partial sweep has no completion criterion and degrades into a per-site judgment call.
  A zero-hit grep converts "did we get everything" from an opinion into a test.
  The package-name patterns alone are insufficient — the largest swept class, the provenance doc-comments in `websterengine`/`webstercli`, writes plain "builder", which only the bare-word pattern sees.
- **Applies to:** all batches

### Decision: mechanical-rename-vs-hand-rewrite

- **Decision:** the phase/gate token rename `builder` → `webster` is a true rename and may be done mechanically.
  The provenance comments and the falsified-claim prose are rewrites and are done by hand, one sentence at a time.
  Both halves are gated by the same zero-hit grep.
- **Rationale:** no regex turns "mirroring builderengine's own runlevel.go naming note" into standalone prose;
  scripting that half produces nonsense that needs a full hand pass anyway.
  The shared grep gate gives both halves the same completion criterion regardless of how they were produced.
  Per this repo's tooling rules, any script used for the rename must not use `sed`.
- **Applies to:** code-retirement, comment-sweep, module-phase-docs

### Decision: provenance-comments-rewritten-to-stand-alone

- **Decision:** every doc-comment explaining a webster mechanism by reference to builder's is rewritten to state its reason directly, dropping the builder reference — never deleted down to a fragment, and never marked historical ("the since-deleted builderengine").
- **Rationale:** several comments exist almost entirely to state the derivation and would explain nothing if the clause were merely cut;
  marking them historical keeps a deleted package's name in ~40 places, which is exactly what the sweep removes.
  Comments citing the `builder-is-frozen-copy-not-move` Shared Decision need their **premise** replaced, since this task falsifies that decision outright.
- **Applies to:** comment-sweep

### Decision: link-repair-scoped-to-permanently-deleted-files

- **Decision:** wherever this task deletes a file that does not come back, it repairs every inbound link to it — even in another task's territory — and touches nothing else on those lines. `docs/reference/builder-contract.md` is the only such file.
  Every inbound `docs/reference/plan-format.md` link is left **dangling on purpose**.
- **Rationale:** `plan-format.md` returns under the same name two tasks later, so repairing those links would retarget them at `plan-format-v3.md`, which the next task renames back to `plan-format.md` — churning every link twice to land where it started.
  Several of those sentences also assert that v2 exists *as distinct from* v3, so retargeting makes them self-duplicating rather than merely stale.
  The dangling window is recorded as designed behaviour in `manifest/designs/shed-followups.md`, so no override is filed for it.
  The **prose** asserting v2 still exists is a separate matter and is this task's under the v2-coexistence class — the link mechanics differ, the prose ownership does not.
- **Applies to:** reference-contracts, module-phase-docs

### Decision: phase-rename-includes-the-live-enum

- **Decision:** the loom phase rename `builder` → `webster` includes `internal/loomengine/coherence.go`'s `validPhases` map and its `docs/reference/status-schema.md` enum twin, overriding `shed-followups.md`'s deferral of both to a later build task.
  The resulting break for an on-disk `status.json` carrying `phase: "builder"` is **accepted, not handled** — no shim, no read-time migration.
- **Rationale:** renaming the prose while leaving the validator would ship docs saying "Webster phase" against live code that rejects `phase: "webster"` — internally contradictory, and worse than either extreme.
  The accepted break rests on a stated, falsifiable assumption: `lyx loom` is unbuilt, so no real `status.json` with `phase: "builder"` exists outside test fixtures.
  If the implementer finds one in a live worktree, that invalidates the assumption and is a finding to report, not a case to quietly handle.
- **Applies to:** code-retirement, module-phase-docs

### Decision: shed-followups-is-a-live-spec

- **Decision:** `manifest/designs/shed-followups.md` is edited in this task to record four ownership overrides and to repair two instructions this task invalidated.
  It is a named exclusion in the acceptance grep.
- **Rationale:** the file is the specification five downstream tasks execute against, not documentation.
  A stale file inventory or a reference to a deleted grounding doc is a live defect in another task's instructions, not documentation drift.
- **Applies to:** spec-repair-acceptance

### Decision: no-new-tests

- **Decision:** no new tests are written anywhere in this task.
- **Rationale:** the existing suite is the test, and four guards fail loudly on a half-removal: `cmd/lyx/helptree_test.go` (orphaned or half-unregistered command), `cmd/lyx/notransients_test.go` (leftover transient case), `internal/configreg/configreg_test.go` (module-list drift), and `cmd/lyx/sandbox_coverage_test.go` (a `**Covers:**` tag naming an unregistered module).
  `internal/configcli/configcli_test.go` and `internal/loomengine/preflight_integration_test.go` are the two non-obvious additions — the first fails without ever importing builder, the second on any divergence between `validPhases` and its fixtures.
- **Applies to:** all batches

## All Files Touched

_Full union of every `Creates:` / `Edits:` / `Moves:` **target** path across every batch, sorted alphabetically (Move **source** paths are excluded — they disappear, like `Deletes:` tokens).
Cards are the source of truth;
this section is the input `_plan_validate.py`'s `all-files-touched-mismatch` check cross-references against the derived union of every card's `Edits:`/`Creates:`/Move-target paths, to catch drift between the hand/agent-maintained list here and that derived union._

- `.gitattributes`
- `CONSTRAINTS.md`
- `README.md`
- `cmd/lyx/constructoranchoring_test.go`
- `cmd/lyx/helptree_test.go`
- `cmd/lyx/main.go`
- `cmd/lyx/notransients_test.go`
- `cmd/lyx/rawgitmutation_test.go`
- `docs/overview.md`
- `docs/reference/discussion-format.md`
- `docs/reference/model-spec.md`
- `docs/reference/plan-format-v3.md`
- `docs/reference/status-schema.md`
- `docs/reference/webster-contract.md`
- `docs/sandbox-howto.md`
- `docs/sandbox-hub.md`
- `docs/skills.md`
- `internal/configcli/configcli_test.go`
- `internal/configreg/configreg.go`
- `internal/configreg/configreg_test.go`
- `internal/fabricengine/trailer_test.go`
- `internal/fabricengine/weftgit_exclude_test.go`
- `internal/loomengine/coherence.go`
- `internal/loomengine/config_test.go`
- `internal/loomengine/configtemplate.go`
- `internal/loomengine/preflight_integration_test.go`
- `internal/modelspec/modelspec.go`
- `internal/pattern/doc.go`
- `internal/pattern/leaf_enforcement_test.go`
- `internal/perchengine/doc.go`
- `internal/planparser/validate.go`
- `internal/reedengine/headertemplate.go`
- `internal/scoutcli/cli.go`
- `internal/scoutcli/cli_test.go`
- `internal/scoutengine/doc.go`
- `internal/webstercli/beginbatch.go`
- `internal/webstercli/cli.go`
- `internal/webstercli/cli_test.go`
- `internal/webstercli/pause.go`
- `internal/webstercli/run.go`
- `internal/webstercli/status.go`
- `internal/webstercli/sync_integration_test.go`
- `internal/webstercli/validate.go`
- `internal/webstercli/verbs_test.go`
- `internal/websterengine/archive.go`
- `internal/websterengine/audit.go`
- `internal/websterengine/beginbatch.go`
- `internal/websterengine/beginbatch_test.go`
- `internal/websterengine/classify.go`
- `internal/websterengine/config.go`
- `internal/websterengine/config_test.go`
- `internal/websterengine/digest.go`
- `internal/websterengine/doc.go`
- `internal/websterengine/fingerprint.go`
- `internal/websterengine/gitwrap.go`
- `internal/websterengine/outcome.go`
- `internal/websterengine/pause.go`
- `internal/websterengine/poll.go`
- `internal/websterengine/recordbatch.go`
- `internal/websterengine/recoverbatch.go`
- `internal/websterengine/recoverbatch_test.go`
- `internal/websterengine/render.go`
- `internal/websterengine/report.go`
- `internal/websterengine/roles.go`
- `internal/websterengine/roles_test.go`
- `internal/websterengine/runlevel.go`
- `internal/websterengine/state.go`
- `internal/websterengine/state_test.go`
- `internal/websterengine/strand.go`
- `internal/websterengine/summary_test.go`
- `internal/websterengine/template.go`
- `internal/websterengine/template_test.go`
- `manifest/designs/finalize.md`
- `manifest/designs/hardener.md`
- `manifest/designs/loom.md`
- `manifest/designs/raddle.md`
- `manifest/designs/review-finding-classification.md`
- `manifest/designs/self-report.md`
- `manifest/designs/shed-followups.md`
- `manifest/roadmap.md`
- `tools/sandbox/SANDBOX-CORE-SUITE.md`
- `tools/sandbox/SANDBOX-WEBSTER-SUITE.md`
- `tools/sandbox/main.go`
- `tools/sandbox/suite.go`
