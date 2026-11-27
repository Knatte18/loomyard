// carryover.go carries the instructions an operator typed into an unfinished run over into the next run of the same role:
// the sent-texts record that tells shuttle's own sends from typed instructions, the reading of a run's operator instructions, and the predecessor lookup a run start makes.

package shuttleengine

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/Knatte18/loomyard/internal/logger"
)

// noticeInstructionMax is the length an instruction is cut to in the carry-over notice line.
const noticeInstructionMax = 200

// recordSentText appends text as one JSON string line to the sent-texts file in runDir.
// A failed write is logged and never fails the send: a missing line can only make that text read as an operator instruction later.
func recordSentText(runDir, text string) {
	line, _ := json.Marshal(text)
	f, err := os.OpenFile(filepath.Join(runDir, sentTextsFileName), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		logger.Warn("shuttle: sent text not recorded, a later carry-over may read it as an operator instruction", "runDir", runDir, "error", err)
		return
	}
	defer f.Close()
	if _, err := f.Write(append(line, '\n')); err != nil {
		logger.Warn("shuttle: sent text not recorded, a later carry-over may read it as an operator instruction", "runDir", runDir, "error", err)
	}
}

// sentTexts returns the trimmed texts the sent-texts file in runDir holds.
// A missing or unreadable file reads as none, and a malformed line is skipped.
func sentTexts(runDir string) map[string]bool {
	texts := map[string]bool{}
	f, err := os.Open(filepath.Join(runDir, sentTextsFileName))
	if err != nil {
		return texts
	}
	defer f.Close()
	scanner := bufio.NewScanner(f)
	scanner.Buffer(nil, 16<<20)
	for scanner.Scan() {
		var text string
		if json.Unmarshal(scanner.Bytes(), &text) == nil {
			texts[strings.TrimSpace(text)] = true
		}
	}
	return texts
}

// runTurnStart is one turn start of a run past its prompt offset.
type runTurnStart struct {
	// end is the events-file offset just past the turn start's line.
	end int64
	// operatorInstruction is the instruction an operator typed to start the turn, empty when the run's prompt, a send into the run or the provider itself started it.
	operatorInstruction string
}

// runTurnStarts returns, in file order, the turn starts of the run rs in runDir past its prompt offset.
// A turn start's instruction is the operator's unless it is the turn start of the run's prompt or a send into the run typed its text.
// The prompt's turn start is the first one when the record holds no deferred prompt line, else the one whose instruction is that line, so a run that died before its prompt landed keeps its first instruction.
// A turn start the events file holds no Raw line for is not read.
// An engine without a SessionSignalParser and an unreadable events file yield none.
func runTurnStarts(runDir string, rs RunState, engine Engine) []runTurnStart {
	if _, ok := engine.(SessionSignalParser); !ok {
		return nil
	}
	data, err := os.ReadFile(rs.EventsPath)
	if err != nil {
		return nil
	}
	promptOffset := min(max(rs.PromptOffset, 0), int64(len(data)))
	sent := sentTexts(runDir)
	var starts []runTurnStart
	promptLine := strings.TrimSpace(rs.PromptLine)
	promptSeen := false
	for _, positioned := range positionedTurnStarts(engine, data[promptOffset:], promptOffset) {
		start := runTurnStart{end: positioned.end}
		instruction := positioned.signal.Instruction
		switch {
		case !promptSeen && (promptLine == "" || instruction == promptLine):
			promptSeen = true
		case instruction != "" && !sent[instruction]:
			start.operatorInstruction = instruction
		}
		starts = append(starts, start)
	}
	return starts
}

// operatorInstructions returns, in order, the instructions an operator typed into the run rs in runDir:
// the instructions of its turn starts past its prompt offset, without the turn start of the run's prompt and without any text a send into the run typed.
// A turn start the events file holds no Raw line for is not read.
// An engine without a SessionSignalParser and an unreadable events file yield none.
func operatorInstructions(runDir string, rs RunState, engine Engine) []string {
	var instructions []string
	for _, start := range runTurnStarts(runDir, rs, engine) {
		if start.operatorInstruction != "" {
			instructions = append(instructions, start.operatorInstruction)
		}
	}
	return instructions
}

