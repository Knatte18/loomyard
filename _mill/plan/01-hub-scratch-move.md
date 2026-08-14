# Batch: hub-scratch-move

```yaml
task: "Move <hub>/.lyx into <hub>/_board"
batch: "hub-scratch-move"
number: 1
cards: 12
verify: go test ./internal/fabricengine/... ./internal/reedengine/... ./internal/reedcli/... ./cmd/lyx/... && go test -tags integration ./internal/fabricengine/... && go test -tags smoke ./internal/reedcli/...
depends-on: []
```

## Batch Scope

This batch relocates the hub-wide never-tracked tree from `<hub>/.lyx` to `<hub>/_board/.lyx` and nothing else — the `_board` convenience junction is untouched here and is deleted whole in batch 2.
It adds `fabricengine.HubScratchDir` as the sole constructor of the new path, relocates `CloneHub`'s eager creation of it from step 4 to after step 7 (`ensureBoardWorktree`) with a new checked `seedWeftArtifactExcludes(boardDir)` call in front of it, re-points `reedengine.HubLogsDir` through `HubScratchDir`, and resolves the import cycle that new production edge would otherwise close through `clone_test.go`.
It also drops the now-unjustified `.lyx` append from `hubSlugReservedNames()` and folds that wrapper away, and updates every prose surface naming the old hub-level path — including `CONSTRAINTS.md`'s Treadle Runner-Seam Invariant, whose transitive-exclusion clause goes false with the new import edge.

**External interface batch 2 consumes:** `fabricengine.HubScratchDir(hub string) string`, and the fact that `internal/fabricengine/clone_test.go` no longer imports `reedengine`.

**Batch-local decision:** the new external-test-package files land in the same directory (`internal/fabricengine/`) as `package fabricengine_test`, not in a new directory.
That package is already the directory's dominant convention (71 external test files against 42 in-package) and shares the single `TestMain` in `testmain_test.go`, so no new `TestMain` is added.

## Cards

### Card 1: Add `fabricengine.HubScratchDir`

- **Context:**
  - `internal/lyxdirs/dirs.go`
  - `internal/lyxcwd/enforcement_test.go`
  - `CONSTRAINTS.md`
- **Edits:**
  - `internal/fabricengine/junctionnames.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Add an exported `HubScratchDir(hub string) string` to `internal/fabricengine/junctionnames.go`, placed immediately after the existing `StencilsDir` function, returning `filepath.Join(BoardDir(hub), lyxdirs.DotLyxDirName)`.
  It must compose from `BoardDir` and `lyxdirs.DotLyxDirName` — never a bare `"_board"` or `".lyx"` string literal — because `TestEnforcement_GeometryLiterals` bans both tokens in path-construction context outside `internal/lyxcwd`/`internal/fabricengine` and `internal/lyxdirs` respectively.
  Its doc comment states it is the hub-wide machine-local scratch tree `<hub>/_board/.lyx`, the ephemeral sibling of `StencilsDir`'s durable `<hub>/_board/_lyx` tree, that it is a real directory and never a junction, and that it is the sole constructor of this path (the told-never-derives rule `StencilsDir`'s own comment already states).
  Do not change `BoardDir`, `StencilsDir`, `BoardWriteLockPath`, or `HubPath`.
- **Commit:** `feat(fabricengine): add HubScratchDir constructor for <hub>/_board/.lyx`

### Card 2: Relocate `CloneHub`'s scratch-dir creation and seed the board's excludes

- **Context:**
  - `internal/fabricengine/junctionnames.go`
  - `internal/fabricengine/weftgit.go`
  - `internal/fabricengine/reconcile.go`
  - `internal/fabricengine/bolt.go`
  - `internal/fabriccli/clone.go`
  - `internal/lyxdirs/dirs.go`
  - `CONSTRAINTS.md`
- **Edits:**
  - `internal/fabricengine/clone.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** In `CloneHub`, delete the step-4 block that creates the hub-level `.lyx` directory — the comment block beginning "`<hub>/.lyx` is a fabric-recognised hub-level geometry element", the `dotLyxPath := filepath.Join(hubPath, lyxdirs.DotLyxDirName)` assignment, its `os.MkdirAll` with the direct `return CloneResult{}, err`, and its `rec.Append(KindDirCreated, dotLyxPath, "")` — leaving `createExclusiveDir`'s `hubTok` block immediately followed by the step-5 warp clone.
  Re-add the creation in step 7, after the existing `rec.Append(KindWorktreeCreated, boardDir, "")` line and before step 8's stale-`.fabric-anchor` check, in this exact order: first a checked `seedWeftArtifactExcludes(boardDir)` call whose failure returns `teardownHub(rec, cwd, hubPath, hubTok, fmt.Errorf("seed weft artifact excludes in board worktree: %w", err))`; then `scratchDir := HubScratchDir(hubPath)` with `os.MkdirAll(scratchDir, 0o755)` whose failure likewise returns through `teardownHub`; then `rec.Append(KindDirCreated, scratchDir, "")`.
  The `rec.Append(KindDirCreated, ...)` is required by the Mutation Record Invariant and must not be dropped in the move.
  Write a replacement comment block above the two calls stating: the hub-wide never-tracked tree is `<hub>/_board/.lyx`, the mirrored ephemeral sibling of `<hub>/_board/_lyx`, created here rather than at step 4 because `_board` does not exist until `ensureBoardWorktree` has run;
  the excludes are seeded first because the exposure is the board's stage-all commit, not untracked dirt — `internal/fabriccli/clone.go` runs `CloneHub` then `NewBolt(res.BoardDir).Commit(...)` and only afterwards `WireJunctionsWith`, which is what seeds `.lyx/` into the weft common gitdir, so without this call anything written into `_board/.lyx` before that commit lands on `weft:main`;
  and the seed failure is fatal rather than best-effort because `Bolt.Commit`'s `commitWeftAt` → `gitrepo.StageAllAndCommit` path seeds nothing, so the board's excludes are not self-healing the way `reconcile.go`'s best-effort call assumes.
  Do not seed excludes on the warp side and do not touch `wireBoardLink` or its call site in this card.
