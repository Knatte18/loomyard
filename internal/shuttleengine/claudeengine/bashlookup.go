// bashlookup.go classes a Bash tool call's command as a read-only lookup or not, for the batching nudge's single-lookup chain:
// a quote-aware scan splits the command into its pipe and chain segments, and a table keyed by command name judges each segment.

package claudeengine

import (
	"slices"
	"strings"
)

// lookupCommands maps each command a read-only lookup may run to the check its arguments must pass.
var lookupCommands = map[string]func(args []string) bool{
	"grep": anyArgs,
	"rg":   anyArgs,
	"ls":   anyArgs,
	"cat":  anyArgs,
	"head": anyArgs,
	"tail": anyArgs,
	"wc":   anyArgs,
	"find": findReadsOnly,
	"sed":  sedPrintsOnly,
	"git":  gitReads,
}

// findWritingActions are the find actions that run a command, delete a file or write one.
var findWritingActions = []string{"-exec", "-execdir", "-ok", "-okdir", "-delete", "-fprint", "-fprint0", "-fprintf", "-fls"}

// gitReadSubcommands are the git subcommands that only read the repository.
var gitReadSubcommands = []string{"log", "show", "diff", "status", "blame", "grep", "ls-files", "ls-tree", "rev-parse", "cat-file"}

// discardTarget is the one file a lookup's output redirect may name.
const discardTarget = "/dev/null"

// isReadOnlyLookup reports whether command is a read-only lookup:
// one or more segments joined by a pipe, `;`, `&&`, `||` or a newline, each running a command of lookupCommands with arguments its check accepts.
// A command substitution, a process substitution, a subshell, a heredoc, a background `&`, an output redirect to any file but /dev/null, an unterminated quote and an empty command are no lookup.
// The scan reads static text and is a heuristic: an argument a listed command treats as a program, such as rg's --pre, passes, while a git read with an option that writes a file or runs a program is no lookup.
func isReadOnlyLookup(command string) bool {
	segments, ok := splitShellSegments(command)
	if !ok {
		return false
	}
	found := false
	for _, words := range segments {
		if len(words) == 0 {
			continue
		}
		check, listed := lookupCommands[words[0]]
		if !listed || !check(words[1:]) {
			return false
		}
		found = true
	}
	return found
}

// anyArgs accepts every argument list.
func anyArgs([]string) bool { return true }

// findReadsOnly reports whether a find call's args hold none of findWritingActions.
func findReadsOnly(args []string) bool {
	return !slices.ContainsFunc(args, func(arg string) bool { return slices.Contains(findWritingActions, arg) })
}

// sedPrintsOnly reports whether a sed call's args turn off automatic printing and never edit in place.
func sedPrintsOnly(args []string) bool {
	quiet := false
	for _, arg := range args {
		short := strings.HasPrefix(arg, "-") && !strings.HasPrefix(arg, "--")
		switch {
		case strings.HasPrefix(arg, "--in-place"), short && strings.Contains(arg, "i"):
			return false
		case arg == "--quiet", arg == "--silent", short && strings.Contains(arg, "n"):
			quiet = true
		}
	}
	return quiet
}

// gitReads reports whether a git call's args, past the global options -C <dir> and --no-pager, name one of gitReadSubcommands.
// Any other global option is no read, since -c can set a program git runs.
// A subcommand argument that writes a file or runs a program, as gitWritingOption names, is no read either.
func gitReads(args []string) bool {
	for len(args) > 0 {
		switch args[0] {
		case "-C":
			if len(args) < 2 {
				return false
			}
			args = args[2:]
		case "--no-pager":
			args = args[1:]
		default:
			return slices.Contains(gitReadSubcommands, args[0]) && !slices.ContainsFunc(args[1:], gitWritingOption)
		}
	}
	return false
}

// gitWritingLongOptions are the long options of a git read that write a file or run a program.
var gitWritingLongOptions = []string{"--output", "--ext-diff", "--open-files-in-pager"}

