// faketmux_test.go is the one scripted tmux fake the reedengine tests install on an engine's
// TmuxCmd.execHook, so no test hand-writes its own recording closure.

package reedengine

import (
	"errors"
	"strconv"
	"sync"
	"testing"
)

// liveBoxFormat is the display-message format the live window-size query spends.
const liveBoxFormat = "#{window_width} #{window_height}"

type tmuxAnswer struct {
	out string
	err error
}

// fakeTmux answers every tmux round trip from per-verb scripts and logs each call's argv.
// An unscripted verb answers empty with no error.
// It is safe for one goroutine to script and read it while another drives the engine.
type fakeTmux struct {
	t *testing.T

	mu        sync.Mutex
	verbs     map[string]tmuxAnswer
	formats   map[string]tmuxAnswer
	forbidden map[string]bool
	calls     [][]string
}

// installFakeTmux installs a fresh fakeTmux on e's tmux seam.
func installFakeTmux(t *testing.T, e *Engine) *fakeTmux {
	t.Helper()
	f := &fakeTmux{
		t:         t,
		verbs:     map[string]tmuxAnswer{},
		formats:   map[string]tmuxAnswer{},
		forbidden: map[string]bool{},
	}
	e.tmux.execHook = f.exec
	return f
}

// answer scripts verb's answer from now on, replacing any earlier one.
func (f *fakeTmux) answer(verb, out string, err error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.verbs[verb] = tmuxAnswer{out, err}
}

// answerFormat scripts a display-message answer keyed by its trailing format string.
// It takes precedence over answer("display-message", ...), which stays the answer for every other format.
func (f *fakeTmux) answerFormat(format, out string, err error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.formats[format] = tmuxAnswer{out, err}
}

// answerSession scripts the answers a session with live panes gives: list-panes, the generation
// probe and the live window-size query.
func (f *fakeTmux) answerSession(live []LivePane, box string, boxErr error) {
	f.answer("list-panes", encodeLivePanes(live), nil)
	f.answerFormat(paneGenerationFormat, "$0|1|1000", nil)
	f.answer("display-message", box, boxErr)
}

// mustNotCall fails the test, at the call, whenever one of verbs runs.
func (f *fakeTmux) mustNotCall(verbs ...string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, v := range verbs {
		f.forbidden[v] = true
	}
}

// Calls returns every call's argv, in order.
func (f *fakeTmux) Calls() [][]string {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([][]string, len(f.calls))
	copy(out, f.calls)
	return out
}

// Sequence returns the verb of every call, in order, keeping only the listed verbs when any are given.
func (f *fakeTmux) Sequence(only ...string) []string {
	var out []string
	for _, c := range f.Calls() {
		if len(only) == 0 || containsArg(only, c[0]) {
			out = append(out, c[0])
		}
	}
	return out
}

// ArgvFor returns the argv of every call to verb, in order.
func (f *fakeTmux) ArgvFor(verb string) [][]string {
	var out [][]string
	for _, c := range f.Calls() {
		if c[0] == verb {
			out = append(out, c)
		}
	}
	return out
}

// Count reports how many calls ran verb.
func (f *fakeTmux) Count(verb string) int {
	return len(f.ArgvFor(verb))
}

func (f *fakeTmux) exec(capture bool, args ...string) (string, error) {
	verb := args[0]
	f.mu.Lock()
	f.calls = append(f.calls, append([]string{}, args...))
	forbidden := f.forbidden[verb]
	ans, ok := f.verbs[verb]
	if verb == "display-message" {
		if byFormat, found := f.formats[args[len(args)-1]]; found {
			ans, ok = byFormat, true
		}
	}
	f.mu.Unlock()

	if forbidden {
		f.t.Errorf("tmux %s was called with %v, want it never called", verb, args)
		return "", errors.New("tmux " + verb + " must not be called")
	}
	if !ok {
		return "", nil
	}
	return ans.out, ans.err
}

// encodeLivePanes renders live back into list-panes' own six-field wire format, matching
// overlay.go's listPanes/parsePaneList round trip.
func encodeLivePanes(live []LivePane) string {
	out := ""
	for _, p := range live {
		dead := "0"
		if p.Dead {
			dead = "1"
		}
		out += p.ID + " " + dead + " " + strconv.Itoa(p.Top) + " " + strconv.Itoa(p.Width) + " " + strconv.Itoa(p.Height) + " " + strconv.Itoa(p.PID) + "\n"
	}
	return out
}
