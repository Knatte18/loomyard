// resolve.go implements the dev/prod lym binary resolution the sandbox launcher uses.
// resolveLym prefers the derived .dev-bin binary when it exists on disk and otherwise falls back to a PATH lookup.
// prependPath builds the PATH a child process sees when a dev binary is in play.
// This is the only file in package main that performs a bare PATH lookup of "lym":
// every other call site resolves through resolveLym, so the dev/prod distinction stays in one place.

package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/Knatte18/loomyard/tools/internal/devbin"
)

// Source labels reported by resolveLym, naming which binary a caller got.
// Callers stamp the label into fingerprints and log output.
const (
	// sourceDev marks a binary resolved from the derived .dev-bin directory.
	sourceDev = "dev"
	// sourceProd marks a binary resolved from the operator's PATH.
	sourceProd = "prod"
)

// devBinPath derives the dev binary path; tests replace it to control the dev-binary lookup.
var devBinPath = devbin.BinPath

// resolveLym picks the lym binary to run: the derived .dev-bin binary if it exists, otherwise the PATH lookup of "lym".
// An error from devBinPath means no dev binary is available; an error from the PATH lookup is fatal.
func resolveLym() (path string, source string, err error) {
	// devBinPath only derives a path and does not guarantee the file exists.
	// Stat it, so a repo checked out without a dev build falls through to prod rather than a dangling path.
	if devPath, derivErr := devBinPath(); derivErr == nil {
		if _, statErr := os.Stat(devPath); statErr == nil {
			return devPath, sourceDev, nil
		}
	}

	prodPath, lookErr := lookPath("lym")
	if lookErr != nil {
		return "", "", fmt.Errorf(
			"lym not found on PATH -- deploy the binary (or run deploy-dev) before running the suite: %w",
			lookErr,
		)
	}
	return prodPath, sourceProd, nil
}

// prependPath returns a copy of environ with dir prepended to the PATH entry's value.
// When dir is empty, environ is returned unchanged.
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

	// No PATH entry (matched case-insensitively) was found;
	// add one so the child process still sees dir on its search path.
	return append(result, "PATH="+dir)
}

// splitEnvEntry splits a "KEY=VALUE" entry into key and value.
// found reports whether an "=" separator was present.
func splitEnvEntry(entry string) (key, value string, found bool) {
	idx := strings.IndexByte(entry, '=')
	if idx < 0 {
		return "", "", false
	}
	return entry[:idx], entry[idx+1:], true
}
