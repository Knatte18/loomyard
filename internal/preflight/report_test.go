// report_test.go covers Report's own bookkeeping methods — AddFailure and Has — and the
// OK == (len(Failures) == 0) invariant, spawning nothing.

package preflight

import "testing"

func TestReport_AddFailure(t *testing.T) {
	var r Report
	r.OK = true

	r.AddFailure(CheckGeometry, "not inside a git repository")

	if len(r.Failures) != 1 {
		t.Fatalf("len(r.Failures) = %d; want 1", len(r.Failures))
	}
	got := r.Failures[0]
	want := Failure{Check: CheckGeometry, Reason: "not inside a git repository"}
	if got != want {
		t.Errorf("r.Failures[0] = %+v; want %+v", got, want)
	}
	if r.OK {
		t.Errorf("r.OK = true after AddFailure; want false")
	}
}

func TestReport_AddFailure_Multiple(t *testing.T) {
	var r Report

	r.AddFailure(CheckWorktreeClean, "dirty")
	r.AddFailure(CheckFabricReady, "not ready")

	if len(r.Failures) != 2 {
		t.Fatalf("len(r.Failures) = %d; want 2", len(r.Failures))
	}
	if r.OK {
		t.Errorf("r.OK = true after two AddFailure calls; want false")
	}
	if r.Failures[0].Check != CheckWorktreeClean || r.Failures[1].Check != CheckFabricReady {
		t.Errorf("r.Failures = %+v; want CheckWorktreeClean then CheckFabricReady", r.Failures)
	}
}

func TestReport_Has(t *testing.T) {
	tests := []struct {
		name  string
		r     Report
		check CheckID
		want  bool
	}{
		{
			name:  "ZeroValueReport",
			r:     Report{},
			check: CheckGeometry,
			want:  false,
		},
		{
			name:  "RecordedCheckID",
			r:     Report{Failures: []Failure{{Check: CheckJunction, Reason: "broken"}}},
			check: CheckJunction,
			want:  true,
		},
		{
			name:  "UnrecordedCheckID",
			r:     Report{Failures: []Failure{{Check: CheckJunction, Reason: "broken"}}},
			check: CheckFabricSync,
			want:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.r.Has(tt.check)
			if got != tt.want {
				t.Errorf("Report.Has(%q) = %v; want %v", tt.check, got, tt.want)
			}
		})
	}
}

// TestReport_OKFailuresInvariant checks the OK == (len(Failures) == 0) invariant on a Report shaped
// the way a check's return value is shaped -- OK explicitly set true before any AddFailure call --
// rather than on the bare zero value, whose OK defaults to false with an empty Failures slice and so
// does not itself satisfy the invariant; that shape is never what a check returns.
func TestReport_OKFailuresInvariant(t *testing.T) {
	r := Report{OK: true}
	if !(r.OK == (len(r.Failures) == 0)) {
		t.Errorf("clean-pass Report violates OK == (len(Failures) == 0): %+v", r)
	}

	r.AddFailure(CheckGeometry, "reason")
	if !(r.OK == (len(r.Failures) == 0)) {
		t.Errorf("after AddFailure, Report violates OK == (len(Failures) == 0): %+v", r)
	}
}
