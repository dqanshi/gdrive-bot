// Package drive wraps the Google Drive API v3 for everything interactive:
// listing folders, search, rename, delete, and sharing. Bulk file
// transfer is handled separately by internal/rclone, which is better
// suited to large multi-threaded uploads.
package drive

import (
	"context"
	"fmt"

	"golang.org/x/oauth2"
	"google.golang.org/api/drive/v3"
	"google.golang.org/api/option"
)

// Client wraps a *drive.Service for one user.
type Client struct {
	svc *drive.Service
}

// NewClient builds a Drive API client from an already-refreshing
// oauth2.TokenSource. Callers obtain that token source via
// auth.Manager.TokenSource, which persists refreshed access tokens back
// to MongoDB through the onRefresh callback passed there — so this layer
// never has to think about token storage.
func NewClient(ctx context.Context, ts oauth2.TokenSource) (*Client, error) {
	svc, err := drive.NewService(ctx, option.WithTokenSource(ts))
	if err != nil {
		return nil, fmt.Errorf("drive: new service: %w", err)
	}
	return &Client{svc: svc}, nil
}
