// refscanner.go implements RefScanner, fabric's answer to "does this command reference fabric's
// two-checkout mechanism" — the audit policy a consumer like websterengine needs without ever
// holding the weft path or the command-spelling pattern itself.

package fabricengine

import (
	"regexp"
	"slices"
	"strings"
	"unicode"

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
		exactPattern: regexp.MustCompile(regexp.QuoteMeta(RecordsWorktree(l))),
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
	return blankQuotedSpans(cmd, false)
}

// blankQuotedSpans is blankQuoted's scan.
// With singleOnly set it blanks single-quoted spans only and keeps double-quoted ones, since a shell runs a substitution inside double quotes but never inside single quotes.
func blankQuotedSpans(cmd string, singleOnly bool) string {
	var b strings.Builder
	b.Grow(len(cmd))
	var quote rune
	escaped := false
	for _, r := range cmd {
		before := quote
		keep := false
		switch {
		case escaped:
			escaped = false
			keep = quote == 0
		case r == '\\' && quote != '\'':
			escaped = true
			keep = quote == 0
		case quote == 0 && (r == '\'' || r == '"'):
			quote = r
		case r == quote:
			quote = 0
		default:
			keep = quote == 0
		}
		if singleOnly && (before == '"' || quote == '"') {
			keep = true
		}
		if keep {
			b.WriteRune(r)
		} else {
			b.WriteByte(' ')
		}
	}
	return b.String()
}

// readOnlyCommands is the closed list of readers a read-only command is built from.
var readOnlyCommands = map[string]bool{
	"cat": true, "head": true, "tail": true, "ls": true, "wc": true, "grep": true, "jq": true, "cut": true,
}

// FabricReaderVerbs returns the names of the `lyx fabric` verbs that only read: list, pairs, status, diff, shortname, prune and cleanup.
// It declares names only; the arguments each verb may take without mutating stay with the consumer that applies them.
// Every call returns a fresh slice, so no caller can change another's set.
func FabricReaderVerbs() []string {
	return []string{"list", "pairs", "status", "diff", "shortname", "prune", "cleanup"}
}

// substitutionMarkers are the spellings of command and process substitution.
var substitutionMarkers = []string{"$(", "`", "<(", ">("}

// IsReadOnlyCommand reports whether cmd is built only from allow-listed read forms, each in command position, joined by `|`, `||`, `;`, `&&` or a newline.
// The forms are the readers cat, head, tail, ls, wc, grep, jq and cut; `lyx` help (command words then `--help` or `-h`); the `lyx fabric` readers list, pairs, status, shortname, diff with one revision, prune and cleanup without flags; and a closed set of `git` reads, optionally behind `-C <dir>`.
// It is false for any output redirection (`>`, `>>`, `>|`, `&>`, a numbered descriptor redirection, `<>`), for command or process substitution, for backgrounding, and for any other command or form, `sort`, `tee`, `find`, `env`, `xargs`, an interpreter, `go` and every other `lyx` or `git` invocation included.
// A `lyx`, `git` or `sed` segment is split into shell words, honouring single and double quotes, and judged on the dequoted text.
// Such a word may hold neither `$` nor a backslash, quoted or not, nor an unquoted brace or parenthesis; a leading `~` is text.
// A quoted word starting with `-` is still a flag, so a quoted writing flag is rejected, and a quoted glob character is text wherever a positional is admitted.
// An unquoted glob character (`*`, `?`, `[`, `#`, or `^` at a word's start) is rejected in every `lyx` word, and in every `git` word except after the `--` of `log`, `show`, `diff` and `status`, where every word is a pathspec.
// `sed` is read-only only as `-n`, one line number or range of two line numbers followed by `p`, and file words.
// The two stderr redirections `2>&1` and `2>/dev/null`, each a whole token that does not open its segment, are dropped before the redirection scan; every other redirection fails it.
// A separator or redirection character inside a quoted span is text.
// A substitution is found with only single-quoted spans blanked, because a shell runs one inside double quotes.
// A command word is read in its original spelling, so a first word holding a quote or a backslash, quoted whole or only in part, is never read-only: it is not a name the list can match.
// A trailing `--json` is a help form of a `lyx` segment, except for a command that declares a `--json` of its own, local or persistent on a group, which shadows the global flag and runs.
// The classifier does not know that set; a test over the real command tree pins it, so a new one is a reviewed change against this rule.
// Today those are the generic shed `status` verbs under each mount, which only read.
// The classifier assumes bash or zsh with any glob option set and sees static shape only, so a loop, an expansion, a substitution, a zsh glob qualifier, a bash extglob and a command behind `bash -c` stay non-read-only.
func IsReadOnlyCommand(cmd string) bool {
	cmd = strings.TrimSpace(cmd)
	if cmd == "" {
		return false
	}
	substitutionView := blankQuotedSpans(cmd, true)
	for _, marker := range substitutionMarkers {
		if strings.Contains(substitutionView, marker) {
			return false
		}
	}
	cmd = dropStderrRedirections(cmd)
	unquoted := blankQuoted(cmd)
	if strings.Contains(unquoted, ">") {
		return false
	}
	// Each replacement keeps its length, so the separated copy stays rune-aligned with cmd.
	separated := strings.NewReplacer("&&", " ;", "||", " ;", "\n", ";", "|", ";").Replace(unquoted)
	if strings.Contains(separated, "&") {
		return false
	}
	original, hidden := []rune(cmd), []rune(separated)
	segmentStart := 0
	for i := 0; i <= len(hidden); i++ {
		if i < len(hidden) && hidden[i] != ';' {
			continue
		}
		if !readOnlySegment(string(original[segmentStart:i])) {
			return false
		}
		segmentStart = i + 1
	}
	return true
}

