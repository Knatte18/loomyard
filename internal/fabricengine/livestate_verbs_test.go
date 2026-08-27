//go:build integration

// verbs.go builds the tranche-1 verb table: the nine gate-reaching verbs, each with its own Arrange
// fixture, its Run call, and its Expect function deriving a per-state Expectation; plus the seventeen
// hostile-input cases and the omission table recording every verb/state pair deliberately excluded
// from the cross product batch 7 drives.
//
// # The verified dirtiness-scope table
//
// Every dirtiness cell's outcome is DERIVED from this table, never observed and then written down — a
// cell written from observed behaviour asserts nothing, which is the exact failure mode that let a
// green suite coexist with eight data-loss defects across the crucible rounds this harness exists to
// replay.
// The table covers the declarations reachable in tranche 1 with force=false and is NOT an exhaustive
// index of every pathRequest in the package: Remove's weft-side gate rows (weftwiring.go:199
// dirtyScopeAll, :219 dirtyCheckedOutBranch), Checkout's checkout.go:200 and Add's add.go:268/:292
// are deliberately absent because none of them changes a tranche-1 cell's derived outcome, and a
// later tranche that adds a force axis must extend this table rather than assume it already covers
// them.
//
//   - Add own pre-flight (add.go:43) — scopeTracked, the prime warp worktree.
//   - Checkout own pre-flight (checkout.go:42) — scopeTracked, the weft worktree; Checkout has NO
//     warp-side dirtiness probe at all (checkout.go:38-85 goes straight to `git switch`).
//   - Remove own pre-flight (remove.go:69) — scopeAll, the pair's warp worktree.
//   - Remove's refuseDirtyWeftWorktree (remove.go:79/:144) — scopeAll, the pair's weft worktree.
//   - Remove's gate call (remove.go:196/:230) — dirtyScopeAll, the pair's warp worktree.
//   - Remove's gate call (launchers.go:172/:193) — dirtinessNA.
//   - Prune own pre-flight (prune.go:214) — scopeTracked, the weft path.
//   - Prune's gate call (prune.go:269/:292) — dirtyScopeTracked, stale pair paths.
//   - Pull own pre-flight (pull.go:143) — scopeTracked, the prime warp worktree.
//   - Pull's gate call via Fabric.ResetHard (destroy.go:762) — dirtyScopeTracked, the prime warp
//     worktree.
//   - Reconcile own pre-flight (reconcile.go:301) — scopeAll, the board.
//   - Cleanup's gate call — branch-shaped, dirtyCheckedOutBranch.
//   - UnwireJunctions' gate call — link-shaped, dirtinessNA.
//   - CloneHub{Reset}'s gate call (clone.go:585) — dirtinessNA.
//
// The derivation rule: a scopeTracked verb against an untracked-only state expects Proceeds; a
// scopeAll verb against the same state expects a refusal; a verb with NO probe for the dirtied side
// gets an explicit per-cell expectation, never a derived one.
// dirtyWarpTracked x Checkout is the only such case in tranche 1, and it is git-decided: `git switch`
// either carries the tracked modification across or refuses with "your local changes would be
// overwritten", so the cell asserts what must hold either way — the tracked modification is still on
// disk afterwards, and the pair is not half-switched (either both sides moved or neither did).
// Guessing which branch git takes would be asserting git's behaviour rather than fabric's; the
// disjunction still catches what matters, since a half-switched pair is exactly what Checkout's
// documented all-or-nothing rollback exists to prevent.
//
// # CloneHub{Reset: true}
//
// This column is re-scoped to the ownership axis and is not run against the ten states.
// resetHub declares dirtinessNA (clone.go:585), so no dirtiness state can refuse it — total hub
// destruction on --reset is correct behaviour — which would make the permitted root the hub root
// itself and the manifest diff vacuous across all ten states.
// The gate is structurally unreachable for Reset: resetHub refuses a non-hub at its own pre-flight
// (clone.go:573-577, !looksLikeHub) BEFORE the pathRequest at :579 is built, and the gate's
// pathOwnershipFabricHub predicate is that SAME looksLikeHub call (destroy.go:346-350), so a
// RefusedByGate(CheckOwnership) expectation would fail against a correct binary.
// This column therefore uses RefusedBefore("is not a fabric hub") and drives exactly two targets: a
// <derived>-HUB-named directory that is not a hub (refused at the pre-flight, contents surviving —
// R4's `clone --reset` defect), and a real hub (torn down and rebuilt through fabriccli.CloneAndWire,
// the positive case proving the column is not trivially always-refusing).
//
// # The seventeen hostile-input cases
//
// All carry States: []string{"clean"} and both anchors.
// Add and Remove each take the same seven slug-shaped inputs: "", ".", "..", "../x", a
// weftname.Suffix-suffixed name, a reserved hub name, and a leading "-".
// Checkout takes two branch-shaped inputs (a nonexistent branch and a flag-shaped name), because
// checkout.go:38-85 validates no branch at all.
// UnwireJunctions takes one: a junction name escaping its worktree ("../x"), in scope despite taking
// neither a slug nor a branch, because its names []string is the only route to the link executor's
// containment check.
// Each input's expectation is derived from what validateWorktreeSlug (slug.go:30) actually rejects —
// never asserting whatever the current behaviour happens to be, which would assert nothing.
// A leading "-" is NOT rejected by validateWorktreeSlug — it is not on any of that function's five
// rejection rules — so no gate check owns it and neither refusal kind applies; those cells (and the
// two Checkout hostile cells) are written against the safe expectation that the argument never reaches
// git as a flag and nothing outside the permitted roots is destroyed.

package fabricengine_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/fabriccli"
	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/fslink"
	"github.com/Knatte18/loomyard/internal/hubforge"
	"github.com/Knatte18/loomyard/internal/lyxcwd"
	"github.com/Knatte18/loomyard/internal/lyxdirs"
	"github.com/Knatte18/loomyard/internal/weftname"
)

// VerbFixture carries whatever a VerbCase's Arrange produced: a pair slug, an advanced upstream SHA,
// a broken remote, a checked-out branch — and the StateTarget the state builders need.
// Not every field is populated by every verb's Arrange; each Arrange populates only the fields its own
// Run and Expect closures consume.
type VerbFixture struct {
	// Slug is the pair slug this cell operates on, when the verb under test takes one.
	Slug string
	// Target is the StateTarget a hostile or dirty state plants against, resolved per verb per the
	// verified dirtiness-scope table above.
	Target StateTarget
	// BrokenOriginURL is the nonexistent remote URL Add's Arrange points the prime warp worktree's
	// origin at, so Add's push fails after the branch and worktree already exist.
	BrokenOriginURL string
	// AdvancedFromSHA is the prime warp worktree's HEAD before Pull's Arrange pushes a new commit to
	// the hub's own warp bare.
	AdvancedFromSHA string
	// AdvancedToSHA is the new commit Pull's Arrange pushed, the upstream tip Pull is expected to
	// advance to.
	AdvancedToSHA string
	// OriginalBranch is the prime warp worktree's branch before Checkout's Run switches it away.
	OriginalBranch string
	// CheckoutBranch is the warp branch Checkout's Run passes: the pair slug's own warp branch,
	// distinct from OriginalBranch.
	CheckoutBranch string
	// ResetCwd, ResetWeftURL, ResetWarpURL and ResetHubPath carry the CloneHub{Reset: true} column's
	// own fixture: the parent directory, the two remote URLs, and the derived hub path Reset targets.
	ResetCwd     string
	ResetWeftURL string
	ResetWarpURL string
	ResetHubPath string
}

// ExpectationKind enumerates the closed set of outcomes a cell can declare: a refusal from fabric's
// destructive gate, a refusal from one of fabric's own pre-flight checks, the verb proceeding to its
// intended effect, or -- for the one tranche-1 cell where the outcome is git's own to decide, not
// fabric's -- either of the last two.
type ExpectationKind int

const (
	// KindRefusedByGate means the error contains "<check> check failed" for the declared Check.
	KindRefusedByGate ExpectationKind = iota
	// KindRefusedBefore means the error contains the declared substring and does not contain "check
	// failed".
	KindRefusedBefore
	// KindProceeds means the verb succeeds, its intended effect lands, and the state's planted
	// content survives.
	KindProceeds
	// KindEitherProceedsOrRefusedBefore means the outcome is git's own to decide rather than
	// fabric's: either the verb succeeds (err == nil), or it fails with a RefusedBefore-shaped error
	// carrying the declared Substring. checkoutCase's dirtyWarpTracked cell is the one tranche-1 case
	// this exists for -- see this file's own doc comment, "dirtyWarpTracked x Checkout is the only
	// such case in tranche 1" -- and it is deliberately NOT a per-verb conditional in the driver: it
	// is one more closed, data-declared outcome a cell can name, exactly like the other three.
	KindEitherProceedsOrRefusedBefore
)

