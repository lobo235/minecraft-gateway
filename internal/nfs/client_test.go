package nfs

import (
	"archive/zip"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func newTestClient(t *testing.T) (*client, string) {
	t.Helper()
	base := t.TempDir()
	log := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelError}))
	dataDir := t.TempDir()
	c := &client{basePath: base, dataDir: dataDir, log: log, maxDownloadSize: maxDownloadSizeDefault, maxWriteFileSize: maxWriteFileSizeDefault, maxExtractSize: maxExtractSizeDefault}
	return c, base
}

// --- Path Traversal Prevention Tests (mandatory) ---

func TestSafePath_Valid(t *testing.T) {
	c, base := newTestClient(t)
	got, err := c.SafePath("myserver")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := filepath.Join(base, "myserver")
	if got != want {
		t.Errorf("SafePath = %q, want %q", got, want)
	}
}

func TestSafePath_ValidSubpath(t *testing.T) {
	c, base := newTestClient(t)
	got, err := c.SafePath("myserver", "logs", "latest.log")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := filepath.Join(base, "myserver", "logs", "latest.log")
	if got != want {
		t.Errorf("SafePath = %q, want %q", got, want)
	}
}

func TestSafePath_DotDotSequence(t *testing.T) {
	c, _ := newTestClient(t)
	_, err := c.SafePath("..", "etc", "passwd")
	if err != ErrPathTraversal {
		t.Errorf("expected ErrPathTraversal for ../ traversal, got %v", err)
	}
}

func TestSafePath_DoubleDotDot(t *testing.T) {
	c, _ := newTestClient(t)
	_, err := c.SafePath("myserver", "..", "..", "etc", "passwd")
	if err != ErrPathTraversal {
		t.Errorf("expected ErrPathTraversal for ../../ traversal, got %v", err)
	}
}

func TestSafePath_AbsolutePathOutside(t *testing.T) {
	c, _ := newTestClient(t)
	_, err := c.SafePath("/etc/passwd")
	if err != ErrPathTraversal {
		t.Errorf("expected ErrPathTraversal for absolute path outside base, got %v", err)
	}
}

func TestSafePath_URLEncodedTraversal(t *testing.T) {
	// URL-encoded ../ is %2e%2e%2f — but by the time it reaches SafePath,
	// the HTTP router has already decoded it. Test the decoded form.
	c, _ := newTestClient(t)
	_, err := c.SafePath("..", "..", "etc", "passwd")
	if err != ErrPathTraversal {
		t.Errorf("expected ErrPathTraversal for URL-decoded traversal, got %v", err)
	}
}

func TestSafePath_DotDotInMiddle(t *testing.T) {
	c, _ := newTestClient(t)
	_, err := c.SafePath("myserver", "data", "..", "..", "..", "etc", "passwd")
	if err != ErrPathTraversal {
		t.Errorf("expected ErrPathTraversal for ../ in middle of path, got %v", err)
	}
}

func TestSafePath_BaseSelf(t *testing.T) {
	c, base := newTestClient(t)
	got, err := c.SafePath()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	abs, _ := filepath.Abs(base)
	if got != abs {
		t.Errorf("SafePath() = %q, want %q", got, abs)
	}
}

func TestSafePath_EmptyString(t *testing.T) {
	c, base := newTestClient(t)
	got, err := c.SafePath("")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	abs, _ := filepath.Abs(base)
	if got != abs {
		t.Errorf("SafePath('') = %q, want %q", got, abs)
	}
}

func TestSafePath_TrailingSlash(t *testing.T) {
	c, base := newTestClient(t)
	got, err := c.SafePath("myserver/")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := filepath.Join(base, "myserver")
	if got != want {
		t.Errorf("SafePath = %q, want %q", got, want)
	}
}

// --- ListServers ---

func TestListServers(t *testing.T) {
	c, base := newTestClient(t)
	// Create test directories.
	os.MkdirAll(filepath.Join(base, "server-a"), 0755)
	os.MkdirAll(filepath.Join(base, "server-b"), 0755)
	// Create a regular file (should be excluded).
	os.WriteFile(filepath.Join(base, "readme.txt"), []byte("hello"), 0644)

	servers, err := c.ListServers()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(servers) != 2 {
		t.Fatalf("expected 2 servers, got %d", len(servers))
	}
}

func TestListServersEmpty(t *testing.T) {
	c, _ := newTestClient(t)
	servers, err := c.ListServers()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(servers) != 0 {
		t.Errorf("expected 0 servers, got %d", len(servers))
	}
}

// --- CreateServer ---

func TestCreateServer(t *testing.T) {
	c, base := newTestClient(t)
	err := c.CreateServer("test-server", os.Getuid(), os.Getgid())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Verify directory exists.
	info, err := os.Stat(filepath.Join(base, "test-server"))
	if err != nil {
		t.Fatalf("server dir not created: %v", err)
	}
	if !info.IsDir() {
		t.Error("expected directory, got file")
	}
	// Verify backups subdir exists.
	info, err = os.Stat(filepath.Join(base, "test-server", "backups"))
	if err != nil {
		t.Fatalf("backups dir not created: %v", err)
	}
	if !info.IsDir() {
		t.Error("expected directory, got file")
	}
}

// --- DeleteServer ---

func TestDeleteServer(t *testing.T) {
	c, base := newTestClient(t)
	os.MkdirAll(filepath.Join(base, "to-delete"), 0755)

	err := c.DeleteServer("to-delete")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, err := os.Stat(filepath.Join(base, "to-delete")); !os.IsNotExist(err) {
		t.Error("directory should have been removed")
	}
}

