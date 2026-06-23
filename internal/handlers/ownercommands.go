package handlers

import (
	"context"
	"fmt"
	"os"
	"runtime"
	"strconv"
	"strings"
	"time"

	"gdrive-bot/internal/models"
	"gdrive-bot/internal/utils"

	"github.com/PaulSonOfLars/gotgbot/v2"
	"github.com/PaulSonOfLars/gotgbot/v2/ext"
	"github.com/shirou/gopsutil/v3/cpu"
	"github.com/shirou/gopsutil/v3/mem"
)

// requireOwner replies and returns false if the caller is not the bot owner.
func (d *Deps) requireOwner(b *gotgbot.Bot, ctx *ext.Context) bool {
	if d.Guard.IsOwner(ctx.EffectiveUser.Id) {
		return true
	}
	_, _ = ctx.EffectiveMessage.Reply(b, "🚫 Owner only.", nil)
	return false
}

// commandArg returns everything after the command name, trimmed.
func commandArg(ctx *ext.Context) string {
	parts := strings.SplitN(ctx.EffectiveMessage.Text, " ", 2)
	if len(parts) < 2 {
		return ""
	}
	return strings.TrimSpace(parts[1])
}

// ─── Auth management ─────────────────────────────────────────────────────────

func (d *Deps) Authorize(b *gotgbot.Bot, ctx *ext.Context) error {
	if !d.requireOwner(b, ctx) {
		return nil
	}
	targetID, err := strconv.ParseInt(commandArg(ctx), 10, 64)
	if err != nil {
		_, err := ctx.EffectiveMessage.Reply(b, "Usage: /auth <user_id>", nil)
		return err
	}
	c := context.Background()
	if _, err := d.DB.Users.EnsureExists(c, targetID, "", false); err != nil {
		return err
	}
	if err := d.DB.Users.SetAuthed(c, targetID, true, ctx.EffectiveUser.Id); err != nil {
		return err
	}
	d.Guard.Invalidate(c, targetID)
	_, err = ctx.EffectiveMessage.Reply(b,
		fmt.Sprintf("✅ User <code>%d</code> authorized.", targetID),
		&gotgbot.SendMessageOpts{ParseMode: "HTML"})
	return err
}

func (d *Deps) Unauth(b *gotgbot.Bot, ctx *ext.Context) error {
	if !d.requireOwner(b, ctx) {
		return nil
	}
	targetID, err := strconv.ParseInt(commandArg(ctx), 10, 64)
	if err != nil {
		_, err := ctx.EffectiveMessage.Reply(b, "Usage: /unauth <user_id>", nil)
		return err
	}
	c := context.Background()
	if err := d.DB.Users.SetAuthed(c, targetID, false, ctx.EffectiveUser.Id); err != nil {
		return err
	}
	d.Guard.Invalidate(c, targetID)
	_, err = ctx.EffectiveMessage.Reply(b, fmt.Sprintf("🚫 User <code>%d</code> unauthorized.", targetID),
		&gotgbot.SendMessageOpts{ParseMode: "HTML"})
	return err
}

// ─── Runtime tuning ───────────────────────────────────────────────────────────

func (d *Deps) SetWorkers(b *gotgbot.Bot, ctx *ext.Context) error {
	if !d.requireOwner(b, ctx) {
		return nil
	}
	n, err := strconv.Atoi(commandArg(ctx))
	if err != nil || n < 1 {
		_, err := ctx.EffectiveMessage.Reply(b, "Usage: /setworkers <positive integer>", nil)
		return err
	}
	if _, err := d.DB.Settings.Update(context.Background(), ctx.EffectiveUser.Id,
		func(s *models.Settings) { s.Workers = n }); err != nil {
		return err
	}
	d.Queue.Resize(n)
	_, err = ctx.EffectiveMessage.Reply(b, fmt.Sprintf("✅ Worker pool resized to %d.", n), nil)
	return err
}

func (d *Deps) SetQueue(b *gotgbot.Bot, ctx *ext.Context) error {
	if !d.requireOwner(b, ctx) {
		return nil
	}
	n, err := strconv.Atoi(commandArg(ctx))
	if err != nil || n < 1 {
		_, err := ctx.EffectiveMessage.Reply(b, "Usage: /setqueue <positive integer>", nil)
		return err
	}
	if _, err := d.DB.Settings.Update(context.Background(), ctx.EffectiveUser.Id,
		func(s *models.Settings) { s.QueueSize = n }); err != nil {
		return err
	}
	_, err = ctx.EffectiveMessage.Reply(b,
		fmt.Sprintf("✅ Queue size set to %d.\nThe new size takes effect after /restart.", n), nil)
	return err
}