// String reports k's name in prose, so a cell failure message names the expectation kind it violated.
func (k ExpectationKind) String() string {
	switch k {
	case KindRefusedByGate:
		return "RefusedByGate"
	case KindRefusedBefore:
		return "RefusedBefore"
	case KindProceeds:
		return "Proceeds"
	case KindEitherProceedsOrRefusedBefore:
		return "EitherProceedsOrRefusedBefore"
	default:
		return "unknown"
	}
}

// Expectation is what one cell — a verb case run against one named state — is derived to produce.
// PermittedRoots is present on all three kinds: a refusal is not assumed side-effect-free, which is
// why the refusal kinds carry permit roots too.
// Effect is populated for clean-state cells regardless of Kind, since card 17's wiring suite needs an
// extra, verb-specific check beyond the generic error-shape and manifest-diff checks every cell gets.
type Expectation struct {
	Kind           ExpectationKind
	Check          fabricengine.Check
	Substring      string
	PermittedRoots []string
	Effect         func(tb testing.TB, h *hubforge.Hub, f VerbFixture)
}

// VerbCase is one verb under test: its own fixture builder, its call into fabric, the states it runs
// against, and the per-state Expectation it derives.
// Arrange is a required field, not an optional convenience: most verbs need a fixture no state builds,
// and folding that work into Run would be a correctness bug rather than a style choice — the
// before-manifest is captured before Run, so an arrangement mutation folded into Run would show up in
// the diff as an unpermitted change.
// States is an optional restriction: empty means the case inherits every state in the package-level
// States slice, which is the default and what every ordinary verb uses, so a newly appended verb still
// gets the full matrix automatically.
// Only the hostile-input cases and the CloneHub{Reset} column set it, always to a State whose Apply is
// itself never invoked for a hostile case ("clean" — a no-op — or, for CloneHub{Reset}, a case-specific
// pseudo-state name resolved entirely by Arrange).
// Run returns the call's own fabricengine.Mutations record alongside its error: the twelve verbs under
// test return twelve heterogeneous result types, so a cell needs one uniform accessor —
// MutationRecord.Mutated() — to read whichever one it drove, and discarding the typed result the way
// this field used to would leave nothing for a cell to cross-check against the manifest diff.
type VerbCase struct {
	Name    string
	Arrange func(tb testing.TB, h *hubforge.Hub) VerbFixture
	Run     func(tb testing.TB, h *hubforge.Hub, f VerbFixture) (fabricengine.Mutations, error)
	States  []string
	Expect  func(state string) Expectation
}

// Omission names one verb/state pair deliberately excluded from the cross product, with the reason —
// so a green matrix can be audited against what it did not run.
// An arbitrarily-planted state is worse than an absent one: it produces a green cell that proves
// nothing while reading like coverage.
type Omission struct {
	Verb   string
	State  string
	Reason string
}

// Omissions is the durable, exported record of every verb/state pair excluded from the cross product.
// It starts with the structural-state omissions derived in the plan's overview and grows with every
// dirtiness- or structural-state omission this package's own derivation work found while deriving each
// cell against the verified dirtiness-scope table and the actual pathRequest call sites each verb
// reaches -- batch 7's full-matrix run (card 19) is what surfaced the rows below the original fifteen,
// each grounded in a verified read of the production call site it names, never in what a run happened
// to produce.
var Omissions = []Omission{
	{Verb: "Cleanup", State: "trackedSymlinkAtWiredPath", Reason: "Cleanup's only gate call is deleteBranch (cleanup.go:283), branch-shaped; no structural state names a path it acts on"},
	{Verb: "Cleanup", State: "foreignDirAtFabricOwnedPath", Reason: "Cleanup's only gate call is deleteBranch (cleanup.go:283), branch-shaped; no structural state names a path it acts on"},
	{Verb: "Cleanup", State: "unrelatedGitCloneAtWeftNamedPath", Reason: "Cleanup's only gate call is deleteBranch (cleanup.go:283), branch-shaped; no structural state names a path it acts on"},
	{Verb: "Cleanup", State: "staleWiredJunction", Reason: "Cleanup's only gate call is deleteBranch (cleanup.go:283), branch-shaped; no structural state names a path it acts on"},
	{Verb: "Checkout", State: "trackedSymlinkAtWiredPath", Reason: "Checkout's only gate call is deleteBranch (checkout.go:195-203), branch-shaped; no structural state names a path it acts on"},
	{Verb: "Checkout", State: "foreignDirAtFabricOwnedPath", Reason: "Checkout's only gate call is deleteBranch (checkout.go:195-203), branch-shaped; no structural state names a path it acts on"},
	{Verb: "Checkout", State: "unrelatedGitCloneAtWeftNamedPath", Reason: "Checkout's only gate call is deleteBranch (checkout.go:195-203), branch-shaped; no structural state names a path it acts on"},
	{Verb: "Checkout", State: "staleWiredJunction", Reason: "Checkout's only gate call is deleteBranch (checkout.go:195-203), branch-shaped; no structural state names a path it acts on"},
	{Verb: "Pull", State: "trackedSymlinkAtWiredPath", Reason: "Pull's only gate call is Fabric.ResetHard (destroy.go:762), a warp checkout reset rather than a path executor"},
	{Verb: "Pull", State: "foreignDirAtFabricOwnedPath", Reason: "Pull's only gate call is Fabric.ResetHard (destroy.go:762), a warp checkout reset rather than a path executor"},
	{Verb: "Pull", State: "unrelatedGitCloneAtWeftNamedPath", Reason: "Pull's only gate call is Fabric.ResetHard (destroy.go:762), a warp checkout reset rather than a path executor"},
	{Verb: "Pull", State: "staleWiredJunction", Reason: "Pull's only gate call is Fabric.ResetHard (destroy.go:762), a warp checkout reset rather than a path executor"},
	{Verb: "Add", State: "trackedSymlinkAtWiredPath", Reason: "Add's gate calls (add.go:263-295) act on the pair it is creating; that junction path does not exist before Run"},
	{Verb: "Add", State: "staleWiredJunction", Reason: "Add's gate calls (add.go:263-295) act on the pair it is creating; that junction path does not exist before Run"},
	{Verb: "UnwireJunctions", State: "unrelatedGitCloneAtWeftNamedPath", Reason: "UnwireJunctions' only gate call is the removeLink inside unseedJunctionRecords (junction.go:474-483), reached via unseedLyxJunction; it never visits a weft-named path"},

	// Card 19's full-matrix run: Reconcile has no path-executor gate call at all -- reconcile.go
	// contains no pathRequest -- so no structural state, of any shape, names a path Reconcile's own
	// pre-flight (reconcile.go:301, scopeAll on the board) ever inspects.
	{Verb: "Reconcile", State: "trackedSymlinkAtWiredPath", Reason: "Reconcile has no path-executor gate call (reconcile.go contains no pathRequest); its own pre-flight is scopeAll on the board, not a per-path check"},
	{Verb: "Reconcile", State: "staleWiredJunction", Reason: "Reconcile has no path-executor gate call (reconcile.go contains no pathRequest); its own pre-flight is scopeAll on the board, not a per-path check"},
	{Verb: "Reconcile", State: "foreignDirAtFabricOwnedPath", Reason: "Reconcile has no path-executor gate call (reconcile.go contains no pathRequest); its own pre-flight is scopeAll on the board, not a per-path check"},
	{Verb: "Reconcile", State: "unrelatedGitCloneAtWeftNamedPath", Reason: "Reconcile has no path-executor gate call (reconcile.go contains no pathRequest); its own pre-flight is scopeAll on the board, not a per-path check"},

	// Card 19's full-matrix run: Cleanup's own Arrange tears down the pair's weft worktree by hand
	// (mirroring an orphan managed branch, cleanup's own precondition) before the state is applied, so
	// no weft-targeted dirtiness state can plant into a checkout that no longer exists.
	{Verb: "Cleanup", State: "dirtyWeftTracked", Reason: "Cleanup's Arrange removes the pair's weft worktree before the state is applied, mirroring cleanup's own orphan-branch precondition; the checkout a weft dirtiness state would plant into does not exist"},
	{Verb: "Cleanup", State: "dirtyWeftUntracked", Reason: "Cleanup's Arrange removes the pair's weft worktree before the state is applied, mirroring cleanup's own orphan-branch precondition; the checkout a weft dirtiness state would plant into does not exist"},
	{Verb: "Cleanup", State: "bothDirty", Reason: "Cleanup's Arrange removes the pair's weft worktree before the state is applied, mirroring cleanup's own orphan-branch precondition; the checkout bothDirty's weft half would plant into does not exist"},

	// Card 19's full-matrix run: Remove's own path-executor gate calls (remove.go's
	// removeWarpWorktreeDir, ownedRegisteredLinkedWorktree on the pair's own registered worktree; and
	// removeWarpJunction's ownership-filtered link sweep) never reach an independent foreign directory
	// or unrelated clone at another path -- only the pair's own worktree and its own wired junctions,
	// which the retained link-shaped states (trackedSymlinkAtWiredPath, staleWiredJunction) already
	// cover.
	{Verb: "Remove", State: "foreignDirAtFabricOwnedPath", Reason: "Remove's own path-executor gate calls target the pair's registered worktree and its own wired junctions; no ownership check reaches an independent foreign directory at another path"},
	{Verb: "Remove", State: "unrelatedGitCloneAtWeftNamedPath", Reason: "Remove's own path-executor gate calls target the pair's registered worktree and its own wired junctions; no ownership check reaches an independent unrelated clone at another weft-named path"},

	// Card 19's full-matrix run: Prune's only path-executor gate call (prune.go:264,
	// ownedRegisteredLinkedWorktree on the orphan's own weft directory) targets a directory, never a
	// link -- prune.go has no removeLink/repointLink call at all, so no link-shaped structural state
	// names a path it acts on.
	{Verb: "Prune", State: "trackedSymlinkAtWiredPath", Reason: "Prune's only path-executor gate call targets a directory (prune.go:264, ownedRegisteredLinkedWorktree); it has no removeLink/repointLink call, so no link-shaped structural state names a path it acts on"},
	{Verb: "Prune", State: "staleWiredJunction", Reason: "Prune's only path-executor gate call targets a directory (prune.go:264, ownedRegisteredLinkedWorktree); it has no removeLink/repointLink call, so no link-shaped structural state names a path it acts on"},

	// Card 19's full-matrix run: UnwireJunctions' own StructuralPath, like Remove's, is the pair's own
	// pre-existing wired junction link. foreignDirAtFabricOwnedPathState's Apply does not remove that
	// link first (only trackedSymlinkAtWiredPath and staleWiredJunction do), so its os.MkdirAll would
	// silently no-op through the still-intact link and write through it, rather than genuinely park a
	// foreign directory at the path -- the same structural reason unrelatedGitCloneAtWeftNamedPath is
	// already omitted for this verb.
	{Verb: "UnwireJunctions", State: "foreignDirAtFabricOwnedPath", Reason: "UnwireJunctions' StructuralPath is the pair's own pre-existing wired junction link; foreignDirAtFabricOwnedPathState's Apply does not remove it first, so os.MkdirAll would silently no-op through the intact link instead of parking a foreign directory"},
}

