# Batch: clone-commits-module-configs

```yaml
task: 'fabric: clone doesn''t commit written module configs'
batch: 'clone-commits-module-configs'
number: 3
cards: 3
verify: go test -count=1 ./internal/configengine/... ./internal/fabriccli/... && go test -count=1 -tags integration ./internal/fabriccli/... ./internal/fabricengine/... ./internal/hubforge/... ./internal/preflight/... ./internal/preflightshed/...
depends-on: [1, 2]
```

## Batch Scope

This batch is the fix itself and its proof: `fabriccli.CloneAndWire` gains a weft commit of the per-worktree module configs it just materialised, the doc comments that enumerate that wiring sequence and the post-clone fixture state are brought back into agreement with it, and two new test files prove both halves of the reported symptom plus the fixture-helper regression batch 2 pre-empted.

It is one batch because the production edit and its tests share almost their entire `Context:` — `internal/fabricengine/commitweftpaths.go`, `internal/configreg/configreg.go`, `internal/configengine/config.go`, `internal/hubforge`'s hub factory — and because a Sonnet session holding the clone sequence in its head is exactly the session that should be writing the assertions about it.

It depends on batch 1 for `configengine.ConfigFileRel`, which card 3 calls, and on batch 2 for the `--allow-empty` fixture tolerance, without which `internal/preflight` and `internal/preflightshed` fail nine subtests the moment card 3 lands (measured;
see the overview's `fixture-tolerance-lands-before-the-fix` Decision).

Batch-local decision beyond the overview's: card 4 carries an explicit temporary-revert step to establish the TDD property the discussion asks for without ever committing a red tree.

## Cards

### Card 3: commit the materialised module configs at the end of `CloneAndWire`

- **Context:**
  - `internal/fabricengine/commitweftpaths.go`
  - `internal/fabricengine/clone.go`
  - `internal/fabricengine/mutation.go`
  - `internal/configengine/config.go`
  - `internal/configsync/configsync.go`
  - `internal/configreg/configreg.go`
  - `internal/hubforge/seed.go`
  - `CONSTRAINTS.md`
- **Edits:**
  - `internal/fabriccli/clone.go`
  - `internal/hubforge/hub.go`
  - `internal/hubforge/doc.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  In `internal/fabriccli/clone.go`, extend the final block of `CloneAndWire` — the one that follows `configsync.ReconcileAll(res.WeftBase, true)` — so it accumulates a commit pathspec alongside the existing recording loop and then commits it.

  Declare a `relPaths []string` immediately before the existing `for _, r := range results` loop.
  Inside that loop's existing `if r.Applied` body, keep the `rec.Append(fabricengine.KindFileWritten, configengine.ConfigFile(res.WeftBase, r.Module), "")` call exactly where it is and append `configengine.ConfigFileRel(r.Module)` to `relPaths` after it.
  The append must stay after the `rec.Append`, and the whole loop must stay before the commit call, because array order is the only thing carrying ordering in the mutation vocabulary and `CommitWeftPaths` appends its `KindCommitCreated` entry last.

  After the loop, call `fabricengine.CommitAnchoredPaths(rec, l, relPaths, "fabric clone: record module configs", fabricengine.SyncOptions{})`, discarding the returned sha and committed bool and returning `fabricengine.CloneResult{}, err` on a non-nil error — matching every other post-recorder failure site in this function, whose named-result plus `defer func() { res.Mutations = rec.Snapshot() }()` idiom carries the accumulated record through the zero return.
  Do not add any recording of the commit at this call site: `CommitWeftPaths` already appends `KindCommitCreated` at its own success site, and duplicating it would break the Mutation Record Invariant.
  Do not push the weft primary branch, do not use `fabricengine.NewBolt`, and do not call raw git.
  `l` is already in scope from the `lyxcwd.Resolve(res.PrimeCwd)` call earlier in the function;
  do not re-resolve it and do not name a weft path anywhere in this package.

  `relPaths` being empty on an adopt-path re-clone is a legitimate outcome, not a bug: `CommitWeftPaths`'s own `len(relPaths) == 0` guard returns with no lock taken and nothing recorded.
  Do not add a guard of your own.

  Update this file's header comment (the six-line comment above `package fabriccli`), which enumerates the wiring sequence, so its enumeration includes the new per-worktree config commit.
  Update `CloneAndWire`'s own doc comment the same way: its first sentence enumerates "repo-wide fabric.yaml materialization, the weft:main anchor+config commit and push, warp junction wiring, and per-worktree config reconciliation" and must now also name the per-worktree config commit that follows reconciliation, stated as a commit on the weft primary branch with no push.

  In `internal/hubforge/hub.go`, update the `Hub` type's or `NewHub`'s doc comment to describe the post-clone state a `CloneAndWire`-built hub now arrives in: the weft prime worktree is clean, with each registered non-`fabric` module's config committed on the weft primary branch, rather than carrying those files as untracked content.
  In `internal/hubforge/doc.go`, update the package comment's seeding-contract paragraph — the one already stating that a real hub arrives with every registered module's default config already materialized — so it also says those configs arrive committed, and that this is why `SeedConfig` commits with an empty stage allowed.
  Keep both edits to the doc text;
  do not change hubforge's behaviour in this card.
- **Commit:** `fix(fabriccli): commit the module configs clone materialises`

### Card 4: integration proof that clone commits the configs and a new pair inherits them

- **Context:**
  - `internal/fabriccli/clone.go`
  - `internal/fabriccli/testmain_test.go`
  - `internal/fabriccli/pushbypass_integration_test.go`
  - `internal/hubforge/hub.go`
  - `internal/hubforge/doc.go`
  - `internal/configreg/configreg.go`
  - `internal/configengine/config.go`
  - `internal/gitkit/gitkit.go`
  - `internal/fabricengine/mutation.go`
  - `internal/fabricengine/commitweftpaths.go`
  - `CONSTRAINTS.md`
- **Edits:** none
- **Creates:**
  - `internal/fabriccli/cloneconfigcommit_integration_test.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  Create one new test file whose first non-empty line is the build constraint `//go:build integration`, in `package fabriccli_test`, carrying a file-header comment naming what it proves.
  The external test package and the tag are both mandatory: `CONSTRAINTS.md`'s hubforge Fabric-Fixture Invariant forbids any package inside `internal/fabriccli`'s dependency set from importing `hubforge` and sanctions an external `*_test` package instead, and its Test Tier Purity Invariant forbids `hubforge.NewHub` from an untagged file.
  `internal/fabriccli/testmain_test.go` already provides the `gitkit.HermeticGitEnv()` `TestMain` for this package;
  do not add a second one.

  Derive the expected module set in every test below from `configreg.Modules()` filtered to entries whose `Name` is not `"fabric"`, never from a hard-coded list of nine — a tenth registered module must not silently make these tests wrong.

  Write five tests:

  1. Weft prime is clean after clone.
     Build a hub with `hubforge.NewHub(t, ".")`, then assert `gitkit.GitStatusPorcelain(t, h.PrimeWeft())` is empty, and that `git ls-files` run in `h.PrimeWeft()` lists `configengine.ConfigFileRel(m.Name)` (slash-separated, as git reports it) for every module in the derived set.
  2. A pair created off the clone has its configs.
     From the same kind of hub, call `hubforge.AddPair(t, h, <slug>)`, then assert the new pair's weft sibling carries the anchored `loom.yaml` config on disk — `configengine.ConfigFile` joined against the pair's anchored weft base, derived from `h.PairWeftSibling(slug)` and `h.Anchor`.
     This is the direct end-to-end proof of the reported symptom, stopping short of spawning loom itself.
  3. Anchor scoping.
     Repeat test 1 against `hubforge.NewHub(t, "backend")` and assert the committed paths git reports are prefixed with the anchor, `backend/_lyx/config/<module>.yaml`, proving the commit was anchor-scoped rather than run at the weft base, which at a non-`"."` anchor is a subdirectory of the worktree root.
  4. One commit, not one per module.
     Assert that `git log --oneline` on the weft primary branch of a freshly-built hub contains the subject `fabric clone: record module configs` exactly once, and that a second `configsync.ReconcileAll` over the same weft base reports `Applied` false for every module.
  5. Mutation record shape.
     Call `fabriccli.CloneAndWire` directly (not through `hubforge.NewHub`) against bare repos, and assert the returned `res.Mutated().Entries()` contains one `fabricengine.KindFileWritten` entry per module in the derived set, followed by a `fabricengine.KindCommitCreated` entry whose `Target` is the weft worktree, with the commit entry last — array order is part of the vocabulary.
     If wiring a direct `CloneAndWire` call in this file would duplicate `hubforge`'s bare-template machinery, drive the hub through `hubforge.NewHub` and read the record it produced instead, keeping the same ordering assertion;
     do not copy `hubforge`'s template builder into this file.

  Before finalising, establish the TDD property the discussion asks for without committing a red tree: temporarily comment out the `fabricengine.CommitAnchoredPaths` call card 3 added to `internal/fabriccli/clone.go`, confirm tests 1 through 5 fail, then restore the call and confirm they pass.
  Leave `internal/fabriccli/clone.go` byte-identical to card 3's committed state — the temporary edit is a check, never part of this card's diff.

  Do not add an injection seam for making `CommitWeftPaths` fail;
  the commit-error return path is deliberately untested, per the discussion's `error-path-returns-zero-result` Decision.
- **Commit:** `test(fabriccli): prove clone commits module configs and pairs inherit them`

### Card 5: regression test that `SeedConfig` survives a redundant seed

- **Context:**
  - `internal/hubforge/seed.go`
  - `internal/hubforge/hub.go`
  - `internal/hubforge/hub_test.go`
  - `internal/hubforge/testmain_test.go`
  - `internal/configengine/config.go`
  - `internal/configreg/configreg.go`
  - `CONSTRAINTS.md`
- **Edits:** none
- **Creates:**
  - `internal/hubforge/seed_test.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  Create `internal/hubforge/seed_test.go` whose first non-empty line is `//go:build integration`, in the in-package `package hubforge` (matching `hub_test.go`, which is also in-package and integration-tagged), with a file-header comment naming what it proves.
  Do not add a `TestMain`;
  `testmain_test.go` already installs the hermetic git environment for this package on both tagged and untagged runs.

  Write one test asserting that `SeedConfig` does not fatal when handed a seed byte-identical to what the clone already committed.
  Build a hub with `NewHub(t, ".")`, read the already-materialised `loom` config from `configengine.ConfigFile(h.WeftBase, "loom")`, then call `SeedConfig(t, h, map[string]string{"loom": string(<those exact bytes>)})` and assert the call returns normally.
  A `t.Fatalf` from inside the helper fails the test on its own, so no extra assertion machinery is needed beyond reaching the line after the call.

  Add a second case in the same test, or a sibling test, that seeds a genuinely different `loom` config and asserts the file on disk at `configengine.ConfigFile(h.WeftBase, "loom")` afterwards holds the new content — so the `--allow-empty` flag is shown not to have turned the helper into a no-op.

  This test fails without batch 2's `--allow-empty` edit once card 3 has landed, and is the only coverage that exercises that flag: `internal/hubforge`'s existing suite passes either way, because no existing test seeds a byte-identical override.
  Do not weaken it into a test of `git commit --allow-empty` itself.
- **Commit:** `test(hubforge): cover SeedConfig against a redundant byte-identical seed`

## Batch Tests

`verify:` runs two chained commands.

The untagged half, `go test -count=1 ./internal/configengine/... ./internal/fabriccli/...`, keeps batch 1's accessor unit test green alongside `internal/fabriccli`'s own untagged suite (`argsarity_test.go`, `cli_test.go`, `envelope_test.go`), which compiles against the edited `clone.go`.

The tagged half, `go test -count=1 -tags integration ./internal/fabriccli/... ./internal/fabricengine/... ./internal/hubforge/... ./internal/preflight/... ./internal/preflightshed/...`, covers the two new test files (cards 4 and 5) plus the three packages whose fixtures observe post-clone weft state.
`internal/fabricengine` is in scope deliberately: it is the heaviest consumer of `hubforge.NewHub` in the repo, its live-state harness inspects real worktree cleanliness, and it is the package most likely to encode the old dirty shape.
It is also the slow one — roughly 40 s of the batch's ~50 s verify — which is the price of catching a fixture-state regression at the introducing batch rather than at the done gate.

This is a targeted scope, not the full repo gate.
The full gate (`go test ./... && go test -tags integration ./...`) is `mill-config.yaml`'s `pipeline.done_gate` and runs once at Handoff.
The discussion's Testing §8 regression sweep is already known to be empty beyond the two `internal/preflight*` sites batch 2 fixed — measured during Phase: Plan by applying the whole change as a throwaway spike and running both halves of the gate against it — so no card in this batch carries an open-ended "update whatever breaks" edit list.
See the overview's `spike-verified-no-regression-sweep` Decision for the measurement, and `preexisting-stencilcli-failure-blocks-done-gate` for the one unrelated failure the Handoff done gate will surface.
