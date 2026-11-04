// dirbase_test.go — tier 0 unit tests for driftengine.RebaseDirsOntoRoot.
//
// Seeds directories under t.TempDir() and rewrites in-memory drift.yaml bytes; no hub, no git spawn.

package driftengine

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"

	"github.com/Knatte18/loomyard/internal/configengine"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// rebasedStep is the slice of a drift step the rebase tests decode.
type rebasedStep struct {
	Command    string            `yaml:"command"`
	Dir        string            `yaml:"dir"`
	TimeoutMin int               `yaml:"timeout_min"`
	Env        map[string]string `yaml:"env"`
}

// rebasedDoc is the slice of a drift.yaml the rebase tests decode.
type rebasedDoc struct {
	Enabled bool        `yaml:"enabled"`
	Capture rebasedStep `yaml:"capture"`
	Compare rebasedStep `yaml:"compare"`
}

// TestRebaseDirsOntoRoot pins that a relative capture or compare dir moves from the anchored directory to the worktree root:
// a dir naming a directory under the anchor is rewritten (even when the root holds one too).
// A dir under the root alone is kept.
// A dir naming neither is flagged with its way forward.
// An absolute, unset, null or undeclared dir is kept, and other keys survive.
// An unparseable document is an ErrInvalid error.
// A second run over the result changes nothing.
func TestRebaseDirsOntoRoot(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()
	const anchorName = "dotnet"
	tests := []struct {
		name string
		// sameDir anchors the worktree at its root.
		sameDir bool
		// dirs are directories created under the root before the run.
		dirs []string
		// yaml is the input; ABS stands for the absolute root path.
		yaml string
		// wantCapture and wantCompare are the dirs the result decodes to.
		wantCapture, wantCompare string
		// wantSame is whether the bytes come back unchanged.
		wantSame bool
		// wantRewrote and wantFlagged are the keys, in order, of the lines returned.
		wantRewrote, wantFlagged []string
		wantInvalid              bool
	}{
		{
			name: "SameDirChangesNothing", sameDir: true,
			yaml:        "capture:\n  dir: out\ncompare:\n  dir: .\n",
			wantCapture: "out", wantCompare: ".", wantSame: true,
		},
		{
			name: "DotBecomesAnchorDirectory",
			dirs: []string{anchorName},
			yaml: "capture:\n  dir: .\ncompare:\n  dir: .\n",
			// The dot names both the anchor and the root; the file predates the move, so the anchor wins.
			wantCapture: "dotnet", wantCompare: "dotnet",
			wantRewrote: []string{"capture.dir", "compare.dir"},
		},
		{
			name: "UnderAnchorIsRewritten",
			dirs: []string{"dotnet/out"},
			yaml: "capture:\n  dir: out\n",
			// compare is undeclared and stays absent.
			wantCapture: "dotnet/out", wantCompare: "",
			wantRewrote: []string{"capture.dir"},
		},
		{
			name:        "UnderAnchorWinsOverRoot",
			dirs:        []string{"dotnet/out", "out"},
			yaml:        "compare:\n  dir: out\n",
			wantCapture: "", wantCompare: "dotnet/out",
			wantRewrote: []string{"compare.dir"},
		},
		{
			name:        "UnderRootAloneIsKept",
			dirs:        []string{anchorName},
			yaml:        "capture:\n  dir: dotnet\n",
			wantCapture: "dotnet", wantSame: true,
		},
		{
			name:        "NamingNeitherIsFlagged",
			dirs:        []string{anchorName},
			yaml:        "capture:\n  dir: missing\ncompare:\n  dir: gone\n",
			wantCapture: "missing", wantCompare: "gone", wantSame: true,
			wantFlagged: []string{"capture.dir", "compare.dir"},
		},
		{
			name:        "RewrittenAndFlaggedTogether",
			dirs:        []string{"dotnet/out"},
			yaml:        "capture:\n  dir: out\ncompare:\n  dir: gone\n",
			wantCapture: "dotnet/out", wantCompare: "gone",
			wantRewrote: []string{"capture.dir"}, wantFlagged: []string{"compare.dir"},
		},
		{
			name:        "AbsoluteIsKept",
			dirs:        []string{anchorName},
			yaml:        "capture:\n  dir: ABS\n",
			wantCapture: "ABS", wantSame: true,
		},
		{
			name:        "UnsetAndUndeclaredAreKept",
			dirs:        []string{anchorName},
			yaml:        "capture:\n  command: run\ncompare:\n  dir: \"\"\n",
			wantCapture: "", wantCompare: "", wantSame: true,
		},
		{
			name:        "NullDirsAreKept",
			dirs:        []string{anchorName},
			yaml:        "capture:\n  dir: null\ncompare:\n  dir: ~\n",
			wantCapture: "", wantCompare: "", wantSame: true,
		},
		{
			name:        "UnparseableYamlIsInvalid",
			yaml:        "capture: {dir: [\n",
			wantInvalid: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			root := t.TempDir()
			for _, dir := range tt.dirs {
				if err := os.MkdirAll(filepath.Join(root, filepath.FromSlash(dir)), 0o755); err != nil {
					t.Fatal(err)
				}
			}
			anchor := filepath.Join(root, anchorName)
			if tt.sameDir {
				anchor = root
			}
			input := []byte(strings.ReplaceAll(tt.yaml, "ABS", filepath.ToSlash(root)))
			wantAbs := func(dir string) string { return strings.ReplaceAll(dir, "ABS", filepath.ToSlash(root)) }

			out, rewrote, flagged, err := RebaseDirsOntoRoot(input, root, anchor)
			if tt.wantInvalid {
				if !errors.Is(err, configengine.ErrInvalid) {
					t.Errorf("RebaseDirsOntoRoot(unparseable) error = %v; want one marked ErrInvalid", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("RebaseDirsOntoRoot() error = %v; want nil", err)
			}
			if tt.wantSame && string(out) != string(input) {
				t.Errorf("RebaseDirsOntoRoot() bytes = %q; want them unchanged %q", out, input)
			}
			if !tt.wantSame && string(out) == string(input) {
				t.Errorf("RebaseDirsOntoRoot() bytes = %q; want them rewritten", out)
			}
			var got rebasedDoc
			if err := yaml.Unmarshal(out, &got); err != nil {
				t.Fatalf("result does not parse: %v\n%s", err, out)
			}
			if got.Capture.Dir != wantAbs(tt.wantCapture) || got.Compare.Dir != wantAbs(tt.wantCompare) {
				t.Errorf("RebaseDirsOntoRoot() dirs = capture %q, compare %q; want capture %q, compare %q", got.Capture.Dir, got.Compare.Dir, wantAbs(tt.wantCapture), wantAbs(tt.wantCompare))
			}
			assertLineKeys(t, "rewrote", rewrote, tt.wantRewrote, "")
			assertLineKeys(t, "flagged", flagged, tt.wantFlagged, "way forward: lym config drift --set ")

			// A second run over the result with the same dirs changes nothing.
			again, rewroteAgain, _, err := RebaseDirsOntoRoot(out, root, anchor)
			if err != nil {
				t.Fatalf("second RebaseDirsOntoRoot() error = %v; want nil", err)
			}
			if len(rewroteAgain) != 0 || string(again) != string(out) {
				t.Errorf("second RebaseDirsOntoRoot() = %q with rewrote %v; want the bytes unchanged and no rewrite", again, rewroteAgain)
			}
		})
	}
}

// TestRebaseDirsOntoRoot_KeepsOtherKeys pins that a rewrite changes only the dir: the header comment, command, timeout_min, env and enabled survive and the compare step is untouched.
func TestRebaseDirsOntoRoot_KeepsOtherKeys(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()
	root := t.TempDir()
	anchor := filepath.Join(root, "dotnet")
	if err := os.MkdirAll(anchor, 0o755); err != nil {
		t.Fatal(err)
	}
	input := []byte("# drift header\nenabled: true\ncapture:\n  command: profile --out {baseline_dir}\n  dir: .\n  timeout_min: 30\n  env:\n    NAME: \"1\"\ncompare:\n  command: cmp {baseline_dir}\n  timeout_min: 5\n")
	out, _, _, err := RebaseDirsOntoRoot(input, root, anchor)
	if err != nil {
		t.Fatalf("RebaseDirsOntoRoot() error = %v; want nil", err)
	}
	if !strings.Contains(string(out), "# drift header") {
		t.Errorf("RebaseDirsOntoRoot() = %q; want the header comment kept", out)
	}
	var got rebasedDoc
	if err := yaml.Unmarshal(out, &got); err != nil {
		t.Fatalf("result does not parse: %v\n%s", err, out)
	}
	want := rebasedDoc{
		Enabled: true,
		Capture: rebasedStep{Command: "profile --out {baseline_dir}", Dir: "dotnet", TimeoutMin: 30, Env: map[string]string{"NAME": "1"}},
		Compare: rebasedStep{Command: "cmp {baseline_dir}", TimeoutMin: 5},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("RebaseDirsOntoRoot() decoded = %+v; want %+v", got, want)
	}
}

// assertLineKeys fails unless lines is one line per key in wantKeys, in order, each starting with its key and holding wantPart followed by that key's set form.
func assertLineKeys(t *testing.T, label string, lines, wantKeys []string, wantPart string) {
	t.Helper()
	if len(lines) != len(wantKeys) {
		t.Errorf("%s lines = %q; want %d, one per key %v", label, lines, len(wantKeys), wantKeys)
		return
	}
	for i, key := range wantKeys {
		if !strings.HasPrefix(lines[i], key+" ") {
			t.Errorf("%s line %d = %q; want it to start with %q", label, i, lines[i], key+" ")
		}
		if wantPart != "" && !strings.Contains(lines[i], wantPart+key+"=<dir>") {
			t.Errorf("%s line %d = %q; want it to hold %q", label, i, lines[i], wantPart+key+"=<dir>")
		}
	}
}