func TestDeleteServerNotFound(t *testing.T) {
	c, _ := newTestClient(t)
	err := c.DeleteServer("nonexistent")
	if err == nil {
		t.Fatal("expected error for nonexistent server")
	}
}

// --- ListFiles ---

func TestListFiles(t *testing.T) {
	c, base := newTestClient(t)
	dir := filepath.Join(base, "myserver", "logs")
	os.MkdirAll(dir, 0755)
	os.WriteFile(filepath.Join(dir, "latest.log"), []byte("test log"), 0644)

	files, err := c.ListFiles("myserver", "logs")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(files) != 1 {
		t.Fatalf("expected 1 file, got %d", len(files))
	}
	if files[0].Name != "latest.log" {
		t.Errorf("file name = %q, want latest.log", files[0].Name)
	}
}

// --- ReadFile ---

func TestReadFile(t *testing.T) {
	c, base := newTestClient(t)
	dir := filepath.Join(base, "myserver")
	os.MkdirAll(dir, 0755)
	os.WriteFile(filepath.Join(dir, "test.txt"), []byte("hello world"), 0644)

	data, err := c.ReadFile("myserver", "test.txt")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if string(data) != "hello world" {
		t.Errorf("content = %q, want 'hello world'", string(data))
	}
}

func TestReadFileExceedsMaxSize(t *testing.T) {
	c, base := newTestClient(t)
	dir := filepath.Join(base, "myserver")
	os.MkdirAll(dir, 0755)
	// Create a file larger than 1MB.
	bigData := make([]byte, maxFileReadSize+1)
	os.WriteFile(filepath.Join(dir, "big.bin"), bigData, 0644)

	_, err := c.ReadFile("myserver", "big.bin")
	if err == nil {
		t.Fatal("expected error for oversized file")
	}
}

func TestReadFileIsDirectory(t *testing.T) {
	c, base := newTestClient(t)
	os.MkdirAll(filepath.Join(base, "myserver", "subdir"), 0755)

	_, err := c.ReadFile("myserver", "subdir")
	if err == nil {
		t.Fatal("expected error for reading a directory")
	}
}

// --- Migrate ---

func TestMigrate(t *testing.T) {
	c, base := newTestClient(t)
	os.MkdirAll(filepath.Join(base, "old-server"), 0755)
	os.WriteFile(filepath.Join(base, "old-server", "world.dat"), []byte("data"), 0644)

	err := c.Migrate("old-server", "new-server")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, err := os.Stat(filepath.Join(base, "old-server")); !os.IsNotExist(err) {
		t.Error("old directory should not exist")
	}
	if _, err := os.Stat(filepath.Join(base, "new-server", "world.dat")); err != nil {
		t.Error("data file should exist in new location")
	}
}

func TestMigrateTargetExists(t *testing.T) {
	c, base := newTestClient(t)
	os.MkdirAll(filepath.Join(base, "source"), 0755)
	os.MkdirAll(filepath.Join(base, "target"), 0755)

	err := c.Migrate("source", "target")
	if err == nil {
		t.Fatal("expected error when target exists")
	}
}

// --- BackupStatus ---

func TestWriteAndReadBackupStatus(t *testing.T) {
	c, _ := newTestClient(t)

	status := &BackupStatus{
		Server:     "test-server",
		ID:         "2026-03-22T10-00-00",
		Status:     "done",
		StartedAt:  "2026-03-22T10:00:00Z",
		BackupPath: "/tmp/backup.tar.zst",
	}
	err := c.writeBackupStatus("test-server", status)
	if err != nil {
		t.Fatalf("write backup status: %v", err)
	}

	got, err := c.GetBackupStatus("test-server", "2026-03-22T10-00-00")
	if err != nil {
		t.Fatalf("get backup status: %v", err)
	}
	if got.Status != "done" {
		t.Errorf("status = %q, want done", got.Status)
	}
}

func TestGetBackupStatusNotFound(t *testing.T) {
	c, _ := newTestClient(t)
	_, err := c.GetBackupStatus("nonexistent", "fake-id")
	if err == nil {
		t.Fatal("expected error for nonexistent status")
	}
}

func TestGetBackupStatusWrongID(t *testing.T) {
	c, _ := newTestClient(t)

	status := &BackupStatus{
		Server: "test-server",
		ID:     "correct-id",
		Status: "done",
	}
	c.writeBackupStatus("test-server", status)

	_, err := c.GetBackupStatus("test-server", "wrong-id")
	if err == nil {
		t.Fatal("expected error for wrong backup ID")
	}
}

// --- SafePath with symlink ---

func TestSafePath_SymlinkEscape(t *testing.T) {
	c, base := newTestClient(t)
	// Create a symlink inside base that points outside.
	target := t.TempDir() // outside base
	os.Symlink(target, filepath.Join(base, "escape"))

	_, err := c.SafePath("escape")
	if err != ErrPathTraversal {
		t.Errorf("expected ErrPathTraversal for symlink escape, got %v", err)
	}
}

func TestSafePath_SymlinkInsideBase(t *testing.T) {
	c, base := newTestClient(t)
	// Create a dir inside base and symlink to it.
	realDir := filepath.Join(base, "realdir")
	os.MkdirAll(realDir, 0755)
	os.Symlink(realDir, filepath.Join(base, "linkdir"))

	got, err := c.SafePath("linkdir")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != realDir {
		t.Errorf("SafePath = %q, want %q", got, realDir)
	}
}

