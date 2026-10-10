// contractevidence_test.go covers contractFileStatus and AcceptPendingAudit over contract-file
// paths, without git.
// Untagged, with a shuttlefake.Engine — no git, no subprocess spawns.

package websterengine

import (
	"errors"
	"os"
	"path/filepath"
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
		wantWriter   string
	}{
		{name: "absent file is cleared", path: absent, wantContract: true, wantCleared: true},
		{
			name: "master's later write clears", path: present,
			writes:       RunWrites{Master: []shuttleengine.WriteEvent{succeeded(present, 2*time.Minute)}, Forks: []shuttleengine.WriteEvent{fork}},
			wantContract: true, wantCleared: true,
		},
		{
			name: "master's later write clears over a recovery write too", path: present,
			writes:       RunWrites{Master: []shuttleengine.WriteEvent{succeeded(present, 3*time.Minute)}, Forks: []shuttleengine.WriteEvent{fork}, Recoveries: []shuttleengine.WriteEvent{succeeded(present, 2*time.Minute)}},
			wantContract: true, wantCleared: true,
		},
		{
			name: "a recovery write after master's does not clear", path: present,
			writes:       RunWrites{Master: []shuttleengine.WriteEvent{succeeded(present, 0)}, Recoveries: []shuttleengine.WriteEvent{succeeded(present, time.Minute)}},
			wantContract: true, wantWriter: writerRecovery,
		},
		{
			name: "the later of a fork and a recovery write is named", path: present,
			writes:       RunWrites{Master: []shuttleengine.WriteEvent{succeeded(present, 0)}, Forks: []shuttleengine.WriteEvent{fork}, Recoveries: []shuttleengine.WriteEvent{succeeded(present, 2*time.Minute)}},
			wantContract: true, wantWriter: writerRecovery,
		},
		{
			name: "master's later failed write does not clear", path: present,
			writes: RunWrites{
				Master: []shuttleengine.WriteEvent{{Path: present, At: evidenceEpoch.Add(2 * time.Minute)}},
				Forks:  []shuttleengine.WriteEvent{fork},
			},
			wantContract: true, wantWriter: writerFork,
		},
		{
			name: "the fork's write last does not clear", path: present,
			writes:       RunWrites{Master: []shuttleengine.WriteEvent{succeeded(present, 0)}, Forks: []shuttleengine.WriteEvent{fork}},
			wantContract: true, wantWriter: writerFork,
		},
		{
			name: "no master write does not clear", path: present,
			writes:       RunWrites{Forks: []shuttleengine.WriteEvent{fork}},
			wantContract: true, wantWriter: writerFork,
		},
		{
			name: "a non-contract _lyx path is no contract path", path: filepath.Join(geom.WebsterDir, "state.json"),
			writes: RunWrites{Forks: []shuttleengine.WriteEvent{fork}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			contract, cleared, writer, err := contractFileStatus(geom, tt.writes, tt.path)
			if err != nil {
				t.Fatalf("contractFileStatus: %v", err)
			}
			if contract != tt.wantContract || cleared != tt.wantCleared || writer != tt.wantWriter {
				t.Errorf("contractFileStatus = (%v, %v, %q), want (%v, %v, %q)", contract, cleared, writer, tt.wantContract, tt.wantCleared, tt.wantWriter)
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

// TestAcceptPendingAudit_ContractFiles pins accept-audit over contract-file findings: absent files
// or a Master write after the fork's clear the finding, reporting whether the files were absent, and
// a fork write after Master's refuses, naming the delete route and leaving the finding pending.
func TestAcceptPendingAudit_ContractFiles(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		// fixture returns the finding's paths and the engine answering the audit.
		fixture      func(t *testing.T, geom Geometry) ([]string, shuttleengine.Engine)
		wantOnAbsent bool
		// wantRefusal lists what the refusal names; empty expects an accept.
		wantRefusal func(paths []string) []string
	}{
		{
			name: "absent contract files accept",
			fixture: func(t *testing.T, geom Geometry) ([]string, shuttleengine.Engine) {
				return []string{OutcomePath(geom.WebsterDir), summaryparser.Path(geom.WebsterDir)}, writesEngine(nil, nil)
			},
			wantOnAbsent: true,
		},
		{
			name: "Master writing after the fork accepts",
			fixture: func(t *testing.T, geom Geometry) ([]string, shuttleengine.Engine) {
				path := writeContractFile(t, geom)
				return []string{path}, writesEngine([]shuttleengine.WriteEvent{succeeded(path, 2*time.Minute)}, []shuttleengine.WriteEvent{succeeded(path, time.Minute)})
			},
		},
		{
			name: "the fork writing last refuses naming the delete route",
			fixture: func(t *testing.T, geom Geometry) ([]string, shuttleengine.Engine) {
				path := writeContractFile(t, geom)
				return []string{path}, writesEngine([]shuttleengine.WriteEvent{succeeded(path, 0)}, []shuttleengine.WriteEvent{succeeded(path, time.Minute)})
			},
			wantRefusal: func(paths []string) []string {
				return []string{"rm " + paths[0], "after Master's last write", "lyx webster accept-audit"}
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			geom := evidenceGeom(t)
			paths, engine := tt.fixture(t, geom)
			st := contractState(paths...)

			got, onAbsent, err := AcceptPendingAudit(engine, st, geom, nil)
			if tt.wantRefusal != nil {
				if !errors.Is(err, ErrAuditNotAcceptable) {
					t.Fatalf("AcceptPendingAudit error = %v; want ErrAuditNotAcceptable", err)
				}
				for _, want := range tt.wantRefusal(paths) {
					if !strings.Contains(err.Error(), want) {
						t.Errorf("error %q lacks %q", err, want)
					}
				}
				if len(st.PendingAuditFindings) != 1 {
					t.Errorf("pending = %v; want unchanged", st.PendingAuditFindings)
				}
				return
			}
			if err != nil {
				t.Fatalf("AcceptPendingAudit: %v", err)
			}
			if len(got) != 1 || len(st.PendingAuditFindings) != 0 {
				t.Errorf("accepted %v, pending %v; want one cleared finding", got, st.PendingAuditFindings)
			}
			if onAbsent != tt.wantOnAbsent {
				t.Errorf("onAbsentContract = %v; want %v", onAbsent, tt.wantOnAbsent)
			}
		})
	}
}
