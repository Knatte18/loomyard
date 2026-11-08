// livestate_states_test.go defines the hostile states each cross-product cell applies before the verb under test runs.
// A dirtiness state modifies or creates content in a checkout the verb acts on.
// A structural state plants a foreign, stale, or operator-owned entity at the fabric-owned path the verb acts on.
// Each state's Apply resolves against a StateTarget the cell computes for the verb under test.
// It never uses a fixed path of its own choosing:
// a state that dirties or plants at a path the verb never touches asserts nothing about that verb.
//
// Every state's Apply asserts its own establishment inline, via tb.Fatalf.
// States additionally wraps each structural state so an Apply that changes nothing on disk fails too.
// Both checks protect every caller of States, the cross product included.

package fabricengine_test

import (
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/Knatte18/loomyard/internal/fslink"
	"github.com/Knatte18/loomyard/internal/gitkit"
	"github.com/Knatte18/loomyard/internal/hubforge"
)

// StateTarget carries the paths one cell resolves for the verb under test.
// The warp and weft checkouts are what a dirtiness state dirties.
// The fabric-owned path is what a structural state plants at.
// A cell resolves these per verb, never fixed by the state.
// A hub holds several checkouts, and each verb acts on only some of them.
// A state that dirties a checkout the verb never touches asserts nothing.
type StateTarget struct {
	// WarpCheckout is the warp-side git checkout a dirtiness state should dirty.
	WarpCheckout string
	// WeftCheckout is the weft-side git checkout a dirtiness state should dirty.
	WeftCheckout string
	// StructuralPath is the fabric-owned path a structural state plants at:
	// a wired junction path for a state that plants a symlink or junction;
	// another fabric-owned path for a state that plants a directory or clone.
	StructuralPath string
}

// State is one named hostile or clean condition a cell plants before the verb under test runs.
// Apply plants the condition against target and asserts inline that it took.
type State struct {
	// Name identifies the state; it is also embedded into every sentinel file a dirtiness or
	// structural state plants, so a failure message names the state that failed to establish itself.
	Name string
	// Apply plants this state's condition against target, then asserts it took.
	Apply func(tb testing.TB, h *hubforge.Hub, target StateTarget)
}

// States is the ordered hostile-state matrix every cross-product cell draws from.
// The order is stable so a driven failure's cell name is reproducible across runs.
var States = withVacuityCheck([]State{
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
})

// structuralStateNames is the set of states whose Apply must change the hub manifest.
// Dirtiness states and clean are not listed.
// Their inline porcelain assertions already prove establishment, and clean plants nothing to diff.
var structuralStateNames = map[string]bool{
	"trackedSymlinkAtWiredPath":        true,
	"staleWiredJunction":               true,
	"foreignDirAtFabricOwnedPath":      true,
	"unrelatedGitCloneAtWeftNamedPath": true,
}

// withVacuityCheck returns states with each structural state's Apply wrapped.
// The wrapper fails tb when the hub manifest is identical before and after the Apply.
// A structural state that silently no-ops would make every cell using it vacuous while reading like coverage.
// The check runs on the first Apply of each structural state at each anchor, and later Applies skip it:
// a manifest capture hashes every file of the hub, so checking every cell would double the captures a cell takes.
func withVacuityCheck(states []State) []State {
	wrapped := make([]State, len(states))
	for i, state := range states {
		wrapped[i] = state
		if !structuralStateNames[state.Name] {
			continue
		}
		name, apply := state.Name, state.Apply
		var checkedAnchors sync.Map
		wrapped[i].Apply = func(tb testing.TB, h *hubforge.Hub, target StateTarget) {
			tb.Helper()

			if _, checked := checkedAnchors.LoadOrStore(h.Anchor, true); checked {
				apply(tb, h, target)
				return
			}
			before := CaptureManifest(tb, h.Path)
			apply(tb, h, target)
			after := CaptureManifest(tb, h.Path)
			if changes := DiffManifest(before, after, nil); len(changes) == 0 {
				tb.Fatalf("%s at anchor %q: applying the state produced no manifest diff; want at least one change", name, h.Anchor)
			}
		}
	}
	return wrapped
}

// sentinelFileName returns the sentinel file name state stateName plants.
// The name identifies the state,
// so a failure message reads "the operator's uncommitted file is gone" rather than "a path list changed".
// The checkout or directory the file is planted into identifies the cell.
// Each cell resolves its own StateTarget, so the name need not repeat that.
func sentinelFileName(stateName string) string {
	return "livestate-sentinel-" + stateName + ".txt"
}

// trackedDirtMarker is the line plantTrackedDirt appends to a sentinel file's committed content.
// Its presence on disk is what assertTrackedContentSurvives checks, whatever git status the sentinel reports.
// The reported status is " M" when the file stays tracked and modified;
// it is "??" when a branch switch carries the file into a tree that never tracked it.
const trackedDirtMarker = "livestate sentinel dirty"

