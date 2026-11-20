package main

import (
	"fmt"
	"os"
	"regexp"
)

var isoDate = regexp.MustCompile(`\d{4}-\d{2}-\d{2}`)

// lastUpdated returns the date in the document's first paragraph ("Last updated: 2026-11-19").
// A change to the policy bumps it, and the translation's date must follow.
func lastUpdated(path string) (string, error) {
	src, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	d := isoDate.FindString(string(src))
	if d == "" {
		return "", fmt.Errorf("%s has no date such as 2026-11-19", path)
	}
	return d, nil
}
