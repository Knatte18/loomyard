# Plan: plan-format v3: flat card list

```yaml
task: 'plan-format v3: flat card list'
slug: plan-format-v3
approved: false
started: 20260725-060638
parent: main
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
    name: promote-v3-reference
    file: 01-promote-v3-reference.md
    depends-on: []
    verify: go build ./...
  - number: 2
    name: neighbour-doc-crosslinks
    file: 02-neighbour-doc-crosslinks.md
    depends-on: [1]
    verify: go build ./...
```

## Shared Decisions

_Cross-cutting decisions every batch inherits. This is a docs-only task — no
Go source is touched. Everything in scope is Markdown under `docs/` and
`manifest/`._

### Decision: coexistence-not-replacement

- **Decision:** v3 lands as a **new** durable reference doc
  `docs/reference/plan-format-v3.md`. The existing v2 doc
  `docs/reference/plan-format.md` stays **live and valid** and gets exactly one
  softening edit (its Status header). v2 references in neighbour docs are NOT
  stale and are NOT legacy-labelled — the only neighbour edits are *additive*
  cross-links to v3.
- **Rationale:** the shipped `builder`/`webster` code still parses v2; a
  replace-in-place would make `plan-format.md` claim a contract no shipped code
  honors. v2 retires later, when the separate **webster: rewrite for flat card
  list** roadmap item lands and `builder` is deleted.
- **Applies to:** all batches

### Decision: link-paths-up-and-over

- **Decision:** the new doc lives at `docs/reference/plan-format-v3.md`. Every
  repointed inbound link and every carried-over outbound link uses the correct
  relative path for its own file's location:
  - From a `manifest/designs/*.md` file → `../../docs/reference/plan-format-v3.md`.
  - From `manifest/roadmap.md` (in `manifest/`) → `../docs/reference/plan-format-v3.md`.
  - From `docs/overview.md` (in `docs/`) → `reference/plan-format-v3.md`.
  - From a `docs/reference/*.md` sibling (`plan-format.md`, `builder-contract.md`,
    `model-spec.md`) → `plan-format-v3.md` (same directory).
  - The new `docs/reference/plan-format-v3.md`'s own `## Related` links back to
    the design docs → `../../manifest/designs/<name>.md`.
- **Rationale:** the inbound links today are same-directory (`plan-format-v3.md`)
  because the target lives in `manifest/designs/`; once it moves to
  `docs/reference/` those bare paths dangle. Only correct relative paths resolve.
- **Applies to:** all batches

### Decision: create-plus-delete-not-a-move

- **Decision:** promoting the design doc to the reference doc is a **`Creates:`
  of `docs/reference/plan-format-v3.md`** plus a **`Deletes:` (git rm) of
  `manifest/designs/plan-format-v3.md`** — it is deliberately NOT a `Moves:`
  pair, and there is **no `## Rename mechanic`** anywhere in this plan.
- **Rationale:** the content is substantially transformed on promotion (design-
  lifecycle framing removed, batch framing stripped, `NN.C`→`N` numbering,
  the detailed DAG/scheduling design *relocated out* into `webster-rewrite.md`,
  a coexistence note added, `## Related` links repointed). A `git mv` implies a
  preserved file; this is a rewrite-and-split, so create+delete is the honest
  encoding.
- **Applies to:** promote-v3-reference

### Decision: docs-only-verify

- **Decision:** each batch's `verify:` is `go build ./...` — a cheap sanity gate
  proving no Go source was accidentally touched (the v2 parser/validator and its
  `testdata/` plans must stay byte-identical). Link-consistency and
  worked-example internal consistency are verified by reading and by the grep
  commands listed in each batch's `## Batch Tests`, not by a runnable test.
- **Rationale:** the task changes only Markdown; there is no runtime surface a Go
  test exercises. `go build ./...` is fast, git-root-relative, and is exactly the
  "no code was touched" check the discussion names.
- **Applies to:** all batches

## All Files Touched

_Full union of every `Creates:` / `Edits:` / `Moves:` **target** path across
every batch, sorted alphabetically (`Deletes:` tokens excluded — they
disappear)._

- `docs/overview.md`
- `docs/reference/builder-contract.md`
- `docs/reference/model-spec.md`
- `docs/reference/plan-format-v3.md`
- `docs/reference/plan-format.md`
- `manifest/designs/codeintel-redesign.md`
- `manifest/designs/loom-planner.md`
- `manifest/designs/loom.md`
- `manifest/designs/webster-parallel-execution.md`
- `manifest/designs/webster-rewrite.md`
- `manifest/roadmap.md`