- **Commit:** `refactor(fabricengine): create hub scratch at <hub>/_board/.lyx after the board worktree`

### Card 3: Break the test-binary import cycle in `clone_test.go`

- **Context:**
  - `internal/fabricengine/testmain_test.go`
  - `internal/fabricengine/junctionnames.go`
- **Edits:**
  - `internal/fabricengine/clone_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Remove `TestReedHubLogsDir_MkdirAllIdempotentAgainstFabricCreatedDotLyx` and its doc comment from `internal/fabricengine/clone_test.go` entirely — card 8 re-creates it in an external test package.
  Remove the `github.com/Knatte18/loomyard/internal/reedengine` import from this file's import block, and the `github.com/Knatte18/loomyard/internal/lyxcwd` import if that test was its only user.
  After this card `clone_test.go` must import `reedengine` nowhere;
  that absence is what keeps card 4's production `reedengine → fabricengine` edge legal, since this file is `package fabricengine` and its imports compile into the `fabricengine` test binary.
  This card must land **before** card 4, not after: the moment `reedengine` imports `fabricengine` while `clone_test.go` still imports `reedengine`, `go test ./internal/fabricengine/...` fails to compile with "import cycle not allowed in test", so any intermediate commit in the other order is broken.
  The idempotency test is absent from the tree between this card and card 8, which re-creates it in an external test package;
  that gap is deliberate and closes inside this same batch.
  Update `TestCloneHub_CreatesHubDotLyx`: rename it to `TestCloneHub_CreatesHubScratchDir`, change its assertion target from `filepath.Join(res.HubPath, lyxdirs.DotLyxDirName)` to `HubScratchDir(res.HubPath)`, and rewrite its doc comment — the directory is now the hub-wide ephemeral sibling of `<hub>/_board/_lyx`, created after the board worktree exists, still a real directory rather than a junction.
  Update the file's own header comment, which says the file covers "CloneHub's hub-level `<hub>/.lyx` materialisation".
- **Commit:** `test(fabricengine): drop reedengine import from in-package clone_test`

### Card 4: Re-point `reedengine.HubLogsDir` at the board-anchored path

- **Context:**
  - `internal/fabricengine/junctionnames.go`
  - `internal/lyxcwd/lyxcwd.go`
  - `internal/lyxdirs/dirs.go`
  - `CONSTRAINTS.md`
- **Edits:**
  - `internal/reedengine/lifecycle.go`
  - `internal/reedengine/serverlog.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Card 3 must already have landed before this card starts — this is the card that introduces the `reedengine → fabricengine` production edge, and it does not compile against a `clone_test.go` that still imports `reedengine`.
  Change `HubLogsDir` in `internal/reedengine/lifecycle.go` to return `filepath.Join(fabricengine.HubScratchDir(l.HubPath), "logs")`, adding the `github.com/Knatte18/loomyard/internal/fabricengine` import.
  Drop the now-unused `lyxdirs` import only if no other reference to it survives in that file — `stateDir()` still uses `lyxdirs.DotLyxDirName` and must not change, so the import stays.
  Rewrite `HubLogsDir`'s doc comment: it is still hub-anchored (one server per hub, one deterministic place) but now lives under the hub-wide `<hub>/_board/.lyx` scratch tree obtained from `fabricengine.HubScratchDir`, never derived here.
  Update the two inline comments in this file reading "its own cwd is the hub's `.lyx/logs` dir" — one in the `Down` teardown branch, one in the pane-reap comment below it — to name `<hub>/_board/.lyx/logs`.
  Update `internal/reedengine/serverlog.go`'s file header, which describes pruning "the per-hub server's log files under the hub's `.lyx/logs/`", to the new path.
  Do not change `stateDir()`.
  Do not change the `os.MkdirAll(logsDir, 0o755)` call in the boot path — it stays and is what the idempotency test in card 8 pins.
