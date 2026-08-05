# Batch: test-sweep

```yaml
task: 'fabric: shrink hubgeometry to the minimal illusion primitive (slice 7)'
batch: test-sweep
number: 4
cards: 7
verify: go vet -tags "integration smoke scout" ./... && go test ./... && go test -tags integration ./...
depends-on: [3]
```


## Batch Scope

Seven cards: the five test-file family sweeps, the leaf/seam invariant updates, and the batch-1 slice of the enforcement-guard rewrite. This is the batch that restores a green tree — it is the first point in the plan where `go test ./...` can pass, and its `verify` is therefore the full suite.

Roughly 60 of the ~100 test files here build a synthetic `Layout` struct literal. Each becomes a `Location` literal supplying `HubPath`/`WorktreeName`/`AnchorRel` in place of `Hub`/`WorktreeRoot`/`RelPath`, with `Cwd` dropped. The one case needing judgement rather than substitution: a literal that set `Cwd` to a value different from `WorktreeRoot` was exercising a subdirectory invocation, which the strict gate now rejects — the test either sets `AnchorRel` to the intended subpath or asserts `ErrCwdOutsideAnchor`, decided per test from what it is actually checking, and **never** by loosening the gate.

The two non-sweep cards close the batch: card 18 renames the module across the six leaf and seam invariants and retitles the Hub Geometry Invariant to the Cwd Resolution Invariant, and card 19 switches the guard's two allowlisted directory literals and seeds the per-token ownership map with the two rows batches 1-2 earned.

External interface batch 5 consumes: a green tree, `configengine.LyxDirName` as the single declarer of `_lyx`, and the ownership map's staged shape.

## Cards

### Card 13: test sweep — fabric

- **Context:**
  - `internal/lyxcwd/lyxcwd.go`
  - `internal/lyxtest/lyxtest.go`
