// diffpieces.go splits a review round's unified diff into pieces of one package each.
// Every piece is small enough to read whole, and the pieces are written beside the reviewer's instruction files.

package burlerengine

import (
	"fmt"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"unicode/utf8"
)

// diffPieceMaxBytes caps the size of one diff piece.
// An agent's file read refuses a file whose text passes about 25,000 tokens.
// Dense text such as a base85 binary patch runs near two bytes per token.
// 40,000 bytes of diff stays under that.
const diffPieceMaxBytes = 40_000

// diffPieceMaxLines caps the number of lines of one diff piece.
// An agent's file read returns 2,000 lines by default, so a piece under this cap reads whole.
const diffPieceMaxLines = 1_500

// diffPieceNameMaxBytes caps the package part of a piece's file name.
// A deep package path would otherwise push the name past a filesystem's name limit.
const diffPieceNameMaxBytes = 80

// diffDirName is the directory under a review half's instruction directory that holds the round's diff pieces.
const diffDirName = "diff"

// rootPackage is the package of a file at the repository root.
const rootPackage = "."

// pieceCap bounds one diff piece by its size in bytes and by its number of line breaks.
type pieceCap struct {
	maxBytes int
	maxLines int
}

// fits reports whether text of the given size and line-break count is within the cap.
func (c pieceCap) fits(size, lineBreaks int) bool {
	return size <= c.maxBytes && lineBreaks <= c.maxLines
}

// diffPiece is one written piece of a round's diff.
// Package is the directory its files sit in, and Part counts from 1 among the Parts pieces of that package.
type diffPiece struct {
	Path    string
	Package string
	Part    int
	Parts   int
}

// packageDiff is the diff text of one package's files, in the order git wrote them.
type packageDiff struct {
	pkg   string
	files []string
}

// diffUnit is a run of diff text that is never split across pieces.
// Continuation is the file header a piece repeats when it opens with this unit, so its hunks stay tied to their file.
// It is empty for the first unit of a file, which carries the header itself.
type diffUnit struct {
	text         string
	continuation string
}

// writeDiffPieces writes diff into dir as pieces of one package each and returns them in order.
// No piece passes limit in bytes or in lines.
// A package whose files pass limit is split at file boundaries into numbered parts.
// One file that passes limit is split at its hunk starts, then at line ends.
// Each piece of it after the first opens with the file's header, when that header fits within half a piece.
// A whitespace-only diff writes nothing, creates no directory and returns no piece.
func writeDiffPieces(dir, diff string, limit pieceCap) ([]diffPiece, error) {
	if strings.TrimSpace(diff) == "" {
		return nil, nil
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("burler: stage the diff: %w", err)
	}
	var pieces []diffPiece
	for _, group := range groupDiffByPackage(diff) {
		var units []diffUnit
		for _, file := range group.files {
			units = append(units, boundedUnits(file, limit)...)
		}
		texts := packUnits(units, limit)
		for i, text := range texts {
			piece := diffPiece{Package: group.pkg, Part: i + 1, Parts: len(texts)}
			piece.Path = filepath.Join(dir, pieceFileName(len(pieces)+1, piece))
			if err := os.WriteFile(piece.Path, []byte(text), 0o644); err != nil {
				return nil, fmt.Errorf("burler: stage the diff: %w", err)
			}
			pieces = append(pieces, piece)
		}
	}
	return pieces, nil
}

// pieceFileName names the seq-th piece of a diff after its package, with its part number when the package has several.
// A package path past diffPieceNameMaxBytes keeps its last bytes, which hold its innermost directories.
func pieceFileName(seq int, piece diffPiece) string {
	name := strings.ReplaceAll(piece.Package, "/", "-")
	if piece.Package == rootPackage {
		name = "root"
	}
	if len(name) > diffPieceNameMaxBytes {
		name = name[len(name)-diffPieceNameMaxBytes:]
		for !utf8.RuneStart(name[0]) {
			name = name[1:]
		}
	}
	if piece.Parts > 1 {
		name += fmt.Sprintf("-part%d", piece.Part)
	}
	return fmt.Sprintf("%03d-%s.diff", seq, name)
}

// groupDiffByPackage splits diff into its per-file sections and groups them by the directory of each file's path.
// Groups keep the order of their first file, and each group keeps its files' order.
func groupDiffByPackage(diff string) []packageDiff {
	var groups []packageDiff
	index := map[string]int{}
	for _, file := range splitBeforeLines(diff, "diff --git ") {
		pkg := diffFilePackage(file)
		i, ok := index[pkg]
		if !ok {
			i = len(groups)
			index[pkg] = i
			groups = append(groups, packageDiff{pkg: pkg})
		}
		groups[i].files = append(groups[i].files, file)
	}
	return groups
}

// diffFilePackage returns the directory of the file a diff section changes.
// It reads the section's `+++` line, or its `---` line for a deleted file, so a path holding " b/" is read whole.
// A section with neither, such as a binary change, is read from its `diff --git` line instead.
// A section with no path, and a file at the repository root, belong to rootPackage.
func diffFilePackage(section string) string {
	var oldSide, newSide string
	for _, line := range strings.Split(section, "\n") {
		if strings.HasPrefix(line, "@@ ") {
			break
		}
		if rest, ok := strings.CutPrefix(line, "--- "); ok {
			oldSide = diffSidePath(rest)
		} else if rest, ok := strings.CutPrefix(line, "+++ "); ok {
			newSide = diffSidePath(rest)
		}
	}
	switch {
	case strings.HasPrefix(newSide, "b/"):
		return path.Dir(newSide[len("b/"):])
	case newSide == "/dev/null" && strings.HasPrefix(oldSide, "a/"):
		return path.Dir(oldSide[len("a/"):])
	}
	return diffHeaderPackage(section)
}

