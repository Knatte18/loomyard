# Plan: fabric: live-state integration harness (slice 13)

```yaml
task: 'fabric: live-state integration harness (slice 13)'
slug: 'fabric-live-state-harness'
approved: true
started: '20260811-102636'
parent: 'main'
root: ""
verify: go build ./... && go vet ./...
```

## Batch Index

_The fenced yaml block below is the authoritative DAG mill-go reads to schedule batches.
Every batch lives at `NN-<batch-slug>.md` in this directory and is mirrored as one entry here._

```yaml
batches:
  - number: 1
    name: enforcement-and-extraction
    file: 01-enforcement-and-extraction.md
    depends-on: []
    verify: go build ./... && go test ./internal/lyxcwd/ -run TestEnforcement && go test ./cmd/lyx/ -run TestNoDestructiveBypass_FabricengineProductionSource && go test -tags integration ./internal/fabriccli/
  - number: 2
    name: package-skeleton-and-hub-factory
    file: 02-package-skeleton-and-hub-factory.md
    depends-on: [1]
    verify: go build ./... && go test -tags integration ./internal/fabricengine/fabrictest/ && go test -tags integration ./internal/fabricengine/ -run TestCommitWeft_LockArtifactsExcludedFromStatus && go test ./cmd/lyx/ -run 'TestTierPurity_UntaggedTestsSpawnNothing|TestHermeticGitEnv_GitSpawningPackagesHaveTestMain' && go test ./internal/lyxcwd/ -run TestEnforcement
  - number: 3
    name: manifest-capture-and-diff
    file: 03-manifest-capture-and-diff.md
    depends-on: [2]
    verify: go build ./... && go test -tags integration ./internal/fabricengine/fabrictest/ && go test ./cmd/lyx/ -run 'TestTierPurity_UntaggedTestsSpawnNothing|TestNoDestructiveBypass_FabricengineProductionSource' && go test ./internal/lyxcwd/ -run TestEnforcement
  - number: 4
    name: refusal-expectation-helpers
    file: 04-refusal-expectation-helpers.md
    depends-on: [2]
    verify: go build ./... && go test -tags integration ./internal/fabricengine/fabrictest/ && go test ./internal/lyxcwd/ -run TestEnforcement
  - number: 5
    name: hostile-state-matrix
    file: 05-hostile-state-matrix.md
    depends-on: [3]
    verify: go build ./... && go test -tags integration ./internal/fabricengine/fabrictest/ && go test ./cmd/lyx/ -run TestNoDestructiveBypass_FabricengineProductionSource && go test ./internal/lyxcwd/ -run TestEnforcement
  - number: 6
    name: verb-table-and-expectations
    file: 06-verb-table-and-expectations.md
    depends-on: [4, 5]
    verify: go build ./... && go test -tags integration ./internal/fabricengine/fabrictest/ && go test ./internal/lyxcwd/ -run TestEnforcement
  - number: 7
    name: cross-product-driver
    file: 07-cross-product-driver.md
    depends-on: [6]
    verify: go build ./... && go test -tags integration ./internal/fabricengine/fabrictest/
  - number: 8
    name: sabotage-proof-and-docs
    file: 08-sabotage-proof-and-docs.md
    depends-on: [7]
    verify: go build ./... && go test -tags integration ./internal/fabricengine/fabrictest/ && go test ./internal/lyxcwd/ -run 'TestEnforcement|TestEnforcement_MarkdownLinks'
```

## Shared Decisions

_Cross-cutting decisions every batch inherits: naming conventions, error-handling posture, test frameworks, style/lint constraints.
One subsection per decision.
Batch-local decisions live in each batch file._

### Decision: enforcement-sweep-result

