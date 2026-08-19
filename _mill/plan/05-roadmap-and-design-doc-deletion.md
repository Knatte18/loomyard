# Batch: roadmap-and-design-doc-deletion

```yaml
task: "invariants and docs for the told-geometry rule"
batch: "roadmap-and-design-doc-deletion"
number: 5
cards: 1
verify: go test ./internal/lyxcwd/...
depends-on: [1]
```

## Batch Scope

This batch closes out the producers-standalone design doc per the Documentation Lifecycle: it deletes `manifest/designs/producers-standalone.md` and, in the same commit, repoints all five `manifest/roadmap.md` lines that link to it.

It is a single card by design, not by size.
The deletion and the five link rewrites must land together: `internal/lyxcwd/docslink_test.go` resolves the file part of every inline markdown link under `manifest/` and `docs/`, so a commit that deletes the doc without fixing the links — or fixes the links in a commit that does not delete the doc — leaves the tree in a state a reviewer would have to reason about separately.
Landing them together makes the whole change one atomic, verifiable step.

It depends on batch 1 because all five rewritten lines point at `../CONSTRAINTS.md#told-geometry-invariant`, and that anchor does not exist until batch 1 lands.

The task's brief for this file is narrow: `manifest/roadmap.md` moves only on completing or adding a planned item.
Completing a planned item is exactly this case, which is what licenses the move.
Do not touch any other Planned, Someday, or Done entry.

## Cards

### Card 15: complete the Planned item, repoint four Done entries, delete the design doc

- **Context:**
  - `CONSTRAINTS.md`
  - `internal/preflight/doc.go`
  - `internal/hubgeom/doc.go`
  - `internal/standalonegeom/doc.go`
  - `internal/lyxcwd/docslink_test.go`
  - `internal/buildinfo/doc.go`
- **Edits:**
  - `manifest/roadmap.md`
- **Creates:** none
- **Deletes:**
  - `manifest/designs/producers-standalone.md`
- **Moves:** none
- **Requirements:**
  Five link-bearing lines in `manifest/roadmap.md` reference the design doc today.
  Every one of them changes, and the doc is deleted in the same commit.

  **1. Move the Planned item into Done.**
  The `## Planned` section holds exactly one item, "producers standalone: invariants and docs", whose entry ends with a `See [designs/producers-standalone.md](designs/producers-standalone.md).` line.
  Move that whole entry to the **head** of the `## Done` section, above the existing "producers standalone: told-geometry foundations" entry, and repoint its `See` line at the new invariant — an inline markdown link with the target `../CONSTRAINTS.md#told-geometry-invariant`.
  Keep the entry's wording as the shipped description of what landed rather than a forward-looking promise, and keep it to a name plus one or two sentences per the file's own Maintenance rules.

  Leave `## Planned` present but empty of items, with its "Committed to, in this order, next." lede intact.
  Do not promote anything out of `## Someday` to fill it — that is a scoping decision for a separate task.

  No renumbering is needed anywhere.
  Every item in this file is written literally as `1.` and rendered sequentially by CommonMark, so inserting an entry at the head of a section just works.

  **2. Reword the four existing Done entries.**
  Four producers-standalone entries already sit in `## Done`: "told-geometry foundations", "mid-layer", "producer engines", and "the standalone CLI path".
  Each ends with a `See [designs/producers-standalone.md](designs/producers-standalone.md) — the doc survives this task because …` line.

  Drop the "the doc survives this task because …" clause from all four — those clauses are false statements the moment the doc is deleted, independent of the link breaking.
  Repoint each `See` line at `../CONSTRAINTS.md#told-geometry-invariant` and, where it adds something the invariant does not carry, at the relevant package documentation.
  Write a package-documentation reference as a prose mention in the style the file already uses elsewhere ("See the `internal/gitkit` and `internal/hubforge` package documentation.") rather than as a markdown link into a `.go` file — a prose mention is what the neighbouring Done entries do, and it keeps the line readable.
  `internal/preflight/doc.go`, `internal/hubgeom/doc.go`, and `internal/standalonegeom/doc.go` are the packages whose docs carry the durable rationale the deleted design doc held.

  Change nothing else about the four entries — their names and their one-or-two-sentence descriptions stay as they are.

  **3. Delete `manifest/designs/producers-standalone.md`.**
  Use `git rm` so the deletion is staged as a deletion rather than an untracked removal.
  The Documentation Lifecycle deletes a module-design doc when its module lands;
  every wave of this one has landed.
  Its durable content already lives in the three package docs named above, and its rule form now lives in `CONSTRAINTS.md`'s Told-Geometry Invariant.

  **Before committing, grep the whole repository for any remaining reference to `producers-standalone`.**
  Exactly one non-roadmap reference existed when this plan was written — a prose mention in `internal/buildinfo/doc.go`, which batch 3 rewords.
  Do not edit `internal/buildinfo/doc.go` from this card;
  if the grep finds it still present, batch 3 has not landed yet and that is expected, since the two batches have no ordering constraint.
  If the grep finds any *other* reference, fix it here rather than leaving a dangling pointer.
- **Commit:** `docs(roadmap): complete the producers-standalone line and delete its design doc`

## Batch Tests

`verify: go test ./internal/lyxcwd/...` runs `TestEnforcement_MarkdownLinks`, which is the gate that makes this batch verifiable rather than merely reviewed.
`manifest/roadmap.md` is a link **scan source**, so all five rewritten `See` lines have their file part and `#anchor` resolved against the tree — and, critically, the guard fails on any link still pointing at `designs/producers-standalone.md` after the deletion.
A partial rewrite (four lines fixed, one missed) therefore cannot pass.

The same test enforces the self-expiring `docsLinkAllowlist`.
Neither of its two entries keys `manifest/roadmap.md`, so this batch neither strands nor needs an allowlist entry — but do not add one to work around a broken link.
A dangling link here means a `See` line was missed, and the fix is the `See` line.

`TestEnforcement_FabricVocabulary` does not reach `manifest/roadmap.md` (its `.md` walk covers `internal/**/*.md` and `contracts/stencils/**/*.md` only), so vocabulary in the rewritten entries is a review obligation.

No Go file is touched, so the overview's module-wide `verify: go build ./...` is a no-op here.
