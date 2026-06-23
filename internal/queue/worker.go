package queue

import (
	"context"
	"fmt"
	"log"
	"sync"
	"sync/atomic"

	"gdrive-bot/internal/database"
	"gdrive-bot/internal/models"
	"gdrive-bot/internal/utils"
)

// Manager owns the job channel, the live worker goroutines, and per-task
// retry bookkeeping. Worker count and queue size are both adjustable at
// runtime via Resize/Requeue without restarting the bot, per the
// /setworkers and /setqueue owner commands.
type Manager struct {
	db        *database.DB
	processor Processor
	maxRetries int

	mu      sync.Mutex
	jobs    chan *Job
	cancels map[string]context.CancelFunc // taskID -> cancel, for /cancel support
	active  int32                          // atomic count of in-flight jobs

	workerCancel context.CancelFunc
	workerWG     sync.WaitGroup
}

func NewManager(db *database.DB, processor Processor, queueSize, maxRetries int) *Manager {
	return &Manager{
		db:         db,
		processor:  processor,
		maxRetries: maxRetries,
		jobs:       make(chan *Job, queueSize),
		cancels:    make(map[string]context.CancelFunc),
	}
}

// Start launches `workers` worker goroutines. Call Stop before calling
// Start again (e.g. when /setworkers changes the count).
func (m *Manager) Start(workers int) {
	ctx, cancel := context.WithCancel(context.Background())
	m.workerCancel = cancel
	for i := 0; i < workers; i++ {
		m.workerWG.Add(1)
		go m.runWorker(ctx, i)
	}
	log.Printf("queue: started %d workers", workers)
}

// Stop signals all workers to finish their current job and exit, then
// waits for them. Used both at shutdown and when resizing the pool.
func (m *Manager) Stop() {
	if m.workerCancel != nil {
		m.workerCancel()
	}
	m.workerWG.Wait()
}

// Resize stops the current pool and starts a new one with the given
// worker count — used by /setworkers to take effect immediately.
func (m *Manager) Resize(workers int) {
	m.Stop()
	m.Start(workers)
}

// QueueSize returns the configured channel capacity (current /setqueue
// value), used by /stats.
func (m *Manager) QueueSize() int {
	return cap(m.jobs)
}

// ActiveCount returns how many jobs are currently being processed.
func (m *Manager) ActiveCount() int32 {
	return atomic.LoadInt32(&m.active)
}

// PendingCount returns how many jobs are sitting in the channel waiting
// for a free worker.
func (m *Manager) PendingCount() int {
	return len(m.jobs)
}

// Enqueue persists the task and pushes it onto the job channel. Returns
// an error immediately (without blocking) if the queue is full, so
// handlers can tell the user to retry rather than hanging.
func (m *Manager) Enqueue(ctx context.Context, t *models.Task) (*Job, error) {
	if t.ID == "" {
		t.ID = utils.NewID()
	}
	if err := m.db.Tasks.Insert(ctx, t); err != nil {
		return nil, fmt.Errorf("queue: persist task: %w", err)
	}

	jobCtx, cancel := context.WithCancel(context.Background())
	job := &Job{Task: t, Ctx: jobCtx, Cancel: cancel}

	m.mu.Lock()
	m.cancels[t.ID] = cancel
	m.mu.Unlock()

	select {
	case m.jobs <- job:
		return job, nil
	default:
		cancel()
		m.mu.Lock()
		delete(m.cancels, t.ID)
		m.mu.Unlock()
		_ = m.db.Tasks.UpdateStatus(ctx, t.ID, models.StatusFailed, "queue full")
		return nil, fmt.Errorf("queue: full (capacity %d)", cap(m.jobs))
	}
}

// Cancel stops a specific in-flight or queued task by ID, used for a
// future /cancel command and internally on /shutdown.
func (m *Manager) Cancel(taskID string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	cancel, ok := m.cancels[taskID]
	if ok {
		cancel()
		delete(m.cancels, taskID)
	}
	return ok
}

// Resume re-enqueues every task left in a non-terminal state from before
// a restart (StatusQueued/Downloading/Uploading), called once at startup.
func (m *Manager) Resume(ctx context.Context) error {
	pending, err := m.db.Tasks.Pending(ctx)
	if err != nil {
		return fmt.Errorf("queue: load pending tasks: %w", err)
	}
	for i := range pending {
		t := pending[i]
		jobCtx, cancel := context.WithCancel(context.Background())
		job := &Job{Task: &t, Ctx: jobCtx, Cancel: cancel}
		m.mu.Lock()
		m.cancels[t.ID] = cancel
		m.mu.Unlock()
		select {
		case m.jobs <- job:
		default:
			log.Printf("queue: could not resume task %s, queue full", t.ID)
		}
	}
	if len(pending) > 0 {
		log.Printf("queue: resumed %d task(s) from previous run", len(pending))
	}
	return nil
}

func (m *Manager) runWorker(ctx context.Context, id int) {
	defer m.workerWG.Done()
	for {
		select {
		case <-ctx.Done():
			return
		case job, ok := <-m.jobs:
			if !ok {
				return
			}
			m.process(job)
		}
	}
}

func (m *Manager) process(job *Job) {
	atomic.AddInt32(&m.active, 1)
	defer atomic.AddInt32(&m.active, -1)
	defer func() {
		m.mu.Lock()
		delete(m.cancels, job.Task.ID)
		m.mu.Unlock()
		job.Cancel()
	}()

	retryCfg := utils.DefaultRetryConfig(m.maxRetries)
	err := utils.Retry(job.Ctx, retryCfg, func(attempt int) error {
		if attempt > 1 {
			_ = m.db.Tasks.IncrementRetry(context.Background(), job.Task.ID)
		}
		return m.processor.Process(job)
	})

	if err != nil {
		log.Printf("queue: task %s failed permanently: %v", job.Task.ID, err)
		_ = m.db.Tasks.UpdateStatus(context.Background(), job.Task.ID, models.StatusFailed, err.Error())
	}
}
