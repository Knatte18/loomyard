// scope_test.go covers how a --card scope joins a module's main packages to the cards' packages.
// It needs no process and no hub.

package gatecli

import (
	"reflect"
	"testing"

	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

func TestWithMainPackages(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	tests := []struct {
		name    string
		modules []moduleTargets
		dir     string
		mains   []string
		want    []moduleTargets
	}{
		{
			name:    "NoMains",
			modules: []moduleTargets{{dir: ".", targets: []string{"./lib"}}},
			dir:     ".",
			want:    []moduleTargets{{dir: ".", targets: []string{"./lib"}}},
		},
		{
			name:    "AddedToTheModuleTheCardsName",
			modules: []moduleTargets{{dir: ".", targets: []string{"./lib"}}},
			dir:     ".",
			mains:   []string{"./tool"},
			want:    []moduleTargets{{dir: ".", targets: []string{"./lib", "./tool"}}},
		},
		{
			name:    "ListedOnceWhenTheCardsAlreadyNameIt",
			modules: []moduleTargets{{dir: ".", targets: []string{"./tool", "./lib"}}},
			dir:     ".",
			mains:   []string{"./tool", "./cmd/other"},
			want:    []moduleTargets{{dir: ".", targets: []string{"./tool", "./lib", "./cmd/other"}}},
		},
		{
			name:    "ModuleTheCardsLeaveOutJoinsLast",
			modules: []moduleTargets{{dir: "nested", targets: []string{"./n"}}},
			dir:     ".",
			mains:   []string{"./tool"},
			want: []moduleTargets{
				{dir: "nested", targets: []string{"./n"}},
				{dir: ".", targets: []string{"./tool"}},
			},
		},
		{
			name:    "ModuleDirMatchedAfterCleaning",
			modules: []moduleTargets{{dir: "./sub/", targets: []string{"./a"}}},
			dir:     "sub",
			mains:   []string{"./b"},
			want:    []moduleTargets{{dir: "./sub/", targets: []string{"./a", "./b"}}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := withMainPackages(tt.modules, tt.dir, tt.mains); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("withMainPackages() = %+v; want %+v", got, tt.want)
			}
		})
	}
}
