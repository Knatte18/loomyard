//go:build tier0

package shedrun

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/lymcwd"
	"github.com/Knatte18/loomyard/internal/lymdirs"
)

// worktreeName is the worktree name of every syntheticLocation, the run-id "self" resolves to.
const worktreeName = "worktree"

// syntheticLocation returns a *lymcwd.Location anchored at a t.TempDir()-rooted worktree, with no
// git or filesystem I/O of its own -- every constructor under test is a plain filepath.Join, so this
// only needs to carry an AnchorPath() that resolves predictably.
func syntheticLocation(t *testing.T) *lymcwd.Location {
	t.Helper()
	hub := t.TempDir()
	return &lymcwd.Location{
		RepoName:     "repo",
		HubPath:      hub,
		WorktreeName: worktreeName,
		AnchorRel:    ".",
	}
}

// TestPathConstructors pins every path constructor's layout under the anchor: durable files under _lym/shed/<run>, ephemeral ones under .lym/shed/<run>.
func TestPathConstructors(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		got  func(l *lymcwd.Location) string
		want func(l *lymcwd.Location) string
	}{
		{"RunDir", func(l *lymcwd.Location) string { return RunDir(l, "self") },
			func(l *lymcwd.Location) string { return filepath.Join(l.AnchorPath(), "_lym", "shed", worktreeName) }},
		{"SeedFile", func(l *lymcwd.Location) string { return SeedFile(l, "self") },
			func(l *lymcwd.Location) string { return filepath.Join(RunDir(l, "self"), "seed.json") }},
		{"StatusFile", func(l *lymcwd.Location) string { return StatusFile(l, "self") },
			func(l *lymcwd.Location) string { return filepath.Join(RunDir(l, "self"), "status.json") }},
		{"StepsDir", func(l *lymcwd.Location) string { return StepsDir(l, "self") },
			func(l *lymcwd.Location) string { return filepath.Join(ScratchDir(l, "self"), "steps") }},
		{"LoopLock", func(l *lymcwd.Location) string { return LoopLock(l, "self") },
			func(l *lymcwd.Location) string { return filepath.Join(StepsDir(l, "self"), "loop.lock") }},
		{"LoopPIDFile", func(l *lymcwd.Location) string { return LoopPIDFile(l, "self") },
			func(l *lymcwd.Location) string { return filepath.Join(StepsDir(l, "self"), "loop.pid") }},
		{"LoopLog", func(l *lymcwd.Location) string { return LoopLog(l, "self") },
			func(l *lymcwd.Location) string { return filepath.Join(StepsDir(l, "self"), "loop.log") }},
		{"LoopEnvelope", func(l *lymcwd.Location) string { return LoopEnvelope(l, "self") },
			func(l *lymcwd.Location) string { return filepath.Join(StepsDir(l, "self"), "loop-envelope.json") }},
		{"LoopDelivered", func(l *lymcwd.Location) string { return LoopDelivered(l, "self", "abc") },
			func(l *lymcwd.Location) string {
				return filepath.Join(StepsDir(l, "self"), "loop-envelope.abc.delivered.json")
			}},
		{"StepTraceCopy", func(l *lymcwd.Location) string { return StepTraceCopy(l, "self", "t1") },
			func(l *lymcwd.Location) string { return filepath.Join(StepsDir(l, "self"), "t1.trace.log") }},
		{"StepStderr", func(l *lymcwd.Location) string { return StepStderr(l, "self", "t1") },
			func(l *lymcwd.Location) string { return filepath.Join(StepsDir(l, "self"), "t1.stderr.log") }},
		{"ScratchDir", func(l *lymcwd.Location) string { return ScratchDir(l, "self") },
			func(l *lymcwd.Location) string { return filepath.Join(l.AnchorPath(), ".lym", "shed", worktreeName) }},
		{"GateFindingsFile", func(l *lymcwd.Location) string { return GateFindingsFile(l, "self", "Darn-Write") },
			func(l *lymcwd.Location) string {
				return filepath.Join(ScratchDir(l, "self"), "gate-findings", "Darn-Write.md")
			}},
		{"RunLock", func(l *lymcwd.Location) string { return RunLock(l, "self") },
			func(l *lymcwd.Location) string { return filepath.Join(ScratchDir(l, "self"), "run.lock") }},
		{"StatusLock", func(l *lymcwd.Location) string { return StatusLock(l, "self") },
			func(l *lymcwd.Location) string { return filepath.Join(ScratchDir(l, "self"), "status.json.lock") }},
		{"PullRequestRecord", func(l *lymcwd.Location) string { return PullRequestRecord(l, "self") },
			func(l *lymcwd.Location) string { return filepath.Join(ScratchDir(l, "self"), "pull-request.json") }},
		{"PullRequestRecordLock", func(l *lymcwd.Location) string { return PullRequestRecordLock(l, "self") },
			func(l *lymcwd.Location) string { return filepath.Join(ScratchDir(l, "self"), "pull-request.json.lock") }},
		{"LastCommitMarker", func(l *lymcwd.Location) string { return LastCommitMarker(l, "self") },
			func(l *lymcwd.Location) string { return filepath.Join(ScratchDir(l, "self"), "last-commit") }},
		{"BattenWatchedMarker", func(l *lymcwd.Location) string { return BattenWatchedMarker(l, "self") },
			func(l *lymcwd.Location) string { return filepath.Join(ScratchDir(l, "self"), BattenWatchedFileName) }},
		{"DriveReportsDir", func(l *lymcwd.Location) string { return DriveReportsDir(l, "self") },
			func(l *lymcwd.Location) string { return filepath.Join(RunDir(l, "self"), "drive-reports") }},
		{"ArbiterDir", func(l *lymcwd.Location) string { return ArbiterDir(l, "self") },
			func(l *lymcwd.Location) string { return filepath.Join(RunDir(l, "self"), "arbiter") }},
		{"ArbiterAttemptDir", func(l *lymcwd.Location) string { return ArbiterAttemptDir(l, "self", 2) },
			func(l *lymcwd.Location) string { return filepath.Join(ArbiterDir(l, "self"), "attempt-2") }},
		{"ArbiterMarker", func(l *lymcwd.Location) string { return ArbiterMarker(l, "self") },
			func(l *lymcwd.Location) string { return filepath.Join(ScratchDir(l, "self"), "arbiter-inflight.json") }},
		{"DriverLock", func(l *lymcwd.Location) string { return DriverLock(l, "self") },
			func(l *lymcwd.Location) string { return filepath.Join(ScratchDir(l, "self"), "driver.lock") }},
		{"TacklerDir", func(l *lymcwd.Location) string { return TacklerDir(l, "self") },
			func(l *lymcwd.Location) string { return filepath.Join(RunDir(l, "self"), "tackler") }},
		{"TacklerRel", func(l *lymcwd.Location) string { return TacklerRel(l, "self") },
			func(l *lymcwd.Location) string { return filepath.Join("_lym", "shed", worktreeName, "tackler") }},
		{"TacklerAttemptDir", func(l *lymcwd.Location) string { return TacklerAttemptDir(l, "self", 2) },
			func(l *lymcwd.Location) string { return filepath.Join(TacklerDir(l, "self"), "attempt-2") }},
		{"TacklerMarker", func(l *lymcwd.Location) string { return TacklerMarker(l, "self") },
			func(l *lymcwd.Location) string { return filepath.Join(ScratchDir(l, "self"), "tackler-inflight.json") }},
		{"CancelMarker", func(l *lymcwd.Location) string { return CancelMarker(l, "self") },
			func(l *lymcwd.Location) string {
				return filepath.Join(ScratchRoot(l), worktreeName+".cancel.json")
			}},
		{"CancelMarkerSlug", func(l *lymcwd.Location) string { return CancelMarker(l, worktreeName) },
			func(l *lymcwd.Location) string {
				return filepath.Join(l.AnchorPath(), ".lym", "shed", worktreeName+".cancel.json")
			}},
		{"CancelMarkerLock", func(l *lymcwd.Location) string { return CancelMarkerLock(l, "self") },
			func(l *lymcwd.Location) string {
				return filepath.Join(ScratchRoot(l), worktreeName+".cancel.lock")
			}},
		{"CancelMarkerLockSlug", func(l *lymcwd.Location) string { return CancelMarkerLock(l, worktreeName) },
			func(l *lymcwd.Location) string {
				return filepath.Join(l.AnchorPath(), ".lym", "shed", worktreeName+".cancel.lock")
			}},
		{"SettleRecords", func(l *lymcwd.Location) string { return SettleRecords(l, "self") },
			func(l *lymcwd.Location) string { return filepath.Join(RunDir(l, "self"), "go-settle.jsonl") }},
		{"PrimeRunLock", PrimeRunLock,
			func(l *lymcwd.Location) string { return filepath.Join(l.AnchorPath(), ".lym", "shed", "run.lock") }},
		{"LandLock", LandLock,
			func(l *lymcwd.Location) string { return filepath.Join(l.AnchorPath(), ".lym", "shed", "land.lock") }},
		{"SeedRel", func(l *lymcwd.Location) string { return SeedRel(l, "self") },
			func(l *lymcwd.Location) string { return filepath.Join("_lym", "shed", worktreeName, "seed.json") }},
		{"StatusRel", func(l *lymcwd.Location) string { return StatusRel(l, "self") },
			func(l *lymcwd.Location) string { return filepath.Join("_lym", "shed", worktreeName, "status.json") }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			l := syntheticLocation(t)
			if got, want := tt.got(l), tt.want(l); got != want {
				t.Errorf("%s = %q; want %q", tt.name, got, want)
			}
			if strings.HasPrefix(tt.name, "CancelMarker") {
				for _, dir := range []string{RunDir(l, "self"), ScratchDir(l, "self")} {
					if strings.HasPrefix(tt.got(l), dir+string(filepath.Separator)) {
						t.Errorf("%s = %q lies under %q", tt.name, tt.got(l), dir)
					}
				}
			}
		})
	}
}

