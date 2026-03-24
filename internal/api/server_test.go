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

	"github.com/lobo235/minecraft-gateway/internal/nfs"
)

// --- Mock NFS client ---

type mockNFS struct {
	servers          []string
	listErr          error
	createErr        error
	deleteErr        error
	diskUsage        int64
	diskUsageErr     error
	files            []nfs.FileEntry
	listFilesErr     error
	fileContent      []byte
	readFileErr      error
	grepResult       *nfs.GrepResult
	grepErr          error
	backups          []nfs.BackupInfo
	listBackErr      error
	backupID         string
	startBackErr     error
	backupStatus     *nfs.BackupStatus
	getStatusErr     error
	restoreErr       error
	migrateErr       error
	downloadID       string
	startDownloadErr error
	downloadStatus   *nfs.DownloadStatus
	getDownloadErr   error
	archiveEntries   []nfs.ArchiveEntry
	archiveErr       error
	writeFileErr     error
	moveFileErr      error
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
func (m *mockNFS) StartDownload(string, string, string, bool, int, int, nfs.DownloadMode) (string, error) {
	return m.downloadID, m.startDownloadErr
}
func (m *mockNFS) GetDownloadStatus(string, string) (*nfs.DownloadStatus, error) {
	return m.downloadStatus, m.getDownloadErr
}
func (m *mockNFS) ListArchiveContents(string, string) ([]nfs.ArchiveEntry, error) {
	return m.archiveEntries, m.archiveErr
}
func (m *mockNFS) WriteFile(string, string, string, int, int) error {
	return m.writeFileErr
}
func (m *mockNFS) MoveFile(string, string, string) error {
	return m.moveFileErr
}
func (m *mockNFS) MaxWriteFileSize() int64 { return 1048576 }

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

// --- RCON error paths ---

func TestRCONExecute_NotFoundError(t *testing.T) {
	s := newTestServer(&mockNFS{}, &mockRCON{
		err: fmt.Errorf("resolve rcon endpoint: no running allocation with rcon port found for mc-test"),
	})
	body := map[string]string{"command": "say hello"}
	rr := doRequest(s.Handler(), "POST", "/servers/mc-test/rcon", body)

	if rr.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404", rr.Code)
	}
}

func TestRCONExecute_UpstreamError(t *testing.T) {
	s := newTestServer(&mockNFS{}, &mockRCON{
		err: fmt.Errorf("rcon connect to 10.0.0.1:25575: connection refused"),
	})
	body := map[string]string{"command": "say hello"}
	rr := doRequest(s.Handler(), "POST", "/servers/mc-test/rcon", body)

	if rr.Code != http.StatusBadGateway {
		t.Errorf("status = %d, want 502", rr.Code)
	}
}

func TestRCONExecute_InvalidServerName(t *testing.T) {
	s := newTestServer(&mockNFS{}, &mockRCON{})
	body := map[string]string{"command": "say hello"}
	rr := doRequest(s.Handler(), "POST", "/servers/INVALID!/rcon", body)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", rr.Code)
	}
}

func TestRCONExecute_InvalidBody(t *testing.T) {
	s := newTestServer(&mockNFS{}, &mockRCON{})
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
	s := newTestServer(&mockNFS{}, &mockRCON{err: fmt.Errorf("connection failed")})
	body := map[string]string{"player": "Steve"}
	rr := doRequest(s.Handler(), "POST", "/servers/mc-test/rcon/op", body)

	if rr.Code != http.StatusBadGateway {
		t.Errorf("status = %d, want 502", rr.Code)
	}
}

func TestRCONOp_MissingPlayer(t *testing.T) {
	s := newTestServer(&mockNFS{}, &mockRCON{})
	body := map[string]string{"player": ""}
	rr := doRequest(s.Handler(), "POST", "/servers/mc-test/rcon/op", body)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", rr.Code)
	}
}

// --- RCON Deop error paths ---

func TestRCONDeop_UpstreamError(t *testing.T) {
	s := newTestServer(&mockNFS{}, &mockRCON{err: fmt.Errorf("connection failed")})
	body := map[string]string{"player": "Steve"}
	rr := doRequest(s.Handler(), "POST", "/servers/mc-test/rcon/deop", body)

	if rr.Code != http.StatusBadGateway {
		t.Errorf("status = %d, want 502", rr.Code)
	}
}

func TestRCONDeop_InvalidPlayer(t *testing.T) {
	s := newTestServer(&mockNFS{}, &mockRCON{})
	body := map[string]string{"player": "invalid player!"}
	rr := doRequest(s.Handler(), "POST", "/servers/mc-test/rcon/deop", body)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", rr.Code)
	}
}

