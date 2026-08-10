# Batch: path-callsites

```yaml
task: 'fabric: one ownership-and-dirtiness gate for all destruction (slice 12)'
batch: 'path-callsites'
number: 3
cards: 7
verify: go test ./internal/fabricengine/... && go test -tags integration ./internal/fabricengine/...
depends-on: [2]
```

## Batch Scope

This batch routes every path-shaped and link-shaped destructive call site in `internal/fabricengine` onto batch 2's executors, except the two inside `clone.go` (batch 4) and the four `git branch -D` sites (batch 5).
That is `Remove`'s worktree removal and its re-gated fallback, `Prune`'s stale-pair removal, the launcher and portal teardowns, the weft worktree removal, the junction-record removal that has no containment check today, and the four link teardowns plus two link re-points.
It is one batch because these sites share one shape — declare a container, an ownership kind and a dirtiness member, then call an executor — and because two of the three gaps the slice closes live here and are only meaningful together with the conversions around them.

It runs in parallel with batch 4, which touches only `clone.go`;
there is no file overlap.
Batch 5 depends on this batch because both edit `internal/fabricengine/weftwiring.go`.

Batch-local decisions beyond `## Shared Decisions`:

- Two helper signatures gain a container parameter, because a gated site cannot declare containment against a parent it never receives.
  `removeJunctionRecords` gains a leading `container string`;
  `unseedJunctionRecords` gains the same.
  Both callers have `l` and `slug` in scope and pass `WorktreePath(l, slug)`.
- Container derivation follows the discussion's single rule rather than a per-site table: the container is the fabric-geometry parent the target is supposed to live under.
  For a worktree or hub child that is `l.HubPath`;
  for a launcher target it is the launchers directory;
  for a portal link it is the portals directory;
  for a junction inside a pair it is that pair's warp worktree root.
- Nothing in this batch changes a dirtiness scope.
  Every gated row carries the member the discussion's disposition tables name, which is the scope that site probes today.

## Cards

### Card 13: gate Remove's worktree removal and re-gate its fallback

- **Context:**
  - `internal/fabricengine/destroy.go`
  - `internal/fabricengine/worktreelist.go`
- **Edits:**
  - `internal/fabricengine/remove.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** convert `removeWarpWorktreeDir` onto the gate.
  Build one `pathRequest` with `what` naming the warp worktree, container `l.HubPath`, target `target`, `slug` nil, ownership `ownedRegisteredLinkedWorktree(l.WorktreePath())`, dirtiness `dirtyScopeAll()` and `force` from the parameter, then call `removeGitWorktree`.
  Keep the existing `run git worktree remove for %s: %w` spawn-failure message and the existing "git refused … it is not a linked worktree of this repo, so fabric will not delete the directory itself" message, both built from the exit code and stderr the executor now returns.
  Then re-gate the fallback: replace the bare `os.RemoveAll(target)` with a second gated call, `removePath` on a `pathRequest` carrying the same container, target and ownership kind and `dirtiness: dirtyScopeAll()`.
  This is one of the three gaps the slice closes and it must not be reduced to an ordering argument.
  The fallback fires on *any* nonzero exit from `git worktree remove`, and `git worktree remove` without `--force` refuses on untracked files;
  a fallback that is not itself gated would therefore delete exactly the untracked files git had just declined to discard.
  Keep the existing `fallback removal failed: %w` wording for an operational failure, and let a `*destructiveRefusal` propagate unwrapped so `errors.As` still works at the caller.
  Leave the post-fallback `worktree prune` call and its "bookkeeping only" comment exactly as they are — it is not a destructive primitive.
  Keep `isRegisteredLinkedWorktree`, its wrapper `isRegisteredLinkedWorktreeIn` and the pre-fallback registration test where they are: the gate now runs the same predicate, but the site's own check is what produces the specific error message the operator sees, per the overview's "existing per-site refusal messages are kept" decision.
- **Commit:** `refactor(fabricengine): route Remove's worktree removal and fallback through the gate`

### Card 14: surface refusals on Remove's four best-effort teardown calls

