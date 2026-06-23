package models

import "time"

// BreadcrumbEntry is one segment of the folder path shown above the
// explorer listing, e.g. Root / Movies / Anime / Season1.
type BreadcrumbEntry struct {
	FolderID string `bson:"folder_id"`
	Name     string `bson:"name"`
}

// ExplorerState is keyed per (user, chat, message) so the same message can
// be edited in place as the user paginates, opens folders, or searches,
// rather than the bot ever sending a new message.
type ExplorerState struct {
	UserID    int64  `bson:"user_id"`
	ChatID    int64  `bson:"chat_id"`
	MessageID int64  `bson:"message_id"`

	CurrentFolderID string            `bson:"current_folder_id"`
	Breadcrumb      []BreadcrumbEntry `bson:"breadcrumb"`

	Page     int `bson:"page"`
	PageSize int `bson:"page_size"`

	// SearchQuery, when non-empty, scopes the listing to a Drive
	// full-text/name search within CurrentFolderID instead of a plain
	// folder listing.
	SearchQuery string `bson:"search_query,omitempty"`

	// NextPageToken cache from the Drive API for the current page,
	// keyed by page number, so "Previous" doesn't require re-walking
	// from page 1.
	PageTokens map[int]string `bson:"page_tokens,omitempty"`

	// AwaitingRename, when set, means the user's next text message should
	// be treated as the new filename for this Drive file ID.
	AwaitingRenameFileID string `bson:"awaiting_rename_file_id,omitempty"`

	// AwaitingSearch, when true, means the user's next text message
	// should be treated as a search query for this explorer message.
	AwaitingSearch bool `bson:"awaiting_search,omitempty"`

	UpdatedAt time.Time `bson:"updated_at"`
}
