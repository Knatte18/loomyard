# fabric — fixer report (round 7, `fable-high-r7`)

All four findings from `_mill/fabric-review-fable-high-r7.md` are implemented, verified against the real
substrate, and committed one-per-fix on branch `fabric-crucible-hardening` (not pushed). The full test
suite is green hermetically (repo-wide `go test ./...`, and the audited packages at `-count=5`), under the
full integration suite, and under 4× concurrent integration copies (all PASS, zero substrate markers).

## Findings fixed

### F1 (MEDIUM) — `writeLaunchers` uncontained hub-relative writes
- **Change:** `internal/fabricengine/launchers.go` — `writeLaunchers` and `writeLauncherScriptIfChanged`
  now perform every mkdir/write through an `os.Root` rooted at `l.HubPath` (added a `hubRel` helper),
  so any component of `_launchers/<AnchorRel>/<slug>/…` escaping the hub is refused at write time. Doc
  comments reworded to describe the rooted flow.
- **Test:** `internal/fabricengine/launchers_containment_integration_test.go`
  (`TestAdd_DoesNotWriteOutsideHubThroughLauncherSymlink`, leaf + container subtests): asserts Add fails
  closed, no script lands outside the hub, and no false `file_written` launcher record.
- **Verification:** live repro (`repro_launchers.sh`, `repro_siblings.sh` Test B) before the fix wrote
  `ide.sh`/`fabric-checkout.sh`/`ide-menu.sh` into an out-of-hub dir with `ok:true`; after the fix both the
  leaf and container vectors fail closed (`mkdirat _launchers/<slug>: file exists` / `path escapes from
  parent`), out-of-hub dir empty. Subpath-anchor vector (symlink at `_launchers/backend`) also fails
  closed (`path escapes from parent`). Sabotage-proved: reverting to raw `os.MkdirAll`/`os.WriteFile`
  fails both subtests. Clean add + reconcile repair path unaffected (launchers land inside the hub, 0755).

### F2 (LOW) — `createPortal` uncontained junction via fslink
- **Change:** `internal/fabricengine/portals.go` — `createPortal` now calls `ensureContainedLinkParent`
  (new helper) to materialise the portal link's parent chain through an `os.Root` rooted at `l.HubPath`
  before handing the leaf to `fslink.CreateDirLink`, so an escaping `_portals`/intermediate symlink is
  refused at mkdir time. A comment mention of the raw token was reworded so the fixed file carries no
  banned token.
- **Test:** `internal/fabricengine/portals_containment_integration_test.go`
  (`TestAdd_DoesNotCreatePortalOutsideHubThroughContainerSymlink`).
- **Verification:** live repro (`repro_siblings.sh` Test A) before the fix created a portal junction in an
  out-of-hub dir with `ok:true`; after, it fails closed (`mkdirat _portals: file exists`), out-of-hub dir
  empty. The leaf vector remains refused by fslink's clobber guard (Test C). Sabotage-proved: removing the
  `ensureContainedLinkParent` call makes the test fail. Clean portal wiring unaffected.

### F3 (LOW/scope) — no write-side raw-primitive guard
- **Change:** added `cmd/lyx/uncontainedwrite_test.go`
  (`TestNoUncontainedWrite_FabricengineProductionSource`), the write-side twin of the destruction
  chokepoint guard: bans `os.MkdirAll(`/`os.Mkdir(`/`os.WriteFile(`/`os.Create(`/`os.OpenFile(`/
  `os.Symlink(`/`os.Link(` under `internal/fabricengine` outside a per-file allowlist that names why each
  remaining raw write is safe (git-owned path, or a directory a contained minter created in the same
  call). The two fixed writers carry no banned token (they route through `os.Root`) and are deliberately
  absent from the allowlist. Recorded as CONSTRAINTS.md's **Fabric Write-Side Containment Invariant** and
  in `doc.go`'s destruction-chokepoint section (same commit). Allowlisted the guard file in
  `cmd/lyx/tierpurity_test.go` (it spawns `go env GOMOD` and carries its banned tokens as scan data,
  exactly like the delete-side guard).
- **Verification:** guard passes; sabotage-proved — reintroducing a raw `os.WriteFile` in the
  non-allowlisted `launchers.go` makes the guard fail with the expected message.

### F4 (LOW/doc-accuracy) — `createExclusiveDir` overstated its containment guarantee
- **Change:** corrected the `createExclusiveDir` doc (`internal/fabricengine/destroy.go`) and the
  CONSTRAINTS invariant line to state the accurate guarantee: rooting at path's parent refuses a symlink
  at the LEAF it creates (EEXIST), but `os.OpenRoot` resolves the parent argument, so an intermediate
  ancestor symlink is NOT refused — safe only because the sole caller (CloneHub) mints a single-component
  hub leaf under the operator-chosen clone parent; a caller with attacker-influenced parent ancestry must
  use the fixed-container rooting pattern (`removeContainedPath`/`containedWorktreeAdd`) instead.
- **Test:** added `TestCreateExclusiveDir_RefusesLeafSymlink` (`destroy_toctou_test.go`) — the dedicated
  leaf-refusal regression round 5's F2/F3 lacked. Sabotage-proved: regressing `createExclusiveDir` to
  `os.MkdirAll` (which follows a leaf symlink-to-dir) makes it fail.
