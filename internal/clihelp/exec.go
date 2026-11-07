// exec.go holds the clihelp package's exit state, its RunCLI seam, and its RunE wrappers and root-execution helpers.

// Package clihelp provides the shared cobra infrastructure used by cmd/lym and every module's RunCLI seam.
// It holds the per-invocation exit state.
// Its seam adapter wires a cobra command tree into an io.Writer-based call contract.
// It also holds the RunE wrapper for legacy handlers and the helpers that wrap Cobra errors in the JSON envelope.
//
// It owns the global --json flag (InstallJSONHelp).
// Under it, every command, leaf or group, prints its help as structured JSON and exits 0, and no command runs.
// A command that declares its own local --json flag shadows the global one and keeps its own meaning.
//
// It owns the readable summary of operator verbs (UseReadableOutput).
// Operator-audience commands print each JSON envelope line as a short summary when their output is a terminal;
// the global --envelope flag keeps the envelope there.
// An output that is no terminal, such as an agent's or a script's, always gets the envelope.
//
// The command index appends a command's index note (IndexNoteAnnotation).
// It prints the note only in the audience that IndexNoteAudienceAnnotation names;
// a note without that annotation never prints.
package clihelp

import (
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/spf13/cobra"

	"github.com/Knatte18/loomyard/internal/lymcwd"
	"github.com/Knatte18/loomyard/internal/output"
)

// init clears cobra's Windows mousetrap help text.
// The mousetrap check costs startup time in the test suite.
// lym is never launched by double-click, so the text is never shown.
func init() {
	cobra.MousetrapHelpText = ""
}

// ctxKey is an unexported type for context keys in this package,
// preventing collisions with keys defined in other packages.
type ctxKey struct{}

// exitKey is the context key under which an *exitState is stored.
var exitKey = ctxKey{}

// exitState records the exit code, the abort flag, and the steps to run once execution ends for one CLI invocation.
// It is allocated fresh per invocation by NewExitContext and is never shared across invocations.
// That makes it safe for concurrent use across parallel tests.
type exitState struct {
	code   int
	abort  bool
	finish []func()
}

// onFinish registers step to run once the invocation carried by ctx ends execution.
// Steps run in registration order, before any cobra error envelope is written.
// It is a no-op when ctx carries no exit state.
func onFinish(ctx context.Context, step func()) {
	es := exitStateFromCtx(ctx)
	if es == nil {
		return
	}
	es.finish = append(es.finish, step)
}

// Code returns the recorded exit code, which is 0 unless a non-zero code was recorded.
// It exists so code outside clihelp can read the exit code, since the field itself is unexported.
func (es *exitState) Code() int {
	return es.code
}

// NewExitContext allocates a fresh *exitState, stores it in a child context derived from parent, and returns both.
// Call it once per CLI invocation.
// Never store the returned *exitState in a package-level variable, since that would break parallel-test isolation.
func NewExitContext(parent context.Context) (context.Context, *exitState) {
	es := &exitState{}
	return context.WithValue(parent, exitKey, es), es
}

// exitStateFromCtx returns the exit state carried by ctx, or nil when ctx carries none.
func exitStateFromCtx(ctx context.Context) *exitState {
	es, _ := ctx.Value(exitKey).(*exitState)
	return es
}

// SetExit records code as the exit code of the invocation carried by ctx.
// A zero code is ignored, so a success never overwrites a failure code.
// It is also a no-op when ctx carries no exit state.
func SetExit(ctx context.Context, code int) {
	if code == 0 {
		return
	}
	es := exitStateFromCtx(ctx)
	if es == nil {
		return
	}
	es.code = code
}

// Abort records code as the exit code of the invocation carried by ctx and sets the abort flag.
// Subsequent RunE bodies then short-circuit without executing.
// A PersistentPreRunE calls it on a fatal setup error.
// That keeps leaf commands from running against an uninitialised environment.
// It is a no-op when code is zero or when ctx carries no exit state.
func Abort(ctx context.Context, code int) {
	if code == 0 {
		return
	}
	es := exitStateFromCtx(ctx)
	if es == nil {
		return
	}
	es.code = code
	es.abort = true
}

// ShouldAbort reports whether Abort was called on the exit state carried by ctx.
// A RunE body should return nil at its start when this reports true.
// A PersistentPreRunE that aborted has already written the error response and recorded the exit code.
func ShouldAbort(ctx context.Context) bool {
	es := exitStateFromCtx(ctx)
	if es == nil {
		return false
	}
	return es.abort
}

