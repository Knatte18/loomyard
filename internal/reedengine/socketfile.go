package reedengine

import (
	"fmt"
	"net"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"time"
)

// socketDirFromEnv is the per-user directory tmux puts its `-L` sockets in, resolved as tmux does:
// `$TMUX_TMPDIR`, else `/tmp`, then `tmux-<uid>`.
func socketDirFromEnv() string {
	base := os.Getenv("TMUX_TMPDIR")
	if base == "" {
		base = "/tmp"
	}
	return filepath.Join(base, "tmux-"+strconv.Itoa(os.Getuid()))
}

// socketGoneWait bounds how long removeSocketFileOnceGone waits for a server that was sent `kill-server` to stop answering.
const socketGoneWait = 2 * time.Second

// removeSocketFileOnceGone removes the socket file of the `-L` key key under dir after a `kill-server` that was not confirmed by a process scan.
// `kill-server` is asynchronous, so it retries removeStaleSocket until the file is gone or wait has passed.
// A file still there after wait stays, with no error.
func removeSocketFileOnceGone(dir, key string, wait time.Duration) error {
	path := filepath.Join(dir, key)
	for deadline := time.Now().Add(wait); ; time.Sleep(processExitPoll) {
		if err := removeStaleSocket(dir, key); err != nil {
			return err
		}
		if _, err := os.Lstat(path); os.IsNotExist(err) || time.Now().After(deadline) {
			return nil
		}
	}
}

// removeStaleSocket removes the socket file of the `-L` key key under dir, once the server that owned it is gone.
// It removes the file only when the path is a socket, nothing answers a connection on it, and a second `os.Lstat` reports the same file as the first.
// A live listener, a non-socket path or a changed file identity leaves the path alone, and a missing path is no error.
// It is a no-op on Windows, where psmux keeps no socket file.
// It removes the one path for key and never globs the directory.
func removeStaleSocket(dir, key string) error {
	if runtime.GOOS == "windows" {
		return nil
	}
	path := filepath.Join(dir, key)
	first, err := os.Lstat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("stat socket %s: %w", path, err)
	}
	if first.Mode()&os.ModeSocket == 0 {
		return nil
	}
	if conn, err := net.Dial("unix", path); err == nil {
		_ = conn.Close()
		return nil
	}
	second, err := os.Lstat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("restat socket %s: %w", path, err)
	}
	if !os.SameFile(first, second) {
		return nil
	}
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("remove socket %s: %w", path, err)
	}
	return nil
}