// hubRelative returns abs's path relative to h.Path, slash-normalised, for use as a manifest-relative
// permitted root or StateTarget.StructuralPath derivation.
// Every geometry path this file needs is obtained from an fabricengine accessor first and converted
// here, never spelled as a raw literal, per the geometry-through-accessors Shared Decision.
func hubRelative(tb testing.TB, h *hubforge.Hub, abs string) string {
	tb.Helper()

	rel, err := filepath.Rel(h.Path, abs)
	if err != nil {
		tb.Fatalf("hubRelative(%s): %v", abs, err)
	}
	return filepath.ToSlash(rel)
}

// firstWiredJunctionLink returns the warp-side path of slug's first wired junction (e.g. "_lyx"),
// resolved via fabricengine.WiredNames and fabricengine.WarpJunctions -- the same accessors
// unwireJunctionsCase's own Run uses -- for a cell whose structural states need a real, pre-existing
// link to remove and replant, per the geometry-through-accessors Shared Decision.
func firstWiredJunctionLink(tb testing.TB, h *hubforge.Hub, slug string) string {
	tb.Helper()

	names, err := fabricengine.WiredNames(h.BoardDir())
	if err != nil {
		tb.Fatalf("fabricengine.WiredNames(%s): %v", h.BoardDir(), err)
	}
	if len(names) == 0 {
		tb.Fatalf("fabricengine.WiredNames(%s): no wired names", h.BoardDir())
	}
	return fabricengine.WarpJunctions(h.Location, slug, names)[0].Link
}

// pairPermittedRoots returns the four hub-relative roots a pair's full teardown or creation may touch:
// its warp worktree, its weft sibling, its portal junction, and its launcher directory.
func pairPermittedRoots(tb testing.TB, h *hubforge.Hub, slug string) []string {
	tb.Helper()

	return []string{
		hubRelative(tb, h, h.PairWarpWorktree(slug)),
		hubRelative(tb, h, h.PairWeftSibling(slug)),
		hubRelative(tb, h, h.PairPortalLink(slug)),
		hubRelative(tb, h, h.PairLauncherDir(slug)),
	}
}

// slugPermittedRoots returns the four hub-relative roots a pair named slug may occupy, unioned across
// both anchors ("." and "backend"), for use inside a VerbCase's Expect closure — which, unlike an
// Effect closure, has no *hubforge.Hub in scope to resolve an accessor against, only the state name.
// A permitted root that never matches anything in a given hub's own anchor is harmless: DiffManifest's
// pathPermitted only ever narrows what a diff flags, never widens it, so carrying the unused anchor's
// variant alongside the live one cannot hide a genuine unpermitted change.
// The synthetic *lyxcwd.Location built here (HubPath left empty) is legal because every accessor this
// function calls composes paths via filepath.Join, which drops an empty leading HubPath cleanly,
// leaving exactly the hub-relative suffix an accessor would otherwise require a real hub to produce.
func slugPermittedRoots(slug string) []string {
	var roots []string
	for _, anchor := range []string{".", "backend"} {
		loc := &lyxcwd.Location{WorktreeName: slug, AnchorRel: anchor}
		portal := fabricengine.PortalLink(loc, slug)
		launcher := fabricengine.LauncherDir(loc, slug)
		roots = append(roots,
			filepath.ToSlash(slug),
			filepath.ToSlash(weftname.SiblingPath("", slug)),
			filepath.ToSlash(portal),
			filepath.ToSlash(launcher),
			// pruneEmptyAncestors removes an anchor-level parent left empty by the leaf's own
			// removal (e.g. "_portals/backend" once "_portals/backend/<slug>" is gone); the parent
			// itself is therefore also a legitimate part of a full pair teardown, not an
			// unpermitted change.
			filepath.ToSlash(filepath.Dir(portal)),
			filepath.ToSlash(filepath.Dir(launcher)),
			primeWorktreeAdminPermittedRoot(slug),
			primeWeftAdminPermittedRoot(slug),
		)
	}
	return roots
}

// portalAndLauncherPermittedRoots returns slug's portal-link and launcher-dir hub-relative roots,
// unioned across both anchors, for the one tranche-1 refusal that is not side-effect-free: a dirty
// Remove has already destroyed these two before its own dirty pre-flight ever runs (remove.go:61-76).
// Each leaf's own anchor-level parent is included too, matching slugPermittedRoots' own reasoning:
// pruneEmptyAncestors removes an anchor-level parent left empty by the leaf's own removal (e.g.
// "_portals/backend" once "_portals/backend/<slug>" is the only entry there and is gone), so the
// parent itself is also a legitimate part of this partial teardown, not an unpermitted change.
func portalAndLauncherPermittedRoots(slug string) []string {
	var roots []string
	for _, anchor := range []string{".", "backend"} {
		loc := &lyxcwd.Location{WorktreeName: slug, AnchorRel: anchor}
		portal := fabricengine.PortalLink(loc, slug)
		launcher := fabricengine.LauncherDir(loc, slug)
		roots = append(roots,
			filepath.ToSlash(portal),
			filepath.ToSlash(launcher),
			filepath.ToSlash(filepath.Dir(portal)),
			filepath.ToSlash(filepath.Dir(launcher)),
		)
	}
	return roots
}

// escapeRelName returns a "../"-prefixed junction name carrying exactly enough ".." segments to walk
// all the way out of the hub once joined onto WorktreePath(l,slug)/anchorRel — one to cancel the slug
// segment itself, plus one more per anchorRel path segment (anchorRel "." contributes none).
func escapeRelName(anchorRel, name string) string {
	depth := 1
	if anchorRel != "." {
		depth += len(strings.Split(anchorRel, "/"))
	}
	return strings.Repeat("../", depth) + name
}

// primeWorktreeAdminPermittedRoot returns the hub-relative path of the prime warp worktree's own
// git-admin bookkeeping entry for a linked worktree named slug: the ".git/worktrees/<slug>" directory
// a successful `git worktree remove` clears as part of any pair teardown.
// "warp-bare" is this package's own fixed prime-worktree name (buildBareTemplate, hub.go) — not a
// fabric geometry token, so spelling it here does not touch the geometry-through-accessors ban.
func primeWorktreeAdminPermittedRoot(slug string) string {
	return filepath.ToSlash(filepath.Join("warp-bare", ".git", "worktrees", slug))
}

// primeWeftAdminPermittedRoot is primeWorktreeAdminPermittedRoot's weft-side counterpart: the prime
// weft sibling's own ".git/worktrees/<slug-weft>" bookkeeping entry, cleared the same way on a
// successful weft-worktree teardown.
// "warp-bare-weft" is this package's own fixed prime-weft-sibling name (weftname.SiblingPath applied
// to the fixed "warp-bare" prime name), not a fabric geometry token.
func primeWeftAdminPermittedRoot(slug string) string {
	return filepath.ToSlash(filepath.Join("warp-bare-weft", ".git", "worktrees", weftname.SiblingPath("", slug)))
}

