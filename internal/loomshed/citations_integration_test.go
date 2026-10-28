//go:build tier1

// citations_integration_test.go covers the loom discussion check's citation test against a real git repository, since a cited path counts only when git tracks its root entry.

package loomshed

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/gitkit"
	"github.com/Knatte18/loomyard/internal/planglyph"
	"github.com/Knatte18/loomyard/internal/planindex"
)

// unavailableIndex is a planindex.Index whose glyph resolution reports a quarry outage.
type unavailableIndex struct{ planindex.Index }

func (unavailableIndex) ResolveGlyphs(string, []string) ([]planindex.GlyphAnswer, error) {
	return nil, fmt.Errorf("%w: test outage", planindex.ErrQuarryUnavailable)
}

// citationFixtureFiles are the worktree files the citation rows cite, committed to the fixture repository.
var citationFixtureFiles = map[string]string{
	"internal/real/real.go":               "package real\n\ntype Thing struct{ Field int }\n\nfunc Do() {}\n",
	"internal/real/twice_a.go":            "package real\n\nfunc Twice() {}\n",
	"internal/real/twice_b.go":            "package real\n\nfunc Twice() {}\n",
	"internal/dup/a/x/x.go":               "package x\n\nfunc Foo() {}\n",
	"internal/dup/b/x/x.go":               "package x\n\nfunc Foo() {}\n",
	"internal/real/testdata/real/real.go": "package real\n\nfunc Other() {}\n",
	"docs/overview.md":                    "# Overview\n",
	"_lym/keep.md":                        "kept\n",
	"go.mod":                              "module example.com/m\n\ngo 1.22\n",
}

// newCitationRepo returns a git repository holding files, all committed.
func newCitationRepo(t *testing.T, files map[string]string) string {
	t.Helper()
	root := t.TempDir()
	gitkit.Git(t, root, "init", "-b", "main")
	gitkit.Git(t, root, "config", "user.email", "test@test.com")
	gitkit.Git(t, root, "config", "user.name", "Test")
	writeCitationFiles(t, root, files)
	gitkit.Git(t, root, "add", "-A")
	gitkit.Git(t, root, "commit", "-m", "citation fixture")
	return root
}

func writeCitationFiles(t *testing.T, root string, files map[string]string) {
	t.Helper()
	for rel, content := range files {
		full := filepath.Join(root, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatalf("MkdirAll(%q) = %v", full, err)
		}
		if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
			t.Fatalf("WriteFile(%q) = %v", full, err)
		}
	}
}

// citingRecord returns validDecisionRecord with body as its Decisions text and appendix after its last section.
func citingRecord(body, appendix string) string {
	return strings.Replace(validDecisionRecord, "Decisions text.", body, 1) + appendix
}

