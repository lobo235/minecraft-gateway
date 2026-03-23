package api

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/lobo235/minecraft-gateway/internal/nfs"
)

// --- Mock NFS client ---

type mockNFS struct {
	servers      []string
	listErr      error
	createErr    error
	deleteErr    error
	diskUsage    int64
	diskUsageErr error
	files        []nfs.FileEntry
	listFilesErr error
	fileContent  []byte
	readFileErr  error
	grepResult   *nfs.GrepResult
	grepErr      error
	backups      []nfs.BackupInfo
	listBackErr  error
	backupID     string
	startBackErr error
	backupStatus *nfs.BackupStatus
	getStatusErr error
	restoreErr   error
	migrateErr   error
}

func (m *mockNFS) SafePath(parts ...string) (string, error) { return "", nil }
func (m *mockNFS) ListServers() ([]string, error)           { return m.servers, m.listErr }
func (m *mockNFS) CreateServer(string, int, int) error      { return m.createErr }
func (m *mockNFS) DeleteServer(string) error                { return m.deleteErr }
func (m *mockNFS) DiskUsage(string) (int64, error)          { return m.diskUsage, m.diskUsageErr }
func (m *mockNFS) ListFiles(string, string) ([]nfs.FileEntry, error) {
	return m.files, m.listFilesErr
}
func (m *mockNFS) ReadFile(string, string) ([]byte, error) { return m.fileContent, m.readFileErr }
func (m *mockNFS) GrepFiles(string, string, string) (*nfs.GrepResult, error) {
	return m.grepResult, m.grepErr
}
func (m *mockNFS) ListBackups(string) ([]nfs.BackupInfo, error) { return m.backups, m.listBackErr }
func (m *mockNFS) StartBackup(string) (string, error)           { return m.backupID, m.startBackErr }
func (m *mockNFS) GetBackupStatus(string, string) (*nfs.BackupStatus, error) {
	return m.backupStatus, m.getStatusErr
}
func (m *mockNFS) Restore(string, string) error { return m.restoreErr }
func (m *mockNFS) Migrate(string, string) error { return m.migrateErr }

// --- Mock RCON client ---

type mockRCON struct {
	response string
	err      error
}

func (m *mockRCON) Execute(string, string) (string, error) { return m.response, m.err }

// --- Helpers ---

func newTestServer(nfsClient nfs.Client, rconClient *mockRCON) *Server {
	log := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelError}))
	return NewServer(nfsClient, rconClient, "test-api-key", "test", log)
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
	s := newTestServer(&mockNFS{}, &mockRCON{})
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
	s := newTestServer(&mockNFS{servers: []string{"a"}}, &mockRCON{})
	rr := doRequestNoAuth(s.Handler(), "GET", "/servers")

	if rr.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", rr.Code)
	}
}

// --- List Servers ---

func TestListServers(t *testing.T) {
	s := newTestServer(&mockNFS{servers: []string{"mc-1", "mc-2"}}, &mockRCON{})
	rr := doRequest(s.Handler(), "GET", "/servers", nil)

	if rr.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", rr.Code)
	}
	var resp map[string][]string
	json.NewDecoder(rr.Body).Decode(&resp)
	if len(resp["servers"]) != 2 {
		t.Errorf("expected 2 servers, got %d", len(resp["servers"]))
	}
}

// --- Create Server ---

func TestCreateServer(t *testing.T) {
	s := newTestServer(&mockNFS{}, &mockRCON{})
	body := map[string]any{"name": "mc-new", "uid": 1000, "gid": 1000}
	rr := doRequest(s.Handler(), "POST", "/servers", body)

	if rr.Code != http.StatusCreated {
		t.Errorf("status = %d, want 201", rr.Code)
	}
}

func TestCreateServerInvalidName(t *testing.T) {
	s := newTestServer(&mockNFS{}, &mockRCON{})
	body := map[string]any{"name": "INVALID_NAME!", "uid": 1000, "gid": 1000}
	rr := doRequest(s.Handler(), "POST", "/servers", body)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", rr.Code)
	}
}

func TestCreateServerMissingName(t *testing.T) {
	s := newTestServer(&mockNFS{}, &mockRCON{})
	body := map[string]any{"uid": 1000, "gid": 1000}
	rr := doRequest(s.Handler(), "POST", "/servers", body)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", rr.Code)
	}
}

// --- Delete Server ---

