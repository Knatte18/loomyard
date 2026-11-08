// keeper.go makes, adopts, joins, invites into and retires the Slack channels of a hub's orchs.
// It records them in the hub's channel map.

package slackengine

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"sort"
	"strings"
	"sync"

	"github.com/Knatte18/loomyard/internal/agentname"
	"github.com/Knatte18/loomyard/internal/logger"
)

const (
	// heldByOrchPrefix and heldByHubPrefix open the answer of heldElsewhere, which says what holds a channel.
	heldByOrchPrefix = "orch "
	heldByHubPrefix  = "hub "
	// maxArchivedNameTries bounds how many dated names a retiring channel tries before it gives up.
	maxArchivedNameTries = 50
	// nameTakenCode is the code Slack answers a create or a rename of a taken name with.
	nameTakenCode = "name_taken"
)

// errChannelRefused marks a channel the keeper will not make or adopt, so a caller tells it from a transient failure.
var errChannelRefused = errors.New("slack: the channel was refused")

// channelKeeper keeps the Slack channel of each orch of one hub.
// It is safe for concurrent use; one orch's channel is ensured by one goroutine at a time.
type channelKeeper struct {
	geom     Geometry
	slack    SlackClient
	liveHubs LiveHubs
	mailbox  Mailbox
	clock    Clock

	mu sync.Mutex
	// orchLocks holds the mutex that serializes ensuring one orch's channel.
	orchLocks map[string]*sync.Mutex
	// refused holds the orchs whose retirement Slack refused.
	refused map[string]bool
	// noticed holds the keys of the notice episodes that have mailed their notice.
	noticed map[string]bool
	// botUserID is the Slack user id of the machine's bot, read once.
	botUserID string
}

// newChannelKeeper returns a keeper over the seams; it panics on a nil LiveHubs, which the relay's wiring always sets.
func newChannelKeeper(geom Geometry, slack SlackClient, liveHubs LiveHubs, mailbox Mailbox, clock Clock) *channelKeeper {
	if liveHubs == nil {
		panic("slackengine: the channel keeper needs the live-hub seam")
	}
	return &channelKeeper{
		geom:      geom,
		slack:     slack,
		liveHubs:  liveHubs,
		mailbox:   mailbox,
		clock:     clock,
		orchLocks: map[string]*sync.Mutex{},
		refused:   map[string]bool{},
		noticed:   map[string]bool{},
	}
}

// orchLock returns the mutex of one orch's channel.
func (k *channelKeeper) orchLock(orch string) *sync.Mutex {
	k.mu.Lock()
	defer k.mu.Unlock()
	lock, ok := k.orchLocks[orch]
	if !ok {
		lock = &sync.Mutex{}
		k.orchLocks[orch] = lock
	}
	return lock
}

// ensure returns the open channel entry of the orch, making the channel when the map holds none or an archived one.
// An open entry that records another operator than the setup's, or none, is invited for the setup's operator.
// The setup is read at each ensure, so a changed operator applies at the next trigger.
func (k *channelKeeper) ensure(ctx context.Context, orch string) (ChannelEntry, error) {
	lock := k.orchLock(orch)
	lock.Lock()
	defer lock.Unlock()

	channelMap, err := ReadChannelMap(k.geom.ChannelMapPath)
	if err != nil {
		return ChannelEntry{}, err
	}
	if entry, held := channelMap.Channels[orch]; !held || entry.Archived {
		return k.create(ctx, orch)
	}
	if err := k.inviteOperator(ctx, orch); err != nil {
		logger.Debug("slackengine: could not invite the operator into a held channel", "orch", orch, "error", err)
	}
	return k.openEntry(orch)
}

// openEntry reads the orch's entry from the channel map.
func (k *channelKeeper) openEntry(orch string) (ChannelEntry, error) {
	channelMap, err := ReadChannelMap(k.geom.ChannelMapPath)
	if err != nil {
		return ChannelEntry{}, err
	}
	entry, held := channelMap.Channels[orch]
	if !held {
		return ChannelEntry{}, fmt.Errorf("slack: the channel map %s holds no channel for %s", k.geom.ChannelMapPath, orch)
	}
	return entry, nil
}

