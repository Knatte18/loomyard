# Batch: shedrun-leaf

```yaml
task: 'Seeded Shed core: run addressing, seed contract, batten'
batch: shedrun-leaf
number: 1
cards: 4
verify: go test ./internal/shedrun/...
depends-on: []
```

## Batch Scope

This batch creates `internal/shedrun`, the new leaf package that is the sole declarer of the `shed` durable-directory segment, the run-id vocabulary, and the `seed.json` contract, and records the Shed Run-Directory Invariant that binds it.
It is one batch because the package is a pure leaf over told paths with one JSON file's worth of I/O, and because every later batch consumes it: batch 4 retargets loom's status and run-lock paths onto it, batch 6 retargets batten's, and batch 7 builds the `lyx shed` run-addressing pre-run on top of `ReadSeed`/`List`.

The external interface later batches consume: ten path constructors taking a `*lyxcwd.Location` and a run-id, the `SelfRunID`/`ValidateRunID`/`IsReserved` trio, the `Seed` struct with `ReadSeed`/`WriteSeed`/`List`, and the two closed vocabularies (`DriverGo`/`DriverLLM` and `RecipeLoom`/`RecipeBatten`).

Batch-local decision beyond the overview's: the package performs no cwd resolution of any kind.
Every constructor is a plain `filepath.Join` onto the told `*lyxcwd.Location`'s `AnchorPath()`, and the `_lyx`/`.lyx` segments come from `lyxdirs.LyxDirName`/`lyxdirs.DotLyxDirName`, never as literals.
`internal/lifecyclecli/paths.go` and `internal/loomengine/config.go` are the two shapes to copy.

## Cards

### Card 1: shedrun path constructors

- **Context:**
  - `internal/lifecyclecli/paths.go`
  - `internal/loomengine/config.go`
  - `internal/lyxcwd/lyxcwd.go`
  - `internal/lyxdirs/dirs.go`