// --- RCON Whitelist error paths ---

func TestRCONWhitelist_UpstreamError(t *testing.T) {
	s := newTestServer(&mockNFS{}, &mockRCON{err: fmt.Errorf("connection failed")})
	body := map[string]any{"action": "add", "player": "Steve"}
	rr := doRequest(s.Handler(), "POST", "/servers/mc-test/rcon/whitelist", body)

	if rr.Code != http.StatusBadGateway {
		t.Errorf("status = %d, want 502", rr.Code)
	}
}

func TestRCONWhitelist_InvalidPlayer(t *testing.T) {
	s := newTestServer(&mockNFS{}, &mockRCON{})
	body := map[string]any{"action": "add", "player": "bad name!"}
	rr := doRequest(s.Handler(), "POST", "/servers/mc-test/rcon/whitelist", body)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", rr.Code)
	}
}

func TestRCONWhitelist_RemoveAction(t *testing.T) {
	s := newTestServer(&mockNFS{}, &mockRCON{response: "Removed Steve from the whitelist"})
	body := map[string]any{"action": "remove", "player": "Steve"}
	rr := doRequest(s.Handler(), "POST", "/servers/mc-test/rcon/whitelist", body)

	if rr.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", rr.Code)
	}
}

// --- RCON Players error paths ---

func TestRCONPlayers_NotFoundError(t *testing.T) {
	s := newTestServer(&mockNFS{}, &mockRCON{
		err: fmt.Errorf("resolve rcon endpoint: no running allocation with rcon port found for mc-test"),
	})
	rr := doRequest(s.Handler(), "GET", "/servers/mc-test/rcon/players", nil)

	if rr.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404", rr.Code)
	}
}

func TestRCONPlayers_UpstreamError(t *testing.T) {
	s := newTestServer(&mockNFS{}, &mockRCON{err: fmt.Errorf("connection refused")})
	rr := doRequest(s.Handler(), "GET", "/servers/mc-test/rcon/players", nil)

	if rr.Code != http.StatusBadGateway {
		t.Errorf("status = %d, want 502", rr.Code)
	}
}

func TestRCONPlayers_InvalidServerName(t *testing.T) {
	s := newTestServer(&mockNFS{}, &mockRCON{})
	rr := doRequest(s.Handler(), "GET", "/servers/INVALID!/rcon/players", nil)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", rr.Code)
	}
}

// --- ListServers error path ---

func TestListServers_Error(t *testing.T) {
	s := newTestServer(&mockNFS{listErr: fmt.Errorf("disk error")}, &mockRCON{})
	rr := doRequest(s.Handler(), "GET", "/servers", nil)

	if rr.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want 500", rr.Code)
	}
}

func TestListServers_NilReturnsEmptyArray(t *testing.T) {
	s := newTestServer(&mockNFS{servers: nil}, &mockRCON{})
	rr := doRequest(s.Handler(), "GET", "/servers", nil)

	if rr.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", rr.Code)
	}
	var resp map[string][]string
	json.NewDecoder(rr.Body).Decode(&resp)
	if resp["servers"] == nil {
		t.Error("expected non-nil servers array")
	}
	if len(resp["servers"]) != 0 {
		t.Errorf("expected 0 servers, got %d", len(resp["servers"]))
	}
}

// --- CreateServer error paths ---

func TestCreateServer_PathTraversal(t *testing.T) {
	s := newTestServer(&mockNFS{createErr: nfs.ErrPathTraversal}, &mockRCON{})
	body := map[string]any{"name": "mc-test", "uid": 1000, "gid": 1000}
	rr := doRequest(s.Handler(), "POST", "/servers", body)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", rr.Code)
	}
}

func TestCreateServer_InternalError(t *testing.T) {
	s := newTestServer(&mockNFS{createErr: fmt.Errorf("disk full")}, &mockRCON{})
	body := map[string]any{"name": "mc-test", "uid": 1000, "gid": 1000}
	rr := doRequest(s.Handler(), "POST", "/servers", body)

	if rr.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want 500", rr.Code)
	}
}

func TestCreateServer_InvalidBody(t *testing.T) {
	s := newTestServer(&mockNFS{}, &mockRCON{})
	req := httptest.NewRequest("POST", "/servers", bytes.NewBufferString("not json"))
	req.Header.Set("Authorization", "Bearer test-api-key")
	rr := httptest.NewRecorder()
	s.Handler().ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", rr.Code)
	}
}

