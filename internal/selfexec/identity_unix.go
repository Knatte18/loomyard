//go:build !windows

// identity_unix.go stats a file's identity and execs the process through the Unix exec call.

package selfexec

import (
	"os"
	"syscall"
	"testing"
)

// statIdentity reads the device, inode, size and modification time of the file at path.
func statIdentity(path string) (identity, error) {
	info, err := os.Stat(path)
	if err != nil {
		return identity{}, err
	}
	raw, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return identity{}, errNoFileIdentity
	}
	return identity{
		device:       uint64(raw.Dev),
		inode:        uint64(raw.Ino),
		size:         info.Size(),
		modTimeNanos: info.ModTime().UnixNano(),
	}, nil
}

// execSelf replaces the process with path under argv and env.
// It returns errUnderTest without exec'ing inside a test binary.
func execSelf(path string, argv, env []string) error {
	if testing.Testing() {
		return errUnderTest
	}
	return syscall.Exec(path, argv, env)
}
