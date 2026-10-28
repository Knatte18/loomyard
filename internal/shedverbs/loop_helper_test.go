// loop_helper_test.go holds the step verb's text fixture and the loop envelope readers that the loop tests of every test tier share.

package shedverbs

import (
	"reflect"
	"sort"
	"testing"
)

func stepTexts() VerbTexts {
	return VerbTexts{Step: VerbText{Use: "step", Short: "step the fake shed"}}
}

// loopOf is the envelope's loop object.
func loopOf(t *testing.T, env map[string]any) map[string]any {
	t.Helper()
	loop, ok := env["loop"].(map[string]any)
	if !ok {
		t.Fatalf("envelope %v has no loop object", env)
	}
	return loop
}

// assertLoopKeys checks the loop object's closed key set.
func assertLoopKeys(t *testing.T, loop map[string]any, withRestep bool) {
	t.Helper()
	want := []string{"steps", "first_producer", "stop", "detail", "status_moved", "hand_back", "restep_hold", "current_producer", "history_length", "state", "interrupt_policy", "trace_copy", "stderr_path", "pause_reason"}
	if withRestep {
		want = append(want, "restep")
	}
	var got []string
	for key := range loop {
		got = append(got, key)
	}
	sort.Strings(got)
	sort.Strings(want)
	if !reflect.DeepEqual(got, want) {
		t.Errorf("loop keys = %v; want the closed set %v", got, want)
	}
}
