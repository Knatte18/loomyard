// batchingnudge_test.go pins the batching nudge.
// It covers which breach a transcript tail fires and what holds it back, and the count its state file keeps.
// It also covers the transcript tail read, the bounds of claude.yaml's batching_nudge block, and the hook and state file Prepare installs from it.

package claudeengine

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/configengine"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
	"github.com/Knatte18/loomyard/internal/state"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// nudgeFixtureBase is the time the fixture transcripts and files are dated from.
var nudgeFixtureBase = time.Date(2026, 11, 2, 12, 0, 0, 0, time.UTC)

// The entries of a fixture message that are not a call, and the one-entry messages that stand for a line the nudge must skip.
const (
	// nudgeLineBreak starts a new transcript line of the same message, as Claude Code writes one line per content block.
	nudgeLineBreak = "~"
	// nudgeTextBlock adds a text block, which holds no call.
	nudgeTextBlock = "text"
	// nudgeSidechain is a message written as a sidechain line holding one Read.
	nudgeSidechain = "!sidechain"
	// nudgeNoID is a message written as a line holding one Read and no message id.
	nudgeNoID = "!noid"
)

// nudgeCallID names call j of fixture message i, the calls counted without the non-call entries.
func nudgeCallID(message, call int) string {
	return fmt.Sprintf("call-%d-%d", message, call)
}

// firstNudgeCall returns the first entry of message that is a call.
func firstNudgeCall(message []string) string {
	for _, entry := range message {
		if entry != nudgeLineBreak && entry != nudgeTextBlock {
			return entry
		}
	}
	return ""
}

// writeNudgeTranscript writes a transcript into dir holding the main-chain assistant lines of each message, each message followed by a user line answering its first call, and returns its path.
// A call is written "Tool" or "Tool:file", the file named relative to dir; message i is dated i+1 minutes after nudgeFixtureBase and reports tokens of context.
// A message is one line unless it holds nudgeLineBreak entries, and the one-entry messages nudgeSidechain and nudgeNoID are written as the lines their names say.
func writeNudgeTranscript(t *testing.T, dir string, tokens int, messages [][]string) string {
	t.Helper()
	var lines []string
	appendRecord := func(record map[string]any) {
		data, err := json.Marshal(record)
		if err != nil {
			t.Fatalf("marshal transcript line: %v", err)
		}
		lines = append(lines, string(data))
	}
	for i, message := range messages {
		id, sidechain := fmt.Sprintf("msg-%d", i), false
		switch {
		case slices.Equal(message, []string{nudgeSidechain}):
			message, sidechain = []string{"Read:short.go"}, true
		case slices.Equal(message, []string{nudgeNoID}):
			message, id = []string{"Read:short.go"}, ""
		}
		lineBlocks := [][]map[string]any{nil}
		calls := 0
		for _, entry := range message {
			var block map[string]any
			switch entry {
			case nudgeLineBreak:
				lineBlocks = append(lineBlocks, nil)
				continue
			case nudgeTextBlock:
				block = map[string]any{"type": "text", "text": "thinking aloud"}
			default:
				name, file, _ := strings.Cut(entry, ":")
				input := map[string]any{}
				if file != "" {
					input["file_path"] = filepath.Join(dir, file)
				}
				block = map[string]any{"type": "tool_use", "id": nudgeCallID(i, calls), "name": name, "input": input}
				calls++
			}
			lineBlocks[len(lineBlocks)-1] = append(lineBlocks[len(lineBlocks)-1], block)
		}
		for _, blocks := range lineBlocks {
			content := map[string]any{"content": blocks, "usage": map[string]any{"input_tokens": tokens}}
			if id != "" {
				content["id"] = id
			}
			appendRecord(map[string]any{
				"type":        "assistant",
				"isSidechain": sidechain,
				"timestamp":   nudgeFixtureBase.Add(time.Duration(i+1) * time.Minute).Format(time.RFC3339Nano),
				"message":     content,
			})
		}
		appendRecord(map[string]any{
			"type":    "user",
			"message": map[string]any{"content": []map[string]any{{"type": "tool_result", "tool_use_id": nudgeCallID(i, 0), "content": "an assistant result"}}},
		})
	}
	transcript := filepath.Join(dir, "transcript.jsonl")
	if err := os.WriteFile(transcript, []byte(strings.Join(lines, "\n")+"\n"), 0o644); err != nil {
		t.Fatalf("write transcript: %v", err)
	}
	return transcript
}