// stderrRedirections are the two redirections of standard error that write nothing a reader could change.
var stderrRedirections = []string{"2>&1", "2>/dev/null"}

// dropStderrRedirections blanks, in cmd, each whitespace-delimited unquoted token spelled exactly as one of stderrRedirections that does not open its segment.
// The blanks keep cmd's length in runes, so the result stays aligned with a quote-blanked copy of itself.
func dropStderrRedirections(cmd string) string {
	original, view := []rune(cmd), []rune(blankQuoted(cmd))
	for i := 0; i < len(view); {
		if unicode.IsSpace(view[i]) {
			i++
			continue
		}
		start := i
		for i < len(view) && !unicode.IsSpace(view[i]) {
			i++
		}
		// A real space must border the token, since a blanked quote also reads as a space in view.
		bordered := (start == 0 || unicode.IsSpace(original[start-1])) && (i == len(original) || unicode.IsSpace(original[i]))
		if bordered && slices.Contains(stderrRedirections, string(view[start:i])) && !opensSegment(view, start) {
			for j := start; j < i; j++ {
				original[j] = ' '
			}
		}
	}
	return string(original)
}

// opensSegment reports whether only blanks separate view[at] from the start of the command or a segment separator.
func opensSegment(view []rune, at int) bool {
	for j := at - 1; j >= 0; j-- {
		switch {
		case strings.ContainsRune(";&|\n", view[j]):
			return true
		case !unicode.IsSpace(view[j]):
			return false
		}
	}
	return true
}

// readOnlySegment reports whether one separated segment, in its raw text, is a listed reader, a read-only lyx form, a read-only git form or a read-only sed print.
func readOnlySegment(segment string) bool {
	fields := strings.Fields(segment)
	if len(fields) == 0 {
		return false
	}
	if readOnlyCommands[fields[0]] {
		return true
	}
	if fields[0] != "lyx" && fields[0] != "git" && fields[0] != "sed" {
		return false
	}
	words, ok := splitShellWords(segment)
	if !ok {
		return false
	}
	switch fields[0] {
	case "lyx":
		return readOnlyLyxSegment(words)
	case "git":
		return readOnlyGitSegment(words)
	}
	return readOnlySedSegment(words)
}

// shellWord is one word of a segment after the shell's quote removal.
type shellWord struct {
	// text is the word with its quotes removed.
	text string
	// quoted is whether any part of the word was quoted.
	quoted bool
	// glob is whether the word holds an unquoted glob character: `*`, `?`, `[` or `#` anywhere, or `^` at its start, which zsh's extended globbing reads as a glob.
	glob bool
}

// splitShellWords splits one segment into shellWords, honouring single and double quotes, with adjacent quoted and unquoted parts joining into one word.
// It reports false when any word holds `$` or a backslash, quoted or not, an unquoted brace or parenthesis, or an unterminated quote, since the shell would expand or reinterpret it.
func splitShellWords(segment string) ([]shellWord, bool) {
	var words []shellWord
	var current strings.Builder
	var word shellWord
	var quote rune
	inWord := false
	for _, r := range segment {
		if r == '$' || r == '\\' {
			return nil, false
		}
		switch {
		case quote != 0:
			if r == quote {
				quote = 0
			} else {
				current.WriteRune(r)
			}
		case r == '\'' || r == '"':
			quote, inWord, word.quoted = r, true, true
		case unicode.IsSpace(r):
			if inWord {
				word.text = current.String()
				words = append(words, word)
				current.Reset()
				word, inWord = shellWord{}, false
			}
		case strings.ContainsRune("{}()", r):
			return nil, false
		default:
			if strings.ContainsRune("*?[#", r) || (r == '^' && !inWord) {
				word.glob = true
			}
			inWord = true
			current.WriteRune(r)
		}
	}
	if quote != 0 {
		return nil, false
	}
	if inWord {
		word.text = current.String()
		words = append(words, word)
	}
	return words, true
}

