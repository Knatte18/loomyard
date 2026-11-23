// rows.go declares the shell guard rows, the patterns they match and the steer and notice text each carries, and the init-time check on that text.

package shellguard

import (
	"fmt"
	"maps"
	"slices"
	"strings"

	"github.com/Knatte18/loomyard/internal/agentname"
	"github.com/Knatte18/loomyard/internal/gateslot"
)

// Switch names the config switch a row hangs on.
type Switch string

const (
	// SwitchNone marks a row installed for every run.
	SwitchNone Switch = ""
	// SwitchDenyPython marks the row installed for every run.
	// Its hook asks the hub's current deny_python on each matching call.
	// Its notice follows deny_python at launch.
	SwitchDenyPython Switch = "deny_python"
	// SwitchDenyPublish marks the row a launch spec turns on to keep a session from publishing: git push and the gh calls beyond reads, comments and title and body edits.
	SwitchDenyPublish Switch = "deny_publish"
	// SwitchDenyRawTmux marks a row installed and announced for every run whose launch spec does not exempt itself.
	// A spec exempts itself when its session's task is to start tmux servers of its own.
	SwitchDenyRawTmux Switch = "deny_raw_tmux"
)

// Row is one guard against shell commands.
type Row struct {
	// Name identifies the row.
	Name string
	// Patterns are grep -E patterns; the guard denies when every one of them matches the payload.
	Patterns []string
	// Steer is the deny reason shown to the agent.
	Steer string
	// Notice is the system-prompt sentence announcing the guard.
	Notice string
	// Switch is the config switch the row hangs on.
	Switch Switch
	// Forms are the literal command forms the row denies, each a whole token sequence, for the no-denied-recovery scan; empty for a pattern-shaped row.
	Forms [][]string
	// EngineTexts are the row's own steer and notice for an engine whose tools the plain text names wrongly, keyed by the engine's registry name.
	EngineTexts map[string]EngineText
}

// EngineText is one engine's steer and notice for a row; an empty field takes the row's plain text.
type EngineText struct {
	Steer  string
	Notice string
}

// SteerFor returns the deny reason shown to an agent on the named engine: the engine's own steer when the row carries one, otherwise the plain steer.
func (row Row) SteerFor(engine string) string {
	if text := row.EngineTexts[engine].Steer; text != "" {
		return text
	}
	return row.Steer
}

// NoticeFor returns the system-prompt sentence announcing the row to an agent on the named engine: the engine's own notice when the row carries one, otherwise the plain notice.
func (row Row) NoticeFor(engine string) string {
	if text := row.EngineTexts[engine].Notice; text != "" {
		return text
	}
	return row.Notice
}

// engineCodex is the registry name of the Codex engine, which the rows' Codex texts are keyed by.
const engineCodex = "codex"

// editToolsRule is the way an agent changes and reads files, which the steers of the python and shell-edit rows carry and only the shell-edit row's notice states, so the system prompt states it once.
const editToolsRule = "change a file by reading it with Read, whole or a span through offset and limit, and then editing it with Edit, all edits of one file in one message, or write a whole file with Write; search files with grep"

// codexEditToolsRule is the way an agent on Codex changes and reads files: Codex has no Read, Edit or Write tool and changes a file through its apply_patch tool.
// The Codex steers and notice carry it where the plain ones carry editToolsRule.
const codexEditToolsRule = "change a file with the apply_patch tool, and read or search files with cat, sed -n, rg or grep"

// pythonRefusal states the python deny, why it stands and the way forward for work that needs Python itself.
// It names the version check, which agents otherwise run as a probe before choosing how to edit or parse.
const pythonRefusal = "python and python3 are refused in every lym session, a version check included, so never probe for Python: what it runs differs from machine to machine with the installed packages, venvs and installations, and it is often very slow; when the work itself needs Python, as OCR does, say so to the operator or your parent, who can set " + string(SwitchDenyPython) + ": false for the hub, and never work round the guard"

// pythonRoutes opens the python row's own steer text, which follows the shared whole-command clause.
// It holds the refusal and the parse routes in Python's place.
const pythonRoutes = pythonRefusal + "; for work that does not need Python itself, parse JSON with jq and text with grep or awk; "

// steerPythonDeny refuses a Bash python invocation and names the parse and edit routes in its place; it must contain no single/double quote or backslash (checked at init).
const steerPythonDeny = pythonRoutes + editToolsRule

// codexSteerPythonDeny is steerPythonDeny with the Codex edit route.
const codexSteerPythonDeny = pythonRoutes + codexEditToolsRule

// noticePythonDeny announces the python deny; the edit rule its steer carries is announced by the shell-edit row's notice.
const noticePythonDeny = pythonRefusal + "."

// shellEditRefusal opens the shell-edit row's own steer text, which follows the shared whole-command clause.
// It names the commands the row refuses.
const shellEditRefusal = "sed -i, perl -i, and a heredoc, redirect or tee into a source file are refused here; "

// steerShellEditDeny refuses a Bash edit of a file; it must contain no single/double quote or backslash (checked at init).
const steerShellEditDeny = shellEditRefusal + editToolsRule

// noticeShellEditDeny announces the shell-edit deny in the same wording as its steer.
const noticeShellEditDeny = steerShellEditDeny + "."

// codexSteerShellEditDeny is steerShellEditDeny with the Codex edit route.
const codexSteerShellEditDeny = shellEditRefusal + codexEditToolsRule

// commandFieldPattern is the grep -E pattern matching the opening of the "command" key's string value in the raw PreToolUse payload JSON.
const commandFieldPattern = `"command"[[:space:]]*:[[:space:]]*"`

// doubleQuotedOpenPattern matches the opening JSON-escaped double quote of a shell double-quoted string and the characters inside it: anything but a JSON escape of a double quote, which closes it.
const doubleQuotedOpenPattern = `\\"([^"\\]|\\[^"])*`

// shellUnitPattern matches one unit of a shell command as JSON encodes it, outside any quoting:
// an unquoted character, a JSON escape pair other than an escaped double quote, a single-quoted string, a double-quoted string, or a < that opens no heredoc and no process substitution.
// A heredoc operator, a lone backslash and the JSON string's closing quote match no unit, so a run of units stops before them.
const shellUnitPattern = `([^"\\'<]|\\[^"]|'[^']*'|` + doubleQuotedOpenPattern + `\\"|<[^<"\\'(])`

