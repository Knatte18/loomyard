// audit_spawnboundary_test.go covers the fork-transcript spawn-boundary
// exclusion: on Claude Code 2.1.205 a fork transcript's first assistant line
// replays the PARENT's own spawning Agent tool_use (observed live in webster's
// hardening round fable-r1), and counting it flagged every legitimate fork as
// a nested-Agent violation. The audit excludes that entry by the tool_use id
// recorded in the sibling <transcript>.meta.json; these tests pin both the
// exclusion and the documented no-meta fallback (skip nothing).

package claudeengine

import (
	"os"
	"path/filepath"
	"testing"
)

// writeSpawnBoundaryFixture copies the spawn-boundary fork transcript into dir
// and returns its path, optionally writing the sibling .meta.json carrying the
// spawning call's toolUseId.
func writeSpawnBoundaryFixture(t *testing.T, withMeta bool) string {
	t.Helper()
	dir := t.TempDir()
	transcript := filepath.Join(dir, "agent-a93e40b96f277c37a.jsonl")
	copyFixture(t, "fork-spawn-boundary.jsonl", transcript)
	if withMeta {
		meta := `{"agentType":"fork","isFork":true,"toolUseId":"toolu_01SpawnBoundary","spawnDepth":1}`
		if err := os.WriteFile(filepath.Join(dir, "agent-a93e40b96f277c37a.meta.json"), []byte(meta), 0o644); err != nil {
			t.Fatalf("write meta.json: %v", err)
		}
	}
	return transcript
}

func TestAuditForkTranscript_SpawnBoundaryAgentCallExcluded(t *testing.T) {
	transcript := writeSpawnBoundaryFixture(t, true)

	report, err := auditForkTranscript(transcript)
	if err != nil {
		t.Fatalf("auditForkTranscript() error: %v", err)
	}

	// The only Agent tool_use in the transcript is the parent's own spawning
	// call — the fork itself never called Agent, so the audit must report zero.
	if report.AgentCalls != 0 {
		t.Errorf("AgentCalls = %d; want 0 (the spawn-boundary Agent call is the parent's, not the fork's)", report.AgentCalls)
	}
	if got := report.ToolCalls["Agent"]; got != 0 {
		t.Errorf(`ToolCalls["Agent"] = %d; want 0`, got)
	}

	// The fork's own genuine activity must still be fully counted.
	if got := report.ToolCalls["Read"]; got != 1 {
		t.Errorf(`ToolCalls["Read"] = %d; want 1`, got)
	}
	if len(report.BashCommands) != 1 {
		t.Errorf("len(BashCommands) = %d; want 1", len(report.BashCommands))
	}
	if !report.ReportReturned {
		t.Error("ReportReturned = false; want true (final assistant message carried text)")
	}
}

func TestAuditForkTranscript_MissingMetaFallsBackToCountingEverything(t *testing.T) {
	transcript := writeSpawnBoundaryFixture(t, false)

	report, err := auditForkTranscript(transcript)
	if err != nil {
		t.Fatalf("auditForkTranscript() error: %v", err)
	}

	// Without the sibling meta.json there is no spawn id to exclude by, so the
	// documented fallback is the pre-meta behavior: count everything. This
	// pins that the exclusion is meta-keyed, never a blind first-line skip.
	if report.AgentCalls != 1 {
		t.Errorf("AgentCalls = %d; want 1 (no meta.json means nothing is excluded)", report.AgentCalls)
	}
}
