// Package ratelimit counts requests per key in fixed windows, in memory.
package ratelimit

import (
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/sumiredc/killy/internal/clock"
)

// Limiter allows Limit requests per key in each window of Period. A key's window starts with its
// first allowed request and ends Period later; a request at exactly that end starts a new window.
// Counts live in memory, so a restart resets them.
type Limiter struct {
	limit  int
	period time.Duration
	clock  clock.Clock

	mu        sync.Mutex
	windows   map[string]window
	lastSweep time.Time
}

type window struct {
	start time.Time
	n     int
}

func New(limit int, period time.Duration, c clock.Clock) *Limiter {
	return &Limiter{limit: limit, period: period, clock: c, windows: map[string]window{}}
}

// Allow reports whether one more request for key fits in its window, and counts it if it does.
// A refused request is not counted.
func (l *Limiter) Allow(key string) bool {
	now := l.clock.Now()
	l.mu.Lock()
	defer l.mu.Unlock()
	l.sweep(now)
	w, ok := l.windows[key]
	if !ok || !now.Before(w.start.Add(l.period)) {
		w = window{start: now}
	}
	if w.n >= l.limit {
		return false
	}
	w.n++
	l.windows[key] = w
	return true
}

// sweep drops windows that have ended, so keys seen once do not stay in memory. It looks at every
// key at most once per period.
func (l *Limiter) sweep(now time.Time) {
	if now.Before(l.lastSweep.Add(l.period)) {
		return
	}
	l.lastSweep = now
	for key, w := range l.windows {
		if !now.Before(w.start.Add(l.period)) {
			delete(l.windows, key)
		}
	}
}

// ClientAddr is the address rate limits count by: CF-Connecting-IP, which Cloudflare sets to the
// client's address, or the TCP peer when the header is missing (the server run without Cloudflare).
func ClientAddr(r *http.Request) string {
	if ip := strings.TrimSpace(r.Header.Get("CF-Connecting-IP")); ip != "" {
		return ip
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}
