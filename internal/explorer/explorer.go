// Package explorer builds the interactive /myfiles file browser: it
// combines internal/drive (the actual Drive API calls) with persisted
// internal/models.ExplorerState (breadcrumb, pagination, search) to
// produce the text + inline keyboard for a single Telegram message that
// gets edited in place as the user navigates — never re-sent.
package explorer

import (
	"context"
	"fmt"
	"strings"
	"time"

	"gdrive-bot/internal/cache"
	"gdrive-bot/internal/database"
	"gdrive-bot/internal/drive"
	"gdrive-bot/internal/models"
	"gdrive-bot/internal/utils"

	"github.com/PaulSonOfLars/gotgbot/v2"
)

// Service ties together Drive access, persisted state, and caching for
// one user's explorer session(s).
type Service struct {
	db    *database.DB
	cache cache.Cache
}

func NewService(db *database.DB, c cache.Cache) *Service {
	return &Service{db: db, cache: c}
}

// View is the rendered result for a single explorer message.
type View struct {
	Text     string
	Keyboard [][]gotgbot.InlineKeyboardButton
}

// stateKey scopes cached listing pages per explorer message, so flipping
// pages back and forth doesn't re-hit the Drive API every time.
func (s *Service) listCacheKey(userID, chatID, messageID int64, folderID, query string, page int) string {
	return fmt.Sprintf("explist:%d:%d:%d:%s:%s:%d", userID, chatID, messageID, folderID, query, page)
}

type cachedPage struct {
	Entries       []drive.Entry
	NextPageToken string
}

// Open initializes (or resets) explorer state for a message to show the
// root folder, used by the /myfiles command handler.
func (s *Service) Open(ctx context.Context, dc *drive.Client, userID, chatID, messageID int64, pageSize int) (*View, error) {
	state := &models.ExplorerState{
		UserID:          userID,
		ChatID:          chatID,
		MessageID:       messageID,
		CurrentFolderID: "root",
		Page:            1,
		PageSize:        pageSize,
		PageTokens:      map[int]string{},
	}
	breadcrumb, err := dc.BuildBreadcrumb(ctx, "root")
	if err != nil {
		return nil, err
	}
	state.Breadcrumb = breadcrumb
	if err := s.db.ExplorerStates.Upsert(ctx, state); err != nil {
		return nil, err
	}
	return s.render(ctx, dc, state)
}

// OpenFolder navigates into a subfolder, extending the breadcrumb.
func (s *Service) OpenFolder(ctx context.Context, dc *drive.Client, userID, chatID, messageID int64, folderID string) (*View, error) {
	state, err := s.mustState(ctx, userID, chatID, messageID)
	if err != nil {
		return nil, err
	}
	name, err := dc.FolderName(ctx, folderID)
	if err != nil {
		return nil, err
	}
	state.Breadcrumb = append(state.Breadcrumb, models.BreadcrumbEntry{FolderID: folderID, Name: name})
	state.CurrentFolderID = folderID
	state.Page = 1
	state.SearchQuery = ""
	state.PageTokens = map[int]string{}
	if err := s.db.ExplorerStates.Upsert(ctx, state); err != nil {
		return nil, err
	}
	return s.render(ctx, dc, state)
}

// Back pops one breadcrumb level (or stays at root if already there).
func (s *Service) Back(ctx context.Context, dc *drive.Client, userID, chatID, messageID int64) (*View, error) {
	state, err := s.mustState(ctx, userID, chatID, messageID)
	if err != nil {
		return nil, err
	}
	if len(state.Breadcrumb) > 1 {
		state.Breadcrumb = state.Breadcrumb[:len(state.Breadcrumb)-1]
	}
	last := state.Breadcrumb[len(state.Breadcrumb)-1]
	state.CurrentFolderID = last.FolderID
	state.Page = 1
	state.SearchQuery = ""
	state.PageTokens = map[int]string{}
	if err := s.db.ExplorerStates.Upsert(ctx, state); err != nil {
		return nil, err
	}
	return s.render(ctx, dc, state)
}

// Home jumps straight back to the root folder.
func (s *Service) Home(ctx context.Context, dc *drive.Client, userID, chatID, messageID int64) (*View, error) {
	state, err := s.mustState(ctx, userID, chatID, messageID)
	if err != nil {
		return nil, err
	}
	state.Breadcrumb = state.Breadcrumb[:1]
	state.CurrentFolderID = "root"
	state.Page = 1
	state.SearchQuery = ""
	state.PageTokens = map[int]string{}
	if err := s.db.ExplorerStates.Upsert(ctx, state); err != nil {
		return nil, err
	}
	return s.render(ctx, dc, state)
}

