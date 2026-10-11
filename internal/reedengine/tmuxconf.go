// tmuxconf.go — the operator tmux config feature: the `tmux_conf` key's validation and the boot guard that sources it.

package reedengine

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"unicode"

	"github.com/Knatte18/loomyard/internal/logger"
)

// tmuxConfEnv is the environment variable the `tmux_conf` template line reads.
const tmuxConfEnv = "LYX_REED_TMUX_CONF"

// tmuxConfForbiddenChars are the non-whitespace characters a tmux config path may not hold:
// source-file treats its argument as a glob, and the boot nests the path in a command string tmux parses a second time.
const tmuxConfForbiddenChars = `*?[\'";#{}$`

// tmuxConfPath validates the `tmux_conf` value and returns the path to source.
// An empty raw returns the empty string.
// A leading `~/` expands to home; the result must be an absolute path to an existing regular file holding no whitespace and none of the characters tmux would reinterpret.
// Every error names the key `tmux_conf`, the variable tmuxConfEnv and the path.
func tmuxConfPath(raw, home string) (string, error) {
	if raw == "" {
		return "", nil
	}
	path := raw
	if strings.HasPrefix(path, "~/") {
		path = home + path[1:]
	}
	refuse := func(reason string) (string, error) {
		return "", fmt.Errorf("reed: the configured tmux config %q (key tmux_conf, variable %s) %s; set tmux_conf in reed.yaml to an absolute path to an existing regular file, or leave it empty", path, tmuxConfEnv, reason)
	}
	if !filepath.IsAbs(path) {
		return refuse("is not an absolute path")
	}
	if strings.ContainsFunc(path, func(r rune) bool { return unicode.IsSpace(r) || strings.ContainsRune(tmuxConfForbiddenChars, r) }) {
		return refuse("holds whitespace or one of the characters " + tmuxConfForbiddenChars)
	}
	info, err := os.Stat(path)
	if err != nil {
		return refuse("cannot be found: " + err.Error())
	}
	if !info.Mode().IsRegular() {
		return refuse("is not a regular file")
	}
	return path, nil
}

// tmuxConfClaimedOption is the server option the boot that wins the claim sets: `sourced` when it sourced a file, `empty` when it had none.
const tmuxConfClaimedOption = "@lyx_tmux_conf_claimed"

// reedSessionOption is the session option every reed session carries from its creation, which the session filter reads.
const reedSessionOption = "@lyx_reed_session"

// serverIdentityLocked returns the answer of the server's `#{pid} #{start_time}`, which names one server incarnation.
// It returns the empty string when the round trip fails or prints nothing, as for a socket with no server.
func (e *Engine) serverIdentityLocked() string {
	out, err := e.tmux.output("display-message", "-p", "#{pid} #{start_time}")
	if err != nil {
		return ""
	}
	return strings.TrimSpace(out)
}

// bootGuardArgv is the one chained tmux invocation of the boot guard.
// A candidate boot opens with an `if-shell` that claims the server's one-time source, racing other candidates on the server option, and sources confPath when it wins and confPath is non-empty.
// Every guard ends by pinning `destroy-unattached` and `exit-unattached` off, so the operator file cannot end the server.
func bootGuardArgv(candidate bool, confPath string) []string {
	var argv []string
	if candidate {
		claim := "set-option -s " + tmuxConfClaimedOption + " empty"
		if confPath != "" {
			claim = "set-option -s " + tmuxConfClaimedOption + " sourced ; source-file " + confPath
		}
		argv = append(argv, "if-shell", "-F", "#{"+tmuxConfClaimedOption+"}", "", claim, ";")
	}
	return append(argv,
		"set-option", "-g", "destroy-unattached", "off", ";",
		"set-option", "-s", "exit-unattached", "off",
	)
}

// guardBootSessionLocked runs the boot guard on the server the boot just reached, then reads back that this session and both pins survived the operator file.
// The boot is a candidate for the source when the server's identity now differs from identityBefore, the one read before the spawn.
// The guard invocation's exit status is only ever a warning, since one status carries the claim, the source and both pins; the read-back decides.
// It fails when the session is gone, naming the key, the variable and confPath, or when either pin does not read `off`.
func (e *Engine) guardBootSessionLocked(identityBefore, confPath string) error {
	candidate := e.serverIdentityLocked() != identityBefore
	logger.Info("reed: tmux config claim", "socket", e.Socket(), "session", e.SessionName(), "candidate", candidate, "path", confPath)

	if err := e.tmux.run(bootGuardArgv(candidate, confPath)...); err != nil {
		if candidate && confPath != "" {
			logger.Warn("reed: the tmux config did not source cleanly", "socket", e.Socket(), "path", confPath, "err", err)
			fmt.Fprintf(e.warnOut, "reed: warning: sourcing tmux_conf %s failed: %v\n", confPath, err)
		} else {
			logger.Warn("reed: the boot guard did not run cleanly", "socket", e.Socket(), "err", err)
		}
	}

	up, err := e.tmux.hasSession(e.SessionName())
	if err != nil {
		return fmt.Errorf("check session after the boot guard: %w", err)
	}
	if !up {
		return fmt.Errorf("reed: session %q ended while the tmux config %q (key tmux_conf, variable %s) was sourced; remove from the file the command that ends the session, such as an untargeted `set destroy-unattached on` or a `kill-session`", e.SessionName(), confPath, tmuxConfEnv)
	}

	out, err := e.tmux.output("show-options", "-gv", "destroy-unattached", ";", "show-options", "-sv", "exit-unattached")
	if err != nil {
		return fmt.Errorf("read back the boot guard pins: %w", err)
	}
	if lines := strings.Fields(out); len(lines) != 2 || lines[0] != "off" || lines[1] != "off" {
		return fmt.Errorf("reed: destroy-unattached and exit-unattached must read off after the boot guard, got %q (key tmux_conf, variable %s, path %q)", strings.TrimSpace(out), tmuxConfEnv, confPath)
	}
	return nil
}
