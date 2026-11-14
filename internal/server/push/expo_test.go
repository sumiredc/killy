package push

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

// expoServer records the requests it gets and answers each with answer.
type expoServer struct {
	mu       sync.Mutex
	requests []*http.Request
	bodies   [][]byte
	answer   func(w http.ResponseWriter, msgs []Message)
}

func newExpoServer(t *testing.T, answer func(w http.ResponseWriter, msgs []Message)) (*expoServer, *httptest.Server) {
	t.Helper()
	e := &expoServer{answer: answer}
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		e.mu.Lock()
		e.requests = append(e.requests, r)
		e.bodies = append(e.bodies, body)
		e.mu.Unlock()
		var msgs []Message
		_ = json.Unmarshal(body, &msgs)
		e.answer(w, msgs)
	}))
	t.Cleanup(ts.Close)
	return e, ts
}

func okTickets(w http.ResponseWriter, msgs []Message) {
	data := make([]map[string]string, len(msgs))
	for i := range msgs {
		data[i] = map[string]string{"status": "ok", "id": "ticket"}
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"data": data})
}

func messages(n int) []Message {
	msgs := make([]Message, n)
	for i := range msgs {
		msgs[i] = Message{To: "ExponentPushToken[" + strings.Repeat("a", i+1) + "]", Title: "Killy", Body: "b", Sound: "default", Priority: "high"}
	}
	return msgs
}

func TestExpoPostsTheMessagesAsAJSONArrayToItsURL(t *testing.T) {
	e, ts := newExpoServer(t, okTickets)
	x := &Expo{URL: ts.URL + "/--/api/v2/push/send"}
	msgs := messages(2)
	if _, err := x.Send(context.Background(), msgs); err != nil {
		t.Fatal(err)
	}
	if len(e.requests) != 1 {
		t.Fatalf("%d requests, want 1", len(e.requests))
	}
	r := e.requests[0]
	if r.Method != http.MethodPost || r.URL.Path != "/--/api/v2/push/send" {
		t.Errorf("%s %s, want POST /--/api/v2/push/send", r.Method, r.URL.Path)
	}
	if got := r.Header.Get("Content-Type"); got != "application/json" {
		t.Errorf("Content-Type %q, want application/json", got)
	}
	var got []Message
	if err := json.Unmarshal(e.bodies[0], &got); err != nil {
		t.Fatalf("body is not a JSON array of messages: %v (%s)", err, e.bodies[0])
	}
	if len(got) != 2 || got[0] != msgs[0] || got[1] != msgs[1] {
		t.Errorf("body %s, want the 2 messages in order", e.bodies[0])
	}
}

func TestExpoSendsToExpoPushServiceByDefault(t *testing.T) {
	if DefaultExpoURL != "https://exp.host/--/api/v2/push/send" {
		t.Errorf("DefaultExpoURL %q", DefaultExpoURL)
	}
	var got string
	x := &Expo{Client: &http.Client{Transport: roundTrip(func(r *http.Request) (*http.Response, error) {
		got = r.URL.String()
		return jsonResponse(http.StatusOK, `{"data":[{"status":"ok"}]}`), nil
	})}}
	if _, err := x.Send(context.Background(), messages(1)); err != nil {
		t.Fatal(err)
	}
	if got != DefaultExpoURL {
		t.Errorf("sent to %q, want %q", got, DefaultExpoURL)
	}
}

func TestExpoSendsTheAccessTokenOnlyWhenItIsSet(t *testing.T) {
	e, ts := newExpoServer(t, okTickets)
	if _, err := (&Expo{URL: ts.URL}).Send(context.Background(), messages(1)); err != nil {
		t.Fatal(err)
	}
	if _, err := (&Expo{URL: ts.URL, AccessToken: "secret-token"}).Send(context.Background(), messages(1)); err != nil {
		t.Fatal(err)
	}
	if got, ok := e.requests[0].Header["Authorization"]; ok {
		t.Errorf("Authorization %q without an access token, want none", got)
	}
	if got := e.requests[1].Header.Get("Authorization"); got != "Bearer secret-token" {
		t.Errorf("Authorization %q, want Bearer secret-token", got)
	}
}

func TestExpoReturnsTheTicketsInTheOrderOfTheMessages(t *testing.T) {
	_, ts := newExpoServer(t, func(w http.ResponseWriter, msgs []Message) {
		_, _ = io.WriteString(w, `{"data":[
			{"status":"ok","id":"1"},
			{"status":"error","message":"gone","details":{"error":"DeviceNotRegistered"}},
			{"status":"error","message":"too many","details":{"error":"MessageRateExceeded"}}]}`)
	})
	tickets, err := (&Expo{URL: ts.URL}).Send(context.Background(), messages(3))
	if err != nil {
		t.Fatal(err)
	}
	if len(tickets) != 3 {
		t.Fatalf("%d tickets, want 3", len(tickets))
	}
	if tickets[0].Status != "ok" {
		t.Errorf("ticket 0: %+v, want ok", tickets[0])
	}
	if tickets[1].Status != "error" || tickets[1].Details.Error != DeviceNotRegistered {
		t.Errorf("ticket 1: %+v, want DeviceNotRegistered", tickets[1])
	}
	if tickets[2].Status != "error" || tickets[2].Details.Error != "MessageRateExceeded" {
		t.Errorf("ticket 2: %+v, want MessageRateExceeded", tickets[2])
	}
}

