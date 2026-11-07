// proposal.go declares the raddle proposal's types.
// Parse decodes a proposal file and refuses an unknown key; Hash returns the approval hash.

package raddleparser

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"maps"
	"reflect"
	"slices"
	"strings"
)

// specPath is the format contract the proposal is written against.
const specPath = "contracts/specs/raddle-proposal-spec.md"

// The shapes a proposal may take.
const (
	ShapeTapestry = "tapestry"
	ShapeTasks    = "tasks"
	ShapeProject  = "project"
	ShapeBeam     = "beam"
)

// The recipes a part may name.
const (
	RecipeTapestry = "tapestry"
	RecipeDarn     = "darn"
)

// TypeSketch is the board's type label of a Sketch, which no filed entry may carry.
const TypeSketch = "sketch"

// Proposal is the decoded proposal file.
// The Project field is nil when the file holds no project key.
type Proposal struct {
	Sketch       string       `json:"sketch"`
	Shape        string       `json:"shape"`
	Project      *Project     `json:"project,omitempty"`
	BoardChecked []BoardCheck `json:"board_checked"`
	Parts        []Part       `json:"parts"`
	Cost         Cost         `json:"cost"`
}

// Project is the project a `project` or `beam` shape files:
// its slug, title and the board type label the agent picked for it.
type Project struct {
	Slug  string `json:"slug"`
	Title string `json:"title"`
	Type  string `json:"type"`
}

// BoardCheck is a board entry the agent weighed, with the `revised` stamp it read, in RFC 3339 form.
type BoardCheck struct {
	Slug    string `json:"slug"`
	Revised string `json:"revised"`
}

// Part is one entry the proposal files.
// ExpectedCostUSD is nil when a cell the figure rests on reads "too few runs".
type Part struct {
	Slug           string    `json:"slug"`
	Title          string    `json:"title"`
	Type           string    `json:"type"`
	Recipe         string    `json:"recipe"`
	Profile        string    `json:"profile"`
	Decisions      []string  `json:"decisions"`
	Packages       []Package `json:"packages"`
	Contract       Contract  `json:"contract"`
	DependsOn      []string  `json:"depends_on"`
	OpenRisks      []string  `json:"open_risks"`
	Criteria       []string  `json:"criteria"`
	PatternChanges []string  `json:"pattern_changes"`
	// PatternClusters maps the carried PATTERN changes a live entry absorbs onto that entry.
	PatternClusters []PatternCluster `json:"pattern_clusters,omitempty"`
	ExpectedCostUSD *float64         `json:"expected_cost_usd"`
	CostLine        string           `json:"cost_line"`
}

// PatternCluster moves one PATTERN change of the Sketch onto the live PATTERN.md entry that absorbs it,
// so the part's record declares that entry.
// Text is the live entry's whole new text.
type PatternCluster struct {
	SketchID string `json:"sketch_id"`
	Entry    string `json:"entry"`
	Text     string `json:"text"`
}

// PatternEntryIDs returns the entry ids the part's record declares:
// its PatternChanges in order, each one a cluster maps replaced by that cluster's entry.
func (p Part) PatternEntryIDs() []string {
	ids := slices.Clone(p.PatternChanges)
	for _, cluster := range p.PatternClusters {
		if i := slices.Index(ids, cluster.SketchID); i >= 0 {
			ids[i] = cluster.Entry
		}
	}
	return ids
}

// Package is a package a part changes and the rough number of changed lines.
type Package struct {
	Path string `json:"path"`
	Size int    `json:"size"`
}

// Contract is what a part takes in, delivers and exports.
type Contract struct {
	Takes    string `json:"takes"`
	Delivers string `json:"delivers"`
	Exports  string `json:"exports"`
}

// Cost is the proposal's expected cost, the next-best split and the cost cells both rest on.
// ExpectedUSD is nil when a cell the figure rests on reads "too few runs".
type Cost struct {
	ExpectedUSD *float64   `json:"expected_usd"`
	Line        string     `json:"line"`
	NextBest    NextBest   `json:"next_best"`
	Cells       []CostCell `json:"cells"`
}

// NextBest is the next-best split in one line, with its expected cost.
type NextBest struct {
	Split       string   `json:"split"`
	ExpectedUSD *float64 `json:"expected_usd"`
	Line        string   `json:"line"`
}

// CostCell names a `lym report cost` cell a cost figure rests on.
type CostCell struct {
	Recipe       string `json:"recipe"`
	Profile      string `json:"profile"`
	StoredRecord bool   `json:"stored_record"`
}

// Parse decodes the bytes of a proposal file.
// It refuses a file that is not one JSON object of the spec's form.
// It also refuses a value of the wrong type and a key the spec does not pin.
// Each refusal is an error that names the way forward.
// It finds an unknown key by reading each object a second time as a map of raw values.
// It compares the keys of that map with the json tags of the type that receives the object.
// It never asks the decoder to disallow unknown fields.
func Parse(data []byte) (Proposal, error) {
	var raw json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return Proposal{}, fmt.Errorf("raddleparser: the proposal is not valid JSON: %w; way forward: write one JSON object in the form of %s", err, specPath)
	}
	if err := rejectUnknownKeys(raw, reflect.TypeOf(Proposal{}), "the proposal"); err != nil {
		return Proposal{}, err
	}
	var proposal Proposal
	if err := json.Unmarshal(raw, &proposal); err != nil {
		return Proposal{}, fmt.Errorf("raddleparser: the proposal does not fit its format: %w; way forward: write each value in the form %s pins", err, specPath)
	}
	return proposal, nil
}

// rejectUnknownKeys walks raw alongside typ.
// It returns an error naming the first object key, in sorted order, that no json tag of the receiving struct holds.
// A value of the wrong shape is left to the typed decode, which names it.
func rejectUnknownKeys(raw json.RawMessage, typ reflect.Type, path string) error {
	for typ.Kind() == reflect.Pointer {
		typ = typ.Elem()
	}
	switch typ.Kind() {
	case reflect.Slice:
		var items []json.RawMessage
		if json.Unmarshal(raw, &items) != nil {
			return nil
		}
		for i, item := range items {
			if err := rejectUnknownKeys(item, typ.Elem(), fmt.Sprintf("%s[%d]", path, i)); err != nil {
				return err
			}
		}
	case reflect.Struct:
		var fields map[string]json.RawMessage
		if json.Unmarshal(raw, &fields) != nil {
			return nil
		}
		known := jsonFieldTypes(typ)
		for _, key := range slices.Sorted(maps.Keys(fields)) {
			fieldType, ok := known[key]
			if !ok {
				return fmt.Errorf("raddleparser: %s holds the unknown key %q; way forward: remove it, or write only the keys %s pins", path, key, specPath)
			}
			if err := rejectUnknownKeys(fields[key], fieldType, path+"."+key); err != nil {
				return err
			}
		}
	}
	return nil
}

// jsonFieldTypes maps each json key of the struct type typ to the type of its field.
func jsonFieldTypes(typ reflect.Type) map[string]reflect.Type {
	fields := map[string]reflect.Type{}
	for i := 0; i < typ.NumField(); i++ {
		name, _, _ := strings.Cut(typ.Field(i).Tag.Get("json"), ",")
		if name != "" && name != "-" {
			fields[name] = typ.Field(i).Type
		}
	}
	return fields
}

// Hash returns the hex SHA-256 of the proposal file's bytes, the approval hash.
func Hash(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}
