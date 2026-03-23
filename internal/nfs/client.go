package nfs

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// Client provides filesystem operations on the NFS-backed Minecraft server volume.
type Client interface {
	// SafePath resolves a path relative to the NFS base and validates it against traversal attacks.
	SafePath(parts ...string) (string, error)
	// ListServers returns all server directory names.
	ListServers() ([]string, error)
	// CreateServer initialises a new server directory with the given ownership.
	CreateServer(name string, uid, gid int) error
	// DeleteServer removes a server directory.
	DeleteServer(name string) error
	// DiskUsage returns the disk usage in bytes for a server directory.
	DiskUsage(name string) (int64, error)
	// ListFiles returns file entries at the given sub-path within a server directory.
	ListFiles(serverName, subPath string) ([]FileEntry, error)
	// ReadFile reads a file's contents (max 1MB).
	ReadFile(serverName, subPath string) ([]byte, error)
	// GrepFiles runs grep on a path within a server directory.
	GrepFiles(serverName, subPath, pattern string) (*GrepResult, error)
	// ListBackups returns available backups for a server.
	ListBackups(serverName string) ([]BackupInfo, error)
	// StartBackup triggers an async backup, returning the backup ID.
	StartBackup(serverName string) (string, error)
	// GetBackupStatus returns the status of a backup by ID.
	GetBackupStatus(serverName, backupID string) (*BackupStatus, error)
	// Restore restores a server from a backup.
	Restore(serverName, backupID string) error
	// Migrate renames a server directory.
	Migrate(serverName, newName string) error
}

// FileEntry represents a file or directory in a listing.
type FileEntry struct {
	Name    string `json:"name"`
	Size    int64  `json:"size"`
	IsDir   bool   `json:"is_dir"`
	ModTime string `json:"mod_time"`
}

// GrepResult holds grep output.
type GrepResult struct {
	Lines     []string `json:"lines"`
	Count     int      `json:"count"`
	Truncated bool     `json:"truncated"`
}

// BackupInfo describes an available backup file.
type BackupInfo struct {
	ID      string `json:"id"`
	Path    string `json:"path"`
	Size    int64  `json:"size"`
	Created string `json:"created"`
}

// BackupStatus tracks the state of an async backup operation.
type BackupStatus struct {
	Server      string `json:"server"`
	ID          string `json:"id"`
	Status      string `json:"status"` // running, done, failed
	StartedAt   string `json:"started_at"`
	CompletedAt string `json:"completed_at,omitempty"`
	BackupPath  string `json:"backup_path,omitempty"`
	Error       string `json:"error,omitempty"`
}

const (
	maxFileReadSize = 1 << 20 // 1MB
	maxGrepLines    = 10000
	maxGrepBytes    = 5 * (1 << 20) // 5MB
	backupsDir      = "backups"
)

type client struct {
	basePath string
	dataDir  string
	log      *slog.Logger
	mu       sync.Mutex // protects backup status file writes
}

// NewClient creates a new NFS filesystem client.
func NewClient(basePath, dataDir string, log *slog.Logger) Client {
	return &client{basePath: basePath, dataDir: dataDir, log: log}
}

// SafePath resolves path parts relative to basePath and verifies the result is within basePath.
func (c *client) SafePath(parts ...string) (string, error) {
	// Reject any part that is an absolute path — these bypass filepath.Join's relative resolution.
	for _, p := range parts {
		if filepath.IsAbs(p) {
			return "", ErrPathTraversal
		}
	}
	combined := filepath.Join(append([]string{c.basePath}, parts...)...)
	abs, err := filepath.Abs(combined)
	if err != nil {
		return "", fmt.Errorf("resolve path: %w", err)
	}
	// Ensure the resolved path is within basePath (or is basePath itself).
	base, err := filepath.Abs(c.basePath)
	if err != nil {
		return "", fmt.Errorf("resolve base path: %w", err)
	}
	if abs != base && !strings.HasPrefix(abs, base+string(filepath.Separator)) {
		return "", ErrPathTraversal
	}
	return abs, nil
}

// ErrPathTraversal is returned when a path resolves outside the NFS base directory.
var ErrPathTraversal = fmt.Errorf("path traversal detected")

// ListServers returns directory names under the NFS base path.
func (c *client) ListServers() ([]string, error) {
	entries, err := os.ReadDir(c.basePath)
	if err != nil {
		return nil, fmt.Errorf("list servers: %w", err)
	}
	var servers []string
	for _, e := range entries {
		if e.IsDir() {
			servers = append(servers, e.Name())
		}
	}
	return servers, nil
}

