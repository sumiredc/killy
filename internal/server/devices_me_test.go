package server

import (
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"

	"github.com/sumiredc/killy/internal/server/memstore"
)

func (s *testServer) patchDevice(t *testing.T, deviceToken, body string) (int, string) {
	t.Helper()
	r := httptest.NewRequest(http.MethodPatch, "/app/v1/devices/me", strings.NewReader(body))
	r.RemoteAddr = phoneAddr + ":40000"
	r.Header.Set("Content-Type", "application/json")
	if deviceToken != "" {
		r.Header.Set("Authorization", "Bearer "+deviceToken)
	}
	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, r)
	return w.Code, w.Body.String()
}

// deviceRow returns the stored row of the n-th registered device.
func (s *testServer) deviceRow(t *testing.T, n int) memstore.DeviceRow {
	t.Helper()
	rows := s.mem.Devices()
	if len(rows) <= n {
		t.Fatalf("%d devices, want at least %d", len(rows), n+1)
	}
	return rows[n]
}

func TestANewDeviceHasTheAppSwitchOn(t *testing.T) {
	s := newTestServer(t)
	s.newDevice(t)
	if row := s.deviceRow(t, 0); !row.Notify {
		t.Errorf("device %+v, want the app-wide switch on (the default of the devices table)", row)
	}
}

func TestPatchingThePushTokenSavesItAndLeavesTheSwitch(t *testing.T) {
	s := newTestServer(t)
	device := s.newDevice(t)
	if status, body := s.patchDevice(t, device, `{"push_token":"ExponentPushToken[a]"}`); status != http.StatusNoContent {
		t.Fatalf("PATCH: %d %s, want 204", status, body)
	}
	if row := s.deviceRow(t, 0); row.PushToken != "ExponentPushToken[a]" || !row.Notify {
		t.Errorf("device %+v, want the push token saved and the switch still on", row)
	}
}

func TestPatchingTheSwitchSavesItAndLeavesThePushToken(t *testing.T) {
	s := newTestServer(t)
	device := s.newDevice(t)
	s.patchDevice(t, device, `{"push_token":"ExponentPushToken[a]"}`)
	if status, body := s.patchDevice(t, device, `{"notify":false}`); status != http.StatusNoContent {
		t.Fatalf("PATCH: %d %s, want 204", status, body)
	}
	if row := s.deviceRow(t, 0); row.Notify || row.PushToken != "ExponentPushToken[a]" {
		t.Errorf("device %+v, want the switch off and the push token kept", row)
	}
	s.patchDevice(t, device, `{"notify":true}`)
	if row := s.deviceRow(t, 0); !row.Notify {
		t.Errorf("device %+v, want the switch on again", row)
	}
}

func TestAnEmptyOrNullPatchChangesNothing(t *testing.T) {
	for _, body := range []string{`{}`, `{"push_token":null}`, `{"notify":null}`, `{"push_token":null,"notify":null}`} {
		t.Run(body, func(t *testing.T) {
			s := newTestServer(t)
			device := s.newDevice(t)
			s.patchDevice(t, device, `{"push_token":"ExponentPushToken[a]","notify":false}`)
			before := s.deviceRow(t, 0)
			if status, resp := s.patchDevice(t, device, body); status != http.StatusNoContent {
				t.Fatalf("PATCH: %d %s, want 204", status, resp)
			}
			if after := s.deviceRow(t, 0); after.PushToken != before.PushToken || after.Notify != before.Notify {
				t.Errorf("device %+v after %s, want %+v unchanged", after, body, before)
			}
		})
	}
}

func TestAPushTokenOf512BytesIsAccepted(t *testing.T) {
	s := newTestServer(t)
	device := s.newDevice(t)
	token := strings.Repeat("x", 512)
	if status, body := s.patchDevice(t, device, `{"push_token":"`+token+`"}`); status != http.StatusNoContent {
		t.Fatalf("PATCH: %d %s, want 204", status, body)
	}
	if row := s.deviceRow(t, 0); row.PushToken != token {
		t.Errorf("push token of %d bytes, want the 512-byte one", len(row.PushToken))
	}
}

