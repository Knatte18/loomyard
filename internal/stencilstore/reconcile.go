// reconcile.go implements the runtime read path (Read), the once-per-process seed/refresh pass
// (Reconcile, ForceRefresh), and the drift-notification warnings that accompany it.
// Reconcile never blocks and never returns a non-zero-affecting error for a drift signal -- both
// drift warnings it emits are logger.Warn lines only, per the seeding-trigger Shared Decision.

package stencilstore

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/stencil"
)

// gitattributesName is the file Reconcile seeds under baseDir so a checked-out stencil never
// arrives CRLF-converted regardless of the operator's core.autocrlf setting.
const gitattributesName = ".gitattributes"

// gitattributesContent is the single line Reconcile writes into a fresh .gitattributes.
const gitattributesContent = "*.md text eol=lf\n"

// Read reads name's current content from baseDir with a plain os.ReadFile on every call -- no
// caching, so an on-disk edit takes effect immediately on the next Read.
// It never falls back to a shipped default and never writes; a missing file is reported as an
// error, not silently substituted, per the missing-board-is-a-hard-error Shared Decision.
func Read(baseDir, name string) ([]byte, error) {
	content, err := os.ReadFile(Path(baseDir, name))
	if err != nil {
		return nil, fmt.Errorf("stencilstore: read stencil %q from %s: %w", name, baseDir, err)
	}
	return content, nil
}

// Reconcile is the once-per-process seed/refresh pass: for every name in registry.Names() it reads
// the on-disk file, classifies it against the registry's shipped default, and acts per the
// edit-detection table (see Classify) and dev/prod Mode.
// It also seeds baseDir/.gitattributes when absent, and, when sourceDir is non-empty, warns on any
// board-copy-vs-worktree-source drift.
// It returns the baseDir-relative, slash-separated paths it actually wrote, in registry.Names()
// order, and writes nothing at all when every file is already correct.
func Reconcile(baseDir string, registry Registry, mode Mode, sourceDir string) ([]string, error) {
	var written []string

	for _, name := range registry.Names() {
		shipped, known := registry.Default(name)
		if !known {
			continue
		}

		path := Path(baseDir, name)
		onDisk, readErr := os.ReadFile(path)
		exists := readErr == nil
		if readErr != nil && !os.IsNotExist(readErr) {
			return written, fmt.Errorf("stencilstore: reconcile stencil %q: %w", name, readErr)
		}

		state := Classify(onDisk, exists, shipped)
		wrote, writeErr := reconcileOne(path, name, state, onDisk, shipped, mode)
		if writeErr != nil {
			return written, writeErr
		}
		if wrote {
			written = append(written, filepath.ToSlash(RelPath(name)))
		}
	}

	wroteAttrs, err := seedGitattributes(baseDir)
	if err != nil {
		return written, err
	}
	if wroteAttrs {
		written = append(written, gitattributesName)
	}

	if sourceDir != "" {
		warnPortBackDrift(baseDir, registry, sourceDir)
	}

	return written, nil
}

