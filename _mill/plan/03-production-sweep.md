# Batch: production-sweep

```yaml
task: 'fabric: shrink hubgeometry to the minimal illusion primitive (slice 7)'
batch: production-sweep
number: 3
cards: 5
verify: go build ./...
depends-on: [2]
```


## Batch Scope

Five cards, one per package family, cutting every **production** consumer over to `lyxcwd.Location`: fabric, webster, builder/burler/loom, config/board/ide plus the leaf libraries, and the perch/scout/shuttle/reed runtime engines. Nothing here changes behaviour beyond the field-source swap — `.Cwd` becomes `.AnchorPath()`, `.WorktreeRoot` becomes `.WorktreePath()`, `.Hub` becomes `.HubPath`, `.RelPath` becomes `.AnchorRel`, `.Repo` becomes `.RepoName`.

The split is by package family rather than by file count, so each card's diff is one subsystem's cutover and each is one commit. No card spans more than one family; the largest is 27 files.

Batch-local decision — `verify` is **`go build ./...`**, not the full suite. After this batch every production file compiles, but test files still name `hubgeometry`, so `go test` cannot pass until batch 4. `go build ./...` is the strongest statement that is actually true at this point and it is a real gate: it proves all ~85 production cutovers type-check together, which is precisely what this batch delivers.

## Cards

### Card 8: production sweep — fabric

- **Context:**
  - `internal/lyxcwd/lyxcwd.go`
  - `internal/lyxcwd/anchor.go`
- **Edits:**
  - `internal/fabriccli/clone.go`
  - `internal/fabriccli/fabric.go`
  - `internal/fabriccli/unwire.go`
  - `internal/fabriccli/weft_verbs.go`
  - `internal/fabricengine/add.go`
  - `internal/fabricengine/branchname.go`
  - `internal/fabricengine/checkout.go`
  - `internal/fabricengine/cleanup.go`
  - `internal/fabricengine/clone.go`
  - `internal/fabricengine/commit.go`
  - `internal/fabricengine/drift.go`
  - `internal/fabricengine/hook.go`
  - `internal/fabricengine/hostclean.go`
  - `internal/fabricengine/hostlayout.go`
  - `internal/fabricengine/junction.go`
  - `internal/fabricengine/junctionnames.go`
  - `internal/fabricengine/launchers.go`
  - `internal/fabricengine/list.go`
  - `internal/fabricengine/portals.go`
  - `internal/fabricengine/prune.go`
  - `internal/fabricengine/pull.go`
  - `internal/fabricengine/reconcile.go`
  - `internal/fabricengine/remove.go`
  - `internal/fabricengine/status.go`
  - `internal/fabricengine/unwire.go`
  - `internal/fabricengine/weftgit.go`
  - `internal/fabricengine/weftwiring.go`
  - `internal/fabricengine/worktreelist.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** In every listed file, swap the import `github.com/Knatte18/loomyard/internal/hubgeometry` for `github.com/Knatte18/loomyard/internal/lyxcwd`, the qualifier `hubgeometry.` for `lyxcwd.`, and the type `*hubgeometry.Layout` for `*lyxcwd.Location`. Rewrite field reads: `.Cwd` becomes `.AnchorPath()`, `.WorktreeRoot` becomes `.WorktreePath()`, `.Hub` becomes `.HubPath`, `.RelPath` becomes `.AnchorRel`, `.Repo` becomes `.RepoName`. The `.Cwd` sites are the ones to read carefully: nearly all pass a base directory into `configengine.Load`, which stats `<base>/_lyx`, so `AnchorPath()` is the directory they actually want — under the strict gate from card 6 the two are provably equal, and where they were not equal before (an invocation from a subdirectory) the old value was wrong. Change no behaviour beyond the field-source swap. Correct any godoc comment in these files that names `hubgeometry` or `Layout` in the same pass. `worktreelist.go` was moved into `fabricengine` in batch 1's card 3 but kept its `*hubgeometry.Layout` parameters, since the rename itself was still two batches away; it needs the identical field-source swap as every other file in this card. `fabricengine` is the heaviest consumer and the one where `.WorktreeRoot` dominates rather than `.Cwd`; those become `.WorktreePath()` with no semantic change at all. `fabriccli/weft_verbs.go:102` passes the raw unfiltered `cfg.Dirs()` into `ScopedPathspec(l.RelPath, …)` — that argument becomes `l.AnchorRel` and the raw/filtered asymmetry stays exactly as it is; it is load-bearing and batch 7 depends on it.
- **Commit:** `refactor(fabric): point fabricengine and fabriccli at lyxcwd.Location`

### Card 9: production sweep — webster

- **Context:**
  - `internal/lyxcwd/lyxcwd.go`
  - `internal/fabricengine/fabric.go`
- **Edits:**
  - `internal/webstercli/beginbatch.go`
  - `internal/webstercli/cli.go`
  - `internal/webstercli/recordbatch.go`
  - `internal/webstercli/recoverbatch.go`
  - `internal/webstercli/run.go`
  - `internal/webstercli/validate.go`
  - `internal/webstercli/weft.go`
  - `internal/websterengine/audit.go`
  - `internal/websterengine/beginbatch.go`
  - `internal/websterengine/recordbatch.go`
  - `internal/websterengine/recoverbatch.go`
  - `internal/websterengine/render.go`
  - `internal/websterengine/runlevel.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** In every listed file, swap the import `github.com/Knatte18/loomyard/internal/hubgeometry` for `github.com/Knatte18/loomyard/internal/lyxcwd`, the qualifier `hubgeometry.` for `lyxcwd.`, and the type `*hubgeometry.Layout` for `*lyxcwd.Location`. Rewrite field reads: `.Cwd` becomes `.AnchorPath()`, `.WorktreeRoot` becomes `.WorktreePath()`, `.Hub` becomes `.HubPath`, `.RelPath` becomes `.AnchorRel`, `.Repo` becomes `.RepoName`. The `.Cwd` sites are the ones to read carefully: nearly all pass a base directory into `configengine.Load`, which stats `<base>/_lyx`, so `AnchorPath()` is the directory they actually want — under the strict gate from card 6 the two are provably equal, and where they were not equal before (an invocation from a subdirectory) the old value was wrong. Change no behaviour beyond the field-source swap. Correct any godoc comment in these files that names `hubgeometry` or `Layout` in the same pass. `websterengine/runlevel.go` and `webstercli/cli.go` hold the bulk: a `Layout` field on the deps struct plus ~15 `.Cwd`/`.WorktreeRoot` reads each. The deps-struct field keeps its name and changes type.
