# Batch: gitkit leaf

```yaml
task: 'lyxtest: build real fabric hubs, invert the lyxtest/fabric dependency'
batch: 'gitkit leaf'
number: 1
cards: 6
verify: go vet -tags integration ./... && go vet -tags smoke ./... && go vet -tags scout ./... && go test -tags integration ./internal/gitkit/... ./internal/lyxcwd/... ./cmd/lyx/...
depends-on: []
```

## Rename mechanic

For each `Moves:` pair the implementer MUST:

1. Run `git mv <old> <new>` FIRST, before making any other change to the moved file.
2. Make ONLY surgical edits — touch only the lines that must change after the move (package declaration, imports, identifier retargeting, message-prefix strings).
3. Use a full-file `Creates:` entry only for genuinely new files that have no predecessor.
4. Never write the relocated file from scratch and delete the original — that breaks git rename history and inflates review diffs.

## Batch Scope

This batch turns `internal/lyxtest` into `internal/gitkit`, the below-fabric leaf, without deleting any helper yet.
It is a pure rename plus one narrowing addition: `CopyRepo`/`RepoFixture` is introduced as the primitive repo fixture pinned to `internal/lyxcwd`, `internal/lyxcwd`'s nine `CopyWarpHub` sites migrate onto it, and a machine guard pins `CopyRepo`'s caller set immediately.
`CopyWarpHub`, `CopyPaired`, `CopyPairedLocal` and `CopyWeft` stay alive as-is so the tree keeps compiling while later batches migrate their 132 call sites;
batch 11 deletes them.

The external interface later batches consume is `gitkit.MustRun`, `gitkit.SeedConfig`, `gitkit.HermeticGitEnv`, `gitkit.GitStatusPorcelain` (added in batch 2) and `gitkit.CopyRepo`.

Batch-local decision: the repo-wide rename is done with one bare-word `perl -pi -e` sweep across 125 files rather than by hand, then proved by `go vet -tags integration ./...` and by batch 11 card 69's zero-hit grep gate.
The sweep matches the bare word `lyxtest`, not just `internal/lyxtest` and `lyxtest.`, because comment prose uses possessive and bare-word forms that a qualifier-only sweep silently leaves behind — including in four production files and in seven test files no other card in this plan touches.
It deliberately also rewrites the guard-token string literals in `cmd/lyx/tierpurity_test.go` and `cmd/lyx/hermeticenv_test.go`, which is the wanted outcome — those tokens must track the package name.

## Cards

### Card 1: Move internal/lyxtest to internal/gitkit

- **Context:**
  - `CONSTRAINTS.md`
- **Edits:** none
- **Creates:** none
- **Deletes:** none
- **Moves:**
  - `internal/lyxtest/lyxtest.go` -> `internal/gitkit/gitkit.go`
  - `internal/lyxtest/hermetic.go` -> `internal/gitkit/hermetic.go`
  - `internal/lyxtest/reexecguard.go` -> `internal/gitkit/reexecguard.go`
  - `internal/lyxtest/doc.go` -> `internal/gitkit/doc.go`
  - `internal/lyxtest/lyxtest_test.go` -> `internal/gitkit/gitkit_test.go`
  - `internal/lyxtest/reexecguard_test.go` -> `internal/gitkit/reexecguard_test.go`
  - `internal/lyxtest/leaf_enforcement_test.go` -> `internal/gitkit/leaf_enforcement_test.go`
  - `internal/lyxtest/bench_test.go` -> `internal/gitkit/bench_test.go`
- **Requirements:**
  `git mv` each pair first, then change `package lyxtest` to `package gitkit` in every moved file.
  Rewrite the internal name strings that carry the old package name: `os.MkdirTemp`'s pattern `"lyxtest-gitconfig-*"` in `HermeticGitEnv` becomes `"gitkit-gitconfig-*"`, `"lyxtest-warphub-*"` in `buildWarpHub` becomes `"gitkit-repo-*"`, `"lyxtest-weftprime-*"` in `buildWeftPrime` becomes `"gitkit-weftprime-*"`, `"lyxtest-weftonly-*"` in `buildWeftOnly` becomes `"gitkit-weftonly-*"`, and `refuseCLIReexec`'s `fmt.Fprintf` message prefix `"lyxtest: this test binary…"` becomes `"gitkit: this test binary…"`.
  Update the leading file-header comment of each moved file so it names `gitkit`, and update `TestLeafInvariant_AllowlistOnly`'s `lyxtestDir` local variable and its failure message to name `gitkit`.
  No behavior change and no signature change in this card — the four `Copy*` helpers, `MustRun`, `SeedConfig`, `HermeticGitEnv` and the three `sync.Once` template builders all survive unchanged apart from the string edits named above.
  After this card `internal/lyxtest/` is an empty directory and the repo does not compile;
  card 2 restores it.
