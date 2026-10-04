// spawn_test.go covers primeResolveLogLevel, the pure decision behind the prime-name failure log level.

package ideengine

import (
	"errors"
	"log/slog"
	"testing"
)

func TestPrimeResolveLogLevel(t *testing.T) {
	boom := errors.New("boom")
	tests := []struct {
		name      string
		target    spawnTarget
		err       error
		wantLevel slog.Level
		wantLog   bool
	}{
		{"prime with error warns", targetPrime, boom, slog.LevelWarn, true},
		{"task with error is debug", targetTask, boom, slog.LevelDebug, true},
		{"unknown with error warns", targetUnknown, boom, slog.LevelWarn, true},
		{"prime without error logs nothing", targetPrime, nil, 0, false},
		{"task without error logs nothing", targetTask, nil, 0, false},
		{"unknown without error logs nothing", targetUnknown, nil, 0, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			level, ok := primeResolveLogLevel(tt.target, tt.err)
			if ok != tt.wantLog || (ok && level != tt.wantLevel) {
				t.Errorf("got (%v, %v), want (%v, %v)", level, ok, tt.wantLevel, tt.wantLog)
			}
		})
	}
}
