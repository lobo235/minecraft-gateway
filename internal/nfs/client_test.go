package nfs

import (
	"log/slog"
	"os"
	"path/filepath"
	"testing"
)

func newTestClient(t *testing.T) (*client, string) {
	t.Helper()
	base := t.TempDir()
	log := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelError}))
	dataDir := t.TempDir()
	c := &client{basePath: base, dataDir: dataDir, log: log}
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
