# Batch: retier-offline-loop

```yaml
task: 'Fix test-suite regression: slow Tier 1 + 2 red packages + stale benchmarks'
batch: retier-offline-loop
number: 2
cards: 6
verify: go test -tags integration ./internal/boardcli ./internal/perchcli ./internal/muxcli ./internal/configcli ./cmd/lyx -count=1
depends-on: []
```

## Batch Scope

Restore the offline tier's premise: move every git-spawning / fixture-copying
untagged test in `internal/boardcli`, `internal/perchcli`, `internal/muxcli`,
`internal/configcli`, and `cmd/lyx` behind `//go:build integration`, following
the pattern the sibling CLI packages (`idecli`, `initcli`, `weftcli`,
`warpcli`) already use. Genuinely spawn-free tests stay untagged so every
package keeps a Tier 1 presence. Tests move verbatim (Shared Decision
`moved-code-is-verbatim`); nothing is deleted or renamed (Shared Decision
`test-names-are-preserved`). Batch 3's tier-purity guard depends on this batch:
the guard must find zero untagged banned-token files when it lands. All cards
are same-package file surgery with no cross-card file overlap; they form one
batch because they share one idea (re-tiering) and one verify run.

## Cards

### Card 3: boardcli — tag contract file, extract spawn-free unit file

- **Context:**
  - `_mill/discussion.md`
  - `internal/idecli/cli_test.go`
- **Edits:**
  - `internal/boardcli/cli_test.go`
- **Creates:**
  - `internal/boardcli/cli_unit_test.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** Add `//go:build integration` to
  `internal/boardcli/cli_test.go` and extend its file doc comment with one
  sentence: seedCwd spawns `git init` and every `RunCLI` spawns `git
  rev-parse` via `hubgeometry.Resolve`, so the file is integration-tagged per
  the Test Tier Purity Invariant; spawn-free CLI tests live in
  `cli_unit_test.go`. Create untagged `internal/boardcli/cli_unit_test.go`
  (package `boardcli_test`) containing, moved verbatim from `cli_test.go`:
  the `runCLI` helper (it must live in the untagged file so both files see it
  — Shared Decision `helper-placement-under-split-tags`), `TestCLINoArg`, and
  `TestCLIUnknownSubcommand` (both use only `t.TempDir()`/`t.Chdir` and never
  reach layout resolution). Remove those three declarations from
  `cli_test.go`; `seedCwd` and everything else stays there. Fix both files'
  import blocks to exactly what each references (`cli_unit_test.go` needs
  `bytes`, `encoding/json`, `strings`, `testing`, and
  `github.com/Knatte18/loomyard/internal/boardcli`).
- **Commit:** `test(boardcli): gate git-spawning CLI tests behind integration tag`

### Card 4: perchcli — extract fixture-backed pause tests to integration file

- **Context:**
  - `_mill/discussion.md`
- **Edits:**
  - `internal/perchcli/cli_test.go`
- **Creates:**
  - `internal/perchcli/cli_integration_test.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** Create `internal/perchcli/cli_integration_test.go`
  (package `perchcli`, first line `//go:build integration`, short file doc
  comment naming the Test Tier Purity Invariant) containing, moved verbatim
  from `cli_test.go`: `seedPerchFixture`, `TestRunCLI_Pause_InvalidRunID`,
  `TestRunCLI_Pause_FinishedBlockRefused`,
  `TestRunCLI_Pause_NestedInitAnchorsRunDirsAtCwd`,
  `TestRunCLI_Pause_NoSuchRun`, and
  `TestRunCLI_Pause_WritesFlagAndIsIdempotent` — every declaration that
  touches `lyxtest.CopyPaired`. `cli_test.go` keeps `TestRunCLI_NoArgs`,
  `TestRunCLI_UnknownSubcommand`, `TestRunCLI_GroupGuard_OutsideGitRepo`,
  `TestCommand_EveryCommandHasShort`, and `TestRunCLI_Pause_MissingRunID`;
  update its file doc comment's sentence about "the pause-verb tests appended
  to this file" to point at `cli_integration_test.go`. Trim `cli_test.go`'s
  import block to `bytes`, `strings`, `testing`, and
  `github.com/spf13/cobra`; the integration file imports what its moved code
  references (`bytes`, `os`, `path/filepath`, `strings`, `testing`,
  `hubgeometry`, `lyxtest`, `muxengine`, `perchengine`, `shuttleengine`).
- **Commit:** `test(perchcli): gate fixture-backed pause tests behind integration tag`

### Card 5: perchcli — extract weft-sync run tests to integration file

- **Context:**
  - `_mill/discussion.md`
- **Edits:**
  - `internal/perchcli/run_test.go`
- **Creates:**
  - `internal/perchcli/run_integration_test.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** Create `internal/perchcli/run_integration_test.go`
  (package `perchcli`, `//go:build integration`, short doc comment naming the
  Test Tier Purity Invariant) containing, moved verbatim from `run_test.go`:
  `TestRunCLI_Run_WeftSyncRunsOnEngineError`,
  `TestRunCLI_Run_WeftCommitExcludesLockFiles`,
  `TestRunCLI_Run_BusyBlockSkipsWeftSync`, and the `gitLsFiles` and
  `gitLogOneline` helpers — everything that touches `lyxtest.CopyPairedLocal`
  or `exec.Command`. `run_test.go` keeps `TestRunCLI_Run_MissingProfile`,
  `TestRunCLI_Run_InvalidRunID`, `TestDecodeProfile`,
  `TestDecodeProfile_FullValidFieldMapping`, `TestRunIdentity_DeriveRunIDShape`,
  `TestDeriveBlockRunID_StableAcrossTuningOverlay`,
  `TestDecodeProfile_EmptyRoundCapsStaysNonNil`, and
  `TestResolveRunTarget_DerivesIDBeforeOverlay`; update its file doc comment
  to note the weft-sync tests now live in `run_integration_test.go`. Trim
  `run_test.go`'s imports to what remains (drops `os/exec`, `hubgeometry`,
  `lock`, `lyxtest`, `muxengine`, `shuttleengine`; keeps `bytes`, `os`,
  `path/filepath`, `strings`, `testing`, `time`, `perchengine` — verify by
  compiling); the integration file imports what its moved code references.
