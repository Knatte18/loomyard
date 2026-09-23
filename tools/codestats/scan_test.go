package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// testClock is the fixed instant every build() call in this file is given, so a report's
// rendered timestamp is assertable rather than whatever the wall clock said.
var testClock = time.Date(2026, 9, 23, 9, 15, 0, 0, time.UTC)

// TestCScanner_Classify covers the C-family classifier line by line, including the two
// states that must survive a line break and the two ways a // can appear without being a
// comment at all.
func TestCScanner_Classify(t *testing.T) {
	tests := []struct {
		name        string
		lines       []string
		wantCode    []bool
		wantComment []bool
	}{
		{
			name:        "LineComment",
			lines:       []string{"// a comment"},
			wantCode:    []bool{false},
			wantComment: []bool{true},
		},
		{
			name:        "TrailingCommentIsCode",
			lines:       []string{"x := 1 // why"},
			wantCode:    []bool{true},
			wantComment: []bool{true},
		},
		{
			name:        "BlockSpansLines",
			lines:       []string{"/* open", "still inside", "close */ x := 1"},
			wantCode:    []bool{false, false, true},
			wantComment: []bool{true, true, true},
		},
		{
			name:        "SlashesInsideStringAreCode",
			lines:       []string{`u := "https://example.com"`},
			wantCode:    []bool{true},
			wantComment: []bool{false},
		},
		{
			name:        "SlashesInsideRawStringSpanningLinesAreCode",
			lines:       []string{"s := `first", "// not a comment", "last`"},
			wantCode:    []bool{true, true, true},
			wantComment: []bool{false, false, false},
		},
		{
			name:        "EscapedQuoteDoesNotEndTheString",
			lines:       []string{`s := "a \" // b"`},
			wantCode:    []bool{true},
			wantComment: []bool{false},
		},
		{
			name:        "InlineBlockThenCode",
			lines:       []string{"x := /* note */ 1"},
			wantCode:    []bool{true},
			wantComment: []bool{true},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var scanner cScanner
			for i, line := range tc.lines {
				code, comment := scanner.classify(line)
				if code != tc.wantCode[i] {
					t.Errorf("line %d %q: code = %v; want %v", i, line, code, tc.wantCode[i])
				}
				if comment != tc.wantComment[i] {
					t.Errorf("line %d %q: comment = %v; want %v", i, line, comment, tc.wantComment[i])
				}
			}
		})
	}
}

// TestHashClassify pins the #-family classifier, including the documented overcount: a #
// inside a quoted string reads as a comment, which is the accepted cost of staying
// string-unaware.
func TestHashClassify(t *testing.T) {
	tests := []struct {
		line        string
		wantCode    bool
		wantComment bool
	}{
		{"# whole line", false, true},
		{"   # indented", false, true},
		{"key: value  # trailing", true, true},
		{"key: value", true, false},
		{`echo "a # b"`, true, true},
	}

	for _, tc := range tests {
		code, comment := hashClassify(tc.line)
		if code != tc.wantCode || comment != tc.wantComment {
			t.Errorf("hashClassify(%q) = (%v, %v); want (%v, %v)",
				tc.line, code, comment, tc.wantCode, tc.wantComment)
		}
	}
}

// TestIsTestPath covers both conventions: Go pins test-ness in the file name, everything
// else in a directory or file name that says so.
func TestIsTestPath(t *testing.T) {
	tests := []struct {
		rel  string
		want bool
	}{
		{"internal/loomcli/cli.go", false},
		{"internal/loomcli/cli_test.go", true},
		{"src/Foo/Bar.cs", false},
		{"src/Foo/BarTests.cs", true},
		{"src/Foo.Tests/Bar.cs", true},
		{"tests/helpers/util.py", true},
		{"docs/overview.md", false},
		{"test/fixture.yaml", true},
	}

	for _, tc := range tests {
		got := isTestPath(tc.rel, strings.ToLower(filepath.Ext(tc.rel)))
		if got != tc.want {
			t.Errorf("isTestPath(%q) = %v; want %v", tc.rel, got, tc.want)
		}
	}
}

// TestSplitLines asserts a trailing newline adds no phantom line and CRLF is normalised,
// since every per-file count is built on this.
func TestSplitLines(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want []string
	}{
		{"Empty", "", nil},
		{"NoTrailingNewline", "a\nb", []string{"a", "b"}},
		{"TrailingNewline", "a\nb\n", []string{"a", "b"}},
		{"CRLF", "a\r\nb\r\n", []string{"a", "b"}},
		{"BlankLineKept", "a\n\nb\n", []string{"a", "", "b"}},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := splitLines([]byte(tc.in))
			if len(got) != len(tc.want) {
				t.Fatalf("splitLines(%q) = %q; want %q", tc.in, got, tc.want)
			}
			for i := range got {
				if got[i] != tc.want[i] {
					t.Errorf("splitLines(%q)[%d] = %q; want %q", tc.in, i, got[i], tc.want[i])
				}
			}
		})
	}
}

