package frictionengine

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/friction"
	"github.com/Knatte18/loomyard/internal/modelspec"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
	"github.com/Knatte18/loomyard/internal/testkit/shedfake"
)

// reflectionStencilFixture is a minimal, valid reflection stencil carrying exactly the markers
// buildReflectionSpec fills.
const reflectionStencilFixture = "# Reflection\n\nDir: {{.friction_dir}}\n\nReport: {{.report_path}}\n\nTask: {{.task_slug}}\n\nNotes:\n{{.note_list}}\n"

// fakeClock is the Clock seam a test injects to assert an exact archive directory name rather than a
// pattern, and to advance time mid-run.
type fakeClock struct {
	now time.Time
}

func (f *fakeClock) Now() time.Time {
	return f.now
}

func (f *fakeClock) advance(d time.Duration) {
	f.now = f.now.Add(d)
}

// fixedClock returns a fakeClock at a fixed instant, whose archive directory suffix is 20260912-103000.
func fixedClock() *fakeClock {
	return &fakeClock{now: time.Date(2026, 9, 12, 10, 30, 0, 0, time.UTC)}
}

// doneShuttle returns a fake whose Run completes cleanly and whose Attach finds nothing.
func doneShuttle() *shedfake.Shuttle {
	return &shedfake.Shuttle{Result: shuttleengine.Result{Outcome: shuttleengine.OutcomeDone}}
}

// requireExists fails the test when path does not exist.
func requireExists(t *testing.T, path string) {
	t.Helper()
	if _, err := os.Stat(path); err != nil {
		t.Errorf("%s does not exist: %v", path, err)
	}
}

// requireAbsent fails the test when path exists.
func requireAbsent(t *testing.T, path string) {
	t.Helper()
	if _, err := os.Stat(path); err == nil {
		t.Errorf("%s exists; want it gone", path)
	}
}

// entryNames returns the sorted entry names of dir.
func entryNames(t *testing.T, dir string) []string {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("ReadDir(%s): %v", dir, err)
	}
	names := make([]string, len(entries))
	for i, e := range entries {
		names[i] = e.Name()
	}
	return names
}

// writeRecordFor writes the covered-notes record naming the given bare note stems.
func writeRecordFor(t *testing.T, deps Deps, stems ...string) {
	t.Helper()
	notes := make([]string, len(stems))
	for i, s := range stems {
		notes[i] = s + ".md"
	}
	if err := writeRecord(filepath.Join(deps.FrictionDir, coveredRecordFileName), notes); err != nil {
		t.Fatalf("writeRecord: %v", err)
	}
}

// writeReport writes a reflection report into deps.FrictionDir.
func writeReport(t *testing.T, deps Deps) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(deps.FrictionDir, friction.ReportFileName), []byte("report"), 0o644); err != nil {
		t.Fatalf("WriteFile(report): %v", err)
	}
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
		TaskSlug:      "test-task",
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
	shuttle := &shedfake.Shuttle{}
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
	if len(shuttle.Specs) != 0 {
		t.Errorf("Shuttle.Run called %d times; want 0", len(shuttle.Specs))
	}
}

func TestReflect_EmptyDirectory_Skipped(t *testing.T) {
	shuttle := &shedfake.Shuttle{}
	deps := newTestDeps(t, shuttle, nil)

	report, err := Reflect(deps)
	if err != nil {
		t.Fatalf("Reflect() error = %v; want nil", err)
	}
	if report.Status != StatusSkipped {
		t.Errorf("Reflect() status = %q; want %q", report.Status, StatusSkipped)
	}
	if len(shuttle.Specs) != 0 {
		t.Errorf("Shuttle.Run called %d times; want 0", len(shuttle.Specs))
	}
}

func TestReflect_OnlyNonMarkdownFiles_Skipped(t *testing.T) {
	shuttle := &shedfake.Shuttle{}
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
	if len(shuttle.Specs) != 0 {
		t.Errorf("Shuttle.Run called %d times; want 0", len(shuttle.Specs))
	}
}

