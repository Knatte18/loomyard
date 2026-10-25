//go:build tier0

// start_test.go drives the start verb's RunE over a fake strandOps, session starter and watcher spawn, asserting the recorded calls and the saved state without a spawn.

package orchcli

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/boardengine"
	"github.com/Knatte18/loomyard/internal/clihelp"
	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/hubgeom"
	"github.com/Knatte18/loomyard/internal/lock"
	"github.com/Knatte18/loomyard/internal/lyxcwd"
	"github.com/Knatte18/loomyard/internal/lyxdirs"
	"github.com/Knatte18/loomyard/internal/orchengine"
	"github.com/Knatte18/loomyard/internal/pauserecord"
	"github.com/Knatte18/loomyard/internal/reedengine"
	"github.com/Knatte18/loomyard/internal/reedengine/render"
	"github.com/Knatte18/loomyard/internal/segmentcolor"
	"github.com/Knatte18/loomyard/internal/shell"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
	"github.com/Knatte18/loomyard/internal/statuspane"
	"github.com/Knatte18/loomyard/internal/testkit/locationkit"
	"github.com/Knatte18/loomyard/internal/testkit/stencilkit"
	"github.com/spf13/cobra"
)

// fakeStarter records the specs it is asked to start and answers a fixed guid and warning, and the resumed session id or else sessionID.
type fakeStarter struct {
	specs     []shuttleengine.Spec
	guid      string
	sessionID string
	warning   string
	err       error
	// calls, when set, receives "start" as each session starts.
	calls *[]string
}

func (f *fakeStarter) StartSession(spec shuttleengine.Spec) (startedSession, error) {
	if f.calls != nil {
		*f.calls = append(*f.calls, "start")
	}
	f.specs = append(f.specs, spec)
	sessionID := f.sessionID
	if spec.ResumeSessionID != "" {
		sessionID = spec.ResumeSessionID
	}
	return startedSession{GUID: f.guid, SessionID: sessionID, Warning: f.warning}, f.err
}

// scriptedCreel is a recording fake of creel whose withdraws and releases answer the errors and withdrawn messages a test scripts, one per call in call order.
// Every call is recorded as "<method>:<receiver>" in the fake's own list and in order, the harness's order of start's calls.
type scriptedCreel struct {
	*fakeCreel
	order        *[]string
	withdrawErrs []error
	releaseErrs  []error
	withdrawn    [][]orchengine.CreelWithdrawn
	// onCall, when set, runs before each withdraw and release is answered.
	onCall func()
}

func (s *scriptedCreel) Withdraw(receiver string) ([]orchengine.CreelWithdrawn, error) {
	*s.order = append(*s.order, "withdraw:"+receiver)
	if s.onCall != nil {
		s.onCall()
	}
	if _, err := s.fakeCreel.Withdraw(receiver); err != nil {
		return nil, err
	}
	if err := popError(&s.withdrawErrs); err != nil {
		return nil, err
	}
	if len(s.withdrawn) == 0 {
		return nil, nil
	}
	withdrawn := s.withdrawn[0]
	s.withdrawn = s.withdrawn[1:]
	return withdrawn, nil
}

func (s *scriptedCreel) Release(receiver string) error {
	*s.order = append(*s.order, "release:"+receiver)
	if s.onCall != nil {
		s.onCall()
	}
	if err := s.fakeCreel.Release(receiver); err != nil {
		return err
	}
	return popError(&s.releaseErrs)
}

// popError removes and returns the first scripted error, nil when none is left.
func popError(errs *[]error) error {
	if len(*errs) == 0 {
		return nil
	}
	err := (*errs)[0]
	*errs = (*errs)[1:]
	return err
}

// startHarness bundles a receiver with its fakes.
type startHarness struct {
	// root is the command tree the last run executed.
	root    *cobra.Command
	cli     *orchCLI
	strands *fakeStrands
	starter *fakeStarter
	creel   *scriptedCreel
	spawns  int
	// sleeps counts the waits start asked for;
	// onSleep, when set, runs on each.
	sleeps  int
	onSleep func()
	// calls is the order of start's adds, replaces, removes, session starts and creel withdraws and releases.
	calls []string
	// exe and build are what the receiver's executable and currentBuild answer; a test changes them before it runs start.
	exe   string
	build statuspane.BuildIdentity
	// reconciles counts the reconcile calls and callsAtReconcile is how many of calls had run when the last one came;
	// report and reconcileErr are what the fake reconcile answers.
	reconciles       int
	callsAtReconcile int
	report           fabricengine.OriginReport
	reconcileErr     error
	// claims, when set, wires the prime claim to answer each call with the next entry, the last one repeating; claimCalls counts the calls.
	claims     []claimAnswer
	claimCalls int
	// pauseOwned, when set, wires the owner pause to fail with pauseOwnedErr, record each slug in ownerPaused and count the calls callsAtOwnerPause saw of the strand operations.
	pauseOwned        bool
	pauseOwnedErr     error
	ownerPaused       []string
	callsAtOwnerPause int
}

// claimAnswer is one scripted answer of the fake prime claim.
type claimAnswer struct {
	claimed, lostTo string
	err             error
}

func newStartHarness(t *testing.T, strands ...reedengine.StrandStatus) *startHarness {
	t.Helper()
	fake := &fakeStrands{strands: strands}
	c := newTestCLI(t, fake)
	c.paths.StartLockPath = filepath.Join(c.paths.Dir, "start.lock")
	c.paths.HandoffsDir = filepath.Join(c.paths.Dir, "handoffs")
	c.paths.RolePath = filepath.Join(c.paths.Dir, "role.md")
	c.paths.NoteTemplatePath = filepath.Join(c.paths.Dir, "note-template.md")
	c.cfg.Model, c.cfg.Effort = "opus", "high"
	c.stencilsDir = stencilkit.Seed(t)
	c.location = locationkit.Location(t.TempDir(), "main", "")
	c.runsWindowTitle = "beam: prime"
	boardDir := fabricengine.BoardDir(c.location.HubPath)
	if err := os.MkdirAll(boardDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(boardDir, fabricengine.ShortnameFileName), []byte("tst"), 0o644); err != nil {
		t.Fatal(err)
	}
	h := &startHarness{
		cli: c, strands: fake, starter: &fakeStarter{guid: "new-guid", sessionID: "minted-id"},
		exe:   "/fixed/lyx",
		build: statuspane.BuildIdentity{Path: "/fixed/lyx", Size: 1, ModTime: 2},
	}
	fake.calls, h.starter.calls = &h.calls, &h.calls
	h.creel = &scriptedCreel{fakeCreel: &fakeCreel{}, order: &h.calls}
	c.newCreel = func() (orchengine.Creel, error) { return h.creel, nil }
	c.executable = func() (string, error) { return h.exe, nil }
	c.currentBuild = func() (statuspane.BuildIdentity, error) { return h.build, nil }
	c.starter = h.starter
	c.reedUp = func() error { return nil }
	c.reconcile = func() (fabricengine.OriginReport, error) {
		h.reconciles++
		h.callsAtReconcile = len(h.calls)
		return h.report, h.reconcileErr
	}
	c.spawnWatcher = func() error { h.spawns++; return nil }
	c.sleep = func(time.Duration) {
		h.sleeps++
		if h.onSleep != nil {
			h.onSleep()
		}
	}
	return h
}

