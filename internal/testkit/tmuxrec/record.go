// record.go holds the Recorder, which runs reed's calls on a real tmux server, process table and user manager.
// It keeps every answer for a Recording.

package tmuxrec

import (
	"errors"
	"fmt"
	"regexp"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/Knatte18/loomyard/internal/proc"
	"github.com/Knatte18/loomyard/internal/reedengine"
)

// Recorder passes reed's calls to the real outside and keeps each answer.
// It is safe for concurrent use.
type Recorder struct {
	socket  string
	runTmux TmuxRun

	mu    sync.Mutex
	calls []Call
	// pids holds every process id a call named, the ones a recording replaces with placeholders.
	pids map[int]bool
}

// TmuxRun runs one tmux subcommand on a real server and returns its standard output.
// A server it starts reads no operator config.
// A failure carries tmux's standard error in its text, and a call that exited nonzero reports its code through ExitCode.
type TmuxRun func(stdin []byte, args ...string) (string, error)

// NewRecorder returns a recorder whose tmux calls run reaches the real server of the -L socket key socket.
func NewRecorder(socket string, run TmuxRun) *Recorder {
	return &Recorder{socket: socket, runTmux: run, pids: map[int]bool{}}
}

// Outside returns the reedengine.Outside that passes reed's calls through the recorder.
// newGUID is the guid source the engine mints strand guids from.
func (r *Recorder) Outside(newGUID func() (string, error)) reedengine.Outside {
	return reedengine.Outside{Tmux: r.tmux, PaneAge: r.paneAge, CapMemory: r.capMemory, NewGUID: newGUID}
}

func (r *Recorder) tmux(capture bool, stdin []byte, args ...string) (string, error) {
	out, err := r.runTmux(stdin, args...)
	if !capture {
		out = ""
	}
	for _, pid := range formatPIDs(args, out) {
		r.keepPID(pid)
	}
	call := Call{Kind: KindTmux, Args: append([]string(nil), args...), Capture: capture, Stdin: string(stdin), Out: out, Err: errText(err)}
	var exitErr interface{ ExitCode() int }
	if errors.As(err, &exitErr) {
		call.Exit = exitErr.ExitCode()
	}
	r.keep(call)
	return out, err
}

// formatPIDs returns the process ids out holds in the fields of the format args asks for that expand a pid:
// #{pane_pid} and the server's #{pid}.
// A format and each line of out are split alike on spaces, tabs and bars,
// and every pid field of reed's formats comes before any free-text field, so the positions line up.
func formatPIDs(args []string, out string) []int {
	format := ""
	for _, arg := range args {
		if strings.Contains(arg, "#{") {
			format = arg
		}
	}
	split := func(s string) []string {
		return strings.FieldsFunc(s, func(c rune) bool { return c == ' ' || c == '\t' || c == '|' })
	}
	var positions []int
	for i, field := range split(format) {
		if strings.Contains(field, "#{pane_pid}") || field == "#{pid}" {
			positions = append(positions, i)
		}
	}
	var pids []int
	for _, line := range strings.Split(out, "\n") {
		fields := split(line)
		for _, i := range positions {
			if i < len(fields) {
				if pid, err := strconv.Atoi(fields[i]); err == nil {
					pids = append(pids, pid)
				}
			}
		}
	}
	return pids
}

func (r *Recorder) paneAge(pid int) (time.Duration, bool) {
	age, known := reedengine.PaneProcessAge(pid)
	r.keepPID(pid)
	r.keep(Call{Kind: KindPaneAge, PID: strconv.Itoa(pid), AgeMS: age.Milliseconds(), Known: known})
	return age, known
}

func (r *Recorder) capMemory(pid int, unit string, limit int64) (string, error) {
	cgroup, err := proc.CapProcessMemory(pid, unit, limit)
	r.keepPID(pid)
	r.keep(Call{Kind: KindCapMemory, PID: strconv.Itoa(pid), Unit: unit, Limit: limit, Out: cgroup, Err: errText(err)})
	return cgroup, err
}

func (r *Recorder) keep(call Call) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.calls = append(r.calls, call)
}

func (r *Recorder) keepPID(pid int) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if pid > 0 {
		r.pids[pid] = true
	}
}

