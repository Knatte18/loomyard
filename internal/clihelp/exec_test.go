// exec_test.go tests the exit-state holder, the Execute seam adapter, and WrapRun.
// Tests use synthetic cobra command trees built in-test — no real lyx commands.

package clihelp

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"strings"
	"sync"
	"testing"

	"github.com/spf13/cobra"

	"github.com/Knatte18/loomyard/internal/lyxcwd"
)

// handlerReturning returns a WrapRun-compatible handler function that writes
// nothing and returns the given exit code.
func handlerReturning(code int) func(io.Writer, []string) int {
	return func(_ io.Writer, _ []string) int { return code }
}

func TestExecute_SuccessHandlerReturnsZero(t *testing.T) {
	t.Parallel()

	root := &cobra.Command{Use: "root", Short: "test root"}
	root.AddCommand(&cobra.Command{
		Use:  "ok",
		RunE: WrapRun(handlerReturning(0)),
	})

	var buf bytes.Buffer
	got := Execute(root, &buf, []string{"ok"})
	if got != 0 {
		t.Errorf("Execute(ok) = %d; want 0", got)
	}
}

func TestExecute_FailHandlerReturnsOne(t *testing.T) {
	t.Parallel()

	root := &cobra.Command{Use: "root", Short: "test root"}
	root.AddCommand(&cobra.Command{
		Use:  "fail",
		RunE: WrapRun(handlerReturning(1)),
	})

	var buf bytes.Buffer
	got := Execute(root, &buf, []string{"fail"})
	if got != 1 {
		t.Errorf("Execute(fail) = %d; want 1", got)
	}
}

func TestExecute_UnknownSubcommandReturnsOneAndWritesUnknownCommand(t *testing.T) {
	t.Parallel()

	root := &cobra.Command{Use: "root", Short: "test root"}
	root.AddCommand(&cobra.Command{Use: "known", Short: "known sub"})

	var buf bytes.Buffer
	got := Execute(root, &buf, []string{"bogus"})
	if got != 1 {
		t.Errorf("Execute(bogus) = %d; want 1", got)
	}

	// The cobra error message must still be present — now embedded in the JSON value.
	if !strings.Contains(buf.String(), "unknown command") {
		t.Errorf("Execute(bogus) output = %q; want to contain \"unknown command\"", buf.String())
	}

	// The output must be a well-formed JSON envelope with ok=false.
	var env map[string]any
	if err := json.Unmarshal([]byte(strings.TrimSpace(buf.String())), &env); err != nil {
		t.Errorf("Execute(bogus) output is not valid JSON: %v; output: %q", err, buf.String())
	} else if ok, _ := env["ok"].(bool); ok {
		t.Errorf("Execute(bogus) envelope ok = true; want false")
	}
}

func TestWrapRun_ShortCircuitsAfterAbort(t *testing.T) {
	t.Parallel()

	// Track whether the leaf RunE body ran.
	ran := false

	root := &cobra.Command{
		Use:   "root",
		Short: "test root",
		// PersistentPreRunE signals abort before any leaf RunE fires.
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			Abort(cmd.Context(), 2)
			return nil
		},
	}
	sub := &cobra.Command{
		Use:  "leaf",
		RunE: WrapRun(func(_ io.Writer, _ []string) int { ran = true; return 0 }),
	}
	root.AddCommand(sub)

	var buf bytes.Buffer
	code := Execute(root, &buf, []string{"leaf"})

	if ran {
		t.Error("WrapRun: leaf body ran after Abort; want short-circuit")
	}
	if code != 2 {
		t.Errorf("Execute after Abort = %d; want 2", code)
	}
}

func TestExecute_ConcurrentInvocationsDoNotCrossExitCodes(t *testing.T) {
	t.Parallel()

	// Run two concurrent Execute calls — one returning 0, one returning 7 —
	// and assert that each reports its own code. This guards the per-invocation
	// holder invariant: if exitState were a package-level variable the codes
	// would race and at least one assertion would flake.
	const iterations = 50

	var wg sync.WaitGroup
	wg.Add(2 * iterations)

	for i := 0; i < iterations; i++ {
		// Success invocation: always expects 0.
		go func() {
			defer wg.Done()
			root := &cobra.Command{Use: "root"}
			root.AddCommand(&cobra.Command{
				Use:  "sub",
				RunE: WrapRun(handlerReturning(0)),
			})
			var buf bytes.Buffer
			if code := Execute(root, &buf, []string{"sub"}); code != 0 {
				t.Errorf("concurrent success invocation = %d; want 0", code)
			}
		}()

		// Failure invocation: always expects 7.
		go func() {
			defer wg.Done()
			root := &cobra.Command{Use: "root"}
			root.AddCommand(&cobra.Command{
				Use:  "sub",
				RunE: WrapRun(handlerReturning(7)),
			})
			var buf bytes.Buffer
			if code := Execute(root, &buf, []string{"sub"}); code != 7 {
				t.Errorf("concurrent failure invocation = %d; want 7", code)
			}
		}()
	}

	wg.Wait()
}

