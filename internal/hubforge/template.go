// template.go builds hubforge's per-binary hub templates and hands them out.
// A template is a fully wired hub built once per test binary and per key:
// a shape of the closed Shapes set, or a Registration a test package declares.
// CopyHub and CopyRegistered relocate a template into a test's own temp directory.
// SharedHub returns a shape's template itself for a test that only reads it.
// The fingerprint taken at build time catches a write to a shared template.

package hubforge

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/fslink"
	"github.com/Knatte18/loomyard/internal/gitkit"
	"github.com/Knatte18/loomyard/internal/lymcwd"
	"github.com/Knatte18/loomyard/internal/lymtemp"
)

// TemplatePairSlug is the slug of the one pair a pair-carrying Shape's template is built with.
const TemplatePairSlug = "pair"

// templatePrefix begins the last element of every template root.
// No tb.TempDir path element carries this prefix, so no copy path has a template root as a string prefix.
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

// Shapes returns the closed set of shapes CopyHub and SharedHub accept:
// the "." and "backend" anchors, each with no pair and with the one pair TemplatePairSlug.
func Shapes() []Shape {
	var shapes []Shape
	for _, anchor := range []string{".", "backend"} {
		shapes = append(shapes, Shape{Anchor: anchor}, Shape{Anchor: anchor, Pairs: []string{TemplatePairSlug}})
	}
	return shapes
}

// Registration is a hub template a test package declares once, as a package-level value, and copies with CopyRegistered.
// Its template is the Base shape's hub with Build run on it, built on first use and once per test binary.
// It holds a start state the closed shape set cannot: commits, pushes and pairs under fixed slugs.
// A registration is identified by its address, and its Key is unique among the binary's registrations and shape names.
type Registration struct {
	// Key names the template in every failure about it.
	Key string
	// Base is the anchor and the ordered pair slugs the hub is built with before Build runs.
	Base Shape
	// Build brings the template's hub to its start state; a nil Build leaves the base hub as it is.
	// It captures no per-call data: a test derives the paths it needs from its copy's Hub,
	// and a start state that varies takes a registration of its own.
	// Its testing.TB fails the build on Cleanup, Setenv, TempDir, Chdir, ArtifactDir or a skip,
	// so no template state is tied to the first caller's lifetime or reaches a path outside the template.
	// It makes no process-wide change the TB cannot see, since every later copy or test in the binary would inherit it:
	// no os.Setenv, os.MkdirTemp, global git config, package-global seam setter or once-only registration.
	// It also reaches no code whose result such a registration changes, such as a Bolt replay.
	// It pushes every commit it makes on the board, and pushes nothing else into the records bare:
	// the build fails when the board's HEAD is not its branch's tip in the records bare.
	Build func(tb testing.TB, h *Hub)
}

// shapeRegistrations holds the registration each shape of Shapes is built through, keyed by the shape's name.
var shapeRegistrations = func() map[string]*Registration {
	registrations := map[string]*Registration{}
	for _, shape := range Shapes() {
		registrations[shape.name()] = &Registration{Key: shape.name(), Base: shape}
	}
	return registrations
}()

// shapeRegistration returns shape's registration.
// A shape outside Shapes fails tb with a message that points at NewHub and Registration.
func shapeRegistration(tb testing.TB, shape Shape) *Registration {
	tb.Helper()

	registration, ok := shapeRegistrations[shape.name()]
	if !ok {
		tb.Fatalf("hubforge: shape %q is outside Shapes(); build such a hub with NewHub or a Registration", shape.name())
	}
	return registration
}

// hubTemplate is one built template: the root that holds the bares and the hub, and the Hub built there.
// primeCwd is the directory the Hub's Location resolves from, and fingerprint is the digest of the tree at build time.
// poisoned is set once a write to the shared tree is seen.
type hubTemplate struct {
	name        string
	root        string
	hub         *Hub
	primeCwd    string
	fingerprint string
	poisoned    atomic.Bool
}

// templateSlot is the cache entry of one key: the registration that owns the key and the template built for it.
// mu serialises the key's build, so templates of different keys build at once.
// failed is set once the build failed, so no later caller rebuilds it.
type templateSlot struct {
	registration *Registration
	mu           sync.Mutex
	template     *hubTemplate
	failed       bool
}

