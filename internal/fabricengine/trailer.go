// trailer.go holds the Warp-SHA git commit trailer, the record of which warp SHA a weft commit corresponds to.
// It holds fabric's helpers that format and parse that trailer in a commit message.
// The trailer is the sole source of truth for warp<->weft correspondence; corrindex.go builds a derived cache on it.
// It lives inside weft's own versioned commit history, so it cannot drift from the commit it describes.

package fabricengine

import (
	"fmt"
	"regexp"
	"strings"
)

// WarpSHATrailerKey is the git-trailer key fabric writes into every weft commit message as "Warp-SHA: <sha>".
const WarpSHATrailerKey = "Warp-SHA"

// SnapshotTrailerKey is the git-trailer key fabric writes into a weft commit message.
// It is written once per caller-supplied snapshot tag, alongside the Warp-SHA trailer.
const SnapshotTrailerKey = "Snapshot"

// snapshotTagPattern is the single-line charset a snapshot tag must match: letters, digits, dot, underscore, hyphen.
// It excludes newline, carriage return and colon.
// Those characters would let a tag inject a fake or unrelated trailer line into the commit message.
var snapshotTagPattern = regexp.MustCompile(`^[A-Za-z0-9._-]+$`)

// ErrInvalidSnapshotTag is the error for a snapshot tag outside the allowed charset.
// It names the rejected tag so the caller can report it.
type ErrInvalidSnapshotTag struct {
	Tag string
}

// Error implements the error interface, naming the offending tag.
func (e *ErrInvalidSnapshotTag) Error() string {
	return fmt.Sprintf("fabricengine: invalid snapshot tag %q: must match %s", e.Tag, snapshotTagPattern.String())
}

// validateSnapshotTag returns an *ErrInvalidSnapshotTag when tag is not a valid snapshot tag, and nil otherwise.
// A valid tag is non-empty and made only of letters, digits, dot, underscore and hyphen.
func validateSnapshotTag(tag string) error {
	if !snapshotTagPattern.MatchString(tag) {
		return &ErrInvalidSnapshotTag{Tag: tag}
	}
	return nil
}

// appendSnapshotTrailers returns message with one "Snapshot: <tag>" trailer line appended per tag, in order.
// It validates every tag before it appends any line, so an invalid tag fails the call with nothing written.
// A message therefore never carries only some of its snapshot trailers.
// Appended lines join an existing trailer block directly, rather than starting a new paragraph.
// An empty tags slice returns message unchanged with a nil error.
func appendSnapshotTrailers(message string, tags []string) (string, error) {
	if len(tags) == 0 {
		return message, nil
	}
	for _, tag := range tags {
		if err := validateSnapshotTag(tag); err != nil {
			return "", err
		}
	}

	result := strings.TrimRight(message, "\n")
	for i, tag := range tags {
		trailerLine := fmt.Sprintf("%s: %s", SnapshotTrailerKey, tag)
		if i == 0 && !endsInTrailerBlock(result) {
			// The first line decides whether to start a new trailer-block paragraph.
			// Every later line joins the block this call has just started.
			result += "\n\n" + trailerLine
			continue
		}
		result += "\n" + trailerLine
	}
	return result, nil
}

// trailerLinePattern matches one git-trailer-shaped line, a key, a colon and a value.
// It decides whether a message's final paragraph is already a trailer block.
// It follows git's loose trailer-line recognition.
var trailerLinePattern = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9-]*:\s+\S.*$`)

// appendWarpSHATrailer returns message with a "Warp-SHA: <warpSHA>" trailer line appended.
// It joins an existing trailer block directly.
// Otherwise it starts a new block after a blank line, as git's trailer conventions do.
func appendWarpSHATrailer(message, warpSHA string) string {
	trimmed := strings.TrimRight(message, "\n")
	trailerLine := fmt.Sprintf("%s: %s", WarpSHATrailerKey, warpSHA)

	if endsInTrailerBlock(trimmed) {
		return trimmed + "\n" + trailerLine
	}
	return trimmed + "\n\n" + trailerLine
}

// endsInTrailerBlock reports whether message's final paragraph consists entirely of trailer-shaped lines.
// That paragraph starts after the last blank line.
// A message with no blank line has no such paragraph, so it is never a trailer block.
func endsInTrailerBlock(message string) bool {
	if message == "" {
		return false
	}

	lines := strings.Split(message, "\n")

	lastParagraphStart := 0
	for i, line := range lines {
		if strings.TrimSpace(line) == "" {
			lastParagraphStart = i + 1
		}
	}

	if lastParagraphStart == 0 {
		return false
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

// TaskCommitOf returns the code-side commit a records commit message names in its trailer.
// The bool is false when the message carries no such trailer.
func TaskCommitOf(message string) (string, bool) {
	return parseWarpSHATrailer(message)
}

// parseWarpSHATrailer extracts the Warp-SHA trailer value from a full commit message.
// When multiple "Warp-SHA:" lines exist, the last one wins, as in git's trailer semantics.
// ok is false when the message carries no Warp-SHA trailer at all.
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
		sha, ok = value, true
	}
	return sha, ok
}
