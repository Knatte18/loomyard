// registry_test.go pins the hand-maintained registry in stencils.go against the on-disk *.md tree: every family subfolder's stencil file must have a matching entries row, and every entries row must have a matching file, in both directions.
// Without this test a hand-maintained registry could silently omit a file -- invisible to `lym stencil list`, never seeded, never validated.

package stencils

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/contracts/orchrole"
	"github.com/Knatte18/loomyard/internal/stencilstore"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// fixedStencils holds the instruction texts lym renders from embedded bytes alone and never seeds to a board copy, keyed by name.
// They live outside this package's tree, so the registry never lists them, and every scan over the shipped stencils reads them through shippedStencils.
var fixedStencils = fixedStencilTexts()

// fixedStencilTexts returns the orch role's text and each of its on-demand files' texts, the latter named `orch-` and the file's name without its extension.
func fixedStencilTexts() map[string][]byte {
	texts := map[string][]byte{"orch-role": orchrole.Role}
	for file, text := range orchrole.OnDemand {
		texts["orch-"+strings.TrimSuffix(file, ".md")] = text
	}
	return texts
}

// shippedStencils returns every registered stencil's shipped default and every fixed stencil's text, keyed by name.
func shippedStencils(t *testing.T) map[string][]byte {
	t.Helper()

	reg := Registry()
	texts := make(map[string][]byte, len(reg.Names())+len(fixedStencils))
	for _, name := range reg.Names() {
		def, ok := reg.Default(name)
		if !ok {
			t.Fatalf("Registry().Default(%q) = _, false; want true for a name Registry().Names() returned", name)
		}
		texts[name] = def
	}
	for name, text := range fixedStencils {
		texts[name] = text
	}
	return texts
}

// stencilNamesOnDisk walks this package's own directory and returns the stencil name derived from
// each *.md file found in a family subfolder: the filename without its ".md" extension.
// An engine block's file, under the engine blocks directory's per-engine subfolder, gives the name stencilstore.EngineBlockName forms.
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
			if fileEntry.IsDir() && familyEntry.Name() == stencilstore.EngineBlocksDir {
				names = append(names, engineBlockNamesOnDisk(t, filepath.Join(familyDir, fileEntry.Name()), fileEntry.Name())...)
				continue
			}
			if fileEntry.IsDir() || !strings.HasSuffix(fileEntry.Name(), ".md") {
				continue
			}
			names = append(names, strings.TrimSuffix(fileEntry.Name(), ".md"))
		}
	}
	return names
}

// engineBlockNamesOnDisk returns the stencil name of each *.md file in engineDir, the engine blocks directory's subfolder for engine.
func engineBlockNamesOnDisk(t *testing.T, engineDir, engine string) []string {
	t.Helper()

	blockFiles, err := os.ReadDir(engineDir)
	if err != nil {
		t.Fatalf("os.ReadDir(%q) = _, %v; want nil error", engineDir, err)
	}
	var names []string
	for _, blockEntry := range blockFiles {
		if blockEntry.IsDir() || !strings.HasSuffix(blockEntry.Name(), ".md") {
			continue
		}
		names = append(names, stencilstore.EngineBlockName(engine, strings.TrimSuffix(blockEntry.Name(), ".md")))
	}
	return names
}

// TestRegistry_MatchesOnDiskTree verifies the registry and the on-disk *.md tree name exactly the same stencils in both directions: a .md present but unregistered fails, and a registered name with no .md fails.
//
//testtiming:keep pins that the registry and the on-disk stencil tree name the same stencils in both directions, which no other test compares
func TestRegistry_MatchesOnDiskTree(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

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

// TestRegistry_DefaultsAndRelPathAreConsistent verifies every registered entry's Default returns non-empty bytes, and that stencilstore.RelPath(name) resolves to the file's actual relative path -- pinning the family-from-first-token derivation against the on-disk layout.
// It also pins the generic Bouncer prompt templates by name, so a later accidental removal fails with a message naming the Bouncer rather than as a diff in the generic on-disk-tree comparison TestRegistry_MatchesOnDiskTree performs.
//
//testtiming:keep pins that RelPath resolves each registered name to its real file and that the Bouncer templates stay registered, which the scans over stencil bodies do not
func TestRegistry_DefaultsAndRelPathAreConsistent(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	packageDir, err := os.Getwd()
	if err != nil {
		t.Fatalf("os.Getwd() = _, %v; want nil error", err)
	}

	reg := Registry()

	names := reg.Names()
	nameSet := make(map[string]bool, len(names))
	for _, name := range names {
		nameSet[name] = true
	}
	for _, name := range []string{"bouncer-template-seed", "bouncer-template-judge", "bouncer-template-escalation", "bouncer-template-parent-notice"} {
		if !nameSet[name] {
			t.Errorf("Registry().Names() = %v; want it to contain %q", names, name)
		}
	}

	// The orch role is a fixed stencil rendered from embedded bytes; a registry entry would seed an editable board copy.
	if nameSet["orch-template-role"] {
		t.Errorf("Registry().Names() = %v; want it to lack %q", names, "orch-template-role")
	}
	if _, known := reg.Default("orch-template-role"); known {
		t.Errorf("Registry().Default(%q) = _, true; want false", "orch-template-role")
	}

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
