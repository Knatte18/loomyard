// dobbynoticestamp_integration_test.go proves a dobby's run notices carry the run's autonomy, over a real hub.
// Creel's daemon runs in process on a fake delivery.

package beamcli

import (
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/Knatte18/loomyard/internal/agentname"
	"github.com/Knatte18/loomyard/internal/configengine"
	"github.com/Knatte18/loomyard/internal/creelcli"
	"github.com/Knatte18/loomyard/internal/creelengine"
	"github.com/Knatte18/loomyard/internal/hubforge"
	"github.com/Knatte18/loomyard/internal/hubgeom"
	"github.com/Knatte18/loomyard/internal/lymcwd"
	"github.com/Knatte18/loomyard/internal/lymdirs"
	"github.com/Knatte18/loomyard/internal/orchcli"
	"github.com/Knatte18/loomyard/internal/orchengine"
	"github.com/Knatte18/loomyard/internal/reedengine"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// TestHubDobbyDeps_RunNoticePostCarriesTheRunsAutonomy pins that each dobby stamps a run notice with the run's autonomy.
// The prime's dobby signs unscoped and a beam's dobby signs in its own scope, and both stamp the run "task1".
// The value is read from the run's beam.yaml when each notice is built.
// It runs without t.Parallel: the strand environment variable is process-global.
func TestHubDobbyDeps_RunNoticePostCarriesTheRunsAutonomy(t *testing.T) {
	tiergate.Require(t, 1)
	t.Setenv(agentname.StrandNameEnv, "")

	h := hubforge.CopyHub(t, hubforge.Shape{Anchor: "."})
	hubforge.AddPair(t, h, "task1")
	prime := h.Location
	slack, err := hubgeom.SlackGeometry(prime.HubPath)
	if err != nil {
		t.Fatalf("SlackGeometry = %v; want nil", err)
	}
	receiver := slack.Shortname + ":orch-held"
	if err := orchengine.SaveState(orchcli.PrimePaths(prime), orchengine.State{Phase: orchengine.PhaseIdle, Strand: "g1"}); err != nil {
		t.Fatalf("orchengine.SaveState() = %v; want nil", err)
	}
	reedState := &reedengine.ReedState{Strands: []reedengine.Strand{{GUID: "g1", Name: receiver}}}
	if err := reedengine.SaveState(filepath.Join(prime.AnchorPath(), lymdirs.DotLymDirName), reedState); err != nil {
		t.Fatalf("reedengine.SaveState() = %v; want nil", err)
	}
	startCreelDaemon(t, h, receiver)
	task, err := lymcwd.ResolveWorktree(h.PairCodeWorktree("task1"))
	if err != nil {
		t.Fatalf("lymcwd.ResolveWorktree(task1) = %v; want nil", err)
	}

	tests := []struct {
		name     string
		dobby    string
		autonomy string
		wantFrom string
	}{
		{"PrimeDobbyOn", prime.WorktreeName, "true", slack.Shortname + ":DOBBY"},
		{"BeamDobbyOff", "task1", "false", slack.Shortname + ":task1:DOBBY"},
	}
	type mail struct{ from, subject string }
	var want []mail
	for _, tt := range tests {
		geom, err := hubgeom.DobbyGeometry(prime, tt.dobby)
		if err != nil {
			t.Fatalf("%s: DobbyGeometry = %v; want nil", tt.name, err)
		}
		deps, err := hubDobbyDeps(prime, geom)
		if err != nil {
			t.Fatalf("%s: hubDobbyDeps = %v; want nil", tt.name, err)
		}
		path := configengine.ConfigFile(task.AnchorPath(), "beam")
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatalf("MkdirAll(%q) = %v; want nil", filepath.Dir(path), err)
		}
		if err := os.WriteFile(path, []byte("autonomy: "+tt.autonomy+"\n"), 0o644); err != nil {
			t.Fatalf("WriteFile(%q) = %v; want nil", path, err)
		}
		line := tt.name + ": the driver died"
		if err := deps.RunNotices.Post(t.Context(), "task1", line); err != nil {
			t.Fatalf("%s: RunNotices.Post = %v; want nil", tt.name, err)
		}
		want = append(want, mail{tt.wantFrom, line + "; autonomy: " + tt.autonomy})
	}

	reader, err := creelcli.NewProgramClient(prime, "CHECK")
	if err != nil {
		t.Fatalf("NewProgramClient(CHECK) = %v; want nil", err)
	}
	entries, err := reader.Log(creelengine.LogFilter{})
	if err != nil {
		t.Fatalf("Log() = %v; want nil", err)
	}
	var got []mail
	for _, entry := range entries {
		if entry.Event == "posted" {
			got = append(got, mail{entry.From, entry.Subject})
		}
	}
	if !slices.Equal(got, want) {
		t.Errorf("the posted mails = %q; want %q", got, want)
	}
}
