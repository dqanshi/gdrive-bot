package models

import "time"

// User represents an authorized (or owner) Telegram user and, once they've
// completed /login, their Google OAuth credentials + rclone remote name.
type User struct {
	TelegramID int64     `bson:"telegram_id"`
	Username   string    `bson:"username,omitempty"`
	IsOwner    bool      `bson:"is_owner"`
	IsAuthed   bool      `bson:"is_authed"`
	AuthedBy   int64     `bson:"authed_by,omitempty"`
	AuthedAt   time.Time `bson:"authed_at,omitempty"`

	// Google OAuth token. AccessToken is short-lived and refreshed
	// automatically using RefreshToken — the user only logs in once.
	GoogleToken *GoogleToken `bson:"google_token,omitempty"`

	// RcloneRemote is the name of the rclone remote created for this
	// user during /login, e.g. "user_123456789".
	RcloneRemote string `bson:"rclone_remote,omitempty"`

	// RootFolderID, if set, scopes /myfiles to a specific Drive folder
	// instead of "root". Empty means full Drive root.
	RootFolderID string `bson:"root_folder_id,omitempty"`

	CreatedAt time.Time `bson:"created_at"`
	UpdatedAt time.Time `bson:"updated_at"`
}

// GoogleToken mirrors golang.org/x/oauth2.Token fields, persisted in Mongo.
type GoogleToken struct {
	AccessToken  string    `bson:"access_token"`
	RefreshToken string    `bson:"refresh_token"`
	TokenType    string    `bson:"token_type"`
	Expiry       time.Time `bson:"expiry"`
}

// PendingLogin tracks an in-progress /login conversation so the bot knows
// what to do with the next message the user sends (the redirect URL, then
// the auth code). Stored in Mongo (not memory) so a bot restart mid-login
// doesn't strand the user.
type PendingLogin struct {
	TelegramID  int64     `bson:"telegram_id"`
	Step        int       `bson:"step"` // 1 = waiting for redirect URL, 2 = waiting for code
	AuthCode    string    `bson:"auth_code,omitempty"`
	MessageID   int64     `bson:"message_id"` // message being edited throughout the flow
	ChatID      int64     `bson:"chat_id"`
	CreatedAt   time.Time `bson:"created_at"`
	ExpiresAt   time.Time `bson:"expires_at"`
}