// Page moves forward (delta=1) or backward (delta=-1) one page within the
// current folder/search scope.
func (s *Service) Page(ctx context.Context, dc *drive.Client, userID, chatID, messageID int64, delta int) (*View, error) {
	state, err := s.mustState(ctx, userID, chatID, messageID)
	if err != nil {
		return nil, err
	}
	newPage := state.Page + delta
	if newPage < 1 {
		newPage = 1
	}
	state.Page = newPage
	if err := s.db.ExplorerStates.Upsert(ctx, state); err != nil {
		return nil, err
	}
	return s.render(ctx, dc, state)
}

// Search scopes the current folder listing to entries whose name
// contains query.
func (s *Service) Search(ctx context.Context, dc *drive.Client, userID, chatID, messageID int64, query string) (*View, error) {
	state, err := s.mustState(ctx, userID, chatID, messageID)
	if err != nil {
		return nil, err
	}
	state.SearchQuery = query
	state.Page = 1
	state.PageTokens = map[int]string{}
	if err := s.db.ExplorerStates.Upsert(ctx, state); err != nil {
		return nil, err
	}
	return s.render(ctx, dc, state)
}

// ClearSearch drops back to a plain folder listing.
func (s *Service) ClearSearch(ctx context.Context, dc *drive.Client, userID, chatID, messageID int64) (*View, error) {
	return s.Search(ctx, dc, userID, chatID, messageID, "")
}

// Refresh re-renders the current view, invalidating any cached page so
// freshly uploaded/renamed/deleted files show up immediately.
func (s *Service) Refresh(ctx context.Context, dc *drive.Client, userID, chatID, messageID int64) (*View, error) {
	state, err := s.mustState(ctx, userID, chatID, messageID)
	if err != nil {
		return nil, err
	}
	s.cache.InvalidatePrefix(ctx, fmt.Sprintf("explist:%d:%d:%d:", userID, chatID, messageID))
	return s.render(ctx, dc, state)
}

// AwaitRename marks the explorer state so the user's next text message is
// treated as the new name for fileID, then returns a prompt View used as
// the message body (caller is expected to send/edit with a "Send new
// filename." prompt and a Cancel button — see handlers/myfiles.go).
func (s *Service) AwaitRename(ctx context.Context, userID, chatID, messageID int64, fileID string) error {
	state, err := s.mustState(ctx, userID, chatID, messageID)
	if err != nil {
		return err
	}
	state.AwaitingRenameFileID = fileID
	return s.db.ExplorerStates.Upsert(ctx, state)
}

// PendingPrompt describes an explorer message currently waiting on a
// text reply (either a rename or a search query).
type PendingPrompt struct {
	MessageID    int64
	RenameFileID string // non-empty if waiting on a rename
	IsSearch     bool   // true if waiting on a search query
}

// FindPending looks across this user's explorer messages in chatID for
// one currently waiting on a rename or search reply, consuming
// (clearing) whichever flag it finds so it only fires once.
func (s *Service) FindPending(ctx context.Context, userID, chatID int64) (*PendingPrompt, error) {
	state, err := s.db.ExplorerStates.FindAwaiting(ctx, userID, chatID)
	if err != nil {
		return nil, err
	}
	p := &PendingPrompt{MessageID: state.MessageID}
	if state.AwaitingRenameFileID != "" {
		p.RenameFileID = state.AwaitingRenameFileID
		state.AwaitingRenameFileID = ""
	} else if state.AwaitingSearch {
		p.IsSearch = true
		state.AwaitingSearch = false
	}
	if err := s.db.ExplorerStates.Upsert(ctx, state); err != nil {
		return nil, err
	}
	return p, nil
}

// AwaitSearch marks the explorer state so the user's next text message is
// treated as a search query, triggered by the "🔍 Search" button.
func (s *Service) AwaitSearch(ctx context.Context, userID, chatID, messageID int64) error {
	state, err := s.mustState(ctx, userID, chatID, messageID)
	if err != nil {
		return err
	}
	state.AwaitingSearch = true
	return s.db.ExplorerStates.Upsert(ctx, state)
}

// RenderCurrent re-renders the explorer at whatever folder/page/search it
// was last left at — used when backing out of the file or download menu
// back to the listing.
func (s *Service) RenderCurrent(ctx context.Context, dc *drive.Client, userID, chatID, messageID int64) (*View, error) {
	state, err := s.mustState(ctx, userID, chatID, messageID)
	if err != nil {
		return nil, err
	}
	return s.render(ctx, dc, state)
}

