// overlay_test.go covers `lyx loom validate-plan --overlay`: that it reports what the flag-less run reports over the overlaid plan, writes no byte under the plan directory, and refuses each unusable input, name and flag combination with its cause and way forward.
// The verb is driven directly against the leaf command, as validate_test.go does, so every case is tier 1.

package loomcli

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"io/fs"
	"maps"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/clihelp"
	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/planglyph"
	"github.com/Knatte18/loomyard/internal/planparser"
	"github.com/Knatte18/loomyard/internal/shedrecipe"
	"github.com/Knatte18/loomyard/internal/testkit/envelope"
	"github.com/Knatte18/loomyard/internal/testkit/plankit"
	"github.com/Knatte18/loomyard/internal/websterengine"
)

// snapshotPlanDir returns every file under dir by its relative name, or nil when dir does not exist.
func snapshotPlanDir(t *testing.T, dir string) map[string]string {
	t.Helper()
	files := make(map[string]string)
	err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		data, readErr := os.ReadFile(path)
		if readErr != nil {
			return readErr
		}
		relative, relErr := filepath.Rel(dir, path)
		files[relative] = string(data)
		return relErr
	})
	if err != nil && !os.IsNotExist(err) {
		t.Fatalf("snapshot %s: %v", dir, err)
	}
	return files
}

