# Batch: lyxdirs-single-declarer

```yaml
task: .lyx hygiene -- relocate transients out of _lyx, fix .lyx junction geometry (slice 9)
batch: lyxdirs-single-declarer
number: 1
cards: 8
verify: go vet -tags integration ./... && go test ./...
depends-on: []
```

## Batch Scope

This batch creates the zero-dependency leaf `internal/lyxdirs` and makes it the single declarer of both directory-name tokens: `LyxDirName` (`"_lyx"`, moved out of `internal/configengine`) and `DotLyxDirName` (`".lyx"`, replacing five private `dotLyxDirName` consts).
It is one batch because it is a single mechanical identifier sweep whose halves cannot land separately: the moment `internal/lyxcwd/enforcement_test.go` starts policing `".lyx"` (card 7), every private declarer must already be gone (card 5), and the moment `configengine.LyxDirName` disappears (card 2), all 30 production references and all 26 test references must already point at the new home (cards 3–4).
No behaviour changes here — every resolved path is byte-identical before and after.

**External interface the later batches consume:** `lyxdirs.LyxDirName` and `lyxdirs.DotLyxDirName`, and the fact that a raw `"_lyx"`/`".lyx"` literal outside `internal/lyxdirs` now fails `go test`.

**Batch-local decision:** `internal/configengine` does **not** keep a deprecation alias for `LyxDirName`.
A `const LyxDirName = lyxdirs.LyxDirName` alias would leave two spellings in the tree and defeat the single-declarer point;
the compile errors from removing it outright are exactly the sweep list cards 3–4 work through.

**Batch-local decision:** `internal/lyxcwd` must NOT import `internal/lyxdirs`.
Its import set stays capped at stdlib plus `internal/gitexec` (Cwd Resolution Invariant), and it constructs neither token today — its `".lyx-anchor"` const in `anchor.go` is a different literal and never matches the exact-token rule.

## Cards

### Card 1: create the internal/lyxdirs leaf package

- **Context:**
  - `internal/configengine/config.go`
  - `internal/pattern/pattern.go`
  - `internal/lyxcwd/enforcement_test.go`
  - `docs/overview.md`
- **Edits:** none
- **Creates:**
  - `internal/lyxdirs/doc.go`
  - `internal/lyxdirs/dirs.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** create `package lyxdirs` with **zero** imports (stdlib included — none are needed).
  `dirs.go` declares exactly two exported consts: `LyxDirName = "_lyx"` and `DotLyxDirName = ".lyx"`.
  Each const's godoc states its half of the pair: `LyxDirName` is the durable, weft-synced, git-tracked directory;
  `DotLyxDirName` is the ephemeral, machine-bound, never-git-tracked sibling, and every never-tracked file lives under it at the mirrored subpath of the `_lyx` content it relates to.
  Both docs state that `internal/lyxdirs` is the sole declarer of its literal, enforced by `TestEnforcement_GeometryLiterals` in `internal/lyxcwd/enforcement_test.go`.
  `doc.go` carries the package doc: a stdlib-free leaf existing solely so `configengine`, `logger`, `lyxtest`, `fabricengine` and every module engine can name the two directories without any of them owning the pair, and without risking the `fabricengine` → `logger` → `lyxcwd` cycle.
  Do not add any accessor function — this package is two consts and nothing else.
  Match the file-header comment style of `internal/pattern/pattern.go` (a lowercase filename-prefixed sentence).
- **Commit:** `feat(lyxdirs): add zero-dependency leaf owning _lyx and .lyx`

### Card 2: remove LyxDirName from configengine and retarget its own uses

- **Context:**
  - `internal/lyxdirs/dirs.go`
  - `internal/fabricengine/config.go`
  - `internal/loomengine/config.go`
  - `internal/perchengine/config.go`
- **Edits:**
  - `internal/configengine/config.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** delete `const LyxDirName = "_lyx"` and its godoc from `internal/configengine/config.go`.
  Add the import `"github.com/Knatte18/loomyard/internal/lyxdirs"` and replace the two in-package uses — the `filepath.Join(cwd, LyxDirName)` at `config.go`'s `lyxDir` local and the `filepath.Join(baseDir, LyxDirName, configDirName)` in `ConfigDir` — with `lyxdirs.LyxDirName`.
  Update `configDirName`'s godoc, which currently says "subdirectory name within LyxDirName", to name `lyxdirs.LyxDirName`.
  Also retarget the two `_lyx` substrings in `FindBaseDir`'s error strings — `"not initialized: _lyx/ directory not found"` and `"stat _lyx: %w"` — onto the const, e.g. `fmt.Errorf("not initialized: %s/ directory not found", lyxdirs.LyxDirName)` and `fmt.Errorf("stat %s: %w", lyxdirs.LyxDirName, err)`.
  These are not caught by `TestEnforcement_GeometryLiterals` (a format-string operand is not a path-construction context), so they would otherwise be the one place in this file still hard-coding the name after the move.
  Do NOT change the observable message text — three packages substring-match on `"not initialized"` in their own `LoadConfig` (`internal/fabricengine/config.go`, `internal/loomengine/config.go`, `internal/perchengine/config.go`), so keep that prefix byte-identical.
  Doc-comment prose mentioning `_lyx` may stay as prose;
  only the const and these two format strings are retargeted.
  No alias const is left behind (see the batch-local decision).