// shellLineUnitPattern matches what shellUnitPattern matches except a JSON-escaped newline, so a run of it stays on one line.
const shellLineUnitPattern = `([^"\\'<]|\\[^"n]|'[^']*'|` + doubleQuotedOpenPattern + `\\"|<[^<"\\'(])`

// commandSeparatorPattern matches what ends one command and puts the next word in command position:
// a ; & | ( <( >( or backtick, which also covers $( and ( subshells, or a JSON-escaped newline.
const commandSeparatorPattern = `([;&|` + "`" + `]|[<>]?\(|\\n)`

// topLevelReachPattern matches a run of shell units from the start of the command up to a separator, or nothing.
// A separator after a heredoc operator counts only on that operator's line.
const topLevelReachPattern = `(` + shellUnitPattern + `*(` + commandSeparatorPattern + `|<<-?` + shellLineUnitPattern + `*([;&|` + "`" + `]|[<>]?\()))?`

// commandWordPrefixPattern matches what may stand between command position and the command word:
// whitespace, then any of env, exec, xargs, sudo and time and the shell keywords that open a command, then a path prefix.
// The keywords are do, then, else, if, elif, while and until.
const commandWordPrefixPattern = `[[:space:]]*((env|exec|xargs|sudo|time|do|then|else|if|elif|while|until)[[:space:]]+)*([^[:space:]"\\;&|()` + "`" + `]*/)?`

// shellDashCPattern matches a shell given its script as an argument and the whitespace before the script:
// sh, bash, dash, ksh or zsh, any flag words, and then -c alone or ending a cluster such as -lc or -ec.
const shellDashCPattern = `(ba|da|k|z)?sh([[:space:]]+-[A-Za-z-]+)*[[:space:]]+-[A-Za-z]*c[A-Za-z]*[[:space:]]+`

// singleQuotedScriptStringOpenPattern matches the opening of a double-quoted string in a single-quoted shell -c script.
// It matches the opening JSON-escaped double quote and the characters inside the string so far.
// A single quote closes the script, even inside that string.
const singleQuotedScriptStringOpenPattern = `\\"([^"\\']|\\[^"])*`

// singleQuotedScriptUnitPattern matches one unit of a single-quoted shell -c script as JSON encodes it:
// what shellUnitPattern matches bar a single-quoted string.
// A single quote closes the script, even inside a double-quoted string of the script.
const singleQuotedScriptUnitPattern = `([^"\\'<]|\\[^"]|` + singleQuotedScriptStringOpenPattern + `\\"|<[^<"\\'(])`

// singleQuotedScriptLineUnitPattern matches what singleQuotedScriptUnitPattern matches except a JSON-escaped newline.
// A run of it thus stays on one line.
const singleQuotedScriptLineUnitPattern = `([^"\\'<]|\\[^"n]|` + singleQuotedScriptStringOpenPattern + `\\"|<[^<"\\'(])`

// doubleQuotedScriptUnitPattern matches one unit of a double-quoted shell -c script as JSON encodes it:
// what shellUnitPattern matches bar a double-quoted string.
// A JSON-escaped double quote closes the script, even inside a single-quoted string of the script.
const doubleQuotedScriptUnitPattern = `([^"\\'<]|\\[^"]|'[^'"]*'|<[^<"\\'(])`

// doubleQuotedScriptLineUnitPattern matches what doubleQuotedScriptUnitPattern matches except a JSON-escaped newline.
// A run of it thus stays on one line.
const doubleQuotedScriptLineUnitPattern = `([^"\\'<]|\\[^"n]|'[^'"]*'|<[^<"\\'(])`

// singleQuotedScriptReachPattern matches a shell -c with a single-quoted script in command position.
// It matches the script up to a command position inside it:
// the script's start, or a separator reached through a run of the script's units.
// A separator after a heredoc operator counts only on that operator's line.
const singleQuotedScriptReachPattern = commandWordPrefixPattern + shellDashCPattern +
	`'(` + singleQuotedScriptUnitPattern + `*(` + commandSeparatorPattern + `|<<-?` + singleQuotedScriptLineUnitPattern + `*([;&|` + "`" + `]|[<>]?\()))?`

// doubleQuotedScriptReachPattern matches a shell -c with a double-quoted script in command position.
// It matches the script up to a command position inside it, as for a single-quoted script.
const doubleQuotedScriptReachPattern = commandWordPrefixPattern + shellDashCPattern +
	`\\"(` + doubleQuotedScriptUnitPattern + `*(` + commandSeparatorPattern + `|<<-?` + doubleQuotedScriptLineUnitPattern + `*([;&|` + "`" + `]|[<>]?\()))?`

// shellScriptReachPattern matches a shell -c in command position and its script up to a command position inside it.
// The script may be in either quote.
const shellScriptReachPattern = `(` + singleQuotedScriptReachPattern + `|` + doubleQuotedScriptReachPattern + `)`

// commandPositionPattern is the grep -E prefix that puts the pattern after it in command position of the raw PreToolUse payload JSON, with an optional path prefix.
// Command position is the start of the command or right after a separator.
// Any of env, exec, xargs, sudo, time and the shell keywords that open a command, such as do and then, may follow.
// The separator is reached through a run of shell units, and one after a heredoc operator counts only on that operator's line.
// The script of a shell -c in command position, single-quoted or double-quoted, is read the same way one level deep:
// its start and a separator inside it put the next word in command position.
// The guards thus refuse inside the script what they refuse outside it.
// One exception: a lym variable expanded inside a double-quoted string of a double-quoted script is not read.
// It sees static text only: it misreads a JSON-escaped double quote inside a double-quoted string and a quote inside a comment, and it sees nothing after a heredoc operator's line.
// A double-quoted string is one unit, so a command substitution inside it, as in out="$(python3 x.py)", is never in command position and passes.
// A < before a quote or a backslash, as in <"f" or <<<"f", ends the run of units, so a command after it passes.
// A script behind a nested shell -c, eval, find -exec or a flag of xargs hides its commands,
// and a lone single quote inside a double-quoted script hides the commands after it.
const commandPositionPattern = commandFieldPattern + topLevelReachPattern + `(` + shellScriptReachPattern + `)?` + commandWordPrefixPattern

// shellScope is one place a command can stand in the payload JSON.
// The places are the top level, a single-quoted shell -c script and a double-quoted one.
// It carries the place's own reading of a command's words.
// A row's scan over the words that follow a command word thus stops at the script's closing quote.
// It never runs on into a later command.
type shellScope struct {
	// position matches the payload up to a command word in the scope, a path prefix included.
	position string
	// quoted matches one quoted string a word of the scope can hold, as an alternation body.
	quoted string
	// quotedOpen matches the opening of such a string and the characters inside it so far.
	quotedOpen string
	// closing is the character a bare word of the scope excludes besides the shell's own.
	// It is the quote that closes a single-quoted script.
	closing string
	// escapeExcluded is the characters a JSON escape pair in the scope's text may not hold after its backslash.
	// It is in bracket syntax.
	escapeExcluded string
}

