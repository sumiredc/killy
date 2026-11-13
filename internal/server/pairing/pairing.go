// Package pairing issues the 6-character codes an operator types into the app to add a server.
package pairing

import (
	"crypto/rand"
	"errors"
	"math/big"
	"sync"
	"time"

	"github.com/sumiredc/killy/internal/clock"
)

// Lifetime is how long a code stays valid after it is issued.
const Lifetime = 10 * time.Minute

var (
	ErrUnknown = errors.New("pairing code is unknown")
	ErrExpired = errors.New("pairing code has expired")
	ErrUsed    = errors.New("pairing code was already used")
)

// Codes keeps the codes in memory; a restart drops them and the operator runs killy agent pair again.
// It is safe for concurrent use.
type Codes struct {
	clock clock.Clock

	// generate makes a random code; tests replace it to force a collision.
	generate func() (string, error)

	mu    sync.Mutex
	codes map[string]entry
	// latest is each agent's current code. Only that one is valid, which keeps the codes in memory
	// to one per agent however often an agent asks.
	latest map[string]string
}

type entry struct {
	agentID string
	expires time.Time
	used    bool
}

func New(c clock.Clock) *Codes {
	return &Codes{clock: c, generate: newCode, codes: map[string]entry{}, latest: map[string]string{}}
}

// Issue returns a new code for agentID: 6 characters of A-Z and 0-9, not equal to any code still
// kept. The agent's previous code stops working.
func (c *Codes) Issue(agentID string) (string, error) {
	now := c.clock.Now()
	c.mu.Lock()
	defer c.mu.Unlock()
	c.sweep(now)
	for {
		code, err := c.generate()
		if err != nil {
			return "", err
		}
		if _, taken := c.codes[code]; taken {
			continue
		}
		if prev, ok := c.latest[agentID]; ok {
			delete(c.codes, prev)
		}
		c.codes[code] = entry{agentID: agentID, expires: now.Add(Lifetime)}
		c.latest[agentID] = code
		return code, nil
	}
}

// keepExpired is how long an expired code is kept, so a late claim is told it expired rather
// than that it is unknown.
const keepExpired = time.Hour

func (c *Codes) sweep(now time.Time) {
	for code, e := range c.codes {
		if !now.Before(e.expires.Add(keepExpired)) {
			delete(c.codes, code)
			if c.latest[e.agentID] == code {
				delete(c.latest, e.agentID)
			}
		}
	}
}

const alphabet = "ABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"

func newCode() (string, error) {
	b := make([]byte, 6)
	max := big.NewInt(int64(len(alphabet)))
	for i := range b {
		n, err := rand.Int(rand.Reader, max)
		if err != nil {
			return "", err
		}
		b[i] = alphabet[n.Int64()]
	}
	return string(b), nil
}

// Lookup returns the agent a code was issued for. A code is valid until Lifetime after it was
// issued; from that instant on it is ErrExpired.
func (c *Codes) Lookup(code string) (string, error) {
	now := c.clock.Now()
	c.mu.Lock()
	defer c.mu.Unlock()
	e, ok := c.codes[code]
	if !ok {
		return "", ErrUnknown
	}
	if !now.Before(e.expires) {
		return "", ErrExpired
	}
	return e.agentID, nil
}

// Claim uses a code once. It returns the agent the code was issued for and a release function that
// makes the code usable again, for when linking the device fails afterwards. Of two claims of the
// same code, only one succeeds; the other gets ErrUsed. A used code is reported as used even after
// it expires.
func (c *Codes) Claim(code string) (agentID string, release func(), err error) {
	now := c.clock.Now()
	c.mu.Lock()
	defer c.mu.Unlock()
	e, ok := c.codes[code]
	switch {
	case !ok:
		return "", nil, ErrUnknown
	case e.used:
		return "", nil, ErrUsed
	case !now.Before(e.expires):
		return "", nil, ErrExpired
	}
	e.used = true
	c.codes[code] = e
	release = func() {
		c.mu.Lock()
		defer c.mu.Unlock()
		if e, ok := c.codes[code]; ok {
			e.used = false
			c.codes[code] = e
		}
	}
	return e.agentID, release, nil
}