- **Edits:**
  - `internal/fabriccli/cli_test.go`
  - `internal/fabricengine/add_branch_exists_test.go`
  - `internal/fabricengine/add_rollback_adopt_test.go`
  - `internal/fabricengine/add_test.go`
  - `internal/fabricengine/branchname_test.go`
  - `internal/fabricengine/checkout_index_refresh_test.go`
  - `internal/fabricengine/checkout_rollback_test.go`
  - `internal/fabricengine/clone_adopt_test.go`
  - `internal/fabricengine/commit_integration_test.go`
  - `internal/fabricengine/config_driven_junctions_integration_test.go`
  - `internal/fabricengine/config_test.go`
  - `internal/fabricengine/hook_test.go`
  - `internal/fabricengine/junction_pattern_integration_test.go`
  - `internal/fabricengine/junction_repoint_test.go`
  - `internal/fabricengine/junction_test.go`
  - `internal/fabricengine/junctionnames_test.go`
  - `internal/fabricengine/pull_integration_test.go`
  - `internal/fabricengine/reconcile_stale_registration_test.go`
  - `internal/fabricengine/reconcile_stale_removal_test.go`
  - `internal/fabricengine/remove_junctions_integration_test.go`
  - `internal/fabricengine/unwire_test.go`
  - `internal/fabricengine/warpforward_integration_test.go`
  - `internal/fabricengine/weftgit_exclude_test.go`
  - `internal/fabricengine/weftwiring_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Same mechanical substitution as cards 8-12, applied to test files. Each synthetic `hubgeometry.Layout` struct literal becomes a `lyxcwd.Location` literal supplying `HubPath`/`WorktreeName`/`AnchorRel` in place of `Hub`/`WorktreeRoot`/`RelPath`, with `Cwd` dropped. A literal that set only `WorktreeRoot` becomes `HubPath: filepath.Dir(<old value>), WorktreeName: filepath.Base(<old value>)`. A literal that set `Cwd` to a value different from `WorktreeRoot` was exercising a subdirectory invocation; under the strict gate that case is now `ErrCwdOutsideAnchor`, so the test either sets `AnchorRel` to the intended subpath or asserts the error — decide per test from what it is actually checking, and never by loosening the gate. `fabricengine`'s 23 test files carry the largest share of synthetic literals and most set `WorktreeRoot` and `Hub` together, which map cleanly onto `HubPath`/`WorktreeName`. 19 of the 25 in-package test files reach unexported identifiers, so none may be converted to `package fabricengine_test` to dodge a problem — if a substitution seems to need that, it is the substitution that is wrong.
- **Commit:** `test(fabric): point fabricengine and fabriccli tests at lyxcwd.Location`

### Card 14: test sweep — webster

- **Context:**
  - `internal/lyxcwd/lyxcwd.go`
  - `internal/lyxtest/lyxtest.go`
- **Edits:**
  - `internal/webstercli/cli_test.go`
  - `internal/webstercli/smoke_test.go`
  - `internal/webstercli/verbs_test.go`
  - `internal/webstercli/weft_integration_test.go`
  - `internal/websterengine/audit_test.go`
  - `internal/websterengine/beginbatch_test.go`
  - `internal/websterengine/config_test.go`
  - `internal/websterengine/recordbatch_test.go`
  - `internal/websterengine/recoverbatch_test.go`
  - `internal/websterengine/runlevel_test.go`
  - `internal/websterengine/template_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Same mechanical substitution as cards 8-12, applied to test files. Each synthetic `hubgeometry.Layout` struct literal becomes a `lyxcwd.Location` literal supplying `HubPath`/`WorktreeName`/`AnchorRel` in place of `Hub`/`WorktreeRoot`/`RelPath`, with `Cwd` dropped. A literal that set only `WorktreeRoot` becomes `HubPath: filepath.Dir(<old value>), WorktreeName: filepath.Base(<old value>)`. A literal that set `Cwd` to a value different from `WorktreeRoot` was exercising a subdirectory invocation; under the strict gate that case is now `ErrCwdOutsideAnchor`, so the test either sets `AnchorRel` to the intended subpath or asserts the error — decide per test from what it is actually checking, and never by loosening the gate.
- **Commit:** `test(webster): point websterengine and webstercli tests at lyxcwd.Location`

### Card 15: test sweep — builder, burler, loom, treadle

- **Context:**
  - `internal/lyxcwd/lyxcwd.go`
  - `internal/lyxtest/lyxtest.go`
