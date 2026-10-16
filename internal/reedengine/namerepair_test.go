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

	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/reedengine/render"
	"github.com/Knatte18/loomyard/internal/testkit/logcapture"
)

// fakeNamer is a SessionNamer answering from fixed fields and recording the drift queries it receives.
type fakeNamer struct {
	drift   bool
	idle    bool
	queries [][3]string
	// text is the rename text's prefix, "/rename " when empty.
	text string
}

// untagged is the namer map a strand with no provider tag is repaired through.
func untagged(f fakeNamer) map[string]fakeNamer { return map[string]fakeNamer{"": f} }

func (f *fakeNamer) SessionNameDrift(sessionID, workdir, want string) bool {
	f.queries = append(f.queries, [3]string{sessionID, workdir, want})
	return f.drift
}

func (f *fakeNamer) SessionIdle(string) bool { return f.idle }

func (f *fakeNamer) RenameText(want string) string {
	if f.text == "" {
		return "/rename " + want
	}
	return f.text + want
}

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
	return Strand{GUID: guid, Name: name, PaneID: pane, SessionID: session, Display: render.Display{Anchor: render.AnchorOwnWindow}}
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

// TestRepairNames_SessionName pins that a drifted session name is renamed by one paste-submit of the namer's rename text.
// The rename goes into an idle pane with no active operator, logged, after exactly one drift query for the strand's session.
// A busy pane, an active operator or an absent drift types nothing, and an unreadable client list does not hold the rename.
// A row with failedRuns runs the repair that many times on one engine before heal, then once after it.
// It pins that the failure logs one Warn and one Debug per further run, and the heal one Info.
// It captures the process-global logger at Debug, so it stays serial.
func TestRepairNames_SessionName(t *testing.T) {
	activeClient := "%1 " + strconv.FormatInt(time.Now().Unix(), 10) + "\n"
	tests := []struct {
		name string
		// provider is the tag on the strand, and namers the session namers by tag the pass repairs through.
		provider string
		namers   map[string]fakeNamer
		// title is the live pane's title, the strand's name when empty.
		title string
		// renamedBy is the tag of the namer the rename goes through, the empty tag when unset.
		renamedBy string
		// wantTitleRepair is whether the row's pane-title mismatch is repaired by select-pane -T.
		wantTitleRepair bool
		clients         string
		clientsErr      error
		wantRename      bool
		// ownWindow anchors the strand own-window and lists its pane only in windowPanes, the session-wide listing.
		ownWindow   bool
		windowPanes string
		// failedRuns is how many repairs run on one engine before heal; zero makes the row a single repair.
		failedRuns int
		// fail scripts the fake tmux to fail before the first repair; heal scripts it healthy again before the last.
		fail, heal func(f *fakeTmux)
		// failedRenames is whether each repair run before heal still renames.
		failedRenames bool
		// failMsg is the failure line every run before heal logs, and healMsg the recovery line the repair after heal logs.
		failMsg, healMsg string
	}{
		{name: "RenamedOnIdlePane", namers: untagged(fakeNamer{drift: true, idle: true}), wantRename: true},
		{
			name:        "OwnWindowStrandRenamedFromTheSessionListing",
			namers:      untagged(fakeNamer{drift: true, idle: true}),
			wantRename:  true,
			ownWindow:   true,
			windowPanes: "@3 1 - %1 0 4321 tc:s:worker\n",
		},
		{name: "BusyPaneTypesNothing", namers: untagged(fakeNamer{drift: true, idle: false})},
		{name: "NoDriftTypesNothing", namers: untagged(fakeNamer{drift: false, idle: true})},
		{name: "ActiveOperatorTypesNothing", namers: untagged(fakeNamer{drift: true, idle: true}), clients: activeClient},
		{
			name:     "TaggedStrandRenamedThroughItsTagsNamer",
			provider: "claude",
			namers: map[string]fakeNamer{
				"claude": {drift: true, idle: true, text: "/claude-rename "},
				"":       {drift: false, idle: true, text: "/empty-rename "},
			},
			renamedBy:  "claude",
			wantRename: true,
		},
		{
			name: "UntaggedStrandRenamedThroughTheEmptyTagsNamer",
			namers: map[string]fakeNamer{
				"claude": {drift: false, idle: true, text: "/claude-rename "},
				"":       {drift: true, idle: true, text: "/empty-rename "},
			},
			wantRename: true,
		},
		{
			name:     "StrandWithoutANamerForItsTagGetsTheTitleRepairOnly",
			provider: "codex",
			namers: map[string]fakeNamer{
				"claude": {drift: true, idle: true},
				"":       {drift: true, idle: true},
			},
			title:           "codex",
			wantTitleRepair: true,
		},
		{
			name:          "UnreadableClientListStillRenames",
			namers:        untagged(fakeNamer{drift: true, idle: true}),
			clientsErr:    errors.New("list-clients failed"),
			wantRename:    true,
			failedRuns:    2,
			failedRenames: true,
			heal:          func(f *fakeTmux) { f.answer("list-clients", "", nil) },
			failMsg:       "operator activity unreadable",
			healMsg:       "operator activity readable again for session name repair",
		},
		{
			name:       "FailedCaptureLogsOncePerEpisode",
			namers:     untagged(fakeNamer{drift: true, idle: true}),
			wantRename: true,
			failedRuns: 3,
			fail:       func(f *fakeTmux) { f.answer("capture-pane", "", errors.New("capture failed")) },
			heal:       func(f *fakeTmux) { f.answer("capture-pane", "idle screen", nil) },
			failMsg:    "could not capture pane for session name repair",
			healMsg:    "session name repair captures the pane again",
		},
		{
			name:       "FailedRenameLogsOncePerEpisode",
			namers:     untagged(fakeNamer{drift: true, idle: true}),
			wantRename: true,
			failedRuns: 3,
			fail:       func(f *fakeTmux) { f.answer("load-buffer", "", errors.New("paste failed")) },
			heal:       func(f *fakeTmux) { f.answer("load-buffer", "", nil) },
			failMsg:    "could not type the session rename",
			healMsg:    "session rename types again",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			logs := logcapture.CaptureVerbose(t)
			logger.SetVerbosity(2)
			strand := repairStrand("g1", "tc:s:worker", "%1", "sess-1")
			strand.Provider = tt.provider
			if tt.ownWindow {
				strand.Display.Anchor = render.AnchorOwnWindow
			}
			title := tt.title
			if title == "" {
				title = strand.Name
			}
			e, fake := newRepairTestEngine(t,
				[]Strand{strand},
				[]LivePane{{ID: "%1", Title: title}})
			if tt.ownWindow {
				fake.answer("list-panes", "", nil)
				fake.answerWindowPanes(tt.windowPanes, nil)
			}
			fake.answer("list-clients", tt.clients, tt.clientsErr)
			if tt.fail != nil {
				tt.fail(fake)
			}
			fakes := make(map[string]*fakeNamer, len(tt.namers))
			namers := make(map[string]SessionNamer, len(tt.namers))
			for tag, f := range tt.namers {
				f := f
				fakes[tag] = &f
				namers[tag] = &f
			}

			runs := 1 + tt.failedRuns
			repairedRuns := 0
			for run := 0; run < runs; run++ {
				healed := run == tt.failedRuns
				if healed && tt.heal != nil {
					tt.heal(fake)
				}
				want := tt.wantRename && (healed || tt.failedRenames)
				repaired, err := e.repairNames(namers)
				if err != nil {
					t.Fatalf("repairNames run %d: %v", run, err)
				}
				want = want || tt.wantTitleRepair
				if repaired != want {
					t.Errorf("repairNames run %d repaired = %v, want %v", run, repaired, want)
				}
				if repaired {
					repairedRuns++
				}
			}

			if tt.wantTitleRepair {
				if sel := fake.ArgvFor("select-pane"); len(sel) != 1 || !reflect.DeepEqual(sel[0], []string{"select-pane", "-t", "%1", "-T", "tc:s:worker"}) {
					t.Errorf("select-pane calls = %v, want one -T tc:s:worker on %%1", sel)
				}
				for tag, f := range fakes {
					if len(f.queries) != 0 {
						t.Errorf("namer %q drift queries = %v, want none for a strand tagged %q", tag, f.queries, tt.provider)
					}
				}
			}
			pastes := fake.ArgvFor("load-buffer")
			if !tt.wantRename {
				if len(pastes) != 0 {
					t.Errorf("load-buffer calls = %v, want none", pastes)
				}
				return
			}
			wantQuery := [3]string{"sess-1", e.geom.PaneCwd, "tc:s:worker"}
			for tag, f := range fakes {
				if tag != tt.renamedBy {
					if len(f.queries) != 0 {
						t.Errorf("namer %q drift queries = %v, want none", tag, f.queries)
					}
					continue
				}
				if len(f.queries) != runs {
					t.Errorf("drift queries = %v, want %d of [%v]", f.queries, runs, wantQuery)
				}
				for _, query := range f.queries {
					if query != wantQuery {
						t.Errorf("drift query = %v, want %v", query, wantQuery)
					}
				}
			}
			// A failed capture never reaches the paste, so only the healed run pastes.
			wantPastes := runs
			if strings.Contains(tt.failMsg, "capture") {
				wantPastes = 1
			}
			if len(pastes) != wantPastes {
				t.Fatalf("load-buffer calls = %v, want %d paste-submit invocations", pastes, wantPastes)
			}
			argv := pastes[0]
			if !containsArg(argv, "paste-buffer") || !containsArg(argv, "-p") || !containsArg(argv, "-d") {
				t.Errorf("paste-submit argv = %v, want paste-buffer -p -d", argv)
			}
			if argv[len(argv)-1] != "Enter" || !containsArg(argv, "send-keys") {
				t.Errorf("paste-submit argv = %v, want it to end in send-keys Enter", argv)
			}
			wantStdin := make([]string, wantPastes)
			for i := range wantStdin {
				wantStdin[i] = fakes[tt.renamedBy].RenameText("tc:s:worker")
			}
			if got := fake.StdinFor("load-buffer"); !reflect.DeepEqual(got, wantStdin) {
				t.Errorf("load-buffer stdin = %q, want %q", got, wantStdin)
			}
			if keys := fake.ArgvFor("send-keys"); len(keys) != 0 {
				t.Errorf("separate send-keys calls = %v, want none, the whole write is one invocation", keys)
			}
			if got := countLogLines(logs.String(), "INFO", "reed: repaired session name"); got != repairedRuns {
				t.Errorf("session repair INFO lines = %d, want %d; log:\n%s", got, repairedRuns, logs.String())
			}
			if tt.failedRuns == 0 {
				return
			}
			if got := countLogLines(logs.String(), "WARN", tt.failMsg); got != 1 {
				t.Errorf("failure WARN lines = %d, want exactly 1; log:\n%s", got, logs.String())
			}
			if got := countLogLines(logs.String(), "DEBUG", tt.failMsg); got != tt.failedRuns-1 {
				t.Errorf("failure DEBUG lines = %d, want %d; log:\n%s", got, tt.failedRuns-1, logs.String())
			}
			if got := countLogLines(logs.String(), "DEBUG", "failed_ticks="); got < tt.failedRuns-1 {
				t.Errorf("failure DEBUG lines carrying failed_ticks = %d, want at least %d; log:\n%s", got, tt.failedRuns-1, logs.String())
			}
			if got := countLogLines(logs.String(), "INFO", tt.healMsg); got != 1 {
				t.Errorf("recovery INFO lines = %d, want exactly 1; log:\n%s", got, logs.String())
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
		if _, err := e.repairNames(map[string]SessionNamer{"": namer}); err != nil {
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
