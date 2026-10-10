// webstergeom_test.go pins WebsterGeometry against the websterengine accessors, planparser.PlanDir
// and fabricengine.StencilsDir themselves, so this test cannot drift from those packages' own joins.
// Following hubgeom_test.go's fixture discipline, hub, worktree root, and anchor path stay three
// distinct directories so a field mix-up inside WebsterGeometry surfaces instead of passing
// silently.

package hubgeom

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/configengine"
	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/gateslot"
	"github.com/Knatte18/loomyard/internal/lyxcwd"
	"github.com/Knatte18/loomyard/internal/planparser"
	"github.com/Knatte18/loomyard/internal/verifytree"
	"github.com/Knatte18/loomyard/internal/websterengine"
)

//testtiming:keep pins every WebsterGeometry directory against the accessors of the packages that own them, which the integration test covering its blocks never reads
func TestWebsterGeometry(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		anchorRel string
	}{
		{"subpath-anchored fixture", filepath.Join("sub", "dir")},
		{"unanchored fixture", "."},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			root := t.TempDir()
			hub := filepath.Join(root, "some-hub-LYXHUB")
			worktreeName := "some-worktree"
			worktreeRoot := filepath.Join(hub, worktreeName)
			anchorPath := filepath.Join(worktreeRoot, tt.anchorRel)

			l := &lyxcwd.Location{
				RepoName:     "distinct-repo-name",
				HubPath:      hub,
				WorktreeName: worktreeName,
				AnchorRel:    tt.anchorRel,
			}

			got := WebsterGeometry(l)

			if got.AnchorRoot != anchorPath {
				t.Errorf("WebsterGeometry(l).AnchorRoot = %q; want %q", got.AnchorRoot, anchorPath)
			}
			if got.WorktreeRoot != l.AnchorPath() {
				t.Errorf("WebsterGeometry(l).WorktreeRoot = %q; want %q (l.AnchorPath())", got.WorktreeRoot, l.AnchorPath())
			}
			if got.RepoRoot != worktreeRoot {
				t.Errorf("WebsterGeometry(l).RepoRoot = %q; want %q (l.WorktreePath(), not the anchor path)", got.RepoRoot, worktreeRoot)
			}
			if tt.anchorRel != "." && got.WorktreeRoot == l.WorktreePath() {
				// The subpath-anchored row must catch a later "consistency fix"
				// that converges webster's field on reed's WorktreePath: the two
				// only coincide when AnchorRel is ".", which this row deliberately
				// is not.
				t.Errorf("WebsterGeometry(l).WorktreeRoot = %q; want != l.WorktreePath() %q", got.WorktreeRoot, l.WorktreePath())
			}
			if want := websterengine.Dir(anchorPath); got.WebsterDir != want {
				t.Errorf("WebsterGeometry(l).WebsterDir = %q; want %q", got.WebsterDir, want)
			}
			if want := websterengine.ReportsDir(anchorPath); got.ReportsDir != want {
				t.Errorf("WebsterGeometry(l).ReportsDir = %q; want %q", got.ReportsDir, want)
			}
			if want := websterengine.ScratchDir(anchorPath); got.ScratchDir != want {
				t.Errorf("WebsterGeometry(l).ScratchDir = %q; want %q", got.ScratchDir, want)
			}
			if want := websterengine.PromptsDir(anchorPath); got.PromptsDir != want {
				t.Errorf("WebsterGeometry(l).PromptsDir = %q; want %q", got.PromptsDir, want)
			}
			if want := fabricengine.StencilsDir(hub); got.StencilsDir != want {
				t.Errorf("WebsterGeometry(l).StencilsDir = %q; want %q", got.StencilsDir, want)
			}
			if want := fabricengine.SpecsDir(hub); got.SpecsDir != want {
				t.Errorf("WebsterGeometry(l).SpecsDir = %q; want %q", got.SpecsDir, want)
			}
			if want := planparser.PlanDir(anchorPath); got.PlanDir != want {
				t.Errorf("WebsterGeometry(l).PlanDir = %q; want %q", got.PlanDir, want)
			}
			if want := verifytree.Dir(anchorPath); got.VerifyDir != want {
				t.Errorf("WebsterGeometry(l).VerifyDir = %q; want %q", got.VerifyDir, want)
			}
			if want := gateslot.WaitDir(anchorPath); got.GateWaitDir != want {
				t.Errorf("WebsterGeometry(l).GateWaitDir = %q; want %q", got.GateWaitDir, want)
			}
			if got.GateSlots == nil {
				t.Fatal("WebsterGeometry(l).GateSlots = nil; want the hub's pool")
			}
			boardDir := fabricengine.BoardDir(hub)
			if want := gateslot.Dir(boardDir); got.GateSlots.Dir != want {
				t.Errorf("WebsterGeometry(l).GateSlots.Dir = %q; want %q", got.GateSlots.Dir, want)
			}
			gateConfig := configengine.ConfigFile(boardDir, "gate")
			if _, err := got.GateSlots.Limits(); err == nil || !strings.Contains(err.Error(), gateConfig) || !strings.Contains(err.Error(), `run "lyx fabric reconcile"`) {
				t.Errorf("GateSlots.Limits() before gate.yaml exists = %v; want an error naming %q and lyx fabric reconcile", err, gateConfig)
			}
			if err := os.MkdirAll(filepath.Dir(gateConfig), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(gateConfig, []byte("slots: 0\ngo_parallel: 5\ncli_wait_sec: 7\n"), 0o644); err != nil {
				t.Fatal(err)
			}
			if _, err := got.GateSlots.Limits(); err == nil || !strings.Contains(err.Error(), gateConfig) || !strings.Contains(err.Error(), `fix it with "lyx config gate" from the prime`) {
				t.Errorf("GateSlots.Limits() over slots: 0 = %v; want an error naming %q and lyx config gate", err, gateConfig)
			}
			if err := os.WriteFile(gateConfig, []byte("slots: 3\ngo_parallel: 5\ncli_wait_sec: 7\n"), 0o644); err != nil {
				t.Fatal(err)
			}
			wantLimits := gateslot.Limits{Slots: 3, GoParallel: 5, CLIWait: 7 * time.Second}
			if limits, err := got.GateSlots.Limits(); err != nil || limits != wantLimits {
				t.Errorf("GateSlots.Limits() = (%+v, %v); want the board dir's gate.yaml as %+v", limits, err, wantLimits)
			}
		})
	}
}
