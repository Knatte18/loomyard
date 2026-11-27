// audit_nested_test.go covers the inherited history a fork's transcript replays from the session or a subagent.
// A later fork of several carries the spawn calls of the forks before it.
// These tests pin that the audit counts only the fork's own calls and names the subagent that spawned it.

package claudeengine

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// nestedToolUse renders one assistant transcript line holding a single tool_use block.
func nestedToolUse(id, name, input string) string {
	return fmt.Sprintf(`{"type":"assistant","message":{"role":"assistant","content":[{"type":"tool_use","id":%q,"name":%q,"input":%s}]}}`, id, name, input)
}

// nestedReport renders one assistant transcript line holding a final text report.
const nestedReport = `{"type":"assistant","message":{"role":"assistant","content":[{"type":"text","text":"Report."}]}}`

// The tool calls of the general-purpose subagent that spawns two forks in turn.
var (
	spawnerMainSpawn = nestedToolUse("toolu_MainSpawn", "Agent", `{"subagent_type":"general-purpose","prompt":"p"}`)
	spawnerBash      = nestedToolUse("toolu_SpawnerBash", "Bash", `{"command":"git commit -am spawner"}`)
	spawnerFirstFork = nestedToolUse("toolu_FirstFork", "Agent", `{"subagent_type":"fork","prompt":"first"}`)
	spawnerLastFork  = nestedToolUse("toolu_LastFork", "Agent", `{"subagent_type":"fork","prompt":"last"}`)
)

// The session's own calls that its directly spawned fork replays.
var (
	sessionBash       = nestedToolUse("toolu_SessionBash", "Bash", `{"command":"git commit -am session"}`)
	sessionOtherSpawn = nestedToolUse("toolu_SessionOtherSpawn", "Agent", `{"subagent_type":"fork","prompt":"other"}`)
)

// writeNestedLayout writes a session that spawned a general-purpose subagent, agent-gp, and a fork, agent-direct.
// agent-gp spawned forks agent-first and agent-last in turn.
// Each fork's transcript opens with the history it inherits from its spawner, then holds its own calls.
// agent-last's own calls include an Agent call of its own.
// It sets HOME and returns the session id and workdir.
func writeNestedLayout(t *testing.T) (sessionID, workdir string) {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	workdir = "/home/op/nested"
	sessionID = "sess-nested"
	projectDir := filepath.Join(home, ".claude", "projects", encodeForTest(workdir))
	subagents := filepath.Join(projectDir, sessionID, "subagents")
	files := map[string]string{
		filepath.Join(projectDir, sessionID+".jsonl"): strings.Join([]string{spawnerMainSpawn, sessionBash, sessionOtherSpawn}, "\n"),
		filepath.Join(subagents, "agent-direct.jsonl"): strings.Join([]string{
			sessionBash, sessionOtherSpawn,
			nestedToolUse("toolu_DirectRead", "Read", `{"file_path":"/c"}`), nestedReport,
		}, "\n"),
		filepath.Join(subagents, "agent-direct.meta.json"): `{"agentType":"fork","isFork":true,"toolUseId":"toolu_DirectFork","spawnDepth":1}`,
		filepath.Join(subagents, "agent-gp.jsonl"):         strings.Join([]string{spawnerBash, spawnerFirstFork, spawnerLastFork, nestedReport}, "\n"),
		filepath.Join(subagents, "agent-gp.meta.json"):     `{"agentType":"general-purpose","toolUseId":"toolu_MainSpawn","spawnDepth":1}`,
		filepath.Join(subagents, "agent-first.jsonl"): strings.Join([]string{
			spawnerBash, spawnerFirstFork,
			nestedToolUse("toolu_FirstRead", "Read", `{"file_path":"/a"}`), nestedReport,
		}, "\n"),
		filepath.Join(subagents, "agent-first.meta.json"): `{"agentType":"fork","isFork":true,"toolUseId":"toolu_FirstFork","parentAgentId":"gp","spawnDepth":2}`,
		filepath.Join(subagents, "agent-last.jsonl"): strings.Join([]string{
			spawnerBash, spawnerFirstFork, spawnerLastFork,
			nestedToolUse("toolu_LastRead", "Read", `{"file_path":"/b"}`),
			nestedToolUse("toolu_LastOwnSpawn", "Agent", `{"subagent_type":"fork","prompt":"own"}`), nestedReport,
		}, "\n"),
		filepath.Join(subagents, "agent-last.meta.json"): `{"agentType":"fork","isFork":true,"toolUseId":"toolu_LastFork","parentAgentId":"gp","spawnDepth":2}`,
	}
	for path, content := range files {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatalf("mkdir %q: %v", filepath.Dir(path), err)
		}
		if err := os.WriteFile(path, []byte(content+"\n"), 0o644); err != nil {
			t.Fatalf("write %q: %v", path, err)
		}
	}
	return sessionID, workdir
}

