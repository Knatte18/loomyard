//go:build integration

// states_test.go proves every hostile state in States establishes exactly what its own doc comment
// claims, by applying each state directly against a real hub and asserting on the resulting condition —
// never by driving a verb, which would confound the state's own establishment with the verb's
// behaviour.
// This is the suite the batch's own doc comment calls the single most important guard in the harness: a
// dirtyWarpTracked state that silently failed to dirty anything would make every cell using it vacuous
// while reading like coverage, and this file is what a broken state builder fails loudly against during
// development rather than months later inside an opaque cross-product failure.

package fabrictest

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/fslink"
	"github.com/Knatte18/loomyard/internal/lyxdirs"
)

// structuralStateNames is the set of states this suite also proves against a manifest diff: applying
// one of these and re-capturing must produce at least one change against the pre-state manifest, so a
// structural state that silently no-ops cannot pass this suite by accident.
// Dirtiness states and clean are excluded: their own porcelain/cleanliness assertions already prove
// establishment directly, and clean plants nothing to diff.
var structuralStateNames = map[string]bool{
	"trackedSymlinkAtWiredPath":        true,
	"staleWiredJunction":               true,
	"foreignDirAtFabricOwnedPath":      true,
	"unrelatedGitCloneAtWeftNamedPath": true,
}

// resolveStateTarget builds the StateTarget a real cell would resolve for stateName against h: the
// prime warp and weft checkouts for every dirtiness state, and a state-appropriate fabric-owned path for
// every structural state — an existing wired junction link for the two link states, and a fresh,
// not-yet-claimed pair path for the two foreign-entity states (a weft-named sibling path specifically
// for unrelatedGitCloneAtWeftNamedPath, matching that state's own name).
func resolveStateTarget(t *testing.T, h *Hub, stateName string) StateTarget {
	t.Helper()

	target := StateTarget{
		WarpCheckout: h.PrimeWorktree(),
		WeftCheckout: h.PrimeWeft(),
	}

	switch stateName {
	case "trackedSymlinkAtWiredPath", "staleWiredJunction":
		linkAbs, _ := firstWiredJunction(t, h)
		target.StructuralPath = linkAbs
	case "foreignDirAtFabricOwnedPath":
		target.StructuralPath = h.PairWarpWorktree("states-foreign-owner")
	case "unrelatedGitCloneAtWeftNamedPath":
		target.StructuralPath = h.PairWeftSibling("states-clone-owner")
	}

	return target
}

// newPorcelainLines returns the lines present in afterStatus but absent from beforeStatus, so a
// dirtiness assertion is immune to whatever baseline noise a checkout already carried before the state
// ran — a freshly cloned hub's prime weft sibling, in particular, always carries one such baseline
// line (see assertCleanHubEstablished's own doc comment for the full account), which a raw "every line
// must match" scan over the full porcelain output would wrongly blame on the state under test.
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

// assertOnlyNewShape fails t unless checkout carries at least one porcelain line new since
// beforeStatus, and every new line carries wantPrefix — never the opposite shape, which is the negative
// half of the tracked/untracked distinction dirtyWeftUntracked exists to make observable.
func assertOnlyNewShape(t *testing.T, checkout, beforeStatus, wantPrefix string) {
	t.Helper()

	newLines := newPorcelainLines(beforeStatus, GitStatusPorcelain(t, checkout))
	if len(newLines) == 0 {
		t.Fatalf("state failed to establish: no new dirt appeared in %s", checkout)
	}
	for _, line := range newLines {
		if !strings.HasPrefix(line, wantPrefix) {
			t.Fatalf("state established the wrong dirtiness shape in %s: new line %q; want prefix %q", checkout, line, wantPrefix)
		}
	}
}

