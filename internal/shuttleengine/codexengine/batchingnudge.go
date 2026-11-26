// batchingnudge.go implements the Codex batching nudge:
// codex.yaml's batching_nudge block, the PostToolUse hook command, and the per-run state file Prepare seeds.
// It also holds the judgment that reads the calling thread's rollout for a breach of lym's batching rules,
// and it answers with one line of additional context.

package codexengine

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"time"

	"github.com/Knatte18/loomyard/internal/shell"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
	"github.com/Knatte18/loomyard/internal/shuttleengine/bashlookup"
	"github.com/Knatte18/loomyard/internal/state"
)

var _ shuttleengine.BatchingNudger = (*Codex)(nil)

// BatchingNudgeConfig is codex.yaml's batching_nudge block.
// It holds the switches and thresholds of the after-tool-call hook that nudges a session back to batching its calls.
// Prepare copies it into the run's state file, so a running session keeps the values it started with.
type BatchingNudgeConfig struct {
	// Enabled installs the hook; off, a launch carries no batching hook and Prepare writes no state file.
	Enabled bool `yaml:"enabled" json:"enabled"`
	// LookupChain switches the single-lookup chain breach.
	LookupChain bool `yaml:"lookup_chain" json:"lookup_chain"`
	// ReadSlice switches the read-slice breach.
	ReadSlice bool `yaml:"read_slice" json:"read_slice"`
	// EditChain switches the edit-only chain breach.
	EditChain bool `yaml:"edit_chain" json:"edit_chain"`
	// ContextTokens is the calling thread's own context size, in tokens, below which the hook stays silent.
	ContextTokens int `yaml:"context_tokens" json:"context_tokens"`
	// LookupRun is the number of consecutive single-lookup exec cells that fires the single-lookup chain breach.
	LookupRun int `yaml:"lookup_run" json:"lookup_run"`
	// EditRun is the number of consecutive apply_patch-only exec cells that fires the edit-only chain breach.
	EditRun int `yaml:"edit_run" json:"edit_run"`
	// SliceLines is the line count under which a line-range read of a file is a breach.
	SliceLines int `yaml:"slice_lines" json:"slice_lines"`
	// MaxNudges is the most nudges each thread of a run, the session's and each child's, gets,
	// so a session that ignores the line is not nagged on every turn.
	MaxNudges int `yaml:"max_nudges" json:"max_nudges"`
}

// batchingNudgeKey is codex.yaml's key of the batching_nudge block.
const batchingNudgeKey = "batching_nudge"

// validateBatchingNudge refuses a batching_nudge block a run could not use while the hook is on:
// a negative context_tokens, or a lookup_run, edit_run, slice_lines or max_nudges below 1.
// The refusal names the key.
func validateBatchingNudge(nudge BatchingNudgeConfig) error {
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
			return fmt.Errorf("%s is %d, below its least value %d", keyName(batchingNudgeKey+"."+bound.key), bound.value, bound.least)
		}
	}
	return nil
}

// toolNameApplyPatch is the tool name a nested apply_patch call's PostToolUse payload carries.
const toolNameApplyPatch = "apply_patch"

// batchingNudgeStateFileName is the file in a run directory that holds the run's nudge settings and counts.
const batchingNudgeStateFileName = "batching-nudge.json"

// batchingNudgeHookCommand returns the PostToolUse command of the batching hook over the run's state file at statePath:
// the shuttle verb, told this engine's registry name.
// It prints the verb's output when the verb prints one and the empty hook output otherwise, exiting 0 either way,
// so a missing or failing lym never shows the session a hook error.
func batchingNudgeHookCommand(statePath string) string {
	verb := "lym shuttle " + shuttleengine.BatchingHookVerb + " " + engineName + " " + shell.Posix().Quote(statePath)
	return `out=$(` + verb + `); [ -n "$out" ] && printf '%s\n' "$out" || ` + emptyHookOutput
}