// writeNudgeFile writes a file of lines lines named name into dir, last modified at modified.
func writeNudgeFile(t *testing.T, dir, name string, lines int, modified time.Time) {
	t.Helper()
	filePath := filepath.Join(dir, name)
	if err := os.WriteFile(filePath, []byte(strings.Repeat("line\n", lines)), 0o644); err != nil {
		t.Fatalf("write %s: %v", name, err)
	}
	if err := os.Chtimes(filePath, modified, modified); err != nil {
		t.Fatalf("date %s: %v", name, err)
	}
}

// TestBatchingNudge pins which breach a call fires, from the call's own input and the transcript tail, and what holds a nudge back.
// A fired nudge counts in the state file and records its message; a silent call leaves the state file as it was.
func TestBatchingNudge(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()
	settings := BatchingNudgeConfig{Enabled: true, LookupChain: true, ReadSlice: true, EditChain: true, ContextTokens: 150000, LookupRun: 3, EditRun: 2, SliceLines: 20, MaxNudges: 2}
	lookupChain := [][]string{{"Read:short.go"}, {"Grep"}, {"Glob"}}
	readLongAgain := [][]string{{"Read:long.go"}, {"Bash"}, {"Read:long.go"}}
	shortRead := [][]string{{"Bash"}, {"Read:short.go"}}
	offsetTen, limitFive, offsetOne := 10, 5, 1
	shortSliceNudge := func(dir string) string {
		return fmt.Sprintf(readSliceNudge, filepath.Join(dir, "short.go"), fmt.Sprintf(sliceShortReason, 20))
	}
	lookupNudge := func(string) string { return fmt.Sprintf(lookupChainNudge, 3) }
	tests := []struct {
		name     string
		tokens   int
		messages [][]string
		// offset and limit are the current call's Read arguments, the last message's first call; nil leaves one out.
		offset, limit *int
		// longModified is how long after nudgeFixtureBase long.go was last modified.
		longModified time.Duration
		subagent     bool
		noState      bool
		// corruptState seeds a state file that is not JSON.
		corruptState bool
		before       func(*batchingNudgeState)
		// input changes the hook input before it is sent; dir is the fixture directory.
		input func(dir string, hookInput map[string]any)
		// payload replaces the hook input outright.
		payload []byte
		wantErr bool
		// want returns the nudge line for the fixture directory; nil wants no nudge.
		want func(dir string) string
	}{
		{name: "LookupChainFires", tokens: 200000, messages: lookupChain, want: lookupNudge},
		{name: "LookupChainShorterThanTheRunIsSilent", tokens: 200000, messages: [][]string{{"Bash"}, {"Read:short.go"}, {"Grep"}}},
		{name: "BatchedCurrentMessageIsSilent", tokens: 200000, messages: [][]string{{"Read:short.go"}, {"Grep"}, {"Read:short.go", "Read:long.go"}}},
		{name: "BelowTheContextThresholdIsSilent", tokens: 100000, messages: lookupChain},
		{name: "LookupChainSplitAcrossLinesStillCountsAsOneMessage", tokens: 200000, messages: [][]string{{"Read:short.go"}, {"Grep"}, {nudgeTextBlock, nudgeLineBreak, "Glob"}}, want: lookupNudge},
		{name: "SidechainLineDoesNotBreakTheChain", tokens: 200000, messages: [][]string{{"Read:short.go"}, {nudgeSidechain}, {"Grep"}, {"Glob"}}, want: lookupNudge},
		{name: "SidechainLineIsNotCountedInTheChain", tokens: 200000, messages: [][]string{{"Read:short.go"}, {nudgeSidechain}, {"Grep"}}},
		{name: "LineWithoutAMessageIDIsNotCountedInTheChain", tokens: 200000, messages: [][]string{{"Read:short.go"}, {nudgeNoID}, {"Grep"}}},
		{name: "SlicedReadOfAShortFileFires", tokens: 200000, messages: shortRead, offset: &offsetTen, limit: &limitFive, want: shortSliceNudge},
		{name: "SlicedReadByLimitAloneFires", tokens: 200000, messages: shortRead, limit: &limitFive, want: shortSliceNudge},
		{name: "SlicedReadByOffsetPastTheFirstLineAloneFires", tokens: 200000, messages: shortRead, offset: &offsetTen, want: shortSliceNudge},
		{name: "ReadFromTheFirstLineAloneIsSilent", tokens: 200000, messages: shortRead, offset: &offsetOne},
		{name: "WholeReadOfAShortFileIsSilent", tokens: 200000, messages: shortRead},
		{name: "SlicedReadOfAFileAtExactlyTheSliceLinesIsSilent", tokens: 200000, messages: [][]string{{"Bash"}, {"Read:exact.go"}}, offset: &offsetTen, limit: &limitFive},
		{name: "LastLineWithoutANewlineCounts", tokens: 200000, messages: [][]string{{"Bash"}, {"Read:unterminated.go"}}, offset: &offsetTen, limit: &limitFive},
		{name: "SlicedReadOfAnUnreadLongFileIsSilent", tokens: 200000, messages: [][]string{{"Bash"}, {"Read:long.go"}}, offset: &offsetTen, limit: &limitFive},
		{
			name: "SlicedReadOfAnUnchangedFileReadBeforeFires", tokens: 200000, messages: readLongAgain, offset: &offsetTen, limit: &limitFive,
			want: func(dir string) string {
				return fmt.Sprintf(readSliceNudge, filepath.Join(dir, "long.go"), sliceReadReason)
			},
		},
		{name: "SlicedReadOfAFileChangedSinceIsSilent", tokens: 200000, messages: readLongAgain, offset: &offsetTen, limit: &limitFive, longModified: 30 * time.Minute},
		{
			name: "ReadSliceSwitchedOffIsSilent", tokens: 200000, messages: shortRead, offset: &offsetTen, limit: &limitFive,
			before: func(s *batchingNudgeState) { s.Settings.ReadSlice = false },
		},
		{name: "LookupChainOutranksReadSlice", tokens: 200000, messages: [][]string{{"Read:short.go"}, {"Grep"}, {"Read:short.go"}}, offset: &offsetTen, limit: &limitFive, want: lookupNudge},
		{name: "EditChainFires", tokens: 200000, messages: [][]string{{"Edit:short.go"}, {"Write:long.go"}}, want: func(string) string { return fmt.Sprintf(editChainNudge, 2) }},
		{name: "MessageOfSeveralEditsCountsAsOneEditMessage", tokens: 200000, messages: [][]string{{"Edit:short.go"}, {"Edit:short.go", "Edit:long.go"}}, want: func(string) string { return fmt.Sprintf(editChainNudge, 2) }},
		{name: "EditMessageAlsoHoldingAReadIsSilent", tokens: 200000, messages: [][]string{{"Edit:short.go"}, {"Edit:short.go", "Read:long.go"}}},
		{name: "EditAfterALookupIsSilent", tokens: 200000, messages: [][]string{{"Read:short.go"}, {"Edit:short.go"}}},
		{
			name: "EditChainSwitchedOffIsSilent", tokens: 200000, messages: [][]string{{"Edit:short.go"}, {"Write:long.go"}},
			before: func(s *batchingNudgeState) { s.Settings.EditChain = false },
		},
		{name: "SubagentCallIsSilent", tokens: 200000, messages: lookupChain, subagent: true},
		{name: "NudgeCapReachedIsSilent", tokens: 200000, messages: lookupChain, before: func(s *batchingNudgeState) { s.Nudges = 2 }},
		{name: "MessageAlreadyNudgedIsSilent", tokens: 200000, messages: lookupChain, before: func(s *batchingNudgeState) { s.Nudges, s.NudgedMessage = 1, "msg-2" }},
		{name: "SwitchedOffBreachIsSilent", tokens: 200000, messages: lookupChain, before: func(s *batchingNudgeState) { s.Settings.LookupChain = false }},
		{name: "NoStateFileIsSilent", tokens: 200000, messages: lookupChain, noState: true},
		{name: "MalformedPayloadIsAnError", tokens: 200000, messages: lookupChain, payload: []byte("{not json"), wantErr: true},
		{name: "EmptyTranscriptPathIsSilent", tokens: 200000, messages: lookupChain, input: func(_ string, in map[string]any) { in["transcript_path"] = "" }},
		{name: "EmptyToolUseIDIsSilent", tokens: 200000, messages: lookupChain, input: func(_ string, in map[string]any) { in["tool_use_id"] = "" }},
		{
			name: "UnreadableTranscriptIsSilent", tokens: 200000, messages: lookupChain,
			input: func(dir string, in map[string]any) { in["transcript_path"] = filepath.Join(dir, "missing.jsonl") },
		},
		{name: "ToolUseIDAbsentFromTheTailIsSilent", tokens: 200000, messages: lookupChain, input: func(_ string, in map[string]any) { in["tool_use_id"] = "call-absent" }},
		{name: "CorruptStateFileIsAnError", tokens: 200000, messages: lookupChain, corruptState: true, wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			dir := t.TempDir()
			writeNudgeFile(t, dir, "short.go", 5, nudgeFixtureBase)
			writeNudgeFile(t, dir, "long.go", 40, nudgeFixtureBase.Add(tt.longModified))
			writeNudgeFile(t, dir, "exact.go", 20, nudgeFixtureBase)
			if err := os.WriteFile(filepath.Join(dir, "unterminated.go"), []byte(strings.Repeat("line\n", 19)+"line"), 0o644); err != nil {
				t.Fatalf("write unterminated.go: %v", err)
			}
			transcript := writeNudgeTranscript(t, dir, tt.tokens, tt.messages)
			statePath := filepath.Join(dir, batchingNudgeStateFileName)
			before := batchingNudgeState{Settings: settings}
			if tt.before != nil {
				tt.before(&before)
			}
			switch {
			case tt.corruptState:
				if err := os.WriteFile(statePath, []byte("{not json"), 0o644); err != nil {
					t.Fatalf("seed corrupt state: %v", err)
				}
			case !tt.noState:
				if err := state.WriteJSON(statePath, batchingNudgeLockPath(statePath), before); err != nil {
					t.Fatalf("seed state: %v", err)
				}
			}

			last := len(tt.messages) - 1
			name, file, _ := strings.Cut(firstNudgeCall(tt.messages[last]), ":")
			toolInput := map[string]any{}
			if file != "" {
				toolInput["file_path"] = filepath.Join(dir, file)
			}
			if tt.offset != nil {
				toolInput["offset"] = *tt.offset
			}
			if tt.limit != nil {
				toolInput["limit"] = *tt.limit
			}
			hookInput := map[string]any{"hook_event_name": "PostToolUse", "transcript_path": transcript, "tool_name": name, "tool_use_id": nudgeCallID(last, 0), "tool_input": toolInput}
			if tt.subagent {
				hookInput["agent_id"] = "fork-1"
			}
			if tt.input != nil {
				tt.input(dir, hookInput)
			}
			payload := tt.payload
			if payload == nil {
				var err error
				if payload, err = json.Marshal(hookInput); err != nil {
					t.Fatalf("marshal hook input: %v", err)
				}
			}

			got, err := New().BatchingNudge(statePath, payload)
			if tt.wantErr {
				if err == nil {
					t.Errorf("BatchingNudge() error = nil; want an error")
				}
				if got != nil {
					t.Errorf("BatchingNudge() = %s; want no output with the error", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("BatchingNudge() error: %v", err)
			}
			wantAfter := before
			if tt.want == nil {
				if got != nil {
					t.Errorf("BatchingNudge() = %s; want no output", got)
				}
			} else {
				want, err := hookContext(hookEventPostToolUse, tt.want(dir))
				if err != nil {
					t.Fatalf("hookContext: %v", err)
				}
				if string(got) != string(want) {
					t.Errorf("BatchingNudge() = %s; want %s", got, want)
				}
				wantAfter.Nudges++
				wantAfter.NudgedMessage = fmt.Sprintf("msg-%d", last)
			}

			after, found, err := state.ReadJSON[batchingNudgeState](statePath, batchingNudgeLockPath(statePath))
			if err != nil {
				t.Fatalf("read state: %v", err)
			}
			if tt.noState {
				if found {
					t.Errorf("state file = %+v; want none written", after)
				}
				return
			}
			if after != wantAfter {
				t.Errorf("state after = %+v; want %+v", after, wantAfter)
			}
		})
	}
}

// TestReadTranscriptTail pins the window of the transcript the nudge reads: a file under the budget whole, a larger one cut to its complete lines, and nothing when the window holds no line end.
func TestReadTranscriptTail(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()
	tests := []struct {
		name    string
		content string
		budget  int64
		want    string
		wantNil bool
	}{
		{name: "FileUnderTheBudgetIsReadWhole", content: "aaa\nbbb\nccc\n", budget: 100, want: "aaa\nbbb\nccc\n"},
		{name: "FileOverTheBudgetDropsTheCutFirstLine", content: "aaa\nbbb\nccc\n", budget: 6, want: "ccc\n"},
		{name: "WindowWithoutALineEndIsNil", content: "aaa\nbbb", budget: 2, wantNil: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			transcript := filepath.Join(t.TempDir(), "transcript.jsonl")
			if err := os.WriteFile(transcript, []byte(tt.content), 0o644); err != nil {
				t.Fatalf("write transcript: %v", err)
			}
			got, err := readTranscriptTail(transcript, tt.budget)
			if err != nil {
				t.Fatalf("readTranscriptTail() error: %v", err)
			}
			if tt.wantNil {
				if got != nil {
					t.Errorf("readTranscriptTail(%q, %d) = %q; want nil", tt.content, tt.budget, got)
				}
				return
			}
			if string(got) != tt.want {
				t.Errorf("readTranscriptTail(%q, %d) = %q; want %q", tt.content, tt.budget, got, tt.want)
			}
		})
	}
}

