// namerepair_test.go pins the name-repair pass on a fake tmux and a fake SessionNamer: which titles are rewritten, when a session name is renamed, and what is logged.

package reedengine

import (
	"context"
	"errors"
	"reflect"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/reedengine/render"
	"github.com/Knatte18/loomyard/internal/testkit/logcapture"
)

// fakeNamer is a SessionNamer answering from fixed fields and recording the drift queries it receives.
type fakeNamer struct {
	drift   bool
	idle    bool
	queries [][3]string
}

func (f *fakeNamer) SessionNameDrift(sessionID, workdir, want string) bool {
	f.queries = append(f.queries, [3]string{sessionID, workdir, want})
	return f.drift
}

func (f *fakeNamer) SessionIdle(string) bool { return f.idle }

func (f *fakeNamer) RenameText(want string) string { return "/rename " + want }

// encodeTitledPanes renders live in list-panes' wire format, title included.
func encodeTitledPanes(live []LivePane) string {
	out := ""
	for _, p := range live {
		dead := "0"
		if p.Dead {
			dead = "1"
		}
		out += strings.Join([]string{p.ID, dead, strconv.Itoa(p.Top), strconv.Itoa(p.Width), strconv.Itoa(p.Height), strconv.Itoa(p.PID), p.Title}, " ") + "\n"
	}
	return out
}

// newRepairTestEngine persists strands, scripts the fake tmux with live, and returns the engine and its fake.
func newRepairTestEngine(t *testing.T, strands []Strand, live []LivePane) (*Engine, *fakeTmux) {
	t.Helper()
	e := newTestEngine(t)
	if err := SaveState(e.stateDir(), &ReedState{Strands: strands}); err != nil {
		t.Fatalf("SaveState: %v", err)
	}
	fake := installFakeTmux(t, e)
	fake.answer("list-panes", encodeTitledPanes(live), nil)
	fake.answerFormat(paneGenerationFormat, "$0|1|1000", nil)
	fake.answer("capture-pane", "idle screen", nil)
	return e, fake
}

func repairStrand(guid, name, pane, session string) Strand {
	return Strand{GUID: guid, Name: name, PaneID: pane, SessionID: session, Display: render.Display{Anchor: render.AnchorBelowParent}}
}

//testtiming:keep pins which strands get a title repair: a bound live drifted pane only, never a matching, dead, unbound or vanished one; its covering tests run this code without asserting it
func TestPlanTitleRepairs(t *testing.T) {
	strands := []Strand{
		repairStrand("drifted", "tc:s:a", "%1", ""),
		repairStrand("matching", "tc:s:b", "%2", ""),
		repairStrand("dead", "tc:s:c", "%3", ""),
		repairStrand("unbound", "tc:s:d", "", ""),
		repairStrand("gone", "tc:s:e", "%9", ""),
	}
	live := []LivePane{
		{ID: "%1", Title: "claude"},
		{ID: "%2", Title: "tc:s:b"},
		{ID: "%3", Title: "stale", Dead: true},
	}
	got := planTitleRepairs(strands, live)
	want := []titleRepair{{GUID: "drifted", PaneID: "%1", OldTitle: "claude", Name: "tc:s:a"}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("planTitleRepairs = %+v, want %+v", got, want)
	}
}

// TestRepairNames_PaneTitle pins that a drifted pane title is rewritten with one select-pane -T and logged,
// while a matching title is left alone.
//
//testtiming:keep pins a drifted pane title being rewritten with exactly one select-pane -T and logged while a matching title is left alone; its covering tests run this code without asserting it
func TestRepairNames_PaneTitle(t *testing.T) {
	tests := []struct {
		name       string
		title      string
		wantRepair bool
	}{
		{"DriftedTitleIsRewrittenAndLogged", "claude", true},
		{"MatchingTitleIsLeftAlone", "tc:s:worker", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			logs := logcapture.CaptureVerbose(t)
			e, fake := newRepairTestEngine(t,
				[]Strand{repairStrand("g1", "tc:s:worker", "%1", "")},
				[]LivePane{{ID: "%1", Title: tt.title}})

			repaired, err := e.repairNames(nil)
			if err != nil {
				t.Fatalf("repairNames: %v", err)
			}
			if repaired != tt.wantRepair {
				t.Errorf("repairNames repaired = %v, want %v", repaired, tt.wantRepair)
			}

			sel := fake.ArgvFor("select-pane")
			logged := strings.Contains(logs.String(), "reed: repaired pane title")
			if !tt.wantRepair {
				if len(sel) != 0 {
					t.Errorf("select-pane calls = %v, want none", sel)
				}
				if logged {
					t.Errorf("log = %q, want no title repair line", logs.String())
				}
				return
			}
			if len(sel) != 1 || !reflect.DeepEqual(sel[0], []string{"select-pane", "-t", "%1", "-T", "tc:s:worker"}) {
				t.Errorf("select-pane calls = %v, want one -T tc:s:worker on %%1", sel)
			}
			if !logged {
				t.Errorf("log = %q, want the title repair line", logs.String())
			}
		})
	}
}