// TestBattenWatchedContent pins the batten-watched marker's format: a pid alone is one line, a pid with a scratch directory and a row round-trips, and a first line that is no positive pid fails to parse.
func TestBattenWatchedContent(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		pid         int
		scratchDir  string
		producer    string
		wantContent string
	}{
		{name: "pid alone", pid: 4242, wantContent: "4242\n"},
		{name: "pid with scratch directory and row", pid: 4242, scratchDir: "/hub/.lym/batten", producer: "Run-Shed", wantContent: "4242\n/hub/.lym/batten\nRun-Shed\n"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			content := BattenWatchedContent(tt.pid, tt.scratchDir, tt.producer)
			if string(content) != tt.wantContent {
				t.Fatalf("BattenWatchedContent() = %q; want %q", content, tt.wantContent)
			}
			pid, scratchDir, producer, err := ParseBattenWatched(content)
			if err != nil || pid != tt.pid || scratchDir != tt.scratchDir || producer != tt.producer {
				t.Errorf("ParseBattenWatched() = %d, %q, %q, %v; want %d, %q, %q, nil", pid, scratchDir, producer, err, tt.pid, tt.scratchDir, tt.producer)
			}
		})
	}
	for _, bad := range []string{"", "not a pid\n", "0\n", "-3\n/dir\nrow\n"} {
		if _, _, _, err := ParseBattenWatched([]byte(bad)); err == nil {
			t.Errorf("ParseBattenWatched(%q) error = nil; want a refusal", bad)
		}
	}
}

