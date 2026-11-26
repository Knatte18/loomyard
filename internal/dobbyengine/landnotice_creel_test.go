// landnotice_creel_test.go shows the landing notice end to end.
// A dobby daemon's notice goes through a real creel store, scheduler and sweep.
// It reaches an orch strand that is listed only after the landing.

package dobbyengine

import (
	"context"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/creelengine"
	"github.com/Knatte18/loomyard/internal/testkit/creelfake"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

//testtiming:keep delivery through a real creel store to an orch strand listed after the landing; coverage of its blocks by other tests does not show an assertion of this
func TestLandNotice_ReachesAnOrchStrandListedAfterTheLanding(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()
	const (
		orchName = "ly:orch"
		driver   = "ly:alpha:driver"
		peer     = "ly:beta:driver"
		wantLine = "[dobby notice] task: landed as 0123456789ab"
	)

	root := t.TempDir()
	geom := creelengine.Geometry{
		HubPath: filepath.Join(root, "hub"), Shortname: "ly", StateDir: filepath.Join(root, "state"),
		JournalDir: filepath.Join(root, "state", "journal"),
	}
	delivery := creelfake.NewDelivery()
	delivery.SetStrands(creelfake.LiveStrand(driver), creelfake.LiveStrand(peer))
	clock := creelfake.NewClock(time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC))
	store, err := creelengine.OpenStore(geom, delivery, clock)
	if err != nil {
		t.Fatalf("OpenStore() error = %v; want nil", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	scheduler := creelengine.NewScheduler(store, delivery, clock)
	scheduler.Start()
	t.Cleanup(scheduler.Stop)
	// Mail held by a listed strand keeps the sweep running while the orch is down.
	if _, err := store.PostWith(creelengine.StrandSender(driver), peer, "keeps the sweep running", creelengine.PostOptions{}); err != nil {
		t.Fatalf("Post(%s -> %s) error = %v; want nil", driver, peer, err)
	}

	h := newPassHarness(t)
	var posts int
	h.d.deps.LandNotice = func(string) (bool, error) { return true, nil }
	h.d.deps.Notice = func(_ context.Context, line string) error {
		posts++
		_, err := store.PostWith(creelengine.ProgramSender("ly:DOBBY"), orchName, line, creelengine.PostOptions{})
		return err
	}
	h.fake.runs = []RunView{{Slug: "task", State: runStateRunning, Producer: producerRunShed}}
	h.tick(false)
	h.fake.runs = []RunView{{Slug: "task", State: runStateDone, Producer: "Worktree-Teardown", PastLand: true, Landed: true, LandingCommit: "0123456789abcdef0123456789abcdef01234567"}}
	h.tick(false)

	if posts != 1 {
		t.Fatalf("landing tick posted %d lines; want one", posts)
	}
	if memory := h.stateRecord().LandNotices; len(memory) != 0 {
		t.Errorf("state LandNotices = %+v; want none once the notice is settled", memory)
	}

	// The sweep tells creel's own waits from the delivery workers' by length.
	// Workers wait up to 8 seconds; the sweep waits up to 50.
	sweepWaits := func(n int) []time.Duration {
		return clock.AwaitWaitsWhere(n, func(d time.Duration) bool { return d > 8*time.Second && d <= 50*time.Second })
	}
	for range 4 {
		waits := sweepWaits(1)
		clock.Advance(waits[len(waits)-1])
		sweepWaits(len(waits) + 1)
	}

	// Stopping waits for the delivery workers the last advance woke.
	// One still probing once the orch is listed would type the notice into its pane ahead of the read.
	scheduler.Stop()
	delivery.SetStrands(creelfake.LiveStrand(driver), creelfake.LiveStrand(peer), creelfake.LiveStrand(orchName))
	reading, err := store.Read(creelengine.StrandSender(orchName), "")
	if err != nil {
		t.Fatalf("Read(%s) error = %v; want nil", orchName, err)
	}
	if err := reading.Ack(); err != nil {
		t.Fatalf("Ack() error = %v; want nil", err)
	}
	var bodies []string
	for _, msg := range reading.Messages {
		bodies = append(bodies, msg.Body)
	}
	if !slices.Equal(bodies, []string{wantLine}) {
		t.Errorf("orch strand read %q; want %q", bodies, []string{wantLine})
	}

	h.tick(false)
	if posts != 1 {
		t.Errorf("Notice calls = %d after a tick with the orch listed; want the one line only", posts)
	}
}
