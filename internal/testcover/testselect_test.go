package testcover_test

import (
	"fmt"
	"maps"
	"os"
	"path"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/testcover"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// selectCommit is the commit every fixture map was built from.
const selectCommit = "c0ffee"

// targetCommit is the commit that holds the working tree's files, for a selection whose target is a commit.
const targetCommit = "7a9e7"

// removedFile marks a fixture file the working tree no longer holds.
const removedFile = "\x00removed"

// linkedSource is the linked package a's main file at the map's commit.
const linkedSource = `package a

import (
	"strconv"
	"strings"
)

const Limit = 3

const (
	KindA = iota
	KindB
)

const (
	SizeA = 1
	SizeB
)

var registered []string

type Box struct{ N int }

func (b Box) Size() int {
	return b.N
}

func F(s string) string {
	return strings.ToUpper(s)
}

func G() string {
	return strconv.Itoa(Limit)
}

func init() {
	registered = append(registered, "one")
}

func init() {
	registered = append(registered, "two")
}
`

// ownSource is the selected package b's main file at the map's commit.
const ownSource = "package b\n\nfunc Own() int {\n\treturn 1\n}\n\nfunc Other() int {\n\treturn 2\n}\n"

// ownTestSource is b's test file of Test functions and a helper at the map's commit.
const ownTestSource = `package b

import "testing"

func helper() int {
	return 1
}

func TestOwn(t *testing.T) {
	if Own() != helper() {
		t.Fatal("Own")
	}
}

func TestOther(t *testing.T) {
	_ = Other()
}
`

// committedFiles are the module's files at the map's commit: the selected package b, the linked packages a and c.
var committedFiles = map[string]string{
	"go.mod":                "module m\n\ngo 1.26\n",
	"a/a.go":                linkedSource,
	"a/gone.go":             "package a\n\nimport \"strings\"\n\nfunc H() int {\n\treturn len(strings.TrimSpace(\" \"))\n}\n",
	"a/assets.go":           "package a\n\nimport \"embed\"\n\n//go:embed static\nvar static embed.FS\n",
	"a/static/page.txt":     "hello\n",
	"a/notes.md":            "notes\n",
	"a/a_test.go":           "package a\n",
	"c/c.go":                "package c\n",
	"b/b.go":                ownSource,
	"b/b_test.go":           "package b\n\nimport \"embed\"\n\n//go:embed note.txt\nvar note embed.FS\n",
	"b/funcs_test.go":       ownTestSource,
	"b/note.txt":            "note\n",
	"b/testdata/in.txt":     "input\n",
	"b/testdata/fixture.go": "package fixture\n",
}

// selectEnvironment is the current environment stamp a selection runs under, bar its module file hashes.
var selectEnvironment = testcover.EnvironmentStamp{
	GoVersion: "go1.26.8", GOOS: "linux", GOARCH: "amd64", TiersDigest: "tiers", HubEnvDigest: "hub-env",
}

// applyFileEdits sets each edited file's text in files, and removes a file whose text is removedFile.
func applyFileEdits(files, edits map[string]string) {
	for file, text := range edits {
		if text == removedFile {
			delete(files, file)
		} else {
			files[file] = text
		}
	}
}

// fakeCommits serves the files of each commit in trees, by revision, and fails to read any other revision.
type fakeCommits struct {
	trees map[string]map[string]string
}

func (f fakeCommits) CodeFileAt(rev, rel string) ([]byte, bool, error) {
	files, known := f.trees[rev]
	if !known {
		return nil, false, fmt.Errorf("object %s not found", rev)
	}
	text, found := files[rel]
	return []byte(text), found, nil
}

// selectFixtureMap returns b's map at selectCommit, recording committed as the module's files.
// It holds one measured test per function of a that b covers and one per function of b.
// It also holds a test the spawn scan marks and a test whose entry records an outcome.
func selectFixtureMap(committed map[string]string) testcover.Map {
	hashesUnder := func(dir string, goFilesDirectly bool) map[string]string {
		hashes := map[string]string{}
		for file, text := range committed {
			rel, under := strings.CutPrefix(file, dir+"/")
			if under && goFilesDirectly == (path.Dir(rel) == "." && path.Ext(rel) == ".go") {
				hashes[rel] = testcover.ContentHash([]byte(text))
			}
		}
		return hashes
	}
	allUnder := func(dir string) map[string]string {
		hashes := hashesUnder(dir, true)
		maps.Copy(hashes, hashesUnder(dir, false))
		return hashes
	}
	environment := selectEnvironment
	environment.GoModHash = testcover.ContentHash([]byte(committed["go.mod"]))
	built := time.Date(2026, 11, 11, 9, 0, 0, 0, time.UTC)
	return testcover.Map{
		Commit:     selectCommit,
		BuiltAt:    built,
		Files:      hashesUnder("b", true),
		OtherFiles: hashesUnder("b", false),
		Linked:     map[string]map[string]string{"m/a": allUnder("a"), "m/c": allUnder("c")},
		Blocks: []testcover.BlockKey{
			{File: "b.go", Func: "Own", Hash: "own"},
			{File: "b.go", Func: "Other", Hash: "other"},
		},
		Funcs: []testcover.FuncKey{
			{Package: "m/a", File: "a.go", Func: "Box.Size"},
			{Package: "m/a", File: "a.go", Func: "F"},
			{Package: "m/a", File: "a.go", Func: "G"},
			{Package: "m/a", File: "a.go", Func: "init"},
			{Package: "m/a", File: "gone.go", Func: "H"},
		},
		Tests: []testcover.MapTest{
			{Name: "TestSize", Funcs: []int{0}},
			{Name: "TestF", Funcs: []int{1}},
			{Name: "TestG", Funcs: []int{2}},
			{Name: "TestInit", Funcs: []int{3}},
			{Name: "TestH", Funcs: []int{4}},
			{Name: "TestOwn", Blocks: []int{0}},
			{Name: "TestOther", Blocks: []int{1}},
			{Name: "TestSpawn", Funcs: []int{1}, Spawn: testcover.SpawnVerdict{OutOfProcess: true}},
			{Name: "TestFailed", Funcs: []int{1}, Outcome: testcover.OutcomeFailed},
		},
		Format: &testcover.MapFormat{Version: testcover.MapFormatVersion, Commit: selectCommit, BuiltAt: built, Environment: environment},
	}
}

// fixtureRecorded are the fixture map's tests that record a passing measurement, in its order.
var fixtureRecorded = []string{"TestSize", "TestF", "TestG", "TestInit", "TestH", "TestOwn", "TestOther"}

// TestSelectorSelect pins which of b's tests a change to b or to its linked package a selects.
// It also pins the changes the map cannot speak for, so b runs whole.
// Every case gives the same verdict with the working tree as target and with a commit holding the same files.
func TestSelectorSelect(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	inertText := func(rel string) bool { return path.Ext(rel) == ".txt" || path.Ext(rel) == ".md" }
	embedAssets := func(directive string) map[string]string {
		return map[string]string{"a/assets.go": "package a\n\nimport \"embed\"\n\n" + directive + "\nvar static embed.FS\n"}
	}
	ownTestEdit := func(old, replacement string) map[string]string {
		return map[string]string{"b/funcs_test.go": strings.Replace(ownTestSource, old, replacement, 1)}
	}
	pageChanged := map[string]string{"a/static/page.txt": "changed\n"}
	tests := []struct {
		name string
		// committed replaces or adds files in the module as the map's commit, the map and the working tree all hold it.
		committed map[string]string
		// commitEdits replace files as the map's commit stores them alone, so the commit differs from the map's record.
		commitEdits map[string]string
		// linkedEdits are old and new text pairs replaced in a/a.go.
		linkedEdits []string
		// files replace or add working files by worktree-relative path; removedFile removes one.
		files map[string]string
		// editMap changes b's map before it is written.
		editMap func(*testcover.Map)
		// mapText, when set, is written as b's map file instead of the map.
		mapText   string
		noMap     bool
		commitRev string
		inert     func(string) bool
		want      []string
		wantMiss  testcover.MissReason
		// wantDetail, when set, is the exact detail of the miss.
		wantDetail string
		// unrecorded names the tests of fixtureRecorded that editMap leaves with no passing measurement.
		unrecorded []string
	}{
		{name: "NoChange"},
		{name: "EditInsideFunctionBody", linkedEdits: []string{"strings.ToUpper(s)", "strings.ToLower(s)"},
			want: []string{"TestF"}},
		{name: "ChangedParameterTypes", linkedEdits: []string{"func F(s string) string {", "func F(s string, _ ...int) string {"},
			want: []string{"TestF"}},
		{name: "DeletedFile", files: map[string]string{"a/gone.go": removedFile}, want: []string{"TestH"}},
		{name: "EditToOneOfTwoInits", linkedEdits: []string{`"two"`, `"three"`}, want: []string{"TestInit"}},
		{name: "CommentAndFormatting", linkedEdits: []string{
			"func G() string {", "// G renders the limit.\nfunc G() string {",
			"return b.N", "return b.N // the count",
			"type Box struct{ N int }", "type Box struct {\n\tN int\n}",
		}},
		{name: "ReorderedImports", linkedEdits: []string{"\t\"strconv\"\n\t\"strings\"\n", "\t\"strings\"\n\t\"strconv\"\n"}},
		{name: "AddedConst", linkedEdits: []string{"const Limit = 3\n", "const Limit = 3\n\nconst Extra = 1\n"}},
		{name: "AddedVarWithoutInitializer", linkedEdits: []string{"var registered []string\n", "var registered []string\n\nvar spare int\n"}},
		{name: "AddedTypeWithoutMethods", linkedEdits: []string{"type Box struct{ N int }\n", "type Box struct{ N int }\n\ntype Extra struct{}\n"}},
		{name: "AddedFunctionWithStandardLibraryImport",
			files: map[string]string{"a/show.go": "package a\n\nimport \"fmt\"\n\nfunc Show() string {\n\treturn fmt.Sprint(1)\n}\n"}},
		{name: "AddedImportOfLinkedPackage", files: map[string]string{"a/link.go": "package a\n\nimport _ \"m/c\"\n"}},
		{name: "LinkedPackageTestFile", files: map[string]string{"a/a_test.go": "package a\n\nvar touched int\n"}},
		{name: "NestedPackageFile", files: map[string]string{"a/sub/s.go": "package sub\n"}},
		{name: "InertFileNoGoEmbedNames", files: map[string]string{"a/notes.md": "more notes\n"}, inert: inertText},
		{name: "InertFileUnderUnrelatedDirectory", files: map[string]string{"a/other/x.txt": "x\n"}, inert: inertText},
		{name: "EditInsideMethodBody", linkedEdits: []string{"return b.N\n", "return b.N + 1\n"}, want: []string{"TestSize"}},
		{name: "RemovedInit", linkedEdits: []string{"\nfunc init() {\n\tregistered = append(registered, \"two\")\n}\n", ""},
			want: []string{"TestInit"}},
		{name: "OwnFunctionBodyEdited", files: map[string]string{"b/b.go": strings.Replace(ownSource, "return 1", "return 3", 1)},
			want: []string{"TestOwn"}},
		{name: "OwnGoFileAdded", files: map[string]string{"b/extra.go": "package b\n"}},
		{name: "OwnGoFileRemoved", files: map[string]string{"b/b.go": removedFile}, want: []string{"TestOther", "TestOwn"}},
		{name: "OwnTestFunctionEdited", files: ownTestEdit("_ = Other()", "_ = Other() + 1"), want: []string{"TestOther"}},
		{name: "OwnTestFunctionAdded", files: ownTestEdit("func TestOther", "func TestNew(t *testing.T) {}\n\nfunc TestOther"),
			want: []string{"TestNew"}},
		{name: "OwnTestFunctionRemoved", files: ownTestEdit("func TestOther(t *testing.T) {\n\t_ = Other()\n}\n", "")},
		{name: "OwnTestFileWithOnlyTestFunctionsAdded",
			files: map[string]string{"b/more_test.go": "package b\n\nimport \"testing\"\n\nfunc TestMore(t *testing.T) {}\n"},
			want:  []string{"TestMore"}},
		{name: "OwnTestFileImportingTestingRemoved",
			committed: map[string]string{"b/more_test.go": "package b\n\nimport \"testing\"\n\nfunc TestMore(t *testing.T) {}\n"},
			files:     map[string]string{"b/more_test.go": removedFile}},
		{name: "OwnTestFileImportingLinkedPackageRemoved",
			committed: map[string]string{"b/reg_test.go": "package b\n\nimport _ \"m/c\"\n\nfunc TestReg() {}\n"},
			files:     map[string]string{"b/reg_test.go": removedFile}, wantMiss: testcover.MissTestFile},
		{name: "ExternalTestFileImportingOwnPackageAdded",
			files: map[string]string{"b/ext_test.go": "package b_test\n\nimport (\n\t\"testing\"\n\n\t\"m/b\"\n)\n\nfunc TestExt(t *testing.T) { _ = b.Own() }\n"},
			want:  []string{"TestExt"}},
		{name: "OwnTestStandardLibraryImportAdded",
			files: ownTestEdit(`import "testing"`, "import (\n\t_ \"fmt\"\n\t\"testing\"\n)")},
		{name: "BuildFailureBesideTestEntries", editMap: func(m *testcover.Map) { m.BuildFailure = "build failed" }},
		{name: "SpawnMarkedTestDeclaredInNewFile", files: map[string]string{"b/spawn_test.go": "package b\n\nfunc TestSpawn(t *T) {}\n"},
			want: []string{"TestSpawn"}},
		{name: "OutcomeEntryIsNoRecordedTest", linkedEdits: []string{"strings.ToUpper(s)", "strings.ToLower(s)"},
			editMap: func(m *testcover.Map) { m.Tests[1].Outcome = testcover.OutcomeTimedOut }, unrecorded: []string{"TestF"}},

		{name: "NoMap", noMap: true, wantMiss: testcover.MissNoMap},
		{name: "MapUnreadable", mapText: "{not json", wantMiss: testcover.MissMapUnreadable},
		{name: "BuildFailureWithNoTestEntries",
			editMap:  func(m *testcover.Map) { m.BuildFailure, m.Tests = "build failed", nil },
			wantMiss: testcover.MissBuildFailure, wantDetail: "build failed"},
		{name: "NoMarker", editMap: func(m *testcover.Map) { m.Format = nil }, wantMiss: testcover.MissUnmarked},
		{name: "MarkerDiffersFromMap", editMap: func(m *testcover.Map) { m.Format.Commit = "older" }, wantMiss: testcover.MissUnmarked},
		{name: "TestNamesFunctionOutsideTable", editMap: func(m *testcover.Map) { m.Tests[0].Funcs = []int{9} },
			wantMiss: testcover.MissMapUnreadable},
		{name: "TestNamesBlockOutsideTable", editMap: func(m *testcover.Map) { m.Tests[0].Blocks = []int{9} },
			wantMiss: testcover.MissMapUnreadable},
		{name: "ToolchainVersionDiffers", editMap: func(m *testcover.Map) { m.Format.Environment.GoVersion = "go1.25.0" },
			wantMiss: testcover.MissEnvironment, wantDetail: "the map's stamp differs in go_version"},
		{name: "GOOSDiffers", editMap: func(m *testcover.Map) { m.Format.Environment.GOOS = "plan9" },
			wantMiss: testcover.MissEnvironment, wantDetail: "the map's stamp differs in goos"},
		{name: "GOARCHDiffers", editMap: func(m *testcover.Map) { m.Format.Environment.GOARCH = "riscv64" },
			wantMiss: testcover.MissEnvironment, wantDetail: "the map's stamp differs in goarch"},
		{name: "TiersDigestDiffers", editMap: func(m *testcover.Map) { m.Format.Environment.TiersDigest = "other-tiers" },
			wantMiss: testcover.MissEnvironment, wantDetail: "the map's stamp differs in tiers_digest"},
		{name: "HubEnvDigestDiffers", editMap: func(m *testcover.Map) { m.Format.Environment.HubEnvDigest = "other-env" },
			wantMiss: testcover.MissEnvironment, wantDetail: "the map's stamp differs in hub_env_digest"},
		{name: "GoModChanged", files: map[string]string{"go.mod": "module m\n\ngo 1.27\n"},
			wantMiss: testcover.MissEnvironment, wantDetail: "the map's stamp differs in go_mod_hash"},
		{name: "GoSumAdded", files: map[string]string{"go.sum": "example.com/x v1.0.0 h1:x=\n"},
			wantMiss: testcover.MissEnvironment, wantDetail: "the map's stamp differs in go_sum_hash"},
		{name: "CommitUnreadable", commitRev: "other", wantMiss: testcover.MissCommitUnreadable},
		{name: "OwnTestHelperEdited", files: ownTestEdit("return 1", "return 2"), wantMiss: testcover.MissTestFile},
		{name: "OwnTestMainAdded", files: ownTestEdit("func TestOwn", "func TestMain(m *testing.M) {}\n\nfunc TestOwn"),
			wantMiss: testcover.MissTestFile},
		{name: "OwnTestImportOutsideLinkedSetAdded",
			files:    ownTestEdit(`import "testing"`, "import (\n\t_ \"example.com/x\"\n\t\"testing\"\n)"),
			wantMiss: testcover.MissTestFile},
		{name: "OwnTestPackageClauseChanged", files: ownTestEdit("package b\n", "package b_test\n"), wantMiss: testcover.MissTestFile},
		{name: "OwnTestPackageLevelVarAdded", files: ownTestEdit("func helper", "var count int\n\nfunc helper"),
			wantMiss: testcover.MissTestFile},
		{name: "OwnTestMethodNamedLikeATestAdded",
			files:    map[string]string{"b/more_test.go": "package b\n\nfunc (s TestSuite) Run() {}\n"},
			wantMiss: testcover.MissTestFile},
		{name: "OwnTestFileDoesNotParse", files: ownTestEdit("func TestOther(t *testing.T) {", "func TestOther(t {"),
			wantMiss: testcover.MissParse},
		{name: "OwnTestFileDirectiveRemoved", files: map[string]string{"b/b_test.go": "package b\n\nvar touched int\n"},
			wantMiss: testcover.MissDirective},
		{name: "AddedInitInNewFile", files: map[string]string{"a/extra.go": "package a\n\nfunc init() {}\n"},
			wantMiss: testcover.MissInit},
		{name: "OwnInitAdded", files: map[string]string{"b/extra.go": "package b\n\nfunc init() {}\n"}, wantMiss: testcover.MissInit},
		{name: "OwnConstChanged", committed: map[string]string{"b/limits.go": "package b\n\nconst Max = 1\n"},
			files: map[string]string{"b/limits.go": "package b\n\nconst Max = 2\n"}, wantMiss: testcover.MissNonBody},
		{name: "EmbeddedFileInert", files: map[string]string{"a/static/page.txt": "changed\n"}, inert: inertText,
			wantMiss: testcover.MissNonGoFile},
		{name: "NonGoFileNotInert", files: map[string]string{"a/notes.md": "more notes\n"}, wantMiss: testcover.MissNonGoFile},
		{name: "UntrackedNonGoFile", files: map[string]string{"a/new.json": "{}\n"}, wantMiss: testcover.MissNonGoFile},
		{name: "ChangedConst", linkedEdits: []string{"const Limit = 3", "const Limit = 4"}, wantMiss: testcover.MissNonBody},
		{name: "ChangedStructField", linkedEdits: []string{"type Box struct{ N int }", "type Box struct{ N, M int }"},
			wantMiss: testcover.MissNonBody},
		{name: "AddedVarWithInitializer", linkedEdits: []string{"var registered []string\n", "var registered []string\n\nvar spare = 1\n"},
			wantMiss: testcover.MissNonBody},
		{name: "ConstAddedToIotaGroup", linkedEdits: []string{"\tKindB\n", "\tKindB\n\tKindC\n"}, wantMiss: testcover.MissNonBody},
		{name: "RemovedImport", linkedEdits: []string{"\t\"strconv\"\n", "", "strconv.Itoa(Limit)", `"3"`},
			wantMiss: testcover.MissNonBody},
		{name: "AddedImportOutsideLinkedSet", files: map[string]string{"a/link.go": "package a\n\nimport _ \"example.com/x\"\n"},
			wantMiss: testcover.MissNonBody},
		{name: "AddedImportOfUnlinkedModulePackage", files: map[string]string{"a/link.go": "package a\n\nimport _ \"m/d\"\n"},
			wantMiss: testcover.MissNonBody},
		{name: "ReceiverValueToPointer", linkedEdits: []string{"func (b Box) Size", "func (b *Box) Size"}, wantMiss: testcover.MissMethodSet},
		{name: "AddedMethod", linkedEdits: []string{"func (b Box) Size", "func (b Box) Twice() int {\n\treturn 2 * b.N\n}\n\nfunc (b Box) Size"},
			wantMiss: testcover.MissMethodSet},
		{name: "AddedFunctionNamedMax", linkedEdits: []string{"func G() string {", "func max() int {\n\treturn 0\n}\n\nfunc G() string {"},
			wantMiss: testcover.MissPredeclared},
		{name: "AddedDirective", linkedEdits: []string{"func G() string {", "//go:noinline\nfunc G() string {"},
			wantMiss: testcover.MissDirective},
		{name: "ChangedDirective", files: map[string]string{"a/assets.go": strings.Replace(committedFiles["a/assets.go"], "embed static", "embed static/*.txt", 1)},
			wantMiss: testcover.MissDirective},
		{name: "FileDoesNotParse", linkedEdits: []string{"func G() string {", "func G( string {"}, wantMiss: testcover.MissParse},
		{name: "MarkerBuildTimeDiffersFromMap",
			editMap:  func(m *testcover.Map) { m.Format.BuiltAt = m.Format.BuiltAt.Add(time.Second) },
			wantMiss: testcover.MissUnmarked},
		{name: "MarkerVersionDiffers", editMap: func(m *testcover.Map) { m.Format.Version++ }, wantMiss: testcover.MissUnmarked},
		{name: "ProbeFileAbsentAtCommit", commitEdits: map[string]string{"b/b.go": removedFile},
			wantMiss: testcover.MissCommitUnreadable},
		{name: "FileDoesNotParseAtCommit", commitEdits: map[string]string{"a/gone.go": "package a\n\nfunc H( {\n"},
			files:    map[string]string{"a/gone.go": "package a\n\nfunc H() int {\n\treturn 2\n}\n"},
			wantMiss: testcover.MissParse},
		{name: "DeletedFileBlankImportsModulePackage", committed: map[string]string{"a/reg.go": "package a\n\nimport _ \"m/c\"\n"},
			files: map[string]string{"a/reg.go": removedFile}, wantMiss: testcover.MissNonBody},
		{name: "RemovedConst", linkedEdits: []string{"const Limit = 3\n", ""}, wantMiss: testcover.MissNonBody},
		{name: "RemovedVar", linkedEdits: []string{"var registered []string\n", ""}, wantMiss: testcover.MissNonBody},
		{name: "RemovedType", linkedEdits: []string{"type Box struct{ N int }\n", ""}, wantMiss: testcover.MissNonBody},
		{name: "ChangedPackageClause", linkedEdits: []string{"package a\n", "package z\n"}, wantMiss: testcover.MissNonBody},
		{name: "ConstAddedToImplicitValueGroup", linkedEdits: []string{"\tSizeB\n", "\tSizeB\n\tSizeC\n"}, wantMiss: testcover.MissNonBody},
		{name: "RemovedDirective",
			files:    map[string]string{"a/assets.go": strings.Replace(committedFiles["a/assets.go"], "//go:embed static\n", "", 1)},
			wantMiss: testcover.MissDirective},
		{name: "AddedVarNamedLen", linkedEdits: []string{"var registered []string\n", "var registered []string\n\nvar len int\n"},
			wantMiss: testcover.MissPredeclared},
		{name: "AddedConstNamedCap", linkedEdits: []string{"const Limit = 3\n", "const Limit = 3\n\nconst cap = 1\n"},
			wantMiss: testcover.MissPredeclared},
		{name: "AddedTypeNamedError", linkedEdits: []string{"type Box struct{ N int }\n", "type Box struct{ N int }\n\ntype error struct{}\n"},
			wantMiss: testcover.MissPredeclared},
		{name: "AddedMethodNamedMax",
			linkedEdits: []string{"func (b Box) Size", "func (b Box) max() int {\n\treturn 0\n}\n\nfunc (b Box) Size"},
			wantMiss:    testcover.MissMethodSet},
		{name: "RemovedMethod", linkedEdits: []string{"func (b Box) Size() int {\n\treturn b.N\n}\n\n", ""}, wantMiss: testcover.MissMethodSet},
		{name: "ChangedMethodResultType", linkedEdits: []string{"func (b Box) Size() int {", "func (b Box) Size() int64 {"},
			wantMiss: testcover.MissMethodSet},
		{name: "AddedImportOfDotlessNonStandardPath", files: map[string]string{"a/link.go": "package a\n\nimport _ \"foo/bar\"\n"},
			wantMiss: testcover.MissNonBody},
		{name: "OwnNonGoFileChanged", files: map[string]string{"b/note.txt": "changed\n"}, inert: nil, wantMiss: testcover.MissNonGoFile},
		{name: "OwnTestdataFileChanged", files: map[string]string{"b/testdata/in.txt": "changed\n"}, wantMiss: testcover.MissNonGoFile},
		{name: "OwnTestdataGoFileChanged", files: map[string]string{"b/testdata/fixture.go": "package fixture\n\nvar x int\n"},
			inert: inertText, wantMiss: testcover.MissNonGoFile},
		{name: "OwnTestFileEmbedsInertFile", files: map[string]string{"b/note.txt": "changed\n"}, inert: inertText,
			wantMiss: testcover.MissNonGoFile},
		{name: "RecordedNonGoFileRemoved", files: map[string]string{"a/notes.md": removedFile}, wantMiss: testcover.MissNonGoFile},
		{name: "EmbedQuotedPattern", committed: embedAssets(`//go:embed "static/page.txt"`), files: pageChanged, inert: inertText,
			wantMiss: testcover.MissNonGoFile},
		{name: "EmbedBackquotedPattern", committed: embedAssets("//go:embed `static/page.txt`"), files: pageChanged, inert: inertText,
			wantMiss: testcover.MissNonGoFile},
		{name: "EmbedAllPrefix", committed: embedAssets("//go:embed all:static"), files: pageChanged, inert: inertText,
			wantMiss: testcover.MissNonGoFile},
		{name: "EmbedTwoPatterns", committed: embedAssets("//go:embed other.txt static/page.txt"), files: pageChanged, inert: inertText,
			wantMiss: testcover.MissNonGoFile},
		{name: "EmbedGlobPattern", committed: embedAssets("//go:embed static/*.txt"), files: pageChanged, inert: inertText,
			wantMiss: testcover.MissNonGoFile},
		{name: "EmbedMalformedDirective", committed: embedAssets(`//go:embed "static`), files: pageChanged, inert: inertText,
			wantMiss: testcover.MissParse},
		{name: "EmbedPackageDoesNotParse", committed: map[string]string{"a/assets.go": "package a\n\nfunc (\n"}, files: pageChanged,
			inert: inertText, wantMiss: testcover.MissParse},
		{name: "LinkedTestFileEmbedIsNoSource",
			committed: map[string]string{
				"a/assets.go": "package a\n",
				"a/a_test.go": "package a\n\nimport \"embed\"\n\n//go:embed static/page.txt\nvar page embed.FS\n",
			},
			files: pageChanged, inert: inertText},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			committed := maps.Clone(committedFiles)
			applyFileEdits(committed, tt.committed)
			atCommit := maps.Clone(committed)
			applyFileEdits(atCommit, tt.commitEdits)
			working := maps.Clone(committed)
			for i := 0; i < len(tt.linkedEdits); i += 2 {
				old, replacement := tt.linkedEdits[i], tt.linkedEdits[i+1]
				if !strings.Contains(working["a/a.go"], old) {
					t.Fatalf("a/a.go holds no %q to replace", old)
				}
				working["a/a.go"] = strings.Replace(working["a/a.go"], old, replacement, 1)
			}
			applyFileEdits(working, tt.files)
			root := t.TempDir()
			var workingFiles []string
			for file, text := range working {
				workingFiles = append(workingFiles, file)
				full := filepath.Join(root, filepath.FromSlash(file))
				if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(full, []byte(text), 0o644); err != nil {
					t.Fatal(err)
				}
			}
			slices.Sort(workingFiles)

			mapsDir := t.TempDir()
			switch {
			case tt.mapText != "":
				mapPath := testcover.MapPath(mapsDir, "b")
				if err := os.MkdirAll(filepath.Dir(mapPath), 0o755); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(mapPath, []byte(tt.mapText), 0o644); err != nil {
					t.Fatal(err)
				}
			case !tt.noMap:
				m := selectFixtureMap(committed)
				if tt.editMap != nil {
					tt.editMap(&m)
				}
				if err := testcover.WriteMap(mapsDir, "b", m); err != nil {
					t.Fatal(err)
				}
			}
			mapRev := selectCommit
			if tt.commitRev != "" {
				mapRev = tt.commitRev
			}

			goRoot := t.TempDir()
			for _, standard := range []string{"embed", "fmt", "strconv", "strings", "testing"} {
				if err := os.MkdirAll(filepath.Join(goRoot, "src", standard), 0o755); err != nil {
					t.Fatal(err)
				}
			}
			config := testcover.SelectorConfig{
				GoRoot: goRoot, MapsDir: mapsDir, WorktreeRoot: root, ModuleRel: ".", ModulePath: "m",
				Commits:     fakeCommits{trees: map[string]map[string]string{mapRev: atCommit, targetCommit: working}},
				Inert:       tt.inert,
				Environment: selectEnvironment,
			}
			var wantRecorded []string
			if tt.wantMiss == "" {
				wantRecorded = slices.DeleteFunc(slices.Clone(fixtureRecorded), func(name string) bool { return slices.Contains(tt.unrecorded, name) })
			}
			for _, target := range []testcover.Target{{Files: workingFiles}, {Commit: targetCommit, Files: workingFiles}} {
				got, err := testcover.NewSelector(config, target).Select("b")
				if err != nil {
					t.Fatalf("Select(b) against %q error = %v", target.Commit, err)
				}
				switch {
				case tt.wantMiss == "" && got.Miss != nil:
					t.Errorf("Select(b) against %q missed %s; want tests %v", target.Commit, got.Miss, tt.want)
				case tt.wantMiss != "" && (got.Miss == nil || got.Miss.Reason != tt.wantMiss || got.Miss.Detail == ""):
					t.Errorf("Select(b) against %q = %+v, miss %v; want a miss for %q with a detail", target.Commit, got, got.Miss, tt.wantMiss)
				case tt.wantDetail != "" && got.Miss.Detail != tt.wantDetail:
					t.Errorf("Select(b) against %q miss detail = %q; want %q", target.Commit, got.Miss.Detail, tt.wantDetail)
				case !reflect.DeepEqual(got.Tests, tt.want):
					t.Errorf("Select(b) against %q .Tests = %v; want %v", target.Commit, got.Tests, tt.want)
				}
				var gotRecorded []string
				for _, recorded := range got.Recorded {
					gotRecorded = append(gotRecorded, recorded.Name)
				}
				if !slices.Equal(gotRecorded, wantRecorded) {
					t.Errorf("Select(b) against %q .Recorded = %v; want %v, the map's passing entries, and none with a miss",
						target.Commit, gotRecorded, wantRecorded)
				}
			}
		})
	}
}

// TestSelectorSelect_UnreadableTarget pins that a commit target the object store cannot read gives an error and no miss.
func TestSelectorSelect_UnreadableTarget(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	mapsDir := t.TempDir()
	if err := testcover.WriteMap(mapsDir, "b", selectFixtureMap(committedFiles)); err != nil {
		t.Fatal(err)
	}
	config := testcover.SelectorConfig{
		MapsDir: mapsDir, ModuleRel: ".", ModulePath: "m", Environment: selectEnvironment,
		Commits: fakeCommits{trees: map[string]map[string]string{selectCommit: committedFiles}},
	}
	got, err := testcover.NewSelector(config, testcover.Target{Commit: "unknown"}).Select("b")
	if err == nil {
		t.Error("Select(b) against an unreadable commit error = nil; want the read failure")
	}
	if got.Miss != nil {
		t.Errorf("Select(b) against an unreadable commit miss = %s; want none, with the error returned", got.Miss)
	}
}