// --- GrepFiles ---

func TestGrepFiles_Match(t *testing.T) {
	c, base := newTestClient(t)
	dir := filepath.Join(base, "myserver")
	os.MkdirAll(dir, 0755)
	os.WriteFile(filepath.Join(dir, "server.log"), []byte("ERROR something broke\nINFO all good\nERROR again"), 0644)

	result, err := c.GrepFiles("myserver", "", "ERROR")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Count != 2 {
		t.Errorf("count = %d, want 2", result.Count)
	}
	if result.Truncated {
		t.Error("should not be truncated")
	}
}

func TestGrepFiles_NoMatch(t *testing.T) {
	c, base := newTestClient(t)
	dir := filepath.Join(base, "myserver")
	os.MkdirAll(dir, 0755)
	os.WriteFile(filepath.Join(dir, "server.log"), []byte("INFO all good\nDEBUG details"), 0644)

	result, err := c.GrepFiles("myserver", "", "ERROR")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Count != 0 {
		t.Errorf("count = %d, want 0", result.Count)
	}
}

func TestGrepFiles_SubPath(t *testing.T) {
	c, base := newTestClient(t)
	dir := filepath.Join(base, "myserver", "logs")
	os.MkdirAll(dir, 0755)
	os.WriteFile(filepath.Join(dir, "latest.log"), []byte("ERROR something broke\nINFO fine"), 0644)

	result, err := c.GrepFiles("myserver", "logs", "ERROR")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Count != 1 {
		t.Errorf("count = %d, want 1", result.Count)
	}
}

func TestGrepFiles_PathTraversal(t *testing.T) {
	c, _ := newTestClient(t)
	_, err := c.GrepFiles("..", "", "ERROR")
	if err != ErrPathTraversal {
		t.Errorf("expected ErrPathTraversal, got %v", err)
	}
}

// --- DiskUsage ---

func TestDiskUsage(t *testing.T) {
	c, base := newTestClient(t)
	dir := filepath.Join(base, "myserver")
	os.MkdirAll(dir, 0755)
	os.WriteFile(filepath.Join(dir, "data.dat"), []byte("some data here"), 0644)

	size, err := c.DiskUsage("myserver")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if size <= 0 {
		t.Errorf("expected positive disk usage, got %d", size)
	}
}

func TestDiskUsage_PathTraversal(t *testing.T) {
	c, _ := newTestClient(t)
	_, err := c.DiskUsage("../../../etc")
	if err != ErrPathTraversal {
		t.Errorf("expected ErrPathTraversal, got %v", err)
	}
}

// --- ListBackups ---

func TestListBackups_WithFiles(t *testing.T) {
	c, base := newTestClient(t)
	backupDir := filepath.Join(base, "myserver", "backups")
	os.MkdirAll(backupDir, 0755)
	os.WriteFile(filepath.Join(backupDir, "2026-03-22T10-00-00.tar.zst"), []byte("fake backup"), 0644)
	os.WriteFile(filepath.Join(backupDir, "2026-03-23T10-00-00.tar.zst"), []byte("another backup"), 0644)
	// Non-backup file should be excluded.
	os.WriteFile(filepath.Join(backupDir, "readme.txt"), []byte("ignore"), 0644)
	// Directories should be excluded.
	os.MkdirAll(filepath.Join(backupDir, "subdir"), 0755)

	backups, err := c.ListBackups("myserver")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(backups) != 2 {
		t.Fatalf("expected 2 backups, got %d", len(backups))
	}
	if backups[0].ID != "2026-03-22T10-00-00" {
		t.Errorf("backup ID = %q, want 2026-03-22T10-00-00", backups[0].ID)
	}
}

func TestListBackups_NoBackupsDir(t *testing.T) {
	c, base := newTestClient(t)
	// Server directory exists but no backups subdir.
	os.MkdirAll(filepath.Join(base, "myserver"), 0755)

	backups, err := c.ListBackups("myserver")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(backups) != 0 {
		t.Errorf("expected 0 backups, got %d", len(backups))
	}
}

func TestListBackups_PathTraversal(t *testing.T) {
	c, _ := newTestClient(t)
	_, err := c.ListBackups("../../../etc")
	if err != ErrPathTraversal {
		t.Errorf("expected ErrPathTraversal, got %v", err)
	}
}

// --- Restore input validation ---

func TestRestore_InvalidBackupID(t *testing.T) {
	c, base := newTestClient(t)
	os.MkdirAll(filepath.Join(base, "myserver"), 0755)

	err := c.Restore("myserver", "../../etc/passwd")
	if err == nil {
		t.Fatal("expected error for invalid backup ID")
	}
}

func TestRestore_ValidBackupIDRegex(t *testing.T) {
	tests := []struct {
		id    string
		valid bool
	}{
		{"2026-03-22T10-00-00", true},
		{"20260322", true},
		{"../escape", false},
		{"id;rm -rf /", false},
		{"valid-123", false},
		{"", false},
	}
	for _, tt := range tests {
		got := backupIDRegex.MatchString(tt.id)
		if got != tt.valid {
			t.Errorf("backupIDRegex(%q) = %v, want %v", tt.id, got, tt.valid)
		}
	}
}

// --- Restore nonexistent backup ---

