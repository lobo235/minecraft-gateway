package api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
)

// --- Mock RCON client ---

type mockRCON struct {
	response string
	err      error
}

func (m *mockRCON) Execute(string, string) (string, error) { return m.response, m.err }

// --- Helpers ---

func newTestServer(rconClient *mockRCON) *Server {
	log := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelError}))
	return NewServer(rconClient, "test-api-key", "test", log)
}

func doRequest(handler http.Handler, method, path string, body any) *httptest.ResponseRecorder {
	var reqBody *bytes.Buffer
	if body != nil {
		data, _ := json.Marshal(body)
		reqBody = bytes.NewBuffer(data)
	} else {
		reqBody = &bytes.Buffer{}
	}
	req := httptest.NewRequest(method, path, reqBody)
	req.Header.Set("Authorization", "Bearer test-api-key")
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	return rr
}

func doRequestNoAuth(handler http.Handler, method, path string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	return rr
}

// --- Health ---

func TestHealthEndpoint(t *testing.T) {
	s := newTestServer(&mockRCON{})
	rr := doRequestNoAuth(s.Handler(), "GET", "/health")

	if rr.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", rr.Code)
	}
	var resp healthResponse
	json.NewDecoder(rr.Body).Decode(&resp)
	if resp.Status != "ok" {
		t.Errorf("status = %q, want ok", resp.Status)
	}
	if resp.Version != "test" {
		t.Errorf("version = %q, want test", resp.Version)
	}
}

// --- Auth ---

func TestUnauthorized(t *testing.T) {
	s := newTestServer(&mockRCON{})
	rr := doRequestNoAuth(s.Handler(), "POST", "/servers/mc-test/rcon")

	if rr.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", rr.Code)
	}
}

func TestWrongAPIKey(t *testing.T) {
	s := newTestServer(&mockRCON{})
	req := httptest.NewRequest("POST", "/servers/mc-test/rcon", bytes.NewBufferString(`{"command":"say hi"}`))
	req.Header.Set("Authorization", "Bearer wrong-key")
	rr := httptest.NewRecorder()
	s.Handler().ServeHTTP(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", rr.Code)
	}
}

func TestNoBearerPrefix(t *testing.T) {
	s := newTestServer(&mockRCON{})
	req := httptest.NewRequest("POST", "/servers/mc-test/rcon", bytes.NewBufferString(`{"command":"say hi"}`))
	req.Header.Set("Authorization", "test-api-key")
	rr := httptest.NewRecorder()
	s.Handler().ServeHTTP(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", rr.Code)
	}
}

// --- RCON ---

func TestRCONExecute(t *testing.T) {
	s := newTestServer(&mockRCON{response: "Done!"})
	body := map[string]string{"command": "say hello"}
	rr := doRequest(s.Handler(), "POST", "/servers/mc-test/rcon", body)

	if rr.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", rr.Code)
	}
	var resp map[string]string
	json.NewDecoder(rr.Body).Decode(&resp)
	if resp["response"] != "Done!" {
		t.Errorf("response = %q, want 'Done!'", resp["response"])
	}
}

func TestRCONExecuteMissingCommand(t *testing.T) {
	s := newTestServer(&mockRCON{})
	body := map[string]string{}
	rr := doRequest(s.Handler(), "POST", "/servers/mc-test/rcon", body)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", rr.Code)
	}
}

func TestRCONOp(t *testing.T) {
	s := newTestServer(&mockRCON{response: "Made Steve a server operator"})
	body := map[string]string{"player": "Steve"}
	rr := doRequest(s.Handler(), "POST", "/servers/mc-test/rcon/op", body)

	if rr.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", rr.Code)
	}
}

func TestRCONOpInvalidPlayer(t *testing.T) {
	s := newTestServer(&mockRCON{})
	body := map[string]string{"player": "invalid player name!"}
	rr := doRequest(s.Handler(), "POST", "/servers/mc-test/rcon/op", body)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", rr.Code)
	}
}

func TestRCONDeop(t *testing.T) {
	s := newTestServer(&mockRCON{response: "Made Steve no longer a server operator"})
	body := map[string]string{"player": "Steve"}
	rr := doRequest(s.Handler(), "POST", "/servers/mc-test/rcon/deop", body)

	if rr.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", rr.Code)
	}
}

func TestRCONWhitelist(t *testing.T) {
	s := newTestServer(&mockRCON{response: "Added Steve to the whitelist"})
	body := map[string]any{"action": "add", "player": "Steve"}
	rr := doRequest(s.Handler(), "POST", "/servers/mc-test/rcon/whitelist", body)

	if rr.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", rr.Code)
	}
}

func TestRCONWhitelistInvalidAction(t *testing.T) {
	s := newTestServer(&mockRCON{})
	body := map[string]any{"action": "invalid", "player": "Steve"}
	rr := doRequest(s.Handler(), "POST", "/servers/mc-test/rcon/whitelist", body)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", rr.Code)
	}
}

func TestRCONPlayers(t *testing.T) {
	s := newTestServer(&mockRCON{response: "There are 2 of a max of 20 players online: Steve, Alex"})
	rr := doRequest(s.Handler(), "GET", "/servers/mc-test/rcon/players", nil)

	if rr.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", rr.Code)
	}
}

// --- Server name validation across RCON endpoints ---

func TestInvalidServerNameRejected(t *testing.T) {
	s := newTestServer(&mockRCON{})
	handler := s.Handler()

	endpoints := []struct {
		method string
		path   string
	}{
		{"POST", "/servers/BAD_NAME/rcon"},
		{"POST", "/servers/BAD_NAME/rcon/op"},
		{"POST", "/servers/BAD_NAME/rcon/deop"},
		{"POST", "/servers/BAD_NAME/rcon/whitelist"},
		{"GET", "/servers/BAD_NAME/rcon/players"},
	}

	for _, ep := range endpoints {
		t.Run(ep.method+" "+ep.path, func(t *testing.T) {
			rr := doRequest(handler, ep.method, ep.path, nil)
			if rr.Code != http.StatusBadRequest {
				t.Errorf("status = %d, want 400 for %s %s", rr.Code, ep.method, ep.path)
			}
		})
	}
}

