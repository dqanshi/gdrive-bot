package download

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path"
	"strings"

	"gdrive-bot/internal/utils"
)

// DirectLink downloads url to localPath, streaming Progress updates.
// Works for any direct HTTP/HTTPS URL the user sends to the bot.
func DirectLink(ctx context.Context, rawURL, localPath string, progressCh chan<- Progress) error {
	defer close(progressCh)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return fmt.Errorf("download: build request: %w", err)
	}
	// Some hosts reject requests without a browser-like UA.
	req.Header.Set("User-Agent", "Mozilla/5.0 (compatible; GDriveBot/1.0)")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("download: request: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("download: server returned status %d", resp.StatusCode)
	}

	out, err := os.Create(localPath)
	if err != nil {
		return fmt.Errorf("download: create local file: %w", err)
	}
	defer out.Close()

	return streamWithProgress(ctx, resp.Body, out, resp.ContentLength, progressCh)
}

// SuggestFilename derives a local filename from a direct-link URL,
// falling back to a random name if the URL has no useful path segment
// (e.g. a bare query-string download endpoint).
func SuggestFilename(rawURL string) string {
	u, err := url.Parse(rawURL)
	if err != nil {
		return "file_" + utils.NewID()
	}
	base := path.Base(u.Path)
	if base == "" || base == "." || base == "/" || !strings.Contains(base, ".") {
		return "file_" + utils.NewID()
	}
	return utils.SafeFilename(base)
}

// IsDirectLink does a light heuristic check so the upload handler can
// tell a pasted URL apart from, say, the OAuth redirect URL pasted during
// /login (which also starts with http://).
func IsDirectLink(s string) bool {
	u, err := url.Parse(s)
	if err != nil {
		return false
	}
	return (u.Scheme == "http" || u.Scheme == "https") && u.Host != ""
}