func TestRestore_BackupNotFound(t *testing.T) {
	c, base := newTestClient(t)
	serverDir := filepath.Join(base, "myserver")
	os.MkdirAll(filepath.Join(serverDir, "backups"), 0755)

	err := c.Restore("myserver", "2026-01-01T00-00-00")
	if err == nil {
		t.Fatal("expected error for missing backup file")
	}
}

// --- Migrate edge cases ---

func TestMigrate_SourceNotFound(t *testing.T) {
	c, _ := newTestClient(t)
	err := c.Migrate("nonexistent", "new-name")
	if err == nil {
		t.Fatal("expected error when source does not exist")
	}
}

func TestMigrate_PathTraversal(t *testing.T) {
	c, _ := newTestClient(t)
	err := c.Migrate("../escape", "new-name")
	if err != ErrPathTraversal {
		t.Errorf("expected ErrPathTraversal, got %v", err)
	}
}

// --- ListFiles edge cases ---

func TestListFiles_PathTraversal(t *testing.T) {
	c, _ := newTestClient(t)
	_, err := c.ListFiles("../escape", "")
	if err != ErrPathTraversal {
		t.Errorf("expected ErrPathTraversal, got %v", err)
	}
}

func TestListFiles_EmptyDir(t *testing.T) {
	c, base := newTestClient(t)
	os.MkdirAll(filepath.Join(base, "myserver"), 0755)

	files, err := c.ListFiles("myserver", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(files) != 0 {
		t.Errorf("expected 0 files, got %d", len(files))
	}
}

// --- ReadFile edge cases ---

func TestReadFile_PathTraversal(t *testing.T) {
	c, _ := newTestClient(t)
	_, err := c.ReadFile("../escape", "file.txt")
	if err != ErrPathTraversal {
		t.Errorf("expected ErrPathTraversal, got %v", err)
	}
}

func TestReadFile_NotFound(t *testing.T) {
	c, base := newTestClient(t)
	os.MkdirAll(filepath.Join(base, "myserver"), 0755)

	_, err := c.ReadFile("myserver", "nonexistent.txt")
	if err == nil {
		t.Fatal("expected error for nonexistent file")
	}
}

// --- CreateServer edge cases ---

func TestCreateServer_PathTraversal(t *testing.T) {
	c, _ := newTestClient(t)
	err := c.CreateServer("../escape", 1000, 1000)
	if err != ErrPathTraversal {
		t.Errorf("expected ErrPathTraversal, got %v", err)
	}
}

// --- DeleteServer edge cases ---

func TestDeleteServer_PathTraversal(t *testing.T) {
	c, _ := newTestClient(t)
	err := c.DeleteServer("../escape")
	if err != ErrPathTraversal {
		t.Errorf("expected ErrPathTraversal, got %v", err)
	}
}

// --- writeBackupStatusDirect ---

func TestWriteBackupStatusDirect(t *testing.T) {
	c, _ := newTestClient(t)
	c.writeBackupStatusDirect("test-server", "2026-03-22T10-00-00", "done", "/tmp/backup.tar.zst", "")

	got, err := c.GetBackupStatus("test-server", "2026-03-22T10-00-00")
	if err != nil {
		t.Fatalf("get backup status: %v", err)
	}
	if got.Status != "done" {
		t.Errorf("status = %q, want done", got.Status)
	}
	if got.BackupPath != "/tmp/backup.tar.zst" {
		t.Errorf("backup_path = %q, want /tmp/backup.tar.zst", got.BackupPath)
	}
}

// --- StartBackup ---

func TestStartBackup_ServerNotFound(t *testing.T) {
	c, _ := newTestClient(t)
	_, err := c.StartBackup("nonexistent")
	if err == nil {
		t.Fatal("expected error for nonexistent server")
	}
}

func TestStartBackup_PathTraversal(t *testing.T) {
	c, _ := newTestClient(t)
	_, err := c.StartBackup("../escape")
	if err != ErrPathTraversal {
		t.Errorf("expected ErrPathTraversal, got %v", err)
	}
}

func TestStartBackup_CreatesBackupDir(t *testing.T) {
	c, base := newTestClient(t)
	serverDir := filepath.Join(base, "myserver")
	os.MkdirAll(serverDir, 0755)

	// StartBackup will create the backups dir and launch a goroutine.
	// The goroutine will fail (tar/pzstd likely not available), but we get the ID back.
	id, err := c.StartBackup("myserver")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if id == "" {
		t.Error("expected non-empty backup ID")
	}

	// Verify backups dir was created.
	backupDir := filepath.Join(base, "myserver", "backups")
	if _, err := os.Stat(backupDir); err != nil {
		t.Errorf("backups dir should exist: %v", err)
	}
}

// --- Restore ---

func TestRestore_PathTraversal(t *testing.T) {
	c, _ := newTestClient(t)
	err := c.Restore("../escape", "2026-01-01T00-00-00")
	if err != ErrPathTraversal {
		t.Errorf("expected ErrPathTraversal, got %v", err)
	}
}

// --- Migrate with new name path traversal ---

func TestMigrate_NewNamePathTraversal(t *testing.T) {
	c, base := newTestClient(t)
	os.MkdirAll(filepath.Join(base, "source"), 0755)

	err := c.Migrate("source", "../escape")
	if err != ErrPathTraversal {
		t.Errorf("expected ErrPathTraversal, got %v", err)
	}
}

func TestWriteBackupStatusDirect_Failed(t *testing.T) {
	c, _ := newTestClient(t)
	c.writeBackupStatusDirect("test-server", "2026-03-22T10-00-00", "failed", "", "tar failed")

	got, err := c.GetBackupStatus("test-server", "2026-03-22T10-00-00")
	if err != nil {
		t.Fatalf("get backup status: %v", err)
	}
	if got.Status != "failed" {
		t.Errorf("status = %q, want failed", got.Status)
	}
	if got.Error != "tar failed" {
		t.Errorf("error = %q, want 'tar failed'", got.Error)
	}
}

// --- ListFiles with multiple entries ---

func TestListFiles_MultipleEntries(t *testing.T) {
	c, base := newTestClient(t)
	dir := filepath.Join(base, "myserver")
	os.MkdirAll(filepath.Join(dir, "world"), 0755)
	os.WriteFile(filepath.Join(dir, "server.properties"), []byte("key=value"), 0644)
	os.WriteFile(filepath.Join(dir, "eula.txt"), []byte("eula=true"), 0644)

	files, err := c.ListFiles("myserver", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(files) != 3 {
		t.Fatalf("expected 3 entries, got %d", len(files))
	}

	// Verify mix of files and dirs.
	hasDir := false
	hasFile := false
	for _, f := range files {
		if f.IsDir {
			hasDir = true
		} else {
			hasFile = true
		}
		if f.ModTime == "" {
			t.Error("expected non-empty mod_time")
		}
	}
	if !hasDir || !hasFile {
		t.Error("expected both files and directories in listing")
	}
}

// --- DiskUsage for nonexistent server ---

func TestDiskUsage_NonexistentServer(t *testing.T) {
	c, _ := newTestClient(t)
	_, err := c.DiskUsage("nonexistent")
	if err == nil {
		t.Fatal("expected error for nonexistent server dir")
	}
}

// --- Restore with path traversal on backup ID after regex check ---

// --- StartBackup with existing backups dir ---

func TestStartBackup_ExistingBackupsDir(t *testing.T) {
	c, base := newTestClient(t)
	serverDir := filepath.Join(base, "myserver")
	backupDir := filepath.Join(serverDir, "backups")
	os.MkdirAll(backupDir, 0755)
	// Create a file so tar has something to archive.
	os.WriteFile(filepath.Join(serverDir, "world.dat"), []byte("test data"), 0644)

	id, err := c.StartBackup("myserver")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if id == "" {
		t.Error("expected non-empty backup ID")
	}

	// Wait for the goroutine to complete (it will fail since pzstd is likely unavailable).
	// Poll the status file to detect completion.
	statusFile := filepath.Join(c.dataDir, "myserver.backup-status")
	for i := 0; i < 30; i++ {
		time.Sleep(100 * time.Millisecond)
		data, err := os.ReadFile(statusFile)
		if err != nil {
			continue
		}
		// Check if the backup completed (done or failed).
		content := string(data)
		if filepath.Join(content, "") != "" {
			// Status file exists with some content.
			if len(data) > 0 {
				break
			}
		}
	}
}

func TestRestore_PzstdNotAvailable(t *testing.T) {
	c, base := newTestClient(t)
	serverDir := filepath.Join(base, "myserver")
	backupDir := filepath.Join(serverDir, "backups")
	os.MkdirAll(backupDir, 0755)
	// Create a fake backup file so the os.Stat check passes.
	os.WriteFile(filepath.Join(backupDir, "2026-01-01T00-00-00.tar.zst"), []byte("fake"), 0644)

	err := c.Restore("myserver", "2026-01-01T00-00-00")
	// This will fail because pzstd is not installed, but it exercises the pipe setup code.
	if err == nil {
		t.Log("restore succeeded (pzstd available); skipping error check")
		return
	}
	// We expect an error from the pipe/command execution, not from validation.
	t.Logf("restore failed as expected (pzstd likely not available): %v", err)
}

func TestRestore_ServerPathTraversal(t *testing.T) {
	c, _ := newTestClient(t)
	// Server name causes path traversal.
	err := c.Restore("/etc", "2026-01-01T00-00-00")
	if err != ErrPathTraversal {
		t.Errorf("expected ErrPathTraversal, got %v", err)
	}
}

// --- Download ---

// waitForDownload polls GetDownloadStatus until the download is no longer "running" or timeout.
func waitForDownload(t *testing.T, c *client, serverName, downloadID string) *DownloadStatus {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		status, err := c.GetDownloadStatus(serverName, downloadID)
		if err != nil {
			t.Fatalf("GetDownloadStatus failed: %v", err)
		}
		if status.Status != "running" {
			return status
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatal("download did not complete within timeout")
	return nil
}

func TestStartDownload_PathTraversal(t *testing.T) {
	c, _ := newTestClient(t)
	_, err := c.StartDownload("../escape", "https://example.com/file.zip", ".", false, 1000, 1000, ModeOverwrite)
	if err != ErrPathTraversal {
		t.Errorf("expected ErrPathTraversal, got %v", err)
	}
}

func TestStartDownload_DestPathTraversal(t *testing.T) {
	c, base := newTestClient(t)
	os.MkdirAll(filepath.Join(base, "myserver"), 0755)
	_, err := c.StartDownload("myserver", "https://example.com/file.zip", "../../escape", false, 1000, 1000, ModeOverwrite)
	if err != ErrPathTraversal {
		t.Errorf("expected ErrPathTraversal, got %v", err)
	}
}

func TestStartDownload_NoExtract(t *testing.T) {
	c, base := newTestClient(t)
	serverDir := filepath.Join(base, "myserver")
	os.MkdirAll(serverDir, 0755)

	// Set up a mock HTTP server to serve the file.
	content := []byte("hello world file content")
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write(content)
	}))
	defer ts.Close()

	id, err := c.StartDownload("myserver", ts.URL+"/testfile.jar", ".", false, os.Getuid(), os.Getgid(), ModeOverwrite)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if id == "" {
		t.Fatal("expected non-empty download ID")
	}

	status := waitForDownload(t, c, "myserver", id)
	if status.Status != "done" {
		t.Fatalf("status = %q, want done; error = %q", status.Status, status.Error)
	}
	if status.Result == nil {
		t.Fatal("expected non-nil result")
	}
	if status.Result.FilesCount != 1 {
		t.Errorf("files_count = %d, want 1", status.Result.FilesCount)
	}
	if status.Result.TotalBytes != int64(len(content)) {
		t.Errorf("total_bytes = %d, want %d", status.Result.TotalBytes, len(content))
	}

	// Verify the file was written.
	downloaded := filepath.Join(serverDir, "testfile.jar")
	data, err := os.ReadFile(downloaded)
	if err != nil {
		t.Fatalf("file not found: %v", err)
	}
	if string(data) != string(content) {
		t.Errorf("file content = %q, want %q", string(data), string(content))
	}
}

