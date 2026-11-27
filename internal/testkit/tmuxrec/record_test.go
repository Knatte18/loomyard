// record_test.go covers how a recorded call's paths and process ids become placeholders.

package tmuxrec

import (
	"testing"

	"github.com/google/go-cmp/cmp"

	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

func TestPlaceholdCall_ReplacesBoundPathsAndFormatPIDs(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()
	const paneFormat = "#{pane_id} #{pane_dead} #{pane_top} #{pane_width} #{pane_height} #{pane_pid} #{pane_title}\t#{pane_current_command}\t#{pane_start_command}"
	bindings := Bindings{"{tmp}": "/tmp/run", "{hub}": "/tmp/run/001"}
	tests := []struct {
		name    string
		format  string
		out     string
		wantOut string
	}{
		{"APanePIDField", paneFormat, "%0 0 0 220 50 4711 host\tbash\tbash\n", "%0 0 0 220 50 {pid:1} host\tbash\tbash\n"},
		{"TheServerPIDBesideANumberThatIsNoPID", "#{pid} #{start_time}", "812 1795807440\n", "{pid:1} 1795807440\n"},
		{"APathUnderANestedBoundPath", "#{socket_path}", "/tmp/run/001/pr-session\n", "{hub}/pr-session\n"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			args := []string{"display-message", "-p", tt.format}
			pids := map[int]bool{}
			for _, pid := range formatPIDs(args, tt.out) {
				pids[pid] = true
			}

			got := placeholdCall(Call{Kind: KindTmux, Args: args, Out: tt.out}, bindings, pidPlaceholder(pids))

			if diff := cmp.Diff(tt.wantOut, got.Out); diff != "" {
				t.Errorf("placeholdCall() output mismatch (-want +got):\n%s", diff)
			}
		})
	}
}
