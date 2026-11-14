package server

import (
	"context"
	"errors"
	"net/http"
	"regexp"
	"strings"
	"unicode/utf8"

	"github.com/sumiredc/killy/internal/server/pairing"
	"github.com/sumiredc/killy/internal/server/ratelimit"
	"github.com/sumiredc/killy/internal/server/store"
)

const (
	maxPushTokenLen = 512
	maxNameRunes    = 100
)

var codePattern = regexp.MustCompile(`^[A-Z0-9]{6}$`)

type deviceRequest struct {
	PushToken *string `json:"push_token"`
}

type deviceResponse struct {
	DeviceToken string `json:"device_token"`
}

func (s *Server) registerDevice(w http.ResponseWriter, r *http.Request) {
	var req deviceRequest
	if err := decodeJSON(w, r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request")
		return
	}
	pushToken := ""
	if req.PushToken != nil {
		pushToken = *req.PushToken
		if pushToken == "" || len(pushToken) > maxPushTokenLen {
			writeError(w, http.StatusBadRequest, "invalid_request")
			return
		}
	}
	// Without this limit a client could register new devices to get around the per-device kill limit.
	if !s.DeviceRegistrations.Allow(ratelimit.ClientAddr(r)) {
		writeError(w, http.StatusTooManyRequests, "rate_limited")
		return
	}
	token, err := newToken()
	if err != nil {
		s.Log.Error("generate device token", "err", err)
		writeError(w, http.StatusServiceUnavailable, "unavailable")
		return
	}
	if _, err := s.Store.CreateDevice(r.Context(), hashToken(token), pushToken); err != nil {
		s.Log.Error("register device", "err", err)
		writeError(w, http.StatusServiceUnavailable, "unavailable")
		return
	}
	writeJSON(w, http.StatusOK, deviceResponse{DeviceToken: token})
}

type claimRequest struct {
	Code string  `json:"code"`
	Name *string `json:"name"`
}

// serverResponse is a server as one phone sees it (Server in docs/openapi.yaml).
type serverResponse struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Hostname string `json:"hostname"`
	Notify   bool   `json:"notify"`
}

func (s *Server) claimPairing(w http.ResponseWriter, r *http.Request) {
	device, ok := s.authDevice(w, r)
	if !ok {
		return
	}
	// Every claim is counted as a failure up front and refunded unless it fails. While failures
	// fill the window every claim is refused, a valid code included, so a guess that hits during a
	// brute-force run does not get through; claims still in flight count too.
	refund, ok := s.FailedClaims.Take(ratelimit.ClientAddr(r))
	if !ok {
		writeError(w, http.StatusTooManyRequests, "rate_limited")
		return
	}
	var req claimRequest
	if err := decodeJSON(w, r, &req); err != nil || !codePattern.MatchString(req.Code) {
		refund()
		writeError(w, http.StatusBadRequest, "invalid_request")
		return
	}
	name, ok := displayName(req.Name)
	if !ok {
		refund()
		writeError(w, http.StatusBadRequest, "invalid_request")
		return
	}

	agentID, release, err := s.Codes.Claim(req.Code)
	if err != nil {
		switch {
		case errors.Is(err, pairing.ErrUnknown):
			writeError(w, http.StatusNotFound, "code_unknown")
		case errors.Is(err, pairing.ErrUsed):
			writeError(w, http.StatusConflict, "code_used")
		default:
			writeError(w, http.StatusGone, "code_expired")
		}
		return
	}
	refund()
	// Not cancelled when the phone hangs up: a link the database committed must not leave its code
	// usable again.
	ctx := context.WithoutCancel(r.Context())
	// The claim does not wait for the agent: it only needs the agent's registration.
	agent, err := s.Store.AgentByID(ctx, agentID)
	if err == nil {
		var link store.Link
		link, err = s.Store.LinkDevice(ctx, device.ID, agentID, name, agent.Hostname)
		if err == nil {
			s.NotifyDevice(device.ID)
			writeJSON(w, http.StatusOK, serverResponse{ID: agentID, Name: link.Name, Hostname: agent.Hostname, Notify: link.Notify})
			return
		}
	}
	// The code was not used: the phone may try it again.
	release()
	s.Log.Error("link device to agent", "agent", agentID, "err", err)
	writeError(w, http.StatusServiceUnavailable, "unavailable")
}

// displayName applies the rules a display name follows wherever it comes in (claim and PATCH): it
// is refused (false) over maxNameRunes, counted before trimming, or with U+0000, which PostgreSQL
// text cannot hold; it is saved trimmed; and a name that is missing or empty after trimming is nil,
// which keeps the previous name.
func displayName(raw *string) (*string, bool) {
	if raw == nil {
		return nil, true
	}
	if utf8.RuneCountInString(*raw) > maxNameRunes || strings.ContainsRune(*raw, 0) {
		return nil, false
	}
	n := strings.TrimSpace(*raw)
	if n == "" {
		return nil, true
	}
	return &n, true
}

// authDevice finds the device of the request's bearer token. When it returns false it has already
// written the error response.
func (s *Server) authDevice(w http.ResponseWriter, r *http.Request) (store.Device, bool) {
	token, ok := bearerToken(r)
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return store.Device{}, false
	}
	device, err := s.Store.DeviceByToken(r.Context(), hashToken(token))
	if errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return store.Device{}, false
	}
	if err != nil {
		s.Log.Error("find device by token", "err", err)
		writeError(w, http.StatusServiceUnavailable, "unavailable")
		return store.Device{}, false
	}
	return device, true
}
