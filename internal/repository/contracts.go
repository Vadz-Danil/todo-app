package repository

import (
	"context"
	"time"

	"todo-app/internal/models"

	"github.com/google/uuid"
)

// TaskRepository is the persistence contract for board tasks.
//
// Ownership is enforced in SQL: every method takes userID and must scope its
// WHERE clause by it, returning apperrors.ErrTaskNotFound when nothing matches.
type TaskRepository interface {
	CreateTask(ctx context.Context, task *models.Task) error
	CreateTasksBulk(ctx context.Context, tasks []models.Task) error
	GetTaskByID(ctx context.Context, userID, taskID uuid.UUID) (*models.Task, error)
	ListTasks(ctx context.Context, userID uuid.UUID, filter models.TaskFilter) ([]models.Task, error)
	UpdateTask(ctx context.Context, userID, taskID uuid.UUID, patch models.TaskPatch) (*models.Task, error)
	DeleteTask(ctx context.Context, userID, taskID uuid.UUID) error

	// AppendPosition returns a position value that places a task at the end of
	// the given column.
	AppendPosition(ctx context.Context, userID uuid.UUID, status models.TaskStatus) (float64, error)

	// PositionBetween returns a position value that places a task between two
	// neighbours in the given column. afterID is the task that ends up directly
	// ABOVE the moved task, beforeID the one directly BELOW it. Either may be
	// nil to mean "top of column" / "bottom of column" respectively.
	PositionBetween(ctx context.Context, userID uuid.UUID, status models.TaskStatus, afterID, beforeID *uuid.UUID) (float64, error)

	RecordStatusChange(ctx context.Context, change *models.StatusChange) error
}

// AnalyticsRepository serves raw rows; all aggregation happens in the service
// layer so it stays unit-testable without a database.
type AnalyticsRepository interface {
	// LoadWindow returns every task relevant to [from, to]: anything created at
	// or before `to` that is either still open or was completed at or after
	// `from`. That covers created-in-range, completed-in-range and open-at-end.
	LoadWindow(ctx context.Context, userID uuid.UUID, from, to time.Time) ([]models.Task, error)

	// GlobalCounts returns the all-time task total and the per-status snapshot.
	GlobalCounts(ctx context.Context, userID uuid.UUID) (int, map[models.TaskStatus]int, error)

	// CompletionDays returns all-time completion counts grouped by calendar day
	// in loc (YYYY-MM-DD), ascending, for streak calculation. It takes a
	// *time.Location rather than a zone name so the zone is resolved by Go's
	// tzdata, which the database's own copy may not agree with.
	CompletionDays(ctx context.Context, userID uuid.UUID, loc *time.Location) ([]models.DayCount, error)

	StatusChanges(ctx context.Context, userID uuid.UUID, from, to time.Time) ([]models.StatusChange, error)

	SprintSummaries(ctx context.Context, userID uuid.UUID, from, to time.Time) ([]models.SprintStatsItem, error)

	// FirstTaskAt is the creation time of the user's oldest task; nil when the
	// user has no tasks. Used to bound the "all_time" period.
	FirstTaskAt(ctx context.Context, userID uuid.UUID) (*time.Time, error)
}

type SprintRepository interface {
	CreateSprint(ctx context.Context, sprint *models.Sprint) error
	GetSprintByID(ctx context.Context, userID, sprintID uuid.UUID) (*models.Sprint, error)
	ListSprints(ctx context.Context, userID uuid.UUID) ([]models.Sprint, error)
	UpdateSprint(ctx context.Context, sprint *models.Sprint) error
	DeleteSprint(ctx context.Context, userID, sprintID uuid.UUID) error
}

type PlanningRepository interface {
	CreateSession(ctx context.Context, session *models.PlanningSession) error
	GetSession(ctx context.Context, userID, sessionID uuid.UUID) (*models.PlanningSession, error)
	ListSessions(ctx context.Context, userID uuid.UUID, limit int) ([]models.PlanningSession, error)
	UpdateSession(ctx context.Context, session *models.PlanningSession) error
	DeleteSession(ctx context.Context, userID, sessionID uuid.UUID) error
}

type SummaryRepository interface {
	// GetByFingerprint returns apperrors.ErrSessionNotFound-free semantics: a
	// cache miss yields (nil, nil).
	GetByFingerprint(ctx context.Context, userID uuid.UUID, fingerprint string) (*models.AISummary, error)
	SaveSummary(ctx context.Context, summary *models.AISummary) error
	ListSummaries(ctx context.Context, userID uuid.UUID, limit int) ([]models.AISummary, error)
}

type SubtaskRepository interface {
	ListByTask(ctx context.Context, userID, taskID uuid.UUID) ([]models.Subtask, error)
	ProgressByTasks(ctx context.Context, userID uuid.UUID, taskIDs []uuid.UUID) (map[uuid.UUID]models.SubtaskProgress, error)
	NextPosition(ctx context.Context, userID, taskID uuid.UUID) (float64, error)
	Create(ctx context.Context, item *models.Subtask) error
	Update(ctx context.Context, userID, subtaskID uuid.UUID, patch models.SubtaskPatch) (*models.Subtask, error)
	Delete(ctx context.Context, userID, subtaskID uuid.UUID) error
}

type ShareRepository interface {
	CreateLink(ctx context.Context, link *models.ShareLink) error
	GetByTokenHash(ctx context.Context, tokenHash string) (*models.ShareLink, error)
	ListLinks(ctx context.Context, userID uuid.UUID) ([]models.ShareLink, error)
	RevokeLink(ctx context.Context, userID, linkID uuid.UUID, at time.Time) error
	DeleteLink(ctx context.Context, userID, linkID uuid.UUID) error
	TouchLink(ctx context.Context, linkID uuid.UUID, at time.Time) error
}

type ExportRepository interface {
	CreateTarget(ctx context.Context, target *models.ExportTarget) error
	GetTarget(ctx context.Context, userID, targetID uuid.UUID) (*models.ExportTarget, error)
	ListTargets(ctx context.Context, userID uuid.UUID) ([]models.ExportTarget, error)
	UpdateTarget(ctx context.Context, target *models.ExportTarget) error
	DeleteTarget(ctx context.Context, userID, targetID uuid.UUID) error
	TouchTarget(ctx context.Context, userID, targetID uuid.UUID, statusCode *int, errMsg *string, sentAt time.Time) error
	RecordDelivery(ctx context.Context, delivery *models.ExportDelivery) error
	ListDeliveries(ctx context.Context, userID uuid.UUID, limit int) ([]models.ExportDelivery, error)
}
