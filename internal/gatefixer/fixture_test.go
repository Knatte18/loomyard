// fixture_test.go holds the fakes the package's tests share.
// A real shuttle runner over the shuttlefake reed and engine sits behind a recording starter and stopper.
// The fixture also has an in-memory record store and a scripted clock.

package gatefixer

import (
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/segmentcolor"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
	"github.com/Knatte18/loomyard/internal/testkit/shuttlefake"
)

// fakeClock is a clock whose time starts fixed and advances only when Sleep is called or a test sets now.
type fakeClock struct {
	now time.Time
}

func (c *fakeClock) Now() time.Time        { return c.now }
func (c *fakeClock) Sleep(d time.Duration) { c.now = c.now.Add(d) }

// recordingStarter starts through inner and records every spec it was handed;
// a non-nil err makes Start fail before inner runs.
type recordingStarter struct {
	inner Starter
	err   error
	specs []shuttleengine.Spec
}

func (s *recordingStarter) Start(spec shuttleengine.Spec) (*shuttleengine.Run, error) {
	s.specs = append(s.specs, spec)
	if s.err != nil {
		return nil, s.err
	}
	return s.inner.Start(spec)
}

// recordingStopper records every guid it was asked to stop and fails with err when set.
type recordingStopper struct {
	mu    sync.Mutex
	guids []string
	err   error
}

func (s *recordingStopper) StopStrand(guid string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.guids = append(s.guids, guid)
	return s.err
}

// memoryStore is a Store over a slice; a non-nil liveErr or saveErr makes that call fail.
type memoryStore struct {
	records []*Record
	liveErr error
	saveErr error
}

func (s *memoryStore) Live() (*Record, error) {
	if s.liveErr != nil {
		return nil, s.liveErr
	}
	for i := len(s.records) - 1; i >= 0; i-- {
		if s.records[i].Status == "" {
			return s.records[i], nil
		}
	}
	return nil, nil
}

func (s *memoryStore) Save(rec *Record) error {
	if s.saveErr != nil {
		return s.saveErr
	}
	for i, existing := range s.records {
		if existing.StrandGUID == rec.StrandGUID {
			s.records[i] = rec
			return nil
		}
	}
	s.records = append(s.records, rec)
	return nil
}

// fixture wires Deps over fakes, with the store and the spec a spawn takes.
type fixture struct {
	deps      Deps
	reed      *shuttlefake.Reed
	engine    *shuttlefake.Engine
	starter   *recordingStarter
	stopper   *recordingStopper
	clock     *fakeClock
	store     *memoryStore
	spec      Spec
	replyPath string
	// registered holds the guid of every Register call.
	registered []string
}

// newFixture returns a fixture whose starter runs a real shuttle runner over the shuttlefake reed and engine.
// The clock starts at the Unix epoch.
func newFixture(t *testing.T) *fixture {
	t.Helper()
	anchor := t.TempDir()
	reed := &shuttlefake.Reed{}
	engine := &shuttlefake.Engine{}
	shuttleCfg := shuttleengine.Config{RunDir: t.TempDir(), RunTimeoutMin: 60, StartupTimeoutS: 30}
	runner := shuttleengine.NewRunner(reed, shuttlefake.EngineSet(engine), anchor, anchor, shuttleCfg)

	fx := &fixture{
		reed:      reed,
		engine:    engine,
		starter:   &recordingStarter{inner: runner},
		stopper:   &recordingStopper{},
		clock:     &fakeClock{now: time.Unix(0, 0)},
		store:     &memoryStore{},
		replyPath: filepath.Join(t.TempDir(), "reply.md"),
	}
	fx.deps = Deps{
		Starter:    fx.starter,
		Stopper:    fx.stopper,
		Reed:       reed,
		Engines:    shuttlefake.EngineSet(engine),
		ShuttleCfg: shuttleCfg,
		AnchorRoot: anchor,
		Clock:      fx.clock,
		Register: func(guid string) error {
			fx.registered = append(fx.registered, guid)
			return nil
		},
	}
	fx.spec = Spec{
		Prompt:      []byte("fix the failures"),
		ReportPath:  "/lym/report.md",
		ReplyPath:   fx.replyPath,
		Role:        RoleGateFixer,
		ModelChoice: shuttleengine.ModelChoice{Model: "fixer-model"},
		Skills:      []string{"scribe:prose"},
		Segment:     segmentcolor.Webster,
		Timeout:     30 * time.Minute,
	}
	return fx
}

// spawn spawns the fixture's fixer and returns its record.
func (fx *fixture) spawn(t *testing.T) *Record {
	t.Helper()
	rec, spawned, err := SpawnOrAttach(fx.deps, fx.store, fx.spec)
	if err != nil || !spawned {
		t.Fatalf("SpawnOrAttach() = _, %v, %v; want a spawn", spawned, err)
	}
	return rec
}
