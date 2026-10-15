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
const steerPythonDeny = "lyx agents never run Python; edit files with Edit or Write, search and read with grep, awk, cat or sed -n"

// noticePythonDeny announces the python deny in the same wording as its steer.
const noticePythonDeny = steerPythonDeny + "."

// commandPositionPattern is the grep -E prefix that puts the pattern after it in command position of the raw PreToolUse payload JSON, with an optional path prefix.
// Command position is right after the "command" key's opening quote, after a JSON-escaped newline, after ; & | ( or a backtick (which also covers $( ), or after one of env, exec, xargs, sudo, time.
// The shell single quotes around the patterns built on it forbid a single quote in them.
const commandPositionPattern = `("command"[[:space:]]*:[[:space:]]*"|\\n|[;&|(` + "`" + `]|(^|[^A-Za-z0-9_-])(env|exec|xargs|sudo|time)[[:space:]]+)[[:space:]]*([^[:space:]"\\;&|()` + "`" + `]*/)?`

// pythonCommandPattern is the grep -E pattern the python deny matches against the raw PreToolUse payload JSON.
// It matches python, python3 or python3.<minor>, optionally path-prefixed, as a whole word in command position.
const pythonCommandPattern = commandPositionPattern + `python(3(\.[0-9]+)?)?([^A-Za-z0-9_./-]|$)`

// steerInPlaceEditDeny refuses a Bash sed or perl in-place edit; it must contain no single/double quote or backslash (checked at init).
// It points at the edit directive's rule and does not restate it.
const steerInPlaceEditDeny = "scripted in-place edits are refused here under the edit directive in your prompt; reading with sed -n is fine"

// noticeInPlaceEditDeny announces the in-place edit deny in the same wording as its steer.
const noticeInPlaceEditDeny = steerInPlaceEditDeny + "."

// inPlaceEditCommandPattern is the grep -E pattern the in-place edit deny matches against the raw PreToolUse payload JSON.
// It matches sed or perl in command position, optionally path-prefixed, followed by that command's words up to the next ; & | ) backtick or JSON-escaped newline, then an in-place flag word:
// for sed, -i or -i<suffix>, a short-flag cluster holding i such as -ni or -Ei, or a -- word that is a prefix of --in-place at least as long as --i, optionally followed by =<suffix>;
// for perl, a short-flag cluster holding i such as -i, -i.bak or -pi.
// A short-flag word opens after whitespace with a single dash and letters, so --quiet and --expression= never match as a cluster.
// GNU sed accepts any unambiguous abbreviation of a long option and no other sed long option starts with --i.
// It sees static text only, so it is a guardrail and not a barrier:
// it lets through sed writing by redirection, a w command inside a script, awk, find -exec sed -i, anything behind bash -c, eval or a variable, and anything after a prefix commandPositionPattern does not list.
// It falsely denies a matching spelling inside a quoted argument, a grep alternation or another payload field, and a perl read whose cluster holds an i, such as perl -Mstrict -ne.
const inPlaceEditCommandPattern = commandPositionPattern + `(` +
	`sed[[:space:]]` + inPlaceCommandWordsPattern + `(-[A-Za-z]*i|--i(n(-(p(l(a(c(e)?)?)?)?)?)?)?(=[^[:space:]"\\;&|)` + "`" + `]*)?([[:space:]"\\;&|)` + "`" + `]|$))` +
	`|perl[[:space:]]` + inPlaceCommandWordsPattern + `-[A-Za-z]*i` +
	`)`

// inPlaceCommandWordsPattern matches the words of a command between its name and an in-place flag: optionally anything up to the next separator or JSON-escaped newline, ending on whitespace.
const inPlaceCommandWordsPattern = `(([^;&|)\\` + "`" + `]|\\[^n])*[[:space:]])?`

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
const steerSleepDeny = "the machine sleep commands are refused here; a hibernate goes through lyx hibernate request, which waits until every session is paused, and a pair agent asks its parent session to run it"

// noticeSleepDeny announces the sleep deny in the same wording as its steer.
const noticeSleepDeny = steerSleepDeny + "."

// sleepCommandPattern is the grep -E pattern the sleep deny matches against the raw PreToolUse payload JSON.
// It matches systemctl in command position, optionally path-prefixed, followed by any flag words and then one of the verbs sleep, hibernate, suspend, hybrid-sleep or suspend-then-hibernate as a whole word, so hibernate.target is no verb.
// A word that is not a flag ends the match, so systemctl status and every other verb pass.
const sleepCommandPattern = commandPositionPattern + `systemctl([[:space:]]+-` + goWordPattern + `)*[[:space:]]+(sleep|hibernate|suspend|hybrid-sleep|suspend-then-hibernate)([^A-Za-z0-9_.-]|$)`

// goSubcommandPattern is the grep -E prefix of a go test, build or vet command: go in command position directly before the subcommand, with an optional -C <dir> between them, then whitespace.
const goSubcommandPattern = commandPositionPattern + `go[[:space:]]+(-C[[:space:]=]+[^[:space:]]+[[:space:]]+)?`

// goArgumentsPattern matches the rest of a command line after a go subcommand and ends on whitespace: anything up to the next separator or JSON-escaped newline.
const goArgumentsPattern = `(([^;&|\\]|\\[^n])*[[:space:]])?`

