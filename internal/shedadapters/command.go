// command.go implements CommandProducer, the producer that runs one operator-configured shell command and never halts the run over the command's outcome.

package shedadapters

import (
	"context"
	"fmt"
	"runtime"
	"slices"
	"strings"
	"time"

	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/shedengine"
	"github.com/Knatte18/loomyard/internal/verifyrun"
)

// commandEngine is the engine label a Command row carries in logs and errors.
const commandEngine = "Command"

// CommandRow names the config setting one Command row reads its command from and the time its command may run.
type CommandRow struct {
	// CommandModule and CommandKey name the string setting that holds the command.
	CommandModule, CommandKey string
	// Timeout ends a command that hangs.
	Timeout time.Duration
}

// CommandResult is what a CommandDeps.Run reports about one command.
type CommandResult struct {
	// ExitCode is the command's exit code.
	ExitCode int
	// TimedOut reports that the timeout ended the command.
	TimedOut bool
}

// CommandDeps is what the recipe's owner tells a CommandProducer; the producer derives no path itself.
type CommandDeps struct {
	// Setting is the plain setting lookup through which the command key is read over the owner's ordinary stack to warn about an ignored value.
	Setting func(module, key string) (any, error)
	// ExecutableSetting is the owner's lookup over the template and the board default layer only; the command is read through it and nothing else.
	ExecutableSetting func(module, key string) (any, error)
	// Lyx and Slug are the values of the {lyx} and {slug} placeholders.
	Lyx, Slug string
	// TaskDir returns the value of the {task_dir} placeholder; it is called only when the command holds that placeholder.
	TaskDir func() (string, error)
	// Dir is the directory the command runs in.
	Dir string
	// LogPath returns the log file of row's command.
	LogPath func(row string) string
	// Run runs command once through the platform shell in dir, writing its output to logPath, and ends it after timeout.
	Run func(ctx context.Context, dir, command, logPath string, timeout time.Duration) (CommandResult, error)
	// Rerun is the command that resumes the run, named in every way forward.
	Rerun string
	// GOOS names the platform whose shell quoting the placeholders get; empty means runtime.GOOS.
	GOOS string
}

// CommandProducer runs its row's configured command once and is Done whatever the command did.
type CommandProducer struct {
	name string
	row  CommandRow
	deps CommandDeps
}

var _ shedengine.ShedProducer = (*CommandProducer)(nil)

// NewCommandProducer returns the producer of the row name over row's settings and deps' seams.
func NewCommandProducer(name string, row CommandRow, deps CommandDeps) *CommandProducer {
	return &CommandProducer{name: name, row: row, deps: deps}
}

