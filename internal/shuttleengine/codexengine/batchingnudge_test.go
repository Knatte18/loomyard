// batchingnudge_test.go pins the Codex batching nudge:
// the breaches it fires from a rollout's exec cells, what holds it back, and the count it keeps per thread.

package codexengine

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/configengine"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// nudgeBase is the time the fixture rollout's first top-level call starts.
// Each later call starts a minute after the one before.
var nudgeBase = time.Date(2026, 11, 16, 10, 0, 0, 0, time.UTC)

// fixtureNested is one nested call of a fixture exec cell: a shell command, or an apply_patch when patch is set.
type fixtureNested struct {
	command string
	patch   bool
	// subdir names a directory under the fixture directory the call ran in; empty runs it in the fixture directory.
	subdir string
	// reads are the paths the call's parsed_cmd marks as read, as the command wrote them.
	reads []string
	// plainCwd writes the call's cwd as a plain path instead of a file:// URL.
	plainCwd bool
	// absoluteReads writes each read path joined onto the fixture directory instead of as the command wrote it.
	absoluteReads bool
	// directArgv writes the call's argv as the command's words with no shell flag.
	directArgv bool
}

// fixtureCall is one top-level call of a fixture rollout.
// It is an exec cell of nested calls, or a spawn_agent function call when function is set.
type fixtureCall struct {
	function bool
	nested   []fixtureNested
	// after are items that complete after the call's output, outside every open cell; the last call has none.
	after []fixtureNested
}

// cell returns an exec cell running nested.
func cell(nested ...fixtureNested) fixtureCall {
	return fixtureCall{nested: nested}
}

// mustLine returns record as one JSON rollout line.
func mustLine(t *testing.T, record map[string]any) string {
	t.Helper()
	data, err := json.Marshal(record)
	if err != nil {
		t.Fatalf("marshal rollout line: %v", err)
	}
	return string(data)
}

// cellCallID returns the call id of the fixture's i-th top-level call.
func cellCallID(i int) string { return fmt.Sprintf("call-%d", i) }

// nestedItemID returns the item id of the j-th nested call of the fixture's i-th cell.
func nestedItemID(i, j int) string { return fmt.Sprintf("exec-%d-%d", i, j) }

// itemLine returns the item_completed line of the j-th nested call of cell i, run in dir or its subdir.
func itemLine(t *testing.T, dir string, i, j int, nested fixtureNested) string {
	t.Helper()
	item := map[string]any{"id": nestedItemID(i, j)}
	if nested.patch {
		item["type"] = itemFileChange
		item["changes"] = map[string]any{}
	} else {
		parsed := []map[string]any{}
		for _, read := range nested.reads {
			path := read
			if nested.absoluteReads {
				path = filepath.Join(dir, read)
			}
			parsed = append(parsed, map[string]any{"type": parsedCommandRead, "cmd": nested.command, "name": filepath.Base(read), "path": path})
		}
		argv := []string{"/bin/bash", "-lc", nested.command}
		if nested.directArgv {
			argv = strings.Fields(nested.command)
		}
		cwd := filepath.Join(dir, nested.subdir)
		item["type"] = itemCommandExecution
		item["command"] = argv
		item["cwd"] = "file://" + filepath.ToSlash(cwd)
		if nested.plainCwd {
			item["cwd"] = cwd
		}
		item["parsed_cmd"] = parsed
	}
	at := nudgeBase.Add(time.Duration(i) * time.Minute).Add(time.Second)
	return mustLine(t, map[string]any{"timestamp": at.Format(time.RFC3339Nano), "type": rolloutEventMessage, "payload": map[string]any{"type": rolloutItemCompleted, "item": item}})
}

