//go:build tmux && !windows

// serverconfig_integration_test.go proves against a real tmux server that reed's own server reads no operator config and that its strand panes start non-login shells.
// It also pins the shell options reed's session pins share with the test kit's server config.

package reedengine

import (
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/testkit/tmuxkit"
)

// TestBoot_ReadsNoOperatorConfigAndStartsNonLoginPanes boots reed under a HOME whose `.tmux.conf` sets a marker option and creates a session of its own.
// The key is registered with the kit only after the boot, so reed starts the server itself; a probe or spawn that read the config would leave the marker set or the config's session alive.
// It reads no operator config, pins the resolved shell on the session, and a pane split into the session starts that shell, which a login-shell pane would not report.
//
// The second step pins that the option names the kit's config sets to the shell are the names reed pins per session.
func TestBoot_ReadsNoOperatorConfigAndStartsNonLoginPanes(t *testing.T) {
	t.Run("ReadsNoOperatorConfigAndStartsNonLoginPanes", func(t *testing.T) {
		home := t.TempDir()
		operatorConfig := "set -g @operator_marker yes\nnew-session -d -s operator\n"
		if err := os.WriteFile(filepath.Join(home, ".tmux.conf"), []byte(operatorConfig), 0o644); err != nil {
			t.Fatalf("write operator tmux config: %v", err)
		}
		t.Setenv("HOME", home)

		hubDir := t.TempDir()
		worktreeDir := filepath.Join(hubDir, "worktree")
		if err := os.MkdirAll(worktreeDir, 0o755); err != nil {
			t.Fatalf("mkdir worktree dir: %v", err)
		}
		seedReedConfig(t, worktreeDir)
		cfg, err := LoadConfig(worktreeDir, "reed")
		if err != nil {
			t.Fatalf("LoadConfig: %v", err)
		}
		if _, err := exec.LookPath(cfg.Tmux); err != nil {
			t.Skipf("configured multiplexer binary %q not found: %v", cfg.Tmux, err)
		}
		geom := Geometry{
			SocketKey:     ServerName(hubDir),
			SessionName:   SessionName(worktreeDir),
			AnchorPath:    worktreeDir,
			PaneCwd:       worktreeDir,
			WorktreeRoot:  worktreeDir,
			LogsDir:       filepath.Join(hubDir, "logs"),
			HubPath:       hubDir,
			WorktreeName:  filepath.Base(worktreeDir),
			NameShortname: "tc",
			NameSlug:      "tslug",
		}
		e := New(cfg, geom)

		_, upErr := e.Up()
		// Registered only after the boot, so the server is reed's own and not the kit's pre-started one.
		tmuxkit.KillOnCleanup(t, cfg.Tmux, geom.SocketKey)
		if upErr != nil {
			t.Fatalf("Up(): %v", upErr)
		}

		marker, err := e.tmux.output("show-options", "-gqv", "@operator_marker")
		if err != nil {
			t.Fatalf("show-options -gqv @operator_marker: %v", err)
		}
		if got := strings.TrimSpace(marker); got != "" {
			t.Errorf("@operator_marker = %q, want it unset: the server read the operator's ~/.tmux.conf", got)
		}

		wantShell, err := resolveShellPath(cfg.Shell)
		if err != nil {
			t.Fatalf("resolveShellPath: %v", err)
		}
		target := exactSessionWindowTarget(e.SessionName())
		for _, option := range []string{"default-shell", "default-command"} {
			out, err := e.tmux.output("show-options", "-qv", "-t", target, option)
			if err != nil {
				t.Fatalf("show-options -qv %s: %v", option, err)
			}
			if got := strings.TrimSpace(out); got != wantShell {
				t.Errorf("session %s = %q, want the resolved shell %q", option, got, wantShell)
			}
		}

		out, err := e.tmux.output("split-window", "-d", "-P", "-F", "#{pane_start_command}", "-t", target)
		if err != nil {
			t.Fatalf("split-window: %v", err)
		}
		if got := strings.Trim(strings.TrimSpace(out), `"`); got != wantShell {
			t.Errorf("pane_start_command = %q, want the resolved shell %q: the pane started a login shell", got, wantShell)
		}
	})

	t.Run("KitConfigSharesReedsShellPins", func(t *testing.T) {
		const shell = "/bin/sh"
		kitOnly := map[string]bool{"exit-empty": true, "@lyx_test_server": true}
		var kitShellOptions []string
		for _, line := range strings.Split(strings.TrimSpace(tmuxkit.ConfigText(shell)), "\n") {
			fields := strings.Fields(line)
			if len(fields) != 4 || fields[0] != "set" || fields[1] != "-g" {
				t.Fatalf("kit config line %q, want `set -g <option> <value>`", line)
			}
			switch {
			case fields[3] == strconv.Quote(shell):
				kitShellOptions = append(kitShellOptions, fields[2])
			case !kitOnly[fields[2]]:
				t.Errorf("kit config sets %q, want only the shell options and %v", fields[2], kitOnly)
			}
		}

		var reedShellOptions []string
		for _, argv := range shellOptionArgvs("target", shell) {
			reedShellOptions = append(reedShellOptions, argv[3])
		}
		if strings.Join(kitShellOptions, ",") != strings.Join(reedShellOptions, ",") {
			t.Errorf("kit config pins %v to the shell, reed pins %v per session, want the same options", kitShellOptions, reedShellOptions)
		}
	})
}
