package server

import (
	"bufio"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/sumiredc/killy/internal/server/live"
	"github.com/sumiredc/killy/internal/server/sse"
)

type stateMsg struct {
	Servers []struct {
		ID           string `json:"id"`
		Name         string `json:"name"`
		Hostname     string `json:"hostname"`
		Notify       bool   `json:"notify"`
		Disconnected bool   `json:"disconnected"`
		Parents      []struct {
			PID             int       `json:"pid"`
			StartTime       int64     `json:"starttime"`
			Cmd             string    `json:"cmd"`
			Zombies         []int     `json:"zombies"`
			FirstDetectedAt time.Time `json:"first_detected_at"`
		} `json:"parents"`
	} `json:"servers"`
}

type sseItem struct {
	comment string
	event   string
	data    string
}

type eventStream struct {
	res   *http.Response
	items chan sseItem
}

// openEvents connects to GET /app/v1/events on a real HTTP server running the handler.
func (s *testServer) openEvents(t *testing.T, deviceToken string) *eventStream {
	t.Helper()
	ts := httptest.NewServer(s.Handler())
	t.Cleanup(ts.Close)
	req, _ := http.NewRequest("GET", ts.URL+"/app/v1/events", nil)
	req.Header.Set("Authorization", "Bearer "+deviceToken)
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { res.Body.Close() })
	es := &eventStream{res: res, items: make(chan sseItem, 64)}
	go func() {
		sc := bufio.NewScanner(res.Body)
		var it sseItem
		for sc.Scan() {
			line := sc.Text()
			switch {
			case line == "":
				if it != (sseItem{}) {
					es.items <- it
				}
				it = sseItem{}
			case strings.HasPrefix(line, ":"):
				it.comment = strings.TrimSpace(line[1:])
			case strings.HasPrefix(line, "event: "):
				it.event = line[len("event: "):]
			case strings.HasPrefix(line, "data: "):
				it.data = line[len("data: "):]
			}
		}
		close(es.items)
	}()
	return es
}

func (es *eventStream) next(t *testing.T) sseItem {
	t.Helper()
	select {
	case it, ok := <-es.items:
		if !ok {
			t.Fatal("the stream ended")
		}
		return it
	case <-time.After(2 * time.Second):
		t.Fatal("nothing arrived on the stream within 2 s")
		return sseItem{}
	}
}

func (es *eventStream) nextState(t *testing.T) stateMsg {
	t.Helper()
	it := es.next(t)
	if it.event != "state" {
		t.Fatalf("got %+v, want a state event", it)
	}
	var m stateMsg
	if err := json.Unmarshal([]byte(it.data), &m); err != nil {
		t.Fatalf("state data %q: %v", it.data, err)
	}
	return m
}

func (es *eventStream) quiet(t *testing.T) {
	t.Helper()
	select {
	case it := <-es.items:
		t.Errorf("got %+v, want nothing", it)
	case <-time.After(150 * time.Millisecond):
	}
}

// linkedDevice registers an agent and a device and links them; it returns the agent's token and
// ID and the device token.
func (s *testServer) linkedDevice(t *testing.T, name string) (agentToken, agentID, device string) {
	t.Helper()
	a := s.register(t)
	device = s.newDevice(t)
	if status, resp := s.claim(t, device, phoneAddr, claimBody(a.Code, name)); status != http.StatusOK {
		t.Fatalf("claim: %d %+v", status, resp)
	}
	return *a.AgentToken, a.AgentID, device
}

func TestTheFirstEventIsTheCurrentState(t *testing.T) {
	s := newTestServer(t)
	agentToken, agentID, device := s.linkedDevice(t, "Prod web")
	s.postSnapshot(t, agentToken, snapshotJSON(t0, parent100))
	es := s.openEvents(t, device)
	m := es.nextState(t)
	if len(m.Servers) != 1 {
		t.Fatalf("servers %+v, want 1", m.Servers)
	}
	sv := m.Servers[0]
	if sv.ID != agentID || sv.Name != "Prod web" || sv.Hostname != "web-1" || !sv.Notify || sv.Disconnected {
		t.Errorf("server %+v, want id %s, Prod web, web-1, notify on, connected", sv, agentID)
	}
	if len(sv.Parents) != 1 || sv.Parents[0].PID != 100 || sv.Parents[0].StartTime != 5000 || sv.Parents[0].Cmd != "node evil_daemon.js" ||
		len(sv.Parents[0].Zombies) != 2 || !sv.Parents[0].FirstDetectedAt.Equal(t0) {
		t.Errorf("parents %+v, want parent 100 with 2 zombies first detected at %v", sv.Parents, t0)
	}
}

