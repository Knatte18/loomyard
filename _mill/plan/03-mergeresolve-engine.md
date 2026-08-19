# Batch: mergeresolve engine

```yaml
task: 'landing: Publish + Finalize producers'
batch: 'mergeresolve engine'
number: 3
cards: 8
verify: go test ./internal/mergeresolve/... ./contracts/stencils/... ./internal/lyxcwd/...
depends-on: [1]
```

## Batch Scope

`internal/mergeresolve` — the shared merge-in plus conflict-resolution engine both producers call and neither owns.
It wraps the Fabric merge surface behind a narrow five-method seam, spawns a fresh LLM session on conflict through its own one-method shuttle seam, verifies the resolution mechanically by re-scanning for conflict markers, and either stages-and-concludes or aborts and reports stuck.
It is one batch because the marker scan, the spec builder, and the orchestration are a single decision table that only makes sense read together, and the package's whole point is that the table is exhaustive.
It carries the conflict stencil with it because the stencil registry test fails in both directions — an embedded-but-unregistered file and a registered-but-absent name both break the build — so file and registration must land together.

It depends on batch 1 for `fabricengine.MergeStageResolved` and `fabricengine.StageResult`, which the merge seam names.

The external interface batch 4 consumes is `mergeresolve.New(Deps) (*Resolver, error)` and `(*Resolver).Resolve(ctx, source) (Result, error)`.

Batch-local decisions beyond `## Shared Decisions`:

- **This package is not a producer and has no `shedengine` import at all.** It drives `shuttleengine.Runner` through its own narrow seam rather than through `shedadapters.SingleLLMProducer`, because that adapter *is* a `shedengine.ShedProducer` and this package has no producer seam to satisfy. The narrow seam also keeps the package unit-testable against a fake with no `shedengine` dependency.
- **`Reason` is a returned string, never a log line or a file.** Writing the stuck reason to disk and logging it are the producer's job (batch 4), because the producer is what knows its own name and its own scratch path conventions.

## Cards

### Card 12: package skeleton — Deps, seams, Result

- **Context:**
  - `internal/fabricengine/merge.go`
  - `internal/fabricengine/mergestage.go`
  - `internal/fabricengine/mergelifecycle.go`
  - `internal/fabricengine/mergeerrors.go`
  - `internal/shedadapters/singlellm.go`
  - `internal/shedadapters/doc.go`
  - `internal/loomshed/doc.go`
  - `internal/modelspec/modelspec.go`
