package testcover_test

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/Knatte18/loomyard/internal/testcover"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// keyedSource holds two same-named methods on different receivers.
// It also holds a function with two identical blocks and package-level code.
// The bodies open at column 21 of their func line and close at column 2 of the line of their brace.
const keyedSource = `package p

func (a A) Do() int {
	return 1
}

func (b *B) Do() int {
	return 1
}

func twice(x bool) int {
	if x {
		return 1
	}
	if x {
		return 1
	}
	return 0
}

var v = 1
`

func TestKeyBlocks(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	block := func(startLine, startCol, endLine, endCol int) testcover.ProfileBlock {
		return testcover.ProfileBlock{File: "m/p/p.go", StartLine: startLine, StartCol: startCol, EndLine: endLine, EndCol: endCol, Statements: 1}
	}
	keysOf := func(t *testing.T, source string, blocks ...testcover.ProfileBlock) []testcover.BlockKey {
		t.Helper()
		dir := t.TempDir()
		if err := os.WriteFile(filepath.Join(dir, "p.go"), []byte(source), 0o644); err != nil {
			t.Fatal(err)
		}
		keys, err := testcover.KeyBlocks(dir, blocks)
		if err != nil {
			t.Fatal(err)
		}
		return keys
	}

	base := keysOf(t, keyedSource,
		block(3, 21, 5, 2), block(7, 22, 9, 2), block(12, 7, 14, 3), block(15, 7, 17, 3), block(21, 9, 21, 10))
	if base[0].Func != "A.Do" || base[1].Func != "B.Do" || base[0] == base[1] {
		t.Errorf("same-named methods keyed %+v and %+v; want A.Do and B.Do with different keys", base[0], base[1])
	}
	if base[0].Hash != base[1].Hash {
		t.Errorf("the two method bodies hash %s and %s; want one hash for one text", base[0].Hash, base[1].Hash)
	}
	if base[2].Func != "twice" || base[3].Func != "twice" || base[2].Hash != base[3].Hash || base[2].Ordinal != 0 || base[3].Ordinal != 1 {
		t.Errorf("identical blocks in one function keyed %+v and %+v; want ordinals 0 and 1", base[2], base[3])
	}
	if base[4].Func != testcover.PackageLevelFunc {
		t.Errorf("package-level block keyed under %q; want %q", base[4].Func, testcover.PackageLevelFunc)
	}

	shifted := keysOf(t, "// added above\n\n"+keyedSource,
		block(5, 21, 7, 2), block(9, 22, 11, 2), block(14, 7, 16, 3), block(17, 7, 19, 3), block(23, 9, 23, 10))
	for i := range base {
		if shifted[i] != base[i] {
			t.Errorf("block %d moved two lines down keyed %+v; want its unshifted key %+v", i, shifted[i], base[i])
		}
	}

	edited := keysOf(t, "package p\n\nfunc (a A) Do() int {\n\treturn 2\n}\n", block(3, 21, 5, 2))
	if edited[0] == base[0] {
		t.Errorf("edited block keyed %+v, as before the edit; want a new key", edited[0])
	}

	// q.go repeats p.go's A.Do body, so the two keys differ only in File.
	// Its generic receiver T[K] keys as T.Do.
	twoFiles := t.TempDir()
	const secondSource = "package p\n\nfunc (a A) Do() int {\n\treturn 1\n}\n\nfunc (t T[K]) Do() int {\n\treturn 1\n}\n"
	for name, source := range map[string]string{"p.go": keyedSource, "q.go": secondSource} {
		if err := os.WriteFile(filepath.Join(twoFiles, name), []byte(source), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	inSecondFile := func(startLine, startCol, endLine, endCol int) testcover.ProfileBlock {
		b := block(startLine, startCol, endLine, endCol)
		b.File = "m/p/q.go"
		return b
	}
	crossFile, err := testcover.KeyBlocks(twoFiles, []testcover.ProfileBlock{block(3, 21, 5, 2), inSecondFile(3, 21, 5, 2), inSecondFile(7, 24, 9, 2)})
	if err != nil {
		t.Fatal(err)
	}
	if crossFile[0].File != "p.go" || crossFile[1].File != "q.go" || crossFile[0].Ordinal != 0 || crossFile[1].Ordinal != 0 ||
		crossFile[0].Func != crossFile[1].Func || crossFile[0].Hash != crossFile[1].Hash {
		t.Errorf("the same body in two files keyed %+v and %+v; want one function and hash, each file's own File, both ordinal 0", crossFile[0], crossFile[1])
	}
	if crossFile[2].Func != "T.Do" {
		t.Errorf("generic receiver T[K] keyed %q; want T.Do", crossFile[2].Func)
	}

	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "p.go"), []byte(keyedSource), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := testcover.KeyBlocks(dir, []testcover.ProfileBlock{block(99, 1, 99, 2)}); err == nil {
		t.Error("KeyBlocks of a line past the file's end = nil error; want an error")
	}
}

// TestCoveredKeys pins that a profile's covered blocks of the package come back as the keys KeyBlocks gives them.
// A block the profile lists twice counts once and as covered when either line counts it,
// and a block of a package nested below is left out.
func TestCoveredKeys(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "p.go"), []byte(keyedSource), 0o644); err != nil {
		t.Fatal(err)
	}
	block := func(file string, startLine, startCol, endLine, endCol, count int) testcover.ProfileBlock {
		return testcover.ProfileBlock{File: file, StartLine: startLine, StartCol: startCol, EndLine: endLine, EndCol: endCol, Statements: 1, Count: count}
	}
	profile := []testcover.ProfileBlock{
		block("m/p/p.go", 3, 21, 5, 2, 0),
		block("m/p/p.go", 7, 22, 9, 2, 0),
		block("m/p/p.go", 3, 21, 5, 2, 1),
		block("m/p/sub/s.go", 1, 1, 1, 2, 1),
	}
	keys, err := testcover.KeyBlocks(dir, []testcover.ProfileBlock{block("m/p/p.go", 3, 21, 5, 2, 0)})
	if err != nil {
		t.Fatal(err)
	}
	got, err := testcover.CoveredKeys(dir, "m/p", profile)
	if err != nil {
		t.Fatal(err)
	}
	if want := blockSet(keys[0].String()); !reflect.DeepEqual(got, want) {
		t.Errorf("CoveredKeys() = %v; want %v, A.Do's key alone", got, want)
	}
}
