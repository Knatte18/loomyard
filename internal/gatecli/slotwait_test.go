// slotwait_test.go covers the slot wait a tier run prints on its summary line and keeps in its run records.
// The wait is read from a stand-in clock, so no row waits for a slot.

package gatecli

import (
	"bytes"
	"context"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/gateslot"
	"github.com/Knatte18/loomyard/internal/lymcwd"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// TestSummaryLineShowsTheSlotWaitAlone pins the wait a tier run's summary line shows.
// The stand-in clock reads 300 s later once a slot is held, and 900 s later once the shared build has run.
// So the line shows 300 s only when the wait ends where the slots are taken.
func TestSummaryLineShowsTheSlotWaitAlone(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()
	if runtime.GOOS == "windows" {
		t.Skip("the tier command is a POSIX shell command")
	}

	tests := []struct {
		name    string
		command string
		// build is the shared build step the tier opts into, empty for a tier with none.
		build   string
		copies  int
		carried bool
		// wantLine opens the summary line the run prints.
		wantLine string
		// wantRecordWaits are the slot waits of the run records, in the order they were written.
		wantRecordWaits []float64
	}{
		{
			name: "ASlottedRun", command: "exit 0",
			wantLine: "gate test: tier0 all wait 300.0s discovered ", wantRecordWaits: []float64{300},
		},
		{
			name: "ACarriedTier", command: "exit 0", carried: true,
			wantLine: "gate test: tier0 all carried wait 0.0s discovered ", wantRecordWaits: []float64{0},
		},
		{
			name: "ATierBehindTheSharedBuild", command: "exit 0", build: ": > built",
			wantLine: "gate test: tier0 all wait 300.0s discovered ", wantRecordWaits: []float64{0, 300},
		},
		{
			name: "ARerunTier", command: "if [ ! -e ran ]; then : > ran; echo 'resource temporarily unavailable'; exit 1; fi",
			wantLine: "gate test: tier0 all wait 300.0s discovered ", wantRecordWaits: []float64{300, 0},
		},
		{
			name: "ACopiesRun", command: "exit 0", copies: 2,
			wantLine: "gate test: tier0 all wait 300.0s discovered ", wantRecordWaits: []float64{300},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			dir := t.TempDir()
			pool := &gateslot.Pool{
				Dir:    t.TempDir(),
				Limits: func() (gateslot.Limits, error) { return gateslot.Limits{Slots: 2}, nil },
				Poll:   time.Millisecond,
			}
			start := time.Date(2026, 3, 20, 12, 0, 0, 0, time.UTC)
			var stderr bytes.Buffer
			exe := &execution{
				ctx: context.Background(),
				out: io.Discard,
				request: testRequest{
					root: dir, stderr: &stderr, copies: tt.copies, timeoutUnit: 30 * time.Second, pipeWaitDelay: defaultPipeWaitDelay,
				},
				cfg:       gateslot.Config{Build: gateslot.BuildStep{Command: tt.build, Dir: dir, TimeoutMin: 1}},
				location:  &lymcwd.Location{HubPath: t.TempDir(), WorktreeName: "work"},
				pool:      pool,
				env:       []string{"PATH=" + os.Getenv("PATH")},
				runLogDir: t.TempDir(),
				callID:    "slotwait",
				slotClock: func() time.Time {
					if _, err := os.Stat(filepath.Join(dir, "built")); err == nil {
						return start.Add(900 * time.Second)
					}
					holders, err := pool.Holders()
					if err != nil {
						t.Errorf("Holders() error = %v", err)
					}
					if len(holders) > 0 {
						return start.Add(300 * time.Second)
					}
					return start
				},
			}
			run := tierRun{
				number:      0,
				tier:        gateslot.Tier{Command: tt.command, Counter: counterGoJSON, TimeoutMin: 1, Slots: 1, SharedBuild: tt.build != ""},
				scopeForm:   "all",
				invocations: []invocation{{dir: dir}},
			}

			var end runEnd
			if tt.carried {
				end = exe.reportCarried(run)
			} else {
				end = exe.runGreenAware(run)
			}

			if end.code != 0 || end.halt {
				t.Fatalf("run = %+v, stderr %q; want exit 0 and no halt", end, stderr.String())
			}
			if !slices.ContainsFunc(strings.Split(stderr.String(), "\n"), func(line string) bool { return strings.HasPrefix(line, tt.wantLine) }) {
				t.Errorf("stderr = %q; want a summary line opening %q", stderr.String(), tt.wantLine)
			}
			records, err := gateslot.ReadRunRecords(exe.runLogDir)
			if err != nil {
				t.Fatalf("ReadRunRecords() error = %v", err)
			}
			var waits []float64
			for _, record := range records {
				waits = append(waits, record.SlotWaitSeconds())
			}
			if !slices.Equal(waits, tt.wantRecordWaits) {
				t.Errorf("run records' slot waits = %v; want %v", waits, tt.wantRecordWaits)
			}
		})
	}
}