// run executes start with args and decodes the envelope.
func (h *startHarness) run(t *testing.T, args ...string) (int, map[string]any) {
	t.Helper()
	var out bytes.Buffer
	// start renders the operator index of its root, so the verb runs under a root that has it as a child.
	if len(h.claims) > 0 {
		h.cli.claimPrime = func() (string, string, error) {
			answer := h.claims[min(h.claimCalls, len(h.claims)-1)]
			h.claimCalls++
			return answer.claimed, answer.lostTo, answer.err
		}
	}
	if h.pauseOwned {
		h.cli.pauseOwned = func(_ *lyxcwd.Location, slug string) error {
			h.callsAtOwnerPause = len(h.calls)
			if h.pauseOwnedErr != nil {
				return h.pauseOwnedErr
			}
			h.ownerPaused = append(h.ownerPaused, slug)
			return nil
		}
	}
	h.root = &cobra.Command{Use: "lyx"}
	h.root.AddCommand(h.cli.startCmd())
	code := clihelp.Execute(h.root, &out, append([]string{"start"}, args...))
	var env map[string]any
	if err := json.Unmarshal(out.Bytes(), &env); err != nil {
		t.Fatalf("output %q is not one JSON object: %v", out.String(), err)
	}
	return code, env
}

func (h *startHarness) state(t *testing.T) orchengine.State {
	t.Helper()
	st, err := orchengine.LoadState(h.cli.paths)
	if err != nil {
		t.Fatal(err)
	}
	return st
}

// The creel calls start makes on the hub's fixed orch strand name, as the harness records them.
const (
	withdrawOrch = "withdraw:tst:orch"
	releaseOrch  = "release:tst:orch"
)

func TestStart_EnsuresTheRunsStrandBeforeTheOrch(t *testing.T) {
	t.Parallel()

	otherBuild := statuspane.BuildIdentity{Path: "/older/lyx", Size: 9, ModTime: 9}
	wantDisplay := render.Display{Anchor: render.AnchorOverview, Title: "beam: prime"}
	tests := []struct {
		name    string
		strands []reedengine.StrandStatus
		// sidecarGUID and sidecarBuild, when the guid is set, are the sidecar written before start runs.
		sidecarGUID  string
		sidecarBuild *statuspane.BuildIdentity
		addErr       error
		wantCalls    []string
		// wantSidecarGUID is the guid the sidecar holds after start, empty for none.
		wantSidecarGUID string
	}{
		{
			name:            "no runs strand adds one before the orch starts",
			wantCalls:       []string{"add", withdrawOrch, "start", withdrawOrch, releaseOrch},
			wantSidecarGUID: "added-runs",
		},
		{
			name:            "a live runs strand of this build is kept",
			strands:         []reedengine.StrandStatus{{GUID: "r1", Name: "runs", Live: true}},
			sidecarGUID:     "r1",
			wantCalls:       []string{withdrawOrch, "start", withdrawOrch, releaseOrch},
			wantSidecarGUID: "r1",
		},
		{
			name:            "a live runs strand of another build is replaced",
			strands:         []reedengine.StrandStatus{{GUID: "r1", Name: "runs", Live: true}},
			sidecarGUID:     "r1",
			sidecarBuild:    &otherBuild,
			wantCalls:       []string{"replace", withdrawOrch, "start", withdrawOrch, releaseOrch},
			wantSidecarGUID: "replaced-runs",
		},
		{
			name:            "a dead runs strand is replaced",
			strands:         []reedengine.StrandStatus{{GUID: "r1", Name: "runs"}},
			sidecarGUID:     "r1",
			wantCalls:       []string{"replace", withdrawOrch, "start", withdrawOrch, releaseOrch},
			wantSidecarGUID: "replaced-runs",
		},
		{
			name:      "a failing add leaves start a success with the orch launched",
			addErr:    errors.New("reed refused"),
			wantCalls: []string{"add", withdrawOrch, "start", withdrawOrch, releaseOrch},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newStartHarness(t, tt.strands...)
			h.cli.cfg.PermissionMode = "bypass"
			h.strands.addErr = tt.addErr
			sidecarPath := filepath.Join(h.cli.paths.Dir, runsSidecarFile)
			if tt.sidecarGUID != "" {
				build := h.build
				if tt.sidecarBuild != nil {
					build = *tt.sidecarBuild
				}
				if err := statuspane.WriteSidecar(sidecarPath, tt.sidecarGUID, build); err != nil {
					t.Fatal(err)
				}
			}

			if code, env := h.run(t); code != 0 || env["ok"] != true {
				t.Fatalf("start = exit %d, %v; want a success", code, env)
			}

			if !slices.Equal(h.calls, tt.wantCalls) {
				t.Errorf("calls = %v; want %v", h.calls, tt.wantCalls)
			}
			var specs []reedengine.AddSpec
			for _, added := range h.strands.added {
				specs = append(specs, added.spec)
			}
			for _, replaced := range h.strands.replaced {
				specs = append(specs, replaced.spec)
				if replaced.replaced != "r1" {
					t.Errorf("replaced guid = %q; want r1", replaced.replaced)
				}
			}
			for _, spec := range specs {
				if spec.NameOverride != "runs" || spec.Segment != "" || spec.Display != wantDisplay {
					t.Errorf("runs strand spec = %+v; want the runs name, no segment and the overview display titled after the beam", spec)
				}
				sh := shell.ForGOOS()
				if want := sh.Invoke(h.exe) + " " + sh.Quote("batten") + " " + sh.Quote("overview"); spec.Cmd != want {
					t.Errorf("runs strand command = %q; want %q", spec.Cmd, want)
				}
			}

			sidecar := statuspane.ReadSidecar(sidecarPath)
			switch {
			case tt.wantSidecarGUID == "" && sidecar != nil:
				t.Errorf("sidecar = %+v; want none", sidecar)
			case tt.wantSidecarGUID != "" && (sidecar == nil || sidecar.GUID != tt.wantSidecarGUID || sidecar.Build != h.build):
				t.Errorf("sidecar = %+v; want guid %q with the current build", sidecar, tt.wantSidecarGUID)
			}
		})
	}
}

func TestRunsWindowTitleOf(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		worktree string
		beam     bool
		want     string
	}{
		{"PrimeReadsPrime", "main", false, "beam: prime"},
		{"BeamReadsItsSlug", "workspaces", true, "beam: workspaces"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := runsWindowTitleOf(tt.worktree, tt.beam); got != tt.want {
				t.Errorf("runsWindowTitleOf(%q, %v) = %q; want %q", tt.worktree, tt.beam, got, tt.want)
			}
		})
	}
}

