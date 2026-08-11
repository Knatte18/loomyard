# Batch: package-skeleton-and-hub-factory

```yaml
task: 'fabric: live-state integration harness (slice 13)'
batch: 'package-skeleton-and-hub-factory'
number: 2
cards: 5
verify: go build ./... && go test -tags integration ./internal/fabricengine/fabrictest/ && go test -tags integration ./internal/fabricengine/ -run TestCommitWeft_LockArtifactsExcludedFromStatus && go test ./cmd/lyx/ -run 'TestTierPurity_UntaggedTestsSpawnNothing|TestHermeticGitEnv_GitSpawningPackagesHaveTestMain' && go test ./internal/lyxcwd/ -run TestEnforcement
depends-on: [1]
```

## Batch Scope

This batch creates the `internal/fabricengine/fabrictest` package and its foundation: the untagged `doc.go` and `testmain_test.go` that make the package legal under `go build ./...` and the Hermetic Git Test Environment Invariant, the `sync.Once`-cached bare-pair template and the `CloneAndWire`-backed hub factory, and the two TDD suites that prove both before anything else is built on them.
It is one batch because the template builder and the factory are a single mechanism — the factory is meaningless without pushed-to bares, and the bares' two gotchas are only observable through a clone — and because a Sonnet implementing the factory needs the template's exact shape in the same head.

The external interface batches 3-7 consume is the `Hub` value the factory returns and the `NewHub` entry point.

Batch-local decision: `doc.go` is written now with its permanent prose (package doc, the manifest-vs-Snapshot naming note, the Windows-gap section, the `checkForce`-absent note, the `Remove` pre-refusal anomaly note) and with two explicitly-marked placeholder sections — the measured wall-clock and the sabotage table — that batch 8 fills.
Writing them empty now and filling them later is deliberate: both need numbers that do not exist until the matrix runs.

## Cards

### Card 5: package doc

- **Context:**
  - `internal/boardengine/boardtest/doc.go`
  - `internal/fabricengine/snapshot.go`
  - `internal/fabricengine/destroy.go`
  - `internal/fabricengine/remove.go`
  - `manifest/designs/fabric-windows-verification.md`
