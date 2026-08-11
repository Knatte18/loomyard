//go:build integration

// states.go defines the ten named hostile states every cell in the cross-product matrix batches 6-7
// build can be run against: a dirtiness state modifies or creates content in one of the checkouts a
// verb under test acts on, and a structural state plants a foreign, stale, or operator-owned entity at
// the fabric-owned path a verb under test acts on.
// Each state's Apply resolves against a StateTarget the cell computed for the verb under test — never a
// fixed path of the state's own choosing — because a state that dirties or plants at a path the verb
// never touches asserts nothing about that verb.
//
// Every state's Apply asserts its own establishment inline, via tb.Fatalf, in addition to the
// self-assertion suite states_test.go carries.
// The inline check is what protects a cell running months from now;
// the suite is what protects the state builders during development.
// Neither substitutes for the other.

package fabrictest

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/fslink"
)

// StateTarget carries the paths one cell resolved for the verb under test: the warp checkout a
// dirtiness state should dirty, the weft checkout a dirtiness state should dirty, and the fabric-owned
// path — a wired junction path, or another fabric-owned path — a structural state should plant at.
// A cell resolves these per verb, never fixed by the state, which is what makes the dirty-what-per-cell
// rule expressible: a tranche-1 hub has at least four checkouts, and the gate probes a different one
// per verb, so a state that dirties a checkout the verb never touches asserts nothing.
type StateTarget struct {
	// WarpCheckout is the warp-side git checkout a dirtiness state should dirty.
	WarpCheckout string
	// WeftCheckout is the weft-side git checkout a dirtiness state should dirty.
	WeftCheckout string
	// StructuralPath is the fabric-owned path a structural state should plant at — a wired junction
	// path for trackedSymlinkAtWiredPath and staleWiredJunction, or another fabric-owned path for
	// foreignDirAtFabricOwnedPath and unrelatedGitCloneAtWeftNamedPath.
	StructuralPath string
}

// State is one named hostile or clean condition a cell can plant before the verb under test runs.
// Apply plants the condition against target and asserts inline that the condition actually took — see
// this file's doc comment for why both the inline assertion and states_test.go's suite exist.
type State struct {
	// Name identifies the state; it is also embedded into every sentinel file a dirtiness or
	// structural state plants, so a failure message names the state that failed to establish itself.
	Name string
	// Apply plants this state's condition against target, then asserts it took.
	Apply func(tb testing.TB, h *Hub, target StateTarget)
}

// States is the ordered ten-member hostile-state matrix every cross-product cell draws from.
// The order is stable so a driven failure's cell name is reproducible across runs.
var States = []State{
	cleanState(),
	dirtyWarpTrackedState(),
	dirtyWarpUntrackedState(),
	dirtyWeftTrackedState(),
	dirtyWeftUntrackedState(),
	bothDirtyState(),
	trackedSymlinkAtWiredPathState(),
	foreignDirAtFabricOwnedPathState(),
	unrelatedGitCloneAtWeftNamedPathState(),
	staleWiredJunctionState(),
}

// sentinelFileName returns the sentinel file name state stateName plants: a name that identifies the
// state, so a failure message reads "the operator's uncommitted file is gone" rather than "a path list
// changed".
// The checkout or directory it is planted into is what identifies the cell — each cell resolves its own
// StateTarget, so the file name itself need not repeat that.
func sentinelFileName(stateName string) string {
	return "fabrictest-sentinel-" + stateName + ".txt"
}

// trackedDirtMarker is the line plantTrackedDirt appends to a sentinel file's committed content to
// produce its own local modification. Its presence on disk is what assertTrackedContentSurvives checks
// for, independent of whatever git status shape (" M" tracked-modified, or "??" if a branch switch
// carried it into a tree that never tracked it) the sentinel currently reports.
const trackedDirtMarker = "fabrictest sentinel dirty"

