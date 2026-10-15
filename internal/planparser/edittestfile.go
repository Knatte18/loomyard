// edittestfile.go holds CheckEditTestFile, the plan check that refuses a `_test.go` file listed under a card's Edit group.

package planparser

import (
	"fmt"
	"strings"
)

// editTestFileCheck is the Check ID of a `_test.go` file listed under an Edit group.
const editTestFileCheck = "edit-test-file"

// CheckEditTestFile reports one edit-test-file finding per file self glyph under an Edit group of a card not in done whose path ends in `_test.go`.
// A path with a segment the go tool ignores (`testdata`, `vendor`, or a name starting with `.` or `_`) is exempt, since no test there is compiled.
// A member glyph, a plan: handle and a path are skipped, as are the Create, Delete, Rename, Move, Prosa and Custom groups and Uses.
// A plan whose language has no glyph alphabet is never checked.
// The way forward names each changed test function, fake or helper as a member glyph, and declares each new one as a Create handle.
// It sits outside ValidateFormat and Validate because the plan index appends it.
// It takes the done set where CheckCardFabricReference filters nothing, because a completed card's file has already landed and no writer can fix it.
// It reads plan text only, so it fires whether or not the file exists, and only refuses.
func CheckEditTestFile(plan *Plan, done []Card) []ValidationError {
	lang, ok := planLanguage(plan)
	if !ok {
		return nil
	}
	finished := make(map[string]bool, len(done))
	for _, card := range done {
		finished[cardID(card)] = true
	}

	var findings []ValidationError
	for _, card := range plan.Cards {
		if finished[cardID(card)] {
			continue
		}
		for _, group := range card.TargetGroups {
			if group.Type != CardTypeEdit {
				continue
			}
			for _, ref := range group.Refs {
				if _, disp := lookup(gateEditTestFile, ref); disp != dispKeep {
					continue
				}
				parsed, err := parseGlyph(lang, ref)
				if err != nil || !parsed.IsSelf() {
					continue
				}
				path, _ := parsed.UnitPath()
				if !strings.HasSuffix(path, "_test.go") || hasIgnoredSegment(path) {
					continue
				}
				findings = append(findings, ValidationError{
					Check: editTestFileCheck,
					Card:  cardID(card),
					Ref:   ref,
					Detail: fmt.Sprintf(
						"card %s lists the test file %q under Edit; name each existing test function, fake or helper the card changes as a member glyph under Edit, "+
							"declare each new one as a Create plan: handle on the package directory (or on <dir>_test for an external test package), "+
							"keep a new test file as a Create path, and when the change is confined to the file's header, such as its build constraint, name every member of that file",
						cardID(card), ref),
				})
			}
		}
	}
	return findings
}

// hasIgnoredSegment reports whether any `/`-separated segment of path, the file name included, is one the go tool ignores.
func hasIgnoredSegment(path string) bool {
	for _, segment := range strings.Split(path, "/") {
		if ignoredByGoTool(segment) {
			return true
		}
	}
	return false
}

// ignoredByGoTool reports whether the go tool skips a directory or file named segment: `testdata`, `vendor`, or a name starting with `.` or `_`.
func ignoredByGoTool(segment string) bool {
	return segment == "testdata" || segment == "vendor" || strings.HasPrefix(segment, ".") || strings.HasPrefix(segment, "_")
}