- **Commit:** `refactor(reedengine): anchor HubLogsDir on fabricengine.HubScratchDir`

### Card 5: Update reed's user-visible help and smoke-test prose

- **Context:**
  - `internal/reedengine/lifecycle.go`
  - `CONSTRAINTS.md`
- **Edits:**
  - `internal/reedcli/up.go`
  - `internal/reedcli/smoke_debuglog_test.go`
  - `internal/reedcli/smoke_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** In `internal/reedcli/up.go`, change the cobra `Long` help sentence "enables server verbose logging to `<hub>/.lyx/logs/`" to name `<hub>/_board/.lyx/logs/`.
  This is user-visible output, so the CLI / Cobra Invariant's help-accuracy obligation applies.
  In `internal/reedcli/smoke_debuglog_test.go`, update the file header describing the log destination as "the hub's `.lyx/logs/` dir" to the new path.
  Change no assertion in the smoke test — the smoke tests passing unchanged is itself the assertion that the move is transparent to reed.
  In `internal/reedcli/smoke_test.go`, fix `materializeSibling`'s pre-existing (main-branch) bug: it clones the second worktree into `filepath.Join(h.Container, name)`, a sibling of the hub directory itself rather than a worktree inside it, so `lyxcwd.Resolve` computes a different `HubPath` for it than for the prime worktree and `TestSmokeDownInOneWorktreeLeavesSiblingSessionAlive` (`smoke_teardown_test.go`) fails on socket-name mismatch — reproduced identically on `main`, unrelated to this batch's `.lyx`-relocation edits, but this batch's own `verify:` line runs `-tags smoke ./internal/reedcli/...` so it must be green before this batch reports done. Change the clone target to `filepath.Join(h.Path, name)` so the sibling worktree is a direct child of the hub directory, matching `h.PrimeWorktree()`'s own parentage.
- **Commit:** `docs(reedcli): name <hub>/_board/.lyx/logs in up help and smoke header`

### Card 6: Fold away `hubSlugReservedNames()`

- **Context:**
  - `internal/fabricengine/structuraldirs_test.go`
  - `internal/lyxdirs/dirs.go`
- **Edits:**
  - `internal/fabricengine/junctionnames.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Delete `hubSlugReservedNames()` from `internal/fabricengine/junctionnames.go` together with its doc comment, whose justification — a worktree named `.lyx` would collide with the hub-level `<hub>/.lyx` — no longer holds once the tree moves inside `_board`.
  Re-point its two callers at `HubReservedNames()` directly: the range head in `IsReservedHubName`, and the final argument of `dedupUnion` in `slugReservedNames`.
  Rewrite `IsReservedHubName`'s own doc comment to drop the "`hubSlugReservedNames()` (HubReservedNames() plus `.lyx`)" phrasing and state that `.lyx` is refused via `structuralNeverCommittedDirs`.
  Rewrite `slugReservedNames`'s doc comment to name `HubReservedNames()` instead of `hubSlugReservedNames()`.
  Keep `slugReservedNames(cfg)` itself even though its only caller is `structuraldirs_test.go` — it is the named expression of the slug-reservation set that test exists to pin.
  This must be behaviour-preserving: `.lyx` stays refused as a slug because `structuralNeverCommittedDirs` is `[]string{lyxdirs.DotLyxDirName}` and `IsReservedHubName` already checks that set.
- **Commit:** `refactor(fabricengine): fold hubSlugReservedNames into HubReservedNames`

### Card 7: Prove the slug reservation survives the fold

