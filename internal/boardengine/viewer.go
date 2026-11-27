// viewer.go embeds the board viewer page, a self-contained HTML file.

package boardengine

import _ "embed"

// BoardPageFile is the name of the rendered viewer page.
const BoardPageFile = "board.html"

//go:embed board.html
var viewerPage string

// ViewerPage returns a copy of the viewer page.
// Opened as a file it shows the view projection of the board-data.js beside it, read-only;
// served over HTTP it reads and writes through its server.
func ViewerPage() []byte {
	return []byte(viewerPage)
}
