package server

import (
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"
)

const (
	webOrigin  = "https://killy.sumire-sakamoto.co.jp"
	evilOrigin = "https://evil.example"
)

func TestParseCORSOrigins(t *testing.T) {
	for _, tc := range []struct {
		name  string
		value string
		want  []string
	}{
		{"unset", "", nil},
		{"only spaces", "  ", nil},
		{"one origin", webOrigin, []string{webOrigin}},
		{"two origins", webOrigin + ",http://localhost:8081", []string{webOrigin, "http://localhost:8081"}},
		{"spaces around the items", " " + webOrigin + " , http://localhost:8081 ", []string{webOrigin, "http://localhost:8081"}},
		{"a trailing slash is dropped", webOrigin + "/", []string{webOrigin}},
		{"the scheme and host are lower-cased", "HTTPS://Killy.Sumire-Sakamoto.co.jp", []string{webOrigin}},
		{"the default port is dropped, as a browser does not send it", "https://killy.sumire-sakamoto.co.jp:443,http://localhost:80", []string{webOrigin, "http://localhost"}},
		{"another port is kept", "https://killy.sumire-sakamoto.co.jp:8443", []string{"https://killy.sumire-sakamoto.co.jp:8443"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := ParseCORSOrigins(tc.value)
			if err != nil {
				t.Fatalf("ParseCORSOrigins(%q): %v", tc.value, err)
			}
			if !slices.Equal(got, tc.want) {
				t.Errorf("ParseCORSOrigins(%q) = %q, want %q", tc.value, got, tc.want)
			}
		})
	}
}

func TestParseCORSOriginsRefusesWhatIsNotAnOrigin(t *testing.T) {
	for name, value := range map[string]string{
		"a wildcard":                  "*",
		"a wildcard among others":     webOrigin + ",*",
		"a wildcard in the host":      "https://*.sumire-sakamoto.co.jp",
		"a path":                      webOrigin + "/demo",
		"a query":                     webOrigin + "?a=1",
		"a fragment":                  webOrigin + "#a",
		"user information":            "https://user@killy.sumire-sakamoto.co.jp",
		"no scheme":                   "killy.sumire-sakamoto.co.jp",
		"a scheme that is not http":   "ftp://killy.sumire-sakamoto.co.jp",
		"no host":                     "https://",
		"an empty item in the middle": webOrigin + ",," + evilOrigin,
		"an empty item at the end":    webOrigin + ",",
		"null":                        "null",
		"an empty port":               "https://killy.sumire-sakamoto.co.jp:",
		"a port that is not a number": "https://killy.sumire-sakamoto.co.jp:https",
		"a port with a leading zero":  "https://killy.sumire-sakamoto.co.jp:0443",
		"port 0":                      "https://killy.sumire-sakamoto.co.jp:0",
		"a port above 65535":          "https://killy.sumire-sakamoto.co.jp:65536",
		"a host outside ASCII":        "https://キリー.example",
	} {
		t.Run(name, func(t *testing.T) {
			if got, err := ParseCORSOrigins(value); err == nil {
				t.Errorf("ParseCORSOrigins(%q) = %q, want an error", value, got)
			}
		})
	}
}

// corsServer is a test server that allows webOrigin and one more origin.
func corsServer(t *testing.T) *testServer {
	t.Helper()
	s := newTestServer(t)
	s.CORSOrigins = []string{webOrigin, "http://localhost:8081"}
	return s
}

func (s *testServer) serve(method, path, origin string, header ...string) *httptest.ResponseRecorder {
	r := httptest.NewRequest(method, path, strings.NewReader(`{}`))
	r.RemoteAddr = "198.51.100.1:40000"
	r.Header.Set("Content-Type", "application/json")
	if origin != "" {
		r.Header.Set("Origin", origin)
	}
	for i := 0; i+1 < len(header); i += 2 {
		r.Header.Set(header[i], header[i+1])
	}
	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, r)
	return w
}

func (s *testServer) preflight(path, origin string) *httptest.ResponseRecorder {
	return s.serve("OPTIONS", path, origin,
		"Access-Control-Request-Method", "POST",
		"Access-Control-Request-Headers", "authorization,content-type")
}

// corsHeaders are the response headers that say what a page of another origin may do.
func corsHeaders(w http.ResponseWriter) http.Header {
	out := http.Header{}
	for k, v := range w.Header() {
		if strings.HasPrefix(k, "Access-Control-") {
			out[k] = v
		}
	}
	return out
}

func varies(w http.ResponseWriter, field string) bool {
	for _, v := range w.Header().Values("Vary") {
		for _, f := range strings.Split(v, ",") {
			if strings.EqualFold(strings.TrimSpace(f), field) {
				return true
			}
		}
	}
	return false
}

