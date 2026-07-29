# Plan: burler: split the round prompt into an orchestrator + three instruction files

```yaml
task: 'burler: split the round prompt into an orchestrator + three instruction files'
slug: burler-prompt-split
approved: true
started: '20260729-192844'
parent: main
root: ""
verify: null
```

## Batch Index

_The fenced yaml block below is the authoritative DAG mill-go reads to schedule batches. Every batch lives at `NN-<batch-slug>.md` in this directory and is mirrored as one entry here._

```yaml
batches:
  - number: 1
    name: split-round-prompt
    file: 01-split-round-prompt.md
    depends-on: []
    verify: go test ./internal/burlerengine/... ./internal/stencil/...
```

## Shared Decisions

### Decision: Single atomic batch

- **Decision:** The entire refactor — the four template assets, `template.go`'s embeds, `composePrompt`'s new signature, `Engine.Run`'s materialization, every test update, and both doc updates — is ONE batch (nine cards), not split across batches.
- **Rationale:** `composePrompt` changes signature (from returning one string to returning an orchestrator string plus three `(path, content)` instruction pairs). The moment that lands, every caller and every test that calls it stops compiling. `Engine.Run`, `prompt_test.go`, `template_test.go`, and `engine_test.go` must all move in lockstep or the package will not build and `verify` (a `go test`) fails. Go compilation coupling makes a cross-batch split impossible here. Cards are the logical unit; `verify` runs only at the batch boundary, so intermediate card states need not individually compile.
- **Applies to:** all batches

### Decision: Docs land in this batch (mill commits per card)

- **Decision:** `doc.go` and `CONSTRAINTS.md` updates are cards 8 and 9 of this same batch. The project's "docs land in the same commit" rule is realized here as "same batch/plan", because mill's card model commits each card separately — there is no single git commit spanning the code and its docs.
- **Rationale:** The Documentation Lifecycle invariant (`CONSTRAINTS.md`) requires the module doc and any touched invariant to update alongside the change, never deferred to a later task. Same-batch cards satisfy that intent within mill's per-card commit model.
- **Applies to:** all batches

### Decision: `DotLyxDir()` is Cwd-anchored — tests must set `Layout.Cwd`

- **Decision:** Every `*hubgeometry.Layout` a test constructs for `Engine.Run` must set `Cwd` to a per-test `t.TempDir()`, not only `WorktreeRoot`.
- **Rationale:** `(*Layout).DotLyxDir()` returns `filepath.Join(l.Cwd, ".lyx")` (`internal/hubgeometry/hubgeometry.go:472-474`) — Cwd-anchored, not WorktreeRoot-anchored. After this change `Engine.Run` calls `os.MkdirAll` under `layout.DotLyxDir()`. The existing test helpers build `&hubgeometry.Layout{WorktreeRoot: root}` with `Cwd` unset, so `DotLyxDir()` would resolve to `.lyx` relative to the test process's working directory — writing `.lyx/burler/round-*` into the real package source tree on every engine test. Setting `Cwd: root` scopes materialization to the temp dir.
- **Applies to:** split-round-prompt

### Decision: Split, not rename — no `Moves:`

- **Decision:** The monolith `review-prompt-template.md` is expressed as one `Deletes:` plus four `Creates:` (the four new assets), never as a `Moves:` pair.
- **Rationale:** No single file is relocated 1:1; the monolith's sections are redistributed across four new files, none of which is "the monolith renamed". `Moves:` is only for a 1:1 relocation, so this batch has no `Moves:` and no `## Rename mechanic` section.
- **Applies to:** split-round-prompt

### Decision: Guard-test tokens are disjoint from the retained framing

- **Decision:** The orchestrator legitimately keeps the two-jobs A/B framing including a one-line job-B summary ("even if the verdict was APPROVED — non-blocking polish still gets fixed"). The orchestrator guard test asserts absence of tokens that appear ONLY in the downstream bodies: instruction 3's `"not whether it gets fixed"`; instruction 2's review-file YAML keys in colon form `"verdict:"` and `"findings:"`; and instruction 2's cluster fork-spawn phrasing `"SINGLE message"` and `"subagent_type"`. It must NOT assert on the generic phrase "fix-everything", which would collide with the retained framing.
- **Rationale:** The colon form (`verdict:`) is chosen precisely so the job-B one-liner's bare word "verdict" does not false-fail the guard; a regression that inlines a downstream body back into the orchestrator reintroduces at least one disjoint token.
- **Applies to:** split-round-prompt

## All Files Touched

- `.gitattributes`
- `CONSTRAINTS.md`
- `docs/shared-libs/stencil.md`
- `internal/burlerengine/doc.go`
- `internal/burlerengine/engine.go`
- `internal/burlerengine/engine_test.go`
- `internal/burlerengine/instruction-1-explore-template.md`
- `internal/burlerengine/instruction-2-review-template.md`
- `internal/burlerengine/instruction-3-fix-template.md`
- `internal/burlerengine/prompt.go`
- `internal/burlerengine/prompt_test.go`
- `internal/burlerengine/round-orchestrator-template.md`
- `internal/burlerengine/template.go`
- `internal/burlerengine/template_test.go`