// batchingTailBytes bounds the rollout tail the judgment reads, so a call at a large context costs one bounded read.
const batchingTailBytes = 4 << 20

// batchingRolloutWait bounds the judgment's wait for a line-range read's own item to reach the rollout.
// The judgment rereads the rollout every tick, at most attempts times, while the file grows.
type batchingRolloutWait struct {
	tick     time.Duration
	attempts int
	// sleep pauses between attempts; a test passes one that changes the rollout instead of waiting.
	sleep func(time.Duration)
}

// The production tick and attempts bound the wait for a call's item to reach the rollout.
const (
	batchingRolloutTick     = 25 * time.Millisecond
	batchingRolloutAttempts = 80
)

// defaultBatchingRolloutWait returns the production wait for a call's item to reach the rollout.
func defaultBatchingRolloutWait() batchingRolloutWait {
	return batchingRolloutWait{tick: batchingRolloutTick, attempts: batchingRolloutAttempts, sleep: time.Sleep}
}

// The nudge lines, one per breach; each names the breach and quotes the rule it breaks in Codex's terms.
const (
	lookupChainNudge = "[lym batching] Your last %d exec calls each ran one lookup and nothing else. The rule: list every question, then run every grep and read for all of them in one exec call; wait for a result only to choose the next call, then run every call it unlocks and every call you know in one exec call."
	readSliceNudge   = "[lym batching] You printed a line range of %s, %s. The rule: read a file whole, or every span you need from it, in one exec call, and never grep for a line and then print a range around it."
	editChainNudge   = "[lym batching] Your last %d exec calls each held only apply_patch edits. The rule: decide every edit before you make any, then make all of them in one exec call."
)

// The reasons a line-range read breaks the rule, each filling readSliceNudge.
const (
	sliceShortReason = "a file under %d lines"
	sliceReadReason  = "a file you already read that has not changed since"
)

// errNoNudge ends a state update that nudges nothing, so the state file is left unwritten.
var errNoNudge = errors.New("no nudge")

// batchingNudgeCount is the nudges one thread of a run got.
type batchingNudgeCount struct {
	Nudges int `json:"nudges"`
	// NudgedCell is the call id of the exec cell the latest nudge answered.
	// It keeps the nested calls of one cell from nudging more than once.
	NudgedCell string `json:"nudged_cell"`
}

// batchingNudgeState is a run's nudge state file.
// It holds the settings Prepare copied from codex.yaml and each thread's nudge count.
type batchingNudgeState struct {
	Settings BatchingNudgeConfig `json:"settings"`
	// Threads holds each thread's count, keyed by the rollout path its hook payloads carry as transcript_path:
	// a Codex payload carries no agent id, and a child's carries its parent's session id.
	Threads map[string]batchingNudgeCount `json:"threads,omitempty"`
}

