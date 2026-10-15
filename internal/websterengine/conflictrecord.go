// conflictrecord.go declares the never-tracked plan-conflict record begin-batch leaves in webster's scratch directory, and its writers and removers.
package websterengine

import (
	"fmt"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strings"

	"github.com/Knatte18/loomyard/internal/lyxdirs"
)

const (
	// conflictRecordName is the file begin-batch writes when it refuses with a plan conflict.
	conflictRecordName = "plan-conflict.md"
	// routedConflictName is the file holding the findings of the conflict last routed to plan review.
	routedConflictName = "plan-conflict-routed.md"
)

// ConflictRecordRel returns the conflict record's forward-slashed path relative to the anchor, as recipe text names it.
func ConflictRecordRel() string {
	return path.Join(lyxdirs.DotLyxDirName, websterDirName, conflictRecordName)
}

// ConflictRecordPath returns the conflict record's path under the told scratchDir.
func ConflictRecordPath(scratchDir string) string {
	return filepath.Join(scratchDir, conflictRecordName)
}

// RoutedConflictPath returns the last-routed conflict's path under the told scratchDir.
func RoutedConflictPath(scratchDir string) string {
	return filepath.Join(scratchDir, routedConflictName)
}

// WriteConflictRecord writes the conflict record naming conflict's batch, cause and blocking findings.
// The scratch directory must already exist: every caller holds a lock file in it.
func WriteConflictRecord(scratchDir string, conflict *PlanConflictError) error {
	var b strings.Builder
	fmt.Fprintf(&b, "# Plan conflict at begin-batch %02d\ncause: %s\n\n", conflict.Batch, conflict.Cause)
	for _, finding := range conflict.Findings {
		b.WriteString("- " + finding + "\n")
	}
	if err := os.WriteFile(ConflictRecordPath(scratchDir), []byte(b.String()), 0o644); err != nil {
		return fmt.Errorf("webster: write plan-conflict record: %w", err)
	}
	return nil
}

// RemoveConflictRecord removes the conflict record, and is a no-op when it is absent.
func RemoveConflictRecord(scratchDir string) error {
	return removeIfPresent(ConflictRecordPath(scratchDir))
}

// RemoveRoutedConflict removes the last-routed conflict, and is a no-op when it is absent.
func RemoveRoutedConflict(scratchDir string) error {
	return removeIfPresent(RoutedConflictPath(scratchDir))
}

// removeIfPresent removes the file at filePath and returns nil when there is none.
func removeIfPresent(filePath string) error {
	if err := os.Remove(filePath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("webster: remove %s: %w", filePath, err)
	}
	return nil
}

// ConflictRoute is what ClaimConflictRoute decided about the conflict record.
// Found is whether a record naming at least one finding exists.
// New is whether its findings differ from the last-routed ones, so a reroute to plan review has not yet been asked for them.
// Findings is the record's finding lines, set when Found.
type ConflictRoute struct {
	Found    bool
	New      bool
	Findings []string
}

// ClaimConflictRoute decides whether the conflict record asks for a new reroute to plan review.
// An absent record, or one naming no finding (a torn write), claims nothing.
// A record whose findings, compared as a set, differ from the routed file's is new: its findings replace the routed file's before the claim returns.
// A record whose findings equal the routed ones is a recurrence and writes nothing.
// A read or write error is returned.
func ClaimConflictRoute(scratchDir string) (ConflictRoute, error) {
	recordFindings, err := readFindingLines(ConflictRecordPath(scratchDir))
	if err != nil {
		return ConflictRoute{}, err
	}
	if len(recordFindings) == 0 {
		return ConflictRoute{}, nil
	}
	routedFindings, err := readFindingLines(RoutedConflictPath(scratchDir))
	if err != nil {
		return ConflictRoute{}, err
	}
	route := ConflictRoute{Found: true, Findings: recordFindings}
	if sameFindingSet(recordFindings, routedFindings) {
		return route, nil
	}
	var routed strings.Builder
	for _, finding := range recordFindings {
		routed.WriteString("- " + finding + "\n")
	}
	if err := os.WriteFile(RoutedConflictPath(scratchDir), []byte(routed.String()), 0o644); err != nil {
		return ConflictRoute{}, fmt.Errorf("webster: write routed plan-conflict file: %w", err)
	}
	route.New = true
	return route, nil
}

// readFindingLines returns the text after "- " of every finding line in the file at filePath, and nil when the file is absent.
func readFindingLines(filePath string) ([]string, error) {
	data, err := os.ReadFile(filePath)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("webster: read %s: %w", filePath, err)
	}
	var findings []string
	for _, line := range strings.Split(string(data), "\n") {
		if finding, ok := strings.CutPrefix(line, "- "); ok {
			findings = append(findings, finding)
		}
	}
	return findings, nil
}

// sameFindingSet reports whether a and b hold the same findings, order ignored.
func sameFindingSet(a, b []string) bool {
	sortedA, sortedB := slices.Clone(a), slices.Clone(b)
	slices.Sort(sortedA)
	slices.Sort(sortedB)
	return slices.Equal(sortedA, sortedB)
}