// --- DeleteServer error paths ---

func TestDeleteServer_PathTraversal(t *testing.T) {
	s := newTestServer(&mockNFS{deleteErr: nfs.ErrPathTraversal}, &mockRCON{})
	rr := doRequest(s.Handler(), "DELETE", "/servers/mc-test?confirm=true", nil)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", rr.Code)
	}
}

func TestDeleteServer_NotFound(t *testing.T) {
	s := newTestServer(&mockNFS{deleteErr: fmt.Errorf("server not found: mc-gone")}, &mockRCON{})
	rr := doRequest(s.Handler(), "DELETE", "/servers/mc-gone?confirm=true", nil)

	if rr.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404", rr.Code)
	}
}

func TestDeleteServer_InternalError(t *testing.T) {
	s := newTestServer(&mockNFS{deleteErr: fmt.Errorf("permission denied")}, &mockRCON{})
	rr := doRequest(s.Handler(), "DELETE", "/servers/mc-test?confirm=true", nil)

	if rr.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want 500", rr.Code)
	}
}

func TestDeleteServer_InvalidName(t *testing.T) {
	s := newTestServer(&mockNFS{}, &mockRCON{})
	rr := doRequest(s.Handler(), "DELETE", "/servers/INVALID!?confirm=true", nil)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", rr.Code)
	}
}

// --- DiskUsage error paths ---

func TestDiskUsage_PathTraversal(t *testing.T) {
	s := newTestServer(&mockNFS{diskUsageErr: nfs.ErrPathTraversal}, &mockRCON{})
	rr := doRequest(s.Handler(), "GET", "/servers/mc-test/disk-usage", nil)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", rr.Code)
	}
}

func TestDiskUsage_InternalError(t *testing.T) {
	s := newTestServer(&mockNFS{diskUsageErr: fmt.Errorf("du failed")}, &mockRCON{})
	rr := doRequest(s.Handler(), "GET", "/servers/mc-test/disk-usage", nil)

	if rr.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want 500", rr.Code)
	}
}

// --- ListFiles error paths ---

func TestListFiles_PathTraversal(t *testing.T) {
	s := newTestServer(&mockNFS{listFilesErr: nfs.ErrPathTraversal}, &mockRCON{})
	rr := doRequest(s.Handler(), "GET", "/servers/mc-test/files?path=../../etc", nil)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", rr.Code)
	}
}

func TestListFiles_InternalError(t *testing.T) {
	s := newTestServer(&mockNFS{listFilesErr: fmt.Errorf("io error")}, &mockRCON{})
	rr := doRequest(s.Handler(), "GET", "/servers/mc-test/files", nil)

	if rr.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want 500", rr.Code)
	}
}

func TestListFiles_NilReturnsEmptyArray(t *testing.T) {
	s := newTestServer(&mockNFS{files: nil}, &mockRCON{})
	rr := doRequest(s.Handler(), "GET", "/servers/mc-test/files", nil)

	if rr.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", rr.Code)
	}
}

// --- ReadFile error paths ---

func TestReadFile_ExceedsMax(t *testing.T) {
	s := newTestServer(&mockNFS{readFileErr: fmt.Errorf("file size 2000000 exceeds maximum of 1048576 bytes")}, &mockRCON{})
	rr := doRequest(s.Handler(), "GET", "/servers/mc-test/files/read?path=big.bin", nil)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", rr.Code)
	}
}

func TestReadFile_NotFound(t *testing.T) {
	s := newTestServer(&mockNFS{readFileErr: fmt.Errorf("stat file: no such file or directory")}, &mockRCON{})
	rr := doRequest(s.Handler(), "GET", "/servers/mc-test/files/read?path=missing.txt", nil)

	if rr.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404", rr.Code)
	}
}

func TestReadFile_InternalError(t *testing.T) {
	s := newTestServer(&mockNFS{readFileErr: fmt.Errorf("permission denied")}, &mockRCON{})
	rr := doRequest(s.Handler(), "GET", "/servers/mc-test/files/read?path=secret.txt", nil)

	if rr.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want 500", rr.Code)
	}
}

// --- Grep error paths ---

func TestGrepFiles_PathTraversal(t *testing.T) {
	s := newTestServer(&mockNFS{grepErr: nfs.ErrPathTraversal}, &mockRCON{})
	rr := doRequest(s.Handler(), "GET", "/servers/mc-test/files/grep?path=../../etc&pattern=root", nil)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", rr.Code)
	}
}