- **Context:**
  - `internal/fabricengine/junctionnames.go`
  - `internal/lyxdirs/dirs.go`
- **Edits:**
  - `internal/fabricengine/structuraldirs_test.go`
  - `internal/fabricengine/junctionnames_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** In `internal/fabricengine/structuraldirs_test.go`, add a test asserting `slugReservedNames(Config{Pathspec: "_extra"})` contains `lyxdirs.DotLyxDirName` exactly once, sourced from `structuralNeverCommittedDirs`.
  Write this assertion before card 6's fold is applied if implementing TDD-first, so the removal is proven behaviour-preserving rather than assumed;
  it must pass identically on both sides of the fold.
  Leave the existing `TestDeployedLyxPathspec_YieldsNoDuplicateLyx` and `TestHubReservedNames_StillReturnsExactlyTheThreeHubStructuralTokens` cases unchanged.
  In `internal/fabricengine/junctionnames_test.go`, update the comment block above the "default pathspec union reserves exactly four names" subtest: the two sentences attributing `_board`/`_portals`/`_launchers` and `.lyx` to `hubSlugReservedNames()` must name `HubReservedNames()` and `structuralNeverCommittedDirs` respectively.
  The assertions in that subtest are unchanged.
- **Commit:** `test(fabricengine): pin .lyx slug reservation on structuralNeverCommittedDirs`

### Card 8: New external-package coverage for `HubScratchDir` and reed idempotency

- **Context:**
  - `internal/fabricengine/junctionnames.go`
  - `internal/fabricengine/clone_test.go`
  - `internal/fabricengine/testmain_test.go`
  - `internal/reedengine/lifecycle.go`
  - `internal/lyxcwd/lyxcwd.go`
  - `internal/lyxdirs/dirs.go`
  - `CONSTRAINTS.md`
- **Edits:** none
- **Creates:**
  - `internal/fabricengine/hubscratch_test.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** Create `internal/fabricengine/hubscratch_test.go` declaring `package fabricengine_test`, untagged (it spawns nothing — pure `filepath.Join` arithmetic plus `os.MkdirAll` under `t.TempDir()`), importing `fabricengine`, `reedengine`, `lyxcwd`, and `lyxdirs`.
  Its header comment states why it is an external test package: it imports both `fabricengine` and `reedengine`, which an in-package file cannot do without closing the `fabricengine`[test] → `reedengine` → `fabricengine` cycle.
  It adds no `TestMain` — the package shares the single one in `testmain_test.go`.
  Add `TestHubScratchDir_IsBoardAnchored` asserting `fabricengine.HubScratchDir(hub)` equals `filepath.Join(fabricengine.BoardDir(hub), lyxdirs.DotLyxDirName)` for a synthetic hub path, and that it is a sibling of `fabricengine.StencilsDir(hub)`'s `_lyx` component rather than nested under it.
  Add `TestHubScratchDir_IgnoresAnchorRel` asserting the value is byte-identical for a subpath-anchored hub — `HubScratchDir` takes a bare hub string and must never pick up `AnchorRel`, because the board's `_lyx`/`.lyx` trees are flat.
  Re-create `TestReedHubLogsDir_MkdirAllIdempotentAgainstFabricCreatedDotLyx` here, moved verbatim from `clone_test.go` except that its pre-created directory is `fabricengine.HubScratchDir(hubPath)` instead of `filepath.Join(hubPath, lyxdirs.DotLyxDirName)`, and its doc comment names the new path and states that reed's own boot-path `os.MkdirAll(HubLogsDir(...))` remains idempotent against fabric's eager creation.
- **Commit:** `test(fabricengine): cover HubScratchDir and reed log-dir idempotency`

### Card 9: Clone-ordering coverage that fails without the new seed call

- **Context:**
  - `internal/fabricengine/clone.go`
  - `internal/fabricengine/junctionnames.go`
  - `internal/fabricengine/weftgit.go`
  - `internal/fabricengine/gitexclude.go`
  - `internal/fabricengine/junction_pattern_integration_test.go`
  - `internal/fabricengine/clone_adopt_test.go`
  - `internal/fabricengine/testmain_test.go`
  - `internal/fabricengine/bolt.go`
  - `internal/lyxdirs/dirs.go`
