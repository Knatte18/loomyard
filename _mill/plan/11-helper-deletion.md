# Batch: helper deletion

```yaml
task: 'lyxtest: build real fabric hubs, invert the lyxtest/fabric dependency'
batch: 'helper deletion'
number: 11
cards: 5
verify: go vet -tags integration ./... && go vet -tags smoke ./... && go test -tags integration ./internal/gitkit/... ./internal/lyxcwd/... ./cmd/lyx/... ./internal/fabricengine/... ./internal/fabriccli/...
depends-on: [10]
```

## Batch Scope

Every consumer is migrated, so this batch deletes what they no longer call: `CopyPaired`, `CopyPairedLocal`, `CopyWeft`, the transitional `CopyWarpHub` wrapper, the three fixture structs they returned, and the two `sync.Once` template builders (`buildWeftPrime`, `buildWeftOnly`) that existed only to feed them.
What survives in `internal/gitkit` afterwards is the leaf and nothing else: `MustRun`, `SeedConfig`, `GitStatusPorcelain`, `HermeticGitEnv`, `refuseCLIReexec`, and `CopyRepo` with its one-package caller set.

The grep gate in card 70 is this task's completeness proof, and card 69 is what makes it satisfiable — the per-call-site cards replace call expressions and leave comment prose naming the same retired helpers behind.
It is a card rather than a sentence in a commit message because "drift becomes impossible by construction" is the whole point of the task, and an unproved claim of it is worse than no claim.

Batch-local decision: the gate greps for the *fixture* usage of the pairing shim, not for the bare string `NewPairedForTest`.
Batch 8 card 51 renamed that shim to `NewPairedFromPathsForTest` and narrowed it to `internal/fabricengine/fabric_test.go`'s untagged unit test of the `newPaired` constructor, which is not a fixture and was never in this task's scope — see batch 8's `## Batch Scope` for the full reasoning behind that deviation.

## Cards

### Card 67: Delete the retired fixture helpers

- **Context:**
  - `internal/gitkit/callerset_enforcement_test.go`
  - `internal/gitkit/leaf_enforcement_test.go`
- **Edits:**
  - `internal/gitkit/gitkit.go`
  - `internal/lyxcwd/enforcement_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  Delete from `internal/gitkit/gitkit.go`: `CopyPaired`, `CopyPairedLocal`, `CopyWeft`, the transitional `CopyWarpHub` wrapper, the structs `WarpFixture`, `PairedFixture` and `WeftFixture`, and the template builders `buildWeftPrime` and `buildWeftOnly` together with their `sync.Once`/path package-level variable blocks.
  Keep `buildWarpHub` and its variable block — `CopyRepo` is its only remaining consumer — but rename it and its variables to say what they now build: `buildRepoTemplate`, `repoTemplateOnce`, `repoTemplatePath`, `repoTemplateBarePath`.
  Keep `MustRun`, `SeedConfig`, `GitStatusPorcelain`, `RepoFixture`, `CopyRepo`, and the private `initRepo`, `initBareRemote`, `commitAll`, `mustGit`, `stripHookSamples`, `copyDirRecursive`, `rewriteOriginURLInConfig`.
  Delete any Go import left unused by the deletions — `weftname` in particular, if `buildWeftPrime` and `buildWeftOnly` were its only consumers.
  Do **not** remove `internal/gitkit`'s entry from `internal/lyxcwd/enforcement_test.go`'s `weftnameImportOwners` map when that happens: the map is an allowlist of directories that *may* import `weftname`, not a record of which currently do, and `CONSTRAINTS.md`'s Fabric Vocabulary Invariant names `internal/gitkit` in that subset unconditionally.
  Removing the entry would put the code at odds with a landed invariant to no benefit.
  Do not widen `leaf_enforcement_test.go`'s `allowedImports` map to compensate for anything;
  the leaf's import set only ever shrinks here.
- **Commit:** `refactor(gitkit): delete the retired paired and weft fixtures`

### Card 68: Prune the retired helpers' tests

- **Context:**
  - `internal/gitkit/gitkit.go`
- **Edits:**
  - `internal/gitkit/gitkit_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  Delete from `internal/gitkit/gitkit_test.go` every test covering a helper card 67 removed — the `CopyPaired`, `CopyPairedLocal`, `CopyWeft` and `CopyWarpHub`-wrapper cases — and keep every test covering a survivor: `MustRun`, `SeedConfig`, `CopyRepo`, `rewriteOriginURLInConfig`, `copyDirRecursive`'s symlink refusal, and the template builder's quiet-config properties.
  A deleted test whose subject survives under a new name is retargeted, not dropped: `buildWarpHub`'s coverage becomes `buildRepoTemplate`'s.
  State in the commit message how many tests were deleted and that every one of them named a deleted helper — this is the one place in the task where deleting a test is correct, so it should be auditable.
