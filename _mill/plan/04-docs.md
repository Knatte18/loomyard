# Batch: docs

```yaml
task: 'shedadapters: Burler-round producer'
batch: 'docs'
number: 4
cards: 4
verify: go test ./internal/lyxcwd/ ./internal/shedadapters/
depends-on: [3]
```

## Batch Scope

This batch lands the documentation the Documentation Lifecycle requires in the same task as the code: the `internal/shedadapters` package doc gains its fourth adapter and the durable statement of the two-sided pair predicate, `manifest/designs/shed.md` and `docs/overview.md` stop saying "three adapters", and `manifest/roadmap.md` moves this task's Planned item to Done while correcting the still-Planned `Bouncer` item's seed-vs-judge predicate.
It is one batch because every card is prose over an interface batch 3 has already frozen, and it depends on batch 3 so no doc claims a symbol that does not yet exist.
There is no external interface for a later batch to consume — this is the plan's last batch.
Batch-local decision differing from `## Shared Decisions`: nothing.

## Cards

### Card 11: `internal/shedadapters` package doc

- **Context:**
  - `internal/shedadapters/burler.go`
  - `internal/shedadapters/focus.go`
  - `internal/shedadapters/perch.go`
  - `internal/shedadapters/singlellm.go`
  - `internal/shedadapters/ctx.go`
  - `internal/shedadapters/archive.go`
  - `internal/shedengine/producer.go`
- **Edits:**
  - `internal/shedadapters/doc.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Update `internal/shedadapters/doc.go` so it describes four adapters rather than three.
  Rewrite the opening paragraph so the count is four and `BurlerProducer` is named alongside `SingleLLMProducer`, `PerchProducer`, and `WebsterProducer`, described as wrapping one `burlerengine` A-review/B-fix round as a single Shed row.
  Add a `BurlerProducer` bullet to the `# Outcome mapping` section stating that a completed round maps to `Stuck` — never `Done` — with the round's own review path as the pointer, that the `Stuck` is a routine hand-off to the segment's `Bouncer` via `OnStuck` rather than a real stuck condition, and that every non-done shuttle outcome surviving the bounded retry is an engine-level error rather than `Stuck`.
  Add a sentence to `# Told, never derived` stating that `BurlerProducer` is told an absolute run directory and an already-constructed runner, and takes the same injected clock `SingleLLMProducer` does, resolving only the archive filename's same-second collision suffix.
  Add a new section documenting the round-artifact convention as the binding two-sided contract, so it survives independently of the roadmap entry that is deleted when the `Bouncer` item completes.
  It must state: artifact paths are flat inside the told run directory, one canonical pair per round with no attempt suffix, `round-<N>-review.md` and `round-<N>-fixer-report.md` with `N` a positive decimal integer carrying no leading zeros; a retry writes to the same two paths, because a retry is a second try at the one artifact the round owes rather than a second artifact; the presence of **both** files means, and only means, that round `N` completed and produced a usable review; the round producer uses exactly that pair predicate to decide whether to advance and the `Bouncer` uses exactly that pair predicate to tell its seed call from its judge call; the structured next-round directive is JSON at `round-<N>-focus.json` beside them, whose token names the round the directives are **for**, not the round that produced them, so a `Bouncer` rejecting round `N` writes the file for round `N+1` and the seed call writes the file for round 1; and that reading that file is fail-safe end to end, degrading to "no directive" with a warning rather than erroring, including at application time when a well-formed directive cannot be honoured.
  Add a paragraph to `# Shared cancellation rule` stating that `BurlerProducer` does not need the section's success exception at all — not that it reads the exception differently.
  It must explain that `internal/shedengine/producer.go` binds every implementation to surface cancellation as a non-nil error and never as `Stuck`, that this producer therefore always errors under cancellation including on a completed round, and that the exception's purpose — never discarding a paid-for artifact — is served instead by an archive carve-out: a completed-then-cancelled round keeps its two files, so from-disk round resolution advances past it on the next call and only the re-derivable in-memory verdict is dropped.
  Add to `# Limitations` that `BurlerProducer` installs no mid-run cancellation bridge, because `burlerengine` exposes no pause seam, so a cancel is observed only once the round reaches a terminal outcome or its own `RunOpts.Timeout` elapses.
  Keep the file's existing `--` comment style and its section ordering.
- **Commit:** `docs(shedadapters): document BurlerProducer and the round-artifact contract`

### Card 12: `manifest/designs/shed.md`

- **Context:**
  - `internal/shedadapters/doc.go`
  - `internal/shedadapters/burler.go`
  - `manifest/roadmap.md`
