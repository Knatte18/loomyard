// check_test.go holds Check, the pure PATTERN format checker only tests run, and covers it:
// one case per finding kind plus the clean overviews.

package pattern

import (
	"fmt"
	"io/fs"
	"path"
	"regexp"
	"sort"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// entryNamePattern is the grammar of an entry name.
const entryNamePattern = `PATTERN-[a-z0-9-]+`

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
	// backgroundLinkRe matches the `[background](target)` link of an entry line.
	backgroundLinkRe = regexp.MustCompile(`\[background\]\(([^)]*)\)`)
)

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
		add(KindOversizeOverview, overviewFileName, 0, "overview is %d bytes, %d over the %d cap", len(data), len(data)-MaxOverviewBytes, MaxOverviewBytes)
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

		if n := len([]rune(EntryRuleText(line))); n > MaxEntryTextChars {
			add(KindLongLine, subject, lineNo, "entry rule text is %d characters, over the %d cap", n, MaxEntryTextChars)
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

func entryLine(name, link string) string {
	line := fmt.Sprintf("- `%s` — Doing a thing: the rule.", name)
	if link != "" {
		line += fmt.Sprintf(" — [background](%s)", link)
	}
	return line
}

func overview(lines ...string) *fstest.MapFile {
	return &fstest.MapFile{Data: []byte("# PATTERN\n\n## Topic\n\n" + strings.Join(lines, "\n") + "\n")}
}

func TestCheck(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()
	bg := &fstest.MapFile{Data: []byte("background\n")}
	// A long name, "(test)" and a link put this line past the old 360-character whole-line cap while its rule text sits at the cap.
	longName := "PATTERN-" + strings.Repeat("n", 60)
	textAtCap := "- `" + longName + "` — " + strings.Repeat("x", MaxEntryTextChars) + " (test) — [background](pattern/" + longName + ".md)"
	textOverCap := "- `PATTERN-long` — " + strings.Repeat("x", MaxEntryTextChars+1)

	tests := []struct {
		name string
		fsys fstest.MapFS
		want []string // "kind:subject"
		// wantLine is the line of the one expected finding; zero leaves it unasserted.
		wantLine int
		// wantMessage is the message of the one expected finding; empty leaves it unasserted.
		wantMessage string
	}{
		{
			name: "bad name",
			fsys: fstest.MapFS{"PATTERN.md": overview(entryLine("PATTERN-Bad_Name", ""))},
			want: []string{KindBadName + ":PATTERN-Bad_Name"},
		},
		{
			name:     "duplicate name",
			fsys:     fstest.MapFS{"PATTERN.md": overview(entryLine("PATTERN-a", ""), entryLine("PATTERN-a", ""))},
			want:     []string{KindDuplicateName + ":PATTERN-a"},
			wantLine: 6,
		},
		{
			name: "two-line entry",
			fsys: fstest.MapFS{"PATTERN.md": overview(entryLine("PATTERN-a", ""), "  and it goes on")},
			want: []string{KindContinuedEntry + ":PATTERN-a"},
		},
		{
			name:        "rule text over the cap",
			fsys:        fstest.MapFS{"PATTERN.md": overview(textOverCap)},
			want:        []string{KindLongLine + ":PATTERN-long"},
			wantMessage: fmt.Sprintf("entry rule text is %d characters, over the %d cap", MaxEntryTextChars+1, MaxEntryTextChars),
		},
		{
			name: "rule text at the cap behind a long name, marker and link",
			fsys: fstest.MapFS{
				"PATTERN.md":                  overview(textAtCap),
				"pattern/" + longName + ".md": bg,
			},
		},
		{
			name: "oversize overview",
			fsys: fstest.MapFS{"PATTERN.md": &fstest.MapFile{Data: []byte(entryLine("PATTERN-a", "") + "\n" + strings.Repeat("\n", MaxOverviewBytes))}},
			want: []string{KindOversizeOverview + ":PATTERN.md"},
			wantMessage: fmt.Sprintf("overview is %d bytes, %d over the %d cap",
				len(entryLine("PATTERN-a", ""))+1+MaxOverviewBytes, len(entryLine("PATTERN-a", ""))+1, MaxOverviewBytes),
		},
		{
			name: "empty overview",
			fsys: fstest.MapFS{"PATTERN.md": &fstest.MapFile{Data: []byte("  \n\n")}},
			want: []string{KindEmptyOverview + ":PATTERN.md"},
		},
		{
			name: "entry-less overview",
			fsys: fstest.MapFS{"PATTERN.md": &fstest.MapFile{Data: []byte("# PATTERN\n\nJust prose.\n")}},
			want: []string{KindNoEntries + ":PATTERN.md"},
		},
		{
			name: "unresolved link",
			fsys: fstest.MapFS{"PATTERN.md": overview(entryLine("PATTERN-a", "pattern/PATTERN-a.md"))},
			want: []string{KindUnresolvedLink + ":PATTERN-a"},
		},
		{
			name: "link to another entry's file",
			fsys: fstest.MapFS{
				"PATTERN.md":           overview(entryLine("PATTERN-a", "pattern/PATTERN-b.md"), entryLine("PATTERN-b", "pattern/PATTERN-b.md")),
				"pattern/PATTERN-b.md": bg,
			},
			want: []string{KindWrongLink + ":PATTERN-a"},
		},
		{
			name: "orphan background file",
			fsys: fstest.MapFS{
				"PATTERN.md":           overview(entryLine("PATTERN-a", "")),
				"pattern/PATTERN-z.md": bg,
			},
			want: []string{KindOrphanBackground + ":pattern/PATTERN-z.md"},
		},
		{
			name: "absent overview",
			fsys: fstest.MapFS{},
			want: []string{KindAbsentOverview + ":PATTERN.md"},
		},
		{
			name: "clean without background links",
			fsys: fstest.MapFS{"PATTERN.md": overview(entryLine("PATTERN-a", ""), entryLine("PATTERN-b", ""))},
		},
		{
			name: "clean with background links",
			fsys: fstest.MapFS{
				"PATTERN.md":           overview(entryLine("PATTERN-a", "pattern/PATTERN-a.md"), entryLine("PATTERN-b", "")),
				"pattern/PATTERN-a.md": bg,
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			var got []string
			findings := Check(tt.fsys)
			for _, f := range findings {
				got = append(got, f.Kind+":"+f.Subject)
			}
			if strings.Join(got, "|") != strings.Join(tt.want, "|") {
				t.Fatalf("findings = %v, want %v", got, tt.want)
			}
			if tt.wantLine != 0 && findings[0].Line != tt.wantLine {
				t.Errorf("finding line = %d, want %d", findings[0].Line, tt.wantLine)
			}
			if tt.wantMessage != "" && findings[0].Message != tt.wantMessage {
				t.Errorf("finding message = %q, want %q", findings[0].Message, tt.wantMessage)
			}
		})
	}
}

func TestEntryRuleText(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()
	tests := []struct {
		name string
		line string
		want string
	}{
		{"BareText", "- `PATTERN-a` — When: the rule.", "When: the rule."},
		{"TestMarker", "- `PATTERN-a` — When: the rule. (test)", "When: the rule."},
		{"Link", "- `PATTERN-a` — When: the rule. — [background](pattern/PATTERN-a.md)", "When: the rule."},
		{"TestMarkerAndLink", "- `PATTERN-a` — When: the rule. (test) — [background](pattern/PATTERN-a.md)", "When: the rule."},
		{"MarkerInsideTextKept", "- `PATTERN-a` — A (test) mid-text stays.", "A (test) mid-text stays."},
		{"NoBacktickedName", "- loose text (test)", "loose text"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := EntryRuleText(tt.line); got != tt.want {
				t.Errorf("EntryRuleText(%q) = %q; want %q", tt.line, got, tt.want)
			}
		})
	}
}
