// refusedverb_helper_test.go holds the seeds, fakes and way-forward assertions the tests of every tier in this package share.

package webstercli

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/gateslot"
	"github.com/Knatte18/loomyard/internal/hubgeom"
	"github.com/Knatte18/loomyard/internal/lymcwd"
	"github.com/Knatte18/loomyard/internal/planglyph"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
	"github.com/Knatte18/loomyard/internal/testkit/plankit"
	"github.com/Knatte18/loomyard/internal/websterengine"
)

// refusedVerbClause is the clause every way forward that names a fabric verb a sandboxed session cannot run carries.
const refusedVerbClause = "a session lym refuses the verb from reports status: FAILED and the orch runs it"

// containsString reports whether haystack contains needle.
func containsString(haystack []string, needle string) bool {
	for _, s := range haystack {
		if s == needle {
			return true
		}
	}
	return false
}

// failingInstructionFiles is a lister that fails as for a master engine the engine set does not hold.
func failingInstructionFiles() ([]string, error) {
	return nil, fmt.Errorf("webster: the master role's engine %q is not held by this lym: %w; way forward: set master in webster.yaml to an alias whose engine the set holds with lym config webster, then re-run the verb", "gone", shuttleengine.ErrEngineNotRegistered)
}

// seedHubGateConfig writes the hub-wide gate.yaml into hubPath's board directory: the shipped template with tier0 declared, so every gate lists it.
// hubgeom.WebsterGeometry's GateConfig reads it there.
func seedHubGateConfig(t *testing.T, hubPath string) {
	t.Helper()
	configDir := filepath.Join(fabricengine.BoardDir(hubPath), "_lym", "config")
	if err := os.MkdirAll(configDir, 0o755); err != nil {
		t.Fatalf("MkdirAll(%q) = %v; want nil", configDir, err)
	}
	contents := strings.Replace(gateslot.ConfigTemplate(), "tiers: {}", `tiers:
  tier0: {command: "go test -json {targets} {args}", all_targets: ./..., timeout_min: 60, counter: go-json}`, 1)
	if err := os.WriteFile(filepath.Join(configDir, "gate.yaml"), []byte(contents), 0o644); err != nil {
		t.Fatalf("WriteFile(gate.yaml) = %v; want nil", err)
	}
}

// hubGeometryWithIndex returns the hub geometry of layout with the real code index the CLI layer wires.
func hubGeometryWithIndex(layout *lymcwd.Location) websterengine.Geometry {
	geom := hubgeom.WebsterGeometry(layout)
	geom.Index = planglyph.NewIndex(fabricengine.NewReferenceRule())
	return geom
}

// testPlanFingerprint recomputes the plan-identity hash websterengine's own unexported fingerprint computes -- duplicated here since this test lives in webstercli, an external package, and that algorithm is deliberately not exported.
// It must stay in lock-step with websterengine's own implementation: a SHA-256 digest over every "*.md" file's sorted name and contents in planDir.
func testPlanFingerprint(t *testing.T, planDir string) string {
	t.Helper()
	entries, err := os.ReadDir(planDir)
	if err != nil {
		t.Fatalf("ReadDir(%q): %v", planDir, err)
	}
	var names []string
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".md") {
			continue
		}
		names = append(names, e.Name())
	}
	sort.Strings(names)

	h := sha256.New()
	for _, name := range names {
		data, err := os.ReadFile(filepath.Join(planDir, name))
		if err != nil {
			t.Fatalf("ReadFile(%q): %v", name, err)
		}
		h.Write([]byte(name))
		h.Write([]byte{0})
		h.Write(data)
		h.Write([]byte{0})
	}
	return hex.EncodeToString(h.Sum(nil))
}

// seedRunState writes a minimal state.json, fingerprint-matched to c's on-disk plan, standing in for the state the run verb would have already created before Master ever calls a bracket verb.
func seedRunState(t *testing.T, c *websterCLI) *websterengine.State {
	t.Helper()
	st := &websterengine.State{
		RunGUID:         "guid-1",
		PlanFingerprint: testPlanFingerprint(t, c.geom.PlanDir),
		MasterStrand:    "master-strand-1",
		MasterSessionID: "master-session-1",
		Batches:         map[int]*websterengine.BatchState{},
	}
	if err := websterengine.RestampPlanBaseline(st, c.geom.PlanDir, c.geom.WebsterDir); err != nil {
		t.Fatalf("RestampPlanBaseline() error = %v", err)
	}
	if err := websterengine.SaveState(c.geom.WebsterDir, c.geom.ScratchDir, st); err != nil {
		t.Fatalf("SaveState() error = %v", err)
	}
	return st
}