// shellScopes are the places a command can stand, in the order commandRowPattern renders them.
var shellScopes = []shellScope{
	{
		position:       commandFieldPattern + topLevelReachPattern + commandWordPrefixPattern,
		quoted:         `'[^']*'|` + doubleQuotedOpenPattern + `\\"`,
		quotedOpen:     `('[^']*|` + doubleQuotedOpenPattern + `)`,
		escapeExcluded: "n",
	},
	{
		position:       commandFieldPattern + topLevelReachPattern + singleQuotedScriptReachPattern + commandWordPrefixPattern,
		quoted:         singleQuotedScriptStringOpenPattern + `\\"`,
		quotedOpen:     singleQuotedScriptStringOpenPattern,
		closing:        "'",
		escapeExcluded: "n",
	},
	{
		position:       commandFieldPattern + topLevelReachPattern + doubleQuotedScriptReachPattern + commandWordPrefixPattern,
		quoted:         `'[^'"]*'`,
		quotedOpen:     `'[^'"]*`,
		escapeExcluded: `n"`,
	},
}

// argument returns a grep -E pattern matching one argument word of a command in the scope as JSON encodes it:
// unquoted characters and quoted strings run together.
// A backslash, a separator, a parenthesis, a redirect operator or a backtick ends the word.
// A run of arguments thus stays inside one command on one line.
func (scope shellScope) argument() string {
	return `([^[:space:]"\\;&|()'<>` + "`" + `]|` + scope.quoted + `)+`
}

// word returns a grep -E pattern matching the characters of one command-line word in the scope.
func (scope shellScope) word() string {
	return `[^[:space:]"\\;&|()` + scope.closing + "`" + `]*`
}

// goWordEnd returns a grep -E pattern matching what ends a word in the scope.
// It is a separator, whitespace, a quote the scope reads as a word end, or the end of the payload.
func (scope shellScope) goWordEnd() string {
	return `([[:space:]"\\;&|()` + scope.closing + "`" + `]|$)`
}

// goArguments returns a grep -E pattern matching the rest of a command line in the scope after a go subcommand.
// It ends on whitespace and takes anything up to the next separator, JSON-escaped newline or the end of the scope.
func (scope shellScope) goArguments() string {
	return `(([^;&|\\` + scope.closing + `]|\\[^` + scope.escapeExcluded + `])*[[:space:]])?`
}

// ghWordChars returns the bracket body of the characters one gh argument word holds in the scope.
func (scope shellScope) ghWordChars() string {
	return ghWordClass + scope.closing
}

// mutationScan returns a grep -E pattern matching the rest of a gh api command in the scope.
// The match runs up to a GraphQL mutation word.
// The scan stays on one command: it stops at an unquoted separator or newline.
// It crosses a newline only inside a quoted string, so a mutation on a later line of a multi-line query is found.
func (scope shellScope) mutationScan() string {
	return `([^;&|\\'"]|\\[^"n]|` + scope.quoted + `)*(mutation|` + scope.quotedOpen + `mutation)`
}

// commandRowPattern returns the grep -E alternation of one pattern per scope.
// Each is the scope's position followed by what build returns for the scope.
// A row's argument scans thus stay inside the script their command word was reached in.
// build returns a pattern that is grouped on its own when it holds a top-level alternation.
func commandRowPattern(build func(scope shellScope) string) string {
	alternatives := make([]string, len(shellScopes))
	for i, scope := range shellScopes {
		alternatives[i] = `(` + scope.position + build(scope) + `)`
	}
	return strings.Join(alternatives, "|")
}

// pythonCommandPattern is the grep -E pattern the python deny matches against the raw PreToolUse payload JSON.
// It matches python, python3 or python3.<minor>, optionally path-prefixed, as a whole word in command position.
const pythonCommandPattern = commandPositionPattern + `python(3(\.[0-9]+)?)?([^A-Za-z0-9_./-]|$)`

// steerComputedRmDeny refuses a Bash rm on a computed path; it must contain no single/double quote or backslash (checked at init).
// It names the literal-path way forward and no scratch place.
const steerComputedRmDeny = "rm on a computed path, one built from a variable or a command substitution, is refused here because it raises a permission prompt that stalls the run; remove a file by its literal path"

// noticeComputedRmDeny announces the computed-path rm deny in the same wording as its steer.
const noticeComputedRmDeny = steerComputedRmDeny + "."

// rmCommandPattern is the grep -E pattern matching rm as a whole word in command position of the raw PreToolUse payload JSON.
const rmCommandPattern = commandPositionPattern + `rm([^A-Za-z0-9_./-]|$)`

// shellExpansionPattern is the grep -E pattern matching a command substitution, $( or a backtick, or a shell variable expansion, $ followed by a letter, a digit, _, {, @ or *.
// The @ and * take in the positional parameters $@ and $*; the special parameters $$, $?, $# and $! are not matched.
const shellExpansionPattern = "[$][A-Za-z0-9_{@*(]|`"

// sourceFilePattern matches a file path word ending in a source file's extension, followed by a word end.
// It names Go, markdown, YAML, TOML, shell and .NET sources; a .json, .txt or .log target passes, so command output captured into a scratch file with one of those three extensions is not refused.
// A scratch file with a listed extension is written with Write.
const sourceFilePattern = `[^[:space:]"\\;&|()<>'` + "`" + `]*\.(go|mod|sum|md|ya?ml|toml|sh|ps1|cs|csproj|sln|props|targets)([^A-Za-z0-9_./-]|$)`

// inPlaceEditCommandPattern matches, in command position, sed with an in-place flag, perl with an in-place switch, or tee writing a source file.
// The flag may follow any argument words: sed takes -i, --in-place, or -i at the end of a cluster of its argument-free flags such as -Ei; perl takes -i at the end of a cluster of its argument-free switches such as -pi or -0777i.
// A cluster holding a switch that takes the rest of the word as its value, such as perl -Mstrict, is no in-place switch.
var inPlaceEditCommandPattern = commandRowPattern(func(scope shellScope) string {
	argument := scope.argument()
	return `(sed([[:space:]]+` + argument + `)*[[:space:]]+(-[nsuzEr]*i|--in-place)` +
		`|perl([[:space:]]+` + argument + `)*[[:space:]]+-[nlpaswcTtWX0-9]*i` +
		`|tee([[:space:]]+` + argument + `)*[[:space:]]+` + sourceFilePattern + `)`
})