// currentBranchName returns the branch checked out at dir, via `git rev-parse --abbrev-ref HEAD`,
// failing tb on any error.
func currentBranchName(tb testing.TB, dir string) string {
	tb.Helper()

	cmd := exec.Command("git", "rev-parse", "--abbrev-ref", "HEAD")
	cmd.Dir = dir
	out, err := cmd.Output()
	if err != nil {
		tb.Fatalf("git rev-parse --abbrev-ref HEAD in %s: %v", dir, err)
	}
	return strings.TrimSpace(string(out))
}

// liveStateCurrentSHA returns dir's checked-out commit SHA, via `git rev-parse HEAD`, failing tb on any error.
func liveStateCurrentSHA(tb testing.TB, dir string) string {
	tb.Helper()

	cmd := exec.Command("git", "rev-parse", "HEAD")
	cmd.Dir = dir
	out, err := cmd.Output()
	if err != nil {
		tb.Fatalf("git rev-parse HEAD in %s: %v", dir, err)
	}
	return strings.TrimSpace(string(out))
}

// branchExists reports whether branch resolves in the repo at dir.
func branchExists(dir, branch string) bool {
	cmd := exec.Command("git", "rev-parse", "--verify", "refs/heads/"+branch)
	cmd.Dir = dir
	return cmd.Run() == nil
}

// assertGone fails tb unless path does not exist (via os.Lstat, so a dangling link still counts as
// present).
func assertGone(tb testing.TB, path string) {
	tb.Helper()

	if _, err := os.Lstat(path); !os.IsNotExist(err) {
		tb.Errorf("assertGone(%s): still present (lstat err: %v)", path, err)
	}
}

// assertExists fails tb unless path exists (via os.Lstat).
func assertExists(tb testing.TB, path string) {
	tb.Helper()

	if _, err := os.Lstat(path); err != nil {
		tb.Errorf("assertExists(%s): %v", path, err)
	}
}

// addCase builds Topology.Add's VerbCase.
//
// Arrange breaks the warp origin remote (`git remote set-url origin <nonexistent>`) so the push at the
// end of Add fails after the branch and worktree already exist, triggering rollbackAdd — the same
// injection TestBranchOwnership_RefusalHoldsAtOtherDeletionSites already uses, a proven trigger rather
// than a guess.
// Without it, Add never reaches the gate at all: rollbackAdd fires only at post-creation sites
// (add.go:139-204), and none of the ten states induces one on its own.
//
// Because the arrangement forces every non-preflight-refused path through rollbackAdd, and rollback's
// own removal calls (dirtinessNA, correct ownership on a clean hub) succeed unconditionally, Add's
// clean-state Run genuinely returns an error (the push failure) and the hub is left exactly as it was
// before Run — this is the verified behaviour of add.go's own documented rollback contract, not an
// assumption, and it is why this cell's Expectation is RefusedBefore rather than Proceeds.
func addCase() VerbCase {
	return VerbCase{
		Name: "Add",
		Arrange: func(tb testing.TB, h *hubforge.Hub) VerbFixture {
			tb.Helper()

			slug := "verb-add-owner"
			broken := filepath.Join(tb.TempDir(), "nonexistent-origin")
			mustGit(h.PrimeWorktree(), "remote", "set-url", "origin", broken)

			return VerbFixture{
				Slug:            slug,
				BrokenOriginURL: broken,
				Target: StateTarget{
					WarpCheckout: h.PrimeWorktree(),
					WeftCheckout: h.PrimeWeft(),
					// StructuralPath is the pair's own warp worktree path, which does not exist yet --
					// exactly what add.go's own gate call (add.go:263-295) acts on during rollback, and
					// the reason the two link-shaped structural states are omitted for Add (the junction
					// paths do not exist before Run either) while the two directory-shaped ones, which
					// plant fresh content via os.MkdirAll rather than replacing an existing link, remain.
					StructuralPath: h.PairWarpWorktree(slug),
				},
			}
		},
		Run: func(tb testing.TB, h *hubforge.Hub, f VerbFixture) (fabricengine.Mutations, error) {
			tb.Helper()
			res, err := h.Topology.Add(h.Location, f.Slug, fabricengine.AddOptions{})
			return res.Mutated(), err
		},
		Expect: func(state string) Expectation {
			switch state {
			case "dirtyWarpTracked", "bothDirty":
				// Add's own pre-flight (scopeTracked on the prime warp worktree) refuses before
				// anything is created — the broken remote is never even reached.
				return Expectation{
					Kind:      KindRefusedBefore,
					Substring: "source worktree has uncommitted changes",
					Effect: func(tb testing.TB, h *hubforge.Hub, f VerbFixture) {
						assertGone(tb, h.PairWarpWorktree(f.Slug))
					},
				}
			case "foreignDirAtFabricOwnedPath", "unrelatedGitCloneAtWeftNamedPath":
				// The state plants directly at the pair's own warp worktree path -- the one path Add's
				// own gate call (add.go:263-295) acts on -- before Run, so `git worktree add` itself
				// refuses outright: its target directory already exists and is non-empty. This is a
				// git-level pre-flight failure, reached before add.go creates the branch or the
				// broken-remote arrangement is ever consulted, so neither the branch nor any wiring is
				// ever created.
				return Expectation{
					Kind:      KindRefusedBefore,
					Substring: "already exists",
					Effect: func(tb testing.TB, h *hubforge.Hub, f VerbFixture) {
						tb.Helper()
						if branchExists(h.PrimeWorktree(), f.Slug) {
							tb.Errorf("Add unexpectedly created branch %q despite refusing at worktree creation", f.Slug)
						}
					},
				}
			default:
				// Every other state's own pre-flight (or absence of one) lets Add proceed through
				// creation and wiring; the broken-remote arrangement then fails the push
				// unconditionally, and rollbackAdd reverts every step it can.
				// The warp branch is the one artifact rollback cannot revert: rollbackAdd's own
				// branch-delete step (add.go:287-308) gates on ownedManagedBranch, whose
				// resolveManagedBranch (destroy.go:471-499) requires either a non-empty
				// BranchPrefix or a "-weft" suffix — this hub's Topology carries an empty
				// BranchPrefix (fabricengine.Config{}), so a bare warp branch name never matches
				// and the gate refuses the rollback's own cleanup silently (`_ = t.rollbackAdd`).
				// This is genuine, verified behaviour of the production code this harness must not
				// alter, not an assumption.
				return Expectation{
					Kind:      KindRefusedBefore,
					Substring: "push branch",
					Effect: func(tb testing.TB, h *hubforge.Hub, f VerbFixture) {
						tb.Helper()
						assertGone(tb, h.PairWarpWorktree(f.Slug))
						assertGone(tb, h.PairWeftSibling(f.Slug))
						assertGone(tb, h.PairPortalLink(f.Slug))
						assertGone(tb, h.PairLauncherDir(f.Slug))
						if !branchExists(h.PrimeWorktree(), f.Slug) {
							tb.Errorf("Add's rollback unexpectedly deleted branch %q despite the empty-BranchPrefix ownership gap", f.Slug)
						}
						if got := mustGitRemoteURL(tb, h.PrimeWorktree()); got != f.BrokenOriginURL {
							tb.Errorf("Add's Arrange origin URL = %q; want %q", got, f.BrokenOriginURL)
						}
					},
				}
			}
		},
	}
}

// mustGitRemoteURL returns `git remote get-url origin` in dir, failing tb on error.
func mustGitRemoteURL(tb testing.TB, dir string) string {
	tb.Helper()

	cmd := exec.Command("git", "remote", "get-url", "origin")
	cmd.Dir = dir
	out, err := cmd.Output()
	if err != nil {
		tb.Fatalf("git remote get-url origin in %s: %v", dir, err)
	}
	return strings.TrimSpace(string(out))
}

