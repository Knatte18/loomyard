// template.go builds hubforge's per-binary hub templates and hands them out: CopyHub relocates a template into a test's own temp directory, SharedHub returns the template itself for a test that only reads.
// A template is a fully-wired hub built once per test binary and per shape; the fingerprint taken at build time is how a write to a shared template is caught.

package hubforge

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/fslink"
	"github.com/Knatte18/loomyard/internal/gitkit"
	"github.com/Knatte18/loomyard/internal/lyxcwd"
)

// TemplatePairSlug is the slug of the one pair a pair-carrying Shape's template is built with.
const TemplatePairSlug = "pair"

// templatePrefix begins the last element of every template root, a name no tb.TempDir path element carries, so no copy path has a template root as a string prefix.
const templatePrefix = "hubforge-template-"

// Shape is the hub a template is built as: the anchor ("." or "backend") plus the ordered slugs of the pairs added to it.
type Shape struct {
	Anchor string
	Pairs  []string
}

// name is the fixture name failures cite: the anchor, a plus sign and the comma-joined pair slugs.
func (s Shape) name() string {
	return s.Anchor + "+" + strings.Join(s.Pairs, ",")
}

// Shapes returns the closed set of shapes CopyHub and SharedHub accept: the "." and "backend" anchors, each with no pair and with the one pair TemplatePairSlug.
func Shapes() []Shape {
	var shapes []Shape
	for _, anchor := range []string{".", "backend"} {
		shapes = append(shapes, Shape{Anchor: anchor}, Shape{Anchor: anchor, Pairs: []string{TemplatePairSlug}})
	}
	return shapes
}

// hubTemplate is one built template: the root holding the bares and the hub, the Hub as built there, the prime cwd the Hub's Location resolves from, the build-time fingerprint, and whether a write has since been seen.
type hubTemplate struct {
	name        string
	root        string
	hub         *Hub
	primeCwd    string
	fingerprint string
	poisoned    atomic.Bool
}

// templates caches the built templates by shape name; templatesMu guards it and serialises the builds.
var (
	templates   = map[string]*hubTemplate{}
	templatesMu sync.Mutex
)

// templateFor returns shape's template, building it on first use in a directory that lives as long as the test binary.
// A shape outside Shapes is a tb.Fatalf pointing at NewHub.
func templateFor(tb testing.TB, shape Shape) *hubTemplate {
	tb.Helper()

	templatesMu.Lock()
	defer templatesMu.Unlock()

	name := shape.name()
	known := false
	for _, candidate := range Shapes() {
		known = known || candidate.name() == name
	}
	if !known {
		tb.Fatalf("hubforge: shape %q is outside Shapes(); build such a hub with NewHub", name)
	}
	if template, ok := templates[name]; ok {
		return template
	}

	defer markFixtureBuild()()

	root, err := os.MkdirTemp("", templatePrefix+"*")
	if err != nil {
		tb.Fatalf("hubforge: template %q: mkdir: %v", name, err)
	}
	root, err = filepath.EvalSymlinks(root)
	if err != nil {
		tb.Fatalf("hubforge: template %q: resolve root: %v", name, err)
	}

	templateWarp, templateWeft := buildBareTemplate()
	warpBare := filepath.Join(root, "bares", "warp-bare")
	weftBare := filepath.Join(root, "bares", "weft-bare")
	if err := copyDirRecursive(templateWarp, warpBare); err != nil {
		tb.Fatalf("hubforge: template %q: copy warp bare: %v", name, err)
	}
	if err := copyDirRecursive(templateWeft, weftBare); err != nil {
		tb.Fatalf("hubforge: template %q: copy weft bare: %v", name, err)
	}
	container := filepath.Join(root, "container")
	if err := os.Mkdir(container, 0o755); err != nil {
		tb.Fatalf("hubforge: template %q: mkdir container: %v", name, err)
	}

	hub, primeCwd := cloneHub(tb, shape.Anchor, warpBare, weftBare, container)
	for _, slug := range shape.Pairs {
		AddPair(tb, hub, slug)
	}

	built, err := fingerprint(root)
	if err != nil {
		tb.Fatalf("hubforge: template %q: fingerprint: %v", name, err)
	}

	template := &hubTemplate{name: name, root: root, hub: hub, primeCwd: primeCwd, fingerprint: built}
	templates[name] = template
	return template
}