- **Edits:**
  - `manifest/designs/shed.md`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Update the three sites in `manifest/designs/shed.md` that are present-tense about what the adapters are and therefore carry the now-stale count.
  In the status blockquote at the top of the file, change the parenthetical listing the shipped adapters so it names four — `SingleLLMProducer`, the `perch` adapter, the `Webster` adapter, and the `Burler`-round adapter — and adjust "the three engine adapters" accordingly.
  In the `## Process — decomposed into several small tasks` section, update the sentence beginning "The three engine adapters" the same way, keeping the sentence's existing claim that each is a small, self-contained wrapper around an already-shipped engine.
  In the `### Engine adapters — a thin, shared seam, not one per producer` section, add a bullet for the `Burler`-round adapter alongside the existing `SingleLLMProducer`, `perch`, and black-box-multi-spawn bullets, stating that it wraps one `burlerengine` A-review/B-fix round as a single Shed row and always hands back to its segment's `Bouncer` via `Stuck`, never advancing on its own.
  In the same section, record the pair predicate durably: the presence of both `round-<N>-review.md` and `round-<N>-fixer-report.md` in the segment's run directory means, and only means, that round `N` completed and produced a usable review, and both the round producer and the `Bouncer` run that same test — the round producer to decide whether to advance, the `Bouncer` to tell its seed call from its judge call.
  Also state there that the segment's round cap is the smaller of the two rows' `MaxBounces` budgets rather than either row's alone, because neither row's bounce episode ever resets and the `Bouncer` — the segment's entry point — runs one `Stuck` ahead, so raising the cap means raising both rows together.
  Do not restate the whole producer contract here; this doc stays the design's narrative and points at the package documentation for the as-built detail.
  Use semantic line breaks and add no new markdown link whose target or anchor does not resolve.
- **Commit:** `docs(shed): add the Burler-round adapter and the pair predicate to the design`

### Card 13: `docs/overview.md`

- **Context:**
  - `internal/shedadapters/doc.go`
  - `manifest/designs/shed.md`
- **Edits:**
  - `docs/overview.md`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Update the three "three adapters" sites in `docs/overview.md`.
  In the module-tree block, change the `internal/shedadapters/` line so it reads as four Shed engine adapters and names the burler round producer alongside `SingleLLMProducer`, perch, and Webster, keeping the line's existing column alignment with its neighbours.
  In the prose sentence naming the shipped engine adapters, and in the status-table sentence that repeats the same list, change both to four and name the new adapter the same way.
  Change nothing else in the file — the module table gains no new row, because the adapter ships inside the already-listed `internal/shedadapters` package rather than as a new module.
  Use semantic line breaks and add no new markdown link whose target or anchor does not resolve.
- **Commit:** `docs(overview): count four Shed engine adapters`

### Card 14: `manifest/roadmap.md`

- **Context:**
  - `internal/shedadapters/doc.go`
  - `internal/shedadapters/burler.go`
  - `manifest/designs/shed.md`
- **Edits:**
  - `manifest/roadmap.md`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Make two distinct edits to `manifest/roadmap.md`.
  First, the ordinary lifecycle move: remove the **shedadapters: Burler-round producer** item from the `### Perch → Shed flattening` subsection of `## Planned` and add a corresponding entry at the end of `## Done`, written as a bold item name plus one or two sentences of what shipped, pointing at the `internal/shedadapters` package documentation for the durable detail, in the same shape as the existing `## Done` entries.
  The Done entry must record that the adapter always returns `Stuck` to its segment's `Bouncer` and never `Done`, that it resolves its round from disk over the pair predicate, and that `burlerengine.Profile.ClusterExclude` shipped with it as the per-call cluster-fan trimming knob.
  Write the item literally as `1.` per the file's own Maintenance section — numbering is automatic and no renumbering is needed anywhere.
  Second, a correction to the still-Planned **Bouncer: the generic review-gate producer** item, distinct from the lifecycle move above: its seed-vs-judge sentence currently reads "if the round producer's report artifact for the current round does not exist yet", a single-artifact predicate that is exactly the review-only-orphan wedge this task designed out.
  Amend that sentence to the pair predicate — the seed call is the case where the current round's review and fixer-report artifacts are not **both** present — and keep the rest of the sentence, including its `Call(ctx)` parenthetical and its `Stuck`/`OnStuck` conclusion, unchanged.
  Third, repair the four cross-references the lifecycle move above makes false, in the same commit, because the `### Perch → Shed flattening` subsection holds exactly two items today and drops to one.
  In the **Bouncer: the generic review-gate producer** item, its opening sentence's "unlike the Burler-round producer above" and its later "an instance of `shedadapters: Burler-round producer` above" both point at an item that is no longer above; reword each to name the now-Done Burler-round producer and point at the `internal/shedadapters` package documentation instead of at a Planned sibling.
  In each of the three loom review-producer items whose line reads "Depends on the two \"Perch → Shed flattening\" items above", reword so the count is correct and the already-shipped half is named as shipped rather than as a pending dependency.
  Change no other Planned or Someday item beyond these four cross-references and the two edits above.
  Use semantic line breaks and add no new markdown link whose target or anchor does not resolve.
- **Commit:** `docs(roadmap): ship the Burler-round producer and fix the Bouncer seed predicate`

## Batch Tests

`verify: go test ./internal/lyxcwd/ ./internal/shedadapters/` runs the two suites this batch can break.
`internal/lyxcwd` carries `docslink_test.go`'s `TestEnforcement_MarkdownLinks`, the Markdown Link Integrity check over every inline link in `manifest/` and `docs/` — the gate for cards 12, 13, and 14 — plus `enforcement_test.go`'s geometry-literal check.
`internal/shedadapters` is included because card 11 edits `doc.go`, a Go file in that package: a malformed edit breaks compilation, and running the package's suite is the cheapest way to catch it.
The overview's module-wide `verify: go build ./...` additionally confirms the whole tree still compiles at this batch's boundary.
