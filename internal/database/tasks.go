package database

import (
	"context"
	"errors"
	"time"

	"gdrive-bot/internal/models"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
)

// TaskRepo persists queue tasks so the queue survives bot restarts: on
// boot, any task left in StatusQueued/StatusDownloading/StatusUploading is
// re-enqueued (see internal/queue.Manager.Resume).
type TaskRepo struct {
	col *mongo.Collection
}

func (r *TaskRepo) Insert(ctx context.Context, t *models.Task) error {
	t.CreatedAt = time.Now()
	t.Status = models.StatusQueued
	_, err := r.col.InsertOne(ctx, t)
	return err
}

func (r *TaskRepo) Get(ctx context.Context, id string) (*models.Task, error) {
	var t models.Task
	err := r.col.FindOne(ctx, bson.M{"_id": id}).Decode(&t)
	if errors.Is(err, mongo.ErrNoDocuments) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &t, nil
}

func (r *TaskRepo) UpdateStatus(ctx context.Context, id string, status models.TaskStatus, lastErr string) error {
	set := bson.M{"status": status}
	if lastErr != "" {
		set["last_error"] = lastErr
	}
	switch status {
	case models.StatusDownloading:
		set["started_at"] = time.Now()
	case models.StatusCompleted, models.StatusFailed, models.StatusCanceled:
		set["completed_at"] = time.Now()
	}
	_, err := r.col.UpdateByID(ctx, id, bson.M{"$set": set})
	return err
}

func (r *TaskRepo) SetResult(ctx context.Context, id, driveFileID string) error {
	_, err := r.col.UpdateByID(ctx, id, bson.M{"$set": bson.M{
		"result_file_id": driveFileID,
		"status":         models.StatusCompleted,
		"completed_at":   time.Now(),
	}})
	return err
}

func (r *TaskRepo) IncrementRetry(ctx context.Context, id string) error {
	_, err := r.col.UpdateByID(ctx, id, bson.M{"$inc": bson.M{"retry_count": 1}})
	return err
}

// Pending returns every task still queued or in-flight, ordered oldest
// first — used both for /stats and to resume the queue after a restart.
func (r *TaskRepo) Pending(ctx context.Context) ([]models.Task, error) {
	cur, err := r.col.Find(ctx, bson.M{"status": bson.M{"$in": []models.TaskStatus{
		models.StatusQueued, models.StatusDownloading, models.StatusUploading,
	}}})
	if err != nil {
		return nil, err
	}
	defer cur.Close(ctx)
	var tasks []models.Task
	if err := cur.All(ctx, &tasks); err != nil {
		return nil, err
	}
	return tasks, nil
}

// CountByStatus is used by /stats to show queue depth per state.
func (r *TaskRepo) CountByStatus(ctx context.Context, status models.TaskStatus) (int64, error) {
	return r.col.CountDocuments(ctx, bson.M{"status": status})
}

// CountForUser enforces per-user queue limits (non-owner/non-auth users
// could otherwise flood the global queue).
func (r *TaskRepo) CountForUser(ctx context.Context, userID int64, statuses []models.TaskStatus) (int64, error) {
	return r.col.CountDocuments(ctx, bson.M{"user_id": userID, "status": bson.M{"$in": statuses}})
}
