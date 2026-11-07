// report.go holds the report's options and result types, and Compute, which builds a report from one source snapshot.

package reportengine

import "time"

// Options sets the run window and the freshness span of a report.
type Options struct {
	// Since is the UTC date before which a finished run is left out.
	Since time.Time

	// FreshDays is how many days a fix may trail the landing it repairs and still count as repairing fresh code.
	FreshDays int
}

// Selection records which branches, tips and window a report was computed over.
type Selection struct {
	ParentBranch  string    `json:"parent_branch"`
	RecordsBranch string    `json:"records_branch"`
	BoardBranch   string    `json:"board_branch"`
	ParentTip     string    `json:"parent_tip"`
	RecordsTip    string    `json:"records_tip"`
	BoardTip      string    `json:"board_tip"`
	Since         time.Time `json:"since"`
	FreshDays     int       `json:"fresh_days"`
}

// Finding is a record that could not be read: the run's tag, the tree-relative file and the error.
type Finding struct {
	Tag   string `json:"tag"`
	File  string `json:"file"`
	Error string `json:"error"`
}

// Report holds the runs, partial runs, landings and weeks of one report, with the selection they came from.
// A figure for a source a run or week lacks is absent, never zero.
type Report struct {
	Selection Selection    `json:"selection"`
	Runs      []RunRow     `json:"runs"`
	Partial   []PartialRun `json:"partial"`
	Landings  []Landing    `json:"landings"`
	Weeks     []Week       `json:"weeks"`

	// Absent counts, per source, the selected runs lacking it.
	Absent   map[string]int `json:"absent"`
	Findings []Finding      `json:"findings"`

	// inputs holds the data that run selection hands the landing join.
	inputs landingInputs
}

// Compute selects the finished runs of src and reads each run's sources.
// An unreadable record is recorded as a finding, never returned as an error.
// An error means a failed read of the repositories themselves.
func Compute(src Sources, opts Options) (Report, error) {
	report := Report{
		Selection: Selection{
			ParentBranch:  src.ParentBranch,
			RecordsBranch: src.RecordsBranch,
			BoardBranch:   src.BoardBranch,
			ParentTip:     src.ParentTip,
			RecordsTip:    src.RecordsTip,
			BoardTip:      src.BoardTip,
			Since:         opts.Since,
			FreshDays:     opts.FreshDays,
		},
		Runs:     []RunRow{},
		Partial:  []PartialRun{},
		Landings: []Landing{},
		Findings: []Finding{},
	}
	if err := selectRuns(src, opts, &report); err != nil {
		return Report{}, err
	}
	markAbsentSegments(report.Runs)
	if err := readLandings(src, opts, &report); err != nil {
		return Report{}, err
	}
	report.Weeks = computeWeeks(report)
	report.Absent = absentCounts(report.Runs)
	return report, nil
}
