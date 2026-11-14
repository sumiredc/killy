package agent

import (
	"bytes"
	"context"
	"errors"
	"net/http"
	"net/url"
	"strings"
	"testing"
)

// qrRecorder is a RenderQR that keeps what it was asked to encode.
type qrRecorder struct {
	contents []string
	err      error
}

func (r *qrRecorder) render(content string) (string, error) {
	r.contents = append(r.contents, content)
	if r.err != nil {
		return "", r.err
	}
	return "[QR]\n", nil
}

func pairWithQR(t *testing.T, path, server string, qr *qrRecorder) (string, error) {
	t.Helper()
	var out bytes.Buffer
	err := Pair(context.Background(), PairOptions{ConfigPath: path, Server: server, Hostname: "web-1", Client: http.DefaultClient, RenderQR: qr.render}, &out)
	return out.String(), err
}

func wantPairURL(code, server string) string {
	return "killy://pair?code=" + code + "&server=" + url.QueryEscape(server)
}

func TestTheFirstPairPutsTheShownCodeAndTheSavedServerInTheQR(t *testing.T) {
	killy := newFakeKilly(t)
	path := configPath(t)
	qr := &qrRecorder{}
	// Typed with a trailing slash; agent.json keeps it without, and so does the QR.
	if _, err := pairWithQR(t, path, killy.URL+"/", qr); err != nil {
		t.Fatalf("Pair: %v", err)
	}
	saved := readConfig(t, path).Server
	if saved != killy.URL {
		t.Fatalf("agent.json server %q, want %q (normalized)", saved, killy.URL)
	}
	if want := wantPairURL("K7Q2ZP", killy.URL); len(qr.contents) != 1 || qr.contents[0] != want {
		t.Errorf("QR contents %q, want [%q]", qr.contents, want)
	}
}

func TestALaterPairWithoutServerPutsTheSavedServerInTheQR(t *testing.T) {
	killy := newFakeKilly(t)
	path := configPath(t)
	if _, err := pairWithQR(t, path, killy.URL, &qrRecorder{}); err != nil {
		t.Fatal(err)
	}
	for _, server := range []string{"", killy.URL + "/"} {
		qr := &qrRecorder{}
		out, err := pairWithQR(t, path, server, qr)
		if err != nil {
			t.Fatalf("--server %q: %v", server, err)
		}
		code := strings.TrimPrefix(codeLine.FindString(out), "Pairing code: ")
		if want := wantPairURL(code, killy.URL); len(qr.contents) != 1 || qr.contents[0] != want {
			t.Errorf("--server %q: QR contents %q, want [%q]", server, qr.contents, want)
		}
	}
}

func TestTheQRNeverCarriesTheAgentToken(t *testing.T) {
	killy := newFakeKilly(t)
	qr := &qrRecorder{}
	if _, err := pairWithQR(t, configPath(t), killy.URL, qr); err != nil {
		t.Fatal(err)
	}
	if len(qr.contents) != 1 || strings.Contains(qr.contents[0], issuedToken) || strings.Contains(qr.contents[0], "token") {
		t.Errorf("QR contents %q, want one without the agent token", qr.contents)
	}
}

func TestPairURLPercentEncodesTheServer(t *testing.T) {
	got := pairURL("K7Q2ZP", "https://killy.example/a b?x=1&y=2#z")
	if want := "killy://pair?code=K7Q2ZP&server=https%3A%2F%2Fkilly.example%2Fa+b%3Fx%3D1%26y%3D2%23z"; got != want {
		t.Errorf("pairURL = %q, want %q", got, want)
	}
	u, err := url.Parse(got)
	if err != nil {
		t.Fatal(err)
	}
	q := u.Query()
	if u.Scheme != "killy" || u.Host != "pair" || q.Get("code") != "K7Q2ZP" || q.Get("server") != "https://killy.example/a b?x=1&y=2#z" || len(q) != 2 {
		t.Errorf("parsed %s://%s %v, want killy://pair with the code and the whole server", u.Scheme, u.Host, q)
	}
}

func TestTheOutputShowsTheCodeTheQRTheHintAndTheAgentIDInThatOrder(t *testing.T) {
	killy := newFakeKilly(t)
	out, err := pairWithQR(t, configPath(t), killy.URL, &qrRecorder{})
	if err != nil {
		t.Fatal(err)
	}
	marks := []string{"Pairing code: K7Q2ZP", "[QR]", "within 10 minutes", "Agent ID: " + issuedAgent}
	last := -1
	for _, m := range marks {
		i := strings.Index(out, m)
		if i <= last {
			t.Fatalf("output %q: %q is missing or out of order (want %q in order)", out, m, marks)
		}
		last = i
	}
}

func TestAQRThatCannotBeMadeStillShowsTheCodeAndSucceeds(t *testing.T) {
	killy := newFakeKilly(t)
	path := configPath(t)
	out, err := pairWithQR(t, path, killy.URL, &qrRecorder{err: errors.New("data too long")})
	if err != nil {
		t.Fatalf("Pair: %v, want success: the code is issued and can be typed", err)
	}
	assertShowsCodeAndAgentButNotToken(t, out, "K7Q2ZP")
	if !strings.Contains(out, "QR code could not be made") {
		t.Errorf("output %q, want it to say the QR code could not be made", out)
	}
	if c := readConfig(t, path); c.Token != issuedToken {
		t.Errorf("agent.json %+v, want the token saved", c)
	}
}

func TestWithoutRenderQRTheQRIsDrawnInTheTerminal(t *testing.T) {
	killy := newFakeKilly(t)
	var out bytes.Buffer
	if err := Pair(context.Background(), PairOptions{ConfigPath: configPath(t), Server: killy.URL, Hostname: "web-1", Client: http.DefaultClient}, &out); err != nil {
		t.Fatal(err)
	}
	if !strings.ContainsAny(out.String(), "█▀▄") {
		t.Errorf("output %q, want a QR code drawn with block characters", out.String())
	}
}

func TestTerminalQRDrawsSquareRowsOfBlocks(t *testing.T) {
	s, err := TerminalQR(wantPairURL("K7Q2ZP", "https://killy.sumire.pink"))
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimRight(s, "\n"), "\n")
	// Two module rows per line: a version-3 code with its border is 37 modules wide, 19 lines tall.
	if len(lines) < 10 {
		t.Fatalf("%d lines, want a QR code", len(lines))
	}
	width := len([]rune(lines[0]))
	for i, l := range lines {
		if n := len([]rune(l)); n != width {
			t.Errorf("line %d is %d wide, want %d like the first", i, n, width)
		}
		if strings.Trim(l, " █▀▄") != "" {
			t.Errorf("line %d %q has characters other than blocks", i, l)
		}
	}
	if height := 2 * len(lines); height < width-1 || height > width+1 {
		t.Errorf("%d lines for %d columns, want a square code (2 rows per line)", len(lines), width)
	}
}
