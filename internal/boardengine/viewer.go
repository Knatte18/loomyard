// viewer.go embeds the board's viewer page, a render output that reads the board's view projection.

package boardengine

import (
	"bytes"
	_ "embed"
)

// BoardPageFile names the render output that holds the viewer page.
const BoardPageFile = "board.html"

// viewerPage is the self-contained viewer page, the same for every board.
//
//go:embed board.html
var viewerPage []byte

// ViewerPage returns a copy of the viewer page's bytes, for a reader that shows it without reading the board directory.
func ViewerPage() []byte {
	return bytes.Clone(viewerPage)
}
