// keeper_test.go covers the channel keeper over the in-memory Slack, mailbox and clock, and the channel maps of two hubs.

package slackengine_test

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"

	"github.com/Knatte18/loomyard/internal/slackengine"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// keeperHarness is a channel keeper over a configured hub, a second live hub and a live-hub seam that can fail.
type keeperHarness struct {
	*harness
	other   slackengine.Geometry
	keeper  *slackengine.ChannelKeeper
	liveMu  sync.Mutex
	liveErr error
}

// newKeeperHarness returns a keeper for newHarness's hub whose live-hub seam names that hub and a second one.
func newKeeperHarness(t *testing.T) *keeperHarness {
	t.Helper()
	k := &keeperHarness{harness: newBareHarness(t), other: newGeometry(t)}
	k.keeper = slackengine.NewChannelKeeper(k.geom, k.slack, k.liveHubs, k.mailbox, k.clock)
	return k
}

func (k *keeperHarness) liveHubs() ([]slackengine.Geometry, error) {
	k.liveMu.Lock()
	defer k.liveMu.Unlock()
	if k.liveErr != nil {
		return nil, k.liveErr
	}
	return []slackengine.Geometry{k.geom, k.other}, nil
}

func (k *keeperHarness) failLiveHubs(err error) {
	k.liveMu.Lock()
	defer k.liveMu.Unlock()
	k.liveErr = err
}

// seedEntry records entry as the orch's entry in the hub's channel map.
func (k *keeperHarness) seedEntry(geom slackengine.Geometry, orch string, entry slackengine.ChannelEntry) {
	k.t.Helper()
	err := slackengine.UpdateChannelMap(geom.ChannelMapPath, func(channelMap *slackengine.ChannelMap) error {
		channelMap.Channels[orch] = entry
		return nil
	})
	if err != nil {
		k.t.Fatalf("UpdateChannelMap = %v", err)
	}
}

// entry returns the orch's entry in this hub's channel map, and reports whether the map holds one.
func (k *keeperHarness) entry(orch string) (slackengine.ChannelEntry, bool) {
	k.t.Helper()
	channelMap, err := slackengine.ReadChannelMap(k.geom.ChannelMapPath)
	if err != nil {
		k.t.Fatalf("ReadChannelMap = %v", err)
	}
	entry, held := channelMap.Channels[orch]
	return entry, held
}

// notices returns the bodies of the `[slack relay]` notices mailed to the hub's orch.
func (k *keeperHarness) notices() []string {
	var bodies []string
	for _, posted := range k.mailbox.accepted() {
		if posted.Receiver == "ly:orch" && strings.HasPrefix(posted.Body, "[slack relay]") {
			bodies = append(bodies, posted.Body)
		}
	}
	return bodies
}

// requireOneNoticeNaming fails the test unless the hub's orch holds exactly one notice, and that notice holds every text.
func (k *keeperHarness) requireOneNoticeNaming(texts ...string) {
	k.t.Helper()
	notices := k.notices()
	if len(notices) != 1 {
		k.t.Fatalf("notices = %q; want exactly one", notices)
	}
	for _, text := range texts {
		if !strings.Contains(notices[0], text) {
			k.t.Errorf("notice = %q; want it to name %q", notices[0], text)
		}
	}
}

// writeUndecodableMap writes a channel map that does not decode at the hub's path.
func writeUndecodableMap(t *testing.T, geom slackengine.Geometry) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(geom.ChannelMapPath), 0o700); err != nil {
		t.Fatalf("MkdirAll = %v", err)
	}
	if err := os.WriteFile(geom.ChannelMapPath, []byte("{"), 0o600); err != nil {
		t.Fatalf("WriteFile = %v", err)
	}
}

