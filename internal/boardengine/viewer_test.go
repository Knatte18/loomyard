// viewer_test.go checks that a render writes the embedded viewer page, which loads nothing from another origin.

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
	out := boardengine.Outputs{Readme: "Home.md", DesignPrefix: "proposal-"}
	page := string(boardengine.ViewerPage())

	for _, tasks := range [][]boardengine.Task{nil, viewFixture()} {
		files, err := boardengine.Render(tasks, "", out, renderTime)
		if err != nil {
			t.Fatalf("Render: %v", err)
		}
		if files[boardengine.BoardPageFile] != page {
			t.Errorf("Render of %d entries wrote a board.html that differs from ViewerPage()", len(tasks))
		}
	}
	if !strings.Contains(page, `<script src="board-data.js"></script>`) {
		t.Errorf("page does not load board-data.js through a script element")
	}
	for _, scheme := range []string{"http://", "https://"} {
		if strings.Contains(page, scheme) {
			t.Errorf("page holds a %s URL", scheme)
		}
	}
}
