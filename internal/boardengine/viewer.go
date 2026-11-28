// viewer.go embeds the viewer page, the static HTML that shows a board and sends its writes.

package boardengine

import (
	_ "embed"
	"slices"
)

// BoardPageFile is the name of the board file that holds the viewer page.
const BoardPageFile = "board.html"

// viewerPage is the viewer page, one self-contained HTML file with one inline style and one inline script.
//
//go:embed board.html
var viewerPage []byte

// ViewerPage returns a copy of the viewer page, which is the same for every board.
func ViewerPage() []byte {
	return slices.Clone(viewerPage)
}
