// batchingnudge_test.go pins the batching nudge: which breach a transcript tail fires, and what holds it back.
// It covers the main session and subagents' transcripts, the state file's counts, and the transcript tail read.
// It also covers the wait for a call to reach the transcript.
// It also covers the bounds of claude.yaml's batching_nudge block and the hook and state file Prepare installs from it.

package claudeengine

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
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

// Fixture message markers: entries that are not calls, and one-entry messages that write a named kind of line.
const (
	// nudgeLineBreak starts a new transcript line of the same message, as Claude Code writes one line per content block.
	nudgeLineBreak = "~"
	// nudgeTextBlock adds a text block, which holds no call.
	nudgeTextBlock = "text"
	// nudgeSidechain is a message written as a sidechain line holding one Read.
	nudgeSidechain = "!sidechain"
	// nudgeNoID is a message written as a line holding one Read and no message id.
	nudgeNoID = "!noid"
	// nudgeMainChain is a message written as a main-chain line holding one Read and a context reading of one token.
	// It stays a main-chain line even in a subagent's transcript.
	nudgeMainChain = "!mainchain"
)

// nudgeCallID returns the ID of call j of fixture message i, counting only the calls and not the non-call entries.
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

// nudgeAgentID is the agent id of the fixture subagent.
const nudgeAgentID = "fork-1"

// nudgeWaitAttempts is the fixture engine's bound on its waits for a call to reach the transcript.
const nudgeWaitAttempts = 3

// nudgeAgentTranscript returns the path of agent agentID's transcript beside the fixture session transcript in dir.
func nudgeAgentTranscript(dir, agentID string) string {
	return filepath.Join(dir, "transcript", subagentsDirName, subagentTranscriptPrefix+agentID+".jsonl")
}

// nudgeCallInput returns the tool name and tool input of a fixture call written "Tool" or "Tool:arg":
// arg is a Bash call's command, and any other call's file, named relative to dir.
func nudgeCallInput(dir, entry string) (string, map[string]any) {
	name, arg, _ := strings.Cut(entry, ":")
	input := map[string]any{}
	switch {
	case arg == "":
	case name == toolBash:
		input["command"] = arg
	default:
		input["file_path"] = filepath.Join(dir, arg)
	}
	return name, input
}

// writeNudgeTranscript writes the transcript at path: the assistant lines of each message.
// Each message is followed by a user line that answers its first call.
// A call is written as a "Tool" or "Tool:arg" entry, with a file named relative to dir;
// message i is dated i+1 minutes after nudgeFixtureBase and reports tokens of context.
// Every assistant line is a sidechain line when sidechain is true, as in a subagent's transcript.
// A message is one line unless it holds a `nudgeLineBreak` entry.
// A message of one `nudgeSidechain`, `nudgeMainChain` or `nudgeNoID` entry is written as that kind of line.
func writeNudgeTranscript(t *testing.T, dir, path string, tokens int, sidechain bool, messages [][]string) {
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
		id, lineSidechain, lineTokens := fmt.Sprintf("msg-%d", i), sidechain, tokens
		switch {
		case slices.Equal(message, []string{nudgeSidechain}):
			message, lineSidechain = []string{"Read:short.go"}, true
		case slices.Equal(message, []string{nudgeMainChain}):
			message, lineSidechain, lineTokens = []string{"Read:short.go"}, false, 1
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
				name, input := nudgeCallInput(dir, entry)
				block = map[string]any{"type": "tool_use", "id": nudgeCallID(i, calls), "name": name, "input": input}
				calls++
			}
			lineBlocks[len(lineBlocks)-1] = append(lineBlocks[len(lineBlocks)-1], block)
		}
		for _, blocks := range lineBlocks {
			content := map[string]any{"content": blocks, "usage": map[string]any{"input_tokens": lineTokens}}
			if id != "" {
				content["id"] = id
			}
			appendRecord(map[string]any{
				"type":        "assistant",
				"isSidechain": lineSidechain,
				"timestamp":   nudgeFixtureBase.Add(time.Duration(i+1) * time.Minute).Format(time.RFC3339Nano),
				"message":     content,
			})
		}
		appendRecord(map[string]any{
			"type":    "user",
			"message": map[string]any{"content": []map[string]any{{"type": "tool_result", "tool_use_id": nudgeCallID(i, 0), "content": "an assistant result"}}},
		})
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("create the transcript directory: %v", err)
	}
	if err := os.WriteFile(path, []byte(strings.Join(lines, "\n")+"\n"), 0o644); err != nil {
		t.Fatalf("write transcript: %v", err)
	}
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

