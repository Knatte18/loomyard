// rows.go declares the shell guard rows, the patterns they match and the steer and notice text each carries, and the init-time check on that text.

package shellguard

import (
	"fmt"
	"strings"

	"github.com/Knatte18/loomyard/internal/agentname"
	"github.com/Knatte18/loomyard/internal/gateslot"
)

// Switch names the config switch a row hangs on.
type Switch string

const (
	// SwitchNone marks a row installed for every run.
	SwitchNone Switch = ""
	// SwitchDenyPython marks the row the python switch of the shuttle config turns off.
	SwitchDenyPython Switch = "deny_python"
	// SwitchDenyPublish marks the row a launch spec turns on to keep a session from publishing: git push and gh.
	SwitchDenyPublish Switch = "deny_publish"
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
}

// steerPythonDeny refuses a Bash python invocation; it must contain no single/double quote or backslash (checked at init).
const steerPythonDeny = "lym agents never run Python; edit files with Edit or Write, search and read with grep, awk, cat or sed -n"

// noticePythonDeny announces the python deny in the same wording as its steer.
const noticePythonDeny = steerPythonDeny + "."

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

// commandPositionPattern is the grep -E prefix that puts the pattern after it in command position of the raw PreToolUse payload JSON, with an optional path prefix.
// Command position is the start of the command, or right after a ; & | ( <( >( or backtick, which also covers $( and ( subshells, or a JSON-escaped newline, then any of env, exec, xargs, sudo, time.
// The separator is reached through a run of shell units, and one after a heredoc operator counts only on that operator's line.
// It sees static text only: it misreads a JSON-escaped double quote inside a double-quoted string and a quote inside a comment, and it sees nothing after a heredoc operator's line.
// A double-quoted string is one unit, so a command substitution inside it, as in out="$(python3 x.py)", is never in command position and passes.
// A < before a quote or a backslash, as in <"f" or <<<"f", ends the run of units, so a command after it passes.
const commandPositionPattern = commandFieldPattern + `(` + shellUnitPattern + `*([;&|` + "`" + `]|[<>]?\(|\\n|<<-?` + shellLineUnitPattern + `*([;&|` + "`" + `]|[<>]?\()))?` +
	`[[:space:]]*((env|exec|xargs|sudo|time)[[:space:]]+)*([^[:space:]"\\;&|()` + "`" + `]*/)?`

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

// steerSleepDeny refuses a Bash systemctl machine sleep command; it must contain no single/double quote or backslash (checked at init).
const steerSleepDeny = "the machine sleep commands are refused here; a hibernate goes through lym hibernate request, which waits until every session is paused, and a pair agent asks its parent session to run it"

// noticeSleepDeny announces the sleep deny in the same wording as its steer.
const noticeSleepDeny = steerSleepDeny + "."

// sleepCommandPattern is the grep -E pattern the sleep deny matches against the raw PreToolUse payload JSON.
// It matches systemctl in command position, optionally path-prefixed, followed by any flag words and then one of the verbs sleep, hibernate, suspend, hybrid-sleep or suspend-then-hibernate as a whole word, so hibernate.target is no verb.
// A word that is not a flag ends the match, so systemctl status and every other verb pass.
const sleepCommandPattern = commandPositionPattern + `systemctl([[:space:]]+-` + goWordPattern + `)*[[:space:]]+(sleep|hibernate|suspend|hybrid-sleep|suspend-then-hibernate)([^A-Za-z0-9_.-]|$)`

// steerPublishDeny refuses a Bash git push or gh; it must contain no single/double quote or backslash (checked at init).
const steerPublishDeny = "git push and gh are refused in this session: lym pushes the task branch after it gates your commits, and only lym opens, merges or closes the pull request; commit your change and tell the operator"

// noticePublishDeny announces the publish deny in the same wording as its steer.
const noticePublishDeny = steerPublishDeny + "."

// publishCommandPattern is the grep -E pattern the publish deny matches against the raw PreToolUse payload JSON.
// It matches, in command position and optionally path-prefixed, either git followed by any flag words, with -C taking its directory word, and then push as a whole word, or gh as a whole word.
const publishCommandPattern = commandPositionPattern + `(git([[:space:]]+(-C[[:space:]=]+` + goWordPattern + `|-` + goWordPattern + `))*[[:space:]]+push|gh)([^A-Za-z0-9_./-]|$)`

