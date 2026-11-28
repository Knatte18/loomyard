// recordings_test.go runs the readers over every recording of KWin and kscreen-doctor.

package kwinplace

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

const recordingsDir = "testdata/recordings"

func readRecording(t *testing.T, name string) []byte {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(recordingsDir, filepath.FromSlash(name)))
	if err != nil {
		t.Fatal(err)
	}
	return data
}

// journalRunIDs returns the run ids a journal recording holds, in the order they first appear.
func journalRunIDs(journal []byte) []string {
	var ids []string
	for line := range strings.SplitSeq(string(journal), "\n") {
		_, rest, found := strings.Cut(line, scriptLineMarker)
		if !found {
			continue
		}
		id, _, _ := strings.Cut(rest, " ")
		if !slices.Contains(ids, id) {
			ids = append(ids, id)
		}
	}
	return ids
}

func applyReportOf(t *testing.T, name, runID string) ApplyReport {
	t.Helper()
	lines, ended := ReportLines(readRecording(t, name), runID)
	if !ended {
		t.Fatalf("ReportLines(%s, %s) did not end", name, runID)
	}
	report, err := ParseApplyReport(lines)
	if err != nil {
		t.Fatalf("ParseApplyReport(%s) error = %v", name, err)
	}
	return report
}

// requireTiledUnderPanel asserts the window stays tiled in the zone, which runs below the usable area.
func requireTiledUnderPanel(t *testing.T, report ApplyReport, windowID string, zone int) {
	t.Helper()
	tile := report.Tiles[zone].AbsoluteGeometry
	if tile.Y+tile.Height <= report.Area.Y+report.Area.Height {
		t.Errorf("zone %d ends at %v, not below the usable area %v", zone, tile.Y+tile.Height, report.Area)
	}
	if !slices.Contains(report.Tiles[zone].Windows, windowID) {
		t.Errorf("zone %d holds %q; want %s", zone, report.Tiles[zone].Windows, windowID)
	}
	i := slices.IndexFunc(report.Windows, func(w AppliedWindow) bool { return w.InternalID == windowID })
	if i < 0 {
		t.Fatalf("window %s is not reported", windowID)
	}
	if w := report.Windows[i]; w.Zone != zone || !w.Tiled {
		t.Errorf("window %s zone = %d tiled = %v; want %d tiled", windowID, w.Zone, w.Tiled, zone)
	}
}