// plantTrackedDirt commits a fresh sentinel file into checkout, then modifies it, so `git status
// --porcelain` reports the sentinel with the tracked-modification " M" shape.
func plantTrackedDirt(tb testing.TB, checkout, stateName string) string {
	tb.Helper()

	name := sentinelFileName(stateName)
	path := filepath.Join(checkout, name)
	if err := os.WriteFile(path, []byte("fabrictest sentinel seed\n"), 0o644); err != nil {
		tb.Fatalf("plantTrackedDirt(%s): write %s: %v", stateName, path, err)
	}
	mustGit(checkout, "add", name)
	mustGit(checkout, "commit", "-m", "fabrictest: seed "+name)
	if err := os.WriteFile(path, []byte("fabrictest sentinel seed\n"+trackedDirtMarker+"\n"), 0o644); err != nil {
		tb.Fatalf("plantTrackedDirt(%s): modify %s: %v", stateName, path, err)
	}
	return name
}

// assertTrackedContentSurvives fails tb unless checkout still carries sentinel on disk with
// plantTrackedDirt's own local modification intact, regardless of whatever git status shape the
// sentinel currently reports.
// This is what makes the check usable for a cell whose outcome is declared
// KindEitherProceedsOrRefusedBefore -- e.g. checkoutCase's dirtyWarpTracked cell, where `git switch`
// itself decides whether the modification stays tracked-modified at the original branch or is carried
// across as untracked content in a tree that never had this path -- rather than only for a cell with a
// single git-status shape it can assert via assertDirtyContains.
func assertTrackedContentSurvives(tb testing.TB, checkout, sentinel string) {
	tb.Helper()

	path := filepath.Join(checkout, sentinel)
	data, err := os.ReadFile(path)
	if err != nil {
		tb.Fatalf("tracked modification lost: read %s: %v", path, err)
	}
	if !strings.Contains(string(data), trackedDirtMarker) {
		tb.Fatalf("tracked modification lost: %s no longer contains %q; got %q", path, trackedDirtMarker, string(data))
	}
}

// plantUntrackedDirt writes a fresh sentinel file into checkout without ever staging or committing it,
// so `git status --porcelain` reports the sentinel with the untracked "??" shape.
func plantUntrackedDirt(tb testing.TB, checkout, stateName string) string {
	tb.Helper()

	name := sentinelFileName(stateName)
	path := filepath.Join(checkout, name)
	if err := os.WriteFile(path, []byte("fabrictest sentinel untracked\n"), 0o644); err != nil {
		tb.Fatalf("plantUntrackedDirt(%s): write %s: %v", stateName, path, err)
	}
	return name
}

// assertDirtyContains fails tb unless checkout's `git status --porcelain` output contains a line for
// sentinel carrying exactly wantPrefix (" M" for tracked, "??" for untracked).
// The shape check is load-bearing, not decorative: a tracked-dirty state that silently also satisfied
// an untracked probe (or vice versa) would collapse the Proceeds derivation dirtyWeftUntracked exists
// to make observable.
func assertDirtyContains(tb testing.TB, checkout, sentinel, wantPrefix string) {
	tb.Helper()

	status := GitStatusPorcelain(tb, checkout)
	if status == "" {
		tb.Fatalf("state failed to establish: %s is clean; want dirty with sentinel %s", checkout, sentinel)
	}

	for _, line := range strings.Split(status, "\n") {
		if !strings.Contains(line, sentinel) {
			continue
		}
		if !strings.HasPrefix(line, wantPrefix) {
			tb.Fatalf("state established sentinel %s with the wrong dirtiness shape: line %q; want prefix %q", sentinel, line, wantPrefix)
		}
		return
	}
	tb.Fatalf("sentinel %s not present in git status --porcelain output: %s", sentinel, status)
}

// assertLinkTarget fails tb unless path is a link whose raw one-hop target, from fslink.RawTarget, is
// exactly want.
func assertLinkTarget(tb testing.TB, path, want string) {
	tb.Helper()

	isLink, err := fslink.IsLink(path)
	if err != nil {
		tb.Fatalf("fslink.IsLink(%s): %v", path, err)
	}
	if !isLink {
		tb.Fatalf("state failed to establish: %s is not a link", path)
	}
	got, err := fslink.RawTarget(path)
	if err != nil {
		tb.Fatalf("fslink.RawTarget(%s): %v", path, err)
	}
	if got != want {
		tb.Fatalf("state failed to establish: %s raw target = %q; want %q", path, got, want)
	}
}

