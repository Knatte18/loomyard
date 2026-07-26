# Discussion: fabric: cutover -- rewire consumers onto fabric, delete warp/weft

```yaml
task: 'fabric: cutover -- rewire consumers onto fabric, delete warp/weft'
slug: fabric-cutover
status: discussing
parent: main
```

## Problem

`fabric` (`internal/fabricengine` + `internal/fabriccli`) already shipped as a **parallel
build** -- a full, no-remainder replacement for the two shipped git-coordination modules
`warp` (host<->weft topology) and `weft` (git into the paired weft repo), validated by
differential back-to-back integration tests against warp/weft on the same fixture. But
nothing in the real system calls fabric yet: every live consumer still imports
`warpengine`/`warpcli`/`weftengine`/`weftcli`.

This task is the **cutover**: rewire every consumer onto `fabricengine`/`fabriccli`, then
delete the four old modules in one coordinated pass. Per
[`manifest/designs/fabric.md`](../manifest/designs/fabric.md)'s Build order, this is
build-order step 2 ("one large, coordinated cutover replaces warp/weft with fabric and
deletes the old modules -- not incremental"). **Why now:** the cutover is what makes
fabric's uniform `<slug>-weft` branch-naming enforcement actually take effect, which the
(not-yet-filed) "board: move storage to weft:main" task depends on. It must land before
that work.

## Scope

**In:**

- Rewire six production consumers onto `fabricengine`:
  `internal/initengine/init.go` + `undo.go`, `internal/loomengine/preflight.go`,
  `internal/buildercli/weft.go`, `internal/webstercli/weft.go`,
  `internal/perchcli/run.go`.
- Collapse the seventh consumer `internal/configreg/configreg.go` (the config-module
  registry): remove the separate `warp` and `weft` config modules, keep the single
  already-registered `fabric` module.
- `internal/configcli/configcli.go`: switch its `weftcli.RunCLI(..., "sync")` dispatch to
  `fabriccli.RunCLI(..., "sync")`.
- `cmd/lyx/main.go`: drop `warpcli`/`weftcli` registration and their names from `root.Long`;
  flip the sandbox shared-hub bootstrap `cloneRun` from `lyx warp clone` to `lyx fabric clone`.
- Delete the four old modules: `internal/warpengine`, `internal/warpcli`,
  `internal/weftengine`, `internal/weftcli` (packages + all their tests).
- Delete the four `internal/fabricengine/*_differential_test.go` files (they import
  warp/weft as the reference fixture and cannot compile once those are gone).
- Rewrite the consumer/registry integration tests that reference warp/weft onto fabric
  (see Testing).
- Docs/invariants in the same effort: delete `manifest/designs/fabric.md`; add a **Done**
  entry to `manifest/roadmap.md`; repoint the nine other docs that link to `fabric.md`
  (incl. `crucible/fabric-review-prompt.md`, `crucible/gitrepo-review-prompt.md`);
  cut the parallel-build paragraph from `internal/fabricengine/doc.go`; update
  `docs/overview.md`; update `CONSTRAINTS.md` (Weft Git Invariant, lyxtest Leaf Invariant);
  remove sandbox CORE-SUITE scenarios S7/S8 and de-parallel-build the FABRIC-SUITE +
  `tools/sandbox/main.go` prose.
- Sweep stale in-code comment references to the deleted modules (full-path refs in
  `internal/lyxtest/doc.go`, `internal/hubgeometry/hubgeometry.go:700`,
  `internal/codeintelcli/cli.go:34`, plus a bare-name review sweep) -- see Testing's
  two-tier grep-gate section for the exact list and Tier-1/Tier-2 rules.

**Out:**

- **Renaming fabric's own API.** The identifiers `warp`/`weft` legitimately persist inside
  fabric: the `Fabric{ Warp, Weft *gitrepo.Repo }` struct fields, `WarpSHATrailerKey =
  "Warp-SHA"`, `WeftBranchName()`, `CommitWeft`/`PushWeft`/`PullWeft`/`StatusWeft`, and
  `hubgeometry`'s `-weft` worktree geometry. These name fabric's two repos/roles, not the
  old modules. They are **not** touched. The grep-clean gate targets import paths + the
  `lyx warp`/`lyx weft` CLI + the `warp`/`weft` config-module identifiers -- not the words.
- Any change to fabric's behaviour or public surface. Fabric is used exactly as shipped.
- On-disk config migration. No automated migration of existing `_lyx/config/` from
  `warpengine.yaml`+`weftengine.yaml` to `fabricengine.yaml`; a live hub regenerates via
  `lyx init`.
- Back-compat aliases for the removed `lyx warp`/`lyx weft` CLI or config modules.
- The downstream "board: move storage to weft:main" task (separate; merely unblocked by this).

## Decisions

### staging -- green at each step, one PR

- Decision: Rewire consumers one group at a time with `go build ./...` + targeted tests
  green after each commit; delete the old modules only once nothing imports them. It is
  still **one task / one PR** ("coordinated, not incremental" per fabric.md means one
  cutover, not spread across releases -- it does not forbid green intermediate commits).
- Rationale: warp/weft and fabric coexist today, so incremental rewiring is possible and
  keeps every commit bisectable and reviewable. Neither warp/weft nor fabric holds
  in-process state across calls (each verb reads git + config fresh from disk), and fabric
  is a differential-validated behavioural superset, so a consumer on `fabricengine` and one
  still on `weftengine` operating on the same weft repo cannot corrupt each other.
- Rejected: single atomic commit (nothing compiles mid-way; hard to review/bisect).

### batch DAG

- Decision: Four batches; A, B, C are independent (parallelizable), D depends on all three.
  - **A -- Consumers:** rewire the six consumer files; rewrite `initengine/init_test.go`
    and `loomengine/preflight_integration_test.go`.
  - **B -- Config flate:** collapse `configreg`; switch `configcli` to `fabriccli.RunCLI`;
    rewrite `configreg/configreg_test.go` and `configcli/configcli_integration_test.go`.
  - **C -- CLI de-registration + sandbox tags (atomic for `cmd/lyx` tests):** `main.go`
    remove warpcli/weftcli registration + `root.Long` names + flip `cloneRun` to
    `fabric clone`; update `cmd/lyx/helptree_test.go` (remove the warp/weft subcommand
    cases); remove CORE-SUITE S7/S8; fix any other pinned `cmd/lyx` fixtures
    (`main_test.go`, `exitcode_test.go`, `unknown_subcommand_test.go`, `jsonhelp_test.go`).
  - **D -- Delete + docs:** delete `internal/{warpengine,warpcli,weftengine,weftcli}` +
    the four `fabricengine/*_differential_test.go`; update
    `internal/lyxtest/leaf_enforcement_test.go`; `CONSTRAINTS.md`; `doc.go` cut; delete
    `manifest/designs/fabric.md` + roadmap Done + repoint 9 links; `docs/overview.md`;
    de-parallel-build FABRIC-SUITE + `tools/sandbox/main.go`; **de-parallel-build fabric's
    own module** -- rewrite `internal/fabriccli/fabric.go` `Short`/`Long` to "sole
    git-coordination module" and drop its `manifest/designs/fabric.md` ref (`:12,:40,:53,:54,:218`),
    and sweep the genuine "parallel-build period" comments in `internal/fabricengine`
    (`clone.go:23`, `cleanup.go:14/63/158`, `fabric.go:96`, `hook.go:9`, `weftgit.go:8/27`)
    + `internal/fabriccli/fabric.go`; **sweep stale deleted-module comment refs**
    (`lyxtest/doc.go`, `hubgeometry.go:700`, `codeintelcli/cli.go:34`, `cmd/lyx/main.go:92`,
    + bare-name review of perchengine/websterengine/reedengine/lyxtest -- see Testing's
    grep-gate section for the exact list and Tier-1/Tier-2 rules).
- Rationale: C removes only CLI *registration* (warpcli/weftcli still compile), so it is
  independent of A/B. The C changes must be atomic because the moment warp/weft leave
  `newRoot()`, `helptree_test.go`'s `requiredModules`, `longlist_test.go`, and
  `sandbox_coverage_test.go` (Covers: tags) all fail unless updated in the same batch. D
  cannot run until every real importer of the old packages is gone (consumers via A,
  configreg via B, CLI registration via C).
- Rejected: fewer/atomic batches (loses green-at-each-step); letting mill-plan choose
  freely (the C-atomicity constraint and the {A,B,C}->D dependency are load-bearing and
  must be honored).

### scripts vs LLM edits -- avoid opening files where possible

- Decision: Prefer a script (Python or shell) for any change specifiable precisely
  **without reading the file's contents**; reserve LLM file-opening for the semantically
  ambiguous edits. The compiler + `go test` are the safety net (Go is statically typed and
  the suite is strong), which is what makes blind script edits safe here.
  - **Script (blind):** deleting whole modules + the four differential tests; removing
    known import/registration lines; retagging/removing suite scenarios; the final grep
    verification sweep.
  - **LLM (opens file):** the four `weftengine.Commit` -> `fabric.New(...).CommitWeft(...)`
    rewrites; the `configreg` collapse; CONSTRAINTS/doc prose.
- Rationale: the biggest cost in an LLM cutover is opening files (tokens); anything a script
  can do without that saves cost with no correctness loss given the type/test safety net.
- Rejected: full rename script (`s/warpengine/fabricengine/g`) -- the symbols do not line
  up 1:1 (see Technical context), so a blanket rename mis-compiles.

### config-module collapse -- full removal, no alias, no migration

- Decision: The `warp` and `weft` config modules are removed entirely; only `fabric`
  remains. Module count drops 13 -> 11. `lyx config warp`/`lyx config weft` cease to exist;
  `lyx init` writes only `fabricengine.yaml`. No deprecated aliases, no on-disk migration.
- Rationale: pre-1.0 dev tooling; a live hub regenerates config via `lyx init`. YAGNI on
  aliases/migration.
- Rejected: keep warp/weft as deprecated aliases; ship a one-shot config migration step.

### fabric.md deletion -- code is the source of truth

- Decision: Delete `manifest/designs/fabric.md`. Add a short **Done** entry to
  `manifest/roadmap.md` (plain text, no link to the deleted file). Repoint the seven other
  docs that link to `fabric.md` to `internal/fabricengine/doc.go`.
- Rationale: fabric.md's own status banner prescribes deletion at cutover once `doc.go`
  is the sole rationale source (it already carries the rationale). Once the module is
  built, the code is the truth; a separate design doc rots.
- Rejected: keep fabric.md as a thin stub pointing to doc.go (leaves a redundant doc the
  documentation-lifecycle says to remove).

### sandbox -- fabric's dedicated hub survives as the fabric suite

- Decision: Keep fabric's dedicated sandbox repos (`lyx-fabric-test` /
  `-weft` / `-HUB`) and the FABRIC-SUITE as-is; they are already seeded for fabric's
  stricter `main-weft` branch-naming. Retire warp/weft's presence in the **shared**
  `lyx-test` hub story: flip `cloneRun` (shared-hub bootstrap, used by the other module
  suites) from `warp clone` to `fabric clone`, and delete CORE-SUITE scenarios S7 (`Covers:
  weft`) and S8 (`Covers: warp`). The dedicated fabric plumbing
  (`fabricCloneRun`/`decideFabricClone`/`runFabricSuite`/`fabric-suite`
  subcommand/`sandbox-fabric-suite.cmd`) stays; only its now-false "parallel-build /
  isolate from warp-weft" prose is cleaned up.
