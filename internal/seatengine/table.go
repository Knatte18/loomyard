// table.go declares the seat table a MultiLLM step runs (Table, Seat), the names its seats take, the reserved marker list, and the table's validation.

package seatengine

import (
	"fmt"
	"maps"
	"slices"
	"strings"
	"time"

	"github.com/Knatte18/loomyard/internal/agentname"
	"github.com/Knatte18/loomyard/internal/segmentcolor"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
	"github.com/Knatte18/loomyard/internal/stencil"
	"github.com/Knatte18/loomyard/internal/stencilstore"
)

const (
	// RoleChair is the chair seat's name, and the role segment its strand's name carries after the table's prefix.
	RoleChair = "chair"
	// RoleAdvisor is the stem of every advisor seat's name.
	RoleAdvisor = "advisor"
)

// reservedMarkers are the marker names the seat engine fills itself, sorted.
// A table or seat value may not use one of them.
var reservedMarkers = []string{
	"advisor_names",
	"anchor_path",
	"chair_name",
	"edit_directive",
	"failed_advisors",
	"inputs",
	"output_files",
	"outputs",
	"parent_directive",
	"seat_name",
	"stencils_dir",
	"worktree_root",
}

// ReservedMarkers returns the marker names the seat engine fills itself, sorted.
func ReservedMarkers() []string {
	return slices.Clone(reservedMarkers)
}

// AdvisorName returns the seat name of the n-th advisor, counting from 1.
func AdvisorName(n int) string {
	return fmt.Sprintf("%s-%d", RoleAdvisor, n)
}

// SeatRole returns the role segment of the strand name of the seat called seat, under the table's role prefix.
func SeatRole(prefix, seat string) string {
	return prefix + "-" + seat
}

// Seat is one agent of a step: its name, the stencil that opens it, its model choice, the files it reads and writes, its skills and its own marker values.
type Seat struct {
	// Name is RoleChair for the chair and AdvisorName(n) for an advisor.
	Name string
	// Stencil is the name of the stencil, read from the told stencils directory, that opens the seat.
	Stencil string
	// Model, Effort and Version select the seat's model; each is empty to defer to the provider default.
	Model   string
	Effort  string
	Version string
	// Inputs are the files the seat reads, and Outputs the files it writes; a seat has at least one output.
	Inputs  []string
	Outputs []string
	// Skills are the skills the seat's session is typed.
	Skills []string
	// Values are the seat's own stencil marker values; a name may not be reserved and a value may not be blank.
	Values map[string]string
}

// Table is the ordered set of seats one step runs, with the settings they share.
type Table struct {
	// RolePrefix is the leading part of every seat's strand role, formed with SeatRole.
	RolePrefix string
	// Segment is the loom segment the seats are colored by.
	Segment segmentcolor.Segment
	// Gate is the chair's gate; advisors run ungated.
	Gate shuttleengine.GateSpec
	// Timeout bounds every seat's run.
	Timeout time.Duration
	// Interactive makes the chair's session interactive; advisors never are.
	Interactive bool
	// Values are stencil marker values every seat shares; a name may not be reserved and a value may not be blank.
	Values map[string]string
	// Optional names the table values that may be empty; such a value renders as nothing wherever its marker stands.
	Optional []string
	// Seats are the table's seats in order: the chair once, and advisors named AdvisorName(1), AdvisorName(2) and so on.
	Seats []Seat
}

// Chair returns the seat named RoleChair, or the zero Seat when the table has none.
func (t Table) Chair() Seat {
	for _, seat := range t.Seats {
		if seat.Name == RoleChair {
			return seat
		}
	}
	return Seat{}
}

// Outputs returns every seat's outputs, in table order.
func (t Table) Outputs() []string {
	var outputs []string
	for _, seat := range t.Seats {
		outputs = append(outputs, seat.Outputs...)
	}
	return outputs
}

// Validate checks the table and returns the first violated rule, naming the seat it concerns.
// The table has exactly one chair, and its other seats are named AdvisorName(1), AdvisorName(2) and so on in table order.
// Every seat has an output, no two seats share an output path, and every advisor output is one of the chair's inputs.
// The role prefix and every seat's formed role are valid agent-name roles.
// Every seat's stencil is readable from stencilsDir, as is every block it includes, and an included block includes nothing.
// A table or seat value neither collides with a reserved marker nor is blank, except a table value named in Optional, which may be blank; an Optional name may not be a reserved marker.
func (t Table) Validate(stencilsDir string) error {
	if err := agentname.ValidateRole(t.RolePrefix); err != nil {
		return fmt.Errorf("seatengine: role prefix: %w", err)
	}
	for _, name := range t.Optional {
		if slices.Contains(reservedMarkers, name) {
			return fmt.Errorf("seatengine: table: optional value %q collides with a reserved marker", name)
		}
	}
	if err := validateValues("table", t.requiredValues()); err != nil {
		return err
	}
	for _, check := range []func() error{t.validateNames, t.validateOutputs, t.validateRoles} {
		if err := check(); err != nil {
			return err
		}
	}
	for _, seat := range t.Seats {
		if err := validateValues(fmt.Sprintf("seat %q", seat.Name), seat.Values); err != nil {
			return err
		}
		if err := validateStencil(stencilsDir, seat); err != nil {
			return err
		}
	}
	return nil
}

