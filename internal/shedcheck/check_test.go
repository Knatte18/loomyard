// check_test.go table-drives Check across every finding kind, its deterministic output order, and
// the tolerated-malformed-input behaviours. Every case builds bare []shedengine.ProducerDef
// literals with Producer left nil -- no fakes of any kind.

package shedcheck

import (
	"testing"

	"github.com/Knatte18/loomyard/internal/shedengine"
)

// wantFinding is the Message-free projection every case's want table compares against: Kind,
// Producer, and Target are the pinned contract, and Message is asserted non-empty separately
// rather than compared for wording.
type wantFinding struct {
	Kind     Kind
	Producer string
	Target   string
}

func assertFindings(t *testing.T, got []Finding, want []wantFinding) {
	t.Helper()
	if len(want) == 0 {
		if got != nil {
			t.Fatalf("Check() = %v; want nil", got)
		}
		return
	}
	if len(got) != len(want) {
		t.Fatalf("Check() returned %d findings; want %d\ngot:  %v\nwant: %v", len(got), len(want), got, want)
	}
	for i := range want {
		if got[i].Kind != want[i].Kind || got[i].Producer != want[i].Producer || got[i].Target != want[i].Target {
			t.Errorf("finding %d = {Kind:%q Producer:%q Target:%q}; want {Kind:%q Producer:%q Target:%q}",
				i, got[i].Kind, got[i].Producer, got[i].Target, want[i].Kind, want[i].Producer, want[i].Target)
		}
		if got[i].Message == "" {
			t.Errorf("finding %d Message is empty; want non-empty", i)
		}
	}
}

