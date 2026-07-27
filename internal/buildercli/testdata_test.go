// testdata_test.go holds the pure file-I/O plan-fixture helpers and
// git-free test doubles shared by every tier: builderengineTestdataDir and
// seedPlanFixture spawn no git, and pollFakeEngine/pollFakeReed are plain
// shuttleengine doubles, so all four stay untagged and available to
// Tier 1 (e.g. run_test.go) as well as the integration-tagged fixtures
// (validate_test.go, poll_test.go, spawnbatch_test.go) and the smoke tier
// (smoke_test.go). Kept in one place so there is exactly one definition
// regardless of which tier compiles it in. The scratch-git helpers the
// integration and smoke tiers share live in gitfixture_test.go instead,
// behind an `integration || smoke` tag, since they spawn real git.

package buildercli

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/Knatte18/loomyard/internal/hubgeometry"
	"github.com/Knatte18/loomyard/internal/reedengine"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
)

// pollFakeEngine is a minimal shuttleengine.Engine double for
// builderengine.TurnEnded: only ParseEvents is scripted, mirroring
// builderengine's own poll_test.go fakeEngine. Used by poll_test.go and
// smoke_test.go, hence untagged here rather than integration-tagged.
type pollFakeEngine struct {
	events []shuttleengine.Event
}

func (e *pollFakeEngine) Prepare(runDir string, spec shuttleengine.Spec, cfg shuttleengine.Config) (shuttleengine.Launch, error) {
	return shuttleengine.Launch{}, nil
}
func (e *pollFakeEngine) ParseEvents(data []byte) ([]shuttleengine.Event, error) {
	return e.events, nil
}
func (e *pollFakeEngine) Startup(capture string) shuttleengine.StartupState {
	return shuttleengine.StartupPending
}
func (e *pollFakeEngine) InterruptSequence() []shuttleengine.PaneInput      { return nil }
func (e *pollFakeEngine) TrustDismissSequence() []shuttleengine.PaneInput   { return nil }
func (e *pollFakeEngine) ComposeSend(text string) []shuttleengine.PaneInput { return nil }

// AuditForks is never reached: this double never runs fork-mode specs.
func (e *pollFakeEngine) AuditForks(sessionID, workdir string) (shuttleengine.ForkAudit, error) {
	return shuttleengine.ForkAudit{}, nil
}

// AuditForksIncremental is never reached, for the same reason as AuditForks.
func (e *pollFakeEngine) AuditForksIncremental(sessionID, workdir string, seenTranscripts map[string]bool) (shuttleengine.ForkAudit, error) {
	return shuttleengine.ForkAudit{}, nil
}

// ModelSwitchSequence is never reached: poll never drives a model switch.
func (e *pollFakeEngine) ModelSwitchSequence(model string) []shuttleengine.PaneInput {
	return nil
}

var _ shuttleengine.Engine = (*pollFakeEngine)(nil)

// pollFakeReed is a minimal shuttleengine.ReedOps double for
// builderengine.StrandLive and poll's terminal cleanup: Status is scripted,
// and RemoveStrand records every call so a test can assert whether the
// terminal branch released the strand. Also used by run_test.go's
// newRunFixture as an inert reed double.
type pollFakeReed struct {
	status         reedengine.StatusResult
	removedStrands []string
}

func (m *pollFakeReed) AddStrand(spec reedengine.AddSpec) (reedengine.Strand, error) {
	return reedengine.Strand{}, nil
}
func (m *pollFakeReed) RemoveStrand(guid string, recursive bool) (reedengine.Removed, error) {
	m.removedStrands = append(m.removedStrands, guid)
	return reedengine.Removed{}, nil
}
func (m *pollFakeReed) Status() (reedengine.StatusResult, error)      { return m.status, nil }
func (m *pollFakeReed) SendText(guid, text string, submit bool) error { return nil }
func (m *pollFakeReed) SendKey(guid, key string) error                { return nil }
func (m *pollFakeReed) CapturePane(guid string) (string, error)       { return "", nil }

var _ shuttleengine.ReedOps = (*pollFakeReed)(nil)

// builderengineTestdataDir returns the absolute path to
// internal/builderengine/testdata/<name>, resolved from this source file's
// own location via runtime.Caller rather than a cwd-relative path: tests
// that seed a fixture call t.Chdir into a scratch worktree first, which
// would otherwise break a plain "../builderengine/testdata/..." relative
// lookup.
func builderengineTestdataDir(name string) string {
	_, thisFile, _, _ := runtime.Caller(0)
	return filepath.Join(filepath.Dir(thisFile), "..", "builderengine", "testdata", name)
}

// seedPlanFixture copies every top-level file from srcDir (one of
// builderengine's own testdata plan fixtures) into hub's plan dir
// (hubgeometry.PlanDir(hub)) -- the Hub Geometry Invariant's own helper,
// never a hand-joined path -- AND into hub itself. The second copy matters
// because validateCmd resolves every card's typed file-op paths against
// c.layout.Cwd (hub, this package's worktreeRoot), never against planDir;
// per the fixture-self-reference decision a fixture's own card paths (e.g.
// plan-valid's Moves: source) are worktree-relative paths that resolve only
// against the fixture directory itself, so builderengine's own tests pass
// that directory as WorktreeRoot directly. buildercli's hub/planDir split
// has no single directory that is both, so both copies are required for
// batch 2's on-disk move-source-missing/move-target-collision checks to
// resolve the same fixture correctly here.
func seedPlanFixture(t *testing.T, hub, srcDir string) {
	t.Helper()

	dstDir := hubgeometry.PlanDir(hub)
	if err := os.MkdirAll(dstDir, 0o755); err != nil {
		t.Fatalf("mkdir plan dir: %v", err)
	}
	entries, err := os.ReadDir(srcDir)
	if err != nil {
		t.Fatalf("ReadDir(%q): %v", srcDir, err)
	}
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		data, err := os.ReadFile(filepath.Join(srcDir, e.Name()))
		if err != nil {
			t.Fatalf("ReadFile(%q): %v", e.Name(), err)
		}
		if err := os.WriteFile(filepath.Join(dstDir, e.Name()), data, 0o644); err != nil {
			t.Fatalf("WriteFile(%q): %v", e.Name(), err)
		}
		if err := os.WriteFile(filepath.Join(hub, e.Name()), data, 0o644); err != nil {
			t.Fatalf("WriteFile(%q): %v", e.Name(), err)
		}
	}
}
