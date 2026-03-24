package nfs

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
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
	// Download fetches a file from url into destPath within a server directory.
	// If extract is true, zip/tar.gz/tar.zst archives are extracted to destPath.
	// All resulting files are chowned to uid:gid.
	// mode controls overwrite behavior: "overwrite" (default), "skip_existing", "clean_first".
	Download(serverName, url, destPath string, extract bool, uid, gid int, mode DownloadMode) (*DownloadResult, error)
	// ListArchiveContents lists file entries inside a zip or tar archive on the server filesystem.
	ListArchiveContents(serverName, path string) ([]ArchiveEntry, error)
	// WriteFile writes content to a file on the server filesystem, creating parent dirs as needed.
	WriteFile(serverName, path, content string, uid, gid int) error
	// MoveFile moves/renames a file or directory within a server's filesystem.
	MoveFile(serverName, srcPath, dstPath string) error
	// MaxWriteFileSize returns the configured maximum write file size.
	MaxWriteFileSize() int64
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

// DownloadResult holds the outcome of a file download operation.
type DownloadResult struct {
	FilesCount int   `json:"files_count"`
	TotalBytes int64 `json:"total_bytes"`
}

// DownloadMode controls how downloads handle existing files.
type DownloadMode string

const (
	// ModeOverwrite overwrites existing files (default behavior).
	ModeOverwrite DownloadMode = "overwrite"
	// ModeSkipExisting skips files that already exist at the destination.
	ModeSkipExisting DownloadMode = "skip_existing"
	// ModeCleanFirst deletes all contents in dest_path before extracting/copying.
	ModeCleanFirst DownloadMode = "clean_first"
)

// ValidDownloadMode returns true if mode is a recognised download mode.
func ValidDownloadMode(mode string) bool {
	switch DownloadMode(mode) {
	case ModeOverwrite, ModeSkipExisting, ModeCleanFirst:
		return true
	default:
		return false
	}
}

// ArchiveEntry represents a file entry inside an archive.
type ArchiveEntry struct {
	Name  string `json:"name"`
	Size  int64  `json:"size"`
	IsDir bool   `json:"is_dir"`
}

// backupIDRegex validates backup IDs to prevent shell injection. IDs are timestamps like "2026-03-22T10-05-00".
var backupIDRegex = regexp.MustCompile(`^[0-9T-]+$`)

const (
	maxFileReadSize = 1 << 20 // 1MB
	maxGrepLines    = 10000
	maxGrepBytes    = 5 * (1 << 20) // 5MB
	backupsDir      = "backups"

	maxDownloadSizeDefault  int64 = 2 * (1 << 30) // 2GB
	maxWriteFileSizeDefault int64 = 1 << 20       // 1MB
	maxExtractSizeDefault   int64 = 10737418240   // 10GB
	downloadTimeout               = 10 * time.Minute
)

type client struct {
	basePath         string
	dataDir          string
	log              *slog.Logger
	mu               sync.Mutex // protects backup status file writes
	maxDownloadSize  int64
	maxWriteFileSize int64
	maxExtractSize   int64
}

// NewClient creates a new NFS filesystem client.
func NewClient(basePath, dataDir string, log *slog.Logger, maxDownloadSize, maxWriteFileSize, maxExtractSize int64) Client {
	if maxDownloadSize <= 0 {
		maxDownloadSize = maxDownloadSizeDefault
	}
	if maxWriteFileSize <= 0 {
		maxWriteFileSize = maxWriteFileSizeDefault
	}
	if maxExtractSize <= 0 {
		maxExtractSize = maxExtractSizeDefault
	}
	return &client{
		basePath:         basePath,
		dataDir:          dataDir,
		log:              log,
		maxDownloadSize:  maxDownloadSize,
		maxWriteFileSize: maxWriteFileSize,
		maxExtractSize:   maxExtractSize,
	}
}