// holdsGlobCharacter reports whether text holds a glob character, wherever it came from.
func holdsGlobCharacter(text string) bool {
	return strings.ContainsAny(text, "*?[#") || strings.HasPrefix(text, "^")
}

// lyxCommandWord matches a cobra verb name: lowercase letters with inner hyphens.
var lyxCommandWord = regexp.MustCompile(`^[a-z]+(-[a-z]+)*$`)

// isLyxCommandWord reports whether word is an unquoted cobra verb name.
func isLyxCommandWord(word shellWord) bool {
	return !word.quoted && lyxCommandWord.MatchString(word.text)
}

// readOnlyLyxSegment reports whether words, the words of one `lyx` segment, are a help form or a `lyx fabric` reader.
// The help form is command words then `--help` or `-h` as the last word, or command words alone then `--json` as the last word, or `--help` or `-h` then `--json`, since the global `--json` raises help.
// A fabric reader is one of FabricReaderVerbs under the argument rule that keeps it from mutating: `diff` takes exactly one word not starting with `-`, and the rest take none.
// A word holding a glob character, quoted or not, rejects the segment.
func readOnlyLyxSegment(words []shellWord) bool {
	if slices.ContainsFunc(words, func(word shellWord) bool { return word.glob || holdsGlobCharacter(word.text) }) {
		return false
	}
	rest := words[1:]
	trailingJSON := len(rest) > 0 && rest[len(rest)-1].text == "--json"
	if trailingJSON {
		rest = rest[:len(rest)-1]
	}
	allCommandWords := func(candidates []shellWord) bool {
		return !slices.ContainsFunc(candidates, func(word shellWord) bool { return !isLyxCommandWord(word) })
	}
	if len(rest) > 0 {
		last := rest[len(rest)-1].text
		if last == "--help" || last == "-h" {
			return allCommandWords(rest[:len(rest)-1])
		}
	}
	if trailingJSON {
		return allCommandWords(rest)
	}
	if len(rest) < 2 || !isLyxCommandWord(rest[0]) || rest[0].text != "fabric" || !isLyxCommandWord(rest[1]) || !slices.Contains(FabricReaderVerbs(), rest[1].text) {
		return false
	}
	args := rest[2:]
	if rest[1].text == "diff" {
		return len(args) == 1 && !strings.HasPrefix(args[0].text, "-")
	}
	return len(args) == 0
}

// sedPrintScript matches a print of one line or of a range of two lines.
var sedPrintScript = regexp.MustCompile(`^[0-9]+(,[0-9]+)?p$`)

// readOnlySedSegment reports whether words, the words of one `sed` segment, are `-n`, a print script of a line or a line range, then file words.
// A file word may not start with `-` or hold an unquoted glob character, so no flag can follow the script; with no file word, sed prints its standard input.
func readOnlySedSegment(words []shellWord) bool {
	if len(words) < 3 || words[1].text != "-n" || !sedPrintScript.MatchString(words[2].text) {
		return false
	}
	return !slices.ContainsFunc(words[3:], func(word shellWord) bool { return strings.HasPrefix(word.text, "-") || word.glob })
}

// gitForm is the shape of one read-only git subcommand's words after the subcommand.
type gitForm struct {
	// flags are the words starting with `-` the form admits before a `--`, matched exactly.
	flags map[string]bool
	// digitsFlag is a flag whose next word is its all-digit value, `-n`.
	digitsFlag string
	// anyDigitsFlag admits a `-<digits>` word as a flag.
	anyDigitsFlag bool
	// flagPrefixes admit a word starting with one of them, `--format=`.
	flagPrefixes []string
	// globAfterDoubleDash admits an unquoted glob character in a word after `--`, where the form reads every word as a pathspec.
	globAfterDoubleDash bool
}

// gitFlagSet returns the set of the given words.
func gitFlagSet(words ...string) map[string]bool {
	set := make(map[string]bool, len(words))
	for _, word := range words {
		set[word] = true
	}
	return set
}

// historyForm is the shape shared by `git log` and `git show`.
var historyForm = gitForm{
	flags:         gitFlagSet("--oneline", "--stat", "--name-only", "--name-status", "--graph", "--decorate", "--all", "-s", "--no-patch"),
	digitsFlag:    "-n",
	anyDigitsFlag: true,
	flagPrefixes:  []string{"--format=", "--pretty="},

	globAfterDoubleDash: true,
}

// gitForms are the read-only git subcommands that take flags and then free positionals.
var gitForms = map[string]gitForm{
	"ls-remote": {flags: gitFlagSet("--heads", "--tags", "--refs", "-h", "-t")},
	"rev-parse": {flags: gitFlagSet("--abbrev-ref", "--short", "--verify", "-q", "--show-toplevel", "--git-dir")},
	"log":       historyForm,
	"show":      historyForm,
	"diff":      {flags: gitFlagSet("--stat", "--name-only", "--name-status", "--cached", "--staged", "--no-color"), globAfterDoubleDash: true},
	"status":    {flags: gitFlagSet("-s", "--short", "--porcelain", "-b", "--branch"), globAfterDoubleDash: true},
}

