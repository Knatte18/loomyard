// exec.go provides the per-invocation exit-state holder, the RunCLI seam adapter,
// the legacy-handler-to-RunE wrapper, the shared root-execution helper, and the
// module-group RunE helper for the clihelp package.
//
// The exit state is carried as a context value (never a package-level variable) so
// that concurrent test invocations each track their own exit code without races.

// Package clihelp provides the shared cobra infrastructure used by cmd/lyx and every
// module's RunCLI seam. It holds per-invocation exit state, a seam adapter that wires
// a cobra command tree into an io.Writer-based call contract, a RunE wrapper that
// bridges legacy handler functions into cobra's RunE signature, and helpers that
// centralise JSON-envelope error wrapping for Cobra-level errors.
package clihelp

import (
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/spf13/cobra"

	"github.com/Knatte18/loomyard/internal/output"
)

// ctxKey is an unexported type for context keys in this package,
// preventing collisions with keys defined in other packages.
type ctxKey struct{}

// exitKey is the context key under which an *exitState is stored.
var exitKey = ctxKey{}

// exitState records the exit code and abort flag for a single CLI invocation.
// It is allocated fresh per invocation by NewExitContext and is never shared
// across invocations, making it safe for concurrent use across parallel tests.
type exitState struct {
	code  int
	abort bool
}

// Code returns the recorded exit code, which is 0 if SetExit was never called with
// a non-zero value. It is the public read accessor for the unexported code field so
// that cmd/lyx/main.go can read the exit code after ExecuteContext returns without
// importing the unexported field directly.
func (es *exitState) Code() int {
	return es.code
}

// NewExitContext allocates a fresh *exitState, stores it in a child context derived
// from parent, and returns both. Call this once per CLI invocation (in Execute or
// in each module's RunCLI seam); never store the returned *exitState in a package-level
// variable — doing so would break parallel-test isolation.
func NewExitContext(parent context.Context) (context.Context, *exitState) {
	es := &exitState{}
	return context.WithValue(parent, exitKey, es), es
}

// exitStateFromCtx retrieves the *exitState stored in ctx by NewExitContext.
// Returns nil if ctx carries no exit state.
func exitStateFromCtx(ctx context.Context) *exitState {
	es, _ := ctx.Value(exitKey).(*exitState)
	return es
}

// SetExit records code in the exit state carried by ctx.
// It is a no-op when code is zero (zero means success; callers must not
// override a failure code with a spurious zero) or when ctx carries no exit state.
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

// Abort records code in the exit state carried by ctx and sets the abort flag,
// signalling that subsequent RunE bodies should short-circuit without executing.
// Used by a PersistentPreRunE that detects a fatal setup error so that leaf
// commands do not run against an uninitialised environment.
// Abort is a no-op when code is zero or when ctx carries no exit state.
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
// A RunE body should check this at its start and return nil immediately when true,
// because a PersistentPreRunE has already written an error response and recorded
// the exit code.
func ShouldAbort(ctx context.Context) bool {
	es := exitStateFromCtx(ctx)
	if es == nil {
		return false
	}
	return es.abort
}

// WrapRun adapts a legacy handler function with signature func(io.Writer, []string) int
// into a cobra RunE. The returned RunE short-circuits (returns nil without calling fn)
// when Abort has been signalled on the command's context, so that a failing
// PersistentPreRunE prevents any leaf command body from running. Otherwise it calls fn,
// passes its exit code to SetExit, and returns nil so cobra does not double-print over
// any JSON output the handler already wrote.
func WrapRun(fn func(out io.Writer, args []string) int) func(*cobra.Command, []string) error {
	return func(cmd *cobra.Command, args []string) error {
		// Short-circuit when a PersistentPreRunE signalled abort; the pre-run
		// already wrote the error response and recorded the exit code.
		if ShouldAbort(cmd.Context()) {
			return nil
		}
		SetExit(cmd.Context(), fn(cmd.OutOrStdout(), args))
		return nil
	}
}

// RunRoot sets SilenceErrors and SilenceUsage on cmd, seeds a fresh exit context,
// runs cmd.ExecuteContext, and on a non-nil cobra error writes the JSON error envelope
// to out and returns 1. On nil error it returns the exit code recorded via SetExit.
// Callers must configure cmd's output writers and args before calling RunRoot; this
// function only supplies the context, the silence flags, and the error-wrapping policy.
// Both Execute and cmd/lyx use RunRoot so the wrapping logic has exactly one implementation.
func RunRoot(cmd *cobra.Command, out io.Writer) int {
	// Silence cobra's own error printing; we emit a JSON envelope instead so the
	// caller always gets a machine-parseable error shape rather than plain text.
	cmd.SilenceErrors = true

	// Suppress the usage block on error paths; the command path already identifies
	// the problem well enough, and a usage dump bloats test snapshots.
	cmd.SilenceUsage = true

	ctx, es := NewExitContext(context.Background())
	if err := cmd.ExecuteContext(ctx); err != nil {
		// Wrap the cobra error in the standard JSON envelope so all error paths
		// (unknown command, bad flag, arg validation) have the same shape as domain
		// errors. TrimSpace strips any newline cobra appends to its error strings.
		return output.Err(out, strings.TrimSpace(err.Error()))
	}
	return es.code
}

// Execute is the RunCLI seam used by every module and by cmd/lyx.
// It wires cmd to write both stdout and stderr into out (so in-process tests
// capture all output from a single buffer), sets args, and delegates to RunRoot
// for context seeding, silence flags, execution, and JSON error wrapping.
func Execute(cmd *cobra.Command, out io.Writer, args []string) int {
	// Merge stdout and stderr into a single writer so in-process tests capture
	// cobra's error text from the same buffer as handler output.
	cmd.SetOut(out)
	cmd.SetErr(out)

	cmd.SetArgs(args)

	return RunRoot(cmd, out)
}

// GroupRunE is the RunE for parent module group commands (e.g. "lyx warp", "lyx board").
// When args is non-empty it returns an error naming the unknown subcommand; when args is
// empty it delegates to the command's built-in help output. Wire this as
// cmd.RunE = clihelp.GroupRunE on each group command so that bare invocations print help
// and invocations with an unrecognised subcommand emit a JSON error envelope via RunRoot.
func GroupRunE(cmd *cobra.Command, args []string) error {
	if len(args) > 0 {
		return fmt.Errorf("unknown subcommand %q for %q", args[0], cmd.CommandPath())
	}
	return cmd.Help()
}
