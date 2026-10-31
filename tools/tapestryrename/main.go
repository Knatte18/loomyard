// Command tapestryrename renames the loom recipe to tapestry across the repository it is run in.
// It leaves every other `loom` in place, since those name the general run every recipe shares.
// It moves the files its move table lists and substitutes the whole tokens its substitution table lists, identifiers, recipe-name values and prose phrases alike, in every file outside its excluded paths.
// It formats each Go file it changes, since a renamed identifier shifts the alignment gofmt gives its neighbours.
// It never runs git and never touches a hub's records.
//
//	go run ./tools/tapestryrename [-dry-run] [-root <dir>]
//
// Run once on 2026-10-31, by the tapestry-rename task.
package main

import (
	"flag"
	"fmt"
	"go/format"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

func main() {
	dryRun := flag.Bool("dry-run", false, "report what would change without moving or writing files")
	root := flag.String("root", ".", "the repository root to rename in")
	flag.Parse()

	moved, err := applyMoves(*root, moves, *dryRun)
	if err != nil {
		fmt.Fprintln(os.Stderr, "tapestryrename:", err)
		os.Exit(1)
	}
	for _, m := range moved {
		fmt.Printf("moved: %s -> %s\n", m.from, m.to)
	}

	changed, err := substituteTree(*root, *dryRun)
	if err != nil {
		fmt.Fprintln(os.Stderr, "tapestryrename:", err)
		os.Exit(1)
	}
	for _, path := range changed {
		fmt.Printf("changed: %s\n", path)
	}
	fmt.Printf("\n%d moved, %d changed\n", len(moved), len(changed))
}

// applyMoves renames each listed file under root whose source exists, refusing a move whose target already exists.
// A move whose source is absent and whose target exists counts as done, so a second run moves nothing.
// It returns the moves it made, or would make under dryRun.
func applyMoves(root string, table []move, dryRun bool) ([]move, error) {
	var done []move
	for _, m := range table {
		from := filepath.Join(root, filepath.FromSlash(m.from))
		to := filepath.Join(root, filepath.FromSlash(m.to))
		if _, err := os.Stat(from); os.IsNotExist(err) {
			continue
		}
		if _, err := os.Stat(to); err == nil {
			return done, fmt.Errorf("move %s: the target %s already exists", m.from, m.to)
		}
		if !dryRun {
			if err := os.Rename(from, to); err != nil {
				return done, err
			}
		}
		done = append(done, m)
	}
	return done, nil
}

// substituteTree runs substitute over every regular file under root outside the excluded paths, formats each Go file it changes, and writes back each file it changes with its mode kept, unless dryRun is set.
// It returns the slash-separated paths, relative to root, of the files it changed.
func substituteTree(root string, dryRun bool) ([]string, error) {
	var changed []string
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		if rel != "." && excluded(rel, entry.IsDir()) {
			if entry.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if !entry.Type().IsRegular() {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		out := substitute(string(data), substitutionKeys(rel))
		if strings.HasSuffix(rel, ".go") && out != string(data) {
			formatted, err := format.Source([]byte(out))
			if err != nil {
				return fmt.Errorf("format %s: %w", rel, err)
			}
			out = string(formatted)
		}
		if out == string(data) {
			return nil
		}
		changed = append(changed, rel)
		if dryRun {
			return nil
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		return os.WriteFile(path, []byte(out), info.Mode().Perm())
	})
	return changed, err
}

// excluded reports whether the slash-separated path rel, relative to the root, lies under an excluded path.
func excluded(rel string, isDir bool) bool {
	if isDir {
		rel += "/"
	}
	for _, prefix := range excludedPaths {
		if strings.HasPrefix(rel, prefix) {
			return true
		}
	}
	return false
}
