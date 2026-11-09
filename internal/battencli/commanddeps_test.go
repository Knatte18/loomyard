// commanddeps_test.go pins the window title that commandDeps hands the Open-Window row:
// the task's console title read from the hub's shortname and board, with a part they do not give left out.

package battencli

import (
	"testing"

	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/testkit/boardkit"
	"github.com/Knatte18/loomyard/internal/testkit/locationkit"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// TestCommandDeps_TitleIsTheTasksConsoleTitle pins that Title reads the hub's shortname and board.
// It gives the task's console title.
// A part the hub or board does not give is left out of it.
func TestCommandDeps_TitleIsTheTasksConsoleTitle(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()
	tests := []struct {
		name string
		// shortname is the hub's recorded shortname; empty records none.
		shortname string
		// entries are the board entries the hub's board holds; none means the hub has no board.
		entries []string
		want    string
	}{
		{"TheTasksHandleOpensTheSlug", "", []string{`{"id":1,"slug":"run","kind":"task","labels":["bug"],"handle":"T4"}`}, "T4: run"},
		{"TheShortnameAndProjectHandleOpenTheTitle", "ly", []string{
			`{"id":1,"slug":"proj","kind":"project","labels":["bug"],"handle":"P3"}`,
			`{"id":2,"slug":"run","kind":"task","labels":["bug"],"project":"proj","handle":"T4"}`,
		}, "ly: P3: T4: run"},
		{"AnEntryOfAnotherSlugLeavesOutTheHandles", "", []string{`{"id":1,"slug":"other","kind":"task","labels":["bug"],"handle":"T4"}`}, "run"},
		{"NoBoardLeavesOutTheHandles", "", nil, "run"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			location := locationkit.Location(t.TempDir(), "main", "")
			if len(tt.entries) > 0 {
				boardkit.WriteBoard(t, fabricengine.BoardDir(location.HubPath), tt.entries...)
			}
			if tt.shortname != "" {
				if err := fabricengine.WriteShortname(fabricengine.BoardDir(location.HubPath), tt.shortname); err != nil {
					t.Fatalf("record the shortname: %v", err)
				}
			}
			c := &battenCLI{}

			deps := c.commandDeps(location, "run", "lym", nil)

			if deps.Title == nil {
				t.Fatal("commandDeps().Title = nil; want a title func")
			}
			if got := deps.Title(); got != tt.want {
				t.Errorf("commandDeps().Title() = %q; want %q", got, tt.want)
			}
		})
	}
}
