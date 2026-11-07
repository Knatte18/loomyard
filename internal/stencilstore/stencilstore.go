// stencilstore.go implements LF normalisation, body hashing, and stamp parsing and writing.
// It also holds the five-row edit-detection classification.
// Every hash and comparison here goes through NormalizeLF and BodyHash, so line endings cannot change the result.

package stencilstore

import (
	"crypto/sha256"
	"encoding/hex"
	"path/filepath"
	"strings"
	"time"

	"github.com/Knatte18/loomyard/internal/stencil"
)

// stampKey is the "lym-stencil: sha256=" marker, located inside a file's leading banner of one line or several.
const stampKey = "lym-stencil: sha256="

// StampPrefix and StampSuffix bracket the stamp line of a file's leading banner, when that banner is created.
// The assembled line is StampPrefix, then the hash, then StampSuffix.
const (
	StampPrefix = "<!-- " + stampKey
	StampSuffix = " -->"
)

// NormalizeLF replaces every CRLF and every remaining lone CR with LF.
// A checkout with core.autocrlf=true returns an LF-seeded file as CRLF,
// so every hash and comparison in the package normalises first to stay immune to that conversion.
func NormalizeLF(b []byte) []byte {
	s := strings.ReplaceAll(string(b), "\r\n", "\n")
	s = strings.ReplaceAll(s, "\r", "\n")
	return []byte(s)
}

// BodyHash returns the lowercase hex sha256 of content's body.
// The body is content with its leading banner removed and its line endings normalised to LF.
// It covers exactly the text the stencil parser reads.
// It never covers the stamp that stores the hash, since a hash cannot cover itself.
func BodyHash(content []byte) string {
	stripped := stencil.StripLeadingComment(string(content))
	sum := sha256.Sum256(NormalizeLF([]byte(stripped)))
	return hex.EncodeToString(sum[:])
}

// leadingBanner locates content's leading `<!-- ... -->` block and reports the byte range [start,end) it spans.
// The block starts after any leading whitespace and ends at the first "-->".
// ok is false when content has no leading banner.
func leadingBanner(content []byte) (banner []byte, start, end int, ok bool) {
	s := string(content)
	trimmed := strings.TrimLeft(s, " \t\r\n")
	if !strings.HasPrefix(trimmed, "<!--") {
		return nil, 0, 0, false
	}
	leadWS := len(s) - len(trimmed)
	closeIdx := strings.Index(trimmed, "-->")
	if closeIdx == -1 {
		return nil, 0, 0, false
	}
	end = leadWS + closeIdx + len("-->")
	return []byte(s[leadWS:end]), leadWS, end, true
}

// isLowerHexDigit reports whether c is one of the 16 lowercase hex digit characters.
func isLowerHexDigit(c byte) bool {
	return (c >= '0' && c <= '9') || (c >= 'a' && c <= 'f')
}

// ParseStamp extracts the sha256=<hex> value from content's leading `<!-- ... -->` banner.
// ok is false when content has no leading banner or the banner has no "lym-stencil:" line.
// It is also false when the value is not exactly 64 lowercase hex characters.
func ParseStamp(content []byte) (hash string, ok bool) {
	banner, _, _, hasBanner := leadingBanner(content)
	if !hasBanner {
		return "", false
	}

	bannerStr := string(banner)
	idx := strings.Index(bannerStr, stampKey)
	if idx == -1 {
		return "", false
	}

	rest := bannerStr[idx+len(stampKey):]
	end := 0
	for end < len(rest) && isLowerHexDigit(rest[end]) {
		end++
	}
	if end != sha256.Size*2 {
		return "", false
	}
	return rest[:end], true
}

