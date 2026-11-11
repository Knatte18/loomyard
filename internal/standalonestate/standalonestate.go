// standalonestate.go derives the hash8 identifier and per-OS state directory of a standalone CLI session.
// The two values name the session's socket, tmux session, and state files.

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

// errRelativeLocalAppData is returned on the Windows branch when localAppData is set to a relative path.
// LOCALAPPDATA has no fallback, so a relative value is refused rather than ignored.
var errRelativeLocalAppData = errors.New("standalonestate: LOCALAPPDATA must be absolute")

// errMissingStateHome is returned on the non-Windows branch when no usable state home remains after
// a relative xdgStateHome has been discarded and home turns out to be empty.
var errMissingStateHome = errors.New("standalonestate: no usable state home: XDG_STATE_HOME is unset or relative and HOME is unset")

// errRelativeHome is returned on the non-Windows branch when HOME is relative.
// A relative home leaves no usable fallback, so it is refused rather than ignored.
var errRelativeHome = errors.New("standalonestate: HOME must be absolute")

// errRelativeStateDir is the final assertion's error, fired when the composed state directory is relative.
// Every input branch has already ruled that out.
// A relative stateDir must never be returned: every consumer joins subdirectories onto it,
// and passes them to os.MkdirAll, which resolves a relative path against the process working directory.
// In standalone mode that directory is the operator's own repository, where lym state must never land.
var errRelativeStateDir = errors.New("standalonestate: derived state directory is not absolute")

// Derive computes the per-OS state directory and hash8 identifier for target, the directory a
// standalone CLI session is anchored at.
// target must already be absolute.
// Derive never consults the process working directory.
// Resolving a relative path belongs at the CLI argument-parsing boundary.
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

// Normalize returns the spelling of target that the identity is hashed from: symlinks resolved, then cleaned.
// The package stays stdlib-only, so it carries its own copy of the cleaning rule.
// It is exported so that every value which must agree with the identity is spelled the same way.
//
// A relative target is cleaned but not resolved.
// Resolving it would consult the process working directory, which this package never does.
// Derivation refuses a relative target, so only a caller that normalizes before validating reaches this branch.
//
// Normalize reads the filesystem to resolve symlinks but creates nothing on it.
// A target that does not exist yet falls back to cleaning alone; an unborn directory still needs a stable identity.
func Normalize(target string) string {
	if !filepath.IsAbs(target) {
		return filepath.Clean(target)
	}
	resolved, err := filepath.EvalSymlinks(target)
	if err != nil {
		return filepath.Clean(target)
	}
	return filepath.Clean(resolved)
}

// derive takes every environment-shaped input as a plain parameter,
// so a test can drive both platform branches without depending on runtime.GOOS.
// An empty string means unset: derive cannot tell unset from set-to-empty, and neither yields a usable directory.
// The returned stateDir is always absolute, or the call fails.
func derive(goos, localAppData, xdgStateHome, home, target string) (stateDir string, hash8 string, err error) {
	if !filepath.IsAbs(target) {
		return "", "", fmt.Errorf("%w: %q", errRelativeTarget, target)
	}

	resolved := Normalize(target)

	// Fold case on Windows only, as path comparison does there.
	// Hashing has no comparison step, so the fold applies to the string that is hashed.
	hashInput := resolved
	if goos == "windows" {
		hashInput = strings.ToLower(hashInput)
	}

	sum := sha256.Sum256([]byte(hashInput))
	hash8 = hex.EncodeToString(sum[:])[:8]

	stateDir, err = stateDirFor(goos, localAppData, xdgStateHome, home, hash8)
	if err != nil {
		return "", "", err
	}

	// Every input's absoluteness is checked in stateDirFor, so this assertion fires only if a branch skips that.
	// It guards the single return rather than each branch.
	if !filepath.IsAbs(stateDir) {
		return "", "", fmt.Errorf("%w: %q", errRelativeStateDir, stateDir)
	}
	return stateDir, hash8, nil
}

// stateDirFor composes the per-target state directory from the per-platform root and hash8.
// Every base comes from the environment and gets the same absoluteness scrutiny as target.
// A relative base would compose into a relative stateDir.
// Consumers resolve that against the process working directory.
// The bases answer a relative value differently because their specifications do.
func stateDirFor(goos, localAppData, xdgStateHome, home, hash8 string) (string, error) {
	root, err := stateRootFor(goos, localAppData, xdgStateHome, home)
	if err != nil {
		return "", err
	}
	return filepath.Join(root, hash8), nil
}