// rolloutFixture returns the lines of a rollout holding a token_count reading of tokens, then calls in order.
// It holds no reading when tokens is 0.
// Every call but the last is closed by its output; the last is the open cell the hook's call runs in.
// Without lastItem the last nested call's item is left out, as Codex has not written it yet.
func rolloutFixture(t *testing.T, dir string, tokens int, calls []fixtureCall, lastItem bool) []string {
	t.Helper()
	var lines []string
	if tokens > 0 {
		lines = append(lines, tokenCountLine(nudgeBase.Add(-time.Minute).Format(time.RFC3339Nano), tokens))
	}
	for i, call := range calls {
		at := nudgeBase.Add(time.Duration(i) * time.Minute).Format(time.RFC3339Nano)
		last := i == len(calls)-1
		if call.function {
			lines = append(lines, mustLine(t, map[string]any{"timestamp": at, "type": rolloutResponseItem, "payload": map[string]any{"type": rolloutFunctionCall, "name": "spawn_agent", "namespace": "collaboration", "call_id": cellCallID(i), "arguments": "{}"}}))
		} else {
			lines = append(lines, mustLine(t, map[string]any{"timestamp": at, "type": rolloutResponseItem, "payload": map[string]any{"type": rolloutCustomToolCall, "name": execCellToolName, "call_id": cellCallID(i), "input": "await tools.exec_command()"}}))
			for j, nested := range call.nested {
				if last && j == len(call.nested)-1 && !lastItem {
					continue
				}
				lines = append(lines, itemLine(t, dir, i, j, nested))
			}
		}
		if !last {
			output := rolloutCustomToolCallOutput
			if call.function {
				output = rolloutFunctionCallOutput
			}
			lines = append(lines, mustLine(t, map[string]any{"timestamp": at, "type": rolloutResponseItem, "payload": map[string]any{"type": output, "call_id": cellCallID(i), "output": ""}}))
			for k, stray := range call.after {
				lines = append(lines, itemLine(t, dir, i, len(call.nested)+k, stray))
			}
		}
	}
	return lines
}

