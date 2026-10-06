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

// TestConfigTemplate covers the generated template, resolved against an empty environment:
// it is valid YAML carrying both the branch_prefix and pathspec keys;
// branch_prefix resolves to an empty string;
// pathspec resolves to an empty set, regardless of environment: _lyx and .lyx are structural and
// injected in code by internal/fabricengine, and are never read from the pathspec key at all, so an
// empty default leaves pathspec free to name only genuinely optional per-repo directories the
// operator adds explicitly — the resolved value is whitespace-split (mirroring Config.Dirs, the
// consumer that actually splits it) rather than compared as one whole string, since a splitting bug
// there would otherwise be silent;
// and an empty pathspec yields an empty Config.Dirs() and pathspecNames (the routing set) and the
// dedupUnion formula junctionNames applies over cfg.Dirs() (the wired-name set) both degrade to
// their structural sets alone over it, with no config-supplied name in either union.
//
//testtiming:keep the config template being valid YAML with both keys, resolving to an empty branch prefix and an empty pathspec, and degrading to the structural sets alone; coverage of its blocks by other tests does not show an assertion of this
func TestConfigTemplate(t *testing.T) {
	t.Parallel()

	got := ConfigTemplate()

	var template map[string]any
	if err := yaml.Unmarshal([]byte(got), &template); err != nil {
		t.Fatalf("ConfigTemplate() is not valid YAML: %v", err)
	}
	for _, key := range []string{"branch_prefix", "pathspec"} {
		if _, ok := template[key]; !ok {
			t.Errorf("ConfigTemplate() missing expected key: %s", key)
		}
	}

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

	pathspec, ok := result["pathspec"]
	if !ok {
		t.Fatalf("resolved template missing key pathspec")
	}
	pathspecStr, ok := pathspec.(string)
	if !ok {
		t.Fatalf("resolved[pathspec] = %#v; want a string", pathspec)
	}
	if fields := strings.Fields(pathspecStr); len(fields) != 0 {
		t.Fatalf("resolved[pathspec] whitespace-split = %v; want empty", fields)
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
