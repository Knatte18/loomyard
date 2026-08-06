// detect.go implements DetectLanguage, marker-based language detection over a target directory.
// It never resolves the process's own cwd — targetDir is a plain argument the caller (batch 3's CLI layer) resolves — and it never spawns a subprocess;
// every check is a stat call.

package scoutengine

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
)

// DetectLanguage identifies which registered language targetDir belongs to, using langOverride if provided or marker-based detection otherwise.
// Returns ErrNoLanguage if no language matches.
func DetectLanguage(targetDir string, reg Registry, langOverride string) (string, Entry, error) {
	if langOverride != "" {
		entry, ok := reg[langOverride]
		if !ok {
			return "", Entry{}, fmt.Errorf("scoutengine: unknown language %q; known languages: %v", langOverride, sortedLanguages(reg))
		}
		return langOverride, entry, nil
	}

	var searched []string
	for _, lang := range precedence {
		entry, ok := reg[lang]
		if !ok {
			continue
		}
		searched = append(searched, entry.Markers...)
		if markersMatch(targetDir, entry) {
			return lang, entry, nil
		}
	}

	return "", Entry{}, fmt.Errorf("scoutengine: %w: searched markers %v under %s", ErrNoLanguage, searched, targetDir)
}

// markersMatch reports whether entry's markers match targetDir per entry.Match.
func markersMatch(targetDir string, entry Entry) bool {
	switch entry.Match {
	case "all":
		for _, marker := range entry.Markers {
			if !markerExists(targetDir, marker) {
				return false
			}
		}
		return true
	default:
		// validateEntry already restricts Match to {"all", "any"} for every
		// registry an operator can construct via LoadRegistry, so "any" is
		// the only remaining case in practice; treating it as the default
		// keeps this switch total without a redundant explicit case.
		for _, marker := range entry.Markers {
			if markerExists(targetDir, marker) {
				return true
			}
		}
		return false
	}
}

// markerExists reports whether marker (a file or directory name) is present
// directly under targetDir.
func markerExists(targetDir, marker string) bool {
	_, err := os.Stat(filepath.Join(targetDir, marker))
	return err == nil
}

// sortedLanguages returns reg's language keys sorted, used to name the valid
// options in an unknown-langOverride error.
func sortedLanguages(reg Registry) []string {
	keys := make([]string, 0, len(reg))
	for k := range reg {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
