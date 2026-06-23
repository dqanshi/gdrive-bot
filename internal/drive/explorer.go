package drive

import (
	"context"
	"fmt"

	"gdrive-bot/internal/models"
)

const driveFolderMimeType = "application/vnd.google-apps.folder"

// Entry is one row in the file explorer: either a folder or a file.
type Entry struct {
	ID       string
	Name     string
	IsFolder bool
	Size     int64
	MimeType string
}

// ListPage lists the contents of folderID (or, if query is non-empty,
// searches by name within folderID instead), one page at a time using
// Drive's native pageToken so "Next"/"Previous" don't need to re-walk
// from the start. Folders are listed before files, then alphabetically,
// matching typical file-manager UX.
func (c *Client) ListPage(ctx context.Context, folderID, query string, pageSize int, pageToken string) (entries []Entry, nextPageToken string, err error) {
	if folderID == "" {
		folderID = "root"
	}

	q := fmt.Sprintf("'%s' in parents and trashed = false", folderID)
	if query != "" {
		q = fmt.Sprintf("%s and name contains '%s'", q, escapeDriveQueryString(query))
	}

	call := c.svc.Files.List().
		Context(ctx).
		Q(q).
		PageSize(int64(pageSize)).
		Fields("nextPageToken, files(id, name, mimeType, size)").
		OrderBy("folder,name_natural").
		SupportsAllDrives(true).
		IncludeItemsFromAllDrives(true)
	if pageToken != "" {
		call = call.PageToken(pageToken)
	}

	res, err := call.Do()
	if err != nil {
		return nil, "", fmt.Errorf("drive: list files: %w", err)
	}

	for _, f := range res.Files {
		entries = append(entries, Entry{
			ID:       f.Id,
			Name:     f.Name,
			IsFolder: f.MimeType == driveFolderMimeType,
			Size:     f.Size,
			MimeType: f.MimeType,
		})
	}
	return entries, res.NextPageToken, nil
}

// FolderName fetches just the display name of a folder, used to extend
// the breadcrumb when the user opens a new folder.
func (c *Client) FolderName(ctx context.Context, folderID string) (string, error) {
	if folderID == "" || folderID == "root" {
		return "Root", nil
	}
	f, err := c.svc.Files.Get(folderID).Context(ctx).Fields("name").SupportsAllDrives(true).Do()
	if err != nil {
		return "", fmt.Errorf("drive: get folder name: %w", err)
	}
	return f.Name, nil
}

// BuildBreadcrumb walks parent links from folderID up to root, returning
// entries ordered Root -> ... -> folderID. Drive API doesn't expose a
// direct "path" field, so this is a bounded walk (capped at 25 levels to
// avoid any pathological loop).
func (c *Client) BuildBreadcrumb(ctx context.Context, folderID string) ([]models.BreadcrumbEntry, error) {
	if folderID == "" || folderID == "root" {
		return []models.BreadcrumbEntry{{FolderID: "root", Name: "Root"}}, nil
	}

	var chain []models.BreadcrumbEntry
	current := folderID
	for i := 0; i < 25 && current != ""; i++ {
		f, err := c.svc.Files.Get(current).Context(ctx).Fields("id, name, parents").SupportsAllDrives(true).Do()
		if err != nil {
			return nil, fmt.Errorf("drive: breadcrumb walk: %w", err)
		}
		chain = append([]models.BreadcrumbEntry{{FolderID: f.Id, Name: f.Name}}, chain...)
		if len(f.Parents) == 0 {
			break
		}
		current = f.Parents[0]
	}
	return append([]models.BreadcrumbEntry{{FolderID: "root", Name: "Root"}}, chain...), nil
}

// escapeDriveQueryString escapes single quotes per Drive's query syntax.
func escapeDriveQueryString(s string) string {
	out := make([]rune, 0, len(s))
	for _, r := range s {
		if r == '\'' {
			out = append(out, '\\')
		}
		out = append(out, r)
	}
	return string(out)
}

