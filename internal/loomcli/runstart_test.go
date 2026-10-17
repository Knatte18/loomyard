// runstart_test.go covers seedRunStart's three outcomes over a hand-built location: a parsed stamp, a zero time after a warning, and a read failure.

package loomcli

import (
	"os"
	"strings"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/lyxcwd"
	"github.com/Knatte18/loomyard/internal/shedrun"
)

// TestSeedRunStart pins the start a seed yields: its parsed stamp, the zero time with one warning when the seed has no usable stamp, and an error naming the seed file when it cannot be decoded.
// It never runs in parallel: the durable sink override is process-global state.
func TestSeedRunStart(t *testing.T) {
	tests := []struct {
		name        string
		seed        string
		writeSeed   bool
		want        time.Time
		wantWarning bool
		wantErr     bool
	}{
		{"ParseableStamp", `{"recipe":"loom","driver":"go","started_at":"2026-10-01T08:00:00Z"}`, true, time.Date(2026, 10, 1, 8, 0, 0, 0, time.UTC), false, false},
		{"NoSeedFile", "", false, time.Time{}, true, false},
		{"NoStartedAtKey", `{"recipe":"loom","driver":"go"}`, true, time.Time{}, true, false},
		{"UnparseableStamp", `{"recipe":"loom","driver":"go","started_at":"yesterday"}`, true, time.Time{}, true, false},
		{"UndecodableSeed", `{not json`, true, time.Time{}, false, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			logger.SetDurableSinkDir(t.TempDir())
			t.Cleanup(func() { logger.SetDurableSinkDir("") })

			l := &lyxcwd.Location{HubPath: t.TempDir(), WorktreeName: "pair", AnchorRel: "."}
			if err := os.MkdirAll(shedrun.RunDir(l, shedrun.SelfRunID), 0o755); err != nil {
				t.Fatalf("create run directory: %v", err)
			}
			if tt.writeSeed {
				if err := os.WriteFile(shedrun.SeedFile(l, shedrun.SelfRunID), []byte(tt.seed), 0o644); err != nil {
					t.Fatalf("write seed: %v", err)
				}
			}

			got, err := seedRunStart(l)
			if (err != nil) != tt.wantErr {
				t.Fatalf("seedRunStart() error = %v; wantErr %v", err, tt.wantErr)
			}
			if tt.wantErr && !strings.Contains(err.Error(), shedrun.SeedFile(l, shedrun.SelfRunID)) {
				t.Errorf("seedRunStart() error = %q; want it to carry the seed file path", err)
			}
			if !got.Equal(tt.want) {
				t.Errorf("seedRunStart() = %v; want %v", got, tt.want)
			}

			warnings := 0
			if data, readErr := os.ReadFile(logger.TraceFile()); readErr == nil {
				for _, line := range strings.Split(string(data), "\n") {
					if strings.Contains(line, "carries no usable start stamp") {
						warnings++
					}
				}
			}
			wantWarnings := 0
			if tt.wantWarning {
				wantWarnings = 1
			}
			if warnings != wantWarnings {
				t.Errorf("start stamp warnings = %d; want %d", warnings, wantWarnings)
			}
		})
	}
}
