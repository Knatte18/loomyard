// census.go counts the git processes a measured package spawns, by subcommand, through git's trace2 event files, and splits fixture setup from code under test on the fixture marker.

package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/Knatte18/loomyard/internal/gitkit"
)

// unknownSubcommand is the census key of a trace file that names no command.
const unknownSubcommand = "unknown"

// subcommandCount is how many git processes ran one subcommand, split by the fixture marker.
type subcommandCount struct {
	fixture int
	code    int
}

// gitCensus holds the git processes of one package: per subcommand, and in total.
type gitCensus struct {
	subcommands map[string]subcommandCount
	fixture     int
	code        int
}

// add records one git process.
func (c *gitCensus) add(subcommand string, fixture bool) {
	if c.subcommands == nil {
		c.subcommands = map[string]subcommandCount{}
	}
	count := c.subcommands[subcommand]
	if fixture {
		count.fixture++
		c.fixture++
	} else {
		count.code++
		c.code++
	}
	c.subcommands[subcommand] = count
}

// traceEnv returns the environment lines that make every git process under a child write one trace2 event file into dir.
// The marker's name is exported into the events, and trace2.maxFiles is set to 0 so git never stops writing files at its cap.
func traceEnv(dir string) []string {
	return []string{
		"GIT_TRACE2_EVENT=" + dir,
		"GIT_TRACE2_ENV_VARS=" + gitkit.FixtureGitEnv,
		"GIT_CONFIG_COUNT=1",
		"GIT_CONFIG_KEY_0=trace2.maxFiles",
		"GIT_CONFIG_VALUE_0=0",
	}
}

// readCensus reads every trace2 event file in dir into a census, one git process per file.
func readCensus(dir string) (gitCensus, error) {
	var census gitCensus
	entries, err := os.ReadDir(dir)
	if err != nil {
		return census, fmt.Errorf("read the trace directory: %w", err)
	}
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		events, err := os.ReadFile(filepath.Join(dir, entry.Name()))
		if err != nil {
			return census, fmt.Errorf("read trace file %s: %w", entry.Name(), err)
		}
		if subcommand, fixture, ok := parseTraceEvents(events); ok {
			census.add(subcommand, fixture)
		}
	}
	return census, nil
}

// traceEvent is the decoded part of one trace2 event line.
type traceEvent struct {
	Event string `json:"event"`
	Name  string `json:"name"`
	Param string `json:"param"`
}

// parseTraceEvents reads one git process's trace2 event file.
// The subcommand is the `cmd_name` event's name, or unknownSubcommand when the file has none; fixture is whether a `def_param` event names the marker.
// ok is false when the file holds no event at all.
func parseTraceEvents(events []byte) (subcommand string, fixture bool, ok bool) {
	subcommand = unknownSubcommand
	for _, line := range bytes.Split(events, []byte("\n")) {
		var event traceEvent
		if json.Unmarshal(line, &event) != nil || event.Event == "" {
			continue
		}
		ok = true
		switch {
		case event.Event == "cmd_name" && event.Name != "":
			subcommand = event.Name
		case event.Event == "def_param" && event.Param == gitkit.FixtureGitEnv:
			fixture = true
		}
	}
	return subcommand, fixture, ok
}

// renderCensus formats the git census of all rows as one table, one row per subcommand, busiest first.
func renderCensus(rows []resourceRow) string {
	var total gitCensus
	for _, row := range rows {
		for subcommand, count := range row.census.subcommands {
			merged := total.subcommands[subcommand]
			merged.fixture += count.fixture
			merged.code += count.code
			if total.subcommands == nil {
				total.subcommands = map[string]subcommandCount{}
			}
			total.subcommands[subcommand] = merged
		}
		total.fixture += row.census.fixture
		total.code += row.census.code
	}

	names := make([]string, 0, len(total.subcommands))
	for name := range total.subcommands {
		names = append(names, name)
	}
	sort.Slice(names, func(i, j int) bool {
		left, right := total.subcommands[names[i]], total.subcommands[names[j]]
		if leftSum, rightSum := left.fixture+left.code, right.fixture+right.code; leftSum != rightSum {
			return leftSum > rightSum
		}
		return names[i] < names[j]
	})

	var out strings.Builder
	out.WriteString("Git processes by subcommand\n\n")
	fmt.Fprintf(&out, "%-30s  %8s  %8s  %8s\n", "SUBCOMMAND", "TOTAL", "FIXTURE", "CODE")
	fmt.Fprintf(&out, "%-30s  %8s  %8s  %8s\n", strings.Repeat("-", 30), "--------", "--------", "--------")
	for _, name := range names {
		count := total.subcommands[name]
		fmt.Fprintf(&out, "%-30s  %8d  %8d  %8d\n", name, count.fixture+count.code, count.fixture, count.code)
	}
	fmt.Fprintf(&out, "%-30s  %8d  %8d  %8d\n", "TOTAL", total.fixture+total.code, total.fixture, total.code)
	out.WriteString("\nFIXTURE counts git that carried the fixture marker, CODE all other git.\n")
	out.WriteString("The split is exact for a serial package and approximate where a hub build overlaps other tests of the same package, because the marker is process-wide during a build; compare before and after on the total.\n")
	return out.String()
}