- **Edits:** none
- **Creates:**
  - `internal/shedrun/doc.go`
  - `internal/shedrun/paths.go`
  - `internal/shedrun/paths_test.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** Create package `shedrun` with a private `shedDirName = "shed"` constant, declared here and nowhere else in the repository.
  Declare ten exported constructors, each taking `l *lyxcwd.Location` first.
  Durable, under `filepath.Join(l.AnchorPath(), lyxdirs.LyxDirName, shedDirName, runID)`: `RunDir(l, runID)`, `SeedFile(l, runID)` ending in `seed.json`, `StatusFile(l, runID)` ending in `status.json`.
  Ephemeral, under `filepath.Join(l.AnchorPath(), lyxdirs.DotLyxDirName, shedDirName, runID)`: `ScratchDir(l, runID)`, `RunLock(l, runID)` ending in `run.lock`, `StatusLock(l, runID)` ending in `status.json.lock`, `LastCommitMarker(l, runID)` ending in `last-commit`.
  Anchor-relative, taking only `runID` and no `*lyxcwd.Location`, for `fabricengine.CommitAnchoredPaths`' `relPaths` argument: `SeedRel(runID)` and `StatusRel(runID)`, each `filepath.Join(lyxdirs.LyxDirName, shedDirName, runID, <filename>)`.
  Hub-scoped, one level above any run-id and taking no run-id at all: `PrimeRunLock(l)` at `filepath.Join(l.AnchorPath(), lyxdirs.DotLyxDirName, shedDirName, "run.lock")`.
  Write `doc.go` stating the package's sole-declarer role over the segment and over `seed.json`, and stating that the seed meant here is the new run-identity artefact, never `internal/loomengine`'s `CheckSeed` sense (the initial `status.json` for a fresh run) and never `shedverbs.KindUnseeded`'s sense (also the status file).
  `paths_test.go` covers every constructor against a synthetic `*lyxcwd.Location`, asserts `RunLock` never equals `StatusLock` for any run-id (`shedengine.Shed` rejects `LockPath == StatusLockPath` outright), and asserts `PrimeRunLock` equals neither for any run-id.
- **Commit:** `feat(shedrun): declare the shed run-directory path constructors`

### Card 2: run-id vocabulary and validation

- **Context:**
  - `internal/shedrun/paths.go`
- **Edits:** none
- **Creates:**
  - `internal/shedrun/runid.go`
  - `internal/shedrun/runid_test.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** Declare `SelfRunID = "self"`, the literal default meaning "this worktree's own primary run".
  Declare `ValidateRunID(runID string) error`, which accepts a single path segment only: it refuses an empty string, a string containing `/` or `\`, the exact strings `.` and `..`, and anything `filepath.Base` does not return unchanged.
  It **accepts** `self`, which must stay addressable because it is the default.
  Declare `IsReserved(runID string) bool`, returning true for `self` alone.
  `IsReserved` is the check consulted wherever a run-id derives from a Board slug — a task slugged `self` would collide with the reserved meaning — and never on an addressing path, where `self` is always legal.
  Say which of the two is which in each function's doc comment.
  `runid_test.go` drives `ValidateRunID` as a table with the traversal cases as explicit rows (`..`, `a/b`, `/abs`, the empty string) plus `self` and an ordinary slug, and asserts `IsReserved` is true for `self` and false for every other row.
- **Commit:** `feat(shedrun): add the run-id vocabulary and single-segment validation`

### Card 3: the seed contract

- **Context:**
  - `internal/shedrun/paths.go`
  - `internal/shedrun/runid.go`
  - `internal/state/state.go`
- **Edits:** none
- **Creates:**
  - `internal/shedrun/seed.go`
  - `internal/shedrun/seed_test.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** Declare `type Seed struct` with three JSON fields: `Recipe string` (`json:"recipe"`), `Driver string` (`json:"driver"`), and `Params map[string]string` (`json:"params,omitempty"`).
  Declare the closed driver vocabulary `DriverGo = "go"` and `DriverLLM = "llm"`, plus `ValidateDriver(driver string) error` which accepts `DriverGo`, refuses `DriverLLM` with a message naming the `seeded driver choice: ly-drive strand as the child's driver` roadmap item that will implement it, and refuses any other value as unknown.
  Declare the closed recipe vocabulary `RecipeLoom = "loom"` and `RecipeBatten = "batten"`, plus `RecipeNames() []string` returning them sorted in a freshly allocated slice, and `ValidateRecipe(name string) error` naming the available recipes on refusal.
  Both vocabularies live here per the overview's `shedrun-owns-the-recipe-name-vocabulary` Shared Decision.
  Declare `ReadSeed(l *lyxcwd.Location, runID string) (Seed, bool, error)`: it calls `ValidateRunID` first, reads `SeedFile(l, runID)` with no lock (the seed is write-once and never mutated mid-run), reports `found == false` when the file is absent, decodes strictly so an unknown key is an error, defaults an absent or empty `Driver` to `DriverGo`, and validates the decoded `Driver` and every `Params` key being non-empty.
  Declare `WriteSeed(l *lyxcwd.Location, runID string, seed Seed) error`: it calls `ValidateRunID`, `ValidateRecipe` and `ValidateDriver`, creates `RunDir(l, runID)`, and is idempotent against a byte-identical existing seed while refusing a disagreeing one with a message naming both the existing and the incoming values.
  Declare `List(l *lyxcwd.Location) ([]string, error)`: it enumerates directories directly under `filepath.Join(l.AnchorPath(), lyxdirs.LyxDirName, shedDirName)`, keeps only those containing a readable `seed.json`, returns the result sorted, and returns an empty slice with no error when the parent directory does not exist at all.
  Declare `MissingSeedMessage(prefix, runID string, existing []string, remedy string) string`, the one shared renderer for the "no seed at this run-id" refusal: it names the addressed run-id, lists `existing` sorted, says plainly that no run is seeded yet when the list is empty, and closes with `remedy`.
  Both `internal/loomcli` and `internal/battencli` call it so the two paths word the refusal identically, and neither may import the other.
  The rendered text carries no `kind` field and no structure a caller could mistake for one — the `step` refusal-kind vocabulary is pinned closed at five values, and a missing run is not a sixth.
  `seed_test.go` covers the write/read round-trip, the absent-`driver`-means-`go` default, the `llm` refusal naming the roadmap item, the unknown-driver and unknown-recipe refusals, `WriteSeed` idempotency against an identical seed and refusal of a disagreeing one, and `List` over a directory holding one valid run, one directory with no `seed.json`, and one unreadable entry, asserting sorted output, and `MissingSeedMessage` naming the addressed run-id, rendering a sorted listing, and reading sensibly when the listing is empty.
- **Commit:** `feat(shedrun): add the seed.json contract, driver and recipe vocabularies`

### Card 4: record the Shed Run-Directory Invariant

- **Context:**
  - `internal/shedrun/paths.go`
  - `internal/shedrun/seed.go`
- **Edits:**
  - `CONSTRAINTS.md`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Add a new `## Shed Run-Directory Invariant` section to `CONSTRAINTS.md`, placed immediately after the existing `## Shed Verb-Set Invariant` section so the four Shed invariants stay adjacent.
  Its body states that `internal/shedrun` is the sole declarer of the `shed` path segment and of the run-id vocabulary including the literal `self`, and the sole parser and writer of `seed.json`; that no other production file names the segment or the filename in path-construction context and no other package decodes or encodes the seed struct; and that a run-id is validated as a single path segment before being joined onto any anchor.
  Write it in the same clipped form the surrounding invariants use — a one-line claim under the heading, then bullets — and follow this repo's semantic-line-break convention.
- **Commit:** `docs(constraints): record the Shed Run-Directory Invariant`

## Batch Tests

`verify: go test ./internal/shedrun/...` runs the three test files this batch creates — `paths_test.go`, `runid_test.go` and `seed_test.go` — and nothing else.
The scope is exactly right for the batch: `internal/shedrun` is brand new, has no callers yet, and no existing package's behaviour changes here.
Every test is Tier 1 by construction: the package resolves nothing, spawns no git, and its only I/O is reading and writing JSON files under a `t.TempDir()`-rooted synthetic `*lyxcwd.Location`, so neither the Test Tier Purity Invariant's spawn ban nor the Hermetic Git Test Environment Invariant's `TestMain` obligation is engaged.

Card 4 adds no runnable surface; the `CONSTRAINTS.md` edit is covered by review discipline, matching how every other invariant in that file is recorded.