func TestStart_NoStrandLaunchesAndSpawnsWatcher(t *testing.T) {
	t.Parallel()

	h := newStartHarness(t)
	h.cli.cfg.PermissionMode = "bypass"
	code, env := h.run(t)

	if code != 0 {
		t.Fatalf("exit = %d; env = %v", code, env)
	}
	if len(h.starter.specs) != 1 || h.spawns != 1 {
		t.Fatalf("starts = %d, spawns = %d; want 1 and 1", len(h.starter.specs), h.spawns)
	}
	spec := h.starter.specs[0]
	if !strings.Contains(spec.Prompt, h.cli.paths.RolePath) {
		t.Errorf("prompt = %q; want the start pointer at the role file", spec.Prompt)
	}
	wantIndex := clihelp.RenderIndex(h.root, clihelp.AudienceOperator)
	role, err := os.ReadFile(h.cli.paths.RolePath)
	if err != nil {
		t.Fatalf("role file not rendered before launch: %v", err)
	}
	if wantIndex == "" || !strings.Contains(string(role), "## Commands") || !strings.Contains(string(role), wantIndex) {
		t.Errorf("role file = %q; want a Commands section carrying the root's operator index %q", role, wantIndex)
	}
	if st := h.state(t); st.Strand != "new-guid" || st.Phase != orchengine.PhaseIdle {
		t.Errorf("state = %+v; want strand new-guid in idle", st)
	}
	if env["action"] != actionRelaunched || env["strand"] != "new-guid" || env["prompt_source"] != orchengine.SourceFresh {
		t.Errorf("envelope = %v", env)
	}

	if spec.PermissionMode != "bypass" {
		t.Errorf("PermissionMode = %q; want bypass", spec.PermissionMode)
	}
	if !spec.AllowAgentTool || !spec.ForkSubagents {
		t.Errorf("AllowAgentTool/ForkSubagents = %v/%v; want both true", spec.AllowAgentTool, spec.ForkSubagents)
	}
	if !spec.Interactive || spec.NameOverride != "orch" || spec.Role != "orch" || !spec.Display.Focus {
		t.Errorf("spec = %+v; want interactive, focused, named orch", spec)
	}
	if spec.Display.Anchor != render.AnchorOwnWindow {
		t.Errorf("Display.Anchor = %q; want %q, so the orch gets a window of its own", spec.Display.Anchor, render.AnchorOwnWindow)
	}
	if spec.Segment != segmentcolor.Coordinator || !spec.ColorByCaller {
		t.Errorf("Segment/ColorByCaller = %q/%v; want %q/true", spec.Segment, spec.ColorByCaller, segmentcolor.Coordinator)
	}
	if !slices.Equal(spec.Skills, []string{"scribe:prose", "scribe:conversation", "ly:board"}) {
		t.Errorf("Skills = %v; want the three orch skills", spec.Skills)
	}
	if spec.SkillLoadTimeout != h.cli.cfg.HandoffTimeout() {
		t.Errorf("SkillLoadTimeout = %v; want the handoff timeout %v", spec.SkillLoadTimeout, h.cli.cfg.HandoffTimeout())
	}
	if want := "cd '" + h.cli.location.AnchorPath() + "'; lyx orch resume-context"; spec.ContextAfterCompaction != want {
		t.Errorf("ContextAfterCompaction = %q; want %q", spec.ContextAfterCompaction, want)
	}
	if spec.Model != "opus" || spec.Effort != "high" {
		t.Errorf("model/effort = %q/%q; want opus/high", spec.Model, spec.Effort)
	}
	if len(spec.OutputFiles) != 1 || filepath.Dir(spec.OutputFiles[0]) != h.cli.paths.Dir || !strings.HasSuffix(spec.OutputFiles[0], ".never") {
		t.Errorf("output files = %v; want one session-*.never sentinel under %s", spec.OutputFiles, h.cli.paths.Dir)
	}

	// An existing strand is not listed in these rows unless strands says so, and the fixed name addresses creel either way.
	boom := errors.New("creel: cannot reach the daemon")
	corpse := []reedengine.StrandStatus{{GUID: "corpse", Name: "tst:orch"}}
	rounds := []struct {
		name         string
		strands      []reedengine.StrandStatus
		args         []string
		withdrawErrs []error
		releaseErrs  []error
		wantCalls    []string
		// wantFailureIn is the text the recorded fresh_launch_failure holds, empty for none.
		wantFailureIn string
	}{
		{
			name:      "a relaunch withdraws and releases before removing the old strand and starting, then again after saving",
			strands:   corpse,
			wantCalls: []string{"add", withdrawOrch, releaseOrch, "remove", "start", withdrawOrch, releaseOrch},
		},
		{
			name:      "a continue after stop with no strand listed sends no first release, starts and records no failure",
			args:      []string{"--continue"},
			wantCalls: []string{"add", withdrawOrch, "start", withdrawOrch, releaseOrch},
		},
		{
			name:          "a first release that fails records the failure and start succeeds",
			strands:       corpse,
			releaseErrs:   []error{boom},
			wantCalls:     []string{"add", withdrawOrch, releaseOrch, "remove", "start", withdrawOrch, releaseOrch},
			wantFailureIn: "release before the fresh launch: " + boom.Error(),
		},
		{
			name:          "a second withdraw that fails records the failure and start succeeds",
			strands:       corpse,
			withdrawErrs:  []error{nil, boom},
			wantCalls:     []string{"add", withdrawOrch, releaseOrch, "remove", "start", withdrawOrch, releaseOrch},
			wantFailureIn: "withdraw after the fresh launch: " + boom.Error(),
		},
		{
			name:          "a second release that fails records the failure and start succeeds",
			releaseErrs:   []error{boom},
			wantCalls:     []string{"add", withdrawOrch, "start", withdrawOrch, releaseOrch},
			wantFailureIn: "release after the fresh launch: " + boom.Error(),
		},
	}
	for _, tt := range rounds {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newStartHarness(t, tt.strands...)
			h.creel.withdrawErrs, h.creel.releaseErrs = tt.withdrawErrs, tt.releaseErrs
			args := tt.args
			if slices.Contains(args, "--continue") {
				saveSeedState(t, h, orchengine.State{Phase: orchengine.PhaseIdle, Strand: "g1", SessionID: adoptTestSessionID})
			}

			if code, env := h.run(t, args...); code != 0 || env["ok"] != true {
				t.Fatalf("start = exit %d, %v; want a success", code, env)
			}
			if !slices.Equal(h.calls, tt.wantCalls) {
				t.Errorf("calls = %v; want %v", h.calls, tt.wantCalls)
			}
			failure := h.state(t).FreshLaunchFailure
			if tt.wantFailureIn == "" && failure != "" || !strings.Contains(failure, tt.wantFailureIn) {
				t.Errorf("fresh_launch_failure = %q; want one holding %q", failure, tt.wantFailureIn)
			}
		})
	}

	t.Run("a withdrawn resume line clears the noticed mark of the entry owning the old recorded GUID only", func(t *testing.T) {
		t.Parallel()

		h := newStartHarness(t, corpse...)
		// PauseGeometry tells the prime from a task worktree by its .git entry.
		if err := os.MkdirAll(filepath.Join(h.cli.location.WorktreePath(), ".git"), 0o755); err != nil {
			t.Fatal(err)
		}
		geometry, err := hubgeom.PauseGeometry(h.cli.location)
		if err != nil {
			t.Fatal(err)
		}
		now := time.Now()
		entryOf := func(guid string, pausedAt time.Time) pauserecord.Entry {
			return pauserecord.Entry{
				Scope: pauserecord.ScopeHub, PausedAt: pausedAt, ResumedAt: now,
				Sessions: []pauserecord.Session{{GUID: guid, Orch: true, ResumeNoticed: true}},
			}
		}
		if _, err := pauserecord.Update(geometry.RecordPath, now, func(r *pauserecord.Record) error {
			r.Resumed = append(r.Resumed, entryOf("old-guid", now.Add(-2*time.Minute)), entryOf("other-guid", now.Add(-time.Minute)))
			return nil
		}); err != nil {
			t.Fatal(err)
		}
		saveSeedState(t, h, orchengine.State{Phase: orchengine.PhaseIdle, Strand: "old-guid"})
		h.creel.withdrawn = [][]orchengine.CreelWithdrawn{{{ID: 5, Subject: pauserecord.ResumeLine}}}

		if code, env := h.run(t); code != 0 {
			t.Fatalf("start = exit %d, %v; want a success", code, env)
		}
		record, err := pauserecord.Read(geometry.RecordPath)
		if err != nil {
			t.Fatal(err)
		}
		noticed := map[string]bool{}
		for _, entry := range record.Resumed {
			noticed[entry.Sessions[0].GUID] = entry.Sessions[0].ResumeNoticed
		}
		if noticed["old-guid"] || !noticed["other-guid"] {
			t.Errorf("noticed marks = %v; want the old GUID's cleared and the other kept", noticed)
		}
	})
}