// requireIntact fails tb, naming the fixture, when the template was poisoned by an earlier write or its tree no longer matches the build-time fingerprint.
func (template *hubTemplate) requireIntact(tb testing.TB) {
	tb.Helper()

	if template.poisoned.Load() {
		tb.Fatalf("hubforge: fixture %q was written to by an earlier test and is poisoned; take CopyHub in the test that writes", template.name)
	}
	current, err := fingerprint(template.root)
	if err != nil {
		tb.Fatalf("hubforge: fixture %q: fingerprint: %v", template.name, err)
	}
	if current != template.fingerprint {
		template.poisoned.Store(true)
		tb.Fatalf("hubforge: fixture %q changed since it was built; a test wrote to the shared hub, so every user of this fixture is suspect", template.name)
	}
}

// CopyHub returns a hub of shape that is the test's own: the shape's template relocated into tb's temp directory, with every path inside it rewritten to the copy's.
// Every checkout of the copy is clean against its HEAD and upstream, as a fresh hub's is.
// A write to the copy touches nothing shared.
// It fails tb, naming the fixture, when the template is poisoned or its tree changed since it was built.
func CopyHub(tb testing.TB, shape Shape) *Hub {
	tb.Helper()

	template := templateFor(tb, shape)
	template.requireIntact(tb)

	root, err := filepath.EvalSymlinks(tb.TempDir())
	if err != nil {
		tb.Fatalf("CopyHub: resolve temp dir: %v", err)
	}
	if err := relocateHub(template.root, root); err != nil {
		tb.Fatalf("CopyHub: fixture %q: %v", template.name, err)
	}
	hub := hubAt(tb, root, shape)
	commitRelocatedBinding(tb, hub.BoardDir())
	return hub
}

// commitRelocatedBinding folds the board checkout's relocated `.lyx-warp` binding into the board's one commit and force-pushes it to the copy's own records bare.
// The relocation rewrites the binding's working-tree file, but the committed blob, compressed in the object store, still names the template's warp bare;
// amending the commit leaves the board clean against its HEAD and its upstream, with the one-commit history a fresh hub's board has.
func commitRelocatedBinding(tb testing.TB, boardDir string) {
	tb.Helper()

	gitkit.Git(tb, boardDir, "add", fabricengine.WarpBindingFileName)
	gitkit.Git(tb, boardDir, "commit", "--amend", "--no-edit", "--quiet")
	gitkit.Git(tb, boardDir, "push", "--force", "--quiet", "origin", "HEAD")
}

// SharedHub returns the template's own hub for a test that only reads it: a file read, a link read or an in-process go-git read.
// Any other access is a write, and the test's cleanup then fails naming the fixture, poisons the template and says the blame may land on a parallel sharer.
// It fails tb, naming the fixture, when the template is poisoned or its tree changed since it was built.
func SharedHub(tb testing.TB, shape Shape) *Hub {
	tb.Helper()

	template := templateFor(tb, shape)
	template.requireIntact(tb)

	tb.Cleanup(func() {
		current, err := fingerprint(template.root)
		if err != nil {
			tb.Errorf("hubforge: fixture %q: fingerprint after the test: %v", template.name, err)
			return
		}
		if current != template.fingerprint {
			template.poisoned.Store(true)
			tb.Errorf("hubforge: shared fixture %q was written to; the blame may land on a parallel sharer, and the fixture is poisoned for the rest of the binary", template.name)
		}
	})
	return template.hub
}

// hubAt builds the Hub for a relocated template at root, rebasing every path of the template's own Hub, and registers the junction teardown NewHub registers.
func hubAt(tb testing.TB, root string, shape Shape) *Hub {
	tb.Helper()

	template := templateFor(tb, shape)
	rebase := func(path string) string {
		rel, err := filepath.Rel(template.root, path)
		if err != nil {
			tb.Fatalf("hubAt: %s is outside the template root %s: %v", path, template.root, err)
		}
		return filepath.Join(root, rel)
	}

	loc, err := lyxcwd.Resolve(rebase(template.primeCwd))
	if err != nil {
		tb.Fatalf("hubAt: lyxcwd.Resolve(%s): %v", rebase(template.primeCwd), err)
	}

	registerTeardown(tb, rebase(template.hub.Path))

	return &Hub{
		Path:        rebase(template.hub.Path),
		Anchor:      template.hub.Anchor,
		Location:    loc,
		Topology:    fabricengine.NewTopology(fabricengine.Config{}),
		CodeBare:    rebase(template.hub.CodeBare),
		RecordsBare: rebase(template.hub.RecordsBare),
		WeftBase:    rebase(template.hub.WeftBase),
		Container:   rebase(template.hub.Container),
		Mutations:   template.hub.Mutations,
	}
}