// goSubcommandPattern is the grep -E prefix of a go test, build or vet command: go in command position directly before the subcommand, with an optional -C <dir> between them, then whitespace.
const goSubcommandPattern = commandPositionPattern + `go[[:space:]]+(-C[[:space:]=]+[^[:space:]]+[[:space:]]+)?`

// goArgumentsPattern matches the rest of a command line after a go subcommand and ends on whitespace: anything up to the next separator or JSON-escaped newline.
const goArgumentsPattern = `(([^;&|\\]|\\[^n])*[[:space:]])?`

// goWordPattern matches the characters of one command-line word.
const goWordPattern = `[^[:space:]"\\;&|()` + "`" + `]*`

// lymEnvNamesPattern matches the name of the slot-inheritance variable, the strand-name variable or the prebuilt-binary variable.
const lymEnvNamesPattern = `(` + gateslot.InheritEnv + `|` + agentname.StrandNameEnv + `|` + gateslot.PrebuiltLymEnv + `)`

// rawGoCommandPattern is the grep -E pattern the raw go deny matches against the raw PreToolUse payload JSON.
// It matches three shapes.
// A go test, build or vet whose arguments include a word holding ... or the word all.
// A go test whose -tags value names tier2 or tier3.
// A command setting or reading the slot-inheritance variable, the strand-name variable or the prebuilt-binary variable: one named outside quotes, or expanded inside a double-quoted string.
// It sees static text only, so it is a guardrail and not a barrier:
// a run hidden behind a script, bash -c, a cd plus a relative pattern it does not cover, a shell variable, a command substitution inside a double-quoted string, a command after a < before a quote, or a quote commandPositionPattern misreads passes, as does any raw package-scoped go test and a variable read by printenv with its name quoted.
// It falsely denies a word holding ... or the word all inside a quoted argument of the go command itself, such as a -run filter.
const rawGoCommandPattern = `(` + goSubcommandPattern + `(test|build|vet)[[:space:]]` + goArgumentsPattern + `(` + goWordPattern + `\.\.\.` + goWordPattern + `|all)([[:space:]"\\;&|()` + "`" + `]|$))` +
	`|(` + goSubcommandPattern + `test[[:space:]]` + goArgumentsPattern + `-{1,2}tags[[:space:]=]+[^[:space:];&|]*(tier2|tier3))` +
	`|(` + commandFieldPattern + `(` + shellUnitPattern + `*([^A-Za-z0-9_"\\'<]|\\[^"]))?` + lymEnvNamesPattern + `)` +
	`|(` + commandFieldPattern + shellUnitPattern + `*` + doubleQuotedOpenPattern + `[$]\{?` + lymEnvNamesPattern + `)`

// refuseRawPattern returns the grep -E pattern the refuse-raw deny matches against the raw PreToolUse payload JSON.
// It matches any listed prefix in command position, its words joined by whitespace runs and the last followed by a word end, so go testing is no go test.
// Each word's . is escaped; the gate config loader admits only letters, digits, . _ and - in a word, so no other metacharacter reaches the pattern.
// It sees static text only, so it is a guardrail and not a barrier:
// it lets through a run behind a script or bash -c, a flag between the prefix's words such as go -C dir test, and an assignment prefix such as GOFLAGS=x go test.
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

