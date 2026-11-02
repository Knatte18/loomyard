// check_test.go tests Check, one row per check, and Validate over a temporary anchor.

package partsparser

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/fabricvocab"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

var validDecisionIDs = []string{"D1", "D2", "D3"}

// existingFile is the one path TestCheck's worktree holds.
const existingFile = "internal/x/existing.go"

func checkNames(findings []Finding) []string {
	names := make([]string, len(findings))
	for i, finding := range findings {
		names[i] = finding.Check
	}
	return names
}

func TestCheck(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	side := fabricvocab.SideTokens()[0]
	tests := []struct {
		name    string
		mutate  func(*Parts)
		ids     []string
		max     int
		want    []string
		wantIn  string
		wantFor string
	}{
		{name: "a valid file passes, a spot inside an area's package and a spot using the first needer's export included", ids: validDecisionIDs, max: 6},
		{name: "parts-none", mutate: func(p *Parts) { p.Parts, p.Spots = nil, nil }, ids: validDecisionIDs, max: 6, want: []string{"parts-none", "decision-unnamed", "decision-unnamed", "decision-unnamed"}, wantIn: "single part covering the whole task"},
		{name: "parts-over-max", ids: validDecisionIDs, max: 1, want: []string{"parts-over-max"}, wantIn: "merge parts"},
		{name: "part-name-invalid", mutate: func(p *Parts) { p.Parts = append(p.Parts, Part{Name: "1bad", Decisions: []string{"D1"}}) }, ids: validDecisionIDs, max: 6, want: []string{"part-name-invalid"}, wantIn: "starts with a letter", wantFor: "1bad"},
		{name: "part-name-repeated", mutate: func(p *Parts) { p.Parts = append(p.Parts, Part{Name: "core", Decisions: []string{"D1"}}) }, ids: validDecisionIDs, max: 6, want: []string{"part-name-repeated"}, wantIn: "rename one of the two", wantFor: "core"},
		{name: "part-name-archive", mutate: func(p *Parts) { p.Parts = append(p.Parts, Part{Name: "archive-old", Decisions: []string{"D1"}}) }, ids: validDecisionIDs, max: 6, want: []string{"part-name-archive"}, wantIn: "rename the part", wantFor: "archive-old"},
		{name: "part-dependency-order, a later part", mutate: func(p *Parts) { p.Parts[0].DependsOn = []string{"wiring"} }, ids: validDecisionIDs, max: 6, want: []string{"part-dependency-order"}, wantIn: "move the depended-on part above it", wantFor: "core"},
		{name: "part-dependency-order, an unknown part", mutate: func(p *Parts) { p.Parts[1].DependsOn = []string{"ghost"} }, ids: validDecisionIDs, max: 6, want: []string{"part-dependency-order"}, wantFor: "wiring"},
		{name: "spot-needer-unknown", mutate: func(p *Parts) { p.Spots[1].NeededBy = []string{"ghost"} }, ids: validDecisionIDs, max: 6, want: []string{"spot-needer-unknown"}, wantIn: "name a part of the file", wantFor: "cmd/main.go"},
		{
			name: "spot-export-order",
			mutate: func(p *Parts) {
				p.Parts[1].Exports = []string{"plan:internal/w#W"}
				p.Spots[0].Uses = append(p.Spots[0].Uses, "plan:internal/w#W")
			},
			ids: validDecisionIDs, max: 6, want: []string{"spot-export-order"}, wantIn: "reorder the parts or move the dependency", wantFor: "internal/reg/reg.go",
		},
		{name: "export-claimed-twice, by two parts", mutate: func(p *Parts) { p.Parts[1].Exports = []string{"internal/x/newfile.go"} }, ids: validDecisionIDs, max: 6, want: []string{"export-claimed-twice"}, wantIn: "keep the export on one of them"},
		{name: "export-claimed-twice, by a part and a spot", mutate: func(p *Parts) { p.Spots[0].Exports = append(p.Spots[0].Exports, "plan:internal/x#NewThing") }, ids: validDecisionIDs, max: 6, want: []string{"export-claimed-twice"}},
		{name: "spot-in-area", mutate: func(p *Parts) { p.Parts[1].Area = append(p.Parts[1].Area, "cmd/main.go") }, ids: validDecisionIDs, max: 6, want: []string{"spot-in-area"}, wantIn: "drop it from the area", wantFor: "wiring"},
		{name: "export-existing, a member glyph", mutate: func(p *Parts) { p.Parts[0].Exports = append(p.Parts[0].Exports, "internal/x#Existing") }, ids: validDecisionIDs, max: 6, want: []string{"export-existing"}, wantIn: "drop an existing member or file", wantFor: "core"},
		{name: "export-existing, a file the worktree holds", mutate: func(p *Parts) { p.Spots[0].Exports = append(p.Spots[0].Exports, existingFile) }, ids: validDecisionIDs, max: 6, want: []string{"export-existing"}, wantFor: "internal/reg/reg.go"},
		{name: "a handle export's name check", mutate: func(p *Parts) { p.Parts[0].Exports = append(p.Parts[0].Exports, "plan:internal/x#"+side+"Thing") }, ids: validDecisionIDs, max: 6, want: []string{"fabric-vocabulary-name"}, wantIn: `part "core": `, wantFor: "core"},
		{name: "decision-unnamed", ids: append([]string{"D4"}, validDecisionIDs...), max: 6, want: []string{"decision-unnamed"}, wantIn: "D4"},
		{name: "decision-unknown", mutate: func(p *Parts) { p.Parts[0].Decisions = []string{"D1", "D9"} }, ids: validDecisionIDs, max: 6, want: []string{"decision-unknown"}, wantIn: "drop or correct the id", wantFor: "core"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			parts := mustParse(t, validParts)
			if tt.mutate != nil {
				tt.mutate(parts)
			}
			got := Check(parts, tt.ids, tt.max, func(path string) bool { return path == existingFile })
			if names := checkNames(got); !slices.Equal(names, tt.want) {
				t.Fatalf("Check() findings = %v; want %v", names, tt.want)
			}
			for _, finding := range got {
				if !strings.Contains(finding.Detail, "way forward:") {
					t.Errorf("finding %+v: Detail does not end in a way forward", finding)
				}
			}
			if len(got) == 0 {
				return
			}
			if !strings.Contains(got[0].Detail, tt.wantIn) {
				t.Errorf("Check() first Detail = %q; want it to contain %q", got[0].Detail, tt.wantIn)
			}
			if tt.wantFor != "" && got[0].Part != tt.wantFor {
				t.Errorf("Check() first Part = %q; want %q", got[0].Part, tt.wantFor)
			}
		})
	}
}

