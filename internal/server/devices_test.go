package server

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/sumiredc/killy/internal/server/pairing"
)

type deviceReply struct {
	DeviceToken string `json:"device_token"`
	Error       string `json:"error"`
}

type serverReply struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Hostname string `json:"hostname"`
	Notify   *bool  `json:"notify"`
	Error    string `json:"error"`
}

func (s *testServer) do(t *testing.T, path, addr, token, body string, header ...string) (int, []byte) {
	t.Helper()
	r := httptest.NewRequest("POST", path, strings.NewReader(body))
	r.RemoteAddr = addr + ":40000"
	r.Header.Set("Content-Type", "application/json")
	if token != "" {
		r.Header.Set("Authorization", "Bearer "+token)
	}
	for i := 0; i+1 < len(header); i += 2 {
		r.Header.Set(header[i], header[i+1])
	}
	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, r)
	return w.Code, w.Body.Bytes()
}

func (s *testServer) doGet(t *testing.T, path, token string) (int, string) {
	t.Helper()
	r := httptest.NewRequest("GET", path, nil)
	if token != "" {
		r.Header.Set("Authorization", "Bearer "+token)
	}
	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, r)
	return w.Code, w.Body.String()
}

func (s *testServer) registerDevice(t *testing.T, addr, body string, header ...string) (int, deviceReply) {
	t.Helper()
	status, b := s.do(t, "/app/v1/devices", addr, "", body, header...)
	var resp deviceReply
	if err := json.Unmarshal(b, &resp); err != nil {
		t.Fatalf("status %d, body %q is not JSON", status, b)
	}
	return status, resp
}

func (s *testServer) newDevice(t *testing.T) string {
	t.Helper()
	status, resp := s.registerDevice(t, "198.51.100.1", `{}`)
	if status != http.StatusOK || resp.DeviceToken == "" {
		t.Fatalf("device registration: %d %+v", status, resp)
	}
	return resp.DeviceToken
}

func (s *testServer) claim(t *testing.T, deviceToken, addr, body string, header ...string) (int, serverReply) {
	t.Helper()
	status, b := s.do(t, "/app/v1/pairings/claim", addr, deviceToken, body, header...)
	var resp serverReply
	if err := json.Unmarshal(b, &resp); err != nil {
		t.Fatalf("status %d, body %q is not JSON", status, b)
	}
	return status, resp
}

// code registers an agent with hostname web-1 and returns its ID and a fresh code.
func (s *testServer) code(t *testing.T) (agentID, code string) {
	t.Helper()
	a := s.register(t)
	return a.AgentID, a.Code
}

func claimBody(code string, name ...string) string {
	if len(name) > 0 {
		return `{"code":"` + code + `","name":"` + name[0] + `"}`
	}
	return `{"code":"` + code + `"}`
}

const phoneAddr = "203.0.113.50"

func TestADeviceCanRegisterWithoutAPushToken(t *testing.T) {
	s := newTestServer(t)
	status, resp := s.registerDevice(t, phoneAddr, `{}`)
	if status != http.StatusOK || len(resp.DeviceToken) < 43 {
		t.Errorf("got %d %+v, want 200 with a device token", status, resp)
	}
	if rows := s.mem.Devices(); len(rows) != 1 || rows[0].PushToken != "" {
		t.Errorf("devices %+v, want one without a push token", rows)
	}
}

func TestADeviceRegistrationKeepsThePushToken(t *testing.T) {
	s := newTestServer(t)
	s.registerDevice(t, phoneAddr, `{"push_token":"ExponentPushToken[abc]"}`)
	if rows := s.mem.Devices(); len(rows) != 1 || rows[0].PushToken != "ExponentPushToken[abc]" {
		t.Errorf("devices %+v, want the push token kept", rows)
	}
}

func TestOnlyTheSHA256OfTheDeviceTokenIsStored(t *testing.T) {
	s := newTestServer(t)
	token := s.newDevice(t)
	rows := s.mem.Devices()
	want := sha256.Sum256([]byte(token))
	if len(rows) != 1 || !bytes.Equal(rows[0].TokenSHA256, want[:]) {
		t.Errorf("devices %+v, want one with the SHA-256 of the token", rows)
	}
	if bytes.Contains(rows[0].TokenSHA256, []byte(token)) || strings.Contains(rows[0].PushToken, token) {
		t.Error("the token itself was handed to the store")
	}
}