- Rationale: the dedicated-hub isolation existed only to keep parallel-build testing from
  touching warp/weft's shared state; at cutover there is no warp/weft state to isolate. The
  shared `lyx-test-HUB` is still needed by perch/builder/webster/burler/core suites, so its
  bootstrap must clone via fabric once `lyx warp clone` no longer exists. The dedicated
  fabric repos are already correctly set up, so re-hosting fabric onto the shared repo
  (which would need migrating to `main-weft` naming) is avoided.
- Rejected: consolidate fabric onto the shared hub (needs GitHub-side migration of the
  shared weft repo to `main-weft` naming -- more work the operator owns); keep both hubs
  with no cleanup (leaves dead isolation plumbing/prose).

### differential tests -- delete, after confirming standalone coverage

- Decision: Delete all four `fabricengine/*_differential_test.go`. Before deleting, confirm
  fabric's standalone tests (`clone_test.go`, `config_test.go`, `hook_test.go`,
  `fabric_test.go`, `reconcile`/`lifecycle`/`weftgit` standalone coverage, etc.) cover what
  the differential tests asserted; port any uniquely-covered assertion to a standalone test
  so no coverage is lost.
- Rationale: differential tests validate fabric == warp/weft on a shared fixture; they have
  served their purpose and physically cannot survive deletion of the reference module. The
  equivalence guarantee is a one-time validation, not an ongoing invariant.
