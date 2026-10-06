// seed_test.go is the Tier-1 suite for CheckSeed, driving it directly over t.TempDir paths.
// It is untagged: no spawn, no git -- CheckSeed itself only stats a file, MkdirAll's a lock
// parent, and decodes JSON.

package loomengine

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/shedengine"
	"github.com/Knatte18/loomyard/internal/state"
)

// writeSeed marshals product into shedengine.Status's Product field and writes the resulting shell
// to statusPath/statusLockPath via state.WriteJSON, failing the test on error.
func writeSeed(t *testing.T, statusPath, statusLockPath string, shed shedengine.Status, product Status) {
	t.Helper()

	raw, err := json.Marshal(product)
	if err != nil {
		t.Fatalf("writeSeed: marshal product: %v", err)
	}
	shed.Product = raw

	if err := os.MkdirAll(filepath.Dir(statusLockPath), 0o755); err != nil {
		t.Fatalf("writeSeed: create status lock parent dir: %v", err)
	}
	if err := state.WriteJSON(statusPath, statusLockPath, shed); err != nil {
		t.Fatalf("writeSeed: state.WriteJSON(...) = %v", err)
	}
}

// coherentFreshShed returns a coherent, fresh shedengine.Status shell naming expectedProducer as
// current_producer, running, with one Done history entry naming a tolerated producer.
func coherentFreshShed(expectedProducer, historyProducer string) shedengine.Status {
	return shedengine.Status{
		CurrentProducer: expectedProducer,
		State:           shedengine.StateRunning,
		History: []shedengine.HistoryEntry{
			{Producer: historyProducer, Outcome: shedengine.Done, At: "2026-07-17T10:01:30Z"},
		},
	}
}

// coherentFreshProduct returns a coherent, fresh loom Status product: non-empty slug/parent and a
// null start_sha.
func coherentFreshProduct() Status {
	return Status{
		Slug:   "loom-contracts",
		Parent: "main",
	}
}

