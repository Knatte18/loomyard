# Plan: fabric: unify warp + weft into one git-coordination module

```yaml
task: 'fabric: unify warp + weft into one git-coordination module'
slug: fabric
approved: true
started: '20260725-063143'
parent: main
root: ""
verify: go test ./...
```

## Batch Index

_The fenced yaml block below is the authoritative DAG mill-go reads to
schedule batches. Every batch lives at `NN-<batch-slug>.md` in this
directory and is mirrored as one entry here._

```yaml
batches:
  - number: 1
    name: gitrepo-growth
    file: 01-gitrepo-growth.md
    depends-on: []
    verify: go test -tags integration ./internal/gitrepo
  - number: 2
    name: fabric-core
    file: 02-fabric-core.md
    depends-on: []
    verify: go test ./internal/fabricengine ./internal/configreg
  - number: 3
    name: fabric-topology-mechanics
    file: 03-fabric-topology-mechanics.md
    depends-on: [2]
    verify: go test -tags integration ./internal/fabricengine
  - number: 4
    name: fabric-pair-lifecycle
    file: 04-fabric-pair-lifecycle.md
    depends-on: [3]
    verify: go test -tags integration ./internal/fabricengine
  - number: 5
    name: fabric-weft-git
    file: 05-fabric-weft-git.md
    depends-on: [1, 2]
    verify: go test -tags integration ./internal/fabricengine
  - number: 6
    name: fabric-cli-registration
    file: 06-fabric-cli-registration.md
    depends-on: [4, 5]
    verify: go test -tags integration ./internal/fabriccli ./cmd/lyx
```

## Shared Decisions

### Decision: parallel build — warp/weft stay untouched

- **Decision:** fabric is built complete and registered ALONGSIDE `warp`/`weft`. No file
  under `internal/warpengine`, `internal/warpcli`, `internal/weftengine`, or
  `internal/weftcli` is edited by any batch; no consumer (`initengine`, `loomengine`,
  `buildercli`, `webstercli`, `perchcli`, `configcli`) is rewired. warp/weft files appear
  only in `Context:` as the behavioral reference. Cutover is a separate future task.
