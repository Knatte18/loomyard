// usage_test.go reads rollout fixtures for the session-usage reading of a finished Codex session.

package codexengine

import (
	"path/filepath"
	"testing"

	"github.com/Knatte18/loomyard/internal/shuttleengine"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// totalCountLine returns a token_count line whose session total is the JSON object body total.
func totalCountLine(timestamp, total string) string {
	return rolloutLine(timestamp, `"type":"token_count","info":{"total_token_usage":{`+total+`},"last_token_usage":{"total_tokens":7}}`)
}

// TestSessionUsage pins the rollout reading: the last total wins, input is net of cached input, output is not added to reasoning output, an absent cache-write key reads zero, and a rollout with no total or no file reads unknown.
func TestSessionUsage(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	const sessionID = "019a-session"
	tests := []struct {
		name   string
		lines  []string
		noFile bool
		want   shuttleengine.SessionUsage
	}{
		{
			name:  "TotalWithCacheWrite",
			lines: []string{totalCountLine("2026-11-05T10:00:00Z", `"input_tokens":1000,"cached_input_tokens":800,"output_tokens":50,"cache_write_input_tokens":30`)},
			want:  shuttleengine.SessionUsage{Known: true, SplitKnown: true, Fresh: 280, Input: 200, CacheRead: 800, CacheWrite: 30, Output: 50},
		},
		{
			name:  "TotalWithoutCacheWrite",
			lines: []string{totalCountLine("2026-11-05T10:00:00Z", `"input_tokens":500,"cached_input_tokens":100,"output_tokens":20`)},
			want:  shuttleengine.SessionUsage{Known: true, SplitKnown: true, Fresh: 420, Input: 400, CacheRead: 100, Output: 20},
		},
		{
			name: "LastTotalWins",
			lines: []string{
				totalCountLine("2026-11-05T10:00:00Z", `"input_tokens":10,"cached_input_tokens":0,"output_tokens":1`),
				totalCountLine("2026-11-05T10:05:00Z", `"input_tokens":300,"cached_input_tokens":100,"output_tokens":9`),
			},
			want: shuttleengine.SessionUsage{Known: true, SplitKnown: true, Fresh: 209, Input: 200, CacheRead: 100, Output: 9},
		},
		{
			name:  "ReasoningOutputIsNotAddedAgain",
			lines: []string{totalCountLine("2026-11-05T10:00:00Z", `"input_tokens":100,"cached_input_tokens":0,"output_tokens":40,"reasoning_output_tokens":25`)},
			want:  shuttleengine.SessionUsage{Known: true, SplitKnown: true, Fresh: 140, Input: 100, Output: 40},
		},
		{
			name: "NoTotal",
			lines: []string{
				rolloutLine("2026-11-05T10:00:00Z", `"type":"task_started","turn_id":"t1"`),
				rolloutLine("2026-11-05T10:00:01Z", `"type":"token_count","info":{"last_token_usage":{"total_tokens":5}}`),
			},
		},
		{name: "NoRolloutFile", noFile: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			repo := newRepoFixture(t)
			anchor, _ := repo.addWorktree(t, "wt")
			dayDir := filepath.Join(repo.lymHome, sessionsDirName, "2026", "11", "05")
			mkdirAll(t, dayDir)
			// A rollout of another session sits beside it and must never be read.
			writeFile(t, filepath.Join(dayDir, "rollout-2026-11-05T09-00-00-other.jsonl"), totalCountLine("2026-11-05T09:00:00Z", `"input_tokens":1,"output_tokens":1`)+"\n")
			if !tt.noFile {
				content := ""
				for _, line := range tt.lines {
					content += line + "\n"
				}
				writeFile(t, filepath.Join(dayDir, "rollout-2026-11-05T10-00-00-"+sessionID+".jsonl"), content)
			}

			engine, _ := newEngine(anchor)
			if got := engine.SessionUsage(sessionID, anchor); got != tt.want {
				t.Errorf("SessionUsage(%q) = %+v; want %+v", sessionID, got, tt.want)
			}
		})
	}
}
