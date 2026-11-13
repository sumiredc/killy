package server

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/sumiredc/killy/internal/clock"
	"github.com/sumiredc/killy/internal/protocol"
	"github.com/sumiredc/killy/internal/server/live"
	"github.com/sumiredc/killy/internal/server/memstore"
	"github.com/sumiredc/killy/internal/server/pairing"
	"github.com/sumiredc/killy/internal/server/ratelimit"
	"github.com/sumiredc/killy/internal/server/sse"
)

var t0 = time.Date(2026, 11, 13, 9, 0, 0, 0, time.UTC)

type testServer struct {
	*Server
	mem   *memstore.Memory
	clock *clock.Fake
}

func newTestServer(t *testing.T) *testServer {
	t.Helper()
	c := clock.NewFake(t0)
	mem := memstore.New()
	return &testServer{
		Server: &Server{
			Store:               mem,
			Codes:               pairing.New(c),
			FirstRuns:           ratelimit.New(FirstRunLimit, FirstRunPeriod, c),
			DeviceRegistrations: ratelimit.New(DeviceRegistrationLimit, DeviceRegistrationPeriod, c),
			FailedClaims:        ratelimit.New(FailedClaimLimit, FailedClaimPeriod, c),
			Live:                live.New(c, mem),
			Broker:              sse.NewBroker(),
			// Long enough that no heartbeat comes between the events a test waits for.
			HeartbeatInterval: time.Hour,
			Log:               slog.New(slog.NewTextHandler(io.Discard, nil)),
		},
		mem:   mem,
		clock: c,
	}
}

// unreachableDB makes every store call fail, as when PostgreSQL cannot be reached.
func (s *testServer) unreachableDB() {
	s.mem.Err = errors.New("dial tcp 127.0.0.1:5432: connect: connection refused")
}

type pairingReply struct {
	AgentToken *string `json:"agent_token"`
	AgentID    string  `json:"agent_id"`
	Code       string  `json:"code"`
	Error      string  `json:"error"`
}

// pair sends POST /agent/v1/pairings from addr (the TCP peer) with an optional bearer token.
func (s *testServer) pair(t *testing.T, addr, token, body string, header ...string) (int, pairingReply) {
	t.Helper()
	r := httptest.NewRequest("POST", "/agent/v1/pairings", strings.NewReader(body))
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
	var resp pairingReply
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("status %d, body %q is not JSON: %v", w.Code, w.Body.String(), err)
	}
	if ct := w.Header().Get("Content-Type"); ct != "application/json" {
		t.Errorf("Content-Type = %q, want application/json", ct)
	}
	return w.Code, resp
}

const hostBody = `{"hostname":"web-1"}`

func (s *testServer) agentCount(t *testing.T) int {
	t.Helper()
	return len(s.mem.Agents())
}

func (s *testServer) register(t *testing.T) pairingReply {
	t.Helper()
	status, resp := s.pair(t, "192.0.2.1", "", hostBody)
	if status != http.StatusOK || resp.AgentToken == nil {
		t.Fatalf("first registration: %d %+v", status, resp)
	}
	return resp
}

var codeShape = regexp.MustCompile(`^[A-Z0-9]{6}$`)

func TestAFirstRegistrationReturnsATokenTheAgentIDAndACode(t *testing.T) {
	s := newTestServer(t)
	status, resp := s.pair(t, "192.0.2.1", "", hostBody)
	if status != http.StatusOK {
		t.Fatalf("status = %d (%s), want 200", status, resp.Error)
	}
	if resp.AgentToken == nil || len(*resp.AgentToken) < 43 {
		t.Errorf("agent_token = %v, want a token of at least 32 bytes in base64url", resp.AgentToken)
	}
	if !protocol.IsUUIDv4(resp.AgentID) {
		t.Errorf("agent_id = %q, want a UUID v4", resp.AgentID)
	}
	if !codeShape.MatchString(resp.Code) {
		t.Errorf("code = %q, want [A-Z0-9]{6}", resp.Code)
	}
	if got, err := s.Codes.Lookup(resp.Code); err != nil || got != resp.AgentID {
		t.Errorf("the code leads to %q, %v; want agent %s", got, err, resp.AgentID)
	}
}

func TestAFirstRegistrationStoresTheTokenHashNotTheToken(t *testing.T) {
	s := newTestServer(t)
	resp := s.register(t)
	rows := s.mem.Agents()
	if len(rows) != 1 || rows[0].ID != resp.AgentID {
		t.Fatalf("stored agents %+v, want the one registered", rows)
	}
	want := sha256.Sum256([]byte(*resp.AgentToken))
	if !bytes.Equal(rows[0].TokenSHA256, want[:]) {
		t.Errorf("stored token hash %x, want SHA-256 of the token %x", rows[0].TokenSHA256, want)
	}
	if rows[0].Hostname != "web-1" {
		t.Errorf("hostname = %q, want web-1", rows[0].Hostname)
	}
	if bytes.Contains(rows[0].TokenSHA256, []byte(*resp.AgentToken)) || strings.Contains(rows[0].Hostname, *resp.AgentToken) {
		t.Error("the token itself was handed to the store")
	}
}

