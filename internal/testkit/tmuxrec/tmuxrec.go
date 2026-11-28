// Package tmuxrec stands in for what reedengine reaches outside the repository:
// the tmux server, the process table's pane ages and the systemd user manager's memory scopes.
//
// A Recording holds what those answered, call by call, while reed drove one scenario over a real tmux server.
// It names the command, date and tmux version it was recorded with.
// Replay answers reed's calls from one recording, as the reedengine.Outside a test passes to reedengine.NewOver,
// so every module above tmux runs for real.
// A recording keeps the paths and process ids of the machine that made it as placeholders,
// and a replay binds them to the test's own directories and to process ids no kernel hands out,
// so no process of the machine running a replay is ever read or signalled.
// The text a call hands tmux on standard input is what the code under test sent,
// so a recording holds only the hub directory in it as a placeholder.
// Replay answers only what was recorded; a call with no recorded answer fails, naming the call.
// A call matches by its arguments and by the text it hands tmux on standard input, as a paste does.
// A paste names a fresh buffer each time, so a recording holds a placeholder for the buffer name,
// and a replayed paste matches whatever buffer name it draws, as long as the call names only that one.
//
// Recordings live in this package's testdata directory, one file per scenario.
// The package's tier0 test replays every scenario over its recording.
// Its tier2 test drives each scenario over real tmux and compares what reed read with the recording.
// Run with -tmuxrec.write, the tier2 test writes each recording again instead.
package tmuxrec

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/Knatte18/loomyard/internal/reedengine"
)

// Call kinds: a tmux subcommand, a pane-age read of the process table and a memory-scope move of the user manager.
const (
	KindTmux      = "tmux"
	KindPaneAge   = "pane-age"
	KindCapMemory = "cap-memory"
)

// pidPlaceholderPrefix starts the placeholder a recording holds in place of a process id.
const pidPlaceholderPrefix = "{pid:"

// injectBufferPlaceholder is what a recording holds in place of the buffer name a paste loads its text into.
const injectBufferPlaceholder = "{inject-buffer}"

// injectBufferPattern matches the buffer name a paste draws.
var injectBufferPattern = regexp.MustCompile(regexp.QuoteMeta(reedengine.InjectBufferPrefix) + `[0-9a-f]+`)

// holdInjectBuffers returns args with each paste buffer name replaced by its placeholder.
// It reports whether args name at most one buffer, as one paste does.
func holdInjectBuffers(args []string) ([]string, bool) {
	held := make([]string, len(args))
	named := ""
	for i, arg := range args {
		for _, name := range injectBufferPattern.FindAllString(arg, -1) {
			if named != "" && name != named {
				return nil, false
			}
			named = name
		}
		held[i] = injectBufferPattern.ReplaceAllString(arg, injectBufferPlaceholder)
	}
	return held, true
}

// replayPIDBase is added to a placeholder's index to make the process id a replay answers.
// It lies above the largest pid a Linux kernel hands out (2^22), so a replayed pid names no process.
const replayPIDBase = 1 << 30

// readVerbs are the tmux subcommands that change nothing on the server.
// A replay answers a read again from the latest recorded answer, since the server stood still between them.
var readVerbs = []string{
	"-V", "capture-pane", "display-message", "has-session", "list-buffers", "list-clients", "list-commands",
	"list-keys", "list-panes", "list-sessions", "list-windows", "show-buffer", "show-environment", "show-hooks",
	"show-options", "show-window-options",
}

// Source names where a recording came from.
type Source struct {
	// Command is the command that recorded it.
	Command string `json:"command"`
	// Date is the day it was recorded, as YYYY-MM-DD.
	Date string `json:"date"`
	// Tmux is the `tmux -V` line of the binary it was recorded from.
	Tmux string `json:"tmux"`
	// OS is the operating system it was recorded on, as runtime.GOOS names it.
	OS string `json:"os"`
}

// Call is one answer the outside gave reed.
type Call struct {
	Kind string `json:"kind"`
	// Args is a tmux call's argv after the server flags.
	Args []string `json:"args,omitempty"`
	// Capture marks a tmux call whose standard output reed read.
	Capture bool `json:"capture,omitempty"`
	// Stdin is the text a tmux call handed tmux on standard input, such as a paste's text.
	Stdin string `json:"stdin,omitempty"`
	// Out is a tmux call's standard output, or a memory-scope move's cgroup.
	Out string `json:"out,omitempty"`
	// Err is the text of the error the call failed with, empty for a call that succeeded.
	Err string `json:"err,omitempty"`
	// Exit is the exit code of a tmux call that ran and exited nonzero.
	Exit int `json:"exit,omitempty"`
	// PID is the placeholder of the process a pane-age read or a memory-scope move named.
	PID string `json:"pid,omitempty"`
	// AgeMS and Known are a pane-age read's answer: the process's age in milliseconds, and whether it could be read.
	AgeMS int64 `json:"age_ms,omitempty"`
	Known bool  `json:"known,omitempty"`
	// Unit and Limit are a memory-scope move's scope name and byte cap.
	Unit  string `json:"unit,omitempty"`
	Limit int64  `json:"limit,omitempty"`
}