- **Edits:**
  - `internal/buildercli/pause_test.go`
  - `internal/buildercli/poll_test.go`
  - `internal/buildercli/run_test.go`
  - `internal/buildercli/smoke_test.go`
  - `internal/buildercli/spawnbatch_test.go`
  - `internal/buildercli/status_test.go`
  - `internal/buildercli/testdata_test.go`
  - `internal/buildercli/weft_integration_test.go`
  - `internal/buildercli/weft_test.go`
  - `internal/builderengine/spawn_test.go`
  - `internal/burlerengine/config_test.go`
  - `internal/burlerengine/engine_test.go`
  - `internal/burlerengine/smoke_cluster_test.go`
  - `internal/burlerengine/smoke_round_test.go`
  - `internal/loomengine/discussion_test.go`
  - `internal/loomengine/plan_test.go`
  - `internal/loomengine/preflight_integration_test.go`
  - `internal/lyxtest/lyxtest_test.go`
  - `internal/treadleengine/smoke_judge_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Same mechanical substitution as cards 8-12, applied to test files. Each synthetic `hubgeometry.Layout` struct literal becomes a `lyxcwd.Location` literal supplying `HubPath`/`WorktreeName`/`AnchorRel` in place of `Hub`/`WorktreeRoot`/`RelPath`, with `Cwd` dropped. A literal that set only `WorktreeRoot` becomes `HubPath: filepath.Dir(<old value>), WorktreeName: filepath.Base(<old value>)`. A literal that set `Cwd` to a value different from `WorktreeRoot` was exercising a subdirectory invocation; under the strict gate that case is now `ErrCwdOutsideAnchor`, so the test either sets `AnchorRel` to the intended subpath or asserts the error — decide per test from what it is actually checking, and never by loosening the gate. The fixture-derived reads in `burlerengine/smoke_cluster_test.go:129,133`, `burlerengine/smoke_round_test.go:298,302` and `treadleengine/smoke_judge_test.go:256,260` are `fixture.Layout.Cwd` and become `fixture.Layout.AnchorPath()`. `treadleengine` is swept here for that one file only; its seam invariant still forbids a direct `internal/lyxcwd` import in production code, and this is a test file, so the allowlist is unaffected.
- **Commit:** `test(builder,burler,loom): point tests at lyxcwd.Location`

### Card 16: test sweep — perch, scout, shuttle, reed

- **Context:**
  - `internal/lyxcwd/lyxcwd.go`
  - `internal/lyxtest/lyxtest.go`
- **Edits:**
  - `internal/perchcli/cli_integration_test.go`
  - `internal/perchcli/run_integration_test.go`
  - `internal/perchengine/config_test.go`
  - `internal/perchengine/run_test.go`
  - `internal/reedcli/cli_integration_test.go`
  - `internal/reedcli/smoke_lifecycle_test.go`
  - `internal/reedengine/config_test.go`
  - `internal/reedengine/contract_integration_test.go`
  - `internal/reedengine/header_test.go`
  - `internal/reedengine/lock_test.go`
  - `internal/reedengine/mouse_boot_integration_test.go`
  - `internal/scoutengine/ensureserver_integration_test.go`
  - `internal/scoutengine/ensureserver_test.go`
  - `internal/scoutengine/leaf_enforcement_test.go`
  - `internal/scoutengine/load_test.go`
  - `internal/scoutengine/refs_integration_test.go`
  - `internal/scoutengine/supervised_integration_test.go`
  - `internal/scoutengine/supervised_scout_test.go`
  - `internal/scoutengine/supervised_test.go`
  - `internal/shuttlecli/cli_test.go`
  - `internal/shuttlecli/smoke_interrupt_test.go`
  - `internal/shuttleengine/config_test.go`
  - `internal/shuttleengine/run_inject_test.go`
  - `internal/shuttleengine/run_test.go`
  - `internal/shuttleengine/rundir_test.go`
  - `internal/shuttleengine/wait_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Same mechanical substitution as cards 8-12, applied to test files. Each synthetic `hubgeometry.Layout` struct literal becomes a `lyxcwd.Location` literal supplying `HubPath`/`WorktreeName`/`AnchorRel` in place of `Hub`/`WorktreeRoot`/`RelPath`, with `Cwd` dropped. A literal that set only `WorktreeRoot` becomes `HubPath: filepath.Dir(<old value>), WorktreeName: filepath.Base(<old value>)`. A literal that set `Cwd` to a value different from `WorktreeRoot` was exercising a subdirectory invocation; under the strict gate that case is now `ErrCwdOutsideAnchor`, so the test either sets `AnchorRel` to the intended subpath or asserts the error — decide per test from what it is actually checking, and never by loosening the gate.
- **Commit:** `test(perch,scout,shuttle,reed): point tests at lyxcwd.Location`

### Card 17: test sweep — config, board, ide and the leaf libraries

- **Context:**
  - `internal/lyxcwd/lyxcwd.go`
  - `internal/lyxtest/lyxtest.go`
