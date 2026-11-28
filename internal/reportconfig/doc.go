// doc.go carries the package godoc for reportconfig.

// Package reportconfig owns the hub-wide report.yaml: the thresholds of `lym report` figures that a hub tunes.
//
// The module's one key, cost_min_runs, is the run count a cell of `lym report cost` needs before it shows its figures.
// A cell with fewer runs reads "too few runs" instead.
// The raddle agent prices every pair's proposals from those figures.
// So the key is classified cross-pair, and a worktree's local override cannot set it.
//
// LoadConfig degrades to the template when the hub holds no report.yaml.
// A report therefore never fails on a hub not yet reconciled.
// The module sits apart from the report engine because configreg registers its template,
// and the engine already reaches configreg through its own imports.
// LoadConfig never resolves cwd itself: the caller hands it the directories (see PATTERN-cwd-resolution).
package reportconfig