- Rejected: keep them (impossible -- they import the deleted modules).

## Technical context

**Consumer call-site map (from exploration).** Most call sites are identity swaps; the only
structurally different one is `weftengine.Commit`:

| Site | Today | Fabric | Shape |
|---|---|---|---|
| `initengine/init.go:67` | `warpengine.WireJunctions(l, slug)` | `fabricengine.WireJunctions` | identity |
| `initengine/undo.go:54` | `warpengine.UnwireJunctions` (-> `UnwireResult`) | `fabricengine.UnwireJunctions` | identity |
| `initengine/undo.go:89` | `weftengine.EnvSyncOptions()` | `fabricengine.EnvSyncOptions()` | identity |
| `initengine/undo.go:90` | `weftengine.Commit(weftWorktree, ScopedPathspec(...), msg, opts)` -> `(committed, err)` | `f, err := fabricengine.New(hostPath, weftWorktree)` (check `err`) then `f.CommitWeft(pathspec, msg, opts)` -> `(sha, committed, err)` | **DIFFERS** |
| `initengine/undo.go:96` | `weftengine.Push(weftWorktree, opts)` | `fabricengine.PushWeftAt(weftWorktree, opts)` | identity shape (free func, renamed) |
| `loomengine/preflight.go:100` | `warpengine.HostClean(l)` | `fabricengine.HostClean(l)` | identity |
| `loomengine/preflight.go:120` | `warpengine.PairInSync(l)` | `fabricengine.PairInSync(l)` | identity |
| `buildercli/weft.go:35,50,53,57` | `ScopedPathspec` / `EnvSyncOptions` / `weftengine.Commit(...)` / `weftengine.Push(...)` | `fabricengine.*` + `New(host,weft).CommitWeft` / `PushWeftAt` | Commit **DIFFERS**, rest identity |
| `webstercli/weft.go:44,60,63,67` | same shape as buildercli | same | Commit **DIFFERS** |
| `perchcli/run.go:368,381,384,391` | same shape as buildercli | same | Commit **DIFFERS** |
| `configcli/configcli.go:395` | `weftcli.RunCLI(w, []string{"sync"})` | `fabriccli.RunCLI(w, []string{"sync"})` | identity signature |

