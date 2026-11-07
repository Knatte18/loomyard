// cli_integration_test.go drives `lym heddle` through the command tree over hubforge hubs.
// It covers the entry verbs and their refusals, and the Bolt write lock and sync around a write.
// It also covers the hub-resolution refusals, and render's worktree choice, exclude seeding and warnings.

package heddlecli

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/clihelp"
	"github.com/Knatte18/loomyard/internal/configengine"
	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/gitexec"
	"github.com/Knatte18/loomyard/internal/heddleengine"
	"github.com/Knatte18/loomyard/internal/hubforge"
	"github.com/Knatte18/loomyard/internal/lock"
	"github.com/Knatte18/loomyard/internal/lymcwd"
	"github.com/Knatte18/loomyard/internal/testkit/envelope"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// excludedPaths are the paths fabric's exclude list names for a worktree root.
var excludedPaths = []string{"CLAUDE.local.md", "AGENTS.override.md", ".heddle"}

// verbRun is one verb run: its raw output and exit code.
type verbRun struct {
	out  string
	code int
}

// runVerb runs args through c's command tree from cwd, with stdin as the verb's input.
func runVerb(c *heddleCLI, cwd, stdin string, args ...string) verbRun {
	var out bytes.Buffer
	cmd := commandOf(c)
	cmd.SetIn(strings.NewReader(stdin))
	code := clihelp.ExecuteIn(cmd, cwd, &out, args)
	return verbRun{out: out.String(), code: code}
}

// recorder is a pair of seams that record their calls.
type recorder struct {
	saves     int
	syncs     []string
	syncError error
}

// cli returns a receiver whose seams record into r; save passes through to the real store write.
func (r *recorder) cli() *heddleCLI {
	return &heddleCLI{
		save: func(boltDir string, store heddleengine.Store) error {
			r.saves++
			return heddleengine.Save(boltDir, store)
		},
		spawnSync: func(boltDir string) error {
			r.syncs = append(r.syncs, boltDir)
			return r.syncError
		},
	}
}

// primeCwd returns the directory a verb runs from in the hub's prime.
func primeCwd(h *hubforge.Hub) string {
	return h.Location.AnchorPath()
}

func requireOK(t *testing.T, run verbRun) envelope.Envelope {
	t.Helper()
	env := envelope.Decode(t, run.out)
	if !env.OK || run.code != 0 {
		t.Fatalf("exit %d, output %s; want ok", run.code, run.out)
	}
	return env
}

func requireRefusal(t *testing.T, run verbRun, wants ...string) {
	t.Helper()
	env := envelope.Decode(t, run.out)
	if env.OK || run.code != 1 {
		t.Fatalf("exit %d, output %s; want a refusal with exit 1", run.code, run.out)
	}
	for _, want := range wants {
		if !strings.Contains(env.Error, want) {
			t.Errorf("refusal %q; want it to contain %q", env.Error, want)
		}
	}
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile(%s): %v", path, err)
	}
}

func git(t *testing.T, dir string, args ...string) string {
	t.Helper()
	out, err := gitexec.Run(args, dir)
	if err != nil {
		t.Fatalf("git %v in %s: %v", args, dir, err)
	}
	return out
}

// excludeFile returns the path of the exclude file the repository checked out at dir shares across its worktrees.
func excludeFile(t *testing.T, dir string) string {
	t.Helper()
	return strings.TrimSpace(git(t, dir, "rev-parse", "--path-format=absolute", "--git-path", "info/exclude"))
}

// stripExcludeLines removes the exclude lines fabric seeded for the rendered paths,
// so the prime is in the state of one that predates the list, which lacks them.
func stripExcludeLines(t *testing.T, dir string) {
	t.Helper()
	path := excludeFile(t, dir)
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	var kept []string
	for _, line := range strings.Split(string(data), "\n") {
		keep := true
		for _, excluded := range excludedPaths {
			if strings.TrimSpace(line) == "/"+excluded {
				keep = false
			}
		}
		if keep {
			kept = append(kept, line)
		}
	}
	writeFile(t, path, strings.Join(kept, "\n"))
}

