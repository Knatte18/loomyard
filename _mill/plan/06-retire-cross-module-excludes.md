# Batch: retire-cross-module-excludes

```yaml
task: .lyx hygiene -- relocate transients out of _lyx, fix .lyx junction geometry (slice 9)
batch: retire-cross-module-excludes
number: 6
cards: 3
verify: go test ./internal/fabricengine/... && go test -tags integration ./internal/fabricengine/...
depends-on: [5]
```

## Batch Scope

This batch deletes `crossModuleMachineLocalExcludes` — the three deep wildcard patterns in `internal/fabricengine/weftgit.go` that existed purely to stop misplaced `_lyx` transients from being committed — now that batches 2–4 have moved every one of those artifacts into `.lyx` and batch 5 machine-guards against them coming back.
`seedWeftArtifactExcludes` keeps seeding fabric's own two operational artifacts (`.weft/` and `gitrepo.PushLockFileName`), which are fabric's runtime artifacts rather than module transients and are explicitly out of this task's scope.

It is one batch because the deletion, its test rewrite, and retiring the `CONSTRAINTS.md` clause and the two docs that describe the mechanism are the same change: leaving any of them behind would document a layer that no longer exists.

**Batch-local decision — the `.lyx/` weft-side exclude entry is NOT added here.**
It belongs with the wiring that materializes the weft-side `.lyx` target (batch 8), because seeding an exclude for a directory that no weft worktree yet contains would be a no-op line whose ordering guarantee nothing tests.
Between this batch and batch 8 nothing is unprotected: every relocated transient now lives under the host-side `.lyx`, which is still kept out of the warp repo by the committed `.gitignore` block batch 8 replaces, and is not inside any weft worktree at all.

## Cards

### Card 33: delete crossModuleMachineLocalExcludes

- **Context:**
  - `internal/lyxdirs/dirs.go`
  - `internal/gitrepo/push.go`
  - `internal/websterengine/state.go`
  - `internal/builderengine/state.go`
- **Edits:**
  - `internal/fabricengine/weftgit.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** delete the `crossModuleMachineLocalExcludes` var and its entire ~40-line godoc block.
  In `seedWeftArtifactExcludes`, change the entries slice from `append([]string{weftLockDirName + "/", gitrepo.PushLockFileName}, crossModuleMachineLocalExcludes...)` to the two-element literal `[]string{weftLockDirName + "/", gitrepo.PushLockFileName}`.
  Rewrite `seedWeftArtifactExcludes`'s own godoc: it seeds fabric's own operational artifacts only — the `.weft/` lock directory and gitrepo's push lock — and no longer carries any module's machine-local patterns, because every module transient now lives under `.lyx` and never enters a weft worktree.
  Keep the rest of the doc's reasoning intact (the exclude file lives in the repo's common gitdir so one seeding covers every linked weft worktree, and it heals worktrees already carrying the artifacts as untracked), and keep the reference to `commitWeft`'s doc for the already-committed limit.
  Drop the `internal/lyxdirs` import from this file if the deletion leaves it unused, and keep `internal/gitrepo`.
  Do not touch `weftPathspecFilter`/`entryMatchesWeft` — the `--exclude-standard` fix is batch 7's.
- **Commit:** `refactor(fabricengine): delete crossModuleMachineLocalExcludes`

### Card 34: rewrite the exclude-layer tests

- **Context:**
  - `internal/fabricengine/weftgit.go`
  - `internal/lyxtest/lyxtest.go`
  - `internal/fabricengine/testmain_test.go`
- **Edits:**
  - `internal/fabricengine/weftgit_exclude_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** this file's `//go:build integration` line stays first.
  Rewrite the tests that assert the deleted patterns.
  `seedWeftArtifactExcludes` must be asserted to seed exactly `.weft/` and gitrepo's push-lock filename, to seed **none** of the three retired `**/_lyx/...` patterns, and to stay idempotent on re-run (a second call leaves the file byte-identical).
  The test near the file's end that reproduces the historic pollution — a `fabric sync`-shaped `ScopedPathspec(rel, []string{"_lyx"})` commit sweeping every module's locks and pause flags into weft history — must be rewritten rather than deleted: keep the same commit shape, but seed the machine-local artifacts under the worktree's `.lyx` tree instead of under `_lyx`, and assert the resulting weft commit carries the durable `_lyx` content while `git ls-files` in the weft worktree returns no `.lock`, no `pause` and no `prompts` entry.
  That preserves the property the old test protected (never committed, never materialized on another machine) while proving it now holds structurally rather than by exclusion.
  Retarget any `[]string{"_lyx"}` literal in this file to `lyxdirs.LyxDirName`, adding the import.
  Update the file-header comment, which today frames the file as covering "artifacts a pathspec-scoped `fabric sync` can never clear".