// redirectWritePattern matches a > or >> redirect, a >| clobbering one included, into a source file, reached through a run of shell units from the start of the command.
// A redirect on a heredoc operator's line counts, as in cat <<EOF > f.go; one inside a heredoc body or a quoted string does not.
// A redirect inside the script of a shell -c in command position counts.
// It is reached through a run of the script's units, a heredoc operator's line included.
// A fd duplication such as 2>&1 and a target in quotes are no match.
const redirectWritePattern = commandFieldPattern + `(` + topLevelReachPattern + commandWordPrefixPattern + shellDashCPattern +
	`('` + singleQuotedScriptUnitPattern + `*(<<-?` + singleQuotedScriptLineUnitPattern + `*)?` +
	`|\\"` + doubleQuotedScriptUnitPattern + `*(<<-?` + doubleQuotedScriptLineUnitPattern + `*)?)|` +
	shellUnitPattern + `*(<<-?` + shellLineUnitPattern + `*)?)>[>|]?[[:space:]]*` + sourceFilePattern

// shellEditPattern is the grep -E pattern the shell-edit deny matches against the raw PreToolUse payload JSON: an in-place edit command or a redirect into a source file.
var shellEditPattern = `(` + inPlaceEditCommandPattern + `)|(` + redirectWritePattern + `)`

// steerSleepDeny refuses a Bash systemctl machine sleep command; it must contain no single/double quote or backslash (checked at init).
const steerSleepDeny = "the machine sleep commands are refused here; a hibernate goes through lym hibernate request, which waits until every session is paused, and a pair agent asks its parent session to run it"

// noticeSleepDeny announces the sleep deny in the same wording as its steer.
const noticeSleepDeny = steerSleepDeny + "."

// sleepCommandPattern is the grep -E pattern the sleep deny matches against the raw PreToolUse payload JSON.
// It matches systemctl in command position, optionally path-prefixed, followed by any flag words and then one of the verbs sleep, hibernate, suspend, hybrid-sleep or suspend-then-hibernate as a whole word, so hibernate.target is no verb.
// A word that is not a flag ends the match, so systemctl status and every other verb pass.
var sleepCommandPattern = commandRowPattern(func(scope shellScope) string {
	return `systemctl([[:space:]]+-` + scope.word() + `)*[[:space:]]+(sleep|hibernate|suspend|hybrid-sleep|suspend-then-hibernate)([^A-Za-z0-9_.-]|$)`
})

// headlessClaudeRefusal opens the headless claude row's own steer text, which follows the shared whole-command clause.
// It holds the refusal and its reason.
const headlessClaudeRefusal = "a headless claude launch, in print mode, is refused here because its work shows in no pane; "

// steerHeadlessClaudeDeny refuses a Bash launch of a headless claude and names the subagent route in its place; it must contain no single/double quote or backslash (checked at init).
const steerHeadlessClaudeDeny = headlessClaudeRefusal + "hand the work to a subagent through the Agent tool instead, which runs inside this session and shows in its pane"

// noticeHeadlessClaudeDeny announces the headless claude deny in the same wording as its steer.
const noticeHeadlessClaudeDeny = steerHeadlessClaudeDeny + "."

// codexSteerHeadlessClaudeDeny names Codex's own subagents as the route, since Codex has no Agent tool.
const codexSteerHeadlessClaudeDeny = headlessClaudeRefusal + "hand the work to a subagent of your own instead, which runs inside this session"

// headlessClaudeCommandPattern is the grep -E pattern the headless claude deny matches against the raw PreToolUse payload JSON.
// It matches claude or claude.exe in command position, optionally path-prefixed, followed by any argument words and then the long print flag, or a single-dash flag word holding p such as -p or -pc, as a whole word, the long flag with an = value included.
// A word that crosses a separator ends the match, so a print flag of a later command passes.
var headlessClaudeCommandPattern = commandRowPattern(func(scope shellScope) string {
	return `claude(\.exe)?([[:space:]]+` + scope.argument() + `)*[[:space:]]+(-[a-z]*p[a-z]*|--print)([^A-Za-z0-9_./-]|$)`
})

// steerAdoptTakeHoldDeny refuses a Bash adopt that takes another machine hold over to this one.
// It must contain no single/double quote or backslash (checked at init).
// It names the operator as the way forward and spells no flag.
const steerAdoptTakeHoldDeny = "a lym adopt that takes the hold of another machine over to this one is refused here; that takeover is the operator to decide, so ask the operator"

// noticeAdoptTakeHoldDeny announces the adopt takeover deny in the same wording as its steer.
const noticeAdoptTakeHoldDeny = steerAdoptTakeHoldDeny + "."

// adoptTakeHoldCommandPattern is the grep -E pattern the adopt takeover deny matches in the payload JSON.
// It matches lym adopt in command position, optionally path-prefixed.
// Any argument words on the same command may follow, then the --take-hold flag word.
// The flag with an = value is included.
// A word that crosses a separator ends the match, so a flag of a later command passes.
// It sees static text only.
// A written script, a command substitution inside a quoted string and the flag spelled through a variable pass.
var adoptTakeHoldCommandPattern = commandRowPattern(func(scope shellScope) string {
	return `lym[[:space:]]+adopt([[:space:]]+` + scope.argument() + `)*[[:space:]]+--take-hold` + wordEndPattern
})

// steerTmuxScriptDeny refuses a raw tmux or script command and names a tmux-tier test under the gate as the way forward.
// It must contain no single/double quote or backslash (checked at init).
const steerTmuxScriptDeny = "a raw tmux or script command is refused in this session: a tmux server or a script it starts outlives the command and runs outside every memory cap lym sets. To learn how tmux behaves, write a tmux-tier test in the repository and run that one test through lym gate test with --tier naming the tier the hub runs tmux tests in, the package that holds the test, and after -- a filter that selects it"

// noticeTmuxScriptDeny announces the tmux and script deny in the same wording as its steer.
const noticeTmuxScriptDeny = steerTmuxScriptDeny + "."

// tmuxScriptCommandPattern is the grep -E pattern the tmux and script deny matches.
// It is matched against the raw PreToolUse payload JSON.
// It matches tmux or script, optionally path-prefixed, as a whole word in command position.
var tmuxScriptCommandPattern = commandPositionPattern + `(tmux|script)` + wordEndPattern

