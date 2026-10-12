// root_windows.go — the root's name and real-directory check on Windows.

//go:build windows

package lyxtemp

// rootName is the root's directory name under the system temp directory, which on Windows is already per user.
func rootName(uid int) string {
	return "lyx"
}

// checkRoot requires path to be a real directory; Windows has no mode or owner to check, so uid is ignored.
func checkRoot(path string, uid int) error {
	_, err := checkRealDirectory(path)
	return err
}
