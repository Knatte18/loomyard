// tasksetting_test.go pins the per-task lookup of beam.pull_request, beam.land_notice, beam.autonomy and beam.run_watch.
// It pins which of a task's own files, its parents' files, the hub default and the template decides.
// It also pins the refusal of a bad value.

package dobbyengine

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/configengine"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// writeBeamLocalLayer writes document as the local beam override under anchor's .lym.
func writeBeamLocalLayer(t *testing.T, anchor, document string) {
	t.Helper()
	path := configengine.LocalFile(anchor, "beam")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("MkdirAll(%q) = %v; want nil", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, []byte(document), 0o644); err != nil {
		t.Fatalf("WriteFile(%q) = %v; want nil", path, err)
	}
}

// lookupAsText runs the lookup of key and returns its answer as on or off text where the key is a switch.
// beam.pull_request's review reads as on, so one set of rows runs under every key.
func lookupAsText(key string, chain TaskSettingChain) (string, error) {
	if key == LandNoticeKey || key == AutonomyKey {
		lookup := LookupLandNotice
		if key == AutonomyKey {
			lookup = LookupAutonomy
		}
		on, err := lookup(chain)
		if err != nil {
			return "", err
		}
		if on {
			return LandNoticeOn, nil
		}
		return LandNoticeOff, nil
	}
	if key == RunWatchKey {
		return LookupRunWatch(chain)
	}
	got, err := LookupPullRequest(chain)
	if got == PullRequestReview {
		return LandNoticeOn, err
	}
	return got, err
}

// settingDocument returns a beam document setting key to value, or "" when value is empty.
// beam.autonomy's value is written unquoted, on as true and off as false, any other value as given.
// beam.pull_request's on is written as review.
func settingDocument(key, value string) string {
	if value == "" {
		return ""
	}
	if key == PullRequestKey && value == "on" {
		value = PullRequestReview
	}
	if key == AutonomyKey {
		switch value {
		case "on":
			value = "true"
		case "off":
			value = "false"
		}
		return key + ": " + value + "\n"
	}
	return key + ": \"" + value + "\"\n"
}

