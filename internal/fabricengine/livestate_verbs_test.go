// verbs.go builds the verb table for the gate-reaching verbs.
// Each verb has its own Build and Arrange fixture steps, Run call, and Expect function.
// The Expect function derives a per-state Expectation.
// The file also holds the hostile-input cases and the omission table of excluded verb/state pairs.
//
// # The verified dirtiness-scope table
//
// Every dirtiness cell's outcome is DERIVED from this table, never observed and written down.
// A cell written from observed behaviour asserts nothing, so a green suite can hide data-loss defects.
// The table covers the declarations reachable with force=false.
// It is not an exhaustive index of every pathRequest in the package.
// Remove's weft-side gate rows in weftwiring.go (dirtyScopeAll, dirtyCheckedOutBranch) are deliberately absent.
// So are the Checkout call in checkout.go and the Add calls in add.go.
// None of them changes a cell's derived outcome here.
// A later change that adds a force axis must extend this table rather than assume it already covers them.
//
//   - Add own pre-flight (add.go) — scopeTracked, the prime warp worktree.
//   - Checkout own pre-flight (checkout.go) — scopeTracked, the weft worktree.
//     Checkout has no warp-side dirtiness probe at all; it goes straight to `git switch`.
//   - Remove own pre-flight (remove.go) — scopeAll, the pair's warp worktree.
//   - Remove's refuseDirtyWeftWorktree (remove.go) — scopeAll, the pair's weft worktree.
//   - Remove's gate call (remove.go) — dirtyScopeAll, the pair's warp worktree.
//   - Remove's gate call (launchers.go) — dirtinessNA.
//   - Prune own pre-flight (prune.go) — scopeTracked, the weft path.
//   - Prune's gate call (prune.go) — dirtyScopeTracked, stale pair paths.
//   - Pull own pre-flight (pull.go) — scopeTracked, the prime warp worktree.
//   - Pull's gate call via Fabric.ResetHard (destroy.go) — dirtyScopeTracked, the prime warp worktree.
//   - Reconcile own pre-flight (reconcile.go) — scopeAll, the board.
//   - Cleanup's gate call — branch-shaped, dirtyCheckedOutBranch.
//   - UnwireJunctions' gate call — link-shaped, dirtinessNA.
//   - CloneHub{Reset}'s gate call (clone.go) — dirtinessNA.
//
// The derivation rule: a scopeTracked verb against an untracked-only state expects Proceeds;
// a scopeAll verb against the same state expects a refusal;
// a verb with no probe for the dirtied side gets an explicit per-cell expectation, never a derived one.
// dirtyWarpTracked x Checkout is the only such case, and git decides it.
// `git switch` either carries the tracked modification across or refuses with "your local changes would be overwritten".
// The cell therefore asserts what must hold either way: the tracked modification is still on disk afterwards,
// and the pair is not half-switched, meaning both sides moved or neither did.
// Guessing which branch git takes would assert git's behaviour rather than fabric's.
// The disjunction still catches what matters.
// Checkout's documented all-or-nothing rollback exists to prevent a half-switched pair.
//
// # CloneHub{Reset: true}
//
// This column is re-scoped to the ownership axis and is not run against the state matrix.
// resetHub declares dirtinessNA, so no dirtiness state can refuse it.
// Total hub destruction on --reset is correct behaviour.
// If a dirtiness state could refuse it, the permitted root would be the hub root itself,
// and the manifest diff would be vacuous across every state in the matrix.
// The gate is structurally unreachable for Reset.
// CloneHub refuses a non-hub offline in checkResetTarget, which calls looksLikeHub.
// That refusal comes before resetHub builds its pathRequest.
// The gate's pathOwnershipFabricHub predicate calls that same looksLikeHub,
// so a RefusedByGate(CheckOwnership) expectation would fail against a correct binary.
// This column therefore uses RefusedBefore("is not a fabric hub") and drives exactly two targets.
// The first is a <derived>-LYMHUB-named directory that is not a hub.
// The pre-flight refuses it, and its contents survive.
// The second is a real hub, torn down and rebuilt through fabriccli.CloneAndWire.
// It is the positive case: the column is not trivially always-refusing.
//
// # The hostile-input cases
//
// All carry States: []string{"clean"} and both anchors.
// Add and Remove each take the same slug-shaped inputs:
// "", ".", "..", "../x", a weftname.Suffix-suffixed name, a reserved hub name, and a leading "-".
// Checkout takes two branch-shaped inputs: a nonexistent branch and a flag-shaped name.
// The checkout code validates no branch at all.
// UnwireJunctions takes one input: a junction name escaping its worktree ("../x").
// It is in scope although it takes neither a slug nor a branch.
// Its names []string is the only route to the link executor's containment check.
// Each input's expectation is derived from what validateWorktreeSlug actually rejects.
// It never asserts whatever the current behaviour happens to be, since that would assert nothing.
// A leading "-" is NOT rejected by validateWorktreeSlug: it is on none of that function's rejection rules.
// No gate check owns it, so neither refusal kind applies.
// Those cells, and the Checkout hostile cells, are written against the safe expectation:
// the argument never reaches git as a flag, and nothing outside the permitted roots is destroyed.

package fabricengine_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/configengine"
	"github.com/Knatte18/loomyard/internal/fabriccli"
	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/fslink"
	"github.com/Knatte18/loomyard/internal/gitkit"
	"github.com/Knatte18/loomyard/internal/hubforge"
	"github.com/Knatte18/loomyard/internal/lymcwd"
	"github.com/Knatte18/loomyard/internal/lymdirs"
	"github.com/Knatte18/loomyard/internal/weftname"
)