func TestAnAllowedOriginGetsItsOwnNameBackOnASuccessfulResponse(t *testing.T) {
	s := corsServer(t)
	for _, origin := range []string{webOrigin, "http://localhost:8081"} {
		w := s.serve("POST", "/app/v1/devices", origin)
		if w.Code != http.StatusOK {
			t.Fatalf("%s: status %d, want 200", origin, w.Code)
		}
		if got := w.Header().Get("Access-Control-Allow-Origin"); got != origin {
			t.Errorf("%s: Access-Control-Allow-Origin %q, want the origin itself", origin, got)
		}
		if !varies(w, "Origin") {
			t.Errorf("%s: Vary %q, want Origin, so a cache keeps the answers of different origins apart", origin, w.Header().Values("Vary"))
		}
	}
}

func TestAnAllowedOriginCanReadAnErrorResponse(t *testing.T) {
	// Without the header the page cannot read the status, so it could not tell 401 from no answer
	// and would never register again with a token the server does not know.
	s := corsServer(t)
	w := s.serve("POST", "/app/v1/demo", webOrigin)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("status %d, want 401 for a request without a token", w.Code)
	}
	if got := w.Header().Get("Access-Control-Allow-Origin"); got != webOrigin {
		t.Errorf("Access-Control-Allow-Origin %q on a 401, want %q", got, webOrigin)
	}
}

func TestAllowingAnOriginDoesNotLetARequestWithoutATokenIn(t *testing.T) {
	s := corsServer(t)
	for _, tc := range []struct{ method, path string }{
		{"POST", "/app/v1/demo"},
		{"POST", "/app/v1/kills"},
		{"POST", "/app/v1/pairings/claim"},
		{"PATCH", "/app/v1/devices/me"},
		{"DELETE", "/app/v1/servers/x"},
		{"GET", "/app/v1/events"},
	} {
		if w := s.serve(tc.method, tc.path, webOrigin, "Authorization", "Bearer wrong"); w.Code != http.StatusUnauthorized {
			t.Errorf("%s %s with a wrong token: status %d, want 401", tc.method, tc.path, w.Code)
		}
	}
}

func TestTheEventStreamOfAnAllowedOriginCarriesTheHeaders(t *testing.T) {
	s := corsServer(t)
	device := s.newDevice(t)
	ts := httptest.NewServer(s.Handler())
	t.Cleanup(ts.Close)
	req, _ := http.NewRequest("GET", ts.URL+"/app/v1/events", nil)
	req.Header.Set("Authorization", "Bearer "+device)
	req.Header.Set("Origin", webOrigin)
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { res.Body.Close() })
	if res.StatusCode != http.StatusOK || !strings.HasPrefix(res.Header.Get("Content-Type"), "text/event-stream") {
		t.Fatalf("status %d, type %q, want an open event stream", res.StatusCode, res.Header.Get("Content-Type"))
	}
	if got := res.Header.Get("Access-Control-Allow-Origin"); got != webOrigin {
		t.Errorf("Access-Control-Allow-Origin %q on the stream, want %q", got, webOrigin)
	}
}

func TestAPreflightFromAnAllowedOriginIsAnsweredBeforeAnythingElse(t *testing.T) {
	s := corsServer(t)
	w := s.preflight("/app/v1/devices", webOrigin)
	if w.Code != http.StatusNoContent {
		t.Fatalf("status %d, want 204", w.Code)
	}
	for field, want := range map[string]string{
		"Access-Control-Allow-Origin":  webOrigin,
		"Access-Control-Allow-Methods": "GET, POST, PATCH, DELETE",
		"Access-Control-Allow-Headers": "Authorization, Content-Type",
	} {
		if got := w.Header().Get(field); got != want {
			t.Errorf("%s %q, want %q", field, got, want)
		}
	}
	if w.Header().Get("Access-Control-Max-Age") == "" {
		t.Error("no Access-Control-Max-Age, so the browser asks again before every request")
	}
	if w.Body.Len() != 0 {
		t.Errorf("body %q, want none", w.Body.String())
	}
}

func TestAPreflightIsAnsweredForEveryAppRoute(t *testing.T) {
	s := corsServer(t)
	for _, path := range []string{
		"/app/v1/devices", "/app/v1/devices/me", "/app/v1/pairings/claim", "/app/v1/demo",
		"/app/v1/events", "/app/v1/kills", "/app/v1/servers/abc", "/app/v1/servers/abc/check",
	} {
		if w := s.preflight(path, webOrigin); w.Code != http.StatusNoContent {
			t.Errorf("preflight %s: status %d, want 204", path, w.Code)
		}
	}
}