// CreateServer initialises a new server directory with the given ownership.
func (c *client) CreateServer(name string, uid, gid int) error {
	dir, err := c.SafePath(name)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("create server dir: %w", err)
	}
	if err := os.Chown(dir, uid, gid); err != nil {
		return fmt.Errorf("chown server dir: %w", err)
	}
	// Create backups subdirectory.
	backupDir := filepath.Join(dir, backupsDir)
	if err := os.MkdirAll(backupDir, 0755); err != nil {
		return fmt.Errorf("create backup dir: %w", err)
	}
	if err := os.Chown(backupDir, uid, gid); err != nil {
		return fmt.Errorf("chown backup dir: %w", err)
	}
	return nil
}

// DeleteServer removes a server directory entirely.
func (c *client) DeleteServer(name string) error {
	dir, err := c.SafePath(name)
	if err != nil {
		return err
	}
	if _, err := os.Stat(dir); os.IsNotExist(err) {
		return fmt.Errorf("server not found: %s", name)
	}
	return os.RemoveAll(dir)
}

// DiskUsage returns the total size of a server directory in bytes.
func (c *client) DiskUsage(name string) (int64, error) {
	dir, err := c.SafePath(name)
	if err != nil {
		return 0, err
	}
	out, err := exec.Command("du", "-sb", dir).Output()
	if err != nil {
		return 0, fmt.Errorf("du command: %w", err)
	}
	var size int64
	_, err = fmt.Sscanf(string(out), "%d", &size)
	if err != nil {
		return 0, fmt.Errorf("parse du output: %w", err)
	}
	return size, nil
}

// ListFiles returns file entries at a sub-path within a server directory.
func (c *client) ListFiles(serverName, subPath string) ([]FileEntry, error) {
	dir, err := c.SafePath(serverName, subPath)
	if err != nil {
		return nil, err
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("list files: %w", err)
	}
	var files []FileEntry
	for _, e := range entries {
		info, err := e.Info()
		if err != nil {
			continue
		}
		files = append(files, FileEntry{
			Name:    e.Name(),
			Size:    info.Size(),
			IsDir:   e.IsDir(),
			ModTime: info.ModTime().UTC().Format(time.RFC3339),
		})
	}
	return files, nil
}

// ReadFile reads a file's contents, enforcing a 1MB limit.
func (c *client) ReadFile(serverName, subPath string) ([]byte, error) {
	filePath, err := c.SafePath(serverName, subPath)
	if err != nil {
		return nil, err
	}
	info, err := os.Stat(filePath)
	if err != nil {
		return nil, fmt.Errorf("stat file: %w", err)
	}
	if info.IsDir() {
		return nil, fmt.Errorf("path is a directory, not a file")
	}
	if info.Size() > maxFileReadSize {
		return nil, fmt.Errorf("file size %d exceeds maximum of %d bytes", info.Size(), maxFileReadSize)
	}
	data, err := os.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("read file: %w", err)
	}
	return data, nil
}

// GrepFiles runs grep on a path within a server directory.
func (c *client) GrepFiles(serverName, subPath, pattern string) (*GrepResult, error) {
	target, err := c.SafePath(serverName, subPath)
	if err != nil {
		return nil, err
	}
	// Use grep with recursive flag for directories.
	args := []string{"-rn", "--", pattern, target}
	cmd := exec.Command("grep", args...)
	out, err := cmd.Output()
	// grep returns exit code 1 when no matches found — that's not an error.
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok && exitErr.ExitCode() == 1 {
			return &GrepResult{Lines: []string{}, Count: 0}, nil
		}
		return nil, fmt.Errorf("grep: %w", err)
	}

	lines := strings.Split(strings.TrimRight(string(out), "\n"), "\n")
	truncated := false
	totalBytes := 0

	var result []string
	for _, line := range lines {
		totalBytes += len(line) + 1
		if len(result) >= maxGrepLines || totalBytes > maxGrepBytes {
			truncated = true
			break
		}
		result = append(result, line)
	}

	return &GrepResult{
		Lines:     result,
		Count:     len(result),
		Truncated: truncated,
	}, nil
}

// ListBackups returns available .tar.zst backup files for a server.
func (c *client) ListBackups(serverName string) ([]BackupInfo, error) {
	backupPath, err := c.SafePath(serverName, backupsDir)
	if err != nil {
		return nil, err
	}
	entries, err := os.ReadDir(backupPath)
	if err != nil {
		if os.IsNotExist(err) {
			return []BackupInfo{}, nil
		}
		return nil, fmt.Errorf("list backups: %w", err)
	}
	var backups []BackupInfo
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".tar.zst") {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		id := strings.TrimSuffix(e.Name(), ".tar.zst")
		backups = append(backups, BackupInfo{
			ID:      id,
			Path:    filepath.Join(backupPath, e.Name()),
			Size:    info.Size(),
			Created: info.ModTime().UTC().Format(time.RFC3339),
		})
	}
	return backups, nil
}