- **Commit:** `refactor(gitkit): move internal/lyxtest to internal/gitkit`

### Card 2: Repo-wide import-path and qualifier rename

- **Context:**
  - `internal/gitkit/gitkit.go`
  - `internal/gitkit/hermetic.go`
- **Edits:**
  - `cmd/lyx/boardguard_test.go`
  - `cmd/lyx/hermeticenv_test.go`
  - `cmd/lyx/testmain_test.go`
  - `cmd/lyx/tierpurity_test.go`
  - `cmd/lyx/tiersleep_test.go`
  - `internal/batcher/config_test.go`
  - `internal/boardcli/testmain_test.go`
  - `internal/boardengine/boardtest/sync_test.go`
  - `internal/boardengine/boardtest/testmain_test.go`
  - `internal/boardengine/sync_integration_test.go`
  - `internal/boardengine/testmain_test.go`
  - `internal/burlerengine/smoke_cluster_test.go`
  - `internal/burlerengine/smoke_round_test.go`
  - `internal/burlerengine/testmain_test.go`
  - `internal/configcli/configcli_integration_test.go`
  - `internal/configcli/testmain_test.go`
  - `internal/fabriccli/cli_test.go`
  - `internal/fabriccli/pushbypass_integration_test.go`
  - `internal/fabriccli/testmain_test.go`
  - `internal/fabricengine/add_branch_exists_test.go`
  - `internal/fabricengine/add_rollback_adopt_test.go`
  - `internal/fabricengine/bolt_integration_test.go`
  - `internal/fabricengine/checkout_index_refresh_test.go`
  - `internal/fabricengine/checkout_rollback_test.go`
  - `internal/fabricengine/cleanreason_integration_test.go`
  - `internal/fabricengine/cleanup_primary_integration_test.go`
  - `internal/fabricengine/clone_adopt_test.go`
  - `internal/fabricengine/clone_emptyweft_integration_test.go`
  - `internal/fabricengine/clone_test.go`
  - `internal/fabricengine/coalesce_integration_test.go`
  - `internal/fabricengine/commit_integration_test.go`
  - `internal/fabricengine/config_driven_junctions_integration_test.go`
  - `internal/fabricengine/destructivegaps_integration_test.go`
  - `internal/fabricengine/diff_integration_test.go`
  - `internal/fabricengine/doc.go`
  - `internal/fabricengine/dotlyxjunction_integration_test.go`
  - `internal/fabricengine/fabric_test.go`
  - `internal/fabricengine/fabrictest/hub.go`
  - `internal/fabricengine/fabrictest/testmain_test.go`
  - `internal/fabricengine/healthreason_integration_test.go`
  - `internal/fabricengine/hook_test.go`
  - `internal/fabricengine/index_integration_test.go`
  - `internal/fabricengine/junction_pattern_integration_test.go`
  - `internal/fabricengine/junction_repoint_test.go`
  - `internal/fabricengine/mutation_record_integration_test.go`
  - `internal/fabricengine/open_integration_test.go`
  - `internal/fabricengine/prune_dirty_integration_test.go`
  - `internal/fabricengine/prune_unowned_integration_test.go`
  - `internal/fabricengine/pull_integration_test.go`
  - `internal/fabricengine/ready_integration_test.go`
  - `internal/fabricengine/reconcile_stale_registration_test.go`
  - `internal/fabricengine/reconcile_stale_removal_test.go`
  - `internal/fabricengine/remove_guard_integration_test.go`
  - `internal/fabricengine/remove_junctions_integration_test.go`
  - `internal/fabricengine/snapshot_integration_test.go`
  - `internal/fabricengine/status_pollution_integration_test.go`
  - `internal/fabricengine/syncweft_integration_test.go`
  - `internal/fabricengine/testmain_test.go`
  - `internal/fabricengine/unwire_test.go`
  - `internal/fabricengine/warpbinding_clone_integration_test.go`
  - `internal/fabricengine/warpbinding_reconcile_integration_test.go`
  - `internal/fabricengine/warpforward_integration_test.go`
  - `internal/fabricengine/warplayout_test.go`
  - `internal/fabricengine/weftgit_exclude_test.go`
  - `internal/fabricengine/weftgit_pathspec_integration_test.go`
  - `internal/fabricengine/weftgit_unborn_warp_test.go`
  - `internal/fabricengine/worktreelist_test.go`
  - `internal/gitexec/testmain_test.go`
  - `internal/gitrepo/commitempty_integration_test.go`
  - `internal/gitrepo/fetch_integration_test.go`
  - `internal/gitrepo/gitrepo_test.go`
  - `internal/gitrepo/gogit_test.go`
  - `internal/gitrepo/keyvalidation_test.go`
  - `internal/gitrepo/parity_test.go`
  - `internal/gitrepo/push_test.go`
  - `internal/gitrepo/testmain_test.go`
  - `internal/gitrepo/worktree_test.go`
  - `internal/idecli/cli_test.go`
  - `internal/idecli/testmain_test.go`
  - `internal/ideengine/testmain_test.go`
  - `internal/loomengine/preflight_integration_test.go`
  - `internal/loomengine/testmain_test.go`
  - `internal/lyxcwd/anchor.go`
  - `internal/lyxcwd/anchor_test.go`
  - `internal/lyxcwd/enforcement_test.go`
  - `internal/lyxcwd/gate_test.go`
  - `internal/lyxcwd/lyxcwd.go`
  - `internal/lyxcwd/lyxcwd_test.go`
  - `internal/lyxcwd/testmain_test.go`
  - `internal/lyxdirs/doc.go`
  - `internal/perchcli/cli_test.go`
  - `internal/perchcli/cli_integration_test.go`
  - `internal/perchcli/run_test.go`
  - `internal/perchcli/run_integration_test.go`
  - `internal/perchcli/testmain_test.go`
  - `internal/perchengine/testmain_test.go`
  - `internal/reedcli/cli_integration_test.go`
  - `internal/reedcli/smoke_attach_test.go`
  - `internal/reedcli/smoke_debuglog_test.go`
  - `internal/reedcli/smoke_lifecycle_test.go`
  - `internal/reedcli/smoke_resume_test.go`
  - `internal/reedcli/smoke_teardown_test.go`
  - `internal/reedcli/smoke_test.go`
  - `internal/reedcli/testmain_test.go`
  - `internal/scoutengine/ensureserver_integration_test.go`
  - `internal/scoutengine/refs_integration_test.go`
  - `internal/scoutengine/toolchain_integration_test.go`
  - `internal/shuttlecli/smoke_guardrail_test.go`
  - `internal/shuttlecli/smoke_interrupt_test.go`
  - `internal/shuttlecli/smoke_run_test.go`
  - `internal/shuttlecli/testmain_test.go`
  - `internal/shuttleengine/seam_enforcement_test.go`
  - `internal/treadleengine/gate_lingering_test.go`
  - `internal/treadleengine/smoke_judge_test.go`
  - `internal/treadleengine/testmain_test.go`
  - `internal/weftname/weftname.go`
  - `internal/weftname/weftname_test.go`
  - `internal/webstercli/testmain_test.go`
  - `internal/webstercli/verbs_test.go`
  - `internal/websterengine/config_test.go`
  - `internal/websterengine/integration_test.go`
  - `internal/websterengine/recoverbatch_test.go`
  - `internal/websterengine/runlevel_test.go`
  - `internal/websterengine/testmain_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  Apply one mechanical sweep across exactly the files listed in `Edits:`, rewriting **every** occurrence of the bare word `lyxtest` to `gitkit` — the import path, the selector-qualified `lyxtest.` form, and bare-word or possessive prose in comments alike:

  ```
  grep -rl '\blyxtest\b' --include=*.go internal cmd | grep -v '^internal/gitkit/' | xargs perl -pi -e 's{\blyxtest\b}{gitkit}g'
  ```

  The bare-word form is deliberate and is what makes batch 11 card 69's zero-hit `grep -rn 'lyxtest' --include=*.go internal cmd` gate satisfiable.
  A narrower sweep matching only `internal/lyxtest` and `lyxtest.` would leave possessive and bare-word prose behind in files no other card owns — `internal/weftname/weftname.go` ("for `lyxtest`'s fixture builders"), `internal/lyxcwd/anchor.go` and `internal/lyxcwd/lyxcwd.go` ("`lyxtest` injects anchors into synthetic hubs"), `internal/batcher/config_test.go`, `internal/gitrepo/keyvalidation_test.go`, `internal/perchcli/cli_test.go`, `internal/perchcli/run_test.go` and others — and card 69 would then fail with no card owning the fix.
  All such files are in this card's `Edits:` list, including three production files (`internal/weftname/weftname.go`, `internal/lyxcwd/anchor.go`, `internal/lyxcwd/lyxcwd.go`) whose only change is comment prose.
  After the sweep, re-read the comments in those three production files: a bare-word substitution can produce a sentence that is now factually wrong rather than merely renamed — a comment saying `gitkit` builds synthetic hubs is no longer true once `hubforge` builds real ones — so correct the claim, not just the name, wherever the sentence describes fixture *shape* rather than fixture *ownership*.

  Then run `gofmt -l internal cmd` and fix any file it names.
  Rewriting comment prose and string literals is intended, not collateral: `cmd/lyx/tierpurity_test.go`'s banned token `"lyxtest.Copy"` must become `"gitkit.Copy"`, `cmd/lyx/hermeticenv_test.go`'s three tokens `"lyxtest.Copy"`, `"lyxtest.MustRun"`, `"lyxtest.SeedConfig"` must become their `gitkit.` forms, and `internal/lyxcwd/enforcement_test.go`'s two allowlist map keys `"internal/lyxtest"` must become `"internal/gitkit"`.
  After the sweep, read `internal/lyxcwd/enforcement_test.go` and confirm both allowlist maps (the production-file scan map and the narrower `weftname`-import map) now key on `internal/gitkit`;
  the `internal/fabricengine/fabrictest` entries in both maps are left untouched here and are handled in batch 2.
  `go vet -tags integration ./...` and `go vet -tags smoke ./...` must both be clean at the end of this card.
- **Commit:** `refactor(gitkit): retarget every lyxtest reference onto gitkit`

### Card 3: Add CopyRepo/RepoFixture and migrate lyxcwd onto it

- **Context:**
  - `internal/lyxcwd/lyxcwd.go`
- **Edits:**
  - `internal/gitkit/gitkit.go`
  - `internal/gitkit/gitkit_test.go`
  - `internal/lyxcwd/anchor_test.go`
  - `internal/lyxcwd/lyxcwd_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  In `internal/gitkit/gitkit.go` add `type RepoFixture struct { Repo string; Bare string }` and `func CopyRepo(tb testing.TB) RepoFixture`, carrying the exact body `CopyWarpHub` has today (copy the `buildWarpHub` template pair into `tb.TempDir()` as `repo`/`bare`, then `rewriteOriginURLInConfig`), and document that it hands out a plain git repo with a bare origin — never a hub — and that it is callable from `internal/lyxcwd` alone.
  Reduce `CopyWarpHub` to a thin deprecated wrapper over `CopyRepo` that maps `RepoFixture{Repo, Bare}` onto the existing `WarpFixture{Hub, Bare}`, with a doc comment saying it is scheduled for deletion once the hub-shaped call sites migrate to `internal/hubforge`;
  keep `WarpFixture` exported and unchanged so the twelve remaining `CopyWarpHub` sites keep compiling.
  Retarget the nine `gitkit.CopyWarpHub(` call sites in `internal/lyxcwd/anchor_test.go` (six) and `internal/lyxcwd/lyxcwd_test.go` (three) onto `gitkit.CopyRepo(`, renaming the `.Hub` field read on each returned fixture to `.Repo`.
  In `internal/gitkit/gitkit_test.go`, retarget the existing `CopyWarpHub` coverage onto `CopyRepo`/`RepoFixture` and keep one case that still exercises the `CopyWarpHub` wrapper so the deprecated shim is not silently untested.
