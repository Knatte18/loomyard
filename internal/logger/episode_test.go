// episode_test.go verifies Episode and EpisodeSet log Warn once per episode, Debug while it repeats and Info at recovery.
// Its test is not parallel: it captures the process-global logger output and verbosity.

package logger

import (
	"reflect"
	"strings"
	"testing"
)

// episodeLineSummaries reduces each captured line to its level, plus failed_ticks and key when present.
func episodeLineSummaries(output string) []string {
	var summaries []string
	for _, line := range strings.Split(strings.TrimSpace(output), "\n") {
		if line == "" {
			continue
		}
		var parts []string
		for _, field := range strings.Fields(line) {
			if strings.HasPrefix(field, "level=") || strings.HasPrefix(field, "failed_ticks=") || strings.HasPrefix(field, "key=") {
				parts = append(parts, strings.TrimPrefix(field, "level="))
			}
		}
		summaries = append(summaries, strings.Join(parts, " "))
	}
	return summaries
}

func TestEpisode_WarnsOncePerEpisodeAndInfoOnRecovery(t *testing.T) {
	tests := []struct {
		name string
		run  func(episode *Episode, set *EpisodeSet)
		want []string
	}{
		{
			name: "repeats log debug with the count, recovery logs info with the final count",
			run: func(episode *Episode, _ *EpisodeSet) {
				episode.Fail("failing", "key", "a")
				episode.Fail("failing", "key", "a")
				episode.Fail("failing", "key", "a")
				episode.Recover("recovered", "key", "a")
			},
			want: []string{"WARN key=a", "DEBUG key=a failed_ticks=2", "DEBUG key=a failed_ticks=3", "INFO key=a failed_ticks=3"},
		},
		{
			name: "recover without a failure logs nothing",
			run: func(episode *Episode, _ *EpisodeSet) {
				episode.Recover("recovered")
			},
			want: nil,
		},
		{
			name: "a failure after a recovery warns again",
			run: func(episode *Episode, _ *EpisodeSet) {
				episode.Fail("failing")
				episode.Recover("recovered")
				episode.Fail("failing")
			},
			want: []string{"WARN", "INFO failed_ticks=1", "WARN"},
		},
		{
			name: "set keys each warn first and recovering one leaves the other running",
			run: func(_ *Episode, set *EpisodeSet) {
				set.Fail("a", "failing", "key", "a")
				set.Fail("b", "failing", "key", "b")
				set.Fail("a", "failing", "key", "a")
				set.Recover("a", "recovered", "key", "a")
				set.Recover("a", "recovered", "key", "a")
				set.Fail("b", "failing", "key", "b")
			},
			want: []string{
				"WARN key=a",
				"WARN key=b",
				"DEBUG key=a failed_ticks=2",
				"INFO key=a failed_ticks=2",
				"DEBUG key=b failed_ticks=2",
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			buf := withCapturedOutput(t)
			SetVerbosity(2)
			t.Cleanup(func() { SetVerbosity(0) })

			tt.run(&Episode{}, &EpisodeSet{})

			if got := episodeLineSummaries(buf.String()); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("logged lines = %q, want %q", got, tt.want)
			}
		})
	}
}