- **Commit:** `refactor(configengine): move LyxDirName to internal/lyxdirs`

### Card 3: retarget every production configengine.LyxDirName reference

- **Context:**
  - `internal/lyxdirs/dirs.go`
  - `internal/configengine/config.go`
- **Edits:**
  - `internal/fabricengine/portals.go`
  - `internal/fabricengine/fabric.go`
  - `internal/fabricengine/reconcile.go`
  - `internal/fabricengine/weftwiring.go`
  - `internal/fabricengine/unwire.go`
  - `internal/fabricengine/junction.go`
  - `internal/fabricengine/junctionnames.go`
  - `internal/fabricengine/weftgit.go`
  - `internal/lyxtest/lyxtest.go`
  - `internal/websterengine/state.go`
  - `internal/builderengine/state.go`
  - `internal/perchengine/identity.go`
  - `internal/loomengine/config.go`
  - `internal/planparser/parse.go`
  - `internal/ideengine/menu.go`
  - `internal/buildercli/weft.go`
  - `internal/webstercli/weft.go`
  - `internal/perchcli/run.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** in each file replace every `configengine.LyxDirName` occurrence — in code **and** in doc comments — with `lyxdirs.LyxDirName`, add the `internal/lyxdirs` import, and drop the `internal/configengine` import from any file that no longer uses it for anything else.
  `internal/planparser/parse.go` uses `path.Join(configengine.LyxDirName, PlanDirName)` (note: `path`, not `filepath`) — keep `path.Join`, change only the identifier.
  `internal/loomengine/config.go` has four call sites (`PlanDir`, `DiscussionDir`, `LoomStatusFile`, `LoomStatusLock`) plus the `discussionDirName` godoc.
  `internal/fabricengine/weftgit.go` has the `crossModuleMachineLocalExcludes` var's three pattern strings plus the godoc above them — retarget the identifier only;
  the var itself is deleted in batch 6, not here.
  Files that still need `configengine` for `Load`/`ConfigDir`/`ConfigFile` keep both imports.
- **Commit:** `refactor: point production code at lyxdirs.LyxDirName`

### Card 4: retarget every test-file configengine.LyxDirName reference

- **Context:**
  - `internal/lyxdirs/dirs.go`
- **Edits:**
  - `internal/fabriccli/cli_test.go`
  - `internal/websterengine/webstergeom_test.go`
  - `internal/ideengine/menu_test.go`
  - `internal/fabricengine/config_test.go`
  - `internal/fabricengine/junction_pattern_integration_test.go`
  - `internal/fabricengine/unwire_test.go`
  - `internal/configengine/set_test.go`
  - `internal/configengine/edit_test.go`
  - `internal/configengine/config_test.go`
  - `internal/shuttleengine/config_test.go`
  - `internal/buildercli/weft_integration_test.go`
  - `internal/boardcli/cli_test.go`
  - `internal/perchcli/run_integration_test.go`
  - `internal/reedengine/config_test.go`
  - `internal/reedengine/contract_integration_test.go`
  - `internal/webstercli/weft_integration_test.go`
  - `internal/loomengine/discussionpath_test.go`
  - `internal/loomengine/planpath_test.go`
  - `internal/loomengine/loomstatus_test.go`
  - `internal/boardengine/config_test.go`
  - `internal/boardengine/boardtest/bench_test.go`
  - `internal/perchengine/config_test.go`
  - `cmd/lyx/constructoranchoring_test.go`
  - `cmd/lyx/exitcode_test.go`
  - `cmd/lyx/main_integration_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** same mechanical substitution as card 3, in test files: `configengine.LyxDirName` → `lyxdirs.LyxDirName`, adding the `internal/lyxdirs` import and dropping `internal/configengine` where it becomes unused.
  `internal/lyxcwd/enforcement_test.go` also references the identifier but is handled in card 7 instead, so this card must not touch it.
  Do not change any assertion's expected value — every path stays byte-identical.
  Several of these files carry `//go:build integration`;
  that line stays first in the file, untouched.
- **Commit:** `test: point tests at lyxdirs.LyxDirName`

### Card 5: delete the five private dotLyxDirName consts

