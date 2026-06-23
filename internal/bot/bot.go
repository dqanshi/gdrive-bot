// Package bot is the composition root: it wires config, database, cache,
// auth, rclone, queue, and explorer together, registers all
// command/callback/message handlers with the gotgbot dispatcher, and
// starts long-polling.
package bot

import (
	"context"
	"log"
	"time"

	"gdrive-bot/internal/auth"
	"gdrive-bot/internal/cache"
	"gdrive-bot/internal/callbacks"
	"gdrive-bot/internal/config"
	"gdrive-bot/internal/database"
	"gdrive-bot/internal/explorer"
	"gdrive-bot/internal/handlers"
	"gdrive-bot/internal/middleware"
	"gdrive-bot/internal/models"
	"gdrive-bot/internal/queue"
	"gdrive-bot/internal/rclone"
	"gdrive-bot/internal/utils"

	"github.com/PaulSonOfLars/gotgbot/v2"
	"github.com/PaulSonOfLars/gotgbot/v2/ext"
)

// Run wires everything together and blocks until the context is
// cancelled (SIGTERM / /shutdown / /restart).
func Run(ctx context.Context, cfg *config.Config) error {
	// ── Directories ──────────────────────────────────────────────────────
	if err := utils.EnsureDir(cfg.DownloadDir); err != nil {
		return err
	}
	if err := utils.EnsureDir(cfg.LogDir); err != nil {
		return err
	}

	// ── MongoDB ──────────────────────────────────────────────────────────
	db, err := database.Connect(ctx, cfg.MongoURI, cfg.MongoDBName)
	if err != nil {
		return err
	}
	defer db.Disconnect(context.Background())
	log.Println("bot: MongoDB connected")

	// ── Cache (Redis or in-memory fallback) ──────────────────────────────
	c := cache.New(cfg.RedisAddr, cfg.RedisPassword, cfg.RedisDB)

	// ── Services ─────────────────────────────────────────────────────────
	guard := middleware.NewGuard(db, c, cfg.OwnerID)
	authMgr := auth.NewManager(cfg.GoogleClientID, cfg.GoogleClientSecret, cfg.GoogleRedirectURI)
	rcMgr := rclone.NewManager(cfg.RclonePath, cfg.RcloneConfigPath)
	explSvc := explorer.NewService(db, c)

	// ── Settings bootstrap ───────────────────────────────────────────────
	defaultS := models.DefaultSettings(
		cfg.DefaultWorkers, cfg.DefaultQueueSize,
		cfg.DefaultDownloadLimitMB, cfg.DefaultUploadLimitMB,
	)
	s, err := db.Settings.GetOrCreate(ctx, defaultS)
	if err != nil {
		log.Printf("bot: settings load error (using defaults): %v", err)
		s = &defaultS
	}

	// ── Telegram client ──────────────────────────────────────────────────
	b, err := gotgbot.NewBot(cfg.BotToken, nil)
	if err != nil {
		return err
	}
	log.Printf("bot: authenticated as @%s (id %d)", b.User.Username, b.User.Id)

	// ── Deps (shared across all handlers) ───────────────────────────────
	deps := &handlers.Deps{
		Config:    cfg,
		DB:        db,
		Cache:     c,
		Guard:     guard,
		Auth:      authMgr,
		Rclone:    rcMgr,
		Explorer:  explSvc,
		StartedAt: time.Now(),
	}

	// ── Queue + Processor ────────────────────────────────────────────────
	proc := NewProcessor(b, deps)
	qMgr := queue.NewManager(db, proc, s.QueueSize, s.MaxRetries)
	deps.Queue = qMgr

	qMgr.Start(s.Workers)
	defer qMgr.Stop()

	if err := qMgr.Resume(ctx); err != nil {
		log.Printf("bot: queue resume warning: %v", err)
	}

	// ── Auto-cleaner ─────────────────────────────────────────────────────
	cleaner := NewCleaner(cfg.DownloadDir, db, s.AutoCleanIntervalMin)
	cleaner.Start(ctx)

	// ── Dispatcher ───────────────────────────────────────────────────────
	dispatcher := ext.NewDispatcher(&ext.DispatcherOpts{
		Error: func(b *gotgbot.Bot, ctx *ext.Context, err error) ext.DispatcherAction {
			log.Printf("handler error [user=%d]: %v",
				func() int64 {
					if ctx.EffectiveUser != nil {
						return ctx.EffectiveUser.Id
					}
					return 0
				}(), err)
			return ext.DispatcherActionNoop
		},
		MaxRoutines: ext.DefaultMaxRoutines,
	})

	cbRouter := callbacks.NewRouter(deps)
	registerHandlers(dispatcher, deps, cbRouter)

	// ── Long-polling ─────────────────────────────────────────────────────
	updater := ext.NewUpdater(dispatcher, nil)
	if err := updater.StartPolling(b, &ext.PollingOpts{
		DropPendingUpdates: true,
		GetUpdatesOpts:     &gotgbot.GetUpdatesOpts{Timeout: 10},
	}); err != nil {
		return err
	}
	log.Printf("bot: polling started — ready")

	<-ctx.Done()
	log.Printf("bot: context cancelled, shutting down")
	updater.Stop()
	return nil
}

