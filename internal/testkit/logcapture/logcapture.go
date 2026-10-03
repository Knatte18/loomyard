// Package logcapture redirects internal/logger output into a buffer for one test and restores it afterwards.
//
// The kit imports only internal/logger,
// so it cannot serve logger's own tests or the packages logger depends on.
package logcapture

import (
	"os"
	"strings"
	"sync"
	"testing"

	"github.com/Knatte18/loomyard/internal/logger"
)

// Buffer collects logger output; writes and String calls are safe from concurrent goroutines.
type Buffer struct {
	mu sync.Mutex
	sb strings.Builder
}

// Write appends p to the buffer.
func (b *Buffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.sb.Write(p)
}

// String returns everything captured so far.
func (b *Buffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.sb.String()
}

// Capture points logger output at a fresh Buffer and restores os.Stderr and the default verbosity in t.Cleanup.
func Capture(t testing.TB) *Buffer {
	t.Helper()
	buf := &Buffer{}
	logger.SetOutput(buf)
	t.Cleanup(func() {
		logger.SetOutput(os.Stderr)
		logger.SetVerbosity(0)
	})
	return buf
}

// CaptureVerbose is Capture with verbosity raised to Info for the duration.
func CaptureVerbose(t testing.TB) *Buffer {
	t.Helper()
	buf := Capture(t)
	logger.SetVerbosity(1)
	return buf
}