**Proven pattern for the four `CommitWeft` rewrites.** `internal/fabriccli/weft_verbs.go:124-126`
already does exactly the target shape: `fab, err := fabricengine.New(l.WorktreeRoot,
l.WeftWorktree())` -- **and checks `err`** (`if err != nil { output.Err(...) }`) -- then
`fab.CommitWeft(pathspec, msg, opts)` (with an extra returned `sha` that consumers can
discard) and `fabricengine.PushWeftAt(weftPath, opts)`. Each of the four `weftengine.Commit`
sites needs the host path (the primary worktree root) in addition to the weft worktree path
to construct the `*Fabric` -- verify each call site has both in scope. **Do not discard
`New`'s error:** `New` returns `(*Fabric, error)` and yields a nil `*Fabric` when a path is
absent (`requireDir`), so a discarded error becomes a nil-deref panic in `CommitWeft`. All
four rewrite sites must propagate `New`'s error exactly as `fabriccli` does, never `f, _ :=`.

**Signature gotchas that break a naive rename:**

- `weftengine.Push(w, opts)` -> free func `fabricengine.PushWeftAt(w, opts)` (name changes).
- `weftengine.Commit(...)` -> no free func in fabric; it is `(*Fabric).CommitWeft(...)` and
  needs the host path (see above).
