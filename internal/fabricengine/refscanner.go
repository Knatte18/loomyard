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

// ReferenceRule is the location-free part of the fabric-reference audit.
// It knows the command spelling and the sibling-name shape, but not any one worktree's own sibling path, so a plan can be checked with it before any worktree exists.
// Heredoc bodies are dropped before any match, since a file an agent writes is content, not a command it runs.
// A weft sibling counts as a path when the name follows a path separator (`/hub/x-weft`, quoted or not).
// A bare word ending in the weft suffix counts only outside quotes, so a string literal such as `"archive-happy-weft"` in a test is text.
// Construct via NewReferenceRule; the zero value is not valid.
type ReferenceRule struct {
	pathPattern *regexp.Regexp
	barePattern *regexp.Regexp
}

// referenceTrim holds the separators a match may end on, trimmed from the returned text.
const referenceTrim = " \t\r\n/\\\"'`;&|)"

// NewReferenceRule returns the location-free reference rule, compiling its regexes once so repeated calls never recompile them.
func NewReferenceRule() *ReferenceRule {
	weftSuffix := regexp.QuoteMeta(weftname.Suffix)
	// The suffix must end the name: `\b` alone would also match inside a slug that merely contains
	// it, such as the task worktree `/hub/fabric-readd-weft-push`, since `-` is a word boundary.
	nameEnd := "(?:$|[\\s/\\\\\"'`;&|)])"
	// A path segment never holds a shell or regex metacharacter, so a backslash escape inside a quoted
	// search pattern (`grep "a\|-weft"`) is not read as a Windows separator before a weft name.
	segment := "[^\\s/\\\\\"'`|&;()<>*?\\[\\]{}$]*"
	return &ReferenceRule{
		pathPattern: regexp.MustCompile("[/\\\\]" + segment + weftSuffix + nameEnd),
		barePattern: regexp.MustCompile(`\S*` + weftSuffix + nameEnd),
	}
}

// MatchPath reports the sibling-name reference in cmd, if any.
// A separator-prefixed name ending in the suffix is found with quotes intact, then a bare word ending in the suffix on a copy with quoted spans blanked.
// The returned text comes from the copy the pattern ran on, without its trailing separator.
func (r *ReferenceRule) MatchPath(cmd string) (string, bool) {
	cmd = stripHeredocBodies(cmd)
	if m := r.pathPattern.FindString(cmd); m != "" {
		return strings.TrimRight(m, referenceTrim), true
	}
	if m := r.barePattern.FindString(blankQuoted(cmd)); m != "" {
		return strings.TrimRight(m, referenceTrim), true
	}
	return "", false
}

// MatchSpelling reports the fabric-driving command spelling in cmd, if any, found on a copy with heredoc bodies stripped and quoted spans blanked.
// The returned text comes from that copy, without the separator that precedes the spelling.
func (r *ReferenceRule) MatchSpelling(cmd string) (string, bool) {
	m := spellingPattern.FindString(blankQuoted(stripHeredocBodies(cmd)))
	if m == "" {
		return "", false
	}
	return strings.TrimSpace(strings.TrimLeft(m, ";&|(\n`$")), true
}

// RefScanner detects a command that references fabric's two-checkout mechanism.
// That is a fabric-driving command spelling in command position, a path touching a weft sibling worktree, or the worktree's own sibling path spelled out exactly.
// The first two are ReferenceRule's; only the exact path needs the worktree.
// Construct via NewRefScanner; the zero value is not valid.
type RefScanner struct {
	rule         *ReferenceRule
	exactPattern *regexp.Regexp
}

// NewRefScanner returns a RefScanner for l's worktree, compiling its regexes once so repeated
// Matches calls (e.g. over every Bash command in a transcript) never recompile them.
func NewRefScanner(l *lyxcwd.Location) *RefScanner {
	return &RefScanner{
		rule:         NewReferenceRule(),
		exactPattern: regexp.MustCompile(regexp.QuoteMeta(WeftWorktree(l))),
	}
}

// Matches reports whether cmd references fabric's two-checkout mechanism, either by spelling
// (lyx fabric/weft/warp, in command position) or by touching a weft sibling worktree's path.
func (s *RefScanner) Matches(cmd string) bool {
	if s.exactPattern.MatchString(stripHeredocBodies(cmd)) {
		return true
	}
	if _, ok := s.rule.MatchPath(cmd); ok {
		return true
	}
	_, ok := s.rule.MatchSpelling(cmd)
	return ok
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