// templateSlots holds one slot per key; templateSlotsMu guards the map only, never a build.
var (
	templateSlots   = map[string]*templateSlot{}
	templateSlotsMu sync.Mutex
)

// slotFor returns registration's slot, adding it on first use.
// It fails tb, naming the key, when another registration already holds the key.
func slotFor(tb testing.TB, registration *Registration) *templateSlot {
	tb.Helper()

	templateSlotsMu.Lock()
	slot, ok := templateSlots[registration.Key]
	if !ok {
		slot = &templateSlot{registration: registration}
		templateSlots[registration.Key] = slot
	}
	templateSlotsMu.Unlock()

	if slot.registration != registration {
		tb.Fatalf("hubforge: two different registrations share the key %q; give each its own key", registration.Key)
	}
	return slot
}

// templateFor returns registration's template.
// It builds the template on first use, in a directory that lives as long as the test binary.
// The caller whose build fails fails with the build's error, naming the key.
// Every later caller fails naming the key, and the template is not built again.
func templateFor(tb testing.TB, registration *Registration) *hubTemplate {
	tb.Helper()

	slot := slotFor(tb, registration)
	slot.mu.Lock()
	defer slot.mu.Unlock()

	if slot.template != nil {
		return slot.template
	}
	if slot.failed {
		tb.Fatalf("hubforge: template %q failed to build for an earlier caller, whose failure holds the build's error; it is not built again", registration.Key)
	}

	template, failures := buildTemplate(tb, registration)
	if len(failures) > 0 {
		slot.failed = true
		tb.Fatalf("hubforge: template %q failed to build: %s", registration.Key, strings.Join(failures, "; "))
	}
	slot.template = template
	return template
}

// buildTemplate builds registration's template on a goroutine of its own, under a templateBuildTB over tb.
// It returns the template, or the failures the build recorded.
func buildTemplate(tb testing.TB, registration *Registration) (*hubTemplate, []string) {
	buildTB := &templateBuildTB{TB: tb}
	var template *hubTemplate
	done := make(chan struct{})
	go func() {
		defer close(done)
		template = buildTemplateIn(buildTB, registration)
	}()
	<-done
	return template, buildTB.recordedFailures()
}

// buildTemplateIn builds registration's template in its own lymtemp root, never from a copy of another template.
// It copies the bares, clones the hub, adds the base shape's pairs and runs Build.
// It then checks the board against its upstream tip and takes the fingerprint.
func buildTemplateIn(tb testing.TB, registration *Registration) *hubTemplate {
	tb.Helper()

	defer markFixtureBuild()()

	root, err := lymtemp.MkdirTemp(templatePrefix + "*")
	if err != nil {
		tb.Fatalf("mkdir: %v", err)
	}
	root, err = filepath.EvalSymlinks(root)
	if err != nil {
		tb.Fatalf("resolve root: %v", err)
	}

	templateWarp, templateWeft := buildBareTemplate()
	warpBare := filepath.Join(root, "bares", "warp-bare")
	weftBare := filepath.Join(root, "bares", "weft-bare")
	if err := copyDirRecursive(templateWarp, warpBare); err != nil {
		tb.Fatalf("copy warp bare: %v", err)
	}
	if err := copyDirRecursive(templateWeft, weftBare); err != nil {
		tb.Fatalf("copy weft bare: %v", err)
	}
	container := filepath.Join(root, "container")
	if err := os.Mkdir(container, 0o755); err != nil {
		tb.Fatalf("mkdir container: %v", err)
	}

	hub, primeCwd := cloneHub(tb, registration.Base.Anchor, warpBare, weftBare, container)
	for _, slug := range registration.Base.Pairs {
		AddPair(tb, hub, slug)
	}
	if registration.Build != nil {
		registration.Build(tb, hub)
	}
	requireBoardAtUpstreamTip(tb, hub)

	built, err := fingerprint(root)
	if err != nil {
		tb.Fatalf("fingerprint: %v", err)
	}
	return &hubTemplate{name: registration.Key, root: root, hub: hub, primeCwd: primeCwd, fingerprint: built}
}

