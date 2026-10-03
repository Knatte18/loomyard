// verifyparse.go parses a verify command's combined output into failure identities.
// It lives in websterengine because the verify gate is its only consumer.

package websterengine

import (
	"strings"
)

// The three legal VerifyFailure.Kind values.
const (
	FailureKindTest    = "test"
	FailureKindPackage = "package"
	FailureKindOpaque  = "opaque"
)

// VerifyFailure names one failing identity from a verify run and the tail of its output.
type VerifyFailure struct {
	// ID is the failure identity: a test name, a package path, or an opaque token.
	ID string `yaml:"id"`
	// Kind is one of FailureKindTest, FailureKindPackage, FailureKindOpaque.
	Kind string `yaml:"kind"`
	// Package is the import path the identity belongs to: the test's package or the failing package itself, and empty for an opaque identity.
	Package string `yaml:"package,omitempty"`
	// Tail is the last lines of the identity's output.
	Tail string `yaml:"tail"`
}

// opaqueFailureID is the id of the single identity reported when a red verify run yields nothing parseable: a failing go vet, go build, or non-Go command.
const opaqueFailureID = "verify-command"

// failureTailMaxLines caps every VerifyFailure.Tail.
const failureTailMaxLines = 40

// Line shapes of go test output.
const (
	failHeaderPrefix = "--- FAIL:"
	failPkgPrefix    = "FAIL\t"
	okPkgPrefix      = "ok  \t"
	noTestPkgPrefix  = "?   \t"
	// failSummaryLine is the bare line a test binary prints last when it finishes with failing tests.
	failSummaryLine = "FAIL"
)

// subtestIndent is the number of spaces go test indents each subtest level's "--- FAIL:" header by.
const subtestIndent = 4

// parseVerifyFailures returns the failure identities in output, in first-seen order and deduplicated by id.
// It returns nil when passed is true.
// A test identity is "<package>.<test path>", naming the deepest failing test or subtest as go test prints it, so a failing TestX/a and a failing TestX/b are distinct identities.
// A package identity is "<package>" and marks a failure no named test explains:
// a build or setup failure, a failing package with no failing test, or a test binary that crashed (a panic, a fatal error, a timeout) or exited before reporting its result, which also hides every test it never ran.
// Each test and package identity carries its import path in Package; the opaque identity has none.
// When neither is found, one opaque identity (opaqueFailureID) carries the output tail.
func parseVerifyFailures(output string, passed bool) []VerifyFailure {
	if passed {
		return nil
	}

	output = strings.ReplaceAll(output, "\r\n", "\n")
	lines := strings.Split(output, "\n")

	var failures []VerifyFailure
	seen := map[string]bool{}
	add := func(f VerifyFailure) {
		if seen[f.ID] {
			return
		}
		seen[f.ID] = true
		failures = append(failures, f)
	}

	var block packageBlock
	blockStart := 0
	for i, line := range lines {
		if name, indent, ok := parseFailHeader(line); ok {
			block.addHeader(name, indent, lines[i+1:])
			continue
		}

		switch {
		case line == failSummaryLine:
			block.summarized = true
		case strings.HasPrefix(line, "panic: ") || strings.HasPrefix(line, "fatal error: "):
			block.crashed = true
		case strings.HasPrefix(line, failPkgPrefix):
			fields := strings.Fields(strings.TrimPrefix(line, failPkgPrefix))
			if len(fields) == 0 {
				continue
			}
			pkg := fields[0]
			for _, t := range block.leaves() {
				add(VerifyFailure{ID: pkg + "." + t.name, Kind: FailureKindTest, Package: pkg, Tail: capTail(t.tail)})
			}
			if len(block.tests) == 0 || block.crashed || !block.summarized {
				add(VerifyFailure{ID: pkg, Kind: FailureKindPackage, Package: pkg, Tail: outputTail(lines[blockStart : i+1])})
			}
			block = packageBlock{}
			blockStart = i + 1
		case strings.HasPrefix(line, okPkgPrefix) || strings.HasPrefix(line, noTestPkgPrefix):
			block = packageBlock{}
			blockStart = i + 1
		}
	}

	if len(failures) == 0 {
		return []VerifyFailure{{ID: opaqueFailureID, Kind: FailureKindOpaque, Tail: outputTail(lines)}}
	}
	return failures
}

// failingTest is one "--- FAIL:" header of a package's output: the test path and the lines nested under it.
type failingTest struct {
	name string
	tail []string
}

// packageBlock accumulates one package's go test output until its "FAIL\t<package>" line names the package.
type packageBlock struct {
	tests []failingTest
	// summarized records the bare "FAIL" line a test binary prints when it finishes with failing tests;
	// a block without it ended abnormally.
	summarized bool
	// crashed records a column-0 panic or fatal error, which stops the test binary mid-run.
	crashed bool
}

// addHeader records the failing test name whose header sits at indent, with the following lines nested deeper than it as its tail.
// go test prints a top-level header at column 0 and nests each subtest header four spaces per level under its parent's block,
// so an indented header is accepted only at exactly its name's nesting depth and under an already-recorded parent;
// any other indented header is a test's own log output, never a new identity.
func (b *packageBlock) addHeader(name string, indent int, following []string) {
	if indent != 0 {
		slash := strings.LastIndex(name, "/")
		if slash < 0 || indent != subtestIndent*strings.Count(name, "/") || !b.has(name[:slash]) {
			return
		}
	}
	if b.has(name) {
		return
	}

	var tail []string
	for _, l := range following {
		if strings.TrimSpace(l) == "" || leadingWhitespace(l) <= indent {
			break
		}
		tail = append(tail, l)
	}
	b.tests = append(b.tests, failingTest{name: name, tail: tail})
}

// has reports whether name is already recorded in the block.
func (b *packageBlock) has(name string) bool {
	for _, t := range b.tests {
		if t.name == name {
			return true
		}
	}
	return false
}

// leaves returns the recorded tests with no recorded failing subtest, in first-seen order:
// a parent fails whenever a subtest does, so only the deepest failing names identify what broke.
func (b *packageBlock) leaves() []failingTest {
	var out []failingTest
	for _, t := range b.tests {
		leaf := true
		for _, other := range b.tests {
			if strings.HasPrefix(other.name, t.name+"/") {
				leaf = false
				break
			}
		}
		if leaf {
			out = append(out, t)
		}
	}
	return out
}

// parseFailHeader returns the test name and indentation of a "--- FAIL: <name> (<duration>)" line.
func parseFailHeader(line string) (name string, indent int, ok bool) {
	indent = leadingWhitespace(line)
	rest, found := strings.CutPrefix(line[indent:], failHeaderPrefix)
	if !found {
		return "", 0, false
	}
	fields := strings.Fields(rest)
	if len(fields) == 0 {
		return "", 0, false
	}
	return fields[0], indent, true
}

// leadingWhitespace returns the number of leading space and tab bytes in line.
func leadingWhitespace(line string) int {
	return len(line) - len(strings.TrimLeft(line, " \t"))
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