func TestStart_LiveStrand(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		// watcher holds watch.lock;
		// stopping records the watcher stopping;
		// releases frees the lock on the first sleep.
		watcher, stopping, releases bool
		wantAction                  string
		wantSpawns                  int
		wantSleeps                  int
		wantStopping                bool
		// seed, when set, is the state saved before start runs, replacing the one stopping seeds.
		seed *orchengine.State
		// wantSessionID is the session id the saved state holds after start.
		wantSessionID string
		// wantCreelCalls are the creel calls start made, none for a strand the state already names.
		wantCreelCalls []string
		// wantStrandAtCalls is the strand the persisted state names at each of those calls, which shows the rounds run around the reset.
		wantStrandAtCalls []string
		// withdrawErr fails the first withdraw: start then binds nothing and reports creel's error.
		withdrawErr error
	}{
		{
			name: "with a watcher does nothing", watcher: true, wantAction: actionAlreadyRunning,
			wantCreelCalls:    []string{"withdraw:orch", "release:orch", "withdraw:orch", "release:orch"},
			wantStrandAtCalls: []string{"", "", "g1", "g1"},
		},
		{
			name: "without a watcher spawns the watcher only", wantAction: actionSpawnedWatcher, wantSpawns: 1,
			wantCreelCalls:    []string{"withdraw:orch", "release:orch", "withdraw:orch", "release:orch"},
			wantStrandAtCalls: []string{"", "", "g1", "g1"},
		},
		{name: "a stopping watcher that exits is replaced", watcher: true, stopping: true, releases: true, wantAction: actionSpawnedWatcher, wantSpawns: 1, wantSleeps: 1, wantSessionID: "g1-id"},
		{name: "a stopping watcher that never exits is reported live", watcher: true, stopping: true, wantAction: actionAlreadyRunning, wantSleeps: 3, wantStopping: true, wantSessionID: "g1-id"},
		{
			name:              "a live strand other than the recorded one clears the session id, with a round of creel calls on each side of the reset",
			seed:              &orchengine.State{Phase: orchengine.PhaseIdle, Strand: "old", SessionID: "old-id"},
			wantAction:        actionSpawnedWatcher,
			wantSpawns:        1,
			wantCreelCalls:    []string{"withdraw:orch", "release:orch", "withdraw:orch", "release:orch"},
			wantStrandAtCalls: []string{"old", "old", "g1", "g1"},
		},
		{
			name:        "a live strand other than the recorded one binds nothing when the first withdraw fails",
			seed:        &orchengine.State{Phase: orchengine.PhaseIdle, Strand: "old", SessionID: "old-id"},
			withdrawErr: errors.New("creel: the daemon did not start"),
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()

			h := newStartHarness(t, reedengine.StrandStatus{GUID: "g1", Name: "orch", Live: true})
			seed := c.seed
			if c.stopping {
				seed = &orchengine.State{Phase: orchengine.PhaseIdle, Strand: "g1", SessionID: "g1-id", WatcherStopping: true}
			}
			if seed != nil {
				if err := orchengine.SaveState(h.cli.paths, *seed); err != nil {
					t.Fatal(err)
				}
			}
			if c.watcher {
				if err := os.MkdirAll(h.cli.paths.Dir, 0o755); err != nil {
					t.Fatal(err)
				}
				l, err := lock.AcquireWriteLock(h.cli.paths.WatchLockPath)
				if err != nil {
					t.Fatal(err)
				}
				released := false
				release := func() {
					if !released {
						released = true
						l.Release()
					}
				}
				defer release()
				if c.releases {
					h.onSleep = release
				}
			}

			var strandAtCalls []string
			h.creel.onCall = func() { strandAtCalls = append(strandAtCalls, h.state(t).Strand) }
			if c.withdrawErr != nil {
				h.creel.withdrawErrs = []error{c.withdrawErr}
				before := h.state(t)

				code, env := h.run(t)
				if msg, _ := env["error"].(string); code == 0 || !strings.Contains(msg, c.withdrawErr.Error()) {
					t.Fatalf("exit = %d; env = %v; want creel's error", code, env)
				}
				if st := h.state(t); !reflect.DeepEqual(st, before) || h.spawns != 0 {
					t.Errorf("state = %+v, spawns = %d; want the state %+v unchanged and no watcher", st, h.spawns, before)
				}
				return
			}

			code, env := h.run(t)
			if code != 0 || env["action"] != c.wantAction {
				t.Fatalf("exit = %d; env = %v", code, env)
			}
			if !slices.Equal(h.creel.fakeCreel.calls, c.wantCreelCalls) || !slices.Equal(strandAtCalls, c.wantStrandAtCalls) {
				t.Errorf("creel calls = %v on strands %v; want %v on strands %v", h.creel.fakeCreel.calls, strandAtCalls, c.wantCreelCalls, c.wantStrandAtCalls)
			}
			if len(h.starter.specs) != 0 || h.spawns != c.wantSpawns {
				t.Errorf("starts = %d, spawns = %d; want 0 and %d", len(h.starter.specs), h.spawns, c.wantSpawns)
			}
			if h.sleeps != c.wantSleeps {
				t.Errorf("sleeps = %d; want %d", h.sleeps, c.wantSleeps)
			}
			if c.wantStopping {
				if env["watcher_stopping"] != true || env["hint"] != stoppingWatcherHint {
					t.Errorf("envelope = %v; want watcher_stopping true and the hint", env)
				}
			} else if _, has := env["watcher_stopping"]; has {
				t.Errorf("envelope = %v; want no watcher_stopping", env)
			}
			if !c.watcher || c.releases {
				if st := h.state(t); st.Strand != "g1" || st.SessionID != c.wantSessionID {
					t.Errorf("state strand = %q, session id = %q; want the adopted g1 and %q", st.Strand, st.SessionID, c.wantSessionID)
				}
			}
		})
	}
}

// TestStart_NeverAttaches is not parallel: it sets the process-global TMUX with t.Setenv.
func TestStart_NeverAttaches(t *testing.T) {
	t.Setenv("TMUX", "/tmp/tmux-1000/default,1234,0")

	for _, args := range [][]string{nil, {"--no-attach"}} {
		h := newStartHarness(t)
		code, env := h.run(t, args...)
		if code != 0 || env["ok"] != true || env["action"] != actionRelaunched || env["strand"] != "new-guid" {
			t.Errorf("start %v with TMUX set: exit = %d; env = %v; want exit 0 and the success envelope", args, code, env)
		}
	}
}

func TestStart_DeadStrandRemovedBeforeStart(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		strand reedengine.StrandStatus
		// recordedSession is the session id saved in the state; empty saves none.
		recordedSession string
		args            []string
		// wantResume is the session id the launch resumes.
		wantResume string
	}{
		{name: "ADeadStrand", strand: reedengine.StrandStatus{GUID: "corpse", Name: "orch", Live: false}},
		{
			// A shell-only orch past the grace reads present, in its own window, and not live.
			name:            "AShellOnlyStrandIsRelaunchedByContinueFromTheRecordedSession",
			strand:          reedengine.StrandStatus{GUID: "corpse", Name: "orch", Live: false, Present: true, OwnWindow: true},
			recordedSession: "sess-9", args: []string{"--continue"}, wantResume: "sess-9",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newStartHarness(t, tt.strand)
			if tt.recordedSession != "" {
				if err := orchengine.SaveState(h.cli.paths, orchengine.State{Strand: "corpse", Phase: orchengine.PhaseIdle, SessionID: tt.recordedSession}); err != nil {
					t.Fatal(err)
				}
			}
			removedBeforeStart := false
			var launched shuttleengine.Spec
			h.cli.starter = starterFunc(func(spec shuttleengine.Spec) (startedSession, error) {
				removedBeforeStart = len(h.strands.removed) == 1 && h.strands.removed[0] == "corpse"
				launched = spec
				return startedSession{GUID: "new-guid", SessionID: "minted-id"}, nil
			})

			if code, env := h.run(t, tt.args...); code != 0 {
				t.Fatalf("exit = %d; env = %v", code, env)
			}
			if !removedBeforeStart {
				t.Error("the corpse strand was not removed before StartSession")
			}
			if len(h.strands.removed) != 1 || launched.ResumeSessionID != tt.wantResume {
				t.Errorf("removed = %v, resumed session = %q; want one removal and %q", h.strands.removed, launched.ResumeSessionID, tt.wantResume)
			}
		})
	}
}

// starterFunc adapts a function to sessionStarter.
type starterFunc func(shuttleengine.Spec) (startedSession, error)

func (f starterFunc) StartSession(spec shuttleengine.Spec) (startedSession, error) { return f(spec) }

