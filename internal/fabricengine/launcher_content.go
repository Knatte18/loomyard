// launcher_content.go builds the byte content and file mode for launcher scripts (ide,
// fabric-checkout, ide-menu, run) as pure, GOOS-parameterized functions.
// Keeping this logic build-tag-free lets it be unit-tested on the Windows warp for both the Windows
// (.cmd) and non-Windows (.sh) branches;
// only the OS I/O in launchers.go depends on the real runtime.GOOS.

package fabricengine

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
// climbs to a target subpath and invokes lyx. On Windows, uses cmd with CRLF
// and mode 0o644. On non-Windows, uses bash with LF and mode 0o755 (executable).
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