func (s *Service) mustState(ctx context.Context, userID, chatID, messageID int64) (*models.ExplorerState, error) {
	state, err := s.db.ExplorerStates.Get(ctx, userID, chatID, messageID)
	if err != nil {
		return nil, fmt.Errorf("explorer: state not found, send /myfiles again: %w", err)
	}
	if state.PageTokens == nil {
		state.PageTokens = map[int]string{}
	}
	return state, nil
}

// render fetches (or reuses a cached) page of entries for state's current
// folder/search/page and builds the resulting View.
func (s *Service) render(ctx context.Context, dc *drive.Client, state *models.ExplorerState) (*View, error) {
	key := s.listCacheKey(state.UserID, state.ChatID, state.MessageID, state.CurrentFolderID, state.SearchQuery, state.Page)

	var page cachedPage
	hit, _ := s.cache.Get(ctx, key, &page)
	if !hit {
		pageToken := state.PageTokens[state.Page]
		entries, nextToken, err := dc.ListPage(ctx, state.CurrentFolderID, state.SearchQuery, state.PageSize, pageToken)
		if err != nil {
			return nil, err
		}
		page = cachedPage{Entries: entries, NextPageToken: nextToken}
		_ = s.cache.Set(ctx, key, page, 2*time.Minute)

		if nextToken != "" {
			state.PageTokens[state.Page+1] = nextToken
			_ = s.db.ExplorerStates.Upsert(ctx, state)
		}
	}

	return &View{
		Text:     renderText(state, page.Entries),
		Keyboard: renderKeyboard(state, page.Entries),
	}, nil
}

func renderText(state *models.ExplorerState, entries []drive.Entry) string {
	var b strings.Builder

	var crumbs []string
	for _, c := range state.Breadcrumb {
		crumbs = append(crumbs, c.Name)
	}
	b.WriteString(strings.Join(crumbs, " / "))
	b.WriteString("\n")

	if state.SearchQuery != "" {
		fmt.Fprintf(&b, "🔎 Search: \"%s\"\n", state.SearchQuery)
	}
	b.WriteString("\n")

	if len(entries) == 0 {
		b.WriteString("(empty folder)")
		return b.String()
	}

	for _, e := range entries {
		if e.IsFolder {
			fmt.Fprintf(&b, "📂 %s\n", e.Name)
		} else {
			fmt.Fprintf(&b, "📄 %s — %s\n", e.Name, utils.HumanBytes(e.Size))
		}
	}
	fmt.Fprintf(&b, "\nPage %d", state.Page)
	return b.String()
}

func renderKeyboard(state *models.ExplorerState, entries []drive.Entry) [][]gotgbot.InlineKeyboardButton {
	var rows [][]gotgbot.InlineKeyboardButton

	for _, e := range entries {
		label := utils.TruncateMiddle(e.Name, 40)
		if e.IsFolder {
			label = "📂 " + label
			rows = append(rows, []gotgbot.InlineKeyboardButton{
				{Text: label, CallbackData: "expl:open:" + e.ID},
			})
		} else {
			label = "📄 " + label
			rows = append(rows, []gotgbot.InlineKeyboardButton{
				{Text: label, CallbackData: "expl:file:" + e.ID},
			})
		}
	}

	nav := []gotgbot.InlineKeyboardButton{}
	if state.Page > 1 {
		nav = append(nav, gotgbot.InlineKeyboardButton{Text: "⬅ Prev", CallbackData: "expl:prevpage"})
	}
	if _, hasNext := state.PageTokens[state.Page+1]; hasNext {
		nav = append(nav, gotgbot.InlineKeyboardButton{Text: "Next ➡", CallbackData: "expl:nextpage"})
	}
	if len(nav) > 0 {
		rows = append(rows, nav)
	}

	controls := []gotgbot.InlineKeyboardButton{
		{Text: "🔙 Back", CallbackData: "expl:back"},
		{Text: "🏠 Home", CallbackData: "expl:home"},
		{Text: "🔄 Refresh", CallbackData: "expl:refresh"},
	}
	rows = append(rows, controls)

	if state.SearchQuery != "" {
		rows = append(rows, []gotgbot.InlineKeyboardButton{
			{Text: "🔍 Search", CallbackData: "expl:search"},
			{Text: "❌ Clear Search", CallbackData: "expl:clearsearch"},
		})
	} else {
		rows = append(rows, []gotgbot.InlineKeyboardButton{
			{Text: "🔍 Search", CallbackData: "expl:search"},
		})
	}

	return rows
}