// forbiddenSteerChars are characters that would corrupt steer text in JSON or shell quoting layers (checked at init).
const forbiddenSteerChars = `'"\`

// forbiddenNoticeChars extends forbiddenSteerChars with line breaks, since the assembled notice rides one command-line argument that reed types with tmux send-keys.
const forbiddenNoticeChars = forbiddenSteerChars + "\n\r"

// Rows returns a fresh slice of the shell guard rows in table order.
// foreground is the engine's phrase for running a long command in the foreground, which the raw go and refuse-raw rows' steers and notices carry.
// refuseRaw lists the command prefixes the hub refuses to run raw; a non-empty list adds the refuse-raw row after the raw go row, and an empty one adds none.
func Rows(foreground string, refuseRaw []string) []Row {
	rawGoSteer := "raw module-wide, tmux-tier and llm-tier go runs are refused here. Gate rule: run lym gate test in the foreground " + foreground + ", never in the background, for example lym gate test ./internal/x, and use its --tier flag for a tier. When it answers that no gate slot is free yet, run the same command again in a turn that does nothing else, so the prompt cache stays warm between calls. Run every other check in the foreground too, and never cd into the directory you already work in. The slot, strand and prebuilt-binary environment variables belong to lym, and a command setting or reading any of them is refused"
	rows := rowsBeforeRefuseRaw(rawGoSteer)
	if len(refuseRaw) > 0 {
		rows = append(rows, refuseRawRow(foreground, refuseRaw))
	}
	return append(rows, rowsAfterRawGo()...)
}

// refuseRawRow returns the refuse-raw row over the listed prefixes.
func refuseRawRow(foreground string, refuseRaw []string) Row {
	forms := make([][]string, len(refuseRaw))
	for i, prefix := range refuseRaw {
		forms[i] = strings.Fields(prefix)
	}
	steer := "a raw " + strings.Join(refuseRaw, " or ") + " run is refused here. Gate rule: run lym gate test in the foreground " + foreground + ", never in the background, with --tier to name the tiers or --card to name a card, for example lym gate test ./internal/x. When it answers that no gate slot is free yet, run the same command again in a turn that does nothing else, so the prompt cache stays warm between calls"
	return Row{
		// Installed for every run, the orch included, when the hub's gate config lists a prefix.
		// A guardrail, not a barrier: refuseRawPattern states what it lets through.
		Name:     "refuse-raw",
		Patterns: []string{refuseRawPattern(refuseRaw)},
		Steer:    steer,
		Notice:   steer + ".",
		Switch:   SwitchNone,
		Forms:    forms,
	}
}

// rowsBeforeRefuseRaw returns the python and raw go rows, the rows that precede the refuse-raw row.
func rowsBeforeRefuseRaw(rawGoSteer string) []Row {
	return []Row{
		{
			// A guardrail, not a barrier.
			// The pattern lets through python as an argument, behind bash -c, sh -c or eval, after a prefix it does not list (env FOO=1, nohup, command, timeout), and through a launcher such as py or uv run.
			// It falsely denies python after a separator inside a quote commandPositionPattern misreads.
			Name:     "python",
			Patterns: []string{pythonCommandPattern},
			Steer:    steerPythonDeny,
			Notice:   noticePythonDeny,
			Switch:   SwitchDenyPython,
			Forms:    [][]string{{"python"}, {"python3"}},
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
			// Refuses before Claude Code raises its statically-unresolvable-target dialog, which shows even in bypass mode and stalls an unwatched run.
			// Installed for every run, the orch included, with no config key.
			// Requires rm in command position and an expansion anywhere in the payload.
			// A guardrail, not a barrier: it falsely denies an rm of a literal path in a command that uses $ elsewhere in the payload, the description field included.
			// It lets through a removal behind find -delete, xargs rm with no $, bash -c or a script, and one through the special parameters it does not list ($$, $?, $#, $!).
			Name:     "computed-rm",
			Patterns: []string{rmCommandPattern, shellExpansionPattern},
			Steer:    steerComputedRmDeny,
			Notice:   noticeComputedRmDeny,
			Switch:   SwitchNone,
		},
		{
			// Installed for every run, the orch included, with no config key.
			// A guardrail, not a barrier: bash -c, eval, a shell variable, systemctl start or isolate of a sleep target and other sleep routes such as busctl, dbus-send and rtcwake pass, and a matching spelling after a separator inside a quote commandPositionPattern misreads is falsely denied.
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
			// Installed only for a launch spec that sets DenyPublish, the PR-Review session.
			// A guardrail, not a barrier: bash -c, a script, env forms the pattern does not list, a git alias or hub pass, and a gh spelling after a separator inside a quote commandPositionPattern misreads is falsely denied.
			// A lym verb that pushes the task branch, such as lym fabric push or the detached both-sides push a run-record commit fires, reaches origin from the session too, so the row gates what lands, not what reaches the pull request.
			Name:     "publish",
			Patterns: []string{publishCommandPattern},
			Steer:    steerPublishDeny,
			Notice:   noticePublishDeny,
			Switch:   SwitchDenyPublish,
		},
	}
}

// checkTexts returns an error naming the first row whose steer holds a character the JSON payload or the quoted hook command cannot carry, or whose notice holds one the one-line launch argument cannot.
func checkTexts(rows []Row) error {
	for _, row := range rows {
		if strings.ContainsAny(row.Steer, forbiddenSteerChars) {
			return fmt.Errorf("shellguard: row %q: steer text contains a forbidden character (one of %q), which would break the JSON payload or the echo hook command: %q", row.Name, forbiddenSteerChars, row.Steer)
		}
		if strings.ContainsAny(row.Notice, forbiddenNoticeChars) {
			return fmt.Errorf("shellguard: row %q: notice text contains a forbidden character (one of %q), which would break the one-line launch argument: %q", row.Name, forbiddenNoticeChars, row.Notice)
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
