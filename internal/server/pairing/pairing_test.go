package pairing

import (
	"errors"
	"fmt"
	"regexp"
	"testing"
	"time"

	"github.com/sumiredc/killy/internal/clock"
)

var t0 = time.Date(2026, 11, 13, 9, 0, 0, 0, time.UTC)

const agentID = "0f8fad5b-d9cb-469f-a165-70867728950e"

func issue(t *testing.T, c *Codes, agentID string) string {
	t.Helper()
	code, err := c.Issue(agentID)
	if err != nil {
		t.Fatalf("Issue: %v", err)
	}
	return code
}

func TestCodesAreSixUpperCaseLettersOrDigits(t *testing.T) {
	c := New(clock.NewFake(t0))
	shape := regexp.MustCompile(`^[A-Z0-9]{6}$`)
	seenLetter, seenDigit := false, false
	for range 500 {
		code := issue(t, c, agentID)
		if !shape.MatchString(code) {
			t.Fatalf("code %q does not match [A-Z0-9]{6}", code)
		}
		seenLetter = seenLetter || regexp.MustCompile(`[A-Z]`).MatchString(code)
		seenDigit = seenDigit || regexp.MustCompile(`[0-9]`).MatchString(code)
	}
	if !seenLetter || !seenDigit {
		t.Errorf("500 codes used letters: %v, digits: %v; want both", seenLetter, seenDigit)
	}
}

func TestEveryIssueGivesADifferentCode(t *testing.T) {
	c := New(clock.NewFake(t0))
	seen := map[string]bool{}
	for i := range 1000 {
		code := issue(t, c, fmt.Sprintf("agent-%d", i))
		if seen[code] {
			t.Fatalf("code %q issued twice", code)
		}
		seen[code] = true
	}
}

func TestACodeLeadsToItsAgent(t *testing.T) {
	c := New(clock.NewFake(t0))
	code := issue(t, c, agentID)
	other := issue(t, c, "7c9e6679-7425-40de-944b-e07fc1f90ae7")
	if got, err := c.Lookup(code); err != nil || got != agentID {
		t.Errorf("Lookup(%s) = %q, %v; want %q", code, got, err, agentID)
	}
	if got, err := c.Lookup(other); err != nil || got != "7c9e6679-7425-40de-944b-e07fc1f90ae7" {
		t.Errorf("Lookup(%s) = %q, %v; want the other agent", other, got, err)
	}
}

func TestACodeIsValidJustBeforeTenMinutes(t *testing.T) {
	f := clock.NewFake(t0)
	c := New(f)
	code := issue(t, c, agentID)
	f.Advance(Lifetime - time.Nanosecond)
	if _, err := c.Lookup(code); err != nil {
		t.Errorf("Lookup just before 10 min: %v, want valid", err)
	}
}

func TestACodeExpiresAtExactlyTenMinutes(t *testing.T) {
	f := clock.NewFake(t0)
	c := New(f)
	code := issue(t, c, agentID)
	f.Advance(Lifetime)
	if _, err := c.Lookup(code); !errors.Is(err, ErrExpired) {
		t.Errorf("Lookup at 10 min: %v, want ErrExpired", err)
	}
}

func TestACodeStaysExpiredAfterTenMinutes(t *testing.T) {
	f := clock.NewFake(t0)
	c := New(f)
	code := issue(t, c, agentID)
	f.Advance(Lifetime + time.Minute)
	if _, err := c.Lookup(code); !errors.Is(err, ErrExpired) {
		t.Errorf("Lookup after 11 min: %v, want ErrExpired", err)
	}
}

func TestAnUnknownCodeIsUnknown(t *testing.T) {
	c := New(clock.NewFake(t0))
	issue(t, c, agentID)
	if _, err := c.Lookup("ZZZZZZ"); !errors.Is(err, ErrUnknown) {
		t.Errorf("Lookup(ZZZZZZ): %v, want ErrUnknown", err)
	}
}

func TestACodeStillKeptIsNotIssuedAgain(t *testing.T) {
	c := New(clock.NewFake(t0))
	draws := []string{"AAAAAA", "AAAAAA", "BBBBBB"}
	c.generate = func() (string, error) {
		code := draws[0]
		draws = draws[1:]
		return code, nil
	}
	first := issue(t, c, agentID)
	second := issue(t, c, "7c9e6679-7425-40de-944b-e07fc1f90ae7")
	if first != "AAAAAA" || second != "BBBBBB" {
		t.Errorf("codes %s, %s; want AAAAAA, then BBBBBB after the repeated draw", first, second)
	}
	if got, _ := c.Lookup("AAAAAA"); got != agentID {
		t.Errorf("AAAAAA leads to %q, want the first agent", got)
	}
}

func TestAnExpiredCodeIsKeptAnHourToBeReportedAsExpired(t *testing.T) {
	f := clock.NewFake(t0)
	c := New(f)
	code := issue(t, c, agentID)
	f.Advance(Lifetime + time.Hour - time.Nanosecond)
	issue(t, c, "7c9e6679-7425-40de-944b-e07fc1f90ae7") // a later Issue is when old codes are dropped
	if _, err := c.Lookup(code); !errors.Is(err, ErrExpired) {
		t.Errorf("Lookup just before 70 min: %v, want ErrExpired", err)
	}
	f.Advance(time.Nanosecond)
	issue(t, c, "16fd2706-8baf-433b-82eb-8c7fada847da")
	if _, err := c.Lookup(code); !errors.Is(err, ErrUnknown) {
		t.Errorf("Lookup at 70 min: %v, want ErrUnknown (dropped)", err)
	}
}

func TestANewCodeReplacesTheAgentsPreviousCode(t *testing.T) {
	c := New(clock.NewFake(t0))
	other := issue(t, c, "7c9e6679-7425-40de-944b-e07fc1f90ae7")
	old := issue(t, c, agentID)
	latest := issue(t, c, agentID)
	if _, err := c.Lookup(old); !errors.Is(err, ErrUnknown) {
		t.Errorf("Lookup(previous code): %v, want ErrUnknown", err)
	}
	if got, err := c.Lookup(latest); err != nil || got != agentID {
		t.Errorf("Lookup(latest code) = %q, %v; want %s", got, err, agentID)
	}
	if _, err := c.Lookup(other); err != nil {
		t.Errorf("another agent's code: %v, want still valid", err)
	}
}