// registerHandlers maps every command, callback, and message type to its
// handler behind the authorization guard.
func registerHandlers(d *ext.Dispatcher, deps *handlers.Deps, cbRouter *callbacks.Router) {
	// auth wrapper — checks authorization, sends denial + returns if not OK.
	auth := func(fn func(*gotgbot.Bot, *ext.Context) error) func(*gotgbot.Bot, *ext.Context) error {
		return func(b *gotgbot.Bot, ctx *ext.Context) error {
			ok, err := deps.Guard.RequireAuth(b, ctx)
			if !ok || err != nil {
				return err
			}
			return fn(b, ctx)
		}
	}

	// ── User commands ─────────────────────────────────────────────────────
	d.AddHandler(newCommandHandler("start", auth(deps.Start)))
	d.AddHandler(newCommandHandler("login", auth(deps.Login)))
	d.AddHandler(newCommandHandler("logout", auth(deps.Logout)))
	d.AddHandler(newCommandHandler("status", auth(deps.Status)))
	d.AddHandler(newCommandHandler("myfiles", auth(deps.MyFiles)))
	d.AddHandler(newCommandHandler("stats", auth(deps.Stats)))
	d.AddHandler(newCommandHandler("disk", auth(deps.Disk)))

	// ── Owner-only commands ───────────────────────────────────────────────
	d.AddHandler(newCommandHandler("auth", auth(deps.Auth)))
	d.AddHandler(newCommandHandler("unauth", auth(deps.Unauth)))
	d.AddHandler(newCommandHandler("setworkers", auth(deps.SetWorkers)))
	d.AddHandler(newCommandHandler("setqueue", auth(deps.SetQueue)))
	d.AddHandler(newCommandHandler("setdownloadlimit", auth(deps.SetDownloadLimit)))
	d.AddHandler(newCommandHandler("setuploadlimit", auth(deps.SetUploadLimit)))
	d.AddHandler(newCommandHandler("users", auth(deps.Users)))
	d.AddHandler(newCommandHandler("broadcast", auth(deps.Broadcast)))
	d.AddHandler(newCommandHandler("clean", auth(deps.Clean)))
	d.AddHandler(newCommandHandler("settings", auth(deps.Settings)))
	d.AddHandler(newCommandHandler("restart", auth(deps.Restart)))
	d.AddHandler(newCommandHandler("shutdown", auth(deps.Shutdown)))

	// ── File uploads (document, video, audio, photo) ──────────────────────
	d.AddHandler(newMessageHandler(
		func(msg *gotgbot.Message) bool {
			return msg.Document != nil || msg.Video != nil ||
				msg.Audio != nil || len(msg.Photo) > 0
		},
		auth(deps.IncomingFile),
	))

	// ── Plain-text (login URLs, rename replies, direct links) ─────────────
	d.AddHandler(newMessageHandler(
		func(msg *gotgbot.Message) bool { return msg.Text != "" },
		auth(deps.IncomingText),
	))

	// ── All inline button presses ─────────────────────────────────────────
	d.AddHandler(newCallbackHandler(cbRouter.Dispatch))
}
