# Plan: invariants and docs for the told-geometry rule

```yaml
task: "invariants and docs for the told-geometry rule"
slug: "standalone-docs-and-invariants"
approved: true
started: "20260819-061409"
parent: "standalone-producers"
root: ""
verify: go build ./...
```

## Batch Index

_The fenced yaml block below is the authoritative DAG mill-go reads to schedule batches.
Every batch lives at `NN-<batch-slug>.md` in this directory and is mirrored as one entry here._

```yaml
batches:
  - number: 1
    name: constraints-invariants
    file: 01-constraints-invariants.md
    depends-on: []
    verify: go test ./internal/lyxcwd/...
  - number: 2
    name: config-strictness-guard
    file: 02-config-strictness-guard.md
    depends-on: [1]
    verify: go test ./cmd/lyx/...
  - number: 3
    name: docgo-audit
    file: 03-docgo-audit.md
    depends-on: [1]
    verify: go test ./internal/lyxcwd/...
  - number: 4
    name: overview-docs
    file: 04-overview-docs.md
    depends-on: [1]
    verify: go test ./internal/lyxcwd/...
  - number: 5
    name: roadmap-and-design-doc-deletion
    file: 05-roadmap-and-design-doc-deletion.md
    depends-on: [1]
    verify: go test ./internal/lyxcwd/...
```

## Shared Decisions

_Cross-cutting decisions every batch inherits._

### Decision: markdown semantic line breaks

- **Decision:** every `.md` line this task writes or edits uses semantic line breaks — one sentence per line, plus a break at an internal independent-clause boundary (a comma followed by "but"/"and"/"or", or a semicolon, where what follows has its own subject and verb).
  Plain newlines only;
  never trailing double-spaces, never a backslash, never a fixed-column hard wrap.
  Table cells and blockquotes stay on one line.
- **Rationale:** `CLAUDE.md`'s markdown rule, binding on every `.md` file in this repo including lines edited inside existing paragraphs.
- **Applies to:** all batches.

### Decision: `CONSTRAINTS.md`'s register is rules-only

- **Decision:** every line added to `CONSTRAINTS.md` states a rule.
  No rationale, no incident narrative, no historical justification, no "T-number" references to tasks.
  Where a reader needs the *why*, the text points at the package doc that carries it (`internal/preflight/doc.go`, `internal/hubgeom/doc.go`, `internal/standalonegeom/doc.go`) rather than restating it.
- **Rationale:** stated in the file's own header at lines 3-6 and binding on every new section.
- **Applies to:** batches 1 and 2.

### Decision: no production Go change

- **Decision:** every Go edit in this task is either a doc comment (`doc.go` prose) or a new/edited `_test.go` file.
  No production Go function, type, signature, or behaviour changes anywhere, and in particular nothing under `internal/lyxcwd` changes.
- **Rationale:** the task is a docs-and-invariants consolidation;
  the one code artefact it produces is the Config Strictness guard, which is a test.
- **Applies to:** all batches.

### Decision: additive doc.go audit, never a rewrite

- **Decision:** where a converted package's `doc.go` already carries told-geometry prose, leave it alone.
  Add at most one sentence (occasionally two) naming the package's tier and whether it is told or resolves.
  Never create a `doc.go` for a package that has none.
- **Rationale:** a blanket rewrite for uniform wording churns already-correct prose for no reader gain and makes the diff unreviewable.
- **Applies to:** batch 3.

### Decision: leave the two allowlisted broken links alone

- **Decision:** `internal/lyxcwd/docslink_test.go`'s `docsLinkAllowlist` is self-expiring — an entry whose `(file, target)` key matches no break in a scan is reported as *deletable*, which is a test failure, not a pass.
  One entry keys `docs/overview.md` with target `../CONSTRAINTS.md#package-naming`.
  That link must be left exactly as it is when `docs/overview.md` is edited — repairing or removing it strands the allowlist entry and fails the build.
  The second entry keys `manifest/designs/loom.md` with target `../../docs/overview.md#hub-geometry-invariants`;
  no `## Hub geometry invariants` heading may be added to `docs/overview.md`, for the same reason in reverse.
- **Rationale:** the allowlist's self-expiring contract is stated in `CONSTRAINTS.md`'s Markdown Link Integrity section.
- **Applies to:** batch 4.

### Decision: the new invariant's anchor slug

- **Decision:** the new `CONSTRAINTS.md` heading is exactly `## Told-Geometry Invariant`, which anchors as `#told-geometry-invariant` under `docslink_test.go`'s `docsLinkSlug` rule (strip the leading `#` run and one space, delete backticks, lowercase, delete every rune that is not a letter/digit/`_`/`-`/space, replace spaces with `-`).
  Every cross-link written by batches 4 and 5 uses that exact slug.
- **Rationale:** a mis-slugged anchor fails `TestEnforcement_MarkdownLinks`, and batch 1 is the only batch that can fix it.
- **Applies to:** batches 1, 3, 4, 5.
  Batch 3 references the heading by name in Go doc comments rather than as a markdown link, so no machine check catches a reference written before the heading exists — the DAG edge is what prevents it.

### Decision: Fabric vocabulary in the three `.md` files is a review obligation

- **Decision:** `TestEnforcement_FabricVocabulary`'s `.md` walk covers `internal/**/*.md` and `contracts/stencils/**/*.md` only, so `CONSTRAINTS.md`, `docs/overview.md`, and `manifest/roadmap.md` are outside it.
  Say Fabric (capital F) for the wired composite;
  say warp/weft only where the two sides genuinely must be told apart (tier 2's description);
  never use `host` in any fabric sense.
  Expect no machine check to catch a slip in those three files.
  The `doc.go` edits in batch 3 *are* inside the walk (its `{internal, cmd}` `.go` walk).
- **Rationale:** stated in `_mill/discussion.md`'s "Guards this task must not trip".
- **Applies to:** batches 1, 3, 4, 5.

## All Files Touched

_Full union of every `Creates:` / `Edits:` / `Moves:` **target** path across every batch, sorted alphabetically._

- `CONSTRAINTS.md`
- `cmd/lyx/configstrictness_test.go`
- `cmd/lyx/tierpurity_test.go`
- `docs/overview.md`
- `internal/buildinfo/doc.go`
- `internal/burlerengine/doc.go`
- `internal/preflight/doc.go`
- `internal/standalonestate/doc.go`
- `internal/tokenvocab/doc.go`
- `manifest/roadmap.md`
