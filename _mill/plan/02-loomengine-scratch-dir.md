# Batch: loomengine-scratch-dir

```yaml
task: 'landing: parent-fabric resolution chain'
batch: loomengine-scratch-dir
number: 2
cards: 2
verify: go test ./internal/loomengine/...
depends-on: []
```

## Batch Scope

This batch adds the one accessor `landingshed.Deps.ScratchDir` needs from `internal/loomengine`: `LoomScratchDir(l)`, at loom's existing `.lyx/loom/` mirrored-subpath directory, per the Durable-vs-Ephemeral State Invariant's "each module exposes a scratch accessor beside its durable one" rule.
It also corrects `LoomStatusLock`'s doc comment, which currently asserts that accessor does not exist.
This batch is independent of batch 1 and batch 3; batch 4 depends on it for `LoomScratchDir` alone.

No card in this batch has a non-empty `Moves:`.

## Cards

### Card 9: Add `LoomScratchDir` and correct `LoomStatusLock`'s doc comment

- **Context:** none
- **Edits:**
  - `internal/loomengine/config.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  Add a new exported function to `config.go`, placed immediately after `LoomBootstrapLock` (the last of the four `.lyx`-anchored accessors in the file): `func LoomScratchDir(l *lyxcwd.Location) string { return filepath.Join(l.AnchorPath(), lyxdirs.DotLyxDirName, loomDirName) }` — the directory `LoomRunLock`, `LoomDriverLog`, and `LoomBootstrapLock` already live in, named as its own accessor rather than those three continuing to inline the join.
  Write its doc comment in this file's existing style (see `LoomBootstrapLock`'s own comment for the pattern): state it is `AnchorPath`-anchored like the other three, and that it names the directory those three already share.

  In `LoomStatusLock`'s existing doc comment (config.go:81-90), delete the clause `` "loomengine has no Dir(l) accessor for a ScratchDir(l) to mirror" `` — that clause is now false — and reword the sentence it sits in so it still reads correctly with the clause removed (the surrounding point, that the lock is stated outright at its mirrored `.lyx` subpath rather than derived by analogy, stays true and stays in the comment).
- **Commit:** `loom: add LoomScratchDir, correct LoomStatusLock's stale doc comment`

### Card 10: Test `LoomScratchDir`'s mirrored placement

- **Context:** none
- **Edits:**
  - `internal/loomengine/config_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  Add a new test function to `config_test.go`, `TestLoomScratchDir_MirrorsRunLockDriverLogAndBootstrapLockParent`, asserting `filepath.Dir(LoomScratchDir(loc))` equals `filepath.Dir(LoomRunLock(loc))`, `filepath.Dir(LoomDriverLog(loc))`, and `filepath.Dir(LoomBootstrapLock(loc))` for a hand-built `*lyxcwd.Location` — the assertion that keeps the four from drifting apart, per the plan's own testing note for this accessor.
  Build the `*lyxcwd.Location` the same way this file's existing tests do (a bare `&lyxcwd.Location{...}` over a `t.TempDir()`, no fixture, no git spawn — this file stays untagged Tier 1).
- **Commit:** `loom: test LoomScratchDir's mirrored placement`

## Batch Tests

`verify: go test ./internal/loomengine/...` — both cards are untagged Tier-1 changes to `config.go`/`config_test.go`, no fixture, no git spawn, per the `verify-scoped-per-package` Shared Decision.
