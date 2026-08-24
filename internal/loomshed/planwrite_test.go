// planwrite_test.go exercises NewPlanWrite's rotate-then-delegate-then-commit sequencing against a
// fake inner shedengine.ShedProducer and a real filesystem tree built under t.TempDir(): rotation
// runs before the inner Call, commit fires exactly once and only on a Done outcome with a nil
// error, and rotation/commit failures surface as wrapped errors rather than shedengine.Stuck. No
// test in this file spawns a process, keeping the file tier 1 per the Test Tier Purity Invariant.
// It is modelled on discussionwrite_test.go but declares its own fake types
// (rotationAwareInnerProducer, planCommitRecorder) rather than reusing that file's
// fakeInnerProducer/commitRecorder, which already exist in this package.

package loomshed

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/planparser"
	"github.com/Knatte18/loomyard/internal/shedengine"
)

// rotationAwareInnerProducer is a caller-settable shedengine.ShedProducer stand-in that also
// records, at Call time, the names of every .md file present directly under planDir -- letting a
// test prove rotation ran before this producer was invoked.
type rotationAwareInnerProducer struct {
	planDir       string
	outcome       shedengine.Outcome
	pointer       shedengine.OutputPointer
	err           error
	calls         int
	mdNamesAtCall []string
}

func (f *rotationAwareInnerProducer) Call(ctx context.Context) (shedengine.Outcome, shedengine.OutputPointer, error) {
	f.calls++
	f.mdNamesAtCall = readMDNames(f.planDir)
	return f.outcome, f.pointer, f.err
}

// readMDNames returns the sorted-by-ReadDir-order names of every top-level ".md" file under dir,
// or nil if dir does not exist.
func readMDNames(dir string) []string {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	var names []string
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".md") {
			names = append(names, e.Name())
		}
	}
	return names
}

// planCommitRecorder is a caller-settable commit closure stand-in: Commit returns commitErr and
// records how many times it was invoked.
type planCommitRecorder struct {
	commitErr error
	calls     int
}

func (c *planCommitRecorder) Commit() error {
	c.calls++
	return c.commitErr
}

// setupPlanDir creates a plan directory (_lyx/plan) under a fresh t.TempDir() anchor, writes the
// given top-level file names (each with content equal to its own name) directly under it, and
// returns the anchor path and the plan directory's absolute path.
func setupPlanDir(t *testing.T, fileNames ...string) (anchorPath, planDir string) {
	t.Helper()
	anchorPath = t.TempDir()
	planDir = planparser.PlanDir(anchorPath)
	if err := os.MkdirAll(planDir, 0o755); err != nil {
		t.Fatalf("MkdirAll(%q) error = %v", planDir, err)
	}
	for _, name := range fileNames {
		if err := os.WriteFile(filepath.Join(planDir, name), []byte(name), 0o644); err != nil {
			t.Fatalf("WriteFile(%q) error = %v", name, err)
		}
	}
	return anchorPath, planDir
}