- **Commit:** `feat(gitkit): add CopyRepo primitive fixture and migrate lyxcwd onto it`

### Card 4: Retarget the gitkit benchmark onto CopyRepo

- **Context:**
  - `internal/gitkit/gitkit.go`
  - `docs/benchmarks/fixture-copy.md`
- **Edits:**
  - `internal/gitkit/bench_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  Replace `BenchmarkCopyPaired`, `BenchmarkCopyPairedLocal`, `BenchmarkCopyPairedParallel` and `BenchmarkCopyPairedLocalParallel` with a single `BenchmarkCopyRepo` measuring the serial cost of `CopyRepo`, keeping the `//go:build integration` tag and the file-header note about `b.TempDir()` cleanup accumulation.
  Update the header's run line to `go test -tags integration -bench BenchmarkCopyRepo -run '^$' ./internal/gitkit`.
  The three fixture benchmarks this file loses are re-created, retargeted onto the real hub, as batch 3's own benchmark file in the `hubforge` package — say so in the header comment so the reproducing trail recorded in `docs/benchmarks/fixture-copy.md` stays followable.
- **Commit:** `test(gitkit): retarget the fixture benchmark onto CopyRepo`

### Card 5: Pin CopyRepo's caller set with a guard test

- **Context:**
  - `internal/gitkit/leaf_enforcement_test.go`
  - `internal/gitkit/gitkit.go`
  - `CONSTRAINTS.md`
