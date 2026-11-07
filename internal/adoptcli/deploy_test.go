// deploy_test.go pins the deploy step's behavior:
// how it judges the running lym's build against the prime's head, and how it reports a failed deploy command.

package adoptcli

import (
	"errors"
	"testing"

	"github.com/Knatte18/loomyard/internal/battenshed"
	"github.com/Knatte18/loomyard/internal/buildvcs"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

func TestDeployFailure(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	errStart := errors.New("no shell")
	tests := []struct {
		name    string
		result  battenshed.DeployResult
		err     error
		wantErr string
	}{
		{name: "AShellThatCouldNotStartIsAnErrorWrappingTheCause", err: errStart, wantErr: "the shell could not start: no shell"},
		{name: "ATimeoutNamesTheDeployTimeout", result: battenshed.DeployResult{TimedOut: true}, wantErr: "timed out after " + battenshed.DeployTimeout.String()},
		{name: "ANonZeroExitCodeIsNamed", result: battenshed.DeployResult{ExitCode: 2}, wantErr: "exit code 2"},
		{name: "AZeroExitCodeIsNoError"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := deployFailure(tt.result, tt.err)
			switch {
			case tt.wantErr == "" && got != nil:
				t.Fatalf("deployFailure(%+v, %v) = %v; want nil", tt.result, tt.err, got)
			case tt.wantErr != "" && (got == nil || got.Error() != tt.wantErr):
				t.Fatalf("deployFailure(%+v, %v) = %v; want %q", tt.result, tt.err, got, tt.wantErr)
			}
			if tt.err != nil && !errors.Is(got, tt.err) {
				t.Errorf("deployFailure(%+v, %v) = %v; want it to wrap the start error", tt.result, tt.err, got)
			}
		})
	}
}

func TestInstalledBehind(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	const (
		head = "2d319fe5aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
		old  = "87553450ec14bbbbbbbbbbbbbbbbbbbbbbbbbbbb"
	)
	errAncestry := errors.New("merge-base broke")
	tests := []struct {
		name        string
		installed   buildvcs.Identity
		present     bool
		isAncestor  bool
		ancestorErr error
		want        bool
		wantErr     bool
	}{
		{name: "AnAncestorOfTheHeadIsBehind", installed: buildvcs.Identity{Revision: old}, present: true, isAncestor: true, want: true},
		{name: "AModifiedBuildOfAnAncestorIsBehind", installed: buildvcs.Identity{Revision: old, Modified: true}, present: true, isAncestor: true, want: true},
		{name: "ABuildOfTheHeadIsNotBehind", installed: buildvcs.Identity{Revision: head}, present: true, isAncestor: true},
		{name: "ACommitTheRepositoryLacksIsNotBehind", installed: buildvcs.Identity{Revision: old}, isAncestor: true},
		{name: "ACommitOffTheHeadsHistoryIsNotBehind", installed: buildvcs.Identity{Revision: old}, present: true},
		{name: "ABuildWithNoStampIsAnError", wantErr: true},
		{name: "AnAncestryReadErrorIsReturned", installed: buildvcs.Identity{Revision: old}, present: true, ancestorErr: errAncestry, wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			exists := func(string) bool { return tt.present }
			ancestor := func(string, string) (bool, error) { return tt.isAncestor, tt.ancestorErr }
			got, err := installedBehind(tt.installed, head, exists, ancestor)
			if (err != nil) != tt.wantErr {
				t.Fatalf("installedBehind(%+v) error = %v; want error %v", tt.installed, err, tt.wantErr)
			}
			if got != tt.want {
				t.Errorf("installedBehind(%+v) = %v; want %v", tt.installed, got, tt.want)
			}
		})
	}
}