func TestParkMarker(t *testing.T) {
	t.Parallel()
	l := syntheticLocation(t)
	for _, runID := range []string{"worktree", "self"} {
		t.Run(runID, func(t *testing.T) {
			t.Parallel()
			got := ParkMarker(l, runID)
			want := filepath.Join(ScratchDir(l, runID), ParkMarkerFileName)
			if got != want {
				t.Errorf("ParkMarker(l, %q) = %q; want %q", runID, got, want)
			}
			rel, err := filepath.Rel(filepath.Join(l.AnchorPath(), ".lym"), got)
			if err != nil || strings.HasPrefix(rel, "..") {
				t.Errorf("ParkMarker(l, %q) = %q; want it under the .lym tree", runID, got)
			}
			report := ResumeReport(l, runID)
			if report == got || filepath.Dir(report) != filepath.Dir(got) || filepath.Dir(report) != ScratchDir(l, runID) {
				t.Errorf("ResumeReport(l, %q) = %q; want a file beside %q under the scratch directory", runID, report, got)
			}
		})
	}
}

func TestRunsRootRel(t *testing.T) {
	t.Parallel()
	l := syntheticLocation(t)
	root := RunsRootRel()
	if want := filepath.Join(lymdirs.LymDirName, "shed"); root != want {
		t.Errorf("RunsRootRel() = %q; want %q", root, want)
	}
	if got, want := RunsRoot(l), filepath.Dir(RunDir(l, "x")); got != want {
		t.Errorf("RunsRoot(l) = %q; want %q", got, want)
	}
	if got, want := ScratchRoot(l), filepath.Dir(ScratchDir(l, "x")); got != want {
		t.Errorf("ScratchRoot(l) = %q; want %q", got, want)
	}
	prefix := root + string(filepath.Separator)
	for name, got := range map[string]string{
		"SeedRel":   SeedRel(l, "x"),
		"StatusRel": StatusRel(l, "x"),
	} {
		if !strings.HasPrefix(got, prefix) {
			t.Errorf("%s(l, %q) = %q; want it strictly under %q", name, "x", got, root)
		}
	}

	for runID, want := range map[string][2]string{
		"x":    {"_lym/shed/x/seed.json", "_lym/shed/x/status.json"},
		"self": {"_lym/shed/self/seed.json", "_lym/shed/self/status.json"},
	} {
		seedRel, statusRel, err := TreeRunFileRels(runID)
		if err != nil || seedRel != want[0] || statusRel != want[1] {
			t.Errorf("TreeRunFileRels(%q) = %q, %q, %v; want %q, %q, nil", runID, seedRel, statusRel, err, want[0], want[1])
		}
	}
	if seedRel, statusRel, err := TreeRunFileRels("a/b"); err == nil || seedRel != "" || statusRel != "" {
		t.Errorf("TreeRunFileRels(%q) = %q, %q, %v; want empty paths and an error", "a/b", seedRel, statusRel, err)
	}

	rels, err := TreeRunRels("my-slug")
	wantRels := RunTreeRels{
		Dir:           filepath.ToSlash(RunDirRel(l, "my-slug")),
		Seed:          filepath.ToSlash(SeedRel(l, "my-slug")),
		Status:        filepath.ToSlash(StatusRel(l, "my-slug")),
		DriveReports:  filepath.ToSlash(DriveReportsRel(l, "my-slug")),
		Arbiter:       filepath.ToSlash(ArbiterRel(l, "my-slug")),
		Alignment:     filepath.ToSlash(filepath.Join(RunDirRel(l, "my-slug"), alignmentDirName)),
		SettleRecords: filepath.ToSlash(filepath.Join(RunDirRel(l, "my-slug"), settleRecordsFileName)),
	}
	if err != nil || rels != wantRels {
		t.Errorf("TreeRunRels(%q) = %+v, %v; want %+v, nil", "my-slug", rels, err, wantRels)
	}
	if rels, err := TreeRunRels("a/b"); err == nil || rels != (RunTreeRels{}) {
		t.Errorf("TreeRunRels(%q) = %+v, %v; want a zero value and an error", "a/b", rels, err)
	}
}

