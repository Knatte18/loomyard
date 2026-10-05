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

func TestRun_PassingCommandCapturesBothStreams(t *testing.T) {
	var out bytes.Buffer
	code, err := Run(context.Background(), "echo to-stdout && echo to-stderr 1>&2", t.TempDir(), &out)
	if err != nil || code != 0 {
		t.Fatalf("Run = (%d, %v); want (0, nil)", code, err)
	}
	if !strings.Contains(out.String(), "to-stdout") || !strings.Contains(out.String(), "to-stderr") {
		t.Errorf("out = %q; want both stdout and stderr", out.String())
	}
}

func TestRun_NonZeroExit(t *testing.T) {
	t.Run("Buffer", func(t *testing.T) {
		var out bytes.Buffer
		code, err := Run(context.Background(), "exit 3", t.TempDir(), &out)
		if err != nil || code != 3 {
			t.Fatalf("Run = (%d, %v); want (3, nil)", code, err)
		}
	})
	t.Run("Discard", func(t *testing.T) {
		code, err := Run(context.Background(), "exit 3", t.TempDir(), io.Discard)
		if err != nil || code != 3 {
			t.Fatalf("Run = (%d, %v); want (3, nil)", code, err)
		}
	})
}

func TestRun_WorkingDirectory(t *testing.T) {
	dir, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	cmd := "pwd"
	if runtime.GOOS == "windows" {
		cmd = "cd"
	}
	var out bytes.Buffer
	if code, err := Run(context.Background(), cmd, dir, &out); err != nil || code != 0 {
		t.Fatalf("Run = (%d, %v); want (0, nil)", code, err)
	}
	if got := strings.TrimSpace(out.String()); !strings.EqualFold(got, dir) {
		t.Errorf("working directory = %q; want %q", got, dir)
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