// --- RCON error paths ---

func TestRCONExecute_NotFoundError(t *testing.T) {
	s := newTestServer(&mockRCON{
		err: fmt.Errorf("resolve rcon endpoint: no running allocation with rcon port found for mc-test"),
	})
	body := map[string]string{"command": "say hello"}
	rr := doRequest(s.Handler(), "POST", "/servers/mc-test/rcon", body)

	if rr.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404", rr.Code)
	}
}

func TestRCONExecute_UpstreamError(t *testing.T) {
	s := newTestServer(&mockRCON{
		err: fmt.Errorf("rcon connect to 10.0.0.1:25575: connection refused"),
	})
	body := map[string]string{"command": "say hello"}
	rr := doRequest(s.Handler(), "POST", "/servers/mc-test/rcon", body)

	if rr.Code != http.StatusBadGateway {
		t.Errorf("status = %d, want 502", rr.Code)
	}
}

func TestRCONExecute_InvalidServerName(t *testing.T) {
	s := newTestServer(&mockRCON{})
	body := map[string]string{"command": "say hello"}
	rr := doRequest(s.Handler(), "POST", "/servers/INVALID!/rcon", body)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", rr.Code)
	}
}

func TestRCONExecute_InvalidBody(t *testing.T) {
	s := newTestServer(&mockRCON{})
	// Send invalid JSON
	req := httptest.NewRequest("POST", "/servers/mc-test/rcon", bytes.NewBufferString("not json"))
	req.Header.Set("Authorization", "Bearer test-api-key")
	rr := httptest.NewRecorder()
	s.Handler().ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", rr.Code)
	}
}

// --- RCON Op error paths ---

func TestRCONOp_UpstreamError(t *testing.T) {
	s := newTestServer(&mockRCON{err: fmt.Errorf("connection failed")})
	body := map[string]string{"player": "Steve"}
	rr := doRequest(s.Handler(), "POST", "/servers/mc-test/rcon/op", body)

	if rr.Code != http.StatusBadGateway {
		t.Errorf("status = %d, want 502", rr.Code)
	}
}

func TestRCONOp_MissingPlayer(t *testing.T) {
	s := newTestServer(&mockRCON{})
	body := map[string]string{"player": ""}
	rr := doRequest(s.Handler(), "POST", "/servers/mc-test/rcon/op", body)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", rr.Code)
	}
}

// --- RCON Deop error paths ---

func TestRCONDeop_UpstreamError(t *testing.T) {
	s := newTestServer(&mockRCON{err: fmt.Errorf("connection failed")})
	body := map[string]string{"player": "Steve"}
	rr := doRequest(s.Handler(), "POST", "/servers/mc-test/rcon/deop", body)

	if rr.Code != http.StatusBadGateway {
		t.Errorf("status = %d, want 502", rr.Code)
	}
}

func TestRCONDeop_InvalidPlayer(t *testing.T) {
	s := newTestServer(&mockRCON{})
	body := map[string]string{"player": "invalid player!"}
	rr := doRequest(s.Handler(), "POST", "/servers/mc-test/rcon/deop", body)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", rr.Code)
	}
}

// --- RCON Whitelist error paths ---

func TestRCONWhitelist_UpstreamError(t *testing.T) {
	s := newTestServer(&mockRCON{err: fmt.Errorf("connection failed")})
	body := map[string]any{"action": "add", "player": "Steve"}
	rr := doRequest(s.Handler(), "POST", "/servers/mc-test/rcon/whitelist", body)

	if rr.Code != http.StatusBadGateway {
		t.Errorf("status = %d, want 502", rr.Code)
	}
}

func TestRCONWhitelist_InvalidPlayer(t *testing.T) {
	s := newTestServer(&mockRCON{})
	body := map[string]any{"action": "add", "player": "bad name!"}
	rr := doRequest(s.Handler(), "POST", "/servers/mc-test/rcon/whitelist", body)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", rr.Code)
	}
}

func TestRCONWhitelist_RemoveAction(t *testing.T) {
	s := newTestServer(&mockRCON{response: "Removed Steve from the whitelist"})
	body := map[string]any{"action": "remove", "player": "Steve"}
	rr := doRequest(s.Handler(), "POST", "/servers/mc-test/rcon/whitelist", body)

	if rr.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", rr.Code)
	}
}

// --- RCON Players error paths ---

func TestRCONPlayers_NotFoundError(t *testing.T) {
	s := newTestServer(&mockRCON{
		err: fmt.Errorf("resolve rcon endpoint: no running allocation with rcon port found for mc-test"),
	})
	rr := doRequest(s.Handler(), "GET", "/servers/mc-test/rcon/players", nil)

	if rr.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404", rr.Code)
	}
}

func TestRCONPlayers_UpstreamError(t *testing.T) {
	s := newTestServer(&mockRCON{err: fmt.Errorf("connection refused")})
	rr := doRequest(s.Handler(), "GET", "/servers/mc-test/rcon/players", nil)

	if rr.Code != http.StatusBadGateway {
		t.Errorf("status = %d, want 502", rr.Code)
	}
}

func TestRCONPlayers_InvalidServerName(t *testing.T) {
	s := newTestServer(&mockRCON{})
	rr := doRequest(s.Handler(), "GET", "/servers/INVALID!/rcon/players", nil)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", rr.Code)
	}
}
