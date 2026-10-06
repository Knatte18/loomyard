package shedengine

import (
	"reflect"
	"testing"
)

func segmentedRouting() Routing {
	return Routing{
		Entry:      "write",
		MaxBounces: 4,
		Producers: []ProducerDef{
			{Name: "write", OnDone: "bouncer"},
			{Name: "bouncer", OnDone: "burn", OnStuck: "burler", Segment: "Review", MaxBounces: 2},
			{Name: "burn", OnDone: "burn2", Segment: "Review"},
			{Name: "burn2", OnDone: "publish"},
			{Name: "burler", OnStuck: "bouncer", Segment: "Review"},
			{Name: "publish"},
		},
	}
}

func TestProgressAt(t *testing.T) {
	t.Parallel()
	straight := Routing{Entry: "a", Producers: []ProducerDef{{Name: "a", OnDone: "b"}, {Name: "b", OnDone: "c"}, {Name: "c"}}}
	tests := []struct {
		name    string
		routing Routing
		at      string
		want    Progress
	}{
		{"straight line", straight, "b", Progress{Step: 2, Steps: 3, Name: "b", Remaining: []string{"c"}}},
		{"a segment counts once", segmentedRouting(), "burn", Progress{Step: 2, Steps: 4, Name: "Review", Remaining: []string{"burn2", "publish"}}},
		{"a burler maps to its segment", segmentedRouting(), "burler", Progress{Step: 2, Steps: 4, Name: "Review", Remaining: []string{"burn2", "publish"}}},
		{"the terminal row has no remaining", segmentedRouting(), "publish", Progress{Step: 4, Steps: 4, Name: "publish"}},
		{"an unknown producer is step zero", segmentedRouting(), "nope", Progress{Step: 0, Steps: 4, Name: "nope", Remaining: []string{"write", "Review", "burn2", "publish"}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := tt.routing.ProgressAt(tt.at)
			if len(got.Remaining) == 0 {
				got.Remaining = nil
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("got %+v; want %+v", got, tt.want)
			}
		})
	}
}

//testtiming:keep pins that an OnDone cycle terminates the step count, which its covering tests do not
func TestProgressAt_CyclicOnDone(t *testing.T) {
	r := Routing{Entry: "a", Producers: []ProducerDef{{Name: "a", OnDone: "b"}, {Name: "b", OnDone: "a"}}}
	if got := r.ProgressAt("b"); got.Steps != 2 || got.Step != 2 {
		t.Errorf("got %+v", got)
	}
}

func TestBounces_CountAndBudgetInheritance(t *testing.T) {
	r := segmentedRouting()
	hist := []HistoryEntry{{Producer: "bouncer", Outcome: Stuck}, {Producer: "burler", Outcome: Stuck}}
	count, budget, in := r.Bounces("bouncer", hist)
	if count != 1 || budget != 2 || !in {
		t.Errorf("bouncer: %d %d %v", count, budget, in)
	}
	if _, budget, _ := (Routing{Entry: "x", MaxBounces: 4, Producers: []ProducerDef{{Name: "x", Segment: "S"}}}).Bounces("x", nil); budget != 4 {
		t.Errorf("shed-level inheritance: budget %d; want 4", budget)
	}
	if _, budget, _ := (Routing{Entry: "x", Producers: []ProducerDef{{Name: "x", Segment: "S"}}}).Bounces("x", nil); budget != defaultMaxBounces {
		t.Errorf("default inheritance: budget %d; want %d", budget, defaultMaxBounces)
	}
	if _, _, in := r.Bounces("write", hist); in {
		t.Errorf("standalone row reported inSegment")
	}
}

func TestBounces_BurlerReportsMainLineRow(t *testing.T) {
	r := segmentedRouting()
	hist := []HistoryEntry{
		{Producer: "bouncer", Outcome: Stuck},
		{Producer: "burler", Outcome: Stuck},
		{Producer: "bouncer", Outcome: Stuck},
		{Producer: "burler", Outcome: Stuck},
	}
	c1, b1, in1 := r.Bounces("bouncer", hist)
	c2, b2, in2 := r.Bounces("burler", hist)
	if c1 != c2 || b1 != b2 || !in1 || !in2 || c1 != 2 {
		t.Errorf("bouncer (%d,%d,%v) vs burler (%d,%d,%v)", c1, b1, in1, c2, b2, in2)
	}
}
