// expand_test.go covers Expand over a map resolver of YAML sources built from Stub rows: the shape an expansion yields and each refusal it makes.

package shedbuild

import (
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// stubRowYAML returns a producers-list item for a Stub row, with each key a "key: value" line of the row.
func stubRowYAML(name string, keys ...string) string {
	return rowYAML(name, append([]string{"engine: Stub"}, keys...))
}

// blockRowForExpand returns a producers-list item for a block row running recipe behind the always check, with each key a "key: value" line of the row.
func blockRowForExpand(name, recipe string, keys ...string) string {
	return rowYAML(name, append([]string{"block: " + recipe, "block_gate: always"}, keys...))
}

func rowYAML(name string, lines []string) string {
	row := "  - name: " + name + "\n"
	for _, line := range lines {
		row += "    " + line + "\n"
	}
	return row
}

// recipeYAML returns a recipe document whose entry is entry and whose rows are rows.
func recipeYAML(entry string, rows ...string) string {
	return "version: 1\nentry: " + entry + "\nterminals: [Post]\nproducers:\n" + strings.Join(rows, "")
}

// mapResolver resolves a recipe name against sources and refuses any other name.
func mapResolver(sources map[string]string) func(string) ([]byte, error) {
	return func(name string) ([]byte, error) {
		source, ok := sources[name]
		if !ok {
			return nil, fmt.Errorf("no recipe named %q", name)
		}
		return []byte(source), nil
	}
}

// parentYAML is a parent recipe with one block row, Blk, running recipe sub between Pre and Post.
func parentYAML(blockKeys ...string) string {
	return recipeYAML("Pre",
		stubRowYAML("Pre", "on_done: Blk"),
		blockRowForExpand("Blk", "sub", blockKeys...),
		stubRowYAML("Post"),
	)
}

func TestExpand(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	subRows := []string{stubRowYAML("S1", "on_done: S2", "segment: seg"), stubRowYAML("S2")}
	sub := recipeYAML("S1", subRows...)

	t.Run("shape", func(t *testing.T) {
		t.Parallel()
		tests := []struct {
			name    string
			parent  string
			sources map[string]string
			want    []Row
		}{
			{
				name:    "one block yields the block row then the sub-recipe's rows under their own names with every empty on_done set to the exit",
				parent:  parentYAML("on_done: Post"),
				sources: map[string]string{"sub": sub},
				want: []Row{
					{Name: "Pre", Engine: "Stub", OnDone: "Blk"},
					{Name: "Blk", Engine: "Block", OnDone: "S1", SkipTo: "Post", Block: "sub", BlockGate: "always"},
					{Name: "S1", Engine: "Stub", OnDone: "S2", Segment: "seg", BlockPath: "Blk"},
					{Name: "S2", Engine: "Stub", OnDone: "Post", BlockPath: "Blk"},
					{Name: "Post", Engine: "Stub"},
				},
			},
			{
				name:   "a block inside a block carries the joined path, takes its innermost exit and an empty on_done inherits the outer exit as its skip target",
				parent: parentYAML("on_done: Post"),
				sources: map[string]string{
					"sub": recipeYAML("O1",
						stubRowYAML("O1", "on_done: Inner"),
						blockRowForExpand("Inner", "inner"),
						stubRowYAML("O2"),
					),
					"inner": recipeYAML("I1", stubRowYAML("I1")),
				},
				want: []Row{
					{Name: "Pre", Engine: "Stub", OnDone: "Blk"},
					{Name: "Blk", Engine: "Block", OnDone: "O1", SkipTo: "Post", Block: "sub", BlockGate: "always"},
					{Name: "O1", Engine: "Stub", OnDone: "Inner", BlockPath: "Blk"},
					{Name: "Inner", Engine: "Block", OnDone: "I1", SkipTo: "Post", Block: "inner", BlockGate: "always", BlockPath: "Blk"},
					{Name: "I1", Engine: "Stub", OnDone: "Post", BlockPath: "Blk/Inner"},
					{Name: "O2", Engine: "Stub", OnDone: "Post", BlockPath: "Blk"},
					{Name: "Post", Engine: "Stub"},
				},
			},
			{
				name:   "an inner block with an on_done of its own exits there",
				parent: parentYAML("on_done: Post"),
				sources: map[string]string{
					"sub": recipeYAML("O1",
						stubRowYAML("O1", "on_done: Inner"),
						blockRowForExpand("Inner", "inner", "on_done: O2"),
						stubRowYAML("O2"),
					),
					"inner": recipeYAML("I1", stubRowYAML("I1")),
				},
				want: []Row{
					{Name: "Pre", Engine: "Stub", OnDone: "Blk"},
					{Name: "Blk", Engine: "Block", OnDone: "O1", SkipTo: "Post", Block: "sub", BlockGate: "always"},
					{Name: "O1", Engine: "Stub", OnDone: "Inner", BlockPath: "Blk"},
					{Name: "Inner", Engine: "Block", OnDone: "I1", SkipTo: "O2", Block: "inner", BlockGate: "always", BlockPath: "Blk"},
					{Name: "I1", Engine: "Stub", OnDone: "O2", BlockPath: "Blk/Inner"},
					{Name: "O2", Engine: "Stub", OnDone: "Post", BlockPath: "Blk"},
					{Name: "Post", Engine: "Stub"},
				},
			},
		}
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				t.Parallel()
				parent, err := Parse([]byte(tt.parent))
				if err != nil {
					t.Fatalf("Parse(parent) = _, %v; want nil", err)
				}
				before, _ := Parse([]byte(tt.parent))

				got, err := Expand(parent, mapResolver(tt.sources))
				if err != nil {
					t.Fatalf("Expand(...) = _, %v; want nil", err)
				}
				if !reflect.DeepEqual(got.Producers, tt.want) {
					t.Errorf("Expand(...).Producers mismatch:\n got  %+v\n want %+v", got.Producers, tt.want)
				}
				if !reflect.DeepEqual(parent, before) {
					t.Errorf("Expand mutated its argument: %+v; want %+v", parent, before)
				}
			})
		}
	})

	t.Run("the parent's terminals and top-level keys stay the run's whatever the sub-recipe sets", func(t *testing.T) {
		t.Parallel()
		parent, err := Parse([]byte(strings.Replace(parentYAML("on_done: Post"), "terminals: [Post]", "terminals: [Post]\nverify_source: tree_scope\namendment_target: Pre", 1)))
		if err != nil {
			t.Fatalf("Parse(parent) = _, %v; want nil", err)
		}
		foreign := strings.Replace(sub, "terminals: [Post]", "terminals: [NotMine]\ncontract: loom\nverify_source: plan\namendment_target: S1\nprofile_recipe: sub", 1)
		got, err := Expand(parent, mapResolver(map[string]string{"sub": foreign}))
		if err != nil {
			t.Fatalf("Expand(...) = _, %v; want nil", err)
		}
		if !reflect.DeepEqual(got.Terminals, []string{"Post"}) {
			t.Errorf("Expand(...).Terminals = %v; want the parent's [Post]", got.Terminals)
		}
		if got.Contract != "" || got.VerifySource != "tree_scope" || got.AmendmentTarget != "Pre" || got.ProfileRecipe != "" {
			t.Errorf("Expand(...) contract, verify_source, amendment_target, profile_recipe = %q, %q, %q, %q; want the parent's \"\", \"tree_scope\", \"Pre\", \"\"", got.Contract, got.VerifySource, got.AmendmentTarget, got.ProfileRecipe)
		}
	})

	t.Run("a recipe with no block is returned unchanged with a nil resolver", func(t *testing.T) {
		t.Parallel()
		plain, err := Parse([]byte(sub))
		if err != nil {
			t.Fatalf("Parse(sub) = _, %v; want nil", err)
		}
		got, err := Expand(plain, nil)
		if err != nil || !reflect.DeepEqual(got, plain) {
			t.Errorf("Expand(plain, nil) = %+v, %v; want the recipe unchanged and nil", got, err)
		}
	})

	refusals := []struct {
		name       string
		parent     string
		sources    map[string]string
		noResolver bool
		want       string
	}{
		{
			name:    "the resolver refusing the recipe",
			parent:  parentYAML("on_done: Post"),
			sources: map[string]string{},
			want:    `shedbuild: block "Blk": recipe "sub": no recipe named "sub"; way forward: set block to an embedded recipe's name`,
		},
		{
			name:       "a nil resolver",
			parent:     parentYAML("on_done: Post"),
			noResolver: true,
			want:       `shedbuild: block "Blk": no recipe resolver is told; way forward: expand the recipe through a caller that tells the resolver of its embedded recipes`,
		},
		{
			name:   "a block naming its own recipe",
			parent: parentYAML("on_done: Post"),
			sources: map[string]string{
				"sub": recipeYAML("S1", stubRowYAML("S1"), blockRowForExpand("Again", "sub", "on_done: S1")),
			},
			want: `shedbuild: block "Again": recipe "sub" includes itself through sub -> sub; way forward: remove the block that closes the cycle`,
		},
		{
			name:   "a block reaching its own recipe through an inner block",
			parent: parentYAML("on_done: Post"),
			sources: map[string]string{
				"sub": recipeYAML("S1", stubRowYAML("S1"), blockRowForExpand("ToB", "b", "on_done: S1")),
				"b":   recipeYAML("B1", stubRowYAML("B1"), blockRowForExpand("ToSub", "sub", "on_done: B1")),
			},
			want: `shedbuild: block "ToSub": recipe "sub" includes itself through sub -> b -> sub; way forward: remove the block that closes the cycle`,
		},
		{
			name:    "an inlined row named like a row outside the block",
			parent:  parentYAML("on_done: Post"),
			sources: map[string]string{"sub": recipeYAML("Pre", stubRowYAML("Pre"))},
			want:    `shedbuild: block "Blk": row "Pre" of recipe "sub" is also a row outside the block; way forward: rename one of the two rows, or use the recipe in one block only`,
		},
		{
			name: "an inlined segment used outside the block",
			parent: recipeYAML("Pre",
				stubRowYAML("Pre", "on_done: Blk", "segment: seg"),
				blockRowForExpand("Blk", "sub", "on_done: Post"),
				stubRowYAML("Post"),
			),
			sources: map[string]string{"sub": sub},
			want:    `shedbuild: block "Blk": segment "seg" of row "S1" is also used outside the block; way forward: give the block's rows a segment of their own`,
		},
		{
			name:    "an inlined on_done naming no row of its recipe",
			parent:  parentYAML("on_done: Post"),
			sources: map[string]string{"sub": recipeYAML("S1", stubRowYAML("S1", "on_done: Ghost"))},
			want:    `shedbuild: block "Blk": row "S1" of recipe "sub" sets on_done "Ghost", which names no row of that recipe; way forward: route the row inside its recipe, or leave on_done empty to reach the block's exit`,
		},
		{
			name:    "an inlined on_stuck naming no row of its recipe",
			parent:  parentYAML("on_done: Post"),
			sources: map[string]string{"sub": recipeYAML("S1", stubRowYAML("S1", "on_stuck: Ghost"))},
			want:    `row "S1" of recipe "sub" sets on_stuck "Ghost", which names no row of that recipe`,
		},
		{
			name:    "an inlined on_reroute naming no row of its recipe",
			parent:  parentYAML("on_done: Post"),
			sources: map[string]string{"sub": recipeYAML("S1", stubRowYAML("S1", "on_reroute: Ghost"))},
			want:    `row "S1" of recipe "sub" sets on_reroute "Ghost", which names no row of that recipe`,
		},
		{
			name:    "an outermost block row with an empty on_done",
			parent:  parentYAML(),
			sources: map[string]string{"sub": sub},
			want:    `shedbuild: block "Blk": on_done is empty; way forward: set on_done to the row the run takes after the block`,
		},
		{
			name:    "an on_done naming the block row itself",
			parent:  parentYAML("on_done: Blk"),
			sources: map[string]string{"sub": sub},
			want:    `shedbuild: block "Blk": on_done "Blk" lies inside the block; way forward: set on_done to a row outside the block`,
		},
		{
			name:    "an on_done naming an inlined row",
			parent:  parentYAML("on_done: S2"),
			sources: map[string]string{"sub": sub},
			want:    `shedbuild: block "Blk": on_done "S2" lies inside the block; way forward: set on_done to a row outside the block`,
		},
	}
	for _, tt := range refusals {
		t.Run("refuses "+tt.name, func(t *testing.T) {
			t.Parallel()
			parent, err := Parse([]byte(tt.parent))
			if err != nil {
				t.Fatalf("Parse(parent) = _, %v; want nil", err)
			}
			resolve := mapResolver(tt.sources)
			if tt.noResolver {
				resolve = nil
			}
			if _, err := Expand(parent, resolve); err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Errorf("Expand(...) error = %v; want it to contain %q", err, tt.want)
			}
		})
	}
}
