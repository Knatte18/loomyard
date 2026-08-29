// logcapture_test.go provides captureLogOutput, a shared test helper that redirects
// internal/logger's Info output into a buffer this package's tests can assert on.
// It must call both logger.SetOutput and logger.SetVerbosity: internal/logger's stderr half
// defaults to the Warn threshold, so SetOutput alone captures nothing at Info, and the durable
// sink half is disabled outright under testing.Testing() unless LYX_TRACE=1 is set, so neither half
// alone is a usable capture seam. os.Stderr is the restore target because internal/logger exports
// no getter for its current writer, and os.Stderr is that package's own declared default.

package reedengine

import (
	"bytes"
	"os"
	"testing"

	"github.com/Knatte18/loomyard/internal/logger"
)

// captureLogOutput redirects internal/logger's Info-and-above output into a buffer for the
// duration of t, restoring the package's stderr defaults via t.Cleanup.
func captureLogOutput(t *testing.T) *bytes.Buffer {
	t.Helper()
	var buf bytes.Buffer
	logger.SetOutput(&buf)
	logger.SetVerbosity(1)
	t.Cleanup(func() {
		logger.SetVerbosity(0)
		logger.SetOutput(os.Stderr)
	})
	return &buf
}
