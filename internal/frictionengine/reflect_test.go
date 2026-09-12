package frictionengine

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/friction"
	"github.com/Knatte18/loomyard/internal/modelspec"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
)

// reflectionStencilFixture is a minimal, valid reflection stencil carrying exactly the three markers
// buildReflectionSpec fills.
const reflectionStencilFixture = "# Reflection\n\nDir: {{.friction_dir}}\n\nReport: {{.report_path}}\n\nNotes:\n{{.note_list}}\n"

// fakeShuttle records every Spec it was handed and returns one scripted Result/error per call, in
// order, following mergeresolve's own fakeShuttle pattern.
type fakeShuttle struct {
	results []shuttleengine.Result
	errs    []error

	specs []shuttleengine.Spec
}

func (f *fakeShuttle) Run(spec shuttleengine.Spec) (shuttleengine.Result, error) {
	f.specs = append(f.specs, spec)
	callNumber := len(f.specs)

	var res shuttleengine.Result
	if callNumber-1 < len(f.results) {
		res = f.results[callNumber-1]
	}
	var err error
	if callNumber-1 < len(f.errs) {
		err = f.errs[callNumber-1]
	}
	return res, err
}

// fakeClock is the Clock seam a test injects to assert an exact archive directory name rather than a
// pattern.
type fakeClock struct {
	now time.Time
}

func (f fakeClock) Now() time.Time {
	return f.now
}

// newTestDeps returns a Deps wired against shuttle and clock, with a fresh friction directory and
// seeded reflection stencil under t.TempDir().
func newTestDeps(t *testing.T, shuttle Shuttle, clock Clock) Deps {
	t.Helper()

	root := t.TempDir()
	frictionDir := filepath.Join(root, "friction")
	stencilsDir := filepath.Join(root, "stencils")

	if err := os.MkdirAll(frictionDir, 0o755); err != nil {
		t.Fatalf("MkdirAll(frictionDir): %v", err)
	}
	if err := os.MkdirAll(filepath.Join(stencilsDir, "friction"), 0o755); err != nil {
		t.Fatalf("MkdirAll(stencilsDir/friction): %v", err)
	}
	if err := os.WriteFile(filepath.Join(stencilsDir, "friction", "friction-template-reflection.md"), []byte(reflectionStencilFixture), 0o644); err != nil {
		t.Fatalf("WriteFile(reflection stencil): %v", err)
	}

	return Deps{
		Shuttle:       shuttle,
		FrictionDir:   frictionDir,
		ArchivePrefix: filepath.Join(root, "friction-"),
		StencilsDir:   stencilsDir,
		FrictionSpec:  "claude:sonnet[effort=high]",
		Registry:      modelspec.Registry{},
		Timeout:       time.Minute,
		Clock:         clock,
	}
}

// writeNote writes a friction note named name (a bare stem, ".md" appended) with content into
// deps.FrictionDir.
func writeNote(t *testing.T, deps Deps, name, content string) {
	t.Helper()
	full := filepath.Join(deps.FrictionDir, name+".md")
	if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile(%s): %v", full, err)
	}
}

func TestReflect_MissingDirectory_Skipped(t *testing.T) {
	shuttle := &fakeShuttle{}
	deps := newTestDeps(t, shuttle, nil)
	if err := os.RemoveAll(deps.FrictionDir); err != nil {
		t.Fatalf("RemoveAll(frictionDir): %v", err)
	}

	report, err := Reflect(deps)
	if err != nil {
		t.Fatalf("Reflect() error = %v; want nil", err)
	}
	if report.Status != StatusSkipped {
		t.Errorf("Reflect() status = %q; want %q", report.Status, StatusSkipped)
	}
	if len(shuttle.specs) != 0 {
		t.Errorf("Shuttle.Run called %d times; want 0", len(shuttle.specs))
	}
}

func TestReflect_EmptyDirectory_Skipped(t *testing.T) {
	shuttle := &fakeShuttle{}
	deps := newTestDeps(t, shuttle, nil)

	report, err := Reflect(deps)
	if err != nil {
		t.Fatalf("Reflect() error = %v; want nil", err)
	}
	if report.Status != StatusSkipped {
		t.Errorf("Reflect() status = %q; want %q", report.Status, StatusSkipped)
	}
	if len(shuttle.specs) != 0 {
		t.Errorf("Shuttle.Run called %d times; want 0", len(shuttle.specs))
	}
}

