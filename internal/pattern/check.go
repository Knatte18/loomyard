// check.go declares the PATTERN format's limits and the Finding a format check reports.
// It also declares EntryRuleText with its line helpers.

package pattern

import (
	"regexp"
	"strings"
)

const (
	// overviewFileName is the overview's file name at the PATTERN root.
	overviewFileName = "PATTERN.md"

	// MaxOverviewBytes caps the overview's size.
	MaxOverviewBytes = 16 * 1024

	// HeadroomBytes is the line a branch's own growth of the overview must not cross, 5% below the cap.
	HeadroomBytes = MaxOverviewBytes * 95 / 100

	// MaxEntryTextChars caps the length in characters of one entry's rule text.
	// The rule text follows the backticked name and its separator,
	// and it drops any trailing background link and test marker.
	// The room a writer has for the wording is then the same whatever the entry's name and tail.
	MaxEntryTextChars = 280

	// entryBulletPrefix opens every entry line.
	entryBulletPrefix = "- "

	// KindOverviewHeadroom marks overview growth past the headroom line.
	KindOverviewHeadroom = "overview-headroom"
)

var (
	// entryHeadRe matches a bullet opening with a backticked name.
	entryHeadRe = regexp.MustCompile("^- `([^`]*)`")
	// entryLinkTailRe matches the background link closing an entry line, with its separator.
	entryLinkTailRe = regexp.MustCompile(` — \[background\]\([^)]*\)$`)
)

// entryTestMarker closes the rule text of an entry a test enforces.
const entryTestMarker = " (test)"

// Finding is one format violation.
// Subject is the entry name or file it concerns;
// Line is the overview line number, or 0 where none applies.
type Finding struct {
	Kind    string
	Subject string
	Line    int
	Message string
}

// continues reports whether line continues the entry bullet above it:
// a non-blank line that opens neither a new bullet nor a heading.
func continues(line string) bool {
	if strings.TrimSpace(line) == "" {
		return false
	}
	return !strings.HasPrefix(line, entryBulletPrefix) && !strings.HasPrefix(line, "#")
}

// EntryRuleText returns the rule text of an entry line:
// what follows the backticked name and its " — " separator.
// A trailing " — [background](...)" link and then a trailing " (test)" marker are cut off.
// A line that does not open with a backticked name keeps everything after its bullet.
func EntryRuleText(line string) string {
	text := strings.TrimPrefix(line, entryBulletPrefix)
	if m := entryHeadRe.FindString(line); m != "" {
		text = strings.TrimPrefix(strings.TrimPrefix(line, m), " — ")
	}
	text = entryLinkTailRe.ReplaceAllString(text, "")
	return strings.TrimSuffix(text, entryTestMarker)
}
