// viewer.go embeds the viewer page, a self-contained HTML file that shows the view projection.

package boardengine

import (
	_ "embed"
	"slices"
)

// BoardPageFile is the rendered file that holds the viewer page.
const BoardPageFile = "board.html"

//go:embed board.html
var viewerPage []byte

// ViewerPage returns a copy of the viewer page's bytes.
// The page is fixed and holds no board data.
// It loads the data from the rendered projection or from the server serving the page.
func ViewerPage() []byte {
	return slices.Clone(viewerPage)
}
