// gitentry_test.go holds the tests of the shared-temp-directory `.git` check.
// They cover its report, its watch and its process classification.

package tmuxkit

import (
	"bytes"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

func TestSharedTempDirs_AreTheRootAndTheSystemTempDirectoryAboveIt(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()
	system := t.TempDir()
	root := filepath.Join(system, "lym-1000")
	got := sharedTempDirs(filepath.Join(root, "rabcd"))
	if len(got) != 2 || got[0] != root || got[1] != system {
		t.Errorf("sharedTempDirs(%q) = %q; want [%q %q]", filepath.Join(root, "rabcd"), got, root, system)
	}
}

func TestCheckNewGitEntries(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()
	at := time.Date(2026, 11, 18, 19, 23, 10, 250_000_000, time.UTC)
	later := at.Add(time.Second)
	otherBinaries := []string{"pid 7 reedcli.test", "pid 9 loomcli.test"}
	ownChild := gitEntrySighting{at: at, attributed: true, ownInDir: true, inDir: []string{`pid 11 ["git" "init"]`}}
	aloneNotInDir := gitEntrySighting{at: at, attributed: true}
	beside := gitEntrySighting{at: at, attributed: true, others: otherBinaries}
	besideWithOwnChild := gitEntrySighting{at: at, attributed: true, others: otherBinaries, ownInDir: true}
	unread := gitEntrySighting{at: at}
	tests := []struct {
		name string
		// seed creates the `.git` entry at path; a nil seed creates none.
		seed        func(t *testing.T, path string)
		preexisting bool
		// sightings is what the watch saw for the entry, in order; none means it saw nothing.
		sightings []gitEntrySighting
		// inFirst seeds the first watched directory instead of the second.
		inFirst bool
		// skipWindows skips the row where creating its entry may need a privilege.
		skipWindows bool
		wantCode    int
		wantOutput  []string
		wantKept    bool
	}{
		{name: "NoEntryKeepsTheCode", wantCode: 0},
		{
			name:        "EntryPresentBeforeTheRunKeepsTheCodeAndTheEntry",
			seed:        mkdirGit,
			preexisting: true,
			wantCode:    0,
			wantKept:    true,
		},
		{
			name:       "EntrySeenWhileAProcessOfThisPackageWorkedInItsDirectoryFailsAndIsRemoved",
			seed:       mkdirGit,
			sightings:  []gitEntrySighting{ownChild},
			wantCode:   1,
			wantOutput: []string{"appeared at 19:23:10.250 while a process of this package was working in", "removed the empty directory"},
		},
		{
			name:       "EntrySeenInFirstDirectoryWhileAProcessOfThisPackageWorkedInItFails",
			seed:       mkdirGit,
			sightings:  []gitEntrySighting{ownChild},
			inFirst:    true,
			wantCode:   1,
			wantOutput: []string{"so it is blamed on this package", "removed the empty directory"},
		},
		{
			name:       "EntrySeenWithNoOtherTestBinaryAndNoProcessOfThisPackageInItKeepsTheCode",
			seed:       mkdirGit,
			sightings:  []gitEntrySighting{aloneNotInDir},
			wantCode:   0,
			wantOutput: []string{"machine finding, this package is not failed for it", "test binaries running beside this one: none", "removed the empty directory"},
		},
		{
			name:      "EntrySeenBesideOtherTestBinariesKeepsTheCodeAndNamesThemAsCandidates",
			seed:      mkdirGit,
			sightings: []gitEntrySighting{beside},
			wantCode:  0,
			wantOutput: []string{
				"machine finding, this package is not failed for it",
				"test binaries running beside this one: pid 7 reedcli.test, pid 9 loomcli.test",
				"the writer is one of them, this package or a process that is no test binary",
				"removed the empty directory",
			},
		},
		{
			name:       "EntrySeenBesideOtherTestBinariesWhileAProcessOfThisPackageWorkedInItsDirectoryFails",
			seed:       mkdirGit,
			sightings:  []gitEntrySighting{besideWithOwnChild},
			wantCode:   1,
			wantOutput: []string{"so it is blamed on this package"},
		},
		{
			name:       "EntrySeenNamesTheProcessesWorkingInItsDirectory",
			seed:       mkdirGit,
			sightings:  []gitEntrySighting{{at: at, attributed: true, others: otherBinaries, inDir: []string{`pid 11 ["git" "init"]`}}},
			wantCode:   0,
			wantOutput: []string{`then: pid 11 ["git" "init"]`},
		},
		{
			name:       "EntrySeenWhereProcessesCannotBeReadKeepsTheCode",
			seed:       mkdirGit,
			sightings:  []gitEntrySighting{unread},
			wantCode:   0,
			wantOutput: []string{"machine finding", "the processes running then are unknown", "removed the empty directory"},
		},
		{
			name:       "EntryNoWatchSawKeepsTheCode",
			seed:       mkdirGit,
			wantCode:   0,
			wantOutput: []string{"machine finding", "no watch saw which test binaries ran", "removed the empty directory"},
		},
		{
			name:       "EntrySeenAndAlreadyRemovedIsStillReported",
			sightings:  []gitEntrySighting{beside},
			wantCode:   0,
			wantOutput: []string{"test binaries running beside this one"},
		},
		{
			name:      "EntrySeenTwiceReportsEachSightingAndFailsOnEither",
			seed:      mkdirGit,
			sightings: []gitEntrySighting{beside, {at: later, attributed: true, ownInDir: true}},
			wantCode:  1,
			wantOutput: []string{
				"appeared at 19:23:10.250 with no process of this package",
				"appeared at 19:23:11.250 while a process of this package was working in",
			},
		},
		{
			name: "NewNonEmptyDirectoryIsKept",
			seed: func(t *testing.T, path string) {
				mkdirGit(t, path)
				if err := os.WriteFile(filepath.Join(path, "HEAD"), []byte("ref: refs/heads/main\n"), 0o644); err != nil {
					t.Fatal(err)
				}
			},
			sightings:  []gitEntrySighting{ownChild},
			wantCode:   1,
			wantOutput: []string{"so it is blamed on this package", "is not an empty directory", "remove it by hand"},
			wantKept:   true,
		},
		{
			name: "NewGitfileIsKept",
			seed: func(t *testing.T, path string) {
				if err := os.WriteFile(path, []byte("gitdir: /elsewhere\n"), 0o644); err != nil {
					t.Fatal(err)
				}
			},
			sightings:  []gitEntrySighting{ownChild},
			wantCode:   1,
			wantOutput: []string{"so it is blamed on this package", "is not a directory", "remove it by hand"},
			wantKept:   true,
		},
		{
			name: "NewSymlinkIsKept",
			seed: func(t *testing.T, path string) {
				if err := os.Symlink(filepath.Join(t.TempDir(), "target"), path); err != nil {
					t.Fatal(err)
				}
			},
			sightings:   []gitEntrySighting{ownChild},
			skipWindows: true,
			wantCode:    1,
			wantOutput:  []string{"so it is blamed on this package", "is not a directory", "remove it by hand"},
			wantKept:    true,
		},
	}
	for _, tt := range tests {
		if tt.skipWindows && runtime.GOOS == "windows" {
			continue
		}
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			dirs := []string{t.TempDir(), t.TempDir()}
			dir := dirs[1]
			if tt.inFirst {
				dir = dirs[0]
			}
			path := filepath.Join(dir, gitEntryName)
			if tt.seed != nil && tt.preexisting {
				tt.seed(t, path)
			}
			before := presentGitEntries(dirs)
			if tt.seed != nil && !tt.preexisting {
				tt.seed(t, path)
			}
			sightings := map[string][]gitEntrySighting{}
			if len(tt.sightings) > 0 {
				sightings[path] = tt.sightings
			}

			var out bytes.Buffer
			got := checkNewGitEntries(&out, dirs, before, sightings, 0)

			if got != tt.wantCode {
				t.Errorf("checkNewGitEntries code = %d; want %d", got, tt.wantCode)
			}
			for _, want := range tt.wantOutput {
				if !strings.Contains(out.String(), want) {
					t.Errorf("checkNewGitEntries output = %q; want it to hold %q", out.String(), want)
				}
			}
			if len(tt.wantOutput) == 0 && out.Len() != 0 {
				t.Errorf("checkNewGitEntries output = %q; want none", out.String())
			}
			if len(tt.wantOutput) != 0 && !strings.Contains(out.String(), path) {
				t.Errorf("checkNewGitEntries output = %q; want it to name %s", out.String(), path)
			}
			_, err := os.Lstat(path)
			if kept := err == nil; kept != tt.wantKept {
				t.Errorf("%s kept = %v; want %v", path, kept, tt.wantKept)
			}
		})
	}
}

