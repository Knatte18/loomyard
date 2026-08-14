# fabric — independent review (round 7, `fable-high-r7`)

Clean-room, adversarial review of `internal/fabricengine` + CLI surface, scoped by the round-7 prompt
as a **full write-side containment audit** (not a `writeLaunchers`-only point fix). Formed independently
before consulting any prior `fabric-review-*` material.

## Executive summary

The campaign's create-side chain (rounds 2–6) is genuinely closed for the *delete* side and for
`containedWorktreeAdd`'s worktree placement. But the round-7 hypothesis holds: five rounds of pressure on
one call path left the two OTHER hub-level structural-container writers on the same `add` code path with
**zero containment protection**. Both write to a persistent hub-root directory (`_launchers`, `_portals`)
that an attacker can pre-plant a *static* symlink at — no race, no observation — and both then report
`ok:true` with a mutation record claiming a hub-relative write while the bytes/link landed OUTSIDE the hub.

- **F1 (MEDIUM, CONFIRMED)** — `writeLaunchers` (`launchers.go`) writes `ide.sh`/`fabric-checkout.sh`/
  `ide-menu.sh` through raw `os.MkdirAll`+`os.WriteFile` to `<hub>/_launchers/<AnchorRel>/<slug>`. A static
  symlink at either the `<slug>` leaf OR the `_launchers` container carries executable script content to an
  attacker-chosen path. This is the delete-side M3's twin, strictly easier to exploit (no timing).
- **F2 (LOW, CONFIRMED)** — `createPortal` (`portals.go`) routes through `fslink.CreateDirLink`, whose
  `prepareLink` refuses only a leaf clobber; a static symlink at the `_portals` container (or an intermediate
  `<AnchorRel>` segment) makes it `MkdirAll`+`Symlink` a portal junction OUTSIDE the hub, `ok:true`. Lower
  blast radius than F1 (a dangling out-of-hub symlink, not overwritten content), same false-success class.
- **F3 (LOW/scope, CONFIRMED-by-absence)** — there is no write-side equivalent of
  `cmd/lyx/destructiveguard_test.go`. Nothing mechanically inventories the package's raw filesystem-WRITE
  primitives, which is exactly why F1/F2 sat undiscovered for five rounds. Building one is this round's most
  durable deliverable.
- **F4 (LOW/doc-accuracy, CONFIRMED)** — `createExclusiveDir`'s doc (and the CONSTRAINTS invariant line)
  claims it refuses an intermediate-ancestor symlink escape; empirically it does not (it roots at the
  parent, which `os.OpenRoot` resolves). Not a live exploit at its sole call site, but a false containment
  claim in an authoritative doc. Found during Job 2, corrected in the doc + a dedicated leaf-refusal test.

The remaining raw-write sites (junction target materialisation, `_board` link, clone `.lyx`/anchor marker,
warp binding, weft lock dir, git hooks / `info/exclude`) are **NOT** in the same exploit class: each writes
into either a git-owned `.git/…` path or a worktree/board directory fabric just minted via a *contained*
minter in the same call, where only a post-creation same-UID race — the documented residual class, same as
the gate's dirtiness window — could redirect them, never a static pre-plant. `add.go`'s own
`os.Stat(target)` pre-check plus `containedWorktreeAdd`'s fail-closed creation forbid pre-planting the
worktree path. I classify these as accepted, documented residuals (not fixed), consistent with the
campaign's existing treatment of post-contained-create race windows.

**Merge-readiness: NOT mergeable until F1/F2/F3 land.** F1 is a real create-side containment/false-success
defect of exactly the class this whole campaign exists to eliminate. Windows path behaviour remains the
permanent out-of-scope limit. N4's dirtiness-probe TOCTOU stays an accepted residual (not re-attempted).

## Scope assessment (plan-vs-shipped)

No scope drift found in the audited surface. The one-repo illusion, the destruction chokepoint, the
mutation-record contract, and the correspondence-index write path are all as `doc.go` describes. The gap is
not scope — it is an asymmetry between the delete side (fully contained: `removeLaunchers`/`removePortal`
route through `refuseUncontainedPath` + `removeContainedPath`/`removeLink` on `os.Root`) and the create side
of the same two hub-level containers (`writeLaunchers`/`createPortal`), which never grew the matching guard.

## Code findings (severity-ranked)

