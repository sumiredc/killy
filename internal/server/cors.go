package server

import (
	"fmt"
	"net/http"
	"net/url"
	"strings"
)

// corsPath is what a page of another origin may call: the app's API. The agent's API is never
// called from a browser, so it gets no CORS header.
const corsPath = "/app/v1/"

// corsMaxAge is how long a browser may reuse a preflight answer, in seconds.
const corsMaxAge = "600"

// ParseCORSOrigins reads KILLY_CORS_ORIGINS: origins (scheme, host and an optional port) separated
// by commas. An unset or blank value means no origin. Anything that is not an origin is an error,
// so a mistake cannot turn into "every origin" or into a setting that never matches.
func ParseCORSOrigins(value string) ([]string, error) {
	if strings.TrimSpace(value) == "" {
		return nil, nil
	}
	var origins []string
	for _, item := range strings.Split(value, ",") {
		origin, err := parseCORSOrigin(item)
		if err != nil {
			return nil, err
		}
		origins = append(origins, origin)
	}
	return origins, nil
}

func parseCORSOrigin(item string) (string, error) {
	item = strings.TrimSpace(item)
	if item == "" {
		return "", fmt.Errorf("an empty origin")
	}
	if strings.Contains(item, "*") {
		return "", fmt.Errorf("origin %q: a wildcard is not allowed", item)
	}
	u, err := url.Parse(strings.TrimSuffix(item, "/"))
	if err != nil {
		return "", fmt.Errorf("origin %q: %w", item, err)
	}
	scheme := strings.ToLower(u.Scheme)
	if (scheme != "http" && scheme != "https") || u.Hostname() == "" ||
		u.User != nil || u.Path != "" || u.RawQuery != "" || u.Fragment != "" || u.Opaque != "" {
		return "", fmt.Errorf("origin %q: want a scheme (http or https), a host and an optional port only", item)
	}
	host := strings.ToLower(u.Host)
	// A browser leaves the default port out of the Origin it sends, so it is left out here too.
	if (scheme == "https" && u.Port() == "443") || (scheme == "http" && u.Port() == "80") {
		host = strings.ToLower(u.Hostname())
		if strings.Contains(host, ":") {
			host = "[" + host + "]"
		}
	}
	return scheme + "://" + host, nil
}

// withCORS lets pages of the allowed origins call the app's API. It answers a preflight itself,
// before authentication and the rate limits, and adds the headers to every other answer under
// /app/v1/, errors and the event stream included. Without an allowed origin it changes nothing.
//
// The headers go on the ResponseWriter itself, which is not wrapped, so the stream's flushes and
// write deadlines (http.ResponseController) work as they do without CORS.
func (s *Server) withCORS(next http.Handler) http.Handler {
	if len(s.CORSOrigins) == 0 {
		return next
	}
	allowed := make(map[string]bool, len(s.CORSOrigins))
	for _, o := range s.CORSOrigins {
		allowed[o] = true
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasPrefix(r.URL.Path, corsPath) {
			next.ServeHTTP(w, r)
			return
		}
		h := w.Header()
		// The answer depends on the origin, so a cache must not give it to another one.
		h.Add("Vary", "Origin")
		origin := r.Header.Get("Origin")
		if !allowed[origin] {
			next.ServeHTTP(w, r)
			return
		}
		h.Set("Access-Control-Allow-Origin", origin)
		if r.Method == http.MethodOptions && r.Header.Get("Access-Control-Request-Method") != "" {
			h.Set("Access-Control-Allow-Methods", "GET, POST, PATCH, DELETE")
			h.Set("Access-Control-Allow-Headers", "Authorization, Content-Type")
			h.Set("Access-Control-Max-Age", corsMaxAge)
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}
