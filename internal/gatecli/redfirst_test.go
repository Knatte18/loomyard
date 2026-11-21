// redfirst_test.go covers the pure parts of `lym gate red-first`.
// They are the read of the changed tests, the base overlay, the verdict and the missing-gate refusal.

package gatecli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"

	"github.com/Knatte18/loomyard/internal/gateslot"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
	"github.com/Knatte18/loomyard/internal/testreach"
)

// memoryTree is an in-memory tree of worktree-relative slash paths and their contents.
type memoryTree map[string][]byte

func (m memoryTree) file(rel string) ([]byte, bool, error) {
	data, found := m[rel]
	return data, found, nil
}

func (m memoryTree) goFiles(dir string) ([]string, error) {
	var names []string
	for rel := range m {
		if filepath.ToSlash(filepath.Dir(rel)) == dir && strings.HasSuffix(rel, ".go") {
			names = append(names, filepath.Base(rel))
		}
	}
	slices.Sort(names)
	return names, nil
}

// failingTree is a memoryTree whose reads fail with the error it holds, for each read it holds one for.
type failingTree struct {
	memoryTree
	goFilesError error
	fileError    error
}

func (f failingTree) file(rel string) ([]byte, bool, error) {
	if f.fileError != nil {
		return nil, false, f.fileError
	}
	return f.memoryTree.file(rel)
}

func (f failingTree) goFiles(dir string) ([]string, error) {
	if f.goFilesError != nil {
		return nil, f.goFilesError
	}
	return f.memoryTree.goFiles(dir)
}

func TestChangedTestsAt_ComparesTheTestFilesOfEachPackageHoldingAChangedOne(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	module := goModule{dir: "/w/sub", rel: "sub", path: "example.com/m"}
	base := memoryTree{
		"sub/a/a_test.go":        []byte("package a\n\nfunc TestKept(t *testing.T) {}\n\nfunc TestFixed(t *testing.T) { _ = 1 }\n"),
		"sub/a/a.go":             []byte("package a\n"),
		"sub/b/b_test.go":        []byte("package b\n\nfunc TestUntouched(t *testing.T) { _ = 1 }\n"),
		"sub/testdata/x_test.go": []byte("package x\n"),
		"sub/c/c_test.go":        []byte("package c\n\nfunc TestCovered(t *testing.T) { _ = 1 }\n"),
	}
	head := memoryTree{
		"sub/c/c_test.go":        []byte("package c\n\nfunc TestCovered(t *testing.T) { _ = 2 }\n"),
		"sub/a/a_test.go":        []byte("package a\n\nfunc TestKept(t *testing.T) {}\n\nfunc TestFixed(t *testing.T) { _ = 2 }\n"),
		"sub/a/other_test.go":    []byte("package a\n\nfunc TestFresh(t *testing.T) {}\n"),
		"sub/a/a.go":             []byte("package a\n\nfunc Fix() {}\n"),
		"sub/b/b_test.go":        []byte("package b\n\nfunc TestUntouched(t *testing.T) { _ = 2 }\n"),
		"sub/testdata/x_test.go": []byte("package x\n\nfunc TestData(t *testing.T) {}\n"),
	}
	// sub/b's test file differs but is not among the changed paths, so its package is not read.
	// sub/c holds a changed test file and no changed production file, so only its test path selects the package.
	changed := []string{"sub/a/a_test.go", "sub/a/other_test.go", "sub/a/a.go", "sub/testdata/x_test.go", "sub/c/c_test.go"}

	got, err := changedTestsAt(base, head, module, changed)
	if err != nil {
		t.Fatalf("changedTestsAt() error = %v; want nil", err)
	}
	want := []testreach.ChangedTest{
		{Package: "example.com/m/a", File: "a_test.go", Name: "TestFixed"},
		{Package: "example.com/m/a", File: "other_test.go", Name: "TestFresh", New: true},
		{Package: "example.com/m/c", File: "c_test.go", Name: "TestCovered"},
	}
	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("changedTestsAt() mismatch (-want +got):\n%s", diff)
	}
}