func TestTheServerIDIsTheAgentIDAndTheClaimedID(t *testing.T) {
	s := newTestServer(t)
	a := s.register(t)
	device := s.newDevice(t)
	_, claimed := s.claim(t, device, phoneAddr, claimBody(a.Code))
	m := s.openEvents(t, device).nextState(t)
	if len(m.Servers) != 1 || m.Servers[0].ID != a.AgentID || m.Servers[0].ID != claimed.ID {
		t.Errorf("state ids %+v, want %s from both the agent and the claim (%s)", m.Servers, a.AgentID, claimed.ID)
	}
}

func TestAServerThatNeverReportedIsSentDisconnectedWithNoParents(t *testing.T) {
	s := newTestServer(t)
	_, _, device := s.linkedDevice(t, "")
	m := s.openEvents(t, device).nextState(t)
	if len(m.Servers) != 1 || !m.Servers[0].Disconnected || m.Servers[0].Parents == nil || len(m.Servers[0].Parents) != 0 {
		t.Errorf("servers %+v, want one disconnected with an empty parent list", m.Servers)
	}
}

func TestADeviceWithoutLinksGetsAnEmptyList(t *testing.T) {
	s := newTestServer(t)
	it := s.openEvents(t, s.newDevice(t)).next(t)
	if it.event != "state" || it.data != `{"servers":[]}` {
		t.Errorf("got %+v, want state {\"servers\":[]}", it)
	}
}

func TestASnapshotSendsStateOnlyToTheDevicesLinkedToItsAgent(t *testing.T) {
	s := newTestServer(t)
	agentToken, agentID, linked := s.linkedDevice(t, "")
	_, otherAgent, unlinked := s.linkedDevice(t, "")
	a, b := s.openEvents(t, linked), s.openEvents(t, unlinked)
	a.nextState(t)
	b.nextState(t)

	s.postSnapshot(t, agentToken, snapshotJSON(t0, parent100))
	m := a.nextState(t)
	if len(m.Servers) != 1 || m.Servers[0].ID != agentID || len(m.Servers[0].Parents) != 1 {
		t.Errorf("linked device got %+v, want its server with parent 100", m.Servers)
	}
	b.quiet(t)
	_ = otherAgent
}

func TestTheStateNeverIncludesServersTheDeviceIsNotLinkedTo(t *testing.T) {
	s := newTestServer(t)
	agentToken, agentID, _ := s.linkedDevice(t, "")
	s.postSnapshot(t, agentToken, snapshotJSON(t0, parent100))
	_, otherAgent, device := s.linkedDevice(t, "")
	m := s.openEvents(t, device).nextState(t)
	if len(m.Servers) != 1 || m.Servers[0].ID != otherAgent {
		t.Errorf("servers %+v, want only %s (not %s)", m.Servers, otherAgent, agentID)
	}
}

func TestADiscardedSnapshotSendsNothing(t *testing.T) {
	s := newTestServer(t)
	agentToken, _, device := s.linkedDevice(t, "")
	s.postSnapshot(t, agentToken, snapshotJSON(t0.Add(time.Minute), parent100))
	es := s.openEvents(t, device)
	es.nextState(t)
	s.postSnapshot(t, agentToken, snapshotJSON(t0, parent200))
	es.quiet(t)
}

func TestEachDeviceSeesItsOwnNameAndNotificationSwitch(t *testing.T) {
	s := newTestServer(t)
	a := s.register(t)
	phoneA, phoneB := s.newDevice(t), s.newDevice(t)
	s.claim(t, phoneA, phoneAddr, claimBody(a.Code, "Prod web"))
	_, again := s.pair(t, "192.0.2.1", *a.AgentToken, hostBody)
	s.claim(t, phoneB, phoneAddr, claimBody(again.Code, "My box"))
	idB := s.mem.Devices()[1].ID
	s.mem.SetLink(idB, a.AgentID, "My box", false)

	mA := s.openEvents(t, phoneA).nextState(t)
	mB := s.openEvents(t, phoneB).nextState(t)
	if sv := mA.Servers[0]; sv.Name != "Prod web" || !sv.Notify {
		t.Errorf("phone A sees %+v, want Prod web with notifications on", sv)
	}
	if sv := mB.Servers[0]; sv.Name != "My box" || sv.Notify {
		t.Errorf("phone B sees %+v, want My box with notifications off", sv)
	}
}

