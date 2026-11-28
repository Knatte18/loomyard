// transport_integration_test.go pins the real stderr git prints for failed remote operations.
// Its cases are an unreachable remote, a removed local origin and a directory holding no repository.
// They also cover an unconfigured remote name and a rejected push.

package gitexec_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/Knatte18/loomyard/internal/gitexec"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

//testtiming:keep pins the real stderr git prints for an unreachable remote, a removed local origin, an unconfigured remote name and a hook-rejected push, which TestIsTransportFailure's hand-built stderr rows do not
func TestIsTransportFailure_RealGit(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()

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

	if err := os.RemoveAll(bare); err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{
		{"push", "origin", "HEAD:refs/heads/x"},
		{"fetch", "origin"},
		{"ls-remote", "origin"},
		{"clone", bare, filepath.Join(t.TempDir(), "clone")},
	} {
		_, err := gitexec.Run(args, dir)
		if err == nil {
			t.Fatalf("git %v against a removed origin succeeded", args)
		}
		if !gitexec.IsTransportFailure(err) {
			t.Errorf("git %v against a removed origin not classified as transport failure: %v", args, err)
		}
	}

	if err := os.Mkdir(bare, 0o755); err != nil {
		t.Fatal(err)
	}
	_, err = gitexec.Run([]string{"push", "origin", "HEAD:refs/heads/x"}, dir)
	if err == nil {
		t.Fatal("push to a directory holding no repository succeeded")
	}
	if gitexec.IsTransportFailure(err) {
		t.Errorf("push to a directory holding no repository classified as transport failure: %v", err)
	}

	_, err = gitexec.Run([]string{"ls-remote", "unconfigured"}, dir)
	if err == nil {
		t.Fatal("ls-remote of an unconfigured remote succeeded")
	}
	if gitexec.IsTransportFailure(err) {
		t.Errorf("ls-remote of an unconfigured remote classified as transport failure: %v", err)
	}
}
