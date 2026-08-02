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

// devBinPath is a testability seam over devbin.BinPath.
var devBinPath = devbin.BinPath

// resolveLyx picks the lyx binary to run: the derived .dev-bin binary when
// it exists, otherwise the PATH-based "lyx" lookup. Errors from devBinPath
// are treated as "no dev binary available"; errors from PATH lookup are fatal.
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
// entry's value. When dir is empty, environ is returned unchanged.
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

// splitEnvEntry splits a "KEY=VALUE" entry into key and value. found reports
// whether an "=" separator was present.
func splitEnvEntry(entry string) (key, value string, found bool) {
	idx := strings.IndexByte(entry, '=')
	if idx < 0 {
		return "", "", false
	}
	return entry[:idx], entry[idx+1:], true
}
