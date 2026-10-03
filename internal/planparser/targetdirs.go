// targetdirs.go holds CardTargetDirs, the one read-only mapping from a card's written targets to the worktree directories they live in.
// It lives here so every ref-shape and glyph-to-path decision stays inside this package, per the Planparser Sole-Parser and Glyph Conversion Chokepoint invariants.

package planparser

import "path"

// TargetDir is one worktree-relative directory a card's targets live in.
type TargetDir struct {
	// Dir is the slash-separated directory, "." for the worktree root.
	Dir string

	// DeleteOnly reports whether every target in Dir sits in a Delete group.
	DeleteOnly bool

	// NamesGo reports whether some non-Delete target in Dir is a .go path, a glyph or a handle naming Go source.
	NamesGo bool
}

// CardTargetDirs returns one entry per directory card's targets live in, in first-seen order.
// A path target maps to its own directory, or to itself when it names a directory.
// A glyph or `plan:` handle maps through Glyph.UnitPath, to the unit's directory when the unit is a file.
// A Rename group contributes its pairs' New side alone.
// Uses is excluded, since it is read rather than written.
// A nil plan reads as the default language.
func CardTargetDirs(plan *Plan, card Card) []TargetDir {
	if plan == nil {
		plan = &Plan{}
	}
	var order []string
	byDir := map[string]*TargetDir{}

	add := func(raw string, deleted bool) {
		dir, namesGo, ok := targetDir(plan, raw)
		if !ok {
			return
		}
		entry, seen := byDir[dir]
		if !seen {
			entry = &TargetDir{Dir: dir, DeleteOnly: true}
			byDir[dir] = entry
			order = append(order, dir)
		}
		if !deleted {
			entry.DeleteOnly = false
			entry.NamesGo = entry.NamesGo || namesGo
		}
	}

	for _, g := range card.TargetGroups {
		deleted := g.Type == CardTypeDelete
		if g.Type == CardTypeRename {
			for _, p := range g.Pairs {
				add(p.New, deleted)
			}
			continue
		}
		for _, ref := range g.Refs {
			add(ref, deleted)
		}
	}

	dirs := make([]TargetDir, 0, len(order))
	for _, dir := range order {
		dirs = append(dirs, *byDir[dir])
	}
	return dirs
}

// targetDir maps one target ref to its directory and whether the ref names Go source.
// ok is false for a ref that cannot be mapped to a directory.
func targetDir(plan *Plan, raw string) (dir string, namesGo, ok bool) {
	body := raw
	isHandle := false
	if b, handle := HandleBody(raw); handle {
		body = b
		isHandle = true
	}

	if lang, langOK := planLanguage(plan); langOK {
		if g, err := parseGlyph(lang, body); err == nil {
			unit, unitOK := g.UnitPath()
			if !unitOK {
				return "", false, false
			}
			return unitDir(unit, true)
		}
	}
	if isHandle {
		return "", false, false
	}
	return unitDir(body, false)
}

// unitDir maps a unit or path spelling to its directory.
// A spelling with a file extension is a file, so its directory is the parent, and it names Go source when the extension is .go.
// A spelling without one is a directory itself, and a glyph's unit there is a package that names Go source.
func unitDir(unit string, fromGlyph bool) (dir string, namesGo, ok bool) {
	if unit == "" {
		return "", false, false
	}
	if hasFileExtension(unit) {
		return path.Dir(path.Clean(unit)), path.Ext(unit) == ".go", true
	}
	return path.Clean(unit), fromGlyph, true
}
