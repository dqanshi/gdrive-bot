package drive

import (
	"context"
	"fmt"

	"google.golang.org/api/drive/v3"
)

// Rename updates a file/folder's display name.
func (c *Client) Rename(ctx context.Context, fileID, newName string) error {
	_, err := c.svc.Files.Update(fileID, &drive.File{Name: newName}).Context(ctx).SupportsAllDrives(true).Do()
	if err != nil {
		return fmt.Errorf("drive: rename: %w", err)
	}
	return nil
}

// Delete permanently removes a file/folder from Drive. The confirmation
// step lives in the handler layer — by the time this is called, the user
// has already confirmed.
func (c *Client) Delete(ctx context.Context, fileID string) error {
	if err := c.svc.Files.Delete(fileID).Context(ctx).SupportsAllDrives(true).Do(); err != nil {
		return fmt.Errorf("drive: delete: %w", err)
	}
	return nil
}

// SetPublic toggles "anyone with the link can view" sharing on or off.
// Turning sharing off removes the "anyone" permission rather than
// deleting the file's other (owner) permissions.
func (c *Client) SetPublic(ctx context.Context, fileID string, public bool) error {
	if public {
		_, err := c.svc.Permissions.Create(fileID, &drive.Permission{
			Type: "anyone",
			Role: "reader",
		}).Context(ctx).SupportsAllDrives(true).Do()
		if err != nil {
			return fmt.Errorf("drive: set public: %w", err)
		}
		return nil
	}

	perms, err := c.svc.Permissions.List(fileID).Context(ctx).Fields("permissions(id,type)").SupportsAllDrives(true).Do()
	if err != nil {
		return fmt.Errorf("drive: list permissions: %w", err)
	}
	for _, p := range perms.Permissions {
		if p.Type == "anyone" {
			if err := c.svc.Permissions.Delete(fileID, p.Id).Context(ctx).SupportsAllDrives(true).Do(); err != nil {
				return fmt.Errorf("drive: remove public permission: %w", err)
			}
		}
	}
	return nil
}

// IsPublic reports whether a file currently has an "anyone" reader
// permission, used to render the correct Share ON/OFF button state.
func (c *Client) IsPublic(ctx context.Context, fileID string) (bool, error) {
	perms, err := c.svc.Permissions.List(fileID).Context(ctx).Fields("permissions(type)").SupportsAllDrives(true).Do()
	if err != nil {
		return false, fmt.Errorf("drive: list permissions: %w", err)
	}
	for _, p := range perms.Permissions {
		if p.Type == "anyone" {
			return true, nil
		}
	}
	return false, nil
}

// ViewLink returns the standard Drive "view in browser" URL.
func ViewLink(fileID string) string {
	return fmt.Sprintf("https://drive.google.com/file/d/%s/view", fileID)
}

// DirectLink returns a direct-download URL. Note Google serves a virus-
// scan interstitial instead of the raw bytes for files over ~100MB; this
// is a Drive platform limitation, not something the bot can bypass.
func DirectLink(fileID string) string {
	return fmt.Sprintf("https://drive.google.com/uc?id=%s&export=download", fileID)
}

// CreateFolder creates a new folder under parentID, used internally if a
// destination folder needs to be created on the fly (e.g. for organizing
// uploads). Not exposed as a direct bot command in the base spec, but
// kept here since the explorer's "create folder" affordance is a natural
// extension owners may want to enable.
func (c *Client) CreateFolder(ctx context.Context, parentID, name string) (string, error) {
	f, err := c.svc.Files.Create(&drive.File{
		Name:     name,
		MimeType: driveFolderMimeType,
		Parents:  []string{parentID},
	}).Context(ctx).SupportsAllDrives(true).Do()
	if err != nil {
		return "", fmt.Errorf("drive: create folder: %w", err)
	}
	return f.Id, nil
}

// FileMeta fetches name/size/mimeType for a single file, used to populate
// the file menu (name, size shown to the user) and to decide whether an
// entry is a folder when only an ID is known (e.g. from a callback).
func (c *Client) FileMeta(ctx context.Context, fileID string) (Entry, error) {
	f, err := c.svc.Files.Get(fileID).Context(ctx).Fields("id, name, mimeType, size").SupportsAllDrives(true).Do()
	if err != nil {
		return Entry{}, fmt.Errorf("drive: get file: %w", err)
	}
	return Entry{
		ID:       f.Id,
		Name:     f.Name,
		IsFolder: f.MimeType == driveFolderMimeType,
		Size:     f.Size,
		MimeType: f.MimeType,
	}, nil
}
