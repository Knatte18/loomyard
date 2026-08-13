//go:build integration

// livestate_doc_test.go documents the live-state integration harness for internal/fabricengine: a set
// of package fabricengine_test files (the livestate_ prefix) that build a hub through internal/hubforge
// and drive fabric against it.
//
// # What this harness is
//
// It drives fabric against a real cloned hub — a warp worktree and its paired weft sibling,
// both backed by real bare git repositories on disk — rather than against hand-assembled fixtures.
// It builds a named hostile-state matrix (fixtures that plant dirty, stale, or otherwise hostile
// on-disk conditions before a verb runs), a verb table (the topology verbs under test, each with its
// own Arrange step and its permitted-removal-roots), and drives the cross product of the two with
// per-cell survival assertions: every cell names what is allowed to change, and anything outside that
// is a failure.
// The package exists because six prior hardening rounds on fabric found their defects exclusively by
// driving real git against a real filesystem with hostile or dirty state — the hermetic suite alone
// never found one.
//
// # Naming: manifest, never snapshot
//
// The filesystem capture this package takes before and after a verb runs is called a manifest —
// Manifest, CaptureManifest, DiffManifest — and never a snapshot.
// internal/fabricengine/snapshot.go already owns the word Snapshot in fabric's own vocabulary: it is
// the `Snapshot: <tag>` trailer recording a warp SHA on the weft branch.
// Reusing "snapshot" for a filesystem capture here would collide with that meaning inside the very
// package this harness drives.
//
// # The Windows gap
//
// This harness carries no runtime.GOOS skip anywhere in its states, cells, or helpers, and would run
// on Windows — but nobody has run it there yet.
// See manifest/designs/fabric-windows-verification.md for the full account of that gap across six
// hardening rounds.
// The one genuine divergence this package's design carries for Windows: the
// trackedSymlinkAtWiredPath state models a git-tracked symlink, which on Windows materialises as a
// junction because it is built through fslink.CreateDirLink rather than a raw os.Symlink.
// The assertion keeps its shape regardless, because the gate's ownedWiredJunction check compares
// fslink.RawTarget, not a platform-specific link representation.
//
// # The three-member Check set
//
// This harness consumes fabricengine.Check directly — CheckContainment, CheckOwnership, CheckDirtiness —
// rather than carrying a copy of its own; there is deliberately no CheckForce member, and fabricengine.
// Check's own doc comment (internal/fabricengine/destroy.go) is this rule's one declarer, not this file.
//
// # One known refusal-with-side-effects anomaly
//
// Remove runs removePortal and removeLaunchers at remove.go:61-66, before its own dirty pre-flight at
// remove.go:68-76.
// A dirty-Remove cell that correctly refuses has therefore already destroyed the pair's portal and
// launcher paths before the refusal is ever returned.
// This is deliberate, documented behaviour, not a defect: the cell that exercises it declares
// _portals/<anchor>/<slug> and _launchers/<anchor>/<slug> as permitted removal roots rather than
// treating their disappearance as a failure.
// "What did this call actually mutate before it failed" is what slice 14's truthfulness work made
// representable in a refusal's own shape: it landed as AssertRecordMatchesDiff (mutationoracle.go),
// wired into every cell in this package (batch 7 card 31).
// This cell's own record now names the portal and launcher deletions RefusedBefore's own path took
// before returning, so the cell asserts both halves at once — that the deletions were allowed under this
// cell's permitted roots, and that the envelope admitted to them.
//
// # Honesty is a distinct property from survival, not a restatement of it
//
// AssertRecordMatchesDiff's own unit tests (mutationoracle_test.go) exercise its rules directly.
// This section is about how it relates to the sabotage table below, because the relationship is easy to
// get wrong.
// destroy.go's chokepoint records a destructive primitive's entry AFTER the primitive succeeds,
// unconditionally on whether the gate that authorised it should have refused — recording is
// check-independent by design, so a caller can trust that a record entry's presence means the primitive
// really ran.
// One consequence follows directly from that design: neutering one of the nine checks below does NOT, on
// its own, produce a lie.
// The sabotaged primitive still runs through the same chokepoint, which still records it honestly — the
// record correctly names an unauthorised destruction, and it is AssertNoUnpermittedChange (survival,
// keyed on permitted roots) that catches the authorisation failure each of the nine rows documents,
// exactly as it already did before this batch.
// Re-neutering any of the nine checks and re-running its cell fails the SAME way it always has; the
// honesty assertion adds no second failure there, because there is no lie for it to catch in that defect
// class.
//
// The defect class the honesty assertion exists for is a different one: a destructive primitive that
// bypasses the recording chokepoint entirely, independent of any authorisation question — an omission,
// not a bad authorisation.
// Batch 7's own cross-product run (card 31) found a real, live instance of exactly that, not a
// hypothetical one: prune.go's removeStalePair calls `git worktree prune` to clear a stale warp-side
// `.git/worktrees/<slug>` admin registration left behind when a pair's warp worktree directory is removed
// by hand, and that call recorded nothing.
// Survival passed every one of those cells — the admin directory is not a path any cell declares (or
// needs to declare) a permitted root over, so nothing about the sweep looked unauthorised.
// The honesty assertion failed instead, with a lie of omission naming the exact uncovered path:
// `warp-bare/.git/worktrees/<slug>: removed (was dir), but no record entry covers it`.
// The fix — recording KindWorktreeRemoved when a before/after probe of the admin path confirms the
// primitive actually cleared it — landed in the same batch, alongside this note.
//
// # Measured wall-clock
//
// A full run (`go test -tags integration ./internal/fabricengine/`, no -run filter,
// exercising every test in the package including this file's own cross-product matrix) measures
// consistently around 4.0-4.3s of package elapsed time (go test's own "ok ... Xs" line) across
// repeated runs, on a 12-core x86_64 Linux machine, at the default -parallel value (12, unset,
// derived from GOMAXPROCS on that machine). This number is recorded for a future reader to notice a
// regression against, not asserted: a timing assertion fails on a loaded CI box rather than on a real
// regression, which is why this repo rejects timing assertions elsewhere too.
//
// # Sabotage-proof table
//
// Batch 8 proved, by manual edit-run-revert cycle against a real checkout of this tree, that the check
// each of the eight evidence-table defects was fixed by still fires today.
// Every edit below was a temporary, uncommitted local working-tree change: the production diff this
// task delivers touches no file this section names, per the no-production-behaviour-change Shared
// Decision.
// Each row names the cell as a (state, verb, anchor) triple, which check was neutered and where, and
// the assertion that fired once it was.
//
//   - (1) trackedSymlinkAtWiredPath x UnwireJunctions x "." -- R1, reconcile destroyed a tracked
//     symlink. The live cell this harness runs for the state is UnwireJunctions, not Reconcile:
//     Reconcile's own WireJunctions/seedLyxJunction re-point path (junction.go) uses
//     ownedDriftedWiredJunction, which never compares the resolved target -- drift is repointLink's own
//     precondition for repair, not a disqualifier -- so no check exists there to neuter, and this state
//     is (correctly) one of Reconcile's four omitted structural states. The load-bearing R1 fix instead
//     lives in destroy.go's ownedWiredJunction target comparison (resolvePathOwnership,
//     pathOwnershipWiredJunction), reached from unseedJunctionRecords (junction.go), which
//     UnwireJunctions calls. Confirmed twice: first against fabricengine's own
//     TestReconcile_PreservesUserSymlinkAtAnchor, by neutering reconcile.go's linkIsFabricOwned (forced
//     to return (true, nil)) together with destroy.go's ownedWiredJunction target check (forced to skip
//     its comparison) -- both were needed, since either check alone still refused (linkIsFabricOwned
//     gates whether applyStaleRemoval enumerates the link at all; ownedWiredJunction's target check
//     refuses it independently even once enumerated) -- and the test failed: "Reconcile removed the
//     hand-authored symlink ...: lstat ...: no such file or directory; want it preserved". Second,
//     independently, this package's own trackedSymlinkAtWiredPath state targets the exact wired path
//     (not an unrelated name), and running it against UnwireJunctions requires no linkIsFabricOwned
//     bypass at all -- unseedJunctionRecords's own inline target-resolution check
//     ("points to unexpected target", junction.go) fires first every time, which is why that state's own
//     UnwireJunctions cell asserts KindRefusedBefore rather than reaching the gate.
//   - (2) dirtyWarpTracked x Pull x "." -- R2, pull discarded uncommitted tracked warp work. Neutered
//     pull.go's own dirty-warp pre-flight (the `if dirty { return result, ErrWarpDirty }` guard in
//     Fabric.Pull), by short-circuiting it to never fire. Ran
//     `go test -tags integration ./internal/fabricengine/ -run 'TestCrossProduct/\./dirtyWarpTracked/Pull$'`:
//     the cell failed with "expected pre-flight refusal containing \"warp worktree has uncommitted
//     changes\"; got err = fabricengine: warp remote diverged and local warp has unpushed commits;
//     aborting, no changes" -- Pull proceeded straight into the rewrite-detected branch instead of
//     refusing.
//   - (3) dirtyWarpUntracked x Remove x "." -- R3, remove destroyed the warp worktree directory via an
//     os.RemoveAll fallback past a git refusal. Neutered remove.go's own dirty-warp pre-flight (the
//     `if dirty { return ... "worktree has uncommitted changes; use --force" }` guard, remove.go:68-76).
//     Ran the same cell's `-run 'TestCrossProduct/\./dirtyWarpUntracked/Remove$'`: it failed on the
//     REFUSAL half, exactly as the batch's own extra requirement demands -- the error was
//     "expected pre-flight refusal containing \"uncommitted changes\"; got err = run git worktree remove
//     for ...: refusing to remove warp worktree: dirtiness check failed for ...: worktree has
//     uncommitted changes; use --force", i.e. RefusedBefore's mandatory "check failed" exclusion is what
//     makes this fail on the Kind mismatch (a gate refusal, not a pre-flight one) rather than passing
//     because the substring still matched. It also failed on the manifest diff in this run (the pair's
//     own junction links had already been swept by remove.go's own portal/launcher/junction teardown,
//     which runs before the pre-flight per the one known tranche-1 anomaly doc.go's omission table
//     records elsewhere), but the row's own requirement is that the refusal-kind failure alone would
//     have been sufficient even had the diff stayed silent, which the "check failed" exclusion is what
//     guarantees.
//   - (4) clean x Cleanup x "." -- R3, cleanup deleted the primary weft branch. This defect is guarded
//     in triplicate today: cleanup.go's own early primaryWeft carve-out (cleanup.go:154), the
//     branch-space liveWarpBranches liveness check one line above it (cleanup.go:145, which alone
//     already protects the primary in fabricengine's own TestCleanup_PrimaryBranchSurvivesForceWhenNotCheckedOut
//     fixture, since that fixture only moves the WEFT side off main-weft and the warp side stays on
//     "main"), and the gate's own independent primaryWeftBranch check inside resolveManagedBranch
//     (destroy.go:486). Neutering any one alone left the branch protected by one of the other two, so
//     the sabotage run neutered all three simultaneously (each short-circuited to never fire) and ran
//     `go test -tags integration ./internal/fabricengine/ -run TestCleanup_PrimaryBranchSurvivesForceWhenNotCheckedOut`:
//     it failed with "Cleanup reported/handled primary weft branch \"main-weft\"; want not reported (live
//     pair)" and "main-weft branch deleted after force Cleanup with primary parked elsewhere; want
//     intact (F1 regression)". This harness's own clean/Cleanup/"." cell exercises both Cleanup's ordinary
//     orphan-deletion path (cleanupCase's own deliberately-orphaned branch) AND this defence
//     independently: cleanupCase's Arrange also moves the prime pair off the hub's default branch
//     (mirroring the hermetic fixture above), and its Effect asserts the primary weft branch still
//     exists afterward, so this row now has two independent live proofs rather than only the hermetic
//     one.
//   - (5) dirtyWarpUntracked x Prune x "." -- R3 (found by the orchestrator), prune removed a path git
//     had just refused. Recorded as NOT independently provable via this exact (state, verb, anchor)
//     triple: pruneCase's own StateTarget.WarpCheckout resolves to the hub's PRIME worktree, a path
//     Prune's own gate never inspects for the stale pair under test, so planting dirt there and
//     neutering any Prune-reachable check leaves this cell's own assertions unchanged (confirmed: ran
//     with prune.go's applyStalePairOwnership ownership check and destroy.go's checkPathDirtiness both
//     neutered, and the cell still passed). The underlying R3 class this row names -- a destructive
//     fallback proceeding past a git refusal without an independent ownership check -- is the same
//     mechanism rows (6) and (7) below prove live and load-bearing for Prune (applyStalePairOwnership /
//     isRegisteredLinkedWorktreeIn, gating removeStalePair's git-worktree-remove-then-fallback path);
//     see those two rows for the actual neuter-run-revert evidence.
//   - (6) foreignDirAtFabricOwnedPath x Prune x "." -- R4, prune removed an ordinary weft-suffixed user
//     directory. Neutered destroy.go's isRegisteredLinkedWorktreeIn (forced to always return true) and
//     checkPathDirtiness (forced to always return nil), together -- the ownership bypass alone left this
//     specific state protected by an unrelated side effect (worktreeDirty erroring against a
//     non-git-repo directory, which checkPathDirtiness turns into its own refusal), so both were
//     neutered to isolate the ownership gate's own contribution. Ran
//     `-run 'TestCrossProduct/\./foreignDirAtFabricOwnedPath/Prune$'`: the cell failed with
//     "unpermitted change(s) outside permitted roots: verb-prune-structural-orphan-weft: removed (was
//     dir) ... verb-prune-structural-orphan-weft/livestate-sentinel-foreignDirAtFabricOwnedPath.txt:
//     removed (was file)" plus the assertPlantedContentSurvives failure naming the same missing sentinel.
//   - (7) unrelatedGitCloneAtWeftNamedPath x Prune x "." -- R4, prune removed an unrelated git clone.
//     Same two-check neutering as row (6). Ran `-run 'TestCrossProduct/\./unrelatedGitCloneAtWeftNamedPath/Prune$'`:
//     the cell failed the same way, the offending paths additionally including the clone's own ".git"
//     directory ("verb-prune-structural-orphan-weft/.git: removed (was dir)").
//   - (8) the Reset column's non-hub target x CloneHub{Reset} x "." -- R4, `clone --reset` destroyed a
//     non-hub `<derived>-HUB`. Neutered clone.go's resetHub own pre-flight (the `if !looksLikeHub(hubPath)`
//     guard, short-circuited to never fire). Ran `-run 'TestCloneHubReset/\./CloneHubReset/NonHubTarget$'`:
//     the cell failed with "expected pre-flight refusal containing \"is not a fabric hub\"; got err =
//     reset: remove hub at ...: refusing to reset hub: ownership check failed for ...: ... does not look
//     like a fabric hub (no _board entry and no weft sibling)" -- the gate's own ownedFabricHub check
//     caught what the pre-flight was neutered out of catching, so the operator-content sentinel survived
//     this particular run, but the cell still failed on the Kind mismatch (a gate refusal, not the
//     pre-flight's), proving the pre-flight itself is load-bearing rather than redundant with the gate.
//   - (9) hostile input ".." x Remove x "." -- R5, `remove ..` destroyed an entire hub. Neutered
//     slug.go's validateWorktreeSlug relative-path-element guard (the
//     `slug != filepath.Clean(slug) || slug == "." || slug == ".."` condition, short-circuited to never
//     fire). Ran `-run 'TestCrossProduct/\./clean/Remove/DotDot$'`: the cell failed with "expected
//     pre-flight refusal containing \"a slug must name a directory\"; got err = check warp worktree
//     status at <hub's own parent dir>: git status --porcelain in <same path>: exit 128: fatal: not a
//     git repository" -- validateWorktreeSlug no longer refused ".." before WorktreePath(l, "..")
//     resolved onto the hub's own parent, and only worktreeDirty's own inability to run git against a
//     non-repository path stopped the run before any further destructive call executed; no filesystem
//     content was actually destroyed in this particular run, and the target directory involved was this
//     one subtest's own isolated t.TempDir(), never a shared or system path.
//
// # Omission table
//
// The exported Omissions slice (verbs.go) names every verb/state pair excluded from the cross
// product, with its reason, so a green matrix can be audited against what it did not run.
// The plan's own cell-enumeration-and-omissions Shared Decision derived fifteen structural-state
// omissions per anchor before batch 7's driver first ran: Cleanup and Checkout each omit all four
// structural states (trackedSymlinkAtWiredPath, foreignDirAtFabricOwnedPath,
// unrelatedGitCloneAtWeftNamedPath, staleWiredJunction) because their only gate call is the
// branch-shaped deleteBranch; Pull omits the same four because its only gate call is
// Fabric.ResetHard, a warp checkout reset rather than a path executor; Add omits
// trackedSymlinkAtWiredPath and staleWiredJunction because its gate calls act on the pair it is
// creating, whose junction paths do not exist before Run; UnwireJunctions omits
// unrelatedGitCloneAtWeftNamedPath because its only gate call never visits a weft-named path.
//
// Batch 7's card 19 (the full-matrix run) found twelve further omissions per anchor while classifying
// every failing cell against the actual pathRequest call sites each verb reaches, none of them
// dirtiness-scope-table omissions in the sense the plan anticipated -- each is grounded in a verified
// read of production code, never in what a run happened to produce:
//   - Reconcile omits all four structural states: reconcile.go contains no pathRequest call at all,
//     so no structural state, of any shape, names a path Reconcile's own pre-flight (scopeAll on the
//     board) ever inspects.
//   - Cleanup additionally omits dirtyWeftTracked, dirtyWeftUntracked and bothDirty: its own Arrange
//     tears down the pair's weft worktree by hand before the state is applied, mirroring cleanup's own
//     orphan-branch precondition, so no weft-targeted dirtiness state has a live checkout to plant
//     into.
//   - Remove omits foreignDirAtFabricOwnedPath and unrelatedGitCloneAtWeftNamedPath: its own
//     path-executor gate calls (removeWarpWorktreeDir's ownedRegisteredLinkedWorktree check, and
//     removeWarpJunction's ownership-filtered link sweep) never reach an independent foreign directory
//     or unrelated clone at another path -- only the pair's own registered worktree and its own wired
//     junctions, which the retained link-shaped states already cover.
//   - Prune omits trackedSymlinkAtWiredPath and staleWiredJunction: its only path-executor gate call
//     (ownedRegisteredLinkedWorktree, prune.go:264) targets a directory, never a link -- prune.go has
//     no removeLink/repointLink call at all.
//   - UnwireJunctions additionally omits foreignDirAtFabricOwnedPath: its StructuralPath, like
//     Remove's, is the pair's own pre-existing wired junction link, and foreignDirAtFabricOwnedPathState's
//     Apply does not remove that link first (only trackedSymlinkAtWiredPath and staleWiredJunction do),
//     so its os.MkdirAll would silently no-op through the still-intact link rather than genuinely park
//     a foreign directory at the path.
//
// Twenty-seven omission rows in total, fifty-four across both anchors, bringing the cross product's
// audited total from the plan's originally-derived 168 cells down to 144 (140 run by TestCrossProduct
// plus the four-cell CloneHub{Reset} column TestCloneHubReset drives separately) -- the count
// assertion in matrix_test.go (assertCellTally) derives this total from the Verbs, States and
// Omissions tables themselves, so it moves automatically with any future change to those tables
// rather than needing to be kept in sync with this paragraph by hand.
//
// # The truthfulness oracle's own contract
//
// AssertRecordMatchesDiff (mutationoracle.go) is a second, independent assertion every cell runs
// beside survival (AssertNoUnpermittedChange): survival asks whether an authorised change stayed
// inside its permitted roots, the oracle asks whether the record the call itself returned honestly
// names what the manifest diff shows actually happened. The two catch different defect classes --
// see "Honesty is a distinct property from survival, not a restatement of it" above -- and both run
// on every cell, unconditionally.
//
// The oracle cross-checks in both directions. Omission asks whether every diff change is accounted
// for by some record entry (assertOmission, over the record's raw, unfolded entries -- a change a
// later entry undid must still have been named by something, since the record's job is naming what
// ran, not what survives to the end). Commission asks whether every manifest-observable record
// entry is backed by a real diff change (assertCommission, over the record's net effect after
// netCommissionEffect folds away a mint-then-undo pair -- a rollback that creates and then destroys
// the same path inside one call nets to zero in the before/after manifest even though the record
// correctly carries both entries, and folding first is what keeps that legitimate case from reading
// as a lie).
//
// Not every fabricengine.Kind is judged the same way in the commission direction. manifestObservableKind
// splits the vocabulary into manifest-observable kinds (path_removed, worktree_removed,
// worktree_created, dir_created, link_removed, link_created -- a real filesystem change
// CaptureManifest would see) and git-state kinds (branch_created, branch_deleted, branch_pushed,
// commit_created, worktree_reset, worktree_switched, push_spawned, repo_advanced -- git-internal
// state CaptureManifest's filesystem walk never fully captures, branch existence being the sharpest
// example: a branch is git bookkeeping, not a manifest-visible file). A git-state kind is exempt from
// commission and is instead asserted through the per-verb Expectation.Effect assertions, which read
// git directly rather than infer state from a manifest that was never going to carry it.
// KindFileWritten is the one kind decided by its Target rather than by this table alone
// (participatesInCommission): a file_written under ".git" records git-state bookkeeping
// CaptureManifest excludes wholesale, so only a file_written outside ".git" participates.
//
// The coverage rule a record entry's Target extends over, in both directions, is wider than "this
// exact path": entryCovers admits segment-wise subtree containment (pathAtOrBelowRoot) plus two
// widenings. A worktree-rooted kind (worktreeRootedKind: worktree_created, worktree_removed,
// worktree_reset, worktree_switched, repo_advanced) additionally covers the derived
// <prime>/.git/worktrees/<name> admin registration -- the stale bookkeeping entry `git worktree
// prune` clears, which a worktree's own creation or destruction can plausibly touch without a
// separate record entry naming that exact admin path (see the omission table's `removeStalePair`
// worked example above). And an ancestor directory of a recorded Target is covered too, when the diff
// change is itself a directory -- the implicit ancestor chains fslink.CreateDirLink and
// writeLaunchers MkdirAll materialise around the roots fabric actually records, and
// pruneEmptyAncestors reaps on the way out, per the record-at-the-covering-root Shared Decision.
//
// DiffManifest is deliberately called twice per cell, against the same before/after manifest pair,
// for two different questions neither call can answer for the other. The first call, filtered
// through the cell's own exp.PermittedRoots, feeds AssertNoUnpermittedChange -- "did an authorised
// change stay where it was allowed to". The second call, always with a nil permitted list
// (`unfiltered := DiffManifest(before, after, nil)`), feeds AssertRecordMatchesDiff -- "does the
// record honestly name what changed, full stop". Permitted roots suppress diff noise for the first
// question only; they never suppress the second, since narrowing the oracle's own input by the same
// allowlist would let a permitted-but-unrecorded change escape both assertions at once, precisely the
// gap the oracle exists to close.
package fabricengine_test
