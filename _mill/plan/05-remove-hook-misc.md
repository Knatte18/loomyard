# Batch: remove-hook-misc

```yaml
task: "Speed up internal/warp integration tests"
batch: "remove-hook-misc"
number: 5
cards: 8
verify: go test -tags integration -run TestRemove ./internal/warp/ && go test -tags integration -run TestInstallPostCheckoutHook ./internal/warp/ && go test -tags integration -run TestList ./internal/warp/ && go test -tags integration -run TestRunDispatchesToWarp ./internal/warp/ && go test -tags integration -run TestWriteLaunchers ./internal/warp/ && go test -tags integration -run TestCreatePortal ./internal/warp/
depends-on: [1]
```

## Batch Scope

The remaining consolidations and deletions across six small, independent test files:
remove (group I + one delete), hook (groups J, K), list (group L), the CLI router (group M),
launchers (one delete), and portals (one fold). Each edit is self-contained per file;
they are batched together because each is a small change and they share no code. Net: 8
fewer fixture builds, no coverage lost. Batch-local: sequential dirty-then-force calls
(group I) and shared read-only subtests (group M) run without `t.Parallel` between steps.

## Cards

### Card 14: Group I — merge Remove dirty-force pairs into two sequential subtests

- **Context:** none
- **Edits:**
  - `internal/warp/remove_test.go`
- **Creates:** none
- **Deletes:** none
- **Requirements:** In `TestRemove`, `HostDirtyWithoutForce` + `HostDirtyWithForce` share
  setup (Add + dirty a host file); the without-force `Remove` fails leaving dirs intact, so
  the force call can run on the same fixture. Likewise `WeftDirtyWithoutForce` +
  `WeftDirtyWithForce`. Because the existing `TestRemove` table body issues exactly one
  `Remove` call per row, these merged cases cannot stay as table rows — implement each as a
  **standalone `t.Run` sequential subtest beside the table**: `HostDirty` (Add + dirty host
  → `Remove(force=false)` errors, dirs intact → `Remove(force=true)` succeeds, dirs gone)
  and `WeftDirty` (same shape for the weft), each on one shared fixture with no `t.Parallel`
  between the two calls. Leave `HappyPath`/`NonexistentSlug` as table rows and the junction
  subtests untouched in this card.
- **Commit:** `test(warp): merge Remove dirty-without/with-force into sequential subtests`

### Card 15: Delete low-value TestRemoveHostJunctionRemoved

- **Context:**
  - `internal/warp/remove_test.go`
- **Edits:**
  - `internal/warp/remove_test.go`
- **Creates:** none
- **Deletes:** none
- **Requirements:** Delete `TestRemoveHostJunctionRemoved` — the flat-topology junction-removal
  case. The load-bearing nested case is `TestRemoveSubpathJunction` (RelPath != "."), which
  exercises the recursive removal path and must be kept.
- **Commit:** `test(warp): drop redundant TestRemoveHostJunctionRemoved (flat topology)`

### Card 16: Group J — delete hook WritesScript (covered by Idempotent)

- **Context:** none
- **Edits:**
  - `internal/warp/hook_test.go`
- **Creates:** none
- **Deletes:** none
- **Requirements:** Delete `TestInstallPostCheckoutHook_WritesScript`. Its assertions (hook
  file written, sentinel present) are fully covered by the first-install phase of
  `TestInstallPostCheckoutHook_Idempotent`. No assertion needs porting.
- **Commit:** `test(warp): delete TestInstallPostCheckoutHook_WritesScript (subsumed by Idempotent)`

### Card 17: Group K — delete hook ChainsExistingHook, port unique assertion to ChainIdempotent

- **Context:** none
- **Edits:**
  - `internal/warp/hook_test.go`
- **Creates:** none
- **Deletes:** none
- **Requirements:** The first-install phase of `TestInstallPostCheckoutHook_ChainIdempotent`
  already establishes and validates the chain (user hook backed up, chained wrapper has the
  sentinel). Port the one assertion unique to `TestInstallPostCheckoutHook_ChainsExistingHook`
  — that the chained wrapper references `post-checkout.user` — into `_ChainIdempotent` after
  its first install, then delete `TestInstallPostCheckoutHook_ChainsExistingHook`.
