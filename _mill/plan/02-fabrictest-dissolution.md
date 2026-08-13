# Batch: fabrictest dissolution

```yaml
task: 'lyxtest: build real fabric hubs, invert the lyxtest/fabric dependency'
batch: 'fabrictest dissolution'
number: 2
cards: 6
verify: go vet -tags integration ./... && go vet -tags smoke ./... && go test -tags integration ./internal/hubforge/... ./internal/fabricengine/... ./internal/lyxcwd/... ./cmd/lyx/...
depends-on: [1]
```

## Rename mechanic

For each `Moves:` pair the implementer MUST:

1. Run `git mv <old> <new>` FIRST, before making any other change to the moved file.
2. Make ONLY surgical edits — touch only the lines that must change after the move (package declaration, imports, identifier retargeting, seam splits).
3. Use a full-file `Creates:` entry only for genuinely new files that have no predecessor.
4. Never write the relocated file from scratch and delete the original — that breaks git rename history and inflates review diffs.

## Batch Scope

This batch dissolves `internal/fabricengine/fabrictest`: its hub factory becomes `internal/hubforge`, and its live-state assertion machinery becomes `package fabricengine_test` files inside `internal/fabricengine/`.
No package named `fabrictest` survives.
The move is mechanical — no new behavior, no new API beyond `gitkit.GitStatusPorcelain` — so the whole batch is provable by recompiling and re-running the live-state matrix that already exists.

The external interface batch 3 consumes is `hubforge.NewHub`, `hubforge.AddPair`, `hubforge.Hub` and its geometry accessors.

Batch-local decision: the twelve relocated live-state files take a `livestate_` filename prefix inside `internal/fabricengine/`, because their bare names (`doc.go` in particular) would collide with the package's own files and because the prefix keeps the harness legible as one unit in a directory of 90 files.

## Cards

### Card 7: Move the hub factory into internal/hubforge

- **Context:**
  - `internal/fabriccli/clone.go`
  - `internal/lyxcwd/lyxcwd.go`
- **Edits:**
  - `internal/gitkit/gitkit.go`
- **Creates:** none
- **Deletes:** none
- **Moves:**
  - `internal/fabricengine/fabrictest/hub.go` -> `internal/hubforge/hub.go`
- **Requirements:**
  `git mv` first, then change `package fabrictest` to `package hubforge` and **delete the `//go:build integration` line** — `hubforge` production code is untagged, exactly as `internal/gitkit/gitkit.go` is, so `go vet ./...` never sees a package with zero untagged files.
  Delete `GitStatusPorcelain` from the moved file and add it to `internal/gitkit/gitkit.go` instead, keeping its `testing.TB` signature and its body verbatim (`git status --porcelain` in `repoPath`, `tb.Fatalf` on error), with a doc comment saying non-empty output means the worktree has uncommitted changes.
  Rewrite the moved file's own strings and comments that name the old package: the `os.MkdirTemp` pattern `"fabrictest-bare-*"` becomes `"hubforge-bare-*"`, the single seed-commit message `"fabrictest: seed warp template"` and the two `README` bodies naming `fabrictest` become their `hubforge` equivalents, and every comment referring to `lyxtest` names `gitkit`.
  Keep both bare-template gotchas byte-for-byte in behavior: the post-push `git -C <warp-bare> symbolic-ref HEAD refs/heads/main`, and the weft bare left genuinely empty and never pushed to.
  Keep `buildBareTemplate`'s deliberate lack of cleanup — one template pair per test binary, left to the OS temp reaper — and its explanatory comment.
  Everything else moves unchanged: `copyBares`, the `Hub` type, `PrimeWorktree`, `PrimeWeft`, `BoardDir`, `PairWarpWorktree`, `PairWeftSibling`, `PairPortalLink`, `PairLauncherDir`, `NewHub`, `AddPair`, and the private `initScratchRepo`, `initBareRepo`, `commitAll`, `mustGit`, `stripHookSamples`, `copyDirRecursive`.
