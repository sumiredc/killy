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
	_, ok := l.take(key)
	return ok
}

// take counts one request for key if it fits, and returns the start of the window it was counted in.
func (l *Limiter) take(key string) (time.Time, bool) {
	now := l.clock.Now()
	l.mu.Lock()
	defer l.mu.Unlock()
	l.sweep(now)
	w, ok := l.windows[key]
	if !ok || !now.Before(w.start.Add(l.period)) {
		w = window{start: now}
	}
	if w.n >= l.limit {
		return time.Time{}, false
	}
	w.n++
	l.windows[key] = w
	return w.start, true
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

// Take counts one request for key like Allow, and returns a refund that uncounts it again. Failed
// claims count each claim up front and refund the ones that turn out not to fail, so claims still
// in flight count too: checking first and counting after the outcome would let concurrent guesses
// get past the limit. A refund is applied once at most and only while the window it was counted in
// lasts.
func (l *Limiter) Take(key string) (refund func(), ok bool) {
	start, ok := l.take(key)
	if !ok {
		return func() {}, false
	}
	var once sync.Once
	return func() {
		once.Do(func() {
			l.mu.Lock()
			defer l.mu.Unlock()
			if w, ok := l.windows[key]; ok && w.start.Equal(start) && w.n > 0 {
				w.n--
				l.windows[key] = w
			}
		})
	}, true
}

// Full reports, without counting anything, whether key has used up its current window.
func (l *Limiter) Full(key string) bool {
	now := l.clock.Now()
	l.mu.Lock()
	defer l.mu.Unlock()
	w, ok := l.windows[key]
	return ok && now.Before(w.start.Add(l.period)) && w.n >= l.limit
}