// writeBatchReport seeds reportsDir with a batch-report YAML file for batch 1 ("01-only") at its plan-format-pinned filename, status OK, using websterengine's own WriteReport so the on-disk shape always matches ParseReport's contract.
// headSHA must equal the worktree's actual current HEAD when the caller drives record-batch's terminal path;
// any non-empty placeholder is fine for await-batch (presence-only) and recover-batch (no such cross-check).
func writeBatchReport(t *testing.T, reportsDir, headSHA string) {
	t.Helper()
	if err := os.MkdirAll(reportsDir, 0o755); err != nil {
		t.Fatalf("mkdir reports dir: %v", err)
	}
	path := filepath.Join(reportsDir, websterengine.ReportFileName(1, "only"))
	report := &websterengine.Report{Status: websterengine.ReportStatusOK, HeadSHA: headSHA}
	if err := websterengine.WriteReport(path, report); err != nil {
		t.Fatalf("write batch report: %v", err)
	}
}

// failingFabricOpen is an openFabric that cannot reach the fabric repo, so fabricSync errors exactly where a failed fabric commit would.
func failingFabricOpen() (*fabricengine.Fabric, error) {
	return nil, fmt.Errorf("fabric commit failed (injected)")
}

// wantWayForward fails unless got carries the trailing way-forward clause and names substr in it.
func wantWayForward(t *testing.T, got, substr string) {
	t.Helper()
	i := strings.Index(got, "way forward:")
	if i < 0 {
		t.Fatalf("output has no way forward clause; got %q", got)
	}
	if !strings.Contains(got[i:], substr) {
		t.Errorf("way forward clause missing %q; got %q", substr, got[i:])
	}
}

// wantFabricSyncWayForward fails unless got carries a way forward that names the fabric commit and the refused-verb clause.
func wantFabricSyncWayForward(t *testing.T, got string) {
	t.Helper()
	wantWayForward(t, got, "lym fabric commit")
	wantWayForward(t, got, refusedVerbClause)
}

// refusalNotes returns the contents of every refusal note in dir, keyed by file name.
func refusalNotes(t *testing.T, dir string) map[string]string {
	t.Helper()
	paths, err := filepath.Glob(filepath.Join(dir, "webster-refusal-*.md"))
	if err != nil {
		t.Fatalf("glob %s: %v", dir, err)
	}
	notes := make(map[string]string, len(paths))
	for _, p := range paths {
		data, err := os.ReadFile(p)
		if err != nil {
			t.Fatalf("read %s: %v", p, err)
		}
		notes[filepath.Base(p)] = string(data)
	}
	return notes
}

// seedMissingIntentPlanDir writes a plan with a card missing the **Intent:** label.
func seedMissingIntentPlanDir(t *testing.T, dir string) {
	t.Helper()
	files := plankit.Render(onlyCreatePlan("", "internal/only/new.go"))
	card := files["01-only.md"]
	cut := bytes.Index(card, []byte("\n**Intent:**"))
	if cut < 0 {
		t.Fatalf("rendered card has no Intent label to remove: %q", card)
	}
	files["01-only.md"] = card[:cut]
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir plan dir: %v", err)
	}
	for name, data := range files {
		if err := os.WriteFile(filepath.Join(dir, name), data, 0o644); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
	}
}

// onlyCreatePlan returns a one-card plan whose card Creates target.
func onlyCreatePlan(language, target string) plankit.Plan {
	return plankit.Plan{
		Approved: true,
		Language: language,
		Framing:  "Framing.",
		Cards: []plankit.Card{{
			Number:  1,
			Slug:    "only",
			Summary: "placeholder card",
			Groups:  []plankit.Group{{Label: "Create", Targets: []string{target}}},
			Intent:  "placeholder card.",
		}},
	}
}

// seedValidPlanDir writes a valid plan with one card into dir.
func seedValidPlanDir(t *testing.T, dir string) {
	t.Helper()
	plankit.Write(t, dir, onlyCreatePlan("", "internal/only/new.go"))
}