- **Edits:** none
- **Creates:**
  - `internal/fabricengine/hubscratch_integration_test.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** Create `internal/fabricengine/hubscratch_integration_test.go` with a `//go:build integration` constraint on its first line and `package fabricengine_test`, reusing `makeBareRemote` (`clone_adopt_test.go`) and `readExcludeLines` (`junction_pattern_integration_test.go`) rather than re-declaring either.
  Add `TestCloneHub_SeedsBoardArtifactExcludesBeforeReturning`: call `fabricengine.CloneHub` directly — never `fabriccli.CloneAndWire`, whose later `WireJunctionsWith` seeding would mask the omission — and assert that at the instant `CloneHub` returns, the weft common gitdir's `info/exclude` reached from `res.BoardDir` already carries the `.lyx/` entry.
  Resolve that file through `git rev-parse --git-path info/exclude` run in `res.BoardDir`, so the assertion follows the same resolution `mutateGitExclude` uses for a linked worktree rather than assuming a path.
  This is the load-bearing assertion for card 2's ordering: a `git status`-is-clean assertion does not qualify, because it passes either way while the directory is empty.
  Add `TestCloneHub_BoardStageAllCommitNeverStagesHubScratch`: after `CloneHub` returns, assert `fabricengine.HubScratchDir(res.HubPath)` exists and is a directory, plant a file inside it, then run `fabricengine.NewBolt(res.BoardDir).Commit(...)` and assert the planted file is absent from the resulting commit's tree.
  That is the failure the ordering exists to prevent, stated directly.
  Register a `t.Cleanup` removing `res.HubPath` in both cases, matching `clone_test.go`'s existing convention.
- **Commit:** `test(fabricengine): pin board exclude seeding order at CloneHub's boundary`

### Card 10: Update `cmd/lyx` anchoring and containment-allowlist prose

- **Context:**
  - `internal/reedengine/lifecycle.go`
  - `internal/fabricengine/junctionnames.go`
  - `internal/fabricengine/clone.go`
  - `internal/fabricengine/destroy.go`
  - `CONSTRAINTS.md`
- **Edits:**
  - `cmd/lyx/constructoranchoring_test.go`
  - `cmd/lyx/uncontainedwrite_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** In `cmd/lyx/constructoranchoring_test.go`, change both `reedengine.HubLogsDir` assertions — the one in `TestConstructorAnchoring_UnanchoredRoot` and the one in `TestConstructorAnchoring_SubpathAnchored` — from `filepath.Join(hub, ".lyx", "logs")` to `filepath.Join(hub, "_board", ".lyx", "logs")`.
  Test files are excluded from `TestEnforcement_GeometryLiterals`, so the literals are legal here and match the file's existing style.
  Update the file's header comment sentence naming `reedengine.HubLogsDir` as "HubPath-anchored, one server per hub" so it says the constructor is hub-anchored *through the board*, and update the two inline comments above those assertions the same way.
  The property under test is unchanged: `HubLogsDir` alone ignores `AnchorRel` and stays byte-identical between the two fixtures.
  In `cmd/lyx/uncontainedwrite_test.go`, rewrite the `internal/fabricengine/clone.go` allowlist reason string: after the relocation the scratch directory is written inside the `_board` worktree that `containedWorktreeAdd` just added, not into the bare hub `createExclusiveDir` minted, so the first clause no longer describes the code.
  The guard passes either way, which is exactly why this string must be corrected by hand.
  Leave every other allowlist entry untouched.
- **Commit:** `test(cmd/lyx): re-anchor HubLogsDir assertions on <hub>/_board/.lyx`

### Card 11: Restate the remaining production and test prose naming `<hub>/.lyx`

- **Context:**
  - `internal/fabricengine/junctionnames.go`
  - `internal/fabricengine/clone.go`
  - `internal/lyxdirs/dirs.go`
- **Edits:**
  - `internal/fabricengine/slug.go`
  - `internal/fabricengine/add_test.go`
  - `internal/fabricengine/destructivegaps_integration_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** In `internal/fabricengine/slug.go`, rewrite the file-header sentence listing "`<Hub>/_board`, `<Hub>/.lyx`, and the weft siblings" as real directories a teardown verb would walk into: `<Hub>/.lyx` is no longer hub geometry, so restate the `.lyx` half on its structural-reservation footing (a worktree slug of `.lyx` is refused via `structuralNeverCommittedDirs`, not because a hub-level directory of that name exists).
  Leave `validateWorktreeSlug`'s behaviour unchanged.
  In `internal/fabricengine/add_test.go`, update `TestAdd_RejectsDotLyxSlug`'s doc comment, which justifies the refusal by collision with "the hub-level `<hub>/.lyx` batch 8 recognises".
  The assertion itself stays valid and unchanged — only its stated reason changes.
  In `internal/fabricengine/destructivegaps_integration_test.go`, update `TestCloneHub_TeardownSucceedsOnAHalfBuiltHub`'s doc comment, which describes the early-clone failure state as "hub created, only `.lyx` materialised, no warp clone and no weft clone yet" — after card 2 nothing but the hub directory itself exists at that point.
  Check whether the surrounding case's setup depends on that state or only the comment does, and change only what is actually false.
