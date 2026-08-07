package service

import (
	"context"
	"time"

	"todo-app/internal/models"

	"github.com/google/uuid"
)

// Task is the board/task use-case boundary consumed by the HTTP handlers.
type Task interface {
	CreateTask(ctx context.Context, userID uuid.UUID, in models.TaskCreate) (*models.Task, error)
	GetTasks(ctx context.Context, userID uuid.UUID, filter models.TaskFilter) ([]models.Task, error)
	GetTask(ctx context.Context, userID, taskID uuid.UUID) (*models.Task, error)
	UpdateTask(ctx context.Context, userID, taskID uuid.UUID, patch models.TaskPatch) (*models.Task, error)
	UpdateTaskStatus(ctx context.Context, taskID string, userID uuid.UUID, status models.TaskStatus, reviewer *string) error
	MoveTask(ctx context.Context, userID, taskID uuid.UUID, status models.TaskStatus, afterID, beforeID *uuid.UUID) (*models.Task, error)
	DeleteTask(ctx context.Context, userID, taskID uuid.UUID) error
	CreateTasksFromPlan(ctx context.Context, userID uuid.UUID, sprintID *uuid.UUID, plan *models.SprintPlan) ([]models.Task, error)
}

// Analytics turns raw task rows into the dashboard payload.
type Analytics interface {
	// ResolveQuery validates and normalises raw query-string values into a
	// concrete window. Empty strings fall back to sensible defaults
	// (period=week, granularity derived from the span, tz=UTC).
	ResolveQuery(period, from, to, granularity, tz string) (models.AnalyticsQuery, error)
	Dashboard(ctx context.Context, userID uuid.UUID, q models.AnalyticsQuery) (*models.Dashboard, error)
}

// PlanningStart opens an AI planning session from a rough list of tasks.
type PlanningStart struct {
	RawTasks             []string
	Notes                string
	HorizonWeeks         int
	CapacityHoursPerWeek float64
	StartsOn             *time.Time
	IncludeBacklog       bool
	Lang                 string
}

// PlanningAnswer answers one clarifying question.
type PlanningAnswer struct {
	QuestionID string
	Answer     string
}

// AI wraps the Gemini-backed features. Every method returns
// apperrors.ErrAIDisabled when no API key is configured.
type AI interface {
	Enabled() bool
	Model() string

	// Summary generates (or serves from cache) the narrative for a window.
	// lang is a short code such as "uk" or "en"; empty defaults to "uk".
	Summary(ctx context.Context, user *models.User, q models.AnalyticsQuery, lang string, refresh bool) (*models.AISummary, error)
	ListSummaries(ctx context.Context, userID uuid.UUID, limit int) ([]models.AISummary, error)

	StartPlanning(ctx context.Context, userID uuid.UUID, in PlanningStart) (*models.PlanningSession, error)
	AnswerPlanning(ctx context.Context, userID, sessionID uuid.UUID, answers []PlanningAnswer) (*models.PlanningSession, error)
	GeneratePlan(ctx context.Context, userID, sessionID uuid.UUID) (*models.PlanningSession, error)
	CommitPlan(ctx context.Context, userID, sessionID uuid.UUID) (*models.Sprint, []models.Task, error)
	GetSession(ctx context.Context, userID, sessionID uuid.UUID) (*models.PlanningSession, error)
	ListSessions(ctx context.Context, userID uuid.UUID, limit int) ([]models.PlanningSession, error)
	DeleteSession(ctx context.Context, userID, sessionID uuid.UUID) error
}

// SprintInput is the create/update payload for a sprint.
type SprintInput struct {
	Name          string
	Goal          *string
	StartsOn      time.Time
	EndsOn        time.Time
	CapacityHours *float64
	Status        models.SprintStatus
}

type Sprint interface {
	Create(ctx context.Context, userID uuid.UUID, in SprintInput) (*models.Sprint, error)
	Get(ctx context.Context, userID, sprintID uuid.UUID) (*models.Sprint, error)
	List(ctx context.Context, userID uuid.UUID) ([]models.Sprint, error)
	Update(ctx context.Context, userID, sprintID uuid.UUID, in SprintInput) (*models.Sprint, error)
	Delete(ctx context.Context, userID, sprintID uuid.UUID) error
}

// TargetInput is the create/update payload for an outbound export target.
// Every field is optional so an update is a true patch: a nil field means
// "leave the stored value alone". Create requires Name and URL.
type TargetInput struct {
	Name    *string
	URL     *string
	Secret  *string
	Headers map[string]string
	Enabled *bool
}

// PushRequest asks the export service to deliver a snapshot. Exactly one of
// TargetID or URL must be set.
type PushRequest struct {
	TargetID *uuid.UUID
	URL      string
	Secret   string
	Headers  map[string]string
	Kind     models.ExportKind
	Query    models.AnalyticsQuery
	Summary  *models.AISummary
}

type Subtask interface {
	List(ctx context.Context, userID, taskID uuid.UUID) ([]models.Subtask, error)
	Create(ctx context.Context, userID, taskID uuid.UUID, in models.SubtaskCreate) (*models.Subtask, error)
	Update(ctx context.Context, userID, subtaskID uuid.UUID, patch models.SubtaskPatch) (*models.Subtask, error)
	Delete(ctx context.Context, userID, subtaskID uuid.UUID) error
	Progress(ctx context.Context, userID uuid.UUID, taskIDs []uuid.UUID) (map[uuid.UUID]models.SubtaskProgress, error)
}

type Share interface {
	CreateLink(ctx context.Context, userID uuid.UUID, in ShareLinkInput) (*models.ShareLink, error)
	ListLinks(ctx context.Context, userID uuid.UUID) ([]models.ShareLink, error)
	RevokeLink(ctx context.Context, userID, linkID uuid.UUID) error
	DeleteLink(ctx context.Context, userID, linkID uuid.UUID) error
	Resolve(ctx context.Context, token string, q models.AnalyticsQuery) (*models.SharedView, error)
}

type Export interface {
	BuildEnvelope(ctx context.Context, user *models.User, kind models.ExportKind, q models.AnalyticsQuery, summary *models.AISummary) (*models.ExportEnvelope, error)
	Push(ctx context.Context, user *models.User, req PushRequest) (*models.ExportDelivery, error)

	CreateTarget(ctx context.Context, userID uuid.UUID, in TargetInput) (*models.ExportTarget, error)
	ListTargets(ctx context.Context, userID uuid.UUID) ([]models.ExportTarget, error)
	UpdateTarget(ctx context.Context, userID, targetID uuid.UUID, in TargetInput) (*models.ExportTarget, error)
	DeleteTarget(ctx context.Context, userID, targetID uuid.UUID) error
	ListDeliveries(ctx context.Context, userID uuid.UUID, limit int) ([]models.ExportDelivery, error)
}