func TestValidatePlanCmd_Overlay(t *testing.T) {
	t.Parallel()

	const cardFile = "01-first-card.md"
	emptyObject := `{}`

	// proposed returns the on-disk card file with old replaced by new, as the JSON object --overlay reads.
	proposed := func(t *testing.T, anchorPath, old, replacement string) string {
		t.Helper()
		data, err := os.ReadFile(filepath.Join(planparser.PlanDir(anchorPath), cardFile))
		if err != nil {
			t.Fatalf("read card file: %v", err)
		}
		edited := strings.Replace(string(data), old, replacement, 1)
		if edited == string(data) {
			t.Fatalf("card file %q does not hold %q", data, old)
		}
		encoded, err := json.Marshal(map[string]string{cardFile: edited})
		if err != nil {
			t.Fatal(err)
		}
		return string(encoded)
	}
	devNull := func(t *testing.T) io.Reader {
		t.Helper()
		f, err := os.Open(os.DevNull)
		if err != nil {
			t.Fatalf("open %s: %v", os.DevNull, err)
		}
		t.Cleanup(func() { f.Close() })
		return f
	}

	cases := []struct {
		name string
		args []string
		// input is the stdin text, unless stdin supplies the reader.
		input string
		stdin func(t *testing.T) io.Reader
		// overlay builds the stdin text from the plan fixture, when set.
		overlay func(t *testing.T, anchorPath string) string
		// setup changes the plan fixture before the run; noPlan leaves no plan on disk at all.
		setup  func(t *testing.T, anchorPath string)
		noPlan bool
		// sameAsFlagless wants the flag-less run's exit code and output.
		sameAsFlagless bool
		wantExit       int
		// wantOut lists substrings the output must hold.
		wantOut []string
	}{
		{name: "an empty object reports what the flag-less run reports", input: emptyObject, sameAsFlagless: true},
		{
			name: "a card naming a missing member is glyph-not-found",
			overlay: func(t *testing.T, anchorPath string) string {
				return proposed(t, anchorPath, "sub#Foo", "sub#Missing")
			},
			wantExit: 1,
			wantOut:  []string{"glyph-not-found"},
		},
		{
			name: "a handle that canonicalizes onto an existing member is create-already-exists",
			overlay: func(t *testing.T, anchorPath string) string {
				encoded, err := json.Marshal(map[string]string{
					cardFile: "# Card 1 — first-card\n\n**Create:**\n- `plan:sub#Draft` -> `func Foo() {}`\n\n**Intent:** declare Foo.\n",
				})
				if err != nil {
					t.Fatal(err)
				}
				return string(encoded)
			},
			wantExit: 1,
			wantOut:  []string{"create-already-exists", "sub#Foo"},
		},
		{
			name: "an overlay of a done card is done-card-edited",
			setup: func(t *testing.T, anchorPath string) {
				data, err := os.ReadFile(filepath.Join(planparser.PlanDir(anchorPath), cardFile))
				if err != nil {
					t.Fatalf("read card file: %v", err)
				}
				sum := sha256.Sum256(data)
				st := &websterengine.State{Batches: map[int]*websterengine.BatchState{
					1: {Slug: "first-card", Cards: []string{"01-first-card"}, CardHashes: map[string]string{"01-first-card": hex.EncodeToString(sum[:])}, Terminal: true, Status: websterengine.DigestStatusDone},
				}}
				if err := websterengine.SaveState(websterengine.Dir(anchorPath), websterengine.ScratchDir(anchorPath), st); err != nil {
					t.Fatalf("SaveState: %v", err)
				}
			},
			overlay: func(t *testing.T, anchorPath string) string {
				return proposed(t, anchorPath, "**Intent:**", "**Intent:** edited after the batch began.")
			},
			wantExit: 1,
			wantOut:  []string{"done-card-edited"},
		},
		{name: "no plan on disk is the parse refusal", input: emptyObject, noPlan: true, wantExit: 1, wantOut: []string{"loom: parse plan at"}},
		{
			name:     "a terminal stdin is no input",
			stdin:    devNull,
			wantExit: 1,
			wantOut:  []string{"validate-plan --overlay: no input", "way forward: pipe one JSON object"},
		},
		{
			name:     "an empty stdin is no input",
			input:    " \n",
			wantExit: 1,
			wantOut:  []string{"validate-plan --overlay: no input", "way forward: pipe one JSON object"},
		},
		{
			name:     "an array is malformed",
			input:    `[]`,
			wantExit: 1,
			wantOut:  []string{"malformed overlay", "way forward: pipe one JSON object", "distinct names and string values"},
		},
		{
			name:     "invalid JSON is malformed",
			input:    `{"a":`,
			wantExit: 1,
			wantOut:  []string{"malformed overlay", "way forward: pipe one JSON object"},
		},
		{
			name:     "a second value after the object is malformed",
			input:    `{"a":"x"} {}`,
			wantExit: 1,
			wantOut:  []string{"malformed overlay", "second value", "way forward: pipe one JSON object"},
		},
		{
			name:     "a name that occurs twice is malformed",
			input:    `{"01-first-card.md":"x","01-first-card.md":"y"}`,
			wantExit: 1,
			wantOut:  []string{"malformed overlay", `"01-first-card.md" occurs twice`, "way forward: pipe one JSON object"},
		},
		{
			name:     "a value that is not a string is malformed",
			input:    `{"01-first-card.md":5}`,
			wantExit: 1,
			wantOut:  []string{"malformed overlay", `the value of "01-first-card.md" is not a string`, "way forward: pipe one JSON object"},
		},
		{
			name:     "a name that is not a plan file is refused",
			input:    `{"amendments.md":"x"}`,
			wantExit: 1,
			wantOut:  []string{"not a plan file", `"amendments.md"`, "way forward: name only `00-overview.md` or a card file the Card Index lists", "lyx loom insert-card"},
		},
		{
			name: "a Card Index card removed from disk is a missing card file",
			setup: func(t *testing.T, anchorPath string) {
				if err := os.Remove(filepath.Join(planparser.PlanDir(anchorPath), cardFile)); err != nil {
					t.Fatalf("remove card file: %v", err)
				}
			},
			input:    `{"01-first-card.md":"x"}`,
			wantExit: 1,
			wantOut:  []string{"card file is missing", `"01-first-card.md"`, "way forward: write the card file the Card Index names, then re-run"},
		},
		{
			name:     "--overlay with --rework is refused before stdin is read",
			args:     []string{"--rework"},
			stdin:    func(t *testing.T) io.Reader { return failingReader{} },
			wantExit: 1,
			wantOut:  []string{"--overlay and --rework are mutually exclusive", "way forward: run --overlay alone"},
		},
		{
			name:     "--overlay with --require-approved is refused before stdin is read",
			args:     []string{"--require-approved"},
			stdin:    func(t *testing.T) io.Reader { return failingReader{} },
			wantExit: 1,
			wantOut:  []string{"--overlay and --require-approved are mutually exclusive", "way forward: run --overlay alone"},
		},
		{
			name:     "--overlay with --lens-review is refused before stdin is read",
			args:     []string{"--lens-review"},
			stdin:    func(t *testing.T) io.Reader { return failingReader{} },
			wantExit: 1,
			wantOut:  []string{"--overlay and --lens-review are mutually exclusive", "way forward: run --overlay alone"},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			anchorPath := t.TempDir()
			worktreeRoot := plankit.Repo(t, map[string]string{"sub/a.go": "package sub\n\nfunc Foo() {}\n"})
			if !tc.noPlan {
				plankit.Write(t, planparser.PlanDir(anchorPath), plankit.Plan{
					Approved: true,
					Language: "go",
					Cards: []plankit.Card{{
						Number:        1,
						Slug:          "first-card",
						Summary:       "the first card",
						Groups:        []plankit.Group{{Label: "Edit", Targets: []string{"sub#Foo"}}},
						Intent:        "change Foo.",
						ImpactSummary: "none",
					}},
				})
			}
			if tc.setup != nil {
				tc.setup(t, anchorPath)
			}
			c := &loomCLI{env: shedrecipe.Env{
				PlanIndex:    planglyph.NewIndex(fabricengine.NewReferenceRule()),
				AnchorPath:   anchorPath,
				WorktreeRoot: worktreeRoot,
			}}
			before := snapshotPlanDir(t, planparser.PlanDir(anchorPath))

			var in io.Reader = strings.NewReader(tc.input)
			if tc.overlay != nil {
				in = strings.NewReader(tc.overlay(t, anchorPath))
			}
			if tc.stdin != nil {
				in = tc.stdin(t)
			}
			cmd := c.validatePlanCmd()
			cmd.SetIn(in)
			var out bytes.Buffer
			exitCode := clihelp.Execute(cmd, &out, append([]string{"--overlay"}, tc.args...))

			if exitCode != tc.wantExit {
				t.Errorf("exit code = %d; want %d (output: %q)", exitCode, tc.wantExit, out.String())
			}
			env := envelope.Decode(t, out.String())
			if env.OK != (tc.wantExit == 0) {
				t.Errorf("envelope ok = %v; want %v", env.OK, tc.wantExit == 0)
			}
			// The decoded error carries the quotes the raw JSON escapes.
			reported := env.Error + "\n" + out.String()
			for _, want := range tc.wantOut {
				if !strings.Contains(reported, want) {
					t.Errorf("output %q lacks %q", out.String(), want)
				}
			}
			if after := snapshotPlanDir(t, planparser.PlanDir(anchorPath)); !maps.Equal(after, before) {
				t.Errorf("plan directory changed: before %q, after %q", before, after)
			}

			if tc.sameAsFlagless {
				var flagless bytes.Buffer
				flaglessExit := clihelp.Execute(c.validatePlanCmd(), &flagless, nil)
				if exitCode != flaglessExit || out.String() != flagless.String() {
					t.Errorf("--overlay {} = exit %d, %q; want the flag-less run's exit %d, %q", exitCode, out.String(), flaglessExit, flagless.String())
				}
			}
		})
	}
}

// failingReader fails the test run's read: a verb that refuses a flag combination must do so before it reads stdin.
type failingReader struct{}

func (failingReader) Read([]byte) (int, error) { return 0, io.ErrNoProgress }
