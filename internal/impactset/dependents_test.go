// dependents_test.go drives the pure core of DependentPackages over the shared hand-built package graph.

package impactset

import (
	"reflect"
	"testing"

	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

func TestDependentsFor(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	tests := []struct {
		name     string
		packages []string
		want     []string
	}{
		{
			name:     "a package gains every package that reaches it, a test import included, sorted after it",
			packages: []string{"./internal/a"},
			want:     []string{"./internal/a", "./cmd/lym", "./internal/b", "./internal/c", "./internal/tmuxonly"},
		},
		{
			name:     "a package nothing reaches stays alone",
			packages: []string{"./internal/d"},
			want:     []string{"./internal/d"},
		},
		{
			name:     "a package the module does not hold yet stays first and adds nothing",
			packages: []string{"./internal/new", "./internal/b"},
			want:     []string{"./internal/new", "./internal/b", "./cmd/lym", "./internal/c"},
		},
		{
			name:     "the cmd/lym main package gains the packages whose tests build the binary",
			packages: []string{"./cmd/lym"},
			want:     []string{"./cmd/lym", "./internal/e"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := dependentsFor(testGraph(), tt.packages); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("dependentsFor(%q) = %q; want %q", tt.packages, got, tt.want)
			}
		})
	}
}
