//go:build integration

// run_integration_test.go exercises Run against real shell processes, so it carries the integration tag.

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
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/logger"
)

// runCapture redirects logger output into a buffer at Info verbosity for one test.
func runCapture(t *testing.T) *bytes.Buffer {
	t.Helper()
	var buf bytes.Buffer
	logger.SetOutput(&buf)
	logger.SetVerbosity(1)
	t.Cleanup(func() {
		logger.SetOutput(os.Stderr)
		logger.SetVerbosity(0)
	})
	return &buf
}

// TestRun table-drives Run over a command that exits: one that passes captures both of its streams,
// a non-zero exit is reported as its code with no error, whether the output goes to a buffer or is
// discarded, and the command runs in the told working directory.
func TestRun(t *testing.T) {
	t.Parallel()

	workingDirectory := "pwd"
	if runtime.GOOS == "windows" {
		workingDirectory = "cd"
	}

	tests := []struct {
		name    string
		command string
		// discard sends the output to io.Discard instead of a buffer.
		discard    bool
		wantCode   int
		wantOutput []string
		// wantOutputIsDir expects the whole trimmed output to be the working directory instead.
		wantOutputIsDir bool
	}{
		{
			name:       "passing command captures both streams",
			command:    "echo to-stdout && echo to-stderr 1>&2",
			wantOutput: []string{"to-stdout", "to-stderr"},
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

			code, err := Run(context.Background(), tt.command, dir, sink)

			if err != nil || code != tt.wantCode {
				t.Fatalf("Run = (%d, %v); want (%d, nil)", code, err, tt.wantCode)
			}
			for _, want := range tt.wantOutput {
				if !strings.Contains(out.String(), want) {
					t.Errorf("out = %q; want it to contain %q", out.String(), want)
				}
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
	buf := runCapture(t)
	t.Setenv("PATH", "")

	code, err := Run(context.Background(), "true", t.TempDir(), &bytes.Buffer{})
	if err == nil || code != -1 {
		t.Fatalf("Run = (%d, %v); want (-1, non-nil)", code, err)
	}
	log := buf.String()
	if !strings.Contains(log, "WARN") || !strings.Contains(log, "cause") {
		t.Errorf("log = %q; want a WARN line carrying cause", log)
	}
}

func TestRun_Cancellation(t *testing.T) {
	buf := runCapture(t)
	long := "sleep 30"
	if runtime.GOOS == "windows" {
		long = "ping -n 30 127.0.0.1"
	}
	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(300 * time.Millisecond)
		cancel()
	}()

	const delay = 500 * time.Millisecond
	start := time.Now()
	code, err := run(ctx, long, t.TempDir(), &bytes.Buffer{}, delay)
	if !errors.Is(err, context.Canceled) || code != -1 {
		t.Fatalf("run = (%d, %v); want (-1, context.Canceled)", code, err)
	}
	if elapsed := time.Since(start); elapsed > 300*time.Millisecond+4*delay {
		t.Errorf("run took %s after cancel; want well under the command's own duration", elapsed)
	}
	if strings.Contains(buf.String(), "WARN") {
		t.Errorf("log = %q; a cancellation must not warn", buf.String())
	}
}
