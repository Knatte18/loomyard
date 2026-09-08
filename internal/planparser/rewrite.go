// rewrite.go implements RewriteRefs, the plan's second write path: the single primitive every
// later ref-substitution occasion calls -- handle canonicalization, handle binding at card
// completion, and drift auto-repair -- rather than each growing its own writer. A full re-render
// from the parsed model is rejected for a sharper reason than triplication: planparser is
// deliberately lenient at the card level, preserving malformed bullets so the validator can
// enumerate every defect, so a round-trip would silently normalize away exactly the defects the
// validator exists to report, and would additionally rewrite backup-mode paths the operator chose
// to allow. RewriteRefs instead substitutes in place, one card file at a time, touching only the
// backtick-wrapped sub-bullet payloads a substitution's key actually names.

package planparser

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// singleRefLineRe matches a "- `ref`" sub-bullet's payload after the leading "- " bullet marker is
// trimmed off, mirroring the exact shape parseRefField recognises (a lone backtick-wrapped token
// with nothing else on the line).
var singleRefLineRe = regexp.MustCompile("^`([^`]+)`$")

// RewriteRefs substitutes, across every card file under planDir, every occurrence of subs' own
// keys with their corresponding replacement value, and returns a "planparser:"-prefixed wrapped
// error for a missing plan directory, an unreadable card file, or a failed write.
// subs is keyed on canonical model strings -- matching what every one of RewriteRefs' three
// callers natively produces -- rather than on-disk lexemes: RewriteRefs bridges the surface<->model
// gap itself, by parsing planDir with ParsePlan to obtain Plan.SurfaceRefs and looking each key up
// in the card being rewritten's own inner map, which holds every lexeme that card spelled the
// canonical ref with, so a card carrying two spellings of one ref has both rewritten. A key with no entry in that card's own SurfaceRefs
// map substitutes itself, which is correct for a ref that was already canonical on disk -- so a
// canonical string two cards spell differently on disk resolves to each card's own lexeme, never to
// whichever card ParsePlan happened to visit last.
// Substitution only ever touches a backtick-wrapped sub-bullet payload matching the same "- `ref`"
// or "- `old` -> `new`" shapes parseRefField and parseRenameField recognise, so prose that happens
// to contain the same string is never rewritten. A card file whose bytes do not actually change is
// left byte-identical (not even rewritten with identical bytes), and an empty or fully-unmatched
// subs map writes nothing at all.
// The one shape-changing case -- a Create declaration bullet collapsing to a plain ref when its
// handle binds to a real glyph -- is documented on rewriteBulletLine.
func RewriteRefs(planDir string, subs map[string]string) error {
	if len(subs) == 0 {
		return nil
	}

	plan, err := ParsePlan(planDir)
	if err != nil {
		return err
	}

	for _, c := range plan.Cards {
		if err := rewriteCardFile(planDir, plan, c, subs); err != nil {
			return err
		}
	}

	return nil
}

// rewriteCardFile rewrites one card file's backtick-wrapped sub-bullet payloads per subs, first
// resolving every substitution's canonical key against c's own SurfaceRefs entry into the on-disk
// lexeme actually being searched for, then applying that lexeme-keyed map line by line. The file is
// written back only when at least one line actually changed.
func rewriteCardFile(planDir string, plan *Plan, c Card, subs map[string]string) error {
	fileName := cardFileName(c.Number, c.Slug)
	filePath := filepath.Join(planDir, fileName)

	data, err := os.ReadFile(filePath)
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
		// The newLine != line guard is what makes the byte-identical promise above hold even for a
		// substitution that maps a payload to itself: rewriteBulletLine reports ok for any matched
		// payload, including one whose reconstruction is identical.
		if newLine, ok := rewriteBulletLine(line, lexemeSubs); ok && newLine != line {
			lines[i] = newLine
			changed = true
		}
	}
	if !changed {
		return nil
	}

	if err := os.WriteFile(filePath, []byte(strings.Join(lines, "\n")), 0o644); err != nil {
		return fmt.Errorf("planparser: write card file %s: %w", filePath, err)
	}
	return nil
}

// cardLexemeSubs resolves subs' canonical keys into cardKey's own on-disk lexemes, via
// plan.SurfaceRefs[cardKey]. A canonical key absent from that inner map resolves to itself.
//
// It emits one substitution per RECORDED LEXEME, not one per canonical key: a card that spells one
// canonical ref two ways across two of its own fields carries two lexemes, and emitting only one
// rewrote one bullet and left the other stale — a half-rewritten card (crucible round
// opus-medium-r6, R6-13).
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

// rewriteBulletLine substitutes line's own bullet payload -- either a single "- `ref`" or an
// "- `old` -> `new`" pair -- per lexemeSubs, and reports whether it changed anything. A line that is
// not one of these two shapes, or whose payload carries no entry in lexemeSubs, reports false and
// returns line unmodified.
//
// One arrow bullet is rewritten to a DIFFERENT shape rather than in place: a Create group's
// declaration bullet, `plan:<handle>` -> `<declaration head>`, whose handle is being bound to its
// real glyph. Substituting in place there leaves `<glyph>` -> `<declaration head>`, which is no
// longer a handle declaration but still carries the arrow, so parseCreateField routes it to
// CreateRaw and the card parses with a blocking handle-malformed AND an empty target list
// (card-field-empty) -- observed live, and it wedged every batch after the one that bound the
// handle. The declaration head existed only to compute the glyph that has now been computed, so the
// bullet collapses to the plain `- <glyph>` ref every other Create target already uses.
//
// The collapse is deliberately narrow: it requires the LEFT token to be a handle whose replacement
// is not one. Canonicalization substitutes one handle for another (still `plan:`-prefixed) and does
// not collapse; a Rename pair's left side is a glyph by contract and does not collapse either.
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
		if oldChanged && strings.HasPrefix(oldRef, HandlePrefix) && !strings.HasPrefix(newOld, HandlePrefix) {
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