- `warpengine.New(cfg) *Worktree` -> `fabricengine.NewTopology(cfg) *Topology` (type changes).
- `warpengine.LoadConfig(root, "warp")` (two args) -> `fabricengine.LoadConfig(root)` (one arg).
- `weftengine.LoadConfig(root)` -> `fabricengine.LoadConfig(root)`.

**ConfigTemplate / configreg collapse.** `fabricengine.ConfigTemplate()` exists and is
**already registered** in `configreg.go:45` as module `"fabric"` (embeds merged
`branch_prefix` from warp + `pathspec` from weft). `configreg.go:51,53` still register
separate `"warp"` (`warpengine.ConfigTemplate`) and `"weft"` (`weftengine.ConfigTemplate`)
rows and import both engines (`configreg.go:18,20`). Delete both rows + both imports; the
`fabric` row already covers it. `configcli` surfaces the module list purely from
`configreg.Names()` (`configcli.go:266` "Known modules:", `:331` `ValidArgs`), so it updates
automatically once the registry changes.

**CLI registration + pinned test sets (`cmd/lyx`).** `main.go:36,38` import warpcli/weftcli;
`:124` `weftcli.Command()`, `:125` `warpcli.Command()` register them; `:89` `root.Long` lists
`... weft, warp, fabric ...`. Remove the four lines and the two names. `helptree_test.go:28`
`requiredModules` contains `"weft","warp"` and `:66-78` hold the full warp + weft subcommand
tables -- delete those cases (the fabric case already covers the union).
`registration_test.go`, `drift_test.go`, `longlist_test.go` are discovery-driven and pass
once `main.go` is consistent. `sandbox_coverage_test.go` fails on stale `Covers:` tags (see
sandbox decision). `tierpurity_test.go`/`hermeticenv_test.go` only shrink their surface and
should stay green.

**Sandbox.** `tools/sandbox/main.go:52` `cloneRun` shells `lyx warp clone` for the shared
hub -> flip to `fabric clone`. Dedicated fabric hub constants/plumbing at `main.go:30-41,
68-75, 131-231, 476-500` stay; the "warp/weft must stay untouched"/"parallel-build" comments
(`:33-35`, `:69-72`, FABRIC-SUITE `:8-10,46-52,259-261`, and the CORE-SUITE precondition
prose) become false and are cleaned up. CORE-SUITE S7 (`Covers: weft`, ~`:257`) and S8
(`Covers: warp`, ~`:280`) must be deleted or retagged (fabric already covered by
FABRIC-SUITE) or `sandbox_coverage_test.go` Assert 2 fails.

**docs/overview.md.** warp/weft rows in the module table (`~:135`, `:192-195`, `:248-249`),
the fabric parallel-build banner (`:250-257`) and its `fabric.md` link (`:257`), and intro
prose (`:7`, `:182`) all reference the old modules and must be rewritten to fabric.

**doc.go.** `internal/fabricengine/doc.go` already carries the durable rationale but retains
a parallel-build paragraph (`:9-12`) that must be cut.

**fabric.md inbound links (9 to repoint + 1 roadmap Done):** `board-weft-storage.md`,
`raddle.md`, `loom-finalize.md`, `host-visibility.md`, `codeintel-redesign.md`,
`docs/overview.md`, `docs/reference/plan-format-v3.md`, `crucible/fabric-review-prompt.md`
(`:58,:288,:417`), `crucible/gitrepo-review-prompt.md` (`:72,:152`) -> repoint to `doc.go`;
`manifest/roadmap.md:20` -> becomes the Done entry.
(`crucible/` is durable review-prompt scaffolding, not ephemeral -- it is in the repoint
scope; `tools/sandbox/SANDBOX-FABRIC-SUITE.md`'s link is handled by the FABRIC-SUITE
de-parallel-build item, not counted here.)

**Transition-window note (harmless).** While some consumers still use `weftengine.Commit`
(no `Warp-SHA:` trailer) and others use `fabric.CommitWeft` (trailer + `RecordCorrespondence`),
weft history is a mix of trailered/untrailered commits. This is design-tolerated:
`RebuildIndex` skips untrailered commits and nearest-older lookups report gaps explicitly.
The mixed state exists only in dev/test within the one PR, never in a released state.

