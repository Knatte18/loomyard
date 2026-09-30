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

// heredocOpener matches a heredoc redirection (`<<EOF`, `<<-EOF`, `<<'EOF'`, `<<"EOF"`) and captures
// its delimiter word.
var heredocOpener = regexp.MustCompile(`<<-?\s*['"]?(\w+)['"]?`)

// RefScanner detects a command that references fabric's two-checkout mechanism: a fabric-driving
// command spelling in command position (spellingPattern) or a path touching a weft sibling worktree.
// Heredoc bodies are dropped before any match, since a file an agent writes is content, not a
// command it runs.
// A weft sibling counts as a path when the name follows a path separator (`/hub/x-weft`, quoted or
// not); a bare word ending in the weft suffix counts only outside quotes, so a string literal such as
// `"archive-happy-weft"` in a test is text.
// Construct via NewRefScanner; the zero value is not valid.
type RefScanner struct {
	pathPattern *regexp.Regexp
	barePattern *regexp.Regexp
}

// NewRefScanner returns a RefScanner for l's worktree, compiling its regexes once so repeated
// Matches calls (e.g. over every Bash command in a transcript) never recompile them.
func NewRefScanner(l *lyxcwd.Location) *RefScanner {
	weftPath := regexp.QuoteMeta(WeftWorktree(l))
	weftSuffix := regexp.QuoteMeta(weftname.Suffix)
	return &RefScanner{
		pathPattern: regexp.MustCompile(weftPath + "|[/\\\\][^\\s/\\\\\"'`]*" + weftSuffix + `\b`),
		barePattern: regexp.MustCompile(`\S*` + weftSuffix + `\b`),
	}
}

// Matches reports whether cmd references fabric's two-checkout mechanism, either by spelling
// (lyx fabric/weft/warp, in command position) or by touching a weft sibling worktree's path.
func (s *RefScanner) Matches(cmd string) bool {
	cmd = stripHeredocBodies(cmd)
	if s.pathPattern.MatchString(cmd) {
		return true
	}
	unquoted := blankQuoted(cmd)
	return s.barePattern.MatchString(unquoted) || spellingPattern.MatchString(unquoted)
}

// stripHeredocBodies returns cmd with the body of every heredoc removed: for each line carrying a
// heredoc opener, the following lines up to and including the one that is exactly the delimiter
// (leading tabs allowed, as `<<-` permits) are dropped.
// An unterminated heredoc drops everything after its opener line.
func stripHeredocBodies(cmd string) string {
	lines := strings.Split(cmd, "\n")
	kept := make([]string, 0, len(lines))
	var pending []string
	for _, line := range lines {
		if len(pending) > 0 {
			if strings.TrimLeft(line, "\t") == pending[0] {
				pending = pending[1:]
			}
			continue
		}
		kept = append(kept, line)
		for _, m := range heredocOpener.FindAllStringSubmatch(line, -1) {
			pending = append(pending, m[1])
		}
	}
	return strings.Join(kept, "\n")
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