func TestGrepFiles_InternalError(t *testing.T) {
	s := newTestServer(&mockNFS{grepErr: fmt.Errorf("grep failed")}, &mockRCON{})
	rr := doRequest(s.Handler(), "GET", "/servers/mc-test/files/grep?path=logs&pattern=ERROR", nil)

	if rr.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want 500", rr.Code)
	}
}

// --- ListBackups error paths ---

func TestListBackups_PathTraversal(t *testing.T) {
	s := newTestServer(&mockNFS{listBackErr: nfs.ErrPathTraversal}, &mockRCON{})
	rr := doRequest(s.Handler(), "GET", "/servers/mc-test/backups", nil)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", rr.Code)
	}
}

func TestListBackups_InternalError(t *testing.T) {
	s := newTestServer(&mockNFS{listBackErr: fmt.Errorf("disk error")}, &mockRCON{})
	rr := doRequest(s.Handler(), "GET", "/servers/mc-test/backups", nil)

	if rr.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want 500", rr.Code)
	}
}

func TestListBackups_NilReturnsEmptyArray(t *testing.T) {
	s := newTestServer(&mockNFS{backups: nil}, &mockRCON{})
	rr := doRequest(s.Handler(), "GET", "/servers/mc-test/backups", nil)

	if rr.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", rr.Code)
	}
}

// --- StartBackup error paths ---

func TestStartBackup_PathTraversal(t *testing.T) {
	s := newTestServer(&mockNFS{startBackErr: nfs.ErrPathTraversal}, &mockRCON{})
	rr := doRequest(s.Handler(), "POST", "/servers/mc-test/backups", nil)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", rr.Code)
	}
}

func TestStartBackup_ServerNotFound(t *testing.T) {
	s := newTestServer(&mockNFS{startBackErr: fmt.Errorf("server not found: mc-test")}, &mockRCON{})
	rr := doRequest(s.Handler(), "POST", "/servers/mc-test/backups", nil)

	if rr.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404", rr.Code)
	}
}

func TestStartBackup_InternalError(t *testing.T) {
	s := newTestServer(&mockNFS{startBackErr: fmt.Errorf("disk full")}, &mockRCON{})
	rr := doRequest(s.Handler(), "POST", "/servers/mc-test/backups", nil)

	if rr.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want 500", rr.Code)
	}
}

// --- GetBackupStatus error paths ---

func TestGetBackupStatus_NotFound(t *testing.T) {
	s := newTestServer(&mockNFS{getStatusErr: fmt.Errorf("backup status not found")}, &mockRCON{})
	rr := doRequest(s.Handler(), "GET", "/servers/mc-test/backups/fake-id", nil)

	if rr.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404", rr.Code)
	}
}

func TestGetBackupStatus_InternalError(t *testing.T) {
	s := newTestServer(&mockNFS{getStatusErr: fmt.Errorf("disk error")}, &mockRCON{})
	rr := doRequest(s.Handler(), "GET", "/servers/mc-test/backups/some-id", nil)

	if rr.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want 500", rr.Code)
	}
}

// --- Restore error paths ---

func TestRestore_PathTraversal(t *testing.T) {
	s := newTestServer(&mockNFS{restoreErr: nfs.ErrPathTraversal}, &mockRCON{})
	body := map[string]string{"backup_id": "2026-03-22T10-00-00"}
	rr := doRequest(s.Handler(), "POST", "/servers/mc-test/restore", body)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", rr.Code)
	}
}

func TestRestore_BackupNotFound(t *testing.T) {
	s := newTestServer(&mockNFS{restoreErr: fmt.Errorf("backup not found: fake-id")}, &mockRCON{})
	body := map[string]string{"backup_id": "fake-id"}
	rr := doRequest(s.Handler(), "POST", "/servers/mc-test/restore", body)

	if rr.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404", rr.Code)
	}
}

func TestRestore_InternalError(t *testing.T) {
	s := newTestServer(&mockNFS{restoreErr: fmt.Errorf("tar extract failed")}, &mockRCON{})
	body := map[string]string{"backup_id": "2026-03-22T10-00-00"}
	rr := doRequest(s.Handler(), "POST", "/servers/mc-test/restore", body)

	if rr.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want 500", rr.Code)
	}
}

func TestRestore_InvalidBody(t *testing.T) {
	s := newTestServer(&mockNFS{}, &mockRCON{})
	req := httptest.NewRequest("POST", "/servers/mc-test/restore", bytes.NewBufferString("not json"))
	req.Header.Set("Authorization", "Bearer test-api-key")
	rr := httptest.NewRecorder()
	s.Handler().ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", rr.Code)
	}
}

