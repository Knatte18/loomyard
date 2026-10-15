// entries_describe_test.go covers describeEntry: its construction-time validation over Config, the
// three injected Env seams and the told DescriptionPath, plus the happy path that runs one table through the seat runner.

package shedrecipe

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/seatengine"
	"github.com/Knatte18/loomyard/internal/segmentcolor"
	"github.com/Knatte18/loomyard/internal/shedengine"
	"github.com/Knatte18/loomyard/internal/testkit/shedfake"
)

// newDescribeTestEnv returns newTestEnv's Env with the Describe fields filled.
func newDescribeTestEnv(t *testing.T) Env {
	t.Helper()
	env := newTestEnv(t)
	dir := t.TempDir()
	env.DescriptionPath = filepath.Join(dir, "summary.md")
	env.DescribeTable = func() (seatengine.Table, error) {
		return seatengine.Table{
			RolePrefix: "describe",
			Segment:    segmentcolor.Describe,
			Seats:      []seatengine.Seat{{Stencil: "landing-template-describe", Outputs: []string{env.DescriptionPath}}},
		}, nil
	}
	env.CommitDescription = func() error { return nil }
	return env
}

func TestDescribeEntry_ConstructionFailures(t *testing.T) {
	gated := gatesCfg("description", 3)
	cases := []struct {
		name   string
		mutate func(*Env)
		cfg    Config
		want   string
	}{
		{"NilDescribeTable", func(e *Env) { e.DescribeTable = nil }, gated, "DescribeTable"},
		{"NilCommitDescription", func(e *Env) { e.CommitDescription = nil }, gated, "CommitDescription"},
		{"NilSeats", func(e *Env) { e.Seats = nil }, gated, "Seats"},
		{"MissingDescriptionPath", func(e *Env) { e.DescriptionPath = "" }, gated, "DescriptionPath"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			env := newDescribeTestEnv(t)
			tc.mutate(&env)
			_, err := describeEntry("Row", tc.cfg, env)
			if err == nil {
				t.Fatalf("describeEntry() error = nil; want non-nil")
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Errorf("describeEntry() error = %v; want it to name %q", err, tc.want)
			}
		})
	}
}

// TestDescribeEntry_HappyPath asserts a fully-filled Env constructs a producer, whose one Call runs the told table through the seat runner under the row's resolved description gate and returns the lead's output.
func TestDescribeEntry_HappyPath(t *testing.T) {
	env := newDescribeTestEnv(t)
	fake := &shedfake.SeatRunner{Results: []seatengine.Result{leadDoneResult(env.DescriptionPath)}}
	env.Seats = fake
	p, err := describeEntry("Row", gatesCfg("description", 2), env)
	if err != nil {
		t.Fatalf("describeEntry() error = %v; want nil", err)
	}
	if p == nil {
		t.Fatalf("describeEntry() = nil producer; want non-nil")
	}

	pointer := shedfake.RequireOutcome(t, p, shedengine.Done)

	if len(fake.GotTables) != 1 {
		t.Fatalf("seat runner saw %d tables; want 1", len(fake.GotTables))
	}
	gate := fake.GotTables[0].Gate
	if len(gate) != 1 || gate[0].Name != "description" || gate[0].Attempts != 2 {
		t.Errorf("table gate = %+v; want one \"description\" entry with 2 attempts", gate)
	}
	if pointer.Path != env.DescriptionPath {
		t.Errorf("Call() OutputPointer.Path = %q; want the lead's output %q", pointer.Path, env.DescriptionPath)
	}
}