- **Commit:** `refactor(hubforge): move the hub factory out of fabrictest`

### Card 8: Move the hub factory's own tests

- **Context:**
  - `internal/hubforge/hub.go`
- **Edits:** none
- **Creates:** none
- **Deletes:** none
- **Moves:**
  - `internal/fabricengine/fabrictest/hub_test.go` -> `internal/hubforge/hub_test.go`
- **Requirements:**
  `git mv` first, then change `package fabrictest` to `package hubforge` and keep the `//go:build integration` tag — this file spawns git, so the Test Tier Purity Invariant requires it stay tagged.
  Retarget any `GitStatusPorcelain` reference onto `gitkit.GitStatusPorcelain`, adding the `internal/gitkit` import if needed, and rename `fabrictest` in comments and test names to `hubforge`.
  The existing coverage of `buildBareTemplate` (warp bare `HEAD` on `refs/heads/main`, weft bare genuinely empty) and of `NewHub` must survive intact;
  batch 3 extends this file rather than replacing it.
- **Commit:** `test(hubforge): move the hub factory's tests with it`

### Card 9: Seed the hubforge package with doc.go and TestMain

- **Context:**
  - `internal/hubforge/hub.go`
  - `internal/gitkit/doc.go`
  - `CONSTRAINTS.md`
