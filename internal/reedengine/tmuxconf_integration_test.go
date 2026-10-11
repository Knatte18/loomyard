//go:build tmux && !windows

// tmuxconf_integration_test.go proves against a real tmux server what an operator tmux config named by `tmux_conf` can and cannot change on a server reed starts.

package reedengine

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/Knatte18/loomyard/internal/testkit/tmuxkit"
)

// writeTmuxConf writes an operator tmux config with the given lines into a temp directory and returns its path.
func writeTmuxConf(t *testing.T, lines ...string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "operator.conf")
	if err := os.WriteFile(path, []byte(strings.Join(lines, "\n")+"\n"), 0o644); err != nil {
		t.Fatalf("write operator tmux config: %v", err)
	}
	return path
}

// tmuxConfEngine builds the engine of worktree under hubDir with tmux_conf set to confPath and its warnings captured.
func tmuxConfEngine(t *testing.T, hubDir, worktree, confPath string) (*Engine, *bytes.Buffer) {
	t.Helper()
	e := newHubEngine(t, hubDir, worktree, func(cfg *Config) { cfg.TmuxConf = confPath })
	var warnings bytes.Buffer
	e.warnOut = &warnings
	return e, &warnings
}

// optionValue reads one option of e's server through show-options, with the scope flags given.
func optionValue(t *testing.T, e *Engine, args ...string) string {
	t.Helper()
	out, err := e.tmux.output(append([]string{"show-options"}, args...)...)
	if err != nil {
		t.Fatalf("show-options %v: %v", args, err)
	}
	return strings.TrimSpace(out)
}

// sessionOption reads one option of e's own session.
func sessionOption(t *testing.T, e *Engine, option string) string {
	t.Helper()
	return optionValue(t, e, "-qv", "-t", exactSessionWindowTarget(e.SessionName()), option)
}

// bootConcurrently runs Up on every engine at once, registers the shared key afterwards, and returns each Up's error.
func bootConcurrently(t *testing.T, engines ...*Engine) []error {
	t.Helper()
	errs := make([]error, len(engines))
	var wg sync.WaitGroup
	for i, e := range engines {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, errs[i] = e.Up()
		}()
	}
	wg.Wait()
	tmuxkit.KillOnCleanup(t, engines[0].cfg.Tmux, engines[0].Socket())
	return errs
}

