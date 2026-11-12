package ratelimit

import (
	"net/http/httptest"
	"testing"
	"time"

	"github.com/sumiredc/killy/internal/clock"
)

var t0 = time.Date(2026, 11, 13, 9, 0, 0, 0, time.UTC)

func allowN(t *testing.T, l *Limiter, key string, n int) {
	t.Helper()
	for i := range n {
		if !l.Allow(key) {
			t.Fatalf("request %d for %s refused, want allowed", i+1, key)
		}
	}
}

func TestAllowAcceptsTheLimitAndRefusesTheNext(t *testing.T) {
	l := New(10, time.Hour, clock.NewFake(t0))
	allowN(t, l, "a", 10)
	if l.Allow("a") {
		t.Error("request 11 allowed, want refused")
	}
}

func TestAllowRefusesUntilTheWindowEnds(t *testing.T) {
	c := clock.NewFake(t0)
	l := New(10, time.Hour, c)
	allowN(t, l, "a", 10)
	c.Advance(time.Hour - time.Nanosecond)
	if l.Allow("a") {
		t.Error("allowed just before the window ends, want refused")
	}
}

func TestAllowStartsANewWindowExactlyWhenThePeriodHasPassed(t *testing.T) {
	c := clock.NewFake(t0)
	l := New(10, time.Hour, c)
	allowN(t, l, "a", 10)
	c.Advance(time.Hour)
	allowN(t, l, "a", 10)
	if l.Allow("a") {
		t.Error("request 11 of the new window allowed, want refused")
	}
}

func TestTheWindowStartsAtTheFirstRequest(t *testing.T) {
	c := clock.NewFake(t0)
	l := New(2, time.Hour, c)
	c.Advance(30 * time.Minute)
	allowN(t, l, "a", 2)
	c.Advance(59 * time.Minute)
	if l.Allow("a") {
		t.Error("allowed 59 min after the first request, want refused")
	}
}

func TestRefusedRequestsDoNotMoveOrFillTheNextWindow(t *testing.T) {
	c := clock.NewFake(t0)
	l := New(2, time.Hour, c)
	allowN(t, l, "a", 2)
	c.Advance(30 * time.Minute)
	for range 5 {
		l.Allow("a")
	}
	c.Advance(30 * time.Minute)
	allowN(t, l, "a", 2)
}

func TestKeysAreCountedSeparately(t *testing.T) {
	l := New(1, time.Hour, clock.NewFake(t0))
	allowN(t, l, "a", 1)
	allowN(t, l, "b", 1)
	if l.Allow("a") {
		t.Error("second request for a allowed, want refused")
	}
}

func TestClientAddrUsesCFConnectingIP(t *testing.T) {
	r := httptest.NewRequest("POST", "/", nil)
	r.RemoteAddr = "10.42.0.7:51234"
	r.Header.Set("CF-Connecting-IP", "203.0.113.9")
	if got := ClientAddr(r); got != "203.0.113.9" {
		t.Errorf("ClientAddr = %q, want 203.0.113.9", got)
	}
}

func TestClientAddrFallsBackToTheTCPPeerWithoutThePort(t *testing.T) {
	r := httptest.NewRequest("POST", "/", nil)
	for remote, want := range map[string]string{
		"192.0.2.4:51234":     "192.0.2.4",
		"[2001:db8::1]:51234": "2001:db8::1",
	} {
		r.RemoteAddr = remote
		if got := ClientAddr(r); got != want {
			t.Errorf("ClientAddr with RemoteAddr %q = %q, want %q", remote, got, want)
		}
	}
}
