// vocabname.go holds CheckFabricVocabularyName, which refuses a declared name the fabric-vocabulary scan would refuse.

package planparser

import (
	"fmt"
	"strings"

	"github.com/Knatte18/loomyard/internal/fabricvocab"
)

// fabricVocabularyNameCheck is the Check ID of a declared name that breaks the fabric vocabulary.
const fabricVocabularyNameCheck = "fabric-vocabulary-name"

// CheckFabricVocabularyName reports a finding per declared name the fabric-vocabulary scan would refuse once it lands.
// A name is the member half of a Create handle or of a Rename pair's New handle.
// It fails when the name is a bare fabric side token in a file the scan polices.
// It also fails when it is a retired host-geometry identifier in non-test code under `internal/` or `cmd/`.
// A handle that does not split into a unit and a member is skipped, because the handle checks already report it.
// A name that breaks both rules gets one finding, and the way forward is to rename the member.
// It is not part of plan validation, so a caller runs it as its own check.
// It reads plan text only, under any `language:`, and only refuses.
func CheckFabricVocabularyName(plan *Plan) []ValidationError {
	var findings []ValidationError
	for _, card := range plan.Cards {
		for _, handle := range OwnedHandles(card) {
			member, broken := fabricVocabularyBreak(plan, handle)
			if broken == "" {
				continue
			}
			findings = append(findings, ValidationError{
				Check: fabricVocabularyNameCheck,
				Card:  cardID(card),
				Ref:   handle,
				Detail: fmt.Sprintf(
					"card %s declares the member %q, which breaks PATTERN-fabric-boundary: it is %s; rename the member",
					cardID(card), member, broken),
			})
		}
	}
	return findings
}

// fabricVocabularyBreak returns handle's member half, and the rule that member breaks as a phrase.
// The phrase is read under plan's language.
// broken is "" when the member breaks no rule or the handle does not split into a unit and a member.
func fabricVocabularyBreak(plan *Plan, handle string) (member, broken string) {
	unit, unitOK := HandleUnit(handle)
	member, memberOK := HandleMember(handle)
	last, lastOK := HandleIdentifier(handle)
	if !unitOK || !memberOK || !lastOK || unit == "" {
		return member, ""
	}
	if body, _ := HandleBody(handle); body != "" {
		if parsed, err := ParseRef(plan, body); err == nil && parsed.Name != "" {
			last, _, _ = strings.Cut(parsed.Name, "<")
		}
	}
	return member, brokenFabricVocabularyRule(unit, member, last, plan.GoToolchain())
}

// brokenFabricVocabularyRule names the rule a declared member breaks as a phrase, or returns "" when it breaks none.
// unit is the handle's package half, member its whole member half and last the member's last dot-segment.
// goToolchain says whether the plan's language is Go, whose unit strings carry the external-test `_test` suffix.
func brokenFabricVocabularyRule(unit, member, last string, goToolchain bool) string {
	first, _, _ := strings.Cut(unit, "/")
	dir := unit
	if goToolchain {
		// An external test package sits in its package's directory, the directory the owner set is keyed by.
		dir = goPackageDir(unit)
	}
	isTestName := hasTestPrefix(last)

	bareScope := first == "internal" || fabricvocab.IsDocRoot(first) || isTestName
	if fabricvocab.BareToken(member) && !fabricvocab.SkipsBareCheck(dir) && bareScope {
		return "a bare fabric side token"
	}
	if !isTestName && fabricvocab.IsHostGeometryIdentifier(last) && (first == "internal" || first == "cmd") {
		return "a retired host-geometry identifier"
	}
	return ""
}

// hasTestPrefix reports whether name carries a Test, Benchmark, Fuzz or Example prefix.
func hasTestPrefix(name string) bool {
	for _, prefix := range []string{"Test", "Benchmark", "Fuzz", "Example"} {
		if strings.HasPrefix(name, prefix) {
			return true
		}
	}
	return false
}