func TestBaseOverlay_ReplacesTheFilesOfTheModuleTheBasesBuildReads(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	module := goModule{dir: "/w/sub", rel: "sub", path: "example.com/m"}
	const baseText = "the base\n"
	// At the base, sub/a's production file embeds data.json and testdata/golden.json.
	// It embeds the directory embedded under all:, and the directory plain.
	// It embeds the globs q*.txt and inner/*.txt, and a backtick-quoted glob back*.txt.
	// It embeds the double-quoted name "my file.txt", and tabbed.txt after a tab.
	// Its test file embeds fixture.json.
	// The file of sub/a/inner embeds nothing.
	baseFiles := map[string]string{
		"sub/a/a.go": "package a\n\nimport _ \"embed\"\n\n" +
			"//go:embed data.json all:embedded \"q*.txt\" plain testdata/golden.json\n" +
			"//go:embed inner/*.txt\n" +
			"//go:embed `back*.txt` \"my file.txt\"\n" +
			"//go:embed\ttabbed.txt\n" +
			"var data string\n",
		"sub/a/a_test.go":  "package a\n\nimport _ \"embed\"\n\n//go:embed fixture.json\nvar fixture string\n",
		"sub/a/inner/i.go": "package inner\n",
	}
	tests := []struct {
		name string
		rel  string
		// baseHolds is whether the base holds the file, with the text baseText.
		baseHolds bool
		// wantReplaced is whether the overlay names the file.
		wantReplaced bool
		// wantCopy is the text of the copy the file maps to; "" when it maps to no copy.
		wantCopy string
	}{
		{name: "ProductionFileTheBaseHoldsMapsToACopyOfTheBaseText", rel: "sub/a/a.go", baseHolds: true, wantReplaced: true, wantCopy: baseText},
		{name: "ProductionFileTheBaseLacksMapsToNoCopy", rel: "sub/a/new.go", wantReplaced: true},
		{name: "TestFileIsLeftAsTheWorkingTreeHoldsIt", rel: "sub/a/a_test.go", baseHolds: true},
		{name: "FileOutsideTheModuleIsLeftAlone", rel: "other/x.go", baseHolds: true},
		{name: "NonGoFileAProductionFileEmbedsMapsToACopyOfTheBaseText", rel: "sub/a/data.json", baseHolds: true, wantReplaced: true, wantCopy: baseText},
		{name: "NonGoFileUnderAnEmbeddedDirectoryMapsToACopy", rel: "sub/a/embedded/deep/data.json", baseHolds: true, wantReplaced: true, wantCopy: baseText},
		{name: "NonGoFileAQuotedGlobEmbedsMapsToACopy", rel: "sub/a/q1.txt", baseHolds: true, wantReplaced: true, wantCopy: baseText},
		{name: "NonGoFileAPackageAboveEmbedsMapsToACopy", rel: "sub/a/inner/embedded.txt", baseHolds: true, wantReplaced: true, wantCopy: baseText},
		{name: "EmbeddedNonGoFileTheBaseLacksMapsToNoCopy", rel: "sub/a/embedded/new.json", wantReplaced: true},
		{name: "NonGoFileNoGoFileEmbedsIsLeftAlone", rel: "sub/a/notes.md", baseHolds: true},
		{name: "NonGoFileOnlyATestFileEmbedsIsLeftAlone", rel: "sub/a/fixture.json", baseHolds: true},
		{name: "NonGoFileWithNoGoFileAtOrAboveItIsLeftAlone", rel: "sub/notes/readme.md", baseHolds: true},
		{name: "FileUnderTestdataIsLeftAlone", rel: "sub/a/testdata/x.go", baseHolds: true},
		{name: "NonGoFileUnderTestdataNoGoFileEmbedsIsLeftAlone", rel: "sub/a/testdata/data.json", baseHolds: true},
		{name: "NonGoFileUnderTestdataAProductionFileEmbedsMapsToACopy", rel: "sub/a/testdata/golden.json", baseHolds: true, wantReplaced: true, wantCopy: baseText},
		{name: "NonGoFileUnderAPlainDirectoryMapsToACopy", rel: "sub/a/plain/ok.json", baseHolds: true, wantReplaced: true, wantCopy: baseText},
		{name: "DotNamedFileUnderAPlainDirectoryIsLeftAlone", rel: "sub/a/plain/.hidden.json", baseHolds: true},
		{name: "UnderscoreNamedFileUnderAPlainDirectoryIsLeftAlone", rel: "sub/a/plain/_x.json", baseHolds: true},
		{name: "FileUnderADotNamedSubdirectoryOfAPlainDirectoryIsLeftAlone", rel: "sub/a/plain/.d/x.json", baseHolds: true},
		{name: "UnderscoreNamedFileUnderAnAllPrefixedDirectoryMapsToACopy", rel: "sub/a/embedded/_x.json", baseHolds: true, wantReplaced: true, wantCopy: baseText},
		{name: "NonGoFileABacktickQuotedGlobEmbedsMapsToACopy", rel: "sub/a/back1.txt", baseHolds: true, wantReplaced: true, wantCopy: baseText},
		{name: "NonGoFileADoubleQuotedNameWithASpaceEmbedsMapsToACopy", rel: "sub/a/my file.txt", baseHolds: true, wantReplaced: true, wantCopy: baseText},
		{name: "NonGoFileATabSeparatedDirectiveEmbedsMapsToACopy", rel: "sub/a/tabbed.txt", baseHolds: true, wantReplaced: true, wantCopy: baseText},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			dir := t.TempDir()
			base := memoryTree{}
			for rel, text := range baseFiles {
				base[rel] = []byte(text)
			}
			if tt.baseHolds {
				base[tt.rel] = []byte(baseText)
			}
			got, err := baseOverlay(dir, "/w", module, []string{tt.rel}, base)
			if err != nil {
				t.Fatalf("baseOverlay() error = %v; want none", err)
			}
			if !tt.wantReplaced {
				if len(got) != 0 {
					t.Errorf("baseOverlay(%s) = %v; want no replacement", tt.rel, got)
				}
				return
			}
			target, named := got[filepath.Join("/w", filepath.FromSlash(tt.rel))]
			if !named || len(got) != 1 {
				t.Fatalf("baseOverlay(%s) = %v; want the one working-tree path of the file", tt.rel, got)
			}
			if tt.wantCopy == "" {
				if target != "" {
					t.Errorf("baseOverlay(%s) maps to %q; want %q, which deletes the file", tt.rel, target, "")
				}
				return
			}
			if filepath.Dir(target) != dir {
				t.Errorf("baseOverlay(%s) maps to %q; want a copy in %s", tt.rel, target, dir)
			}
			data, err := os.ReadFile(target)
			if err != nil {
				t.Fatal(err)
			}
			if string(data) != tt.wantCopy {
				t.Errorf("baseOverlay(%s) copy = %q; want %q", tt.rel, data, tt.wantCopy)
			}
		})
	}
}