// withCount returns s with the nudge count of the thread whose rollout is rolloutPath set to count.
// The map of s itself is left unchanged.
func (s batchingNudgeState) withCount(rolloutPath string, count batchingNudgeCount) batchingNudgeState {
	threads := maps.Clone(s.Threads)
	if threads == nil {
		threads = map[string]batchingNudgeCount{}
	}
	threads[rolloutPath] = count
	s.Threads = threads
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

// batchingHookInput is the part of Codex's PostToolUse input the judgment reads.
type batchingHookInput struct {
	TranscriptPath string `json:"transcript_path"`
	ToolName       string `json:"tool_name"`
	ToolUseID      string `json:"tool_use_id"`
	ToolInput      struct {
		Command string `json:"command"`
	} `json:"tool_input"`
}

// nestedCall is one tool call an exec cell ran: a shell command or an apply_patch.
type nestedCall struct {
	id string
	// patch reports an apply_patch call; a shell call has a command instead.
	patch   bool
	command string
	// recorded reports whether the call's item is in the rollout; a call known only from its hook payload is not.
	recorded bool
	// cwd is the directory the shell call ran in, from its item; empty when unknown.
	cwd string
	// reads are the paths the shell call's item marks as read in its parsed_cmd, as the command wrote them.
	reads []string
}

// lookup reports whether the call is a shell call whose command is a read-only lookup.
func (call nestedCall) lookup() bool {
	return !call.patch && bashlookup.IsReadOnlyLookup(call.command)
}

// topCall is one top-level call item of a rollout, one model call.
// An exec cell holds the nested calls whose items fall between its call and its output.
// Any other call holds none.
type topCall struct {
	callID string
	at     time.Time
	exec   bool
	nested []nestedCall
}

// Rollout names the judgment reads.
const (
	rolloutCustomToolCall       = "custom_tool_call"
	rolloutCustomToolCallOutput = "custom_tool_call_output"
	rolloutFunctionCall         = "function_call"
	rolloutFunctionCallOutput   = "function_call_output"
	rolloutItemCompleted        = "item_completed"
	itemCommandExecution        = "CommandExecution"
	itemFileChange              = "FileChange"
	parsedCommandRead           = "read"
	execCellToolName            = "exec"
)

// rolloutCalls lists the top-level call items of a rollout tail in rollout order.
// An exec custom_tool_call opens a cell, and its output closes it.
// A CommandExecution or FileChange item joins the latest cell open when it completes.
// One outside every open cell is skipped.
// Every other custom_tool_call and every function_call is a top-level call holding no nested call.
// Other items, and lines that do not parse, are skipped.
func rolloutCalls(tail []byte) []topCall {
	var calls []topCall
	// open maps the call id of each exec cell whose output is not written to its index in calls.
	open := map[string]int{}
	for _, line := range bytes.Split(tail, []byte("\n")) {
		var record struct {
			Timestamp string `json:"timestamp"`
			Type      string `json:"type"`
			Payload   struct {
				Type   string `json:"type"`
				CallID string `json:"call_id"`
				Name   string `json:"name"`
				Item   *struct {
					Type      string   `json:"type"`
					ID        string   `json:"id"`
					Command   []string `json:"command"`
					Cwd       string   `json:"cwd"`
					ParsedCmd []struct {
						Type string `json:"type"`
						Path string `json:"path"`
					} `json:"parsed_cmd"`
				} `json:"item"`
			} `json:"payload"`
		}
		if json.Unmarshal(line, &record) != nil {
			continue
		}
		payload := record.Payload
		switch {
		case record.Type == rolloutResponseItem && (payload.Type == rolloutCustomToolCall || payload.Type == rolloutFunctionCall):
			call := topCall{callID: payload.CallID, exec: payload.Type == rolloutCustomToolCall && payload.Name == execCellToolName}
			call.at, _ = time.Parse(time.RFC3339Nano, record.Timestamp)
			if call.exec {
				open[call.callID] = len(calls)
			}
			calls = append(calls, call)
		case record.Type == rolloutResponseItem && (payload.Type == rolloutCustomToolCallOutput || payload.Type == rolloutFunctionCallOutput):
			delete(open, payload.CallID)
		case record.Type == rolloutEventMessage && payload.Type == rolloutItemCompleted && payload.Item != nil && len(open) > 0:
			item := payload.Item
			nested := nestedCall{id: item.ID, recorded: true}
			switch item.Type {
			case itemCommandExecution:
				nested.command = shellCommandOf(item.Command)
				nested.cwd = localPath(item.Cwd)
				for _, parsed := range item.ParsedCmd {
					if parsed.Type == parsedCommandRead && parsed.Path != "" {
						nested.reads = append(nested.reads, parsed.Path)
					}
				}
			case itemFileChange:
				nested.patch = true
			default:
				continue
			}
			cell := slices.Max(slices.Collect(maps.Values(open)))
			calls[cell].nested = append(calls[cell].nested, nested)
		}
	}
	return calls
}

// shellCommandOf returns the command line a CommandExecution's argv runs:
// the script of a `<shell> -c` or `<shell> -lc` argv, and the argv joined by spaces otherwise.
func shellCommandOf(argv []string) string {
	if n := len(argv); n >= 2 && (argv[n-2] == "-c" || argv[n-2] == "-lc") {
		return argv[n-1]
	}
	return strings.Join(argv, " ")
}

// localPath returns the local path a CommandExecution's cwd names, a file:// URL or a plain path.
// On Windows a file URL's drive path, such as /C:/x, loses its leading slash so the result keeps its volume.
func localPath(cwd string) string {
	if parsed, err := url.Parse(cwd); err == nil && parsed.Scheme == "file" {
		path := parsed.Path
		if runtime.GOOS == "windows" && len(path) >= 3 && path[0] == '/' && path[2] == ':' && path[1]|0x20 >= 'a' && path[1]|0x20 <= 'z' {
			path = path[1:]
		}
		return filepath.FromSlash(path)
	}
	return cwd
}

// resolvePath returns path as an absolute, cleaned path, resolved against cwd when it is relative.
// It returns "" for a relative path with no cwd to resolve it against.
func resolvePath(cwd, path string) string {
	if filepath.IsAbs(path) {
		return filepath.Clean(path)
	}
	if cwd == "" {
		return ""
	}
	return filepath.Join(cwd, path)
}

// latestExecCell returns the index of the last exec cell of calls, or -1 when calls hold none.
func latestExecCell(calls []topCall) int {
	for i := len(calls) - 1; i >= 0; i-- {
		if calls[i].exec {
			return i
		}
	}
	return -1
}

// withCall returns nested with call added, unless an item of the same id is already there.
// A recorded item carries the call's cwd and reads, which the hook payload lacks.
func withCall(nested []nestedCall, call nestedCall) []nestedCall {
	if slices.ContainsFunc(nested, func(n nestedCall) bool { return n.id == call.id }) {
		return nested
	}
	return append(slices.Clone(nested), call)
}

// holdsItem reports whether a cell of calls holds the item id.
func holdsItem(calls []topCall, id string) bool {
	return slices.ContainsFunc(calls, func(call topCall) bool {
		return slices.ContainsFunc(call.nested, func(n nestedCall) bool { return n.id == id })
	})
}

// cellKind classes a top-level call by the calls a batching breach is made of.
type cellKind int

const (
	// kindOther is a call that makes no chain: a call that is no exec cell, a cell with no nested call, or a mixed cell.
	kindOther cellKind = iota
	// kindLookup is an exec cell whose only nested call is one read-only lookup.
	kindLookup
	// kindEdits is an exec cell every nested call of which is an apply_patch call.
	kindEdits
)

// kindOf classes call.
func kindOf(call topCall) cellKind {
	if !call.exec || len(call.nested) == 0 {
		return kindOther
	}
	if len(call.nested) == 1 && call.nested[0].lookup() {
		return kindLookup
	}
	for _, nested := range call.nested {
		if !nested.patch {
			return kindOther
		}
	}
	return kindEdits
}

// runEndingAt counts the consecutive calls of kind that end at index last.
func runEndingAt(calls []topCall, last int, kind cellKind) int {
	run := 0
	for i := last; i >= 0 && kindOf(calls[i]) == kind; i-- {
		run++
	}
	return run
}

// BatchingNudge judges the nested tool call a Codex PostToolUse input describes.
// It returns the hook output that adds one nudge line to the calling thread's context, or nil when no nudge is due.
// Codex fires the hook once per nested call of an exec cell, and the call belongs to the rollout's latest exec cell.
// One top-level call counts as one message does on Claude, and the rules are Claude's:
// a run of single-lookup cells, a line-range read of a short file or of one an earlier cell read and unchanged since,
// and a run of apply_patch-only cells.
// Each thread's count and cap are keyed by the payload's transcript_path.
// A payload whose tool is neither Bash nor apply_patch gets no nudge.
// Nor does a Bash call that is no read-only lookup.
// Both leave the rollout and the state file unread.
// A run with no state file, a thread below the context threshold and a thread past its nudge cap get none either.
// Nor does a cell already nudged.
// A line-range read waits a bounded time for its own item, which carries the directory its path is resolved against;
// a read whose item is still missing at the bound gets no read-slice nudge.
// The judgment and the count update hold the state file's lock, so the nested calls of one cell nudge at most once.
// A rollout that cannot be read gets no nudge and no error, since its format is a Codex internal.
// A malformed input or state file is an error.
func (c *Codex) BatchingNudge(statePath string, payload []byte) ([]byte, error) {
	var input batchingHookInput
	if err := json.Unmarshal(payload, &input); err != nil {
		return nil, fmt.Errorf("codexengine: parse the batching hook input: %w", err)
	}
	if input.TranscriptPath == "" || input.ToolUseID == "" {
		return nil, nil
	}
	current := nestedCall{id: input.ToolUseID, patch: input.ToolName == toolNameApplyPatch, command: input.ToolInput.Command}
	if !current.patch && (input.ToolName != toolMatcherBash || !current.lookup()) {
		return nil, nil
	}
	// The state is read before the wait, so a run with no state file and a thread past its cap never wait for the rollout.
	seeded, found, err := state.ReadJSON[batchingNudgeState](statePath, batchingNudgeLockPath(statePath))
	if err != nil {
		return nil, fmt.Errorf("codexengine: read the batching nudge state %s: %w", statePath, err)
	}
	if !found || seeded.Threads[input.TranscriptPath].Nudges >= seeded.Settings.MaxNudges {
		return nil, nil
	}
	sliceFile, sliced := "", false
	if seeded.Settings.ReadSlice && !current.patch {
		sliceFile, sliced = bashlookup.LineRangeFile(current.command)
	}
	tail, ok := c.awaitRollout(input.TranscriptPath, input.ToolUseID, sliced, seeded.Settings.ContextTokens)
	if !ok {
		return nil, nil
	}

	var line string
	err = state.UpdateJSON(statePath, batchingNudgeLockPath(statePath), func(cur batchingNudgeState, found bool) (batchingNudgeState, error) {
		count := cur.Threads[input.TranscriptPath]
		if !found || count.Nudges >= cur.Settings.MaxNudges {
			return cur, errNoNudge
		}
		calls := rolloutCalls(tail)
		cell := latestExecCell(calls)
		if cell < 0 || calls[cell].callID == count.NudgedCell {
			return cur, errNoNudge
		}
		calls[cell].nested = withCall(calls[cell].nested, current)
		line = judgeBatching(cur.Settings, calls, cell, input.ToolUseID, sliceFile, sliced)
		if line == "" {
			return cur, errNoNudge
		}
		return cur.withCount(input.TranscriptPath, batchingNudgeCount{Nudges: count.Nudges + 1, NudgedCell: calls[cell].callID}), nil
	})
	if errors.Is(err, errNoNudge) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("codexengine: update the batching nudge state %s: %w", statePath, err)
	}
	return batchingHookOutput(line)
}

