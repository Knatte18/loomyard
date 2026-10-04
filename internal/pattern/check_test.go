// check_test.go covers Check, one case per finding kind plus the clean overviews.

package pattern

import (
	"fmt"
	"strings"
	"testing"
	"testing/fstest"
)

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
	bg := &fstest.MapFile{Data: []byte("background\n")}
	longLine := entryLine("PATTERN-long", "") + strings.Repeat("x", MaxEntryLineChars)

	tests := []struct {
		name string
		fsys fstest.MapFS
		want []string // "kind:subject"
	}{
		{
			name: "bad name",
			fsys: fstest.MapFS{"PATTERN.md": overview(entryLine("PATTERN-Bad_Name", ""))},
			want: []string{KindBadName + ":PATTERN-Bad_Name"},
		},
		{
			name: "duplicate name",
			fsys: fstest.MapFS{"PATTERN.md": overview(entryLine("PATTERN-a", ""), entryLine("PATTERN-a", ""))},
			want: []string{KindDuplicateName + ":PATTERN-a"},
		},
		{
			name: "two-line entry",
			fsys: fstest.MapFS{"PATTERN.md": overview(entryLine("PATTERN-a", ""), "  and it goes on")},
			want: []string{KindContinuedEntry + ":PATTERN-a"},
		},
		{
			name: "over-long line",
			fsys: fstest.MapFS{"PATTERN.md": overview(longLine)},
			want: []string{KindLongLine + ":PATTERN-long"},
		},
		{
			name: "oversize overview",
			fsys: fstest.MapFS{"PATTERN.md": &fstest.MapFile{Data: []byte(entryLine("PATTERN-a", "") + "\n" + strings.Repeat("\n", MaxOverviewBytes))}},
			want: []string{KindOversizeOverview + ":PATTERN.md"},
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
			var got []string
			for _, f := range Check(tt.fsys) {
				got = append(got, f.Kind+":"+f.Subject)
			}
			if strings.Join(got, "|") != strings.Join(tt.want, "|") {
				t.Fatalf("findings = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestCheck_ReportsLineNumber(t *testing.T) {
	fsys := fstest.MapFS{"PATTERN.md": overview(entryLine("PATTERN-a", ""), entryLine("PATTERN-a", ""))}
	findings := Check(fsys)
	if len(findings) != 1 || findings[0].Line != 6 {
		t.Fatalf("findings = %+v, want one finding at line 6", findings)
	}
}