// MaxWriteFileSize returns the configured maximum write file size.
func (c *client) MaxWriteFileSize() int64 {
	return c.maxWriteFileSize
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
	// Resolve symlinks to prevent escaping the base path via symlink targets.
	// If the path doesn't exist yet (e.g., creating a new server dir), fall back to
	// the unresolved abs path — the prefix check below still applies.
	resolved, err := filepath.EvalSymlinks(abs)
	if err == nil {
		abs = resolved
	}
	// Ensure the resolved path is within basePath (or is basePath itself).
	base, err := filepath.Abs(c.basePath)
	if err != nil {
		return "", fmt.Errorf("resolve base path: %w", err)
	}
	// Also resolve symlinks on the base path for consistent comparison.
	if resolvedBase, err := filepath.EvalSymlinks(base); err == nil {
		base = resolvedBase
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
	// Use grep with recursive flag for directories. Timeout after 30s to prevent DoS.
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	args := []string{"-rn", "--", pattern, target}
	cmd := exec.CommandContext(ctx, "grep", args...)
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
	// Pipe tar output through pzstd without using sh -c to avoid shell injection.
	tarCmd := exec.Command("tar", "--exclude="+backupsDir, "-cf", "-", "-C", serverDir, ".")
	pzstdCmd := exec.Command("pzstd", "-o", backupFile)

	var err error
	pzstdCmd.Stdin, err = tarCmd.StdoutPipe()
	if err != nil {
		c.log.Error("backup pipe setup failed", "server", serverName, "id", id, "error", err)
		c.writeBackupStatusDirect(serverName, id, "failed", "", err.Error())
		return
	}

	if err = pzstdCmd.Start(); err != nil {
		c.log.Error("pzstd start failed", "server", serverName, "id", id, "error", err)
		c.writeBackupStatusDirect(serverName, id, "failed", "", err.Error())
		return
	}

	if err = tarCmd.Run(); err != nil {
		c.log.Error("tar failed", "server", serverName, "id", id, "error", err)
		_ = pzstdCmd.Wait()
		c.writeBackupStatusDirect(serverName, id, "failed", "", err.Error())
		return
	}

	if err = pzstdCmd.Wait(); err != nil {
		c.log.Error("pzstd failed", "server", serverName, "id", id, "error", err)
		c.writeBackupStatusDirect(serverName, id, "failed", "", err.Error())
		return
	}

	c.log.Info("backup completed", "server", serverName, "id", id, "path", backupFile)
	c.writeBackupStatusDirect(serverName, id, "done", backupFile, "")
}

// writeBackupStatusDirect writes backup status under the mutex without calling writeBackupStatus
// to avoid deadlock when called from runBackup.
func (c *client) writeBackupStatusDirect(serverName, id, statusStr, backupPath, errMsg string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if mkdirErr := os.MkdirAll(c.dataDir, 0755); mkdirErr != nil {
		c.log.Error("create data dir failed", "error", mkdirErr)
		return
	}

	status := &BackupStatus{
		Server:      serverName,
		ID:          id,
		Status:      statusStr,
		StartedAt:   time.Now().UTC().Format(time.RFC3339),
		CompletedAt: time.Now().UTC().Format(time.RFC3339),
		BackupPath:  backupPath,
		Error:       errMsg,
	}

	data, err := json.MarshalIndent(status, "", "  ")
	if err != nil {
		c.log.Error("marshal backup status failed", "error", err)
		return
	}

	statusFile := filepath.Join(c.dataDir, serverName+".backup-status")
	if err := os.WriteFile(statusFile, data, 0644); err != nil {
		c.log.Error("write backup status file failed", "error", err)
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
	if !backupIDRegex.MatchString(backupID) {
		return fmt.Errorf("invalid backup ID: %s", backupID)
	}
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

	// Pipe pzstd output through tar without using sh -c to avoid shell injection.
	pzstdCmd := exec.Command("pzstd", "-d", backupFile, "--stdout")
	tarCmd := exec.Command("tar", "-xf", "-", "-C", serverDir, "--exclude="+backupsDir)

	tarCmd.Stdin, err = pzstdCmd.StdoutPipe()
	if err != nil {
		return fmt.Errorf("pipe setup: %w", err)
	}

	if err = tarCmd.Start(); err != nil {
		return fmt.Errorf("tar start: %w", err)
	}

	if err = pzstdCmd.Run(); err != nil {
		_ = tarCmd.Wait()
		return fmt.Errorf("pzstd decompress: %w", err)
	}

	if err = tarCmd.Wait(); err != nil {
		return fmt.Errorf("tar extract: %w", err)
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

// Download fetches a file from url into destPath within a server directory.
// destPath is always treated as a directory (the downloaded file's name comes from the URL).
func (c *client) Download(serverName, rawURL, destPath string, extract bool, uid, gid int, mode DownloadMode) (*DownloadResult, error) {
	dest, err := c.SafePath(serverName, destPath)
	if err != nil {
		return nil, err
	}

	// If destPath looks like a file (has a file extension), use the parent directory
	// as the destination and ignore the filename — the actual filename comes from the URL.
	ext := filepath.Ext(dest)
	if ext != "" && ext != "." {
		dest = filepath.Dir(dest)
	}

	// Handle clean_first mode: remove all contents in dest before downloading.
	if mode == ModeCleanFirst {
		if err := os.MkdirAll(dest, 0755); err != nil {
			return nil, fmt.Errorf("create dest dir: %w", err)
		}
		entries, err := os.ReadDir(dest)
		if err != nil {
			return nil, fmt.Errorf("read dest dir for clean: %w", err)
		}
		for _, entry := range entries {
			if err := os.RemoveAll(filepath.Join(dest, entry.Name())); err != nil {
				return nil, fmt.Errorf("clean dest dir: %w", err)
			}
		}
	}

	// Ensure destination directory exists.
	if err := os.MkdirAll(dest, 0755); err != nil {
		return nil, fmt.Errorf("create dest dir: %w", err)
	}

	// Download to a temp file.
	tmpFile, err := os.CreateTemp("", "mc-download-*")
	if err != nil {
		return nil, fmt.Errorf("create temp file: %w", err)
	}
	tmpPath := tmpFile.Name()
	defer func() { _ = os.Remove(tmpPath) }()

	if err := c.downloadFile(rawURL, tmpFile); err != nil {
		_ = tmpFile.Close()
		return nil, err
	}
	if err := tmpFile.Close(); err != nil {
		return nil, fmt.Errorf("close temp file: %w", err)
	}

	var result *DownloadResult
	if extract {
		result, err = c.extractArchive(tmpPath, dest, rawURL, mode)
		if err != nil {
			return nil, fmt.Errorf("extract: %w", err)
		}
	} else {
		result, err = c.moveFile(tmpPath, dest, rawURL, mode)
		if err != nil {
			return nil, fmt.Errorf("move file: %w", err)
		}
	}

	// Recursively chown all files to uid:gid.
	if err := chownRecursive(dest, uid, gid); err != nil {
		return nil, fmt.Errorf("chown: %w", err)
	}

	return result, nil
}

// downloadFile fetches rawURL into the provided file with a size limit and timeout.
func (c *client) downloadFile(rawURL string, dst *os.File) error {
	ctx, cancel := context.WithTimeout(context.Background(), downloadTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}

	httpClient := &http.Client{
		Timeout: downloadTimeout,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			host := req.URL.Hostname()
			if isPrivateIP(host) {
				return fmt.Errorf("redirect to private IP %s blocked", host)
			}
			if len(via) >= 5 {
				return fmt.Errorf("too many redirects")
			}
			return nil
		},
	}

	resp, err := httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("download: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("download returned status %d", resp.StatusCode)
	}

	limited := io.LimitReader(resp.Body, c.maxDownloadSize+1)
	n, err := io.Copy(dst, limited)
	if err != nil {
		return fmt.Errorf("download write: %w", err)
	}
	if n > c.maxDownloadSize {
		return fmt.Errorf("download exceeds maximum size of %d bytes", c.maxDownloadSize)
	}

	return nil
}

// isPrivateIP checks if a hostname resolves to a private/loopback IP address.
func isPrivateIP(host string) bool {
	// Check for localhost explicitly.
	if strings.EqualFold(host, "localhost") {
		return true
	}

	// Check for metadata service IP.
	if host == "169.254.169.254" {
		return true
	}

	ips, err := net.LookupIP(host)
	if err != nil {
		// If we can't resolve, check if it's a raw IP.
		ip := net.ParseIP(host)
		if ip == nil {
			return false
		}
		ips = []net.IP{ip}
	}

	privateRanges := []struct {
		network *net.IPNet
	}{
		{network: mustParseCIDR("127.0.0.0/8")},
		{network: mustParseCIDR("10.0.0.0/8")},
		{network: mustParseCIDR("172.16.0.0/12")},
		{network: mustParseCIDR("192.168.0.0/16")},
	}

	for _, ip := range ips {
		// Check IPv6 loopback.
		if ip.IsLoopback() {
			return true
		}
		for _, r := range privateRanges {
			if r.network.Contains(ip) {
				return true
			}
		}
	}

	return false
}

// mustParseCIDR parses a CIDR string and panics on failure. Used for static initialization.
func mustParseCIDR(s string) *net.IPNet {
	_, network, err := net.ParseCIDR(s)
	if err != nil {
		panic(fmt.Sprintf("invalid CIDR %q: %v", s, err))
	}
	return network
}

// filenameFromURL extracts the filename from a URL path.
func filenameFromURL(rawURL string) string {
	u, err := http.NewRequest(http.MethodGet, rawURL, nil)
	if err != nil {
		return "download"
	}
	name := filepath.Base(u.URL.Path)
	if name == "" || name == "." || name == "/" {
		return "download"
	}
	return name
}

// moveFile moves the downloaded temp file to the destination directory.
func (c *client) moveFile(tmpPath, destDir, rawURL string, mode DownloadMode) (*DownloadResult, error) {
	filename := filenameFromURL(rawURL)
	destFile := filepath.Join(destDir, filename)

	// In skip_existing mode, check if file already exists.
	if mode == ModeSkipExisting {
		if _, err := os.Stat(destFile); err == nil {
			return &DownloadResult{FilesCount: 0, TotalBytes: 0}, nil
		}
	}

	info, err := os.Stat(tmpPath)
	if err != nil {
		return nil, fmt.Errorf("stat temp file: %w", err)
	}

	// Copy instead of rename to handle cross-device moves.
	src, err := os.Open(tmpPath)
	if err != nil {
		return nil, fmt.Errorf("open temp file: %w", err)
	}
	defer func() { _ = src.Close() }()

	dst, err := os.Create(destFile)
	if err != nil {
		return nil, fmt.Errorf("create dest file: %w", err)
	}
	defer func() { _ = dst.Close() }()

	if _, err := io.Copy(dst, src); err != nil {
		return nil, fmt.Errorf("copy file: %w", err)
	}

	return &DownloadResult{
		FilesCount: 1,
		TotalBytes: info.Size(),
	}, nil
}

// extractArchive detects the archive type and extracts to destDir.
func (c *client) extractArchive(tmpPath, destDir, rawURL string, mode DownloadMode) (*DownloadResult, error) {
	filename := strings.ToLower(filenameFromURL(rawURL))

	switch {
	case strings.HasSuffix(filename, ".zip"):
		return extractZip(tmpPath, destDir, mode, c.maxExtractSize)
	case strings.HasSuffix(filename, ".tar.gz") || strings.HasSuffix(filename, ".tgz"):
		return extractTarGz(tmpPath, destDir, c.maxExtractSize)
	case strings.HasSuffix(filename, ".tar.zst") || strings.HasSuffix(filename, ".tar.zstd"):
		return extractTarZst(tmpPath, destDir, c.maxExtractSize)
	default:
		return nil, fmt.Errorf("unsupported archive format: %s", filename)
	}
}

// extractZip extracts a .zip archive to destDir using archive/zip stdlib.
func extractZip(zipPath, destDir string, mode DownloadMode, maxExtractSize int64) (*DownloadResult, error) {
	r, err := zip.OpenReader(zipPath)
	if err != nil {
		return nil, fmt.Errorf("open zip: %w", err)
	}
	defer func() { _ = r.Close() }()

	var filesCount int
	var totalBytes int64

	for _, f := range r.File {
		// Skip symlink entries.
		if f.Mode()&os.ModeSymlink != 0 {
			continue
		}

		// Prevent zip slip.
		target := filepath.Join(destDir, f.Name)
		rel, err := filepath.Rel(destDir, target)
		if err != nil || strings.HasPrefix(rel, "..") {
			return nil, fmt.Errorf("zip entry %q escapes destination directory", f.Name)
		}

		if f.FileInfo().IsDir() {
			if err := os.MkdirAll(target, 0755); err != nil {
				return nil, fmt.Errorf("create dir %q: %w", f.Name, err)
			}
			continue
		}

		// In skip_existing mode, skip files that already exist.
		if mode == ModeSkipExisting {
			if _, err := os.Stat(target); err == nil {
				continue
			}
		}

		// Ensure parent directory exists.
		if err := os.MkdirAll(filepath.Dir(target), 0755); err != nil {
			return nil, fmt.Errorf("create parent dir: %w", err)
		}

		rc, err := f.Open()
		if err != nil {
			return nil, fmt.Errorf("open zip entry %q: %w", f.Name, err)
		}

		out, err := os.Create(target)
		if err != nil {
			_ = rc.Close()
			return nil, fmt.Errorf("create file %q: %w", f.Name, err)
		}

		n, err := io.Copy(out, rc)
		_ = rc.Close()
		_ = out.Close()
		if err != nil {
			return nil, fmt.Errorf("extract %q: %w", f.Name, err)
		}

		filesCount++
		totalBytes += n

		if totalBytes > maxExtractSize {
			return nil, fmt.Errorf("extracted size exceeds maximum of %d bytes", maxExtractSize)
		}
	}

	return &DownloadResult{FilesCount: filesCount, TotalBytes: totalBytes}, nil
}

// extractTarGz extracts a .tar.gz archive to destDir using compress/gzip + archive/tar in Go.
func extractTarGz(tgzPath, destDir string, maxExtractSize int64) (*DownloadResult, error) {
	f, err := os.Open(tgzPath)
	if err != nil {
		return nil, fmt.Errorf("open tar.gz: %w", err)
	}
	defer func() { _ = f.Close() }()

	gr, err := gzip.NewReader(f)
	if err != nil {
		return nil, fmt.Errorf("gzip reader: %w", err)
	}
	defer func() { _ = gr.Close() }()

	// Limit the decompressed stream to prevent gzip bombs.
	limited := io.LimitReader(gr, maxExtractSize+1)

	return extractTarReader(limited, destDir, maxExtractSize)
}

// extractTarZst extracts a .tar.zst archive to destDir using pzstd for decompression + archive/tar in Go.
func extractTarZst(zstPath, destDir string, maxExtractSize int64) (*DownloadResult, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	pzstdCmd := exec.CommandContext(ctx, "pzstd", "-d", zstPath, "--stdout")
	stdout, err := pzstdCmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("pzstd pipe: %w", err)
	}

	if err = pzstdCmd.Start(); err != nil {
		return nil, fmt.Errorf("pzstd start: %w", err)
	}

	result, tarErr := extractTarReader(stdout, destDir, maxExtractSize)

	if err = pzstdCmd.Wait(); err != nil && tarErr == nil {
		return nil, fmt.Errorf("pzstd decompress: %w", err)
	}
	if tarErr != nil {
		return nil, tarErr
	}

	return result, nil
}

// extractTarReader extracts tar entries from r into destDir with path traversal and size limit checks.
func extractTarReader(r io.Reader, destDir string, maxExtractSize int64) (*DownloadResult, error) {
	tr := tar.NewReader(r)
	var filesCount int
	var totalBytes int64

	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("read tar header: %w", err)
		}

		// Skip symlinks and hard links.
		if hdr.Typeflag == tar.TypeSymlink || hdr.Typeflag == tar.TypeLink {
			continue
		}

		// Skip entries with absolute paths.
		if filepath.IsAbs(hdr.Name) {
			continue
		}

		// Validate the path stays within destDir.
		target := filepath.Join(destDir, hdr.Name)
		rel, err := filepath.Rel(destDir, target)
		if err != nil || strings.HasPrefix(rel, "..") {
			return nil, fmt.Errorf("tar entry %q escapes destination directory", hdr.Name)
		}

		switch hdr.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(target, 0755); err != nil {
				return nil, fmt.Errorf("create dir %q: %w", hdr.Name, err)
			}
		case tar.TypeReg:
			// Ensure parent directory exists.
			if err := os.MkdirAll(filepath.Dir(target), 0755); err != nil {
				return nil, fmt.Errorf("create parent dir: %w", err)
			}

			out, err := os.Create(target)
			if err != nil {
				return nil, fmt.Errorf("create file %q: %w", hdr.Name, err)
			}

			n, copyErr := io.Copy(out, tr)
			_ = out.Close()
			if copyErr != nil {
				return nil, fmt.Errorf("extract %q: %w", hdr.Name, copyErr)
			}

			filesCount++
			totalBytes += n

			if totalBytes > maxExtractSize {
				return nil, fmt.Errorf("extracted size exceeds maximum of %d bytes", maxExtractSize)
			}
		}
	}

	return &DownloadResult{FilesCount: filesCount, TotalBytes: totalBytes}, nil
}

