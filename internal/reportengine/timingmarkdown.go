// timingmarkdown.go renders a TimingReport as markdown: the selection and sleep note, then a table of runs, of phase medians, of time per bucket and of stops per settler.

package reportengine

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/Knatte18/loomyard/internal/durfmt"
)

// The headings of RenderTimingMarkdown's four sections.
const (
	HeadingTimingRuns     = "## Runs"
	HeadingTimingPhases   = "## Phase medians"
	HeadingTimingBuckets  = "## Time per bucket"
	HeadingTimingSettlers = "## Stops by settler"
)

// timingStopKinds lists every stop kind in report order.
var timingStopKinds = []string{StopKindStop, StopKindBounce, StopKindBattenHalt}

// RenderTimingMarkdown renders r as one document: the selection and sleep note, then the run, phase-median, bucket and stops-by-settler tables.
// Every duration is spelled through durfmt.FormatSeconds, and a figure the report holds as nil renders as no data.
func RenderTimingMarkdown(r TimingReport) string {
	var b strings.Builder
	b.WriteString("# Timing report\n\n")
	s := r.Selection
	inFlight := 0
	for _, run := range r.Runs {
		if run.State == RunStateInFlight {
			inFlight++
		}
	}
	fmt.Fprintf(&b, "- records branch: %s at %s\n", s.RecordsBranch, s.RecordsTip)
	fmt.Fprintf(&b, "- since: %s\n", s.Since.UTC().Format("2006-01-02"))
	fmt.Fprintf(&b, "- bucket: %s\n", s.By)
	fmt.Fprintf(&b, "- snapshot: %s\n", s.SnapshotAt.UTC().Format(time.RFC3339))
	fmt.Fprintf(&b, "- runs: %d, in flight: %d, findings: %d\n", len(r.Runs), inFlight, len(r.Findings))
	note := r.SleepNote
	if note == "" {
		note = noData
	}
	fmt.Fprintf(&b, "- sleep: %s\n", note)

	b.WriteString("\n" + HeadingTimingRuns + "\n\n")
	runHeader := []string{"Run", "State", "Start", "Span", "Active"}
	runHeader = append(runHeader, timingWaitKinds...)
	runHeader = append(runHeader, "Sleep")
	runHeader = append(runHeader, timingPhases...)
	runHeader = append(runHeader, "Arbiter attempts", "Alignment attempts", "Parent-review alignment attempts")
	table(&b, runHeader, timingRunRows(r.Runs))

	b.WriteString("\n" + HeadingTimingPhases + "\n\n")
	table(&b, []string{"Phase", "Median", "Runs"}, phaseMedianRows(r.Phases))

	b.WriteString("\n" + HeadingTimingBuckets + "\n\n")
	bucketHeader := []string{"Bucket", "Runs"}
	bucketHeader = append(bucketHeader, timingPhases...)
	bucketHeader = append(bucketHeader, timingWaitKinds...)
	bucketHeader = append(bucketHeader, "Active", "Sleep")
	table(&b, bucketHeader, timingBucketRows(r.Buckets))

	b.WriteString("\n" + HeadingTimingSettlers + "\n\n")
	table(&b, []string{"Bucket", "Stop kind", "Settler", "Count"}, settlerRows(r.Buckets))
	return b.String()
}

// formatSeconds spells a duration of whole seconds through durfmt.FormatSeconds, no data when nil.
func formatSeconds(seconds *int64) string {
	if seconds == nil {
		return noData
	}
	return durfmt.FormatSeconds(time.Duration(*seconds) * time.Second)
}

// secondsOf spells the seconds held under key, no data when the map has none.
func secondsOf(seconds map[string]int64, key string) string {
	value, held := seconds[key]
	if !held {
		return noData
	}
	return formatSeconds(&value)
}

// formatAttempts spells an attempt figure as its count and, in parentheses, its time.
func formatAttempts(figure AttemptFigure) string {
	return fmt.Sprintf("%d (%s)", figure.Count, formatSeconds(figure.Seconds))
}

func timingRunRows(runs []TimingRun) [][]string {
	rows := make([][]string, len(runs))
	for i, run := range runs {
		row := []string{run.Slug, run.State, run.Start.UTC().Format(time.RFC3339), formatSeconds(&run.SpanSeconds), formatSeconds(&run.ActiveSeconds)}
		for _, kind := range timingWaitKinds {
			row = append(row, secondsOf(run.Waits, kind))
		}
		row = append(row, formatSeconds(&run.SleepSeconds))
		for _, phase := range timingPhases {
			row = append(row, formatSeconds(run.Phases[phase]))
		}
		row = append(row, formatAttempts(run.Attempts.Arbiter), formatAttempts(run.Attempts.Alignment), formatAttempts(run.Attempts.ParentReviewAlignment))
		rows[i] = row
	}
	return rows
}

func phaseMedianRows(medians map[string]*PhaseMedian) [][]string {
	var rows [][]string
	for _, phase := range timingPhases {
		if median := medians[phase]; median != nil {
			rows = append(rows, []string{phase, formatSeconds(median.Seconds), fmt.Sprint(median.Runs)})
		}
	}
	return rows
}

func timingBucketRows(buckets []TimingBucket) [][]string {
	rows := make([][]string, len(buckets))
	for i, bucket := range buckets {
		row := []string{bucket.Label, fmt.Sprint(bucket.Runs)}
		for _, phase := range timingPhases {
			row = append(row, secondsOf(bucket.Phases, phase))
		}
		for _, kind := range timingWaitKinds {
			row = append(row, secondsOf(bucket.Waits, kind))
		}
		row = append(row, formatSeconds(&bucket.ActiveSeconds), formatSeconds(&bucket.SleepSeconds))
		rows[i] = row
	}
	return rows
}

func settlerRows(buckets []TimingBucket) [][]string {
	var rows [][]string
	for _, bucket := range buckets {
		for _, kind := range timingStopKinds {
			settlers := make([]string, 0, len(bucket.Settlers[kind]))
			for settler := range bucket.Settlers[kind] {
				settlers = append(settlers, settler)
			}
			sort.Strings(settlers)
			for _, settler := range settlers {
				rows = append(rows, []string{bucket.Label, kind, settler, fmt.Sprint(bucket.Settlers[kind][settler])})
			}
		}
	}
	return rows
}