- **Context:**
  - `internal/lyxdirs/dirs.go`
- **Edits:**
  - `internal/logger/sink.go`
  - `internal/scoutengine/daemonstate.go`
  - `internal/shuttleengine/rundir.go`
  - `internal/shuttleengine/run.go`
  - `internal/reedengine/lifecycle.go`
  - `internal/reedengine/lock.go`
  - `internal/reedengine/spawn.go`
  - `internal/reedengine/strand.go`
  - `internal/burlerengine/engine.go`
  - `internal/burlerengine/doc.go`
  - `internal/shuttleengine/rundir_test.go`
  - `internal/shuttleengine/run_test.go`
  - `internal/reedengine/spawn_test.go`
  - `internal/reedengine/strand_test.go`
  - `internal/reedengine/lock_test.go`
  - `internal/reedengine/contract_integration_test.go`
  - `internal/burlerengine/engine_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** delete the `const dotLyxDirName = ".lyx"` declaration and its godoc from each of the five declaring files (`internal/logger/sink.go`, `internal/scoutengine/daemonstate.go`, `internal/shuttleengine/rundir.go`, `internal/reedengine/lifecycle.go`, `internal/burlerengine/engine.go`), add the `internal/lyxdirs` import to each, and replace every `dotLyxDirName` use with `lyxdirs.DotLyxDirName`.
  Use sites, by file: `logger/sink.go`'s `WorktreeLogsDir`;
  `scoutengine/daemonstate.go`'s `DaemonStateFile` and `DaemonLock`;
  `shuttleengine/rundir.go`'s `runDirRoot` default branch, and `shuttleengine/run.go`'s `sweepOrphansOpportunistic` reed-state join;
  `reedengine/lifecycle.go`'s `HubLogsDir` plus its `SaveState`/`LoadState`/state-path joins, `reedengine/lock.go`'s `withOpLock`, `reedengine/spawn.go`'s two joins, and `reedengine/strand.go`'s three joins;
  `burlerengine/engine.go`'s `burlerDir` join.
  Also reword the prose in `internal/shuttleengine/rundir.go`'s `runDirRoot` godoc and `internal/burlerengine/doc.go` that says "this package's own `dotLyxDirName`" / "never a literal `.lyx` inline", and drop `internal/scoutengine/daemonstate.go`'s and `internal/shuttleengine/rundir.go`'s "stays unpoliced this slice; slice 9 registers a single owner" sentences — slice 9 is this task.
  Because `dotLyxDirName` was package-private, seven test files in the same packages reference it and must be retargeted in this card or the packages stop compiling: `internal/shuttleengine/rundir_test.go` and `run_test.go`, `internal/reedengine/spawn_test.go`, `strand_test.go`, `lock_test.go` and `contract_integration_test.go`, and `internal/burlerengine/engine_test.go` (which also mentions "this package's own `dotLyxDirName` join" in three comments).
  Each becomes `lyxdirs.DotLyxDirName` with the import added;
  keep `//go:build integration` first in `contract_integration_test.go`.
  Anchoring is NOT changed by this card: every join keeps whatever base (`l.WorktreePath()`, `l.HubPath`, `worktreePath`) it has today, in production code and in these tests alike.
  Re-anchoring is batch 4's job.
- **Commit:** `refactor: replace five private dotLyxDirName consts with lyxdirs.DotLyxDirName`

### Card 6: convert fabricengine's two raw _lyx literals

- **Context:**
  - `internal/lyxdirs/dirs.go`