func TestAnInvalidPatchIsRefusedAndChangesNothing(t *testing.T) {
	for name, body := range map[string]string{
		"empty push token":     `{"push_token":""}`,
		"push token over 512":  `{"push_token":"` + strings.Repeat("x", 513) + `","notify":false}`,
		"push token not text":  `{"push_token":3}`,
		"switch not a boolean": `{"notify":"no"}`,
		"not JSON":             `push`,
		"trailing data":        `{} {}`,
	} {
		t.Run(name, func(t *testing.T) {
			s := newTestServer(t)
			device := s.newDevice(t)
			s.patchDevice(t, device, `{"push_token":"ExponentPushToken[a]"}`)
			before := s.deviceRow(t, 0)
			if status, resp := s.patchDevice(t, device, body); status != http.StatusBadRequest || errorCode(t, resp) != "invalid_request" {
				t.Errorf("PATCH: %d %s, want 400 invalid_request", status, resp)
			}
			if after := s.deviceRow(t, 0); after.PushToken != before.PushToken || after.Notify != before.Notify {
				t.Errorf("device %+v, want %+v unchanged", after, before)
			}
		})
	}
}

func TestAnInvalidDeviceTokenCannotPatch(t *testing.T) {
	s := newTestServer(t)
	s.newDevice(t)
	before := s.mem.Devices()
	for _, token := range []string{"", "bm90LWEtZGV2aWNl"} {
		if status, body := s.patchDevice(t, token, `{"push_token":"ExponentPushToken[a]","notify":false}`); status != http.StatusUnauthorized || errorCode(t, body) != "unauthorized" {
			t.Errorf("token %q: %d %s, want 401 unauthorized", token, status, body)
		}
	}
	if after := s.mem.Devices(); !slices.EqualFunc(before, after, func(a, b memstore.DeviceRow) bool { return a.PushToken == b.PushToken && a.Notify == b.Notify }) {
		t.Errorf("devices changed from %+v to %+v", before, after)
	}
}

func TestAPatchIsUnavailableWhenTheDatabaseCannotBeReached(t *testing.T) {
	for name, fail := range map[string]func(*testServer){
		"every call fails": (*testServer).unreachableDB,
		"the update fails": func(s *testServer) { s.mem.FailUpdateDevice = true },
	} {
		t.Run(name, func(t *testing.T) {
			s := newTestServer(t)
			device := s.newDevice(t)
			fail(s)
			if status, body := s.patchDevice(t, device, `{"notify":false}`); status != http.StatusServiceUnavailable || errorCode(t, body) != "unavailable" {
				t.Errorf("PATCH: %d %s, want 503 unavailable", status, body)
			}
		})
	}
}

func TestAPushTokenMovesToTheDeviceThatRegistersItLast(t *testing.T) {
	s := newTestServer(t)
	first, second := s.newDevice(t), s.newDevice(t)
	s.patchDevice(t, first, `{"push_token":"ExponentPushToken[same]"}`)
	if status, body := s.patchDevice(t, second, `{"push_token":"ExponentPushToken[same]"}`); status != http.StatusNoContent {
		t.Fatalf("PATCH: %d %s, want 204", status, body)
	}
	if a, b := s.deviceRow(t, 0), s.deviceRow(t, 1); a.PushToken != "" || b.PushToken != "ExponentPushToken[same]" {
		t.Errorf("first %q, second %q; want the token only on the second device", a.PushToken, b.PushToken)
	}
}

func TestAnotherDevicesDifferentPushTokenIsKept(t *testing.T) {
	s := newTestServer(t)
	first, second := s.newDevice(t), s.newDevice(t)
	s.patchDevice(t, first, `{"push_token":"ExponentPushToken[one]"}`)
	s.patchDevice(t, second, `{"push_token":"ExponentPushToken[two]"}`)
	if a := s.deviceRow(t, 0); a.PushToken != "ExponentPushToken[one]" {
		t.Errorf("first %q, want its own token kept", a.PushToken)
	}
}
