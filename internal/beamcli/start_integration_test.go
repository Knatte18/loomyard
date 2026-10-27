//go:build tier1

// start_integration_test.go runs "lym beam start" through RunCLIIn over a hubforge hub, on every target it refuses before the hub is reconciled or an orch is started.

package beamcli_test

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/Knatte18/loomyard/internal/beamcli"
	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/gitkit"
	"github.com/Knatte18/loomyard/internal/hubforge"
	"github.com/Knatte18/loomyard/internal/hubreconcile"
	"github.com/Knatte18/loomyard/internal/testkit/envelope"
)

func TestStart_RefusesBeforeTheReconcile(t *testing.T) {
	t.Parallel()

	h := hubforge.CopyHub(t, hubforge.Shape{Anchor: "."})
	hubforge.AddPairWith(t, h, "b", fabricengine.AddOptions{SkipPush: true, Beam: true})
	hubforge.AddPairWith(t, h, "task-in-b", fabricengine.AddOptions{SkipPush: true, Parent: "b"})
	hubforge.AddPair(t, h, "task-in-root")
	gitkit.Git(t, h.PrimeWorktree(), "push", "origin", "HEAD:refs/heads/remote-only")
	outside := t.TempDir()

	tests := []struct {
		name string
		cwd  string
		args []string
		want []string
	}{
		{
			name: "TaskPairOfABeamNamesTheBeamAndItsPath",
			cwd:  h.PairCodeWorktree("task-in-b"),
			args: []string{"start"},
			want: []string{"task-in-b is a task pair", "belongs to b", h.PairCodeWorktree("b"), "lym beam start"},
		},
		{
			name: "TaskPairOfTheRootNamesRootAndThePrime",
			cwd:  h.PairCodeWorktree("task-in-root"),
			args: []string{"start"},
			want: []string{"task-in-root is a task pair", "belongs to root", h.PrimeWorktree(), "lym beam start"},
		},
		{
			name: "NamedTaskPairNamesItsBeam",
			cwd:  h.PrimeWorktree(),
			args: []string{"start", "task-in-b"},
			want: []string{"task-in-b is a task pair", "belongs to b", h.PairCodeWorktree("b")},
		},
		{
			name: "UnknownSlugNamesBeamCreateInItsParent",
			cwd:  h.PrimeWorktree(),
			args: []string{"start", "ghost"},
			want: []string{"ghost has no pair on this machine", "lym beam create ghost", "parent worktree"},
		},
		{
			name: "UnknownSlugOnOriginNamesAdopt",
			cwd:  h.PrimeWorktree(),
			args: []string{"start", "remote-only"},
			want: []string{"remote-only has no pair on this machine", "lym adopt remote-only"},
		},
		{
			name: "SlugWithAllNamesBothWaysForward",
			cwd:  h.PrimeWorktree(),
			args: []string{"start", "b", "--all"},
			want: []string{"exclusive", "drop the slug", "drop --all"},
		},
		{
			name: "OutsideAHubNamesWhereToRun",
			cwd:  outside,
			args: []string{"start"},
			want: []string{"not a git repository", "prime worktree or from a beam"},
		},
	}
	for _, tt := range tests {
		// The rows share one hub, so they run in sequence.
		t.Run(tt.name, func(t *testing.T) {
			var out bytes.Buffer
			exit := beamcli.RunCLIIn(tt.cwd, &out, tt.args)

			if exit == 0 {
				t.Errorf("lym beam %v exit = 0; want non-zero", tt.args)
			}
			for _, fragment := range tt.want {
				envelope.RequireErr(t, out.String(), fragment)
			}
			for _, worktree := range []string{h.PrimeWorktree(), h.PairCodeWorktree("b"), h.PairCodeWorktree("task-in-b"), h.PairCodeWorktree("task-in-root")} {
				orchDir := filepath.Join(worktree, ".lym", "orch")
				if _, err := os.Stat(orchDir); !os.IsNotExist(err) {
					t.Errorf("Stat(%s) = %v; want no orch state written", orchDir, err)
				}
			}
			stamp := hubreconcile.Geometry{BoardDir: h.BoardDir()}.StampPath()
			if _, err := os.Stat(stamp); !os.IsNotExist(err) {
				t.Errorf("Stat(%s) = %v; want the hub left unreconciled", stamp, err)
			}
		})
	}
}