func TestAnInvalidDeviceRegistrationIsAnInvalidRequest(t *testing.T) {
	s := newTestServer(t)
	for name, body := range map[string]string{
		"not JSON":            "push",
		"empty push token":    `{"push_token":""}`,
		"push token too long": `{"push_token":"` + strings.Repeat("x", 513) + `"}`,
		"push token not text": `{"push_token":3}`,
	} {
		t.Run(name, func(t *testing.T) {
			if status, resp := s.registerDevice(t, phoneAddr, body); status != http.StatusBadRequest || resp.Error != "invalid_request" {
				t.Errorf("got %d %+v, want 400 invalid_request", status, resp)
			}
		})
	}
	if n := len(s.mem.Devices()); n != 0 {
		t.Errorf("%d devices, want 0", n)
	}
}

func TestDeviceRegistrationsAreLimitedToTenAnHourPerClientAddress(t *testing.T) {
	s := newTestServer(t)
	cf := func(ip string) []string { return []string{"CF-Connecting-IP", ip} }
	for i := range DeviceRegistrationLimit {
		if status, _ := s.registerDevice(t, "10.42.0.7", `{}`, cf(phoneAddr)...); status != http.StatusOK {
			t.Fatalf("registration %d: %d, want 200", i+1, status)
		}
	}
	for range 3 {
		if status, resp := s.registerDevice(t, "10.42.0.7", `{}`, cf(phoneAddr)...); status != http.StatusTooManyRequests || resp.Error != "rate_limited" || resp.DeviceToken != "" {
			t.Errorf("over the limit: %d %+v, want 429 rate_limited without a token", status, resp)
		}
	}
	if n := len(s.mem.Devices()); n != DeviceRegistrationLimit {
		t.Errorf("%d devices, want %d", n, DeviceRegistrationLimit)
	}
	if status, _ := s.registerDevice(t, "10.42.0.7", `{}`, cf("198.51.100.9")...); status != http.StatusOK {
		t.Errorf("another client address: %d, want 200", status)
	}
	s.clock.Advance(DeviceRegistrationPeriod)
	for i := range DeviceRegistrationLimit {
		if status, _ := s.registerDevice(t, "10.42.0.7", `{}`, cf(phoneAddr)...); status != http.StatusOK {
			t.Fatalf("after an hour, registration %d: %d, want 200 (refused ones must not count)", i+1, status)
		}
	}
}

func TestDeviceRegistrationsWithoutCFConnectingIPAreCountedByTheTCPPeer(t *testing.T) {
	s := newTestServer(t)
	for range DeviceRegistrationLimit {
		s.registerDevice(t, "192.0.2.7", `{}`)
	}
	if status, _ := s.registerDevice(t, "192.0.2.7", `{}`); status != http.StatusTooManyRequests {
		t.Errorf("11th from the same peer: %d, want 429", status)
	}
	if status, _ := s.registerDevice(t, "192.0.2.8", `{}`); status != http.StatusOK {
		t.Errorf("another peer: %d, want 200", status)
	}
}

func TestAValidCodeLinksTheDeviceWithoutWaitingForTheAgent(t *testing.T) {
	s := newTestServer(t)
	agentID, code := s.code(t)
	device := s.newDevice(t)
	// The agent has never sent a snapshot.
	status, resp := s.claim(t, device, phoneAddr, claimBody(code, "Web server"))
	if status != http.StatusOK {
		t.Fatalf("status %d (%s), want 200", status, resp.Error)
	}
	if resp.ID != agentID || resp.Name != "Web server" || resp.Hostname != "web-1" || resp.Notify == nil || !*resp.Notify {
		t.Errorf("server %+v, want id %s (the agent ID), name Web server, hostname web-1, notify on", resp, agentID)
	}
	if links := s.mem.Links(); len(links) != 1 || links[0].AgentID != agentID {
		t.Errorf("links %+v, want one to the agent", links)
	}
}

func TestWithoutANameTheServerIsNamedAfterItsHostname(t *testing.T) {
	s := newTestServer(t)
	_, code := s.code(t)
	for _, body := range []string{claimBody(code)} {
		status, resp := s.claim(t, s.newDevice(t), phoneAddr, body)
		if status != http.StatusOK || resp.Name != "web-1" {
			t.Errorf("got %d %+v, want 200 named web-1", status, resp)
		}
	}
}

func TestABlankNameCountsAsNoName(t *testing.T) {
	s := newTestServer(t)
	_, code := s.code(t)
	status, resp := s.claim(t, s.newDevice(t), phoneAddr, claimBody(code, "   "))
	if status != http.StatusOK || resp.Name != "web-1" {
		t.Errorf("got %d %+v, want 200 named web-1", status, resp)
	}
}