// TestReflect_OnlyStaleReport_Skipped covers the stale-report-from-a-timed-out-run case: a directory
// whose only .md entry is friction.ReportFileName must not be mistaken for one note.
func TestReflect_OnlyStaleReport_Skipped(t *testing.T) {
	shuttle := &shedfake.Shuttle{}
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
	if len(shuttle.Specs) != 0 {
		t.Errorf("Shuttle.Run called %d times; want 0", len(shuttle.Specs))
	}
}

// TestReflect_OneNote_SpawnsAndArchives covers the full happy path: exactly one spawn, the composed
// Spec's shape, the prompt naming the friction directory, and the archive-and-recreate.
func TestReflect_OneNote_SpawnsAndArchives(t *testing.T) {
	shuttle := doneShuttle()
	deps := newTestDeps(t, shuttle, fixedClock())
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

	if len(shuttle.Specs) != 1 {
		t.Fatalf("Shuttle.Run called %d times; want 1", len(shuttle.Specs))
	}
	spec := shuttle.Specs[0]
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

	requireExists(t, filepath.Join(archiveDir, coveredRecordFileName))

	// The friction directory is never renamed away; it holds no covered entry afterwards.
	if names := entryNames(t, deps.FrictionDir); len(names) != 0 {
		t.Errorf("friction directory holds %v after archive; want no entries", names)
	}
	if len(shuttle.Specs) != 1 || shuttle.AttachCalled {
		t.Errorf("Run called %d times, Attach called = %v; want 1 and false", len(shuttle.Specs), shuttle.AttachCalled)
	}

	// A second Reflect against the same location reports skipped rather than re-spawning.
	report2, err := Reflect(deps)
	if err != nil {
		t.Fatalf("second Reflect() error = %v; want nil", err)
	}
	if report2.Status != StatusSkipped {
		t.Errorf("second Reflect() status = %q; want %q", report2.Status, StatusSkipped)
	}
	if len(shuttle.Specs) != 1 {
		t.Errorf("Shuttle.Run called %d times after second Reflect; want still 1", len(shuttle.Specs))
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
			shuttle := &shedfake.Shuttle{Result: shuttleengine.Result{Outcome: tt.outcome}}
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
			requireExists(t, filepath.Join(deps.FrictionDir, "note-1.md"))
			requireExists(t, filepath.Join(deps.FrictionDir, coveredRecordFileName))

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
	shuttle := &shedfake.Shuttle{Err: errors.New("run failed")}
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
	shuttle := doneShuttle()
	deps := newTestDeps(t, shuttle, fixedClock())
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
	if len(shuttle.Specs) != 1 {
		t.Fatalf("Shuttle.Run called %d times; want 1", len(shuttle.Specs))
	}
	if len(shuttle.Specs[0].OutputFiles) == 0 {
		t.Fatal("Spec.OutputFiles is empty")
	}
}

// archiveDirFor returns the archive directory for the fixed clock's instant, with an optional -N suffix.
func archiveDirFor(deps Deps, suffix string) string {
	return deps.ArchivePrefix + "20260912-103000" + suffix
}

// TestReflect_LiveAgentAttached_WaitsAndArchives covers a record whose agent is still live: Attach
// finds it, nothing is spawned, and the covered files are archived once it is done.
func TestReflect_LiveAgentAttached_WaitsAndArchives(t *testing.T) {
	shuttle := &shedfake.Shuttle{AttachFound: true, AttachResult: shuttleengine.Result{Outcome: shuttleengine.OutcomeDone}}
	deps := newTestDeps(t, shuttle, fixedClock())
	writeNote(t, deps, "note-1", "x")
	writeRecordFor(t, deps, "note-1")

	report, err := Reflect(deps)
	if err != nil {
		t.Fatalf("Reflect() error = %v; want nil", err)
	}
	if report.Status != StatusReflected || report.NoteCount != 1 {
		t.Errorf("Reflect() = %+v; want reflected over 1 note", report)
	}
	if !shuttle.AttachCalled || shuttle.Called {
		t.Errorf("Attach called = %v, Run called = %v; want true and false", shuttle.AttachCalled, shuttle.Called)
	}
	if !strings.Contains(shuttle.GotAttachSpec.Prompt, "note-1.md") {
		t.Errorf("Attach spec prompt does not list the covered note: %q", shuttle.GotAttachSpec.Prompt)
	}
	requireExists(t, filepath.Join(archiveDirFor(deps, ""), "note-1.md"))
	requireExists(t, filepath.Join(archiveDirFor(deps, ""), coveredRecordFileName))
	if names := entryNames(t, deps.FrictionDir); len(names) != 0 {
		t.Errorf("friction directory holds %v; want no entries", names)
	}
}

// TestReflect_AttachedRunNotDone_FailedLeavesAll covers an attached run that does not finish: the
// notes, record and report stay in place.
func TestReflect_AttachedRunNotDone_FailedLeavesAll(t *testing.T) {
	shuttle := &shedfake.Shuttle{AttachFound: true, AttachResult: shuttleengine.Result{Outcome: shuttleengine.OutcomeTimeout}}
	deps := newTestDeps(t, shuttle, fixedClock())
	writeNote(t, deps, "note-1", "x")
	writeRecordFor(t, deps, "note-1")
	writeReport(t, deps)

	report, err := Reflect(deps)
	if err != nil {
		t.Fatalf("Reflect() error = %v; want nil", err)
	}
	if report.Status != StatusFailed {
		t.Errorf("Reflect() status = %q; want %q", report.Status, StatusFailed)
	}
	want := []string{"note-1.md", coveredRecordFileName, friction.ReportFileName}
	if got := entryNames(t, deps.FrictionDir); strings.Join(got, ",") != strings.Join(want, ",") {
		t.Errorf("friction directory holds %v; want %v", got, want)
	}
	if shuttle.Called {
		t.Error("Run called; want no spawn")
	}
}

// TestReflect_AttachError_FailedNoSpawn covers a probe that cannot answer: it may be hiding a live
// agent, so nothing is spawned and nothing is archived.
func TestReflect_AttachError_FailedNoSpawn(t *testing.T) {
	shuttle := &shedfake.Shuttle{AttachErr: errors.New("probe failed")}
	deps := newTestDeps(t, shuttle, fixedClock())
	writeNote(t, deps, "note-1", "x")
	writeRecordFor(t, deps, "note-1")

	report, err := Reflect(deps)
	if err != nil {
		t.Fatalf("Reflect() error = %v; want nil", err)
	}
	if report.Status != StatusFailed {
		t.Errorf("Reflect() status = %q; want %q", report.Status, StatusFailed)
	}
	if shuttle.Called {
		t.Error("Run called; want no spawn")
	}
	requireExists(t, filepath.Join(deps.FrictionDir, "note-1.md"))
	requireExists(t, filepath.Join(deps.FrictionDir, coveredRecordFileName))
}

// TestReflect_RecordAndReport_ArchivesWithoutSpawn covers a finished prior reflection: no live agent,
// the report present, so the covered files are archived and nothing is spawned.
func TestReflect_RecordAndReport_ArchivesWithoutSpawn(t *testing.T) {
	shuttle := doneShuttle()
	deps := newTestDeps(t, shuttle, fixedClock())
	writeNote(t, deps, "note-1", "x")
	writeRecordFor(t, deps, "note-1")
	writeReport(t, deps)

	report, err := Reflect(deps)
	if err != nil {
		t.Fatalf("Reflect() error = %v; want nil", err)
	}
	if report.Status != StatusReflected || report.NoteCount != 1 {
		t.Errorf("Reflect() = %+v; want reflected over 1 note", report)
	}
	if shuttle.Called {
		t.Error("Run called; want no spawn")
	}
	if want := filepath.Join(archiveDirFor(deps, ""), friction.ReportFileName); report.ReportPath != want {
		t.Errorf("ReportPath = %q; want %q", report.ReportPath, want)
	}
	for _, name := range []string{"note-1.md", coveredRecordFileName, friction.ReportFileName} {
		requireExists(t, filepath.Join(archiveDirFor(deps, ""), name))
	}
	if names := entryNames(t, deps.FrictionDir); len(names) != 0 {
		t.Errorf("friction directory holds %v; want no entries", names)
	}
}

// TestReflect_NoteAfterRecord_LeftForNextSpawn covers a note written after the record: the archive
// leaves it behind and it is the only note the next spawn's prompt lists.
// The two archives of one call share a clock second, so they also land in distinct directories.
func TestReflect_NoteAfterRecord_LeftForNextSpawn(t *testing.T) {
	shuttle := doneShuttle()
	deps := newTestDeps(t, shuttle, fixedClock())
	writeNote(t, deps, "note-1", "x")
	writeRecordFor(t, deps, "note-1")
	writeReport(t, deps)
	writeNote(t, deps, "note-2", "y")

	report, err := Reflect(deps)
	if err != nil {
		t.Fatalf("Reflect() error = %v; want nil", err)
	}
	if report.Status != StatusReflected || report.NoteCount != 1 {
		t.Errorf("Reflect() = %+v; want reflected over the 1 note of the last archive", report)
	}
	if len(shuttle.Specs) != 1 {
		t.Fatalf("Run called %d times; want 1", len(shuttle.Specs))
	}
	prompt := shuttle.Specs[0].Prompt
	if !strings.Contains(prompt, "note-2.md") || strings.Contains(prompt, "note-1.md") {
		t.Errorf("spawn prompt lists the wrong notes: %q", prompt)
	}
	requireExists(t, filepath.Join(archiveDirFor(deps, ""), "note-1.md"))
	requireExists(t, filepath.Join(archiveDirFor(deps, "-2"), "note-2.md"))
	if want := filepath.Join(archiveDirFor(deps, "-2"), friction.ReportFileName); report.ReportPath != want {
		t.Errorf("ReportPath = %q; want %q", report.ReportPath, want)
	}
}

// TestReflect_RecordWithoutReport_DiscardedAndReflectedAgain covers a record whose agent is gone with
// no report: the record is discarded and its notes are reflected by a new spawn.
func TestReflect_RecordWithoutReport_DiscardedAndReflectedAgain(t *testing.T) {
	shuttle := doneShuttle()
	deps := newTestDeps(t, shuttle, fixedClock())
	writeNote(t, deps, "note-1", "x")
	writeRecordFor(t, deps, "note-1")

	report, err := Reflect(deps)
	if err != nil {
		t.Fatalf("Reflect() error = %v; want nil", err)
	}
	if report.Status != StatusReflected {
		t.Errorf("Reflect() status = %q; want %q", report.Status, StatusReflected)
	}
	if !shuttle.AttachCalled || len(shuttle.Specs) != 1 {
		t.Errorf("Attach called = %v, Run called %d times; want true and 1", shuttle.AttachCalled, len(shuttle.Specs))
	}
	if !strings.Contains(shuttle.Specs[0].Prompt, "note-1.md") {
		t.Errorf("spawn prompt does not list the reflected-again note: %q", shuttle.Specs[0].Prompt)
	}
	requireExists(t, filepath.Join(archiveDirFor(deps, ""), "note-1.md"))
}

// TestReflect_UnparsableRecord_DiscardedAndReflectedAgain covers a record that reads but does not
// parse: it never blocks a later reflection.
func TestReflect_UnparsableRecord_DiscardedAndReflectedAgain(t *testing.T) {
	shuttle := doneShuttle()
	deps := newTestDeps(t, shuttle, fixedClock())
	writeNote(t, deps, "note-1", "x")
	if err := os.WriteFile(filepath.Join(deps.FrictionDir, coveredRecordFileName), []byte("not json"), 0o644); err != nil {
		t.Fatalf("WriteFile(record): %v", err)
	}

	report, err := Reflect(deps)
	if err != nil {
		t.Fatalf("Reflect() error = %v; want nil", err)
	}
	if report.Status != StatusReflected {
		t.Errorf("Reflect() status = %q; want %q", report.Status, StatusReflected)
	}
	if shuttle.AttachCalled || len(shuttle.Specs) != 1 {
		t.Errorf("Attach called = %v, Run called %d times; want false and 1", shuttle.AttachCalled, len(shuttle.Specs))
	}
	requireExists(t, filepath.Join(archiveDirFor(deps, ""), "note-1.md"))
}

// TestReflect_NoteWrittenDuringSpawn_StaysAfterArchive covers a note written while the agent runs: the
// archive covers only the recorded notes, so the new one stays for the next reflection.
func TestReflect_NoteWrittenDuringSpawn_StaysAfterArchive(t *testing.T) {
	shuttle := doneShuttle()
	deps := newTestDeps(t, shuttle, fixedClock())
	writeNote(t, deps, "note-1", "x")
	shuttle.DuringRun = func() { writeNote(t, deps, "note-2", "y") }

	report, err := Reflect(deps)
	if err != nil {
		t.Fatalf("Reflect() error = %v; want nil", err)
	}
	if report.Status != StatusReflected || report.NoteCount != 1 {
		t.Errorf("Reflect() = %+v; want reflected over 1 note", report)
	}
	requireExists(t, filepath.Join(archiveDirFor(deps, ""), "note-1.md"))
	requireAbsent(t, filepath.Join(archiveDirFor(deps, ""), "note-2.md"))
	if got := entryNames(t, deps.FrictionDir); strings.Join(got, ",") != "note-2.md" {
		t.Errorf("friction directory holds %v; want only note-2.md", got)
	}
}

// TestReflect_TwoArchivesSameSecond_DistinctDirectories covers two Reflect calls whose archives share
// a clock second.
func TestReflect_TwoArchivesSameSecond_DistinctDirectories(t *testing.T) {
	deps := newTestDeps(t, doneShuttle(), fixedClock())
	writeNote(t, deps, "note-1", "x")
	if report, _ := Reflect(deps); report.Status != StatusReflected {
		t.Fatalf("first Reflect() status = %q; want %q", report.Status, StatusReflected)
	}
	writeNote(t, deps, "note-2", "y")
	if report, _ := Reflect(deps); report.Status != StatusReflected {
		t.Fatalf("second Reflect() status = %q; want %q", report.Status, StatusReflected)
	}
	requireExists(t, filepath.Join(archiveDirFor(deps, ""), "note-1.md"))
	requireExists(t, filepath.Join(archiveDirFor(deps, "-2"), "note-2.md"))
}

// TestReflect_AttachSpendsWholeBudget_NewerNoteUnspawned covers an Attach wait that spends the whole
// budget: the settled set is archived, a newer note is left unspawned and the call returns failed.
func TestReflect_AttachSpendsWholeBudget_NewerNoteUnspawned(t *testing.T) {
	clock := fixedClock()
	shuttle := &shedfake.Shuttle{AttachFound: true, AttachResult: shuttleengine.Result{Outcome: shuttleengine.OutcomeDone}}
	deps := newTestDeps(t, shuttle, clock)
	shuttle.DuringAttach = func() { clock.advance(deps.Timeout) }
	writeNote(t, deps, "note-1", "x")
	writeRecordFor(t, deps, "note-1")
	writeNote(t, deps, "note-2", "y")

	report, err := Reflect(deps)
	if err != nil {
		t.Fatalf("Reflect() error = %v; want nil", err)
	}
	if report.Status != StatusFailed {
		t.Errorf("Reflect() status = %q; want %q", report.Status, StatusFailed)
	}
	if shuttle.Called {
		t.Error("Run called; want no spawn with the budget spent")
	}
	// The archive is stamped after the clock advanced a whole timeout.
	requireExists(t, filepath.Join(deps.ArchivePrefix+"20260912-103100", "note-1.md"))
	if got := entryNames(t, deps.FrictionDir); strings.Join(got, ",") != "note-2.md" {
		t.Errorf("friction directory holds %v; want only note-2.md", got)
	}
}

// TestReflect_AttachSpendsPartOfBudget_SpawnGetsRemainder covers a partial spend: the spawn's
// Spec.Timeout is the budget left.
func TestReflect_AttachSpendsPartOfBudget_SpawnGetsRemainder(t *testing.T) {
	clock := fixedClock()
	shuttle := &shedfake.Shuttle{
		Result:       shuttleengine.Result{Outcome: shuttleengine.OutcomeDone},
		AttachFound:  true,
		AttachResult: shuttleengine.Result{Outcome: shuttleengine.OutcomeDone},
	}
	deps := newTestDeps(t, shuttle, clock)
	shuttle.DuringAttach = func() { clock.advance(20 * time.Second) }
	writeNote(t, deps, "note-1", "x")
	writeRecordFor(t, deps, "note-1")
	writeNote(t, deps, "note-2", "y")

	report, err := Reflect(deps)
	if err != nil {
		t.Fatalf("Reflect() error = %v; want nil", err)
	}
	if report.Status != StatusReflected {
		t.Errorf("Reflect() status = %q; want %q", report.Status, StatusReflected)
	}
	if len(shuttle.Specs) != 1 {
		t.Fatalf("Run called %d times; want 1", len(shuttle.Specs))
	}
	if want := deps.Timeout - 20*time.Second; shuttle.Specs[0].Timeout != want {
		t.Errorf("Spec.Timeout = %s; want %s", shuttle.Specs[0].Timeout, want)
	}
}

// TestReflect_ZeroTimeout_DefersToShuttle covers a zero Deps.Timeout, the friction_timeout_min 0 that
// defers to shuttle's run_timeout_min.
// It carries no budget: the attach probe and the spawn both receive a zero Spec.Timeout.
func TestReflect_ZeroTimeout_DefersToShuttle(t *testing.T) {
	shuttle := &shedfake.Shuttle{
		Result:       shuttleengine.Result{Outcome: shuttleengine.OutcomeDone},
		AttachFound:  true,
		AttachResult: shuttleengine.Result{Outcome: shuttleengine.OutcomeDone},
	}
	deps := newTestDeps(t, shuttle, nil)
	deps.Timeout = 0
	writeNote(t, deps, "note-1", "x")
	writeRecordFor(t, deps, "note-1")
	writeNote(t, deps, "note-2", "y")

	report, err := Reflect(deps)
	if err != nil {
		t.Fatalf("Reflect() error = %v; want nil", err)
	}
	if report.Status != StatusReflected {
		t.Errorf("Reflect() status = %q; want %q", report.Status, StatusReflected)
	}
	if !shuttle.AttachCalled || shuttle.GotAttachSpec.Timeout != 0 {
		t.Errorf("Attach called = %v with Timeout %s; want called with a zero Timeout", shuttle.AttachCalled, shuttle.GotAttachSpec.Timeout)
	}
	if len(shuttle.Specs) != 1 || shuttle.Specs[0].Timeout != 0 {
		t.Errorf("Run specs = %v; want one with a zero Timeout", shuttle.Specs)
	}
}

// TestReflect_UnwritableRecord_FailedNoSpawn covers a record write that fails before the spawn.
func TestReflect_UnwritableRecord_FailedNoSpawn(t *testing.T) {
	if runtime.GOOS == "windows" || os.Geteuid() == 0 {
		t.Skip("directory permissions do not bar writes here")
	}
	shuttle := doneShuttle()
	deps := newTestDeps(t, shuttle, fixedClock())
	writeNote(t, deps, "note-1", "x")
	if err := os.Chmod(deps.FrictionDir, 0o555); err != nil {
		t.Fatalf("Chmod: %v", err)
	}
	t.Cleanup(func() { _ = os.Chmod(deps.FrictionDir, 0o755) })

	report, err := Reflect(deps)
	if err != nil {
		t.Fatalf("Reflect() error = %v; want nil", err)
	}
	if report.Status != StatusFailed {
		t.Errorf("Reflect() status = %q; want %q", report.Status, StatusFailed)
	}
	if shuttle.Called {
		t.Error("Run called; want no spawn")
	}
	requireExists(t, filepath.Join(deps.FrictionDir, "note-1.md"))
}

func TestReflect_DepsValidation(t *testing.T) {
	validDeps := func(t *testing.T) Deps {
		return newTestDeps(t, &shedfake.Shuttle{}, nil)
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

	t.Run("EmptyTaskSlug", func(t *testing.T) {
		deps := validDeps(t)
		deps.TaskSlug = ""
		if _, err := Reflect(deps); err == nil {
			t.Error("Reflect() error = nil; want non-nil for an empty TaskSlug")
		}
	})
}