// slugOf returns the beam slug of an orch's creel name, empty for the hub's own orch.
func slugOf(orch string) (string, error) {
	name, err := agentname.Parse(orch)
	if err != nil {
		return "", fmt.Errorf("slack: name the channel of %q: %w", orch, err)
	}
	return name.Slug, nil
}

// create makes the orch's channel under its name and records it with an empty cursor, replacing an archived entry.
// The entry stands before the operator's invite, so the operator's first message is routable.
// A name Slack reports taken goes on to adopt.
func (k *channelKeeper) create(ctx context.Context, orch string) (ChannelEntry, error) {
	slug, err := slugOf(orch)
	if err != nil {
		return ChannelEntry{}, err
	}
	name := ChannelName(k.geom.Shortname, k.geom.Machine, slug)
	info, err := k.slack.CreateChannel(ctx, name)
	if err != nil {
		if refusal, refused := isAPIError(err); refused && refusal.Code == nameTakenCode {
			return k.adopt(ctx, orch, name)
		}
		return ChannelEntry{}, fmt.Errorf("slack: create the channel %s: %w", name, err)
	}
	if info.Name == "" {
		info.Name = name
	}
	if err := k.record(orch, ChannelEntry{ID: info.ID, Name: info.Name}); err != nil {
		return ChannelEntry{}, err
	}
	if err := k.inviteOperator(ctx, orch); err != nil {
		logger.Debug("slackengine: could not invite the operator into a new channel", "orch", orch, "error", err)
	}
	return k.openEntry(orch)
}

// record stores entry as the orch's entry in the channel map.
func (k *channelKeeper) record(orch string, entry ChannelEntry) error {
	return UpdateChannelMap(k.geom.ChannelMapPath, func(channelMap *ChannelMap) error {
		channelMap.Channels[orch] = entry
		return nil
	})
}

// adopt takes over the live channel called name, which a create found taken.
// It does so when this machine's bot made the channel and no other orch holds it.
// The bot joins it first when it is no member.
// The entry's cursor and thread floor start at the channel's newest message.
// An archived channel, another creator's and one another orch or hub holds are refused with one notice per episode.
// Each refusal returns an error that wraps errChannelRefused.
func (k *channelKeeper) adopt(ctx context.Context, orch, name string) (ChannelEntry, error) {
	info, found, err := k.slack.FindChannel(ctx, name)
	if err != nil {
		return ChannelEntry{}, fmt.Errorf("slack: look up the taken channel %s: %w", name, err)
	}
	if !found {
		return ChannelEntry{}, fmt.Errorf("slack: the channel name %s is taken but the channel list does not show it; the next trigger tries again", name)
	}
	refusalKey := "refused:" + orch
	if info.IsArchived {
		return ChannelEntry{}, k.refuse(refusalKey, name, fmt.Sprintf(
			"the channel %s (%s) is archived, and the relay never unarchives; way forward: rename that channel in Slack, which frees the name", name, info.ID))
	}
	bot, err := k.botUser(ctx)
	if err != nil {
		return ChannelEntry{}, err
	}
	if info.Creator != bot {
		return ChannelEntry{}, k.refuse(refusalKey, name, fmt.Sprintf(
			"the channel %s (%s) was made by another user than this machine's bot; way forward: rename that channel in Slack, which frees the name", name, info.ID))
	}
	holder, err := k.heldElsewhere(orch, info.ID)
	if err != nil {
		return ChannelEntry{}, err
	}
	switch {
	case strings.HasPrefix(holder, heldByHubPrefix):
		return ChannelEntry{}, k.refuse(refusalKey, name, fmt.Sprintf(
			"the channel %s (%s) is held by the hub %s, which records the same shortname %q; "+
				"way forward: remove the extra clone of the hub, since a recorded shortname never changes",
			name, info.ID, strings.TrimPrefix(holder, heldByHubPrefix), k.geom.Shortname))
	case holder != "":
		return ChannelEntry{}, k.refuse(refusalKey, name, fmt.Sprintf(
			"the channel %s (%s) is held by %s of this hub; way forward: rename that channel in Slack, which frees the name",
			name, info.ID, strings.TrimPrefix(holder, heldByOrchPrefix)))
	}
	if !info.IsMember {
		if err := k.slack.Join(ctx, info.ID); err != nil {
			return ChannelEntry{}, fmt.Errorf("slack: join the channel %s: %w", name, err)
		}
	}
	newest, hasMessages, err := k.slack.Latest(ctx, info.ID)
	if err != nil {
		return ChannelEntry{}, fmt.Errorf("slack: read the newest message of the channel %s: %w", name, err)
	}
	if !hasMessages {
		newest = slackTimestamp(k.clock.Now())
	}
	if err := k.record(orch, ChannelEntry{ID: info.ID, Name: info.Name, Cursor: newest, ThreadFloor: newest}); err != nil {
		return ChannelEntry{}, err
	}
	k.endNotice(refusalKey)
	if err := k.inviteOperator(ctx, orch); err != nil {
		logger.Debug("slackengine: could not invite the operator into an adopted channel", "orch", orch, "error", err)
	}
	return k.openEntry(orch)
}

