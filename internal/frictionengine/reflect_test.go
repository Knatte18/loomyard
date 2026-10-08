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
	"github.com/Knatte18/loomyard/internal/segmentcolor"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
	"github.com/Knatte18/loomyard/internal/testkit/shedfake"
	"github.com/Knatte18/loomyard/internal/testkit/stencilkit"
)

// reflectionStencilFixture is a minimal, valid reflection stencil carrying exactly the markers
// buildReflectionSpec fills.
const reflectionStencilFixture = "# Reflection\n\n{{.parent_directive}}\n{{.edit_directive}}\n\nDir: {{.friction_dir}}\n\nReport: {{.report_path}}\n\nTask: {{.task_slug}}\n\nNotes:\n{{.note_list}}\n"

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
	stencilkit.SeedInto(t, stencilsDir)
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

// TestBuildReflectionSpec_SkillsAndParentDirective covers the composed Spec's reflection skill and its parent directive, with and without a recorded parent.
func TestBuildReflectionSpec_SkillsAndParentDirective(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name       string
		parentName string
		want       string
	}{
		{"with parent", "ab:cd:webster", "ab:cd:webster"},
		{"no parent", "", "No parent is recorded"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			deps := newTestDeps(t, doneShuttle(), fixedClock())
			deps.ParentName = tt.parentName

			spec, err := buildReflectionSpec(deps, []string{"note-1.md"}, "/x/report.md")
			if err != nil {
				t.Fatalf("buildReflectionSpec error = %v; want nil", err)
			}
			if !strings.Contains(spec.Prompt, tt.want) {
				t.Errorf("Prompt = %q; want it to contain %q", spec.Prompt, tt.want)
			}
			if !strings.Contains(spec.Prompt, "Edit or Write") {
				t.Errorf("Prompt = %q; want it to contain the edit directive's \"Edit or Write\" sentence", spec.Prompt)
			}
			if tt.parentName == "" && strings.Contains(spec.Prompt, "Your parent is") {
				t.Errorf("Prompt = %q; want the no-parent variant", spec.Prompt)
			}
			if got := strings.Join(spec.Skills, ","); got != "scribe:prose" {
				t.Errorf("Skills = %q; want scribe:prose", got)
			}
		})
	}
}

