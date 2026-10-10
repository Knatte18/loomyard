// entries_multillm.go implements multiLLMEntry, the Constructor for the "MultiLLM" registry row:
// it builds the seatengine.Table of one chair and its advisors from the row's Config.

package shedrecipe

import (
	"fmt"
	"slices"
	"time"

	"github.com/Knatte18/loomyard/internal/modelspec"
	"github.com/Knatte18/loomyard/internal/seatengine"
	"github.com/Knatte18/loomyard/internal/segmentcolor"
	"github.com/Knatte18/loomyard/internal/shedadapters"
	"github.com/Knatte18/loomyard/internal/shedengine"
)

// multiLLMEntry is the Constructor for the "MultiLLM" registry row.
// It validates cfg and env, builds the seat table the row describes, checks it against the told stencils directory, and returns shedadapters.NewMultiLLMProducer(name, table, env.Seats, env.Now).
// Table.Validate runs here, so an unreadable stencil, an unresolvable include, a nested include and a reserved or blank value fail at construction rather than at the first Call.
func multiLLMEntry(name string, cfg Config, env Env) (shedengine.ShedProducer, error) {
	role, err := configString(cfg, "role", true)
	if err != nil {
		return nil, err
	}
	rawSegment, err := configString(cfg, "segment", true)
	if err != nil {
		return nil, err
	}
	seatConfigs, _, err := configMapList(cfg, "seats")
	if err != nil {
		return nil, err
	}
	if len(seatConfigs) == 0 {
		return nil, fmt.Errorf("shedrecipe: MultiLLM: config key %q is required", "seats")
	}
	timeoutMinutes, err := configInt(cfg, "timeout_min", false)
	if err != nil {
		return nil, err
	}
	interactive, err := configBool(cfg, "interactive", false)
	if err != nil {
		return nil, err
	}
	tokens, err := configStringMap(cfg, "tokens", false)
	if err != nil {
		return nil, err
	}
	if err := configRejectUnknown(cfg, "role", "segment", "seats", "gates", "timeout_min", "interactive", "tokens"); err != nil {
		return nil, err
	}
	if timeoutMinutes < 0 {
		return nil, fmt.Errorf("shedrecipe: MultiLLM: config key %q must not be negative, got %d", "timeout_min", timeoutMinutes)
	}

	segment, err := parseSegment("MultiLLM", rawSegment)
	if err != nil {
		return nil, err
	}

	// Every root is validated whether or not a stencil names its marker, so a stencil edit never changes a recipe row's validity.
	if err := requireAbsRoot("MultiLLM", "WorktreeRoot", env.WorktreeRoot); err != nil {
		return nil, err
	}
	if err := requireAbsRoot("MultiLLM", "AnchorPath", env.AnchorPath); err != nil {
		return nil, err
	}
	if err := requireAbsRoot("MultiLLM", "StencilsDir", env.StencilsDir); err != nil {
		return nil, err
	}
	if err := requireSeam("MultiLLM", "Seats", env.Seats); err != nil {
		return nil, err
	}

	gate, err := resolveGateSpec("MultiLLM", cfg, env)
	if err != nil {
		return nil, err
	}

	seats := make([]seatengine.Seat, len(seatConfigs))
	for i, seatConfig := range seatConfigs {
		seats[i], err = multiLLMSeat(i, seatConfig, env)
		if err != nil {
			return nil, err
		}
	}

	table := seatengine.Table{
		RolePrefix:  role,
		Segment:     segment,
		Gate:        gate,
		Timeout:     time.Duration(timeoutMinutes) * time.Minute,
		Interactive: interactive,
		Values:      tokens,
		Seats:       seats,
	}
	if err := table.Validate(env.StencilsDir); err != nil {
		return nil, fmt.Errorf("shedrecipe: MultiLLM: %w", err)
	}

	return shedadapters.NewMultiLLMProducer(name, table, env.Seats, env.Now), nil
}

