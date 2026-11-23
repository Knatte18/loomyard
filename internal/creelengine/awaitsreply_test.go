// awaitsreply_test.go pins awaits-reply: which questions a strand still waits on, in the store and through the daemon.

package creelengine_test

import (
	"testing"

	"github.com/Knatte18/loomyard/internal/creelengine"
	"github.com/Knatte18/loomyard/internal/testkit/creelfake"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

func TestStore_AwaitsReply(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()
	tests := []struct {
		name string
		// setup brings the store to the state the row asks alpha about.
		setup func(t *testing.T, e *env)
		want  bool
	}{
		{"AQuestionAStrandHoldsUnansweredIsAwaited", func(t *testing.T, e *env) {
			e.postMarked(t, strand(alpha), orch, "which schema?")
		}, true},
		{"AnAnswerTheAskerHasNotReadIsStillAwaited", func(t *testing.T, e *env) {
			id := e.postMarked(t, strand(alpha), orch, "which schema?")
			e.readBodies(t, strand(orch), "")
			if _, err := e.store.Reply(strand(orch), id, "v2"); err != nil {
				t.Fatalf("Reply: %v", err)
			}
		}, true},
		{"AnAnswerTheAskerReadEndsTheWait", func(t *testing.T, e *env) {
			id := e.postMarked(t, strand(alpha), orch, "which schema?")
			e.readBodies(t, strand(orch), "")
			if _, err := e.store.Reply(strand(orch), id, "v2"); err != nil {
				t.Fatalf("Reply: %v", err)
			}
			e.readBodies(t, strand(alpha), "")
		}, false},
		{"APlainMessageIsNoQuestion", func(t *testing.T, e *env) {
			e.post(t, strand(alpha), orch, "fyi")
		}, false},
		{"AQuestionAProgramHoldsIsNeverChasedSoItIsNotCounted", func(t *testing.T, e *env) {
			// A read makes batten's mailbox known, so it can hold mail.
			e.readBodies(t, program(batten), "")
			e.postMarked(t, strand(alpha), batten, "which schema?")
		}, false},
		{"AnotherStrandsQuestionIsNotTheAskers", func(t *testing.T, e *env) {
			e.postMarked(t, strand(beta), orch, "which schema?")
		}, false},
		{"AQuestionSettledUnansweredEndsTheWait", func(t *testing.T, e *env) {
			e.writeJournal(t, "2026-10-01",
				`{"event":"posted","at":"2026-10-01T12:00:00Z","id":1,"from":"`+alpha+`","to":"`+orch+`","kind":"mail","body":"which schema?","expect_reply":true}`,
				`{"event":"read","at":"2026-10-01T12:00:00Z","id":1,"by":"`+orch+`"}`,
				`{"event":"unanswered","at":"2026-10-01T12:30:00Z","id":1,"reason":"`+creelengine.UnansweredNoReply+`"}`,
			)
			e.reopen(t)
		}, false},
		{"AQuestionReturnedUnreadToItsAskerIsNotAwaited", func(t *testing.T, e *env) {
			e.writeJournal(t, "2026-10-01",
				`{"event":"posted","at":"2026-10-01T12:00:00Z","id":1,"from":"`+alpha+`","to":"`+orch+`","kind":"mail","body":"which schema?","expect_reply":true}`,
				`{"event":"returned","at":"2026-10-01T12:30:00Z","id":1,"to":"`+alpha+`","receiver":"`+orch+`"}`,
			)
			e.reopen(t)
		}, false},
		{"UnreadMailAnsweringAnotherStrandsQuestionIsNotTheAskers", func(t *testing.T, e *env) {
			id := e.postMarked(t, strand(beta), orch, "which schema?")
			e.readBodies(t, strand(orch), "")
			replyID, err := e.store.Reply(strand(orch), id, "v2")
			if err != nil {
				t.Fatalf("Reply: %v", err)
			}
			if err := e.store.Forward(strand(beta), replyID, alpha); err != nil {
				t.Fatalf("Forward: %v", err)
			}
		}, false},
		{"AQuestionItsRemovedAskerLeftIsNoLongerAwaited", func(t *testing.T, e *env) {
			_, waiter, err := e.store.PostWait(strand(alpha), orch, "which schema?")
			if err != nil {
				t.Fatalf("PostWait: %v", err)
			}
			e.start(t)
			e.del.SetStrands(creelfake.LiveStrand(beta), creelfake.LiveStrand(orch))
			e.sweepStep()
			e.sweepLast()
			if outcome := awaitOutcome(t, waiter); outcome.Kind != creelengine.OutcomeUnanswered {
				t.Fatalf("wait answered %v; want unanswered", outcome.Kind)
			}
		}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			e := newEnv(t)
			tt.setup(t, e)
			got, err := e.store.AwaitsReply(alpha)
			if err != nil {
				t.Fatalf("AwaitsReply(%q): %v", alpha, err)
			}
			if got != tt.want {
				t.Errorf("AwaitsReply(%q) = %v; want %v", alpha, got, tt.want)
			}
		})
	}

	t.Run("AnEmptyStrandIsRefused", func(t *testing.T) {
		t.Parallel()
		e := newEnv(t)
		_, err := e.store.AwaitsReply("")
		wantErr(t, err, "names no strand", "way forward")
	})
}

func TestDaemon_AwaitsReplyAnswersThroughTheProtocol(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()
	r := newRig(t, true)
	r.start()
	c := r.client(strandClaim(alpha), nil, creelfake.NewClock(epoch))
	mustCall(t, c, creelengine.Request{Verb: creelengine.VerbPost, Receiver: orch, Text: "which schema?", ExpectReply: true})
	for _, tt := range []struct {
		strand string
		want   bool
	}{{alpha, true}, {beta, false}} {
		if got := mustCall(t, c, creelengine.Request{Verb: creelengine.VerbAwaitsReply, From: tt.strand}).AwaitsReply; got != tt.want {
			t.Errorf("awaits-reply for %q = %v; want %v", tt.strand, got, tt.want)
		}
	}
}
