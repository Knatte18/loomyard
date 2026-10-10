//go:build llm

// smoke_guardrail_test.go is the live proof of the deny-and-steer guardrail path the hooks research flagged as unprobed
// (docs/research/reed-hooks-exploration.md: "the deny-and-steer path itself is not yet probed"):
// a REAL claude, when its Agent tool call is denied by the PreToolUse hook, actually resumes in-session on the steered instruction rather than stalling or aborting the turn,
// and a REAL claude asked to pose a question stays held, notifies its parent once and finishes once answered.
// Follows the same conventions as smoke_run_test.go,
// whose helpers (deferHubRelease, reedStatusStrand) this
// file reuses.

package shuttlecli

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/hubforge"
	"github.com/Knatte18/loomyard/internal/reedcli"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
	"github.com/Knatte18/loomyard/internal/testkit/llmkit"
)

// TestSmokeGuardrailDeniesAgentTool proves the "deny-and-steer" path this round closes from the
// hooks research's open item: the run's prompt explicitly instructs the agent to dispatch a
// subagent via its in-process Agent tool to write the output file, falling back to writing it
// directly only if the Agent tool turns out to be unavailable.
// The PreToolUse(Agent) hook denies the call and steers the model back into this pane
// (claudeengine's steerAgentDeny reason);
// the run still reaching "done" with the file written is the direct, live proof that the deny fired
// AND the steer redirected the work in-session, rather than the turn stalling or the agent giving
// up once its preferred tool was refused.
func TestSmokeGuardrailDeniesAgentTool(t *testing.T) {
	llmkit.Claude(t, "LYX_REED_CLAUDE")

	h := hubforge.CopyHub(t, hubforge.Shape{Anchor: "."})
	deferHubRelease(t, h.Path)
	registerReedServer(t, h)
	t.Chdir(h.PrimeWorktree())
	t.Cleanup(func() {
		var buf bytes.Buffer
		reedcli.RunCLI(&buf, []string{"down"})
	})

	var reedOut bytes.Buffer
	if code := reedcli.RunCLI(&reedOut, []string{"up"}); code != 0 {
		t.Fatalf("reed up = %d; want 0, output: %s", code, reedOut.String())
	}

	outputPath := filepath.Join(h.PrimeWorktree(), "smoke-guardrail-agent-output.txt")
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
		"--model", smokeClaudeModel,
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

// TestSmokeGuardrailQuestionHoldsRunUntilAnswered proves the AskUserQuestion PreToolUse deny's steer (claudeengine's steerAskUserQuestionDeny reason) leaves an autonomous run held:
// an agent told to ask the operator a question before writing anything states it and ends its turn, Wait does not return,
// and the parent's notifier is called exactly once for that held turn end.
// The test then answers through Run.Send with a text ending in MessageTail, and the run finishes done with the output file written —
// the same live state the sandbox suite's S2 hold scenario depends on.
func TestSmokeGuardrailQuestionHoldsRunUntilAnswered(t *testing.T) {
	llmkit.Claude(t, "LYX_REED_CLAUDE")

	h := hubforge.CopyHub(t, hubforge.Shape{Anchor: "."})
	deferHubRelease(t, h.Path)
	registerReedServer(t, h)
	t.Chdir(h.PrimeWorktree())
	t.Cleanup(func() {
		var buf bytes.Buffer
		reedcli.RunCLI(&buf, []string{"down"})
	})

	var reedOut bytes.Buffer
	if code := reedcli.RunCLI(&reedOut, []string{"up"}); code != 0 {
		t.Fatalf("reed up = %d; want 0, output: %s", code, reedOut.String())
	}

	runner, _, _, _ := newSmokeRunner(t)
	var (
		noticesMu sync.Mutex
		notices   []string
	)
	noticeCount := func() int {
		noticesMu.Lock()
		defer noticesMu.Unlock()
		return len(notices)
	}
	runner.SetNotifier(func(line string) error {
		noticesMu.Lock()
		defer noticesMu.Unlock()
		notices = append(notices, line)
		return nil
	})

	outputPath := filepath.Join(h.PrimeWorktree(), "smoke-guardrail-question-output.txt")
	prompt := fmt.Sprintf(
		"Before writing anything to %s, stop and ask me which of two options you should "+
			"pick — do not guess, and do not write the file until I answer.",
		outputPath,
	)
	run, err := runner.Start(shuttleengine.Spec{
		Prompt:      prompt,
		OutputFiles: []string{outputPath},
		Model:       smokeClaudeModel,
		Timeout:     5 * time.Minute,
	})
	if err != nil {
		t.Fatalf("runner.Start: %v", err)
	}

	waitCh := make(chan waitOutcome, 1)
	go func() {
		result, waitErr := run.Wait()
		waitCh <- waitOutcome{result, waitErr}
	}()

	noticeDeadline := time.Now().Add(3 * time.Minute)
	for noticeCount() == 0 {
		select {
		case res := <-waitCh:
			t.Fatalf("run.Wait returned (outcome=%s err=%v) before the held turn end was notified; want it to keep waiting", res.result.Outcome, res.err)
		default:
		}
		if time.Now().After(noticeDeadline) {
			t.Fatal("no hold notice within 3m; want one for the agent's question turn end")
		}
		time.Sleep(time.Second)
	}

	if _, err := os.Stat(outputPath); !os.IsNotExist(err) {
		t.Errorf("output file %s exists while held (stat err=%v); want it not yet written", outputPath, err)
	}
	select {
	case res := <-waitCh:
		t.Fatalf("run.Wait returned (outcome=%s err=%v) at the held turn end; want it to keep waiting", res.result.Outcome, res.err)
	default:
	}

	answer := shuttleengine.WithMessageTail(fmt.Sprintf("Pick the first option and write exactly DONE to %s.", outputPath))
	if err := run.Send(answer); err != nil {
		t.Fatalf("run.Send: %v", err)
	}

	select {
	case res := <-waitCh:
		if res.err != nil {
			t.Fatalf("run.Wait: %v", res.err)
		}
		if res.result.Outcome != shuttleengine.OutcomeDone {
			t.Fatalf("run.Wait outcome = %q; want %q after the answer", res.result.Outcome, shuttleengine.OutcomeDone)
		}
	case <-time.After(5 * time.Minute):
		t.Fatal("run.Wait did not return within 5m after the answer")
	}

	if _, err := os.Stat(outputPath); err != nil {
		t.Errorf("output file %s missing after the run finished done (stat err=%v)", outputPath, err)
	}
	if got := noticeCount(); got != 1 {
		t.Errorf("notifier called %d times; want exactly 1, for the one held turn end", got)
	}
}