// awaitRollout returns the tail of the rollout at rolloutPath for the judgment.
// It reports false at once for a rollout that cannot be read.
// It reports false at once too for a thread whose context is below contextTokens or unknown.
// The context is the first tail's newest token_count.
// With needItem it rereads the tail, only when the file's size changed, until a cell holds the item toolUseID.
// It returns the last tail it read when the wait's bound passes first.
func (c *Codex) awaitRollout(rolloutPath, toolUseID string, needItem bool, contextTokens int) ([]byte, bool) {
	wait := c.rolloutWait
	readSize := int64(-1)
	var tail []byte
	for attempt := 0; ; attempt++ {
		info, err := os.Stat(rolloutPath)
		if err != nil {
			return nil, false
		}
		if info.Size() != readSize {
			readSize = info.Size()
			tail, err = shuttleengine.ReadFileTail(rolloutPath, batchingTailBytes)
			if err != nil {
				return nil, false
			}
			if attempt == 0 {
				if reading := latestContext(parseRolloutEvents(tail)); !reading.Known || reading.Tokens < contextTokens {
					return nil, false
				}
			}
			if !needItem || holdsItem(rolloutCalls(tail), toolUseID) {
				return tail, true
			}
		}
		if attempt >= wait.attempts {
			return tail, true
		}
		wait.sleep(wait.tick)
	}
}

