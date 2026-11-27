// reinterpret_test.go pins the reinterpretation that drops a hub's edit_loop_edits.

package shuttleengine_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/configengine"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// TestDropEditLoopEdits pins what the rewrite does to a hub's shuttle.yaml.
// It drops edit_loop_edits when its value turns the watch off, a 0 or a null.
// It keeps every other key and names the way forward in the line it reports.
// Any other count stays in the file and is flagged with the way forward.
// A file without the key comes back unchanged, and one that does not parse or is no mapping is an invalid-config error.
func TestDropEditLoopEdits(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()
	tests := []struct {
		name     string
		existing string
		want     string
		// wantRewrote and wantFlagged are texts the one line of that list holds; the other list wants no line.
		// Both empty wants no line at all.
		wantRewrote string
		wantFlagged string
		wantErr     bool
	}{
		{name: "AZeroCountIsDropped", existing: "edit_loop_edits: 0\n", want: "{}\n", wantRewrote: "edit_loop_edits dropped"},
		{name: "ANullCountIsDropped", existing: "edit_loop_edits:\n", want: "{}\n", wantRewrote: "edit_loop_edits dropped"},
		{
			name:        "AZeroCountIsDroppedAndOtherKeysKept",
			existing:    "deny_python: false\nedit_loop_edits: 0\nedit_loop_window_s: 120\n",
			want:        "deny_python: false\nedit_loop_window_s: 120\n",
			wantRewrote: "edit_loop_edits dropped",
		},
		{
			name:        "ANonZeroCountIsKeptAndFlagged",
			existing:    "deny_python: false\nedit_loop_edits: 6\nedit_loop_window_s: 120\n",
			want:        "deny_python: false\nedit_loop_edits: 6\nedit_loop_window_s: 120\n",
			wantFlagged: "edit_loop_edits 6 kept",
		},
		{name: "AFileWithoutTheKeyIsUnchanged", existing: "deny_python: false\n", want: "deny_python: false\n"},
		{name: "AnEmptyFileIsUnchanged", existing: "", want: ""},
		{name: "AnUnparsableFileIsInvalid", existing: "edit_loop_edits: [\n", wantErr: true},
		{name: "ANonMappingDocumentIsInvalid", existing: "- a\n", wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, rewrote, flagged, err := shuttleengine.DropEditLoopEdits([]byte(tt.existing), "/root", "/root")
			if tt.wantErr {
				if !errors.Is(err, configengine.ErrInvalid) {
					t.Errorf("DropEditLoopEdits(%q) error = %v; want one marked configengine.ErrInvalid", tt.existing, err)
				}
				return
			}
			if err != nil {
				t.Fatalf("DropEditLoopEdits(%q) error: %v", tt.existing, err)
			}
			if string(got) != tt.want {
				t.Errorf("DropEditLoopEdits(%q) = %q; want %q", tt.existing, got, tt.want)
			}
			assertLines(t, tt.existing, "rewrote", rewrote, tt.wantRewrote)
			assertLines(t, tt.existing, "flagged", flagged, tt.wantFlagged)
		})
	}
}

// assertLines checks that lines holds nothing when want is empty.
// Otherwise it checks for one line that holds want and lym config shuttle.
func assertLines(t *testing.T, existing, list string, lines []string, want string) {
	t.Helper()
	if want == "" {
		if len(lines) != 0 {
			t.Errorf("DropEditLoopEdits(%q) %s = %q; want none", existing, list, lines)
		}
		return
	}
	if len(lines) != 1 || !strings.Contains(lines[0], want) || !strings.Contains(lines[0], "lym config shuttle") {
		t.Errorf("DropEditLoopEdits(%q) %s = %q; want one line naming %q and lym config shuttle", existing, list, lines, want)
	}
}
