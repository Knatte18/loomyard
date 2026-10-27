//go:build tier1

// cli_test.go drives `lym tender pause`, `status`, `resume` and `revive` through the cobra commands over a hubforge hub with one pair and no agents running, and the network gate installer over the same hub.
// No test but TestNewShuttlePair_TellsTheHubsPauseGeometry calls t.Parallel: each sets the strand-name variable or reads the process-wide network gate.

package tendercli

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/spf13/cobra"

	"github.com/Knatte18/loomyard/internal/agentname"
	"github.com/Knatte18/loomyard/internal/clihelp"
	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/gitkit"
	"github.com/Knatte18/loomyard/internal/hubforge"
	"github.com/Knatte18/loomyard/internal/hubgeom"
	"github.com/Knatte18/loomyard/internal/lymcwd"
	"github.com/Knatte18/loomyard/internal/pauserecord"
	"github.com/Knatte18/loomyard/internal/runproc"
	"github.com/Knatte18/loomyard/internal/testkit/envelope"
)

const pairSlug = hubforge.TemplatePairSlug

// verb runs a pause-module command in cwd and decodes its envelope.
func verb(t *testing.T, command *cobra.Command, cwd string, args ...string) (envelope.Envelope, int) {
	t.Helper()
	var out bytes.Buffer
	code := clihelp.ExecuteIn(command, cwd, &out, args)
	return envelope.Decode(t, out.String()), code
}

func newHub(t *testing.T) *hubforge.Hub {
	t.Helper()
	return hubforge.CopyHub(t, hubforge.Shape{Anchor: ".", Pairs: []string{pairSlug}})
}

// liveEntries reads the hub's pause record and returns its live and resumed entries.
func liveEntries(t *testing.T, h *hubforge.Hub) (live, resumed []pauserecord.Entry) {
	t.Helper()
	geometry, err := hubgeom.PauseGeometry(h.Location)
	if err != nil {
		t.Fatal(err)
	}
	record, err := pauserecord.Read(geometry.RecordPath)
	if err != nil {
		t.Fatal(err)
	}
	return record.Live, record.Resumed
}

func report(t *testing.T, env envelope.Envelope, key string) map[string]any {
	t.Helper()
	body, ok := env.Raw[key].(map[string]any)
	if !ok {
		t.Fatalf("envelope %v has no %q object", env.Raw, key)
	}
	return body
}

func TestStatus_PrintsOutstandingFailedPushes(t *testing.T) {
	t.Setenv(agentname.StrandNameEnv, "")
	h := newHub(t)
	pair := h.PairCodeWorktree(pairSlug)

	none, code := verb(t, Command(), pair, "status")
	if !none.OK || code != 0 {
		t.Fatalf("status: ok=%v code=%d error=%q", none.OK, code, none.Error)
	}
	if rows, isArray := none.Raw["failed_pushes"].([]any); !isArray || len(rows) != 0 {
		t.Errorf("failed_pushes with no record = %v; want an empty array", none.Raw["failed_pushes"])
	}

	record := fabricengine.FailedPush{
		Repo:     "code",
		RepoPath: pair,
		Branch:   "feature/x",
		Class:    fabricengine.PushFailureTransport,
		Error:    "connection refused",
	}
	if err := fabricengine.RecordPushFailure(h.Location.HubPath, record, time.Now()); err != nil {
		t.Fatalf("RecordPushFailure: %v", err)
	}

	withRecord, code := verb(t, Command(), pair, "status")
	if !withRecord.OK || code != 0 {
		t.Fatalf("status: ok=%v code=%d error=%q", withRecord.OK, code, withRecord.Error)
	}
	rows, _ := withRecord.Raw["failed_pushes"].([]any)
	if len(rows) != 1 {
		t.Fatalf("failed_pushes = %v; want the one record", withRecord.Raw["failed_pushes"])
	}
	row, _ := rows[0].(map[string]any)
	for key, want := range map[string]any{"repo": "code", "branch": "feature/x", "class": "transport", "error": "connection refused", "count": float64(1)} {
		if row[key] != want {
			t.Errorf("failed_pushes row %s = %v; want %v", key, row[key], want)
		}
	}
	if wayForward, _ := row["way_forward"].(string); !strings.Contains(wayForward, "lym fabric push") || !strings.Contains(wayForward, pair) {
		t.Errorf("failed_pushes way_forward = %q; want the push step run in %s", wayForward, pair)
	}
}

