// diffpieces_test.go pins how a round's diff is split into per-package pieces under the size cap,
// and that Engine.Review writes the pieces and names them in the explore step.

package burlerengine

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// fileSection returns a diff section for path with one hunk per body.
func fileSection(path string, hunks ...string) string {
	section := "diff --git a/" + path + " b/" + path + "\nindex 1111111..2222222 100644\n--- a/" + path + "\n+++ b/" + path + "\n"
	for _, hunk := range hunks {
		section += "@@ -1,1 +1,1 @@\n" + hunk
	}
	return section
}

// repeatedLines returns count lines of the form "+<prefix>NN", numbered from first.
func repeatedLines(prefix string, first, count int) string {
	var lines strings.Builder
	for i := first; i < first+count; i++ {
		lines.WriteString("+" + prefix + string(rune('0'+i/10)) + string(rune('0'+i%10)) + "\n")
	}
	return lines.String()
}

// TestWriteDiffPieces proves each package's files land in pieces of their own, in order, every piece under the cap.
// It proves the pieces of a package hold its files' text exactly, split at file, hunk and then line boundaries.
// It proves a split file's later pieces open with its header, and an empty diff writes nothing.
// It also proves a package is read from the file's path and a deep package still gets a file name.
// A directory or piece that cannot be written is an error.
func TestWriteDiffPieces(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	alpha := fileSection("internal/alpha/a.go", "-old\n+new\n")
	alphaTwo := fileSection("internal/alpha/b.go", "-x\n+y\n")
	beta := fileSection("internal/beta/c.go", "-p\n+q\n")
	root := fileSection("README.md", "-r\n+s\n")
	spaced := fileSection("docs/foo b/bar.md", "-m\n+n\n")
	longLine := fileSection("internal/wide/w.go", "+"+strings.Repeat("z", 500)+"\n")
	multibyteLine := fileSection("internal/wide/w.go", "+"+strings.Repeat("é", 300)+"\n")
	manyShortLines := fileSection("internal/lines/l.go", strings.Repeat("+x\n", 40))
	deepPackage := strings.Repeat("deep/", 70) + "leaf"
	deep := fileSection(deepPackage+"/d.go", "-d\n+e\n")
	deepName := strings.ReplaceAll(deepPackage, "/", "-")
	deepName = deepName[len(deepName)-diffPieceNameMaxBytes:]

	// bigFile holds three hunks under a header of its own, which every piece after its first repeats.
	// The cap lets the header and the first hunk fill the first piece, and not the second hunk.
	// The second hunk fits a piece of its own, and the third does not, so it is cut at line ends.
	const bigPath = "internal/big/h.go"
	bigHeader := fileSection(bigPath)
	hunkOne := "@@ -1,1 +1,1 @@\n" + strings.Repeat("+aaaa\n", 10)
	hunkTwo := "@@ -1,1 +1,1 @@\n" + strings.Repeat("+bbbb\n", 20)
	hunkThree := "@@ -1,1 +1,1 @@\n" + repeatedLines("c", 1, 50)
	bigFile := bigHeader + hunkOne + hunkTwo + hunkThree
	bigCap := len(bigHeader) + len(hunkOne) + len(hunkTwo) - 1
	smallBesideBig := fileSection("internal/big/a.go", "-s\n+t\n")

	type wantPiece struct {
		pkg          string
		part, parts  int
		name         string
		wantContents string
	}
	tests := []struct {
		name  string
		diff  string
		limit pieceCap
		want  []wantPiece
		// wantJoined, per package, is the text its pieces must join to when their contents are not pinned one by one.
		wantJoined map[string]string
		// blocked names what stands in the way of the write: "directory" or "piece".
		blocked string
	}{
		{name: "EmptyDiffWritesNothing", diff: " \n", limit: pieceCap{1000, 1000}},
		{
			name: "OnePiecePerPackageInFirstSeenOrder", diff: alpha + beta + alphaTwo + root, limit: pieceCap{1000, 1000},
			want: []wantPiece{
				{pkg: "internal/alpha", part: 1, parts: 1, name: "001-internal-alpha.diff", wantContents: alpha + alphaTwo},
				{pkg: "internal/beta", part: 1, parts: 1, name: "002-internal-beta.diff", wantContents: beta},
				{pkg: rootPackage, part: 1, parts: 1, name: "003-root.diff", wantContents: root},
			},
		},
		{
			name: "PackageOverTheByteCapSplitsAtFiles", diff: alpha + alphaTwo, limit: pieceCap{len(alpha) + 1, 1000},
			want: []wantPiece{
				{pkg: "internal/alpha", part: 1, parts: 2, name: "001-internal-alpha-part1.diff", wantContents: alpha},
				{pkg: "internal/alpha", part: 2, parts: 2, name: "002-internal-alpha-part2.diff", wantContents: alphaTwo},
			},
		},
		{
			name: "PackageOverTheLineCapSplitsAtFiles", diff: alpha + alphaTwo, limit: pieceCap{100_000, strings.Count(alpha, "\n")},
			want: []wantPiece{
				{pkg: "internal/alpha", part: 1, parts: 2, name: "001-internal-alpha-part1.diff", wantContents: alpha},
				{pkg: "internal/alpha", part: 2, parts: 2, name: "002-internal-alpha-part2.diff", wantContents: alphaTwo},
			},
		},
		{
			name: "FileOverTheCapSplitsAtHunksThenLineEnds", diff: bigFile, limit: pieceCap{bigCap, 1000},
			want: []wantPiece{
				{pkg: "internal/big", part: 1, parts: 3, name: "001-internal-big-part1.diff", wantContents: bigHeader + hunkOne},
				{
					pkg: "internal/big", part: 2, parts: 3, name: "002-internal-big-part2.diff",
					wantContents: bigHeader + hunkTwo + "@@ -1,1 +1,1 @@\n" + repeatedLines("c", 1, 11),
				},
				{pkg: "internal/big", part: 3, parts: 3, name: "003-internal-big-part3.diff", wantContents: bigHeader + repeatedLines("c", 12, 39)},
			},
		},
		{
			// The small file and the big file's header fit one piece together, and the header's first hunk does not.
			// The header then moves to the next piece with that hunk, so no piece ends on a bare header.
			name: "SplitFileHeaderNeverEndsAPiece", diff: smallBesideBig + bigFile, limit: pieceCap{bigCap, 1000},
			want: []wantPiece{
				{pkg: "internal/big", part: 1, parts: 4, name: "001-internal-big-part1.diff", wantContents: smallBesideBig},
				{pkg: "internal/big", part: 2, parts: 4, name: "002-internal-big-part2.diff", wantContents: bigHeader + hunkOne},
				{
					pkg: "internal/big", part: 3, parts: 4, name: "003-internal-big-part3.diff",
					wantContents: bigHeader + hunkTwo + "@@ -1,1 +1,1 @@\n" + repeatedLines("c", 1, 11),
				},
				{pkg: "internal/big", part: 4, parts: 4, name: "004-internal-big-part4.diff", wantContents: bigHeader + repeatedLines("c", 12, 39)},
			},
		},
		{name: "ManyShortLinesOverTheLineCapSplit", diff: manyShortLines, limit: pieceCap{100_000, 6}, wantJoined: map[string]string{"internal/lines": manyShortLines}},
		{name: "LineOverTheCapIsCut", diff: longLine, limit: pieceCap{120, 1000}, wantJoined: map[string]string{"internal/wide": longLine}},
		{name: "LineOverTheCapIsCutOnARuneBoundary", diff: multibyteLine, limit: pieceCap{200, 1000}, wantJoined: map[string]string{"internal/wide": multibyteLine}},
		{
			name: "PackageIsReadFromThePathNotTheHeader", diff: spaced, limit: pieceCap{1000, 1000},
			want: []wantPiece{{pkg: "docs/foo b", part: 1, parts: 1, name: "001-docs-foo b.diff", wantContents: spaced}},
		},
		{
			name: "DeepPackageNameIsCapped", diff: deep, limit: pieceCap{10_000, 1000},
			want: []wantPiece{{pkg: deepPackage, part: 1, parts: 1, name: "001-" + deepName + ".diff", wantContents: deep}},
		},
		{name: "DirectoryThatCannotBeCreatedIsAnError", diff: alpha, limit: pieceCap{1000, 1000}, blocked: "directory"},
		{name: "PieceThatCannotBeWrittenIsAnError", diff: alpha, limit: pieceCap{1000, 1000}, blocked: "piece"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			dir := filepath.Join(t.TempDir(), diffDirName)
			switch tt.blocked {
			case "directory":
				if err := os.WriteFile(dir, nil, 0o644); err != nil {
					t.Fatalf("WriteFile(%s) = %v; want nil", dir, err)
				}
			case "piece":
				if err := os.MkdirAll(filepath.Join(dir, "001-internal-alpha.diff"), 0o755); err != nil {
					t.Fatalf("MkdirAll(piece) = %v; want nil", err)
				}
			}

			pieces, err := writeDiffPieces(dir, tt.diff, tt.limit)

			if tt.blocked != "" {
				if err == nil {
					t.Fatalf("writeDiffPieces() = %+v, nil; want an error with a %s that cannot be written", pieces, tt.blocked)
				}
				return
			}
			if err != nil {
				t.Fatalf("writeDiffPieces() = %v; want nil error", err)
			}
			if tt.want == nil && tt.wantJoined == nil {
				if len(pieces) != 0 {
					t.Errorf("writeDiffPieces() = %+v; want no piece", pieces)
				}
				if _, err := os.Stat(dir); !os.IsNotExist(err) {
					t.Errorf("Stat(%s) = %v; want the directory absent", dir, err)
				}
				return
			}
			joined := map[string]string{}
			for i, piece := range pieces {
				data, err := os.ReadFile(piece.Path)
				if err != nil {
					t.Fatalf("ReadFile(piece %d) = %v; want nil", i, err)
				}
				if len(data) > tt.limit.maxBytes || strings.Count(string(data), "\n") > tt.limit.maxLines {
					t.Errorf("piece %d is %d bytes and %d line breaks; want at most %d and %d", i, len(data), strings.Count(string(data), "\n"), tt.limit.maxBytes, tt.limit.maxLines)
				}
				if !utf8.Valid(data) {
					t.Errorf("piece %d is not valid UTF-8", i)
				}
				joined[piece.Package] += string(data)
				if tt.want == nil {
					continue
				}
				want := tt.want[i]
				if piece.Package != want.pkg || piece.Part != want.part || piece.Parts != want.parts || filepath.Base(piece.Path) != want.name || filepath.Dir(piece.Path) != dir {
					t.Errorf("piece %d = %+v; want package %q part %d of %d at %s in %s", i, piece, want.pkg, want.part, want.parts, want.name, dir)
				}
				if string(data) != want.wantContents {
					t.Errorf("piece %d contents = %q; want %q", i, data, want.wantContents)
				}
			}
			if tt.want != nil && len(pieces) != len(tt.want) {
				t.Errorf("writeDiffPieces() wrote %d pieces; want %d", len(pieces), len(tt.want))
			}
			for pkg, want := range tt.wantJoined {
				if joined[pkg] != want {
					t.Errorf("pieces of %s join to %q; want %q", pkg, joined[pkg], want)
				}
				if len(pieces) < 2 {
					t.Errorf("writeDiffPieces() wrote %d pieces of %s; want it split", len(pieces), pkg)
				}
			}
		})
	}
}