- **Commit:** `test(fabricengine): assert the exclude file carries fabric artifacts only`

### Card 35: retire the cross-module-exclusions clause and its docs

- **Context:**
  - `internal/fabricengine/weftgit.go`
  - `cmd/lyx/notransients_test.go`
  - `cmd/lyx/sandbox_coverage_test.go`
- **Edits:**
  - `CONSTRAINTS.md`
  - `docs/reference/builder-contract.md`
  - `tools/sandbox/SANDBOX-BUILDER-SUITE.md`
  - `tools/sandbox/SANDBOX-WEBSTER-SUITE.md`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** in `CONSTRAINTS.md`'s `## Fabric Git Invariant (warp + weft)`, rewrite the `**Cross-module exclusions.**` bullet.
  Keep its first clause — every weft-commit caller passes a positive-only file list built via `fabricengine.ScopedPathspec`, no `:(exclude)` pathspec magic.
  Replace the second clause, which today says machine-local artifacts "are excluded **solely** at the git-exclude layer (`fabricengine.seedWeftArtifactExcludes`), never per-call", with the new reality: machine-local artifacts are not in the weft tree at all — they live under `.lyx` (see the Durable-vs-Ephemeral State Invariant), and `seedWeftArtifactExcludes` now covers only fabric's own `.weft/` and push-lock artifacts.
  **Keep the `**Known limitation:**` sentence** — a pre-fix sync's already-committed artifacts still need a manual `git rm --cached <path>`, and that is the documented remedy, deliberately not automated by this task.
  In `docs/reference/builder-contract.md`, rewrite the sentence describing `*.lock` and `*/builder/pause` as "kept out solely at the git-exclude layer (`fabricengine.seedWeftArtifactExcludes`)" to say they live under `.lyx` and so never reach a weft pathspec;
  keep the surrounding "three weft-commit points" structure and its rationale untouched.
  Check the same file's earlier `run.lock` section, which says "Like every `*.lock`, it is excluded from fabric commits", and correct it the same way.
  In `tools/sandbox/SANDBOX-BUILDER-SUITE.md`'s scenario **B10**, rewrite the Goal and Watch text: the property being proven is unchanged (no machine-local artifact ever appears in a weft commit or as a tracked file), but the artifacts are now at `.lyx/builder/pause`, `.lyx/webster/pause`, `.lyx/webster/prompts/*.md` and `.lyx/**/*.lock`, and the mechanism is that they are outside the weft tree rather than held back by an exclude layer.
  Keep the `**Covers:** builder` line, the scenario id, and the `**Verdict:**` line exactly as they are — `cmd/lyx/sandbox_coverage_test.go` parses the Covers line.
  Also update its note that the sandbox is the one known repo carrying a committed `.gitignore` `.lyx/` block from a pre-fix binary, stating the manual remedy (delete the `.lyx/` line from the lyx-managed block) since no code path removes it.
  In `tools/sandbox/SANDBOX-WEBSTER-SUITE.md`, change the `_lyx/webster/prompts/02-*.md` reference in the digest-carry-forward confirmation to `.lyx/webster/prompts/02-*.md`, leaving the `_lyx/webster/summary.md` and `_lyx/webster/outcome.yaml` references alone — those are durable and unmoved.
  Follow the repo's semantic-line-break markdown rule in every file.
- **Commit:** `docs: retire the cross-module-exclusions mechanism`

## Batch Tests

`verify: go test ./internal/fabricengine/... && go test -tags integration ./internal/fabricengine/...` — one package, with the tagged run required because card 34 edits `internal/fabricengine/weftgit_exclude_test.go` (`//go:build integration`) and because the pollution-reproduction test is a real paired-fixture git test that only exists under that tag.

Covered files: `internal/fabricengine/weftgit_exclude_test.go`, plus the rest of the package's untagged and tagged suites re-run as regression cover — in particular `weftgit_pathspec_integration_test.go`, `commit_integration_test.go`, `commit_partial_integration_test.go` and `syncweft_integration_test.go`, which all commit through `commitWeftLocked` and would surface an accidental change to the surviving two exclude entries.

The rewritten pollution test is the one that matters: deleting it outright would remove the only end-to-end evidence that a plain `lyx fabric sync` cannot sweep another module's runtime noise into weft history.
Re-pointing it at `.lyx` proves the same property now holds because the files are not there to be swept, which is the whole point of the relocation.

`tools/sandbox/*SUITE.md` is operator-driven black-box documentation with no runnable assertion, so card 35's suite edits are verified only by `cmd/lyx/sandbox_coverage_test.go`'s `Covers:`-line parse, which the untagged `go test ./cmd/lyx/...` run in batch 5 and the repo-wide done gate both exercise;
this batch's own `verify:` does not cover it, which is why card 35 is explicit about leaving the `Covers:` and `Verdict:` lines untouched.
