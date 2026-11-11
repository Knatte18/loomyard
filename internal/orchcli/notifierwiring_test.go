// notifierwiring_test.go pins the route of every runner's hold notice: each module wires its shuttle runner's notifier through NotifyAsker.
// It also pins that the root pre-run installs the hub's push and reconcile notifiers.

package orchcli_test

import (
	"os"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// TestRootPreRun_InstallsTheHubNotifiers proves the root command's pre-run installs the push and reconcile notifiers, so every lym process routes their notices to the prime orch.
// The pre-run builds its installs behind a testing guard, so the test reads the wiring source.
func TestRootPreRun_InstallsTheHubNotifiers(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()
	const path = "../../cmd/lym/main.go"
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	_, afterPreRun, found := strings.Cut(string(data), "PersistentPreRunE: func(")
	if !found {
		t.Fatalf("%s holds no root PersistentPreRunE", path)
	}
	preRun, _, _ := strings.Cut(afterPreRun, "seedStencils(cmd)")
	for _, call := range []string{"orchcli.InstallPushNotifier(cmd.Context())", "orchcli.InstallReconcileNotifier(cmd.Context())", "orchcli.InstallRunMailDropper()"} {
		if !strings.Contains(preRun, call) {
			t.Errorf("%s: the root pre-run = %q; want it to call %s", path, preRun, call)
		}
	}
}

// TestRunnerNotifiers_ResolveWithTheAskerRule proves each module that builds a shuttle runner wires its notifier through NotifyAsker, so a held agent's notice reaches the orch its own question reached.
// The runner keeps its notifier private and the wiring builds the runner inside the engine stack, so the test reads the wiring source.
func TestRunnerNotifiers_ResolveWithTheAskerRule(t *testing.T) {
	tiergate.Require(t, 0)
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