- **Edits:** none
- **Creates:**
  - `internal/hubforge/doc.go`
  - `internal/hubforge/testmain_test.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  Write `internal/hubforge/doc.go`, untagged, as the package doc: `hubforge` is the repo-wide real-hub fixture factory, it builds every hub fixture through `fabriccli.CloneAndWire` and never replicates the wiring by hand, and it asserts nothing about fabric — which is why its name does not end in `test`.
  Record why `CloneAndWire` rather than `fabricengine.CloneHub`: `CloneHub` alone yields a partial hub (warp clone, weft clone, board, anchor marker, warp binding) with no junctions and no repo-wide `fabric.yaml`, leaving three of the destruction gate's eight path-ownership kinds structurally unreachable.
  Record the copy-the-bares-clone-the-hub model and why a hub cannot itself be copied (its junctions carry absolute targets), and record that no package inside `internal/fabriccli`'s dependency set may import `hubforge` — such tests use an external `*_test` package or `gitkit` — and that this is self-enforcing, since the import would be a compile error.
  Point at the `hubforge` Fabric-Fixture Invariant in `CONSTRAINTS.md` for the machine-checked half.
  Write `internal/hubforge/testmain_test.go`, untagged for the same reason `fabrictest`'s was — it must be compiled into the test binary on a plain `go test` as well as under `-tags integration` — with a `TestMain` calling `gitkit.HermeticGitEnv()` before `m.Run()`.
- **Commit:** `docs(hubforge): add the package doc and hermetic TestMain`

### Card 10: Relocate the live-state machinery into package fabricengine_test

- **Context:**
  - `internal/hubforge/hub.go`
  - `internal/gitkit/gitkit.go`
  - `internal/fabricengine/export_test.go`
  - `internal/fabricengine/testmain_test.go`
- **Edits:** none
- **Creates:**
  - `internal/fabricengine/livestate_helpers_test.go`
- **Deletes:**
  - `internal/fabricengine/fabrictest/testmain_test.go`
- **Moves:**
  - `internal/fabricengine/fabrictest/doc.go` -> `internal/fabricengine/livestate_doc_test.go`
  - `internal/fabricengine/fabrictest/states.go` -> `internal/fabricengine/livestate_states_test.go`
  - `internal/fabricengine/fabrictest/verbs.go` -> `internal/fabricengine/livestate_verbs_test.go`
  - `internal/fabricengine/fabrictest/manifest.go` -> `internal/fabricengine/livestate_manifest_test.go`
  - `internal/fabricengine/fabrictest/mutationoracle.go` -> `internal/fabricengine/livestate_mutationoracle_test.go`
  - `internal/fabricengine/fabrictest/refusal.go` -> `internal/fabricengine/livestate_refusal_test.go`
  - `internal/fabricengine/fabrictest/states_test.go` -> `internal/fabricengine/livestate_states_selftest_test.go`
  - `internal/fabricengine/fabrictest/verbs_test.go` -> `internal/fabricengine/livestate_verbs_selftest_test.go`
  - `internal/fabricengine/fabrictest/manifest_test.go` -> `internal/fabricengine/livestate_manifest_selftest_test.go`
  - `internal/fabricengine/fabrictest/mutationoracle_test.go` -> `internal/fabricengine/livestate_mutationoracle_selftest_test.go`
  - `internal/fabricengine/fabrictest/refusal_test.go` -> `internal/fabricengine/livestate_refusal_selftest_test.go`
  - `internal/fabricengine/fabrictest/matrix_test.go` -> `internal/fabricengine/livestate_matrix_test.go`
- **Requirements:**
  `git mv` all twelve pairs first, then change every moved file's package clause to `package fabricengine_test` and give every one of them a `//go:build integration` line, including `livestate_doc_test.go`, which carries no build tag today.
  Delete `internal/fabricengine/fabrictest/testmain_test.go` outright rather than moving it: `internal/fabricengine/testmain_test.go` already provides the test binary's single `TestMain`, and a second one in the external test package would not compile.
  Retarget the identifiers the moved files lose: `NewHub` becomes `hubforge.NewHub`, `AddPair` becomes `hubforge.AddPair`, the `*Hub` type becomes `*hubforge.Hub`, and `GitStatusPorcelain` becomes `gitkit.GitStatusPorcelain`, adding the `internal/hubforge` and `internal/gitkit` imports to each file that needs them.
  Create `internal/fabricengine/livestate_helpers_test.go` (`package fabricengine_test`, `//go:build integration`) holding a single unexported `mustGit(dir string, args ...string)` copied verbatim from `internal/hubforge/hub.go`, since `states.go`, `verbs.go`, `manifest_test.go` and `refusal_test.go` call it and it does not move with them.
  Resolve redeclaration errors caused by the merge into an existing package: `currentSHA` already exists in `internal/fabricengine`'s test files, so prefix the relocated copy as `liveStateCurrentSHA` (and apply the same `liveState`-prefix rule to any further collision the compiler reports) rather than deleting either side.
  In `livestate_doc_test.go`, retarget the package-doc prose onto its new home — it now documents the live-state harness that lives in `package fabricengine_test`, whose hub factory is `internal/hubforge` — and drop its "crucible campaign" framing, since `crucible/` is three markdown prompt files and a review process with no code.
  The state matrix, the verb table and the sabotage-proof table must survive intact.
- **Commit:** `refactor(fabricengine): relocate the live-state harness into package fabricengine_test`

### Card 11: Retarget fabrictest's two external consumers

- **Context:**
  - `internal/gitkit/gitkit.go`
