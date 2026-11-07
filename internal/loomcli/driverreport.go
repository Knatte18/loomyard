// driverreport.go composes the path of one loom driver attempt's drive report.
// It also holds the one production entropy source that path composes over.

package loomcli

import (
	"crypto/rand"
	"fmt"
	"path/filepath"
	"time"

	"github.com/Knatte18/loomyard/internal/lymcwd"
	"github.com/Knatte18/loomyard/internal/sheddriver"
	"github.com/Knatte18/loomyard/internal/shedrun"
)

// driverReportPath composes the path to one loom driver attempt's drive report.
// The path is shedrun.DriveReportsDir(l, runID) joined with the file name that sheddriver.ReportFileName builds.
// The file name has the form "drive-report-<compact-timestamp>-<4-hex>.md".
//
// now and rand are injected seams rather than read directly, so this composer stays pure and a test
// can drive it with no real clock and no real entropy source.
// Both halves of the suffix are load-bearing.
// The timestamp, from now(), keeps a directory listing of past attempts in chronological order.
// The random part, from rand(), keeps a same-second relaunch legal.
// `Spec.validate` rejects an output file that already exists,
// so a relaunch after a driver stopped must not name the report the first attempt wrote.
//
// The directory is durable, so the reports can be committed and survive teardown.
// The directory comes from shedrun.DriveReportsDir.
// This package never spells the run-directory layout out, since shedrun owns it.
func driverReportPath(l *lymcwd.Location, runID string, now func() time.Time, rand func() string) string {
	return filepath.Join(shedrun.DriveReportsDir(l, runID), sheddriver.ReportFileName(now(), rand()))
}

// newDriverReportRand returns the production random source for driverReportPath's rand seam.
// It is a closure that reads crypto/rand and renders four lowercase hex characters.
// It is the only place in this package that reads a real entropy source, so driverReportPath stays pure.
func newDriverReportRand() func() string {
	return func() string {
		var b [2]byte
		// A read failure is not handled here: rand.Read on crypto/rand's global reader practically never fails.
		// The suffix only avoids collisions, not guesses,
		// so a zeroed buffer printing "0000" risks only a same-second collision, which the timestamp already makes unlikely.
		_, _ = rand.Read(b[:])
		return fmt.Sprintf("%04x", b)
	}
}