// removeCase builds Topology.Remove's VerbCase.
// Its dirty-refusal cells declare PortalLink and LauncherDir as permitted roots, because
// remove.go:61-66 runs removePortal and removeLaunchers BEFORE the dirty pre-flight at :68-76, so a
// correctly-refusing cell has already destroyed them — the one tranche-1 refusal that is not
// side-effect-free (see doc.go's record of it).
func removeCase() VerbCase {
	return VerbCase{
		Name: "Remove",
		Arrange: func(tb testing.TB, h *hubforge.Hub) VerbFixture {
			tb.Helper()

			slug := "verb-remove-owner"
			hubforge.AddPair(tb, h, slug)
			return VerbFixture{
				Slug: slug,
				Target: StateTarget{
					WarpCheckout: h.PairWarpWorktree(slug),
					WeftCheckout: h.PairWeftSibling(slug),
					// StructuralPath is the pair's own wired junction link: remove.go's link sweep
					// (scanOnDiskJunctionNames + removeWarpJunction) is ownership-filtered -- "only the
					// links fabric itself created there" -- which the two link-shaped structural states
					// exercise directly. The two directory-shaped states are omitted for Remove (see
					// Omissions): its ownership gate never reaches an independent foreign directory.
					StructuralPath: firstWiredJunctionLink(tb, h, slug),
				},
			}
		},
		Run: func(tb testing.TB, h *hubforge.Hub, f VerbFixture) (fabricengine.Mutations, error) {
			tb.Helper()
			res, err := h.Topology.Remove(h.Location, f.Slug, false)
			return res.Mutated(), err
		},
		Expect: func(state string) Expectation {
			switch state {
			case "dirtyWarpTracked", "dirtyWarpUntracked", "dirtyWeftTracked", "dirtyWeftUntracked", "bothDirty":
				// remove.go:61-66 runs removePortal and removeLaunchers BEFORE the dirty pre-flight
				// at :68-76, so a correctly-refusing cell has already destroyed them — the one
				// tranche-1 refusal that is not side-effect-free.
				return Expectation{
					Kind:           KindRefusedBefore,
					Substring:      "uncommitted changes",
					PermittedRoots: portalAndLauncherPermittedRoots("verb-remove-owner"),
					Effect: func(tb testing.TB, h *hubforge.Hub, f VerbFixture) {
						assertGone(tb, h.PairPortalLink(f.Slug))
						assertGone(tb, h.PairLauncherDir(f.Slug))
						assertExists(tb, h.PairWarpWorktree(f.Slug))
					},
				}
			default:
				return Expectation{
					Kind:           KindProceeds,
					PermittedRoots: slugPermittedRoots("verb-remove-owner"),
					Effect: func(tb testing.TB, h *hubforge.Hub, f VerbFixture) {
						tb.Helper()
						assertGone(tb, h.PairWarpWorktree(f.Slug))
						assertGone(tb, h.PairWeftSibling(f.Slug))
						assertGone(tb, h.PairPortalLink(f.Slug))
						assertGone(tb, h.PairLauncherDir(f.Slug))
					},
				}
			}
		},
	}
}

// pruneCase builds Topology.Prune's VerbCase.
// Arrange produces a STALE pair: the warp worktree directory is removed by hand (bypassing fabric),
// leaving the weft sibling and warp's own worktree registration behind exactly as an operator's manual
// `rm -rf` would.
func pruneCase() VerbCase {
	return VerbCase{
		Name: "Prune",
		Arrange: func(tb testing.TB, h *hubforge.Hub) VerbFixture {
			tb.Helper()

			slug := "verb-prune-owner"
			hubforge.AddPair(tb, h, slug)
			if err := os.RemoveAll(h.PairWarpWorktree(slug)); err != nil {
				tb.Fatalf("remove warp worktree %s to stage a stale pair: %v", h.PairWarpWorktree(slug), err)
			}
			return VerbFixture{
				Slug: slug,
				Target: StateTarget{
					WarpCheckout: h.PrimeWorktree(),
					WeftCheckout: h.PairWeftSibling(slug),
					// StructuralPath is a SEPARATE weft-named sibling path that was never Added at all
					// -- an orphan directory shaped exactly like debris Prune's own orphan pass would
					// enumerate, distinct from "verb-prune-owner"'s own (genuinely registered) stale
					// pair. Prune's only ownership gate (prune.go:264, ownedRegisteredLinkedWorktree)
					// is exactly what must refuse to touch it: it is never a registered linked worktree
					// of this hub's weft repo, so it must survive Prune's own teardown of the real stale
					// pair untouched.
					StructuralPath: h.PairWeftSibling("verb-prune-structural-orphan"),
				},
			}
		},
		Run: func(tb testing.TB, h *hubforge.Hub, f VerbFixture) (fabricengine.Mutations, error) {
			tb.Helper()
			res, err := h.Topology.Prune(h.Location, true, false)
			return res.Mutated(), err
		},
		Expect: func(state string) Expectation {
			switch state {
			case "dirtyWeftTracked", "bothDirty":
				// applyStalePairProtection (prune.go:206-224) probes scopeTracked dirtiness on the
				// stale pair's own weft worktree and marks the entry Protected rather than returning a
				// Go error -- Prune reports per-entry outcomes in its own PruneResult, not as an error
				// from the call itself, so this cell's Kind is Proceeds (err == nil) and the assertion
				// that matters is that the protected entry survives untouched.
				return Expectation{
					Kind:           KindProceeds,
					PermittedRoots: slugPermittedRoots("verb-prune-owner"),
					Effect: func(tb testing.TB, h *hubforge.Hub, f VerbFixture) {
						tb.Helper()
						assertExists(tb, h.PairWeftSibling(f.Slug))
						assertExists(tb, h.PairPortalLink(f.Slug))
						assertExists(tb, h.PairLauncherDir(f.Slug))
					},
				}
			default:
				return Expectation{
					Kind:           KindProceeds,
					PermittedRoots: slugPermittedRoots("verb-prune-owner"),
					Effect: func(tb testing.TB, h *hubforge.Hub, f VerbFixture) {
						tb.Helper()
						assertGone(tb, h.PairWeftSibling(f.Slug))
						assertGone(tb, h.PairPortalLink(f.Slug))
						assertGone(tb, h.PairLauncherDir(f.Slug))
					},
				}
			}
		},
	}
}

// cleanupCase builds Topology.Cleanup's VerbCase.
// Arrange produces an orphan managed branch: a pair is created, then its warp side (worktree and
// branch) is torn down by hand, leaving the weft branch with no live warp sibling.
// Arrange also moves the prime pair off the hub's default branch, exactly as
// TestCleanup_ProtectsPrimaryWeftBranchAfterCheckout (cleanup_primary_integration_test.go) does: the
// <Hub>/_board worktree stays on the original branch, which is what fabricengine.primaryWeftBranch
// reads to determine the repo's primary weft line (cleanup.go:206), so it alone still records the
// primary after the move. Without this, branch-space liveness alone would still protect the primary
// weft branch (some warp worktree is checked out on it), leaving this cell unable to independently
// exercise cleanup.go's own primaryWeft carve-out or the gate's primaryWeftBranch check -- both of
// which only matter once liveness alone stops protecting the primary line.
//
// Run calls Cleanup with force=true. Before raddleFoldedBack's removal this was required: the old
// fold-back gate unconditionally protected an orphan branch unless force was set, so without it no
// orphan branch could ever be deleted regardless of state. raddleFoldedBack is gone now, and
// Cleanup's own force parameter is reserved and consulted by no gate (see cleanup.go) — apply alone
// already deletes an orphan branch (TestCleanup_DryRunMatchesApplyVerdict). force=true is kept here
// as the pre-existing argument choice; it is no longer load-bearing for this cell's clean-state
// effect.
func cleanupCase() VerbCase {
	return VerbCase{
		Name: "Cleanup",
		Arrange: func(tb testing.TB, h *hubforge.Hub) VerbFixture {
			tb.Helper()

			original := currentBranchName(tb, h.PrimeWorktree())

			slug := "verb-cleanup-owner"
			hubforge.AddPair(tb, h, slug)
			mustGit(h.PrimeWorktree(), "worktree", "remove", "--force", h.PairWarpWorktree(slug))
			mustGit(h.PrimeWorktree(), "branch", "-D", slug)
			// The weft branch must not be checked out anywhere either: cleanup.go:162-166
			// unconditionally protects a checked-out branch, and the pair's own weft worktree is
			// still on it after only the warp side is torn down.
			mustGit(h.PrimeWeft(), "worktree", "remove", "--force", h.PairWeftSibling(slug))

			// Move the prime pair off the default branch. The _board worktree (never touched here)
			// keeps recording "original" as the primary warp branch, so the primary weft branch is
			// now neither the checked-out weft worktree's own branch nor named by any live pair --
			// exactly the condition that makes Cleanup's primaryWeft carve-out load-bearing.
			mustGit(h.PrimeWorktree(), "checkout", "-b", "verb-cleanup-alt")
			mustGit(h.PrimeWeft(), "checkout", "-b", fabricengine.WeftBranchName("verb-cleanup-alt"))

			return VerbFixture{
				Slug:           slug,
				OriginalBranch: original,
				Target: StateTarget{
					WarpCheckout: h.PrimeWorktree(),
					WeftCheckout: h.PairWeftSibling(slug),
				},
			}
		},
		Run: func(tb testing.TB, h *hubforge.Hub, f VerbFixture) (fabricengine.Mutations, error) {
			tb.Helper()
			res, err := h.Topology.Cleanup(h.Location, true, true)
			return res.Mutated(), err
		},
		Expect: func(state string) Expectation {
			return Expectation{
				Kind: KindProceeds,
				Effect: func(tb testing.TB, h *hubforge.Hub, f VerbFixture) {
					tb.Helper()
					orphan := fabricengine.WeftBranchName(f.Slug)
					if branchExists(h.PairWeftSibling(f.Slug), orphan) {
						tb.Errorf("Cleanup left orphan branch %q behind", orphan)
					}
					// Card 16's clean-state effect: "orphan managed branches gone, primary weft
					// branch intact." Arrange moved the prime pair off f.OriginalBranch precisely so
					// this assertion is independently provable here rather than only in
					// fabricengine_test's hermetic TestCleanup_ProtectsPrimaryWeftBranchAfterCheckout.
					primary := fabricengine.WeftBranchName(f.OriginalBranch)
					if !branchExists(h.PrimeWeft(), primary) {
						tb.Errorf("Cleanup deleted the primary weft branch %q", primary)
					}
				},
			}
		},
	}
}