// TestClassifyFile asserts the three buckets partition the file: a line is code, comment
// or blank, never two of them, so Lines always equals their sum.
func TestClassifyFile(t *testing.T) {
	src := "// header\npackage main\n\n/* block\n   continues */\nfunc f() {} // trailing\n"
	got := classifyFile("internal/x/f.go", []byte(src))

	if got.Lines != 6 {
		t.Errorf("Lines = %d; want 6", got.Lines)
	}
	if got.Code != 2 {
		t.Errorf("Code = %d; want 2 (package main, func f)", got.Code)
	}
	if got.Comment != 3 {
		t.Errorf("Comment = %d; want 3 (header, block open, block close)", got.Comment)
	}
	if got.Blank != 1 {
		t.Errorf("Blank = %d; want 1", got.Blank)
	}
	if got.Code+got.Comment+got.Blank != got.Lines {
		t.Errorf("Code+Comment+Blank = %d; want Lines = %d -- the buckets must partition the file",
			got.Code+got.Comment+got.Blank, got.Lines)
	}
	if got.IsTest {
		t.Error("IsTest = true; want false")
	}
}

// TestScan_SkipsAndCollects drives the walk over a temp tree: dotted and build
// directories are never descended, an exclude pattern removes a file, and a binary file
// is left out while its text siblings are counted.
func TestScan_SkipsAndCollects(t *testing.T) {
	root := t.TempDir()
	write := func(rel, content string) {
		t.Helper()
		path := filepath.Join(root, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatalf("MkdirAll(%s): %v", path, err)
		}
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatalf("WriteFile(%s): %v", path, err)
		}
	}

	write("a.go", "package a\n")
	write("a_test.go", "package a\n")
	write("docs/readme.md", "# title\n\ntext\n")
	write("generated/big.go", "package generated\n")
	write(".hidden/secret.go", "package secret\n")
	write("vendor/dep/dep.go", "package dep\n")
	write("blob.go", "package blob\n\x00binary\n")

	files, err := Scan(root, false, []string{"generated"})
	if err != nil {
		t.Fatalf("Scan() error = %v; want nil", err)
	}

	got := map[string]bool{}
	for _, f := range files {
		got[f.Path] = true
	}
	for _, want := range []string{"a.go", "a_test.go", "docs/readme.md"} {
		if !got[want] {
			t.Errorf("Scan() missing %q; got %v", want, got)
		}
	}
	for _, unwanted := range []string{"generated/big.go", ".hidden/secret.go", "vendor/dep/dep.go", "blob.go"} {
		if got[unwanted] {
			t.Errorf("Scan() included %q; want it skipped", unwanted)
		}
	}
}

// TestBuild_SplitsSourceFromProse asserts markdown is counted in Total but never in
// Source, which is what keeps the test share a share of code rather than of prose.
func TestBuild_SplitsSourceFromProse(t *testing.T) {
	files := []FileStats{
		{Path: "a.go", Ext: ".go", Lines: 10, Code: 8, Comment: 1, Blank: 1},
		{Path: "a_test.go", Ext: ".go", IsTest: true, Lines: 6, Code: 5, Comment: 0, Blank: 1},
		{Path: "r.md", Ext: ".md", Lines: 4, Code: 3, Blank: 1},
	}
	r := build("/root", testClock, files)

	if r.Source.Files != 2 {
		t.Errorf("Source.Files = %d; want 2 -- markdown must not count as source", r.Source.Files)
	}
	if r.Total.Files != 3 {
		t.Errorf("Total.Files = %d; want 3", r.Total.Files)
	}
	if r.Test.Code != 5 || r.Production.Code != 8 {
		t.Errorf("Test.Code/Production.Code = %d/%d; want 5/8", r.Test.Code, r.Production.Code)
	}
	if r.Markdown.Lines != 4 {
		t.Errorf("Markdown.Lines = %d; want 4", r.Markdown.Lines)
	}
	if got := pct(r.Test.Code, r.Source.Code); got != "38.5%" {
		t.Errorf("test share = %s; want 38.5%% (5 of 13)", got)
	}
}

// TestPct_ZeroWholeIsDash asserts an empty bucket prints a dash: a language with no lines
// has no comment share, and 0.0%% would read as one measured at zero.
func TestPct_ZeroWholeIsDash(t *testing.T) {
	if got := pct(0, 0); got != "-" {
		t.Errorf("pct(0, 0) = %q; want %q", got, "-")
	}
}

// TestRender_ReportsEverySection asserts the rendered table carries the three questions
// the command exists to answer, so a section cannot silently go missing.
func TestRender_ReportsEverySection(t *testing.T) {
	report := build("/root", testClock, []FileStats{
		{Path: "a.go", Ext: ".go", Lines: 10, Code: 8, Comment: 1, Blank: 1},
		{Path: "r.md", Ext: ".md", Lines: 4, Code: 3, Blank: 1},
	})
	report.DurationMS = 1500

	var out strings.Builder
	render(&out, report)

	for _, want := range []string{
		"## By extension", "## Test vs production", "## Markdown",
		"Test share of source **code lines**", "2026-09-23T09:15:00Z",
		"**Scan took:** 1.5s",
	} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("render() output missing %q; got:\n%s", want, out.String())
		}
	}
}
