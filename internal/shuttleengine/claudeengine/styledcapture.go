// styledcapture.go turns a styled pane capture (the screen with its SGR attributes) into the plain capture the box readings take.
// A box that holds only Claude's placeholder or prompt suggestion, which Claude draws faint, comes back blank, so it reads as empty.

package claudeengine

import (
	"strconv"
	"strings"
	"unicode"

	"github.com/Knatte18/loomyard/internal/shuttleengine"
)

var _ shuttleengine.StyledBoxReader = (*Claude)(nil)

const (
	escapeByte    = '\x1b'
	bellByte      = '\a'
	sgrFaint      = 2
	sgrNormal     = 22
	sgrReset      = 0
	sgrForeground = 38
	sgrBackground = 48
	sgrUnderline  = 58
	sgrColorRGB   = 2
	sgrColor256   = 5
)

// styledRun is a stretch of one line's text drawn under one faint attribute.
type styledRun struct {
	text  string
	faint bool
}

// styledCell is one rune of a line with the faint attribute it was drawn under.
type styledCell struct {
	r     rune
	faint bool
}

// PlainBoxCapture answers the plain capture of a styled one, with a placeholder-only input box blanked.
// Every SGR, OSC and other escape sequence is removed and each line's trailing ASCII spaces are trimmed as `capture-pane -p` trims them, so for a capture with no placeholder the answer is what `capture-pane -p` returns.
// The box is blanked, its faint cells replaced by spaces, only when everything in it but a lone unstyled `%` is faint and the faint text is not the collapsed-paste placeholder.
// Unstyled text, a box mixing unstyled and faint text, and a faint collapsed-paste placeholder come back unblanked,
// so a release that restyles the placeholder reads as a draft.
// A caller reads the box, the running turn and every other classification from the one plain capture it answers.
func (c *Claude) PlainBoxCapture(styled string) string {
	rawLines := strings.Split(styled, "\n")
	cells := make([][]styledCell, len(rawLines))
	plain := make([]string, len(rawLines))
	for i, line := range rawLines {
		cells[i] = styledCells(parseStyledLine(line))
		plain[i] = plainLine(cells[i])
	}

	interior, ok := inputBoxInterior(strings.Join(plain, "\n"))
	if !ok {
		return strings.Join(plain, "\n")
	}
	// The box's interior holds the last caret line, which is the last one in the capture,
	// so its offset inside the interior locates where the interior starts.
	start := lastCaretLine(plain) - lastCaretLine(interior)
	boxCells := cells[start : start+len(interior)]
	if placeholderOnly(boxCells) {
		for i, lineCells := range boxCells {
			plain[start+i] = plainLine(blankedCells(lineCells))
		}
	}
	return strings.Join(plain, "\n")
}

// parseStyledLine splits one capture line into runs of text by faint attribute, dropping every escape sequence.
// The attribute starts off at the line's start.
func parseStyledLine(line string) []styledRun {
	var runs []styledRun
	var text strings.Builder
	faint := false
	flush := func() {
		if text.Len() > 0 {
			runs = append(runs, styledRun{text: text.String(), faint: faint})
			text.Reset()
		}
	}

	for i := 0; i < len(line); {
		if line[i] != escapeByte {
			start := i
			for i < len(line) && line[i] != escapeByte {
				i++
			}
			text.WriteString(line[start:i])
			continue
		}
		if i+1 >= len(line) {
			break
		}
		switch line[i+1] {
		case '[':
			end := i + 2
			for end < len(line) && (line[end] < '@' || line[end] > '~') {
				end++
			}
			if end >= len(line) {
				i = len(line)
				continue
			}
			if line[end] == 'm' {
				next := applySGR(line[i+2:end], faint)
				if next != faint {
					flush()
					faint = next
				}
			}
			i = end + 1
		case ']':
			end := i + 2
			for end < len(line) && line[end] != bellByte && !(line[end] == escapeByte && end+1 < len(line) && line[end+1] == '\\') {
				end++
			}
			switch {
			case end >= len(line):
				i = len(line)
			case line[end] == bellByte:
				i = end + 1
			default:
				i = end + 2
			}
		default:
			i += 2
		}
	}
	flush()
	return runs
}