func TestAFailedClaimCreatesNoLinkAndSaysWhy(t *testing.T) {
	s := newTestServer(t)
	device := s.newDevice(t)
	_, used := s.code(t)
	if status, _ := s.claim(t, device, phoneAddr, claimBody(used)); status != http.StatusOK {
		t.Fatal("first claim failed")
	}
	_, expired := s.code(t)
	s.clock.Advance(pairing.Lifetime)
	before := len(s.mem.Links())
	for name, tc := range map[string]struct {
		code   string
		status int
		error  string
	}{
		"unknown": {"ZZZZZZ", http.StatusNotFound, "code_unknown"},
		"used":    {used, http.StatusConflict, "code_used"},
		"expired": {expired, http.StatusGone, "code_expired"},
	} {
		t.Run(name, func(t *testing.T) {
			status, resp := s.claim(t, s.newDevice(t), phoneAddr, claimBody(tc.code))
			if status != tc.status || resp.Error != tc.error {
				t.Errorf("got %d %+v, want %d %s", status, resp, tc.status, tc.error)
			}
		})
	}
	if n := len(s.mem.Links()); n != before {
		t.Errorf("%d links, want %d (no new link)", n, before)
	}
}

func TestTheSameCodeClaimedTwiceAtOnceMakesOneLink(t *testing.T) {
	s := newTestServer(t)
	_, code := s.code(t)
	devices := []string{s.newDevice(t), s.newDevice(t)}
	statuses := make([]int, 2)
	var wg sync.WaitGroup
	for i, d := range devices {
		wg.Add(1)
		go func() {
			defer wg.Done()
			statuses[i], _ = s.do(t, "/app/v1/pairings/claim", phoneAddr, d, claimBody(code))
		}()
	}
	wg.Wait()
	ok, used := 0, 0
	for _, st := range statuses {
		switch st {
		case http.StatusOK:
			ok++
		case http.StatusConflict:
			used++
		}
	}
	if ok != 1 || used != 1 {
		t.Errorf("statuses %v, want one 200 and one 409", statuses)
	}
	if n := len(s.mem.Links()); n != 1 {
		t.Errorf("%d links, want 1", n)
	}
}

func TestTwoDevicesLinkTheSameAgentUnderTheirOwnNames(t *testing.T) {
	s := newTestServer(t)
	first := s.register(t)
	phoneA, phoneB := s.newDevice(t), s.newDevice(t)
	s.claim(t, phoneA, phoneAddr, claimBody(first.Code, "Prod web"))
	status, again := s.pair(t, "192.0.2.1", *first.AgentToken, hostBody)
	if status != http.StatusOK {
		t.Fatal("second code not issued")
	}
	statusB, resp := s.claim(t, phoneB, phoneAddr, claimBody(again.Code, "My box"))
	if statusB != http.StatusOK || resp.ID != first.AgentID || resp.Name != "My box" {
		t.Errorf("second phone: %d %+v, want the same agent named My box", statusB, resp)
	}
	names := map[string]bool{}
	for _, l := range s.mem.Links() {
		if l.AgentID == first.AgentID {
			names[l.Name] = true
		}
	}
	if len(names) != 2 || !names["Prod web"] || !names["My box"] {
		t.Errorf("link names %v, want Prod web and My box", names)
	}
}

func TestAClaimWithAnInvalidDeviceTokenIsRefusedAndLinksNothing(t *testing.T) {
	s := newTestServer(t)
	_, code := s.code(t)
	for name, token := range map[string]string{"no token": "", "unknown token": "bm90LWEtZGV2aWNlLXRva2Vu"} {
		t.Run(name, func(t *testing.T) {
			if status, resp := s.claim(t, token, phoneAddr, claimBody(code)); status != http.StatusUnauthorized || resp.Error != "unauthorized" {
				t.Errorf("got %d %+v, want 401 unauthorized", status, resp)
			}
		})
	}
	if n := len(s.mem.Links()); n != 0 {
		t.Errorf("%d links, want 0", n)
	}
	// The code was not used up by the refused claims.
	if status, _ := s.claim(t, s.newDevice(t), phoneAddr, claimBody(code)); status != http.StatusOK {
		t.Errorf("claim with a valid device: %d, want 200", status)
	}
}