func TestExpoRefusesMoreThan100MessagesWithoutSending(t *testing.T) {
	e, ts := newExpoServer(t, okTickets)
	x := &Expo{URL: ts.URL}
	if _, err := x.Send(context.Background(), messages(MaxMessages)); err != nil {
		t.Fatalf("%d messages: %v, want them sent", MaxMessages, err)
	}
	if _, err := x.Send(context.Background(), messages(MaxMessages+1)); err == nil {
		t.Errorf("%d messages: no error", MaxMessages+1)
	}
	if len(e.requests) != 1 {
		t.Errorf("%d requests, want only the one of %d messages", len(e.requests), MaxMessages)
	}
}

func TestExpoFailsOnAnAnswerThatIsNot200OrNotTickets(t *testing.T) {
	for name, answer := range map[string]func(w http.ResponseWriter){
		"400": func(w http.ResponseWriter) {
			w.WriteHeader(http.StatusBadRequest)
			_, _ = io.WriteString(w, `{"errors":[{"code":"VALIDATION_ERROR"}]}`)
		},
		"500": func(w http.ResponseWriter) { w.WriteHeader(http.StatusInternalServerError) },
		"429 with tickets": func(w http.ResponseWriter) {
			w.WriteHeader(http.StatusTooManyRequests)
			_, _ = io.WriteString(w, `{"data":[{"status":"ok"}]}`)
		},
		"not JSON": func(w http.ResponseWriter) { _, _ = io.WriteString(w, "<html>") },
		"no data":  func(w http.ResponseWriter) { _, _ = io.WriteString(w, `{"errors":[{"code":"INTERNAL_SERVER_ERROR"}]}`) },
	} {
		t.Run(name, func(t *testing.T) {
			_, ts := newExpoServer(t, func(w http.ResponseWriter, _ []Message) { answer(w) })
			if tickets, err := (&Expo{URL: ts.URL}).Send(context.Background(), messages(1)); err == nil {
				t.Errorf("no error, tickets %+v", tickets)
			}
		})
	}
}

func TestExpoFailsWhenItCannotConnect(t *testing.T) {
	ts := httptest.NewServer(http.NotFoundHandler())
	url := ts.URL
	ts.Close()
	if _, err := (&Expo{URL: url}).Send(context.Background(), messages(1)); err == nil {
		t.Error("no error")
	}
}

func TestExpoGivesARequest10Seconds(t *testing.T) {
	if RequestTimeout != 10*time.Second {
		t.Errorf("RequestTimeout %v, want 10s", RequestTimeout)
	}
	var left time.Duration
	var hasDeadline bool
	x := &Expo{Client: &http.Client{Transport: roundTrip(func(r *http.Request) (*http.Response, error) {
		var d time.Time
		d, hasDeadline = r.Context().Deadline()
		left = time.Until(d)
		return jsonResponse(http.StatusOK, `{"data":[{"status":"ok"}]}`), nil
	})}}
	if _, err := x.Send(context.Background(), messages(1)); err != nil {
		t.Fatal(err)
	}
	if !hasDeadline || left > RequestTimeout || left < RequestTimeout-time.Second {
		t.Errorf("deadline in %v (set: %v), want about %v", left, hasDeadline, RequestTimeout)
	}
}

func TestExpoGivesUpOnAStalledRequest(t *testing.T) {
	release := make(chan struct{})
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-release:
		case <-r.Context().Done():
		}
	}))
	defer ts.Close()
	defer close(release)
	start := time.Now()
	if _, err := (&Expo{URL: ts.URL, Timeout: 100 * time.Millisecond}).Send(context.Background(), messages(1)); err == nil {
		t.Error("no error")
	}
	if took := time.Since(start); took > 5*time.Second {
		t.Errorf("took %v", took)
	}
}

func TestExpoStopsWhenItsContextIsCancelled(t *testing.T) {
	release := make(chan struct{})
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-release:
		case <-r.Context().Done():
		}
	}))
	defer ts.Close()
	defer close(release)
	ctx, cancel := context.WithCancel(context.Background())
	time.AfterFunc(50*time.Millisecond, cancel)
	start := time.Now()
	if _, err := (&Expo{URL: ts.URL}).Send(ctx, messages(1)); err == nil {
		t.Error("no error")
	}
	if took := time.Since(start); took > 5*time.Second {
		t.Errorf("took %v", took)
	}
}

func TestExpoErrorsDoNotContainTheAccessToken(t *testing.T) {
	_, ts := newExpoServer(t, func(w http.ResponseWriter, _ []Message) { w.WriteHeader(http.StatusUnauthorized) })
	_, err := (&Expo{URL: ts.URL, AccessToken: "secret-token"}).Send(context.Background(), messages(1))
	if err == nil {
		t.Fatal("no error")
	}
	if strings.Contains(err.Error(), "secret-token") {
		t.Errorf("error %q contains the access token", err)
	}
}

type roundTrip func(*http.Request) (*http.Response, error)

func (f roundTrip) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func jsonResponse(status int, body string) *http.Response {
	return &http.Response{StatusCode: status, Header: http.Header{"Content-Type": {"application/json"}}, Body: io.NopCloser(strings.NewReader(body))}
}