// TestExecuteIn_HandlerObservesInjectedCwd verifies that a handler reading
// lyxcwd.CwdFrom(cmd.Context()) observes the exact directory passed to
// ExecuteIn.
func TestExecuteIn_HandlerObservesInjectedCwd(t *testing.T) {
	t.Parallel()

	const want = "/injected/cwd"
	var got string

	root := &cobra.Command{Use: "root", Short: "test root"}
	root.AddCommand(&cobra.Command{
		Use: "where",
		RunE: WrapRunCtx(func(ctx context.Context, _ io.Writer, _ []string) int {
			cwd, err := lyxcwd.CwdFrom(ctx)
			if err != nil {
				t.Fatalf("lyxcwd.CwdFrom() error = %v; want nil", err)
			}
			got = cwd
			return 0
		}),
	})

	var buf bytes.Buffer
	if code := ExecuteIn(root, want, &buf, []string{"where"}); code != 0 {
		t.Errorf("ExecuteIn(where) = %d; want 0", code)
	}
	if got != want {
		t.Errorf("lyxcwd.CwdFrom(cmd.Context()) = %q; want %q", got, want)
	}
}

// TestExecute_HandlerObservesProcessCwd verifies that the same command
// driven through Execute (not ExecuteIn) observes the process cwd instead
// of an injected one.
func TestExecute_HandlerObservesProcessCwd(t *testing.T) {
	t.Parallel()

	want, err := lyxcwd.Getwd()
	if err != nil {
		t.Fatalf("lyxcwd.Getwd() error = %v; want nil", err)
	}
	var got string

	root := &cobra.Command{Use: "root", Short: "test root"}
	root.AddCommand(&cobra.Command{
		Use: "where",
		RunE: WrapRunCtx(func(ctx context.Context, _ io.Writer, _ []string) int {
			cwd, cwdErr := lyxcwd.CwdFrom(ctx)
			if cwdErr != nil {
				t.Fatalf("lyxcwd.CwdFrom() error = %v; want nil", cwdErr)
			}
			got = cwd
			return 0
		}),
	})

	var buf bytes.Buffer
	if code := Execute(root, &buf, []string{"where"}); code != 0 {
		t.Errorf("Execute(where) = %d; want 0", code)
	}
	if got != want {
		t.Errorf("lyxcwd.CwdFrom(cmd.Context()) via Execute = %q; want process cwd %q", got, want)
	}
}

// TestRunRootCtx_PropagatesContextValue verifies that RunRootCtx given a
// context carrying a value propagates that value into the command's
// context.
func TestRunRootCtx_PropagatesContextValue(t *testing.T) {
	t.Parallel()

	const want = "/propagated/cwd"
	var got string

	root := &cobra.Command{Use: "root", Short: "test root"}
	root.AddCommand(&cobra.Command{
		Use: "leaf",
		RunE: WrapRunCtx(func(ctx context.Context, _ io.Writer, _ []string) int {
			cwd, err := lyxcwd.CwdFrom(ctx)
			if err != nil {
				t.Fatalf("lyxcwd.CwdFrom() error = %v; want nil", err)
			}
			got = cwd
			return 0
		}),
	})
	root.SetArgs([]string{"leaf"})

	var buf bytes.Buffer
	root.SetOut(&buf)
	root.SetErr(&buf)

	ctx := lyxcwd.WithCwd(context.Background(), want)
	if code := RunRootCtx(ctx, root, &buf); code != 0 {
		t.Errorf("RunRootCtx() = %d; want 0", code)
	}
	if got != want {
		t.Errorf("propagated cwd = %q; want %q", got, want)
	}
}

// TestWrapRunCtx_ReceivesCommandContext verifies that a WrapRunCtx-wrapped
// handler receives the command's own context, observed by reading a value
// seeded into it.
func TestWrapRunCtx_ReceivesCommandContext(t *testing.T) {
	t.Parallel()

	const want = "/seeded/cwd"
	var got string

	root := &cobra.Command{Use: "root", Short: "test root"}
	root.AddCommand(&cobra.Command{
		Use: "leaf",
		RunE: WrapRunCtx(func(ctx context.Context, _ io.Writer, _ []string) int {
			cwd, err := lyxcwd.CwdFrom(ctx)
			if err != nil {
				t.Fatalf("lyxcwd.CwdFrom() error = %v; want nil", err)
			}
			got = cwd
			return 0
		}),
	})

	var buf bytes.Buffer
	if code := ExecuteIn(root, want, &buf, []string{"leaf"}); code != 0 {
		t.Errorf("ExecuteIn(leaf) = %d; want 0", code)
	}
	if got != want {
		t.Errorf("WrapRunCtx handler observed cwd = %q; want %q", got, want)
	}
}

// TestWrapRunCtx_ShortCircuitsAfterAbort verifies that a WrapRunCtx-wrapped
// handler short-circuits without running when Abort was called on the
// command's context.
func TestWrapRunCtx_ShortCircuitsAfterAbort(t *testing.T) {
	t.Parallel()

	ran := false

	root := &cobra.Command{
		Use:   "root",
		Short: "test root",
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			Abort(cmd.Context(), 2)
			return nil
		},
	}
	sub := &cobra.Command{
		Use: "leaf",
		RunE: WrapRunCtx(func(_ context.Context, _ io.Writer, _ []string) int {
			ran = true
			return 0
		}),
	}
	root.AddCommand(sub)

	var buf bytes.Buffer
	code := Execute(root, &buf, []string{"leaf"})

	if ran {
		t.Error("WrapRunCtx: leaf body ran after Abort; want short-circuit")
	}
	if code != 2 {
		t.Errorf("Execute after Abort = %d; want 2", code)
	}
}