// refuse mails the notice of a refused channel once per episode and returns the error that wraps errChannelRefused.
func (k *channelKeeper) refuse(key, name, line string) error {
	k.notice(key, line)
	return fmt.Errorf("%w: %s", errChannelRefused, name)
}

// heldElsewhere answers what holds the channel for an orch other than orch.
// The answer is `orch <creel name>` for another orch of this hub, `hub <path>` for another live hub,
// and the empty string when none does.
// A map that cannot be read, or a failed live-hub read, fails closed:
// the error is returned and the hub's orch gets one notice naming the map's path or carrying the live-hub error as it is.
// The test sees only the hubs the live-hub list names, so a hub missing from it is unseen until it records itself.
func (k *channelKeeper) heldElsewhere(orch, channelID string) (string, error) {
	ownKey := "map:" + k.geom.ChannelMapPath
	own, err := ReadChannelMap(k.geom.ChannelMapPath)
	if err != nil {
		k.notice(ownKey, err.Error())
		return "", err
	}
	k.endNotice(ownKey)
	orchs := make([]string, 0, len(own.Channels))
	for held := range own.Channels {
		orchs = append(orchs, held)
	}
	sort.Strings(orchs)
	for _, held := range orchs {
		if held != orch && own.Channels[held].ID == channelID {
			return heldByOrchPrefix + held, nil
		}
	}

	live, err := k.liveHubs()
	if err != nil {
		k.notice(liveHubsNoticeKey, err.Error())
		return "", fmt.Errorf("slack: read the live hubs to judge channel %s: %w", channelID, err)
	}
	k.endNotice(liveHubsNoticeKey)
	for _, hub := range live {
		if hub.HubPath == k.geom.HubPath {
			continue
		}
		hubKey := "map:" + hub.ChannelMapPath
		other, err := ReadChannelMap(hub.ChannelMapPath)
		if err != nil {
			k.notice(hubKey, err.Error())
			return "", err
		}
		k.endNotice(hubKey)
		for _, entry := range other.Channels {
			if entry.ID == channelID {
				return heldByHubPrefix + hub.HubPath, nil
			}
		}
	}
	return "", nil
}

// join makes the bot a member of the orch's open channel after a post or a read was refused with `not_in_channel`.
// It joins only when heldElsewhere clears the channel.
func (k *channelKeeper) join(ctx context.Context, orch string) error {
	entry, err := k.openEntry(orch)
	if err != nil {
		return err
	}
	if entry.Archived {
		return fmt.Errorf("slack: the channel of %s is archived and is not joined", orch)
	}
	holder, err := k.heldElsewhere(orch, entry.ID)
	if err != nil {
		return err
	}
	if holder != "" {
		return k.refuse("refused:"+orch, entry.Name, fmt.Sprintf(
			"the channel %s (%s) is also held by %s; way forward: rename that channel in Slack, which frees the name",
			entry.Name, entry.ID, strings.TrimPrefix(strings.TrimPrefix(holder, heldByOrchPrefix), heldByHubPrefix)))
	}
	if err := k.slack.Join(ctx, entry.ID); err != nil {
		return fmt.Errorf("slack: join the channel %s: %w", entry.Name, err)
	}
	return nil
}