- **Decision:** The discussion requires a sweep of every enforcement test in the tree for directory-scoped ownership before cards are written.
  That sweep has been performed and its result is authoritative for this plan.
  Exactly **four** machine guards reach `internal/fabricengine/fabrictest` and need attention, and no fifth exists:
  1. `TestEnforcement_FabricVocabulary` (`internal/lyxcwd/enforcement_test.go`) — two exact-match directory maps, `fabricVocabularyOwners` and `weftnameImportOwners`;
     both need an `internal/fabricengine/fabrictest` row (batch 1, card 2).
  2. `TestNoDestructiveBypass_FabricengineProductionSource` (`cmd/lyx/destructiveguard_test.go`) — `filepath.WalkDir` over `internal/fabricengine`, skipping only `*_test.go`;
     needs a `fabrictest` subdirectory exclusion (batch 1, card 3).
  3. `TestEnforcement_GeometryLiterals` (`internal/lyxcwd/enforcement_test.go`) — `geometryTokenOwners`, keyed on exact directory;
     handled by routing every geometry path through the already-exported accessors and adding **no** owner row.
  4. `TestTierPurity_UntaggedTestsSpawnNothing` and `TestHermeticGitEnv_GitSpawningPackagesHaveTestMain` (`cmd/lyx/`) — satisfied structurally by the build-tag layout plus `testmain_test.go`, with no allowlist entry.
- **Ruled out by inspection, with the reason each does not apply:** `cmd/lyx/notransients_test.go` and `cmd/lyx/constructoranchoring_test.go` enumerate named constructors, not directories;
  `internal/lyxcwd/raddle_guard_test.go` walks `internal/lyxcwd` only;
  `cmd/lyx/rawgitmutation_test.go` scopes to `internal/websterengine` and `cmd/lyx/boardguard_test.go` to `internal/boardengine`;
  `cmd/lyx/ghguard_test.go`, `cmd/lyx/gitrepoboundary_test.go`, `internal/gitrepo/noforceadd_test.go` and every `*_enforcement_test.go` leaf guard scope to packages this task does not add files to;
  `cmd/lyx/tiersleep_test.go` flags long literal sleeps in untagged test files, and this task adds none;
  `internal/lyxcwd/docslink_test.go` scans `manifest/` and `docs/` markdown only, and applies to batch 8's doc edits rather than to the package.
- **Applies to:** all batches

### Decision: build-tag-layout

- **Decision:** In `internal/fabricengine/fabrictest`, `doc.go` and `testmain_test.go` carry **no** build tag;
  every other file in the package carries `//go:build integration` as its first line.
  `doc.go` untagged is what keeps `go build ./...` from failing with "build constraints exclude all Go files";
  everything else tagged is what stops an untagged test file importing the factory and smuggling git spawns past the substring-based Test Tier Purity guard.
- **Applies to:** all batches

### Decision: geometry-through-accessors

- **Decision:** No file in `fabrictest` may spell `_board`, `-weft`, `-HUB`, `_portals`, `_launchers`, `_lyx` or `.lyx` as a string literal in a `filepath.Join` argument, a binary `+` operand, or a string const value.
  Every geometry path is built from the exported accessors: `fabricengine.PortalsDir`, `fabricengine.PortalLink`, `fabricengine.LauncherDir`, `fabricengine.BoardDir`, `fabricengine.BoardDirName`, `fabricengine.HubPath`, `fabricengine.HubSuffix`, `fabricengine.WorktreePath`, `fabricengine.WeftWorktree`, `fabricengine.WeftWorktreePath`, `fabricengine.WiredNames`, `fabricengine.DeriveWarpName`, `fabricengine.WeftBranchName`, `weftname.Suffix`, `weftname.SiblingPath`, `weftname.BareSiblingPath`, `lyxdirs.LyxDirName`, `lyxdirs.DotLyxDirName`.
- **Rationale:** `TestEnforcement_GeometryLiterals` is an AST check keyed on the exact directory string, and `internal/fabricengine/fabrictest` has no owner row and deliberately never gets one — an owner row would grant standing permission to spell geometry tokens raw, while accessors make the harness break loudly if fabric ever relocates a directory.
- **Applies to:** all batches

### Decision: windows-portable-by-construction

- **Decision:** No `runtime.GOOS == "windows"` skip appears anywhere in this package's states, cells, or helpers.
  The harness is written portable instead, by these rules:
  all link creation and inspection goes through `internal/fslink` (`CreateDirLink`, `IsLink`, `PointsTo`, `RawTarget`, `Remove`, `RemoveLinksIn`) and never `os.Symlink`/`os.Readlink`/`os.Lstat` link inspection;
  launcher paths are matched by directory root or by the same GOOS branch the production code uses, never by a hardcoded `.sh`/`.cmd` extension;
  manifest keys are `filepath.ToSlash`-normalised hub-relative paths;
  manifest path comparison folds case (`strings.EqualFold`) — `lyxcwd.samePath` does exactly this but is unexported, so `fabrictest` carries its own equivalent;
  bare-remote URLs handed to git go through `filepath.ToSlash`;
  no state and no assertion depends on Unix permission bits, and no state calls `chmod`;
  template and slug names stay short, because `<hub>/_launchers/<anchor>/<slug>/<script>` nests deep against Windows' 260-character default limit.
