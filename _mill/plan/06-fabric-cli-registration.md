# Batch: fabric-cli-registration

```yaml
task: 'fabric: unify warp + weft into one git-coordination module'
batch: fabric-cli-registration
number: 6
cards: 5
verify: go test -tags integration ./internal/fabriccli ./cmd/lyx
depends-on: [4, 5]
```

## Batch Scope

Delivers the user-visible module: `internal/fabriccli` with the flat 14-verb tree
(`lyx fabric clone|add|list|remove|checkout|pairs|reconcile|prune|cleanup|status|commit|push|pull|sync`),
registration in `cmd/lyx` with every pinned guard updated, the lyxtest leaf-invariant
additions, the sandbox suite (file + runner wiring against the dedicated
`Knatte18/lyx-fabric-test` repos), and the documentation set (overview, roadmap, design
status note). Batch-local decision: per the overview's red-commit decision, cards 27–28
precede the atomic registration commit (card 29); `registration_test.go` and the
sandbox-coverage guard are red between those commits and green at card 29.

## Cards

### Card 27: fabriccli topology verbs

- **Context:**
  - `internal/warpcli/warp.go`
  - `internal/warpcli/clone.go`
  - `internal/clihelp/exec.go`
  - `internal/clihelp/jsonhelp.go`
  - `internal/output/output.go`
  - `internal/hubgeometry/hubgeometry.go`
  - `internal/fabricengine/topology.go`
  - `internal/fabricengine/config.go`
  - `internal/fabricengine/clone.go`
  - `internal/fabricengine/add.go`
  - `internal/fabricengine/checkout.go`
  - `internal/fabricengine/reconcile.go`
  - `internal/fabricengine/status.go`
  - `internal/fabricengine/prune.go`
  - `internal/fabricengine/cleanup.go`
  - `internal/fabricengine/remove.go`
  - `internal/fabricengine/list.go`