func TestValidateLoomDiscussion_Citations(t *testing.T) {
	t.Parallel()

	repo := newCitationRepo(t, citationFixtureFiles)
	index := planglyph.NewIndex(fabricengine.NewReferenceRule())

	const (
		pathWayForward   = "way forward: correct the path, or mark one the task creates or removes with ` (new)` or ` (removed)` right after the closing backtick"
		symbolWayForward = "way forward: correct it as `lym quarry glyphs <dir>` lists the members, or mark it ` (new)` or ` (removed)` right after the closing backtick"
	)
	const patternBullet = "\n## PATTERN changes\n\n- `PATTERN-x` — adds: a rule. — added after Discussion (parent, 2026-03-04)\n"

	tests := []struct {
		name     string
		body     string
		appendix string
		// token is the cited text the first finding must name with its line.
		token      string
		wantChecks []string
		wantDetail []string
	}{
		{name: "MissingPath", body: "See `internal/missing/file.go`.", token: "internal/missing/file.go", wantChecks: []string{"citation-path-missing"}, wantDetail: []string{pathWayForward}},
		{name: "MissingPathMarkedNew", body: "See `internal/missing/file.go` (new)."},
		{name: "MissingPathMarkedRemoved", body: "See `internal/missing/file.go` (removed)."},
		{name: "ExternalLymAndPlaceholderPathsPass", body: "`github.com/Knatte18/quarry/glyph`, `_lym/plan/missing.md` and `internal/<name>/x.go`."},
		{name: "MissingPathInFencedBlockPasses", body: "```\n`internal/missing/file.go`\n```"},
		{name: "ExistingDirectoryPasses", body: "`internal/real/`."},
		{name: "FileWithTrailingSlashFails", body: "`internal/real/real.go/`.", token: "internal/real/real.go/", wantChecks: []string{"citation-path-missing"}},
		{name: "MissingGlyphMember", body: "`internal/real#Nope`.", token: "internal/real#Nope", wantChecks: []string{"citation-symbol-missing"}, wantDetail: []string{symbolWayForward}},
		{name: "MissingQualifiedMember", body: "`real.Nope`.", token: "real.Nope", wantChecks: []string{"citation-symbol-missing"}, wantDetail: []string{symbolWayForward}},
		{name: "ExistingSymbolsPass", body: "`internal/real#Thing`, `real.Thing`, `real.Do()`, `real.Do(x)` and `real.Thing.Field`."},
		{
			name: "QualifiedSymbolInTwoPackagesIsAmbiguous", body: "`x.Foo` beside `real.Thing`.", token: "x.Foo",
			wantChecks: []string{"citation-symbol-ambiguous"},
			wantDetail: []string{"internal/dup/a/x", "internal/dup/b/x", "way forward: spell the symbol as a glyph"},
		},
		{
			name: "GlyphDeclaredTwiceIsAmbiguous", body: "`internal/real#Twice`.", token: "internal/real#Twice",
			wantChecks: []string{"citation-symbol-ambiguous"},
			wantDetail: []string{"internal/real/twice_a.go", "internal/real/twice_b.go", "way forward: cite the declaring file meant by its path in place of the symbol"},
		},
		{name: "NonCitationsPassUntested", body: "`## Decisions`, `C#`, `docs/overview.md#Modules`, `state.json` and `os.ReadFile`."},
		{name: "AddedAfterDiscussionEntryIsNotTested", body: "Text.\n\n### Added after Discussion (orch, 2026-10-28): t\n\nSee `internal/missing/file.go`."},
		{name: "SuffixedPatternBulletIsNotTested", body: "Text.", appendix: strings.Replace(patternBullet, "a rule.", "a rule on `internal/missing/file.go`.", 1)},
		{name: "CarryOverBlockIsNotTested", body: "<!-- lym:carry-over loom -->\n`internal/missing/file.go`\n<!-- /lym:carry-over loom -->"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			record := citingRecord(tt.body, tt.appendix)
			decisionRecordPath, supportLogPath := writeDiscussionFixture(t, t.TempDir(), record, "support log")

			findings, err := ValidateLoomDiscussion(decisionRecordPath, supportLogPath, filepath.Join(t.TempDir(), "relevant.md"), repo, index)
			if err != nil {
				t.Fatalf("ValidateLoomDiscussion() error = %v; want nil", err)
			}
			if len(findings) != len(tt.wantChecks) {
				t.Fatalf("ValidateLoomDiscussion() = %v; want checks %v", findings, tt.wantChecks)
			}
			wantLine := strings.Count(record[:strings.Index(record, tt.token+"`")+1], "\n") + 1
			for i, finding := range findings {
				if finding.Check != tt.wantChecks[i] || finding.Path != decisionRecordPath {
					t.Errorf("finding %d = %+v; want Check %q and the record's path", i, finding, tt.wantChecks[i])
				}
				if !strings.Contains(finding.Detail, tt.token) || !strings.Contains(finding.Detail, fmt.Sprintf("line %d", wantLine)) {
					t.Errorf("finding %d Detail = %q; want it to name %q and line %d", i, finding.Detail, tt.token, wantLine)
				}
				for _, part := range tt.wantDetail {
					if !strings.Contains(finding.Detail, part) {
						t.Errorf("finding %d Detail = %q; want it to contain %q", i, finding.Detail, part)
					}
				}
			}
		})
	}

	t.Run("EmptyIndexTestsNoPath", func(t *testing.T) {
		t.Parallel()
		empty := t.TempDir()
		gitkit.Git(t, empty, "init", "-b", "main")
		decisionRecordPath, supportLogPath := writeDiscussionFixture(t, t.TempDir(), citingRecord("`internal/missing/file.go`", ""), "support log")
		findings, err := ValidateLoomDiscussion(decisionRecordPath, supportLogPath, filepath.Join(t.TempDir(), "relevant.md"), empty, index)
		if err != nil || len(findings) != 0 {
			t.Errorf("ValidateLoomDiscussion() = %v, %v; want no findings and no error", findings, err)
		}
	})

	t.Run("StagedRootEntryIsTracked", func(t *testing.T) {
		t.Parallel()
		staged := newCitationRepo(t, citationFixtureFiles)
		if err := os.MkdirAll(filepath.Join(staged, "extra"), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(staged, "extra", "present.md"), []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
		gitkit.Git(t, staged, "add", "extra/present.md")
		decisionRecordPath, supportLogPath := writeDiscussionFixture(t, t.TempDir(), citingRecord("`extra/missing.md`", ""), "support log")
		findings, err := ValidateLoomDiscussion(decisionRecordPath, supportLogPath, filepath.Join(t.TempDir(), "relevant.md"), staged, index)
		if err != nil || len(findings) != 1 || findings[0].Check != "citation-path-missing" {
			t.Errorf("ValidateLoomDiscussion() = %v, %v; want one citation-path-missing finding", findings, err)
		}
	})

	t.Run("QuarryOutageIsAnErrorNotAFinding", func(t *testing.T) {
		t.Parallel()
		decisionRecordPath, supportLogPath := writeDiscussionFixture(t, t.TempDir(), citingRecord("`internal/real#Thing`", ""), "support log")
		findings, err := ValidateLoomDiscussion(decisionRecordPath, supportLogPath, filepath.Join(t.TempDir(), "relevant.md"), repo, unavailableIndex{})
		if !errors.Is(err, planindex.ErrQuarryUnavailable) || findings != nil {
			t.Errorf("ValidateLoomDiscussion() = %v, %v; want no findings and an error wrapping ErrQuarryUnavailable", findings, err)
		}
	})

	t.Run("DarnRecordCitationsAreNotTested", func(t *testing.T) {
		t.Parallel()
		decisionRecordPath, supportLogPath := writeDiscussionFixture(t, t.TempDir(), citingRecord("`internal/missing/file.go`", ""), "support log")
		findings, err := ValidateDarnDiscussion(decisionRecordPath, supportLogPath, func() ([][]byte, error) { return nil, nil })
		if err != nil || len(findings) != 0 {
			t.Errorf("ValidateDarnDiscussion() = %v, %v; want no findings and no error", findings, err)
		}
	})
}
