# Batch: planparser-archive-name-and-loomshed-decorator

```yaml
task: 'loom: Plan-Write producer'
batch: 'planparser-archive-name-and-loomshed-decorator'
number: 1
cards: 3
verify: go test ./internal/planparser/... ./internal/loomshed/...
depends-on: []
```

## Batch Scope

This batch builds the two lowest layers of the Plan-Write producer: `planparser.ArchiveDirName`, the pure string helper that declares the plan directory's archive-subdirectory name, and `loomshed.NewPlanWrite`, the decorator that rotates the stale plan directory before delegating and commits after a `Done` outcome.
It is one batch because the decorator is the sole consumer of the helper and the two are meaningless apart: the helper exists only so the archive name is declared by the package that owns the plan directory's path vocabulary, per the Planparser Sole-Parser Invariant.
The external interface batch 2 consumes is `loomshed.NewPlanWrite(name string, inner shedengine.ShedProducer, commit func() error, anchorPath string, now func() time.Time) shedengine.ShedProducer`.
Batch-local decision, differing from nothing in `## Shared Decisions`: the compact UTC stamp layout `20060102T150405Z` is re-declared as an unexported package constant in `internal/loomshed` rather than shared from `internal/shedadapters`, because that package's own copy is unexported and `internal/loomshed`'s package doc already records deliberate duplication of `internal/shedadapters` helpers for exactly this reason.

## Cards

### Card 1: planparser declares the archive subdirectory name

- **Context:**
  - `internal/planparser/doc.go`
  - `CONSTRAINTS.md`
- **Edits:**
  - `internal/planparser/parse.go`
  - `internal/planparser/planpath_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Add an exported `ArchiveDirName(stamp, suffix string) string` to `internal/planparser/parse.go`, placed immediately after the existing `PlanDirRel` function and before `PlanDir`. It returns the concatenation `"archive-" + stamp + suffix` and performs no filesystem work whatsoever — it joins no anchor path, formats no time value, and stats nothing. Introduce an unexported `archiveDirPrefix = "archive-"` constant beside the existing `PlanDirName` constant and build the return value from it rather than from a repeated string literal, matching how `PlanDirRel` builds its own value from `PlanDirName`. Its doc comment must state that the caller supplies both the already-formatted compact UTC stamp and the already-chosen same-second collision suffix, that `internal/loomshed` is what performs the corresponding `os.MkdirAll`/`os.Rename` calls, and that the function stays in this package because the Planparser Sole-Parser Invariant makes `planparser` the sole declarer of the plan directory's path — a subdirectory of that directory being part of the same path vocabulary. Do not add any import to `internal/planparser/parse.go`; the helper must keep the package stdlib-only and must not turn it into a filesystem mutator. In `internal/planparser/planpath_test.go`, add `TestArchiveDirName`, a table test over at least the empty-suffix case (`stamp: "20260824T170000Z"`, `suffix: ""` → `"archive-20260824T170000Z"`) and the collision-suffix case (`suffix: "-1"` → `"archive-20260824T170000Z-1"`), following that file's existing `t.Errorf` message shape.
- **Commit:** `feat(planparser): declare the plan archive subdirectory name`

### Card 2: the PlanWrite rotate-then-delegate-then-commit decorator

- **Context:**
  - `internal/loomshed/discussionwrite.go`
  - `internal/loomshed/discussionwrite_test.go`
  - `internal/loomshed/planvalidate.go`
  - `internal/loomshed/ctx.go`
  - `internal/planparser/parse.go`
  - `internal/shedadapters/archive.go`
- **Edits:** none
- **Creates:**
  - `internal/loomshed/planwrite.go`
  - `internal/loomshed/planwrite_test.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** Write `internal/loomshed/planwrite_test.go` first, then `internal/loomshed/planwrite.go` — the expected behaviour is fully specified below before any code exists, so this card is a TDD card.

  `internal/loomshed/planwrite.go` declares an unexported `planWrite` struct with fields `name string`, `inner shedengine.ShedProducer`, `commit func() error`, `anchorPath string`, and `now func() time.Time`, a `var _ shedengine.ShedProducer = (*planWrite)(nil)` assertion, and an exported constructor `NewPlanWrite(name string, inner shedengine.ShedProducer, commit func() error, anchorPath string, now func() time.Time) shedengine.ShedProducer` that defaults a nil `now` to `time.Now` and returns the seam interface so `internal/shedrecipe` can construct it while `planWrite` stays unexported. Declare an unexported package constant `archiveTimestampFormat = "20060102T150405Z"` in this file, and say in its doc comment that it duplicates `internal/shedadapters`' identically-named unexported constant deliberately, the same way this package's `entryErr`/`cancelErr` already duplicate that package's own unexported helpers.

  `Call(ctx context.Context)` performs, in this exact order: (1) rotate the stale plan directory, returning `"", shedengine.OutputPointer{}, fmt.Errorf("loomshed: %s: rotate stale plan directory: %w", p.name, err)` on failure without ever touching `p.inner`; (2) `outcome, pointer, err := p.inner.Call(ctx)`, returning those three results verbatim whenever `err` is non-nil or `outcome != shedengine.Done`; (3) on `Done` with a nil error, invoke `p.commit()` and, on failure, return `"", shedengine.OutputPointer{}, fmt.Errorf("loomshed: %s: commit produced artifacts: %w", p.name, err)`; (4) otherwise return the inner outcome and pointer unchanged. `planWrite` must not consult `entryErr` or `cancelErr` itself — record in the `Call` doc comment that the wrapped `*shedadapters.SingleLLMProducer` entry-checks the context as its first act and owns the whole cancellation obligation, and that rotation running before that entry check means a run cancelled between the two leaves an archive directory behind and no new plan, which is acceptable because the archive is committed content rather than dirt and the next entry rotates an already-empty directory as a no-op.

  The rotation is an unexported method on `planWrite`. It resolves the plan directory as `planparser.PlanDir(p.anchorPath)` — never by naming the `_lyx` literal, which the Lyxdirs Single-Declarer Invariant forbids in production path-construction context. It calls `os.ReadDir` on that directory and returns a nil error when the error satisfies `os.IsNotExist`. It collects the names of entries that are neither directories (`e.IsDir()`) nor non-`.md` files, and returns a nil error without creating anything when that collection is empty. Otherwise it formats `p.now().UTC().Format(archiveTimestampFormat)` as the stamp, walks the suffix sequence `""`, `"-1"`, `"-2"`, … calling `planparser.ArchiveDirName(stamp, suffix)` for each and joining the result onto the plan directory, and takes the first candidate whose `os.Stat` reports not-exist (returning any other stat error as-is, mirroring `firstFreeArchivePath` in `internal/shedadapters/archive.go`). It then `os.MkdirAll`s that archive directory with mode `0o755` and `os.Rename`s each collected `.md` file into it. Only files move, never directories, so a second rotation can never nest a previous archive directory inside a new one.

  `internal/loomshed/planwrite_test.go` is modelled on `internal/loomshed/discussionwrite_test.go` and must not reuse that file's `fakeInnerProducer` or `commitRecorder` type names — both already exist in the same package, so declare differently-named fakes. Cover: `Done` with a nil error invokes commit exactly once and returns the inner outcome and pointer verbatim; `Stuck` leaves commit uninvoked and returns the inner results unchanged; a non-nil inner error leaves commit uninvoked; a commit failure surfaces as a returned error wrapping the original, with an empty outcome rather than `Stuck`, and with error text naming the producer; rotation happens before the inner `Call`, proven by an inner fake that records the plan directory's contents at call time and asserting it saw zero `.md` files; rotation moves every top-level `.md` file including `00-overview.md` into the archive directory with each file's content preserved; a pre-existing archive subdirectory is still present afterwards and was not nested inside the new one; rotation over an absent plan directory is a no-op with a nil error and creates nothing; rotation over an existing but empty plan directory creates no archive directory; two rotations under a clock pinned to one instant produce `archive-<stamp>` and `archive-<stamp>-1`; a rotation failure returns an error and leaves the inner producer's call count at zero, driven portably by pre-creating a regular file at the plan directory's own path so `os.ReadDir` fails with a not-a-directory error rather than a not-exist error; and `NewPlanWrite` with a nil `now` neither panics nor errors and still produces an archive directory. Every test builds its tree under `t.TempDir()` and spawns no process, keeping the file tier 1 per the Test Tier Purity Invariant.