// --- Migrate error paths ---

func TestMigrate_PathTraversal(t *testing.T) {
	s := newTestServer(&mockNFS{migrateErr: nfs.ErrPathTraversal}, &mockRCON{})
	body := map[string]string{"new_name": "mc-new"}
	rr := doRequest(s.Handler(), "POST", "/servers/mc-test/migrate", body)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", rr.Code)
	}
}

func TestMigrate_NotFound(t *testing.T) {
	s := newTestServer(&mockNFS{migrateErr: fmt.Errorf("server not found: mc-test")}, &mockRCON{})
	body := map[string]string{"new_name": "mc-new"}
	rr := doRequest(s.Handler(), "POST", "/servers/mc-test/migrate", body)

	if rr.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404", rr.Code)
	}
}

func TestMigrate_Conflict(t *testing.T) {
	s := newTestServer(&mockNFS{migrateErr: fmt.Errorf("target server already exists: mc-new")}, &mockRCON{})
	body := map[string]string{"new_name": "mc-new"}
	rr := doRequest(s.Handler(), "POST", "/servers/mc-test/migrate", body)

	if rr.Code != http.StatusConflict {
		t.Errorf("status = %d, want 409", rr.Code)
	}
}

func TestMigrate_InternalError(t *testing.T) {
	s := newTestServer(&mockNFS{migrateErr: fmt.Errorf("rename failed")}, &mockRCON{})
	body := map[string]string{"new_name": "mc-new"}
	rr := doRequest(s.Handler(), "POST", "/servers/mc-test/migrate", body)

	if rr.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want 500", rr.Code)
	}
}

func TestMigrate_MissingNewName(t *testing.T) {
	s := newTestServer(&mockNFS{}, &mockRCON{})
	body := map[string]string{}
	rr := doRequest(s.Handler(), "POST", "/servers/mc-test/migrate", body)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", rr.Code)
	}
}

func TestMigrate_InvalidBody(t *testing.T) {
	s := newTestServer(&mockNFS{}, &mockRCON{})
	req := httptest.NewRequest("POST", "/servers/mc-test/migrate", bytes.NewBufferString("not json"))
	req.Header.Set("Authorization", "Bearer test-api-key")
	rr := httptest.NewRecorder()
	s.Handler().ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", rr.Code)
	}
}

// --- Auth edge cases ---

func TestWrongAPIKey(t *testing.T) {
	s := newTestServer(&mockNFS{}, &mockRCON{})
	req := httptest.NewRequest("GET", "/servers", nil)
	req.Header.Set("Authorization", "Bearer wrong-key")
	rr := httptest.NewRecorder()
	s.Handler().ServeHTTP(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", rr.Code)
	}
}

func TestNoBearerPrefix(t *testing.T) {
	s := newTestServer(&mockNFS{}, &mockRCON{})
	req := httptest.NewRequest("GET", "/servers", nil)
	req.Header.Set("Authorization", "test-api-key")
	rr := httptest.NewRecorder()
	s.Handler().ServeHTTP(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", rr.Code)
	}
}

// --- Download handler tests ---

func TestDownloadHandler_Success(t *testing.T) {
	mock := &mockNFS{
		downloadID: "2026-03-24T10-00-00",
	}
	s := newTestServer(mock, &mockRCON{})
	body := map[string]any{
		"url":       "https://edge.forgecdn.net/files/test.zip",
		"dest_path": "mods",
		"extract":   true,
		"uid":       1001,
		"gid":       1001,
	}
	rr := doRequest(s.Handler(), "POST", "/servers/myserver/download", body)
	if rr.Code != http.StatusCreated {
		t.Errorf("status = %d, want 201", rr.Code)
	}

	var resp map[string]string
	json.Unmarshal(rr.Body.Bytes(), &resp)
	if resp["status"] != "running" {
		t.Errorf("status = %v, want running", resp["status"])
	}
	if resp["id"] != "2026-03-24T10-00-00" {
		t.Errorf("id = %v, want 2026-03-24T10-00-00", resp["id"])
	}
}

func TestDownloadHandler_InvalidServerName(t *testing.T) {
	s := newTestServer(&mockNFS{}, &mockRCON{})
	body := map[string]any{"url": "https://edge.forgecdn.net/files/test.zip"}
	rr := doRequest(s.Handler(), "POST", "/servers/INVALID/download", body)
	if rr.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", rr.Code)
	}
}