// errText returns err's text, empty for nil.
func errText(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

// Recording returns what the recorder kept in session, with every bound value and process id replaced by its placeholder.
// The tmux server's own pid is read here, so the server must still be up.
// command names what recorded it, and readings are what reed read at the scenario's steps.
func (r *Recorder) Recording(command string, session *Session, readings []Reading) (Recording, error) {
	bindings := session.Bindings
	if err := checkNameBindings(bindings); err != nil {
		return Recording{}, err
	}
	version, err := r.runTmux(nil, "-V")
	if err != nil {
		return Recording{}, fmt.Errorf("tmuxrec: read the tmux version: %w", err)
	}
	serverPID, err := r.runTmux(nil, "display-message", "-p", "#{pid}")
	if err != nil {
		return Recording{}, fmt.Errorf("tmuxrec: read the tmux server's pid: %w", err)
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if pid, err := strconv.Atoi(strings.TrimSpace(serverPID)); err == nil {
		r.pids[pid] = true
	}
	placehold := pidPlaceholder(r.pids)
	calls := make([]Call, len(r.calls))
	for i, call := range r.calls {
		calls[i] = placeholdCall(call, bindings, placehold)
	}
	placeheld := make([]Reading, len(readings))
	for i, reading := range readings {
		placeheld[i] = Reading{Step: reading.Step, Err: placehold(bindings.placehold(reading.Err))}
		for _, strand := range reading.Strands {
			placeheld[i].Strands = append(placeheld[i].Strands, strand)
		}
	}
	return Recording{
		Source: Source{
			Command: command,
			Date:    time.Now().UTC().Format(time.DateOnly),
			Tmux:    strings.TrimSpace(version),
			OS:      runtime.GOOS,
		},
		StartupTimeoutS: session.StartupTimeoutS,
		Calls:           calls,
		Readings:        placeheld,
	}, nil
}

// shortestBoundName is the fewest bytes a bound user or host name has.
// Placeholding replaces substrings, and a shorter name also occurs inside other text.
const shortestBoundName = 4

// checkNameBindings fails for a user or host name whose placeholding would rewrite other text of a recording.
func checkNameBindings(bindings Bindings) error {
	for _, placeholder := range []string{"{user}", "{host}"} {
		name, bound := bindings[placeholder]
		if !bound {
			continue
		}
		if len(name) < shortestBoundName {
			return fmt.Errorf("tmuxrec: the %s name %q is shorter than %d bytes, so a recording cannot placehold it", placeholder, name, shortestBoundName)
		}
		for _, fixed := range []string{WorktreeName, Shortname, EngineName, LaunchCmd, SessionID, GUIDPrefix} {
			if strings.Contains(fixed, name) {
				return fmt.Errorf("tmuxrec: the %s name %q occurs inside the session's name %q, so a recording cannot placehold it", placeholder, name, fixed)
			}
		}
	}
	return nil
}

// pidPlaceholder returns a function that replaces each whole-number token naming one of pids with its placeholder.
// The placeholders are numbered by ascending pid, so a recording made again numbers them alike.
func pidPlaceholder(pids map[int]bool) func(string) string {
	sorted := make([]int, 0, len(pids))
	for pid := range pids {
		sorted = append(sorted, pid)
	}
	sort.Ints(sorted)
	index := make(map[string]string, len(sorted))
	for i, pid := range sorted {
		index[strconv.Itoa(pid)] = fmt.Sprintf("%s%d}", pidPlaceholderPrefix, i+1)
	}
	return func(s string) string {
		return numberToken.ReplaceAllStringFunc(s, func(token string) string {
			if placeholder, ok := index[token]; ok {
				return placeholder
			}
			return token
		})
	}
}

// numberToken matches a run of digits that no other digit touches.
var numberToken = regexp.MustCompile(`[0-9]+`)

// placeholdCall returns call with every paste buffer name, bound value and process id replaced by its placeholder.
// The buffer names go first, since a run of digits in one can spell a process id.
// The text on standard input is what the code under test sends.
// Its numbers are no ids tmux handed out, so it keeps them.
// Of the bound values it holds only the hub directory as a placeholder.
// Any other bound value can be literal text of the code's own.
func placeholdCall(call Call, bindings Bindings, placehold func(string) string) Call {
	held := call
	held.Args = make([]string, len(call.Args))
	for i, arg := range call.Args {
		held.Args[i] = placehold(bindings.placehold(injectBufferPattern.ReplaceAllString(arg, injectBufferPlaceholder)))
	}
	held.Stdin = Bindings{"{hub}": bindings["{hub}"]}.placehold(call.Stdin)
	held.Out = placehold(bindings.placehold(call.Out))
	held.Err = placehold(bindings.placehold(call.Err))
	held.Unit = placehold(bindings.placehold(call.Unit))
	held.PID = placehold(call.PID)
	return held
}
