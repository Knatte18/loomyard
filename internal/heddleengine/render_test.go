//go:build tier0

// render_test.go drives Render, Show and Passages over temporary directories standing in for a worktree's root, its scratch directory and the Bolt checkout.

package heddleengine

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/configengine"
	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/lock"
	"github.com/Knatte18/loomyard/internal/lymdirs"
)

// observeWindow is how long a test watches a blocked render stay blocked.
const observeWindow = 50 * time.Millisecond

// renderFixture is the three directories a render works over, with the exclude list a seeded worktree holds.
type renderFixture struct {
	root     string
	scratch  string
	bolt     string
	excluded []string
}

func newRenderFixture(t *testing.T, store Store) renderFixture {
	t.Helper()
	fixture := renderFixture{
		root:     t.TempDir(),
		scratch:  t.TempDir(),
		bolt:     t.TempDir(),
		excluded: []string{"CLAUDE.local.md", "AGENTS.override.md", ".heddle"},
	}
	saveStore(t, fixture.bolt, store)
	return fixture
}

func (f renderFixture) request(only ...string) RenderRequest {
	return RenderRequest{Root: f.root, ScratchDir: f.scratch, BoltDir: f.bolt, Excluded: f.excluded, Only: only}
}

func (f renderFixture) writeRoot(t *testing.T, rel, content string) {
	t.Helper()
	path := filepath.Join(f.root, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func (f renderFixture) readRoot(t *testing.T, rel string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(f.root, filepath.FromSlash(rel)))
	if err != nil {
		t.Fatalf("read %s: %v", rel, err)
	}
	return string(data)
}

func (f renderFixture) rootHas(rel string) bool {
	_, err := os.Lstat(filepath.Join(f.root, filepath.FromSlash(rel)))
	return err == nil
}

// claudeFileWant returns the CLAUDE.local.md a render writes: the marker, imports when non-empty, the shipped layer's section, then the repository's texts under their heading when any are given.
func claudeFileWant(imports string, texts ...string) string {
	parts := []string{Marker}
	if imports != "" {
		parts = append(parts, imports)
	}
	parts = append(parts, shippedHeading)
	for _, entry := range shippedStore.Ordered() {
		if slices.Contains(entry.Targets, "claude-file") {
			parts = append(parts, entry.Text)
		}
	}
	if len(texts) > 0 {
		parts = append(append(parts, repositoryHeading), texts...)
	}
	return strings.Join(parts, "\n\n") + "\n"
}

// writeHeddleConfig writes content as the hub's heddle.yaml in the Bolt checkout bolt.
func writeHeddleConfig(t *testing.T, bolt, content string) {
	t.Helper()
	path := configengine.ConfigFile(bolt, configModule)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

// entryFor builds an entry aimed at one or more targets.
func entryFor(slug, text string, order int, targets ...string) Entry {
	return Entry{Slug: slug, Text: text, Order: order, Targets: targets}
}

// renderOutcome is what a render running in a goroutine returned.
type renderOutcome struct {
	report Report
	err    error
}

// renderInBackground starts the render and returns the channel its outcome arrives on.
func renderInBackground(r Renderer, req RenderRequest) <-chan renderOutcome {
	done := make(chan renderOutcome, 1)
	go func() {
		report, err := r.Render(req)
		done <- renderOutcome{report, err}
	}()
	return done
}

// requireStillBlocked fails when the render finished within the observe window.
func requireStillBlocked(t *testing.T, done <-chan renderOutcome) {
	t.Helper()
	select {
	case outcome := <-done:
		t.Fatalf("render finished while its lock was held: %+v", outcome)
	case <-time.After(observeWindow):
	}
}

// requireFinished waits for the render and fails on an error.
func requireFinished(t *testing.T, done <-chan renderOutcome) Report {
	t.Helper()
	select {
	case outcome := <-done:
		if outcome.err != nil {
			t.Fatalf("Render() = %v", outcome.err)
		}
		return outcome.report
	case <-time.After(30 * time.Second):
		t.Fatal("render did not finish after its lock was released")
		return Report{}
	}
}

func mustRender(t *testing.T, f renderFixture, only ...string) Report {
	t.Helper()
	report, err := Renderer{}.Render(f.request(only...))
	if err != nil {
		t.Fatalf("Render() = %v", err)
	}
	return report
}

// useShippedLayer replaces the package's shipped layer with the entries in json until the test ends.
func useShippedLayer(t *testing.T, json string) {
	t.Helper()
	previous := shippedStore
	shippedStore = decodeShipped([]byte(json))
	t.Cleanup(func() { shippedStore = previous })
}

// TestShowAndPassagesReadTheShippedLayerFirst is not parallel: it replaces shippedStore, which every render reads.
func TestShowAndPassagesReadTheShippedLayerFirst(t *testing.T) {
	useShippedLayer(t, `{"entries": [
  {"slug": "ship-file", "text": "shipped file text", "order": 10, "targets": ["claude-file"]},
  {"slug": "ship-prompt", "text": "shipped prompt", "order": 5, "targets": ["claude-prompt"]}
]}`)
	repository := Store{Entries: []Entry{entryFor("mine", "repository prompt", 1, "claude-prompt")}}

	tests := []struct {
		name         string
		config       string
		override     string
		wantPrompt   string
		wantFileText string
	}{
		{"NoOverride", "", "", "shipped prompt\n\nrepository prompt", "shipped file text"},
		{"OverrideOn", LymOverrideKey + ": true\n", "tuned prompt", "tuned prompt\n\nrepository prompt", "tuned file text"},
		{"OverrideOff", LymOverrideKey + ": false\n", "tuned prompt", "shipped prompt\n\nrepository prompt", "shipped file text"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := newRenderFixture(t, repository)
			if tt.config != "" {
				writeHeddleConfig(t, f.bolt, tt.config)
			}
			if tt.override != "" {
				var overrides Store
				if err := overrides.SetOverride("ship-prompt", tt.override); err != nil {
					t.Fatal(err)
				}
				if err := overrides.SetOverride("ship-file", strings.Replace(tt.override, "prompt", "file text", 1)); err != nil {
					t.Fatal(err)
				}
				if err := SaveOverrides(f.bolt, overrides); err != nil {
					t.Fatal(err)
				}
			}
			r := Renderer{}

			shown, err := r.Show(f.bolt, "claude-prompt", f.root)
			if err != nil || shown != tt.wantPrompt {
				t.Errorf("Show(claude-prompt) = %q, %v; want %q", shown, err, tt.wantPrompt)
			}
			passages, err := r.Passages(f.bolt, "claude", "")
			if want := strings.Split(tt.wantPrompt, "\n\n"); err != nil || !slices.Equal(passages, want) {
				t.Errorf("Passages(claude) = %q, %v; want %q", passages, err, want)
			}
			shown, err = r.Show(f.bolt, "claude-file", f.root)
			if want := strings.Join([]string{Marker, shippedHeading, tt.wantFileText}, "\n\n") + "\n"; err != nil || shown != want {
				t.Errorf("Show(claude-file) = %q, %v; want %q", shown, err, want)
			}
		})
	}
}

func TestRender(t *testing.T) {
	t.Parallel()

	claudeStore := Store{Entries: []Entry{
		entryFor("second", "two", 2, "claude-file"),
		entryFor("first", "one", 1, "claude-file"),
	}}

	t.Run("ClaudeFileJoinsEntriesInOrder", func(t *testing.T) {
		t.Parallel()
		f := newRenderFixture(t, claudeStore)
		report := mustRender(t, f)
		if want := claudeFileWant("", "one", "two"); f.readRoot(t, "CLAUDE.local.md") != want {
			t.Errorf("CLAUDE.local.md = %q; want %q", f.readRoot(t, "CLAUDE.local.md"), want)
		}
		if !slices.Equal(report.Written, []string{"CLAUDE.local.md"}) || len(report.Warnings) != 0 {
			t.Errorf("report = %+v; want only CLAUDE.local.md written", report)
		}
		if f.rootHas(".heddle") {
			t.Error(".heddle left behind")
		}
	})

	t.Run("ClaudeFileImportsAgentsFiles", func(t *testing.T) {
		t.Parallel()
		tests := []struct {
			name  string
			files []string
			want  string
		}{
			{"RootAgents", []string{"AGENTS.md"}, "@AGENTS.md"},
			{"ClaudeDirAgents", []string{".claude/AGENTS.md"}, "@.claude/AGENTS.md"},
			{"BothAgents", []string{"AGENTS.md", ".claude/AGENTS.md"}, "@AGENTS.md\n@.claude/AGENTS.md"},
			{"RootClaude", []string{"AGENTS.md", "CLAUDE.md"}, ""},
			{"ClaudeDirClaude", []string{"AGENTS.md", ".claude/CLAUDE.md"}, ""},
		}
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				t.Parallel()
				f := newRenderFixture(t, Store{Entries: []Entry{entryFor("only", "text", 1, "claude-file")}})
				for _, rel := range tt.files {
					f.writeRoot(t, rel, "base\n")
				}
				mustRender(t, f)
				if want := claudeFileWant(tt.want, "text"); f.readRoot(t, "CLAUDE.local.md") != want {
					t.Errorf("CLAUDE.local.md = %q; want %q", f.readRoot(t, "CLAUDE.local.md"), want)
				}
			})
		}
	})

	t.Run("CodexFileOpensWithRootAgents", func(t *testing.T) {
		t.Parallel()
		f := newRenderFixture(t, Store{Entries: []Entry{entryFor("c", "codex text", 1, "codex-file")}})
		f.writeRoot(t, "AGENTS.md", "repository rules\n")
		mustRender(t, f)
		if want := Marker + "\n\nrepository rules\n\ncodex text\n"; f.readRoot(t, "AGENTS.override.md") != want {
			t.Errorf("AGENTS.override.md = %q; want %q", f.readRoot(t, "AGENTS.override.md"), want)
		}
	})

	t.Run("ClaudeFileHoldsTheShippedLayerWithNoEntry", func(t *testing.T) {
		t.Parallel()
		f := newRenderFixture(t, Store{})
		report := mustRender(t, f)
		if want := claudeFileWant(""); f.readRoot(t, "CLAUDE.local.md") != want {
			t.Errorf("CLAUDE.local.md = %q; want %q", f.readRoot(t, "CLAUDE.local.md"), want)
		}
		if !slices.Equal(report.Written, []string{"CLAUDE.local.md"}) || f.rootHas("AGENTS.override.md") {
			t.Errorf("report = %+v; want only CLAUDE.local.md written", report)
		}
	})

	t.Run("RepositoryEntryUnderAShippedSlugRendersUnderTheRepositoryHeading", func(t *testing.T) {
		t.Parallel()
		f := newRenderFixture(t, Store{Entries: []Entry{entryFor(shippedStore.Ordered()[0].Slug, "mine", 1, "claude-file")}})
		mustRender(t, f)
		if want := claudeFileWant("", "mine"); f.readRoot(t, "CLAUDE.local.md") != want {
			t.Errorf("CLAUDE.local.md = %q; want %q", f.readRoot(t, "CLAUDE.local.md"), want)
		}
	})

	t.Run("OverrideReplacesTheShippedTextOnlyWhileTheSettingIsOn", func(t *testing.T) {
		t.Parallel()
		shipped := shippedStore.Ordered()[0]
		tests := []struct {
			name         string
			config       string
			wantOverride bool
		}{
			{"SettingOn", LymOverrideKey + ": true\n", true},
			{"SettingOff", LymOverrideKey + ": false\n", false},
		}
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				t.Parallel()
				f := newRenderFixture(t, Store{})
				writeHeddleConfig(t, f.bolt, tt.config)
				var overrides Store
				if err := overrides.SetOverride(shipped.Slug, "tuned text"); err != nil {
					t.Fatal(err)
				}
				if err := SaveOverrides(f.bolt, overrides); err != nil {
					t.Fatal(err)
				}
				mustRender(t, f)
				want := claudeFileWant("")
				if tt.wantOverride {
					want = strings.Replace(want, shipped.Text, "tuned text", 1)
				}
				if got := f.readRoot(t, "CLAUDE.local.md"); got != want {
					t.Errorf("CLAUDE.local.md = %q; want %q", got, want)
				}
			})
		}
	})

	t.Run("OverrideWithoutEffectLeavesTheShippedText", func(t *testing.T) {
		t.Parallel()
		tests := []struct {
			name   string
			config string
			slug   string
		}{
			{"NoHeddleConfigDefaultsOff", "", shippedStore.Ordered()[0].Slug},
			{"StaleOverrideOfAnUnshippedSlug", LymOverrideKey + ": true\n", "no-longer-shipped"},
		}
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				t.Parallel()
				f := newRenderFixture(t, Store{})
				if tt.config != "" {
					writeHeddleConfig(t, f.bolt, tt.config)
				}
				if err := SaveOverrides(f.bolt, Store{Entries: []Entry{{Slug: tt.slug, Text: "tuned text"}}}); err != nil {
					t.Fatal(err)
				}
				report := mustRender(t, f)
				if want := claudeFileWant(""); f.readRoot(t, "CLAUDE.local.md") != want {
					t.Errorf("CLAUDE.local.md = %q; want %q", f.readRoot(t, "CLAUDE.local.md"), want)
				}
				if len(report.Warnings) != 0 {
					t.Errorf("warnings = %v; want none", report.Warnings)
				}
			})
		}
	})

	t.Run("UnusableOverrideFileLeavesTheShippedTextStanding", func(t *testing.T) {
		t.Parallel()
		shipped := shippedStore.Ordered()[0]
		tests := []struct {
			name        string
			file        string
			wantWarning bool
		}{
			{"MalformedJSON", `{"entries": [`, true},
			{"BlankText", `{"entries": [{"slug": "` + shipped.Slug + `", "text": "  \n"}]}`, false},
		}
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				t.Parallel()
				f := newRenderFixture(t, claudeStore)
				writeHeddleConfig(t, f.bolt, LymOverrideKey+": true\n")
				if err := os.WriteFile(filepath.Join(f.bolt, OverrideFileName), []byte(tt.file), 0o644); err != nil {
					t.Fatal(err)
				}
				report := mustRender(t, f)
				if want := claudeFileWant("", "one", "two"); f.readRoot(t, "CLAUDE.local.md") != want {
					t.Errorf("CLAUDE.local.md = %q; want %q", f.readRoot(t, "CLAUDE.local.md"), want)
				}
				if tt.wantWarning {
					if len(report.Warnings) != 1 || !strings.Contains(report.Warnings[0], OverrideFileName) {
						t.Errorf("warnings = %v; want one naming %s", report.Warnings, OverrideFileName)
					}
				} else if len(report.Warnings) != 0 {
					t.Errorf("warnings = %v; want none", report.Warnings)
				}
			})
		}
	})

	t.Run("StaleMarkedRenderIsRemovedEvenOffExcluded", func(t *testing.T) {
		t.Parallel()
		f := newRenderFixture(t, Store{})
		f.excluded = nil
		f.writeRoot(t, "AGENTS.override.md", Marker+"\n\nold\n")
		report := mustRender(t, f)
		if f.rootHas("AGENTS.override.md") || !slices.Equal(report.Removed, []string{"AGENTS.override.md"}) {
			t.Errorf("stale render kept or not reported: removed %v", report.Removed)
		}
	})

	t.Run("UnmarkedFileIsLeftAlone", func(t *testing.T) {
		t.Parallel()
		f := newRenderFixture(t, claudeStore)
		f.writeRoot(t, "CLAUDE.local.md", "mine\n")
		report := mustRender(t, f)
		if f.readRoot(t, "CLAUDE.local.md") != "mine\n" {
			t.Error("unmarked CLAUDE.local.md was overwritten")
		}
		if len(report.Warnings) != 1 || !strings.Contains(report.Warnings[0], "CLAUDE.local.md") {
			t.Errorf("warnings = %v; want one naming CLAUDE.local.md", report.Warnings)
		}

		f = newRenderFixture(t, Store{Entries: []Entry{entryFor("c", "codex", 1, "codex-file")}})
		f.writeRoot(t, "CLAUDE.local.md", "mine\n")
		mustRender(t, f)
		if f.readRoot(t, "CLAUDE.local.md") != "mine\n" {
			t.Error("unmarked CLAUDE.local.md was removed")
		}
	})

	t.Run("OffExcludedTargetIsNotWritten", func(t *testing.T) {
		t.Parallel()
		tests := []struct {
			name     string
			excluded []string
			want     string
		}{
			{"TargetOff", []string{".heddle"}, fabricengine.ExcludeWayForward("CLAUDE.local.md")},
			{"TempDirOff", []string{"CLAUDE.local.md"}, fabricengine.ExcludeWayForward(".heddle")},
		}
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				t.Parallel()
				f := newRenderFixture(t, claudeStore)
				f.excluded = tt.excluded
				report := mustRender(t, f)
				if f.rootHas("CLAUDE.local.md") || f.rootHas(".heddle") {
					t.Error("a file was written although the exclude list lacks a path")
				}
				if !slices.Contains(report.Warnings, tt.want) {
					t.Errorf("warnings = %v; want %q", report.Warnings, tt.want)
				}
			})
		}
	})

	t.Run("BlockedTempDirWritesNoFile", func(t *testing.T) {
		t.Parallel()
		tests := []struct {
			name  string
			setup func(f renderFixture, t *testing.T)
		}{
			{"File", func(f renderFixture, t *testing.T) { f.writeRoot(t, ".heddle", "x") }},
			{"DirWithOtherFile", func(f renderFixture, t *testing.T) { f.writeRoot(t, ".heddle/notes.txt", "x") }},
		}
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				t.Parallel()
				f := newRenderFixture(t, claudeStore)
				tt.setup(f, t)
				report := mustRender(t, f)
				if f.rootHas("CLAUDE.local.md") {
					t.Error("CLAUDE.local.md written although .heddle is blocked")
				}
				if len(report.Warnings) != 1 || !strings.Contains(report.Warnings[0], ".heddle") || !strings.Contains(report.Warnings[0], "lym heddle render") {
					t.Errorf("warnings = %v; want one naming .heddle and lym heddle render", report.Warnings)
				}
			})
		}
	})

	t.Run("LeftoverTempFileIsOverwritten", func(t *testing.T) {
		t.Parallel()
		f := newRenderFixture(t, claudeStore)
		f.writeRoot(t, ".heddle/CLAUDE.local.md.tmp", "half a render")
		mustRender(t, f)
		if !strings.HasPrefix(f.readRoot(t, "CLAUDE.local.md"), Marker) || f.rootHas(".heddle") {
			t.Error("leftover temp file not overwritten, or .heddle left behind")
		}
	})

	t.Run("OnlyLimitsTheTargets", func(t *testing.T) {
		t.Parallel()
		f := newRenderFixture(t, Store{Entries: []Entry{entryFor("both", "t", 1, "claude-file", "codex-file")}})
		mustRender(t, f, "codex-file")
		if f.rootHas("CLAUDE.local.md") || !f.rootHas("AGENTS.override.md") {
			t.Error("Only codex-file did not leave CLAUDE.local.md unwritten and AGENTS.override.md written")
		}
	})

	t.Run("HeldBoltLockFailsTheRender", func(t *testing.T) {
		t.Parallel()
		f := newRenderFixture(t, claudeStore)
		held, err := lock.AcquireWriteLock(filepath.Join(f.bolt, fabricengine.BoardWriteLockFile))
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = held.Release() }()
		_, err = Renderer{BoltLockWait: observeWindow}.Render(f.request())
		if err == nil || !strings.Contains(err.Error(), "lym heddle render") {
			t.Errorf("Render() = %v; want an error naming lym heddle render", err)
		}
		if f.rootHas("CLAUDE.local.md") || f.rootHas(".heddle") {
			t.Error("a failed render wrote something")
		}
	})

	t.Run("ReadWaitsForTheBoltLock", func(t *testing.T) {
		t.Parallel()
		f := newRenderFixture(t, Store{})
		if err := os.Remove(filepath.Join(f.bolt, FileName)); err != nil {
			t.Fatal(err)
		}
		held, err := lock.AcquireWriteLock(filepath.Join(f.bolt, fabricengine.BoardWriteLockFile))
		if err != nil {
			t.Fatal(err)
		}
		done := renderInBackground(Renderer{BoltLockWait: 30 * time.Second}, f.request())
		requireStillBlocked(t, done)
		if f.rootHas("CLAUDE.local.md") {
			t.Error("a file was written while the Bolt lock was held")
		}
		saveStore(t, f.bolt, claudeStore)
		if err := held.Release(); err != nil {
			t.Fatal(err)
		}
		requireFinished(t, done)
		if want := claudeFileWant("", "one", "two"); f.readRoot(t, "CLAUDE.local.md") != want {
			t.Errorf("CLAUDE.local.md = %q; want the new store's text", f.readRoot(t, "CLAUDE.local.md"))
		}
	})

	t.Run("RenderLockSerializesRenders", func(t *testing.T) {
		t.Parallel()
		f := newRenderFixture(t, claudeStore)
		lockDir := filepath.Join(f.scratch, lymdirs.HeddleDirName)
		if err := os.MkdirAll(lockDir, 0o755); err != nil {
			t.Fatal(err)
		}
		held, err := lock.AcquireWriteLock(filepath.Join(lockDir, "render.lock"))
		if err != nil {
			t.Fatal(err)
		}
		done := renderInBackground(Renderer{}, f.request())
		requireStillBlocked(t, done)
		if f.rootHas("CLAUDE.local.md") || f.rootHas(".heddle") {
			t.Error("a render wrote while another held the render lock")
		}
		if err := held.Release(); err != nil {
			t.Fatal(err)
		}
		requireFinished(t, done)
		if !f.rootHas("CLAUDE.local.md") {
			t.Error("the render did not complete after the render lock was released")
		}
	})

	t.Run("ShowAndPassagesWriteNothing", func(t *testing.T) {
		t.Parallel()
		f := newRenderFixture(t, Store{Entries: []Entry{
			entryFor("b", "prompt two", 2, "claude-prompt"),
			entryFor("a", "prompt one", 1, "claude-prompt"),
			entryFor("c", "file text", 3, "claude-file"),
		}})
		r := Renderer{}
		shown, err := r.Show(f.bolt, "claude-file", f.root)
		if err != nil || shown != claudeFileWant("", "file text") {
			t.Errorf("Show(claude-file) = %q, %v", shown, err)
		}
		shown, err = r.Show(f.bolt, "claude-prompt", f.root)
		if err != nil || shown != "prompt one\n\nprompt two" {
			t.Errorf("Show(claude-prompt) = %q, %v", shown, err)
		}
		if _, err := r.Show(f.bolt, "nope", f.root); err == nil || !strings.Contains(err.Error(), "claude-file") {
			t.Errorf("Show(nope) = %v; want an error naming the valid targets", err)
		}
		claude, err := r.Passages(f.bolt, "claude", "")
		if err != nil || !slices.Equal(claude, []string{"prompt one", "prompt two"}) {
			t.Errorf("Passages(claude) = %v, %v", claude, err)
		}
		codex, err := r.Passages(f.bolt, "codex", "")
		if err != nil || len(codex) != 0 {
			t.Errorf("Passages(codex) = %v, %v; want none", codex, err)
		}
		if f.rootHas("CLAUDE.local.md") || f.rootHas(".heddle") {
			t.Error("Show or Passages wrote a file")
		}
	})

	t.Run("OrchPromptReachesOnlyOrchSessions", func(t *testing.T) {
		t.Parallel()
		f := newRenderFixture(t, Store{Entries: []Entry{
			entryFor("rules", "orch rule", 2, "orch-prompt"),
			entryFor("both", "shared rule", 3, "claude-prompt", "orch-prompt"),
			entryFor("house", "house rule", 1, "claude-prompt"),
		}})
		r := Renderer{}
		tests := []struct {
			name   string
			engine string
			role   string
			want   []string
		}{
			{"ClaudeOrch", "claude", "orch", []string{"house rule", "orch rule", "shared rule"}},
			{"ClaudeNumberedOrch", "claude", "orch-2", []string{"house rule", "orch rule", "shared rule"}},
			{"CodexOrch", "codex", "orch", []string{"orch rule", "shared rule"}},
			{"ClaudeWriter", "claude", "writer", []string{"house rule", "shared rule"}},
			{"ClaudeOrchLookalike", "claude", "orchard", []string{"house rule", "shared rule"}},
			{"CodexWriter", "codex", "writer", nil},
		}
		for _, tt := range tests {
			got, err := r.Passages(f.bolt, tt.engine, tt.role)
			if err != nil || !slices.Equal(got, tt.want) {
				t.Errorf("%s: Passages(%q, %q) = %v, %v; want %v", tt.name, tt.engine, tt.role, got, err, tt.want)
			}
		}
		shown, err := r.Show(f.bolt, "orch-prompt", f.root)
		if err != nil || shown != "orch rule\n\nshared rule" {
			t.Errorf("Show(orch-prompt) = %q, %v; want the two orch passages", shown, err)
		}
	})
}