- **Rationale:** operator decision recorded in `_mill/discussion.md` ("fabric skal
  eksistere SAMTIDIG med warp og weft-modulene, og testet back to back").
- **Applies to:** all batches

### Decision: package shape and type vocabulary

- **Decision:** two flat packages: `internal/fabricengine` (engine, returns `(T, error)`,
  never imports cobra/`io.Writer`) and `internal/fabriccli` (cobra tree, JSON envelope).
  The cross-repo coordination handle is `fabricengine.Fabric` with exported fields
  `Warp *gitrepo.Repo` and `Weft *gitrepo.Repo` — no forwarding methods. Hub-scoped
  topology verbs (add/remove/checkout/reconcile/status/prune/cleanup/list) live on
  `fabricengine.Topology`, a config-carrying holder mirroring `warpengine.Worktree`'s
  shape (`NewTopology(cfg Config) *Topology`, methods take `*hubgeometry.Layout`) so the
  differential mapping to warp is one-to-one; topology ops are hub-scoped and have no
  per-pair `Fabric` instance. The design-sketch name `Trunk` is obsolete and must not
  appear anywhere.
- **Rationale:** discussion's "Module structure and naming" decision; warp parity keeps
  the differential tests mechanical.
- **Applies to:** all batches

### Decision: branch naming — `weftBranch = hostBranch + hubgeometry.WeftSuffix`

- **Decision:** the single derivation is `fabricengine.WeftBranchName(hostBranch string)
  string`, returning `hostBranch + hubgeometry.WeftSuffix`. Every fabric code path that
  needs a weft branch name calls it (add, remove, checkout, reconcile, status, drift,
  cleanup inverse via `hubgeometry.WeftHostSlug`, clone's `main-weft` primary). The
  literal `-weft` never appears in fabric Go code — `TestEnforcement_GeometryLiterals`
  bans the token outside `internal/hubgeometry`; the exported constant/helpers are the
  compliant spelling.
- **Rationale:** discussion's "Branch naming" decision; Hub Geometry Invariant.
- **Applies to:** all batches

### Decision: deliberate duplication of warp mechanics, differential validation

- **Decision:** fabricengine gets self-contained adapted copies of warpengine's
  unexported junction/portal/launcher/hook/weftwiring/hostlayout code (no shared helper
  package, no exporting warpengine internals). Copies are validated primarily by
  differential back-to-back integration tests (same lyxtest fixture copied twice, warp op
  on one copy, fabric op on the other, equivalent end state asserted modulo the
  deliberate deltas); pure-logic copies additionally get untagged unit tests.
  The known deliberate deltas the differential comparisons normalize: (1) fabric weft
  branch names carry the `-weft` suffix; (2) fabric's checkout launcher is
  `fabric-checkout<ext>` invoking `lyx fabric checkout` (warp's is `warp-checkout<ext>`);
  (3) fabric's post-checkout hook uses its own sentinel; (4) fabric weft commits carry a
  `Warp-SHA:` trailer; (5) fabric's clone leaves the weft primary on `main-weft`.
- **Rationale:** discussion's "Self-contained junction/portal/launcher mechanics" and
  "Differential back-to-back integration tests" decisions.
- **Applies to:** fabric-topology-mechanics, fabric-pair-lifecycle, fabric-weft-git

### Decision: weft-git parity constants and locks

- **Decision:** fabric reuses weft's exact operational constants for parity: env gates
  `WEFT_SKIP_GIT`/`WEFT_SKIP_PUSH` (read by `fabricengine.EnvSyncOptions`), default
  commit message `"weft sync"` (`fabricengine.DefaultCommitMessage`), and the write-lock
  path `<weftPath>/.weft/weft.write.lock` (an `internal/lock` flock held at the fabric
  layer around gitrepo calls, boardengine-style) — same path as weftengine's so both
  modules serialize against each other during the parallel period. There is NO separate
  fabric push lock: push serialization reuses gitrepo's `PushCoalesced` /
  `.gitrepo-push.lock`. fabric never calls `StageAllAndCommit` (board's opt-in exception
  per `internal/gitrepo/doc.go`); all staging is explicit-list `StageAndCommit`.
- **Rationale:** discussion's "SyncWeft: behavior parity" and "Most git mechanics grow
  into gitrepo" decisions; gitrepo doc.go's consumer rules.
- **Applies to:** fabric-weft-git, fabric-cli-registration

### Decision: correspondence index layering

- **Decision:** the index component (`corrindex.go`) takes an explicit file path, never
  touches git, persists via `internal/state`'s `WriteJSON`/`ReadJSON` (lock path =
  index path + `.lock`), stores entries `{WarpSHA, WeftSHA string, WarpSeq int}` sorted
  by `WarpSeq` (first-parent commit count of the warp SHA), and answers exact and
  binary-search nearest-at-or-before lookups. The fabric layer (methods on `Fabric`)
  owns everything git: gitdir resolution (`git rev-parse --git-dir` in the weft
  worktree — per-worktree by design), `WarpSeq` computation
  (`git rev-list --count --first-parent <sha>` in the warp repo), and `RebuildIndex`'s
  trailer scan of the current weft branch. Trailers in weft history are the sole source
  of truth; the index is derived state — rebuilding it on cache staleness is
  self-correction, not auto-recovery. Stale SHAs that survive a rebuild (history
  rewrite) surface as typed errors, never trigger re-sync.
- **Rationale:** discussion's "Correspondence index" and "Stale SHA handling" decisions;
  Test Tier Purity Invariant (component tests stay untagged).
- **Applies to:** fabric-core, fabric-weft-git

### Decision: test-package discipline

- **Decision:** `internal/fabricengine` has exactly one `TestMain`
  (`testmain_test.go`, package `fabricengine`, created in fabric-core) calling
  `lyxtest.HermeticGitEnv()`. Differential tests import both warpengine and fabricengine
  and therefore live in package `fabricengine_test` (external test package, same
  directory) — legal alongside package-internal tests, sharing the one TestMain per test
  binary. Every git-spawning test file is `//go:build integration`-tagged; pure-logic
  tests stay untagged (Tier 1).
- **Rationale:** Hermetic Git Test Environment Invariant; Test Tier Purity Invariant.
- **Applies to:** all batches

### Decision: batch-internal red commits in fabric-cli-registration

- **Decision:** in batch 6, cards 27–28 create `fabriccli` with a `Command()` before
  card 29 registers it in `cmd/lyx`; `registration_test.go` (exists ⇒ registered) and
  the sandbox-coverage guard are red at those intermediate commits and green again at
  card 29, which lands registration + pinned-set updates + the `**Covers:** fabric`
  suite file in ONE commit. The batch `verify:` runs after the full batch and passes.
- **Rationale:** the guards' exists⇒registered and covered⇔registered assertions cannot
  be satisfied incrementally in either order; one atomic registration commit is the
  smallest green step.
- **Applies to:** fabric-cli-registration

## All Files Touched

- `CONSTRAINTS.md`
- `cmd/lyx/helptree_test.go`
- `cmd/lyx/main.go`
- `docs/overview.md`
- `internal/configreg/configreg.go`
- `internal/configreg/configreg_test.go`
- `internal/fabriccli/cli_test.go`
- `internal/fabriccli/clone.go`
- `internal/fabriccli/fabric.go`
- `internal/fabriccli/spawn.go`
- `internal/fabriccli/testmain_test.go`
- `internal/fabriccli/weft_verbs.go`
- `internal/fabricengine/add.go`
- `internal/fabricengine/ancestors.go`
- `internal/fabricengine/ancestors_test.go`
- `internal/fabricengine/branchname.go`
- `internal/fabricengine/branchname_test.go`
- `internal/fabricengine/checkout.go`
- `internal/fabricengine/cleanup.go`
- `internal/fabricengine/clone.go`
- `internal/fabricengine/clone_differential_test.go`
- `internal/fabricengine/clone_test.go`
- `internal/fabricengine/config.go`
- `internal/fabricengine/config_test.go`
- `internal/fabricengine/corrindex.go`
- `internal/fabricengine/corrindex_test.go`
- `internal/fabricengine/doc.go`
- `internal/fabricengine/drift.go`
- `internal/fabricengine/fabric.go`
- `internal/fabricengine/fabric_test.go`
- `internal/fabricengine/hook.go`
- `internal/fabricengine/hook_test.go`
- `internal/fabricengine/hostclean.go`
- `internal/fabricengine/hostlayout.go`
- `internal/fabricengine/index.go`
- `internal/fabricengine/index_integration_test.go`
- `internal/fabricengine/junction.go`
- `internal/fabricengine/launcher_content.go`
- `internal/fabricengine/launcher_content_test.go`
- `internal/fabricengine/launchers.go`
- `internal/fabricengine/lifecycle_differential_test.go`
- `internal/fabricengine/list.go`
- `internal/fabricengine/portals.go`
- `internal/fabricengine/post-checkout.sh`
- `internal/fabricengine/prune.go`
- `internal/fabricengine/reconcile.go`
- `internal/fabricengine/reconcile_differential_test.go`
- `internal/fabricengine/remove.go`
- `internal/fabricengine/revert.go`
- `internal/fabricengine/revert_test.go`
- `internal/fabricengine/status.go`
- `internal/fabricengine/syncweft.go`
- `internal/fabricengine/syncweft_integration_test.go`
- `internal/fabricengine/template.go`
- `internal/fabricengine/template.yaml`
- `internal/fabricengine/template_test.go`
- `internal/fabricengine/testmain_test.go`
- `internal/fabricengine/topology.go`
- `internal/fabricengine/trailer.go`
- `internal/fabricengine/trailer_test.go`
- `internal/fabricengine/weftgit.go`
- `internal/fabricengine/weftgit_differential_test.go`
- `internal/fabricengine/weftwiring.go`
- `internal/gitrepo/doc.go`
- `internal/gitrepo/pull.go`
- `internal/gitrepo/pull_test.go`
- `internal/gitrepo/reset.go`
- `internal/gitrepo/reset_test.go`
- `internal/lyxtest/leaf_enforcement_test.go`
- `manifest/designs/fabric.md`
- `manifest/roadmap.md`
- `sandbox-fabric-suite.cmd`
- `tools/sandbox/SANDBOX-FABRIC-SUITE.md`
- `tools/sandbox/main.go`
- `tools/sandbox/main_test.go`
