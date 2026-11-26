// reinterpret.go carries a hub's loom.yaml across the change of what code.auto_accept: true does at a spent budget.

package loomengine

import (
	"fmt"

	"gopkg.in/yaml.v3"

	"github.com/Knatte18/loomyard/internal/configengine"
)

// CodeAutoAcceptBlockingReinterpretationID names the change to code.auto_accept.
// The setting stopped accepting a round with an open BLOCKING finding at a spent budget.
const CodeAutoAcceptBlockingReinterpretationID = "loom-code-auto-accept-escalates-blocking"

// FlagCodeAutoAccept carries a hub loom.yaml across a change to what code.auto_accept: true does.
// It no longer accepts a round with an open BLOCKING finding at a spent budget.
// Such a round now escalates to the run's parent, which accepts it or orders another round.
// A file that sets code.auto_accept to true is flagged with that way forward, and left unchanged.
// So is a file that sets none, and an absent file (nil existing), since both take the template's default of true.
// A file that sets it to false is neither changed nor flagged.
// The worktrees argument is unused, as the check judges the file alone.
func FlagCodeAutoAccept(existing []byte, _ []configengine.HubWorktree) (rewritten []byte, rewrote, flagged []string, err error) {
	var areas struct {
		Code map[string]yaml.Node `yaml:"code"`
	}
	if err := yaml.Unmarshal(existing, &areas); err != nil {
		return nil, nil, nil, fmt.Errorf("parse the loom config: %w", err)
	}
	if node, set := areas.Code[leafAutoAccept]; set {
		var enabled bool
		if err := node.Decode(&enabled); err != nil {
			return nil, nil, nil, fmt.Errorf("read %s of the loom config: %w", FullKeyName(codeAutoAcceptKey), err)
		}
		if !enabled {
			return existing, nil, nil, nil
		}
	}
	return existing, nil, []string{fmt.Sprintf(
		"has %s: true, set here or taken from the template default, "+
			"which no longer accepts a round with an open BLOCKING finding at a spent budget "+
			"on a perch whose recipe row sets blocking_escalates, as the shipped Code-Review rows do; "+
			"such a round now escalates to the run's parent with cause budget, which accepts it or orders another round, "+
			"and the value stays as it is",
		FullKeyName(codeAutoAcceptKey))}, nil
}
