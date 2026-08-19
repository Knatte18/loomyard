# Batch: docgo-audit

```yaml
task: "invariants and docs for the told-geometry rule"
batch: "docgo-audit"
number: 3
cards: 5
verify: go test ./internal/lyxcwd/...
depends-on: []
```

## Batch Scope

This batch is the `doc.go` audit across the fifteen packages the producers-standalone waves converted.
Twelve of the fifteen need no edit and are not cards here — seven already carry correct told-geometry prose (`internal/shuttleengine`, `internal/reedengine`, `internal/pattern`, `internal/perchengine`, `internal/websterengine`, `internal/planparser`, `internal/scoutengine`) and three have no `doc.go` at all (`internal/configengine`, `internal/webstercli`, `internal/scoutcli`), which this task deliberately does not create.
The five remaining packages each get one card.

It is one batch because every card is the same shape — read a package doc, add one sentence naming its tier and whether it is told or resolves — and because a reviewer wants the five sentences side by side to check they do not contradict each other or the prose already in each file.

The batch has no dependency: it touches only Go doc comments, in five packages none of the other batches touch.

The `internal/buildinfo` card is the batch's only two-part card — it also rewords the package's prose reference to the design doc batch 5 deletes.
That reword is here rather than in batch 5 because it is a Go doc comment in a package this batch is already auditing, and because `internal/lyxcwd/docslink_test.go` does not catch a prose mention, so the two edits have no ordering constraint between them.

Batch-local decision: every sentence added here sits inside `TestEnforcement_FabricVocabulary`'s `{internal, cmd}` `.go` walk, unlike this task's three `.md` files.
Do not write `warp`, `weft`, or any fabric-sense `host` phrase in these sentences — none of the five packages needs to tell the two sides apart, and the walk will fail the build if one appears.

## Cards

### Card 7: `internal/tokenvocab` — name its tier

- **Context:**
  - `internal/tokenvocab/leaf_enforcement_test.go`
  - `CONSTRAINTS.md`
- **Edits:**
  - `internal/tokenvocab/doc.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  Add one sentence to the package doc comment stating that `tokenvocab` is told its geometry and derives none of its own — it takes plain path strings from its caller rather than resolving them, so it requires none of the three resolution tiers and runs identically inside a lyx hub and outside one.
  Point at `CONSTRAINTS.md`'s Told-Geometry Invariant by name for the rule, without restating the tier table.

  Place it adjacent to the existing "Leaf invariant" paragraph, which already names `internal/tokenvocab/leaf_enforcement_test.go` and `TestLeafInvariant_AllowlistOnly` — the same test that machine-enforces this package's told-geometry property, since its allowlist omits `internal/lyxcwd`.
  Say that explicitly in the new sentence rather than leaving the connection implicit: this package's told-geometry property is machine-enforced by that same allowlist.

  Leave every existing sentence in the file untouched, including the `doc.go`-carries-the-package-godoc header comment above the package doc.
- **Commit:** `docs(tokenvocab): name the package's told-geometry tier in its package doc`

### Card 8: `internal/preflight` — say that it resolves rather than is told

- **Context:**
  - `internal/preflight/predicates.go`
  - `internal/preflight/preflight.go`
  - `CONSTRAINTS.md`
- **Edits:**
  - `internal/preflight/doc.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  This package is the audit's one *resolver*, not a told package, and its doc must say so.
  Add one sentence near the top of the package doc — after the opening paragraph, before the "# The report-not-error contract" heading — stating that `preflight` is the tier-2 layer and legitimately resolves geometry rather than being told it: it imports `internal/lyxcwd` in production and calls `lyxcwd.Resolve` through `ResolveMode`, which is exactly its job as the precondition layer above the engines.
  Point at `CONSTRAINTS.md`'s Told-Geometry Invariant by name.

  The existing opening paragraph already names tier 1 and tier 2 and the mode resolver.
  Do not restate any of that — the new sentence adds only the told-versus-resolves classification and the pointer, which is what is absent today.

  Leave the "# Why there are three functions" section entirely alone.
  It is the durable home the new invariant points readers at, and it is already correct.
- **Commit:** `docs(preflight): classify the package as tier 2 and a resolver, not a told package`

### Card 9: `internal/standalonestate` — name its tier

- **Context:**
  - `internal/standalonestate/leaf_enforcement_test.go`
  - `CONSTRAINTS.md`
- **Edits:**
  - `internal/standalonestate/doc.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  Add one sentence stating that `standalonestate` is told its geometry — `Derive` takes an already-absolute target from its caller and requires none of the three resolution tiers — and that this property is machine-enforced by `internal/standalonestate/leaf_enforcement_test.go`'s `TestLeafInvariant_AllowlistOnly`, whose stdlib-only allowlist omits `internal/lyxcwd`.
  Point at `CONSTRAINTS.md`'s Told-Geometry Invariant by name.

  Place it beside the existing paragraph that already explains why `Derive` requires an absolute target and defers relative-path resolution to the CLI argument-parsing boundary.
  That paragraph states the *mechanism*;
  the new sentence names the *rule* it is an instance of.
  Do not rewrite it.