// Call implements shedengine.ShedProducer.
// See the package documentation's Command section for the order it follows.
// Only an unreadable command setting or a missing executable-key lookup is an error; a command that fails, times out or cannot start is warned about and the row is Done.
func (p *CommandProducer) Call(ctx context.Context) (shedengine.Outcome, shedengine.OutputPointer, error) {
	if err := entryErr(ctx, p.name, commandEngine); err != nil {
		return "", shedengine.OutputPointer{}, err
	}
	fail := func(format string, args ...any) (shedengine.Outcome, shedengine.OutputPointer, error) {
		return "", shedengine.OutputPointer{}, fmt.Errorf("shedadapters: %s (%s): %s", p.name, commandEngine, fmt.Sprintf(format, args...))
	}
	done := shedengine.Done
	if p.deps.ExecutableSetting == nil {
		return fail("the recipe's owner tells no executable-key lookup; way forward: wire an executable-key lookup over the template and the board default layer in the recipe's owner, then re-run the run on the rebuilt lyx (%s)", p.deps.Rerun)
	}

	setCommand := fmt.Sprintf("`lyx config %s --layer default --set %s=<value>` from the prime", p.row.CommandModule, p.row.CommandKey)
	value, err := p.deps.ExecutableSetting(p.row.CommandModule, p.row.CommandKey)
	if err != nil {
		return fail("read the command %s.%s: %v; way forward: correct it with %s, then %s", p.row.CommandModule, p.row.CommandKey, err, setCommand, p.deps.Rerun)
	}
	command, isString := value.(string)
	if !isString {
		return fail("the command %s.%s holds %v, not a string; way forward: correct it with %s, then %s", p.row.CommandModule, p.row.CommandKey, value, setCommand, p.deps.Rerun)
	}
	p.warnOnIgnoredCommand(command, setCommand)

	if strings.TrimSpace(command) == "" {
		logger.Info("shedadapters: command row has an empty command; passing on", "producer", p.name, "engine", commandEngine)
		return done, shedengine.OutputPointer{}, nil
	}

	values := map[string]string{}
	if strings.Contains(command, "{lyx}") {
		values["lyx"] = p.deps.Lyx
	}
	if strings.Contains(command, "{slug}") {
		values["slug"] = p.deps.Slug
	}
	if strings.Contains(command, "{task_dir}") {
		taskDir, err := p.deps.TaskDir()
		if err != nil {
			return fail("resolve the task worktree for {task_dir}: %v; way forward: correct it, then %s", err, p.deps.Rerun)
		}
		values["task_dir"] = taskDir
	}
	goos := p.deps.GOOS
	if goos == "" {
		goos = runtime.GOOS
	}
	expanded, refused, err := expandPlaceholders(command, goos, values)
	if err != nil {
		logger.Warn("shedadapters: command row cannot quote a placeholder value; passing on without running it", "producer", p.name, "engine", commandEngine, "placeholder", refused, "way_forward", refusedPlaceholderWayForward(refused), "error", err)
		return done, shedengine.OutputPointer{}, nil
	}

	logPath := p.deps.LogPath(p.name)
	logger.Info("shedadapters: command row starting its command", "producer", p.name, "engine", commandEngine, "dir", p.deps.Dir, "log", logPath)
	result, runErr := p.deps.Run(ctx, p.deps.Dir, expanded, logPath, p.row.Timeout)
	if cerr := cancelErr(ctx, p.name, commandEngine); cerr != nil {
		return "", shedengine.OutputPointer{}, cerr
	}
	switch {
	case runErr != nil:
		logger.Warn("shedadapters: command row's command could not run; passing on", "producer", p.name, "engine", commandEngine, "log", logPath, "error", runErr)
	case result.TimedOut:
		logger.Warn("shedadapters: command row's command timed out; passing on", "producer", p.name, "engine", commandEngine, "log", logPath, "timeout", p.row.Timeout)
	case result.ExitCode != 0:
		logger.Warn("shedadapters: command row's command failed; passing on", "producer", p.name, "engine", commandEngine, "log", logPath, "exit_code", result.ExitCode)
	default:
		logger.Info("shedadapters: command row's command exited", "producer", p.name, "engine", commandEngine, "log", logPath)
	}
	return done, shedengine.OutputPointer{}, nil
}

// warnOnIgnoredCommand warns when the owner's ordinary stack answers a different non-empty command than the one the row runs, since only the template and the board default layer are read.
// A failed read of the ordinary stack skips the comparison and never stops the row.
func (p *CommandProducer) warnOnIgnoredCommand(command, setCommand string) {
	other, err := p.deps.Setting(p.row.CommandModule, p.row.CommandKey)
	if err != nil {
		logger.Info("shedadapters: command row could not read the command key over the ordinary stack; skipping the ignored-value check", "producer", p.name, "engine", commandEngine, "error", err)
		return
	}
	if text, isString := other.(string); isString && text != "" && text != command {
		logger.Warn("shedadapters: command row ignores a command set in another layer; only the template and the board default layer are read", "producer", p.name, "engine", commandEngine, "setting", p.row.CommandModule+"."+p.row.CommandKey, "way_forward", "set it with "+setCommand)
	}
}

// refusedPlaceholderWayForward names how to avoid a placeholder whose value cmd cannot quote.
func refusedPlaceholderWayForward(placeholder string) string {
	switch placeholder {
	case "{task_dir}":
		return "put the hub at a path without a percent sign"
	case "{lyx}":
		return "build lyx into a Go bin dir whose path has no percent sign"
	default:
		return "use a " + placeholder + " value without a percent sign"
	}
}

// expandPlaceholders replaces each {name} of values in command with the value quoted by verifyrun.QuoteArg for goos, in one pass, so brace text inside a value is never expanded again.
// Any other brace text, ${HOME} included, passes through literally.
// A value QuoteArg refuses returns that error and the refused placeholder, such as "{task_dir}", with no expanded text.
func expandPlaceholders(command, goos string, values map[string]string) (expanded, refusedPlaceholder string, err error) {
	names := make([]string, 0, len(values))
	for name := range values {
		names = append(names, name)
	}
	slices.Sort(names)

	pairs := make([]string, 0, 2*len(names))
	for _, name := range names {
		quoted, quoteErr := verifyrun.QuoteArg(goos, values[name])
		if quoteErr != nil {
			return "", "{" + name + "}", fmt.Errorf("quote {%s}: %w", name, quoteErr)
		}
		pairs = append(pairs, "{"+name+"}", quoted)
	}
	return strings.NewReplacer(pairs...).Replace(command), "", nil
}