// VerbFixture carries what a VerbCase's Arrange produced: a pair slug, an advanced upstream SHA,
// a broken remote, a checked-out branch, and the StateTarget the state builders need.
// Each Arrange populates only the fields that its own Run and Expect closures consume.
type VerbFixture struct {
	// Slug is the pair slug this cell operates on, when the verb under test takes one.
	Slug string
	// Target is the StateTarget that a hostile or dirty state plants against.
	// Each verb resolves it from the verified dirtiness-scope table above.
	Target StateTarget
	// LauncherBlocker is a directory planted at the pair's launcher script path, failing Add's launcher step.
	LauncherBlocker string
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

// ExpectationKind is the closed set of outcomes a cell can declare.
// A cell can expect a refusal from fabric's destructive gate, or from one of fabric's pre-flight checks.
// It can also expect the verb to proceed to its intended effect.
// A cell whose outcome git decides, not fabric, may accept either a proceed or a pre-flight refusal.
type ExpectationKind int

const (
	// KindRefusedByGate means the error contains "<check> check failed" for the declared Check.
	KindRefusedByGate ExpectationKind = iota
	// KindRefusedBefore means the error contains the declared substring,
	// and it does not contain "check failed".
	KindRefusedBefore
	// KindProceeds means the verb succeeds and its intended effect lands.
	// The state's planted content survives the call.
	KindProceeds
	// KindEitherProceedsOrRefusedBefore means git, not fabric, decides the outcome.
	// The verb may succeed, with err == nil.
	// Or it may fail with a RefusedBefore-shaped error carrying the declared Substring.
	// The driver has no per-verb branch for this kind.
	// A cell names it as data, one more closed outcome beside the other kinds.
	// Cells whose outcome fabric decides use the other kinds.
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

// Expectation is what a cell, one verb case run against one named state, is derived to produce.
// PermittedRoots is set for every Kind, since a refusal is not assumed side-effect-free.
// Effect is set for clean-state cells regardless of Kind.
// It carries a verb-specific check beyond the generic error-shape and manifest-diff checks every cell gets.
type Expectation struct {
	Kind           ExpectationKind
	Check          fabricengine.Check
	Substring      string
	PermittedRoots []string
	Effect         func(tb testing.TB, h *hubforge.Hub, f VerbFixture)
}

// VerbCase is one verb under test.
// It holds the fixture builder, the call into fabric, the states it runs against, and the Expectation it derives.
// Arrange is a required field, not a convenience: most verbs need a fixture no state builds.
// Folding that work into Run would be a correctness bug.
// The before-manifest is captured before Run, so a mutation folded into Run would show up as an unpermitted change.
// Build is optional and holds the fixture steps a hubforge template build can run, such as adding a pair.
// It runs once per anchor in a registered template, and each cell copies that template.
// Arrange then derives the fixture from the copy's Hub.
// It keeps only the steps a build cannot run, such as placing a directory outside the hub.
// States is an optional restriction.
// Empty means the case inherits every state in the package-level States slice, which ordinary verbs use.
// So a newly appended verb gets the full matrix automatically.
// Only hostile-input cases and the CloneHub{Reset} column set it.
// Their state's Apply is never invoked.
// It is the no-op "clean" state, or for CloneHub{Reset} a pseudo-state name that Arrange resolves.
// Run returns the call's own fabricengine.Mutations record alongside its error.
// The verbs under test return heterogeneous result types, so cells read the record through one uniform accessor.
// That accessor is MutationRecord.Mutated().
// Discarding the typed result would leave nothing for a cell to cross-check against the manifest diff.
type VerbCase struct {
	Name    string
	Build   func(tb testing.TB, h *hubforge.Hub)
	Arrange func(tb testing.TB, h *hubforge.Hub) VerbFixture
	Run     func(tb testing.TB, h *hubforge.Hub, f VerbFixture) (fabricengine.Mutations, error)
	States  []string
	Expect  func(state string) Expectation
}

// Omission names one verb/state pair excluded from the cross product, with the reason.
// Recording the pair keeps the gap visible, since an absent pair reads like coverage.
type Omission struct {
	Verb   string
	State  string
	Reason string
}

// Omissions is the durable, exported record of every verb/state pair excluded from the cross product.
// Each row is derived from the verified dirtiness-scope table and the production call sites its verb reaches.
// No row is derived from what a test run happened to produce.
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
	{Verb: "UnwireJunctions", State: "unrelatedGitCloneAtWeftNamedPath", Reason: "UnwireJunctions' only gate call is the removeLink inside unseedJunctionRecords (junction.go:474-483), reached via unseedLymJunction; it never visits a weft-named path"},

	// Reconcile has no path-executor gate call: reconcile.go contains no pathRequest.
	// Its pre-flight is scopeAll on the board, so no structural state, of any shape, names a path it inspects.
	{Verb: "Reconcile", State: "trackedSymlinkAtWiredPath", Reason: "Reconcile has no path-executor gate call (reconcile.go contains no pathRequest); its own pre-flight is scopeAll on the board, not a per-path check"},
	{Verb: "Reconcile", State: "staleWiredJunction", Reason: "Reconcile has no path-executor gate call (reconcile.go contains no pathRequest); its own pre-flight is scopeAll on the board, not a per-path check"},
	{Verb: "Reconcile", State: "foreignDirAtFabricOwnedPath", Reason: "Reconcile has no path-executor gate call (reconcile.go contains no pathRequest); its own pre-flight is scopeAll on the board, not a per-path check"},
	{Verb: "Reconcile", State: "unrelatedGitCloneAtWeftNamedPath", Reason: "Reconcile has no path-executor gate call (reconcile.go contains no pathRequest); its own pre-flight is scopeAll on the board, not a per-path check"},

	// Cleanup's Build removes the pair's weft worktree by hand before the state is applied.
	// No weft-targeted dirtiness state can plant into a checkout that no longer exists.
	{Verb: "Cleanup", State: "dirtyWeftTracked", Reason: "Cleanup's Build removes the pair's weft worktree before the state is applied, mirroring cleanup's own orphan-branch precondition; the checkout a weft dirtiness state would plant into does not exist"},
	{Verb: "Cleanup", State: "dirtyWeftUntracked", Reason: "Cleanup's Build removes the pair's weft worktree before the state is applied, mirroring cleanup's own orphan-branch precondition; the checkout a weft dirtiness state would plant into does not exist"},
	{Verb: "Cleanup", State: "bothDirty", Reason: "Cleanup's Build removes the pair's weft worktree before the state is applied, mirroring cleanup's own orphan-branch precondition; the checkout bothDirty's weft half would plant into does not exist"},

	// Remove's own path-executor gate calls target only the pair's registered worktree and its own wired junctions.
	// No ownership check reaches an independent foreign directory or unrelated clone at another path.
	// The link-shaped states trackedSymlinkAtWiredPath and staleWiredJunction cover the wired junctions.
	{Verb: "Remove", State: "foreignDirAtFabricOwnedPath", Reason: "Remove's own path-executor gate calls target the pair's registered worktree and its own wired junctions; no ownership check reaches an independent foreign directory at another path"},
	{Verb: "Remove", State: "unrelatedGitCloneAtWeftNamedPath", Reason: "Remove's own path-executor gate calls target the pair's registered worktree and its own wired junctions; no ownership check reaches an independent unrelated clone at another weft-named path"},

	// Prune's only path-executor gate call targets a directory, never a link.
	// It has no removeLink or repointLink call, so no link-shaped structural state names a path it acts on.
	{Verb: "Prune", State: "trackedSymlinkAtWiredPath", Reason: "Prune's only path-executor gate call targets a directory (prune.go:264, ownedRegisteredLinkedWorktree); it has no removeLink/repointLink call, so no link-shaped structural state names a path it acts on"},
	{Verb: "Prune", State: "staleWiredJunction", Reason: "Prune's only path-executor gate call targets a directory (prune.go:264, ownedRegisteredLinkedWorktree); it has no removeLink/repointLink call, so no link-shaped structural state names a path it acts on"},

	// UnwireJunctions' StructuralPath is the pair's own pre-existing wired junction link.
	// foreignDirAtFabricOwnedPathState's Apply does not remove that link first.
	// Its os.MkdirAll would silently write through the intact link instead of parking a foreign directory at the path.
	{Verb: "UnwireJunctions", State: "foreignDirAtFabricOwnedPath", Reason: "UnwireJunctions' StructuralPath is the pair's own pre-existing wired junction link; foreignDirAtFabricOwnedPathState's Apply does not remove it first, so os.MkdirAll would silently no-op through the intact link instead of parking a foreign directory"},
}

// hubRelative returns abs relative to h.Path, slash-normalised, as a manifest-relative path.
// Geometry paths come from fabricengine accessors, never from raw literals.
func hubRelative(tb testing.TB, h *hubforge.Hub, abs string) string {
	tb.Helper()

	rel, err := filepath.Rel(h.Path, abs)
	if err != nil {
		tb.Fatalf("hubRelative(%s): %v", abs, err)
	}
	return filepath.ToSlash(rel)
}

// firstWiredJunctionLink returns the warp-side path of slug's first wired junction, such as "_lym".
// It resolves the path through fabricengine.WiredNames and fabricengine.WarpJunctions.
// The result is a real, pre-existing link that a cell can remove and replant.
func firstWiredJunctionLink(tb testing.TB, h *hubforge.Hub, slug string) string {
	tb.Helper()

	names, err := fabricengine.WiredNames(configengine.Dirs{Board: h.BoardDir()})
	if err != nil {
		tb.Fatalf("fabricengine.WiredNames(%s): %v", h.BoardDir(), err)
	}
	if len(names) == 0 {
		tb.Fatalf("fabricengine.WiredNames(%s): no wired names", h.BoardDir())
	}
	return fabricengine.WarpJunctions(h.Location, slug, names)[0].Link
}

// pairPermittedRoots returns the hub-relative roots a pair's full teardown or creation may touch:
// its warp worktree, its weft sibling, and its launcher directory.
func pairPermittedRoots(tb testing.TB, h *hubforge.Hub, slug string) []string {
	tb.Helper()

	return []string{
		hubRelative(tb, h, h.PairCodeWorktree(slug)),
		hubRelative(tb, h, h.PairRecordsSibling(slug)),
		hubRelative(tb, h, h.PairLauncherDir(slug)),
	}
}

// slugPermittedRoots returns the hub-relative roots a pair named slug may occupy, unioned across
// both anchors ("." and "backend").
// A VerbCase's Expect closure has no *hubforge.Hub to resolve an accessor against, only the state name.
// A permitted root that matches nothing in a given hub's anchor is harmless: DiffManifest's
// pathPermitted only narrows what a diff flags, so an unused anchor's root cannot hide an unpermitted change.
// The *lymcwd.Location built below leaves HubPath empty.
// That is valid because every accessor this function calls composes paths with filepath.Join.
// filepath.Join drops an empty leading element, so each accessor yields exactly the hub-relative suffix.
func slugPermittedRoots(slug string) []string {
	// A Remove or Prune cell rewrites the hub workspace file, which is named after the warp repo.
	roots := []string{filepath.ToSlash(fabricengine.HubWorkspacePath(&lymcwd.Location{RepoName: "warp-bare"}))}
	for _, anchor := range []string{".", "backend"} {
		loc := &lymcwd.Location{WorktreeName: slug, AnchorRel: anchor}
		launcher := fabricengine.LauncherDir(loc, slug)
		roots = append(roots,
			filepath.ToSlash(slug),
			filepath.ToSlash(weftname.SiblingPath("", slug)),
			filepath.ToSlash(launcher),
			// pruneEmptyAncestors removes an anchor-level parent left empty by the leaf's own
			// removal (e.g. "_launchers/backend" once "_launchers/backend/<slug>" is gone); the parent
			// itself is therefore also a legitimate part of a full pair teardown, not an
			// unpermitted change.
			filepath.ToSlash(filepath.Dir(launcher)),
			primeWorktreeAdminPermittedRoot(slug),
			primeWeftAdminPermittedRoot(slug),
		)
	}
	return roots
}

// escapeRelName returns a "../"-prefixed junction name.
// Joined onto the worktree path and anchorRel, it walks out of the hub.
// It carries one ".." for the slug segment, plus one per anchorRel path segment; an anchorRel of "." adds none.
func escapeRelName(anchorRel, name string) string {
	depth := 1
	if anchorRel != "." {
		depth += len(strings.Split(anchorRel, "/"))
	}
	return strings.Repeat("../", depth) + name
}

// primeWorktreeAdminPermittedRoot returns the hub-relative path of the git-admin entry for the linked worktree slug.
// It is the ".git/worktrees/<slug>" directory that a successful `git worktree remove` clears in any pair teardown.
// "warp-bare" is this package's fixed name for the prime warp worktree.
func primeWorktreeAdminPermittedRoot(slug string) string {
	return filepath.ToSlash(filepath.Join("warp-bare", ".git", "worktrees", slug))
}

// primeWeftAdminPermittedRoot returns the hub-relative path of the git-admin entry for the weft sibling of slug.
// It is the ".git/worktrees/<slug-weft>" entry of the prime weft sibling.
// A successful weft-worktree teardown clears it.
// "warp-bare-weft" is this package's fixed name for the prime weft sibling.
func primeWeftAdminPermittedRoot(slug string) string {
	return filepath.ToSlash(filepath.Join("warp-bare-weft", ".git", "worktrees", weftname.SiblingPath("", slug)))
}

// assertGone fails tb unless path does not exist.
// It uses os.Lstat, so a dangling link still counts as present.
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
// Arrange plants a directory at the pair's ide launcher script path.
// Add's launcher step then fails after the branch and both worktrees already exist, which triggers rollbackAdd.
// Without it, Add never reaches the gate.
// rollbackAdd fires only at post-creation sites, and no state induces one on its own.
//
// The arrangement sends every path that the pre-flight does not refuse through rollbackAdd.
// On a clean hub, rollback's removal calls succeed unconditionally.
// A clean-state Run therefore returns the launcher error.
// It leaves the hub as it was before Run, apart from the blocker.
// That is add.go's documented rollback contract, so this case expects RefusedBefore rather than Proceeds.
// The case has no Build: the blocker is one directory and no git step, so a template would only add a build.
func addCase() VerbCase {
	return VerbCase{
		Name: "Add",
		Arrange: func(tb testing.TB, h *hubforge.Hub) VerbFixture {
			tb.Helper()

			slug := "verb-add-owner"
			blocker := plantLauncherBlocker(tb, h.Location, slug)

			return VerbFixture{
				Slug:            slug,
				LauncherBlocker: blocker,
				Target: StateTarget{
					WarpCheckout: h.PrimeWorktree(),
					WeftCheckout: h.PrimeRecords(),
					// StructuralPath is the pair's warp worktree path, which does not exist yet.
					// The link-shaped structural states are omitted for Add: junction paths do not exist before Run.
					// The directory-shaped states remain.
					// They plant fresh content with os.MkdirAll rather than replacing an existing link.
					StructuralPath: h.PairCodeWorktree(slug),
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
				// Add's pre-flight refuses a dirty tracked prime warp worktree before anything is created.
				// The broken remote is never reached.
				return Expectation{
					Kind:      KindRefusedBefore,
					Substring: "source worktree has uncommitted changes",
					Effect: func(tb testing.TB, h *hubforge.Hub, f VerbFixture) {
						assertGone(tb, h.PairCodeWorktree(f.Slug))
					},
				}
			case "foreignDirAtFabricOwnedPath", "unrelatedGitCloneAtWeftNamedPath":
				// The state plants content directly at the pair's warp worktree path before Run.
				// git worktree add then refuses, because its target directory already exists and is non-empty.
				// That is a git-level pre-flight failure, reached before the branch is created.
				// The broken remote is never consulted, so neither the branch nor any wiring is created.
				return Expectation{
					Kind:      KindRefusedBefore,
					Substring: "already exists",
					Effect: func(tb testing.TB, h *hubforge.Hub, f VerbFixture) {
						tb.Helper()
						if gitkit.BranchExists(tb, h.PrimeWorktree(), f.Slug) {
							tb.Errorf("Add unexpectedly created branch %q despite refusing at worktree creation", f.Slug)
						}
					},
				}
			default:
				// Every other state's own pre-flight (or absence of one) lets Add proceed through both worktrees;
				// the launcher blocker then fails the launcher step unconditionally, its error naming the script path and so the slug.
				// rollbackAdd reverts every step it can.
				// The warp branch is reverted too, whatever the BranchPrefix.
				// The rollback removes the planted blocker and its launcher directory, which the pair's roots permit.
				return Expectation{
					Kind:           KindRefusedBefore,
					PermittedRoots: slugPermittedRoots("verb-add-owner"),
					Substring:      "verb-add-owner",
					Effect: func(tb testing.TB, h *hubforge.Hub, f VerbFixture) {
						tb.Helper()
						assertGone(tb, h.PairCodeWorktree(f.Slug))
						assertGone(tb, h.PairRecordsSibling(f.Slug))
						assertGone(tb, h.PairLauncherDir(f.Slug))
						if gitkit.BranchExists(tb, h.PrimeWorktree(), f.Slug) {
							tb.Errorf("Add's rollback left branch %q behind", f.Slug)
						}
					},
				}
			}
		},
	}
}

// removeCase builds Topology.Remove's VerbCase.
// Its dirty-refusal cells declare no permitted roots, because a correctly-refusing cell changes nothing.
func removeCase() VerbCase {
	const slug = "verb-remove-owner"
	return VerbCase{
		Name: "Remove",
		Build: func(tb testing.TB, h *hubforge.Hub) {
			tb.Helper()
			hubforge.AddPair(tb, h, slug)
		},
		Arrange: func(tb testing.TB, h *hubforge.Hub) VerbFixture {
			tb.Helper()
			return VerbFixture{
				Slug: slug,
				Target: StateTarget{
					WarpCheckout: h.PairCodeWorktree(slug),
					WeftCheckout: h.PairRecordsSibling(slug),
					// StructuralPath is the pair's own wired junction link.
					// The link sweep in remove.go (scanOnDiskJunctionNames and removeWarpJunction) is ownership-filtered;
					// it removes only the links fabric itself created there.
					// The two link-shaped structural states exercise that filter.
					// The two directory-shaped states are omitted for Remove (see Omissions):
					// its ownership gate never reaches an independent foreign directory.
					StructuralPath: firstWiredJunctionLink(tb, h, slug),
				},
			}
		},
		Run: func(tb testing.TB, h *hubforge.Hub, f VerbFixture) (fabricengine.Mutations, error) {
			tb.Helper()
			res, err := h.Topology.Remove(h.Location, f.Slug, false, false)
			return res.Mutated(), err
		},
		Expect: func(state string) Expectation {
			switch state {
			case "dirtyWarpTracked", "dirtyWarpUntracked", "dirtyWeftTracked", "dirtyWeftUntracked", "bothDirty":
				// Remove runs both no-force dirtiness checks before its archive and every teardown,
				// so a refused cell leaves the launchers and warp worktree in place.
				return Expectation{
					Kind:      KindRefusedBefore,
					Substring: "uncommitted changes",
					Effect: func(tb testing.TB, h *hubforge.Hub, f VerbFixture) {
						assertExists(tb, h.PairLauncherDir(f.Slug))
						assertExists(tb, h.PairCodeWorktree(f.Slug))
					},
				}
			default:
				return Expectation{
					Kind:           KindProceeds,
					PermittedRoots: slugPermittedRoots(slug),
					Effect: func(tb testing.TB, h *hubforge.Hub, f VerbFixture) {
						tb.Helper()
						assertGone(tb, h.PairCodeWorktree(f.Slug))
						assertGone(tb, h.PairRecordsSibling(f.Slug))
						assertGone(tb, h.PairLauncherDir(f.Slug))
					},
				}
			}
		},
	}
}

// pruneCase builds Topology.Prune's VerbCase.
// Build produces a stale pair: the warp worktree directory is removed by hand, bypassing fabric,
// so the weft sibling and warp's worktree registration stay behind, as an operator's manual `rm -rf` would.
func pruneCase() VerbCase {
	const slug = "verb-prune-owner"
	return VerbCase{
		Name: "Prune",
		Build: func(tb testing.TB, h *hubforge.Hub) {
			tb.Helper()

			hubforge.AddPair(tb, h, slug)
			if err := os.RemoveAll(h.PairCodeWorktree(slug)); err != nil {
				tb.Fatalf("remove warp worktree %s to stage a stale pair: %v", h.PairCodeWorktree(slug), err)
			}
		},
		Arrange: func(tb testing.TB, h *hubforge.Hub) VerbFixture {
			tb.Helper()
			return VerbFixture{
				Slug: slug,
				Target: StateTarget{
					WarpCheckout: h.PrimeWorktree(),
					WeftCheckout: h.PairRecordsSibling(slug),
					// StructuralPath is a weft-named sibling path that Add never created.
					// It is an orphan directory of the kind Prune's orphan pass enumerates.
					// Prune's ownership gate (prune.go:264, ownedRegisteredLinkedWorktree) must refuse to touch it;
					// it is not a registered linked worktree of this hub's weft repo, so the teardown leaves it in place.
					StructuralPath: h.PairRecordsSibling("verb-prune-structural-orphan"),
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
				// applyStalePairProtection (prune.go:206-224) probes scopeTracked dirtiness on the stale pair's weft worktree.
				// A dirty entry is marked Protected, not returned as a Go error.
				// Prune reports per-entry outcomes in its PruneResult, so this cell's Kind is Proceeds (err == nil).
				// What matters is that the protected entry survives untouched.
				return Expectation{
					Kind:           KindProceeds,
					PermittedRoots: slugPermittedRoots(slug),
					Effect: func(tb testing.TB, h *hubforge.Hub, f VerbFixture) {
						tb.Helper()
						assertExists(tb, h.PairRecordsSibling(f.Slug))
						assertExists(tb, h.PairLauncherDir(f.Slug))
					},
				}
			default:
				return Expectation{
					Kind:           KindProceeds,
					PermittedRoots: slugPermittedRoots(slug),
					Effect: func(tb testing.TB, h *hubforge.Hub, f VerbFixture) {
						tb.Helper()
						assertGone(tb, h.PairRecordsSibling(f.Slug))
						assertGone(tb, h.PairLauncherDir(f.Slug))
					},
				}
			}
		},
	}
}

// cleanupCase builds Topology.Cleanup's VerbCase.
// Build produces an orphan managed branch.
// It creates a pair and tears down its warp side (worktree and branch) by hand.
// That leaves the weft branch with no live warp sibling.
// Build also moves the prime pair off the hub's default branch.
// The hub's Bolt checkout stays on the original branch, so Arrange reads the original branch there.
// fabricengine.primaryWeftBranch reads that to find the repo's primary weft line (cleanup.go:206).
// It alone still records the primary after the move.
// Without the move, branch-space liveness alone would still protect the primary weft branch:
// some warp worktree is checked out on it.
// cleanup.go's primaryWeft carve-out and the gate's primaryWeftBranch check would go unexercised;
// both matter only once liveness alone stops protecting the primary line.
//
// Run calls Cleanup with force=true.
// Cleanup's force parameter is reserved and no gate consults it, so apply alone deletes an orphan branch.
// The argument is kept as it was; this cell's clean-state effect does not depend on it.
func cleanupCase() VerbCase {
	const slug = "verb-cleanup-owner"
	return VerbCase{
		Name: "Cleanup",
		Build: func(tb testing.TB, h *hubforge.Hub) {
			tb.Helper()

			hubforge.AddPair(tb, h, slug)
			mustGit(h.PrimeWorktree(), "worktree", "remove", "--force", h.PairCodeWorktree(slug))
			mustGit(h.PrimeWorktree(), "branch", "-D", slug)
			// The weft branch must not be checked out anywhere either: Cleanup never deletes a checked-out branch.
			// The pair's own weft worktree stays on it after only the warp side is torn down, so it is removed too.
			mustGit(h.PrimeRecords(), "worktree", "remove", "--force", h.PairRecordsSibling(slug))

			// Move the prime pair off the default branch.
			// The Bolt checkout, never touched here, keeps recording "original" as the primary warp branch.
			// So the primary weft branch is neither the checked-out weft worktree's own branch nor named by any live pair:
			// the condition under which Cleanup's primaryWeft carve-out is load-bearing.
			mustGit(h.PrimeWorktree(), "checkout", "-b", "verb-cleanup-alt")
			mustGit(h.PrimeRecords(), "checkout", "-b", fabricengine.RecordsBranchName("verb-cleanup-alt"))
		},
		Arrange: func(tb testing.TB, h *hubforge.Hub) VerbFixture {
			tb.Helper()
			return VerbFixture{
				Slug:           slug,
				OriginalBranch: gitkit.CurrentBranch(tb, h.BoardDir()),
				Target: StateTarget{
					WarpCheckout: h.PrimeWorktree(),
					WeftCheckout: h.PairRecordsSibling(slug),
				},
			}
		},
		Run: func(tb testing.TB, h *hubforge.Hub, f VerbFixture) (fabricengine.Mutations, error) {
			tb.Helper()
			res, err := h.Topology.Cleanup(h.Location, true, true, false)
			return res.Mutated(), err
		},
		Expect: func(state string) Expectation {
			return Expectation{
				Kind: KindProceeds,
				Effect: func(tb testing.TB, h *hubforge.Hub, f VerbFixture) {
					tb.Helper()
					orphan := fabricengine.RecordsBranchName(f.Slug)
					if gitkit.BranchExists(tb, h.PrimeRecords(), orphan) {
						tb.Errorf("Cleanup left orphan branch %q behind", orphan)
					}
					// The clean-state effect: orphan managed branches are gone, and the primary weft branch stays intact.
					// Arrange moves the prime pair off f.OriginalBranch,
					// so no live pair or checked-out weft worktree holds the primary weft branch.
					primary := fabricengine.RecordsBranchName(f.OriginalBranch)
					if !gitkit.BranchExists(tb, h.PrimeRecords(), primary) {
						tb.Errorf("Cleanup deleted the primary weft branch %q", primary)
					}
				},
			}
		},
	}
}

// checkoutPermittedRoots returns the one root a proceeding Checkout cell may touch: the hub workspace file.
// A switch renames that file's first folder to the prime's new branch name.
func checkoutPermittedRoots() []string {
	return []string{filepath.ToSlash(fabricengine.HubWorkspacePath(&lymcwd.Location{RepoName: "warp-bare"}))}
}

// checkoutCase builds Topology.Checkout's VerbCase.
// Build adds a pair, and Run passes that pair's own warp branch.
// Without the pair, the cells are vacuous.
// A factory-built hub carries only the clone's own branch, and Checkout on that branch is a no-op.
func checkoutCase() VerbCase {
	const slug = "verb-checkout-owner"
	return VerbCase{
		Name: "Checkout",
		Build: func(tb testing.TB, h *hubforge.Hub) {
			tb.Helper()

			hubforge.AddPair(tb, h, slug)
			// Remove the pair's own worktrees, never its branches, so both branches are free for the prime pair to switch onto.
			// Without this, `git switch` refuses: the branch is still checked out at the pair worktree this Build created.
			mustGit(h.PrimeWorktree(), "worktree", "remove", "--force", h.PairCodeWorktree(slug))
			mustGit(h.PrimeRecords(), "worktree", "remove", "--force", h.PairRecordsSibling(slug))
		},
		Arrange: func(tb testing.TB, h *hubforge.Hub) VerbFixture {
			tb.Helper()
			return VerbFixture{
				Slug:           slug,
				OriginalBranch: gitkit.CurrentBranch(tb, h.PrimeWorktree()),
				CheckoutBranch: slug,
				Target: StateTarget{
					WarpCheckout: h.PrimeWorktree(),
					WeftCheckout: h.PrimeRecords(),
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
				// Checkout's pre-flight checks the weft worktree for tracked modifications (scopeTracked);
				// a tracked modification there refuses before `git switch` runs.
				return Expectation{
					Kind:      KindRefusedBefore,
					Substring: "weft worktree has uncommitted changes",
					Effect: func(tb testing.TB, h *hubforge.Hub, f VerbFixture) {
						tb.Helper()
						if got := gitkit.CurrentBranch(tb, h.PrimeWorktree()); got != f.OriginalBranch {
							tb.Errorf("prime warp branch after refused Checkout = %q; want unchanged %q", got, f.OriginalBranch)
						}
						if got := gitkit.CurrentBranch(tb, h.PrimeRecords()); got != fabricengine.RecordsBranchName(f.OriginalBranch) {
							tb.Errorf("prime weft branch after refused Checkout = %q; want unchanged %q", got, fabricengine.RecordsBranchName(f.OriginalBranch))
						}
					},
				}
			case "dirtyWarpTracked":
				// Checkout has no warp-side dirtiness probe: it goes straight to `git switch`.
				// So git decides the outcome.
				// `git switch` either carries the tracked modification across or refuses: "your local changes would be overwritten".
				// Both are correct; what must hold either way is that the pair is never half-switched.
				return Expectation{
					Kind:      KindEitherProceedsOrRefusedBefore,
					Substring: "warp switch",
					// A switch in the prime renames the workspace file's first folder.
					PermittedRoots: checkoutPermittedRoots(),
					Effect: func(tb testing.TB, h *hubforge.Hub, f VerbFixture) {
						tb.Helper()
						warpBranch := gitkit.CurrentBranch(tb, h.PrimeWorktree())
						weftBranch := gitkit.CurrentBranch(tb, h.PrimeRecords())
						switched := warpBranch == f.CheckoutBranch
						weftSwitched := weftBranch == fabricengine.RecordsBranchName(f.CheckoutBranch)
						if switched != weftSwitched {
							tb.Errorf("Checkout left the pair half-switched: warp branch = %q, weft branch = %q", warpBranch, weftBranch)
						}
						if !switched && warpBranch != f.OriginalBranch {
							tb.Errorf("Checkout neither switched to %q nor left the original branch %q; warp branch = %q", f.CheckoutBranch, f.OriginalBranch, warpBranch)
						}
						// The tracked modification must be on disk either way.
						// Git decides the status shape it reports: still tracked-modified,
						// or carried into a branch that never tracked this path.
						// So the assertion checks the content, not the status.
						assertTrackedContentSurvives(tb, h.PrimeWorktree(), sentinelFileName("dirtyWarpTracked"))
					},
				}
			default:
				return Expectation{
					Kind:           KindProceeds,
					PermittedRoots: checkoutPermittedRoots(),
					Effect: func(tb testing.TB, h *hubforge.Hub, f VerbFixture) {
						tb.Helper()
						if got := gitkit.CurrentBranch(tb, h.PrimeWorktree()); got != f.CheckoutBranch {
							tb.Errorf("prime warp branch after Checkout = %q; want %q", got, f.CheckoutBranch)
						}
						wantWeft := fabricengine.RecordsBranchName(f.CheckoutBranch)
						if got := gitkit.CurrentBranch(tb, h.PrimeRecords()); got != wantWeft {
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
					WeftCheckout: h.PrimeRecords(),
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
// Build adds a pair; clean-state effect: junctions gone, worktree intact.
func unwireJunctionsCase() VerbCase {
	const slug = "verb-unwire-owner"
	return VerbCase{
		Name: "UnwireJunctions",
		Build: func(tb testing.TB, h *hubforge.Hub) {
			tb.Helper()
			hubforge.AddPair(tb, h, slug)
		},
		Arrange: func(tb testing.TB, h *hubforge.Hub) VerbFixture {
			tb.Helper()
			return VerbFixture{
				Slug: slug,
				Target: StateTarget{
					WarpCheckout: h.PairCodeWorktree(slug),
					WeftCheckout: h.PairRecordsSibling(slug),
					// StructuralPath is the pair's own first wired junction link.
					// unseedJunctionRecords walks every wired junction in order and stops at the first one that fails its pre-flight.
					// A foreign link whose resolved target differs from the expected weft target fails it;
					// a dangling link fails it too.
					// That check runs before the ownership gate for that link.
					// Replanting just the first name is therefore enough to make the whole call refuse.
					StructuralPath: firstWiredJunctionLink(tb, h, slug),
				},
			}
		},
		Run: func(tb testing.TB, h *hubforge.Hub, f VerbFixture) (fabricengine.Mutations, error) {
			tb.Helper()
			names, err := fabricengine.WiredNames(configengine.Dirs{Board: h.BoardDir()})
			if err != nil {
				tb.Fatalf("fabricengine.WiredNames(%s): %v", h.BoardDir(), err)
			}
			res, err := fabricengine.UnwireJunctions(h.Location, f.Slug, names)
			return res.Mutated(), err
		},
		Expect: func(state string) Expectation {
			// The wired junction names are structural (lymdirs.LymDirName, lymdirs.DotLymDirName), never operator-editable,
			// so both anchor variants of both names are permitted unconditionally, which is harmless when one goes unused.
			var permitted []string
			for _, anchor := range []string{".", "backend"} {
				loc := &lymcwd.Location{WorktreeName: slug, AnchorRel: anchor}
				for _, name := range []string{lymdirs.LymDirName, lymdirs.DotLymDirName} {
					permitted = append(permitted, filepath.ToSlash(fabricengine.WarpJunctions(loc, slug, []string{name})[0].Link))
				}
			}

			switch state {
			case "trackedSymlinkAtWiredPath":
				// unseedJunctionRecords resolves the link's actual target and compares it against the weft target fabric expects;
				// an operator-owned link resolves elsewhere, so the call refuses before the ownership gate runs.
				// Nothing is removed, not even the untouched second junction.
				return Expectation{
					Kind:      KindRefusedBefore,
					Substring: "points to unexpected target",
				}
			case "staleWiredJunction":
				// The link's target does not resolve at all, so fslink.PointsTo fails outright before the ownership gate runs.
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
						names, err := fabricengine.WiredNames(configengine.Dirs{Board: h.BoardDir()})
						if err != nil {
							tb.Fatalf("fabricengine.WiredNames(%s): %v", h.BoardDir(), err)
						}
						for _, j := range fabricengine.WarpJunctions(h.Location, f.Slug, names) {
							assertGone(tb, j.Link)
						}
						assertExists(tb, h.PairCodeWorktree(f.Slug))
					},
				}
			}
		},
	}
}

// pullCase builds Fabric.Pull's VerbCase (reached via fabricengine.Open).
// Arrange pushes a new commit to the scenario's own warp bare, or the entire column is vacuous:
// Pull returns early when localHEAD equals upstreamSHA, before the dirty check and before any ResetHard.
func pullCase() VerbCase {
	return VerbCase{
		Name: "Pull",
		Arrange: func(tb testing.TB, h *hubforge.Hub) VerbFixture {
			tb.Helper()

			from := gitkit.RevParse(tb, h.PrimeWorktree(), "HEAD")

			scratch := tb.TempDir()
			mustGit(filepath.Dir(scratch), "clone", h.CodeBare, filepath.Base(scratch))
			gitkit.CommitFile(tb, scratch, "pull-advance.txt", "livestate: pull advance\n", "livestate: advance warp bare for Pull")
			mustGit(scratch, "push", "origin", "HEAD:"+gitkit.CurrentBranch(tb, h.PrimeWorktree()))
			to := gitkit.RevParse(tb, scratch, "HEAD")

			return VerbFixture{
				AdvancedFromSHA: from,
				AdvancedToSHA:   to,
				Target: StateTarget{
					WarpCheckout: h.PrimeWorktree(),
					WeftCheckout: h.PrimeRecords(),
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
				// Pull's own pre-flight is scopeTracked on the prime warp worktree;
				// a tracked modification there refuses before ResetHard runs.
				// The warp bare's advance from this cell's own Arrange never lands.
				return Expectation{
					Kind:      KindRefusedBefore,
					Substring: "warp worktree has uncommitted changes",
					Effect: func(tb testing.TB, h *hubforge.Hub, f VerbFixture) {
						tb.Helper()
						// Not f.AdvancedFromSHA: dirtyWarpTracked's plantTrackedDirt seeds a fresh commit, then leaves the change uncommitted.
						// So the prime warp's HEAD has already moved past AdvancedFromSHA when Run executes.
						// A refused Pull must guarantee that it never reached ResetHard:
						// HEAD never advances to the unrelated, unreachable upstream tip.
						if got := gitkit.RevParse(tb, h.PrimeWorktree(), "HEAD"); got == f.AdvancedToSHA {
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
						if got := gitkit.RevParse(tb, h.PrimeWorktree(), "HEAD"); got != f.AdvancedToSHA {
							tb.Errorf("prime warp HEAD after Pull = %s; want %s", got, f.AdvancedToSHA)
						}
					},
				}
			}
		},
	}
}

// cloneHubResetNonHubCase builds the CloneHub{Reset: true} column's non-hub target:
// an operator directory whose name matches the derived <name>-LYMHUB path.
// CloneHub refuses it at its offline checkResetTarget pre-flight, and its contents survive.
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

// cloneHubResetRealHubCase builds the CloneHub{Reset: true} column's positive target:
// a real hub torn down and rebuilt through fabriccli.CloneAndWire.
// It proves the column is not trivially always-refusing.
func cloneHubResetRealHubCase() VerbCase {
	return VerbCase{
		Name:   "CloneHubReset/RealHub",
		States: []string{"clean"},
		Arrange: func(tb testing.TB, h *hubforge.Hub) VerbFixture {
			tb.Helper()
			return VerbFixture{
				ResetCwd:     h.Container,
				ResetWarpURL: filepath.ToSlash(h.CodeBare),
				ResetWeftURL: filepath.ToSlash(h.RecordsBare),
				ResetHubPath: h.Path,
			}
		},
		Run: func(tb testing.TB, h *hubforge.Hub, f VerbFixture) (fabricengine.Mutations, error) {
			tb.Helper()
			// The hub's anchor marker is committed on the prime's weft branch and not pushed,
			// so the re-clone names the anchor again.
			res, err := fabriccli.CloneAndWire(f.ResetCwd, fabricengine.CloneOptions{
				WeftURL: f.ResetWeftURL,
				WarpURL: f.ResetWarpURL,
				Subpath: h.Anchor,
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

// slugHostileInputs returns the slug-shaped hostile inputs that Add and Remove each take.
// Each input is paired with the expectation validateWorktreeSlug produces for it.
// The leading-dash input is the exception: validateWorktreeSlug does not reject it.
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
		{"ReservedHubName", fabricengine.BoardDirName, "that name is reserved for lym hub geometry"},
		{"LeadingDash", "-flag", ""},
	}
}

// addHostileCases returns Add's hostile-input VerbCase entries.
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
				// LeadingDash: validateWorktreeSlug does not reject it.
				// `git worktree add -b -flag <target>` fails, since git's own argument parser reads "-flag" as an unknown option.
				// Nothing is created, which is the safe outcome this cell proves.
				return Expectation{
					Kind:      KindRefusedBefore,
					Substring: "worktree",
					Effect: func(tb testing.TB, h *hubforge.Hub, f VerbFixture) {
						tb.Helper()
						assertGone(tb, h.PairCodeWorktree(f.Slug))
					},
				}
			},
		})
	}
	return cases
}

// removeHostileCases returns Remove's hostile-input VerbCase entries.
// No pair is arranged for any of them.
// Every input is refused before Remove looks at whether a pair by that name exists.
// Arranging one would risk the whole-hub deletion that the ".." case exists to prove is refused.
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
				res, err := h.Topology.Remove(h.Location, f.Slug, false, false)
				return res.Mutated(), err
			},
			Expect: func(state string) Expectation {
				want := in.want
				if want == "" {
					// LeadingDash: validateWorktreeSlug does not reject it.
					// Remove reaches its target-exists stat, finds nothing named "-flag", and refuses safely.
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

// checkoutHostileCases returns Checkout's branch-shaped hostile-input VerbCase entries.
// Checkout validates no branch at all, so the inputs reach `git switch` directly.
// Each fails safely: git itself refuses a nonexistent branch and an option-shaped argument alike.
// Each cell also asserts that the pair is not half-switched.
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
					OriginalBranch: gitkit.CurrentBranch(tb, h.PrimeWorktree()),
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
						if got := gitkit.CurrentBranch(tb, h.PrimeWorktree()); got != f.OriginalBranch {
							tb.Errorf("prime warp branch after refused Checkout = %q; want unchanged %q", got, f.OriginalBranch)
						}
					},
				}
			},
		})
	}
	return cases
}

// unwireJunctionsHostileCase returns UnwireJunctions' hostile-input VerbCase: a junction name
// escaping its worktree ("../x"), the sole route to the link executor's containment check.
//
// The junction name is WorktreePath(l,slug)/AnchorRel/name.
// The number of ".." segments needed to walk out of the hub depends on AnchorRel's depth.
// One ".." cancels "slug" alone at anchor ".";
// at anchor "backend", two are needed, one for "backend" and one for "slug".
// escapeRelName computes that depth, so the link reaches outside the hub at both anchors.
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

// Verbs is the verb table: the ordinary verbs, the CloneHub{Reset: true} targets, and the hostile-input cases.
// The cross-product driver consumes it as the external interface.
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
