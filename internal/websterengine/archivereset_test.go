// archivereset_test.go covers ArchiveRunAfterReset: the pending-findings guard it runs against HEAD and the in-place archive it performs.
// Tier 1: the package's fakeGit and a run record under t.TempDir(), no git process.

package websterengine_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/websterengine"
)

// resetArchiveFixture is a run record on disk, started at the fake git's root commit, holding one pending audit finding.
type resetArchiveFixture struct {
	t      *testing.T
	git    *fakeGit
	geom   websterengine.Geometry
	state  *websterengine.State
	marker string
}

func newResetArchiveFixture(t *testing.T) *resetArchiveFixture {
	t.Helper()
	root := t.TempDir()
	git := newFakeGit()
	geom := websterengine.Geometry{
		WorktreeRoot: filepath.Join(root, "worktree"),
		WebsterDir:   filepath.Join(root, "webster"),
		ReportsDir:   filepath.Join(root, "webster", "reports"),
		PromptsDir:   filepath.Join(root, "scratch", "prompts"),
		ScratchDir:   filepath.Join(root, "scratch"),
		PlanDir:      filepath.Join(root, "worktree", "plan"),
		Git:          git,
	}
	st := &websterengine.State{
		RunGUID:        "run-1",
		PlanFileHashes: map[string]string{"00-overview.md": "recorded"},
		Batches:        map[int]*websterengine.BatchState{1: {Slug: "one", Kind: "fork", StartSHA: git.head}},
		PendingAuditFindings: []websterengine.PendingAuditFinding{
			{ID: "sess/parent:write:1", Class: "parent-write", Detail: "master wrote a file"},
		},
	}
	marker := filepath.Join(geom.ReportsDir, "marker.yaml")
	for path, content := range map[string]string{
		marker:                                  "x",
		filepath.Join(geom.PromptsDir, "01.md"): "p",
		filepath.Join(geom.WorktreeRoot, "base.txt"): "base",
		filepath.Join(geom.PlanDir, "01-new.md"):     "new",
	} {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if err := websterengine.SaveState(geom.WebsterDir, geom.ScratchDir, st); err != nil {
		t.Fatal(err)
	}
	return &resetArchiveFixture{t: t, git: git, geom: geom, state: st, marker: marker}
}

// exists reports whether path is on disk.
func exists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// archived lists the entries of dir matching pattern.
func archived(t *testing.T, dir, pattern string) []string {
	t.Helper()
	matches, err := filepath.Glob(filepath.Join(dir, pattern))
	if err != nil {
		t.Fatal(err)
	}
	return matches
}

// TestArchiveRunAfterReset pins the guard's three refusals, each naming the clearing step and then the reset, with the run record left in place,
// and the archive a pending finding does not block: state.json and the reports dir renamed, the prompts cleared, one drop warning per finding.
func TestArchiveRunAfterReset(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		// arrange returns the fixture, with the finding's path in place.
		arrange func(t *testing.T) *resetArchiveFixture
		// wantRefusal holds the parts the refusal carries; empty means the record is archived.
		wantRefusal []string
	}{
		{
			name: "an uncleared contract file a fork wrote last",
			arrange: func(t *testing.T) *resetArchiveFixture {
				fx := newResetArchiveFixture(t)
				contract := websterengine.OutcomePath(fx.geom.WebsterDir)
				fx.state.PendingAuditFindings[0].Paths = []string{contract}
				if err := os.WriteFile(contract, []byte("outcome: done\n"), 0o644); err != nil {
					t.Fatal(err)
				}
				return fx
			},
			wantRefusal: []string{"reset --to start would drop pending audit findings", "1) rm ", "2) lyx webster reset --to start"},
		},
		{
			name: "a plan path differing from the recorded plan",
			arrange: func(t *testing.T) *resetArchiveFixture {
				fx := newResetArchiveFixture(t)
				fx.state.PendingAuditFindings[0].Paths = []string{filepath.Join(fx.geom.PlanDir, "01-new.md")}
				return fx
			},
			wantRefusal: []string{"plan file(s) differ from the plan the run recorded", "lyx webster restore-plan", "lyx webster rebaseline --card NN", `"lyx webster reset --to start"`},
		},
		{
			name: "a suspect path differing from HEAD",
			arrange: func(t *testing.T) *resetArchiveFixture {
				fx := newResetArchiveFixture(t)
				suspect := filepath.Join(fx.geom.WorktreeRoot, "base.txt")
				fx.state.PendingAuditFindings[0].Paths = []string{suspect}
				fx.git.differing[suspect] = true
				return fx
			},
			wantRefusal: []string{"suspect paths still differ from HEAD", "1) git checkout ", "-- " + "%suspect", "2) lyx webster reset --to start"},
		},
		{
			name: "a branch rewritten under the run",
			arrange: func(t *testing.T) *resetArchiveFixture {
				fx := newResetArchiveFixture(t)
				fx.state.Batches[1].StartSHA = strings.Repeat("ab", 20)
				fx.state.PendingAuditFindings[0].Paths = []string{filepath.Join(fx.geom.WorktreeRoot, "base.txt")}
				return fx
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			fx := tt.arrange(t)
			warnings, err := websterengine.ArchiveRunAfterReset(nil, fx.geom, fx.state)

			if len(tt.wantRefusal) > 0 {
				if err == nil {
					t.Fatal("ArchiveRunAfterReset error = nil, want a refusal")
				}
				for _, part := range tt.wantRefusal {
					part = strings.ReplaceAll(part, "%suspect", filepath.Join(fx.geom.WorktreeRoot, "base.txt"))
					if !strings.Contains(err.Error(), part) {
						t.Errorf("error = %v, want it to contain %q", err, part)
					}
				}
				if !exists(filepath.Join(fx.geom.WebsterDir, "state.json")) || !exists(fx.marker) {
					t.Error("a refusal archived the run record; want state.json and the reports dir in place")
				}
				return
			}

			if err != nil {
				t.Fatalf("ArchiveRunAfterReset error = %v, want the record archived", err)
			}
			if len(warnings) != 1 || !strings.Contains(warnings[0], "reset --to start dropped pending audit finding sess/parent:write:1") {
				t.Errorf("warnings = %v, want one drop warning naming the finding", warnings)
			}
			if exists(filepath.Join(fx.geom.WebsterDir, "state.json")) || len(archived(t, fx.geom.WebsterDir, "state-*.json")) != 1 {
				t.Error("state.json was not renamed to a stamped archive")
			}
			if exists(fx.marker) || len(archived(t, fx.geom.WebsterDir, "reports-*")) != 1 || !exists(fx.geom.ReportsDir) {
				t.Error("the reports dir was not archived and recreated empty")
			}
			if exists(filepath.Join(fx.geom.PromptsDir, "01.md")) {
				t.Error("the rendered prompts were not cleared")
			}
		})
	}
}