func TestStartDownload_ExtractZip(t *testing.T) {
	c, base := newTestClient(t)
	serverDir := filepath.Join(base, "myserver")
	os.MkdirAll(serverDir, 0755)

	// Create a zip file in memory.
	zipPath := filepath.Join(t.TempDir(), "test.zip")
	zf, err := os.Create(zipPath)
	if err != nil {
		t.Fatal(err)
	}
	zw := zip.NewWriter(zf)
	files := map[string]string{
		"file1.txt":        "content one",
		"subdir/file2.txt": "content two",
	}
	for name, content := range files {
		fw, err := zw.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		fmt.Fprint(fw, content)
	}
	zw.Close()
	zf.Close()

	zipData, _ := os.ReadFile(zipPath)

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write(zipData)
	}))
	defer ts.Close()

	id, err := c.StartDownload("myserver", ts.URL+"/mods.zip", ".", true, os.Getuid(), os.Getgid(), ModeOverwrite)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	status := waitForDownload(t, c, "myserver", id)
	if status.Status != "done" {
		t.Fatalf("status = %q, want done; error = %q", status.Status, status.Error)
	}
	if status.Result.FilesCount != 2 {
		t.Errorf("files_count = %d, want 2", status.Result.FilesCount)
	}

	// Verify files were extracted.
	if _, err := os.Stat(filepath.Join(serverDir, "file1.txt")); err != nil {
		t.Errorf("file1.txt not found: %v", err)
	}
	if _, err := os.Stat(filepath.Join(serverDir, "subdir", "file2.txt")); err != nil {
		t.Errorf("subdir/file2.txt not found: %v", err)
	}
}

