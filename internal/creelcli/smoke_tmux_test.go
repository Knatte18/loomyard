//go:build tmux

// smoke_tmux_test.go proves creel's delivery on a real tmux pane that runs a shell script standing in for a provider.
// The script draws a ready marker and an input box line, echoes each line it reads and redraws idle, and a fake shuttle engine reads those frames.

package creelcli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/configengine"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
	"github.com/Knatte18/loomyard/internal/testkit/shuttlefake"
)

const (
	// fakeProviderReady is the marker the script draws above its input box line.
	fakeProviderReady = "READY>"
	// fakeProviderBox is the input box line the script draws, left empty while the script is idle.
	fakeProviderBox = "[ input ]"
)

// fakeProviderScript draws the ready marker and the input box line, echoes each line it reads and redraws idle.
const fakeProviderScript = `#!/bin/sh
draw() { printf 'READY>\n[ input ] '; }
draw
while IFS= read -r line; do
  printf 'GOT: %s\n' "$line"
  draw
done
`

// scriptProviderEngine is a shuttle engine over the fake provider's frames: ready once the marker is drawn, idle when the last line is the empty input box.
type scriptProviderEngine struct {
	shuttlefake.Engine
}

var _ shuttleengine.SessionCycler = (*scriptProviderEngine)(nil)

func (e *scriptProviderEngine) Startup(capture string) shuttleengine.StartupState {
	if strings.Contains(capture, fakeProviderReady) {
		return shuttleengine.StartupReady
	}
	return shuttleengine.StartupPending
}

func (e *scriptProviderEngine) ComposeSend(text string) []shuttleengine.PaneInput {
	return []shuttleengine.PaneInput{{Text: text, Submit: true}}
}

func (e *scriptProviderEngine) IdleSession(capture string) bool {
	lines := strings.Split(strings.TrimRight(capture, " \n"), "\n")
	return strings.TrimSpace(lines[len(lines)-1]) == fakeProviderBox
}

func (e *scriptProviderEngine) PaneTooShort(string) bool { return false }

func (e *scriptProviderEngine) ContextTokens(shuttleengine.Event) shuttleengine.ContextReading {
	return shuttleengine.ContextReading{}
}

func (e *scriptProviderEngine) CompactedSince(shuttleengine.Event, time.Time) (shuttleengine.CompactionBoundary, bool) {
	return shuttleengine.CompactionBoundary{}, false
}

func (e *scriptProviderEngine) ClearSessionSequence() []shuttleengine.PaneInput { return nil }

func (e *scriptProviderEngine) CompactSessionSequence(string) []shuttleengine.PaneInput { return nil }

func (e *scriptProviderEngine) ReloadPluginsSequence() []shuttleengine.PaneInput { return nil }

// TestSmokeCreelFakeProviderPane runs the real daemon on the real socket over a hub whose one strand is the fake provider.
// A post is acknowledged and its reminder is typed into the pane, a privileged inject is typed and reported injected, and the log shows the events.
func TestSmokeCreelFakeProviderPane(t *testing.T) {
	// The strand name in the environment is process-global state, set by newSmokeHub.
	smoke := newSmokeHub(t)
	scriptPath := filepath.Join(t.TempDir(), "fake-provider.sh")
	if err := os.WriteFile(scriptPath, []byte(fakeProviderScript), 0o755); err != nil {
		t.Fatalf("WriteFile(%q) = %v", scriptPath, err)
	}
	name, guid := smoke.addStrand(t, "fakeprovider", "sh "+scriptPath)
	if capture, ok := smoke.awaitPane(t, guid, fakeProviderBox); !ok {
		t.Fatalf("the fake provider drew no input box; pane:\n%s", capture)
	}
	smoke.startDaemon(t, func(shuttleengine.Config, configengine.Dirs) (shuttleengine.EngineSet, error) {
		return shuttleengine.NewEngineSet("script", map[string]shuttleengine.Engine{"script": &scriptProviderEngine{}}), nil
	})

	code, posted := smoke.creel("post", name, "hello from creel")
	if code != 0 {
		t.Fatalf("lyx creel post = %d; output: %s", code, posted)
	}
	if capture, ok := smoke.awaitPane(t, guid, "[creel] 1 unread"); !ok {
		t.Fatalf("the reminder was not typed into the pane; pane:\n%s", capture)
	}

	const injected = "creel-injected-line"
	code, out := smoke.creel("inject", "--wait", "--timeout", "60s", name, injected)
	if code != 0 || !strings.Contains(out, `"outcome":"injected"`) {
		t.Fatalf("lyx creel inject --wait = %d, %s; want exit 0 injected", code, out)
	}
	if capture, ok := smoke.awaitPane(t, guid, "GOT: "+injected); !ok {
		t.Fatalf("the injected line was not echoed by the fake provider; pane:\n%s", capture)
	}

	code, logged := smoke.creel("log")
	if code != 0 || !strings.Contains(logged, `"event":"posted"`) {
		t.Errorf("lyx creel log = %d, %s; want exit 0 with the posted event", code, logged)
	}
}
