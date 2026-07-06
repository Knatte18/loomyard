//go:build smoke

// smoke_guardrail_test.go is the live proof of the deny-and-steer guardrail
// path the hooks research flagged as unprobed
// (docs/research/mux-hooks-exploration.md: "the deny-and-steer path itself
// is not yet probed"): a REAL claude, when its Agent tool call is denied by
// the PreToolUse hook, actually resumes in-session on the steered
// instruction rather than stalling or aborting the turn, and a REAL claude
// asked to pose a question surfaces it as the run loop's classified
// "asking" outcome. Follows the same conventions as smoke_run_test.go,
// whose helpers (claudeBinaryPath, deferHubRelease, muxStatusStrand) this
// file reuses.

package shuttlecli

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/lyxtest"
	"github.com/Knatte18/loomyard/internal/muxcli"
	"github.com/Knatte18/loomyard/internal/muxengine"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
)

// TestSmokeGuardrailDeniesAgentTool proves the "deny-and-steer" path this
// round closes from the hooks research's open item: the run's prompt
// explicitly instructs the agent to dispatch a subagent via its in-process
// Agent tool to write the output file, falling back to writing it directly
// only if the Agent tool turns out to be unavailable. The PreToolUse(Agent)
// hook denies the call and steers the model back into this pane
// (claudeengine's steerAgentDeny reason); the run still reaching "done"
// with the file written is the direct, live proof that the deny fired AND
// the steer redirected the work in-session, rather than the turn stalling
// or the agent giving up once its preferred tool was refused.
func TestSmokeGuardrailDeniesAgentTool(t *testing.T) {
	claudeBinaryPath(t)

	fixture := lyxtest.CopyPaired(t)
	lyxtest.SeedConfig(t, fixture.Hub, map[string]string{
		"shuttle": shuttleengine.ConfigTemplate(),
		"mux":     muxengine.ConfigTemplate(),
	})
	deferHubRelease(t, fixture.Hub)
	t.Chdir(fixture.Hub)
	t.Cleanup(func() {
		var buf bytes.Buffer
		muxcli.RunCLI(&buf, []string{"down"})
	})

	var muxOut bytes.Buffer
	if code := muxcli.RunCLI(&muxOut, []string{"up"}); code != 0 {
		t.Fatalf("mux up = %d; want 0, output: %s", code, muxOut.String())
	}

	outputPath := filepath.Join(fixture.Hub, "smoke-guardrail-agent-output.txt")
	prompt := fmt.Sprintf(
		"Dispatch a subagent via your Agent tool to write exactly DONE to %s and then stop. "+
			"Only write the file yourself, directly, if the Agent tool turns out to be unavailable to you.",
		outputPath,
	)

	var out bytes.Buffer
	code := RunCLI(&out, []string{
		"run",
		"--prompt", prompt,
		"--output-file", outputPath,
		"--timeout", "5m",
	})
	if code != 0 {
		t.Fatalf("shuttle run = %d; want 0, output: %s", code, out.String())
	}

	var result map[string]any
	if err := json.Unmarshal(out.Bytes(), &result); err != nil {
		t.Fatalf("parse run result: %v; output: %s", err, out.String())
	}
	if outcome, _ := result["outcome"].(string); outcome != "done" {
		t.Fatalf("run outcome = %q; want \"done\" (the Agent-tool deny must steer the model back into this pane, not stall the turn); output: %s", outcome, out.String())
	}

	data, err := os.ReadFile(outputPath)
	if err != nil {
		t.Fatalf("read output file: %v", err)
	}
	if got := strings.TrimSpace(string(data)); got != "DONE" {
		t.Errorf("output file content = %q; want \"DONE\"", got)
	}
}

// TestSmokeGuardrailAskingSurfacesQuestion proves the AskUserQuestion
// PreToolUse deny's steer (claudeengine's steerAskUserQuestionDeny reason)
// surfaces as the run loop's "asking" outcome: an autonomous run instructed
// to ask the operator a question before writing anything must end its turn
// with that question as its last message, without writing the output file,
// and the strand/run directory must survive for the operator to answer
// into — the same live state the sandbox suite's S2 operator-assisted
// scenario depends on.
func TestSmokeGuardrailAskingSurfacesQuestion(t *testing.T) {
	claudeBinaryPath(t)

	fixture := lyxtest.CopyPaired(t)
	lyxtest.SeedConfig(t, fixture.Hub, map[string]string{
		"shuttle": shuttleengine.ConfigTemplate(),
		"mux":     muxengine.ConfigTemplate(),
	})
	deferHubRelease(t, fixture.Hub)
	t.Chdir(fixture.Hub)
	t.Cleanup(func() {
		var buf bytes.Buffer
		muxcli.RunCLI(&buf, []string{"down"})
	})

	var muxOut bytes.Buffer
	if code := muxcli.RunCLI(&muxOut, []string{"up"}); code != 0 {
		t.Fatalf("mux up = %d; want 0, output: %s", code, muxOut.String())
	}

	outputPath := filepath.Join(fixture.Hub, "smoke-guardrail-asking-output.txt")
	prompt := fmt.Sprintf(
		"Before writing anything to %s, stop and ask me which of two options you should "+
			"pick — do not guess, and do not write the file until I answer.",
		outputPath,
	)

	var out bytes.Buffer
	code := RunCLI(&out, []string{
		"run",
		"--prompt", prompt,
		"--output-file", outputPath,
		"--timeout", "5m",
	})
	if code != 0 {
		t.Fatalf("shuttle run = %d; want 0, output: %s", code, out.String())
	}

	var result map[string]any
	if err := json.Unmarshal(out.Bytes(), &result); err != nil {
		t.Fatalf("parse run result: %v; output: %s", err, out.String())
	}
	if outcome, _ := result["outcome"].(string); outcome != "asking" {
		t.Fatalf("run outcome = %q; want \"asking\"; output: %s", outcome, out.String())
	}
	if msg, _ := result["lastAssistantMessage"].(string); strings.TrimSpace(msg) == "" {
		t.Errorf("run result lastAssistantMessage is empty; want the agent's question; output: %s", out.String())
	}

	if _, err := os.Stat(outputPath); !os.IsNotExist(err) {
		t.Errorf("output file %s exists after an \"asking\" outcome (stat err=%v); want it not yet written", outputPath, err)
	}

	guid, _ := result["guid"].(string)
	if guid == "" {
		t.Fatalf("run result missing guid: %v", result)
	}
	strand, found := muxStatusStrand(t, guid)
	if !found {
		t.Fatalf("mux status missing strand %s after an \"asking\" outcome; want it still tracked", guid)
	}
	if live, _ := strand["live"].(bool); !live {
		t.Errorf("strand %s live = false after an \"asking\" outcome; want true", guid)
	}

	runDir, _ := result["runDir"].(string)
	if runDir == "" {
		t.Fatalf("run result missing runDir: %v", result)
	}
	if _, err := os.Stat(runDir); err != nil {
		t.Errorf("run dir %s missing after an \"asking\" outcome (stat err=%v); want it to persist for diagnosis", runDir, err)
	}
}
