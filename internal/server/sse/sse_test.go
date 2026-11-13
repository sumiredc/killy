package sse

import (
	"bufio"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestStartSendsTheEventStreamHeaders(t *testing.T) {
	rec := httptest.NewRecorder()
	if _, err := Start(rec); err != nil {
		t.Fatalf("Start: %v", err)
	}
	if ct := rec.Header().Get("Content-Type"); ct != "text/event-stream" {
		t.Errorf("Content-Type = %q, want text/event-stream", ct)
	}
	if cc := rec.Header().Get("Cache-Control"); cc != "no-cache" {
		t.Errorf("Cache-Control = %q, want no-cache", cc)
	}
	if rec.Code != http.StatusOK || !rec.Flushed {
		t.Errorf("status %d, flushed %v; want 200 sent at once", rec.Code, rec.Flushed)
	}
}

func TestEventWritesNameAndJSONData(t *testing.T) {
	rec := httptest.NewRecorder()
	w, _ := Start(rec)
	if err := w.Event("state", map[string]any{"servers": []int{}}); err != nil {
		t.Fatal(err)
	}
	if got, want := rec.Body.String(), "event: state\ndata: {\"servers\":[]}\n\n"; got != want {
		t.Errorf("body %q, want %q", got, want)
	}
}

func TestHeartbeatWritesAComment(t *testing.T) {
	rec := httptest.NewRecorder()
	w, _ := Start(rec)
	if err := w.Heartbeat(); err != nil {
		t.Fatal(err)
	}
	if got := rec.Body.String(); got != ": hb\n\n" {
		t.Errorf("body %q, want %q", got, ": hb\n\n")
	}
}

func TestAStreamOutlivesTheServersWriteTimeout(t *testing.T) {
	send := make(chan struct{})
	srv := httptest.NewUnstartedServer(http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		w, err := Start(rw)
		if err != nil {
			return
		}
		<-send
		_ = w.Heartbeat()
	}))
	srv.Config.WriteTimeout = 100 * time.Millisecond
	srv.Start()
	defer srv.Close()

	res, err := http.Get(srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	time.Sleep(300 * time.Millisecond)
	close(send)
	line, err := bufio.NewReader(res.Body).ReadString('\n')
	if err != nil || strings.TrimSpace(line) != ": hb" {
		t.Errorf("read %q, %v after the write timeout; want the heartbeat", line, err)
	}
}

func received(ch <-chan struct{}) bool {
	select {
	case <-ch:
		return true
	case <-time.After(50 * time.Millisecond):
		return false
	}
}

func TestNotifyReachesOnlyTheDevicesNamed(t *testing.T) {
	b := NewBroker()
	a, cancelA := b.Subscribe("a")
	defer cancelA()
	other, cancelOther := b.Subscribe("b")
	defer cancelOther()
	b.Notify("a")
	if !received(a) {
		t.Error("device a was not notified")
	}
	if received(other) {
		t.Error("device b was notified")
	}
}

func TestEveryStreamOfADeviceIsNotified(t *testing.T) {
	b := NewBroker()
	one, c1 := b.Subscribe("a")
	defer c1()
	two, c2 := b.Subscribe("a")
	defer c2()
	b.Notify("a")
	if !received(one) || !received(two) {
		t.Error("not every stream of the device was notified")
	}
}

func TestNotificationsBeforeAReadAreMergedAndNeverBlock(t *testing.T) {
	b := NewBroker()
	ch, cancel := b.Subscribe("a")
	defer cancel()
	done := make(chan struct{})
	go func() {
		for range 100 {
			b.Notify("a")
		}
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("Notify blocked on a stream that does not read")
	}
	if !received(ch) {
		t.Fatal("no notification")
	}
	if received(ch) {
		t.Error("100 notifications before a read arrived as more than one")
	}
}

func TestACancelledStreamIsNotNotified(t *testing.T) {
	b := NewBroker()
	ch, cancel := b.Subscribe("a")
	cancel()
	b.Notify("a")
	if received(ch) {
		t.Error("a cancelled stream was notified")
	}
	if n := len(b.subs); n != 0 {
		t.Errorf("%d devices still tracked, want 0", n)
	}
}