- **Commit:** `refactor(webster): point websterengine and webstercli at lyxcwd.Location`

### Card 10: production sweep — builder, burler, loom

- **Context:**
  - `internal/lyxcwd/lyxcwd.go`
  - `internal/fabricengine/fabric.go`
- **Edits:**
  - `internal/buildercli/cli.go`
  - `internal/buildercli/poll.go`
  - `internal/buildercli/run.go`
  - `internal/buildercli/spawnbatch.go`
  - `internal/buildercli/validate.go`
  - `internal/buildercli/weft.go`
  - `internal/builderengine/spawn.go`
  - `internal/burlercli/cli.go`
  - `internal/burlerengine/config.go`
  - `internal/burlerengine/engine.go`
  - `internal/loomengine/discussion.go`
  - `internal/loomengine/plan.go`
  - `internal/loomengine/preflight.go`
  - `internal/planparser/parse.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** In every listed file, swap the import `github.com/Knatte18/loomyard/internal/hubgeometry` for `github.com/Knatte18/loomyard/internal/lyxcwd`, the qualifier `hubgeometry.` for `lyxcwd.`, and the type `*hubgeometry.Layout` for `*lyxcwd.Location`. Rewrite field reads: `.Cwd` becomes `.AnchorPath()`, `.WorktreeRoot` becomes `.WorktreePath()`, `.Hub` becomes `.HubPath`, `.RelPath` becomes `.AnchorRel`, `.Repo` becomes `.RepoName`. The `.Cwd` sites are the ones to read carefully: nearly all pass a base directory into `configengine.Load`, which stats `<base>/_lyx`, so `AnchorPath()` is the directory they actually want — under the strict gate from card 6 the two are provably equal, and where they were not equal before (an invocation from a subdirectory) the old value was wrong. Change no behaviour beyond the field-source swap. Correct any godoc comment in these files that names `hubgeometry` or `Layout` in the same pass. `lyxtest/lyxtest.go` is deliberately absent from this card: card 5 already retargeted it — batch 2's `go vet -tags integration ./internal/lyxcwd/...` type-checks it as a dependency of the module's own tests, so its qualifier-and-type swap could not wait for this batch. Verify it needs nothing further here rather than re-sweeping it.
- **Commit:** `refactor(builder,burler,loom): point callers at lyxcwd.Location`

### Card 11: production sweep — config, board, ide and the leaf libraries

- **Context:**
  - `internal/lyxcwd/lyxcwd.go`
  - `internal/configengine/config.go`
- **Edits:**
  - `internal/boardcli/cli.go`
  - `internal/configcli/configcli.go`
  - `internal/configcli/menu.go`
  - `internal/configengine/config.go`
  - `internal/configengine/edit.go`
  - `internal/configengine/set.go`
  - `internal/configsync/configsync.go`
  - `internal/envsource/envsource.go`
  - `internal/idecli/cli.go`
  - `internal/ideengine/menu.go`
  - `internal/ideengine/spawn.go`
  - `internal/logger/sink.go`
  - `internal/modelspec/load.go`
  - `internal/pattern/pattern.go`
  - `internal/tokenvocab/tokenvocab.go`
  - `internal/vscode/color.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** In every listed file, swap the import `github.com/Knatte18/loomyard/internal/hubgeometry` for `github.com/Knatte18/loomyard/internal/lyxcwd`, the qualifier `hubgeometry.` for `lyxcwd.`, and the type `*hubgeometry.Layout` for `*lyxcwd.Location`. Rewrite field reads: `.Cwd` becomes `.AnchorPath()`, `.WorktreeRoot` becomes `.WorktreePath()`, `.Hub` becomes `.HubPath`, `.RelPath` becomes `.AnchorRel`, `.Repo` becomes `.RepoName`. The `.Cwd` sites are the ones to read carefully: nearly all pass a base directory into `configengine.Load`, which stats `<base>/_lyx`, so `AnchorPath()` is the directory they actually want — under the strict gate from card 6 the two are provably equal, and where they were not equal before (an invocation from a subdirectory) the old value was wrong. Change no behaviour beyond the field-source swap. Correct any godoc comment in these files that names `hubgeometry` or `Layout` in the same pass. `tokenvocab/tokenvocab.go:25`'s `repo` token becomes `c.Layout.RepoName` — it is the single production consumer of that field, and the rendered token value changes only for a non-hub layout. `logger/sink.go:74,79` calls `Getwd` then `Resolve` to place its trace file; that call stays exactly where it is, because `fabricengine/coalesce.go:18` and `spawn.go:19` import `logger`, so moving resolution into `fabricengine` would produce `fabricengine → logger → fabricengine`. Keeping the module stdlib-plus-`gitexec` is what holds that cycle closed, and this card must not add an import that breaks it. Correct the godoc in `planparser/parse.go` that names the old package or type; `logger/logger.go:48,409` and `pattern/doc.go:13` carry the same class of stale godoc but are **not** this card's to touch — their corrections are owned by cards 26 and 25, the cards that relocate the constructors those comments describe.