// read reports whether the call changes nothing outside, so a replay may answer it again.
// A tmux call that chains commands is a read only when every command of the chain is.
func (c Call) read() bool {
	switch c.Kind {
	case KindPaneAge:
		return true
	case KindTmux:
		verb := true
		for _, arg := range c.Args {
			if verb && !slices.Contains(readVerbs, arg) {
				return false
			}
			verb = arg == ";"
		}
		return len(c.Args) > 0
	}
	return false
}

// matches reports whether c answers the call want, whose placeholders are already bound alike.
func (c Call) matches(want Call) bool {
	return c.Kind == want.Kind && slices.Equal(c.Args, want.Args) && c.Capture == want.Capture && c.Stdin == want.Stdin &&
		c.PID == want.PID && c.Unit == want.Unit && c.Limit == want.Limit
}

// StrandReading is what reed's status read reported for one strand, the fields a scenario compares.
type StrandReading struct {
	GUID     string `json:"guid"`
	Name     string `json:"name"`
	Live     bool   `json:"live"`
	Retiring bool   `json:"retiring,omitempty"`
}

// Reading is what reed reported at one step of a scenario.
type Reading struct {
	Step    string          `json:"step"`
	Strands []StrandReading `json:"strands"`
	// Err is the text of the error the step failed with.
	Err string `json:"err,omitempty"`
}

// Recording is one scenario's calls, in the order reed made them, and what reed read at each step.
type Recording struct {
	Source Source `json:"source"`
	// StartupTimeoutS is the session runner's startup timeout the scenario ran under, in seconds.
	StartupTimeoutS int       `json:"startup_timeout_s"`
	Calls           []Call    `json:"calls"`
	Readings        []Reading `json:"readings"`
}

// Path returns the file of the recording named name in dir.
func Path(dir, name string) string {
	return filepath.Join(dir, name+".json")
}

// Load reads the recording at path.
func Load(path string) (Recording, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Recording{}, fmt.Errorf("tmuxrec: read recording: %w", err)
	}
	var rec Recording
	if err := json.Unmarshal(data, &rec); err != nil {
		return Recording{}, fmt.Errorf("tmuxrec: decode recording %s: %w", path, err)
	}
	return rec, nil
}

// Save writes rec to path.
func Save(path string, rec Recording) error {
	data, err := json.MarshalIndent(rec, "", "  ")
	if err != nil {
		return fmt.Errorf("tmuxrec: encode recording: %w", err)
	}
	if err := os.WriteFile(path, append(data, '\n'), 0o644); err != nil {
		return fmt.Errorf("tmuxrec: write recording: %w", err)
	}
	return nil
}

// Bindings maps each placeholder a recording holds, such as "{hub}", to the value it stands for.
type Bindings map[string]string

// placehold replaces every bound value in s with its placeholder, the longest value first,
// so a path under another bound path keeps its own placeholder.
func (b Bindings) placehold(s string) string {
	names := make([]string, 0, len(b))
	for name := range b {
		names = append(names, name)
	}
	sort.Slice(names, func(i, j int) bool { return len(b[names[i]]) > len(b[names[j]]) })
	for _, name := range names {
		if b[name] != "" {
			s = strings.ReplaceAll(s, b[name], name)
		}
	}
	return s
}

// bind replaces every placeholder in s with its bound value, and each pid placeholder with its replayed pid.
func (b Bindings) bind(s string) string {
	for name, value := range b {
		s = strings.ReplaceAll(s, name, value)
	}
	return pidPlaceholderPattern.ReplaceAllStringFunc(s, func(placeholder string) string {
		return strconv.Itoa(replayPID(placeholder))
	})
}

// pidPlaceholderPattern matches a pid placeholder.
var pidPlaceholderPattern = regexp.MustCompile(`\{pid:[0-9]+\}`)

// replayPID returns the process id a replay answers for a pid placeholder.
func replayPID(placeholder string) int {
	index, _ := strconv.Atoi(strings.TrimSuffix(strings.TrimPrefix(placeholder, pidPlaceholderPrefix), "}"))
	return replayPIDBase + index
}

// Replay answers reed's calls from one recording.
// It is safe for concurrent use.
type Replay struct {
	name     string
	bindings Bindings
	source   Source

	mu    sync.Mutex
	calls []Call
	// next is the index of the first call no answer has passed yet.
	next int
	// unanswered records each call the recording held no answer for, in order.
	unanswered []string
}

// NewReplay returns a replay of rec, named name in its failures, with its placeholders bound by bindings.
func NewReplay(name string, rec Recording, bindings Bindings) *Replay {
	calls := make([]Call, len(rec.Calls))
	for i, call := range rec.Calls {
		calls[i] = bindCall(call, bindings)
	}
	return &Replay{name: name, bindings: bindings, source: rec.Source, calls: calls}
}

