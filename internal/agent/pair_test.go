package agent

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"testing"
)

const (
	issuedToken = "c2VjcmV0LXRva2VuLWZvci10ZXN0cy1vbmx5LTEyMzQ1Njc4"
	issuedAgent = "0f8fad5b-d9cb-469f-a165-70867728950e"
)

// fakeKilly answers POST /agent/v1/pairings like the Killy server and records each request.
type fakeKilly struct {
	*httptest.Server
	status int

	mu       sync.Mutex
	requests []recorded
}

type recorded struct {
	auth     string
	hostname string
}

func newFakeKilly(t *testing.T) *fakeKilly {
	f := &fakeKilly{status: http.StatusOK}
	f.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body struct{ Hostname string }
		_ = json.NewDecoder(r.Body).Decode(&body)
		f.mu.Lock()
		f.requests = append(f.requests, recorded{auth: r.Header.Get("Authorization"), hostname: body.Hostname})
		n := len(f.requests)
		f.mu.Unlock()
		if r.Method != "POST" || r.URL.Path != "/agent/v1/pairings" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(f.status)
		switch {
		case f.status == http.StatusTooManyRequests:
			_, _ = w.Write([]byte(`{"error":"rate_limited"}`))
		case f.status == http.StatusUnauthorized:
			_, _ = w.Write([]byte(`{"error":"unauthorized"}`))
		case r.Header.Get("Authorization") == "":
			_, _ = w.Write([]byte(`{"agent_token":"` + issuedToken + `","agent_id":"` + issuedAgent + `","code":"K7Q2ZP"}`))
		default:
			code := []string{"", "A1B2C3", "D4E5F6", "G7H8I9"}[min(n, 3)]
			_, _ = w.Write([]byte(`{"agent_id":"` + issuedAgent + `","code":"` + code + `"}`))
		}
	}))
	t.Cleanup(f.Close)
	return f
}

func (f *fakeKilly) received() []recorded {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]recorded(nil), f.requests...)
}

var codeLine = regexp.MustCompile(`(?m)^Pairing code: [A-Z0-9]{6}$`)

func configPath(t *testing.T) string {
	return filepath.Join(t.TempDir(), "etc-killy", "agent.json")
}

func pair(t *testing.T, path, server string) (string, error) {
	t.Helper()
	var out bytes.Buffer
	err := Pair(context.Background(), PairOptions{ConfigPath: path, Server: server, Hostname: "web-1", Client: http.DefaultClient}, &out)
	return out.String(), err
}

func readConfig(t *testing.T, path string) Config {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read agent.json: %v", err)
	}
	var c Config
	if err := json.Unmarshal(b, &c); err != nil {
		t.Fatalf("agent.json is not JSON: %v", err)
	}
	return c
}

func writeConfig(t *testing.T, path string, c Config) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	b, _ := json.Marshal(c)
	if err := os.WriteFile(path, b, 0o600); err != nil {
		t.Fatal(err)
	}
}

func assertShowsCodeAndAgentButNotToken(t *testing.T, out, code string) {
	t.Helper()
	if !strings.Contains(out, code) || !strings.Contains(out, issuedAgent) {
		t.Errorf("output %q, want the code %s and the agent ID %s", out, code, issuedAgent)
	}
	if strings.Contains(out, issuedToken) {
		t.Errorf("output %q shows the agent token", out)
	}
}

func TestTheFirstPairRegistersWithoutATokenAndSavesTokenAndServer(t *testing.T) {
	killy := newFakeKilly(t)
	path := configPath(t)
	out, err := pair(t, path, killy.URL)
	if err != nil {
		t.Fatalf("Pair: %v", err)
	}
	if got := killy.received(); len(got) != 1 || got[0].auth != "" || got[0].hostname != "web-1" {
		t.Errorf("requests %+v, want one without Authorization carrying hostname web-1", got)
	}
	if c := readConfig(t, path); c != (Config{Server: killy.URL, Token: issuedToken}) {
		t.Errorf("agent.json = %+v, want the server URL and the issued token", c)
	}
	assertShowsCodeAndAgentButNotToken(t, out, "K7Q2ZP")
}

func TestTheFirstPairCreatesAgentJSONWithMode0600(t *testing.T) {
	killy := newFakeKilly(t)
	path := configPath(t)
	if _, err := pair(t, path, killy.URL); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if perm := info.Mode().Perm(); perm != 0o600 {
		t.Errorf("agent.json mode = %o, want 600", perm)
	}
}