- **Edits:**
  - `cmd/lyx/exitcode_test.go`
  - `cmd/lyx/main_integration_test.go`
  - `internal/boardcli/cli_test.go`
  - `internal/boardcli/notes_test.go`
  - `internal/boardengine/boardtest/bench_test.go`
  - `internal/boardengine/config_test.go`
  - `internal/configcli/configcli_integration_test.go`
  - `internal/configcli/configcli_test.go`
  - `internal/configcli/reconcile_integration_test.go`
  - `internal/configengine/config_test.go`
  - `internal/configengine/edit_test.go`
  - `internal/configengine/set_test.go`
  - `internal/configsync/configsync_test.go`
  - `internal/ideengine/menu_test.go`
  - `internal/ideengine/spawn_test.go`
  - `internal/modelspec/leaf_enforcement_test.go`
  - `internal/modelspec/load_test.go`
  - `internal/modelspec/template_test.go`
  - `internal/pattern/leaf_enforcement_test.go`
  - `internal/pattern/pattern_test.go`
  - `internal/tokenvocab/leaf_enforcement_test.go`
  - `internal/tokenvocab/tokenvocab_test.go`
  - `internal/vscode/color_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Same mechanical substitution as cards 8-12, applied to test files. Each synthetic `hubgeometry.Layout` struct literal becomes a `lyxcwd.Location` literal supplying `HubPath`/`WorktreeName`/`AnchorRel` in place of `Hub`/`WorktreeRoot`/`RelPath`, with `Cwd` dropped. A literal that set only `WorktreeRoot` becomes `HubPath: filepath.Dir(<old value>), WorktreeName: filepath.Base(<old value>)`. A literal that set `Cwd` to a value different from `WorktreeRoot` was exercising a subdirectory invocation; under the strict gate that case is now `ErrCwdOutsideAnchor`, so the test either sets `AnchorRel` to the intended subpath or asserts the error — decide per test from what it is actually checking, and never by loosening the gate. `cmd/lyx/main_integration_test.go` and `exitcode_test.go` exercise the CLI end to end from a fixture; a subdirectory invocation there is now `ErrCwdOutsideAnchor` and the assertion must reflect that rather than be relaxed.
- **Commit:** `test(config,board,ide): point tests at lyxcwd.Location`

### Card 18: update the leaf and seam invariants

- **Context:**
  - `internal/lyxcwd/lyxcwd.go`
  - `docs/overview.md`
  - `manifest/designs/fabric-unified-view.md`
- **Edits:**
  - `CONSTRAINTS.md`
  - `internal/modelspec/leaf_enforcement_test.go`
  - `internal/scoutengine/leaf_enforcement_test.go`
  - `internal/tokenvocab/leaf_enforcement_test.go`
  - `internal/pattern/leaf_enforcement_test.go`
  - `internal/lyxtest/leaf_enforcement_test.go`
  - `internal/treadleengine/seam_enforcement_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Rename `internal/hubgeometry` to `internal/lyxcwd` in the allowlist maps of `modelspec`, `scoutengine`, `tokenvocab` and `pattern`, and in every doc comment naming the package across all six enforcement files. A stale package name in a leaf allowlist silently stops enforcing, and an over-wide allowlist stops enforcing just as quietly, so neither is cosmetic. `modelspec` and `scoutengine` also carry `internal/configengine` from card 2 — verify, do not duplicate. `tokenvocab` and `pattern` are **not** widened: `tokenvocab` holds only a `*Location` and reads `RepoName`, `pattern` keeps only worktree-side constructors, so `internal/lyxcwd` alone remains each one's correct non-stdlib entry. **Correction to the discussion's `leaf-invariant-updates` decision**, which called for widening `lyxtest`'s allowlist: `lyxtest/leaf_enforcement_test.go` enforces a **banned-imports list**, not an allowlist, and `weftname`/`configengine`/`lyxcwd` are not on it — so the test needs only its doc-comment wording updated, and adding those imports requires no code change there. In `CONSTRAINTS.md`, retitle the **Hub Geometry Invariant** to the **Cwd Resolution Invariant** and rewrite it to the narrow post-shrink contract: `internal/lyxcwd` owns cwd resolution and nothing else; `Resolve` exposes only `RepoName`/`HubPath`/`WorktreeName`/`AnchorRel` and the two derived accessors, never a weft path, a junction path or any per-module subdirectory; cwd must equal `AnchorPath()` exactly, with `ErrCwdOutsideAnchor` otherwise; `ResolveWithAnchor` and `ResolveWorktree` are ungated and the first is a documented bypass; the module's imports are capped at stdlib plus `internal/gitexec`, which is what keeps `fabricengine` → `logger` → `lyxcwd` acyclic; and a module's own durable subdirectory is that module's private relative-path constant joined onto `AnchorPath()` — replacing the current line-12 wording that says `cwd`, which must land in step with `manifest/designs/fabric-unified-view.md` in batch 8 so the two never disagree. Update the `internal/hubgeometry` name in the other five invariants. Leave the enforcement pointer at line 18 naming `enforcement_test.go`; card 19 moves the file path.
