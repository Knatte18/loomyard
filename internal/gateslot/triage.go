// triage.go reads the triage lines out of a failure report and renders them as a findings section.

package gateslot

import (
	"fmt"
	"maps"
	"slices"
	"strings"
)

// TriageLines returns, per failed test, the triage lines the first failure report in log holds under it.
// A test is named `<package>.<Test>` as the report's two-space test line names it, a subtest path included.
// Each line keeps its TriageLinePrefix.
// It returns nil when log holds no report or the report holds no triage line.
func TriageLines(log string) map[string][]string {
	lines := strings.Split(strings.ReplaceAll(log, "\r\n", "\n"), "\n")
	start := slices.Index(lines, FailureReportHeader)
	if start < 0 {
		return nil
	}
	var triage map[string][]string
	test := ""
	for _, line := range lines[start+1:] {
		if line == "" || strings.HasPrefix(line, failureReportEnd) {
			break
		}
		switch {
		case strings.HasPrefix(line, "    "):
			text := strings.TrimPrefix(line, "    ")
			if test == "" || !strings.HasPrefix(text, TriageLinePrefix) {
				continue
			}
			if triage == nil {
				triage = map[string][]string{}
			}
			triage[test] = append(triage[test], text)
		case strings.HasPrefix(line, "  "):
			test = reportTestIdentity(line)
		default:
			test = ""
		}
	}
	return triage
}

// reportTestIdentity returns `<package>.<Test>` for a failure report's `  <package> <test>:` line, subtest path included.
// Any other line under a run, such as a failure outside any test or a cause, names none.
func reportTestIdentity(line string) string {
	fields := strings.Fields(line)
	if len(fields) != 2 || !strings.HasSuffix(fields[1], ":") {
		return ""
	}
	test := strings.TrimSuffix(fields[1], ":")
	if top, _, _ := strings.Cut(test, "/"); !strings.HasPrefix(top, "Test") {
		return ""
	}
	return fields[0] + "." + test
}

// RenderTriageSection returns the `Triage:` section of a findings text.
// It lists each test of triage, sorted, with its lines under it.
// It returns "" when triage holds no line.
func RenderTriageSection(triage map[string][]string) string {
	if len(triage) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString("Triage:\n")
	for _, test := range slices.Sorted(maps.Keys(triage)) {
		fmt.Fprintf(&b, "- %s\n", test)
		for _, line := range triage[test] {
			fmt.Fprintf(&b, "    %s\n", line)
		}
	}
	return b.String()
}
