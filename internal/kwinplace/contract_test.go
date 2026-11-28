// contract_test.go drives a real KWin session and compares what the package reads from it with the recordings.
// It needs a Plasma 6 Wayland desktop and is run by hand.

package kwinplace

import (
	"encoding/json"
	"maps"
	"os"
	"os/exec"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/lock"
	"github.com/Knatte18/loomyard/internal/proc"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// lineKinds returns the kinds the report lines hold, sorted.
func lineKinds(lines []string) []string {
	var kinds []string
	for _, line := range lines {
		kind, _, _ := strings.Cut(line, " ")
		kinds = append(kinds, kind)
	}
	slices.Sort(kinds)
	return slices.Compact(kinds)
}

// addFieldNames adds to names the path of every object key in value, looking into nested objects and arrays.
func addFieldNames(names map[string]bool, path string, value any) {
	switch v := value.(type) {
	case map[string]any:
		for key, child := range v {
			names[path+"."+key] = true
			addFieldNames(names, path+"."+key, child)
		}
	case []any:
		for _, child := range v {
			addFieldNames(names, path, child)
		}
	}
}

// reportFieldNames returns the field names the report lines hold, each under its line's kind, sorted.
func reportFieldNames(t *testing.T, lines []string) []string {
	t.Helper()
	names := map[string]bool{}
	for _, line := range lines {
		kind, payload, _ := strings.Cut(line, " ")
		var value any
		if err := json.Unmarshal([]byte(payload), &value); err != nil {
			t.Fatalf("decode %q: %v", line, err)
		}
		addFieldNames(names, kind, value)
	}
	return slices.Sorted(maps.Keys(names))
}

// requireOutputFields asserts every output of the kscreen JSON holds the fields that rank the screens.
func requireOutputFields(t *testing.T, source string, kscreenJSON []byte) {
	t.Helper()
	var screens struct {
		Outputs []map[string]any `json:"outputs"`
	}
	if err := json.Unmarshal(kscreenJSON, &screens); err != nil {
		t.Fatalf("decode %s kscreen JSON: %v", source, err)
	}
	if len(screens.Outputs) == 0 {
		t.Errorf("%s kscreen JSON holds no output", source)
	}
	for _, output := range screens.Outputs {
		for _, field := range []string{"name", "enabled", "priority"} {
			if _, ok := output[field]; !ok {
				t.Errorf("%s output %v lacks %q", source, output["name"], field)
			}
		}
	}
}

func TestContract_TheRealKWinAnswersAsRecorded(t *testing.T) {
	tiergate.Require(t, 3)
	t.Parallel()

	runner, err := NewRunner()
	if err != nil {
		t.Fatal(err)
	}
	ctx := t.Context()
	tilingOutput, err := runner.ReadTilingOutput(ctx)
	if err != nil {
		t.Fatalf("ReadTilingOutput() error = %v", err)
	}
	kscreenJSON, err := runner.call(ctx, kscreenProgram, "-j")
	if err != nil {
		t.Fatal(err)
	}
	requireOutputFields(t, "real", kscreenJSON)
	requireOutputFields(t, "recorded", readRecording(t, "neve-kscreen/kscreen.json"))

	lines, err := runner.runScript(ctx, func(runID string) string { return QueryScript(runID, tilingOutput) }, time.Minute)
	if err != nil {
		t.Fatalf("query run error = %v", err)
	}
	recorded, _ := ReportLines(readRecording(t, "neve-query/journal.txt"), queryRunID)
	if got, want := lineKinds(lines), lineKinds(recorded); !slices.Equal(got, want) {
		t.Errorf("report kinds = %q; want %q", got, want)
	}
	if got, want := reportFieldNames(t, lines), reportFieldNames(t, recorded); !slices.Equal(got, want) {
		t.Errorf("report fields = %q; want %q", got, want)
	}
	snapshot, err := ParseQueryReport(lines)
	if err != nil {
		t.Fatalf("ParseQueryReport() error = %v", err)
	}
	if snapshot.CurrentDesktop == "" || len(snapshot.Desktops) == 0 || len(snapshot.Windows) == 0 || len(snapshot.Tiles) != len(snapshot.Desktops) {
		t.Errorf("snapshot lacks fields: current %q, %d desktops, %d windows, %d tiled", snapshot.CurrentDesktop, len(snapshot.Desktops), len(snapshot.Windows), len(snapshot.Tiles))
	}
	if !slices.ContainsFunc(snapshot.Outputs, func(o Output) bool { return o.Name == tilingOutput }) {
		t.Errorf("tiling output %q is not among %+v", tilingOutput, snapshot.Outputs)
	}
	if !slices.ContainsFunc(snapshot.Windows, func(w Window) bool {
		// The script reports an empty output name when it reads none, so the name is checked here and not by the key.
		return w.InternalID != "" && w.PID > 0 && w.ResourceClass != "" && w.Caption != "" && w.Output != "" && w.Frame != (Rect{})
	}) {
		t.Errorf("no window has every field filled: %+v", snapshot.Windows)
	}
	tileCount := 0
	for _, tiles := range snapshot.Tiles {
		for _, tile := range tiles {
			tileCount++
			if tile.AbsoluteGeometry == (Rect{}) {
				t.Errorf("tile has no geometry: %+v", tile)
			}
		}
	}
	if tileCount == 0 {
		t.Error("the snapshot holds no tile")
	}

	scriptLock, err := runner.lockScripts(ctx, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	defer func(held *lock.FileLock) { held.Release() }(scriptLock)
	plugin := pluginNamePrefix + "contract-" + randomRunID()
	file, err := writeScriptFile(QueryScript(randomRunID(), tilingOutput))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Remove(file) })
	t.Cleanup(func() { runner.unload(t.Context(), plugin) })

	answer := func(method string, args ...string) string {
		t.Helper()
		output, err := runner.call(ctx, qdbusProgram, append([]string{kwinService, scriptingObject, method}, args...)...)
		if err != nil {
			t.Fatal(err)
		}
		return strings.TrimSpace(string(output))
	}
	recording := func(name string) string { return strings.TrimSpace(string(readRecording(t, "neve-qdbus/"+name))) }

	// A load answers the script's id, a number of at least 0 that differs from run to run.
	first := answer("loadScript", file, plugin)
	for _, load := range []string{first, recording("load.txt")} {
		if id, err := strconv.Atoi(load); err != nil || id < 0 {
			t.Errorf("load = %q; want a script id, first load %q and recorded %q", load, first, recording("load.txt"))
		}
	}
	if got, want := answer("loadScript", file, plugin), recording("load-taken.txt"); got != want {
		t.Errorf("second load = %q; want %q", got, want)
	}
	if got, want := answer("isScriptLoaded", plugin), recording("loaded-true.txt"); got != want {
		t.Errorf("isScriptLoaded while loaded = %q; want %q", got, want)
	}
	if err := runner.unload(ctx, plugin); err != nil {
		t.Fatalf("script %s did not unload: %v", plugin, err)
	}
	if got, want := answer("isScriptLoaded", plugin), recording("loaded-false.txt"); got != want {
		t.Errorf("isScriptLoaded after the unload = %q; want %q", got, want)
	}
}