func TestAnAgentGoingQuietAndComingBackSendsState(t *testing.T) {
	s := newTestServer(t)
	agentToken, _, device := s.linkedDevice(t, "")
	s.postSnapshot(t, agentToken, snapshotJSON(t0, parent100))
	es := s.openEvents(t, device)
	es.nextState(t)
	s.CheckConnections(t.Context())
	es.quiet(t)

	s.clock.Advance(live.DisconnectedAfter)
	s.CheckConnections(t.Context())
	if m := es.nextState(t); !m.Servers[0].Disconnected {
		t.Errorf("after 60 s: %+v, want disconnected", m.Servers[0])
	}
	s.postSnapshot(t, agentToken, snapshotJSON(t0.Add(live.DisconnectedAfter), parent100))
	if m := es.nextState(t); m.Servers[0].Disconnected {
		t.Errorf("after a new snapshot: %+v, want connected", m.Servers[0])
	}
}

func TestAClaimSendsStateToThatDeviceOnly(t *testing.T) {
	s := newTestServer(t)
	device, other := s.newDevice(t), s.newDevice(t)
	es, otherStream := s.openEvents(t, device), s.openEvents(t, other)
	es.nextState(t)
	otherStream.nextState(t)
	a := s.register(t)
	s.claim(t, device, phoneAddr, claimBody(a.Code, "New"))
	if m := es.nextState(t); len(m.Servers) != 1 || m.Servers[0].Name != "New" {
		t.Errorf("after the claim: %+v, want the new server", m.Servers)
	}
	otherStream.quiet(t)
}

func TestNotifyDeviceSendsStateToThatDeviceOnly(t *testing.T) {
	s := newTestServer(t)
	_, agentID, device := s.linkedDevice(t, "Old")
	_, _, other := s.linkedDevice(t, "")
	es, otherStream := s.openEvents(t, device), s.openEvents(t, other)
	es.nextState(t)
	otherStream.nextState(t)
	s.mem.SetLink(s.mem.Links()[0].DeviceID, agentID, "Renamed", true)
	s.NotifyDevice(s.mem.Links()[0].DeviceID)
	if m := es.nextState(t); m.Servers[0].Name != "Renamed" {
		t.Errorf("after NotifyDevice: %+v, want Renamed", m.Servers)
	}
	otherStream.quiet(t)
}

func TestAHeartbeatIsSentOnEveryInterval(t *testing.T) {
	s := newTestServer(t)
	s.HeartbeatInterval = 30 * time.Millisecond
	es := s.openEvents(t, s.newDevice(t))
	es.nextState(t)
	for range 2 {
		if it := es.next(t); it.comment != "hb" {
			t.Errorf("got %+v, want the : hb comment", it)
		}
	}
}

func TestTheDefaultHeartbeatIsEveryTwentySeconds(t *testing.T) {
	if sse.HeartbeatInterval != 20*time.Second {
		t.Errorf("sse.HeartbeatInterval = %v, want 20s (docs/ARCHITECTURE.md, Timing)", sse.HeartbeatInterval)
	}
	if d := (&Server{}).heartbeatInterval(); d != sse.HeartbeatInterval {
		t.Errorf("a Server without HeartbeatInterval beats every %v, want sse.HeartbeatInterval", d)
	}
}

func TestAClosedStreamStopsListening(t *testing.T) {
	s := newTestServer(t)
	_, _, device := s.linkedDevice(t, "")
	es := s.openEvents(t, device)
	es.nextState(t)
	deviceID := s.mem.Devices()[0].ID
	if n := s.Broker.Subscribers(deviceID); n != 1 {
		t.Fatalf("%d subscribers while open, want 1", n)
	}
	es.res.Body.Close()
	deadline := time.Now().Add(2 * time.Second)
	for s.Broker.Subscribers(deviceID) != 0 {
		if time.Now().After(deadline) {
			t.Fatal("the stream is still subscribed 2 s after the client hung up")
		}
		time.Sleep(5 * time.Millisecond)
	}
}