// TestBoot_TmuxConf boots reed with an operator file named by tmux_conf, each time into a server reed starts itself.
func TestBoot_TmuxConf(t *testing.T) {
	t.Run("FileOptionsStandOnTheServer", func(t *testing.T) {
		conf := writeTmuxConf(t, "set -g @operator_marker yes", "set -g prefix C-a", "set -g escape-time 7")
		e, _ := tmuxConfEngine(t, t.TempDir(), "worktree", conf)
		if err := bootOwnServer(t, e); err != nil {
			t.Fatalf("Up(): %v", err)
		}
		for option, want := range map[string]string{"@operator_marker": "yes", "prefix": "C-a", "escape-time": "7"} {
			if got := optionValue(t, e, "-gqv", option); got != want {
				t.Errorf("%s = %q, want %q", option, got, want)
			}
		}
	})

	t.Run("ReedsPinsAndTheTwoGuardsWin", func(t *testing.T) {
		conf := writeTmuxConf(t,
			"set -g mouse off",
			"set -g remain-on-exit off",
			"set -g default-command /bin/false",
			"set -g exit-unattached on",
			"set -g destroy-unattached on",
		)
		hubDir := t.TempDir()
		first, _ := tmuxConfEngine(t, hubDir, "first", conf)
		if err := bootOwnServer(t, first); err != nil {
			t.Fatalf("Up(): %v", err)
		}
		wantShell, err := resolveShellPath(first.cfg.Shell)
		if err != nil {
			t.Fatalf("resolveShellPath: %v", err)
		}
		for option, want := range map[string]string{"mouse": first.cfg.Mouse, "remain-on-exit": "on", "destroy-unattached": "off"} {
			if got := optionValue(t, first, "-gqv", option); got != want {
				t.Errorf("global %s = %q, want %q", option, got, want)
			}
		}
		if got := optionValue(t, first, "-sqv", "exit-unattached"); got != "off" {
			t.Errorf("exit-unattached = %q, want off", got)
		}
		if got := sessionOption(t, first, "default-command"); got != wantShell {
			t.Errorf("session default-command = %q, want %q", got, wantShell)
		}
		if got := sessionOption(t, first, "destroy-unattached"); got != "off" {
			t.Errorf("first session destroy-unattached = %q, want off", got)
		}

		second, _ := tmuxConfEngine(t, hubDir, "second", conf)
		if _, err := second.Up(); err != nil {
			t.Fatalf("second Up(): %v", err)
		}
		if got := sessionOption(t, second, "destroy-unattached"); got != "off" {
			t.Errorf("second session destroy-unattached = %q, want off", got)
		}
	})

	t.Run("UntargetedDestroyUnattachedFailsTheBoot", func(t *testing.T) {
		conf := writeTmuxConf(t, "set destroy-unattached on")
		e, _ := tmuxConfEngine(t, t.TempDir(), "worktree", conf)
		err := bootOwnServer(t, e)
		if err == nil {
			t.Fatal("Up() = nil, want an error: the file ends the session")
		}
		for _, want := range []string{"tmux_conf", tmuxConfEnv, conf} {
			if !strings.Contains(err.Error(), want) {
				t.Errorf("Up() error = %q, want it to name %q", err, want)
			}
		}
	})

	t.Run("AppendingFileIsSourcedOncePerServerStart", func(t *testing.T) {
		conf := writeTmuxConf(t, "set -ag @operator_list x")

		t.Run("SecondWorktreeBootedLater", func(t *testing.T) {
			hubDir := t.TempDir()
			first, _ := tmuxConfEngine(t, hubDir, "first", conf)
			if err := bootOwnServer(t, first); err != nil {
				t.Fatalf("Up(): %v", err)
			}
			second, _ := tmuxConfEngine(t, hubDir, "second", conf)
			if _, err := second.Up(); err != nil {
				t.Fatalf("second Up(): %v", err)
			}
			if got := optionValue(t, first, "-gqv", "@operator_list"); got != "x" {
				t.Errorf("@operator_list = %q, want one append", got)
			}
		})

		t.Run("TwoWorktreesBootedConcurrently", func(t *testing.T) {
			hubDir := t.TempDir()
			first, _ := tmuxConfEngine(t, hubDir, "first", conf)
			second, _ := tmuxConfEngine(t, hubDir, "second", conf)
			for i, err := range bootConcurrently(t, first, second) {
				if err != nil {
					t.Fatalf("concurrent Up() #%d: %v", i, err)
				}
			}
			if got := optionValue(t, first, "-gqv", "@operator_list"); got != "x" {
				t.Errorf("@operator_list = %q, want one append", got)
			}
		})
	})

	t.Run("ConcurrentBootsWithTheTwoGlobalKillersKeepBothSessions", func(t *testing.T) {
		conf := writeTmuxConf(t, "set -g destroy-unattached on", "set -g exit-unattached on")
		hubDir := t.TempDir()
		first, _ := tmuxConfEngine(t, hubDir, "first", conf)
		second, _ := tmuxConfEngine(t, hubDir, "second", conf)
		for i, err := range bootConcurrently(t, first, second) {
			if err != nil {
				t.Fatalf("concurrent Up() #%d: %v", i, err)
			}
		}
		for _, e := range []*Engine{first, second} {
			if got := sessionOption(t, e, "destroy-unattached"); got != "off" {
				t.Errorf("session %s destroy-unattached = %q, want off", e.SessionName(), got)
			}
		}
	})

	t.Run("ParseErrorBootsAppliesNothingAndWarns", func(t *testing.T) {
		conf := writeTmuxConf(t, "set -g @before yes", "this-is-not-a-command", "set -g @after yes")
		e, warnings := tmuxConfEngine(t, t.TempDir(), "worktree", conf)
		if err := bootOwnServer(t, e); err != nil {
			t.Fatalf("Up(): %v", err)
		}
		for _, option := range []string{"@before", "@after"} {
			if got := optionValue(t, e, "-gqv", option); got != "" {
				t.Errorf("%s = %q, want unset: a parse error applies nothing", option, got)
			}
		}
		if !strings.Contains(warnings.String(), conf) {
			t.Errorf("warnOut = %q, want it to carry %q", warnings.String(), conf)
		}
	})

	t.Run("RunTimeErrorBetweenValidLinesAppliesBothAndWarns", func(t *testing.T) {
		conf := writeTmuxConf(t, "set -g @before yes", "set -g no-such-option x", "set -g @after yes")
		e, warnings := tmuxConfEngine(t, t.TempDir(), "worktree", conf)
		if err := bootOwnServer(t, e); err != nil {
			t.Fatalf("Up(): %v", err)
		}
		for _, option := range []string{"@before", "@after"} {
			if got := optionValue(t, e, "-gqv", option); got != "yes" {
				t.Errorf("%s = %q, want yes", option, got)
			}
		}
		if !strings.Contains(warnings.String(), conf) {
			t.Errorf("warnOut = %q, want it to carry %q", warnings.String(), conf)
		}
	})

	t.Run("RunTimeErrorBesideTheGlobalKillersBootsWarnsAndReadsBothPinsOff", func(t *testing.T) {
		conf := writeTmuxConf(t, "set -g destroy-unattached on", "set -g no-such-option x", "set -g exit-unattached on")
		e, warnings := tmuxConfEngine(t, t.TempDir(), "worktree", conf)
		if err := bootOwnServer(t, e); err != nil {
			t.Fatalf("Up(): %v", err)
		}
		if got := optionValue(t, e, "-gqv", "destroy-unattached"); got != "off" {
			t.Errorf("global destroy-unattached = %q, want off", got)
		}
		if got := optionValue(t, e, "-sqv", "exit-unattached"); got != "off" {
			t.Errorf("exit-unattached = %q, want off", got)
		}
		if !strings.Contains(warnings.String(), conf) {
			t.Errorf("warnOut = %q, want it to carry %q", warnings.String(), conf)
		}
	})
}
