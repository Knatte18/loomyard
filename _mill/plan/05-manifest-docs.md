# Batch: manifest-docs

```yaml
task: 'Bouncer: the generic review-gate producer'
batch: 'manifest-docs'
number: 5
cards: 3
verify: go test ./internal/lyxcwd/...
depends-on: [4]
```

## Batch Scope

This batch amends the three markdown documents the Bouncer falsifies or completes: `manifest/designs/shed.md`'s engine-adapter section, `docs/overview.md`'s module map and `shed` bullet, and `manifest/roadmap.md`'s item placement.
It runs last because the roadmap moves only on an item completing, and it is separate from batch 3's `doc.go` card because these are manifest-level documents rather than the package's own Go doc.
No batch-local decisions differ from `## Shared Decisions` in the overview.

## Cards

### Card 12: the `shed.md` engine-adapter amendment

- **Context:**
  - `internal/shedadapters/doc.go`
  - `internal/shedadapters/bouncer.go`
  - `manifest/roadmap.md`
  - `_mill/discussion.md`
  - `CONSTRAINTS.md`
- **Edits:**
  - `manifest/designs/shed.md`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  Amend the "Engine adapters — a thin, shared seam, not one per producer" section of `manifest/designs/shed.md` in exactly two places, editing the claims themselves rather than appending a note beside them.

  First, the rule "`Shed` needs not one adapter per producer, but one per distinct engine type" and its restatement "the adapter count scales with the number of distinct *engines* in play, never with the number of producers".
  The Bouncer is a second `shuttleengine`-backed member alongside `SingleLLMProducer`, so the count no longer scales with engines alone.
  Restate the rule as: one adapter per distinct engine type, plus one entry per producer that is itself new logic over an already-adapted engine rather than a translation of a different one.
  Name the Bouncer as the first of that second kind, and say explicitly that the distinction is what keeps the original rule true for the cases it was written about.

  Second, the per-engine bullet list.
  The `SingleLLMProducer` bullet describes the shuttle case as one whose parameterization "lives entirely in the caller's own `shuttleengine.Spec` source, which the adapter evaluates once per call and never templates itself" — which is precisely what the Bouncer does not do, since it composes its own prompt from stencils.
  Add a new bullet covering the Bouncer: `shuttleengine`-backed like `SingleLLMProducer`, but templating its own prompt from a rubric stencil and a generic template, with judge-specific work before and after the spawn.
  Amend the `perch` bullet — "`perch` needs one adapter, reusable by every review-gate producer" — with the note that the Burler/Bouncer pair supersedes it for review gates, pointing at the Someday `Bouncer → Perch` item in `manifest/roadmap.md` for perch's own fate.

  Keep semantic line breaks throughout, and cross-reference roadmap items by bold item name rather than by number, which is the convention `manifest/roadmap.md`'s own maintenance note states.
- **Commit:** `docs(shed): amend the engine-adapter rule for the Bouncer`

### Card 13: the `overview.md` module map

- **Context:**
  - `internal/shedadapters/doc.go`
  - `manifest/designs/shed.md`
  - `internal/lyxcwd/docslink_test.go`
- **Edits:**
  - `docs/overview.md`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  Amend `docs/overview.md` in the two places that count the package's adapters.
  Its module-tree line for `internal/shedadapters` reads "the three Shed engine adapters (SingleLLMProducer, perch, Webster) over shuttle/perch/websterengine"; make it four and name the Bouncer, keeping the line's one-line tree-entry shape and its existing column alignment.
  Its `shed` bullet says the same thing twice — once as "The three shipped engine adapters — `SingleLLMProducer` over `shuttle`, the `perch` adapter, and the `Webster` adapter — live in one package" and again in the implementation-status sentence.
  Amend both to four, naming the Bouncer and describing it as the generic review-gate producer rather than as a wrapper over an engine, since it is not one.

  Keep semantic line breaks.
  Add no new markdown link unless it is needed; any link added here is checked by `internal/lyxcwd/docslink_test.go`.
- **Commit:** `docs(overview): count the Bouncer among the shed engine adapters`

### Card 14: move the roadmap item to Done

- **Context:**
  - `internal/shedadapters/doc.go`
  - `manifest/designs/shed.md`
- **Edits:**
  - `manifest/roadmap.md`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  Move the Planned item titled "Bouncer: the generic review-gate producer" out of the "Perch → Shed flattening" group in `manifest/roadmap.md` and into the `## Done` section, placed at the top of that section where the most recent entries sit.
  Rewrite it to the length a Done entry carries — a name plus one or two sentences of what shipped — per the maintenance note's own rule that a Done entry points at the module's package documentation rather than carrying a design writeup.
  State what landed: the generic `Bouncer` producer in `internal/shedadapters`, its four `Call` modes told apart by on-disk artifacts alone, its three own file contracts, the exported `ResolveRound` helper both halves of a segment share, and the two generic stencil templates.
  Point at the `internal/shedadapters` package documentation and at `manifest/designs/shed.md`, in the same shape neighbouring Done entries use.
  Change no literal list number anywhere — every item is written as `1.` and rendered sequentially, so moving an item needs no renumbering.

  Repair the cross-references the move invalidates, in `manifest/roadmap.md` itself.
  The "loom: Discussion-Review producer" item says "instantiating the `Bouncer` producer above with it" and "see the `Bouncer` item above"; both are false once the item sits in `## Done`, which follows the whole Planned section.
  Reword each to name the Bouncer as shipped rather than as an item above.
  Each of the three `loom` review-producer items also says "Depends on the two \"Perch → Shed flattening\" items above", which is false once only the "shedadapters: Burler-round producer" item remains in that group.
  Reword each of those three to name the shipped Bouncer and the still-Planned Burler-round producer.
  Check the "loom: real LLM producers" group's own intro sentence, which says the three review-producer tasks depend on the whole "Perch → Shed flattening" group above, and reword it the same way if the move makes it read as false.
  These four-plus edits are surgical wording repairs, not scope changes: leave every one of those items' own scope, ordering, and placement exactly as it stands.

  Leave every other roadmap item where it is.
  In particular, leave the Planned "shedadapters: Burler-round producer" item and the three `loom` review-producer items in Planned — they are separate tasks, and only their cross-reference wording changes here.
  Do not delete any file under `manifest/designs/`: the documentation lifecycle's delete-on-landing rule applies to a module's own design doc, and this task ships inside `internal/shedadapters` rather than as a module with a doc of its own; `manifest/designs/shed.md` is `shed`'s doc and `shed` is not what landed here.
- **Commit:** `docs(roadmap): move the Bouncer item to Done`

## Batch Tests

`verify: go test ./internal/lyxcwd/...` runs `docslink_test.go`, whose `TestEnforcement_MarkdownLinks` is the Markdown Link Integrity check binding every markdown link in `manifest/` and `docs/` — the one runnable gate a documentation-only batch can fail.
Package-wide scope over `internal/lyxcwd` rather than a single test function is deliberate and cheap: the package's suite is fast, and the enforcement tests there are exactly the ones a doc edit can break.
The batch's remaining correctness — that the amended claims are true of the shipped code — is a review obligation, which is how `CONSTRAINTS.md` marks both the Producer Pointer-Rule Invariant and the Documentation Lifecycle.