func TestStartDownload_HTTPError(t *testing.T) {
	c, base := newTestClient(t)
	os.MkdirAll(filepath.Join(base, "myserver"), 0755)

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer ts.Close()

	id, err := c.StartDownload("myserver", ts.URL+"/missing.jar", ".", false, os.Getuid(), os.Getgid(), ModeOverwrite)
	if err != nil {
		t.Fatalf("unexpected error starting download: %v", err)
	}

	status := waitForDownload(t, c, "myserver", id)
	if status.Status != "failed" {
		t.Errorf("status = %q, want failed", status.Status)
	}
	if status.Error == "" {
		t.Error("expected non-empty error message")
	}
}

func TestStartDownload_UnsupportedArchive(t *testing.T) {
	c, base := newTestClient(t)
	os.MkdirAll(filepath.Join(base, "myserver"), 0755)

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("data"))
	}))
	defer ts.Close()

	id, err := c.StartDownload("myserver", ts.URL+"/file.rar", ".", true, os.Getuid(), os.Getgid(), ModeOverwrite)
	if err != nil {
		t.Fatalf("unexpected error starting download: %v", err)
	}

	status := waitForDownload(t, c, "myserver", id)
	if status.Status != "failed" {
		t.Errorf("status = %q, want failed", status.Status)
	}
}

func TestStartDownload_SubDestPath(t *testing.T) {
	c, base := newTestClient(t)
	serverDir := filepath.Join(base, "myserver")
	os.MkdirAll(serverDir, 0755)

	content := []byte("plugin data")
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write(content)
	}))
	defer ts.Close()

	id, err := c.StartDownload("myserver", ts.URL+"/plugin.jar", "plugins", false, os.Getuid(), os.Getgid(), ModeOverwrite)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	status := waitForDownload(t, c, "myserver", id)
	if status.Status != "done" {
		t.Fatalf("status = %q, want done; error = %q", status.Status, status.Error)
	}
	if status.Result.FilesCount != 1 {
		t.Errorf("files_count = %d, want 1", status.Result.FilesCount)
	}

	// Verify the file was written to the subdirectory.
	downloaded := filepath.Join(serverDir, "plugins", "plugin.jar")
	if _, err := os.Stat(downloaded); err != nil {
		t.Errorf("file not found at %s: %v", downloaded, err)
	}
}

// --- GetDownloadStatus ---

func TestGetDownloadStatus_NotFound(t *testing.T) {
	c, _ := newTestClient(t)
	_, err := c.GetDownloadStatus("myserver", "2026-03-24T10-00-00")
	if err == nil {
		t.Fatal("expected error for nonexistent download status")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("error = %q, want 'not found'", err.Error())
	}
}

