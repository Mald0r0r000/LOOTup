package transfer

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/cespare/xxhash/v2"
	"github.com/pkg/sftp"
	"golang.org/x/crypto/ssh"
)

// FileInfo holds metadata about a file to transfer
type FileInfo struct {
	LocalPath  string
	RelPath    string
	Size       int64
}

// Result holds the outcome of a single file transfer
type Result struct {
	File      FileInfo
	Hash      uint64
	BytesSent int64
	Err       error
	Verified  bool
}

// ProgressMsg is sent to report transfer progress
type ProgressMsg struct {
	File       FileInfo
	BytesSent  int64
	TotalBytes int64
	Done       bool
	Err        error
	Hash       uint64
}

// Transferer manages SFTP file transfers with parallel workers
type Transferer struct {
	sftpClient  *sftp.Client
	sshClient   *ssh.Client
	source      string
	destPath    string
	concurrency int
	files       []FileInfo
	totalBytes  int64
	progressCh  chan ProgressMsg
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

// Walk scans the source directory and builds the file list
func (t *Transferer) Walk() error {
	t.files = nil
	t.totalBytes = 0

	return filepath.Walk(t.source, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		// Skip directories and system files
		if info.IsDir() {
			base := filepath.Base(path)
			if strings.HasPrefix(base, ".") && path != t.source {
				return filepath.SkipDir
			}
			return nil
		}

		// Skip hidden/system files
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

// Run starts the parallel transfer. It blocks until all files are transferred.
func (t *Transferer) Run() error {
	defer close(t.progressCh)

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

// transferFile copies a single file via SFTP with on-the-fly xxHash64 via io.TeeReader
func (t *Transferer) transferFile(fi FileInfo) error {
	// Open local file
	srcFile, err := os.Open(fi.LocalPath)
	if err != nil {
		return fmt.Errorf("open %s: %w", fi.LocalPath, err)
	}
	defer srcFile.Close()

	// Ensure remote directory exists
	remotePath := filepath.Join(t.destPath, fi.RelPath)
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
	Source     string
	DestPath  string
	Files     []FileInfo
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
// If xxhsum is not available, returns false without error (trusts local hash).
func (t *Transferer) VerifyRemote(remotePath string, expectedHash uint64) (bool, error) {
	session, err := t.sshClient.NewSession()
	if err != nil {
		return false, fmt.Errorf("ssh session: %w", err)
	}
	defer session.Close()

	// Try xxhsum — if not found, skip verification
	cmd := fmt.Sprintf("xxhsum %s 2>/dev/null || echo SKIP", remotePath)
	output, err := session.CombinedOutput(cmd)
	if err != nil {
		return false, nil // Skip verification
	}

	result := strings.TrimSpace(string(output))
	if result == "SKIP" || result == "" {
		return false, nil // xxhsum not available, skip
	}

	// Parse xxhsum output: "HASH  filename"
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
