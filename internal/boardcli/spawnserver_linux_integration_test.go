//go:build linux

// spawnserver_linux_integration_test.go starts a stub in place of the board server and reads the server log it writes.

package boardcli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

func TestSpawnServerBinaryLogsBothStreams(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()
	dir := t.TempDir()
	stub := filepath.Join(dir, "stub")
	script := "#!/bin/sh\necho '{\"ok\":false,\"error\":\"cannot listen\"}'\necho 'on stderr' >&2\n"
	if err := os.WriteFile(stub, []byte(script), 0o755); err != nil {
		t.Fatalf("write the stub server: %v", err)
	}
	logPath := filepath.Join(dir, "state", "server.log")

	if err := spawnServerBinary(stub, dir, logPath); err != nil {
		t.Fatalf("spawnServerBinary: %v", err)
	}

	want := "{\"ok\":false,\"error\":\"cannot listen\"}\non stderr\n"
	deadline := time.Now().Add(time.Minute)
	for {
		got, err := os.ReadFile(logPath)
		if err != nil {
			t.Fatalf("read the server log: %v", err)
		}
		if strings.Count(string(got), "\n") >= 2 || time.Now().After(deadline) {
			if string(got) != want {
				t.Errorf("server log = %q; want %q", got, want)
			}
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
}