- **Commit:** `test(gitkit): prune the retired fixtures' tests`

### Card 69: Sweep the retired helper names out of prose

- **Context:**
  - `internal/gitkit/gitkit.go`
  - `internal/hubforge/hub.go`
- **Edits:**
  - `internal/configcli/configcli_test.go`
  - `internal/loomengine/config_test.go`
  - `internal/fabricengine/export_test.go`
  - `internal/fabricengine/reconcile_stale_registration_test.go`
  - `internal/fabricengine/commit_lock_integration_test.go`
  - `internal/fabriccli/cli_test.go`
  - `internal/fabriccli/pushbypass_integration_test.go`
  - `internal/boardengine/boardtest/sync_test.go`
  - `internal/fabricengine/commit_integration_test.go`
  - `internal/fabricengine/bolt_integration_test.go`
  - `internal/fabricengine/hook_test.go`
  - `internal/fabricengine/index_integration_test.go`
  - `internal/fabricengine/pull_integration_test.go`
  - `internal/fabricengine/warplayout_test.go`
  - `internal/fabricengine/config_driven_junctions_integration_test.go`
  - `internal/fabricengine/weftgit_exclude_test.go`
  - `internal/fabricengine/warpbinding_reconcile_integration_test.go`
  - `internal/fabricengine/unwire_test.go`
  - `internal/perchcli/cli_test.go`
  - `internal/perchcli/run_test.go`
  - `internal/gitkit/bench_test.go`
  - `internal/hubforge/bench_test.go`
  - `cmd/lyx/boardguard_test.go`
  - `cmd/lyx/tierpurity_test.go`
  - `internal/gitkit/leaf_enforcement_test.go`
  - `internal/fabricengine/fabric_test.go`
