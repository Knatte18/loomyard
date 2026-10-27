//go:build tier1

// delivery_test.go covers the hub delivery over a hubforge hub with a pair, reed states seeded on disk and the worktree wiring replaced by a stub.

package creelcli

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"

	"github.com/Knatte18/loomyard/internal/configengine"
	"github.com/Knatte18/loomyard/internal/creelengine"
	"github.com/Knatte18/loomyard/internal/hubforge"
	"github.com/Knatte18/loomyard/internal/hubgeom"
	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/lymcwd"
	"github.com/Knatte18/loomyard/internal/lymdirs"
	"github.com/Knatte18/loomyard/internal/pauserecord"
	"github.com/Knatte18/loomyard/internal/reedengine"
	"github.com/Knatte18/loomyard/internal/segmentcolor"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
	"github.com/Knatte18/loomyard/internal/testkit/logcapture"
	"github.com/Knatte18/loomyard/internal/testkit/shuttlefake"
)

const pairSlug = hubforge.TemplatePairSlug

// stubOps answers each worktreeOps call as scripted.
type stubOps struct {
	status       reedengine.StatusResult
	statusErr    error
	idle         bool
	tooShort     bool
	draft        bool
	noInputBox   bool
	sendErr      error
	commandErr   error
	interruptErr error
	// onStatus, when set, runs first in Status, so a test can change the worktree between the resolve and the status step.
	onStatus func()
	// onWire, when set, runs when the worktree is wired, and wireErr is what the wiring then answers.
	onWire  func()
	wireErr error
}

func (s *stubOps) Status() (reedengine.StatusResult, error) {
	if s.onStatus != nil {
		s.onStatus()
	}
	return s.status, s.statusErr
}

func (s *stubOps) SessionIdle(string) (shuttleengine.IdleProbe, error) {
	return shuttleengine.IdleProbe{Idle: s.idle, TooShort: s.tooShort, Draft: s.draft, NoInputBox: s.noInputBox}, nil
}

func (s *stubOps) Send(string, string) error { return s.sendErr }

func (s *stubOps) RunCommand(string, creelengine.Command) error { return s.commandErr }

func (s *stubOps) Interrupt(string) error { return s.interruptErr }

// recordingCommands is a sessionCommands that records the one call it received as "method(arguments)".
type recordingCommands struct {
	called string
}

func (r *recordingCommands) ClearSession(guid string) error {
	r.called = "ClearSession(" + guid + ")"
	return nil
}

func (r *recordingCommands) CompactSession(guid, focus string) error {
	r.called = "CompactSession(" + guid + "," + focus + ")"
	return nil
}

func (r *recordingCommands) ReloadPlugins(guid string) error {
	r.called = "ReloadPlugins(" + guid + ")"
	return nil
}

func (r *recordingCommands) TypeColor(guid string, color segmentcolor.Color) error {
	r.called = "TypeColor(" + guid + "," + string(color) + ")"
	return nil
}

func (r *recordingCommands) LoadSkills(guid string, skills []string) error {
	r.called = "LoadSkills(" + guid + "," + strings.Join(skills, "+") + ")"
	return nil
}

// restoringPane is a pane drawn like Claude's across an interrupt: a running turn over an empty box until the Escape, then the stopped turn's prompt restored into the box until a C-u deletes it.
type restoringPane struct {
	mu       sync.Mutex
	escaped  bool
	restored string
}

func (p *restoringPane) capture(string) (string, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	switch {
	case !p.escaped:
		return "RUNNING\n❯ ", nil
	default:
		return "IDLE\n❯ " + p.restored, nil
	}
}

func (p *restoringPane) sendKey(_, key string) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	switch key {
	case "Escape":
		p.escaped, p.restored = true, "write the tests"
	case "C-u":
		p.restored = ""
	}
	return nil
}

// restoringEngine is a shuttlefake engine for a restoringPane: RUNNING reads as a running turn, the box is the text after the caret, and the restored-prompt clear is one C-u.
type restoringEngine struct {
	*shuttlefake.Engine
}

var (
	_ shuttleengine.InputBoxReader        = (*restoringEngine)(nil)
	_ shuttleengine.RestoredPromptClearer = (*restoringEngine)(nil)
	_ shuttleengine.InterruptGuard        = (*restoringEngine)(nil)
)

