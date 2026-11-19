// viewer.go embeds the board's viewer page, the static board.html every render writes beside the README.

package boardengine

import (
	_ "embed"
	"slices"
)

// BoardPageFile is the name of the render output that holds the viewer page.
const BoardPageFile = "board.html"

// viewerPage is the viewer page's bytes, embedded at build time.
//
//go:embed board.html
var viewerPage []byte

// ViewerPage returns a copy of the viewer page, the exact bytes every render writes as BoardPageFile.
func ViewerPage() []byte {
	return slices.Clone(viewerPage)
}