// diffSidePath returns the path a `---` or `+++` line names after its marker.
// Git ends a path holding a space with a tab and writes a path holding special bytes as a quoted C string.
func diffSidePath(rest string) string {
	rest = strings.TrimSuffix(rest, "\t")
	if strings.HasPrefix(rest, `"`) {
		if unquoted, err := strconv.Unquote(rest); err == nil {
			return unquoted
		}
	}
	return rest
}

// diffHeaderPackage returns the directory of the file a section's `diff --git` line names on its new side.
// A section with no such line belongs to rootPackage.
func diffHeaderPackage(section string) string {
	header, _, _ := strings.Cut(section, "\n")
	header = strings.ReplaceAll(header, `"`, "")
	i := strings.LastIndex(header, " b/")
	if !strings.HasPrefix(header, "diff --git ") || i < 0 {
		return rootPackage
	}
	return path.Dir(header[i+len(" b/"):])
}

// boundedUnits breaks one file's diff into consecutive units that each fit a piece.
// A file within limit is one unit.
// A larger file is cut at its hunk starts first and at line ends after,
// and every unit after its header repeats that header when it opens a piece.
// A single line longer than a piece is cut at a rune boundary.
func boundedUnits(file string, limit pieceCap) []diffUnit {
	if limit.fits(len(file), strings.Count(file, "\n")) {
		return []diffUnit{{text: file}}
	}
	header := continuationHeader(file, limit)
	room := pieceCap{maxBytes: limit.maxBytes - len(header), maxLines: limit.maxLines - strings.Count(header, "\n")}
	var units []diffUnit
	for _, hunk := range splitBeforeLines(file[len(header):], "@@ ") {
		if room.fits(len(hunk), strings.Count(hunk, "\n")) {
			units = append(units, diffUnit{text: hunk, continuation: header})
			continue
		}
		for _, line := range strings.SplitAfter(hunk, "\n") {
			for len(line) > room.maxBytes {
				cut := runeBoundaryAtOrBefore(line, room.maxBytes)
				units = append(units, diffUnit{text: line[:cut], continuation: header})
				line = line[cut:]
			}
			if line != "" {
				units = append(units, diffUnit{text: line, continuation: header})
			}
		}
	}
	if header != "" {
		// The header travels with the file's first unit, so no piece ends on a header alone.
		units[0] = diffUnit{text: header + units[0].text}
	}
	return units
}

// continuationHeader returns the text of file before its first hunk.
// It returns "" when the file has no hunk, or when that text passes half of limit and would leave little room for hunks.
func continuationHeader(file string, limit pieceCap) string {
	chunks := splitBeforeLines(file, "@@ ")
	if len(chunks) < 2 {
		return ""
	}
	header := chunks[0]
	if 2*len(header) > limit.maxBytes || 2*strings.Count(header, "\n") > limit.maxLines {
		return ""
	}
	return header
}

// runeBoundaryAtOrBefore returns the largest offset of text, at most limit, that does not fall inside a UTF-8 rune.
// It returns limit when no such offset above 0 exists, which only invalid UTF-8 causes.
// The text must be longer than limit.
func runeBoundaryAtOrBefore(text string, limit int) int {
	cut := limit
	for cut > 0 && !utf8.RuneStart(text[cut]) {
		cut--
	}
	if cut == 0 {
		return limit
	}
	return cut
}

// packUnits joins consecutive units into texts that each fit limit, each unit whole in one text.
// A text that opens with a unit carrying a continuation header starts with that header.
// Every unit, with its continuation, is assumed to fit limit alone.
func packUnits(units []diffUnit, limit pieceCap) []string {
	var texts []string
	var current strings.Builder
	breaks := 0
	for _, unit := range units {
		unitBreaks := strings.Count(unit.text, "\n")
		if current.Len() > 0 && !limit.fits(current.Len()+len(unit.text), breaks+unitBreaks) {
			texts = append(texts, current.String())
			current.Reset()
			breaks = 0
		}
		if current.Len() == 0 {
			current.WriteString(unit.continuation)
			breaks += strings.Count(unit.continuation, "\n")
		}
		current.WriteString(unit.text)
		breaks += unitBreaks
	}
	if current.Len() > 0 {
		texts = append(texts, current.String())
	}
	return texts
}

// splitBeforeLines splits text before every line that starts with prefix, keeping every byte.
// Text before the first such line is a chunk of its own.
func splitBeforeLines(text, prefix string) []string {
	var chunks []string
	start := 0
	for at := 0; at < len(text); {
		if at > start && strings.HasPrefix(text[at:], prefix) {
			chunks = append(chunks, text[start:at])
			start = at
		}
		end := strings.IndexByte(text[at:], '\n')
		if end < 0 {
			break
		}
		at += end + 1
	}
	return append(chunks, text[start:])
}

// formatDiffPieces renders pieces as one backtick-wrapped bullet each.
// A bullet names the piece's package, and its part when the package has several.
func formatDiffPieces(pieces []diffPiece) string {
	var b strings.Builder
	for _, piece := range pieces {
		fmt.Fprintf(&b, "- `%s` — `%s`", piece.Path, piece.Package)
		if piece.Package == rootPackage {
			b.WriteString(" (the repository root)")
		}
		if piece.Parts > 1 {
			fmt.Fprintf(&b, ", part %d of %d", piece.Part, piece.Parts)
		}
		b.WriteString("\n")
	}
	return b.String()
}