func TestRecordings_EveryRecordingReads(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	entries, err := os.ReadDir(recordingsDir)
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		dir := filepath.Join(recordingsDir, entry.Name())
		if journal, err := os.ReadFile(filepath.Join(dir, "journal.txt")); err == nil {
			for _, runID := range journalRunIDs(journal) {
				lines, ended := ReportLines(journal, runID)
				if !ended {
					t.Errorf("%s run %s has no end marker", entry.Name(), runID)
				}
				var err error
				if strings.HasPrefix(entry.Name(), "neve-apply") && !slices.ContainsFunc(lines, func(l string) bool { return strings.HasPrefix(l, kindCurrent) }) {
					_, err = ParseApplyReport(lines)
				} else {
					_, err = ParseQueryReport(lines)
				}
				if err != nil && !errors.Is(err, ErrNoTilingOutput) {
					t.Errorf("%s run %s: %v", entry.Name(), runID, err)
				}
			}
		}
		if data, err := os.ReadFile(filepath.Join(dir, "kscreen.json")); err == nil {
			if _, err := TilingOutput(data); err != nil {
				t.Errorf("TilingOutput(%s) error = %v", entry.Name(), err)
			}
		}
	}

	t.Run("Query", func(t *testing.T) {
		t.Parallel()
		const runID = "3e269ffe3222"
		const gridDesktop = "5d6bb87b-8f11-4cb4-b721-297136f4e2c2"
		journal := readRecording(t, "neve-query/journal.txt")

		lines, ended := ReportLines(journal, runID)
		if !ended || len(lines) != 38 {
			t.Fatalf("ReportLines() = %d lines, ended %v; want the run's 38 lines, ended", len(lines), ended)
		}
		snapshot, err := ParseQueryReport(lines)
		if err != nil {
			t.Fatalf("ParseQueryReport() error = %v", err)
		}
		if snapshot.CurrentDesktop != "d96ae0ca-6369-4493-9eff-ad0f7743711b" || !snapshot.PerDesktopTiling {
			t.Errorf("current = %q perDesktop = %v", snapshot.CurrentDesktop, snapshot.PerDesktopTiling)
		}
		if len(snapshot.Desktops) != 10 || len(snapshot.Tiles) != len(snapshot.Desktops) {
			t.Errorf("desktops = %d with tiles for %d; want 10 and 10", len(snapshot.Desktops), len(snapshot.Tiles))
		}
		first := snapshot.Tiles[gridDesktop][0]
		want := Rect{X: 2048, Y: 0, Width: 640, Height: 720}
		if first.AbsoluteGeometry != want || first.Padding != 4 || !slices.Equal(first.Windows, []string{"{361307ab-c0d4-43ce-9f90-fcb0f7188820}"}) {
			t.Errorf("first zone = %+v", first)
		}
		if !slices.ContainsFunc(snapshot.Windows, func(w Window) bool { return w.InternalID == "{361307ab-c0d4-43ce-9f90-fcb0f7188820}" }) {
			t.Errorf("snapshot windows lack the window the first zone holds")
		}
		if zone, ok := zoneHolding(snapshot.Tiles[gridDesktop], "{7028d260-eabc-4af0-b043-3833ac29ac27}"); !ok || zone != 1 {
			t.Errorf("zoneHolding() = %d, %v; want 1", zone, ok)
		}

		cut := journal[:strings.Index(string(journal), runID+" end ")]
		if _, ended := ReportLines(cut, runID); ended {
			t.Errorf("ReportLines() of a journal cut before the end marker reads as ended")
		}
	})

	t.Run("NoOutput", func(t *testing.T) {
		t.Parallel()
		lines, _ := ReportLines(readRecording(t, "neve-query-no-output/journal.txt"), "ddb78d6734d7")
		_, err := ParseQueryReport(lines)
		if !errors.Is(err, ErrNoTilingOutput) || !strings.Contains(err.Error(), "NOPE-1") {
			t.Errorf("ParseQueryReport() error = %v; want ErrNoTilingOutput naming NOPE-1", err)
		}
	})

	t.Run("UnknownKindRefused", func(t *testing.T) {
		t.Parallel()
		lines := []string{"bogus {}"}
		if _, err := ParseQueryReport(lines); err == nil || !strings.Contains(err.Error(), `unknown kind "bogus"`) {
			t.Errorf("ParseQueryReport() error = %v; want the unknown kind named", err)
		}
		if _, err := ParseApplyReport(lines); err == nil || !strings.Contains(err.Error(), `unknown kind "bogus"`) {
			t.Errorf("ParseApplyReport() error = %v; want the unknown kind named", err)
		}
	})

	t.Run("Apply", func(t *testing.T) {
		t.Parallel()
		report := applyReportOf(t, "neve-apply/journal.txt", "d5083f4ee1ed")
		if want := (Rect{X: 2048, Y: 0, Width: 2560, Height: 1368}); report.Area != want || len(report.Tiles) != 4 {
			t.Fatalf("area = %v with %d zones; want %v with 4", report.Area, len(report.Tiles), want)
		}
		requireTiledUnderPanel(t, report, "{039c9409-39b2-4563-b6f8-c104f2d75b17}", 2)
	})

	t.Run("ApplyOneRow", func(t *testing.T) {
		t.Parallel()
		report := applyReportOf(t, "neve-apply-one-row/journal.txt", "111ffdc5b886")
		if len(report.Tiles) != 2 {
			t.Fatalf("zones = %d; want 2", len(report.Tiles))
		}
		requireTiledUnderPanel(t, report, "{c4836ac8-556a-4ffb-88df-15c3a01190ff}", 0)
		requireTiledUnderPanel(t, report, "{5c963c1e-af35-47e5-883d-3fa5d0ea8b8f}", 1)
	})

	t.Run("Kscreen", func(t *testing.T) {
		t.Parallel()
		for _, name := range []string{"neve-kscreen/kscreen.json", "neve-kscreen-laptop-off/kscreen.json"} {
			got, err := TilingOutput(readRecording(t, name))
			if err != nil || got != "DP-2" {
				t.Errorf("TilingOutput(%s) = %q, %v; want DP-2", name, got, err)
			}
		}

		var recorded struct {
			Outputs []map[string]any `json:"outputs"`
		}
		if err := json.Unmarshal(readRecording(t, "neve-kscreen/kscreen.json"), &recorded); err != nil {
			t.Fatal(err)
		}
		marshal := func() []byte {
			data, err := json.Marshal(recorded)
			if err != nil {
				t.Fatal(err)
			}
			return data
		}
		// Reversed, another enabled output comes before the one of the lowest priority.
		slices.Reverse(recorded.Outputs)
		if got, err := TilingOutput(marshal()); err != nil || got != "DP-2" {
			t.Errorf("TilingOutput() of the reversed outputs = %q, %v; want DP-2", got, err)
		}
		recorded.Outputs = slices.DeleteFunc(recorded.Outputs, func(o map[string]any) bool { return o["enabled"] == true })
		if got, err := TilingOutput(marshal()); err == nil {
			t.Errorf("TilingOutput() with no enabled output = %q; want an error", got)
		}
	})
}
