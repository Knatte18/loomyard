// projection.go holds the pure projection of the overview under a plan's prescribed entry lines and removals, and the read of one entry's bullet.

package pattern

import "strings"

// ProjectOverview returns the overview as it stands once prescribed entry lines are applied and removed entries are dropped, and never changes overview.
// Each line of prescribed is applied in order by its entry ID:
// it replaces the whole bullet of that ID, continuation lines included, or is appended at the end of the overview when the ID is absent.
// A later line for the same ID therefore wins, and a line whose head names no entry ID is skipped.
// Every bullet of an ID in removes is then deleted, continuation lines included; an absent ID changes nothing.
// CRLF in overview is read as LF, and an appended line is always terminated by a newline.
func ProjectOverview(overview []byte, prescribed []string, removes []string) []byte {
	lines, trailingNewline := splitOverviewLines(overview)

	for _, entryLine := range prescribed {
		head := entryHeadRe.FindStringSubmatch(entryLine)
		if head == nil {
			continue
		}
		start, end, found := findBullet(lines, head[1])
		if !found {
			lines = append(lines, entryLine)
			trailingNewline = true
			continue
		}
		lines = append(lines[:start], append([]string{entryLine}, lines[end:]...)...)
	}

	for _, id := range removes {
		for {
			start, end, found := findBullet(lines, id)
			if !found {
				break
			}
			lines = append(lines[:start], lines[end:]...)
		}
	}

	projected := strings.Join(lines, "\n")
	if trailingNewline && len(lines) > 0 {
		projected += "\n"
	}
	return []byte(projected)
}

// EntryBullet returns the bullet of the entry named id as the overview holds it, the entry line and its continuation lines joined by newlines.
// It answers false when the overview holds no such entry.
func EntryBullet(overview []byte, id string) (string, bool) {
	lines, _ := splitOverviewLines(overview)
	start, end, found := findBullet(lines, id)
	if !found {
		return "", false
	}
	return strings.Join(lines[start:end], "\n"), true
}

// splitOverviewLines reads overview as LF text and returns its lines without the final empty element a trailing newline would leave, and whether it ended in a newline.
func splitOverviewLines(overview []byte) ([]string, bool) {
	text := strings.ReplaceAll(string(overview), "\r\n", "\n")
	if text == "" {
		return nil, false
	}
	trailingNewline := strings.HasSuffix(text, "\n")
	return strings.Split(strings.TrimSuffix(text, "\n"), "\n"), trailingNewline
}

// findBullet returns the half-open line range of the first bullet whose entry head names id, the entry line and the continuation lines below it.
func findBullet(lines []string, id string) (start, end int, found bool) {
	for i, line := range lines {
		if !strings.HasPrefix(line, entryBulletPrefix) {
			continue
		}
		head := entryHeadRe.FindStringSubmatch(line)
		if head == nil || head[1] != id {
			continue
		}
		end = i + 1
		for end < len(lines) && continues(lines[end]) {
			end++
		}
		return i, end, true
	}
	return 0, 0, false
}
