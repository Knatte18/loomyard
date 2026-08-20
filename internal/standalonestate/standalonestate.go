// standalonestate.go derives, from a target directory, the hash8 identifier and per-OS state
// directory a standalone CLI session uses for its socket, session, and state files.

package standalonestate

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

// errRelativeTarget is returned when target is not an absolute path.
var errRelativeTarget = errors.New("standalonestate: target must be absolute")

// errMissingLocalAppData is returned on the Windows branch when localAppData is empty.
var errMissingLocalAppData = errors.New("standalonestate: LOCALAPPDATA is unset")

// errMissingStateHome is returned on the non-Windows branch when both xdgStateHome and home are
// empty.
var errMissingStateHome = errors.New("standalonestate: XDG_STATE_HOME and HOME are both unset")

// Derive computes the per-OS state directory and hash8 identifier for target, a directory a
// standalone CLI session is anchored at.
// target must already be absolute -- Derive never consults the process working directory, since
// that resolution belongs at the CLI argument-parsing boundary.
func Derive(target string) (stateDir string, hash8 string, err error) {
	goos := runtime.GOOS
	localAppData := os.Getenv("LOCALAPPDATA")
	xdgStateHome := os.Getenv("XDG_STATE_HOME")

	// os.UserHomeDir is only consulted on the non-Windows branch of derive, so a failure here
	// can never surface as an error on the Windows branch, which never reads home.
	var home string
	if goos != "windows" {
		home, _ = os.UserHomeDir()
	}

	return derive(goos, localAppData, xdgStateHome, home, target)
}

// derive is the injectable seam behind Derive: every environment-shaped input is a plain
// parameter, so both platform rows can be driven from a test without runtime.GOOS being a
// compile-time constant getting in the way.
// An empty string at this boundary means "unset" -- derive cannot distinguish unset from
// set-to-empty, which is correct because neither an empty LOCALAPPDATA nor an empty
// XDG_STATE_HOME is a usable directory.
func derive(goos, localAppData, xdgStateHome, home, target string) (stateDir string, hash8 string, err error) {
	if !filepath.IsAbs(target) {
		return "", "", fmt.Errorf("%w: %q", errRelativeTarget, target)
	}

	// Normalize the same way internal/lyxcwd/anchor.go's normalizePath does, reimplemented
	// rather than imported because this package must stay stdlib-only: resolve symlinks,
	// falling back to Clean alone when the target does not exist on disk yet.
	resolved, evalErr := filepath.EvalSymlinks(target)
	if evalErr != nil {
		resolved = filepath.Clean(target)
	} else {
		resolved = filepath.Clean(resolved)
	}

	// Fold case on Windows only, matching lyxcwd's samePath rule exactly. samePath folds case
	// at comparison time; hashing has no comparison step, so the fold must happen to the string
	// that is hashed instead.
	hashInput := resolved
	if goos == "windows" {
		hashInput = strings.ToLower(hashInput)
	}

	sum := sha256.Sum256([]byte(hashInput))
	hash8 = hex.EncodeToString(sum[:])[:8]

	if goos == "windows" {
		if localAppData == "" {
			return "", "", errMissingLocalAppData
		}
		return filepath.Join(localAppData, "lyx", hash8), hash8, nil
	}

	if xdgStateHome != "" {
		return filepath.Join(xdgStateHome, "lyx", hash8), hash8, nil
	}
	if home == "" {
		return "", "", errMissingStateHome
	}
	return filepath.Join(home, ".local", "state", "lyx", hash8), hash8, nil
}