// checkoutCase builds Topology.Checkout's VerbCase.
// Arrange adds a pair, and Run passes that pair's own warp branch — without this the cells are
// vacuous: a factory-built hub carries only the clone's own branch, so Checkout handed that branch
// no-ops.
func checkoutCase() VerbCase {
	return VerbCase{
		Name: "Checkout",
		Arrange: func(tb testing.TB, h *hubforge.Hub) VerbFixture {
			tb.Helper()

			slug := "verb-checkout-owner"
			hubforge.AddPair(tb, h, slug)
			// Remove the pair's own worktrees (never its branches), so both branches are free for
			// the prime pair to switch onto — mirroring the everyday "lyx fabric checkout" reuse
			// add.go's own error message points to ("switch a pair onto it"). Without this, `git
			// switch` refuses because the branch is already checked out at the pair worktree this
			// same Arrange just created.
			mustGit(h.PrimeWorktree(), "worktree", "remove", "--force", h.PairWarpWorktree(slug))
			mustGit(h.PrimeWeft(), "worktree", "remove", "--force", h.PairWeftSibling(slug))
			original := currentBranchName(tb, h.PrimeWorktree())

			return VerbFixture{
				Slug:           slug,
				OriginalBranch: original,
				CheckoutBranch: slug,
				Target: StateTarget{
					WarpCheckout: h.PrimeWorktree(),
					WeftCheckout: h.PrimeWeft(),
				},
			}
		},
		Run: func(tb testing.TB, h *hubforge.Hub, f VerbFixture) (fabricengine.Mutations, error) {
			tb.Helper()
			res, err := h.Topology.Checkout(h.Location, f.CheckoutBranch)
			return res.Mutated(), err
		},
		Expect: func(state string) Expectation {
			switch state {
			case "dirtyWeftTracked", "bothDirty":
				// Checkout's own pre-flight (checkout.go:42) is scopeTracked on the weft worktree; a
				// tracked modification there refuses before `git switch` ever runs.
				return Expectation{
					Kind:      KindRefusedBefore,
					Substring: "weft worktree has uncommitted changes",
					Effect: func(tb testing.TB, h *hubforge.Hub, f VerbFixture) {
						tb.Helper()
						if got := currentBranchName(tb, h.PrimeWorktree()); got != f.OriginalBranch {
							tb.Errorf("prime warp branch after refused Checkout = %q; want unchanged %q", got, f.OriginalBranch)
						}
						if got := currentBranchName(tb, h.PrimeWeft()); got != fabricengine.WeftBranchName(f.OriginalBranch) {
							tb.Errorf("prime weft branch after refused Checkout = %q; want unchanged %q", got, fabricengine.WeftBranchName(f.OriginalBranch))
						}
					},
				}
			case "dirtyWarpTracked":
				// Checkout has NO warp-side dirtiness probe at all (checkout.go:38-85 goes straight to
				// `git switch`), so the outcome is git's own to decide: `git switch` either carries the
				// tracked modification across, or refuses with "your local changes would be
				// overwritten". Both are correct; what must hold either way is that the pair is never
				// half-switched -- see this file's own doc comment for the fuller account.
				return Expectation{
					Kind:      KindEitherProceedsOrRefusedBefore,
					Substring: "warp switch",
					Effect: func(tb testing.TB, h *hubforge.Hub, f VerbFixture) {
						tb.Helper()
						warpBranch := currentBranchName(tb, h.PrimeWorktree())
						weftBranch := currentBranchName(tb, h.PrimeWeft())
						switched := warpBranch == f.CheckoutBranch
						weftSwitched := weftBranch == fabricengine.WeftBranchName(f.CheckoutBranch)
						if switched != weftSwitched {
							tb.Errorf("Checkout left the pair half-switched: warp branch = %q, weft branch = %q", warpBranch, weftBranch)
						}
						if !switched && warpBranch != f.OriginalBranch {
							tb.Errorf("Checkout neither switched to %q nor left the original branch %q; warp branch = %q", f.CheckoutBranch, f.OriginalBranch, warpBranch)
						}
						// The tracked modification must be on disk either way -- see this file's own
						// doc comment on why the git-status shape it reports (still tracked-modified,
						// or carried into a branch that never tracked this path) is git's to decide,
						// not the content surviving at all.
						assertTrackedContentSurvives(tb, h.PrimeWorktree(), sentinelFileName("dirtyWarpTracked"))
					},
				}
			default:
				return Expectation{
					Kind: KindProceeds,
					Effect: func(tb testing.TB, h *hubforge.Hub, f VerbFixture) {
						tb.Helper()
						if got := currentBranchName(tb, h.PrimeWorktree()); got != f.CheckoutBranch {
							tb.Errorf("prime warp branch after Checkout = %q; want %q", got, f.CheckoutBranch)
						}
						wantWeft := fabricengine.WeftBranchName(f.CheckoutBranch)
						if got := currentBranchName(tb, h.PrimeWeft()); got != wantWeft {
							tb.Errorf("prime weft branch after Checkout = %q; want %q", got, wantWeft)
						}
						if f.CheckoutBranch == f.OriginalBranch {
							tb.Errorf("Checkout's Arrange produced the same branch already checked out: %q", f.CheckoutBranch)
						}
					},
				}
			}
		},
	}
}

// reconcileCase builds Topology.Reconcile's VerbCase.
// Arrange needs no fixture beyond the built hub.
func reconcileCase() VerbCase {
	return VerbCase{
		Name: "Reconcile",
		Arrange: func(tb testing.TB, h *hubforge.Hub) VerbFixture {
			tb.Helper()
			return VerbFixture{
				Target: StateTarget{
					WarpCheckout: h.PrimeWorktree(),
					WeftCheckout: h.PrimeWeft(),
				},
			}
		},
		Run: func(tb testing.TB, h *hubforge.Hub, f VerbFixture) (fabricengine.Mutations, error) {
			tb.Helper()
			res, err := h.Topology.Reconcile(h.Location)
			return res.Mutated(), err
		},
		Expect: func(state string) Expectation {
			return Expectation{
				Kind: KindProceeds,
				Effect: func(tb testing.TB, h *hubforge.Hub, f VerbFixture) {
					tb.Helper()
					if _, err := h.Topology.Reconcile(h.Location); err != nil {
						tb.Errorf("second Reconcile call: %v", err)
					}
				},
			}
		},
	}
}

// unwireJunctionsCase builds fabricengine.UnwireJunctions' VerbCase.
// Arrange adds a pair; clean-state effect: junctions gone, worktree intact.
func unwireJunctionsCase() VerbCase {
	return VerbCase{
		Name: "UnwireJunctions",
		Arrange: func(tb testing.TB, h *hubforge.Hub) VerbFixture {
			tb.Helper()

			slug := "verb-unwire-owner"
			hubforge.AddPair(tb, h, slug)
			return VerbFixture{
				Slug: slug,
				Target: StateTarget{
					WarpCheckout: h.PairWarpWorktree(slug),
					WeftCheckout: h.PairWeftSibling(slug),
					// StructuralPath is the pair's own first wired junction link. unseedJunctionRecords
					// (junction.go:420-489) walks every wired junction in order and bails out the moment
					// one fails its own pre-flight (a foreign link's resolved target not matching the
					// expected weft target, or a dangling link that cannot be resolved at all) — before
					// the ownership gate ever runs for that link — so replanting just the first name is
					// enough to make the whole call refuse.
					StructuralPath: firstWiredJunctionLink(tb, h, slug),
				},
			}
		},
		Run: func(tb testing.TB, h *hubforge.Hub, f VerbFixture) (fabricengine.Mutations, error) {
			tb.Helper()
			names, err := fabricengine.WiredNames(h.BoardDir())
			if err != nil {
				tb.Fatalf("fabricengine.WiredNames(%s): %v", h.BoardDir(), err)
			}
			res, err := fabricengine.UnwireJunctions(h.Location, f.Slug, names)
			return res.Mutated(), err
		},
		Expect: func(state string) Expectation {
			// The wired junction names are structural (lyxdirs.LyxDirName, lyxdirs.DotLyxDirName),
			// never operator-editable per junctionnames.go, so both anchor variants of both names
			// are permitted unconditionally — matching slugPermittedRoots' "harmless if unused"
			// posture.
			var permitted []string
			for _, anchor := range []string{".", "backend"} {
				loc := &lyxcwd.Location{WorktreeName: "verb-unwire-owner", AnchorRel: anchor}
				for _, name := range []string{lyxdirs.LyxDirName, lyxdirs.DotLyxDirName} {
					permitted = append(permitted, filepath.ToSlash(fabricengine.WarpJunctions(loc, "verb-unwire-owner", []string{name})[0].Link))
				}
			}

			switch state {
			case "trackedSymlinkAtWiredPath":
				// unseedJunctionRecords resolves the link's actual target and compares it against the
				// weft target fabric itself expects (junction.go:465-472); an operator-owned link
				// resolves elsewhere, so the call refuses before the ownership gate ever runs, and
				// nothing -- not even the untouched second junction -- is removed.
				return Expectation{
					Kind:      KindRefusedBefore,
					Substring: "points to unexpected target",
				}
			case "staleWiredJunction":
				// The link's target no longer resolves at all, so fslink.PointsTo fails outright
				// (junction.go:461-464) before the ownership gate ever runs.
				return Expectation{
					Kind:      KindRefusedBefore,
					Substring: "resolve link target",
				}
			default:
				return Expectation{
					Kind:           KindProceeds,
					PermittedRoots: permitted,
					Effect: func(tb testing.TB, h *hubforge.Hub, f VerbFixture) {
						tb.Helper()
						names, err := fabricengine.WiredNames(h.BoardDir())
						if err != nil {
							tb.Fatalf("fabricengine.WiredNames(%s): %v", h.BoardDir(), err)
						}
						for _, j := range fabricengine.WarpJunctions(h.Location, f.Slug, names) {
							assertGone(tb, j.Link)
						}
						assertExists(tb, h.PairWarpWorktree(f.Slug))
					},
				}
			}
		},
	}
}

