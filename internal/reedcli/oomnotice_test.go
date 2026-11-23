// oomnotice_test.go pins which strand panes the watchdog tells of an OOM kill, the line it tells it in,
// and that a new kill reaches the parent orch's post once.

package reedcli

import (
	"context"
	"errors"
	"reflect"
	"slices"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/proc"
	"github.com/Knatte18/loomyard/internal/reedengine"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

func TestPlanPaneOOMNotices(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	// Each pane shell's pid reads as the cgroup and kill count below; pid 4 cannot be read.
	// Pid 6 shares a tmux scope the memory cap never reached.
	kills := map[int]struct {
		cgroup string
		count  int
	}{
		1: {"/app.slice/lym-pane-1.scope", 0},
		2: {"/app.slice/lym-pane-2.scope", 2},
		3: {"/app.slice/lym-pane-3.scope", 3},
		6: {"/app.slice/tmux-spawn-0a1b.scope", 3},
	}
	readKills := func(pid int) (string, int, error) {
		k, ok := kills[pid]
		if !ok {
			return "", 0, errors.New("no such process")
		}
		return k.cgroup, k.count, nil
	}
	strands := []reedengine.StrandStatus{
		{Name: "quiet", PaneID: "%1", PanePID: 1},
		{Name: "first-sight", PaneID: "%2", PanePID: 2},
		{Name: "one-more", PaneID: "%3", PanePID: 3},
		{Name: "unreadable", PaneID: "%4", PanePID: 4},
		{Name: "dead", PaneID: "%5"},
		{Name: "uncapped", PaneID: "%6", PanePID: 6},
	}
	tests := []struct {
		name     string
		seen     paneOOMKills
		want     []paneOOMNotice
		wantSeen paneOOMKills
	}{
		{
			"ANewKillIsToldFromTheCountToldBeforeAndAGonePaneIsForgotten",
			paneOOMKills{"/app.slice/lym-pane-3.scope": 2, "/app.slice/lym-pane-9.scope": 1},
			[]paneOOMNotice{
				{strand: "first-sight", pane: "%2", cgroup: "/app.slice/lym-pane-2.scope", told: 0, newKills: 2},
				{strand: "one-more", pane: "%3", cgroup: "/app.slice/lym-pane-3.scope", told: 2, newKills: 1},
			},
			paneOOMKills{"/app.slice/lym-pane-3.scope": 2},
		},
		{
			"KillsAlreadyToldAreNotToldAgain",
			paneOOMKills{"/app.slice/lym-pane-2.scope": 2, "/app.slice/lym-pane-3.scope": 3},
			nil,
			paneOOMKills{"/app.slice/lym-pane-2.scope": 2, "/app.slice/lym-pane-3.scope": 3},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := planPaneOOMNotices(strands, readKills, tt.seen)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("planPaneOOMNotices() = %+v; want %+v", got, tt.want)
			}
			if !reflect.DeepEqual(tt.seen, tt.wantSeen) {
				t.Errorf("planPaneOOMNotices() left seen %v; want %v", tt.seen, tt.wantSeen)
			}
		})
	}
}

func TestPaneOOMNoticeLine(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	tests := []struct {
		name       string
		told       int
		killed     []proc.OOMKill
		journalErr error
		want       string
	}{
		{
			"TheJournalNamesTheKillAfterTheOneAlreadyTold",
			1,
			[]proc.OOMKill{{PID: 10, Name: "go"}, {PID: 11, Name: "script"}},
			nil,
			"reed: the kernel's OOM killer ended script (pid 11) at the memory cap of strand ly:darn, pane %7, in worktree task-a; the pane lives on",
		},
		{
			"TheJournalHoldsOnlyTheKillAlreadyTold",
			1,
			[]proc.OOMKill{{PID: 10, Name: "go"}},
			nil,
			"reed: the kernel's OOM killer ended 1 process(es) the kernel journal does not name at the memory cap of strand ly:darn, pane %7, in worktree task-a; the pane lives on",
		},
		{
			"TheJournalNamesNoKill",
			0,
			nil,
			nil,
			"reed: the kernel's OOM killer ended 1 process(es) the kernel journal does not name at the memory cap of strand ly:darn, pane %7, in worktree task-a; the pane lives on",
		},
		{
			"TheJournalCannotBeRead",
			0,
			nil,
			errors.New("permission denied"),
			"reed: the kernel's OOM killer ended 1 process(es) the kernel journal does not name (permission denied) at the memory cap of strand ly:darn, pane %7, in worktree task-a; the pane lives on",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			notice := paneOOMNotice{strand: "ly:darn", pane: "%7", cgroup: "/app.slice/lym-pane-7.scope", told: tt.told, newKills: 1}
			if got := paneOOMNoticeLine("task-a", notice, tt.killed, tt.journalErr); got != tt.want {
				t.Errorf("paneOOMNoticeLine() = %q; want %q", got, tt.want)
			}
		})
	}
}