- **How found:** during Job 2, re-evaluating round 5's "no dedicated test" item, a scratch experiment
  showed `createExclusiveDir(<c>/escape/leaf)` with `escape -> outside` created `outside/leaf` with a nil
  error — the doc claimed that escape was refused. Not a live exploit (the sole call site is safe), but an
  authoritative-doc inaccuracy corrected in the same class as rounds 3/4's accuracy corrections.

## Prior-round items re-evaluated (no fix needed)

- **Round 4 F2 (WARN-log test claim):** round 6 was right — `TestAddRollback_RefusedWarpBranchDeletionLogsWarn`
  genuinely sabotage-proves the log. Empirically confirmed: neutralising `rollbackAdd`'s `logger.Warn`
  hunk makes the test FAIL at add_rollback_adopt_test.go:219. No fix.
- **Round 5 F2/F3 (missing dedicated test):** addressed as part of F4 — added
  `TestCreateExclusiveDir_RefusesLeafSymlink`, the dedicated regression for the shared create-side minter.

## Accepted residuals (considered, deliberately NOT fixed — with reasons)

- **Worktree-internal writes after `containedWorktreeAdd`** (`seedLyxJunction`'s `os.MkdirAll(target)`,
  `wireBoardLink`, junction link creation) and **clone-bootstrap writes** (`clone.go` `.lyx`/anchor marker,
  `warpbinding.go`, `weftgit.go`): race-only, not statically pre-plantable — `add.go:83` refuses a
  pre-existing worktree path and the minters (`createExclusiveDir`/`containedWorktreeAdd`) are fail-closed,
  so an attacker cannot pre-plant a static symlink; only a post-creation same-UID race could redirect them,
  the same accepted residual class as the gate's documented dirtiness window. Each is an allowlisted,
  reasoned entry in the F3 guard rather than a routed write.
- **Git-owned writes** (`hook.go` hooks dir, `gitexclude.go` `.git/info`): paths resolved by git, never
  derived from an operator slug; outside the hub-symlink threat model. Allowlisted in the F3 guard.
- **N4 dirtiness-probe TOCTOU** and **Windows path behaviour**: unchanged accepted residual / permanent
  out-of-scope limit, not re-attempted.

## Test commands run + results

- `go build ./...` → OK.
- `go vet ./internal/fabricengine/... ./internal/fabriccli/... ./internal/gitexec/... ./internal/gitrepo/...` → OK.
- `go test ./internal/fabricengine/... ./internal/fabriccli/... ./internal/gitexec/... ./internal/gitrepo/... ./cmd/lyx/... -count=5` → all `ok`.
- `go test ./...` (repo-wide hermetic) → ALL PASS.
- `go test -tags integration ./internal/fabricengine/... ./internal/fabriccli/... ./internal/gitexec/... ./internal/gitrepo/... -count=1` → all `ok`.
- N× concurrent integration (compile once, 4 copies, `-test.parallel=8`) → 4/4 PASS, zero substrate
  markers (the only `FAIL` grep hits are the test slug `push-fail`, case-insensitive).
- `TestHermeticGitEnv_GitSpawningPackagesHaveTestMain`, `TestTierPurity_UntaggedTestsSpawnNothing`,
  `TestEnforcement_FabricVocabulary`, `TestNoDestructiveBypass_FabricengineProductionSource`,
  `TestMutationRecord_FabricengineProductionSource`, `TestNoUncontainedWrite_FabricengineProductionSource`
  → all green.
- Live driving: `./deploy-dev` after every source change; F1/F2 vectors (leaf, container, subpath-anchor)
  re-attacked against the deployed binary — all fail closed, zero out-of-hub artifacts; clean add +
  reconcile repair verified working.

## Teardown

All scratch hubs / repro temp dirs / EVIL dirs removed. Confirmed zero stray git processes and zero
leftover fabric lock files outside torn-down temp dirs.

## Could NOT verify

- Windows directory-junction behaviour (`os.Root` on Windows, fslink junctions): permanent
  never-executed gap from a Linux host, same limit as every prior round. The `os.Root`-based fixes rely on
  the same kernel `openat` semantics the delete-side R3/create-side R5/R6 fixes already rely on, which the
  repo's own testing likewise cannot exercise on Windows.

## Changed files

- `internal/fabricengine/launchers.go` (F1)
- `internal/fabricengine/launchers_containment_integration_test.go` (F1, new)
- `internal/fabricengine/portals.go` (F2, F3-reword)
- `internal/fabricengine/portals_containment_integration_test.go` (F2, new)
- `cmd/lyx/uncontainedwrite_test.go` (F3, new)
- `cmd/lyx/tierpurity_test.go` (F3 allowlist)
- `internal/fabricengine/doc.go` (F3, F4 docs)
- `CONSTRAINTS.md` (F3 invariant, F4 correction)
- `internal/fabricengine/destroy.go` (F4 doc)
- `internal/fabricengine/destroy_toctou_test.go` (F4 test, new function)
- `_mill/fabric-review-fable-high-r7.md`, `_mill/fabric-review-fable-high-r7-fixer-report.md` (deliverables)

## Merge-readiness

**MERGEABLE.** The two confirmed create-side containment defects (F1 executable-content escape, F2 portal
escape) are closed and re-attacked; the durable write-side guard (F3) prevents the next uncontained write;
the doc inaccuracy (F4) is corrected with a dedicated regression test. Windows path behaviour is the
permanent stated limit; N4's dirtiness-probe TOCTOU remains the accepted documented residual.
