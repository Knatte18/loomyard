// selfexec_test.go pins when a Watcher execs onto a changed binary and when it keeps running.

package selfexec

import (
	"errors"
	"os"
	"reflect"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/testkit/logcapture"
)

// statStep is one scripted stat result.
type statStep struct {
	id  identity
	err error
}

func idOf(inode uint64) statStep { return statStep{id: identity{inode: inode}} }

var statFailed = statStep{err: errors.New("stat failed")}

type execCall struct {
	path string
	argv []string
	env  []string
}

// TestWatcher_Check mutates the process-global logger output, so it does not run in parallel.
func TestWatcher_Check(t *testing.T) {
	execFailure := errors.New("exec failed")
	tests := []struct {
		name string
		// steps[0] is the start identity; each later step answers one Check.
		steps    []statStep
		execErr  error
		wantExec int
		wantWarn int
	}{
		{name: "an unchanged identity never execs", steps: []statStep{idOf(1), idOf(1), idOf(1)}},
		{name: "a changed identity seen once never execs", steps: []statStep{idOf(1), idOf(2)}},
		{name: "a changed identity seen on two consecutive checks execs once", steps: []statStep{idOf(1), idOf(2), idOf(2)}, wantExec: 1},
		{name: "a changed identity that flips back before the second check never execs", steps: []statStep{idOf(1), idOf(2), idOf(1), idOf(2)}},
		{name: "a failed stat between two changed checks never execs", steps: []statStep{idOf(1), idOf(2), statFailed, idOf(2)}},
		{name: "a failed exec keeps running and is not called again for that identity", steps: []statStep{idOf(1), idOf(2), idOf(2), idOf(2), idOf(2)}, execErr: execFailure, wantExec: 1, wantWarn: 1},
		{name: "a later different identity after a failed one execs again after two checks", steps: []statStep{idOf(1), idOf(2), idOf(2), idOf(3), idOf(3)}, execErr: execFailure, wantExec: 2, wantWarn: 2},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			buf := logcapture.Capture(t)
			next := 0
			stat := func(path string) (identity, error) {
				if path != "/bin/self" {
					t.Errorf("stat path = %q, want the recorded path", path)
				}
				step := tt.steps[next]
				next++
				return step.id, step.err
			}
			var calls []execCall
			exec := func(path string, argv, env []string) error {
				calls = append(calls, execCall{path, argv, env})
				return tt.execErr
			}
			w := newWatcher("/bin/self", stat, exec)
			for range tt.steps[1:] {
				w.Check()
			}
			if len(calls) != tt.wantExec {
				t.Fatalf("exec calls = %d, want %d", len(calls), tt.wantExec)
			}
			for _, call := range calls {
				if call.path != "/bin/self" || !reflect.DeepEqual(call.argv, os.Args) || !reflect.DeepEqual(call.env, os.Environ()) {
					t.Errorf("exec call = %+v, want the recorded path, os.Args and os.Environ()", call)
				}
			}
			if got := strings.Count(buf.String(), "could not re-exec"); got != tt.wantWarn {
				t.Errorf("exec-failure Warn lines = %d, want %d; log:\n%s", got, tt.wantWarn, buf.String())
			}
		})
	}

	t.Run("execSelf under go test refuses before it can replace the test binary", func(t *testing.T) {
		err := execSelf("/nonexistent/selfexec-test-binary", os.Args, os.Environ())
		if !errors.Is(err, errUnderTest) {
			t.Fatalf("execSelf error = %v, want errUnderTest", err)
		}
	})
}
