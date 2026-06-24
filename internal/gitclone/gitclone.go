// Package gitclone bootstraps a fresh lyx Hub by cloning host, weft, and board repositories
// into a dormant Hub directory. The clone operation is deterministic, creates no junctions,
// and does not activate lyx — the produced Hub is inert until lyx init is run separately.
// This package owns the URL-derivation logic, clone orchestration with strict-abort teardown,
// and the JSON CLI entry point (RunCLI).
package gitclone

import (
	"strings"
)

const (
	// hubSuffix is the directory suffix appended to the derived repo name to form the Hub directory.
	hubSuffix = "-HUB"

	// weftSuffix is the directory suffix appended to the repo name to form the weft directory.
	weftSuffix = "-weft"

	// boardDirName is the directory name for the board repository within the Hub.
	boardDirName = "_board"
)

// deriveHostName extracts the host repository basename from a raw URL or file path.
//
// It trims any trailing slash or backslash, then takes the final path segment of rawURL
// after splitting on forward slash, backslash, and colon (for HTTPS URLs, file paths,
// and SCP-form URLs like git@github.com:user/repo.git).
// A single trailing .git extension is stripped if present. Returns an empty string if no
// basename can be extracted or if the URL contains no path segments.
//
// Examples:
//   - "https://github.com/u/repo.git" → "repo"
//   - "https://github.com/u/repo" → "repo"
//   - "git@github.com:u/repo.git" → "repo"
//   - "https://github.com/u/repo/" → "repo"
//   - "C:\path\to\repo.git" → "repo"
//   - "" → ""
func deriveHostName(rawURL string) string {
	// Trim trailing slashes (both forward and back)
	rawURL = strings.TrimSuffix(rawURL, "/")
	rawURL = strings.TrimSuffix(rawURL, "\\")

	// Split on forward slash, backslash, and colon to handle HTTPS, file paths, and SCP forms
	var parts []string
	for _, seg := range strings.FieldsFunc(rawURL, func(r rune) bool { return r == '/' || r == '\\' || r == ':' }) {
		if seg != "" {
			parts = append(parts, seg)
		}
	}

	if len(parts) == 0 {
		return ""
	}

	// Take the last segment and strip .git suffix
	name := parts[len(parts)-1]
	name = strings.TrimSuffix(name, ".git")

	return name
}

// deriveBoardURL derives the board repository URL from a weft repository URL.
//
// It strips a single trailing .git suffix from weftURL if present, then appends .wiki.git.
// This ensures that both "…/weft.git" and "…/weft" yield "…/weft.wiki.git".
//
// Examples:
//   - "https://github.com/u/weft.git" → "https://github.com/u/weft.wiki.git"
//   - "https://github.com/u/weft" → "https://github.com/u/weft.wiki.git"
func deriveBoardURL(weftURL string) string {
	weftURL = strings.TrimSuffix(weftURL, ".git")
	return weftURL + ".wiki.git"
}