// admits reports whether args, the words after a subcommand, are made only of flags the form lists and positionals; every word after a `--` is a positional.
// A word with an unquoted glob character is rejected before the `--`, and after it unless the form admits one there.
func (f gitForm) admits(args []shellWord) bool {
	for i := 0; i < len(args); i++ {
		word := args[i]
		text := word.text
		switch {
		case text == "--":
			return f.globAfterDoubleDash || !slices.ContainsFunc(args[i+1:], func(w shellWord) bool { return w.glob })
		case word.glob:
			return false
		case !strings.HasPrefix(text, "-"):
		case f.flags[text]:
		case text == f.digitsFlag && f.digitsFlag != "":
			i++
			if i >= len(args) || !allDigits(args[i].text) {
				return false
			}
		case f.anyDigitsFlag && allDigits(text[1:]):
		case slices.ContainsFunc(f.flagPrefixes, func(prefix string) bool { return strings.HasPrefix(text, prefix) }):
		default:
			return false
		}
	}
	return true
}

// allDigits reports whether s is one or more ASCII digits.
func allDigits(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

// readOnlyGitSegment reports whether words, the words of one `git` segment, are a listed read form.
// The only global option admitted is `-C <dir>`, both words unquoted and the directory free of a glob character.
// Every word starting with `-` before a `--` must be a flag the subcommand's form lists, matched exactly, and every other word a positional the form admits.
// The subcommand word is unquoted, and so is each word that selects a `remote` or `branch` form.
// A word the form does not judge, in `cat-file` and `config`, rejects an unquoted glob character, since the shell would expand it into names that could spell a flag.
func readOnlyGitSegment(words []shellWord) bool {
	rest := words[1:]
	if len(rest) >= 2 && rest[0].text == "-C" && !rest[0].quoted {
		dir := rest[1]
		if dir.quoted || dir.glob || strings.HasPrefix(dir.text, "-") {
			return false
		}
		rest = rest[2:]
	}
	if len(rest) == 0 || rest[0].quoted {
		return false
	}
	subcommand, args := rest[0].text, rest[1:]
	if form, ok := gitForms[subcommand]; ok {
		return form.admits(args)
	}
	switch subcommand {
	case "remote":
		return readOnlyGitRemote(args)
	case "branch":
		return readOnlyGitBranch(args)
	case "cat-file":
		return len(args) == 2 && slices.Contains([]string{"-t", "-s", "-e", "-p"}, args[0].text) && !args[0].glob &&
			!strings.HasPrefix(args[1].text, "-") && !args[1].glob
	case "config":
		return len(args) >= 2 && len(args) <= 3 && slices.Contains([]string{"--get", "--get-regexp"}, args[0].text) && !args[0].glob &&
			!slices.ContainsFunc(args[1:], func(word shellWord) bool { return strings.HasPrefix(word.text, "-") || word.glob })
	}
	return false
}

// readOnlyGitRemote reports whether args, the words after `git remote`, are `-v` alone, `get-url` with `--push` or `--all` then one name, or `show` with `-n` then names.
func readOnlyGitRemote(args []shellWord) bool {
	if len(args) == 1 && args[0].text == "-v" {
		return true
	}
	if len(args) == 0 || args[0].quoted {
		return false
	}
	switch args[0].text {
	case "get-url":
		return gitForm{flags: gitFlagSet("--push", "--all")}.admits(args[1:]) && countPositionals(args[1:]) == 1
	case "show":
		return gitForm{flags: gitFlagSet("-n")}.admits(args[1:])
	}
	return false
}

// readOnlyGitBranch reports whether args, the words after `git branch`, are listing flags with no positional, or `--list` then patterns.
// A pattern may hold a quoted glob character, which is text, but not an unquoted one.
func readOnlyGitBranch(args []shellWord) bool {
	if len(args) > 0 && args[0].text == "--list" {
		return !args[0].quoted && !slices.ContainsFunc(args[1:], func(word shellWord) bool { return strings.HasPrefix(word.text, "-") || word.glob })
	}
	listing := gitFlagSet("-a", "--all", "-r", "--remotes", "-v", "-vv", "--show-current")
	return !slices.ContainsFunc(args, func(word shellWord) bool { return !listing[word.text] })
}

// countPositionals returns how many words do not start with `-`.
func countPositionals(words []shellWord) int {
	count := 0
	for _, word := range words {
		if !strings.HasPrefix(word.text, "-") {
			count++
		}
	}
	return count
}