**Complete importer list (verified, non-test):** `cmd/lyx/main.go` (warpcli/weftcli),
`configreg/configreg.go` (both engines), `configcli/configcli.go` (weftcli),
`initengine/init.go` + `undo.go`, `loomengine/preflight.go`, `buildercli/weft.go`,
`webstercli/weft.go`, `perchcli/run.go`. No other production importers exist -- other grep
hits are comments or `hubgeometry` geometry helpers, not module imports.

## Constraints

From `CONSTRAINTS.md` (this cutover must update the ones that reference warp/weft, in the
same commit per the repo's Documentation Lifecycle / task-completion rule):

- **Weft Git Invariant** -- currently names "`weftengine` **or** `fabricengine`" and
  "`warpengine` **or** `fabricengine`" as dual owners during parallel build, with a note
  that the dual ownership "lasts only until the warp/weft cutover task." This task collapses
  both bullets to name **only** `internal/fabricengine`.
- **lyxtest Leaf Invariant** -- its example list of banned feature packages names
  `warpengine`/`warpcli`/`weftengine`/`weftcli`. Remove those (keep fabricengine/fabriccli).
  `internal/lyxtest/leaf_enforcement_test.go`'s `bannedImports` list is a string-match walk
  (compiles even after the packages are gone) but is stale and must be updated for accuracy.
- **CLI / Cobra Invariant** -- update the pinned sets in `cmd/lyx/{helptree,registration,
  longlist,drift}_test.go` in the same commit as the registration change; every remaining
  command keeps a non-empty `Short`; re-read the affected `Short`/`Long` for accuracy.
- **Sandbox Suite Coverage** -- exists => covered-or-excluded, checked against the live
  cobra root; removing warp/weft from `newRoot()` requires removing their `Covers:` tags in
  the same change. fabric stays covered by FABRIC-SUITE.
- **Hub Geometry Invariant** -- unaffected: fabric already routes cwd/geometry through
  `hubgeometry`; do not introduce raw geometry tokens during the rewires.
- **Documentation Lifecycle** -- fabric.md is the mechanical-vs-durable case: durable
  rationale lives in `doc.go` (kept), the design doc is deleted at cutover.

No new cross-cutting invariant is introduced by this task, so no addition to `CONSTRAINTS.md`
beyond the edits above.

## Testing

**Verification strategy (per user).** During each batch, run **targeted** integration tests
for just the touched module(s) plus the enforcement-test packages the change affects (they
are slow in aggregate). Run the **full** suite once at the very end as the acceptance gate.

- **Batch A:** `go test -tags integration ./internal/{initengine,loomengine,buildercli,webstercli,perchcli}/...`
- **Batch B:** `go test -tags integration ./internal/{configreg,configcli}/...`
- **Batch C:** `go test ./cmd/lyx/...` (help-tree, registration, longlist, drift,
  sandbox-coverage, tierpurity, hermeticenv guards).
- **Batch D:** delete + docs, then the **final gate:** `go build ./...` &&
  `go test ./... -tags integration` all green, plus the grep-clean gate below.

**Grep-clean gate (end of D) -- two tiers.** The words "warp"/"weft" legitimately survive in
fabric's own API (`Warp`/`Weft` fields, `Warp-SHA`, `WeftBranchName`, `CommitWeft`,
hubgeometry `-weft`) and in descriptive prose, so a blanket "zero occurrences" gate is
wrong. Instead:

- **Tier 1 (hard, zero matches):** no import of
  `github.com/Knatte18/loomyard/internal/{warpengine,warpcli,weftengine,weftcli}` in any
  `.go` file; no `lyx warp`/`lyx weft` CLI registration (`warpcli.Command()`/`weftcli.Command()`);
  no `warp`/`weft` config-module identifiers in `configreg`. This is the acceptance blocker.
- **Tier 2 (soft sweep, reviewed):** `grep -rn -E 'internal/(warp|weft)(cli|engine)'` over
  `//`-comment lines and docs -- every hit that *names a deleted module* is repointed to
  fabric (see the comment-sweep in Batch D); bare "warp"/"weft" words that describe fabric's
  weft repo/role are allowed to remain. Tier 2 is a review obligation, not a hard zero-match,
  precisely because a hard match on `internal/weftcli` would flag legitimate comment prose.
