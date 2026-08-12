# Batch: helper deletion

```yaml
task: 'lyxtest: build real fabric hubs, invert the lyxtest/fabric dependency'
batch: 'helper deletion'
number: 11
cards: 4
verify: go vet -tags integration ./... && go vet -tags smoke ./... && go test -tags integration ./internal/gitkit/... ./internal/lyxcwd/... ./cmd/lyx/... ./internal/fabricengine/... ./internal/fabriccli/...
depends-on: [10]
```

## Batch Scope

Every consumer is migrated, so this batch deletes what they no longer call: `CopyPaired`, `CopyPairedLocal`, `CopyWeft`, the transitional `CopyWarpHub` wrapper, the three fixture structs they returned, and the two `sync.Once` template builders (`buildWeftPrime`, `buildWeftOnly`) that existed only to feed them.
What survives in `internal/gitkit` afterwards is the leaf and nothing else: `MustRun`, `SeedConfig`, `GitStatusPorcelain`, `HermeticGitEnv`, `refuseCLIReexec`, and `CopyRepo` with its one-package caller set.

The grep gate in card 69 is this task's completeness proof.
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
  Delete any import left unused by the deletions — `weftname` in particular, if `buildWeftPrime` was its only consumer, in which case `internal/lyxcwd/enforcement_test.go`'s `weftname`-import allowlist entry for `internal/gitkit` must go too (check whether that map still needs the entry and remove it if not).
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

### Card 69: Prove no stand-in hub survives anywhere

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

  Write the results into the commit message of card 70 (the next card in this batch), since this card produces no diff of its own.
  If any grep fails, fix it under the batch that owns the file rather than here.
- **Commit:** none

### Card 70: Record the migration's completeness in the module docs

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
  Carry card 69's grep results into this card's commit message.
- **Commit:** `docs(gitkit,hubforge): record the finished fixture contracts`

## Batch Tests

`verify:` compile-checks the repo under both tags, then runs the five suites that can regress from deleting a helper: `internal/gitkit` (its own pruned tests plus both enforcement guards), `internal/lyxcwd` (the nine `CopyRepo` sites and the import allowlists card 67 may shrink), `cmd/lyx` (the tier-purity and hermetic token guards), and `internal/fabricengine` plus `internal/fabriccli` (the two largest consumers, where a missed migration would surface as a compile failure the narrower suites would not see).

`go vet -tags smoke ./...` is what covers the smoke-tagged files in `internal/reedcli`, `internal/shuttlecli`, `internal/burlerengine` and `internal/treadleengine`, which reference the deleted helpers today and must not after this batch.
Compile-checking is the only gate available for them, for the reason batches 4 and 5 already stated.
