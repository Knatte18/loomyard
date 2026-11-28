//go:build !windows

// termwindow_integration_test.go runs the window's shell line under bash, with a stub executable standing in for lym.
// It pins when the window ends and when it keeps its shell, and which command line Launcher.Open starts.

package termwindow

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/proc"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

func TestLauncher_OpenRunsTheChosenCommandLine(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()

	program, err := exec.LookPath("sh")
	if err != nil {
		t.Skip("sh is not on PATH")
	}
	tests := []struct {
		name     string
		separate bool
		want     string
	}{
		{name: "TodaysCommandLine", want: "plain"},
		{name: "ProcessOfItsOwn", separate: true, want: "separate"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			marker := filepath.Join(t.TempDir(), "marker")
			launcher := Launcher{
				name:    "sh",
				program: program,
				arguments: func(string, string, string) []string {
					return []string{"-c", fmt.Sprintf("echo plain > %q", marker)}
				},
				separate: func(string, string, string) []string {
					return []string{"-c", fmt.Sprintf("echo separate > %q; exec sleep 60", marker)}
				},
			}

			pid, err := launcher.Open(t.TempDir(), "title", "lym", tt.separate)
			if err != nil {
				t.Fatalf("Open() error = %v", err)
			}
			if tt.separate {
				if pid <= 0 {
					t.Fatalf("Open() pid = %d; want a live process id", pid)
				}
				t.Cleanup(func() {
					if err := proc.KillPID(pid); err != nil {
						t.Errorf("KillPID(%d) = %v", pid, err)
					}
				})
				if parent, ok := proc.ParentPID(pid); !ok || parent != os.Getpid() {
					t.Errorf("ParentPID(%d) = %d, %v; want %d", pid, parent, ok, os.Getpid())
				}
			} else if pid != 0 {
				t.Errorf("Open() pid = %d; want 0", pid)
			}

			deadline := time.After(30 * time.Second)
			tick := time.NewTicker(10 * time.Millisecond)
			defer tick.Stop()
			for {
				data, err := os.ReadFile(marker)
				if err == nil && strings.HasSuffix(string(data), "\n") {
					if got := strings.TrimSpace(string(data)); got != tt.want {
						t.Errorf("marker = %q; want %q", got, tt.want)
					}
					return
				}
				select {
				case <-deadline:
					t.Fatalf("no marker written: %v", err)
				case <-tick.C:
				}
			}
		})
	}
}

func TestAttachLine_EndsOrKeepsTheShellByAttachStatus(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()

	if _, err := exec.LookPath("bash"); err != nil {
		t.Skip("bash is not on PATH")
	}

	tests := []struct {
		name string
		// stub is the script body after the shebang, given the pair directory.
		stub          func(dir string) string
		dirPresent    bool
		waitPolls     int
		pollInterval  time.Duration
		wantStatus    bool
		wantKeptShell bool
	}{
		{
			name:         "a stub exiting 0 ends without the shell",
			stub:         func(string) string { return "exit 0" },
			dirPresent:   true,
			waitPolls:    1,
			pollInterval: time.Second,
		},
		{
			name:         "a stub exiting 3 with the directory absent prints the status and ends",
			stub:         func(string) string { return "exit 3" },
			waitPolls:    1,
			pollInterval: time.Second,
			wantStatus:   true,
		},
		{
			name:          "a stub exiting 3 with the directory present prints the status and keeps the shell",
			stub:          func(string) string { return "exit 3" },
			dirPresent:    true,
			waitPolls:     1,
			pollInterval:  time.Second,
			wantStatus:    true,
			wantKeptShell: true,
		},
		{
			name:         "a directory removed during the wait ends the window",
			stub:         func(dir string) string { return fmt.Sprintf("(sleep 0.2; rmdir %q) &\nexit 3", dir) },
			dirPresent:   true,
			waitPolls:    60,
			pollInterval: 50 * time.Millisecond,
			wantStatus:   true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			root := t.TempDir()
			dir := filepath.Join(root, "pair dir")
			if tt.dirPresent {
				if err := os.Mkdir(dir, 0o755); err != nil {
					t.Fatal(err)
				}
			}
			stub := filepath.Join(root, "lym stub")
			if err := os.WriteFile(stub, []byte("#!/bin/sh\n"+tt.stub(dir)+"\n"), 0o755); err != nil {
				t.Fatal(err)
			}

			ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
			defer cancel()
			cmd := exec.CommandContext(ctx, "bash", "-c", attachLineWaiting(stub, dir, tt.waitPolls, tt.pollInterval))
			cmd.Stdin = strings.NewReader("echo kept-shell\n")
			output, err := cmd.CombinedOutput()
			if err != nil {
				t.Fatalf("bash -c over the window line: %v; output: %s", err, output)
			}

			if got := strings.Contains(string(output), "status 3"); got != tt.wantStatus {
				t.Errorf("output names the attach status = %v; want %v; output: %s", got, tt.wantStatus, output)
			}
			if got := strings.Contains(string(output), "kept-shell"); got != tt.wantKeptShell {
				t.Errorf("output holds the kept shell's echo = %v; want %v; output: %s", got, tt.wantKeptShell, output)
			}
		})
	}
}