- **Tier 2b (parallel-build prose, scoped to fabric's own module):**
  `grep -rn -iE 'parallel[- ]build' internal/fabricengine/ internal/fabriccli/` -- every
  "parallel-build period" comment/help string describing fabric as *coexisting with the live
  warp/weft modules* is rewritten (fabric is now the sole owner). **Exclude the
  test-concurrency false positives** that also match "parallel": `t.Parallel()` and "parallel
  to Add's logic" at `internal/fabricengine/{add.go:25,46, reconcile.go:269}` and
  `internal/fabriccli/fabric.go:542` -- those describe Go test concurrency, not the
  parallel build, and stay untouched. Reviewed, not hard zero-match.

**Stale full-path comment refs to repoint in Batch D (verified, not scheduled elsewhere):**
`internal/lyxtest/doc.go:2-3` (lists all four deleted packages),
`internal/hubgeometry/hubgeometry.go:700` ("seeders in internal/warpengine"),
`internal/codeintelcli/cli.go:34` ("see internal/weftcli.Command"), and
`cmd/lyx/main.go:92` (bare-word comment "board, ide, reed, weft" -- drop the stale "weft"
when registration is removed; sits at the Batch C registration edit). Plus a bare-name
review sweep of `internal/perchengine/{doc.go,engine.go}`, `internal/websterengine/audit.go`,
`internal/reedengine/config.go`, `internal/lyxtest/hermetic.go` -- repoint only where they
name the deleted *modules*, leave where they describe the weft repo concept. These are
comments (build-safe), but the Documentation-Lifecycle "no rot" rule and the Tier-1 gate
both require the module-naming ones to be fixed.

**Fabric's own module de-parallel-build (Tier 2b, Batch D).** `internal/fabriccli/fabric.go`
carries the last live `manifest/designs/fabric.md` reference (`:54`, in a `.go` Long string --
missed by a docs-only grep) plus parallel-build framing in its `Short` (`:40`) and `Long`
(`:12,:53,:218`): rewrite these to present fabric as the sole git-coordination module and drop
the `fabric.md` ref (this is also a CLI/Cobra help-accuracy obligation, not just a comment).
Sweep the genuine parallel-build-period comments in `internal/fabricengine`
(`clone.go:23`, `cleanup.go:14/63/158`, `fabric.go:96`, `hook.go:9`, `weftgit.go:8/27`).
Do **not** touch the `t.Parallel()`/"parallel to Add's logic" mentions (see Tier 2b exclusions).

**Tests to rewrite onto fabric (not delete):**

- `internal/initengine/init_test.go` -- imports both engines; loops `["board","warp","weft"]`
  and calls `warpengine.LoadConfig(root,"warp")` / `weftengine.LoadConfig(root)`. Rewrite to
  `["board","fabric"]` + `fabricengine.LoadConfig(root)`.
- `internal/loomengine/preflight_integration_test.go` -- `warpengine.WireJunctions` fixture
  setup -> `fabricengine.WireJunctions`.
- `internal/configreg/configreg_test.go` -- pins the module-name list (drop `warp`,`weft`)
  and asserts `weftengine.ConfigTemplate()` (re-point to fabric or drop).
- `internal/configcli/configcli_integration_test.go` -- builds its fixture via
  `warpengine.New().Add()` + `warpengine.WireJunctions` + `weftcli.RunCLI` and dispatches the
  `"warp"` config module. Rewrite onto `fabricengine.NewTopology().Add()` + `fabriccli` +
  the `"fabric"` module.

**Tests to delete (import the module under test; cannot survive):**

- All of `internal/warpengine/*_test.go`, `internal/warpcli/*_test.go`,
  `internal/weftcli/*_test.go`, `internal/weftengine/*_test.go` (deleted with their packages).
- The four `internal/fabricengine/{clone,lifecycle,reconcile,weftgit}_differential_test.go`
  -- **but first** confirm their assertions are covered by fabric's standalone tests; port
  any unique assertion to a standalone test before deleting (see the differential-tests
  decision). fabricengine's other tests (`fabric_test.go`, `template_test.go`, `hook_test.go`,
  `clone_test.go`, `config_test.go`, etc.) only mention warp/weft in comments and survive.

