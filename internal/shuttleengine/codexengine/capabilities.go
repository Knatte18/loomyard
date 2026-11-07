// capabilities.go implements Codex's side of creel's hook delivery:
// the hook-input parse, the block that hands mail to the session, and the idle wake through `codex queue`.

package codexengine

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
)

var (
	_ shuttleengine.ToolBoundaryContextRenderer = (*Codex)(nil)
	_ shuttleengine.IdleWaker                   = (*Codex)(nil)
)

// HookInput parses the JSON object Codex passes a hook on standard input.
// `PostToolUse` maps to the tool-boundary event and `Stop` to the stop event;
// any other event leaves `Event` empty.
// Codex runs no subagent whose calls reach these hooks, so no input is marked as a subagent call.
func (c *Codex) HookInput(payload []byte) (shuttleengine.HookInput, error) {
	if !bytes.HasPrefix(bytes.TrimSpace(payload), []byte("{")) {
		return shuttleengine.HookInput{}, fmt.Errorf("parse hook input: the payload is not a JSON object")
	}
	var parsed struct {
		HookEventName  string `json:"hook_event_name"`
		StopHookActive bool   `json:"stop_hook_active"`
	}
	if err := json.Unmarshal(payload, &parsed); err != nil {
		return shuttleengine.HookInput{}, fmt.Errorf("parse hook input: %w", err)
	}
	input := shuttleengine.HookInput{StopHookActive: parsed.StopHookActive}
	switch parsed.HookEventName {
	case hookEventPostToolUse:
		input.Event = shuttleengine.HookEventToolBoundary
	case hookEventStop:
		input.Event = shuttleengine.HookEventStop
	}
	return input, nil
}

// HookOutput renders the JSON a `PostToolUse` or `Stop` hook prints to hand mail to the session.
// The JSON sets `decision` to `block` and `reason` to the mail.
// Codex hands the reason to the model at its next step and shows the block to the operator as `Blocked by hook`.
// With no mail it returns `{}`, the empty hook output.
// The echo is dropped: the idle wake queued the same lines as a user turn the operator already sees.
// Codex shows a system message beside the reason, so an echo would read as new mail.
func (c *Codex) HookOutput(_ shuttleengine.HookEvent, mail, _ string) ([]byte, error) {
	if mail == "" {
		return []byte("{}"), nil
	}
	data, err := json.Marshal(struct {
		Decision string `json:"decision"`
		Reason   string `json:"reason"`
	}{Decision: "block", Reason: mail})
	if err != nil {
		return nil, fmt.Errorf("marshal hook output: %w", err)
	}
	return data, nil
}

// RewakeOutput returns no text and exit code 0:
// no exit of a Codex hook wakes an idle session, so the idle wake starts the turn itself.
func (c *Codex) RewakeOutput(string) (stderr []byte, exitCode int) {
	return nil, 0
}

// queueTimeout bounds one `codex queue` call, which returns once Codex has queued the message.
const queueTimeout = 30 * time.Second

// WakeIdle starts a turn carrying text in the idle Codex session that the waiter hook's payload names.
// It queues the text through `codex queue --thread <session_id> --message <text>`.
// It runs the executable that `LYM_CODEX_BINARY` names in the waiter's environment, or `codex` on PATH if unset.
// It runs under the CODEX_HOME that environment sets.
// A payload naming no session id, or a queue that fails or outlasts its timeout, returns an error and starts no turn.
func (c *Codex) WakeIdle(payload []byte, text string) error {
	var parsed struct {
		SessionID string `json:"session_id"`
	}
	if err := json.Unmarshal(payload, &parsed); err != nil {
		return fmt.Errorf("codexengine: parse the idle waiter's hook input: %w", err)
	}
	if parsed.SessionID == "" {
		return errors.New("codexengine: the idle waiter's hook input names no session_id, so no Codex thread can be queued")
	}
	bin := os.Getenv(codexBinaryEnvKey)
	if bin == "" {
		bin = codexBinary(Config{})
	}
	ctx, cancel := context.WithTimeout(context.Background(), queueTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, bin, "queue", "--thread", parsed.SessionID, "--message", text)
	logger.Info("codexengine: queueing creel mail as a turn of the idle Codex session", "binary", bin, "thread", parsed.SessionID)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("codexengine: %s queue --thread %s: %w: %s", bin, parsed.SessionID, err, strings.TrimSpace(string(out)))
	}
	logger.Info("codexengine: queued creel mail as a turn of the idle Codex session", "thread", parsed.SessionID)
	return nil
}