func TestStopStreamsEndsOpenStreams(t *testing.T) {
	s := newTestServer(t)
	es := s.openEvents(t, s.newDevice(t))
	es.nextState(t)
	s.StopStreams()
	select {
	case _, ok := <-es.items:
		if ok {
			t.Error("got an item, want the stream to end")
		}
	case <-time.After(2 * time.Second):
		t.Error("the stream is still open 2 s after StopStreams")
	}
}

func TestWatchConnectionsChecksOnEveryTick(t *testing.T) {
	s := newTestServer(t)
	agentToken, _, device := s.linkedDevice(t, "")
	s.postSnapshot(t, agentToken, snapshotJSON(t0, parent100))
	es := s.openEvents(t, device)
	es.nextState(t)
	ticks := make(chan time.Time)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go s.WatchConnections(ctx, ticks)
	s.clock.Advance(live.DisconnectedAfter)
	ticks <- t0
	if m := es.nextState(t); !m.Servers[0].Disconnected {
		t.Errorf("after a tick past 60 s: %+v, want disconnected", m.Servers[0])
	}
}

func TestTheConnectionCheckRunsEverySecond(t *testing.T) {
	if ConnectionCheckInterval != time.Second {
		t.Errorf("ConnectionCheckInterval = %v, want 1s", ConnectionCheckInterval)
	}
}

func TestADisconnectIsToldAgainIfTheDevicesCouldNotBeFound(t *testing.T) {
	s := newTestServer(t)
	agentToken, _, device := s.linkedDevice(t, "")
	s.postSnapshot(t, agentToken, snapshotJSON(t0, parent100))
	es := s.openEvents(t, device)
	es.nextState(t)
	s.clock.Advance(live.DisconnectedAfter)
	s.mem.FailDevicesOfAgent = true
	s.CheckConnections(t.Context())
	es.quiet(t)
	s.mem.FailDevicesOfAgent = false
	s.CheckConnections(t.Context())
	if m := es.nextState(t); !m.Servers[0].Disconnected {
		t.Errorf("on the next check: %+v, want disconnected", m.Servers[0])
	}
}

func TestAStreamCannotOpenWhenTheDatabaseCannotBeReached(t *testing.T) {
	s := newTestServer(t)
	device := s.newDevice(t)
	s.mem.FailLinksOfDevice = true
	ts := httptest.NewServer(s.Handler())
	defer ts.Close()
	req, _ := http.NewRequest("GET", ts.URL+"/app/v1/events", nil)
	req.Header.Set("Authorization", "Bearer "+device)
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	res.Body.Close()
	if res.StatusCode != http.StatusServiceUnavailable {
		t.Errorf("status %d, want 503", res.StatusCode)
	}
}

func TestAnOpenStreamEndsWhenItsStateCannotBeBuilt(t *testing.T) {
	s := newTestServer(t)
	_, _, device := s.linkedDevice(t, "")
	es := s.openEvents(t, device)
	es.nextState(t)
	s.mem.FailLinksOfDevice = true
	s.NotifyDevice(s.mem.Devices()[0].ID)
	select {
	case _, ok := <-es.items:
		if ok {
			t.Error("got an item, want the stream to end so the phone reconnects")
		}
	case <-time.After(2 * time.Second):
		t.Error("the stream is still open")
	}
}

func TestTheStreamHasEventStreamHeaders(t *testing.T) {
	s := newTestServer(t)
	es := s.openEvents(t, s.newDevice(t))
	if es.res.StatusCode != http.StatusOK || es.res.Header.Get("Content-Type") != "text/event-stream" || es.res.Header.Get("Cache-Control") != "no-cache" {
		t.Errorf("status %d, headers %v; want 200, text/event-stream, no-cache", es.res.StatusCode, es.res.Header)
	}
}

func TestAnInvalidDeviceTokenCannotOpenTheStream(t *testing.T) {
	s := newTestServer(t)
	ts := httptest.NewServer(s.Handler())
	defer ts.Close()
	for _, auth := range []string{"", "Bearer bm90LWEtZGV2aWNl"} {
		req, _ := http.NewRequest("GET", ts.URL+"/app/v1/events", nil)
		if auth != "" {
			req.Header.Set("Authorization", auth)
		}
		res, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		var body struct{ Error string }
		_ = json.NewDecoder(res.Body).Decode(&body)
		res.Body.Close()
		if res.StatusCode != http.StatusUnauthorized || body.Error != "unauthorized" {
			t.Errorf("Authorization %q: %d %+v, want 401 unauthorized", auth, res.StatusCode, body)
		}
	}
}
