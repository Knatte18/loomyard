// posix.go implements PosixPath, the Windows-to-git-bash path converter that claudeengine embeds into hook commands.
// Hook commands run under git-bash on Windows, where a bare backslash path is silently misinterpreted.
// Backslash is git-bash's escape character.
// Every path handed to a hook command must go through this conversion first.

package shuttleengine

import (
	"fmt"
	"strings"
)

// PosixPath converts an absolute Windows path, such as `C:\a b\c`, to git-bash POSIX form, such as `/c/a b/c`.
// The drive letter is lowercased and moved behind a leading slash;
// backslashes become forward slashes.
// It returns an error for non-drive-rooted paths (UNC or relative).
func PosixPath(p string) (string, error) {
	// Normalize separators so the drive-root check only reasons about forward slashes.
	normalized := strings.ReplaceAll(p, `\`, "/")

	// A drive-rooted path is exactly "<letter>:/...".
	// This also rejects UNC paths ("//") and relative paths.
	if len(normalized) < 3 || !isDriveLetter(normalized[0]) || normalized[1] != ':' || normalized[2] != '/' {
		return "", fmt.Errorf("shuttle: PosixPath: not a drive-rooted absolute path: %q", p)
	}

	drive := strings.ToLower(normalized[:1])
	rest := normalized[2:] // keeps the leading "/" before the path body
	return "/" + drive + rest, nil
}

// isDriveLetter reports whether b is an ASCII letter.
func isDriveLetter(b byte) bool {
	return (b >= 'a' && b <= 'z') || (b >= 'A' && b <= 'Z')
}
