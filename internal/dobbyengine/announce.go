// announce.go is a beam's dobby's announcement step: while the beam's landing record holds a pull request, it tells the beam's orch about each landing into the beam branch, so the orch keeps the description and the pull request listing every member.

package dobbyengine

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// announceLandings posts one line to the beam's own orch for the landings into the beam branch since the head it last announced, and records the head it read once the line went out.
// With no pull request recorded it does nothing.
// The last announced head is read from the geometry's AnnouncePath, or from the landing record's announce head when that file is missing, so a fresh machine repeats at most one notice.
// A head that moved past only commits that record lym's own state is recorded with no line, so the notice cannot feed itself.
// Every landing counts, a member filed after the pull request opened included, and no entry's `notify_on_land` is read.
// A failed read, post or record is logged once per episode and retried at the next tick; nothing else depends on it.
// The prime's dobby and a dobby with no landing seams do nothing.
func (d *daemon) announceLandings(ctx context.Context) {
	if d.deps.LandingPullRequest == nil || d.deps.BeamLandingsSince == nil || d.geom.Slug == "" {
		return
	}
	recordHead, recorded, err := d.deps.LandingPullRequest()
	if err != nil {
		d.announceRead.Fail("dobbyengine: the landing record's pull request is unreadable; no member landing is announced", "worktree", d.geom.WorktreeName, "error", err)
		return
	}
	if !recorded {
		d.announceRead.Recover("dobbyengine: the landing record reads again", "worktree", d.geom.WorktreeName)
		return
	}
	from, err := d.announcedHead(recordHead)
	if err != nil {
		d.announceRead.Fail("dobbyengine: the announced head is unreadable; no member landing is announced", "worktree", d.geom.WorktreeName, "error", err)
		return
	}
	head, landed, err := d.deps.BeamLandingsSince(from)
	if err != nil {
		d.announceRead.Fail("dobbyengine: the beam's landings are unreadable; no member landing is announced", "worktree", d.geom.WorktreeName, "error", err)
		return
	}
	d.announceRead.Recover("dobbyengine: the beam's landings read again", "worktree", d.geom.WorktreeName)

	if len(landed) > 0 {
		line := fmt.Sprintf("%s: landed into the beam while its pull request is recorded: %s; run `lym beam describe %s`, then `lym beam pr update %s`", d.geom.Slug, strings.Join(landed, "; "), d.geom.Slug, d.geom.Slug)
		if err := d.deps.Notice(ctx, line); err != nil {
			d.announcePost.Fail("dobbyengine: the member landing notice did not go out; the next tick tries again", "worktree", d.geom.WorktreeName, "error", err)
			return
		}
	}
	if head == from {
		d.announcePost.Recover("dobbyengine: the member landing announcement works again", "worktree", d.geom.WorktreeName)
		return
	}
	if err := d.recordAnnouncedHead(head); err != nil {
		d.announcePost.Fail("dobbyengine: the announced head could not be recorded; the next tick announces again", "worktree", d.geom.WorktreeName, "error", err)
		return
	}
	d.announcePost.Recover("dobbyengine: the member landing announcement works again", "worktree", d.geom.WorktreeName)
}

// announcedHead returns the head the dobby last announced up to: the AnnouncePath file's, or recordHead when the file is missing or empty.
func (d *daemon) announcedHead(recordHead string) (string, error) {
	data, err := os.ReadFile(d.geom.AnnouncePath)
	if errors.Is(err, fs.ErrNotExist) {
		return recordHead, nil
	}
	if err != nil {
		return "", err
	}
	if head := strings.TrimSpace(string(data)); head != "" {
		return head, nil
	}
	return recordHead, nil
}

// recordAnnouncedHead writes head as the last announced head.
func (d *daemon) recordAnnouncedHead(head string) error {
	if err := os.MkdirAll(filepath.Dir(d.geom.AnnouncePath), 0o755); err != nil {
		return err
	}
	return os.WriteFile(d.geom.AnnouncePath, []byte(head+"\n"), 0o644)
}
