package server

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/sumiredc/killy/internal/server/push"
	"github.com/sumiredc/killy/internal/server/store"
)

// fakeSender stands in for Expo Push Service. It records every request; answer decides the
// tickets (all ok when nil), and hold, when set, keeps every request waiting until it is closed.
type fakeSender struct {
	mu     sync.Mutex
	calls  [][]push.Message
	answer func(msgs []push.Message) ([]push.Ticket, error)
	hold   chan struct{}
}

func (f *fakeSender) Send(ctx context.Context, msgs []push.Message) ([]push.Ticket, error) {
	f.mu.Lock()
	f.calls = append(f.calls, append([]push.Message(nil), msgs...))
	answer, hold := f.answer, f.hold
	f.mu.Unlock()
	if hold != nil {
		select {
		case <-hold:
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	if answer != nil {
		return answer(msgs)
	}
	return tickets(msgs, func(push.Message) push.Ticket { return ok }), nil
}

func (f *fakeSender) setAnswer(a func(msgs []push.Message) ([]push.Ticket, error)) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.answer = a
}

// sent returns every message sent so far, in order.
func (f *fakeSender) sent() []push.Message {
	f.mu.Lock()
	defer f.mu.Unlock()
	var all []push.Message
	for _, c := range f.calls {
		all = append(all, c...)
	}
	return all
}

func (f *fakeSender) requests() [][]push.Message {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([][]push.Message(nil), f.calls...)
}

// sentTo counts the messages sent to the push token.
func (f *fakeSender) sentTo(token string) int {
	n := 0
	for _, m := range f.sent() {
		if m.To == token {
			n++
		}
	}
	return n
}

var ok = push.Ticket{Status: "ok"}

func ticketError(code string) push.Ticket {
	t := push.Ticket{Status: "error", Message: code}
	t.Details.Error = code
	return t
}

func tickets(msgs []push.Message, each func(push.Message) push.Ticket) []push.Ticket {
	ts := make([]push.Ticket, len(msgs))
	for i, m := range msgs {
		ts[i] = each(m)
	}
	return ts
}

// report sends a snapshot of the parents, each later than the last, and waits for the pushes it
// started.
func (s *testServer) report(t *testing.T, agentToken string, parents ...string) {
	t.Helper()
	s.scans++
	if status, body := s.postSnapshot(t, agentToken, snapshotJSON(t0.Add(time.Duration(s.scans)*time.Second), strings.Join(parents, ","))); status != http.StatusNoContent {
		t.Fatalf("snapshot: %d %s", status, body)
	}
	s.pushes.Wait()
}

// withPush links a device to a new agent and gives it the push token.
func (s *testServer) withPush(t *testing.T, name, pushToken string) (agentToken, agentID, device string) {
	t.Helper()
	agentToken, agentID, device = s.linkedDevice(t, name)
	if status, body := s.patchDevice(t, device, `{"push_token":"`+pushToken+`"}`); status != http.StatusNoContent {
		t.Fatalf("push token: %d %s", status, body)
	}
	return
}

const tokenA, tokenB = "ExponentPushToken[A]", "ExponentPushToken[B]"

func failAll(msgs []push.Message) ([]push.Ticket, error) {
	return nil, errors.New("connection refused")
}

func TestAParentIsNotifiedOnceHoweverOftenItIsReported(t *testing.T) {
	s := newTestServer(t)
	agentToken, _, _ := s.withPush(t, "Prod web", tokenA)
	for range 3 {
		s.report(t, agentToken, parent100)
	}
	if n := s.sender.sentTo(tokenA); n != 1 {
		t.Errorf("%d pushes, want 1 for one parent reported 3 times", n)
	}
}

func TestAReusedPIDWithAnotherStartTimeIsANewParent(t *testing.T) {
	s := newTestServer(t)
	agentToken, _, _ := s.withPush(t, "Prod web", tokenA)
	s.report(t, agentToken, parent100)
	s.report(t, agentToken, `{"pid":100,"starttime":6000,"cmd":"node evil_daemon.js","zombies":[103]}`)
	if n := s.sender.sentTo(tokenA); n != 2 {
		t.Errorf("%d pushes, want 2: PID 100 with another start time is another parent", n)
	}
}

func TestARestartedServerDoesNotNotifyAgain(t *testing.T) {
	s := newTestServer(t)
	agentToken, _, _ := s.withPush(t, "Prod web", tokenA)
	s.report(t, agentToken, parent100)
	again := newTestServerOn(t, s.mem)
	again.report(t, agentToken, parent100)
	if n := again.sender.sentTo(tokenA); n != 0 {
		t.Errorf("%d pushes after the restart, want 0: the parent was notified before it", n)
	}
}

func TestAFailedSendIsNotRecordedAndIsSentAgainOnTheNextSnapshot(t *testing.T) {
	s := newTestServer(t)
	agentToken, agentID, _ := s.withPush(t, "Prod web", tokenA)
	s.sender.setAnswer(func(msgs []push.Message) ([]push.Ticket, error) {
		return tickets(msgs, func(push.Message) push.Ticket { return ticketError("MessageRateExceeded") }), nil
	})
	s.report(t, agentToken, parent100)
	if n := len(s.mem.Notified(agentID)); n != 0 {
		t.Fatalf("%d notified rows after a failed send, want 0", n)
	}
	s.sender.setAnswer(nil)
	s.report(t, agentToken, parent100)
	if n := s.sender.sentTo(tokenA); n != 2 {
		t.Errorf("%d pushes, want 2: the failed one sent again", n)
	}
	if n := len(s.mem.Notified(agentID)); n != 1 {
		t.Errorf("%d notified rows after the second send succeeded, want 1", n)
	}
	s.report(t, agentToken, parent100)
	if n := s.sender.sentTo(tokenA); n != 2 {
		t.Errorf("%d pushes, want 2: nothing after it succeeded", n)
	}
}

func TestAfterFourFailedSendsThePairIsGivenUpAndRecorded(t *testing.T) {
	s := newTestServer(t)
	agentToken, agentID, _ := s.withPush(t, "Prod web", tokenA)
	s.sender.setAnswer(failAll)
	for i := range push.MaxAttempts {
		s.report(t, agentToken, parent100)
		if n := s.sender.sentTo(tokenA); n != i+1 {
			t.Fatalf("after snapshot %d: %d pushes, want %d", i+1, n, i+1)
		}
	}
	if n := len(s.mem.Notified(agentID)); n != 1 {
		t.Errorf("%d notified rows after %d failures, want 1 (given up)", n, push.MaxAttempts)
	}
	s.report(t, agentToken, parent100)
	if n := s.sender.sentTo(tokenA); n != push.MaxAttempts {
		t.Errorf("%d pushes, want %d: none after giving up", n, push.MaxAttempts)
	}
}

func TestAFailedRequestCountsAgainstEveryPairInIt(t *testing.T) {
	s := newTestServer(t)
	agentToken, agentID, phoneA, phoneB := s.twoPhones(t)
	s.patchDevice(t, phoneA, `{"push_token":"`+tokenA+`"}`)
	s.patchDevice(t, phoneB, `{"push_token":"`+tokenB+`"}`)
	s.sender.setAnswer(failAll)
	for range push.MaxAttempts {
		s.report(t, agentToken, parent100)
	}
	if a, b := s.sender.sentTo(tokenA), s.sender.sentTo(tokenB); a != push.MaxAttempts || b != push.MaxAttempts {
		t.Errorf("pushes A %d, B %d; want %d each", a, b, push.MaxAttempts)
	}
	if n := len(s.mem.Notified(agentID)); n != 2 {
		t.Errorf("%d notified rows, want both pairs given up", n)
	}
}

func TestAnAnswerWithTheWrongNumberOfTicketsFailsEveryPair(t *testing.T) {
	s := newTestServer(t)
	agentToken, agentID, phoneA, phoneB := s.twoPhones(t)
	s.patchDevice(t, phoneA, `{"push_token":"`+tokenA+`"}`)
	s.patchDevice(t, phoneB, `{"push_token":"`+tokenB+`"}`)
	s.sender.setAnswer(func(msgs []push.Message) ([]push.Ticket, error) { return []push.Ticket{ok}, nil })
	s.report(t, agentToken, parent100)
	if n := len(s.mem.Notified(agentID)); n != 0 {
		t.Errorf("%d notified rows after too few tickets, want 0: a ticket cannot be matched to its pair", n)
	}
	s.sender.setAnswer(func(msgs []push.Message) ([]push.Ticket, error) { return []push.Ticket{ok, ok, ok}, nil })
	s.report(t, agentToken, parent100)
	if n := len(s.mem.Notified(agentID)); n != 0 {
		t.Errorf("%d notified rows after too many tickets, want 0", n)
	}
}

func TestATicketErrorCountsOnlyItsOwnPair(t *testing.T) {
	s := newTestServer(t)
	agentToken, agentID, phoneA, phoneB := s.twoPhones(t)
	s.patchDevice(t, phoneA, `{"push_token":"`+tokenA+`"}`)
	s.patchDevice(t, phoneB, `{"push_token":"`+tokenB+`"}`)
	s.sender.setAnswer(func(msgs []push.Message) ([]push.Ticket, error) {
		return tickets(msgs, func(m push.Message) push.Ticket {
			if m.To == tokenB {
				return ticketError("MessageTooBig")
			}
			return ok
		}), nil
	})
	s.report(t, agentToken, parent100)
	rows := s.mem.Notified(agentID)
	if len(rows) != 1 || rows[0].DeviceID != s.deviceRow(t, 0).ID {
		t.Fatalf("notified %+v, want phone A only", rows)
	}
	s.sender.setAnswer(nil)
	s.report(t, agentToken, parent100)
	if a, b := s.sender.sentTo(tokenA), s.sender.sentTo(tokenB); a != 1 || b != 2 {
		t.Errorf("pushes A %d, B %d; want A once, B sent again", a, b)
	}
}

func TestDeviceNotRegisteredRemovesThePushTokenUntilItIsRegisteredAgain(t *testing.T) {
	s := newTestServer(t)
	agentToken, agentID, device := s.withPush(t, "Prod web", tokenA)
	s.sender.setAnswer(func(msgs []push.Message) ([]push.Ticket, error) {
		return tickets(msgs, func(push.Message) push.Ticket { return ticketError(push.DeviceNotRegistered) }), nil
	})
	s.report(t, agentToken, parent100)
	if row := s.deviceRow(t, 0); row.PushToken != "" {
		t.Fatalf("push token %q, want it removed", row.PushToken)
	}
	if n := len(s.mem.Notified(agentID)); n != 0 {
		t.Errorf("%d notified rows, want 0: DeviceNotRegistered is neither notified nor a failure", n)
	}
	s.sender.setAnswer(nil)
	for range push.MaxAttempts {
		s.report(t, agentToken, parent100)
	}
	if n := s.sender.sentTo(tokenA); n != 1 {
		t.Fatalf("%d pushes, want 1: nothing is sent without a push token", n)
	}
	s.patchDevice(t, device, `{"push_token":"`+tokenA+`"}`)
	s.report(t, agentToken, parent100)
	if n := s.sender.sentTo(tokenA); n != 2 {
		t.Errorf("%d pushes, want 2: sent again once the token is registered again", n)
	}
}

func TestAPushTokenRegisteredAgainWhileSendingIsKept(t *testing.T) {
	s := newTestServer(t)
	agentToken, _, device := s.withPush(t, "Prod web", tokenA)
	s.sender.setAnswer(func(msgs []push.Message) ([]push.Ticket, error) {
		// The app registers a new token while Expo is answering for the old one.
		s.mem.UpdateDevice(context.Background(), s.deviceRow(t, 0).ID, ptr(tokenB), nil)
		return tickets(msgs, func(push.Message) push.Ticket { return ticketError(push.DeviceNotRegistered) }), nil
	})
	s.report(t, agentToken, parent100)
	if row := s.deviceRow(t, 0); row.PushToken != tokenB {
		t.Errorf("push token %q, want the new %q kept", row.PushToken, tokenB)
	}
	_ = device
}

func ptr[T any](v T) *T { return &v }

func TestAParentThatLeavesTheSnapshotStartsCountingFailuresAgain(t *testing.T) {
	s := newTestServer(t)
	agentToken, _, _ := s.withPush(t, "Prod web", tokenA)
	s.sender.setAnswer(failAll)
	for range push.MaxAttempts - 1 {
		s.report(t, agentToken, parent100)
	}
	s.report(t, agentToken) // the parent left
	for range push.MaxAttempts - 1 {
		s.report(t, agentToken, parent100)
	}
	// Without the reset this would be the 7th failure of one pair, long given up.
	s.report(t, agentToken, parent100)
	if n, want := s.sender.sentTo(tokenA), 2*push.MaxAttempts-1; n != want {
		t.Errorf("%d pushes, want %d: the count starts again once the parent left", n, want)
	}
}

func TestAParentThatLeavesAndComesBackIsNotifiedAgain(t *testing.T) {
	s := newTestServer(t)
	agentToken, _, _ := s.withPush(t, "Prod web", tokenA)
	s.report(t, agentToken, parent100)
	s.report(t, agentToken, parent100)
	s.report(t, agentToken)
	s.report(t, agentToken, parent100)
	if n := s.sender.sentTo(tokenA); n != 2 {
		t.Errorf("%d pushes, want 2: once per episode", n)
	}
}

func TestAParentThatLeftWhileTheServerWasDownIsNotifiedWhenItComesBack(t *testing.T) {
	s := newTestServer(t)
	agentToken, agentID, _ := s.withPush(t, "Prod web", tokenA)
	s.report(t, agentToken, parent100)
	again := newTestServerOn(t, s.mem)
	again.report(t, agentToken) // the first snapshot after the restart has no parent 100
	if n := len(again.mem.Notified(agentID)); n != 0 {
		t.Fatalf("%d notified rows, want the record gone with the parent", n)
	}
	again.report(t, agentToken, parent100)
	if n := again.sender.sentTo(tokenA); n != 1 {
		t.Errorf("%d pushes after it came back, want 1", n)
	}
}

func TestOnlyDevicesWithAPushTokenAndBothSwitchesOnAreNotified(t *testing.T) {
	for _, tc := range []struct {
		app, server, token bool
	}{
		{true, true, true}, {true, true, false}, {true, false, true}, {true, false, false},
		{false, true, true}, {false, true, false}, {false, false, true}, {false, false, false},
	} {
		t.Run(fmt.Sprintf("app=%v server=%v token=%v", tc.app, tc.server, tc.token), func(t *testing.T) {
			s := newTestServer(t)
			agentToken, agentID, device := s.linkedDevice(t, "Prod web")
			if tc.token {
				s.patchDevice(t, device, `{"push_token":"`+tokenA+`"}`)
			}
			s.patchDevice(t, device, fmt.Sprintf(`{"notify":%v}`, tc.app))
			s.patchServer(t, device, agentID, fmt.Sprintf(`{"notify":%v}`, tc.server))
			s.report(t, agentToken, parent100)
			want := 0
			if tc.app && tc.server && tc.token {
				want = 1
			}
			if n := len(s.sender.sent()); n != want {
				t.Errorf("%d pushes, want %d", n, want)
			}
		})
	}
}

func TestADeviceThatTurnsASwitchBackOnIsNotifiedOnTheNextSnapshot(t *testing.T) {
	for _, which := range []string{"app", "server"} {
		t.Run(which, func(t *testing.T) {
			s := newTestServer(t)
			agentToken, agentID, device := s.withPush(t, "Prod web", tokenA)
			turn := func(on bool) {
				if which == "app" {
					s.patchDevice(t, device, fmt.Sprintf(`{"notify":%v}`, on))
				} else {
					s.patchServer(t, device, agentID, fmt.Sprintf(`{"notify":%v}`, on))
				}
			}
			turn(false)
			s.report(t, agentToken, parent100)
			turn(true)
			s.report(t, agentToken, parent100)
			if n := s.sender.sentTo(tokenA); n != 1 {
				t.Errorf("%d pushes, want 1 for the parent that was there when the switch came back on", n)
			}
		})
	}
}

func TestADeviceLinkedLaterIsNotifiedOfAParentAlreadyThere(t *testing.T) {
	s := newTestServer(t)
	a := s.register(t)
	early := s.newDevice(t)
	s.claim(t, early, phoneAddr, claimBody(a.Code))
	s.patchDevice(t, early, `{"push_token":"`+tokenA+`"}`)
	s.report(t, *a.AgentToken, parent100)
	late := s.newDevice(t)
	_, again := s.pair(t, "192.0.2.1", *a.AgentToken, hostBody)
	s.claim(t, late, phoneAddr, claimBody(again.Code))
	s.patchDevice(t, late, `{"push_token":"`+tokenB+`"}`)
	s.report(t, *a.AgentToken, parent100)
	if a, b := s.sender.sentTo(tokenA), s.sender.sentTo(tokenB); a != 1 || b != 1 {
		t.Errorf("pushes early %d, late %d; want one each", a, b)
	}
}

func TestADeviceThatRemovesAndAddsTheServerAgainIsNotifiedAgain(t *testing.T) {
	s := newTestServer(t)
	a := s.register(t)
	device := s.newDevice(t)
	s.claim(t, device, phoneAddr, claimBody(a.Code))
	s.patchDevice(t, device, `{"push_token":"`+tokenA+`"}`)
	s.report(t, *a.AgentToken, parent100)
	s.deleteServer(t, device, a.AgentID)
	if n := len(s.mem.Notified(a.AgentID)); n != 0 {
		t.Fatalf("%d notified rows after the unlink, want 0", n)
	}
	_, again := s.pair(t, "192.0.2.1", *a.AgentToken, hostBody)
	s.claim(t, device, phoneAddr, claimBody(again.Code))
	s.report(t, *a.AgentToken, parent100)
	if n := s.sender.sentTo(tokenA); n != 2 {
		t.Errorf("%d pushes, want 2: a device linked again is like one linked later", n)
	}
}

func TestThePushHasTheDesignTextAndEachDevicesOwnName(t *testing.T) {
	s := newTestServer(t)
	agentToken, _, phoneA, phoneB := s.twoPhones(t)
	s.patchDevice(t, phoneA, `{"push_token":"`+tokenA+`"}`)
	s.patchDevice(t, phoneB, `{"push_token":"`+tokenB+`"}`)
	s.report(t, agentToken, parent100)
	want := map[string]push.Message{
		tokenA: {To: tokenA, Title: "Killy", Body: "zombie ×2 on Prod web\n……！", Sound: "default", Priority: "high"},
		tokenB: {To: tokenB, Title: "Killy", Body: "zombie ×2 on My box\n……！", Sound: "default", Priority: "high"},
	}
	got := s.sender.sent()
	if len(got) != 2 {
		t.Fatalf("pushes %+v, want 2", got)
	}
	for _, m := range got {
		if m != want[m.To] {
			t.Errorf("push %+v, want %+v", m, want[m.To])
		}
	}
}

func TestADiscardedSnapshotSendsNoPush(t *testing.T) {
	s := newTestServer(t)
	agentToken, _, _ := s.withPush(t, "Prod web", tokenA)
	s.sender.setAnswer(failAll)
	s.report(t, agentToken, parent100)
	// Scanned before the one already applied: Report discards it.
	if status, body := s.postSnapshot(t, agentToken, snapshotJSON(t0, parent100)); status != http.StatusNoContent {
		t.Fatalf("snapshot: %d %s", status, body)
	}
	s.pushes.Wait()
	if n := s.sender.sentTo(tokenA); n != 1 {
		t.Errorf("%d pushes, want 1: the discarded snapshot must not send the pending one again", n)
	}
}

func TestPushesGoInRequestsOfAtMost100(t *testing.T) {
	s := newTestServer(t)
	agentToken, _, phoneA, phoneB := s.twoPhones(t)
	s.patchDevice(t, phoneA, `{"push_token":"`+tokenA+`"}`)
	s.patchDevice(t, phoneB, `{"push_token":"`+tokenB+`"}`)
	parents := make([]string, 51)
	for i := range parents {
		parents[i] = fmt.Sprintf(`{"pid":%d,"starttime":1,"cmd":"x","zombies":[%d]}`, 1000+i, 2000+i)
	}
	s.report(t, agentToken, parents...)
	var sizes []int
	for _, r := range s.sender.requests() {
		sizes = append(sizes, len(r))
	}
	if fmt.Sprint(sizes) != "[100 2]" {
		t.Errorf("request sizes %v, want [100 2] for 102 pushes", sizes)
	}
}

func TestAPushExpoAcceptedButNotRecordedIsNotSentAgain(t *testing.T) {
	s := newTestServer(t)
	agentToken, agentID, _ := s.withPush(t, "Prod web", tokenA)
	s.mem.FailRecordNotified = true
	s.report(t, agentToken, parent100)
	s.report(t, agentToken, parent100)
	if n := s.sender.sentTo(tokenA); n != 1 {
		t.Errorf("%d pushes, want 1: Expo took it, only the record failed", n)
	}
	s.mem.FailRecordNotified = false
	s.report(t, agentToken, parent100)
	if n := len(s.mem.Notified(agentID)); n != 1 {
		t.Errorf("%d notified rows, want the record made on a later snapshot", n)
	}
	if n := s.sender.sentTo(tokenA); n != 1 {
		t.Errorf("%d pushes, want still 1", n)
	}
}

func TestASnapshotIsAnsweredWithoutWaitingForExpoAndSendingStartsAtOnce(t *testing.T) {
	s := newTestServer(t)
	agentToken, _, _ := s.withPush(t, "Prod web", tokenA)
	hold := make(chan struct{})
	s.sender.mu.Lock()
	s.sender.hold = hold
	s.sender.mu.Unlock()
	defer close(hold)
	done := make(chan int, 1)
	go func() {
		status, _ := s.postSnapshot(t, agentToken, snapshotJSON(t0.Add(time.Second), parent100))
		done <- status
	}()
	select {
	case status := <-done:
		if status != http.StatusNoContent {
			t.Errorf("snapshot: %d, want 204", status)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("the snapshot waited for Expo")
	}
	deadline := time.Now().Add(2 * time.Second)
	for len(s.sender.requests()) == 0 {
		if time.Now().After(deadline) {
			t.Fatal("no push was started for the snapshot")
		}
		time.Sleep(time.Millisecond)
	}
}

func TestNotifiedRowsAreForThisAgentsParentsOnly(t *testing.T) {
	s := newTestServer(t)
	tokA, agentA, _ := s.withPush(t, "a", tokenA)
	tokB, agentB, _ := s.withPush(t, "b", tokenB)
	s.report(t, tokA, parent100)
	s.report(t, tokB, parent200)
	a, b := s.mem.Notified(agentA), s.mem.Notified(agentB)
	if len(a) != 1 || a[0].Parent != (store.ParentKey{PID: 100, StartTime: 5000}) || len(b) != 1 || b[0].Parent != (store.ParentKey{PID: 200, StartTime: 7000}) {
		t.Errorf("notified A %+v, B %+v; want each agent's own parent", a, b)
	}
}

// holdSends makes every push wait until the returned func is called; it then lets them go and
// waits for the sends to end.
func (s *testServer) holdSends(t *testing.T) (release func()) {
	t.Helper()
	hold := make(chan struct{})
	s.sender.mu.Lock()
	s.sender.hold = hold
	s.sender.mu.Unlock()
	return func() {
		s.sender.mu.Lock()
		s.sender.hold = nil
		s.sender.mu.Unlock()
		close(hold)
		s.pushes.Wait()
	}
}

// postAndWaitForSend posts a snapshot and waits until a request to Expo has started for it.
func (s *testServer) postAndWaitForSend(t *testing.T, agentToken string, parents ...string) {
	t.Helper()
	before := len(s.sender.requests())
	s.scans++
	if status, body := s.postSnapshot(t, agentToken, snapshotJSON(t0.Add(time.Duration(s.scans)*time.Second), strings.Join(parents, ","))); status != http.StatusNoContent {
		t.Fatalf("snapshot: %d %s", status, body)
	}
	deadline := time.Now().Add(2 * time.Second)
	for len(s.sender.requests()) == before {
		if time.Now().After(deadline) {
			t.Fatal("no push was started for the snapshot")
		}
		time.Sleep(time.Millisecond)
	}
}

func TestASnapshotThatArrivesWhileSendingIsSentAfterwardsWithoutAnotherSnapshot(t *testing.T) {
	s := newTestServer(t)
	agentToken, _, _ := s.withPush(t, "Prod web", tokenA)
	release := s.holdSends(t)
	s.postAndWaitForSend(t, agentToken, parent100)
	s.scans++
	if status, body := s.postSnapshot(t, agentToken, snapshotJSON(t0.Add(time.Duration(s.scans)*time.Second), parent100+","+parent200)); status != http.StatusNoContent {
		t.Fatalf("snapshot: %d %s", status, body)
	}
	release()
	sent := s.sender.sent()
	if len(sent) != 2 || !strings.Contains(sent[1].Body, "zombie ×1") {
		t.Errorf("sent %+v, want parent 100 and then parent 200 (1 zombie)", sent)
	}
}

func TestNothingIsSentOnceTheNotifierIsClosed(t *testing.T) {
	s := newTestServer(t)
	agentToken, _, _ := s.withPush(t, "Prod web", tokenA)
	s.pushes.Close()
	s.report(t, agentToken, parent100)
	if n := s.sender.sentTo(tokenA); n != 0 {
		t.Errorf("%d pushes after Close, want 0", n)
	}
}

func TestAParentAcceptedButNotRecordedThatLeavesAndComesBackIsNotifiedAgain(t *testing.T) {
	s := newTestServer(t)
	agentToken, _, _ := s.withPush(t, "Prod web", tokenA)
	s.mem.FailRecordNotified = true
	s.report(t, agentToken, parent100)
	s.report(t, agentToken) // the parent left, and the record still fails
	s.mem.FailRecordNotified = false
	s.report(t, agentToken, parent100)
	if n := s.sender.sentTo(tokenA); n != 2 {
		t.Errorf("%d pushes, want 2: the parent came back, a new episode", n)
	}
}

func TestAPairWhoseDeviceIsUnlinkedWhileSendingIsNotRecorded(t *testing.T) {
	s := newTestServer(t)
	a := s.register(t)
	device := s.newDevice(t)
	s.claim(t, device, phoneAddr, claimBody(a.Code))
	s.patchDevice(t, device, `{"push_token":"`+tokenA+`"}`)
	release := s.holdSends(t)
	s.postAndWaitForSend(t, *a.AgentToken, parent100)
	s.deleteServer(t, device, a.AgentID)
	release()
	if rows := s.mem.Notified(a.AgentID); len(rows) != 0 {
		t.Errorf("notified %+v, want none: the link was gone when Expo answered", rows)
	}
}

func TestAPairWhoseParentLeavesWhileSendingIsNotRecorded(t *testing.T) {
	s := newTestServer(t)
	agentToken, agentID, _ := s.withPush(t, "Prod web", tokenA)
	release := s.holdSends(t)
	s.postAndWaitForSend(t, agentToken, parent100)
	s.scans++
	if status, body := s.postSnapshot(t, agentToken, snapshotJSON(t0.Add(time.Duration(s.scans)*time.Second), "")); status != http.StatusNoContent {
		t.Fatalf("snapshot: %d %s", status, body)
	}
	release()
	if rows := s.mem.Notified(agentID); len(rows) != 0 {
		t.Errorf("notified %+v, want none: the parent was gone when Expo answered", rows)
	}
}
