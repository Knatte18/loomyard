// processcap_test.go covers the process cap's kill decisions over a scripted process list,
// and the run marker it watches.

package gatecli

import (
	"context"
	"errors"
	"slices"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/proc"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// capSample is what the fake process list returns on one call.
type capSample struct {
	pids []int
	err  error
}

// capRoot is the pid of the process that starts the tier commands in a scripted cap.
const capRoot = 100

// runScriptedCap runs a cap of limit over script and returns its line and the pids it killed, in order.
// It returns once the script's last list has been taken.
// A non-nil running makes the cap count only processes below one of those tier command pids, over tree as the sample.
func runScriptedCap(limit int, script []capSample, tree []proc.ProcessSample, running []int) (string, []int) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var killed []int
	taken := 0
	var commands *runningCommands
	if running != nil {
		commands = &runningCommands{}
		for _, pid := range running {
			commands.add(pid)
		}
	}
	c := processCap{
		commands: commands,
		root:     capRoot,
		sample:   func(int) ([]proc.ProcessSample, error) { return tree, nil },
		limit:    limit,
		period:   time.Second,
		marker:   "LYM_GATE_TEST_PID=7",
		list: func(string) ([]int, error) {
			next := script[taken]
			taken++
			if taken == len(script) {
				cancel()
			}
			return next.pids, next.err
		},
		kill: func(pid int) error {
			killed = append(killed, pid)
			return nil
		},
		after: func(time.Duration) <-chan time.Time {
			elapsed := make(chan time.Time, 1)
			elapsed <- time.Time{}
			return elapsed
		},
	}
	return c.run(ctx), killed
}

func TestProcessCap(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()
	const limit = 3
	tests := []struct {
		name   string
		script []capSample
		// tree and running are the process sample and the running tier command pids; nil running counts every listed pid.
		tree       []proc.ProcessSample
		running    []int
		wantLine   string
		wantKilled []int
	}{
		{
			name:   "AtTheCapNothingIsKilled",
			script: []capSample{{pids: []int{1, 2}}, {pids: []int{1, 2, 3}}},
		},
		{
			name:       "OverTheCapEveryListedProcessIsKilledThenAndAfter",
			script:     []capSample{{pids: []int{1, 2}}, {pids: []int{1, 2, 3, 4}}, {pids: []int{5}}},
			wantLine:   processCapLine(4, limit),
			wantKilled: []int{1, 2, 3, 4, 5},
		},
		{
			name:       "AFailedListIsSkipped",
			script:     []capSample{{err: errors.New("proc unreadable")}, {pids: []int{1, 2, 3, 4}}},
			wantLine:   processCapLine(4, limit),
			wantKilled: []int{1, 2, 3, 4},
		},
		{
			name:   "AnUnsupportedPlatformEndsTheCap",
			script: []capSample{{err: errors.ErrUnsupported}, {pids: []int{1, 2, 3, 4}}},
		},
		{
			name:   "MarkedProcessesOutsideEveryTierCommandAreNotCounted",
			script: []capSample{{pids: []int{1, 2, 3, 4}}},
			tree: []proc.ProcessSample{
				{PID: 10, Parent: capRoot}, {PID: 1, Parent: 10}, {PID: 2, Parent: 10},
				{PID: 3, Parent: capRoot}, {PID: 4, Parent: capRoot},
			},
			running: []int{10},
		},
		{
			name:   "MarkedProcessesBelowATierCommandAndOnesTheSampleLostAreCounted",
			script: []capSample{{pids: []int{1, 2, 3, 5}}},
			tree: []proc.ProcessSample{
				{PID: 10, Parent: capRoot}, {PID: 1, Parent: 10}, {PID: 2, Parent: 10}, {PID: 3, Parent: 2},
			},
			running:    []int{10},
			wantLine:   processCapLine(4, limit),
			wantKilled: []int{1, 2, 3, 5},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			line, killed := runScriptedCap(limit, tt.script, tt.tree, tt.running)

			if line != tt.wantLine {
				t.Errorf("processCap.run() = %q; want %q", line, tt.wantLine)
			}
			if !slices.Equal(killed, tt.wantKilled) {
				t.Errorf("processCap.run() killed %v; want %v", killed, tt.wantKilled)
			}
		})
	}
}

func TestRunMarker(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()
	tests := []struct {
		name string
		env  []string
		want string
	}{
		{"NoMarker", []string{"HOME=/home/u"}, ""},
		{"OneMarker", []string{"HOME=/home/u", enclosingGateEnv + "=12"}, enclosingGateEnv + "=12"},
		{"TheLastMarkerWins", []string{enclosingGateEnv + "=12", enclosingGateEnv + "=34"}, enclosingGateEnv + "=34"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := runMarker(tt.env); got != tt.want {
				t.Errorf("runMarker(%q) = %q; want %q", tt.env, got, tt.want)
			}
		})
	}
}
