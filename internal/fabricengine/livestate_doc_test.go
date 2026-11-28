// livestate_doc_test.go documents the live-state integration harness for internal/fabricengine.
// The harness is the set of package fabricengine_test files with the livestate_ prefix.
// They build a hub through internal/hubforge and drive fabric against it.
//
// # What this harness is
//
// The harness drives fabric against a real cloned hub: a warp worktree and its paired weft sibling,
// both backed by real bare git repositories on disk.
// It does not use hand-assembled fixtures.
// A named hostile-state matrix plants dirty, stale, or otherwise hostile on-disk conditions before a verb runs.
// A verb table lists the topology verbs under test, each with its own fixture steps and permitted removal roots.
// A verb's Build steps run once per anchor in a registered hubforge template, which each cell copies;
// its Arrange derives the rest of the fixture from the copy.
// The cross product of the two runs, and every cell asserts survival: the cell names what may change,
// and any change outside that is a failure.
// Defects of this kind surface only when real git runs against a real filesystem in hostile or dirty state.
//
// # Naming: manifest, never snapshot
//
// The filesystem capture taken before and after a verb runs is called a manifest.
// The names are Manifest, CaptureManifest and DiffManifest; never call the capture a snapshot.
// In fabric's own vocabulary, Snapshot names the `Snapshot: <tag>` trailer.
// That trailer records a warp SHA on the weft branch, and this package drives that code.
//
// # The Windows gap
//
// The harness has no runtime.GOOS skip in its states, cells, or helpers, so it would run on Windows.
// Nobody has run it there yet.
// The one divergence the design carries for Windows is the trackedSymlinkAtWiredPath state.
// That state models a git-tracked symlink;
// on Windows it becomes a junction, since it is built through fslink.CreateDirLink rather than os.Symlink.
// The assertion keeps its shape anyway:
// the gate's ownedWiredJunction check compares fslink.RawTarget, not a platform-specific link representation.
//
// # The three-member Check set
//
// The harness consumes fabricengine.Check directly, with its three members:
// CheckContainment, CheckOwnership and CheckDirtiness.
// It carries no copy of its own, and it has no CheckForce member.
// The rule is declared once, on fabricengine.Check.
//
// # Honesty is a distinct property from survival
//
// AssertRecordMatchesDiff checks the honesty of the record, and its own unit tests live in mutationoracle_test.go.
// The destructive chokepoint records a primitive's entry after the primitive succeeds, whatever the gate decided.
// Recording does not depend on the check, so a record entry's presence means the primitive really ran.
// Neutering one of the checks in the sabotage table therefore does not, on its own, produce a lie:
// the primitive still runs through the chokepoint, and the record names it honestly.
// AssertNoUnpermittedChange keys on permitted roots, and it catches the authorisation failure instead.
// The cell fails on the authorisation check.
// The honesty assertion adds no second failure, because there is no lie to catch.
//
// The defect class the honesty assertion exists for is a different one: a destructive primitive that
// bypasses the recording chokepoint entirely, whatever the authorisation question — an omission,
// not a bad authorisation.
// `prune.go`'s `removeStalePair` calls `git worktree prune` to clear a stale warp-side admin registration.
// The call is recorded as `KindWorktreeRemoved` only when a before/after probe of `.git/worktrees/<slug>` confirms it.
// No cell declares the admin directory a permitted root, so survival never flags the sweep.
// The honesty assertion does flag it, with a lie of omission naming the uncovered path:
// `warp-bare/.git/worktrees/<slug>: removed (was dir), but no record entry covers it`.
//
// # Runtime
//
// The package's runtime is noted so that a reader can compare a later run against it.
// It is not asserted, since a timing assertion fails on a loaded machine rather than on a real regression.
//
// # Sabotage-proof table
//
// Each row records one sabotage run: the checks neutered by temporary, uncommitted working-tree edits.
// Each row names the cell as a (state, verb, anchor) triple.
// Each row also gives the check neutered and where, and the assertion that fired.
//
//   - (1) trackedSymlinkAtWiredPath x UnwireJunctions x "." -- reconcile removed a tracked symlink.
//     This cell runs UnwireJunctions, not Reconcile.
//     Reconcile's re-point path (junction.go) uses ownedDriftedWiredJunction, which never compares the target.
//     That path has no check to neuter, so this state is one of Reconcile's four omitted structural states.
//     The check that refuses this cell is destroy.go's ownedWiredJunction target comparison.
//     resolvePathOwnership does the comparison,
//     and UnwireJunctions reaches it through unseedJunctionRecords (junction.go).
//     refuses it independently even once enumerated).
//     TestReconcile_PreservesUserSymlinkAtAnchor fails only when both checks are disabled.
//     The trackedSymlinkAtWiredPath state targets the exact wired path.
//     Its UnwireJunctions cell needs no linkIsFabricOwned bypass: the target-resolution check in
//     unseedJunctionRecords (junction.go) refuses first, so the cell asserts KindRefusedBefore.
//   - (2) dirtyWarpTracked x Pull x "." -- Pull must refuse to discard uncommitted tracked warp work.
//     The dirty-warp pre-flight in Fabric.Pull (pull.go) refuses it: `if dirty { return result, ErrWarpDirty }`.
//     With that guard disabled, Pull proceeds into the rewrite-detected branch instead of refusing.
//   - (3) dirtyWarpUntracked x Remove x "." -- remove must refuse a dirty warp worktree, not delete it.
//     The pre-flight in remove.go (`if dirty { return ... "worktree has uncommitted changes; use --force" }`)
//     refuses it. With that check disabled, the cell fails on its refusal half.
//     RefusedBefore excludes any error containing "check failed".
//     So a gate refusal fails the cell even when its substring matches.
//     The manifest diff also fails, because remove.go tears down the pair's junction links before the pre-flight.
//   - (4) clean x Cleanup x "." -- cleanup must never delete the primary weft branch.
//     Three checks guard it: the primaryWeft carve-out in cleanup.go, the liveWarpBranches liveness check
//     one line above that carve-out, and the primaryWeftBranch check in resolveManagedBranch (destroy.go).
//     The liveness check alone protects the primary in TestCleanup_PrimaryBranchSurvivesForceWhenNotCheckedOut.
//     That fixture moves only the weft side off main-weft.
//     Disabling any one check leaves the branch protected by the other two.
//     With all three disabled, that test fails: Cleanup reports the primary weft branch "main-weft" as handled.
//     The clean/Cleanup/"." cell covers the same protection.
//     It also covers Cleanup's ordinary orphan deletion, through the branch cleanupCase deliberately orphans.
//     Its Build moves the prime pair off the hub's default branch.
//     Its Effect asserts that the primary weft branch still exists afterward.
//   - (5) dirtyWarpUntracked x Prune x "." checks that Prune does not remove a path git has just refused.
//     This cell cannot prove that on its own.
//     pruneCase's StateTarget.WarpCheckout resolves to the hub's prime worktree.
//     Prune's gate never inspects that worktree for the stale pair under test.
//     Planting dirt there and neutering any Prune-reachable check leaves this cell's assertions unchanged.
//     Rows (6) and (7) cover the same mechanism for Prune: a fallback that goes ahead after a git refusal.
//     They do so through applyStalePairOwnership and isRegisteredLinkedWorktreeIn.
//     Both gate removeStalePair's fallback path.
//   - (6) foreignDirAtFabricOwnedPath x Prune x "." guards against Prune removing a weft-suffixed user directory.
//     The ownership check alone is masked for this state.
//     worktreeDirty errors on a non-git directory, and checkPathDirtiness turns that error into a refusal.
//     Isolating the ownership gate in isRegisteredLinkedWorktreeIn means neutering checkPathDirtiness too.
//   - (7) unrelatedGitCloneAtWeftNamedPath x Prune x "." guards against Prune removing an unrelated git clone.
//     Both checks in row (6) protect this state too.
//   - (8) the Reset column's non-hub target x CloneHub{Reset} x "." guards against clone --reset.
//     A reset must never destroy a non-hub `<derived>-LYMHUB`.
//     checkResetTarget refuses a target that does not look like a fabric hub.
//     The gate's ownedFabricHub check refuses the same target too.
//     this particular run, but the cell still failed on the Kind mismatch (a gate refusal, not the
//     checkResetTarget's), proving checkResetTarget itself is load-bearing rather than redundant with the gate.
//   - (9) hostile input ".." x Remove x "." -- validateWorktreeSlug refuses relative path elements.
//     Without that refusal, WorktreePath resolves ".." onto the hub's own parent directory,
//     and a Remove there would destroy the hub.
//
// # Omission table
//
// The exported Omissions slice (verbs.go) names every verb/state pair excluded from the cross
// product, with its reason, so a green matrix can be audited against what it did not run.
// The four structural states are:
// trackedSymlinkAtWiredPath, foreignDirAtFabricOwnedPath, unrelatedGitCloneAtWeftNamedPath and staleWiredJunction.
// Cleanup and Checkout each omit all four, because their only gate call is the branch-shaped deleteBranch.
// Pull omits the same four, because its only gate call is Fabric.ResetHard.
// That call resets a warp checkout; it is not a path executor.
// Add omits trackedSymlinkAtWiredPath and staleWiredJunction, because its gate calls act on the pair it is creating.
// Its junction paths do not exist before Run.
// UnwireJunctions omits unrelatedGitCloneAtWeftNamedPath, because its only gate call never visits a weft-named path.
//
// Further omissions per anchor follow from the pathRequest call sites each verb reaches.
// Each is grounded in a read of production code, not in what a run happened to produce:
//   - Reconcile omits all four structural states: reconcile.go contains no pathRequest call,
//     so no structural state, of any shape, names a path that Reconcile's pre-flight (scopeAll on the board) inspects.
//   - Cleanup additionally omits dirtyWeftTracked, dirtyWeftUntracked and bothDirty.
//     Its Build tears down the pair's weft worktree by hand before the state is applied.
//     This mirrors cleanup's orphan-branch precondition.
//     So no weft-targeted dirtiness state has a live checkout to plant into.
//   - Remove omits foreignDirAtFabricOwnedPath and unrelatedGitCloneAtWeftNamedPath.
//     Its path-executor gate calls reach only the pair's own registered worktree and its wired junctions.
//     Those are the only paths it reaches, so the retained link-shaped states already cover them.
//   - Prune omits trackedSymlinkAtWiredPath and staleWiredJunction: its only path-executor gate call,
//     ownedRegisteredLinkedWorktree, targets a directory, never a link.
//     prune.go has no removeLink or repointLink call at all.
//   - UnwireJunctions additionally omits foreignDirAtFabricOwnedPath.
//     Its StructuralPath is the pair's own pre-existing wired junction link,
//     and Apply of foreignDirAtFabricOwnedPathState does not remove that link first.
//     Only trackedSymlinkAtWiredPath and staleWiredJunction do.
//     So its os.MkdirAll would silently no-op through the intact link; no foreign directory gets parked at the path.
//
// assertCellTally in matrix_test.go derives the cell total from the Verbs, States and Omissions tables.
// A change to those tables moves the total without any edit to this comment.
// TestCloneHubReset drives the CloneHub{Reset} column separately from TestCrossProduct.
//
// # The truthfulness oracle's own contract
//
// AssertRecordMatchesDiff (mutationoracle.go) is a second assertion that every cell runs beside survival.
// Survival (AssertNoUnpermittedChange) asks whether an authorised change stayed inside its permitted roots.
// The oracle asks whether the record the call returned names what the manifest diff shows happened.
// The two catch different defect classes, so both run on every cell.
//
// The oracle cross-checks in both directions.
// Omission asks whether every diff change is accounted for by some record entry.
// assertOmission checks the record's raw, unfolded entries.
// A change that a later entry undid must still be named, since the record names what ran, not what survives.
// Commission asks whether every manifest-observable record entry is backed by a real diff change.
// assertCommission checks the record's net effect, after netCommissionEffect folds away each mint-then-undo pair.
// A rollback that creates and then destroys the same path in one call nets to zero in the before/after manifest,
// yet the record carries both entries; folding first keeps that legitimate case from reading as a lie.
//
// Not every fabricengine.Kind is judged the same way in the commission direction.
// manifestObservableKind splits the kinds into two groups.
// The manifest-observable kinds are real filesystem changes that CaptureManifest would see.
// The git-state kinds are git-internal state that CaptureManifest's filesystem walk never fully captures;
// branch existence is the sharpest example, since a branch is git bookkeeping, not a manifest-visible file.
// A git-state kind is exempt from commission.
// It is asserted instead through the per-verb Expectation.Effect assertions.
// Those read git directly rather than infer state from a manifest that cannot carry it.
// KindFileWritten is the one kind decided by its Target rather than by manifestObservableKind alone.
// In participatesInCommission, a file_written under ".git" records git-state bookkeeping;
// CaptureManifest excludes that wholesale, so only a file_written outside ".git" participates.
//
// A record entry's Target covers a change in both directions, wider than the exact path.
// entryCovers admits segment-wise subtree containment (pathAtOrBelowRoot) plus two widenings.
// A path_renamed entry also covers a change at or below its Detail, the renamed source.
// A kind in worktreeRootedKind also covers the derived <prime>/.git/worktrees/<name> admin registration.
// That registration is the stale bookkeeping entry `git worktree prune` clears.
// A worktree's creation or destruction can touch it without a record entry naming that exact admin path.
// An ancestor directory of a recorded Target is covered too, when the diff change is itself a directory.
// Such ancestors are implicit chains around the roots fabric records.
// fslink.CreateDirLink creates them.
//
// DiffManifest is called twice per cell, on the same before and after manifest pair.
// The two calls answer two questions that neither can answer for the other.
// The first call, filtered through the cell's exp.PermittedRoots, feeds AssertNoUnpermittedChange.
// That assertion checks that an authorised change stayed where it was allowed to.
// The second call passes a nil permitted list (`unfiltered := DiffManifest(before, after, nil)`).
// It feeds AssertRecordMatchesDiff, which checks that the record honestly names what changed.
// Permitted roots suppress diff noise for the first assertion only.
// They never suppress it for the second assertion.
// Narrowing the oracle's own input by the same allowlist would hide permitted but unrecorded changes.
// Such a change would then escape both assertions at once, the gap the oracle exists to close.
package fabricengine_test