// StartBackup triggers an async backup using pzstd, returning immediately with a backup ID.
func (c *client) StartBackup(serverName string) (string, error) {
	serverDir, err := c.SafePath(serverName)
	if err != nil {
		return "", err
	}
	if _, err := os.Stat(serverDir); os.IsNotExist(err) {
		return "", fmt.Errorf("server not found: %s", serverName)
	}

	backupDir, err := c.SafePath(serverName, backupsDir)
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(backupDir, 0755); err != nil {
		return "", fmt.Errorf("create backup dir: %w", err)
	}

	id := time.Now().UTC().Format("2006-01-02T15-04-05")
	backupFile := filepath.Join(backupDir, id+".tar.zst")

	status := &BackupStatus{
		Server:    serverName,
		ID:        id,
		Status:    "running",
		StartedAt: time.Now().UTC().Format(time.RFC3339),
	}
	if err := c.writeBackupStatus(serverName, status); err != nil {
		return "", fmt.Errorf("write backup status: %w", err)
	}

	go c.runBackup(serverName, serverDir, backupFile, id)

	return id, nil
}

func (c *client) runBackup(serverName, serverDir, backupFile, id string) {
	// tar the server directory (excluding the backups subdirectory) and pipe through pzstd.
	cmd := exec.Command("sh", "-c",
		fmt.Sprintf("tar --exclude='%s' -cf - -C '%s' . | pzstd -o '%s'",
			backupsDir, serverDir, backupFile))

	err := cmd.Run()

	c.mu.Lock()
	defer c.mu.Unlock()

	status := &BackupStatus{
		Server:      serverName,
		ID:          id,
		StartedAt:   time.Now().UTC().Format(time.RFC3339),
		CompletedAt: time.Now().UTC().Format(time.RFC3339),
	}
	if err != nil {
		status.Status = "failed"
		status.Error = err.Error()
		c.log.Error("backup failed", "server", serverName, "id", id, "error", err)
	} else {
		status.Status = "done"
		status.BackupPath = backupFile
		c.log.Info("backup completed", "server", serverName, "id", id, "path", backupFile)
	}
	if writeErr := c.writeBackupStatus(serverName, status); writeErr != nil {
		c.log.Error("failed to write backup status", "server", serverName, "error", writeErr)
	}
}

// GetBackupStatus reads the backup status file for a given server and backup ID.
func (c *client) GetBackupStatus(serverName, backupID string) (*BackupStatus, error) {
	statusFile := filepath.Join(c.dataDir, serverName+".backup-status")
	data, err := os.ReadFile(statusFile)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("backup status not found")
		}
		return nil, fmt.Errorf("read backup status: %w", err)
	}
	var status BackupStatus
	if err := json.Unmarshal(data, &status); err != nil {
		return nil, fmt.Errorf("parse backup status: %w", err)
	}
	if status.ID != backupID {
		return nil, fmt.Errorf("backup status not found")
	}
	return &status, nil
}

// Restore extracts a backup archive into the server directory.
func (c *client) Restore(serverName, backupID string) error {
	serverDir, err := c.SafePath(serverName)
	if err != nil {
		return err
	}
	backupFile, err := c.SafePath(serverName, backupsDir, backupID+".tar.zst")
	if err != nil {
		return err
	}
	if _, err := os.Stat(backupFile); os.IsNotExist(err) {
		return fmt.Errorf("backup not found: %s", backupID)
	}

	// Decompress and extract, excluding the backups directory itself.
	cmd := exec.Command("sh", "-c",
		fmt.Sprintf("pzstd -d '%s' --stdout | tar -xf - -C '%s' --exclude='%s'",
			backupFile, serverDir, backupsDir))
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("restore failed: %s: %w", string(out), err)
	}
	return nil
}

// Migrate renames a server directory from oldName to newName.
func (c *client) Migrate(oldName, newName string) error {
	oldDir, err := c.SafePath(oldName)
	if err != nil {
		return err
	}
	newDir, err := c.SafePath(newName)
	if err != nil {
		return err
	}
	if _, err := os.Stat(oldDir); os.IsNotExist(err) {
		return fmt.Errorf("server not found: %s", oldName)
	}
	if _, err := os.Stat(newDir); err == nil {
		return fmt.Errorf("target server already exists: %s", newName)
	}
	return os.Rename(oldDir, newDir)
}

func (c *client) writeBackupStatus(serverName string, status *BackupStatus) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if err := os.MkdirAll(c.dataDir, 0755); err != nil {
		return fmt.Errorf("create data dir: %w", err)
	}
	statusFile := filepath.Join(c.dataDir, serverName+".backup-status")
	data, err := json.MarshalIndent(status, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal backup status: %w", err)
	}
	return os.WriteFile(statusFile, data, 0644)
}