- **Edits:** none
- **Creates:**
  - `internal/mergeresolve/doc.go`
  - `internal/mergeresolve/deps.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** Create the package with its documentation and its told-value surface.

  `internal/mergeresolve/doc.go` carries the `Package mergeresolve` godoc: it states that the package merges a source branch into the current pair and resolves any resulting conflicts through a fresh LLM session, that it takes told absolute paths and has no direct production import of `internal/lyxcwd` per the Told-Geometry Invariant, and that it is a plain package rather than a `ShedProducer`.
  The doc describes **one repo** throughout — this package is not in the Fabric Vocabulary Invariant's owner set, so its identifiers, string literals, and comments may not carry either fabric-internal side's name, and that ban is machine-enforced by an AST walk over every identifier.

  `internal/mergeresolve/deps.go` declares:

  - `MergeSurface`, the narrow five-method seam over the Fabric merge verbs: `MergeIn(source string) (fabricengine.MergeResult, error)`, `MergeStageResolved(paths []string) (fabricengine.StageResult, error)`, `MergeContinue(msg string) (fabricengine.MergeResult, error)`, `MergeAbort() (fabricengine.MergeResult, error)`, and `MergeInProgress() (bool, error)`.
    Add a compile-time assertion that `*fabricengine.Fabric` satisfies it.
    Every verb named here is vocabulary-free by construction, which is why the seam can exist in this package at all.
  - `Shuttle`, the one-method seam `Run(shuttleengine.Spec) (shuttleengine.Result, error)`, with a compile-time assertion that `*shuttleengine.Runner` satisfies it — the same shape `shedadapters.Shuttle` already uses.
  - `Deps`, carrying every told value: `Fabric MergeSurface`, `Shuttle Shuttle`, `WorktreeRoot string` (the absolute root the conflict paths are relative to, needed for the marker scan's own file reads), `ScratchDir string` (the told absolute scratch directory the resolution report is written into), `StencilsDir string`, `ConflictSpec string` (the model-spec string), `Registry modelspec.Registry`, and `Timeout time.Duration`.
    Every field's doc comment states it is told by the caller and derived by nobody here.
  - `Outcome`, a string type with exactly two values, `OutcomeResolved` and `OutcomeStuck`; and `Result`, carrying `Outcome Outcome`, `Reason string`, and `AlreadyUpToDate bool`.
    `Reason` is documented as the human-facing explanation the calling producer logs and files — this package never writes it anywhere itself.
  - `Resolver`, the constructed type, and `func New(deps Deps) (*Resolver, error)`, which rejects a nil `Fabric`, a nil `Shuttle`, and an empty `WorktreeRoot`, `ScratchDir`, or `StencilsDir` up front with a distinct error each, rather than nil-panicking or writing to the wrong place at call time.
- **Commit:** `feat(mergeresolve): add package skeleton, seams, and told-value Deps`

### Card 13: the conflict-marker scan

- **Context:**
  - `internal/mergeresolve/deps.go`
- **Edits:** none
- **Creates:**
  - `internal/mergeresolve/markers.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** Create `internal/mergeresolve/markers.go` declaring an unexported `scanUnresolved(worktreeRoot string, paths []string) (unresolved []string, err error)`.

  For each entry in `paths` it joins the entry onto `worktreeRoot` and reads the file.
  Behaviour, exactly:

  - a file that no longer exists is **resolved by deletion**, not a failure: skip the marker scan for it and do not add it to `unresolved`.
    `ConflictedFiles()` includes delete/modify conflicts whose correct resolution is that the file is gone, and the caller still passes such a path to the staging verb, which stages the removal.
  - a read error that is *not* a not-exist error is a genuine failure: return it wrapped, distinct from the deletion case.
  - otherwise scan the content line by line and treat the file as still unresolved when any line **starts with** one of the three markers: the seven-character less-than run followed by a space, the seven-character equals run alone, and the seven-character greater-than run followed by a space.
    The match is content-only and line-anchored — never a substring match anywhere in a line.

  Document the consequence rather than hiding it: a resolved file whose own legitimate content carries line-start conflict markers can never pass this scan, and its merge escalates to stuck.
  That is the safe direction, because the conclude step is irreversible at the Fabric layer, and such a file is out of scope for automated resolution.
  Also document the layering: this scan is only a pre-check, and the Fabric-level index guard remains the authoritative gate — the scan can make this package refuse something Fabric would have accepted, never the reverse.
- **Commit:** `feat(mergeresolve): add line-anchored conflict-marker scan`

### Card 14: the conflict session spec builder

- **Context:**
  - `internal/mergeresolve/deps.go`
  - `internal/loomengine/discussion.go`
  - `internal/loomengine/prompt.go`
  - `internal/shuttleengine/spec.go`
  - `internal/stencilstore/reconcile.go`
  - `internal/stencil/stencil.go`
  - `internal/modelspec/modelspec.go`
