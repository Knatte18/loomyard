// contractevidence_test.go covers contractFileStatus and the sites that consult it without git:
// AcceptPendingAudit and pendingPathsWayForward over contract-file paths only.
// Untagged, with a shuttlefake.Engine — no git, no subprocess spawns.

package websterengine

import (
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/shuttleengine"
	"github.com/Knatte18/loomyard/internal/summaryparser"
	"github.com/Knatte18/loomyard/internal/testkit/shuttlefake"
)

var evidenceEpoch = time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)

// evidenceGeom is a geometry whose directories are all scratch, so no path in it is in a git repository.
func evidenceGeom(t *testing.T) Geometry {
	t.Helper()
	root := t.TempDir()
	return Geometry{
		WorktreeRoot: filepath.Join(root, "wt"),
		PlanDir:      filepath.Join(root, "wt", "_lyx", "plan"),
		WebsterDir:   filepath.Join(root, "wt", "_lyx", "webster"),
		ScratchDir:   filepath.Join(root, "wt", ".lyx", "webster"),
	}
}

// writeContractFile creates the run's outcome.yaml and returns its path.
func writeContractFile(t *testing.T, geom Geometry) string {
	t.Helper()
	path := OutcomePath(geom.WebsterDir)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("outcome: done\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func succeeded(path string, offset time.Duration) shuttleengine.WriteEvent {
	return shuttleengine.WriteEvent{Path: path, At: evidenceEpoch.Add(offset), Succeeded: true}
}

func TestContractFileStatus(t *testing.T) {
	geom := evidenceGeom(t)
	present := writeContractFile(t, geom)
	absent := summaryparser.Path(geom.WebsterDir)
	fork := succeeded(present, time.Minute)

	tests := []struct {
		name         string
		path         string
		writes       RunWrites
		wantContract bool
		wantCleared  bool
	}{
		{name: "absent file is cleared", path: absent, wantContract: true, wantCleared: true},
		{
			name: "master's later write clears", path: present,
			writes:       RunWrites{Master: []shuttleengine.WriteEvent{succeeded(present, 2*time.Minute)}, Forks: []shuttleengine.WriteEvent{fork}},
			wantContract: true, wantCleared: true,
		},
		{
			name: "master's later failed write does not clear", path: present,
			writes: RunWrites{
				Master: []shuttleengine.WriteEvent{{Path: present, At: evidenceEpoch.Add(2 * time.Minute)}},
				Forks:  []shuttleengine.WriteEvent{fork},
			},
			wantContract: true,
		},
		{
			name: "the fork's write last does not clear", path: present,
			writes:       RunWrites{Master: []shuttleengine.WriteEvent{succeeded(present, 0)}, Forks: []shuttleengine.WriteEvent{fork}},
			wantContract: true,
		},
		{
			name: "no master write does not clear", path: present,
			writes:       RunWrites{Forks: []shuttleengine.WriteEvent{fork}},
			wantContract: true,
		},
		{
			name: "a non-contract _lyx path is no contract path", path: filepath.Join(geom.WebsterDir, "state.json"),
			writes: RunWrites{Forks: []shuttleengine.WriteEvent{fork}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			contract, cleared, err := contractFileStatus(geom, tt.writes, tt.path)
			if err != nil {
				t.Fatalf("contractFileStatus: %v", err)
			}
			if contract != tt.wantContract || cleared != tt.wantCleared {
				t.Errorf("contractFileStatus = (%v, %v), want (%v, %v)", contract, cleared, tt.wantContract, tt.wantCleared)
			}
		})
	}
}

// contractState is a state recording one Master session, whose findings name paths.
func contractState(paths ...string) *State {
	return &State{
		MasterSessionID:      "s1",
		PendingAuditFindings: []PendingAuditFinding{{ID: "f1", Class: "fork-contract-write", Detail: "a fork wrote a contract file", Paths: paths}},
	}
}

// writesEngine answers every session's audit with the given Master and fork events.
func writesEngine(master, fork []shuttleengine.WriteEvent) *shuttlefake.Engine {
	return &shuttlefake.Engine{AuditForksFn: func(string, string) (shuttleengine.ForkAudit, error) {
		return shuttleengine.ForkAudit{
			ParentWriteEvents: master,
			Forks:             []shuttleengine.ForkReport{{WriteEvents: fork}},
		}, nil
	}}
}

func TestAcceptPendingAudit_ContractFilesAbsentAccepts(t *testing.T) {
	geom := evidenceGeom(t)
	st := contractState(OutcomePath(geom.WebsterDir), summaryparser.Path(geom.WebsterDir))

	got, onAbsent, err := AcceptPendingAudit(writesEngine(nil, nil), st, geom, nil)
	if err != nil {
		t.Fatalf("AcceptPendingAudit: %v", err)
	}
	if len(got) != 1 || len(st.PendingAuditFindings) != 0 {
		t.Errorf("accepted %v, pending %v; want one cleared finding", got, st.PendingAuditFindings)
	}
	if !onAbsent {
		t.Error("onAbsentContract = false; want true on absent contract files")
	}
}

func TestAcceptPendingAudit_MasterWroteAfterForkAccepts(t *testing.T) {
	geom := evidenceGeom(t)
	path := writeContractFile(t, geom)
	st := contractState(path)
	engine := writesEngine([]shuttleengine.WriteEvent{succeeded(path, 2*time.Minute)}, []shuttleengine.WriteEvent{succeeded(path, time.Minute)})

	got, onAbsent, err := AcceptPendingAudit(engine, st, geom, nil)
	if err != nil {
		t.Fatalf("AcceptPendingAudit: %v", err)
	}
	if len(got) != 1 || len(st.PendingAuditFindings) != 0 {
		t.Errorf("accepted %v, pending %v; want one cleared finding", got, st.PendingAuditFindings)
	}
	if onAbsent {
		t.Error("onAbsentContract = true; want false when the file exists")
	}
}

func TestAcceptPendingAudit_ForkWroteLastRefusesNamingDeleteRoute(t *testing.T) {
	geom := evidenceGeom(t)
	path := writeContractFile(t, geom)
	st := contractState(path)
	engine := writesEngine([]shuttleengine.WriteEvent{succeeded(path, 0)}, []shuttleengine.WriteEvent{succeeded(path, time.Minute)})

	_, _, err := AcceptPendingAudit(engine, st, geom, nil)
	if !errors.Is(err, ErrAuditNotAcceptable) {
		t.Fatalf("AcceptPendingAudit error = %v; want ErrAuditNotAcceptable", err)
	}
	for _, want := range []string{"rm " + path, "after Master's last write", "lyx webster accept-audit"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q lacks %q", err, want)
		}
	}
	if len(st.PendingAuditFindings) != 1 {
		t.Errorf("pending = %v; want unchanged", st.PendingAuditFindings)
	}
}

