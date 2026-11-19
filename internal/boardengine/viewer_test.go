// viewer_test.go unit-tests the embedded viewer page in viewer.go:
// that every render writes it verbatim and what it loads.

package boardengine_test

import (
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/boardengine"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// TestViewerPage pins that Render writes the embedded page byte for byte, whatever the board holds,
// and that the page loads its data through a script tag and names no outside address.
//
//testtiming:keep pins that every render writes the embedded page bytes and that the page loads its data by script tag with no outside URL
func TestViewerPage(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()
	page := string(boardengine.ViewerPage())

	empty, err := boardengine.Render(nil, "", viewOutputs, renderTime)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	changed, err := boardengine.Render(viewFixture(), viewPrimeOwner, viewOutputs, renderTime)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	for name, files := range map[string]map[string]string{"an empty board": empty, "a changed board": changed} {
		if files[boardengine.BoardPageFile] != page {
			t.Errorf("%s: %s differs from ViewerPage()", name, boardengine.BoardPageFile)
		}
	}

	if !strings.Contains(page, `<script src="`+boardengine.BoardDataFile+`">`) {
		t.Errorf("page holds no <script src> tag for %s", boardengine.BoardDataFile)
	}
	if strings.Contains(page, "http://") || strings.Contains(page, "https://") {
		t.Errorf("page names an http or https address; want it to load nothing beyond its own origin")
	}
}
