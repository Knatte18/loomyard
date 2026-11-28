// viewer_test.go covers the viewer page of viewer.go as a render output.

package boardengine_test

import (
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/boardengine"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// TestViewerPage pins the page a render writes, and that it loads its data file and nothing from the network.
// The page's behaviour in a browser is checked by review, since the suite runs no JavaScript.
func TestViewerPage(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()
	out := boardengine.Outputs{Readme: "README.md", DesignPrefix: "design-"}
	page := string(boardengine.ViewerPage())
	for _, tasks := range [][]boardengine.Task{nil, viewFixture()} {
		files, err := boardengine.Render(tasks, "", out, renderTime)
		if err != nil {
			t.Fatalf("Render: %v", err)
		}
		if files[boardengine.BoardPageFile] != page {
			t.Errorf("board.html of %d entries differs from ViewerPage()", len(tasks))
		}
	}
	if !strings.Contains(page, `<script src="board-data.js"></script>`) {
		t.Errorf("page does not load board-data.js through a script tag")
	}
	for _, scheme := range []string{"http://", "https://"} {
		if strings.Contains(page, scheme) {
			t.Errorf("page holds a %s URL", scheme)
		}
	}
}