func TestPlanWrite_Call(t *testing.T) {
	fixedNow := func() time.Time {
		return time.Date(2026, 8, 24, 17, 0, 0, 0, time.UTC)
	}

	t.Run("DoneWithNilErrorInvokesCommitOnceAndReturnsInnerVerbatim", func(t *testing.T) {
		anchorPath, planDir := setupPlanDir(t, "00-overview.md")
		inner := &rotationAwareInnerProducer{
			planDir: planDir,
			outcome: shedengine.Done,
			pointer: shedengine.OutputPointer{Path: "00-overview.md"},
		}
		commit := &planCommitRecorder{}

		p := NewPlanWrite("Plan-Write", inner, commit.Commit, anchorPath, fixedNow)
		outcome, pointer, err := p.Call(context.Background())
		if err != nil {
			t.Fatalf("Call() error = %v; want nil", err)
		}
		if outcome != shedengine.Done {
			t.Errorf("Call() outcome = %q; want %q", outcome, shedengine.Done)
		}
		if pointer != inner.pointer {
			t.Errorf("Call() pointer = %+v; want %+v", pointer, inner.pointer)
		}
		if commit.calls != 1 {
			t.Errorf("commit.calls = %d; want 1", commit.calls)
		}
		if inner.calls != 1 {
			t.Errorf("inner.calls = %d; want 1", inner.calls)
		}
	})

	t.Run("StuckLeavesCommitUninvokedAndReturnsInnerVerbatim", func(t *testing.T) {
		anchorPath, planDir := setupPlanDir(t, "00-overview.md")
		inner := &rotationAwareInnerProducer{planDir: planDir, outcome: shedengine.Stuck}
		commit := &planCommitRecorder{}

		p := NewPlanWrite("Plan-Write", inner, commit.Commit, anchorPath, fixedNow)
		outcome, pointer, err := p.Call(context.Background())
		if err != nil {
			t.Fatalf("Call() error = %v; want nil", err)
		}
		if outcome != shedengine.Stuck {
			t.Errorf("Call() outcome = %q; want %q", outcome, shedengine.Stuck)
		}
		if pointer != (shedengine.OutputPointer{}) {
			t.Errorf("Call() pointer = %+v; want the zero value", pointer)
		}
		if commit.calls != 0 {
			t.Errorf("commit.calls = %d; want 0", commit.calls)
		}
	})

	t.Run("InnerErrorLeavesCommitUninvoked", func(t *testing.T) {
		anchorPath, planDir := setupPlanDir(t, "00-overview.md")
		innerErr := errors.New("inner producer failed")
		inner := &rotationAwareInnerProducer{planDir: planDir, err: innerErr}
		commit := &planCommitRecorder{}

		p := NewPlanWrite("Plan-Write", inner, commit.Commit, anchorPath, fixedNow)
		_, _, err := p.Call(context.Background())
		if !errors.Is(err, innerErr) {
			t.Errorf("Call() error = %v; want it to wrap %v", err, innerErr)
		}
		if commit.calls != 0 {
			t.Errorf("commit.calls = %d; want 0", commit.calls)
		}
	})

	t.Run("CommitErrorSurfacesAsErrorNamingProducerWithEmptyOutcome", func(t *testing.T) {
		anchorPath, planDir := setupPlanDir(t, "00-overview.md")
		commitErr := errors.New("git commit failed")
		inner := &rotationAwareInnerProducer{
			planDir: planDir,
			outcome: shedengine.Done,
			pointer: shedengine.OutputPointer{Path: "00-overview.md"},
		}
		commit := &planCommitRecorder{commitErr: commitErr}

		p := NewPlanWrite("Plan-Write", inner, commit.Commit, anchorPath, fixedNow)
		outcome, pointer, err := p.Call(context.Background())
		if !errors.Is(err, commitErr) {
			t.Errorf("Call() error = %v; want it to wrap %v", err, commitErr)
		}
		if outcome != "" {
			t.Errorf("Call() outcome = %q; want the empty value, never %q", outcome, shedengine.Stuck)
		}
		if pointer != (shedengine.OutputPointer{}) {
			t.Errorf("Call() pointer = %+v; want the zero value", pointer)
		}
		if !strings.Contains(err.Error(), "Plan-Write") {
			t.Errorf("Call() error = %q; want it to name the producer %q", err.Error(), "Plan-Write")
		}
	})

	t.Run("RotationHappensBeforeInnerCall", func(t *testing.T) {
		anchorPath, planDir := setupPlanDir(t, "00-overview.md", "01-card-one.md")
		inner := &rotationAwareInnerProducer{planDir: planDir, outcome: shedengine.Done}
		commit := &planCommitRecorder{}

		p := NewPlanWrite("Plan-Write", inner, commit.Commit, anchorPath, fixedNow)
		if _, _, err := p.Call(context.Background()); err != nil {
			t.Fatalf("Call() error = %v; want nil", err)
		}
		if len(inner.mdNamesAtCall) != 0 {
			t.Errorf("inner saw %d .md files at call time; want 0 (rotation must run first)", len(inner.mdNamesAtCall))
		}
	})

	t.Run("RotationMovesEveryTopLevelMDFilePreservingContent", func(t *testing.T) {
		anchorPath, planDir := setupPlanDir(t, "00-overview.md", "01-card-one.md")
		inner := &rotationAwareInnerProducer{planDir: planDir, outcome: shedengine.Done}
		commit := &planCommitRecorder{}

		p := NewPlanWrite("Plan-Write", inner, commit.Commit, anchorPath, fixedNow)
		if _, _, err := p.Call(context.Background()); err != nil {
			t.Fatalf("Call() error = %v; want nil", err)
		}

		archiveDir := filepath.Join(planDir, planparser.ArchiveDirName(fixedNow().UTC().Format(archiveTimestampFormat), ""))
		for _, name := range []string{"00-overview.md", "01-card-one.md"} {
			data, err := os.ReadFile(filepath.Join(archiveDir, name))
			if err != nil {
				t.Fatalf("ReadFile(%q) error = %v", name, err)
			}
			if string(data) != name {
				t.Errorf("archived %s content = %q; want %q", name, string(data), name)
			}
			if _, err := os.Stat(filepath.Join(planDir, name)); !os.IsNotExist(err) {
				t.Errorf("%s still present at plan directory root after rotation", name)
			}
		}
	})

	t.Run("PreexistingArchiveSubdirIsNotNestedInsideNewOne", func(t *testing.T) {
		anchorPath, planDir := setupPlanDir(t, "00-overview.md")
		oldArchive := filepath.Join(planDir, planparser.ArchiveDirName("20260101T000000Z", ""))
		if err := os.MkdirAll(oldArchive, 0o755); err != nil {
			t.Fatalf("MkdirAll(%q) error = %v", oldArchive, err)
		}
		if err := os.WriteFile(filepath.Join(oldArchive, "00-overview.md"), []byte("old"), 0o644); err != nil {
			t.Fatalf("WriteFile error = %v", err)
		}

		inner := &rotationAwareInnerProducer{planDir: planDir, outcome: shedengine.Done}
		commit := &planCommitRecorder{}
		p := NewPlanWrite("Plan-Write", inner, commit.Commit, anchorPath, fixedNow)
		if _, _, err := p.Call(context.Background()); err != nil {
			t.Fatalf("Call() error = %v; want nil", err)
		}

		if _, err := os.Stat(oldArchive); err != nil {
			t.Errorf("pre-existing archive dir %q missing after rotation: %v", oldArchive, err)
		}
		if _, err := os.Stat(filepath.Join(oldArchive, planparser.ArchiveDirName(fixedNow().UTC().Format(archiveTimestampFormat), ""))); !os.IsNotExist(err) {
			t.Errorf("new archive directory was nested inside the pre-existing one")
		}
	})

	t.Run("RotationOverAbsentPlanDirectoryIsNoOp", func(t *testing.T) {
		anchorPath := t.TempDir()
		planDir := planparser.PlanDir(anchorPath)
		inner := &rotationAwareInnerProducer{planDir: planDir, outcome: shedengine.Done}
		commit := &planCommitRecorder{}

		p := NewPlanWrite("Plan-Write", inner, commit.Commit, anchorPath, fixedNow)
		if _, _, err := p.Call(context.Background()); err != nil {
			t.Fatalf("Call() error = %v; want nil", err)
		}
		if _, err := os.Stat(planDir); !os.IsNotExist(err) {
			t.Errorf("plan directory %q was created by rotation over an absent directory", planDir)
		}
	})

	t.Run("RotationOverEmptyPlanDirectoryCreatesNoArchiveDirectory", func(t *testing.T) {
		anchorPath, planDir := setupPlanDir(t)
		inner := &rotationAwareInnerProducer{planDir: planDir, outcome: shedengine.Done}
		commit := &planCommitRecorder{}

		p := NewPlanWrite("Plan-Write", inner, commit.Commit, anchorPath, fixedNow)
		if _, _, err := p.Call(context.Background()); err != nil {
			t.Fatalf("Call() error = %v; want nil", err)
		}

		entries, err := os.ReadDir(planDir)
		if err != nil {
			t.Fatalf("ReadDir(%q) error = %v", planDir, err)
		}
		if len(entries) != 0 {
			t.Errorf("plan directory has %d entries after rotating an empty directory; want 0", len(entries))
		}
	})

	t.Run("TwoRotationsUnderPinnedClockProduceStampThenStampDash1", func(t *testing.T) {
		anchorPath, planDir := setupPlanDir(t, "00-overview.md")
		inner := &rotationAwareInnerProducer{planDir: planDir, outcome: shedengine.Done}
		commit := &planCommitRecorder{}
		p := NewPlanWrite("Plan-Write", inner, commit.Commit, anchorPath, fixedNow)

		if _, _, err := p.Call(context.Background()); err != nil {
			t.Fatalf("first Call() error = %v; want nil", err)
		}
		if err := os.WriteFile(filepath.Join(planDir, "00-overview.md"), []byte("second"), 0o644); err != nil {
			t.Fatalf("WriteFile error = %v", err)
		}
		if _, _, err := p.Call(context.Background()); err != nil {
			t.Fatalf("second Call() error = %v; want nil", err)
		}

		stamp := fixedNow().UTC().Format(archiveTimestampFormat)
		first := filepath.Join(planDir, planparser.ArchiveDirName(stamp, ""))
		second := filepath.Join(planDir, planparser.ArchiveDirName(stamp, "-1"))
		if _, err := os.Stat(first); err != nil {
			t.Errorf("Stat(%q) error = %v; want the first rotation's archive directory to exist", first, err)
		}
		if _, err := os.Stat(second); err != nil {
			t.Errorf("Stat(%q) error = %v; want the second rotation's archive directory to exist", second, err)
		}
	})

	t.Run("RotationFailureReturnsErrorAndLeavesInnerCallCountZero", func(t *testing.T) {
		anchorPath := t.TempDir()
		planDir := planparser.PlanDir(anchorPath)
		// Pre-create a regular file at the plan directory's own path, portably forcing os.ReadDir
		// to fail with a not-a-directory error rather than a not-exist error.
		if err := os.MkdirAll(filepath.Dir(planDir), 0o755); err != nil {
			t.Fatalf("MkdirAll error = %v", err)
		}
		if err := os.WriteFile(planDir, []byte("not a directory"), 0o644); err != nil {
			t.Fatalf("WriteFile(%q) error = %v", planDir, err)
		}

		inner := &rotationAwareInnerProducer{planDir: planDir, outcome: shedengine.Done}
		commit := &planCommitRecorder{}
		p := NewPlanWrite("Plan-Write", inner, commit.Commit, anchorPath, fixedNow)
		_, _, err := p.Call(context.Background())
		if err == nil {
			t.Fatalf("Call() error = nil; want a non-nil rotation error")
		}
		if inner.calls != 0 {
			t.Errorf("inner.calls = %d; want 0", inner.calls)
		}
	})

	t.Run("NewPlanWriteWithNilNowNeitherPanicsNorErrorsAndStillProducesArchiveDirectory", func(t *testing.T) {
		anchorPath, planDir := setupPlanDir(t, "00-overview.md")
		inner := &rotationAwareInnerProducer{planDir: planDir, outcome: shedengine.Done}
		commit := &planCommitRecorder{}

		p := NewPlanWrite("Plan-Write", inner, commit.Commit, anchorPath, nil)
		if _, _, err := p.Call(context.Background()); err != nil {
			t.Fatalf("Call() error = %v; want nil", err)
		}

		entries, err := os.ReadDir(planDir)
		if err != nil {
			t.Fatalf("ReadDir(%q) error = %v", planDir, err)
		}
		var sawArchiveDir bool
		for _, e := range entries {
			if e.IsDir() && strings.HasPrefix(e.Name(), "archive-") {
				sawArchiveDir = true
			}
		}
		if !sawArchiveDir {
			t.Errorf("no archive-* directory found under %q after a nil-now rotation", planDir)
		}
	})
}
