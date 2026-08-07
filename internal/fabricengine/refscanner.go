// refscanner.go implements RefScanner, fabric's answer to "does this command reference fabric's
// two-checkout mechanism" — the audit policy a consumer like websterengine needs without ever
// holding the weft path or the command-spelling pattern itself.

package fabricengine

import (
	"fmt"
	"regexp"

	"github.com/Knatte18/loomyard/internal/lyxcwd"
	"github.com/Knatte18/loomyard/internal/weftname"
)

// RefScanner detects a command that references fabric's two-checkout mechanism: a fabric-driving
// command spelling (lyx fabric/weft/warp) or a path touching the weft sibling worktree.
// Construct via NewRefScanner; the zero value is not valid.
type RefScanner struct {
	pattern *regexp.Regexp
}

// NewRefScanner returns a RefScanner for l's worktree, compiling its regex once so repeated
// Matches calls (e.g. over every Bash command in a transcript) never recompile it.
func NewRefScanner(l *lyxcwd.Location) *RefScanner {
	weftPath := regexp.QuoteMeta(WeftWorktree(l))
	weftSuffix := regexp.QuoteMeta(weftname.Suffix)
	pattern := fmt.Sprintf(
		`lyx(?:\.exe)?\s+(fabric|weft|warp)\b|%s|\S*%s\b`,
		weftPath, weftSuffix,
	)
	return &RefScanner{pattern: regexp.MustCompile(pattern)}
}

// Matches reports whether cmd references fabric's two-checkout mechanism, either by spelling
// (lyx fabric/weft/warp) or by touching the weft sibling worktree's path.
func (s *RefScanner) Matches(cmd string) bool {
	return s.pattern.MatchString(cmd)
}
