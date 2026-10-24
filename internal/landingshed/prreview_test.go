//go:build tier0

// prreview_test.go covers the PR-Review stub: it passes with its flag off and on, logs once only when on, and surfaces a cancelled context as an error.

package landingshed

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/shedengine"
	"github.com/Knatte18/loomyard/internal/testkit/logcapture"
)

// TestPRReview_Call stays serial: logcapture redirects the process-global logger.
func TestPRReview_Call(t *testing.T) {
	const logLine = "the PR review is enabled but not built"

	tests := []struct {
		name    string
		enabled bool
		wantLog int
	}{
		{"FlagOffLogsNothing", false, 0},
		{"FlagOnLogsOnce", true, 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			logs := logcapture.CaptureVerbose(t)

			outcome, pointer, err := NewPRReview("PR-Review", tt.enabled).Call(context.Background())

			if err != nil || outcome != shedengine.Done || !reflect.DeepEqual(pointer, shedengine.OutputPointer{}) {
				t.Errorf("Call() = %q, %+v, %v; want Done, an empty pointer, nil", outcome, pointer, err)
			}
			if got := strings.Count(logs.String(), logLine); got != tt.wantLog {
				t.Errorf("Call() logged the not-built line %d times; want %d\n%s", got, tt.wantLog, logs.String())
			}
		})
	}

	t.Run("CancelledContextIsAnError", func(t *testing.T) {
		logs := logcapture.CaptureVerbose(t)
		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		outcome, _, err := NewPRReview("PR-Review", true).Call(ctx)

		if !errors.Is(err, context.Canceled) || outcome != "" {
			t.Errorf("Call() = %q, %v; want an empty outcome and an error wrapping context.Canceled", outcome, err)
		}
		if strings.Contains(logs.String(), logLine) {
			t.Errorf("a cancelled call logged the not-built line\n%s", logs.String())
		}
	})
}
