package loomcli

import (
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/loomshed"
	"github.com/Knatte18/loomyard/internal/shedrun"
)

// TestDriveSkill_NamesParkMarker pins the ly-drive skill to the Go-declared park marker filename, so renaming either side without the other fails.
func TestDriveSkill_NamesParkMarker(t *testing.T) {
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	skill := filepath.Join(filepath.Dir(file), "..", "..", "plugins", "ly", "skills", "ly-drive", "SKILL.md")
	body, err := os.ReadFile(skill)
	if err != nil {
		t.Fatalf("read skill: %v", err)
	}
	want := "`" + shedrun.ParkMarkerFileName + "`"
	if !strings.Contains(string(body), want) {
		t.Errorf("SKILL.md does not contain %s", want)
	}
}

// readDriveSkill returns the ly-drive skill text.
func readDriveSkill(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	skill := filepath.Join(filepath.Dir(file), "..", "..", "plugins", "ly", "skills", "ly-drive", "SKILL.md")
	body, err := os.ReadFile(skill)
	if err != nil {
		t.Fatalf("read skill: %v", err)
	}
	return string(body)
}

// TestDriveSkill_FilesNothing pins that the driver files no issue itself: the run's reflection is the one filer.
func TestDriveSkill_FilesNothing(t *testing.T) {
	body := readDriveSkill(t)
	if strings.Contains(body, "lyx selfreport create") {
		t.Error("SKILL.md names `lyx selfreport create`; the driver files nothing")
	}
}

// TestDriveSkill_NamesFrictionFailed pins the parent notification on a `done` stop whose envelope reports a failed reflection.
func TestDriveSkill_NamesFrictionFailed(t *testing.T) {
	if !strings.Contains(readDriveSkill(t), "friction: failed") {
		t.Error("SKILL.md does not contain `friction: failed`")
	}
}

// TestDriveSkill_NamesParentNotice pins the relay of an `awaiting` stop's `parent_notice` to the parent.
func TestDriveSkill_NamesParentNotice(t *testing.T) {
	if !strings.Contains(readDriveSkill(t), "`parent_notice`") {
		t.Error("SKILL.md does not contain `parent_notice`")
	}
}

// TestDriveSkill_NamesNoLoomRow pins the skill's own claim that it carries no phase knowledge: no row name in loomshed.InterruptPolicies appears as a whole word.
func TestDriveSkill_NamesNoLoomRow(t *testing.T) {
	body := readDriveSkill(t)
	for row := range loomshed.InterruptPolicies {
		re := regexp.MustCompile(`(^|[^A-Za-z-])` + regexp.QuoteMeta(row) + `($|[^A-Za-z-])`)
		if re.MatchString(body) {
			t.Errorf("SKILL.md names the loom row %q", row)
		}
	}
}