// ListArchiveContents lists file entries inside a zip or tar archive on the server filesystem.
func (c *client) ListArchiveContents(serverName, archivePath string) ([]ArchiveEntry, error) {
	fullPath, err := c.SafePath(serverName, archivePath)
	if err != nil {
		return nil, err
	}

	lower := strings.ToLower(archivePath)
	switch {
	case strings.HasSuffix(lower, ".zip"):
		return listZipContents(fullPath)
	case strings.HasSuffix(lower, ".tar.gz") || strings.HasSuffix(lower, ".tgz"):
		return listTarGzContents(fullPath)
	case strings.HasSuffix(lower, ".tar.zst") || strings.HasSuffix(lower, ".tar.zstd"):
		return listTarZstContents(fullPath)
	default:
		return nil, fmt.Errorf("unsupported archive format: %s", filepath.Base(archivePath))
	}
}

// listZipContents lists entries inside a zip file.
func listZipContents(zipPath string) ([]ArchiveEntry, error) {
	r, err := zip.OpenReader(zipPath)
	if err != nil {
		return nil, fmt.Errorf("open zip: %w", err)
	}
	defer func() { _ = r.Close() }()

	var entries []ArchiveEntry
	for _, f := range r.File {
		entries = append(entries, ArchiveEntry{
			Name:  f.Name,
			Size:  int64(f.UncompressedSize64),
			IsDir: f.FileInfo().IsDir(),
		})
	}
	return entries, nil
}

