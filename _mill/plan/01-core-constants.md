# Batch: core-constants

```yaml
task: "Rename hub container suffix from -HUB to -LYXHUB"
batch: "core-constants"
number: 1
cards: 3
verify: go test ./internal/lyxcwd/... ./internal/fabricengine/...
depends-on: []
```

## Batch Scope

This batch performs the only behaviour-bearing change in the task: it swaps the value of both sanctioned declarers of the hub-suffix literal, retires the `-HUB` row from the enforcement registry that polices them, and records the resulting cross-cutting rule in `CONSTRAINTS.md` — all in one commit, because `TestEnforcement_GeometryLiterals` fails on any partial move.
It then pins the two behaviours the swap changes: `HubPath` construction (one assertion moved off the constant onto the literal, so the test can fail if the constant moves again) and `RepoName` derivation (including the documented clean-break degradation for a hub still named with the retired suffix).

The external interface every later batch consumes is the new constant value `-LYXHUB` and the new policed token name.
Batches 2 through 6 are pure vocabulary sweeps over comments, fixtures, and prose;
none of them changes behaviour, and each depends on this batch only.

Batch-local decision beyond `## Shared Decisions`: card 1 is deliberately a single multi-file card rather than three, because the invariant it records requires all three sites to move in the same commit.

## Cards

### Card 1: Swap both declarers, the enforcement registry, and record the invariant

- **Context:**
  - `internal/fabricengine/clone.go`
  - `internal/fabricengine/portals.go`
  - `internal/fabricengine/launchers.go`
  - `internal/reedengine/server.go`
- **Edits:**
  - `internal/lyxcwd/lyxcwd.go`
  - `internal/fabricengine/junctionnames.go`
  - `internal/lyxcwd/enforcement_test.go`
  - `CONSTRAINTS.md`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  Work in this order so the enforcement test is what proves both declarers moved, rather than a grep.

  First, in `internal/lyxcwd/enforcement_test.go`, retire the old token outright — no row for it survives:
  - In the `geometryToken` closure inside `TestEnforcement_GeometryLiterals`, change the `switch` case list entry `"-HUB"` to `"-LYXHUB"`.
  - In the `geometryTokenOwners` map, change the key `"-HUB"` to `"-LYXHUB"`, keeping the owner slice `{"internal/lyxcwd", "internal/fabricengine"}` unchanged.
  - In the explanatory comment immediately above the `"_board"` entry of that map, change the phrase naming `"-HUB"` as dual-owned so it names `"-LYXHUB"` instead;
    the rest of that comment (the `boardDir`/`boardDirName` and `hubSuffix` explanation) stays as written.
  - In the `positives` table of AST-predicate fixtures, rename the fixture entry `const_HUB` to `const_LYXHUB` and change its `src` from ``package p; const s = "-HUB"`` to ``package p; const s = "-LYXHUB"``.

  Running `go test ./internal/lyxcwd/...` at this point must fail: the two production constants still declare the retired literal, which is now unowned for the new token.
  Observe that failure before continuing — it is the proof the registry actually polices the swap.

  Second, swap the private declarer in `internal/lyxcwd/lyxcwd.go`:
  - Change the `hubSuffix` const value from `"-HUB"` to `"-LYXHUB"`.
  - In its doc comment, change the example `"loomyard" → "loomyard-HUB"` to `"loomyard" → "loomyard-LYXHUB"`.
  - `buildLocation`'s `RepoName: strings.TrimSuffix(filepath.Base(hubPath), hubSuffix)` expression is unchanged — it reads the constant and needs no edit.

  Third, swap the exported declarer in `internal/fabricengine/junctionnames.go`:
  - Change the `HubSuffix` const value from `"-HUB"` to `"-LYXHUB"`.
  - In its doc comment, change the example `"loomyard" → "loomyard-HUB"` to `"loomyard" → "loomyard-LYXHUB"`.
  - `HubPath(parent, name)`'s `filepath.Join(parent, name+HubSuffix)` expression is unchanged.

  `go test ./internal/lyxcwd/... ./internal/fabricengine/...` must now pass.

  Fourth, add a new top-level section to `CONSTRAINTS.md`, placed immediately after the existing `## Hub Containment Invariant` section and immediately before `## gitkit Leaf Invariant`.
  Title it `## Hub Suffix Invariant`, and follow the file's established shape — a one-or-two-sentence claim, then bullets.
  The claim: `-LYXHUB` is the sole hub container suffix, and no code parses, trims, or recognises the retired `-HUB`.
  The bullets must record, in substance:
  - The suffix is declared twice by sanction — `internal/lyxcwd` holds the private `hubSuffix` const, used for `RepoName` derivation, and `internal/fabricengine` holds the exported `HubSuffix` const, used by `HubPath`.
    Both move together, and `TestEnforcement_GeometryLiterals`' `geometryTokenOwners` row is the third site that must move with them.
  - Hub discovery is name-independent — the hub is `filepath.Dir(workTreeRoot)` and `looksLikeHub` is structural — so a hub still carrying the retired suffix still resolves;
    only `Location.RepoName`, a display-only value never used to construct a path, degrades.
  - A hub carrying the retired suffix is never renamed in place: `PortalLink` and `LauncherDir` materialise links against the hub's absolute path at creation time, and `ServerName` hashes that absolute path into the tmux socket key.
    The operator removes the old container by hand and re-clones;
    `clone --reset` resolves the new suffix only and never reaches it.

  Follow the repository's semantic-line-break markdown rule in the new section: one sentence per line, and a break at internal independent-clause boundaries.
  Use the four Context files to confirm each claim before writing it — `looksLikeHub` in `internal/fabricengine/clone.go`, `PortalLink` in `internal/fabricengine/portals.go`, `LauncherDir` in `internal/fabricengine/launchers.go`, and `ServerName` in `internal/reedengine/server.go`.
