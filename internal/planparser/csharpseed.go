// csharpseed.go holds CardCSharpSeeds, the read-only mapping from a C# plan's card targets to the files and type names a type-closure test scoper starts from.
// It lives here so every ref-shape and glyph decision stays inside this package, per the Planparser Sole-Parser invariant.

package planparser

import "strings"

// CSharpSeed is one card target as a C# test scoper reads it.
type CSharpSeed struct {
	// Ref is the target as the card spells it.
	Ref string

	// File is the worktree-relative slash path the target names, empty when it names none.
	File string

	// Types are the type names the target seeds, outermost first; empty with an empty File means the target seeds nothing.
	Types []string
}

// CardCSharpSeeds returns one seed per target ref of every target group of cards, in card and body order.
// Every group type counts, a Rename group contributing both sides of each pair, and Uses is left out, since it is read rather than written.
// A ref that names a file seeds that File.
// A member glyph or `plan:` handle seeds each name of its owner chain, and a glyph with no owner seeds its own name as a type, since C# has no namespace-level members.
// A seeded name drops its generic or arity suffix from the first `<` and a leading `@`.
// Any other ref, such as a directory path or a namespace-only self glyph, yields a seed with neither File nor Types.
// It reads no disk: existence and the file extension are the caller's to judge.
func CardCSharpSeeds(plan *Plan, cards []Card) []CSharpSeed {
	if plan == nil {
		plan = &Plan{}
	}
	var seeds []CSharpSeed
	for _, card := range cards {
		for _, group := range card.TargetGroups {
			for _, ref := range group.Refs {
				seeds = append(seeds, cSharpSeed(plan, ref))
			}
		}
	}
	return seeds
}

// cSharpSeed maps one target ref to its seed.
func cSharpSeed(plan *Plan, ref string) CSharpSeed {
	seed := CSharpSeed{Ref: ref}
	if file, ok := RefFile(plan, ref); ok {
		seed.File = file
		return seed
	}
	body := ref
	if stripped, isHandle := HandleBody(ref); isHandle {
		body = stripped
	}
	parsed, err := ParseRef(plan, body)
	if err != nil || parsed.IsSelf() {
		return seed
	}
	names := parsed.Owner
	if len(names) == 0 {
		names = []string{parsed.Name}
	}
	for _, name := range names {
		name, _, _ = strings.Cut(strings.TrimPrefix(name, "@"), "<")
		if name != "" {
			seed.Types = append(seed.Types, name)
		}
	}
	return seed
}
