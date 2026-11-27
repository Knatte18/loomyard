// validate_integration_test.go drives the validate-review verb's round checks against a real task worktree.
// The worktree comes from hubforge.
// The verb resolves it from the seam cwd and reads the run directory through its link.

package burlercli

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/clihelp"
	"github.com/Knatte18/loomyard/internal/hubforge"
	"github.com/Knatte18/loomyard/internal/loomengine"
	"github.com/Knatte18/loomyard/internal/lymcwd"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// layOutRoundOne writes a complete round 1 into runDir:
// a review with one finding, the engine's fixer-report and a snapshot record.
// A brokenRecord puts a directory where the snapshot record belongs, so a round context cannot be built from it.
func layOutRoundOne(t *testing.T, runDir string, brokenRecord bool) {
	t.Helper()
	if err := os.MkdirAll(runDir, 0o755); err != nil {
		t.Fatalf("MkdirAll(%s) = %v; want nil", runDir, err)
	}
	files := map[string]string{
		"round-1-review.md": "---\nverdict: BLOCKING\nfindings:\n" +
			"  - id: F1\n    severity: BLOCKING\n    class: design\n    location: a.go:1\n    summary: s1\n    fix: f1\n---\n",
		"round-1-fixer-report.md": "---\nspawned: false\ndispositions: []\n---\nnothing fixed\n",
	}
	if !brokenRecord {
		files["round-1-snapshot.json"] = `{"fix_scope":"overlay","target_paths":["a.go"],"decisions_routed":false}`
	} else if err := os.Mkdir(filepath.Join(runDir, "round-1-snapshot.json"), 0o755); err != nil {
		t.Fatalf("Mkdir(snapshot record) = %v; want nil", err)
	}
	for name, content := range files {
		if err := os.WriteFile(filepath.Join(runDir, name), []byte(content), 0o644); err != nil {
			t.Fatalf("WriteFile(%s) = %v; want nil", name, err)
		}
	}
}

// TestValidateReviewVerb_RoundChecks runs the verb on an incremental round 2 review.
// Its `prior` list misses round 1's finding.
// It exits 1 with the round-check refusal only for a review in a run directory under the resolved worktree's review runs.
// Every other placement gets the parse alone and exits 0.
// A run directory whose round context cannot be built exits 0 and names the builder's error in `round_checks`.
func TestValidateReviewVerb_RoundChecks(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()

	hub := hubforge.NewHub(t, ".")
	worktree := hubforge.AddPair(t, hub, "roundchecks").Path
	location, err := lymcwd.ResolveWithin(worktree)
	if err != nil {
		t.Fatalf("ResolveWithin(%s) = %v; want nil", worktree, err)
	}
	reviews := loomengine.LoomReviewsDir(location)

	tests := []struct {
		name string
		// runDir is where round 2's review is written; round 1 is laid out beside it unless bare.
		runDir       string
		bare         bool
		brokenRecord bool
		// cwd is the seam cwd the verb runs from; empty is the worktree.
		cwd string
		// wantRefusal is the substring of the refusal an exit 1 carries, empty for exit 0.
		wantRefusal string
		// wantNote is the substring of the `round_checks` field an exit 0 carries, empty for none.
		wantNote string
	}{
		{name: "InRunDirectoryRefused", runDir: filepath.Join(reviews, "run-refused"), wantRefusal: "has no `prior` entry for finding \"F1\""},
		{name: "OutsideAnyRunDirectory", runDir: filepath.Join(worktree, "scratch"), bare: true},
		{name: "SeamCwdOutsideAGitRepository", runDir: filepath.Join(reviews, "run-no-repo"), cwd: t.TempDir()},
		{name: "RunDirectoryOutsideTheReviewRunRoot", runDir: filepath.Join(worktree, "elsewhere")},
		{name: "BuilderErrorFallsBackToTheParse", runDir: filepath.Join(reviews, "run-broken"), brokenRecord: true, wantNote: "round context could not be built"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if !tt.bare {
				layOutRoundOne(t, tt.runDir, tt.brokenRecord)
			} else if err := os.MkdirAll(tt.runDir, 0o755); err != nil {
				t.Fatalf("MkdirAll(%s) = %v; want nil", tt.runDir, err)
			}
			review := filepath.Join(tt.runDir, "round-2-review.md")
			if err := os.WriteFile(review, []byte("---\nverdict: APPROVED\n---\nnothing found\n"), 0o644); err != nil {
				t.Fatalf("WriteFile(%s) = %v; want nil", review, err)
			}
			cwd := tt.cwd
			if cwd == "" {
				cwd = worktree
			}

			var out bytes.Buffer
			code := clihelp.ExecuteIn(Command(), cwd, &out, []string{"validate-review", review})

			var envelope map[string]any
			if err := json.Unmarshal(out.Bytes(), &envelope); err != nil {
				t.Fatalf("envelope %q is not JSON: %v", out.String(), err)
			}
			if tt.wantRefusal != "" {
				message, _ := envelope["error"].(string)
				if code != 1 || envelope["ok"] != false || !strings.Contains(message, tt.wantRefusal) || !strings.Contains(message, "way forward: rewrite the review file") {
					t.Errorf("validate-review = exit %d, envelope %v; want exit 1 with the round-check refusal containing %q", code, envelope, tt.wantRefusal)
				}
				return
			}
			if code != 0 || envelope["ok"] != true || envelope["review_file"] != review {
				t.Fatalf("validate-review = exit %d, envelope %v; want exit 0, ok true", code, envelope)
			}
			note, _ := envelope["round_checks"].(string)
			if (tt.wantNote == "") != (note == "") || !strings.Contains(note, tt.wantNote) {
				t.Errorf("round_checks = %q; want it to contain %q", note, tt.wantNote)
			}
		})
	}
}
