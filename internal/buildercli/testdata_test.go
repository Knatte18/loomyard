// testdata_test.go holds the pure file-I/O plan-fixture helpers and
// git-free test doubles shared by both tiers: builderengineTestdataDir and
// seedPlanFixture spawn no git, and pollFakeMux is a plain
// shuttleengine.MuxOps double, so all three stay untagged and available to
// Tier 1 (e.g. run_test.go) as well as the integration-tagged fixtures that
// also use them (validate_test.go, poll_test.go, spawnbatch_test.go,
// smoke_test.go). Kept in one place so there is exactly one definition
// regardless of which tier compiles it in.

package buildercli

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/Knatte18/loomyard/internal/hubgeometry"
	"github.com/Knatte18/loomyard/internal/muxengine"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
)

// pollFakeMux is a minimal shuttleengine.MuxOps double for
// builderengine.StrandLive and poll's terminal cleanup: Status is scripted,
// and RemoveStrand records every call so a test can assert whether the
// terminal branch released the strand. Also used by run_test.go's
// newRunFixture as an inert mux double.
type pollFakeMux struct {
	status         muxengine.StatusResult
	removedStrands []string
}

func (m *pollFakeMux) AddStrand(spec muxengine.AddSpec) (muxengine.Strand, error) {
	return muxengine.Strand{}, nil
}
func (m *pollFakeMux) RemoveStrand(guid string, recursive bool) (muxengine.Removed, error) {
	m.removedStrands = append(m.removedStrands, guid)
	return muxengine.Removed{}, nil
}
func (m *pollFakeMux) Status() (muxengine.StatusResult, error)       { return m.status, nil }
func (m *pollFakeMux) SendText(guid, text string, submit bool) error { return nil }
func (m *pollFakeMux) SendKey(guid, key string) error                { return nil }
func (m *pollFakeMux) CapturePane(guid string) (string, error)       { return "", nil }

var _ shuttleengine.MuxOps = (*pollFakeMux)(nil)

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