// TestChannelKeeper covers ensuring, adopting, joining, inviting and retiring the channels of a hub's orchs.
func TestChannelKeeper(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()
	ctx := context.Background()
	const primeOrch, paymentsOrch = "ly:orch", "ly:payments:orch"
	nameTaken := func(method string) error { return &slackengine.APIError{Method: method, Code: "name_taken"} }

	t.Run("ANewOrchGetsItsChannelBeforeTheOperatorIsInvited", func(t *testing.T) {
		t.Parallel()
		k := newKeeperHarness(t)
		var atInvite slackengine.ChannelEntry
		k.slack.onMethod = func(method string) {
			if method == "Invite" {
				atInvite, _ = k.entry(primeOrch)
			}
		}
		got, err := k.keeper.Ensure(ctx, primeOrch)
		want := slackengine.ChannelEntry{ID: "C1", Name: "ly-neve-prime", Operator: testOperator}
		if err != nil || !cmp.Equal(got, want) {
			t.Fatalf("Ensure(%s) = %+v, %v; want %+v", primeOrch, got, err, want)
		}
		if atInvite.ID != "C1" || atInvite.Cursor != "" || atInvite.Operator != "" {
			t.Errorf("entry at the invite = %+v; want the channel recorded with an empty cursor and no operator", atInvite)
		}
		if diff := cmp.Diff([]string{testOperator}, k.slack.invited["C1"]); diff != "" {
			t.Errorf("invited users mismatch (-want +got):\n%s", diff)
		}
	})

	t.Run("AnOpenEntryRecordingAnotherOperatorOrNoneIsInvitedAndNothingIsCreated", func(t *testing.T) {
		t.Parallel()
		for _, recorded := range []string{"U0", ""} {
			k := newKeeperHarness(t)
			k.seedEntry(k.geom, primeOrch, slackengine.ChannelEntry{ID: "C7", Name: "ly-neve-prime", Operator: recorded})
			got, err := k.keeper.Ensure(ctx, primeOrch)
			if err != nil || got.ID != "C7" || got.Operator != testOperator {
				t.Errorf("Ensure over an entry recording %q = %+v, %v; want C7 recording %s", recorded, got, err, testOperator)
			}
			if created := len(k.slack.logged("CreateChannel")); created != 0 {
				t.Errorf("CreateChannel calls over an entry recording %q = %d; want 0", recorded, created)
			}
			if diff := cmp.Diff([]string{testOperator}, k.slack.invited["C7"]); diff != "" {
				t.Errorf("invited users over an entry recording %q mismatch (-want +got):\n%s", recorded, diff)
			}
		}
	})

	t.Run("ANameTakenThatCannotBeJudgedReturnsItsErrorAndAdoptsNothing", func(t *testing.T) {
		t.Parallel()
		rows := []struct {
			name    string
			prepare func(k *keeperHarness)
			wantErr string
		}{
			{"AuthTestFails", func(k *keeperHarness) {
				k.slack.seedChannel(slackengine.ChannelInfo{ID: "CX", Name: "ly-neve-prime", Creator: "UBOT"})
				k.slack.failNext("AuthTest", errors.New("auth is down"))
			}, "auth is down"},
			{"TheTakenNameIsNoLongerListed", func(k *keeperHarness) {
				k.slack.failNext("CreateChannel", nameTaken("conversations.create"))
			}, "ly-neve-prime"},
		}
		for _, row := range rows {
			t.Run(row.name, func(t *testing.T) {
				t.Parallel()
				k := newKeeperHarness(t)
				row.prepare(k)
				_, err := k.keeper.Ensure(ctx, primeOrch)
				if err == nil || !strings.Contains(err.Error(), row.wantErr) {
					t.Errorf("Ensure = %v; want an error naming %q", err, row.wantErr)
				}
				if joins := len(k.slack.logged("Join")); joins != 0 {
					t.Errorf("Join calls = %d; want 0", joins)
				}
				if _, held := k.entry(primeOrch); held {
					t.Errorf("the map holds an entry for %s; want none", primeOrch)
				}
			})
		}
	})

	t.Run("AnEnsuredOrchIsNotLookedUpAgainAndConcurrentEnsuresMakeOneChannel", func(t *testing.T) {
		t.Parallel()
		k := newKeeperHarness(t)
		var wg sync.WaitGroup
		for range 2 {
			wg.Add(1)
			go func() {
				defer wg.Done()
				if _, err := k.keeper.Ensure(ctx, primeOrch); err != nil {
					t.Errorf("Ensure = %v", err)
				}
			}()
		}
		wg.Wait()
		if _, err := k.keeper.Ensure(ctx, primeOrch); err != nil {
			t.Fatalf("a later Ensure = %v", err)
		}
		if created := len(k.slack.logged("CreateChannel")); created != 1 {
			t.Errorf("CreateChannel calls = %d; want 1", created)
		}
		if found := len(k.slack.logged("FindChannel")); found != 0 {
			t.Errorf("FindChannel calls = %d; want 0", found)
		}
	})

	t.Run("ATakenNameOfTheBotsLiveChannelIsJoinedAndAdoptedAtItsNewestMessage", func(t *testing.T) {
		t.Parallel()
		emptyChannelStart := fmt.Sprintf("%d.000000", time.Date(2026, 11, 5, 12, 0, 0, 0, time.UTC).Unix())
		rows := []struct {
			name       string
			messages   []string
			wantCursor string
		}{
			{"WithMessages", []string{"1700000000.000100", "1700000000.000300"}, "1700000000.000300"},
			{"EmptyChannelStartsAtNow", nil, emptyChannelStart},
		}
		for _, row := range rows {
			t.Run(row.name, func(t *testing.T) {
				t.Parallel()
				k := newKeeperHarness(t)
				k.slack.seedChannel(slackengine.ChannelInfo{ID: "CX", Name: "ly-neve-prime", Creator: "UBOT"})
				for _, ts := range row.messages {
					k.slack.add("CX", slackengine.Message{TS: ts, User: "U9"})
				}
				got, err := k.keeper.Ensure(ctx, primeOrch)
				want := slackengine.ChannelEntry{ID: "CX", Name: "ly-neve-prime", Cursor: row.wantCursor, ThreadFloor: row.wantCursor, Operator: testOperator}
				if err != nil || !cmp.Equal(got, want) {
					t.Fatalf("Ensure = %+v, %v; want %+v", got, err, want)
				}
				joins := k.slack.logged("Join")
				if len(joins) != 1 || joins[0].Channel != "CX" {
					t.Errorf("Join calls = %+v; want one for CX", joins)
				}
			})
		}
	})

	t.Run("ATakenNameThatIsNotTheBotsToAdoptIsRefusedWithOneNoticePerEpisode", func(t *testing.T) {
		t.Parallel()
		rows := []struct {
			name    string
			prepare func(k *keeperHarness)
			// wantNotice holds the texts the notice names; an empty text stands for the second hub's path.
			wantNotice []string
		}{
			{"ArchivedChannel", func(k *keeperHarness) {
				k.slack.seedChannel(slackengine.ChannelInfo{ID: "CX", Name: "ly-neve-prime", Creator: "UBOT", IsArchived: true})
			}, []string{"ly-neve-prime", "rename that channel in Slack"}},
			{"AnotherUsersChannel", func(k *keeperHarness) {
				k.slack.seedChannel(slackengine.ChannelInfo{ID: "CX", Name: "ly-neve-prime", Creator: "UOTHER", IsMember: true})
			}, []string{"ly-neve-prime", "rename that channel in Slack"}},
			{"ChannelOfTheSecondHubsMap", func(k *keeperHarness) {
				k.slack.seedChannel(slackengine.ChannelInfo{ID: "CX", Name: "ly-neve-prime", Creator: "UBOT", IsMember: true})
				k.seedEntry(k.other, primeOrch, slackengine.ChannelEntry{ID: "CX", Name: "ly-neve-prime"})
			}, []string{"", "remove the extra clone of the hub"}},
			{"ChannelOfAnotherOrchOfThisHub", func(k *keeperHarness) {
				k.slack.seedChannel(slackengine.ChannelInfo{ID: "CX", Name: "ly-neve-prime", Creator: "UBOT", IsMember: true})
				k.seedEntry(k.geom, paymentsOrch, slackengine.ChannelEntry{ID: "CX", Name: "ly-neve-prime"})
			}, []string{paymentsOrch, "rename that channel in Slack"}},
		}
		for _, row := range rows {
			t.Run(row.name, func(t *testing.T) {
				t.Parallel()
				k := newKeeperHarness(t)
				row.prepare(k)
				for attempt := 1; attempt <= 2; attempt++ {
					if _, err := k.keeper.Ensure(ctx, primeOrch); !errors.Is(err, slackengine.ErrChannelRefused) {
						t.Fatalf("Ensure #%d = %v; want an error wrapping the refusal", attempt, err)
					}
				}
				var wantNotice []string
				for _, text := range row.wantNotice {
					if text == "" {
						text = k.other.HubPath
					}
					wantNotice = append(wantNotice, text)
				}
				k.requireOneNoticeNaming(wantNotice...)
				for _, method := range []string{"Join", "Rename", "Archive"} {
					if calls := len(k.slack.logged(method)); calls != 0 {
						t.Errorf("%s calls = %d; want 0", method, calls)
					}
				}
				if _, held := k.entry(primeOrch); held {
					t.Errorf("the map holds an entry for %s; want none", primeOrch)
				}
			})
		}
	})

	t.Run("AMapOrALiveHubReadThatFailsClosesAdoptAndJoinWithOneNoticeNamingIt", func(t *testing.T) {
		t.Parallel()
		const hubsRecord = "/machine/hubs.json"
		rows := []struct {
			name       string
			prepare    func(k *keeperHarness)
			wantNotice func(k *keeperHarness) string
		}{
			{"SecondHubMapDoesNotDecode", func(k *keeperHarness) { writeUndecodableMap(k.t, k.other) },
				func(k *keeperHarness) string { return k.other.ChannelMapPath }},
			{"LiveHubsReadFails", func(k *keeperHarness) {
				k.failLiveHubs(fmt.Errorf("read the hubs record %s: boom", hubsRecord))
			}, func(*keeperHarness) string { return hubsRecord }},
		}
		for _, row := range rows {
			t.Run(row.name, func(t *testing.T) {
				t.Parallel()
				k := newKeeperHarness(t)
				k.slack.seedChannel(slackengine.ChannelInfo{ID: "CX", Name: "ly-neve-prime", Creator: "UBOT"})
				k.seedEntry(k.geom, paymentsOrch, slackengine.ChannelEntry{ID: "CP", Name: "ly-neve-payments"})
				row.prepare(k)

				if _, err := k.keeper.Ensure(ctx, primeOrch); err == nil {
					t.Errorf("Ensure = nil; want an error while the judgment fails")
				}
				if err := k.keeper.Join(ctx, paymentsOrch); err == nil {
					t.Errorf("Join = nil; want an error while the judgment fails")
				}
				if joins := len(k.slack.logged("Join")); joins != 0 {
					t.Errorf("Join calls = %d; want 0", joins)
				}
				if _, held := k.entry(primeOrch); held {
					t.Errorf("the map holds an entry for %s; want none", primeOrch)
				}
				k.requireOneNoticeNaming(row.wantNotice(k))
			})
		}
	})

	t.Run("AnArchivedEntryGetsANewChannelUnderItsNameAndNothingCanUnarchive", func(t *testing.T) {
		t.Parallel()
		k := newKeeperHarness(t)
		k.seedEntry(k.geom, primeOrch, slackengine.ChannelEntry{ID: "C9", Name: "ly-neve-prime-2026-11-01", Archived: true, Operator: testOperator})
		got, err := k.keeper.Ensure(ctx, primeOrch)
		want := slackengine.ChannelEntry{ID: "C1", Name: "ly-neve-prime", Operator: testOperator}
		if err != nil || !cmp.Equal(got, want) {
			t.Errorf("Ensure over an archived entry = %+v, %v; want %+v", got, err, want)
		}
		if stored, _ := k.entry(primeOrch); !cmp.Equal(stored, want) {
			t.Errorf("stored entry = %+v; want the new channel to replace the archived one: %+v", stored, want)
		}
		clientType := reflect.TypeOf((*slackengine.SlackClient)(nil)).Elem()
		for i := range clientType.NumMethod() {
			if name := clientType.Method(i).Name; strings.Contains(strings.ToLower(name), "unarchive") {
				t.Errorf("the Slack seam has the method %s; want none that unarchives", name)
			}
		}
	})

	t.Run("InviteOperatorsRetriesARefusedInviteAndNoticesOncePerEpisode", func(t *testing.T) {
		t.Parallel()
		k := newKeeperHarness(t)
		k.seedEntry(k.geom, primeOrch, slackengine.ChannelEntry{ID: "C5", Name: "ly-neve-prime", Operator: "U0"})
		refused := &slackengine.APIError{Method: "conversations.invite", Code: "user_not_found"}
		k.slack.failNext("Invite", refused, refused)

		for call := 1; call <= 2; call++ {
			if err := k.keeper.InviteOperators(ctx); !errors.Is(err, refused) {
				t.Errorf("InviteOperators #%d = %v; want the refusal", call, err)
			}
		}
		k.requireOneNoticeNaming("user_not_found", "lym slack setup --operator <user-id>")
		if stored, _ := k.entry(primeOrch); stored.Operator != "U0" {
			t.Errorf("operator after refused invites = %q; want U0", stored.Operator)
		}

		if err := k.keeper.InviteOperators(ctx); err != nil {
			t.Fatalf("InviteOperators #3 = %v", err)
		}
		if stored, _ := k.entry(primeOrch); stored.Operator != testOperator {
			t.Errorf("operator after the invite stood = %q; want %s", stored.Operator, testOperator)
		}
		if got := len(k.notices()); got != 1 {
			t.Errorf("notices after the invite stood = %d; want the one of the episode", got)
		}
	})

	t.Run("Retire", func(t *testing.T) {
		t.Parallel()
		const datedName, baseName = "ly-neve-payments-2026-11-05", "ly-neve-payments"
		newRetireHarness := func(t *testing.T) *keeperHarness {
			k := newKeeperHarness(t)
			k.slack.seedChannel(slackengine.ChannelInfo{ID: "C1", Name: "ly-neve-prime", Creator: "UBOT", IsMember: true})
			k.slack.seedChannel(slackengine.ChannelInfo{ID: "C2", Name: baseName, Creator: "UBOT", IsMember: true})
			k.seedEntry(k.geom, primeOrch, slackengine.ChannelEntry{ID: "C1", Name: "ly-neve-prime"})
			k.seedEntry(k.geom, paymentsOrch, slackengine.ChannelEntry{ID: "C2", Name: baseName})
			return k
		}
		requireArchivedUnder := func(t *testing.T, k *keeperHarness, name string) {
			t.Helper()
			info, found, err := k.slack.FindChannel(ctx, name)
			if err != nil || !found || !info.IsArchived || info.ID != "C2" {
				t.Errorf("FindChannel(%s) = %+v, %v, %v; want the archived channel C2", name, info, found, err)
			}
			if stored, _ := k.entry(paymentsOrch); stored.Name != name || !stored.Archived {
				t.Errorf("stored entry = %+v; want it archived under %s", stored, name)
			}
		}

		t.Run("ABeamNoLongerListedIsRenamedWithTheDateAndArchivedAndThePrimeNever", func(t *testing.T) {
			t.Parallel()
			k := newRetireHarness(t)
			if err := k.keeper.Retire(ctx, []string{"payments"}, false); err != nil || k.slack.callCount() != 0 {
				t.Fatalf("Retire with the beam listed = %v after %d Slack calls; want none", err, k.slack.callCount())
			}
			if err := k.keeper.Retire(ctx, nil, false); err != nil {
				t.Fatalf("Retire = %v", err)
			}
			requireArchivedUnder(t, k, datedName)
			if stored, _ := k.entry(primeOrch); stored.Name != "ly-neve-prime" || stored.Archived {
				t.Errorf("the prime's entry = %+v; want it untouched", stored)
			}
			for _, call := range k.slack.logged("Rename") {
				if call.Channel == "C1" {
					t.Errorf("Rename of the prime's channel was called")
				}
			}
		})

		t.Run("ARenameRefusedAsTakenGoesOnToTheNextSuffix", func(t *testing.T) {
			t.Parallel()
			k := newRetireHarness(t)
			k.slack.seedChannel(slackengine.ChannelInfo{ID: "C9", Name: datedName})
			if err := k.keeper.Retire(ctx, nil, false); err != nil {
				t.Fatalf("Retire = %v", err)
			}
			requireArchivedUnder(t, k, datedName+"-2")
		})

		t.Run("ARefusedArchiveLeavesTheEntryOpenUnderItsDatedNameUntilRetried", func(t *testing.T) {
			t.Parallel()
			k := newRetireHarness(t)
			k.slack.failNext("Archive", errors.New("slack is down"))
			if err := k.keeper.Retire(ctx, nil, false); err == nil {
				t.Fatalf("Retire = nil; want the archive's error")
			}
			if stored, _ := k.entry(paymentsOrch); stored.Name != datedName || stored.Archived {
				t.Errorf("stored entry = %+v; want it open under %s", stored, datedName)
			}
			callsBefore := k.slack.callCount()
			if err := k.keeper.Retire(ctx, nil, false); err != nil || k.slack.callCount() != callsBefore {
				t.Errorf("Retire without retryRefused = %v after %d new Slack calls; want none", err, k.slack.callCount()-callsBefore)
			}
			if err := k.keeper.Retire(ctx, nil, true); err != nil {
				t.Fatalf("Retire with retryRefused = %v", err)
			}
			requireArchivedUnder(t, k, datedName)
			if renames := len(k.slack.logged("Rename")); renames != 1 {
				t.Errorf("Rename calls = %d; want the one before the refused archive", renames)
			}
		})

		t.Run("ARenameRefusedWithAnotherCodeIsRetriedOnlyWithRetryRefused", func(t *testing.T) {
			t.Parallel()
			k := newRetireHarness(t)
			k.slack.failNext("Rename", &slackengine.APIError{Method: "conversations.rename", Code: "not_authed"})
			if err := k.keeper.Retire(ctx, nil, false); err == nil {
				t.Fatalf("Retire = nil; want the rename's error")
			}
			if stored, _ := k.entry(paymentsOrch); stored.Name != baseName || stored.Archived {
				t.Errorf("stored entry = %+v; want it open under %s", stored, baseName)
			}
			callsBefore := k.slack.callCount()
			if err := k.keeper.Retire(ctx, nil, false); err != nil || k.slack.callCount() != callsBefore {
				t.Errorf("Retire without retryRefused = %v after %d new Slack calls; want none", err, k.slack.callCount()-callsBefore)
			}
			if err := k.keeper.Retire(ctx, nil, true); err != nil {
				t.Fatalf("Retire with retryRefused = %v", err)
			}
			requireArchivedUnder(t, k, datedName)
		})
	})
}
