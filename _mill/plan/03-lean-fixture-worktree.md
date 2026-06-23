# Batch: lean-fixture-worktree

```yaml
task: "Speed up and stabilize the integration test tier"
batch: lean-fixture-worktree
number: 3
cards: 3
verify: go test -tags integration ./internal/worktree ./internal/lyxtest -count=1
depends-on: []
```

## Batch Scope

Cut the wasted weft-bare copy from the paired fixture for the `SkipPush:true` worktree
tests. `Add` always pushes the *host* branch (add.go:172, unconditional) but `SkipPush`
suppresses only the *weft* push (add.go:182-183), so the host bare must stay while the
weft-bare is dead weight for every current `Add` test. A new lean builder copies three of
the four repos, trimming ~25% of the per-test filesystem-copy + Defender cost that is the
worktree package's real wall-clock floor. Because no existing test pushes the weft branch,
this batch also adds one weft-pushing test on the full `CopyPaired` to keep that builder
exercised and to cover the previously-uncovered `pushWeftBranch` path. Independent of the
board batches (touches only `internal/lyxtest` + `internal/worktree`).

External interface: `lyxtest.CopyPairedLocal` (new). Batch-local decision: the lean
fixture leaves `WeftBare == ""` and leaves the weft-prime's origin URL unrewritten (it is
never reached when the weft push is suppressed).

## Cards

### Card 11: Add CopyPairedLocal lean fixture

- **Context:**
  - `internal/lyxtest/doc.go`
  - `internal/paths/paths.go`
- **Edits:**
  - `internal/lyxtest/lyxtest.go`
- **Creates:** none
- **Deletes:** none
- **Requirements:** Add `func CopyPairedLocal(tb testing.TB) PairedFixture` to `lyxtest.go`,
  modeled on `CopyPaired` (lyxtest.go:500) but **omitting the weft-bare**. It copies the
  host hub (`buildHostHub`), the host bare, and the weft-prime (`buildWeftPrime`) into
  `tb.TempDir()` via `copyDirRecursive`, rewrites only the copied hub's origin URL to the
  copied host bare via `rewriteOriginURLInConfig`, resolves `Layout` via `paths.Resolve`
  on the copied hub, and returns a `PairedFixture` with `Container`, `Hub`, `Bare`,
  `WeftPrime`, `Layout` populated and **`WeftBare: ""`**. Do NOT copy the weft-bare and do
  NOT rewrite the weft-prime's origin URL (it points at the shared template weft-bare and is
  never reached under `SkipPush:true`). Add a doc comment stating it is the lean variant for
  `SkipPush:true` tests and that pushing the weft branch against it is unsupported.
- **Commit:** `feat(lyxtest): add CopyPairedLocal lean fixture (no weft-bare)`

### Card 12: Switch SkipPush worktree tests to the lean fixture

- **Context:**
  - `internal/lyxtest/lyxtest.go`
  - `internal/worktree/add.go`
- **Edits:**
  - `internal/worktree/weft_test.go`
  - `internal/worktree/add_test.go`
  - `internal/worktree/remove_test.go`
- **Creates:** none
- **Deletes:** none
- **Requirements:** Replace `lyxtest.CopyPaired(t)` with `lyxtest.CopyPairedLocal(t)` at the
  ten call sites: `weft_test.go` lines 26, 58, 113, 212, 254; `remove_test.go`
  lines 138, 192, 225; `add_test.go` lines 121, 180. Nine of these call
  `Add(..., AddOptions{SkipPush: true})` (host push only). The tenth, `weft_test.go:254`
  (`TestWeftRollbackOnPostHostCreateFailure`), does **not** call `Add` — it invokes
  `rollbackAdd` directly — but it never pushes the weft branch either, so the lean fixture is
  equally safe there. None of the ten read `f.WeftBare`, so the swap is a drop-in. Do not
  change any assertions or the `Add` options. Leave
  `cli_test.go`/`launchers_test.go`/`portals_test.go`/`list_test.go` (which use
  `CopyHostHub`, not `CopyPaired`) untouched.
- **Commit:** `test(worktree): use CopyPairedLocal for SkipPush tests`

### Card 13: Add a weft-pushing test on the full fixture

- **Context:**
  - `internal/lyxtest/lyxtest.go`
  - `internal/worktree/add.go`
  - `internal/worktree/weft.go`
- **Edits:**
  - `internal/worktree/weft_test.go`
- **Creates:** none
- **Deletes:** none
- **Requirements:** Add a new test `TestWeftSpawnPushesWeftBranch` to `weft_test.go` that uses
  the full `lyxtest.CopyPaired(t)` and calls `w.Add(f.Layout, slug, AddOptions{})` (neither
  `SkipPush` nor `SkipGit` set), then asserts the weft branch landed on the weft-bare —
  e.g. run `git ls-remote f.WeftBare` (or `git -C f.WeftBare log --oneline <branch>`) and
  assert the mirrored branch ref exists. This is the only test that exercises the weft-bare
  as a live push target and covers `pushWeftBranch` (weft.go), closing the pre-existing gap.
  Add `t.Parallel()`. Use the same `New(Config{})` / slug pattern as the sibling weft tests.
- **Commit:** `test(worktree): cover weft-branch push on full fixture`

## Batch Tests

`verify: go test -tags integration ./internal/worktree ./internal/lyxtest -count=1` runs the
worktree integration suite (now on the lean fixture for SkipPush tests, plus the new
weft-pushing test) and the `lyxtest` package (whose `TestCopyPaired` still exercises the full
builder, unaffected by `CopyPairedLocal`). Scope is exactly the two packages this batch edits.
