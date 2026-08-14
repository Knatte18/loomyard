// registry_test.go pins the hand-maintained registry in stencils.go against the on-disk *.md tree:
// every family subfolder's stencil file must have a matching entries row, and every entries row must
// have a matching file, in both directions.
// Without this test a hand-maintained registry could silently omit a file -- invisible to
// `lyx stencil list`, never seeded, never validated.

package stencils

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/stencilstore"
)

// stencilNamesOnDisk walks this package's own directory and returns the stencil name derived from
// each *.md file found in a family subfolder: the filename without its ".md" extension.
func stencilNamesOnDisk(t *testing.T) []string {
	t.Helper()

	packageDir, err := os.Getwd()
	if err != nil {
		t.Fatalf("os.Getwd() = _, %v; want nil error", err)
	}

	entries, err := os.ReadDir(packageDir)
	if err != nil {
		t.Fatalf("os.ReadDir(%q) = _, %v; want nil error", packageDir, err)
	}

	var names []string
	for _, familyEntry := range entries {
		if !familyEntry.IsDir() {
			continue
		}
		familyDir := filepath.Join(packageDir, familyEntry.Name())
		familyFiles, err := os.ReadDir(familyDir)
		if err != nil {
			t.Fatalf("os.ReadDir(%q) = _, %v; want nil error", familyDir, err)
		}
		for _, fileEntry := range familyFiles {
			if fileEntry.IsDir() || !strings.HasSuffix(fileEntry.Name(), ".md") {
				continue
			}
			names = append(names, strings.TrimSuffix(fileEntry.Name(), ".md"))
		}
	}
	return names
}

// TestRegistry_MatchesOnDiskTree verifies the registry and the on-disk *.md tree name exactly the
// same stencils in both directions: a .md present but unregistered fails, and a registered name with
// no .md fails.
func TestRegistry_MatchesOnDiskTree(t *testing.T) {
	onDisk := stencilNamesOnDisk(t)
	registered := Registry().Names()

	onDiskSet := make(map[string]bool, len(onDisk))
	for _, name := range onDisk {
		onDiskSet[name] = true
	}
	registeredSet := make(map[string]bool, len(registered))
	for _, name := range registered {
		registeredSet[name] = true
	}

	var unregistered []string
	for _, name := range onDisk {
		if !registeredSet[name] {
			unregistered = append(unregistered, name)
		}
	}
	sort.Strings(unregistered)
	if len(unregistered) > 0 {
		t.Errorf("stencils present on disk with no registry entry: %v", unregistered)
	}

	var fileless []string
	for _, name := range registered {
		if !onDiskSet[name] {
			fileless = append(fileless, name)
		}
	}
	sort.Strings(fileless)
	if len(fileless) > 0 {
		t.Errorf("stencils registered with no matching .md file on disk: %v", fileless)
	}
}

// TestRegistry_DefaultsAndRelPathAreConsistent verifies every registered entry's Default returns
// non-empty bytes, and that stencilstore.RelPath(name) resolves to the file's actual relative path --
// pinning the family-from-first-token derivation against the on-disk layout.
func TestRegistry_DefaultsAndRelPathAreConsistent(t *testing.T) {
	packageDir, err := os.Getwd()
	if err != nil {
		t.Fatalf("os.Getwd() = _, %v; want nil error", err)
	}

	reg := Registry()
	for _, name := range reg.Names() {
		def, known := reg.Default(name)
		if !known {
			t.Errorf("Registry().Default(%q) = _, false; want true for a name Registry().Names() returned", name)
			continue
		}
		if len(def) == 0 {
			t.Errorf("Registry().Default(%q) = <empty>, true; want non-empty bytes", name)
		}

		relPath := stencilstore.RelPath(name)
		absPath := filepath.Join(packageDir, filepath.FromSlash(relPath))
		if _, err := os.Stat(absPath); err != nil {
			t.Errorf("stencilstore.RelPath(%q) = %q, resolved to %q, which does not exist: %v", name, relPath, absPath, err)
		}
	}
}