// reconcileOne applies one registry name's classified state to disk, per Reconcile's per-row
// requirements, and reports whether it wrote the file.
func reconcileOne(path, name string, state State, onDisk, shipped []byte, mode Mode) (bool, error) {
	switch state {
	case StateAbsent:
		if err := writeStamped(path, shipped, BodyHash(shipped)); err != nil {
			return false, err
		}
		return true, nil

	case StateUntouched:
		if BodyHash(shipped) == BodyHash(onDisk) {
			return false, nil
		}
		if mode == ModeDev {
			// The remedy is named at the point of failure rather than left for a reader to find,
			// because without it this warning reads as benign housekeeping while it is in fact
			// reporting that every producer reading this stencil will run on the OLDER on-disk text.
			// A dev build refuses to refresh so it never clobbers a board's stencils with whatever
			// is in a working tree, and the refusal is one-way: an older installed binary running in
			// prod mode DOES refresh, so it can downgrade a board's stencils and the newer dev build
			// can then only warn about it, on every single invocation, forever.
			logger.Warn("stencilstore: dev build does not refresh an untouched stencil; producers will read the OLDER on-disk copy -- run \"lyx stencil sync\" to force-refresh it", "stencil", name, "path", path)
			return false, nil
		}
		if err := writeStamped(path, shipped, BodyHash(shipped)); err != nil {
			return false, err
		}
		return true, nil

	case StateReconciled:
		restamped := ApplyStamp(onDisk, BodyHash(onDisk))
		if string(restamped) == string(onDisk) {
			return false, nil
		}
		if err := os.WriteFile(path, restamped, 0o644); err != nil {
			return false, fmt.Errorf("stencilstore: restamp stencil %q: %w", name, err)
		}
		return true, nil

	case StateEdited:
		if BodyHash(shipped) != BodyHash(onDisk) {
			logger.Warn("stencilstore: edited stencil has fallen behind a newer shipped default; see lyx stencil diff", "stencil", name)
		}
		return false, nil

	default:
		return false, fmt.Errorf("stencilstore: stencil %q classified as unknown state %v", name, state)
	}
}

// writeStamped writes shipped, stamped with hash, to path, creating parent directories as needed.
func writeStamped(path string, shipped []byte, hash string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("stencilstore: create parent directory for %s: %w", path, err)
	}
	if err := os.WriteFile(path, ApplyStamp(shipped, hash), 0o644); err != nil {
		return fmt.Errorf("stencilstore: write stencil at %s: %w", path, err)
	}
	return nil
}

// seedGitattributes writes baseDir/.gitattributes with gitattributesContent when it does not yet
// exist, and reports whether it wrote the file.
// It never rewrites an existing .gitattributes.
func seedGitattributes(baseDir string) (bool, error) {
	path := filepath.Join(baseDir, gitattributesName)
	if _, err := os.Stat(path); err == nil {
		return false, nil
	} else if !os.IsNotExist(err) {
		return false, fmt.Errorf("stencilstore: stat %s: %w", path, err)
	}

	if err := os.MkdirAll(baseDir, 0o755); err != nil {
		return false, fmt.Errorf("stencilstore: create stencils directory %s: %w", baseDir, err)
	}
	if err := os.WriteFile(path, []byte(gitattributesContent), 0o644); err != nil {
		return false, fmt.Errorf("stencilstore: write %s: %w", path, err)
	}
	return true, nil
}

// warnPortBackDrift compares each registry name's on-disk board copy against sourceDir's worktree
// copy and emits one logger.Warn per differing stencil.
// A missing source file is skipped silently; this comparison never returns an error and never
// affects an exit code, per the drift-notification-is-logger-warn-and-never-blocks Shared Decision.
func warnPortBackDrift(baseDir string, registry Registry, sourceDir string) {
	for _, name := range registry.Names() {
		boardPath := Path(baseDir, name)
		boardContent, err := os.ReadFile(boardPath)
		if err != nil {
			continue
		}

		sourcePath := filepath.Join(sourceDir, filepath.FromSlash(RelPath(name)))
		sourceContent, err := os.ReadFile(sourcePath)
		if err != nil {
			continue
		}

		boardBody := NormalizeLF([]byte(stencil.StripLeadingComment(string(boardContent))))
		sourceBody := NormalizeLF([]byte(stencil.StripLeadingComment(string(sourceContent))))
		if string(boardBody) != string(sourceBody) {
			logger.Warn("stencilstore: board copy has drifted from worktree source; see lyx stencil promote", "stencil", name)
		}
	}
}

// ForceRefresh performs the refresh row even on a stencil a ModeDev pass would leave untouched.
// It is the entry point `lyx stencil sync` calls, which is why an explicit sync refreshes even from
// a -dev-stamped binary.
func ForceRefresh(baseDir string, registry Registry, sourceDir string) ([]string, error) {
	return Reconcile(baseDir, registry, ModeProduction, sourceDir)
}
