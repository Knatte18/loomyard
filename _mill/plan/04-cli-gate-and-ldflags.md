# Batch: cli-gate-and-ldflags

```yaml
task: "lift the orchestrator preflight out of loomengine, plus the shared standalone-CLI foundations"
batch: "cli-gate-and-ldflags"
number: 4
cards: 4
verify: go test ./cmd/lyx/... ./tools/deploy/... ./internal/lyxcwd/... && go test -tags integration ./cmd/lyx/...
depends-on: [1, 3]
```

## Batch Scope

This batch wires the two new surfaces into the CLI and closes the live defect.
`cmd/lyx/stencilseed.go` stops declaring its own build-channel variable and stops seeding on a bare `lyxcwd.Resolve` success: it reads `buildinfo.IsDev()` through `stencilstore.ModeFor` (batch 1) and gates the seed on `preflight.HubPresent` (batch 3).
`tools/deploy/main.go`'s ldflags path is repointed at the variable's new home in the same batch, because a stale `-X main.buildChannel=dev` against a removed `main.buildChannel` fails **silently** — Go's linker does not error on an unmatched `-X` — producing a dev binary that behaves as production with no build error and no visible symptom.
The drift guard in `tools/deploy/main_test.go` is what turns that same-commit rule from a review obligation into a machine check; `tools/` lies outside every existing enforcement walk, so nothing else in the repo would ever catch the mismatch.

Batch-local decision: the seed gates on `preflight.HubPresent`, never `preflight.Wired`.
Gating on `Wired` would stop seeding stencils at `<hub>/_board`, in an unpaired sibling, and in a worktree whose pair was removed — three real-hub situations that seed correctly today.
Card 16's three positive rows exist specifically so that narrowing cannot land unnoticed.

## Cards

### Card 15: gate the stencil seed on hub presence and read the build channel from buildinfo

- **Context:**
  - `internal/preflight/predicates.go`
  - `internal/buildinfo/buildinfo.go`
  - `internal/stencilstore/stencilstore.go`
  - `cmd/lyx/main.go`
  - `cmd/lyx/helptree_test.go`
  - `cmd/lyx/registration_test.go`