- **Context:**
  - `internal/fabricengine/destroy.go`
  - `internal/fabricengine/portals.go`
  - `internal/fabricengine/launchers.go`
- **Edits:**
  - `internal/fabricengine/remove.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** `Topology.Remove` discards the error from four teardown helpers that become gated in this batch, so a gate refusal would vanish at exactly the verb the slice's worst defect came from.
  Apply the `surfaceRefusal` shape from the overview's `## Shared Decisions` to all four: the two early `removePortal` and `removeLaunchers` calls, the `removeWarpJunction` call inside the link sweep, and the `unwireBoardLink` call beside it.
  Each becomes a call whose result is passed through `surfaceRefusal`, returning `RemoveResult{}` and that error when non-nil, and otherwise continuing with today's control flow exactly — including the link-sweep's existing rule that a failed sweep leaves `linksRemoved` unincremented rather than aborting.
  An operational failure stays discarded at all four sites;
  only a `*destructiveRefusal` propagates.
  Do not reorder the four calls.
  The portal and launcher teardowns run after the slug and prime checks but before the git removal on purpose, so they still run when the worktree directory is already gone, and that ordering is load-bearing.
- **Commit:** `fix(fabricengine): surface gate refusals from Remove's best-effort teardowns`

### Card 15: gate Prune's stale-pair removal

- **Context:**
  - `internal/fabricengine/destroy.go`
  - `internal/fabricengine/worktreelist.go`
- **Edits:**
  - `internal/fabricengine/prune.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** convert the two destructive calls inside `removeStalePair`.
  The `git worktree remove --force` call becomes `removeGitWorktree` on a `pathRequest` with container `l.HubPath`, target `weftPath`, ownership `ownedRegisteredLinkedWorktree(weftRepoRoot)`, dirtiness `dirtyScopeTracked()` and `force` true, run from `weftRepoRoot`.
  The `os.RemoveAll(weftPath)` fallback becomes `removePath` on a request carrying the same container, target, ownership kind and dirtiness member.
  Preserve every existing `pe.Error` string verbatim, including the "it is no longer a linked worktree of %s, so fabric will not delete the directory itself" message and the "remove weft worktree %q failed (git exit %d); fallback cleanup also failed: %v" message, both now built from the exit code and stderr the executor returns.
  Also apply `surfaceRefusal` to the `removePortal` and `removeLaunchers` calls at the top of `removeStalePair`, recording the refusal in `pe.Error` and returning false rather than swallowing it — the portal and launcher teardown there is keyed on a slug the orphan pass *derived* from a directory name, which is precisely the input a refusal is most likely to be about.
  Leave `applyStalePairOwnership` and `applyStalePairProtection` exactly where they are and unchanged apart from batch 1's probe migration: they compute the `Unowned` and `Protected` flags a dry run reports, which the gate cannot replace because the gate only ever runs in apply mode.
  Do not move either of them behind the gate and do not let the gate's refusal set those flags — a dry run must keep answering the question `--apply` would act on, and it does so by running the same two helpers in both modes.
- **Commit:** `refactor(fabricengine): route Prune's stale-pair removal through the gate`

### Card 16: gate the launcher teardown

- **Context:**
  - `internal/fabricengine/destroy.go`
  - `internal/fabricengine/ancestors.go`
