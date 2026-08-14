// validate.go implements Validate, which compares each registry stencil's on-disk marker set
// against its shipped default's marker set and reports the mismatches as Findings.

package stencilstore

import (
	"fmt"
	"os"
	"sort"

	"github.com/Knatte18/loomyard/internal/stencil"
)

// Finding severities Validate can report.
const (
	SeverityError   = "error"
	SeverityWarning = "warning"
)

// Finding describes one marker mismatch between a stencil's on-disk body and its shipped default.
type Finding struct {
	// Name is the stencil the finding concerns.
	Name string
	// Marker is the offending top-level marker name.
	Marker string
	// Severity is SeverityError or SeverityWarning.
	Severity string
}

// Validate compares each registry stencil's on-disk marker set against its shipped default's marker
// set via stencil.TopLevelMarkers, and reports the mismatches.
// A marker present on disk but absent from the shipped default is SeverityError, since it will break
// stencil.Fill at the point of use.
// A shipped-default marker absent from the on-disk body is SeverityWarning: legal customisation that
// silently drops that content.
// Findings are sorted by name then marker.
// A stencil missing from disk is an error return, not a finding.
func Validate(baseDir string, registry Registry) ([]Finding, error) {
	var findings []Finding

	for _, name := range registry.Names() {
		shipped, known := registry.Default(name)
		if !known {
			continue
		}

		onDisk, err := os.ReadFile(Path(baseDir, name))
		if err != nil {
			return nil, fmt.Errorf("stencilstore: validate stencil %q: %w", name, err)
		}

		onDiskMarkers, err := stencil.TopLevelMarkers(onDisk)
		if err != nil {
			return nil, fmt.Errorf("stencilstore: validate stencil %q: parse on-disk template: %w", name, err)
		}
		shippedMarkers, err := stencil.TopLevelMarkers(shipped)
		if err != nil {
			return nil, fmt.Errorf("stencilstore: validate stencil %q: parse shipped template: %w", name, err)
		}

		shippedSet := make(map[string]bool, len(shippedMarkers))
		for _, marker := range shippedMarkers {
			shippedSet[marker] = true
		}
		onDiskSet := make(map[string]bool, len(onDiskMarkers))
		for _, marker := range onDiskMarkers {
			onDiskSet[marker] = true
		}

		for _, marker := range onDiskMarkers {
			if !shippedSet[marker] {
				findings = append(findings, Finding{Name: name, Marker: marker, Severity: SeverityError})
			}
		}
		for _, marker := range shippedMarkers {
			if !onDiskSet[marker] {
				findings = append(findings, Finding{Name: name, Marker: marker, Severity: SeverityWarning})
			}
		}
	}

	sort.Slice(findings, func(i, j int) bool {
		if findings[i].Name != findings[j].Name {
			return findings[i].Name < findings[j].Name
		}
		return findings[i].Marker < findings[j].Marker
	})
	return findings, nil
}