- **Edits:** none
- **Creates:**
  - `internal/fabriccli/fabric.go`
  - `internal/fabriccli/clone.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** `fabric.go`: `func Command() *cobra.Command` building the `fabric`
  parent (`RunE: clihelp.GroupRunE`, non-empty `Short` on parent and every sub, `Long`
  with concrete examples on self-discoverable verbs) and `func RunCLI(out io.Writer,
  args []string) int` = `clihelp.Execute(Command(), out, args)`. Topology verbs mirror
  warpcli's handlers one-to-one — `add <slug>`, `list`, `remove [--force] <slug>`,
  `checkout [branch]` (default from `git branch --show-current` like warp),
  `pairs`, `reconcile`, `prune [--apply]`, `cleanup [--apply] [--force]` — each
  resolving `hubgeometry.Getwd()`→`Resolve`, loading `fabricengine.LoadConfig(cwd)`,
  driving `fabricengine.NewTopology(cfg)`, and emitting warp's envelope field shapes
  (`output.Ok`/`output.Err`, same JSON keys: slug/branch/path/pushed, worktrees,
  pairs, entries, …). `clone.go`: `clone [--reset] <host> <weft> [board]` mirroring
  warpcli's `runCloneWithReset` on `fabricengine.RemoveAll`/`fabricengine.CloneHub`,
  emitting `hub`/`board`. Help text describes fabric's uniform `-weft` branch scheme
  where warp's mentions mirrored names (CLI/Cobra Invariant help-accuracy obligation).
- **Commit:** `feat(fabriccli): fabric command tree with topology verbs`

### Card 28: fabriccli weft verbs, detached sync, CLI tests

- **Context:**
  - `internal/weftcli/cli.go`
  - `internal/weftcli/spawn.go`
  - `internal/weftcli/cli_test.go`
  - `internal/weftcli/testmain_test.go`
  - `internal/warpcli/warp_test.go`
  - `internal/proc/proc_linux.go`
  - `internal/proc/proc_windows.go`
  - `internal/lyxtest/lyxtest.go`
  - `internal/hubgeometry/hubgeometry.go`
  - `internal/fabricengine/fabric.go`
  - `internal/fabricengine/weftgit.go`
  - `internal/fabricengine/config.go`
- **Edits:**
  - `internal/fabriccli/fabric.go`
- **Creates:**
  - `internal/fabriccli/weft_verbs.go`
  - `internal/fabriccli/spawn.go`
  - `internal/fabriccli/cli_test.go`
  - `internal/fabriccli/testmain_test.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** `weft_verbs.go`: verbs `status`, `commit`, `push`, `pull`, `sync`
  mirroring weftcli's semantics on the fabric engine. Normal mode (PersistentPreRunE
  logic mirroring weftcli's): resolve Layout, `weftBaseDir = filepath.Join
  (l.WeftWorktree(), l.RelPath)`, `fabricengine.LoadConfig(weftBaseDir)`, pathspec via
  `fabricengine.ScopedPathspec(l.RelPath, cfg.Dirs())`, and a `Fabric` from
  `fabricengine.New(l.WorktreeRoot, l.WeftWorktree())`; verbs call `StatusWeft`,
  `CommitWeft(pathspec, fabricengine.DefaultCommitMessage, fabricengine.
  EnvSyncOptions())`, push = `CommitWeft` then `PushWeft` (weftcli parity), `PullWeft`,
  sync = `CommitWeft` then `spawnPush`. Bypass mode: hidden persistent `--weft-path`
  flag; when set, only `push` is legal (other verbs emit `subcommand requires a
  worktree context`, exit 1) and it calls `fabricengine.PushWeftAt(weftPath,
  fabricengine.SyncOptions{})`. `spawn.go`: `spawnPush(weftPath)` copy of weftcli's —
  env-gate early return, `exec.Command(exe, "fabric", "--weft-path", abs, "push")`,
  `proc.Detach`, `cmd.Start()` without Wait. `cli_test.go` (integration-tagged,
  package `fabriccli_test`, `t.Chdir` fixtures like warpcli's): no-arg listing names
  all 14 verbs; unknown subcommand → JSON `ok=false`, exit 1; `--weft-path status`
  gate error; `pairs` returns `pairs` key; `commit --help` documents the fixed
  `weft sync` message and the `Warp-SHA` trailer; env-map test (`WEFT_SKIP_PUSH=1`
  push exits 0 `ok=true`). `testmain_test.go`: hermetic TestMain.
- **Commit:** `feat(fabriccli): weft verbs with detached sync push`

### Card 29: registration, pinned guards, leaf invariant, sandbox suite file

- **Context:**
  - `cmd/lyx/registration_test.go`
  - `cmd/lyx/longlist_test.go`
  - `cmd/lyx/drift_test.go`
  - `cmd/lyx/sandbox_coverage_test.go`
  - `tools/sandbox/SANDBOX-CORE-SUITE.md`
  - `internal/fabriccli/fabric.go`
  - `_mill/discussion.md`
- **Edits:**
  - `cmd/lyx/main.go`
  - `cmd/lyx/helptree_test.go`
  - `internal/lyxtest/leaf_enforcement_test.go`
- **Creates:**
  - `tools/sandbox/SANDBOX-FABRIC-SUITE.md`
- **Deletes:** none
- **Moves:** none
- **Requirements:** ONE commit. `cmd/lyx/main.go`: import
  `github.com/Knatte18/loomyard/internal/fabriccli`, add `fabriccli.Command()` to
  `root.AddCommand(...)`, append `fabric` to the root `Long`'s "Available modules:"
  sentence. `helptree_test.go`: add `"fabric"` to `requiredModules` and a
  `TestHelpTree_VerbModuleSubcommands` entry with `wantSubs` = the 14 verbs (union of
  the existing warp and weft entries' lists). `leaf_enforcement_test.go`: add
  `internal/fabricengine` and `internal/fabriccli` to `bannedImports` and extend the
  banned-package prose in the package/test doc comments. `SANDBOX-FABRIC-SUITE.md`:
  scenarios in the CORE-suite format (`### S<N> -- <title>`, `**Goal:**`,
  `**Covers:** fabric`, `**Watch:**`, `**Verdict:** ` OK/WARN/FAIL literal, `---`
  separators) against the DEDICATED empty test repos
  `https://github.com/Knatte18/lyx-fabric-test` (host) and
  `https://github.com/Knatte18/lyx-fabric-test-weft` (weft) — never the warp/weft
  sandbox repo: a clone scenario exercising the default derived board URL
  (`<weftURL>.wiki.git`; the operator has initialized that wiki) and asserting the
  weft primary lands on `main-weft`; a lifecycle scenario (add/pairs/checkout/
  reconcile/prune/cleanup) noting the `<slug>-weft` branch expectations; a weft
  scenario (status/commit/push/pull/sync) noting the fixed `weft sync` message, the
  `Warp-SHA` trailer, and the detached sync push lag (mirroring S7's guidance).
  `drift_test.go`/`registration_test.go`/`longlist_test.go`/`sandbox_coverage_test.go`
  need no edits (derive-from-tree) — listed as Context to verify they go green.
- **Commit:** `feat(lyx): register fabric module with pinned guards and sandbox suite`

### Card 30: sandbox runner wiring

- **Context:**
  - `tools/sandbox/suite.go`
  - `tools/sandbox/suite_test.go`
  - `tools/sandbox/SANDBOX-FABRIC-SUITE.md`
  - `sandbox-core-suite.cmd`
  - `sandbox-mux-suite.cmd`
- **Edits:**
  - `tools/sandbox/main.go`
  - `tools/sandbox/main_test.go`
- **Creates:**
  - `sandbox-fabric-suite.cmd`
- **Deletes:** none
- **Moves:** none
- **Requirements:** `tools/sandbox/main.go`: add a `fabric-suite` subcommand following
  the existing `mux-suite`/`shuttle-suite` dispatch pattern, with fabric-specific
  constants for its dedicated hub (host `https://github.com/Knatte18/lyx-fabric-test`,
  weft `https://github.com/Knatte18/lyx-fabric-test-weft`, hub `lyx-fabric-test-HUB`)
  and a clone step that shells `lyx fabric clone` (NOT `lyx warp clone`) via the
  existing `cloneRun` seam pattern, then launches the suite agent on
  `SANDBOX-FABRIC-SUITE.md`. The existing warp-based `build`/`suite` paths and their
  constants stay untouched. `main_test.go`: add a `TestRun_FabricSuiteRoutesToLaunch`
  mirroring `TestRun_MuxSuiteRoutesToLaunch`. `sandbox-fabric-suite.cmd`: repo-root
  wrapper copying `sandbox-mux-suite.cmd`'s shape with the `fabric-suite` argument.
- **Commit:** `feat(sandbox): fabric-suite runner against dedicated fabric test repos`

### Card 31: documentation set

- **Context:**
  - `internal/fabricengine/doc.go`
  - `CONSTRAINTS.md`
  - `_mill/discussion.md`
- **Edits:**
  - `docs/overview.md`
  - `manifest/designs/fabric.md`
  - `manifest/roadmap.md`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** `docs/overview.md`: add a `fabric` bullet to the module list
  (unified git-coordination module over two gitrepo.Repo instances; CLI
  `lyx fabric clone|add|list|remove|checkout|pairs|reconcile|prune|cleanup|status|commit|push|pull|sync`;
  marked "parallel build — warp/weft remain the owners until cutover"; warp and weft
  bullets stay untouched); add `internal/fabriccli` and `internal/fabricengine` to the
  repo-tree block; leave the execution-stack section alone (fabric is not part of the
  orchestration spine). `manifest/designs/fabric.md`: replace the "Design — not
  built" status note with a parallel-build-done note (built and registered alongside
  warp/weft; validated by differential tests; only the cutover task remains; durable
  rationale now also lives in `internal/fabricengine/doc.go`; file is deleted at
  cutover per the Documentation Lifecycle); update the `Trunk` sketch mentions to the
  shipped `Fabric`/`Topology` names and mark the three open questions as resolved
  (nearest-older revert, typed-error staleness, push-timing left to callers).
  `manifest/roadmap.md`: amend the Planned `fabric` item in place — parallel build
  landed (link the design doc), cutover remains; the item stays under Planned, no new
  roadmap entries.
- **Commit:** `docs: fabric parallel build documented in overview, design, roadmap`

## Batch Tests

`verify: go test -tags integration ./internal/fabriccli ./cmd/lyx` runs the new CLI
tests plus every cmd/lyx guard this batch must satisfy: `drift_test.go` (Short on all
15 new commands), `helptree_test.go` (module + 14 subcommands), `registration_test.go`
(exists ⇒ registered), `longlist_test.go` (fabric in root Long),
`sandbox_coverage_test.go` (Covers: fabric), and the cmd/lyx e2e suite. The lyxtest
leaf guard and configreg pins run under the overview's module-wide `go test ./...` at
the batch boundary.
