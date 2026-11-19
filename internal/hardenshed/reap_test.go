// reap_test.go covers Reap over fake process, record and server seams.

package hardenshed

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/proc"
	"github.com/Knatte18/loomyard/internal/runproc"
	"github.com/Knatte18/loomyard/internal/state"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// reapFake records what Reap asks of its seams and answers from the fields a test sets.
type reapFake struct {
	processes []proc.Process
	stats     []proc.StatEntry
	owners    map[string]map[string][]runproc.Record
	stopped   map[string]bool
	stopErr   map[string]error

	ended        [][2]string
	endedProcess []int
	stopCalls    []string
}

func (f *reapFake) deps(root string, point ReapPoint) ReapDeps {
	return ReapDeps{
		Root: root, Row: "HardenFix", Point: point, CallerPID: 100,
		ListProcesses: func() ([]proc.Process, error) { return f.processes, nil },
		ListStats:     func() ([]proc.StatEntry, error) { return f.stats, nil },
		Owners:        func(hub string) (map[string][]runproc.Record, error) { return f.owners[hub], nil },
		End: func(hub, owner string, caller int) ([]runproc.Ended, error) {
			if caller != 100 {
				return nil, errors.New("End was not told the caller's pid")
			}
			f.ended = append(f.ended, [2]string{hub, owner})
			return nil, nil
		},
		EndProcess: func(pid int, _, _ string) ([]runproc.Ended, error) {
			f.endedProcess = append(f.endedProcess, pid)
			return nil, nil
		},
		StopServer: func(hub string) (bool, error) {
			f.stopCalls = append(f.stopCalls, hub)
			return f.stopped[hub], f.stopErr[hub]
		},
	}
}

