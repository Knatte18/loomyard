// bootwindowlog_test.go pins the Info line that names the first window of a fresh boot.
// It runs against the scripted fake tmux and needs no live server.

package reedengine

import (
	"errors"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/testkit/logcapture"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// TestLogBootWindow_NamesTheFirstWindowAndLogsNoInfoLineOnAFailedRead pins the boot-window Info line.
// A readable answer logs the window id, pane id and name.
// A failed or malformed read logs no Info line.
// It reads the logger's output through logcapture, which is process-global, so it does not call t.Parallel.
func TestLogBootWindow_NamesTheFirstWindowAndLogsNoInfoLineOnAFailedRead(t *testing.T) {
	tiergate.Require(t, 0)
	const infoLine = "reed: opened the session's first window"
	tests := []struct {
		name string
		out  string
		err  error
		want []string
	}{
		{"ThreeFieldsAreLogged", "@0 %1 bash\n", nil, []string{"window=@0", "pane=%1", "name=bash"}},
		{"ANameWithSpacesStaysOneField", "@0 %1 my shell\n", nil, []string{"window=@0", "pane=%1", `name="my shell"`}},
		{"AFailedReadLogsNoInfoLine", "@0 %1 bash\n", errors.New("display-message failed"), nil},
		{"ASingleTokenLogsNoInfoLine", "@0\n", nil, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			logs := logcapture.CaptureVerbose(t)
			e := newTestEngine(t)
			fake := installFakeTmux(t, e)
			fake.answerFormat(bootWindowFormat, tt.out, tt.err)

			e.logBootWindowLocked()

			got := logs.String()
			if tt.want == nil {
				if strings.Contains(got, infoLine) {
					t.Errorf("logBootWindowLocked() logged %q for answer %q, err %v; want no Info line\nlogs: %s", infoLine, tt.out, tt.err, got)
				}
				return
			}
			if !strings.Contains(got, infoLine) {
				t.Fatalf("logBootWindowLocked() logs lack %q\nlogs: %s", infoLine, got)
			}
			for _, field := range tt.want {
				if !strings.Contains(got, field) {
					t.Errorf("logBootWindowLocked() logs lack %s\nlogs: %s", field, got)
				}
			}
		})
	}
}