// inviteOperator invites the setup's operator into the orch's open channel.
// It records the operator in the entry once the invite stands.
// It does nothing when the setup names no operator or the entry already records it.
// An invite Slack refuses mails the hub's orch one notice per episode.
// The notice names the code and `lym slack setup --operator <user-id>`.
func (k *channelKeeper) inviteOperator(ctx context.Context, orch string) error {
	setup, found, err := ReadSetup(k.geom.SetupPath)
	if err != nil {
		return err
	}
	if !found || setup.OperatorID == "" {
		return nil
	}
	entry, err := k.openEntry(orch)
	if err != nil {
		return err
	}
	if entry.Archived || entry.Operator == setup.OperatorID {
		return nil
	}
	noticeKey := "invite:" + orch
	if err := k.slack.Invite(ctx, entry.ID, setup.OperatorID); err != nil {
		if refusal, refused := isAPIError(err); refused {
			k.notice(noticeKey, fmt.Sprintf(
				"Slack refused inviting the operator %s into %s with %s; way forward: run `lym slack setup --operator <user-id>` from the operator's terminal with a user id Slack accepts",
				setup.OperatorID, entry.Name, refusal.Code))
		}
		return fmt.Errorf("slack: invite the operator into %s: %w", entry.Name, err)
	}
	k.endNotice(noticeKey)
	return UpdateChannelMap(k.geom.ChannelMapPath, func(channelMap *ChannelMap) error {
		if current, held := channelMap.Channels[orch]; held && current.ID == entry.ID {
			current.Operator = setup.OperatorID
			channelMap.Channels[orch] = current
		}
		return nil
	})
}

// inviteOperators invites the setup's operator into every open entry that records another operator or none.
// It returns the joined errors of the invites that failed.
func (k *channelKeeper) inviteOperators(ctx context.Context) error {
	channelMap, err := ReadChannelMap(k.geom.ChannelMapPath)
	if err != nil {
		return err
	}
	orchs := make([]string, 0, len(channelMap.Channels))
	for orch, entry := range channelMap.Channels {
		if !entry.Archived {
			orchs = append(orchs, orch)
		}
	}
	sort.Strings(orchs)
	var failures []error
	for _, orch := range orchs {
		if err := k.inviteOperator(ctx, orch); err != nil {
			failures = append(failures, err)
		}
	}
	return errors.Join(failures...)
}

// retire renames and archives the channel of each open beam entry whose slug beams does not list.
// The prime's entry is never retired.
// The rename gives the channel its name with the UTC date, trying the next suffix while Slack answers `name_taken`,
// and the entry records the new name as soon as the rename stands; the archive then marks the entry archived.
// An entry already under a dated name is not renamed again.
// A rename or an archive Slack refuses leaves the entry open, returns the error and marks the orch refused;
// a later call skips a refused orch unless retryRefused is set.
// The caller calls it only after a read of the hub's beams that stood.
func (k *channelKeeper) retire(ctx context.Context, beams []string, retryRefused bool) error {
	channelMap, err := ReadChannelMap(k.geom.ChannelMapPath)
	if err != nil {
		return err
	}
	prime, err := orchName(k.geom.Shortname, "")
	if err != nil {
		return err
	}
	orchs := make([]string, 0, len(channelMap.Channels))
	for orch := range channelMap.Channels {
		orchs = append(orchs, orch)
	}
	sort.Strings(orchs)
	var failures []error
	for _, orch := range orchs {
		entry := channelMap.Channels[orch]
		if orch == prime || entry.Archived {
			continue
		}
		slug, err := slugOf(orch)
		if err != nil || slices.Contains(beams, slug) {
			continue
		}
		k.mu.Lock()
		skip := k.refused[orch] && !retryRefused
		k.mu.Unlock()
		if skip {
			continue
		}
		err = k.retireEntry(ctx, orch, slug, entry)
		k.mu.Lock()
		if err != nil {
			k.refused[orch] = true
		} else {
			delete(k.refused, orch)
		}
		k.mu.Unlock()
		if err != nil {
			failures = append(failures, err)
		}
	}
	return errors.Join(failures...)
}