- **Edits:**
  - `internal/fabricengine/launchers.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** convert `removeLaunchers`'s three `os.Remove` calls — the two named launcher scripts and the launcher directory itself — onto `removePath`.
  Each is a `pathRequest` with container `launchersDir(l)`, the respective target, `slug` set to a `slugSpec` carrying `slug` and the caller's junction-name set, ownership `ownedUnderGeometryRoot(launchersDir(l))`, dirtiness `dirtinessNA("launcher scripts are generated artifacts, never edited content")` and `force` false.
  `removeLaunchers` currently takes only `(l *lyxcwd.Location, slug string)` and has no junction-name set in scope, so give the `slug` field a nil `*slugSpec` rather than widening the signature across its three call sites;
  slug validation for these paths already happens at `Remove`'s and `Add`'s entry points, and `Prune`'s derived slugs are validated at the worktree-removal sites in cards 13 and 15.
  Keep the existing `refuseUncontainedPath(launchersDir(l), launcherDir, "launcher dir")` call at the top: the gate now runs containment too, but this call produces the message the operator sees.
  Keep the `os.IsNotExist` tolerance at all three sites — the gate's absent-target rule already returns a no-op success, so the tolerance is belt and braces rather than duplication, and removing it would make the function's idempotence depend on the gate alone.
  Keep both existing error messages and the trailing `pruneEmptyAncestors` call, which is not a destructive primitive: `os.Remove` on a directory is refused by the OS when non-empty and the loop halts on the first refusal.
  `ownedUnderGeometryRoot` rather than a "hub geometry child" kind is deliberate — on a subpath-anchored hub the script files sit three or more levels below the launchers directory, so the kind must admit deep descendants and non-directory targets.
- **Commit:** `refactor(fabricengine): route the launcher teardown through the gate`

### Card 17: gate the portal teardown

- **Context:**
  - `internal/fabricengine/destroy.go`
  - `internal/fabricengine/ancestors.go`
- **Edits:**
  - `internal/fabricengine/portals.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** convert `removePortal`'s `fslink.Remove(link)` onto `removeLink`.
  The `pathRequest` carries container `PortalsDir(l)`, target `link`, ownership `ownedWiredJunction([]string{PortalLink(l, slug)}, portalTarget(l, slug))`, dirtiness `dirtinessNA("a junction holds no content; the weft target it points at is untouched")` and `force` false.
  The wired-link set is a one-element slice because a portal has exactly one wired location, and passing it explicitly is what stops the kind degenerating to bare link-ness — a user's own symlink at some other path is a link too, and that is R1's defect.
  Keep the existing `refuseUncontainedPath(PortalsDir(l), link, "portal")` call and the existing `remove portal %s: %w` wrapper, and keep the trailing `pruneEmptyAncestors` call.
  Preserve the documented idempotence — "Returns nil if the link does not exist" — which the gate's absent-target rule now supplies directly.
- **Commit:** `refactor(fabricengine): route the portal teardown through the gate`

### Card 18: gate the weft teardown and close the junction-record containment gap

- **Context:**
  - `internal/fabricengine/destroy.go`
  - `internal/fabricengine/junction.go`
  - `internal/fabricengine/worktreelist.go`
- **Edits:**
  - `internal/fabricengine/weftwiring.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** two conversions in this file, one of which closes a live gap.
  First, `removeJunctionRecords` calls `fslink.Remove` straight on a slug-derived `WarpJunction.Link` with **no containment check at all**, while its two siblings both call `refuseUncontainedPath` — and it is reached from both `Remove` and `rollbackAdd`.
  Give it a leading `container string` parameter and convert its loop body to `removeLink`, with target `j.Link`, ownership `ownedWiredJunction(links, j.Target)` where `links` is the `.Link` value of every junction in the slice, dirtiness `dirtinessNA("a junction holds no content; the weft target it points at is untouched")` and `force` false.
  Update `removeWarpJunction` to pass `WorktreePath(l, slug)` as the container.
  Keep the best-effort accumulate-and-continue loop with `errors.Join`: a per-junction operational failure must not stop the sweep, and the caller-side `surfaceRefusal` in card 14 is what stops a refusal being lost.
  Second, convert `removeWeftWorktree`'s `git worktree remove` call onto `removeGitWorktree`, with container `l.HubPath`, target `weftPath`, ownership `ownedRegisteredLinkedWorktree(weftRoot)`, dirtiness `dirtyScopeAll()` — matching `refuseDirtyWeftWorktree`'s current untracked-inclusive scope — and `force` from the parameter, run from `weftRoot`.
  Preserve the existing `firstErr` accumulation shape and the existing "git worktree remove failed with exit code %d" message.
  Leave the `git branch -D` call in this function alone;
  batch 5 converts it.
  Leave the trailing `worktree prune` call alone.
- **Commit:** `fix(fabricengine): gate the weft teardown and close removeJunctionRecords' containment gap`

