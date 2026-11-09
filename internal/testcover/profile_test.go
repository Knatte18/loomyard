package testcover_test

import (
	"reflect"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/testcover"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

func TestParseProfile(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	profile := "mode: set\n" +
		"m/a/x.go:1.1,2.2 1 1\n" +
		"m/a/x.go:3.1,4.2 2 0\n" +
		"m/a/b/y.go:5.1,6.2 1 3\n"
	blocks, err := testcover.ParseProfile(strings.NewReader(profile))
	if err != nil {
		t.Fatal(err)
	}
	want := []testcover.ProfileBlock{
		{File: "m/a/x.go", StartLine: 1, StartCol: 1, EndLine: 2, EndCol: 2, Statements: 1, Count: 1},
		{File: "m/a/x.go", StartLine: 3, StartCol: 1, EndLine: 4, EndCol: 2, Statements: 2, Count: 0},
		{File: "m/a/b/y.go", StartLine: 5, StartCol: 1, EndLine: 6, EndCol: 2, Statements: 1, Count: 3},
	}
	if !reflect.DeepEqual(blocks, want) {
		t.Fatalf("ParseProfile = %+v\nwant           %+v", blocks, want)
	}

	if got, want := testcover.CoveredPositions(blocks), blockSet("m/a/x.go:1.1,2.2", "m/a/b/y.go:5.1,6.2"); !reflect.DeepEqual(got, want) {
		t.Errorf("CoveredPositions = %v; want %v", got, want)
	}
	if got := testcover.PackageBlocks(blocks, "m/a"); !reflect.DeepEqual(got, want[:2]) {
		t.Errorf("PackageBlocks(m/a) = %+v; want the two blocks of m/a/x.go, not the nested package's", got)
	}

	for _, malformed := range []string{"no-count-here", "m/a/x.go:1.1,2.2 1 x", "m/a/x.go:1.1 1 1", "x.go 1 1"} {
		if _, err := testcover.ParseProfile(strings.NewReader("mode: set\n" + malformed + "\n")); err == nil {
			t.Errorf("ParseProfile(%q) = nil error; want a malformed line error", malformed)
		}
	}
}
