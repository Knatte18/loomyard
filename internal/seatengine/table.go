// table.go declares the seat table a MultiLLM step runs (Table, Seat), the names its seats take, the reserved marker list, and the table's validation.

package seatengine

import (
	"errors"
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

// leadName is the name of the lead, the table's first seat.
const leadName = ""

// chairDirectiveBlock is the shipped block that tells the lead how to work with its advisors, which the lead's stencil of an advisor table of more than one seat includes.
const chairDirectiveBlock = "seat-directive-chair"

// writerChairDirectiveBlock is the shipped block that tells the lead how to work with its writers, which the lead's stencil of a writer table of more than one seat includes in place of chairDirectiveBlock.
const writerChairDirectiveBlock = "seat-directive-writer-chair"

// writerDirectiveBlock is the shipped block that tells a writer how to work with the chair, which every writer seat's stencil includes.
const writerDirectiveBlock = "seat-directive-writer"

// assistantChairDirectiveBlock is the shipped block that tells the lead how to wait for its assistants, which the lead's stencil of an assistant table includes in place of chairDirectiveBlock.
const assistantChairDirectiveBlock = "seat-directive-assistant-chair"

// assistantDirectiveBlock is the shipped block that tells an assistant it writes its outputs and nothing else, which the stencil of an assistant with no subagent reports includes.
const assistantDirectiveBlock = "seat-directive-assistant"

// fannedAssistantDirectiveBlock is the shipped block that tells an assistant running subagents that they are its own work and which files it writes, which the stencil of an assistant with subagent reports includes in place of assistantDirectiveBlock.
const fannedAssistantDirectiveBlock = "seat-directive-assistant-fanned"

// reservedMarkers are the marker names the seat engine fills itself, sorted.
// A table or seat value may not use one of them.
var reservedMarkers = []string{
	"advisor_names",
	"anchor_path",
	"assistants",
	"chair_name",
	"edit_directive",
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

// SeatRole returns the role segment of the strand name of the seat called seat, under the table's role prefix.
// The lead has no name, so its role is the prefix alone.
func SeatRole(prefix, seat string) string {
	if seat == "" {
		return prefix
	}
	return prefix + "-" + seat
}

// ValidateSeatName checks the name of a seat after the lead: non-empty and a valid agent-name role, so it starts with a letter and the role formed from it never reads as reed's numbered form of the lead's role.
func ValidateSeatName(name string) error {
	if name == "" {
		return fmt.Errorf("the name is empty")
	}
	return agentname.ValidateRole(name)
}

// Seat is one agent of a step: its name, the stencil that opens it, its model choice, the files it reads and writes, its skills and its own marker values.
type Seat struct {
	// Name is empty for the lead, the table's first seat, and unique and valid under ValidateSeatName for every later seat.
	Name string
	// Stencil is the name of the stencil, read from the told stencils directory, that opens the seat.
	Stencil string
	// ModelChoice selects the seat's engine and model; each field is empty to defer to the engine set's default or the provider default.
	shuttleengine.ModelChoice
	// Inputs are the files the seat reads, and Outputs the files it writes; a seat has at least one output.
	Inputs  []string
	Outputs []string
	// Skills are the skills the seat's session is typed.
	Skills []string
	// Values are the seat's own stencil marker values; a name may not be reserved and a value may not be blank.
	Values map[string]string
	// Gate is an advisor's own gate, empty for an ungated advisor; the lead's gate is the table's, so the lead sets none.
	Gate shuttleengine.GateSpec
	// ForkSubagents authorizes the seat's session to spawn in-session fork subagents; the engine audits none.
	ForkSubagents bool
	// Requires lists what the seat requires of its engine beyond forks, which ForkSubagents implies.
	Requires []shuttleengine.Requirement
	// ModelKey names the config key the seat's model came from, so a Start refusal for an unmet requirement names the key to change; the zero value stands for a seat whose key no caller names.
	ModelKey shuttleengine.ModelKey
	// Required marks an advisor the step depends on: any end of its run that Accept does not accept ends the step, and its held turn end still notifies the parent.
	// The lead is never Required.
	Required bool
	// Writer marks a writing assistant: a seat that writes a product of its own, which the chair joins.
	// A writer is required by kind, so every end of its run that its gate and Accept do not pass ends the step, but its held turn end does not notify the parent.
	// The lead is never a writer, and a table holds advisors only or writers only.
	Writer bool
	// Assistant marks an assistant: a seat that writes its outputs and nothing else, gets no up line and no mail, and never ends the step.
	// The engine writes its EndMarker once its run ends, whatever the ending, or when it does not start.
	// The lead is never an assistant, and a table holds assistants only after the lead when it holds one.
	Assistant bool
	// EndMarker is an assistant's end-marker path, whose text names how its run ended; only an assistant carries one.
	EndMarker string
	// SubagentReports are the files an assistant saves its subagents' reports in, one per subagent; empty for an assistant that runs none.
	// They are on its write surface but are no outputs, so a run that leaves one unwritten still ends done: the caller judges what a missing report means.
	SubagentReports []string
	// SkipReason, when set on an assistant, keeps it from starting: its end marker then reads not started with this reason.
	SkipReason string
	// Timeout bounds this seat's run; zero takes the table's Timeout.
	Timeout time.Duration
	// GateFindingsPath is where the producer keeps the findings of a writer's failed gate; the engine only carries it onto the seat's result.
	GateFindingsPath string
	// Accept judges the terminal result of a seat that ends the step, a Required advisor or a writer, once it ended done with its gate passed; an error ends the step.
	// Only such a seat carries one.
	Accept func(shuttleengine.Result) error
}

// stencilEngine returns the engine the seat's engine blocks are read for: the engine its model choice names, else defaultEngine.
func (s Seat) stencilEngine(defaultEngine string) string {
	if s.Engine != "" {
		return s.Engine
	}
	return defaultEngine
}

// assistantDirective returns the directive block this assistant's stencil includes: the fanned assistant's when it has subagent reports.
func (s Seat) assistantDirective() string {
	if len(s.SubagentReports) > 0 {
		return fannedAssistantDirectiveBlock
	}
	return assistantDirectiveBlock
}

// endsStep reports whether an end of this seat's run that its gate and Accept do not pass ends the step: a required advisor or a writer.
func (s Seat) endsStep() bool {
	return s.Required || s.Writer
}

// Table is the ordered set of seats one step runs, with the settings they share.
type Table struct {
	// RolePrefix is the leading part of every seat's strand role, formed with SeatRole.
	RolePrefix string
	// Segment is the loom segment the seats are colored by.
	Segment segmentcolor.Segment
	// Gate is the chair's gate; an advisor's gate is its seat's own.
	Gate shuttleengine.GateSpec
	// Round names every seat's run directory; empty keeps the default naming.
	Round string
	// Timeout bounds every seat's run, except a seat that sets its own Timeout, which overrides it.
	Timeout time.Duration
	// Interactive makes the chair's session interactive; advisors never are.
	Interactive bool
	// Values are stencil marker values every seat shares; a name may not be reserved and a value may not be blank.
	Values map[string]string
	// Optional names the table values that may be empty; such a value renders as nothing wherever its marker stands.
	Optional []string
	// Amendment is text appended verbatim to the lead's composed prompt when the table starts fresh; empty leaves the prompt unchanged.
	// A later seat's prompt never carries it, and a resumed lead keeps the prompt it started with.
	Amendment string
	// Seats are the table's seats in order: the lead, which carries no name, then the named advisors or the named writers.
	Seats []Seat
}

// hasWriters reports whether any seat after the lead is a writer.
func (t Table) hasWriters() bool {
	if len(t.Seats) == 0 {
		return false
	}
	return slices.ContainsFunc(t.Seats[1:], func(seat Seat) bool { return seat.Writer })
}

// hasAssistants reports whether any seat after the lead is an assistant.
func (t Table) hasAssistants() bool {
	if len(t.Seats) == 0 {
		return false
	}
	return slices.ContainsFunc(t.Seats[1:], func(seat Seat) bool { return seat.Assistant })
}

// chairDirective returns the directive block the lead's stencil of a table of more than one seat includes: the assistant chair's, the writer chair's or the advisor chair's.
func (t Table) chairDirective() string {
	switch {
	case t.hasAssistants():
		return assistantChairDirectiveBlock
	case t.hasWriters():
		return writerChairDirectiveBlock
	}
	return chairDirectiveBlock
}

// Chair returns the lead, the table's first seat, or the zero Seat when the table has none.
func (t Table) Chair() Seat {
	if len(t.Seats) == 0 {
		return Seat{}
	}
	return t.Seats[0]
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
// The table has at least one seat; the first is the lead and carries no name, and every later seat has a name valid under ValidateSeatName and unique in the table.
// Every seat has an output, no two seats share an output path, and every advisor output is one of the lead's inputs.
// The role prefix and every seat's formed role are valid agent-name roles.
// The lead carries no gate, since the lead's gate is the table's, and is neither Required, a Writer nor given an Accept hook; Accept belongs to a seat that ends the step, a Required advisor or a Writer.
// A table holds advisors only, writers only or assistants only after the lead.
// An assistant carries an end marker no other seat's marker or output shares, and is neither Required, a Writer nor given an Accept hook.
// Every seat's stencil is readable from stencilsDir, as is every block it includes, an engine block read for the seat's engine, and an included block includes nothing.
// defaultEngine is the engine a seat whose model choice names none runs on; an engine block its seat's engine has no copy of is refused naming the block, the engine and the file.
// An assistant is the exception: the missing block keeps it from starting instead, so the step goes on without it.
// The lead's stencil of an advisor table of more than one seat includes seat-directive-chair, the lead's stencil of a writer table includes seat-directive-writer-chair in its place, and the lead's stencil of an assistant table seat-directive-assistant-chair.
// A writer seat's stencil includes seat-directive-writer, and an assistant seat's seat-directive-assistant, or seat-directive-assistant-fanned when the seat has subagent reports.
// A table or seat value neither collides with a reserved marker nor is blank, except a table value named in Optional, which may be blank; an Optional name may not be a reserved marker.
func (t Table) Validate(stencilsDir, defaultEngine string) error {
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
	for _, check := range []func() error{t.validateNames, t.validateOutputs, t.validateRoles, t.validateKinds, t.validateEndMarkers} {
		if err := check(); err != nil {
			return err
		}
	}
	for index, seat := range t.Seats {
		label := t.seatLabel(seat)
		if err := validateSeatRoles(label, seat); err != nil {
			return err
		}
		if err := validateValues(fmt.Sprintf("seat %q", label), seat.Values); err != nil {
			return err
		}
		blocks, err := validateStencil(stencilsDir, seat.stencilEngine(defaultEngine), label, seat.Stencil)
		var missing *stencilstore.MissingEngineBlockError
		if seat.Assistant && errors.As(err, &missing) {
			continue
		}
		if err != nil {
			return err
		}
		if index == 0 && len(t.Seats) > 1 {
			directive := t.chairDirective()
			if !slices.Contains(blocks, directive) {
				return fmt.Errorf("seatengine: the lead's stencil %q does not include %s, which a table of more than one seat needs; include %s in the lead's stencil", seat.Stencil, directive, directive)
			}
		}
		if seat.Writer && !slices.Contains(blocks, writerDirectiveBlock) {
			return fmt.Errorf("seatengine: seat %q: the stencil %q does not include %s, which a writer needs; include %s in the writer's stencil", label, seat.Stencil, writerDirectiveBlock, writerDirectiveBlock)
		}
		if directive := seat.assistantDirective(); seat.Assistant && !slices.Contains(blocks, directive) {
			return fmt.Errorf("seatengine: seat %q: the stencil %q does not include %s, which this assistant needs; include %s in the assistant's stencil", label, seat.Stencil, directive, directive)
		}
	}
	return nil
}

// seatLabel returns the name an error gives seat: its strand role, so the lead, which carries no name, reads as the role prefix.
func (t Table) seatLabel(seat Seat) string {
	return SeatRole(t.RolePrefix, seat.Name)
}

// validateSeatRoles checks the fields only some seats may carry: no gate, Required, Writer or Accept on the lead, and Accept only on a seat that ends the step.
// label is the name an error gives the seat.
func validateSeatRoles(label string, seat Seat) error {
	if seat.Name == leadName {
		switch {
		case len(seat.Gate) > 0:
			return fmt.Errorf("seatengine: the lead carries a gate, but the lead's gate is the table's")
		case seat.Required:
			return fmt.Errorf("seatengine: the lead is marked required, but only an advisor can be")
		case seat.Writer:
			return fmt.Errorf("seatengine: the lead is marked a writer, but only a seat after the lead can be")
		case seat.Assistant:
			return fmt.Errorf("seatengine: the lead is marked an assistant, but only a seat after the lead can be")
		case seat.Accept != nil:
			return fmt.Errorf("seatengine: the lead carries an accept hook, but only a required advisor or a writer can")
		}
		return nil
	}
	if seat.Assistant && (seat.Required || seat.Writer) {
		return fmt.Errorf("seatengine: seat %q: an assistant is never required and never a writer; drop the other mark", label)
	}
	if seat.Accept != nil && !seat.endsStep() {
		return fmt.Errorf("seatengine: seat %q: carries an accept hook but is neither required nor a writer", label)
	}
	return nil
}

// validateKinds checks a table that holds a writer holds writers only after the lead, and one that holds an assistant assistants only.
func (t Table) validateKinds() error {
	kinds := []struct {
		name   string
		holds  bool
		member func(Seat) bool
	}{
		{"writers", t.hasWriters(), func(seat Seat) bool { return seat.Writer }},
		{"assistants", t.hasAssistants(), func(seat Seat) bool { return seat.Assistant }},
	}
	for _, kind := range kinds {
		if !kind.holds {
			continue
		}
		for _, seat := range t.Seats[1:] {
			if !kind.member(seat) {
				return fmt.Errorf("seatengine: seat %q: the table holds %s, but this seat is not one; a table holds advisors only, writers only or assistants only, so give the seat the table's kind or drop it", t.seatLabel(seat), kind.name)
			}
		}
	}
	return nil
}

// validateEndMarkers checks every assistant carries an end marker and every other seat none, and that no end marker is another seat's marker or any seat's output.
func (t Table) validateEndMarkers() error {
	owners := make(map[string]string)
	for _, path := range t.Outputs() {
		owners[path] = "an output"
	}
	for _, seat := range t.Seats {
		label := t.seatLabel(seat)
		switch {
		case seat.Assistant && seat.EndMarker == "":
			return fmt.Errorf("seatengine: seat %q: an assistant needs an end marker; set the seat's EndMarker", label)
		case !seat.Assistant && seat.EndMarker != "":
			return fmt.Errorf("seatengine: seat %q: carries an end marker, but only an assistant can", label)
		case seat.EndMarker == "":
			continue
		}
		if owner, taken := owners[seat.EndMarker]; taken {
			return fmt.Errorf("seatengine: seat %q: end marker %q is already %s", label, seat.EndMarker, owner)
		}
		owners[seat.EndMarker] = fmt.Sprintf("seat %q's end marker", label)
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

// validateNames checks the table has a seat, its first seat carries no name, and every later seat has a valid name no earlier seat has.
func (t Table) validateNames() error {
	if len(t.Seats) == 0 {
		return fmt.Errorf("seatengine: the table has no seat; give it a lead as its first seat")
	}
	if lead := t.Seats[0]; lead.Name != leadName {
		return fmt.Errorf("seatengine: seat %q: the first seat is the lead and carries no name; drop its name", lead.Name)
	}
	seen := make(map[string]bool, len(t.Seats))
	for _, seat := range t.Seats[1:] {
		if err := ValidateSeatName(seat.Name); err != nil {
			return fmt.Errorf("seatengine: seat %q: %w; give the seat a unique name that starts with a letter", seat.Name, err)
		}
		if seen[seat.Name] {
			return fmt.Errorf("seatengine: seat %q: the name is already taken; give the seat a unique name that starts with a letter", seat.Name)
		}
		seen[seat.Name] = true
	}
	return nil
}

// validateOutputs checks every seat has an output, no output path is shared, and every advisor output is a lead input.
func (t Table) validateOutputs() error {
	leadInputs := t.Chair().Inputs
	owners := make(map[string]string)
	for _, seat := range t.Seats {
		label := t.seatLabel(seat)
		if len(seat.Outputs) == 0 {
			return fmt.Errorf("seatengine: seat %q: has no output", label)
		}
		for _, output := range seat.Outputs {
			if owner, shared := owners[output]; shared {
				return fmt.Errorf("seatengine: seat %q: output %q is also seat %q's output", label, output, owner)
			}
			owners[output] = label
			if seat.Name != leadName && !slices.Contains(leadInputs, output) {
				return fmt.Errorf("seatengine: seat %q: output %q is not among the lead's inputs", label, output)
			}
		}
	}
	return nil
}

// validateRoles checks every seat's formed strand role passes the agent-name role grammar.
func (t Table) validateRoles() error {
	for _, seat := range t.Seats {
		label := t.seatLabel(seat)
		if err := agentname.ValidateRole(label); err != nil {
			return fmt.Errorf("seatengine: seat %q: %w", label, err)
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

// validateStencil checks the stencil named stencilName and every block it includes, an engine block read for engine, are readable from stencilsDir, and that no block includes another.
// It returns the sorted names of the blocks the stencil includes; label is the name an error gives the seat the stencil opens.
func validateStencil(stencilsDir, engine, label, stencilName string) ([]string, error) {
	_, blocks, err := readStencilSet(stencilsDir, engine, label, stencilName)
	if err != nil {
		return nil, err
	}
	names := slices.Sorted(maps.Keys(blocks))
	for _, name := range names {
		nested, err := stencil.IncludeNames(blocks[name])
		if err != nil {
			return nil, fmt.Errorf("seatengine: seat %q: include %q: %w", label, name, err)
		}
		if len(nested) > 0 {
			return nil, fmt.Errorf("seatengine: seat %q: include %q declares an include of its own; includes resolve one level deep", label, name)
		}
	}
	return names, nil
}

// readStencilSet reads the stencil named stencilName from stencilsDir, with every block it includes keyed by block name and an engine block read for engine.
// label is the name an error gives the seat the stencil opens.
func readStencilSet(stencilsDir, engine, label, stencilName string) ([]byte, map[string][]byte, error) {
	body, blocks, err := stencilstore.ReadSet(stencilsDir, engine, stencilName)
	if err != nil {
		return nil, nil, fmt.Errorf("seatengine: seat %q: %w", label, err)
	}
	return body, blocks, nil
}
