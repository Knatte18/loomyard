// check.go holds the pure PATTERN format checker: it reads a PATTERN root through an fs.FS,
// writes nothing, and returns findings.
// Nothing in the prompt-render path calls it.

package pattern

import (
	"fmt"
	"io/fs"
	"path"
	"regexp"
	"sort"
	"strings"
)

const (
	// overviewFileName is the overview's file name at the PATTERN root.
	overviewFileName = "PATTERN.md"

	// MaxOverviewBytes caps the overview's size.
	MaxOverviewBytes = 16 * 1024

	// MaxEntryLineChars caps one entry line's length in characters.
	MaxEntryLineChars = 320

	// entryBulletPrefix opens every entry line.
	entryBulletPrefix = "- "

	// entryNamePattern is the grammar of an entry name.
	entryNamePattern = `PATTERN-[a-z0-9-]+`
)

// Finding kinds, the stable identifiers of what Check reports.
const (
	KindAbsentOverview   = "absent-overview"
	KindEmptyOverview    = "empty-overview"
	KindOversizeOverview = "oversize-overview"
	KindNoEntries        = "no-entries"
	KindBadName          = "bad-name"
	KindDuplicateName    = "duplicate-name"
	KindContinuedEntry   = "continued-entry"
	KindLongLine         = "long-line"
	KindUnresolvedLink   = "unresolved-link"
	KindWrongLink        = "wrong-link"
	KindOrphanBackground = "orphan-background"
)

var (
	entryNameRe = regexp.MustCompile("^" + entryNamePattern + "$")
	// entryHeadRe matches a bullet opening with a backticked name.
	entryHeadRe = regexp.MustCompile("^- `([^`]*)`")
	// backgroundLinkRe matches the `[background](target)` link of an entry line.
	backgroundLinkRe = regexp.MustCompile(`\[background\]\(([^)]*)\)`)
)

// Finding is one format violation.
// Subject is the entry name or file it concerns; Line is the overview line number, or 0 where none applies.
type Finding struct {
	Kind    string
	Subject string
	Line    int
	Message string
}

// Check validates the PATTERN under root, the directory holding PATTERN.md and pattern/.
// It reads only through root and returns every finding, in overview order and then file order.
func Check(root fs.FS) []Finding {
	data, err := fs.ReadFile(root, overviewFileName)
	if err != nil {
		return []Finding{{Kind: KindAbsentOverview, Subject: overviewFileName, Message: fmt.Sprintf("cannot read %s: %v", overviewFileName, err)}}
	}

	var findings []Finding
	add := func(kind, subject string, line int, format string, args ...any) {
		findings = append(findings, Finding{Kind: kind, Subject: subject, Line: line, Message: fmt.Sprintf(format, args...)})
	}

	if len(data) > MaxOverviewBytes {
		add(KindOversizeOverview, overviewFileName, 0, "overview is %d bytes, over the %d cap", len(data), MaxOverviewBytes)
	}
	if strings.TrimSpace(string(data)) == "" {
		add(KindEmptyOverview, overviewFileName, 0, "overview is empty or whitespace-only")
		return append(findings, orphans(root, nil)...)
	}

	lines := strings.Split(string(data), "\n")
	seen := map[string]bool{}
	named := map[string]bool{}
	entries := 0
	for i, line := range lines {
		lineNo := i + 1
		if !strings.HasPrefix(line, entryBulletPrefix) {
			continue
		}
		entries++
		name := ""
		if m := entryHeadRe.FindStringSubmatch(line); m != nil {
			name = m[1]
		}
		subject := name
		if subject == "" {
			subject = fmt.Sprintf("line %d", lineNo)
		}
		if !entryNameRe.MatchString(name) {
			add(KindBadName, subject, lineNo, "entry name does not match %s", entryNamePattern)
		} else if seen[name] {
			add(KindDuplicateName, name, lineNo, "entry name is used more than once")
		}
		seen[name] = true

		if n := len([]rune(line)); n > MaxEntryLineChars {
			add(KindLongLine, subject, lineNo, "entry line is %d characters, over the %d cap", n, MaxEntryLineChars)
		}
		if i+1 < len(lines) && continues(lines[i+1]) {
			add(KindContinuedEntry, subject, lineNo, "entry continues onto the next line")
		}

		if m := backgroundLinkRe.FindStringSubmatch(line); m != nil {
			target := m[1]
			want := "pattern/" + name + ".md"
			if _, err := fs.Stat(root, target); err != nil {
				add(KindUnresolvedLink, subject, lineNo, "background link %q does not resolve", target)
			} else if target != want {
				add(KindWrongLink, subject, lineNo, "background link %q is not %q", target, want)
			}
			named[path.Base(target)] = true
		}
	}
	if entries == 0 {
		add(KindNoEntries, overviewFileName, 0, "overview has no entry bullet")
	}
	return append(findings, orphans(root, named)...)
}

// continues reports whether line continues the entry bullet above it:
// a non-blank line that opens neither a new bullet nor a heading.
func continues(line string) bool {
	if strings.TrimSpace(line) == "" {
		return false
	}
	return !strings.HasPrefix(line, entryBulletPrefix) && !strings.HasPrefix(line, "#")
}

// orphans reports every file under pattern/ that no entry's background link names.
func orphans(root fs.FS, named map[string]bool) []Finding {
	dirEntries, err := fs.ReadDir(root, "pattern")
	if err != nil {
		return nil
	}
	var out []Finding
	for _, e := range dirEntries {
		if e.IsDir() || named[e.Name()] {
			continue
		}
		out = append(out, Finding{Kind: KindOrphanBackground, Subject: "pattern/" + e.Name(), Message: "no entry names this background file"})
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Subject < out[j].Subject })
	return out
}
