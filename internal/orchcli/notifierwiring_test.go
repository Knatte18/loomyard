//go:build tier0

// notifierwiring_test.go pins the route of every runner's hold notice: each module wires its shuttle runner's notifier through NotifyAsker.

package orchcli_test

import (
	"os"
	"strings"
	"testing"
)

// TestRunnerNotifiers_ResolveWithTheAskerRule proves each module that builds a shuttle runner wires its notifier through NotifyAsker, so a held agent's notice reaches the orch its own question reached.
// The runner keeps its notifier private and the wiring builds the runner inside the engine stack, so the test reads the wiring source.
func TestRunnerNotifiers_ResolveWithTheAskerRule(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		path string
	}{
		{"Burler", "../burlercli/wiring.go"},
		{"Loom", "../loomcli/wiring.go"},
		{"Webster", "../webstercli/wiring.go"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			data, err := os.ReadFile(tt.path)
			if err != nil {
				t.Fatalf("read %s: %v", tt.path, err)
			}
			_, afterCall, found := strings.Cut(string(data), "runner.SetNotifier(")
			if !found {
				t.Fatalf("%s never calls runner.SetNotifier", tt.path)
			}
			notifier, _, _ := strings.Cut(afterCall, "\n\t})")
			if !strings.Contains(notifier, "orchcli.NotifyAsker(") {
				t.Errorf("%s: the runner's notifier = %q; want it to call orchcli.NotifyAsker", tt.path, notifier)
			}
		})
	}
}
