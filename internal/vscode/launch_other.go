//go:build !windows

package vscode

// Launch returns an error on non-Windows platforms (POSIX).
// VS Code launch is a Windows-only feature; POSIX systems are not supported.
func Launch(worktreeDir string) error {
	return ErrUnsupported
}
