// table_test.go covers Table.Validate against a seeded stencils directory, and the table's name, accessor and reserved-marker helpers.

package seatengine

import (
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"

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

// validTable returns a chair with two advisors whose outputs are the chair's inputs.
func validTable() Table {
	return Table{
		RolePrefix: "multi",
		Segment:    "plan",
		Seats: []Seat{
			{Name: RoleChair, Stencil: "seat-test-chair", Inputs: []string{"/w/a1.md", "/w/a2.md"}, Outputs: []string{"/w/out.md"}},
			{Name: AdvisorName(1), Stencil: "seat-test-advisor", Outputs: []string{"/w/a1.md"}},
			{Name: AdvisorName(2), Stencil: "seat-test-advisor", Outputs: []string{"/w/a2.md"}},
		},
	}
}

func TestTable_Validate(t *testing.T) {
	t.Parallel()
	dir := seatedStencils(t)
	writeStencil(t, dir, "seat-test-nested", "Nested.\n{{template \"seat-test-includer\"}}\n")
	writeStencil(t, dir, "seat-test-includer", "Block.\n{{template \"seat-directive-advisor\"}}\n")
	writeStencil(t, dir, "seat-test-broken", "{{template \"seat-test-absent\"}}\n")

	tests := []struct {
		name    string
		mutate  func(*Table)
		wantErr string
	}{
		{name: "an accepted table"},
		{name: "a lone chair is accepted", mutate: func(tb *Table) { tb.Seats = tb.Seats[:1] }},
		{name: "no chair", mutate: func(tb *Table) { tb.Seats = tb.Seats[1:] }, wantErr: `exactly one seat named "chair", found 0`},
		{name: "two chairs", mutate: func(tb *Table) { tb.Seats[1].Name = RoleChair }, wantErr: `exactly one seat named "chair", found 2`},
		{name: "advisors out of order", mutate: func(tb *Table) { tb.Seats[1].Name = AdvisorName(2) }, wantErr: `seat "advisor-2": want the name "advisor-1"`},
		{name: "a foreign seat name", mutate: func(tb *Table) { tb.Seats[2].Name = "critic" }, wantErr: `seat "critic": want the name "advisor-2"`},
		{name: "a seat with no output", mutate: func(tb *Table) { tb.Seats[0].Outputs = nil }, wantErr: `seat "chair": has no output`},
		{name: "two seats sharing an output", mutate: func(tb *Table) { tb.Seats[2].Outputs = []string{"/w/a1.md"} }, wantErr: `seat "advisor-2": output "/w/a1.md" is also seat "advisor-1"'s output`},
		{name: "an advisor output the chair does not read", mutate: func(tb *Table) { tb.Seats[0].Inputs = []string{"/w/a1.md"} }, wantErr: `seat "advisor-2": output "/w/a2.md" is not among the chair's inputs`},
		{name: "an invalid role prefix", mutate: func(tb *Table) { tb.RolePrefix = "Multi" }, wantErr: "role prefix"},
		{name: "an empty role prefix", mutate: func(tb *Table) { tb.RolePrefix = "" }, wantErr: "role prefix"},
		{name: "an unreadable seat stencil", mutate: func(tb *Table) { tb.Seats[1].Stencil = "seat-test-absent" }, wantErr: `seat "advisor-1": stencilstore: read stencil "seat-test-absent"`},
		{name: "an unreadable included block", mutate: func(tb *Table) { tb.Seats[0].Stencil = "seat-test-broken" }, wantErr: `seat "chair": stencilstore: read stencil "seat-test-absent"`},
		{name: "an included block that includes", mutate: func(tb *Table) { tb.Seats[0].Stencil = "seat-test-nested" }, wantErr: `seat "chair": include "seat-test-includer" declares an include of its own`},
		{name: "a table value naming a reserved marker", mutate: func(tb *Table) { tb.Values = map[string]string{"inputs": "x"} }, wantErr: `table: value "inputs" collides with a reserved marker`},
		{name: "a seat value naming a reserved marker", mutate: func(tb *Table) { tb.Seats[1].Values = map[string]string{"seat_name": "x"} }, wantErr: `seat "advisor-1": value "seat_name" collides with a reserved marker`},
		{name: "a whitespace-only table value", mutate: func(tb *Table) { tb.Values = map[string]string{"topic": " \n"} }, wantErr: `table: value "topic" is empty`},
		{name: "an empty seat value", mutate: func(tb *Table) { tb.Seats[0].Values = map[string]string{"topic": ""} }, wantErr: `seat "chair": value "topic" is empty`},
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

	if got, want := AdvisorName(3), "advisor-3"; got != want {
		t.Errorf("AdvisorName(3) = %q, want %q", got, want)
	}
	if got, want := SeatRole("multi", AdvisorName(2)), "multi-advisor-2"; got != want {
		t.Errorf("SeatRole(multi, advisor-2) = %q, want %q", got, want)
	}

	table := validTable()
	if got := table.Chair(); got.Name != RoleChair || !slices.Equal(got.Outputs, []string{"/w/out.md"}) {
		t.Errorf("Chair() = %+v, want the chair seat", got)
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
	for _, name := range []string{"parent_directive", "edit_directive", "seat_name", "chair_name", "advisor_names", "failed_advisors", "inputs", "outputs", "output_files", "worktree_root", "anchor_path", "stencils_dir"} {
		if !slices.Contains(reserved, name) {
			t.Errorf("ReservedMarkers() = %v, want it to hold %q", reserved, name)
		}
	}
	reserved[0] = "mutated"
	if ReservedMarkers()[0] == "mutated" {
		t.Error("ReservedMarkers() shares its backing list with the caller")
	}
}