func TestStart_ResumesFromHandoffFile(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name       string
		strands    []reedengine.StrandStatus
		fromState  bool
		wantSource string
	}{
		{"dead strand with a last handoff", []reedengine.StrandStatus{{GUID: "corpse", Name: "orch", Live: false}}, true, orchengine.SourceLastHandoff},
		{"handoff flag", nil, false, orchengine.SourceFlag},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()

			h := newStartHarness(t, c.strands...)
			handoff := filepath.Join(t.TempDir(), "handoff.md")
			if err := os.WriteFile(handoff, []byte("h"), 0o644); err != nil {
				t.Fatal(err)
			}
			var args []string
			if c.fromState {
				if err := orchengine.SaveState(h.cli.paths, orchengine.State{Phase: orchengine.PhaseIdle, Strand: "corpse", LastHandoff: handoff}); err != nil {
					t.Fatal(err)
				}
			} else {
				args = []string{"--handoff", handoff}
			}

			code, env := h.run(t, args...)
			if code != 0 || env["prompt_source"] != c.wantSource {
				t.Fatalf("exit = %d; env = %v", code, env)
			}
			if !strings.Contains(h.starter.specs[0].Prompt, handoff) {
				t.Errorf("prompt = %q; want it naming %s", h.starter.specs[0].Prompt, handoff)
			}
		})
	}
}

func TestStart_FreshLaunchResetsAbandonedPhase(t *testing.T) {
	t.Parallel()

	h := newStartHarness(t)
	if err := orchengine.SaveState(h.cli.paths, orchengine.State{Phase: orchengine.PhaseClearing, Strand: "old", PhaseEventsOffset: 9}); err != nil {
		t.Fatal(err)
	}

	if code, env := h.run(t); code != 0 {
		t.Fatalf("exit = %d; env = %v", code, env)
	}
	st := h.state(t)
	if st.Phase != orchengine.PhaseIdle || st.Strand != "new-guid" || st.PhaseEventsOffset != 0 ||
		!strings.Contains(st.LastAbortReason, string(orchengine.PhaseClearing)) {
		t.Errorf("state = %+v; want idle, new-guid, zero offset and the abandoned clearing recorded", st)
	}
	if len(h.starter.specs) != 1 {
		t.Errorf("starts = %d; want the launch prompt alone", len(h.starter.specs))
	}
}

const adoptTestSessionID = "11111111-2222-3333-4444-555555555555"

func TestPrimePaths_JoinsRuleFilesAndPrimeRoot(t *testing.T) {
	t.Parallel()

	hub := t.TempDir()
	got := PrimePaths(locationkit.Location(hub, "main", "sub/dir"))
	want := orchengine.Paths{
		HubRulesPath: filepath.Join(fabricengine.BoardDir(hub), lyxdirs.LyxDirName, "orch", "rules.md"),
		OwnRulesPath: filepath.Join(hub, "main", "sub", "dir", lyxdirs.LyxDirName, "orch", "rules.md"),
		PrimeRoot:    filepath.Join(hub, "main"),
	}
	if got.HubRulesPath != want.HubRulesPath || got.OwnRulesPath != want.OwnRulesPath || got.PrimeRoot != want.PrimeRoot {
		t.Errorf("PrimePaths() rules paths and prime root = %q, %q, %q; want %q, %q, %q", got.HubRulesPath, got.OwnRulesPath, got.PrimeRoot, want.HubRulesPath, want.OwnRulesPath, want.PrimeRoot)
	}
}

func TestStart_Refusals(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name    string
		strands []reedengine.StrandStatus
		// args builds the verb's arguments from a handoff file path that exists.
		args   func(handoff string) []string
		wantIn []string
		// seed, when set, writes the state file before start runs.
		seed func(t *testing.T, h *startHarness)
		// rulesPath, when set, picks the rules path that seed makes unreadable and the refusal must name.
		rulesPath func(p orchengine.Paths) string
	}{
		{
			name: "unreadable hub rules",
			args: func(string) []string { return nil },
			seed: func(t *testing.T, h *startHarness) {
				if err := os.MkdirAll(h.cli.paths.HubRulesPath, 0o755); err != nil {
					t.Fatal(err)
				}
			},
			rulesPath: func(p orchengine.Paths) string { return p.HubRulesPath },
		},
		{
			name: "unreadable own rules",
			args: func(string) []string { return nil },
			seed: func(t *testing.T, h *startHarness) {
				if err := os.MkdirAll(h.cli.paths.OwnRulesPath, 0o755); err != nil {
					t.Fatal(err)
				}
			},
			rulesPath: func(p orchengine.Paths) string { return p.OwnRulesPath },
		},
		{
			name:    "handoff with a live strand",
			strands: []reedengine.StrandStatus{{GUID: "g1", Name: "orch", Live: true}},
			args:    func(handoff string) []string { return []string{"--handoff", handoff} },
			wantIn:  []string{"fresh launch only"},
		},
		{
			name: "two orch strands",
			strands: []reedengine.StrandStatus{
				{GUID: "g1", Name: "orch", Live: true},
				{GUID: "g2", Name: "orch", Live: false},
			},
			args:   func(string) []string { return nil },
			wantIn: []string{"g1", "g2"},
		},
		{
			name:   "adopt with a handoff",
			args:   func(handoff string) []string { return []string{"--adopt", adoptTestSessionID, "--handoff", handoff} },
			wantIn: []string{"exclusive"},
		},
		{
			name:    "adopt with a live strand",
			strands: []reedengine.StrandStatus{{GUID: "g1", Name: "orch", Live: true}},
			args:    func(string) []string { return []string{"--adopt", adoptTestSessionID} },
			wantIn:  []string{"lyx orch stop"},
		},
		{
			name:   "continue with adopt",
			args:   func(string) []string { return []string{"--continue", "--adopt", adoptTestSessionID} },
			wantIn: []string{"drop --adopt"},
		},
		{
			name:   "continue with a handoff",
			args:   func(handoff string) []string { return []string{"--continue", "--handoff", handoff} },
			wantIn: []string{"drop --handoff"},
		},
		{
			name:    "continue with a live strand",
			strands: []reedengine.StrandStatus{{GUID: "g1", Name: "orch", Live: true}},
			args:    func(string) []string { return []string{"--continue"} },
			wantIn:  []string{"lyx orch stop"},
			seed: func(t *testing.T, h *startHarness) {
				saveSeedState(t, h, orchengine.State{Phase: orchengine.PhaseIdle, Strand: "g1", SessionID: "rec-id"})
			},
		},
		{
			name:    "continue with no recorded id",
			strands: []reedengine.StrandStatus{{GUID: "corpse", Name: "orch", Live: false}},
			args:    func(string) []string { return []string{"--continue"} },
			wantIn:  []string{"lyx orch start", "--adopt <id>"},
			seed: func(t *testing.T, h *startHarness) {
				// A state file an older binary wrote carries no session_id key.
				if err := os.MkdirAll(h.cli.paths.Dir, 0o755); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(h.cli.paths.StatePath, []byte(`{"strand":"corpse","phase":"idle"}`), 0o644); err != nil {
					t.Fatal(err)
				}
			},
		},
		{
			name:   "continue with no state file",
			args:   func(string) []string { return []string{"--continue"} },
			wantIn: []string{"lyx orch start", "--adopt <id>"},
		},
		{
			name: "a first withdraw refused reports creel's error with the way forward",
			args: func(string) []string { return nil },
			seed: func(t *testing.T, h *startHarness) {
				h.creel.withdrawErrs = []error{errors.New("creel: the worktree's reed state is unreadable")}
			},
			wantIn: []string{"creel: the worktree's reed state is unreadable", "way forward", "lyx orch start"},
		},
		{
			name: "a first withdraw that cannot reach creel reports its error with the way forward",
			args: func(string) []string { return nil },
			seed: func(t *testing.T, h *startHarness) {
				h.creel.withdrawErrs = []error{errors.New("creel: the daemon did not start; log: listen failed")}
			},
			wantIn: []string{"creel: the daemon did not start; log: listen failed", "way forward", "lyx orch start"},
		},
		{
			name: "a first withdraw refused for a program claim from a non-orch pane names the orch's pane and a terminal outside every strand pane",
			args: func(string) []string { return nil },
			seed: func(t *testing.T, h *startHarness) {
				h.creel.withdrawErrs = []error{errors.New("creel: a program claim comes from a pane that is not the orch's")}
			},
			wantIn: []string{"a program claim comes from a pane that is not the orch's", "the orch's pane", "a terminal outside every strand pane"},
		},
		{
			name: "a beam another machine owns names the owner and the takeover",
			args: func(string) []string { return nil },
			seed: func(t *testing.T, h *startHarness) {
				h.cli.beamStart = func() error { return refuseBeamStart("b", "desktop", "laptop", nil) }
			},
			wantIn: []string{`"laptop"`, "lyx adopt b"},
		},
		{
			name: "a beam held by its landing names the landing",
			args: func(string) []string { return nil },
			seed: func(t *testing.T, h *startHarness) {
				landing := boardengine.RunStatus("running", "Land")
				h.cli.beamStart = func() error { return refuseBeamStart("b", "desktop", "", &landing) }
			},
			wantIn: []string{"landing", "lyx batten status b"},
		},
		{
			name: "a hub with no shortname record fails before any round",
			args: func(string) []string { return nil },
			seed: func(t *testing.T, h *startHarness) {
				if err := os.Remove(filepath.Join(fabricengine.BoardDir(h.cli.location.HubPath), fabricengine.ShortnameFileName)); err != nil {
					t.Fatal(err)
				}
			},
			wantIn: []string{"shortname record", "lyx fabric shortname"},
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()

			h := newStartHarness(t, c.strands...)
			handoff := filepath.Join(t.TempDir(), "handoff.md")
			if err := os.WriteFile(handoff, []byte("h"), 0o644); err != nil {
				t.Fatal(err)
			}

			if c.seed != nil {
				c.seed(t, h)
			}
			before := readStateFile(t, h)

			code, env := h.run(t, c.args(handoff)...)
			msg, _ := env["error"].(string)
			if code == 0 {
				t.Fatalf("exit = 0; env = %v; want a refusal", env)
			}
			for _, want := range c.wantIn {
				if !strings.Contains(msg, want) {
					t.Errorf("refusal %q missing %q", msg, want)
				}
			}
			if c.rulesPath != nil && !strings.Contains(msg, c.rulesPath(h.cli.paths)) {
				t.Errorf("refusal %q missing %q", msg, c.rulesPath(h.cli.paths))
			}
			if len(h.starter.specs) != 0 || h.spawns != 0 {
				t.Errorf("starts = %d, spawns = %d; want none", len(h.starter.specs), h.spawns)
			}
			if len(h.strands.removed) != 0 {
				t.Errorf("removed strands = %v; want none", h.strands.removed)
			}
			if after := readStateFile(t, h); !bytes.Equal(before, after) {
				t.Errorf("state file = %q; want it unchanged from %q", after, before)
			}
		})
	}
}