// TestValidateBatchingNudge pins the bounds of the batching_nudge block: context_tokens may be 0, the other numbers must be at least 1, and a block that is off is not checked.
func TestValidateBatchingNudge(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()
	tests := []struct {
		name   string
		mutate func(*BatchingNudgeConfig)
		// wantKey is the key the refusal names; empty wants the block accepted.
		wantKey string
	}{
		{name: "ValidBlockIsAccepted"},
		{name: "ZeroContextTokensIsAccepted", mutate: func(n *BatchingNudgeConfig) { n.ContextTokens = 0 }},
		{name: "NegativeContextTokensIsRefused", mutate: func(n *BatchingNudgeConfig) { n.ContextTokens = -1 }, wantKey: "batching_nudge.context_tokens"},
		{name: "ZeroLookupRunIsRefused", mutate: func(n *BatchingNudgeConfig) { n.LookupRun = 0 }, wantKey: "batching_nudge.lookup_run"},
		{name: "ZeroEditRunIsRefused", mutate: func(n *BatchingNudgeConfig) { n.EditRun = 0 }, wantKey: "batching_nudge.edit_run"},
		{name: "ZeroSliceLinesIsRefused", mutate: func(n *BatchingNudgeConfig) { n.SliceLines = 0 }, wantKey: "batching_nudge.slice_lines"},
		{name: "ZeroMaxNudgesIsRefused", mutate: func(n *BatchingNudgeConfig) { n.MaxNudges = 0 }, wantKey: "batching_nudge.max_nudges"},
		{name: "DisabledBlockWithBadValuesIsAccepted", mutate: func(n *BatchingNudgeConfig) { *n = BatchingNudgeConfig{ContextTokens: -1} }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			nudge := BatchingNudgeConfig{Enabled: true, ContextTokens: 150000, LookupRun: 3, EditRun: 2, SliceLines: 20, MaxNudges: 2}
			if tt.mutate != nil {
				tt.mutate(&nudge)
			}
			err := ValidateBatchingNudge(Config{BatchingNudge: nudge})
			if tt.wantKey == "" {
				if err != nil {
					t.Errorf("ValidateBatchingNudge() error = %v; want the block accepted", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tt.wantKey) || !strings.Contains(err.Error(), "lym config claude") {
				t.Errorf("ValidateBatchingNudge() error = %v; want a refusal naming %s and lym config claude", err, tt.wantKey)
			}
		})
	}
}

// TestPrepare_BatchingNudgeHook pins that the template's batching_nudge installs the hook naming the run's state file and seeds that file with the block.
// An off switch installs neither, and a threshold below its least value refuses the run before any artifact.
func TestPrepare_BatchingNudgeHook(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()
	template, err := LoadConfig(configengine.Dirs{Anchor: t.TempDir()})
	if err != nil {
		t.Fatalf("load the shipped claude template: %v", err)
	}
	tests := []struct {
		name     string
		mutate   func(*BatchingNudgeConfig)
		wantHook bool
		// wantErr is a text the refusal names; empty wants no refusal.
		wantErr string
	}{
		{name: "OnInstallsTheHookAndSeedsTheState", wantHook: true},
		{name: "OffInstallsNeither", mutate: func(n *BatchingNudgeConfig) { n.Enabled = false }},
		{name: "AThresholdBelowItsLeastRefusesBeforeArtifacts", mutate: func(n *BatchingNudgeConfig) { n.LookupRun = 0 }, wantErr: "batching_nudge.lookup_run"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			claude := templateConfig(t)
			claude.BatchingNudge = template.BatchingNudge
			if tt.mutate != nil {
				tt.mutate(&claude.BatchingNudge)
			}
			runDir := t.TempDir()
			_, err := engineWith(t, claude).Prepare(runDir, shuttleengine.Spec{Prompt: "do the thing"}, shuttleengine.Config{})
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) || !strings.Contains(err.Error(), "lym config claude") {
					t.Errorf("Prepare() error = %v; want a refusal naming %s and lym config claude", err, tt.wantErr)
				}
				if entries, readErr := os.ReadDir(runDir); readErr != nil || len(entries) != 0 {
					t.Errorf("ReadDir(%s) = %d entries, err=%v; want no artifacts after a refused Prepare", runDir, len(entries), readErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("Prepare() error: %v", err)
			}

			statePath := filepath.Join(runDir, batchingNudgeStateFileName)
			hookStatePath := statePath
			if runtime.GOOS == "windows" {
				if hookStatePath, err = shuttleengine.PosixPath(statePath); err != nil {
					t.Fatal(err)
				}
			}
			settingsBytes, err := os.ReadFile(filepath.Join(runDir, "settings.json"))
			if err != nil {
				t.Fatalf("read settings.json: %v", err)
			}
			var got []string
			for _, raw := range hooksFor(parseSettings(t, settingsBytes), "PostToolUse") {
				entry, _ := raw.(map[string]any)
				if entry["matcher"] != batchingNudgeMatcher {
					continue
				}
				hooks, _ := entry["hooks"].([]any)
				for _, hook := range hooks {
					command, _ := hook.(map[string]any)["command"].(string)
					got = append(got, command)
				}
			}
			var want []string
			if tt.wantHook {
				want = []string{batchingNudgeHookCommand(shQuote(hookStatePath))}
			}
			if !slices.Equal(got, want) {
				t.Errorf("batching hook commands = %q; want %q", got, want)
			}

			seeded, found, err := state.ReadJSON[batchingNudgeState](statePath, batchingNudgeLockPath(statePath))
			if err != nil {
				t.Fatalf("read state: %v", err)
			}
			if found != tt.wantHook || (found && seeded != (batchingNudgeState{Settings: claude.BatchingNudge})) {
				t.Errorf("state file = %+v, found=%t; want found=%t holding the block %+v", seeded, found, tt.wantHook, claude.BatchingNudge)
			}
		})
	}
}
