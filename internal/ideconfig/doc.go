// doc.go documents the ideconfig package.

// Package ideconfig owns the hub-wide ide.yaml:
// which files a VS Code window lym opens shows on start, per place, and whether a markdown file opens rendered.
//
// ide.open_files holds one list per place: the prime, a beam and a task.
// Each entry is a path relative to the worktree's anchored directory, the folder its window shows.
// An entry may name the {beam} placeholder, which the opener resolves to the beam's slug.
// The template opens nothing for any place.
// ide.render_markdown, on in the template, makes a markdown file open as VS Code's markdown preview.
// With it off, a markdown file opens as source.
// Both keys only change what a window shows, so a worktree's local override may set them.
//
// LoadConfig degrades to the template when the hub holds no ide.yaml.
// It refuses an ide.open_files entry that is empty, absolute or climbs out of the worktree, naming the key.
// The module sits apart from the ide engine because configreg registers its template,
// and the engine already reaches configreg through its own imports.
// LoadConfig never resolves cwd itself: the caller hands it the directories (see PATTERN-cwd-resolution).
package ideconfig
