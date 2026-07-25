// resolve.go implements the dev/prod lyx binary resolution used across the
// sandbox launcher: resolveLyx picks the derived .dev-bin binary when it is
// present on disk and falls back to the existing PATH lookup otherwise, and
// prependPath builds the PATH a child process should see when a dev binary
// is in play. This is the only file in package main permitted to perform a
// bare-PATH "lyx" lookup; every other call site resolves through resolveLyx
// instead so the dev/prod distinction stays in one place.

package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/Knatte18/loomyard/tools/internal/devbin"
)

// Source labels returned by resolveLyx, identifying which binary a caller
// resolved to. The distinction is surfaced (not just the resolved path) so
// callers can stamp it into fingerprints and log output.
const (
	// sourceDev marks a binary resolved from the derived .dev-bin directory.
	sourceDev = "dev"
	// sourceProd marks a binary resolved from the operator's PATH.
	sourceProd = "prod"
)

// devBinPath is a testability seam over devbin.BinPath so tests can inject a
// fake derived dev-binary path without touching the real repository layout.
var devBinPath = devbin.BinPath

// resolveLyx picks the lyx binary a sandbox launcher should run: the derived
// .dev-bin binary when it exists on disk, otherwise the existing PATH-based
// "lyx" lookup (lookPath, declared in suite.go). Preferring the dev binary
// lets deploy-dev builds be exercised without ever touching the operator's
// PATH; falling back to PATH keeps prod-only flows working exactly as before.
// A devBinPath error (e.g. the repo root cannot be derived) is treated as
// "no dev binary available" and is not fatal -- resolution simply falls
// through to the PATH branch. A lookPath error in the fallback branch is
// fatal and is returned to the caller unchanged in wording, extended to
// mention deploy-dev as an alternative to deploying the prod binary.
func resolveLyx() (path string, source string, err error) {
	// devBinPath itself only derives a path; it does not guarantee the file
	// exists. Stat it explicitly so a repo checked out without a dev build
	// falls through to prod rather than returning a dangling path.
	if devPath, derivErr := devBinPath(); derivErr == nil {
		if _, statErr := os.Stat(devPath); statErr == nil {
			return devPath, sourceDev, nil
		}
	}

	prodPath, lookErr := lookPath("lyx")
	if lookErr != nil {
		return "", "", fmt.Errorf(
			"lyx not found on PATH -- deploy the binary (or run deploy-dev) before running the suite: %w",
			lookErr,
		)
	}
	return prodPath, sourceProd, nil
}

// prependPath returns a copy of environ with dir prepended to the PATH
// entry's value, so a child process launched with the result sees dir
// searched first. The PATH key is matched case-insensitively (Windows uses
// "Path") and edited in place; a fresh "PATH=" entry is appended only when
// none was already present. When dir is empty, environ is returned
// unchanged -- callers use this to distinguish "no dev binary in play" from
// "an empty dev directory", avoiding an accidental no-op prepend.
func prependPath(dir string, environ []string) []string {
	if dir == "" {
		return environ
	}

	result := make([]string, len(environ))
	copy(result, environ)

	for i, entry := range result {
		key, value, found := splitEnvEntry(entry)
		if !found || !strings.EqualFold(key, "PATH") {
			continue
		}
		result[i] = key + "=" + dir + string(os.PathListSeparator) + value
		return result
	}

	// No existing PATH entry (case-insensitive) was found; add one so the
	// child process still sees dir on its search path.
	return append(result, "PATH="+dir)
}

// splitEnvEntry splits a single os.Environ()-style "KEY=VALUE" entry into its
// key and value. found is false when entry contains no "=" separator, which
// should not occur for entries returned by os.Environ but is handled
// defensively since prependPath also accepts caller-constructed slices.
func splitEnvEntry(entry string) (key, value string, found bool) {
	idx := strings.IndexByte(entry, '=')
	if idx < 0 {
		return "", "", false
	}
	return entry[:idx], entry[idx+1:], true
}
