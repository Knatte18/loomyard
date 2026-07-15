# Plan: Reconsider whether lyx mux needs anchor:top at all

```yaml
task: "Reconsider whether lyx mux needs anchor:top at all"
slug: mux-anchor-top-redesign
approved: false
started: 20260715-090830
parent: cluster-fork-spike
root: ""
verify: null
```

## Batch Index

_The fenced yaml block below is the authoritative DAG mill-go reads to
schedule batches. Every batch lives at `NN-<batch-slug>.md` in this
directory and is mirrored as one entry here._

```yaml
batches:
  - number: 1
    name: dereference-consumers
    file: 01-dereference-consumers.md
    depends-on: []
    verify: go build ./... && go test ./internal/muxengine/... ./internal/muxcli/... ./internal/shuttleengine/... && go test -tags smoke -run '^$' ./internal/muxcli/
  - number: 2
    name: delete-render-config-defs
    file: 02-delete-render-config-defs.md
    depends-on: [1]
    verify: go build ./... && go test ./internal/muxengine/...
  - number: 3
    name: docs-and-sandbox
    file: 03-docs-and-sandbox.md
    depends-on: [2]
    verify: go build ./tools/sandbox/...
```

## Shared Decisions

### Decision: verdict is full removal of anchor:top

- **Decision:** `anchor:top` and all `TopBandRows` config/override/flag surface are
  removed outright — not deprecated, not narrowed. The `Anchor` type survives with three
  members (`below-parent`, `hidden`, `own-window`-deferred); only `top` is deleted.
- **Rationale:** Settled in `_mill/discussion.md` (Decisions `remove-not-deprecate-not-narrow`,
  `childless-full-height-is-acceptable`). `below-parent` + `ShrinkWhenWaitingOnChild` (the
  unconditional default on every `lyx mux add`) already covers the mother/child shape for
  Lyx's single-rooted-per-window topology; nothing in production emits `--anchor top`.
- **Applies to:** all batches

### Decision: replace-then-delete staging keeps every batch green

- **Decision:** Batch 1 removes every *reference* to `render.AnchorTop`,
  `render.Display.TopBandRows`, `render.Params.TopBandRows`, and `muxengine.Config.TopBandRows`
  (migrating consumer fixtures to `below-parent`, deleting top-band test cases, dropping the
  CLI flag) while the symbol *definitions* still exist. Batch 2 deletes the now-unreferenced
  definitions and the top-band render logic. Batch 3 is docs + sandbox.
- **Rationale:** mill's per-batch green-build invariant forbids deleting `render.AnchorTop` in
  one batch while another still references it. The two code batches touch **disjoint file
  sets** (verified: batch 1 edits only consumers/tests + `add.go`/`strand.go`/`apply.go`;
  batch 2 edits only `render/{types,policy,rules}.go` + `config.go` + `template.go` + the two
  template yamls), so no file is edited by both — each batch compiles and tests green on its own.
- **Applies to:** dereference-consumers, delete-render-config-defs

### Decision: user-facing strings sweep with their own file's code edits

- **Decision:** The `top`/`top-band` reference sweep mandated by discussion runs where each
  string lives: the two user-facing `add.go` strings (invalid-anchor error → `want
  below-parent|hidden`; `--anchor` flag usage → `placement: below-parent|hidden`) and
  `strand.go`'s `validateAnchor` error string are updated in **batch 1** alongside those files'
  code edits; the internal doc comments in `render/{types,policy,rules}.go` and
  `template.go`'s config-key-list comment are swept in **batch 2** alongside their code edits.
  Every `top`/`top-band` reference is removed across the two code batches — no dead surface
  survives (discussion Decision `keep-partitionByAnchor-simplified` + the round-1 review NOTE
  fix).
- **Rationale:** Splitting a single file across batches would create edit overlap; co-locating
  each string with its file's code edit preserves the disjoint-file-set property above while
  still satisfying "sweep everything".
- **Applies to:** dereference-consumers, delete-render-config-defs

### Decision: Go-native verify, no repo-wide done-gate config mutation

- **Decision:** `verify:` commands use the native Go runner (no `PYTHONPATH=` prefix — that
  shape rule is Python-only). No `pipeline.done_gate` is added, because setting it would mutate
  `mill-config.yaml` mid-task (the `wiki-config-mutation` validator guards exactly this).
  Instead each code batch's `verify:` runs `go build ./...` (catches every cross-package
  compile regression from the symbol removal) plus the affected packages' tests.
- **Rationale:** `go build ./...` over the whole module is the cheap cross-package regression
  catch this removal needs; a config mutation is not worth the friction.
- **Applies to:** all batches

### Decision: smoke tests are build-tagged; compile-check only in this repo

- **Decision:** `internal/muxcli/smoke_*_test.go` are `//go:build smoke` tagged and drive a
  live psmux server, which is not available here. Batch 1 rewrites `smoke_lifecycle_test.go`'s
  `--anchor top` usage; its verify confirms the rewritten file still **compiles** under the tag
  via `go test -tags smoke -run '^$' ./internal/muxcli/` (compiles smoke files, runs zero
  tests, needs no psmux). Real live-psmux confirmation of the new below-parent behavior is the
  operator-run sandbox scenario M18 (batch 3), out-of-band — not a mill-go gate (discussion
  Decision `empirical-test-as-operator-run-sandbox-scenario`).
- **Applies to:** dereference-consumers, docs-and-sandbox

### Decision: no renames anywhere

- **Decision:** This is a pure edit/removal task. No `Moves:` in any card; no `## Rename
  mechanic` section in any batch.
- **Applies to:** all batches

### Decision: docs/overview.md needs no edit

- **Decision:** `docs/overview.md` was checked (lines 280, 327): it mentions `anchor` only
  generically (`anchor / focus / shrinkWhenWaitingOnChild`) and references the unrelated deferred
  `own-window` anchoring milestone. Neither enumerates `top`, so `overview.md` is **not** in the
  plan.
- **Rationale:** Avoid a no-op card; the generic mention stays valid after `top` is removed.
- **Applies to:** docs-and-sandbox

## All Files Touched

- `docs/modules/loom.md`
- `docs/reviews/mux-review-prompt.md`
- `internal/muxcli/add.go`
- `internal/muxcli/smoke_lifecycle_test.go`
- `internal/muxengine/apply.go`
- `internal/muxengine/apply_test.go`
- `internal/muxengine/config.go`
- `internal/muxengine/config_test.go`
- `internal/muxengine/contract_integration_test.go`
- `internal/muxengine/io_test.go`
- `internal/muxengine/lifecycle_test.go`
- `internal/muxengine/lock_test.go`
- `internal/muxengine/render/policy.go`
- `internal/muxengine/render/policy_test.go`
- `internal/muxengine/render/rules.go`
- `internal/muxengine/render/rules_test.go`
- `internal/muxengine/render/types.go`
- `internal/muxengine/state_test.go`
- `internal/muxengine/strand.go`
- `internal/muxengine/strand_test.go`
- `internal/muxengine/template.go`
- `internal/muxengine/template_posix.yaml`
- `internal/muxengine/template_windows.yaml`
- `internal/shuttleengine/run_test.go`
- `internal/shuttleengine/spec_test.go`
- `tools/sandbox/SANDBOX-MUX-SUITE.md`
