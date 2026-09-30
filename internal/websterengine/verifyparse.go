// verifyparse.go parses a verify command's combined output into failure identities.
// It lives in websterengine because webster's triage is its only consumer.

package websterengine

import (
	"strings"
)

// opaqueFailureID is the id of the single identity reported when a red verify run yields nothing parseable: a failing go vet, go build, or non-Go command.
const opaqueFailureID = "verify-command"

// failureTailMaxLines caps every IntegrationFailure.Tail.
const failureTailMaxLines = 40

const (
	failHeaderPrefix = "--- FAIL:"
	failPkgPrefix    = "FAIL\t"
)

// parseVerifyFailures returns the failure identities in output, in first-seen order and deduplicated by id.
// It returns nil when passed is true.
// Test identities are "<package>.<TopLevelTest>", package identities are "<package>",
// and when neither is found one opaque identity (opaqueFailureID) carries the output tail.
func parseVerifyFailures(output string, passed bool) []IntegrationFailure {
	if passed {
		return nil
	}

	output = strings.ReplaceAll(output, "\r\n", "\n")
	lines := strings.Split(output, "\n")

	var failures []IntegrationFailure
	seen := map[string]bool{}
	add := func(f IntegrationFailure) {
		if seen[f.ID] {
			return
		}
		seen[f.ID] = true
		failures = append(failures, f)
	}

	// pending holds this package block's failing top-level tests until its FAIL line names the package.
	var pending []IntegrationFailure
	pendingSeen := map[string]bool{}

	for i := 0; i < len(lines); i++ {
		line := lines[i]

		// go test prints a top-level header at column 0 and nests each subtest header inside its block,
		// so an indented header is a subtest or a test's own log output, never a new identity.
		if rest, ok := strings.CutPrefix(line, failHeaderPrefix); ok {
			fields := strings.Fields(rest)
			if len(fields) == 0 {
				continue
			}
			top, _, _ := strings.Cut(fields[0], "/")
			if pendingSeen[top] {
				continue
			}
			pendingSeen[top] = true

			var tail []string
			for j := i + 1; j < len(lines) && isIndented(lines[j]); j++ {
				tail = append(tail, lines[j])
			}
			pending = append(pending, IntegrationFailure{
				ID:   top,
				Kind: FailureKindTest,
				Tail: capTail(tail),
			})
			continue
		}

		if rest, ok := strings.CutPrefix(line, failPkgPrefix); ok {
			fields := strings.Fields(rest)
			if len(fields) == 0 {
				continue
			}
			pkg := fields[0]
			if len(pending) == 0 {
				add(IntegrationFailure{ID: pkg, Kind: FailureKindPackage, Tail: outputTail(lines)})
				continue
			}
			for _, t := range pending {
				t.ID = pkg + "." + t.ID
				add(t)
			}
			pending = nil
			pendingSeen = map[string]bool{}
		}
	}

	if len(failures) == 0 {
		return []IntegrationFailure{{ID: opaqueFailureID, Kind: FailureKindOpaque, Tail: outputTail(lines)}}
	}
	return failures
}

// isIndented reports whether line is a non-blank line starting with whitespace.
func isIndented(line string) bool {
	return line != "" && (line[0] == ' ' || line[0] == '\t')
}

// outputTail returns the last failureTailMaxLines lines, ignoring trailing blank lines.
func outputTail(lines []string) string {
	end := len(lines)
	for end > 0 && strings.TrimSpace(lines[end-1]) == "" {
		end--
	}
	return capTail(lines[:end])
}

// capTail keeps the last failureTailMaxLines of lines and joins them with newlines.
func capTail(lines []string) string {
	if len(lines) > failureTailMaxLines {
		lines = lines[len(lines)-failureTailMaxLines:]
	}
	return strings.Join(lines, "\n")
}
