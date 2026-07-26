# Plan: fabric: cutover -- rewire consumers onto fabric, delete warp/weft

```yaml
task: 'fabric: cutover -- rewire consumers onto fabric, delete warp/weft'
slug: fabric-cutover
approved: false
started: 20260726-104646
parent: main
root: ""
verify: null
```

## Batch Index

_The fenced yaml block below is the authoritative DAG mill-go reads to
schedule batches. Every batch lives at `NN-<batch-slug>.md` in this
directory and is mirrored as one entry here._

```yaml
batches:
  - number: 1
    name: A -- consumers
    file: 01-consumers.md
    depends-on: []
    verify: go test -tags integration ./internal/initengine/... ./internal/loomengine/... ./internal/buildercli/... ./internal/webstercli/... ./internal/perchcli/...
  - number: 2
    name: B -- config collapse
    file: 02-config-collapse.md
    depends-on: []
    verify: go test -tags integration ./internal/configreg/... ./internal/configcli/...
  - number: 3
    name: C -- CLI de-registration + sandbox tags
    file: 03-cli-dereg.md
    depends-on: []
    verify: go test ./cmd/lyx/... ./tools/sandbox/...
  - number: 4
    name: D1 -- delete modules + enforcement
    file: 04-delete-modules.md
    depends-on: [1, 2, 3]
    verify: go build ./... && go test -tags integration ./internal/fabricengine/... ./internal/lyxtest/...
  - number: 5
    name: D2 -- doc repoint
    file: 05-doc-repoint.md
    depends-on: [4]
    verify: go build ./...
  - number: 6
    name: D3 -- de-parallel-build prose + final gate
    file: 06-deparallel-and-gate.md
    depends-on: [5]
    verify: go build ./... && go test ./... -tags integration
```

## Shared Decisions

_Cross-cutting decisions every batch inherits._

### Decision: green at each commit, one PR

- **Decision:** Every card is a single commit that leaves `go build ./...` and the batch's
  targeted tests green. warp/weft and fabric coexist until batch D1 deletes the old modules,
  so consumers can be rewired incrementally and every intermediate commit stays bisectable.
  This is still one task / one PR ("coordinated, not incremental" means one cutover, not a
  ban on green intermediate commits).
- **Rationale:** Neither warp/weft nor fabric holds in-process state across calls (each verb
  reads git + config fresh from disk), and fabric is a differential-validated behavioural
  superset, so a consumer on `fabricengine` and one still on `weftengine` operating on the
  same weft repo cannot corrupt each other. Mixed trailered/untrailered weft history in the
  transition window is design-tolerated (`RebuildIndex` skips untrailered commits) and
  dev/test-only within this one PR.
- **Applies to:** all batches

### Decision: script where it avoids opening files; LLM only for semantic edits

- **Decision:** Prefer a shell/`git` command for any change specifiable precisely without
  reading the file's contents (whole-module deletion, whole-file `git rm`, the final grep
  sweeps). Reserve file-opening edits for the semantically ambiguous work: the four
  `weftengine.Commit` -> `fabricengine.New(...).CommitWeft(...)` rewrites, the `configreg`
  collapse, the CLI-help rewrite, and the doc/CONSTRAINTS prose. The Go compiler + `go test`
  are the safety net that makes blind script edits safe.
- **Rationale:** The biggest cost in an LLM cutover is opening files; a statically typed
  language with a strong suite lets scripts do the mechanical spine with no correctness loss.
- **Applies to:** all batches

### Decision: eliminate the modules, keep fabric's own warp/weft API names

- **Decision:** The cutover eliminates the four module *imports*
  (`internal/{warpengine,warpcli,weftengine,weftcli}`), the `lyx warp`/`lyx weft` *CLI*, and
  the `warp`/`weft` *config-module identifiers*. It does NOT touch fabric's own API where
  `Warp`/`Weft` legitimately persist: the `Fabric{ Warp, Weft *gitrepo.Repo }` fields,
  `WarpSHATrailerKey = "Warp-SHA"`, `WeftBranchName()`,
  `CommitWeft`/`PushWeft`/`PullWeft`/`StatusWeft`, and `hubgeometry`'s `-weft` geometry.
- **Rationale:** Those identifiers name fabric's two repos/roles, not the deleted modules.
  A blanket word-rename would corrupt fabric's public surface (which is Out of scope).
- **Applies to:** all batches (especially the grep-clean gate in batch D3)

### Decision: signature gotchas -- fabric's API is not a 1:1 rename of warp/weft

- **Decision:** Call-site rewrites must honour these non-identity mappings:
  `weftengine.Push(w, opts)` -> free func `fabricengine.PushWeftAt(w, opts)`;
  `weftengine.Commit(...)` -> method `(*Fabric).CommitWeft(...)` constructed via
  `fabricengine.New(hostPath, weftWorktree)` (needs the host path **and** the weft worktree);
  `warpengine.New(cfg) *Worktree` -> `fabricengine.NewTopology(cfg) *Topology`;
  `warpengine.LoadConfig(root, "warp")` (two args) -> `fabricengine.LoadConfig(root)` (one arg);
  `weftengine.LoadConfig(root)` -> `fabricengine.LoadConfig(root)`. The identity swaps
  (`WireJunctions`, `UnwireJunctions`, `EnvSyncOptions`, `ScopedPathspec`, `HostClean`,
  `PairInSync`) keep the same call shape, only the package qualifier changes.