func TestTheAgentTokenGetsANewCodeForTheSameAgent(t *testing.T) {
	s := newTestServer(t)
	first := s.register(t)
	status, resp := s.pair(t, "192.0.2.1", *first.AgentToken, hostBody)
	if status != http.StatusOK {
		t.Fatalf("status = %d (%s), want 200", status, resp.Error)
	}
	if resp.AgentID != first.AgentID {
		t.Errorf("agent_id = %s, want the same agent %s", resp.AgentID, first.AgentID)
	}
	if !codeShape.MatchString(resp.Code) || resp.Code == first.Code {
		t.Errorf("code = %q, want a new code (the first was %q)", resp.Code, first.Code)
	}
	if resp.AgentToken != nil {
		t.Error("a token was returned again; it is returned only on the first registration")
	}
	if n := s.agentCount(t); n != 1 {
		t.Errorf("%d agents, want 1", n)
	}
	if got, err := s.Codes.Lookup(resp.Code); err != nil || got != first.AgentID {
		t.Errorf("the new code leads to %q, %v; want agent %s", got, err, first.AgentID)
	}
}

func TestAnInvalidTokenIsRefusedAndCreatesNothing(t *testing.T) {
	s := newTestServer(t)
	for name, auth := range map[string]string{
		"unknown token": "Bearer bm90LWEtcmVhbC10b2tlbi1ub3QtYS1yZWFsLXRva2Vu",
		"empty bearer":  "Bearer ",
		"other scheme":  "Basic dXNlcjpwYXNz",
	} {
		t.Run(name, func(t *testing.T) {
			status, resp := s.pair(t, "192.0.2.1", "", hostBody, "Authorization", auth)
			if status != http.StatusUnauthorized || resp.Error != "unauthorized" {
				t.Errorf("got %d %+v, want 401 unauthorized", status, resp)
			}
			if resp.Code != "" || resp.AgentToken != nil {
				t.Errorf("response %+v carries a code or a token", resp)
			}
			if n := s.agentCount(t); n != 0 {
				t.Errorf("%d agents, want 0", n)
			}
		})
	}
}

func TestAnInvalidBodyIsAnInvalidRequestAndCreatesNothing(t *testing.T) {
	s := newTestServer(t)
	for name, body := range map[string]string{
		"not JSON":          "hostname=web-1",
		"no hostname":       `{}`,
		"empty hostname":    `{"hostname":"  "}`,
		"hostname too long": `{"hostname":"` + strings.Repeat("a", 256) + `"}`,
	} {
		t.Run(name, func(t *testing.T) {
			status, resp := s.pair(t, "192.0.2.1", "", body)
			if status != http.StatusBadRequest || resp.Error != "invalid_request" {
				t.Errorf("got %d %+v, want 400 invalid_request", status, resp)
			}
			if n := s.agentCount(t); n != 0 {
				t.Errorf("%d agents, want 0", n)
			}
		})
	}
}

func TestFirstRegistrationsAreLimitedToTenAnHourPerClientAddress(t *testing.T) {
	s := newTestServer(t)
	cf := func(ip string) []string { return []string{"CF-Connecting-IP", ip} }
	for i := range FirstRunLimit {
		if status, resp := s.pair(t, "10.42.0.7", "", hostBody, cf("203.0.113.9")...); status != http.StatusOK {
			t.Fatalf("registration %d: %d %+v, want 200", i+1, status, resp)
		}
	}
	status, resp := s.pair(t, "10.42.0.7", "", hostBody, cf("203.0.113.9")...)
	if status != http.StatusTooManyRequests || resp.Error != "rate_limited" || resp.Code != "" {
		t.Errorf("registration 11: %d %+v, want 429 rate_limited without a code", status, resp)
	}
	if n := s.agentCount(t); n != FirstRunLimit {
		t.Errorf("%d agents, want %d (the refused registration must not create one)", n, FirstRunLimit)
	}

	if status, _ := s.pair(t, "10.42.0.7", "", hostBody, cf("198.51.100.4")...); status != http.StatusOK {
		t.Errorf("another client address: %d, want 200", status)
	}

	s.clock.Advance(FirstRunPeriod)
	if status, _ := s.pair(t, "10.42.0.7", "", hostBody, cf("203.0.113.9")...); status != http.StatusOK {
		t.Errorf("after an hour: %d, want 200", status)
	}
}

