// filingmark_test.go pins the filing mark's place, its relative form and its round trip.

package shedrun

import (
	"path/filepath"
	"testing"

	"github.com/Knatte18/loomyard/internal/lymdirs"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

func TestFilingMark(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()
	l := syntheticLocation(t)

	if want := filepath.Join(l.AnchorPath(), lymdirs.LymDirName, "shed", "slug", "filing-mark"); FilingMarkFile(l, "slug") != want {
		t.Errorf("FilingMarkFile() = %q; want %q", FilingMarkFile(l, "slug"), want)
	}
	if want := filepath.Join(lymdirs.LymDirName, "shed", "slug", "filing-mark"); FilingMarkRel(l, "slug") != want {
		t.Errorf("FilingMarkRel() = %q; want %q", FilingMarkRel(l, "slug"), want)
	}
	if marked, err := FilingMarked(l, "slug"); err != nil || marked {
		t.Errorf("FilingMarked() before a write = %t, %v; want false and nil", marked, err)
	}
	for write := 1; write <= 2; write++ {
		if err := WriteFilingMark(l, "slug"); err != nil {
			t.Fatalf("WriteFilingMark() #%d = %v; want nil", write, err)
		}
		if marked, err := FilingMarked(l, "slug"); err != nil || !marked {
			t.Errorf("FilingMarked() after write #%d = %t, %v; want true and nil", write, marked, err)
		}
	}
}