func TestAPreflightDoesNotUseUpALimitOrNeedAToken(t *testing.T) {
	s := corsServer(t)
	// The registration limit is 10 per hour per address: preflights come before every request.
	for range DeviceRegistrationLimit + 5 {
		if w := s.preflight("/app/v1/devices", webOrigin); w.Code != http.StatusNoContent {
			t.Fatalf("preflight: status %d, want 204", w.Code)
		}
	}
	if w := s.serve("POST", "/app/v1/devices", webOrigin); w.Code != http.StatusOK {
		t.Errorf("registration after the preflights: status %d, want 200", w.Code)
	}
	if n := len(s.mem.Links()); n != 0 {
		t.Errorf("%d links, want none", n)
	}
}

func TestAnOriginThatIsNotAllowedGetsNoPermission(t *testing.T) {
	s := corsServer(t)
	for _, origin := range []string{
		evilOrigin,
		webOrigin + ".evil.example",          // starts with an allowed origin
		"http://killy.sumire-sakamoto.co.jp", // another scheme
		"https://killy.sumire-sakamoto.co.jp:8443",
		"https://KILLY.sumire-sakamoto.co.jp",
		"https://sub.killy.sumire-sakamoto.co.jp",
		"null",
	} {
		w := s.serve("POST", "/app/v1/devices", origin)
		if got := corsHeaders(w); len(got) != 0 {
			t.Errorf("%s: headers %v on a request, want no Access-Control-*", origin, got)
		}
		p := s.preflight("/app/v1/devices", origin)
		if p.Code == http.StatusNoContent || len(corsHeaders(p)) != 0 {
			t.Errorf("%s: preflight status %d, headers %v, want it refused as before", origin, p.Code, corsHeaders(p))
		}
		if !varies(w, "Origin") {
			t.Errorf("%s: Vary %q, want Origin: the answer depends on it", origin, w.Header().Values("Vary"))
		}
	}
}

func TestARequestWithoutAnOriginGetsNoCORSHeaders(t *testing.T) {
	s := corsServer(t)
	w := s.serve("POST", "/app/v1/devices", "")
	if w.Code != http.StatusOK {
		t.Fatalf("status %d, want 200", w.Code)
	}
	if got := corsHeaders(w); len(got) != 0 {
		t.Errorf("headers %v, want no Access-Control-*", got)
	}
}

func TestNoOriginIsEverAnsweredWithAWildcardOrCredentials(t *testing.T) {
	s := corsServer(t)
	for _, origin := range []string{webOrigin, evilOrigin, "null"} {
		for _, w := range []*httptest.ResponseRecorder{
			s.serve("POST", "/app/v1/devices", origin),
			s.preflight("/app/v1/devices", origin),
		} {
			if got := w.Header().Get("Access-Control-Allow-Origin"); got == "*" {
				t.Errorf("%s: Access-Control-Allow-Origin is *", origin)
			}
			if got := w.Header().Get("Access-Control-Allow-Credentials"); got != "" {
				t.Errorf("%s: Access-Control-Allow-Credentials %q, want none", origin, got)
			}
		}
	}
}

func TestTheAgentAPIGetsNoCORSHeaders(t *testing.T) {
	s := corsServer(t)
	w := s.serve("POST", "/agent/v1/pairings", webOrigin)
	if got := corsHeaders(w); len(got) != 0 || varies(w, "Origin") {
		t.Errorf("headers %v, Vary %q on the agent API, want none", got, w.Header().Values("Vary"))
	}
	p := s.preflight("/agent/v1/snapshots", webOrigin)
	if p.Code == http.StatusNoContent || len(corsHeaders(p)) != 0 {
		t.Errorf("preflight of an agent route: status %d, headers %v, want it refused as before", p.Code, corsHeaders(p))
	}
}

func TestWithoutAConfiguredOriginNothingChanges(t *testing.T) {
	s := newTestServer(t)
	w := s.serve("POST", "/app/v1/devices", webOrigin)
	if w.Code != http.StatusOK {
		t.Fatalf("status %d, want 200", w.Code)
	}
	if got := corsHeaders(w); len(got) != 0 || varies(w, "Origin") {
		t.Errorf("headers %v, Vary %q, want none when KILLY_CORS_ORIGINS is not set", got, w.Header().Values("Vary"))
	}
	p := s.preflight("/app/v1/devices", webOrigin)
	if p.Code == http.StatusNoContent || len(corsHeaders(p)) != 0 {
		t.Errorf("preflight: status %d, headers %v, want it refused as before", p.Code, corsHeaders(p))
	}
}
