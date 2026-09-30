package loomcli

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

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
