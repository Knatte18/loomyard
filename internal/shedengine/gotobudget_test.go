//go:build tier0

package shedengine

import "testing"

func gotoTestProducers() []ProducerDef {
	return []ProducerDef{
		{Name: "A1", Segment: "Seg"},
		{Name: "A2", Segment: "Seg"},
		{Name: "B", Segment: "Other"},
		{Name: "Solo"},
	}
}

func stuckHistory(names ...string) []HistoryEntry {
	var h []HistoryEntry
	for _, n := range names {
		h = append(h, HistoryEntry{Producer: n, Outcome: Stuck})
	}
	return h
}

// historyOf concatenates entry groups into one history, so a table row reads as the sequence it pins.
func historyOf(groups ...[]HistoryEntry) []HistoryEntry {
	var history []HistoryEntry
	for _, group := range groups {
		history = append(history, group...)
	}
	return history
}

// TestEpisodeStuckCount_Boundaries pins what bounds a row's stuck episode:
// a goto keeps every count, a reroute ends its target segment's episode, a budget-capped entry ends the scan unless a goto directly follows it,
// and a generation-start entry is counted and ends the scan right after itself.
func TestEpisodeStuckCount_Boundaries(t *testing.T) {
	t.Parallel()
	ps := gotoTestProducers()
	gotoTo := func(producer string) []HistoryEntry {
		return []HistoryEntry{{Producer: producer, Outcome: OutcomeGoto}}
	}
	rerouteTo := func(from, target string) []HistoryEntry {
		return []HistoryEntry{{Producer: from, Outcome: OutcomeReroute, RoutedTo: target}}
	}
	capped := func(producer string) []HistoryEntry {
		return []HistoryEntry{{Producer: producer, Outcome: Stuck, BudgetCapped: true}}
	}
	generationStart := func(producer string) []HistoryEntry {
		return []HistoryEntry{{Producer: producer, Outcome: Stuck, GenerationStart: true}}
	}
	tests := []struct {
		name    string
		history []HistoryEntry
		// want maps a producer's index in gotoTestProducers to its expected stuck count.
		want map[int]int
	}{
		{"goto into a segment keeps every row's count", historyOf(stuckHistory("A1", "A2", "A1", "A2"), gotoTo("A1")), map[int]int{0: 2, 1: 2}},
		{"goto an unsegmented row keeps its count", historyOf(stuckHistory("Solo", "A1"), gotoTo("Solo")), map[int]int{3: 1, 0: 1}},
		{"goto an absent target keeps every count", historyOf(stuckHistory("A1", "Solo"), gotoTo("Gone")), map[int]int{0: 1, 3: 1}},
		{"stuck after a goto adds to the spent count", historyOf(stuckHistory("A1", "A2"), gotoTo("A2"), stuckHistory("A1")), map[int]int{0: 2, 1: 1}},
		{"reroute into a segment ends it", historyOf(stuckHistory("A1", "A2", "A1"), rerouteTo("Solo", "A1")), map[int]int{0: 0, 1: 0}},
		{"reroute into another segment ends neither", historyOf(stuckHistory("A1", "A2"), rerouteTo("Solo", "B")), map[int]int{0: 1, 1: 1}},
		{"reroute onto an unsegmented row ends only that row", historyOf(stuckHistory("Solo", "A1"), rerouteTo("A1", "Solo")), map[int]int{3: 0, 0: 1}},
		{"a capped entry last reads zero, a resume's grant", historyOf(stuckHistory("A1"), capped("A1")), map[int]int{0: 0}},
		{"a capped entry then one stuck reads one", historyOf(stuckHistory("A1", "A1"), capped("A1"), stuckHistory("A1")), map[int]int{0: 1}},
		{"a capped entry then another row's stuck still ends the row's scan", historyOf(stuckHistory("A1"), capped("A1"), stuckHistory("A2")), map[int]int{0: 0}},
		{"a capped entry directly followed by a goto counts back to the episode start", historyOf(stuckHistory("A1", "A1"), capped("A1"), gotoTo("A1")), map[int]int{0: 3}},
		{"a capped entry, a stuck, then a goto reads the grant's spent count", historyOf(stuckHistory("A1", "A1"), capped("A1"), stuckHistory("A1"), gotoTo("A1")), map[int]int{0: 1}},
		{"a generation start after spent stucks reads one", historyOf(stuckHistory("A1", "A1", "A1"), generationStart("A1")), map[int]int{0: 1}},
		{"a stuck after a generation start reads two", historyOf(stuckHistory("A1", "A1", "A1"), generationStart("A1"), stuckHistory("A1")), map[int]int{0: 2}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			for i, want := range tt.want {
				if got := episodeStuckCount(tt.history, ps[i], ps); got != want {
					t.Errorf("%s count = %d; want %d", ps[i].Name, got, want)
				}
			}
		})
	}
}