// bindCall returns call with every placeholder bound.
func bindCall(call Call, bindings Bindings) Call {
	bound := call
	bound.Args = make([]string, len(call.Args))
	for i, arg := range call.Args {
		bound.Args[i] = bindings.bind(arg)
	}
	bound.Stdin = bindings.bind(call.Stdin)
	bound.Out = bindings.bind(call.Out)
	bound.Err = bindings.bind(call.Err)
	bound.Unit = bindings.bind(call.Unit)
	bound.PID = bindings.bind(call.PID)
	return bound
}

// Outside returns the reedengine.Outside that answers from the replay.
// newGUID is the guid source the engine mints strand guids from.
func (r *Replay) Outside(newGUID func() (string, error)) reedengine.Outside {
	return reedengine.Outside{
		Tmux:        r.tmux,
		TmuxVersion: r.source.Tmux,
		PaneAge:     r.paneAge,
		CapMemory:   r.capMemory,
		NewGUID:     newGUID,
	}
}

// Unanswered returns each call the recording held no answer for, in order.
func (r *Replay) Unanswered() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return slices.Clone(r.unanswered)
}

// Remaining returns each recorded change at or after the replay's place, in order: the changes no call has asked for.
func (r *Replay) Remaining() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	var remaining []string
	for _, call := range r.calls[r.next:] {
		if !call.read() {
			remaining = append(remaining, describe(call))
		}
	}
	return remaining
}

func (r *Replay) tmux(capture bool, stdin []byte, args ...string) (string, error) {
	want := Call{Kind: KindTmux, Args: args, Capture: capture, Stdin: string(stdin)}
	held, oneBuffer := holdInjectBuffers(args)
	if !oneBuffer {
		r.mu.Lock()
		defer r.mu.Unlock()
		return "", r.refuseLocked(want)
	}
	want.Args = held
	answer, err := r.answer(want)
	if err != nil {
		return "", err
	}
	return answer.Out, answerErr(answer)
}

func (r *Replay) paneAge(pid int) (time.Duration, bool) {
	answer, err := r.answer(Call{Kind: KindPaneAge, PID: strconv.Itoa(pid)})
	if err != nil {
		return 0, false
	}
	return time.Duration(answer.AgeMS) * time.Millisecond, answer.Known
}

func (r *Replay) capMemory(pid int, unit string, limit int64) (string, error) {
	answer, err := r.answer(Call{Kind: KindCapMemory, PID: strconv.Itoa(pid), Unit: unit, Limit: limit})
	if err != nil {
		return "", err
	}
	return answer.Out, answerErr(answer)
}

// answerErr returns the recorded failure of answer, or nil for a call that succeeded.
// A tmux call that exited nonzero fails with an error that reports its exit code, as the real exec's does.
func answerErr(answer Call) error {
	if answer.Err == "" {
		return nil
	}
	if answer.Exit != 0 {
		return &exitError{text: answer.Err, code: answer.Exit}
	}
	return errors.New(answer.Err)
}

// exitError is a recorded failure of a tmux call that exited nonzero.
type exitError struct {
	text string
	code int
}

func (e *exitError) Error() string { return e.text }

// ExitCode returns the exit code the call exited with.
func (e *exitError) ExitCode() int { return e.code }

// answer returns the recorded answer to want.
// A call that changes something takes the first recorded match from the replay's place on, passing only reads.
// It then moves the place past that match.
// A read takes the first match before the next change.
// With none there, it takes the latest match since the last change the replay passed.
// Nothing changed since that answer.
func (r *Replay) answer(want Call) (Call, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for i := r.next; i < len(r.calls); i++ {
		call := r.calls[i]
		if call.matches(want) {
			r.next = i + 1
			return call, nil
		}
		if !call.read() {
			break
		}
	}
	if want.read() {
		for i := r.next - 1; i >= 0 && r.calls[i].read(); i-- {
			if r.calls[i].matches(want) {
				return r.calls[i], nil
			}
		}
	}
	return Call{}, r.refuseLocked(want)
}

// refuseLocked records want as a call the recording holds no answer to and returns the failure naming it.
// The caller holds the replay's lock.
func (r *Replay) refuseLocked(want Call) error {
	described := describe(want)
	r.unanswered = append(r.unanswered, described)
	return fmt.Errorf("tmuxrec: recording %s holds no answer to %s after call %d", r.name, described, r.next)
}

// describe names a call in a failure.
func describe(call Call) string {
	switch call.Kind {
	case KindTmux:
		if call.Stdin != "" {
			return fmt.Sprintf("tmux %s < %q", strings.Join(call.Args, " "), call.Stdin)
		}
		return "tmux " + strings.Join(call.Args, " ")
	case KindPaneAge:
		return "the pane age of pid " + call.PID
	}
	return fmt.Sprintf("the memory scope %s of pid %s", call.Unit, call.PID)
}