func TestPauseAndResume_WriteLiftAndRefuseOverTheHub(t *testing.T) {
	t.Setenv(agentname.StrandNameEnv, "")
	h := newHub(t)
	prime := h.PrimeWorktree()

	first, code := verb(t, Command(), prime, "pause")
	if !first.OK || code != 0 {
		t.Fatalf("pause: ok=%v code=%d error=%q", first.OK, code, first.Error)
	}
	body := report(t, first, "pause")
	for _, list := range []string{"interrupted", "no_running_turn", "background", "still_busy", "net_ops", "go_producers"} {
		items, isArray := body[list].([]any)
		if !isArray || len(items) != 0 {
			t.Errorf("pause %s = %v; want an empty array with nothing running", list, body[list])
		}
	}
	if live, _ := liveEntries(t, h); len(live) != 1 || live[0].Scope != pauserecord.ScopeHub || live[0].Caller != "EXTERNAL" {
		t.Fatalf("live entries = %+v; want one hub entry from an external caller", live)
	}

	again, _ := verb(t, Command(), prime, "pause")
	if report(t, again, "pause")["already_paused"] != true {
		t.Errorf("repeated pause = %v; want already_paused", again.Raw)
	}
	if live, _ := liveEntries(t, h); len(live) != 1 {
		t.Errorf("live entries after a repeat = %d; want 1", len(live))
	}

	liveBefore, _ := liveEntries(t, h)
	status, code := verb(t, Command(), h.PairCodeWorktree(pairSlug), "status")
	if !status.OK || code != 0 {
		t.Fatalf("status: ok=%v error=%q", status.OK, status.Error)
	}
	if entries, _ := report(t, status, "status")["live"].([]any); len(entries) != 1 {
		t.Errorf("status live = %v; want the one entry", entries)
	}
	if sessions, isArray := report(t, status, "status")["sessions"].([]any); !isArray || len(sessions) != 0 {
		t.Errorf("status sessions = %v; want an empty array with no agents running", report(t, status, "status")["sessions"])
	}
	if stale, _ := report(t, status, "status")["stale"].(map[string]any); stale["supported"] != true {
		t.Errorf("status stale = %v; want supported true", stale)
	}
	if liveAfter, _ := liveEntries(t, h); len(liveAfter) != 1 || !liveAfter[0].PausedAt.Equal(liveBefore[0].PausedAt) {
		t.Errorf("status changed the record: %+v then %+v", liveBefore, liveAfter)
	}

	removedFlag, code := verb(t, Command(), prime, "pause", "--status")
	if removedFlag.OK || code == 0 {
		t.Errorf("pause --status: ok=%v; want a refusal as an unknown flag", removedFlag.OK)
	}
	if live, _ := liveEntries(t, h); len(live) != 1 {
		t.Errorf("live entries after the refused pause --status = %d; want 1", len(live))
	}

	unknown, code := verb(t, Command(), prime, "pause", "no-such-pair")
	if unknown.OK || code == 0 || !strings.Contains(unknown.Error, pairSlug) {
		t.Errorf("unknown slug: ok=%v error=%q; want a refusal listing %q", unknown.OK, unknown.Error, pairSlug)
	}
	nothingStalled, code := verb(t, Command(), prime, "revive")
	if !nothingStalled.OK || code != 0 {
		t.Fatalf("revive: ok=%v error=%q", nothingStalled.OK, nothingStalled.Error)
	}
	if sent, isArray := report(t, nothingStalled, "revive")["sent"].([]any); !isArray || len(sent) != 0 {
		t.Errorf("revive sent = %v; want an empty array with nothing stalled", report(t, nothingStalled, "revive")["sent"])
	}
	unknownRevive, code := verb(t, Command(), prime, "revive", "no-such-pair")
	if unknownRevive.OK || code == 0 || !strings.Contains(unknownRevive.Error, pairSlug) {
		t.Errorf("revive unknown slug: ok=%v error=%q; want a refusal listing %q", unknownRevive.OK, unknownRevive.Error, pairSlug)
	}
	badGrace, code := verb(t, Command(), prime, "pause", "--grace", "61")
	if badGrace.OK || code == 0 || !strings.Contains(badGrace.Error, "0 to 60") {
		t.Errorf("--grace 61: ok=%v error=%q; want a refusal naming the range", badGrace.OK, badGrace.Error)
	}

	slugResume, code := verb(t, Command(), prime, "resume", pairSlug)
	if slugResume.OK || code == 0 || !strings.Contains(slugResume.Error, `"lym tender resume"`) {
		t.Errorf("resume <slug> under a hub entry: ok=%v error=%q; want a refusal naming lym tender resume", slugResume.OK, slugResume.Error)
	}

	// A fresh resolution from another worktree still sees the entry on disk.
	resumed, code := verb(t, Command(), h.PairCodeWorktree(pairSlug), "resume")
	if !resumed.OK || code != 0 {
		t.Fatalf("resume: ok=%v error=%q", resumed.OK, resumed.Error)
	}
	if items, _ := report(t, resumed, "resume")["resumed"].([]any); len(items) != 1 {
		t.Errorf("resume resumed = %v; want the hub entry", items)
	}
	if live, done := liveEntries(t, h); len(live) != 0 || len(done) != 1 {
		t.Errorf("record after resume: live %+v, resumed %+v; want the entry moved", live, done)
	}
	nothing, _ := verb(t, Command(), prime, "resume")
	if items, _ := report(t, nothing, "resume")["resumed"].([]any); !nothing.OK || len(items) != 0 {
		t.Errorf("second resume = %v; want nothing resumed", nothing.Raw)
	}
}