- **Applies to:** all batches

### Decision: manifest-not-snapshot

- **Decision:** The harness operation is called **manifest** throughout — `Manifest`, `CaptureManifest`, `DiffManifest`, `manifest.go`.
  The word "snapshot" never names it.
- **Rationale:** `internal/fabricengine/snapshot.go` already owns Snapshot in fabric's vocabulary (the `Snapshot: <tag>` trailer recording a warp SHA on the weft branch), and reusing the word for a filesystem capture would collide with it inside the very package the harness drives.
- **Applies to:** all batches

### Decision: five-phase-cell-order

- **Decision:** Every cell runs five phases in this fixed order, and no batch may reorder them:
  (1) **Build** — the factory clones a fresh hub at the cell's anchor;
  (2) **Arrange** — `VerbCase.Arrange` establishes the verb's own fixture;
  (3) **State** — `State.Apply` plants the hostile or dirty condition and asserts it took;
  (4) **Capture (before)** — the manifest is taken here, after both arrange and state, so each is baseline rather than diff noise;
  (5) **Run**, then **Capture (after)**, then diff and assert.
- **Rationale:** state must follow arrange because several states plant into what arrange created (a dirty pair worktree does not exist until `Add` has made one), and the before-manifest must follow both or every arrangement mutation shows up as an unpermitted change.
- **Applies to:** all batches

### Decision: cell-enumeration-and-omissions

