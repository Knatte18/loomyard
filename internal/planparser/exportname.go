// exportname.go holds CheckExportName, the plan's name checks run on one `plan:` handle a parts file exports.

package planparser

import (
	"fmt"
	"path/filepath"
	"strings"
)

// handleMalformedCheck is the Check ID of a handle whose spelling cannot name a declared member.
const handleMalformedCheck = "handle-malformed"

// CheckExportName returns the findings the plan's name checks would raise on handle once a part's card declared it.
// It lets the parts check refuse a name before a part writer must create it.
// handle is a `plan:` handle a parts file exports, judged as a Create declaration under the plan format's default language, which a part's plan keeps unless it names another.
// The file-unit rule is applied to every handle, as for a Create declaration, whose unit half is the one a resolver reads; a handle a part meets only as a Rename New side is judged the same way.
// The findings are handle-malformed, for a handle with no unit or with a file as its unit, and fabric-vocabulary-name.
// Each carries handle as its Ref, no Card, and a Detail that ends in its way forward.
func CheckExportName(handle string) []ValidationError {
	plan := &Plan{}
	unit, ok := handleUnit(handle)
	if !ok {
		return []ValidationError{{
			Check:  handleMalformedCheck,
			Ref:    handle,
			Detail: fmt.Sprintf("the export %q carries no \"#\" after %q and so names no unit; way forward: spell it `plan:<package directory>#<Name>`", handle, HandlePrefix),
		}}
	}
	var findings []ValidationError
	if plan.GoToolchain() && strings.HasSuffix(unit, ".go") {
		findings = append(findings, ValidationError{
			Check:  handleMalformedCheck,
			Ref:    handle,
			Detail: fmt.Sprintf("the export %q names the file %q as its unit, and a file-unit member spelling can never resolve; way forward: name its package directory %q as the unit", handle, unit, filepath.ToSlash(filepath.Dir(unit))),
		})
	}
	if member, broken := fabricVocabularyBreak(plan, handle); broken != "" {
		findings = append(findings, ValidationError{
			Check:  fabricVocabularyNameCheck,
			Ref:    handle,
			Detail: fmt.Sprintf("the export %q declares the member %q, which breaks PATTERN-fabric-boundary: it is %s; way forward: rename the member", handle, member, broken),
		})
	}
	return findings
}
