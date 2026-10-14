//go:build integration

// cli_test.go drives `lyx tender pause`, `status` and `resume` through the cobra commands over a hubforge hub with one pair and no agents running, and the network gate installer over the same hub.
// No test calls t.Parallel: each sets the strand-name variable or reads the process-wide network gate.

package pausecli

import (
	"bytes"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/cobra"

	"github.com/Knatte18/loomyard/internal/agentname"
	"github.com/Knatte18/loomyard/internal/clihelp"
	"github.com/Knatte18/loomyard/internal/hubforge"
	"github.com/Knatte18/loomyard/internal/hubgeom"
	"github.com/Knatte18/loomyard/internal/lyxcwd"
	"github.com/Knatte18/loomyard/internal/pauserecord"
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
	badGrace, code := verb(t, Command(), prime, "pause", "--grace", "61")
	if badGrace.OK || code == 0 || !strings.Contains(badGrace.Error, "0 to 60") {
		t.Errorf("--grace 61: ok=%v error=%q; want a refusal naming the range", badGrace.OK, badGrace.Error)
	}

	slugResume, code := verb(t, Command(), prime, "resume", pairSlug)
	if slugResume.OK || code == 0 || !strings.Contains(slugResume.Error, `"lyx resume"`) {
		t.Errorf("resume <slug> under a hub entry: ok=%v error=%q; want a refusal naming lyx resume", slugResume.OK, slugResume.Error)
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

func TestPauseAndResume_ApplyTheCallerRules(t *testing.T) {
	h := newHub(t)
	prime := h.PrimeWorktree()
	agent, err := agentname.Format("lyx", pairSlug, "worker")
	if err != nil {
		t.Fatal(err)
	}
	orch, err := agentname.Format("lyx", "", agentname.RoleOrch)
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
}

func TestInstallNetworkGate_TellsThePairsSlugAndInstallsNothingOutsideAHub(t *testing.T) {
	t.Setenv(agentname.StrandNameEnv, "")
	h := newHub(t)
	allAlive := func(int, string) bool { return true }
	sentinel := pauserecord.Geometry{RecordPath: filepath.Join(t.TempDir(), "sentinel", pauserecord.RecordFileName)}
	t.Cleanup(pauserecord.InstallGate(pauserecord.Gate{Geometry: sentinel}))

	InstallNetworkGate(lyxcwd.WithCwd(t.Context(), t.TempDir()))
	_, probeDone := pauserecord.Network("probe")
	probeDone()
	pauserecord.Network("left-in-flight")
	if ops, err := pauserecord.ListNetOps(sentinel.RecordPath, allAlive); err != nil || len(ops) != 1 || ops[0].Op != "left-in-flight" {
		t.Fatalf("operations under the sentinel gate = %+v, %v; want the one left in flight, since nothing was installed outside a hub", ops, err)
	}

	InstallNetworkGate(lyxcwd.WithCwd(t.Context(), h.PairCodeWorktree(pairSlug)))
	pauserecord.Network("pair-probe")
	geometry, err := hubgeom.PauseGeometry(h.Location)
	if err != nil {
		t.Fatal(err)
	}
	ops, err := pauserecord.ListNetOps(geometry.RecordPath, allAlive)
	if err != nil || len(ops) != 1 || ops[0].Slug != pairSlug || ops[0].Op != "pair-probe" {
		t.Errorf("operations under the hub's record = %+v, %v; want one marked with the pair's slug", ops, err)
	}
}