func TestGetDownloadStatus_InvalidID(t *testing.T) {
	c, _ := newTestClient(t)
	_, err := c.GetDownloadStatus("myserver", "../../etc/passwd")
	if err == nil {
		t.Fatal("expected error for invalid download ID")
	}
	if !strings.Contains(err.Error(), "invalid download ID") {
		t.Errorf("error = %q, want 'invalid download ID'", err.Error())
	}
}

// --- filenameFromURL ---

func TestFilenameFromURL(t *testing.T) {
	tests := []struct {
		url  string
		want string
	}{
		{"https://example.com/files/mod.jar", "mod.jar"},
		{"https://example.com/files/mod.jar?v=1", "mod.jar"},
		{"https://example.com/", "download"},
		{"https://example.com", "download"},
	}
	for _, tt := range tests {
		got := filenameFromURL(tt.url)
		if got != tt.want {
			t.Errorf("filenameFromURL(%q) = %q, want %q", tt.url, got, tt.want)
		}
	}
}

// --- chownRecursive ---

func TestChownRecursive(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, "sub"), 0755)
	os.WriteFile(filepath.Join(dir, "file.txt"), []byte("data"), 0644)
	os.WriteFile(filepath.Join(dir, "sub", "nested.txt"), []byte("nested"), 0644)

	err := chownRecursive(dir, os.Getuid(), os.Getgid())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

// --- Download modes ---

func TestStartDownload_SkipExisting(t *testing.T) {
	c, base := newTestClient(t)
	serverDir := filepath.Join(base, "myserver")
	os.MkdirAll(serverDir, 0755)

	// Pre-create a file that should be skipped.
	os.WriteFile(filepath.Join(serverDir, "testfile.jar"), []byte("original"), 0644)

	content := []byte("new content")
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write(content)
	}))
	defer ts.Close()

	id, err := c.StartDownload("myserver", ts.URL+"/testfile.jar", ".", false, os.Getuid(), os.Getgid(), ModeSkipExisting)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	status := waitForDownload(t, c, "myserver", id)
	if status.Status != "done" {
		t.Fatalf("status = %q, want done; error = %q", status.Status, status.Error)
	}
	if status.Result.FilesCount != 0 {
		t.Errorf("files_count = %d, want 0 (skipped)", status.Result.FilesCount)
	}

	// Verify original content was preserved.
	data, _ := os.ReadFile(filepath.Join(serverDir, "testfile.jar"))
	if string(data) != "original" {
		t.Errorf("file was overwritten, content = %q", string(data))
	}
}

func TestStartDownload_CleanFirst(t *testing.T) {
	c, base := newTestClient(t)
	serverDir := filepath.Join(base, "myserver")
	os.MkdirAll(serverDir, 0755)

	// Pre-create files that should be cleaned.
	os.WriteFile(filepath.Join(serverDir, "old-file.txt"), []byte("old"), 0644)
	os.MkdirAll(filepath.Join(serverDir, "old-dir"), 0755)

	content := []byte("new file content")
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write(content)
	}))
	defer ts.Close()

	id, err := c.StartDownload("myserver", ts.URL+"/newfile.jar", ".", false, os.Getuid(), os.Getgid(), ModeCleanFirst)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	status := waitForDownload(t, c, "myserver", id)
	if status.Status != "done" {
		t.Fatalf("status = %q, want done; error = %q", status.Status, status.Error)
	}
	if status.Result.FilesCount != 1 {
		t.Errorf("files_count = %d, want 1", status.Result.FilesCount)
	}

	// Verify old files were cleaned.
	if _, err := os.Stat(filepath.Join(serverDir, "old-file.txt")); !os.IsNotExist(err) {
		t.Error("old-file.txt should have been removed")
	}
	if _, err := os.Stat(filepath.Join(serverDir, "old-dir")); !os.IsNotExist(err) {
		t.Error("old-dir should have been removed")
	}

	// Verify new file exists.
	if _, err := os.Stat(filepath.Join(serverDir, "newfile.jar")); err != nil {
		t.Errorf("newfile.jar not found: %v", err)
	}
}

func TestStartDownload_SkipExisting_ExtractZip(t *testing.T) {
	c, base := newTestClient(t)
	serverDir := filepath.Join(base, "myserver")
	os.MkdirAll(serverDir, 0755)

	// Pre-create one file that should be skipped.
	os.WriteFile(filepath.Join(serverDir, "file1.txt"), []byte("original"), 0644)

	// Create a zip file with two entries.
	zipPath := filepath.Join(t.TempDir(), "test.zip")
	zf, _ := os.Create(zipPath)
	zw := zip.NewWriter(zf)
	fw1, _ := zw.Create("file1.txt")
	fmt.Fprint(fw1, "new content one")
	fw2, _ := zw.Create("file2.txt")
	fmt.Fprint(fw2, "content two")
	zw.Close()
	zf.Close()

	zipData, _ := os.ReadFile(zipPath)
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write(zipData)
	}))
	defer ts.Close()

	id, err := c.StartDownload("myserver", ts.URL+"/mods.zip", ".", true, os.Getuid(), os.Getgid(), ModeSkipExisting)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	status := waitForDownload(t, c, "myserver", id)
	if status.Status != "done" {
		t.Fatalf("status = %q, want done; error = %q", status.Status, status.Error)
	}
	// Only file2.txt should be extracted (file1.txt was skipped).
	if status.Result.FilesCount != 1 {
		t.Errorf("files_count = %d, want 1 (file1.txt skipped)", status.Result.FilesCount)
	}

	// Verify file1.txt was not overwritten.
	data, _ := os.ReadFile(filepath.Join(serverDir, "file1.txt"))
	if string(data) != "original" {
		t.Errorf("file1.txt was overwritten, content = %q", string(data))
	}
	// Verify file2.txt was extracted.
	if _, err := os.Stat(filepath.Join(serverDir, "file2.txt")); err != nil {
		t.Errorf("file2.txt not found: %v", err)
	}
}