// requireBoardAtUpstreamTip fails tb unless the board's HEAD is the tip of the board's branch in the records bare.
// The tip is read from the bare, never from the board's tracking ref,
// so a commit pushed into the bare from elsewhere counts.
// A copy folds its relocated binding into the board's HEAD and force-pushes it.
// That would drop any commit the bare holds beyond that HEAD.
func requireBoardAtUpstreamTip(tb testing.TB, h *Hub) {
	tb.Helper()

	board := h.BoardDir()
	branch := gitkit.CurrentBranch(tb, board)
	head := gitkit.RevParse(tb, board, "HEAD")
	tip := gitkit.RevParse(tb, h.RecordsBare, "refs/heads/"+branch)
	if head != tip {
		tb.Fatalf("the board's HEAD %s is not the tip %s of %s in the records bare; push every board commit, and push nothing else into the bare", head, tip, branch)
	}
}

// templateBuildTB is the testing.TB a template build runs under, on a goroutine of its own.
// A failure is recorded instead of reported, and a fatal one ends the build's goroutine.
// A call that would tie template state to the calling test, or reach a path outside the template root, fails the build:
// Cleanup, Setenv, TempDir, Chdir, ArtifactDir and every skip.
// Logging and every other call reach the calling test.
type templateBuildTB struct {
	testing.TB
	mu       sync.Mutex
	failures []string
}

func (b *templateBuildTB) Helper() {}

func (b *templateBuildTB) Error(args ...any) {
	b.record(fmt.Sprint(args...))
}

func (b *templateBuildTB) Errorf(format string, args ...any) {
	b.record(fmt.Sprintf(format, args...))
}

func (b *templateBuildTB) Fail() {
	b.record("the build marked itself failed")
}

func (b *templateBuildTB) FailNow() {
	b.Fail()
	runtime.Goexit()
}

func (b *templateBuildTB) Fatal(args ...any) {
	b.Error(args...)
	runtime.Goexit()
}

func (b *templateBuildTB) Fatalf(format string, args ...any) {
	b.Errorf(format, args...)
	runtime.Goexit()
}

func (b *templateBuildTB) Failed() bool {
	return len(b.recordedFailures()) > 0
}

func (b *templateBuildTB) Skip(...any) {
	b.refuse("Skip")
}

func (b *templateBuildTB) Skipf(string, ...any) {
	b.refuse("Skipf")
}

func (b *templateBuildTB) SkipNow() {
	b.refuse("SkipNow")
}

func (b *templateBuildTB) Cleanup(func()) {
	b.refuse("Cleanup")
}

func (b *templateBuildTB) Setenv(string, string) {
	b.refuse("Setenv")
}

func (b *templateBuildTB) TempDir() string {
	b.refuse("TempDir")
	return ""
}

func (b *templateBuildTB) Chdir(string) {
	b.refuse("Chdir")
}

func (b *templateBuildTB) ArtifactDir() string {
	b.refuse("ArtifactDir")
	return ""
}

// refuse fails the build for a call no template build may make.
func (b *templateBuildTB) refuse(call string) {
	b.Fatalf("a template build may not call testing.TB's %s: its result would outlive the build or leave the template root", call)
}

// record adds one failure.
func (b *templateBuildTB) record(failure string) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.failures = append(b.failures, failure)
}

// recordedFailures returns the failures recorded so far.
func (b *templateBuildTB) recordedFailures() []string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return append([]string(nil), b.failures...)
}