// applySGR returns the faint attribute after the SGR parameters params, given the attribute before them.
// The parameters after 38, 48 and 58 are a colour and set nothing: `2;r;g;b` or `5;n` follow in the semicolon form, and the colon form carries them in its own parameter.
func applySGR(params string, faint bool) bool {
	fields := strings.Split(params, ";")
	for i := 0; i < len(fields); i++ {
		head, _, colonForm := strings.Cut(fields[i], ":")
		number := sgrReset
		if head != "" {
			parsed, err := strconv.Atoi(head)
			if err != nil {
				continue
			}
			number = parsed
		}
		switch number {
		case sgrReset, sgrNormal:
			faint = false
		case sgrFaint:
			faint = true
		case sgrForeground, sgrBackground, sgrUnderline:
			if colonForm {
				continue
			}
			if i+1 < len(fields) {
				switch fields[i+1] {
				case strconv.Itoa(sgrColorRGB):
					i += 4
				case strconv.Itoa(sgrColor256):
					i += 2
				}
			}
		}
	}
	return faint
}

// styledCells flattens runs into one cell per rune.
func styledCells(runs []styledRun) []styledCell {
	var cells []styledCell
	for _, run := range runs {
		for _, r := range run.text {
			cells = append(cells, styledCell{r: r, faint: run.faint})
		}
	}
	return cells
}

// plainLine joins cells into text with its trailing ASCII spaces trimmed.
func plainLine(cells []styledCell) string {
	var text strings.Builder
	for _, cell := range cells {
		text.WriteRune(cell.r)
	}
	return strings.TrimRight(text.String(), " ")
}

// lastCaretLine returns the index of the last line carrying gateCaretMarker, or -1 when none does.
func lastCaretLine(lines []string) int {
	caret := -1
	for i, line := range lines {
		if strings.Contains(line, gateCaretMarker) {
			caret = i
		}
	}
	return caret
}

// isBoxSeparator reports whether r separates tokens in a box interior line: whitespace, the caret or a side bar.
func isBoxSeparator(r rune) bool {
	return unicode.IsSpace(r) || strings.ContainsRune(boxInteriorChars, r)
}

// withoutEOLMarks returns cells with every lone unstyled shellEOLMark token removed.
func withoutEOLMarks(cells []styledCell) []styledCell {
	var kept []styledCell
	var token []styledCell
	flushToken := func() {
		isEOLMark := len(token) == 1 && string(token[0].r) == shellEOLMark && !token[0].faint
		if !isEOLMark {
			kept = append(kept, token...)
		}
		token = nil
	}
	for _, cell := range cells {
		if isBoxSeparator(cell.r) {
			flushToken()
			kept = append(kept, cell)
			continue
		}
		token = append(token, cell)
	}
	flushToken()
	return kept
}

// placeholderOnly reports whether the box interior, a lone unstyled shellEOLMark set aside, holds text that is all faint and is not the collapsed-paste placeholder.
func placeholderOnly(interior [][]styledCell) bool {
	var texts []string
	for _, lineCells := range interior {
		var text strings.Builder
		for _, cell := range withoutEOLMarks(lineCells) {
			if isBoxSeparator(cell.r) {
				if unicode.IsSpace(cell.r) {
					text.WriteRune(cell.r)
				}
				continue
			}
			if !cell.faint {
				return false
			}
			text.WriteRune(cell.r)
		}
		if line := strings.TrimSpace(text.String()); line != "" {
			texts = append(texts, line)
		}
	}
	if len(texts) == 0 {
		return false
	}
	return !pastePlaceholderShape.MatchString(strings.Join(texts, " "))
}

// blankedCells returns cells with every faint cell that is not a separator replaced by a space.
func blankedCells(cells []styledCell) []styledCell {
	blanked := make([]styledCell, len(cells))
	for i, cell := range cells {
		if cell.faint && !isBoxSeparator(cell.r) {
			cell.r = ' '
		}
		blanked[i] = cell
	}
	return blanked
}
