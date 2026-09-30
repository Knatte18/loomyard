//go:build integration

// transport_integration_test.go pins the real stderr git prints for an unreachable remote and for a rejected push.

package gitexec_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/Knatte18/loomyard/internal/gitexec"
)

func TestIsTransportFailure_RealGit(t *testing.T) {
	dir := t.TempDir()
	for _, args := range [][]string{
		{"init"},
		{"config", "user.email", "t@example.com"},
		{"config", "user.name", "t"},
		{"commit", "--allow-empty", "-m", "c"},
		{"remote", "add", "origin", "http://127.0.0.1:1/unreachable.git"},
	} {
		if _, err := gitexec.Run(args, dir); err != nil {
			t.Fatalf("git %v: %v", args, err)
		}
	}

	_, err := gitexec.Run([]string{"-c", "push.autoSetupRemote=true", "push", "origin", "HEAD"}, dir)
	if err == nil {
		t.Fatal("push to unreachable remote succeeded")
	}
	if !gitexec.IsTransportFailure(err) {
		t.Errorf("unreachable push not classified as transport failure: %v", err)
	}

	bare := filepath.Join(t.TempDir(), "bare.git")
	if _, err := gitexec.Run([]string{"init", "--bare", bare}, dir); err != nil {
		t.Fatalf("init bare: %v", err)
	}
	hook := filepath.Join(bare, "hooks", "pre-receive")
	if err := os.WriteFile(hook, []byte("#!/bin/sh\nexit 1\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := gitexec.Run([]string{"remote", "set-url", "origin", bare}, dir); err != nil {
		t.Fatal(err)
	}
	_, err = gitexec.Run([]string{"push", "origin", "HEAD:refs/heads/x"}, dir)
	if err == nil {
		t.Fatal("hook-rejected push succeeded")
	}
	if gitexec.IsTransportFailure(err) {
		t.Errorf("rejected push classified as transport failure: %v", err)
	}
}