func TestRequestsWithTheAgentTokenAreNeitherCountedNorLimited(t *testing.T) {
	s := newTestServer(t)
	first := s.register(t) // counted: 1 of 10 for 192.0.2.1
	for i := range FirstRunLimit + 5 {
		if status, resp := s.pair(t, "192.0.2.1", *first.AgentToken, hostBody); status != http.StatusOK {
			t.Fatalf("request %d with the agent token: %d %+v, want 200", i+1, status, resp)
		}
	}
	for i := 1; i < FirstRunLimit; i++ {
		if status, resp := s.pair(t, "192.0.2.1", "", hostBody); status != http.StatusOK {
			t.Fatalf("first-run registration %d: %d %+v, want 200", i+1, status, resp)
		}
	}
}

func TestWithoutCFConnectingIPFirstRegistrationsAreCountedByTheTCPPeer(t *testing.T) {
	s := newTestServer(t)
	for range FirstRunLimit {
		if status, _ := s.pair(t, "192.0.2.1", "", hostBody); status != http.StatusOK {
			t.Fatalf("status %d, want 200", status)
		}
	}
	if status, _ := s.pair(t, "192.0.2.1", "", hostBody); status != http.StatusTooManyRequests {
		t.Errorf("11th from the same peer: %d, want 429", status)
	}
	if status, _ := s.pair(t, "192.0.2.2", "", hostBody); status != http.StatusOK {
		t.Errorf("another peer: %d, want 200", status)
	}
}

func TestAnUnreachableDatabaseMakesThePairingUnavailable(t *testing.T) {
	s := newTestServer(t)
	s.unreachableDB()
	for name, token := range map[string]string{"first run": "", "with a token": "c29tZS10b2tlbg"} {
		t.Run(name, func(t *testing.T) {
			status, resp := s.pair(t, "192.0.2.1", token, hostBody)
			if status != http.StatusServiceUnavailable || resp.Error != "unavailable" {
				t.Errorf("got %d %+v, want 503 unavailable", status, resp)
			}
		})
	}
}

func TestInvalidRequestsDoNotUseUpTheFirstRunLimit(t *testing.T) {
	s := newTestServer(t)
	for range FirstRunLimit {
		if status, _ := s.pair(t, "192.0.2.1", "", `{}`); status != http.StatusBadRequest {
			t.Fatalf("invalid body: %d, want 400", status)
		}
	}
	for i := range FirstRunLimit {
		if status, resp := s.pair(t, "192.0.2.1", "", hostBody); status != http.StatusOK {
			t.Fatalf("valid registration %d after 10 invalid ones: %d %+v, want 200", i+1, status, resp)
		}
	}
}

func TestRegistrationsThatFailOnTheDatabaseStillCount(t *testing.T) {
	s := newTestServer(t)
	s.unreachableDB()
	for range FirstRunLimit {
		if status, _ := s.pair(t, "192.0.2.1", "", hostBody); status != http.StatusServiceUnavailable {
			t.Fatalf("status %d, want 503", status)
		}
	}
	// Otherwise the limit would not apply while the database is down.
	if status, resp := s.pair(t, "192.0.2.1", "", hostBody); status != http.StatusTooManyRequests {
		t.Errorf("11th registration: %d %+v, want 429", status, resp)
	}
}

func TestABodyOver64KiBIsAnInvalidRequest(t *testing.T) {
	s := newTestServer(t)
	body := `{"hostname":"web-1","padding":"` + strings.Repeat("x", 64<<10) + `"}`
	if status, resp := s.pair(t, "192.0.2.1", "", body); status != http.StatusBadRequest || resp.Error != "invalid_request" {
		t.Errorf("got %d %+v, want 400 invalid_request", status, resp)
	}
	if n := s.agentCount(t); n != 0 {
		t.Errorf("%d agents, want 0", n)
	}
}

func TestTheBearerSchemeIsMatchedWithoutRegardToCase(t *testing.T) {
	s := newTestServer(t)
	first := s.register(t)
	status, resp := s.pair(t, "192.0.2.1", "", hostBody, "Authorization", "bearer "+*first.AgentToken)
	if status != http.StatusOK || resp.AgentID != first.AgentID {
		t.Errorf("got %d %+v, want 200 for agent %s", status, resp, first.AgentID)
	}
}

func TestTheAgentTokenLeadsToItsOwnAgentAmongSeveral(t *testing.T) {
	s := newTestServer(t)
	first, second := s.register(t), s.register(t)
	for _, a := range []pairingReply{first, second} {
		status, resp := s.pair(t, "192.0.2.1", *a.AgentToken, hostBody)
		if status != http.StatusOK || resp.AgentID != a.AgentID {
			t.Errorf("token of %s: %d %+v, want that agent", a.AgentID, status, resp)
		}
	}
}
