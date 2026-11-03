// expand.go implements Expand, which inlines the sub-recipe of each block row into its parent recipe.
// It is pure: the recipes it reads reach it as bytes through a told resolver, and it touches no file.

package shedbuild

import (
	"fmt"
	"slices"
	"strings"

	"github.com/Knatte18/loomyard/internal/shedrecipe"
)

// Expand inlines the sub-recipe of every block row of r and returns the resulting Recipe; r itself is never mutated.
// resolve maps a recipe name to its bytes, and each sub-recipe is parsed through Parse and expanded in turn, so a block may hold blocks.
// A recipe with no block row is returned as it is, whatever resolve is.
//
// A block row is emitted with engine shedrecipe.EngineBlock, on_done the sub-recipe's entry and SkipTo the block's exit, followed by the sub-recipe's rows under their own names.
// The block's exit is the row's on_done as its recipe file sets it, or, when that is empty, the exit of the block enclosing it.
// An inlined row's empty on_done, and an inlined block row's empty skip target, become the exit of the innermost block enclosing the row.
// Each inlined row's BlockPath is the names of the block rows enclosing it, outermost first, joined by "/".
// A sub-recipe's terminals are not read; the parent's stay the run's.
//
// Every refusal names the block row and a way forward: a recipe the resolver cannot give, a nil resolver, a recipe that includes itself, a row name or segment an inlined row shares with a row outside its block, an inlined edge naming no row of its recipe, an outermost block row with no on_done, an on_done inside the block, and a sub-recipe setting a key that belongs to the run.
func Expand(r Recipe, resolve func(name string) ([]byte, error)) (Recipe, error) {
	if !slices.ContainsFunc(r.Producers, func(row Row) bool { return row.Block != "" }) {
		return r, nil
	}

	e := &expander{
		resolve:  resolve,
		names:    make(map[string]bool, len(r.Producers)),
		segments: make(map[string]bool),
	}
	e.register(r.Producers)

	rows, err := e.expandRows(r.Producers, "", "", nil)
	if err != nil {
		return Recipe{}, err
	}
	r.Producers = rows
	return r, nil
}

// expander carries the told resolver and the row names and segments of every row seen outside the block being inlined.
type expander struct {
	resolve  func(name string) ([]byte, error)
	names    map[string]bool
	segments map[string]bool
}

// register records the names and non-empty segments of rows as rows outside every block inlined after them.
func (e *expander) register(rows []Row) {
	for _, row := range rows {
		e.names[row.Name] = true
		if row.Segment != "" {
			e.segments[row.Segment] = true
		}
	}
}

// expandRows returns rows with every block row replaced by its block row and inlined rows.
// exit is the exit of the innermost enclosing block, empty at the top level; path is the block path of rows, and chain the names of the recipes entered to reach them.
func (e *expander) expandRows(rows []Row, exit, path string, chain []string) ([]Row, error) {
	out := make([]Row, 0, len(rows))
	for _, row := range rows {
		if row.Block == "" {
			if exit != "" && row.OnDone == "" {
				row.OnDone = exit
			}
			row.BlockPath = path
			out = append(out, row)
			continue
		}

		blockExit := row.OnDone
		if blockExit == "" {
			if exit == "" {
				return nil, blockError(row, "on_done is empty; way forward: set on_done to the row the run takes after the block")
			}
			blockExit = exit
		}
		inlined, entry, err := e.inline(row, blockExit, path, chain)
		if err != nil {
			return nil, err
		}
		out = append(out, Row{
			Name:      row.Name,
			Engine:    shedrecipe.EngineBlock,
			OnDone:    entry,
			SkipTo:    blockExit,
			Block:     row.Block,
			BlockGate: row.BlockGate,
			BlockPath: path,
		})
		out = append(out, inlined...)
	}
	return out, nil
}

// inline resolves, parses and expands the recipe block names, returning its rows ready to follow the block row and the recipe's entry.
// exit is the block's exit, path the block path of the block row, and chain the names of the recipes entered to reach it.
func (e *expander) inline(block Row, exit, path string, chain []string) (rows []Row, entry string, err error) {
	if at := slices.Index(chain, block.Block); at >= 0 {
		loop := append(slices.Clone(chain[at:]), block.Block)
		return nil, "", blockError(block, "recipe %q includes itself through %s; way forward: remove the block that closes the cycle", block.Block, strings.Join(loop, " -> "))
	}
	if e.resolve == nil {
		return nil, "", blockError(block, "no recipe resolver is told; way forward: expand the recipe through a caller that tells the resolver of its embedded recipes")
	}
	data, err := e.resolve(block.Block)
	if err != nil {
		return nil, "", blockError(block, "recipe %q: %w; way forward: set block to an embedded recipe's name", block.Block, err)
	}
	sub, err := Parse(data)
	if err != nil {
		return nil, "", blockError(block, "recipe %q: %w; way forward: set block to an embedded recipe's name", block.Block, err)
	}

	for _, key := range []struct {
		name string
		set  bool
	}{
		{"contract", sub.Contract != ""},
		{"verify_source", sub.VerifySource != ""},
		{"amendment_target", sub.AmendmentTarget != ""},
		{"profile_recipe", sub.ProfileRecipe != ""},
	} {
		if key.set {
			return nil, "", blockError(block, "recipe %q sets %s, which belongs to the run; way forward: remove %s from the block's recipe", block.Block, key.name, key.name)
		}
	}

	own := make(map[string]bool, len(sub.Producers))
	for _, row := range sub.Producers {
		own[row.Name] = true
	}
	for _, row := range sub.Producers {
		for _, edge := range []struct{ key, target string }{{"on_done", row.OnDone}, {"on_stuck", row.OnStuck}, {"on_reroute", row.OnReroute}} {
			if edge.target != "" && !own[edge.target] {
				return nil, "", blockError(block, "row %q of recipe %q sets %s %q, which names no row of that recipe; way forward: route the row inside its recipe, or leave on_done empty to reach the block's exit", row.Name, block.Block, edge.key, edge.target)
			}
		}
	}
	for _, row := range sub.Producers {
		if e.names[row.Name] {
			return nil, "", blockError(block, "row %q of recipe %q is also a row outside the block; way forward: rename one of the two rows, or use the recipe in one block only", row.Name, block.Block)
		}
	}
	for _, row := range sub.Producers {
		if row.Segment != "" && e.segments[row.Segment] {
			return nil, "", blockError(block, "segment %q of row %q is also used outside the block; way forward: give the block's rows a segment of their own", row.Segment, row.Name)
		}
	}
	e.register(sub.Producers)

	blockPath := block.Name
	if path != "" {
		blockPath = path + "/" + block.Name
	}
	rows, err = e.expandRows(sub.Producers, exit, blockPath, append(slices.Clone(chain), block.Block))
	if err != nil {
		return nil, "", err
	}

	if block.OnDone != "" {
		inside := block.OnDone == block.Name
		for _, row := range rows {
			inside = inside || row.Name == block.OnDone
		}
		if inside {
			return nil, "", blockError(block, "on_done %q lies inside the block; way forward: set on_done to a row outside the block", block.OnDone)
		}
	}
	return rows, sub.Entry, nil
}

// blockError returns a refusal prefixed with the block row's name, formatting the case and its way forward from format and args.
func blockError(block Row, format string, args ...any) error {
	return fmt.Errorf("shedbuild: block %q: "+format, append([]any{block.Name}, args...)...)
}
