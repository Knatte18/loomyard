// batchingnudge.go implements the batching nudge: claude.yaml's batching_nudge block, the PostToolUse hook command that runs it, the per-run state file Prepare seeds,
// and the judgment that reads the calling chain's transcript tail for a breach of lym's batching rules and answers with one line of additional context.

package claudeengine

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"maps"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/Knatte18/loomyard/internal/shuttleengine"
	"github.com/Knatte18/loomyard/internal/state"
)

var _ shuttleengine.BatchingNudger = (*Claude)(nil)

// BatchingNudgeConfig is claude.yaml's batching_nudge block: the switches and thresholds of the after-tool-call hook that nudges a session back to batching its calls.
// Prepare copies it into the run's state file, so a running session keeps the values it started with.
type BatchingNudgeConfig struct {
	// Enabled installs the hook; off, a run's settings carry no batching hook and Prepare writes no state file.
	Enabled bool `yaml:"enabled" json:"enabled"`
	// LookupChain switches the single-lookup chain breach.
	LookupChain bool `yaml:"lookup_chain" json:"lookup_chain"`
	// ReadSlice switches the read-slice breach.
	ReadSlice bool `yaml:"read_slice" json:"read_slice"`
	// EditChain switches the edit-only chain breach.
	EditChain bool `yaml:"edit_chain" json:"edit_chain"`
	// ContextTokens is the calling chain's own context size, in tokens, below which the hook stays silent.
	ContextTokens int `yaml:"context_tokens" json:"context_tokens"`
	// LookupRun is the number of consecutive single-lookup messages that fires the single-lookup chain breach.
	LookupRun int `yaml:"lookup_run" json:"lookup_run"`
	// EditRun is the number of consecutive edit-only messages that fires the edit-only chain breach.
	EditRun int `yaml:"edit_run" json:"edit_run"`
	// SliceLines is the line count under which a sliced Read of a file is a breach.
	SliceLines int `yaml:"slice_lines" json:"slice_lines"`
	// MaxNudges is the most nudges one run's session, and each of its subagents and forks, gets, so a session that ignores the line is not nagged on every turn.
	MaxNudges int `yaml:"max_nudges" json:"max_nudges"`
}

// batchingNudgeKey is claude.yaml's key of the batching_nudge block.
const batchingNudgeKey = "batching_nudge"

// ValidateBatchingNudge refuses a batching_nudge block a run could not use while the hook is on:
// a negative context_tokens, or a lookup_run, edit_run, slice_lines or max_nudges below 1.
// The refusal names the key and the way forward, lym config claude.
func ValidateBatchingNudge(cfg Config) error {
	nudge := cfg.BatchingNudge
	if !nudge.Enabled {
		return nil
	}
	bounds := []struct {
		key          string
		value, least int
	}{
		{"context_tokens", nudge.ContextTokens, 0},
		{"lookup_run", nudge.LookupRun, 1},
		{"edit_run", nudge.EditRun, 1},
		{"slice_lines", nudge.SliceLines, 1},
		{"max_nudges", nudge.MaxNudges, 1},
	}
	for _, bound := range bounds {
		if bound.value < bound.least {
			return fmt.Errorf("claudeengine: claude.yaml's %s.%s is %d, below its least value %d; way forward: lym config claude --set %s.%s=<n>",
				batchingNudgeKey, bound.key, bound.value, bound.least, batchingNudgeKey, bound.key)
		}
	}
	return nil
}

// Claude Code's names of the tools a batching breach is made of.
const (
	toolRead  = "Read"
	toolGrep  = "Grep"
	toolGlob  = "Glob"
	toolBash  = "Bash"
	toolEdit  = "Edit"
	toolWrite = "Write"
)

// batchingNudgeMatcher is the PostToolUse matcher of the batching hook: the tools a breach is made of, so no other tool call starts a process.
const batchingNudgeMatcher = toolRead + "|" + toolGrep + "|" + toolGlob + "|" + toolBash + "|" + toolEdit + "|" + toolWrite