// cleanState is the no-op tautology-guard baseline: it plants nothing, so a cell run against it proves
// only that the verb under test succeeds (or refuses) against a genuinely clean hub — the control every
// other state's result is measured against.
func cleanState() State {
	return State{
		Name:  "clean",
		Apply: func(tb testing.TB, h *Hub, target StateTarget) {},
	}
}

// dirtyWarpTrackedState modifies a tracked file in the target warp checkout — born from R2, where a
// destructive verb proceeded against a warp worktree carrying uncommitted tracked changes because its
// dirtiness probe never ran against the checkout the verb actually acted on.
func dirtyWarpTrackedState() State {
	return State{
		Name: "dirtyWarpTracked",
		Apply: func(tb testing.TB, h *Hub, target StateTarget) {
			tb.Helper()

			sentinel := plantTrackedDirt(tb, target.WarpCheckout, "dirtyWarpTracked")
			assertDirtyContains(tb, target.WarpCheckout, sentinel, " M")
		},
	}
}

// dirtyWarpUntrackedState writes a new untracked file into the target warp checkout — born from R3,
// where an operator's not-yet-staged file was destroyed alongside a worktree removal that only checked
// tracked-file dirtiness.
func dirtyWarpUntrackedState() State {
	return State{
		Name: "dirtyWarpUntracked",
		Apply: func(tb testing.TB, h *Hub, target StateTarget) {
			tb.Helper()

			sentinel := plantUntrackedDirt(tb, target.WarpCheckout, "dirtyWarpUntracked")
			assertDirtyContains(tb, target.WarpCheckout, sentinel, "??")
		},
	}
}

// dirtyWeftTrackedState modifies a tracked file in the target weft checkout, the weft-side counterpart
// to dirtyWarpTracked.
func dirtyWeftTrackedState() State {
	return State{
		Name: "dirtyWeftTracked",
		Apply: func(tb testing.TB, h *Hub, target StateTarget) {
			tb.Helper()

			sentinel := plantTrackedDirt(tb, target.WeftCheckout, "dirtyWeftTracked")
			assertDirtyContains(tb, target.WeftCheckout, sentinel, " M")
		},
	}
}

// dirtyWeftUntrackedState writes a new untracked file into the target weft checkout.
// This is the state that makes the dirtiness-scope parameter observable at all: Checkout's dirtiness
// probe at checkout.go:42 is worktreeDirty(scopeTracked, …), which must proceed past this state, while
// Remove's refuseDirtyWeftWorktree at remove.go:79/:144 probes worktreeDirty(scopeAll, …), which must
// refuse against the identical planted condition.
func dirtyWeftUntrackedState() State {
	return State{
		Name: "dirtyWeftUntracked",
		Apply: func(tb testing.TB, h *Hub, target StateTarget) {
			tb.Helper()

			sentinel := plantUntrackedDirt(tb, target.WeftCheckout, "dirtyWeftUntracked")
			assertDirtyContains(tb, target.WeftCheckout, sentinel, "??")
		},
	}
}

// bothDirtyState applies warp and weft dirtiness together — a tracked modification and an untracked
// file on each side — so a cell can prove a verb's dirtiness handling holds even when both checkouts it
// might probe are simultaneously dirty, not just whichever one a single-sided state happens to hit.
func bothDirtyState() State {
	return State{
		Name: "bothDirty",
		Apply: func(tb testing.TB, h *Hub, target StateTarget) {
			tb.Helper()

			warpTracked := plantTrackedDirt(tb, target.WarpCheckout, "bothDirty-warp-tracked")
			warpUntracked := plantUntrackedDirt(tb, target.WarpCheckout, "bothDirty-warp-untracked")
			weftTracked := plantTrackedDirt(tb, target.WeftCheckout, "bothDirty-weft-tracked")
			weftUntracked := plantUntrackedDirt(tb, target.WeftCheckout, "bothDirty-weft-untracked")

			assertDirtyContains(tb, target.WarpCheckout, warpTracked, " M")
			assertDirtyContains(tb, target.WarpCheckout, warpUntracked, "??")
			assertDirtyContains(tb, target.WeftCheckout, weftTracked, " M")
			assertDirtyContains(tb, target.WeftCheckout, weftUntracked, "??")
		},
	}
}