// TestEngine_Review_StagesTheDiff proves a profile with a diff gets its pieces written beside the instruction files.
// Each piece is named in the explore step, with the root and a split package labelled,
// and a profile without a diff gets neither.
func TestEngine_Review_StagesTheDiff(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	// Two files of a thousand lines each pass the line cap together, so their package splits in two parts.
	lines := strings.Repeat("+x\n", 1000)
	diff := fileSection("internal/alpha/a.go", "-old\n+new\n") + fileSection("internal/beta/b.go", "-x\n+y\n") +
		fileSection("README.md", "-r\n+s\n") + fileSection("internal/gamma/one.go", lines) + fileSection("internal/gamma/two.go", lines)
	tests := []struct {
		name       string
		diff       string
		wantPieces []string
		wantText   []string
	}{
		{
			name: "DiffIsStaged", diff: diff,
			wantPieces: []string{"001-internal-alpha.diff", "002-internal-beta.diff", "003-root.diff", "004-internal-gamma-part1.diff", "005-internal-gamma-part2.diff"},
			wantText:   []string{"(the repository root)", "part 1 of 2", "part 2 of 2"},
		},
		{name: "NoDiffStagesNothing"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			root, p := newEngineTestProfile(t)
			p.Diff = tt.diff
			e := newEngineForTest(t, root, doneReview(approvedReview))

			if _, err := e.Review(p, RunOpts{}); err != nil {
				t.Fatalf("Review() = %v; want nil error", err)
			}

			explores, err := filepath.Glob(filepath.Join(root, ".lym", "burler", "review-*", "instruction-1-explore.md"))
			if err != nil || len(explores) != 1 {
				t.Fatalf("Glob(explore step) = %v, %v; want one file", explores, err)
			}
			explore, err := os.ReadFile(explores[0])
			if err != nil {
				t.Fatalf("ReadFile(explore step) = %v; want nil", err)
			}
			pieceDir := filepath.Join(filepath.Dir(explores[0]), diffDirName)
			for _, name := range tt.wantPieces {
				piece := filepath.Join(pieceDir, name)
				if _, err := os.Stat(piece); err != nil {
					t.Errorf("Stat(%s) = %v; want the piece written", piece, err)
				}
				requireContains(t, string(explore), "`"+piece+"`")
			}
			for _, want := range tt.wantText {
				requireContains(t, string(explore), want)
			}
			if tt.wantPieces == nil {
				if _, err := os.Stat(pieceDir); !os.IsNotExist(err) {
					t.Errorf("Stat(%s) = %v; want no diff directory", pieceDir, err)
				}
				requireNotContains(t, string(explore), "in pieces")
			}
		})
	}
}