func TestAnInvalidClaimIsAnInvalidRequest(t *testing.T) {
	s := newTestServer(t)
	device := s.newDevice(t)
	for name, body := range map[string]string{
		"not JSON":      "code",
		"no code":       `{}`,
		"lower case":    `{"code":"abc123"}`,
		"too short":     `{"code":"ABC12"}`,
		"name too long": claimBody("ABC123", strings.Repeat("n", 101)),
	} {
		t.Run(name, func(t *testing.T) {
			if status, resp := s.claim(t, device, phoneAddr, body); status != http.StatusBadRequest || resp.Error != "invalid_request" {
				t.Errorf("got %d %+v, want 400 invalid_request", status, resp)
			}
		})
	}
}

func TestFailedClaimsBlockEveryClaimFromTheAddressForTenMinutes(t *testing.T) {
	s := newTestServer(t)
	device := s.newDevice(t)
	cf := func(ip string) []string { return []string{"CF-Connecting-IP", ip} }
	for i := range FailedClaimLimit {
		if status, resp := s.claim(t, device, "10.42.0.7", claimBody("ZZZZZZ"), cf(phoneAddr)...); status != http.StatusNotFound {
			t.Fatalf("failed claim %d: %d %+v, want 404", i+1, status, resp)
		}
	}
	_, valid := s.code(t)
	for range 3 {
		if status, resp := s.claim(t, device, "10.42.0.7", claimBody(valid), cf(phoneAddr)...); status != http.StatusTooManyRequests || resp.Error != "rate_limited" {
			t.Errorf("valid code while blocked: %d %+v, want 429 rate_limited", status, resp)
		}
	}
	if n := len(s.mem.Links()); n != 0 {
		t.Errorf("%d links, want 0 while blocked", n)
	}
	if status, _ := s.claim(t, device, "10.42.0.7", claimBody(valid), cf("198.51.100.9")...); status != http.StatusOK {
		t.Errorf("another client address: %d, want 200", status)
	}

	s.clock.Advance(FailedClaimPeriod - time.Nanosecond)
	_, late := s.code(t)
	if status, _ := s.claim(t, device, "10.42.0.7", claimBody(late), cf(phoneAddr)...); status != http.StatusTooManyRequests {
		t.Errorf("just before 10 min: %d, want 429", status)
	}
	s.clock.Advance(time.Nanosecond)
	_, after := s.code(t)
	if status, resp := s.claim(t, device, "10.42.0.7", claimBody(after), cf(phoneAddr)...); status != http.StatusOK {
		t.Errorf("at 10 min with a code issued after the window: %d %+v, want 200 (refused claims must not count)", status, resp)
	}
}

func TestFailedClaimsWithoutCFConnectingIPAreCountedByTheTCPPeer(t *testing.T) {
	s := newTestServer(t)
	device := s.newDevice(t)
	for range FailedClaimLimit {
		s.claim(t, device, "192.0.2.7", claimBody("ZZZZZZ"))
	}
	if status, _ := s.claim(t, device, "192.0.2.7", claimBody("ZZZZZZ")); status != http.StatusTooManyRequests {
		t.Errorf("11th from the same peer: %d, want 429", status)
	}
	if status, _ := s.claim(t, device, "192.0.2.8", claimBody("ZZZZZZ")); status != http.StatusNotFound {
		t.Errorf("another peer: %d, want 404", status)
	}
}

func TestSuccessfulClaimsAreNotCountedAsFailures(t *testing.T) {
	s := newTestServer(t)
	device := s.newDevice(t)
	agent := s.register(t)
	for i := range FailedClaimLimit + 2 {
		_, again := s.pair(t, "192.0.2.1", *agent.AgentToken, hostBody)
		if status, resp := s.claim(t, device, phoneAddr, claimBody(again.Code)); status != http.StatusOK {
			t.Fatalf("claim %d: %d %+v, want 200", i+1, status, resp)
		}
	}
}

func TestDeviceAndClaimAreUnavailableWhenTheDatabaseCannotBeReached(t *testing.T) {
	s := newTestServer(t)
	device := s.newDevice(t)
	_, code := s.code(t)
	s.unreachableDB()
	if status, resp := s.registerDevice(t, phoneAddr, `{}`); status != http.StatusServiceUnavailable || resp.Error != "unavailable" {
		t.Errorf("registration: %d %+v, want 503 unavailable", status, resp)
	}
	if status, resp := s.claim(t, device, phoneAddr, claimBody(code)); status != http.StatusServiceUnavailable || resp.Error != "unavailable" {
		t.Errorf("claim: %d %+v, want 503 unavailable", status, resp)
	}
}