func TestDownloadHandler_MissingURL(t *testing.T) {
	s := newTestServer(&mockNFS{}, &mockRCON{})
	body := map[string]any{"dest_path": "mods"}
	rr := doRequest(s.Handler(), "POST", "/servers/myserver/download", body)
	if rr.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", rr.Code)
	}
}

func TestDownloadHandler_DisallowedURL(t *testing.T) {
	s := newTestServer(&mockNFS{}, &mockRCON{})
	body := map[string]any{"url": "https://evil.com/malware.zip"}
	rr := doRequest(s.Handler(), "POST", "/servers/myserver/download", body)
	if rr.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", rr.Code)
	}
	var resp map[string]string
	json.Unmarshal(rr.Body.Bytes(), &resp)
	if resp["code"] != "invalid_body" {
		t.Errorf("code = %q, want invalid_body", resp["code"])
	}
}

func TestDownloadHandler_PathTraversal(t *testing.T) {
	mock := &mockNFS{
		startDownloadErr: nfs.ErrPathTraversal,
	}
	s := newTestServer(mock, &mockRCON{})
	body := map[string]any{
		"url":       "https://edge.forgecdn.net/files/test.zip",
		"dest_path": "../../etc",
	}
	rr := doRequest(s.Handler(), "POST", "/servers/myserver/download", body)
	if rr.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", rr.Code)
	}
}

func TestDownloadHandler_InternalError(t *testing.T) {
	mock := &mockNFS{
		startDownloadErr: fmt.Errorf("download failed"),
	}
	s := newTestServer(mock, &mockRCON{})
	body := map[string]any{
		"url": "https://edge.forgecdn.net/files/test.zip",
	}
	rr := doRequest(s.Handler(), "POST", "/servers/myserver/download", body)
	if rr.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want 500", rr.Code)
	}
}

func TestDownloadHandler_InvalidJSON(t *testing.T) {
	s := newTestServer(&mockNFS{}, &mockRCON{})
	req := httptest.NewRequest("POST", "/servers/myserver/download", bytes.NewBufferString("not json"))
	req.Header.Set("Authorization", "Bearer test-api-key")
	rr := httptest.NewRecorder()
	s.Handler().ServeHTTP(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", rr.Code)
	}
}

func TestDownloadHandler_NoAuth(t *testing.T) {
	s := newTestServer(&mockNFS{}, &mockRCON{})
	rr := doRequestNoAuth(s.Handler(), "POST", "/servers/myserver/download")
	if rr.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", rr.Code)
	}
}

func TestDownloadHandler_InvalidMode(t *testing.T) {
	s := newTestServer(&mockNFS{}, &mockRCON{})
	body := map[string]any{
		"url":  "https://edge.forgecdn.net/files/test.zip",
		"mode": "invalid_mode",
	}
	rr := doRequest(s.Handler(), "POST", "/servers/myserver/download", body)
	if rr.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", rr.Code)
	}
}

func TestDownloadHandler_SkipExistingMode(t *testing.T) {
	mock := &mockNFS{
		downloadID: "2026-03-24T10-00-00",
	}
	s := newTestServer(mock, &mockRCON{})
	body := map[string]any{
		"url":  "https://edge.forgecdn.net/files/test.zip",
		"mode": "skip_existing",
	}
	rr := doRequest(s.Handler(), "POST", "/servers/myserver/download", body)
	if rr.Code != http.StatusCreated {
		t.Errorf("status = %d, want 201", rr.Code)
	}
}

func TestDownloadHandler_CleanFirstMode(t *testing.T) {
	mock := &mockNFS{
		downloadID: "2026-03-24T10-00-00",
	}
	s := newTestServer(mock, &mockRCON{})
	body := map[string]any{
		"url":  "https://edge.forgecdn.net/files/test.zip",
		"mode": "clean_first",
	}
	rr := doRequest(s.Handler(), "POST", "/servers/myserver/download", body)
	if rr.Code != http.StatusCreated {
		t.Errorf("status = %d, want 201", rr.Code)
	}
}

// --- Download status handler tests ---

