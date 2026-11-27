// viewer_test.go covers the viewer page as a render output.

package boardengine_test

import (
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/boardengine"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// TestViewerPage pins that every render writes the fixed viewer page, and what the page loads.
func TestViewerPage(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()
	out := boardengine.Outputs{Readme: "README.md", DesignPrefix: "design-", Types: readmeTypes}

	page := string(boardengine.ViewerPage())
	for _, tasks := range [][]boardengine.Task{viewFixture(), viewFixture()[:1]} {
		files, err := boardengine.Render(tasks, "", out, renderTime)
		if err != nil {
			t.Fatalf("Render: %v", err)
		}
		if files[boardengine.BoardPageFile] != page {
			t.Errorf("board.html of a render of %d entries differs from ViewerPage()", len(tasks))
		}
	}
	if !strings.Contains(page, `<script src="board-data.js"></script>`) {
		t.Error("the page does not load board-data.js through a script element")
	}
	for _, remote := range []string{"http://", "https://"} {
		if strings.Contains(page, remote) {
			t.Errorf("the page holds %q; want nothing loaded beyond its own origin", remote)
		}
	}
}
