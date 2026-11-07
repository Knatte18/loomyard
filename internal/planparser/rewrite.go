// rewrite.go implements RewriteRefs, which substitutes refs in place across the card files of a plan.
// It rewrites only the backtick-wrapped sub-bullet payloads that a substitution's key names.

package planparser

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// singleRefLineRe matches a bullet payload that is one backtick-wrapped ref and nothing else.
// The payload is the bullet's text with its leading "- " marker trimmed.
var singleRefLineRe = regexp.MustCompile("^`([^`]+)`$")

// RewriteRefs substitutes every occurrence of subs' keys with their replacements across the card files under planDir.
// The keys are canonical refs, and each card's own on-disk spellings of a ref are rewritten.
// Only a backtick-wrapped sub-bullet payload of the form "- `ref`" or "- `old` -> `new`" is substituted.
// Prose that contains the same text is left alone.
// A Create declaration bullet whose handle binds to a real glyph collapses to a plain ref bullet.
// A card file whose bytes do not change is left byte-identical, and an empty or unmatched subs map writes nothing.
// It edits the card text in place rather than re-rendering the plan from the parsed model.
// A re-render would normalize away the malformed bullets the validator reports.
// It would also rewrite the backup-mode paths the operator chose to allow.
// It returns an error for a missing plan directory, an unreadable card file, or a failed write.
func RewriteRefs(planDir string, subs map[string]string) error {
	return rewriteRefsIn(
		planDir, subs,
		func(name string) ([]byte, error) { return os.ReadFile(filepath.Join(planDir, name)) },
		func(name string, data []byte) error { return os.WriteFile(filepath.Join(planDir, name), data, 0o644) },
	)
}

// rewriteRefsIn applies subs to the plan whose files read returns, writing each changed card file through write.
func rewriteRefsIn(planDir string, subs map[string]string, read func(name string) ([]byte, error), write func(name string, data []byte) error) error {
	if len(subs) == 0 {
		return nil
	}

	plan, err := ParsePlanFrom(planDir, read)
	if err != nil {
		return err
	}

	for _, c := range plan.Cards {
		if err := rewriteCardFile(plan, c, subs, read, write); err != nil {
			return err
		}
	}

	return nil
}

// rewriteCardFile applies subs to one card file's sub-bullet payloads, matching keys by their on-disk spellings.
// It reads and writes the card by its name under plan.Dir, and writes back only when a line changed.
func rewriteCardFile(plan *Plan, c Card, subs map[string]string, read func(name string) ([]byte, error), write func(name string, data []byte) error) error {
	fileName := cardFileName(c.Number, c.Slug)
	filePath := filepath.Join(plan.Dir, fileName)

	data, err := read(fileName)
	if err != nil {
		return fmt.Errorf("planparser: read card file %s: %w", filePath, err)
	}

	lexemeSubs := cardLexemeSubs(plan, cardID(c), subs)
	if len(lexemeSubs) == 0 {
		return nil
	}

	lines := strings.Split(string(data), "\n")
	changed := false
	for i, line := range lines {
		// The newLine != line guard keeps a card file byte-identical when a substitution maps a payload to itself.
		// rewriteBulletLine reports ok for any matched payload, even one whose reconstruction is identical.
		// Without the guard, the file would be written back with identical bytes.
		if newLine, ok := rewriteBulletLine(line, lexemeSubs); ok && newLine != line {
			lines[i] = newLine
			changed = true
		}
	}
	if !changed {
		return nil
	}

	if err := write(fileName, []byte(strings.Join(lines, "\n"))); err != nil {
		return fmt.Errorf("planparser: write card file %s: %w", filePath, err)
	}
	return nil
}

// cardLexemeSubs resolves each canonical key in subs to the on-disk spellings that cardKey uses for it.
// A key with no recorded spelling resolves to itself.
// It emits one substitution per spelling:
// a card can spell one ref two ways across its own fields, and one substitution alone leaves the other stale.
func cardLexemeSubs(plan *Plan, cardKey string, subs map[string]string) map[string]string {
	lexemeSubs := make(map[string]string, len(subs))
	for canonical, replacement := range subs {
		lexemes := plan.SurfaceRefs[cardKey][canonical]
		if len(lexemes) == 0 {
			lexemeSubs[canonical] = replacement
			continue
		}
		for _, lexeme := range lexemes {
			lexemeSubs[lexeme] = replacement
		}
	}
	return lexemeSubs
}

// rewriteBulletLine substitutes a bullet line's payload per lexemeSubs.
// The payload is either a single "- `ref`" or a "- `old` -> `new`" pair.
// It reports whether the line changed.
// A line of neither shape, or whose payload has no entry in lexemeSubs, comes back unmodified.
//
// A Create declaration bullet, `plan:<handle>` -> `<declaration head>`, collapses once its handle binds to a glyph.
// It becomes a plain `- <glyph>` ref.
// An in-place substitution would leave `<glyph>` -> `<declaration head>`, which no longer parses as a declaration.
// The card would then fail to parse.
//
// The collapse applies only when the left token is a handle and its replacement is not one.
// Canonicalization swaps one `plan:` handle for another and does not collapse;
// a Rename pair's left side is a glyph by contract, so it does not collapse either.
func rewriteBulletLine(line string, lexemeSubs map[string]string) (string, bool) {
	trimmed := strings.TrimSpace(line)
	if !strings.HasPrefix(trimmed, "- ") {
		return line, false
	}
	leading := line[:len(line)-len(strings.TrimLeft(line, " \t"))]
	payload := strings.TrimSpace(strings.TrimPrefix(trimmed, "- "))

	if m := moveLineRe.FindStringSubmatch(payload); m != nil {
		oldRef, newRef := m[1], m[2]
		newOld, oldChanged := lexemeSubs[oldRef]
		newNew, newChanged := lexemeSubs[newRef]
		if !oldChanged && !newChanged {
			return line, false
		}
		if !oldChanged {
			newOld = oldRef
		}
		if !newChanged {
			newNew = newRef
		}
		if oldChanged && IsHandleRef(oldRef) && !IsHandleRef(newOld) {
			return leading + "- `" + newOld + "`", true
		}
		return leading + "- `" + newOld + "` -> `" + newNew + "`", true
	}

	if m := singleRefLineRe.FindStringSubmatch(payload); m != nil {
		if replacement, ok := lexemeSubs[m[1]]; ok {
			return leading + "- `" + replacement + "`", true
		}
	}

	return line, false
}