- **Edits:** none
- **Creates:**
  - `internal/fabricengine/fabrictest/doc.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** Write `package fabrictest`'s doc comment with **no** build tag — it is what keeps the package from failing `go build ./...` with "build constraints exclude all Go files", exactly as `internal/boardengine/boardtest/doc.go` does for its package.
  The doc covers, as named sections in the comment:
  what the package is (the live-state integration harness for `internal/fabricengine` — a real cloned fabric hub, a named hostile-state matrix, a verb table, and the cross product driven with per-cell survival assertions);
  the **naming** rule, that the filesystem capture is called a *manifest* and never a *snapshot*, because `internal/fabricengine/snapshot.go` already owns Snapshot in fabric's vocabulary as the `Snapshot: <tag>` trailer recording a warp SHA on the weft branch;
  the **Windows gap**, that the harness carries no `runtime.GOOS` skip and would run on Windows but nobody has run it there yet, pointing at `manifest/designs/fabric-windows-verification.md`, plus the one genuine divergence — the `trackedSymlinkAtWiredPath` state models a git-tracked symlink, which on Windows materialises as a junction because it is built through `fslink.CreateDirLink`, and the assertion keeps its shape because the gate's `ownedWiredJunction` check compares `fslink.RawTarget`;
  the **three-member `Check` set**, recording that `checkForce` is declared at `destroy.go:39` and rendered by `String()` at `destroy.go:51` but is never constructed into a `destructiveRefusal` anywhere in the tree — force is consulted only inside `checkPathDirtiness`, where it makes the dirtiness check *pass* — so a `CheckForce` constant could never match and must not be added back;
  the **one known refusal-with-side-effects anomaly**, that `Remove` runs `removePortal` and `removeLaunchers` at `remove.go:61-66` before its own dirty pre-flight at `remove.go:68-76`, so a dirty-`Remove` cell that correctly refuses has already destroyed the pair's portal and launcher paths, that this is deliberate documented behaviour and is declared as permitted roots on that cell rather than treated as a defect, and that it is flagged for slice 14's truthfulness work where "what did this call actually mutate before it failed" becomes representable.
  Add two clearly-marked placeholder sections that batch 8 fills and that carry a one-line note saying so: a **measured wall-clock** section, and a **sabotage-proof table** section.
  Add an **omission table** section, likewise marked as populated by batches 6 and 7, which will record every verb/state cell omitted from the cross product together with its reason.
  Every mention of warp/weft in this file is legal because batch 1 registered the directory in `fabricVocabularyOwners`.
- **Commit:** `fabrictest: add the package doc for the live-state harness`

### Card 6: hermetic-git TestMain

- **Context:**
  - `internal/boardengine/boardtest/testmain_test.go`
  - `internal/lyxtest/hermetic.go`
  - `cmd/lyx/hermeticenv_test.go`
  - `CONSTRAINTS.md`
- **Edits:** none
- **Creates:**
  - `internal/fabricengine/fabrictest/testmain_test.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** Mirror `internal/boardengine/boardtest/testmain_test.go` exactly in shape: no build tag, `package fabrictest`, a `TestMain(m *testing.M)` that calls `lyxtest.HermeticGitEnv()` before `os.Exit(m.Run())`.
  It carries no build tag deliberately — the file must be compiled into the test binary on both a plain `go test` and a `-tags integration` run, or the integration-tagged suites would run without the hermetic environment.
  The header comment states that this is what satisfies the Hermetic Git Test Environment Invariant for a package whose every other file spawns git, and names `cmd/lyx/hermeticenv_test.go` as the guard.
- **Commit:** `fabrictest: wire the package into the hermetic git test environment`

### Card 7: bare-pair template and `CloneAndWire`-backed hub factory

- **Context:**
  - `internal/lyxtest/lyxtest.go`
  - `internal/lyxtest/hermetic.go`
  - `internal/fabriccli/clone.go`
  - `internal/fabricengine/clone.go`
  - `internal/fabricengine/junctionnames.go`
  - `internal/fabricengine/junction.go`
  - `internal/fabricengine/topology.go`
  - `internal/fabricengine/config.go`
  - `internal/fabricengine/fabric.go`
  - `internal/fabricengine/weftwiring.go`
  - `internal/fabricengine/portals.go`
  - `internal/fabricengine/launchers.go`
  - `internal/weftname/weftname.go`
  - `internal/lyxcwd/lyxcwd.go`
  - `internal/fabricengine/fabrictest/doc.go`
