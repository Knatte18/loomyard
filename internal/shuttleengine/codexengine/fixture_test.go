// fixture_test.go builds the on-disk fixtures the engine's tests run against: a fake repository with linked worktrees and plain clones, a records checkout beside each worktree, an operator Codex home, and the codex.yaml local layer that points the engine at them.
// No git process runs: the `.git` entries are written as git writes them.

package codexengine

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/Knatte18/loomyard/internal/configengine"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
)

// repoFixture is a fake repository's main checkout and the two Codex homes, all under one resolved temp dir.
type repoFixture struct {
	base         string
	mainRoot     string
	lymHome      string
	operatorHome string
}

// newRepoFixture writes a main checkout with a `.git` directory, and an operator home holding auth.json and sessions; the lym home is left unmade.
func newRepoFixture(t *testing.T) repoFixture {
	t.Helper()
	base, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatalf("resolve temp dir: %v", err)
	}
	repo := repoFixture{
		base:         base,
		mainRoot:     filepath.Join(base, "main"),
		lymHome:      filepath.Join(base, "lym-home"),
		operatorHome: filepath.Join(base, "operator-home"),
	}
	gitDir := filepath.Join(repo.mainRoot, ".git")
	mkdirAll(t, filepath.Join(gitDir, "objects"), filepath.Join(gitDir, "refs"), filepath.Join(repo.operatorHome, sessionsDirName))
	writeFile(t, filepath.Join(gitDir, "HEAD"), "ref: refs/heads/main\n")
	writeFile(t, filepath.Join(repo.operatorHome, authFileName), "{}")
	return repo
}

// addWorktree writes a linked worktree named name with a records checkout beside it.
// It also writes a codex.yaml local layer pointing at the fixture's homes, and returns the worktree's root and the records checkout.
func (r repoFixture) addWorktree(t *testing.T, name string) (anchor, records string) {
	t.Helper()
	anchor = filepath.Join(r.base, name)
	records = filepath.Join(r.base, name+"-records")
	gitDir := filepath.Join(r.mainRoot, ".git", "worktrees", name)
	mkdirAll(t, gitDir, anchor, records)
	writeFile(t, filepath.Join(gitDir, "HEAD"), "ref: refs/heads/"+name+"\n")
	writeFile(t, filepath.Join(gitDir, "commondir"), "../..\n")
	writeFile(t, filepath.Join(anchor, ".git"), "gitdir: "+gitDir+"\n")
	r.writeCodexLayer(t, anchor, "")
	return anchor, records
}

// addClone writes a plain clone named name, whose `.git` is a directory of its own and which has no main checkout,
// and a codex.yaml local layer pointing at the fixture's homes, and returns the clone's root.
func (r repoFixture) addClone(t *testing.T, name string) string {
	t.Helper()
	anchor := filepath.Join(r.base, name)
	gitDir := filepath.Join(anchor, ".git")
	mkdirAll(t, filepath.Join(gitDir, "objects"), filepath.Join(gitDir, "refs"))
	writeFile(t, filepath.Join(gitDir, "HEAD"), "ref: refs/heads/main\n")
	r.writeCodexLayer(t, anchor, "")
	return anchor
}

// writeCodexLayer writes anchor's codex.yaml local layer: the fixture's homes, then extra.
func (r repoFixture) writeCodexLayer(t *testing.T, anchor, extra string) {
	t.Helper()
	r.writeCodexConfig(t, configengine.LocalFile(anchor, "codex"), extra)
}

// writeCodexConfig writes the codex.yaml layer at path: the fixture's homes, then extra.
func (r repoFixture) writeCodexConfig(t *testing.T, path, extra string) {
	t.Helper()
	mkdirAll(t, filepath.Dir(path))
	writeFile(t, path, fmt.Sprintf("home: %s\noperator_home: %s\n%s", r.lymHome, r.operatorHome, extra))
}

// warnLog collects the warnings of one engine.
type warnLog struct {
	mu    sync.Mutex
	lines []string
}

// warn records msg and its arguments as one line.
func (w *warnLog) warn(msg string, args ...any) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.lines = append(w.lines, strings.TrimSpace(fmt.Sprintln(append([]any{msg}, args...)...)))
}

// String returns every warning, one per line.
func (w *warnLog) String() string {
	w.mu.Lock()
	defer w.mu.Unlock()
	return strings.Join(w.lines, "\n")
}

// newEngine returns an engine anchored at anchor whose warnings the returned log collects.
func newEngine(anchor string) (*Codex, *warnLog) {
	log := &warnLog{}
	engine := NewFromConfig(shuttleengine.Config{SubmitRedrawSettleMS: defaultSubmitRedrawSettleMS}, configengine.Dirs{Anchor: anchor})
	engine.warn = log.warn
	return engine, log
}

// mkdirAll creates every directory of dirs.
func mkdirAll(t *testing.T, dirs ...string) {
	t.Helper()
	for _, dir := range dirs {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", dir, err)
		}
	}
}

// writeFile writes content to path.
func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}
