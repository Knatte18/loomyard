// table_test.go covers Table.Validate against a seeded stencils directory, and the table's name, accessor and reserved-marker helpers.

package seatengine

import (
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/shuttleengine"
	"github.com/Knatte18/loomyard/internal/stencilstore"
	"github.com/Knatte18/loomyard/internal/testkit/stencilkit"
)

const (
	chairStencilBody   = "Chair of {{.seat_name}}.\n{{template \"seat-directive-chair\"}}\n"
	advisorStencilBody = "Advisor {{.seat_name}}.\n{{template \"seat-directive-advisor\"}}\n"
)

// writeStencil writes body as the stencil name under dir, in the family folder stencilstore reads it from.
func writeStencil(t *testing.T, dir, name, body string) {
	t.Helper()
	path := stencilstore.Path(dir, name)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("create stencil dir for %s: %v", name, err)
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatalf("write stencil %s: %v", name, err)
	}
}

// seatedStencils returns a stencils directory holding the shipped seat blocks plus a chair and an advisor stencil.
func seatedStencils(t *testing.T) string {
	t.Helper()
	dir := stencilkit.Seed(t)
	writeStencil(t, dir, "seat-test-chair", chairStencilBody)
	writeStencil(t, dir, "seat-test-advisor", advisorStencilBody)
	return dir
}

// validTable returns an unnamed lead with two named advisors whose outputs are the lead's inputs.
func validTable() Table {
	return Table{
		RolePrefix: "multi",
		Segment:    "plan",
		Seats: []Seat{
			{Stencil: "seat-test-chair", Inputs: []string{"/w/a1.md", "/w/a2.md"}, Outputs: []string{"/w/out.md"}},
			{Name: advisorName(1), Stencil: "seat-test-advisor", Outputs: []string{"/w/a1.md"}},
			{Name: advisorName(2), Stencil: "seat-test-advisor", Outputs: []string{"/w/a2.md"}},
		},
	}
}

