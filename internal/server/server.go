// Package server is the Killy server: it relays between agents and phones.
package server

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/sumiredc/killy/internal/server/live"
	"github.com/sumiredc/killy/internal/server/pairing"
	"github.com/sumiredc/killy/internal/server/ratelimit"
	"github.com/sumiredc/killy/internal/server/store"
)

// First-run agent registrations allowed per client address (docs/ARCHITECTURE.md, Rate limits).
const (
	FirstRunLimit  = 10
	FirstRunPeriod = time.Hour
)

const (
	maxBodyBytes    = 64 << 10
	maxHostnameLen  = 255
	tokenRandomSize = 32
)

type Server struct {
	Store *store.Store
	Codes *pairing.Codes
	// FirstRuns counts first-run registrations per client address.
	FirstRuns *ratelimit.Limiter
	// Live holds every agent's latest snapshot.
	Live *live.State
	Log  *slog.Logger
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /agent/v1/pairings", s.createPairing)
	mux.HandleFunc("POST /agent/v1/snapshots", s.postSnapshot)
	return mux
}

type pairingRequest struct {
	Hostname string `json:"hostname"`
}

type pairingResponse struct {
	AgentToken string `json:"agent_token,omitempty"`
	AgentID    string `json:"agent_id"`
	Code       string `json:"code"`
}

func (s *Server) createPairing(w http.ResponseWriter, r *http.Request) {
	var req pairingRequest
	if err := decodeJSON(w, r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request")
		return
	}
	hostname := strings.TrimSpace(req.Hostname)
	if hostname == "" || len(hostname) > maxHostnameLen {
		writeError(w, http.StatusBadRequest, "invalid_request")
		return
	}

	if _, sent := r.Header["Authorization"]; sent {
		s.issueCodeForKnownAgent(w, r)
		return
	}

	// The body is checked before counting, so only registrations that would be accepted use up
	// the limit; a refused one is not counted either.
	if !s.FirstRuns.Allow(ratelimit.ClientAddr(r)) {
		writeError(w, http.StatusTooManyRequests, "rate_limited")
		return
	}
	token, err := newToken()
	if err != nil {
		s.Log.Error("generate agent token", "err", err)
		writeError(w, http.StatusServiceUnavailable, "unavailable")
		return
	}
	agent, err := s.Store.CreateAgent(r.Context(), hashToken(token), hostname)
	if err != nil {
		s.Log.Error("register agent", "err", err)
		writeError(w, http.StatusServiceUnavailable, "unavailable")
		return
	}
	s.respondWithCode(w, agent.ID, token)
}

// issueCodeForKnownAgent gives a new code to the agent the bearer token belongs to. A request that
// sends an Authorization header never registers a new agent, even when the token is wrong.
func (s *Server) issueCodeForKnownAgent(w http.ResponseWriter, r *http.Request) {
	agent, ok := s.authAgent(w, r)
	if !ok {
		return
	}
	s.respondWithCode(w, agent.ID, "")
}

func (s *Server) respondWithCode(w http.ResponseWriter, agentID, token string) {
	code, err := s.Codes.Issue(agentID)
	if err != nil {
		s.Log.Error("issue pairing code", "err", err)
		writeError(w, http.StatusServiceUnavailable, "unavailable")
		return
	}
	writeJSON(w, http.StatusOK, pairingResponse{AgentToken: token, AgentID: agentID, Code: code})
}

// newToken returns 32 random bytes in unpadded base64url.
func newToken() (string, error) {
	b := make([]byte, tokenRandomSize)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

// hashToken is what the database keeps instead of a token, so a leaked database holds no usable token.
func hashToken(token string) []byte {
	h := sha256.Sum256([]byte(token))
	return h[:]
}

// bearerToken reads "Authorization: Bearer <token>". The scheme name is case-insensitive (RFC 9110).
func bearerToken(r *http.Request) (string, bool) {
	scheme, token, found := strings.Cut(r.Header.Get("Authorization"), " ")
	token = strings.TrimSpace(token)
	return token, found && strings.EqualFold(scheme, "Bearer") && token != ""
}

func decodeJSON(w http.ResponseWriter, r *http.Request, v any) error {
	return decodeJSONLimit(w, r, v, maxBodyBytes)
}

func decodeJSONLimit(w http.ResponseWriter, r *http.Request, v any, limit int64) error {
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, limit))
	if err := dec.Decode(v); err != nil {
		return err
	}
	// Anything after the JSON value, such as padding past the size limit, makes the body invalid.
	if _, err := dec.Token(); !errors.Is(err, io.EOF) {
		return errors.New("trailing data after the JSON body")
	}
	return nil
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

// writeError writes the error body every endpoint uses, {"error": "<code>"} (docs/openapi.yaml).
func writeError(w http.ResponseWriter, status int, code string) {
	writeJSON(w, status, map[string]string{"error": code})
}