// goWordPattern matches the characters of one command-line word.
const goWordPattern = `[^[:space:]"\\;&|()` + "`" + `]*`

// rawGoCommandPattern is the grep -E pattern the raw go deny matches against the raw PreToolUse payload JSON.
// It matches three shapes.
// A go test, build or vet whose arguments include a word holding ... or the word all.
// A go test whose -tags value names tmux or llm.
// Any command naming the slot-inheritance variable, the strand-name variable or the prebuilt-binary variable.
// It sees static text only, so it is a guardrail and not a barrier:
// a run hidden behind a script, bash -c, a cd plus a relative pattern it does not cover, or a shell variable passes, as does any raw package-scoped go test.
// It falsely denies a matching spelling inside a quoted argument, a commit message or a grep.
const rawGoCommandPattern = `(` + goSubcommandPattern + `(test|build|vet)[[:space:]]` + goArgumentsPattern + `(` + goWordPattern + `\.\.\.` + goWordPattern + `|all)([[:space:]"\\;&|()` + "`" + `]|$))` +
	`|(` + goSubcommandPattern + `test[[:space:]]` + goArgumentsPattern + `-{1,2}tags[[:space:]=]+[^[:space:];&|]*(tmux|llm))` +
	`|` + gateslot.InheritEnv + `|` + agentname.StrandNameEnv + `|` + gateslot.PrebuiltLyxEnv

// forbiddenSteerChars are characters that would corrupt steer text in JSON or shell quoting layers (checked at init).
const forbiddenSteerChars = `'"\`

// forbiddenNoticeChars extends forbiddenSteerChars with line breaks, since the assembled notice rides one command-line argument that reed types with tmux send-keys.
const forbiddenNoticeChars = forbiddenSteerChars + "\n\r"

// Rows returns a fresh slice of the shell guard rows in table order.
// foreground is the engine's phrase for running a long command in the foreground, which the raw go row's steer and notice carry.
func Rows(foreground string) []Row {
	rawGoSteer := "raw module-wide, tmux-tier and llm-tier go runs are refused here. Gate rule: run lyx gate test in the foreground " + foreground + ", never in the background, for example lyx gate test ./internal/x, and use its --tags flag for a tag. Run every other check in the foreground too, and never cd into the directory you already work in. The slot, strand and prebuilt-binary environment variables belong to lyx, and a command naming any of them is refused"
	return []Row{
		{
			// A guardrail, not a barrier.
			// The pattern lets through python as an argument, behind bash -c, sh -c or eval, after a prefix it does not list (env FOO=1, nohup, command, timeout), and through a launcher such as py or uv run.
			// It falsely denies python after a listed separator inside a quoted argument, a heredoc body or another payload field such as description.
			Name:     "python",
			Patterns: []string{pythonCommandPattern},
			Steer:    steerPythonDeny,
			Notice:   noticePythonDeny,
			Switch:   SwitchDenyPython,
			Forms:    [][]string{{"python"}, {"python3"}},
		},
		{
			// Installed for every run, the orch included, so no session reaches the gate slots around lyx gate test.
			// A guardrail, not a barrier: rawGoCommandPattern states what it lets through and what it falsely denies.
			Name:     "raw-go",
			Patterns: []string{rawGoCommandPattern},
			Steer:    rawGoSteer,
			Notice:   rawGoSteer + ".",
			Switch:   SwitchNone,
		},
		{
			// Installed for every run, the orch included, with no config key.
			// A guardrail, not a barrier: inPlaceEditCommandPattern states what it lets through and what it falsely denies.
			Name:     "in-place-edit",
			Patterns: []string{inPlaceEditCommandPattern},
			Steer:    steerInPlaceEditDeny,
			Notice:   noticeInPlaceEditDeny,
			Switch:   SwitchNone,
			Forms:    [][]string{{"sed", "-i"}, {"perl", "-i"}},
		},
		{
			// Refuses before Claude Code raises its statically-unresolvable-target dialog, which shows even in bypass mode and stalls an unwatched run.
			// Installed for every run, the orch included, with no config key.
			// Requires rm in command position and an expansion anywhere in the payload.
			// A guardrail, not a barrier: it falsely denies an rm of a literal path in a command that uses $ elsewhere, a grep alternation naming rm, and a matching spelling in another payload field such as the description.
			// It lets through a removal behind find -delete, xargs rm with no $, bash -c or a script, and one through the special parameters it does not list ($$, $?, $#, $!).
			Name:     "computed-rm",
			Patterns: []string{rmCommandPattern, shellExpansionPattern},
			Steer:    steerComputedRmDeny,
			Notice:   noticeComputedRmDeny,
			Switch:   SwitchNone,
		},
		{
			// Installed for every run, the orch included, with no config key.
			// A guardrail, not a barrier: bash -c, eval, a shell variable, systemctl start or isolate of a sleep target and other sleep routes such as busctl, dbus-send and rtcwake pass, and a matching spelling inside a quoted argument or another payload field is falsely denied.
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
	if err := checkTexts(Rows("")); err != nil {
		panic(err.Error())
	}
}
