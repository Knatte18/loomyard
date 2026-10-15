// reportfiles.go names the files a driver leaves in a run's drive-reports directory: its stop reports and the stop envelopes it saves.

package sheddriver

import (
	"fmt"
	"time"
)

// reportTimestampLayout is the compact timestamp layout in a drive-reports file name.
// It carries no separators, so a directory listing of past files sorts chronologically by name.
const reportTimestampLayout = "20060102-150405"

// ReportFileName names the stop report written at now: drive-report-<compact timestamp>-<suffix>.md.
// The suffix keeps two reports of one second apart; the caller supplies it.
func ReportFileName(now time.Time, suffix string) string {
	return fmt.Sprintf("drive-report-%s-%s.md", now.Format(reportTimestampLayout), suffix)
}

// EnvelopeFileName names the stop envelope saved at now: drive-envelope-<compact timestamp>-<suffix>.json.
func EnvelopeFileName(now time.Time, suffix string) string {
	return fmt.Sprintf("drive-envelope-%s-%s.json", now.Format(reportTimestampLayout), suffix)
}