func TestPauseOwnedElsewhere_PausesTheSlugOnceUnderTheOwnerCaller(t *testing.T) {
	t.Setenv(agentname.StrandNameEnv, "")
	h := newHub(t)
	var ends []string
	var endErr error
	previous := endOwnedElsewhere
	t.Cleanup(func() { endOwnedElsewhere = previous })
	endOwnedElsewhere = func(location *lymcwd.Location, slug string) ([]runproc.Ended, error) {
		live, _ := liveEntries(t, h)
		if len(live) != 1 {
			t.Errorf("live entries at the end = %+v; want the pause written first", live)
		}
		ends = append(ends, slug)
		return nil, endErr
	}

	for range 2 {
		if err := PauseOwnedElsewhere(h.Location, pairSlug); err != nil {
			t.Fatalf("PauseOwnedElsewhere() error = %v", err)
		}
	}

	if live, _ := liveEntries(t, h); len(live) != 1 || live[0].Scope != pairSlug || live[0].Caller != pauserecord.OwnerCaller {
		t.Errorf("live entries = %+v; want one entry of the pair under the owner caller", live)
	}
	if want := []string{pairSlug, pairSlug}; !reflect.DeepEqual(ends, want) {
		t.Errorf("end calls = %q; want %q", ends, want)
	}

	for name, err := range map[string]error{
		"an end error":    errors.New("signal refused"),
		"a survivors one": &runproc.SurvivorsError{Survivors: []runproc.Ended{{PID: 7, Role: runproc.RoleDriver}}},
	} {
		endErr = err
		if got := PauseOwnedElsewhere(h.Location, pairSlug); got != nil {
			t.Errorf("PauseOwnedElsewhere() with %s = %v; want nil", name, got)
		}
		if live, _ := liveEntries(t, h); len(live) != 1 {
			t.Errorf("live entries with %s = %+v; want the entry standing", name, live)
		}
	}
}

func TestPauseAndResume_ApplyTheCallerRules(t *testing.T) {
	h := newHub(t)
	prime := h.PrimeWorktree()
	agent, err := agentname.Format("lym", pairSlug, "worker")
	if err != nil {
		t.Fatal(err)
	}
	orch, err := agentname.Format("lym", "", agentname.RoleOrch)
	if err != nil {
		t.Fatal(err)
	}

	t.Setenv(agentname.StrandNameEnv, agent)
	for _, tc := range []struct {
		name    string
		command *cobra.Command
		args    []string
	}{
		{"a hub-wide pause", Command(), []string{"pause"}},
		{"a hub-wide resume", Command(), []string{"resume"}},
		{"a pause of another slug", Command(), []string{"pause", "other-pair"}},
		{"a hub-wide revive", Command(), []string{"revive"}},
		{"a revive of another slug", Command(), []string{"revive", "other-pair"}},
		{"a reap", Command(), []string{"reap"}},
	} {
		env, code := verb(t, tc.command, prime, tc.args...)
		if env.OK || code == 0 || !strings.Contains(env.Error, "ask your parent session") {
			t.Errorf("agent %s: ok=%v error=%q; want a refusal naming the parent session", tc.name, env.OK, env.Error)
		}
	}
	if live, _ := liveEntries(t, h); len(live) != 0 {
		t.Fatalf("live entries after refusals = %+v; want none", live)
	}
	if status, code := verb(t, Command(), prime, "status"); !status.OK || code != 0 {
		t.Errorf("agent status: ok=%v error=%q; want it to succeed", status.OK, status.Error)
	}
	if revived, code := verb(t, Command(), prime, "revive", pairSlug); !revived.OK || code != 0 {
		t.Errorf("agent revive of its own pair: ok=%v error=%q; want it to succeed", revived.OK, revived.Error)
	}
	own, code := verb(t, Command(), prime, "pause", pairSlug)
	if !own.OK || code != 0 {
		t.Fatalf("agent pause of its own pair: ok=%v error=%q", own.OK, own.Error)
	}
	if live, _ := liveEntries(t, h); len(live) != 1 || live[0].Scope != pairSlug || live[0].Caller != agent {
		t.Errorf("live entries = %+v; want the pair's entry with the agent as caller", live)
	}
	if lifted, _ := verb(t, Command(), prime, "resume", pairSlug); !lifted.OK {
		t.Errorf("agent resume of its own pair: %q", lifted.Error)
	}

	t.Setenv(agentname.StrandNameEnv, orch)
	if hubWide, _ := verb(t, Command(), prime, "pause"); !hubWide.OK {
		t.Errorf("orch hub-wide pause: %q", hubWide.Error)
	}
	if live, _ := liveEntries(t, h); len(live) != 1 || live[0].Scope != pauserecord.ScopeHub || live[0].Caller != orch {
		t.Errorf("live entries = %+v; want the hub entry with the orch as caller", live)
	}
	if reaped, code := verb(t, Command(), prime, "reap"); !reaped.OK || code != 0 {
		t.Errorf("orch reap: ok=%v error=%q; want no caller refusal", reaped.OK, reaped.Error)
	}
}

