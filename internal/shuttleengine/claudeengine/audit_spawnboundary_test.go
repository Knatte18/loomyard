// audit_spawnboundary_test.go covers what the audit reads from a subagent transcript's sibling <transcript>.meta.json.
// On Claude Code 2.1.205, a fork transcript's first assistant line replays the parent's own spawning Agent tool_use.
// Counting that entry would flag every legitimate fork as a nested-Agent violation,
// so the audit excludes it by the tool_use id that the meta file records.
// These tests pin that exclusion and the fallback for a transcript with no meta file, which skips nothing.

package claudeengine

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// forkMeta is the meta file Claude Code writes beside a fork's transcript.
const forkMeta = `{"agentType":"fork","isFork":true,"toolUseId":"toolu_01SpawnBoundary","spawnDepth":1}`

// writeSpawnBoundaryFixture copies the spawn-boundary fork transcript into a new temporary directory.
// It returns the transcript path.
// When meta is not empty, it also writes meta as the sibling .meta.json.
func writeSpawnBoundaryFixture(t *testing.T, meta string) string {
	t.Helper()
	dir := t.TempDir()
	transcript := filepath.Join(dir, "agent-a93e40b96f277c37a.jsonl")
	copyFixture(t, "fork-spawn-boundary.jsonl", transcript)
	if meta != "" {
		if err := os.WriteFile(filepath.Join(dir, "agent-a93e40b96f277c37a.meta.json"), []byte(meta), 0o644); err != nil {
			t.Fatalf("write meta.json: %v", err)
		}
	}
	return transcript
}

//testtiming:keep pins the spawn-boundary Agent call's exclusion and the fork's own tool counts and report
func TestAuditForkTranscript_SpawnBoundaryAgentCallExcluded(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()
	transcript := writeSpawnBoundaryFixture(t, forkMeta)

	report, err := auditForkTranscript(transcript)
	if err != nil {
		t.Fatalf("auditForkTranscript() error: %v", err)
	}

	// The only Agent tool_use in the transcript is the parent's own spawning call.
	// The fork never called Agent itself, so the audit must report zero.
	if report.AgentCalls != 0 {
		t.Errorf("AgentCalls = %d; want 0 (the spawn-boundary Agent call is the parent's, not the fork's)", report.AgentCalls)
	}
	if got := report.ToolCalls["Agent"]; got != 0 {
		t.Errorf(`ToolCalls["Agent"] = %d; want 0`, got)
	}

	// The fork's own activity must still be counted.
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

//testtiming:keep pins that a missing meta file excludes no Agent call
func TestAuditForkTranscript_MissingMetaFallsBackToCountingEverything(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()
	transcript := writeSpawnBoundaryFixture(t, "")

	report, err := auditForkTranscript(transcript)
	if err != nil {
		t.Fatalf("auditForkTranscript() error: %v", err)
	}

	// Without the sibling meta.json there is no spawn id to exclude by, so every Agent call counts.
	// This pins that the exclusion is keyed by the spawn id in the meta file, not a blind skip of the first line.
	if report.AgentCalls != 1 {
		t.Errorf("AgentCalls = %d; want 1 (no meta.json means nothing is excluded)", report.AgentCalls)
	}
}