// TestAuditForks_SubagentForkCountsOnlyItsOwnCalls checks each subagent's report.
// A fork's inherited spawn calls and Bash command are its spawner's, so only its own calls count.
// It runs sequentially because it sets HOME.
func TestAuditForks_SubagentForkCountsOnlyItsOwnCalls(t *testing.T) {
	tiergate.Require(t, 0)
	sessionID, workdir := writeNestedLayout(t)

	audit, err := New().AuditForks(sessionID, workdir)
	if err != nil {
		t.Fatalf("AuditForks() error: %v", err)
	}
	type want struct {
		agentCalls   int
		reads        int
		bashCommands int
		// spawner is the file name of the spawning subagent's transcript, "" for the session.
		spawner string
	}
	wants := map[string]want{
		"agent-direct.jsonl": {agentCalls: 0, reads: 1, bashCommands: 0},
		"agent-gp.jsonl":     {agentCalls: 2, reads: 0, bashCommands: 1},
		"agent-first.jsonl":  {agentCalls: 0, reads: 1, bashCommands: 0, spawner: "agent-gp.jsonl"},
		"agent-last.jsonl":   {agentCalls: 1, reads: 1, bashCommands: 0, spawner: "agent-gp.jsonl"},
	}
	if len(audit.Forks) != len(wants) {
		t.Fatalf("len(Forks) = %d; want %d", len(audit.Forks), len(wants))
	}
	for _, report := range audit.Forks {
		name := filepath.Base(report.TranscriptPath)
		w, ok := wants[name]
		if !ok {
			t.Errorf("unexpected fork transcript %q", name)
			continue
		}
		if report.AgentCalls != w.agentCalls {
			t.Errorf("%s AgentCalls = %d; want %d", name, report.AgentCalls, w.agentCalls)
		}
		if got := report.ToolCalls["Read"]; got != w.reads {
			t.Errorf(`%s ToolCalls["Read"] = %d; want %d`, name, got, w.reads)
		}
		if len(report.BashCommands) != w.bashCommands {
			t.Errorf("%s BashCommands = %v; want %d command(s)", name, report.BashCommands, w.bashCommands)
		}
		wantSpawner := ""
		if w.spawner != "" {
			wantSpawner = filepath.Join(filepath.Dir(report.TranscriptPath), w.spawner)
		}
		if report.SpawnerTranscriptPath != wantSpawner {
			t.Errorf("%s SpawnerTranscriptPath = %q; want %q", name, report.SpawnerTranscriptPath, wantSpawner)
		}
		if !report.ReportReturned {
			t.Errorf("%s ReportReturned = false; want true", name)
		}
	}
}

// TestAuditForks_MissingSpawningSubagentTranscriptErrors checks that an absent spawner transcript fails the audit.
// Without the spawning subagent's transcript, a fork's inherited history cannot be told from its own calls.
// It runs sequentially because it sets HOME.
func TestAuditForks_MissingSpawningSubagentTranscriptErrors(t *testing.T) {
	tiergate.Require(t, 0)
	sessionID, workdir := writeNestedLayout(t)
	projectDir := filepath.Join(os.Getenv("HOME"), ".claude", "projects", encodeForTest(workdir))
	if err := os.Remove(filepath.Join(projectDir, sessionID, "subagents", "agent-gp.jsonl")); err != nil {
		t.Fatalf("remove spawning transcript: %v", err)
	}

	_, err := New().AuditForks(sessionID, workdir)
	if err == nil || !strings.Contains(err.Error(), "agent-gp.jsonl") {
		t.Errorf("AuditForks() error = %v; want an error naming agent-gp.jsonl", err)
	}
	if errors.Is(err, fs.ErrNotExist) {
		t.Errorf("errors.Is(%v, fs.ErrNotExist) = true; want false", err)
	}
}
