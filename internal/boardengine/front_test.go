// front_test.go — tests for Board.FileFrontTask: what it stores, and the payload it refuses that no board verb reaches.

package boardengine_test

import (
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/boardengine"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

func TestFrontTask(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()
	b := boardengine.New(boardengine.Config{Path: t.TempDir(), Readme: "Home.md", DesignPrefix: "proposal-", Types: testTypes, Labels: testLabels, SkipGit: true})

	task, err := b.FileFrontTask(map[string]any{"slug": "fix", "title": "Fix", "kind": boardengine.KindTask, "labels": bugLabels}, "adopt-check")
	if err != nil {
		t.Fatalf("FileFrontTask() = %v; want nil", err)
	}
	if !task.Front || task.OnLand != "adopt-check" || task.Status == nil || *task.Status != "draft" {
		t.Errorf("FileFrontTask() task = %+v; want a draft front task with on_land adopt-check", task)
	}

	if err := b.SetStatus("fix", nil); err != nil {
		t.Fatalf("SetStatus(nil) = %v; want nil", err)
	}
	stored, found, err := b.GetTask("fix")
	if err != nil || !found || stored.Status != nil || !stored.Front || stored.OnLand != "adopt-check" {
		t.Errorf("GetTask() = %+v, %v, %v; want the released front task with its fields kept", stored, found, err)
	}

	_, err = b.FileFrontTask(map[string]any{"slug": "other", "title": "Other", "kind": boardengine.KindTask, "labels": bugLabels, "front": true}, "adopt-check")
	if err == nil || !strings.Contains(err.Error(), "only lym files a front task") {
		t.Errorf("FileFrontTask() with a front key = %v; want the payload refusal", err)
	}
	if _, found, _ := b.GetTask("other"); found {
		t.Errorf("a refused FileFrontTask stored the entry")
	}
}
