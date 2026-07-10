// launcher_content.go builds the byte content and file mode for launcher scripts
// (ide, warp-checkout, ide-menu) as pure, GOOS-parameterized functions. Keeping
// this logic build-tag-free lets it be unit-tested on the Windows host for both
// the Windows (.cmd) and non-Windows (.sh) branches; only the OS I/O in
// launchers.go depends on the real runtime.GOOS.

package warpengine

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// launcherExt returns the launcher script's file extension for the given GOOS:
// ".cmd" on Windows, ".sh" everywhere else.
func launcherExt(goos string) string {
	if goos == "windows" {
		return ".cmd"
	}
	return ".sh"
}

// launcherScript builds the content and file mode for a launcher script that
// climbs from its own directory to a target worktree subpath via climbRel, then
// invokes "lyx <lyxArgs>".
//
// climbRel is normalized to forward slashes first so callers can pass either
// filepath.Rel output (OS-native separators) or an already-slashed path.
//
// On Windows, the climb is rendered with backslashes and the script uses the
// "@cd /d "%~dp0<climb>" && lyx <lyxArgs>" cmd idiom with CRLF line endings and
// mode 0o644 (matching the pre-existing ide.cmd/warp-checkout.cmd/ide-menu.cmd
// bodies).
//
// On non-Windows, the climb keeps forward slashes and the script is a bash
// shebang script — "#!/usr/bin/env bash\ncd "$(dirname "$0")/<climb>" && lyx
// <lyxArgs>\n" — with LF line endings and mode 0o755 so it is executable.
func launcherScript(goos, climbRel, lyxArgs string) (content []byte, mode os.FileMode) {
	climbFwd := filepath.ToSlash(climbRel)

	if goos == "windows" {
		climbBack := strings.ReplaceAll(climbFwd, "/", "\\")
		text := fmt.Sprintf("@cd /d \"%%~dp0%s\" && lyx %s\r\n", climbBack, lyxArgs)
		return []byte(text), 0o644
	}

	text := fmt.Sprintf("#!/usr/bin/env bash\ncd \"$(dirname \"$0\")/%s\" && lyx %s\n", climbFwd, lyxArgs)
	return []byte(text), 0o755
}