// WrapRun adapts a legacy handler function with signature func(io.Writer, []string) int into a cobra RunE.
// The returned RunE skips fn and returns nil when Abort was called on the command's context,
// so a failing PersistentPreRunE keeps every leaf command body from running.
// Otherwise it passes fn's exit code to SetExit.
// It always returns nil, so cobra does not double-print over JSON output the handler already wrote.
func WrapRun(fn func(out io.Writer, args []string) int) func(*cobra.Command, []string) error {
	return func(cmd *cobra.Command, args []string) error {
		// The PersistentPreRunE that aborted has already written the error response and recorded the exit code.
		if ShouldAbort(cmd.Context()) {
			return nil
		}
		SetExit(cmd.Context(), fn(cmd.OutOrStdout(), args))
		return nil
	}
}

// WrapRunCtx adapts a handler with signature func(context.Context, io.Writer, []string) int into a cobra RunE.
// The handler receives cmd.Context() as its first argument, so it can read an injected cwd through lymcwd.CwdFrom.
// The returned RunE skips fn and returns nil when Abort was called on the command's context.
// Otherwise it passes fn's exit code to SetExit.
// It always returns nil, so cobra does not double-print over JSON output the handler already wrote.
func WrapRunCtx(fn func(ctx context.Context, out io.Writer, args []string) int) func(*cobra.Command, []string) error {
	return func(cmd *cobra.Command, args []string) error {
		// The PersistentPreRunE that aborted has already written the error response and recorded the exit code.
		if ShouldAbort(cmd.Context()) {
			return nil
		}
		SetExit(cmd.Context(), fn(cmd.Context(), cmd.OutOrStdout(), args))
		return nil
	}
}

// RunRootCtx runs cmd under a fresh exit state derived from ctx, and returns the exit code.
// It silences cobra's error and usage output.
// On a cobra error it writes the JSON error envelope to out and returns 1.
// Otherwise it returns the exit code recorded by SetExit.
// Callers configure cmd's output writers and args before the call;
// this function supplies only the context, the silence flags, and the error wrapping.
func RunRootCtx(ctx context.Context, cmd *cobra.Command, out io.Writer) int {
	// Silence cobra's own error printing: the JSON envelope replaces it, so callers get a parseable error shape.
	cmd.SilenceErrors = true

	// Suppress the usage block on error paths: the command path already names the problem,
	// and a usage dump bloats test snapshots.
	cmd.SilenceUsage = true

	execCtx, es := NewExitContext(ctx)
	err := cmd.ExecuteContext(execCtx)
	for _, step := range es.finish {
		step()
	}
	if err != nil {
		// Wrap the cobra error in the standard JSON envelope.
		// Unknown commands, bad flags and argument errors then share the shape of domain errors.
		// TrimSpace strips the newline cobra appends to its error strings.
		return output.Err(out, strings.TrimSpace(err.Error()))
	}
	return es.code
}

// RunRoot runs cmd under a fresh exit state with a background context, and returns the exit code.
// It silences cobra's error and usage output.
// On a cobra error it writes the JSON error envelope to out and returns 1.
// Callers configure cmd's output writers and args first.
// cmd/lym and Execute both go through RunRoot, so the error wrapping has one implementation.
func RunRoot(cmd *cobra.Command, out io.Writer) int {
	return RunRootCtx(context.Background(), cmd, out)
}

// Execute is the RunCLI seam the modules use to run a command tree in process.
// It writes stdout and stderr both into out, so in-process tests capture all output from one buffer.
// It sets args and delegates to RunRoot for the context, the silence flags, and the JSON error wrapping.
func Execute(cmd *cobra.Command, out io.Writer, args []string) int {
	// Merge stdout and stderr into one writer so in-process tests capture cobra's error text with the handler output.
	cmd.SetOut(out)
	cmd.SetErr(out)

	cmd.SetArgs(args)

	return RunRoot(cmd, out)
}

// ExecuteIn runs cmd with cwd seeded into its execution context.
// Command bodies that call lymcwd.CwdFrom(cmd.Context()) see cwd instead of the process working directory.
// It sets cmd's output and arguments first, then delegates to RunRootCtx.
// cwd must be an absolute, non-empty path: lymcwd.WithCwd panics on an empty or relative cwd.
func ExecuteIn(cmd *cobra.Command, cwd string, out io.Writer, args []string) int {
	// Stdout and stderr share one writer.
	// An in-process test then reads cobra's error text beside the handler output.
	cmd.SetOut(out)
	cmd.SetErr(out)

	cmd.SetArgs(args)

	return RunRootCtx(lymcwd.WithCwd(context.Background(), cwd), cmd, out)
}

// GroupRunE is the RunE for a parent command whose children are subcommands.
// With arguments it returns an error naming the unknown subcommand.
// Without arguments it prints the command's help.
// Wire it as cmd.RunE = clihelp.GroupRunE on each group command: a bare invocation then prints help,
// and an unknown subcommand emits a JSON error envelope through RunRoot.
func GroupRunE(cmd *cobra.Command, args []string) error {
	if len(args) > 0 {
		return fmt.Errorf("unknown subcommand %q for %q", args[0], cmd.CommandPath())
	}
	return cmd.Help()
}
