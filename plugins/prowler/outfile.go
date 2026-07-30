// outfile.go writes each fetch's extracted markdown to a uniquely-named file
// under the scratch output directory, and derives a human-readable slug for
// that filename from the fetched URL so a directory listing stays legible.

package main

import (
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// nonAlnumRun matches any run of characters outside [a-z0-9], used to collapse
// a URL's host/path into a filesystem-safe slug.
var nonAlnumRun = regexp.MustCompile(`[^a-z0-9]+`)

// slugForURL derives a short, descriptive, filesystem-safe slug from raw so a
// .scratch/ directory listing reads as "which page was this" at a glance, even
// though the actual uniqueness guarantee comes from os.CreateTemp's random
// suffix in writeOutput, not from this slug.
func slugForURL(raw string) string {
	// An unparseable URL still needs a usable filename, so fall back to a
	// fixed placeholder rather than propagating the parse error.
	parsed, err := url.Parse(raw)
	if err != nil {
		return "page"
	}

	// Combine host and the first non-empty path segment; either may be
	// absent (e.g. a bare path or a host-only URL), so build up from parts
	// that actually exist.
	parts := []string{parsed.Host}
	for _, seg := range strings.Split(parsed.Path, "/") {
		if seg != "" {
			parts = append(parts, seg)
			break
		}
	}
	joined := strings.ToLower(strings.Join(parts, "-"))

	// Collapse every run of non-alphanumeric characters to a single hyphen so
	// the result is safe as a filename component on every target OS.
	slug := nonAlnumRun.ReplaceAllString(joined, "-")
	slug = strings.Trim(slug, "-")

	const maxSlugLen = 40
	if len(slug) > maxSlugLen {
		slug = strings.Trim(slug[:maxSlugLen], "-")
	}

	if slug == "" {
		return "page"
	}
	return slug
}

// writeOutput creates a uniquely-named markdown file under .scratch/ (creating
// that directory if absent), writes content into it, and returns the file's
// absolute path. os.CreateTemp's random suffix guarantees the returned path is
// distinct even across concurrent calls from parallel agents or repeat
// invocations by the same agent, which a caller-computed name (e.g. PID plus
// timestamp) cannot guarantee.
func writeOutput(firstURL, content string) (string, error) {
	const scratchDir = ".scratch"
	if err := os.MkdirAll(scratchDir, 0o755); err != nil {
		return "", err
	}

	pattern := "prowler-" + slugForURL(firstURL) + "-*.md"
	f, err := os.CreateTemp(scratchDir, pattern)
	if err != nil {
		return "", err
	}
	// Capture the name now: it is needed for the return value below, and
	// os.File no longer exposes it once the caller only holds the returned path.
	name := f.Name()

	if _, err := f.WriteString(content); err != nil {
		f.Close()
		return "", err
	}
	if err := f.Close(); err != nil {
		return "", err
	}

	return filepath.Abs(name)
}