// requiredValues returns the table's values without the blank ones named in Optional.
func (t Table) requiredValues() map[string]string {
	required := maps.Clone(t.Values)
	for _, name := range t.Optional {
		if strings.TrimSpace(required[name]) == "" {
			delete(required, name)
		}
	}
	return required
}

// validateNames checks the table has one chair and its other seats are named advisor-1, advisor-2 and so on in order.
func (t Table) validateNames() error {
	chairs := 0
	for _, seat := range t.Seats {
		if seat.Name == RoleChair {
			chairs++
		}
	}
	if chairs != 1 {
		return fmt.Errorf("seatengine: want exactly one seat named %q, found %d", RoleChair, chairs)
	}
	advisors := 0
	for _, seat := range t.Seats {
		if seat.Name == RoleChair {
			continue
		}
		advisors++
		if want := AdvisorName(advisors); seat.Name != want {
			return fmt.Errorf("seatengine: seat %q: want the name %q, since advisors are named in table order from 1", seat.Name, want)
		}
	}
	return nil
}

// validateOutputs checks every seat has an output, no output path is shared, and every advisor output is a chair input.
func (t Table) validateOutputs() error {
	chairInputs := t.Chair().Inputs
	owners := make(map[string]string)
	for _, seat := range t.Seats {
		if len(seat.Outputs) == 0 {
			return fmt.Errorf("seatengine: seat %q: has no output", seat.Name)
		}
		for _, output := range seat.Outputs {
			if owner, shared := owners[output]; shared {
				return fmt.Errorf("seatengine: seat %q: output %q is also seat %q's output", seat.Name, output, owner)
			}
			owners[output] = seat.Name
			if seat.Name != RoleChair && !slices.Contains(chairInputs, output) {
				return fmt.Errorf("seatengine: seat %q: output %q is not among the chair's inputs", seat.Name, output)
			}
		}
	}
	return nil
}

// validateRoles checks every seat's formed strand role passes the agent-name role grammar.
func (t Table) validateRoles() error {
	for _, seat := range t.Seats {
		if err := agentname.ValidateRole(SeatRole(t.RolePrefix, seat.Name)); err != nil {
			return fmt.Errorf("seatengine: seat %q: %w", seat.Name, err)
		}
	}
	return nil
}

// validateValues checks no name in values is a reserved marker and no value is empty or whitespace-only.
// owner names the table or seat the values belong to.
func validateValues(owner string, values map[string]string) error {
	for _, name := range slices.Sorted(maps.Keys(values)) {
		if slices.Contains(reservedMarkers, name) {
			return fmt.Errorf("seatengine: %s: value %q collides with a reserved marker", owner, name)
		}
		if strings.TrimSpace(values[name]) == "" {
			return fmt.Errorf("seatengine: %s: value %q is empty", owner, name)
		}
	}
	return nil
}

// validateStencil checks seat's stencil and every block it includes are readable from stencilsDir, and that no block includes another.
func validateStencil(stencilsDir string, seat Seat) error {
	body, err := stencilstore.Read(stencilsDir, seat.Stencil)
	if err != nil {
		return fmt.Errorf("seatengine: seat %q: %w", seat.Name, err)
	}
	blocks, err := stencil.IncludeNames(body)
	if err != nil {
		return fmt.Errorf("seatengine: seat %q: stencil %q: %w", seat.Name, seat.Stencil, err)
	}
	for _, block := range blocks {
		blockBody, err := stencilstore.Read(stencilsDir, block)
		if err != nil {
			return fmt.Errorf("seatengine: seat %q: %w", seat.Name, err)
		}
		nested, err := stencil.IncludeNames(blockBody)
		if err != nil {
			return fmt.Errorf("seatengine: seat %q: include %q: %w", seat.Name, block, err)
		}
		if len(nested) > 0 {
			return fmt.Errorf("seatengine: seat %q: include %q declares an include of its own; includes resolve one level deep", seat.Name, block)
		}
	}
	return nil
}