- **Rationale:** A naive `s/warpengine/fabricengine/g` mis-compiles; these are the exact
  points a blanket rename would break.
- **Applies to:** A -- consumers, B -- config collapse

### Decision: sweep deleted-module comments in every file you already edit

- **Decision:** Any card that edits a file for its code MUST also, in the same commit, sweep
  that file's `//` comments that name a deleted module (`warpengine`/`warpcli`/`weftengine`/
  `weftcli`, whether full import-path or bare word) -- repoint to `fabricengine`/`fabriccli`
  or reword so no comment names a deleted module. The implementer already has the file open,
  so this costs no extra file reads. Fabric's own API terms (`Warp`/`Weft` fields,
  `Warp-SHA`, `WeftBranchName`, `CommitWeft`, `WeftSuffix`, and any `-weft` geometry) are NOT
  deleted-module names and stay. Comments in files no other card touches are swept in batch
  D3 (cards 22-26).
- **Rationale:** The user directive is "update ALL references to old warp/weft to fabric,"
  and batch D3's final grep gate (card 27) can only be a clean zero-match if every
  deleted-module comment across the tree is already swept -- including fabric's own module's
  `warpengine` provenance comments ("Adapted from warpengine's X.go"), which are numerous.
- **Applies to:** all batches

### Decision: never discard `fabricengine.New`'s error

- **Decision:** Every `CommitWeft` rewrite site uses `f, err := fabricengine.New(hostPath,
  weftWorktree)` and checks `err` before calling `f.CommitWeft(...)`, exactly as
  `internal/fabriccli/weft_verbs.go` already does. Never `f, _ := fabricengine.New(...)`.
- **Rationale:** `New` returns `(*Fabric, error)` and yields a nil `*Fabric` when a path is
  absent (`requireDir`); a discarded error becomes a nil-deref panic in `CommitWeft`.
- **Applies to:** A -- consumers

## All Files Touched

_Full union of every `Creates:` / `Edits:` / `Moves:` target path across every card
(Deletes excluded). Sorted alphabetically._

- `CONSTRAINTS.md`
- `cmd/lyx/exitcode_test.go`
- `cmd/lyx/helptree_test.go`
- `cmd/lyx/jsonhelp_test.go`
- `cmd/lyx/main.go`
- `cmd/lyx/main_test.go`
- `cmd/lyx/unknown_subcommand_test.go`
- `crucible/fabric-review-prompt.md`
- `crucible/gitrepo-review-prompt.md`
- `docs/overview.md`
- `docs/reference/plan-format-v3.md`
- `internal/buildercli/weft.go`
- `internal/codeintelcli/cli.go`
- `internal/configcli/configcli.go`
- `internal/configcli/configcli_integration_test.go`
- `internal/configreg/configreg.go`
- `internal/configreg/configreg_test.go`
- `internal/fabriccli/clone.go`
- `internal/fabriccli/fabric.go`
- `internal/fabriccli/spawn.go`
- `internal/fabriccli/weft_verbs.go`
- `internal/fabricengine/add.go`
- `internal/fabricengine/ancestors.go`
- `internal/fabricengine/checkout.go`
- `internal/fabricengine/cleanup.go`
- `internal/fabricengine/clone.go`
- `internal/fabricengine/config.go`
- `internal/fabricengine/doc.go`
- `internal/fabricengine/drift.go`
- `internal/fabricengine/fabric.go`
- `internal/fabricengine/hook.go`
- `internal/fabricengine/hostclean.go`
- `internal/fabricengine/junction.go`
- `internal/fabricengine/launcher_content.go`
- `internal/fabricengine/launchers.go`
- `internal/fabricengine/list.go`
- `internal/fabricengine/portals.go`
- `internal/fabricengine/prune.go`
- `internal/fabricengine/reconcile.go`
- `internal/fabricengine/remove.go`
- `internal/fabricengine/status.go`
- `internal/fabricengine/topology.go`
- `internal/fabricengine/weftgit.go`
- `internal/fabricengine/weftwiring.go`
- `internal/hubgeometry/hubgeometry.go`
- `internal/initengine/init.go`
- `internal/initengine/init_test.go`
- `internal/initengine/undo.go`
- `internal/loomengine/preflight.go`
- `internal/loomengine/preflight_integration_test.go`
- `internal/lyxtest/doc.go`
- `internal/lyxtest/hermetic.go`
- `internal/lyxtest/leaf_enforcement_test.go`
- `internal/perchcli/run.go`
- `internal/perchengine/doc.go`
- `internal/perchengine/engine.go`
- `internal/reedengine/config.go`
- `internal/webstercli/weft.go`
- `internal/websterengine/audit.go`
- `manifest/designs/board-weft-storage.md`
- `manifest/designs/codeintel-redesign.md`
- `manifest/designs/host-visibility.md`
- `manifest/designs/loom-finalize.md`
- `manifest/designs/raddle.md`
- `manifest/roadmap.md`
- `tools/sandbox/SANDBOX-CORE-SUITE.md`
- `tools/sandbox/SANDBOX-FABRIC-SUITE.md`
- `tools/sandbox/main.go`