// TestContract_TheRealKWinAppliesAsRecorded moves a throwaway window onto a desktop that is not current.
// It builds a grid there, replacing that desktop's grid, so run it only on purpose:
//
//	lym gate test --tier 3 ./internal/kwinplace -- -run TestContract_TheRealKWinAppliesAsRecorded
func TestContract_TheRealKWinAppliesAsRecorded(t *testing.T) {
	tiergate.Require(t, 3)
	t.Parallel()

	runner, err := NewRunner()
	if err != nil {
		t.Fatal(err)
	}
	ctx := t.Context()
	tilingOutput, err := runner.ReadTilingOutput(ctx)
	if err != nil {
		t.Fatalf("ReadTilingOutput() error = %v", err)
	}

	title := "kwinplace-contract-" + randomRunID()
	window := exec.Command("konsole", "--separate", "-p", "tabtitle="+title, "-e", "sleep", "120")
	if err := window.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		proc.KillPID(window.Process.Pid)
		window.Wait()
	})

	var snapshot Snapshot
	var placed Window
	deadline := time.Now().Add(30 * time.Second)
	for placed.InternalID == "" {
		if time.Now().After(deadline) {
			t.Fatal("the throwaway window never appeared in KWin")
		}
		time.Sleep(time.Second)
		snapshot, _, err = runner.Query(ctx, tilingOutput, time.Now(), time.Minute)
		if err != nil {
			t.Fatalf("Query() error = %v", err)
		}
		for _, w := range snapshot.Windows {
			if w.Matches(window.Process.Pid, title) {
				placed = w
			}
		}
	}

	target := ""
	for _, desktop := range snapshot.Desktops {
		if desktop.ID != snapshot.CurrentDesktop {
			target = desktop.ID
		}
	}
	// Two columns give the grid leaves to report; a one by one grid has none.
	order := ApplyOrder{
		TilingOutput: tilingOutput,
		Desktop:      target,
		Rows:         1,
		Cols:         2,
		Zones:        []string{placed.InternalID, ""},
		Placed:       placed.InternalID,
	}
	lines, err := runner.runScript(ctx, func(runID string) string { return ApplyScript(runID, order) }, time.Minute)
	if err != nil {
		t.Fatalf("apply run error = %v", err)
	}
	recorded, _ := ReportLines(readRecording(t, "neve-apply/journal.txt"), applyRunID)
	if got, want := lineKinds(lines), lineKinds(recorded); !slices.Equal(got, want) {
		t.Errorf("report kinds = %q; want %q", got, want)
	}
	report, err := ParseApplyReport(lines)
	if err != nil {
		t.Fatalf("ParseApplyReport() error = %v", err)
	}

	if report.Area == (Rect{}) {
		t.Error("the report has no usable area")
	}
	if len(report.Tiles) != 2 {
		t.Fatalf("tiles = %+v; want 2", report.Tiles)
	}
	for _, tile := range report.Tiles {
		if tile.AbsoluteGeometry == (Rect{}) {
			t.Errorf("tile = %+v; want a geometry", tile)
		}
	}
	// A padding of 0 is a setting a desktop may hold, so the script's line is checked for the field, not the value.
	if !slices.ContainsFunc(lines, func(l string) bool {
		return strings.HasPrefix(l, kindTiles+" ") && strings.Contains(l, `"padding":`)
	}) {
		t.Errorf("no tiles line carries a padding: %q", lines)
	}
	i := slices.IndexFunc(report.Windows, func(w AppliedWindow) bool { return w.InternalID == placed.InternalID })
	if i < 0 {
		t.Fatalf("the placed window is not reported")
	}
	got := report.Windows[i]
	if !slices.Equal(got.Desktops, []string{target}) || got.Frame == (Rect{}) || got.Zone != 0 || !got.Tiled {
		t.Errorf("placed window = %+v; want it on %s alone with a frame, tiled in zone 0", got, target)
	}
}
