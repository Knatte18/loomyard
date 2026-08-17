# Plan: planparser owns the plan-directory path

```yaml
task: "planparser owns the plan-directory path"
slug: "planparser-plan-dir"
approved: false
started: "20260817-144434"
parent: "standalone-producers"
root: ""
verify: null
```

## Batch Index

_The fenced yaml block below is the authoritative DAG mill-go reads to schedule batches.
Every batch lives at `NN-<batch-slug>.md` in this directory and is mirrored as one entry here._

```yaml
batches:
  - number: 1
    name: planparser-path-ownership
    file: 01-planparser-path-ownership.md
    depends-on: []
    verify: go test ./internal/planparser/...
  - number: 2
    name: repoint-and-delete-twins
    file: 02-repoint-and-delete-twins.md
    depends-on: [1]
    verify: go test ./internal/planparser/... ./internal/loomengine/... ./internal/webstercli/... ./internal/websterengine/... ./cmd/lyx/... && go test -tags integration ./internal/webstercli/...
```

## Shared Decisions

_Cross-cutting decisions every batch inherits: naming conventions, error-handling posture, test frameworks, style/lint constraints.
One subsection per decision.
Batch-local decisions live in each batch file._

### Decision: zero behaviour change, inspectable by construction

- **Decision:** Every path this task touches must resolve to the byte-identical string it resolves to today, in a `AnchorRel: "."` worktree and in a subpath-anchored one alike. `planparser.PlanDir`'s body is a character-for-character copy of `loomengine.PlanDir`'s current body with `l.AnchorPath()` replaced by the `anchorPath` parameter — nothing else.
- **Rationale:** The no-behaviour-change claim then reads off the diff instead of being argued from tests.
  Any rewrite of the join (e.g. routing through `PlanDirRel()`) would make the claim require reasoning about separator contracts.
- **Applies to:** all batches

### Decision: plain string parameter, no `*lyxcwd.Location`, no validation branch

- **Decision:** Both new functions are `func(anchorPath string) string` — pure `filepath.Join`, no error return, no guard on an empty or relative argument. `internal/planparser` does not import `internal/lyxcwd` today and must not start.
- **Rationale:** Per the Cwd Resolution Invariant, a module's own durable subdirectory is that module's own relative constant joined onto `AnchorPath()`; the caller resolves geometry and hands over a string.
  A validation branch would be the package's first error-returning path helper and would force error handling into `webstercli`'s pre-run and `loomengine.PlanSpec`, changing signatures the producers-standalone design pinned.
- **Applies to:** all batches

### Decision: anchor-always — every call site passes `AnchorPath()`, never `WorktreePath()`

- **Decision:** Every production call passes `l.AnchorPath()`.
  Neither `l.WorktreePath()`, nor a cwd, nor a git-root value is ever an acceptable argument.
  The rule is stated in both function doc comments and backed by two subpath-anchored test cases (the `loomengine.PlanSpec` case in card 6 and the `PersistentPreRunE` case in card 11).
- **Rationale:** The move trades a compile-time guarantee for a convention: today `loomengine.PlanDir(l)` anchors internally so no caller *can* get it wrong, whereas after the move passing `WorktreePath()` compiles, runs, and silently relocates the plan directory whenever `AnchorRel != "."`.
  The guard therefore has to move to the call sites, deliberately, in this task.
- **Applies to:** all batches

### Decision: card order inside batch 2 keeps every intermediate commit compiling

- **Decision:** Batch 2 repoints every caller onto `planparser.PlanDir`/`PlanOverview` *before* the card that deletes the `loomengine` twins.
  The deletion card (card 13) is the last code card in the batch.
- **Rationale:** Repointing a caller while the twins still exist compiles fine — the twins simply become unused exported functions for the length of a few commits.
  Deleting first would leave every commit between the deletion and the last repoint uncompilable.
  Ordering the cards this way costs nothing and keeps `git bisect` usable across the batch.
- **Applies to:** repoint-and-delete-twins

### Decision: the two `cmd/lyx` guard tables and the `cli_test.go` anchor flip are annotated as weakened, not counted as anchoring coverage

- **Decision:** `cmd/lyx/constructoranchoring_test.go`'s two plan rows, and `internal/webstercli/cli_test.go`'s `newTestCLI` anchor flip, both carry a comment stating precisely what they prove after this task (join arithmetic, `_lyx`-vs-`.lyx` group placement, nested-anchor path handling) and what they no longer prove (which root a production call site reaches for).
- **Rationale:** Both are self-consistent under a wrong root — the guard rows pass `AnchorPath()` in and compare against an `anchor`-derived expectation, and `newTestCLI` both computes `planDir` and seeds the plan into it — so a future reader must not over-trust them.
  Recording the weakening at the rows is cheaper than re-deriving it.
- **Applies to:** repoint-and-delete-twins

### Decision: `planparser.Validate`'s `worktreeRoot` parameter is left untouched

- **Decision:** Do not rename `Validate(plan *Plan, worktreeRoot string)` and do not change what any caller passes it, even though it sits in the same package as the new anchor-always wording.
- **Rationale:** Its two production callers pass different roots — `internal/webstercli/validate.go` passes `c.layout.AnchorPath()`, `internal/websterengine/runlevel.go` passes `deps.WorktreeRoot` — so at any `AnchorRel != "."` at most one of them is right.
  Which one is right is a plan-format semantics question, not a path-ownership question, and answering it changes behaviour.
  This task changes no behaviour.
- **Applies to:** all batches

### Decision: Go comment and markdown conventions

- **Decision:** Both new exported functions carry doc comments beginning with the function name and stating the anchor-always contract explicitly.
  Every `.md` file this task edits (`CONSTRAINTS.md`, `docs/overview.md`) uses semantic line breaks — one sentence per line, no fixed-column hard-wrap.
- **Rationale:** `golang:golang-comments` and the repo's markdown rule in `CLAUDE.md`.
- **Applies to:** all batches

### Decision: the tagged `webstercli` run is part of every batch-2 verify

- **Decision:** Batch 2's `verify:` chains `go test -tags integration ./internal/webstercli/...` after the untagged package list.
- **Rationale:** `internal/webstercli/verbs_test.go` carries `//go:build integration` on line 1, so neither `go test ./...` nor an untagged `go test ./internal/webstercli/...` compiles it.
  Three named in-scope edits live in that file (the call-site repoint, the `AnchorRel` flip, and the new subpath-anchored `PersistentPreRunE` case) and a green untagged run proves nothing about any of them.
- **Applies to:** repoint-and-delete-twins

## All Files Touched

_Full union of every `Creates:` / `Edits:` / `Moves:` **target** path across every batch, sorted alphabetically (Move **source** paths are excluded — they disappear, like `Deletes:` tokens).
Cards are the source of truth;
this section is the input `_plan_validate.py`'s `all-files-touched-mismatch` check cross-references against the derived union of every card's `Edits:`/`Creates:`/Move-target paths, to catch drift between the hand/agent-maintained list here and that derived union._

- `CONSTRAINTS.md`
- `cmd/lyx/constructoranchoring_test.go`
- `cmd/lyx/notransients_test.go`
- `docs/overview.md`
- `internal/loomengine/config.go`
- `internal/loomengine/plan.go`
- `internal/loomengine/plan_test.go`
- `internal/planparser/doc.go`
- `internal/planparser/parse.go`
- `internal/planparser/planpath_test.go`
- `internal/webstercli/cli.go`
- `internal/webstercli/cli_test.go`
- `internal/webstercli/verbs_test.go`
- `internal/websterengine/runlevel.go`
