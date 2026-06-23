package bot

import (
	"context"
	"log"
	"os"
	"path/filepath"
	"time"

	"gdrive-bot/internal/database"
	"gdrive-bot/internal/models"
	"gdrive-bot/internal/utils"
)

// Cleaner periodically removes old local files from the downloads
// directory, preventing disk exhaustion on long-running VPS deployments.
// It only removes files whose associated tasks are in a terminal state
// (completed, failed, canceled) so in-progress downloads are never
// interrupted.
type Cleaner struct {
	downloadDir string
	db          *database.DB
	interval    time.Duration
}

func NewCleaner(downloadDir string, db *database.DB, intervalMin int) *Cleaner {
	if intervalMin <= 0 {
		intervalMin = 60
	}
	return &Cleaner{
		downloadDir: downloadDir,
		db:          db,
		interval:    time.Duration(intervalMin) * time.Minute,
	}
}

// Start runs the cleanup loop in a goroutine, stopping when ctx is
// cancelled (i.e. on /shutdown or SIGTERM).
func (c *Cleaner) Start(ctx context.Context) {
	go c.loop(ctx)
}

func (c *Cleaner) loop(ctx context.Context) {
	ticker := time.NewTicker(c.interval)
	defer ticker.Stop()
	log.Printf("cleaner: started (interval %v)", c.interval)
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			freed, n := c.run(ctx)
			if n > 0 {
				log.Printf("cleaner: removed %d file(s), freed %s", n, utils.HumanBytes(freed))
			}
		}
	}
}

// RunOnce is the same logic called manually by /clean.
func (c *Cleaner) RunOnce() (freed int64, removed int) {
	return c.run(context.Background())
}

func (c *Cleaner) run(ctx context.Context) (freed int64, removed int) {
	entries, err := os.ReadDir(c.downloadDir)
	if err != nil {
		return 0, 0
	}

	// Build a set of task IDs still in a non-terminal state so we can
	// protect their local files from deletion.
	active := c.activeTaskIDs(ctx)

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		// Task-owned files are named "<taskID>_<filename>". Extract the
		// task ID prefix so we can check if it's still active.
		taskID := taskIDFromFilename(name)
		if taskID != "" && active[taskID] {
			continue // task is still running — leave the file alone
		}
		path := filepath.Join(c.downloadDir, name)
		info, statErr := os.Stat(path)
		if statErr != nil {
			continue
		}
		if err := os.Remove(path); err == nil {
			freed += info.Size()
			removed++
		}
	}
	return freed, removed
}

func (c *Cleaner) activeTaskIDs(ctx context.Context) map[string]bool {
	tasks, err := c.db.Tasks.Pending(ctx)
	if err != nil {
		return map[string]bool{}
	}
	m := make(map[string]bool, len(tasks))
	for _, t := range tasks {
		if t.Status == models.StatusDownloading || t.Status == models.StatusUploading {
			m[t.ID] = true
		}
	}
	return m
}

// taskIDFromFilename extracts the hex task ID prefix from the naming
// convention "<16-char-hex>_<filename>" used by processor.go.
func taskIDFromFilename(name string) string {
	if len(name) < 17 || name[16] != '_' {
		return ""
	}
	prefix := name[:16]
	for _, c := range prefix {
		if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f')) {
			return ""
		}
	}
	return prefix
}