// listTarGzContents lists entries inside a .tar.gz file.
func listTarGzContents(tgzPath string) ([]ArchiveEntry, error) {
	f, err := os.Open(tgzPath)
	if err != nil {
		return nil, fmt.Errorf("open tar.gz: %w", err)
	}
	defer func() { _ = f.Close() }()

	gr, err := gzip.NewReader(f)
	if err != nil {
		return nil, fmt.Errorf("gzip reader: %w", err)
	}
	defer func() { _ = gr.Close() }()

	return listTarEntries(gr)
}

// listTarZstContents lists entries inside a .tar.zst file using pzstd to decompress.
func listTarZstContents(zstPath string) ([]ArchiveEntry, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	cmd := exec.CommandContext(ctx, "pzstd", "-d", zstPath, "--stdout")
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("pzstd pipe: %w", err)
	}
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("pzstd start: %w", err)
	}

	entries, tarErr := listTarEntries(stdout)

	if err := cmd.Wait(); err != nil && tarErr == nil {
		return nil, fmt.Errorf("pzstd: %w", err)
	}
	if tarErr != nil {
		return nil, tarErr
	}
	return entries, nil
}

// listTarEntries reads tar headers from a reader and returns archive entries.
func listTarEntries(r io.Reader) ([]ArchiveEntry, error) {
	tr := tar.NewReader(r)
	var entries []ArchiveEntry
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("read tar header: %w", err)
		}
		entries = append(entries, ArchiveEntry{
			Name:  hdr.Name,
			Size:  hdr.Size,
			IsDir: hdr.Typeflag == tar.TypeDir,
		})
	}
	return entries, nil
}