// pullCase builds Fabric.Pull's VerbCase (reached via fabricengine.Open).
// Arrange pushes a new commit to the scenario's own warp bare, or the entire column is vacuous:
// pull.go:210-212 returns early on localHEAD == upstreamSHA, before the dirty check and before any
// ResetHard.
func pullCase() VerbCase {
	return VerbCase{
		Name: "Pull",
		Arrange: func(tb testing.TB, h *hubforge.Hub) VerbFixture {
			tb.Helper()

			from := liveStateCurrentSHA(tb, h.PrimeWorktree())

			scratch := tb.TempDir()
			mustGit(filepath.Dir(scratch), "clone", h.WarpBare, filepath.Base(scratch))
			advanceFile := filepath.Join(scratch, "pull-advance.txt")
			if err := os.WriteFile(advanceFile, []byte("livestate: pull advance\n"), 0o644); err != nil {
				tb.Fatalf("write %s: %v", advanceFile, err)
			}
			mustGit(scratch, "add", "pull-advance.txt")
			mustGit(scratch, "commit", "-m", "livestate: advance warp bare for Pull")
			mustGit(scratch, "push", "origin", "HEAD:"+currentBranchName(tb, h.PrimeWorktree()))
			to := liveStateCurrentSHA(tb, scratch)

			return VerbFixture{
				AdvancedFromSHA: from,
				AdvancedToSHA:   to,
				Target: StateTarget{
					WarpCheckout: h.PrimeWorktree(),
					WeftCheckout: h.PrimeWeft(),
				},
			}
		},
		Run: func(tb testing.TB, h *hubforge.Hub, f VerbFixture) (fabricengine.Mutations, error) {
			tb.Helper()
			fab, err := fabricengine.Open(h.Location)
			if err != nil {
				tb.Fatalf("fabricengine.Open: %v", err)
			}
			res, err := fab.Pull(fabricengine.SyncOptions{})
			return res.Mutated(), err
		},
		Expect: func(state string) Expectation {
			switch state {
			case "dirtyWarpTracked", "bothDirty":
				// Pull's own pre-flight (pull.go:143) is scopeTracked on the prime warp worktree; a
				// tracked modification there refuses before ResetHard ever runs, and the warp bare's
				// advance (this cell's own Arrange) never lands.
				return Expectation{
					Kind:      KindRefusedBefore,
					Substring: "warp worktree has uncommitted changes",
					Effect: func(tb testing.TB, h *hubforge.Hub, f VerbFixture) {
						tb.Helper()
						// Not f.AdvancedFromSHA: dirtyWarpTracked's own plantTrackedDirt seeds a fresh
						// commit before leaving its modification uncommitted, so the prime warp's HEAD
						// has already moved past AdvancedFromSHA by the time Run executes. What a
						// refused Pull must guarantee is that it never reached ResetHard at all, i.e.
						// HEAD never advances all the way to the (unrelated, unreachable) upstream tip.
						if got := liveStateCurrentSHA(tb, h.PrimeWorktree()); got == f.AdvancedToSHA {
							tb.Errorf("prime warp HEAD after refused Pull = %s; want anything but the upstream tip %s", got, f.AdvancedToSHA)
						}
					},
				}
			default:
				return Expectation{
					Kind: KindProceeds,
					Effect: func(tb testing.TB, h *hubforge.Hub, f VerbFixture) {
						tb.Helper()
						if f.AdvancedFromSHA == f.AdvancedToSHA {
							tb.Errorf("Pull's Arrange did not advance the warp bare: from == to == %s", f.AdvancedFromSHA)
						}
						if got := liveStateCurrentSHA(tb, h.PrimeWorktree()); got != f.AdvancedToSHA {
							tb.Errorf("prime warp HEAD after Pull = %s; want %s", got, f.AdvancedToSHA)
						}
					},
				}
			}
		},
	}
}

// cloneHubResetNonHubCase builds the CloneHub{Reset: true} column's non-hub target: an operator
// directory whose name happens to match the derived <name>-HUB path is refused at resetHub's own
// pre-flight, contents fully surviving — R4's `clone --reset` defect.
func cloneHubResetNonHubCase() VerbCase {
	return VerbCase{
		Name:   "CloneHubReset/NonHubTarget",
		States: []string{"clean"},
		Arrange: func(tb testing.TB, h *hubforge.Hub) VerbFixture {
			tb.Helper()

			cwd := tb.TempDir()
			const warpURL = "https://example.invalid/verb-reset-non-hub.git"
			name := fabricengine.DeriveWarpName(warpURL)
			hubPath := fabricengine.HubPath(cwd, name)
			if err := os.MkdirAll(hubPath, 0o755); err != nil {
				tb.Fatalf("mkdir %s: %v", hubPath, err)
			}
			sentinel := filepath.Join(hubPath, "operator-content.txt")
			if err := os.WriteFile(sentinel, []byte("not a hub\n"), 0o644); err != nil {
				tb.Fatalf("write %s: %v", sentinel, err)
			}

			return VerbFixture{
				ResetCwd:     cwd,
				ResetWarpURL: warpURL,
				ResetWeftURL: "https://example.invalid/verb-reset-non-hub-weft.git",
				ResetHubPath: hubPath,
			}
		},
		Run: func(tb testing.TB, h *hubforge.Hub, f VerbFixture) (fabricengine.Mutations, error) {
			tb.Helper()
			res, err := fabricengine.CloneHub(f.ResetCwd, fabricengine.CloneOptions{
				WeftURL: f.ResetWeftURL,
				WarpURL: f.ResetWarpURL,
				Reset:   true,
			})
			return res.Mutated(), err
		},
		Expect: func(state string) Expectation {
			return Expectation{
				Kind:      KindRefusedBefore,
				Substring: "is not a fabric hub",
				Effect: func(tb testing.TB, h *hubforge.Hub, f VerbFixture) {
					tb.Helper()
					assertExists(tb, filepath.Join(f.ResetHubPath, "operator-content.txt"))
				},
			}
		},
	}
}

// cloneHubResetRealHubCase builds the CloneHub{Reset: true} column's positive target: a real hub torn
// down and rebuilt through fabriccli.CloneAndWire, proving the column is not trivially always-refusing.
func cloneHubResetRealHubCase() VerbCase {
	return VerbCase{
		Name:   "CloneHubReset/RealHub",
		States: []string{"clean"},
		Arrange: func(tb testing.TB, h *hubforge.Hub) VerbFixture {
			tb.Helper()
			return VerbFixture{
				ResetCwd:     h.Container,
				ResetWarpURL: filepath.ToSlash(h.WarpBare),
				ResetWeftURL: filepath.ToSlash(h.WeftBare),
				ResetHubPath: h.Path,
			}
		},
		Run: func(tb testing.TB, h *hubforge.Hub, f VerbFixture) (fabricengine.Mutations, error) {
			tb.Helper()
			res, err := fabriccli.CloneAndWire(f.ResetCwd, fabricengine.CloneOptions{
				WeftURL: f.ResetWeftURL,
				WarpURL: f.ResetWarpURL,
				Reset:   true,
			})
			return res.Mutated(), err
		},
		Expect: func(state string) Expectation {
			return Expectation{
				Kind: KindProceeds,
				Effect: func(tb testing.TB, h *hubforge.Hub, f VerbFixture) {
					tb.Helper()
					if _, err := os.Lstat(fabricengine.BoardDir(f.ResetHubPath)); err != nil {
						tb.Errorf("rebuilt hub missing %s: %v", fabricengine.BoardDir(f.ResetHubPath), err)
					}
				},
			}
		},
	}
}

