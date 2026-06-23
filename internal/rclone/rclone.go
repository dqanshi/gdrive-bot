// Package rclone wraps the rclone binary: creating a per-user "drive"
// remote backed by their OAuth token, and running optimized `rclone copy`
// uploads (multi-thread streams, chunked transfer, retries) with parsed
// progress output.
//
// We use rclone specifically for the transfer engine (it is dramatically
// better at large, resumable, multi-threaded uploads than a hand-rolled
// HTTP client). Everything interactive — listing, rename, delete, sharing
// — goes through the Drive API directly in internal/drive, since rclone's
// CLI is awkward for single-file metadata operations and a live bot needs
// those to be fast and structured.
package rclone

import (
	"bufio"
	"context"
	"fmt"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"sync"

	"gdrive-bot/internal/models"
)

// Manager runs rclone commands against a shared config file on disk.
type Manager struct {
	binPath    string
	configPath string
	mu         sync.Mutex // rclone config writes aren't safe for concurrent access
}

func NewManager(binPath, configPath string) *Manager {
	return &Manager{binPath: binPath, configPath: configPath}
}

// RemoteName derives a stable, valid rclone remote name for a Telegram
// user ID, e.g. "user_111111111".
func RemoteName(telegramID int64) string {
	return fmt.Sprintf("user_%d", telegramID)
}

// CreateOrUpdateRemote writes/updates a [user_<id>] section in the rclone
// config pointing at the given OAuth credentials, equivalent to:
//
//	rclone config create user_123 drive client_id=... client_secret=... \
//	  token='{"access_token":...}' root_folder_id=...
func (m *Manager) CreateOrUpdateRemote(ctx context.Context, remote, clientID, clientSecret string, token *models.GoogleToken, rootFolderID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	tokenJSON := fmt.Sprintf(
		`{"access_token":%q,"token_type":%q,"refresh_token":%q,"expiry":%q}`,
		token.AccessToken, token.TokenType, token.RefreshToken, token.Expiry.Format("2006-01-02T15:04:05.999999999Z07:00"),
	)

	args := []string{
		"config", "create", remote, "drive",
		"client_id", clientID,
		"client_secret", clientSecret,
		"scope", "drive",
		"token", tokenJSON,
	}
	if rootFolderID != "" {
		args = append(args, "root_folder_id", rootFolderID)
	}

	cmd := exec.CommandContext(ctx, m.binPath, m.withConfigFlag(args)...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("rclone: create remote: %w (output: %s)", err, string(out))
	}
	return nil
}

// RemoveRemote deletes a user's rclone remote on /logout.
func (m *Manager) RemoveRemote(ctx context.Context, remote string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	cmd := exec.CommandContext(ctx, m.binPath, m.withConfigFlag([]string{"config", "delete", remote})...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("rclone: remove remote: %w (output: %s)", err, string(out))
	}
	return nil
}

func (m *Manager) withConfigFlag(args []string) []string {
	return append([]string{"--config", m.configPath}, args...)
}

// UploadProgress is reported periodically while an upload runs.
type UploadProgress struct {
	BytesDone    int64
	BytesTotal   int64
	SpeedBps     float64
	ETASeconds   int64
	Percent      float64
	Done         bool
	Err          error
}

// transferredRe parses rclone's `--progress` stat line, e.g.:
// "Transferred:   	  120.5 MiB / 512.0 MiB, 24%, 5.123 MiB/s, ETA 1m18s"
var transferredRe = regexp.MustCompile(`Transferred:\s+([\d.]+)\s*(\wiB)\s*/\s*([\d.]+)\s*(\wiB),\s*(\d+)%,\s*([\d.]+)\s*(\wiB)/s(?:,\s*ETA\s*(\S+))?`)

// Upload runs `rclone copy <localPath> <remote>:<destPath>` with
// production-grade transfer flags (multi-thread streams, chunked
// transfers, retries, parallel transfers) and streams parsed progress to
// progressCh until completion. The caller is responsible for draining
// progressCh promptly and closing nothing (Upload closes it).
func (m *Manager) Upload(ctx context.Context, localPath, remote, destFolderID string, maxRetries int, progressCh chan<- UploadProgress) error {
	defer close(progressCh)

	dest := fmt.Sprintf("%s:", remote)

	args := []string{
		"copy", localPath, dest,
		"--config", m.configPath,
		"--progress",
		"--stats", "1s",
		"--stats-one-line",
		"--transfers", "4",
		"--checkers", "8",
		"--drive-chunk-size", "64M",
		"--drive-upload-cutoff", "64M",
		"--retries", strconv.Itoa(maxRetries),
		"--low-level-retries", "10",
		"--retries-sleep", "5s",
	}
	if destFolderID != "" {
		args = append(args, "--drive-root-folder-id", destFolderID)
	}

	cmd := exec.CommandContext(ctx, m.binPath, args...)
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return fmt.Errorf("rclone: stderr pipe: %w", err)
	}
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("rclone: start upload: %w", err)
	}

	scanner := bufio.NewScanner(stderr)
	scanner.Buffer(make([]byte, 64*1024), 1024*1024)
	for scanner.Scan() {
		line := scanner.Text()
		if p, ok := parseProgressLine(line); ok {
			select {
			case progressCh <- p:
			case <-ctx.Done():
			}
		}
	}

	if err := cmd.Wait(); err != nil {
		progressCh <- UploadProgress{Done: true, Err: err}
		return fmt.Errorf("rclone: upload failed: %w", err)
	}
	progressCh <- UploadProgress{Done: true, Percent: 100}
	return nil
}

func parseProgressLine(line string) (UploadProgress, bool) {
	m := transferredRe.FindStringSubmatch(line)
	if m == nil {
		return UploadProgress{}, false
	}
	doneVal, _ := strconv.ParseFloat(m[1], 64)
	totalVal, _ := strconv.ParseFloat(m[3], 64)
	pct, _ := strconv.ParseFloat(m[5], 64)
	speedVal, _ := strconv.ParseFloat(m[6], 64)

	return UploadProgress{
		BytesDone:  int64(doneVal * unitMultiplier(m[2])),
		BytesTotal: int64(totalVal * unitMultiplier(m[4])),
		SpeedBps:   speedVal * unitMultiplier(m[7]),
		Percent:    pct,
		ETASeconds: parseETA(m[8]),
	}, true
}

func unitMultiplier(unit string) float64 {
	switch strings.ToUpper(unit) {
	case "KIB":
		return 1024
	case "MIB":
		return 1024 * 1024
	case "GIB":
		return 1024 * 1024 * 1024
	case "TIB":
		return 1024 * 1024 * 1024 * 1024
	default:
		return 1
	}
}

// parseETA parses rclone's compact duration strings like "1m18s" or "45s"
// into total seconds. Returns 0 if eta is empty or unparseable (e.g. "-").
func parseETA(eta string) int64 {
	if eta == "" || eta == "-" {
		return 0
	}
	var total int64
	re := regexp.MustCompile(`(\d+)([hms])`)
	for _, m := range re.FindAllStringSubmatch(eta, -1) {
		n, _ := strconv.ParseInt(m[1], 10, 64)
		switch m[2] {
		case "h":
			total += n * 3600
		case "m":
			total += n * 60
		case "s":
			total += n
		}
	}
	return total
}