// oomBoundariesOfOnePane returns boundaries that show one capped strand pane whose cgroup counts *kills.
// The journal holds journal, and the post appends to *posted and answers *postErr.
func oomBoundariesOfOnePane(kills *int, journal []proc.OOMKill, posted *[]string, postErr *error) paneOOMBoundaries {
	return paneOOMBoundaries{
		strands: func() ([]reedengine.StrandStatus, error) {
			return []reedengine.StrandStatus{{Name: "ly:darn", PaneID: "%7", PanePID: 7}}, nil
		},
		paneKills:    func(int) (string, int, error) { return "/app.slice/lym-pane-7.scope", *kills, nil },
		journalKills: func(string) ([]proc.OOMKill, error) { return journal, nil },
		post: func(line string) error {
			*posted = append(*posted, line)
			return *postErr
		},
	}
}

func TestReportPaneOOMKills(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	const namedLine = "reed: the kernel's OOM killer ended script (pid 11) at the memory cap of strand ly:darn, pane %7, in worktree task-a; the pane lives on"
	const countedLine = "reed: the kernel's OOM killer ended 1 process(es) the kernel journal does not name at the memory cap of strand ly:darn, pane %7, in worktree task-a; the pane lives on"
	kills := 1
	var posted []string
	var postErr error = errors.New("creel is down")
	boundaries := oomBoundariesOfOnePane(&kills, []proc.OOMKill{{PID: 11, Name: "script"}}, &posted, &postErr)
	seen := paneOOMKills{}

	if got := reportPaneOOMKills("hub", "task-a", boundaries, seen); got {
		t.Errorf("reportPaneOOMKills() with a failing post = %v; want false", got)
	}
	if !slices.Equal(posted, []string{namedLine}) || len(seen) != 0 {
		t.Fatalf("after a failed post: posted %q, seen %v; want one posted line and nothing told", posted, seen)
	}

	postErr = nil
	if got := reportPaneOOMKills("hub", "task-a", boundaries, seen); !got {
		t.Errorf("reportPaneOOMKills() retrying the failed post = %v; want true", got)
	}
	if want := []string{namedLine, namedLine}; !slices.Equal(posted, want) || seen["/app.slice/lym-pane-7.scope"] != 1 {
		t.Fatalf("after the retry: posted %q, seen %v; want %q and the kill told", posted, seen, want)
	}

	if got := reportPaneOOMKills("hub", "task-a", boundaries, seen); got || len(posted) != 2 {
		t.Errorf("reportPaneOOMKills() with the same count = %v, posted %q; want false and no new post", got, posted)
	}

	// A second kill is counted before the journal holds its line, so the journal still holds only the first kill.
	kills = 2
	if got := reportPaneOOMKills("hub", "task-a", boundaries, seen); !got {
		t.Errorf("reportPaneOOMKills() with a new kill = %v; want true", got)
	}
	if want := []string{namedLine, namedLine, countedLine}; !slices.Equal(posted, want) {
		t.Errorf("posted %q; want %q", posted, want)
	}
}

func TestWatchPaneOOMKills_PostsANewKillOnceAndStopsWithItsContext(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	kills := 1
	var postErr error
	var posted []string
	boundaries := oomBoundariesOfOnePane(&kills, []proc.OOMKill{{PID: 11, Name: "script"}}, &posted, &postErr)
	lines := make(chan string, 8)
	boundaries.post = func(line string) error {
		lines <- line
		return nil
	}
	reads := make(chan struct{}, 1024)
	readKills := boundaries.paneKills
	boundaries.paneKills = func(pid int) (string, int, error) {
		select {
		case reads <- struct{}{}:
		default:
		}
		return readKills(pid)
	}
	timing := watchdogTiming{DiscoveryCycle: time.Millisecond, DiscoveryCeiling: time.Millisecond}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		defer close(done)
		watchPaneOOMKills(ctx, "hub", "task-a", boundaries, timing)
	}()

	const want = "reed: the kernel's OOM killer ended script (pid 11) at the memory cap of strand ly:darn, pane %7, in worktree task-a; the pane lives on"
	select {
	case got := <-lines:
		if got != want {
			t.Errorf("the watch posted %q; want %q", got, want)
		}
	case <-time.After(30 * time.Second):
		t.Fatal("the watch posted no notice for a new kill")
	}
	// The watch reads on after the post; the kill must not be told again by any of those reads.
	for range 3 {
		select {
		case <-reads:
		case <-time.After(30 * time.Second):
			t.Fatal("the watch stopped reading the strand panes")
		}
	}
	cancel()
	select {
	case <-done:
	case <-time.After(30 * time.Second):
		t.Fatal("the watch did not stop when its context was done")
	}
	if extra := len(lines); extra != 0 {
		t.Errorf("the watch posted %d more notice(s) for the same kill; want none", extra)
	}
}