// batchingNudgeStateFileName is the file in a run directory that holds the run's nudge settings and count.
const batchingNudgeStateFileName = "batching-nudge.json"

// batchingNudgeHookCommand returns the PostToolUse command of the batching hook: the shuttle verb, told this engine's registry name and the run's state file at quotedStatePath.
// It ends in `; true`, so a missing or failing lym never shows the session a hook error.
func batchingNudgeHookCommand(quotedStatePath string) string {
	return "lym shuttle " + shuttleengine.BatchingHookVerb + " claude " + quotedStatePath + "; true"
}

// batchingTailBytes bounds the transcript tail the judgment reads, so a call at a large context costs one bounded read.
const batchingTailBytes = 4 << 20

// batchingTranscriptWait bounds the judgment's wait for the call it judges to reach the calling chain's transcript.
// Claude Code writes a message's lines to the transcript only after its calls start,
// so a fast call such as a Read or an Edit runs its after-tool-call hook before its own message is there to judge.
// The judgment rereads the transcript every tick, at most attempts times, while the file grows.
type batchingTranscriptWait struct {
	tick     time.Duration
	attempts int
	// sleep pauses between attempts; a test passes one that changes the transcript instead of waiting.
	sleep func(time.Duration)
}

// The production tick and attempts bound the wait for a call to reach the transcript.
// The wait covers the write delay after a message ends.
// It leaves out only the early calls of a message still being written.
const (
	batchingTranscriptTick     = 25 * time.Millisecond
	batchingTranscriptAttempts = 80
)

// defaultBatchingTranscriptWait returns the production wait for a call to reach the transcript.
func defaultBatchingTranscriptWait() batchingTranscriptWait {
	return batchingTranscriptWait{tick: batchingTranscriptTick, attempts: batchingTranscriptAttempts, sleep: time.Sleep}
}

// The nudge lines, one per breach; each names the breach and quotes the rule it breaks.
const (
	lookupChainNudge = "[lym batching] Your last %d messages each made one lookup and nothing else. The rule: list every question, then send every grep and read for all of them in one message; wait for a result only to choose the next call, then send every call it unlocks and every call you know in one message."
	readSliceNudge   = "[lym batching] You read a slice of %s, %s. The rule: read a file whole, or every span you need from it, in one message; this overrides the Read tool's advice to read only the part you need, and never grep for a line and then read a slice around it."
	editChainNudge   = "[lym batching] Your last %d messages each held only edits. The rule: decide every edit before you make any, then make all of them in one message, every Edit and Write call together."
)

// The reasons a sliced Read breaks the rule, each filling readSliceNudge.
const (
	sliceShortReason = "a file under %d lines"
	sliceReadReason  = "a file you already read that has not changed since"
)

// errNoNudge ends a state update that nudges nothing, so the state file is left unwritten.
var errNoNudge = errors.New("no nudge")

// batchingNudgeCount is the nudges one chain of a session got: the main session's, or one subagent's or fork's.
type batchingNudgeCount struct {
	Nudges int `json:"nudges"`
	// NudgedMessage is the id of the assistant message the latest nudge answered, so the parallel calls of one message nudge at most once.
	NudgedMessage string `json:"nudged_message"`
}

// batchingNudgeState is a run's nudge state file: the settings Prepare copied from claude.yaml, and the nudge count of the main session and of each subagent and fork.
type batchingNudgeState struct {
	Settings BatchingNudgeConfig `json:"settings"`
	// batchingNudgeCount is the main session's count.
	batchingNudgeCount
	// Agents holds each subagent's and fork's own count, keyed by its agent id.
	Agents map[string]batchingNudgeCount `json:"agents,omitempty"`
}

