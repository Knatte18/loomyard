package loomcli

import (
	"regexp"
	"strings"
	"testing"

	stencils "github.com/Knatte18/loomyard/contracts/stencils"
	"github.com/Knatte18/loomyard/internal/loomshed"
	"github.com/Knatte18/loomyard/internal/shedrun"
)

// driverStencilBody returns the embedded shed driver stencil text.
func driverStencilBody() string {
	return string(stencils.ShedTemplateDriver)
}

// TestDriverStencil_NamesParkMarker pins the driver stencil to the Go-declared park marker filename, so renaming either side without the other fails.
func TestDriverStencil_NamesParkMarker(t *testing.T) {
	t.Parallel()
	want := "`" + shedrun.ParkMarkerFileName + "`"
	if !strings.Contains(driverStencilBody(), want) {
		t.Errorf("shed driver stencil does not contain %s", want)
	}
}

// TestDriverStencil_FilesNothing pins that the driver files no issue itself: the run's reflection is the one filer.
func TestDriverStencil_FilesNothing(t *testing.T) {
	t.Parallel()
	if strings.Contains(driverStencilBody(), "lyx selfreport create") {
		t.Error("shed driver stencil names `lyx selfreport create`; the driver files nothing")
	}
}

// TestDriverStencil_NamesFrictionFailed pins the parent notification on a `done` stop whose envelope reports a failed reflection.
func TestDriverStencil_NamesFrictionFailed(t *testing.T) {
	t.Parallel()
	if !strings.Contains(driverStencilBody(), "friction: failed") {
		t.Error("shed driver stencil does not contain `friction: failed`")
	}
}

// TestDriverStencil_NamesParentNotice pins the relay of an `awaiting` stop's `parent_notice` to the parent.
func TestDriverStencil_NamesParentNotice(t *testing.T) {
	t.Parallel()
	if !strings.Contains(driverStencilBody(), "`parent_notice`") {
		t.Error("shed driver stencil does not contain `parent_notice`")
	}
}

// TestDriverStencil_NamesNoLoomRow pins the stencil's own claim that it carries no phase knowledge: no row name in loomshed.InterruptPolicies appears as a whole word.
func TestDriverStencil_NamesNoLoomRow(t *testing.T) {
	t.Parallel()
	body := driverStencilBody()
	for row := range loomshed.InterruptPolicies {
		re := regexp.MustCompile(`(^|[^A-Za-z-])` + regexp.QuoteMeta(row) + `($|[^A-Za-z-])`)
		if re.MatchString(body) {
			t.Errorf("shed driver stencil names the loom row %q", row)
		}
	}
}
