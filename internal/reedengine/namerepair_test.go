// namerepair_test.go pins the name-repair pass on a fake tmux and a fake SessionNamer: which titles are rewritten, when a session name is renamed, and what is logged.

package reedengine

import (
	"errors"
	"reflect"
	"strconv"
	"strings"
	"testing"

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

func TestRepairNames_DriftedTitleIsRewrittenAndLogged(t *testing.T) {
	logs := logcapture.CaptureVerbose(t)
	e, fake := newRepairTestEngine(t,
		[]Strand{repairStrand("g1", "tc:s:worker", "%1", "")},
		[]LivePane{{ID: "%1", Title: "claude"}})

	if err := e.repairNames(nil); err != nil {
		t.Fatalf("repairNames: %v", err)
	}
	sel := fake.ArgvFor("select-pane")
	if len(sel) != 1 || !reflect.DeepEqual(sel[0], []string{"select-pane", "-t", "%1", "-T", "tc:s:worker"}) {
		t.Errorf("select-pane calls = %v, want one -T tc:s:worker on %%1", sel)
	}
	if !strings.Contains(logs.String(), "reed: repaired pane title") {
		t.Errorf("log = %q, want the title repair line", logs.String())
	}
}

func TestRepairNames_MatchingTitleIsLeftAlone(t *testing.T) {
	e, fake := newRepairTestEngine(t,
		[]Strand{repairStrand("g1", "tc:s:worker", "%1", "")},
		[]LivePane{{ID: "%1", Title: "tc:s:worker"}})

	if err := e.repairNames(nil); err != nil {
		t.Fatalf("repairNames: %v", err)
	}
	if sel := fake.ArgvFor("select-pane"); len(sel) != 0 {
		t.Errorf("select-pane calls = %v, want none", sel)
	}
}

func TestRepairNames_SessionNameRenamedOnIdlePane(t *testing.T) {
	logs := logcapture.CaptureVerbose(t)
	e, fake := newRepairTestEngine(t,
		[]Strand{repairStrand("g1", "tc:s:worker", "%1", "sess-1")},
		[]LivePane{{ID: "%1", Title: "tc:s:worker"}})
	namer := &fakeNamer{drift: true, idle: true}

	if err := e.repairNames(namer); err != nil {
		t.Fatalf("repairNames: %v", err)
	}
	wantQuery := [3]string{"sess-1", e.geom.PaneCwd, "tc:s:worker"}
	if len(namer.queries) != 1 || namer.queries[0] != wantQuery {
		t.Errorf("drift queries = %v, want [%v]", namer.queries, wantQuery)
	}
	keys := fake.ArgvFor("send-keys")
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
}

func TestRepairNames_BusyPaneTypesNothing(t *testing.T) {
	e, fake := newRepairTestEngine(t,
		[]Strand{repairStrand("g1", "tc:s:worker", "%1", "sess-1")},
		[]LivePane{{ID: "%1", Title: "tc:s:worker"}})

	if err := e.repairNames(&fakeNamer{drift: true, idle: false}); err != nil {
		t.Fatalf("repairNames: %v", err)
	}
	if keys := fake.ArgvFor("send-keys"); len(keys) != 0 {
		t.Errorf("send-keys calls = %v, want none on a busy pane", keys)
	}
}

func TestRepairNames_NoDriftTypesNothing(t *testing.T) {
	e, fake := newRepairTestEngine(t,
		[]Strand{repairStrand("g1", "tc:s:worker", "%1", "sess-1")},
		[]LivePane{{ID: "%1", Title: "tc:s:worker"}})

	if err := e.repairNames(&fakeNamer{drift: false, idle: true}); err != nil {
		t.Fatalf("repairNames: %v", err)
	}
	if keys := fake.ArgvFor("send-keys"); len(keys) != 0 {
		t.Errorf("send-keys calls = %v, want none without drift", keys)
	}
}

func TestRepairNames_SkipsSessionCheckWithoutNamerOrSessionID(t *testing.T) {
	t.Run("NilNamer", func(t *testing.T) {
		e, fake := newRepairTestEngine(t,
			[]Strand{repairStrand("g1", "tc:s:worker", "%1", "sess-1")},
			[]LivePane{{ID: "%1", Title: "tc:s:worker"}})
		if err := e.repairNames(nil); err != nil {
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
		if err := e.repairNames(namer); err != nil {
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

	if err := e.repairNames(nil); err == nil {
		t.Fatal("repairNames: want the session-check error")
	}
	if sel := fake.ArgvFor("select-pane"); len(sel) != 0 {
		t.Errorf("select-pane calls = %v, want none on a down session", sel)
	}
}