- **Decision:** The cell set is generated by the cross product, restricted by `VerbCase.States` and by a recorded omission set.
  This plan owns the enumeration;
  the counts below are the plan's, not a ceiling inherited from the discussion.
  - **Ordinary product:** 8 ordinary verbs (`Add`, `Remove`, `Prune`, `Cleanup`, `Checkout`, `Reconcile`, `UnwireJunctions`, `Pull`) × 10 states × 2 anchors = **160**.
  - **Structural-state omissions, derived from each verb's actual reach into `destroy.go`'s path executors** (verified by reading the `pathRequest`/`removeLink`/`repointLink`/`removePath`/`removeGitWorktree` call sites in each verb's file):
    `Cleanup` −4 (branch-shaped;
    its only gate call is `deleteBranch` at `cleanup.go:275`, so no structural state can name a path it acts on);
    `Checkout` −4 (same reason;
    its only gate call is `deleteBranch` at `checkout.go:195-203`);
    `Pull` −4 (its only gate call is `Fabric.ResetHard` at `destroy.go:762`, a warp checkout reset, not a path executor);
    `Add` −2 for `trackedSymlinkAtWiredPath` and `staleWiredJunction` (its gate calls at `add.go:263-295` act on the pair it is creating, whose junction paths do not exist before `Run`);
    `UnwireJunctions` −1 for `unrelatedGitCloneAtWeftNamedPath` (its only gate call is the `removeLink` inside `unseedJunctionRecords` at `junction.go:474-483`, reached from `UnwireJunctions` at `junction.go:368` via `unseedLyxJunction`, and it never visits a weft-named path — note that `unwire.go:143-152` is a *different* function, `unwireBoardLink`, which `UnwireJunctions` never calls).
    That is **15 per anchor, 30 in total**, leaving **130**.
  - **Hostile-input cases:** 17 (`Add` 7 + `Remove` 7 + `Checkout` 2 + `UnwireJunctions` 1) × the `clean` state only × 2 anchors = **34**.
  - **`CloneHub{Reset: true}` column:** 2 targets × 2 anchors = **4**.
  - **Total: 168 cells.**
- **Dirtiness-state omissions are resolved per cell during batch 6 and recorded, never silently dropped.**
  The rule is the same as for structural states: omit where the verb never probes and never touches the dirtied checkout, and record the omission with its reason in the omission table.
  Any such omission subtracts from the 130 above, and `doc.go`'s omission table plus batch 7's count assertion are what keep the recorded figure and the real figure equal.
- **Applies to:** all batches

### Decision: refusal-expectation-kinds

- **Decision:** A cell declares exactly one of three expectation kinds, and every kind carries a permitted-removal-roots field:
  `RefusedByGate(check)` — the error contains `"<check> check failed"`;
  `RefusedBefore(substring)` — the error contains `substring` **and** does **not** contain `"check failed"`;
  `Proceeds` — the verb succeeds, its intended effect lands, and the state's planted content survives.
- **The `"check failed"` exclusion on `RefusedBefore` is load-bearing, not defensive.**
  The gate's dirtiness reason at `destroy.go:564` is byte-identical to `Remove`'s own pre-flight message at `remove.go:74` — both are exactly `worktree has uncommitted changes; use --force` — so a gate refusal's full string contains the pre-flight string and a naive substring match would report a pre-flight refusal when the gate refused.
- **The exported `Check` set has exactly three members** — `CheckContainment`, `CheckOwnership`, `CheckDirtiness`.
  `checkForce` is declared at `destroy.go:39` and rendered by `String()` at `destroy.go:51` but is never constructed into a `destructiveRefusal` anywhere in the tree, so a `CheckForce` constant could never match and must not be offered.
- **Applies to:** batches 4, 6, 7

### Decision: prefix-rooted-permits

- **Decision:** A cell names permitted **roots**;
  everything at or below a permitted root may vanish or change.
  Anything outside every permitted root that disappears or changes is a failure.
  Refusal cells carry permit roots too — a refusal is not assumed side-effect-free.
- **The one known tranche-1 anomaly is declared out loud, not buried.** `Remove` runs `removePortal` and `removeLaunchers` at `remove.go:61-66`, **before** its own dirty pre-flight at `remove.go:68-76`, so a dirty-`Remove` cell that correctly refuses has already destroyed `_portals/<anchor>/<slug>` and `_launchers/<anchor>/<slug>`.
  Those two paths are permitted roots on that cell, and `doc.go` names the anomaly as the one tranche-1 case where a refusal is not side-effect-free, flagged for slice 14 rather than silently normalised.
- **Applies to:** batches 3, 6, 7

### Decision: no-production-behaviour-change

- **Decision:** `fabriccli.CloneAndWire` is a pure extraction — same sequence, same order, same errors.
  No other production file changes in this task.
  `internal/fabricengine/destroy.go` is untouched in the delivered diff, including during sabotage-proving, whose edits are local working-tree changes reverted immediately and never committed.
- **Applies to:** batches 1, 8

### Decision: commit-message-style

- **Decision:** Commit messages follow the repo's existing `<area>: <summary>` shape (`fabric: …`, `fabrictest: …`, `fabriccli: …`), matching `3184cd5a` and its neighbours in `git log`, not conventional-commit `type(scope):` prefixes.
- **Applies to:** all batches

## All Files Touched

- `CONSTRAINTS.md`
- `cmd/lyx/destructiveguard_test.go`
- `docs/overview.md`
- `internal/fabriccli/clone.go`
- `internal/fabricengine/fabrictest/doc.go`
- `internal/fabricengine/fabrictest/hub.go`
- `internal/fabricengine/fabrictest/hub_test.go`
- `internal/fabricengine/fabrictest/manifest.go`
- `internal/fabricengine/fabrictest/manifest_test.go`
- `internal/fabricengine/fabrictest/matrix_test.go`
- `internal/fabricengine/fabrictest/refusal.go`
- `internal/fabricengine/fabrictest/refusal_test.go`
- `internal/fabricengine/fabrictest/states.go`
- `internal/fabricengine/fabrictest/states_test.go`
- `internal/fabricengine/fabrictest/testmain_test.go`
- `internal/fabricengine/fabrictest/verbs.go`
- `internal/fabricengine/fabrictest/verbs_test.go`
- `internal/fabricengine/weftgit_exclude_test.go`
- `internal/lyxcwd/enforcement_test.go`
- `manifest/designs/fabric-crucible-followups.md`
- `manifest/roadmap.md`
