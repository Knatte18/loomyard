# Batch: initcli-undo

```yaml
task: "Add lyx init --undo / deinit command"
batch: "initcli-undo"
number: 4
cards: 3
verify: go test -tags integration ./internal/initcli/... -count=1
depends-on: [1, 2, 3]
```

## Batch Scope

This batch wires the `--undo` flag onto the existing `lyx init` command and implements
the full reversal orchestration, using the three building blocks the prior batches
added: `weftengine.Commit`/`Push`/`EnvSyncOptions`/`DefaultCommitMessage` (batch 1),
`gitignore.Remove` (batch 2), and `warpengine.UnwireJunctions` (batch 3). It also
updates `docs/overview.md`'s existing **init** bullet in the same batch (not a
follow-up), per this repo's CLAUDE.md instruction that CLI-behaviour changes update
docs in the same commit.

No batch-local decisions differ from `## Shared Decisions` in the overview — in
particular, the "abort scope" and "Push runs unconditionally" decisions there are
load-bearing for Card 8's exact step ordering.

## Cards

### Card 8: Implement `lyx init --undo`

- **Context:**
  - `internal/configcli/configcli.go`
  - `internal/warpengine/junction.go`
  - `internal/gitignore/gitignore.go`
  - `internal/weftengine/sync.go`
  - `internal/weftengine/weft.go`
  - `internal/hubgeometry/hubgeometry.go`
- **Edits:**
  - `internal/initcli/initcli.go`
  - `internal/weftengine/sync.go`
  - `internal/weftengine/sync_test.go`
- **Creates:**
  - `internal/initcli/undo.go`
