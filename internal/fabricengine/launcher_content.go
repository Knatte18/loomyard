// launcher_content.go builds the content and file mode of launcher scripts (ide, fabric-checkout, ide-menu, run).
// It is pure and GOOS-parameterized.
// Keeping it free of build tags lets tests cover both the Windows (.cmd) and non-Windows (.sh) branches.
// Only the OS I/O in launchers.go reads the real runtime.GOOS.

package fabricengine

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func launcherExt(goos string) string {
	if goos == "windows" {
		return ".cmd"
	}
	return ".sh"
}

// launcherScript builds the content and file mode of a launcher script that runs lym with lymArgs.
// It changes into climbRel, relative to the script's own directory.
// On Windows it is a cmd script with CRLF line endings and mode 0o644.
// Elsewhere it is a bash script with LF line endings and mode 0o755, so the file is executable.
func launcherScript(goos, climbRel, lymArgs string) (content []byte, mode os.FileMode) {
	climbFwd := filepath.ToSlash(climbRel)

	if goos == "windows" {
		climbBack := strings.ReplaceAll(climbFwd, "/", "\\")
		text := fmt.Sprintf("@cd /d \"%%~dp0%s\" && lym %s\r\n", climbBack, lymArgs)
		return []byte(text), 0o644
	}

	text := fmt.Sprintf("#!/usr/bin/env bash\ncd \"$(dirname \"$0\")/%s\" && lym %s\n", climbFwd, lymArgs)
	return []byte(text), 0o755
}