- **Edits:**
  - `internal/fabricengine/status.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** in `detectHostPollution`, replace the bare `"_lyx"` in the `[]string{"ls-files", "--", "_lyx", "_pattern", "_raddle"}` argv slice and both `"_lyx"` operands in the `strings.HasPrefix(tracked, "_lyx") || tracked == "_lyx"` switch case with `lyxdirs.LyxDirName`, adding the import.
  Leave `"_pattern"` and `"_raddle"` exactly as they are — they are covered by their own owner rows and are out of this task's scope.
  Update the function's preamble comment, which currently explains the invariant carve-out for `"_pattern"` only, to note that `_lyx` now routes through `lyxdirs.LyxDirName` while `_pattern`/`_raddle` keep their literal form here.
- **Commit:** `refactor(fabricengine): source _lyx from lyxdirs in the pollution scan`

### Card 7: police both tokens and amend the leaf allowlists

- **Context:**
  - `internal/lyxdirs/dirs.go`
  - `internal/scoutengine/daemonstate.go`
  - `internal/lyxtest/lyxtest.go`
- **Edits:**
  - `internal/lyxcwd/enforcement_test.go`
  - `internal/scoutengine/leaf_enforcement_test.go`
  - `internal/lyxtest/doc.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** in `internal/lyxcwd/enforcement_test.go`'s `TestEnforcement_GeometryLiterals`: add `".lyx"` to the `geometryToken` closure's switch case list;
  change the `"_lyx"` row in `geometryTokenOwners` from `{"internal/configengine"}` to `{"internal/lyxdirs"}` and rewrite its comment to say the token's declaration moved to the new leaf;
  add a `".lyx": {"internal/lyxdirs"}` row with a comment stating it is the never-git-tracked half of the pair and that the five private declarers were retired in this slice;
  delete the whole paragraph above `geometryTokenOwners` that explains `".lyx"` is deliberately unpoliced pending slice 9.
  The only `configengine` mentions in this file are inside those two comment lines and disappear with the rewrite — no import change is needed, and this test file must not gain a `lyxdirs` import (it compares directory strings, never the const).
  In `internal/scoutengine/leaf_enforcement_test.go`, add `"github.com/Knatte18/loomyard/internal/lyxdirs": true` to `allowedImports` and name it in the file-header comment's allowed-import list.
  In `internal/lyxtest/doc.go`, extend the sentence "its import set is stdlib plus internal/lyxcwd, internal/weftname, and internal/configengine" to include `internal/lyxdirs`.
  Do not add `internal/lyxdirs` to any other package's allowlist — `internal/treadleengine`, `internal/githubclient`, `internal/modelspec`, `internal/tokenvocab` and `internal/pattern` do not import it, and an unused allowlist entry is exactly the drift this check exists to prevent.
- **Commit:** `test: police .lyx and re-own _lyx to internal/lyxdirs`

### Card 8: record the single-declarer invariant in the docs

- **Context:**
  - `internal/lyxdirs/dirs.go`
  - `internal/lyxcwd/enforcement_test.go`
  - `internal/scoutengine/leaf_enforcement_test.go`
- **Edits:**
  - `CONSTRAINTS.md`
  - `docs/overview.md`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** in `CONSTRAINTS.md`, add a new top-level section `## Lyxdirs Single-Declarer Invariant` stating that `internal/lyxdirs` is the sole declarer of `_lyx` and `.lyx`, that it stays stdlib-only (a zero-import leaf), that no other production file may name either literal in path-construction context, and that this is **Enforced by** `internal/lyxcwd/enforcement_test.go` (`TestEnforcement_GeometryLiterals`).
  Place it immediately after `## Cwd Resolution Invariant` so the geometry clauses stay adjacent.
  In the existing `## Scoutengine Leaf Invariant`, add `internal/lyxdirs` to the allowed-import prose list.
  In the existing `## lyxtest Leaf Invariant`, add `internal/lyxdirs` to its stated import set.
  In `docs/overview.md`, add a `├── internal/lyxdirs/` line to the source-tree block with the one-line description "the two directory-name tokens (`_lyx` durable, `.lyx` ephemeral), a zero-import leaf", placed next to `internal/lyxcwd/`, and add `internal/lyxdirs` to the shared-infrastructure package list in the paragraph that enumerates `internal/configengine`, `internal/gitexec`, … `internal/pattern`.
  Follow the repo's semantic-line-break markdown rule: one sentence per line, break long sentences at internal independent-clause boundaries, never hard-wrap at a fixed column.
- **Commit:** `docs: record the lyxdirs single-declarer invariant`

## Batch Tests

`verify: go test ./...` — the unbounded scope is deliberate and is the focused scope for this batch: the const move touches 19 production files across 18 packages and 26 test files across 20 packages, so the compile surface *is* the repo.
The overview's module-wide `go build ./... && go vet -tags integration ./...` additionally type-checks the `//go:build integration` test files card 4 edits, which a plain `go test ./...` never compiles.

The behavioural assertions that matter here already exist and must keep passing unchanged, since no resolved path changes:
`internal/lyxcwd/enforcement_test.go`'s `TestEnforcement_GeometryLiterals` (now covering `".lyx"` — this is the batch's own new coverage, and it fails if card 5 misses a declarer),
`internal/scoutengine/leaf_enforcement_test.go`'s `TestLeafInvariant_AllowlistOnly`,
`internal/lyxtest/leaf_enforcement_test.go`'s `TestLeafInvariant`,
`internal/treadleengine/seam_enforcement_test.go`'s `TestRunnerSeamInvariant_AllowlistOnly` (must still pass with **no** new allowlist entry),
`cmd/lyx/constructoranchoring_test.go`'s two anchoring tests,
and `internal/loomengine/loomstatus_test.go`, `internal/websterengine/webstergeom_test.go`, `internal/fabricengine/config_test.go`.

No new test file is added: card 1's package is two consts with no behaviour, and its ownership is asserted by the amended geometry-literal guard rather than by a `lyxdirs`-local test.