- **Commit:** `test(perchcli): gate weft-sync run tests behind integration tag`

### Card 6: muxcli — extract fixture-backed seam tests to integration file

- **Context:**
  - `_mill/discussion.md`
- **Edits:**
  - `internal/muxcli/cli_test.go`
- **Creates:**
  - `internal/muxcli/cli_integration_test.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** Create `internal/muxcli/cli_integration_test.go` (package
  `muxcli`, `//go:build integration`, short doc comment naming the Test Tier
  Purity Invariant) containing, moved verbatim from `cli_test.go`:
  `TestRunCLI_ResolvesLayoutAndConfig`, `TestRunCLI_AddNotUp_FriendlyError`,
  and `TestRunCLI_RemoveNotUp_FriendlyError` — the three tests using
  `lyxtest.CopyPaired`. `cli_test.go` keeps `TestRunCLI_NoArgs`,
  `TestRunCLI_UnknownSubcommand`, `TestRunCLI_NotAGitRepo`, and
  `TestAttachArgv`; update its file doc comment ("config resolution against a
  real fixture hub" now lives in `cli_integration_test.go`). Trim
  `cli_test.go`'s imports (drops `lyxtest` and `muxengine`); the integration
  file imports `bytes`, `encoding/json`, `strings`, `testing`, `lyxtest`,
  `muxengine`.
- **Commit:** `test(muxcli): gate fixture-backed seam tests behind integration tag`

### Card 7: configcli — move git-init tests behind integration tag

- **Context:**
  - `_mill/discussion.md`
- **Edits:**
  - `internal/configcli/configcli_test.go`
  - `internal/configcli/configcli_integration_test.go`
  - `internal/configcli/reconcile_test.go`
- **Creates:**
  - `internal/configcli/reconcile_integration_test.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** Move `TestDispatchSet_PreservedKeyDetectedByReconcile`
  verbatim from `configcli_test.go` into the existing integration-tagged
  `configcli_integration_test.go` (it spawns `gitexec.RunGit(["init"], …)`).
  Create `internal/configcli/reconcile_integration_test.go` (same package as
  `reconcile_test.go`, `//go:build integration`, short doc comment naming the
  Test Tier Purity Invariant) containing `TestReconcile_DryRun` and
  `TestReconcile_Apply` moved verbatim from `reconcile_test.go` (both spawn
  `gitexec.RunGit(["init"], …)`); `reconcile_test.go` keeps
  `TestReconcile_NotAGitRepo` (no spawn — it asserts the not-a-git-repo error
  path). Fix all four files' import blocks to exactly what each references
  (in particular, drop `gitexec` from any file that no longer spawns).
- **Commit:** `test(configcli): gate git-init tests behind integration tag`

### Card 8: cmd/lyx — move git-backed dispatch tests, tag cross-compile gate

- **Context:**
  - `_mill/discussion.md`
- **Edits:**
  - `cmd/lyx/main_test.go`
  - `cmd/lyx/crosscompile_test.go`
- **Creates:**
  - `cmd/lyx/main_integration_test.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** Two independent gatings in one package. (1) Create
  `cmd/lyx/main_integration_test.go` (package `main`, `//go:build
  integration`, short doc comment naming the Test Tier Purity Invariant)
  containing `TestRunDispatchesToBoard`, `TestRunBoardErrorPropagatesExitCode`,
  and `TestRunDispatchesToConfigReconcile` moved verbatim from `main_test.go`
  — the three tests spawning `gitexec.RunGit(["init"], …)`. `main_test.go`
  keeps every other test (in-process cobra dispatch, no spawns); trim both
  files' import blocks accordingly. (2) Add `//go:build integration` as the
  first line of `cmd/lyx/crosscompile_test.go` and update its file doc
  comment: the durable Linux cross-compile gate now runs on every Tier 2
  (`-tags integration`) run rather than on every `go test`, because a
  whole-module `GOOS=linux go build ./...` does not belong in the offline
  loop (Test Tier Purity Invariant); the per-batch `GOOS=linux go build`
  development gates it mirrors are unchanged. (3) Equivalence check before
  committing (Shared Decision `test-names-are-preserved`): run
  `go test -tags integration ./internal/boardcli ./internal/perchcli
  ./internal/muxcli ./internal/configcli ./cmd/lyx -list '.*'` and confirm the
  emitted name set equals the union of the names enumerated in cards 3–8
  (which are exhaustive), and run the same `-list` without the tag to confirm
  each of the five packages still exposes its untagged remainder. Any missing
  name means a misplaced declaration in cards 3–8 — fix placement before
  committing.
- **Commit:** `test(cmd/lyx): gate git-backed dispatch tests and cross-compile gate behind integration`

## Batch Tests

`verify:` runs the five touched packages under `-tags integration -count=1`,
which compiles and executes BOTH the tagged and untagged test files in each
package (the integration build is the superset) — proving the moved tests
still pass, the trimmed import blocks compile under both build configurations
that include the files, and no test was lost (card 8's `-list` equivalence
check makes name preservation explicit). The plain untagged build is additionally
exercised by the module-wide overview verify (`go test ./... -count=1`) at
the batch boundary, which now runs without any git spawn in these packages.
