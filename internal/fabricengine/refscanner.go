// refscanner.go implements RefScanner, fabric's answer to "does this command reference fabric's
// two-checkout mechanism" — the audit policy a consumer like websterengine needs without ever
// holding the weft path or the command-spelling pattern itself.

package fabricengine

import (
	"regexp"
	"strings"

	"github.com/Knatte18/loomyard/internal/lyxcwd"
	"github.com/Knatte18/loomyard/internal/weftname"
)

// spellingPattern matches a fabric-driving command spelling (lyx fabric/weft/warp) in command
// position: the start of the command, or after a shell separator (`;`, `&`, `|`, `(`, a newline, `$(`
// or a backtick), optionally behind command wrappers, env assignments and a directory prefix.
// It runs over the command with its quoted spans blanked (see blankQuoted), so a search pattern
// such as `grep "lyx fabric remove" doc.md` is text, not an invocation.
// A spelling hidden inside a quoted `bash -c "…"` is not caught; the audit guards well-meaning
// agents against a slip, not an adversary.
var spellingPattern = regexp.MustCompile(
	"(?:^|[;&|(\\n`]|\\$\\()\\s*(?:(?:env|exec|command|nohup|time|xargs)\\s+|\\w+=\\S*\\s+)*" +
		`(?:\S*[/\\])?lyx(?:\.exe)?\s+(?:fabric|weft|warp)\b`,
)

// RefScanner detects a command that references fabric's two-checkout mechanism: a fabric-driving
// command spelling in command position (spellingPattern) or a path touching the weft sibling
// worktree, matched anywhere in the command, quoted or not.
// Construct via NewRefScanner; the zero value is not valid.
type RefScanner struct {
	pathPattern *regexp.Regexp
}

// NewRefScanner returns a RefScanner for l's worktree, compiling its path regex once so repeated
// Matches calls (e.g. over every Bash command in a transcript) never recompile it.
func NewRefScanner(l *lyxcwd.Location) *RefScanner {
	weftPath := regexp.QuoteMeta(WeftWorktree(l))
	weftSuffix := regexp.QuoteMeta(weftname.Suffix)
	return &RefScanner{pathPattern: regexp.MustCompile(weftPath + `|\S*` + weftSuffix + `\b`)}
}

// Matches reports whether cmd references fabric's two-checkout mechanism, either by spelling
// (lyx fabric/weft/warp, in command position) or by touching the weft sibling worktree's path.
func (s *RefScanner) Matches(cmd string) bool {
	return s.pathPattern.MatchString(cmd) || spellingPattern.MatchString(blankQuoted(cmd))
}

// blankQuoted returns cmd with the content of every single- and double-quoted span replaced by
// spaces, quotes included, so a separator or a spelling inside a quoted argument can never look like
// command position.
// A backslash outside single quotes escapes the next character, as in POSIX shells; an unterminated
// quote blanks to the end of the command.
func blankQuoted(cmd string) string {
	var b strings.Builder
	b.Grow(len(cmd))
	var quote rune
	escaped := false
	for _, r := range cmd {
		switch {
		case escaped:
			escaped = false
			if quote == 0 {
				b.WriteRune(r)
				continue
			}
		case r == '\\' && quote != '\'':
			escaped = true
			if quote == 0 {
				b.WriteRune(r)
				continue
			}
		case quote == 0 && (r == '\'' || r == '"'):
			quote = r
		case r == quote:
			quote = 0
		case quote == 0:
			b.WriteRune(r)
			continue
		}
		b.WriteByte(' ')
	}
	return b.String()
}
