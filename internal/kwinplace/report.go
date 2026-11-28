// report.go reads the marker lines the KWin scripts log to the journal.

package kwinplace

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

// scriptLineMarker opens every line a script logs, before the run id.
const scriptLineMarker = "lym-kwinplace "

// The kinds of line a script logs.
const (
	kindCurrent  = "current"
	kindDesktop  = "desktop"
	kindOutput   = "output"
	kindTiling   = "tiling"
	kindTiles    = "tiles"
	kindWindow   = "window"
	kindArea     = "area"
	kindNoOutput = "no-output"
	kindEnd      = "end"
)

// ErrNoTilingOutput reports that KWin drives no output of the name a script was told to tile on.
var ErrNoTilingOutput = errors.New("kwinplace: KWin has no output of the tiling screen's name")

// ReportLines returns the lines of the run runID in journal, in order, each as "<kind> <json>".
// It also reports whether the run's end marker is among them.
// Lines after the end marker and lines of other runs are left out.
func ReportLines(journal []byte, runID string) ([]string, bool) {
	prefix := scriptLineMarker + runID + " "
	var lines []string
	for line := range strings.SplitSeq(string(journal), "\n") {
		_, rest, found := strings.Cut(line, prefix)
		if !found {
			continue
		}
		lines = append(lines, rest)
		if kind, _, _ := strings.Cut(rest, " "); kind == kindEnd {
			return lines, true
		}
	}
	return lines, false
}

// decodeReport calls handle with the kind and JSON of each line, the end marker left out.
// A no-output line is ErrNoTilingOutput naming the output.
func decodeReport(lines []string, handle func(kind string, decode func(v any) error) error) error {
	for _, line := range lines {
		kind, payload, _ := strings.Cut(line, " ")
		decode := func(v any) error {
			if err := json.Unmarshal([]byte(payload), v); err != nil {
				return fmt.Errorf("kwinplace: decode %s line: %w", kind, err)
			}
			return nil
		}
		switch kind {
		case kindEnd:
		case kindNoOutput:
			var missing struct {
				Output string `json:"output"`
			}
			if err := decode(&missing); err != nil {
				return err
			}
			return fmt.Errorf("%w: %q", ErrNoTilingOutput, missing.Output)
		default:
			if err := handle(kind, decode); err != nil {
				return err
			}
		}
	}
	return nil
}

// desktopTiles is the payload of a tiles line.
type desktopTiles struct {
	Desktop string `json:"desktop"`
	Tiles   []Tile `json:"tiles"`
}

// ParseQueryReport decodes the lines of a query run into the snapshot they describe.
func ParseQueryReport(lines []string) (Snapshot, error) {
	snapshot := Snapshot{Tiles: map[string][]Tile{}}
	err := decodeReport(lines, func(kind string, decode func(any) error) error {
		switch kind {
		case kindCurrent:
			var current struct {
				ID string `json:"id"`
			}
			err := decode(&current)
			snapshot.CurrentDesktop = current.ID
			return err
		case kindDesktop:
			var desktop Desktop
			err := decode(&desktop)
			snapshot.Desktops = append(snapshot.Desktops, desktop)
			return err
		case kindOutput:
			var output Output
			err := decode(&output)
			snapshot.Outputs = append(snapshot.Outputs, output)
			return err
		case kindTiling:
			var tiling struct {
				PerDesktop bool `json:"perDesktop"`
			}
			err := decode(&tiling)
			snapshot.PerDesktopTiling = tiling.PerDesktop
			return err
		case kindTiles:
			var tiles desktopTiles
			err := decode(&tiles)
			snapshot.Tiles[tiles.Desktop] = tiles.Tiles
			return err
		case kindWindow:
			var window Window
			err := decode(&window)
			snapshot.Windows = append(snapshot.Windows, window)
			return err
		}
		return fmt.Errorf("kwinplace: a query report holds a line of unknown kind %q", kind)
	})
	if err != nil {
		return Snapshot{}, err
	}
	return snapshot, nil
}

// ParseApplyReport decodes the lines of an apply run into its report.
func ParseApplyReport(lines []string) (ApplyReport, error) {
	var report ApplyReport
	err := decodeReport(lines, func(kind string, decode func(any) error) error {
		switch kind {
		case kindArea:
			return decode(&report.Area)
		case kindTiles:
			var tiles desktopTiles
			err := decode(&tiles)
			report.Tiles = tiles.Tiles
			return err
		case kindWindow:
			var window AppliedWindow
			err := decode(&window)
			report.Windows = append(report.Windows, window)
			return err
		}
		return fmt.Errorf("kwinplace: an apply report holds a line of unknown kind %q", kind)
	})
	if err != nil {
		return ApplyReport{}, err
	}
	return report, nil
}