// retireEntry renames the entry's channel to its dated name unless it already has one, then archives it.
func (k *channelKeeper) retireEntry(ctx context.Context, orch, slug string, entry ChannelEntry) error {
	baseName := ChannelName(k.geom.Shortname, k.geom.Machine, slug)
	if entry.Name == baseName {
		renamed, err := k.renameDated(ctx, entry.ID, baseName)
		if err != nil {
			return err
		}
		entry.Name = renamed
		if err := k.updateEntry(orch, entry.ID, func(current *ChannelEntry) { current.Name = renamed }); err != nil {
			return err
		}
	}
	if err := k.slack.Archive(ctx, entry.ID); err != nil {
		return fmt.Errorf("slack: archive the channel %s: %w", entry.Name, err)
	}
	return k.updateEntry(orch, entry.ID, func(current *ChannelEntry) { current.Archived = true })
}

// renameDated renames the channel to baseName with today's date, trying the next suffix while Slack answers `name_taken`.
// It returns the name the rename gave.
func (k *channelKeeper) renameDated(ctx context.Context, channelID, baseName string) (string, error) {
	for attempt := 1; attempt <= maxArchivedNameTries; attempt++ {
		dated := ArchivedName(baseName, k.clock.Now(), attempt)
		err := k.slack.Rename(ctx, channelID, dated)
		if err == nil {
			return dated, nil
		}
		if refusal, refused := isAPIError(err); !refused || refusal.Code != nameTakenCode {
			return "", fmt.Errorf("slack: rename the channel %s: %w", baseName, err)
		}
	}
	return "", fmt.Errorf("slack: rename the channel %s: every dated name up to attempt %d is taken", baseName, maxArchivedNameTries)
}

// updateEntry applies change to the orch's entry when it still holds the channel id.
func (k *channelKeeper) updateEntry(orch, channelID string, change func(*ChannelEntry)) error {
	return UpdateChannelMap(k.geom.ChannelMapPath, func(channelMap *ChannelMap) error {
		if current, held := channelMap.Channels[orch]; held && current.ID == channelID {
			change(&current)
			channelMap.Channels[orch] = current
		}
		return nil
	})
}

// botUser returns the Slack user id of this machine's bot, which auth.test gives; it is read once and kept.
func (k *channelKeeper) botUser(ctx context.Context) (string, error) {
	k.mu.Lock()
	known := k.botUserID
	k.mu.Unlock()
	if known != "" {
		return known, nil
	}
	identity, err := k.slack.AuthTest(ctx)
	if err != nil {
		return "", fmt.Errorf("slack: read the bot's user id: %w", err)
	}
	k.mu.Lock()
	k.botUserID = identity.UserID
	k.mu.Unlock()
	return identity.UserID, nil
}

// notice mails the hub's orch one `[slack relay]` notice with line for the episode key.
// Until endNotice ends the episode, a later notice of the same key does nothing.
func (k *channelKeeper) notice(key, line string) {
	k.mu.Lock()
	if k.noticed[key] {
		k.mu.Unlock()
		return
	}
	k.noticed[key] = true
	k.mu.Unlock()

	receiver, err := orchName(k.geom.Shortname, "")
	if err == nil {
		_, err = k.mailbox.Post(receiver, relayNoticePrefix+" "+line)
	}
	if err != nil {
		logger.Warn("slackengine: the relay notice was not posted", "receiver", receiver, "error", err)
		k.endNotice(key)
	}
}

// endNotice ends the episode of the notice key.
func (k *channelKeeper) endNotice(key string) {
	k.mu.Lock()
	defer k.mu.Unlock()
	delete(k.noticed, key)
}
