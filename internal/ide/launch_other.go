//go:build !windows

package ide

// launchCode returns an error on non-Windows platforms (POSIX).
// VS Code launch is a Windows-only feature; POSIX systems are not supported.
func launchCode(worktreeDir string) error {
	return ErrIDEUnsupported
}
