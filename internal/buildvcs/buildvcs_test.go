// buildvcs_test.go covers the mapping from build settings onto an Identity.

package buildvcs

import (
	"runtime/debug"
	"testing"
)

func TestIdentityOf(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		settings []debug.BuildSetting
		want     Identity
	}{
		{"both keys", []debug.BuildSetting{{Key: "vcs.revision", Value: "abc"}, {Key: "vcs.modified", Value: "true"}}, Identity{Revision: "abc", Modified: true}},
		{"modified false", []debug.BuildSetting{{Key: "vcs.revision", Value: "abc"}, {Key: "vcs.modified", Value: "false"}}, Identity{Revision: "abc"}},
		{"no vcs keys", nil, Identity{}},
		{"unrelated keys only", []debug.BuildSetting{{Key: "GOOS", Value: "linux"}, {Key: "-ldflags", Value: "-s"}}, Identity{}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := identityOf(tt.settings); got != tt.want {
				t.Errorf("identityOf() = %+v, want %+v", got, tt.want)
			}
		})
	}
}