- **Edits:**
  - `internal/fabricengine/dotlyxjunction_integration_test.go`
  - `internal/fabricengine/weftgit_exclude_test.go`
  - `internal/fabricengine/commitweftat_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  In `internal/fabricengine/dotlyxjunction_integration_test.go` and `internal/fabricengine/weftgit_exclude_test.go`, replace each `fabrictest.GitStatusPorcelain(` call with `gitkit.GitStatusPorcelain(` and drop the now-unused `internal/fabricengine/fabrictest` import;
  both files are already `package fabricengine_test`, so nothing else about them changes in this card.
  Update the comments in both files that name `fabrictest.GitStatusPorcelain` so they name `gitkit.GitStatusPorcelain`.
  In `internal/fabricengine/commitweftat_test.go`, update the comment explaining why its local `gitStatusPorcelain` copy cannot fold into the shared helper: the reason is unchanged (the file is in-package `fabricengine`), but the helper it names is now `gitkit.GitStatusPorcelain`, and with `gitkit` being a leaf below fabric this copy is in fact now removable — leave the code alone and note in the comment that folding it in is follow-up work outside this task's scope.
  After this card, `grep -rn 'fabrictest' --include=*.go internal cmd` must return only the comment hits that card 12 fixes.
- **Commit:** `refactor(fabricengine): point the live-state consumers at gitkit.GitStatusPorcelain`

### Card 12: Retire every fabrictest path reference

- **Context:**
  - `internal/fabricengine/livestate_doc_test.go`
  - `CONSTRAINTS.md`
- **Edits:**
  - `cmd/lyx/destructiveguard_test.go`
  - `internal/lyxcwd/enforcement_test.go`
  - `internal/fabriccli/clone.go`
  - `internal/fabricengine/mutation.go`
  - `internal/fabricengine/doc.go`
  - `internal/fabricengine/destroy_test.go`
  - `internal/fabricengine/refusalof_test.go`
  - `internal/fabricengine/mutation_record_integration_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  In `cmd/lyx/destructiveguard_test.go`, delete the `"internal/fabricengine/fabrictest"` entry from the subtree-exclusion allowlist together with its reason string.
  The exclusion is dead rather than merely relocated: the guard's own walk already skips `*_test.go` files, and every relocated live-state builder is now a `_test.go` file.
  In `internal/lyxcwd/enforcement_test.go`, delete the `"internal/fabricengine/fabrictest"` key from both allowlist maps (the production-file scan map and the narrower `weftname`-import map) along with their comments;
  `internal/hubforge` does **not** replace them in either map, because `hubforge` imports `weftname` only if `hub.go` does — check, and add `"internal/hubforge"` to the `weftname` map only if the compiler or the test says it is needed.
  In `internal/fabriccli/clone.go` and `internal/fabricengine/mutation.go`, rewrite the comments naming `fabrictest` so they name `internal/hubforge` (`clone.go`'s comment is the one warning that a second copy of `CloneAndWire`'s wiring is a hazard — keep that point, just retarget the package name).
  In `internal/fabricengine/doc.go`, rewrite the `fabrictest` reference to name the in-package `fabricengine_test` live-state harness and `internal/hubforge`.
  Finish with a **bare-word** sweep, not a qualifier-only one — `grep -rln '\bfabrictest\b' --include=*.go internal cmd` must come back empty at the end of this card, and the three files added to `Edits:` above (`internal/fabricengine/destroy_test.go`, `internal/fabricengine/refusalof_test.go`, `internal/fabricengine/mutation_record_integration_test.go`) carry exactly that bare-word prose and no `fabrictest.` selector, so a qualifier-only pass leaves them behind and batch 11 card 69's zero-hit gate fails with no card owning the fix.
  Each is a one-line comment naming `fabrictest` as the package that duplicates a refusal message, holds a mutation-oracle copy, or reads a record through the exported surface;
  retarget each to name the relocated `package fabricengine_test` live-state harness, and check the claim still holds after the rename rather than only the name.
- **Commit:** `refactor: retire every internal/fabricengine/fabrictest reference`

## Batch Tests

`verify:` compile-checks the whole repo under both tags, then runs the four suites this batch moves or touches: `internal/hubforge` (the relocated `hub_test.go`), `internal/fabricengine` (the entire relocated live-state matrix — `livestate_matrix_test.go` plus the five self-test files — alongside the package's own 88 test files), `internal/lyxcwd` (`enforcement_test.go`'s allowlist maps) and `cmd/lyx` (`destructiveguard_test.go`'s exclusion list).

`internal/fabricengine`'s integration suite is the expensive one in the repo and is run in full here deliberately: this batch relocates ~4960 lines of assertion machinery into that package, and a scoped subset would not prove the merge kept the matrix intact.
