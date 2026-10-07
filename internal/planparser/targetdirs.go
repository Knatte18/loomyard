// targetdirs.go holds CardTargetDirs and RefFile, the read-only mappings from a card's refs to the worktree directories and files they name.
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

// RefFile returns the worktree-relative file a card ref names, and false when the ref names no file.
// A path ref with a file extension is itself, and a glyph or `plan:` handle whose unit is a file is that file.
// A directory or package unit and an unmappable ref have no file.
// A nil plan reads as the default language.
func RefFile(plan *Plan, ref string) (string, bool) {
	if plan == nil {
		plan = &Plan{}
	}
	unit, _, ok := refUnit(plan, ref)
	if !ok || !hasFileExtension(unit) {
		return "", false
	}
	return path.Clean(unit), true
}

// targetDir maps one target ref to its directory and whether the ref names Go source.
// ok is false for a ref that cannot be mapped to a directory.
func targetDir(plan *Plan, raw string) (dir string, namesGo, ok bool) {
	unit, fromGlyph, ok := refUnit(plan, raw)
	if !ok {
		return "", false, false
	}
	return unitDir(unit, fromGlyph)
}

// refUnit maps one ref to the unit or path spelling it names, and whether that spelling came from a glyph.
// ok is false for a ref that names no unit.
func refUnit(plan *Plan, raw string) (unit string, fromGlyph, ok bool) {
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
			return unit, true, true
		}
	}
	if isHandle {
		return "", false, false
	}
	return body, false, true
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
