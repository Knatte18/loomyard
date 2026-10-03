package logcapture

import (
	"strings"
	"sync"
	"testing"

	"github.com/Knatte18/loomyard/internal/logger"
)

func TestCapture_CapturesWarnButNotInfo(t *testing.T) {
	buf := Capture(t)
	logger.Info("quiet-info")
	logger.Warn("loud-warn")

	got := buf.String()
	if !strings.Contains(got, "loud-warn") {
		t.Errorf("captured %q, want it to contain the warning", got)
	}
	if strings.Contains(got, "quiet-info") {
		t.Errorf("captured %q, want the info line filtered at default verbosity", got)
	}
}

func TestCaptureVerbose_CapturesInfo(t *testing.T) {
	buf := CaptureVerbose(t)
	logger.Info("verbose-info")

	if got := buf.String(); !strings.Contains(got, "verbose-info") {
		t.Errorf("captured %q, want it to contain the info line", got)
	}
}

func TestCapture_RestoresOutputAfterTest(t *testing.T) {
	var buf *Buffer
	t.Run("inner", func(t *testing.T) {
		buf = CaptureVerbose(t)
		logger.Info("inside")
	})
	logger.Warn("after-restore")

	got := buf.String()
	if !strings.Contains(got, "inside") {
		t.Errorf("captured %q, want the line logged inside the test", got)
	}
	if strings.Contains(got, "after-restore") {
		t.Errorf("captured %q, want output restored once the test ended", got)
	}
}

func TestCapture_ConcurrentWritesAndReads(t *testing.T) {
	buf := Capture(t)
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(2)
		go func() {
			defer wg.Done()
			for j := 0; j < 50; j++ {
				logger.Warn("concurrent")
			}
		}()
		go func() {
			defer wg.Done()
			for j := 0; j < 50; j++ {
				_ = buf.String()
			}
		}()
	}
	wg.Wait()

	if got := strings.Count(buf.String(), "concurrent"); got != 8*50 {
		t.Errorf("captured %d lines, want %d", got, 8*50)
	}
}
