// mains_integration_test.go drives ModuleMainPackages over real modules, whose package listing spawns `go list`.

package impactset

import (
	"context"
	"slices"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

func TestModuleMainPackages(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()

	t.Run("every main package of the module is listed sorted, and no library or testdata package", func(t *testing.T) {
		t.Parallel()
		moduleDir := t.TempDir()
		writeRepoFile(t, moduleDir, "go.mod", "module example.com/m\n\ngo 1.26\n")
		writeRepoFile(t, moduleDir, "main.go", "package main\n\nfunc main() {}\n")
		writeRepoFile(t, moduleDir, "cmd/tool/main.go", "package main\n\nfunc main() {}\n")
		writeRepoFile(t, moduleDir, "lib/lib.go", "package lib\n")
		writeRepoFile(t, moduleDir, "lib/testdata/fixture/main.go", "package main\n\nfunc main() {}\n")

		got, err := ModuleMainPackages(context.Background(), moduleDir)
		if err != nil {
			t.Fatalf("ModuleMainPackages() error = %v; want nil", err)
		}
		if want := []string{".", "./cmd/tool"}; !slices.Equal(got, want) {
			t.Errorf("ModuleMainPackages() = %q; want %q", got, want)
		}
	})

	t.Run("a module with no main package lists none", func(t *testing.T) {
		t.Parallel()
		moduleDir := t.TempDir()
		writeRepoFile(t, moduleDir, "go.mod", "module example.com/m\n\ngo 1.26\n")
		writeRepoFile(t, moduleDir, "lib/lib.go", "package lib\n")

		got, err := ModuleMainPackages(context.Background(), moduleDir)
		if err != nil || len(got) != 0 {
			t.Errorf("ModuleMainPackages() = %q, %v; want no package and nil", got, err)
		}
	})

	t.Run("a main package that fails to load stays listed", func(t *testing.T) {
		t.Parallel()
		moduleDir := t.TempDir()
		writeRepoFile(t, moduleDir, "go.mod", "module example.com/m\n\ngo 1.26\n")
		writeRepoFile(t, moduleDir, "broken/main.go", "package main\n\nimport _ \"embed\"\n\n//go:embed missing.txt\nvar missing string\n\nfunc main() {}\n")

		got, err := ModuleMainPackages(context.Background(), moduleDir)
		if err != nil {
			t.Fatalf("ModuleMainPackages() error = %v; want nil", err)
		}
		if want := []string{"./broken"}; !slices.Equal(got, want) {
			t.Errorf("ModuleMainPackages() = %q; want %q", got, want)
		}
	})

	t.Run("a directory with no go.mod is an error naming it", func(t *testing.T) {
		t.Parallel()
		bare := t.TempDir()
		if _, err := ModuleMainPackages(context.Background(), bare); err == nil || !strings.Contains(err.Error(), bare) {
			t.Errorf("ModuleMainPackages(%s) error = %v; want an error naming the directory", bare, err)
		}
	})
}
