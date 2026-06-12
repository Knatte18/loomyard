// daemon.go — daemon subcommand: foreground poller with crash-loop guard.
//
// cmdDaemon runs a long-lived foreground process that polls the psmux session
// at regular intervals. If the session dies, it attempts to recover it (up to
// maxRecoveries times within a windowDur window). Recoveries are tracked in a
// daemon-process-local slice that resets on daemon restart.

package muxpoc

import (
	"fmt"
	"io"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/Knatte18/mhgo/internal/output"
)

const (
	maxRecoveries = 3
	windowDur     = 60 * time.Second
)

// cmdDaemon runs a foreground polling loop that monitors and recovers the
// psmux session. Returns 0 only on clean signal shutdown; returns output.Err
// on unrecoverable state errors.
func cmdDaemon(out io.Writer, cfg Config) int {
	cwd, _ := os.Getwd()
	mux := NewPsmuxCmd(cfg)

	// Crash-loop guard: maintain a ring of recovery timestamps
	var recoveries []time.Time

	// Set up OS signal handling
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)

	// Set up ticker for polling
	ticker := time.NewTicker(cfg.Interval)
	defer ticker.Stop()

	// Print start message to stderr
	fmt.Fprintf(os.Stderr, "muxpoc daemon started, polling every %v\n", cfg.Interval)

	// Main loop
	for {
		select {
		case <-sigCh:
			// Clean signal shutdown
			fmt.Fprintf(os.Stderr, "daemon stopping\n")
			return output.Ok(out, map[string]any{"message": "daemon stopped"})

		case <-ticker.C:
			// Poll for session health
			state, warn, err := LoadState(cwd)
			if err != nil {
				fmt.Fprintf(os.Stderr, "error loading state: %v\n", err)
				return output.Err(out, fmt.Sprintf("load state: %v", err))
			}
			if warn != "" {
				fmt.Fprintf(os.Stderr, "%s\n", warn)
			}

			// If no state, nothing to watch
			if state == nil {
				continue
			}

			// Check if session is still alive
			up, err := mux.hasSession(state.Session)
			if err != nil {
				fmt.Fprintf(os.Stderr, "error checking session: %v\n", err)
				continue
			}

			// Session is healthy
			if up {
				continue
			}

			// Session is dead. Check crash-loop guard.
			// Prune recoveries to entries within the last windowDur
			now := time.Now()
			prunedRecoveries := []time.Time{}
			for _, ts := range recoveries {
				if now.Sub(ts) < windowDur {
					prunedRecoveries = append(prunedRecoveries, ts)
				}
			}
			recoveries = prunedRecoveries

			// Check if we've hit the recovery limit
			if len(recoveries) >= maxRecoveries {
				fmt.Fprintf(os.Stderr, "crash-loop detected (>= %d recoveries in %s), giving up on session %s\n",
					maxRecoveries, windowDur, state.Session)
				continue
			}

			// Attempt recovery
			recoveries = append(recoveries, now)
			fmt.Fprintf(os.Stderr, "session %s died, recovering (attempt %d)\n",
				state.Session, len(recoveries))

			// Call coldRecover with io.Discard to suppress JSON output
			exitCode := coldRecover(io.Discard, cfg, cwd, state, mux)
			if exitCode == 0 {
				fmt.Fprintf(os.Stderr, "recovery complete\n")
			} else {
				fmt.Fprintf(os.Stderr, "recovery failed\n")
			}
		}
	}
}
