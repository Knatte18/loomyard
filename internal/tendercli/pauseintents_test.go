// pauseintents_test.go pins the two pause-intent steps the catch-up and the upgrade's first read wire:
// the pause entries written from the intents of the runs a machine owns,
// and a recorded intent whose owed push is an error.
// The tests run over a hubforge hub and set the strand-name variable, so none calls t.Parallel.

package tendercli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/agentname"
	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/gitkit"
	"github.com/Knatte18/loomyard/internal/hubforge"
	"github.com/Knatte18/loomyard/internal/hubgeom"
	"github.com/Knatte18/loomyard/internal/pauserecord"
	"github.com/Knatte18/loomyard/internal/shedrun"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// writeBattenRecord writes content as the batten record of slug on the hub's prime.
func writeBattenRecord(t *testing.T, h *hubforge.Hub, slug, content string) {
	t.Helper()
	statusPath := shedrun.StatusFile(h.Location, slug)
	if err := os.MkdirAll(filepath.Dir(statusPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(statusPath, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

const (
	pausedWithIntent = `{"state":"paused","current_producer":"Run-Shed","pause_intent":{"by":"orch","at":"2026-11-15T09:00:00Z"}}`
	pausedNoIntent   = `{"state":"paused","current_producer":"Run-Shed"}`
)

func TestPauseIntentEntries(t *testing.T) {
	tiergate.Require(t, 1)
	t.Setenv(agentname.StrandNameEnv, "")
	h := newHub(t)
	writeBattenRecord(t, h, "unreadable", "{not json")
	writeBattenRecord(t, h, "with-intent", pausedWithIntent)
	writeBattenRecord(t, h, "without-intent", pausedNoIntent)
	writeBattenRecord(t, h, "already-paused", pausedWithIntent)
	geometry, err := hubgeom.PauseGeometry(h.Location)
	if err != nil {
		t.Fatal(err)
	}
	seeded := pauserecord.Entry{Scope: "already-paused", Caller: pauserecord.AdoptCaller, PausedAt: time.Unix(1000, 0)}
	if _, err := pauserecord.Update(geometry.RecordPath, time.Now(), func(record *pauserecord.Record) error {
		record.Live = []pauserecord.Entry{seeded}
		return nil
	}); err != nil {
		t.Fatal(err)
	}

	err = PauseIntentEntries(h.Location, []string{"unreadable", "with-intent", "without-intent", "already-paused", "no-record"})

	if err == nil || !strings.Contains(err.Error(), "unreadable") {
		t.Errorf("PauseIntentEntries() error = %v; want one naming the run whose record does not read", err)
	}
	live, _ := liveEntries(t, h)
	byScope := map[string]pauserecord.Entry{}
	for _, entry := range live {
		byScope[entry.Scope] = entry
	}
	if len(live) != 2 {
		t.Errorf("live entries = %+v; want the one written beside the seeded one", live)
	}
	if entry, ok := byScope["with-intent"]; !ok || entry.Caller != agentname.External {
		t.Errorf("entry of the run with an intent = %+v, %v; want one under the default caller", entry, ok)
	}
	if entry := byScope["already-paused"]; entry.Caller != pauserecord.AdoptCaller || !entry.PausedAt.Equal(seeded.PausedAt) {
		t.Errorf("entry of the run with a live entry = %+v; want the seeded entry unchanged", entry)
	}
}

func TestRecordPauseIntent_ReturnsAnOwedPush(t *testing.T) {
	tiergate.Require(t, 1)
	t.Setenv(agentname.StrandNameEnv, "")
	h := newHub(t)
	writeBattenRecord(t, h, "run", pausedNoIntent)
	// The hub's records branch has no upstream, so the push goes once with no backoff and the unreachable origin is owed.
	gitkit.Git(t, h.PrimeRecords(), "remote", "set-url", "origin", "http://127.0.0.1:1/x")

	err := RecordPauseIntent(h.Location, "run")

	if err == nil || !fabricengine.IsPushOwed(err) {
		t.Errorf("RecordPauseIntent() error = %v; want the owed push's error", err)
	}
}
