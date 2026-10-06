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

// TestEpisodeStuckCount_GotoResets pins which producers' stuck episodes a goto history entry ends:
// the target's whole segment, only the target row when it has no segment, nothing for an absent target,
// and a stuck after a goto counts from one.
func TestEpisodeStuckCount_GotoResets(t *testing.T) {
	t.Parallel()
	ps := gotoTestProducers()
	gotoTo := func(producer string) HistoryEntry { return HistoryEntry{Producer: producer, Outcome: OutcomeGoto} }
	tests := []struct {
		name    string
		history []HistoryEntry
		// want maps a producer's index in gotoTestProducers to its expected stuck count.
		want map[int]int
	}{
		{"goto into a segment resets the whole segment", append(stuckHistory("A1", "A2", "A1", "A2"), gotoTo("A1")), map[int]int{0: 0, 1: 0}},
		{"goto into another segment resets neither", append(stuckHistory("A1", "A2", "A1"), gotoTo("B")), map[int]int{0: 2, 1: 1}},
		{"goto an unsegmented row resets only that row", append(stuckHistory("Solo", "A1"), gotoTo("Solo")), map[int]int{3: 0, 0: 1}},
		{"goto an absent target resets nothing", append(stuckHistory("A1", "Solo"), gotoTo("Gone")), map[int]int{0: 1, 3: 1}},
		{"stuck after a goto counts from one", append(append(stuckHistory("A1", "A2"), gotoTo("A2")), stuckHistory("A1")...), map[int]int{0: 1, 1: 0}},
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