// githubAllowed maps each gh command the publish row lets through to the subcommands of it the row lets through, nil for every subcommand.
// They read GitHub, comment on a pull request or edit its title and body; every other gh command and subcommand is refused.
// Each word is lowercase letters only, which notWordPattern relies on.
var githubAllowed = map[string][]string{
	"api": nil,
	"pr":  {"checks", "comment", "diff", "edit", "list", "review", "view"},
	"run": {"view"},
}

// steerPublishDeny returns the publish row's steer, naming the gh calls githubAllowed lets through; it must contain no single/double quote or backslash (checked at init).
func steerPublishDeny() string {
	commands := slices.Sorted(maps.Keys(githubAllowed))
	calls := make([]string, len(commands))
	for i, command := range commands {
		calls[i] = "gh " + command
		if subcommands := githubAllowed[command]; subcommands != nil {
			calls[i] += " " + joinWords(subcommands, "or")
		}
	}
	return "git push is refused in this session, and gh runs only its reads and comments: " + joinWords(calls, "and") +
		"; gh api takes no writing method, no --input and no GraphQL mutation, gh pr review no approval or change request, and gh pr edit no new base." +
		" lym pushes the task branch after it gates your commits, and only lym opens, merges or closes the pull request; commit your change and tell the operator"
}

// joinWords joins words with commas and conjunction before the last, as in a, b or c.
func joinWords(words []string, conjunction string) string {
	if len(words) == 1 {
		return words[0]
	}
	return strings.Join(words[:len(words)-1], ", ") + " " + conjunction + " " + words[len(words)-1]
}

// wordEndPattern matches what ends a command-line word: a character no word, path or flag holds, or the end of the payload.
const wordEndPattern = `([^A-Za-z0-9_./-]|$)`

// ghWordClass is the grep -E bracket body of the characters one gh argument word holds: neither whitespace, a separator, a quote, a redirect nor a backtick.
const ghWordClass = `[:space:]"\\;&|()<>` + "`"

// ghRepoFlagsPattern returns a pattern matching any run of -R or --repo flags with their repository word in the scope.
// It is the one flag gh takes between a command and its subcommand.
func ghRepoFlagsPattern(scope shellScope) string {
	return `([[:space:]]+(-R|--repo)([[:space:]]+|=)[^` + scope.ghWordChars() + `]+)*`
}

// ghArgumentsPattern returns a pattern matching any argument words in the scope after a gh subcommand.
// It also matches the whitespace before the next word, on one command.
func ghArgumentsPattern(scope shellScope) string {
	return `([[:space:]]+` + scope.argument() + `)*[[:space:]]+`
}

// ghWriteMethodPattern matches an HTTP method that writes, in upper or lower case.
const ghWriteMethodPattern = `([Pp][Oo][Ss][Tt]|[Pp][Uu][Tt]|[Pp][Aa][Tt][Cc][Hh]|[Dd][Ee][Ll][Ee][Tt][Ee])`

// optionalQuotePattern matches one JSON-escaped double quote or nothing, the quote a shell double-quoted word opens or closes with.
const optionalQuotePattern = `(\\")?`

// quotableWord returns a grep -E pattern matching word bare or inside a pair of JSON-escaped double quotes.
func quotableWord(word string) string {
	return optionalQuotePattern + word + optionalQuotePattern
}

// notWordPattern returns a grep -E pattern matching one whole gh argument word in the scope, up to a word end.
// The word is none of words and opens with no dash, so a flag word never matches.
// A word in double quotes is read by what is inside them, so a listed word in quotes is no match and an unlisted one is.
// Each word must be lowercase letters only.
func notWordPattern(scope shellScope, words []string) string {
	return optionalQuotePattern + notWordFrom(scope, words, true) + wordEndPattern
}

// notWordFrom returns a grep -E group matching the rest of a word in the scope, after a prefix already read.
// The rest is no suffix in suffixes.
// It matches the empty rest only when suffixes holds no empty suffix and first is false, since a word is never empty.
func notWordFrom(scope shellScope, suffixes []string, first bool) string {
	wordChars := scope.ghWordChars()
	children := map[byte][]string{}
	terminal := false
	for _, suffix := range suffixes {
		if suffix == "" {
			terminal = true
			continue
		}
		children[suffix[0]] = append(children[suffix[0]], suffix[1:])
	}
	letters := slices.Sorted(maps.Keys(children))
	excluded := wordChars + string(letters)
	if first {
		excluded += "-"
	}
	alternatives := []string{`[^` + excluded + `][^` + wordChars + `]*`}
	for _, letter := range letters {
		alternatives = append(alternatives, string(letter)+notWordFrom(scope, children[letter], false))
	}
	group := `(` + strings.Join(alternatives, "|") + `)`
	if !terminal && !first {
		group += "?"
	}
	return group
}

// publishCommandPattern returns the grep -E pattern the publish deny matches against the raw PreToolUse payload JSON.
// In command position and optionally path-prefixed, it matches git followed by any flag words, with -C taking its directory word, and then push as a whole word;
// a gh command, or a subcommand of a command githubAllowed limits, that githubAllowed does not list;
// gh pr review with an approve or request-changes flag, alone or in a cluster of short flags, gh pr edit with a base flag and any value attached to the short one;
// and gh api with a writing -X or --method, with --input, or with mutation anywhere in the rest of its command, a quoted multi-line query included.
// A command, subcommand or method word may sit in double quotes.
// A gh word opening with a dash is no command, so gh --version passes.
func publishCommandPattern() string {
	return commandRowPattern(publishCommandIn)
}

// publishCommandIn returns the publish deny's alternatives after a command word in the scope, grouped.
func publishCommandIn(scope shellScope) string {
	gh := `gh` + ghRepoFlagsPattern(scope) + `[[:space:]]+`
	subcommand := func(command, sub string) string {
		return quotableWord(command) + ghRepoFlagsPattern(scope) + `[[:space:]]+` + sub
	}
	arguments := ghArgumentsPattern(scope)
	word := scope.word()
	alternatives := []string{
		`git([[:space:]]+(-C[[:space:]=]+` + word + `|-` + word + `))*[[:space:]]+push` + wordEndPattern,
		gh + notWordPattern(scope, slices.Sorted(maps.Keys(githubAllowed))),
		gh + subcommand("pr", quotableWord("review")) + arguments + `(-[A-Za-z]*[ar][A-Za-z]*|--approve|--request-changes)` + wordEndPattern,
		gh + subcommand("pr", quotableWord("edit")) + arguments + `(-B|--base` + wordEndPattern + `)`,
		gh + quotableWord(`api`) + arguments + `(-X[[:space:]=]*|--method([[:space:]]+|=))` + optionalQuotePattern + ghWriteMethodPattern + wordEndPattern,
		gh + quotableWord(`api`) + arguments + `--input` + wordEndPattern,
		gh + quotableWord(`api`) + wordEndPattern + scope.mutationScan(),
	}
	for _, command := range slices.Sorted(maps.Keys(githubAllowed)) {
		if subcommands := githubAllowed[command]; subcommands != nil {
			alternatives = append(alternatives, gh+subcommand(command, notWordPattern(scope, subcommands)))
		}
	}
	return `(` + strings.Join(alternatives, "|") + `)`
}