func TestBaseOverlay_ReturnsTheErrorOfAFailedBaseReadWhileFindingEmbeddingFiles(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	module := goModule{dir: "/w/sub", rel: "sub", path: "example.com/m"}
	boom := errors.New("boom")
	baseFiles := memoryTree{"sub/a/a.go": []byte("package a\n\n//go:embed data.json\nvar data string\n")}
	tests := []struct {
		name     string
		base     failingTree
		wantText string
	}{
		{"ListingTheGoFilesFails", failingTree{memoryTree: baseFiles, goFilesError: boom}, "list the Go files of sub/a at the base"},
		{"ReadingAGoFileFails", failingTree{memoryTree: baseFiles, fileError: boom}, "read sub/a/a.go at the base"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, err := baseOverlay(t.TempDir(), "/w", module, []string{"sub/a/data.json"}, tt.base)
			if !errors.Is(err, boom) || !strings.Contains(err.Error(), tt.wantText) {
				t.Errorf("baseOverlay() error = %v; want it to wrap %v and contain %q", err, boom, tt.wantText)
			}
			if got != nil {
				t.Errorf("baseOverlay() = %v; want a nil replacement map", got)
			}
		})
	}
}

func TestWriteOverlay_HoldsTheReplacementsUnderTheReplaceKey(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	dir := t.TempDir()
	replace := map[string]string{"/w/a.go": "/tmp/0-a.go", "/w/new.go": ""}
	overlayPath, err := writeOverlay(dir, replace)
	if err != nil {
		t.Fatalf("writeOverlay() error = %v; want none", err)
	}
	if overlayPath != filepath.Join(dir, "overlay.json") {
		t.Errorf("writeOverlay() path = %q; want overlay.json in the directory", overlayPath)
	}
	data, err := os.ReadFile(overlayPath)
	if err != nil {
		t.Fatal(err)
	}
	var got map[string]map[string]string
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("overlay file %q is no JSON object of objects: %v", data, err)
	}
	if diff := cmp.Diff(map[string]map[string]string{"Replace": replace}, got); diff != "" {
		t.Errorf("writeOverlay() file mismatch (-want +got):\n%s", diff)
	}
}

