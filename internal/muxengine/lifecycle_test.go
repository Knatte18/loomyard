// lifecycle_test.go drives the lifecycle ops' planning seams — the parts
// that decide what would run without needing a live psmux server:
// planUpLaunches (Up never launches anything) and planResumeLaunches across
// the three states the discussion calls out (server dead, server-up/
// CLI-restarted, a single strand's pane died). Any real-psmux round trip
// (ensureServerAndSessionLocked, and Up/Resume/Down/Status themselves) is
// out of hermetic reach and is not exercised here.

package muxengine

import (
	"testing"

	"github.com/Knatte18/loomyard/internal/muxengine/render"
)

func guids(strands []Strand) []string {
	out := make([]string, len(strands))
	for i, s := range strands {
		out[i] = s.GUID
	}
	return out
}

func TestPlanUpLaunches_NeverLaunchesAnyStrand(t *testing.T) {
	tables := [][]Strand{
		nil,
		{{GUID: "a", Display: render.Display{Anchor: render.AnchorBelowParent}}},
		{
			{GUID: "a", Display: render.Display{Anchor: render.AnchorHidden}},
			{GUID: "b", PaneID: "%1", Display: render.Display{Anchor: render.AnchorBelowParent}},
		},
	}
	for _, strands := range tables {
		if got := planUpLaunches(strands); got != nil {
			t.Errorf("planUpLaunches(%+v) = %v, want nil (Up never launches a strand command)", strands, got)
		}
	}
}

func TestNoSessionMessage_StrandCountVariants(t *testing.T) {
	tests := []struct {
		name        string
		strandCount int
		want        string
	}{
		{
			// Zero strands persisted (or no mux.json at all): nothing for
			// resume to rebuild, so today's bare "up" pointer is unchanged.
			name:        "ZeroStrands_BareUpPointer",
			strandCount: 0,
			want:        `no mux session; run "lyx mux up"`,
		},
		{
			name:        "OneStrand_ResumePointer",
			strandCount: 1,
			want:        `no mux session (1 strands persisted); run "lyx mux resume" to rebuild, or "lyx mux up" for a bare substrate`,
		},
		{
			name:        "ThreeStrands_ResumePointer",
			strandCount: 3,
			want:        `no mux session (3 strands persisted); run "lyx mux resume" to rebuild, or "lyx mux up" for a bare substrate`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := noSessionMessage(tt.strandCount); got != tt.want {
				t.Errorf("noSessionMessage(%d) = %q, want %q", tt.strandCount, got, tt.want)
			}
		})
	}
}

func TestPlanResumeLaunches_ThreeLifecycleStates(t *testing.T) {
	notLive := Strand{GUID: "a", PaneID: "%1", Display: render.Display{Anchor: render.AnchorBelowParent}}
	stillLive := Strand{GUID: "b", PaneID: "%2", Display: render.Display{Anchor: render.AnchorBelowParent}}
	hidden := Strand{GUID: "c", Display: render.Display{Anchor: render.AnchorHidden}}

	tests := []struct {
		name    string
		strands []Strand
		liveIDs map[string]bool
		want    []string
	}{
		{
			// Server dead (reboot): list-panes reports nothing live at all,
			// so every not-hidden strand — even ones with a stale PaneID —
			// must be relaunched.
			name:    "ServerDead_EveryNonHiddenStrandRelaunched",
			strands: []Strand{notLive, stillLive, hidden},
			liveIDs: map[string]bool{},
			want:    []string{"a", "b"},
		},
		{
			// Server up, CLI restarted (the normal one-shot case): every
			// strand's pane is still alive, so nothing needs relaunching.
			name:    "ServerUpCLIRestarted_NothingRelaunched",
			strands: []Strand{notLive, stillLive, hidden},
			liveIDs: map[string]bool{"%1": true, "%2": true},
			want:    nil,
		},
		{
			// A single strand's pane died: only that strand's pane id is
			// missing from liveIDs, so only it gets relaunched;
			// already-live strands are left untouched.
			name:    "SingleStrandPaneDied_OnlyThatStrandRelaunched",
			strands: []Strand{notLive, stillLive, hidden},
			liveIDs: map[string]bool{"%2": true},
			want:    []string{"a"},
		},
		{
			name:    "HiddenStrandNeverRelaunched",
			strands: []Strand{hidden},
			liveIDs: map[string]bool{},
			want:    nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := guids(planResumeLaunches(tt.strands, tt.liveIDs))
			if !equalStringSlices(got, tt.want) {
				t.Errorf("planResumeLaunches() guids = %v, want %v", got, tt.want)
			}
		})
	}
}