func TestGetDownloadStatusHandler_Success(t *testing.T) {
	mock := &mockNFS{
		downloadStatus: &nfs.DownloadStatus{
			ID:        "2026-03-24T10-00-00",
			Status:    "done",
			URL:       "https://edge.forgecdn.net/files/test.zip",
			DestPath:  "mods",
			Extract:   true,
			StartedAt: "2026-03-24T10:00:00Z",
			Result:    &nfs.DownloadResult{FilesCount: 3, TotalBytes: 12345},
		},
	}
	s := newTestServer(mock, &mockRCON{})
	rr := doRequest(s.Handler(), "GET", "/servers/myserver/downloads/2026-03-24T10-00-00", nil)
	if rr.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", rr.Code)
	}
	var resp nfs.DownloadStatus
	json.Unmarshal(rr.Body.Bytes(), &resp)
	if resp.Status != "done" {
		t.Errorf("status = %q, want done", resp.Status)
	}
	if resp.Result == nil || resp.Result.FilesCount != 3 {
		t.Errorf("result.files_count = %v, want 3", resp.Result)
	}
}

func TestGetDownloadStatusHandler_NotFound(t *testing.T) {
	mock := &mockNFS{
		getDownloadErr: fmt.Errorf("download status not found"),
	}
	s := newTestServer(mock, &mockRCON{})
	rr := doRequest(s.Handler(), "GET", "/servers/myserver/downloads/2026-03-24T10-00-00", nil)
	if rr.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404", rr.Code)
	}
}

func TestGetDownloadStatusHandler_InvalidID(t *testing.T) {
	mock := &mockNFS{
		getDownloadErr: fmt.Errorf("invalid download ID"),
	}
	s := newTestServer(mock, &mockRCON{})
	rr := doRequest(s.Handler(), "GET", "/servers/myserver/downloads/bad!id@here", nil)
	if rr.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404", rr.Code)
	}
}

func TestGetDownloadStatusHandler_InvalidServerName(t *testing.T) {
	s := newTestServer(&mockNFS{}, &mockRCON{})
	rr := doRequest(s.Handler(), "GET", "/servers/INVALID/downloads/2026-03-24T10-00-00", nil)
	if rr.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", rr.Code)
	}
}

func TestGetDownloadStatusHandler_NoAuth(t *testing.T) {
	s := newTestServer(&mockNFS{}, &mockRCON{})
	rr := doRequestNoAuth(s.Handler(), "GET", "/servers/myserver/downloads/2026-03-24T10-00-00")
	if rr.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", rr.Code)
	}
}

// --- Archive Contents handler tests ---

func TestArchiveContentsHandler_Success(t *testing.T) {
	entries := []nfs.ArchiveEntry{
		{Name: "file1.txt", Size: 100, IsDir: false},
		{Name: "subdir/", Size: 0, IsDir: true},
	}
	s := newTestServer(&mockNFS{archiveEntries: entries}, &mockRCON{})
	rr := doRequest(s.Handler(), "GET", "/servers/myserver/archive-contents?path=mods.zip", nil)
	if rr.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", rr.Code)
	}
	var resp map[string][]nfs.ArchiveEntry
	json.NewDecoder(rr.Body).Decode(&resp)
	if len(resp["entries"]) != 2 {
		t.Errorf("expected 2 entries, got %d", len(resp["entries"]))
	}
}

func TestArchiveContentsHandler_MissingPath(t *testing.T) {
	s := newTestServer(&mockNFS{}, &mockRCON{})
	rr := doRequest(s.Handler(), "GET", "/servers/myserver/archive-contents", nil)
	if rr.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", rr.Code)
	}
}

func TestArchiveContentsHandler_InvalidServerName(t *testing.T) {
	s := newTestServer(&mockNFS{}, &mockRCON{})
	rr := doRequest(s.Handler(), "GET", "/servers/INVALID/archive-contents?path=test.zip", nil)
	if rr.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", rr.Code)
	}
}

func TestArchiveContentsHandler_PathTraversal(t *testing.T) {
	s := newTestServer(&mockNFS{archiveErr: nfs.ErrPathTraversal}, &mockRCON{})
	rr := doRequest(s.Handler(), "GET", "/servers/myserver/archive-contents?path=../../etc/passwd", nil)
	if rr.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", rr.Code)
	}
}

func TestArchiveContentsHandler_NotFound(t *testing.T) {
	s := newTestServer(&mockNFS{archiveErr: fmt.Errorf("open zip: no such file or directory")}, &mockRCON{})
	rr := doRequest(s.Handler(), "GET", "/servers/myserver/archive-contents?path=missing.zip", nil)
	if rr.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404", rr.Code)
	}
}

func TestArchiveContentsHandler_InternalError(t *testing.T) {
	s := newTestServer(&mockNFS{archiveErr: fmt.Errorf("corrupt archive")}, &mockRCON{})
	rr := doRequest(s.Handler(), "GET", "/servers/myserver/archive-contents?path=bad.zip", nil)
	if rr.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want 500", rr.Code)
	}
}

