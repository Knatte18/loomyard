// driverreport.go implements the per-attempt drive report path: a pure composer over injected clock
// and random seams, and the one production entropy source it composes over. It lives beside
// bootstrap.go per the pure-decisions-live-in-bootstrap-go Shared Decision.

package loomcli

import (
	"crypto/rand"
	"fmt"
	"path/filepath"
	"time"

	"github.com/Knatte18/loomyard/internal/lyxcwd"
	"github.com/Knatte18/loomyard/internal/shedrun"
)

// driverReportTimestampLayout is the compact timestamp format driverReportPath interpolates: a
// directory listing of past attempts sorts correctly by name under this layout, since it carries no
// separators that would sort out of chronological order.
const driverReportTimestampLayout = "20060102-150405"

// driverReportPath composes the path to one ly-drive attempt's drive report, under the run's
// ephemeral scratch directory: shedrun.ScratchDir(l, runID) joined with
// "drive-report-<compact-timestamp>-<4-hex>.md".
//
// now and rand are injected seams rather than read directly, so this composer stays pure and a test
// can drive it with no real clock and no real entropy source. Both halves of the suffix are
// load-bearing. The timestamp, from now(), is what keeps a directory listing of past attempts
// readable in chronological order. The random component, from rand(), is what keeps a same-second
// relaunch legal: Spec.validate rejects an output file that already exists, so a second bootstrap
// after a driver stopped must not name the report the first one wrote -- and a same-second relaunch
// is not hypothetical, since corpse removal plus a fresh start inside one second is exactly what
// batch 7's instantly-exiting stub pane produces.
//
// The directory is composed through shedrun.ScratchDir and never by spelling ".lyx" or "shed" in
// this package: the Lyxdirs Single-Declarer Invariant and the Shed Run-Directory Invariant both make
// that a shedrun obligation, and ScratchDir is the shipped accessor for exactly this placement.
func driverReportPath(l *lyxcwd.Location, runID string, now func() time.Time, rand func() string) string {
	filename := fmt.Sprintf("drive-report-%s-%s.md", now().Format(driverReportTimestampLayout), rand())
	return filepath.Join(shedrun.ScratchDir(l, runID), filename)
}

// newDriverReportRand returns the production random source driverReportPath's rand seam consumes: a
// closure reading crypto/rand and rendering four lowercase hex characters. It is the only place in
// this package that reads a real entropy source, so driverReportPath itself stays pure.
func newDriverReportRand() func() string {
	return func() string {
		var b [2]byte
		// A read failure here is not treated specially: rand.Read against crypto/rand's global
		// reader practically never fails, and this suffix's job is collision-avoidance, not
		// cryptographic security, so a zeroed buffer degrading to "0000" costs nothing worse than a
		// same-second collision the timestamp component already makes unlikely.
		_, _ = rand.Read(b[:])
		return fmt.Sprintf("%04x", b)
	}
}
