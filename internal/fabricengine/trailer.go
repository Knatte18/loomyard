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

// SnapshotTrailerKey is the git-trailer key fabric writes into a weft commit's
// message, once per caller-supplied snapshot tag, alongside the Warp-SHA
// trailer (see appendSnapshotTrailers).
const SnapshotTrailerKey = "Snapshot"

// snapshotTagPattern is the single-line token charset a snapshot tag must
// match: letters, digits, dot, underscore, hyphen. It deliberately excludes
// newline, carriage return, and colon — the trailer-injection vector a tag
// containing one of those characters would otherwise open (an attacker- or
// bug-supplied tag could inject a fake trailer line, or an unrelated one,
// into the commit message).
var snapshotTagPattern = regexp.MustCompile(`^[A-Za-z0-9._-]+$`)

// ErrInvalidSnapshotTag is a typed error returned by validateSnapshotTag,
// naming the offending tag so a caller (or an operator reading the error)
// knows exactly which supplied tag was rejected.
type ErrInvalidSnapshotTag struct {
	Tag string
}

// Error implements the error interface, naming the offending tag.
func (e *ErrInvalidSnapshotTag) Error() string {
	return fmt.Sprintf("fabricengine: invalid snapshot tag %q: must match %s", e.Tag, snapshotTagPattern.String())
}

// validateSnapshotTag returns an *ErrInvalidSnapshotTag when tag does not
// match snapshotTagPattern, and nil otherwise.
func validateSnapshotTag(tag string) error {
	if !snapshotTagPattern.MatchString(tag) {
		return &ErrInvalidSnapshotTag{Tag: tag}
	}
	return nil
}

// appendSnapshotTrailers returns message with one "Snapshot: <tag>" trailer
// line appended per entry in tags, in order. Every tag is validated via
// validateSnapshotTag BEFORE any line is appended, so a single invalid tag
// fails the whole call with nothing written — a caller must not end up
// committing a message with some, but not all, of its intended snapshot
// trailers. Reuses endsInTrailerBlock so the appended lines join an
// existing trailer block (e.g. the Warp-SHA trailer a caller has already
// appended) directly, rather than starting a new paragraph. An empty tags
// slice returns message unchanged with a nil error.
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
			// Only the FIRST appended line needs to decide whether to start a
			// new trailer-block paragraph; every subsequent line joins the
			// block this call itself just started.
			result += "\n\n" + trailerLine
			continue
		}
		result += "\n" + trailerLine
	}
	return result, nil
}

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
// after its last blank line) consists entirely of trailer-shaped lines. The
// FIRST paragraph is the commit subject and is never a trailer block, no
// matter how trailer-shaped its lines look: a one-line message like
// "builder: poll 01-json-flag done" matches the loose trailer pattern, and
// treating it as a trailer block would join the appended Warp-SHA line into
// the subject paragraph, polluting every such commit's `git log --oneline`
// subject (found live in the fabric-cutover review, round fable-r1).
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

	// The subject paragraph can never be a trailer block: a trailer block
	// only exists as a paragraph AFTER the subject (git's own convention --
	// interpret-trailers never treats a subject-only message as trailers).
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