// requireIntact fails tb, naming the fixture, when an earlier write poisoned the template.
// It also fails when the tree no longer matches the build-time fingerprint.
func (template *hubTemplate) requireIntact(tb testing.TB) {
	tb.Helper()

	if template.poisoned.Load() {
		tb.Fatalf("hubforge: fixture %q was written to by an earlier test and is poisoned; take CopyHub or CopyRegistered in the test that writes", template.name)
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

// CopyHub returns a hub of shape that the test owns.
// It is the shape's template relocated into tb's temp directory, with every path inside it rewritten to the copy's.
// Every checkout of the copy is clean against its HEAD and upstream, as a fresh hub's is.
// A write to the copy touches nothing shared.
// It fails tb, naming the fixture, when the template failed to build, is poisoned or changed since it was built.
func CopyHub(tb testing.TB, shape Shape) *Hub {
	tb.Helper()

	return copyTemplate(tb, templateFor(tb, shapeRegistration(tb, shape)))
}

// CopyRegistered returns a hub that the test owns, holding the commits, pushes and pairs of registration's build.
// It is the registration's template relocated into tb's temp directory.
// Every path inside it is rewritten to the copy's.
// The board's binding is folded into its HEAD.
// Every checkout of the copy is clean against its HEAD and upstream.
// A write to the copy touches nothing shared.
// It fails tb, naming the key, when the template failed to build, for this caller or an earlier one.
// It also fails when another registration holds the key.
// It also fails when the template is poisoned or changed since it was built.
func CopyRegistered(tb testing.TB, registration *Registration) *Hub {
	tb.Helper()

	return copyTemplate(tb, templateFor(tb, registration))
}

// copyTemplate relocates template into tb's temp directory and returns the copy's Hub.
// It fails tb, naming the fixture, when the template is poisoned or its tree changed since it was built.
func copyTemplate(tb testing.TB, template *hubTemplate) *Hub {
	tb.Helper()

	template.requireIntact(tb)

	root, err := filepath.EvalSymlinks(tb.TempDir())
	if err != nil {
		tb.Fatalf("hubforge: copy of %q: resolve temp dir: %v", template.name, err)
	}
	if err := relocateHub(template.root, root); err != nil {
		tb.Fatalf("hubforge: copy of %q: %v", template.name, err)
	}
	hub := hubAt(tb, template, root)
	commitRelocatedBinding(tb, fabricengine.BoltDir(hub.Path))
	return hub
}

// commitRelocatedBinding amends the board's HEAD commit to fold in the relocated `.lym-warp` binding.
// It then force-pushes that commit to the copy's own records bare.
// The relocation rewrites the binding's working-tree file.
// The committed blob, compressed in the object store, still names the template's warp bare.
// Amending the commit leaves the board clean against its HEAD and upstream.
// It keeps the board's history as long as the template's.
func commitRelocatedBinding(tb testing.TB, boardDir string) {
	tb.Helper()

	gitkit.Git(tb, boardDir, "add", fabricengine.WarpBindingFileName)
	gitkit.Git(tb, boardDir, "commit", "--amend", "--no-edit", "--quiet")
	gitkit.Git(tb, boardDir, "push", "--force", "--quiet", "origin", "HEAD")
}

// SharedHub returns the template's own hub for a test that only reads it:
// a file read, a link read or an in-process go-git read.
// Any other access is a write.
// The test's cleanup then fails naming the fixture and poisons the template.
// The error says the blame may land on a parallel sharer.
// It fails tb, naming the fixture, when the template is poisoned or its tree changed since it was built.
func SharedHub(tb testing.TB, shape Shape) *Hub {
	tb.Helper()

	template := templateFor(tb, shapeRegistration(tb, shape))
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

// hubAt builds the Hub for template relocated to root, rebasing every path of the template's own Hub.
// It also registers the junction teardown for the Hub's path.
func hubAt(tb testing.TB, template *hubTemplate, root string) *Hub {
	tb.Helper()

	rebase := func(path string) string {
		rel, err := filepath.Rel(template.root, path)
		if err != nil {
			tb.Fatalf("hubAt: %s is outside the template root %s: %v", path, template.root, err)
		}
		return filepath.Join(root, rel)
	}

	loc, err := lymcwd.Resolve(rebase(template.primeCwd))
	if err != nil {
		tb.Fatalf("hubAt: lymcwd.Resolve(%s): %v", rebase(template.primeCwd), err)
	}

	RegisterTeardown(tb, rebase(template.hub.Path))

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
// Regular files are copied without following links.
// The src path, in both separator spellings, is replaced by dst in every text file.
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

// findRootReference returns the first file or link under dir whose content or link target names root.
// It matches root in either separator spelling, and returns "" when none does.
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

// fingerprint returns a digest of the sorted entries of every regular file under root.
// Each entry holds the file's relative path, size and modification time.
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
