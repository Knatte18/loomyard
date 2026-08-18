# Batch: websterengine Geometry and RefMatcher

```yaml
task: websterengine + webstercli told-geometry, and Webster standalone entry
batch: websterengine Geometry and RefMatcher
number: 4
cards: 3
verify: go test ./internal/websterengine/...
depends-on: []
```

## Batch Scope

This batch adds the two new websterengine-owned types the rest of the migration is built on, and nothing else:
`websterengine.Geometry`, the eight told values webster needs, and `RefMatcher` plus its never-matching implementation `NeverMatches`.
Both additions are purely additive — no existing signature changes, no existing field moves — so the whole repository stays green after this batch, and batches 6 and 7 can then depend on the types existing without also depending on each other's edits.
The external interface batch 6 consumes is `websterengine.Geometry` (built by `hubgeom.WebsterGeometry` and by `internal/standalonegeom`);
the one batch 7 consumes is `RefMatcher`, which replaces `*fabricengine.RefScanner` in `CheckFork`/`CheckParent`.

## Cards

### Card 10: Declare `websterengine.Geometry`

- **Context:**
  - `internal/reedengine/geometry.go`
  - `internal/websterengine/state.go`
  - `internal/websterengine/runlevel.go`
  - `internal/hubgeom/hubgeom.go`
- **Edits:** none
- **Creates:**
  - `internal/websterengine/geometry.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** Create `internal/websterengine/geometry.go` declaring `type Geometry struct` with exactly eight string fields, mirroring the shape `reedengine.Geometry` already proved:
  `AnchorRoot`, `WorktreeRoot`, `WebsterDir`, `ReportsDir`, `PromptsDir`, `ScratchDir`, `StencilsDir`, `PlanDir`.
  Declare the type only — no constructor, no validator, no default, and no method — exactly as `internal/reedengine/geometry.go` does;
  populating every field is entirely the caller's obligation.
  The file must import nothing.
  Each field carries its own doc comment.
  `AnchorRoot` is the base every `_lyx`/`.lyx` join and every module config read hangs off.
  `WorktreeRoot`'s comment is load-bearing and must state three things:
  it is the repo checkout the head-SHA capture and the dirty-worktree check read;
  it is also the fork-audit workdir, which must equal the pane's actual cwd (`reedengine.Geometry.PaneCwd`), because the audit resolves transcript-relative write paths against it;
  and it is **not** the same notion as `reedengine.Geometry.WorktreeRoot` — webster's is the anchor-anchored value every one of its CLI call sites passes today, while reed's is the worktree path, and the two coincide only at an unanchored anchor.
  Say outright that this collision is deliberate continuity and must not be "fixed" by converging either field on the other.
  `StencilsDir` is the told absolute directory the prompt stencils are read from at call time;
  `PlanDir` is the told directory `planparser` parses.
  The file-header comment must state that `hubgeom.WebsterGeometry` is the hub-mode teller and `internal/standalonegeom` the told-mode one, and that the dependency direction is one-way — those packages import `websterengine`, never the reverse.
- **Commit:** `feat(websterengine): declare Geometry, the eight told values webster is given`

### Card 11: Declare `RefMatcher` and `NeverMatches`

- **Context:**
  - `internal/fabricengine/refscanner.go`
  - `internal/websterengine/integration.go`
- **Edits:**
  - `internal/websterengine/audit.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** In `internal/websterengine/audit.go`, declare `type RefMatcher interface { Matches(cmd string) bool }` — the narrow seam `CheckFork` and `CheckParent` consult for the fabric-reference violation class, satisfied by `*fabricengine.RefScanner` without any adapter, since that type already has the identical method.
  Declare beside it `type NeverMatches struct{}` with a `Matches(string) bool` method that always returns `false`, the pinned supplier for a mode with no fabric repo at all.
  `NeverMatches` lives here, beside the interface it implements, rather than in a geometry package that has no business knowing webster's audit vocabulary, and it is a named exported type rather than an inline literal so the four `Deps`-construction sites in batch 8 share one supplier instead of re-inventing it.
  Its doc comment must state why it exists: `CheckFork` and `CheckParent` call `Matches` unguarded, so a nil `RefMatcher` is a panic on the first standalone `record-batch`, and the field must therefore never be nil in either mode.
  Do not change `CheckFork`'s or `CheckParent`'s signatures in this card — batch 7 does that.
  Do not remove the `internal/fabricengine` import from this file in this card;
  it is still used by the two signatures until batch 7.
- **Commit:** `feat(websterengine): declare the RefMatcher seam and its NeverMatches supplier`

### Card 12: Pin `NeverMatches`

- **Context:**
  - `internal/websterengine/audit.go`
  - `internal/websterengine/geometry.go`
- **Edits:**
  - `internal/websterengine/audit_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Add a test to `internal/websterengine/audit_test.go` asserting `NeverMatches{}` returns `false` for a command spelling a real `*fabricengine.RefScanner` would match — reuse one of the fabric-referencing command strings the existing `TestRefScannerMatches` cases already drive — and also for an empty string and an ordinary non-fabric command.
  Add a compile-time assertion in the same file that both `NeverMatches` and `*fabricengine.RefScanner` satisfy `RefMatcher`, written as two `var _ RefMatcher = ...` declarations, so a later signature drift on either side fails at compile time rather than at the first standalone run.
  Leave every existing case in this file unchanged — `TestRefScannerMatches` builds a real scanner from a `fakeLayout` and that coverage is what the Fabric Git Invariant cites as its machine check.
- **Commit:** `test(websterengine): pin NeverMatches and the RefMatcher satisfaction of both suppliers`

## Batch Tests

`verify:` is `go test ./internal/websterengine/...`.
The scope is exactly the package this batch adds to;
nothing outside it is touched, and both additions are new declarations that cannot regress an existing caller.
Card 12 is the batch's assertion surface: the `NeverMatches` behaviour test plus the two compile-time `RefMatcher` satisfaction declarations, the second of which is what guarantees batch 7's signature swap is a drop-in for the real scanner rather than a change that silently needs an adapter.
`websterengine.Geometry` has no behaviour of its own to test at this point — it is a field-only struct with no constructor — so its correctness is pinned in batch 6, where `hubgeom.WebsterGeometry` and `internal/standalonegeom` first populate it and the two builders' table tests assert every field.