func TestDeleteServer(t *testing.T) {
	s := newTestServer(&mockNFS{}, &mockRCON{})
	rr := doRequest(s.Handler(), "DELETE", "/servers/mc-test?confirm=true", nil)

	if rr.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", rr.Code)
	}
}

func TestDeleteServerMissingConfirm(t *testing.T) {
	s := newTestServer(&mockNFS{}, &mockRCON{})
	rr := doRequest(s.Handler(), "DELETE", "/servers/mc-test", nil)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", rr.Code)
	}
}

// --- Disk Usage ---

func TestDiskUsage(t *testing.T) {
	s := newTestServer(&mockNFS{diskUsage: 1024000}, &mockRCON{})
	rr := doRequest(s.Handler(), "GET", "/servers/mc-test/disk-usage", nil)

	if rr.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", rr.Code)
	}
}

// --- List Files ---

func TestListFiles(t *testing.T) {
	files := []nfs.FileEntry{
		{Name: "world", IsDir: true, Size: 4096},
	}
	s := newTestServer(&mockNFS{files: files}, &mockRCON{})
	rr := doRequest(s.Handler(), "GET", "/servers/mc-test/files?path=.", nil)

	if rr.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", rr.Code)
	}
}

// --- Read File ---

func TestReadFile(t *testing.T) {
	s := newTestServer(&mockNFS{fileContent: []byte("log data")}, &mockRCON{})
	rr := doRequest(s.Handler(), "GET", "/servers/mc-test/files/read?path=logs/latest.log", nil)

	if rr.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", rr.Code)
	}
}

func TestReadFileMissingPath(t *testing.T) {
	s := newTestServer(&mockNFS{}, &mockRCON{})
	rr := doRequest(s.Handler(), "GET", "/servers/mc-test/files/read", nil)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", rr.Code)
	}
}

// --- Grep ---

func TestGrepFiles(t *testing.T) {
	result := &nfs.GrepResult{Lines: []string{"match1"}, Count: 1}
	s := newTestServer(&mockNFS{grepResult: result}, &mockRCON{})
	rr := doRequest(s.Handler(), "GET", "/servers/mc-test/files/grep?path=logs&pattern=ERROR", nil)

	if rr.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", rr.Code)
	}
}

func TestGrepFilesMissingPattern(t *testing.T) {
	s := newTestServer(&mockNFS{}, &mockRCON{})
	rr := doRequest(s.Handler(), "GET", "/servers/mc-test/files/grep?path=logs", nil)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", rr.Code)
	}
}

// --- Backups ---

func TestListBackups(t *testing.T) {
	backups := []nfs.BackupInfo{{ID: "2026-03-22T10-00-00", Size: 1024}}
	s := newTestServer(&mockNFS{backups: backups}, &mockRCON{})
	rr := doRequest(s.Handler(), "GET", "/servers/mc-test/backups", nil)

	if rr.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", rr.Code)
	}
}

func TestStartBackup(t *testing.T) {
	s := newTestServer(&mockNFS{backupID: "2026-03-22T10-00-00"}, &mockRCON{})
	rr := doRequest(s.Handler(), "POST", "/servers/mc-test/backups", nil)

	if rr.Code != http.StatusAccepted {
		t.Errorf("status = %d, want 202", rr.Code)
	}
}

func TestGetBackupStatus(t *testing.T) {
	status := &nfs.BackupStatus{Server: "mc-test", ID: "abc", Status: "done"}
	s := newTestServer(&mockNFS{backupStatus: status}, &mockRCON{})
	rr := doRequest(s.Handler(), "GET", "/servers/mc-test/backups/abc", nil)

	if rr.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", rr.Code)
	}
}

// --- Restore ---

func TestRestore(t *testing.T) {
	s := newTestServer(&mockNFS{}, &mockRCON{})
	body := map[string]string{"backup_id": "2026-03-22T10-00-00"}
	rr := doRequest(s.Handler(), "POST", "/servers/mc-test/restore", body)

	if rr.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", rr.Code)
	}
}

func TestRestoreMissingBackupID(t *testing.T) {
	s := newTestServer(&mockNFS{}, &mockRCON{})
	body := map[string]string{}
	rr := doRequest(s.Handler(), "POST", "/servers/mc-test/restore", body)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", rr.Code)
	}
}

// --- Migrate ---