// roleKey returns the key a run start matches its predecessor by: the spec's name override, else its role.
func roleKey(spec Spec) string {
	if spec.NameOverride != "" {
		return spec.NameOverride
	}
	return spec.Role
}

// unfinished reports whether rs is a run that ended without finishing: stopped, died, timed out or ended on API errors, or reading running with its waiter gone.
func unfinished(rs RunState) bool {
	switch rs.Outcome {
	case runOutcomeStopped, string(OutcomeDied), string(OutcomeTimeout), string(OutcomeAPIError):
		return true
	case runOutcomeRunning:
		return !runIsLive(rs)
	}
	return false
}

// carriedInstructions returns the instructions a new run of the role key carries over: the carried and operator instructions of the newest run of key under the runner's run-directory root, without repeats, when that run is unfinished.
// An empty key, no run of key, a newest run that is not unfinished and a run whose engine is not registered carry none.
func (r *Runner) carriedInstructions(key string) []string {
	if key == "" {
		return nil
	}
	root := runDirRoot(r.cfg, r.anchorPath)
	entries, err := os.ReadDir(root)
	if err != nil {
		return nil
	}
	var newest RunState
	var newestDir string
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		runDir := filepath.Join(root, entry.Name())
		rs, found, err := loadRunState(runDir)
		if err != nil || !found || rs.RoleKey != key {
			continue
		}
		if newestDir == "" || rs.CreatedAt > newest.CreatedAt || (rs.CreatedAt == newest.CreatedAt && runDir > newestDir) {
			newest, newestDir = rs, runDir
		}
	}
	if newestDir == "" || !unfinished(newest) {
		return nil
	}
	engine, _, err := r.engineFor(newest)
	if err != nil {
		return nil
	}
	seen := map[string]bool{}
	var carried []string
	for _, instruction := range append(append([]string(nil), newest.CarriedInstructions...), operatorInstructions(newestDir, newest, engine)...) {
		if !seen[instruction] {
			seen[instruction] = true
			carried = append(carried, instruction)
		}
	}
	return carried
}

// carryOverBlock renders instructions as the block appended to a new run's prompt.
func carryOverBlock(instructions []string) string {
	var block strings.Builder
	block.WriteString("\n\n## Operator instructions carried over\n\n")
	block.WriteString("The session this one replaces ended before it finished, after the operator typed these instructions into it.\n")
	block.WriteString("Follow them as the operator's own, beside the instructions above.\n")
	for i, instruction := range instructions {
		fmt.Fprintf(&block, "\nInstruction %d:\n\n%s\n", i+1, instruction)
	}
	return block.String()
}

// noticeCarriedInstructions logs the carry-over into the run started on strandName and sends the runner's notifier one line naming it, when one is set.
// A notifier error is only logged.
func (r *Runner) noticeCarriedInstructions(strandName, key string, instructions []string) {
	quoted := make([]string, len(instructions))
	for i, instruction := range instructions {
		text := strings.Join(strings.Fields(instruction), " ")
		if runes := []rune(text); len(runes) > noticeInstructionMax {
			text = string(runes[:noticeInstructionMax]) + "…"
		}
		quoted[i] = fmt.Sprintf("%q", text)
	}
	line := fmt.Sprintf("shuttle: %s replaces an unfinished session of role %s; the operator instructions it took are carried over into the new prompt: %s",
		strandName, key, strings.Join(quoted, "; "))
	logger.Warn("shuttle: operator instructions carried over into a re-spawned run", "strand", strandName, "role", key, "instructions", len(instructions))
	if r.notifier == nil {
		return
	}
	if err := r.notifier(line); err != nil {
		logger.Warn("shuttle: carry-over notice failed", "strand", strandName, "error", err)
	}
}