func (e *restoringEngine) InterruptSequence() []shuttleengine.PaneInput {
	return []shuttleengine.PaneInput{{Key: "Escape"}}
}

func (e *restoringEngine) RunningTurn(capture string) bool {
	return strings.Contains(capture, "RUNNING")
}

func (e *restoringEngine) InterruptSettle() time.Duration { return 2 * time.Second }

func (e *restoringEngine) InputBoxText(capture string) (string, bool) {
	_, box, found := strings.Cut(capture, "❯")
	return strings.TrimSpace(box), found
}

func (e *restoringEngine) SubmitSettle() time.Duration { return 0 }

func (e *restoringEngine) TypeSequence(text string) []shuttleengine.PaneInput {
	return []shuttleengine.PaneInput{{Text: text}}
}

func (e *restoringEngine) RestoredPromptClearSequence(string) []shuttleengine.PaneInput {
	return []shuttleengine.PaneInput{{Key: "C-u"}}
}

// stepClock is a shuttleengine.Clock whose Sleep advances its time and returns, so a runner's settles cost nothing.
type stepClock struct {
	now time.Time
}

func (c *stepClock) Now() time.Time { return c.now }

func (c *stepClock) Sleep(d time.Duration) { c.now = c.now.Add(d) }

// newStubbedDelivery returns the delivery of hub with its wiring answered by stubs keyed by worktree path.
func newStubbedDelivery(hub *hubforge.Hub, stubs map[string]*stubOps) *hubDelivery {
	delivery := newHubDelivery(hub.Path, nil)
	delivery.wire = func(location *lymcwd.Location) (worktreeOps, error) {
		stub, ok := stubs[location.WorktreePath()]
		if !ok {
			return nil, fmt.Errorf("no stub for %s", location.WorktreePath())
		}
		if stub.onWire != nil {
			stub.onWire()
		}
		if stub.wireErr != nil {
			return nil, stub.wireErr
		}
		return stub, nil
	}
	return delivery
}

// seedReedState persists the strands (name to guid) in the reed state of the worktree at path.
func seedReedState(t *testing.T, path string, strands map[string]string) {
	t.Helper()
	location, err := lymcwd.ResolveWorktree(path)
	if err != nil {
		t.Fatalf("ResolveWorktree(%q) = %v", path, err)
	}
	reedState := &reedengine.ReedState{}
	for name, guid := range strands {
		reedState.Strands = append(reedState.Strands, reedengine.Strand{Name: name, GUID: guid})
	}
	dotLym := filepath.Join(location.AnchorPath(), lymdirs.DotLymDirName)
	if err := os.MkdirAll(dotLym, 0o755); err != nil {
		t.Fatalf("MkdirAll(%q) = %v", dotLym, err)
	}
	if err := reedengine.SaveState(dotLym, reedState); err != nil {
		t.Fatalf("SaveState(%q) = %v", dotLym, err)
	}
}

// pauseRecordPath is where the hub's pause record lives, told through the worktree at path.
func pauseRecordPath(t *testing.T, path string) string {
	t.Helper()
	location, err := lymcwd.ResolveWorktree(path)
	if err != nil {
		t.Fatalf("ResolveWorktree(%q) = %v", path, err)
	}
	geometry, err := hubgeom.PauseGeometry(location)
	if err != nil {
		t.Fatalf("PauseGeometry() = %v", err)
	}
	return geometry.RecordPath
}