func TestALaterPairSendsTheSavedTokenToTheSavedServer(t *testing.T) {
	killy := newFakeKilly(t)
	path := configPath(t)
	if _, err := pair(t, path, killy.URL); err != nil {
		t.Fatal(err)
	}
	for _, flag := range []string{"", killy.URL, killy.URL + "/"} {
		out, err := pair(t, path, flag)
		if err != nil {
			t.Fatalf("Pair with --server %q: %v", flag, err)
		}
		got := killy.received()
		if len(got) == 0 {
			t.Fatalf("--server %q: no request sent", flag)
		}
		if last := got[len(got)-1]; last.auth != "Bearer "+issuedToken {
			t.Errorf("--server %q: Authorization = %q, want the saved token", flag, last.auth)
		}
		if !codeLine.MatchString(out) || !strings.Contains(out, issuedAgent) || strings.Contains(out, issuedToken) {
			t.Errorf("--server %q: output %q, want a pairing code, the agent ID and no token", flag, out)
		}
	}
	if c := readConfig(t, path); c != (Config{Server: killy.URL, Token: issuedToken}) {
		t.Errorf("agent.json changed to %+v", c)
	}
}

func TestALaterPairWithAnotherServerSendsNothing(t *testing.T) {
	saved, other := newFakeKilly(t), newFakeKilly(t)
	path := configPath(t)
	writeConfig(t, path, Config{Server: saved.URL, Token: issuedToken})
	_, err := pair(t, path, other.URL)
	if err == nil || !strings.Contains(err.Error(), saved.URL) {
		t.Errorf("err = %v, want a refusal naming the saved server %s", err, saved.URL)
	}
	if n := len(saved.received()) + len(other.received()); n != 0 {
		t.Errorf("%d requests sent, want none", n)
	}
}

func TestARateLimitedFirstPairTellsToWaitAndSavesNothing(t *testing.T) {
	killy := newFakeKilly(t)
	killy.status = http.StatusTooManyRequests
	path := configPath(t)
	_, err := pair(t, path, killy.URL)
	if !errors.Is(err, ErrRateLimited) || !strings.Contains(err.Error(), "wait") {
		t.Errorf("err = %v, want ErrRateLimited asking to wait", err)
	}
	if _, err := os.Stat(path); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("agent.json: %v, want it not to exist", err)
	}
	if left, _ := os.ReadDir(filepath.Dir(path)); len(left) != 0 {
		t.Errorf("files left next to agent.json: %v", left)
	}
}

func TestTheFirstPairNeedsTheServerFlag(t *testing.T) {
	path := configPath(t)
	if _, err := pair(t, path, ""); err == nil || !strings.Contains(err.Error(), "--server") {
		t.Errorf("err = %v, want an error asking for --server", err)
	}
}

func TestPairRefusesPlainHTTPToAnotherMachine(t *testing.T) {
	path := configPath(t)
	_, err := pair(t, path, "http://killy.example.com")
	if err == nil || !strings.Contains(err.Error(), "https") {
		t.Errorf("err = %v, want a refusal asking for https", err)
	}
}

func TestALaterPairWithARejectedTokenSaysSo(t *testing.T) {
	killy := newFakeKilly(t)
	killy.status = http.StatusUnauthorized
	path := configPath(t)
	writeConfig(t, path, Config{Server: killy.URL, Token: issuedToken})
	_, err := pair(t, path, "")
	if err == nil || !strings.Contains(err.Error(), "token") || strings.Contains(err.Error(), issuedToken) {
		t.Errorf("err = %v, want an error about the rejected token that does not show it", err)
	}
}

func TestTheFirstPairSendsNothingWhenAgentJSONCannotBeWritten(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root can write the directory whatever its mode")
	}
	killy := newFakeKilly(t)
	dir := filepath.Join(t.TempDir(), "etc-killy")
	if err := os.Mkdir(dir, 0o500); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(dir, 0o700) })
	// Registering first would create an agent whose only token is then lost, and use up one of
	// this address's first-run registrations.
	_, err := pair(t, filepath.Join(dir, "agent.json"), killy.URL)
	if err == nil {
		t.Fatal("Pair succeeded although agent.json cannot be written")
	}
	if n := len(killy.received()); n != 0 {
		t.Errorf("%d requests sent, want none", n)
	}
}

func TestPairDoesNotFollowARedirectWithTheToken(t *testing.T) {
	elsewhere := newFakeKilly(t)
	redirecting := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, elsewhere.URL+r.URL.Path, http.StatusTemporaryRedirect)
	}))
	t.Cleanup(redirecting.Close)
	path := configPath(t)
	writeConfig(t, path, Config{Server: redirecting.URL, Token: issuedToken})
	if _, err := pair(t, path, ""); err == nil {
		t.Error("Pair succeeded through a redirect")
	}
	if n := len(elsewhere.received()); n != 0 {
		t.Errorf("the redirect target received %d requests, want none", n)
	}
}