func TestRunCheckingGitEntries(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()
	tests := []struct {
		name string
		// preexisting creates the `.git` before the run starts.
		preexisting bool
		// duringRun makes the fake run create the `.git` and wait until the watch has taken its snapshot.
		duringRun bool
		// renameFirst makes the fake run move the preexisting `.git` away before it creates a new one.
		renameFirst bool
		// missingDir makes the first watched directory one that does not exist.
		missingDir bool
		// ownInDir is what the fake snapshot reports for the run's own processes working in the directory.
		ownInDir bool
		// others is what the fake snapshot reports as the other test binaries running.
		others        []string
		runCode       int
		wantCode      int
		wantSnapshots int
	}{
		{name: "RunLeavingNoEntryKeepsTheCode", runCode: 3, wantCode: 3},
		{name: "RunCreatingAnEntryWithAProcessOfItsOwnInTheDirectoryFails", duringRun: true, ownInDir: true, wantCode: 1, wantSnapshots: 1},
		{name: "RunCreatingAnEntryWithAProcessOfItsOwnInTheDirectoryKeepsAFailingCodeFailing", duringRun: true, ownInDir: true, runCode: 2, wantCode: 1, wantSnapshots: 1},
		{name: "RunCreatingAnEntryWithNoProcessOfItsOwnInTheDirectoryKeepsTheCode", duringRun: true, runCode: 3, wantCode: 3, wantSnapshots: 1},
		{name: "RunCreatingAnEntryBesideOtherBinariesKeepsTheCode", duringRun: true, others: []string{"pid 7 reedcli.test"}, runCode: 3, wantCode: 3, wantSnapshots: 1},
		{name: "EntryPresentBeforeTheRunKeepsTheCode", preexisting: true, runCode: 0, wantCode: 0},
		{name: "RunMovingTheEntryAwayAndCreatingAnotherStillFails", preexisting: true, renameFirst: true, duringRun: true, ownInDir: true, wantCode: 1, wantSnapshots: 1},
		{name: "DirectoryWithNoWatchLeavesTheOtherWatched", missingDir: true, duringRun: true, ownInDir: true, wantCode: 1, wantSnapshots: 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			dirs := []string{t.TempDir(), t.TempDir()}
			if tt.missingDir {
				dirs[0] = filepath.Join(dirs[0], "absent")
			}
			target := dirs[0]
			if tt.missingDir {
				target = dirs[1]
			}
			path := filepath.Join(target, gitEntryName)
			if tt.preexisting {
				mkdirGit(t, path)
			}
			snapshotted := make(chan string, 8)
			var taken atomic.Int32
			snapshot := func(dir string, at time.Time) gitEntrySighting {
				taken.Add(1)
				snapshotted <- dir
				return gitEntrySighting{at: at, attributed: true, others: tt.others, ownInDir: tt.ownInDir}
			}
			run := func() int {
				if tt.renameFirst {
					if err := os.Rename(path, path+"-moved"); err != nil {
						t.Error(err)
					}
				}
				if tt.duringRun {
					mkdirGit(t, path)
					select {
					case dir := <-snapshotted:
						if dir != target {
							t.Errorf("snapshot dir = %q; want %q", dir, target)
						}
					case <-time.After(10 * time.Second):
						t.Errorf("no snapshot within 10s of creating %s", path)
					}
				}
				return tt.runCode
			}

			var out bytes.Buffer
			got := runCheckingGitEntries(&out, dirs, snapshot, run)

			if got != tt.wantCode {
				t.Errorf("runCheckingGitEntries code = %d; want %d", got, tt.wantCode)
			}
			if named := strings.Contains(out.String(), path); named != tt.duringRun {
				t.Errorf("runCheckingGitEntries output = %q; names %s = %v, want %v", out.String(), path, named, tt.duringRun)
			}
			if got := int(taken.Load()); got != tt.wantSnapshots {
				t.Errorf("runCheckingGitEntries took %d snapshots; want %d", got, tt.wantSnapshots)
			}
		})
	}
}