// ApplyStamp returns content with the stamp line in its leading banner set to hash.
// An existing "lym-stencil:" line in the banner is replaced in place;
// when the banner has none, the stamp line is inserted as its last line.
// When content has no leading banner, a one-line banner is prepended, followed by a blank line.
// ApplyStamp never alters the body: BodyHash(ApplyStamp(c, h)) equals BodyHash(c) always.
// Every byte it touches lies inside the leading banner, which the body hash strips.
func ApplyStamp(content []byte, hash string) []byte {
	banner, start, end, ok := leadingBanner(content)
	if !ok {
		newBanner := StampPrefix + hash + StampSuffix + "\n\n"
		result := make([]byte, 0, len(newBanner)+len(content))
		result = append(result, newBanner...)
		result = append(result, content...)
		return result
	}

	bannerStr := string(banner)
	var newBanner string
	if idx := strings.Index(bannerStr, stampKey); idx != -1 {
		rest := bannerStr[idx+len(stampKey):]
		hexEnd := 0
		for hexEnd < len(rest) && isLowerHexDigit(rest[hexEnd]) {
			hexEnd++
		}
		newBanner = bannerStr[:idx+len(stampKey)] + hash + rest[hexEnd:]
	} else {
		head := strings.TrimRight(bannerStr[:len(bannerStr)-len("-->")], " \t\n")
		newBanner = head + "\n" + stampKey + hash + StampSuffix
	}

	result := make([]byte, 0, len(newBanner)+len(content)-len(banner))
	result = append(result, content[:start]...)
	result = append(result, newBanner...)
	result = append(result, content[end:]...)
	return result
}

// Mode selects whether an untouched stencil is refreshed from its shipped default or left alone.
type Mode int

// ModeUnstamped and ModeDev seed an absent stencil but warn instead of refreshing an untouched one.
// ModeProduction refreshes an untouched stencil whose shipped default is newer than the recorded writer.
// ModeUnstamped is the zero value, so an unset Mode never refreshes.
const (
	ModeUnstamped  Mode = 0
	ModeDev        Mode = 1
	ModeProduction Mode = 2
)

// ModeFor is the single three-way mapping from a build's channel and VCS stamp to a Mode:
// ModeProduction only when production and cleanStamp both hold, else ModeDev when dev, else ModeUnstamped.
// Callers pass buildinfo.IsDev(), buildinfo.IsProduction() and buildvcs.Running().Clean().
func ModeFor(dev, production, cleanStamp bool) Mode {
	switch {
	case production && cleanStamp:
		return ModeProduction
	case dev:
		return ModeDev
	default:
		return ModeUnstamped
	}
}

// Writer names the binary that wrote a board copy: its VCS revision and commit time.
// The zero Writer means no recorded writer.
type Writer struct {
	Revision string
	Time     time.Time
}

// writerKeyBuild and writerKeyTime prefix the two writer tokens that follow the hex in a banner's stamp line.
const (
	writerKeyBuild = "build="
	writerKeyTime  = "time="
)

// stampTail locates the writer tokens that follow the stamp's hex in content's leading banner:
// the byte range [hexEnd, tailEnd) runs from the hex's end to the end of the stamp line or to the banner's close.
// closing reports that the range ends at the banner's close, so a rebuilt tail must keep a space before it.
// ok is false when content has no leading banner or the banner carries no stamp line.
func stampTail(content []byte) (hexEnd, tailEnd int, closing, ok bool) {
	banner, start, end, hasBanner := leadingBanner(content)
	if !hasBanner {
		return 0, 0, false, false
	}
	idx := strings.Index(string(banner), stampKey)
	if idx == -1 {
		return 0, 0, false, false
	}
	hexEnd = start + idx + len(stampKey)
	for hexEnd < end && isLowerHexDigit(content[hexEnd]) {
		hexEnd++
	}
	closeStart := end - len("-->")
	tailEnd = hexEnd
	for tailEnd < closeStart && content[tailEnd] != '\n' && content[tailEnd] != '\r' {
		tailEnd++
	}
	return hexEnd, tailEnd, tailEnd == closeStart, true
}

