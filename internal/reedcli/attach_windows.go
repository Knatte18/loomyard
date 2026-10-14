//go:build windows

// attach_windows.go hands the operator's terminal to tmux as a child process, since Windows has no exec that replaces a process.

package reedcli

// handOver runs tmux with args as a child that inherits the operator's stdio and returns its exit code.
func handOver(tmux string, args []string) (exitCode int, err error) {
	return runAttachChild(tmux, args)
}