### Card 19: gate the four link teardowns and the two link re-points

- **Context:**
  - `internal/fabricengine/destroy.go`
  - `internal/fabricengine/weftwiring.go`
  - `internal/fabricengine/junctionnames.go`
- **Edits:**
  - `internal/fabricengine/unwire.go`
  - `internal/fabricengine/junction.go`
  - `internal/fabricengine/reconcile.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** convert the remaining four `fslink.Remove` call sites, two of which are teardowns and two of which are repairs, and surface refusals at the one best-effort caller left.
  In `internal/fabricengine/unwire.go`, `unwireBoardLink`'s removal becomes `removeLink` with container `WorktreePath(l, slug)`, target `link`, ownership `ownedWiredJunction([]string{link}, BoardDir(l.HubPath))` and dirtiness `dirtinessNA("a junction holds no content; the weft target it points at is untouched")`.
  In `internal/fabricengine/junction.go`, `unseedJunctionRecords`'s removal becomes `removeLink` with ownership `ownedWiredJunction(links, targetResolved)` where `links` is the `.Link` value of every junction in the slice;
  give the function a leading `container string` parameter and pass `WorktreePath(l, slug)` from its one caller.
  Both teardown sites keep their existing pre-checks and existing error messages verbatim — the is-it-a-link refusal and the mis-pointed-target refusal — per the overview's "existing per-site refusal messages are kept" decision.
  Also in `internal/fabricengine/junction.go`, the two re-point sites — the one inside `seedLyxJunction` and the one inside `wireBoardLink` — call `repointLink` rather than `removeLink`, with ownership `ownedDriftedWiredJunction` over the same wired-link set and no force parameter at all.
  A re-point deliberately does not compare the resolved target, because a drifted or dangling target is the precondition for repairing it;
  a separate executor rather than a flag on the teardown one is what stops a teardown site opting out of the comparison that protects it.
  Keep both re-point sites' `fslink.CreateDirLink` follow-up and both existing `re-point junction %s: %w` and `re-point board junction %s: %w` wrappers.
  In `internal/fabricengine/reconcile.go`, the stale-junction sweep calls `removeWarpJunction` with its error discarded and then unconditionally appends the name to its `removed` list.
  That helper is now gated, so on a `*destructiveRefusal` the sweep must log via `logger.Warn` — it is a `void` helper with no propagation path, which is the case the overview's refusal-surfacing decision covers — and must **not** append the name, because reporting a junction as removed after the gate refused to remove it is the reporting defect this slice's step 5 exists to stop.
  An operational failure keeps today's behaviour exactly: discarded, name still appended.
- **Commit:** `refactor(fabricengine): route every link teardown and re-point through the gate`

## Batch Tests

`verify:` runs both tiers of `internal/fabricengine`.
The integration tier is required here and not optional: this batch is the first to change what the destructive verbs actually do, and the discussion's `### Existing regression cover` section names the tests that police it — `TestPrune_RefusesHubDirectoryItDoesNotOwn`, `TestPrune_RefusesUnrelatedGitCloneInHub`, `TestPrune_ProtectsDirtyWeftWorktreeUntilForced`, `TestAdd_RejectsReservedHubNameSlug`, plus `remove_guard_integration_test.go`, `remove_reserved_integration_test.go`, `prune_unowned_integration_test.go` and `prune_dirty_integration_test.go`.
Those are sabotage-proved tests written for the eight original defects, and a consolidating refactor that keeps every dirtiness scope and every ownership rule intact is exactly what they are good at policing.

Scope stays the one package.
No file outside `internal/fabricengine` is touched, and the module-wide `go build ./...` at the batch boundary covers the two helper signatures that change.

No new test is written in this batch.
The two gaps it closes — `removeJunctionRecords`' missing containment and `removeWarpWorktreeDir`'s un-regated fallback — get their dedicated integration tests in batch 7, where they sit beside the third gap's test and share its fixture setup rather than duplicating it here.
