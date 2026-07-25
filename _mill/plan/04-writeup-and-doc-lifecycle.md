# Batch: writeup-and-doc-lifecycle

```yaml
task: 'git-native-library: feasibility spike'
batch: writeup-and-doc-lifecycle
number: 4
cards: 2
verify: go test -tags integration ./internal/gitnativepoc/ && go test ./cmd/lyx/ -run 'TestTierPurity_UntaggedTestsSpawnNothing|TestHermeticGitEnv_GitSpawningPackagesHaveTestMain'
depends-on: [2, 3]
```

## Batch Scope

This batch turns the empirical results from batches 2 and 3 into the durable
deliverable and closes the documentation lifecycle. It authors the spike write-up
as the package godoc (`internal/gitnativepoc/doc.go`) — the recommendation, the
per-operation MIGRATE/CLI-BOUND table, the evidence, and the pinned go-git
version — then deletes the now-superseded design draft, moves the roadmap item to
Done (no link), and adds the package to the `docs/overview.md` module map. It
depends on both batch 2 and batch 3 because the verdicts it records come from
running their parity harness. The implementer MUST run
`go test -tags integration ./internal/gitnativepoc/` and read the verdict
comments in `read_test.go`/`write_test.go` before writing the table — the
write-up reports observed results, never guesses.

## Cards

### Card 11: author the spike write-up as package godoc

- **Context:**
  - `_mill/discussion.md`
  - `internal/gitnativepoc/read_test.go`
  - `internal/gitnativepoc/write_test.go`
  - `internal/gitnativepoc/read.go`
  - `internal/gitnativepoc/write.go`
  - `internal/gitrepo/doc.go`
  - `manifest/designs/git-native-library.md`
  - `go.mod`
- **Edits:** none
- **Creates:**
  - `internal/gitnativepoc/doc.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** Create `internal/gitnativepoc/doc.go` in `package gitnativepoc`
  as the **canonical package-doc file** (a `// Package gitnativepoc ...` godoc
  comment; this is the single package-doc comment for the package —
  `gitnativepoc.go` carries only a plain file comment). The godoc must contain:
  (1) the overall recommendation — one of **ADOPT** / **ADOPT-PARTIAL** /
  **DECLINE** — chosen from the observed verdicts; (2) the per-operation
  **MIGRATE / CLI-BOUND** table covering every operation exercised in batches 2–3
  (CurrentSHA, SHAExists, ChangedFilesSince, SnapshotSHA, remoteName, hasUnpushed,
  isStrictDescendant, StageAndCommit, StageAllAndCommit, Push/rebase-retry,
  SetSnapshotSHA), each with the deciding rubric gate ((a) typed, (b) parity,
  (c) Windows) and the evidence (which test proves it); (3) the pinned go-git
  version, read from `go.mod`, recorded verbatim; (4) an explicit statement that
  this write-up **supersedes** `manifest/designs/git-native-library.md`'s
  "read-only subset" framing — the spike widened to the full write surface
  including rebase-retry; (5) any Windows-hinged verdict marked **Win11-pending**,
  since verification ran on Linux only. Base every verdict on the actual test
  output, not assumption. Keep it as Go doc-comment prose (godoc-rendered), not
  Markdown tables that break godoc.
- **Commit:** `docs(gitnativepoc): spike write-up as package godoc`

### Card 12: close the documentation lifecycle

- **Context:**
  - `CONSTRAINTS.md`
  - `_mill/discussion.md`
- **Edits:**
  - `manifest/roadmap.md`
  - `docs/overview.md`
- **Creates:** none
- **Deletes:**
  - `manifest/designs/git-native-library.md`
- **Moves:** none
- **Requirements:** (1) Delete `manifest/designs/git-native-library.md` — a
  mechanical design draft for a now-landed spike; its own header anticipates
  deletion, and the durable write-up now lives in `doc.go`. (2) In
  `manifest/roadmap.md`, move the `git-native-library: feasibility spike` item
  from the `## Planned` section to the `## Done` section, **with no link** (Done
  entries deliberately do not link — roadmap Maintenance lines 207–211), and drop
  the now-dangling `[designs/git-native-library.md](...)` reference from the moved
  entry; keep the remaining Planned numbering intact (the auto-renumbering `1.`
  convention handles ordinals). (3) In `docs/overview.md`, add a one-line entry
  for `internal/gitnativepoc/` to the shared-lib ASCII module map (the block
  listing `internal/gitexec`, `internal/gitrepo`, etc.), describing it as the
  throwaway-but-kept go-git feasibility-spike package (experimental, not wired
  into production). Do not add a `CONSTRAINTS.md` entry — a non-registered
  experimental package introduces no new cross-cutting invariant (confirmed
  against CONSTRAINTS.md in Context).
- **Commit:** `docs: close git-native-library doc lifecycle (delete draft, roadmap Done, overview map)`

## Batch Tests

`verify` runs `go test -tags integration ./internal/gitnativepoc/` — which
compiles `doc.go` (the write-up package doc) as part of the package and re-runs
the full parity suite, confirming the write-up's evidence still holds — plus the
two scoped guard tests to confirm the package still satisfies the Test Tier
Purity and Hermetic Git Test Environment Invariants after the doc changes. The
roadmap/overview/design-doc edits have no runnable surface; they are verified by
review against the documentation-lifecycle conventions cited in the card.