// assertBothNewShapes fails t unless checkout carries at least one new tracked-modification line and at
// least one new untracked line since beforeStatus, proving bothDirty genuinely established both shapes
// rather than only one.
func assertBothNewShapes(t *testing.T, checkout, beforeStatus string) {
	t.Helper()

	newLines := newPorcelainLines(beforeStatus, GitStatusPorcelain(t, checkout))

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

// assertCleanHubEstablished fails t unless every checkout in h is genuinely clean of operator content.
//
// The warp checkout and the repo-wide board checkout must carry no git status --porcelain output at
// all. The prime weft sibling is allowed exactly one line: the freshly materialised, still-empty
// structural wiring directory WireJunctions creates before anything has ever been synced into it — git
// never tracks an empty directory, so it shows as untracked ("?? _lyx/" at anchor ".", "?? backend/" at
// anchor "backend", where the whole untracked subtree collapses to its top segment).
// This is fabric's own machine-local wiring scratch, not operator dirt — the same status
// junction.go's own doc comment gives ".lyx" ("always lyx's own machine-local scratch"). Anything beyond
// that single expected line is a genuine finding, not baseline noise.
func assertCleanHubEstablished(t *testing.T, h *Hub) {
	t.Helper()

	for _, checkout := range []string{h.PrimeWorktree(), h.BoardDir()} {
		if status := GitStatusPorcelain(t, checkout); status != "" {
			t.Errorf("clean state failed to establish: %s is not clean: %s", checkout, status)
		}
	}

	weftStatus := GitStatusPorcelain(t, h.PrimeWeft())
	if weftStatus == "" {
		return
	}

	allowed := "?? " + h.Anchor + "/"
	if h.Anchor == "." {
		allowed = "?? " + lyxdirs.LyxDirName + "/"
	}
	for _, line := range strings.Split(weftStatus, "\n") {
		if line == "" || line == allowed {
			continue
		}
		t.Errorf("clean state failed to establish: %s carries unexpected dirt: %q (full status: %s)", h.PrimeWeft(), line, weftStatus)
	}
}

// gitPathIsTracked reports whether relPath is tracked by git in dir, via `git ls-files
// --error-unmatch`.
func gitPathIsTracked(t *testing.T, dir, relPath string) bool {
	t.Helper()

	cmd := exec.Command("git", "ls-files", "--error-unmatch", relPath)
	cmd.Dir = dir
	return cmd.Run() == nil
}

// assertTrackedSymlinkEstablished fails t unless target.StructuralPath is a link whose raw one-hop
// target resolves to a real, on-disk directory and is git-tracked in target.WarpCheckout — the three
// properties trackedSymlinkAtWiredPath's own doc comment claims.
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

// assertStaleJunctionEstablished fails t unless target.StructuralPath is a link whose raw one-hop
// target does not resolve on disk.
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

// assertForeignDirEstablished fails t unless target.StructuralPath is a real, ordinary directory
// carrying its planted content, and is not itself a git checkout.
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

// assertUnrelatedCloneEstablished fails t unless target.StructuralPath is a real git repository
// carrying at least one commit and a clean working tree — the clean tree is what makes the dirty gate
// have nothing to say about it, leaving only the ownership gate between a destructive verb and the
// whole clone.
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

	if status := GitStatusPorcelain(t, target.StructuralPath); status != "" {
		t.Errorf("state failed to establish: %s is not clean: %s", target.StructuralPath, status)
	}
}

// assertStateEstablished dispatches to the assertion matching stateName, directly proving the
// condition that state's own doc comment in states.go claims.
// beforeWarp and beforeWeft are target.WarpCheckout's and target.WeftCheckout's own git status
// --porcelain output, captured before state.Apply ran, so a dirtiness assertion can isolate what the
// state itself changed from whatever baseline a checkout already carried.
func assertStateEstablished(t *testing.T, h *Hub, stateName string, target StateTarget, beforeWarp, beforeWeft string) {
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

// TestStates proves every state in States establishes what it claims, at both anchors, on its own
// freshly built hub.
func TestStates(t *testing.T) {
	for _, state := range States {
		state := state

		t.Run(state.Name, func(t *testing.T) {
			for _, anchor := range []string{".", "backend"} {
				anchor := anchor

				t.Run(anchor, func(t *testing.T) {
					t.Parallel()

					h := NewHub(t, anchor)
					target := resolveStateTarget(t, h, state.Name)

					beforeWarp := GitStatusPorcelain(t, target.WarpCheckout)
					beforeWeft := GitStatusPorcelain(t, target.WeftCheckout)

					var before Manifest
					if structuralStateNames[state.Name] {
						before = CaptureManifest(t, h.Path)
					}

					state.Apply(t, h, target)

					assertStateEstablished(t, h, state.Name, target, beforeWarp, beforeWeft)

					// A structural state that silently no-ops must not pass this suite by accident: the
					// manifest diff is the independent proof that applying the state actually changed
					// something on disk, distinct from the state-specific assertion above.
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