func TestArchiveContentsHandler_NilReturnsEmptyArray(t *testing.T) {
	s := newTestServer(&mockNFS{archiveEntries: nil}, &mockRCON{})
	rr := doRequest(s.Handler(), "GET", "/servers/myserver/archive-contents?path=empty.zip", nil)
	if rr.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", rr.Code)
	}
}

func TestArchiveContentsHandler_NoAuth(t *testing.T) {
	s := newTestServer(&mockNFS{}, &mockRCON{})
	rr := doRequestNoAuth(s.Handler(), "GET", "/servers/myserver/archive-contents?path=test.zip")
	if rr.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", rr.Code)
	}
}

// --- Write File handler tests ---

func TestWriteFileHandler_Success(t *testing.T) {
	s := newTestServer(&mockNFS{}, &mockRCON{})
	body := map[string]any{
		"path":    "server.properties",
		"content": "motd=Hello World",
		"uid":     1001,
		"gid":     1001,
	}
	rr := doRequest(s.Handler(), "POST", "/servers/myserver/files/write", body)
	if rr.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", rr.Code)
	}
	var resp map[string]string
	json.NewDecoder(rr.Body).Decode(&resp)
	if resp["status"] != "ok" {
		t.Errorf("status = %q, want ok", resp["status"])
	}
	if resp["path"] != "server.properties" {
		t.Errorf("path = %q, want server.properties", resp["path"])
	}
}

func TestWriteFileHandler_MissingPath(t *testing.T) {
	s := newTestServer(&mockNFS{}, &mockRCON{})
	body := map[string]any{
		"content": "data",
		"uid":     1001,
		"gid":     1001,
	}
	rr := doRequest(s.Handler(), "POST", "/servers/myserver/files/write", body)
	if rr.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", rr.Code)
	}
}

func TestWriteFileHandler_InvalidServerName(t *testing.T) {
	s := newTestServer(&mockNFS{}, &mockRCON{})
	body := map[string]any{
		"path":    "test.txt",
		"content": "data",
		"uid":     1001,
		"gid":     1001,
	}
	rr := doRequest(s.Handler(), "POST", "/servers/INVALID/files/write", body)
	if rr.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", rr.Code)
	}
}

func TestWriteFileHandler_PathTraversal(t *testing.T) {
	s := newTestServer(&mockNFS{writeFileErr: nfs.ErrPathTraversal}, &mockRCON{})
	body := map[string]any{
		"path":    "../../etc/passwd",
		"content": "data",
		"uid":     1001,
		"gid":     1001,
	}
	rr := doRequest(s.Handler(), "POST", "/servers/myserver/files/write", body)
	if rr.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", rr.Code)
	}
}

func TestWriteFileHandler_ExceedsMax(t *testing.T) {
	s := newTestServer(&mockNFS{writeFileErr: fmt.Errorf("content size 2000000 exceeds maximum of 1048576 bytes")}, &mockRCON{})
	body := map[string]any{
		"path":    "test.txt",
		"content": "data",
		"uid":     1001,
		"gid":     1001,
	}
	rr := doRequest(s.Handler(), "POST", "/servers/myserver/files/write", body)
	if rr.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", rr.Code)
	}
}

func TestWriteFileHandler_InternalError(t *testing.T) {
	s := newTestServer(&mockNFS{writeFileErr: fmt.Errorf("disk full")}, &mockRCON{})
	body := map[string]any{
		"path":    "test.txt",
		"content": "data",
		"uid":     1001,
		"gid":     1001,
	}
	rr := doRequest(s.Handler(), "POST", "/servers/myserver/files/write", body)
	if rr.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want 500", rr.Code)
	}
}

func TestWriteFileHandler_InvalidJSON(t *testing.T) {
	s := newTestServer(&mockNFS{}, &mockRCON{})
	req := httptest.NewRequest("POST", "/servers/myserver/files/write", bytes.NewBufferString("not json"))
	req.Header.Set("Authorization", "Bearer test-api-key")
	rr := httptest.NewRecorder()
	s.Handler().ServeHTTP(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", rr.Code)
	}
}

func TestWriteFileHandler_NoAuth(t *testing.T) {
	s := newTestServer(&mockNFS{}, &mockRCON{})
	rr := doRequestNoAuth(s.Handler(), "POST", "/servers/myserver/files/write")
	if rr.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", rr.Code)
	}
}
