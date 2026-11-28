// triage_test.go covers the triage lines a failed test gets from the coverage maps.

package gatecli

import (
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"

	"github.com/Knatte18/loomyard/internal/testcover"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
	"github.com/Knatte18/loomyard/internal/testreach"
)

func TestTriageLookup_Lines(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	const (
		modulePath = "ex.com/m"
		mapCommit  = "c1abc"
	)
	funcNames := []string{"F", "G", "H", "I", "J", "K"}
	var changeSet []testreach.ChangedFunc
	var blocks []testcover.BlockKey
	var everyBlock []int
	for i, name := range funcNames {
		changeSet = append(changeSet, testreach.ChangedFunc{Key: testcover.FuncKey{Package: modulePath + "/a", File: "a.go", Func: name}})
		blocks = append(blocks, testcover.BlockKey{File: "a.go", Func: name, Hash: "h"})
		everyBlock = append(everyBlock, i)
	}
	blocks = append(blocks, testcover.BlockKey{File: "z.go", Func: "Z", Hash: "h"})
	sharedDir := filepath.Join(t.TempDir(), "shared")
	if err := testcover.WriteMap(sharedDir, "a", testcover.Map{
		Commit: mapCommit, BuiltAt: time.Now().Add(-time.Hour), Blocks: blocks,
		Tests: []testcover.MapTest{
			{Name: "TestSix", Blocks: everyBlock},
			{Name: "TestTwo", Blocks: []int{0, 1}},
			{Name: "TestNone", Blocks: []int{len(funcNames)}},
		},
	}); err != nil {
		t.Fatal(err)
	}
	module := testreach.Module{Dir: t.TempDir(), Path: modulePath}
	open := func() (*testreach.Reader, error) { return testreach.Open(module, sharedDir, "") }
	changedSet := func(string) ([]testreach.ChangedFunc, error) { return changeSet, nil }
	withBase := func() (string, bool, error) { return "base", true, nil }
	prefix := "triage: "

	tests := []struct {
		name      string
		open      func() (*testreach.Reader, error)
		mergeBase func() (string, bool, error)
		changed   func(string) ([]testreach.ChangedFunc, error)
		test      string
		want      []string
		wantNone  bool
	}{
		{
			name: "A test reaching two changed functions names them and the map's commit",
			open: open, mergeBase: withBase, changed: changedSet, test: "TestTwo",
			want: []string{prefix + "reaches changed functions: a.F, a.G; map commit " + mapCommit},
		},
		{
			name: "A test reaching six names five and the count of the rest",
			open: open, mergeBase: withBase, changed: changedSet, test: "TestSix",
			want: []string{prefix + "reaches changed functions: a.F, a.G, a.H, a.I, a.J and 1 more; map commit " + mapCommit},
		},
		{
			name: "A test reaching none says so and gives the command for its own line",
			open: open, mergeBase: withBase, changed: changedSet, test: "TestNone",
			want: []string{
				prefix + "reaches none of this run's changes; map commit " + mapCommit,
				prefix + "lym gate reach --test a.TestNone --since <last green commit>",
			},
		},
		{
			name: "A subtest is looked up by its top-level test",
			open: open, mergeBase: withBase, changed: changedSet, test: "TestTwo/sub/deeper",
			want: []string{prefix + "reaches changed functions: a.F, a.G; map commit " + mapCommit},
		},
		{
			name: "A test no map holds is not in the coverage maps",
			open: open, mergeBase: withBase, changed: changedSet, test: "TestUnmapped",
			want: []string{prefix + "not in the coverage maps"},
		},
		{
			name: "No parent branch gives a line with its reason and no command",
			open: open, mergeBase: func() (string, bool, error) { return "", false, nil }, test: "TestNone",
			want: []string{prefix + "no change set exists, no parent branch; map commit " + mapCommit},
		},
		{
			name: "A failed merge base read gives a line with its reason and no command",
			open: open, mergeBase: func() (string, bool, error) { return "", false, errors.New("git broke") }, test: "TestNone",
			want: []string{prefix + "no change set exists, the merge base read failed; map commit " + mapCommit},
		},
		{
			name: "A failed read of the changed functions gives a line with its reason and no command",
			open: open, mergeBase: withBase, test: "TestNone",
			changed: func(string) ([]testreach.ChangedFunc, error) { return nil, errors.New("read broke") },
			want:    []string{prefix + "no change set exists, the changed functions could not be read; map commit " + mapCommit},
		},
		{
			name: "A failed open builds no lookup",
			open: func() (*testreach.Reader, error) { return nil, errors.New("maps broke") }, mergeBase: withBase, changed: changedSet,
			test: "TestTwo", wantNone: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			lookup, ok := newTriageLookup(tt.open, tt.mergeBase, tt.changed)
			if ok == tt.wantNone {
				t.Fatalf("newTriageLookup() ok = %v; want %v", ok, !tt.wantNone)
			}
			if !ok {
				return
			}
			lookup.relative = func(importPath string) string { return strings.TrimPrefix(importPath, modulePath+"/") }
			if diff := cmp.Diff(tt.want, lookup.lines(modulePath+"/a", tt.test)); diff != "" {
				t.Errorf("lines(a, %s) mismatch (-want +got):\n%s", tt.test, diff)
			}
		})
	}
}