func TestReflect_OnlyNonMarkdownFiles_Skipped(t *testing.T) {
	shuttle := &fakeShuttle{}
	deps := newTestDeps(t, shuttle, nil)
	if err := os.WriteFile(filepath.Join(deps.FrictionDir, "notes.txt"), []byte("hello"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	report, err := Reflect(deps)
	if err != nil {
		t.Fatalf("Reflect() error = %v; want nil", err)
	}
	if report.Status != StatusSkipped {
		t.Errorf("Reflect() status = %q; want %q", report.Status, StatusSkipped)
	}
	if len(shuttle.specs) != 0 {
		t.Errorf("Shuttle.Run called %d times; want 0", len(shuttle.specs))
	}
}

// TestReflect_OnlyStaleReport_Skipped covers the stale-report-from-a-timed-out-run case: a directory
// whose only .md entry is friction.ReportFileName must not be mistaken for one note.
func TestReflect_OnlyStaleReport_Skipped(t *testing.T) {
	shuttle := &fakeShuttle{}
	deps := newTestDeps(t, shuttle, nil)
	if err := os.WriteFile(filepath.Join(deps.FrictionDir, friction.ReportFileName), []byte("stale"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	report, err := Reflect(deps)
	if err != nil {
		t.Fatalf("Reflect() error = %v; want nil", err)
	}
	if report.Status != StatusSkipped {
		t.Errorf("Reflect() status = %q; want %q", report.Status, StatusSkipped)
	}
	if len(shuttle.specs) != 0 {
		t.Errorf("Shuttle.Run called %d times; want 0", len(shuttle.specs))
	}
}

// TestReflect_OneNote_SpawnsAndArchives covers the full happy path: exactly one spawn, the composed
// Spec's shape, the prompt naming the friction directory, and the archive-and-recreate.
func TestReflect_OneNote_SpawnsAndArchives(t *testing.T) {
	shuttle := &fakeShuttle{results: []shuttleengine.Result{{Outcome: shuttleengine.OutcomeDone}}}
	clock := fakeClock{now: time.Date(2026, 9, 12, 10, 30, 0, 0, time.UTC)}
	deps := newTestDeps(t, shuttle, clock)
	writeNote(t, deps, "note-1", "something went wrong")

	report, err := Reflect(deps)
	if err != nil {
		t.Fatalf("Reflect() error = %v; want nil", err)
	}
	if report.Status != StatusReflected {
		t.Errorf("Reflect() status = %q; want %q", report.Status, StatusReflected)
	}
	if report.NoteCount != 1 {
		t.Errorf("Reflect() NoteCount = %d; want 1", report.NoteCount)
	}

	if len(shuttle.specs) != 1 {
		t.Fatalf("Shuttle.Run called %d times; want 1", len(shuttle.specs))
	}
	spec := shuttle.specs[0]
	if spec.Model != "sonnet" {
		t.Errorf("Spec.Model = %q; want %q", spec.Model, "sonnet")
	}
	if spec.Effort != "high" {
		t.Errorf("Spec.Effort = %q; want %q", spec.Effort, "high")
	}
	if spec.Timeout != deps.Timeout {
		t.Errorf("Spec.Timeout = %s; want %s", spec.Timeout, deps.Timeout)
	}
	if spec.Interactive {
		t.Error("Spec.Interactive = true; want false")
	}
	if spec.ForkSubagents {
		t.Error("Spec.ForkSubagents = true; want false")
	}
	if spec.Role != "friction" {
		t.Errorf("Spec.Role = %q; want %q", spec.Role, "friction")
	}
	if len(spec.OutputFiles) == 0 {
		t.Error("Spec.OutputFiles is empty; want at least one entry")
	}
	if !strings.Contains(spec.Prompt, deps.FrictionDir) {
		t.Errorf("Spec.Prompt does not name the friction directory %q", deps.FrictionDir)
	}

	archiveDir := deps.ArchivePrefix + "20260912-103000"
	if _, err := os.Stat(archiveDir); err != nil {
		t.Errorf("archive directory %q does not exist: %v", archiveDir, err)
	}
	archivedNote := filepath.Join(archiveDir, "note-1.md")
	if _, err := os.Stat(archivedNote); err != nil {
		t.Errorf("archived note %q does not exist: %v", archivedNote, err)
	}

	wantReportPath := filepath.Join(archiveDir, friction.ReportFileName)
	if report.ReportPath != wantReportPath {
		t.Errorf("Reflect() ReportPath = %q; want %q", report.ReportPath, wantReportPath)
	}

	// The original friction directory must exist again, recreated empty.
	entries, err := os.ReadDir(deps.FrictionDir)
	if err != nil {
		t.Fatalf("ReadDir(frictionDir) after archive: %v", err)
	}
	if len(entries) != 0 {
		t.Errorf("recreated friction directory has %d entries; want 0", len(entries))
	}

	// A second Reflect against the same location reports skipped rather than re-spawning.
	report2, err := Reflect(deps)
	if err != nil {
		t.Fatalf("second Reflect() error = %v; want nil", err)
	}
	if report2.Status != StatusSkipped {
		t.Errorf("second Reflect() status = %q; want %q", report2.Status, StatusSkipped)
	}
	if len(shuttle.specs) != 1 {
		t.Errorf("Shuttle.Run called %d times after second Reflect; want still 1", len(shuttle.specs))
	}
}

// TestReflect_ShuttleOutcomes_FailedNoArchive covers the three non-done outcomes: each yields
// StatusFailed with a nil error, and the friction directory is left exactly as it was -- no archive
// sibling is created.
func TestReflect_ShuttleOutcomes_FailedNoArchive(t *testing.T) {
	tests := []struct {
		name    string
		outcome shuttleengine.Outcome
	}{
		{"Died", shuttleengine.OutcomeDied},
		{"Timeout", shuttleengine.OutcomeTimeout},
		{"Asking", shuttleengine.OutcomeAsking},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			shuttle := &fakeShuttle{results: []shuttleengine.Result{{Outcome: tt.outcome}}}
			deps := newTestDeps(t, shuttle, nil)
			writeNote(t, deps, "note-1", "something went wrong")

			report, err := Reflect(deps)
			if err != nil {
				t.Fatalf("Reflect() error = %v; want nil", err)
			}
			if report.Status != StatusFailed {
				t.Errorf("Reflect() status = %q; want %q", report.Status, StatusFailed)
			}

			if _, err := os.Stat(deps.FrictionDir); err != nil {
				t.Errorf("friction directory %q no longer exists: %v", deps.FrictionDir, err)
			}
			note := filepath.Join(deps.FrictionDir, "note-1.md")
			if _, err := os.Stat(note); err != nil {
				t.Errorf("note %q no longer exists: %v", note, err)
			}

			matches, err := filepath.Glob(deps.ArchivePrefix + "*")
			if err != nil {
				t.Fatalf("Glob: %v", err)
			}
			if len(matches) != 0 {
				t.Errorf("archive sibling(s) created: %v; want none", matches)
			}
		})
	}
}

// TestReflect_ShuttleRunError_FailedNoArchive covers a plain Shuttle.Run error: StatusFailed, a nil
// Reflect error, and no archive.
func TestReflect_ShuttleRunError_FailedNoArchive(t *testing.T) {
	shuttle := &fakeShuttle{errs: []error{errors.New("run failed")}}
	deps := newTestDeps(t, shuttle, nil)
	writeNote(t, deps, "note-1", "something went wrong")

	report, err := Reflect(deps)
	if err != nil {
		t.Fatalf("Reflect() error = %v; want nil", err)
	}
	if report.Status != StatusFailed {
		t.Errorf("Reflect() status = %q; want %q", report.Status, StatusFailed)
	}
	if _, err := os.Stat(deps.FrictionDir); err != nil {
		t.Errorf("friction directory %q no longer exists: %v", deps.FrictionDir, err)
	}
}

// TestReflect_StaleReportDeletedBeforeSpec covers the stale-report delete: a
// friction.ReportFileName present alongside a real note before Reflect runs is deleted before the
// spec is composed, so the composed Spec.OutputFiles entry does not already exist.
func TestReflect_StaleReportDeletedBeforeSpec(t *testing.T) {
	shuttle := &fakeShuttle{results: []shuttleengine.Result{{Outcome: shuttleengine.OutcomeDone}}}
	deps := newTestDeps(t, shuttle, fakeClock{now: time.Now().UTC()})
	writeNote(t, deps, "note-1", "something went wrong")
	stalePath := filepath.Join(deps.FrictionDir, friction.ReportFileName)
	if err := os.WriteFile(stalePath, []byte("stale"), 0o644); err != nil {
		t.Fatalf("WriteFile(stale report): %v", err)
	}

	report, err := Reflect(deps)
	if err != nil {
		t.Fatalf("Reflect() error = %v; want nil", err)
	}
	if report.Status != StatusReflected {
		t.Fatalf("Reflect() status = %q; want %q", report.Status, StatusReflected)
	}
	if len(shuttle.specs) != 1 {
		t.Fatalf("Shuttle.Run called %d times; want 1", len(shuttle.specs))
	}
	if len(shuttle.specs[0].OutputFiles) == 0 {
		t.Fatal("Spec.OutputFiles is empty")
	}
}

func TestReflect_DepsValidation(t *testing.T) {
	validDeps := func(t *testing.T) Deps {
		return newTestDeps(t, &fakeShuttle{}, nil)
	}

	t.Run("NilShuttle", func(t *testing.T) {
		deps := validDeps(t)
		deps.Shuttle = nil
		if _, err := Reflect(deps); err == nil {
			t.Error("Reflect() error = nil; want non-nil for a nil Shuttle")
		}
	})

	t.Run("EmptyFrictionDir", func(t *testing.T) {
		deps := validDeps(t)
		deps.FrictionDir = ""
		if _, err := Reflect(deps); err == nil {
			t.Error("Reflect() error = nil; want non-nil for an empty FrictionDir")
		}
	})

	t.Run("RelativeFrictionDir", func(t *testing.T) {
		deps := validDeps(t)
		deps.FrictionDir = "relative/friction"
		if _, err := Reflect(deps); err == nil {
			t.Error("Reflect() error = nil; want non-nil for a relative FrictionDir")
		}
	})

	t.Run("EmptyArchivePrefix", func(t *testing.T) {
		deps := validDeps(t)
		deps.ArchivePrefix = ""
		if _, err := Reflect(deps); err == nil {
			t.Error("Reflect() error = nil; want non-nil for an empty ArchivePrefix")
		}
	})

	t.Run("EmptyStencilsDir", func(t *testing.T) {
		deps := validDeps(t)
		deps.StencilsDir = ""
		if _, err := Reflect(deps); err == nil {
			t.Error("Reflect() error = nil; want non-nil for an empty StencilsDir")
		}
	})
}