- **Edits:**
  - `cmd/lyx/stencilseed.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  Delete the package-level `var buildChannel string` declaration and its doc comment from `cmd/lyx/stencilseed.go`; `internal/buildinfo` owns that variable now.

  In `seedStencilsAt`, replace the three-line `mode := stencilstore.ModeProduction` / `if buildChannel == "dev"` / `mode = stencilstore.ModeDev` sequence with a single `mode := stencilstore.ModeFor(buildinfo.IsDev())`.
  Change nothing else about `seedStencilsAt` — the `fabricengine.StencilsDir` base, the `contracts/stencils` source-dir probe with its empty-string-means-no-source-tree comment, the `stencilstore.Reconcile` call, and the `fabricengine.CommitSeededStencils` call all stay exactly as they are.

  Extract the gate into a new function `func stencilSeedTarget(ctx context.Context) (hub, worktree string, ok bool)`.
  It calls `lyxcwd.CwdFrom(ctx)` and returns `("", "", false)` on error, exactly as today — the root pre-run resolves no hub for commands that legitimately have none, so the pass is skipped rather than failing.
  It then calls `preflight.HubPresent(cwd)` and returns `("", "", false)` when the predicate is false, and otherwise returns `(l.HubPath, l.WorktreePath(), true)`.
  It must not call `lyxcwd.Resolve` itself — `preflight.HubPresent` performs that resolution and returns the `*lyxcwd.Location`, so a second call would double the `git rev-parse` spawn this pre-run performs before every single command.
  Document on `stencilSeedTarget` why it exists as a separate value-returning function: `seedStencils` returns immediately under `testing.Testing()`, so a test can never observe the gate through it, and extracting the decision is what makes the gate directly assertable — the same rationale `seedStencilsAt` already carries.

  Reduce `seedStencils(ctx context.Context)` to its `testing.Testing()` early return, with its existing comment intact, followed by a call to `stencilSeedTarget` and, when `ok` is true, a call to `seedStencilsAt(hub, worktree)`.

  Document on `stencilSeedTarget` that the predicate is `preflight.HubPresent` and not `preflight.Wired`, and why: the write this pass performs targets the hub-level stencils directory, so the honest precondition is that the hub-level directory exists, whereas `preflight.Wired` probes this worktree's own pairing and would stop seeding in three real-hub situations that seed correctly today.
  Without that note the next reader "fixes" the gate onto `preflight.Wired`.

  Keep the file's existing header comment accurate — it currently describes `seedStencils` as resolving geometry and being a no-op under `go test`, and now there are three functions, not two.
  Registration, `Short` strings, and the help tree must be untouched, so `cmd/lyx/helptree_test.go` and `cmd/lyx/registration_test.go` keep passing unedited.
  Do not use the tokens `weft` or `warp` anywhere in this file.
- **Commit:** `fix(lyx): gate stencil seeding on hub presence, not bare geometry`

### Card 16: prove the gate closes the fictional-hub write without narrowing a working path

- **Context:**
  - `cmd/lyx/stencilseed.go`
  - `cmd/lyx/testmain_test.go`
  - `internal/hubforge/hub.go`
  - `internal/gitkit/gitkit.go`
  - `internal/fabricengine/junctionnames.go`
  - `internal/lyxcwd/lyxcwd.go`
- **Edits:** none
- **Creates:**
  - `cmd/lyx/stencilseed_integration_test.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  A test file whose first non-empty line is `//go:build integration`, in `package main` alongside the rest of `cmd/lyx`'s tests.
  It spawns git through its fixtures, and `cmd/lyx/testmain_test.go` already supplies the `TestMain` calling `gitkit.HermeticGitEnv()`, so no new `TestMain` is added.

  Drive `stencilSeedTarget` directly, threading the cwd in with `lyxcwd.WithCwd(ctx, dir)` rather than changing the process working directory — `cmd/lyx/cwdmutation_test.go` guards this package's migrated files against reintroducing `t.Chdir(` or `os.Chdir(`, and a new file should not add one.

  The negative row is the defect this task exists to close: build a plain git repository under `t.TempDir()` with no hub-level sibling directory beside it, call `stencilSeedTarget` against its root, and assert both that `ok` is false **and** that no hub-level `_board` directory was created beside the repository afterwards.
  Assert the absence on disk, not merely the boolean — the boolean pins an implementation detail, the absent directory pins the actual defect.
  Reach that expected-absent path through `fabricengine.BoardDir` applied to the repository's parent directory rather than joining a `_board` literal, which the geometry-literal enforcement walk forbids.

  Then three positive rows against a `hubforge.NewHub` fixture, each asserting `ok` is true and that the returned `hub` and `worktree` match the fixture's own values:

  1. cwd at an ordinary worktree in the hub;
  2. cwd at the hub-level board directory (`fabricengine.BoardDir` of the fixture's hub);
  3. cwd at a worktree whose paired sibling has been removed.

  Every one of these three would fail if the gate were `preflight.Wired`, which is exactly why they are here: without them the gate could be narrowed to always-false in the common cases and nothing would catch it.
- **Commit:** `test(lyx): pin the stencil-seed gate against plain repos and three real-hub cases`

### Card 17: repoint the ldflags path at internal/buildinfo

- **Context:**
  - `internal/buildinfo/buildinfo.go`
  - `cmd/lyx/stencilseed.go`
- **Edits:**
  - `tools/deploy/main.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  In the `if dev` branch that appends the linker flags, change the `-X` argument from `-X main.buildChannel=dev` to `-X github.com/Knatte18/loomyard/internal/buildinfo.Channel=dev`.
  Change nothing else about the build invocation — the `-o dest` output path, the `./cmd/lyx` package argument, the working directory, and the stdout/stderr wiring all stay as they are.
  Update the comment above the append so it names the variable's new home and keeps the existing dev-builds-seed-but-do-not-refresh reasoning.
  This must land in the same commit as card 15's removal of `cmd/lyx/stencilseed.go`'s `buildChannel`: Go's linker silently ignores an unmatched `-X`, so a stale path against a removed variable produces a dev binary that behaves as production with no build error and no test failure.
- **Commit:** `fix(deploy): stamp the build channel at its new internal/buildinfo home`

### Card 18: guard the ldflags path against silent drift

- **Context:**
  - `tools/deploy/main.go`
  - `internal/buildinfo/buildinfo.go`
- **Edits:**
  - `tools/deploy/main_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  Add one untagged test to the existing `package main` test file, keeping the file's current tier-1 purity: read source files with `os.ReadFile` and spawn nothing — no `go build`, no `go env`, no `exec.Command`.

  Locate the repository root relative to the test's own source file via `runtime.Caller(0)` and `filepath.Dir` — `tools/deploy` is two levels below the root — then assert two things:

  1. `tools/deploy/main.go`'s source contains the exact string `-X github.com/Knatte18/loomyard/internal/buildinfo.Channel=dev`.
  2. `internal/buildinfo/buildinfo.go`'s source declares an exported `Channel` variable, so the symbol the linker is asked to stamp actually exists at that path.
     Parse it with `go/parser` and inspect the declared names rather than substring-matching the whole file, so the assertion is about a real declaration and not an incidental mention in a comment.

  Word the failure message so it says what silently breaks: Go's linker does not error on an unmatched `-X`, so a rename of either the variable or its package leaves a `-dev` build behaving as production, with no build error, no test failure, and no visible symptom until someone notices stencils refreshing when they should not.
  Keep the file's existing header comment accurate — it currently says the tests cover `resolveDest` only.
- **Commit:** `test(deploy): guard the buildinfo ldflags path against silent drift`

## Batch Tests

`verify:` runs `go test ./cmd/lyx/... ./tools/deploy/... ./internal/lyxcwd/...` followed by `go test -tags integration ./cmd/lyx/...`.

The untagged `./cmd/lyx/...` run is the important half for card 15: that package hosts `tierpurity_test.go`, `hermeticenv_test.go`, `cwdmutation_test.go`, `helptree_test.go`, `registration_test.go`, `notransients_test.go` and `constructoranchoring_test.go`, so an untagged git spawn, a missing `TestMain`, a reintroduced `os.Chdir`, or an accidental change to the help tree all fail here rather than at the done gate.
`./tools/deploy/...` covers the existing `resolveDest` tests plus card 18's new drift guard.
The tagged run is required because card 16 creates `cmd/lyx/stencilseed_integration_test.go`, which carries `//go:build integration` and is not compiled at all without the tag — the negative plain-repo row and the three positive real-hub rows would otherwise never execute.
`./internal/lyxcwd/...` is included for `TestEnforcement_FabricVocabulary` and `TestEnforcement_GeometryLiterals`, which are the only checks covering the edited `cmd/lyx/stencilseed.go` for a vocabulary or geometry-literal slip.
`tools/` lies outside both enforcement walks, so vocabulary in `tools/deploy/main.go` stays a review obligation.
