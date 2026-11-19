// serveopts_test.go tests the serve and open verb wiring that no refusal test reaches:
// which strand names pass the refusal and the opener wait of a named caller.
// It also covers the board and the process record the server runs with.

package boardcli

import (
	"os"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/agentname"
	"github.com/Knatte18/loomyard/internal/boardengine"
	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/lymcwd"
	"github.com/Knatte18/loomyard/internal/runproc"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// hubLocation returns a resolved hub worktree location over a temp hub.
func hubLocation(t *testing.T) *lymcwd.Location {
	t.Helper()
	return &lymcwd.Location{HubPath: t.TempDir(), RepoName: "repo", WorktreeName: "prime", AnchorRel: "."}
}

// TestRefuseBoardVerb pins which callers pass the refusal of serve and open.
// It is not parallel: it sets LYM_STRAND_NAME.
func TestRefuseBoardVerb(t *testing.T) {
	tiergate.Require(t, 0)

	tests := []struct {
		name     string
		strand   string
		resolved bool
		want     string
	}{
		{"NoStrandNamePasses", "", true, ""},
		{"OrchStrandNamePasses", "ly:orch", true, ""},
		{"RunAgentIsRefused", "ly:my-task:webster", true, "the operator's tools"},
		{"NoResolvedHubIsRefused", "", false, "needs the hub's board config"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv(agentname.StrandNameEnv, tt.strand)
			var location *lymcwd.Location
			if tt.resolved {
				location = hubLocation(t)
			}
			err := refuseBoardVerb("serve", location)
			switch {
			case tt.want == "" && err != nil:
				t.Errorf("refuseBoardVerb = %v; want a pass", err)
			case tt.want != "" && (err == nil || !strings.Contains(err.Error(), tt.want)):
				t.Errorf("refuseBoardVerb = %v; want a refusal holding %q", err, tt.want)
			}
		})
	}
}

// TestProductionOpenDeps pins that a named caller's opener wait takes the keep-warm cap and an unnamed one has none.
// It is not parallel: it sets LYM_STRAND_NAME.
func TestProductionOpenDeps(t *testing.T) {
	tiergate.Require(t, 0)

	tests := []struct {
		name    string
		strand  string
		wantCap bool
	}{
		{"UnnamedCallerWaitsForTheOpenerToExit", "", false},
		{"NamedCallerIsCappedByTheKeepWarmResolver", "ly:orch", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv(agentname.StrandNameEnv, tt.strand)
			deps := productionOpenDeps(hubLocation(t))
			if got := deps.OpenerCap != 0; got != tt.wantCap {
				t.Errorf("OpenerCap = %v; want a non-zero cap: %v", deps.OpenerCap, tt.wantCap)
			}
		})
	}
}

// TestServeOptions pins the board and the process record the server runs with.
func TestServeOptions(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	hub := t.TempDir()
	boardConfig := boardengine.Config{
		Path:         t.TempDir(),
		Readme:       "README.md",
		DesignPrefix: "design-",
		Types:        []boardengine.Label{{Name: "bug"}},
		SkipGit:      true,
		StrandName:   "ly:my-task:webster",
	}
	opts := serveOptions(boardConfig, hub)

	t.Run("BoardCarriesNoStrandName", func(t *testing.T) {
		t.Parallel()
		// A run agent's strand name refuses a task creation, so an accepted one shows the name was cleared.
		fields := map[string]any{"slug": "new-task", "kind": "task", "title": "New", "labels": []any{"bug"}}
		if _, err := opts.Board.UpsertTask(fields); err != nil {
			t.Errorf("UpsertTask through the server's board = %v; want the operator's write accepted", err)
		}
	})

	t.Run("RegisterRecordsTheBoardServerUnderTheHubOwner", func(t *testing.T) {
		t.Parallel()
		if err := opts.Register(); err != nil {
			t.Fatalf("Register: %v", err)
		}
		records, err := runproc.List(fabricengine.RunProcDir(hub), runproc.HubOwner)
		if err != nil {
			t.Fatalf("List: %v", err)
		}
		if len(records) != 1 || records[0].Role != runproc.RoleBoardServer || records[0].PID != os.Getpid() {
			t.Errorf("records under the hub owner = %+v; want this process once, as the board server", records)
		}
	})
}