// multiLLMSeat builds the i-th seat of a MultiLLM row from its "seats" element.
// Inputs and outputs are resolved under env.WorktreeRoot, and the model spec against env.Models.
func multiLLMSeat(i int, cfg Config, env Env) (seatengine.Seat, error) {
	wrap := func(err error) error {
		return fmt.Errorf("shedrecipe: MultiLLM: config key %q element %d: %w", "seats", i, err)
	}

	name, err := configString(cfg, "name", true)
	if err != nil {
		return seatengine.Seat{}, wrap(err)
	}
	stencilName, err := configString(cfg, "stencil", true)
	if err != nil {
		return seatengine.Seat{}, wrap(err)
	}
	modelSpec, err := configString(cfg, "model", true)
	if err != nil {
		return seatengine.Seat{}, wrap(err)
	}
	inputs, err := configStringSlice(cfg, "inputs", false)
	if err != nil {
		return seatengine.Seat{}, wrap(err)
	}
	outputs, err := configStringSlice(cfg, "outputs", true)
	if err != nil {
		return seatengine.Seat{}, wrap(err)
	}
	skills, err := configStringSlice(cfg, "skills", false)
	if err != nil {
		return seatengine.Seat{}, wrap(err)
	}
	values, err := configStringMap(cfg, "values", false)
	if err != nil {
		return seatengine.Seat{}, wrap(err)
	}
	if err := configRejectUnknown(cfg, "name", "stencil", "model", "inputs", "outputs", "skills", "values"); err != nil {
		return seatengine.Seat{}, wrap(err)
	}

	model, effort, version, err := resolveSeatModel("MultiLLM", i, modelSpec, env.Models)
	if err != nil {
		return seatengine.Seat{}, err
	}

	resolvedInputs, err := resolveSeatPaths(env.WorktreeRoot, "inputs", i, inputs)
	if err != nil {
		return seatengine.Seat{}, err
	}
	resolvedOutputs, err := resolveSeatPaths(env.WorktreeRoot, "outputs", i, outputs)
	if err != nil {
		return seatengine.Seat{}, err
	}

	return seatengine.Seat{
		Name:    name,
		Stencil: stencilName,
		Model:   model,
		Effort:  effort,
		Version: version,
		Inputs:  resolvedInputs,
		Outputs: resolvedOutputs,
		Skills:  skills,
		Values:  values,
	}, nil
}

// resolveSeatPaths resolves each path of a seat's key under root, naming the seat's index on failure.
func resolveSeatPaths(root, key string, i int, paths []string) ([]string, error) {
	var resolved []string
	for _, path := range paths {
		joined, err := resolveUnderRoot(fmt.Sprintf("MultiLLM seat %d", i), key, root, path)
		if err != nil {
			return nil, err
		}
		resolved = append(resolved, joined)
	}
	return resolved, nil
}

// resolveSeatModel parses the model-spec string of the i-th seat of entry and resolves it against reg.
// It returns the provider model and the spec's effort and version parameters.
// An unparsable spec and an unknown alias are errors naming the seat.
func resolveSeatModel(entry string, i int, spec string, reg modelspec.Registry) (model, effort, version string, err error) {
	parsed, err := modelspec.Parse(spec)
	if err != nil {
		return "", "", "", fmt.Errorf("shedrecipe: %s: seat %d: model %q: %w", entry, i, spec, err)
	}
	resolved, err := reg.Resolve(parsed)
	if err != nil {
		return "", "", "", fmt.Errorf("shedrecipe: %s: seat %d: model %q: %w", entry, i, spec, err)
	}
	return resolved.Model, resolved.Params["effort"], resolved.Params["version"], nil
}

// parseSegment returns raw as a loom segment, or an error naming entry and the legal segments.
func parseSegment(entry, raw string) (segmentcolor.Segment, error) {
	segment := segmentcolor.Segment(raw)
	if !slices.Contains(segmentcolor.Segments(), segment) {
		return "", fmt.Errorf("shedrecipe: %s: config key %q value %q is not one of %v", entry, "segment", raw, segmentcolor.Segments())
	}
	return segment, nil
}
