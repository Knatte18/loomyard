// gitentry.go holds Main's check for a `.git` that appears directly in a shared temp directory during a run.
// It also holds the watch that names the test binaries and processes running at the moment the entry appeared.

package tmuxkit

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/Knatte18/loomyard/internal/fswatch"
)

// gitEntryName is the repository marker.
// Left directly in a shared temp directory, it turns everything below that directory into one repository.
const gitEntryName = ".git"

// testBinarySuffix ends the name of every binary `go test` builds.
const testBinarySuffix = ".test"

// maxArgvChars bounds how much of a process's argv a finding quotes.
const maxArgvChars = 200

// gitEntryCause is the part of every finding that says what wrote the entry and why it matters.
const gitEntryCause = "a test or the code it drives wrote outside its own t.TempDir, often through a path joined onto an empty or parent directory, and go's VCS stamping and git's discovery read everything below it as one repository"

// gitEntrySighting is what the watch saw when a `.git` appeared in a shared temp directory.
type gitEntrySighting struct {
	// at is when the watch saw the entry appear.
	at time.Time
	// attributed is false where the process table cannot be read or does not hold this binary.
	// Then neither the other test binaries nor this binary's own processes are known.
	attributed bool
	// others describes each test binary running when the entry appeared, other than this one and its descendants.
	others []string
	// inDir describes each process whose working directory was the directory the entry appeared in.
	inDir []string
	// ownInDir reports whether this binary or one of its descendants had its working directory in that directory.
	// It is the only evidence that ties the entry to this binary.
	ownInDir bool
}

// processInfo is one process as the snapshot reads it.
type processInfo struct {
	pid  int
	ppid int
	argv []string
	cwd  string
}

// sharedTempDirs returns the directories above the run directory dir that every test binary on the machine shares.
// They are the lym temp root holding dir and the system temp directory holding that root.
func sharedTempDirs(dir string) []string {
	root := filepath.Dir(dir)
	return []string{root, filepath.Dir(root)}
}

// presentGitEntries returns the set of `.git` paths that exist directly in dirs, whatever their type.
func presentGitEntries(dirs []string) map[string]bool {
	present := map[string]bool{}
	for _, dir := range dirs {
		path := filepath.Join(dir, gitEntryName)
		if _, err := os.Lstat(path); err == nil {
			present[path] = true
		}
	}
	return present
}

// runCheckingGitEntries snapshots the `.git` entries in dirs, watches dirs while it calls run, and checks them after.
// snapshot names the processes running at the moment the watch sees an entry appear.
// It returns run's exit code, or 1 when the check blames this run for an entry, and names each finding on w.
func runCheckingGitEntries(w io.Writer, dirs []string, snapshot func(dir string, at time.Time) gitEntrySighting, run func() int) int {
	before := presentGitEntries(dirs)
	stop := watchGitEntries(dirs, snapshot)
	code := run()
	return checkNewGitEntries(w, dirs, before, stop(), code)
}

// watchGitEntries watches dirs for a `.git` being created and calls snapshot the moment one is.
// The returned stop ends the watches and returns every sighting per entry path, in the order they were taken.
// A directory whose watch cannot be opened yields no sighting; its entries are still checked after the run.
func watchGitEntries(dirs []string, snapshot func(dir string, at time.Time) gitEntrySighting) (stop func() map[string][]gitEntrySighting) {
	var (
		mu        sync.Mutex
		sightings = map[string][]gitEntrySighting{}
		consumers sync.WaitGroup
		watchers  []*fswatch.Watcher
	)
	for _, dir := range dirs {
		watcher, err := fswatch.Watch(dir, gitEntryName)
		if err != nil {
			continue
		}
		watchers = append(watchers, watcher)
		consumers.Add(1)
		go func() {
			defer consumers.Done()
			for event := range watcher.Events() {
				if event.Op != "create" {
					continue
				}
				sighting := snapshot(dir, time.Now())
				mu.Lock()
				path := filepath.Join(dir, gitEntryName)
				sightings[path] = append(sightings[path], sighting)
				mu.Unlock()
			}
		}()
	}
	return func() map[string][]gitEntrySighting {
		for _, watcher := range watchers {
			_ = watcher.Close()
		}
		consumers.Wait()
		return sightings
	}
}

// checkNewGitEntries reports each `.git` directly in dirs that appeared during the run, and returns the run's code.
// An entry appeared when a sighting holds it or when it exists now and before does not hold it.
// Every sighting of an entry is reported.
// The code becomes 1 only for an entry seen appearing while this binary or a descendant worked in its directory.
// Any other entry is a machine finding that names the binaries running beside this one and keeps the code.
// An entry that still exists is removed when it is an empty directory, so it breaks no later build.
// Any other existing entry is named for removal by hand.
// The entries are reported in the order of dirs.
func checkNewGitEntries(w io.Writer, dirs []string, before map[string]bool, sightings map[string][]gitEntrySighting, code int) int {
	for _, dir := range dirs {
		path := filepath.Join(dir, gitEntryName)
		seen := sightings[path]
		info, err := os.Lstat(path)
		exists := err == nil
		if len(seen) == 0 && (!exists || before[path]) {
			continue
		}
		if len(seen) == 0 {
			fmt.Fprintln(w, describeGitEntry(path, dir, gitEntrySighting{}, false))
		}
		for _, sighting := range seen {
			fmt.Fprintln(w, describeGitEntry(path, dir, sighting, true))
			if sighting.attributed && sighting.ownInDir {
				code = 1
			}
		}
		if exists {
			fmt.Fprintln(w, removeStrayGitEntry(path, info))
		}
	}
	return code
}