- **Edits:** none
- **Creates:**
  - `internal/mergeresolve/spec.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** Create `internal/mergeresolve/spec.go` declaring an unexported spec builder that takes the resolver's told values, the conflicted path list, and the attempt number, and returns a `shuttleengine.Spec`.

  It follows `loomengine.DiscussionSpec`'s exact resolution shape: `modelspec.Parse(deps.ConflictSpec)`, then `deps.Registry.Resolve(spec)`, then `Model: resolved.Model`, `Effort: resolved.Params["effort"]`, `Version: resolved.Params["version"]`.
  The prompt is read from the told stencils directory via `stencilstore.Read(deps.StencilsDir, "landing-template-conflict")` and filled with `stencil.Fill` — never composed from a Go string literal, per the Stencil Ownership Invariant.
  The fill values are the conflicted paths rendered as a list and the absolute report path.

  The spec sets `Interactive: false`, `Role: "conflict"`, and `Timeout` from the told duration.

  `OutputFiles` names exactly one **fresh, absolute** path: the resolution report at `<ScratchDir>/conflict-resolution-r<attempt>.md`.
  Three properties of that choice are load-bearing and each needs stating in the builder's own comment:

  1. Absolute rather than relative, because a relative entry is resolved against a worktree root that is not this scratch directory's parent on an anchored layout, which would land the report in the wrong directory.
  2. Per-attempt (`r1`, `r2`), because the spec validator rejects an entry that already exists on disk, so a retry reusing the first attempt's path would fail before the session even started.
  3. Never the conflicted paths themselves — those exist on disk by definition, so the validator would reject them outright, and the pre-run archive step would archive the very files needing resolution.

  The builder also creates `ScratchDir` with `os.MkdirAll` before returning, on this write path, since a told directory may not exist yet.
  Creating a told directory is legal under the Told-Geometry Invariant; deriving one would not be.
- **Commit:** `feat(mergeresolve): build the conflict session shuttle spec`

### Card 15: Resolve — the decision table

- **Context:**
  - `internal/mergeresolve/deps.go`
  - `internal/mergeresolve/markers.go`
  - `internal/mergeresolve/spec.go`
  - `internal/fabricengine/mergeerrors.go`
  - `internal/fabricengine/merge.go`
  - `internal/fabricengine/mergelifecycle.go`
  - `internal/shedadapters/singlellm.go`
  - `internal/shedadapters/ctx.go`
  - `internal/loomshed/ctx.go`
  - `internal/shuttleengine/run.go`
- **Edits:** none
- **Creates:**
  - `internal/mergeresolve/mergeresolve.go`
  - `internal/mergeresolve/ctx.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** Create `internal/mergeresolve/ctx.go` with this package's own unexported `entryErr` and `cancelErr` helpers, shaped exactly like `internal/loomshed/ctx.go`'s pair but naming this package in their message prefix.

  Create `internal/mergeresolve/mergeresolve.go` declaring `func (r *Resolver) Resolve(ctx context.Context, source string) (Result, error)`, whose body is the full decision table:

  1. `entryErr` first — a cancelled context at entry surfaces as a non-nil error, never as a stuck verdict.
  2. Crash recovery: call `MergeInProgress()`; when true, call `MergeAbort()` unconditionally and start a clean attempt, because the caller re-invokes this verbatim after a crash and a half-resolved worktree left by a killed session is not a state to resume into.
  3. Call `MergeIn(source)`. Error dispositions, each with its own `Reason` text:
     - `*fabricengine.ErrForeignMergeState` → stuck, and no abort call of any kind. Real merge state a human left behind is never touched.
     - `*fabricengine.ErrUnmergeableState` → stuck with the error surfaced, and no abort: that call already self-aborted the whole attempt, and a second abort would report no-merge-in-progress and turn a clear diagnosis into a confusing one.
     - any other typed or untyped error → the same catch-all stuck path with the error text surfaced, and no abort. The guard errors refuse before mutating anything, so there is nothing to unwind, and a future error type falls into escalate-and-say-why rather than a wrong branch.
  4. `AlreadyUpToDate` → resolved immediately: no session, no staging, no conclude.
  5. No conflicts → resolved: the merge concluded itself, so there is nothing further to do.
  6. Conflicts present → the resolution loop, at most two attempts:
     a. build the spec for this attempt and call the shuttle seam once;
     b. map the run outcome: a done outcome proceeds to verification; an asking, died, timeout, or unrecognized outcome consults `cancelErr` first and then goes to the abort-and-stuck path with its own reason naming the outcome, and never reaches the conclude call;
     c. on a done outcome, run the marker scan over the conflicted paths; a scan error is a hard error return;
     d. scan clean → call `MergeStageResolved` with exactly the conflicted paths, **then** `MergeContinue("")`, in that order, and return resolved. The ordering is the entire reason the staging verb exists;
     e. scan still reporting unresolved paths → retry the session once with a fresh attempt number, and on the second failure call `MergeAbort()` and return stuck naming the paths that stayed unresolved.
  7. Every non-success exit consults `cancelErr` first, replacing its verdict with a non-nil error when the context was cancelled during the run — a stuck returned under a cancelled context is indistinguishable from a genuine verdict to the caller.

  Document at the top of `Resolve` that the marker scan, not the session's own verdict, is what decides a conflict is resolved, and why: the conclude step is irreversible at the Fabric layer and the abort call is the only checkpoint covering the attempt window.
