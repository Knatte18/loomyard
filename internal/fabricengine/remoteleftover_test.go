// remoteleftover_test.go pins the delete command a leftover refusal names, which must reach the leftover's origin ref.

package fabricengine

import (
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

func TestErrRemoteLeftoverError_NamesTheDeleteOfTheFullOriginRef(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	tests := []struct {
		name       string
		err        *ErrRemoteLeftover
		wantDelete string
		wantAbsent string
	}{
		{
			"WeftBranchInTheOneRepositoryNamespace",
			&ErrRemoteLeftover{Slug: "s", Branch: "s-weft", RemoteTip: "abc", kind: leftoverDivergedAdoptedWeft, originRef: "refs/lym/heads/s-weft"},
			`"git push origin --delete refs/lym/heads/s-weft"`,
			`--delete s-weft`,
		},
		{
			"WeftBranchInTheIdentityNamespace",
			&ErrRemoteLeftover{Slug: "s", Branch: "s-weft", RemoteTip: "abc", kind: leftoverDivergedAdoptedWeft, originRef: "refs/heads/s-weft"},
			`"git push origin --delete refs/heads/s-weft"`,
			"",
		},
		{
			"WarpBranchKeepsItsShortName",
			&ErrRemoteLeftover{Slug: "s", Branch: "s", RemoteTip: "abc", kind: leftoverDivergedWarp},
			`"git push origin --delete s"`,
			"",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := tt.err.Error()
			if !strings.Contains(got, tt.wantDelete) {
				t.Errorf("Error() = %q; want it to hold %s", got, tt.wantDelete)
			}
			if tt.wantAbsent != "" && strings.Contains(got, tt.wantAbsent) {
				t.Errorf("Error() = %q; want it free of %s", got, tt.wantAbsent)
			}
		})
	}
}
