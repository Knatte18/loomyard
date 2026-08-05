MILL_REVIEW_BEGIN
# Review: fabric: shrink hubgeometry to the minimal illusion primitive (slice 7) — holistic

```yaml
verdict: REQUEST_CHANGES
reviewer_model: fablehigh
reviewer_self_id: Claude (Fable 5, model id claude-fable-5)
reviewed_file: plan/
date: 2026-08-05
```

## Findings

### [BLOCKING] Card 2 deletes config symbols with the module's own unit test uncovered
**Location:** batch 1, card 2
**Issue:** `hubgeometry_unit_test.go` (in-package, untagged) calls the deleted symbols unqualified — `ConfigDir` (`:23`), `ConfigFile` (`:36`), `DotEnv` (`:96`), `LyxDirName` in `TestLyxDirNameConstant` (`:109`) and in the want-expressions of the surviving `PerchRunsDir`/`PlanDir`/`BuilderDir`/`BuilderReportsDir` sub-tests (`:49,61,73,85`) — and the file is in no batch-1 card's `Edits:` with any instruction for these, so batch 1's own `go test ./...` fails to compile package `hubgeometry`.
**Fix:** Add `hubgeometry_unit_test.go` to card 2's `Edits:` naming the disposition: delete the `ConfigDir`/`ConfigFile`/`DotEnv`/`TestLyxDirNameConstant` sub-tests (coverage superseded by `configengine`'s own tests card 2 edits) and retarget the surviving want-expressions to `configengine.LyxDirName` (legal — after card 2 `configengine` no longer imports `hubgeometry`).

### [BLOCKING] Card 3's fallout enumeration misses two sub-tests in files it lists
**Location:** batch 1, card 3
**Issue:** The card claims the `Prime`/`WeftRepoRoot`/`deriveRepo` deletions reach "four test files", but `geometry_test.go`'s `TestWeftLayoutMethodParity` sets `Prime:` (`:180`) and asserts `layout.WeftRepoRoot()` (`:192`), and `hubgeometry_unit_test.go`'s `TestDeriveRepo` (`:154-192`) calls the deleted `deriveRepo` directly — both files are in card 3's `Edits:` with no instruction naming either sub-test, so batch 1's gate goes red on unenumerated fallout.
**Fix:** Name both: drop the `Prime:` field and the `WeftRepoRoot` assertion from `TestWeftLayoutMethodParity` (its `WeftWorktreePath`/`WeftWorktree` halves survive until card 31), and delete `TestDeriveRepo` with its subject.

### [BLOCKING] Card 34 privatizes BoardDir/IsReservedHubName with two lyxcwd test files unlisted
**Location:** batch 6, card 34
**Issue:** `lyxcwd/anchor_test.go:26` (external package, integration-tagged — compiled by batch 6's tagged vet and `go test -tags integration ./internal/lyxcwd/...`) calls `BoardDir(hub)` and is in no batch-6 card at all, and `lyxcwd_test.go`'s `TestIsReservedHubName_Pattern` (`:786-791`) calls the moved `IsReservedHubName` and is not in card 34's `Edits:` (cards 32/33 edit that file only for the junction/portal lifts); `lyxcwd/geometry_test.go` is listed but its `TestBoardDir`/`TestHubPath`/`TestIsReservedHubName` sub-tests (`:49-107`, `:212-289`) get no named disposition either.
**Fix:** Add `anchor_test.go` and `lyxcwd_test.go` to card 34's `Edits:` and name each rewrite — external-package lyxcwd tests may import `fabricengine` (`fabricengine.BoardDir`/`fabricengine.IsReservedHubName`), or the sub-tests move to `fabricengine/junctionnames_test.go` — and state what happens to geometry_test.go's three sub-tests.

### [BLOCKING] Card 35 deletes HostPatternLink(Here) with two caller files unlisted
**Location:** batch 6, card 35
**Issue:** `fabricengine/reconcile_stale_removal_test.go:113,273,380` call `hostLayout.HostPatternLinkHere()` and `remove_junctions_integration_test.go:69` calls `nestedLayout.HostPatternLink(slug)` — both `package fabricengine_test`, neither in card 35's `Edits:` (they appear in cards 31/32/34 for other symbols only, with no pattern-accessor rewrite named), so batch 6's `go test ./internal/fabricengine/...` fails to compile after the deletion.
**Fix:** Add both files to card 35's `Edits:` with the generic-join rewrite named (`filepath.Join(fabricengine.HostLyxLinkHere(...)`-style base + `pattern.DirName`, matching the card's rule for the other test readers).

### [NIT] Card 18 renames the lyxtest Leaf Invariant instead of correcting it
**Location:** batch 4, card 18
**Issue:** The card's CONSTRAINTS.md instruction is rename-only for the five other invariants, but after cards 1-2 `lyxtest` also imports `internal/weftname` and `internal/configengine`, so "imports only stdlib and `internal/lyxcwd`" would be a false statement of the (banned-list) guard.
**Fix:** Have card 18 restate the lyxtest invariant's import claim to name the real post-batch-1 import set.

### [NIT] Batch-5/6 relocations leave four comments naming symbols lyxcwd no longer has
**Location:** batches 5-6, cards 21/22/23/34
**Issue:** Card 18's mechanical rename produces `lyxcwd.BuilderDir` (`builderengine/state.go`), `lyxcwd.PerchRunsDir` (`perchengine/identity_test.go:`), `lyxcwd.WebsterPromptsDir` (`fabricengine/weftgit.go`) and `lyxcwd.BoardDir` (`remove_junctions_integration_test.go:76`) in comments, which go stale when cards 21/23/22/34 move those symbols, and card 41's zero-residue grep polices only `hubgeometry` so they survive the plan.
**Fix:** Add the four comment fixes to the cards that relocate each symbol (or to card 41's closing sweep).

## Verdict

REQUEST_CHANGES
Four caller-coverage misses — two break batch 1's own gate, two break batch 6's.
MILL_REVIEW_END
