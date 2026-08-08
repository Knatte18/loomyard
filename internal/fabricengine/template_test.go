// template_test.go — tests for the fabric ConfigTemplate generator.
//
// Covers: ConfigTemplate returns valid YAML with both expected keys and resolves to the correct
// defaults when the environment is empty.

package fabricengine

import (
	"reflect"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/yamlengine"
	"gopkg.in/yaml.v3"
)

// TestConfigTemplate_ValidYAML asserts that ConfigTemplate returns valid YAML that can be parsed
// without error.
func TestConfigTemplate_ValidYAML(t *testing.T) {
	got := ConfigTemplate()
	var result map[string]any
	if err := yaml.Unmarshal([]byte(got), &result); err != nil {
		t.Errorf("ConfigTemplate() is not valid YAML: %v", err)
	}
}

// TestConfigTemplate_HasBothKeys asserts that the template contains both the branch_prefix and
// pathspec keys.
func TestConfigTemplate_HasBothKeys(t *testing.T) {
	got := ConfigTemplate()
	var result map[string]any
	if err := yaml.Unmarshal([]byte(got), &result); err != nil {
		t.Fatalf("ConfigTemplate() is not valid YAML: %v", err)
	}

	if _, ok := result["branch_prefix"]; !ok {
		t.Errorf("ConfigTemplate() missing expected key: branch_prefix")
	}
	if _, ok := result["pathspec"]; !ok {
		t.Errorf("ConfigTemplate() missing expected key: pathspec")
	}
}

// TestConfigTemplate_ResolvesToEmptyBranchPrefix asserts that resolving the template against an
// empty environment yields an empty string for branch_prefix.
func TestConfigTemplate_ResolvesToEmptyBranchPrefix(t *testing.T) {
	got := ConfigTemplate()
	resolved, err := yamlengine.Resolve([]byte(got), nil)
	if err != nil {
		t.Fatalf("Resolve() failed: %v", err)
	}

	var result map[string]any
	if err := yaml.Unmarshal(resolved, &result); err != nil {
		t.Fatalf("resolved YAML is not valid: %v", err)
	}

	branchPrefix, ok := result["branch_prefix"]
	if !ok {
		t.Fatalf("resolved template missing key branch_prefix")
	}
	if branchPrefix != "" {
		t.Errorf("resolved[branch_prefix] = %q; want %q", branchPrefix, "")
	}
}

// TestConfigTemplate_PathspecResolvesToEmpty asserts that the template's pathspec default resolves to
// an empty set, regardless of environment: _lyx and .lyx are structural and injected in code by
// internal/fabricengine, and are never read from the pathspec key at all, so an empty default leaves
// pathspec free to name only genuinely optional per-repo directories the operator adds explicitly.
// The resolved value is whitespace-split (mirroring Config.Dirs, the consumer that actually splits
// it) rather than compared as one whole string, since the value is whitespace-split at the consumer
// -- a splitting bug there would otherwise be silent.
func TestConfigTemplate_PathspecResolvesToEmpty(t *testing.T) {
	got := ConfigTemplate()
	resolved, err := yamlengine.Resolve([]byte(got), nil)
	if err != nil {
		t.Fatalf("Resolve() failed: %v", err)
	}

	var result map[string]any
	if err := yaml.Unmarshal(resolved, &result); err != nil {
		t.Fatalf("resolved YAML is not valid: %v", err)
	}

	pathspec, ok := result["pathspec"]
	if !ok {
		t.Fatalf("resolved template missing key pathspec")
	}
	pathspecStr, ok := pathspec.(string)
	if !ok {
		t.Fatalf("resolved[pathspec] = %#v; want a string", pathspec)
	}
	got2 := strings.Fields(pathspecStr)
	want := []string{}
	if len(got2) != len(want) {
		t.Fatalf("resolved[pathspec] whitespace-split = %v; want %v", got2, want)
	}
}

// TestConfigTemplate_EmptyPathspecDegradesToStructuralSetsAlone asserts that an empty pathspec
// yields an empty Config.Dirs() and that pathspecNames (the routing set) and the dedupUnion formula
// junctionNames applies over cfg.Dirs() (the wired-name set) both degrade to their structural sets
// alone over it, with no config-supplied name in either union.
func TestConfigTemplate_EmptyPathspecDegradesToStructuralSetsAlone(t *testing.T) {
	got := ConfigTemplate()
	resolved, err := yamlengine.Resolve([]byte(got), nil)
	if err != nil {
		t.Fatalf("Resolve() failed: %v", err)
	}

	var cfg Config
	if err := yaml.Unmarshal(resolved, &cfg); err != nil {
		t.Fatalf("resolved YAML does not unmarshal into Config: %v", err)
	}

	if len(cfg.Dirs()) != 0 {
		t.Errorf("cfg.Dirs() = %v; want empty", cfg.Dirs())
	}

	gotPathspecNames := pathspecNames(cfg)
	wantPathspecNames := structuralCommittedDirs
	if !reflect.DeepEqual(gotPathspecNames, wantPathspecNames) {
		t.Errorf("pathspecNames(cfg) = %v; want %v (structuralCommittedDirs alone)", gotPathspecNames, wantPathspecNames)
	}

	// junctionNames itself reads its config from disk, but its wired-name-set formula is
	// dedupUnion(structuralCommittedDirs, structuralNeverCommittedDirs, filterHubReserved(cfg.Dirs())):
	// applying that same formula here proves it degrades to the two structural sets alone.
	gotJunctionNames := dedupUnion(structuralCommittedDirs, structuralNeverCommittedDirs, filterHubReserved(cfg.Dirs()))
	wantJunctionNames := dedupUnion(structuralCommittedDirs, structuralNeverCommittedDirs)
	if !reflect.DeepEqual(gotJunctionNames, wantJunctionNames) {
		t.Errorf("junctionNames formula over cfg.Dirs() = %v; want %v (both structural sets alone)", gotJunctionNames, wantJunctionNames)
	}
}