// judgeBatching returns the nudge line for the first breach the call toolUseID in cell makes, or "" for none.
// It checks a single-lookup chain, then a read slice of sliceFile when sliced, then an edit-only chain.
// Each is checked only while its switch is on.
func judgeBatching(settings BatchingNudgeConfig, calls []topCall, cell int, toolUseID, sliceFile string, sliced bool) string {
	kind := kindOf(calls[cell])
	if settings.LookupChain && kind == kindLookup {
		if run := runEndingAt(calls, cell, kindLookup); run >= settings.LookupRun {
			return fmt.Sprintf(lookupChainNudge, run)
		}
	}
	if sliced {
		index := slices.IndexFunc(calls[cell].nested, func(n nestedCall) bool { return n.id == toolUseID })
		if reason := sliceBreach(settings, calls, cell, calls[cell].nested[index], sliceFile); reason != "" {
			return fmt.Sprintf(readSliceNudge, sliceFile, reason)
		}
	}
	if settings.EditChain && kind == kindEdits {
		if run := runEndingAt(calls, cell, kindEdits); run >= settings.EditRun {
			return fmt.Sprintf(editChainNudge, run)
		}
	}
	return ""
}

// sliceBreach returns why the line-range read current of sliceFile, in cell, breaks the rule, or "" when it does not.
// The file, resolved against the read's own cwd, holds fewer than the settings' slice lines,
// or the latest earlier cell holding a shell call that read it started no earlier than the file's last modification.
// An earlier read's path is resolved against that call's own cwd.
// A read whose item is not recorded, and a file that cannot be read or statted, are no breach.
func sliceBreach(settings BatchingNudgeConfig, calls []topCall, cell int, current nestedCall, sliceFile string) string {
	if !current.recorded {
		return ""
	}
	path := resolvePath(current.cwd, sliceFile)
	if path == "" {
		return ""
	}
	lines, err := shuttleengine.CountLinesUpTo(path, settings.SliceLines)
	if err != nil {
		return ""
	}
	if lines < settings.SliceLines {
		return fmt.Sprintf(sliceShortReason, settings.SliceLines)
	}
	info, err := os.Stat(path)
	if err != nil {
		return ""
	}
	for i := cell - 1; i >= 0; i-- {
		for _, nested := range calls[i].nested {
			if !slices.ContainsFunc(nested.reads, func(read string) bool { return resolvePath(nested.cwd, read) == path }) {
				continue
			}
			if !calls[i].at.IsZero() && !info.ModTime().After(calls[i].at) {
				return sliceReadReason
			}
			return ""
		}
	}
	return ""
}

// batchingHookOutput renders the PostToolUse output that hands line to the model as additional context.
// The line is also the system message the operator sees.
// Codex never reads the output as a failed call, as it would a block.
func batchingHookOutput(line string) ([]byte, error) {
	type specificOutput struct {
		HookEventName     string `json:"hookEventName"`
		AdditionalContext string `json:"additionalContext"`
	}
	data, err := json.Marshal(struct {
		SystemMessage      string         `json:"systemMessage"`
		HookSpecificOutput specificOutput `json:"hookSpecificOutput"`
	}{SystemMessage: line, HookSpecificOutput: specificOutput{HookEventName: hookEventPostToolUse, AdditionalContext: line}})
	if err != nil {
		return nil, fmt.Errorf("codexengine: marshal the batching hook output: %w", err)
	}
	return data, nil
}
