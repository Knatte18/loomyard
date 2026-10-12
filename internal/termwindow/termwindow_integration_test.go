//go:build integration && !windows

// termwindow_integration_test.go runs the window's shell line under bash, with a stub executable standing in for lyx, to pin when the window ends and when it keeps its shell.

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
)

func TestAttachLine_EndsOrKeepsTheShellByAttachStatus(t *testing.T) {
	t.Parallel()

	if _, err := exec.LookPath("bash"); err != nil {
		t.Skip("bash is not on PATH")
	}

	tests := []struct {
		name string
		// stub is the script body after the shebang, given the pair directory.
		stub          func(dir string) string
		dirPresent    bool
		waitSeconds   int
		wantStatus    bool
		wantKeptShell bool
	}{
		{
			name:        "a stub exiting 0 ends without the shell",
			stub:        func(string) string { return "exit 0" },
			dirPresent:  true,
			waitSeconds: 1,
		},
		{
			name:        "a stub exiting 3 with the directory absent prints the status and ends",
			stub:        func(string) string { return "exit 3" },
			waitSeconds: 1,
			wantStatus:  true,
		},
		{
			name:          "a stub exiting 3 with the directory present prints the status and keeps the shell",
			stub:          func(string) string { return "exit 3" },
			dirPresent:    true,
			waitSeconds:   1,
			wantStatus:    true,
			wantKeptShell: true,
		},
		{
			name:        "a directory removed during the wait ends the window",
			stub:        func(dir string) string { return fmt.Sprintf("(sleep 1; rmdir %q) &\nexit 3", dir) },
			dirPresent:  true,
			waitSeconds: 3,
			wantStatus:  true,
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
			stub := filepath.Join(root, "lyx stub")
			if err := os.WriteFile(stub, []byte("#!/bin/sh\n"+tt.stub(dir)+"\n"), 0o755); err != nil {
				t.Fatal(err)
			}

			ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
			defer cancel()
			cmd := exec.CommandContext(ctx, "bash", "-c", attachLineWaiting(stub, dir, tt.waitSeconds))
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