// saveSeedState saves st as the state file start finds.
func saveSeedState(t *testing.T, h *startHarness, st orchengine.State) {
	t.Helper()
	if err := orchengine.SaveState(h.cli.paths, st); err != nil {
		t.Fatal(err)
	}
}

// readStateFile returns the state file's bytes, nil while it is absent.
func readStateFile(t *testing.T, h *startHarness) []byte {
	t.Helper()
	data, err := os.ReadFile(h.cli.paths.StatePath)
	if err != nil && !os.IsNotExist(err) {
		t.Fatal(err)
	}
	return data
}

// envList returns the JSON array under key, failing the test when the key is absent or not an array.
func envList(t *testing.T, env map[string]any, key string) []any {
	t.Helper()
	list, ok := env[key].([]any)
	if !ok {
		t.Fatalf("envelope %s = %#v; want an array; env = %v", key, env[key], env)
	}
	return list
}

func TestStart_ReconcilesTheHubFirst(t *testing.T) {
	t.Parallel()

	const stopSentinel = "SENTINEL-way-forward-of-the-stopped-branch"
	addRemovedPair := func(h *startHarness) {
		h.report.Mutations.Append(fabricengine.KindWorktreeRemoved, "/hub/ended-slug", "ended pair removed")
	}
	liveOrch := reedengine.StrandStatus{GUID: "g1", Name: "orch", Live: true}
	tests := []struct {
		name    string
		strands []reedengine.StrandStatus
		prepare func(t *testing.T, h *startHarness)
		wantOK  bool
		check   func(t *testing.T, h *startHarness, env map[string]any)
	}{
		{
			name: "the reconcile runs before the launch and its report and the failed pushes are on the envelope",
			prepare: func(t *testing.T, h *startHarness) {
				h.report.Branches = []fabricengine.BranchOutcome{{Branch: "main", Repo: "code", State: fabricengine.BranchFastForward, Moved: true}}
				failed := fabricengine.FailedPush{Repo: "code", Branch: "feature", Class: fabricengine.PushFailureAuth, Error: "403"}
				if err := fabricengine.RecordPushFailure(h.cli.location.HubPath, failed, time.Now()); err != nil {
					t.Fatal(err)
				}
			},
			wantOK: true,
			check: func(t *testing.T, h *startHarness, env map[string]any) {
				if h.reconciles != 1 || h.callsAtReconcile != 0 || len(h.starter.specs) != 1 {
					t.Errorf("reconciles = %d at call %d, starts = %d; want one reconcile before the one launch", h.reconciles, h.callsAtReconcile, len(h.starter.specs))
				}
				if report, ok := env["reconcile"].(map[string]any); !ok || len(envList(t, report, "Branches")) != 1 {
					t.Errorf("envelope reconcile = %#v; want the report with its one branch", env["reconcile"])
				}
				pushes := envList(t, env, "failed_pushes")
				if row, _ := pushes[0].(map[string]any); len(pushes) != 1 || row["branch"] != "feature" {
					t.Errorf("failed_pushes = %v; want the one recorded push", pushes)
				}
				if got := envList(t, env, "mutations"); len(got) != 0 || env["partial"] != false {
					t.Errorf("mutations = %v, partial = %v; want an empty array and false", got, env["partial"])
				}
			},
		},
		{
			name:    "a teardown mutation is on the envelope",
			prepare: func(t *testing.T, h *startHarness) { addRemovedPair(h) },
			wantOK:  true,
			check: func(t *testing.T, h *startHarness, env map[string]any) {
				if got := envList(t, env, "mutations"); len(got) != 1 || env["partial"] != false {
					t.Errorf("mutations = %v, partial = %v; want the one mutation and false", got, env["partial"])
				}
			},
		},
		{
			name: "a stop launches and its summary is a line of the start prompt",
			prepare: func(t *testing.T, h *startHarness) {
				h.report.Branches = []fabricengine.BranchOutcome{{Branch: "main", Repo: "code", State: fabricengine.BranchDirty, WayForward: stopSentinel}}
			},
			wantOK: true,
			check: func(t *testing.T, h *startHarness, env map[string]any) {
				summary := h.report.Summary()
				if !strings.Contains(summary, stopSentinel) {
					t.Fatalf("summary = %q; want the stop's way forward", summary)
				}
				if len(h.starter.specs) != 1 || !strings.Contains(h.starter.specs[0].Prompt, summary) {
					t.Errorf("starts = %d; want one whose prompt carries %q", len(h.starter.specs), summary)
				}
			},
		},
		{
			name: "a launch failing after a teardown mutation reports it as partial",
			prepare: func(t *testing.T, h *startHarness) {
				addRemovedPair(h)
				h.starter.err = errors.New("provider never came up")
			},
			check: func(t *testing.T, h *startHarness, env map[string]any) {
				if got := envList(t, env, "mutations"); len(got) != 1 || env["partial"] != true {
					t.Errorf("mutations = %v, partial = %v; want the one mutation and true", got, env["partial"])
				}
			},
		},
		{
			name:    "a launch failing after an empty reconcile record is not partial",
			prepare: func(t *testing.T, h *startHarness) { h.starter.err = errors.New("provider never came up") },
			check: func(t *testing.T, h *startHarness, env map[string]any) {
				if got := envList(t, env, "mutations"); len(got) != 0 || env["partial"] != false {
					t.Errorf("mutations = %v, partial = %v; want an empty array and false", got, env["partial"])
				}
			},
		},
		{
			name:    "an unreachable origin launches with the warning",
			prepare: func(t *testing.T, h *startHarness) { h.report.Unreachable = true },
			wantOK:  true,
			check: func(t *testing.T, h *startHarness, env map[string]any) {
				if warning, _ := env["warning"].(string); !strings.Contains(warning, "unreachable") || len(h.starter.specs) != 1 {
					t.Errorf("warning = %q, starts = %d; want the unreachable warning and one launch", warning, len(h.starter.specs))
				}
			},
		},
		{
			name: "a reconcile error launches with the error reported",
			prepare: func(t *testing.T, h *startHarness) {
				h.reconcileErr = errors.New("ls-remote failed for the records repository")
			},
			wantOK: true,
			check: func(t *testing.T, h *startHarness, env map[string]any) {
				if warning, _ := env["warning"].(string); !strings.Contains(warning, "ls-remote failed for the records repository") || len(h.starter.specs) != 1 {
					t.Errorf("warning = %q, starts = %d; want the reconcile error and one launch", warning, len(h.starter.specs))
				}
			},
		},
		{
			name: "a hub with no prime owner claims it once and reports the claim",
			prepare: func(t *testing.T, h *startHarness) {
				h.claims = []claimAnswer{{claimed: "desktop"}}
			},
			wantOK: true,
			check: func(t *testing.T, h *startHarness, env map[string]any) {
				if warning, _ := env["warning"].(string); h.claimCalls != 1 || !strings.Contains(warning, "claimed for machine desktop") || len(h.starter.specs) != 1 {
					t.Errorf("claims = %d, warning = %q, starts = %d; want one claim reported and one launch", h.claimCalls, warning, len(h.starter.specs))
				}
			},
		},
		{
			name: "a lost claim reports the other machine and the launch goes on",
			prepare: func(t *testing.T, h *startHarness) {
				h.claims = []claimAnswer{{lostTo: "laptop"}}
			},
			wantOK: true,
			check: func(t *testing.T, h *startHarness, env map[string]any) {
				if warning, _ := env["warning"].(string); !strings.Contains(warning, "machine laptop") || len(h.starter.specs) != 1 {
					t.Errorf("warning = %q, starts = %d; want the other machine named and one launch", warning, len(h.starter.specs))
				}
			},
		},
		{
			name: "a failing claim warns and the launch goes on",
			prepare: func(t *testing.T, h *startHarness) {
				h.claims = []claimAnswer{{err: errors.New("origin is unreachable")}}
			},
			wantOK: true,
			check: func(t *testing.T, h *startHarness, env map[string]any) {
				if warning, _ := env["warning"].(string); !strings.Contains(warning, "origin is unreachable") || len(h.starter.specs) != 1 {
					t.Errorf("warning = %q, starts = %d; want the failure and one launch", warning, len(h.starter.specs))
				}
			},
		},
		{
			name: "an owned_elsewhere entry is paused once before the launch and named in owner_pauses",
			prepare: func(t *testing.T, h *startHarness) {
				h.report.Entries = []fabricengine.EntryOutcome{{Slug: "p1", State: fabricengine.EntryOwnedElsewhere, Local: true, Owner: "laptop"}}
				h.pauseOwned = true
			},
			wantOK: true,
			check: func(t *testing.T, h *startHarness, env map[string]any) {
				pauses := envList(t, env, "owner_pauses")
				row, _ := pauses[0].(map[string]any)
				if !reflect.DeepEqual(h.ownerPaused, []string{"p1"}) || len(pauses) != 1 || row["slug"] != "p1" || row["owner"] != "laptop" || h.callsAtOwnerPause != 0 || len(h.starter.specs) != 1 {
					t.Errorf("paused = %v, owner_pauses = %v, calls before the pause = %d, starts = %d; want p1 paused once before any strand call and one launch", h.ownerPaused, pauses, h.callsAtOwnerPause, len(h.starter.specs))
				}
			},
		},
		{
			name: "a failing owner pause warns naming the re-run of orch start and the launch goes on",
			prepare: func(t *testing.T, h *startHarness) {
				h.report.Entries = []fabricengine.EntryOutcome{{Slug: "p1", State: fabricengine.EntryOwnedElsewhere, Local: true, Owner: "laptop"}}
				h.pauseOwned, h.pauseOwnedErr = true, errors.New("tender down")
			},
			wantOK: true,
			check: func(t *testing.T, h *startHarness, env map[string]any) {
				warning, _ := env["warning"].(string)
				if !strings.Contains(warning, "tender down") || !strings.Contains(warning, "re-run `lyx orch start`, which pauses it again") || len(envList(t, env, "owner_pauses")) != 0 || len(h.starter.specs) != 1 {
					t.Errorf("warning = %q, starts = %d; want the failure and the re-run named, nothing paused and one launch", warning, len(h.starter.specs))
				}
			},
		},
		{
			name: "with no owner pause wired an owned_elsewhere entry is named under the warnings and the launch goes on",
			prepare: func(t *testing.T, h *startHarness) {
				h.report.Entries = []fabricengine.EntryOutcome{{Slug: "p1", State: fabricengine.EntryOwnedElsewhere, Local: true, Owner: "laptop"}}
			},
			wantOK: true,
			check: func(t *testing.T, h *startHarness, env map[string]any) {
				if warning, _ := env["warning"].(string); !strings.Contains(warning, "p1 is owned by machine laptop") || len(h.starter.specs) != 1 {
					t.Errorf("warning = %q, starts = %d; want p1 and its owner named and one launch", warning, len(h.starter.specs))
				}
			},
		},
		{
			name:    "an already-live strand still runs the reconcile and reports it",
			strands: []reedengine.StrandStatus{liveOrch},
			prepare: func(t *testing.T, h *startHarness) { addRemovedPair(h) },
			wantOK:  true,
			check: func(t *testing.T, h *startHarness, env map[string]any) {
				if h.reconciles != 1 || env["action"] != actionSpawnedWatcher || len(envList(t, env, "mutations")) != 1 {
					t.Errorf("reconciles = %d, env = %v; want one reconcile and its mutation reported", h.reconciles, env)
				}
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newStartHarness(t, tt.strands...)
			tt.prepare(t, h)
			code, env := h.run(t)
			if (code == 0) != tt.wantOK {
				t.Fatalf("exit = %d; env = %v; want success = %v", code, env, tt.wantOK)
			}
			tt.check(t, h, env)
		})
	}
}

