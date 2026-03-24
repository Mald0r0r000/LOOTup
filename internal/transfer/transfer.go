package transfer

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/cespare/xxhash/v2"
	"github.com/pkg/sftp"
	"golang.org/x/crypto/ssh"
)

// FileInfo holds metadata about a file to transfer
type FileInfo struct {
	LocalPath string
	RelPath   string
	Size      int64
}

// ProgressMsg is sent to report transfer progress
type ProgressMsg struct {
	File       FileInfo
	BytesSent  int64
	TotalBytes int64
	Done       bool
	Skipped    bool
	Err        error
	Hash       uint64
}

// HashEntry records a per-file hash for verification log
type HashEntry struct {
	RelPath string `json:"rel_path"`
	Hash    string `json:"hash_xxh64"`
	Size    int64  `json:"size"`
}

// Transferer manages SFTP file transfers with parallel workers
type Transferer struct {
	sftpClient  *sftp.Client
	sshClient   *ssh.Client
	source      string
	destPath    string
	concurrency int
	resumeMode  bool
	files       []FileInfo
	totalBytes  int64
	progressCh  chan ProgressMsg
	startTime   time.Time

	// Hash log — populated during transfer
	mu         sync.Mutex
	HashLog    []HashEntry
	SkippedN   int64
	SkippedB   int64
}

// New creates a new Transferer
func New(sshConn *ssh.Client, source, destPath string, concurrency int) (*Transferer, error) {
	sftpClient, err := sftp.NewClient(sshConn)
	if err != nil {
		return nil, fmt.Errorf("sftp client: %w", err)
	}

	return &Transferer{
		sftpClient:  sftpClient,
		sshClient:   sshConn,
		source:      source,
		destPath:    destPath,
		concurrency: concurrency,
		progressCh:  make(chan ProgressMsg, 100),
	}, nil
}

// SetResumeMode enables skip-existing for resume transfers
func (t *Transferer) SetResumeMode(resume bool) {
	t.resumeMode = resume
}

// Walk scans the source directory and builds the file list
func (t *Transferer) Walk() error {
	t.files = nil
	t.totalBytes = 0

	return filepath.Walk(t.source, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		if info.IsDir() {
			base := filepath.Base(path)
			if strings.HasPrefix(base, ".") && path != t.source {
				return filepath.SkipDir
			}
			return nil
		}

		base := filepath.Base(path)
		if strings.HasPrefix(base, ".") {
			return nil
		}

		relPath, err := filepath.Rel(t.source, path)
		if err != nil {
			return err
		}

		fi := FileInfo{
			LocalPath: path,
			RelPath:   relPath,
			Size:      info.Size(),
		}
		t.files = append(t.files, fi)
		t.totalBytes += info.Size()
		return nil
	})
}

// Files returns the list of files to transfer
func (t *Transferer) Files() []FileInfo {
	return t.files
}

// TotalBytes returns total bytes to transfer
func (t *Transferer) TotalBytes() int64 {
	return t.totalBytes
}

// Progress returns the progress channel
func (t *Transferer) Progress() <-chan ProgressMsg {
	return t.progressCh
}

// StartTime returns when transfer began
func (t *Transferer) StartTime() time.Time {
	return t.startTime
}

// Run starts the parallel transfer. It blocks until all files are transferred.
func (t *Transferer) Run() error {
	defer close(t.progressCh)
	t.startTime = time.Now()

	fileCh := make(chan FileInfo, len(t.files))
	for _, f := range t.files {
		fileCh <- f
	}
	close(fileCh)

	var wg sync.WaitGroup
	var firstErr atomic.Value

	for i := 0; i < t.concurrency; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for fi := range fileCh {
				if err := t.transferFile(fi); err != nil {
					firstErr.CompareAndSwap(nil, err)
					t.progressCh <- ProgressMsg{
						File: fi,
						Done: true,
						Err:  err,
					}
				}
			}
		}()
	}

	wg.Wait()

	if v := firstErr.Load(); v != nil {
		return v.(error)
	}
	return nil
}

