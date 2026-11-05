// entries_raddle_test.go covers raddleWriteEntry and raddleFileEntry: their construction-time validation of Env's raddle paths and seams, and the build over a filled Env.

package shedrecipe

import (
	"testing"

	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

func TestRaddleEntries(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	tests := []struct {
		name  string
		field string
		entry Constructor
		cfg   Config
		edit  func(*Env)
	}{
		{"WriteRelativeStatePath", "RaddleWrite.Deps.StatePath", raddleWriteEntry, nil, func(e *Env) { e.RaddleWrite.Deps.StatePath = "state.json" }},
		{"WriteNilAutoApprove", "RaddleWrite.Deps.AutoApprove", raddleWriteEntry, nil, func(e *Env) { e.RaddleWrite.Deps.AutoApprove = nil }},
		{"WriteNilFreshEntry", "RaddleWrite.Deps.FreshEntry", raddleWriteEntry, nil, func(e *Env) { e.RaddleWrite.Deps.FreshEntry = nil }},
		{"WriteNilCommit", "RaddleWrite.Deps.Commit", raddleWriteEntry, nil, func(e *Env) { e.RaddleWrite.Deps.Commit = nil }},
		{"WriteNilTable", "RaddleWrite.Table", raddleWriteEntry, nil, func(e *Env) { e.RaddleWrite.Table = nil }},
		{"WriteNilSeats", "Seats", raddleWriteEntry, nil, func(e *Env) { e.Seats = nil }},
		{"WriteGateNilBoard", "RaddleWrite.Check.Board", raddleWriteEntry, gatesCfg("raddle", 3), func(e *Env) { e.RaddleWrite.Check.Board = nil }},
		{"FileRelativeStageDir", "RaddleFile.StageDir", raddleFileEntry, nil, func(e *Env) { e.RaddleFile.StageDir = "stage" }},
		{"FileNilEntry", "RaddleFile.Entry", raddleFileEntry, nil, func(e *Env) { e.RaddleFile.Entry = nil }},
		{"FileNilAlreadyFiled", "RaddleFile.AlreadyFiled", raddleFileEntry, nil, func(e *Env) { e.RaddleFile.AlreadyFiled = nil }},
		{"FileNilCommit", "RaddleFile.Commit", raddleFileEntry, nil, func(e *Env) { e.RaddleFile.Commit = nil }},
		{"FileUnknownKey", "gates", raddleFileEntry, gatesCfg("raddle", 3), func(*Env) {}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			env := newTestEnv(t)
			tt.edit(&env)
			_, err := tt.entry(RowIdentity{Name: "Row"}, tt.cfg, env)
			if err == nil {
				t.Fatalf("entry() error = nil; want a refusal naming %s", tt.field)
			}
			assertErrContains(t, err, tt.field)
		})
	}

	builds := []struct {
		name  string
		entry Constructor
		cfg   Config
	}{
		{"WriteBuildsWithAFilledEnv", raddleWriteEntry, gatesCfg("raddle", 3)},
		{"FileBuildsWithAFilledEnv", raddleFileEntry, nil},
	}
	for _, tt := range builds {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			producer, err := tt.entry(RowIdentity{Name: "Row"}, tt.cfg, newTestEnv(t))
			if err != nil || producer == nil {
				t.Fatalf("entry() = %v, %v; want a producer and nil", producer, err)
			}
		})
	}
}