// goCommandIn returns the raw go deny's go test, build and vet alternatives after the scope's position, grouped.
// The go word sits directly before the subcommand, with an optional -C <dir> between them.
// A word holding ... or the word all in the arguments, or a -tags value naming tier2 or tier3, denies.
// A single quote is a word end only in a single-quoted script, where it closes the script;
// at the top level it is part of a quoted argument, so go test -run 'TestA all' ./internal/x passes.
func goCommandIn(scope shellScope) string {
	subcommand := `go[[:space:]]+(-C[[:space:]=]+[^[:space:]` + scope.closing + `]+[[:space:]]+)?`
	word := scope.word()
	return `((` + subcommand + `(test|build|vet)[[:space:]]` + scope.goArguments() + `(` + word + `\.\.\.` + word + `|all)` + scope.goWordEnd() + `)` +
		`|(` + subcommand + `test[[:space:]]` + scope.goArguments() + `-{1,2}tags[[:space:]=]+[^[:space:];&|` + scope.closing + `]*(tier2|tier3)))`
}

// lymEnvNamesPattern matches the name of the slot-inheritance variable, the strand-name variable or the prebuilt-binary variable.
const lymEnvNamesPattern = `(` + gateslot.InheritEnv + `|` + agentname.StrandNameEnv + `|` + gateslot.PrebuiltLymEnv + `)`

// rawGoCommandPattern is the grep -E pattern the raw go deny matches against the raw PreToolUse payload JSON.
// It matches three shapes.
// A go test, build or vet whose arguments include a word holding ... or the word all.
// A go test whose -tags value names tier2 or tier3.
// A command setting or reading the slot-inheritance variable, the strand-name variable or the prebuilt-binary variable.
// It matches one named outside quotes or expanded inside a double-quoted string.
// It also matches one named in the script of a shell -c outside a quoted string of the script.
// It matches one expanded inside a double-quoted string of a single-quoted script too.
// A double-quoted string inside a double-quoted script is not read.
// It sees static text only, so it is a guardrail and not a barrier.
// A run hidden behind a script file passes.
// So do a cd plus a relative pattern it does not cover, a shell variable and a quote commandPositionPattern misreads.
// So do a command substitution inside a double-quoted string and a command after a < before a quote.
// So do any raw package-scoped go test and a variable read by printenv with its name quoted.
// It falsely denies a word holding ... or the word all inside a quoted argument of the go command itself, such as a -run filter.
var rawGoCommandPattern = commandRowPattern(goCommandIn) +
	`|(` + commandFieldPattern + `(` + shellUnitPattern + `*([^A-Za-z0-9_"\\'<]|\\[^"]))?` + lymEnvNamesPattern + `)` +
	`|(` + commandFieldPattern + shellUnitPattern + `*` + doubleQuotedOpenPattern + `[$]\{?` + lymEnvNamesPattern + `)` +
	`|(` + commandFieldPattern + topLevelReachPattern + commandWordPrefixPattern + shellDashCPattern + `'` +
	singleQuotedScriptUnitPattern + `*` + singleQuotedScriptStringOpenPattern + `[$]\{?` + lymEnvNamesPattern + `)` +
	`|(` + commandFieldPattern + topLevelReachPattern + commandWordPrefixPattern + shellDashCPattern +
	`('(` + singleQuotedScriptUnitPattern + `*([^A-Za-z0-9_"\\'<]|\\[^"]))?|\\"(` + doubleQuotedScriptUnitPattern + `*([^A-Za-z0-9_"\\'<]|\\[^"]))?)` + lymEnvNamesPattern + `)`

// refuseRawPattern returns the grep -E pattern the refuse-raw deny matches against the raw PreToolUse payload JSON.
// It matches any listed prefix in command position, its words joined by whitespace runs and the last followed by a word end, so go testing is no go test.
// Each word's . is escaped; the gate config loader admits only letters, digits, . _ and - in a word, so no other metacharacter reaches the pattern.
// It sees static text only, so it is a guardrail and not a barrier:
// it lets through a run behind a script file, a flag between the prefix's words such as go -C dir test,
// and an assignment prefix such as GOFLAGS=x go test.
func refuseRawPattern(prefixes []string) string {
	alternatives := make([]string, len(prefixes))
	for i, prefix := range prefixes {
		words := strings.Fields(prefix)
		for j, word := range words {
			words[j] = strings.ReplaceAll(word, ".", `\.`)
		}
		alternatives[i] = strings.Join(words, `[[:space:]]+`)
	}
	return commandPositionPattern + `(` + strings.Join(alternatives, "|") + `)([^A-Za-z0-9_./-]|$)`
}

// wholeCommandRefusal opens every steer, plain and per engine.
// A guard deny refuses the whole shell command, so none of its other parts ran either.
// The row's own text follows it and names the refused part, so the agent runs again only the parts it still needs.
const wholeCommandRefusal = "Nothing in this command ran: the guard refuses the whole command for one part, so run the parts you still need again without it. The refused part: "

// openSteersWithWholeCommandRefusal prefixes every steer of rows, plain and per engine, with wholeCommandRefusal.
// Notices stay as they are, since a notice announces a guard before any command runs.
func openSteersWithWholeCommandRefusal(rows []Row) []Row {
	for i := range rows {
		rows[i].Steer = wholeCommandRefusal + rows[i].Steer
		for engine, text := range rows[i].EngineTexts {
			if text.Steer != "" {
				text.Steer = wholeCommandRefusal + text.Steer
				rows[i].EngineTexts[engine] = text
			}
		}
	}
	return rows
}