func TestMigrate(t *testing.T) {
	s := newTestServer(&mockNFS{}, &mockRCON{})
	body := map[string]string{"new_name": "mc-new"}
	rr := doRequest(s.Handler(), "POST", "/servers/mc-test/migrate", body)

	if rr.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", rr.Code)
	}
}

func TestMigrateInvalidNewName(t *testing.T) {
	s := newTestServer(&mockNFS{}, &mockRCON{})
	body := map[string]string{"new_name": "INVALID!"}
	rr := doRequest(s.Handler(), "POST", "/servers/mc-test/migrate", body)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", rr.Code)
	}
}

// --- RCON ---

func TestRCONExecute(t *testing.T) {
	s := newTestServer(&mockNFS{}, &mockRCON{response: "Done!"})
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
	s := newTestServer(&mockNFS{}, &mockRCON{})
	body := map[string]string{}
	rr := doRequest(s.Handler(), "POST", "/servers/mc-test/rcon", body)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", rr.Code)
	}
}

func TestRCONOp(t *testing.T) {
	s := newTestServer(&mockNFS{}, &mockRCON{response: "Made Steve a server operator"})
	body := map[string]string{"player": "Steve"}
	rr := doRequest(s.Handler(), "POST", "/servers/mc-test/rcon/op", body)

	if rr.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", rr.Code)
	}
}

func TestRCONOpInvalidPlayer(t *testing.T) {
	s := newTestServer(&mockNFS{}, &mockRCON{})
	body := map[string]string{"player": "invalid player name!"}
	rr := doRequest(s.Handler(), "POST", "/servers/mc-test/rcon/op", body)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", rr.Code)
	}
}

func TestRCONDeop(t *testing.T) {
	s := newTestServer(&mockNFS{}, &mockRCON{response: "Made Steve no longer a server operator"})
	body := map[string]string{"player": "Steve"}
	rr := doRequest(s.Handler(), "POST", "/servers/mc-test/rcon/deop", body)

	if rr.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", rr.Code)
	}
}

func TestRCONWhitelist(t *testing.T) {
	s := newTestServer(&mockNFS{}, &mockRCON{response: "Added Steve to the whitelist"})
	body := map[string]any{"action": "add", "player": "Steve"}
	rr := doRequest(s.Handler(), "POST", "/servers/mc-test/rcon/whitelist", body)

	if rr.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", rr.Code)
	}
}

func TestRCONWhitelistInvalidAction(t *testing.T) {
	s := newTestServer(&mockNFS{}, &mockRCON{})
	body := map[string]any{"action": "invalid", "player": "Steve"}
	rr := doRequest(s.Handler(), "POST", "/servers/mc-test/rcon/whitelist", body)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", rr.Code)
	}
}

func TestRCONPlayers(t *testing.T) {
	s := newTestServer(&mockNFS{}, &mockRCON{response: "There are 2 of a max of 20 players online: Steve, Alex"})
	rr := doRequest(s.Handler(), "GET", "/servers/mc-test/rcon/players", nil)

	if rr.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", rr.Code)
	}
}

// --- Server name validation across endpoints ---

func TestInvalidServerNameRejected(t *testing.T) {
	s := newTestServer(&mockNFS{}, &mockRCON{})
	handler := s.Handler()

	endpoints := []struct {
		method string
		path   string
	}{
		{"DELETE", "/servers/BAD_NAME?confirm=true"},
		{"GET", "/servers/BAD_NAME/disk-usage"},
		{"GET", "/servers/BAD_NAME/files"},
		{"GET", "/servers/BAD_NAME/files/read?path=x"},
		{"GET", "/servers/BAD_NAME/files/grep?path=x&pattern=y"},
		{"GET", "/servers/BAD_NAME/backups"},
		{"POST", "/servers/BAD_NAME/backups"},
		{"GET", "/servers/BAD_NAME/backups/id"},
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

// --- Path traversal through handler ---

func TestPathTraversalInHandler(t *testing.T) {
	mn := &mockNFS{
		readFileErr: nfs.ErrPathTraversal,
	}
	s := newTestServer(mn, &mockRCON{})
	rr := doRequest(s.Handler(), "GET", "/servers/mc-test/files/read?path=../../etc/passwd", nil)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", rr.Code)
	}
	var resp errorResponse
	json.NewDecoder(rr.Body).Decode(&resp)
	if resp.Code != "path_traversal" {
		t.Errorf("code = %q, want path_traversal", resp.Code)
	}
}
