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
// in the card being rewritten's own inner map. A key with no entry in that card's own SurfaceRefs
// map substitutes itself, which is correct for a ref that was already canonical on disk -- so a
// canonical string two cards spell differently on disk resolves to each card's own lexeme, never to
// whichever card ParsePlan happened to visit last.
// Substitution only ever touches a backtick-wrapped sub-bullet payload matching the same "- `ref`"
// or "- `old` -> `new`" shapes parseRefField and parseRenameField recognise, so prose that happens
// to contain the same string is never rewritten. A card file whose bytes do not actually change is
// left byte-identical (not even rewritten with identical bytes), and an empty or fully-unmatched
// subs map writes nothing at all.
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
		if newLine, ok := rewriteBulletLine(line, lexemeSubs); ok {
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
func cardLexemeSubs(plan *Plan, cardKey string, subs map[string]string) map[string]string {
	lexemeSubs := make(map[string]string, len(subs))
	for canonical, replacement := range subs {
		lexeme := canonical
		if surface := plan.SurfaceRefs[cardKey][canonical]; surface != "" {
			lexeme = surface
		}
		lexemeSubs[lexeme] = replacement
	}
	return lexemeSubs
}

// rewriteBulletLine substitutes line's own bullet payload -- either a single "- `ref`" or an
// "- `old` -> `new`" pair -- per lexemeSubs, and reports whether it changed anything. A line that is
// not one of these two shapes, or whose payload carries no entry in lexemeSubs, reports false and
// returns line unmodified.
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
		return leading + "- `" + newOld + "` -> `" + newNew + "`", true
	}

	if m := singleRefLineRe.FindStringSubmatch(payload); m != nil {
		if replacement, ok := lexemeSubs[m[1]]; ok {
			return leading + "- `" + replacement + "`", true
		}
	}

	return line, false
}