func (d *Deps) SetDownloadLimit(b *gotgbot.Bot, ctx *ext.Context) error {
	if !d.requireOwner(b, ctx) {
		return nil
	}
	n, err := strconv.ParseInt(commandArg(ctx), 10, 64)
	if err != nil || n < 0 {
		_, err := ctx.EffectiveMessage.Reply(b, "Usage: /setdownloadlimit <MB, 0 = unlimited>", nil)
		return err
	}
	if _, err := d.DB.Settings.Update(context.Background(), ctx.EffectiveUser.Id,
		func(s *models.Settings) { s.DownloadLimitMB = n }); err != nil {
		return err
	}
	_, err = ctx.EffectiveMessage.Reply(b, fmt.Sprintf("✅ Download limit: %s.", limitLabel(n)), nil)
	return err
}

func (d *Deps) SetUploadLimit(b *gotgbot.Bot, ctx *ext.Context) error {
	if !d.requireOwner(b, ctx) {
		return nil
	}
	n, err := strconv.ParseInt(commandArg(ctx), 10, 64)
	if err != nil || n < 0 {
		_, err := ctx.EffectiveMessage.Reply(b, "Usage: /setuploadlimit <MB, 0 = unlimited>", nil)
		return err
	}
	if _, err := d.DB.Settings.Update(context.Background(), ctx.EffectiveUser.Id,
		func(s *models.Settings) { s.UploadLimitMB = n }); err != nil {
		return err
	}
	_, err = ctx.EffectiveMessage.Reply(b, fmt.Sprintf("✅ Upload limit: %s.", limitLabel(n)), nil)
	return err
}

func limitLabel(mb int64) string {
	if mb == 0 {
		return "unlimited"
	}
	return fmt.Sprintf("%d MB", mb)
}

// ─── Information & maintenance ────────────────────────────────────────────────

func (d *Deps) Stats(b *gotgbot.Bot, ctx *ext.Context) error {
	c := context.Background()

	cpuPct, _ := cpu.Percent(200*time.Millisecond, false)
	vmem, _ := mem.VirtualMemory()
	total, used, free, _ := utils.DiskUsage(d.Config.DownloadDir)

	queued, _ := d.DB.Tasks.CountByStatus(c, models.StatusQueued)
	downloading, _ := d.DB.Tasks.CountByStatus(c, models.StatusDownloading)
	uploading, _ := d.DB.Tasks.CountByStatus(c, models.StatusUploading)
	totalUsers, authedUsers, _ := d.DB.Users.CountAll(c)

	cpuStr := "n/a"
	if len(cpuPct) > 0 {
		cpuStr = fmt.Sprintf("%.1f%%", cpuPct[0])
	}

	text := fmt.Sprintf(
		"📊 <b>Stats</b>\n\n"+
			"<b>System</b>\n"+
			"CPU: %s\n"+
			"RAM: %s / %s (%.0f%%)\n"+
			"Disk: %s used / %s free / %s total\n\n"+
			"<b>Queue</b>\n"+
			"Workers: %d active, %d pending\n"+
			"Tasks — queued: %d  downloading: %d  uploading: %d\n\n"+
			"<b>Users</b>\n"+
			"%d authorized / %d total\n\n"+
			"<b>Uptime</b>: %s",
		cpuStr,
		utils.HumanBytes(int64(vmem.Used)), utils.HumanBytes(int64(vmem.Total)),
		float64(vmem.UsedPercent),
		utils.HumanBytes(int64(used)), utils.HumanBytes(int64(free)), utils.HumanBytes(int64(total)),
		d.Queue.ActiveCount(), d.Queue.PendingCount(),
		queued, downloading, uploading,
		authedUsers, totalUsers,
		utils.HumanDuration(time.Since(d.StartedAt)),
	)
	_, err := ctx.EffectiveMessage.Reply(b, text, &gotgbot.SendMessageOpts{ParseMode: "HTML"})
	return err
}

func (d *Deps) Disk(b *gotgbot.Bot, ctx *ext.Context) error {
	total, used, free, err := utils.DiskUsage(d.Config.DownloadDir)
	if err != nil {
		_, err := ctx.EffectiveMessage.Reply(b, "Couldn't read disk: "+err.Error(), nil)
		return err
	}
	text := fmt.Sprintf("💽 <b>Disk</b>\nUsed: %s\nFree: %s\nTotal: %s",
		utils.HumanBytes(int64(used)),
		utils.HumanBytes(int64(free)),
		utils.HumanBytes(int64(total)))
	_, err = ctx.EffectiveMessage.Reply(b, text, &gotgbot.SendMessageOpts{ParseMode: "HTML"})
	return err
}