- **Commit:** `test(warp): merge hook ChainsExistingHook assertion into ChainIdempotent`

### Card 18: Group L — fold List SingleWorktree into TwoWorktrees pre-add check

- **Context:** none
- **Edits:**
  - `internal/warp/list_test.go`
- **Creates:** none
- **Deletes:** none
- **Requirements:** `TestList/SingleWorktree` is read-only (asserts List returns 1 entry,
  Main=true, branch=main, non-empty Head) on the same fresh `CopyHostHub` that
  `TestList/TwoWorktrees` starts from before adding a second worktree. Port the
  single-worktree assertions to `TestList/TwoWorktrees` as a pre-add check (before
  `git worktree add`), then delete the `SingleWorktree` subtest. The shared pre-add and
  post-add assertions run sequentially.
- **Commit:** `test(warp): fold List SingleWorktree into TwoWorktrees pre-add check`

### Card 19: Group M — share one setupCLIRepo across read-only router subtests

- **Context:**
  - `internal/warp/warp_test.go`
- **Edits:**
  - `internal/warp/warp_test.go`
- **Creates:** none
- **Deletes:** none
- **Requirements:** `TestRunDispatchesToWarp/List` and `/UnknownSubcommand` are both
  read-only (neither mutates the filesystem) and already serial (RunCLI reads
  `os.Getwd()`). Restructure so a single `setupCLIRepo` (one `CopyHostHub`) is created once
  and reused by both read-only subtests instead of one build each. The mutating
  `/RemoveWithForceFlag` subtest must keep its own `setupCLIRepo` call. Do not add
  `t.Parallel` to the shared read-only subtests.
- **Commit:** `test(warp): share one setupCLIRepo across read-only router subtests`

### Card 20: Delete duplicate TestWriteLaunchers/DotRelPath

- **Context:** none
- **Edits:**
  - `internal/warp/launchers_test.go`
- **Creates:** none
- **Deletes:** none
- **Requirements:** Delete the `DotRelPath` subtest of `TestWriteLaunchers`. It is an exact
  duplicate of `EmptyRelPath` — both resolve `RelPath="."` from `f.Hub` and exercise the
  identical launcher climb; only the slug and a literal string differ. Keep `EmptyRelPath`
  and `NonEmptyRelPath`.
- **Commit:** `test(warp): drop duplicate TestWriteLaunchers/DotRelPath subtest`

### Card 21: Fold TestCreatePortalMultipleSubpaths assertion into TestCreatePortal

- **Context:** none
- **Edits:**
  - `internal/warp/portals_test.go`
- **Creates:** none
- **Deletes:** none
- **Requirements:** `TestCreatePortalMultipleSubpaths` uniquely asserts that two distinct
  subpaths for one slug yield non-colliding portal links (`link1 != link2`). Rather than
  rely on "structurally guaranteed", port that two-subpath `createPortal` + `link1 != link2`
  assertion as an added sequential step in `TestCreatePortal` (on its existing fixture),
  then delete the standalone `TestCreatePortalMultipleSubpaths`. Keep `TestCreatePortal` and
  `TestCreatePortalRootRelPath`. This removes the separate fixture build while keeping the
  collision-avoidance assertion.
- **Commit:** `test(warp): fold MultipleSubpaths collision assertion into TestCreatePortal`

## Batch Tests

`verify` runs six pipe-free `-run` prefixes — `TestRemove`, `TestInstallPostCheckoutHook`,
`TestList`, `TestRunDispatchesToWarp`, `TestWriteLaunchers`, `TestCreatePortal` — chained
with `&&`, covering every function across the six edited files. Confirms the merged
sequential subtests (group I), the ported chain assertion (K), the shared-fixture router
subtests (M), and the fold/deletes left all six files compiling and green.
