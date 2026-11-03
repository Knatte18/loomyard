// commanddeps_test.go pins the window title commandDeps hands the Open-Window row: the slug opened with the task's board handle.

package battencli

import (
	"testing"

	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/testkit/boardkit"
	"github.com/Knatte18/loomyard/internal/testkit/locationkit"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// TestCommandDeps_TitleOpensTheSlugWithTheTasksHandle pins that Title reads the hub's board for the task's handle, and falls back to the bare slug when the board names none.
func TestCommandDeps_TitleOpensTheSlugWithTheTasksHandle(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()
	tests := []struct {
		name string
		// entry is the one board entry the hub's board holds; empty means the hub has no board.
		entry string
		want  string
	}{
		{"TheTasksHandleOpensTheSlug", `{"id":1,"slug":"run","kind":"task","labels":["bug"],"handle":"T4"}`, "T4: run"},
		{"AnEntryOfAnotherSlugKeepsTheSlug", `{"id":1,"slug":"other","kind":"task","labels":["bug"],"handle":"T4"}`, "run"},
		{"NoBoardKeepsTheSlug", "", "run"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			location := locationkit.Location(t.TempDir(), "main", "")
			if tt.entry != "" {
				boardkit.WriteBoard(t, fabricengine.BoardDir(location.HubPath), tt.entry)
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
