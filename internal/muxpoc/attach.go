// attach.go — socket derivation and the (best-effort) visible-window attach.
//
// Per design decision 6, mhgo does not own OS window management beyond popping ONE
// maximized terminal attached to the session. That pop is the only place muxpoc launches
// something VISIBLE (not windowless): a Windows Terminal running pwsh that attaches to the
// psmux session. Everything else stays windowless. Best-effort by design.
package muxpoc

import (
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"
)

// socketFor derives a stable, per-repo psmux socket label from the cwd, so each repo's
// muxpoc lives on its own isolated server and never touches the operator's real psmux.
func socketFor(cwd string) string {
	base := filepath.Base(cwd)
	var b strings.Builder
	b.WriteString("muxpoc-")
	for _, c := range base {
		if (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') {
			b.WriteRune(c)
		} else {
			b.WriteByte('-')
		}
	}
	return b.String()
}

// opAttach pops a maximized Windows Terminal attached to the session. It is intentionally
// VISIBLE (not windowless) and detached, and is best-effort: failures to find `wt` are
// reported but not fatal to the rest of muxpoc.
func opAttach(cfg Config, cwd string) (map[string]any, error) {
	r := &Runner{Bin: cfg.Psmux, Socket: socketFor(cwd)}
	st, have, err := loadState(cwd)
	if err != nil {
		return nil, err
	}
	if !have || !r.hasSession(st.Session) {
		return nil, fmt.Errorf("no running muxpoc session; run `mhgo muxpoc up` first")
	}
	// pwsh -NoExit -NoProfile -Command "& 'psmux' -L <socket> attach -t <session>"
	attachCmd := fmt.Sprintf("& '%s' -L %s attach -t %s", cfg.Psmux, r.Socket, st.Session)
	cmd := exec.Command("wt",
		"-w", "new", "--maximized", "--title", "muxpoc",
		cfg.Pwsh, "-NoExit", "-NoProfile", "-Command", attachCmd)
	// Deliberately NOT windowless — this is the one visible pop.
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("could not launch Windows Terminal (wt): %w", err)
	}
	return map[string]any{"action": "attach", "session": st.Session, "socket": r.Socket}, nil
}
