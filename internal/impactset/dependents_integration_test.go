// dependents_integration_test.go drives DependentPackages over real modules, whose package listing spawns `go list`.

package impactset

import (
	"context"
	"slices"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

func TestDependentPackages(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()

	t.Run("a package gains its importer and a package whose tagged test imports it, and no unrelated package", func(t *testing.T) {
		t.Parallel()
		moduleDir := t.TempDir()
		writeRepoFile(t, moduleDir, "go.mod", "module example.com/m\n\ngo 1.26\n")
		writeRepoFile(t, moduleDir, "lib/lib.go", "package lib\n\nfunc One() int { return 1 }\n")
		writeRepoFile(t, moduleDir, "user/user.go", "package user\n\nimport \"example.com/m/lib\"\n\nfunc Two() int { return lib.One() + 1 }\n")
		writeRepoFile(t, moduleDir, "probe/probe.go", "package probe\n")
		writeRepoFile(t, moduleDir, "probe/probe_test.go", "//go:build tier1\n\npackage probe\n\nimport (\n\t\"testing\"\n\n\t\"example.com/m/lib\"\n)\n\nfunc TestOne(t *testing.T) { _ = lib.One() }\n")
		writeRepoFile(t, moduleDir, "other/other.go", "package other\n")

		got, err := DependentPackages(context.Background(), moduleDir, []string{"./lib"})
		if err != nil {
			t.Fatalf("DependentPackages() error = %v; want nil", err)
		}
		if want := []string{"./lib", "./probe", "./user"}; !slices.Equal(got, want) {
			t.Errorf("DependentPackages() = %q; want %q", got, want)
		}
	})

	t.Run("a directory with no go.mod is an error naming it", func(t *testing.T) {
		t.Parallel()
		bare := t.TempDir()
		if _, err := DependentPackages(context.Background(), bare, []string{"./lib"}); err == nil || !strings.Contains(err.Error(), bare) {
			t.Errorf("DependentPackages(%s) error = %v; want an error naming the directory", bare, err)
		}
	})
}