- **Commit:** `docs(standalonestate): name the package's told-geometry tier in its package doc`

### Card 10: `internal/burlerengine` — name its tier

- **Context:**
  - `internal/burlerengine/geometry.go`
  - `CONSTRAINTS.md`
- **Edits:**
  - `internal/burlerengine/doc.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  The package doc carries no told-geometry statement of any kind today.
  Add one sentence stating that `burlerengine` is a producer: it is told the absolute paths it operates on through its `Geometry` struct, derives none of its own, requires none of the three resolution tiers, and therefore runs in a directory that is not a git repository.
  Name `internal/hubgeom` and `internal/standalonegeom` as the struct's two sole constructors, in hub mode and told mode respectively.
  Point at `CONSTRAINTS.md`'s Told-Geometry Invariant by name.

  State plainly that this property is a **review obligation** here, not machine-enforced — this package has no import-allowlist test policing the absence of `internal/lyxcwd`.
  Writing it as though it were guarded would put a false claim in the package doc and contradict the invariant's own review-obligation list.

  Read `internal/burlerengine/geometry.go` first to confirm the `Geometry` struct's name and that its fields are caller-supplied absolute paths.
  If the shape differs, describe what is actually there rather than the specified text.

  Place the sentence in the package doc's opening region, before the "# The A/B round" heading, and leave every existing paragraph untouched.
- **Commit:** `docs(burlerengine): name the package's told-geometry tier in its package doc`

### Card 11: `internal/buildinfo` — name its tier and drop the design-doc reference

- **Context:**
  - `internal/buildinfo/leaf_enforcement_test.go`
  - `CONSTRAINTS.md`
- **Edits:**
  - `internal/buildinfo/doc.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  Two edits in one card, because both land in the same short package doc.

  **First, reword the design-doc reference.**
  The file today reads, byte-exactly:

```
// Its accessor is IsDev() rather than the StencilMode() the producers-standalone design doc names,
```

  That design doc is deleted by batch 5, so the phrase becomes a pointer to a file that will not exist.
  `internal/lyxcwd/docslink_test.go` does not catch it, because it is prose rather than a markdown link — which is exactly why it has to be fixed by hand here.
  Reword it to attribute the `StencilMode()` naming to the earlier design rather than to a file path, keeping the sentence's real content intact: *why* the accessor is `IsDev()` and not `StencilMode()`.
  Do not delete the sentence — the reason it gives is worth keeping, and the following sentences depend on it.

  **Second, add one sentence naming the tier.**
  State that `buildinfo` is told nothing and resolves nothing — it is an import-free leaf carrying a build-time stamp, so it requires none of the three resolution tiers — and that its exclusion of `internal/lyxcwd` is machine-enforced by `internal/buildinfo/leaf_enforcement_test.go`'s `TestLeafInvariant_AllowlistOnly`, whose allowlist is empty.
  Point at `CONSTRAINTS.md`'s Told-Geometry Invariant by name.

  Read `internal/buildinfo/leaf_enforcement_test.go` first to confirm the allowlist is genuinely empty (the Buildinfo Leaf Invariant says the package imports nothing at all, not even stdlib).
  If it is not empty, say what it actually allows.
- **Commit:** `docs(buildinfo): name the package's tier and drop the deleted design doc's name`

## Batch Tests

`verify: go test ./internal/lyxcwd/...` runs `TestEnforcement_FabricVocabulary`, whose `{internal, cmd}` `.go` walk is the one machine check that reaches this batch's edits — it fails the build if any of the five new sentences introduces a `warp`/`weft` token outside the fabric owner set or a fabric-sense `host` phrase anywhere.
The same package's `TestEnforcement_GeometryLiterals` walk covers the same files and fails on a policed geometry token written as a string literal outside its owner directory, which a doc comment could trip.

Compilation of the five edited packages is covered by the overview's module-wide `verify: go build ./...`, which runs after this batch's own verify passes.
That is the check that catches an unterminated comment or a doc comment accidentally detached from its `package` clause — the only way a `doc.go` edit can break the build.

Nothing else in the batch has a runnable surface: the content of a package doc sentence is a review obligation, and the audit's own correctness (that no added sentence contradicts prose already in that file) is named in `_mill/discussion.md` as a manual review obligation rather than a machine check.