// UserStateDir returns the per-user `lym` state directory that every standalone state directory sits under.
// It reads the environment the same way the derivation does, creates nothing, and answers an absolute path or an error.
func UserStateDir() (string, error) {
	goos := runtime.GOOS
	var home string
	if goos != "windows" {
		home, _ = os.UserHomeDir()
	}
	return stateRootFor(goos, os.Getenv("LOCALAPPDATA"), os.Getenv("XDG_STATE_HOME"), home)
}

// stateRootFor returns the per-platform `lym` directory under the state base.
// It is the part of a state directory that does not depend on the target.
func stateRootFor(goos, localAppData, xdgStateHome, home string) (string, error) {
	if goos == "windows" {
		if localAppData == "" {
			return "", errMissingLocalAppData
		}
		if !filepath.IsAbs(localAppData) {
			return "", fmt.Errorf("%w: %q", errRelativeLocalAppData, localAppData)
		}
		return filepath.Join(localAppData, "lym"), nil
	}

	// A relative XDG_STATE_HOME is ignored rather than refused, as the XDG Base Directory specification requires.
	// Ignoring it falls back to $HOME/.local/state, the safer answer:
	// honoring a relative path would write lym state into the operator's repository.
	if filepath.IsAbs(xdgStateHome) {
		return filepath.Join(xdgStateHome, "lym"), nil
	}
	if home == "" {
		return "", errMissingStateHome
	}
	if !filepath.IsAbs(home) {
		return "", fmt.Errorf("%w: %q", errRelativeHome, home)
	}
	return filepath.Join(home, ".local", "state", "lym"), nil
}

// UserConfigDir returns the per-user `lym` config directory.
// It is `%LOCALAPPDATA%\lym` on Windows, elsewhere `$XDG_CONFIG_HOME/lym` or `$HOME/.config/lym`.
// A non-empty XDG_CONFIG_HOME is taken as given, relative or not, so files kept there keep their paths.
// It returns ("", false) when no variable it reads is set, and creates nothing.
func UserConfigDir() (string, bool) {
	return userConfigDirFor(runtime.GOOS, os.Getenv("LOCALAPPDATA"), os.Getenv("XDG_CONFIG_HOME"), os.Getenv("HOME"))
}

// userConfigDirFor resolves UserConfigDir's directory from plain parameters, so a test drives both platform rows.
func userConfigDirFor(goos, localAppData, xdgConfigHome, home string) (string, bool) {
	if goos == "windows" {
		if localAppData == "" {
			return "", false
		}
		return filepath.Join(localAppData, "lym"), true
	}
	if xdgConfigHome != "" {
		return filepath.Join(xdgConfigHome, "lym"), true
	}
	if home != "" {
		return filepath.Join(home, ".config", "lym"), true
	}
	return "", false
}

// AbsoluteUserConfigDir returns the per-user `lym` config directory as an absolute path,
// so a file kept there never resolves against a process's working directory.
// A relative XDG_CONFIG_HOME is ignored in favour of `$HOME/.config/lym`,
// as the XDG Base Directory specification requires.
// A relative HOME or LOCALAPPDATA is an error naming the variable, since no fallback is left.
// It returns ("", false, nil) when no variable it reads is set, and creates nothing.
func AbsoluteUserConfigDir() (string, bool, error) {
	return absoluteUserConfigDirFor(runtime.GOOS, os.Getenv("LOCALAPPDATA"), os.Getenv("XDG_CONFIG_HOME"), os.Getenv("HOME"))
}

// absoluteUserConfigDirFor resolves AbsoluteUserConfigDir's directory from plain parameters,
// so a test drives both platform rows.
func absoluteUserConfigDirFor(goos, localAppData, xdgConfigHome, home string) (string, bool, error) {
	if goos == "windows" {
		if localAppData == "" {
			return "", false, nil
		}
		if !filepath.IsAbs(localAppData) {
			return "", false, fmt.Errorf("%w: %q", errRelativeLocalAppData, localAppData)
		}
		return filepath.Join(localAppData, "lym"), true, nil
	}
	if filepath.IsAbs(xdgConfigHome) {
		return filepath.Join(xdgConfigHome, "lym"), true, nil
	}
	if home == "" {
		return "", false, nil
	}
	if !filepath.IsAbs(home) {
		return "", false, fmt.Errorf("%w: %q", errRelativeHome, home)
	}
	return filepath.Join(home, ".config", "lym"), true, nil
}