// TestBatchingNudge pins which breach a call fires, from the call's own input and its chain's transcript tail.
// It also pins what holds a nudge back.
// A fired nudge counts in the state file under its chain, the main session's or the subagent's.
// It records its message there; a silent call leaves the state file as it was.
// A call its transcript does not hold yet is waited for, up to the bound, unless the chain would be silent anyway.
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
		// longModified is how long after `nudgeFixtureBase` the file long.go was last modified.
		longModified time.Duration
		// agentID is the hook input's agent id.
		// When it is set, the messages are written as that agent's transcript, beside a session transcript holding no chain.
		agentID string
		noState bool
		// corruptState seeds a state file that is not JSON.
		corruptState bool
		before       func(*batchingNudgeState)
		// input changes the hook input before it is sent; dir is the fixture directory.
		input func(dir string, hookInput map[string]any)
		// payload replaces the hook input outright.
		payload []byte
		// late leaves the last message out of the chain's transcript until the engine's lateAfter-th wait.
		// That is how Claude Code writes a message: after its calls start.
		late bool
		// lateAfter is the number of the wait at which a late message reaches the transcript; zero means the first.
		// Each earlier wait grows the transcript by one more sidechain message, which holds no call.
		// A main-chain transcript skips those lines, so a row with a lateAfter is a main-session row.
		lateAfter int
		// wantSleeps is how many times the engine waits for the call to reach the transcript.
		wantSleeps int
		// nudgedMessage is the message id a fired nudge records; empty means the last message's.
		nudgedMessage string
		wantErr       bool
		// want returns the nudge line for the fixture directory; nil wants no nudge.
		want func(dir string) string
	}{
		{name: "LookupChainFires", tokens: 200000, messages: lookupChain, want: lookupNudge},
		{name: "LookupChainShorterThanTheRunIsSilent", tokens: 200000, messages: [][]string{{"Bash"}, {"Read:short.go"}, {"Grep"}}},
		{name: "BatchedCurrentMessageIsSilent", tokens: 200000, messages: [][]string{{"Read:short.go"}, {"Grep"}, {"Read:short.go", "Read:long.go"}}},
		{name: "BelowTheContextThresholdIsSilent", tokens: 100000, messages: lookupChain},
		{name: "ReadOnlyBashCommandsCountAsLookups", tokens: 200000, messages: [][]string{{"Bash:grep -n x short.go"}, {"Read:short.go"}, {"Bash:git log --oneline | head -5"}}, want: lookupNudge},
		{name: "BashCommandThatIsNoLookupBreaksTheChain", tokens: 200000, messages: [][]string{{"Bash:grep -n x short.go"}, {"Bash:go vet ./internal/x"}, {"Grep"}, {"Glob"}}},
		{name: "BashCallThatIsNoLookupIsSilent", tokens: 200000, messages: [][]string{{"Read:short.go"}, {"Grep"}, {"Bash:lym board list"}}},
		// A corrupt state file is an error once it is read, so a silent answer shows the call returned before the state.
		{name: "BashCallThatIsNoLookupLeavesTheStateUnread", tokens: 200000, messages: [][]string{{"Read:short.go"}, {"Grep"}, {"Bash:lym board list"}}, corruptState: true},
		{name: "LookupChainSplitAcrossLinesStillCountsAsOneMessage", tokens: 200000, messages: [][]string{{"Read:short.go"}, {"Grep"}, {nudgeTextBlock, nudgeLineBreak, "Glob"}}, want: lookupNudge},
		{name: "SidechainLineDoesNotBreakTheChain", tokens: 200000, messages: [][]string{{"Read:short.go"}, {nudgeSidechain}, {"Grep"}, {"Glob"}}, want: lookupNudge},
		{name: "SidechainLineIsNotCountedInTheChain", tokens: 200000, messages: [][]string{{"Read:short.go"}, {nudgeSidechain}, {"Grep"}}},
		{name: "LineWithoutAMessageIDIsNotCountedInTheChain", tokens: 200000, messages: [][]string{{"Read:short.go"}, {nudgeNoID}, {"Grep"}}},
		{name: "SubagentMainChainLineIsNotCountedInTheChain", tokens: 200000, messages: [][]string{{"Read:short.go"}, {nudgeMainChain}, {"Grep"}}, agentID: nudgeAgentID},
		{
			// The main-chain line is the transcript's last assistant line and reads below the threshold,
			// so the agent's context reading is its earlier sidechain line.
			name: "SubagentMainChainLineIsNotItsContextReading", tokens: 200000, messages: [][]string{{"Read:short.go"}, {"Grep"}, {"Glob"}, {nudgeMainChain}}, agentID: nudgeAgentID,
			input: func(_ string, in map[string]any) { in["tool_use_id"] = nudgeCallID(2, 0) }, nudgedMessage: "msg-2", want: lookupNudge,
		},
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
		{name: "SubagentLookupChainFiresFromItsOwnTranscript", tokens: 200000, messages: lookupChain, agentID: nudgeAgentID, want: lookupNudge},
		{name: "SubagentEditChainFires", tokens: 200000, messages: [][]string{{"Edit:short.go"}, {"Write:long.go"}}, agentID: nudgeAgentID, want: func(string) string { return fmt.Sprintf(editChainNudge, 2) }},
		{name: "SubagentBelowTheContextThresholdByItsOwnReadingIsSilent", tokens: 100000, messages: lookupChain, agentID: nudgeAgentID},
		{name: "SubagentNudgesCountApartFromTheSession", tokens: 200000, messages: lookupChain, agentID: nudgeAgentID, before: func(s *batchingNudgeState) { s.Nudges = 2 }, want: lookupNudge},
		{
			name: "SubagentPastItsOwnCapIsSilent", tokens: 200000, messages: lookupChain, agentID: nudgeAgentID,
			before: func(s *batchingNudgeState) { *s = s.withCount(nudgeAgentID, batchingNudgeCount{Nudges: 2}) },
		},
		{
			name: "SubagentTranscriptPathNamingTheAgentFileIsReadAsIs", tokens: 200000, messages: lookupChain, agentID: nudgeAgentID,
			input: func(dir string, in map[string]any) { in["transcript_path"] = nudgeAgentTranscript(dir, nudgeAgentID) }, want: lookupNudge,
		},
		{name: "AgentIDHoldingASlashIsSilent", tokens: 200000, messages: lookupChain, agentID: "nested/agent"},
		{name: "AgentIDHoldingABackslashIsSilent", tokens: 200000, messages: lookupChain, agentID: `nested\agent`},
		{name: "AgentIDOfOneDotIsSilent", tokens: 200000, messages: lookupChain, agentID: "."},
		{name: "AgentIDOfTwoDotsIsSilent", tokens: 200000, messages: lookupChain, agentID: ".."},
		{name: "NudgeCapReachedIsSilent", tokens: 200000, messages: lookupChain, before: func(s *batchingNudgeState) { s.Nudges = 2 }},
		{name: "MessageAlreadyNudgedIsSilent", tokens: 200000, messages: lookupChain, before: func(s *batchingNudgeState) { s.Nudges, s.NudgedMessage = 1, "msg-2" }},
		{
			name: "SubagentMessageAlreadyNudgedIsSilent", tokens: 200000, messages: lookupChain, agentID: nudgeAgentID,
			before: func(s *batchingNudgeState) {
				*s = s.withCount(nudgeAgentID, batchingNudgeCount{Nudges: 1, NudgedMessage: "msg-2"})
			},
		},
		{name: "SwitchedOffBreachIsSilent", tokens: 200000, messages: lookupChain, before: func(s *batchingNudgeState) { s.Settings.LookupChain = false }},
		{name: "NoStateFileIsSilent", tokens: 200000, messages: lookupChain, noState: true},
		{name: "MalformedPayloadIsAnError", tokens: 200000, messages: lookupChain, payload: []byte("{not json"), wantErr: true},
		{name: "EmptyTranscriptPathIsSilent", tokens: 200000, messages: lookupChain, input: func(_ string, in map[string]any) { in["transcript_path"] = "" }},
		{name: "EmptyToolUseIDIsSilent", tokens: 200000, messages: lookupChain, input: func(_ string, in map[string]any) { in["tool_use_id"] = "" }},
		{
			name: "UnreadableTranscriptIsSilent", tokens: 200000, messages: lookupChain,
			input: func(dir string, in map[string]any) { in["transcript_path"] = filepath.Join(dir, "missing.jsonl") },
		},
		{
			name: "ToolUseIDAbsentFromTheTailIsSilentAfterTheBoundedWait", tokens: 200000, messages: lookupChain,
			input: func(_ string, in map[string]any) { in["tool_use_id"] = "call-absent" }, wantSleeps: nudgeWaitAttempts,
		},
		{
			name: "EditReachingTheTranscriptDuringTheWaitFires", tokens: 200000, messages: [][]string{{"Edit:short.go"}, {"Write:long.go"}}, late: true, wantSleeps: 1,
			want: func(string) string { return fmt.Sprintf(editChainNudge, 2) },
		},
		{name: "LateCallBelowTheContextThresholdIsSilentWithoutWaiting", tokens: 100000, messages: lookupChain, late: true},
		{name: "LateCallPastTheNudgeCapIsSilentWithoutWaiting", tokens: 200000, messages: lookupChain, late: true, before: func(s *batchingNudgeState) { s.Nudges = 2 }},
		{name: "NoStateFileIsSilentWithoutWaitingForALateCall", tokens: 200000, messages: lookupChain, late: true, noState: true},
		{
			name: "SubagentPastItsOwnCapIsSilentWithoutWaitingForALateCall", tokens: 200000, messages: lookupChain, late: true, agentID: nudgeAgentID,
			before: func(s *batchingNudgeState) { *s = s.withCount(nudgeAgentID, batchingNudgeCount{Nudges: 2}) },
		},
		{
			name: "SubagentEditReachingTheTranscriptDuringTheWaitFires", tokens: 200000, messages: [][]string{{"Edit:short.go"}, {"Write:long.go"}},
			late: true, agentID: nudgeAgentID, wantSleeps: 1, want: func(string) string { return fmt.Sprintf(editChainNudge, 2) },
		},
		{name: "SubagentWithNoReadingYetIsSilentWithoutWaiting", tokens: 200000, messages: [][]string{{"Grep"}}, late: true, agentID: nudgeAgentID},
		{
			name: "EditReachingTheTranscriptAfterSeveralRereadsFires", tokens: 200000, messages: [][]string{{"Edit:short.go"}, {"Write:long.go"}},
			late: true, lateAfter: 2, wantSleeps: 2, want: func(string) string { return fmt.Sprintf(editChainNudge, 2) },
		},
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
			transcript := filepath.Join(dir, "transcript.jsonl")
			chainTranscript, chainSidechain := transcript, false
			if tt.agentID != "" {
				writeNudgeTranscript(t, dir, transcript, 200000, false, [][]string{{"Bash"}})
				chainTranscript, chainSidechain = nudgeAgentTranscript(dir, tt.agentID), true
			}
			written := tt.messages
			if tt.late {
				written = tt.messages[:len(tt.messages)-1]
			}
			writeNudgeTranscript(t, dir, chainTranscript, tt.tokens, chainSidechain, written)
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
			name, toolInput := nudgeCallInput(dir, firstNudgeCall(tt.messages[last]))
			if tt.offset != nil {
				toolInput["offset"] = *tt.offset
			}
			if tt.limit != nil {
				toolInput["limit"] = *tt.limit
			}
			hookInput := map[string]any{"hook_event_name": "PostToolUse", "transcript_path": transcript, "tool_name": name, "tool_use_id": nudgeCallID(last, 0), "tool_input": toolInput}
			if tt.agentID != "" {
				hookInput["agent_id"] = tt.agentID
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

			lateAfter := max(tt.lateAfter, 1)
			sleeps := 0
			engine := New()
			engine.transcriptWait = batchingTranscriptWait{tick: time.Millisecond, attempts: nudgeWaitAttempts, sleep: func(time.Duration) {
				sleeps++
				switch {
				case !tt.late:
				case sleeps < lateAfter:
					grown := slices.Clone(written)
					for range sleeps {
						grown = append(grown, []string{nudgeSidechain})
					}
					writeNudgeTranscript(t, dir, chainTranscript, tt.tokens, chainSidechain, grown)
				case sleeps == lateAfter:
					writeNudgeTranscript(t, dir, chainTranscript, tt.tokens, chainSidechain, tt.messages)
				}
			}}
			got, err := engine.BatchingNudge(statePath, payload)
			if sleeps != tt.wantSleeps {
				t.Errorf("BatchingNudge() waited %d times; want %d", sleeps, tt.wantSleeps)
			}
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
				nudged := tt.nudgedMessage
				if nudged == "" {
					nudged = fmt.Sprintf("msg-%d", last)
				}
				counted := batchingNudgeCount{Nudges: before.count(tt.agentID).Nudges + 1, NudgedMessage: nudged}
				wantAfter = before.withCount(tt.agentID, counted)
			}

			if tt.corruptState {
				return
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
			if !reflect.DeepEqual(after, wantAfter) {
				t.Errorf("state after = %+v; want %+v", after, wantAfter)
			}
		})
	}
}

