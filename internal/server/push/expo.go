package push

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// DefaultExpoURL is Expo Push Service's send endpoint.
const DefaultExpoURL = "https://exp.host/--/api/v2/push/send"

// RequestTimeout is how long one request to Expo may take before it counts as failed. The
// notifier has one send per agent, so a stalled request must not hold back later pushes.
const RequestTimeout = 10 * time.Second

// Expo sends push messages to Expo Push Service. The zero value sends to DefaultExpoURL without
// an access token, with http.DefaultClient and RequestTimeout.
type Expo struct {
	URL string
	// AccessToken is sent as a bearer token when set; Expo requires it only when push security
	// is turned on for the project. It is never logged or put in an error.
	AccessToken string
	Client      *http.Client
	Timeout     time.Duration
}

// Send posts at most MaxMessages messages in one request and returns Expo's tickets, which are in
// the order of the messages. Any answer other than 200 with tickets is an error.
func (e *Expo) Send(ctx context.Context, msgs []Message) ([]Ticket, error) {
	if len(msgs) > MaxMessages {
		return nil, fmt.Errorf("%d messages, Expo takes at most %d", len(msgs), MaxMessages)
	}
	body, err := json.Marshal(msgs)
	if err != nil {
		return nil, err
	}
	timeout := e.Timeout
	if timeout == 0 {
		timeout = RequestTimeout
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	url := e.URL
	if url == "" {
		url = DefaultExpoURL
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	if e.AccessToken != "" {
		req.Header.Set("Authorization", "Bearer "+e.AccessToken)
	}
	client := e.Client
	if client == nil {
		client = http.DefaultClient
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("expo push: %w", err)
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, fmt.Errorf("expo push: read answer: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("expo push: status %d", resp.StatusCode)
	}
	var answer struct {
		Data []Ticket `json:"data"`
	}
	if err := json.Unmarshal(data, &answer); err != nil {
		return nil, fmt.Errorf("expo push: answer is not JSON: %w", err)
	}
	if answer.Data == nil {
		return nil, fmt.Errorf("expo push: answer has no tickets")
	}
	return answer.Data, nil
}
