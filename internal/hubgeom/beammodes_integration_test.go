// beammodes_integration_test.go drives BeamModes over one real hub from hubforge:
// the worktree each setting is read from, the beam's parent branch and its recorded pull request.

package hubgeom_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"

	"github.com/Knatte18/loomyard/internal/boardengine"
	"github.com/Knatte18/loomyard/internal/configengine"
	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/gitkit"
	"github.com/Knatte18/loomyard/internal/hubforge"
	"github.com/Knatte18/loomyard/internal/hubgeom"
	"github.com/Knatte18/loomyard/internal/loomengine"
	"github.com/Knatte18/loomyard/internal/lymcwd"
	"github.com/Knatte18/loomyard/internal/pawlengine"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// writeBeamFile writes document as the beam.yaml under dir, replacing any file there.
func writeBeamFile(t *testing.T, dir, document string) {
	t.Helper()
	path := configengine.ConfigFile(dir, "beam")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("MkdirAll(%q) = %v; want nil", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, []byte(document), 0o644); err != nil {
		t.Fatalf("WriteFile(%q) = %v; want nil", path, err)
	}
}

// TestBeamModes pins what the board's reader answers for the root and a beam.
// Each value comes with the worktree whose own files set it: root for the prime, empty for the hub-wide default.
// The beam also gets its parent branch from its origin record and its pull request from its landing record.
// A beam named with no worktree on this machine is left out.
func TestBeamModes(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()

	h := hubforge.CopyHub(t, hubforge.Shape{Anchor: "."})
	l := h.Location
	const beam = "bm-beam"
	hubforge.AddPairWith(t, h, beam, fabricengine.AddOptions{SkipPush: true})
	beamLocation, err := lymcwd.Resolve(fabricengine.WorktreePath(l, beam))
	if err != nil {
		t.Fatalf("lymcwd.Resolve(beam pair): %v", err)
	}
	writeBeamFile(t, h.BoardDir(), "autonomy: false\npull_request: \"off\"\n")
	writeBeamFile(t, l.AnchorPath(), "autonomy: true\n")
	writeBeamFile(t, beamLocation.AnchorPath(), "autonomy: unattended\npull_request: \"trail\"\n")
	const pullRequestURL = "https://github.com/example/repo/pull/12"
	setPullRequest := func(rec *pawlengine.LandingRecord) error {
		rec.PullRequest = &pawlengine.LandingPullRequest{Number: 12, URL: pullRequestURL}
		return nil
	}
	if err := pawlengine.UpdateLanding(loomengine.BeamLandingPath(beamLocation), loomengine.BeamLandingLockPath(beamLocation), beam, setPullRequest); err != nil {
		t.Fatalf("UpdateLanding(%q): %v", beam, err)
	}
	primeBranch := strings.TrimSpace(gitkit.Git(t, l.WorktreePath(), "branch", "--show-current"))

	got, err := hubgeom.BeamModes(h.BoardDir(), []string{beam, "bm-elsewhere"})
	if err != nil {
		t.Fatalf("BeamModes() error = %v; want nil", err)
	}
	want := boardengine.BeamModes{
		HubWide: boardengine.BeamMode{
			Autonomy:    boardengine.SettingValue{Value: "off"},
			PullRequest: boardengine.SettingValue{Value: "off"},
		},
		Root: boardengine.BeamMode{
			Autonomy:    boardengine.SettingValue{Value: "on", SetBy: boardengine.RootSubject},
			PullRequest: boardengine.SettingValue{Value: "off"},
		},
		Beams: map[string]boardengine.BeamMode{beam: {
			Autonomy:          boardengine.SettingValue{Value: "unattended", SetBy: beam},
			PullRequest:       boardengine.SettingValue{Value: "trail", SetBy: beam},
			ParentBranch:      primeBranch,
			PullRequestNumber: 12,
			PullRequestURL:    pullRequestURL,
		}},
	}
	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("BeamModes() mismatch (-want +got):\n%s", diff)
	}
}