// TestEngineConstructorsSetTheTranscriptWait pins that both constructors give the engine the production wait.
// A zero wait never waits, which would return the hook to judging a call before its message reaches the transcript.
func TestEngineConstructorsSetTheTranscriptWait(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()
	tests := []struct {
		name   string
		engine *Claude
	}{
		{name: "New", engine: New()},
		{name: "NewFromConfig", engine: NewFromConfig(shuttleengine.Config{}, configengine.Dirs{})},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			wait := tt.engine.transcriptWait
			if wait.tick != batchingTranscriptTick {
				t.Errorf("transcriptWait.tick = %v; want %v", wait.tick, batchingTranscriptTick)
			}
			if wait.attempts != batchingTranscriptAttempts {
				t.Errorf("transcriptWait.attempts = %d; want %d", wait.attempts, batchingTranscriptAttempts)
			}
			if wait.sleep == nil {
				t.Errorf("transcriptWait.sleep = nil; want a sleep function")
			}
		})
	}
}

// TestReadTranscriptTail pins the window of the transcript the nudge reads.
// A file under the budget is read whole, and a larger one is cut to its complete lines.
// A window holding no line end yields nothing.
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

// TestValidateBatchingNudge pins the bounds of the batching_nudge block:
// context_tokens may be 0, the other numbers must be at least 1, and a block that is off is not checked.
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

// TestPrepare_BatchingNudgeHook pins that the template's batching_nudge installs the hook.
// The hook names the run's state file, and that file is seeded with the block.
// An off switch installs neither, and a threshold below its least value refuses the run before any artifact.
func TestPrepare_BatchingNudgeHook(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()
	template, err := LoadConfig(configengine.Dirs{Anchor: t.TempDir()})
	if err != nil {
		t.Fatalf("load the shipped claude template: %v", err)
	}
	matcherTools := strings.Split(batchingNudgeMatcher, "|")
	for _, tool := range []string{toolRead, toolGrep, toolGlob, toolBash, toolEdit, toolWrite} {
		if !slices.Contains(matcherTools, tool) {
			t.Errorf("batchingNudgeMatcher = %q; want it to hold %s", batchingNudgeMatcher, tool)
		}
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
			if found != tt.wantHook || (found && !reflect.DeepEqual(seeded, batchingNudgeState{Settings: claude.BatchingNudge})) {
				t.Errorf("state file = %+v, found=%t; want found=%t holding the block %+v", seeded, found, tt.wantHook, claude.BatchingNudge)
			}
		})
	}
}