// TestReflect_OneNote_SpawnsAndArchives covers the full happy path: exactly one spawn, the composed
// Spec's shape, the prompt naming the friction directory, and the archive-and-recreate.
func TestReflect_OneNote_SpawnsAndArchives(t *testing.T) {
	t.Parallel()
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
	if spec.Segment != segmentcolor.Landing {
		t.Errorf("Spec.Segment = %q; want %q", spec.Segment, segmentcolor.Landing)
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

// TestReflect_StaleReportDeletedBeforeSpec covers the stale-report delete: a
// friction.ReportFileName present alongside a real note before Reflect runs is deleted before the
// spec is composed, so the composed Spec.OutputFiles entry does not already exist.
func TestReflect_StaleReportDeletedBeforeSpec(t *testing.T) {
	t.Parallel()
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

// TestReflect_LiveAgentAttached_WaitsAndArchives covers a record whose agent is still live:
// Attach finds it, nothing is spawned, and the covered files are archived once it is done.
func TestReflect_LiveAgentAttached_WaitsAndArchives(t *testing.T) {
	t.Parallel()
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

// TestReflect_RecordAndReport_ArchivesWithoutSpawn covers a finished prior reflection:
// no live agent, the report present, so the covered files are archived and nothing is spawned.
func TestReflect_RecordAndReport_ArchivesWithoutSpawn(t *testing.T) {
	t.Parallel()
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

// TestReflect_NoteAfterRecord_LeftForNextSpawn covers a note written after the record:
// the archive leaves it behind and it is the only note the next spawn's prompt lists.
// The two archives of one call share a clock second, so they also land in distinct directories.
func TestReflect_NoteAfterRecord_LeftForNextSpawn(t *testing.T) {
	t.Parallel()
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

// TestReflect_NoteWrittenDuringSpawn_StaysAfterArchive covers a note written while the agent runs:
// the archive covers only the recorded notes, so the new one stays for the next reflection.
func TestReflect_NoteWrittenDuringSpawn_StaysAfterArchive(t *testing.T) {
	t.Parallel()
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

// TestReflect_UnwritableRecord_FailedNoSpawn covers a record write that fails before the spawn.
func TestReflect_UnwritableRecord_FailedNoSpawn(t *testing.T) {
	t.Parallel()
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

// TestReflect_Skipped covers every friction directory with nothing to reflect: Reflect reports skipped and spawns nothing.
// That is a missing directory, an empty one, one holding only a non-markdown file, and one whose only .md entry is a stale friction.ReportFileName left by a timed-out run, which must not be mistaken for one note.
func TestReflect_Skipped(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name  string
		setup func(t *testing.T, deps Deps)
	}{
		{"missing directory", func(t *testing.T, deps Deps) {
			if err := os.RemoveAll(deps.FrictionDir); err != nil {
				t.Fatalf("RemoveAll(frictionDir): %v", err)
			}
		}},
		{"empty directory", func(t *testing.T, deps Deps) {}},
		{"only non-markdown files", func(t *testing.T, deps Deps) {
			if err := os.WriteFile(filepath.Join(deps.FrictionDir, "notes.txt"), []byte("hello"), 0o644); err != nil {
				t.Fatalf("WriteFile: %v", err)
			}
		}},
		{"only stale report", func(t *testing.T, deps Deps) {
			if err := os.WriteFile(filepath.Join(deps.FrictionDir, friction.ReportFileName), []byte("stale"), 0o644); err != nil {
				t.Fatalf("WriteFile: %v", err)
			}
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			shuttle := &shedfake.Shuttle{}
			deps := newTestDeps(t, shuttle, nil)
			tt.setup(t, deps)

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
		})
	}
}

// TestReflect_ShuttleFailures_FailedNoArchive covers every way a spawned run fails to finish: the three non-done outcomes and a plain Shuttle.Run error.
// Each yields StatusFailed with a nil Reflect error, and the friction directory is left exactly as it was, with the note and the covered record in place and no archive sibling created.
func TestReflect_ShuttleFailures_FailedNoArchive(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		shuttle *shedfake.Shuttle
	}{
		{"Died", &shedfake.Shuttle{Result: shuttleengine.Result{Outcome: shuttleengine.OutcomeDied}}},
		{"Timeout", &shedfake.Shuttle{Result: shuttleengine.Result{Outcome: shuttleengine.OutcomeTimeout}}},
		{"RunError", &shedfake.Shuttle{Err: errors.New("run failed")}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			deps := newTestDeps(t, tt.shuttle, nil)
			writeNote(t, deps, "note-1", "something went wrong")

			report, err := Reflect(deps)
			if err != nil {
				t.Fatalf("Reflect() error = %v; want nil", err)
			}
			if report.Status != StatusFailed {
				t.Errorf("Reflect() status = %q; want %q", report.Status, StatusFailed)
			}

			requireExists(t, deps.FrictionDir)
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

// TestReflect_AttachFailures_FailedNoSpawn covers a record whose attach probe leaves the reflection unfinished: an attached run that does not finish, and a probe that cannot answer and so may be hiding a live agent.
// Each yields StatusFailed, nothing is spawned and the friction directory is left exactly as it was, so the notes, record and report stay in place.
func TestReflect_AttachFailures_FailedNoSpawn(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name       string
		shuttle    *shedfake.Shuttle
		withReport bool
	}{
		{"attached run not done", &shedfake.Shuttle{AttachFound: true, AttachResult: shuttleengine.Result{Outcome: shuttleengine.OutcomeTimeout}}, true},
		{"probe error", &shedfake.Shuttle{AttachErr: errors.New("probe failed")}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			deps := newTestDeps(t, tt.shuttle, fixedClock())
			writeNote(t, deps, "note-1", "x")
			writeRecordFor(t, deps, "note-1")
			if tt.withReport {
				writeReport(t, deps)
			}
			before := entryNames(t, deps.FrictionDir)

			report, err := Reflect(deps)
			if err != nil {
				t.Fatalf("Reflect() error = %v; want nil", err)
			}
			if report.Status != StatusFailed {
				t.Errorf("Reflect() status = %q; want %q", report.Status, StatusFailed)
			}
			if tt.shuttle.Called {
				t.Error("Run called; want no spawn")
			}
			if got := entryNames(t, deps.FrictionDir); strings.Join(got, ",") != strings.Join(before, ",") {
				t.Errorf("friction directory holds %v; want it unchanged at %v", got, before)
			}
		})
	}
}

// TestReflect_UnusableRecord_DiscardedAndReflectedAgain covers a record that cannot block a later reflection, so its note is reflected by a new spawn:
// one whose agent is gone with no report, and one that reads but does not parse.
// The unparsable record is discarded without an attach probe.
func TestReflect_UnusableRecord_DiscardedAndReflectedAgain(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name             string
		writeRecord      func(t *testing.T, deps Deps)
		wantAttachCalled bool
	}{
		{"agent gone without report", func(t *testing.T, deps Deps) { writeRecordFor(t, deps, "note-1") }, true},
		{"unparsable record", func(t *testing.T, deps Deps) {
			if err := os.WriteFile(filepath.Join(deps.FrictionDir, coveredRecordFileName), []byte("not json"), 0o644); err != nil {
				t.Fatalf("WriteFile(record): %v", err)
			}
		}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			shuttle := doneShuttle()
			deps := newTestDeps(t, shuttle, fixedClock())
			writeNote(t, deps, "note-1", "x")
			tt.writeRecord(t, deps)

			report, err := Reflect(deps)
			if err != nil {
				t.Fatalf("Reflect() error = %v; want nil", err)
			}
			if report.Status != StatusReflected {
				t.Errorf("Reflect() status = %q; want %q", report.Status, StatusReflected)
			}
			if shuttle.AttachCalled != tt.wantAttachCalled || len(shuttle.Specs) != 1 {
				t.Errorf("Attach called = %v, Run called %d times; want %v and 1", shuttle.AttachCalled, len(shuttle.Specs), tt.wantAttachCalled)
			}
			if !strings.Contains(shuttle.Specs[0].Prompt, "note-1.md") {
				t.Errorf("spawn prompt does not list the reflected-again note: %q", shuttle.Specs[0].Prompt)
			}
			requireExists(t, filepath.Join(archiveDirFor(deps, ""), "note-1.md"))
		})
	}
}

// TestReflect_AttachBudget covers how a reflection's timeout budget is spent after the attach wait, with note-1 recorded and note-2 newer.
// A wait that spends the whole budget archives the settled set, leaves the newer note unspawned and fails.
// A partial spend gives the spawn the budget left as its Spec.Timeout.
// A zero Deps.Timeout, the friction_timeout_min 0 that defers to shuttle's run_timeout_min, carries no budget: the attach probe and the spawn both receive a zero Spec.Timeout.
func TestReflect_AttachBudget(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		timeout time.Duration
		advance time.Duration
		// wantRunTimeout is the Spec.Timeout of the one spawn; ignored when wantStatus is failed, which spawns nothing.
		wantRunTimeout time.Duration
		wantStatus     string
	}{
		{"whole budget spent", time.Minute, time.Minute, 0, StatusFailed},
		{"part of budget spent", time.Minute, 20 * time.Second, 40 * time.Second, StatusReflected},
		{"zero timeout defers to shuttle", 0, 0, 0, StatusReflected},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			clock := fixedClock()
			shuttle := &shedfake.Shuttle{
				Result:       shuttleengine.Result{Outcome: shuttleengine.OutcomeDone},
				AttachFound:  true,
				AttachResult: shuttleengine.Result{Outcome: shuttleengine.OutcomeDone},
			}
			deps := newTestDeps(t, shuttle, clock)
			deps.Timeout = tt.timeout
			shuttle.DuringAttach = func() { clock.advance(tt.advance) }
			writeNote(t, deps, "note-1", "x")
			writeRecordFor(t, deps, "note-1")
			writeNote(t, deps, "note-2", "y")

			report, err := Reflect(deps)
			if err != nil {
				t.Fatalf("Reflect() error = %v; want nil", err)
			}
			if report.Status != tt.wantStatus {
				t.Errorf("Reflect() status = %q; want %q", report.Status, tt.wantStatus)
			}

			if tt.wantStatus == StatusFailed {
				if shuttle.Called {
					t.Error("Run called; want no spawn with the budget spent")
				}
				// The archive is stamped after the clock advanced a whole timeout.
				requireExists(t, filepath.Join(deps.ArchivePrefix+"20260912-103100", "note-1.md"))
				if got := entryNames(t, deps.FrictionDir); strings.Join(got, ",") != "note-2.md" {
					t.Errorf("friction directory holds %v; want only note-2.md", got)
				}
				return
			}
			if len(shuttle.Specs) != 1 {
				t.Fatalf("Run called %d times; want 1", len(shuttle.Specs))
			}
			if shuttle.Specs[0].Timeout != tt.wantRunTimeout {
				t.Errorf("Spec.Timeout = %s; want %s", shuttle.Specs[0].Timeout, tt.wantRunTimeout)
			}
			if tt.timeout == 0 && (!shuttle.AttachCalled || shuttle.GotAttachSpec.Timeout != 0) {
				t.Errorf("Attach called = %v with Timeout %s; want called with a zero Timeout", shuttle.AttachCalled, shuttle.GotAttachSpec.Timeout)
			}
		})
	}
}

// TestReflect_DepsValidation covers each Deps field Reflect refuses: a nil Shuttle, an empty or relative FrictionDir, and an empty ArchivePrefix, StencilsDir or TaskSlug.
func TestReflect_DepsValidation(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name   string
		mutate func(deps *Deps)
	}{
		{"NilShuttle", func(deps *Deps) { deps.Shuttle = nil }},
		{"EmptyFrictionDir", func(deps *Deps) { deps.FrictionDir = "" }},
		{"RelativeFrictionDir", func(deps *Deps) { deps.FrictionDir = "relative/friction" }},
		{"EmptyArchivePrefix", func(deps *Deps) { deps.ArchivePrefix = "" }},
		{"EmptyStencilsDir", func(deps *Deps) { deps.StencilsDir = "" }},
		{"EmptyTaskSlug", func(deps *Deps) { deps.TaskSlug = "" }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			deps := newTestDeps(t, &shedfake.Shuttle{}, nil)
			tt.mutate(&deps)
			if _, err := Reflect(deps); err == nil {
				t.Errorf("Reflect() error = nil; want non-nil for %s", tt.name)
			}
		})
	}
}