- **Commit:** `refactor(config,board,ide): point callers at lyxcwd.Location`

### Card 12: production sweep — perch, scout, shuttle, reed

- **Context:**
  - `internal/lyxcwd/lyxcwd.go`
  - `internal/configengine/config.go`
- **Edits:**
  - `internal/perchcli/cli.go`
  - `internal/perchcli/run.go`
  - `internal/perchengine/doc.go`
  - `internal/perchengine/engine.go`
  - `internal/perchengine/identity.go`
  - `internal/reedcli/cli.go`
  - `internal/reedengine/lifecycle.go`
  - `internal/reedengine/lock.go`
  - `internal/reedengine/strand.go`
  - `internal/scoutcli/cli.go`
  - `internal/scoutengine/daemonstate.go`
  - `internal/scoutengine/doc.go`
  - `internal/scoutengine/ensureserver.go`
  - `internal/scoutengine/load.go`
  - `internal/shuttlecli/cli.go`
  - `internal/shuttleengine/run.go`
  - `internal/shuttleengine/rundir.go`
  - `internal/shuttleengine/wait.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** In every listed file, swap the import `github.com/Knatte18/loomyard/internal/hubgeometry` for `github.com/Knatte18/loomyard/internal/lyxcwd`, the qualifier `hubgeometry.` for `lyxcwd.`, and the type `*hubgeometry.Layout` for `*lyxcwd.Location`. Rewrite field reads: `.Cwd` becomes `.AnchorPath()`, `.WorktreeRoot` becomes `.WorktreePath()`, `.Hub` becomes `.HubPath`, `.RelPath` becomes `.AnchorRel`, `.Repo` becomes `.RepoName`. The `.Cwd` sites are the ones to read carefully: nearly all pass a base directory into `configengine.Load`, which stats `<base>/_lyx`, so `AnchorPath()` is the directory they actually want — under the strict gate from card 6 the two are provably equal, and where they were not equal before (an invocation from a subdirectory) the old value was wrong. Change no behaviour beyond the field-source swap. Correct any godoc comment in these files that names `hubgeometry` or `Layout` in the same pass. `scoutengine/ensureserver.go:300` builds a synthetic `&hubgeometry.Layout{WorktreeRoot: worktreeRoot}` purely to call `ScoutDaemonLock`; `Location` has no such field, so re-express it as `&lyxcwd.Location{HubPath: filepath.Dir(worktreeRoot), WorktreeName: filepath.Base(worktreeRoot)}`. It disappears entirely in batch 5, when `ScoutDaemonLock` relocates into `scoutengine` as a plain-path function needing no `Location` at all. Correct the godoc in `scoutengine/doc.go` at `:25` (the import-ceiling enumeration, which must name `internal/lyxcwd` and `internal/configengine` — the imports the package actually has after cards 2 and 12), `:209` and `:217`, and in `scoutengine/daemonstate.go:7` — both files are in this card's `Edits:` for exactly these comment corrections. `perchengine/doc.go` and `burlerengine/doc.go` were listed in an earlier draft as needing the same correction; planning-time inspection found no `hubgeometry`/`Layout` reference in either, so `perchengine/doc.go` stays in `Edits:` only for the mechanical sweep's verify-nothing-needed pass and `burlerengine/doc.go` is not this card's concern (card 27 edits it for `.lyx`). `reedengine/lifecycle.go` and `reedengine/strand.go` were missing from the original `Edits:` list despite reading `e.layout.Cwd`/`e.layout.WorktreeRoot` on the same `*Engine` whose `layout` field this card's `lock.go` retypes to `*lyxcwd.Location` — both need the identical field-source swap so `go build ./...` stays green; batch 5 edits both files again for the relocated-constructor work, which is unrelated to this fix.
- **Commit:** `refactor(perch,scout,shuttle,reed): point callers at lyxcwd.Location`


## Batch Tests

`verify: go build ./...` — the whole production tree compiles. This is not a weakened gate but the accurate one for this batch: test files are swept in batch 4, so `go test` is red by construction here, while `go build` failing would mean a genuine defect in one of the five cutovers.

No new tests are added. The `.Cwd` → `.AnchorPath()` substitution is the one place where the sweep is not purely mechanical, and it is covered by the existing suites at batch 4: nearly every `.Cwd` site passes a base directory into `configengine.Load`, which stats `<base>/_lyx`, so `AnchorPath()` is the directory those callers actually want. Under the strict gate from batch 2 the two values are provably equal; where they differed before — an invocation from a subdirectory — the old value was wrong, and the existing config-loading tests are what demonstrate the new one is right.