// WriteFile writes content to a file on the server filesystem, creating parent dirs as needed.
func (c *client) WriteFile(serverName, filePath, content string, uid, gid int) error {
	if int64(len(content)) > c.maxWriteFileSize {
		return fmt.Errorf("content size %d exceeds maximum of %d bytes", len(content), c.maxWriteFileSize)
	}

	fullPath, err := c.SafePath(serverName, filePath)
	if err != nil {
		return err
	}

	// Create parent directories as needed.
	if err := os.MkdirAll(filepath.Dir(fullPath), 0755); err != nil {
		return fmt.Errorf("create parent dirs: %w", err)
	}

	if err := os.WriteFile(fullPath, []byte(content), 0644); err != nil {
		return fmt.Errorf("write file: %w", err)
	}

	if err := os.Chown(fullPath, uid, gid); err != nil {
		return fmt.Errorf("chown file: %w", err)
	}

	return nil
}

// MoveFile moves/renames a file or directory within a server's filesystem.
func (c *client) MoveFile(serverName, srcPath, dstPath string) error {
	src, err := c.SafePath(serverName, srcPath)
	if err != nil {
		return err
	}
	dst, err := c.SafePath(serverName, dstPath)
	if err != nil {
		return err
	}

	if _, err := os.Stat(src); os.IsNotExist(err) {
		return fmt.Errorf("source not found: %s", srcPath)
	}

	// Create parent directory of destination.
	if err := os.MkdirAll(filepath.Dir(dst), 0755); err != nil {
		return fmt.Errorf("create dest parent: %w", err)
	}

	return os.Rename(src, dst)
}

// chownRecursive sets ownership of all files and directories under root to uid:gid.
func chownRecursive(root string, uid, gid int) error {
	return filepath.Walk(root, func(path string, _ os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		return os.Chown(path, uid, gid)
	})
}