func TestJudgeRedFirst_ReadsTheRunsExitCodeAndFailureReport(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	const pkg = "example.com/m/calc"
	clamp := testreach.ChangedTest{Package: pkg, File: "calc_test.go", Name: "TestClamp", New: true}
	scale := testreach.ChangedTest{Package: pkg, File: "calc_test.go", Name: "TestScale"}
	other := testreach.ChangedTest{Package: "example.com/m/other", File: "other_test.go", Name: "TestRun"}
	tests := []testreach.ChangedTest{clamp, scale, other}
	report := func(failureLines ...string) string {
		lines := append([]string{gateslot.FailureReportHeader, "tier0 (exit 1), log /l/tier0.log:"}, failureLines...)
		return "ok  \tsome/pkg\n" + strings.Join(lines, "\n") + "\ngate test: failed runs: tier0 (exit 1)\n"
	}

	rows := []struct {
		name     string
		exitCode int
		captured string
		want     redFirstVerdict
	}{
		{name: "ARunThatPassedIsGreen", exitCode: 0, captured: "ok\n", want: redFirstVerdict{outcome: redFirstGreen}},
		{
			name: "ATestTheReportNamesAsFailedIsRed", exitCode: 1,
			captured: report("  "+pkg+" TestClamp:", "    --- FAIL: TestClamp"),
			want:     redFirstVerdict{outcome: redFirstRed, red: []testreach.ChangedTest{clamp}},
		},
		{
			name: "EveryTestOfAPackageThatFailedOutsideAnyTestIsRed", exitCode: 2,
			captured: report("  " + pkg + gateslot.ReportOutsideAnyTestSuffix + ":"),
			want:     redFirstVerdict{outcome: redFirstRed, red: []testreach.ChangedTest{clamp, scale}},
		},
		{name: "ARunWithNoReportJudgedNone", exitCode: 1, captured: "timeout\n", want: redFirstVerdict{outcome: redFirstUnjudged}},
		{
			name: "AFailureOfAnotherTestJudgedNone", exitCode: 1,
			captured: report("  " + pkg + " TestUnrelated:"),
			want:     redFirstVerdict{outcome: redFirstUnjudged},
		},
		{
			name: "AFailureOfTheTestUnderARunThatIsNoTierJudgedNone", exitCode: 1,
			captured: report("vet tier1 (exit 1), log /l/vet.log:", "  "+pkg+" TestClamp:"),
			want:     redFirstVerdict{outcome: redFirstUnjudged},
		},
		{
			name: "ATestTheReportNamesBesideACutLineIsRed", exitCode: 1,
			captured: report("  "+pkg+" TestClamp:", gateslot.ReportCutPrefix+"12 more lines cut (tier0: 12)"),
			want:     redFirstVerdict{outcome: redFirstRed, red: []testreach.ChangedTest{clamp}},
		},
		{
			name: "ATestTheReportNamesBesideAnotherPackagesFailureOutsideAnyTestIsRed", exitCode: 1,
			captured: report("  example.com/m/unrelated"+gateslot.ReportOutsideAnyTestSuffix+":", "  "+pkg+" TestClamp:"),
			want:     redFirstVerdict{outcome: redFirstRed, red: []testreach.ChangedTest{clamp}},
		},
	}
	for _, tt := range rows {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := judgeRedFirst(tt.exitCode, tt.captured, tests)
			if diff := cmp.Diff(tt.want, got, cmp.AllowUnexported(redFirstVerdict{})); diff != "" {
				t.Errorf("judgeRedFirst(%d, ...) mismatch (-want +got):\n%s", tt.exitCode, diff)
			}
		})
	}
}

func TestNoSeamWayForward_NamesTheRecordTheParentAddsUnderTheGatesTitle(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	got := noSeamWayForward()
	for _, want := range []string{"the run's parent records", "`lym loom decision add`", `under a title opening "` + gateslot.NoTestSeamTitle + `"`} {
		if !strings.Contains(got, want) {
			t.Errorf("noSeamWayForward() = %q; want it to contain %q", got, want)
		}
	}
}

func TestTestsByPackage_SplitsTheTestsOneSliceToAPackageInFirstSeenOrder(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	a1 := testreach.ChangedTest{Package: "example.com/m/a", File: "a_test.go", Name: "TestRun"}
	b1 := testreach.ChangedTest{Package: "example.com/m/b", File: "b_test.go", Name: "TestRun"}
	a2 := testreach.ChangedTest{Package: "example.com/m/a", File: "a_test.go", Name: "TestOther"}
	got := testsByPackage([]testreach.ChangedTest{a1, b1, a2})
	want := [][]testreach.ChangedTest{{a1, a2}, {b1}}
	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("testsByPackage() mismatch (-want +got):\n%s", diff)
	}
}

func TestRunRedFirst_RefusesAMissingGateKeyWithAWayForward(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	var out bytes.Buffer
	code := runRedFirst(context.Background(), &out, redFirstRequest{})
	var answer struct {
		OK    bool   `json:"ok"`
		Error string `json:"error"`
	}
	if err := json.Unmarshal(out.Bytes(), &answer); err != nil {
		t.Fatalf("runRedFirst printed %q; want one JSON envelope: %v", out.String(), err)
	}
	_, wayForward, _ := strings.Cut(answer.Error, "way forward:")
	if code != 1 || answer.OK || !strings.Contains(answer.Error, "--gate names no gate key") || !strings.Contains(wayForward, "pass --gate") {
		t.Errorf("runRedFirst() = exit %d, ok %v, error %q; want exit 1 naming the missing key with a way forward to pass --gate", code, answer.OK, answer.Error)
	}
}
