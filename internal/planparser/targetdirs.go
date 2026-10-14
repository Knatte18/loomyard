// targetdirs.go holds CardTargetDirs, RefFile, RefScope and RefCovers, the read-only mappings from a card's refs to the worktree directories and files they name.
// It lives here so every ref-shape and glyph-to-path decision stays inside this package, per the Planparser Sole-Parser and Glyph Conversion Chokepoint invariants.

package planparser

import (
	"path"
	"strings"
)

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
// An external test package's unit maps to its package directory.
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

// RefScope returns the worktree scope a card ref covers, and false when the ref names no unit.
// A unit or path with a file extension is that cleaned file and isFile is true, so a changed file is covered only by equality.
// Any other unit is its package directory with isFile false, a glyph's or `plan:` handle's unit taken through goPackageDir, so an external test package's pseudo-unit names its package directory.
// A nil plan reads as the default language.
func RefScope(plan *Plan, ref string) (scope string, isFile bool, ok bool) {
	if plan == nil {
		plan = &Plan{}
	}
	unit, fromGlyph, ok := refUnit(plan, ref)
	if !ok || unit == "" {
		return "", false, false
	}
	if hasFileExtension(unit) {
		return path.Clean(unit), true, true
	}
	if fromGlyph {
		unit = goPackageDir(unit)
	}
	return path.Clean(unit), false, true
}

// RefCovers reports whether cover, a Rename pair's old side or a Delete target, covers target, a ref an earlier card writes.
// A cover naming a member, a glyph or `plan:` handle that is not a self glyph, covers a target naming the same member.
// A cover naming a file covers a target in that same file, and a glyph or handle target whose package directory is the file's directory.
// A cover naming a directory, a plain directory path or a package self glyph, covers a target whose directory lies at or under it.
// A ref that maps to no unit neither covers nor is covered.
// A nil plan reads as the default language.
func RefCovers(plan *Plan, cover, target string) bool {
	if plan == nil {
		plan = &Plan{}
	}
	if member, ok := refMember(plan, cover); ok {
		targetMember, targetOK := refMember(plan, target)
		return targetOK && member == targetMember
	}

	coverScope, coverIsFile, ok := RefScope(plan, cover)
	if !ok {
		return false
	}
	targetScope, targetIsFile, ok := RefScope(plan, target)
	if !ok {
		return false
	}

	if coverScope == "." {
		return true
	}
	if coverIsFile {
		if targetIsFile {
			return targetScope == coverScope
		}
		_, targetFromGlyph, _ := refUnit(plan, target)
		return targetFromGlyph && targetScope == path.Dir(coverScope)
	}

	targetDir := targetScope
	if targetIsFile {
		targetDir = path.Dir(targetScope)
	}
	return targetDir == coverScope || strings.HasPrefix(targetDir, coverScope+"/")
}

// refMember returns the canonical glyph string of a ref naming a member, a glyph or a `plan:` handle whose body parses to a glyph that is not a self glyph.
// ok is false for any other ref.
func refMember(plan *Plan, ref string) (string, bool) {
	body := ref
	if b, handle := HandleBody(ref); handle {
		body = b
	}
	lang, ok := planLanguage(plan)
	if !ok {
		return "", false
	}
	g, err := parseGlyph(lang, body)
	if err != nil || g.IsSelf() {
		return "", false
	}
	return g.String(), true
}

// targetDir maps one target ref to its directory and whether the ref names Go source.
// ok is false for a ref that cannot be mapped to a directory.
func targetDir(plan *Plan, raw string) (dir string, namesGo, ok bool) {
	unit, fromGlyph, ok := refUnit(plan, raw)
	if !ok {
		return "", false, false
	}
	if fromGlyph {
		unit = goPackageDir(unit)
	}
	return unitDir(unit, fromGlyph)
}

// goPackageDir maps a Go unit string, a glyph's UnitPath result or a handle's package half, to the directory its package lives in.
// It is the one sanctioned adjustment of a Go unit string under PATTERN-glyph-conversion-chokepoint:
// quarry names an external test package's unit `<dir>_test`, a pseudo-unit with no directory of its own, so a last path segment ending in `_test` with no file extension comes back without that suffix.
// Any other unit is returned unchanged.
// It reads no disk, so a real package directory whose own name ends in `_test` maps to its sibling.
func goPackageDir(unit string) string {
	if hasFileExtension(unit) {
		return unit
	}
	return strings.TrimSuffix(unit, "_test")
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