- **Commit:** `feat(mergeresolve): implement Resolve's merge and conflict decision table`

### Card 16: told-geometry seam enforcement

- **Context:**
  - `internal/loomshed/seam_enforcement_test.go`
  - `internal/mergeresolve/deps.go`
  - `internal/mergeresolve/mergeresolve.go`
  - `internal/mergeresolve/spec.go`
  - `internal/mergeresolve/markers.go`
  - `internal/mergeresolve/ctx.go`
- **Edits:** none
- **Creates:**
  - `internal/mergeresolve/seam_enforcement_test.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** Create `internal/mergeresolve/seam_enforcement_test.go`, modelled directly on `internal/loomshed/seam_enforcement_test.go`: same `filepath.WalkDir` over non-test `.go` files, same `parser.ParseFile` with `parser.ImportsOnly`, same stdlib-detection rule, and the same allowlist-membership shape rather than a bare denylist.
  Name the test `TestToldGeometryInvariant_AllowlistOnly` and the map `mergeresolveAllowedImports`.
  The allowlist holds exactly the non-stdlib import paths this package's production files actually use — the fabric engine, the shuttle engine, the model-spec package, the stencil store, and the stencil filler — and nothing else.
  Its comment explains, as `loomshed`'s does, that a membership list rather than a denylist catches anything that would drag geometry resolution in, with no maintenance beyond a genuine new dependency, and that a transitive reach is explicitly fine.
- **Commit:** `test(mergeresolve): enforce the Told-Geometry import allowlist`

### Card 17: the decision table under test

- **Context:**
  - `internal/mergeresolve/mergeresolve.go`
  - `internal/mergeresolve/deps.go`
  - `internal/mergeresolve/markers.go`
  - `internal/mergeresolve/spec.go`
  - `internal/fabricengine/mergeerrors.go`
  - `internal/shuttleengine/run.go`
  - `internal/shedadapters/singlellm_test.go`
- **Edits:** none
- **Creates:**
  - `internal/mergeresolve/mergeresolve_test.go`
  - `internal/mergeresolve/markers_test.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** Write the unit tier against two fakes — one implementing the merge seam and recording its call order, one implementing the shuttle seam and returning a scripted outcome per attempt.
  Both fakes live in the test files; the whole package is designed to be testable with no real git and no real model, so these files stay untagged and spawn nothing.

  `mergeresolve_test.go` covers, one test per row:

  - clean merge → no session spawned at all, verdict resolved;
  - conflict, then markers gone on re-read → the staging verb called with exactly the conflicted paths, then the conclude call, verdict resolved;
  - the ordering assertion: the conclude call never precedes the staging call — assert on the recorded call order, not merely that both happened;
  - conflict, markers still present after the session → exactly one retry, then the abort call and verdict stuck;
  - merge-in-progress true at entry → the abort call happens before any new attempt;
  - foreign-merge-state error → stuck, and the abort call never happens;
  - unmergeable-state error from the merge-in → stuck with the error surfaced, and the abort call never happens;
  - an unrecognized typed error → the same catch-all stuck path, proving the default is escalate rather than fall through;
  - shuttle outcomes asking, died, and timeout → each mapped to stuck, and the conclude call never happens;
  - context cancellation → surfaced as a non-nil error, never as a stuck verdict;
  - already-up-to-date → resolved with no session and no conclude call;
  - the retry uses a report path distinct from the first attempt's, so the spec validator's already-exists rejection cannot fire on the second attempt — assert on the two specs the shuttle fake received;
  - the scratch directory absent on disk → the first spec build creates it rather than failing.

  `markers_test.go` covers the scan in isolation: a file with each of the three markers at line start; a file whose markers appear mid-line only, which must pass; a path that no longer exists, which counts as resolved by deletion and is not reported unresolved; and a read error other than not-exist, which is a genuine failure distinct from the deletion case.