// seedStore saves a store holding the entries into the hub's Bolt checkout.
func seedStore(t *testing.T, h *hubforge.Hub, entries map[string][]string) {
	t.Helper()
	var store heddleengine.Store
	order := 0
	for slug, targets := range entries {
		order++
		if err := store.Set(slug, heddleengine.Patch{Text: ptr("text of " + slug), Order: &order, Targets: targets}); err != nil {
			t.Fatalf("Set(%s): %v", slug, err)
		}
	}
	if err := heddleengine.Save(fabricengine.BoltDir(h.Path), store); err != nil {
		t.Fatalf("Save: %v", err)
	}
}

func ptr[T any](v T) *T { return &v }

// shippedSlugs returns the slugs of the entries lym ships, in render order.
func shippedSlugs() []string {
	var slugs []string
	for _, entry := range (heddleengine.Layers{}).Ordered() {
		slugs = append(slugs, entry.Slug)
	}
	return slugs
}

// listedWithShipped returns the slugs list prints for a store holding the repository slugs: the shipped ones first.
func listedWithShipped(repository ...string) string {
	return strings.Join(append(shippedSlugs(), repository...), ",")
}

// listedEntry returns the entry list prints under slug, failing when none is listed.
func listedEntry(t *testing.T, env envelope.Envelope, slug string) map[string]any {
	t.Helper()
	list, _ := env.Raw["entries"].([]any)
	for _, item := range list {
		if entry, _ := item.(map[string]any); entry["slug"] == slug {
			return entry
		}
	}
	t.Fatalf("list = %v; want an entry %q", list, slug)
	return nil
}

// setHubOverride writes the hub's heddle.yaml with lym_override set to on.
func setHubOverride(t *testing.T, h *hubforge.Hub, on bool) {
	t.Helper()
	path := configengine.ConfigFile(fabricengine.BoltDir(h.Path), "heddle")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	value := "false"
	if on {
		value = "true"
	}
	writeFile(t, path, heddleengine.LymOverrideKey+": "+value+"\n")
}

func entrySlugs(t *testing.T, env envelope.Envelope) []string {
	t.Helper()
	list, _ := env.Raw["entries"].([]any)
	var slugs []string
	for _, item := range list {
		entry, _ := item.(map[string]any)
		if _, hasText := entry["text"]; hasText {
			t.Errorf("list entry %v carries its text; want none", entry)
		}
		slug, _ := entry["slug"].(string)
		slugs = append(slugs, slug)
	}
	return slugs
}