// TestCheckSeed drives CheckSeed over each seed a run can be started from, against loom's own row ("Loom-Preflight", tolerating the generic "Preflight" row before it) unless a row names another producer.
// A coherent seed reports OK; each incoherent one reports the failure that names its defect:
// a missing file, a file that does not decode (malformed JSON, an unknown top-level field), a product that does not decode as loom's status shape, an out-of-vocabulary history outcome (the file a broken adapter leaves behind, since Shed records an outcome verbatim) whose failure ends in the seed-a-new-run way forward, and a told producer name that is genuinely told rather than assumed.
// The deep-lock-parent row is the MkdirAll guard's regression: only the lock parent is missing, and that guard stops a worktree with no ephemeral tree from escalating the verdict to an infra error.
func TestCheckSeed(t *testing.T) {
	t.Parallel()
	loomRowTolerated := []string{"Preflight", "Loom-Preflight"}
	tests := []struct {
		name string
		// setup writes the status file under dir and returns the status and lock paths.
		setup      func(t *testing.T, dir string) (statusPath, statusLockPath string)
		producer   string
		tolerated  []string
		wantOK     bool
		wantCheck  CheckID
		wantReason string
		// wantWayForward, when set, also requires exactly one failure whose reason ends with it.
		wantWayForward string
	}{
		{
			name: "status file missing",
			setup: func(t *testing.T, dir string) (string, string) {
				return filepath.Join(dir, "status.json"), filepath.Join(dir, "ephemeral", "status.json.lock")
			},
			wantCheck: CheckSeedMissing,
		},
		{
			name: "malformed JSON",
			setup: func(t *testing.T, dir string) (string, string) {
				statusPath := filepath.Join(dir, "status.json")
				if err := os.WriteFile(statusPath, []byte("{not valid json"), 0o644); err != nil {
					t.Fatalf("write malformed status file: %v", err)
				}
				return statusPath, filepath.Join(dir, "ephemeral", "status.json.lock")
			},
			wantCheck: CheckSeedIncoherent,
		},
		{
			name: "unknown top-level field",
			setup: func(t *testing.T, dir string) (string, string) {
				statusPath := filepath.Join(dir, "status.json")
				body := `{"current_producer":"Loom-Preflight","state":"running","error":"","pause_requested":false,"activity":{"now":"","last":"","wait":""},"history":[],"product":{},"bogus_field":true}`
				if err := os.WriteFile(statusPath, []byte(body), 0o644); err != nil {
					t.Fatalf("write status file with unknown field: %v", err)
				}
				return statusPath, filepath.Join(dir, "ephemeral", "status.json.lock")
			},
			wantCheck: CheckSeedIncoherent,
		},
		{
			name: "product does not decode as loom's status",
			setup: func(t *testing.T, dir string) (string, string) {
				statusPath := filepath.Join(dir, "status.json")
				statusLockPath := filepath.Join(dir, "ephemeral", "status.json.lock")
				shed := coherentFreshShed("Loom-Preflight", "Loom-Preflight")
				shed.Product = []byte(`{"slug": 7}`)
				if err := os.MkdirAll(filepath.Dir(statusLockPath), 0o755); err != nil {
					t.Fatalf("create status lock parent dir: %v", err)
				}
				if err := state.WriteJSON(statusPath, statusLockPath, shed); err != nil {
					t.Fatalf("state.WriteJSON(...) = %v", err)
				}
				return statusPath, statusLockPath
			},
			wantCheck:  CheckSeedIncoherent,
			wantReason: "product does not decode as loom's status shape",
		},
		{
			name: "out-of-vocabulary outcome names the way forward",
			setup: func(t *testing.T, dir string) (string, string) {
				statusPath := filepath.Join(dir, "status.json")
				statusLockPath := filepath.Join(dir, "ephemeral", "status.json.lock")
				shed := coherentFreshShed("Loom-Preflight", "Preflight")
				shed.History[0].Outcome = "weird"
				writeSeed(t, statusPath, statusLockPath, shed, coherentFreshProduct())
				return statusPath, statusLockPath
			},
			wantCheck:      CheckSeedIncoherent,
			wantWayForward: "way forward: seed a new run",
		},
		{
			name: "coherent post-row-1 seed",
			setup: func(t *testing.T, dir string) (string, string) {
				statusPath := filepath.Join(dir, "status.json")
				statusLockPath := filepath.Join(dir, "ephemeral", "status.json.lock")
				writeSeed(t, statusPath, statusLockPath, coherentFreshShed("Loom-Preflight", "Preflight"), coherentFreshProduct())
				return statusPath, statusLockPath
			},
			wantOK: true,
		},
		{
			name: "lock parent several levels deep and missing",
			setup: func(t *testing.T, dir string) (string, string) {
				statusPath := filepath.Join(dir, "status.json")
				// The status file itself is written to an already-existing directory -- only the lock
				// parent is missing, since that is what the guard exists to create.
				shed := coherentFreshShed("Loom-Preflight", "Preflight")
				raw, err := json.Marshal(coherentFreshProduct())
				if err != nil {
					t.Fatalf("marshal product: %v", err)
				}
				shed.Product = raw
				if err := state.WriteJSON(statusPath, filepath.Join(dir, "seed.lock"), shed); err != nil {
					t.Fatalf("state.WriteJSON(...) = %v", err)
				}
				return statusPath, filepath.Join(dir, "a", "b", "c", "d", "status.json.lock")
			},
			wantOK: true,
		},
		{
			name: "told producer names are genuinely told",
			setup: func(t *testing.T, dir string) (string, string) {
				statusPath := filepath.Join(dir, "status.json")
				statusLockPath := filepath.Join(dir, "ephemeral", "status.json.lock")
				writeSeed(t, statusPath, statusLockPath, coherentFreshShed("Loom-Preflight", "Preflight"), coherentFreshProduct())
				return statusPath, statusLockPath
			},
			producer:  "Some-Other-Producer",
			tolerated: []string{"Some-Other-Producer"},
			wantCheck: CheckSeedIncoherent,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			statusPath, statusLockPath := tt.setup(t, t.TempDir())
			producer, tolerated := tt.producer, tt.tolerated
			if producer == "" {
				producer, tolerated = "Loom-Preflight", loomRowTolerated
			}

			report, err := CheckSeed(statusPath, statusLockPath, producer, tolerated)
			if err != nil {
				t.Fatalf("CheckSeed(...) error = %v; want nil (a determined verdict, not an infra error)", err)
			}
			if tt.wantOK {
				if !report.OK || len(report.Failures) != 0 {
					t.Errorf("CheckSeed(...) = %+v; want OK true, no failures", report)
				}
				return
			}
			if report.OK {
				t.Errorf("CheckSeed(...).OK = true; want false")
			}
			if !containsCheck(report.Failures, tt.wantCheck) {
				t.Errorf("CheckSeed(...).Failures = %+v; want to contain %q", report.Failures, tt.wantCheck)
			}
			if tt.wantReason != "" {
				found := false
				for _, f := range report.Failures {
					if f.Check == tt.wantCheck && strings.Contains(f.Reason, tt.wantReason) {
						found = true
					}
				}
				if !found {
					t.Errorf("CheckSeed(...).Failures = %+v; want a %q entry whose reason contains %q", report.Failures, tt.wantCheck, tt.wantReason)
				}
			}
			if tt.wantWayForward != "" {
				if len(report.Failures) != 1 || !strings.HasSuffix(report.Failures[0].Reason, tt.wantWayForward) {
					t.Errorf("CheckSeed(...).Failures = %+v; want exactly one failure ending in %q", report.Failures, tt.wantWayForward)
				}
			}
		})
	}
}