func TestReap(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	// newRoot returns a sandbox root listing hubs "one" and "two", only "two" with a directory, and an unlisted container.
	newRoot := func(t *testing.T) (root string, hubs []string) {
		root = SandboxRoot(t.TempDir())
		hubs = []string{filepath.Join(root, "hub-one", "tst-LYMHUB"), filepath.Join(root, "hub-two", "tst-LYMHUB")}
		for _, dir := range []string{hubs[1], filepath.Join(root, "orphan", "code-bare")} {
			if err := os.MkdirAll(dir, 0o755); err != nil {
				t.Fatal(err)
			}
		}
		if err := os.WriteFile(filepath.Join(root, "orphan", "code-bare", "HEAD"), []byte("ref: refs/heads/main\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		path, lockPath := hubListPaths(root)
		if err := state.WriteJSON(path, lockPath, hubList{Hubs: hubs}); err != nil {
			t.Fatal(err)
		}
		return root, hubs
	}

	t.Run("EndsRecordedProcessesThroughEndAndUnrecordedOnesThroughEndProcess", func(t *testing.T) {
		t.Parallel()
		root, hubs := newRoot(t)
		fake := &reapFake{
			owners: map[string]map[string][]runproc.Record{hubs[1]: {"run": {{PID: 11}}}},
			processes: []proc.Process{
				{PID: 11, Cwd: hubs[1]},
				{PID: 12, Argv: []string{"lym", "--hub=" + hubs[1]}},
				{PID: 13, Cwd: root + "2"},
				{PID: 14, Cwd: "/elsewhere"},
			},
		}

		strays, err := Reap(fake.deps(root, ReapEnd))
		if err != nil {
			t.Fatalf("Reap() error = %v", err)
		}

		if want := [][2]string{{hubs[1], "run"}}; !reflect.DeepEqual(fake.ended, want) {
			t.Errorf("End calls = %v; want %v", fake.ended, want)
		}
		if want := []int{12}; !reflect.DeepEqual(fake.endedProcess, want) {
			t.Errorf("EndProcess pids = %v; want %v", fake.endedProcess, want)
		}
		want := []StrayEntry{
			{Row: "HardenFix", Point: ReapEnd, Kind: StrayProcess, PID: 11, Recorded: true},
			{Row: "HardenFix", Point: ReapEnd, Kind: StrayProcess, PID: 12, Argv: []string{"lym", "--hub=" + hubs[1]}},
		}
		if !reflect.DeepEqual(strays, want) {
			t.Errorf("Reap() strays = %+v; want %+v", strays, want)
		}
	})

	t.Run("LeavesTheCallersAncestors", func(t *testing.T) {
		t.Parallel()
		root, _ := newRoot(t)
		fake := &reapFake{
			processes: []proc.Process{{PID: 90, Cwd: root}, {PID: 100, Cwd: root}, {PID: 12, Cwd: root}},
			stats:     []proc.StatEntry{{PID: 100, PPID: 90}, {PID: 90, PPID: 1}, {PID: 12, PPID: 1}},
		}
		if _, err := Reap(fake.deps(root, ReapEnd)); err != nil {
			t.Fatalf("Reap() error = %v", err)
		}
		if want := []int{12}; !reflect.DeepEqual(fake.endedProcess, want) {
			t.Errorf("EndProcess pids = %v; want %v", fake.endedProcess, want)
		}
	})

	t.Run("StopsAGoneHubsServerAndPrunesEveryHubWithNoLiveServerLeft", func(t *testing.T) {
		t.Parallel()
		root, hubs := newRoot(t)
		fake := &reapFake{stopped: map[string]bool{hubs[0]: true}}

		strays, err := Reap(fake.deps(root, ReapHalt))
		if err != nil {
			t.Fatalf("Reap() error = %v", err)
		}

		if !slices.Equal(fake.stopCalls, hubs) {
			t.Errorf("StopServer calls = %v; want %v", fake.stopCalls, hubs)
		}
		if want := []StrayEntry{{Row: "HardenFix", Point: ReapHalt, Kind: StrayServer, Hub: hubs[0]}}; !reflect.DeepEqual(strays, want) {
			t.Errorf("Reap() strays = %+v; want %+v", strays, want)
		}
		if listed, err := ListSandbox(root); err != nil || len(listed) != 0 {
			t.Errorf("ListSandbox() = %v, %v; want every hub pruned", listed, err)
		}
		entries, err := os.ReadDir(root)
		if err != nil {
			t.Fatal(err)
		}
		for _, entry := range entries {
			if entry.Name() != hubListFileName && entry.Name() != hubListFileName+hubListLockSuffix {
				t.Errorf("the root still holds %s; want only the hub list", entry.Name())
			}
		}
	})

	t.Run("KeepsAnEntryWhoseServerCouldNotBeStoppedAndNamesTheWayForward", func(t *testing.T) {
		t.Parallel()
		root, hubs := newRoot(t)
		fake := &reapFake{stopErr: map[string]error{hubs[0]: errors.New("tmux unreachable")}}

		_, err := Reap(fake.deps(root, ReapClear))

		if err == nil || !strings.Contains(err.Error(), "tmux unreachable") ||
			!strings.Contains(err.Error(), "way forward: run lym harden sandbox clear in the task worktree again") {
			t.Fatalf("Reap() error = %v; want the stop failure with its way forward", err)
		}
		if listed, _ := ListSandbox(root); !slices.Equal(listed, []string{hubs[0]}) {
			t.Errorf("ListSandbox() = %v; want only %v kept", listed, hubs[0])
		}
	})

	t.Run("MarksWhatAStartReapFindsAsInterrupted", func(t *testing.T) {
		t.Parallel()
		root, _ := newRoot(t)
		fake := &reapFake{processes: []proc.Process{{PID: 12, Cwd: root}}}
		strays, err := Reap(fake.deps(root, ReapStart))
		if err != nil || len(strays) != 1 || !strays[0].Interrupted {
			t.Errorf("Reap() = %+v, %v; want one interrupted entry", strays, err)
		}
	})

	t.Run("ReapsOnlyServersWithoutAProcessLister", func(t *testing.T) {
		t.Parallel()
		root, hubs := newRoot(t)
		fake := &reapFake{processes: []proc.Process{{PID: 12, Cwd: root}}, stopped: map[string]bool{hubs[1]: true}}
		deps := fake.deps(root, ReapEnd)
		deps.ListProcesses = nil

		strays, err := Reap(deps)
		if err != nil {
			t.Fatalf("Reap() error = %v", err)
		}
		if len(fake.endedProcess) != 0 {
			t.Errorf("EndProcess pids = %v; want none", fake.endedProcess)
		}
		if len(strays) != 1 || strays[0].Kind != StrayServer {
			t.Errorf("Reap() strays = %+v; want the one server", strays)
		}
	})
}