// TestRepairNames_SessionName pins that a drifted session name is renamed by typing the namer's rename text then Enter
// into an idle pane, logged, after exactly one drift query for the strand's session, while a busy pane or an absent drift types nothing.
func TestRepairNames_SessionName(t *testing.T) {
	tests := []struct {
		name       string
		namer      fakeNamer
		wantRename bool
	}{
		{"RenamedOnIdlePane", fakeNamer{drift: true, idle: true}, true},
		{"BusyPaneTypesNothing", fakeNamer{drift: true, idle: false}, false},
		{"NoDriftTypesNothing", fakeNamer{drift: false, idle: true}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			logs := logcapture.CaptureVerbose(t)
			e, fake := newRepairTestEngine(t,
				[]Strand{repairStrand("g1", "tc:s:worker", "%1", "sess-1")},
				[]LivePane{{ID: "%1", Title: "tc:s:worker"}})
			namer := tt.namer

			repaired, err := e.repairNames(&namer)
			if err != nil {
				t.Fatalf("repairNames: %v", err)
			}
			if repaired != tt.wantRename {
				t.Errorf("repairNames repaired = %v, want %v", repaired, tt.wantRename)
			}

			keys := fake.ArgvFor("send-keys")
			if !tt.wantRename {
				if len(keys) != 0 {
					t.Errorf("send-keys calls = %v, want none", keys)
				}
				return
			}
			wantQuery := [3]string{"sess-1", e.geom.PaneCwd, "tc:s:worker"}
			if len(namer.queries) != 1 || namer.queries[0] != wantQuery {
				t.Errorf("drift queries = %v, want [%v]", namer.queries, wantQuery)
			}
			if len(keys) != 2 {
				t.Fatalf("send-keys calls = %v, want the literal text then Enter", keys)
			}
			if keys[0][3] != "-l" || !strings.Contains(keys[0][4], "/rename tc:s:worker") {
				t.Errorf("first send-keys = %v, want the literal rename text", keys[0])
			}
			if keys[1][len(keys[1])-1] != "Enter" {
				t.Errorf("second send-keys = %v, want Enter", keys[1])
			}
			if !strings.Contains(logs.String(), "reed: repaired session name") {
				t.Errorf("log = %q, want the session repair line", logs.String())
			}
		})
	}
}

func TestRepairNames_SkipsSessionCheckWithoutNamerOrSessionID(t *testing.T) {
	t.Run("NilNamer", func(t *testing.T) {
		e, fake := newRepairTestEngine(t,
			[]Strand{repairStrand("g1", "tc:s:worker", "%1", "sess-1")},
			[]LivePane{{ID: "%1", Title: "tc:s:worker"}})
		if _, err := e.repairNames(nil); err != nil {
			t.Fatalf("repairNames: %v", err)
		}
		if got := fake.ArgvFor("capture-pane"); len(got) != 0 {
			t.Errorf("capture-pane calls = %v, want none", got)
		}
	})
	t.Run("EmptySessionID", func(t *testing.T) {
		e, _ := newRepairTestEngine(t,
			[]Strand{repairStrand("g1", "tc:s:worker", "%1", "")},
			[]LivePane{{ID: "%1", Title: "tc:s:worker"}})
		namer := &fakeNamer{drift: true, idle: true}
		if _, err := e.repairNames(namer); err != nil {
			t.Fatalf("repairNames: %v", err)
		}
		if len(namer.queries) != 0 {
			t.Errorf("drift queries = %v, want none for a strand with no SessionID", namer.queries)
		}
	})
}

func TestRepairNames_UnanswerableSessionCheckRepairsNothing(t *testing.T) {
	e, fake := newRepairTestEngine(t,
		[]Strand{repairStrand("g1", "tc:s:worker", "%1", "")},
		[]LivePane{{ID: "%1", Title: "claude"}})
	fake.answer("has-session", "", errors.New("tmux unreachable"))

	if _, err := e.repairNames(nil); err == nil {
		t.Fatal("repairNames: want the session-check error")
	}
	if sel := fake.ArgvFor("select-pane"); len(sel) != 0 {
		t.Errorf("select-pane calls = %v, want none on a down session", sel)
	}
}

// TestWatchNames_RepairedAnswerChoosesTheNextWait pins that the waits climb to the ceiling while no pass repairs anything, and return to the base after a pass that repaired a title.
func TestWatchNames_RepairedAnswerChoosesTheNextWait(t *testing.T) {
	e, fake := newRepairTestEngine(t,
		[]Strand{repairStrand("g1", "tc:s:worker", "%1", "")},
		[]LivePane{{ID: "%1", Title: "tc:s:worker"}})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var waits []time.Duration
	timing := nameRepairTiming{
		Base:    time.Second,
		Ceiling: 4 * time.Second,
		After: func(d time.Duration) <-chan time.Time {
			waits = append(waits, d)
			switch len(waits) {
			case 4:
				fake.answer("list-panes", encodeTitledPanes([]LivePane{{ID: "%1", Title: "claude"}}), nil)
			case 5:
				cancel()
				return nil
			}
			fired := make(chan time.Time, 1)
			fired <- time.Time{}
			return fired
		},
	}

	if err := e.watchNames(ctx, nil, timing); !errors.Is(err, context.Canceled) {
		t.Fatalf("watchNames = %v, want context.Canceled", err)
	}
	want := []time.Duration{time.Second, 2 * time.Second, 4 * time.Second, 4 * time.Second, time.Second}
	if !reflect.DeepEqual(waits, want) {
		t.Errorf("waits = %v, want %v", waits, want)
	}
}
