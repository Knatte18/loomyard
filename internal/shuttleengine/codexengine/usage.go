// usage.go reads a finished Codex session's token usage from its rollout file for shuttle's session-usage capability.

package codexengine

import (
	"path/filepath"

	"github.com/Knatte18/loomyard/internal/shuttleengine"
)

var _ shuttleengine.UsageReader = (*Codex)(nil)

// SessionUsage reads the tokens of session sessionID from its rollout, the rollout-*-<session id>.jsonl file under the sessions directory of the lym Codex home.
// A missing home config, missing rollout, unreadable rollout or rollout with no total reads unknown, never an error.
// workdir is unused: Codex keys its rollouts by date and session id, not by the pane cwd.
func (c *Codex) SessionUsage(sessionID, _ string) shuttleengine.SessionUsage {
	if sessionID == "" {
		return shuttleengine.SessionUsage{}
	}
	codex, err := LoadConfig(c.dirs)
	if err != nil {
		return shuttleengine.SessionUsage{}
	}
	homes, err := resolveHomes(codex)
	if err != nil {
		return shuttleengine.SessionUsage{}
	}
	paths, err := filepath.Glob(filepath.Join(homes.lym, sessionsDirName, "*", "*", "*", "rollout-*-"+sessionID+".jsonl"))
	if err != nil || len(paths) == 0 {
		return shuttleengine.SessionUsage{}
	}
	usage, _ := rolloutTotalUsage(paths[0])
	return usage
}

// rolloutTotalUsage reads the last token_count total of the rollout at path as a session usage, and reports whether it found one.
// Input is the total's input tokens less its cached ones; Output already holds the reasoning tokens, so they are never added again.
// A total without cache-write tokens reads cache-write zero, not unknown.
// An unreadable rollout or one with no total returns the unknown reading.
func rolloutTotalUsage(path string) (shuttleengine.SessionUsage, bool) {
	events, err := readRolloutEvents(path)
	if err != nil {
		return shuttleengine.SessionUsage{}, false
	}
	for i := len(events) - 1; i >= 0; i-- {
		if events[i].kind != eventTokenCount || !events[i].hasTotal {
			continue
		}
		total := events[i].total
		usage := shuttleengine.SessionUsage{
			Known:      true,
			SplitKnown: true,
			Input:      total.Input - total.CachedIn,
			CacheRead:  total.CachedIn,
			CacheWrite: total.CacheWrite,
			Output:     total.Output,
		}
		usage.Fresh = usage.Input + usage.CacheWrite + usage.Output
		return usage, true
	}
	return shuttleengine.SessionUsage{}, false
}