func TestStart_StarterErrorFailsAndSavesNoState(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		args []string
		err  string
		// seed, when set, is the state saved before start runs.
		seed *orchengine.State
		// wantIn are further texts the error carries.
		wantIn []string
	}{
		{name: "fresh launch", err: "provider never came up"},
		{name: "adopt", args: []string{"--adopt", adoptTestSessionID}, err: "no transcript for that session"},
		{
			name:   "continue",
			args:   []string{"--continue"},
			err:    "no transcript for that session",
			seed:   &orchengine.State{Phase: orchengine.PhaseIdle, SessionID: "rec-id"},
			wantIn: []string{"lyx orch start --adopt <id>"},
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()

			h := newStartHarness(t)
			h.starter.err = errors.New(c.err)
			if c.seed != nil {
				saveSeedState(t, h, *c.seed)
			}
			before := h.state(t)

			code, env := h.run(t, c.args...)
			msg, _ := env["error"].(string)
			if code == 0 || !strings.Contains(msg, c.err) {
				t.Fatalf("exit = %d; env = %v", code, env)
			}
			for _, want := range c.wantIn {
				if !strings.Contains(msg, want) {
					t.Errorf("error %q missing %q", msg, want)
				}
			}
			if h.spawns != 0 {
				t.Errorf("spawns = %d; want none after a failed launch", h.spawns)
			}
			if st := h.state(t); !reflect.DeepEqual(st, before) {
				t.Errorf("state = %+v; want the seeded state %+v, unchanged", st, before)
			}
		})
	}
}