func TestLookupTaskSettings(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	// Each row sets on or off at some places, empty meaning the place sets nothing, and runs under both keys.
	tests := []struct {
		name string
		// task, parent and prime are the worktree files of the chain's anchors, nearest first.
		task, parent, prime string
		// taskLocal is the task's local file.
		taskLocal string
		// hub is the board's default layer.
		hub  string
		want string
		// wantSource is the anchor index LookupWithSource names as the place that set the value.
		wantSource int
	}{
		{name: "the task's local file wins over its worktree file and over the parent", task: "off", taskLocal: "on", parent: "off", want: "on", wantSource: 0},
		{name: "the task's local file turns a worktree on off", task: "on", taskLocal: "off", parent: "on", want: "off", wantSource: 0},
		{name: "a task that sets nothing takes the parent's value over the prime's", parent: "off", prime: "on", want: "off", wantSource: 1},
		{name: "a task that sets nothing takes the parent's on over the hub default", parent: "on", hub: "off", want: "on", wantSource: 1},
		{name: "a lower value overrides a parent's on", task: "off", parent: "on", want: "off", wantSource: 0},
		{name: "a lower value overrides a parent's off", task: "on", parent: "off", want: "on", wantSource: 0},
		{name: "the prime's own file applies when nothing nearer sets the key", prime: "off", hub: "on", want: "off", wantSource: 2},
		{name: "a hub default that is on applies when no anchor sets the key", hub: "on", want: "on", wantSource: HubWideSource},
		{name: "a hub default that is off applies when no anchor sets the key", hub: "off", want: "off", wantSource: HubWideSource},
		{name: "a hub default never counts as the prime's own setting", prime: "on", hub: "off", want: "on", wantSource: 2},
	}
	for _, key := range []string{PullRequestKey, LandNoticeKey, AutonomyKey} {
		for _, tt := range tests {
			t.Run(key+"/"+tt.name, func(t *testing.T) {
				t.Parallel()
				taskDir, parentDir, primeDir, board := t.TempDir(), t.TempDir(), t.TempDir(), t.TempDir()
				for dir, value := range map[string]string{taskDir: tt.task, parentDir: tt.parent, primeDir: tt.prime} {
					if value != "" {
						writeBeamLayer(t, dir, settingDocument(key, value))
					}
				}
				if tt.taskLocal != "" {
					writeBeamLocalLayer(t, taskDir, settingDocument(key, tt.taskLocal))
				}
				if tt.hub != "" {
					writeBeamLayer(t, board, settingDocument(key, tt.hub))
				}

				chain := TaskSettingChain{Anchors: []string{taskDir, parentDir, primeDir}, Board: board}
				got, err := lookupAsText(key, chain)
				if err != nil || got != tt.want {
					t.Errorf("lookup of %s = %q, %v; want %q", key, got, err, tt.want)
				}
				if _, source, err := LookupWithSource(chain, key); err != nil || source != tt.wantSource {
					t.Errorf("LookupWithSource(%s) source = %d, %v; want %d", key, source, err, tt.wantSource)
				}
			})
		}
	}

	t.Run("run_watch takes the nearest value that sets it, else the template's", func(t *testing.T) {
		t.Parallel()
		rows := []struct {
			name                                string
			task, taskLocal, parent, prime, hub string
			want                                string
		}{
			{name: "the task's local file wins over its worktree file", task: RunWatchOff, taskLocal: RunWatchRows, want: RunWatchRows},
			{name: "a task that sets nothing takes the parent's value over the prime's", parent: RunWatchOff, prime: RunWatchRows, want: RunWatchOff},
			{name: "the hub default applies when no anchor sets the key", hub: RunWatchRows, want: RunWatchRows},
			{name: "nothing set anywhere takes the template's value", want: RunWatchStops},
		}
		for _, row := range rows {
			t.Run(row.name, func(t *testing.T) {
				t.Parallel()
				taskDir, parentDir, primeDir, board := t.TempDir(), t.TempDir(), t.TempDir(), t.TempDir()
				for dir, value := range map[string]string{taskDir: row.task, parentDir: row.parent, primeDir: row.prime, board: row.hub} {
					if value != "" {
						writeBeamLayer(t, dir, settingDocument(RunWatchKey, value))
					}
				}
				if row.taskLocal != "" {
					writeBeamLocalLayer(t, taskDir, settingDocument(RunWatchKey, row.taskLocal))
				}

				got, err := LookupRunWatch(TaskSettingChain{Anchors: []string{taskDir, parentDir, primeDir}, Board: board})
				if err != nil || got != row.want {
					t.Errorf("LookupRunWatch() = %q, %v; want %q", got, err, row.want)
				}
			})
		}
	})

	t.Run("pull_request merge-in and trail pass through as set", func(t *testing.T) {
		t.Parallel()
		for _, value := range []string{PullRequestMergeIn, PullRequestTrail} {
			anchor := t.TempDir()
			writeBeamLayer(t, anchor, settingDocument(PullRequestKey, value))
			got, err := LookupPullRequest(TaskSettingChain{Anchors: []string{anchor}})
			if err != nil || got != value {
				t.Errorf("LookupPullRequest with %q set = %q, %v; want %q", value, got, err, value)
			}
		}
	})

	t.Run("nothing set anywhere takes the template's values", func(t *testing.T) {
		t.Parallel()
		chain := TaskSettingChain{Anchors: []string{t.TempDir(), t.TempDir()}, Board: t.TempDir()}
		if got, err := LookupPullRequest(chain); err != nil || got != "review" {
			t.Errorf("LookupPullRequest() = %q, %v; want review", got, err)
		}
		if got, err := LookupLandNotice(chain); err != nil || got {
			t.Errorf("LookupLandNotice() = %v, %v; want false", got, err)
		}
		if got, err := LookupAutonomy(chain); err != nil || got {
			t.Errorf("LookupAutonomy() = %v, %v; want false", got, err)
		}
		if got, source, err := LookupWithSource(chain, PullRequestKey); err != nil || got != "review" || source != HubWideSource {
			t.Errorf("LookupWithSource(pull_request) = %q, %d, %v; want review, %d", got, source, err, HubWideSource)
		}
	})

	t.Run("a chain with no board dir skips the hub default", func(t *testing.T) {
		t.Parallel()
		got, err := LookupPullRequest(TaskSettingChain{Anchors: []string{t.TempDir()}})
		if err != nil || got != "review" {
			t.Errorf("LookupPullRequest() = %q, %v; want review", got, err)
		}
	})
}

