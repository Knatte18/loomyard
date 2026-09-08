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

// errRelativeLocalAppData is returned on the Windows branch when localAppData is set to a relative
// path.
// Unlike XDG_STATE_HOME, which has a specified fallback to degrade to, LOCALAPPDATA has none: there
// is nowhere correct to go, so the only honest answer is a loud refusal.
var errRelativeLocalAppData = errors.New("standalonestate: LOCALAPPDATA must be absolute")

// errMissingStateHome is returned on the non-Windows branch when no usable state home remains after
// a relative xdgStateHome has been discarded and home turns out to be empty.
var errMissingStateHome = errors.New("standalonestate: no usable state home: XDG_STATE_HOME is unset or relative and HOME is unset")

// errRelativeHome is returned on the non-Windows branch when the home directory is relative.
// A relative home is not a spec-defined "ignore me" the way a relative XDG_STATE_HOME is -- it is a
// broken environment with no remaining fallback, so it is a loud refusal.
var errRelativeHome = errors.New("standalonestate: home directory must be absolute")

// errRelativeStateDir is the final assertion's error, fired when the composed state directory came
// out relative even though every input branch above claims to have ruled that out.
// It exists because a relative stateDir is the one result this package must never hand back: every
// consumer joins subdirectories onto it and passes those to os.MkdirAll, so a relative value
// silently resolves against the process working directory -- which in standalone mode is the
// operator's own repository, making lyx write its state tree and its trace logs inside the very
// checkout standalonegeom.LogsDir exists to keep them out of.
var errRelativeStateDir = errors.New("standalonestate: derived state directory is not absolute")

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

// Normalize returns the spelling of target that Derive hashes: symlinks resolved and the result
// cleaned, the same way internal/lyxcwd/anchor.go's normalizePath does -- reimplemented rather than
// imported because this package must stay stdlib-only.
//
// It is exported because every value that has to AGREE with Derive's identity must be spelled the
// way Derive spelled it, and re-implementing the rule at each such site is exactly how the two drift
// apart: standalonegeom.ReedGeometry builds the readable half of its tmux session name from the
// target's basename while the socket key and state directory both come from Derive's hash, so a
// symlinked and a real spelling of one repository produced two DIFFERENT session names on one shared
// socket, sharing one reed.json (R4 review finding R4-24). Exposing the rule here keeps it declared
// once, in the package that owns the identity.
//
// A relative target is cleaned but deliberately NOT resolved: filepath.EvalSymlinks would resolve it
// against the process working directory, and this package never consults one (see the package doc
// and CONSTRAINTS.md's Standalonestate Leaf Invariant). Derive rejects a relative target outright, so
// that branch only ever serves a caller normalizing before it has validated.
//
// Normalize READS the filesystem -- resolving a symlink is nothing else -- but creates nothing on it,
// and a target that does not exist on disk yet falls back to Clean alone rather than failing, since
// an unborn directory still needs a stable identity.
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

// derive is the injectable seam behind Derive: every environment-shaped input is a plain
// parameter, so both platform rows can be driven from a test without runtime.GOOS being a
// compile-time constant getting in the way.
// An empty string at this boundary means "unset" -- derive cannot distinguish unset from
// set-to-empty, which is correct because neither an empty LOCALAPPDATA nor an empty
// XDG_STATE_HOME is a usable directory.
// The returned stateDir is always absolute or the call fails: see stateDirFor for the per-base
// rules and errRelativeStateDir for why a relative one would be the worst possible answer.
func derive(goos, localAppData, xdgStateHome, home, target string) (stateDir string, hash8 string, err error) {
	if !filepath.IsAbs(target) {
		return "", "", fmt.Errorf("%w: %q", errRelativeTarget, target)
	}

	resolved := Normalize(target)

	// Fold case on Windows only, matching lyxcwd's samePath rule exactly. samePath folds case
	// at comparison time; hashing has no comparison step, so the fold must happen to the string
	// that is hashed instead.
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

	// The absoluteness of every input is already checked in stateDirFor, so this assertion can only
	// fire on a future branch that forgets to -- which is exactly why it guards the single return
	// rather than each branch. See errRelativeStateDir for what a relative value would cost.
	if !filepath.IsAbs(stateDir) {
		return "", "", fmt.Errorf("%w: %q", errRelativeStateDir, stateDir)
	}
	return stateDir, hash8, nil
}

// stateDirFor holds derive's per-platform choice of state-directory base, split out so derive keeps
// one place to assert the composed result is absolute rather than one per branch.
//
// Every base here comes from the environment, and an environment-supplied base gets exactly the same
// absoluteness scrutiny target already gets: a relative base composes into a relative stateDir, and
// derive's consumers resolve that against the process working directory -- the operator's own
// repository in standalone mode.
// The three bases answer a relative value differently only because their specifications do.
func stateDirFor(goos, localAppData, xdgStateHome, home, hash8 string) (string, error) {
	if goos == "windows" {
		if localAppData == "" {
			return "", errMissingLocalAppData
		}
		if !filepath.IsAbs(localAppData) {
			return "", fmt.Errorf("%w: %q", errRelativeLocalAppData, localAppData)
		}
		return filepath.Join(localAppData, "lyx", hash8), nil
	}

	// A relative XDG_STATE_HOME is IGNORED rather than refused, because the XDG Base Directory
	// specification says exactly that: "If an implementation encounters a relative path in any of
	// these variables it should consider the path invalid and ignore it." Ignoring it lands on the
	// specified $HOME/.local/state fallback below, which is both what the spec asks for and the
	// safer of the two answers -- the alternative, honoring it, is what wrote lyx state into the
	// operator's repository.
	if filepath.IsAbs(xdgStateHome) {
		return filepath.Join(xdgStateHome, "lyx", hash8), nil
	}
	if home == "" {
		return "", errMissingStateHome
	}
	if !filepath.IsAbs(home) {
		return "", fmt.Errorf("%w: %q", errRelativeHome, home)
	}
	return filepath.Join(home, ".local", "state", "lyx", hash8), nil
}