// count returns the nudge count of the chain agentID names, the main session's for an empty id.
func (s batchingNudgeState) count(agentID string) batchingNudgeCount {
	if agentID == "" {
		return s.batchingNudgeCount
	}
	return s.Agents[agentID]
}

// withCount returns s with the nudge count of the chain agentID names set to count, leaving s's own agent map unchanged.
func (s batchingNudgeState) withCount(agentID string, count batchingNudgeCount) batchingNudgeState {
	if agentID == "" {
		s.batchingNudgeCount = count
		return s
	}
	agents := maps.Clone(s.Agents)
	if agents == nil {
		agents = map[string]batchingNudgeCount{}
	}
	agents[agentID] = count
	s.Agents = agents
	return s
}

// batchingNudgeLockPath returns the lock file guarding the state file at statePath.
func batchingNudgeLockPath(statePath string) string {
	return statePath + ".lock"
}

// writeBatchingNudgeState seeds the state file at statePath with settings and no nudges.
func writeBatchingNudgeState(statePath string, settings BatchingNudgeConfig) error {
	return state.WriteJSON(statePath, batchingNudgeLockPath(statePath), batchingNudgeState{Settings: settings})
}

// batchingHookInput is the part of Claude Code's PostToolUse input the judgment reads.
type batchingHookInput struct {
	TranscriptPath string `json:"transcript_path"`
	AgentID        string `json:"agent_id"`
	ToolName       string `json:"tool_name"`
	ToolUseID      string `json:"tool_use_id"`
	ToolInput      struct {
		FilePath string `json:"file_path"`
		Offset   *int   `json:"offset"`
		Limit    *int   `json:"limit"`
		Command  string `json:"command"`
	} `json:"tool_input"`
}

