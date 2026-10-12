// vocabname.go holds CheckFabricVocabularyName, the plan check that refuses a declared name the fabric-vocabulary scan would refuse.

package planparser

import (
	"fmt"
	"strings"

	"github.com/Knatte18/loomyard/internal/fabricvocab"
)

// fabricVocabularyNameCheck is the Check ID of a declared name that breaks the fabric vocabulary.
const fabricVocabularyNameCheck = "fabric-vocabulary-name"

// CheckFabricVocabularyName reports one fabric-vocabulary-name finding per name a plan declares that the fabric-vocabulary scan would refuse once the name lands.
// A name is the member half of a Create handle or of a Rename pair's New handle.
// It fails when it carries a bare fabric side token in a file the scan polices, or is a retired host-geometry identifier in non-test code under `internal/` or `cmd/`.
// A handle that does not split into a unit and a member is skipped, since the handle checks already report it.
// A name failing both rules gets one finding; its way forward is to rename the member.
// It sits outside ValidateFormat and Validate, since the plan index appends it beside CheckCardFabricReference.
// It reads plan text only, under any `language:`, and only refuses.
func CheckFabricVocabularyName(plan *Plan) []ValidationError {
	var findings []ValidationError
	for _, card := range plan.Cards {
		for _, handle := range OwnedHandles(card) {
			unit, unitOK := HandleUnit(handle)
			member, memberOK := HandleMember(handle)
			last, lastOK := HandleIdentifier(handle)
			if !unitOK || !memberOK || !lastOK || unit == "" {
				continue
			}
			broken := brokenFabricVocabularyRule(unit, member, last)
			if broken == "" {
				continue
			}
			findings = append(findings, ValidationError{
				Check: fabricVocabularyNameCheck,
				Card:  cardID(card),
				Ref:   handle,
				Detail: fmt.Sprintf(
					"card %s declares the member %q, which breaks PATTERN-fabric-vocabulary: it is %s; rename the member",
					cardID(card), member, broken),
			})
		}
	}
	return findings
}

// brokenFabricVocabularyRule names the rule a declared member breaks as a phrase, or returns "" when it breaks none.
// unit is the handle's package half, member its whole member half and last the member's last dot-segment.
func brokenFabricVocabularyRule(unit, member, last string) string {
	first, _, _ := strings.Cut(unit, "/")
	// An external test package sits in its package's directory, which is what the scan's owner check reads.
	dir := strings.TrimSuffix(unit, "_test")
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
