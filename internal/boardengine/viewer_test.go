// viewer_test.go tests the viewer page as a render output.

package boardengine_test

import (
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/boardengine"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

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
	if !strings.Contains(page, `<script src="board-data.js">`) {
		t.Errorf("the page does not load board-data.js through a script tag")
	}
	for _, remote := range []string{"http://", "https://"} {
		if strings.Contains(page, remote) {
			t.Errorf("the page holds %q", remote)
		}
	}
}