// chainTranscript returns the transcript the call's own chain is written to:
// the session's for a main-chain call, and for a subagent's or fork's call its own transcript under the subagents directory beside the session's, unless transcript_path already names that file.
// It reports false for an agent id that is not a plain file name part.
func (in batchingHookInput) chainTranscript() (string, bool) {
	if in.AgentID == "" {
		return in.TranscriptPath, true
	}
	if strings.ContainsAny(in.AgentID, `/\`) || in.AgentID == "." || in.AgentID == ".." {
		return "", false
	}
	name := subagentTranscriptPrefix + in.AgentID + ".jsonl"
	if filepath.Base(in.TranscriptPath) == name {
		return in.TranscriptPath, true
	}
	return filepath.Join(strings.TrimSuffix(in.TranscriptPath, ".jsonl"), subagentsDirName, name), true
}

// sliced reports whether the call reads part of a file: a Read with a limit, or with an offset past the first line.
func (in batchingHookInput) sliced() bool {
	if in.ToolName != toolRead {
		return false
	}
	return in.ToolInput.Limit != nil || (in.ToolInput.Offset != nil && *in.ToolInput.Offset > 1)
}

// BatchingNudge judges the tool call a PostToolUse input describes and returns the hook output that adds one nudge line to the calling chain's context, or nil when no nudge is due.
// A call from a subagent or fork is judged against that agent's own transcript, context reading and nudge count, under the same settings as the main session.
// A Bash call whose command is no read-only lookup, a run with no state file, a chain below the context threshold, a chain past its nudge cap and a message already nudged get none.
// Claude Code writes a call's message to the transcript after the call starts,
// so a call whose message the transcript does not hold yet is waited for a bounded time;
// a call still missing at the bound gets none.
// The context threshold is read from the first tail, which lacks the call's own message while the call is waited for,
// so the one call whose message first reaches the threshold gets none.
// The judgment and the count update hold the state file's lock, so the parallel calls of one message nudge at most once.
// A transcript that cannot be read gets no nudge and no error, since its format is a Claude Code internal; a malformed input or state file is an error.
func (c *Claude) BatchingNudge(statePath string, payload []byte) ([]byte, error) {
	var input batchingHookInput
	if err := json.Unmarshal(payload, &input); err != nil {
		return nil, fmt.Errorf("claudeengine: parse the batching hook input: %w", err)
	}
	if input.TranscriptPath == "" || input.ToolUseID == "" {
		return nil, nil
	}
	// A Bash call that is no lookup makes a message that is neither a lookup nor edits, so no breach can fire and the transcript is left unread.
	if input.ToolName == toolBash && !isReadOnlyLookup(input.ToolInput.Command) {
		return nil, nil
	}
	transcript, ok := input.chainTranscript()
	if !ok {
		return nil, nil
	}
	sidechain := input.AgentID != ""
	// The state is read before the wait, so a run with no state file and a chain past its cap never wait for the transcript.
	seeded, found, err := state.ReadJSON[batchingNudgeState](statePath, batchingNudgeLockPath(statePath))
	if err != nil {
		return nil, fmt.Errorf("claudeengine: read the batching nudge state %s: %w", statePath, err)
	}
	if !found || seeded.count(input.AgentID).Nudges >= seeded.Settings.MaxNudges {
		return nil, nil
	}
	tail, ok := c.awaitCallInTranscript(transcript, input.ToolUseID, sidechain, seeded.Settings.ContextTokens)
	if !ok {
		return nil, nil
	}

	var line string
	err = state.UpdateJSON(statePath, batchingNudgeLockPath(statePath), func(cur batchingNudgeState, found bool) (batchingNudgeState, error) {
		count := cur.count(input.AgentID)
		if !found || count.Nudges >= cur.Settings.MaxNudges {
			return cur, errNoNudge
		}
		reading, _ := latestReading(tail, sidechain)
		if !reading.Known || reading.Tokens < cur.Settings.ContextTokens {
			return cur, errNoNudge
		}
		messages := tailMessages(tail, sidechain)
		current := messageHolding(messages, input.ToolUseID)
		if messages[current].id == count.NudgedMessage {
			return cur, errNoNudge
		}
		line = judgeBatching(cur.Settings, messages, current, input)
		if line == "" {
			return cur, errNoNudge
		}
		return cur.withCount(input.AgentID, batchingNudgeCount{Nudges: count.Nudges + 1, NudgedMessage: messages[current].id}), nil
	})
	if errors.Is(err, errNoNudge) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("claudeengine: update the batching nudge state %s: %w", statePath, err)
	}
	return hookContext(hookEventPostToolUse, line)
}

// awaitCallInTranscript waits for a message of the chain to hold the call toolUseID.
// It returns the tail of the transcript at transcriptPath once one does.
// It reports false when none does within the wait's bound.
// It reports false at once for a transcript that cannot be read.
// It also reports false at once for a chain whose context is below contextTokens by the first tail's latest reading.
// While the call's message is not in the transcript yet, that reading is the previous message's,
// so the one call whose message first reaches contextTokens is left silent.
// A chain the judgment would leave silent therefore never waits.
// It rereads the tail only when the file's size changed since the last read.
func (c *Claude) awaitCallInTranscript(transcriptPath, toolUseID string, sidechain bool, contextTokens int) ([]byte, bool) {
	wait := c.transcriptWait
	readSize := int64(-1)
	for attempt := 0; ; attempt++ {
		info, err := os.Stat(transcriptPath)
		if err != nil {
			return nil, false
		}
		if info.Size() != readSize {
			readSize = info.Size()
			tail, err := readTranscriptTail(transcriptPath, batchingTailBytes)
			if err != nil {
				return nil, false
			}
			if attempt == 0 {
				if reading, _ := latestReading(tail, sidechain); !reading.Known || reading.Tokens < contextTokens {
					return nil, false
				}
			}
			if messageHolding(tailMessages(tail, sidechain), toolUseID) >= 0 {
				return tail, true
			}
		}
		if attempt >= wait.attempts {
			return nil, false
		}
		wait.sleep(wait.tick)
	}
}

// readTranscriptTail returns the complete lines of the last budget bytes of the transcript at transcriptPath, the whole file when it is smaller.
// The window's first line is dropped unless the window starts the file, since it may be cut.
func readTranscriptTail(transcriptPath string, budget int64) ([]byte, error) {
	f, err := os.Open(transcriptPath)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return nil, err
	}
	start := info.Size() - budget
	if start < 0 {
		start = 0
	}
	tail := make([]byte, info.Size()-start)
	if _, err := f.ReadAt(tail, start); err != nil && err != io.EOF {
		return nil, err
	}
	if start > 0 {
		nl := bytes.IndexByte(tail, '\n')
		if nl < 0 {
			return nil, nil
		}
		tail = tail[nl+1:]
	}
	return tail, nil
}

// tailMessage is one assistant message of a transcript tail's chain: its id, the time of its first line and its tool calls in order.
type tailMessage struct {
	id    string
	at    time.Time
	calls []tailCall
}

// tailCall is one tool call of a tail message, with the file path or the shell command its input names.
type tailCall struct {
	id       string
	name     string
	filePath string
	command  string
}

// lookup reports whether the call is one lookup: a Read, Grep or Glob call, or a Bash call whose command is a read-only lookup.
func (call tailCall) lookup() bool {
	switch call.name {
	case toolRead, toolGrep, toolGlob:
		return true
	case toolBash:
		return isReadOnlyLookup(call.command)
	}
	return false
}

// tailMessages groups the assistant lines of tail's chain by message id, in the order each message first appears:
// the sidechain lines of a subagent's transcript when sidechain is true, and the main-chain lines otherwise.
// A line that does not parse, a line of the other chain and a line with no message id are skipped.
func tailMessages(tail []byte, sidechain bool) []tailMessage {
	var messages []tailMessage
	index := map[string]int{}
	for _, line := range bytes.Split(tail, []byte("\n")) {
		if !bytes.Contains(line, []byte(`"assistant"`)) {
			continue
		}
		var record struct {
			Type        string `json:"type"`
			IsSidechain bool   `json:"isSidechain"`
			Timestamp   string `json:"timestamp"`
			Message     struct {
				ID      string          `json:"id"`
				Content json.RawMessage `json:"content"`
			} `json:"message"`
		}
		if json.Unmarshal(line, &record) != nil || record.Type != "assistant" || record.IsSidechain != sidechain || record.Message.ID == "" {
			continue
		}
		i, seen := index[record.Message.ID]
		if !seen {
			at, _ := time.Parse(time.RFC3339Nano, record.Timestamp)
			i = len(messages)
			index[record.Message.ID] = i
			messages = append(messages, tailMessage{id: record.Message.ID, at: at})
		}
		messages[i].calls = append(messages[i].calls, contentToolCalls(record.Message.Content)...)
	}
	return messages
}

// contentToolCalls returns the tool calls of an assistant line's content, with the file path or shell command a call's input names.
func contentToolCalls(content json.RawMessage) []tailCall {
	var blocks []struct {
		Type  string `json:"type"`
		ID    string `json:"id"`
		Name  string `json:"name"`
		Input struct {
			FilePath string `json:"file_path"`
			Command  string `json:"command"`
		} `json:"input"`
	}
	if json.Unmarshal(content, &blocks) != nil {
		return nil
	}
	var calls []tailCall
	for _, block := range blocks {
		if block.Type == "tool_use" {
			calls = append(calls, tailCall{id: block.ID, name: block.Name, filePath: block.Input.FilePath, command: block.Input.Command})
		}
	}
	return calls
}

// messageHolding returns the index of the message holding the call toolUseID, or -1 when no message holds it.
func messageHolding(messages []tailMessage, toolUseID string) int {
	for i := len(messages) - 1; i >= 0; i-- {
		for _, call := range messages[i].calls {
			if call.id == toolUseID {
				return i
			}
		}
	}
	return -1
}

// messageKind classes a message by the calls a batching breach is made of.
type messageKind int

const (
	// kindOther is a message that holds no call, or calls that make no chain.
	kindOther messageKind = iota
	// kindLookup is a message that holds exactly one lookup call and nothing else.
	kindLookup
	// kindEdits is a message that holds only Edit or Write calls.
	kindEdits
)

// kindOf classes message m.
func kindOf(m tailMessage) messageKind {
	if len(m.calls) == 0 {
		return kindOther
	}
	if len(m.calls) == 1 && m.calls[0].lookup() {
		return kindLookup
	}
	for _, call := range m.calls {
		if call.name != toolEdit && call.name != toolWrite {
			return kindOther
		}
	}
	return kindEdits
}

// runEndingAt counts the consecutive messages of kind that end at index last.
func runEndingAt(messages []tailMessage, last int, kind messageKind) int {
	run := 0
	for i := last; i >= 0 && kindOf(messages[i]) == kind; i-- {
		run++
	}
	return run
}

// judgeBatching returns the nudge line for the first breach the call in message current makes, or "" for none:
// a single-lookup chain, then a read slice, then an edit-only chain, each only while its switch is on.
func judgeBatching(settings BatchingNudgeConfig, messages []tailMessage, current int, input batchingHookInput) string {
	kind := kindOf(messages[current])
	if settings.LookupChain && kind == kindLookup {
		if run := runEndingAt(messages, current, kindLookup); run >= settings.LookupRun {
			return fmt.Sprintf(lookupChainNudge, run)
		}
	}
	if settings.ReadSlice && input.sliced() {
		if reason := sliceBreach(settings, messages, current, input.ToolInput.FilePath); reason != "" {
			return fmt.Sprintf(readSliceNudge, input.ToolInput.FilePath, reason)
		}
	}
	if settings.EditChain && kind == kindEdits {
		if run := runEndingAt(messages, current, kindEdits); run >= settings.EditRun {
			return fmt.Sprintf(editChainNudge, run)
		}
	}
	return ""
}

// sliceBreach returns why a sliced Read of filePath in message current breaks the rule, or "" when it does not:
// the file holds fewer than the settings' slice lines, or the latest earlier message that read it is not older than the file's last modification.
// A file that cannot be read or statted is no breach.
func sliceBreach(settings BatchingNudgeConfig, messages []tailMessage, current int, filePath string) string {
	lines, err := countLinesUpTo(filePath, settings.SliceLines)
	if err != nil {
		return ""
	}
	if lines < settings.SliceLines {
		return fmt.Sprintf(sliceShortReason, settings.SliceLines)
	}
	info, err := os.Stat(filePath)
	if err != nil {
		return ""
	}
	for i := current - 1; i >= 0; i-- {
		for _, call := range messages[i].calls {
			if call.name != toolRead || call.filePath != filePath {
				continue
			}
			if !messages[i].at.IsZero() && !info.ModTime().After(messages[i].at) {
				return sliceReadReason
			}
			return ""
		}
	}
	return ""
}

// countLinesUpTo counts the lines of the file at filePath, stopping once the count reaches limit; a last line without its newline counts.
func countLinesUpTo(filePath string, limit int) (int, error) {
	f, err := os.Open(filePath)
	if err != nil {
		return 0, err
	}
	defer f.Close()
	buf := make([]byte, 32*1024)
	lines, unterminated := 0, false
	for lines < limit {
		n, err := f.Read(buf)
		lines += bytes.Count(buf[:n], []byte{'\n'})
		if n > 0 {
			unterminated = buf[n-1] != '\n'
		}
		if err == io.EOF {
			if unterminated {
				lines++
			}
			return lines, nil
		}
		if err != nil {
			return 0, err
		}
	}
	return lines, nil
}
