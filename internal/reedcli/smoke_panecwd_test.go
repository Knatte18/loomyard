//go:build smoke

// smoke_panecwd_test.go pins where a strand's pane actually comes up: at the anchor reed was TOLD
// (Geometry.AnchorPath), never at whatever directory the lyx process happens to be standing in.
// Every other smoke file drives reed with t.Chdir, which makes the two indistinguishable — the cwd
// gate in lyxcwd.Resolve forces process cwd == AnchorPath on that path, so a pane spawned at either
// one looks identical. This file drives the RunCLIIn seam instead, which seeds the cwd into the
// execution context WITHOUT moving the process, so the told anchor and the process cwd are two
// different directories and the distinction becomes observable.

package reedcli

import (
	"bytes"
	"testing"

	"github.com/Knatte18/loomyard/internal/hubforge"
)

// TestSmokeStrandPaneSpawnsAtToldAnchorNotProcessCwd is the regression guard for the pane-cwd
// defect this round's R1 review found: launchStrandLocked's split-window carried no -c, so tmux
// resolved the new pane's cwd from the invoking CLIENT rather than from the anchor reed was told.
// Verified live (tmux 3.6) that a client-issued split lands in the calling process's cwd — neither
// the target pane's cwd nor the session's — so a strand's command ran wherever lyx happened to
// stand. Under t.Chdir that is accidentally the anchor; under the RunCLIIn seam it is not, and
// every strand pane came up in the wrong tree while reed reported success.
//
// The FIRST strand adopts the session's initial pane, which new-session already created with -c, so
// it is correct either way and is asserted only as a control. The SECOND strand is the one that
// exercises the split path and the one the defect broke.
func TestSmokeStrandPaneSpawnsAtToldAnchorNotProcessCwd(t *testing.T) {
	tmuxPath := tmuxBinaryPath(t)

	h := hubforge.NewHub(t, ".")
	deferHubRelease(t, h.PrimeWorktree())
	anchor := h.Location.AnchorPath()

	// The process stands somewhere with no relationship to the hub at all;
	// the worktree reaches reed only through the injected cwd below.
	elsewhere := t.TempDir()
	t.Chdir(elsewhere)

	runIn := func(args ...string) (int, string) {
		t.Helper()
		var out bytes.Buffer
		code := RunCLIIn(anchor, &out, args)
		return code, out.String()
	}

	t.Cleanup(func() {
		var buf bytes.Buffer
		RunCLIIn(anchor, &buf, []string{"down"})
	})

	if code, out := runIn("up"); code != 0 {
		t.Fatalf("up = %d; want 0, output: %s", code, out)
	}

	launch := smokeReapLaunchCmd()
	adopted := addStrandIn(t, anchor, launch, "--name", "adopted")
	split := addStrandIn(t, anchor, launch, "--name", "split")

	socket, session := socketAndSessionIn(t, anchor)
	if session == "" {
		t.Fatalf("status reported no session")
	}

	tests := []struct {
		name string
		guid string
	}{
		{"adopted initial pane (control)", adopted},
		{"split pane", split},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			paneID := paneIDForStrandIn(t, anchor, tt.guid)
			if got := paneCurrentPath(t, tmuxPath, socket, paneID); got != anchor {
				t.Errorf("pane %s current path = %q; want the told anchor %q (process cwd was %q)", paneID, got, anchor, elsewhere)
			}
		})
	}
}
