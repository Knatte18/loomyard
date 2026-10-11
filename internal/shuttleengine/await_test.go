package shuttleengine

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// awaitClock is a fake clock whose Sleep advances the time, records each length and runs onSleep with the 1-based call number.
type awaitClock struct {
	*fakeClock
	sleeps  []time.Duration
	onSleep func(call int)
}

func (c *awaitClock) Sleep(d time.Duration) {
	c.fakeClock.Sleep(d)
	c.sleeps = append(c.sleeps, d)
	if c.onSleep != nil {
		c.onSleep(len(c.sleeps))
	}
}

// TestAwaitAny pins the capped poll over a fake clock: which path it names, when it answers not ready, what it returns on a stat error or a cancelled context, and that no sleep passes one second or the remaining cap.
func TestAwaitAny(t *testing.T) {
	t.Parallel()

	cancelled, cancel := context.WithCancel(context.Background())
	cancel()

	tests := []struct {
		name string
		ctx  context.Context
		// paths are names under the temp directory, except a "blocker/" prefix, which sits under a regular file.
		paths []string
		cap   time.Duration
		// seed names the files that exist before the poll.
		seed []string
		// appear maps a Sleep call number to the file created by it.
		appear map[int]string
		// wantPath is the name the poll answers, empty for not ready or an error.
		wantPath  string
		wantReady bool
		wantErr   bool
		// wantSleeps are the lengths Sleep is called with.
		wantSleeps []time.Duration
	}{
		{
			name:      "existing path answers at once",
			paths:     []string{"a"},
			cap:       10 * time.Second,
			seed:      []string{"a"},
			wantPath:  "a",
			wantReady: true,
		},
		{
			name:       "the path that appears first is named",
			paths:      []string{"a", "b"},
			cap:        10 * time.Second,
			appear:     map[int]string{2: "b"},
			wantPath:   "b",
			wantReady:  true,
			wantSleeps: []time.Duration{time.Second, time.Second},
		},
		{
			name:       "cap elapsing answers not ready and the last sleep is the remaining cap",
			paths:      []string{"a"},
			cap:        2500 * time.Millisecond,
			wantSleeps: []time.Duration{time.Second, time.Second, 500 * time.Millisecond},
		},
		{
			name:    "a stat error other than not-exist is returned",
			paths:   []string{"blocker/a"},
			cap:     10 * time.Second,
			seed:    []string{"blocker"},
			wantErr: true,
		},
		{
			name:    "a cancelled context is returned",
			ctx:     cancelled,
			paths:   []string{"a"},
			cap:     10 * time.Second,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			dir := t.TempDir()
			for _, name := range tt.seed {
				if err := os.WriteFile(filepath.Join(dir, name), nil, 0o644); err != nil {
					t.Fatalf("seed %q: %v", name, err)
				}
			}
			paths := make([]string, len(tt.paths))
			for i, name := range tt.paths {
				paths[i] = filepath.Join(dir, name)
			}
			clock := &awaitClock{fakeClock: newFakeClock(time.Unix(0, 0))}
			clock.onSleep = func(call int) {
				if name, ok := tt.appear[call]; ok {
					if err := os.WriteFile(filepath.Join(dir, name), nil, 0o644); err != nil {
						t.Errorf("create %q: %v", name, err)
					}
				}
			}
			ctx := tt.ctx
			if ctx == nil {
				ctx = context.Background()
			}

			got, ready, err := awaitAny(ctx, clock, paths, tt.cap)

			if (err != nil) != tt.wantErr {
				t.Fatalf("awaitAny() error = %v; want error %v", err, tt.wantErr)
			}
			wantPath := ""
			if tt.wantPath != "" {
				wantPath = filepath.Join(dir, tt.wantPath)
			}
			if got != wantPath || ready != tt.wantReady {
				t.Errorf("awaitAny() = (%q, %v); want (%q, %v)", got, ready, wantPath, tt.wantReady)
			}
			if len(clock.sleeps) != len(tt.wantSleeps) {
				t.Fatalf("sleeps = %v; want %v", clock.sleeps, tt.wantSleeps)
			}
			for i, want := range tt.wantSleeps {
				if clock.sleeps[i] != want {
					t.Errorf("sleeps = %v; want %v", clock.sleeps, tt.wantSleeps)
					break
				}
			}
		})
	}
}
