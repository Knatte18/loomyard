// prompt.go composes each seat's stencil value map, its filled prompt and its shuttle spec.

package seatengine

import (
	"fmt"
	"maps"
	"slices"
	"strings"

	"github.com/Knatte18/loomyard/internal/agentname"
	"github.com/Knatte18/loomyard/internal/editdirective"
	"github.com/Knatte18/loomyard/internal/parentdirective"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
	"github.com/Knatte18/loomyard/internal/stencil"
	"github.com/Knatte18/loomyard/internal/stencilstore"
)

// noneValue is what a list marker renders when its list is empty.
const noneValue = "(none)"

// strandNames returns the full strand name each seat of table takes, keyed by seat name.
// The names are deterministic, so seats address each other by them before any strand exists.
func strandNames(geom Geometry, table Table) (map[string]string, error) {
	names := make(map[string]string, len(table.Seats))
	for _, seat := range table.Seats {
		name, err := agentname.Format(geom.Shortname, geom.Slug, SeatRole(table.RolePrefix, seat.Name))
		if err != nil {
			return nil, fmt.Errorf("seatengine: seat %q: %w", seat.Name, err)
		}
		names[seat.Name] = name
	}
	return names, nil
}

// joinLines joins items one per line, or renders noneValue for an empty list.
func joinLines(items []string) string {
	if len(items) == 0 {
		return noneValue
	}
	return strings.Join(items, "\n")
}

// seatValues returns the value map seat's stencil is filled from: the table's values, then the seat's own, then every reserved marker.
// names is strandNames' answer, and failed lists the strand names of advisors that never started.
// A chair's parent directive names the run's parent; an advisor's names the chair and is never interactive.
func seatValues(geom Geometry, table Table, seat Seat, names map[string]string, failed []string) (map[string]string, error) {
	values := make(map[string]string, len(table.Values)+len(seat.Values)+len(reservedMarkers))
	maps.Copy(values, table.Values)
	maps.Copy(values, seat.Values)

	parentName, interactive := names[RoleChair], false
	if seat.Name == RoleChair {
		parentName, interactive = geom.ParentName, table.Interactive
	}
	parent, err := parentdirective.Directive(geom.StencilsDir, parentName, interactive)
	if err != nil {
		return nil, fmt.Errorf("seatengine: seat %q: %w", seat.Name, err)
	}
	edit, err := editdirective.Directive(geom.StencilsDir)
	if err != nil {
		return nil, fmt.Errorf("seatengine: seat %q: %w", seat.Name, err)
	}

	var advisors []string
	for _, other := range table.Seats {
		if other.Name != RoleChair {
			advisors = append(advisors, names[other.Name])
		}
	}
	failedAdvisors := failed
	if seat.Name != RoleChair {
		failedAdvisors = nil
	}

	values[parentdirective.MarkerName] = parent
	values[editdirective.MarkerName] = edit
	values["seat_name"] = names[seat.Name]
	values["chair_name"] = names[RoleChair]
	values["advisor_names"] = joinLines(advisors)
	values["failed_advisors"] = joinLines(failedAdvisors)
	values["inputs"] = joinLines(seat.Inputs)
	values["outputs"] = joinLines(seat.Outputs)
	values["output_files"] = joinLines(seat.Outputs)
	values["worktree_root"] = geom.WorktreeRoot
	values["anchor_path"] = geom.AnchorPath
	values["stencils_dir"] = geom.StencilsDir

	for _, name := range reservedMarkers {
		if _, filled := values[name]; !filled {
			return nil, fmt.Errorf("seatengine: seat %q: reserved marker %q has no value", seat.Name, name)
		}
	}
	return values, nil
}

// Prompts returns the prompt every seat of table reads at a fresh start, keyed by seat name, and starts nothing.
// It validates table against geom.StencilsDir, so a table that would be refused at start is refused here.
// No advisor is counted as failed, so a chair's failed-advisors list reads as empty.
func Prompts(geom Geometry, table Table) (map[string]string, error) {
	if err := table.Validate(geom.StencilsDir); err != nil {
		return nil, err
	}
	names, err := strandNames(geom, table)
	if err != nil {
		return nil, err
	}
	prompts := make(map[string]string, len(table.Seats))
	for _, seat := range table.Seats {
		values, err := seatValues(geom, table, seat, names, nil)
		if err != nil {
			return nil, err
		}
		prompt, err := composeSeatPrompt(geom.StencilsDir, seat, values, table.Optional)
		if err != nil {
			return nil, err
		}
		prompts[seat.Name] = prompt
	}
	return prompts, nil
}

// composePrompt reads seat's stencil and every block it includes from stencilsDir and fills them from values, with no marker optional.
func composePrompt(stencilsDir string, seat Seat, values map[string]string) (string, error) {
	return composeSeatPrompt(stencilsDir, seat, values, nil)
}

// composeSeatPrompt reads seat's stencil and every block it includes from stencilsDir and fills them from values.
// A marker named in optional renders as nothing when its value is empty, in the stencil and in every block.
func composeSeatPrompt(stencilsDir string, seat Seat, values map[string]string, optional []string) (string, error) {
	body, err := stencilstore.Read(stencilsDir, seat.Stencil)
	if err != nil {
		return "", fmt.Errorf("seatengine: seat %q: %w", seat.Name, err)
	}
	blockNames, err := stencil.IncludeNames(body)
	if err != nil {
		return "", fmt.Errorf("seatengine: seat %q: stencil %q: %w", seat.Name, seat.Stencil, err)
	}
	blocks := make(map[string][]byte, len(blockNames))
	for _, name := range blockNames {
		block, err := stencilstore.Read(stencilsDir, name)
		if err != nil {
			return "", fmt.Errorf("seatengine: seat %q: %w", seat.Name, err)
		}
		blocks[name] = block
	}
	filled, err := stencil.FillWith(body, blocks, values, optional)
	if err != nil {
		return "", fmt.Errorf("seatengine: seat %q: fill stencil %q: %w", seat.Name, seat.Stencil, err)
	}
	return string(filled), nil
}

// seatSpec returns the shuttle spec that runs seat with prompt.
// Only the chair is interactive; only an advisor keeps its pane and holds its turn end quietly.
// The chair's gate rides the start call, not the spec.
func seatSpec(table Table, seat Seat, prompt string) shuttleengine.Spec {
	isChair := seat.Name == RoleChair
	return shuttleengine.Spec{
		Prompt:      prompt,
		OutputFiles: slices.Clone(seat.Outputs),
		Model:       seat.Model,
		Effort:      seat.Effort,
		Version:     seat.Version,
		Interactive: isChair && table.Interactive,
		QuietHold:   !isChair,
		KeepPane:    !isChair,
		Segment:     table.Segment,
		Role:        SeatRole(table.RolePrefix, seat.Name),
		Timeout:     table.Timeout,
		Skills:      slices.Clone(seat.Skills),
	}
}
