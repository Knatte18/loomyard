// recordings_realtmux_test.go drives each scenario over real tmux, to compare or rewrite its recording.

package tmuxrec_test

import (
	"bytes"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"

	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/proc"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
	"github.com/Knatte18/loomyard/internal/testkit/tmuxkit"
	"github.com/Knatte18/loomyard/internal/testkit/tmuxrec"
)

// writeDir, when set, is the directory the real-tmux test writes each scenario's recording into, instead of comparing.
var writeDir = flag.String("tmuxrec.write", "", "write the recordings again from real tmux into this directory")

// recordCommand is the command a recording names as its source.
const recordCommand = "lym gate test --tier 2 ./internal/testkit/tmuxrec -- -run TestRecordings_RealTmux -tmuxrec.write <dir>"

// paneEndPoll is how long a scenario waits between two status reads while a pane's process ends.
const paneEndPoll = 200 * time.Millisecond

// realTmux returns the run of tmuxPath on the server of the -L socket key socket, as reed's own tmux call runs it:
// a server it starts reads no operator config, and a failure carries tmux's standard error in its text.
func realTmux(tmuxPath, socket string) tmuxrec.TmuxRun {
	return func(stdin []byte, args ...string) (string, error) {
		full := append([]string{"-f", os.DevNull, "-L", socket}, args...)
		logger.Debug("tmuxrec: tmux", "args", full)
		cmd := exec.Command(tmuxPath, full...)
		var stdout, stderr bytes.Buffer
		cmd.Stdout, cmd.Stderr = &stdout, &stderr
		if stdin != nil {
			cmd.Stdin = bytes.NewReader(stdin)
		}
		err := cmd.Run()
		if err != nil {
			if msg := strings.TrimSpace(stderr.String()); msg != "" {
				err = fmt.Errorf("%w: %s", err, msg)
			}
		}
		return stdout.String(), err
	}
}

// TestRecordings_RealTmux drives every scenario over real tmux and compares what reed read with its recording.
// With -tmuxrec.write it writes each recording again instead.
func TestRecordings_RealTmux(t *testing.T) {
	tiergate.Require(t, 2)
	t.Parallel()
	tmuxPath, err := exec.LookPath("tmux")
	if err != nil {
		t.Skipf("no tmux binary on PATH: %v", err)
	}
	for name, scenario := range scenarios {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			socket := tmuxkit.Socket(t, tmuxPath)
			recorder := tmuxrec.NewRecorder(socket, realTmux(tmuxPath, socket))
			session := tmuxrec.RecordSession(t, recorder, scenario.startupTimeoutS)
			endProcess := func(pid int) {
				if err := proc.KillPID(pid); err != nil {
					t.Fatalf("end the pane's process %d: %v", pid, err)
				}
			}

			got := scenario.run(t, &world{session: session, endProcess: endProcess, wait: func() { time.Sleep(paneEndPoll) }})

			rec, err := recorder.Recording(recordCommand, session, got)
			if err != nil {
				t.Fatalf("Recording() error = %v", err)
			}
			if *writeDir != "" {
				if err := tmuxrec.Save(tmuxrec.Path(*writeDir, name), rec); err != nil {
					t.Fatalf("Save() error = %v", err)
				}
				return
			}
			stored, err := tmuxrec.Load(tmuxrec.Path("testdata", name))
			if err != nil {
				t.Fatalf("Load() error = %v", err)
			}
			if diff := cmp.Diff(stored.Readings, rec.Readings); diff != "" {
				t.Errorf("readings over real tmux mismatch (-recorded +got):\n%s", diff)
			}
		})
	}
}