// describeGitEntry returns the finding line for the `.git` at path in dir, from the sighting when seen is true.
func describeGitEntry(path, dir string, sighting gitEntrySighting, seen bool) string {
	var line string
	switch {
	case !seen:
		line = fmt.Sprintf("tmuxkit: machine finding, this package is not failed for it: %s appeared during this package's run, and no watch saw which test binaries ran when it appeared; %s", path, gitEntryCause)
	case !sighting.attributed:
		line = fmt.Sprintf("tmuxkit: machine finding, this package is not failed for it: %s appeared at %s, and the processes running then are unknown: this platform has no process table to read, or the one read does not hold this binary; %s", path, sighting.at.Format(time.TimeOnly+".000"), gitEntryCause)
	case sighting.ownInDir:
		line = fmt.Sprintf("tmuxkit: %s appeared at %s while a process of this package was working in %s, so it is blamed on this package; %s", path, sighting.at.Format(time.TimeOnly+".000"), dir, gitEntryCause)
	default:
		beside := "none"
		if len(sighting.others) > 0 {
			beside = strings.Join(sighting.others, ", ")
		}
		line = fmt.Sprintf("tmuxkit: machine finding, this package is not failed for it: %s appeared at %s with no process of this package working in %s; test binaries running beside this one: %s; the writer is one of them, this package or a process that is no test binary; %s", path, sighting.at.Format(time.TimeOnly+".000"), dir, beside, gitEntryCause)
	}
	if seen && len(sighting.inDir) > 0 {
		line += fmt.Sprintf("; processes working in %s then: %s", dir, strings.Join(sighting.inDir, ", "))
	}
	return line
}

// removeStrayGitEntry removes the `.git` entry at path, described by info, when it is an empty directory.
// It returns the line that says what became of it.
// A symlink or file is never removed or followed.
func removeStrayGitEntry(path string, info fs.FileInfo) string {
	if !info.IsDir() {
		return fmt.Sprintf("tmuxkit: %s is not a directory; remove it by hand", path)
	}
	err := os.Remove(path)
	switch {
	case err == nil:
		return fmt.Sprintf("tmuxkit: removed the empty directory %s", path)
	case errors.Is(err, fs.ErrNotExist):
		return fmt.Sprintf("tmuxkit: %s was already removed by another process", path)
	}
	if entries, readErr := os.ReadDir(path); readErr == nil && len(entries) > 0 {
		return fmt.Sprintf("tmuxkit: %s is not an empty directory; remove it by hand", path)
	}
	return fmt.Sprintf("tmuxkit: could not remove %s: %v; remove it by hand", path, err)
}

// snapshotProcesses returns the sighting of a `.git` that appeared in dir at at, read from /proc.
// Off Linux it returns an unattributed sighting.
// So it does when the table it reads holds no readable entry for this binary itself.
func snapshotProcesses(dir string, at time.Time) gitEntrySighting {
	if runtime.GOOS != "linux" {
		return gitEntrySighting{at: at}
	}
	self := os.Getpid()
	listedSelf := false
	var procs []processInfo
	for _, pid := range Pids() {
		argv, ok := ProcArgv(pid)
		if !ok || len(argv) == 0 {
			continue
		}
		if pid == self {
			listedSelf = true
		}
		ppid, _ := ProcPPID(pid)
		cwd, _ := ProcCwd(pid)
		procs = append(procs, processInfo{pid: pid, ppid: ppid, argv: argv, cwd: cwd})
	}
	if !listedSelf {
		return gitEntrySighting{at: at}
	}
	others, inDir, ownInDir := classifyProcesses(self, dir, procs)
	return gitEntrySighting{at: at, attributed: true, others: others, inDir: inDir, ownInDir: ownInDir}
}

// classifyProcesses splits procs for a `.git` that appeared in dir, seen by the process self.
// others describes every test binary that is neither self nor a descendant of self, by pid and binary name.
// inDir describes every process working in dir, by pid and argv.
// ownInDir reports whether self or a descendant of self works in dir.
func classifyProcesses(self int, dir string, procs []processInfo) (others, inDir []string, ownInDir bool) {
	parents := make(map[int]int, len(procs))
	for _, p := range procs {
		parents[p.pid] = p.ppid
	}
	for _, p := range procs {
		own := descendsFrom(p.pid, self, parents)
		if p.cwd == dir {
			inDir = append(inDir, fmt.Sprintf("pid %d %s", p.pid, quoteArgv(p.argv)))
			ownInDir = ownInDir || own
		}
		if strings.HasSuffix(filepath.Base(p.argv[0]), testBinarySuffix) && !own {
			others = append(others, fmt.Sprintf("pid %d %s", p.pid, filepath.Base(p.argv[0])))
		}
	}
	return others, inDir, ownInDir
}

// descendsFrom reports whether pid is ancestor or one of its descendants, following parents.
// A parent chain that loops or leaves the table ends the walk.
func descendsFrom(pid, ancestor int, parents map[int]int) bool {
	seen := map[int]bool{}
	for pid > 0 && !seen[pid] {
		if pid == ancestor {
			return true
		}
		seen[pid] = true
		next, ok := parents[pid]
		if !ok {
			return false
		}
		pid = next
	}
	return false
}

// quoteArgv returns argv quoted, cut to maxArgvChars characters.
func quoteArgv(argv []string) string {
	quoted := fmt.Sprintf("%q", argv)
	if runes := []rune(quoted); len(runes) > maxArgvChars {
		return string(runes[:maxArgvChars]) + "…"
	}
	return quoted
}