func TestValidate(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	const record = "## Decisions\n\n### D1 — a\n\nx.\n\n### D2 — b\n\ny.\n\n### D3 — c\n\nz.\n"

	// seed writes the record and, when parts is non-empty, the parts file under a fresh anchor.
	seed := func(t *testing.T, recordText, partsText string) (anchor, recordPath string) {
		t.Helper()
		anchor = t.TempDir()
		recordPath = filepath.Join(anchor, "record.md")
		if recordText != "" {
			if err := os.WriteFile(recordPath, []byte(recordText), 0o644); err != nil {
				t.Fatalf("WriteFile(record) = %v; want nil", err)
			}
		}
		if partsText != "" {
			if err := os.MkdirAll(Dir(anchor), 0o755); err != nil {
				t.Fatalf("MkdirAll(%q) = %v; want nil", Dir(anchor), err)
			}
			if err := os.WriteFile(File(anchor), []byte(partsText), 0o644); err != nil {
				t.Fatalf("WriteFile(parts) = %v; want nil", err)
			}
		}
		return anchor, recordPath
	}

	tests := []struct {
		name       string
		recordText string
		partsText  string
		// worktreeFile is a repository-relative file the worktree holds, or empty for none.
		worktreeFile string
		want         []string
	}{
		{name: "a valid file and record", recordText: record, partsText: validParts},
		{name: "an absent parts file", recordText: record, want: []string{"parts-missing"}},
		{name: "a malformed parts file", recordText: record, partsText: "not a parts file\n", want: []string{"parts-malformed"}},
		{name: "a check finding passes through", recordText: record + "\n### D4 — d\n\nw.\n", partsText: validParts, want: []string{"decision-unnamed"}},
		{name: "an exported path the worktree already holds", recordText: record, partsText: validParts, worktreeFile: "internal/x/newfile.go", want: []string{"export-existing"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			anchor, recordPath := seed(t, tt.recordText, tt.partsText)
			worktree := t.TempDir()
			if tt.worktreeFile != "" {
				path := filepath.Join(worktree, filepath.FromSlash(tt.worktreeFile))
				if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
					t.Fatalf("MkdirAll() = %v; want nil", err)
				}
				if err := os.WriteFile(path, []byte("package x\n"), 0o644); err != nil {
					t.Fatalf("WriteFile(%q) = %v; want nil", path, err)
				}
			}
			got, err := Validate(anchor, worktree, recordPath, 6)
			if err != nil {
				t.Fatalf("Validate() = _, %v; want nil error", err)
			}
			if names := checkNames(got); !slices.Equal(names, tt.want) {
				t.Errorf("Validate() findings = %v; want %v", names, tt.want)
			}
		})
	}

	t.Run("an absent record is an error naming validate-discussion", func(t *testing.T) {
		t.Parallel()
		anchor, recordPath := seed(t, "", validParts)
		findings, err := Validate(anchor, t.TempDir(), recordPath, 6)
		if err == nil || !strings.Contains(err.Error(), "lym loom validate-discussion") {
			t.Errorf("Validate() = %v, %v; want an error naming lym loom validate-discussion", findings, err)
		}
	})
}
