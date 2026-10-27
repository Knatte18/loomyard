//go:build tier1

// beam_integration_test.go proves an orch can run in a beam over one hub: a marked beam pair's orch paths lie under its own anchor, its reed geometry names the orch strand under the beam's slug, and HubOrchs lists it while skipping a pair not yet wired.

package orchcli

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/agentname"
	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/fslink"
	"github.com/Knatte18/loomyard/internal/hubforge"
	"github.com/Knatte18/loomyard/internal/hubgeom"
	"github.com/Knatte18/loomyard/internal/lymcwd"
	"github.com/Knatte18/loomyard/internal/lymdirs"
	"github.com/Knatte18/loomyard/internal/testkit/logcapture"
)

func TestOrchInBeam(t *testing.T) {
	t.Parallel()

	const slug = "b"
	h := hubforge.CopyHub(t, hubforge.Shape{Anchor: "."})
	hubforge.AddPairWith(t, h, slug, fabricengine.AddOptions{SkipPush: true})
	if marked, err := fabricengine.MarkBeam(fabricengine.NewMutations(h.Location.HubPath), h.Location, slug); err != nil || !marked {
		t.Fatalf("MarkBeam(%q) = (%v, %v); want (true, nil)", slug, marked, err)
	}
	beam, err := lymcwd.ResolveWorktree(h.PairCodeWorktree(slug))
	if err != nil {
		t.Fatalf("ResolveWorktree(%q): %v", slug, err)
	}

	paths := orchPaths(beam)
	anchor := beam.AnchorPath()
	if want := filepath.Join(anchor, lymdirs.DotLymDirName, orchDirName); paths.Dir != want {
		t.Errorf("orch dir of the beam = %q; want %q", paths.Dir, want)
	}
	if want := filepath.Join(anchor, lymdirs.LymDirName, orchDirName, "rules.md"); paths.OwnRulesPath != want {
		t.Errorf("own rules path of the beam = %q; want %q", paths.OwnRulesPath, want)
	}
	primeAnchor := h.Location.AnchorPath()
	for _, got := range []string{paths.Dir, paths.OwnRulesPath, paths.StatePath} {
		if strings.HasPrefix(got, primeAnchor+string(filepath.Separator)) && !strings.HasPrefix(got, anchor+string(filepath.Separator)) {
			t.Errorf("beam orch path %q lies under the prime's anchor %q", got, primeAnchor)
		}
	}

	geom, err := hubgeom.ReedGeometry(beam)
	if err != nil {
		t.Fatalf("ReedGeometry(%q): %v", slug, err)
	}
	got, err := agentname.Format(geom.NameShortname, geom.NameSlug, agentname.RoleOrch)
	if err != nil {
		t.Fatalf("Format the orch strand name: %v", err)
	}
	if want := hubforge.TestShortname + ":" + slug + ":orch"; got != want {
		t.Errorf("orch strand name in the beam = %q; want %q", got, want)
	}
}

// It is not parallel: logcapture.CaptureVerbose swaps the process-global logger to read the warnings.
func TestHubOrchs_ListsWiredBeamsAndSkipsUnwiredPairs(t *testing.T) {
	logs := logcapture.CaptureVerbose(t)

	h := hubforge.CopyHub(t, hubforge.Shape{Anchor: "."})
	for _, slug := range []string{"wired", "unwired"} {
		hubforge.AddPairWith(t, h, slug, fabricengine.AddOptions{SkipPush: true})
		if marked, err := fabricengine.MarkBeam(fabricengine.NewMutations(h.Location.HubPath), h.Location, slug); err != nil || !marked {
			t.Fatalf("MarkBeam(%q) = (%v, %v); want (true, nil)", slug, marked, err)
		}
	}
	unwiredLym := fabricengine.CodeLymLink(h.Location, "unwired")
	if err := fslink.Remove(unwiredLym); err != nil {
		t.Fatalf("Remove(%q): %v", unwiredLym, err)
	}

	logs.Reset()
	orchs, err := HubOrchs(h.Location)
	if err != nil {
		t.Fatalf("HubOrchs() error = %v", err)
	}
	var got []string
	for _, orch := range orchs {
		got = append(got, orch.WorktreeName)
	}
	if want := []string{h.Location.WorktreeName, "wired"}; !slices.Equal(got, want) {
		t.Errorf("HubOrchs() = %v; want %v", got, want)
	}
	if _, err := os.Lstat(unwiredLym); !os.IsNotExist(err) {
		t.Errorf("Lstat(%q) error = %v; want HubOrchs to leave no _lym in the unwired pair", unwiredLym, err)
	}
	if strings.Contains(logs.String(), "level=WARN") {
		t.Errorf("HubOrchs() logged %q; want no warning for a pair still being wired", logs.String())
	}
}
