// Package fabrictest is the live-state integration harness for internal/fabricengine.
//
// # What this package is
//
// fabrictest drives fabric against a real cloned hub — a warp worktree and its paired weft sibling,
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
// crucible hardening rounds.
// The one genuine divergence this package's design carries for Windows: the
// trackedSymlinkAtWiredPath state models a git-tracked symlink, which on Windows materialises as a
// junction because it is built through fslink.CreateDirLink rather than a raw os.Symlink.
// The assertion keeps its shape regardless, because the gate's ownedWiredJunction check compares
// fslink.RawTarget, not a platform-specific link representation.
//
// # The three-member Check set
//
// The exported Check set fabricengine's destructive gate carries has exactly three members:
// CheckContainment, CheckOwnership, CheckDirtiness.
// checkForce is declared at destroy.go:39 and rendered by String() at destroy.go:51, but it is never
// constructed into a *destructiveRefusal anywhere in the tree — force is consulted only inside
// checkPathDirtiness, where it makes the dirtiness check pass rather than fail.
// A CheckForce constant could therefore never match a real refusal, and must not be added back.
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
// It is flagged here for slice 14's truthfulness work, where "what did this call actually mutate
// before it failed" becomes representable in a refusal's own shape.
//
// # Measured wall-clock
//
// A full run (`go test -tags integration ./internal/fabricengine/fabrictest/`, no -run filter,
// exercising every test in the package including this file's own cross-product matrix) measures
// consistently around 4.0-4.3s of package elapsed time (go test's own "ok ... Xs" line) across
// repeated runs, on a 12-core x86_64 Linux machine, at the default -parallel value (12, unset,
// derived from GOMAXPROCS on that machine). This number is recorded for a future reader to notice a
// regression against, not asserted: a timing assertion fails on a loaded CI box rather than on a real
// regression, which is why this repo rejects timing assertions elsewhere too.
//
// # Sabotage-proof table
//
// Placeholder — batch 8 fills this section with the sabotage-proof table: for each production file
// this harness exists to catch a regression in, the specific mutation applied and the cell(s) that
// caught it.
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
package fabrictest