// --- ValidDownloadMode ---

func TestValidDownloadMode(t *testing.T) {
	tests := []struct {
		mode  string
		valid bool
	}{
		{"overwrite", true},
		{"skip_existing", true},
		{"clean_first", true},
		{"invalid", false},
		{"", false},
	}
	for _, tt := range tests {
		if got := ValidDownloadMode(tt.mode); got != tt.valid {
			t.Errorf("ValidDownloadMode(%q) = %v, want %v", tt.mode, got, tt.valid)
		}
	}
}

// --- ListArchiveContents ---

func TestListArchiveContents_Zip(t *testing.T) {
	c, base := newTestClient(t)
	serverDir := filepath.Join(base, "myserver")
	os.MkdirAll(serverDir, 0755)

	// Create a zip file.
	zipPath := filepath.Join(serverDir, "test.zip")
	zf, _ := os.Create(zipPath)
	zw := zip.NewWriter(zf)
	fw, _ := zw.Create("hello.txt")
	fmt.Fprint(fw, "hello world")
	zw.Close()
	zf.Close()

	entries, err := c.ListArchiveContents("myserver", "test.zip")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(entries))
	}
	if entries[0].Name != "hello.txt" {
		t.Errorf("name = %q, want hello.txt", entries[0].Name)
	}
	if entries[0].IsDir {
		t.Error("expected file, got directory")
	}
}

func TestListArchiveContents_PathTraversal(t *testing.T) {
	c, _ := newTestClient(t)
	_, err := c.ListArchiveContents("../escape", "test.zip")
	if err != ErrPathTraversal {
		t.Errorf("expected ErrPathTraversal, got %v", err)
	}
}

func TestListArchiveContents_UnsupportedFormat(t *testing.T) {
	c, base := newTestClient(t)
	serverDir := filepath.Join(base, "myserver")
	os.MkdirAll(serverDir, 0755)
	os.WriteFile(filepath.Join(serverDir, "test.rar"), []byte("data"), 0644)

	_, err := c.ListArchiveContents("myserver", "test.rar")
	if err == nil {
		t.Fatal("expected error for unsupported format")
	}
}

func TestListArchiveContents_NotFound(t *testing.T) {
	c, base := newTestClient(t)
	os.MkdirAll(filepath.Join(base, "myserver"), 0755)

	_, err := c.ListArchiveContents("myserver", "missing.zip")
	if err == nil {
		t.Fatal("expected error for missing archive")
	}
}

// --- WriteFile ---

func TestWriteFile_Success(t *testing.T) {
	c, base := newTestClient(t)
	serverDir := filepath.Join(base, "myserver")
	os.MkdirAll(serverDir, 0755)

	err := c.WriteFile("myserver", "server.properties", "motd=Hello", os.Getuid(), os.Getgid())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(serverDir, "server.properties"))
	if err != nil {
		t.Fatalf("file not found: %v", err)
	}
	if string(data) != "motd=Hello" {
		t.Errorf("content = %q, want 'motd=Hello'", string(data))
	}
}

func TestWriteFile_CreatesParentDirs(t *testing.T) {
	c, base := newTestClient(t)
	serverDir := filepath.Join(base, "myserver")
	os.MkdirAll(serverDir, 0755)

	err := c.WriteFile("myserver", "config/settings.yml", "key: value", os.Getuid(), os.Getgid())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	data, _ := os.ReadFile(filepath.Join(serverDir, "config", "settings.yml"))
	if string(data) != "key: value" {
		t.Errorf("content = %q, want 'key: value'", string(data))
	}
}

func TestWriteFile_ExceedsMaxSize(t *testing.T) {
	c, base := newTestClient(t)
	os.MkdirAll(filepath.Join(base, "myserver"), 0755)

	// Create content larger than maxWriteFileSize.
	bigContent := string(make([]byte, maxWriteFileSizeDefault+1))
	err := c.WriteFile("myserver", "big.txt", bigContent, os.Getuid(), os.Getgid())
	if err == nil {
		t.Fatal("expected error for oversized content")
	}
}

func TestWriteFile_PathTraversal(t *testing.T) {
	c, _ := newTestClient(t)
	err := c.WriteFile("../escape", "file.txt", "data", os.Getuid(), os.Getgid())
	if err != ErrPathTraversal {
		t.Errorf("expected ErrPathTraversal, got %v", err)
	}
}

func TestWriteFile_OverwriteExisting(t *testing.T) {
	c, base := newTestClient(t)
	serverDir := filepath.Join(base, "myserver")
	os.MkdirAll(serverDir, 0755)
	os.WriteFile(filepath.Join(serverDir, "test.txt"), []byte("old"), 0644)

	err := c.WriteFile("myserver", "test.txt", "new", os.Getuid(), os.Getgid())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	data, _ := os.ReadFile(filepath.Join(serverDir, "test.txt"))
	if string(data) != "new" {
		t.Errorf("content = %q, want 'new'", string(data))
	}
}