- **Creates:**
  - `internal/fabricengine/gitsha_integration_test.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  The per-call-site migration cards in batches 4 through 10 replace **call expressions**;
  they never touch comment prose naming the same helpers, and there is roughly a hundred hits' worth of it — a bare-token grep for the four retired names returns 244 hits against the 141 call expressions the plan's own arithmetic counts.
  Every one of those surplus hits names an identifier card 67 has just deleted, and card 70's zero-hit gate fails on all of them.
  Sweep them here, after the deletions and before the gate:

  ```
  grep -rl 'CopyPaired\|CopyPairedLocal\|CopyWeft\|CopyWarpHub' --include=*.go internal cmd
  ```

  For each file the grep names, rewrite the prose to describe what the code now does.
  This is **not** a mechanical substitution and must not be done with `perl -pi -e`: the retired names have no one-to-one replacement — `CopyPaired`, `CopyPairedLocal` and `CopyWeft` all become `hubforge.NewHub`, `CopyWarpHub` becomes `hubforge.NewHub` at twelve sites and `gitkit.CopyRepo` at nine, and a sentence like "no `CopyWeft`, no `SeedConfig` — this is a `t.TempDir()` unit test" is making a claim about *tiering* that stays true only if the replacement names the right successor.
  Read each sentence and rewrite the claim.

  Two of the files carry no `lyxtest` token at all, so batch 1 card 2's sweep never selected them and no other card in this plan names them — they are in this card's `Edits:` list for exactly that reason:
  `internal/configcli/configcli_test.go` (its header contrasts itself with the "e2e test with real `fabriccli.RunCLI` over `CopyPaired`" in its integration sibling) and `internal/loomengine/config_test.go` (its header says "no `CopyWeft`, no `SeedConfig`" to explain why it is a Tier-1 test).
  Both are one-line header-comment fixes.
  Every other file the grep names was expected to be already covered by an earlier batch's own `Edits:` list — but the earlier batches' work only migrated call expressions, and the grep gate covers comment prose those cards were never scoped to touch.
  Re-running the grep after card 67's deletions surfaced 19 further files, all pure comment-prose hits (no further call expressions or type references, verified individually below) in files this task has already read and edited once for their call-site migration.
  They are added to this card's `Edits:` list rather than spawning one plan-edit commit per file, since the correction is uniform in kind (rewrite the sentence naming a deleted identifier) even though each sentence's replacement wording differs.

  Deviation found while implementing: `internal/fabricengine/export_test.go`'s `newCommitFixture` still held a live call expression, `gitkit.CopyWeft(t)`, not just prose — a call site an earlier batch missed rather than migrated.
  `newCommitFixture` is called from four in-package (`package fabricengine`) files, so it cannot take a `hubforge.NewHub` fixture (the Fabric-Fixture Invariant forbids that import there);
  it gets its own minimal in-package plain-weft-repo builder, `newPlainWeftRepo`, mirroring the existing `newPlainWarpRepo` sibling but tracking `_lyx/config.yaml` the way the deleted `buildWeftOnly` template did.
  Added to this card's `Edits:` list for that reason.

  A second deviation, same shape: `internal/fabricengine/reconcile_stale_registration_test.go`'s `newFabricFixture` still returned `gitkit.PairedFixture` as a literal struct type, not just a name in prose — its own doc comment even said the field-mapping wrapper was "kept only so ... callers do not all need to change their field-access pattern in this same batch," anticipating exactly this card's cleanup.
  Replaced with a package-local `fabricFixture` struct carrying the identical field set, so this file's twenty existing `newFabricFixture` callers across the package need no change.
  Added to this card's `Edits:` list for the same reason as the first deviation.

  A third, differently-shaped deviation, surfaced only by this card's `go vet -tags smoke ./...` run: batch 8's export-shim growth (`grow the export shim for the relocating weft suites`) added `export_test.go`'s untagged `WeftWriteLockPathForTest`, wrapping `weftWriteLockPath`, a symbol defined only in the `//go:build integration`-tagged `commit_lock_integration_test.go`.
  Neither batch 8 nor 9 nor 10 runs `go vet -tags smoke`, so the resulting undefined-symbol failure under the smoke tag was invisible until this batch's `verify:` restored that gate.
  Fixed by moving the wrapper into `commit_lock_integration_test.go` itself, alongside the symbol it wraps, so both share the same build tag;
  its one caller (`commit_integration_test.go`, also integration-tagged) is unaffected.
  Added to this card's `Edits:` list for the same reason as the other two.

  A fourth deviation, found while confirming card 70's `lyxtest` gate in advance: `internal/gitkit/leaf_enforcement_test.go`'s own doc comment named `lyxtest` by token — "feature packages' own tests import lyxtest, so a reverse import would close a test-build cycle" — a leftover from before `internal/hubforge` existed that batch 1 card 2's sweep never caught since it lives in `internal/gitkit` itself, not one of the files that sweep targeted.
  `internal/hubforge` is `lyxtest`'s replacement and closes the identical cycle (it imports `gitkit`), so the sentence is retargeted onto it.
  Added to this card's `Edits:` list for the same reason as the other three.

  A fifth deviation, surfaced only by this card's own `go test -tags integration ./cmd/lyx/...` run (which no batch since batch 1/2/3 has exercised): `TestTierPurity_UntaggedTestsSpawnNothing` failed against five files.
  Two are prose-only, fixed by rewording (`internal/configcli/configcli_test.go` and `internal/perchcli/cli_test.go` / `run_test.go`, all three already in this card's own edit set, whose replacement text had written the literal token `hubforge.NewHub` into an untagged file).
  A third, `internal/fabricengine/fabric_test.go`, is a pre-existing batch-8 bug (its own untagged doc comment named `hubforge.NewHub` by token) that no earlier batch's `verify:` ever ran `cmd/lyx` tests to catch; reworded the same way.
  The fourth and structurally different one: `internal/fabricengine/export_test.go` (untagged) carried three fixture helpers — `currentSHA`, `bareBranchSHA`, `commitMessageAt` (plus `commitWarp`, which calls `currentSHA`) — that spawn git directly via `os/exec.Command` to capture output, a pre-existing batch-8/9 bug for the same untested-gate reason.
  Every caller of all four is integration-tagged, so they move, with their `ForTest` wrappers, into a new file, `internal/fabricengine/gitsha_integration_test.go` (`//go:build integration`), added to this card's `Creates:` list.
  `export_test.go`'s own now-unused `os/exec` and `strings` imports are dropped.

  Finish by confirming `grep -rn 'CopyPaired\|CopyPairedLocal\|CopyWeft\|CopyWarpHub' --include=*.go internal cmd` is empty, which is card 70's third gate satisfied in advance.
- **Commit:** `docs(test): retarget prose naming the retired fixture helpers`

### Card 70: Prove no stand-in hub survives anywhere

- **Context:**
  - `internal/gitkit/gitkit.go`
  - `internal/hubforge/hub.go`
  - `internal/fabricengine/export_test.go`
  - `CONSTRAINTS.md`
- **Edits:** none
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  Verification-only gate, no diff.
  Run each of these greps over `internal/` and `cmd/` and confirm the stated result:

  - `grep -rn 'lyxtest' --include=*.go internal cmd` — zero hits.
  - `grep -rn 'fabrictest' --include=*.go internal cmd` — zero hits.
  - `grep -rn 'CopyPaired\|CopyPairedLocal\|CopyWeft\|CopyWarpHub' --include=*.go internal cmd` — zero hits.
  - `grep -rn 'NewPairedForTest' --include=*.go internal cmd` — zero hits, the shim having been renamed in batch 8.
  - `grep -rno 'gitkit\.CopyRepo(' --include=*.go internal cmd` — hits only in `internal/lyxcwd`, nine of them, matching `TestCopyRepoCallerSet_LyxcwdOnly`.
  - `grep -rno 'hubforge\.NewHub(' --include=*.go internal cmd | wc -l` — at least 132 minus whatever consolidation the migration produced where two adjacent sites collapsed onto one fixture;
    report the actual number and account for any shortfall against 132 by naming the call sites that merged, rather than treating the count as a target to hit.
  - `grep -rln 'internal/hubforge' --include=*.go internal cmd` — no file that also declares a package inside `internal/fabriccli`'s dependency set, i.e. no in-package `fabricengine`, `loomengine`, `treadleengine`, `boardengine`, `burlerengine`, `perchengine`, `websterengine`, `gitrepo` or `lyxcwd` test file.
    The compiler enforces this, but eyeball it: it is the `hubforge` Fabric-Fixture Invariant's whole content.

  Write the results into the commit message of card 71 (the next card in this batch), since this card produces no diff of its own.
  If any grep fails, fix it under the batch that owns the file rather than here.
- **Commit:** none

### Card 71: Record the migration's completeness in the module docs

- **Context:**
  - `internal/gitkit/gitkit.go`
  - `internal/hubforge/hub.go`
  - `internal/hubforge/seed.go`
  - `CONSTRAINTS.md`
- **Edits:**
  - `internal/gitkit/doc.go`
  - `internal/hubforge/doc.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  In `internal/gitkit/doc.go`, delete the transitional sentence batch 1 added saying that `CopyWarpHub`/`CopyPaired`/`CopyPairedLocal`/`CopyWeft` are scheduled for deletion, and replace it with the finished statement: `gitkit` hands out `MustRun`, `SeedConfig`, `GitStatusPorcelain`, `HermeticGitEnv` and the single primitive fixture `CopyRepo`, which is callable from `internal/lyxcwd` alone.
  In `internal/hubforge/doc.go`, record the seeding contract as built: `SeedConfig` writes to the anchor-joined `WeftBase` and commits in the weft worktree, `SeedFabricConfig` writes repo-wide fabric config to `BoardDir` and commits through `fabricengine.NewBolt`, and most former seeding sites need neither because `fabriccli.CloneAndWire` materializes every registered module's default config during the clone.
  Also record the teardown contract: junctions are discovered by walking the hub root with `fslink.IsLink`, never by slug, and removed with `fslink.Remove` before `tb.TempDir()`'s own cleanup.
  Carry card 70's grep results into this card's commit message.
- **Commit:** `docs(gitkit,hubforge): record the finished fixture contracts`

## Batch Tests

`verify:` compile-checks the repo under both tags, then runs the five suites that can regress from deleting a helper: `internal/gitkit` (its own pruned tests plus both enforcement guards), `internal/lyxcwd` (the nine `CopyRepo` sites and the import allowlists card 67 may shrink), `cmd/lyx` (the tier-purity and hermetic token guards), and `internal/fabricengine` plus `internal/fabriccli` (the two largest consumers, where a missed migration would surface as a compile failure the narrower suites would not see).

`go vet -tags smoke ./...` is what covers the smoke-tagged files in `internal/reedcli`, `internal/shuttlecli`, `internal/burlerengine` and `internal/treadleengine`, which reference the deleted helpers today and must not after this batch.
Compile-checking is the only gate available for them, for the reason batches 4 and 5 already stated.
