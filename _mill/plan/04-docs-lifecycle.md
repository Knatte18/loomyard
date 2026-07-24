# Batch: docs-lifecycle

```yaml
task: 'gitrepo: generic, repo-agnostic git primitives'
batch: docs-lifecycle
number: 4
cards: 2
verify: go build ./internal/gitrepo/
depends-on: [1, 2, 3]
```

## Batch Scope

Closes the documentation lifecycle for the shipped module: folds the durable design rationale from
`manifest/designs/gitrepo.md` into the package doc (`doc.go`), deletes the manifest design draft,
retargets the three inbound links that pointed at it, moves the roadmap item Planned → Done, and
adds `gitrepo` to the `docs/overview.md` module map. Depends on batches 1–3 because the package
doc describes the complete, implemented API surface. No runnable behavior changes; `verify` is a
compile check that `doc.go` still builds.

## Cards

### Card 10: fold rationale into package doc, delete manifest draft, retarget links

- **Context:**
  - `manifest/designs/gitrepo.md`
  - `docs/overview.md`
  - `_mill/discussion.md`
- **Edits:**
  - `internal/gitrepo/doc.go`
  - `manifest/designs/fabric.md`
  - `manifest/designs/semantic-index.md`
- **Creates:** none
- **Deletes:**
  - `manifest/designs/gitrepo.md`
- **Moves:** none
- **Requirements:** Read `manifest/designs/gitrepo.md` in full and expand the `// Package gitrepo …`
  comment in `doc.go` to carry its durable rationale: purpose, the relationship to `internal/gitexec`,
  the `Repo` API, the self-correcting snapshot pattern, `SHAExists` history-rewrite safety, scope
  boundaries (not a general-purpose git wrapper), the push surface (`Push`/`PushCoalesced`
  push-only, rebase-retry trigger set, single-pusher lock `.gitrepo-push.lock`), and the snapshot
  remote model (FF-only adopt-on-conflict, fetch-degrades-to-local, monotonically-forward
  precondition). Then delete `manifest/designs/gitrepo.md` (Documentation Lifecycle: a module-design
  draft is removed when its module lands). In `manifest/designs/fabric.md` and
  `manifest/designs/semantic-index.md`, retarget every link to the deleted draft — `[gitrepo](gitrepo.md)`,
  `[gitrepo.md](gitrepo.md)`, and anchor deep-links like `(gitrepo.md#shaexists…)` — to reference the
  shipped package instead: inline `` `internal/gitrepo` `` (dropping the now-dead `#…` anchors) or a
  path link to `../../internal/gitrepo/doc.go`. Do not alter surrounding prose meaning.
- **Commit:** `docs(gitrepo): fold design rationale into package doc, delete manifest draft`

### Card 11: roadmap Planned→Done and overview module map

- **Context:**
  - `_mill/discussion.md`
- **Edits:**
  - `manifest/roadmap.md`
  - `docs/overview.md`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** In `manifest/roadmap.md`, remove the `gitrepo` item from `## Planned` and add a
  `gitrepo` entry under `## Done` in that section's existing entry format. Per the roadmap's own
  Maintenance convention, Done entries do **not** link (the design doc is deleted) — reference the
  shipped package as an inline code-span `` `internal/gitrepo` ``, matching existing Done entries,
  not a markdown link. (The Planned list uses
  repeated `1.` markdown auto-numbering, so no manual renumber is needed — confirm the remaining
  items still render sequentially.) In `docs/overview.md`, add a `internal/gitrepo/` line to the
  file-tree next to the existing `internal/gitexec/` entry (e.g. `typed Repo over one local git
  checkout, built on gitexec`), and add `internal/gitrepo` to the shared-infrastructure list
  sentence that currently enumerates `internal/configengine`, `internal/gitexec`, `internal/lock`,
  ….
- **Commit:** `docs: move gitrepo to Done and add it to the module map`

## Batch Tests

`verify: go build ./internal/gitrepo/` confirms the expanded `doc.go` package comment still
compiles. The rest of the batch is markdown (roadmap, overview, two design docs) and a file
deletion, which have no runnable surface; the compile check is the only automated gate and is
sufficient because no `.go` logic changes here.