//testtiming:keep pins the exact other-test-binary list, the exclusion of a test-binary descendant of self and the pid-name format and order of others and inDir
func TestClassifyProcesses(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()
	const self = 100
	dir := filepath.Join(string(filepath.Separator), "tmp")
	tests := []struct {
		name         string
		procs        []processInfo
		wantOthers   []string
		wantInDir    []string
		wantOwnInDir bool
	}{
		{
			name: "SplitsTheTableIntoOtherBinariesAndProcessesInTheDirectory",
			procs: []processInfo{
				{pid: 1, ppid: 0, argv: []string{"/sbin/init"}, cwd: "/"},
				{pid: 50, ppid: 1, argv: []string{"go", "test"}, cwd: "/repo"},
				{pid: self, ppid: 50, argv: []string{"/build/b001/tmuxkit.test"}, cwd: "/repo/internal/testkit/tmuxkit"},
				{pid: 101, ppid: self, argv: []string{"/build/b001/tmuxkit.test", "-test.run=Helper"}, cwd: "/repo"},
				{pid: 102, ppid: 101, argv: []string{"git", "init"}, cwd: dir},
				{pid: 200, ppid: 50, argv: []string{"/build/b002/reedcli.test", "-test.v"}, cwd: "/repo/internal/reedcli"},
				{pid: 300, ppid: 1, argv: []string{"/build/b003/loomcli.test"}, cwd: dir},
				{pid: 400, ppid: 999, argv: []string{"bash"}, cwd: "/home"},
			},
			wantOthers:   []string{"pid 200 reedcli.test", "pid 300 loomcli.test"},
			wantInDir:    []string{`pid 102 ["git" "init"]`, `pid 300 ["/build/b003/loomcli.test"]`},
			wantOwnInDir: true,
		},
		{
			name: "NoProcessOfThisBinaryInTheDirectoryIsNotOwn",
			procs: []processInfo{
				{pid: self, ppid: 1, argv: []string{"/build/b001/tmuxkit.test"}, cwd: "/repo"},
				{pid: 300, ppid: 1, argv: []string{"/build/b003/loomcli.test"}, cwd: dir},
			},
			wantOthers: []string{"pid 300 loomcli.test"},
			wantInDir:  []string{`pid 300 ["/build/b003/loomcli.test"]`},
		},
		{
			name: "ThisBinaryWorkingInTheDirectoryIsOwn",
			procs: []processInfo{
				{pid: self, ppid: 1, argv: []string{"/build/b001/tmuxkit.test"}, cwd: dir},
			},
			wantInDir:    []string{`pid 100 ["/build/b001/tmuxkit.test"]`},
			wantOwnInDir: true,
		},
		{
			name: "ParentChainThatLoopsCountsAsAnotherBinary",
			procs: []processInfo{
				{pid: self, ppid: 1, argv: []string{"/build/b001/tmuxkit.test"}, cwd: "/repo"},
				{pid: 500, ppid: 501, argv: []string{"/build/b004/looping.test"}, cwd: "/repo"},
				{pid: 501, ppid: 500, argv: []string{"sh"}, cwd: "/repo"},
			},
			wantOthers: []string{"pid 500 looping.test"},
		},
		{
			name: "ParentMissingFromTheTableCountsAsAnotherBinary",
			procs: []processInfo{
				{pid: self, ppid: 1, argv: []string{"/build/b001/tmuxkit.test"}, cwd: "/repo"},
				{pid: 600, ppid: 999, argv: []string{"/build/b005/orphan.test"}, cwd: "/repo"},
			},
			wantOthers: []string{"pid 600 orphan.test"},
		},
		{
			name: "LongArgvIsCutAtTheLimit",
			procs: []processInfo{
				{pid: 700, ppid: 1, argv: []string{strings.Repeat("a", 300)}, cwd: dir},
			},
			wantInDir: []string{`pid 700 ["` + strings.Repeat("a", 198) + "…"},
		},
		{
			name: "LongArgvIsCutAtACharacterNotInsideOne",
			procs: []processInfo{
				{pid: 700, ppid: 1, argv: []string{strings.Repeat("é", 300)}, cwd: dir},
			},
			wantInDir: []string{`pid 700 ["` + strings.Repeat("é", 198) + "…"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			others, inDir, ownInDir := classifyProcesses(self, dir, tt.procs)
			if !slices.Equal(others, tt.wantOthers) {
				t.Errorf("classifyProcesses others = %q; want %q", others, tt.wantOthers)
			}
			if !slices.Equal(inDir, tt.wantInDir) {
				t.Errorf("classifyProcesses inDir = %q; want %q", inDir, tt.wantInDir)
			}
			if ownInDir != tt.wantOwnInDir {
				t.Errorf("classifyProcesses ownInDir = %v; want %v", ownInDir, tt.wantOwnInDir)
			}
		})
	}
}

// mkdirGit creates path as an empty directory.
func mkdirGit(t *testing.T, path string) {
	t.Helper()
	if err := os.Mkdir(path, 0o755); err != nil {
		t.Fatal(err)
	}
}