- **Edits:** none
- **Creates:**
  - `internal/fabricengine/fabrictest/hub.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** `//go:build integration` on the first line.
  Provide three things.
  **(a) A `sync.Once`-cached bare-pair template**, mirroring `lyxtest`'s `buildWarpHub`/`buildWeftPrime` pattern: one warp bare and one weft bare per test binary, built into an `os.MkdirTemp` directory, with fsmonitor, auto-maintenance and `gc.auto` disabled and hook samples stripped exactly as `lyxtest.initRepo`/`initBareRemote` do.
  The warp bare is built by initialising a scratch work repo on `main`, writing a root `README` **and** a `backend/` subdirectory containing at least one committed file, committing, and pushing to the bare — one template serves both anchors, because a `.`-anchored hub simply ignores `backend/`.
  Two gotchas are encoded here and nowhere else, each with a comment naming it:
  first, `git init --bare` leaves `HEAD` on `master` while the pushed branch is `main`, so the builder must run `git -C <bare> symbolic-ref HEAD refs/heads/main` after the push;
  second, the **weft** bare must be left genuinely empty and never pushed to, or `CloneHub`'s bootstrap guard at `clone.go:172` (`!probe.WeftLooksLikeWeft`) refuses it — the warp bare, by contrast, must have content pushed.
  `lyxtest`'s own bares cannot be reused: `initBareRemote` creates them and adds them as `origin` but never pushes, which is exactly why the `symbolic-ref` gotcha never arises there.
  Expose a `copyBares(tb testing.TB) (warpBare, weftBare string)` helper that copies the cached template pair into the test's own `tb.TempDir()`, so each scenario pushes to its own copy and cells never race;
  copy-the-bares is the deliberate choice over building fresh bares per scenario, which would cost a full `git init --bare` plus work repo plus commit plus push per cell.
  **(b) A `Hub` struct** carrying at minimum: `Path` (the hub root), `Anchor` (the `AnchorRel` value, `"."` or `"backend"`), `Location` (`*lyxcwd.Location`, obtained from `lyxcwd.Resolve(res.PrimeCwd)`, never constructed by hand — the Cwd Resolution Invariant), `Topology` (`*fabricengine.Topology` from `fabricengine.NewTopology(fabricengine.Config{})`), `WarpBare`, `WeftBare`, and `Container` (the `tb.TempDir()` the hub was cloned into).
  Add accessor methods that batches 3-7 use instead of joining geometry by hand: the prime warp worktree, the prime weft sibling, the board dir, a pair's warp worktree, a pair's weft sibling, a pair's portal link, and a pair's launcher directory — every one built from the exported `fabricengine`/`weftname`/`lyxdirs` accessors named in the overview's geometry-through-accessors decision.
  **(c) `NewHub(tb testing.TB, anchor string) *Hub`**, the factory: copy the bares, then call `fabriccli.CloneAndWire(container, fabricengine.CloneOptions{WeftURL: ..., WarpURL: ..., Subpath: ...})` with `Subpath` empty for the `.` anchor and `"backend"` for the subpath anchor, both bare URLs passed through `filepath.ToSlash`.
  It calls `tb.Fatalf` on any error, resolves the `*lyxcwd.Location` from `res.PrimeCwd`, and returns the populated `Hub`.
  The factory calls `CloneAndWire` and never replicates the wiring sequence: `fabricengine.CloneHub` alone produces a partial hub — warp clone, weft clone, board, anchor marker, warp binding, but **no junctions and no repo-wide `fabric.yaml`** — which leaves three of the gate's eight path-ownership kinds (`ownedWiredJunction`, `ownedDriftedWiredJunction`, `ownedUnderGeometryRoot`) structurally unreachable.
  Also expose an `AddPair(tb testing.TB, h *Hub, slug string)` helper that drives `h.Topology.Add(h.Location, slug, fabricengine.AddOptions{})` and fatals on error, since several verbs' `Arrange` funcs need a pair.
  Every git spawn in this file goes through `lyxtest.MustRun` or an equivalent local helper — never a hand-rolled `exec.Command` whose failure is silently ignored.
  Keep template and slug names short, per the Windows path-length rule.
- **Commit:** `fabrictest: add the bare-pair template and the CloneAndWire-backed hub factory`

### Card 8: TDD suite for the template builder and the factory

- **Context:**
  - `internal/fabricengine/fabrictest/hub.go`
  - `internal/fabricengine/clone.go`
  - `internal/fabricengine/junctionnames.go`
  - `internal/fabricengine/warpbinding.go`
  - `internal/fabricengine/portals.go`
  - `internal/fabricengine/launchers.go`
  - `internal/lyxcwd/lyxcwd.go`
  - `internal/boardengine/boardtest/sync_test.go`