- **Edits:** none
- **Creates:**
  - `internal/gitkit/callerset_enforcement_test.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  Add `TestCopyRepoCallerSet_LyxcwdOnly` in `package gitkit`, untagged (it spawns no git), modelled on `TestLeafInvariant_AllowlistOnly`'s `go/parser` walk.
  It resolves the repository root from `runtime.Caller(0)` by walking up from the `gitkit` source directory, then walks every `.go` file under `internal/` and `cmd/` excluding `internal/gitkit/` itself, parses each with `go/parser` in full (not `ImportsOnly`, since the call expression matters), and reports a failure for any file outside `internal/lyxcwd` containing a selector call `gitkit.CopyRepo(` — matched on the AST `*ast.SelectorExpr` with `X` naming the `gitkit` import and `Sel.Name == "CopyRepo"`, never on raw text, so this file's own doc comment cannot trip it.
  The failure message must name the offending file and say that every other package takes a real hub from `internal/hubforge`, citing the `gitkit` Leaf Invariant in `CONSTRAINTS.md`.
  This is the guard that catches a later migration leaving a hub-shaped site on the primitive fixture.
- **Commit:** `test(gitkit): pin CopyRepo's caller set to internal/lyxcwd`

### Card 6: Rewrite the gitkit package doc

- **Context:**
  - `CONSTRAINTS.md`
  - `internal/gitkit/gitkit.go`
  - `internal/gitkit/hermetic.go`
  - `internal/gitkit/callerset_enforcement_test.go`
- **Edits:**
  - `internal/gitkit/doc.go`
  - `internal/gitkit/leaf_enforcement_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  Rewrite `internal/gitkit/doc.go` as the `gitkit` package doc: `gitkit` is the below-fabric leaf holding git primitives only — `MustRun`, `SeedConfig`, `HermeticGitEnv`, `CopyRepo` — and it never imports fabric, because 23 packages call `HermeticGitEnv` from `TestMain` and eleven of them sit inside `internal/fabriccli`'s dependency set, so a fabric import here would stop their `TestMain` files compiling.
  State that `CopyRepo` is pinned to `internal/lyxcwd` by `TestCopyRepoCallerSet_LyxcwdOnly` while `MustRun`/`SeedConfig`/`HermeticGitEnv` are unpinned, that hub fixtures come from `internal/hubforge` instead, and that `CopyWarpHub`/`CopyPaired`/`CopyPairedLocal`/`CopyWeft` are transitional and deleted in this task's final batch.
  Keep the existing two-layer hermetic-git-environment section intact, renamed onto `gitkit`.
  In `internal/gitkit/leaf_enforcement_test.go`, update the file-header comment and the `t.Errorf` text to name the `gitkit` Leaf Invariant;
  the `allowedImports` map itself is already correct (`configengine`, `lyxcwd`, `lyxdirs`, `weftname`) and must not be widened.
- **Commit:** `docs(gitkit): rewrite the package doc for the leaf role`

## Batch Tests

`verify:` compile-checks the whole repo under both the `integration` and `smoke` tags (`go vet` type-checks test files, which is what catches the 116-file rename sweep going wrong), then runs the three suites this batch actually changes: `internal/gitkit` (`gitkit_test.go`, `reexecguard_test.go`, `leaf_enforcement_test.go`, `callerset_enforcement_test.go`), `internal/lyxcwd` (the nine migrated `CopyRepo` sites plus `enforcement_test.go`'s allowlist maps), and `cmd/lyx` (`tierpurity_test.go` and `hermeticenv_test.go`, whose banned-token string data this batch rewrites).

The `smoke`-tagged suites in `internal/reedcli`, `internal/shuttlecli`, `internal/burlerengine` and `internal/treadleengine` are compile-checked via `go vet -tags smoke ./...` and deliberately never run: they spawn live tmux sessions and LLM agents, which is not something a per-batch verify may do.
