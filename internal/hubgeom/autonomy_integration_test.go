// autonomy_integration_test.go drives Autonomy over one real hub from hubforge:
// what the reader adds to the per-task lookup, its start at the named worktree and its warn-once failure.

package hubgeom_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/configengine"
	"github.com/Knatte18/loomyard/internal/dobbyengine"
	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/hubforge"
	"github.com/Knatte18/loomyard/internal/hubgeom"
	"github.com/Knatte18/loomyard/internal/lymcwd"
	"github.com/Knatte18/loomyard/internal/testkit/logcapture"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// writeAutonomy writes beam.autonomy under anchor as value, or removes the file when value is empty.
func writeAutonomy(t *testing.T, anchor, value string) {
	t.Helper()
	path := configengine.ConfigFile(anchor, "beam")
	if value == "" {
		if err := os.RemoveAll(path); err != nil {
			t.Fatalf("remove %s: %v", path, err)
		}
		return
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("MkdirAll(%q) = %v; want nil", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, []byte("autonomy: "+value+"\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(%q) = %v; want nil", path, err)
	}
}

// TestAutonomy pins what the reader adds to the per-task lookup:
// it starts at the worktree it is named.
// It reads the prime's name as the prime's chain.
// It reads false with one warning for a failed read.
// The rows rewrite the one hub's beam files and capture the process-wide logger, so they run serially.
func TestAutonomy(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()

	h := hubforge.CopyHub(t, hubforge.Shape{Anchor: "."})
	l := h.Location
	const (
		beamPair = "au-beam"
		task     = "au-task"
	)
	hubforge.AddPairWith(t, h, beamPair, fabricengine.AddOptions{SkipPush: true})
	beamLocation, err := lymcwd.Resolve(fabricengine.WorktreePath(l, beamPair))
	if err != nil {
		t.Fatalf("lymcwd.Resolve(beam pair): %v", err)
	}
	if _, err := h.Topology.Add(beamLocation, task, fabricengine.AddOptions{SkipPush: true}); err != nil {
		t.Fatalf("Add(%q) from the beam pair: %v", task, err)
	}
	board := `{"version":1,"entries":[` +
		`{"id":0,"slug":"` + beamPair + `","title":"Beam","kind":"project","labels":["bug"],"beam":true},` +
		`{"id":1,"slug":"` + task + `","title":"Task","kind":"task","labels":["bug"],"project":"` + beamPair + `"}]}`
	if err := os.WriteFile(filepath.Join(h.BoardDir(), "board.json"), []byte(board), 0o644); err != nil {
		t.Fatalf("write board.json: %v", err)
	}
	taskAnchor := fabricengine.WorktreePath(l, task)
	beamAnchor := fabricengine.WorktreePath(l, beamPair)

	tests := []struct {
		name string
		// taskValue, beamValue and primeValue are the autonomy each worktree's file sets, empty meaning no file.
		taskValue, beamValue, primeValue string
		// slug is the worktree the reader is named.
		slug string
		want dobbyengine.AutonomyLevel
	}{
		{name: "a task true under a beam false reads true", taskValue: "true", beamValue: "false", slug: task, want: dobbyengine.AutonomyOn},
		{name: "a task that sets nothing under a beam true reads true", beamValue: "true", slug: task, want: dobbyengine.AutonomyOn},
		{name: "a task false under a beam true reads false", taskValue: "false", beamValue: "true", slug: task, want: dobbyengine.AutonomyOff},
		{name: "a task unattended under a beam true reads unattended", taskValue: "unattended", beamValue: "true", slug: task, want: dobbyengine.AutonomyUnattended},
		{name: "the beam's own name starts at the beam, not at its parent", beamValue: "true", primeValue: "false", slug: beamPair, want: dobbyengine.AutonomyOn},
		{name: "the prime's own name reads the prime's chain", primeValue: "true", slug: l.WorktreeName, want: dobbyengine.AutonomyOn},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			writeAutonomy(t, taskAnchor, tt.taskValue)
			writeAutonomy(t, beamAnchor, tt.beamValue)
			writeAutonomy(t, l.AnchorPath(), tt.primeValue)
			if got, refusal, read := hubgeom.AutonomyWithRefusal(l, tt.slug); got != tt.want || refusal != nil || !read {
				t.Errorf("AutonomyWithRefusal(%q) = %q, %v, %v; want %q, nil, true", tt.slug, got, refusal, read, tt.want)
			}
		})
	}

	const warning = "hubgeom: read autonomy failed"
	t.Run("a refused value reads false, hands back its refusal and warns once", func(t *testing.T) {
		logs := logcapture.Capture(t)
		writeAutonomy(t, taskAnchor, "")
		writeAutonomy(t, beamAnchor, `"yes"`)
		writeAutonomy(t, l.AnchorPath(), "")
		for call := 1; call <= 2; call++ {
			got, refusal, read := hubgeom.AutonomyWithRefusal(l, task)
			if got != dobbyengine.AutonomyOff || read {
				t.Errorf("AutonomyWithRefusal(%q) call %d = %q, read %v; want %q, false", task, call, got, read, dobbyengine.AutonomyOff)
			}
			if refusal == nil || !strings.Contains(refusal.Error(), "lym config beam --set autonomy=") {
				t.Errorf("AutonomyWithRefusal(%q) call %d refusal = %v; want one naming the way forward", task, call, refusal)
			}
		}
		if got := strings.Count(logs.String(), warning); got != 1 {
			t.Errorf("warnings = %d; want 1 in %q", got, logs.String())
		}
		if !strings.Contains(logs.String(), beamPair) {
			t.Errorf("log %q does not name the beam's worktree %q", logs.String(), beamPair)
		}
	})

	t.Run("a location that is no git worktree reads false with a warning and no refusal", func(t *testing.T) {
		logs := logcapture.Capture(t)
		plain := &lymcwd.Location{HubPath: t.TempDir(), WorktreeName: "au-plain", AnchorRel: "."}
		if got, refusal, read := hubgeom.AutonomyWithRefusal(plain, ""); got != dobbyengine.AutonomyOff || refusal != nil || read {
			t.Errorf("AutonomyWithRefusal(plain, \"\") = %q, %v, %v; want %q, nil, false", got, refusal, read, dobbyengine.AutonomyOff)
		}
		if got := strings.Count(logs.String(), warning); got != 1 {
			t.Errorf("warnings = %d; want 1 in %q", got, logs.String())
		}
	})
}
