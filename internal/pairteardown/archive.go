// archive.go copies a pair's trace and verify logs into the hub's log archive before the pair is removed, and prunes the archive to its retention.

package pairteardown

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/verifytree"
)

// logArchiveRetention is how long an archive entry is kept before the next teardown's prune removes it.
const logArchiveRetention = 30 * 24 * time.Hour

// archiveStampLayout is the layout of an archive entry's directory name, a UTC timestamp.
const archiveStampLayout = "20060102T150405Z"

// archiveSource is one group of a pair's log files: the files, and the entry subdirectory they are copied into.
type archiveSource struct {
	name  string
	files []string
}

// logArchive is the hub's log archive: one directory per slug under root, holding one timestamped entry per archived teardown.
type logArchive struct {
	root string
	now  func() time.Time
	// removeAll removes a path and everything under it; os.RemoveAll in production.
	removeAll func(path string) error
}

// write copies each source's files into a new entry under root/slug, in a subdirectory named for the source, and returns the entry's path and the number of files copied.
// With no files in any source it creates nothing and returns an empty entry.
// A copy failure removes the partial entry and returns the error.
func (a logArchive) write(slug string, sources []archiveSource) (entry string, files int, err error) {
	for _, source := range sources {
		files += len(source.files)
	}
	if files == 0 {
		return "", 0, nil
	}

	slugDir := filepath.Join(a.root, slug)
	if err := os.MkdirAll(slugDir, 0o755); err != nil {
		return "", 0, fmt.Errorf("create the archive directory of %q: %w", slug, err)
	}
	entry, err = createEntry(slugDir, a.now().UTC().Format(archiveStampLayout))
	if err != nil {
		return "", 0, err
	}
	for _, source := range sources {
		if err := copyFiles(filepath.Join(entry, source.name), source.files); err != nil {
			return "", 0, errors.Join(err, os.RemoveAll(entry))
		}
	}
	return entry, files, nil
}

// createEntry makes the entry directory stamp under slugDir exclusively, trying stamp-2, stamp-3 and on when the name is taken.
func createEntry(slugDir, stamp string) (string, error) {
	for attempt := 1; ; attempt++ {
		name := stamp
		if attempt > 1 {
			name = fmt.Sprintf("%s-%d", stamp, attempt)
		}
		entry := filepath.Join(slugDir, name)
		err := os.Mkdir(entry, 0o755)
		if err == nil {
			return entry, nil
		}
		if !errors.Is(err, fs.ErrExist) {
			return "", fmt.Errorf("create the archive entry %s: %w", entry, err)
		}
	}
}

// copyFiles copies each of sources into dir, keeping their names.
func copyFiles(dir string, sources []string) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	for _, source := range sources {
		if err := copyFile(source, filepath.Join(dir, filepath.Base(source))); err != nil {
			return err
		}
	}
	return nil
}

// copyFile copies the file source to the new file target.
func copyFile(source, target string) error {
	in, err := os.Open(source)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(target, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		return errors.Join(err, out.Close())
	}
	return out.Close()
}

// prune removes every entry older than logArchiveRetention, and each slug directory it leaves empty.
// A name that is not an archive timestamp and a plain file are left alone; a path already gone counts as removed, and any other failure logs a Warn and pruning goes on.
func (a logArchive) prune() {
	slugs, err := os.ReadDir(a.root)
	if err != nil {
		if !errors.Is(err, fs.ErrNotExist) {
			logger.Warn("pairteardown: could not read the log archive to prune it", "root", a.root, "cause", err)
		}
		return
	}
	for _, slug := range slugs {
		if slug.IsDir() {
			a.pruneSlug(filepath.Join(a.root, slug.Name()))
		}
	}
}

// pruneSlug removes the expired entries of slugDir, then slugDir itself when nothing is left in it.
func (a logArchive) pruneSlug(slugDir string) {
	entries, err := os.ReadDir(slugDir)
	if err != nil {
		if !errors.Is(err, fs.ErrNotExist) {
			logger.Warn("pairteardown: could not read an archived slug to prune it", "dir", slugDir, "cause", err)
		}
		return
	}
	kept := 0
	for _, entry := range entries {
		if entry.IsDir() && a.expired(entry.Name()) && a.remove(filepath.Join(slugDir, entry.Name())) {
			continue
		}
		kept++
	}
	if kept == 0 {
		a.remove(slugDir)
	}
}

// expired reports whether the entry name, with or without a collision suffix, is an archive timestamp older than the retention.
func (a logArchive) expired(name string) bool {
	stamp, _, _ := strings.Cut(name, "-")
	at, err := time.Parse(archiveStampLayout, stamp)
	return err == nil && a.now().Sub(at) > logArchiveRetention
}

// remove removes path through removeAll and reports whether it is gone; a failure other than a missing path logs a Warn.
func (a logArchive) remove(path string) bool {
	err := a.removeAll(path)
	if err == nil || errors.Is(err, fs.ErrNotExist) {
		return true
	}
	logger.Warn("pairteardown: could not remove an expired log archive path", "path", path, "cause", err)
	return false
}

// archivePairLogs copies the pair's trace and verify logs into the hub's log archive, after pruning it.
// It is best effort: a failure logs a Warn and changes nothing about the teardown, and the pair's own files are copied, never moved.
func (t *Teardown) archivePairLogs(slug string) {
	anchor := t.taskAnchor(slug)
	verifyDir := verifytree.Dir(anchor)
	verifyPaths := verifytree.NewPaths(anchor, verifyDir)
	traceFiles, traceErr := regularFilesIn(logger.LogsDirAt(anchor), func(string) bool { return true })
	verifyFiles, verifyErr := regularFilesIn(verifyDir, func(path string) bool { return verifytree.IsLogPath(verifyPaths, path) })
	if err := errors.Join(traceErr, verifyErr); err != nil {
		logger.Warn("pairteardown: could not list the pair's logs for the archive", "slug", slug, "cause", err)
		return
	}

	archive := logArchive{root: fabricengine.LogArchiveDir(t.prime.HubPath), now: time.Now, removeAll: os.RemoveAll}
	archive.prune()
	entry, files, err := archive.write(slug, []archiveSource{{name: "trace", files: traceFiles}, {name: "verify", files: verifyFiles}})
	if err != nil {
		logger.Warn("pairteardown: could not archive the pair's logs", "slug", slug, "cause", err)
		return
	}
	if entry != "" {
		logger.Info("pairteardown: archived the pair's logs", "slug", slug, "entry", entry, "files", files)
	}
}

// regularFilesIn returns the paths of the regular files directly in dir that keep accepts; a missing dir holds none.
func regularFilesIn(dir string, keep func(path string) bool) ([]string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, nil
		}
		return nil, err
	}
	var paths []string
	for _, entry := range entries {
		path := filepath.Join(dir, entry.Name())
		if entry.Type().IsRegular() && keep(path) {
			paths = append(paths, path)
		}
	}
	return paths, nil
}
