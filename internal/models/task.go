package models

import (
	"time"

	"github.com/google/uuid"
)

type TaskStatus string

const (
	StatusTodo       TaskStatus = "TODO"
	StatusInProgress TaskStatus = "IN_PROGRESS"
	StatusInReview   TaskStatus = "IN_REVIEW"
	StatusDone       TaskStatus = "DONE"
)

// BoardStatuses is the canonical left-to-right column order of the kanban board.
var BoardStatuses = []TaskStatus{StatusTodo, StatusInProgress, StatusInReview, StatusDone}

func (s TaskStatus) IsValid() bool {
	switch s {
	case StatusTodo, StatusInProgress, StatusInReview, StatusDone:
		return true
	}
	return false
}

type TaskPriority string

const (
	PriorityLow    TaskPriority = "LOW"
	PriorityMedium TaskPriority = "MEDIUM"
	PriorityHigh   TaskPriority = "HIGH"
	PriorityUrgent TaskPriority = "URGENT"
)

// Priorities is ordered from least to most urgent.
var Priorities = []TaskPriority{PriorityLow, PriorityMedium, PriorityHigh, PriorityUrgent}

func (p TaskPriority) IsValid() bool {
	switch p {
	case PriorityLow, PriorityMedium, PriorityHigh, PriorityUrgent:
		return true
	}
	return false
}

// Rank orders priorities for sorting: URGENT first.
func (p TaskPriority) Rank() int {
	switch p {
	case PriorityUrgent:
		return 0
	case PriorityHigh:
		return 1
	case PriorityMedium:
		return 2
	case PriorityLow:
		return 3
	}
	return 4
}

type Task struct {
	ID            uuid.UUID    `json:"id"`
	UserID        uuid.UUID    `json:"user_id"`
	Title         string       `json:"title"`
	Description   *string      `json:"description,omitempty"`
	Status        TaskStatus   `json:"status"`
	Priority      TaskPriority `json:"priority"`
	Reviewer      *string      `json:"reviewer,omitempty"`
	EstimateHours *float64     `json:"estimate_hours,omitempty"`
	BufferHours   *float64     `json:"buffer_hours,omitempty"`
	SpentHours    *float64     `json:"spent_hours,omitempty"`
	Blockers      *string      `json:"blockers,omitempty"`
	SprintID      *uuid.UUID   `json:"sprint_id,omitempty"`
	DueDate       *time.Time   `json:"due_date,omitempty"`
	StartedAt     *time.Time   `json:"started_at,omitempty"`
	CompletedAt   *time.Time   `json:"completed_at,omitempty"`
	Position      float64      `json:"position"`
	CreatedAt     time.Time    `json:"created_at"`
	UpdatedAt     time.Time    `json:"updated_at"`
}

// TaskCreate is the validated input for a new task.
type TaskCreate struct {
	Title         string
	Description   *string
	Status        TaskStatus
	Priority      TaskPriority
	Reviewer      *string
	EstimateHours *float64
	BufferHours   *float64
	Blockers      *string
	SprintID      *uuid.UUID
	DueDate       *time.Time
}

// TaskFilter narrows a task listing. Zero values mean "no filter".
type TaskFilter struct {
	Statuses   []TaskStatus
	Priorities []TaskPriority
	SprintID   *uuid.UUID
	Search     string
	From       *time.Time
	To         *time.Time
}

// TaskPatch carries a partial update. A nil field means "leave unchanged";
// a non-nil field whose inner pointer is nil means "clear this column".
type TaskPatch struct {
	Title         *string
	Description   **string
	Status        *TaskStatus
	Priority      *TaskPriority
	Reviewer      **string
	EstimateHours **float64
	BufferHours   **float64
	SpentHours    **float64
	Blockers      **string
	SprintID      **uuid.UUID
	DueDate       **time.Time
	Position      *float64

	// Set by the service from a status transition, never straight from a request.
	StartedAt   **time.Time
	CompletedAt **time.Time
}

type StatusChange struct {
	ID         uuid.UUID   `json:"id"`
	TaskID     uuid.UUID   `json:"task_id"`
	UserID     uuid.UUID   `json:"user_id"`
	FromStatus *TaskStatus `json:"from_status,omitempty"`
	ToStatus   TaskStatus  `json:"to_status"`
	ChangedAt  time.Time   `json:"changed_at"`
}
