// stencilstore.go implements the LF normalisation, body hashing, stamp parsing/writing, and the
// five-row edit-detection classification that the rest of the package builds on.
// Every hash and every comparison in this package routes through NormalizeLF and BodyHash,
// which is what makes the classification immune to a checkout's line-ending conversion.

package stencilstore

import (
	"crypto/sha256"
	"encoding/hex"
	"path/filepath"
	"strings"

	"github.com/Knatte18/loomyard/internal/stencil"
)

// stampKey is the "lyx-stencil: sha256=" marker text located inside a file's leading banner
// comment, independent of whether that banner is a single line or spans several.
const stampKey = "lyx-stencil: sha256="

// StampPrefix and StampSuffix bracket the stamp line written into a file's leading banner when that
// banner does not yet exist: the assembled line is StampPrefix + <hash> + StampSuffix.
const (
	StampPrefix = "<!-- " + stampKey
	StampSuffix = " -->"
)

// NormalizeLF replaces every CRLF and every remaining lone CR with LF.
// A checkout with core.autocrlf=true returns an LF-seeded file as CRLF, so every hash and every
// comparison in this package routes through NormalizeLF first to stay immune to that conversion.
func NormalizeLF(b []byte) []byte {
	s := strings.ReplaceAll(string(b), "\r\n", "\n")
	s = strings.ReplaceAll(s, "\r", "\n")
	return []byte(s)
}

// BodyHash returns the lowercase hex sha256 of content's body: the leading banner comment stripped
// via stencil.StripLeadingComment, then LF-normalised.
// The hash therefore covers exactly the text stencil.Fill parses, and never covers the stamp that
// stores it -- a hash cannot cover itself.
func BodyHash(content []byte) string {
	stripped := stencil.StripLeadingComment(string(content))
	sum := sha256.Sum256(NormalizeLF([]byte(stripped)))
	return hex.EncodeToString(sum[:])
}

// leadingBanner locates content's leading `<!-- ... -->` block, mirroring the trimming and
// first-"-->"-wins rule stencil.StripLeadingComment applies, and reports the byte range [start,end)
// it spans within content.
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
// ok is false when content has no leading banner, the banner carries no "lyx-stencil:" line, or the
// value found is not exactly 64 lowercase hex characters.
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

// ApplyStamp returns content with the stamp line present in its leading banner set to hash.
// When content already has a leading `<!-- ... -->` banner, an existing "lyx-stencil:" line is
// replaced in place, or the stamp line is inserted as the banner's last line when absent.
// When content has no leading banner, a new one-line banner is prepended, followed by a blank line.
// ApplyStamp never alters the body: BodyHash(ApplyStamp(c, h)) == BodyHash(c) always holds, because
// every byte it touches lies within the leading banner that BodyHash strips before hashing.
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

// Mode selects how Reconcile treats an untouched stencil (StateUntouched).
type Mode int

// ModeProduction refreshes an untouched stencil whose shipped default has changed; ModeDev seeds but
// never refreshes, warning instead.
// ModeProduction is the zero value, so an unstamped binary and a zero Mode both classify as
// production.
const (
	ModeProduction Mode = iota
	ModeDev
)

// ModeFor is the single mapping site from "is this a dev build" to a Mode: callers pass
// buildinfo.IsDev() into it, getting ModeDev when dev is true and ModeProduction otherwise.
// ModeProduction being iota's zero value is what makes an unstamped binary safely classify as
// production even before this function ever runs.
func ModeFor(dev bool) Mode {
	if dev {
		return ModeDev
	}
	return ModeProduction
}

// Registry supplies the name-to-shipped-default lookup Reconcile and Validate classify and seed
// against.
// It is passed in as an argument rather than imported, so this package never depends on its
// caller's registry package.
type Registry interface {
	// Names returns the registry's stencil names in a stable order.
	Names() []string
	// Default returns name's shipped default content, and whether name is a known stencil.
	Default(name string) ([]byte, bool)
}

// RelPath returns name's path relative to a stencils baseDir: `<family>/<name>.md`, where <family>
// is the substring of name up to its first "-".
// Returns `name + ".md"` with no family directory when name contains no "-".
func RelPath(name string) string {
	if idx := strings.Index(name, "-"); idx != -1 {
		return name[:idx] + "/" + name + ".md"
	}
	return name + ".md"
}

// Path returns name's absolute on-disk path under baseDir.
func Path(baseDir, name string) string {
	return filepath.Join(baseDir, filepath.FromSlash(RelPath(name)))
}

// State classifies an on-disk stencil file's relationship to its shipped default and its own stamp.
type State int

// The five states Classify can report, evaluated in the fixed order documented on Classify.
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

// Classify evaluates the five-row edit-detection table against a stencil's on-disk content and its
// shipped default, in this fixed order:
//  1. !exists -> StateAbsent.
//  2. a stamp is present and BodyHash(onDisk) equals it -> StateUntouched.
//  3. BodyHash(onDisk) equals BodyHash(shipped) -> StateReconciled, regardless of what the stamp
//     said -- the body has caught up with the shipped default and is treated as untouched from now
//     on.
//  4. a stamp is present and both comparisons above failed -> StateEdited.
//  5. the stamp is missing or unparseable -> StateEdited.
//
// Every ambiguous case resolves toward StateEdited, so Reconcile never overwrites an operator's
// work it cannot positively rule out as an edit.
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
