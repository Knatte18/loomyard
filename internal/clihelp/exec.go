// exec.go provides the per-invocation exit-state holder, the RunCLI seam adapter,
// and the legacy-handler-to-RunE wrapper for the clihelp package.
//
// The exit state is carried as a context value (never a package-level variable) so
// that concurrent test invocations each track their own exit code without races.

// Package clihelp provides the shared cobra infrastructure used by cmd/lyx and every
// module's RunCLI seam. It holds per-invocation exit state, a seam adapter that wires
// a cobra command tree into an io.Writer-based call contract, and a RunE wrapper that
// bridges legacy handler functions into cobra's RunE signature.
package clihelp

import (
	"context"
	"io"

	"github.com/spf13/cobra"
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

// Execute is the RunCLI seam used by every module and by cmd/lyx.
// It wires cmd to write both stdout and stderr into out (so in-process tests
// capture all output from a single buffer), sets SilenceUsage so cobra does not
// dump a full usage block on error paths, runs the command tree with args, and
// returns the recorded exit code (or 1 for a cobra-level error such as an unknown
// command or bad flag). SilenceErrors is left at its default false so cobra still
// writes "unknown command"/"unknown flag" messages into out.
func Execute(cmd *cobra.Command, out io.Writer, args []string) int {
	// Merge stdout and stderr into a single writer so in-process tests capture
	// cobra's error text from the same buffer as handler output.
	cmd.SetOut(out)
	cmd.SetErr(out)

	// Suppress the full usage block on error paths; the command path already
	// identifies the problem well enough, and the block bloats test snapshots.
	cmd.SilenceUsage = true

	ctx, es := NewExitContext(context.Background())
	cmd.SetArgs(args)

	if err := cmd.ExecuteContext(ctx); err != nil {
		return 1
	}
	return es.code
}
