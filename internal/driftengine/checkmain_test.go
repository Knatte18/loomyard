//go:build tier0

// checkmain_test.go — tier 0 tests for CheckMainCapture's admitted rows and each refusal's way forward.

package driftengine

import (
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/shedengine"
)

// TestCheckMainCapture covers each admitted row and state, a blocked status at another row, a moved HEAD, and a dirty tree at Drift-Baseline and at a Drift-Await halt.
func TestCheckMainCapture(t *testing.T) {
	t.Parallel()

	status := func(row string, state shedengine.State, reason string) shedengine.Status {
		return shedengine.Status{CurrentProducer: row, State: state, Error: reason}
	}
	tests := []struct {
		name    string
		status  shedengine.Status
		head    string
		dirty   []string
		want    []string
		wantNot []string
	}{
		{name: "running at Drift-Baseline", status: status("Drift-Baseline", shedengine.StateRunning, ""), head: "bp"},
		{name: "a capture halt at Drift-Baseline", status: status("Drift-Baseline", shedengine.StateBlocked, CaptureHaltPrefix+"exit 2"), head: "bp"},
		{name: "a halt at Drift-Await", status: status("Drift-Await", shedengine.StateBlocked, "waiting"), head: "bp"},
		{
			name: "a blocked status at Drift-Baseline that is no capture halt", status: status("Drift-Baseline", shedengine.StateBlocked, "decision needed"), head: "bp",
			want: []string{"drift capture refused:", "other than a capture halt", "lyx loom resume"},
		},
		{
			name: "running at Drift-Await", status: status("Drift-Await", shedengine.StateRunning, ""), head: "bp",
			want: []string{"drift capture refused:", "Drift-Await (running)", "run lyx drift capture at a Drift-Await halt or at a Drift-Baseline capture halt"},
		},
		{
			name: "blocked at another row", status: status("Code-Review", shedengine.StateBlocked, "x"), head: "bp",
			want: []string{"drift capture refused:", "Code-Review (blocked)"},
		},
		{
			name: "a moved HEAD", status: status("Drift-Baseline", shedengine.StateRunning, ""), head: "later",
			want: []string{"drift capture refused:", "HEAD later", "branch point bp", "lyx drift defer, then lyx loom resume"},
		},
		{
			name: "a dirty tree at Drift-Baseline", status: status("Drift-Baseline", shedengine.StateRunning, ""), head: "bp", dirty: []string{"a.go", "b.go"},
			want:    []string{"drift capture refused:", "a.go, b.go", "restore those paths to HEAD or stash them", "lyx loom resume, which captures again", "lyx drift defer, then lyx loom resume"},
			wantNot: []string{"lyx drift capture again", "remove"},
		},
		{
			name: "a dirty tree at a Drift-Await halt", status: status("Drift-Await", shedengine.StateBlocked, "waiting"), head: "bp", dirty: []string{"a.go"},
			want:    []string{"drift capture refused:", "restore those paths to HEAD or stash them", "lyx drift capture again, then lyx loom resume", "lyx drift defer, then lyx loom resume"},
			wantNot: []string{"captures again", "remove"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			err := CheckMainCapture(tt.status, tt.head, "bp", tt.dirty)
			if tt.want == nil {
				if err != nil {
					t.Fatalf("CheckMainCapture() = %v; want nil", err)
				}
				return
			}
			if err == nil {
				t.Fatalf("CheckMainCapture() = nil; want an error containing %q", tt.want)
			}
			for _, want := range tt.want {
				if !strings.Contains(err.Error(), want) {
					t.Errorf("CheckMainCapture() error = %q; want it to contain %q", err.Error(), want)
				}
			}
			for _, not := range tt.wantNot {
				if strings.Contains(err.Error(), not) {
					t.Errorf("CheckMainCapture() error = %q; want it not to contain %q", err.Error(), not)
				}
			}
		})
	}
}