// gitWritingOption reports whether a git subcommand argument writes a file or runs a program:
// a long option of gitWritingLongOptions, written in full or as the abbreviation of four or more characters that git also accepts, or any option naming -O.
// A short-option bundle holding an O counts, since -O takes the rest of its argument as the pager command.
func gitWritingOption(arg string) bool {
	if !strings.HasPrefix(arg, "-") {
		return false
	}
	if !strings.HasPrefix(arg, "--") {
		return strings.Contains(arg, "O")
	}
	name, _, _ := strings.Cut(arg, "=")
	if strings.HasPrefix(name, "--output") {
		return true
	}
	return len(name) >= 4 && slices.ContainsFunc(gitWritingLongOptions, func(long string) bool { return strings.HasPrefix(long, name) })
}

// splitShellSegments splits command into its segments, each its list of words with quotes removed and redirects to /dev/null dropped.
// It reports false for a construct isReadOnlyLookup refuses outright.
func splitShellSegments(command string) ([][]string, bool) {
	runes := []rune(command)
	var segments [][]string
	var words []string
	var word strings.Builder
	inWord := false
	endWord := func() {
		if inWord {
			words = append(words, word.String())
			word.Reset()
			inWord = false
		}
	}
	endSegment := func() {
		endWord()
		segments = append(segments, words)
		words = nil
	}
	next := func(i int) rune {
		if i+1 < len(runes) {
			return runes[i+1]
		}
		return 0
	}
	for i := 0; i < len(runes); i++ {
		r := runes[i]
		switch {
		case r == '\\':
			if i+1 < len(runes) {
				i++
				if runes[i] != '\n' {
					word.WriteRune(runes[i])
					inWord = true
				}
			}
		case r == '\'':
			end := slices.Index(runes[i+1:], '\'')
			if end < 0 {
				return nil, false
			}
			word.WriteString(string(runes[i+1 : i+1+end]))
			inWord = true
			i += end + 1
		case r == '"':
			end, ok := scanDoubleQuoted(runes, i+1, &word)
			if !ok {
				return nil, false
			}
			inWord = true
			i = end
		case r == '`', r == '(', r == ')', r == '$' && next(i) == '(':
			return nil, false
		case r == ' ', r == '\t':
			endWord()
		case r == '\n', r == ';':
			endSegment()
		case r == '|':
			if next(i) == '|' || next(i) == '&' {
				i++
			}
			endSegment()
		case r == '&':
			switch next(i) {
			case '&':
				i++
				endSegment()
			case '>':
				endWord()
				end, ok := skipDiscardRedirect(runes, i+1)
				if !ok {
					return nil, false
				}
				i = end
			default:
				return nil, false
			}
		case r == '>':
			if inWord && strings.Trim(word.String(), "0123456789") == "" {
				word.Reset()
				inWord = false
			}
			endWord()
			end, ok := skipDiscardRedirect(runes, i)
			if !ok {
				return nil, false
			}
			i = end
		case r == '<':
			if next(i) == '(' || next(i) == '<' {
				return nil, false
			}
			endWord()
		default:
			word.WriteRune(r)
			inWord = true
		}
	}
	endSegment()
	return segments, true
}

// scanDoubleQuoted appends to word the text of the double-quoted string that starts at runes[start], and returns the index of its closing quote.
// It reports false for an unterminated string or one holding a command substitution.
func scanDoubleQuoted(runes []rune, start int, word *strings.Builder) (int, bool) {
	for i := start; i < len(runes); i++ {
		switch {
		case runes[i] == '"':
			return i, true
		case runes[i] == '`', runes[i] == '$' && i+1 < len(runes) && runes[i+1] == '(':
			return 0, false
		case runes[i] == '\\' && i+1 < len(runes):
			i++
		}
		word.WriteRune(runes[i])
	}
	return 0, false
}

// skipDiscardRedirect reads the output redirect whose first '>' is runes[at] and returns the index of its last rune.
// It reports false unless the redirect duplicates a descriptor, as in >&2, or names discardTarget.
func skipDiscardRedirect(runes []rune, at int) (int, bool) {
	i := at + 1
	if i < len(runes) && runes[i] == '>' {
		i++
	}
	if i < len(runes) && runes[i] == '&' {
		digits := i + 1
		for digits < len(runes) && runes[digits] >= '0' && runes[digits] <= '9' {
			digits++
		}
		return digits - 1, digits > i+1
	}
	for i < len(runes) && (runes[i] == ' ' || runes[i] == '\t') {
		i++
	}
	end := i
	for end < len(runes) && !strings.ContainsRune(" \t\n;|&", runes[end]) {
		end++
	}
	return end - 1, string(runes[i:end]) == discardTarget
}