- **Commit:** `feat(loomshed): add the PlanWrite rotate-and-commit decorator`

### Card 3: retire Plan-Write from the stub's declared row inventory

- **Context:**
  - `internal/loomshed/planwrite.go`
  - `internal/loomshed/loomshed.go`
- **Edits:**
  - `internal/loomshed/stub.go`
  - `internal/loomshed/doc.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** In `internal/loomshed/stub.go`, `stubProducer`'s doc comment currently reads "It backs four rows of loom's 13-row producer list that no task has built for real yet -- Discussion-Review, Plan-Write, Plan-Review, and Webster-Review". Change "four rows" to "three rows" and drop `Plan-Write` from the list, leaving Discussion-Review, Plan-Review, and Webster-Review. In `internal/loomshed/doc.go`, the package doc opens "Package loomshed owns loom's own seven producer constructors" — change `seven` to `eight`, since `NewPlanWrite` is the eighth. Make no other change to either file; the code in both is untouched.
- **Commit:** `docs(loomshed): drop Plan-Write from the stub's row inventory`

## Batch Tests

`verify: go test ./internal/planparser/... ./internal/loomshed/...` covers both packages this batch touches.
`internal/planparser` is included because card 1 adds `ArchiveDirName` and its table test there;
`internal/loomshed` is included because card 2 adds `planwrite.go`/`planwrite_test.go` and card 3 edits two doc comments in it.
Both packages are untagged tier 1, so the run is fast and needs no build tag.
No third package is affected: nothing outside `internal/loomshed` calls `NewPlanWrite` yet, and `ArchiveDirName` is a pure addition with no existing caller.
