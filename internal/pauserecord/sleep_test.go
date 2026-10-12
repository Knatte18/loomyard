// sleep_test.go pins sleep detection and the awake-time measure over sleeps and pauses.

package pauserecord

import (
	"testing"
	"time"
)

func TestSleepWatch(t *testing.T) {
	t.Parallel()

	type reading struct {
		wall  time.Duration
		awake time.Duration
	}
	tests := []struct {
		name     string
		readings []reading
		want     []Interval
	}{
		{"first reading records nothing", []reading{{0, 0}}, nil},
		{
			"hours of wall against seconds awake",
			[]reading{{0, 0}, {3 * time.Hour, 5 * time.Second}},
			[]Interval{{epoch.Add(5 * time.Second), epoch.Add(3 * time.Hour)}},
		},
		{"excess at the tolerance is no sleep", []reading{{0, 0}, {time.Hour, time.Hour - SleepTolerance}}, nil},
		{
			"excess just over the tolerance counts",
			[]reading{{0, 0}, {time.Hour, time.Hour - SleepTolerance - time.Second}},
			[]Interval{{epoch.Add(time.Hour - SleepTolerance - time.Second), epoch.Add(time.Hour)}},
		},
		{"equal deltas are no sleep", []reading{{time.Hour, 10 * time.Second}, {2 * time.Hour, time.Hour + 10*time.Second}}, nil},
		{"wall behind awake is no sleep", []reading{{0, 0}, {time.Minute, time.Hour}}, nil},
		{
			"a sleep older than retention is dropped",
			[]reading{{0, 0}, {time.Hour, 0}, {Retention + 2*time.Hour, Retention + time.Hour}},
			nil,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			var watch SleepWatch
			for _, r := range tt.readings {
				watch.Observe(epoch.Add(r.wall), r.awake)
			}
			got := watch.Sleeps()
			if len(got) != len(tt.want) {
				t.Fatalf("sleeps = %+v, want %+v", got, tt.want)
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Fatalf("sleeps = %+v, want %+v", got, tt.want)
				}
			}
		})
	}
}

func TestAwakeSince(t *testing.T) {
	t.Parallel()
	at := func(minutes int) time.Time { return epoch.Add(time.Duration(minutes) * time.Minute) }
	sleep := []Interval{{at(30), at(50)}}

	tests := []struct {
		name     string
		activity time.Time
		excluded [][]Interval
		want     time.Duration
	}{
		{"activity before a sleep subtracts the whole sleep", at(0), [][]Interval{sleep}, 40 * time.Minute},
		{"activity after a sleep subtracts none", at(55), [][]Interval{sleep}, 5 * time.Minute},
		{
			"a pause overlapping a sleep subtracts the overlap once",
			at(0),
			[][]Interval{sleep, {{at(40), at(60)}}},
			30 * time.Minute,
		},
		{"no intervals leaves the whole span", at(0), nil, time.Hour},
		{"exclusions longer than the span floor at zero", at(40), [][]Interval{{{at(0), at(60)}}}, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := AwakeSince(tt.activity, at(60), tt.excluded...); got != tt.want {
				t.Fatalf("AwakeSince = %v, want %v", got, tt.want)
			}
		})
	}
}
