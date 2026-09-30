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

func TestEpisodeStuckCount_GotoResetsSegment(t *testing.T) {
	ps := gotoTestProducers()
	h := stuckHistory("A1", "A2", "A1", "A2")
	h = append(h, HistoryEntry{Producer: "A1", Outcome: OutcomeGoto})
	for _, def := range ps[:2] {
		if got := episodeStuckCount(h, def, ps); got != 0 {
			t.Errorf("%s count = %d; want 0 after goto into its segment", def.Name, got)
		}
	}
}

func TestEpisodeStuckCount_GotoOtherSegmentResetsNeither(t *testing.T) {
	ps := gotoTestProducers()
	h := stuckHistory("A1", "A2", "A1")
	h = append(h, HistoryEntry{Producer: "B", Outcome: OutcomeGoto})
	if got := episodeStuckCount(h, ps[0], ps); got != 2 {
		t.Errorf("A1 count = %d; want 2", got)
	}
	if got := episodeStuckCount(h, ps[1], ps); got != 1 {
		t.Errorf("A2 count = %d; want 1", got)
	}
}

func TestEpisodeStuckCount_GotoUnsegmentedResetsOnlyThatRow(t *testing.T) {
	ps := gotoTestProducers()
	h := stuckHistory("Solo", "A1")
	h = append(h, HistoryEntry{Producer: "Solo", Outcome: OutcomeGoto})
	if got := episodeStuckCount(h, ps[3], ps); got != 0 {
		t.Errorf("Solo count = %d; want 0", got)
	}
	if got := episodeStuckCount(h, ps[0], ps); got != 1 {
		t.Errorf("A1 count = %d; want 1", got)
	}
}

func TestEpisodeStuckCount_GotoAbsentTargetResetsNothing(t *testing.T) {
	ps := gotoTestProducers()
	h := stuckHistory("A1", "Solo")
	h = append(h, HistoryEntry{Producer: "Gone", Outcome: OutcomeGoto})
	if got := episodeStuckCount(h, ps[0], ps); got != 1 {
		t.Errorf("A1 count = %d; want 1", got)
	}
	if got := episodeStuckCount(h, ps[3], ps); got != 1 {
		t.Errorf("Solo count = %d; want 1", got)
	}
}

func TestEpisodeStuckCount_StuckAfterGotoCountsFromOne(t *testing.T) {
	ps := gotoTestProducers()
	h := stuckHistory("A1", "A2")
	h = append(h, HistoryEntry{Producer: "A2", Outcome: OutcomeGoto})
	h = append(h, stuckHistory("A1")...)
	if got := episodeStuckCount(h, ps[0], ps); got != 1 {
		t.Errorf("A1 count = %d; want 1", got)
	}
	if got := episodeStuckCount(h, ps[1], ps); got != 0 {
		t.Errorf("A2 count = %d; want 0", got)
	}
}