// ApplyWriter returns content with the writer keys after the hex in its stamp line set to writer:
// existing `build=` and `time=` keys are rewritten, and a zero writer removes them.
// The body is never touched, and content without a stamp line comes back unchanged.
func ApplyWriter(content []byte, writer Writer) []byte {
	hexEnd, tailEnd, closing, ok := stampTail(content)
	if !ok {
		return content
	}

	var tail strings.Builder
	for _, token := range strings.Fields(string(content[hexEnd:tailEnd])) {
		if !strings.HasPrefix(token, writerKeyBuild) && !strings.HasPrefix(token, writerKeyTime) {
			tail.WriteString(" " + token)
		}
	}
	if writer.Revision != "" {
		tail.WriteString(" " + writerKeyBuild + writer.Revision)
	}
	if !writer.Time.IsZero() {
		tail.WriteString(" " + writerKeyTime + writer.Time.UTC().Format(time.RFC3339))
	}
	if closing {
		tail.WriteString(" ")
	}

	result := make([]byte, 0, len(content)+tail.Len())
	result = append(result, content[:hexEnd]...)
	result = append(result, tail.String()...)
	result = append(result, content[tailEnd:]...)
	return result
}

// ParseWriter reads the writer keys from content's stamp line, and returns the zero Writer when they are absent.
// A time that is not RFC 3339 leaves Time zero.
func ParseWriter(content []byte) Writer {
	hexEnd, tailEnd, _, ok := stampTail(content)
	if !ok {
		return Writer{}
	}

	var writer Writer
	for _, token := range strings.Fields(string(content[hexEnd:tailEnd])) {
		switch {
		case strings.HasPrefix(token, writerKeyBuild):
			writer.Revision = strings.TrimPrefix(token, writerKeyBuild)
		case strings.HasPrefix(token, writerKeyTime):
			if parsed, err := time.Parse(time.RFC3339, strings.TrimPrefix(token, writerKeyTime)); err == nil {
				writer.Time = parsed
			}
		}
	}
	return writer
}

// Registry supplies each stencil name and its shipped default content.
// It is passed in as an argument rather than imported, so this package never depends on its caller's registry package.
type Registry interface {
	// Names returns the registry's stencil names in a stable order.
	Names() []string
	// Default returns name's shipped default content, and whether name is a known stencil.
	Default(name string) ([]byte, bool)
}

// RelPath returns name's path relative to a stencils baseDir: `<family>/<name>.md`.
// The family is the part of name before its first "-".
// A name with no "-" has no family directory and returns as `name + ".md"`.
// A name containing "/" is already a path and returns as `name + ".md"`, the form engine block names take.
func RelPath(name string) string {
	if strings.Contains(name, "/") {
		return name + ".md"
	}
	if idx := strings.Index(name, "-"); idx != -1 {
		return name[:idx] + "/" + name + ".md"
	}
	return name + ".md"
}

// Path returns name's on-disk path under baseDir, with the platform's path separators.
func Path(baseDir, name string) string {
	return filepath.Join(baseDir, filepath.FromSlash(RelPath(name)))
}

// State classifies an on-disk stencil file's relationship to its shipped default and its own stamp.
type State int

// The states a stencil file can be classified into.
const (
	StateAbsent State = iota
	StateUntouched
	StateReconciled
	StateEdited
)

// String implements fmt.Stringer, naming each State for logging and test failure messages.
func (s State) String() string {
	switch s {
	case StateAbsent:
		return "StateAbsent"
	case StateUntouched:
		return "StateUntouched"
	case StateReconciled:
		return "StateReconciled"
	case StateEdited:
		return "StateEdited"
	default:
		return "StateUnknown"
	}
}

// Classify reports a stencil file's state from its on-disk content, existence, and shipped default.
// It evaluates these cases in order:
//  1. !exists -> StateAbsent.
//  2. a stamp is present and BodyHash(onDisk) equals it -> StateUntouched.
//  3. BodyHash(onDisk) equals BodyHash(shipped) -> StateReconciled, regardless of what the stamp says;
//     the body has caught up with the shipped default and is treated as untouched from then on.
//  4. a stamp is present and both comparisons above failed -> StateEdited.
//  5. the stamp is missing or unparseable -> StateEdited.
//
// Every ambiguous case resolves toward StateEdited, so an operator's work is never overwritten on a guess.
func Classify(onDisk []byte, exists bool, shipped []byte) State {
	if !exists {
		return StateAbsent
	}

	stamp, stamped := ParseStamp(onDisk)
	if stamped && BodyHash(onDisk) == stamp {
		return StateUntouched
	}
	if BodyHash(onDisk) == BodyHash(shipped) {
		return StateReconciled
	}
	return StateEdited
}