func TestPendingPathsWayForward_ContractPaths(t *testing.T) {
	geom := evidenceGeom(t)
	path := writeContractFile(t, geom)

	st := contractState(path)

	cleared, _, err := pendingPathsWayForward(geom, st, RunWrites{Master: []shuttleengine.WriteEvent{succeeded(path, time.Minute)}}, []string{path}, false, "lyx webster run")
	if err != nil {
		t.Fatalf("cleared: %v", err)
	}
	if want := []string{"lyx webster accept-audit", "lyx webster run"}; !slices.Equal(cleared, want) {
		t.Errorf("cleared steps = %q; want %q", cleared, want)
	}

	uncleared, notes, err := pendingPathsWayForward(geom, st, RunWrites{Forks: []shuttleengine.WriteEvent{succeeded(path, time.Minute)}}, []string{path}, false, "lyx webster run")
	if err != nil {
		t.Fatalf("uncleared: %v", err)
	}
	if want := []string{"rm " + path, "lyx webster accept-audit", "lyx webster run"}; !slices.Equal(uncleared, want) {
		t.Errorf("uncleared steps = %q; want %q", uncleared, want)
	}
	if !strings.Contains(notes[path], "after Master's last write") {
		t.Errorf("uncleared note = %q; want it to say a fork wrote last", notes[path])
	}
}