// trackedSymlinkAtWiredPathState replaces a wired junction path with a link the operator owns — born
// from R1, where fabric silently adopted or destroyed a git-tracked symlink an operator had committed
// at a path fabric itself wires, because the gate's ownership check never distinguished "fabric's own
// wiring" from "a link fabric did not create".
//
// A git-tracked symlink needs core.symlinks=true plus Developer Mode on Windows; building it through
// fslink.CreateDirLink materialises it as a junction there instead, and the gate's ownedWiredJunction
// check compares fslink.RawTarget, so the assertion keeps its shape even though the on-disk artifact
// differs by platform.
// See doc.go's "Windows gap" section for the fuller account of this divergence.
func trackedSymlinkAtWiredPathState() State {
	return State{
		Name: "trackedSymlinkAtWiredPath",
		Apply: func(tb testing.TB, h *Hub, target StateTarget) {
			tb.Helper()

			// The path already carries fabric's own wired junction; remove it before planting the
			// operator-owned link in its place. Legal here per this package's directory exclusion from
			// the destructive-bypass guard (see this file's own doc comment).
			if err := fslink.Remove(target.StructuralPath); err != nil {
				tb.Fatalf("trackedSymlinkAtWiredPath: remove existing entry at %s: %v", target.StructuralPath, err)
			}

			operatorOwned := filepath.Join(tb.TempDir(), sentinelFileName("trackedSymlinkAtWiredPath"))
			if err := os.MkdirAll(operatorOwned, 0o755); err != nil {
				tb.Fatalf("trackedSymlinkAtWiredPath: mkdir operator-owned target %s: %v", operatorOwned, err)
			}
			if err := fslink.CreateDirLink(target.StructuralPath, operatorOwned); err != nil {
				tb.Fatalf("trackedSymlinkAtWiredPath: create link %s -> %s: %v", target.StructuralPath, operatorOwned, err)
			}

			rel, err := filepath.Rel(target.WarpCheckout, target.StructuralPath)
			if err != nil {
				tb.Fatalf("trackedSymlinkAtWiredPath: relativize %s against %s: %v", target.StructuralPath, target.WarpCheckout, err)
			}
			// WireJunctions seeded this exact path into .git/info/exclude so fabric's OWN junction
			// never shows as untracked; "-f" is what an operator committing a link of their own at that
			// same path would need too, and is exactly the shape this state models.
			mustGit(target.WarpCheckout, "add", "-f", rel)
			mustGit(target.WarpCheckout, "commit", "-m", "fabrictest: track operator-owned link at wired path")

			assertLinkTarget(tb, target.StructuralPath, operatorOwned)
		},
	}
}

// staleWiredJunctionState replaces a wired junction with one whose raw target no longer resolves — R1's
// repair-side counterpart: the junction genuinely is fabric's own wiring, but its target has gone stale
// (the paired worktree it pointed at was removed by hand, mirrored here by materialising the target and
// then removing it after linking), and a verb's repair or unwire path must handle a dangling target
// rather than fail obscurely against it.
func staleWiredJunctionState() State {
	return State{
		Name: "staleWiredJunction",
		Apply: func(tb testing.TB, h *Hub, target StateTarget) {
			tb.Helper()

			if err := fslink.Remove(target.StructuralPath); err != nil {
				tb.Fatalf("staleWiredJunction: remove existing entry at %s: %v", target.StructuralPath, err)
			}

			staleTarget := filepath.Join(tb.TempDir(), sentinelFileName("staleWiredJunction"))
			if err := os.MkdirAll(staleTarget, 0o755); err != nil {
				tb.Fatalf("staleWiredJunction: mkdir %s: %v", staleTarget, err)
			}
			if err := fslink.CreateDirLink(target.StructuralPath, staleTarget); err != nil {
				tb.Fatalf("staleWiredJunction: create link %s -> %s: %v", target.StructuralPath, staleTarget, err)
			}
			// Remove the target after linking to it, mirroring the real defect this state is named
			// for: an operator removing the paired worktree a wired junction still points at, by hand,
			// without going through fabric's own teardown. Legal here per this package's directory
			// exclusion from the destructive-bypass guard.
			if err := os.Remove(staleTarget); err != nil {
				tb.Fatalf("staleWiredJunction: remove planted target %s to make it stale: %v", staleTarget, err)
			}

			isLink, err := fslink.IsLink(target.StructuralPath)
			if err != nil {
				tb.Fatalf("fslink.IsLink(%s): %v", target.StructuralPath, err)
			}
			if !isLink {
				tb.Fatalf("state failed to establish: %s is not a link", target.StructuralPath)
			}
			if _, err := os.Stat(staleTarget); !os.IsNotExist(err) {
				tb.Fatalf("state failed to establish: stale target %s unexpectedly resolves (stat err %v)", staleTarget, err)
			}
		},
	}
}

