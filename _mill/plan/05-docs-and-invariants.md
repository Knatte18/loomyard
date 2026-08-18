# Batch: docs-and-invariants

```yaml
task: "lift the orchestrator preflight out of loomengine, plus the shared standalone-CLI foundations"
batch: "docs-and-invariants"
number: 5
cards: 3
verify: go test ./internal/lyxcwd/...
depends-on: [1, 2, 3, 4]
```

## Batch Scope

This batch lands the repo-level documentation and invariant edits the project's task-completion rule requires for a task that adds modules: three rows in `docs/overview.md`'s directory tree plus its shared-infrastructure sentence, two bullets in `docs/shared-libs/README.md`, and two new leaf-invariant sections in `CONSTRAINTS.md`.
It depends on all four code batches because every sentence it writes describes something those batches shipped — in particular the two enforcement tests the new `CONSTRAINTS.md` sections must name as their enforcement basis.
Each of the three new packages also carries its own `doc.go`, written in the batch that created it, so nothing here duplicates package-level documentation.

Batch-local decision, carried from the discussion and authoritative over any reviewer instinct to be symmetric: `internal/preflight` gets a `docs/overview.md` tree row and its `doc.go`, and **nothing else**.
It is deliberately absent from `docs/shared-libs/README.md` and from the shared-infrastructure sentence, because that file's stated line is that a shared lib does one mechanical thing and carries no domain logic, and `preflight` carries orchestrator precondition policy — which checks constitute readiness, how a failure is classified, what blocks a downstream read.
`internal/buildinfo` and `internal/standalonestate` are mechanical in exactly the intended sense and do belong there.
`manifest/roadmap.md` is deliberately untouched: the roadmap moves on completing a planned wave, not per task.

## Cards

### Card 19: add the three packages to the overview's module tree

- **Context:**
  - `internal/preflight/doc.go`
  - `internal/buildinfo/doc.go`
  - `internal/standalonestate/doc.go`
- **Edits:**
  - `docs/overview.md`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  Add three rows to the fenced directory tree, in the block that already lists `internal/hubgeom/`, `internal/lyxcwd/`, `internal/lyxdirs/`, `internal/modelspec/` and `internal/tokenvocab/`, matching the existing two-space-aligned description column:

  - `internal/preflight/` — the orchestrator-agnostic tier-1 and tier-2 precondition checks (geometry, worktree-pair cleanliness, Fabric readiness and sync) plus the shared `Report` result type, placed next to `internal/hubgeom/`.
  - `internal/buildinfo/` — the ldflags-stamped build channel, a zero-import leaf.
  - `internal/standalonestate/` — the target-path-to-`hash8`-and-state-directory derivation, a stdlib-only leaf.

  The tree's final entry uses the `└──` box-drawing prefix; every inserted row uses `├──` and the final entry keeps `└──`, so the tree does not render broken.

  Separately, extend the shared-infrastructure sentence that currently enumerates `internal/configengine`, `internal/gitexec`, `internal/gitrepo`, `internal/lock`, `internal/logger`, `internal/output`, `internal/lyxcwd`, `internal/lyxdirs`, `internal/state`, `internal/shell`, `internal/modelspec`, `internal/tokenvocab` and `internal/pattern`, adding `internal/buildinfo` and `internal/standalonestate` — and **not** `internal/preflight`, per this batch's scope note.

  Write prose one sentence per line with no fixed-column hard wrap, matching the file's existing style.
  Do not use the tokens `weft` or `warp` in the added text.
  Add no new markdown links: `docs/` is a scanned source for the Markdown Link Integrity check, so any link written here must resolve, file part and `#anchor` alike.
- **Commit:** `docs(overview): add preflight, buildinfo and standalonestate to the module tree`

### Card 20: list the two new leaves as implementation-only libraries

- **Context:**
  - `internal/buildinfo/doc.go`
  - `internal/standalonestate/doc.go`
  - `docs/overview.md`
- **Edits:**
  - `docs/shared-libs/README.md`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  Add exactly two bullets to the `## Implementation-only libraries` section, matching the existing `internal/fsx` / `internal/lock` / `internal/state` bullet shape (a backticked import path, an em dash, a one-line description):

  - `internal/buildinfo` — the ldflags-stamped build channel (`Channel`, `IsDev`), a zero-import leaf so any CLI can read it with no cycle risk; the mapping to a stencil mode lives in `stencilstore.ModeFor`.
  - `internal/standalonestate` — pure derivation from an absolute target path to a `hash8` and its per-OS state directory, creating nothing on disk.

  Add them to `## Implementation-only libraries`, never to `## Libraries`: that section's contract is one dedicated `<name>.md` doc file per entry, and both of these are documented in their own `doc.go`, exactly as `internal/modelspec` and `internal/state` already are.
  Create no new `.md` file under `docs/shared-libs/`.
  Do **not** add `internal/preflight` to either section, per this batch's scope note.
  Write one sentence per line with no fixed-column hard wrap, and add no new markdown links — this file is a scanned source for the Markdown Link Integrity check.