func TestDriveReportsRel(t *testing.T) {
	t.Parallel()
	anchored := syntheticLocation(t)
	anchored.AnchorRel = "sub"
	for name, l := range map[string]*lymcwd.Location{
		"unanchored": syntheticLocation(t),
		"anchored":   anchored,
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			for relName, pair := range map[string][2]string{
				"DriveReportsRel": {DriveReportsRel(l, "self"), DriveReportsDir(l, "self")},
				"ArbiterRel":      {ArbiterRel(l, "self"), ArbiterDir(l, "self")},
			} {
				want, err := filepath.Rel(l.AnchorPath(), pair[1])
				if err != nil {
					t.Fatalf("filepath.Rel: %v", err)
				}
				if pair[0] != want {
					t.Errorf("%s = %q; want %q", relName, pair[0], want)
				}
			}
		})
	}
}

// TestLocksAreDistinct pins that a run's RunLock and StatusLock never share a file, which shedengine.Shed's own validation rejects outright (a shared file would hang on the first persist rather than fail), and that PrimeRunLock and LandLock are distinct hub-scoped locks from each other and from any single run-id's locks.
//
//testtiming:keep pins that RunLock, StatusLock, PrimeRunLock and LandLock never share a file, which its covering tests do not
func TestLocksAreDistinct(t *testing.T) {
	t.Parallel()
	l := syntheticLocation(t)
	prime := PrimeRunLock(l)
	land := LandLock(l)
	if prime == land {
		t.Errorf("PrimeRunLock(l) == LandLock(l) = %q; must differ", prime)
	}
	for _, runID := range []string{"self", "abc123", "run-two", "another-run-id"} {
		t.Run(runID, func(t *testing.T) {
			t.Parallel()
			if got := RunLock(l, runID); got == StatusLock(l, runID) {
				t.Errorf("RunLock(l, %q) == StatusLock(l, %q) = %q; must differ", runID, runID, got)
			}
			if prime == RunLock(l, runID) {
				t.Errorf("PrimeRunLock(l) == RunLock(l, %q) = %q; must differ", runID, prime)
			}
			if prime == StatusLock(l, runID) {
				t.Errorf("PrimeRunLock(l) == StatusLock(l, %q) = %q; must differ", runID, prime)
			}
			if land == RunLock(l, runID) || land == StatusLock(l, runID) {
				t.Errorf("LandLock(l) = %q; must differ from RunLock and StatusLock of %q", land, runID)
			}
			if got := LoopLock(l, runID); got == RunLock(l, runID) || got == StatusLock(l, runID) {
				t.Errorf("LoopLock(l, %q) = %q; must differ from RunLock and StatusLock", runID, got)
			}
			if LoopDelivered(l, runID, "loop-a") == LoopDelivered(l, runID, "loop-b") {
				t.Errorf("LoopDelivered(l, %q, ...) is the same file for two loop ids", runID)
			}
			if name := LoopJobName(l, runID); strings.ContainsAny(name, `/\`) || name == LoopJobName(syntheticLocation(t), runID) {
				t.Errorf("LoopJobName(l, %q) = %q; want no path separator and a name that differs between two locations", runID, name)
			}
		})
	}
}