// TestBatchingNudge pins which breach a nested call fires, from the call's own hook input and its thread's rollout.
// It also pins what holds a nudge back.
// A fired nudge counts in the state file under its thread's rollout path and records its cell.
// A silent call leaves the state file as it was.
// Only a line-range read waits for its own item, up to the bound.
func TestBatchingNudge(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()
	settings := BatchingNudgeConfig{Enabled: true, LookupChain: true, ReadSlice: true, EditChain: true, ContextTokens: 150000, LookupRun: 3, EditRun: 2, SliceLines: 20, MaxNudges: 2}
	lookup := fixtureNested{command: "grep -n x short.txt"}
	patch := fixtureNested{patch: true}
	other := fixtureNested{command: "go vet ./internal/x"}
	readLong := fixtureNested{command: "cat long.txt", reads: []string{"long.txt"}}
	sliceLong := cell(fixtureNested{command: "head -n 5 long.txt", reads: []string{"long.txt"}})
	sliceShort := cell(fixtureNested{command: "sed -n 1,2p short.txt", reads: []string{"short.txt"}})
	lookupDirect := fixtureNested{command: "grep -n x short.txt", directArgv: true}
	readLongPlain := fixtureNested{command: "cat long.txt", reads: []string{"long.txt"}, plainCwd: true}
	sliceLongPlain := cell(fixtureNested{command: "head -n 5 long.txt", reads: []string{"long.txt"}, plainCwd: true})
	readLongAbsolute := fixtureNested{command: "cat long.txt", reads: []string{"long.txt"}, absoluteReads: true}
	lookupNudge := fmt.Sprintf(lookupChainNudge, 3)
	shortNudge := fmt.Sprintf(readSliceNudge, "short.txt", fmt.Sprintf(sliceShortReason, 20))
	readAgainNudge := fmt.Sprintf(readSliceNudge, "long.txt", sliceReadReason)
	const otherThread = "/elsewhere/rollout.jsonl"
	tests := []struct {
		name  string
		calls []fixtureCall
		// tokens is the thread's context reading; 0 writes none.
		tokens int
		// itemMissing leaves the hook's call's item out of the rollout until the itemAt-th wait, or for good when itemAt is 0.
		itemMissing bool
		itemAt      int
		// toolName replaces the hook input's tool name.
		toolName string
		// payload replaces the hook input outright.
		payload []byte
		// omitToolUseID leaves tool_use_id out of the hook input.
		omitToolUseID bool
		// rolloutAbsent points the hook input's transcript_path at a file that does not exist.
		rolloutAbsent bool
		// configure changes the seeded settings, so a row can turn a switch off.
		configure    func(*BatchingNudgeConfig)
		noState      bool
		corruptState bool
		// before seeds the state's thread counts; rollout is the path of the hook's thread.
		before func(threads map[string]batchingNudgeCount, rollout string)
		// longModified is how long after nudgeBase the file long.txt was last modified.
		longModified time.Duration
		wantSleeps   int
		wantErr      bool
		// want is the nudge line; empty wants no nudge.
		want string
	}{
		{name: "LookupChainFires", tokens: 200000, calls: []fixtureCall{cell(lookup), cell(lookup), cell(lookup)}, want: lookupNudge},
		{name: "LookupChainShorterThanTheRunIsSilent", tokens: 200000, calls: []fixtureCall{cell(other), cell(lookup), cell(lookup)}},
		{name: "CallWhoseItemIsNotWrittenYetCountsInTheLatestCell", tokens: 200000, calls: []fixtureCall{cell(lookup), cell(lookup), cell(lookup)}, itemMissing: true, want: lookupNudge},
		{name: "CellOfTwoLookupsIsSilent", tokens: 200000, calls: []fixtureCall{cell(lookup), cell(lookup), cell(lookup, lookup)}},
		{name: "FunctionCallEndsTheLookupChain", tokens: 200000, calls: []fixtureCall{cell(lookup), cell(lookup), {function: true}, cell(lookup), cell(lookup)}},
		{name: "ShellCellThatIsNoLookupEndsTheChain", tokens: 200000, calls: []fixtureCall{cell(lookup), cell(other), cell(lookup), cell(lookup)}},
		// A corrupt state file is an error once it is read, so a silent answer shows the call returned before the state.
		{name: "ShellCallThatIsNoLookupLeavesTheStateUnread", tokens: 200000, calls: []fixtureCall{cell(lookup), cell(lookup), cell(other)}, corruptState: true},
		{name: "OtherToolLeavesTheStateUnread", tokens: 200000, calls: []fixtureCall{cell(lookup), cell(lookup), cell(lookup)}, toolName: "collaborationspawn_agent", corruptState: true},
		{name: "EditChainFires", tokens: 200000, calls: []fixtureCall{cell(patch), cell(patch)}, want: fmt.Sprintf(editChainNudge, 2)},
		{name: "CellMixingAnEditAndALookupEndsTheEditChain", tokens: 200000, calls: []fixtureCall{cell(patch, lookup), cell(patch)}},
		{name: "FunctionCallEndsTheEditChain", tokens: 200000, calls: []fixtureCall{cell(patch), {function: true}, cell(patch)}},
		{name: "BelowTheContextThresholdIsSilent", tokens: 100000, calls: []fixtureCall{cell(lookup), cell(lookup), cell(lookup)}},
		{name: "NoContextReadingIsSilent", calls: []fixtureCall{cell(lookup), cell(lookup), cell(lookup)}},
		{name: "NoStateFileIsSilent", tokens: 200000, calls: []fixtureCall{cell(lookup), cell(lookup), cell(lookup)}, noState: true},
		{name: "CorruptStateIsAnError", tokens: 200000, calls: []fixtureCall{cell(lookup), cell(lookup), cell(lookup)}, corruptState: true, wantErr: true},
		{name: "MalformedInputIsAnError", payload: []byte("{"), wantErr: true},
		{
			name: "ThreadPastItsCapIsSilent", tokens: 200000, calls: []fixtureCall{cell(lookup), cell(lookup), cell(lookup)},
			before: func(threads map[string]batchingNudgeCount, rollout string) {
				threads[rollout] = batchingNudgeCount{Nudges: 2}
			},
		},
		{
			name: "CellAlreadyNudgedIsSilent", tokens: 200000, calls: []fixtureCall{cell(lookup), cell(lookup), cell(lookup)},
			before: func(threads map[string]batchingNudgeCount, rollout string) {
				threads[rollout] = batchingNudgeCount{Nudges: 1, NudgedCell: cellCallID(2)}
			},
		},
		{
			name: "AnotherThreadAtItsCapLeavesThisThreadsCountItsOwn", tokens: 200000, calls: []fixtureCall{cell(lookup), cell(lookup), cell(lookup)},
			before: func(threads map[string]batchingNudgeCount, _ string) {
				threads[otherThread] = batchingNudgeCount{Nudges: 2}
			},
			want: lookupNudge,
		},
		{name: "LineRangeReadOfAShortFileFires", tokens: 200000, calls: []fixtureCall{sliceShort}, want: shortNudge},
		{name: "LineRangeReadOfALongFileReadFirstIsSilent", tokens: 200000, calls: []fixtureCall{sliceLong}},
		{name: "LineRangeReadOfAFileReadEarlierAndUnchangedFires", tokens: 200000, calls: []fixtureCall{cell(readLong), cell(other), sliceLong}, longModified: -time.Hour, want: readAgainNudge},
		{name: "LineRangeReadOfAFileChangedSinceItsReadIsSilent", tokens: 200000, calls: []fixtureCall{cell(readLong), cell(other), sliceLong}, longModified: time.Hour},
		{
			name: "EarlierReadInAnotherDirectoryIsSilent", tokens: 200000, longModified: -time.Hour,
			calls: []fixtureCall{cell(fixtureNested{command: "cat long.txt", subdir: "sub", reads: []string{"long.txt"}}), sliceLong},
		},
		{
			name: "EarlierReadOfTheSameFileFromAnotherDirectoryFires", tokens: 200000, longModified: -time.Hour, want: readAgainNudge,
			calls: []fixtureCall{cell(fixtureNested{command: "cat ../long.txt", subdir: "sub", reads: []string{"../long.txt"}}), sliceLong},
		},
		{name: "LineRangeReadWaitsForItsItem", tokens: 200000, calls: []fixtureCall{sliceShort}, itemMissing: true, itemAt: 2, wantSleeps: 2, want: shortNudge},
		{name: "LineRangeReadWithoutItsItemAtTheBoundIsSilent", tokens: 200000, calls: []fixtureCall{sliceShort}, itemMissing: true, wantSleeps: 3},
		{name: "UnreadableRolloutIsSilent", tokens: 200000, calls: []fixtureCall{cell(lookup), cell(lookup), cell(lookup)}, rolloutAbsent: true},
		{name: "PayloadWithoutAToolUseIDIsSilent", tokens: 200000, calls: []fixtureCall{cell(lookup), cell(lookup), cell(lookup)}, itemMissing: true, omitToolUseID: true},
		{
			name: "LookupChainSwitchedOffIsSilent", tokens: 200000, calls: []fixtureCall{cell(lookup), cell(lookup), cell(lookup)},
			configure: func(s *BatchingNudgeConfig) { s.LookupChain = false },
		},
		{
			name: "ReadSliceSwitchedOffIsSilent", tokens: 200000, calls: []fixtureCall{sliceShort},
			configure: func(s *BatchingNudgeConfig) { s.ReadSlice = false },
		},
		{
			name: "EditChainSwitchedOffIsSilent", tokens: 200000, calls: []fixtureCall{cell(patch), cell(patch)},
			configure: func(s *BatchingNudgeConfig) { s.EditChain = false },
		},
		{name: "PlainPathCwdStillFiresTheReadAgainNudge", tokens: 200000, calls: []fixtureCall{cell(readLongPlain), cell(other), sliceLongPlain}, longModified: -time.Hour, want: readAgainNudge},
		{name: "AbsoluteReadPathStillFiresTheReadAgainNudge", tokens: 200000, calls: []fixtureCall{cell(readLongAbsolute), cell(other), sliceLong}, longModified: -time.Hour, want: readAgainNudge},
		{name: "LookupWithAnArgvHoldingNoShellStillCountsInTheChain", tokens: 200000, calls: []fixtureCall{cell(lookupDirect), cell(lookupDirect), cell(lookup)}, want: lookupNudge},
		{
			name: "ItemAfterItsCellsOutputJoinsNoCell", tokens: 200000, want: lookupNudge,
			calls: []fixtureCall{cell(lookup), {nested: []fixtureNested{lookup}, after: []fixtureNested{lookup}}, cell(lookup)},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			dir := t.TempDir()
			writeFile(t, filepath.Join(dir, "short.txt"), strings.Repeat("line\n", 5))
			long := filepath.Join(dir, "long.txt")
			writeFile(t, long, strings.Repeat("line\n", 30))
			modified := nudgeBase.Add(tt.longModified)
			if err := os.Chtimes(long, modified, modified); err != nil {
				t.Fatalf("date long.txt: %v", err)
			}
			rollout := filepath.Join(dir, "rollout.jsonl")
			writeFile(t, rollout, strings.Join(rolloutFixture(t, dir, tt.tokens, tt.calls, !tt.itemMissing), "\n")+"\n")

			statePath := filepath.Join(dir, batchingNudgeStateFileName)
			rowSettings := settings
			if tt.configure != nil {
				tt.configure(&rowSettings)
			}
			seeded := batchingNudgeState{Settings: rowSettings, Threads: map[string]batchingNudgeCount{}}
			if tt.before != nil {
				tt.before(seeded.Threads, rollout)
			}
			switch {
			case tt.corruptState:
				writeFile(t, statePath, "not json")
			case !tt.noState:
				data, err := json.Marshal(seeded)
				if err != nil {
					t.Fatalf("marshal state: %v", err)
				}
				writeFile(t, statePath, string(data))
			}
			stateBefore, _ := os.ReadFile(statePath)

			payload := tt.payload
			var currentCell int
			if payload == nil {
				currentCell = len(tt.calls) - 1
				nested := tt.calls[currentCell].nested
				current := nested[len(nested)-1]
				toolName := toolMatcherBash
				if current.patch {
					toolName = toolNameApplyPatch
				}
				if tt.toolName != "" {
					toolName = tt.toolName
				}
				input := map[string]any{
					"hook_event_name": hookEventPostToolUse, "transcript_path": rollout, "cwd": dir, "tool_name": toolName,
					"tool_use_id": nestedItemID(currentCell, len(nested)-1), "tool_input": map[string]any{"command": current.command},
				}
				if tt.omitToolUseID {
					delete(input, "tool_use_id")
				}
				if tt.rolloutAbsent {
					input["transcript_path"] = filepath.Join(dir, "absent.jsonl")
				}
				var err error
				payload, err = json.Marshal(input)
				if err != nil {
					t.Fatalf("marshal hook input: %v", err)
				}
			}
			sleeps := 0
			engine := &Codex{rolloutWait: batchingRolloutWait{tick: time.Millisecond, attempts: 3, sleep: func(time.Duration) {
				sleeps++
				if tt.itemMissing && sleeps == tt.itemAt {
					nested := tt.calls[currentCell].nested
					line := itemLine(t, dir, currentCell, len(nested)-1, nested[len(nested)-1])
					f, err := os.OpenFile(rollout, os.O_APPEND|os.O_WRONLY, 0o644)
					if err != nil {
						t.Fatalf("open rollout: %v", err)
					}
					defer f.Close()
					if _, err := f.WriteString(line + "\n"); err != nil {
						t.Fatalf("append item: %v", err)
					}
				}
			}}}

			got, err := engine.BatchingNudge(statePath, payload)
			if tt.wantErr {
				if err == nil {
					t.Errorf("BatchingNudge() = %s, nil; want an error", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("BatchingNudge() error: %v", err)
			}
			if sleeps != tt.wantSleeps {
				t.Errorf("BatchingNudge() waited %d times; want %d", sleeps, tt.wantSleeps)
			}
			stateAfter, _ := os.ReadFile(statePath)
			if tt.want == "" {
				if got != nil {
					t.Errorf("BatchingNudge() = %s; want no nudge", got)
				}
				if string(stateAfter) != string(stateBefore) {
					t.Errorf("state file after a silent call = %s; want it unchanged, %s", stateAfter, stateBefore)
				}
				return
			}
			var output struct {
				SystemMessage      string `json:"systemMessage"`
				HookSpecificOutput struct {
					HookEventName     string `json:"hookEventName"`
					AdditionalContext string `json:"additionalContext"`
				} `json:"hookSpecificOutput"`
			}
			if err := json.Unmarshal(got, &output); err != nil {
				t.Fatalf("BatchingNudge() = %s; want hook output JSON: %v", got, err)
			}
			if output.HookSpecificOutput.HookEventName != hookEventPostToolUse || output.HookSpecificOutput.AdditionalContext != tt.want || output.SystemMessage != tt.want {
				t.Errorf("BatchingNudge() = %s; want the line %q as PostToolUse additional context and system message", got, tt.want)
			}
			want := seeded.withCount(rollout, batchingNudgeCount{Nudges: seeded.Threads[rollout].Nudges + 1, NudgedCell: cellCallID(currentCell)})
			var after batchingNudgeState
			if err := json.Unmarshal(stateAfter, &after); err != nil || !reflect.DeepEqual(after, want) {
				t.Errorf("state file after a nudge = %s, %v; want %+v", stateAfter, err, want)
			}
		})
	}
}

// TestLocalPath pins the local path a CommandExecution's file:// cwd names:
// a drive path keeps no leading slash on Windows, so it carries its volume, and a POSIX path stays rooted.
func TestLocalPath(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()
	drivePath := "/C:/work/repo"
	if runtime.GOOS == "windows" {
		drivePath = filepath.FromSlash("C:/work/repo")
	}
	tests := []struct {
		name string
		cwd  string
		want string
	}{
		{"DrivePath", "file:///C:/work/repo", drivePath},
		{"PosixPath", "file:///home/x/repo", filepath.FromSlash("/home/x/repo")},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := localPath(tt.cwd); got != tt.want {
				t.Errorf("localPath(%q) = %q; want %q", tt.cwd, got, tt.want)
			}
		})
	}
}

// TestNewFromConfig_SetsTheRolloutWait pins that the constructor gives the nudge the production wait for a call's item.
func TestNewFromConfig_SetsTheRolloutWait(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()
	wait := NewFromConfig(shuttleengine.Config{}, configengine.Dirs{}, Seams{}).rolloutWait
	if wait.tick != batchingRolloutTick || wait.attempts != batchingRolloutAttempts || wait.sleep == nil {
		t.Errorf("NewFromConfig().rolloutWait = %+v; want tick %v, %d attempts and a sleep", wait, batchingRolloutTick, batchingRolloutAttempts)
	}
}
