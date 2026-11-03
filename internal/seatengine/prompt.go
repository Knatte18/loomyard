// prompt.go composes each seat's stencil value map, its filled prompt and its shuttle spec.

package seatengine

import (
	"errors"
	"fmt"
	"maps"
	"slices"
	"strings"

	"github.com/Knatte18/loomyard/internal/agentname"
	"github.com/Knatte18/loomyard/internal/editdirective"
	"github.com/Knatte18/loomyard/internal/parentdirective"
	"github.com/Knatte18/loomyard/internal/reedengine/render"
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
			return nil, fmt.Errorf("seatengine: seat %q: %w", table.seatLabel(seat), err)
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
// names is strandNames' answer.
// A chair's parent directive names the run's parent; an advisor's names the chair and is never interactive; an assistant's is the no-parent variant, since it sends no message.
// An assistant's edit directive is its engine's own copy, in that engine's tool wording; every other seat's is the shared one.
func seatValues(geom Geometry, table Table, seat Seat, names map[string]string) (map[string]string, error) {
	values := make(map[string]string, len(table.Values)+len(seat.Values)+len(reservedMarkers))
	maps.Copy(values, table.Values)
	maps.Copy(values, seat.Values)

	parentName, interactive := names[leadName], false
	switch {
	case seat.Name == leadName:
		parentName, interactive = geom.ParentName, table.Interactive
	case seat.Assistant:
		parentName = ""
	}
	label := table.seatLabel(seat)
	parent, err := parentdirective.Directive(geom.StencilsDir, parentName, interactive)
	if err != nil {
		return nil, fmt.Errorf("seatengine: seat %q: %w", label, err)
	}
	var edit string
	if seat.Assistant {
		edit, err = editdirective.EngineDirective(geom.StencilsDir, seat.stencilEngine(geom.DefaultEngine))
	} else {
		edit, err = editdirective.Directive(geom.StencilsDir)
	}
	if err != nil {
		return nil, fmt.Errorf("seatengine: seat %q: %w", label, err)
	}

	var advisors []string
	for _, other := range table.Seats {
		if other.Name != leadName {
			advisors = append(advisors, names[other.Name])
		}
	}

	values[parentdirective.MarkerName] = parent
	values[editdirective.MarkerName] = edit
	values["seat_name"] = names[seat.Name]
	values["chair_name"] = names[leadName]
	values["advisor_names"] = joinLines(advisors)
	values["assistants"] = assistantList(geom, table, names)
	values["inputs"] = joinLines(seat.Inputs)
	values["outputs"] = joinLines(seat.Outputs)
	values["output_files"] = joinLines(seat.Outputs)
	values["worktree_root"] = geom.WorktreeRoot
	values["anchor_path"] = geom.AnchorPath
	values["stencils_dir"] = geom.StencilsDir

	for _, name := range reservedMarkers {
		if _, filled := values[name]; !filled {
			return nil, fmt.Errorf("seatengine: seat %q: reserved marker %q has no value", label, name)
		}
	}
	return values, nil
}

// Prompts returns the prompt every seat of table reads at a fresh start, keyed by seat name, and starts nothing.
// The lead's prompt ends with the table's Amendment.
// It validates table against geom.StencilsDir and geom.DefaultEngine, so a table that would be refused at start is refused here.
func Prompts(geom Geometry, table Table) (map[string]string, error) {
	if err := table.Validate(geom.StencilsDir, geom.DefaultEngine); err != nil {
		return nil, err
	}
	names, err := strandNames(geom, table)
	if err != nil {
		return nil, err
	}
	prompts := make(map[string]string, len(table.Seats))
	for _, seat := range table.Seats {
		values, err := seatValues(geom, table, seat, names)
		if err != nil {
			return nil, err
		}
		prompt, err := composeSeatPrompt(geom, table, seat, values)
		if err != nil {
			return nil, err
		}
		prompts[seat.Name] = amendLeadPrompt(table, seat, prompt)
	}
	return prompts, nil
}

// CheckMarkers checks that a value fills every top-level marker of every seat's stencil and of every block it includes, read from stencilsDir, an engine block read for the seat's engine.
// defaultEngine is the engine a seat whose model choice names none runs on.
// A non-blank table or seat value, a name in Optional or a marker the engine fills itself fills a marker, so a table that passes composes no prompt with an unfilled marker.
// The first seat with an unfilled marker is refused naming its strand role, its stencil and every unfilled marker.
// Such a marker means either the stencils are newer than the running binary, as after a version skew where a newer binary refreshed them, or a value is blank.
// The refusal names both causes and a way forward for each: a binary that fills the marker, or text for the blank value.
// It applies none of Validate's rules, and starts nothing.
// An assistant whose engine lacks an engine block its stencil includes is passed over, since the missing block keeps it from starting instead.
func (t Table) CheckMarkers(stencilsDir, defaultEngine string) error {
	for _, seat := range t.Seats {
		label := t.seatLabel(seat)
		body, blocks, err := readStencilSet(stencilsDir, seat.stencilEngine(defaultEngine), label, seat.Stencil)
		var missing *stencilstore.MissingEngineBlockError
		if seat.Assistant && errors.As(err, &missing) {
			continue
		}
		if err != nil {
			return err
		}
		var unfilled []string
		for _, source := range append([][]byte{body}, slices.Collect(maps.Values(blocks))...) {
			markers, err := stencil.TopLevelMarkers(source)
			if err != nil {
				return fmt.Errorf("seatengine: seat %q: stencil %q: %w", label, seat.Stencil, err)
			}
			for _, marker := range markers {
				if !t.fills(seat, marker) && !slices.Contains(unfilled, marker) {
					unfilled = append(unfilled, marker)
				}
			}
		}
		if len(unfilled) > 0 {
			slices.Sort(unfilled)
			return fmt.Errorf("seatengine: seat %q: stencil %q has marker(s) %s that no value fills, because the stencils in %s are newer than this lym binary or a value is blank; way forward: re-step with a lym binary that fills them, or give the blank value text", label, seat.Stencil, strings.Join(unfilled, ", "), stencilsDir)
		}
	}
	return nil
}

// fills reports whether a value fills marker in seat's stencil: Optional names it, the engine fills it itself, or the seat's value, else the table's, is not blank.
func (t Table) fills(seat Seat, marker string) bool {
	if slices.Contains(t.Optional, marker) || slices.Contains(reservedMarkers, marker) {
		return true
	}
	value, inSeat := seat.Values[marker]
	if !inSeat {
		value = t.Values[marker]
	}
	return strings.TrimSpace(value) != ""
}

// amendLeadPrompt returns prompt followed by the table's Amendment when seat is the lead, and prompt unchanged for any other seat.
func amendLeadPrompt(table Table, seat Seat, prompt string) string {
	if seat.Name != leadName {
		return prompt
	}
	return prompt + table.Amendment
}

// composeSeatPrompt reads seat's stencil and every block it includes from geom.StencilsDir, an engine block for the seat's engine, and fills them from values.
// A marker named in table's Optional renders as nothing when its value is empty, in the stencil and in every block.
func composeSeatPrompt(geom Geometry, table Table, seat Seat, values map[string]string) (string, error) {
	label := table.seatLabel(seat)
	body, blocks, err := readStencilSet(geom.StencilsDir, seat.stencilEngine(geom.DefaultEngine), label, seat.Stencil)
	if err != nil {
		return "", err
	}
	filled, err := stencil.FillWith(body, blocks, values, table.Optional)
	if err != nil {
		return "", fmt.Errorf("seatengine: seat %q: fill stencil %q: %w", label, seat.Stencil, err)
	}
	return string(filled), nil
}

// seatSpec returns the shuttle spec that runs seat with prompt.
// Only the chair is interactive; every seat after the lead keeps its pane and holds its turn end quietly unless it is a required advisor.
// A seat's own Timeout, when non-zero, bounds its run in place of the table's.
// Every seat of a table declares the own-window anchor with the table's window group and seat count.
// The seat's gate rides the start call, not the spec.
func seatSpec(table Table, seat Seat, prompt string) shuttleengine.Spec {
	isChair := seat.Name == leadName
	timeout := table.Timeout
	if seat.Timeout != 0 {
		timeout = seat.Timeout
	}
	return shuttleengine.Spec{
		Prompt:        prompt,
		OutputFiles:   slices.Clone(seat.Outputs),
		ModelChoice:   seat.ModelChoice,
		ForkSubagents: seat.ForkSubagents,
		Requires:      slices.Clone(seat.Requires),
		Interactive:   isChair && table.Interactive,
		QuietHold:     !isChair && !seat.Required,
		KeepPane:      !isChair,
		Segment:       table.Segment,
		Role:          SeatRole(table.RolePrefix, seat.Name),
		Round:         table.Round,
		Timeout:       timeout,
		Skills:        slices.Clone(seat.Skills),
		Display:       render.Display{Anchor: render.AnchorOwnWindow, Window: tableWindow(table), WindowSize: len(table.Seats)},
	}
}

// tableWindow returns the window group the seats of table's instance declare: the role prefix, followed by `-` and the round when the round is set, so two rounds of one table never name the same group.
// A table of one seat returns "", which gives its seat a window to itself named by its role.
func tableWindow(table Table) string {
	if len(table.Seats) == 1 {
		return ""
	}
	if table.Round == "" {
		return table.RolePrefix
	}
	return table.RolePrefix + "-" + table.Round
}

// seatGate returns the gate seat runs under: the table's for the chair, the seat's own for an advisor.
func seatGate(table Table, seat Seat) shuttleengine.GateSpec {
	if seat.Name == leadName {
		return table.Gate
	}
	return seat.Gate
}
