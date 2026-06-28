// exec_test.go tests the exit-state holder, the Execute seam adapter, and WrapRun.
// Tests use synthetic cobra command trees built in-test — no real lyx commands.

package clihelp

import (
	"bytes"
	"encoding/json"
	"io"
	"strings"
	"sync"
	"testing"

	"github.com/spf13/cobra"
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