func TestHubDelivery(t *testing.T) {
	t.Parallel()

	newHub := func(t *testing.T) (*hubforge.Hub, string, string) {
		hub := hubforge.CopyHub(t, hubforge.Shape{Anchor: ".", Pairs: []string{pairSlug}})
		return hub, hub.PrimeWorktree(), hub.PairCodeWorktree(pairSlug)
	}

	t.Run("Strands lists the prime and the pair with slugs, liveness and pane pids", func(t *testing.T) {
		t.Parallel()
		hub, prime, pair := newHub(t)
		seedReedState(t, prime, map[string]string{"ly:orch": "guid-orch"})
		seedReedState(t, pair, map[string]string{"ly:pair:impl": "guid-impl", "ly:pair:dormant": "guid-dormant"})
		delivery := newStubbedDelivery(hub, map[string]*stubOps{
			prime: {status: reedengine.StatusResult{Strands: []reedengine.StrandStatus{{GUID: "guid-orch", Live: true, PanePID: 11}}}},
			pair:  {status: reedengine.StatusResult{Strands: []reedengine.StrandStatus{{GUID: "guid-impl", Live: true, PanePID: 22}, {GUID: "guid-dormant"}}}},
		})

		got, err := delivery.Strands()
		if err != nil {
			t.Fatalf("Strands() = %v", err)
		}
		want := []creelengine.Strand{
			{Name: "ly:orch", GUID: "guid-orch", Worktree: prime, Live: true, PanePID: 11},
			{Name: "ly:pair:dormant", GUID: "guid-dormant", Worktree: pair, Slug: pairSlug},
			{Name: "ly:pair:impl", GUID: "guid-impl", Worktree: pair, Slug: pairSlug, Live: true, PanePID: 22},
		}
		sortStrands(got)
		if diff := cmp.Diff(want, got); diff != "" {
			t.Errorf("Strands() mismatch (-want +got):\n%s", diff)
		}
	})

	t.Run("Strands reads a worktree without a session as dormant and fails on another status error", func(t *testing.T) {
		t.Parallel()
		hub, prime, pair := newHub(t)
		seedReedState(t, pair, map[string]string{"ly:pair:impl": "guid-impl"})
		stubs := map[string]*stubOps{
			prime: {},
			pair:  {statusErr: fmt.Errorf("status: %w", reedengine.ErrNoSession)},
		}
		delivery := newStubbedDelivery(hub, stubs)

		got, err := delivery.Strands()
		if err != nil || len(got) != 1 || got[0].Live {
			t.Fatalf("Strands() with no session = (%+v, %v); want one dormant strand", got, err)
		}

		stubs[pair].statusErr = errors.New("tmux exploded")
		_, err = delivery.Strands()
		if err == nil || !strings.Contains(err.Error(), pair) {
			t.Errorf("Strands() with a failing status error = %v; want one naming %s", err, pair)
		}
	})

	t.Run("Strands fails naming the file on a corrupt reed state", func(t *testing.T) {
		t.Parallel()
		hub, prime, pair := newHub(t)
		seedReedState(t, pair, map[string]string{"ly:pair:impl": "guid-impl"})
		corrupt := filepath.Join(pair, lymdirs.DotLymDirName, "reed.json")
		if err := os.WriteFile(corrupt, []byte("{not json"), 0o644); err != nil {
			t.Fatalf("WriteFile(%q) = %v", corrupt, err)
		}
		delivery := newStubbedDelivery(hub, map[string]*stubOps{prime: {}, pair: {}})

		_, err := delivery.Strands()
		if err == nil || !strings.Contains(err.Error(), "reed.json") {
			t.Errorf("Strands() with a corrupt reed.json = %v; want one naming the file", err)
		}
	})

	t.Run("wireShuttle tells the runner the hub's pause record with an empty slug", func(t *testing.T) {
		t.Parallel()
		_, _, pair := newHub(t)
		location, err := lymcwd.ResolveWorktree(pair)
		if err != nil {
			t.Fatalf("ResolveWorktree(%q) = %v", pair, err)
		}
		enginesFor := func(shuttleengine.Config, configengine.Dirs) (shuttleengine.EngineSet, error) {
			return shuttlefake.EngineSet(&shuttlefake.Engine{}), nil
		}

		ops, err := wireShuttle(location, enginesFor)
		if err != nil {
			t.Fatalf("wireShuttle() = %v", err)
		}
		want := pauserecord.Geometry{RecordPath: pauseRecordPath(t, pair)}
		if got := ops.runner.PauseGeometry(); got != want {
			t.Errorf("runner PauseGeometry() = %+v; want %+v, the hub's record path with no slug", got, want)
		}
	})

	t.Run("Send and Interrupt map shuttle's sentinels onto creel's", func(t *testing.T) {
		t.Parallel()
		hub, prime, _ := newHub(t)
		other := errors.New("boom")
		tests := []struct {
			name string
			in   error
			want error
		}{
			{"busy", fmt.Errorf("wrapped: %w", shuttleengine.ErrSessionBusy), creelengine.ErrBusy},
			{"paused", fmt.Errorf("wrapped: %w", shuttleengine.ErrPaused), creelengine.ErrBusy},
			{"not landed", fmt.Errorf("wrapped: %w", shuttleengine.ErrSubmissionNotLanded), creelengine.ErrNotLanded},
			{"sent text in draft", fmt.Errorf("wrapped: %w", shuttleengine.ErrSentTextInDraft), creelengine.ErrNotLanded},
			{"foreign box text", fmt.Errorf("wrapped: %w", shuttleengine.ErrForeignBoxText), creelengine.ErrNotLanded},
			{"no running turn", fmt.Errorf("wrapped: %w", shuttleengine.ErrNoRunningTurn), creelengine.ErrNoRunningTurn},
			{"other", other, other},
			{"nil", nil, nil},
		}
		strand := creelengine.Strand{Name: "ly:orch", GUID: "guid-orch", Worktree: prime}
		for _, tt := range tests {
			stub := &stubOps{sendErr: tt.in, interruptErr: tt.in}
			delivery := newStubbedDelivery(hub, map[string]*stubOps{prime: stub})
			if got := delivery.Send(strand, "hello"); got != tt.want {
				t.Errorf("Send() with %s = %v; want %v", tt.name, got, tt.want)
			}
			if got := delivery.Interrupt(strand); got != tt.want {
				t.Errorf("Interrupt() with %s = %v; want %v", tt.name, got, tt.want)
			}
		}
		clearFailed := fmt.Errorf("wrapped: %w", shuttleengine.ErrInterruptClearFailed)
		stub := &stubOps{interruptErr: clearFailed}
		got := newStubbedDelivery(hub, map[string]*stubOps{prime: stub}).Interrupt(strand)
		if !errors.Is(got, creelengine.ErrClearFailed) || !strings.Contains(got.Error(), clearFailed.Error()) {
			t.Errorf("Interrupt() with a failed clear = %v; want it wrapping ErrClearFailed and keeping shuttle's message", got)
		}
	})

	t.Run("Each command reaches its session method, and Command maps shuttle's sentinels onto creel's", func(t *testing.T) {
		t.Parallel()
		tests := []struct {
			command creelengine.Command
			want    string
		}{
			{creelengine.Command{Name: creelengine.CommandClear}, "ClearSession(guid-orch)"},
			{creelengine.Command{Name: creelengine.CommandCompact, Argument: "keep the plan"}, "CompactSession(guid-orch,keep the plan)"},
			{creelengine.Command{Name: creelengine.CommandReloadPlugins}, "ReloadPlugins(guid-orch)"},
			{creelengine.Command{Name: creelengine.CommandColor, Argument: "blue"}, "TypeColor(guid-orch,blue)"},
			{creelengine.Command{Name: creelengine.CommandLoadSkills, Argument: "scribe:prose scribe:testing"}, "LoadSkills(guid-orch,scribe:prose+scribe:testing)"},
		}
		for _, tt := range tests {
			recorder := &recordingCommands{}
			if err := runSessionCommand(recorder, "guid-orch", tt.command); err != nil || recorder.called != tt.want {
				t.Errorf("runSessionCommand(%s) called %q, %v; want %q", tt.command.Name, recorder.called, err, tt.want)
			}
		}

		hub, prime, _ := newHub(t)
		strand := creelengine.Strand{Name: "ly:orch", GUID: "guid-orch", Worktree: prime}
		other := errors.New("boom")
		for _, sentinel := range []struct {
			in   error
			want error
		}{
			{fmt.Errorf("wrapped: %w", shuttleengine.ErrSessionBusy), creelengine.ErrBusy},
			{fmt.Errorf("wrapped: %w", shuttleengine.ErrPaused), creelengine.ErrBusy},
			{fmt.Errorf("wrapped: %w", shuttleengine.ErrSubmissionNotLanded), creelengine.ErrNotLanded},
			{other, other},
			{nil, nil},
		} {
			delivery := newStubbedDelivery(hub, map[string]*stubOps{prime: {commandErr: sentinel.in}})
			if got := delivery.Command(strand, creelengine.Command{Name: creelengine.CommandClear}); got != sentinel.want {
				t.Errorf("Command() with %v = %v; want %v", sentinel.in, got, sentinel.want)
			}
		}
	})

	t.Run("Interrupt through a runner on a strand with no run record sends the key and then the restored-prompt clear", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()
		pane := &restoringPane{}
		reed := &shuttlefake.Reed{
			Strands:       []reedengine.StrandStatus{{GUID: "guid-hand", Name: "ly:hand", PaneID: "%1", Live: true}},
			CapturePaneFn: pane.capture,
			SendKeyFn:     pane.sendKey,
		}
		runner := shuttleengine.NewRunner(reed, shuttlefake.EngineSet(&restoringEngine{Engine: &shuttlefake.Engine{}}), dir, dir, shuttleengine.Config{})
		runner.SetClock(&stepClock{now: time.Unix(0, 0)})

		if err := (shuttleOps{runner: runner}).Interrupt("guid-hand"); err != nil {
			t.Fatalf("shuttleOps.Interrupt() = %v; want the key and the clear to go through", err)
		}
		var keys []string
		for _, call := range reed.SendKeyCalls {
			keys = append(keys, call.Key)
		}
		if diff := cmp.Diff([]string{"Escape", "C-u"}, keys); diff != "" {
			t.Errorf("keys sent mismatch (-want +got):\n%s", diff)
		}
	})

	t.Run("Idle reports the stub's probe", func(t *testing.T) {
		t.Parallel()
		hub, prime, _ := newHub(t)
		strand := creelengine.Strand{Name: "ly:orch", GUID: "guid-orch", Worktree: prime}
		tests := []struct {
			name string
			stub stubOps
			want creelengine.Idleness
		}{
			{"idle", stubOps{idle: true}, creelengine.Idleness{Idle: true}},
			{"too short", stubOps{tooShort: true}, creelengine.Idleness{Stall: creelengine.StallTooShort}},
			{"draft", stubOps{draft: true}, creelengine.Idleness{Stall: creelengine.StallDraft}},
			{"too short and draft", stubOps{tooShort: true, draft: true}, creelengine.Idleness{Stall: creelengine.StallTooShort}},
			{"no input box", stubOps{noInputBox: true}, creelengine.Idleness{Stall: creelengine.StallNoInputBox}},
			{"too short and no input box", stubOps{tooShort: true, noInputBox: true}, creelengine.Idleness{Stall: creelengine.StallTooShort}},
			{"draft and no input box", stubOps{draft: true, noInputBox: true}, creelengine.Idleness{Stall: creelengine.StallDraft}},
			{"not idle with no reason", stubOps{}, creelengine.Idleness{}},
		}
		for _, tt := range tests {
			stub := tt.stub
			delivery := newStubbedDelivery(hub, map[string]*stubOps{prime: &stub})
			if got, err := delivery.Idle(strand); err != nil || got != tt.want {
				t.Errorf("Idle() with %s = (%+v, %v); want (%+v, nil)", tt.name, got, err, tt.want)
			}
		}
	})

	t.Run("Paused follows the pause record's scope", func(t *testing.T) {
		t.Parallel()
		hub, prime, pair := newHub(t)
		recordPath := pauseRecordPath(t, prime)
		primeStrand := creelengine.Strand{Name: "ly:orch", GUID: "g1", Worktree: prime}
		pairStrand := creelengine.Strand{Name: "ly:pair:impl", GUID: "g2", Worktree: pair, Slug: pairSlug}
		delivery := newStubbedDelivery(hub, nil)

		pause := func(scope string) {
			t.Helper()
			_, err := pauserecord.Update(recordPath, time.Now(), func(record *pauserecord.Record) error {
				record.Live = []pauserecord.Entry{{Scope: scope, PausedAt: time.Now().Add(-time.Minute)}}
				return nil
			})
			if err != nil {
				t.Fatalf("Update() = %v", err)
			}
		}
		tests := []struct {
			name                  string
			scope                 string
			wantPrime, wantPaired bool
		}{
			{"hub entry", pauserecord.ScopeHub, true, true},
			{"pair entry", pairSlug, false, true},
		}
		for _, tt := range tests {
			pause(tt.scope)
			if got, err := delivery.Paused(primeStrand); err != nil || got != tt.wantPrime {
				t.Errorf("Paused(prime strand) under a %s = (%v, %v); want (%v, nil)", tt.name, got, err, tt.wantPrime)
			}
			if got, err := delivery.Paused(pairStrand); err != nil || got != tt.wantPaired {
				t.Errorf("Paused(pair strand) under a %s = (%v, %v); want (%v, nil)", tt.name, got, err, tt.wantPaired)
			}
		}

		if err := os.WriteFile(recordPath, []byte("{not json"), 0o644); err != nil {
			t.Fatalf("WriteFile(%q) = %v", recordPath, err)
		}
		if _, err := delivery.Paused(primeStrand); err == nil {
			t.Error("Paused() on an undecodable pause record = nil error; want one")
		}
	})

	t.Run("ClaimPause records the strand in the entry covering it", func(t *testing.T) {
		t.Parallel()
		hub, prime, pair := newHub(t)
		recordPath := pauseRecordPath(t, prime)
		orchStrand := creelengine.Strand{Name: "ly:orch", GUID: "g1", Worktree: prime}
		scoutStrand := creelengine.Strand{Name: "ly:scout", GUID: "g3", Worktree: prime}
		pairStrand := creelengine.Strand{Name: "ly:pair:impl", GUID: "g2", Worktree: pair, Slug: pairSlug}
		delivery := newStubbedDelivery(hub, nil)

		pausedAt := time.Now().Add(-time.Minute)
		setLive := func(scope string) {
			t.Helper()
			_, err := pauserecord.Update(recordPath, time.Now(), func(record *pauserecord.Record) error {
				*record = pauserecord.Record{Live: []pauserecord.Entry{{Scope: scope, PausedAt: pausedAt}}}
				return nil
			})
			if err != nil {
				t.Fatalf("Update() = %v", err)
			}
		}
		sessions := func() []pauserecord.Session {
			t.Helper()
			record, err := pauserecord.Read(recordPath)
			if err != nil {
				t.Fatalf("Read() = %v", err)
			}
			var found []pauserecord.Session
			for _, entry := range record.Live {
				found = append(found, entry.Sessions...)
			}
			return found
		}
		claim := func(strand creelengine.Strand) creelengine.PauseClaim {
			t.Helper()
			got, err := delivery.ClaimPause(strand)
			if err != nil {
				t.Fatalf("ClaimPause(%s) = %v", strand.Name, err)
			}
			return got
		}

		setLive(pairSlug)
		if got := claim(pairStrand); !got.Covered {
			t.Errorf("ClaimPause(pair strand) under a pair entry = %+v; want covered", got)
		}
		if diff := cmp.Diff([]pauserecord.Session{{Strand: pairStrand.Name, GUID: "g2", Slug: pairSlug}}, sessions()); diff != "" {
			t.Errorf("pair entry sessions mismatch (-want +got):\n%s", diff)
		}
		if got := claim(orchStrand); got.Covered || got.StillLive != nil || len(sessions()) != 1 {
			t.Errorf("ClaimPause(prime strand) under a pair entry = %+v with sessions %v; want uncovered and nothing recorded", got, sessions())
		}

		setLive(pauserecord.ScopeHub)
		claim(orchStrand)
		claim(scoutStrand)
		want := []pauserecord.Session{{GUID: "g1", Orch: true}, {Strand: "ly:scout", GUID: "g3"}}
		if diff := cmp.Diff(want, sessions()); diff != "" {
			t.Errorf("hub entry sessions mismatch (-want +got):\n%s", diff)
		}

		live := claim(pairStrand).StillLive
		if got, err := live(); err != nil || !got {
			t.Errorf("StillLive() on a live entry = (%v, %v); want (true, nil)", got, err)
		}
		_, err := pauserecord.Update(recordPath, time.Now(), func(record *pauserecord.Record) error {
			record.Resumed, record.Live = record.Live, nil
			return nil
		})
		if err != nil {
			t.Fatalf("Update() = %v", err)
		}
		if got, err := live(); err != nil || got {
			t.Errorf("StillLive() on a resumed entry = (%v, %v); want (false, nil)", got, err)
		}

		if err := os.WriteFile(recordPath, []byte("{not json"), 0o644); err != nil {
			t.Fatalf("WriteFile(%q) = %v", recordPath, err)
		}
		if _, err := delivery.ClaimPause(orchStrand); err == nil {
			t.Error("ClaimPause() on an undecodable pause record = nil error; want one")
		}
	})
}