- **Commit:** `docs(constraints): retitle Hub Geometry to the Cwd Resolution Invariant`

### Card 19: batch-1 slice of the enforcement-guard rewrite

- **Context:**
  - `internal/weftname/weftname.go`
  - `internal/configengine/config.go`
  - `CONSTRAINTS.md`
- **Edits:**
  - `internal/lyxcwd/enforcement_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Switch both allowlisted directory literals — `enforcement_test.go:138` (`TestEnforcement`'s `os.Getwd`/`--show-toplevel` ban) and `:420` (`TestEnforcement_GeometryLiterals`) — from `internal/hubgeometry` to `internal/lyxcwd`, along with every doc comment and failure message naming the old directory. The package rename alone would fail this test from card 5 onward, which is why the guard is rewritten incrementally rather than once in batch 8. Then replace the single-package allowlist in `TestEnforcement_GeometryLiterals` with a per-token ownership map keyed by token value, seeded with exactly the rows this batch earns: `-weft` owned by `internal/weftname`, and `_lyx` owned by `internal/configengine` **and**, transitionally, `internal/lyxcwd` for the private `lyxDirName` const card 2 left behind. Every other token (`_board`, `-HUB`, `_portals`, `_launchers`, `_raddle`, `_pattern`) keeps `internal/lyxcwd` as its owner for now; batch 6 moves those rows and batch 8 removes the transitional `_lyx` co-owner. A per-token map is strictly stronger than a blanket allowlist — it encodes *who* owns each token rather than "one package owns all of them", and it is what proves each batch moved ownership rather than copying code. Keep the existing `predicate` sub-test shape (synthetic positive/negative Go snippets parsed with `go/parser`, whole-token matching by exact equality after `strconv.Unquote`, so `_boardroom` and `-weft-bare` stay negatives) and keep the `scanned_non_empty` sanity sub-test — a misconfigured walk must not produce a vacuous pass. `.lyx` stays unpoliced this slice, as it is today; slice 9 is where it gets an owner, and adding it now would have to be undone one slice later.
- **Commit:** `test(lyxcwd): stage the geometry guard onto a per-token ownership map`

## Batch Tests

`verify` runs the repo-wide tagged type-check plus the full untagged suite (`go test ./...`), unbounded on purpose: this is the batch that has to prove the whole rename landed, so any narrower scope would leave exactly the regressions it exists to catch. It is also the first execution of the `gate_test.go` table created in batch 2 and of the `weftname` round-trips from batch 1.

Two assertions in this batch pin decisions rather than mechanics. `cmd/lyx/main_integration_test.go` and `exitcode_test.go` drive the CLI end to end from a fixture, so a subdirectory invocation there now returns `ErrCwdOutsideAnchor` — the assertion must be updated to expect the error, and relaxing the gate to keep the old assertion green would discard the whole point of the strict-equality change. And `TestEnforcement_GeometryLiterals` must pass with the staged map: `-weft` owned by `internal/weftname` and `_lyx` owned by `internal/configengine` plus the transitional `internal/lyxcwd` entry, with every other token still on `internal/lyxcwd`. A token that batches 1-2 copied rather than moved fails that guard loudly, which is the property the staging exists to give.