// plantTrackedDirt commits a fresh sentinel file into checkout, then modifies it.
// This makes `git status --porcelain` report the sentinel with the tracked-modification " M" shape.
func plantTrackedDirt(tb testing.TB, checkout, stateName string) string {
	tb.Helper()

	name := sentinelFileName(stateName)
	path := filepath.Join(checkout, name)
	gitkit.CommitFile(tb, checkout, name, "livestate sentinel seed\n", "livestate: seed "+name)
	if err := os.WriteFile(path, []byte("livestate sentinel seed\n"+trackedDirtMarker+"\n"), 0o644); err != nil {
		tb.Fatalf("plantTrackedDirt(%s): modify %s: %v", stateName, path, err)
	}
	return name
}

// assertTrackedContentSurvives fails tb unless sentinel in checkout still holds its tracked modification.
// It ignores the git status shape, which the verb under test may change.
// A cell whose outcome is proceed or refuse cannot fix the shape in advance:
// the modification may stay tracked-modified, or be carried across as untracked content.
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
	if err := os.WriteFile(path, []byte("livestate sentinel untracked\n"), 0o644); err != nil {
		tb.Fatalf("plantUntrackedDirt(%s): write %s: %v", stateName, path, err)
	}
	return name
}

// assertDirtyContains fails tb unless checkout's `git status --porcelain` output contains a line for
// sentinel carrying exactly wantPrefix (" M" for tracked, "??" for untracked).
// The shape check is load-bearing: it stops a tracked-dirty state satisfying an untracked probe, or the reverse.
// Otherwise the two probes would not tell their states apart.
func assertDirtyContains(tb testing.TB, checkout, sentinel, wantPrefix string) {
	tb.Helper()

	status := gitkit.GitStatusPorcelain(tb, checkout)
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

// assertLinkTarget fails tb unless path is a link whose raw one-hop target, from fslink.RawTarget, is exactly want.
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

// cleanState is the control state: it plants nothing, so it guards against tautological cells.
// A cell run against it proves only that the verb under test succeeds or refuses on a clean hub,
// and every other state's result is measured against that.
func cleanState() State {
	return State{
		Name:  "clean",
		Apply: func(tb testing.TB, h *hubforge.Hub, target StateTarget) {},
	}
}

// dirtyWarpTrackedState modifies a tracked file in the target warp checkout.
// It guards against a destructive verb proceeding on a warp worktree with uncommitted tracked changes:
// the dirtiness probe checked a checkout other than the one the verb acts on.
func dirtyWarpTrackedState() State {
	return State{
		Name: "dirtyWarpTracked",
		Apply: func(tb testing.TB, h *hubforge.Hub, target StateTarget) {
			tb.Helper()

			sentinel := plantTrackedDirt(tb, target.WarpCheckout, "dirtyWarpTracked")
			assertDirtyContains(tb, target.WarpCheckout, sentinel, " M")
		},
	}
}

// dirtyWarpUntrackedState writes a new untracked file into the target warp checkout.
// It guards against a worktree removal that checks only tracked-file dirtiness:
// such a removal destroys an operator's not-yet-staged file.
func dirtyWarpUntrackedState() State {
	return State{
		Name: "dirtyWarpUntracked",
		Apply: func(tb testing.TB, h *hubforge.Hub, target StateTarget) {
			tb.Helper()

			sentinel := plantUntrackedDirt(tb, target.WarpCheckout, "dirtyWarpUntracked")
			assertDirtyContains(tb, target.WarpCheckout, sentinel, "??")
		},
	}
}

// dirtyWeftTrackedState modifies a tracked file in the target weft checkout.
func dirtyWeftTrackedState() State {
	return State{
		Name: "dirtyWeftTracked",
		Apply: func(tb testing.TB, h *hubforge.Hub, target StateTarget) {
			tb.Helper()

			sentinel := plantTrackedDirt(tb, target.WeftCheckout, "dirtyWeftTracked")
			assertDirtyContains(tb, target.WeftCheckout, sentinel, " M")
		},
	}
}

// dirtyWeftUntrackedState writes a new untracked file into the target weft checkout.
// It makes the dirtiness scope observable: a tracked-only probe must proceed past this state,
// and a probe over all files must refuse against the same planted condition.
func dirtyWeftUntrackedState() State {
	return State{
		Name: "dirtyWeftUntracked",
		Apply: func(tb testing.TB, h *hubforge.Hub, target StateTarget) {
			tb.Helper()

			sentinel := plantUntrackedDirt(tb, target.WeftCheckout, "dirtyWeftUntracked")
			assertDirtyContains(tb, target.WeftCheckout, sentinel, "??")
		},
	}
}

// bothDirtyState dirties both checkouts: a tracked modification and an untracked file in each.
// A cell can then prove that a verb's dirtiness handling holds when both checkouts it might probe are dirty at once.
func bothDirtyState() State {
	return State{
		Name: "bothDirty",
		Apply: func(tb testing.TB, h *hubforge.Hub, target StateTarget) {
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

// trackedSymlinkAtWiredPathState replaces a wired junction path with a link the operator owns.
// It guards against fabric adopting or destroying a git-tracked symlink an operator committed at a wired path:
// the ownership check must tell fabric's own wiring apart from a link fabric did not create.
//
// A git-tracked symlink needs core.symlinks=true, and on Windows it also needs Developer Mode.
// Built through fslink.CreateDirLink, the link is a junction on Windows instead,
// so the on-disk artifact differs by platform while the raw target the ownership check compares stays the same.
func trackedSymlinkAtWiredPathState() State {
	return State{
		Name: "trackedSymlinkAtWiredPath",
		Apply: func(tb testing.TB, h *hubforge.Hub, target StateTarget) {
			tb.Helper()

			// The path already carries fabric's own wired junction, so remove it before planting the
			// operator-owned link in its place.
			// This package's directory is excluded from the destructive-bypass guard, so the removal is legal here.
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
			// WireJunctions seeded this exact path into .git/info/exclude.
			// That keeps fabric's own junction from showing as untracked.
			// The "-f" flag is what an operator committing their own link at that path needs, so this state models that shape.
			gitkit.Git(tb, target.WarpCheckout, "add", "-f", rel)
			gitkit.Git(tb, target.WarpCheckout, "commit", "-m", "livestate: track operator-owned link at wired path")

			assertLinkTarget(tb, target.StructuralPath, operatorOwned)
		},
	}
}

// staleWiredJunctionState replaces a wired junction with one whose raw target no longer resolves.
// The junction is still fabric's own wiring, but its target has gone stale.
// A verb's repair or unwire path must handle a dangling target rather than fail obscurely against it.
func staleWiredJunctionState() State {
	return State{
		Name: "staleWiredJunction",
		Apply: func(tb testing.TB, h *hubforge.Hub, target StateTarget) {
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
			// Remove the target after linking to it, so the junction dangles.
			// This mirrors an operator removing the paired worktree a wired junction still points at, by hand,
			// without going through fabric's own teardown.
			// This package's directory is excluded from the destructive-bypass guard, so the removal is legal here.
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

// foreignDirAtFabricOwnedPathState parks an ordinary operator directory at a fabric-owned path.
// The ownership check must tell a directory fabric put there from one an operator placed there.
// A destructive verb must not treat the operator's directory as fabric's own content and destroy it.
func foreignDirAtFabricOwnedPathState() State {
	return State{
		Name: "foreignDirAtFabricOwnedPath",
		Apply: func(tb testing.TB, h *hubforge.Hub, target StateTarget) {
			tb.Helper()

			if err := os.MkdirAll(target.StructuralPath, 0o755); err != nil {
				tb.Fatalf("foreignDirAtFabricOwnedPath: mkdir %s: %v", target.StructuralPath, err)
			}
			contentPath := filepath.Join(target.StructuralPath, sentinelFileName("foreignDirAtFabricOwnedPath"))
			if err := os.WriteFile(contentPath, []byte("livestate: operator content\n"), 0o644); err != nil {
				tb.Fatalf("foreignDirAtFabricOwnedPath: write %s: %v", contentPath, err)
			}

			if _, err := os.Stat(contentPath); err != nil {
				tb.Fatalf("state failed to establish: planted content missing at %s: %v", contentPath, err)
			}
		},
	}
}

// unrelatedGitCloneAtWeftNamedPathState parks an independently git init-ed repository at a weft-named path.
// The repository has a committed file and a clean working tree.
// It is not a plain directory, and its clean tree gives the dirty gate nothing to say about it.
// Only the ownership gate stands between a destructive verb and the whole clone.
func unrelatedGitCloneAtWeftNamedPathState() State {
	return State{
		Name: "unrelatedGitCloneAtWeftNamedPath",
		Apply: func(tb testing.TB, h *hubforge.Hub, target StateTarget) {
			tb.Helper()

			if err := os.MkdirAll(target.StructuralPath, 0o755); err != nil {
				tb.Fatalf("unrelatedGitCloneAtWeftNamedPath: mkdir %s: %v", target.StructuralPath, err)
			}
			gitkit.Git(tb, target.StructuralPath, "init", "-b", "main")
			gitkit.Git(tb, target.StructuralPath, "config", "user.email", "test@test.com")
			gitkit.Git(tb, target.StructuralPath, "config", "user.name", "Test")
			gitkit.CommitFile(tb, target.StructuralPath, sentinelFileName("unrelatedGitCloneAtWeftNamedPath"), "livestate: unrelated repository content\n", "livestate: seed unrelated clone")

			status := gitkit.GitStatusPorcelain(tb, target.StructuralPath)
			if status != "" {
				tb.Fatalf("state failed to establish: unrelated clone at %s is not clean: %s", target.StructuralPath, status)
			}
		},
	}
}