func (d *Deps) Users(b *gotgbot.Bot, ctx *ext.Context) error {
	if !d.requireOwner(b, ctx) {
		return nil
	}
	users, err := d.DB.Users.ListAuthorized(context.Background())
	if err != nil {
		return err
	}
	var sb strings.Builder
	fmt.Fprintf(&sb, "👥 <b>%d authorized user(s)</b>\n\n", len(users))
	for _, u := range users {
		role := "user"
		if u.IsOwner {
			role = "owner"
		}
		uname := u.Username
		if uname == "" {
			uname = "(no username)"
		}
		fmt.Fprintf(&sb, "• <code>%d</code> — @%s [%s]\n", u.TelegramID, uname, role)
	}
	_, err = ctx.EffectiveMessage.Reply(b, sb.String(), &gotgbot.SendMessageOpts{ParseMode: "HTML"})
	return err
}

func (d *Deps) Broadcast(b *gotgbot.Bot, ctx *ext.Context) error {
	if !d.requireOwner(b, ctx) {
		return nil
	}
	msg := commandArg(ctx)
	if msg == "" {
		_, err := ctx.EffectiveMessage.Reply(b, "Usage: /broadcast <message>", nil)
		return err
	}
	users, err := d.DB.Users.ListAuthorized(context.Background())
	if err != nil {
		return err
	}
	sent := 0
	for _, u := range users {
		if u.TelegramID == ctx.EffectiveUser.Id {
			sent++ // count self
			continue
		}
		if _, e := b.SendMessage(u.TelegramID, "📢 "+msg, nil); e == nil {
			sent++
		}
	}
	_, err = ctx.EffectiveMessage.Reply(b,
		fmt.Sprintf("✅ Broadcast sent to %d/%d users.", sent, len(users)), nil)
	return err
}

func (d *Deps) Clean(b *gotgbot.Bot, ctx *ext.Context) error {
	if !d.requireOwner(b, ctx) {
		return nil
	}
	freed, _ := utils.DirSize(d.Config.DownloadDir)
	if err := os.RemoveAll(d.Config.DownloadDir); err != nil {
		_, err := ctx.EffectiveMessage.Reply(b, "Cleanup failed: "+err.Error(), nil)
		return err
	}
	_ = utils.EnsureDir(d.Config.DownloadDir)
	_, err := ctx.EffectiveMessage.Reply(b,
		fmt.Sprintf("🧹 Cleaned up downloads directory. Freed %s.", utils.HumanBytes(freed)), nil)
	return err
}

func (d *Deps) Settings(b *gotgbot.Bot, ctx *ext.Context) error {
	if !d.requireOwner(b, ctx) {
		return nil
	}
	s, err := d.DB.Settings.GetOrCreate(context.Background(), defaultSettings(d.Config))
	if err != nil {
		return err
	}
	text := fmt.Sprintf(
		"⚙️ <b>Settings</b>\n\n"+
			"/setworkers <n>          — workers: <b>%d</b>\n"+
			"/setqueue <n>            — queue size: <b>%d</b>\n"+
			"/setdownloadlimit <MB|0> — dl limit: <b>%s</b>\n"+
			"/setuploadlimit <MB|0>   — ul limit: <b>%s</b>\n\n"+
			"Explorer page size: %d\n"+
			"Max retries: %d\n"+
			"GOMAXPROCS: %d",
		s.Workers, s.QueueSize,
		limitLabel(s.DownloadLimitMB), limitLabel(s.UploadLimitMB),
		s.PageSize, s.MaxRetries, runtime.GOMAXPROCS(0),
	)
	_, err = ctx.EffectiveMessage.Reply(b, text, &gotgbot.SendMessageOpts{ParseMode: "HTML"})
	return err
}

func (d *Deps) Restart(b *gotgbot.Bot, ctx *ext.Context) error {
	if !d.requireOwner(b, ctx) {
		return nil
	}
	_, err := ctx.EffectiveMessage.Reply(b, "♻️ Restarting…", nil)
	go func() {
		time.Sleep(500 * time.Millisecond)
		os.Exit(0) // supervisor (systemd/pm2) brings it back up
	}()
	return err
}

func (d *Deps) Shutdown(b *gotgbot.Bot, ctx *ext.Context) error {
	if !d.requireOwner(b, ctx) {
		return nil
	}
	_, err := ctx.EffectiveMessage.Reply(b, "🛑 Shutting down.", nil)
	go func() {
		time.Sleep(500 * time.Millisecond)
		d.Queue.Stop()
		os.Exit(0)
	}()
	return err
}
