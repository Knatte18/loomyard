// livestate_states_selftest_test.go proves each hostile state in States establishes what its doc comment claims.
// It applies each state directly to a real hub and asserts on the resulting condition, never by driving a verb,
// so the state's own establishment is not confounded with the verb's behaviour.
// A dirtyWarpTracked state that dirtied nothing would make every cell using it vacuous while reading like coverage,
// so this suite fails loudly against a broken state builder during development.

package fabricengine_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/fslink"
	"github.com/Knatte18/loomyard/internal/gitkit"
	"github.com/Knatte18/loomyard/internal/hubforge"
	"github.com/Knatte18/loomyard/internal/lymdirs"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// structuralStateNames is the set of states this suite also proves against a manifest diff.
// Applying one of them and re-capturing must change the manifest,
// so a structural state that silently no-ops cannot pass.
// Dirtiness states and clean are not listed.
// Their porcelain or cleanliness assertions already prove establishment, and clean plants nothing to diff.
var structuralStateNames = map[string]bool{
	"trackedSymlinkAtWiredPath":        true,
	"staleWiredJunction":               true,
	"foreignDirAtFabricOwnedPath":      true,
	"unrelatedGitCloneAtWeftNamedPath": true,
}

// resolveStateTarget builds the StateTarget that a cell resolves for stateName against h.
// Every dirtiness state resolves to the prime warp and weft checkouts.
// A structural state resolves to a fabric-owned path.
// The two link states resolve to an existing wired junction link.
// The two foreign-entity states resolve to a fresh, not-yet-claimed pair path.
// The foreign-entity state unrelatedGitCloneAtWeftNamedPath gets a weft-named sibling path.
func resolveStateTarget(t *testing.T, h *hubforge.Hub, stateName string) StateTarget {
	t.Helper()

	target := StateTarget{
		WarpCheckout: h.PrimeWorktree(),
		WeftCheckout: h.PrimeRecords(),
	}

	switch stateName {
	case "trackedSymlinkAtWiredPath", "staleWiredJunction":
		linkAbs, _ := firstWiredJunction(t, h)
		target.StructuralPath = linkAbs
	case "foreignDirAtFabricOwnedPath":
		target.StructuralPath = h.PairCodeWorktree("states-foreign-owner")
	case "unrelatedGitCloneAtWeftNamedPath":
		target.StructuralPath = h.PairRecordsSibling("states-clone-owner")
	}

	return target
}

// newPorcelainLines returns the porcelain lines present in afterStatus but absent from beforeStatus.
// Comparing against that baseline keeps a dirtiness assertion immune to noise a checkout already carried.
// A freshly cloned prime weft sibling always carries one such line.
// A raw scan of the full output would wrongly blame that line on the state under test.
func newPorcelainLines(beforeStatus, afterStatus string) []string {
	before := make(map[string]bool)
	for _, line := range strings.Split(beforeStatus, "\n") {
		if line != "" {
			before[line] = true
		}
	}

	var added []string
	for _, line := range strings.Split(afterStatus, "\n") {
		if line == "" || before[line] {
			continue
		}
		added = append(added, line)
	}
	return added
}

// assertOnlyNewShape fails t unless checkout carries at least one porcelain line new since beforeStatus,
// and every new line carries wantPrefix.
// The prefix check also rejects the other dirtiness shape, so tracked and untracked changes stay distinct.
func assertOnlyNewShape(t *testing.T, checkout, beforeStatus, wantPrefix string) {
	t.Helper()

	newLines := newPorcelainLines(beforeStatus, gitkit.GitStatusPorcelain(t, checkout))
	if len(newLines) == 0 {
		t.Fatalf("state failed to establish: no new dirt appeared in %s", checkout)
	}
	for _, line := range newLines {
		if !strings.HasPrefix(line, wantPrefix) {
			t.Fatalf("state established the wrong dirtiness shape in %s: new line %q; want prefix %q", checkout, line, wantPrefix)
		}
	}
}

// assertBothNewShapes fails t unless checkout gains a tracked-modification line and an untracked line.
// Requiring both shapes rules out a state that establishes only one of them.
func assertBothNewShapes(t *testing.T, checkout, beforeStatus string) {
	t.Helper()

	newLines := newPorcelainLines(beforeStatus, gitkit.GitStatusPorcelain(t, checkout))

	var hasTracked, hasUntracked bool
	for _, line := range newLines {
		switch {
		case strings.HasPrefix(line, " M"):
			hasTracked = true
		case strings.HasPrefix(line, "??"):
			hasUntracked = true
		default:
			t.Fatalf("unexpected new git status --porcelain line in %s: %q", checkout, line)
		}
	}
	if !hasTracked || !hasUntracked {
		t.Fatalf("bothDirty failed to establish both shapes in %s: tracked=%v untracked=%v (new lines: %v)", checkout, hasTracked, hasUntracked, newLines)
	}
}

