package fabricengine

import (
	"testing"

	"github.com/Knatte18/loomyard/internal/buildvcs"
)

func TestSeedCommitSignature(t *testing.T) {
	t.Parallel()

	id := buildvcs.Identity{Revision: "0123456789abcdef0123456789abcdef01234567"}
	label := binaryLabel(id, "production", "/bin/lyx")
	stencilPath := StencilsSubtreeRel() + "/loom/a.md"
	specPath := SpecsSubtreeRel() + "/loom/spec.md"

	equal := []struct {
		name string
		got  string
		want string
	}{
		{"label with channel", label, "(" + id.Label() + " production /bin/lyx)"},
		{"label with empty channel", binaryLabel(id, "", "/bin/lyx"), "(" + id.Label() + " unstamped /bin/lyx)"},
		{"label of an unstamped identity", binaryLabel(buildvcs.Identity{}, "dev", "/bin/lyx"), "(unknown dev /bin/lyx)"},
		{"seed subject", SeedCommitMessage("stencils", label), "lyx: seed stencils " + label},
		{"sync subject", SyncCommitMessage("specs", label), "lyx: sync specs " + label},
	}
	for _, tc := range equal {
		if tc.got != tc.want {
			t.Errorf("%s: got %q, want %q", tc.name, tc.got, tc.want)
		}
	}

	recognizer := []struct {
		name    string
		message string
		paths   []string
		want    bool
	}{
		{"labelled seed of stencils", SeedCommitMessage("stencils", label), []string{stencilPath}, true},
		{"labelled seed with a body", SeedCommitMessage("specs", label) + "\n\nbody", []string{stencilPath, specPath}, true},
		{"bare seed from a pre-change binary", "lyx: seed stencils", []string{stencilPath}, false},
		{"empty label", "lyx: seed stencils ()", []string{stencilPath}, false},
		{"sync commit", SyncCommitMessage("stencils", label), []string{stencilPath}, false},
		{"hub-wide config commit", "lyx: hub config " + label, []string{"_lyx/config/x.yaml"}, false},
		{"path outside the subtrees", SeedCommitMessage("stencils", label), []string{stencilPath, "board.json"}, false},
		{"no paths", SeedCommitMessage("stencils", label), nil, false},
	}
	for _, tc := range recognizer {
		if got := IsSeedCommit(tc.message, tc.paths); got != tc.want {
			t.Errorf("%s: IsSeedCommit = %v, want %v", tc.name, got, tc.want)
		}
	}
}
