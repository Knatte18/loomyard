// archive.go copies a campaign round's files into the tracked harden directory and commits them.

package hardenshed

import (
	"fmt"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/Knatte18/loomyard/internal/lymdirs"
	"github.com/Knatte18/loomyard/internal/shedadapters"
)

// ledgerFileSuffix ends the name of a Bouncer ledger file, `round-<n>-bouncer-ledger.md`.
const ledgerFileSuffix = "-bouncer-ledger.md"

// ArchiveDeps is what the round archive needs told.
type ArchiveDeps struct {
	// RunDir is the run directory the round's files are read from.
	RunDir string
	// AnchorRoot is the absolute anchor root the `_lym` archive directory sits under.
	AnchorRoot string
	// Commit commits the anchor-relative, slash-separated paths with message.
	// A path whose content did not change commits nothing.
	Commit func(relPaths []string, message string) error
}

// ArchiveRound copies round's files from the run directory into `_lym/harden/round-<round>/` and commits that directory.
// The files are every `round-<round>-*` file, every `harden-*-<round>.json` file and the latest Bouncer ledger.
// It replaces the directory's earlier content, so archiving one round twice leaves the same directory.
func ArchiveRound(deps ArchiveDeps, round int) error {
	relDir := path.Join(lymdirs.LymDirName, lymdirs.HardenDirName, fmt.Sprintf("round-%d", round))
	dir := filepath.Join(deps.AnchorRoot, filepath.FromSlash(relDir))
	if err := os.RemoveAll(dir); err != nil {
		return fmt.Errorf("archive round %d: clear %s: %w", round, dir, err)
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("archive round %d: create %s: %w", round, dir, err)
	}

	sources, err := roundFiles(deps.RunDir, round)
	if err != nil {
		return fmt.Errorf("archive round %d: %w", round, err)
	}
	ledger, found, err := latestLedger(deps.RunDir)
	if err != nil {
		return fmt.Errorf("archive round %d: %w", round, err)
	}
	if found {
		sources = append(sources, ledger)
	}
	for _, source := range sources {
		if err := copyFile(source, filepath.Join(dir, filepath.Base(source))); err != nil {
			return fmt.Errorf("archive round %d: %w", round, err)
		}
	}
	if err := deps.Commit([]string{relDir}, fmt.Sprintf("harden: archive round %d", round)); err != nil {
		return fmt.Errorf("archive round %d: commit %s: %w", round, relDir, err)
	}
	return nil
}

// ArchiveFinalLedger copies the run directory's latest Bouncer ledger into `_lym/harden/` and commits it.
// It fails when the run directory holds no ledger.
func ArchiveFinalLedger(deps ArchiveDeps) error {
	ledger, found, err := latestLedger(deps.RunDir)
	if err != nil {
		return fmt.Errorf("archive the final ledger: %w", err)
	}
	if !found {
		return fmt.Errorf("archive the final ledger: %s holds no bouncer ledger", deps.RunDir)
	}
	relPath := path.Join(lymdirs.LymDirName, lymdirs.HardenDirName, filepath.Base(ledger))
	if err := copyFile(ledger, filepath.Join(deps.AnchorRoot, filepath.FromSlash(relPath))); err != nil {
		return fmt.Errorf("archive the final ledger: %w", err)
	}
	if err := deps.Commit([]string{relPath}, "harden: archive the final bouncer ledger"); err != nil {
		return fmt.Errorf("archive the final ledger: commit %s: %w", relPath, err)
	}
	return nil
}

// roundFiles returns the paths of the run directory's files that belong to round:
// the shed's `round-<round>-*` files and the campaign's `harden-*-<round>.json` files.
func roundFiles(runDir string, round int) ([]string, error) {
	entries, err := os.ReadDir(runDir)
	if err != nil {
		return nil, fmt.Errorf("list %s: %w", runDir, err)
	}
	shedPrefix := fmt.Sprintf("round-%d-", round)
	hardenSuffix := fmt.Sprintf("-%d.json", round)
	var files []string
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() {
			continue
		}
		if strings.HasPrefix(name, shedPrefix) || (strings.HasPrefix(name, hardenFilePrefix) && strings.HasSuffix(name, hardenSuffix)) {
			files = append(files, filepath.Join(runDir, name))
		}
	}
	return files, nil
}

// latestLedger returns the path of the run directory's Bouncer ledger of the highest round, and whether one exists.
func latestLedger(runDir string) (string, bool, error) {
	entries, err := os.ReadDir(runDir)
	if err != nil {
		return "", false, fmt.Errorf("list %s: %w", runDir, err)
	}
	var latest string
	var latestRound int
	for _, entry := range entries {
		round, ok := shedadapters.OutputRound(entry.Name())
		if !ok || entry.IsDir() || !strings.HasSuffix(entry.Name(), ledgerFileSuffix) || round <= latestRound {
			continue
		}
		latest, latestRound = filepath.Join(runDir, entry.Name()), round
	}
	return latest, latest != "", nil
}

// copyFile writes the content of source to target, replacing target.
func copyFile(source, target string) error {
	content, err := os.ReadFile(source)
	if err != nil {
		return fmt.Errorf("read %s: %w", source, err)
	}
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return fmt.Errorf("create the directory of %s: %w", target, err)
	}
	if err := os.WriteFile(target, content, 0o644); err != nil {
		return fmt.Errorf("write %s: %w", target, err)
	}
	return nil
}