func TestStart_AdoptLaunchesResumeSpec(t *testing.T) {
	t.Parallel()

	adoptArgs := []string{"--adopt", adoptTestSessionID}
	oldIDHint := previousSessionHint("old-id")
	corpse := []reedengine.StrandStatus{{GUID: "corpse", Name: "orch", Live: false}}
	cases := []struct {
		name    string
		strands []reedengine.StrandStatus
		warning string
		args    []string
		// handoff appends --handoff with a handoff file that exists.
		handoff bool
		// recorded is the session id the seeded state holds.
		recorded string
		// seedStrand is the strand the seeded state names.
		seedStrand string
		// noSessionID makes the starter name no session id for a launch that resumes none.
		noSessionID bool
		// stopping seeds WatcherStopping and holds watch.lock for the whole run.
		stopping bool
		// stopFirst runs the stop verb, then reports no strand as reed does after the removal.
		stopFirst bool

		wantResumeID  string
		wantSource    string
		wantSaved     string
		wantHint      string
		wantSessionID string
	}{
		{name: "dead strand", strands: corpse, args: adoptArgs, wantResumeID: adoptTestSessionID, wantSource: orchengine.SourceAdopt, wantSaved: adoptTestSessionID},
		{name: "no strand", args: adoptArgs, wantResumeID: adoptTestSessionID, wantSource: orchengine.SourceAdopt, wantSaved: adoptTestSessionID},
		{name: "starter warning reaches the envelope", warning: "registry unreadable", args: adoptArgs, wantResumeID: adoptTestSessionID, wantSource: orchengine.SourceAdopt, wantSaved: adoptTestSessionID},
		{name: "plain launch with none recorded", wantSource: orchengine.SourceFresh, wantSaved: "minted-id"},
		{name: "plain launch replaces a recorded id", recorded: "old-id", wantSource: orchengine.SourceFresh, wantSaved: "minted-id", wantHint: oldIDHint},
		{name: "handoff launch replaces a recorded id", handoff: true, recorded: "old-id", wantSource: orchengine.SourceFlag, wantSaved: "minted-id", wantHint: oldIDHint},
		{name: "a launch naming no id records none and keeps the previous in the hint", recorded: "old-id", noSessionID: true, wantSource: orchengine.SourceFresh, wantSaved: "", wantHint: oldIDHint},
		{name: "adopt naming another id", args: adoptArgs, recorded: "old-id", wantResumeID: adoptTestSessionID, wantSource: orchengine.SourceAdopt, wantSaved: adoptTestSessionID, wantHint: oldIDHint},
		{name: "adopt naming the recorded id", args: adoptArgs, recorded: adoptTestSessionID, wantResumeID: adoptTestSessionID, wantSource: orchengine.SourceAdopt, wantSaved: adoptTestSessionID},
		{
			name:          "continue after stop resumes the recorded id",
			strands:       []reedengine.StrandStatus{{GUID: "g1", Name: "orch", Live: true}},
			args:          []string{"--continue"},
			recorded:      adoptTestSessionID,
			seedStrand:    "g1",
			stopFirst:     true,
			wantResumeID:  adoptTestSessionID,
			wantSource:    orchengine.SourceAdopt,
			wantSaved:     adoptTestSessionID,
			wantSessionID: adoptTestSessionID,
		},
		{
			name:       "a stopping watcher and a replaced id carry both hints",
			recorded:   "old-id",
			stopping:   true,
			wantSource: orchengine.SourceFresh,
			wantSaved:  "minted-id",
			wantHint:   stoppingWatcherHint + "; " + oldIDHint,
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()

			h := newStartHarness(t, c.strands...)
			h.starter.warning = c.warning
			if c.noSessionID {
				h.starter.sessionID = ""
			}
			h.cli.cfg.PermissionMode = "bypass"
			seed := orchengine.State{Phase: orchengine.PhaseIdle, Strand: c.seedStrand, LastHandoff: "/keep/me.md", SessionID: c.recorded, WatcherStopping: c.stopping}
			args := slices.Clone(c.args)
			if c.handoff {
				handoff := filepath.Join(t.TempDir(), "handoff.md")
				if err := os.WriteFile(handoff, []byte("h"), 0o644); err != nil {
					t.Fatal(err)
				}
				seed.LastHandoff = handoff
				args = append(args, "--handoff", handoff)
			}
			saveSeedState(t, h, seed)
			if c.stopping {
				l, err := lock.AcquireWriteLock(h.cli.paths.WatchLockPath)
				if err != nil {
					t.Fatal(err)
				}
				defer l.Release()
			}
			if c.stopFirst {
				if code, env := runVerb(t, h.cli.stopCmd()); code != 0 {
					t.Fatalf("stop exit = %d; env = %v", code, env)
				}
				h.strands.strands = nil
			}

			code, env := h.run(t, args...)
			if code != 0 || env["prompt_source"] != c.wantSource || env["action"] != actionRelaunched {
				t.Fatalf("exit = %d; env = %v", code, env)
			}
			if got, _ := env["hint"].(string); got != c.wantHint {
				t.Errorf("hint = %q; want %q", got, c.wantHint)
			}
			if got, _ := env["session_id"].(string); got != c.wantSessionID {
				t.Errorf("session_id = %q; want %q", got, c.wantSessionID)
			}
			if c.wantHint == "" {
				if _, has := env["hint"]; has {
					t.Errorf("envelope = %v; want no hint", env)
				}
			}
			if c.wantSessionID == "" {
				if _, has := env["session_id"]; has {
					t.Errorf("envelope = %v; want no session_id", env)
				}
			}
			if c.warning == "" {
				if _, has := env["warning"]; has {
					t.Errorf("envelope = %v; want no warning", env)
				}
			} else if env["warning"] != c.warning {
				t.Errorf("warning = %v; want %q on the envelope", env["warning"], c.warning)
			}
			if len(h.starter.specs) != 1 || h.spawns != 1 {
				t.Fatalf("starts = %d, spawns = %d; want 1 and 1", len(h.starter.specs), h.spawns)
			}
			spec := h.starter.specs[0]
			if spec.ResumeSessionID != c.wantResumeID || spec.PermissionMode != "bypass" || !spec.AllowAgentTool || !spec.ForkSubagents {
				t.Errorf("spec = %+v; want resume id %q, bypass, AllowAgentTool and ForkSubagents", spec, c.wantResumeID)
			}
			if c.wantSource == orchengine.SourceAdopt {
				wantPrompt, err := orchengine.RenderAdoptPrompt(h.cli.stencilsDir, h.cli.paths.RolePath)
				if err != nil {
					t.Fatal(err)
				}
				if spec.Prompt != wantPrompt {
					t.Errorf("prompt = %q; want the adopt prompt %q", spec.Prompt, wantPrompt)
				}
			}
			if st := h.state(t); st.Strand != "new-guid" || st.SessionID != c.wantSaved || (!c.handoff && st.LastHandoff != "/keep/me.md") {
				t.Errorf("state = %+v; want LastHandoff kept, strand new-guid and session id %q", st, c.wantSaved)
			}
		})
	}
}
