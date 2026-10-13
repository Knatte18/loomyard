//go:build windows

// identity_windows.go gives the Windows build no file identity and no exec, so a Watcher there never replaces the process.

package selfexec

// statIdentity always fails on Windows, so every Check reads as unchanged.
func statIdentity(string) (identity, error) {
	return identity{}, errNoFileIdentity
}

// execSelf is never reached on Windows, where statIdentity gives the Watcher no start identity.
func execSelf(string, []string, []string) error {
	return errNoFileIdentity
}
