// deploy_integration_test.go reads the deploy step's state over a hubforge hub.
// The hub's landing config and prime history decide that state.

package adoptcli_test

import (
	"errors"
	"os"
	"testing"

	"github.com/Knatte18/loomyard/internal/adoptcli"
	"github.com/Knatte18/loomyard/internal/buildvcs"
	"github.com/Knatte18/loomyard/internal/configengine"
	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/gitkit"
	"github.com/Knatte18/loomyard/internal/hubforge"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// seedLandingConfig writes yaml as the hub-wide landing config in h's Bolt checkout and commits it.
// The deploy keys are barred from the worktree layer, which is why the config lives in the Bolt checkout.
func seedLandingConfig(t *testing.T, h *hubforge.Hub, yaml string) {
	t.Helper()
	boltDir := fabricengine.BoltDir(h.Path)
	if err := os.MkdirAll(configengine.ConfigDir(boltDir), 0o755); err != nil {
		t.Fatalf("create the Bolt config directory: %v", err)
	}
	if err := os.WriteFile(configengine.ConfigFile(boltDir, "landing"), []byte(yaml), 0o644); err != nil {
		t.Fatalf("write the landing config: %v", err)
	}
	if _, _, err := fabricengine.NewBolt(boltDir).Commit("test: seed landing config", fabricengine.SyncOptions{}); err != nil {
		t.Fatalf("commit the landing config: %v", err)
	}
}

func TestReadDeployState(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()

	const (
		deployCommand = "go run ./tools/deploy"
		missingCommit = "1111111111111111111111111111111111111111"
	)
	listing := func(branch string) string {
		return "deploy: " + deployCommand + "\ndeploy_branches:\n  - " + branch + "\n"
	}
	errBuildRead := errors.New("not a Go binary")
	tests := []struct {
		name    string
		landing func(branch string) string
		// installed answers the running lym's build given the prime's commit before and at its head;
		// nil fails the test when the build is read.
		installed  func(old, head string) (buildvcs.Identity, error)
		wantEmpty  bool
		wantBehind bool
		wantErr    error
	}{
		{name: "NoDeployCommandIsAnEmptyStateAndReadsNoBuild", landing: func(string) string { return "deploy: \"\"\n" }, wantEmpty: true},
		{name: "ADeployBranchListWithoutThePrimesBranchIsAnEmptyStateAndReadsNoBuild", landing: func(string) string {
			return "deploy: " + deployCommand + "\ndeploy_branches:\n  - elsewhere\n"
		}, wantEmpty: true},
		{name: "ABuildOfAnAncestorOfTheHeadIsBehind", landing: listing, installed: func(old, _ string) (buildvcs.Identity, error) {
			return buildvcs.Identity{Revision: old}, nil
		}, wantBehind: true},
		{name: "ABuildOfTheHeadIsNotBehind", landing: listing, installed: func(_, head string) (buildvcs.Identity, error) {
			return buildvcs.Identity{Revision: head}, nil
		}},
		{name: "ABuildOfACommitThePrimeLacksIsNotBehind", landing: listing, installed: func(string, string) (buildvcs.Identity, error) {
			return buildvcs.Identity{Revision: missingCommit}, nil
		}},
		{name: "ABuildReadErrorIsReturned", landing: listing, installed: func(string, string) (buildvcs.Identity, error) {
			return buildvcs.Identity{}, errBuildRead
		}, wantErr: errBuildRead},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			h := hubforge.NewHub(t, ".")
			prime := h.PrimeWorktree()
			branch := gitkit.CurrentBranch(t, prime)
			old := gitkit.RevParse(t, prime, "HEAD")
			head := gitkit.CommitFile(t, prime, "ahead.txt", "ahead", "ahead of the build")
			seedLandingConfig(t, h, tt.landing(branch))

			readInstalled := func() (buildvcs.Identity, error) {
				if tt.installed == nil {
					t.Errorf("the running lym's build was read for a config that deploys nothing into %s", branch)
					return buildvcs.Identity{}, errors.New("build read")
				}
				return tt.installed(old, head)
			}
			got, err := adoptcli.ReadDeployState(h.Location, readInstalled)
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("ReadDeployState error = %v; want %v", err, tt.wantErr)
			}
			if tt.wantErr != nil {
				return
			}
			want := adoptcli.DeployState{}
			if !tt.wantEmpty {
				installed, _ := tt.installed(old, head)
				want = adoptcli.DeployState{Command: deployCommand, Installed: installed.Label(), Head: head, Behind: tt.wantBehind}
			}
			if got != want {
				t.Errorf("ReadDeployState = %+v; want %+v", got, want)
			}
		})
	}
}