func TestHeddleCLI(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()

	t.Run("EntryVerbs", func(t *testing.T) {
		t.Parallel()
		h := hubforge.CopyHub(t, hubforge.Shape{Anchor: "."})
		cwd := primeCwd(h)
		rec := &recorder{}
		c := rec.cli()

		textFile := filepath.Join(t.TempDir(), "rules.md")
		writeFile(t, textFile, "rules from a file")
		requireOK(t, runVerb(c, cwd, "", "set", "rules", "--text-file", textFile, "--order", "20", "--targets", "claude-file"))
		requireOK(t, runVerb(c, cwd, "text from stdin", "set", "intro", "--text-file", "-", "--order", "10", "--targets", "claude-prompt,claude-file"))

		overview, err := os.ReadFile(filepath.Join(fabricengine.BoltDir(h.Path), heddleengine.OverviewFileName))
		if err != nil {
			t.Fatalf("read %s: %v; want a set to render it beside heddle.json", heddleengine.OverviewFileName, err)
		}
		for _, want := range []string{"### `rules`\n", "### `intro`\n", "rules from a file"} {
			if !strings.Contains(string(overview), want) {
				t.Errorf("%s after two sets lacks %q", heddleengine.OverviewFileName, want)
			}
		}

		listed := requireOK(t, runVerb(c, cwd, "", "list"))
		if got, want := strings.Join(entrySlugs(t, listed), ","), listedWithShipped("intro", "rules"); got != want {
			t.Errorf("list slugs = %q; want %q, the shipped entries first, in render order", got, want)
		}
		if layer := listedEntry(t, listed, "rules")["layer"]; layer != heddleengine.LayerRepository {
			t.Errorf("list layer of rules = %v; want %s", layer, heddleengine.LayerRepository)
		}

		got := requireOK(t, runVerb(c, cwd, "", "get", "rules"))
		entry, _ := got.Raw["entry"].(map[string]any)
		if entry["text"] != "rules from a file" || entry["order"] != float64(20) {
			t.Errorf("get rules = %v; want the file's text at order 20", entry)
		}

		requireOK(t, runVerb(c, cwd, "", "set", "rules", "--order", "30"))
		got = requireOK(t, runVerb(c, cwd, "", "get", "rules"))
		entry, _ = got.Raw["entry"].(map[string]any)
		if entry["text"] != "rules from a file" || entry["order"] != float64(30) || len(entry["targets"].([]any)) != 1 {
			t.Errorf("get rules after a partial set = %v; want only the order changed", entry)
		}

		requireOK(t, runVerb(c, cwd, "", "remove", "intro"))
		listed = requireOK(t, runVerb(c, cwd, "", "list"))
		if got, want := strings.Join(entrySlugs(t, listed), ","), listedWithShipped("rules"); got != want {
			t.Errorf("list slugs after remove = %q; want %q", got, want)
		}
		if len(rec.syncs) != 4 {
			t.Errorf("syncs = %d; want one per write", len(rec.syncs))
		}
	})

	t.Run("EntryCollidingWithAShippedSlugIsListedAndRemoved", func(t *testing.T) {
		t.Parallel()
		h := hubforge.CopyHub(t, hubforge.Shape{Anchor: "."})
		cwd := primeCwd(h)
		c := (&recorder{}).cli()
		shipped := shippedSlugs()[0]
		colliding := heddleengine.Store{Entries: []heddleengine.Entry{{Slug: shipped, Text: "mine", Order: 1, Targets: []string{"claude-file"}}}}
		if err := heddleengine.Save(fabricengine.BoltDir(h.Path), colliding); err != nil {
			t.Fatalf("Save: %v", err)
		}

		listed := requireOK(t, runVerb(c, cwd, "", "list"))
		if got, want := strings.Join(entrySlugs(t, listed), ","), listedWithShipped(shipped); got != want {
			t.Errorf("list slugs = %q; want %q, the shipped entry and the colliding one", got, want)
		}
		got := requireOK(t, runVerb(c, cwd, "", "get", shipped))
		shadowed, _ := got.Raw["shadowed"].([]any)
		if len(shadowed) != 1 || shadowed[0].(map[string]any)["text"] != "mine" {
			t.Errorf("get %s shadowed = %v; want the colliding repository entry with its text", shipped, shadowed)
		}
		requireOK(t, runVerb(c, cwd, "", "set", shipped, "--order", "5"))
		requireOK(t, runVerb(c, cwd, "", "remove", shipped))
		if got, want := strings.Join(entrySlugs(t, requireOK(t, runVerb(c, cwd, "", "list"))), ","), listedWithShipped(); got != want {
			t.Errorf("list slugs after remove = %q; want %q", got, want)
		}
		requireRefusal(t, runVerb(c, cwd, "", "remove", shipped), "ships with lym")
	})

	t.Run("MalformedOverrideFileIsWarnedOfAndListStillWorks", func(t *testing.T) {
		t.Parallel()
		h := hubforge.CopyHub(t, hubforge.Shape{Anchor: "."})
		cwd := primeCwd(h)
		c := (&recorder{}).cli()
		setHubOverride(t, h, true)
		writeFile(t, filepath.Join(fabricengine.BoltDir(h.Path), heddleengine.OverrideFileName), `{"entries": [`)

		listed := requireOK(t, runVerb(c, cwd, "", "list"))
		if got, want := strings.Join(entrySlugs(t, listed), ","), listedWithShipped(); got != want {
			t.Errorf("list slugs = %q; want %q", got, want)
		}
		warnings, _ := listed.Raw["warnings"].([]any)
		if len(warnings) != 1 || !strings.Contains(warnings[0].(string), heddleengine.OverrideFileName) {
			t.Errorf("list warnings = %v; want one naming %s", warnings, heddleengine.OverrideFileName)
		}
	})

	t.Run("OverrideVerbs", func(t *testing.T) {
		t.Parallel()
		h := hubforge.CopyHub(t, hubforge.Shape{Anchor: "."})
		cwd := primeCwd(h)
		rec := &recorder{}
		c := rec.cli()
		shipped := shippedSlugs()[0]
		overridePath := filepath.Join(fabricengine.BoltDir(h.Path), heddleengine.OverrideFileName)

		requireRefusal(t, runVerb(c, cwd, "t", "set", shipped, "--text-file", "-", "--order", "1", "--targets", "claude-file"), "ships with lym", "--layer lym")
		requireRefusal(t, runVerb(c, cwd, "", "remove", shipped), "ships with lym")
		requireRefusal(t, runVerb(c, cwd, "tuned", "set", shipped, "--layer", "lym", "--text-file", "-"), "lym config heddle --set lym_override=true")
		requireRefusal(t, runVerb(c, cwd, "tuned", "set", shipped, "--layer", "nope", "--text-file", "-"), "--layer lym")
		if _, err := os.Stat(overridePath); !errors.Is(err, os.ErrNotExist) {
			t.Errorf("stat %s: %v; want no override saved after refusals", overridePath, err)
		}

		setHubOverride(t, h, true)
		requireRefusal(t, runVerb(c, cwd, "tuned", "set", shipped, "--layer", "lym", "--text-file", "-", "--order", "3"), "--text-file only")
		requireRefusal(t, runVerb(c, cwd, "", "set", shipped, "--layer", "lym"), "--text-file")
		requireRefusal(t, runVerb(c, cwd, "tuned", "set", "house-rules", "--layer", "lym", "--text-file", "-"), "lym heddle list")
		if len(rec.syncs) != 0 {
			t.Errorf("syncs = %d; want none after refusals", len(rec.syncs))
		}

		requireOK(t, runVerb(c, cwd, "tuned", "set", shipped, "--layer", "lym", "--text-file", "-"))
		if listed := listedEntry(t, requireOK(t, runVerb(c, cwd, "", "list")), shipped); listed["layer"] != heddleengine.LayerShipped || listed["overridden"] != true {
			t.Errorf("list entry %v; want layer %s, overridden", listed, heddleengine.LayerShipped)
		}
		got := requireOK(t, runVerb(c, cwd, "", "get", shipped))
		if entry, _ := got.Raw["entry"].(map[string]any); entry["text"] != "tuned" {
			t.Errorf("get %s = %v; want the override's text", shipped, entry)
		}

		setHubOverride(t, h, false)
		if listed := listedEntry(t, requireOK(t, runVerb(c, cwd, "", "list")), shipped); listed["overridden"] != nil {
			t.Errorf("list entry %v with the setting off; want it not overridden", listed)
		}
		requireOK(t, runVerb(c, cwd, "", "remove", shipped, "--layer", "lym"))
		requireRefusal(t, runVerb(c, cwd, "", "remove", shipped, "--layer", "lym"), "lym heddle list")
		if len(rec.syncs) != 2 || rec.saves != 0 {
			t.Errorf("syncs = %d, repository saves = %d; want one sync per override write and no heddle.json save", len(rec.syncs), rec.saves)
		}
	})

	t.Run("RefusalsSaveAndSyncNothing", func(t *testing.T) {
		t.Parallel()
		h := hubforge.CopyHub(t, hubforge.Shape{Anchor: "."})
		cwd := primeCwd(h)
		storePath := filepath.Join(fabricengine.BoltDir(h.Path), heddleengine.FileName)
		rec := &recorder{}
		c := rec.cli()

		requireRefusal(t, runVerb(c, cwd, "text", "set", "rules", "--text-file", "-", "--order", "1", "--targets", "no-such-target"), "no-such-target", "way forward")
		requireRefusal(t, runVerb(c, cwd, "", "remove", "missing"), "missing", "lym heddle list")
		requireRefusal(t, runVerb(c, cwd, "", "get", "missing"), "missing", "lym heddle list")
		if _, err := os.Stat(storePath); !errors.Is(err, os.ErrNotExist) {
			t.Errorf("stat %s: %v; want heddle.json absent after only refusals", storePath, err)
		}
		if rec.saves != 0 || len(rec.syncs) != 0 {
			t.Errorf("saves = %d, syncs = %d; want none for a refused change", rec.saves, len(rec.syncs))
		}

		requireRefusal(t, runVerb(c, cwd, "", "show", "no-such-target"), "claude-file")
	})

	t.Run("EmptyTargetsOnExistingEntryIsRefused", func(t *testing.T) {
		t.Parallel()
		h := hubforge.CopyHub(t, hubforge.Shape{Anchor: "."})
		seedStore(t, h, map[string][]string{"rules": {"claude-file"}})
		rec := &recorder{}
		c := rec.cli()

		requireRefusal(t, runVerb(c, primeCwd(h), "", "set", "rules", "--targets", ""), "--targets", "way forward")
		if rec.saves != 0 || len(rec.syncs) != 0 {
			t.Errorf("saves = %d, syncs = %d; want none for a refused change", rec.saves, len(rec.syncs))
		}
	})

	t.Run("SyncFailureKeepsTheSave", func(t *testing.T) {
		t.Parallel()
		h := hubforge.CopyHub(t, hubforge.Shape{Anchor: "."})
		rec := &recorder{syncError: errors.New("no binary to spawn")}
		run := runVerb(rec.cli(), primeCwd(h), "text", "set", "rules", "--text-file", "-", "--order", "1", "--targets", "claude-file")
		env := requireOK(t, run)
		if warning, _ := env.Raw["warning"].(string); !strings.Contains(warning, "lym board sync") {
			t.Errorf("warning = %q; want it to name lym board sync", warning)
		}
		store, err := heddleengine.Load(fabricengine.BoltDir(h.Path))
		if err != nil {
			t.Fatalf("Load: %v", err)
		}
		if _, found := store.Get("rules"); !found {
			t.Errorf("store after a failed sync start lacks the saved entry")
		}
	})

	t.Run("ChangeStoreReturnsErrorsAndWarning", func(t *testing.T) {
		t.Parallel()
		h := hubforge.CopyHub(t, hubforge.Shape{Anchor: "."})
		text, order := "rule", 1
		set := func(store *heddleengine.Store) error {
			return store.Set("rules", heddleengine.Patch{Text: &text, Order: &order, Targets: []string{"claude-file"}})
		}

		outside := &recorder{}
		if _, err := outside.cli().changeStore(t.TempDir(), set); err == nil || !strings.Contains(err.Error(), "run lym heddle from a code worktree of a hub") {
			t.Errorf("changeStore outside a hub error = %v; want the hub-resolution refusal", err)
		}
		if outside.saves != 0 || len(outside.syncs) != 0 {
			t.Errorf("saves = %d, syncs = %d; want none outside a hub", outside.saves, len(outside.syncs))
		}

		refused := &recorder{}
		refusal := errors.New("store refuses")
		if _, err := refused.cli().changeStore(primeCwd(h), func(*heddleengine.Store) error { return refusal }); !errors.Is(err, refusal) {
			t.Errorf("changeStore error = %v; want %v", err, refusal)
		}
		if refused.saves != 0 || len(refused.syncs) != 0 {
			t.Errorf("saves = %d, syncs = %d; want none for a refused change", refused.saves, len(refused.syncs))
		}

		failing := &recorder{syncError: errors.New("no binary to spawn")}
		warning, err := failing.cli().changeStore(primeCwd(h), set)
		if err != nil {
			t.Fatalf("changeStore with a failed sync start error: %v; want nil", err)
		}
		if !strings.Contains(warning, "lym board sync") {
			t.Errorf("changeStore warning = %q; want it to name lym board sync", warning)
		}
		store, err := heddleengine.Load(fabricengine.BoltDir(h.Path))
		if err != nil {
			t.Fatalf("Load: %v", err)
		}
		if _, found := store.Get("rules"); !found {
			t.Errorf("store after a failed sync start lacks the saved entry")
		}
	})

	t.Run("WriteHoldsTheBoltLockAndCommitsNothing", func(t *testing.T) {
		t.Parallel()
		h := hubforge.CopyHub(t, hubforge.Shape{Anchor: "."})
		cwd := primeCwd(h)
		boltDir := fabricengine.BoltDir(h.Path)
		lockPath, err := fabricengine.BoltLockPath(boltDir, fabricengine.BoardWriteLockFile)
		if err != nil {
			t.Fatalf("BoltLockPath: %v", err)
		}
		headBefore := git(t, boltDir, "rev-parse", "HEAD")
		boltStatusBefore := git(t, boltDir, "status", "--porcelain", "--untracked-files=no")
		primeStatusBefore := git(t, cwd, "status", "--porcelain")

		var lockHeldDuringSave, lockFreeDuringSync bool
		syncs := 0
		c := &heddleCLI{
			save: func(dir string, store heddleengine.Store) error {
				probe, acquired, err := lock.TryAcquireWriteLock(lockPath)
				if err == nil && acquired {
					_ = probe.Release()
				}
				lockHeldDuringSave = err == nil && !acquired
				return heddleengine.Save(dir, store)
			},
			spawnSync: func(dir string) error {
				syncs++
				probe, acquired, err := lock.TryAcquireWriteLock(lockPath)
				if err == nil && acquired {
					_ = probe.Release()
				}
				lockFreeDuringSync = err == nil && acquired
				if dir != boltDir {
					t.Errorf("spawnSync(%s); want the Bolt checkout %s", dir, boltDir)
				}
				return nil
			},
		}
		requireOK(t, runVerb(c, cwd, "text", "set", "rules", "--text-file", "-", "--order", "1", "--targets", "claude-file"))

		if !lockHeldDuringSave {
			t.Errorf("the Bolt write lock was free during the save; want it held")
		}
		if syncs != 1 || !lockFreeDuringSync {
			t.Errorf("syncs = %d, lock free during sync = %v; want one sync after the lock is released", syncs, lockFreeDuringSync)
		}
		if head := git(t, boltDir, "rev-parse", "HEAD"); head != headBefore {
			t.Errorf("Bolt HEAD = %s; want %s, no foreground commit", head, headBefore)
		}
		if status := git(t, boltDir, "status", "--porcelain", "--untracked-files=no"); status != boltStatusBefore {
			t.Errorf("Bolt tracked status = %q; want %q", status, boltStatusBefore)
		}
		if status := git(t, cwd, "status", "--porcelain"); status != primeStatusBefore {
			t.Errorf("prime status = %q; want %q, no worktree file changed", status, primeStatusBefore)
		}
	})

	t.Run("HubResolutionRefusals", func(t *testing.T) {
		t.Parallel()
		h := hubforge.CopyHub(t, hubforge.Shape{Anchor: "."})
		rec := &recorder{}
		c := rec.cli()
		boltDir := fabricengine.BoltDir(h.Path)

		requireRefusal(t, runVerb(c, t.TempDir(), "", "list"), "run lym heddle from a code worktree of a hub")

		for name, cwd := range map[string]string{"the Bolt checkout": boltDir, "the prime's records sibling": h.PrimeRecords()} {
			location, err := lymcwd.Resolve(cwd)
			if err != nil {
				t.Fatalf("Resolve(%s): %v", cwd, err)
			}
			want := fabricengine.RequireDrivableWorktree(location)
			if want == nil {
				t.Fatalf("RequireDrivableWorktree(%s) = nil; want a refusal", name)
			}
			requireRefusal(t, runVerb(c, cwd, "", "list"), want.Error())
		}
		if _, err := os.Stat(filepath.Join(boltDir, heddleengine.FileName)); !errors.Is(err, os.ErrNotExist) {
			t.Errorf("stat heddle.json: %v; want it absent after refusals", err)
		}

		moved := boltDir + ".moved"
		if err := os.Rename(boltDir, moved); err != nil {
			t.Fatalf("Rename: %v", err)
		}
		t.Cleanup(func() { _ = os.Rename(moved, boltDir) })
		requireRefusal(t, runVerb(c, primeCwd(h), "", "list"), "Bolt checkout", "lym fabric clone")
	})

	t.Run("RenderChoosesAnExistingWorktree", func(t *testing.T) {
		t.Parallel()
		h := hubforge.CopyHub(t, hubforge.Shape{Anchor: ".", Pairs: []string{hubforge.TemplatePairSlug}})
		cwd := primeCwd(h)
		seedStore(t, h, map[string][]string{"rules": {"claude-file", "codex-file"}})
		stripExcludeLines(t, cwd)
		c := (&recorder{}).cli()

		if err := os.RemoveAll(h.PairCodeWorktree(hubforge.TemplatePairSlug)); err != nil {
			t.Fatalf("RemoveAll: %v", err)
		}
		requireRefusal(t, runVerb(c, cwd, "", "render", "nosuch"), "lym fabric list")
		requireRefusal(t, runVerb(c, cwd, "", "render", hubforge.TemplatePairSlug), "lym fabric list", "git worktree prune")

		exclude, err := os.ReadFile(excludeFile(t, cwd))
		if err != nil {
			t.Fatalf("read exclude: %v", err)
		}
		for _, excluded := range excludedPaths {
			if strings.Contains(string(exclude), excluded) {
				t.Errorf("exclude file holds %s after refused renders; want nothing seeded", excluded)
			}
		}
		for _, name := range []string{"CLAUDE.local.md", "AGENTS.override.md"} {
			if _, err := os.Stat(filepath.Join(h.PrimeWorktree(), name)); !errors.Is(err, os.ErrNotExist) {
				t.Errorf("stat %s: %v; want no file written", name, err)
			}
		}
	})

	t.Run("RenderSeedsAndWritesThePrime", func(t *testing.T) {
		t.Parallel()
		h := hubforge.CopyHub(t, hubforge.Shape{Anchor: "."})
		cwd := primeCwd(h)
		seedStore(t, h, map[string][]string{"rules": {"claude-file", "codex-file"}})
		stripExcludeLines(t, cwd)
		c := (&recorder{}).cli()

		requireOK(t, runVerb(c, cwd, "", "render"))

		exclude, err := os.ReadFile(excludeFile(t, cwd))
		if err != nil {
			t.Fatalf("read exclude: %v", err)
		}
		for _, excluded := range excludedPaths {
			if !strings.Contains(string(exclude), "/"+excluded+"\n") {
				t.Errorf("exclude file lacks /%s after render", excluded)
			}
		}
		for _, name := range []string{"CLAUDE.local.md", "AGENTS.override.md"} {
			data, err := os.ReadFile(filepath.Join(h.PrimeWorktree(), name))
			if err != nil || !strings.HasPrefix(string(data), heddleengine.Marker) {
				t.Errorf("%s = %q, %v; want a rendered file", name, data, err)
			}
		}
	})

	t.Run("RenderWritesFileTargetsAtTheAnchor", func(t *testing.T) {
		t.Parallel()
		h := hubforge.CopyHub(t, hubforge.Shape{Anchor: "backend"})
		cwd := primeCwd(h)
		seedStore(t, h, map[string][]string{"rules": {"claude-file", "codex-file"}})
		c := (&recorder{}).cli()

		requireOK(t, runVerb(c, cwd, "", "render"))

		for _, name := range []string{"CLAUDE.local.md", "AGENTS.override.md"} {
			data, err := os.ReadFile(filepath.Join(h.Location.AnchorPath(), name))
			if err != nil || !strings.HasPrefix(string(data), heddleengine.Marker) {
				t.Errorf("%s at the anchor = %q, %v; want a rendered file", name, data, err)
			}
			if _, err := os.Stat(filepath.Join(h.PrimeWorktree(), name)); !errors.Is(err, os.ErrNotExist) {
				t.Errorf("stat %s at the worktree root: %v; want no file written", name, err)
			}
		}

		writeFile(t, filepath.Join(h.Location.AnchorPath(), "AGENTS.md"), "agents text at the anchor\n")
		run := runVerb(c, cwd, "", "show", "codex-file")
		if run.code != 0 || !strings.Contains(run.out, "agents text at the anchor") {
			t.Errorf("show codex-file: exit %d, output %q; want the AGENTS.md at the anchor in the rendered text", run.code, run.out)
		}
	})

	t.Run("RenderWarnsOfAnUnmarkedFile", func(t *testing.T) {
		t.Parallel()
		h := hubforge.CopyHub(t, hubforge.Shape{Anchor: "."})
		cwd := primeCwd(h)
		seedStore(t, h, map[string][]string{"rules": {"claude-file"}})
		unmarked := filepath.Join(h.PrimeWorktree(), "CLAUDE.local.md")
		writeFile(t, unmarked, "my own notes\n")

		env := requireOK(t, runVerb((&recorder{}).cli(), cwd, "", "render"))
		if warnings, _ := env.Raw["warnings"].([]any); len(warnings) == 0 || !strings.Contains(warnings[0].(string), "CLAUDE.local.md") {
			t.Errorf("warnings = %v; want one naming CLAUDE.local.md", env.Raw["warnings"])
		}
		if data, _ := os.ReadFile(unmarked); string(data) != "my own notes\n" {
			t.Errorf("CLAUDE.local.md = %q; want the unmarked file untouched", data)
		}
	})

	t.Run("RenderRefusesWhileTheBoltLockIsHeld", func(t *testing.T) {
		t.Parallel()
		h := hubforge.CopyHub(t, hubforge.Shape{Anchor: "."})
		cwd := primeCwd(h)
		seedStore(t, h, map[string][]string{"rules": {"claude-file"}})
		boltLockPath, err := fabricengine.BoltLockPath(fabricengine.BoltDir(h.Path), fabricengine.BoardWriteLockFile)
		if err != nil {
			t.Fatalf("BoltLockPath: %v", err)
		}
		held, err := lock.AcquireWriteLock(boltLockPath)
		if err != nil {
			t.Fatalf("AcquireWriteLock: %v", err)
		}
		t.Cleanup(func() { _ = held.Release() })

		c := (&recorder{}).cli()
		c.renderer = heddleengine.Renderer{BoltLockWait: 50 * time.Millisecond}
		requireRefusal(t, runVerb(c, cwd, "", "render"), "lym heddle render")
		if _, err := os.Stat(filepath.Join(h.PrimeWorktree(), "CLAUDE.local.md")); !errors.Is(err, os.ErrNotExist) {
			t.Errorf("stat CLAUDE.local.md: %v; want no file written", err)
		}
	})

	t.Run("ShowWritesNothing", func(t *testing.T) {
		t.Parallel()
		h := hubforge.CopyHub(t, hubforge.Shape{Anchor: "."})
		cwd := primeCwd(h)
		seedStore(t, h, map[string][]string{"rules": {"claude-file"}})
		statusBefore := git(t, cwd, "status", "--porcelain")

		run := runVerb((&recorder{}).cli(), cwd, "", "show", "claude-file")
		if run.code != 0 || !strings.Contains(run.out, "text of rules") {
			t.Errorf("show claude-file: exit %d, output %q; want the rendered text", run.code, run.out)
		}
		if _, err := os.Stat(filepath.Join(h.PrimeWorktree(), "CLAUDE.local.md")); !errors.Is(err, os.ErrNotExist) {
			t.Errorf("stat CLAUDE.local.md: %v; want no file", err)
		}
		if status := git(t, cwd, "status", "--porcelain"); status != statusBefore {
			t.Errorf("prime status = %q; want %q", status, statusBefore)
		}
	})
}