func TestAClaimThatFailsOnTheDatabaseLeavesTheCodeUsable(t *testing.T) {
	s := newTestServer(t)
	device := s.newDevice(t)
	_, code := s.code(t)
	s.mem.FailLink = true
	if status, _ := s.claim(t, device, phoneAddr, claimBody(code)); status != http.StatusServiceUnavailable {
		t.Fatalf("status %d, want 503", status)
	}
	s.mem.FailLink = false
	if status, resp := s.claim(t, device, phoneAddr, claimBody(code)); status != http.StatusOK {
		t.Errorf("retry: %d %+v, want 200 (the code was not used up)", status, resp)
	}
}

func TestClaimingAnAlreadyLinkedServerAgainKeepsItsNameUnlessOneIsGiven(t *testing.T) {
	s := newTestServer(t)
	agent := s.register(t)
	device := s.newDevice(t)
	s.claim(t, device, phoneAddr, claimBody(agent.Code, "Prod web"))
	_, again := s.pair(t, "192.0.2.1", *agent.AgentToken, hostBody)
	if status, resp := s.claim(t, device, phoneAddr, claimBody(again.Code)); status != http.StatusOK || resp.Name != "Prod web" {
		t.Errorf("claim again without a name: %d %+v, want 200 keeping Prod web", status, resp)
	}
	_, third := s.pair(t, "192.0.2.1", *agent.AgentToken, hostBody)
	if status, resp := s.claim(t, device, phoneAddr, claimBody(third.Code, "Renamed")); status != http.StatusOK || resp.Name != "Renamed" {
		t.Errorf("claim again with a name: %d %+v, want 200 named Renamed", status, resp)
	}
	if n := len(s.mem.Links()); n != 1 {
		t.Errorf("%d links, want 1", n)
	}
}

func TestInvalidClaimsDoNotCountAsFailures(t *testing.T) {
	s := newTestServer(t)
	device := s.newDevice(t)
	for range FailedClaimLimit + 2 {
		s.claim(t, device, phoneAddr, `{"code":"bad"}`)
	}
	_, code := s.code(t)
	if status, resp := s.claim(t, device, phoneAddr, claimBody(code)); status != http.StatusOK {
		t.Errorf("valid claim after invalid ones: %d %+v, want 200", status, resp)
	}
}

func TestUnauthorizedClaimsDoNotCountAsFailures(t *testing.T) {
	s := newTestServer(t)
	for range FailedClaimLimit + 2 {
		s.claim(t, "bm90LWEtZGV2aWNlLXRva2Vu", phoneAddr, claimBody("ZZZZZZ"))
	}
	_, code := s.code(t)
	if status, resp := s.claim(t, s.newDevice(t), phoneAddr, claimBody(code)); status != http.StatusOK {
		t.Errorf("valid claim after unauthorized ones: %d %+v, want 200", status, resp)
	}
}

// heldClaim is a claim whose body the test sends later, so the request is in flight meanwhile.
type heldClaim struct {
	body *io.PipeWriter
	done chan int
}

func (s *testServer) holdClaim(device string) heldClaim {
	pr, pw := io.Pipe()
	h := heldClaim{body: pw, done: make(chan int, 1)}
	r := httptest.NewRequest("POST", "/app/v1/pairings/claim", pr)
	r.RemoteAddr = phoneAddr + ":40000"
	r.Header.Set("Authorization", "Bearer "+device)
	go func() {
		w := httptest.NewRecorder()
		s.Handler().ServeHTTP(w, r)
		h.done <- w.Code
	}()
	return h
}

func TestClaimsInFlightCountTowardsTheFailureLimit(t *testing.T) {
	s := newTestServer(t)
	device := s.newDevice(t)
	held := make([]heldClaim, FailedClaimLimit)
	for i := range held {
		held[i] = s.holdClaim(device)
	}
	// Wait until all ten are past the limit check and reading their bodies.
	deadline := time.Now().Add(2 * time.Second)
	for !s.FailedClaims.Full(phoneAddr) {
		if time.Now().After(deadline) {
			t.Fatal("the held claims never reached the limit check")
		}
		time.Sleep(time.Millisecond)
	}
	_, code := s.code(t)
	if status, resp := s.claim(t, device, phoneAddr, claimBody(code)); status != http.StatusTooManyRequests {
		t.Errorf("valid claim while ten guesses are in flight: %d %+v, want 429", status, resp)
	}
	for _, h := range held {
		_, _ = io.WriteString(h.body, claimBody("ZZZZZZ"))
		_ = h.body.Close()
		if status := <-h.done; status != http.StatusNotFound {
			t.Errorf("held guess: %d, want 404", status)
		}
	}
	if n := len(s.mem.Links()); n != 0 {
		t.Errorf("%d links, want 0", n)
	}
}