// transferFile copies a single file via SFTP with on-the-fly xxHash64
func (t *Transferer) transferFile(fi FileInfo) error {
	remotePath := filepath.Join(t.destPath, fi.RelPath)

	// Resume mode: skip if remote file exists with same size
	if t.resumeMode {
		if remoteStat, err := t.sftpClient.Stat(remotePath); err == nil {
			if remoteStat.Size() == fi.Size {
				// File already transferred — skip
				t.mu.Lock()
				t.SkippedN++
				t.SkippedB += fi.Size
				t.mu.Unlock()

				t.progressCh <- ProgressMsg{
					File:       fi,
					BytesSent:  fi.Size,
					TotalBytes: fi.Size,
					Done:       true,
					Skipped:    true,
				}
				return nil
			}
		}
	}

	// Open local file
	srcFile, err := os.Open(fi.LocalPath)
	if err != nil {
		return fmt.Errorf("open %s: %w", fi.LocalPath, err)
	}
	defer srcFile.Close()

	// Ensure remote directory exists
	remoteDir := filepath.Dir(remotePath)
	if err := t.sftpClient.MkdirAll(remoteDir); err != nil {
		return fmt.Errorf("mkdir %s: %w", remoteDir, err)
	}

	// Create remote file
	dstFile, err := t.sftpClient.Create(remotePath)
	if err != nil {
		return fmt.Errorf("create remote %s: %w", remotePath, err)
	}
	defer dstFile.Close()

	// Hash on-the-fly: pipe through io.TeeReader(srcFile, xxhash)
	hasher := xxhash.New()
	teeReader := io.TeeReader(srcFile, hasher)

	// Wrap in a progress-reporting reader
	pr := &progressReader{
		reader: teeReader,
		file:   fi,
		ch:     t.progressCh,
	}

	// Copy
	written, err := io.Copy(dstFile, pr)
	if err != nil {
		return fmt.Errorf("copy %s: %w", fi.RelPath, err)
	}

	localHash := hasher.Sum64()
	hashStr := fmt.Sprintf("%016x", localHash)

	// Record hash in log
	t.mu.Lock()
	t.HashLog = append(t.HashLog, HashEntry{
		RelPath: fi.RelPath,
		Hash:    hashStr,
		Size:    written,
	})
	t.mu.Unlock()

	// Report completion
	t.progressCh <- ProgressMsg{
		File:       fi,
		BytesSent:  written,
		TotalBytes: fi.Size,
		Done:       true,
		Hash:       localHash,
	}

	return nil
}

// DryRunResult holds dry run summary
type DryRunResult struct {
	Source    string
	DestPath string
	Files    []FileInfo
	TotalSize int64
}

// DryRun simulates the transfer without copying
func (t *Transferer) DryRun() (*DryRunResult, error) {
	if err := t.Walk(); err != nil {
		return nil, err
	}
	return &DryRunResult{
		Source:    t.source,
		DestPath:  t.destPath,
		Files:     t.files,
		TotalSize: t.totalBytes,
	}, nil
}

// VerifyRemote tries to verify a file's hash on the remote host via SSH.
func (t *Transferer) VerifyRemote(remotePath string, expectedHash uint64) (bool, error) {
	session, err := t.sshClient.NewSession()
	if err != nil {
		return false, fmt.Errorf("ssh session: %w", err)
	}
	defer session.Close()

	cmd := fmt.Sprintf("xxhsum %s 2>/dev/null || echo SKIP", remotePath)
	output, err := session.CombinedOutput(cmd)
	if err != nil {
		return false, nil
	}

	result := strings.TrimSpace(string(output))
	if result == "SKIP" || result == "" {
		return false, nil
	}

	parts := strings.Fields(result)
	if len(parts) < 1 {
		return false, nil
	}

	var remoteHash uint64
	if _, err := fmt.Sscanf(parts[0], "%x", &remoteHash); err != nil {
		return false, nil
	}

	return remoteHash == expectedHash, nil
}

// Close closes the SFTP client
func (t *Transferer) Close() error {
	return t.sftpClient.Close()
}

// FormatBytes returns a human-readable byte size
func FormatBytes(b int64) string {
	const unit = 1024
	if b < unit {
		return fmt.Sprintf("%d B", b)
	}
	div, exp := int64(unit), 0
	for n := b / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(b)/float64(div), "KMGTPE"[exp])
}

// FormatSpeed returns human-readable speed (e.g. "125.3 MB/s")
func FormatSpeed(bytesPerSec float64) string {
	return FormatBytes(int64(bytesPerSec)) + "/s"
}

// FormatDuration returns a compact duration (e.g. "2m 30s")
func FormatDuration(d time.Duration) string {
	d = d.Round(time.Second)
	h := int(d.Hours())
	m := int(d.Minutes()) % 60
	s := int(d.Seconds()) % 60
	if h > 0 {
		return fmt.Sprintf("%dh %02dm", h, m)
	}
	if m > 0 {
		return fmt.Sprintf("%dm %02ds", m, s)
	}
	return fmt.Sprintf("%ds", s)
}

// progressReader wraps a reader and reports progress
type progressReader struct {
	reader    io.Reader
	file      FileInfo
	ch        chan ProgressMsg
	bytesRead int64
}

func (pr *progressReader) Read(p []byte) (int, error) {
	n, err := pr.reader.Read(p)
	pr.bytesRead += int64(n)

	if n > 0 {
		pr.ch <- ProgressMsg{
			File:       pr.file,
			BytesSent:  pr.bytesRead,
			TotalBytes: pr.file.Size,
		}
	}

	return n, err
}