- **Commit:** `docs(shared-libs): list buildinfo and standalonestate as implementation-only`

### Card 21: record the two new leaf invariants

- **Context:**
  - `internal/buildinfo/leaf_enforcement_test.go`
  - `internal/standalonestate/leaf_enforcement_test.go`
  - `internal/tokenvocab/leaf_enforcement_test.go`
  - `tools/deploy/main_test.go`
- **Edits:**
  - `CONSTRAINTS.md`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  Insert two new sections immediately after the existing `## Tokenvocab Leaf Invariant` section and before `## Scout Engine-Seam Invariant`, so they sit with the file's other leaf invariants.
  Insert there specifically to keep the edit disjoint from the `## Pattern Leaf Invariant` section further down, which a sibling task in this wave rewords — the two edits then rebase as a mechanical append-versus-reword rather than a conflict.

  `## Buildinfo Leaf Invariant` states that `internal/buildinfo` production code imports nothing at all — not even the standard library — so `cmd/lyx` and every standalone CLI package can read the build channel with no cycle risk;
  that the package exposes `Channel` and `IsDev()` only, and deliberately does not return a `stencilstore.Mode`, because `internal/stencilstore` imports `internal/logger` and `internal/stencil` and returning its type would destroy the leaf property;
  that the mapping site is `stencilstore.ModeFor`;
  and that the ldflags stamp path `github.com/Knatte18/loomyard/internal/buildinfo.Channel` is guarded against silent drift by a test in `tools/deploy/main_test.go`, because Go's linker does not error on an unmatched `-X`.
  Close with an **Enforced by** line naming `internal/buildinfo/leaf_enforcement_test.go` (`TestLeafInvariant_AllowlistOnly`).

  `## Standalonestate Leaf Invariant` states that `internal/standalonestate` production code imports only the standard library, with no permitted non-stdlib import;
  that the package never resolves a working directory — no `filepath.Abs`, no `os.Getwd` — and rejects a relative target with an error, keeping cwd resolution wholly with `internal/lyxcwd` per the Cwd Resolution Invariant;
  and that `Derive` creates nothing on disk.
  Close with an **Enforced by** line naming `internal/standalonestate/leaf_enforcement_test.go` (`TestLeafInvariant_AllowlistOnly`), and note that the no-`filepath.Abs` half is a review obligation rather than a machine check.

  Match the file's own house style: rules only, no rationale narrative, no incident history, bullet form, one sentence per line, and an **Enforced by** line naming the test that fails the build.
  Do not add the three-tier invariant and do not reword the Cwd Resolution Invariant — both belong to a later task in this decomposition, and writing the three-tier rule now would pin a model the producer packages do not yet implement.
  Add no markdown links: `CONSTRAINTS.md` is not a scanned source for the Markdown Link Integrity check, so any link written inside these sections is checked by nobody.
- **Commit:** `docs(constraints): add the buildinfo and standalonestate leaf invariants`

## Batch Tests

`verify:` runs `go test ./internal/lyxcwd/...`.

That package hosts `docslink_test.go`'s `TestEnforcement_MarkdownLinks`, which scans `manifest/` and `docs/` as link sources and therefore covers both of this batch's documentation edits — every link in the edited `docs/overview.md` and `docs/shared-libs/README.md` is resolved, file part and `#anchor` alike.
It also hosts `enforcement_test.go`'s `TestEnforcement_FabricVocabulary`, whose `.md` half walks `internal/**/*.md` and `contracts/stencils/**/*.md`; the files edited here sit outside that walk, but the Go half still covers the three new packages' `doc.go` files from the earlier batches, so a vocabulary slip anywhere in the task's production surface fails here as well.

No test covers the `CONSTRAINTS.md` edit, and that is a stated property of the repo rather than a gap in this plan: the Markdown Link Integrity invariant says outright that `.md` files outside `manifest/` and `docs/` — root docs such as `CONSTRAINTS.md` included — have their outgoing links checked by nobody.
Links *pointing at* the new `CONSTRAINTS.md` anchors from a scanned file would be checked; links written *inside* the new sections are a review obligation.
This batch adds none, which is why card 21 forbids them outright.
