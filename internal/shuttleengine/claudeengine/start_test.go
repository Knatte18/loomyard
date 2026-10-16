// start_test.go drives shuttle's real start path with Claude's own box and session readings over a fake reed.

package claudeengine

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/reedengine"
	"github.com/Knatte18/loomyard/internal/segmentcolor"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
	"github.com/Knatte18/loomyard/internal/testkit/shuttlefake"
)

// virtualClock is a clock whose Sleep advances it without waiting.
type virtualClock struct{ now time.Time }

func (c *virtualClock) Now() time.Time        { return c.now }
func (c *virtualClock) Sleep(d time.Duration) { c.now = c.now.Add(d) }

// cannedLaunchEngine is Claude with a canned Prepare, so every box and session reading is Claude's own.
type cannedLaunchEngine struct {
	*Claude
}

func (cannedLaunchEngine) Prepare(string, shuttleengine.Spec, shuttleengine.Config) (shuttleengine.Launch, error) {
	return shuttleengine.Launch{Cmd: "claude", SessionID: "start-test", PromptLine: "Read the prompt."}, nil
}

func TestStart_LoneEOLMarkBoxTakesStartupSends(t *testing.T) {
	t.Parallel()

	capture, err := os.ReadFile(filepath.Join("testdata", "pane-idle-eol-mark.txt"))
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	worktree := t.TempDir()
	reed := &shuttlefake.Reed{
		CapturePaneFn: func(string) (string, error) { return string(capture), nil },
		// A capture with no escape sequences is its own styled capture.
		CapturePaneStyledFn: func(string) (string, error) { return string(capture), nil },
	}
	reed.AddStrandFn = func(spec reedengine.AddSpec) (reedengine.Strand, error) {
		strand := reedengine.Strand{GUID: "start-test-guid", Name: spec.Role, SessionID: spec.SessionID, Color: segmentcolor.Blue}
		reed.Strands = append(reed.Strands, reedengine.StrandStatus{GUID: strand.GUID, Name: strand.Name, Live: true})
		return strand, nil
	}

	claude := New()
	cfg := shuttleengine.Config{PollIntervalMS: 1000, LivenessEveryNPolls: 1, RunTimeoutMin: 60, StartupTimeoutS: 30, BackgroundShellWaitMin: 10}
	runner := shuttleengine.NewRunner(reed, shuttlefake.EngineSet(cannedLaunchEngine{claude}), worktree, worktree, cfg)
	runner.SetClock(&virtualClock{now: time.Date(2026, 10, 7, 12, 0, 0, 0, time.UTC)})

	spec := shuttleengine.Spec{
		Prompt:      "start",
		OutputFiles: []string{filepath.Join(worktree, "output.md")},
		Role:        "start-test",
		Timeout:     time.Hour,
		Skills:      []string{"scribe:prose"},
	}
	run, err := runner.StartGated(spec, nil)
	if err != nil || run == nil {
		t.Fatalf("StartGated = (%v, %v); want a run and a nil error", run, err)
	}

	pasted := make(map[string]bool)
	for _, call := range reed.PasteSubmitCalls {
		pasted[call.Text] = true
	}
	// No turn end reaches the events file, so the launch-carried load turn times out on the virtual clock.
	// The color and the prompt then go out without a typed load message.
	for _, sent := range []string{"/color blue", "Read the prompt."} {
		if !pasted[sent] {
			t.Errorf("the pane never received %q; pastes: %v", sent, reed.PasteSubmitCalls)
		}
	}
	if loadMessage := claude.SkillLoadMessage(spec.Skills); pasted[loadMessage] {
		t.Errorf("the pane received the load message %q; the launch line carries it", loadMessage)
	}
}