func TestInstallNetworkGate_TellsThePairsSlugAndInstallsNothingOutsideAHub(t *testing.T) {
	t.Setenv(agentname.StrandNameEnv, "")
	h := newHub(t)
	allAlive := func(int, string) bool { return true }
	sentinel := pauserecord.Geometry{RecordPath: filepath.Join(t.TempDir(), "sentinel", pauserecord.RecordFileName)}
	t.Cleanup(pauserecord.InstallGate(pauserecord.Gate{Geometry: sentinel}))

	// An agent worktree nested in the prime is a git worktree whose parent is no hub.
	agentWorktrees := filepath.Join(h.PrimeWorktree(), ".claude", "worktrees")
	agentWorktree := filepath.Join(agentWorktrees, "agent")
	gitkit.Git(t, h.PrimeWorktree(), "worktree", "add", "--detach", agentWorktree)
	for _, outside := range []string{t.TempDir(), agentWorktree} {
		InstallNetworkGate(lymcwd.WithCwd(t.Context(), outside), nil, DetachedChild{})
		_, probeDone := pauserecord.Network("probe")
		probeDone()
	}
	pauserecord.Network("left-in-flight")
	if ops, err := pauserecord.ListNetOps(sentinel.RecordPath, allAlive); err != nil || len(ops) != 1 || ops[0].Op != "left-in-flight" {
		t.Fatalf("operations under the sentinel gate = %+v, %v; want the one left in flight, since nothing was installed outside a hub", ops, err)
	}
	if _, err := os.Stat(filepath.Join(agentWorktrees, "_board")); !os.IsNotExist(err) {
		t.Errorf("Stat(agent worktree's sibling _board) error = %v; want it absent", err)
	}

	InstallNetworkGate(lymcwd.WithCwd(t.Context(), h.PairCodeWorktree(pairSlug)), nil, DetachedChild{})
	pauserecord.Network("pair-probe")
	geometry, err := hubgeom.PauseGeometry(h.Location)
	if err != nil {
		t.Fatal(err)
	}
	ops, err := pauserecord.ListNetOps(geometry.RecordPath, allAlive)
	if err != nil || len(ops) != 1 || ops[0].Slug != pairSlug || ops[0].Op != "pair-probe" {
		t.Errorf("operations under the hub's record = %+v, %v; want one marked with the pair's slug", ops, err)
	}

	// A detached push child starts in the hub's board worktree and is told its slug.
	boardTests := []struct {
		name      string
		anchor    string
		pauseSlug string
	}{
		{"TellsItsSlug", ".", "child"},
		{"EmptySlugIsKeptNotBoard", ".", ""},
		{"SubpathAnchoredHub", "backend", "child"},
	}
	for _, tt := range boardTests {
		t.Run(tt.name, func(t *testing.T) {
			hub := hubforge.CopyHub(t, hubforge.Shape{Anchor: tt.anchor})
			slug := tt.pauseSlug
			boardWorktree := fabricengine.BoardDir(hub.Location.HubPath)
			InstallNetworkGate(lymcwd.WithCwd(t.Context(), boardWorktree), &slug, DetachedChild{})
			pauserecord.Network("board-probe")
			geometry, err := hubgeom.PauseGeometry(hub.Location)
			if err != nil {
				t.Fatal(err)
			}
			ops, err := pauserecord.ListNetOps(geometry.RecordPath, allAlive)
			if err != nil || len(ops) != 1 || ops[0].Slug != tt.pauseSlug || ops[0].Op != "board-probe" {
				t.Errorf("operations under the hub's record = %+v, %v; want one marked with slug %q", ops, err, tt.pauseSlug)
			}
		})
	}

	// A detached child's flags decide what the installed gate reads as covering it.
	setLive := func(t *testing.T, recordPath string, entries ...pauserecord.Entry) {
		t.Helper()
		if _, err := pauserecord.Update(recordPath, time.Now(), func(r *pauserecord.Record) error {
			r.Live = entries
			return nil
		}); err != nil {
			t.Fatal(err)
		}
	}
	hubEntry := pauserecord.Entry{Scope: pauserecord.ScopeHub, Caller: "EXTERNAL", PausedAt: time.Now()}
	ownerEntry := pauserecord.Entry{Scope: "child", Caller: pauserecord.OwnerCaller, PausedAt: time.Now()}
	childTests := []struct {
		name        string
		cwd         func(hub *hubforge.Hub) string
		pauseSlug   *string
		child       DetachedChild
		live        []pauserecord.Entry
		wantCovered bool
	}{
		{"DeferrableUnderHubEntry", func(hub *hubforge.Hub) string { return fabricengine.BoardDir(hub.Location.HubPath) }, ptr("child"), DetachedChild{Deferrable: true}, []pauserecord.Entry{hubEntry}, true},
		{"ParentLockedPassesOwnerEntry", func(hub *hubforge.Hub) string { return fabricengine.BoardDir(hub.Location.HubPath) }, ptr("child"), DetachedChild{Deferrable: true, ParentLocked: true}, []pauserecord.Entry{ownerEntry}, false},
		{"HubScopeIgnoresTaskSlugEntry", func(hub *hubforge.Hub) string { return hub.PairCodeWorktree(pairSlug) }, nil, DetachedChild{Deferrable: true, HubScope: true}, []pauserecord.Entry{{Scope: pairSlug, Caller: "EXTERNAL", PausedAt: time.Now()}}, false},
		{"HubScopeHeldByHubEntry", func(hub *hubforge.Hub) string { return hub.PairCodeWorktree(pairSlug) }, nil, DetachedChild{Deferrable: true, HubScope: true}, []pauserecord.Entry{{Scope: pairSlug, Caller: "EXTERNAL", PausedAt: time.Now()}, hubEntry}, true},
	}
	for _, tt := range childTests {
		t.Run(tt.name, func(t *testing.T) {
			hub := newHub(t)
			geometry, err := hubgeom.PauseGeometry(hub.Location)
			if err != nil {
				t.Fatal(err)
			}
			setLive(t, geometry.RecordPath, tt.live...)
			InstallNetworkGate(lymcwd.WithCwd(t.Context(), tt.cwd(hub)), tt.pauseSlug, tt.child)
			if got := pauserecord.GateCovered(); got != tt.wantCovered {
				t.Errorf("GateCovered() = %v; want %v", got, tt.wantCovered)
			}
		})
	}
}

// ptr returns a pointer to a copy of s.
func ptr(s string) *string {
	return &s
}

// TestNewShuttlePair_TellsTheHubsPauseGeometry pins that a pair's runner reads the hub's pause record with no slug of its own, so each send judges its target by the target's strand name.
// It sets no process-wide state, so it runs parallel beside the file's serial tests.
func TestNewShuttlePair_TellsTheHubsPauseGeometry(t *testing.T) {
	t.Parallel()
	h := newHub(t)
	location, err := lymcwd.ResolveWorktree(h.PairCodeWorktree(pairSlug))
	if err != nil {
		t.Fatal(err)
	}
	geometry, err := hubgeom.PauseGeometry(location)
	if err != nil {
		t.Fatal(err)
	}

	pair, err := newShuttlePair(location)
	if err != nil {
		t.Fatalf("newShuttlePair() = %v", err)
	}

	want := pauserecord.Geometry{RecordPath: geometry.RecordPath}
	if got := pair.runner.PauseGeometry(); got != want {
		t.Errorf("runner PauseGeometry() = %+v; want %+v, the hub's record path with no slug", got, want)
	}
}