- **Commit:** `docs(fabricengine): restate .lyx slug reservation off hub geometry`

### Card 12: Repo-level documentation for the relocated scratch tree

- **Context:**
  - `internal/fabricengine/junctionnames.go`
  - `internal/fabricengine/clone.go`
  - `internal/reedengine/lifecycle.go`
  - `internal/treadleengine/runner.go`
  - `internal/shuttleengine/reed.go`
  - `internal/treadleengine/seam_enforcement_test.go`
- **Edits:**
  - `README.md`
  - `docs/overview.md`
  - `manifest/designs/fabric-unified-view.md`
  - `CONSTRAINTS.md`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** In `README.md`, remove the `.lyx/` entry from the hub-tree diagram's top level and show it nested under `_board/` instead;
  update the following sentence describing `.lyx/` as ephemeral and machine-bound so it does not imply a hub-level directory.
  In `docs/overview.md`, make the same change to its hub-tree diagram, and remove `.lyx` from the sentence "`_board`, `_portals`, `_launchers`, and `.lyx` are hub geometry" — `.lyx` stays slug-reserved but is no longer hub geometry.
  Leave the `_board`, `_portals` and `_launchers` diagram lines and the `_board` junction paragraphs alone;
  batch 2 owns those.
  In `manifest/designs/fabric-unified-view.md`, update the shipped-correction sentence "`HubLogsDir` alone joins onto `Location.HubPath`, deliberately hub-anchored" to say it joins onto `fabricengine.HubScratchDir(Location.HubPath)`, and update the slice-9 bullet "`<hub>/.lyx` shipped as a new hub-level geometry element alongside `<hub>/_board`" with a note that it was subsequently moved inside `_board`.
  In `CONSTRAINTS.md`, reword the final clause of the Treadle Runner-Seam Invariant's `internal/stencilstore` bullet — "…runs once at `cmd/lyx`'s root pre-run rather than lazily inside `stencilstore.Read`, which is what keeps `internal/fabricengine` off treadle's stack".
  That clause is false once `reedengine → fabricengine` lands, because `treadleengine → shuttleengine → reedengine → fabricengine` is now a real transitive path.
  State instead what the pre-run seeding actually buys — treadle is told its stencils directory and derives none of its own — with no claim of a transitive exclusion.
  Do not touch that invariant's import allowlist or its "policed on direct imports only" sentence;
  both remain true and `seam_enforcement_test.go` keeps passing unchanged.
  Do not amend the Durable-vs-Ephemeral State Invariant or the Hub Containment Invariant — both were already written in the discussion commit and are the spec.
- **Commit:** `docs: relocate hub scratch to <hub>/_board/.lyx across README, overview, designs, CONSTRAINTS`

## Batch Tests

`verify:` runs the three untagged packages this batch changes (`internal/fabricengine`, `internal/reedengine`, `internal/reedcli`) plus `cmd/lyx`, then the integration tier for `internal/fabricengine`, then the smoke tier for `internal/reedcli`.

- `cmd/lyx` is in scope for two reasons beyond card 10's own edits: it hosts the repo-wide guards this batch can trip — `tierpurity_test.go` (card 8's new untagged file), `hermeticenv_test.go`, and the Markdown Link Integrity check over card 12's doc edits.
- `-tags integration ./internal/fabricengine/...` covers card 9's new file and card 11's edit to `destructivegaps_integration_test.go`, both of which carry the `integration` constraint and are invisible to an untagged run.
- `-tags smoke ./internal/reedcli/...` covers card 4's edit to `smoke_debuglog_test.go`;
  the reed smoke tests passing unchanged is the end-to-end evidence that re-pointing `HubLogsDir` is transparent to the server.
- `internal/lyxcwd` is deliberately absent: `TestEnforcement_GeometryLiterals` walks the whole source tree from within that package, but nothing in this batch adds a geometry literal to a production file outside the owner directories, and card 1's composition through `BoardDir`/`lyxdirs.DotLyxDirName` is exactly what keeps it green.
  The repo-wide `done_gate` covers it at task end.
