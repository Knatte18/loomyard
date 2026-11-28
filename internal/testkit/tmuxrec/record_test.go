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

// A paste's text holds the hub directory as a placeholder, which a replay binds again.
// It keeps its numbers and every other bound value as the code sent them.
func TestPlaceholdCall_HoldsAPastesBufferNameAndHubAndKeepsTheRestOfItsText(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()
	bindings := Bindings{"{hub}": "/tmp/run/001", "{tmp}": "/tmp", "{user}": "operator"}
	// The buffer name holds a run of digits that is also a pid the call named.
	pids := map[int]bool{4711: true}
	args := func(buffer string) []string {
		return []string{"load-buffer", "-b", buffer, "-", ";", "paste-buffer", "-p", "-d", "-b", buffer, "-t", "%2"}
	}
	call := Call{Kind: KindTmux, Args: args("lym-inject-4711abcd"), Stdin: "see /tmp/run/001/notes, line 4711, /tmp/x, operator"}

	got := placeholdCall(call, bindings, pidPlaceholder(pids))

	want := Call{Kind: KindTmux, Args: args("{inject-buffer}"), Stdin: "see {hub}/notes, line 4711, /tmp/x, operator"}
	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("placeholdCall() mismatch (-want +got):\n%s", diff)
	}

	replay := NewReplay("held", Recording{Calls: []Call{got}}, Bindings{"{hub}": "/r/h"})
	sent := []byte("see /r/h/notes, line 4711, /tmp/x, operator")
	if _, err := replay.Outside(nil).Tmux(false, sent, args("lym-inject-0f0f0f0f")...); err != nil {
		t.Errorf("Tmux() of the text under the replay's hub error = %v; want the held paste to answer it", err)
	}
}
