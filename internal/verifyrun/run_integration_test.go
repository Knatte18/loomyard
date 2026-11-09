// run_integration_test.go exercises Run against real shell processes.

package verifyrun

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// runCapture redirects logger output into a buffer at Info verbosity for one test.
func runCapture(t *testing.T) *bytes.Buffer {
	t.Helper()
	var buf bytes.Buffer
	logger.SetOutput(&buf)
	logger.SetVerbosity(1)
	t.Cleanup(func() {
		logger.SetOutput(logger.DefaultOutput())
		logger.SetVerbosity(0)
	})
	return &buf
}

// TestRun table-drives Run over commands that exit.
// A passing command captures both of its streams.
// A non-zero exit is reported as its code with no error, whether the output goes to a buffer or is discarded.
// The command runs in the told working directory.
func TestRun(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()

	workingDirectory, echoVariable := "pwd", "echo $VERIFYRUN_TOLD_VARIABLE"
	if runtime.GOOS == "windows" {
		workingDirectory, echoVariable = "cd", "echo %VERIFYRUN_TOLD_VARIABLE%"
	}

	quotedValue := `two words, 'single' and "double" and 100%`
	quotedArgument, err := QuoteArg(runtime.GOOS, quotedValue)
	if err != nil && runtime.GOOS != "windows" {
		t.Fatal(err)
	}

	tests := []struct {
		name    string
		command string
		// wantExact, when set, expects the whole output to equal it.
		wantExact string
		// posixOnly marks a row whose command needs a POSIX utility, so Windows generates no subtest for it.
		posixOnly bool
		// discard sends the output to io.Discard instead of a buffer.
		discard bool
		// env is the told environment; nil runs with the parent's.
		env        []string
		wantCode   int
		wantOutput []string
		// wantOutputIsDir expects the whole trimmed output to be the working directory.
		wantOutputIsDir bool
	}{
		{
			name:       "passing command captures both streams",
			command:    "echo to-stdout && echo to-stderr 1>&2",
			wantOutput: []string{"to-stdout", "to-stderr"},
		},
		{
			name:       "a variable set only in the told environment reaches the shell",
			command:    echoVariable,
			env:        append(os.Environ(), "VERIFYRUN_TOLD_VARIABLE=from-told-env"),
			wantOutput: []string{"from-told-env"},
		},
		{
			name:       "a quoted argument reaches the command as one literal",
			command:    "printf '%s|' " + quotedArgument,
			wantOutput: []string{quotedValue + "|"},
			wantExact:  quotedValue + "|",
			posixOnly:  true,
		},
		{name: "non-zero exit into a buffer", command: "exit 3", wantCode: 3},
		{name: "non-zero exit with output discarded", command: "exit 3", discard: true, wantCode: 3},
		{
			name:            "runs in the told working directory",
			command:         workingDirectory,
			wantOutputIsDir: true,
		},
	}
	for _, tt := range tests {
		if tt.posixOnly && runtime.GOOS == "windows" {
			continue
		}
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			dir, err := filepath.EvalSymlinks(t.TempDir())
			if err != nil {
				t.Fatal(err)
			}
			var out bytes.Buffer
			var sink io.Writer = &out
			if tt.discard {
				sink = io.Discard
			}

			code, err := Run(context.Background(), tt.command, dir, tt.env, sink)

			if err != nil || code != tt.wantCode {
				t.Fatalf("Run = (%d, %v); want (%d, nil)", code, err, tt.wantCode)
			}
			for _, want := range tt.wantOutput {
				if !strings.Contains(out.String(), want) {
					t.Errorf("out = %q; want it to contain %q", out.String(), want)
				}
			}
			if tt.wantExact != "" && out.String() != tt.wantExact {
				t.Errorf("out = %q; want %q", out.String(), tt.wantExact)
			}
			if tt.wantOutputIsDir {
				if got := strings.TrimSpace(out.String()); !strings.EqualFold(got, dir) {
					t.Errorf("working directory = %q; want %q", got, dir)
				}
			}
		})
	}
}

func TestRun_SpawnFailure(t *testing.T) {
	tiergate.Require(t, 1)
	buf := runCapture(t)
	t.Setenv("PATH", "")

	code, err := Run(context.Background(), "true", t.TempDir(), nil, &bytes.Buffer{})
	if err == nil || code != -1 {
		t.Fatalf("Run = (%d, %v); want (-1, non-nil)", code, err)
	}
	log := buf.String()
	if !strings.Contains(log, "WARN") || !strings.Contains(log, "cause") {
		t.Errorf("log = %q; want a WARN line carrying cause", log)
	}
}

// firstWrite is a writer that closes seen on its first write and discards everything.
type firstWrite struct {
	once sync.Once
	seen chan struct{}
}

func (w *firstWrite) Write(p []byte) (int, error) {
	w.once.Do(func() { close(w.seen) })
	return len(p), nil
}

func TestRun_Cancellation(t *testing.T) {
	tiergate.Require(t, 1)
	buf := runCapture(t)
	// The sleep starts before the first write, so that write means a child is running.
	// On Windows ping prints its banner once it runs.
	// The command outlasts the minute the test allows after the cancel,
	// so a run that waits on it fails however loaded the machine.
	long := "sleep 300 & echo up; wait"
	if runtime.GOOS == "windows" {
		long = "ping -n 300 127.0.0.1"
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	// The command's own first output says it runs, so the cancel needs no fixed pause.
	out := &firstWrite{seen: make(chan struct{})}
	cancelled := make(chan time.Time, 1)
	go func() {
		<-out.seen
		cancelled <- time.Now()
		cancel()
	}()

	const delay = 500 * time.Millisecond
	code, err := run(ctx, long, t.TempDir(), nil, out, delay)
	returned := time.Now()
	if !errors.Is(err, context.Canceled) || code != -1 {
		t.Fatalf("run = (%d, %v); want (-1, context.Canceled)", code, err)
	}
	if elapsed := returned.Sub(<-cancelled); elapsed > time.Minute {
		t.Errorf("run took %s after cancel; want well under the command's own duration", elapsed)
	}
	if strings.Contains(buf.String(), "WARN") {
		t.Errorf("log = %q; a cancellation must not warn", buf.String())
	}
	if runtime.GOOS != "windows" && !strings.Contains(buf.String(), "sending SIGTERM") {
		t.Errorf("log = %q; want the SIGTERM Info line", buf.String())
	}
}