// relocateHub copies the tree at src into the existing, empty dst and rewrites it to stand at dst.
// Regular files are copied without following links, and the src path, in both separator spellings, is replaced by dst in every text file.
// Each link is recreated, after the files so a junction's target exists, with its target rewritten the same way.
// It fails when any file or link of the copy still names src.
func relocateHub(src, dst string) error {
	replacer := pathReplacer(src, dst)

	type link struct{ path, target string }
	var links []link

	err := filepath.WalkDir(src, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		if rel == "." {
			return nil
		}
		copyPath := filepath.Join(dst, rel)

		isLink, err := fslink.IsLink(path)
		if err != nil {
			return err
		}
		if isLink {
			target, err := fslink.RawTarget(path)
			if err != nil {
				return err
			}
			links = append(links, link{path: copyPath, target: replacer.Replace(target)})
			return nil
		}

		info, err := d.Info()
		if err != nil {
			return err
		}
		if d.IsDir() {
			return os.MkdirAll(copyPath, info.Mode().Perm()|0o700)
		}
		if !info.Mode().IsRegular() {
			return fmt.Errorf("%s is neither a regular file, a directory nor a link", path)
		}

		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if !bytes.Contains(data, []byte{0}) {
			data = []byte(replacer.Replace(string(data)))
		}
		if err := os.WriteFile(copyPath, data, info.Mode().Perm()); err != nil {
			return err
		}
		return os.Chmod(copyPath, info.Mode().Perm())
	})
	if err != nil {
		return fmt.Errorf("relocate %s to %s: %w", src, dst, err)
	}

	for _, l := range links {
		if err := fslink.CreateDirLink(l.path, l.target); err != nil {
			return fmt.Errorf("relocate %s to %s: %w", src, dst, err)
		}
	}

	stale, err := findRootReference(dst, src)
	if err != nil {
		return fmt.Errorf("verify relocation of %s to %s: %w", src, dst, err)
	}
	if stale != "" {
		return fmt.Errorf("relocation of %s to %s left %s naming the template root", src, dst, stale)
	}
	return nil
}

// pathReplacer rewrites src to dst in both separator spellings.
func pathReplacer(src, dst string) *strings.Replacer {
	pairs := []string{src, dst}
	if filepath.ToSlash(src) != src {
		pairs = append(pairs, filepath.ToSlash(src), filepath.ToSlash(dst))
	}
	return strings.NewReplacer(pairs...)
}

// findRootReference returns the first file or link under dir whose content or target names root, in either separator spelling, or "" when none does.
func findRootReference(dir, root string) (string, error) {
	needles := [][]byte{[]byte(root)}
	if filepath.ToSlash(root) != root {
		needles = append(needles, []byte(filepath.ToSlash(root)))
	}
	names := func(data []byte) bool {
		for _, needle := range needles {
			if bytes.Contains(data, needle) {
				return true
			}
		}
		return false
	}

	found := ""
	err := filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		isLink, err := fslink.IsLink(path)
		if err != nil {
			return err
		}
		if isLink {
			target, err := fslink.RawTarget(path)
			if err != nil {
				return err
			}
			if names([]byte(target)) {
				found = path
				return filepath.SkipAll
			}
			return nil
		}
		if !d.Type().IsRegular() {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if names(data) {
			found = path
			return filepath.SkipAll
		}
		return nil
	})
	return found, err
}

// fingerprint is a digest of the sorted list of every regular file under root with its relative path, size and modification time.
// A link is not followed and not listed.
func fingerprint(root string) (string, error) {
	var entries []string
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.Type().IsRegular() {
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		entries = append(entries, fmt.Sprintf("%s\x00%d\x00%d", filepath.ToSlash(rel), info.Size(), info.ModTime().UnixNano()))
		return nil
	})
	if err != nil {
		return "", err
	}

	sort.Strings(entries)
	sum := sha256.Sum256([]byte(strings.Join(entries, "\n")))
	return hex.EncodeToString(sum[:]), nil
}