// slugHostileInputs returns the seven slug-shaped hostile inputs Add and Remove each take, paired with
// the expectation validateWorktreeSlug (slug.go:30) actually produces for each — except the trailing
// leading-dash input, which validateWorktreeSlug does not reject at all.
func slugHostileInputs() []struct {
	name  string
	input string
	want  string // empty means "not rejected by validateWorktreeSlug; safe-proceeds expectation instead"
} {
	return []struct {
		name  string
		input string
		want  string
	}{
		{"Empty", "", "a slug must not be empty"},
		{"Dot", ".", "a slug must name a directory"},
		{"DotDot", "..", "a slug must name a directory"},
		{"Escaping", "../x", "a slug must be a single path component"},
		{"WeftSuffixed", "hostile" + weftname.Suffix, "that suffix is reserved for weft worktrees"},
		{"ReservedHubName", fabricengine.BoardDirName, "that name is reserved for lyx hub geometry"},
		{"LeadingDash", "-flag", ""},
	}
}

// addHostileCases returns Add's seven hostile-input VerbCase entries.
func addHostileCases() []VerbCase {
	var cases []VerbCase
	for _, in := range slugHostileInputs() {
		in := in
		cases = append(cases, VerbCase{
			Name:   "Add/" + in.name,
			States: []string{"clean"},
			Arrange: func(tb testing.TB, h *hubforge.Hub) VerbFixture {
				return VerbFixture{Slug: in.input}
			},
			Run: func(tb testing.TB, h *hubforge.Hub, f VerbFixture) (fabricengine.Mutations, error) {
				tb.Helper()
				res, err := h.Topology.Add(h.Location, f.Slug, fabricengine.AddOptions{})
				return res.Mutated(), err
			},
			Expect: func(state string) Expectation {
				if in.want != "" {
					return Expectation{
						Kind:      KindRefusedBefore,
						Substring: in.want,
						Effect: func(tb testing.TB, h *hubforge.Hub, f VerbFixture) {
							tb.Helper()
							assertGone(tb, filepath.Join(h.Path, "verb-add-hostile-sentinel-never-created"))
						},
					}
				}
				// LeadingDash: validateWorktreeSlug does not reject it. `git worktree add -b -flag
				// <target>` genuinely fails (git's own arg parser treats "-flag" as an unknown
				// option to the branch-creation step), so nothing is ever created — the safe
				// outcome this cell exists to prove, verified live rather than assumed.
				return Expectation{
					Kind:      KindRefusedBefore,
					Substring: "worktree",
					Effect: func(tb testing.TB, h *hubforge.Hub, f VerbFixture) {
						tb.Helper()
						assertGone(tb, h.PairWarpWorktree(f.Slug))
					},
				}
			},
		})
	}
	return cases
}

// removeHostileCases returns Remove's seven hostile-input VerbCase entries.
// No pair is arranged for any of them: every one of the seven inputs is refused before Remove ever
// looks at whether a pair by that name exists, and arranging one would risk the very whole-hub deletion
// the ".." case exists to prove is refused.
func removeHostileCases() []VerbCase {
	var cases []VerbCase
	for _, in := range slugHostileInputs() {
		in := in
		cases = append(cases, VerbCase{
			Name:   "Remove/" + in.name,
			States: []string{"clean"},
			Arrange: func(tb testing.TB, h *hubforge.Hub) VerbFixture {
				return VerbFixture{Slug: in.input}
			},
			Run: func(tb testing.TB, h *hubforge.Hub, f VerbFixture) (fabricengine.Mutations, error) {
				tb.Helper()
				res, err := h.Topology.Remove(h.Location, f.Slug, false)
				return res.Mutated(), err
			},
			Expect: func(state string) Expectation {
				want := in.want
				if want == "" {
					// LeadingDash: validateWorktreeSlug does not reject it, so Remove reaches its
					// target-exists stat, finds nothing named "-flag", and refuses safely.
					want = "not found"
				}
				return Expectation{
					Kind:      KindRefusedBefore,
					Substring: want,
					Effect: func(tb testing.TB, h *hubforge.Hub, f VerbFixture) {
						tb.Helper()
						assertExists(tb, h.Path)
					},
				}
			},
		})
	}
	return cases
}

// checkoutHostileCases returns Checkout's two branch-shaped hostile-input VerbCase entries.
// checkout.go:38-85 validates no branch at all, so both inputs reach `git switch` directly; both fail
// safely (git itself refuses a nonexistent branch and an option-shaped argument alike), and both cells
// additionally assert no half-switched pair.
func checkoutHostileCases() []VerbCase {
	inputs := []struct {
		name  string
		input string
	}{
		{"NonexistentBranch", "verb-checkout-hostile-nonexistent-branch"},
		{"FlagShapedName", "-flag"},
	}

	var cases []VerbCase
	for _, in := range inputs {
		in := in
		cases = append(cases, VerbCase{
			Name:   "Checkout/" + in.name,
			States: []string{"clean"},
			Arrange: func(tb testing.TB, h *hubforge.Hub) VerbFixture {
				tb.Helper()
				return VerbFixture{
					OriginalBranch: currentBranchName(tb, h.PrimeWorktree()),
					CheckoutBranch: in.input,
				}
			},
			Run: func(tb testing.TB, h *hubforge.Hub, f VerbFixture) (fabricengine.Mutations, error) {
				tb.Helper()
				res, err := h.Topology.Checkout(h.Location, f.CheckoutBranch)
				return res.Mutated(), err
			},
			Expect: func(state string) Expectation {
				return Expectation{
					Kind:      KindRefusedBefore,
					Substring: "warp switch",
					Effect: func(tb testing.TB, h *hubforge.Hub, f VerbFixture) {
						tb.Helper()
						if got := currentBranchName(tb, h.PrimeWorktree()); got != f.OriginalBranch {
							tb.Errorf("prime warp branch after refused Checkout = %q; want unchanged %q", got, f.OriginalBranch)
						}
					},
				}
			},
		})
	}
	return cases
}

// unwireJunctionsHostileCase returns UnwireJunctions' one hostile-input VerbCase: a junction name
// escaping its worktree ("../x"), the sole route to the link executor's containment check.
//
// The junction-name join is WorktreePath(l,slug)/AnchorRel/name (junction.go's WarpJunctions), so the
// number of ".." segments needed to walk all the way out of the hub depends on AnchorRel's own depth:
// one ".." cancels "slug" alone at anchor ".", but two are needed at anchor "backend" (one for
// "backend", one for "slug") — escapeRelName computes exactly that depth so the escape genuinely
// reaches outside the hub at both anchors, rather than only at ".".
func unwireJunctionsHostileCase() VerbCase {
	const escapeName = "verb-unwire-hostile-escape-link"

	return VerbCase{
		Name:   "UnwireJunctions/EscapingJunctionName",
		States: []string{"clean"},
		Arrange: func(tb testing.TB, h *hubforge.Hub) VerbFixture {
			tb.Helper()

			slug := "verb-unwire-hostile-owner"
			escapeLink := filepath.Join(h.Path, escapeName)
			elsewhere := tb.TempDir()
			if err := fslink.CreateDirLink(escapeLink, elsewhere); err != nil {
				tb.Fatalf("create escape link %s: %v", escapeLink, err)
			}
			return VerbFixture{Slug: slug}
		},
		Run: func(tb testing.TB, h *hubforge.Hub, f VerbFixture) (fabricengine.Mutations, error) {
			tb.Helper()
			res, err := fabricengine.UnwireJunctions(h.Location, f.Slug, []string{escapeRelName(h.Anchor, escapeName)})
			return res.Mutated(), err
		},
		Expect: func(state string) Expectation {
			return Expectation{
				Kind:  KindRefusedByGate,
				Check: fabricengine.CheckContainment,
				Effect: func(tb testing.TB, h *hubforge.Hub, f VerbFixture) {
					tb.Helper()
					assertExists(tb, filepath.Join(h.Path, escapeName))
				},
			}
		},
	}
}

// Verbs is the exported tranche-1 verb table: the eight ordinary verbs, the two CloneHub{Reset: true}
// targets, and the seventeen hostile-input cases.
// This is the external interface batch 7's cross-product driver consumes.
var Verbs = buildVerbs()

func buildVerbs() []VerbCase {
	verbs := []VerbCase{
		addCase(),
		removeCase(),
		pruneCase(),
		cleanupCase(),
		checkoutCase(),
		reconcileCase(),
		unwireJunctionsCase(),
		pullCase(),
		cloneHubResetNonHubCase(),
		cloneHubResetRealHubCase(),
	}
	verbs = append(verbs, addHostileCases()...)
	verbs = append(verbs, removeHostileCases()...)
	verbs = append(verbs, checkoutHostileCases()...)
	verbs = append(verbs, unwireJunctionsHostileCase())
	return verbs
}