**No new TDD candidates.** This is a rewire of behaviour-preserving call sites guarded by an
existing strong suite + the Go compiler; the safety net is `go build` + `go test`, not new
test-first code. New test writing is limited to (a) rewriting the four integration tests
above onto fabric and (b) any standalone assertion ported off a differential test.

## Q&A log

- **Q:** Is this a mechanical rename like the previous module rename, solvable with one
  script? **A:** No -- fabric's API does not line up 1:1 with warp/weft (Push renamed, Commit
  restructured, Worktree->Topology, LoadConfig arity), so it is a hybrid: scriptable
  deletion/verify spine + a semantic core of ~10 hand-edited files.
- **Q:** Staging -- green at each step or one atomic switch? **A:** Green at each step, one PR.
- **Q:** Is incremental staging safe given shared weft state? **A:** Yes -- no in-process
  state across calls, fabric is a validated superset, and mixed trailered/untrailered weft
  history in the transition window is design-tolerated and dev/test-only.
- **Q:** How much to lean on scripts? **A:** Use a script for anything specifiable without
  reading the file (the big token/cost win is not opening files); the compiler + `go test`
  are the safety net. LLM opens files only for the semantic edits.
- **Q:** Config-module collapse -- alias/migration or clean removal? **A:** Clean removal;
  warp/weft config modules fully phased out; no alias, no on-disk migration (regenerate via
  `lyx init`).
- **Q:** fabric.md -- delete, stub, or keep? **A:** Delete; add a short Done entry in
  roadmap.md (no link); repoint the nine other inbound links (incl. the two `crucible/`
  review prompts) to `doc.go`. The code is the
  source of truth once the module is built.
- **Q:** Sandbox -- which hub survives? **A:** Fabric's dedicated hub survives as the fabric
  suite (already seeded for `main-weft` naming); flip the shared-hub bootstrap `cloneRun`
  from `warp clone` to `fabric clone`; remove CORE-SUITE S7/S8; de-parallel-build the prose.
- **Q:** Differential tests -- keep or delete? **A:** Delete all four (they import the
  deleted reference modules); first confirm/port coverage to fabric's standalone tests.
- **Q:** Verification -- full suite each time? **A:** Targeted integration tests per touched
  module during work; full suite once at the end as the acceptance gate.
- **Q:** "Update ALL warp/weft references" -- does that include fabric's own API? **A:** No.
  Eliminate the module imports, `lyx warp`/`lyx weft` CLI, and `warp`/`weft` config
  identifiers; keep fabric's own `Warp`/`Weft` fields, `Warp-SHA`, `WeftBranchName`,
  `CommitWeft`, and hubgeometry `-weft` geometry.
- **Q:** (review r1 gap) Stale full-path deleted-module refs survive in production comments
  (`lyxtest/doc.go`, `hubgeometry.go:700`, `codeintelcli/cli.go:34`) and a naive grep gate
  would flag them -- how to handle comments + gate scope? **A:** Add a comment-sweep to
  Batch D repointing module-naming comments to fabric, and define the acceptance gate as
  two tiers -- Tier 1 hard zero-match on import paths + CLI + config identifiers, Tier 2 a
  reviewed soft sweep of comment refs (fix where they name deleted modules, leave where they
  describe fabric's weft repo/role).
- **Q:** (review r2 gap) The `fabric.md` inbound-link list said "seven" but
  `crucible/fabric-review-prompt.md` and `crucible/gitrepo-review-prompt.md` also link to it
  -- repoint or scope out? **A:** Repoint both; `crucible/` is durable review-prompt
  scaffolding, so the count is nine repointed links, zero dangling.
- **Q:** (review r3 gaps) Fabric's own module still describes the parallel-build world --
  `fabriccli/fabric.go` `Short`/`Long` + a live `fabric.md` ref (`.go` string, missed by the
  md-grep), and "parallel-build period" comments across `fabricengine`/`fabriccli`. In scope?
  **A:** Yes -- de-parallel-build fabric's own module in Batch D (Tier 2b), rewriting the CLI
  help + dropping the `fabric.md` ref + sweeping the genuine parallel-build comments, while
  explicitly excluding `t.Parallel()`/"parallel to Add" test-concurrency false positives.
```
