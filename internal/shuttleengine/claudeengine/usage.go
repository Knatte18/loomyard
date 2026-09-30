// usage.go implements shuttleengine.SessionCycler.ContextTokens for Claude: the context a session holds is read from its own transcript, which a Stop payload names through transcript_path.
// A /clear starts a new session ID and transcript file and every later Stop payload names the new file,
// so reading the path from the turn end being evaluated follows the session through a clear with no state.
// The reader degrades to "usage unknown" on every failure and never errors, since the transcript format is a Claude Code internal.
// All payload and transcript shape knowledge stays in this file, per the Shuttle Provider-Seam Invariant.
package claudeengine

import (
	"encoding/json"
	"os"
	"strings"

	"github.com/Knatte18/loomyard/internal/shuttleengine"
)

// ContextTokens reads the context tokens the transcript named by turnEnd's transcript_path holds.
// It returns (0, false) for a missing transcript_path, an unreadable file, or a transcript with no usable main-chain assistant entry.
func (c *Claude) ContextTokens(turnEnd shuttleengine.Event) (int, bool) {
	var payload struct {
		TranscriptPath string `json:"transcript_path"`
	}
	if err := json.Unmarshal(turnEnd.Raw, &payload); err != nil || payload.TranscriptPath == "" {
		return 0, false
	}
	data, err := os.ReadFile(payload.TranscriptPath)
	if err != nil {
		return 0, false
	}
	return transcriptContextTokens(data)
}

// transcriptContextTokens returns input + cache-creation + cache-read tokens of the last main-chain assistant entry in the JSONL transcript data.
// Sidechain entries are ignored, as are entries whose three fields sum to zero (a synthetic entry for an API error or interrupt says nothing about the context),
// and malformed lines are skipped.
func transcriptContextTokens(data []byte) (int, bool) {
	type usage struct {
		Input         int `json:"input_tokens"`
		CacheCreation int `json:"cache_creation_input_tokens"`
		CacheRead     int `json:"cache_read_input_tokens"`
	}
	type entry struct {
		Type        string `json:"type"`
		IsSidechain bool   `json:"isSidechain"`
		Message     struct {
			Usage *usage `json:"usage"`
		} `json:"message"`
	}

	tokens, known := 0, false
	for _, line := range strings.Split(string(data), "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}
		var e entry
		if err := json.Unmarshal([]byte(trimmed), &e); err != nil {
			continue
		}
		if e.Type != "assistant" || e.IsSidechain || e.Message.Usage == nil {
			continue
		}
		sum := e.Message.Usage.Input + e.Message.Usage.CacheCreation + e.Message.Usage.CacheRead
		if sum == 0 {
			continue
		}
		tokens, known = sum, true
	}
	return tokens, known
}