- **Deletes:** none
- **Moves:** none
- **Discovered-during-implementation fix:** `weftengine.Commit`'s `git add --
  <pathspec>` step errors with "pathspec ... did not match any files" (exit
  128) when the pathspec target has already been fully removed from both the
  working tree and the git index by a prior commit (e.g. a second `lyx init
  --undo` run after the first one already committed the `_lyx` deletion) —
  the exact case Card 8 step 4's "always call `weftengine.Commit`, regardless
  of whether `weftLyxDir` existed this invocation" requirement exercises.
  `Commit` now tolerates that specific git stderr message as a "nothing to
  stage" no-op (matching its existing `committed == false` contract) instead
  of surfacing it as a hard error, so a caller that unconditionally
  re-invokes `Commit` against a since-cleared pathspec stays idempotent.
  Covered by a new case in `internal/weftengine/sync_test.go`'s `TestCommit`.
- **Requirements:**
  - In `internal/initcli/initcli.go`'s `Command()`: register a bool flag
    `initCmd.Flags().Bool("undo", false, "reverse a previous init: remove the _lyx
    junction, weft-side content, and the .gitignore/.git-exclude entries it added")`
    (assign the `*cobra.Command` returned by the existing composite literal to a local
    variable first, mirroring `configcli.Command()`'s `configCmd` pattern, so the flag
    can be registered on it and read back in the `RunE` closure). Change the `RunE`
    assignment from `clihelp.WrapRun(runInit)` to a `clihelp.WrapRun(func(out
    io.Writer, args []string) int { ... })` closure that reads `undo, _ :=
    initCmd.Flags().GetBool("undo")` and dispatches to `runUndo(out, args)` when true,
    else `runInit(out, args)` — `runInit`'s own signature and body are unchanged.
  - Update the `init` command's `Long` text to document `--undo` with a concrete
    example invocation and a one-sentence description of what it reverses (junction,
    weft-side `_lyx` content, `.gitignore` block, `.git/info/exclude` entry), per the
    CLI/Cobra Invariant's "help accuracy is a review obligation" rule. Update `Short`
    from `"scaffold _lyx/config/ in the current directory"` to `"scaffold _lyx/config/
    in the current directory (or reverse it with --undo)"` — a small tweak for
    `--help` subcommand-listing discoverability, since the command becomes
    bidirectional. `Long` remains the authoritative, detailed documentation of
    `--undo`'s exact behavior.
  - In new file `internal/initcli/undo.go`, implement
    `func runUndo(out io.Writer, args []string) int`, following this exact step order
    (per the overview's "abort scope" and "Push runs unconditionally" Shared
    Decisions):
    1. Resolve `cwd` via `hubgeometry.Getwd()` and `l` via `hubgeometry.Resolve(cwd)`,
       with the identical error-handling shape `runInit` uses (wrapped message for the
       `Getwd` error, bare passthrough of `hubgeometry.Resolve`'s error). Unlike
       `runInit`, do **not** add a "no weft pairing" early-exit check — there is no
       equivalent gate for `--undo` (see the overview's "no separate pre-gate" Shared
       Decision).
    2. `slug := filepath.Base(l.WorktreeRoot)` (identical derivation to `runInit`).
    3. `result, err := warpengine.UnwireJunctions(l, slug)`. If `err != nil`: return
       `output.Err(out, err.Error())` **immediately** — do not run any of steps 4-5
       below. This is the full-abort behavior from the "any junction inconsistency is a
       hard error" Shared Decision.
    4. Weft-side content: first check whether a weft worktree exists at all via
       `os.Stat(l.WeftWorktree())`. On a stat error other than not-exist (`err != nil &&
       !os.IsNotExist(err)`), return `output.Err(out, err.Error())` immediately —
       mirror `runInit`'s own `os.Stat(lyxDir)` handling a few lines into that file for
       this same stat-error-vs-not-exist distinction. If it does **not** exist (a
       truly-unpaired host — the same condition `runInit` itself hard-gates on with "no
       weft pairing"), skip every remaining part of this step entirely: do not call
       `os.RemoveAll`, `weftengine.Commit`, or `weftengine.Push`. Track
       `weftContentStatus := "not_present"` and proceed straight to step 5. This guard
       is required because `weftengine.Commit`'s internal `ensureLockDir` calls
       `os.MkdirAll(weftPath + "/.weft", ...)` unconditionally — calling `Commit`
       against a nonexistent `l.WeftWorktree()` would silently create a stray
       `<slug>-weft/.weft/` directory tree on disk and then fail with a "not a git
       repository" error from the subsequent `git add`, breaking the "clean no-op on a
       never-paired host" case the "no separate pre-gate" Shared Decision requires.
       If the weft worktree *does* exist: `weftLyxDir := l.WeftLyxDirFor(slug)`. Check
       `os.Stat(weftLyxDir)`, applying the identical stat-error-vs-not-exist handling
       as above. If it exists, call `os.RemoveAll(weftLyxDir)` and check its returned
       error — on failure, return `output.Err(out, err.Error())` (matching every other
       `os.RemoveAll` call site in `warpengine`, e.g. `remove.go`, `prune.go`,
       `launchers.go`, which all surface the error rather than ignoring it) — then track
       `weftContentStatus := "cleared"`; if it does not exist, track
       `weftContentStatus := "not_present"` (do not call `os.RemoveAll` in this case).
       Then — **regardless of whether weftLyxDir existed this invocation** (but only
       once we already know the weft worktree itself exists) — always call
       `weftengine.Commit(l.WeftWorktree(), weftengine.ScopedPathspec(l.RelPath,
       []string{hubgeometry.LyxDirName}), "lyx init --undo: clear _lyx",
       weftengine.EnvSyncOptions())`; on error, return `output.Err(out, err.Error())`.
       Then, unconditionally (not gated on the `committed` bool `Commit` returned),
       call `weftengine.Push(l.WeftWorktree(), weftengine.EnvSyncOptions())`; on error,
       return `output.Err(out, err.Error())`. This unconditional-Push behavior is what
       recovers a prior partial run where the deletion committed locally but the push
       failed.
    5. `.gitignore` revert: `changed, err := gitignore.Remove(cwd, ".lyx/")`. On error,
       return `output.Err(out, err.Error())`. Track `gitignoreStatus := "reverted"` if
       `changed`, else `"unchanged"`.
    6. Emit success JSON via `output.Ok(out, map[string]any{...})` with these exact
       keys: `"lyx_junction"` (`"removed"` if `result.JunctionRemoved` else
       `"not_present"`), `"weft_content"` (`weftContentStatus` from step 4),
       `"git_exclude"` (`"reverted"` if `result.ExcludeChanged` else `"unchanged"`),
       `"gitignore"` (`gitignoreStatus` from step 5).
- **Commit:** `feat(initcli): add lyx init --undo`

### Card 9: Document `--undo` in `docs/overview.md`

- **Context:**
  - `internal/initcli/initcli.go`
- **Edits:**
  - `docs/overview.md`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  - Extend the existing **init** bullet (around line 212: "scaffolds the `_lyx/`
    directory structure and creates all module config files via reconciliation against
    templates... Idempotent: does not clobber existing config files. ✅ Implemented.")
    with a clause noting `--undo` reverses that scaffolding (junction, weft-side
    content, `.gitignore` block, `.git/info/exclude` entry) for test/sandbox cleanup.
    Keep it as a single bullet in the same terse module-table style as the surrounding
    entries — do not add a new top-level bullet or subsection.
- **Commit:** `docs(overview): document lyx init --undo`

### Card 10: Test `lyx init --undo`

- **Context:**
  - `internal/initcli/initcli.go`
  - `internal/initcli/undo.go`
  - `internal/initcli/initcli_test.go`
  - `internal/warpengine/junction.go`
  - `internal/gitignore/gitignore.go`
  - `internal/hubgeometry/hubgeometry.go`
  - `internal/lyxtest/lyxtest.go`
  - `internal/fslink/fslink.go`
- **Creates:**
  - `internal/initcli/undo_test.go`
- **Edits:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  - `//go:build integration` build tag, using `lyxtest.CopyPairedLocal(t)` per the
    existing `initcli_test.go` fixture pattern for every sub-test except
    `TestRunInit_Undo_PartialRecovery` part (b), which needs `lyxtest.CopyPaired(t)`
    (see that test's own entry below for why). Use `t.Setenv("WEFT_SKIP_PUSH", "1")`
    (and/or `WEFT_SKIP_GIT`) in tests that don't need to exercise the real commit/push
    path.
  - `TestRunInit_Undo_HappyPath`: run `initcli.RunInit(&buf, []string{})` then
    `initcli.RunInit(&buf2, []string{"--undo"})` (the same seam handles both the
    forward and `--undo` paths, dispatching on the flag); assert the JSON output has
    `ok: true` with `lyx_junction:
    "removed"`, `weft_content: "cleared"`, `git_exclude: "reverted"`, `gitignore:
    "reverted"`; assert on disk that the host junction is gone, the weft-side `_lyx`
    directory is gone, `git status --porcelain` in the weft worktree is clean (deletion
    was committed, and pushed unless `WEFT_SKIP_PUSH` was set), the `.gitignore`
    managed block is fully removed (not just emptied — assert the marker strings are
    entirely absent), and the `.git/info/exclude` line is gone.
  - `TestRunInit_Undo_NeverInitialized`: `lyxtest.CopyPairedLocal`'s weft-prime
    template always pre-seeds `_lyx/config/placeholder` (via
    `lyxtest.buildWeftPrime`) purely as fixture scaffolding for other tests —
    production `warpengine` spawn code (`Add`/`WireJunctions`) never creates this file,
    so it does not reflect a real never-initialized directory. Before calling `--undo`,
    remove that placeholder (and the now-empty `_lyx/config` and `_lyx` directories) at
    `filepath.Join(fixture.WeftPrime, "_lyx")` so the fixture genuinely represents "no
    weft-side content, no host init ever ran." Then run `--undo` with no prior `init`;
    assert `ok: true` with `lyx_junction: "not_present"`, `weft_content: "not_present"`,
    `git_exclude: "unchanged"`, `gitignore: "unchanged"`, and no error.
  - `TestRunInit_Undo_NoWeftPairing`: covers the truly-unpaired host case (no weft
    sibling worktree exists at all — not merely "never `init`'d" but "never `warp
    add`'d either"), mirroring `initcli_test.go`'s existing `TestRunInit_NoPairing`
    fixture: a bare git repo created directly via `gitexec.RunGit([]string{"init"},
    tmpDir)`, with no weft sibling. Run `--undo` (no prior `init` is possible here,
    since `init` itself refuses to run without a weft pairing); assert `ok: true`,
    `weft_content: "not_present"`, no error, and — critically — assert no
    `<slug>-weft` directory (nor a stray `.weft` lock dir under it) was created as a
    side effect of the `--undo` call. This is the exact regression the weft-worktree
    existence guard in Card 8 step 4 exists to prevent.
  - `TestRunInit_Undo_Idempotent`: run `--undo` twice in a row after a prior `init`;
    assert the second run matches `TestRunInit_Undo_NeverInitialized`'s expected
    output shape (clean no-op).
  - `TestRunInit_Undo_RealDirectoryGuard`: two-phase setup — (1) run `init`
    successfully first so the junction, gitignore block, exclude line, and weft-content
    all legitimately exist; (2) remove the junction (`fslink.Remove` or equivalent) and
    replace it with a real directory containing a file, simulating external corruption
    after the fact; (3) run `--undo`. Assert a non-zero exit code / JSON `error` field,
    and assert *everything* is left untouched — the real directory and its contents,
    the weft-side `_lyx` content, the `.gitignore` managed block, and the
    `.git/info/exclude` line all remain exactly as they were before the `--undo` call.
  - `TestRunInit_Undo_TargetMismatch`: after `init`, remove the junction and recreate
    it pointing at a different, unrelated directory; run `--undo`; assert a non-zero
    exit code / JSON `error` field, and assert the junction is untouched (still points
    at the wrong target) and the weft-side content, `.gitignore`, and
    `.git/info/exclude` are all untouched too (same full-abort assertions as the
    real-directory-guard test).
  - `TestRunInit_Undo_PartialRecovery`: part (a) uses `lyxtest.CopyPairedLocal(t)` like
    every other sub-test in this file: after `init`, manually remove the host junction
    only (simulating a crash between removing the junction and clearing weft content)
    and assert a subsequent `--undo` run finishes cleanly (no error) and clears the
    still-present weft-side content. Part (b) needs a real weft-bare remote to assert
    against, which `CopyPairedLocal` does not provide (its doc comment states
    `WeftBare` is left empty and that pushing against it is unsupported) — use
    `lyxtest.CopyPaired(t)` for this sub-test instead: after `init`, manually perform
    the weft-side deletion and commit (mirroring what step 4 of `runUndo` would do) but
    do not push, then run `--undo` again (without `WEFT_SKIP_PUSH` set, so the real
    push path executes) and assert it succeeds and the pending commit is now pushed —
    asserting the local weft repo's `HEAD` matches `WeftBare`'s `HEAD` after the second
    `--undo` call. This is the scenario the "Push runs unconditionally" Shared Decision
    exists to handle.
- **Commit:** `test(initcli): cover lyx init --undo`

## Batch Tests

`verify` runs `go test -tags integration ./internal/initcli/... -count=1` — both the
existing `initcli_test.go` and the new `undo_test.go` require `-tags integration` (real
git worktree fixtures via `lyxtest`). The overview's top-level `verify: go build ./...`
runs after this batch too, as the final cross-package compile confirmation for the
whole plan.
