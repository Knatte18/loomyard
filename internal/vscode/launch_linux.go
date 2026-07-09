package vscode

// Launch returns an error on Linux.
// VS Code launch is a Windows-only feature; Linux is not supported.
func Launch(worktreeDir string) error {
	return ErrUnsupported
}
