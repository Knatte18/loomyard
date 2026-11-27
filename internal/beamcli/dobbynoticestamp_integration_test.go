// dobbynoticestamp_integration_test.go proves gate-hold notices, run notices and watch lines carry the run's autonomy.
// The proof runs over a real hub, with creel's daemon in process on a fake delivery.

package beamcli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"
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

// TestHubDobbyDeps_RunNoticePostCarriesTheRunsAutonomy pins the autonomy stamp on a dobby's mails about a run:
// the gate-hold notice, the run notice and the watch line.
// The prime's dobby signs unscoped and a beam's dobby signs in its own scope, and both stamp the run "task1".
// The value is read from the run's beam.yaml when each mail is built.
// A watch line carries no tags, so no mail here replaces another unread one.
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
		holdLine := tt.name + ": a gate slot is held"
		if err := deps.GateHolds.Post(t.Context(), "task1", holdLine); err != nil {
			t.Fatalf("%s: GateHolds.Post = %v; want nil", tt.name, err)
		}
		want = append(want, mail{tt.wantFrom, holdLine + "; autonomy: " + tt.autonomy})

		line := tt.name + ": the driver died"
		if err := deps.RunNotices.Post(t.Context(), "task1", line); err != nil {
			t.Fatalf("%s: RunNotices.Post = %v; want nil", tt.name, err)
		}
		want = append(want, mail{tt.wantFrom, line + "; autonomy: " + tt.autonomy})

		watchLine := tt.name + ": the run halted"
		if posted, err := deps.RunWatch.Post(t.Context(), "task1", watchLine); !posted || err != nil {
			t.Fatalf("%s: RunWatch.Post = %v, %v; want true, nil", tt.name, posted, err)
		}
		want = append(want, mail{tt.wantFrom, watchLine + "; autonomy: " + tt.autonomy})
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
		if entry.Event == "undelivered" && entry.Reason == creelengine.SupersededReason {
			t.Errorf("message %d was superseded; want every mail standing unread", entry.ID)
		}
	}
	if !slices.Equal(got, want) {
		t.Errorf("the posted mails = %q; want %q", got, want)
	}

	// The log shows no tags, so they are read from the journal's posted records.
	creel, err := hubgeom.CreelGeometry(prime.HubPath, t.TempDir())
	if err != nil {
		t.Fatalf("CreelGeometry() = %v; want nil", err)
	}
	days, err := filepath.Glob(filepath.Join(creel.JournalDir, "*"))
	if err != nil {
		t.Fatalf("Glob(%q) = %v; want nil", creel.JournalDir, err)
	}
	watchRecords := 0
	for _, day := range days {
		data, err := os.ReadFile(day)
		if err != nil {
			t.Fatalf("ReadFile(%q) = %v; want nil", day, err)
		}
		for line := range strings.Lines(string(data)) {
			var record struct{ Event, Body, Topic, Run string }
			if err := json.Unmarshal([]byte(line), &record); err != nil {
				t.Fatalf("Unmarshal(%q) = %v; want nil", line, err)
			}
			if record.Event != "posted" || !strings.Contains(record.Body, "the run halted") {
				continue
			}
			watchRecords++
			if record.Topic != "" || record.Run != "" {
				t.Errorf("watch line %q has topic %q and run %q; want neither", record.Body, record.Topic, record.Run)
			}
		}
	}
	if watchRecords != len(tests) {
		t.Errorf("the journal holds %d posted watch lines; want %d", watchRecords, len(tests))
	}
}