// TestHubDelivery_SkipsAWorktreeThatNoLongerResolves pins that Strands skips a listed worktree that no longer resolves as a worktree and returns the others' strands.
// It logs the first skip of a path at Info for a teardown and at Warn otherwise, and a repeated skip at Debug.
// An unusable recorded anchor, which fails the resolve for another reason than not being a git repository, still fails the call.
// It is not parallel: logcapture.CaptureVerbose swaps the process-global logger, and its rows assert on the captured lines.
func TestHubDelivery_SkipsAWorktreeThatNoLongerResolves(t *testing.T) {
	newHub := func(t *testing.T) (*hubforge.Hub, string, string) {
		hub := hubforge.CopyHub(t, hubforge.Shape{Anchor: ".", Pairs: []string{pairSlug}})
		return hub, hub.PrimeWorktree(), hub.PairCodeWorktree(pairSlug)
	}
	replaceGitEntry := func(t *testing.T, pair string, content *string) {
		t.Helper()
		gitEntry := filepath.Join(pair, ".git")
		if err := os.Remove(gitEntry); err != nil {
			t.Fatalf("Remove(%q) = %v", gitEntry, err)
		}
		if content == nil {
			if err := os.Mkdir(gitEntry, 0o755); err != nil {
				t.Fatalf("Mkdir(%q) = %v", gitEntry, err)
			}
			return
		}
		if err := os.WriteFile(gitEntry, []byte(*content), 0o644); err != nil {
			t.Fatalf("WriteFile(%q) = %v", gitEntry, err)
		}
	}
	gitdirLine := func(dir string) *string {
		line := "gitdir: " + dir + "\n"
		return &line
	}
	countLines := func(output, level string) int {
		count := 0
		for _, line := range strings.Split(output, "\n") {
			if strings.Contains(line, "level="+level) && strings.Contains(line, worktreeGoneMessage) {
				count++
			}
		}
		return count
	}

	tests := []struct {
		name string
		// arrange breaks the pair and returns the stubs of the worktrees.
		arrange      func(t *testing.T, prime, pair string) map[string]*stubOps
		wantLevel    string
		wantInLog    []string
		wantNotInLog []string
		// directoryGone is set when the row removes the worktree's directory: git then lists the worktree prunable, so the second call never reaches its skip.
		directoryGone bool
	}{
		{
			name: "AnEmptyGitDirectoryWarnsAndNamesTheCauseOnly",
			arrange: func(t *testing.T, prime, pair string) map[string]*stubOps {
				replaceGitEntry(t, pair, nil)
				return map[string]*stubOps{prime: {}, pair: {}}
			},
			wantLevel:    "WARN",
			wantInLog:    []string{".git is a directory, not a worktree link"},
			wantNotInLog: []string{"git worktree repair"},
		},
		{
			name: "AGitfileNamingAnExistingEmptyDirectoryWarnsAndNamesTheRepairStep",
			arrange: func(t *testing.T, prime, pair string) map[string]*stubOps {
				replaceGitEntry(t, pair, gitdirLine(t.TempDir()))
				return map[string]*stubOps{prime: {}, pair: {}}
			},
			wantLevel: "WARN",
			wantInLog: []string{"git worktree repair PAIR"},
		},
		{
			name: "AGitfileNamingAMissingDirectoryIsATeardownAtInfo",
			arrange: func(t *testing.T, prime, pair string) map[string]*stubOps {
				replaceGitEntry(t, pair, gitdirLine(filepath.Join(t.TempDir(), "gone")))
				return map[string]*stubOps{prime: {}, pair: {}}
			},
			wantLevel:    "INFO",
			wantNotInLog: []string{"git worktree repair"},
		},
		{
			name: "ADirectoryRemovedBetweenTheResolveAndTheStatusIsATeardownAtInfo",
			arrange: func(t *testing.T, prime, pair string) map[string]*stubOps {
				seedReedState(t, pair, map[string]string{"ly:pair:impl": "guid-impl"})
				return map[string]*stubOps{
					prime: {},
					pair: {
						statusErr: errors.New("tmux exploded"),
						onStatus: func() {
							if err := os.RemoveAll(pair); err != nil {
								t.Errorf("RemoveAll(%q) = %v", pair, err)
							}
						},
					},
				}
			},
			wantLevel: "INFO", directoryGone: true,
		},
		{
			name: "ADirectoryRemovedWhileWiringIsATeardownAtInfo",
			arrange: func(t *testing.T, prime, pair string) map[string]*stubOps {
				seedReedState(t, pair, map[string]string{"ly:pair:impl": "guid-impl"})
				return map[string]*stubOps{
					prime: {},
					pair: {
						wireErr: errors.New("wire failed"),
						onWire: func() {
							if err := os.RemoveAll(pair); err != nil {
								t.Errorf("RemoveAll(%q) = %v", pair, err)
							}
						},
					},
				}
			},
			wantLevel: "INFO", directoryGone: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			hub, prime, pair := newHub(t)
			seedReedState(t, prime, map[string]string{"ly:orch": "guid-orch"})
			stubs := tt.arrange(t, prime, pair)
			stubs[prime] = &stubOps{status: reedengine.StatusResult{Strands: []reedengine.StrandStatus{{GUID: "guid-orch", Live: true, PanePID: 11}}}}
			delivery := newStubbedDelivery(hub, stubs)
			logs := logcapture.CaptureVerbose(t)
			logger.SetVerbosity(2)

			for call := 1; call <= 2; call++ {
				got, err := delivery.Strands()
				if err != nil {
					t.Fatalf("Strands() call %d = %v; want the skip to leave the call whole", call, err)
				}
				want := []creelengine.Strand{{Name: "ly:orch", GUID: "guid-orch", Worktree: prime, Live: true, PanePID: 11}}
				if diff := cmp.Diff(want, got); diff != "" {
					t.Errorf("Strands() call %d mismatch (-want +got):\n%s", call, diff)
				}
			}

			output := logs.String()
			for _, level := range []string{"WARN", "INFO"} {
				wantCount := 0
				if level == tt.wantLevel {
					wantCount = 1
				}
				if got := countLines(output, level); got != wantCount {
					t.Errorf("%s skip lines over two calls = %d; want %d\nlog: %s", level, got, wantCount, output)
				}
			}
			wantDebug := 1
			if tt.directoryGone {
				wantDebug = 0
			}
			if got := countLines(output, "DEBUG"); got != wantDebug {
				t.Errorf("DEBUG skip lines over two calls = %d; want %d", got, wantDebug)
			}
			for _, want := range tt.wantInLog {
				if want = strings.ReplaceAll(want, "PAIR", pair); !strings.Contains(output, want) {
					t.Errorf("log lacks %q\nlog: %s", want, output)
				}
			}
			for _, unwanted := range tt.wantNotInLog {
				if strings.Contains(output, unwanted) {
					t.Errorf("log holds %q, want it absent\nlog: %s", unwanted, output)
				}
			}
		})
	}

	t.Run("AnUnusableRecordedAnchorFailsTheCall", func(t *testing.T) {
		hub, prime, pair := newHub(t)
		anchorFile := filepath.Join(hub.PrimeRecords(), lymcwd.AnchorFileName)
		if err := os.WriteFile(anchorFile, []byte("../escape\n"), 0o644); err != nil {
			t.Fatalf("WriteFile(%q) = %v", anchorFile, err)
		}
		delivery := newStubbedDelivery(hub, map[string]*stubOps{prime: {}, pair: {}})

		_, err := delivery.Strands()
		if !errors.Is(err, lymcwd.ErrInvalidAnchor) || errors.Is(err, errWorktreeGone) {
			t.Errorf("Strands() with an unusable anchor = %v; want a failure wrapping ErrInvalidAnchor and not a skip", err)
		}
	})
}

// sortStrands orders strands by name, since the seeded states hold them in map order.
func sortStrands(strands []creelengine.Strand) {
	sort.Slice(strands, func(i, j int) bool { return strands[i].Name < strands[j].Name })
}
