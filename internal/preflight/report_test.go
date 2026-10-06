// report_test.go covers Report's own bookkeeping methods — AddFailure and Has — and the
// OK == (len(Failures) == 0) invariant, spawning nothing.

package preflight

import "testing"

// TestReport_AddFailure covers AddFailure on a Report shaped the way a check's return value is shaped -- OK explicitly set true before any AddFailure call, rather than the bare zero value whose OK defaults to false with an empty Failures slice and so does not itself satisfy the invariant: each recorded failure is appended in order with its check and reason, and the first one flips OK to false, so OK == (len(Failures) == 0) holds before and after.
//
//testtiming:keep pins AddFailure's append order, failure fields and OK flip, which the CheckResolved table covering its blocks only reads back through the CheckID set
func TestReport_AddFailure(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		adds []Failure
	}{
		{name: "no failure keeps OK"},
		{name: "one failure", adds: []Failure{{Check: CheckGeometry, Reason: "not inside a git repository"}}},
		{name: "multiple failures keep their order", adds: []Failure{{Check: CheckWorktreeClean, Reason: "dirty"}, {Check: CheckFabricReady, Reason: "not ready"}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			r := Report{OK: true}
			for _, f := range tt.adds {
				r.AddFailure(f.Check, f.Reason)
			}

			if len(r.Failures) != len(tt.adds) {
				t.Fatalf("len(r.Failures) = %d; want %d", len(r.Failures), len(tt.adds))
			}
			for i, want := range tt.adds {
				if r.Failures[i] != want {
					t.Errorf("r.Failures[%d] = %+v; want %+v", i, r.Failures[i], want)
				}
			}
			if wantOK := len(tt.adds) == 0; r.OK != wantOK {
				t.Errorf("r.OK = %v; want %v", r.OK, wantOK)
			}
			if r.OK != (len(r.Failures) == 0) {
				t.Errorf("Report violates OK == (len(Failures) == 0): %+v", r)
			}
		})
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
