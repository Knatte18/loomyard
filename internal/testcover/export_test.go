// export_test.go exposes the unexported test output collector to the package's external tests.

package testcover

import "time"

// StampOutput writes chunks, in order, to the output collector of the test name, stamped by now.
// It returns the arrival times the collector recorded for the test's `=== RUN` line and its result line.
// A time is zero where its line never arrived.
func StampOutput(name string, now func() time.Time, chunks ...string) (started, ended time.Time) {
	out := &testOutput{name: name, now: now}
	for _, chunk := range chunks {
		out.Write([]byte(chunk))
	}
	return out.started, out.ended
}