// assertCleanHubEstablished fails t unless every checkout in h is clean of operator content.
//
// The warp checkout and the board checkout must carry no git status --porcelain output.
// The prime weft sibling may carry exactly one line, for the still-empty wiring directory WireJunctions creates.
// Git never tracks an empty directory, so the line shows as untracked:
// "?? _lym/" at anchor "." or "?? backend/" at anchor "backend".
// This is fabric's own machine-local wiring scratch, not operator dirt.
// Any other line is a genuine finding, not baseline noise.
func assertCleanHubEstablished(t *testing.T, h *hubforge.Hub) {
	t.Helper()

	for _, checkout := range []string{h.PrimeWorktree(), h.BoardDir()} {
		if status := gitkit.GitStatusPorcelain(t, checkout); status != "" {
			t.Errorf("clean state failed to establish: %s is not clean: %s", checkout, status)
		}
	}

	weftStatus := gitkit.GitStatusPorcelain(t, h.PrimeRecords())
	if weftStatus == "" {
		return
	}

	allowed := "?? " + h.Anchor + "/"
	if h.Anchor == "." {
		allowed = "?? " + lymdirs.LymDirName + "/"
	}
	for _, line := range strings.Split(weftStatus, "\n") {
		if line == "" || line == allowed {
			continue
		}
		t.Errorf("clean state failed to establish: %s carries unexpected dirt: %q (full status: %s)", h.PrimeRecords(), line, weftStatus)
	}
}

// gitPathIsTracked reports whether relPath is tracked by git in dir.
// It asks git through `git ls-files --error-unmatch`.
func gitPathIsTracked(t *testing.T, dir, relPath string) bool {
	t.Helper()

	cmd := exec.Command("git", "ls-files", "--error-unmatch", relPath)
	cmd.Dir = dir
	return cmd.Run() == nil
}

// assertTrackedSymlinkEstablished fails t unless target.StructuralPath is a link.
// Its raw one-hop target must exist on disk, and the link must be git-tracked in target.WarpCheckout.
func assertTrackedSymlinkEstablished(t *testing.T, target StateTarget) {
	t.Helper()

	isLink, err := fslink.IsLink(target.StructuralPath)
	if err != nil {
		t.Fatalf("fslink.IsLink(%s): %v", target.StructuralPath, err)
	}
	if !isLink {
		t.Fatalf("state failed to establish: %s is not a link", target.StructuralPath)
	}

	raw, err := fslink.RawTarget(target.StructuralPath)
	if err != nil {
		t.Fatalf("fslink.RawTarget(%s): %v", target.StructuralPath, err)
	}
	if raw == "" {
		t.Fatalf("state failed to establish: %s carries an empty raw target", target.StructuralPath)
	}
	if _, err := os.Stat(raw); err != nil {
		t.Errorf("state failed to establish: planted raw target %s does not exist: %v", raw, err)
	}

	rel, err := filepath.Rel(target.WarpCheckout, target.StructuralPath)
	if err != nil {
		t.Fatalf("filepath.Rel(%s, %s): %v", target.WarpCheckout, target.StructuralPath, err)
	}
	if !gitPathIsTracked(t, target.WarpCheckout, rel) {
		t.Errorf("state failed to establish: %s is not git-tracked in %s", rel, target.WarpCheckout)
	}
}

// assertStaleJunctionEstablished fails t unless target.StructuralPath is a link.
// Its raw one-hop target must not resolve on disk.
func assertStaleJunctionEstablished(t *testing.T, target StateTarget) {
	t.Helper()

	isLink, err := fslink.IsLink(target.StructuralPath)
	if err != nil {
		t.Fatalf("fslink.IsLink(%s): %v", target.StructuralPath, err)
	}
	if !isLink {
		t.Fatalf("state failed to establish: %s is not a link", target.StructuralPath)
	}

	raw, err := fslink.RawTarget(target.StructuralPath)
	if err != nil {
		t.Fatalf("fslink.RawTarget(%s): %v", target.StructuralPath, err)
	}
	if _, statErr := os.Stat(raw); !os.IsNotExist(statErr) {
		t.Errorf("state failed to establish: raw target %s unexpectedly resolves (stat err: %v)", raw, statErr)
	}
}