// forbiddenSteerChars are characters that would corrupt steer text in JSON or shell quoting layers (checked at init).
const forbiddenSteerChars = `'"\`

// forbiddenNoticeChars extends forbiddenSteerChars with line breaks, since the assembled notice rides one command-line argument that reed types with tmux send-keys.
const forbiddenNoticeChars = forbiddenSteerChars + "\n\r"

// Rows returns a fresh slice of the shell guard rows in table order.
// Each steer opens with the clause that nothing in the refused command ran.
// foreground is the engine's phrase for running a long command in the foreground.
// The gate rule in the raw go row's steer and notice and the refuse-raw row's steer carries it.
// refuseRaw lists the command prefixes the hub refuses to run raw;
// a non-empty list adds the refuse-raw row after the raw go row, and an empty one adds none.
// The raw go row's steer names only the compile checks refuseRaw does not refuse.
func Rows(foreground string, refuseRaw []string) []Row {
	rawGoSteer := "raw module-wide, tmux-tier and llm-tier go runs are refused here." + compileCheckSentence(refuseRaw) + " Gate rule: run lym gate test in the foreground " + foreground + ", never in the background, for example lym gate test ./internal/x, and use its --tier flag for a tier. When lym gate test or lym gate card-verify answers done: false, run the same command again in a turn that does nothing else, so the prompt cache stays warm between calls. Run every other check in the foreground too, and never cd into the directory you already work in. The slot, strand and prebuilt-binary environment variables belong to lym, and a command setting or reading any of them is refused"
	rows := rowsBeforeRefuseRaw(rawGoSteer)
	if len(refuseRaw) > 0 {
		rows = append(rows, refuseRawRow(foreground, refuseRaw))
	}
	return openSteersWithWholeCommandRefusal(append(rows, rowsAfterRawGo()...))
}

// compileCheckForms are the package-scoped compile checks the raw go row's steer offers in place of a refused ./... or all run.
var compileCheckForms = []string{"go build ./internal/x", "go vet ./internal/x"}

// compileCheckSentence returns the raw go steer's sentence, with its leading space, naming each compile check no refuseRaw prefix refuses,
// so the steer never offers a refused command; it returns "" when the prefixes refuse every one.
func compileCheckSentence(refuseRaw []string) string {
	var allowed []string
	for _, form := range compileCheckForms {
		if !refusedRaw(form, refuseRaw) {
			allowed = append(allowed, form)
		}
	}
	if len(allowed) == 0 {
		return ""
	}
	return " A compile check names its packages, as in " + strings.Join(allowed, " or ") + ", never ./... or all."
}

// refusedRaw reports whether a refuseRaw prefix refuses command, that is whether the prefix's words open command's words.
func refusedRaw(command string, refuseRaw []string) bool {
	words := strings.Fields(command)
	for _, prefix := range refuseRaw {
		prefixWords := strings.Fields(prefix)
		if len(prefixWords) <= len(words) && slices.Equal(prefixWords, words[:len(prefixWords)]) {
			return true
		}
	}
	return false
}

// refuseRawRow returns the refuse-raw row over the listed prefixes.
func refuseRawRow(foreground string, refuseRaw []string) Row {
	forms := make([][]string, len(refuseRaw))
	for i, prefix := range refuseRaw {
		forms[i] = strings.Fields(prefix)
	}
	refused := "a raw " + strings.Join(refuseRaw, " or ") + " run is refused here"
	steer := refused + ". Gate rule: run lym gate test in the foreground " + foreground + ", never in the background, with --tier to name the tiers or --card to name a card, for example lym gate test ./internal/x. When lym gate test or lym gate card-verify answers done: false, run the same command again in a turn that does nothing else, so the prompt cache stays warm between calls"
	return Row{
		// Installed for every run, the orch included, when the hub's gate config lists a prefix.
		// A guardrail, not a barrier: refuseRawPattern states what it lets through.
		Name:     "refuse-raw",
		Patterns: []string{refuseRawPattern(refuseRaw)},
		Steer:    steer,
		// The notice names only what is refused and the scopes it adds.
		Notice: refused + "; run it through lym gate test, with --tier to name the tiers or --card to name a card.",
		Switch: SwitchNone,
		Forms:  forms,
	}
}

// rowsBeforeRefuseRaw returns the python and raw go rows, the rows that precede the refuse-raw row.
func rowsBeforeRefuseRaw(rawGoSteer string) []Row {
	return []Row{
		{
			// A guardrail, not a barrier.
			// The pattern lets through python as an argument and python behind eval.
			// It lets through python after a prefix it does not list (env FOO=1, nohup, command, timeout).
			// It lets through python launched by a launcher such as py or uv run.
			// It falsely denies python after a separator inside a quote commandPositionPattern misreads.
			Name:        "python",
			Patterns:    []string{pythonCommandPattern},
			Steer:       steerPythonDeny,
			Notice:      noticePythonDeny,
			Switch:      SwitchDenyPython,
			Forms:       [][]string{{"python"}, {"python3"}},
			EngineTexts: map[string]EngineText{engineCodex: {Steer: codexSteerPythonDeny}},
		},
		{
			// Installed for every run, the orch included, so no session reaches the gate slots around lym gate test.
			// A guardrail, not a barrier: rawGoCommandPattern states what it lets through and what it falsely denies.
			Name:     "raw-go",
			Patterns: []string{rawGoCommandPattern},
			Steer:    rawGoSteer,
			Notice:   rawGoSteer + ".",
			Switch:   SwitchNone,
		},
	}
}

// rowsAfterRawGo returns the rows that follow the raw go and refuse-raw rows.
func rowsAfterRawGo() []Row {
	return []Row{
		{
			// Installed for every run, the orch included, with no config key, so every session edits through the Edit tool, which takes only a file read with Read.
			// A guardrail, not a barrier.
			// An edit behind a script file, awk, cp or mv, behind find -exec or behind xargs with a flag passes.
			// A redirect into a quoted target passes.
			// So does a redirect into a file whose extension sourceFilePattern does not name.
			// So does anything after a heredoc operator's line.
			// It falsely denies a matching spelling after a separator inside a quote commandPositionPattern misreads.
			Name:        "shell-edit",
			Patterns:    []string{shellEditPattern},
			Steer:       steerShellEditDeny,
			Notice:      noticeShellEditDeny,
			Switch:      SwitchNone,
			Forms:       [][]string{{"sed", "-i"}, {"perl", "-i"}, {"perl", "-pi"}},
			EngineTexts: map[string]EngineText{engineCodex: {Steer: codexSteerShellEditDeny, Notice: codexSteerShellEditDeny + "."}},
		},
		{
			// Refuses before Claude Code raises its statically-unresolvable-target dialog, which shows even in bypass mode and stalls an unwatched run.
			// Installed for every run, the orch included, with no config key.
			// Requires rm in command position and an expansion anywhere in the payload.
			// A guardrail, not a barrier: it falsely denies an rm of a literal path in a command that uses $ elsewhere in the payload, the description field included.
			// It lets through a removal behind find -delete, behind xargs rm with no $ and behind a script file.
			// It lets through a removal through the special parameters it does not list ($$, $?, $#, $!).
			Name:     "computed-rm",
			Patterns: []string{rmCommandPattern, shellExpansionPattern},
			Steer:    steerComputedRmDeny,
			Notice:   noticeComputedRmDeny,
			Switch:   SwitchNone,
		},
		{
			// Installed for every run, the orch included, with no config key.
			// A guardrail, not a barrier.
			// Eval, a shell variable and systemctl start or isolate of a sleep target pass.
			// So do other sleep routes such as busctl, dbus-send and rtcwake.
			// A matching spelling after a separator inside a quote commandPositionPattern misreads is falsely denied.
			Name:     "machine-sleep",
			Patterns: []string{sleepCommandPattern},
			Steer:    steerSleepDeny,
			Notice:   noticeSleepDeny,
			Switch:   SwitchNone,
			Forms: [][]string{
				{"systemctl", "sleep"},
				{"systemctl", "hibernate"},
				{"systemctl", "suspend"},
				{"systemctl", "hybrid-sleep"},
				{"systemctl", "suspend-then-hibernate"},
			},
		},
		{
			// Installed for every run, the orch included, with no config key, so no session hands work to an agent whose work shows in no pane.
			// A guardrail, not a barrier.
			// A launch behind a script file, an assignment prefix such as X=1 claude, or env with an assignment passes.
			// A claude subcommand, or a command launched through one, taking a single-dash flag word holding p of its own is falsely denied.
			// So is a matching spelling after a separator inside a quote commandPositionPattern misreads.
			Name:        "headless-claude",
			Patterns:    []string{headlessClaudeCommandPattern},
			Steer:       steerHeadlessClaudeDeny,
			Notice:      noticeHeadlessClaudeDeny,
			Switch:      SwitchNone,
			Forms:       [][]string{{"claude", "-p"}, {"claude", "--print"}},
			EngineTexts: map[string]EngineText{engineCodex: {Steer: codexSteerHeadlessClaudeDeny, Notice: codexSteerHeadlessClaudeDeny + "."}},
		},
		{
			// Installed for every run, the orch included, with no config key, so no agent moves another machine hold to this one.
			// A guardrail, not a barrier.
			// A written script, a command substitution inside a quoted string and a variable-spelled flag pass.
			// A matching spelling after a separator inside a quote commandPositionPattern misreads is falsely denied.
			Name:     "adopt-take-hold",
			Patterns: []string{adoptTakeHoldCommandPattern},
			Steer:    steerAdoptTakeHoldDeny,
			Notice:   noticeAdoptTakeHoldDeny,
			Switch:   SwitchNone,
			Forms:    [][]string{{"lym", "adopt", "--take-hold"}},
		},
		{
			// Installed for every run on every engine and for every role, interactive sessions included.
			// Only a launch spec that sets AllowRawTmux leaves it out.
			// A tmux server or a script started from the shell outlives the command.
			// It runs outside the memory scope lym gives each test binary.
			// A script with its input on an endless stream queues that input in memory without bound.
			// The sampler's spec sets AllowRawTmux, since its trials start tmux servers of their own.
			// A guardrail, not a barrier.
			// A launch behind a script file, eval, nohup, setsid or timeout passes, and so does one spelled through a variable.
			// A program named script or tmux under another directory, such as tools/script, is falsely denied.
			// So is a matching spelling after a separator inside a quote commandPositionPattern misreads.
			Name:     "tmux-script",
			Patterns: []string{tmuxScriptCommandPattern},
			Steer:    steerTmuxScriptDeny,
			Notice:   noticeTmuxScriptDeny,
			Switch:   SwitchDenyRawTmux,
		},
		{
			// Installed only for a launch spec that sets DenyPublish, such as the PR-Review session's, which reads the pull request, comments on it and edits its title and body through gh.
			// A guardrail, not a barrier.
			// A script file, env forms the pattern does not list, a git alias or hub pass.
			// So does a flag word between gh and its command.
			// So does a gh api call that writes by an implied POST with field flags.
			// So does a GraphQL mutation read from a file.
			// A gh spelling after a separator inside a quote commandPositionPattern misreads is falsely denied.
			// So is a gh pr review short-flag cluster whose attached body holds an a or an r.
			// A gh api call with an unbalanced apostrophe before its mutation word passes, since the scan reads a quoted string whole.
			// A lym verb that pushes the task branch, such as lym fabric push or the detached both-sides push a run-record commit fires, reaches origin from the session too, so the row gates what lands, not what reaches the pull request.
			Name:     "publish",
			Patterns: []string{publishCommandPattern()},
			Steer:    steerPublishDeny(),
			Notice:   steerPublishDeny() + ".",
			Switch:   SwitchDenyPublish,
		},
	}
}

// checkTexts returns an error naming the first row whose steer, plain or an engine's, holds a character the JSON payload or the quoted hook command cannot carry, or whose notice holds one the one-line launch argument cannot.
func checkTexts(rows []Row) error {
	for _, row := range rows {
		texts := []EngineText{{Steer: row.Steer, Notice: row.Notice}}
		for _, engine := range slices.Sorted(maps.Keys(row.EngineTexts)) {
			texts = append(texts, row.EngineTexts[engine])
		}
		for _, text := range texts {
			if strings.ContainsAny(text.Steer, forbiddenSteerChars) {
				return fmt.Errorf("shellguard: row %q: steer text contains a forbidden character (one of %q), which would break the JSON payload or the echo hook command: %q", row.Name, forbiddenSteerChars, text.Steer)
			}
			if strings.ContainsAny(text.Notice, forbiddenNoticeChars) {
				return fmt.Errorf("shellguard: row %q: notice text contains a forbidden character (one of %q), which would break the one-line launch argument: %q", row.Name, forbiddenNoticeChars, text.Notice)
			}
		}
	}
	return nil
}

// init panics if any shipped row's text holds a forbidden character (checked at package load).
func init() {
	if err := checkTexts(Rows("", []string{"go test"})); err != nil {
		panic(err.Error())
	}
}
