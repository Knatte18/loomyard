// posix.go implements PosixPath, the Windows-to-git-bash path converter the
// claude engine embeds into hook commands. Hook commands run under
// git-bash on Windows, so a bare backslash path would be silently
// misinterpreted (backslash is git-bash's escape character) — every path
// handed to a hook command must go through this conversion first.

package shuttleengine

import (
	"fmt"
	"strings"
)

// PosixPath converts an absolute Windows path (e.g. `C:\a b\c`) to its
// git-bash POSIX form (`/c/a b/c`): the drive letter is lower-cased and
// moved behind a leading slash, and every backslash becomes a forward
// slash. Forward-slash input is tolerated (a path already using `/`
// separators passes through the same drive-letter rewrite). PosixPath
// returns an error naming p when p is not drive-rooted — a UNC path
// (`\\host\share\...`) or a relative path has no POSIX drive-letter form
// and is rejected rather than guessed at.
func PosixPath(p string) (string, error) {
	// Normalize separators up front so the drive-root check below only has
	// one shape (forward slashes) to reason about.
	normalized := strings.ReplaceAll(p, `\`, "/")

	// A drive-rooted path is exactly "<letter>:/...". Anything shorter, or
	// missing the colon at index 1, is not drive-rooted (this also rejects
	// UNC paths, which start with "//", and relative paths).
	if len(normalized) < 3 || !isDriveLetter(normalized[0]) || normalized[1] != ':' || normalized[2] != '/' {
		return "", fmt.Errorf("shuttle: PosixPath: not a drive-rooted absolute path: %q", p)
	}

	drive := strings.ToLower(normalized[:1])
	rest := normalized[2:] // keeps the leading "/" before the path body
	return "/" + drive + rest, nil
}

// isDriveLetter reports whether b is an ASCII letter, the only valid
// character in a Windows drive letter.
func isDriveLetter(b byte) bool {
	return (b >= 'a' && b <= 'z') || (b >= 'A' && b <= 'Z')
}
