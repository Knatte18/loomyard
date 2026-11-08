// binary.go resolves the path of the running lym binary for a command that another process runs later.

package selfexec

import (
	"errors"
	"os"
	"testing"
)

// ErrTestBinary refuses a resolved path that names the running test binary.
var ErrTestBinary = errors.New("selfexec: the binary is the running go test binary, which no pinned or spawned lym command may run")

// LymBinary returns the path of the running lym binary.
// It serves a command that a pane, a tmux job, a binding or a detached process runs later.
// Under go test it returns ErrTestBinary, since the running binary is then a test binary.
// A test binary handed a lym verb runs its whole suite.
// Each tmux session or process that suite starts may run the same command again, without end.
func LymBinary() (string, error) {
	return ResolveLymBinary(os.Executable)
}

// ResolveLymBinary returns the path executable resolves.
// It refuses the path with ErrTestBinary when it names the running test binary.
// Under go test, a path equal to the test binary's own is refused.
// So is every path when the test binary's own path does not resolve.
// A resolve error is returned as it is.
// It lets a caller with an injectable path lookup keep the refusal, while a test's stand-in path passes.
func ResolveLymBinary(executable func() (string, error)) (string, error) {
	path, err := executable()
	if err != nil || !testing.Testing() {
		return path, err
	}
	self, err := os.Executable()
	if err != nil || path == self {
		return "", ErrTestBinary
	}
	return path, nil
}
