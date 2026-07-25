// trailer.go — the Warp-SHA git commit trailer: fabric's format and parse
// helpers for recording, inside a weft commit's own message, which warp SHA
// that weft commit corresponds to. The trailer is the sole source of truth for
// warp<->weft correspondence (see corrindex.go, the derived-cache layer built
// on top of it); it lives inside weft's own versioned commit history, so it can
// never drift out of sync with the commit it describes.

package fabricengine

import (
	"fmt"
	"regexp"
	"strings"
)

// WarpSHATrailerKey is the git-trailer key fabric writes into every weft
// commit's message, in the same convention as git's own "Co-authored-by:"
// trailer: "Warp-SHA: <sha>".
const WarpSHATrailerKey = "Warp-SHA"

// trailerLinePattern matches a single git-trailer-shaped line: a token key
// (letters, digits, hyphens) followed by a colon, at least one space, and a
// non-empty value. It is used to decide whether a commit message's final
// paragraph is already a trailer block (see endsInTrailerBlock), mirroring
// git's own loose trailer-line recognition.
var trailerLinePattern = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9-]*:\s+\S.*$`)

// appendWarpSHATrailer returns message with a "Warp-SHA: <warpSHA>" trailer
// line appended, following git's trailer conventions (the same shape as a
// "Co-authored-by:" trailer): the trailer line joins message's existing
// trailer block directly if the message already ends in one (its final
// paragraph is entirely trailer-shaped lines), or starts a new trailer block
// separated from the body by a blank line otherwise.
func appendWarpSHATrailer(message, warpSHA string) string {
	trimmed := strings.TrimRight(message, "\n")
	trailerLine := fmt.Sprintf("%s: %s", WarpSHATrailerKey, warpSHA)

	if endsInTrailerBlock(trimmed) {
		// The message's last paragraph is already trailer-shaped lines
		// (e.g. a prior "Co-authored-by:"); join the new trailer directly so
		// the whole trailer block stays one paragraph, per git convention.
		return trimmed + "\n" + trailerLine
	}
	return trimmed + "\n\n" + trailerLine
}

// endsInTrailerBlock reports whether message's final paragraph (the lines
// after its last blank line, or the whole message if it has none) consists
// entirely of trailer-shaped lines. An empty message has no trailer block.
func endsInTrailerBlock(message string) bool {
	if message == "" {
		return false
	}

	lines := strings.Split(message, "\n")

	// Find the start of the final paragraph: one past the last blank line.
	lastParagraphStart := 0
	for i, line := range lines {
		if strings.TrimSpace(line) == "" {
			lastParagraphStart = i + 1
		}
	}
	lastParagraph := lines[lastParagraphStart:]
	if len(lastParagraph) == 0 {
		return false
	}

	for _, line := range lastParagraph {
		if strings.TrimSpace(line) == "" {
			continue
		}
		if !trailerLinePattern.MatchString(line) {
			return false
		}
	}
	return true
}

// parseWarpSHATrailer extracts the Warp-SHA trailer value from a full commit
// message. When the message carries multiple "Warp-SHA:" lines (which should
// not normally happen, but is tolerated rather than rejected), the last one
// wins, matching git's own trailer semantics. Surrounding whitespace around
// the key and value is tolerated. Reports ok=false when the message carries no
// Warp-SHA trailer at all.
func parseWarpSHATrailer(message string) (sha string, ok bool) {
	prefix := WarpSHATrailerKey + ":"
	for _, line := range strings.Split(message, "\n") {
		trimmed := strings.TrimSpace(line)
		if !strings.HasPrefix(trimmed, prefix) {
			continue
		}
		value := strings.TrimSpace(strings.TrimPrefix(trimmed, prefix))
		if value == "" {
			continue
		}
		// Overwrite on every match so the last occurrence wins.
		sha, ok = value, true
	}
	return sha, ok
}