- **Edits:** none
- **Creates:**
  - `internal/fabricengine/fabrictest/hub_test.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** `//go:build integration` on the first line, `package fabrictest` (in-package, so the template builder's unexported helpers are reachable).
  Two test groups, each `t.Parallel()`.
  **Template builder** — assert the warp bare's `HEAD` resolves to `refs/heads/main` (the `symbolic-ref` gotcha, which a naive builder gets wrong and which is invisible until a clone checks out the wrong branch);
  assert the warp bare has a commit reachable on `main` carrying both a root `README` entry and a `backend/` entry (use `git -C <bare> ls-tree`);
  assert the weft bare is genuinely empty — no refs — because `CloneHub`'s bootstrap guard refuses a non-empty one.
  **Hub factory** — a subtest per anchor (`"."` and `"backend"`) asserting the built hub has the prime warp worktree, the `<name>`-weft sibling, the board directory, a resolved `.lyx-anchor` whose value matches the requested anchor (read it through `lyxcwd.Resolve`'s `AnchorRel`, not by reading the marker file), a recorded warp binding, wired junctions present on the prime warp (compare `fabricengine.WiredNames` on the board dir against what exists in the worktree, using `fslink.IsLink` rather than `os.Lstat`), and a repo-wide `fabric.yaml` at the board dir.
  Then, in the same subtest, call `AddPair` with a short slug and assert the pair's portal link and launcher directory both exist, resolved through `fabricengine.PortalLink` and `fabricengine.LauncherDir` — never a joined `_portals`/`_launchers` literal.
  These assertions are what prove "real hub, not hand-assembled", which is the whole reason the factory calls `CloneAndWire` rather than `CloneHub`.
- **Commit:** `fabrictest: prove the bare template and the hub factory build a real fabric hub`

### Card 9: fold the movable duplicate `gitStatusPorcelain` into `fabrictest`

- **Context:**
  - `internal/fabricengine/commitweftat_test.go`
- **Edits:**
  - `internal/fabricengine/fabrictest/hub.go`
  - `internal/fabricengine/weftgit_exclude_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Add an exported `GitStatusPorcelain(tb testing.TB, repoPath string) string` to `internal/fabricengine/fabrictest/hub.go`, returning the raw output of `git status --porcelain` run in `repoPath` and calling `tb.Fatalf` on failure — the same body as the existing `gitStatusPorcelain` in `internal/fabricengine/weftgit_exclude_test.go`, with `*testing.T` widened to `testing.TB` so states and cells can use it too.
  Then delete the local `gitStatusPorcelain` from `weftgit_exclude_test.go` and retarget its call sites to `fabrictest.GitStatusPorcelain`, adding the `internal/fabricengine/fabrictest` import.
  That file is already `//go:build integration` and already `package fabricengine_test`, so the import is legal — `fabricengine_test` importing `fabrictest` which imports `fabricengine` is fine because Go compiles external test packages separately.
  Leave the second copy, in `internal/fabricengine/commitweftat_test.go`, exactly where it is: that file is in-package `package fabricengine`, so importing `fabrictest` there would close the `fabricengine → fabrictest → fabricengine` cycle.
  It is stuck permanently and that is not a defect to fix here — say so in a one-line comment above the surviving copy.
- **Commit:** `fabrictest: fold the movable duplicate gitStatusPorcelain into the harness`

## Batch Tests

`verify:` runs five scoped commands.
`go build ./...` proves the untagged `doc.go` keeps the new package legal in a default build.
`go test -tags integration ./internal/fabricengine/fabrictest/` runs card 8's TDD suite, which is the batch's real proof.
`go test -tags integration ./internal/fabricengine/ -run TestCommitWeft_LockArtifactsExcludedFromStatus` covers card 9's retargeting without paying for the whole `fabricengine` integration suite, which is minutes long.
`go test ./cmd/lyx/ -run 'TestTierPurity_UntaggedTestsSpawnNothing|TestHermeticGitEnv_GitSpawningPackagesHaveTestMain'` is the guard pair a new git-spawning package trips first: tier purity fails if any untagged test file in the package carries a spawn token, hermetic-env fails if the package has no `HermeticGitEnv` call.
`go test ./internal/lyxcwd/ -run TestEnforcement` catches a bare geometry literal or a vocabulary violation in the new files at the batch that introduces it, rather than at whichever later batch happens to run a wider suite.
