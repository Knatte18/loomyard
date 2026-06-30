// selfreport.go contains the gh-CLI executor, argument builder, and CreateIssue
// domain function for the selfreportengine package. It holds everything from the
// selfreport module that does not belong to the cobra command layer:
// the RunGH seam, realRunGH, buildCreateArgs, lastNonEmptyLine, and CreateIssue.

// Package selfreportengine provides the domain kernel for filing GitHub issues
// via the gh CLI. It exposes CreateIssue as the single entry point and RunGH
// as a swappable seam for testing, keeping all implementation details
// (targetRepo, buildCreateArgs, lastNonEmptyLine, realRunGH) unexported.
package selfreportengine

import (
	"bytes"
	"errors"
	"fmt"
	"os/exec"
	"strconv"
	"strings"

	"github.com/Knatte18/loomyard/internal/proc"
)

// targetRepo is the hardcoded GitHub repository that all issues are filed against.
// It is a constant so that tests can verify the exact --repo argument without any
// config-file or environment-variable indirection.
const targetRepo = "Knatte18/loomyard"

// RunGH is the seam through which all gh invocations flow.
// Tests replace it with a fake that records the argv and returns caller-specified
// values, so no real gh binary or network is needed during unit testing.
var RunGH = realRunGH

// realRunGH executes the gh CLI with the supplied argument slice and returns the
// captured stdout, stderr, exit code, and any non-exit-code error.
//
// It first calls exec.LookPath("gh") to confirm the binary is on PATH; when gh
// is not found it returns the LookPath error immediately so that CreateIssue can
// distinguish a missing binary from a generic exec failure via errors.Is.
// Otherwise it runs gh, captures stdout/stderr into buffers, suppresses a console
// window on Windows via proc.HideWindow, and extracts the exit code from
// *exec.ExitError so that a non-zero gh exit is not surfaced as a Go error.
func realRunGH(args []string) (stdout, stderr string, exitCode int, err error) {
	// Confirm the binary is on PATH before attempting to run it; the LookPath
	// error is structurally distinct from a runtime exec failure and lets callers
	// use errors.Is(err, exec.ErrNotFound) to generate a clearer message.
	if _, lookErr := exec.LookPath("gh"); lookErr != nil {
		return "", "", -1, lookErr
	}

	cmd := exec.Command("gh", args...)

	var outBuf, errBuf bytes.Buffer
	cmd.Stdout = &outBuf
	cmd.Stderr = &errBuf

	// Suppress the console window on Windows; no-op on other platforms.
	proc.HideWindow(cmd)

	runErr := cmd.Run()

	// Extract the exit code from *exec.ExitError; a non-zero exit is not a
	// Go error for this function — only genuine exec failures propagate as err.
	exitCode = 0
	if exitErr, ok := runErr.(*exec.ExitError); ok {
		exitCode = exitErr.ExitCode()
	} else if runErr != nil {
		// A non-ExitError failure (e.g. IO error) propagates with exit code -1
		// to distinguish it from a normal non-zero exit.
		return "", "", -1, runErr
	}

	return outBuf.String(), errBuf.String(), exitCode, nil
}

// buildCreateArgs assembles the gh argument list for creating an issue on the
// target repository. The body argument is included only when non-nil, and each
// label in labels gets its own --label flag pair, preserving multi-label order.
func buildCreateArgs(title string, body *string, labels []string) []string {
	args := []string{"issue", "create", "--repo", targetRepo, "--title", title}
	if body != nil {
		args = append(args, "--body", *body)
	}
	for _, l := range labels {
		args = append(args, "--label", l)
	}
	return args
}

// CreateIssue files a GitHub issue with the given title, optional body, and
// labels via the gh CLI. It returns the issue URL and parsed issue number on
// success. When the number cannot be parsed from the URL path segment, it returns
// (url, 0, nil) — a zero number signals cli.go to omit the "number" field from
// the JSON envelope.
//
// Error handling distinguishes three cases:
//   - gh binary not on PATH: errors.Is(err, exec.ErrNotFound) → "gh not found on PATH"
//   - generic exec failure: non-ExitError err → "failed to run gh"
//   - gh reported non-zero exit: exitCode != 0 → "gh issue create failed: <stderr>"
func CreateIssue(title string, body *string, labels []string) (url string, number int, err error) {
	stdout, stderr, exitCode, runErr := RunGH(buildCreateArgs(title, body, labels))
	if runErr != nil {
		// Distinguish a missing gh binary from a generic exec failure so the
		// error message guides the user toward installing gh vs. investigating
		// a runtime problem.
		if errors.Is(runErr, exec.ErrNotFound) {
			return "", 0, fmt.Errorf("gh not found on PATH: %w", runErr)
		}
		return "", 0, fmt.Errorf("failed to run gh: %w", runErr)
	}
	if exitCode != 0 {
		return "", 0, fmt.Errorf("gh issue create failed: %s", strings.TrimSpace(stderr))
	}

	// Take the last non-empty trimmed line of stdout as the issue URL; gh prints
	// the URL on the last line of successful output.
	url = lastNonEmptyLine(stdout)

	// Parse the trailing path segment as the issue number; if it is not an integer
	// (e.g. the URL format changed or stdout was unexpected), return 0 so the caller
	// omits "number" from the JSON envelope rather than surfacing a spurious error.
	segments := strings.Split(url, "/")
	if len(segments) > 0 {
		if n, parseErr := strconv.Atoi(segments[len(segments)-1]); parseErr == nil {
			number = n
		}
	}

	return url, number, nil
}

// lastNonEmptyLine returns the last non-empty trimmed line from s.
// It is used to extract the issue URL from gh's stdout, which may contain
// trailing newlines or blank lines after the URL.
func lastNonEmptyLine(s string) string {
	lines := strings.Split(s, "\n")
	for i := len(lines) - 1; i >= 0; i-- {
		if t := strings.TrimSpace(lines[i]); t != "" {
			return t
		}
	}
	return ""
}