func TestLookupTaskSettings_ExpandsAnEnvMarkerAsLoadConfigDoes(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	tests := []struct {
		name string
		// env is the place's .env file.
		env string
		// want is the lookup's answer; wantRefusal names a substring of its refusal instead.
		want, wantRefusal string
	}{
		{name: "a marker the place's .env resolves reads as its value", env: "LYM_TEST_PULL_REQUEST_MODE=trail\n", want: PullRequestTrail},
		{name: "a marker that resolves to a value the key does not take is refused", env: "LYM_TEST_PULL_REQUEST_MODE=required\n", wantRefusal: "required"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			anchor := t.TempDir()
			writeBeamLayer(t, anchor, "pull_request: \"${env:LYM_TEST_PULL_REQUEST_MODE}\"\n")
			if err := os.WriteFile(filepath.Join(anchor, ".env"), []byte(tt.env), 0o644); err != nil {
				t.Fatalf("WriteFile(.env) = %v; want nil", err)
			}

			got, err := LookupPullRequest(TaskSettingChain{Anchors: []string{anchor}})
			if tt.wantRefusal != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantRefusal) {
					t.Errorf("LookupPullRequest() = %q, %v; want a refusal containing %q", got, err, tt.wantRefusal)
				}
				return
			}
			if err != nil || got != tt.want {
				t.Errorf("LookupPullRequest() = %q, %v; want %q", got, err, tt.want)
			}
		})
	}
}

//testtiming:keep pins the lookup refusal naming the setting file, the value and the way forward, which the lookup rows never read
func TestLookupTaskSettings_RefusesAValueTheKeyDoesNotTake(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	tests := []struct {
		name  string
		key   string
		value string
		// place is where the bad value is written: worktree, local or hub.
		place string
		// wantValues, when set, is the list of values the refusal must name.
		wantValues string
	}{
		{name: "run_watch all in a worktree file", key: RunWatchKey, value: "all", place: "worktree", wantValues: "[stops rows off]"},
		{name: "pull_request required in a worktree file", key: PullRequestKey, value: "required", place: "worktree"},
		{name: "land_notice all in a worktree file", key: LandNoticeKey, value: "all", place: "worktree"},
		{name: "land_notice per-task in a worktree file", key: LandNoticeKey, value: "per-task", place: "worktree"},
		{name: "pull_request on, renamed to review, in a worktree file", key: PullRequestKey, value: "on", place: "worktree"},
		{name: "pull_request merge, renamed to trail, in a local file", key: PullRequestKey, value: "merge", place: "local"},
		{name: "pull_request unknown in a local file", key: PullRequestKey, value: "maybe", place: "local"},
		{name: "land_notice merge in the hub default", key: LandNoticeKey, value: "merge", place: "hub"},
		{name: "autonomy yes in a worktree file", key: AutonomyKey, value: "yes", place: "worktree"},
		{name: "autonomy on in a local file", key: AutonomyKey, value: "on", place: "local"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			anchor, board := t.TempDir(), t.TempDir()
			// Quoted, so autonomy's on is written as the text "on" the key refuses, not as the boolean settingDocument maps it to.
			document := tt.key + ": \"" + tt.value + "\"\n"
			var wantPath string
			switch tt.place {
			case "worktree":
				writeBeamLayer(t, anchor, document)
				wantPath = configengine.ConfigFile(anchor, "beam")
			case "local":
				writeBeamLocalLayer(t, anchor, document)
				wantPath = configengine.LocalFile(anchor, "beam")
			case "hub":
				writeBeamLayer(t, board, document)
				wantPath = configengine.ConfigFile(board, "beam")
			}

			_, err := lookupAsText(tt.key, TaskSettingChain{Anchors: []string{anchor}, Board: board})
			if err == nil {
				t.Fatalf("lookup of %s = _, nil; want a refusal", tt.key)
			}
			for _, want := range []string{wantPath, tt.key, tt.value, tt.wantValues, "lym config beam --set " + tt.key + "="} {
				if !strings.Contains(err.Error(), want) {
					t.Errorf("lookup error = %q; want it to contain %q", err.Error(), want)
				}
			}
		})
	}
}