// foreignDirAtFabricOwnedPathState parks an ordinary operator directory at a fabric-owned path — born
// from R4, where fabric treated a plain directory an operator had placed at a path fabric itself owns
// as though it were fabric's own content, and destroyed it without the ownership check ever
// distinguishing "fabric put this here" from "an operator's directory happens to sit here".
func foreignDirAtFabricOwnedPathState() State {
	return State{
		Name: "foreignDirAtFabricOwnedPath",
		Apply: func(tb testing.TB, h *Hub, target StateTarget) {
			tb.Helper()

			if err := os.MkdirAll(target.StructuralPath, 0o755); err != nil {
				tb.Fatalf("foreignDirAtFabricOwnedPath: mkdir %s: %v", target.StructuralPath, err)
			}
			contentPath := filepath.Join(target.StructuralPath, sentinelFileName("foreignDirAtFabricOwnedPath"))
			if err := os.WriteFile(contentPath, []byte("fabrictest: operator content\n"), 0o644); err != nil {
				tb.Fatalf("foreignDirAtFabricOwnedPath: write %s: %v", contentPath, err)
			}

			if _, err := os.Stat(contentPath); err != nil {
				tb.Fatalf("state failed to establish: planted content missing at %s: %v", contentPath, err)
			}
		},
	}
}

// unrelatedGitCloneAtWeftNamedPathState parks a complete, independently git init-ed repository — a
// committed file and a clean working tree — at a weft-named path: R4's second shape, where the foreign
// entity is not a plain directory but a genuine repository the dirty gate has nothing to say about (its
// tree is clean), so only the ownership gate stands between a destructive verb and the whole clone.
func unrelatedGitCloneAtWeftNamedPathState() State {
	return State{
		Name: "unrelatedGitCloneAtWeftNamedPath",
		Apply: func(tb testing.TB, h *Hub, target StateTarget) {
			tb.Helper()

			if err := os.MkdirAll(target.StructuralPath, 0o755); err != nil {
				tb.Fatalf("unrelatedGitCloneAtWeftNamedPath: mkdir %s: %v", target.StructuralPath, err)
			}
			mustGit(target.StructuralPath, "init", "-b", "main")
			mustGit(target.StructuralPath, "config", "user.email", "test@test.com")
			mustGit(target.StructuralPath, "config", "user.name", "Test")

			contentPath := filepath.Join(target.StructuralPath, sentinelFileName("unrelatedGitCloneAtWeftNamedPath"))
			if err := os.WriteFile(contentPath, []byte("fabrictest: unrelated repository content\n"), 0o644); err != nil {
				tb.Fatalf("unrelatedGitCloneAtWeftNamedPath: write %s: %v", contentPath, err)
			}
			mustGit(target.StructuralPath, "add", ".")
			mustGit(target.StructuralPath, "commit", "-m", "fabrictest: seed unrelated clone")

			status := GitStatusPorcelain(tb, target.StructuralPath)
			if status != "" {
				tb.Fatalf("state failed to establish: unrelated clone at %s is not clean: %s", target.StructuralPath, status)
			}
		},
	}
}
