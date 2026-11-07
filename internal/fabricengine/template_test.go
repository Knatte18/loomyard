// template_test.go — tests for the fabric ConfigTemplate generator.
//
// Covers: ConfigTemplate returns valid YAML with its three expected keys.
// It also resolves to the correct defaults when the environment is empty.

package fabricengine

import (
	"reflect"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
	"github.com/Knatte18/loomyard/internal/yamlengine"
	"gopkg.in/yaml.v3"
)

// TestConfigTemplate covers the generated template, resolved against an empty environment:
// it is valid YAML carrying the branch_prefix, pathspec and warp_exclude keys;
// branch_prefix resolves to an empty string;
// pathspec resolves to an empty set, whatever the environment.
// _lym and .lym are structural and injected in code, so the key never supplies them;
// an empty default leaves pathspec free to name only the optional per-repo directories the operator adds.
// The resolved value is whitespace-split and compared name by name, so a splitting bug cannot pass silently;
// warp_exclude resolves to exactly CLAUDE.local.md, AGENTS.override.md, .heddle and .claude/settings.local.json;
// and an empty pathspec leaves the routing set and the wired-name set at their structural sets alone.
//
//testtiming:keep the config template being valid YAML with its three keys, resolving to an empty branch prefix, an empty pathspec and the default warp exclude paths, and degrading to the structural sets alone; coverage of its blocks by other tests does not show an assertion of this
func TestConfigTemplate(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	got := ConfigTemplate()

	var template map[string]any
	if err := yaml.Unmarshal([]byte(got), &template); err != nil {
		t.Fatalf("ConfigTemplate() is not valid YAML: %v", err)
	}
	for _, key := range []string{"branch_prefix", "pathspec", "warp_exclude"} {
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

	if got, want := cfg.WarpExcludePaths(), []string{"CLAUDE.local.md", "AGENTS.override.md", ".heddle", ".claude/settings.local.json"}; !reflect.DeepEqual(got, want) {
		t.Errorf("cfg.WarpExcludePaths() = %v; want %v", got, want)
	}

	if len(cfg.Dirs()) != 0 {
		t.Errorf("cfg.Dirs() = %v; want empty", cfg.Dirs())
	}

	gotPathspecNames := pathspecNames(cfg)
	wantPathspecNames := structuralCommittedDirs
	if !reflect.DeepEqual(gotPathspecNames, wantPathspecNames) {
		t.Errorf("pathspecNames(cfg) = %v; want %v (structuralCommittedDirs alone)", gotPathspecNames, wantPathspecNames)
	}

	// junctionNames reads its config from disk, so the test applies its wired-name-set formula directly:
	// dedupUnion(structuralCommittedDirs, structuralNeverCommittedDirs, filterHubReserved(cfg.Dirs())):
	// over an empty pathspec it degrades to the two structural sets alone.
	gotJunctionNames := dedupUnion(structuralCommittedDirs, structuralNeverCommittedDirs, filterHubReserved(cfg.Dirs()))
	wantJunctionNames := dedupUnion(structuralCommittedDirs, structuralNeverCommittedDirs)
	if !reflect.DeepEqual(gotJunctionNames, wantJunctionNames) {
		t.Errorf("junctionNames formula over cfg.Dirs() = %v; want %v (both structural sets alone)", gotJunctionNames, wantJunctionNames)
	}
}