// assertForeignDirEstablished fails t unless target.StructuralPath is an ordinary directory with its planted content.
// The directory must not itself be a git checkout.
func assertForeignDirEstablished(t *testing.T, target StateTarget) {
	t.Helper()

	info, err := os.Stat(target.StructuralPath)
	if err != nil {
		t.Fatalf("state failed to establish: %s missing: %v", target.StructuralPath, err)
	}
	if !info.IsDir() {
		t.Fatalf("state failed to establish: %s is not a directory", target.StructuralPath)
	}

	contentPath := filepath.Join(target.StructuralPath, sentinelFileName("foreignDirAtFabricOwnedPath"))
	if _, err := os.Stat(contentPath); err != nil {
		t.Errorf("state failed to establish: planted content missing at %s: %v", contentPath, err)
	}

	if _, err := os.Stat(filepath.Join(target.StructuralPath, ".git")); !os.IsNotExist(err) {
		t.Errorf("state failed to establish: %s unexpectedly looks like a git checkout", target.StructuralPath)
	}
}

// assertUnrelatedCloneEstablished fails t unless target.StructuralPath is a clean git repository with a commit.
// The clean tree keeps the dirty gate silent, so only the ownership gate governs the clone.
func assertUnrelatedCloneEstablished(t *testing.T, target StateTarget) {
	t.Helper()

	if _, err := os.Stat(filepath.Join(target.StructuralPath, ".git")); err != nil {
		t.Fatalf("state failed to establish: %s does not look like a git checkout: %v", target.StructuralPath, err)
	}

	cmd := exec.Command("git", "rev-parse", "HEAD")
	cmd.Dir = target.StructuralPath
	if err := cmd.Run(); err != nil {
		t.Errorf("state failed to establish: %s has no commits: %v", target.StructuralPath, err)
	}

	if status := gitkit.GitStatusPorcelain(t, target.StructuralPath); status != "" {
		t.Errorf("state failed to establish: %s is not clean: %s", target.StructuralPath, status)
	}
}

// assertStateEstablished dispatches to the assertion matching stateName, proving the condition that state claims.
// beforeWarp and beforeWeft hold the git status --porcelain output of the warp and weft checkouts.
// They are captured before state.Apply ran.
// Comparing against them isolates what the state itself changed from any baseline a checkout already carried.
func assertStateEstablished(t *testing.T, h *hubforge.Hub, stateName string, target StateTarget, beforeWarp, beforeWeft string) {
	t.Helper()

	switch stateName {
	case "clean":
		assertCleanHubEstablished(t, h)
	case "dirtyWarpTracked":
		assertOnlyNewShape(t, target.WarpCheckout, beforeWarp, " M")
	case "dirtyWarpUntracked":
		assertOnlyNewShape(t, target.WarpCheckout, beforeWarp, "??")
	case "dirtyWeftTracked":
		assertOnlyNewShape(t, target.WeftCheckout, beforeWeft, " M")
	case "dirtyWeftUntracked":
		assertOnlyNewShape(t, target.WeftCheckout, beforeWeft, "??")
	case "bothDirty":
		assertBothNewShapes(t, target.WarpCheckout, beforeWarp)
		assertBothNewShapes(t, target.WeftCheckout, beforeWeft)
	case "trackedSymlinkAtWiredPath":
		assertTrackedSymlinkEstablished(t, target)
	case "staleWiredJunction":
		assertStaleJunctionEstablished(t, target)
	case "foreignDirAtFabricOwnedPath":
		assertForeignDirEstablished(t, target)
	case "unrelatedGitCloneAtWeftNamedPath":
		assertUnrelatedCloneEstablished(t, target)
	default:
		t.Fatalf("states_test.go: no establishment assertion registered for state %q — add one alongside its States entry", stateName)
	}
}

// TestStates proves every state in States establishes what it claims, at both anchors.
// Each anchor runs on its own freshly built hub.
func TestStates(t *testing.T) {
	tiergate.Require(t, 1)
	for _, state := range States {
		state := state

		t.Run(state.Name, func(t *testing.T) {
			for _, anchor := range []string{".", "backend"} {
				anchor := anchor

				t.Run(anchor, func(t *testing.T) {
					t.Parallel()

					h := hubforge.CopyHub(t, hubforge.Shape{Anchor: anchor})
					target := resolveStateTarget(t, h, state.Name)

					beforeWarp := gitkit.GitStatusPorcelain(t, target.WarpCheckout)
					beforeWeft := gitkit.GitStatusPorcelain(t, target.WeftCheckout)

					var before Manifest
					if structuralStateNames[state.Name] {
						before = CaptureManifest(t, h.Path)
					}

					state.Apply(t, h, target)

					assertStateEstablished(t, h, state.Name, target, beforeWarp, beforeWeft)

					// A structural state that silently no-ops must not pass this suite by accident:
					// the manifest diff is the independent proof that applying the state changed something on disk.
					// It is distinct from the state-specific assertion above.
					if structuralStateNames[state.Name] {
						after := CaptureManifest(t, h.Path)
						if changes := DiffManifest(before, after, nil); len(changes) == 0 {
							t.Errorf("%s: applying the state produced no manifest diff; want at least one change", state.Name)
						}
					}
				})
			}
		})
	}
}
