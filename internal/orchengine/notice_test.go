package orchengine

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/shuttleengine"
)

func noticePaths(t *testing.T, strand string) Paths {
	t.Helper()
	p := testPaths(t)
	p.NoticesDir = filepath.Join(p.Dir, "notices")
	if strand != "" {
		if err := SaveState(p, State{Strand: strand, Phase: PhaseIdle}); err != nil {
			t.Fatal(err)
		}
	}
	return p
}

var noticeEpoch = time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)

func TestQueueNotice_NoStrandLogsOnly(t *testing.T) {
	p := noticePaths(t, "")
	queued, err := QueueNotice(p, "hello", noticeEpoch)
	if err != nil || queued {
		t.Fatalf("QueueNotice = %v, %v, want not queued and no error", queued, err)
	}
	if _, err := os.Stat(p.NoticesDir); !os.IsNotExist(err) {
		t.Errorf("notices dir stat = %v, want absent", err)
	}
}

func TestQueueNotice_RefusesMultiLineAndSlashLeading(t *testing.T) {
	p := noticePaths(t, "s1")
	for _, line := range []string{"a\nb", "a\rb", "/clear", "/compact now"} {
		queued, err := QueueNotice(p, line, noticeEpoch)
		if err == nil || queued {
			t.Errorf("QueueNotice(%q) = %v, %v, want a refusal", line, queued, err)
		}
	}
	if got, err := ListNotices(p); err != nil || len(got) != 0 {
		t.Errorf("queue = %v, %v, want empty", got, err)
	}
}

func TestQueueNotice_ListingOrderAndUniqueness(t *testing.T) {
	t.Run("files sort by arrival and list oldest first", func(t *testing.T) {
		t.Parallel()
		p := noticePaths(t, "s1")
		// Queue out of lexical order of content, with later times for later calls.
		for i, line := range []string{"zulu", "alpha", "mike"} {
			if queued, err := QueueNotice(p, line, noticeEpoch.Add(time.Duration(i)*time.Nanosecond)); err != nil || !queued {
				t.Fatalf("QueueNotice(%q) = %v, %v", line, queued, err)
			}
		}
		got, err := ListNotices(p)
		if err != nil {
			t.Fatal(err)
		}
		want := []string{"zulu", "alpha", "mike"}
		if len(got) != len(want) {
			t.Fatalf("queue = %v, want %v", got, want)
		}
		for i := range want {
			if got[i].Line != want[i] {
				t.Errorf("notice %d = %q, want %q", i, got[i].Line, want[i])
			}
		}
	})

	t.Run("same instant never shares a file", func(t *testing.T) {
		t.Parallel()
		p := noticePaths(t, "s1")
		for i := 0; i < 5; i++ {
			if _, err := QueueNotice(p, "same", noticeEpoch); err != nil {
				t.Fatal(err)
			}
		}
		if got, _ := ListNotices(p); len(got) != 5 {
			t.Errorf("queue holds %d notices, want 5", len(got))
		}
	})
}

func TestQueueNotice_CapDropsTheOldest(t *testing.T) {
	p := noticePaths(t, "s1")
	total := maxNotices + 3
	for i := 0; i < total; i++ {
		line := fmt.Sprintf("n%03d", i)
		if _, err := QueueNotice(p, line, noticeEpoch.Add(time.Duration(i)*time.Millisecond)); err != nil {
			t.Fatal(err)
		}
	}
	got, err := ListNotices(p)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != maxNotices {
		t.Fatalf("queue holds %d notices, want %d", len(got), maxNotices)
	}
	if first, want := got[0].Line, fmt.Sprintf("n%03d", total-maxNotices); first != want {
		t.Errorf("oldest kept = %q, want %q", first, want)
	}
	if last, want := got[len(got)-1].Line, fmt.Sprintf("n%03d", total-1); last != want {
		t.Errorf("newest = %q, want %q", last, want)
	}
}

//testtiming:keep pins that removing an absent notice and dropping an absent queue return nil, which its covering tests do not assert
func TestRemoveAndDropNotices(t *testing.T) {
	p := noticePaths(t, "s1")
	for i, line := range []string{"a", "b"} {
		if _, err := QueueNotice(p, line, noticeEpoch.Add(time.Duration(i))); err != nil {
			t.Fatal(err)
		}
	}
	got, _ := ListNotices(p)
	if err := RemoveNotice(got[0]); err != nil {
		t.Fatal(err)
	}
	if err := RemoveNotice(got[0]); err != nil {
		t.Errorf("removing an absent notice = %v, want nil", err)
	}
	if rest, _ := ListNotices(p); len(rest) != 1 || rest[0].Line != "b" {
		t.Errorf("queue = %v, want [b]", rest)
	}
	if err := DropNotices(p); err != nil {
		t.Fatal(err)
	}
	if err := DropNotices(p); err != nil {
		t.Errorf("dropping an absent queue = %v, want nil", err)
	}
	if rest, err := ListNotices(p); err != nil || len(rest) != 0 {
		t.Errorf("queue = %v, %v, want empty", rest, err)
	}
}

func TestQueueConditionalNotice(t *testing.T) {
	t.Parallel()

	p := noticePaths(t, "s1")
	stalled := NoticeCondition{State: shuttleengine.SessionIdleStalled, Cause: shuttleengine.SessionCauseAPIError}
	busy := NoticeCondition{State: shuttleengine.SessionBusy, Cause: shuttleengine.SessionCauseTurn}

	if queued, err := QueueConditionalNotice(p, "carry on", stalled, noticeEpoch); err != nil || !queued {
		t.Fatalf("QueueConditionalNotice = %v, %v, want queued", queued, err)
	}
	got, err := ListNotices(p)
	if err != nil || len(got) != 1 {
		t.Fatalf("queue = %v, %v, want the one notice", got, err)
	}
	if got[0].Line != "carry on" || got[0].Condition == nil || *got[0].Condition != stalled {
		t.Errorf("notice = %+v, want line %q under %+v", got[0], "carry on", stalled)
	}
	if strings.HasSuffix(got[0].Path, noticeSuffix) {
		t.Errorf("notice file %q ends in %q, want a name an older watcher does not list", got[0].Path, noticeSuffix)
	}

	if queued, err := QueueConditionalNotice(p, "carry on", stalled, noticeEpoch.Add(time.Second)); err != nil || queued {
		t.Fatalf("second QueueConditionalNotice = %v, %v, want a duplicate skipped", queued, err)
	}
	if got, _ := ListNotices(p); len(got) != 1 {
		t.Fatalf("queue holds %d notices after the duplicate, want 1", len(got))
	}

	if queued, err := QueueNotice(p, "carry on", noticeEpoch.Add(2*time.Second)); err != nil || !queued {
		t.Fatalf("QueueNotice = %v, %v, want the unconditioned line queued beside it", queued, err)
	}
	if queued, err := QueueConditionalNotice(p, "carry on", busy, noticeEpoch.Add(3*time.Second)); err != nil || !queued {
		t.Fatalf("QueueConditionalNotice under another condition = %v, %v, want queued", queued, err)
	}
	got, _ = ListNotices(p)
	if len(got) != 3 || got[1].Condition != nil || got[2].Condition == nil || *got[2].Condition != busy {
		t.Errorf("queue = %+v, want the conditioned, the unconditioned and the other conditioned notice in arrival order", got)
	}
}
