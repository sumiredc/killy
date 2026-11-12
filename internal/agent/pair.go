// Package agent is the killy agent: pairing with the Killy server, and later reporting zombies
// and running kills.
package agent

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
)

// DefaultConfigPath is where the agent keeps its token and the URL of its Killy server.
const DefaultConfigPath = "/etc/killy/agent.json"

// Config is the content of agent.json. Token is a secret.
type Config struct {
	Server string `json:"server"`
	Token  string `json:"token"`
}

// ErrRateLimited means the Killy server refused a first-run registration because too many came
// from this network recently.
var ErrRateLimited = errors.New("the Killy server is limiting new registrations from this network; wait a while and run killy agent pair again")

type PairOptions struct {
	ConfigPath string
	// Server is the --server flag. Required on the first run; afterwards it may be empty or must
	// match the saved URL.
	Server   string
	Hostname string
	Client   *http.Client
}

type pairingResponse struct {
	AgentToken string `json:"agent_token"`
	AgentID    string `json:"agent_id"`
	Code       string `json:"code"`
}

// Pair registers the agent on its first run, or asks for a new code with the saved token, and
// prints the code and the agent ID to out. It never prints the token.
func Pair(ctx context.Context, opts PairOptions, out io.Writer) error {
	cfg, err := LoadConfig(opts.ConfigPath)
	switch {
	case errors.Is(err, fs.ErrNotExist):
		return firstPair(ctx, opts, out)
	case err != nil:
		return err
	}
	// The token belongs to the saved server. Sending it anywhere else would hand it to that server.
	if opts.Server != "" && normalizeURL(opts.Server) != normalizeURL(cfg.Server) {
		return fmt.Errorf("%s is paired with %s; refusing to send its token to %s (remove the file to pair with another Killy server)",
			opts.ConfigPath, cfg.Server, opts.Server)
	}
	resp, err := requestCode(ctx, opts, cfg.Server, cfg.Token)
	if err != nil {
		return err
	}
	printCode(out, resp)
	return nil
}

func firstPair(ctx context.Context, opts PairOptions, out io.Writer) error {
	if opts.Server == "" {
		return fmt.Errorf("--server is required the first time (no %s yet)", opts.ConfigPath)
	}
	server := normalizeURL(opts.Server)
	// Make sure agent.json can be written before registering: the server returns the token only
	// once, so a registration whose token cannot be saved leaves an agent nobody can use and uses
	// up one of this address's first-run registrations.
	pending, err := prepareConfig(opts.ConfigPath)
	if err != nil {
		return fmt.Errorf("cannot write %s (run killy agent pair as root): %w", opts.ConfigPath, err)
	}
	defer pending.discard()
	resp, err := requestCode(ctx, opts, server, "")
	if err != nil {
		return err
	}
	if resp.AgentToken == "" {
		return errors.New("the Killy server registered the agent but returned no token")
	}
	if err := pending.commit(Config{Server: server, Token: resp.AgentToken}); err != nil {
		return err
	}
	printCode(out, resp)
	return nil
}

func requestCode(ctx context.Context, opts PairOptions, server, token string) (pairingResponse, error) {
	if err := checkServerURL(server); err != nil {
		return pairingResponse{}, err
	}
	body, err := json.Marshal(map[string]string{"hostname": opts.Hostname})
	if err != nil {
		return pairingResponse{}, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, server+"/agent/v1/pairings", bytes.NewReader(body))
	if err != nil {
		return pairingResponse{}, err
	}
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	res, err := noRedirects(opts.Client).Do(req)
	if err != nil {
		return pairingResponse{}, fmt.Errorf("reach the Killy server: %w", err)
	}
	defer res.Body.Close()
	switch res.StatusCode {
	case http.StatusOK:
	case http.StatusTooManyRequests:
		return pairingResponse{}, ErrRateLimited
	case http.StatusUnauthorized:
		return pairingResponse{}, fmt.Errorf("the Killy server does not accept the token in %s; remove the file and pair again", opts.ConfigPath)
	default:
		var e struct{ Error string }
		_ = json.NewDecoder(io.LimitReader(res.Body, 4096)).Decode(&e)
		return pairingResponse{}, fmt.Errorf("the Killy server answered %s (%s)", res.Status, e.Error)
	}
	var resp pairingResponse
	if err := json.NewDecoder(io.LimitReader(res.Body, 64<<10)).Decode(&resp); err != nil {
		return pairingResponse{}, fmt.Errorf("read the Killy server's answer: %w", err)
	}
	if resp.AgentID == "" || resp.Code == "" {
		return pairingResponse{}, errors.New("the Killy server's answer has no agent ID or code")
	}
	return resp, nil
}

func printCode(out io.Writer, resp pairingResponse) {
	fmt.Fprintf(out, "Pairing code: %s\n", resp.Code)
	fmt.Fprintln(out, "Enter it in the Killy app (Settings → Add server) within 10 minutes. It works once.")
	fmt.Fprintf(out, "Agent ID: %s\n", resp.AgentID)
}

func normalizeURL(s string) string {
	return strings.TrimRight(strings.TrimSpace(s), "/")
}

// checkServerURL allows https, and plain http only to this machine, so the token never crosses a
// network unencrypted.
func checkServerURL(s string) error {
	u, err := url.Parse(s)
	if err != nil || u.Host == "" {
		return fmt.Errorf("%q is not a Killy server URL", s)
	}
	if u.Scheme == "https" {
		return nil
	}
	if u.Scheme == "http" {
		host := u.Hostname()
		if ip := net.ParseIP(host); host == "localhost" || (ip != nil && ip.IsLoopback()) {
			return nil
		}
	}
	return fmt.Errorf("%s: use https (plain http is allowed only to localhost)", s)
}

// LoadConfig reads agent.json. A missing file gives an error matching fs.ErrNotExist.
func LoadConfig(path string) (Config, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return Config{}, err
	}
	var c Config
	if err := json.Unmarshal(b, &c); err != nil || c.Server == "" || c.Token == "" {
		return Config{}, fmt.Errorf("%s is not a valid agent configuration", path)
	}
	return c, nil
}

// SaveConfig writes agent.json with mode 0600, replacing it in one rename so a crash cannot leave
// a half-written file.
func SaveConfig(path string, c Config) error {
	pending, err := prepareConfig(path)
	if err != nil {
		return err
	}
	defer pending.discard()
	return pending.commit(c)
}

// pendingConfig is an open temporary file next to agent.json that becomes agent.json on commit.
type pendingConfig struct {
	path string
	f    *os.File
	done bool
}

func prepareConfig(path string) (*pendingConfig, error) {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, err
	}
	f, err := os.CreateTemp(dir, ".agent-*.json")
	if err != nil {
		return nil, err
	}
	if err := f.Chmod(0o600); err != nil {
		f.Close()
		os.Remove(f.Name())
		return nil, err
	}
	return &pendingConfig{path: path, f: f}, nil
}

func (p *pendingConfig) commit(c Config) error {
	b, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}
	if _, err := p.f.Write(append(b, '\n')); err != nil {
		return err
	}
	if err := p.f.Sync(); err != nil {
		return err
	}
	if err := p.f.Close(); err != nil {
		return err
	}
	if err := os.Rename(p.f.Name(), p.path); err != nil {
		return err
	}
	p.done = true
	return nil
}

// discard removes the temporary file unless commit renamed it into place.
func (p *pendingConfig) discard() {
	if p.done {
		return
	}
	p.f.Close()
	os.Remove(p.f.Name())
}
