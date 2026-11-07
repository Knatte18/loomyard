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

// socketDirFromEnv returns the per-user directory that tmux puts its `-L` sockets in.
// It resolves the directory as tmux does: `$TMUX_TMPDIR`, else `/tmp`, then `tmux-<uid>`.
func socketDirFromEnv() string {
	return socketDirUnder(socketBaseFromEnv())
}

// socketBaseFromEnv returns the directory that holds tmux's per-user socket directory: `$TMUX_TMPDIR`, else `/tmp`.
func socketBaseFromEnv() string {
	if base := os.Getenv("TMUX_TMPDIR"); base != "" {
		return base
	}
	return "/tmp"
}

// socketDirUnder is the per-user socket directory `tmux-<uid>` under base.
func socketDirUnder(base string) string {
	return filepath.Join(base, "tmux-"+strconv.Itoa(os.Getuid()))
}

// resolvedSocketDir returns the per-user socket directory under the socket base, with its symlinks resolved.
// It resolves symlinks so that the path is measured at the length tmux sees.
// A base that cannot be resolved is used as it is.
func resolvedSocketDir() string {
	base := socketBaseFromEnv()
	if resolved, err := filepath.EvalSymlinks(base); err == nil {
		base = resolved
	}
	return socketDirUnder(base)
}

// unixSocketPathLimit returns the longest socket path in bytes that the OS accepts for goos.
// The limit is the `sun_path` size less the terminating NUL: 108 bytes on linux, 104 elsewhere.
func unixSocketPathLimit(goos string) int {
	if goos == "linux" {
		return 107
	}
	return 103
}

// checkSocketPathLength returns an error when the socket path for the `-L` key under dir is longer than limit bytes.
// The error names the path, its length and the limit, and it suggests a shorter `TMUX_TMPDIR`.
func checkSocketPathLength(dir, key string, limit int) error {
	path := filepath.Join(dir, key)
	if len(path) <= limit {
		return nil
	}
	return fmt.Errorf("tmux socket path %s is %d bytes, over the %d-byte limit of a unix socket path; set TMUX_TMPDIR to a shorter directory", path, len(path), limit)
}

// socketGoneWait bounds how long to wait for a server sent `kill-server` to stop answering on its socket.
const socketGoneWait = 2 * time.Second

// removeSocketFileOnceGone removes the socket file of the `-L` key under dir once its server is gone.
// It runs after a `kill-server` that a process scan did not confirm.
// `kill-server` is asynchronous, so it retries the removal until the file is gone or wait has passed.
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

// removeStaleSocket removes the socket file of the `-L` key under dir, once the server that owned it is gone.
// It removes the file only when the path is a socket that nothing answers a connection on,
// and a second `os.Lstat` reports the same file as the first.
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