- **Commit:** `refactor(hub): rename the hub container suffix from -HUB to -LYXHUB`

### Card 2: Sweep and pin `internal/fabricengine/junctionnames_test.go`

- **Context:**
  - `internal/fabricengine/junctionnames.go`
- **Edits:**
  - `internal/fabricengine/junctionnames_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  In `TestBoardDir`, the table case whose `hub` field is `"/repos/loomyard-HUB"` and whose `want` field is `filepath.Join("/repos/loomyard-HUB", BoardDirName)` carries the suffix as a synthetic path fixture.
  Change both occurrences to `"/repos/loomyard-LYXHUB"`.

  In `TestHubPath`, both table cases currently express `want` through the constant (`filepath.Join("/repos", "loomyard"+HubSuffix)` and `filepath.Join("/home/user/code", "myproject"+HubSuffix)`), which makes them tautological — they pass for any value the constant takes.
  Change the `want` of the `"simple repo name"` case to the hard literal `filepath.Join("/repos", "loomyard-LYXHUB")`, so a future change to `HubSuffix` fails this test.
  Leave the `"nested parent"` case expressed through `+HubSuffix`, so the test still covers that `HubPath` composes from the constant rather than from a hardcoded string of its own.

  Update `TestHubPath`'s doc comment to state that the mix is deliberate: one case pins the literal suffix value, the other pins composition through `HubSuffix`.
- **Commit:** `test(fabricengine): pin TestHubPath to the literal -LYXHUB suffix`

### Card 3: Pin `RepoName` suffix trimming, including the clean-break degradation

- **Context:**
  - `internal/lyxcwd/lyxcwd.go`
  - `internal/lyxcwd/gate_test.go`
- **Edits:** none
- **Creates:**
  - `internal/lyxcwd/reponame_test.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  Create an untagged test file in `package lyxcwd` (the in-package test package, matching `internal/lyxcwd/gate_test.go`'s style) holding a single table test named `TestBuildLocation_RepoNameSuffixTrimming`.

  The test calls `buildLocation` directly with `applyGate` set to `false`, so it spawns no git and touches no disk — this keeps it inside the untagged tier.
  `signature: buildLocation(cwd, workTreeRoot, hubPath, anchorRel string, applyGate bool) (*Location, error)` — signature inlined, no file read needed beyond confirming it.
  Pass a synthetic `cwd` and `workTreeRoot` (e.g. the hub path joined with a worktree name) and `"."` as `anchorRel`.

  Three cases, asserting the resulting `Location.RepoName`:
  - A hub container basename of `loomyard-LYXHUB` yields `RepoName == "loomyard"` — the current suffix is trimmed.
  - A hub container basename of `loomyard-HUB` yields `RepoName == "loomyard-HUB"` — the retired suffix is not trimmed.
    Give this case a comment recording that this is the documented, accepted degradation of the clean break, not a bug: nothing parses the retired suffix any more, and `RepoName` is display-only.
  - A hub container basename of `loomyard`, with no suffix at all, yields `RepoName == "loomyard"` — trimming a suffix that is not present is a no-op.

  Give the file a header comment stating that it covers `hubSuffix` trimming in `buildLocation` and pins the clean-break behaviour.
- **Commit:** `test(lyxcwd): pin RepoName suffix trimming and the -HUB degradation`

## Batch Tests

`verify: go test ./internal/lyxcwd/... ./internal/fabricengine/...` runs both packages whose production constants and tests this batch changes.

The untagged run covers the three gates that matter here:
- `TestEnforcement_GeometryLiterals` in `internal/lyxcwd/enforcement_test.go` — walks the whole repository source tree and fails if either declarer's value disagrees with the registered owner set for the policed token, which is what proves both constants moved together.
- `TestHubPath` and `TestBoardDir` in `internal/fabricengine/junctionnames_test.go` — construction through the constant, with one case now pinned to the literal.
- `TestBuildLocation_RepoNameSuffixTrimming` in the new `internal/lyxcwd/reponame_test.go` — derivation, including the retired-suffix degradation.

The scope is the two packages this batch edits, not the repository — the remaining packages carry only vocabulary fixtures and are swept in batches 3 through 6.
The repo-wide regression net for the task as a whole is `pipeline.done_gate`, already configured as `go test ./... && go test -tags integration ./...`.

The integration-tagged files in both packages are not run by this batch's verify;
none of them is edited here, and card 1's changes are compile-compatible with them because both constants keep their names and types.