func TestTable_Validate(t *testing.T) {
	t.Parallel()
	dir := seatedStencils(t)
	writeStencil(t, dir, "seat-test-nested", "Nested.\n{{template \"seat-test-includer\"}}\n")
	writeStencil(t, dir, "seat-test-includer", "Block.\n{{template \"seat-directive-advisor\"}}\n")
	writeStencil(t, dir, "seat-test-broken", "{{template \"seat-test-absent\"}}\n")
	writeStencil(t, dir, "seat-test-bare", "Lead {{.seat_name}}.\n")

	tests := []struct {
		name    string
		mutate  func(*Table)
		wantErr string
	}{
		{name: "an accepted table"},
		{name: "a gate on the lead", mutate: func(tb *Table) { tb.Seats[0].Gate = shuttleengine.GateSpec{{Name: "g", Attempts: 1}} }, wantErr: `the lead carries a gate`},
		{name: "a required lead", mutate: func(tb *Table) { tb.Seats[0].Required = true }, wantErr: `the lead is marked required`},
		{name: "an accept hook on the lead", mutate: func(tb *Table) { tb.Seats[0].Accept = func(shuttleengine.Result) error { return nil } }, wantErr: `the lead carries an accept hook`},
		{name: "an accept hook on an advisor that is not required", mutate: func(tb *Table) { tb.Seats[1].Accept = func(shuttleengine.Result) error { return nil } }, wantErr: `seat "advisor-1": carries an accept hook but is not required`},
		{name: "a required advisor with an accept hook", mutate: func(tb *Table) {
			tb.Seats[1].Required = true
			tb.Seats[1].Accept = func(shuttleengine.Result) error { return nil }
		}},
		{name: "an advisor gate and fork authorization on any seat", mutate: func(tb *Table) {
			tb.Seats[1].Gate = shuttleengine.GateSpec{{Name: "g", Attempts: 1}}
			tb.Seats[0].ForkSubagents, tb.Seats[1].ForkSubagents = true, true
			tb.Round = "r1"
		}},
		{name: "a lone lead is accepted", mutate: func(tb *Table) { tb.Seats = tb.Seats[:1] }},
		{name: "a lone lead needs no chair directive", mutate: func(tb *Table) {
			tb.Seats = tb.Seats[:1]
			tb.Seats[0].Stencil = "seat-test-bare"
		}},
		{name: "a table with no seat", mutate: func(tb *Table) { tb.Seats = nil }, wantErr: `the table has no seat; give it a lead`},
		{name: "a named lead", mutate: func(tb *Table) { tb.Seats[0].Name = "critic" }, wantErr: `seat "critic": the first seat is the lead and carries no name; drop its name`},
		{name: "an unnamed later seat", mutate: func(tb *Table) { tb.Seats[1].Name = "" }, wantErr: `give the seat a unique name that starts with a letter`},
		{name: "a duplicate later name", mutate: func(tb *Table) { tb.Seats[2].Name = tb.Seats[1].Name }, wantErr: `seat "advisor-1": the name is already taken; give the seat a unique name that starts with a letter`},
		{name: "a numeric later name", mutate: func(tb *Table) { tb.Seats[1].Name = "2" }, wantErr: `seat "2": `},
		{name: "a later name with an upper-case letter", mutate: func(tb *Table) { tb.Seats[1].Name = "Critic" }, wantErr: `seat "Critic": `},
		{name: "a lead stencil without the chair directive beside advisors", mutate: func(tb *Table) { tb.Seats[0].Stencil = "seat-test-bare" }, wantErr: `include seat-directive-chair in the lead's stencil`},
		{name: "a seat with no output", mutate: func(tb *Table) { tb.Seats[0].Outputs = nil }, wantErr: `seat "": has no output`},
		{name: "two seats sharing an output", mutate: func(tb *Table) { tb.Seats[2].Outputs = []string{"/w/a1.md"} }, wantErr: `seat "advisor-2": output "/w/a1.md" is also seat "advisor-1"'s output`},
		{name: "an advisor output the lead does not read", mutate: func(tb *Table) { tb.Seats[0].Inputs = []string{"/w/a1.md"} }, wantErr: `seat "advisor-2": output "/w/a2.md" is not among the lead's inputs`},
		{name: "an invalid role prefix", mutate: func(tb *Table) { tb.RolePrefix = "Multi" }, wantErr: "role prefix"},
		{name: "an empty role prefix", mutate: func(tb *Table) { tb.RolePrefix = "" }, wantErr: "role prefix"},
		{name: "an unreadable seat stencil", mutate: func(tb *Table) { tb.Seats[1].Stencil = "seat-test-absent" }, wantErr: `seat "advisor-1": stencilstore: read stencil "seat-test-absent"`},
		{name: "an unreadable included block", mutate: func(tb *Table) { tb.Seats[0].Stencil = "seat-test-broken" }, wantErr: `seat "": stencilstore: read stencil "seat-test-absent"`},
		{name: "an included block that includes", mutate: func(tb *Table) { tb.Seats[0].Stencil = "seat-test-nested" }, wantErr: `seat "": include "seat-test-includer" declares an include of its own`},
		{name: "a table value naming a reserved marker", mutate: func(tb *Table) { tb.Values = map[string]string{"inputs": "x"} }, wantErr: `table: value "inputs" collides with a reserved marker`},
		{name: "a seat value naming a reserved marker", mutate: func(tb *Table) { tb.Seats[1].Values = map[string]string{"seat_name": "x"} }, wantErr: `seat "advisor-1": value "seat_name" collides with a reserved marker`},
		{name: "a whitespace-only table value", mutate: func(tb *Table) { tb.Values = map[string]string{"topic": " \n"} }, wantErr: `table: value "topic" is empty`},
		{name: "an empty seat value", mutate: func(tb *Table) { tb.Seats[0].Values = map[string]string{"topic": ""} }, wantErr: `seat "": value "topic" is empty`},
		{name: "a blank table value named optional", mutate: func(tb *Table) {
			tb.Values = map[string]string{"topic": " "}
			tb.Optional = []string{"topic"}
		}},
		{name: "a blank table value beside an optional name", mutate: func(tb *Table) {
			tb.Values = map[string]string{"topic": "", "focus": "x"}
			tb.Optional = []string{"focus"}
		}, wantErr: `table: value "topic" is empty`},
		{name: "an optional name naming a reserved marker", mutate: func(tb *Table) { tb.Optional = []string{"inputs"} }, wantErr: `table: optional value "inputs" collides with a reserved marker`},
		{name: "a blank seat value whose name is optional", mutate: func(tb *Table) {
			tb.Seats[0].Values = map[string]string{"topic": ""}
			tb.Optional = []string{"topic"}
		}, wantErr: `seat "": value "topic" is empty`},
		{name: "unreserved non-blank values", mutate: func(tb *Table) {
			tb.Values = map[string]string{"topic": "x"}
			tb.Seats[0].Values = map[string]string{"focus": "y"}
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			table := validTable()
			if tt.mutate != nil {
				tt.mutate(&table)
			}
			err := table.Validate(dir)
			if tt.wantErr == "" {
				if err != nil {
					t.Fatalf("Validate() = %v, want nil", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("Validate() = %v, want an error containing %q", err, tt.wantErr)
			}
		})
	}
}

func TestTable_NamesAccessorsAndReservedMarkers(t *testing.T) {
	t.Parallel()

	if got, want := SeatRole("multi", ""), "multi"; got != want {
		t.Errorf("SeatRole(multi, \"\") = %q, want the prefix alone, %q", got, want)
	}
	if got, want := SeatRole("multi", "critic"), "multi-critic"; got != want {
		t.Errorf("SeatRole(multi, critic) = %q, want %q", got, want)
	}

	table := validTable()
	if got := table.Chair(); got.Name != "" || !slices.Equal(got.Outputs, []string{"/w/out.md"}) {
		t.Errorf("Chair() = %+v, want the first seat", got)
	}
	if got := (Table{}).Chair(); !reflect.DeepEqual(got, Seat{}) {
		t.Errorf("Chair() of an empty table = %+v, want the zero seat", got)
	}
	if got, want := table.Outputs(), []string{"/w/out.md", "/w/a1.md", "/w/a2.md"}; !slices.Equal(got, want) {
		t.Errorf("Outputs() = %v, want %v in table order", got, want)
	}

	reserved := ReservedMarkers()
	if !slices.IsSorted(reserved) {
		t.Errorf("ReservedMarkers() = %v, want a sorted list", reserved)
	}
	for _, name := range []string{"parent_directive", "edit_directive", "seat_name", "chair_name", "advisor_names", "inputs", "outputs", "output_files", "worktree_root", "anchor_path", "stencils_dir"} {
		if !slices.Contains(reserved, name) {
			t.Errorf("ReservedMarkers() = %v, want it to hold %q", reserved, name)
		}
	}
	reserved[0] = "mutated"
	if ReservedMarkers()[0] == "mutated" {
		t.Error("ReservedMarkers() shares its backing list with the caller")
	}
}