- **Commit:** `test(mergeresolve): cover the merge and conflict decision table`

### Card 18: the conflict-resolution stencil

- **Context:**
  - `contracts/stencils/loom/loom-template-discussion.md`
  - `contracts/stencils/webster/webster-body-implementer.md`
  - `internal/mergeresolve/spec.go`
- **Edits:** none
- **Creates:**
  - `contracts/stencils/landing/landing-template-conflict.md`
- **Deletes:** none
- **Moves:** none
- **Requirements:** Create the conflict-resolution prompt at `contracts/stencils/landing/landing-template-conflict.md`, in the style of the existing stencils in sibling family folders, with fill placeholders matching exactly the value keys the spec builder passes: the conflicted path list and the report path.

  Content requirements, each of which is a constraint rather than a preference:

  - It names only unified, worktree-relative paths and describes **one repo**. It never mentions a second repository or either fabric-internal side's name — the machine-enforced vocabulary walk covers this directory's Markdown, so a forbidden token fails the build.
  - It instructs the session to resolve each listed path by editing files in place, and to write the report at the given path as its terminal act, since that file is the run's return value.
  - It forbids running git in any form. Every mutating git operation belongs to the engine layer, in-process, and never to an agent — the session resolves file content and nothing else. State that prohibition on a single line each time it appears.
  - It must **never contain a literal line-start conflict marker.** The marker scan is line-anchored, and this file is one an agent may copy text from; describe markers indented or inside a fence instead, so a copied fragment can never make a resolved file fail the scan.
- **Commit:** `feat(stencils): add the landing conflict-resolution prompt`

### Card 19: register the stencil

- **Context:**
  - `contracts/stencils/registry_test.go`
  - `contracts/stencils/landing/landing-template-conflict.md`
- **Edits:**
  - `contracts/stencils/stencils.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Register the new stencil in `contracts/stencils/stencils.go`, which is the only place a stencil's on-disk path and its Go identifier are both named.
  Add an embedded byte var `LandingTemplateConflict` carrying a `//go:embed landing/landing-template-conflict.md` directive and a doc comment naming it as the landing conflict-resolution producer's shipped-default prompt, declared alongside the other family vars.
  Add the matching row `{"landing-template-conflict", &LandingTemplateConflict},` to the `entries` registry slice.
  The registry test fails in both directions — an on-disk file with no row, and a row with no file — so both halves are mandatory and neither is optional.
- **Commit:** `feat(stencils): register landing-template-conflict`

## Batch Tests

`verify:` runs three packages' fast tiers, all untagged, because nothing in this batch needs a real git repository or a real model — the whole package is designed around two injectable seams and its tests spawn nothing, which keeps it in tier 1 per the Test Tier Purity Invariant.

- `./internal/mergeresolve/...` runs the decision-table tier (card 17), the marker-scan tier (card 17), and the import allowlist guard (card 16).
- `./contracts/stencils/...` runs `TestRegistry_MatchesOnDiskTree`, the both-directions check that cards 18 and 19 satisfy only together.
- `./internal/lyxcwd/...` runs the Fabric Vocabulary enforcement walk, which covers both this package's production Go and the new stencil Markdown — a forbidden token in either is a build failure, so this is the gate that proves the vocabulary constraint held rather than merely being remembered.