func TestCheck(t *testing.T) {
	// defectProducers carries one instance of every non-endpoint finding kind at once: a dangling
	// edge, an unreachable row, an unexpected silent terminal, a done cycle, and a blind gate.
	// Reused across the endpoint-suppression cases below, so those cases prove the endpoint kinds
	// are returned alone even against a graph that would otherwise fail every other check too.
	defectProducers := []shedengine.ProducerDef{
		{Name: "E", OnDone: "Gate"},
		{Name: "Gate", OnDone: "Term", OnStuck: "Bystander"},
		{Name: "Term", OnDone: "UT"},
		{Name: "UT"},
		{Name: "Bystander", OnDone: "CycA"},
		{Name: "CycA", OnDone: "CycB"},
		{Name: "CycB", OnDone: "CycA"},
		{Name: "Dangler", OnDone: "ghost"},
	}

	tests := []struct {
		name      string
		producers []shedengine.ProducerDef
		entry     string
		terminals []string
		want      []wantFinding
	}{
		{
			name: "clean linear graph ending at a told terminal",
			producers: []shedengine.ProducerDef{
				{Name: "A", OnDone: "B"},
				{Name: "B", OnDone: "C"},
				{Name: "C"},
			},
			entry:     "A",
			terminals: []string{"C"},
			want:      nil,
		},
		{
			name: "clean graph with a legitimate backwards bounce that routes forward again",
			producers: []shedengine.ProducerDef{
				{Name: "A", OnDone: "B"},
				{Name: "B", OnDone: "C", OnStuck: "A"},
				{Name: "C"},
			},
			entry:     "A",
			terminals: []string{"C"},
			want:      nil,
		},
		{
			name: "gate-return path exists only through a stuck edge (perch shape)",
			producers: []shedengine.ProducerDef{
				{Name: "G", OnDone: "Z", OnStuck: "T"},
				{Name: "T", OnDone: "Z", OnStuck: "G"},
				{Name: "Z"},
			},
			entry:     "G",
			terminals: []string{"Z"},
			want:      nil,
		},
		{
			name: "self OnStuck",
			producers: []shedengine.ProducerDef{
				{Name: "A", OnStuck: "A"},
			},
			entry:     "A",
			terminals: []string{"A"},
			want:      nil,
		},
		{
			name: "OnStuck empty on an ordinary row, OnDone empty on a told terminal",
			producers: []shedengine.ProducerDef{
				{Name: "A", OnDone: "B"},
				{Name: "B"},
			},
			entry:     "A",
			terminals: []string{"B"},
			want:      nil,
		},
		{
			name:      "empty entry",
			producers: defectProducers,
			entry:     "",
			terminals: []string{"Term"},
			want:      []wantFinding{{KindBadEntry, "", ""}},
		},
		{
			name:      "entry naming no row",
			producers: defectProducers,
			entry:     "ghost-entry",
			terminals: []string{"Term"},
			want:      []wantFinding{{KindBadEntry, "ghost-entry", ""}},
		},
		{
			name:      "nil terminals",
			producers: defectProducers,
			entry:     "E",
			terminals: nil,
			want:      []wantFinding{{KindNoTerminals, "", ""}},
		},
		{
			name:      "empty terminals",
			producers: defectProducers,
			entry:     "E",
			terminals: []string{},
			want:      []wantFinding{{KindNoTerminals, "", ""}},
		},
		{
			name:      "a terminal naming no row, and an empty terminal name",
			producers: defectProducers,
			entry:     "E",
			terminals: []string{"missing", "", "Term"},
			want: []wantFinding{
				{KindBadTerminal, "missing", ""},
				{KindBadTerminal, "", ""},
			},
		},
		{
			name:      "empty entry together with an empty terminal name",
			producers: defectProducers,
			entry:     "",
			terminals: []string{""},
			want: []wantFinding{
				{KindBadEntry, "", ""},
				{KindBadTerminal, "", ""},
			},
		},
		{
			name: "OnDone and OnStuck on one row each naming no row",
			producers: []shedengine.ProducerDef{
				{Name: "A", OnDone: "ghost1", OnStuck: "ghost2"},
			},
			entry:     "A",
			terminals: []string{"A"},
			want: []wantFinding{
				{KindDanglingTarget, "A", "ghost1"},
				{KindDanglingTarget, "A", "ghost2"},
			},
		},
		{
			name: "reachable row with a dangling OnDone is not also unexpected-terminal",
			producers: []shedengine.ProducerDef{
				{Name: "A", OnDone: "ghost"},
			},
			entry:     "A",
			terminals: []string{"A"},
			want:      []wantFinding{{KindDanglingTarget, "A", "ghost"}},
		},
		{
			name: "reachable row with a dangling OnStuck is not also blind-gate",
			producers: []shedengine.ProducerDef{
				{Name: "A", OnStuck: "ghost"},
			},
			entry:     "A",
			terminals: []string{"A"},
			want:      []wantFinding{{KindDanglingTarget, "A", "ghost"}},
		},
		{
			name: "a row reachable only through a stuck edge is not unreachable",
			producers: []shedengine.ProducerDef{
				{Name: "A", OnStuck: "B"},
				{Name: "B", OnDone: "A"},
			},
			entry:     "A",
			terminals: []string{"A"},
			want:      nil,
		},
		{
			name: "a row reachable through nothing, and an unreachable told terminal",
			producers: []shedengine.ProducerDef{
				{Name: "E"},
				{Name: "Ghost"},
			},
			entry:     "E",
			terminals: []string{"E", "Ghost"},
			want:      []wantFinding{{KindUnreachable, "Ghost", ""}},
		},
		{
			name: "reachable non-terminal row with empty OnDone, unreachable row with empty OnDone",
			producers: []shedengine.ProducerDef{
				{Name: "E", OnDone: "A"},
				{Name: "A"},
				{Name: "C"},
			},
			entry:     "E",
			terminals: []string{"E"},
			want: []wantFinding{
				{KindUnreachable, "C", ""},
				{KindUnexpectedTerminal, "A", ""},
			},
		},
		{
			name: "a told terminal with a non-empty OnDone is not a finding",
			producers: []shedengine.ProducerDef{
				{Name: "T", OnDone: "Next"},
				{Name: "Next"},
			},
			entry:     "T",
			terminals: []string{"T", "Next"},
			want:      nil,
		},
		{
			name: "two-row done cycle",
			producers: []shedengine.ProducerDef{
				{Name: "E", OnDone: "P1"},
				{Name: "P1", OnDone: "P2"},
				{Name: "P2", OnDone: "P1"},
			},
			entry:     "E",
			terminals: []string{"E"},
			want: []wantFinding{
				{KindDoneCycle, "P1", "P2"},
				{KindDoneCycle, "P2", "P1"},
			},
		},
		{
			name: "three-row done cycle",
			producers: []shedengine.ProducerDef{
				{Name: "E", OnDone: "Q1"},
				{Name: "Q1", OnDone: "Q2"},
				{Name: "Q2", OnDone: "Q3"},
				{Name: "Q3", OnDone: "Q1"},
			},
			entry:     "E",
			terminals: []string{"E"},
			want: []wantFinding{
				{KindDoneCycle, "Q1", "Q2"},
				{KindDoneCycle, "Q2", "Q3"},
				{KindDoneCycle, "Q3", "Q1"},
			},
		},
		{
			name: "one-row done cycle",
			producers: []shedengine.ProducerDef{
				{Name: "E", OnDone: "S"},
				{Name: "S", OnDone: "S"},
			},
			entry:     "E",
			terminals: []string{"E"},
			want:      []wantFinding{{KindDoneCycle, "S", "S"}},
		},
		{
			name: "two disjoint done cycles, lower-index cycle discovered second by a naive walk",
			producers: []shedengine.ProducerDef{
				{Name: "E", OnDone: "B1", OnStuck: "A1"},
				{Name: "A1", OnDone: "A2"},
				{Name: "A2", OnDone: "A1", OnStuck: "E"},
				{Name: "B1", OnDone: "B2"},
				{Name: "B2", OnDone: "B1"},
			},
			entry:     "E",
			terminals: []string{"E"},
			want: []wantFinding{
				{KindDoneCycle, "A1", "A2"},
				{KindDoneCycle, "A2", "A1"},
				{KindDoneCycle, "B1", "B2"},
				{KindDoneCycle, "B2", "B1"},
			},
		},
		{
			name: "a blind gate: G bounces to T, T routes forward past G and never back",
			producers: []shedengine.ProducerDef{
				{Name: "G", OnDone: "X", OnStuck: "T"},
				{Name: "T", OnDone: "Y"},
				{Name: "X"},
				{Name: "Y"},
			},
			entry:     "G",
			terminals: []string{"X", "Y"},
			want:      []wantFinding{{KindBlindGate, "G", "T"}},
		},
		{
			name:      "nil producers",
			producers: nil,
			entry:     "E",
			terminals: []string{"T"},
			want: []wantFinding{
				{KindBadEntry, "E", ""},
				{KindBadTerminal, "T", ""},
			},
		},
		{
			name:      "empty producers",
			producers: []shedengine.ProducerDef{},
			entry:     "E",
			terminals: []string{"T"},
			want: []wantFinding{
				{KindBadEntry, "E", ""},
				{KindBadTerminal, "T", ""},
			},
		},
		{
			name: "a row with an empty Name",
			producers: []shedengine.ProducerDef{
				{Name: ""},
				{Name: "Real"},
			},
			entry:     "Real",
			terminals: []string{"Real"},
			want:      []wantFinding{{KindUnreachable, "", ""}},
		},
		{
			name: "duplicate Name: the shadowed later row comes back as unreachable only",
			producers: []shedengine.ProducerDef{
				{Name: "Dup"},
				{Name: "Dup", OnDone: "ghost", OnStuck: "ghost2"},
			},
			entry:     "Dup",
			terminals: []string{"Dup"},
			want:      []wantFinding{{KindUnreachable, "Dup", ""}},
		},
		{
			name:      "one graph carrying several kinds at once, in the fixed check order",
			producers: defectProducers,
			entry:     "E",
			terminals: []string{"Term"},
			want: []wantFinding{
				{KindDanglingTarget, "Dangler", "ghost"},
				{KindUnreachable, "Dangler", ""},
				{KindUnexpectedTerminal, "UT", ""},
				{KindDoneCycle, "CycA", "CycB"},
				{KindDoneCycle, "CycB", "CycA"},
				{KindBlindGate, "Gate", "Bystander"},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Check(tt.producers, tt.entry, tt.terminals)
			assertFindings(t, got, tt.want)
		})
	}
}