### F1 — `writeLaunchers` writes hub-relative script content with zero containment (MEDIUM, CONFIRMED)
`internal/fabricengine/launchers.go:98-168` (`writeLaunchers`), specifically the raw
`os.MkdirAll(launcherDir, …)` at :107, `os.MkdirAll(filepath.Dir(menuPath), …)` at :145, and the
`os.WriteFile` sites at :162/:178 (`writeLauncherScriptIfChanged`).

Scenario (reproduced live, 100% first attempt, no timing): with hub write access, plant
`ln -s <outside> <hub>/_launchers/<slug>` (or `ln -s <outside> <hub>/_launchers`) BEFORE `lyx fabric add
<slug>`. `os.MkdirAll` follows the symlink-to-directory and `os.WriteFile` writes `ide.sh` +
`fabric-checkout.sh` into the attacker's target. `add` returns `ok:true`, `partial:false`, and a mutation
record claiming `file_written _launchers/<slug>/ide.sh` — the delete-side M3 false-success shape.

Impact: executable script content written to an attacker-chosen path (can overwrite a victim's files); the
mutation record and CLI envelope both lie about where the write landed; contained rollback cannot clean up
the out-of-hub debris.

Fix: route every `writeLaunchers` write through an `os.Root` rooted at `l.HubPath` (the true containment
boundary; `_launchers` is a structural directory directly under it), mirroring `createExclusiveDir`'s
create-side pattern — `root.MkdirAll`/`root.WriteFile`/`root.Stat`/`root.ReadFile` refuse any component
escaping the hub at write time. Preserve the record semantics (stat-before for `dir_created`,
read-before-write for `file_written`).

### F2 — `createPortal` creates an out-of-hub junction via an uncontained `_portals` (LOW, CONFIRMED)
`internal/fabricengine/portals.go:46-54` (`createPortal` → `fslink.CreateDirLink`).

Scenario (reproduced live): plant `ln -s <outside> <hub>/_portals` before the first `add`.
`fslink.prepareLink` Lstat's only the LEAF (`_portals/<AnchorRel>/<slug>`), so the container symlink is not
caught; `os.MkdirAll(filepath.Dir(link))` follows it and `os.Symlink` creates the portal junction outside
the hub. `add` reports `ok:true` and records `link_created _portals/<slug>`. The LEAF vector is refused
(prepareLink's clobber guard — verified: "link already exists"), so only the container/intermediate vector
escapes.

Impact: a dangling out-of-hub symlink + false success; lower than F1 (no content overwrite). Contained
rollback (`removePortal`) will not reach outside to clean it.

Fix: add a front-of-function containment assertion for `createPortal` that rejects a `_portals` (or
intermediate) symlink escaping the hub before the link is created — the create-side counterpart of
`removePortal`'s `refuseUncontainedPath`, but rooted so the container-is-symlink case is caught (plain
`refuseUncontainedPath` resolves the container itself and would miss it). Realised by materialising the
portal's parent chain through an `os.Root` rooted at `l.HubPath` and asserting the leaf's parent is
contained before handing the leaf to `fslink`.

### F3 — no write-side raw-primitive guard test exists (LOW / scope, CONFIRMED)
There is a delete-side inventory (`cmd/lyx/destructiveguard_test.go`'s
`TestNoDestructiveBypass_FabricengineProductionSource`) but no write-side equivalent. A machine-checked
inventory of every raw filesystem-WRITE primitive under `internal/fabricengine`, each cross-checked against
an explicit allowlist (safe-raw-with-reason vs. must-route-through-containment), is what prevents an eighth
round chasing a ninth link. Fix: add `TestNoUncontainedWrite_FabricengineProductionSource`, modelled on the
delete-side guard, banning `os.MkdirAll(`/`os.Mkdir(`/`os.WriteFile(`/`os.Create(`/`os.OpenFile(`/
`os.Symlink(`/`os.Link(` outside an allowlist that documents each safe raw site.

### F4 — `createExclusiveDir`'s doc overstates its containment guarantee (LOW / doc-accuracy, CONFIRMED)
`internal/fabricengine/destroy.go:886-917` (`createExclusiveDir` doc comment) and `CONSTRAINTS.md`'s
Fabric Destruction Chokepoint Invariant line for the create-side minters.

Found during Job 2 while re-evaluating round 5's F2/F3 "no dedicated test" item (my own experiment, not
from any prior review). The doc claims rooting the `Mkdir` at path's parent "atomically refuses any
intermediate component escaping the parent … a planted-intermediate-symlink escape is refused rather than
followed." Empirically FALSE on Go 1.26: `createExclusiveDir` roots at `filepath.Dir(path)` and creates
only `filepath.Base(path)` through the root, so `os.OpenRoot` resolves the parent argument (symlinks
included) BEFORE rooting. A symlink planted in path's parent ancestry is followed exactly as `os.Mkdir`
would follow it — a scratch test creating `<container>/escape -> <outside>` then
`createExclusiveDir(<container>/escape/leaf)` created `<outside>/leaf` with a nil error. Only a symlink at
the LEAF (path's final component) is refused (EEXIST, verified).

Not a live exploit: the sole caller (`clone.go`'s `createExclusiveDir(rec, hubPath)`) passes a
single-component hub leaf under the operator-chosen clone parent — an operator-controlled location, not a
path an attacker can plant a symlink inside the way it can inside a live hub. But an authoritative
invariant doc asserting a containment property the code does not provide is a latent trap: a future caller
passing an attacker-influenced parent ancestry, trusting the claim, would escape. Fix: correct the doc
(destroy.go + CONSTRAINTS.md) to state the accurate guarantee (leaf-only refusal; parent ancestry resolved
by OpenRoot; safe here only because the hub leaf sits under the operator-controlled clone parent) and add
the round-5-F2/F3 dedicated leaf-symlink-refusal regression test that was missing.

## Prior-round minor items (re-evaluated)

- **Round 4 F2 (WARN-log test-coverage claim): NOT actually open — round 6 was right.**
  `TestAddRollback_RefusedWarpBranchDeletionLogsWarn` (`add_rollback_adopt_test.go:191`) rebinds the logger
  sink via `logger.SetOutput` and asserts the exact WARN line, so reverting `rollbackAdd`'s `logger.Warn`
  hunk fails it. It genuinely sabotage-proves the log — empirically confirmed in Job 2: neutralising
  `rollbackAdd`'s `logger.Warn` hunk makes the test FAIL at add_rollback_adopt_test.go:219. No fix needed.
- **Round 5 F2/F3 dedicated tests:** to be assessed in Job 2; round 6's `containedWorktreeAdd` tests already
  exercise the shared helper heavily. Low value; note in fixer report.

## Accepted residuals (considered, deliberately not fixed)

- Worktree-internal writes after `containedWorktreeAdd` (`seedLyxJunction`'s `os.MkdirAll(target)` into the
  weft worktree; `wireBoardLink`; junction link creation): race-only, not statically pre-plantable
  (`add.go:83` refuses a pre-existing worktree path; the minter is fail-closed). Same residual class as the
  gate's documented dirtiness window.
- Clone-bootstrap writes (`clone.go` `.lyx` dir + `.lyx-anchor` marker; `warpbinding.go` `.lyx-warp`;
  `weftgit.go` `.weft`): hub minted via `createExclusiveDir` and board/weft worktrees via
  `containedWorktreeAdd` in the same call; race-only.
- Git-owned writes (`hook.go` into git-resolved hooks dir; `gitexclude.go` `.git/info/exclude`): paths
  resolved by git, never derived from an operator slug; outside the hub-symlink threat model.
- N4 dirtiness-probe TOCTOU: accepted since round 3, not re-attempted.
- Windows path behaviour: permanent never-executed gap on a Linux host.

## What was tested

(Appended incrementally; commands + observed results below.)

- `go build ./...` → OK.
- `go vet ./internal/fabricengine/... ./internal/fabriccli/...` → OK.
- `go test ./internal/fabricengine/... ./internal/fabriccli/... ./cmd/lyx/... -count=1` → all `ok` (baseline green).
- LIVE repro F1 (`repro_launchers.sh`): built local warp bare + empty weft bare, `lyx fabric clone`,
  planted `ln -s $EVIL <hub>/_launchers/pwned`, `lyx fabric add pwned --json` → `ok:true`, `ide.sh` +
  `fabric-checkout.sh` landed in `$EVIL` (outside hub), record claimed `_launchers/pwned/ide.sh`. CONFIRMED.
- LIVE repro siblings (`repro_siblings.sh`): Test A planted `_portals`→EVIL, `add` created portal junction
  `task1 -> …/task1/_lyx` inside EVIL, `ok:true` (F2 CONFIRMED). Test B planted `_launchers`→EVIL, all three
  scripts landed in EVIL (F1 via container vector CONFIRMED). Test C planted `_portals/task3` leaf symlink →
  refused ("link already exists"), `add` rolled back (portal leaf is protected).
